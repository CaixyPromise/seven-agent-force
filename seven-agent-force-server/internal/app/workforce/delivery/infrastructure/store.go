package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

type IDGenerator interface{ NextID() int64 }

type Store struct {
	db                    *pgxpool.Pool
	idGen                 IDGenerator
	notificationRecorder  tasknotification.TransactionalRecorder
	toolAuthority         deliveryfacade.MachineAuthority
	conversationProjector deliveryfacade.ConversationProjector
	credentialMu          sync.RWMutex
	frozenCredentials     application.FrozenCredentialResolver
	frozenRepositories    taskfacade.FrozenRepositoryPlanReader
	repositoryBindings    projectfacade.DeliveryRepositoryBindingReader
}

func New(db *pgxpool.Pool, idGen IDGenerator) *Store { return &Store{db: db, idGen: idGen} }

func (s *Store) SetFrozenRepositoryPlanReader(reader taskfacade.FrozenRepositoryPlanReader) {
	s.frozenRepositories = reader
}

func (s *Store) SetDeliveryRepositoryBindingReader(reader projectfacade.DeliveryRepositoryBindingReader) {
	s.repositoryBindings = reader
}

// userHasDeliveryApprovalTx deliberately resolves the authorization in three
// narrow queries. Besides keeping the persistence boundary readable, this
// avoids coupling Delivery to a wide Framework RBAC join whose shape is hard
// to govern and easy to accidentally broaden.
func userHasDeliveryApprovalTx(ctx context.Context, tx pgx.Tx, principalID int64) (bool, error) {
	var userID int64
	err := tx.QueryRow(ctx, `SELECT user_id FROM workforce_principals WHERE id=$1 AND principal_type='USER'`, principalID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	rows, err := tx.Query(ctx, `SELECT ur."roleId" FROM sys_user_role ur JOIN sys_role r ON r.id=ur."roleId" WHERE ur."userId"=$1 AND ur."isDeleted"=false AND r."isDeleted"=false AND r.status=0`, userID)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	roleIDs := make([]int64, 0, 4)
	for rows.Next() {
		var roleID int64
		if err = rows.Scan(&roleID); err != nil {
			return false, err
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err = rows.Err(); err != nil {
		return false, err
	}
	rows.Close()

	postRows, err := tx.Query(ctx, `SELECT "postId" FROM sys_user_position WHERE "userId"=$1 AND "isDeleted"=false`, userID)
	if err != nil {
		return false, err
	}
	defer postRows.Close()
	postIDs := make([]int64, 0, 2)
	for postRows.Next() {
		var postID int64
		if err = postRows.Scan(&postID); err != nil {
			return false, err
		}
		postIDs = append(postIDs, postID)
	}
	if err = postRows.Err(); err != nil {
		return false, err
	}
	postRows.Close()
	if len(postIDs) > 0 {
		postRoleRows, queryErr := tx.Query(ctx, `SELECT pr."roleId" FROM sys_post_role pr JOIN sys_role r ON r.id=pr."roleId" WHERE pr."postId"=ANY($1) AND r."isDeleted"=false AND r.status=0`, postIDs)
		if queryErr != nil {
			return false, queryErr
		}
		defer postRoleRows.Close()
		for postRoleRows.Next() {
			var roleID int64
			if err = postRoleRows.Scan(&roleID); err != nil {
				return false, err
			}
			roleIDs = append(roleIDs, roleID)
		}
		if err = postRoleRows.Err(); err != nil {
			return false, err
		}
	}
	if len(roleIDs) == 0 {
		roleIDs = []int64{-1}
	}

	var authorized bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sys_role_permission rp JOIN sys_permission p ON p.id=rp."permissionId" WHERE rp."roleId"=ANY($1) AND p.code='workforce:delivery:approve' AND p."isDeleted"=false AND p.status=0)`, roleIDs).Scan(&authorized)
	if err != nil || authorized {
		return authorized, err
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sys_role_menu rm JOIN sys_menu m ON m.id=rm."menuId" WHERE rm."roleId"=ANY($1) AND m.permission='workforce:delivery:approve' AND m."isDeleted"=false AND m.status=0)`, roleIDs).Scan(&authorized)
	if err != nil || authorized {
		return authorized, err
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sys_user_permission up JOIN sys_permission p ON p.id=up."permissionId" WHERE up."userId"=$1 AND up."isDeleted"=false AND p.code='workforce:delivery:approve' AND p."isDeleted"=false AND p.status=0 AND (NOT up.type OR up."expireTime" IS NULL OR up."expireTime">CURRENT_TIMESTAMP))`, userID).Scan(&authorized)
	return authorized, err
}

func (s *Store) SetTaskNotificationRecorder(recorder tasknotification.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}
func (s *Store) SetToolAuthority(authority deliveryfacade.MachineAuthority) {
	if s != nil {
		s.toolAuthority = authority
	}
}
func (s *Store) SetConversationProjector(projector deliveryfacade.ConversationProjector) {
	if s != nil {
		s.conversationProjector = projector
	}
}

func (s *Store) SetFrozenCredentialResolver(resolver application.FrozenCredentialResolver) {
	if s == nil {
		return
	}
	s.credentialMu.Lock()
	defer s.credentialMu.Unlock()
	s.frozenCredentials = resolver
}

func (s *Store) resolveFrozenCredential(ctx context.Context, envelope application.EncryptedCredentialEnvelope) (application.DeliveryCredential, error) {
	s.credentialMu.RLock()
	resolver := s.frozenCredentials
	s.credentialMu.RUnlock()
	if resolver == nil {
		return application.DeliveryCredential{}, errors.New("DELIVERY_CREDENTIAL_RESOLVER_UNAVAILABLE")
	}
	return resolver.ResolveFrozenDeliveryCredential(ctx, envelope)
}

// Approval notifications belong to Delivery, rather than to the Agent's
// conversation projection. Each committed approval gate gets its own audit
// occurrence, preserving earlier notifications across reauthorization.
func (s *Store) recordDeliveryWaitingApproval(ctx context.Context, tx pgx.Tx, tenantID, taskID, turnID, requestID int64) error {
	if s.notificationRecorder == nil {
		return nil
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.waiting_approval", map[string]any{"deliveryRequestId": requestID})
	if err != nil {
		return err
	}
	_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceDeliveryRequest, SourceID: requestID, SourceOccurrence: occurrence, CurrentStatus: "WAITING_APPROVAL", ReasonCode: "DELIVERY_WAITING_APPROVAL", Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: time.Now().UTC()})
	return err
}

func (s *Store) recordDeliveryAttention(ctx context.Context, tx pgx.Tx, requestID int64, status, reason string) error {
	if s == nil || s.notificationRecorder == nil {
		return nil
	}
	if status != "MANUAL_REVIEW" && status != "OUTCOME_UNKNOWN" && status != "FAILED" {
		return nil
	}
	var tenantID, taskID, turnID int64
	if err := tx.QueryRow(ctx, `SELECT tenant_id,task_id,task_turn_id FROM workforce_delivery_requests WHERE id=$1`, requestID).Scan(&tenantID, &taskID, &turnID); err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.attention_required", map[string]any{"deliveryRequestId": requestID, "status": status, "reasonCode": reason})
	if err != nil {
		return err
	}
	reasonCode := "DELIVERY_MANUAL_REVIEW"
	if status == "OUTCOME_UNKNOWN" {
		reasonCode = "DELIVERY_OUTCOME_UNKNOWN"
	}
	if status == "FAILED" {
		reasonCode = "DELIVERY_FAILED"
	}
	_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceDeliveryRequest, SourceID: requestID, SourceOccurrence: occurrence, PreviousStatus: "RUNNING", CurrentStatus: status, ReasonCode: reasonCode, Summary: reason, Recoverability: tasknotification.RecoverabilityUserRecoverable, OccurredAt: time.Now().UTC()})
	return err
}

// recordDeliveryCompleted emits a distinct, exactly-once delivery fact only
// after every frozen remote side effect has been read back successfully. Task
// completion remains a separate fact and is never recreated here.
func (s *Store) recordDeliveryCompleted(ctx context.Context, tx pgx.Tx, requestID int64) error {
	if s == nil || s.notificationRecorder == nil {
		return nil
	}
	var tenantID, taskID, turnID int64
	err := tx.QueryRow(ctx, `UPDATE workforce_delivery_requests r
		SET delivery_completed_notified_at=CURRENT_TIMESTAMP
		WHERE r.id=$1 AND r.status='SUCCEEDED' AND r.delivery_completed_notified_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM workforce_delivery_operations o WHERE o.delivery_request_id=r.id AND o.operation_kind='CREATE_PULL_REQUEST' AND o.status<>'SUCCEEDED')
		RETURNING r.tenant_id,r.task_id,r.task_turn_id`, requestID).Scan(&tenantID, &taskID, &turnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	occurrence, err := eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.completed", map[string]any{"deliveryRequestId": requestID})
	if err != nil {
		return err
	}
	_, err = s.notificationRecorder.RecordTransition(ctx, tx, tasknotification.TaskNotificationTransition{TenantID: tenantID, TaskID: taskID, TaskTurnID: turnID, SourceType: tasknotification.SourceDeliveryRequest, SourceID: requestID, SourceOccurrence: occurrence, PreviousStatus: "RUNNING", CurrentStatus: "SUCCEEDED", ReasonCode: "DELIVERY_SUCCEEDED", Recoverability: tasknotification.RecoverabilityTerminal, OccurredAt: time.Now().UTC()})
	return err
}

func (s *Store) CreateRequest(ctx context.Context, request application.CreateRequest) (application.RequestView, bool, error) {
	submittedBody, _ := json.Marshal(request.Action)
	sum := sha256.Sum256(append([]byte(fmt.Sprintf("%d\x00", request.TaskID)), submittedBody...))
	requestDigest := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, false, err
	}
	defer tx.Rollback(ctx)
	if existing, stored, findErr := findRequestByKey(ctx, tx, request.TenantID, request.IdempotencyKey); findErr == nil {
		if stored != requestDigest {
			return existing, false, domain.ErrIdentityConflict
		}
		return existing, true, tx.Commit(ctx)
	} else if !errors.Is(findErr, pgx.ErrNoRows) {
		return application.RequestView{}, false, findErr
	}
	var turnID, checkpointID, processorRelease int64
	var processorDigest, resultCommit, resultTree, selectionDigest, taskStatus string
	var format int
	err = tx.QueryRow(ctx, `SELECT t.current_task_turn_id,t.status,tr.processor_release_id,tr.processor_digest,c.id,c.format_version,c.result_commit,c.result_tree,c.selection_digest FROM workforce_tasks t JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id JOIN workforce_continuation_checkpoints c ON c.task_turn_id=tr.id AND c.status='AVAILABLE' WHERE t.id=$1 AND t.tenant_id=$2 ORDER BY c.available_at DESC LIMIT 1 FOR UPDATE OF t,tr,c`, request.TaskID, request.TenantID).Scan(&turnID, &taskStatus, &processorRelease, &processorDigest, &checkpointID, &format, &resultCommit, &resultTree, &selectionDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, false, application.ErrNotFound
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	if taskStatus != "SUCCEEDED" || format != 2 {
		return application.RequestView{}, false, errors.New("CONTINUATION_CHECKPOINT_FORMAT_UNSUPPORTED")
	}
	var actorPrincipalID int64
	if request.ActorServiceCode != "" {
		actorPrincipalID, err = principalinfra.EnsureService(ctx, tx, s.idGen, request.ActorServiceCode)
	} else {
		actorPrincipalID, err = principalinfra.EnsureUser(ctx, tx, s.idGen, request.Actor)
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	plan, err := s.loadFrozenRepositoryPlan(ctx, request.TenantID, request.TaskID, turnID)
	if errors.Is(err, application.ErrNotFound) {
		return application.RequestView{}, false, errors.New("TASK_GIT_DELIVERY_NOT_CONFIGURED")
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	var expectedBase string
	request.Action, expectedBase, err = domain.BindActionToGitPlan(request.Action, domain.FrozenGitPlan{TargetID: plan.ProjectID, BaseBranch: plan.BaseBranch, BaseCommit: plan.BaseCommit, DestinationBranch: plan.ResultBranch, DeliveryMode: plan.DeliveryMode, PreviousDeliveryCommit: plan.PreviousDeliveryCommit})
	if err != nil {
		return application.RequestView{}, false, errors.New("DELIVERY_ACTION_GIT_PLAN_CONFLICT")
	}
	body, _ := json.Marshal(request.Action)
	target, err := plan.target(request.TenantID)
	if err != nil {
		return application.RequestView{}, false, err
	}
	targetIdentityDigest, err := domain.ComputeTargetIdentityDigest(target)
	if err != nil {
		return application.RequestView{}, false, err
	}
	prepared, err := domain.Prepare(domain.RequestInput{TenantID: request.TenantID, TaskID: request.TaskID, TaskTurnID: turnID, CheckpointID: checkpointID, ProcessorReleaseID: processorRelease, ProcessorDigest: processorDigest, ResultCommit: resultCommit, ResultTree: resultTree, SelectionDigest: selectionDigest, ExpectedBaseCommit: expectedBase, Action: request.Action, Target: target, TargetConfigDigest: plan.TargetConfigDigest, CredentialVersion: fmt.Sprint(plan.Credential.Version), AuthorizationDigest: plan.PushAuthorizationDigest, Now: time.Now()})
	if err != nil {
		return application.RequestView{}, false, err
	}
	reviewVersion, reviewAuthorizationDigest := fmt.Sprint(plan.Credential.Version), plan.ReviewAuthorizationDigest
	id := s.idGen.NextID()
	initialStatus := prepared.InitialStatus
	autoAuthorized := plan.DeliveryAuthorization == "AUTO_AFTER_GATES" && plan.AuthorizedPrincipalID > 0 && len(plan.PlanDigest) == 64 &&
		plan.TargetConfigDigest == target.ConfigDigest && plan.PushAuthorizationDigest == prepared.AuthorizationDigest &&
		((plan.DeliveryMode == "NEW_BRANCH" && plan.ReviewAuthorizationDigest == "") || (plan.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" && len(reviewAuthorizationDigest) == 64))
	if autoAuthorized {
		var stillAuthorized bool
		stillAuthorized, err = userHasDeliveryApprovalTx(ctx, tx, plan.AuthorizedPrincipalID)
		if err != nil {
			return application.RequestView{}, false, err
		}
		autoAuthorized = stillAuthorized
	}
	if autoAuthorized {
		initialStatus = "READY"
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_requests (id,tenant_id,task_id,task_turn_id,checkpoint_id,action_type,action_json,action_digest,processor_release_id,processor_digest,result_commit,result_tree,selection_digest,project_id,target_config_digest,target_identity_digest,policy_digest,credential_version,authorization_digest,base_branch,destination_branch,expected_base_commit,delivery_commit,commit_timestamp,approval_mode,status,idempotency_key,request_digest,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)`, id, request.TenantID, request.TaskID, turnID, checkpointID, request.Action.Type, body, prepared.ActionDigest, processorRelease, processorDigest, resultCommit, resultTree, selectionDigest, plan.ProjectID, prepared.TargetConfigDigest, targetIdentityDigest, prepared.PolicyDigest, prepared.CredentialVersion, prepared.AuthorizationDigest, prepared.BaseBranch, prepared.DestinationBranch, prepared.ExpectedBaseCommit, prepared.DeliveryCommit, prepared.CommitTimestamp, prepared.ApprovalMode, initialStatus, request.IdempotencyKey, requestDigest, actorPrincipalID)
	if err != nil {
		return application.RequestView{}, false, err
	}
	pushOperationID := s.idGen.NextID()
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_operations (id,tenant_id,delivery_request_id,operation_kind,status,authorization_version,authorization_digest,idempotency_key,request_digest) VALUES ($1,$2,$3,'PUSH_BRANCH',$4,$5,$6,$7,$8)`, pushOperationID, request.TenantID, id, initialStatus, prepared.CredentialVersion, prepared.AuthorizationDigest, fmt.Sprintf("delivery-operation:%d:push", id), prepared.ActionDigest)
	if err != nil {
		return application.RequestView{}, false, err
	}
	if plan.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" {
		_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_operations (id,tenant_id,delivery_request_id,operation_kind,status,authorization_version,authorization_digest,idempotency_key,request_digest) VALUES ($1,$2,$3,'CREATE_PULL_REQUEST','WAITING_DEPENDENCY',$4,$5,$6,$7)`, s.idGen.NextID(), request.TenantID, id, reviewVersion, reviewAuthorizationDigest, fmt.Sprintf("delivery-operation:%d:review", id), prepared.PolicyDigest)
		if err != nil {
			return application.RequestView{}, false, err
		}
	}
	if autoAuthorized {
		autoBody, _ := json.Marshal(map[string]any{"requestId": id, "gitPlanDigest": plan.PlanDigest, "authorizationDigest": prepared.AuthorizationDigest})
		autoSum := sha256.Sum256(autoBody)
		decisionAuthorization := deliveryDecisionAuthorizationDigest(plan.PlanDigest, target.ConfigDigest, prepared.AuthorizationDigest)
		_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_decisions (id,tenant_id,task_id,delivery_request_id,action,expected_version,principal_id,idempotency_key,request_digest,authorization_digest,reason) VALUES ($1,$2,$3,$4,'AUTO_FROM_TASK_PLAN',1,$5,$6,$7,$8,'')`, s.idGen.NextID(), request.TenantID, request.TaskID, id, plan.AuthorizedPrincipalID, fmt.Sprintf("delivery-auto:%d", id), hex.EncodeToString(autoSum[:]), decisionAuthorization)
		if err != nil {
			return application.RequestView{}, false, err
		}
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET approved_by_principal_id=$2,approved_at=CURRENT_TIMESTAMP,version=version+1 WHERE id=$1`, id, plan.AuthorizedPrincipalID)
		if err != nil {
			return application.RequestView{}, false, err
		}
	}
	var sourceAttemptID, adapterGeneration int64
	if err = tx.QueryRow(ctx, `SELECT a.id,COALESCE(r.adapter_session_generation,1) FROM workforce_task_attempts a LEFT JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id WHERE a.task_turn_id=$1 ORDER BY a.attempt_no DESC,a.id DESC LIMIT 1`, turnID).Scan(&sourceAttemptID, &adapterGeneration); err != nil {
		return application.RequestView{}, false, err
	}
	defaultSummary := "任务结果已准备交付。"
	defaultSummaryDigest := domain.DigestText(defaultSummary)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_intents (id,tenant_id,task_id,task_turn_id,attempt_id,adapter_session_generation,local_tool_call_key,native_tool_call_id,round_key,summary,summary_digest,git_plan_digest,delivery_mode,delivery_authorization,status) VALUES ($1,$2,$3,$4,$5,$6,'platform-default','platform-default','platform-finalizer',$7,$8,$9,$10,$11,'WAITING_FINALIZATION') ON CONFLICT (task_turn_id) DO NOTHING`, s.idGen.NextID(), request.TenantID, request.TaskID, turnID, sourceAttemptID, adapterGeneration, defaultSummary, defaultSummaryDigest, plan.PlanDigest, plan.DeliveryMode, plan.DeliveryAuthorization)
	if err != nil {
		return application.RequestView{}, false, err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_intents SET delivery_request_id=$2,status=CASE WHEN $3 THEN 'DELIVERY_RUNNING' ELSE 'WAITING_USER_DECISION' END,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND task_turn_id=$4 AND git_plan_digest=$5`, request.TenantID, id, autoAuthorized, turnID, plan.PlanDigest)
	if err != nil {
		return application.RequestView{}, false, err
	}
	if err = s.projectDelivery(ctx, tx, request.TenantID, turnID); err != nil {
		return application.RequestView{}, false, err
	}
	if initialStatus == "READY" {
		if err = s.schedule(ctx, tx, id); err != nil {
			return application.RequestView{}, false, err
		}
	}
	if initialStatus == "WAITING_APPROVAL" {
		if err = s.recordDeliveryWaitingApproval(ctx, tx, request.TenantID, request.TaskID, turnID, id); err != nil {
			return application.RequestView{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, false, err
	}
	view, err := s.getRequest(ctx, request.TenantID, id)
	return view, false, err
}

func (s *Store) ListRequests(ctx context.Context, tenant int64, task int64) ([]application.RequestView, error) {
	rows, err := s.db.Query(ctx, requestSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY created_at DESC,id DESC`, tenant, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []application.RequestView{}
	for rows.Next() {
		item, scanErr := scanRequest(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for index := range items {
		if err = s.hydrateReview(ctx, &items[index]); err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (s *Store) GetRequest(ctx context.Context, tenant int64, id int64) (application.RequestView, error) {
	return s.getRequest(ctx, tenant, id)
}
func (s *Store) GetAttempt(ctx context.Context, tenant int64, id int64) (application.AttemptView, error) {
	var item application.AttemptView
	err := s.db.QueryRow(ctx, `SELECT a.id,a.request_id,a.attempt_number,a.worker_id,a.worker_instance_id,a.host_boot_id,a.phase,a.status,a.remote_before,a.remote_after,a.reason_code,
		COALESCE(to_char(a.started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(a.verified_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(a.finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
		FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id WHERE a.id=$1 AND r.tenant_id=$2`, id, tenant).Scan(&item.ID, &item.RequestID, &item.AttemptNumber, &item.WorkerID, &item.WorkerInstanceID, &item.HostBootID, &item.Phase, &item.Status, &item.RemoteBefore, &item.RemoteAfter, &item.ReasonCode, &item.StartedAt, &item.VerifiedAt, &item.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, application.ErrNotFound
	}
	return item, err
}
func (s *Store) Approve(ctx context.Context, a application.Approval) (application.RequestView, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, a.Actor)
	if err != nil {
		return application.RequestView{}, err
	}
	var projectID, planProjectID int64
	var readbackFirst bool
	var bindingVersion *int64
	var taskID, turnID int64
	var status, targetDigest, planTargetDigest, authorizationDigest, planAuthorizationDigest string
	err = tx.QueryRow(ctx, `SELECT r.status,r.project_id,r.target_config_digest,r.authorization_digest,p.project_id,p.target_config_digest,p.push_authorization_digest,r.reauthorized_at IS NOT NULL,r.reauthorized_project_version_id,r.task_id,r.task_turn_id
		FROM workforce_delivery_requests r
		JOIN workforce_task_git_plans p ON p.tenant_id=r.tenant_id AND p.task_id=r.task_id AND p.task_turn_id=r.task_turn_id AND p.status='ACTIVE'
		WHERE r.id=$1 AND r.tenant_id=$2 FOR UPDATE OF r`, a.RequestID, a.TenantID).Scan(&status, &projectID, &targetDigest, &authorizationDigest, &planProjectID, &planTargetDigest, &planAuthorizationDigest, &readbackFirst, &bindingVersion, &taskID, &turnID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, application.ErrNotFound
	}
	if err != nil {
		return application.RequestView{}, err
	}
	if bindingVersion != nil {
		plan, planErr := s.loadDeliveryRepositoryPlan(ctx, a.TenantID, taskID, turnID, a.RequestID)
		if planErr != nil {
			return application.RequestView{}, planErr
		}
		planProjectID, planTargetDigest, planAuthorizationDigest = plan.ProjectID, plan.TargetConfigDigest, plan.PushAuthorizationDigest
	}
	if projectID != planProjectID || targetDigest != planTargetDigest || authorizationDigest != planAuthorizationDigest {
		return application.RequestView{}, domain.ErrIdentityConflict
	}
	if status == "SUCCEEDED" {
		// A review credential can be explicitly reauthorized after the branch
		// has been delivered.  Approving that operation must not schedule a
		// second push of the immutable DeliveryCommit.
		command, updateErr := tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',reason_code='',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='CREATE_PULL_REQUEST' AND status='WAITING_APPROVAL'`, a.RequestID)
		if updateErr != nil {
			return application.RequestView{}, updateErr
		}
		if command.RowsAffected() != 1 {
			return application.RequestView{}, domain.ErrApprovalRequired
		}
		if err = tx.Commit(ctx); err != nil {
			return application.RequestView{}, err
		}
		return s.getRequest(ctx, a.TenantID, a.RequestID)
	}
	if status != "WAITING_APPROVAL" {
		return application.RequestView{}, domain.ErrApprovalRequired
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET approved_by_principal_id=$2,approved_at=CURRENT_TIMESTAMP,status='READY',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, a.RequestID, actorPrincipalID)
	if err != nil {
		return application.RequestView{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH' AND status='WAITING_APPROVAL'`, a.RequestID); err != nil {
		return application.RequestView{}, err
	}
	if err = s.scheduleMode(ctx, tx, a.RequestID, readbackFirst); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	return s.getRequest(ctx, a.TenantID, a.RequestID)
}

// Decide is the user-facing Delivery decision. It binds one immutable request
// version and never accepts a remote, refspec, branch, force flag, or credential
// from the browser.
func (s *Store) Decide(ctx context.Context, d application.Decision) (application.RequestView, bool, error) {
	body, _ := json.Marshal(struct {
		TaskID, RequestID, ExpectedVersion int64
		Action, Reason                     string
	}{d.TaskID, d.RequestID, d.ExpectedVersion, d.Action, d.Reason})
	sum := sha256.Sum256(body)
	requestDigest := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, false, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, d.Actor)
	if err != nil {
		return application.RequestView{}, false, err
	}
	var existingID, existingRequest int64
	var existingDigest string
	err = tx.QueryRow(ctx, `SELECT id,delivery_request_id,request_digest FROM workforce_delivery_decisions WHERE tenant_id=$1 AND principal_id=$2 AND idempotency_key=$3`, d.TenantID, principalID, d.IdempotencyKey).Scan(&existingID, &existingRequest, &existingDigest)
	if err == nil {
		if existingRequest != d.RequestID || existingDigest != requestDigest {
			return application.RequestView{}, false, domain.ErrDecisionConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return application.RequestView{}, false, err
		}
		view, getErr := s.getRequest(ctx, d.TenantID, d.RequestID)
		return view, true, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, false, err
	}
	var status, authorizationDigest, targetDigest, gitPlanDigest, planTargetDigest, planAuthorizationDigest string
	var version, taskID, projectID, planProjectID int64
	var readbackFirst bool
	var bindingVersion *int64
	var turnID int64
	err = tx.QueryRow(ctx, `SELECT r.version,r.task_id,r.project_id,r.status,r.authorization_digest,r.target_config_digest,p.plan_digest,p.project_id,p.target_config_digest,p.push_authorization_digest,r.reauthorized_at IS NOT NULL,r.reauthorized_project_version_id,r.task_turn_id
		FROM workforce_delivery_requests r JOIN workforce_task_git_plans p ON p.task_turn_id=r.task_turn_id AND p.task_id=r.task_id AND p.tenant_id=r.tenant_id AND p.status='ACTIVE'
		WHERE r.id=$1 AND r.tenant_id=$2 FOR UPDATE OF r`, d.RequestID, d.TenantID).Scan(&version, &taskID, &projectID, &status, &authorizationDigest, &targetDigest, &gitPlanDigest, &planProjectID, &planTargetDigest, &planAuthorizationDigest, &readbackFirst, &bindingVersion, &turnID)
	if errors.Is(err, pgx.ErrNoRows) || taskID != d.TaskID {
		return application.RequestView{}, false, application.ErrNotFound
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	if bindingVersion != nil {
		plan, planErr := s.loadDeliveryRepositoryPlan(ctx, d.TenantID, taskID, turnID, d.RequestID)
		if planErr != nil {
			return application.RequestView{}, false, planErr
		}
		planProjectID, planTargetDigest, planAuthorizationDigest = plan.ProjectID, plan.TargetConfigDigest, plan.PushAuthorizationDigest
	}
	if projectID != planProjectID || targetDigest != planTargetDigest || authorizationDigest != planAuthorizationDigest {
		return application.RequestView{}, false, domain.ErrReauthorization
	}
	if version != d.ExpectedVersion {
		return application.RequestView{}, false, domain.ErrRequestStale
	}
	if status != "WAITING_APPROVAL" {
		return application.RequestView{}, false, domain.ErrDecisionNotAllowed
	}
	decisionID := s.idGen.NextID()
	decisionAuthorization := deliveryDecisionAuthorizationDigest(gitPlanDigest, targetDigest, authorizationDigest)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_decisions (id,tenant_id,task_id,delivery_request_id,action,expected_version,principal_id,idempotency_key,request_digest,authorization_digest,reason) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, decisionID, d.TenantID, d.TaskID, d.RequestID, d.Action, d.ExpectedVersion, principalID, d.IdempotencyKey, requestDigest, decisionAuthorization, d.Reason)
	if err != nil {
		return application.RequestView{}, false, err
	}
	if d.Action == "SKIP_THIS_RESULT" {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='CANCELLED',reason_code='USER_SKIPPED',version=version+1,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='WAITING_APPROVAL'`, d.RequestID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='CANCELLED',reason_code='USER_SKIPPED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, d.RequestID)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_delivery_intents SET status='SKIPPED',version=version+1,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1`, d.RequestID)
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET approved_by_principal_id=$2,approved_at=CURRENT_TIMESTAMP,status='READY',version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='WAITING_APPROVAL'`, d.RequestID, principalID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH' AND status='WAITING_APPROVAL'`, d.RequestID)
		}
		if err == nil {
			err = s.scheduleMode(ctx, tx, d.RequestID, readbackFirst)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE workforce_delivery_intents SET status='DELIVERY_RUNNING',version=version+1,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1`, d.RequestID)
		}
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	if err = s.projectDelivery(ctx, tx, d.TenantID, turnID); err != nil {
		return application.RequestView{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, false, err
	}
	view, err := s.getRequest(ctx, d.TenantID, d.RequestID)
	return view, false, err
}

// Restart reopens only the skipped DeliveryRequest for the Task's current,
// still-deliverable ResultCommit. It never accepts or reconstructs a Git target
// from browser input.
func (s *Store) Restart(ctx context.Context, r application.Restart) (application.RequestView, bool, error) {
	body, _ := json.Marshal(struct {
		TaskID       int64
		ResultCommit string
	}{r.TaskID, r.ResultCommit})
	sum := sha256.Sum256(body)
	requestDigest := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, false, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, r.Actor)
	if err != nil {
		return application.RequestView{}, false, err
	}
	var existingRequest int64
	var existingDigest string
	err = tx.QueryRow(ctx, `SELECT delivery_request_id,request_digest FROM workforce_delivery_decisions WHERE tenant_id=$1 AND principal_id=$2 AND idempotency_key=$3`, r.TenantID, principalID, r.IdempotencyKey).Scan(&existingRequest, &existingDigest)
	if err == nil {
		if existingDigest != requestDigest {
			return application.RequestView{}, false, domain.ErrDecisionConflict
		}
		if err = tx.Commit(ctx); err != nil {
			return application.RequestView{}, false, err
		}
		view, getErr := s.getRequest(ctx, r.TenantID, existingRequest)
		return view, true, getErr
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, false, err
	}
	var requestID, turnID, version int64
	var status, reasonCode, taskStatus, authorizationDigest, targetDigest, gitPlanDigest string
	var resultAvailable bool
	err = tx.QueryRow(ctx, `
		SELECT r.id,r.task_turn_id,r.version,r.status,r.reason_code,t.status,r.authorization_digest,r.target_config_digest,p.plan_digest
		FROM workforce_delivery_requests r
		JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
		JOIN workforce_task_git_plans p ON p.task_turn_id=r.task_turn_id AND p.tenant_id=r.tenant_id
		WHERE r.tenant_id=$1 AND r.task_id=$2 AND r.result_commit=$3 AND t.current_task_turn_id=r.task_turn_id
		ORDER BY r.id DESC LIMIT 1 FOR UPDATE OF r,t`, r.TenantID, r.TaskID, r.ResultCommit).Scan(&requestID, &turnID, &version, &status, &reasonCode, &taskStatus, &authorizationDigest, &targetDigest, &gitPlanDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, false, domain.ErrResultNotCurrent
	}
	if err != nil {
		return application.RequestView{}, false, err
	}
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workforce_continuation_checkpoints c JOIN workforce_delivery_requests r ON r.checkpoint_id=c.id WHERE r.id=$1 AND c.task_id=$2 AND c.task_turn_id=$3 AND c.status='AVAILABLE' AND c.result_commit=r.result_commit)`, requestID, r.TaskID, turnID).Scan(&resultAvailable)
	if err != nil {
		return application.RequestView{}, false, err
	}
	if status != "CANCELLED" || reasonCode != "USER_SKIPPED" || taskStatus != "SUCCEEDED" || !resultAvailable {
		return application.RequestView{}, false, domain.ErrDecisionNotAllowed
	}
	decisionID := s.idGen.NextID()
	decisionAuthorization := deliveryDecisionAuthorizationDigest(gitPlanDigest, targetDigest, authorizationDigest)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_decisions (id,tenant_id,task_id,delivery_request_id,action,expected_version,principal_id,idempotency_key,request_digest,authorization_digest,reason) VALUES ($1,$2,$3,$4,'RESTART_DELIVERY',$5,$6,$7,$8,$9,'')`, decisionID, r.TenantID, r.TaskID, requestID, version, principalID, r.IdempotencyKey, requestDigest, decisionAuthorization)
	if err != nil {
		return application.RequestView{}, false, err
	}
	command, err := tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='WAITING_APPROVAL',reason_code='',approved_by_principal_id=NULL,approved_at=NULL,finished_at=NULL,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='CANCELLED' AND reason_code='USER_SKIPPED'`, requestID)
	if err != nil {
		return application.RequestView{}, false, err
	}
	if command.RowsAffected() != 1 {
		return application.RequestView{}, false, domain.ErrRequestStale
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='WAITING_APPROVAL',reason_code='',finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH' AND status='CANCELLED' AND reason_code='USER_SKIPPED'`, requestID); err != nil {
		return application.RequestView{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='WAITING_DEPENDENCY',reason_code='',finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='CREATE_PULL_REQUEST' AND status='CANCELLED' AND reason_code='USER_SKIPPED'`, requestID); err != nil {
		return application.RequestView{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_intents SET status='WAITING_USER_DECISION',version=version+1,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1`, requestID); err != nil {
		return application.RequestView{}, false, err
	}
	if err = s.recordDeliveryWaitingApproval(ctx, tx, r.TenantID, r.TaskID, turnID, requestID); err != nil {
		return application.RequestView{}, false, err
	}
	if err = s.projectDelivery(ctx, tx, r.TenantID, turnID); err != nil {
		return application.RequestView{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, false, err
	}
	view, err := s.getRequest(ctx, r.TenantID, requestID)
	return view, false, err
}

func deliveryDecisionAuthorizationDigest(gitPlanDigest, targetConfigDigest, credentialAuthorizationDigest string) string {
	sum := sha256.Sum256([]byte(gitPlanDigest + "\x00" + targetConfigDigest + "\x00" + credentialAuthorizationDigest))
	return hex.EncodeToString(sum[:])
}
func (s *Store) Cancel(ctx context.Context, tenant int64, actor int64, id int64) (application.RequestView, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN 'CANCELLING' ELSE 'CANCELLED' END,reason_code='USER_CANCELLED',updated_at=CURRENT_TIMESTAMP,finished_at=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN finished_at ELSE CURRENT_TIMESTAMP END WHERE id=$1 AND tenant_id=$2 AND status IN ('WAITING_APPROVAL','MANUAL_ONLY','READY','DISPATCHED','RUNNING','CONFIRMING')`, id, tenant)
	if err != nil {
		return application.RequestView{}, err
	}
	if tag.RowsAffected() == 0 {
		return application.RequestView{}, domain.ErrIdentityConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN 'CANCELLING' ELSE 'CANCELLED' END,reason_code='USER_CANCELLED',finished_at=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN finished_at ELSE CURRENT_TIMESTAMP END,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, id); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	_ = actor
	return s.getRequest(ctx, tenant, id)
}

// CancelForTask is the transaction-aware Task lifecycle fence. It closes both
// the authoritative Delivery state and its AI-native intent projection without
// treating an already verified remote delivery as reversible.
func (s *Store) CancelForTask(ctx context.Context, tx pgx.Tx, taskID int64, reason string) error {
	if taskID < 1 || strings.TrimSpace(reason) == "" {
		return domain.ErrIdentityConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_delivery_requests
		SET status=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN 'CANCELLING' ELSE 'CANCELLED' END,
			reason_code=$2,updated_at=CURRENT_TIMESTAMP,
			finished_at=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN finished_at ELSE CURRENT_TIMESTAMP END
		WHERE task_id=$1 AND status IN ('WAITING_APPROVAL','MANUAL_ONLY','READY','DISPATCHED','RUNNING','CONFIRMING','RETRY_WAIT','OUTCOME_UNKNOWN')`, taskID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_delivery_operations
		SET status=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN 'CANCELLING' ELSE 'CANCELLED' END,
			reason_code=$2,updated_at=CURRENT_TIMESTAMP,
			finished_at=CASE WHEN status IN ('RUNNING','CONFIRMING') THEN finished_at ELSE CURRENT_TIMESTAMP END
		WHERE delivery_request_id IN (SELECT id FROM workforce_delivery_requests WHERE task_id=$1)
			AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED')`, taskID, reason); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_delivery_intents
		SET status='CANCELLED',version=version+1,closed_at=COALESCE(closed_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP
		WHERE task_id=$1 AND status NOT IN ('DELIVERED','SKIPPED','CANCELLED')`, taskID); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT tenant_id,task_turn_id FROM workforce_delivery_intents WHERE task_id=$1`, taskID)
	if err != nil {
		return err
	}
	var projections [][2]int64
	for rows.Next() {
		var tenantID, turnID int64
		if err = rows.Scan(&tenantID, &turnID); err != nil {
			rows.Close()
			return err
		}
		projections = append(projections, [2]int64{tenantID, turnID})
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, projection := range projections {
		if err = s.projectDelivery(ctx, tx, projection[0], projection[1]); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileRetries publishes the next immutable DeliveryAttempt only after the
// prior attempt proved that the remote ref remained unchanged. OUTCOME_UNKNOWN
// is deliberately excluded: it requires read-only arbitration first.
func (s *Store) ReconcileRetries(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 128 {
		limit = 32
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT r.id FROM workforce_delivery_requests r JOIN workforce_delivery_attempts a ON a.request_id=r.id WHERE r.status='RETRY_WAIT' AND a.status='RETRY_WAIT' AND a.next_retry_at<=CURRENT_TIMESTAMP AND a.attempt_number=(SELECT MAX(x.attempt_number) FROM workforce_delivery_attempts x WHERE x.request_id=r.id) ORDER BY a.next_retry_at FOR UPDATE OF r SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	idsToSchedule := make([]int64, 0, limit)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		idsToSchedule = append(idsToSchedule, id)
	}
	rows.Close()
	for _, id := range idsToSchedule {
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='READY',updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND status='RETRY_WAIT'`, id); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, id); err != nil {
			return 0, err
		}
		if err = s.schedule(ctx, tx, id); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(idsToSchedule), nil
}

// ReconcileUnknown creates a readback-only attempt. The Worker receives no
// path capable of issuing a push authorization until this observation proves
// the prior side effect did not occur.
func (s *Store) ReconcileUnknown(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 128 {
		limit = 32
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT r.id FROM workforce_delivery_requests r WHERE r.status='OUTCOME_UNKNOWN' AND NOT EXISTS (SELECT 1 FROM workforce_delivery_attempts a WHERE a.request_id=r.id AND a.status IN ('PENDING','RUNNING')) ORDER BY r.updated_at FOR UPDATE OF r SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	idsToSchedule := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		idsToSchedule = append(idsToSchedule, id)
	}
	rows.Close()
	for _, id := range idsToSchedule {
		if err = s.scheduleMode(ctx, tx, id, true); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(idsToSchedule), nil
}

// Reconcile schedules exactly one readback-only attempt for an uncertain
// delivery. It never issues a push authorization or creates a review.
func (s *Store) Reconcile(ctx context.Context, tenant, actor, requestID int64) (application.RequestView, error) {
	if tenant <= 0 || actor <= 0 || requestID <= 0 {
		return application.RequestView{}, application.ErrReconciliationNotAllowed
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var taskID int64
	var actionType string
	if err = tx.QueryRow(ctx, `SELECT status,task_id,action_type FROM workforce_delivery_requests WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, requestID, tenant).Scan(&status, &taskID, &actionType); errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, application.ErrNotFound
	} else if err != nil {
		return application.RequestView{}, err
	}
	if status == "CONFIRMING" {
		rows, readErr := tx.Query(ctx, `SELECT id,status,phase,lease_expires_at FROM workforce_delivery_attempts WHERE request_id=$1 AND status IN ('PENDING','RUNNING') ORDER BY id FOR UPDATE`, requestID)
		if readErr != nil {
			return application.RequestView{}, readErr
		}
		expired := []int64{}
		now := time.Now().UTC()
		for rows.Next() {
			var id int64
			var attemptStatus, phase string
			var expiry *time.Time
			if err = rows.Scan(&id, &attemptStatus, &phase, &expiry); err != nil {
				rows.Close()
				return application.RequestView{}, err
			}
			if !domain.CanReconcileExpiredPreparedPush(actionType, status, attemptStatus, phase, expiry, now) {
				rows.Close()
				return application.RequestView{}, application.ErrReconciliationNotAllowed
			}
			expired = append(expired, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return application.RequestView{}, err
		}
		if len(expired) == 0 {
			return application.RequestView{}, application.ErrReconciliationNotAllowed
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_attempts SET status='OUTCOME_UNKNOWN',reason_code='DELIVERY_LEASE_EXPIRED',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=ANY($1::bigint[])`, expired); err != nil {
			return application.RequestView{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='OUTCOME_UNKNOWN',reason_code='DELIVERY_LEASE_EXPIRED',version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID); err != nil {
			return application.RequestView{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='OUTCOME_UNKNOWN',reason_code='DELIVERY_LEASE_EXPIRED',updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, requestID); err != nil {
			return application.RequestView{}, err
		}
		if _, err = eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.prepared_lease_expired", map[string]any{"deliveryRequestId": requestID, "expiredAttemptIds": expired, "actorUserId": actor, "mode": "READBACK_ONLY"}); err != nil {
			return application.RequestView{}, err
		}
		status = "OUTCOME_UNKNOWN"
	}
	if status != "OUTCOME_UNKNOWN" && status != "MANUAL_REVIEW" {
		return application.RequestView{}, application.ErrReconciliationNotAllowed
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_delivery_attempts WHERE request_id=$1 AND status IN ('PENDING','RUNNING'))`, requestID).Scan(&active); err != nil {
		return application.RequestView{}, err
	}
	if active {
		return application.RequestView{}, application.ErrReconciliationNotAllowed
	}
	if err = s.scheduleMode(ctx, tx, requestID, true); err != nil {
		return application.RequestView{}, err
	}
	if _, err = eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.reconcile_requested", map[string]any{"deliveryRequestId": requestID, "actorUserId": actor, "mode": "READBACK_ONLY"}); err != nil {
		return application.RequestView{}, err
	}
	if err = s.projectDeliveryRequest(ctx, tx, requestID); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	return s.getRequest(ctx, tenant, requestID)
}

// ReconcileResultActions converts persisted PASS actions into DeliveryRequest
// records only after the trustworthy result and format-2 Checkpoint are both
// available. The deterministic idempotency key makes the reconciliation safe
// across Server restarts and multiple loops.
func (s *Store) ReconcileResultActions(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 128 {
		limit = 32
	}
	// Keep the candidate read within the governance table-width limit. The
	// immutable Git plan is loaded below per candidate, so reconciliation does
	// not turn into a four-table join merely to learn its delivery mode.
	rows, err := s.db.Query(ctx, `SELECT t.tenant_id,t.id,e.id,e.contract_json,tr.id
		FROM workforce_tasks t
		JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id
		JOIN workforce_result_evaluation_cycles e ON e.task_turn_id=tr.id AND e.decision='PASS' AND e.status='PERSISTED'
		WHERE t.status='SUCCEEDED'
		ORDER BY e.id DESC LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type candidate struct {
		tenant       int64
		task         int64
		cycle        int64
		turn         int64
		deliveryMode string
		body         []byte
	}
	items := make([]candidate, 0, limit)
	for rows.Next() {
		var item candidate
		if err = rows.Scan(&item.tenant, &item.task, &item.cycle, &item.body, &item.turn); err != nil {
			return 0, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	created := 0
	for _, item := range items {
		if err = s.db.QueryRow(ctx, `SELECT COALESCE(delivery_mode,'NONE') FROM workforce_task_git_plans WHERE task_id=$1 AND task_turn_id=$2 AND status='ACTIVE' ORDER BY created_at DESC LIMIT 1`, item.task, item.turn).Scan(&item.deliveryMode); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				item.deliveryMode = "NONE"
			} else {
				return created, err
			}
		}
		var checkpointReady bool
		if err = s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM workforce_continuation_checkpoints WHERE task_turn_id=$1 AND status='AVAILABLE' AND format_version=2)`, item.turn).Scan(&checkpointReady); err != nil {
			return created, err
		}
		if !checkpointReady {
			continue
		}
		var contract struct {
			Actions []domain.Action `json:"actions"`
		}
		if json.Unmarshal(item.body, &contract) != nil {
			continue
		}
		for index, action := range actionsForFrozenGitPlan(contract.Actions, item.deliveryMode) {
			if action.Type != domain.ActionGitPush && action.Type != domain.ActionCustomDelivery {
				continue
			}
			key := fmt.Sprintf("result-action:%d:%d", item.cycle, index)
			_, replay, createErr := s.CreateRequest(ctx, application.CreateRequest{TaskID: item.task, TenantID: item.tenant, ActorServiceCode: "result-delivery", IdempotencyKey: key, Action: action})
			if createErr != nil {
				// A persisted PASS action is durable business intent. Silently
				// skipping a failed request creation leaves the Task successful
				// while making its required approval impossible to discover or
				// retry. Return the failure to the scheduled caller instead.
				return created, createErr
			}
			if !replay {
				created++
			}
		}
	}
	return created, nil
}

// actionsForFrozenGitPlan preserves a Processor's explicit delivery choices.
// When a user has selected a delivery plan in Task Create, however, the plan
// itself is the durable intent to deliver the independently verified result.
// The default Result Processor intentionally contains no Git-specific logic,
// so it needs this narrow derived action. CreateRequest subsequently binds the
// empty target/base/branch fields to the frozen TaskTurn plan and rejects any
// drift; this helper never grants an arbitrary remote or refspec.
func actionsForFrozenGitPlan(actions []domain.Action, deliveryMode string) []domain.Action {
	result := append([]domain.Action(nil), actions...)
	for _, action := range result {
		if action.Type == domain.ActionGitPush || action.Type == domain.ActionCustomDelivery {
			return result
		}
	}
	if deliveryMode != "NEW_BRANCH" && deliveryMode != "NEW_BRANCH_AND_DRAFT_REVIEW" {
		return result
	}
	return append(result, domain.Action{Type: domain.ActionGitPush, When: domain.ApprovalAfter})
}
func (s *Store) Retry(ctx context.Context, tenant int64, actor int64, id int64) (application.RequestView, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM workforce_delivery_requests WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, id, tenant).Scan(&status); err != nil {
		return application.RequestView{}, err
	}
	if status != "FAILED" && status != "OUTCOME_UNKNOWN" && status != "MANUAL_REVIEW" {
		return application.RequestView{}, domain.ErrIdentityConflict
	}
	if status == "OUTCOME_UNKNOWN" {
		return application.RequestView{}, errors.New("DELIVERY_READBACK_REQUIRED")
	}
	// Automatic delivery retries are deliberately bounded.  An operator can
	// explicitly reauthorize a request after a credential or platform repair;
	// that starts a new, separately audited delivery-authority epoch without
	// ever re-running the Agent or changing the frozen result.  A plain retry
	// must not pretend it scheduled a fourth automatic attempt.
	epochAttempts, err := deliveryRetryEpochAttemptCount(ctx, tx, id)
	if err != nil {
		return application.RequestView{}, err
	}
	if epochAttempts >= 3 {
		return application.RequestView{}, domain.ErrIdentityConflict
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='READY',reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id)
	if err != nil {
		return application.RequestView{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='READY',reason_code='',finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, id); err != nil {
		return application.RequestView{}, err
	}
	if err = s.schedule(ctx, tx, id); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	_ = actor
	return s.getRequest(ctx, tenant, id)
}

func (s *Store) ReauthorizeRequest(ctx context.Context, request application.Reauthorization) (application.RequestView, error) {
	if request.TenantID <= 0 || request.Actor <= 0 || request.RequestID <= 0 || request.ProjectVersionID < 0 || strings.TrimSpace(request.Reason) == "" || len([]byte(request.Reason)) > 2048 {
		return application.RequestView{}, domain.ErrReauthorization
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	var taskID, turnID, projectID, planProjectID int64
	var action, status, reason, taskStatus, targetDigest, planTargetDigest, authorizationDigest, planAuthorizationDigest string
	var currentResult, active, available bool
	var previousBinding *int64
	var previousCredentialVersion int64
	err = tx.QueryRow(ctx, `SELECT r.task_id,r.task_turn_id,r.project_id,r.action_type,r.status,r.reason_code,t.status,
		t.current_task_turn_id=r.task_turn_id,r.target_config_digest,p.project_id,p.target_config_digest,
		r.authorization_digest,p.push_authorization_digest,r.reauthorized_project_version_id
		FROM workforce_delivery_requests r
		JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
		JOIN workforce_task_git_plans p ON p.task_id=r.task_id AND p.task_turn_id=r.task_turn_id AND p.tenant_id=r.tenant_id AND p.status='ACTIVE'
		WHERE r.id=$1 AND r.tenant_id=$2 FOR UPDATE OF r,t`, request.RequestID, request.TenantID).Scan(&taskID, &turnID, &projectID, &action, &status, &reason, &taskStatus, &currentResult, &targetDigest, &planProjectID, &planTargetDigest, &authorizationDigest, &planAuthorizationDigest, &previousBinding)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, application.ErrNotFound
	}
	if err != nil {
		return application.RequestView{}, err
	}
	// Keep the request/task locks while checking the bounded recovery dependencies.
	err = tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM workforce_delivery_attempts a WHERE a.request_id=r.id AND a.status IN ('PENDING','RUNNING')),
		EXISTS(SELECT 1 FROM workforce_continuation_checkpoints c WHERE c.id=r.checkpoint_id AND c.task_id=r.task_id AND c.task_turn_id=r.task_turn_id AND c.status='AVAILABLE' AND c.result_commit=r.result_commit)
		FROM workforce_delivery_requests r WHERE r.id=$1 AND r.tenant_id=$2`, request.RequestID, request.TenantID).Scan(&active, &available)
	if err != nil {
		return application.RequestView{}, err
	}
	if previousBinding != nil {
		plan, planErr := s.loadDeliveryRepositoryPlan(ctx, request.TenantID, taskID, turnID, request.RequestID)
		if planErr != nil {
			return application.RequestView{}, planErr
		}
		planTargetDigest, planAuthorizationDigest = plan.TargetConfigDigest, plan.PushAuthorizationDigest
		previousCredentialVersion = plan.Credential.Version
	}
	epochAttempts, err := deliveryRetryEpochAttemptCount(ctx, tx, request.RequestID)
	if err != nil {
		return application.RequestView{}, err
	}
	if !domain.CanReauthorizeExhaustedPush(action, status, reason, epochAttempts, active) || taskStatus != "SUCCEEDED" || !currentResult || !available || projectID != planProjectID || targetDigest == "" || targetDigest != planTargetDigest || authorizationDigest == "" || authorizationDigest != planAuthorizationDigest {
		return application.RequestView{}, domain.ErrReauthorization
	}
	var replacement *frozenRepositoryPlan
	if request.ProjectVersionID != 0 {
		if s.repositoryBindings == nil {
			return application.RequestView{}, domain.ErrReauthorization
		}
		binding, bindingErr := s.repositoryBindings.DeliveryRepositoryBinding(ctx, request.TenantID, projectID, request.ProjectVersionID)
		if bindingErr != nil || !binding.Current || binding.CredentialVersion < previousCredentialVersion {
			return application.RequestView{}, domain.ErrReauthorization
		}
		original, planErr := s.loadFrozenRepositoryPlan(ctx, request.TenantID, taskID, turnID)
		if planErr != nil {
			return application.RequestView{}, planErr
		}
		effective, bindErr := bindDeliveryRepository(original, binding)
		if bindErr != nil {
			return application.RequestView{}, bindErr
		}
		replacement = &effective
	}
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, request.Actor)
	if err != nil {
		return application.RequestView{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='WAITING_APPROVAL',reason_code='',version=version+1,
		approved_by_principal_id=NULL,approved_at=NULL,finished_at=NULL,
		reauthorized_by_principal_id=$2,reauthorized_at=CURRENT_TIMESTAMP,reauthorization_reason=$3,updated_at=CURRENT_TIMESTAMP
		WHERE id=$1`, request.RequestID, principalID, strings.TrimSpace(request.Reason)); err != nil {
		return application.RequestView{}, err
	}
	if replacement != nil {
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET reauthorized_project_version_id=$2,target_config_digest=$3,credential_version=$4,authorization_digest=$5 WHERE id=$1`, request.RequestID, request.ProjectVersionID, replacement.TargetConfigDigest, replacement.credentialVersion(), replacement.PushAuthorizationDigest); err != nil {
			return application.RequestView{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET authorization_version=$2,authorization_digest=CASE WHEN operation_kind='PUSH_BRANCH' THEN $3 ELSE $4 END WHERE delivery_request_id=$1 AND operation_kind IN ('PUSH_BRANCH','CREATE_PULL_REQUEST') AND status<>'SUCCEEDED'`, request.RequestID, replacement.credentialVersion(), replacement.PushAuthorizationDigest, replacement.ReviewAuthorizationDigest); err != nil {
			return application.RequestView{}, err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='WAITING_APPROVAL',reason_code='',finished_at=NULL,next_retry_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, request.RequestID); err != nil {
		return application.RequestView{}, err
	}
	if _, err = eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.request_reauthorized", map[string]any{"deliveryRequestId": request.RequestID, "taskTurnId": turnID, "actorUserId": request.Actor, "previousReasonCode": reason, "mode": "READBACK_AFTER_APPROVAL", "projectVersionId": request.ProjectVersionID}); err != nil {
		return application.RequestView{}, err
	}
	if err = s.recordDeliveryWaitingApproval(ctx, tx, request.TenantID, taskID, turnID, request.RequestID); err != nil {
		return application.RequestView{}, err
	}
	if err = s.projectDeliveryRequest(ctx, tx, request.RequestID); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	return s.getRequest(ctx, request.TenantID, request.RequestID)
}

// ReauthorizeReview is deliberately narrower than Delivery reauthorization:
// once the DeliveryCommit is on the remote, a review-token replacement cannot
// reopen or re-push the branch.  It only moves the frozen review operation
// back to an explicit approval gate.
func (s *Store) ReauthorizeReview(ctx context.Context, request application.Reauthorization) (application.RequestView, error) {
	if request.TenantID <= 0 || request.Actor <= 0 || request.OperationID <= 0 {
		return application.RequestView{}, domain.ErrReauthorization
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.RequestView{}, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, request.Actor)
	if err != nil {
		return application.RequestView{}, err
	}
	var requestID int64
	var operationStatus, requestStatus string
	var attemptCount int
	err = tx.QueryRow(ctx, `SELECT o.delivery_request_id,o.status,o.attempt_count,r.status
		FROM workforce_delivery_operations o
		JOIN workforce_delivery_requests r ON r.id=o.delivery_request_id AND r.tenant_id=o.tenant_id
		WHERE o.id=$1 AND o.tenant_id=$2 AND o.operation_kind='CREATE_PULL_REQUEST'
		FOR UPDATE OF o,r`, request.OperationID, request.TenantID).Scan(&requestID, &operationStatus, &attemptCount, &requestStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.RequestView{}, application.ErrNotFound
	}
	if err != nil {
		return application.RequestView{}, err
	}
	if requestStatus != "SUCCEEDED" || operationStatus != "MANUAL_REVIEW" || attemptCount >= 3 {
		return application.RequestView{}, domain.ErrReauthorization
	}
	command, err := tx.Exec(ctx, `UPDATE workforce_pull_request_publications
		SET status='PENDING',reason_code='',updated_at=CURRENT_TIMESTAMP
		WHERE delivery_operation_id=$1 AND status='MANUAL_REVIEW'`, request.OperationID)
	if err != nil || command.RowsAffected() != 1 {
		if err != nil {
			return application.RequestView{}, err
		}
		return application.RequestView{}, domain.ErrReauthorization
	}
	command, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations
		SET status='READY',reason_code='',finished_at=NULL,next_retry_at=NULL,
			reauthorized_by_principal_id=$2,reauthorized_at=CURRENT_TIMESTAMP,reauthorization_reason=$3,
			updated_at=CURRENT_TIMESTAMP
		WHERE id=$1 AND status='MANUAL_REVIEW'`, request.OperationID, principalID, strings.TrimSpace(request.Reason))
	if err != nil || command.RowsAffected() != 1 {
		if err != nil {
			return application.RequestView{}, err
		}
		return application.RequestView{}, domain.ErrReauthorization
	}
	var taskID int64
	if err = tx.QueryRow(ctx, `SELECT task_id FROM workforce_delivery_requests WHERE id=$1`, requestID).Scan(&taskID); err != nil {
		return application.RequestView{}, err
	}
	if _, err = eventlog.AppendWithSequence(ctx, tx, taskID, 0, "delivery.review_reauthorized", map[string]any{"deliveryRequestId": requestID, "reviewOperationId": request.OperationID, "actorUserId": request.Actor}); err != nil {
		return application.RequestView{}, err
	}
	if err = s.projectDeliveryRequest(ctx, tx, requestID); err != nil {
		return application.RequestView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.RequestView{}, err
	}
	return s.getRequest(ctx, request.TenantID, requestID)
}

func (s *Store) schedule(ctx context.Context, tx pgx.Tx, requestID int64) error {
	return s.scheduleMode(ctx, tx, requestID, false)
}

func (s *Store) scheduleMode(ctx context.Context, tx pgx.Tx, requestID int64, reconciliationOnly bool) error {
	var actionType, transport string
	if err := tx.QueryRow(ctx, `SELECT r.action_type,UPPER(p.repository_scheme) FROM workforce_delivery_requests r JOIN workforce_task_git_plans p ON p.task_id=r.task_id AND p.task_turn_id=r.task_turn_id AND p.tenant_id=r.tenant_id AND p.status='ACTIVE' WHERE r.id=$1 AND r.project_id=p.project_id`, requestID).Scan(&actionType, &transport); err != nil {
		return err
	}
	if transport == "HTTP" || transport == "HTTPS" {
		transport = domain.TransportHTTPS
	}
	var worker int64
	var instance, boot string
	var err error
	switch {
	case actionType == domain.ActionCustomDelivery && transport != domain.TransportLocalBare:
		var harnessDigest string
		if harnessDigest, err = deliveryHarnessDigestTx(ctx, tx, requestID); err != nil {
			return err
		}
		if strings.TrimSpace(harnessDigest) == "" {
			return domain.ErrIdentityConflict
		}
		// Host-wide self-check evidence proves the Worker can enforce the Docker
		// topology, while the inventory join proves the *exact frozen Harness*
		// also declared the immutable gateway command.  A different image on the
		// same Worker must never make a Custom Delivery eligible by accident.
		err = tx.QueryRow(ctx, `SELECT w.id,w.current_instance_id,s.host_boot_id
FROM workforce_workers w
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
JOIN workforce_worker_images i ON i.worker_id=w.id AND i.instance_id=s.instance_id
WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL
  AND s.ended_at IS NULL AND s.last_heartbeat_at>CURRENT_TIMESTAMP-INTERVAL '35 seconds'
  AND s.consumer_state='CONSUMING'
  AND s.capabilities_json @> '{"customDelivery":true,"deliveryEgressEnforced":true}'::jsonb
  AND i.role='attempt' AND i.image_digest=$1
  AND i.admission_status='ADMITTED' AND i.presence_status='PRESENT'
  AND i.capabilities_json @> '{"capabilities":[{"name":"delivery.egress-gateway","status":"supported"}]}'::jsonb
ORDER BY s.last_heartbeat_at DESC,w.id LIMIT 1`, harnessDigest).Scan(&worker, &instance, &boot)
	case actionType == domain.ActionCustomDelivery:
		err = tx.QueryRow(ctx, `SELECT w.id,w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.last_heartbeat_at>CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.capabilities_json @> '{"customDelivery":true}'::jsonb ORDER BY s.last_heartbeat_at DESC,w.id LIMIT 1`).Scan(&worker, &instance, &boot)
	default:
		err = tx.QueryRow(ctx, `SELECT w.id,w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.last_heartbeat_at>CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.capabilities_json @> '{"gitDelivery":true}'::jsonb ORDER BY s.last_heartbeat_at DESC,w.id LIMIT 1`).Scan(&worker, &instance, &boot)
	}
	if err != nil {
		return err
	}
	var number int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(attempt_number),0)+1 FROM workforce_delivery_attempts WHERE request_id=$1`, requestID).Scan(&number); err != nil {
		return err
	}
	epochAttempts, err := deliveryRetryEpochAttemptCount(ctx, tx, requestID)
	if err != nil {
		return err
	}
	if epochAttempts >= 3 {
		status, reason := "FAILED", "DELIVERY_RETRY_EXHAUSTED"
		if reconciliationOnly {
			status, reason = "MANUAL_REVIEW", "DELIVERY_READBACK_RETRY_EXHAUSTED"
		}
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status=$2,reason_code=$3,finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID, status, reason)
		if err == nil {
			err = s.recordDeliveryAttention(ctx, tx, requestID, status, reason)
		}
		return err
	}
	attemptID, messageID := s.idGen.NextID(), s.idGen.NextID()
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_attempts (id,request_id,attempt_number,worker_id,worker_instance_id,host_boot_id,status,reconciliation_only) VALUES ($1,$2,$3,$4,$5,$6,'PENDING',$7)`, attemptID, requestID, number, worker, instance, boot, reconciliationOnly)
	if err != nil {
		return err
	}
	message := domain.Message{SchemaVersion: 1, Kind: domain.MessageKind, MessageID: messageID, WorkerID: worker, ResourceID: attemptID, Generation: 1}
	payload, _ := json.Marshal(message)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'delivery_attempt',$2,'delivery.attempt.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, attemptID, payload, fmt.Sprintf("postprocess.%d", worker))
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_requests SET status='DISPATCHED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, requestID)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE workforce_delivery_operations SET status='DISPATCHED',attempt_count=$2,updated_at=CURRENT_TIMESTAMP WHERE delivery_request_id=$1 AND operation_kind='PUSH_BRANCH'`, requestID, number)
	}
	return err
}

// deliveryRetryEpochAttemptCount leaves the immutable attempt history intact
// while counting the bounded automatic retry budget from the most recent
// explicit reauthorization.  Reauthorization is an operator action with a
// fresh approval gate; it is not an implicit extension of the retry policy.
func deliveryRetryEpochAttemptCount(ctx context.Context, tx pgx.Tx, requestID int64) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT COUNT(*)
FROM workforce_delivery_attempts a
JOIN workforce_delivery_requests r ON r.id=a.request_id
WHERE a.request_id=$1
  AND a.created_at>=COALESCE(r.reauthorized_at,r.created_at)`, requestID).Scan(&count)
	return count, err
}

type rowScanner interface{ Scan(...any) error }

const requestSelect = `SELECT id,version,task_id,task_turn_id,checkpoint_id,action_type,project_id,base_branch,destination_branch,result_commit,result_tree,selection_digest,delivery_commit,approval_mode,COALESCE(approved_by_principal_id,0),COALESCE(to_char(approved_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),status,reason_code,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_delivery_requests`

func scanRequest(row rowScanner) (application.RequestView, error) {
	var v application.RequestView
	err := row.Scan(&v.ID, &v.Version, &v.TaskID, &v.TaskTurnID, &v.CheckpointID, &v.ActionType, &v.ProjectID, &v.BaseBranch, &v.DestinationBranch, &v.ResultCommit, &v.ResultTree, &v.SelectionDigest, &v.DeliveryCommit, &v.ApprovalMode, &v.ApprovedBy, &v.ApprovedAt, &v.Status, &v.ReasonCode, &v.CreatedAt)
	return v, err
}
func (s *Store) getRequest(ctx context.Context, tenant int64, id int64) (application.RequestView, error) {
	v, err := scanRequest(s.db.QueryRow(ctx, requestSelect+` WHERE tenant_id=$1 AND id=$2`, tenant, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, application.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	return v, s.hydrateReview(ctx, &v)
}

func (s *Store) hydrateReview(ctx context.Context, request *application.RequestView) error {
	var view application.ReviewView
	err := s.db.QueryRow(ctx, `SELECT o.id,COALESCE(p.status,o.status),COALESCE(p.provider_type,''),COALESCE(p.provider_number,''),COALESCE(p.web_url,''),COALESCE(NULLIF(p.reason_code,''),o.reason_code,'') FROM workforce_delivery_operations o LEFT JOIN workforce_pull_request_publications p ON p.delivery_operation_id=o.id WHERE o.delivery_request_id=$1 AND o.operation_kind='CREATE_PULL_REQUEST'`, request.ID).Scan(&view.OperationID, &view.Status, &view.ProviderType, &view.ProviderNumber, &view.WebURL, &view.ReasonCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	request.Review = &view
	return nil
}
func findRequestByKey(ctx context.Context, tx pgx.Tx, tenant int64, key string) (application.RequestView, string, error) {
	v, err := scanRequest(tx.QueryRow(ctx, requestSelect+` WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, key))
	if err != nil {
		return v, "", err
	}
	var digest string
	err = tx.QueryRow(ctx, `SELECT request_digest FROM workforce_delivery_requests WHERE id=$1`, v.ID).Scan(&digest)
	return v, digest, err
}

var _ = fmt.Sprint
