package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
)

// RevisionStore has no pool or transaction owner. It contains SQL primitives;
// revision policy and transitions belong to application/domain.
type RevisionStore struct{}

func NewRevisionStore() *RevisionStore { return &RevisionStore{} }

func (s *RevisionStore) FindSafePointRevision(ctx context.Context, tx pgx.Tx, tenant, task, turn, attempt int64) (*facade.Revision, error) {
	r, e := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM workforce_completion_check_revisions WHERE tenant_id=$1 AND task_id=$2 AND source_task_turn_id=$3 AND source_attempt_id=$4 AND status IN ('PENDING','APPLIED') ORDER BY revision_number DESC LIMIT 1`, tenant, task, turn, attempt))
	if errors.Is(e, pgx.ErrNoRows) || errors.Is(e, facade.ErrNotFound) {
		return nil, nil
	}
	return &r, e
}

var _ application.RevisionRepository = (*RevisionStore)(nil)

func (s *RevisionStore) LoadRevisionSource(ctx context.Context, tx pgx.Tx, tenant, task int64) (application.RevisionSource, error) {
	var out application.RevisionSource
	var input, refs []byte
	e := tx.QueryRow(ctx, `SELECT t.tenant_id,t.id,tr.id,t.version,t.status,tr.instruction,COALESCE(tr.completion_check_json,'{"mode":"NONE"}'::jsonb),COALESCE(tr.completion_check_digest,''),COALESCE(tr.completion_check_references_json,'[]'::jsonb),COALESCE(tr.completion_check_baseline_result_id,0),tr.execution_snapshot_digest,tr.processor_release_id,tr.processor_digest,i.source_commit,i.source_identity_digest FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id AND tr.task_id=t.id AND tr.tenant_id=t.tenant_id JOIN workforce_task_input_snapshots i ON i.task_id=t.id AND i.task_turn_id=tr.id AND i.tenant_id=t.tenant_id WHERE t.tenant_id=$1 AND t.id=$2 FOR UPDATE OF t`, tenant, task).Scan(&out.TenantID, &out.TaskID, &out.TaskTurnID, &out.Version, &out.Status, &out.Instruction, &input, &out.CheckDigest, &refs, &out.BaselineResultID, &out.ExecutionSnapshotDigest, &out.ProcessorReleaseID, &out.ProcessorDigest, &out.SourceCommit, &out.SourceIdentityDigest)
	if errors.Is(e, pgx.ErrNoRows) {
		return out, facade.ErrNotFound
	}
	if e != nil {
		return out, e
	}
	if json.Unmarshal(input, &out.Input) != nil || json.Unmarshal(refs, &out.References) != nil {
		return out, facade.ErrConflict
	}
	// Task remains locked in this caller-owned transaction while resolving the
	// latest source Attempt; keep each SELECT within the three-table ceiling.
	if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT id FROM workforce_task_attempts WHERE task_id=$1 AND task_turn_id=$2 ORDER BY id DESC LIMIT 1),0)`, task, out.TaskTurnID).Scan(&out.AttemptID); e != nil {
		return out, e
	}
	digest, e := facade.FrozenDigest(out.Input, out.References)
	if e != nil {
		return out, facade.ErrConflict
	}
	if out.CheckDigest == "" {
		if out.Input.Mode != "NONE" || out.Input.Instructions != "" || len(out.Input.ReferenceAssetIDs) > 0 {
			return out, facade.ErrConflict
		}
		out.CheckDigest = digest
	} else if out.CheckDigest != digest {
		return out, facade.ErrConflict
	}
	return out, nil
}

// freezeSelectedReferences is shared with the existing ADD_VERIFICATION path.
// Only tenant-authorized references already frozen into the Task input are eligible.
func freezeSelectedReferences(ctx context.Context, tx pgx.Tx, tenant, task, turn int64, ids []int64) ([]facade.Reference, error) {
	out := make([]facade.Reference, 0, len(ids))
	for _, id := range ids {
		var ref facade.Reference
		e := tx.QueryRow(ctx, `SELECT a.id,a.display_name,a.content_sha256,a.object_sha256,a.size_bytes FROM workforce_reference_assets a JOIN workforce_task_input_references r ON r.reference_asset_id=a.id JOIN workforce_task_input_snapshots i ON i.id=r.task_input_snapshot_id WHERE i.tenant_id=$1 AND i.task_id=$2 AND i.task_turn_id=$3 AND a.id=$4 AND a.tenant_id=$1 AND a.content_sha256=r.reference_content_sha256`, tenant, task, turn, id).Scan(&ref.ReferenceAssetID, &ref.Name, &ref.ContentSHA256, &ref.ObjectSHA256, &ref.SizeBytes)
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, facade.ErrInvalid
		}
		if e != nil {
			return nil, e
		}
		out = append(out, ref)
	}
	return out, nil
}
func (s *RevisionStore) ResolveRevisionReferences(ctx context.Context, tx pgx.Tx, tenant, task, turn int64, ids []int64) ([]facade.Reference, error) {
	return freezeSelectedReferences(ctx, tx, tenant, task, turn, ids)
}

const revisionColumns = `id,tenant_id,task_id,source_task_turn_id,source_attempt_id,source_version,revision_number,version,status,creator_user_id,actor_principal_id,idempotency_key,request_digest,base_input_json,base_digest,input_json,input_digest,check_digest,references_json,COALESCE(source_baseline_result_id,0),source_commit,source_identity_digest,execution_snapshot_digest,processor_release_id,processor_digest,COALESCE(applied_turn_id,0),created_at,applied_at`

func scanRevision(row pgx.Row) (facade.Revision, error) {
	var r facade.Revision
	var base, input, refs []byte
	e := row.Scan(&r.ID, &r.TenantID, &r.TaskID, &r.SourceTaskTurnID, &r.SourceAttemptID, &r.SourceVersion, &r.RevisionNumber, &r.Version, &r.Status, &r.CreatorUserID, &r.PrincipalID, &r.IdempotencyKey, &r.RequestDigest, &base, &r.BaseDigest, &input, &r.InputDigest, &r.CheckDigest, &refs, &r.SourceBaselineResultID, &r.SourceCommit, &r.SourceIdentityDigest, &r.ExecutionSnapshotDigest, &r.ProcessorReleaseID, &r.ProcessorDigest, &r.AppliedTurnID, &r.CreatedAt, &r.AppliedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, facade.ErrNotFound
	}
	if e != nil {
		return r, e
	}
	if json.Unmarshal(base, &r.Base) != nil || json.Unmarshal(input, &r.Input) != nil || json.Unmarshal(refs, &r.References) != nil {
		return r, facade.ErrConflict
	}
	f, e := facade.Freeze(&r.Input)
	if e != nil || f.Digest != r.InputDigest {
		return r, facade.ErrConflict
	}
	digest, e := facade.FrozenDigest(r.Input, r.References)
	if e != nil || digest != r.CheckDigest {
		return r, facade.ErrConflict
	}
	return r, nil
}
func (s *RevisionStore) FindRevisionRequest(ctx context.Context, tx pgx.Tx, tenant, task, user int64, key string) (*facade.Revision, error) {
	r, e := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM workforce_completion_check_revisions WHERE tenant_id=$1 AND task_id=$2 AND creator_user_id=$3 AND idempotency_key=$4`, tenant, task, user, key))
	if errors.Is(e, facade.ErrNotFound) {
		return nil, nil
	}
	return &r, e
}
func (s *RevisionStore) FindPendingRevision(ctx context.Context, tx pgx.Tx, tenant, task int64) (*facade.Revision, error) {
	r, e := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM workforce_completion_check_revisions WHERE tenant_id=$1 AND task_id=$2 AND status='PENDING'`, tenant, task))
	if errors.Is(e, facade.ErrNotFound) {
		return nil, nil
	}
	return &r, e
}
func (s *RevisionStore) GetRevision(ctx context.Context, tx pgx.Tx, tenant, task, id int64) (facade.Revision, error) {
	return scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM workforce_completion_check_revisions WHERE tenant_id=$1 AND task_id=$2 AND id=$3 FOR UPDATE`, tenant, task, id))
}
func (s *RevisionStore) NextRevisionNumber(ctx context.Context, tx pgx.Tx, tenant, task int64) (int64, error) {
	var n int64
	e := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number),0)+1 FROM workforce_completion_check_revisions WHERE tenant_id=$1 AND task_id=$2`, tenant, task).Scan(&n)
	return n, e
}
func revisionNullable(n int64) any {
	if n <= 0 {
		return nil
	}
	return n
}
func (s *RevisionStore) InsertRevision(ctx context.Context, tx pgx.Tx, r facade.Revision) error {
	base, e := json.Marshal(r.Base)
	if e != nil {
		return e
	}
	input, e := json.Marshal(r.Input)
	if e != nil {
		return e
	}
	refs, e := json.Marshal(r.References)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO workforce_completion_check_revisions(id,tenant_id,task_id,source_task_turn_id,source_attempt_id,source_version,revision_number,version,status,creator_user_id,actor_principal_id,idempotency_key,request_digest,base_input_json,base_digest,input_json,input_digest,check_digest,references_json,source_baseline_result_id,source_commit,source_identity_digest,execution_snapshot_digest,processor_release_id,processor_digest,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)`, r.ID, r.TenantID, r.TaskID, r.SourceTaskTurnID, r.SourceAttemptID, r.SourceVersion, r.RevisionNumber, r.Version, r.Status, r.CreatorUserID, r.PrincipalID, r.IdempotencyKey, r.RequestDigest, base, r.BaseDigest, input, r.InputDigest, r.CheckDigest, refs, revisionNullable(r.SourceBaselineResultID), r.SourceCommit, r.SourceIdentityDigest, r.ExecutionSnapshotDigest, r.ProcessorReleaseID, r.ProcessorDigest, r.CreatedAt)
	return e
}
func (s *RevisionStore) SaveRevisionState(ctx context.Context, tx pgx.Tx, r facade.Revision) error {
	tag, e := tx.Exec(ctx, `UPDATE workforce_completion_check_revisions SET status=$4,version=$5,applied_turn_id=$6,applied_at=$7 WHERE tenant_id=$1 AND task_id=$2 AND id=$3 AND version=$5-1`, r.TenantID, r.TaskID, r.ID, r.Status, r.Version, revisionNullable(r.AppliedTurnID), r.AppliedAt)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return facade.ErrStale
	}
	return nil
}
