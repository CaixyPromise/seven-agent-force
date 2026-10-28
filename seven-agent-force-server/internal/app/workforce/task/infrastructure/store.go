package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	executionaccessfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	interactionfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	reviewrunfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	tasknotificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	workspaceinputfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
)

var (
	ErrNotFound                    = errors.New("task not found")
	ErrIdempotencyConflict         = errors.New("idempotency key payload conflict")
	ErrInvalidTransition           = errors.New("invalid task status transition")
	ErrTaskNotClaimable            = errors.New("task is not claimable")
	ErrProjectConfigurationChanged = errors.New("PROJECT_CONFIGURATION_CHANGED")
	// ErrLegacyProcessorTaskActive makes the P0.9 task-create cutover explicit:
	// a still-active task that was created under validationCommands must finish
	// through its historical contract before this tenant can enqueue a task under
	// a Result Processor contract. Terminal history is intentionally ignored.
	ErrLegacyProcessorTaskActive = errors.New("RESULT_PROCESSOR_LEGACY_TASK_ACTIVE")
)

type IDGenerator interface {
	NextID() int64
}

type Store struct {
	db                   *pgxpool.Pool
	idGen                IDGenerator
	submissionGuard      SubmissionGuard
	notifications        tasknotificationfacade.SubscriptionWriter
	notificationRecorder tasknotificationfacade.TransactionalRecorder
	reviewRuns           reviewrunfacade.TaskLifecycle
	interactions         interactionfacade.AttemptLifecycle
	executionAccess      executionaccessfacade.Lifecycle
	deliveryLifecycle    deliveryfacade.TaskLifecycle
	reviewRetention      reviewrunfacade.WorkspaceRetention
	models               providerfacade.TaskSelectionValidator
	runtimes             routingfacade.RuntimeCatalog
}

func (s *Store) SetModelSelectionValidator(validator providerfacade.TaskSelectionValidator) {
	s.models = validator
}
func (s *Store) SetRuntimeCatalog(catalog routingfacade.RuntimeCatalog) { s.runtimes = catalog }

type ListFilter struct {
	TenantID  int64
	ProjectID int64
	Query     string
	View      string
	From      *time.Time
	To        *time.Time
	Cursor    int64
	Limit     int
}

type TaskPage struct {
	Items      []domain.Task
	NextCursor int64
}

func NewStore(db *pgxpool.Pool, idGen IDGenerator) *Store { return &Store{db: db, idGen: idGen} }

// IndependentReviewInput returns the frozen Turn instruction for the exact
// tenant/task/turn occurrence.  It is intentionally a narrow read contract:
// no mutable Task workspace or internal execution diagnostics cross this
// boundary.
func (s *Store) IndependentReviewInput(ctx context.Context, tenantID, taskID, turnID int64) (taskfacade.IndependentReviewInput, error) {
	if s == nil || s.db == nil || tenantID < 1 || taskID < 1 || turnID < 1 {
		return taskfacade.IndependentReviewInput{}, ErrNotFound
	}
	var result taskfacade.IndependentReviewInput
	var inputJSON, referencesJSON []byte
	err := s.db.QueryRow(ctx, `SELECT t.tenant_id,t.id,turn.id,turn.instruction,COALESCE(turn.completion_check_json,'null'::jsonb),COALESCE(turn.completion_check_digest,''),COALESCE(turn.completion_check_references_json,'[]'::jsonb),v.review_prompt
FROM workforce_tasks t
JOIN workforce_task_turns turn ON turn.id=$3 AND turn.task_id=t.id AND turn.tenant_id=t.tenant_id
JOIN workforce_project_versions v ON v.tenant_id=t.tenant_id AND v.project_id=t.project_id AND v.id=t.project_version_id
WHERE t.tenant_id=$1 AND t.id=$2`, tenantID, taskID, turnID).Scan(&result.TenantID, &result.TaskID, &result.TaskTurnID, &result.Instruction, &inputJSON, &result.CompletionCheckDigest, &referencesJSON, &result.ReviewPrompt)
	if errors.Is(err, pgx.ErrNoRows) {
		return taskfacade.IndependentReviewInput{}, ErrNotFound
	}
	if err != nil || !taskfacade.ValidInstruction(result.Instruction) {
		return taskfacade.IndependentReviewInput{}, ErrNotFound
	}
	var input *completioncheck.Input
	if err = json.Unmarshal(inputJSON, &input); err != nil {
		return taskfacade.IndependentReviewInput{}, err
	}
	if input != nil {
		var digestReferences []completioncheck.Reference
		if json.Unmarshal(referencesJSON, &digestReferences) != nil {
			return taskfacade.IndependentReviewInput{}, completioncheck.ErrConflict
		}
		digest, e := completioncheck.FrozenDigest(*input, digestReferences)
		if e != nil || digest != result.CompletionCheckDigest {
			return taskfacade.IndependentReviewInput{}, completioncheck.ErrConflict
		}
		result.Instructions = input.Instructions
		var refs []domain.CompletionCheckReference
		if err = json.Unmarshal(referencesJSON, &refs); err != nil {
			return taskfacade.IndependentReviewInput{}, err
		}
		for _, id := range input.ReferenceAssetIDs {
			found := false
			for _, ref := range refs {
				if ref.ReferenceAssetID == id {
					result.References = append(result.References, taskfacade.IndependentReviewReference{ReferenceAssetID: id, Name: ref.Name, ContentSHA256: ref.ContentSHA256, SizeBytes: ref.SizeBytes})
					found = true
					break
				}
			}
			if !found {
				return taskfacade.IndependentReviewInput{}, completioncheck.ErrConflict
			}
		}
	}
	if input == nil && result.CompletionCheckDigest != "" {
		return taskfacade.IndependentReviewInput{}, completioncheck.ErrConflict
	}
	return result, nil
}

type SubmissionGuard interface {
	ValidateApplicationForSubmission(context.Context, pgx.Tx, int64, int64, int64) error
}

func (s *Store) SetSubmissionGuard(guard SubmissionGuard) {
	if s != nil {
		s.submissionGuard = guard
	}
}

func (s *Store) SetNotificationSubscriptions(writer tasknotificationfacade.SubscriptionWriter) {
	if s != nil {
		s.notifications = writer
	}
}
func (s *Store) SetTaskNotificationRecorder(recorder tasknotificationfacade.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}

// SetReviewRunLifecycle binds the stable cross-module cancellation fence.
// Task owns the transaction; ReviewRun owns how its leases and outbox records
// are invalidated inside that transaction.
func (s *Store) SetReviewRunLifecycle(lifecycle reviewrunfacade.TaskLifecycle) {
	if s != nil {
		s.reviewRuns = lifecycle
	}
}

func (s *Store) SetDeliveryLifecycle(lifecycle deliveryfacade.TaskLifecycle) {
	if s != nil {
		s.deliveryLifecycle = lifecycle
	}
}

// SetReviewRunWorkspaceRetention keeps Slice 1's GC authority fail-closed as
// new active ReviewRun states are introduced.
func (s *Store) SetReviewRunWorkspaceRetention(retention reviewrunfacade.WorkspaceRetention) {
	if s != nil {
		s.reviewRetention = retention
	}
}

type CreateParams struct {
	Transaction                  pgx.Tx
	StagePurpose                 string
	ApprovedPlan                 json.RawMessage
	FrozenProjectVersion         bool
	ProjectSpec                  *projectfacade.FrozenTaskSpec
	CompletionCheck              *completioncheck.Snapshot
	TenantID                     int64
	IdempotencyKey               string
	RequestHash                  string
	TaskInput                    workspaceinputfacade.FrozenTaskInput
	Name                         string
	Tags                         []string
	CreatedBy                    int64
	Instruction                  string
	ResultProcessorSelectionMode string
	ResultProcessorExecution     processorfacade.Execution
	ReviewEnabled                bool
	ReviewPrompt                 string
	ModelCredentialID            int64
	CredentialVersion            int64
	ModelProtocol                string
	PrimaryModel                 string
	SelectedRuntimeReleaseID     int64
	Priority                     int
	ExecutionTimeoutSeconds      int
	SubmissionChannel            string
	PrincipalType                string
	IntegrationApplicationID     int64
	ScenarioKey                  string
	ScenarioVersion              int
	ScenarioDigest               string
	ExternalCorrelationID        string
	NotificationSubscription     *tasknotificationfacade.SubscriptionCommand
}

func RequestHashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Store) FindIdempotent(ctx context.Context, tenantID int64, creatorUserID int64, idempotencyKey, requestHash string, integrationApplicationID int64) (domain.Task, bool, bool, error) {
	if creatorUserID <= 0 {
		return domain.Task{}, false, false, errors.New("TASK_CREATOR_IDENTITY_REQUIRED")
	}
	if integrationApplicationID > 0 {
		tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return domain.Task{}, false, false, err
		}
		defer tx.Rollback(ctx)
		if s.submissionGuard == nil {
			return domain.Task{}, false, false, errors.New("integration application submission guard is unavailable")
		}
		if err := s.submissionGuard.ValidateApplicationForSubmission(ctx, tx, tenantID, integrationApplicationID, creatorUserID); err != nil {
			return domain.Task{}, false, false, err
		}
		var existingID int64
		var existingHash string
		err = tx.QueryRow(ctx, `SELECT id,request_hash FROM workforce_tasks WHERE tenant_id=$1 AND created_by=$2 AND idempotency_key=$3`, tenantID, creatorUserID, idempotencyKey).Scan(&existingID, &existingHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Task{}, false, false, tx.Commit(ctx)
		}
		if err != nil {
			return domain.Task{}, false, false, err
		}
		if existingHash != requestHash {
			return domain.Task{}, false, true, tx.Commit(ctx)
		}
		task, err := s.getWith(ctx, tx, existingID)
		if err != nil {
			return domain.Task{}, false, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Task{}, false, false, err
		}
		return task, true, false, nil
	}
	var existingID int64
	var existingHash string
	err := s.db.QueryRow(ctx, `SELECT id, request_hash FROM workforce_tasks WHERE tenant_id=$1 AND created_by=$2 AND idempotency_key=$3`, tenantID, creatorUserID, idempotencyKey).Scan(&existingID, &existingHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, false, false, nil
	}
	if err != nil {
		return domain.Task{}, false, false, err
	}
	if existingHash != requestHash {
		return domain.Task{}, false, true, nil
	}
	task, err := s.Get(ctx, existingID)
	return task, true, false, err
}

func (s *Store) Create(ctx context.Context, p CreateParams) (domain.Task, bool, error) {
	if p.Transaction != nil {
		return s.createTx(ctx, p.Transaction, p)
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Task{}, false, err
	}
	defer tx.Rollback(ctx)
	task, replay, err := s.createTx(ctx, tx, p)
	if err != nil {
		return domain.Task{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Task{}, false, err
	}
	return task, replay, nil
}

func (s *Store) createTx(ctx context.Context, tx pgx.Tx, p CreateParams) (domain.Task, bool, error) {
	if p.SelectedRuntimeReleaseID < 1 || p.CreatedBy < 1 {
		return domain.Task{}, false, errors.New("TASK_SELECTION_OR_ACTOR_REQUIRED")
	}
	var err error

	// Application submissions are serialized with enable/disable before any
	// idempotency decision. Holding this row lock until commit makes a disabled
	// application a hard boundary for both new creates and concurrent replays.
	if p.IntegrationApplicationID > 0 {
		if s.submissionGuard == nil {
			return domain.Task{}, false, errors.New("integration application submission guard is unavailable")
		}
		if err := s.submissionGuard.ValidateApplicationForSubmission(ctx, tx, p.TenantID, p.IntegrationApplicationID, p.CreatedBy); err != nil {
			return domain.Task{}, false, err
		}
	}

	var existingID int64
	var existingHash string
	err = tx.QueryRow(ctx, `SELECT id, request_hash FROM workforce_tasks WHERE tenant_id=$1 AND created_by=$2 AND idempotency_key=$3`, p.TenantID, p.CreatedBy, p.IdempotencyKey).Scan(&existingID, &existingHash)
	if err == nil {
		if existingHash != p.RequestHash {
			return domain.Task{}, false, ErrIdempotencyConflict
		}
		task, err := s.getWith(ctx, tx, existingID)
		return task, true, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, false, err
	}
	// New P0.9 tasks never execute validationCommands. Do not silently run two
	// result contracts concurrently for one tenant while a legacy task is still
	// active: that would make retry/recovery semantics ambiguous. CANCELLING is
	// deliberately excluded because it has already been fenced and must not
	// prevent unrelated new work from being created.
	var legacyActive bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM workforce_tasks
  WHERE tenant_id=$1
	AND result_processor_release_id IS NULL
    AND status IN ('QUEUED','PREPARING','RUNNING','VALIDATING','PACKAGING','UPLOADING','RETRY_WAIT','REVIEWING','MANUAL_REVIEW')
)`, p.TenantID).Scan(&legacyActive); err != nil {
		return domain.Task{}, false, err
	}
	if legacyActive {
		return domain.Task{}, false, ErrLegacyProcessorTaskActive
	}
	var (
		projectID                   int64
		projectVersionID            int64
		executionRequirements       = json.RawMessage(`{}`)
		executionRequirementsDigest string
	)
	if p.ProjectSpec != nil {
		spec := p.ProjectSpec
		if spec.ProjectID < 1 || spec.ProjectVersionID < 1 || len(spec.ExecutionRequirementsDigest) != 64 {
			return domain.Task{}, false, errors.New("PROJECT_CONFIGURATION_INVALID")
		}
		var status string
		var currentVersion int64
		if err := tx.QueryRow(ctx, `SELECT status,current_version_id FROM workforce_projects WHERE tenant_id=$1 AND id=$2 FOR SHARE`, p.TenantID, spec.ProjectID).Scan(&status, &currentVersion); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.Task{}, false, errors.New("PROJECT_NOT_FOUND")
			}
			return domain.Task{}, false, err
		}
		if status != projectfacade.StatusActive || (!p.FrozenProjectVersion && currentVersion != spec.ProjectVersionID) {
			return domain.Task{}, false, ErrProjectConfigurationChanged
		}
		if p.FrozenProjectVersion {
			var versionID int64
			if err := tx.QueryRow(ctx, `SELECT id FROM workforce_project_versions WHERE id=$1 AND project_id=$2 AND tenant_id=$3 FOR SHARE`, spec.ProjectVersionID, spec.ProjectID, p.TenantID).Scan(&versionID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return domain.Task{}, false, ErrProjectConfigurationChanged
				}
				return domain.Task{}, false, err
			}
		}
		requirementsJSON, err := json.Marshal(spec.ExecutionRequirements)
		if err != nil {
			return domain.Task{}, false, errors.New("PROJECT_CONFIGURATION_INVALID")
		}
		projectID = spec.ProjectID
		projectVersionID = spec.ProjectVersionID
		executionRequirements = requirementsJSON
		executionRequirementsDigest = spec.ExecutionRequirementsDigest
	}
	plan := p.TaskInput.Snapshot.GitPlan

	if p.ProjectSpec == nil {
		return domain.Task{}, false, errors.New("PROJECT_REQUIRED")
	}
	if s.models == nil || s.runtimes == nil {
		return domain.Task{}, false, errors.New("Task execution selection validators are unavailable")
	}
	selection, err := s.models.ValidateTaskSelectionInTransaction(ctx, tx, p.TenantID, p.ModelCredentialID, p.PrimaryModel)
	if err != nil {
		return domain.Task{}, false, err
	}
	if err = s.runtimes.ValidateTaskSelectionInTransaction(ctx, tx, p.TenantID, p.ProjectSpec.ProjectVersionID, p.SelectedRuntimeReleaseID, selection.Protocol, p.ProjectSpec.ExecutionRequirements); err != nil {
		return domain.Task{}, false, err
	}
	p.CredentialVersion, p.ModelProtocol = selection.CredentialVersion, selection.Protocol
	// The Task freezes business policy only. Runtime, Provider, model, Hook and
	// Worker identities are resolved later by executionrouting and persisted on
	// the Assignment/Attempt.
	snapshot := domain.ExecutionSnapshot{IsolationProfile: "seven.worker-image/v1", StagePurpose: p.StagePurpose, ApprovedPlan: p.ApprovedPlan}
	if p.ResultProcessorExecution.ReleaseID <= 0 || strings.TrimSpace(p.ResultProcessorExecution.Digest) == "" {
		return domain.Task{}, false, errors.New("Result Processor execution identity is required")
	}
	snapshot.ResultProcessorReleaseID = p.ResultProcessorExecution.ReleaseID
	snapshot.ResultProcessorDigest = p.ResultProcessorExecution.Digest
	snapshot.EnvironmentConfigDigest = p.ResultProcessorExecution.EnvironmentConfigDigest
	snapshot.ResultProcessorEnvironment = append(json.RawMessage(nil), p.ResultProcessorExecution.Environment...)
	snapshot.SecretVersionIdentities = append(json.RawMessage(nil), p.ResultProcessorExecution.SecretVersionIdentities...)
	snapshot.ResultProcessorParameters = append(json.RawMessage(nil), p.ResultProcessorExecution.Parameters...)
	snapshot.MaxEvaluationCycles = p.ResultProcessorExecution.MaxEvaluationCycles
	snapshot.ContinuationPolicy = p.ResultProcessorExecution.ContinuationPolicy
	snapshot.DeliveryPolicy = p.ResultProcessorExecution.DeliveryPolicy
	taskID := s.idGen.NextID()
	turnID := s.idGen.NextID()
	plan, err = expandResultBranch(plan, taskID)
	if err != nil {
		return domain.Task{}, false, err
	}
	p.TaskInput.Snapshot.GitPlan = plan
	p.TaskInput.Snapshot.Digest = ""
	inputIdentityJSON, _ := json.Marshal(p.TaskInput.Snapshot)
	inputIdentityDigest := sha256.Sum256(inputIdentityJSON)
	p.TaskInput.Snapshot.Digest = hex.EncodeToString(inputIdentityDigest[:])
	if p.CompletionCheck != nil {
		snapshot.CompletionCheck = &p.CompletionCheck.Input
		snapshot.CompletionCheckDigest = p.CompletionCheck.Digest
		snapshot.CompletionCheckInputDigest = p.CompletionCheck.Digest
		for _, id := range p.CompletionCheck.Input.ReferenceAssetIDs {
			found := false
			for _, ref := range p.TaskInput.References {
				if ref.ID == id {
					snapshot.CompletionCheckReferences = append(snapshot.CompletionCheckReferences, domain.CompletionCheckReference{ReferenceAssetID: id, Name: ref.DisplayName, ContentSHA256: ref.ContentSHA256, ObjectSHA256: ref.ObjectSHA256, SizeBytes: ref.SizeBytes})
					found = true
					break
				}
			}
			if !found {
				return domain.Task{}, false, completioncheck.ErrInvalid
			}
		}
		var digestErr error
		snapshot.CompletionCheckDigest, digestErr = completioncheck.FrozenDigest(*snapshot.CompletionCheck, snapshot.CompletionCheckReferences)
		if digestErr != nil {
			return domain.Task{}, false, digestErr
		}
	}
	snapshot.ExecutionDeadlineSeconds = p.ExecutionTimeoutSeconds
	executionRequirementDigest := executionRequirementsDigest
	tagsJSON, _ := json.Marshal(p.Tags)
	insertResult, err := tx.Exec(ctx, `
INSERT INTO workforce_tasks (id,tenant_id,idempotency_key,request_hash,name,tags_json,created_by,metadata_version,repository_url,repository_ref,task_instruction,validation_commands_json,result_processor_release_id,result_processor_parameters_json,priority,status,queued_at,current_task_turn_id,current_turn_number,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,project_id,project_version_id,review_enabled,model_credential_id,primary_model,selected_runtime_release_id,execution_timeout_seconds,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,$6,$7,1,'','',$8,'[]'::jsonb,$9,$10,$11,'QUEUED',CURRENT_TIMESTAMP,$12,1,$13,NULLIF($14::BIGINT,0),NULLIF($15,''),NULLIF($16::INTEGER,0),NULLIF($17,''),NULLIF($18,''),$19,$20,$21,$22,$23,$24,NULLIF($25::INTEGER,0),$26,$27)
ON CONFLICT (tenant_id,created_by,idempotency_key) DO NOTHING`,
		taskID, p.TenantID, p.IdempotencyKey, p.RequestHash, p.Name, tagsJSON, p.CreatedBy, p.Instruction, snapshot.ResultProcessorReleaseID, defaultJSON(snapshot.ResultProcessorParameters), p.Priority, turnID, normalizedChannel(p.SubmissionChannel), p.IntegrationApplicationID, p.ScenarioKey, p.ScenarioVersion, p.ScenarioDigest, p.ExternalCorrelationID, projectID, projectVersionID, p.ReviewEnabled, p.ModelCredentialID, p.PrimaryModel, p.SelectedRuntimeReleaseID, p.ExecutionTimeoutSeconds, executionRequirements, executionRequirementsDigest)
	if err != nil {
		return domain.Task{}, false, err
	}
	if insertResult.RowsAffected() == 0 {
		if err := tx.QueryRow(ctx, `SELECT id,request_hash FROM workforce_tasks WHERE tenant_id=$1 AND created_by=$2 AND idempotency_key=$3`, p.TenantID, p.CreatedBy, p.IdempotencyKey).Scan(&existingID, &existingHash); err != nil {
			return domain.Task{}, false, err
		}
		if existingHash != p.RequestHash {
			return domain.Task{}, false, ErrIdempotencyConflict
		}
		task, err := s.getWith(ctx, tx, existingID)
		return task, true, err
	}
	if len(p.ApprovedPlan) > 0 {
		if !json.Valid(p.ApprovedPlan) || len(p.ApprovedPlan) > 256*1024 {
			return domain.Task{}, false, errors.New("WORK_ITEM_PLAN_INPUT_INVALID")
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET work_item_input_json=$2 WHERE id=$1`, taskID, p.ApprovedPlan); err != nil {
			return domain.Task{}, false, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_turns (id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,environment_config_digest,environment_snapshot_json,secret_version_identities,parameters_json,status) VALUES ($1,$2,$3,1,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,'ACTIVE')`, turnID, taskID, p.TenantID, p.Instruction, executionRequirementDigest, taskID, p.CreatedBy, snapshot.ResultProcessorReleaseID, snapshot.ResultProcessorDigest, snapshot.EnvironmentConfigDigest, defaultJSON(snapshot.ResultProcessorEnvironment), defaultJSONArray(snapshot.SecretVersionIdentities), defaultJSON(snapshot.ResultProcessorParameters)); err != nil {
		return domain.Task{}, false, err
	}
	inputID := s.idGen.NextID()
	completionJSON, _ := json.Marshal(snapshot.CompletionCheck)
	completionReferencesJSON, _ := json.Marshal(snapshot.CompletionCheckReferences)
	if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_json=$2,completion_check_digest=$3,completion_check_references_json=$4,completion_check_input_digest=$5 WHERE id=$1`, turnID, completionJSON, snapshot.CompletionCheckDigest, completionReferencesJSON, snapshot.CompletionCheckInputDigest); err != nil {
		return domain.Task{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_input_snapshots (id,tenant_id,task_id,task_turn_id,source_kind,workspace_source_revision_id,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest) VALUES ($1,$2,$3,$4,$5,NULLIF($6::BIGINT,0),$7,$8,$9,$10,$11,$12,$13,$14)`, inputID, p.TenantID, taskID, turnID, p.TaskInput.Snapshot.SourceKind, p.TaskInput.Source.RevisionID, p.TaskInput.Source.Commit, p.TaskInput.Source.Tree, p.TaskInput.Source.IdentityDigest, p.TaskInput.Source.ObjectKey, p.TaskInput.Source.ContentSHA256, p.TaskInput.Source.ObjectSHA256, p.TaskInput.Source.SizeBytes, p.TaskInput.Snapshot.Digest); err != nil {
		return domain.Task{}, false, err
	}
	for index, item := range p.TaskInput.References {
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_input_references (task_input_snapshot_id,reference_asset_id,reference_content_sha256,display_order) VALUES ($1,$2,$3,$4)`, inputID, item.ID, item.ContentSHA256, index); err != nil {
			return domain.Task{}, false, err
		}
	}
	planJSON, _ := json.Marshal(plan)
	planSum := sha256.Sum256(planJSON)
	deliveryAuthorizedByPrincipalID := int64(0)
	if p.ProjectSpec != nil {
		deliveryAuthorizedByPrincipalID = p.ProjectSpec.GitPlan.DeliveryAuthorizedByPrincipalID
	}
	frozenRepository := projectfacade.FrozenGitPlan{}
	if p.ProjectSpec != nil {
		frozenRepository = p.ProjectSpec.GitPlan
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_task_git_plans (id,tenant_id,task_id,task_turn_id,project_id,project_version_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_config_digest,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,review_base_source_revision_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,delivery_authorized_at,target_config_digest,push_authorization_digest,review_authorization_digest,full_project_import,plan_digest) VALUES ($1,$2,$3,$4,NULLIF($5::BIGINT,0),NULLIF($6::BIGINT,0),$7,$8,$9,$10,$11,$12,$13,$14,decode($15,'base64'),decode($16,'base64'),$17,$18,NULLIF($19::BIGINT,0),$20,$21,$22,$23,$24,$25,$26,$27::TEXT,CASE WHEN $27::TEXT='AUTO_AFTER_GATES' THEN NULLIF($28::BIGINT,0) ELSE NULL::BIGINT END,CASE WHEN $27::TEXT='AUTO_AFTER_GATES' THEN CURRENT_TIMESTAMP ELSE NULL::TIMESTAMPTZ END,$29,$30,$31,$32,$33)`, s.idGen.NextID(), p.TenantID, taskID, turnID, frozenRepository.ProjectID, frozenRepository.ProjectVersionID, frozenRepository.RepositoryProviderType, frozenRepository.RepositoryScheme, frozenRepository.RepositoryHost, frozenRepository.RepositoryPort, frozenRepository.RepositoryPath, frozenRepository.RepositoryDisplayURL, frozenRepository.RepositoryWebURL, frozenRepository.RepositoryConfigDigest, frozenRepository.RepositoryCredentialCiphertext, frozenRepository.RepositoryCredentialEDEK, frozenRepository.RepositoryCredentialWrapKeyRef, frozenRepository.RepositoryCredentialVersion, plan.ReviewBaseSourceRevisionID, p.TaskInput.Source.RefType, p.TaskInput.Source.RefName, p.TaskInput.Source.Commit, plan.ReviewBaseBranch, plan.ReviewBaseCommit, plan.ResultBranch, plan.DeliveryMode, plan.DeliveryAuthorization, deliveryAuthorizedByPrincipalID, plan.TargetConfigDigest, plan.PushAuthorizationDigest, plan.ReviewAuthorizationDigest, plan.FullProjectImport, hex.EncodeToString(planSum[:])); err != nil {
		return domain.Task{}, false, err
	}
	if err := appendEventTx(ctx, tx, taskID, 0, "task.queued", map[string]any{"projectId": projectID, "projectVersionID": projectVersionID, "resultProcessorReleaseId": snapshot.ResultProcessorReleaseID, "taskTurnId": turnID}); err != nil {
		return domain.Task{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_execution_nodes (id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest,wait_reason_code) VALUES ($1,$2,$3,'main','QUEUED',1,$4,'WAITING_SELECTED_RUNTIME')`, s.idGen.NextID(), taskID, turnID, executionRequirementDigest)
	if err != nil {
		return domain.Task{}, false, err
	}
	if s.notifications == nil {
		return domain.Task{}, false, errors.New("Task notification subscription writer is unavailable")
	}
	command := tasknotificationfacade.DefaultSubscriptionCommand()
	if p.NotificationSubscription != nil {
		command = *p.NotificationSubscription
	}
	if _, _, err = s.notifications.CreateSubscription(ctx, tx, p.TenantID, taskID, p.CreatedBy, command); err != nil {
		return domain.Task{}, false, err
	}
	task, err := s.getWith(ctx, tx, taskID)
	return task, false, err
}

func expandResultBranch(plan workspaceinputdomain.GitPlan, taskID int64) (workspaceinputdomain.GitPlan, error) {
	if plan.DeliveryMode == workspaceinputdomain.DeliveryNone {
		return plan, nil
	}
	if taskID < 1 || strings.Count(plan.ResultBranch, "{task-id}") != 1 {
		return workspaceinputdomain.GitPlan{}, errors.New("PROJECT_RESULT_BRANCH_PATTERN_INVALID")
	}
	plan.ResultBranch = strings.Replace(plan.ResultBranch, "{task-id}", strconv.FormatInt(taskID, 10), 1)
	if err := workspaceinputdomain.ValidateGitPlan(plan, workspaceinputdomain.RepositoryPolicy{ReviewSupported: true}); err != nil {
		return workspaceinputdomain.GitPlan{}, errors.New("PROJECT_RESULT_BRANCH_PATTERN_INVALID")
	}
	return plan, nil
}

func normalizedChannel(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "WEB"
	}
	return value
}

func defaultJSON(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(`{}`)
	}
	return value
}

func defaultJSONArray(value json.RawMessage) json.RawMessage {
	if len(value) == 0 || !json.Valid(value) {
		return json.RawMessage(`[]`)
	}
	return value
}

func stringArrayJSON(values []string) json.RawMessage {
	if values == nil {
		return json.RawMessage(`[]`)
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return encoded
}

func (s *Store) Get(ctx context.Context, id int64) (domain.Task, error) {
	return s.getWith(ctx, s.db, id)
}

type rowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Store) getWith(ctx context.Context, q rowQuerier, id int64) (domain.Task, error) {
	var task domain.Task
	var commands, tagsJSON, snapshotJSON, processorParameters, modelCredentialJSON []byte
	err := q.QueryRow(ctx, `
SELECT id,tenant_id,name,tags_json,created_by,metadata_version,repository_url,repository_ref,COALESCE(resolved_base_commit,''),task_instruction,
validation_commands_json,'null'::jsonb,COALESCE(result_processor_release_id,0),COALESCE(result_processor_parameters_json,'{}'::jsonb),status,priority,COALESCE(result_summary,''),
COALESCE(latest_attempt_id,0),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),version,
COALESCE(execution_timeout_seconds,0),execution_active_milliseconds,
COALESCE((SELECT to_char(MAX(e.created_at) AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_task_events e WHERE e.task_id=workforce_tasks.id AND e.attempt_id IS NOT NULL AND (e.event_type IN ('harness.log','harness.event','task.validating') OR e.event_type LIKE 'agent.%' OR e.event_type LIKE 'tool.%')),''),
COALESCE(cancel_request_id,0),cancel_generation,COALESCE(cancel_requested_by_principal_id,0),
COALESCE(to_char(cancel_requested_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(cancel_reason,''),COALESCE(current_task_turn_id,0),current_turn_number
,COALESCE(project_id,0),COALESCE(project_version_id,0),COALESCE(selected_runtime_release_id,0),review_enabled,COALESCE(model_credential_id,0),COALESCE(primary_model,''),COALESCE((SELECT jsonb_build_array(mc.name,mc.protocol) FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),'[]'::jsonb),COALESCE(execution_requirements_digest,'')
FROM workforce_tasks WHERE id=$1`, id).Scan(
		&task.ID, &task.TenantID, &task.Name, &tagsJSON, &task.CreatedBy, &task.MetadataVersion, &task.RepositoryURL, &task.RepositoryRef, &task.ResolvedBaseCommit,
		&task.Instruction, &commands, &snapshotJSON, &task.ResultProcessorReleaseID, &processorParameters, &task.Status, &task.Priority,
		&task.ResultSummary, &task.LatestAttemptID, &task.CreatedAt, &task.StartedAt, &task.FinishedAt, &task.Version, &task.HarnessTimeoutSeconds, &task.ExecutionActiveMilliseconds, &task.LastProgressAt,
		&task.CancelRequestID, &task.CancelGeneration, &task.CancelRequestedBy, &task.CancelRequestedAt, &task.CancelReason, &task.TaskTurnID, &task.CurrentTurnNumber,
		&task.ProjectID, &task.ProjectVersionID, &task.SelectedRuntimeReleaseID, &task.ReviewEnabled, &task.ModelCredentialID, &task.PrimaryModel, &modelCredentialJSON, &task.ExecutionRequirementsDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, ErrNotFound
	}
	if err != nil {
		return domain.Task{}, err
	}
	var credentialLabel []string
	if err := json.Unmarshal(modelCredentialJSON, &credentialLabel); err != nil {
		return domain.Task{}, err
	}
	if len(credentialLabel) == 2 {
		task.ModelCredentialName, task.ModelProtocol = credentialLabel[0], credentialLabel[1]
	}
	if task.ModelCredentialID > 0 {
		task.ModelSelection = &domain.ModelSelectionView{CredentialID: task.ModelCredentialID, CredentialName: task.ModelCredentialName, Protocol: task.ModelProtocol, PrimaryModel: task.PrimaryModel}
	}
	if task.SelectedRuntimeReleaseID > 0 {
		var selected domain.SelectedRuntimeView
		if q.QueryRow(ctx, `SELECT id,COALESCE(NULLIF(manifest_json#>>'{metadata,name}',''),alias),image_digest FROM workforce_harness_releases WHERE id=$1`, task.SelectedRuntimeReleaseID).Scan(&selected.RuntimeReleaseID, &selected.Name, &selected.ImageDigest) == nil {
			task.SelectedRuntime = &selected
		}
	}
	_ = q.QueryRow(ctx, `SELECT i.source_kind,COUNT(r.reference_asset_id) FROM workforce_task_input_snapshots i LEFT JOIN workforce_task_input_references r ON r.task_input_snapshot_id=i.id WHERE i.task_id=$1 AND i.task_turn_id=$2 GROUP BY i.source_kind`, task.ID, task.TaskTurnID).Scan(&task.InputKind, &task.ReferenceCount)
	_ = json.Unmarshal(commands, &task.ValidationCommands)
	_ = json.Unmarshal(tagsJSON, &task.Tags)
	task.ResultProcessorParameters = append(json.RawMessage(nil), processorParameters...)
	if string(snapshotJSON) != "null" {
		var snapshot domain.ExecutionSnapshot
		if json.Unmarshal(snapshotJSON, &snapshot) == nil {
			task.ExecutionSnapshot = &snapshot
		}
	}
	var execution domain.ExecutionView
	err = q.QueryRow(ctx, `SELECT id,node_key,status,COALESCE(wait_reason_code,''),execution_requirement_digest,generation,max_attempts,retry_count,COALESCE(to_char(next_retry_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(last_retry_reason_code,'') FROM workforce_execution_nodes WHERE task_turn_id=$1 AND node_key='main'`, task.TaskTurnID).Scan(&execution.ExecutionNodeID, &execution.NodeKey, &execution.NodeStatus, &execution.QueueWaitReason, &execution.ExecutionRequirementDigest, &execution.Generation, &execution.MaxAttempts, &execution.RetryCount, &execution.NextRetryAt, &execution.LastRetryReasonCode)
	if err == nil {
		var adapterCapabilitiesJSON, resolvedSnapshotJSON []byte
		resolved := domain.ResolvedExecutionView{}
		assignmentErr := q.QueryRow(ctx, `SELECT a.id,a.status,a.worker_id,a.worker_instance_id,
a.agent_runtime_release_id,hr.alias,a.runtime_image_digest,a.adapter_identity,a.adapter_version,a.adapter_protocol,a.adapter_capabilities_json,
a.hook_configuration_digest,a.resolved_execution_snapshot_json,a.model_credential_id,COALESCE(mc.name,''),a.model_protocol,a.primary_model,a.credential_version,
a.worker_image_contract,a.isolation_profile,a.selection_digest,COALESCE((a.resolved_execution_snapshot_json->>'executionDeadlineSeconds')::integer,0)
FROM workforce_task_assignments a
JOIN workforce_harness_releases hr ON hr.id=a.agent_runtime_release_id
LEFT JOIN workforce_model_credentials mc ON mc.id=a.model_credential_id AND mc.tenant_id=$2
WHERE a.execution_node_id=$1 ORDER BY a.created_at DESC LIMIT 1`, execution.ExecutionNodeID, task.TenantID).Scan(
			&execution.AssignmentID, &execution.AssignmentStatus, &execution.WorkerID, &execution.WorkerInstanceID,
			&resolved.RuntimeReleaseID, &resolved.RuntimeAlias, &resolved.RuntimeImageDigest, &resolved.AdapterID, &resolved.AdapterVersion, &resolved.AdapterProtocol, &adapterCapabilitiesJSON,
			&resolved.HookConfigurationDigest, &resolvedSnapshotJSON, &resolved.ModelCredentialID, &resolved.ModelCredentialName, &resolved.ModelProtocol, &resolved.PrimaryModel, &resolved.CredentialVersion,
			&resolved.WorkerImageContract, &resolved.IsolationProfile, &resolved.SelectionDigest, &resolved.ExecutionDeadlineSecs)
		if assignmentErr != nil && !errors.Is(assignmentErr, pgx.ErrNoRows) {
			return domain.Task{}, assignmentErr
		}
		if execution.AssignmentID != 0 {
			if err := json.Unmarshal(adapterCapabilitiesJSON, &resolved.AdapterCapabilities); err != nil {
				return domain.Task{}, err
			}
			resolved.Hooks, err = resolvedHookViews(resolvedSnapshotJSON)
			if err != nil {
				return domain.Task{}, err
			}
			execution.Resolved = &resolved
			runtimeErr := q.QueryRow(ctx, `SELECT r.host_boot_id,r.attempt_id,r.expected_container_name,COALESCE(r.container_id,''),r.execution_identity_digest,r.status,r.owner_epoch,COALESCE(to_char(r.execution_lease_expires_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.container_created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.supervisor_ready_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.run_permit_issued_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.harness_started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(a.delivery_acked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),ta.status,ta.attempt_no,COALESCE(r.recovery_previous_status,''),COALESCE(to_char(r.recovery_started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.reattach_deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(r.pending_worker_instance_id,''),COALESCE(r.pending_owner_epoch,0),COALESCE(to_char(r.reattached_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(r.watchdog_deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(r.terminal_reason_code,'') FROM workforce_attempt_runtimes r JOIN workforce_task_assignments a ON a.id=r.assignment_id JOIN workforce_task_attempts ta ON ta.id=r.attempt_id WHERE r.assignment_id=$1`, execution.AssignmentID).Scan(&execution.HostBootID, &execution.AttemptID, &execution.ContainerName, &execution.ContainerID, &execution.ExecutionIdentityDigest, &execution.RuntimeStatus, &execution.OwnerEpoch, &execution.ExecutionLeaseExpiresAt, &execution.ContainerCreatedAt, &execution.SupervisorReadyAt, &execution.RunPermitIssuedAt, &execution.HarnessStartedAt, &execution.DeliveryAckedAt, &execution.AttemptStatus, &execution.AttemptNumber, &execution.RecoveryPreviousStatus, &execution.RecoveryStartedAt, &execution.ReattachDeadline, &execution.PendingWorkerInstanceID, &execution.PendingOwnerEpoch, &execution.ReattachedAt, &execution.WatchdogDeadline, &execution.TerminalReasonCode)
			if runtimeErr != nil && !errors.Is(runtimeErr, pgx.ErrNoRows) {
				return domain.Task{}, runtimeErr
			}
		}
		task.Execution = &execution
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Task{}, err
	}
	return task, nil
}

// resolvedHookViews reads only the Hook identities needed by the Task detail
// projection. Historical Assignment snapshots can contain retired business
// fields whose current strict domain decoders must reject on executable paths;
// those unrelated fields must not make an otherwise valid historical Task
// unreadable.
func resolvedHookViews(snapshotJSON []byte) ([]domain.ResolvedHookView, error) {
	var snapshot struct {
		PackageReleases []domain.HookPackageReleaseSnapshot `json:"packageReleases"`
	}
	if err := json.Unmarshal(snapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("invalid resolved execution snapshot: %w", err)
	}
	hooks := make([]domain.ResolvedHookView, 0, len(snapshot.PackageReleases))
	for _, hook := range snapshot.PackageReleases {
		hooks = append(hooks, domain.ResolvedHookView{ReleaseID: hook.ID, HookID: hook.PackageID, Name: hook.Name, Version: hook.Version})
	}
	return hooks, nil
}

func (s *Store) List(ctx context.Context, limit int) ([]domain.Task, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
SELECT id,tenant_id,name,tags_json,created_by,metadata_version,repository_url,repository_ref,COALESCE(resolved_base_commit,''),task_instruction,
validation_commands_json,'null'::jsonb,COALESCE(result_processor_release_id,0),COALESCE(result_processor_parameters_json,'{}'::jsonb),status,priority,COALESCE(result_summary,''),
COALESCE(latest_attempt_id,0),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),version,
COALESCE(execution_timeout_seconds,0),
COALESCE(project_id,0),COALESCE(project_version_id,0),COALESCE(selected_runtime_release_id,0),review_enabled,model_credential_id,primary_model,COALESCE((SELECT name FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE((SELECT protocol FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE(execution_requirements_digest,'')
FROM workforce_tasks ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	items, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTaskInputSummaries(ctx, items)
}

func (s *Store) ListTenant(ctx context.Context, tenantID int64, limit int) ([]domain.Task, error) {
	if tenantID <= 0 {
		return []domain.Task{}, nil
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
SELECT id,tenant_id,name,tags_json,created_by,metadata_version,repository_url,repository_ref,COALESCE(resolved_base_commit,''),task_instruction,
validation_commands_json,'null'::jsonb,COALESCE(result_processor_release_id,0),COALESCE(result_processor_parameters_json,'{}'::jsonb),status,priority,COALESCE(result_summary,''),
COALESCE(latest_attempt_id,0),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),version,
COALESCE(execution_timeout_seconds,0),
COALESCE(project_id,0),COALESCE(project_version_id,0),COALESCE(selected_runtime_release_id,0),review_enabled,model_credential_id,primary_model,COALESCE((SELECT name FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE((SELECT protocol FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE(execution_requirements_digest,'')
		FROM workforce_tasks WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	items, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTaskInputSummaries(ctx, items)
}

func (s *Store) ListProject(ctx context.Context, tenantID, projectID int64, limit int) ([]domain.Task, error) {
	if tenantID <= 0 || projectID <= 0 {
		return []domain.Task{}, nil
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
SELECT id,tenant_id,name,tags_json,created_by,metadata_version,repository_url,repository_ref,COALESCE(resolved_base_commit,''),task_instruction,
validation_commands_json,'null'::jsonb,COALESCE(result_processor_release_id,0),COALESCE(result_processor_parameters_json,'{}'::jsonb),status,priority,COALESCE(result_summary,''),
COALESCE(latest_attempt_id,0),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),version,
COALESCE(execution_timeout_seconds,0),
COALESCE(project_id,0),COALESCE(project_version_id,0),COALESCE(selected_runtime_release_id,0),review_enabled,model_credential_id,primary_model,COALESCE((SELECT name FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE((SELECT protocol FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE(execution_requirements_digest,'')
FROM workforce_tasks WHERE tenant_id=$1 AND project_id=$2 ORDER BY created_at DESC LIMIT $3`, tenantID, projectID, limit)
	if err != nil {
		return nil, err
	}
	items, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}
	return s.hydrateTaskInputSummaries(ctx, items)
}

func (s *Store) ListPage(ctx context.Context, filter ListFilter) (TaskPage, error) {
	if filter.TenantID < 1 {
		return TaskPage{Items: []domain.Task{}}, nil
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 30
	}
	args := []any{filter.TenantID}
	where := []string{"tenant_id=$1"}
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if filter.ProjectID > 0 {
		where = append(where, "project_id="+add(filter.ProjectID))
	}
	if value := strings.TrimSpace(filter.Query); value != "" {
		p := add("%" + value + "%")
		where = append(where, "(name ILIKE "+p+" OR task_instruction ILIKE "+p+")")
	}
	if filter.From != nil {
		where = append(where, "created_at >= "+add(*filter.From))
	}
	if filter.To != nil {
		where = append(where, "created_at < "+add(*filter.To))
	}
	if filter.Cursor > 0 {
		p := add(filter.Cursor)
		where = append(where, "(created_at,id) < (SELECT created_at,id FROM workforce_tasks WHERE tenant_id=$1 AND id="+p+")")
	}
	switch filter.View {
	case "", "all":
	case "active":
		where = append(where, "status NOT IN ('SUCCEEDED','FAILED','TIMED_OUT','CANCELLED')")
	case "succeeded":
		where = append(where, "status='SUCCEEDED'")
	case "failed":
		where = append(where, "status IN ('FAILED','TIMED_OUT')")
	case "cancelled":
		where = append(where, "status='CANCELLED'")
	case "waiting":
		where = append(where, `(status IN ('CREATED','QUEUED','PREPARING','INTERACTION_SUSPENDED','RETRY_WAIT','MANUAL_REVIEW','REVIEWING') OR EXISTS(SELECT 1 FROM workforce_execution_nodes n WHERE n.task_id=workforce_tasks.id AND n.wait_reason_code<>''))`)
	case "attention":
		where = append(where, `(status IN ('INTERACTION_SUSPENDED','MANUAL_REVIEW','FAILED','TIMED_OUT') OR EXISTS(SELECT 1 FROM workforce_agent_interactions i WHERE i.tenant_id=workforce_tasks.tenant_id AND i.task_id=workforce_tasks.id AND i.kind='QUESTION' AND i.is_blocking AND i.state='OPEN' AND i.expires_at>CURRENT_TIMESTAMP) OR (SELECT d.status FROM workforce_delivery_requests d WHERE d.tenant_id=workforce_tasks.tenant_id AND d.task_id=workforce_tasks.id ORDER BY d.created_at DESC,d.id DESC LIMIT 1) IN ('WAITING_APPROVAL','FAILED','MANUAL_REVIEW'))`)
	default:
		return TaskPage{}, errors.New("TASK_LIST_VIEW_INVALID")
	}
	limitParam := add(filter.Limit + 1)
	query := `SELECT id,tenant_id,name,tags_json,created_by,metadata_version,repository_url,repository_ref,COALESCE(resolved_base_commit,''),task_instruction,
validation_commands_json,'null'::jsonb,COALESCE(result_processor_release_id,0),COALESCE(result_processor_parameters_json,'{}'::jsonb),status,priority,COALESCE(result_summary,''),
COALESCE(latest_attempt_id,0),to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),version,COALESCE(execution_timeout_seconds,0),
COALESCE(project_id,0),COALESCE(project_version_id,0),COALESCE(selected_runtime_release_id,0),review_enabled,model_credential_id,primary_model,COALESCE((SELECT name FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE((SELECT protocol FROM workforce_model_credentials mc WHERE mc.id=workforce_tasks.model_credential_id AND mc.tenant_id=workforce_tasks.tenant_id),''),COALESCE(execution_requirements_digest,'')
FROM workforce_tasks WHERE ` + strings.Join(where, " AND ") + ` ORDER BY created_at DESC,id DESC LIMIT ` + limitParam
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return TaskPage{}, err
	}
	items, err := scanTasks(rows)
	if err != nil {
		return TaskPage{}, err
	}
	next := int64(0)
	if len(items) > filter.Limit {
		next = items[filter.Limit-1].ID
		items = items[:filter.Limit]
	}
	items, err = s.hydrateTaskInputSummaries(ctx, items)
	if err != nil {
		return TaskPage{}, err
	}
	return TaskPage{Items: items, NextCursor: next}, nil
}

func (s *Store) hydrateTaskInputSummaries(ctx context.Context, tasks []domain.Task) ([]domain.Task, error) {
	if len(tasks) == 0 {
		return tasks, nil
	}
	ids := make([]int64, 0, len(tasks))
	index := make(map[int64]int, len(tasks))
	for i := range tasks {
		ids = append(ids, tasks[i].ID)
		index[tasks[i].ID] = i
	}
	rows, err := s.db.Query(ctx, `
SELECT current_input.task_id,current_input.source_kind,COUNT(r.reference_asset_id)
FROM (
  SELECT DISTINCT ON (task_id) id,task_id,source_kind
  FROM workforce_task_input_snapshots
  WHERE task_id=ANY($1)
  ORDER BY task_id,created_at DESC,id DESC
) current_input
LEFT JOIN workforce_task_input_references r ON r.task_input_snapshot_id=current_input.id
GROUP BY current_input.task_id,current_input.source_kind`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var kind string
		var count int
		if err = rows.Scan(&id, &kind, &count); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			tasks[i].InputKind, tasks[i].ReferenceCount = kind, count
		}
	}
	return tasks, rows.Err()
}

func (s *Store) Backlog(ctx context.Context, tenantID int64) (domain.BacklogSummary, error) {
	var result domain.BacklogSummary
	var oldestSeconds int64
	if err := s.db.QueryRow(ctx, `
SELECT COUNT(*),COALESCE(EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP-MIN(t.created_at)))::bigint,0)
FROM workforce_tasks t
JOIN workforce_execution_nodes n ON n.task_id=t.id
WHERE t.tenant_id=$1 AND t.status='QUEUED' AND n.status='QUEUED'`, tenantID).Scan(&result.QueuedTasks, &oldestSeconds); err != nil {
		return result, err
	}
	result.OldestWaitSeconds = oldestSeconds
	rows, err := s.db.Query(ctx, `
SELECT COALESCE(NULLIF(n.wait_reason_code,''),'ASSIGNMENT_RETRY_PENDING'),COUNT(*)
FROM workforce_tasks t
JOIN workforce_execution_nodes n ON n.task_id=t.id
WHERE t.tenant_id=$1 AND t.status='QUEUED' AND n.status='QUEUED'
GROUP BY COALESCE(NULLIF(n.wait_reason_code,''),'ASSIGNMENT_RETRY_PENDING')
ORDER BY COUNT(*) DESC,1`, tenantID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.BacklogReasonCount
		if err := rows.Scan(&item.ReasonCode, &item.Count); err != nil {
			return result, err
		}
		result.Reasons = append(result.Reasons, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	// Keep the operator summary bounded as the fleet grows. These are three
	// fixed-width batch reads (session, inventory and capacity), not per-Worker
	// probes, and each stays inside the Framework's logical-read width budget.
	type candidate struct {
		id       int64
		instance string
		max      int
	}
	workerRows, err := s.db.Query(ctx, `
SELECT w.id,w.current_instance_id,LEAST(w.max_active_attempts,s.max_active_attempts)
FROM workforce_workers w
JOIN workforce_worker_sessions s
  ON s.instance_id=w.current_instance_id AND s.worker_id=w.id
WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL
  AND s.ended_at IS NULL
  AND s.last_heartbeat_at>CURRENT_TIMESTAMP-INTERVAL '35 seconds'
  AND s.docker_ready=TRUE AND s.consumer_state='CONSUMING'
  AND COALESCE((s.resource_snapshot_json->>'workspaceDiskAvailableBytes')::bigint,0)>=5368709120
  AND COALESCE((s.resource_snapshot_json->>'workspaceDiskUsagePercent')::numeric,100)<95`)
	if err != nil {
		return result, err
	}
	var candidates []candidate
	var workerIDs []int64
	for workerRows.Next() {
		var item candidate
		if err := workerRows.Scan(&item.id, &item.instance, &item.max); err != nil {
			workerRows.Close()
			return result, err
		}
		candidates = append(candidates, item)
		workerIDs = append(workerIDs, item.id)
	}
	if err := workerRows.Err(); err != nil {
		workerRows.Close()
		return result, err
	}
	workerRows.Close()
	if len(candidates) == 0 {
		return result, nil
	}

	admitted := make(map[string]bool, len(candidates))
	imageRows, err := s.db.Query(ctx, `
SELECT worker_id,instance_id
FROM workforce_worker_images
WHERE worker_id=ANY($1) AND role='attempt'
  AND admission_status='ADMITTED' AND presence_status='PRESENT'
GROUP BY worker_id,instance_id`, workerIDs)
	if err != nil {
		return result, err
	}
	for imageRows.Next() {
		var workerID int64
		var instanceID string
		if err := imageRows.Scan(&workerID, &instanceID); err != nil {
			imageRows.Close()
			return result, err
		}
		admitted[fmt.Sprintf("%d\x00%s", workerID, instanceID)] = true
	}
	if err := imageRows.Err(); err != nil {
		imageRows.Close()
		return result, err
	}
	imageRows.Close()

	active := make(map[int64]int, len(candidates))
	assignmentRows, err := s.db.Query(ctx, `
SELECT worker_id,COUNT(*)
FROM workforce_task_assignments
WHERE worker_id=ANY($1) AND status IN ('RESERVED','PUBLISHED','CLAIMED')
GROUP BY worker_id`, workerIDs)
	if err != nil {
		return result, err
	}
	for assignmentRows.Next() {
		var workerID int64
		var count int
		if err := assignmentRows.Scan(&workerID, &count); err != nil {
			assignmentRows.Close()
			return result, err
		}
		active[workerID] = count
	}
	if err := assignmentRows.Err(); err != nil {
		assignmentRows.Close()
		return result, err
	}
	assignmentRows.Close()

	for _, worker := range candidates {
		if !admitted[fmt.Sprintf("%d\x00%s", worker.id, worker.instance)] {
			continue
		}
		result.CompatibleWorkers++
		if active[worker.id] < worker.max {
			result.AvailableWorkers++
		}
	}
	return result, nil
}

func scanTasks(rows pgx.Rows) ([]domain.Task, error) {
	defer rows.Close()
	var result []domain.Task
	for rows.Next() {
		var task domain.Task
		var commands, tagsJSON, snapshotJSON, processorParameters []byte
		if err := rows.Scan(
			&task.ID, &task.TenantID, &task.Name, &tagsJSON, &task.CreatedBy, &task.MetadataVersion, &task.RepositoryURL, &task.RepositoryRef, &task.ResolvedBaseCommit,
			&task.Instruction, &commands, &snapshotJSON, &task.ResultProcessorReleaseID, &processorParameters, &task.Status, &task.Priority,
			&task.ResultSummary, &task.LatestAttemptID, &task.CreatedAt, &task.StartedAt, &task.FinishedAt, &task.Version, &task.HarnessTimeoutSeconds,
			&task.ProjectID, &task.ProjectVersionID, &task.SelectedRuntimeReleaseID, &task.ReviewEnabled, &task.ModelCredentialID, &task.PrimaryModel, &task.ModelCredentialName, &task.ModelProtocol, &task.ExecutionRequirementsDigest,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(commands, &task.ValidationCommands)
		_ = json.Unmarshal(tagsJSON, &task.Tags)
		task.ResultProcessorParameters = append(json.RawMessage(nil), processorParameters...)
		if string(snapshotJSON) != "null" {
			var snapshot domain.ExecutionSnapshot
			if json.Unmarshal(snapshotJSON, &snapshot) == nil {
				task.ExecutionSnapshot = &snapshot
			}
		}
		task.ModelSelection = &domain.ModelSelectionView{CredentialID: task.ModelCredentialID, CredentialName: task.ModelCredentialName, Protocol: task.ModelProtocol, PrimaryModel: task.PrimaryModel}
		result = append(result, task)
	}
	return result, rows.Err()
}

func (s *Store) UpdateMetadata(ctx context.Context, tenantID int64, taskID int64, actor int64, name string, tags []string, expectedVersion int64) (domain.Task, error) {
	tagsJSON, _ := json.Marshal(tags)
	command, err := s.db.Exec(ctx, `UPDATE workforce_tasks SET name=$4,tags_json=$5,metadata_version=metadata_version+1 WHERE id=$1 AND tenant_id=$2 AND metadata_version=$3`, taskID, tenantID, expectedVersion, name, tagsJSON)
	if err != nil {
		return domain.Task{}, err
	}
	if command.RowsAffected() == 0 {
		var exists bool
		if err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_tasks WHERE id=$1 AND tenant_id=$2)`, taskID, tenantID).Scan(&exists); err != nil {
			return domain.Task{}, err
		}
		if !exists {
			return domain.Task{}, ErrNotFound
		}
		return domain.Task{}, domain.ErrTaskMetadataConflict
	}
	_ = actor
	return s.Get(ctx, taskID)
}

type CancelResult struct {
	CancelRequestID int64  `json:"cancelRequestId,string"`
	PreviousStatus  string `json:"previousStatus"`
	CurrentStatus   string `json:"currentStatus"`
	EffectiveStage  string `json:"effectiveStage"`
	TenantID        int64  `json:"-"`
	AttemptID       int64  `json:"-"`
}

// The status parameter also participates in a CASE comparison. Give it one
// explicit SQL type so PostgreSQL does not infer TEXT there and VARCHAR from
// the migrated status column.
const cancelTaskUpdateSQL = `UPDATE workforce_tasks SET status=$2::VARCHAR,version=version+1,finished_at=CASE WHEN $2::VARCHAR='CANCELLED' THEN CURRENT_TIMESTAMP ELSE finished_at END,cancel_request_id=$3,cancel_generation=cancel_generation+1,cancel_requested_by_principal_id=$4,cancel_requested_at=CURRENT_TIMESTAMP,cancel_reason=$5 WHERE id=$1`

func (s *Store) Cancel(ctx context.Context, taskID int64, actor int64, reason string) (CancelResult, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CancelResult{}, err
	}
	defer tx.Rollback(ctx)
	var status domain.TaskStatus
	var tenantID, attemptID, turnID, creatorID int64
	if err := tx.QueryRow(ctx, `SELECT status,tenant_id,COALESCE(latest_attempt_id,0),COALESCE(current_task_turn_id,0),created_by FROM workforce_tasks WHERE id=$1 FOR UPDATE`, taskID).Scan(&status, &tenantID, &attemptID, &turnID, &creatorID); errors.Is(err, pgx.ErrNoRows) {
		return CancelResult{}, ErrNotFound
	} else if err != nil {
		return CancelResult{}, err
	}
	manualIndependentReview := false
	if status == domain.TaskManualReview {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_review_runs WHERE task_id=$1 AND task_turn_id=$2 AND status IN ('MANUAL_REVIEW','REJECTED'))`, taskID, turnID).Scan(&manualIndependentReview); err != nil {
			return CancelResult{}, err
		}
	}
	if status.Terminal() && !manualIndependentReview {
		return CancelResult{PreviousStatus: string(status), CurrentStatus: string(status), EffectiveStage: string(status), TenantID: tenantID, AttemptID: attemptID}, tx.Commit(ctx)
	}
	if s.interactions != nil && attemptID > 0 {
		if err = s.interactions.CloseAttemptTx(ctx, tx, tenantID, taskID, attemptID, "CANCELLED"); err != nil {
			return CancelResult{}, err
		}
	}
	if s.executionAccess != nil {
		if err = s.executionAccess.CancelTaskTx(ctx, tx, tenantID, taskID, time.Now().UTC()); err != nil {
			return CancelResult{}, err
		}
	}
	if status == domain.TaskCancelling {
		var cancelID, cancelPrincipalID, cancelUserID int64
		var cancelPrincipalType string
		if err := tx.QueryRow(ctx, `SELECT COALESCE(t.cancel_request_id,0),COALESCE(t.cancel_requested_by_principal_id,0),COALESCE(p.user_id,0),COALESCE(p.principal_type,'SERVICE') FROM workforce_tasks t LEFT JOIN workforce_principals p ON p.id=t.cancel_requested_by_principal_id WHERE t.id=$1`, taskID).Scan(&cancelID, &cancelPrincipalID, &cancelUserID, &cancelPrincipalType); err != nil {
			return CancelResult{}, err
		}
		if attemptID != 0 {
			var nodeID, assignmentID int64
			var attemptStatus string
			err = tx.QueryRow(ctx, `SELECT execution_node_id,assignment_id,status FROM workforce_task_attempts WHERE id=$1 AND task_id=$2 FOR UPDATE`, attemptID, taskID).Scan(&nodeID, &assignmentID, &attemptStatus)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return CancelResult{}, err
			}
			if err == nil {
				var runtimeExists bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workforce_attempt_runtimes WHERE attempt_id=$1)`, attemptID).Scan(&runtimeExists); err != nil {
					return CancelResult{}, err
				}
				if canCancelWithoutRuntime(string(status), attemptStatus, runtimeExists) {
					_, _ = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',lease_expires_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,error_code='USER_CANCELLED' WHERE id=$1 AND status='PREPARING'`, attemptID)
					_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, nodeID)
					_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code='USER_CANCELLED',delivery_disposition='CANCELLED_BEFORE_RUNTIME' WHERE id=$1 AND status='CLAIMED'`, assignmentID)
					_, _ = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=(SELECT current_task_turn_id FROM workforce_tasks WHERE id=$1) AND status='ACTIVE'`, taskID)
					if _, err = tx.Exec(ctx, `UPDATE workforce_tasks SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND status='CANCELLING'`, taskID); err != nil {
						return CancelResult{}, err
					}
					occurrence, appendErr := eventlog.AppendWithSequence(ctx, tx, taskID, attemptID, "task.cancelled", map[string]any{"from": status, "stage": "BEFORE_RUNTIME"})
					if appendErr != nil {
						return CancelResult{}, appendErr
					}
					if cancelUserID != creatorID && s.notificationRecorder != nil && turnID > 0 {
						reasonCode := "TASK_CANCELLED_BY_OPERATOR"
						if cancelPrincipalType != "USER" {
							reasonCode = "TASK_CANCELLED_BY_SYSTEM"
						}
						if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotificationfacade.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotificationfacade.SourceTaskCancel, SourceID: cancelID, SourceOccurrence: occurrence, PreviousStatus: string(status), CurrentStatus: string(domain.TaskCancelled), ReasonCode: reasonCode, ActorPrincipalID: cancelPrincipalID, Recoverability: tasknotificationfacade.RecoverabilityTerminal, OccurredAt: time.Now().UTC()}); err != nil {
							return CancelResult{}, err
						}
					}
					return CancelResult{CancelRequestID: cancelID, PreviousStatus: string(status), CurrentStatus: string(domain.TaskCancelled), EffectiveStage: "PREPARING", TenantID: tenantID, AttemptID: attemptID}, tx.Commit(ctx)
				}
			}
		}
		if s.reviewRuns != nil {
			if err := s.reviewRuns.CancelForTask(ctx, tx, taskID, "TASK_CANCELLED"); err != nil {
				return CancelResult{}, err
			}
		}
		return CancelResult{CancelRequestID: cancelID, PreviousStatus: string(status), CurrentStatus: string(status), EffectiveStage: string(status), TenantID: tenantID, AttemptID: attemptID}, tx.Commit(ctx)
	}
	next := domain.TaskCancelling
	if status == domain.TaskCreated || status == domain.TaskQueued || status == domain.TaskInteractionSuspended {
		next = domain.TaskCancelled
	}
	if status == domain.TaskReviewing {
		// An independent ReviewRun has no Agent runtime to await. Its own facade
		// fences the review lease in this transaction, so cancellation can reach
		// a terminal Task state immediately.
		next = domain.TaskCancelled
	}
	if manualIndependentReview {
		next = domain.TaskCancelled
	}
	var preRuntimeAttemptID, preRuntimeNodeID, preRuntimeAssignmentID int64
	if status == domain.TaskPreparing && attemptID != 0 {
		var attemptStatus string
		err = tx.QueryRow(ctx, `SELECT id,execution_node_id,assignment_id,status FROM workforce_task_attempts WHERE id=$1 AND task_id=$2 FOR UPDATE`, attemptID, taskID).Scan(&preRuntimeAttemptID, &preRuntimeNodeID, &preRuntimeAssignmentID, &attemptStatus)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return CancelResult{}, err
		}
		if err == nil {
			var runtimeExists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workforce_attempt_runtimes WHERE attempt_id=$1)`, attemptID).Scan(&runtimeExists); err != nil {
				return CancelResult{}, err
			}
			if canCancelWithoutRuntime(string(status), attemptStatus, runtimeExists) {
				next = domain.TaskCancelled
			}
		}
	}
	if !status.CanTransition(next) && !(status == domain.TaskPreparing && next == domain.TaskCancelled) && !manualIndependentReview {
		return CancelResult{}, ErrInvalidTransition
	}
	cancelID := s.idGen.NextID()
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return CancelResult{}, err
	}
	_, err = tx.Exec(ctx, cancelTaskUpdateSQL, taskID, next, cancelID, actorPrincipalID, reason)
	if err != nil {
		return CancelResult{}, err
	}
	if s.reviewRuns != nil {
		if err := s.reviewRuns.CancelForTask(ctx, tx, taskID, "TASK_CANCELLED"); err != nil {
			return CancelResult{}, err
		}
	}
	if s.deliveryLifecycle != nil {
		if err := s.deliveryLifecycle.CancelForTask(ctx, tx, taskID, "TASK_CANCELLED"); err != nil {
			return CancelResult{}, err
		}
	}
	if next == domain.TaskCancelled {
		if status == domain.TaskInteractionSuspended {
			for _, query := range []string{
				`UPDATE workforce_task_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status='INTERACTION_SUSPENDED'`,
				`UPDATE workforce_attempt_runtimes SET status='CANCELLED',terminal_reason_code='USER_CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE attempt_id IN(SELECT id FROM workforce_task_attempts WHERE task_id=$1) AND status='INTERACTION_SUSPENDED'`,
				`UPDATE workforce_task_turns SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status='INTERACTION_SUSPENDED'`,
				`UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status='INTERACTION_SUSPENDED'`,
			} {
				if _, err = tx.Exec(ctx, query, taskID); err != nil {
					return CancelResult{}, err
				}
			}
		}
		if preRuntimeAttemptID != 0 {
			_, _ = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',lease_expires_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,error_code='USER_CANCELLED' WHERE id=$1 AND status='PREPARING'`, preRuntimeAttemptID)
			_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='PREPARING'`, preRuntimeNodeID)
			_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='COMPLETED',completed_at=CURRENT_TIMESTAMP,release_reason_code='USER_CANCELLED',delivery_disposition='CANCELLED_BEFORE_RUNTIME' WHERE id=$1 AND status='CLAIMED'`, preRuntimeAssignmentID)
		}
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',lease_expires_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,error_code='USER_CANCELLED' WHERE task_id=$1 AND status IN ('REVIEWING','MANUAL_REVIEW')`, taskID)
		_, _ = tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status IN ('QUEUED','PREPARING','REVIEWING','MANUAL_REVIEW')`, taskID)
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=(SELECT current_task_turn_id FROM workforce_tasks WHERE id=$1) AND status IN ('ACTIVE','REVIEWING','MANUAL_REVIEW')`, taskID)
		_, _ = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='CANCELLED',completed_at=CURRENT_TIMESTAMP,release_reason_code='USER_CANCELLED' WHERE task_id=$1 AND status IN ('RESERVED','PUBLISHED')`, taskID)
		_, _ = tx.Exec(ctx, `UPDATE workforce_outbox_events SET status='SUPERSEDED',last_error='USER_CANCELLED' WHERE aggregate_type='assignment' AND aggregate_id IN (SELECT id FROM workforce_task_assignments WHERE task_id=$1) AND status='PENDING'`, taskID)
	}
	_, _ = tx.Exec(ctx, `UPDATE workforce_finalization_runs SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status IN ('PENDING','RUNNING','RETRY_WAIT')`, taskID)
	_, _ = tx.Exec(ctx, `UPDATE workforce_artifact_upload_attempts SET status='CANCELLED',updated_at=CURRENT_TIMESTAMP WHERE task_id=$1 AND status IN ('PENDING','UPLOADING','VERIFYING','RETRY_WAIT')`, taskID)
	eventType := "task.cancel_requested"
	if next == domain.TaskCancelled {
		eventType = "task.cancelled"
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, 0, eventType, map[string]any{"from": status, "actorUserId": actor})
	if err != nil {
		return CancelResult{}, err
	}
	if next == domain.TaskCancelled && actor != creatorID && s.notificationRecorder != nil && turnID > 0 {
		if _, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotificationfacade.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotificationfacade.SourceTaskCancel, SourceID: cancelID, SourceOccurrence: occurrence, PreviousStatus: string(status), CurrentStatus: string(next), ReasonCode: "TASK_CANCELLED_BY_OPERATOR", ActorPrincipalID: actorPrincipalID, Recoverability: tasknotificationfacade.RecoverabilityTerminal, OccurredAt: time.Now().UTC()}); err != nil {
			return CancelResult{}, err
		}
	}
	return CancelResult{CancelRequestID: cancelID, PreviousStatus: string(status), CurrentStatus: string(next), EffectiveStage: string(status), TenantID: tenantID, AttemptID: attemptID}, tx.Commit(ctx)
}

func canCancelWithoutRuntime(taskStatus, attemptStatus string, runtimeExists bool) bool {
	return (taskStatus == "PREPARING" || taskStatus == "CANCELLING") && attemptStatus == "PREPARING" && !runtimeExists
}

func (s *Store) Events(ctx context.Context, taskID int64, afterID int64, limit int) ([]domain.Event, error) {
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.Query(ctx, `
SELECT id,task_id,COALESCE(attempt_id,0),event_type,sequence,payload_json,
to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM workforce_task_events WHERE task_id=$1 AND id>$2 ORDER BY id LIMIT $3`, taskID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		if err := rows.Scan(&event.ID, &event.TaskID, &event.AttemptID, &event.Type, &event.Sequence, &event.Payload, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) Artifacts(ctx context.Context, taskID int64) ([]domain.Artifact, error) {
	rows, err := s.db.Query(ctx, `
SELECT id,task_id,attempt_id,artifact_type,name,object_key,sha256,size_bytes,media_type,status,metadata_json,
to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_artifacts WHERE task_id=$1 ORDER BY created_at`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.Artifact
	for rows.Next() {
		var item domain.Artifact
		if err := rows.Scan(&item.ID, &item.TaskID, &item.AttemptID, &item.Type, &item.Name, &item.ObjectKey, &item.SHA256, &item.SizeBytes, &item.MediaType, &item.Status, &item.Metadata, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) Artifact(ctx context.Context, taskID int64, artifactID int64) (domain.Artifact, error) {
	var item domain.Artifact
	err := s.db.QueryRow(ctx, `SELECT id,task_id,attempt_id,artifact_type,name,object_key,sha256,size_bytes,media_type,status,metadata_json,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_artifacts WHERE task_id=$1 AND id=$2`, taskID, artifactID).Scan(&item.ID, &item.TaskID, &item.AttemptID, &item.Type, &item.Name, &item.ObjectKey, &item.SHA256, &item.SizeBytes, &item.MediaType, &item.Status, &item.Metadata, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Artifact{}, ErrNotFound
	}
	return item, err
}

type OutboxEvent struct {
	ID           int64
	TaskID       int64
	Payload      json.RawMessage
	Attempts     int
	Exchange     string
	RoutingKey   string
	ExpirationMS int64
}

func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	// Task notifications share this transactional outbox table, but they are
	// delivered directly through NotificationClient by an independent publisher.
	// Excluding that aggregate here prevents a Notification Center outage from
	// blocking Assignment and post-process RabbitMQ messages in this ordered
	// batch publisher.
	rows, err := s.db.Query(ctx, `SELECT id,aggregate_id,payload_json,attempts,COALESCE(exchange_name,''),COALESCE(routing_key,''),COALESCE(message_expiration_ms,0) FROM workforce_outbox_events WHERE status='PENDING' AND aggregate_type<>'task_notification' AND (next_retry_at IS NULL OR next_retry_at<=CURRENT_TIMESTAMP) ORDER BY created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []OutboxEvent
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(&event.ID, &event.TaskID, &event.Payload, &event.Attempts, &event.Exchange, &event.RoutingKey, &event.ExpirationMS); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Store) MarkOutboxPublished(ctx context.Context, id int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var aggregateType, aggregateID string
	if err = tx.QueryRow(ctx, `UPDATE workforce_outbox_events SET status='PUBLISHED',published_at=CURRENT_TIMESTAMP,last_error=NULL WHERE id=$1 AND status='PENDING' RETURNING aggregate_type,aggregate_id`, id).Scan(&aggregateType, &aggregateID); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	if aggregateType == "assignment" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_assignments SET status='PUBLISHED',published_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='RESERVED'`, aggregateID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkOutboxFailed(ctx context.Context, id int64, publishErr error) error {
	message := publishErr.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	_, err := s.db.Exec(ctx, `UPDATE workforce_outbox_events SET attempts=attempts+1,next_retry_at=CURRENT_TIMESTAMP+LEAST(INTERVAL '30 seconds', INTERVAL '1 second' * power(2,LEAST(attempts,5))),last_error=$2 WHERE id=$1 AND status='PENDING'`, id, message)
	return err
}

func (s *Store) Transition(ctx context.Context, taskID int64, attemptID int64, from, to domain.TaskStatus, eventType string, payload any) error {
	if !from.CanTransition(to) {
		return ErrInvalidTransition
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status=$3,version=version+1,finished_at=CASE WHEN $3 IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT') THEN CURRENT_TIMESTAMP ELSE finished_at END,verified_at=CASE WHEN $3='SUCCEEDED' THEN CURRENT_TIMESTAMP ELSE verified_at END WHERE id=$1 AND status=$2`, taskID, from, to)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	if to == domain.TaskSucceeded || to == domain.TaskFailed || to == domain.TaskCancelled || to == domain.TaskTimedOut {
		if err := s.closeExecutionAccessTx(ctx, tx, taskID); err != nil {
			return err
		}
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, eventType, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetCloneResult(ctx context.Context, taskID int64, attemptID int64, baseCommit string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE workforce_tasks SET resolved_base_commit=$2 WHERE id=$1`, taskID, baseCommit); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET base_commit=$2 WHERE id=$1`, attemptID, baseCommit); err != nil {
		return err
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, "workspace.ready", map[string]any{"baseCommit": baseCommit}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetContainer(ctx context.Context, taskID int64, attemptID int64, containerID string, imageDigest string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET container_id=$2,status='RUNNING',runtime_image_digest=$3,image_contract_version='seven.worker-image/v1',supervisor_protocol_version=1,last_heartbeat_at=CURRENT_TIMESTAMP,last_progress_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID, containerID, imageDigest); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='RUNNING',version=version+1 WHERE id=$1 AND status='PREPARING'`, taskID); err != nil {
		return err
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, "attempt.running", map[string]any{"containerId": containerID, "imageDigest": imageDigest}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) AddEvent(ctx context.Context, taskID int64, attemptID int64, eventType string, payload any) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := appendEventTx(ctx, tx, taskID, attemptID, eventType, payload); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func appendEventTx(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, eventType string, payload any) error {
	return eventlog.Append(ctx, tx, taskID, attemptID, eventType, payload)
}

func RequestHash(value any) (string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:]), nil
}

func (s *Store) IsCancelling(ctx context.Context, taskID int64) (bool, error) {
	var status domain.TaskStatus
	if err := s.db.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1`, taskID).Scan(&status); err != nil {
		return false, err
	}
	return status == domain.TaskCancelling, nil
}

func (s *Store) Fail(ctx context.Context, taskID int64, attemptID int64, code string, cause error) error {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, _ = tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED',finished_at=CURRENT_TIMESTAMP,error_code=$2,error_message=$3 WHERE id=$1`, attemptID, code, message)
	_, _ = tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary=$2,version=version+1 WHERE id=$1 AND status NOT IN ('SUCCEEDED','CANCELLED','FAILED')`, taskID, code)
	_, _ = tx.Exec(ctx, `UPDATE workforce_task_turns SET status='FAILED',finished_at=CURRENT_TIMESTAMP WHERE id=(SELECT current_task_turn_id FROM workforce_tasks WHERE id=$1) AND status='ACTIVE'`, taskID)
	if err := s.closeExecutionAccessTx(ctx, tx, taskID); err != nil {
		return err
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, "task.failed", map[string]any{"code": code, "message": message}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) CompleteCancelled(ctx context.Context, taskID int64, attemptID int64) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`, attemptID); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='CANCELLED',finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND status='CANCELLING'`, taskID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	if err := s.closeExecutionAccessTx(ctx, tx, taskID); err != nil {
		return err
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, "task.cancelled", map[string]any{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordCheck(ctx context.Context, taskID int64, attemptID int64, ordinal int, name, command string, exitCode int, output, status string) error {
	digest := sha256.Sum256([]byte(command))
	_, err := s.db.Exec(ctx, `INSERT INTO workforce_check_runs (id,task_id,attempt_id,ordinal,name,command_digest,status,exit_code,timeout_seconds,output,started_at,finished_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,120,$9,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, s.idGen.NextID(), taskID, attemptID, ordinal, name, hex.EncodeToString(digest[:]), status, exitCode, output)
	return err
}

type CheckEvidence struct {
	Ordinal        int    `json:"ordinal"`
	Name           string `json:"name"`
	CommandDigest  string `json:"commandDigest"`
	Status         string `json:"status"`
	ExitCode       int    `json:"exitCode"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Output         string `json:"output"`
}

func (s *Store) CheckEvidence(ctx context.Context, taskID int64, attemptID int64) ([]CheckEvidence, error) {
	rows, err := s.db.Query(ctx, `SELECT ordinal,name,command_digest,status,COALESCE(exit_code,-1),timeout_seconds,COALESCE(output,'') FROM workforce_check_runs WHERE task_id=$1 AND attempt_id=$2 ORDER BY ordinal`, taskID, attemptID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]CheckEvidence, 0)
	for rows.Next() {
		var item CheckEvidence
		if err := rows.Scan(&item.Ordinal, &item.Name, &item.CommandDigest, &item.Status, &item.ExitCode, &item.TimeoutSeconds, &item.Output); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RecordCompletion(ctx context.Context, taskID int64, attemptID int64, raw []byte) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	sessionID := s.idGen.NextID()
	if _, err := tx.Exec(ctx, `
INSERT INTO workforce_harness_sessions (id,task_id,attempt_id,provider,status,cli_version,adapter_version,stopped_at)
SELECT $1,$2,$3,r.harness_type,'COMPLETED',r.supervisor_version,r.harness_version,CURRENT_TIMESTAMP
FROM workforce_task_attempts a
JOIN workforce_harness_releases r ON r.id=COALESCE((a.resolved_execution_snapshot_json->>'harnessReleaseId')::BIGINT,0)
WHERE a.id=$3 AND a.task_id=$2 AND NOT EXISTS (SELECT 1 FROM workforce_completion_proposals p WHERE p.attempt_id=$3)`, sessionID, taskID, attemptID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO workforce_completion_proposals (id,task_id,attempt_id,harness_session_id,schema_version,raw_json,validation_status) VALUES ($1,$2,$3,$4,1,$5,$6) ON CONFLICT (attempt_id) DO NOTHING`, s.idGen.NextID(), taskID, attemptID, sessionID, raw, domain.CompletionProposalAccepted); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordArtifact(ctx context.Context, artifact domain.Artifact) error {
	_, err := s.db.Exec(ctx, `INSERT INTO workforce_artifacts (id,task_id,attempt_id,artifact_type,name,object_key,sha256,size_bytes,media_type,status,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (object_key) DO NOTHING`, artifact.ID, artifact.TaskID, artifact.AttemptID, artifact.Type, artifact.Name, artifact.ObjectKey, artifact.SHA256, artifact.SizeBytes, artifact.MediaType, artifact.Status, artifact.Metadata)
	return err
}

func (s *Store) FinishSuccess(ctx context.Context, taskID int64, attemptID int64, resultCommit string, resultTree string, summary string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='SUCCEEDED',result_commit=$2,result_tree=$3,finished_at=CURRENT_TIMESTAMP,exit_code=0 WHERE id=$1`, attemptID, resultCommit, resultTree); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='SUCCEEDED',result_summary=$2,verified_at=CURRENT_TIMESTAMP,finished_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1 AND status='UPLOADING'`, taskID, summary)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	if err := s.closeExecutionAccessTx(ctx, tx, taskID); err != nil {
		return err
	}
	if err := appendEventTx(ctx, tx, taskID, attemptID, "task.succeeded", map[string]any{"resultCommit": resultCommit, "resultTree": resultTree}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
