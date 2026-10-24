package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNormalizeDescriptorCanonicalizesTargetsAndCwd(t *testing.T) {
	d, err := NormalizeDescriptor(Descriptor{Purpose: " install locked deps ", Command: "pnpm install --frozen-lockfile", Cwd: "packages/../web", Targets: []Target{{Host: "REGISTRY.NPMJS.ORG.", Protocol: "HTTPS", Port: 443}}})
	if err != nil {
		t.Fatal(err)
	}
	if d.Purpose != "install locked deps" || d.Cwd != "web" || len(d.Targets) != 1 || d.Targets[0].Host != "registry.npmjs.org" {
		t.Fatalf("unexpected normalized descriptor: %#v", d)
	}
}

func TestNetworkCommandDescriptorDigestMatchesRuntimeSDKFixture(t *testing.T) {
	d, err := NormalizeDescriptor(Descriptor{
		Purpose: "Install the project's npm dependencies with pnpm after ordinary container DNS access to the npm registry failed.",
		Command: "corepack pnpm install",
		Cwd:     ".",
		Targets: []Target{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Digest(d), "5ca9b25142863741b35e6b3ff85212799dd39338ec6751ca86c8651b4f4cf10b"; got != want {
		t.Fatalf("cross-runtime descriptor digest mismatch: got %s want %s", got, want)
	}
}

func TestNormalizeDescriptorRejectsDuplicateCanonicalTarget(t *testing.T) {
	_, err := NormalizeDescriptor(Descriptor{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "EXAMPLE.com.", Protocol: "HTTPS", Port: 443}, {Host: "example.com", Protocol: "https", Port: 443}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestNormalizeDescriptorRejectsSameHostOnMultiplePorts(t *testing.T) {
	_, err := NormalizeDescriptor(Descriptor{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "example.com", Protocol: "http", Port: 80}, {Host: "example.com", Protocol: "https", Port: 443}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ambiguous hostname rejection, got %v", err)
	}
}

func TestNormalizeDescriptorRejectsEscapesAndUnboundedTargets(t *testing.T) {
	cases := []Descriptor{
		{Purpose: "x", Command: "echo x", Cwd: "../outside", Targets: []Target{{Host: "example.com", Protocol: "https", Port: 443}}},
		{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "127.0.0.1", Protocol: "https", Port: 443}}},
		{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "*.example.com", Protocol: "https", Port: 443}}},
		{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "user@example.com", Protocol: "https", Port: 443}}},
		{Purpose: "x", Command: "echo x", Cwd: ".", Targets: []Target{{Host: "example.com/path", Protocol: "https", Port: 443}}},
	}
	for i, tc := range cases {
		if _, err := NormalizeDescriptor(tc); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}

func TestNormalizeDescriptorRejectsSecretsInConversationPurpose(t *testing.T) {
	for _, purpose := range []string{
		"download dependencies with api_key=super-secret-value",
		"use https://operator:password@example.com",
		"use ghp_abcdefghijklmnopqrstuvwxyz123456",
		strings.Repeat("A", 128),
	} {
		_, err := NormalizeDescriptor(Descriptor{Purpose: purpose, Command: "echo x", Cwd: ".", Targets: []Target{{Host: "example.com", Protocol: "https", Port: 443}}})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("sensitive purpose accepted: %q, err=%v", purpose, err)
		}
	}
}

func TestOperationStateSeparatesDecisionExecutionAndDelivery(t *testing.T) {
	now := time.Now().UTC()
	o := Operation{Status: StatusWaitingApproval, Version: 3, ExpiresAt: now.Add(time.Hour)}
	if err := o.ApplyDecision(3, DecisionAllowOnce, now); err != nil || o.Status != StatusReady {
		t.Fatalf("decision: %#v %v", o, err)
	}
	if err := o.Dispatch(now); err != nil || o.Status != StatusDispatched {
		t.Fatal(err)
	}
	if err := o.Start(now); err != nil || o.Status != StatusRunning {
		t.Fatal(err)
	}
	code := 0
	if err := o.Finish(StatusSucceeded, Digest("result"), "ok", &code, now); err != nil || o.ResultDeliveryStatus != DeliverySaved {
		t.Fatal(err)
	}
	if err := o.MarkDelivered(false, false, now); err != nil || o.ResultDeliveryStatus != DeliveryAdapter {
		t.Fatal(err)
	}
	if err := o.MarkDelivered(true, false, now); err != nil || o.ResultDeliveryStatus != DeliveryNativeResolved || o.AgentContinued {
		t.Fatal(err)
	}
	if err := o.MarkDelivered(true, true, now); err != nil || !o.AgentContinued {
		t.Fatal(err)
	}
}

func TestOperationRejectsStaleDecisionAndUnknownOutcomeIsTerminal(t *testing.T) {
	now := time.Now().UTC()
	o := Operation{Status: StatusWaitingApproval, Version: 2, ExpiresAt: now.Add(time.Hour)}
	if !errors.Is(o.ApplyDecision(1, DecisionAllowOnce, now), ErrStale) {
		t.Fatal("expected stale")
	}
	o.Status = StatusRunning
	if err := o.Finish(StatusOutcomeUnknown, Digest("unknown"), "result unavailable", nil, now); err != nil {
		t.Fatal(err)
	}
	if err := o.Dispatch(now); !errors.Is(err, ErrActionNotAllowed) {
		t.Fatalf("unexpected retry: %v", err)
	}
}

func TestResultDigestBindsOutputAndOutputIsBounded(t *testing.T) {
	code := 0
	if ResultDigest(StatusSucceeded, &code, "ok", "one") == ResultDigest(StatusSucceeded, &code, "ok", "two") {
		t.Fatal("output is not digest-bound")
	}
	if _, err := NormalizeOutput(string(make([]byte, MaxOutputBytes+1))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized output accepted: %v", err)
	}
}

func TestCloseAndDeliveryAcknowledgementsAreIdempotent(t *testing.T) {
	now := time.Now().UTC()
	o := Operation{Status: StatusRunning, Version: 1}
	if err := o.Close(true, now); err != nil || o.Status != StatusCancelled || o.ResultDeliveryStatus != DeliveryNativeResolved {
		t.Fatal(err)
	}
	version := o.Version
	if err := o.Close(true, now); err != nil || o.Version != version {
		t.Fatalf("close replay mutated: %#v %v", o, err)
	}
	if err := o.MarkDelivered(true, false, now); err != nil {
		t.Fatal(err)
	}
	version = o.Version
	if err := o.MarkDelivered(true, false, now); err != nil || o.Version != version {
		t.Fatalf("ack replay mutated: %#v %v", o, err)
	}
}

func TestGrantCoversExactSubsetAndRevocation(t *testing.T) {
	now := time.Now().UTC()
	g := Grant{Status: GrantActive, Version: 1, ExpiresAt: now.Add(time.Hour), Targets: []Target{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}}}
	if !g.Covers(g.Targets, now) || g.Covers([]Target{{Host: "registry.npmjs.org", Protocol: "https", Port: 80}}, now) {
		t.Fatal("grant subset mismatch")
	}
	if err := g.Revoke(1, now); err != nil || g.Status != GrantRevoked || g.Covers(g.Targets, now) {
		t.Fatalf("revoke: %#v %v", g, err)
	}
}
