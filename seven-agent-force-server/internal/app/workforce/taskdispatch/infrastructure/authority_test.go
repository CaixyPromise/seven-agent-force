package infrastructure

import (
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

func TestAttemptAuthorityFencesOwnerAndExpiredLease(t *testing.T) {
	lease := "lease-value"
	ctx := AttemptContext{WorkerID: 1001, InstanceID: "wki", HostBootID: "boot", Generation: 3, OwnerEpoch: 2, Lease: hashToken(lease), LeaseExpiresAt: time.Now().Add(time.Minute)}
	envelope := domain.MachineEnvelope{WorkerID: 1001, InstanceID: "wki", HostBootID: "boot", Generation: 3, OwnerEpoch: 2}
	if !ctx.Authorizes(envelope, lease, time.Now()) {
		t.Fatal("exact current execution identity should authorize")
	}
	envelope.OwnerEpoch = 1
	if ctx.Authorizes(envelope, lease, time.Now()) {
		t.Fatal("stale owner epoch must be fenced")
	}
	envelope.OwnerEpoch = 2
	if ctx.Authorizes(envelope, lease, time.Now().Add(2*time.Minute)) {
		t.Fatal("expired execution lease must be fenced")
	}
}

func TestRunPermitSecretsAreStableAndDomainSeparated(t *testing.T) {
	lease := "lease-value-with-high-entropy-in-production"
	permitID := int64(123)
	permit := deriveRunSecret(lease, "permit", permitID)
	if replay := deriveRunSecret(lease, "permit", permitID); replay != permit {
		t.Fatalf("permit replay changed: %q != %q", replay, permit)
	}
	if nonce := deriveRunSecret(lease, "nonce", permitID); nonce == permit {
		t.Fatal("run permit token and nonce must be domain separated")
	}
	if other := deriveRunSecret(lease, "permit", 456); other == permit {
		t.Fatal("different permit identity must produce a different token")
	}
	if otherLease := deriveRunSecret("other-lease", "permit", permitID); otherLease == permit {
		t.Fatal("different execution lease must produce a different token")
	}
}
