package securehttp

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// TLSPolicy is connection-scoped trust material. The CA is public trust data,
// not a credential; it is nevertheless bounded and included in the immutable
// Repository Connection digest.
type TLSPolicy struct {
	CAPEM      string `json:"caPem,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	SPKISHA256 string `json:"spkiSha256,omitempty"`
}

func NewClient(raw []byte, targetHost string, timeout time.Duration) (*http.Client, error) {
	transport, err := NewTransport(raw, targetHost)
	if err != nil {
		return nil, err
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return errors.New("SECURE_HTTP_REDIRECT_FORBIDDEN")
		},
	}, nil
}

// NewPinnedClient keeps the DNS result checked by the caller for the full
// request lifetime. TLS still presents targetHost as SNI and is verified
// against targetHost, while the dial itself can only reach one of the already
// authorised addresses. This closes the validation-to-connect DNS rebinding
// gap without accepting a redirect or an environment proxy.
func NewPinnedClient(raw []byte, targetHost string, targetPort int, addresses []net.IP, timeout time.Duration) (*http.Client, error) {
	if targetPort < 1 || targetPort > 65535 || len(addresses) == 0 {
		return nil, errors.New("SECURE_HTTP_PINNED_ADDRESS_INVALID")
	}
	transport, err := NewTransport(raw, targetHost)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	firstAddress := ""
	for _, address := range addresses {
		if address == nil || address.IsUnspecified() {
			return nil, errors.New("SECURE_HTTP_PINNED_ADDRESS_INVALID")
		}
		allowed[address.String()] = struct{}{}
		// Prefer IPv4 when both loopback families are authorized. Docker Desktop
		// localhost port publishing is commonly IPv4-only; this remains pinned
		// to an already authorized address and does not introduce fallback DNS.
		if firstAddress == "" || (net.ParseIP(firstAddress).To4() == nil && address.To4() != nil) {
			firstAddress = address.String()
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, splitErr := net.SplitHostPort(address)
		if splitErr != nil || !strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(targetHost, ".")) || port != strconv.Itoa(targetPort) {
			return nil, errors.New("SECURE_HTTP_DIAL_TARGET_MISMATCH")
		}
		// Deterministically choose the first authorised address. A caller may
		// retry the complete operation after fresh policy validation instead of
		// silently switching to a newly resolved destination mid-request.
		if _, ok := allowed[firstAddress]; !ok {
			return nil, errors.New("SECURE_HTTP_PINNED_ADDRESS_INVALID")
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(firstAddress, port))
	}
	// A connection whose destination has been pinned by a policy must be able to
	// operate through the common local and enterprise TLS forwarders used by
	// repository providers.  GitHub's API, for example, can reject the custom
	// DialContext + HTTP/2 negotiation while serving the same pinned TLS
	// connection over HTTP/1.1.  HTTP/2 is an optimisation rather than an
	// authority guarantee, so keep the connection on HTTP/1.1 here.  The target
	// address, SNI/certificate policy, no-redirect rule and no-proxy rule remain
	// unchanged.
	transport.ForceAttemptHTTP2 = false
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("SECURE_HTTP_REDIRECT_FORBIDDEN")
	}}, nil
}

func NewTransport(raw []byte, targetHost string) (*http.Transport, error) {
	policy, err := ParsePolicy(raw, targetHost)
	if err != nil {
		return nil, err
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if policy.CAPEM != "" && !roots.AppendCertsFromPEM([]byte(policy.CAPEM)) {
		return nil, errors.New("TLS_POLICY_CA_INVALID")
	}
	appendRuntimeGitCAs(roots)
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: policy.ServerName}
	if policy.SPKISHA256 != "" {
		want, _ := hex.DecodeString(policy.SPKISHA256)
		tlsConfig.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("TLS_POLICY_CERTIFICATE_MISSING")
			}
			sum := sha256.Sum256(state.PeerCertificates[0].RawSubjectPublicKeyInfo)
			if !equalBytes(sum[:], want) {
				return errors.New("TLS_POLICY_SPKI_MISMATCH")
			}
			return nil
		}
	}
	return &http.Transport{
		// Repository and review traffic is governed by the connection-scoped
		// egress policy. Inheriting HTTP(S)_PROXY would introduce an unpinned
		// second destination outside that policy.
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsConfig,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}, nil
}

// appendRuntimeGitCAs carries the host's explicitly configured Git trust roots
// into provider API readback. The paths are operator-owned runtime
// configuration, bounded to regular files, and never accepted from a Project
// or Task request.
func appendRuntimeGitCAs(roots *x509.CertPool) {
	for _, name := range []string{"GIT_SSL_CAINFO", "SSL_CERT_FILE"} {
		path := strings.TrimSpace(os.Getenv(name))
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 1<<20 {
			continue
		}
		pem, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		roots.AppendCertsFromPEM(pem)
		for index := range pem {
			pem[index] = 0
		}
	}
}

func ParsePolicy(raw []byte, targetHost string) (TLSPolicy, error) {
	policy := TLSPolicy{}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed != "" && trimmed != "{}" && trimmed != "null" {
		if len(raw) > 128<<10 {
			return policy, errors.New("TLS_POLICY_TOO_LARGE")
		}
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&policy); err != nil {
			return policy, errors.New("TLS_POLICY_INVALID")
		}
	}
	targetHost = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(targetHost), "."))
	policy.ServerName = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(policy.ServerName), "."))
	if policy.ServerName == "" {
		policy.ServerName = targetHost
	}
	if targetHost == "" || policy.ServerName != targetHost {
		return policy, errors.New("TLS_POLICY_SERVER_NAME_MISMATCH")
	}
	policy.SPKISHA256 = strings.ToLower(strings.TrimSpace(policy.SPKISHA256))
	if policy.SPKISHA256 != "" {
		decoded, err := hex.DecodeString(policy.SPKISHA256)
		if err != nil || len(decoded) != sha256.Size {
			return policy, errors.New("TLS_POLICY_SPKI_INVALID")
		}
	}
	return policy, nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	var different byte
	for index := range left {
		different |= left[index] ^ right[index]
	}
	return different == 0
}
