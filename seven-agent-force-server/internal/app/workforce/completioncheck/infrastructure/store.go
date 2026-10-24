package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	principalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"path"
	"regexp"
	"strings"
	"unicode"
)

type IDGenerator interface{ NextID() int64 }
type Store struct {
	db         *pgxpool.Pool
	ids        IDGenerator
	turns      continuation.NextTurnCreator
	principals principalfacade.Resolver
}

func New(db *pgxpool.Pool, ids IDGenerator, turns continuation.NextTurnCreator, principals ...principalfacade.Resolver) *Store {
	s := &Store{db: db, ids: ids, turns: turns}
	if len(principals) > 0 {
		s.principals = principals[0]
	}
	return s
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (s *Store) Summary(ctx context.Context, tenant, task int64) (application.Summary, error) {
	return summary(ctx, s.db, tenant, task)
}
func summary(ctx context.Context, q queryer, tenant, task int64) (application.Summary, error) {
	out := application.Summary{AvailableActions: []string{}}
	var raw, referencesJSON []byte
	var taskStatus string
	err := q.QueryRow(ctx, `SELECT COALESCE(tr.completion_check_json,'null'::jsonb), COALESCE(tr.completion_check_digest,''),t.version,tr.id,COALESCE(tr.completion_check_references_json,'[]'::jsonb),t.status,
 COALESCE((SELECT c.id FROM workforce_continuation_checkpoints c WHERE c.task_id=t.id AND c.task_turn_id=tr.id AND c.decision='WAIT_USER' AND c.status='AVAILABLE' AND c.format_version=2 AND c.consumed_by_turn_id IS NULL AND c.expires_at>CURRENT_TIMESTAMP ORDER BY c.id DESC LIMIT 1),0)
 FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id AND tr.tenant_id=t.tenant_id WHERE t.tenant_id=$1 AND t.id=$2`, tenant, task).Scan(&raw, &out.Digest, &out.Version, &out.ExpectedCurrentTurnID, &referencesJSON, &taskStatus, &out.SourceCheckpointID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, facade.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	var definitionUnavailable bool
	out.Input, definitionUnavailable, err = decodeSummaryDefinition(raw, out.Digest)
	if err != nil {
		return out, err
	}
	if definitionUnavailable {
		out.ReasonCode = "CHECK_DEFINITION_UNAVAILABLE"
	}
	if out.Input != nil {
		var refs []facade.Reference
		if json.Unmarshal(referencesJSON, &refs) != nil {
			return out, facade.ErrConflict
		}
		digest, e := facade.FrozenDigest(*out.Input, refs)
		if e != nil || digest != out.Digest {
			return out, facade.ErrConflict
		}
	}
	if out.Input == nil && out.Digest != "" && !definitionUnavailable {
		return out, facade.ErrConflict
	}
	rows, err := q.Query(ctx, `SELECT id,task_turn_id,attempt_id,COALESCE(evaluation_cycle_id,0),phase,check_digest,status,summary,observation_json FROM workforce_completion_check_results WHERE tenant_id=$1 AND task_id=$2 AND check_digest=$4 AND ((phase='CURRENT' AND task_turn_id=$3) OR (phase='BASELINE' AND id=(SELECT completion_check_baseline_result_id FROM workforce_task_turns WHERE id=$3 AND tenant_id=$1))) ORDER BY id DESC`, tenant, task, out.ExpectedCurrentTurnID, out.Digest)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		e := facade.Evidence{TenantID: tenant, TaskID: task}
		var observation []byte
		if err = rows.Scan(&e.ID, &e.TaskTurnID, &e.AttemptID, &e.EvaluationCycleID, &e.Phase, &e.CheckDigest, &e.Status, &e.Summary, &observation); err != nil {
			return out, err
		}
		if err = json.Unmarshal(observation, &e.Observation); err != nil {
			return out, err
		}
		if e.Phase == "BASELINE" && out.Baseline == nil {
			out.Baseline = &e
		}
		if e.Phase == "CURRENT" && out.CurrentResult == nil {
			out.CurrentResult = &e
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	out.CheckpointReady = out.SourceCheckpointID > 0
	definitionChanged := out.Baseline != nil && out.CurrentResult != nil && out.Baseline.Observation.DefinitionDigest != out.CurrentResult.Observation.DefinitionDigest
	if definitionChanged && !definitionUnavailable {
		out.ReasonCode = "CHECK_DEFINITION_CHANGED"
		out.CurrentResult.Summary = "检查定义发生变化，请补充验证或授权修复。"
	}
	if !definitionUnavailable && out.CheckpointReady && out.Baseline != nil && out.CurrentResult != nil && (definitionChanged || (out.CurrentResult.Status != "PASSED" && out.CurrentResult.Status != "NOT_CONFIGURED")) {
		var eligible bool
		if err = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_continuation_checkpoints c JOIN workforce_result_evaluation_cycles ec ON ec.id=c.evaluation_cycle_id JOIN workforce_task_turns tr ON tr.id=c.task_turn_id WHERE c.id=$1 AND c.task_id=$2 AND c.task_turn_id=$3 AND c.attempt_id=$4 AND c.evaluation_cycle_id=$5 AND ec.status='PERSISTED' AND ec.decision='WAIT_USER' AND tr.status='REVIEWING')`, out.SourceCheckpointID, task, out.ExpectedCurrentTurnID, out.CurrentResult.AttemptID, out.CurrentResult.EvaluationCycleID).Scan(&eligible); err != nil {
			return out, err
		}
		if eligible && taskStatus == "REVIEWING" {
			observation := out.CurrentResult.Observation
			if !definitionChanged && observation.Status == "FAILED" && hexDigest.MatchString(observation.WorkspaceDigest) && (len(observation.Failures) > 0 || (observation.DiagnosticKnown && !observation.DiagnosticTruncated && hexDigest.MatchString(observation.DiagnosticDigest))) {
				out.AvailableActions = append(out.AvailableActions, facade.ContinueWithLimitations)
			}
			out.AvailableActions = append(out.AvailableActions, facade.AuthorizeRepair, facade.AddVerification)
		}
	}
	return out, nil
}

func decodeSummaryDefinition(raw []byte, digest string) (*facade.Input, bool, error) {
	var input *facade.Input
	if err := json.Unmarshal(raw, &input); err == nil {
		return input, false, nil
	}
	// Historical results remain useful even when their executable definition
	// contains the former reviewFocus field. Recognize only that exact retired
	// shape: other valid JSON which fails the current strict contract remains a
	// conflict so newly corrupted definitions cannot be hidden by this read path.
	// Do not interpret or execute the retired definition and do not offer
	// decisions; expose only persisted evidence under its original digest.
	if !json.Valid(raw) || !hexDigest.MatchString(digest) {
		return nil, false, facade.ErrConflict
	}
	var legacy map[string]json.RawMessage
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, false, facade.ErrConflict
	}
	retiredFocus, ok := legacy["reviewFocus"]
	if !ok {
		return nil, false, facade.ErrConflict
	}
	var focus string
	if err := json.Unmarshal(retiredFocus, &focus); err != nil {
		return nil, false, facade.ErrConflict
	}
	delete(legacy, "reviewFocus")
	currentRaw, err := json.Marshal(legacy)
	if err != nil || json.Unmarshal(currentRaw, &input) != nil || input == nil {
		return nil, false, facade.ErrConflict
	}
	return nil, true, nil
}

func (s *Store) RecordEvidence(ctx context.Context, tx pgx.Tx, e facade.Evidence) (facade.Evidence, bool, error) {
	var validationErr error
	e, validationErr = normalizeEvidence(e)
	if validationErr != nil {
		return e, false, validationErr
	}
	if tx == nil || s.ids == nil || e.TenantID <= 0 || e.TaskID <= 0 || e.TaskTurnID <= 0 || e.AttemptID <= 0 || len(e.CheckDigest) != 64 || len(e.Summary) > 4000 || (e.Phase != "BASELINE" && e.Phase != "CURRENT") || e.Observation.CheckDigest != e.CheckDigest || string(e.Observation.Status) != e.Status {
		return e, false, facade.ErrInvalid
	}
	switch e.Status {
	case "PASSED", "FAILED", "UNAVAILABLE", "NO_TESTS", "NOT_CONFIGURED", "TIMED_OUT":
	default:
		return e, false, facade.ErrInvalid
	}
	if (e.Phase == "BASELINE" && e.EvaluationCycleID != 0) || (e.Phase == "CURRENT" && e.EvaluationCycleID <= 0) {
		return e, false, facade.ErrInvalid
	}
	var digest string
	var frozenJSON, refJSON []byte
	err := tx.QueryRow(ctx, `SELECT COALESCE(tr.completion_check_digest,''),COALESCE(tr.completion_check_json,'null'::jsonb),COALESCE(tr.completion_check_references_json,'[]'::jsonb) FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id JOIN workforce_task_attempts a ON a.task_id=t.id AND a.task_turn_id=tr.id WHERE t.tenant_id=$1 AND t.id=$2 AND tr.id=$3 AND a.id=$4 FOR UPDATE OF t`, e.TenantID, e.TaskID, e.TaskTurnID, e.AttemptID).Scan(&digest, &frozenJSON, &refJSON)
	if errors.Is(err, pgx.ErrNoRows) || digest != e.CheckDigest {
		return e, false, facade.ErrConflict
	}
	if err != nil {
		return e, false, err
	}
	var frozen *facade.Input
	if json.Unmarshal(frozenJSON, &frozen) != nil || frozen == nil {
		return e, false, facade.ErrConflict
	}
	var frozenRefs []facade.Reference
	if json.Unmarshal(refJSON, &frozenRefs) != nil {
		return e, false, facade.ErrConflict
	}
	computed, err := facade.FrozenDigest(*frozen, frozenRefs)
	if err != nil || computed != digest {
		return e, false, facade.ErrConflict
	}
	raw, err := json.Marshal(e.Observation)
	if err != nil {
		return e, false, err
	}
	e.ID = s.ids.NextID()
	if err = tx.QueryRow(ctx, `SELECT source_commit,source_identity_digest FROM workforce_task_input_snapshots WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3`, e.TenantID, e.TaskID, e.TaskTurnID).Scan(&e.SourceCommit, &e.SourceIdentityDigest); err != nil {
		return e, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO workforce_completion_check_results(id,tenant_id,task_id,task_turn_id,attempt_id,phase,check_digest,status,summary,observation_json,evaluation_cycle_id,source_commit,source_identity_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11::BIGINT,0),$12,$13) ON CONFLICT DO NOTHING`, e.ID, e.TenantID, e.TaskID, e.TaskTurnID, e.AttemptID, e.Phase, e.CheckDigest, e.Status, e.Summary, raw, e.EvaluationCycleID, e.SourceCommit, e.SourceIdentityDigest)
	if err != nil {
		return e, false, err
	}
	if tag.RowsAffected() == 1 {
		if e.Phase == "BASELINE" {
			tag, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_baseline_result_id=$2 WHERE id=$1 AND completion_check_baseline_result_id IS NULL`, e.TaskTurnID, e.ID)
			if err != nil {
				return e, false, err
			}
			if tag.RowsAffected() != 1 {
				return e, false, facade.ErrConflict
			}
		}
		return e, false, nil
	}
	var equal bool
	err = tx.QueryRow(ctx, `SELECT id,status=$5 AND summary=$6 AND observation_json=$7::jsonb FROM workforce_completion_check_results WHERE tenant_id=$1 AND attempt_id=$2 AND phase=$3 AND check_digest=$4 AND COALESCE(evaluation_cycle_id,0)=$8`, e.TenantID, e.AttemptID, e.Phase, e.CheckDigest, e.Status, e.Summary, raw, e.EvaluationCycleID).Scan(&e.ID, &equal)
	if err != nil {
		return e, false, err
	}
	if !equal {
		return e, false, facade.ErrConflict
	}
	return e, true, nil
}

var hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var diagnosticCode = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)

func normalizeEvidence(e facade.Evidence) (facade.Evidence, error) {
	if e.Observation.DefinitionDigest != "" && !hexDigest.MatchString(e.Observation.DefinitionDigest) {
		return e, facade.ErrInvalid
	}
	if e.Observation.DiagnosticKnown && (e.Observation.DiagnosticTruncated || !hexDigest.MatchString(e.Observation.DiagnosticDigest)) {
		return e, facade.ErrInvalid
	}
	if !e.Observation.DiagnosticKnown && e.Observation.DiagnosticDigest != "" {
		return e, facade.ErrInvalid
	}
	if e.Status == "FAILED" && !hexDigest.MatchString(e.Observation.WorkspaceDigest) {
		return e, facade.ErrInvalid
	}
	if e.Observation.WorkspaceDigest != "" && !hexDigest.MatchString(e.Observation.WorkspaceDigest) {
		return e, facade.ErrInvalid
	}
	if !hexDigest.MatchString(e.CheckDigest) || e.Observation.CheckDigest != e.CheckDigest || string(e.Observation.Status) != e.Status || e.Observation.TestCount < 0 || e.Observation.TestCount > 10000000 || (!e.Observation.TestCountKnown && e.Observation.TestCount != 0) || len(e.Observation.Failures) > 256 {
		return e, facade.ErrInvalid
	}
	seen := map[string]bool{}
	if e.Status == "PASSED" && len(e.Observation.Failures) > 0 {
		return e, facade.ErrInvalid
	}
	for _, f := range e.Observation.Failures {
		if !diagnosticCode.MatchString(f.Key) || len(f.Path) > 1024 || f.Path == "" || path.IsAbs(f.Path) || path.Clean(f.Path) != f.Path || strings.Contains(f.Path, "\\") || strings.Contains(f.Path, ":") || strings.Contains(f.Path, "..") || strings.IndexFunc(f.Path, unicode.IsControl) >= 0 || !hexDigest.MatchString(f.Fingerprint) || seen[f.Fingerprint] {
			return e, facade.ErrInvalid
		}
		seen[f.Fingerprint] = true
	}
	switch e.Status {
	case "PASSED":
		e.Summary = "检查命令执行成功。"
		if e.Observation.TestCountKnown {
			e.Summary = "已识别的检查通过。"
		}
	case "FAILED":
		e.Summary = "检查未通过。"
	case "UNAVAILABLE":
		e.Summary = "暂时无法执行检查。"
	case "NO_TESTS":
		e.Summary = "未发现可执行的测试。"
	case "NOT_CONFIGURED":
		e.Summary = "未配置检查。"
	case "TIMED_OUT":
		e.Summary = "检查执行超时。"
	default:
		return e, facade.ErrInvalid
	}
	e.Summary = facade.SafeSummary(e.Observation)
	switch e.Observation.ReasonCode {
	case "", "CHECK_COMMAND_UNAVAILABLE", "CHECK_SNAPSHOT_UNAVAILABLE", "CHECK_EXECUTION_FAILED", "CHECK_TIMED_OUT", "CHECK_NO_TESTS", "CHECK_PASSED", "CHECK_NOT_CONFIGURED":
	default:
		e.Observation.ReasonCode = ""
	}
	if e.Observation.ExitCode != nil && (*e.Observation.ExitCode < 0 || *e.Observation.ExitCode > 255) {
		return e, facade.ErrInvalid
	}
	return e, nil
}

func (s *Store) Decide(ctx context.Context, d application.Decision) (application.DecisionResult, error) {
	d, err := application.NormalizeDecision(d)
	if err != nil {
		return application.DecisionResult{}, err
	}
	raw, _ := json.Marshal(struct {
		Actor    int64
		Decision application.Decision
	}{d.ActorID, d})
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.DecisionResult{}, err
	}
	defer tx.Rollback(ctx)
	var id int64
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_tasks WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, d.TenantID, d.TaskID).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = facade.ErrNotFound
		}
		return application.DecisionResult{}, err
	}
	var result application.DecisionResult
	var stored string
	err = tx.QueryRow(ctx, `SELECT id,task_turn_id,request_digest FROM workforce_completion_check_decisions WHERE tenant_id=$1 AND task_id=$2 AND idempotency_key=$3`, d.TenantID, d.TaskID, d.IdempotencyKey).Scan(&result.DecisionID, &result.TaskTurnID, &stored)
	if err == nil {
		if stored != digest {
			return result, facade.ErrIdempotencyConflict
		}
		result.Replayed = true
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	state, err := summary(ctx, tx, d.TenantID, d.TaskID)
	if err != nil {
		return result, err
	}
	allowed := false
	for _, action := range state.AvailableActions {
		if action == d.Action {
			allowed = true
		}
	}
	if state.Version != d.ExpectedVersion || state.CurrentResult == nil || state.CurrentResult.ID != d.ResultID {
		return result, facade.ErrStale
	}
	if !allowed {
		return result, facade.ErrActionNotAllowed
	}
	var originalInstruction string
	if err = tx.QueryRow(ctx, `SELECT instruction FROM workforce_task_turns WHERE tenant_id=$1 AND task_id=$2 AND turn_number=1`, d.TenantID, d.TaskID).Scan(&originalInstruction); err != nil {
		return result, err
	}
	instruction := originalInstruction + "\n\nContinue with the exact recorded optional-check limitations. Mandatory checks remain required."
	if d.Action == facade.AuthorizeRepair {
		instruction = originalInstruction + "\n\nAuthorized repair scope:\n" + d.RepairScope
	}
	if d.Action == facade.AddVerification {
		instruction = originalInstruction + "\n\nRun the newly specified optional verification against the original frozen source baseline, then verify the current work. Mandatory checks remain required."
	}
	turn, _, err := s.turns.CreateNextTurnTx(ctx, tx, continuation.NextTurnRequest{TenantID: d.TenantID, TaskID: d.TaskID, CreatorUserID: d.ActorID, SourceCheckpointID: state.SourceCheckpointID, ExpectedCurrentTurnID: state.ExpectedCurrentTurnID, Instruction: instruction, IdempotencyKey: "completion-check:" + digest})
	if err != nil {
		return result, err
	}
	result.TaskTurnID = turn.ID
	if d.CompletionCheck != nil {
		frozenReferences, freezeErr := freezeSelectedReferences(ctx, tx, d.TenantID, d.TaskID, turn.ID, d.CompletionCheck.ReferenceAssetIDs)
		if freezeErr != nil {
			return result, freezeErr
		}
		snapshot, _ := facade.Freeze(d.CompletionCheck)
		input, _ := json.Marshal(snapshot.Input)
		frozenDigest, digestErr := facade.FrozenDigest(snapshot.Input, frozenReferences)
		if digestErr != nil {
			return result, digestErr
		}
		refsJSON, _ := json.Marshal(frozenReferences)
		if _, err = tx.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_json=$2,completion_check_digest=$3,completion_check_input_digest=$4,completion_check_references_json=$5,completion_check_baseline_result_id=NULL WHERE id=$1`, turn.ID, input, frozenDigest, snapshot.Digest, refsJSON); err != nil {
			return result, err
		}
	}
	result.DecisionID = s.ids.NextID()
	if s.principals == nil {
		return result, errors.New("completion check principal resolver is unavailable")
	}
	actorPrincipalID, err := s.principals.EnsureUser(ctx, tx, s.ids, d.ActorID)
	if err != nil {
		return result, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_completion_check_decisions(id,tenant_id,task_id,source_task_turn_id,task_turn_id,checkpoint_id,actor_principal_id,action,result_id,baseline_result_id,reason,repair_scope,idempotency_key,request_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, result.DecisionID, d.TenantID, d.TaskID, state.ExpectedCurrentTurnID, turn.ID, state.SourceCheckpointID, actorPrincipalID, d.Action, d.ResultID, state.Baseline.ID, d.Reason, d.RepairScope, d.IdempotencyKey, digest)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
