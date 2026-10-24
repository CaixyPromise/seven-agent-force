package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

// ListDeliverySummaries returns the deliberately content-free delivery list.
// Scope ownership is established in SQL before pagination so another scope
// cannot influence the caller's page count or result ordering.
func (r *Repository) ListDeliverySummaries(ctx context.Context, query domain.DeliveryQuery) ([]domain.DeliverySummary, int64, error) {
	where := []string{"isDeleted=0"}
	args := make([]any, 0, 8)
	scopePredicate, scopeArgs := deliveryDiagnosticScopePredicate(strings.TrimSpace(query.ScopeID))
	where = append(where, scopePredicate)
	args = append(args, scopeArgs...)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		where = append(where, "(deliveryId LIKE ? OR targetMasked LIKE ? OR traceId LIKE ?)")
		keyword = "%" + keyword + "%"
		args = append(args, keyword, keyword, keyword)
	}
	if sceneCode := strings.TrimSpace(query.SceneCode); sceneCode != "" {
		where = append(where, "sceneCode=?")
		args = append(args, sceneCode)
	}
	if channelCode := strings.TrimSpace(query.ChannelCode); channelCode != "" {
		where = append(where, "channelCode=?")
		args = append(args, channelCode)
	}
	if status := strings.TrimSpace(query.Status); status != "" {
		where = append(where, "status=?")
		args = append(args, status)
	}
	return selectInboxPage[domain.DeliverySummary](ctx, r, deliverySummarySelectBase(r), where, "createTime DESC, id DESC", query.Current, query.PageSize, args...)
}

// FindDeliveryForDiagnostic obtains one scope-owned delivery. It selects the
// minimum rendered fields needed for a later, separately authorized content
// read and never reads payload JSON, raw target, provider response or secret
// material into the diagnostic path.
func (r *Repository) FindDeliveryForDiagnostic(ctx context.Context, scopeID, deliveryID string) (*domain.Delivery, error) {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return nil, nil
	}
	scopePredicate, scopeArgs := deliveryDiagnosticScopePredicate(strings.TrimSpace(scopeID))
	query := deliveryDiagnosticSelectBase(r) + " WHERE deliveryId=? AND isDeleted=0 AND " + scopePredicate + " LIMIT 1"
	args := append([]any{deliveryID}, scopeArgs...)
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var item domain.Delivery
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// FindDeliveryEphemeralContent reads only the encrypted, TTL-bound content
// envelope for a delivery already constrained to the current scope.
func (r *Repository) FindDeliveryEphemeralContent(ctx context.Context, scopeID, deliveryID string) (*domain.DeliveryEphemeralContent, error) {
	if strings.TrimSpace(scopeID) == "" || strings.TrimSpace(deliveryID) == "" {
		return nil, nil
	}
	query := `SELECT id, deliveryId, scopeId, ciphertext, edek, wrapKeyRef, expiresAt, createTime, updateTime
FROM sys_notification_delivery_ephemeral_content
WHERE scopeId=? AND deliveryId=? LIMIT 1`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var item domain.DeliveryEphemeralContent
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(scopeID), strings.TrimSpace(deliveryID)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// InsertDeliveryEphemeralContent persists the only encrypted rendering for a
// SECRET_EPHEMERAL delivery. Duplicate writes never replace an accepted
// envelope, preserving idempotency and the original expiry.
func (r *Repository) InsertDeliveryEphemeralContent(ctx context.Context, item *domain.DeliveryEphemeralContent) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.DeliveryID) == "" || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.Ciphertext) == "" || strings.TrimSpace(item.EDEK) == "" || strings.TrimSpace(item.WrapKeyRef) == "" || item.ExpiresAt.IsZero() {
		return fmt.Errorf("notification delivery ephemeral content is invalid")
	}
	query := `INSERT INTO sys_notification_delivery_ephemeral_content (id, deliveryId, scopeId, ciphertext, edek, wrapKeyRef, expiresAt)
VALUES (?, ?, ?, ?, ?, ?, ?)`
	if r.isPostgres() {
		query += ` ON CONFLICT (deliveryId) DO NOTHING`
	} else {
		query += ` ON DUPLICATE KEY UPDATE id=id`
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.DeliveryID, item.ScopeID, item.Ciphertext, item.EDEK, item.WrapKeyRef, item.ExpiresAt.UTC())
	return err
}

// InsertDeliveryDiagnosticAudit saves the content-free audit trail for every
// allowed and denied single-delivery diagnostic attempt.
func (r *Repository) InsertDeliveryDiagnosticAudit(ctx context.Context, item *domain.DeliveryDiagnosticAudit) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.DeliveryID) == "" || item.ActorID <= 0 || strings.TrimSpace(item.ContentTier) == "" || strings.TrimSpace(item.ReasonCode) == "" || strings.TrimSpace(item.ResultCode) == "" {
		return fmt.Errorf("notification delivery diagnostic audit is invalid")
	}
	query := `INSERT INTO sys_notification_delivery_diagnostic_audit (id, scopeId, deliveryId, actorId, contentTier, reasonCode, ticketReference, resultCode, traceId)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.ScopeID, item.DeliveryID, item.ActorID, item.ContentTier, item.ReasonCode, nullIfBlank(item.TicketReference), item.ResultCode, nullIfBlank(item.TraceID))
	return err
}

func deliverySummarySelectBase(r *Repository) string {
	return `SELECT id, deliveryId, sceneCode, channelCode, channelType, templateCode, COALESCE(targetMasked, '') targetMasked,
status, retryCount, maxRetry, nextRetryAt, COALESCE(lastError, '') lastError, COALESCE(traceId, '') traceId, sentAt,
COALESCE(contentTier, 'SENSITIVE') contentTier, createTime, updateTime
FROM sys_notification_delivery`
}

func deliveryDiagnosticSelectBase(r *Repository) string {
	return `SELECT id, deliveryId, sceneCode, channelCode, channelType, templateCode,
COALESCE(renderedSubject, '') renderedSubject, COALESCE(renderedText, '') renderedText,
COALESCE(contentTier, 'SENSITIVE') contentTier, status, createTime, updateTime
FROM sys_notification_delivery`
}

// deliveryDiagnosticScopePredicate is intentionally relation-based rather
// than channel-only. A delivery accepted through the modern Client belongs to
// the immutable notification or encrypted external-target snapshot; older V1
// rows retain the channel fallback only for their local legacy scope.
func deliveryDiagnosticScopePredicate(scopeID string) (string, []any) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return "1=0", nil
	}
	matchScope := func(alias string) string {
		if scopeID == "local" {
			// Legacy V1 configuration had no scope value. It is compatible only
			// with the local installation, never a Hub or Node scope.
			return "(" + alias + ".scopeId=? OR " + alias + ".scopeId IS NULL)"
		}
		return alias + ".scopeId=?"
	}
	notificationScope := matchScope("diagnosticNotification")
	externalScope := matchScope("diagnosticExternalTarget")
	channelScope := matchScope("diagnosticChannel")
	predicate := `(EXISTS (SELECT 1 FROM sys_notification diagnosticNotification
WHERE diagnosticNotification.id=sys_notification_delivery.notificationId AND ` + notificationScope + `)
OR EXISTS (SELECT 1 FROM sys_notification_external_target diagnosticExternalTarget
WHERE diagnosticExternalTarget.id=sys_notification_delivery.externalTargetId AND ` + externalScope + `)
OR EXISTS (SELECT 1 FROM sys_notification_channel diagnosticChannel
WHERE diagnosticChannel.channelCode=sys_notification_delivery.channelCode AND diagnosticChannel.isDeleted=0 AND ` + channelScope + `))`
	return predicate, []any{scopeID, scopeID, scopeID}
}
