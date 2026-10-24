package facade

import (
	"context"
	"time"
)

type WorkPreference struct {
	ID             int64  `json:"id,string"`
	OrganizationID int64  `json:"organizationId,string"`
	UserID         int64  `json:"userId,string"`
	EventFamily    string `json:"eventFamily"`
	ChannelKind    string `json:"channelKind"`
	Enabled        bool   `json:"enabled"`
	Version        int64  `json:"version,string"`
}

type WorkPreferenceUpdate struct {
	Enabled         bool  `json:"enabled"`
	ExpectedVersion int64 `json:"expectedVersion,string"`
}

type NotificationFacade interface {
	NotificationClient
	EnqueueChallengeOTP(ctx context.Context, request ChallengeOTPRequest) error
	ResolveReadiness(ctx context.Context, sceneCode string, userID int64) (*Readiness, error)
	ResolveReadinessForOrganization(ctx context.Context, organizationID int64, sceneCode string, userID int64) (*Readiness, error)
	ListAvailableGroupRoutes(ctx context.Context, organizationID, userID int64, sceneCode string) ([]GroupRouteOption, error)
	ResolvePublicationStatus(ctx context.Context, organizationID int64, receiptIdentity string, viewerUserID int64) (*PublicationStatus, error)
	IsWorkPreferenceEnabled(ctx context.Context, organizationID, userID int64, eventFamily, channelKind string) (bool, error)
}

type GroupRouteOption struct {
	ID           int64  `json:"id,string"`
	Name         string `json:"name"`
	Provider     string `json:"provider"`
	MentionReady bool   `json:"mentionReady"`
}

type PublicationChannelStatus struct {
	ChannelKind    string     `json:"channelKind"`
	DisplayName    string     `json:"displayName"`
	State          string     `json:"state"`
	MentionState   string     `json:"mentionState"`
	SafeReasonCode string     `json:"safeReasonCode,omitempty"`
	AcceptedAt     *time.Time `json:"acceptedAt,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type PublicationStatus struct {
	Channels []PublicationChannelStatus `json:"channels"`
}

type GroupRouteRecord struct {
	ID                     int64      `json:"id,string"`
	Name                   string     `json:"name"`
	Provider               string     `json:"provider"`
	Transport              string     `json:"transport"`
	RequiredPermissionCode string     `json:"requiredPermissionCode"`
	Status                 string     `json:"status"`
	Version                int64      `json:"version,string"`
	LastProbeStatus        string     `json:"lastProbeStatus"`
	LastProbeReasonCode    string     `json:"lastProbeReasonCode,omitempty"`
	LastProbeAt            *time.Time `json:"lastProbeAt,omitempty"`
	ReferencedTaskCount    int64      `json:"referencedTaskCount"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type GroupRouteSaveRequest struct {
	ID                     int64  `json:"id,string,omitempty"`
	Name                   string `json:"name"`
	Provider               string `json:"provider"`
	Transport              string `json:"transport"`
	RequiredPermissionCode string `json:"requiredPermissionCode"`
	EnterpriseConnectionID int64  `json:"enterpriseConnectionId,string,omitempty"`
	CandidateToken         string `json:"candidateToken,omitempty"`
	WebhookURL             string `json:"webhookUrl,omitempty"`
	WebhookSigningSecret   string `json:"webhookSigningSecret,omitempty"`
	ExpectedVersion        int64  `json:"expectedVersion,string,omitempty"`
}

type GroupRouteStateRequest struct {
	ExpectedVersion int64 `json:"expectedVersion,string"`
}

type GroupRouteImpact struct {
	ActiveTasks          int64 `json:"activeTasks"`
	ResumableTasks       int64 `json:"resumableTasks"`
	Subscriptions        int64 `json:"subscriptions"`
	PendingRecords       int64 `json:"pendingRecords"`
	PendingDeliveries    int64 `json:"pendingDeliveries"`
	HistoricalReferences int64 `json:"historicalReferences"`
	CanDelete            bool  `json:"canDelete"`
}

type GroupCandidate struct {
	CandidateToken string `json:"candidateToken"`
	DisplayName    string `json:"displayName"`
	Description    string `json:"description,omitempty"`
}

type ReadinessCapability struct {
	Available      bool   `json:"available"`
	RecipientReady bool   `json:"recipientReady,omitempty"`
	DirectReady    bool   `json:"directReady,omitempty"`
	MentionReady   bool   `json:"mentionReady,omitempty"`
	GroupReady     bool   `json:"groupReady,omitempty"`
	BoundRouteName string `json:"boundRouteName,omitempty"`
	ReasonCode     string `json:"reasonCode,omitempty"`
}

type Readiness struct {
	SceneCode string              `json:"sceneCode"`
	InApp     ReadinessCapability `json:"inApp"`
	Email     ReadinessCapability `json:"email"`
	Feishu    ReadinessCapability `json:"feishu"`
	WeCom     ReadinessCapability `json:"wecom"`
	TaskLink  ReadinessCapability `json:"taskLink"`
	Action    struct {
		Configured bool `json:"configured"`
	} `json:"action"`
}

// EnterpriseConnectionTestRequest sends one deliberately supplied third-party
// target a short probe through an already saved application connection. It is
// an operator diagnostic only: it creates no inbox recipient and persists no
// target, provider token, request body, or third-party read state.
type EnterpriseConnectionTestRequest struct {
	ConnectionRef  string               `json:"connectionRef"`
	IdentityKind   ExternalIdentityKind `json:"identityKind"`
	Subject        string               `json:"subject"`
	ProviderParams map[string]any       `json:"providerParams,omitempty"`
	Text           string               `json:"text,omitempty"`
}

// ProviderError contains the bounded source fields returned by a provider for
// a controlled probe. It deliberately excludes raw bodies, credentials,
// token values, endpoint URLs, and the supplied third-party target.
type ProviderError struct {
	Provider   string `json:"provider"`
	HTTPStatus int    `json:"httpStatus,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	LogID      string `json:"logId,omitempty"`
}

// EnterpriseConnectionTestResult contains the normalized delivery outcome and
// a bounded source-provider error when the controlled probe did not succeed.
// It never exposes credentials, token values, raw target, provider response
// body, or a claim of human receipt/read.
type EnterpriseConnectionTestResult struct {
	Status            string                     `json:"status"`
	FailureClass      string                     `json:"failureClass,omitempty"`
	ProviderReference string                     `json:"providerReference,omitempty"`
	Diagnostic        string                     `json:"diagnostic,omitempty"`
	ProviderError     *ProviderError             `json:"providerError,omitempty"`
	Warnings          []ProviderParameterWarning `json:"warnings,omitempty"`
}

// StaticConnectionTestRequest probes one saved HTTP Connector or fixed group
// webhook. It accepts no recipient, URL, headers, secret or raw body, so the
// operator can validate the saved connection without creating a notification
// or changing its outbound capability.
type StaticConnectionTestRequest struct {
	ConnectionRef string `json:"connectionRef"`
	Text          string `json:"text,omitempty"`
}

// StaticConnectionTestResult is the bounded result of a non-persistent static
// connection probe. ProviderError is populated only from a short known-shape
// failed response and never includes a raw response body or connection secret.
type StaticConnectionTestResult struct {
	Status            string         `json:"status"`
	FailureClass      string         `json:"failureClass,omitempty"`
	ProviderReference string         `json:"providerReference,omitempty"`
	Diagnostic        string         `json:"diagnostic,omitempty"`
	ProviderError     *ProviderError `json:"providerError,omitempty"`
}

// ProviderChannelConfig is the small public configuration surface for the
// supported enterprise application channels. Secrets are always supplied via
// SecretPlain and are never returned by read APIs.
type ProviderChannelConfig struct {
	FeishuAppID  string `json:"feishuAppId,omitempty"`
	WeComCorpID  string `json:"weComCorpId,omitempty"`
	WeComAgentID string `json:"weComAgentId,omitempty"`
}

// HTTPConnectorConfig is the small typed management surface for a static
// HTTP connector. It has no URL override, proxy, credential value, raw body,
// expression or script field. The backend derives the secret reference from
// AuthenticationMode and keeps it out of read responses.
type HTTPConnectorConfig struct {
	EndpointURL         string                      `json:"endpointUrl"`
	EgressPolicyRef     string                      `json:"egressPolicyRef,omitempty"`
	Method              string                      `json:"method"`
	AuthenticationMode  string                      `json:"authenticationMode"`
	FieldMappings       []HTTPConnectorFieldMapping `json:"fieldMappings"`
	HeaderAllowlist     []string                    `json:"headerAllowlist,omitempty"`
	IdempotencyHeader   string                      `json:"idempotencyHeader"`
	TimeoutMilliseconds int                         `json:"timeoutMilliseconds"`
	SuccessStatusCodes  []int                       `json:"successStatusCodes,omitempty"`
}

// HTTPConnectorFieldMapping maps one approved notification field to a
// top-level request JSON property. The mapping is deliberately not an
// expression language or arbitrary body template.
type HTTPConnectorFieldMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// WebhookProfileConfig is the typed, non-secret management surface for a
// fixed Feishu or WeCom group profile. The URL and provider signing material
// are accepted only as secret write inputs and are never returned here.
type WebhookProfileConfig struct {
	TimeoutMilliseconds int   `json:"timeoutMilliseconds"`
	SuccessStatusCodes  []int `json:"successStatusCodes,omitempty"`
}

// ProviderParameterDescriptor describes one provider-declared optional
// delivery value for the administration UI. It is not a raw payload schema.
type ProviderParameterDescriptor struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	ValueType     string `json:"valueType"`
	MaxItems      int    `json:"maxItems,omitempty"`
	MaxValueBytes int    `json:"maxValueBytes,omitempty"`
	AllowDefault  bool   `json:"allowDefault"`
}

// ProviderParameterSetting is an operator-configured enable/default row.
// Its JSON representation is internal persistence only; the UI edits this
// structured row through an editable table.
type ProviderParameterSetting struct {
	Key          string `json:"key"`
	Enabled      bool   `json:"enabled"`
	DefaultValue any    `json:"defaultValue,omitempty"`
}

type ChallengeOTPRequest struct {
	ToEmail   string         `json:"toEmail"`
	Code      string         `json:"code"`
	Scene     string         `json:"scene"`
	SceneName string         `json:"sceneName"`
	TTL       time.Duration  `json:"ttl"`
	Metadata  map[string]any `json:"metadata"`
}

type PageResult[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Current  int   `json:"current"`
	PageSize int   `json:"pageSize"`
}

type ChannelRecord struct {
	ID                        int64                         `json:"id,string"`
	ChannelCode               string                        `json:"channelCode"`
	ChannelName               string                        `json:"channelName"`
	ChannelType               string                        `json:"channelType"`
	Status                    int                           `json:"status"`
	Priority                  int                           `json:"priority"`
	ConfigJSON                string                        `json:"configJson"`
	RateLimitJSON             string                        `json:"rateLimitJson"`
	MetadataJSON              string                        `json:"metadataJson"`
	ProviderConfig            *ProviderChannelConfig        `json:"providerConfig,omitempty"`
	HTTPConnectorConfig       *HTTPConnectorConfig          `json:"httpConnectorConfig,omitempty"`
	WebhookProfileConfig      *WebhookProfileConfig         `json:"webhookProfileConfig,omitempty"`
	ProviderParameterCatalog  []ProviderParameterDescriptor `json:"providerParameterCatalog,omitempty"`
	ProviderParameterSettings []ProviderParameterSetting    `json:"providerParameterSettings,omitempty"`
	SecretConfigured          bool                          `json:"secretConfigured"`
	CreateTime                time.Time                     `json:"createTime"`
	UpdateTime                time.Time                     `json:"updateTime"`
}

type ChannelUpsertRequest struct {
	ID                   int64                  `json:"id,string"`
	ChannelCode          string                 `json:"channelCode"`
	ChannelName          string                 `json:"channelName"`
	ChannelType          string                 `json:"channelType"`
	Status               int                    `json:"status"`
	Priority             int                    `json:"priority"`
	ConfigJSON           string                 `json:"configJson"`
	SecretPlain          string                 `json:"secretPlain"`
	RateLimitJSON        string                 `json:"rateLimitJson"`
	MetadataJSON         string                 `json:"metadataJson"`
	ProviderConfig       *ProviderChannelConfig `json:"providerConfig,omitempty"`
	HTTPConnectorConfig  *HTTPConnectorConfig   `json:"httpConnectorConfig,omitempty"`
	WebhookProfileConfig *WebhookProfileConfig  `json:"webhookProfileConfig,omitempty"`
	// WebhookURL and WebhookSigningSecret are write-only secret inputs for the
	// fixed group profiles. The server encrypts a validated internal envelope
	// and never echoes either value in ChannelRecord.
	WebhookURL                string                     `json:"webhookUrl,omitempty"`
	WebhookSigningSecret      string                     `json:"webhookSigningSecret,omitempty"`
	ProviderParameterSettings []ProviderParameterSetting `json:"providerParameterSettings,omitempty"`
}

// TemplateRevisionVariable is the public G6.1 schema row. It intentionally
// has no raw JSON field: the API and UI exchange structured variable data.
type TemplateRevisionVariable struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Required       bool   `json:"required"`
	MaxLength      int    `json:"maxLength,omitempty"`
	SampleValue    any    `json:"sampleValue,omitempty"`
	Classification string `json:"classification"`
}

// TemplateRevisionDraftInput is the versioned-template authoring shape. It
// deliberately contains no route, channel, scene, recipient or secret.
type TemplateRevisionDraftInput struct {
	TemplateName               string                     `json:"templateName"`
	Locale                     string                     `json:"locale"`
	SubjectTemplate            string                     `json:"subjectTemplate"`
	TextTemplate               string                     `json:"textTemplate"`
	HTMLTemplate               string                     `json:"htmlTemplate"`
	MarkdownTemplate           string                     `json:"markdownTemplate"`
	PrimaryActionLabelTemplate string                     `json:"primaryActionLabelTemplate"`
	Variables                  []TemplateRevisionVariable `json:"variables"`
}

type TemplateRevisionRecord struct {
	ID                         int64                      `json:"id,string"`
	RevisionNo                 int                        `json:"revisionNo"`
	State                      string                     `json:"state"`
	RevisionVersion            int                        `json:"revisionVersion"`
	SubjectTemplate            string                     `json:"subjectTemplate"`
	TextTemplate               string                     `json:"textTemplate"`
	HTMLTemplate               string                     `json:"htmlTemplate"`
	MarkdownTemplate           string                     `json:"markdownTemplate"`
	PrimaryActionLabelTemplate string                     `json:"primaryActionLabelTemplate"`
	Variables                  []TemplateRevisionVariable `json:"variables"`
	ContentDigest              string                     `json:"contentDigest"`
	PublishedAt                *time.Time                 `json:"publishedAt,omitempty"`
	PublishedBy                *int64                     `json:"publishedBy,string,omitempty"`
	CreateTime                 time.Time                  `json:"createTime"`
	UpdateTime                 time.Time                  `json:"updateTime"`
}

// TemplateDefinitionRecord is the versioned template identity used by current
// authoring and delivery selection.
type TemplateDefinitionRecord struct {
	ID               int64                   `json:"id,string"`
	TemplateCode     string                  `json:"templateCode"`
	TemplateName     string                  `json:"templateName"`
	Locale           string                  `json:"locale"`
	CurrentDraft     *TemplateRevisionRecord `json:"currentDraft,omitempty"`
	CurrentPublished *TemplateRevisionRecord `json:"currentPublished,omitempty"`
	// Revisions is returned by the detail and mutation responses so a published
	// or superseded revision remains readable without becoming editable.
	Revisions  []TemplateRevisionRecord `json:"revisions,omitempty"`
	Version    int                      `json:"version"`
	CreateTime time.Time                `json:"createTime"`
	UpdateTime time.Time                `json:"updateTime"`
}

type TemplateDefinitionCreateRequest struct {
	TemplateCode string                     `json:"templateCode"`
	Draft        TemplateRevisionDraftInput `json:"draft"`
}

type TemplateRevisionSaveRequest struct {
	ExpectedVersion int                        `json:"expectedVersion"`
	Draft           TemplateRevisionDraftInput `json:"draft"`
}

type TemplateRevisionPublishRequest struct {
	ExpectedVersion int `json:"expectedVersion"`
}

type TemplateRevisionPreviewRequest struct {
	Draft     TemplateRevisionDraftInput `json:"draft"`
	Variables map[string]any             `json:"variables"`
}

type TemplateRevisionPreviewResponse struct {
	Subject            string `json:"subject"`
	Text               string `json:"text"`
	HTML               string `json:"html"`
	Markdown           string `json:"markdown"`
	PrimaryActionLabel string `json:"primaryActionLabel"`
}

// SceneRevisionDraftInput is the deliberately small G6.2 authoring shape.
// The receiver kind plus optional saved connection forms one sending way;
// dynamic members, groups, URLs, credentials and provider parameters never
// belong to this configuration.
type SceneRevisionDraftInput struct {
	SceneName          string `json:"sceneName"`
	ReceiverKind       string `json:"receiverKind"`
	TemplateRevisionID int64  `json:"templateRevisionId,string"`
	ConnectionRef      string `json:"connectionRef,omitempty"`
	Enabled            bool   `json:"enabled"`
}

// SceneRevisionRecord is safe management metadata. It exposes no message
// body, template variable input, target, URL or credential.
type SceneRevisionRecord struct {
	ID                 int64      `json:"id,string"`
	RevisionNo         int        `json:"revisionNo"`
	State              string     `json:"state"`
	RevisionVersion    int        `json:"revisionVersion"`
	Enabled            bool       `json:"enabled"`
	TemplateRevisionID int64      `json:"templateRevisionId,string"`
	ConnectionRef      string     `json:"connectionRef,omitempty"`
	SendingWay         string     `json:"sendingWay"`
	PublishedAt        *time.Time `json:"publishedAt,omitempty"`
	PublishedBy        *int64     `json:"publishedBy,string,omitempty"`
	CreateTime         time.Time  `json:"createTime"`
	UpdateTime         time.Time  `json:"updateTime"`
}

// SceneDefinitionRecord is the current versioned scene management identity.
type SceneDefinitionRecord struct {
	ID               int64                 `json:"id,string"`
	SceneCode        string                `json:"sceneCode"`
	SceneName        string                `json:"sceneName"`
	ReceiverKind     string                `json:"receiverKind"`
	CurrentDraft     *SceneRevisionRecord  `json:"currentDraft,omitempty"`
	CurrentPublished *SceneRevisionRecord  `json:"currentPublished,omitempty"`
	Revisions        []SceneRevisionRecord `json:"revisions,omitempty"`
	Version          int                   `json:"version"`
	CreateTime       time.Time             `json:"createTime"`
	UpdateTime       time.Time             `json:"updateTime"`
}

type SceneDefinitionCreateRequest struct {
	SceneCode string                  `json:"sceneCode"`
	Draft     SceneRevisionDraftInput `json:"draft"`
}

type SceneRevisionSaveRequest struct {
	ExpectedVersion int                     `json:"expectedVersion"`
	Draft           SceneRevisionDraftInput `json:"draft"`
}

type SceneRevisionPublishRequest struct {
	ExpectedVersion int `json:"expectedVersion"`
}

type DeliveryRecord struct {
	ID                int64      `json:"id,string"`
	DeliveryID        string     `json:"deliveryId"`
	SceneCode         string     `json:"sceneCode"`
	ChannelCode       string     `json:"channelCode"`
	ChannelType       string     `json:"channelType"`
	TemplateCode      string     `json:"templateCode"`
	TargetMasked      string     `json:"targetMasked"`
	Status            string     `json:"status"`
	RetryCount        int        `json:"retryCount"`
	MaxRetry          int        `json:"maxRetry"`
	NextRetryAt       *time.Time `json:"nextRetryAt"`
	LastError         string     `json:"lastError"`
	TraceID           string     `json:"traceId"`
	SentAt            *time.Time `json:"sentAt"`
	CreateTime        time.Time  `json:"createTime"`
	UpdateTime        time.Time  `json:"updateTime"`
	RenderedSubject   string     `json:"renderedSubject"`
	ProviderReference string     `json:"providerReference,omitempty"`
}

// DeliverySummaryRecord is the safe delivery-log contract used by ordinary
// management pages. It intentionally has no rendered content, payload JSON,
// raw third-party target, raw provider response, credential, or secret.
type DeliverySummaryRecord struct {
	ID             int64      `json:"id,string"`
	DeliveryID     string     `json:"deliveryId"`
	SceneCode      string     `json:"sceneCode"`
	ChannelCode    string     `json:"channelCode"`
	ChannelType    string     `json:"channelType"`
	TemplateCode   string     `json:"templateCode"`
	TargetMasked   string     `json:"targetMasked"`
	Status         string     `json:"status"`
	RetryCount     int        `json:"retryCount"`
	MaxRetry       int        `json:"maxRetry"`
	NextRetryAt    *time.Time `json:"nextRetryAt,omitempty"`
	FailureCode    string     `json:"failureCode,omitempty"`
	FailureMessage string     `json:"failureMessage,omitempty"`
	TraceID        string     `json:"traceId,omitempty"`
	SentAt         *time.Time `json:"sentAt,omitempty"`
	CreateTime     time.Time  `json:"createTime"`
	UpdateTime     time.Time  `json:"updateTime"`
}

// DeliveryDiagnosticContentRequest carries the minimal accountable context
// for one content read. It never accepts a target, a template override, a
// provider parameter, a credential, or content in the request body.
type DeliveryDiagnosticContentRequest struct {
	ReasonCode      string `json:"reasonCode"`
	TicketReference string `json:"ticketReference,omitempty"`
}

// DeliveryDiagnosticContent is returned only by the dedicated single-record
// diagnostic endpoint after the relevant capability and, where necessary,
// fresh authentication proof have passed. HTML and Markdown are deliberately
// excluded so the browser never executes message markup in this workspace.
type DeliveryDiagnosticContent struct {
	DeliveryID  string     `json:"deliveryId"`
	ContentTier string     `json:"contentTier"`
	Subject     string     `json:"subject,omitempty"`
	Text        string     `json:"text,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}
