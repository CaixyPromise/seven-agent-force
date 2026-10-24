package securehttp

import (
	"context"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPinnedClientUsesOperatorConfiguredGitCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	caPath := filepath.Join(t.TempDir(), "git-ca.pem")
	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err = os.WriteFile(caPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_SSL_CAINFO", caPath)
	t.Setenv("SSL_CERT_FILE", "")
	client, err := NewPinnedClient(nil, host, port, []net.IP{net.ParseIP(host)}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestParsePolicyRequiresExactServerNameAndValidSPKI(t *testing.T) {
	t.Parallel()
	valid := []byte(`{"serverName":"git.localhost","spkiSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`)
	policy, err := ParsePolicy(valid, "git.localhost")
	if err != nil || policy.ServerName != "git.localhost" {
		t.Fatalf("policy=%+v err=%v", policy, err)
	}
	for _, input := range []struct {
		raw, host, reason string
	}{
		{`{"serverName":"other.localhost"}`, "git.localhost", "TLS_POLICY_SERVER_NAME_MISMATCH"},
		{`{"spkiSha256":"abcd"}`, "git.localhost", "TLS_POLICY_SPKI_INVALID"},
		{`{"unknown":true}`, "git.localhost", "TLS_POLICY_INVALID"},
	} {
		_, err = ParsePolicy([]byte(input.raw), input.host)
		if err == nil || err.Error() != input.reason {
			t.Fatalf("raw=%s err=%v", input.raw, err)
		}
	}
}

func TestPinnedClientRejectsASecondDestination(t *testing.T) {
	t.Parallel()
	client, err := NewPinnedClient(nil, "example.test", 443, []net.IP{net.ParseIP("203.0.113.10")}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport := client.Transport.(*http.Transport)
	if _, err = transport.DialContext(context.Background(), "tcp", "other.example.test:443"); err == nil || err.Error() != "SECURE_HTTP_DIAL_TARGET_MISMATCH" {
		t.Fatalf("err=%v", err)
	}
}

func TestPinnedClientUsesHTTP11ForPinnedProviderCompatibility(t *testing.T) {
	t.Parallel()
	client, err := NewPinnedClient(nil, "example.test", 443, []net.IP{net.ParseIP("203.0.113.10")}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("pinned client transport type is unexpected")
	}
	if transport.ForceAttemptHTTP2 {
		t.Fatal("pinned client must not force HTTP/2 through a policy-pinned dialer")
	}
}
