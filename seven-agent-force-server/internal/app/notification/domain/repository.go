package domain

import (
	"context"
	"errors"
	"time"
)

// ErrScopedConfigurationNotFound means an update addressed a notification
// configuration record outside the active runtime scope, or a record that no
// longer exists. Callers deliberately do not distinguish the two cases.
var ErrScopedConfigurationNotFound = errors.New("notification configuration was not found in the current scope")

type Repository interface {
	ListChannels(ctx context.Context, query ChannelQuery) ([]Channel, int64, error)
	FindChannelByCode(ctx context.Context, channelCode string) (*Channel, error)
	UpsertChannel(ctx context.Context, item *Channel) error
	// FindTemplateByCode is retained only for the isolated Challenge OTP
	// delivery path. Legacy template management is no longer exposed.
	FindTemplateByCode(ctx context.Context, templateCode string) (*Template, error)
	// FindActiveSceneBinding is retained only for the isolated Challenge OTP
	// delivery path. Legacy scene management is no longer exposed.
	FindActiveSceneBinding(ctx context.Context, scopeID, sceneCode string) (*SceneBinding, error)
	InsertDelivery(ctx context.Context, item *Delivery) error
	FindDeliveryByID(ctx context.Context, deliveryID string) (*Delivery, error)
	FindDeliveryByDigest(ctx context.Context, digest string) (*Delivery, error)
	ListDeliveriesByNotificationID(ctx context.Context, notificationID int64) ([]Delivery, error)
	InsertHTTPDeliverySnapshot(ctx context.Context, item *HTTPDeliverySnapshot) error
	FindHTTPDeliverySnapshotByDeliveryID(ctx context.Context, deliveryID string) (*HTTPDeliverySnapshot, error)
	ListDeliveries(ctx context.Context, query DeliveryQuery) ([]Delivery, int64, error)
	MarkDeliverySending(ctx context.Context, deliveryID string) (bool, error)
	MarkDeliverySent(ctx context.Context, deliveryID string, sentAt time.Time) error
	MarkDeliveryRetry(ctx context.Context, deliveryID string, retryCount int, nextRetryAt time.Time, lastError string) error
	MarkDeliveryFailed(ctx context.Context, deliveryID string, retryCount int, lastError string) error
	MarkDeliveryProviderAccepted(ctx context.Context, deliveryID, providerReference string, acceptedAt time.Time) error
	MarkDeliveryUnknown(ctx context.Context, deliveryID, diagnostic string) error
	InsertDeliveryAttempt(ctx context.Context, item *DeliveryAttempt) error
	InsertExternalTargets(ctx context.Context, items []ExternalTarget) error
	FindExternalTargetByID(ctx context.Context, externalTargetID int64) (*ExternalTarget, error)
	ListExternalTargetsByNotificationID(ctx context.Context, notificationID int64) ([]ExternalTarget, error)
	AppendOutbox(ctx context.Context, event *OutboxEvent) error
	ListReadyOutbox(ctx context.Context, limit int) ([]OutboxEvent, error)
	FindReadyOutbox(ctx context.Context, eventID, eventType string) (*OutboxEvent, error)
	ListUnknownOutbox(ctx context.Context, limit int) ([]OutboxEvent, error)
	TryClaimOutbox(ctx context.Context, id int64, eventType, worker string) (*OutboxLease, bool, error)
	MarkOutbox(ctx context.Context, id int64, eventType, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error)
	BeginConsume(ctx context.Context, messageID, consumer, worker, detail string) (*ConsumeLease, bool, error)
	MarkConsumed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
	MarkConsumeFailed(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error)
	FindLogicalNotificationByIdempotency(ctx context.Context, scopeID, eventKey, idempotencyKey string) (*LogicalNotification, error)
	FindLogicalNotificationByID(ctx context.Context, notificationID int64) (*LogicalNotification, error)
	CreateLogicalNotification(ctx context.Context, item *LogicalNotification) (bool, error)
	MarkLogicalNotificationMaterialized(ctx context.Context, notificationID int64) error
	InsertRecipients(ctx context.Context, items []Recipient) (int, error)
	InsertInboxRecipients(ctx context.Context, items []Recipient) ([]Recipient, error)
	CreateMaterializationTask(ctx context.Context, item *MaterializationTask) (bool, error)
	FindMaterializationTaskByNotificationID(ctx context.Context, notificationID int64) (*MaterializationTask, error)
	ListReadyMaterializationTasks(ctx context.Context, scopeID string, limit int) ([]MaterializationTask, error)
	TryClaimMaterializationTask(ctx context.Context, scopeID string, taskID int64, worker string, now time.Time) (*MaterializationTask, bool, error)
	AdvanceMaterializationTask(ctx context.Context, scopeID string, taskID int64, leaseToken, cursor, status string, materializedCount int64, nextRunAt time.Time) (bool, error)
	FailMaterializationTask(ctx context.Context, scopeID string, taskID int64, leaseToken, status, lastError string, retryCount int, nextRunAt time.Time) (bool, error)
	ListInboxRecipients(ctx context.Context, query InboxQuery) ([]Recipient, error)
	ListUnreadInboxRecipients(ctx context.Context, scopeID string, userID int64, limit int) ([]Recipient, error)
	ListInboxRecipientChanges(ctx context.Context, query InboxChangeQuery) ([]Recipient, error)
	ListExpiredInboxRecipients(ctx context.Context, scopeID string, now time.Time, limit int) ([]Recipient, error)
	LockExpiredInboxRecipient(ctx context.Context, recipientID int64, now time.Time) (*Recipient, error)
	FindInboxRecipient(ctx context.Context, scopeID string, userID int64, recipientID string) (*Recipient, error)
	CountUnreadInboxRecipients(ctx context.Context, scopeID string, userID int64) (int64, error)
	LockMailbox(ctx context.Context, scopeID string, userID int64, mailboxKey string) (*Mailbox, error)
	AdvanceMailboxChange(ctx context.Context, scopeID string, userID int64) (*Mailbox, error)
	CompareAndSetInboxRecipient(ctx context.Context, item *Recipient, expectedMailboxVersion int64) (bool, error)
}

// TemplateRevisionRepository is deliberately separate from Repository so the
// additive G6.1 template workspace does not force every existing notification
// test double or delivery adapter to implement unrelated revision methods.
// Production SQL repositories implement both interfaces.
type TemplateRevisionRepository interface {
	ListTemplateDefinitions(ctx context.Context, query TemplateDefinitionQuery) ([]TemplateDefinition, int64, error)
	FindTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*TemplateDefinition, error)
	FindTemplateDefinitionByID(ctx context.Context, definitionID int64) (*TemplateDefinition, error)
	LockTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*TemplateDefinition, error)
	FindTemplateRevisionByID(ctx context.Context, revisionID int64) (*TemplateRevision, error)
	ListTemplateRevisionsByIDs(ctx context.Context, revisionIDs []int64) ([]TemplateRevision, error)
	FindTemplateRevisionByDefinitionAndState(ctx context.Context, definitionID int64, state string) (*TemplateRevision, error)
	// ListTemplateRevisionsByDefinition returns every immutable revision in
	// newest-first order. The caller must first scope-check the definition.
	ListTemplateRevisionsByDefinition(ctx context.Context, definitionID int64) ([]TemplateRevision, error)
	InsertTemplateDefinition(ctx context.Context, item *TemplateDefinition) error
	InsertTemplateRevision(ctx context.Context, item *TemplateRevision) error
	UpdateTemplateDefinitionMetadata(ctx context.Context, definitionID int64, templateName, locale string, actorID int64) error
	UpdateTemplateRevisionDraft(ctx context.Context, item *TemplateRevision, expectedVersion int) (bool, error)
	SetTemplateDefinitionDraft(ctx context.Context, definitionID, revisionID int64, expectedDefinitionVersion int) (bool, error)
	PublishTemplateRevision(ctx context.Context, definitionID, revisionID int64, expectedRevisionVersion int, actorID int64, publishedAt time.Time) (bool, error)
	InsertTemplateRevisionAudit(ctx context.Context, item *TemplateRevisionAudit) error
}

// SceneRevisionRepository is deliberately separate from Repository for the
// same reason as TemplateRevisionRepository: the additive G6.2 workspace
// must not force unrelated legacy delivery test doubles to implement scene
// authoring or acceptance-snapshot storage. Production SQL repositories
// implement this interface in addition to Repository.
type SceneRevisionRepository interface {
	ListSceneDefinitions(ctx context.Context, query SceneDefinitionQuery) ([]SceneDefinition, int64, error)
	FindSceneDefinitionByCodeAndReceiverKind(ctx context.Context, scopeID, sceneCode, receiverKind string) (*SceneDefinition, error)
	FindSceneDefinitionByID(ctx context.Context, definitionID int64) (*SceneDefinition, error)
	LockSceneDefinitionByCodeAndReceiverKind(ctx context.Context, scopeID, sceneCode, receiverKind string) (*SceneDefinition, error)
	FindSceneRevisionByID(ctx context.Context, revisionID int64) (*SceneRevision, error)
	ListSceneRevisionsByIDs(ctx context.Context, revisionIDs []int64) ([]SceneRevision, error)
	ListSceneRevisionsByDefinition(ctx context.Context, definitionID int64) ([]SceneRevision, error)
	InsertSceneDefinition(ctx context.Context, item *SceneDefinition) error
	InsertSceneRevision(ctx context.Context, item *SceneRevision) error
	UpdateSceneDefinitionMetadata(ctx context.Context, definitionID int64, sceneName string, actorID int64) error
	UpdateSceneRevisionDraft(ctx context.Context, item *SceneRevision, expectedVersion int) (bool, error)
	SetSceneDefinitionDraft(ctx context.Context, definitionID, revisionID int64, expectedDefinitionVersion int) (bool, error)
	PublishSceneRevision(ctx context.Context, definitionID, revisionID int64, expectedRevisionVersion int, actorID int64, publishedAt time.Time) (bool, error)
	InsertSceneRevisionAudit(ctx context.Context, item *SceneRevisionAudit) error
	InsertSceneSnapshot(ctx context.Context, item *SceneSnapshot) error
	ListSceneSnapshotsByNotificationID(ctx context.Context, notificationID int64) ([]SceneSnapshot, error)
}

// DeliveryDiagnosticsRepository is separate from Repository so the G6.3
// diagnostic surface does not force focused legacy test doubles or delivery
// adapters to gain access to diagnostic content. Production SQL repositories
// implement this narrow, scope-bound port.
type DeliveryDiagnosticsRepository interface {
	// ListDeliverySummaries returns only the safe management projection for the
	// current runtime scope.
	ListDeliverySummaries(ctx context.Context, query DeliveryQuery) ([]DeliverySummary, int64, error)
	// FindDeliveryForDiagnostic returns exactly one delivery only after a
	// scope-bound SQL lookup. Callers must not fall back to the unscoped
	// delivery repository methods for an operator diagnostic read.
	FindDeliveryForDiagnostic(ctx context.Context, scopeID, deliveryID string) (*Delivery, error)
	FindDeliveryEphemeralContent(ctx context.Context, scopeID, deliveryID string) (*DeliveryEphemeralContent, error)
	InsertDeliveryEphemeralContent(ctx context.Context, item *DeliveryEphemeralContent) error
	InsertDeliveryDiagnosticAudit(ctx context.Context, item *DeliveryDiagnosticAudit) error
}
