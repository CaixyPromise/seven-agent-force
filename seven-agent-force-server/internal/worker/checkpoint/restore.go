package checkpoint

import (
	"context"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	base "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"os"
	"path/filepath"
)

type Restored struct {
	AdapterSessionGeneration int64
	AutomaticRevisionCount   int
	CheckpointIdentity       string
	ExecutionBudget          ExecutionBudget
}

// Machine-only saved answer, separate from the older native session snapshot.
type RestoreInput struct {
	Manifest            Manifest        `json:"manifest"`
	InteractionResponse json.RawMessage `json:"interactionResponse"`
	QuestionRequest     json.RawMessage `json:"questionRequest"`
	// Server-issued target-container ledger, separate from immutable source bytes.
	ExecutionBudget ExecutionBudget `json:"executionBudget"`
}

// RestoreFromSpool only accepts an exact server-frozen expected manifest and
// fresh Attempt-private destination directories. Caller decrypts into its
// private spool first with the normal checkpoint AEAD and purpose identity.
func RestoreFromSpool(ctx context.Context, body []byte, expected Manifest, files base.SpoolFrozen, repo, state string) (Restored, error) {
	m, err := DecodeManifest(body)
	if err != nil || m != expected {
		return Restored{}, domain.ErrInteractionSuspend
	}
	// The source container deadline can expire during a legitimate cold wait.
	// Server enforces checkpoint expiry and materializes the new container ledger.
	if m.ExecutionBudget.ActiveElapsedMillis >= m.ExecutionBudget.ActiveLimitMillis {
		return Restored{}, domain.ErrInteractionSuspend
	}
	for _, item := range []struct {
		file  base.SpoolBlob
		want  Blob
		limit int64
	}{{files.RepositoryBundle, m.RepositoryBundle, base.MaxArchiveBytes}, {files.WorkspaceOverlay, m.WorkspaceOverlay, base.MaxArchiveBytes}, {files.Session, m.Session, base.MaxSessionArchiveBytes}} {
		actual, e := FileBlob(item.file.Path, item.limit)
		if e != nil || blobMeta(actual) != item.want {
			return Restored{}, domain.ErrInteractionSuspend
		}
	}
	if _, err = os.Lstat(state); !os.IsNotExist(err) {
		return Restored{}, domain.ErrInteractionSuspend
	}
	for _, dir := range []string{repo} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			return Restored{}, err
		}
		info, e := os.Lstat(dir)
		if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return Restored{}, domain.ErrInteractionSuspend
		}
		entries, e := os.ReadDir(dir)
		if e != nil || len(entries) != 0 {
			return Restored{}, domain.ErrInteractionSuspend
		}
	}
	if _, err = Git(ctx, repo, "init", "-q"); err != nil {
		return Restored{}, err
	}
	ref := "refs/seven/checkpoints/" + m.CheckpointIdentity
	if _, err = Git(ctx, repo, "fetch", "-q", files.RepositoryBundle.Path, ref+":"+ref); err != nil {
		return Restored{}, err
	}
	if _, err = Git(ctx, repo, "checkout", "-q", "--detach", m.ResultCommit); err != nil {
		return Restored{}, err
	}
	commit, e := Git(ctx, repo, "rev-parse", "HEAD")
	if e != nil || commit != m.ResultCommit {
		return Restored{}, domain.ErrInteractionSuspend
	}
	tree, e := Git(ctx, repo, "rev-parse", "HEAD^{tree}")
	if e != nil || tree != m.ResultTree {
		return Restored{}, domain.ErrInteractionSuspend
	}
	for _, item := range []struct {
		path, destination, prefix string
		limit                     int64
		overlay                   bool
	}{{files.WorkspaceOverlay.Path, repo, "overlay", base.MaxArchiveBytes, true}, {files.Session.Path, state, "session", base.MaxSessionArchiveBytes, false}} {
		f, e := os.Open(filepath.Clean(item.path))
		if e != nil {
			return Restored{}, e
		}
		if item.overlay {
			e = base.ExtractOverlay(ctx, f, item.destination, item.prefix, item.limit)
		} else {
			e = base.Extract(ctx, f, item.destination, item.prefix, item.limit)
		}
		closeErr := f.Close()
		if e != nil {
			return Restored{}, e
		}
		if closeErr != nil {
			return Restored{}, closeErr
		}
	}
	return Restored{AdapterSessionGeneration: m.AdapterSessionGeneration, AutomaticRevisionCount: m.AutomaticRevisionCount, CheckpointIdentity: m.CheckpointIdentity, ExecutionBudget: m.ExecutionBudget}, nil
}
