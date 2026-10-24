package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	msgoutbox "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/outbox"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db     *sqlx.DB
	events *msgoutbox.Store
	guard  *msgoutbox.ConsumeGuard
}

const notificationOutboxOwner = "notification"

const (
	notificationWriteBatchLimit = 100
	notificationWriteChunkSize  = 50
)

var notificationOutboxEventTypes = []string{
	domain.OutboxEventNotificationDispatch,
	domain.OutboxEventNotificationIntent,
	domain.OutboxEventNotificationInboxChanged,
}

var notificationInboxPostgresIdentifiers = []string{
	"sys_notification_channel",
	"sys_notification_template",
	"sys_notification_template_definition",
	"sys_notification_template_revision",
	"sys_notification_template_revision_audit",
	"sys_notification_scene_definition",
	"sys_notification_scene_revision",
	"sys_notification_scene_revision_audit",
	"sys_notification_scene_snapshot",
	"sys_notification_scene_binding",
	"sys_notification_materialization_task",
	"sys_notification_mailbox",
	"sys_notification_recipient",
	"sys_notification_external_target",
	"sys_notification_delivery",
	"sys_notification_delivery_attempt",
	"sys_notification_http_delivery_snapshot",
	"sys_notification_delivery_ephemeral_content",
	"sys_notification_delivery_diagnostic_audit",
	"sys_notification",
	"sys_outbox_event",
	"eventId",
	"eventOwner",
	"eventType",
	"aggregateType",
	"aggregateId",
	"errorMsg",
	"notificationId",
	"deliveryId",
	"channelCode",
	"channelName",
	"channelType",
	"channelPriority",
	"templateCode",
	"templateName",
	"templateDefinitionId",
	"sceneDefinitionId",
	"sceneRevisionId",
	"sceneSnapshotId",
	"receiverKind",
	"currentDraftRevisionId",
	"currentPublishedRevisionId",
	"revisionNo",
	"revisionVersion",
	"state",
	"variableSchemaJson",
	"contentDigest",
	"connectionDigest",
	"templateRevisionId",
	"templateContentDigest",
	"renderedDigest",
	"variableDigest",
	"resolution",
	"errorCode",
	"publishedAt",
	"publishedBy",
	"fromRevisionNo",
	"toRevisionNo",
	"action",
	"actorId",
	"sceneCode",
	"sceneName",
	"locale",
	"subjectTemplate",
	"textTemplate",
	"htmlTemplate",
	"markdownTemplate",
	"primaryActionLabelTemplate",
	"jsonTemplate",
	"variablesJson",
	"enabled",
	"maxRetry",
	"retryIntervalSeconds",
	"configJson",
	"secretCiphertext",
	"secretEdek",
	"secretWrapKeyRef",
	"rateLimitJson",
	"metadataJson",
	"updaterId",
	"isDeleted",
	"status",
	"priority",
	"idempotencyKey",
	"requestDigest",
	"requestFingerprint",
	"audienceJson",
	"scheduleAt",
	"expiresAt",
	"expiredAt",
	"creatorId",
	"scopeId",
	"scopeType",
	"organizationId",
	"enterpriseConnectionId",
	"enterpriseConnectionVersion",
	"enterpriseConnectionDigest",
	"externalTargetId",
	"externalIdentityId",
	"externalIdentityVersion",
	"authorityDigest",
	"grantType",
	"targetMasked",
	"payloadJson",
	"renderedSubject",
	"renderedText",
	"renderedHtml",
	"renderedMarkdown",
	"presentationType",
	"actionType",
	"actionLabelSnapshot",
	"actionStyle",
	"actionUrlSnapshot",
	"renderedPayloadDigest",
	"contentTier",
	"ciphertext",
	"edek",
	"wrapKeyRef",
	"reasonCode",
	"ticketReference",
	"resultCode",
	"connectionRef",
	"providerCode",
	"identityKind",
	"subjectCiphertext",
	"subjectEdek",
	"subjectWrapKeyRef",
	"subjectDigest",
	"subjectDigestKeyRef",
	"providerParamsJson",
	"attemptId",
	"attemptNo",
	"failureClass",
	"providerReference",
	"eventKey",
	"deepLink",
	"traceId",
	"recipientId",
	"userId",
	"firstSeenAt",
	"readAt",
	"archivedAt",
	"mailboxVersion",
	"mailboxKey",
	"changeSequence",
	"taskId",
	"materializationCursor",
	"materializedCount",
	"retryCount",
	"nextRetryAt",
	"sentAt",
	"nextRunAt",
	"leaseOwner",
	"leaseToken",
	"leaseUntil",
	"lastError",
	"createTime",
	"updateTime",
}

var notificationPostgresRenderer = dbstore.MustNewPostgresRenderer(notificationInboxPostgresIdentifiers)

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db, events: msgoutbox.NewStore(db), guard: msgoutbox.NewConsumeGuard(db)}
}

func (r *Repository) ListChannels(ctx context.Context, query domain.ChannelQuery) ([]domain.Channel, int64, error) {
	where, args := []string{"isDeleted=0"}, []any{}
	if scopeID := strings.TrimSpace(query.ScopeID); scopeID != "" {
		if scopeID == "local" {
			// Pre-scope V1 channels are a local-only compatibility case. A Hub or
			// Node must not treat an ambiguous legacy channel as its own.
			where = append(where, "(scopeId=? OR scopeId IS NULL)")
			args = append(args, scopeID)
		} else {
			where = append(where, "scopeId=?")
			args = append(args, scopeID)
		}
	}
	if query.Keyword != "" {
		where = append(where, "(channelCode LIKE ? OR channelName LIKE ?)")
		kw := "%" + strings.TrimSpace(query.Keyword) + "%"
		args = append(args, kw, kw)
	}
	if query.ChannelType != "" {
		where = append(where, "channelType=?")
		args = append(args, strings.ToUpper(strings.TrimSpace(query.ChannelType)))
	}
	if query.Status != nil {
		where = append(where, "status=?")
		args = append(args, *query.Status)
	}
	return selectInboxPage[domain.Channel](ctx, r, channelSelectBase(r), where, "priority ASC, id DESC", query.Current, query.PageSize, args...)
}

func (r *Repository) FindChannelByCode(ctx context.Context, channelCode string) (*domain.Channel, error) {
	var item domain.Channel
	query := channelSelectBase(r) + " WHERE channelCode=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), channelCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListChannelsByCodes(ctx context.Context, channelCodes []string) ([]domain.Channel, error) {
	if len(channelCodes) == 0 {
		return []domain.Channel{}, nil
	}
	if len(channelCodes) > 100 {
		return nil, fmt.Errorf("notification channel batch exceeds limit")
	}
	query, args, err := sqlx.In(channelSelectBase(r)+" WHERE channelCode IN (?) AND isDeleted=0 ORDER BY channelCode ASC", channelCodes)
	if err != nil {
		return nil, fmt.Errorf("build notification channel batch query: %w", err)
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	items := make([]domain.Channel, 0, len(channelCodes))
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		return nil, fmt.Errorf("list notification channels by code: %w", err)
	}
	return items, nil
}

func (r *Repository) UpsertChannel(ctx context.Context, item *domain.Channel) error {
	if item == nil {
		return fmt.Errorf("notification channel is nil")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if existing, err := r.FindChannelByCode(ctx, item.ChannelCode); err != nil {
		return err
	} else if existing != nil {
		// The application performs a scope check before this write, but that
		// check alone is not a concurrency boundary. Keep the predicate in the
		// UPDATE itself so a same-code channel inserted or changed by another
		// scope between the read and write cannot be reassigned here.
		query := `UPDATE sys_notification_channel SET channelName=?, channelType=?, scopeId=?, scopeType=?, organizationId=?, enterpriseConnectionId=?, enterpriseConnectionVersion=?, enterpriseConnectionDigest=?, status=?, priority=?, configJson=?, secretCiphertext=?, secretEdek=?, secretWrapKeyRef=?, rateLimitJson=?, metadataJson=?, updaterId=?, updateTime=NOW() WHERE channelCode=? AND isDeleted=0`
		args := []any{item.ChannelName, item.ChannelType, nullIfBlank(item.ScopeID), normalizedChannelScopeType(item), item.OrganizationID, item.EnterpriseConnectionID, item.EnterpriseConnectionVersion, strings.TrimSpace(item.EnterpriseConnectionDigest), item.Status, item.Priority, nullJSON(item.ConfigJSON), nullIfBlank(item.SecretCiphertext), nullIfBlank(item.SecretEDEK), nullIfBlank(item.SecretWrapKeyRef), nullJSON(item.RateLimitJSON), nullJSON(item.MetadataJSON), item.UpdaterID, item.ChannelCode}
		query, args = appendConfigurationScopeCondition(query, args, item.ScopeID)
		result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), args...)
		if err != nil {
			return err
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			return nil
		}
		// MySQL can report zero affected rows for an unchanged update. Re-read
		// only to distinguish that no-op from a foreign or concurrently removed
		// configuration; never turn a scoped miss into an INSERT/update escape.
		reloaded, err := r.FindChannelByCode(ctx, item.ChannelCode)
		if err != nil {
			return err
		}
		if reloaded != nil && configurationScopeMatches(item.ScopeID, reloaded.ScopeID) {
			return nil
		}
		return domain.ErrScopedConfigurationNotFound
	}
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`INSERT INTO sys_notification_channel (id, channelCode, channelName, channelType, scopeId, scopeType, organizationId, enterpriseConnectionId, enterpriseConnectionVersion, enterpriseConnectionDigest, status, priority, configJson, secretCiphertext, secretEdek, secretWrapKeyRef, rateLimitJson, metadataJson, creatorId, updaterId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)),
		item.ID, item.ChannelCode, item.ChannelName, item.ChannelType, nullIfBlank(item.ScopeID), normalizedChannelScopeType(item), item.OrganizationID, item.EnterpriseConnectionID, item.EnterpriseConnectionVersion, strings.TrimSpace(item.EnterpriseConnectionDigest), item.Status, item.Priority, nullJSON(item.ConfigJSON), nullIfBlank(item.SecretCiphertext), nullIfBlank(item.SecretEDEK), nullIfBlank(item.SecretWrapKeyRef), nullJSON(item.RateLimitJSON), nullJSON(item.MetadataJSON), item.CreatorID, item.UpdaterID)
	return err
}

func (r *Repository) FindTemplateByCode(ctx context.Context, templateCode string) (*domain.Template, error) {
	var item domain.Template
	query := templateSelectBase(r) + " WHERE templateCode=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), templateCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindActiveSceneBinding(ctx context.Context, scopeID, sceneCode string) (*domain.SceneBinding, error) {
	var item domain.SceneBinding
	query := sceneBindingSelectBase(r) + " WHERE sceneCode=? AND enabled=? AND isDeleted=0"
	args := []any{sceneCode, boolInt(true)}
	query, args = appendConfigurationScopeCondition(query, args, scopeID)
	order, orderArgs := configurationScopeOrder(scopeID, "priority ASC, id DESC")
	args = append(args, orderArgs...)
	query += " ORDER BY " + order + " LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) InsertDelivery(ctx context.Context, item *domain.Delivery) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`INSERT INTO sys_notification_delivery (id, deliveryId, requestDigest, organizationId, notificationId, externalTargetId, sceneSnapshotId, sceneCode, channelCode, channelType, templateCode, target, targetMasked, payloadJson, renderedSubject, renderedText, renderedHtml, renderedMarkdown, presentationType, actionLabelSnapshot, actionUrlSnapshot, renderedPayloadDigest, contentTier, status, retryCount, maxRetry, nextRetryAt, lastError, providerReference, traceId, creatorId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)),
		item.ID, item.DeliveryID, item.RequestDigest, item.OrganizationID, item.NotificationID, item.ExternalTargetID, item.SceneSnapshotID, item.SceneCode, item.ChannelCode, item.ChannelType, item.TemplateCode, nullIfBlank(item.Target), nullIfBlank(item.TargetMasked), nullJSON(item.PayloadJSON), nullIfBlank(item.RenderedSubject), nullIfBlank(item.RenderedText), nullIfBlank(item.RenderedHTML), nullIfBlank(item.RenderedMarkdown), normalizedDeliveryPresentation(item.Presentation), strings.TrimSpace(item.ActionLabelSnapshot), strings.TrimSpace(item.ActionURLSnapshot), strings.TrimSpace(item.RenderedPayloadDigest), domain.NormalizeDeliveryContentTier(item.ContentTier), item.Status, item.RetryCount, item.MaxRetry, item.NextRetryAt, nullIfBlank(item.LastError), nullIfBlank(item.ProviderReference), nullIfBlank(item.TraceID), item.CreatorID)
	return err
}

func (r *Repository) InsertDeliveries(ctx context.Context, items []domain.Delivery) error {
	if len(items) == 0 {
		return nil
	}
	if err := requireNotificationTransaction(ctx); err != nil {
		return err
	}
	if len(items) > notificationWriteBatchLimit {
		return fmt.Errorf("notification delivery batch exceeds limit")
	}
	for index := range items {
		item := &items[index]
		if item.ID <= 0 || strings.TrimSpace(item.DeliveryID) == "" || strings.TrimSpace(item.RequestDigest) == "" || strings.TrimSpace(item.ChannelCode) == "" || strings.TrimSpace(item.ChannelType) == "" || strings.TrimSpace(item.Status) == "" {
			return fmt.Errorf("notification delivery is invalid")
		}
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	const insert = `INSERT INTO sys_notification_delivery (id, deliveryId, requestDigest, organizationId, notificationId, externalTargetId, sceneSnapshotId, sceneCode, channelCode, channelType, templateCode, target, targetMasked, payloadJson, renderedSubject, renderedText, renderedHtml, renderedMarkdown, presentationType, actionLabelSnapshot, actionUrlSnapshot, renderedPayloadDigest, contentTier, status, retryCount, maxRetry, nextRetryAt, lastError, providerReference, traceId, creatorId) VALUES `
	const row = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for start := 0; start < len(items); start += notificationWriteChunkSize {
		end := min(start+notificationWriteChunkSize, len(items))
		args := make([]any, 0, (end-start)*31)
		for index := start; index < end; index++ {
			item := &items[index]
			args = append(args,
				item.ID, item.DeliveryID, item.RequestDigest, item.OrganizationID, item.NotificationID, item.ExternalTargetID, item.SceneSnapshotID, item.SceneCode, item.ChannelCode, item.ChannelType, item.TemplateCode,
				nullIfBlank(item.Target), nullIfBlank(item.TargetMasked), nullJSON(item.PayloadJSON), nullIfBlank(item.RenderedSubject), nullIfBlank(item.RenderedText), nullIfBlank(item.RenderedHTML),
				nullIfBlank(item.RenderedMarkdown), normalizedDeliveryPresentation(item.Presentation), strings.TrimSpace(item.ActionLabelSnapshot), strings.TrimSpace(item.ActionURLSnapshot), strings.TrimSpace(item.RenderedPayloadDigest), domain.NormalizeDeliveryContentTier(item.ContentTier), item.Status, item.RetryCount, item.MaxRetry, item.NextRetryAt, nullIfBlank(item.LastError),
				nullIfBlank(item.ProviderReference), nullIfBlank(item.TraceID), item.CreatorID,
			)
		}
		query := insert + repeatedRows(row, end-start)
		if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) FindDeliveryByID(ctx context.Context, deliveryID string) (*domain.Delivery, error) {
	return r.findDelivery(ctx, "deliveryId=?", deliveryID)
}

func (r *Repository) FindDeliveryByDigest(ctx context.Context, digest string) (*domain.Delivery, error) {
	return r.findDelivery(ctx, "requestDigest=?", digest)
}

// ListDeliveriesByNotificationID is used only to compare the immutable route
// selection of a repeated semantic Client request. It is intentionally not a
// general unscoped management query.
func (r *Repository) ListDeliveriesByNotificationID(ctx context.Context, notificationID int64) ([]domain.Delivery, error) {
	if notificationID <= 0 {
		return []domain.Delivery{}, nil
	}
	query := deliverySelectBase(r) + " WHERE notificationId=? AND isDeleted=0 ORDER BY id ASC"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	items := make([]domain.Delivery, 0)
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), notificationID); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) InsertHTTPDeliverySnapshot(ctx context.Context, item *domain.HTTPDeliverySnapshot) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.DeliveryID) == "" || strings.TrimSpace(item.ScopeID) == "" || !domain.IsStaticHTTPChannelType(item.ChannelType) || strings.TrimSpace(item.ChannelCode) == "" || strings.TrimSpace(item.ConfigJSON) == "" {
		return fmt.Errorf("notification HTTP delivery snapshot is invalid")
	}
	if (strings.TrimSpace(item.SecretCiphertext) == "") != (strings.TrimSpace(item.SecretEDEK) == "") || (strings.TrimSpace(item.SecretCiphertext) == "") != (strings.TrimSpace(item.SecretWrapKeyRef) == "") {
		return fmt.Errorf("notification HTTP delivery snapshot secret envelope is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`INSERT INTO sys_notification_http_delivery_snapshot (id, deliveryId, scopeId, channelCode, channelType, channelPriority, configJson, secretCiphertext, secretEdek, secretWrapKeyRef) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)),
		item.ID, item.DeliveryID, item.ScopeID, item.ChannelCode, item.ChannelType, item.ChannelPriority, nullJSON(item.ConfigJSON), nullIfBlank(item.SecretCiphertext), nullIfBlank(item.SecretEDEK), nullIfBlank(item.SecretWrapKeyRef))
	return err
}

func (r *Repository) InsertHTTPDeliverySnapshots(ctx context.Context, items []domain.HTTPDeliverySnapshot) error {
	if len(items) == 0 {
		return nil
	}
	if err := requireNotificationTransaction(ctx); err != nil {
		return err
	}
	if len(items) > notificationWriteBatchLimit {
		return fmt.Errorf("notification HTTP delivery snapshot batch exceeds limit")
	}
	for index := range items {
		item := &items[index]
		if item.ID <= 0 || strings.TrimSpace(item.DeliveryID) == "" || strings.TrimSpace(item.ScopeID) == "" || !domain.IsStaticHTTPChannelType(item.ChannelType) || strings.TrimSpace(item.ChannelCode) == "" || strings.TrimSpace(item.ConfigJSON) == "" {
			return fmt.Errorf("notification HTTP delivery snapshot is invalid")
		}
		if (strings.TrimSpace(item.SecretCiphertext) == "") != (strings.TrimSpace(item.SecretEDEK) == "") || (strings.TrimSpace(item.SecretCiphertext) == "") != (strings.TrimSpace(item.SecretWrapKeyRef) == "") {
			return fmt.Errorf("notification HTTP delivery snapshot secret envelope is invalid")
		}
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	const insert = `INSERT INTO sys_notification_http_delivery_snapshot (id, deliveryId, scopeId, channelCode, channelType, channelPriority, configJson, secretCiphertext, secretEdek, secretWrapKeyRef) VALUES `
	const row = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for start := 0; start < len(items); start += notificationWriteChunkSize {
		end := min(start+notificationWriteChunkSize, len(items))
		args := make([]any, 0, (end-start)*10)
		for index := start; index < end; index++ {
			item := &items[index]
			args = append(args, item.ID, item.DeliveryID, item.ScopeID, item.ChannelCode, item.ChannelType, item.ChannelPriority, nullJSON(item.ConfigJSON), nullIfBlank(item.SecretCiphertext), nullIfBlank(item.SecretEDEK), nullIfBlank(item.SecretWrapKeyRef))
		}
		query := insert + repeatedRows(row, end-start)
		if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) FindHTTPDeliverySnapshotByDeliveryID(ctx context.Context, deliveryID string) (*domain.HTTPDeliverySnapshot, error) {
	if strings.TrimSpace(deliveryID) == "" {
		return nil, nil
	}
	var item domain.HTTPDeliverySnapshot
	query := `SELECT id, deliveryId, scopeId, channelCode, channelType, channelPriority, ` + r.jsonText("configJson") + ` configJson,
COALESCE(secretCiphertext, '') secretCiphertext, COALESCE(secretEdek, '') secretEdek, COALESCE(secretWrapKeyRef, '') secretWrapKeyRef, createTime, updateTime
FROM sys_notification_http_delivery_snapshot WHERE deliveryId=? LIMIT 1`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), deliveryID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListDeliveries(ctx context.Context, query domain.DeliveryQuery) ([]domain.Delivery, int64, error) {
	where, args := []string{"isDeleted=0"}, []any{}
	if query.Keyword != "" {
		where = append(where, "(deliveryId LIKE ? OR targetMasked LIKE ? OR traceId LIKE ?)")
		kw := "%" + strings.TrimSpace(query.Keyword) + "%"
		args = append(args, kw, kw, kw)
	}
	if query.SceneCode != "" {
		where = append(where, "sceneCode=?")
		args = append(args, query.SceneCode)
	}
	if query.ChannelCode != "" {
		where = append(where, "channelCode=?")
		args = append(args, query.ChannelCode)
	}
	if query.Status != "" {
		where = append(where, "status=?")
		args = append(args, query.Status)
	}
	return selectInboxPage[domain.Delivery](ctx, r, deliverySelectBase(r), where, "createTime DESC, id DESC", query.Current, query.PageSize, args...)
}

func (r *Repository) MarkDeliverySending(ctx context.Context, deliveryID string) (bool, error) {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status='SENDING', updateTime=NOW() WHERE deliveryId=? AND status='PENDING' AND isDeleted=0`)), deliveryID)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (r *Repository) MarkDeliverySent(ctx context.Context, deliveryID string, sentAt time.Time) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status='SENT', sentAt=?, lastError=NULL, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)), sentAt, deliveryID)
	return err
}

func (r *Repository) MarkDeliveryRetry(ctx context.Context, deliveryID string, retryCount int, nextRetryAt time.Time, lastError string) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status='PENDING', retryCount=?, nextRetryAt=?, lastError=?, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)), retryCount, nextRetryAt, nullIfBlank(lastError), deliveryID)
	return err
}

func (r *Repository) MarkDeliveryFailed(ctx context.Context, deliveryID string, retryCount int, lastError string) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status='FAILED', retryCount=?, nextRetryAt=NULL, lastError=?, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)), retryCount, nullIfBlank(lastError), deliveryID)
	return err
}

func (r *Repository) MarkDeliveryCanceled(ctx context.Context, deliveryID, reasonCode string) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status='CANCELED', nextRetryAt=NULL, lastError=?, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)), nullIfBlank(reasonCode), deliveryID)
	return err
}

func (r *Repository) MarkDeliveryProviderAccepted(ctx context.Context, deliveryID, providerReference string, acceptedAt time.Time) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status=?, sentAt=?, providerReference=?, lastError=NULL, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)),
		domain.DeliveryStatusProviderAccepted, acceptedAt, nullIfBlank(providerReference), deliveryID)
	return err
}

func (r *Repository) MarkDeliveryUnknown(ctx context.Context, deliveryID, diagnostic string) error {
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_delivery SET status=?, nextRetryAt=NULL, lastError=?, updateTime=NOW() WHERE deliveryId=? AND isDeleted=0`)),
		domain.DeliveryStatusUnknown, nullIfBlank(diagnostic), deliveryID)
	return err
}

func (r *Repository) InsertDeliveryAttempt(ctx context.Context, item *domain.DeliveryAttempt) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.AttemptID) == "" || strings.TrimSpace(item.DeliveryID) == "" || item.AttemptNo <= 0 {
		return fmt.Errorf("notification delivery attempt is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_delivery_attempt (id, attemptId, deliveryId, attemptNo, status, failureClass, providerReference, diagnostic)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	if r.isPostgres() {
		query += ` ON CONFLICT (deliveryId, attemptNo) DO NOTHING`
	} else {
		query += ` ON DUPLICATE KEY UPDATE id=id`
	}
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.AttemptID, item.DeliveryID, item.AttemptNo,
		item.Status, nullIfBlank(item.FailureClass), nullIfBlank(item.ProviderReference), nullIfBlank(item.Diagnostic))
	return err
}

func (r *Repository) InsertExternalTargets(ctx context.Context, items []domain.ExternalTarget) error {
	if len(items) == 0 {
		return nil
	}
	if err := requireNotificationTransaction(ctx); err != nil {
		return err
	}
	if len(items) > notificationWriteBatchLimit {
		return fmt.Errorf("notification external target batch exceeds limit")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	const insert = `INSERT INTO sys_notification_external_target (id, externalTargetId, notificationId, scopeId, organizationId, userId, externalIdentityId, externalIdentityVersion, enterpriseConnectionId, enterpriseConnectionVersion, authorityDigest, grantType, connectionRef, providerCode, identityKind, subjectCiphertext, subjectEdek, subjectWrapKeyRef, subjectDigest, subjectDigestKeyRef, providerParamsJson) VALUES `
	const row = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for index := range items {
		item := &items[index]
		if item.ID <= 0 || item.NotificationID <= 0 || strings.TrimSpace(item.ExternalTargetID) == "" || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.ConnectionRef) == "" || strings.TrimSpace(item.ProviderCode) == "" || strings.TrimSpace(item.IdentityKind) == "" || strings.TrimSpace(item.SubjectCiphertext) == "" || strings.TrimSpace(item.SubjectEDEK) == "" || strings.TrimSpace(item.SubjectWrapKeyRef) == "" || strings.TrimSpace(item.SubjectDigest) == "" || strings.TrimSpace(item.SubjectDigestKeyRef) == "" {
			return fmt.Errorf("notification external target is invalid")
		}
	}
	for start := 0; start < len(items); start += notificationWriteChunkSize {
		end := min(start+notificationWriteChunkSize, len(items))
		args := make([]any, 0, (end-start)*21)
		for index := start; index < end; index++ {
			item := &items[index]
			// authorityDigest and grantType are deliberately empty (rather than
			// NULL) for non-enterprise recipients such as email.  The existing
			// PostgreSQL and MySQL schemas make those audit columns NOT NULL with
			// an empty-string default; supplying NULL here bypasses that default
			// and rolls the entire notification publication transaction back.
			args = append(args, item.ID, item.ExternalTargetID, item.NotificationID, item.ScopeID, nullablePositiveID(item.OrganizationID), nullablePositiveID(item.UserID), nullablePositiveID(item.ExternalIdentityID), item.ExternalIdentityVersion, nullablePositiveID(item.EnterpriseConnectionID), item.EnterpriseConnectionVersion, strings.TrimSpace(item.AuthorityDigest), strings.TrimSpace(item.GrantType), item.ConnectionRef, item.ProviderCode, item.IdentityKind,
				item.SubjectCiphertext, item.SubjectEDEK, nullIfBlank(item.SubjectWrapKeyRef), item.SubjectDigest, item.SubjectDigestKeyRef, nullJSON(item.ProviderParamsJSON))
		}
		query := insert + repeatedRows(row, end-start)
		if r.isPostgres() {
			query += ` ON CONFLICT (notificationId, connectionRef, identityKind, subjectDigest) DO NOTHING`
		} else {
			query += ` ON DUPLICATE KEY UPDATE id=id`
		}
		if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) FindExternalTargetByID(ctx context.Context, externalTargetID int64) (*domain.ExternalTarget, error) {
	if externalTargetID <= 0 {
		return nil, nil
	}
	return r.findExternalTarget(ctx, "id=?", externalTargetID)
}

func (r *Repository) ListExternalTargetsByNotificationID(ctx context.Context, notificationID int64) ([]domain.ExternalTarget, error) {
	if notificationID <= 0 {
		return []domain.ExternalTarget{}, nil
	}
	query := externalTargetSelectBase(r) + ` WHERE notificationId=? ORDER BY connectionRef ASC, identityKind ASC, subjectDigest ASC`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.ExternalTarget
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), notificationID); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) AppendOutbox(ctx context.Context, event *domain.OutboxEvent) error {
	if event == nil {
		return r.events.Append(ctx, nil)
	}
	return r.events.Append(ctx, &msgoutbox.Event{ID: event.ID, EventID: event.EventID, EventOwner: notificationOutboxOwner, ScopeID: event.ScopeID, EventType: event.EventType, AggregateType: event.AggregateType, AggregateID: event.AggregateID, Payload: event.Payload, Status: event.Status, RetryCount: event.RetryCount, NextRetryAt: event.NextRetryAt, LastError: event.LastError})
}

func (r *Repository) AppendOutboxBatch(ctx context.Context, events []domain.OutboxEvent) error {
	if len(events) == 0 {
		return nil
	}
	if err := requireNotificationTransaction(ctx); err != nil {
		return err
	}
	if len(events) > notificationWriteBatchLimit {
		return fmt.Errorf("notification outbox batch exceeds limit")
	}
	for index := range events {
		event := &events[index]
		if event.ID <= 0 || strings.TrimSpace(event.EventID) == "" || strings.TrimSpace(event.EventType) == "" || strings.TrimSpace(event.AggregateType) == "" || strings.TrimSpace(event.AggregateID) == "" {
			return fmt.Errorf("notification outbox event is invalid")
		}
		if strings.TrimSpace(event.Status) == "" {
			event.Status = "PENDING"
		}
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	const insert = `INSERT INTO sys_outbox_event (id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status, retryCount, nextRetryAt, errorMsg) VALUES `
	const row = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	for start := 0; start < len(events); start += notificationWriteChunkSize {
		end := min(start+notificationWriteChunkSize, len(events))
		args := make([]any, 0, (end-start)*12)
		for index := start; index < end; index++ {
			event := &events[index]
			args = append(args, event.ID, event.EventID, notificationOutboxOwner, nullIfBlank(event.ScopeID), event.EventType, event.AggregateType, event.AggregateID,
				event.Payload, event.Status, event.RetryCount, nullTime(event.NextRetryAt), nullIfBlank(event.LastError))
		}
		query := insert + repeatedRows(row, end-start)
		if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListReadyOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	events, err := r.events.ListReady(ctx, notificationOutboxOwner, notificationOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, mapOutboxEvent(event))
	}
	return result, nil
}

// ListReadyOutboxForScope selects ready notification work in SQL before
// LIMIT. This prevents a busy installation/Hub/Node from claiming or
// starving another scope's durable events.
func (r *Repository) ListReadyOutboxForScope(ctx context.Context, scopeID string, limit int) ([]domain.OutboxEvent, error) {
	events, err := r.events.ListReadyForScope(ctx, notificationOutboxOwner, scopeID, notificationOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, mapOutboxEvent(event))
	}
	return result, nil
}

func (r *Repository) FindReadyOutbox(ctx context.Context, eventID, eventType string) (*domain.OutboxEvent, error) {
	event, err := r.events.FindReady(ctx, notificationOutboxOwner, eventType, eventID)
	if err != nil || event == nil {
		return nil, err
	}
	result := mapOutboxEvent(*event)
	return &result, nil
}

// FindReadyOutboxForScope obtains one exact scope-owned event for controlled
// recovery/acceptance work. It never falls back to an unscoped lookup.
func (r *Repository) FindReadyOutboxForScope(ctx context.Context, scopeID, eventID, eventType string) (*domain.OutboxEvent, error) {
	event, err := r.events.FindReadyForScope(ctx, notificationOutboxOwner, scopeID, eventType, eventID)
	if err != nil || event == nil {
		return nil, err
	}
	result := mapOutboxEvent(*event)
	return &result, nil
}

func (r *Repository) ListUnknownOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	events, err := r.events.ListUnknownReady(ctx, notificationOutboxOwner, notificationOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, mapOutboxEvent(event))
	}
	return result, nil
}

// ListUnknownOutboxForScope keeps unknown notification events fail-closed
// while preserving the same scope boundary as normal relay work.
func (r *Repository) ListUnknownOutboxForScope(ctx context.Context, scopeID string, limit int) ([]domain.OutboxEvent, error) {
	events, err := r.events.ListUnknownReadyForScope(ctx, notificationOutboxOwner, scopeID, notificationOutboxEventTypes, limit)
	if err != nil {
		return nil, err
	}
	result := make([]domain.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, mapOutboxEvent(event))
	}
	return result, nil
}

func mapOutboxEvent(event msgoutbox.Event) domain.OutboxEvent {
	return domain.OutboxEvent{
		ID:            event.ID,
		EventID:       event.EventID,
		EventOwner:    event.EventOwner,
		ScopeID:       event.ScopeID,
		EventType:     event.EventType,
		AggregateType: event.AggregateType,
		AggregateID:   event.AggregateID,
		Payload:       event.Payload,
		Status:        event.Status,
		RetryCount:    event.RetryCount,
		NextRetryAt:   event.NextRetryAt,
		LastError:     event.LastError,
		CreateTime:    event.CreateTime,
		UpdateTime:    event.UpdateTime,
	}
}

func (r *Repository) TryClaimOutbox(ctx context.Context, id int64, eventType, worker string) (*domain.OutboxLease, bool, error) {
	lease, claimed, err := r.events.Claim(ctx, notificationOutboxOwner, eventType, id, worker)
	if err != nil || !claimed {
		return nil, claimed, err
	}
	return &domain.OutboxLease{Token: lease.Token, Until: lease.Until}, true, nil
}

func (r *Repository) MarkOutbox(ctx context.Context, id int64, eventType, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	return r.events.Mark(ctx, notificationOutboxOwner, eventType, id, leaseToken, status, lastError, retryCount, nextRetryAt)
}

func (r *Repository) BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*domain.ConsumeLease, bool, error) {
	lease, claimed, err := r.guard.Begin(ctx, messageID, consumer, worker, detail)
	if err != nil || !claimed {
		return nil, claimed, err
	}
	return &domain.ConsumeLease{Token: lease.Token, Until: lease.Until}, true, nil
}

func (r *Repository) MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return r.guard.Finish(ctx, messageID, consumer, leaseToken, detail)
}

func (r *Repository) MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return r.guard.Fail(ctx, messageID, consumer, leaseToken, detail)
}

func (r *Repository) FindLogicalNotificationByIdempotency(ctx context.Context, scopeID, eventKey, idempotencyKey string) (*domain.LogicalNotification, error) {
	return r.findLogicalNotification(ctx, "scopeId=? AND eventKey=? AND idempotencyKey=?", scopeID, eventKey, idempotencyKey)
}

func (r *Repository) FindLogicalNotificationByID(ctx context.Context, notificationID int64) (*domain.LogicalNotification, error) {
	if notificationID <= 0 {
		return nil, nil
	}
	return r.findLogicalNotification(ctx, "id=?", notificationID)
}

func (r *Repository) CreateLogicalNotification(ctx context.Context, item *domain.LogicalNotification) (bool, error) {
	if item == nil || item.ID <= 0 {
		return false, fmt.Errorf("logical notification is invalid")
	}
	query := `INSERT INTO sys_notification (id, notificationId, scopeId, organizationId, eventKey, idempotencyKey, requestFingerprint, audienceJson, category, priority, mandatory, title, content, deepLink, actionType, actionLabelSnapshot, actionStyle, scheduleAt, expiresAt, traceId, status, creatorId)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if r.isPostgres() {
		query += ` ON CONFLICT (scopeId, eventKey, idempotencyKey) DO NOTHING`
	} else {
		query += ` ON DUPLICATE KEY UPDATE id=id`
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)),
		item.ID, item.NotificationID, item.ScopeID, item.OrganizationID, item.EventKey, item.IdempotencyKey, item.RequestFingerprint, nullJSON(item.AudienceJSON),
		item.Category, item.Priority, r.databaseBool(item.Mandatory), item.Title, item.Content, nullIfBlank(item.DeepLink), nullIfBlank(item.ActionType), nullIfBlank(item.ActionLabelSnapshot), nullIfBlank(item.ActionStyle), item.ScheduleAt, item.ExpiresAt,
		nullIfBlank(item.TraceID), item.Status, item.CreatorID,
	)
	if err != nil {
		return false, err
	}
	existing, err := r.FindLogicalNotificationByIdempotency(ctx, item.ScopeID, item.EventKey, item.IdempotencyKey)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, fmt.Errorf("logical notification idempotency row was not readable after insert")
	}
	return existing.ID == item.ID, nil
}

func (r *Repository) MarkLogicalNotificationMaterialized(ctx context.Context, notificationID int64) error {
	if notificationID <= 0 {
		return fmt.Errorf("notification id is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification SET status=?, updateTime=NOW() WHERE id=?`)), domain.NotificationStatusMaterialized, notificationID)
	return err
}

func (r *Repository) InsertRecipients(ctx context.Context, items []domain.Recipient) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	query := `INSERT INTO sys_notification_recipient (id, recipientId, notificationId, scopeId, userId, eventKey, category, priority, mandatory, title, content, deepLink, actionType, actionLabelSnapshot, actionStyle, expiresAt, firstSeenAt, readAt, archivedAt, mailboxVersion)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if r.isPostgres() {
		query += ` ON CONFLICT (notificationId, userId) DO NOTHING`
	} else {
		query = `INSERT INTO sys_notification_recipient (id, recipientId, notificationId, scopeId, userId, eventKey, category, priority, mandatory, title, content, deepLink, actionType, actionLabelSnapshot, actionStyle, expiresAt, firstSeenAt, readAt, archivedAt, mailboxVersion)
VALUES (LAST_INSERT_ID(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	inserted := 0
	for _, item := range items {
		if item.ID <= 0 || item.NotificationID <= 0 || item.UserID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.RecipientID) == "" {
			return inserted, fmt.Errorf("notification recipient is invalid")
		}
		result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)),
			item.ID, item.RecipientID, item.NotificationID, item.ScopeID, item.UserID, item.EventKey, item.Category, item.Priority, r.databaseBool(item.Mandatory),
			item.Title, item.Content, nullIfBlank(item.DeepLink), nullIfBlank(item.ActionType), nullIfBlank(item.ActionLabelSnapshot), nullIfBlank(item.ActionStyle), item.ExpiresAt, item.FirstSeenAt, item.ReadAt, item.ArchivedAt, item.MailboxVersion,
		)
		if err != nil {
			return inserted, err
		}
		if r.isPostgres() {
			rows, err := result.RowsAffected()
			if err != nil {
				return inserted, err
			}
			if rows > 0 {
				inserted += int(rows)
			}
			continue
		}
		insertedID, err := result.LastInsertId()
		if err != nil {
			return inserted, err
		}
		if insertedID == item.ID {
			inserted++
		}
	}
	return inserted, nil
}

// InsertInboxRecipients creates only new recipient projections and assigns a
// serialized mailbox sequence in the same caller transaction. A duplicate
// `(notificationId,userId)` does not advance the mailbox or emit a result.
func (r *Repository) InsertInboxRecipients(ctx context.Context, items []domain.Recipient) ([]domain.Recipient, error) {
	if len(items) == 0 {
		return []domain.Recipient{}, nil
	}
	if len(items) > notificationWriteBatchLimit {
		return nil, fmt.Errorf("notification recipient batch exceeds limit")
	}
	ordered := append([]domain.Recipient(nil), items...)
	for _, item := range ordered {
		if item.ID <= 0 || item.NotificationID <= 0 || item.UserID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.RecipientID) == "" {
			return nil, fmt.Errorf("notification recipient is invalid")
		}
	}
	// Mailbox sequences are the cross-instance ordering fence for visible
	// inbox changes. Always acquire them in one deterministic order so callers
	// cannot introduce an inverted multi-mailbox lock order.
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ScopeID != ordered[j].ScopeID {
			return ordered[i].ScopeID < ordered[j].ScopeID
		}
		if ordered[i].UserID != ordered[j].UserID {
			return ordered[i].UserID < ordered[j].UserID
		}
		return ordered[i].ID < ordered[j].ID
	})
	query := `INSERT INTO sys_notification_recipient (id, recipientId, notificationId, scopeId, userId, eventKey, category, priority, mandatory, title, content, deepLink, actionType, actionLabelSnapshot, actionStyle, expiresAt, firstSeenAt, readAt, archivedAt, mailboxVersion)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`
	if r.isPostgres() {
		query += ` ON CONFLICT (notificationId, userId) DO NOTHING RETURNING id`
	} else {
		query = `INSERT INTO sys_notification_recipient (id, recipientId, notificationId, scopeId, userId, eventKey, category, priority, mandatory, title, content, deepLink, actionType, actionLabelSnapshot, actionStyle, expiresAt, firstSeenAt, readAt, archivedAt, mailboxVersion)
VALUES (LAST_INSERT_ID(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	created := make([]domain.Recipient, 0, len(ordered))
	for _, item := range ordered {
		inserted := false
		if r.isPostgres() {
			var insertedID int64
			err := sqlx.GetContext(ctx, exec, &insertedID, exec.Rebind(r.inboxSQL(query)),
				item.ID, item.RecipientID, item.NotificationID, item.ScopeID, item.UserID, item.EventKey, item.Category, item.Priority, r.databaseBool(item.Mandatory),
				item.Title, item.Content, nullIfBlank(item.DeepLink), nullIfBlank(item.ActionType), nullIfBlank(item.ActionLabelSnapshot), nullIfBlank(item.ActionStyle), item.ExpiresAt, item.FirstSeenAt, item.ReadAt, item.ArchivedAt,
			)
			if err != nil {
				if err == sql.ErrNoRows {
					continue
				}
				return created, err
			}
			inserted = insertedID == item.ID
		} else {
			result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)),
				item.ID, item.RecipientID, item.NotificationID, item.ScopeID, item.UserID, item.EventKey, item.Category, item.Priority, r.databaseBool(item.Mandatory),
				item.Title, item.Content, nullIfBlank(item.DeepLink), nullIfBlank(item.ActionType), nullIfBlank(item.ActionLabelSnapshot), nullIfBlank(item.ActionStyle), item.ExpiresAt, item.FirstSeenAt, item.ReadAt, item.ArchivedAt,
			)
			if err != nil {
				return created, err
			}
			insertedID, err := result.LastInsertId()
			if err != nil {
				return created, err
			}
			inserted = insertedID == item.ID
		}
		if !inserted {
			continue
		}
		mailbox, err := r.AdvanceMailboxChange(ctx, item.ScopeID, item.UserID)
		if err != nil {
			return created, err
		}
		result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_recipient
SET mailboxVersion=?, updateTime=NOW()
WHERE id=? AND scopeId=? AND userId=? AND mailboxVersion=0`)), mailbox.ChangeSequence, item.ID, item.ScopeID, item.UserID)
		if err != nil {
			return created, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return created, err
		}
		if rows != 1 {
			return created, fmt.Errorf("notification recipient mailbox sequence update was superseded")
		}
		item.MailboxVersion = mailbox.ChangeSequence
		created = append(created, item)
	}
	return created, nil
}

func (r *Repository) CreateMaterializationTask(ctx context.Context, item *domain.MaterializationTask) (bool, error) {
	if item == nil || item.ID <= 0 || item.NotificationID <= 0 {
		return false, fmt.Errorf("notification materialization task is invalid")
	}
	query := `INSERT INTO sys_notification_materialization_task (id, taskId, notificationId, scopeId, audienceJson, materializationCursor, status, materializedCount, retryCount, nextRunAt, leaseOwner, leaseToken, leaseUntil, lastError)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if r.isPostgres() {
		query += ` ON CONFLICT (notificationId) DO NOTHING`
	} else {
		query += ` ON DUPLICATE KEY UPDATE id=id`
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.TaskID, item.NotificationID, item.ScopeID, nullJSON(item.AudienceJSON), item.Cursor, item.Status, item.MaterializedCount, item.RetryCount, item.NextRunAt, nullIfBlank(item.LeaseOwner), nullIfBlank(item.LeaseToken), item.LeaseUntil, nullIfBlank(item.LastError))
	if err != nil {
		return false, err
	}
	existing, err := r.FindMaterializationTaskByNotificationID(ctx, item.NotificationID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, fmt.Errorf("notification materialization task was not readable after insert")
	}
	return existing.ID == item.ID, nil
}

func (r *Repository) FindMaterializationTaskByNotificationID(ctx context.Context, notificationID int64) (*domain.MaterializationTask, error) {
	if notificationID <= 0 {
		return nil, nil
	}
	return r.findMaterializationTask(ctx, "notificationId=?", notificationID)
}

func (r *Repository) ListReadyMaterializationTasks(ctx context.Context, scopeID string, limit int) ([]domain.MaterializationTask, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return nil, fmt.Errorf("notification materialization scope is required")
	}
	if limit <= 0 {
		limit = 20
	}
	now := time.Now().UTC()
	query := materializationTaskSelectBase(r) + ` WHERE scopeId=? AND (
  (status=? AND nextRunAt <= ?)
  OR (status=? AND (leaseUntil IS NULL OR leaseUntil <= ?))
) ORDER BY nextRunAt ASC, id ASC LIMIT ?`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.MaterializationTask
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), scopeID, domain.TaskStatusPending, now, domain.TaskStatusProcessing, now, limit); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) TryClaimMaterializationTask(ctx context.Context, scopeID string, taskID int64, worker string, now time.Time) (*domain.MaterializationTask, bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || taskID <= 0 || strings.TrimSpace(worker) == "" {
		return nil, false, fmt.Errorf("notification materialization claim is invalid")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	leaseToken := uuid.NewString()
	leaseUntil := now.Add(2 * time.Minute)
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_materialization_task
SET status=?, leaseOwner=?, leaseToken=?, leaseUntil=?, retryCount=CASE WHEN status=? THEN retryCount + 1 ELSE retryCount END, updateTime=NOW()
WHERE scopeId=? AND id=? AND (
  (status=? AND nextRunAt <= ?)
  OR (status=? AND (leaseUntil IS NULL OR leaseUntil <= ?))
)`)), domain.TaskStatusProcessing, strings.TrimSpace(worker), leaseToken, leaseUntil, domain.TaskStatusProcessing, scopeID, taskID, domain.TaskStatusPending, now, domain.TaskStatusProcessing, now)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 0 {
		return nil, false, nil
	}
	task, err := r.findMaterializationTask(ctx, "scopeId=? AND id=?", scopeID, taskID)
	if err != nil {
		return nil, false, err
	}
	if task == nil || task.LeaseToken != leaseToken {
		return nil, false, fmt.Errorf("notification materialization lease was not readable")
	}
	return task, true, nil
}

func (r *Repository) AdvanceMaterializationTask(ctx context.Context, scopeID string, taskID int64, leaseToken, cursor, status string, materializedCount int64, nextRunAt time.Time) (bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || taskID <= 0 || strings.TrimSpace(leaseToken) == "" || strings.TrimSpace(status) == "" {
		return false, fmt.Errorf("notification materialization advance is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_materialization_task
SET materializationCursor=?, status=?, materializedCount=?, nextRunAt=?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, lastError=NULL, updateTime=NOW()
WHERE scopeId=? AND id=? AND status=? AND leaseToken=?`)), cursor, status, materializedCount, nextRunAt, scopeID, taskID, domain.TaskStatusProcessing, leaseToken)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) FailMaterializationTask(ctx context.Context, scopeID string, taskID int64, leaseToken, status, lastError string, retryCount int, nextRunAt time.Time) (bool, error) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" || taskID <= 0 || strings.TrimSpace(leaseToken) == "" || (status != domain.TaskStatusPending && status != domain.TaskStatusFailed) {
		return false, fmt.Errorf("notification materialization failure update is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_materialization_task
SET status=?, retryCount=?, nextRunAt=?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, lastError=?, updateTime=NOW()
WHERE scopeId=? AND id=? AND status=? AND leaseToken=?`)), status, retryCount, nextRunAt, nullIfBlank(lastError), scopeID, taskID, domain.TaskStatusProcessing, leaseToken)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) ListInboxRecipients(ctx context.Context, query domain.InboxQuery) ([]domain.Recipient, error) {
	if query.UserID <= 0 || strings.TrimSpace(query.ScopeID) == "" {
		return nil, fmt.Errorf("notification inbox query is invalid")
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	where := []string{"scopeId=?", "userId=?", "expiredAt IS NULL", "(expiresAt IS NULL OR expiresAt > ?)"}
	args := []any{query.ScopeID, query.UserID, time.Now().UTC()}
	if query.Archived {
		where = append(where, "archivedAt IS NOT NULL")
	} else {
		where = append(where, "archivedAt IS NULL")
	}
	if query.Cursor != nil {
		where = append(where, "(createTime < ? OR (createTime = ? AND id < ?))")
		args = append(args, query.Cursor.CreateTime.UTC(), query.Cursor.CreateTime.UTC(), query.Cursor.ID)
	}
	args = append(args, query.Limit)
	querySQL := r.recipientSelectBase() + " WHERE " + strings.Join(where, " AND ") + " ORDER BY createTime DESC, id DESC LIMIT ?"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.Recipient
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(querySQL)), args...); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListUnreadInboxRecipients(ctx context.Context, scopeID string, userID int64, limit int) ([]domain.Recipient, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return nil, fmt.Errorf("notification unread preview query is invalid")
	}
	if limit <= 0 || limit > 5 {
		limit = 5
	}
	query := r.recipientSelectBase() + ` WHERE scopeId=? AND userId=? AND expiredAt IS NULL AND archivedAt IS NULL AND readAt IS NULL AND (expiresAt IS NULL OR expiresAt > ?) ORDER BY createTime DESC, id DESC LIMIT ?`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.Recipient
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), scopeID, userID, time.Now().UTC(), limit); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListInboxRecipientChanges(ctx context.Context, query domain.InboxChangeQuery) ([]domain.Recipient, error) {
	if query.UserID <= 0 || strings.TrimSpace(query.ScopeID) == "" || query.AfterSequence < 0 || query.UntilSequence < query.AfterSequence {
		return nil, fmt.Errorf("notification inbox change query is invalid")
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	querySQL := r.recipientSelectBase() + ` WHERE scopeId=? AND userId=? AND mailboxVersion>? AND mailboxVersion<=? ORDER BY mailboxVersion ASC LIMIT ?`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.Recipient
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(querySQL)), query.ScopeID, query.UserID, query.AfterSequence, query.UntilSequence, query.Limit); err != nil {
		return nil, err
	}
	return items, nil
}

// ListExpiredInboxRecipients finds a bounded batch of due projections. The
// caller locks each candidate again before changing it so concurrent workers
// cannot allocate two mailbox versions for the same expiry.
func (r *Repository) ListExpiredInboxRecipients(ctx context.Context, scopeID string, now time.Time, limit int) ([]domain.Recipient, error) {
	if strings.TrimSpace(scopeID) == "" {
		return nil, fmt.Errorf("notification inbox expiry scope is invalid")
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	query := r.recipientSelectBase() + ` WHERE scopeId=? AND expiredAt IS NULL AND expiresAt IS NOT NULL AND expiresAt <= ? ORDER BY expiresAt ASC, id ASC LIMIT ?`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var items []domain.Recipient
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), scopeID, now.UTC(), limit); err != nil {
		return nil, err
	}
	return items, nil
}

// LockExpiredInboxRecipient obtains the current due recipient in the
// surrounding transaction. A worker that arrives after another worker already
// processed it receives nil instead of writing another mailbox change.
func (r *Repository) LockExpiredInboxRecipient(ctx context.Context, recipientID int64, now time.Time) (*domain.Recipient, error) {
	if recipientID <= 0 {
		return nil, fmt.Errorf("notification inbox expiry recipient is invalid")
	}
	query := r.recipientSelectBase() + ` WHERE id=? AND expiredAt IS NULL AND expiresAt IS NOT NULL AND expiresAt <= ? FOR UPDATE`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var item domain.Recipient
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), recipientID, now.UTC()); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindInboxRecipient(ctx context.Context, scopeID string, userID int64, recipientID string) (*domain.Recipient, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" || strings.TrimSpace(recipientID) == "" {
		return nil, nil
	}
	var item domain.Recipient
	query := r.recipientSelectBase() + ` WHERE scopeId=? AND userId=? AND recipientId=? AND expiredAt IS NULL AND (expiresAt IS NULL OR expiresAt > ?) LIMIT 1`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), scopeID, userID, recipientID, time.Now().UTC()); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) CountUnreadInboxRecipients(ctx context.Context, scopeID string, userID int64) (int64, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return 0, fmt.Errorf("notification unread count query is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var count int64
	query := `SELECT COUNT(1) FROM sys_notification_recipient WHERE scopeId=? AND userId=? AND expiredAt IS NULL AND archivedAt IS NULL AND readAt IS NULL AND (expiresAt IS NULL OR expiresAt > ?)`
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(r.inboxSQL(query)), scopeID, userID, time.Now().UTC()); err != nil {
		return 0, err
	}
	return count, nil
}

// LockMailbox creates a mailbox lazily and locks its durable sequence while a
// count/list/preview snapshot is read in the caller transaction.
func (r *Repository) LockMailbox(ctx context.Context, scopeID string, userID int64, mailboxKey string) (*domain.Mailbox, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return nil, fmt.Errorf("notification mailbox query is invalid")
	}
	if strings.TrimSpace(mailboxKey) == "" {
		mailboxKey = "mbx_" + uuid.NewString()
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	insert := `INSERT INTO sys_notification_mailbox (scopeId, userId, mailboxKey, changeSequence) VALUES (?, ?, ?, 0)`
	if r.isPostgres() {
		insert += ` ON CONFLICT (scopeId, userId) DO NOTHING`
	} else {
		insert += ` ON DUPLICATE KEY UPDATE id=id`
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(insert)), scopeID, userID, mailboxKey); err != nil {
		return nil, err
	}
	return r.findMailboxForUpdate(ctx, scopeID, userID)
}

// AdvanceMailboxChange serializes one visible recipient mutation for the
// mailbox. Callers must use it inside the same transaction as the recipient
// write and its content-free outbox intent.
func (r *Repository) AdvanceMailboxChange(ctx context.Context, scopeID string, userID int64) (*domain.Mailbox, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return nil, fmt.Errorf("notification mailbox advance is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	mailboxKey := "mbx_" + uuid.NewString()
	if r.isPostgres() {
		query := `INSERT INTO sys_notification_mailbox (scopeId, userId, mailboxKey, changeSequence)
VALUES (?, ?, ?, 1)
ON CONFLICT (scopeId, userId) DO UPDATE
SET changeSequence=sys_notification_mailbox.changeSequence + 1, updateTime=NOW()
RETURNING id, scopeId, userId, mailboxKey, changeSequence, createTime, updateTime`
		var mailbox domain.Mailbox
		if err := sqlx.GetContext(ctx, exec, &mailbox, exec.Rebind(r.inboxSQL(query)), scopeID, userID, mailboxKey); err != nil {
			return nil, err
		}
		return &mailbox, nil
	}
	query := `INSERT INTO sys_notification_mailbox (scopeId, userId, mailboxKey, changeSequence)
VALUES (?, ?, ?, 1)
ON DUPLICATE KEY UPDATE changeSequence=changeSequence + 1, updateTime=NOW()`
	if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), scopeID, userID, mailboxKey); err != nil {
		return nil, err
	}
	return r.findMailboxForUpdate(ctx, scopeID, userID)
}

func (r *Repository) CompareAndSetInboxRecipient(ctx context.Context, item *domain.Recipient, expectedMailboxVersion int64) (bool, error) {
	if item == nil || item.ID <= 0 || item.UserID <= 0 || expectedMailboxVersion <= 0 {
		return false, fmt.Errorf("notification inbox compare-and-set is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_recipient
SET firstSeenAt=?, readAt=?, archivedAt=?, expiredAt=?, mailboxVersion=?, updateTime=NOW()
WHERE id=? AND scopeId=? AND userId=? AND mailboxVersion=?`)), item.FirstSeenAt, item.ReadAt, item.ArchivedAt, item.ExpiredAt, item.MailboxVersion, item.ID, item.ScopeID, item.UserID, expectedMailboxVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *Repository) findMailboxForUpdate(ctx context.Context, scopeID string, userID int64) (*domain.Mailbox, error) {
	if userID <= 0 || strings.TrimSpace(scopeID) == "" {
		return nil, fmt.Errorf("notification mailbox lookup is invalid")
	}
	query := `SELECT id, scopeId, userId, mailboxKey, changeSequence, createTime, updateTime
FROM sys_notification_mailbox WHERE scopeId=? AND userId=? FOR UPDATE`
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var mailbox domain.Mailbox
	if err := sqlx.GetContext(ctx, exec, &mailbox, exec.Rebind(r.inboxSQL(query)), scopeID, userID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(mailbox.MailboxKey) == "" || mailbox.ChangeSequence < 0 {
		return nil, fmt.Errorf("notification mailbox row is invalid")
	}
	return &mailbox, nil
}

func (r *Repository) findLogicalNotification(ctx context.Context, where string, args ...any) (*domain.LogicalNotification, error) {
	var item domain.LogicalNotification
	query := logicalNotificationSelectBase(r) + " WHERE " + where + " LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) findMaterializationTask(ctx context.Context, where string, args ...any) (*domain.MaterializationTask, error) {
	var item domain.MaterializationTask
	query := materializationTaskSelectBase(r) + " WHERE " + where + " LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) findDelivery(ctx context.Context, where string, arg any) (*domain.Delivery, error) {
	var item domain.Delivery
	query := deliverySelectBase(r) + " WHERE " + where + " AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), arg); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) findExternalTarget(ctx context.Context, where string, arg any) (*domain.ExternalTarget, error) {
	var item domain.ExternalTarget
	query := externalTargetSelectBase(r) + " WHERE " + where + " LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), arg); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func logicalNotificationSelectBase(r *Repository) string {
	return `SELECT id, notificationId, scopeId, organizationId, eventKey, idempotencyKey, requestFingerprint, ` + r.jsonText("audienceJson") + ` audienceJson,
category, priority, mandatory, title, content, COALESCE(deepLink, '') deepLink, COALESCE(actionType, '') actionType, COALESCE(actionLabelSnapshot, '') actionLabelSnapshot, COALESCE(actionStyle, '') actionStyle, scheduleAt, expiresAt, COALESCE(traceId, '') traceId,
status, creatorId, createTime, updateTime FROM sys_notification`
}

func materializationTaskSelectBase(r *Repository) string {
	return `SELECT id, taskId, notificationId, scopeId, ` + r.jsonText("audienceJson") + ` audienceJson, COALESCE(materializationCursor, '') materializationCursor, status,
materializedCount, retryCount, nextRunAt, COALESCE(leaseOwner, '') leaseOwner, COALESCE(leaseToken, '') leaseToken, leaseUntil,
COALESCE(lastError, '') lastError, createTime, updateTime FROM sys_notification_materialization_task`
}

func (r *Repository) recipientSelectBase() string {
	return `SELECT id, recipientId, notificationId, scopeId, userId, eventKey, category, priority, mandatory, title, content,
COALESCE(deepLink, '') deepLink, COALESCE(actionType, '') actionType, COALESCE(actionLabelSnapshot, '') actionLabelSnapshot, COALESCE(actionStyle, '') actionStyle, expiresAt, expiredAt, firstSeenAt, readAt, archivedAt, mailboxVersion, createTime, updateTime
FROM sys_notification_recipient`
}

func channelSelectBase(r *Repository) string {
	return `SELECT id, channelCode, channelName, channelType, COALESCE(scopeId, '') scopeId, COALESCE(scopeType, 'SYSTEM') scopeType, organizationId, enterpriseConnectionId, COALESCE(enterpriseConnectionVersion, 0) enterpriseConnectionVersion, COALESCE(enterpriseConnectionDigest, '') enterpriseConnectionDigest, status, priority, ` + r.jsonText("configJson") + ` configJson,
COALESCE(secretCiphertext, '') secretCiphertext, COALESCE(secretEdek, '') secretEdek, COALESCE(secretWrapKeyRef, '') secretWrapKeyRef,
` + r.jsonText("rateLimitJson") + ` rateLimitJson, ` + r.jsonText("metadataJson") + ` metadataJson, creatorId, updaterId, createTime, updateTime, isDeleted
FROM sys_notification_channel`
}

func externalTargetSelectBase(r *Repository) string {
	return `SELECT id, externalTargetId, notificationId, scopeId, COALESCE(organizationId, 0) organizationId, COALESCE(userId, 0) userId, COALESCE(externalIdentityId, 0) externalIdentityId, COALESCE(externalIdentityVersion, 0) externalIdentityVersion, COALESCE(enterpriseConnectionId, 0) enterpriseConnectionId, COALESCE(enterpriseConnectionVersion, 0) enterpriseConnectionVersion, COALESCE(authorityDigest, '') authorityDigest, COALESCE(grantType, '') grantType, connectionRef, providerCode, identityKind,
COALESCE(subjectCiphertext, '') subjectCiphertext, COALESCE(subjectEdek, '') subjectEdek, COALESCE(subjectWrapKeyRef, '') subjectWrapKeyRef,
COALESCE(subjectDigest, '') subjectDigest, COALESCE(subjectDigestKeyRef, '') subjectDigestKeyRef, ` + r.jsonText("providerParamsJson") + ` providerParamsJson,
createTime, updateTime FROM sys_notification_external_target`
}

func (r *Repository) jsonText(column string) string {
	if r != nil && r.isPostgres() {
		return `COALESCE(` + column + `::text, '')`
	}
	return `COALESCE(JSON_UNQUOTE(` + column + `), '')`
}

func (r *Repository) isPostgres() bool {
	return r != nil && dbstore.IsPostgres(r.db)
}

func (r *Repository) inboxSQL(query string) string {
	if r == nil {
		return query
	}
	return notificationPostgresRenderer.Render(r.db, query)
}

func templateSelectBase(r *Repository) string {
	return `SELECT id, templateCode, COALESCE(scopeId, '') scopeId, templateName, sceneCode, channelType, locale, COALESCE(subjectTemplate, '') subjectTemplate, COALESCE(textTemplate, '') textTemplate, COALESCE(htmlTemplate, '') htmlTemplate, COALESCE(markdownTemplate, '') markdownTemplate, ` + r.jsonText("jsonTemplate") + ` jsonTemplate, ` + r.jsonText("variablesJson") + ` variablesJson, status, version, creatorId, updaterId, createTime, updateTime, isDeleted FROM sys_notification_template`
}

func sceneBindingSelectBase(r *Repository) string {
	return `SELECT id, sceneCode, COALESCE(scopeId, '') scopeId, sceneName, channelCode, templateCode, enabled, priority, maxRetry, retryIntervalSeconds, ` + r.jsonText("metadataJson") + ` metadataJson, creatorId, updaterId, createTime, updateTime, isDeleted FROM sys_notification_scene_binding`
}

func deliverySelectBase(r *Repository) string {
	return `SELECT id, deliveryId, requestDigest, organizationId, notificationId, externalTargetId, sceneSnapshotId, sceneCode, channelCode, channelType, templateCode, COALESCE(target, '') target, COALESCE(targetMasked, '') targetMasked, ` + r.jsonText("payloadJson") + ` payloadJson, COALESCE(renderedSubject, '') renderedSubject, COALESCE(renderedText, '') renderedText, COALESCE(renderedHtml, '') renderedHtml, COALESCE(renderedMarkdown, '') renderedMarkdown, COALESCE(presentationType, 'TEXT') presentationType, COALESCE(actionLabelSnapshot, '') actionLabelSnapshot, COALESCE(actionUrlSnapshot, '') actionUrlSnapshot, COALESCE(renderedPayloadDigest, '') renderedPayloadDigest, COALESCE(contentTier, 'SENSITIVE') contentTier, status, retryCount, maxRetry, nextRetryAt, COALESCE(lastError, '') lastError, COALESCE(providerReference, '') providerReference, COALESCE(traceId, '') traceId, sentAt, creatorId, createTime, updateTime, isDeleted FROM sys_notification_delivery`
}

func normalizedChannelScopeType(item *domain.Channel) string {
	if item != nil && strings.EqualFold(strings.TrimSpace(item.ScopeType), "ORGANIZATION") {
		return "ORGANIZATION"
	}
	return "SYSTEM"
}

func normalizedDeliveryPresentation(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return domain.DeliveryPresentationText
	}
	return value
}

func nullablePositiveID(value int64) any {
	if value > 0 {
		return value
	}
	return nil
}

func selectPage[T any](ctx context.Context, db *sqlx.DB, base string, where []string, order string, current, pageSize int, args ...any) ([]T, int64, error) {
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	condition := " WHERE " + strings.Join(where, " AND ")
	exec := dbstore.SQLXExecutor(ctx, db)
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind("SELECT COUNT(1) FROM ("+base+condition+") t"), args...); err != nil {
		return nil, 0, err
	}
	query := base + condition + " ORDER BY " + order + " LIMIT ? OFFSET ?"
	pageArgs := append(append([]any{}, args...), pageSize, (current-1)*pageSize)
	var items []T
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(query), pageArgs...); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// selectInboxPage applies the notification migration's camel-case quoting to
// the complete static query, including its predicates and ordering. The
// generic legacy page helper cannot do this because it has no repository or
// database-dialect context.
func selectInboxPage[T any](ctx context.Context, r *Repository, base string, where []string, order string, current, pageSize int, args ...any) ([]T, int64, error) {
	if r == nil {
		return nil, 0, fmt.Errorf("notification repository is nil")
	}
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	condition := " WHERE " + strings.Join(where, " AND ")
	exec := dbstore.SQLXExecutor(ctx, r.db)
	var total int64
	countQuery := r.inboxSQL("SELECT COUNT(1) FROM (" + base + condition + ") t")
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind(countQuery), args...); err != nil {
		return nil, 0, err
	}
	query := r.inboxSQL(base + condition + " ORDER BY " + order + " LIMIT ? OFFSET ?")
	pageArgs := append(append([]any{}, args...), pageSize, (current-1)*pageSize)
	var items []T
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(query), pageArgs...); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func repeatedRows(row string, count int) string {
	if count <= 0 {
		return ""
	}
	rows := make([]string, count)
	for index := range rows {
		rows[index] = row
	}
	return strings.Join(rows, ",")
}

func requireNotificationTransaction(ctx context.Context) error {
	if dbstore.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("notification batch write requires an active transaction")
	}
	return nil
}

func nullJSON(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

// appendConfigurationScopeWhere binds management reads to the local runtime
// scope before pagination. Historical unscoped records remain visible only to
// the local runtime until an update claims them explicitly.
func appendConfigurationScopeWhere(where []string, args []any, scopeID string) ([]string, []any) {
	condition, scopeArgs := configurationScopeCondition(scopeID)
	if condition == "" {
		return where, args
	}
	return append(where, condition), append(args, scopeArgs...)
}

func appendConfigurationScopeCondition(query string, args []any, scopeID string) (string, []any) {
	condition, scopeArgs := configurationScopeCondition(scopeID)
	if condition == "" {
		return query, args
	}
	return query + " AND " + condition, append(args, scopeArgs...)
}

func configurationScopeCondition(scopeID string) (string, []any) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return "", nil
	}
	if scopeID == "local" {
		return "(scopeId=? OR scopeId IS NULL)", []any{scopeID}
	}
	return "scopeId=?", []any{scopeID}
}

// configurationScopeMatches mirrors configurationScopeCondition for the
// zero-affected-row path. The local runtime may still operate on historical
// records without a scope; an empty requested scope preserves the legacy
// repository behavior for callers that have not opted into scope isolation.
func configurationScopeMatches(scopeID, storedScopeID string) bool {
	scopeID = strings.TrimSpace(scopeID)
	storedScopeID = strings.TrimSpace(storedScopeID)
	if scopeID == "" {
		return true
	}
	if scopeID == "local" {
		return storedScopeID == "" || storedScopeID == "local"
	}
	return storedScopeID == scopeID
}

func configurationScopeOrder(scopeID, fallback string) (string, []any) {
	if strings.TrimSpace(scopeID) == "local" {
		// If a local replacement and a legacy configuration share a code or
		// scene, prefer the explicitly attributed local record.
		return "CASE WHEN scopeId=? THEN 0 ELSE 1 END, " + fallback, []any{"local"}
	}
	return fallback, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (r *Repository) databaseBool(value bool) any {
	if r != nil && r.isPostgres() {
		return value
	}
	return boolInt(value)
}
