package main

import (
	"context"
	continuationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	wcp "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"io"
)

// Downloads/decrypts all four private objects through the existing AEAD/hash
// verifier, then restores exact bundle/overlay/session identity. HTTP/claim
// wiring supplies the server-frozen purpose-aware manifest independently.
func (w *worker) restoreInteractionSuspend(ctx context.Context, r continuationdomain.RestoreContract, expected wcp.Manifest, key []byte, spool, repo, state string) (wcp.Restored, error) {
	if r.FormatVersion != 2 || r.CheckpointIdentity != expected.CheckpointIdentity || r.TaskExecutionSnapshotDigest != expected.SnapshotDigest || r.SessionLineageDigest != expected.SessionDigest {
		return wcp.Restored{}, continuationdomain.ErrInteractionSuspend
	}
	frozen := base.SpoolFrozen{FormatVersion: 2, Identity: r.CheckpointIdentity}
	var manifestBody []byte
	for _, item := range []struct {
		object, cipherSHA, plainSHA, kind string
		size, limit                       int64
		target                            *base.SpoolBlob
	}{
		{r.RepositoryBundleObjectKey, r.RepositoryBundleObjectSHA256, r.RepositoryBundleContentSHA256, "repository-bundle", r.RepositoryBundleSizeBytes, base.MaxArchiveBytes, &frozen.RepositoryBundle},
		{r.WorkspaceOverlayObjectKey, r.WorkspaceOverlayObjectSHA256, r.WorkspaceOverlayContentSHA256, "workspace-overlay", r.WorkspaceOverlaySizeBytes, base.MaxArchiveBytes, &frozen.WorkspaceOverlay},
		{r.SessionObjectKey, r.SessionObjectSHA256, r.SessionContentSHA256, "session", r.SessionSizeBytes, base.MaxSessionArchiveBytes, &frozen.Session},
		{r.ManifestObjectKey, r.ManifestObjectSHA256, r.ManifestContentSHA256, "manifest", r.ManifestSizeBytes, 1 << 20, &frozen.Manifest},
	} {
		f, err := w.downloadCheckpointPlain(ctx, item.object, item.cipherSHA, item.plainSHA, item.size, key, r.CheckpointIdentity, item.kind, spool, item.limit)
		if err != nil {
			return wcp.Restored{}, err
		}
		defer removeClosed(f)
		blob, err := wcp.FileBlob(f.Name(), item.limit)
		if err != nil {
			return wcp.Restored{}, err
		}
		*item.target = blob
		if item.kind == "manifest" {
			manifestBody, err = io.ReadAll(io.LimitReader(f, (1<<20)+1))
			if err != nil {
				return wcp.Restored{}, err
			}
		}
	}
	return wcp.RestoreFromSpool(ctx, manifestBody, expected, frozen, repo, state)
}
