package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type suspendGuardTest struct {
	invoke  bool
	failure error
	held    *bool
}

func (g suspendGuardTest) WithStoppedQuiescentWorkspace(_ context.Context, _ int64, fn func() error) error {
	if g.failure != nil {
		return g.failure
	}
	if !g.invoke {
		return nil
	}
	*g.held = true
	defer func() { *g.held = false }()
	return fn()
}

func TestInteractionSuspendUnavailableProofNeverCaptures(t *testing.T) {
	input := interactionSuspendInput{InteractionID: 5, Checkpoint: checkpoint.Input{TaskID: 1, TaskTurnID: 2, Generation: 1, AttemptID: 3, Decision: domain.PurposeInteractionSuspend, SessionDigest: "session", ExecutionSnapshotDigest: "snapshot"}}
	captured := false
	pack := func(context.Context, interactionSuspendInput, string) (checkpoint.SpoolFrozen, error) {
		captured = true
		return checkpoint.SpoolFrozen{}, nil
	}
	key := func(context.Context, string) (application.CheckpointKeyMaterialization, error) {
		t.Fatal("issued key without proof")
		return application.CheckpointKeyMaterialization{}, nil
	}
	upload := func(context.Context, string, string, checkpoint.SpoolBlob) error {
		t.Fatal("uploaded without proof")
		return nil
	}
	for _, guard := range []interactionSuspendGuard{nil, suspendGuardTest{}, suspendGuardTest{failure: errors.New("native still running")}} {
		if _, err := captureInteractionSuspend(context.Background(), guard, pack, key, upload, input, t.TempDir()); err == nil || captured {
			t.Fatalf("unproven capture err=%v called=%v", err, captured)
		}
	}
}

func TestInteractionSuspendEncryptedCaptureAndFailedReadback(t *testing.T) {
	for _, failReadback := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "readback mismatch"}[failReadback], func(t *testing.T) {
			spool := t.TempDir()
			held := false
			input := interactionSuspendInput{InteractionID: 5, Checkpoint: checkpoint.Input{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 1, Decision: domain.PurposeInteractionSuspend, SessionDigest: "lineage", ExecutionSnapshotDigest: "frozen"}}
			identity := domain.InteractionCheckpointIdentity(1, 2, 3, 1, 5, 1, 1, "frozen", sha256Hex([]byte("prepare")))
			makeBlob := func(name string, body []byte) checkpoint.SpoolBlob {
				path := filepath.Join(spool, name)
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
				return checkpoint.SpoolBlob{Path: path, SHA256: sha256Hex(body), SizeBytes: int64(len(body))}
			}
			frozen := checkpoint.SpoolFrozen{FormatVersion: 2, Identity: identity, RepositoryBundle: makeBlob("bundle", []byte("bundle bytes")), WorkspaceOverlay: makeBlob("overlay", []byte("overlay bytes")), Session: makeBlob("session", []byte("private session bytes"))}
			input.AdapterSessionGeneration, input.CurrentRound, input.SuspendControlKeySHA256 = 1, 1, sha256Hex([]byte("prepare"))
			input.ExecutionBudget = wcp.ExecutionBudget{Version: 1, TaskID: 1, AttemptID: 3, Generation: 1, OwnerEpoch: 1, ActiveLimitMillis: 600000, AbsoluteDeadlineUnixMillis: time.Now().Add(time.Hour).UnixMilli()}
			meta := func(b checkpoint.SpoolBlob) wcp.Blob { return wcp.Blob{SHA256: b.SHA256, SizeBytes: b.SizeBytes} }
			manifest, _ := json.Marshal(wcp.Manifest{SchemaVersion: 2, Format: "REPOSITORY_BUNDLE_OVERLAY", Purpose: domain.PurposeInteractionSuspend, CheckpointIdentity: identity, TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 1, InteractionID: 5, AdapterSessionGeneration: 1, SessionDigest: "lineage", SnapshotDigest: "frozen", BaseCommit: "base", ResultCommit: "result", ResultTree: "tree", SelectionDigest: "selection", CurrentRound: input.CurrentRound, SuspendControlKeySHA256: input.SuspendControlKeySHA256, ExecutionBudget: input.ExecutionBudget, RepositoryBundle: meta(frozen.RepositoryBundle), WorkspaceOverlay: meta(frozen.WorkspaceOverlay), Session: meta(frozen.Session)})
			frozen.Manifest = makeBlob("manifest", manifest)
			pack := func(context.Context, interactionSuspendInput, string) (checkpoint.SpoolFrozen, error) {
				if !held {
					t.Fatal("pack outside guard")
				}
				return frozen, nil
			}
			keyBytes := bytes.Repeat([]byte{7}, 32)
			key := func(context.Context, string) (application.CheckpointKeyMaterialization, error) {
				return application.CheckpointKeyMaterialization{Algorithm: "AES-256-GCM-CHUNKED", PlainKeyBase64: base64.StdEncoding.EncodeToString(keyBytes), AADIdentity: identity, Envelope: application.KeyEnvelope{KeyID: "test", Ciphertext: "wrapped", EDEK: "test"}}, nil
			}
			count := 0
			upload := func(_ context.Context, _ string, _ string, blob checkpoint.SpoolBlob) error {
				if !held {
					t.Fatal("readback outside guard")
				}
				count++
				body, err := os.ReadFile(blob.Path)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(body, []byte("private session bytes")) {
					t.Fatal("plaintext uploaded")
				}
				if sha256Hex(body) != blob.SHA256 || int64(len(body)) != blob.SizeBytes {
					t.Fatal("encrypted metadata mismatch")
				}
				kind := []string{"repository-bundle", "workspace-overlay", "session", "manifest"}[count-1]
				var plain bytes.Buffer
				if err = checkpoint.DecryptStream(bytes.NewReader(body), &plain, keyBytes, checkpointAAD(identity, kind)); err != nil {
					t.Fatal(err)
				}
				if kind == "session" && plain.String() != "private session bytes" {
					t.Fatal("session changed")
				}
				if failReadback {
					return errors.New("CONTINUATION_CHECKPOINT_READBACK_MISMATCH")
				}
				return nil
			}
			result, err := captureInteractionSuspend(context.Background(), suspendGuardTest{invoke: true, held: &held}, pack, key, upload, input, spool)
			if failReadback {
				if err == nil || result.Identity != "" || count != 1 {
					t.Fatalf("failure published capture %+v %v", result, err)
				}
			} else if err != nil || result.Purpose != domain.PurposeInteractionSuspend || count != 4 || result.Session.ContentSHA256 != frozen.Session.SHA256 {
				t.Fatalf("capture %+v %v", result, err)
			}
			if held {
				t.Fatal("guard leaked")
			}
		})
	}
}

func TestInteractionSuspendCannotLaunderThroughResultPacker(t *testing.T) {
	held := false
	input := interactionSuspendInput{InteractionID: 5, Checkpoint: checkpoint.Input{TaskID: 1, TaskTurnID: 2, AttemptID: 3, Generation: 1, EvaluationCycleID: 5, Decision: domain.PurposeInteractionSuspend, SessionDigest: "session", ExecutionSnapshotDigest: "snapshot"}}
	pack := func(ctx context.Context, in interactionSuspendInput, spool string) (checkpoint.SpoolFrozen, error) {
		if !held {
			t.Fatal("guard released before pack")
		}
		return checkpoint.FreezeToSpool(ctx, in.Checkpoint, spool)
	}
	key := func(context.Context, string) (application.CheckpointKeyMaterialization, error) {
		t.Fatal("unsupported pack issued key")
		return application.CheckpointKeyMaterialization{}, nil
	}
	upload := func(context.Context, string, string, checkpoint.SpoolBlob) error {
		t.Fatal("unsupported pack uploaded")
		return nil
	}
	if _, err := captureInteractionSuspend(context.Background(), suspendGuardTest{invoke: true, held: &held}, pack, key, upload, input, t.TempDir()); err == nil {
		t.Fatal("old result packer accepted suspend")
	}
	if held {
		t.Fatal("guard leaked")
	}
}
