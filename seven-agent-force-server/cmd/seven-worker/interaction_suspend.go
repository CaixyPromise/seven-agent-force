package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	continuationapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	workercheckpoint "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
)

// This port must be implemented by the trusted Worker runtime controller. It
// invokes capture only after proving the native process stopped AND workspace
// writers quiescent, and holds that exclusion until capture returns. Adapter
// messages, request timeouts and a successful stop request are not proof.
type interactionSuspendGuard interface {
	WithStoppedQuiescentWorkspace(context.Context, int64, func() error) error
}

type interactionSuspendInput struct {
	InteractionID            int64
	OperationID              int64
	AdapterSessionGeneration int64
	AutomaticRevisionCount   int
	ExecutionBudget          workercheckpoint.ExecutionBudget
	CurrentRound             int64
	SuspendControlKeySHA256  string
	Checkpoint               processorcheckpoint.Input
}

func freezeInteractionSuspendToSpool(ctx context.Context, input interactionSuspendInput, spool string) (processorcheckpoint.SpoolFrozen, error) {
	return workercheckpoint.FreezeToSpool(ctx, workercheckpoint.Input{Checkpoint: input.Checkpoint, InteractionID: input.InteractionID, OperationID: input.OperationID, AdapterSessionGeneration: input.AdapterSessionGeneration, AutomaticRevisionCount: input.AutomaticRevisionCount, ExecutionBudget: input.ExecutionBudget, CurrentRound: input.CurrentRound, SuspendControlKeySHA256: input.SuspendControlKeySHA256}, spool)
}

// Production callable path: reserve returns the exact key/identity/counters;
// this binds the real purpose-aware packer to the existing verified object store.
func (w *worker) captureReservedInteractionSuspend(ctx context.Context, guard interactionSuspendGuard, reservation continuationapp.InteractionReservationResult, input interactionSuspendInput, spool string) (interactionSuspendCapture, error) {
	if reservation.Disposition != "RESERVED" || reservation.Reservation.ID <= 0 {
		return interactionSuspendCapture{}, continuationdomain.ErrInteractionSuspend
	}
	input.AdapterSessionGeneration = reservation.Reservation.AdapterSessionGeneration
	input.AutomaticRevisionCount = reservation.Reservation.AutomaticRevisionCount
	input.CurrentRound = reservation.Reservation.CurrentRound
	input.SuspendControlKeySHA256 = reservation.Reservation.SuspendControlKeySHA256
	if input.Checkpoint.ExecutionSnapshotDigest != reservation.Reservation.SnapshotDigest || (reservation.Reservation.SessionDigest != "" && input.Checkpoint.SessionDigest != reservation.Reservation.SessionDigest) {
		return interactionSuspendCapture{}, continuationdomain.ErrInteractionSuspend
	}
	key := func(_ context.Context, identity string) (continuationapp.CheckpointKeyMaterialization, error) {
		if identity != reservation.Reservation.Identity {
			return continuationapp.CheckpointKeyMaterialization{}, continuationdomain.ErrInteractionSuspend
		}
		return reservation.Key, nil
	}
	return w.captureInteractionSuspend(ctx, guard, freezeInteractionSuspendToSpool, key, input, spool)
}

// Test seam; production uses freezeInteractionSuspendToSpool above. Never
// substitute PASS/WAIT_USER to make the result validator accept a suspension.
type interactionSuspendPacker func(context.Context, interactionSuspendInput, string) (processorcheckpoint.SpoolFrozen, error)
type interactionSuspendKey func(context.Context, string) (continuationapp.CheckpointKeyMaterialization, error)
type interactionSuspendUpload func(context.Context, string, string, processorcheckpoint.SpoolBlob) error

type interactionSuspendBlob struct {
	ObjectKey, ContentSHA256, ObjectSHA256 string
	SizeBytes                              int64
}

// Worker-private metadata only, not an operator response or ordinary artifact.
type interactionSuspendCapture struct {
	Purpose                            string
	InteractionID                      int64
	OperationID                        int64
	Identity                           string
	FormatVersion                      int
	Bundle, Overlay, Session, Manifest interactionSuspendBlob
	Envelope                           continuationapp.KeyEnvelope
}

func captureInteractionSuspend(ctx context.Context, guard interactionSuspendGuard, pack interactionSuspendPacker, issueKey interactionSuspendKey, upload interactionSuspendUpload, input interactionSuspendInput, spool string) (interactionSuspendCapture, error) {
	var result interactionSuspendCapture
	if guard == nil || pack == nil || issueKey == nil || upload == nil || (input.InteractionID > 0) == (input.OperationID > 0) || input.Checkpoint.TaskID <= 0 || input.Checkpoint.TaskTurnID <= 0 || input.Checkpoint.Generation <= 0 || input.Checkpoint.AttemptID <= 0 || input.Checkpoint.Decision != continuationdomain.PurposeInteractionSuspend || input.Checkpoint.SessionDigest == "" || input.Checkpoint.ExecutionSnapshotDigest == "" {
		return result, continuationdomain.ErrInteractionSuspend
	}
	called := false
	err := guard.WithStoppedQuiescentWorkspace(ctx, input.Checkpoint.AttemptID, func() error {
		if called {
			return continuationdomain.ErrInteractionSuspend
		}
		called = true
		if err := ctx.Err(); err != nil {
			return err
		}
		frozen, err := pack(ctx, input, spool)
		if err != nil {
			return err
		}
		expectedIdentity := continuationdomain.InteractionCheckpointIdentity(input.Checkpoint.TaskID, input.Checkpoint.TaskTurnID, input.Checkpoint.AttemptID, input.Checkpoint.Generation, input.InteractionID, input.AdapterSessionGeneration, input.CurrentRound, input.Checkpoint.ExecutionSnapshotDigest, input.SuspendControlKeySHA256)
		if input.OperationID > 0 {
			expectedIdentity = continuationdomain.OperationCheckpointIdentity(input.Checkpoint.TaskID, input.Checkpoint.TaskTurnID, input.Checkpoint.AttemptID, input.Checkpoint.Generation, input.OperationID, input.AdapterSessionGeneration, input.CurrentRound, input.Checkpoint.ExecutionSnapshotDigest, input.SuspendControlKeySHA256)
		}
		if frozen.FormatVersion != 2 || frozen.Identity != expectedIdentity {
			return continuationdomain.ErrInteractionSuspend
		}
		if err = verifyInteractionSuspendBlob(frozen.Manifest, 1<<20); err != nil {
			return err
		}
		body, err := os.ReadFile(frozen.Manifest.Path)
		if err != nil {
			return err
		}
		manifest, err := workercheckpoint.DecodeManifest(body)
		if err != nil || manifest.CheckpointIdentity != frozen.Identity || manifest.InteractionID != input.InteractionID || manifest.OperationID != input.OperationID || manifest.AttemptID != input.Checkpoint.AttemptID || manifest.SessionDigest != input.Checkpoint.SessionDigest || manifest.SnapshotDigest != input.Checkpoint.ExecutionSnapshotDigest || manifest.ExecutionBudget != input.ExecutionBudget || manifest.CurrentRound != input.CurrentRound || manifest.SuspendControlKeySHA256 != input.SuspendControlKeySHA256 || manifest.AdapterSessionGeneration != input.AdapterSessionGeneration || manifest.AutomaticRevisionCount != input.AutomaticRevisionCount || sha256Hex(body) != frozen.Manifest.SHA256 {
			return continuationdomain.ErrInteractionSuspend
		}
		for _, pair := range []struct {
			source processorcheckpoint.SpoolBlob
			meta   workercheckpoint.Blob
		}{{frozen.RepositoryBundle, manifest.RepositoryBundle}, {frozen.WorkspaceOverlay, manifest.WorkspaceOverlay}, {frozen.Session, manifest.Session}} {
			if pair.source.SHA256 != pair.meta.SHA256 || pair.source.SizeBytes != pair.meta.SizeBytes {
				return continuationdomain.ErrInteractionSuspend
			}
		}
		material, err := issueKey(ctx, frozen.Identity)
		if err != nil {
			return err
		}
		key, err := base64.StdEncoding.DecodeString(material.PlainKeyBase64)
		if err != nil || len(key) != 32 || material.Algorithm != "AES-256-GCM-CHUNKED" || material.AADIdentity != frozen.Identity || material.Envelope.KeyID == "" {
			return continuationdomain.ErrInteractionSuspend
		}
		defer clear(key)
		bundle, overlay, session, identity := processorcheckpoint.ObjectKeysV2(input.Checkpoint, frozen.Identity)
		result = interactionSuspendCapture{Purpose: continuationdomain.PurposeInteractionSuspend, InteractionID: input.InteractionID, OperationID: input.OperationID, Identity: frozen.Identity, FormatVersion: 2, Envelope: material.Envelope}
		for _, blob := range []struct {
			kind, key string
			source    processorcheckpoint.SpoolBlob
			target    *interactionSuspendBlob
		}{
			{"repository-bundle", bundle, frozen.RepositoryBundle, &result.Bundle},
			{"workspace-overlay", overlay, frozen.WorkspaceOverlay, &result.Overlay},
			{"session", session, frozen.Session, &result.Session},
			{"manifest", identity, frozen.Manifest, &result.Manifest},
		} {
			limit := int64(processorcheckpoint.MaxArchiveBytes)
			if blob.kind == "session" {
				limit = processorcheckpoint.MaxSessionArchiveBytes
			}
			if err := verifyInteractionSuspendBlob(blob.source, limit); err != nil {
				return err
			}
			encrypted, err := encryptCheckpointBlob(blob.source, filepath.Join(spool, blob.kind+".enc"), key, checkpointAAD(frozen.Identity, blob.kind))
			if err != nil {
				return err
			}
			// Production parent binds worker.putCheckpointFile: Put + bounded GET
			// hash/size verification. An upload-only callback is not this contract.
			if err = upload(ctx, blob.key, "application/octet-stream", encrypted); err != nil {
				return err
			}
			*blob.target = interactionSuspendBlob{ObjectKey: blob.key, ContentSHA256: blob.source.SHA256, ObjectSHA256: encrypted.SHA256, SizeBytes: encrypted.SizeBytes}
		}
		return nil
	})
	if err != nil {
		return interactionSuspendCapture{}, err
	}
	if !called {
		return interactionSuspendCapture{}, continuationdomain.ErrInteractionSuspend
	}
	return result, nil
}

func verifyInteractionSuspendBlob(blob processorcheckpoint.SpoolBlob, limit int64) error {
	info, err := os.Lstat(blob.Path)
	if err != nil || !info.Mode().IsRegular() || blob.SizeBytes <= 0 || blob.SizeBytes > limit || info.Size() != blob.SizeBytes {
		return continuationdomain.ErrInteractionSuspend
	}
	f, err := os.Open(blob.Path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, limit+1))
	if err != nil || n != blob.SizeBytes || hex.EncodeToString(h.Sum(nil)) != blob.SHA256 {
		return continuationdomain.ErrInteractionSuspend
	}
	return nil
}

// The concrete upload seam always includes the existing encrypted readback
// verifier. Parent still owns authority/key issuance, runtime guard and wiring.
func (w *worker) captureInteractionSuspend(ctx context.Context, guard interactionSuspendGuard, pack interactionSuspendPacker, key interactionSuspendKey, input interactionSuspendInput, spool string) (interactionSuspendCapture, error) {
	return captureInteractionSuspend(ctx, guard, pack, key, w.putCheckpointFile, input, spool)
}
