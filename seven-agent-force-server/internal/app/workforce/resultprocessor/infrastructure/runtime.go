package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

// EnvironmentValueResolver is deliberately small. The Workforce module wires
// it to Framework ConfigFacade; the Store never knows how a secret is stored
// or encrypted. It receives the exact ref/version frozen in TaskTurn #1.
type EnvironmentValueResolver interface {
	ResolveResultProcessorValue(ctx context.Context, tenantID int64, ref string, version string, phase string) (string, error)
}

// EnvironmentSecretBindingVerifier is implemented by the Framework Config
// adapter. It lets authoring fail early when a SECRET ref/version is not an
// approved, exact ConfigFacade value rather than deferring that discovery to a
// running Agent Attempt.
type EnvironmentSecretBindingVerifier interface {
	VerifyResultProcessorSecretRef(ctx context.Context, ref, version, phase string) error
}

// EnvironmentSecretRegistrar registers only the declared fully-qualified
// ConfigFacade key as an internal consumer. It never carries a secret value.
type EnvironmentSecretRegistrar interface {
	RegisterResultProcessorSecretRef(ctx context.Context, ref string) error
}

func (s *Store) SetEnvironmentValueResolver(resolver EnvironmentValueResolver) {
	if s != nil {
		s.environmentResolver = resolver
	}
}

// MaterializeEvaluationEnvironment resolves only the immutable bindings
// frozen in the current TaskTurn. It is invoked after the Agent process group
// has ended and returns values solely to the Worker Machine API response.
func (s *Store) MaterializeEvaluationEnvironment(ctx context.Context, identity facade.MachineIdentity, attemptID int64, leaseToken string, cycleID int64, cycleNumber int) (facade.Materialization, error) {
	if err := validRuntimeIdentity(identity, attemptID, leaseToken); err != nil {
		return facade.Materialization{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.Materialization{}, err
	}
	defer tx.Rollback(ctx)
	scope, err := s.loadEvaluationScope(ctx, tx, identity, attemptID, leaseToken)
	if err != nil {
		return facade.Materialization{}, err
	}
	if scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING" {
		return facade.Materialization{}, ErrConflict
	}
	if cycleID < 0 || cycleNumber < 1 || cycleNumber > scope.MaxCycles {
		return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_CYCLE_INVALID")
	}
	cycleID, err = s.authorizeCompletionCheckCycle(ctx, tx, scope, attemptID, cycleID, cycleNumber)
	if err != nil {
		return facade.Materialization{}, err
	}
	environment, err := decodeFrozenEnvironment(scope.Environment)
	if err != nil {
		return facade.Materialization{}, err
	}
	variables := make(map[string]string, len(environment.Bindings))
	redaction := make([]string, 0)
	for _, binding := range environment.Bindings {
		if binding.Phase != "EVALUATE" {
			return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_PHASE_INVALID")
		}
		if binding.NetworkPolicy != "" && binding.NetworkPolicy != "NONE" {
			// Capability-dependent target egress is intentionally not guessed by
			// the Server. An image/worker capable policy will be introduced with
			// the matching admission evidence before it can be selected here.
			return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_NETWORK_POLICY_UNAVAILABLE")
		}
		switch binding.Type {
		case "CONFIG":
			value, err := jsonValueToString(binding.Config)
			if err != nil {
				return facade.Materialization{}, fmt.Errorf("decode config binding %q: %w", binding.Name, err)
			}
			variables[binding.Name] = value
		case "SECRET":
			if s.environmentResolver == nil {
				return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_RESOLVER_UNAVAILABLE")
			}
			if !secretFrozen(scope.SecretVersions, binding.Name, binding.SecretRef, binding.SecretVersion) {
				return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_IDENTITY_DRIFT")
			}
			value, err := s.environmentResolver.ResolveResultProcessorValue(ctx, scope.TenantID, binding.SecretRef, binding.SecretVersion, binding.Phase)
			if err != nil {
				return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_MATERIALIZE_FAILED: %w", err)
			}
			if len(value) > 64<<10 {
				return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_TOO_LARGE")
			}
			variables[binding.Name] = value
			if value != "" {
				redaction = append(redaction, value)
			}
		default:
			return facade.Materialization{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_TYPE_INVALID")
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return facade.Materialization{}, err
	}
	return facade.Materialization{CycleID: cycleID, Variables: variables, ExpiresAt: time.Now().UTC().Add(2 * time.Minute).Format(time.RFC3339Nano), RedactionHint: redaction}, nil
}

// PersistEvaluation is the server-side acknowledgement boundary. No
// Supervisor action after process(context) is legal until this method commits
// an immutable cycle and returns its directive.
func (s *Store) PersistEvaluation(ctx context.Context, identity facade.MachineIdentity, attemptID int64, leaseToken string, candidate facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
	if err := validRuntimeIdentity(identity, attemptID, leaseToken); err != nil {
		return facade.EvaluationDirective{}, err
	}
	if candidate.CycleID <= 0 || candidate.CycleNumber < 1 || len(candidate.Contract) == 0 {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_EVALUATION_CANDIDATE_INVALID")
	}
	contract, err := processor.ValidateContract(candidate.Contract)
	if err != nil {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_CONTRACT_INVALID: %w", err)
	}
	digest, err := processor.ContractDigest(contract)
	if err != nil {
		return facade.EvaluationDirective{}, err
	}
	if candidate.ContractDigest != "" && candidate.ContractDigest != digest {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_CONTRACT_DIGEST_MISMATCH")
	}
	if len(candidate.SessionDigest) > 128 || len(candidate.CandidateTreeDigest) > 128 || len(candidate.FindingsDigest) > 128 {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_EVALUATION_IDENTITY_TOO_LARGE")
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return facade.EvaluationDirective{}, err
	}
	defer tx.Rollback(ctx)
	scope, err := s.loadEvaluationScope(ctx, tx, identity, attemptID, leaseToken)
	if err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("AUTHORIZE", err)
	}
	if scope.TaskStatus == "CANCELLING" || scope.TaskStatus == "CANCELLED" || scope.TaskStatus == "TIMED_OUT" || scope.TaskStatus == "FAILED" || scope.TaskStatus == "SUCCEEDED" || scope.TaskStatus == "MANUAL_REVIEW" {
		return facade.EvaluationDirective{}, ErrConflict
	}
	if scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING" && scope.TaskStatus != "REVIEWING" {
		return facade.EvaluationDirective{}, ErrConflict
	}
	if candidate.ProcessorReleaseID != scope.ProcessorReleaseID || candidate.ProcessorDigest != scope.ProcessorDigest {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_IDENTITY_MISMATCH")
	}
	if candidate.CycleNumber > scope.MaxCycles {
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_CYCLE_LIMIT_REACHED")
	}

	// Server-owned observations constrain the candidate before continuation.
	contract, err = s.completionCheckGate(ctx, tx, scope, attemptID, candidate.CycleID, contract)
	if err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("COMPLETION_CHECK", err)
	}
	digest, err = processor.ContractDigest(contract)
	if err != nil {
		return facade.EvaluationDirective{}, err
	}

	// The immutable cycle number is the primary idempotency boundary. An
	// identical candidate gets an ACK replay; a different body never changes an
	// already committed decision.
	var existingDigest, existingDecision, existingReason, existingMessage string
	var existingID int64
	err = tx.QueryRow(ctx, `SELECT contract_digest,decision,id,reason_code,message FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND cycle_number=$2 FOR UPDATE`, attemptID, candidate.CycleNumber).Scan(&existingDigest, &existingDecision, &existingID, &existingReason, &existingMessage)
	if err == nil {
		if existingDigest != digest {
			return facade.EvaluationDirective{}, ErrConflict
		}
		// The ordinary same-owner response-loss case is replayed by the Machine
		// idempotency middleware before it reaches this Store. Reaching this
		// branch means a valid current authority (most importantly a reattached
		// Worker) needs to continue a previously persisted candidate. REVISE
		// therefore receives a fresh, one-time resume nonce; terminal and PASS
		// decisions are reissued as their original safe directive. We never emit
		// a bare ACK_REPLAY here because it cannot tell the Supervisor whether it
		// may resume, propose, or stop.
		directive, replayErr := s.directiveForPersistedCycle(ctx, tx, existingID, existingDecision, existingReason, existingMessage, scope.MaxCycles-candidate.CycleNumber)
		if replayErr != nil {
			return facade.EvaluationDirective{}, evaluationStoreError("REPLAY", replayErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return facade.EvaluationDirective{}, evaluationStoreError("REPLAY_COMMIT", err)
		}
		return directive, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return facade.EvaluationDirective{}, evaluationStoreError("LOOKUP", err)
	}
	// WAIT_USER already moves the Task to REVIEWING. Current-owner replay may
	// reissue that exact persisted cycle above, but cannot create another cycle.
	if scope.TaskStatus == "REVIEWING" {
		return facade.EvaluationDirective{}, ErrConflict
	}
	var previousTree, previousFindings, previousReason string
	_ = tx.QueryRow(ctx, `SELECT candidate_tree_digest,findings_digest,reason_code FROM workforce_result_evaluation_cycles WHERE attempt_id=$1 AND cycle_number=$2`, attemptID, candidate.CycleNumber-1).Scan(&previousTree, &previousFindings, &previousReason)
	noProgress := contract.Decision == "REVISE" && candidate.CycleNumber > 1 && previousTree == candidate.CandidateTreeDigest && previousFindings == candidate.FindingsDigest && previousReason == contract.ReasonCode
	if noProgress {
		contract.Decision = "MANUAL_REVIEW"
		contract.ReasonCode = "RESULT_PROCESSOR_NO_PROGRESS"
		if contract.Message == "" {
			contract.Message = "连续两轮验收结果没有变化，已停止自动修正。"
		}
		digest, err = processor.ContractDigest(contract)
		if err != nil {
			return facade.EvaluationDirective{}, err
		}
	}

	cycleID := candidate.CycleID
	resumeNonce := ""
	resumeNonceHash := ""
	directive := facade.EvaluationDirective{CycleID: cycleID, Decision: contract.Decision, ReasonCode: contract.ReasonCode, Message: contract.Message, RemainingCycles: scope.MaxCycles - candidate.CycleNumber}
	switch contract.Decision {
	case "REVISE":
		if candidate.CycleNumber >= scope.MaxCycles {
			contract.Decision = "MANUAL_REVIEW"
			contract.ReasonCode = "RESULT_PROCESSOR_CYCLE_LIMIT_REACHED"
			digest, err = processor.ContractDigest(contract)
			if err != nil {
				return facade.EvaluationDirective{}, err
			}
			directive.Decision, directive.ReasonCode, directive.Message = contract.Decision, contract.ReasonCode, contract.Message
			directive.Directive = facade.DirectiveStopManualReview
		} else {
			resumeNonce, err = randomToken()
			if err != nil {
				return facade.EvaluationDirective{}, err
			}
			resumeNonceHash = sha256Hex(resumeNonce)
			directive.Directive = facade.DirectiveResumeSameSession
			directive.ResumeNonce = resumeNonce
			directive.ManagedFeedback = managedFeedback(contract, scope.MaxCycles-candidate.CycleNumber)
		}
	case "PASS":
		directive.Directive = facade.DirectiveProposeCompletion
	case "WAIT_USER":
		directive.Directive = facade.DirectiveWaitForUser
	case "REJECT":
		directive.Directive = facade.DirectiveStopRejected
	case "MANUAL_REVIEW":
		directive.Directive = facade.DirectiveStopManualReview
	default:
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_DECISION_INVALID")
	}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		return facade.EvaluationDirective{}, err
	}
	directive.Contract, directive.ContractDigest = contractJSON, digest
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_evaluation_cycles (id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,session_digest,candidate_tree_digest,findings_digest,contract_digest,decision,reason_code,message,contract_json,status,resume_nonce_sha256,persisted_at,completed_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'PERSISTED',$15,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, cycleID, scope.TaskID, scope.TurnID, scope.NodeID, attemptID, candidate.CycleNumber, candidate.SessionDigest, candidate.CandidateTreeDigest, candidate.FindingsDigest, digest, contract.Decision, contract.ReasonCode, contract.Message, contractJSON, resumeNonceHash); err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("PERSIST", classifyConflict(err))
	}
	// Output declarations become immutable with the persisted EvaluationCycle.
	// IMMEDIATE bytes are still independently frozen by the Result Coordinator;
	// ON_DEMAND rows are only declarations until an explicitly requested
	// MaterializationRun verifies bytes against the encrypted Checkpoint.
	if contract.Decision == "PASS" || contract.Decision == "WAIT_USER" || contract.Decision == "MANUAL_REVIEW" {
		for _, output := range contract.Outputs {
			selectionJSON, marshalErr := json.Marshal(output.Selection)
			if marshalErr != nil {
				return facade.EvaluationDirective{}, evaluationStoreError("OUTPUT_DECLARATION", marshalErr)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_outputs
				(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,name,source_path,storage,output_kind,archive_format,selection_spec_json,status)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'DECLARED')
				ON CONFLICT (evaluation_cycle_id,name) DO UPDATE
				SET source_path=EXCLUDED.source_path,storage=EXCLUDED.storage,output_kind=EXCLUDED.output_kind,archive_format=EXCLUDED.archive_format,selection_spec_json=EXCLUDED.selection_spec_json
				WHERE workforce_result_outputs.attempt_id=EXCLUDED.attempt_id
				  AND workforce_result_outputs.status='DECLARED'`,
				s.idGen.NextID(), scope.TaskID, scope.TurnID, attemptID, cycleID, output.Name, output.Path, output.Storage, output.Kind, output.ArchiveFormat, selectionJSON); err != nil {
				return facade.EvaluationDirective{}, evaluationStoreError("OUTPUT_DECLARATION", classifyConflict(err))
			}
		}
	}
	if candidate.SessionDigest != "" {
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET session_digest=CASE WHEN session_digest='' THEN $2 WHEN session_digest=$2 THEN session_digest ELSE session_digest END WHERE id=$1`, scope.TurnID, candidate.SessionDigest); err != nil {
			return facade.EvaluationDirective{}, evaluationStoreError("SESSION", err)
		}
	}
	if err = s.applyEvaluationDecision(ctx, tx, scope, attemptID, contract, cycleID); err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("DECISION", err)
	}
	if err = appendProcessorEvent(ctx, tx, scope.TaskID, attemptID, "result.evaluation."+strings.ToLower(contract.Decision), map[string]any{"cycleId": cycleID, "cycleNumber": candidate.CycleNumber, "decision": contract.Decision, "reasonCode": contract.ReasonCode, "contractDigest": digest}); err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("AUDIT", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return facade.EvaluationDirective{}, evaluationStoreError("COMMIT", err)
	}
	return directive, nil
}

// evaluationStoreError keeps the Machine API's operator-visible reason code
// precise enough to identify the failed durable boundary without returning a
// database, secret-resolution, or Result Contract value to the Worker.
// Known state conflicts retain their established fencing semantics.
func evaluationStoreError(stage string, err error) error {
	if err == nil || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound) {
		return err
	}
	return fmt.Errorf("RESULT_PROCESSOR_EVALUATION_%s_FAILED: %w", stage, err)
}

func (s *Store) ListEvaluations(ctx context.Context, tenantID int64, taskID int64) ([]facade.EvaluationView, error) {
	rows, err := s.db.Query(ctx, `SELECT e.id,e.task_id,e.task_turn_id,e.execution_node_id,e.attempt_id,e.cycle_number,e.session_digest,e.candidate_tree_digest,e.findings_digest,e.contract_digest,e.decision,e.reason_code,e.message,e.contract_json,e.status,to_char(e.started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(e.persisted_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(e.completed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_result_evaluation_cycles e JOIN workforce_tasks t ON t.id=e.task_id WHERE e.task_id=$1 AND t.tenant_id=$2 ORDER BY e.cycle_number`, taskID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]facade.EvaluationView, 0)
	for rows.Next() {
		var value facade.EvaluationView
		if err := rows.Scan(&value.ID, &value.TaskID, &value.TaskTurnID, &value.ExecutionNodeID, &value.AttemptID, &value.CycleNumber, &value.SessionDigest, &value.CandidateTreeDigest, &value.FindingsDigest, &value.ContractDigest, &value.Decision, &value.ReasonCode, &value.Message, &value.Contract, &value.Status, &value.StartedAt, &value.PersistedAt, &value.CompletedAt); err != nil {
			return nil, err
		}
		items = append(items, value)
	}
	return items, rows.Err()
}

func (s *Store) ListResultOutputs(ctx context.Context, tenantID int64, taskID int64) ([]facade.ResultOutputView, error) {
	rows, err := s.db.Query(ctx, `SELECT o.id,o.task_id,o.task_turn_id,o.evaluation_cycle_id,o.name,o.output_kind,o.archive_format,o.storage,o.status,COALESCE(o.artifact_id,0),o.sha256,o.size_bytes,to_char(o.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_outputs o JOIN workforce_tasks t ON t.id=o.task_id WHERE o.task_id=$1 AND t.tenant_id=$2 ORDER BY o.created_at,o.id`, taskID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]facade.ResultOutputView, 0)
	for rows.Next() {
		var item facade.ResultOutputView
		if err = rows.Scan(&item.ID, &item.TaskID, &item.TaskTurnID, &item.EvaluationCycleID, &item.Name, &item.OutputKind, &item.ArchiveFormat, &item.Storage, &item.Status, &item.ArtifactID, &item.SHA256, &item.SizeBytes, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type evaluationScope struct {
	TenantID                                 int64
	TaskID                                   int64
	TurnID                                   int64
	NodeID                                   string
	ProcessorReleaseID                       int64
	ProcessorDigest                          string
	TaskStatus, AttemptStatus, RuntimeStatus string
	MaxCycles                                int
	SecretVersions                           json.RawMessage
	Environment                              json.RawMessage
}

func (s *Store) loadEvaluationScope(ctx context.Context, tx pgx.Tx, identity facade.MachineIdentity, attemptID int64, lease string) (evaluationScope, error) {
	var scope evaluationScope
	var storedWorker int64
	var storedInstance, storedBoot, storedLease string
	var generation, ownerEpoch int64
	var expires time.Time
	// Lock Task before Attempt/Runtime, matching native interactions and leases.
	// An explicit parent read avoids a joined row-mark order occupying Attempt
	// while an interaction already holds Task. Processor authority is
	// frozen per TaskTurn and is loaded from that Turn below; runtime identity is
	// owned by the Attempt and never re-resolved from Project defaults.
	err := tx.QueryRow(ctx, `SELECT t.id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=$1 FOR UPDATE OF t`, attemptID).Scan(&scope.TaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return scope, ErrNotFound
	}
	if err != nil {
		return scope, err
	}
	err = tx.QueryRow(ctx, `SELECT t.tenant_id,a.task_id,a.task_turn_id,a.execution_node_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.generation,a.owner_epoch,a.lease_token,a.lease_expires_at,t.status,a.status,r.status FROM workforce_task_attempts a JOIN workforce_tasks t ON t.id=a.task_id JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.id=$1 FOR UPDATE OF a,t,r`, attemptID).Scan(&scope.TenantID, &scope.TaskID, &scope.TurnID, &scope.NodeID, &storedWorker, &storedInstance, &storedBoot, &generation, &ownerEpoch, &storedLease, &expires, &scope.TaskStatus, &scope.AttemptStatus, &scope.RuntimeStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return scope, ErrNotFound
	}
	if err != nil {
		return scope, err
	}
	if storedWorker != identity.WorkerID || storedInstance != identity.InstanceID || storedBoot != identity.HostBootID || generation != identity.Generation || ownerEpoch != identity.OwnerEpoch || storedLease != sha256Hex(lease) || !expires.After(time.Now()) {
		return scope, ErrConflict
	}
	if err = tx.QueryRow(ctx, `SELECT turn.processor_release_id,turn.processor_digest,
  COALESCE((release.manifest_json->>'maxEvaluationCycles')::integer,3),
  turn.secret_version_identities,turn.environment_snapshot_json
FROM workforce_task_turns turn
JOIN workforce_result_processor_releases release ON release.id=turn.processor_release_id AND release.archive_sha256=turn.processor_digest
WHERE turn.id=$1 AND turn.task_id=$2 AND turn.tenant_id=$3`, scope.TurnID, scope.TaskID, scope.TenantID).Scan(&scope.ProcessorReleaseID, &scope.ProcessorDigest, &scope.MaxCycles, &scope.SecretVersions, &scope.Environment); errors.Is(err, pgx.ErrNoRows) {
		return scope, ErrNotFound
	} else if err != nil {
		return scope, err
	}
	if scope.TurnID == 0 || scope.ProcessorReleaseID == 0 || scope.ProcessorDigest == "" || scope.MaxCycles < 1 || scope.MaxCycles > processor.MaximumEvaluationCycles || len(scope.Environment) == 0 {
		return scope, fmt.Errorf("RESULT_PROCESSOR_EXECUTION_SNAPSHOT_INVALID")
	}
	if scope.RuntimeStatus != "RUNNING" {
		return scope, ErrConflict
	}
	return scope, nil
}

func decodeFrozenEnvironment(raw json.RawMessage) (frozenEnvironment, error) {
	var environment frozenEnvironment
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&environment); err != nil {
		return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
	}
	seen := make(map[string]struct{}, len(environment.Bindings))
	for _, binding := range environment.Bindings {
		if binding.Name == "" || binding.Phase != "EVALUATE" || (binding.Type != "CONFIG" && binding.Type != "SECRET") {
			return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
		}
		if _, duplicate := seen[binding.Name]; duplicate {
			return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
		}
		seen[binding.Name] = struct{}{}
		if binding.Type == "CONFIG" && !json.Valid(binding.Config) {
			return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
		}
		if binding.Type == "SECRET" && (binding.SecretRef == "" || binding.SecretVersion == "" || len(binding.Config) != 0) {
			return frozenEnvironment{}, fmt.Errorf("RESULT_PROCESSOR_ENVIRONMENT_SNAPSHOT_INVALID")
		}
	}
	return environment, nil
}

func (s *Store) environmentBindingsTx(ctx context.Context, tx pgx.Tx, tenantID int64, releaseID int64) ([]EnvironmentBinding, error) {
	rows, err := tx.Query(ctx, `SELECT id,tenant_id,processor_release_id,name,type,phase,COALESCE(config_json,'null'::jsonb),secret_ref,secret_version,task_override_allowed,network_policy FROM workforce_result_processor_environment_bindings WHERE tenant_id=$1 AND processor_release_id=$2 ORDER BY name`, tenantID, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EnvironmentBinding, 0)
	for rows.Next() {
		var item EnvironmentBinding
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ProcessorReleaseID, &item.Name, &item.Type, &item.Phase, &item.Config, &item.SecretRef, &item.SecretVersion, &item.TaskOverrideAllowed, &item.NetworkPolicy); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) applyEvaluationDecision(ctx context.Context, tx pgx.Tx, scope evaluationScope, attemptID int64, contract processor.ResultContract, cycleID int64) error {
	switch contract.Decision {
	case "REVISE":
		_, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='RUNNING',version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status IN ('RUNNING','VALIDATING')`, scope.TaskID, scope.TurnID)
		return err
	case "PASS":
		_, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='VALIDATING',version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status IN ('RUNNING','VALIDATING')`, scope.TaskID, scope.TurnID)
		return err
	case "WAIT_USER":
		if _, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='REVIEWING',result_summary=$3,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status IN ('RUNNING','VALIDATING')`, scope.TaskID, scope.TurnID, contract.ReasonCode); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workforce_task_turns SET status='REVIEWING',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ACTIVE'`, scope.TurnID)
		return err
	case "REJECT":
		if _, err := tx.Exec(ctx, `UPDATE workforce_task_attempts SET status='FAILED',finished_at=CURRENT_TIMESTAMP,exit_code=1 WHERE id=$1 AND status IN ('RUNNING','PREPARING')`, attemptID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET status='FAILED',terminal_reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE attempt_id=$1`, attemptID, contract.ReasonCode); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE workforce_execution_nodes SET status='FAILED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, scope.NodeID); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='FAILED',finished_at=CURRENT_TIMESTAMP,result_summary=$3,version=version+1 WHERE id=$1 AND current_task_turn_id=$2`, scope.TaskID, scope.TurnID, contract.ReasonCode)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 1 {
			occurrence, eventErr := eventlog.AppendWithSequence(ctx, tx, scope.TaskID, attemptID, "task.failed", map[string]any{"reasonCode": contract.ReasonCode, "evaluationCycleId": cycleID})
			if eventErr != nil {
				return eventErr
			}
			if s.notificationRecorder != nil {
				_, eventErr = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: scope.TenantID, TaskID: scope.TaskID, TaskTurnID: scope.TurnID, SourceType: tasknotification.SourceResultEvaluation, SourceID: cycleID, SourceOccurrence: occurrence, PreviousStatus: "VALIDATING", CurrentStatus: "FAILED", ReasonCode: "PROCESSOR_REJECT", Summary: contract.ReasonCode, Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: time.Now().UTC()})
			}
			return eventErr
		}
		return nil
	case "MANUAL_REVIEW":
		if _, err := tx.Exec(ctx, `UPDATE workforce_tasks SET status='MANUAL_REVIEW',result_summary=$3,version=version+1 WHERE id=$1 AND current_task_turn_id=$2 AND status IN ('RUNNING','VALIDATING')`, scope.TaskID, scope.TurnID, contract.ReasonCode); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE workforce_task_turns SET status='MANUAL_REVIEW',finished_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='ACTIVE'`, scope.TurnID)
		return err
	default:
		return fmt.Errorf("RESULT_PROCESSOR_DECISION_INVALID")
	}
}

// directiveForPersistedCycle recreates only the safe continuation instruction
// for a durable EvaluationCycle. The persisted Result Contract stays
// immutable; the encrypted transport response/nonce is scoped to the current
// Worker authority and can be renewed after a legitimate same-host reattach.
func (s *Store) directiveForPersistedCycle(ctx context.Context, tx pgx.Tx, cycleID int64, decision string, reason string, message string, remaining int) (facade.EvaluationDirective, error) {
	d := facade.EvaluationDirective{CycleID: cycleID, Decision: decision, ReasonCode: reason, Message: message, RemainingCycles: remaining}
	if err := tx.QueryRow(ctx, `SELECT contract_json,contract_digest FROM workforce_result_evaluation_cycles WHERE id=$1`, cycleID).Scan(&d.Contract, &d.ContractDigest); err != nil {
		return facade.EvaluationDirective{}, err
	}
	switch decision {
	case "REVISE":
		nonce, err := randomToken()
		if err != nil {
			return facade.EvaluationDirective{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_result_evaluation_cycles SET resume_nonce_sha256=$2 WHERE id=$1 AND decision='REVISE'`, cycleID, sha256Hex(nonce)); err != nil {
			return facade.EvaluationDirective{}, err
		}
		d.Directive = facade.DirectiveResumeSameSession
		d.ResumeNonce = nonce
		d.ManagedFeedback = managedFeedback(processor.ResultContract{Decision: decision, ReasonCode: reason, Message: message}, remaining)
	case "PASS":
		d.Directive = facade.DirectiveProposeCompletion
	case "WAIT_USER":
		d.Directive = facade.DirectiveWaitForUser
	case "REJECT":
		d.Directive = facade.DirectiveStopRejected
	case "MANUAL_REVIEW":
		d.Directive = facade.DirectiveStopManualReview
	default:
		return facade.EvaluationDirective{}, fmt.Errorf("RESULT_PROCESSOR_PERSISTED_DECISION_INVALID")
	}
	return d, nil
}

func appendProcessorEvent(ctx context.Context, tx pgx.Tx, taskID int64, attemptID int64, eventType string, payload any) error {
	return eventlog.Append(ctx, tx, taskID, attemptID, eventType, payload)
}

func validRuntimeIdentity(identity facade.MachineIdentity, attemptID int64, lease string) error {
	if identity.WorkerID <= 0 || strings.TrimSpace(identity.InstanceID) == "" || strings.TrimSpace(identity.HostBootID) == "" || identity.Generation < 1 || identity.OwnerEpoch < 1 || attemptID <= 0 || strings.TrimSpace(lease) == "" {
		return ErrConflict
	}
	return nil
}

func managedFeedback(contract processor.ResultContract, remaining int) string {
	body, _ := json.Marshal(map[string]any{
		"reasonCode": contract.ReasonCode, "message": contract.Message,
		"checks": contract.Checks, "findings": contract.Findings,
		"remainingCycles": remaining,
	})
	return string(body)
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func jsonValueToString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	default:
		body, err := json.Marshal(typed)
		return string(body), err
	}
}

func secretFrozen(raw json.RawMessage, name, ref, version string) bool {
	var values []struct {
		Name    string `json:"name"`
		Ref     string `json:"ref"`
		Version string `json:"version"`
	}
	if json.Unmarshal(raw, &values) != nil {
		return false
	}
	for _, value := range values {
		if value.Name == name && value.Ref == ref && value.Version == version {
			return true
		}
	}
	return false
}
