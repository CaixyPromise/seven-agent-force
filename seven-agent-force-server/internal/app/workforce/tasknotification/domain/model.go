package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type FactType string

const (
	FactActionRequired    FactType = "TASK_ACTION_REQUIRED"
	FactInterrupted       FactType = "TASK_INTERRUPTED"
	FactCompleted         FactType = "TASK_COMPLETED"
	FactDeliveryCompleted FactType = "DELIVERY_COMPLETED"
)

type SourceType string

const (
	SourceContinuationCheckpoint SourceType = "CONTINUATION_CHECKPOINT"
	SourceFinalizationRun        SourceType = "FINALIZATION_RUN"
	SourceResultEvaluation       SourceType = "RESULT_EVALUATION"
	SourceDeliveryRequest        SourceType = "DELIVERY_REQUEST"
	SourceReviewOperation        SourceType = "REVIEW_OPERATION"
	SourceIndependentReviewRun   SourceType = "INDEPENDENT_REVIEW_RUN"
	SourceRuntimeRecovery        SourceType = "RUNTIME_RECOVERY"
	SourceTaskTimeout            SourceType = "TASK_TIMEOUT"
	SourceArtifactUpload         SourceType = "ARTIFACT_UPLOAD"
	SourceTaskAttempt            SourceType = "TASK_ATTEMPT"
	SourceTaskCancel             SourceType = "TASK_CANCEL"
)

type Focus string

const (
	FocusEvaluation   Focus = "evaluation"
	FocusResult       Focus = "result"
	FocusDelivery     Focus = "delivery"
	FocusContinuation Focus = "continuation"
)

type ChannelType string

const (
	ChannelInApp        ChannelType = "IN_APP"
	ChannelEmailUser    ChannelType = "EMAIL_USER"
	ChannelFeishuDirect ChannelType = "FEISHU_DIRECT"
	ChannelWeComDirect  ChannelType = "WECOM_DIRECT"
	ChannelGroupRoute   ChannelType = "GROUP_ROUTE"
)

const (
	RecordPending    = "PENDING"
	RecordProcessing = "PROCESSING"
	RecordRetryWait  = "RETRY_WAIT"
	RecordPublished  = "PUBLISHED"
	RecordSuppressed = "SUPPRESSED"
	RecordFailed     = "FAILED"
)

var (
	ErrFactInvalid               = errors.New("TASK_NOTIFICATION_FACT_INVALID")
	ErrSubscriptionConflict      = errors.New("TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT")
	ErrRecipientInvalid          = errors.New("TASK_NOTIFICATION_RECIPIENT_INVALID")
	ErrStaleTurn                 = errors.New("TASK_NOTIFICATION_STALE_TURN")
	ErrDeepLinkInvalid           = errors.New("TASK_NOTIFICATION_DEEP_LINK_INVALID")
	ErrIdempotencyConflict       = errors.New("TASK_NOTIFICATION_IDEMPOTENCY_CONFLICT")
	ErrPublishRetryable          = errors.New("TASK_NOTIFICATION_PUBLISH_RETRYABLE")
	ErrPublishFailed             = errors.New("TASK_NOTIFICATION_PUBLISH_FAILED")
	ErrSceneNotConfigured        = errors.New("TASK_NOTIFICATION_SCENE_NOT_CONFIGURED")
	ErrPublicWebURLNotConfigured = errors.New("PUBLIC_WEB_BASE_URL_NOT_CONFIGURED")
	ErrEnterpriseConnection      = errors.New("ENTERPRISE_CONNECTION_NOT_CONFIGURED")
	ErrExternalIdentity          = errors.New("EXTERNAL_IDENTITY_NOT_VERIFIED")
	ErrEnterpriseGrant           = errors.New("ENTERPRISE_IDENTITY_GRANT_REQUIRED")
	ErrExternalLoginGrant        = errors.New("EXTERNAL_LOGIN_GRANT_REQUIRED")
)

var safeReasonCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

type ChannelSelection struct {
	Type         ChannelType `json:"kind"`
	GroupRouteID int64       `json:"routeId,string,omitempty"`
}

func (c *ChannelSelection) UnmarshalJSON(data []byte) error {
	var wire struct {
		Kind        ChannelType `json:"kind"`
		LegacyType  ChannelType `json:"type"`
		RouteID     json.Number `json:"routeId"`
		LegacyRoute string      `json:"routeRef"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	c.Type = wire.Kind
	if c.Type == "" {
		c.Type = wire.LegacyType
	}
	if wire.RouteID != "" {
		value, err := strconv.ParseInt(string(wire.RouteID), 10, 64)
		if err != nil {
			return err
		}
		c.GroupRouteID = value
	}
	if c.GroupRouteID == 0 && strings.TrimSpace(wire.LegacyRoute) != "" {
		value, err := strconv.ParseInt(strings.TrimSpace(wire.LegacyRoute), 10, 64)
		if err == nil {
			c.GroupRouteID = value
		}
	}
	return nil
}

type Subscription struct {
	ID               int64              `json:"id,string"`
	TenantID         int64              `json:"tenantId,string"`
	TaskID           int64              `json:"taskId,string"`
	SubscriberUserID int64              `json:"subscriberUserId,string"`
	Events           []FactType         `json:"events"`
	Channels         []ChannelSelection `json:"channels"`
	Enabled          bool               `json:"enabled"`
	Version          int64              `json:"version,string"`
	CreatedBy        int64              `json:"createdBy,string"`
	UpdatedBy        int64              `json:"updatedBy,string"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}

type SubscriptionCommand struct {
	Enabled  bool               `json:"enabled"`
	Events   []FactType         `json:"events"`
	Channels []ChannelSelection `json:"channels"`
}

func DefaultSubscriptionCommand() SubscriptionCommand {
	return SubscriptionCommand{
		Enabled:  true,
		Events:   []FactType{FactActionRequired, FactInterrupted, FactCompleted, FactDeliveryCompleted},
		Channels: []ChannelSelection{{Type: ChannelInApp}},
	}
}

func NewSubscription(tenantID, taskID, subscriberUserID int64, command SubscriptionCommand) (Subscription, error) {
	defaults := DefaultSubscriptionCommand()
	if len(command.Events) == 0 {
		command.Events = defaults.Events
	}
	if len(command.Channels) == 0 {
		command.Channels = defaults.Channels
	}
	item := Subscription{
		TenantID: tenantID, TaskID: taskID, SubscriberUserID: subscriberUserID,
		Events: CanonicalEvents(command.Events), Channels: CanonicalChannels(command.Channels), Enabled: command.Enabled, Version: 1,
	}
	if err := item.Validate(); err != nil {
		return Subscription{}, err
	}
	return item, nil
}

func (s Subscription) Validate() error {
	if s.TenantID <= 0 || s.TaskID <= 0 || s.SubscriberUserID <= 0 || s.Version <= 0 {
		return ErrRecipientInvalid
	}
	if len(s.Events) == 0 || len(s.Events) > 4 || len(s.Channels) == 0 || len(s.Channels) > 7 {
		return ErrFactInvalid
	}
	seenEvents := make(map[FactType]struct{}, len(s.Events))
	for _, event := range s.Events {
		if !event.Valid() {
			return ErrFactInvalid
		}
		if _, duplicate := seenEvents[event]; duplicate {
			return ErrFactInvalid
		}
		seenEvents[event] = struct{}{}
	}
	seenChannels := make(map[string]struct{}, len(s.Channels))
	groupCount := 0
	for _, channel := range s.Channels {
		if !channel.Type.Valid() {
			return ErrFactInvalid
		}
		if channel.Type == ChannelGroupRoute {
			groupCount++
		}
		if (channel.Type == ChannelGroupRoute) != (channel.GroupRouteID > 0) {
			return ErrFactInvalid
		}
		key := string(channel.Type) + "\x00" + strconv.FormatInt(channel.GroupRouteID, 10)
		if _, duplicate := seenChannels[key]; duplicate {
			return ErrFactInvalid
		}
		seenChannels[key] = struct{}{}
	}
	if groupCount > 3 {
		return ErrFactInvalid
	}
	return nil
}

func (f FactType) Valid() bool {
	return f == FactActionRequired || f == FactInterrupted || f == FactCompleted || f == FactDeliveryCompleted
}

func (s SourceType) Valid() bool {
	switch s {
	case SourceContinuationCheckpoint, SourceFinalizationRun, SourceResultEvaluation, SourceDeliveryRequest, SourceReviewOperation, SourceIndependentReviewRun, SourceRuntimeRecovery, SourceTaskTimeout, SourceArtifactUpload, SourceTaskAttempt, SourceTaskCancel:
		return true
	default:
		return false
	}
}

func (f Focus) Valid() bool {
	return f == FocusEvaluation || f == FocusResult || f == FocusDelivery || f == FocusContinuation
}

func (c ChannelType) Valid() bool {
	return c == ChannelInApp || c == ChannelEmailUser || c == ChannelFeishuDirect || c == ChannelWeComDirect || c == ChannelGroupRoute
}

type TaskNotificationFact struct {
	TenantID            int64
	TaskID              int64
	TaskTurnID          int64
	FactType            FactType
	SourceType          SourceType
	SourceID            int64
	SourceOccurrence    int64
	FactIdentityVersion int
	AttentionUserID     int64
	MentionAttention    bool
	SceneCode           string
	ReasonCode          string
	Focus               Focus
	Summary             string
	OccurredAt          time.Time
}

// Recoverability describes whether the authoritative transition still has an
// automatic or human recovery path. It is deliberately independent from a
// module's internal status names so notification policy remains centralized.
type Recoverability string

const (
	RecoverabilityAutoRecovering  Recoverability = "AUTO_RECOVERING"
	RecoverabilityUserRecoverable Recoverability = "USER_RECOVERABLE"
	RecoverabilityTerminal        Recoverability = "TERMINAL"
)

func (r Recoverability) Valid() bool {
	return r == RecoverabilityAutoRecovering || r == RecoverabilityUserRecoverable || r == RecoverabilityTerminal
}

type TaskNotificationTransition struct {
	TenantID             int64
	TaskID               int64
	TaskTurnID           int64
	SourceType           SourceType
	SourceID             int64
	SourceOccurrence     int64
	PreviousStatus       string
	CurrentStatus        string
	ReasonCode           string
	ActorPrincipalID     int64
	AttentionUserID      int64
	Recoverability       Recoverability
	RetryBudgetExhausted bool
	Summary              string
	OccurredAt           time.Time
}

func (t TaskNotificationTransition) Validate() error {
	if t.TenantID <= 0 || t.TaskID <= 0 || t.TaskTurnID <= 0 || t.SourceID <= 0 || t.SourceOccurrence <= 0 || !t.SourceType.Valid() || !t.Recoverability.Valid() || t.OccurredAt.IsZero() {
		return ErrFactInvalid
	}
	if t.ReasonCode == "" || !safeReasonCode.MatchString(t.ReasonCode) || len(strings.TrimSpace(t.Summary)) > 1024 || hasControlExceptWhitespace(strings.TrimSpace(t.Summary)) {
		return ErrFactInvalid
	}
	if t.ActorPrincipalID < 0 || t.AttentionUserID < 0 {
		return ErrFactInvalid
	}
	return nil
}

type TransitionDisposition string

const (
	DispositionNone              TransitionDisposition = "NO_NOTIFICATION"
	DispositionActionRequired    TransitionDisposition = "TASK_ACTION_REQUIRED"
	DispositionInterrupted       TransitionDisposition = "TASK_INTERRUPTED"
	DispositionCompleted         TransitionDisposition = "TASK_COMPLETED"
	DispositionDeliveryCompleted TransitionDisposition = "DELIVERY_COMPLETED"
)

type TransitionDecision struct {
	Disposition TransitionDisposition
	FactType    FactType
	Focus       Focus
	SceneCode   string
	MentionUser bool
	Silent      bool
}

// ClassifyTransition is the single policy for converting authoritative Task
// state into a user-visible notification fact. Callers report what happened;
// they never select text, scenes, channels, links, or mentions.
func ClassifyTransition(t TaskNotificationTransition) (TransitionDecision, error) {
	if err := t.Validate(); err != nil {
		return TransitionDecision{}, err
	}
	if t.Recoverability == RecoverabilityAutoRecovering {
		return TransitionDecision{Disposition: DispositionNone}, nil
	}
	reason := strings.TrimSpace(t.ReasonCode)
	switch reason {
	case "AGENT_INPUT_REQUIRED", "AGENT_OPERATION_APPROVAL_REQUIRED":
		if t.SourceType != SourceTaskAttempt || t.Recoverability != RecoverabilityUserRecoverable {
			return TransitionDecision{}, ErrFactInvalid
		}
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusContinuation, SceneCode: "workforce_task_action_required", MentionUser: true}, nil
	case "PROCESSOR_WAIT_USER":
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusEvaluation, SceneCode: "workforce_task_action_required", MentionUser: true}, nil
	case "PROCESSOR_MANUAL_REVIEW":
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusContinuation, SceneCode: "workforce_task_action_required"}, nil
	case "FINALIZATION_SOURCE_LOST":
		if t.Recoverability == RecoverabilityTerminal {
			return TransitionDecision{Disposition: DispositionInterrupted, FactType: FactInterrupted, Focus: FocusResult, SceneCode: "workforce_task_interrupted"}, nil
		}
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusResult, SceneCode: "workforce_task_action_required"}, nil
	case "FINALIZATION_RETRY_EXHAUSTED", "ARTIFACT_UPLOAD_RETRY_EXHAUSTED",
		"INDEPENDENT_REVIEW_RETRY_EXHAUSTED", "INDEPENDENT_REVIEW_REJECTED", "INDEPENDENT_REVIEW_REPORT_INVALID",
		"INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED", "INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS",
		"INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE", "INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT":
		if t.Recoverability == RecoverabilityTerminal {
			return TransitionDecision{Disposition: DispositionInterrupted, FactType: FactInterrupted, Focus: FocusResult, SceneCode: "workforce_task_interrupted"}, nil
		}
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusResult, SceneCode: "workforce_task_action_required"}, nil
	case "DELIVERY_WAITING_APPROVAL":
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusDelivery, SceneCode: "workforce_delivery_approval_required", MentionUser: t.AttentionUserID > 0}, nil
	case "DELIVERY_MANUAL_REVIEW", "DELIVERY_OUTCOME_UNKNOWN", "DELIVERY_FAILED", "REVIEW_ACCESS_NOT_VERIFIED", "REVIEW_AUTHORIZATION_STALE", "REVIEW_RETRY_EXHAUSTED":
		return TransitionDecision{Disposition: DispositionActionRequired, FactType: FactActionRequired, Focus: FocusDelivery, SceneCode: "workforce_delivery_manual_review", MentionUser: t.AttentionUserID > 0}, nil
	case "TASK_TURN_SUCCEEDED", "INDEPENDENT_REVIEW_HUMAN_ACCEPTED":
		return TransitionDecision{Disposition: DispositionCompleted, FactType: FactCompleted, Focus: FocusResult, SceneCode: "workforce_task_completed"}, nil
	case "DELIVERY_SUCCEEDED":
		if t.SourceType != SourceDeliveryRequest {
			return TransitionDecision{}, ErrFactInvalid
		}
		return TransitionDecision{Disposition: DispositionDeliveryCompleted, FactType: FactDeliveryCompleted, Focus: FocusDelivery, SceneCode: "workforce_delivery_completed"}, nil
	case "TASK_CANCELLED_BY_CREATOR":
		return TransitionDecision{Disposition: DispositionNone, Silent: true}, nil
	case "PROCESSOR_REJECT", "AGENT_RETRY_EXHAUSTED", "RUNTIME_UNRECOVERABLE", "TASK_FAILED", "QUEUE_TIMEOUT", "SUPERVISOR_STARTUP_TIMEOUT", "TASK_TIMED_OUT", "TASK_CANCELLED_BY_OPERATOR", "TASK_CANCELLED_BY_SYSTEM":
		return TransitionDecision{Disposition: DispositionInterrupted, FactType: FactInterrupted, Focus: interruptedFocus(t.SourceType), SceneCode: "workforce_task_interrupted", MentionUser: true}, nil
	default:
		return TransitionDecision{}, ErrFactInvalid
	}
}

func interruptedFocus(source SourceType) Focus {
	switch source {
	case SourceResultEvaluation:
		return FocusEvaluation
	case SourceDeliveryRequest, SourceReviewOperation:
		return FocusDelivery
	case SourceTaskAttempt, SourceRuntimeRecovery, SourceTaskCancel:
		return FocusContinuation
	default:
		return FocusResult
	}
}

func (f TaskNotificationFact) Validate() error {
	if f.TenantID <= 0 || f.TaskID <= 0 || f.TaskTurnID <= 0 || f.SourceID <= 0 || !f.FactType.Valid() || !f.SourceType.Valid() || !f.Focus.Valid() {
		return ErrFactInvalid
	}
	if f.ReasonCode != "" && !safeReasonCode.MatchString(f.ReasonCode) {
		return ErrFactInvalid
	}
	if f.OccurredAt.IsZero() || !validFactCombination(f.FactType, f.SourceType, f.Focus) {
		return ErrFactInvalid
	}
	summary := strings.TrimSpace(f.Summary)
	if len(summary) > 1024 || hasControlExceptWhitespace(summary) {
		return ErrFactInvalid
	}
	return nil
}

func validFactCombination(fact FactType, source SourceType, focus Focus) bool {
	switch fact {
	case FactActionRequired:
		return (source == SourceTaskAttempt && focus == FocusContinuation) ||
			(source == SourceContinuationCheckpoint && (focus == FocusEvaluation || focus == FocusContinuation)) ||
			(source == SourceFinalizationRun && focus == FocusResult) ||
			(source == SourceArtifactUpload && focus == FocusResult) ||
			(source == SourceIndependentReviewRun && focus == FocusResult) ||
			(source == SourceDeliveryRequest && focus == FocusDelivery) ||
			(source == SourceReviewOperation && focus == FocusDelivery) ||
			(source == SourceRuntimeRecovery && focus == FocusContinuation)
	case FactInterrupted:
		return (source == SourceResultEvaluation && focus == FocusEvaluation) ||
			(source == SourceTaskAttempt && focus == FocusContinuation) ||
			(source == SourceTaskCancel && focus == FocusContinuation) ||
			(source == SourceRuntimeRecovery && focus == FocusContinuation) ||
			(source == SourceTaskTimeout && focus == FocusResult) ||
			(source == SourceDeliveryRequest && focus == FocusDelivery) ||
			(source == SourceReviewOperation && focus == FocusDelivery)
	case FactCompleted:
		return (source == SourceFinalizationRun || source == SourceIndependentReviewRun) && focus == FocusResult
	case FactDeliveryCompleted:
		return source == SourceDeliveryRequest && focus == FocusDelivery
	default:
		return false
	}
}

type Record struct {
	ID                          int64             `json:"id,string"`
	TenantID                    int64             `json:"tenantId,string"`
	TaskID                      int64             `json:"taskId,string"`
	TaskTurnID                  int64             `json:"taskTurnId,string"`
	SourceType                  SourceType        `json:"sourceType"`
	SourceID                    int64             `json:"sourceId,string"`
	SourceOccurrence            int64             `json:"sourceOccurrence,string"`
	FactIdentityVersion         int               `json:"factIdentityVersion"`
	FactType                    FactType          `json:"factType"`
	ReasonCode                  string            `json:"reasonCode,omitempty"`
	Focus                       Focus             `json:"focus"`
	SubscriptionID              int64             `json:"subscriptionId,string"`
	SubscriberUserID            int64             `json:"subscriberUserId,string"`
	SubscriptionVersion         int64             `json:"subscriptionVersion,string"`
	TaskNameSnapshot            string            `json:"taskNameSnapshot"`
	TurnNumber                  int               `json:"turnNumber"`
	SummarySnapshot             string            `json:"summarySnapshot,omitempty"`
	TitleSnapshot               string            `json:"titleSnapshot"`
	ContentSnapshot             string            `json:"contentSnapshot"`
	PublicationContractVersion  int               `json:"publicationContractVersion"`
	SceneCode                   string            `json:"sceneCode,omitempty"`
	TemplateVariablesJSON       string            `json:"-"`
	DeepLinkSnapshot            string            `json:"deepLink"`
	IdempotencyIdentity         string            `json:"idempotencyIdentity"`
	RequestDigest               string            `json:"requestDigest"`
	Status                      string            `json:"status"`
	PublishAttempts             int               `json:"publishAttempts"`
	NextRetryAt                 *time.Time        `json:"nextRetryAt,omitempty"`
	LeaseToken                  string            `json:"-"`
	LeaseUntil                  *time.Time        `json:"-"`
	NotificationReceiptIdentity string            `json:"notificationReceiptIdentity,omitempty"`
	WarningCode                 string            `json:"warningCode,omitempty"`
	LastErrorCode               string            `json:"lastErrorCode,omitempty"`
	OccurredAt                  time.Time         `json:"occurredAt"`
	CreatedAt                   time.Time         `json:"createdAt"`
	PublishedAt                 *time.Time        `json:"publishedAt,omitempty"`
	UpdatedAt                   time.Time         `json:"updatedAt"`
	Channels                    []RecordChannel   `json:"channels,omitempty"`
	Recipients                  []RecordRecipient `json:"recipients,omitempty"`
}

type RecipientRole string
type PersonalChannelPolicy string

const (
	RecipientSubscriber              RecipientRole         = "SUBSCRIBER"
	RecipientAttention               RecipientRole         = "ATTENTION"
	PersonalChannelsTaskSubscription PersonalChannelPolicy = "TASK_SUBSCRIPTION"
	PersonalChannelsUserPreference   PersonalChannelPolicy = "USER_PREFERENCE"
)

type RecordRecipient struct {
	UserID                int64                 `json:"userId,string"`
	Role                  RecipientRole         `json:"role"`
	PersonalChannelPolicy PersonalChannelPolicy `json:"personalChannelPolicy"`
	InAppRequired         bool                  `json:"inAppRequired"`
}

type RecordChannel struct {
	Type          ChannelType `json:"kind"`
	GroupRouteID  int64       `json:"routeId,string,omitempty"`
	MentionUserID int64       `json:"mentionUserId,string,omitempty"`
}

func BuildIdempotencyIdentity(r Record) string {
	identity := "workforce-task:" + strconv.FormatInt(r.TaskID, 10) +
		":turn:" + strconv.FormatInt(r.TaskTurnID, 10) +
		":" + string(r.FactType) + ":" + string(r.SourceType) + ":" + strconv.FormatInt(r.SourceID, 10)
	if r.FactIdentityVersion >= 2 {
		identity += ":occ:" + strconv.FormatInt(r.SourceOccurrence, 10)
	}
	return identity + ":user:" + strconv.FormatInt(r.SubscriberUserID, 10)
}

func FreezePresentation(record *Record) error {
	if record == nil || record.TaskID <= 0 || record.TaskTurnID <= 0 || record.SubscriberUserID <= 0 || record.TurnNumber <= 0 || strings.TrimSpace(record.TaskNameSnapshot) == "" {
		return ErrFactInvalid
	}
	deepLink, err := BuildTaskDeepLinkForSource(record.TaskID, record.TaskTurnID, record.Focus, record.SourceType, record.SourceID)
	if err != nil {
		return err
	}
	record.DeepLinkSnapshot = deepLink
	record.IdempotencyIdentity = BuildIdempotencyIdentity(*record)
	requestedScene := strings.TrimSpace(record.SceneCode)
	switch record.FactType {
	case FactActionRequired:
		record.SceneCode = "workforce_task_action_required"
		record.TitleSnapshot = record.TaskNameSnapshot + " 需要你处理"
		switch requestedScene {
		case "workforce_delivery_approval_required":
			record.ContentSnapshot = fmt.Sprintf("第 %d 轮的交付结果正在等待批准。", record.TurnNumber)
		case "workforce_delivery_manual_review":
			record.ContentSnapshot = fmt.Sprintf("第 %d 轮的交付结果需要人工确认。", record.TurnNumber)
		default:
			record.ContentSnapshot = fmt.Sprintf("第 %d 轮需要补充信息或人工处理，请打开任务继续。", record.TurnNumber)
		}
	case FactCompleted:
		record.SceneCode = "workforce_task_completed"
		record.TitleSnapshot = record.TaskNameSnapshot + " 已完成"
		record.ContentSnapshot = fmt.Sprintf("第 %d 轮结果已完成并安全保存。", record.TurnNumber)
	case FactDeliveryCompleted:
		record.SceneCode = "workforce_delivery_completed"
		record.TitleSnapshot = record.TaskNameSnapshot + " 已交付"
		record.ContentSnapshot = fmt.Sprintf("第 %d 轮结果已完成远端核验。", record.TurnNumber)
	case FactInterrupted:
		record.SceneCode = "workforce_task_interrupted"
		record.TitleSnapshot = record.TaskNameSnapshot + " 执行中断"
		record.ContentSnapshot = fmt.Sprintf("第 %d 轮未能继续执行，请打开任务查看。", record.TurnNumber)
	default:
		return ErrFactInvalid
	}
	if requestedScene != "" {
		record.SceneCode = requestedScene
	}
	record.PublicationContractVersion = 2
	variables := map[string]string{
		"taskName": record.TaskNameSnapshot, "turnNumber": strconv.Itoa(record.TurnNumber),
		"statusLabel": record.TitleSnapshot, "summary": record.SummarySnapshot,
		"reasonLabel": safeReasonLabel(record.ReasonCode), "occurredAt": record.OccurredAt.UTC().Format(time.RFC3339),
	}
	encoded, err := json.Marshal(variables)
	if err != nil {
		return ErrFactInvalid
	}
	record.TemplateVariablesJSON = string(encoded)
	return nil
}

func safeReasonLabel(reasonCode string) string {
	switch strings.TrimSpace(reasonCode) {
	case "PROCESSOR_WAIT_USER":
		return "需要补充信息"
	case "PROCESSOR_MANUAL_REVIEW":
		return "需要人工确认"
	case "TASK_TURN_SUCCEEDED":
		return "当前轮已完成"
	case "PROCESSOR_REJECT":
		return "结果验收未通过"
	case "AGENT_RETRY_EXHAUSTED", "RUNTIME_UNRECOVERABLE", "TASK_FAILED":
		return "执行无法自动恢复"
	case "QUEUE_TIMEOUT", "SUPERVISOR_STARTUP_TIMEOUT", "TASK_TIMED_OUT":
		return "任务执行超时"
	case "TASK_CANCELLED_BY_OPERATOR", "TASK_CANCELLED_BY_SYSTEM":
		return "任务已由其他处理人或系统取消"
	case "FINALIZATION_SOURCE_LOST":
		return "收尾所需的执行环境已不可用"
	case "FINALIZATION_RETRY_EXHAUSTED":
		return "结果收尾需要人工恢复"
	case "ARTIFACT_UPLOAD_RETRY_EXHAUSTED":
		return "结果文件上传需要人工恢复"
	case "DELIVERY_WAITING_APPROVAL":
		return "交付结果等待批准"
	case "DELIVERY_MANUAL_REVIEW", "DELIVERY_FAILED", "REVIEW_ACCESS_NOT_VERIFIED", "REVIEW_AUTHORIZATION_STALE", "REVIEW_RETRY_EXHAUSTED":
		return "交付需要人工处理"
	case "DELIVERY_OUTCOME_UNKNOWN":
		return "交付结果需要确认"
	default:
		return "请打开任务查看"
	}
}

func (r Record) DeepLink() (string, error) {
	expected, err := BuildTaskDeepLinkForSource(r.TaskID, r.TaskTurnID, r.Focus, r.SourceType, r.SourceID)
	if err != nil {
		return "", err
	}
	if r.DeepLinkSnapshot == "" {
		return expected, nil
	}
	if r.DeepLinkSnapshot != expected {
		return "", ErrDeepLinkInvalid
	}
	return expected, nil
}

func RecordRequestDigest(r Record) string {
	input := struct {
		TenantID                   string            `json:"tenantId"`
		TaskID                     string            `json:"taskId"`
		TaskTurnID                 string            `json:"taskTurnId"`
		SourceType                 SourceType        `json:"sourceType"`
		SourceID                   string            `json:"sourceId"`
		SourceOccurrence           string            `json:"sourceOccurrence"`
		FactIdentityVersion        int               `json:"factIdentityVersion"`
		FactType                   FactType          `json:"factType"`
		ReasonCode                 string            `json:"reasonCode"`
		Focus                      Focus             `json:"focus"`
		SubscriptionID             string            `json:"subscriptionId"`
		SubscriberUserID           string            `json:"subscriberUserId"`
		SubscriptionVersion        string            `json:"subscriptionVersion"`
		TaskName                   string            `json:"taskName"`
		TurnNumber                 int               `json:"turnNumber"`
		Summary                    string            `json:"summary"`
		Title                      string            `json:"title"`
		Content                    string            `json:"content"`
		PublicationContractVersion int               `json:"publicationContractVersion"`
		SceneCode                  string            `json:"sceneCode"`
		TemplateVariablesJSON      string            `json:"templateVariablesJson"`
		DeepLink                   string            `json:"deepLink"`
		IdempotencyIdentity        string            `json:"idempotencyIdentity"`
		OccurredAt                 string            `json:"occurredAt"`
		Channels                   []RecordChannel   `json:"channels"`
		Recipients                 []RecordRecipient `json:"recipients"`
	}{
		TenantID: strconv.FormatInt(r.TenantID, 10), TaskID: strconv.FormatInt(r.TaskID, 10), TaskTurnID: strconv.FormatInt(r.TaskTurnID, 10),
		SourceType: r.SourceType, SourceID: strconv.FormatInt(r.SourceID, 10), SourceOccurrence: strconv.FormatInt(r.SourceOccurrence, 10), FactIdentityVersion: r.FactIdentityVersion,
		FactType: r.FactType, ReasonCode: r.ReasonCode, Focus: r.Focus,
		SubscriptionID: strconv.FormatInt(r.SubscriptionID, 10), SubscriberUserID: strconv.FormatInt(r.SubscriberUserID, 10),
		SubscriptionVersion: strconv.FormatInt(r.SubscriptionVersion, 10), TaskName: r.TaskNameSnapshot, TurnNumber: r.TurnNumber, Summary: r.SummarySnapshot,
		Title: r.TitleSnapshot, Content: r.ContentSnapshot, DeepLink: r.DeepLinkSnapshot, IdempotencyIdentity: r.IdempotencyIdentity,
		PublicationContractVersion: r.PublicationContractVersion, SceneCode: r.SceneCode, TemplateVariablesJSON: r.TemplateVariablesJSON,
		OccurredAt: r.OccurredAt.UTC().Format(time.RFC3339Nano), Channels: slices.Clone(r.Channels), Recipients: slices.Clone(r.Recipients),
	}
	slices.SortFunc(input.Channels, func(a, b RecordChannel) int {
		return strings.Compare(string(a.Type)+"\x00"+strconv.FormatInt(a.GroupRouteID, 10), string(b.Type)+"\x00"+strconv.FormatInt(b.GroupRouteID, 10))
	})
	slices.SortFunc(input.Recipients, func(a, b RecordRecipient) int {
		return strings.Compare(strconv.FormatInt(a.UserID, 10)+"\x00"+string(a.Role), strconv.FormatInt(b.UserID, 10)+"\x00"+string(b.Role))
	})
	body, _ := json.Marshal(input)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

type OutboxMessage struct {
	SchemaVersion int   `json:"schemaVersion"`
	RecordID      int64 `json:"recordId,string"`
	TaskID        int64 `json:"taskId,string"`
	TaskTurnID    int64 `json:"taskTurnId,string"`
}

const (
	OutboxAggregate = "task_notification"
	OutboxEventType = "task.notification.requested"
)

func CanonicalEvents(events []FactType) []FactType {
	result := slices.Clone(events)
	slices.Sort(result)
	return result
}

func CanonicalChannels(channels []ChannelSelection) []ChannelSelection {
	result := slices.Clone(channels)
	slices.SortFunc(result, func(left, right ChannelSelection) int {
		return strings.Compare(string(left.Type)+"\x00"+strconv.FormatInt(left.GroupRouteID, 10), string(right.Type)+"\x00"+strconv.FormatInt(right.GroupRouteID, 10))
	})
	return result
}

func hasControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0
}

func hasControlExceptWhitespace(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return (r < 0x20 && r != '\n' && r != '\r' && r != '\t') || r == 0x7f
	}) >= 0
}

func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
