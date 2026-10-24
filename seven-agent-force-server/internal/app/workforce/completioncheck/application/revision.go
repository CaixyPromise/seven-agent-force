package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	principal "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type RevisionSource struct {
	TenantID, TaskID, TaskTurnID, AttemptID, Version, BaselineResultID, ProcessorReleaseID                         int64
	Status, Instruction, CheckDigest, SourceCommit, SourceIdentityDigest, ExecutionSnapshotDigest, ProcessorDigest string
	Input                                                                                                          facade.Input
	References                                                                                                     []facade.Reference
}
type RevisionRepository interface {
	FindSafePointRevision(context.Context, pgx.Tx, int64, int64, int64, int64) (*facade.Revision, error)
	LoadRevisionSource(context.Context, pgx.Tx, int64, int64) (RevisionSource, error)
	ResolveRevisionReferences(context.Context, pgx.Tx, int64, int64, int64, []int64) ([]facade.Reference, error)
	FindRevisionRequest(context.Context, pgx.Tx, int64, int64, int64, string) (*facade.Revision, error)
	FindPendingRevision(context.Context, pgx.Tx, int64, int64) (*facade.Revision, error)
	GetRevision(context.Context, pgx.Tx, int64, int64, int64) (facade.Revision, error)
	NextRevisionNumber(context.Context, pgx.Tx, int64, int64) (int64, error)
	InsertRevision(context.Context, pgx.Tx, facade.Revision) error
	SaveRevisionState(context.Context, pgx.Tx, facade.Revision) error
}
type RevisionService struct {
	pool       *pgxpool.Pool
	ids        principal.IDGenerator
	repo       RevisionRepository
	principals principal.Resolver
	authority  facade.RevisionSafePointAuthority
	turns      continuation.CompletionRevisionTurnCreator
}

func NewRevisions(pool *pgxpool.Pool, ids principal.IDGenerator, repo RevisionRepository, principals principal.Resolver, authority facade.RevisionSafePointAuthority, turns continuation.CompletionRevisionTurnCreator) *RevisionService {
	return &RevisionService{pool: pool, ids: ids, repo: repo, principals: principals, authority: authority, turns: turns}
}

var _ facade.RevisionRequester = (*RevisionService)(nil)
var _ facade.PendingReader = (*RevisionService)(nil)
var _ facade.RevisionApplier = (*RevisionService)(nil)

func revisionError(e error) error {
	if e == nil {
		return nil
	}
	for _, safe := range []error{facade.ErrStale, facade.ErrIdempotencyConflict, facade.ErrActionNotAllowed, facade.ErrInvalid, facade.ErrNotFound, facade.ErrConflict} {
		if errors.Is(e, safe) {
			return safe
		}
	}
	if errors.Is(e, domain.ErrInvalid) {
		return facade.ErrInvalid
	}
	return facade.ErrRevisionUnavailable
}
func validRevisionKey(key string) bool {
	if key == "" || len(key) > 128 || !utf8.ValidString(key) || strings.TrimSpace(key) != key {
		return false
	}
	for _, r := range key {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func normalizeRevision(r facade.RevisionRequest) (facade.RevisionRequest, error) {
	if r.ExpectedVersion <= 0 || !validRevisionKey(r.IdempotencyKey) || r.CompletionCheck == nil {
		return r, facade.ErrInvalid
	}
	f, e := facade.Freeze(r.CompletionCheck)
	if e != nil {
		return r, facade.ErrInvalid
	}
	r.CompletionCheck = &f.Input
	return r, nil
}
func (s *RevisionService) RequestRevision(ctx context.Context, u facade.RevisionUser, r facade.RevisionRequest) (out facade.Revision, err error) {
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return out, facade.ErrInvalid
	}
	if !u.CanContinue {
		return out, facade.ErrActionNotAllowed
	}
	r, err = normalizeRevision(r)
	if err != nil {
		return
	}
	if s.pool == nil || s.ids == nil || s.repo == nil || s.principals == nil {
		return out, facade.ErrRevisionUnavailable
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return out, facade.ErrRevisionUnavailable
	}
	defer tx.Rollback(ctx)
	out, err = s.requestTx(ctx, tx, u, r)
	if err != nil {
		return facade.Revision{}, revisionError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.Revision{}, revisionError(err)
	}
	return
}
func (s *RevisionService) requestTx(ctx context.Context, tx pgx.Tx, u facade.RevisionUser, r facade.RevisionRequest) (facade.Revision, error) {
	source, e := s.repo.LoadRevisionSource(ctx, tx, u.TenantID, u.TaskID)
	if e != nil {
		return facade.Revision{}, e
	}
	raw, _ := json.Marshal(struct {
		UserID  int64
		Request facade.RevisionRequest
	}{u.UserID, r})
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	old, e := s.repo.FindRevisionRequest(ctx, tx, u.TenantID, u.TaskID, u.UserID, r.IdempotencyKey)
	if e != nil {
		return facade.Revision{}, e
	}
	if old != nil {
		if old.RequestDigest != digest {
			return facade.Revision{}, facade.ErrIdempotencyConflict
		}
		old.Replayed = true
		return *old, nil
	}
	if source.Version != r.ExpectedVersion {
		return facade.Revision{}, facade.ErrStale
	}
	if source.Status != "RUNNING" || source.AttemptID <= 0 {
		return facade.Revision{}, facade.ErrActionNotAllowed
	}
	old, e = s.repo.FindPendingRevision(ctx, tx, u.TenantID, u.TaskID)
	if e != nil {
		return facade.Revision{}, e
	}
	if old != nil {
		return facade.Revision{}, facade.ErrConflict
	}
	next, e := domain.AdditiveRevision(source.Input, *r.CompletionCheck)
	if e != nil {
		return facade.Revision{}, facade.ErrInvalid
	}
	refs, e := s.repo.ResolveRevisionReferences(ctx, tx, u.TenantID, u.TaskID, source.TaskTurnID, next.ReferenceAssetIDs)
	if e != nil {
		return facade.Revision{}, e
	}
	// Retaining an asset ID must also retain its frozen bytes and metadata.
	for _, oldRef := range source.References {
		found := false
		for _, ref := range refs {
			if ref.ReferenceAssetID == oldRef.ReferenceAssetID {
				if ref != oldRef {
					return facade.Revision{}, facade.ErrConflict
				}
				found = true
			}
		}
		if !found {
			return facade.Revision{}, facade.ErrConflict
		}
	}
	frozen, e := facade.Freeze(&next)
	if e != nil {
		return facade.Revision{}, e
	}
	checkDigest, e := facade.FrozenDigest(frozen.Input, refs)
	if e != nil {
		return facade.Revision{}, e
	}
	principalID, e := s.principals.EnsureUser(ctx, tx, s.ids, u.UserID)
	if e != nil {
		return facade.Revision{}, e
	}
	number, e := s.repo.NextRevisionNumber(ctx, tx, u.TenantID, u.TaskID)
	if e != nil {
		return facade.Revision{}, e
	}
	out := facade.Revision{ID: s.ids.NextID(), TenantID: u.TenantID, TaskID: u.TaskID, SourceTaskTurnID: source.TaskTurnID, SourceAttemptID: source.AttemptID, SourceVersion: source.Version, RevisionNumber: number, Version: 1, Status: domain.RevisionPending, CreatorUserID: u.UserID, PrincipalID: principalID, IdempotencyKey: r.IdempotencyKey, RequestDigest: digest, Base: source.Input, BaseDigest: source.CheckDigest, Input: frozen.Input, References: refs, InputDigest: frozen.Digest, CheckDigest: checkDigest, SourceBaselineResultID: source.BaselineResultID, SourceCommit: source.SourceCommit, SourceIdentityDigest: source.SourceIdentityDigest, ExecutionSnapshotDigest: source.ExecutionSnapshotDigest, ProcessorReleaseID: source.ProcessorReleaseID, ProcessorDigest: source.ProcessorDigest, CreatedAt: time.Now().UTC()}
	if e = s.repo.InsertRevision(ctx, tx, out); e != nil {
		return facade.Revision{}, e
	}
	if e = eventlog.Append(ctx, tx, out.TaskID, out.SourceAttemptID, "completion_check.revision.pending", map[string]any{"revisionId": out.ID, "version": "1"}); e != nil {
		return facade.Revision{}, e
	}
	return out, nil
}
func (s *RevisionService) PendingRevisionTx(ctx context.Context, tx pgx.Tx, tenant, task, turn int64) (*facade.Revision, error) {
	if tx == nil || s.repo == nil {
		return nil, facade.ErrRevisionUnavailable
	}
	if tenant <= 0 || task <= 0 || turn <= 0 {
		return nil, facade.ErrInvalid
	}
	source, e := s.repo.LoadRevisionSource(ctx, tx, tenant, task)
	if e != nil {
		return nil, revisionError(e)
	}
	if source.TaskTurnID != turn {
		return nil, facade.ErrStale
	}
	if source.Status != "RUNNING" && source.Status != "EXECUTION_COMPLETED" && source.Status != "VALIDATING" && source.Status != "REVIEWING" {
		return nil, facade.ErrActionNotAllowed
	}
	r, e := s.repo.FindPendingRevision(ctx, tx, tenant, task)
	if e != nil {
		return nil, revisionError(e)
	}
	if r != nil && (r.SourceTaskTurnID != turn || r.BaseDigest != source.CheckDigest) {
		return nil, facade.ErrStale
	}
	return r, nil
}
func (s *RevisionService) PendingForUser(ctx context.Context, u facade.RevisionUser) (out *facade.Revision, err error) {
	if u.TenantID <= 0 || u.TaskID <= 0 || u.UserID <= 0 {
		return nil, facade.ErrInvalid
	}
	if s.pool == nil || s.repo == nil {
		return nil, facade.ErrRevisionUnavailable
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, facade.ErrRevisionUnavailable
	}
	defer tx.Rollback(ctx)
	if _, e = s.repo.LoadRevisionSource(ctx, tx, u.TenantID, u.TaskID); e != nil {
		return nil, revisionError(e)
	}
	out, e = s.repo.FindPendingRevision(ctx, tx, u.TenantID, u.TaskID)
	if e != nil {
		return nil, revisionError(e)
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, revisionError(e)
	}
	return out, nil
}
func (s *RevisionService) ApplyRevisionTx(ctx context.Context, tx pgx.Tx, p facade.RevisionSafePoint) (out facade.Revision, err error) {
	if tx == nil || s.repo == nil || s.authority == nil || s.turns == nil {
		return out, facade.ErrRevisionUnavailable
	}
	if p.TenantID <= 0 || p.TaskID <= 0 || p.TaskTurnID <= 0 || p.AttemptID <= 0 || p.CheckpointID <= 0 || p.RevisionID <= 0 || p.ExpectedVersion <= 0 || p.Authority.WorkerID <= 0 || p.Authority.InstanceID == "" || p.Authority.HostBootID == "" || p.Authority.Generation <= 0 || p.Authority.OwnerEpoch <= 0 || p.ExecutionLease == "" {
		return out, facade.ErrInvalid
	}
	// The outer transaction must roll back on any error, including post-readback
	// failures. Authority locks the Task before revision/continuation rows.
	if e := s.authority.ValidateRevisionSafePoint(ctx, tx, p); e != nil {
		return out, revisionError(e)
	}
	out, err = s.applyTx(ctx, tx, p)
	if err != nil {
		return facade.Revision{}, revisionError(err)
	}
	return
}
func (s *RevisionService) applyTx(ctx context.Context, tx pgx.Tx, p facade.RevisionSafePoint) (facade.Revision, error) {
	source, e := s.repo.LoadRevisionSource(ctx, tx, p.TenantID, p.TaskID)
	if e != nil {
		return facade.Revision{}, e
	}
	r, e := s.repo.GetRevision(ctx, tx, p.TenantID, p.TaskID, p.RevisionID)
	if e != nil {
		return r, e
	}
	if r.Status == domain.RevisionApplied {
		if r.SourceTaskTurnID != p.TaskTurnID || r.SourceAttemptID != p.AttemptID || p.ExpectedVersion != r.Version-1 {
			return r, facade.ErrStale
		}
		r.Replayed = true
		return r, nil
	}
	if r.Status != domain.RevisionPending || r.Version != p.ExpectedVersion || source.TaskTurnID != p.TaskTurnID || r.SourceTaskTurnID != p.TaskTurnID || source.AttemptID != p.AttemptID || r.SourceAttemptID != p.AttemptID || r.BaseDigest != source.CheckDigest {
		return r, facade.ErrStale
	}
	if source.SourceCommit != r.SourceCommit || source.SourceIdentityDigest != r.SourceIdentityDigest || source.ExecutionSnapshotDigest != r.ExecutionSnapshotDigest || source.ProcessorReleaseID != r.ProcessorReleaseID || source.ProcessorDigest != r.ProcessorDigest {
		return r, facade.ErrConflict
	}
	if _, e = domain.AdditiveRevision(source.Input, r.Input); e != nil {
		return r, facade.ErrConflict
	}
	refs, e := s.repo.ResolveRevisionReferences(ctx, tx, p.TenantID, p.TaskID, p.TaskTurnID, r.Input.ReferenceAssetIDs)
	if e != nil {
		return r, e
	}
	digest, e := facade.FrozenDigest(r.Input, refs)
	if e != nil || digest != r.CheckDigest {
		return r, facade.ErrConflict
	}
	inputJSON, _ := json.Marshal(r.Input)
	refsJSON, _ := json.Marshal(r.References)
	created, e := s.turns.CreateCompletionRevisionTurnTx(ctx, tx, continuation.CompletionRevisionTurnRequest{TenantID: r.TenantID, TaskID: r.TaskID, SourceTaskTurnID: r.SourceTaskTurnID, SourceAttemptID: r.SourceAttemptID, CheckpointID: p.CheckpointID, RevisionID: r.ID, CreatorUserID: r.CreatorUserID, OriginalBaselineResultID: r.SourceBaselineResultID, SourceCommit: r.SourceCommit, SourceIdentityDigest: r.SourceIdentityDigest, ExecutionSnapshotDigest: r.ExecutionSnapshotDigest, InputJSON: inputJSON, ReferencesJSON: refsJSON, InputDigest: r.InputDigest, CheckDigest: r.CheckDigest})
	if e != nil {
		return r, e
	}
	if created.TaskTurnID <= 0 || created.TaskTurnID == source.TaskTurnID {
		return r, facade.ErrConflict
	}
	after, e := s.repo.LoadRevisionSource(ctx, tx, p.TenantID, p.TaskID)
	if e != nil {
		return r, e
	}
	if after.TaskTurnID != created.TaskTurnID || after.SourceIdentityDigest != source.SourceIdentityDigest || after.SourceCommit != source.SourceCommit || after.ExecutionSnapshotDigest != source.ExecutionSnapshotDigest || after.ProcessorReleaseID != source.ProcessorReleaseID || after.ProcessorDigest != source.ProcessorDigest || after.Instruction != source.Instruction || after.CheckDigest != r.CheckDigest || after.BaselineResultID != 0 {
		return r, facade.ErrConflict
	}
	state := domain.RevisionState{Status: r.Status, Version: r.Version}
	if e = state.Apply(p.ExpectedVersion, created.TaskTurnID, time.Now().UTC()); e != nil {
		return r, facade.ErrStale
	}
	r.Status, r.Version, r.AppliedTurnID, r.AppliedAt = state.Status, state.Version, state.AppliedTurnID, state.AppliedAt
	if e = s.repo.SaveRevisionState(ctx, tx, r); e != nil {
		return r, e
	}
	if e = eventlog.Append(ctx, tx, r.TaskID, r.SourceAttemptID, "completion_check.revision.applied", map[string]any{"revisionId": r.ID, "taskTurnId": r.AppliedTurnID, "version": "2"}); e != nil {
		return r, e
	}
	return r, nil
}
