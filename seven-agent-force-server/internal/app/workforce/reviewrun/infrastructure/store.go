// Package infrastructure persists the independent ReviewRun control plane.
// It intentionally contains no Docker, Provider, or report-verdict path: the
// only mutable runtime authority in this Slice is assignment/lease fencing.
package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

type IDGenerator interface{ NextID() int64 }

// ProviderInvocationCanceller is deliberately a one-way runtime seam. The
// ReviewRun store can fence a Run transactionally, then ask the Provider
// invocation service to stop any in-process stream owned by the just-fenced
// ReviewRun Attempt. It does not expose Provider credentials or persistence to
// this module.
type ProviderInvocationCanceller interface {
	CancelReviewRunInvocations(context.Context, int64, int64, int64, string) error
}

type Store struct {
	db                   *pgxpool.Pool
	idGen                IDGenerator
	now                  func() time.Time
	notificationRecorder tasknotification.TransactionalRecorder
	revisionTurnCreator  continuationfacade.ReviewRevisionTurnCreator
	invocationCanceller  ProviderInvocationCanceller
	assignmentResolver   routingfacade.ReviewAssignmentResolver
}

func New(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen, now: time.Now}
}

func NewWithClock(db *pgxpool.Pool, now func() time.Time, idGen IDGenerator) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{db: db, idGen: idGen, now: now}
}

func (s *Store) SetTaskNotificationRecorder(recorder tasknotification.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}

func (s *Store) SetReviewRevisionTurnCreator(creator continuationfacade.ReviewRevisionTurnCreator) {
	if s != nil {
		s.revisionTurnCreator = creator
	}
}

func (s *Store) SetProviderInvocationCanceller(canceller ProviderInvocationCanceller) {
	if s != nil {
		s.invocationCanceller = canceller
	}
}

func (s *Store) SetAssignmentResolver(resolver routingfacade.ReviewAssignmentResolver) {
	if s != nil {
		s.assignmentResolver = resolver
	}
}

// GateVerifiedFinalization is invoked only after every artifact upload was
// independently read back. A review-disabled Task continues without a
// ReviewRun. A review-enabled Task atomically becomes REVIEWING and gets
// exactly one immutable ReviewRun for this finalization occurrence.
func (s *Store) GateVerifiedFinalization(ctx context.Context, tx pgx.Tx, input facade.FinalizationInput) (facade.FinalizationDecision, error) {
	if s == nil || s.idGen == nil || !validFinalizationInput(input) {
		return facade.FinalizationDecision{}, domain.ErrInvalidReviewInput
	}
	var tenantID int64
	var reviewEnabled bool
	var completionCheckDigest string
	err := tx.QueryRow(ctx, `SELECT t.tenant_id,t.review_enabled,COALESCE(tr.completion_check_digest,'')
FROM workforce_tasks t
JOIN workforce_task_turns tr ON tr.id=$2 AND tr.task_id=t.id AND tr.tenant_id=t.tenant_id
WHERE t.id=$1 FOR UPDATE OF t,tr`, input.TaskID, input.TaskTurnID).Scan(&tenantID, &reviewEnabled, &completionCheckDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.FinalizationDecision{}, domain.ErrNotFound
	}
	if err != nil {
		return facade.FinalizationDecision{}, fmt.Errorf("load review execution snapshot: %w", err)
	}
	if tenantID != input.TenantID {
		return facade.FinalizationDecision{}, domain.ErrIdentityConflict
	}
	if !reviewEnabled {
		return facade.FinalizationDecision{ReviewRequired: false}, nil
	}
	// The review occurrence belongs to this frozen Turn. Later Task inputs
	// cannot retroactively change its completion-check criteria or identity.
	identity, err := runIdentity(input, completionCheckDigest)
	if err != nil {
		return facade.FinalizationDecision{}, err
	}
	// Persist the normalized capability sets from the immutable identity. This
	// keeps assignment selection stable when equivalent ProjectVersions use
	// a different ordering or contain duplicate capability declarations.
	identityDigest := digestIdentity(identity)
	runID := s.idGen.NextID()
	err = tx.QueryRow(ctx, `INSERT INTO workforce_review_runs (
 id,tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,
 result_commit,result_tree,hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,
 identity_digest,completion_check_digest,status
) VALUES (
 $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'PENDING'
) ON CONFLICT (tenant_id,task_id,task_turn_id,finalization_run_id) DO NOTHING
RETURNING id`,
		runID, identity.TenantID, identity.TaskID, identity.TaskTurnID, identity.AttemptID, identity.FinalizationRunID,
		identity.ResultCommit, identity.ResultTree, identity.HardTimeoutSeconds,
		identity.TransientRetryLimit, identity.AutomaticRevisionLimit, identityDigest, identity.CompletionCheckDigest).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		var existing domain.RunIdentity
		var existingID int64
		err = tx.QueryRow(ctx, `SELECT id,tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,
 result_commit,result_tree,hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,
 completion_check_digest
FROM workforce_review_runs
WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND finalization_run_id=$4 FOR UPDATE`,
			input.TenantID, input.TaskID, input.TaskTurnID, input.FinalizationRunID).Scan(
			&existingID, &existing.TenantID, &existing.TaskID, &existing.TaskTurnID, &existing.AttemptID, &existing.FinalizationRunID,
			&existing.ResultCommit, &existing.ResultTree, &existing.HardTimeoutSeconds,
			&existing.TransientRetryLimit, &existing.AutomaticRevisionLimit, &existing.CompletionCheckDigest)
		if err != nil {
			return facade.FinalizationDecision{}, fmt.Errorf("load existing ReviewRun: %w", err)
		}
		if !sameIdentity(existing, identity) {
			return facade.FinalizationDecision{}, domain.ErrIdentityConflict
		}
		runID = existingID
	} else if err != nil {
		return facade.FinalizationDecision{}, fmt.Errorf("create ReviewRun: %w", err)
	}

	// These values are changed only inside this same trusted transaction. The
	// insert above makes the operation replay-safe; the helpers accept its
	// already-gated state when an exact replay reaches this point.
	if err = updateReviewingState(ctx, tx, input, runID); err != nil {
		return facade.FinalizationDecision{}, err
	}
	return facade.FinalizationDecision{ReviewRequired: true, ReviewRunID: runID, Status: domain.StatusPending}, nil
}

func updateReviewingState(ctx context.Context, tx pgx.Tx, input facade.FinalizationInput, runID int64) error {
	if _, err := tx.Exec(ctx, `UPDATE workforce_artifacts SET status='PENDING_REVIEW'
WHERE task_id=$1 AND attempt_id=$2 AND status='AVAILABLE'`, input.TaskID, input.AttemptID); err != nil {
		return fmt.Errorf("mark artifacts pending independent review: %w", err)
	}
	if err := transitionToReviewing(ctx, tx, `UPDATE workforce_task_attempts SET status='REVIEWING',result_commit=$2,result_tree=$3,finished_at=NULL,exit_code=0
WHERE id=$1 AND status IN ('EXECUTION_COMPLETED','FINALIZING','REVIEWING')`, input.AttemptID, input.ResultCommit, input.ResultTree); err != nil {
		return fmt.Errorf("gate task attempt for independent review: %w", err)
	}
	if err := transitionToReviewing(ctx, tx, `UPDATE workforce_execution_nodes SET status='REVIEWING',updated_at=CURRENT_TIMESTAMP
WHERE id=(SELECT execution_node_id FROM workforce_finalization_runs WHERE id=$1) AND status IN ('UPLOADING','REVIEWING')`, input.FinalizationRunID); err != nil {
		return fmt.Errorf("gate execution node for independent review: %w", err)
	}
	if err := transitionToReviewing(ctx, tx, `UPDATE workforce_task_turns SET status='REVIEWING',result_commit=$2,result_tree=$3,result_summary='Awaiting independent review',finished_at=NULL
WHERE id=$1 AND status IN ('ACTIVE','REVIEWING')`, input.TaskTurnID, input.ResultCommit, input.ResultTree); err != nil {
		return fmt.Errorf("gate task turn for independent review: %w", err)
	}
	if err := transitionToReviewing(ctx, tx, `UPDATE workforce_tasks SET status='REVIEWING',result_summary='Awaiting independent review',verified_at=CURRENT_TIMESTAMP,finished_at=NULL,
version=CASE WHEN status='UPLOADING' THEN version+1 ELSE version END
WHERE id=$1 AND current_task_turn_id=$2 AND status IN ('UPLOADING','REVIEWING')`, input.TaskID, input.TaskTurnID); err != nil {
		return fmt.Errorf("gate task for independent review: %w", err)
	}
	_ = runID // retained to make the review gate's authority obvious at call sites.
	return nil
}

// transitionToReviewing allows an exact finalizer replay to observe the
// already-gated REVIEWING state, but still fails closed for every unrelated
// terminal/manual state. The ReviewRun identity was locked and verified before
// this point, so REVIEWING can only be the same occurrence's prior commit.
func transitionToReviewing(ctx context.Context, tx pgx.Tx, statement string, args ...any) error {
	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return domain.ErrIdentityConflict
}

func validFinalizationInput(input facade.FinalizationInput) bool {
	return input.TenantID > 0 && input.TaskID > 0 && input.TaskTurnID > 0 && input.AttemptID > 0 && input.FinalizationRunID > 0 && strings.TrimSpace(input.ResultCommit) != "" && strings.TrimSpace(input.ResultTree) != ""
}

func runIdentity(input facade.FinalizationInput, completionCheckDigest string) (domain.RunIdentity, error) {
	if !domain.ValidCompletionCheckDigest(completionCheckDigest) {
		return domain.RunIdentity{}, domain.ErrInvalidReviewInput
	}
	identity := domain.RunIdentity{
		CompletionCheckDigest: completionCheckDigest,
		TenantID:              input.TenantID, TaskID: input.TaskID, TaskTurnID: input.TaskTurnID, AttemptID: input.AttemptID, FinalizationRunID: input.FinalizationRunID,
		ResultCommit: strings.TrimSpace(input.ResultCommit), ResultTree: strings.TrimSpace(input.ResultTree),
		HardTimeoutSeconds:  reviewcontract.HardTimeoutSeconds,
		TransientRetryLimit: reviewcontract.TransientRetryLimit, AutomaticRevisionLimit: reviewcontract.AutomaticRevisionLimit,
	}
	if identity.HardTimeoutSeconds != reviewcontract.HardTimeoutSeconds || identity.TransientRetryLimit != reviewcontract.TransientRetryLimit || identity.AutomaticRevisionLimit != reviewcontract.AutomaticRevisionLimit {
		return domain.RunIdentity{}, domain.ErrInvalidReviewInput
	}
	return identity, nil
}

func normalizeRequiredCapabilities(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || len(value) > 128 || len(result) >= 64 {
			return nil, domain.ErrInvalidReviewInput
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, domain.ErrInvalidReviewInput
	}
	slices.Sort(result)
	return result, nil
}

func sameIdentity(left, right domain.RunIdentity) bool {
	if left.CompletionCheckDigest != right.CompletionCheckDigest {
		return false
	}
	return left.TenantID == right.TenantID && left.TaskID == right.TaskID && left.TaskTurnID == right.TaskTurnID && left.FinalizationRunID == right.FinalizationRunID && left.AttemptID == right.AttemptID && left.ResultCommit == right.ResultCommit && left.ResultTree == right.ResultTree && left.HardTimeoutSeconds == right.HardTimeoutSeconds && left.TransientRetryLimit == right.TransientRetryLimit && left.AutomaticRevisionLimit == right.AutomaticRevisionLimit
}

func digestIdentity(identity domain.RunIdentity) string {
	value := strings.Join([]string{
		fmt.Sprintf("%d", identity.TenantID), fmt.Sprintf("%d", identity.TaskID), fmt.Sprintf("%d", identity.TaskTurnID), fmt.Sprintf("%d", identity.FinalizationRunID), fmt.Sprintf("%d", identity.AttemptID), identity.ResultCommit, identity.ResultTree, reviewcontract.SystemContract, reviewcontract.ReportSchemaVersion, fmt.Sprintf("%d", identity.HardTimeoutSeconds), fmt.Sprintf("%d", identity.TransientRetryLimit), fmt.Sprintf("%d", identity.AutomaticRevisionLimit),
	}, "\x00")
	if identity.CompletionCheckDigest != "" {
		value += "\x00completionCheckDigest\x00" + identity.CompletionCheckDigest
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func canonicalJSONBytes(raw []byte) []byte {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

func jsonBytesEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func (s *Store) SummaryForTask(ctx context.Context, tenantID, taskID int64) (domain.Summary, error) {
	var status string
	err := s.db.QueryRow(ctx, `SELECT status FROM workforce_review_runs WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at DESC,id DESC LIMIT 1`, tenantID, taskID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Summary{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Summary{}, err
	}
	return domain.Summary{Required: true, Status: status}, nil
}

// ReportForTask returns a deliberately narrow, canonical-only projection for
// a task-authorized reader.  The query is tenant-scoped even though callers
// have already authorized the Task: a guessed task ID must never disclose a
// ReviewRun from another organization.
func (s *Store) ReportForTask(ctx context.Context, tenantID, taskID int64) (domain.TaskReport, error) {
	var status, verdict, digest, quality, taskStatus, stopReason string
	var automaticNumber, automaticLimit, noProgress int
	var canonical []byte
	err := s.db.QueryRow(ctx, `SELECT r.status,COALESCE(r.report_verdict,''),COALESCE(r.canonical_report_digest,''),r.canonical_report_json,
 t.quality_outcome,t.status,r.automatic_revision_number,r.automatic_revision_limit,r.no_progress_streak,COALESCE(NULLIF(r.closure_reason_code,''),'')
FROM workforce_review_runs r JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
WHERE r.tenant_id=$1 AND r.task_id=$2
ORDER BY r.created_at DESC,r.id DESC
LIMIT 1`, tenantID, taskID).Scan(&status, &verdict, &digest, &canonical, &quality, &taskStatus, &automaticNumber, &automaticLimit, &noProgress, &stopReason)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TaskReport{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TaskReport{}, err
	}
	result := domain.TaskReport{Required: true, Status: status, Verdict: verdict, ReportDigest: digest, QualityOutcome: quality, AutomaticRevisionCount: automaticNumber, AutomaticRevisionLimit: automaticLimit, NoProgressStreak: noProgress, StopReason: stopReason, AvailableActions: []string{}}
	if taskStatus == "MANUAL_REVIEW" {
		if (verdict == "REVISE" || verdict == "REJECT") && digest != "" {
			result.AvailableActions = append(result.AvailableActions, domain.HumanActionAcceptRisk)
		}
		result.AvailableActions = append(result.AvailableActions, domain.HumanActionAuthorizeRevision, domain.HumanActionCancelTask)
		result.RevisionGuidanceRequired = digest == ""
	}
	if len(canonical) == 0 {
		return result, nil
	}
	// Canonical JSON was generated in SubmitReport. Re-canonicalize it rather
	// than exposing stored bytes; a corrupted historical row (including one
	// with a freshly inserted secret) fails closed if either validation or the
	// persisted digest no longer agrees.
	report, recalculated, err := domain.CanonicalizeReport(canonical)
	if err != nil || digest == "" || !secureEqual(digest, recalculated) {
		return domain.TaskReport{}, domain.ErrInvalidReport
	}
	result.Verdict = report.Verdict
	result.Score = report.Score
	result.Summary = report.Summary
	result.Findings = append([]domain.Finding(nil), report.Findings...)
	result.ReviewTests = append([]domain.ReviewTest(nil), report.ReviewTests...)
	result.Limitations = append([]domain.Limitation(nil), report.Limitations...)
	return result, nil
}

func (s *Store) ReviewRunRetention(ctx context.Context, taskID int64) (facade.RetentionDecision, error) {
	rows, err := s.db.Query(ctx, `SELECT status,finalization_run_id FROM workforce_review_runs WHERE task_id=$1`, taskID)
	if err != nil {
		return facade.RetentionDecision{}, err
	}
	defer rows.Close()
	var finalizationIDs []int64
	for rows.Next() {
		var status string
		var finalizationID int64
		if err := rows.Scan(&status, &finalizationID); err != nil {
			return facade.RetentionDecision{}, err
		}
		if domain.IsActive(status) {
			return facade.RetentionDecision{ActiveReferences: true, PortableStateAvailable: false, ReasonCode: "ACTIVE_REVIEW_RUN"}, nil
		}
		finalizationIDs = append(finalizationIDs, finalizationID)
	}
	if err := rows.Err(); err != nil {
		return facade.RetentionDecision{}, err
	}
	for _, finalizationID := range finalizationIDs {
		var status, commit, tree string
		if err := s.db.QueryRow(ctx, `SELECT status,COALESCE(result_commit,''),COALESCE(result_tree,'') FROM workforce_finalization_runs WHERE id=$1`, finalizationID).Scan(&status, &commit, &tree); err != nil || status != "FROZEN" || commit == "" || tree == "" {
			return facade.RetentionDecision{PortableStateAvailable: false, ReasonCode: "REVIEW_RESULT_EVIDENCE_UNVERIFIED"}, nil
		}
		var missing bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_artifact_upload_attempts WHERE finalization_run_id=$1 AND status<>'VERIFIED')`, finalizationID).Scan(&missing); err != nil {
			return facade.RetentionDecision{}, err
		}
		if missing {
			return facade.RetentionDecision{PortableStateAvailable: false, ReasonCode: "REVIEW_RESULT_EVIDENCE_UNVERIFIED"}, nil
		}
	}
	return facade.RetentionDecision{PortableStateAvailable: true}, nil
}

func (s *Store) CancelForTask(ctx context.Context, tx pgx.Tx, taskID int64, reason string) error {
	if taskID < 1 {
		return domain.ErrInvalidReviewInput
	}
	rows, err := tx.Query(ctx, `SELECT r.id
FROM workforce_review_runs r
JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
WHERE r.task_id=$1 AND (
  r.status IN ('PENDING','ASSIGNED','RUNNING','VALIDATING_REPORT','RETRY_WAIT')
  OR (r.task_turn_id=t.current_task_turn_id AND r.status IN ('MANUAL_REVIEW','REJECTED'))
)
FOR UPDATE OF r`, taskID)
	if err != nil {
		return err
	}
	var runIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		runIDs = append(runIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, runID := range runIDs {
		if _, err := tx.Exec(ctx, `UPDATE workforce_review_runs SET status='CANCELLED',owner_epoch=owner_epoch+1,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,cancelled_at=CURRENT_TIMESTAMP,error_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,error_code=$2,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')`, runID, reason); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workforce_outbox_events SET status='SUPERSEDED',last_error=$2 WHERE aggregate_type='review_run' AND aggregate_id=$1 AND status='PENDING'`, runID, reason); err != nil {
			return err
		}
	}
	return nil
}

type assignCandidate struct {
	runID                     int64
	tenantID                  int64
	taskID                    int64
	taskTurnID                int64
	finalization              int64
	preferredRuntimeReleaseID int64
	modelCredentialID         int64
	credentialVersion         int64
	modelProtocol             string
	primaryModel              string
}

func (s *Store) AssignAvailable(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	assigned := 0
	for i := 0; i < limit; i++ {
		ok, err := s.assignOne(ctx)
		if err != nil {
			return assigned, err
		}
		if !ok {
			break
		}
		assigned++
	}
	return assigned, nil
}

func (s *Store) assignOne(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var candidate assignCandidate
	err = tx.QueryRow(ctx, `SELECT r.id,r.tenant_id,r.task_id,r.task_turn_id,r.finalization_run_id,
t.model_credential_id,c.secret_version,c.protocol,t.primary_model
FROM workforce_review_runs r
JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
JOIN workforce_model_credentials c ON c.id=t.model_credential_id AND c.tenant_id=t.tenant_id AND c.status='ACTIVE'
WHERE r.status IN ('PENDING','RETRY_WAIT') AND (r.next_retry_at IS NULL OR r.next_retry_at<=CURRENT_TIMESTAMP)
ORDER BY r.created_at,r.id FOR UPDATE OF r SKIP LOCKED LIMIT 1`).Scan(&candidate.runID, &candidate.tenantID, &candidate.taskID, &candidate.taskTurnID, &candidate.finalization, &candidate.modelCredentialID, &candidate.credentialVersion, &candidate.modelProtocol, &candidate.primaryModel)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	err = tx.QueryRow(ctx, `SELECT a.agent_runtime_release_id FROM workforce_task_assignments a
JOIN workforce_execution_nodes n ON n.id=a.execution_node_id
WHERE n.task_turn_id=$1 ORDER BY a.created_at DESC,a.id DESC LIMIT 1`, candidate.taskTurnID).Scan(&candidate.preferredRuntimeReleaseID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if s.assignmentResolver == nil {
		return false, errors.New("review assignment resolver is not configured")
	}
	resolved, waitReason, err := s.assignmentResolver.ResolveReviewAssignment(ctx, tx, routingfacade.ReviewRequirements{
		TenantID:                    candidate.tenantID,
		ReviewRunID:                 candidate.runID,
		PreferredRuntimeReleaseID:   candidate.preferredRuntimeReleaseID,
		RequiredAdapterCapabilities: nil,
		ModelCredentialID:           candidate.modelCredentialID, CredentialVersion: candidate.credentialVersion,
		ModelProtocol: candidate.modelProtocol, PrimaryModel: candidate.primaryModel,
	})
	if err != nil {
		return false, err
	}
	if waitReason != "" {
		return false, setReviewWaitReason(ctx, tx, candidate.runID, waitReason)
	}
	selection := domain.ResolvedSelection{
		RuntimeReleaseID:    resolved.RuntimeReleaseID,
		ReviewerImageDigest: resolved.RuntimeImageDigest,
		AdapterIdentity:     resolved.AdapterID,
		AdapterVersion:      resolved.AdapterVersion,
		AdapterProtocol:     resolved.AdapterProtocol,
		AdapterCapabilities: resolved.AdapterCapabilities,
		ModelCredentialID:   resolved.ModelCredentialID,
		CredentialVersion:   resolved.CredentialVersion,
		ModelProtocol:       resolved.ModelProtocol,
		PrimaryModel:        resolved.PrimaryModel,
		ProviderProxyImage:  resolved.ProviderProxyImage,
		WorkerID:            resolved.WorkerID,
		WorkerInstanceID:    resolved.WorkerInstanceID,
		SelectionDigest:     resolved.SelectionDigest,
	}
	expires := s.now().UTC().Add(30 * time.Second)
	var ownerEpoch int64
	var attemptNumber int
	err = tx.QueryRow(ctx, `UPDATE workforce_review_runs
SET status='ASSIGNED',runtime_release_id=$2,reviewer_image_digest=$3,adapter_identity=$4,adapter_version=$5,
 adapter_protocol=$6,adapter_capabilities_json=$7,model_credential_id=$8,credential_version=$9,
 model_protocol=$10,primary_model=$11,provider_proxy_image_digest=$12,
 selection_digest=$13,worker_id=$14,worker_instance_id=$15,host_boot_id='',owner_epoch=owner_epoch+1,
 assignment_generation=assignment_generation+1,attempt_count=attempt_count+1,lease_token_sha256='',lease_expires_at=$16,
 launch_permit_sha256='',launch_permit_expires_at=NULL,wait_reason_code='',next_retry_at=NULL,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND status IN ('PENDING','RETRY_WAIT')
RETURNING owner_epoch,attempt_count`, candidate.runID, selection.RuntimeReleaseID, selection.ReviewerImageDigest, selection.AdapterIdentity,
		selection.AdapterVersion, selection.AdapterProtocol, selection.AdapterCapabilities, selection.ModelCredentialID,
		selection.CredentialVersion, selection.ModelProtocol, selection.PrimaryModel,
		selection.ProviderProxyImage, selection.SelectionDigest, selection.WorkerID, selection.WorkerInstanceID, expires).Scan(&ownerEpoch, &attemptNumber)
	if err != nil {
		return false, err
	}
	attemptID := s.idGen.NextID()
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_review_run_attempts (
id,review_run_id,attempt_number,runtime_release_id,reviewer_image_digest,adapter_identity,adapter_version,
adapter_protocol,adapter_capabilities_json,model_credential_id,credential_version,model_protocol,
primary_model,provider_proxy_image_digest,selection_digest,
worker_id,worker_instance_id,owner_epoch,status,lease_expires_at,assigned_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'ASSIGNED',$19,CURRENT_TIMESTAMP)`,
		attemptID, candidate.runID, attemptNumber, selection.RuntimeReleaseID, selection.ReviewerImageDigest,
		selection.AdapterIdentity, selection.AdapterVersion, selection.AdapterProtocol, selection.AdapterCapabilities,
		selection.ModelCredentialID, selection.CredentialVersion, selection.ModelProtocol, selection.PrimaryModel,
		selection.ProviderProxyImage, selection.SelectionDigest, selection.WorkerID,
		selection.WorkerInstanceID, ownerEpoch, expires); err != nil {
		return false, err
	}
	messageID := s.idGen.NextID()
	message := domain.AssignmentMessage{SchemaVersion: 1, Kind: domain.AssignmentMessageKind, MessageID: messageID, ReviewRunID: candidate.runID, WorkerID: selection.WorkerID, OwnerEpoch: ownerEpoch, ExpiresAt: expires.Format(time.RFC3339Nano)}
	payload, _ := json.Marshal(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms)
VALUES ($1,'review_run',$2,'review_run.assigned',$3,'PENDING','seven.workforce.tasks',$4,30000)`, messageID, candidate.runID, payload, fmt.Sprintf("worker.%d", selection.WorkerID)); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func setReviewWaitReason(ctx context.Context, tx pgx.Tx, runID int64, reason string) error {
	if _, err := tx.Exec(ctx, `UPDATE workforce_review_runs SET wait_reason_code=$2,next_retry_at=CURRENT_TIMESTAMP+INTERVAL '2 seconds',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func containsAll(actual, required []string) bool {
	set := make(map[string]struct{}, len(actual))
	for _, value := range actual {
		set[strings.TrimSpace(value)] = struct{}{}
	}
	for _, value := range required {
		if _, exists := set[strings.TrimSpace(value)]; !exists {
			return false
		}
	}
	return true
}

func (s *Store) Claim(ctx context.Context, e facade.MachineEnvelope, runID int64) (facade.Claim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.Claim{}, err
	}
	defer tx.Rollback(ctx)
	var claim facade.Claim
	var workerID, ownerEpoch int64
	var instanceID, bootID, status string
	var expires time.Time
	var executionDeadline *time.Time
	err = tx.QueryRow(ctx, `SELECT r.tenant_id,r.task_id,r.task_turn_id,r.finalization_run_id,r.result_commit,r.result_tree,r.completion_check_digest,r.reviewer_image_digest,r.model_credential_id,r.credential_version,r.model_protocol,r.primary_model,
	r.provider_proxy_image_digest,
	a.id,r.hard_timeout_seconds,r.worker_id,r.worker_instance_id,r.host_boot_id,r.status,r.owner_epoch,r.lease_expires_at,r.execution_deadline_at
	FROM workforce_review_runs r
	JOIN workforce_review_run_attempts a ON a.review_run_id=r.id AND a.owner_epoch=r.owner_epoch AND a.status='ASSIGNED'
	 AND a.selection_digest=r.selection_digest AND a.reviewer_image_digest=r.reviewer_image_digest
	 AND a.model_credential_id=r.model_credential_id AND a.credential_version=r.credential_version
	WHERE r.id=$1 FOR UPDATE OF r,a`, runID).Scan(&claim.TenantID, &claim.TaskID, &claim.TaskTurnID, &claim.FinalizationRunID, &claim.ResultCommit, &claim.ResultTree, &claim.CompletionCheckDigest, &claim.ReviewerImage, &claim.ModelCredentialID, &claim.CredentialVersion, &claim.ModelProtocol, &claim.PrimaryModel, &claim.ProviderProxyImageDigest, &claim.ReviewRunAttemptID, &claim.HardTimeoutSeconds, &workerID, &instanceID, &bootID, &status, &ownerEpoch, &expires, &executionDeadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.Claim{}, domain.ErrNotFound
	}
	if err != nil {
		return facade.Claim{}, err
	}
	if workerID != e.WorkerID || instanceID != e.InstanceID || e.OwnerEpoch != ownerEpoch || status != domain.StatusAssigned || !expires.After(s.now()) {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	if err := assertCurrentWorker(ctx, tx, e); err != nil {
		return facade.Claim{}, err
	}
	lease, err := newSecret()
	if err != nil {
		return facade.Claim{}, err
	}
	leaseExpires := s.now().UTC().Add(5 * time.Minute)
	if executionDeadline == nil {
		value := s.now().UTC().Add(time.Duration(claim.HardTimeoutSeconds) * time.Second)
		executionDeadline = &value
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET status='RUNNING',host_boot_id=$2,lease_token_sha256=$3,lease_expires_at=$4,execution_deadline_at=COALESCE(execution_deadline_at,$5),started_at=COALESCE(started_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ASSIGNED' AND owner_epoch=$6`, runID, e.HostBootID, hash(lease), leaseExpires, *executionDeadline, ownerEpoch); err != nil {
		return facade.Claim{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET status='RUNNING',host_boot_id=$2,lease_token_sha256=$3,lease_expires_at=$4,claimed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$5 AND status='ASSIGNED'`, runID, e.HostBootID, hash(lease), leaseExpires, ownerEpoch); err != nil {
		return facade.Claim{}, err
	}
	claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, claim.LeaseExpiresAt, claim.ExecutionDeadlineAt = runID, ownerEpoch, lease, leaseExpires.Format(time.RFC3339Nano), executionDeadline.UTC().Format(time.RFC3339Nano)
	claim.SystemContract, claim.ReportSchemaVersion = reviewcontract.SystemContract, reviewcontract.ReportSchemaVersion
	claim.ProviderStructuredContract = reviewcontract.StructuredOutputContractV1
	_ = bootID
	return claim, tx.Commit(ctx)
}

// Recover adopts an already-recorded local container after a Worker process
// restart. The Server treats its persisted Run/Attempt identity as authority;
// local labels are only evidence to compare, never a source of new work.
func (s *Store) Recover(ctx context.Context, e facade.MachineEnvelope, runID int64, container facade.ContainerRegistration) (facade.Claim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.Claim{}, err
	}
	defer tx.Rollback(ctx)
	if !validContainerRegistration(container) {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	var claim facade.Claim
	var workerID, ownerEpoch int64
	var oldInstance, oldBoot, status, containerID, containerName, imageDigest, identity string
	var executionDeadline *time.Time
	err = tx.QueryRow(ctx, `SELECT r.tenant_id,r.task_id,r.task_turn_id,r.finalization_run_id,r.result_commit,r.result_tree,r.completion_check_digest,r.reviewer_image_digest,r.model_credential_id,r.credential_version,r.model_protocol,r.primary_model,
	r.provider_proxy_image_digest,
	a.id,r.hard_timeout_seconds,r.worker_id,r.worker_instance_id,r.host_boot_id,r.status,r.owner_epoch,r.runtime_container_id,r.runtime_container_name,r.runtime_execution_identity_digest,r.execution_deadline_at
	FROM workforce_review_runs r
	JOIN workforce_review_run_attempts a ON a.review_run_id=r.id AND a.owner_epoch=r.owner_epoch AND a.status='RUNNING'
	 AND a.selection_digest=r.selection_digest AND a.reviewer_image_digest=r.reviewer_image_digest
	 AND a.model_credential_id=r.model_credential_id AND a.credential_version=r.credential_version
	WHERE r.id=$1 FOR UPDATE OF r,a`, runID).Scan(&claim.TenantID, &claim.TaskID, &claim.TaskTurnID, &claim.FinalizationRunID, &claim.ResultCommit, &claim.ResultTree, &claim.CompletionCheckDigest, &claim.ReviewerImage, &claim.ModelCredentialID, &claim.CredentialVersion, &claim.ModelProtocol, &claim.PrimaryModel, &claim.ProviderProxyImageDigest, &claim.ReviewRunAttemptID, &claim.HardTimeoutSeconds, &workerID, &oldInstance, &oldBoot, &status, &ownerEpoch, &containerID, &containerName, &identity, &executionDeadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return facade.Claim{}, domain.ErrNotFound
	}
	if err != nil {
		return facade.Claim{}, err
	}
	imageDigest = claim.ReviewerImage
	claim.ProviderStructuredContract = reviewcontract.StructuredOutputContractV1
	if workerID != e.WorkerID || status != domain.StatusRunning || ownerEpoch != e.OwnerEpoch || containerID != container.ContainerID || containerName != container.ContainerName || imageDigest != container.ImageDigest || identity != container.ExecutionIdentityDigest {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	if err = assertCurrentWorker(ctx, tx, e); err != nil {
		return facade.Claim{}, err
	}
	if executionDeadline == nil {
		value := s.now().UTC().Add(time.Duration(claim.HardTimeoutSeconds) * time.Second)
		executionDeadline = &value
		if err = requireOne(ctx, tx, `UPDATE workforce_review_runs SET execution_deadline_at=$2 WHERE id=$1 AND execution_deadline_at IS NULL`, runID, value); err != nil {
			return facade.Claim{}, err
		}
	}
	if !executionDeadline.After(s.now()) {
		return facade.Claim{}, domain.ErrFenceRejected
	}
	lease, err := newSecret()
	if err != nil {
		return facade.Claim{}, err
	}
	expires := s.now().UTC().Add(5 * time.Minute)
	if err = requireOne(ctx, tx, `UPDATE workforce_review_runs
SET worker_instance_id=$2,host_boot_id=$3,lease_token_sha256=$4,lease_expires_at=$5,
    launch_permit_id=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND status='RUNNING' AND owner_epoch=$6`, runID, e.InstanceID, e.HostBootID, hash(lease), expires, e.OwnerEpoch); err != nil {
		return facade.Claim{}, err
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_review_run_attempts
SET worker_instance_id=$3,host_boot_id=$4,lease_token_sha256=$5,lease_expires_at=$6,
    launch_permit_id=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,updated_at=CURRENT_TIMESTAMP
WHERE review_run_id=$1 AND worker_id=$2 AND owner_epoch=$7 AND status='RUNNING'`, runID, e.WorkerID, e.InstanceID, e.HostBootID, hash(lease), expires, e.OwnerEpoch); err != nil {
		return facade.Claim{}, err
	}
	claim.ReviewRunID, claim.OwnerEpoch, claim.LeaseToken, claim.LeaseExpiresAt, claim.ExecutionDeadlineAt = runID, ownerEpoch, lease, expires.Format(time.RFC3339Nano), executionDeadline.UTC().Format(time.RFC3339Nano)
	claim.SystemContract, claim.ReportSchemaVersion = reviewcontract.SystemContract, reviewcontract.ReportSchemaVersion
	_ = oldInstance
	_ = oldBoot
	return claim, tx.Commit(ctx)
}

func (s *Store) RenewLease(ctx context.Context, e facade.MachineEnvelope, runID int64, lease string) (string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := s.assertLeaseOwner(ctx, tx, e, runID, lease); err != nil {
		return "", err
	}
	expires := s.now().UTC().Add(5 * time.Minute)
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs
SET lease_expires_at=$2,
    launch_permit_expires_at=CASE WHEN runtime_container_id<>'' AND launch_permit_id IS NOT NULL AND launch_permit_sha256<>'' THEN $2 ELSE launch_permit_expires_at END,
    updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND status='RUNNING'`, runID, expires); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_run_attempts
SET lease_expires_at=$2,
    launch_permit_expires_at=CASE WHEN container_id<>'' AND launch_permit_id IS NOT NULL AND launch_permit_sha256<>'' THEN $2 ELSE launch_permit_expires_at END,
    updated_at=CURRENT_TIMESTAMP
WHERE review_run_id=$1 AND owner_epoch=$3 AND status='RUNNING'`, runID, expires, e.OwnerEpoch); err != nil {
		return "", err
	}
	return expires.Format(time.RFC3339Nano), tx.Commit(ctx)
}

func (s *Store) AcquireLaunchPermit(ctx context.Context, e facade.MachineEnvelope, runID int64, lease string) (facade.LaunchPermit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.LaunchPermit{}, err
	}
	defer tx.Rollback(ctx)
	if err := s.assertLeaseOwner(ctx, tx, e, runID, lease); err != nil {
		return facade.LaunchPermit{}, err
	}
	token, err := newSecret()
	if err != nil {
		return facade.LaunchPermit{}, err
	}
	permit := facade.LaunchPermit{PermitID: s.idGen.NextID(), PermitToken: token, ExpiresAt: s.now().UTC().Add(90 * time.Second).Format(time.RFC3339Nano), OwnerEpoch: e.OwnerEpoch}
	expires, _ := time.Parse(time.RFC3339Nano, permit.ExpiresAt)
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET launch_permit_id=$2,launch_permit_sha256=$3,launch_permit_expires_at=$4,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='RUNNING' AND owner_epoch=$5`, runID, permit.PermitID, hash(token), expires, e.OwnerEpoch); err != nil {
		return facade.LaunchPermit{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET launch_permit_id=$2,launch_permit_sha256=$3,launch_permit_expires_at=$4,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$5 AND status='RUNNING'`, runID, permit.PermitID, hash(token), expires, e.OwnerEpoch); err != nil {
		return facade.LaunchPermit{}, err
	}
	return permit, tx.Commit(ctx)
}

// RecordContainer consumes the short-lived launch permit while it is fresh and
// binds it to exactly one opaque Docker identity. Once that identity is
// recorded, the same permit becomes completion authority only for that
// container and only while the owner lease remains live. Lease renewals extend
// its completion window; a Worker cannot use it to replace a registered
// container, nor can it survive cancellation, lease transfer or expiry.
func (s *Store) RecordContainer(ctx context.Context, e facade.MachineEnvelope, runID int64, container facade.ContainerRegistration, lease string, permitID int64, permit string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = s.assertLeaseOwner(ctx, tx, e, runID, lease); err != nil {
		return err
	}
	if err = s.assertLaunchPermit(ctx, tx, runID, e.OwnerEpoch, permitID, permit); err != nil {
		return err
	}
	if !validContainerRegistration(container) {
		return domain.ErrFenceRejected
	}
	var expectedImage, registeredID, registeredName, registeredIdentity string
	if err = tx.QueryRow(ctx, `SELECT reviewer_image_digest,COALESCE(runtime_container_id,''),COALESCE(runtime_container_name,''),COALESCE(runtime_execution_identity_digest,'')
FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&expectedImage, &registeredID, &registeredName, &registeredIdentity); err != nil {
		return err
	}
	if expectedImage != container.ImageDigest {
		return domain.ErrFenceRejected
	}
	if registeredID != "" && (registeredID != container.ContainerID || registeredName != container.ContainerName || registeredIdentity != container.ExecutionIdentityDigest) {
		return domain.ErrFenceRejected
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_review_runs
SET runtime_container_id=$2,runtime_container_name=$3,runtime_execution_identity_digest=$4,
    launch_permit_expires_at=lease_expires_at,updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND status='RUNNING' AND owner_epoch=$5`, runID, container.ContainerID, container.ContainerName, container.ExecutionIdentityDigest, e.OwnerEpoch); err != nil {
		return err
	}
	if err = requireOne(ctx, tx, `UPDATE workforce_review_run_attempts
SET container_id=$3,container_name=$4,image_digest=$5,execution_identity_digest=$6,updated_at=CURRENT_TIMESTAMP
WHERE review_run_id=$1 AND owner_epoch=$2 AND status='RUNNING'`, runID, e.OwnerEpoch, container.ContainerID, container.ContainerName, container.ImageDigest, container.ExecutionIdentityDigest); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SubmitReport makes verdict handling an atomic Server-side state transition.
// The Worker submits a bounded candidate; an invalid candidate is intentionally
// not persisted and turns into one durable manual-review requirement.
func (s *Store) SubmitReport(ctx context.Context, e facade.MachineEnvelope, runID int64, submission facade.ReportSubmission) (facade.ReportResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.ReportResult{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.assertLeaseOwner(ctx, tx, e, runID, submission.LeaseToken); err != nil {
		if errors.Is(err, domain.ErrFenceRejected) {
			return facade.ReportResult{}, domain.NewFenceViolation(domain.FenceStageLease)
		}
		return facade.ReportResult{}, err
	}
	if err = s.assertLaunchPermit(ctx, tx, runID, e.OwnerEpoch, submission.PermitID, submission.PermitToken); err != nil {
		if errors.Is(err, domain.ErrFenceRejected) {
			return facade.ReportResult{}, domain.NewFenceViolation(domain.FenceStagePermit)
		}
		return facade.ReportResult{}, err
	}
	if err = s.assertRegisteredContainer(ctx, tx, runID, e.OwnerEpoch); err != nil {
		if errors.Is(err, domain.ErrFenceRejected) {
			return facade.ReportResult{}, domain.NewFenceViolation(domain.FenceStageContainer)
		}
		return facade.ReportResult{}, err
	}
	result, err := tx.Exec(ctx, `UPDATE workforce_review_runs SET status='VALIDATING_REPORT',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$2 AND status='RUNNING'`, runID, e.OwnerEpoch)
	if err != nil {
		return facade.ReportResult{}, err
	}
	if result.RowsAffected() != 1 {
		return facade.ReportResult{}, domain.NewFenceViolation(domain.FenceStageRunState)
	}
	result, err = tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET status='VALIDATING_REPORT',updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$2 AND status='RUNNING'`, runID, e.OwnerEpoch)
	if err != nil {
		return facade.ReportResult{}, err
	}
	if result.RowsAffected() != 1 {
		return facade.ReportResult{}, domain.NewFenceViolation(domain.FenceStageAttemptState)
	}
	report, digest, reportErr := domain.CanonicalizeReport(submission.Report)
	if reportErr == nil {
		reportErr = s.validateCanonicalEvidence(ctx, tx, runID, report)
	}
	if reportErr != nil {
		// The attempt terminal state is REJECTED. REPORT_INVALID remains the
		// reason code; statuses must stay within the durable attempt state
		// machine so an invalid untrusted document cannot break its own
		// fail-closed transition through a database check constraint.
		if err = s.transitionManualReview(ctx, tx, runID, e.OwnerEpoch, "INDEPENDENT_REVIEW_REPORT_INVALID", "REJECTED"); err != nil {
			return facade.ReportResult{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return facade.ReportResult{}, err
		}
		return facade.ReportResult{Status: domain.StatusManualReview}, domain.ErrInvalidReport
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return facade.ReportResult{}, domain.ErrInvalidReport
	}
	usageJSON, err := json.Marshal(sanitizeUsage(submission.Usage))
	if err != nil {
		return facade.ReportResult{}, domain.ErrInvalidReport
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET canonical_report_json=$2,canonical_report_digest=$3,report_verdict=$4,provider_usage_json=$5,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$6 AND status='VALIDATING_REPORT'`, runID, reportJSON, digest, report.Verdict, usageJSON, e.OwnerEpoch); err != nil {
		return facade.ReportResult{}, err
	}
	facts, err := s.calculateProgress(ctx, tx, runID, report)
	if err != nil {
		return facade.ReportResult{}, err
	}
	if err = persistProgress(ctx, tx, runID, facts); err != nil {
		return facade.ReportResult{}, err
	}
	var resultStatus string
	switch report.Verdict {
	case "PASS":
		resultStatus = domain.StatusPassed
		err = s.transitionPassed(ctx, tx, runID, e.OwnerEpoch)
	case "REVISE":
		resultStatus = domain.StatusRevisionRequired
		err = s.transitionRevisionRequired(ctx, tx, runID, e.OwnerEpoch, reportJSON, digest, facts)
	case "REJECT":
		resultStatus = domain.StatusRejected
		err = s.transitionRejected(ctx, tx, runID, e.OwnerEpoch)
	default:
		err = domain.ErrInvalidReport
	}
	if err != nil {
		return facade.ReportResult{}, err
	}
	// Some verdicts intentionally converge to a different authoritative state.
	// In particular, REVISE may stop at MANUAL_REVIEW because the frozen budget
	// is exhausted, progress stalled, or the source Checkpoint is unavailable.
	// Return the durable state instead of the candidate verdict's nominal state.
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_review_runs WHERE id=$1`, runID).Scan(&resultStatus); err != nil {
		return facade.ReportResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.ReportResult{}, err
	}
	return facade.ReportResult{Status: resultStatus, Verdict: report.Verdict, ReportDigest: digest}, nil
}

// validateCanonicalEvidence rechecks every user-visible finding against the
// immutable Finalizer-derived ResultCommit index persisted with the trusted
// finalization manifest. The API Server never accepts the Review Worker's
// local Git inspection as the authority for paths or lines.
func (s *Store) validateCanonicalEvidence(ctx context.Context, tx pgx.Tx, runID int64, report domain.CanonicalReport) error {
	if len(report.Findings) == 0 {
		return nil
	}
	var manifest []byte
	var finalizationStatus, commit, tree string
	err := tx.QueryRow(ctx, `SELECT f.status,COALESCE(f.result_commit,''),COALESCE(f.result_tree,''),f.manifest_json
FROM workforce_review_runs r
JOIN workforce_finalization_runs f ON f.id=r.finalization_run_id
WHERE r.id=$1 FOR UPDATE OF r,f`, runID).Scan(&finalizationStatus, &commit, &tree, &manifest)
	if err != nil || finalizationStatus != "FROZEN" || len(manifest) == 0 {
		return domain.ErrInvalidReport
	}
	var frozen struct {
		ResultCommit   string `json:"resultCommit"`
		ResultTree     string `json:"resultTree"`
		ReviewEvidence []struct {
			Path      string `json:"path"`
			LineCount int    `json:"lineCount"`
		} `json:"reviewEvidence"`
	}
	if json.Unmarshal(manifest, &frozen) != nil || frozen.ResultCommit != commit || frozen.ResultTree != tree || len(frozen.ReviewEvidence) == 0 || len(frozen.ReviewEvidence) > 20_000 {
		return domain.ErrInvalidReport
	}
	linesByPath := make(map[string]int, len(frozen.ReviewEvidence))
	previous := ""
	for _, entry := range frozen.ReviewEvidence {
		if !validEvidencePath(entry.Path) || entry.LineCount < 0 || (previous != "" && previous >= entry.Path) {
			return domain.ErrInvalidReport
		}
		linesByPath[entry.Path] = entry.LineCount
		previous = entry.Path
	}
	for _, finding := range report.Findings {
		lineCount, ok := linesByPath[finding.Evidence.Path]
		if !ok || finding.Evidence.Line < 1 || finding.Evidence.Line > lineCount {
			return domain.ErrInvalidReport
		}
	}
	return nil
}

func validEvidencePath(value string) bool {
	return value != "" && len(value) <= 1024 && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "../")
}

func (s *Store) assertLaunchPermit(ctx context.Context, tx pgx.Tx, runID, ownerEpoch, permitID int64, permit string) error {
	var stored string
	var expires *time.Time
	var storedID, storedOwner int64
	err := tx.QueryRow(ctx, `SELECT COALESCE(launch_permit_id,0),COALESCE(launch_permit_sha256,''),launch_permit_expires_at,owner_epoch FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&storedID, &stored, &expires, &storedOwner)
	if err != nil || storedID != permitID || storedOwner != ownerEpoch || expires == nil || !expires.After(s.now()) || !secureEqual(stored, hash(permit)) {
		return domain.ErrFenceRejected
	}
	return nil
}

// assertRegisteredContainer makes the Report API completion authority refer
// to the same persisted opaque Docker identity that consumed its permit. A
// lease and a valid permit alone are never enough to manufacture a verdict.
func (s *Store) assertRegisteredContainer(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64) error {
	var runIDValue, runName, runIdentity, attemptID, attemptName, attemptImage, attemptIdentity string
	err := tx.QueryRow(ctx, `SELECT r.runtime_container_id,r.runtime_container_name,r.runtime_execution_identity_digest,
       a.container_id,a.container_name,a.image_digest,a.execution_identity_digest
FROM workforce_review_runs r
JOIN workforce_review_run_attempts a ON a.review_run_id=r.id AND a.owner_epoch=r.owner_epoch
WHERE r.id=$1 AND r.owner_epoch=$2 AND r.status='RUNNING' AND a.status='RUNNING'
FOR UPDATE OF r,a`, runID, ownerEpoch).Scan(&runIDValue, &runName, &runIdentity, &attemptID, &attemptName, &attemptImage, &attemptIdentity)
	if err != nil || runIDValue == "" || runName == "" || runIdentity == "" || runIDValue != attemptID || runName != attemptName || runIdentity != attemptIdentity || !validContainerRegistration(facade.ContainerRegistration{ContainerID: runIDValue, ContainerName: runName, ImageDigest: attemptImage, ExecutionIdentityDigest: runIdentity}) {
		return domain.ErrFenceRejected
	}
	return nil
}

func validContainerRegistration(v facade.ContainerRegistration) bool {
	if len(v.ContainerID) < 12 || len(v.ContainerID) > 128 || len(v.ContainerName) < 1 || len(v.ContainerName) > 128 || !strings.HasPrefix(v.ImageDigest, "sha256:") || len(v.ImageDigest) != 71 || len(v.ExecutionIdentityDigest) != 64 {
		return false
	}
	for _, digest := range []string{strings.TrimPrefix(v.ImageDigest, "sha256:"), v.ExecutionIdentityDigest} {
		for _, r := range digest {
			if !(r >= 'a' && r <= 'f') && !(r >= '0' && r <= '9') {
				return false
			}
		}
	}
	for _, r := range v.ContainerName {
		if !(r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func sanitizeUsage(value domain.UsageFacts) domain.UsageFacts {
	if value.Requests < 0 {
		value.Requests = 0
	}
	if value.InputTokens < 0 {
		value.InputTokens = 0
	}
	if value.OutputTokens < 0 {
		value.OutputTokens = 0
	}
	if value.DurationMilli < 0 {
		value.DurationMilli = 0
	}
	if value.Requests > 1000 {
		value.Requests = 1000
	}
	if value.InputTokens > 10_000_000 {
		value.InputTokens = 10_000_000
	}
	if value.OutputTokens > 10_000_000 {
		value.OutputTokens = 10_000_000
	}
	if value.DurationMilli > 86_400_000 {
		value.DurationMilli = 86_400_000
	}
	return value
}

func (s *Store) RecordTransientFailure(ctx context.Context, e facade.MachineEnvelope, runID int64, lease, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.assertLeaseOwner(ctx, tx, e, runID, lease); err != nil {
		return err
	}
	target, err := s.retryOrManual(ctx, tx, runID, e.OwnerEpoch, reason, "FAILED")
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return s.cancelReviewRunInvocation(ctx, target)
}

func (s *Store) transitionPassed(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64) error {
	var tenantID, taskID, turnID, attemptID, finalizationID int64
	var commit, tree string
	if err := tx.QueryRow(ctx, `SELECT tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,result_commit,result_tree
FROM workforce_review_runs WHERE id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT' FOR UPDATE`, runID, ownerEpoch).Scan(&tenantID, &taskID, &turnID, &attemptID, &finalizationID, &commit, &tree); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrFenceRejected
		}
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE workforce_artifacts SET status='AVAILABLE' WHERE task_id=$1 AND attempt_id=$2 AND status='PENDING_REVIEW'`, taskID, attemptID); err != nil || tag.RowsAffected() < 1 {
		if err != nil {
			return err
		}
		return domain.ErrIdentityConflict
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_task_attempts SET status='SUCCEEDED',result_commit=$2,result_tree=$3,finished_at=CURRENT_TIMESTAMP,exit_code=0 WHERE id=$1 AND status='REVIEWING'`, attemptID, commit, tree); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_execution_nodes SET status='SUCCEEDED',updated_at=CURRENT_TIMESTAMP WHERE id=(SELECT execution_node_id FROM workforce_finalization_runs WHERE id=$1) AND status='REVIEWING'`, finalizationID); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_task_turns SET status='SUCCEEDED',result_commit=$2,result_tree=$3,result_summary='Independent review passed',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='REVIEWING'`, turnID, commit, tree); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_tasks SET status='SUCCEEDED',quality_outcome='INDEPENDENT_REVIEW_PASSED',result_summary='Independent review passed',verified_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status='REVIEWING'`, taskID, turnID); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_review_runs SET status='PASSED',finished_at=CURRENT_TIMESTAMP,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_review_run_attempts SET status='COMPLETED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch); err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.succeeded", map[string]any{"taskTurnId": turnID, "resultCommit": commit, "resultTree": tree, "reviewRunId": runID})
	if err != nil {
		return err
	}
	if s.notificationRecorder != nil {
		_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceIndependentReviewRun, SourceID: runID, SourceOccurrence: occurrence, ReasonCode: "TASK_TURN_SUCCEEDED", Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: s.now().UTC()})
	}
	return err
}

func (s *Store) transitionRevisionRequired(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64, reportJSON []byte, reportDigest string, facts progressFacts) error {
	var tenantID, taskID, turnID, attemptID int64
	var automaticLimit int
	if err := tx.QueryRow(ctx, `SELECT tenant_id,task_id,task_turn_id,attempt_id,automatic_revision_limit FROM workforce_review_runs WHERE id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT' FOR UPDATE`, runID, ownerEpoch).Scan(&tenantID, &taskID, &turnID, &attemptID, &automaticLimit); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrFenceRejected
		}
		return err
	}
	stopReason := revisionStopReason(facts, automaticLimit)
	if stopReason == "" {
		if s.revisionTurnCreator == nil {
			stopReason = reasonCheckpointMissing
		} else {
			created, createErr := s.revisionTurnCreator.CreateReviewRevisionTurn(ctx, tx, continuationfacade.ReviewRevisionTurnRequest{
				TenantID: tenantID, TaskID: taskID, SourceTaskTurnID: turnID, SourceAttemptID: attemptID,
				SourceReviewRunID: runID, RevisionKind: continuationfacade.RevisionAutomatic,
				ReportVerdict: "REVISE", ReportDigest: reportDigest, CanonicalReport: reportJSON,
			})
			if createErr != nil {
				if errors.Is(createErr, continuationfacade.ErrReviewRevisionCheckpointUnavailable) {
					stopReason = reasonCheckpointMissing
				} else if errors.Is(createErr, continuationfacade.ErrReviewRevisionIdentityConflict) {
					stopReason = reasonRevisionConflict
				} else {
					return createErr
				}
			} else {
				if err := requireOne(ctx, tx, `UPDATE workforce_review_runs SET status='REVISION_REQUIRED',successor_task_turn_id=$3,finished_at=CURRENT_TIMESTAMP,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch, created.TaskTurnID); err != nil {
					return err
				}
				if err := requireOne(ctx, tx, `UPDATE workforce_review_run_attempts SET status='COMPLETED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch); err != nil {
					return err
				}
				return nil
			}
		}
	}
	return s.transitionManualReview(ctx, tx, runID, ownerEpoch, stopReason, "COMPLETED")
}

func (s *Store) transitionRejected(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64) error {
	if err := requireOne(ctx, tx, `UPDATE workforce_review_runs SET status='REJECTED',finished_at=CURRENT_TIMESTAMP,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch); err != nil {
		return err
	}
	return s.transitionManualReview(ctx, tx, runID, ownerEpoch, "INDEPENDENT_REVIEW_REJECTED", "REJECTED")
}

func (s *Store) transitionManualReview(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64, reason, attemptStatus string) error {
	var tenantID, taskID, turnID, attemptID int64
	var previous string
	if err := tx.QueryRow(ctx, `SELECT tenant_id,task_id,task_turn_id,attempt_id,status FROM workforce_review_runs WHERE id=$1 AND owner_epoch=$2 FOR UPDATE`, runID, ownerEpoch).Scan(&tenantID, &taskID, &turnID, &attemptID, &previous); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrFenceRejected
		}
		return err
	}
	if previous != domain.StatusValidatingReport && previous != domain.StatusRejected {
		return domain.ErrFenceRejected
	}
	if previous != domain.StatusRejected {
		if err := requireOne(ctx, tx, `UPDATE workforce_review_runs SET status='MANUAL_REVIEW',owner_epoch=owner_epoch+1,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,error_code=$2,closure_reason_code=$2,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$3 AND status='VALIDATING_REPORT'`, runID, reason, ownerEpoch); err != nil {
			return err
		}
	} else {
		if err := requireOne(ctx, tx, `UPDATE workforce_review_runs SET error_code=$2,closure_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND owner_epoch=$3 AND status='REJECTED'`, runID, reason, ownerEpoch); err != nil {
			return err
		}
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_review_run_attempts SET status=$3,error_code=$4,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE review_run_id=$1 AND owner_epoch=$2 AND status='VALIDATING_REPORT'`, runID, ownerEpoch, attemptStatus, reason); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_task_attempts SET status='MANUAL_REVIEW',finished_at=NULL WHERE id=$1 AND status='REVIEWING'`, attemptID); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_task_turns SET status='MANUAL_REVIEW',result_summary='Independent review requires manual action',finished_at=NULL WHERE id=$1 AND status='REVIEWING'`, turnID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='MANUAL_REVIEW',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status='REVIEWING'`, taskID); err != nil {
		return err
	}
	if err := requireOne(ctx, tx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary=$2,version=version+1 WHERE id=$1 AND status='REVIEWING'`, taskID, reason); err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "independent_review.attention_required", map[string]any{"reviewRunId": runID, "reasonCode": reason})
	if err != nil {
		return err
	}
	if s.notificationRecorder != nil {
		_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceIndependentReviewRun, SourceID: runID, SourceOccurrence: occurrence, PreviousStatus: previous, CurrentStatus: domain.StatusManualReview, ReasonCode: reason, Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: s.now().UTC()})
	}
	return err
}

func requireOne(ctx context.Context, tx pgx.Tx, statement string, args ...any) error {
	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrFenceRejected
	}
	return nil
}

func (s *Store) ReconcileExpired(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 256 {
		limit = 32
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// A ReviewRun cannot outlive its frozen hard deadline merely because its
	// Worker kept renewing the shorter lease. Both conditions are authoritative
	// fences: a deadline-expired Run is retried or escalated immediately rather
	// than waiting for the last five-minute lease to elapse.
	rows, err := tx.Query(ctx, `SELECT id,owner_epoch FROM workforce_review_runs
WHERE status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')
  AND (lease_expires_at<CURRENT_TIMESTAMP OR (execution_deadline_at IS NOT NULL AND execution_deadline_at<CURRENT_TIMESTAMP))
ORDER BY LEAST(lease_expires_at,COALESCE(execution_deadline_at,lease_expires_at))
FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type expired struct{ id, owner int64 }
	items := make([]expired, 0, limit)
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.owner); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	targets := make([]invocationCancellationTarget, 0, len(items))
	for _, item := range items {
		target, err := s.retryOrManual(ctx, tx, item.id, item.owner, "REVIEW_LEASE_EXPIRED", "EXPIRED")
		if err != nil {
			return 0, err
		}
		targets = append(targets, target)
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	for _, target := range targets {
		if err := s.cancelReviewRunInvocation(ctx, target); err != nil {
			return len(items), err
		}
	}
	return len(items), nil
}

type invocationCancellationTarget struct {
	tenantID           int64
	reviewRunID        int64
	reviewRunAttemptID int64
	reason             string
}

func (s *Store) retryOrManual(ctx context.Context, tx pgx.Tx, runID, ownerEpoch int64, reason, attemptStatus string) (invocationCancellationTarget, error) {
	var tenantID, taskID, turnID, attemptID int64
	var attempts, limit int
	var status string
	err := tx.QueryRow(ctx, `SELECT tenant_id,task_id,task_turn_id,attempt_id,attempt_count,transient_retry_limit,status
FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&tenantID, &taskID, &turnID, &attemptID, &attempts, &limit, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return invocationCancellationTarget{}, domain.ErrNotFound
	}
	if err != nil {
		return invocationCancellationTarget{}, err
	}
	if status != domain.StatusAssigned && status != domain.StatusRunning && status != domain.StatusValidatingReport {
		return invocationCancellationTarget{}, domain.ErrFenceRejected
	}
	var reviewRunAttemptID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_review_run_attempts
WHERE review_run_id=$1 AND owner_epoch=$2 AND status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')
ORDER BY id DESC LIMIT 1 FOR UPDATE`, runID, ownerEpoch).Scan(&reviewRunAttemptID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return invocationCancellationTarget{}, domain.ErrFenceRejected
		}
		return invocationCancellationTarget{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_review_run_attempts SET status=$3,error_code=$4,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP
WHERE review_run_id=$1 AND owner_epoch=$2 AND status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')`, runID, ownerEpoch, attemptStatus, reason); err != nil {
		return invocationCancellationTarget{}, err
	}
	if attempts >= limit {
		if _, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET status='MANUAL_REVIEW',owner_epoch=owner_epoch+1,lease_token_sha256='',lease_expires_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,error_code=$2,closure_reason_code='INDEPENDENT_REVIEW_RETRY_EXHAUSTED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, reason); err != nil {
			return invocationCancellationTarget{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='MANUAL_REVIEW',finished_at=NULL WHERE id=$1 AND status='REVIEWING'`, attemptID); err != nil {
			return invocationCancellationTarget{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='MANUAL_REVIEW',result_summary='Independent review requires manual action',finished_at=NULL WHERE id=$1 AND status='REVIEWING'`, turnID); err != nil {
			return invocationCancellationTarget{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='MANUAL_REVIEW',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status='REVIEWING'`, taskID); err != nil {
			return invocationCancellationTarget{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary='INDEPENDENT_REVIEW_RETRY_EXHAUSTED',version=version+1 WHERE id=$1 AND status='REVIEWING'`, taskID); err != nil {
			return invocationCancellationTarget{}, err
		}
		occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "independent_review.attention_required", map[string]any{"reviewRunId": runID, "reasonCode": "INDEPENDENT_REVIEW_RETRY_EXHAUSTED"})
		if err != nil {
			return invocationCancellationTarget{}, err
		}
		if s.notificationRecorder != nil {
			_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceIndependentReviewRun, SourceID: runID, SourceOccurrence: occurrence, PreviousStatus: status, CurrentStatus: domain.StatusManualReview, ReasonCode: "INDEPENDENT_REVIEW_RETRY_EXHAUSTED", Recoverability: tasknotification.RecoverabilityUserRecoverable, RetryBudgetExhausted: true, OccurredAt: s.now().UTC()})
			if err != nil {
				return invocationCancellationTarget{}, err
			}
		}
		return invocationCancellationTarget{tenantID: tenantID, reviewRunID: runID, reviewRunAttemptID: reviewRunAttemptID, reason: reason}, nil
	}
	next := s.now().UTC().Add([]time.Duration{time.Second, 10 * time.Second, 60 * time.Second}[minInt(attempts-1, 2)])
	// The hard timeout and registered Docker identity both belong to one
	// concrete ReviewRun Attempt, not to the logical ReviewRun. A transient
	// failure has already fenced the old owner and closed that Attempt, so a
	// replacement must not inherit either its deadline or its container. Keeping
	// the old container identity would make the new owner fence-reject its own
	// container registration; the incremented owner epoch still rejects every
	// late action from the old container.
	_, err = tx.Exec(ctx, `UPDATE workforce_review_runs SET status='RETRY_WAIT',owner_epoch=owner_epoch+1,
runtime_release_id=NULL,reviewer_image_digest='',adapter_identity='',adapter_version='',adapter_protocol='',adapter_capabilities_json='[]'::jsonb,
model_credential_id=NULL,credential_version=NULL,model_protocol='',primary_model='',
provider_proxy_image_digest='',selection_digest='',worker_id=NULL,worker_instance_id='',host_boot_id='',lease_token_sha256='',lease_expires_at=NULL,
execution_deadline_at=NULL,launch_permit_sha256='',launch_permit_expires_at=NULL,runtime_container_id='',runtime_container_name='',
runtime_execution_identity_digest='',next_retry_at=$2,error_code=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, next, reason)
	if err != nil {
		return invocationCancellationTarget{}, err
	}
	return invocationCancellationTarget{tenantID: tenantID, reviewRunID: runID, reviewRunAttemptID: reviewRunAttemptID, reason: reason}, nil
}

func (s *Store) cancelReviewRunInvocation(ctx context.Context, target invocationCancellationTarget) error {
	if s == nil || s.invocationCanceller == nil || target.tenantID < 1 || target.reviewRunID < 1 || target.reviewRunAttemptID < 1 {
		return nil
	}
	return s.invocationCanceller.CancelReviewRunInvocations(ctx, target.tenantID, target.reviewRunID, target.reviewRunAttemptID, target.reason)
}

func (s *Store) assertLeaseOwner(ctx context.Context, tx pgx.Tx, e facade.MachineEnvelope, runID int64, lease string) error {
	var workerID, ownerEpoch int64
	var instance, boot, stored, status string
	var expires, executionDeadline *time.Time
	err := tx.QueryRow(ctx, `SELECT COALESCE(worker_id,0),worker_instance_id,host_boot_id,owner_epoch,COALESCE(lease_token_sha256,''),status,lease_expires_at,execution_deadline_at
FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&workerID, &instance, &boot, &ownerEpoch, &stored, &status, &expires, &executionDeadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != domain.StatusRunning || workerID != e.WorkerID || instance != e.InstanceID || boot != e.HostBootID || ownerEpoch != e.OwnerEpoch || expires == nil || !expires.After(s.now()) || executionDeadline == nil || !executionDeadline.After(s.now()) || !secureEqual(stored, hash(lease)) {
		return domain.ErrFenceRejected
	}
	return assertCurrentWorker(ctx, tx, e)
}

func assertCurrentWorker(ctx context.Context, tx pgx.Tx, e facade.MachineEnvelope) error {
	var instance, boot, desired string
	err := tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id,w.desired_state
FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
WHERE w.id=$1 AND s.ended_at IS NULL AND s.docker_ready AND s.consumer_state='CONSUMING'
AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds'`, e.WorkerID).Scan(&instance, &boot, &desired)
	if err != nil || instance != e.InstanceID || boot != e.HostBootID || desired != "ENABLED" {
		return domain.ErrFenceRejected
	}
	return nil
}

func newSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func secureEqual(left, right string) bool {
	return len(left) == len(right) && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
