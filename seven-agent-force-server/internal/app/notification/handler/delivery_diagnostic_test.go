package handler

import (
	"net"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/network"
)

// diagnosticTestConn supplies only the peer identity consumed by the
// transport policy. The embedded interface is never used by these unit tests.
type diagnosticTestConn struct {
	network.Conn
	remote net.Addr
}

func (c diagnosticTestConn) RemoteAddr() net.Addr {
	return c.remote
}

func TestDiagnosticTransportPolicyAcceptsOnlyTrustedTLSAssertion(t *testing.T) {
	policy := DiagnosticTransportPolicy{TrustedProxies: []string{"203.0.113.9"}}

	forged := diagnosticRequestContext("198.51.100.30:443", "https")
	if policy.Allows(forged) {
		t.Fatal("untrusted peer forged X-Forwarded-Proto and was accepted")
	}

	trustedWithoutTLS := diagnosticRequestContext("203.0.113.9:443", "http")
	if policy.Allows(trustedWithoutTLS) {
		t.Fatal("trusted proxy without HTTPS assertion was accepted")
	}

	trustedTLS := diagnosticRequestContext("203.0.113.9:443", "https")
	if !policy.Allows(trustedTLS) {
		t.Fatal("trusted proxy HTTPS assertion was rejected")
	}
	if policy.Allows(diagnosticRequestContext("203.0.113.9:443", "https, http")) {
		t.Fatal("trusted proxy accepted a forwarded protocol chain that may include client-supplied input")
	}

	strictLoopback := diagnosticRequestContext("127.0.0.1:443", "https")
	if policy.Allows(strictLoopback) {
		t.Fatal("loopback bypassed protected transport outside explicit local wiring")
	}
	localPolicy := DiagnosticTransportPolicy{AllowLoopbackInsecure: true}
	if !localPolicy.Allows(diagnosticRequestContext("127.0.0.1:8080", "")) {
		t.Fatal("explicit local development loopback exception was rejected")
	}
}

func TestDiagnosticTransportPolicyRejectsBroadOrPublicTrustedCIDRs(t *testing.T) {
	policy := DiagnosticTransportPolicy{TrustedCIDRs: []string{
		"0.0.0.0/0",
		"10.0.0.0/8",
		"203.0.113.0/24",
		"10.24.0.0/24",
	}}

	if policy.Allows(diagnosticRequestContext("198.51.100.30:443", "https")) {
		t.Fatal("catch-all or public trusted CIDR accepted an arbitrary peer")
	}
	if policy.Allows(diagnosticRequestContext("10.25.1.8:443", "https")) {
		t.Fatal("overly broad private trusted CIDR accepted an arbitrary private peer")
	}
	if !policy.Allows(diagnosticRequestContext("10.24.0.8:443", "https")) {
		t.Fatal("narrow private trusted CIDR rejected its configured proxy peer")
	}

	normalized := policy.normalized()
	if len(normalized.TrustedCIDRs) != 1 || normalized.TrustedCIDRs[0] != "10.24.0.0/24" {
		t.Fatalf("unsafe trusted CIDRs survived normalization: %#v", normalized.TrustedCIDRs)
	}
}

func TestDeliveryDiagnosticRequiresExplicitPermissionAndSetsNoStoreHeaders(t *testing.T) {
	reqCtx := app.NewContext(0)
	securitycontext.Set(reqCtx, &securitycontext.UserContext{
		UserID:  42,
		IsAdmin: true,
	})
	if hasExplicitDeliveryDiagnosticPermission(reqCtx, "system:notification:delivery:diagnostic") {
		t.Fatal("generic administrator flag bypassed explicit diagnostic grant")
	}

	securitycontext.Set(reqCtx, &securitycontext.UserContext{
		UserID: 42,
		Permissions: []string{
			"system:notification:delivery:diagnostic",
			"system:notification:delivery:content:*",
		},
	})
	if !hasExplicitDeliveryDiagnosticPermission(reqCtx, "system:notification:delivery:diagnostic") {
		t.Fatal("explicit general diagnostic capability was rejected")
	}
	if !hasExplicitDeliveryDiagnosticPermission(reqCtx, "system:notification:delivery:content:secret-ephemeral") {
		t.Fatal("explicit content wildcard was rejected")
	}

	setDiagnosticNoStoreHeaders(reqCtx)
	if got := string(reqCtx.Response.Header.Peek("Cache-Control")); got != "no-store, max-age=0" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}
	if got := string(reqCtx.Response.Header.Peek("Pragma")); got != "no-cache" {
		t.Fatalf("Pragma=%q, want no-cache", got)
	}
	if got := string(reqCtx.Response.Header.Peek("Referrer-Policy")); got != "no-referrer" {
		t.Fatalf("Referrer-Policy=%q, want no-referrer", got)
	}
}

func diagnosticRequestContext(remoteAddress, forwardedProto string) *app.RequestContext {
	host, port, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		panic(err)
	}
	portNumber, err := net.LookupPort("tcp", port)
	if err != nil {
		panic(err)
	}
	reqCtx := app.NewContext(0)
	reqCtx.SetConn(diagnosticTestConn{remote: &net.TCPAddr{IP: net.ParseIP(host), Port: portNumber}})
	if forwardedProto != "" {
		reqCtx.Request.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	return reqCtx
}
