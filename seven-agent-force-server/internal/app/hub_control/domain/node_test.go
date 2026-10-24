package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNodeEditMetadataEnforcesStableIdentityAndPermanentIssuerLock(t *testing.T) {
	now := time.Now().UTC()
	node := Node{NodeCode: "node-a", HubIssuer: "https://hub.example.com", ConnectionStatus: ConnectionError, IssuerLockedAt: &now}

	if err := node.EditMetadata(NodeMetadata{NodeCode: "node-b", HubIssuer: node.HubIssuer}, now); !errors.Is(err, ErrNodeCodeImmutable) {
		t.Fatalf("rename error=%v want ErrNodeCodeImmutable", err)
	}
	if err := node.EditMetadata(NodeMetadata{NodeCode: node.NodeCode, HubIssuer: "https://other.example.com"}, now); !errors.Is(err, ErrIssuerLocked) {
		t.Fatalf("issuer edit error=%v want ErrIssuerLocked", err)
	}
}

func TestNodeProvisionStateMachineGuardsReplayAndStaleResults(t *testing.T) {
	now := time.Now().UTC()
	node := Node{NodeCode: "node-a", ConnectionStatus: ConnectionActive, ConnectionVersion: "v1", ConnectionRequestHash: "hash-v1", OIDCClientID: "client-a", OIDCClientSecret: EncryptedSecret{Ciphertext: "cipher", EDEK: "edek", WrapKeyRef: "key"}}

	decision, err := node.StartProvision("v2", "hash-v2", true, now)
	if err != nil || !decision.NewVersion || !decision.NeedsManagedClient || !decision.RotateSecret || node.ConnectionStatus != ConnectionPending || node.IssuerLockedAt == nil {
		t.Fatalf("start decision=%+v node=%+v err=%v", decision, node, err)
	}
	if node.FailProvision("v1", "hash-v1", "old failure", "old-trace", now) {
		t.Fatal("stale failure mutated current Saga")
	}
	if err := node.CompleteProvision("v1", "hash-v1", now); !errors.Is(err, ErrStaleProvisionResult) {
		t.Fatalf("stale completion error=%v", err)
	}
	if err := node.CompleteProvision("v2", "hash-v2", now); err != nil || node.ConnectionStatus != ConnectionActive {
		t.Fatalf("completion node=%+v err=%v", node, err)
	}
	if node.FailProvision("v2", "hash-v2", "late failure", "trace", now) {
		t.Fatal("late same-version failure downgraded ACTIVE")
	}
}

func TestNodeCopyResetsManagedConnectionState(t *testing.T) {
	now := time.Now().UTC()
	source := Node{ID: 1, NodeCode: "node-a", Status: NodeStatusEnabled, HubIssuer: "https://hub.example.com", OIDCClientID: "client-a", OIDCClientSecret: EncryptedSecret{Ciphertext: "cipher", EDEK: "edek", WrapKeyRef: "key"}, ConnectionStatus: ConnectionActive, ConnectionVersion: "v1", ConnectionRequestHash: "hash", IssuerLockedAt: &now, LastConnectionError: "error", LastConnectionTraceID: "trace"}

	copy := source.Copy(2, "node-b", "Node B", source.ManagementBearer, now)
	if copy.NodeCode != "node-b" || copy.Status != NodeStatusDisabled || copy.ConnectionStatus != ConnectionPending || copy.OIDCClientID != "" || copy.OIDCClientSecret.Present() || copy.ConnectionVersion != "" || copy.ConnectionRequestHash != "" || copy.IssuerLockedAt != nil || copy.LastConnectionError != "" || copy.LastConnectionTraceID != "" {
		t.Fatalf("copy retained managed connection state: %+v", copy)
	}
}

func TestNodeOwnsStatusHealthAndBearerMutations(t *testing.T) {
	now := time.Now().UTC()
	node := Node{Status: NodeStatusEnabled}
	bearer := EncryptedSecret{Ciphertext: "cipher", EDEK: "edek", WrapKeyRef: "key"}

	node.SetStatus(NodeStatusDisabled, now)
	node.RecordHealthy(now)
	node.ReplaceManagementBearer(bearer, now)
	if node.Status != NodeStatusDisabled || node.LastHealthyAt == nil || !node.LastHealthyAt.Equal(now) || node.ManagementBearer != bearer || !node.UpdatedAt.Equal(now) {
		t.Fatalf("domain mutations were not applied: %+v", node)
	}
}
