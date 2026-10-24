package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	// SceneRevisionStateDraft is the only mutable scene revision state.
	SceneRevisionStateDraft = "DRAFT"
	// SceneRevisionStatePublished is the one runtime-selectable state.
	SceneRevisionStatePublished = "PUBLISHED"
	// SceneRevisionStateSuperseded keeps old accepted routing readable.
	SceneRevisionStateSuperseded = "SUPERSEDED"

	// SceneReceiverKindInApp chooses the local inbox only and has no connection.
	SceneReceiverKindInApp = "IN_APP"
	// SceneReceiverKindFeishuOpenID chooses a dynamic Feishu member target.
	SceneReceiverKindFeishuOpenID = "FEISHU_OPEN_ID"
	// SceneReceiverKindFeishuChatID chooses a dynamic Feishu group target.
	SceneReceiverKindFeishuChatID = "FEISHU_CHAT_ID"
	// SceneReceiverKindWeComUserID chooses a dynamic WeCom member target.
	SceneReceiverKindWeComUserID = "WECOM_USERID"
	// SceneReceiverKindFixedConnection chooses a saved HTTP/profile connection.
	SceneReceiverKindFixedConnection    = "FIXED_CONNECTION"
	SceneReceiverKindEmailUser          = "EMAIL_USER"
	SceneReceiverKindFeishuWebhookGroup = "FEISHU_WEBHOOK_GROUP"
	SceneReceiverKindWeComWebhookGroup  = "WECOM_WEBHOOK_GROUP"
	SceneReceiverKindHTTPConnectorFixed = "HTTP_CONNECTOR_FIXED"

	// SceneSnapshotResolutionAccepted records a published enabled scene frozen
	// with a semantic notification.
	SceneSnapshotResolutionAccepted = "ACCEPTED"
	// SceneSnapshotResolutionDisabled records a published-but-disabled scene
	// that deliberately created no external delivery.
	SceneSnapshotResolutionDisabled = "SCENE_DISABLED"
)

var (
	// ErrSceneDefinitionNotFound intentionally does not distinguish an absent
	// scene from one in another scope.
	ErrSceneDefinitionNotFound = errors.New("notification scene definition was not found in the current scope")
	// ErrSceneRevisionNotFound is used for a missing or foreign revision.
	ErrSceneRevisionNotFound = errors.New("notification scene revision was not found in the current scope")
	// ErrSceneRevisionConflict is the optimistic-concurrency response.
	ErrSceneRevisionConflict = errors.New("notification scene revision has changed; refresh and try again")
	// ErrSceneRevisionImmutable rejects writes to a published history row.
	ErrSceneRevisionImmutable = errors.New("published notification scene revisions are immutable")

	sceneDefinitionCodePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,95}$`)
)

// SceneDefinition is the stable identity of one scope, business scene and
// receiver kind. It deliberately contains no member, group, URL, secret or
// provider parameter: dynamic targets remain in the semantic Client call.
type SceneDefinition struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// ScopeID owns the configuration and is always checked before reads/writes.
	ScopeID string `db:"scopeId"`
	// SceneCode is the application-owned business scene identifier.
	SceneCode string `db:"sceneCode"`
	// SceneName is the management-only display name.
	SceneName string `db:"sceneName"`
	// ReceiverKind selects one local or third-party receiver category.
	ReceiverKind string `db:"receiverKind"`
	// CurrentDraftRevisionID points to the only mutable revision, if any.
	CurrentDraftRevisionID *int64 `db:"currentDraftRevisionId"`
	// CurrentPublishedRevisionID points to the runtime selection, if any.
	CurrentPublishedRevisionID *int64 `db:"currentPublishedRevisionId"`
	// Version fences pointer transitions.
	Version int `db:"version"`
	// CreatorID is the configuration actor.
	CreatorID *int64 `db:"creatorId"`
	// UpdaterID is the latest configuration actor.
	UpdaterID *int64 `db:"updaterId"`
	// CreateTime is the durable creation timestamp.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest definition update timestamp.
	UpdateTime time.Time `db:"updateTime"`
	// IsDeleted retains normal logical-delete compatibility.
	IsDeleted int `db:"isDeleted"`
}

// SceneRevision freezes exactly one template revision and one allowed sending
// connection. In-app scenes carry an empty ConnectionRef by design.
type SceneRevision struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// SceneDefinitionID owns this revision.
	SceneDefinitionID int64 `db:"sceneDefinitionId"`
	// RevisionNo increases within one scene definition.
	RevisionNo int `db:"revisionNo"`
	// State is DRAFT, PUBLISHED or SUPERSEDED.
	State string `db:"state"`
	// RevisionVersion fences editable draft updates and publication.
	RevisionVersion int `db:"revisionVersion"`
	// Enabled controls whether the published scene may accept new delivery.
	Enabled bool `db:"enabled"`
	// TemplateRevisionID is a published G6.1 template revision.
	TemplateRevisionID int64 `db:"templateRevisionId"`
	// ConnectionRef is a saved connection reference, empty for IN_APP.
	ConnectionRef string `db:"connectionRef"`
	// ConnectionDigest is a secret-free identity digest retained for audit.
	ConnectionDigest string `db:"connectionDigest"`
	// PublishedAt records the immutable transition time.
	PublishedAt *time.Time `db:"publishedAt"`
	// PublishedBy records the actor who published this revision.
	PublishedBy *int64 `db:"publishedBy"`
	// CreatorID is the configuration actor.
	CreatorID *int64 `db:"creatorId"`
	// UpdaterID is the latest configuration actor.
	UpdaterID *int64 `db:"updaterId"`
	// CreateTime is the durable creation timestamp.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is the latest revision update timestamp.
	UpdateTime time.Time `db:"updateTime"`
}

// SceneRevisionAudit stores metadata only. It intentionally has no message
// content, variable value, target, URL, credential or provider response.
type SceneRevisionAudit struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// SceneDefinitionID identifies the scene lifecycle.
	SceneDefinitionID int64 `db:"sceneDefinitionId"`
	// ScopeID scopes operational audit lookup.
	ScopeID string `db:"scopeId"`
	// Action is the lifecycle operation code.
	Action string `db:"action"`
	// FromRevisionNo optionally identifies the source revision.
	FromRevisionNo *int `db:"fromRevisionNo"`
	// ToRevisionNo optionally identifies the target revision.
	ToRevisionNo *int `db:"toRevisionNo"`
	// ErrorCode is a safe stable validation/error classification.
	ErrorCode string `db:"errorCode"`
	// ActorID identifies the administrator or service actor.
	ActorID *int64 `db:"actorId"`
	// CreateTime is the append-only audit timestamp.
	CreateTime time.Time `db:"createTime"`
}

// SceneSnapshot is attached to the accepted logical notification. It freezes
// the selection identity and digests, while provider capability remains in
// the existing encrypted external-target or HTTP snapshot tables.
type SceneSnapshot struct {
	// ID is the internal primary key.
	ID int64 `db:"id"`
	// NotificationID references the accepted logical notification.
	NotificationID int64 `db:"notificationId"`
	// ScopeID owns the accepted notification.
	ScopeID string `db:"scopeId"`
	// SceneCode is retained for human/audit correlation.
	SceneCode string `db:"sceneCode"`
	// ReceiverKind distinguishes one external or in-app selection.
	ReceiverKind string `db:"receiverKind"`
	// SceneDefinitionID identifies the source definition.
	SceneDefinitionID int64 `db:"sceneDefinitionId"`
	// SceneRevisionID identifies the immutable selected revision.
	SceneRevisionID int64 `db:"sceneRevisionId"`
	// TemplateDefinitionID identifies the selected template definition.
	TemplateDefinitionID int64 `db:"templateDefinitionId"`
	// TemplateRevisionID identifies the immutable selected template revision.
	TemplateRevisionID int64 `db:"templateRevisionId"`
	// ConnectionRef is empty for in-app scenes and never includes a URL/secret.
	ConnectionRef string `db:"connectionRef"`
	// ConnectionDigest is a secret-free configuration identity digest.
	ConnectionDigest string `db:"connectionDigest"`
	// TemplateContentDigest identifies the published content without storing it.
	TemplateContentDigest string `db:"templateContentDigest"`
	// RenderedDigest is a one-way digest of the accepted rendered output.
	RenderedDigest string `db:"renderedDigest"`
	// VariableDigest is a one-way digest of the caller's validated variables.
	VariableDigest string `db:"variableDigest"`
	// Resolution is ACCEPTED or SCENE_DISABLED.
	Resolution string `db:"resolution"`
	// CreateTime is the acceptance timestamp.
	CreateTime time.Time `db:"createTime"`
	// UpdateTime is retained for normal repository mapping.
	UpdateTime time.Time `db:"updateTime"`
}

// SceneDefinitionQuery is a scope-bound management listing query.
type SceneDefinitionQuery struct {
	ScopeID  string
	Keyword  string
	Current  int
	PageSize int
}

// ValidateSceneDefinitionCode validates the portable scene-code namespace.
func ValidateSceneDefinitionCode(code string) error {
	if !sceneDefinitionCodePattern.MatchString(strings.TrimSpace(code)) {
		return fmt.Errorf("场景编码只能包含字母、数字、下划线和连字符，且必须以字母开头")
	}
	return nil
}

// NormalizeSceneReceiverKind makes the public receiver categories stable.
func NormalizeSceneReceiverKind(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case SceneReceiverKindInApp, SceneReceiverKindEmailUser, SceneReceiverKindFeishuOpenID, SceneReceiverKindFeishuChatID, SceneReceiverKindWeComUserID, SceneReceiverKindFeishuWebhookGroup, SceneReceiverKindWeComWebhookGroup, SceneReceiverKindHTTPConnectorFixed, SceneReceiverKindFixedConnection:
		return value, nil
	default:
		return "", fmt.Errorf("场景接收对象类别不支持")
	}
}

// SceneReceiverKindForExternalIdentity maps the Client's dynamic target type
// to the one configuration identity it may use.
func SceneReceiverKindForExternalIdentity(identityKind string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(identityKind)) {
	case ExternalIdentityFeishuOpenID:
		return SceneReceiverKindFeishuOpenID, nil
	case ExternalIdentityFeishuChatID:
		return SceneReceiverKindFeishuChatID, nil
	case ExternalIdentityWeComUserID:
		return SceneReceiverKindWeComUserID, nil
	default:
		return "", fmt.Errorf("第三方目标身份类型不支持场景配置")
	}
}

// ValidateSceneConnection enforces the single-sending-way model at the
// domain boundary. It deliberately validates only capability categories; the
// application layer owns scope, enabled state and secret completeness checks.
func ValidateSceneConnection(receiverKind string, channel *Channel, connectionRef string) error {
	kind, err := NormalizeSceneReceiverKind(receiverKind)
	if err != nil {
		return err
	}
	connectionRef = strings.TrimSpace(connectionRef)
	if kind == SceneReceiverKindInApp {
		if connectionRef != "" || channel != nil {
			return fmt.Errorf("站内信场景不允许配置发送连接")
		}
		return nil
	}
	if channel == nil || connectionRef == "" || strings.TrimSpace(channel.ChannelCode) != connectionRef {
		return fmt.Errorf("场景发送方式不能为空")
	}
	switch kind {
	case SceneReceiverKindEmailUser:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeEmail {
			return fmt.Errorf("邮件场景必须选择邮件连接")
		}
	case SceneReceiverKindFeishuOpenID, SceneReceiverKindFeishuChatID:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeFeishuApp {
			return fmt.Errorf("飞书场景必须选择飞书应用连接")
		}
	case SceneReceiverKindWeComUserID:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeWeComApp {
			return fmt.Errorf("企业微信场景必须选择企业微信应用连接")
		}
	case SceneReceiverKindFixedConnection:
		if !IsStaticHTTPChannelType(channel.ChannelType) {
			return fmt.Errorf("固定连接场景必须选择受控 HTTP 连接")
		}
	case SceneReceiverKindFeishuWebhookGroup:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeFeishuWebhook {
			return fmt.Errorf("飞书群场景必须选择飞书群机器人连接")
		}
	case SceneReceiverKindWeComWebhookGroup:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeWeComWebhook {
			return fmt.Errorf("企业微信群场景必须选择企业微信群机器人连接")
		}
	case SceneReceiverKindHTTPConnectorFixed:
		if strings.ToUpper(strings.TrimSpace(channel.ChannelType)) != ChannelTypeHTTPConnector {
			return fmt.Errorf("HTTP Connector 场景必须选择固定 Connector 连接")
		}
	}
	return nil
}

// SceneConnectionDigest is a secret-free stable identity used in scene audit
// and acceptance snapshots. It intentionally excludes config and credentials.
func SceneConnectionDigest(channel *Channel) string {
	if channel == nil {
		return ""
	}
	if channel.EnterpriseConnectionID == nil {
		raw := strings.TrimSpace(channel.ScopeID) + "\x00" + strings.TrimSpace(channel.ChannelCode) + "\x00" + strings.ToUpper(strings.TrimSpace(channel.ChannelType))
		sum := sha256.Sum256([]byte(raw))
		return hex.EncodeToString(sum[:])
	}
	organizationID, enterpriseConnectionID := int64(0), int64(0)
	if channel.OrganizationID != nil {
		organizationID = *channel.OrganizationID
	}
	if channel.EnterpriseConnectionID != nil {
		enterpriseConnectionID = *channel.EnterpriseConnectionID
	}
	raw := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%d\x00%d\x00%s", strings.TrimSpace(channel.ScopeID), strings.TrimSpace(channel.ChannelCode), strings.ToUpper(strings.TrimSpace(channel.ChannelType)), strings.ToUpper(strings.TrimSpace(channel.ScopeType)), organizationID, enterpriseConnectionID, channel.EnterpriseConnectionVersion, strings.TrimSpace(channel.EnterpriseConnectionDigest))
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
