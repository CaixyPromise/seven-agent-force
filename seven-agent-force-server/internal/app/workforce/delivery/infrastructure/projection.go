package infrastructure

import (
	"context"
	"errors"
	"time"

	delivery "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	"github.com/jackc/pgx/v5"
)

func (s *Store) projectDelivery(ctx context.Context, tx pgx.Tx, tenant, turn int64) error {
	if s.conversationProjector == nil {
		return nil
	}
	p := delivery.Projection{AllowedActions: []string{}}
	var requestID, requestVersion int64
	var requestStatus, reason, resultCommit, deliveryCommit, reviewStatus, reviewURL string
	err := tx.QueryRow(ctx, `SELECT i.id,i.version,i.tenant_id,i.task_id,i.task_turn_id,i.attempt_id,i.summary,i.delivery_mode,i.delivery_authorization,
		COALESCE(i.delivery_request_id,0),p.result_branch,p.repository_display_url,i.created_at
		FROM workforce_delivery_intents i
		JOIN workforce_task_git_plans p ON p.task_turn_id=i.task_turn_id AND p.tenant_id=i.tenant_id
		WHERE i.tenant_id=$1 AND i.task_turn_id=$2`, tenant, turn).Scan(
		&p.ID, &p.Version, &p.TenantID, &p.TaskID, &p.TaskTurnID, &p.AttemptID, &p.Summary, &p.DeliveryMode, &p.Authorization,
		&requestID, &p.DestinationBranch, &p.RepositoryName, &p.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	p.IntentID, p.RequestID = p.ID, requestID
	if requestID > 0 {
		if err = tx.QueryRow(ctx, `SELECT version,status,reason_code,result_commit,delivery_commit FROM workforce_delivery_requests WHERE id=$1 AND tenant_id=$2`, requestID, tenant).Scan(&requestVersion, &requestStatus, &reason, &resultCommit, &deliveryCommit); err != nil {
			return err
		}
		if p.DeliveryMode == "NEW_BRANCH_AND_DRAFT_REVIEW" {
			if err = tx.QueryRow(ctx, `SELECT COALESCE(o.status,''),COALESCE(pub.web_url,'') FROM workforce_delivery_operations o LEFT JOIN workforce_pull_request_publications pub ON pub.delivery_operation_id=o.id WHERE o.delivery_request_id=$1 AND o.operation_kind='CREATE_PULL_REQUEST'`, requestID).Scan(&reviewStatus, &reviewURL); err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
	}
	p.IntentVersion, p.RequestVersion = p.Version, requestVersion
	p.ResultCommit, p.DeliveryCommit, p.PullRequestURL, p.ReasonCode = resultCommit, deliveryCommit, reviewURL, reason
	if requestVersion > p.Version {
		p.Version = requestVersion
	}
	p.Status = "WAITING_FINALIZATION"
	switch requestStatus {
	case "WAITING_APPROVAL":
		p.Status, p.AttentionRequired, p.AllowedActions = "WAITING_USER_DECISION", true, []string{"DELIVER", "SKIP_THIS_RESULT"}
	case "READY", "DISPATCHED":
		p.Status = "AUTO_DELIVERY_SCHEDULED"
	case "RUNNING":
		p.Status = "PUSHING"
	case "CONFIRMING":
		p.Status = "VERIFYING_BRANCH"
	case "OUTCOME_UNKNOWN":
		p.Status = "OUTCOME_UNKNOWN"
	case "FAILED", "MANUAL_REVIEW":
		p.Status = "DELIVERY_FAILED"
	case "CANCELLED":
		if reason == "USER_SKIPPED" {
			p.Status, p.AllowedActions = "SKIPPED", []string{"RESTART_DELIVERY"}
		} else {
			p.Status = "CANCELLED"
		}
	case "SUCCEEDED":
		p.RemoteReadbackVerified = true
		if p.DeliveryMode != "NEW_BRANCH_AND_DRAFT_REVIEW" || reviewStatus == "SUCCEEDED" {
			p.Status = "DELIVERED"
		} else {
			switch reviewStatus {
			case "OUTCOME_UNKNOWN":
				p.Status = "OUTCOME_UNKNOWN"
			case "FAILED", "MANUAL_REVIEW", "CANCELLED":
				p.Status = "DELIVERY_FAILED"
			case "CONFIRMING":
				p.Status = "VERIFYING_REVIEW"
			default:
				p.Status = "CREATING_REVIEW"
			}
		}
	}
	if p.OccurredAt.IsZero() {
		p.OccurredAt = time.Now().UTC()
	}
	return s.conversationProjector.ProjectDeliveryTx(ctx, tx, p)
}

func (s *Store) projectDeliveryRequest(ctx context.Context, tx pgx.Tx, requestID int64) error {
	var tenant, turn int64
	if err := tx.QueryRow(ctx, `SELECT tenant_id,task_turn_id FROM workforce_delivery_requests WHERE id=$1`, requestID).Scan(&tenant, &turn); err != nil {
		return err
	}
	if err := syncDeliveryIntentStatus(ctx, tx, requestID); err != nil {
		return err
	}
	return s.projectDelivery(ctx, tx, tenant, turn)
}

// deliveryIntentStatus derives the user-facing intent lifecycle from the
// authoritative DeliveryRequest and, for Draft PR delivery, its required
// publication operation. An unknown remote outcome is deliberately retained
// as its own state: it must never be collapsed into success or retried merely
// because reconciliation was invoked again.
func deliveryIntentStatus(requestStatus, requestReason, deliveryMode, reviewStatus string) string {
	switch requestStatus {
	case "WAITING_APPROVAL", "MANUAL_ONLY":
		return "WAITING_USER_DECISION"
	case "READY", "DISPATCHED", "RUNNING", "CONFIRMING", "RETRY_WAIT", "CANCELLING":
		return "DELIVERY_RUNNING"
	case "OUTCOME_UNKNOWN":
		return "OUTCOME_UNKNOWN"
	case "FAILED", "MANUAL_REVIEW", "DENIED", "EXPIRED":
		return "DELIVERY_FAILED"
	case "CANCELLED":
		if requestReason == "USER_SKIPPED" {
			return "SKIPPED"
		}
		return "CANCELLED"
	case "SUCCEEDED":
		if deliveryMode != "NEW_BRANCH_AND_DRAFT_REVIEW" {
			return "DELIVERED"
		}
		switch reviewStatus {
		case "SUCCEEDED":
			return "DELIVERED"
		case "OUTCOME_UNKNOWN":
			return "OUTCOME_UNKNOWN"
		case "FAILED", "MANUAL_REVIEW", "CANCELLED", "DENIED", "EXPIRED":
			return "DELIVERY_FAILED"
		default:
			return "DELIVERY_RUNNING"
		}
	default:
		return "WAITING_FINALIZATION"
	}
}

func syncDeliveryIntentStatus(ctx context.Context, tx pgx.Tx, requestID int64) error {
	var requestStatus, requestReason, deliveryMode, reviewStatus string
	err := tx.QueryRow(ctx, `SELECT r.status,r.reason_code,i.delivery_mode,
		COALESCE((SELECT o.status FROM workforce_delivery_operations o
			WHERE o.delivery_request_id=r.id AND o.operation_kind='CREATE_PULL_REQUEST'
			ORDER BY o.id DESC LIMIT 1),'')
		FROM workforce_delivery_requests r
		JOIN workforce_delivery_intents i ON i.delivery_request_id=r.id
		WHERE r.id=$1
		FOR UPDATE OF r,i`, requestID).Scan(&requestStatus, &requestReason, &deliveryMode, &reviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	target := deliveryIntentStatus(requestStatus, requestReason, deliveryMode, reviewStatus)
	_, err = tx.Exec(ctx, `UPDATE workforce_delivery_intents
		SET status=$2,version=version+1,updated_at=CURRENT_TIMESTAMP
		WHERE delivery_request_id=$1 AND status<>$2`, requestID, target)
	return err
}
