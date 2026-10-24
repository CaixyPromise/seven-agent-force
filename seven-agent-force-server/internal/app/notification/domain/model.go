package domain

import (
	"strings"
	"time"
)

const (
	ChannelTypeMock     = "MOCK"
	ChannelTypeEmail    = "EMAIL"
	ChannelTypeFeishu   = "FEISHU"
	ChannelTypeWeCom    = "WECOM"
	ChannelTypeDingTalk = "DINGTALK"
	ChannelTypeWebhook  = "WEBHOOK"
	// ChannelTypeFeishuApp is the G4 Feishu tenant application channel. It is
	// dynamically addressed with a per-call open_id or chat_id.
	ChannelTypeFeishuApp = "FEISHU_APP"
	// ChannelTypeWeComApp is the G4 WeCom self-built application channel. It is
	// dynamically addressed with a per-call userid.
	ChannelTypeWeComApp = "WECOM_APP"
	// ChannelTypeHTTPConnector is a static outbound connector. It is never a
	// dynamic person or platform inbox recipient.
	ChannelTypeHTTPConnector = "HTTP_CONNECTOR"
	// ChannelTypeFeishuWebhook is a fixed Feishu group profile. Its group URL
	// and optional signing secret are connection-owned.
	ChannelTypeFeishuWebhook = "FEISHU_WEBHOOK"
	// ChannelTypeWeComWebhook is a fixed WeCom group profile. Its group URL is
	// connection-owned.
	ChannelTypeWeComWebhook = "WECOM_WEBHOOK"

	ChannelStatusEnabled  = 0
	ChannelStatusDisabled = 1

	DeliveryStatusPending  = "PENDING"
	DeliveryStatusSending  = "SENDING"
	DeliveryStatusSent     = "SENT"
	DeliveryStatusFailed   = "FAILED"
	DeliveryStatusCanceled = "CANCELED"
	// DeliveryStatusProviderAccepted means a provider accepted the request. It
	// is not evidence that a human received or read the message.
	DeliveryStatusProviderAccepted = "PROVIDER_ACCEPTED"
	// DeliveryStatusUnknown records an outcome that may have reached the
	// provider but cannot be proven from the local request result.
	DeliveryStatusUnknown = "UNKNOWN"

	// DeliveryContentTierPublic denotes content that was explicitly classified
	// as public by its originating template. It still requires the dedicated
	// per-delivery diagnostic capability before it can be read.
	DeliveryContentTierPublic = "PUBLIC"
	// DeliveryContentTierSensitive is the conservative default for historical
	// and ordinary delivery content. It requires a fresh step-up proof before
	// an operator can read a single delivery's content.
	DeliveryContentTierSensitive = "SENSITIVE"
	// DeliveryContentTierSecretEphemeral denotes a short-lived secret whose
	// rendered content exists only in the encrypted ephemeral payload store.
	// It is never copied into the normal delivery, outbox, log, or cache path.
	DeliveryContentTierSecretEphemeral = "SECRET_EPHEMERAL"

	OutboxEventNotificationDispatch = "notification.dispatch"
	OutboxAggregateNotification     = "notification"

	SceneChallengeOTP = "CHALLENGE_OTP"
)

type Channel struct {
	ID          int64  `db:"id"`
	ChannelCode string `db:"channelCode"`
	ChannelName string `db:"channelName"`
	ChannelType string `db:"channelType"`
	// ScopeID is the installation, Hub, or Node boundary that owns this
	// connection. Legacy channels may be empty until explicitly republished.
	ScopeID                     string    `db:"scopeId"`
	ScopeType                   string    `db:"scopeType"`
	OrganizationID              *int64    `db:"organizationId"`
	EnterpriseConnectionID      *int64    `db:"enterpriseConnectionId"`
	EnterpriseConnectionVersion int64     `db:"enterpriseConnectionVersion"`
	EnterpriseConnectionDigest  string    `db:"enterpriseConnectionDigest"`
	Status                      int       `db:"status"`
	Priority                    int       `db:"priority"`
	ConfigJSON                  string    `db:"configJson"`
	SecretCiphertext            string    `db:"secretCiphertext"`
	SecretEDEK                  string    `db:"secretEdek"`
	SecretWrapKeyRef            string    `db:"secretWrapKeyRef"`
	RateLimitJSON               string    `db:"rateLimitJson"`
	MetadataJSON                string    `db:"metadataJson"`
	CreatorID                   *int64    `db:"creatorId"`
	UpdaterID                   *int64    `db:"updaterId"`
	CreateTime                  time.Time `db:"createTime"`
	UpdateTime                  time.Time `db:"updateTime"`
	IsDeleted                   int       `db:"isDeleted"`
}

type Template struct {
	ID           int64  `db:"id"`
	TemplateCode string `db:"templateCode"`
	// ScopeID is the installation, Hub, or Node boundary that owns this
	// template. Empty legacy templates are readable only by the local scope.
	ScopeID          string    `db:"scopeId"`
	TemplateName     string    `db:"templateName"`
	SceneCode        string    `db:"sceneCode"`
	ChannelType      string    `db:"channelType"`
	Locale           string    `db:"locale"`
	SubjectTemplate  string    `db:"subjectTemplate"`
	TextTemplate     string    `db:"textTemplate"`
	HTMLTemplate     string    `db:"htmlTemplate"`
	MarkdownTemplate string    `db:"markdownTemplate"`
	JSONTemplate     string    `db:"jsonTemplate"`
	VariablesJSON    string    `db:"variablesJson"`
	Status           int       `db:"status"`
	Version          int       `db:"version"`
	CreatorID        *int64    `db:"creatorId"`
	UpdaterID        *int64    `db:"updaterId"`
	CreateTime       time.Time `db:"createTime"`
	UpdateTime       time.Time `db:"updateTime"`
	IsDeleted        int       `db:"isDeleted"`
}

type SceneBinding struct {
	ID        int64  `db:"id"`
	SceneCode string `db:"sceneCode"`
	// ScopeID is the installation, Hub, or Node boundary that owns this
	// binding. Empty legacy bindings are readable only by the local scope.
	ScopeID              string    `db:"scopeId"`
	SceneName            string    `db:"sceneName"`
	ChannelCode          string    `db:"channelCode"`
	TemplateCode         string    `db:"templateCode"`
	Enabled              bool      `db:"enabled"`
	Priority             int       `db:"priority"`
	MaxRetry             int       `db:"maxRetry"`
	RetryIntervalSeconds int       `db:"retryIntervalSeconds"`
	MetadataJSON         string    `db:"metadataJson"`
	CreatorID            *int64    `db:"creatorId"`
	UpdaterID            *int64    `db:"updaterId"`
	CreateTime           time.Time `db:"createTime"`
	UpdateTime           time.Time `db:"updateTime"`
	IsDeleted            int       `db:"isDeleted"`
}

type Delivery struct {
	ID             int64  `db:"id"`
	DeliveryID     string `db:"deliveryId"`
	RequestDigest  string `db:"requestDigest"`
	OrganizationID *int64 `db:"organizationId"`
	// NotificationID points to the semantic logical-notification row when this
	// delivery was created through the modern Client. Legacy V1 rows remain
	// nullable for forward compatibility.
	NotificationID *int64 `db:"notificationId"`
	// ExternalTargetID points to an encrypted third-party target snapshot. It
	// is set only for G4 external application deliveries.
	ExternalTargetID *int64 `db:"externalTargetId"`
	// SceneSnapshotID points to the G6.2 acceptance snapshot when a delivery
	// was selected from a published scene. Legacy V1 deliveries keep it nil.
	SceneSnapshotID       *int64 `db:"sceneSnapshotId"`
	SceneCode             string `db:"sceneCode"`
	ChannelCode           string `db:"channelCode"`
	ChannelType           string `db:"channelType"`
	TemplateCode          string `db:"templateCode"`
	Target                string `db:"target"`
	TargetMasked          string `db:"targetMasked"`
	PayloadJSON           string `db:"payloadJson"`
	RenderedSubject       string `db:"renderedSubject"`
	RenderedText          string `db:"renderedText"`
	RenderedHTML          string `db:"renderedHtml"`
	RenderedMarkdown      string `db:"renderedMarkdown"`
	Presentation          string `db:"presentationType"`
	ActionLabelSnapshot   string `db:"actionLabelSnapshot"`
	ActionURLSnapshot     string `db:"actionUrlSnapshot"`
	RenderedPayloadDigest string `db:"renderedPayloadDigest"`
	// ContentTier controls the narrowly-scoped diagnostic read policy. Legacy
	// rows default to SENSITIVE during read and migration so they cannot become
	// readable through a newly-added public API by accident.
	ContentTier string     `db:"contentTier"`
	Status      string     `db:"status"`
	RetryCount  int        `db:"retryCount"`
	MaxRetry    int        `db:"maxRetry"`
	NextRetryAt *time.Time `db:"nextRetryAt"`
	LastError   string     `db:"lastError"`
	// ProviderReference is a sanitized provider message identifier when the
	// provider acknowledges a request.
	ProviderReference string     `db:"providerReference"`
	TraceID           string     `db:"traceId"`
	SentAt            *time.Time `db:"sentAt"`
	CreatorID         *int64     `db:"creatorId"`
	CreateTime        time.Time  `db:"createTime"`
	UpdateTime        time.Time  `db:"updateTime"`
	IsDeleted         int        `db:"isDeleted"`
}

// DeliverySummary is the safe management-list projection. It deliberately
// omits all rendered content, payload JSON, raw destination and raw provider
// diagnostics. It is not a substitute for a diagnostic-content read.
type DeliverySummary struct {
	ID           int64      `db:"id"`
	DeliveryID   string     `db:"deliveryId"`
	SceneCode    string     `db:"sceneCode"`
	ChannelCode  string     `db:"channelCode"`
	ChannelType  string     `db:"channelType"`
	TemplateCode string     `db:"templateCode"`
	TargetMasked string     `db:"targetMasked"`
	Status       string     `db:"status"`
	RetryCount   int        `db:"retryCount"`
	MaxRetry     int        `db:"maxRetry"`
	NextRetryAt  *time.Time `db:"nextRetryAt"`
	LastError    string     `db:"lastError"`
	TraceID      string     `db:"traceId"`
	SentAt       *time.Time `db:"sentAt"`
	ContentTier  string     `db:"contentTier"`
	CreateTime   time.Time  `db:"createTime"`
	UpdateTime   time.Time  `db:"updateTime"`
}

// DeliveryEphemeralContent stores only an encrypted, TTL-bound payload for a
// one-time secret delivery. The envelope is intentionally separate from
// sys_notification_delivery so normal dispatch, management, outbox and log
// paths do not receive the plaintext secret.
type DeliveryEphemeralContent struct {
	ID         int64     `db:"id"`
	DeliveryID string    `db:"deliveryId"`
	ScopeID    string    `db:"scopeId"`
	Ciphertext string    `db:"ciphertext"`
	EDEK       string    `db:"edek"`
	WrapKeyRef string    `db:"wrapKeyRef"`
	ExpiresAt  time.Time `db:"expiresAt"`
	CreateTime time.Time `db:"createTime"`
	UpdateTime time.Time `db:"updateTime"`
}

// DeliveryDiagnosticAudit is a content-free audit record for one attempted
// diagnostic read. It records why and whether access was allowed without
// retaining content, a raw target, a credential, a provider body, or a
// one-time secret.
type DeliveryDiagnosticAudit struct {
	ID              int64     `db:"id"`
	ScopeID         string    `db:"scopeId"`
	DeliveryID      string    `db:"deliveryId"`
	ActorID         int64     `db:"actorId"`
	ContentTier     string    `db:"contentTier"`
	ReasonCode      string    `db:"reasonCode"`
	TicketReference string    `db:"ticketReference"`
	ResultCode      string    `db:"resultCode"`
	TraceID         string    `db:"traceId"`
	CreateTime      time.Time `db:"createTime"`
}

type OutboxEvent struct {
	ID         int64  `db:"id"`
	EventID    string `db:"eventId"`
	EventOwner string `db:"eventOwner"`
	// ScopeID is the installation, Hub, or Node that owns the event. A
	// notification relay must only claim events from its configured scope.
	ScopeID       string    `db:"scopeId"`
	EventType     string    `db:"eventType"`
	AggregateType string    `db:"aggregateType"`
	AggregateID   string    `db:"aggregateId"`
	Payload       string    `db:"payload"`
	Status        string    `db:"status"`
	RetryCount    int       `db:"retryCount"`
	NextRetryAt   time.Time `db:"nextRetryAt"`
	LastError     string    `db:"errorMsg"`
	CreateTime    time.Time `db:"createTime"`
	UpdateTime    time.Time `db:"updateTime"`
}

// OutboxEventSelection identifies one expected durable event. Callers use it
// when a controlled operation must relay only its own events rather than scan
// a module-wide ready queue.
type OutboxEventSelection struct {
	EventID   string
	EventType string
}

// OutboxLease is the fencing capability returned after an outbox event is
// claimed. A relay must present the token when it completes or retries work.
type OutboxLease struct {
	Token string
	Until time.Time
}

// ConsumeLease fences a message consumer invocation so a stale worker cannot
// overwrite the result of a later retry.
type ConsumeLease struct {
	Token string
	Until time.Time
}

type DeliveryMessage struct {
	MessageID  string `json:"messageId"`
	DeliveryID string `json:"deliveryId"`
	// ScopeID is a durable routing boundary. RabbitMQ topology and the
	// consumer both use it to keep one installation/Hub/Node from processing
	// another scope's delivery.
	ScopeID string `json:"scopeId"`
}

type ChannelQuery struct {
	// ScopeID is set by the notification application service from its stable
	// installation, Hub, or Node identity. It is not accepted as a caller
	// selected tenant filter.
	ScopeID     string
	Keyword     string
	ChannelType string
	Status      *int
	Current     int
	PageSize    int
}

type DeliveryQuery struct {
	// ScopeID is set only by the notification application service. Management
	// callers cannot select a different deployment, Hub, or Node scope through
	// a query parameter.
	ScopeID     string
	Keyword     string
	SceneCode   string
	ChannelCode string
	Status      string
	Current     int
	PageSize    int
}

// IsURLChannelType identifies channel kinds whose configured endpoints must
// pass the shared outbound URL SSRF guard before they are persisted or used.
func IsURLChannelType(channelType string) bool {
	switch strings.ToUpper(strings.TrimSpace(channelType)) {
	case ChannelTypeWebhook,
		ChannelTypeFeishu,
		ChannelTypeWeCom,
		ChannelTypeDingTalk,
		ChannelTypeHTTPConnector,
		ChannelTypeFeishuApp,
		ChannelTypeFeishuWebhook,
		ChannelTypeWeComApp,
		ChannelTypeWeComWebhook:
		return true
	default:
		return false
	}
}
