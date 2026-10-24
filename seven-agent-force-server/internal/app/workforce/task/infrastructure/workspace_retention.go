package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

// WorkspaceGCEligibility derives the only deletion decision the Worker may
// use for its local Task directory. PostgreSQL, encrypted checkpoints and
// artifact/object storage remain authoritative; an unavailable or unknown
// record is intentionally returned as ineligible (or as an error) so the
// Worker fails closed.
func (s *Store) WorkspaceGCEligibility(ctx context.Context, taskID int64) (taskfacade.WorkspaceGCEligibility, error) {
	eligibility := taskfacade.WorkspaceGCEligibility{TaskID: taskID, ReasonCode: "TASK_NOT_FOUND"}
	if s == nil || s.db == nil {
		return eligibility, errors.New("WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE")
	}
	if taskID < 1 {
		return eligibility, errors.New("WORKSPACE_RETENTION_TASK_ID_INVALID")
	}

	var status string
	var finishedAt *time.Time
	err := s.db.QueryRow(ctx, `SELECT status,finished_at FROM workforce_tasks WHERE id=$1`, taskID).Scan(&status, &finishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return eligibility, nil
	}
	if err != nil {
		return eligibility, fmt.Errorf("load workspace retention task: %w", err)
	}

	switch status {
	case "SUCCEEDED", "FAILED", "CANCELLED", "TIMED_OUT":
		eligibility.Terminal = true
	case "MANUAL_REVIEW":
		eligibility.ReasonCode = "TASK_MANUAL_REVIEW_RETAINED"
		return eligibility, nil
	default:
		eligibility.ReasonCode = "TASK_NOT_TERMINAL"
		return eligibility, nil
	}
	if finishedAt == nil || finishedAt.IsZero() {
		eligibility.Terminal = false
		eligibility.ReasonCode = "TASK_TERMINAL_AT_MISSING"
		return eligibility, nil
	}
	eligibility.TerminalAt = finishedAt.UTC()

	for _, reference := range workspaceRetentionReferences {
		var active bool
		if err := s.db.QueryRow(ctx, reference.query, taskID).Scan(&active); err != nil {
			return eligibility, fmt.Errorf("load workspace retention %s: %w", reference.name, err)
		}
		if active {
			eligibility.ActiveReferences = true
			eligibility.ReasonCode = "ACTIVE_" + reference.name
			return eligibility, nil
		}
	}
	if s.reviewRetention != nil {
		decision, err := s.reviewRetention.ReviewRunRetention(ctx, taskID)
		if err != nil {
			return eligibility, fmt.Errorf("load workspace retention ReviewRun: %w", err)
		}
		if decision.ActiveReferences {
			eligibility.ActiveReferences = true
			eligibility.ReasonCode = decision.ReasonCode
			return eligibility, nil
		}
		if !decision.PortableStateAvailable {
			eligibility.ReasonCode = decision.ReasonCode
			return eligibility, nil
		}
	}

	for _, portability := range workspacePortableStateQueries {
		var portable bool
		if err := s.db.QueryRow(ctx, portability.query, taskID).Scan(&portable); err != nil {
			return eligibility, fmt.Errorf("load workspace retention portability %s: %w", portability.name, err)
		}
		if !portable {
			eligibility.ReasonCode = "PORTABLE_" + portability.name + "_UNAVAILABLE"
			return eligibility, nil
		}
	}
	eligibility.PortableStateAvailable = true
	eligibility.ReasonCode = "RETENTION_ELIGIBLE"
	return eligibility, nil
}

type workspaceRetentionReference struct {
	name  string
	query string
}

// These predicates intentionally admit only known completed states. Any new
// status introduced by a later P1 slice is active by default until that slice
// explicitly updates this Server authority. ReviewRun checks are provided by
// its stable facade above so Task never reads ReviewRun's private Store.
var workspaceRetentionReferences = []workspaceRetentionReference{
	{name: "TASK_TURN", query: `SELECT EXISTS (SELECT 1 FROM workforce_task_turns WHERE task_id=$1 AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT','MANUAL_REVIEW'))`},
	{name: "ASSIGNMENT", query: `SELECT EXISTS (SELECT 1 FROM workforce_task_assignments WHERE task_id=$1 AND completed_at IS NULL)`},
	{name: "ATTEMPT", query: `SELECT EXISTS (SELECT 1 FROM workforce_task_attempts WHERE task_id=$1 AND finished_at IS NULL)`},
	{name: "FINALIZATION", query: `SELECT EXISTS (SELECT 1 FROM workforce_finalization_runs WHERE task_id=$1 AND status NOT IN ('FROZEN','FAILED','CANCELLED'))`},
	{name: "ARTIFACT_UPLOAD", query: `SELECT EXISTS (SELECT 1 FROM workforce_artifact_upload_attempts WHERE task_id=$1 AND status NOT IN ('VERIFIED','FAILED','CANCELLED'))`},
	{name: "CONTINUATION_RESTORE", query: `SELECT EXISTS (SELECT 1 FROM workforce_continuation_restore_runs WHERE task_id=$1 AND status NOT IN ('RESTORED','FAILED','CANCELLED'))`},
	{name: "MATERIALIZATION", query: `SELECT EXISTS (SELECT 1 FROM workforce_result_materialization_runs WHERE task_id=$1 AND status NOT IN ('FROZEN','FAILED','CANCELLED'))`},
	{name: "DELIVERY", query: `SELECT EXISTS (SELECT 1 FROM workforce_delivery_requests WHERE task_id=$1 AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED'))`},
}

type workspacePortableStateQuery struct {
	name  string
	query string
}

// A frozen local result may be collected only after every durable artifact it
// represents is verified. Keep each query below the repository's DG2
// three-table read-width limit; the conjunction is evaluated in Go and still
// fails closed on any query error. Checkpoints that remain available must
// carry their encrypted, immutable object identities. Expired checkpoints are
// not runnable and therefore do not require a host directory to be retained.
var workspacePortableStateQueries = []workspacePortableStateQuery{
	{name: "FINALIZATION_ARTIFACT", query: `
SELECT NOT EXISTS (
	SELECT 1
	FROM workforce_finalization_runs f
	WHERE f.task_id=$1 AND f.status='FROZEN'
	  AND NOT EXISTS (
		SELECT 1 FROM workforce_artifact_upload_attempts u
		WHERE u.finalization_run_id=f.id AND u.status='VERIFIED'
	  )
)`},
	{name: "MATERIALIZATION_ARTIFACT", query: `
SELECT NOT EXISTS (
	SELECT 1
	FROM workforce_result_materialization_runs m
	WHERE m.task_id=$1 AND m.status='FROZEN'
	  AND NOT EXISTS (
		SELECT 1 FROM workforce_artifact_upload_attempts u
		WHERE u.materialization_run_id=m.id AND u.status='VERIFIED'
	  )
)`},
	{name: "CHECKPOINT", query: `
SELECT NOT EXISTS (
	SELECT 1
	FROM workforce_continuation_checkpoints c
	WHERE c.task_id=$1
	  AND (
		c.status NOT IN ('AVAILABLE','EXPIRED')
		OR (
			c.status='AVAILABLE' AND (
				c.format_version<>2
				OR c.repository_bundle_object_key=''
				OR c.repository_bundle_content_sha256=''
				OR c.repository_bundle_object_sha256=''
				OR c.workspace_overlay_object_key=''
				OR c.workspace_overlay_content_sha256=''
				OR c.workspace_overlay_object_sha256=''
				OR c.session_object_key=''
				OR c.identity_manifest_object_key=''
				OR c.manifest_sha256=''
			)
		)
	  )
)`},
}
