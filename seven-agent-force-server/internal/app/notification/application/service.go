package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/mail"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

type SecretValueService interface {
	EncryptString(ctx context.Context, plain string) (secretvalueinfra.SecretValue, error)
	DecryptString(ctx context.Context, value secretvalueinfra.SecretValue) (string, error)
}

type ChannelDriver = domain.ChannelDriver
type ResultChannelDriver = domain.ResultChannelDriver
type ProviderError = domain.ProviderError
type DriverResult = domain.DriverResult

const (
	DriverResultProviderAccepted = domain.DriverResultProviderAccepted
	DriverResultFailed           = domain.DriverResultFailed
	DriverResultUnknown          = domain.DriverResultUnknown
)

type DriverRegistry = domain.DriverRegistry

// ChannelURLValidator is an infrastructure port used to enforce the shared
// outbound URL SSRF policy before a URL-capable channel is persisted.
type ChannelURLValidator interface {
	ValidateChannel(ctx context.Context, channel domain.Channel) error
}

// WebhookProfileURLValidator validates a secret-only fixed group endpoint at
// configuration time. It is deliberately optional on the broad URL-validator
// port so narrow legacy test doubles do not accidentally gain an arbitrary
// endpoint capability; production wiring must provide it for G5.2 profiles.
type WebhookProfileURLValidator interface {
	ValidateWebhookProfileEndpoint(ctx context.Context, channelType, endpoint string) error
}

type MessagePublisher interface {
	Enabled() bool
	PublishDispatch(ctx context.Context, message domain.DeliveryMessage) error
}

type channelBatchRepository interface {
	ListChannelsByCodes(ctx context.Context, channelCodes []string) ([]domain.Channel, error)
}

type deliveryBatchRepository interface {
	InsertDeliveries(ctx context.Context, items []domain.Delivery) error
}

type httpDeliverySnapshotBatchRepository interface {
	InsertHTTPDeliverySnapshots(ctx context.Context, items []domain.HTTPDeliverySnapshot) error
}

type groupRouteSnapshotRepository interface {
	InsertGroupRouteSnapshots(ctx context.Context, items []domain.GroupRouteSnapshot) error
	FindGroupRouteSnapshotByDeliveryID(ctx context.Context, deliveryID string) (*domain.GroupRouteSnapshot, error)
	UpdateGroupRouteSnapshotWarning(ctx context.Context, deliveryID, warningCode string) error
}

type outboxBatchRepository interface {
	AppendOutboxBatch(ctx context.Context, events []domain.OutboxEvent) error
}

func (s *Service) loadChannelsByCodes(ctx context.Context, channelCodes []string) (map[string]domain.Channel, error) {
	unique := make(map[string]struct{}, len(channelCodes))
	for _, code := range channelCodes {
		if code = strings.TrimSpace(code); code != "" {
			unique[code] = struct{}{}
		}
	}
	codes := make([]string, 0, len(unique))
	for code := range unique {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		return map[string]domain.Channel{}, nil
	}
	if len(codes) == 1 {
		item, err := s.repo.FindChannelByCode(ctx, codes[0])
		if err != nil {
			return nil, err
		}
		result := make(map[string]domain.Channel, 1)
		if item != nil {
			result[item.ChannelCode] = *item
		}
		return result, nil
	}
	batch, ok := s.repo.(channelBatchRepository)
	if !ok {
		return nil, fmt.Errorf("notification channel batch repository is not configured")
	}
	items, err := batch.ListChannelsByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.Channel, len(items))
	for _, item := range items {
		result[item.ChannelCode] = item
	}
	return result, nil
}

// scopedOutboxRepository is implemented by the SQL repository after the
// scope-aware Outbox migration. Keeping it optional lets focused unit doubles
// remain small while production relay selection is filtered in SQL before
// LIMIT.
type scopedOutboxRepository interface {
	ListReadyOutboxForScope(ctx context.Context, scopeID string, limit int) ([]domain.OutboxEvent, error)
	FindReadyOutboxForScope(ctx context.Context, scopeID, eventID, eventType string) (*domain.OutboxEvent, error)
	ListUnknownOutboxForScope(ctx context.Context, scopeID string, limit int) ([]domain.OutboxEvent, error)
}

// ErrNotificationScopeMismatch is returned before a delivery mutates durable
// state when the event/message belongs to a different installation, Hub, or
// Node scope. It is an infrastructure safety signal, not a provider result.
var ErrNotificationScopeMismatch = errors.New("notification delivery is outside the current scope")

type permanentDispatchConsumeError struct {
	err error
}

func (e permanentDispatchConsumeError) Error() string { return e.err.Error() }

func (e permanentDispatchConsumeError) Unwrap() error { return e.err }

func (e permanentDispatchConsumeError) Permanent() bool { return true }

// InboxRealtime is the optional post-commit hint port. Its failure may delay
// browser freshness, but it must never turn a committed inbox change into a
// failed write or expose notification content outside the explicit read APIs.
type InboxRealtime interface {
	PublishInboxChanged(ctx context.Context, intent domain.InboxChangedIntent) error
	SubscribeInboxChanges(userID int64) (<-chan domain.InboxChangedIntent, func())
}

// AudienceResolver is the cross-module read port used by bounded role
// materialization. It deliberately exposes a cursor and limit rather than an
// unbounded role-member list.
type AudienceResolver interface {
	ListActiveUserIDsByRoleIDPage(ctx context.Context, roleID, afterUserID int64, limit int) ([]int64, error)
}

// VerifiedEmailResolver exposes only an address whose ownership is proven.
// Implementations must never return a historical or administrator-entered
// unverified address as ready.
type VerifiedEmailResolver interface {
	ResolveVerifiedNotificationEmail(ctx context.Context, userID int64) (email string, ready bool, err error)
}

// ExternalTargetDigester produces and verifies keyed, non-plaintext target
// digests. The module binds it to the deployment master-key provider.
type ExternalTargetDigester interface {
	Digest(ctx context.Context, keyRef, scopeID, connectionRef, identityKind, subject string) (digest, resolvedKeyRef string, err error)
}

type EnterpriseIdentityReadinessResolver interface {
	ResolveEnterpriseReadiness(ctx context.Context, organizationID, userID int64, provider string) (available, directReady, mentionReady bool, reasonCode string, err error)
	ResolveEnterpriseNotificationAuthority(ctx context.Context, organizationID, userID int64, provider, grantType string) (identityKind, subject, authorityDigest, connectionConfigDigest string, identityID, identityVersion, connectionID, connectionVersion int64, err error)
	ValidateEnterpriseNotificationAuthority(ctx context.Context, organizationID, userID, identityID, identityVersion, connectionID, connectionVersion int64, authorityDigest, grantType string) error
	AcquireEnterpriseApplicationCredential(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionConfigDigest string) (provider, appID, corpID, agentID, clientSecret string, credentialVersion int64, expiresAt time.Time, err error)
}

type EnterpriseGroupDiscoveryResolver interface {
	DiscoverEnterpriseGroups(context.Context, int64, int64) ([]EnterpriseGroupTarget, error)
}

type EnterpriseGroupTarget struct {
	DisplayName            string
	Description            string
	Target                 string
	ConnectionID           int64
	ConnectionVersion      int64
	ConnectionConfigDigest string
}

type PermissionResolver interface {
	HasPermission(context.Context, int64, string) (bool, error)
}

type Service struct {
	tx                   store.Transactor
	repo                 domain.Repository
	domain               *domain.Service
	secrets              SecretValueService
	drivers              DriverRegistry
	urls                 ChannelURLValidator
	broker               MessagePublisher
	idGen                *xid.Generator
	log                  *zap.Logger
	now                  func() time.Time
	scopeID              string
	audiences            AudienceResolver
	verifiedEmails       VerifiedEmailResolver
	realtime             InboxRealtime
	digester             ExternalTargetDigester
	publicWebBaseURL     domain.PublicWebBaseURL
	enterpriseIdentities EnterpriseIdentityReadinessResolver
	permissions          PermissionResolver
	relayMu              sync.Mutex
}

func (s *Service) BindPermissionResolver(resolver PermissionResolver) {
	if s != nil {
		s.permissions = resolver
	}
}

func (s *Service) BindEnterpriseIdentityResolver(resolver EnterpriseIdentityReadinessResolver) {
	if s != nil {
		s.enterpriseIdentities = resolver
	}
}

// BindPublicWebBaseURL installs the validated platform URL used only when an
// external delivery is accepted. Empty is a supported configuration and keeps
// in-app relative actions fully functional.
func (s *Service) BindPublicWebBaseURL(value domain.PublicWebBaseURL) {
	if s != nil {
		s.publicWebBaseURL = value
	}
}

type DriverMessage = domain.DriverMessage

// SetScopeID binds the service to one local installation, Hub, or Node scope.
// It is set by module wiring and never derived from a request's organization.
func (s *Service) SetScopeID(scopeID string) {
	if s != nil {
		s.scopeID = strings.TrimSpace(scopeID)
	}
}

// BindAudienceResolver supplies the paged user-role projection used only for
// deferred audience materialization.
func (s *Service) BindAudienceResolver(resolver AudienceResolver) {
	if s != nil {
		s.audiences = resolver
	}
}

func (s *Service) BindVerifiedEmailResolver(resolver VerifiedEmailResolver) {
	if s != nil {
		s.verifiedEmails = resolver
	}
}

// BindInboxRealtime installs the optional Redis-backed browser hint bridge.
// Passing nil keeps durable REST reads fully usable without realtime freshness.
func (s *Service) BindInboxRealtime(realtime InboxRealtime) {
	if s != nil {
		s.realtime = realtime
	}
}

// BindExternalTargetDigester installs the keyed target-digest service used by
// external enterprise-member snapshots. External publication fails closed when
// it is not configured.
func (s *Service) BindExternalTargetDigester(digester ExternalTargetDigester) {
	if s != nil {
		s.digester = digester
	}
}

func NewService(tx store.Transactor, repo domain.Repository, domainService *domain.Service, secrets SecretValueService, drivers DriverRegistry, urls ChannelURLValidator, broker MessagePublisher, idGen *xid.Generator) *Service {
	return &Service{
		tx:      tx,
		repo:    repo,
		domain:  domainService,
		secrets: secrets,
		drivers: drivers,
		urls:    urls,
		broker:  broker,
		idGen:   idGen,
		log:     zap.NewNop(),
		now:     time.Now,
	}
}

func (s *Service) SetLogger(log *zap.Logger) {
	if s != nil && log != nil {
		s.log = log.Named("notification")
	}
}

func (s *Service) EnqueueChallengeOTP(ctx context.Context, request facade.ChallengeOTPRequest) error {
	if strings.TrimSpace(request.ToEmail) == "" {
		return nil
	}
	if _, err := mail.ParseAddress(request.ToEmail); err != nil {
		return apperrors.Params("邮箱地址格式不正确")
	}
	ttl := request.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if strings.TrimSpace(request.Code) == "" {
		return apperrors.Params("验证码不能为空")
	}
	sceneName := strings.TrimSpace(request.SceneName)
	if sceneName == "" {
		sceneName = sceneDisplayName(request.Scene)
	}
	vars := map[string]any{
		"AppName":    "SevenFramework",
		"Code":       request.Code,
		"Scene":      request.Scene,
		"SceneName":  sceneName,
		"TTLMinutes": int(ttl.Minutes()),
		"ToEmail":    request.ToEmail,
	}
	for key, value := range request.Metadata {
		vars[key] = value
	}
	return s.enqueueChallengeOTP(ctx, request.ToEmail, ttl, vars)
}

// enqueueChallengeOTP materializes an encrypted, short-lived delivery. It
// intentionally does not call Enqueue: ordinary enqueue persists variables
// and rendered content, which would expose the code to regular delivery
// management, retry, cache and logging paths.
func (s *Service) enqueueChallengeOTP(ctx context.Context, target string, ttl time.Duration, variables map[string]any) error {
	if s == nil || s.repo == nil {
		return fmt.Errorf("notification service is not configured")
	}
	binding, err := s.repo.FindActiveSceneBinding(ctx, s.scopeID, domain.SceneChallengeOTP)
	if err != nil {
		return err
	}
	if binding == nil || !s.sceneBindingBelongsToCurrentScope(binding) {
		return apperrors.Operation("验证码通知场景未配置可用渠道")
	}
	channel, err := s.repo.FindChannelByCode(ctx, binding.ChannelCode)
	if err != nil {
		return err
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled || !s.channelBelongsToCurrentScope(channel) {
		return apperrors.Operation("验证码通知渠道不可用")
	}
	if domain.IsEnterpriseApplicationChannelType(channel.ChannelType) || domain.IsStaticHTTPChannelType(channel.ChannelType) {
		return apperrors.Operation("验证码只能发送到受控成员通知渠道")
	}
	tpl, err := s.repo.FindTemplateByCode(ctx, binding.TemplateCode)
	if err != nil {
		return err
	}
	if tpl == nil || tpl.Status != domain.ChannelStatusEnabled || !s.templateBelongsToCurrentScope(tpl) {
		return apperrors.Operation("验证码通知模板不可用")
	}
	if !strings.EqualFold(strings.TrimSpace(tpl.SceneCode), domain.SceneChallengeOTP) {
		return apperrors.Operation("验证码通知模板场景不匹配")
	}
	rendered, err := renderTemplate(*tpl, variables)
	if err != nil {
		return err
	}
	deliveryID := "otp_" + s.nextStringID()
	expiresAt := s.now().Add(ttl).UTC()
	delivery := &domain.Delivery{
		ID:         s.nextID(),
		DeliveryID: deliveryID,
		// The digest is intentionally based only on a random delivery identifier.
		// A low-entropy code must not be present even as a deterministic digest
		// input that could invite offline guessing.
		RequestDigest: digest("challenge-otp", deliveryID),
		SceneCode:     domain.SceneChallengeOTP,
		ChannelCode:   channel.ChannelCode,
		ChannelType:   channel.ChannelType,
		TemplateCode:  tpl.TemplateCode,
		Target:        strings.TrimSpace(target),
		TargetMasked:  maskTarget(target),
		Presentation:  domain.DeliveryPresentationText,
		ContentTier:   domain.DeliveryContentTierSecretEphemeral,
		Status:        domain.DeliveryStatusPending,
		MaxRetry:      defaultInt(binding.MaxRetry, 0),
	}
	message := domain.DeliveryMessage{MessageID: "notification:" + deliveryID, DeliveryID: deliveryID, ScopeID: s.scopeID}
	outbox := &domain.OutboxEvent{
		ID:            s.nextID(),
		EventID:       message.MessageID,
		ScopeID:       s.scopeID,
		EventType:     domain.OutboxEventNotificationDispatch,
		AggregateType: domain.OutboxAggregateNotification,
		AggregateID:   deliveryID,
		Payload:       mustJSON(message),
		Status:        "PENDING",
	}
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.InsertDelivery(txCtx, delivery); err != nil {
			return err
		}
		if _, err := s.persistEphemeralDeliveryContent(txCtx, deliveryID, expiresAt, ephemeralRenderedContent{
			Subject: rendered.Subject, Text: rendered.Text, HTML: rendered.HTML, Markdown: rendered.Markdown,
		}); err != nil {
			return err
		}
		return s.repo.AppendOutbox(txCtx, outbox)
	}); err != nil {
		// Do not include the destination, variables or rendered output in logs.
		s.logger().Error("notification_challenge_otp_enqueue_failed", zap.String("deliveryId", deliveryID), zap.Error(err))
		return err
	}
	return nil
}

func (s *Service) UpsertChannel(ctx context.Context, request facade.ChannelUpsertRequest, actorID int64) (*facade.ChannelRecord, error) {
	item := &domain.Channel{
		ID:            request.ID,
		ChannelCode:   strings.TrimSpace(request.ChannelCode),
		ChannelName:   strings.TrimSpace(request.ChannelName),
		ChannelType:   strings.ToUpper(strings.TrimSpace(request.ChannelType)),
		ScopeID:       s.scopeID,
		Status:        request.Status,
		Priority:      defaultInt(request.Priority, 100),
		ConfigJSON:    strings.TrimSpace(request.ConfigJSON),
		RateLimitJSON: strings.TrimSpace(request.RateLimitJSON),
		MetadataJSON:  strings.TrimSpace(request.MetadataJSON),
		UpdaterID:     int64Ptr(actorID),
	}
	existing, err := s.repo.FindChannelByCode(ctx, item.ChannelCode)
	if err != nil {
		return nil, err
	}
	if existing != nil && !s.channelBelongsToCurrentScope(existing) {
		return nil, ErrNotificationScopeMismatch
	}
	// Organization enterprise-application channels are provisioned from the
	// Enterprise Identity module. The public Notification channel editor must
	// not detach them from their connection or reintroduce a second copy of the
	// application secret.
	if existing != nil && existing.EnterpriseConnectionID != nil {
		return nil, apperrors.Forbidden("企业个人消息渠道请在企业连接中管理")
	}
	if item.ID <= 0 {
		item.ID = s.nextID()
		item.CreatorID = int64Ptr(actorID)
	}
	if domain.IsEnterpriseApplicationChannelType(item.ChannelType) {
		if strings.TrimSpace(s.scopeID) == "" {
			return nil, fmt.Errorf("notification scope is not configured")
		}
		if err := s.applyEnterpriseApplicationChannelRequest(item, existing, request); err != nil {
			return nil, err
		}
	}
	if item.ChannelType == domain.ChannelTypeHTTPConnector {
		if err := s.applyHTTPConnectorChannelRequest(item, existing, request); err != nil {
			return nil, err
		}
	}
	if domain.IsWebhookProfileChannelType(item.ChannelType) {
		if err := s.applyWebhookProfileChannelRequest(ctx, item, existing, &request); err != nil {
			return nil, err
		}
	}
	if err := s.domain.ValidateChannel(item); err != nil {
		return nil, err
	}
	if domain.IsURLChannelType(item.ChannelType) && s.urls == nil {
		return nil, fmt.Errorf("notification outbound URL guard is not configured")
	}
	if s.urls != nil {
		if err := s.urls.ValidateChannel(ctx, *item); err != nil {
			return nil, err
		}
	}
	clearSecret := item.ChannelType == domain.ChannelTypeHTTPConnector && httpConnectorUsesNoSecret(item.ConfigJSON)
	if clearSecret {
		item.SecretCiphertext = ""
		item.SecretEDEK = ""
		item.SecretWrapKeyRef = ""
	} else if strings.TrimSpace(request.SecretPlain) != "" {
		if s.secrets == nil {
			return nil, fmt.Errorf("notification secret service is not configured")
		}
		encrypted, err := s.secrets.EncryptString(ctx, request.SecretPlain)
		if err != nil {
			return nil, err
		}
		item.SecretCiphertext = encrypted.CiphertextB64
		item.SecretEDEK = encrypted.EDEKB64 // gitleaks:allow -- encrypted envelope-key material, never plaintext
		item.SecretWrapKeyRef = encrypted.WrapKeyRef
	} else if existing != nil {
		item.SecretCiphertext = existing.SecretCiphertext
		item.SecretEDEK = existing.SecretEDEK
		item.SecretWrapKeyRef = existing.SecretWrapKeyRef
	}
	if domain.IsEnterpriseApplicationChannelType(item.ChannelType) && item.Status == domain.ChannelStatusEnabled && (strings.TrimSpace(item.SecretCiphertext) == "" || strings.TrimSpace(item.SecretEDEK) == "" || strings.TrimSpace(item.SecretWrapKeyRef) == "") {
		return nil, apperrors.Params("启用企业应用渠道前必须配置应用密钥")
	}
	if domain.IsWebhookProfileChannelType(item.ChannelType) && item.Status == domain.ChannelStatusEnabled && (strings.TrimSpace(item.SecretCiphertext) == "" || strings.TrimSpace(item.SecretEDEK) == "" || strings.TrimSpace(item.SecretWrapKeyRef) == "") {
		return nil, apperrors.Params("启用群机器人渠道前必须配置 Webhook 地址")
	}
	if err := s.repo.UpsertChannel(ctx, item); err != nil {
		if errors.Is(err, domain.ErrScopedConfigurationNotFound) {
			return nil, ErrNotificationScopeMismatch
		}
		return nil, err
	}
	saved, err := s.repo.FindChannelByCode(ctx, item.ChannelCode)
	if err != nil {
		return nil, err
	}
	if saved == nil || !s.channelBelongsToCurrentScope(saved) {
		return nil, ErrNotificationScopeMismatch
	}
	return mapChannel(*saved), nil
}

// applyEnterpriseApplicationChannelRequest keeps the public configuration and
// the provider's small parameter catalogue structured at the management
// boundary. JSON is only the internal persistence format; raw configuration
// cannot create provider-native request fields.
func (s *Service) applyEnterpriseApplicationChannelRequest(item, existing *domain.Channel, request facade.ChannelUpsertRequest) error {
	if item == nil {
		return fmt.Errorf("enterprise application channel is nil")
	}
	config := domain.EnterpriseApplicationConfig{}
	if request.ProviderConfig != nil {
		config = domain.EnterpriseApplicationConfig{
			AppID:   request.ProviderConfig.FeishuAppID,
			CorpID:  request.ProviderConfig.WeComCorpID,
			AgentID: request.ProviderConfig.WeComAgentID,
		}
	} else if existing != nil {
		parsed, err := domain.ParseEnterpriseApplicationConfig(item.ChannelType, existing.ConfigJSON)
		if err != nil {
			return apperrors.Params("企业应用公开配置无效")
		}
		config = parsed
	} else {
		return apperrors.Params("企业应用公开配置不能为空")
	}
	encodedConfig, err := domain.EncodeEnterpriseApplicationConfig(item.ChannelType, config)
	if err != nil {
		return apperrors.Params("企业应用公开配置无效")
	}
	item.ConfigJSON = encodedConfig

	settings := make([]domain.ProviderParameterSetting, 0, len(request.ProviderParameterSettings))
	if request.ProviderParameterSettings == nil && existing != nil {
		stored, err := domain.ParseProviderParameterSettings(existing.MetadataJSON)
		if err != nil {
			return apperrors.Params("企业应用参数配置无效")
		}
		settings = stored
	} else {
		for _, setting := range request.ProviderParameterSettings {
			settings = append(settings, domain.ProviderParameterSetting{
				Key:          setting.Key,
				Enabled:      setting.Enabled,
				DefaultValue: setting.DefaultValue,
			})
		}
	}
	settings, err = domain.NormalizeProviderParameterSettings(item.ChannelType, settings)
	if err != nil {
		return apperrors.Params("企业应用参数配置无效")
	}
	metadataSource := ""
	if existing != nil {
		metadataSource = existing.MetadataJSON
	}
	metadataJSON, err := domain.MergeProviderParameterSettings(metadataSource, settings)
	if err != nil {
		return apperrors.Params("企业应用参数配置无效")
	}
	item.MetadataJSON = metadataJSON
	return nil
}

// applyHTTPConnectorChannelRequest turns the small typed management contract
// into its internal persisted shape. It intentionally rejects every raw JSON
// escape hatch: a connection cannot carry a caller-provided proxy, body,
// script, expression, credential value, or self-approved network policy.
func (s *Service) applyHTTPConnectorChannelRequest(item, existing *domain.Channel, request facade.ChannelUpsertRequest) error {
	if item == nil {
		return fmt.Errorf("HTTP connector channel is nil")
	}
	if strings.TrimSpace(request.ConfigJSON) != "" || strings.TrimSpace(request.MetadataJSON) != "" || strings.TrimSpace(request.RateLimitJSON) != "" {
		return apperrors.Params("HTTP 连接器只能使用结构化配置，不能提交原始配置或扩展 JSON")
	}

	config := domain.HTTPConnectorConfig{}
	if request.HTTPConnectorConfig != nil {
		if strings.EqualFold(strings.TrimSpace(request.HTTPConnectorConfig.AuthenticationMode), domain.HTTPConnectorAuthMTLS) {
			return apperrors.Params("mTLS 暂不支持")
		}
		config = httpConnectorDomainConfig(*request.HTTPConnectorConfig)
		if strings.EqualFold(strings.TrimSpace(config.Authentication.Mode), domain.HTTPConnectorAuthNone) && strings.TrimSpace(request.SecretPlain) != "" {
			return apperrors.Params("未启用认证的 HTTP 连接器不能配置密钥")
		}
	} else if existing != nil {
		parsed, err := domain.ParseHTTPConnectorConfig(existing.ConfigJSON)
		if err != nil {
			return apperrors.Params("HTTP 连接器配置无效")
		}
		config = parsed
	} else {
		return apperrors.Params("HTTP 连接器配置不能为空")
	}

	normalized, err := domain.NormalizeHTTPConnectorConfig(config)
	if err != nil {
		return apperrors.Params("HTTP 连接器配置无效")
	}
	if normalized.Authentication.Mode != domain.HTTPConnectorAuthNone && strings.TrimSpace(request.SecretPlain) == "" && (existing == nil || strings.TrimSpace(existing.SecretCiphertext) == "") {
		return apperrors.Params("启用认证的 HTTP 连接器必须配置连接密钥")
	}
	encoded, err := domain.EncodeHTTPConnectorConfig(normalized)
	if err != nil {
		return apperrors.Params("HTTP 连接器配置无效")
	}
	item.ConfigJSON = encoded
	item.MetadataJSON = ""
	item.RateLimitJSON = ""
	return nil
}

func httpConnectorDomainConfig(input facade.HTTPConnectorConfig) domain.HTTPConnectorConfig {
	mappings := make([]domain.HTTPConnectorFieldMapping, 0, len(input.FieldMappings))
	for _, mapping := range input.FieldMappings {
		mappings = append(mappings, domain.HTTPConnectorFieldMapping{Source: mapping.Source, Target: mapping.Target})
	}
	mode := strings.ToUpper(strings.TrimSpace(input.AuthenticationMode))
	if mode == "" {
		mode = domain.HTTPConnectorAuthNone
	}
	authentication := domain.HTTPConnectorAuthentication{Mode: mode}
	switch mode {
	case domain.HTTPConnectorAuthNone:
	case domain.HTTPConnectorAuthBearer, domain.HTTPConnectorAuthBasic, domain.HTTPConnectorAuthHMACSHA256:
		authentication.SecretRef = domain.HTTPConnectorSecretRefConnection
	}
	return domain.HTTPConnectorConfig{
		EndpointURL:         input.EndpointURL,
		EgressPolicyRef:     input.EgressPolicyRef,
		Method:              input.Method,
		Authentication:      authentication,
		FieldMappings:       mappings,
		HeaderAllowlist:     append([]string(nil), input.HeaderAllowlist...),
		IdempotencyHeader:   input.IdempotencyHeader,
		TimeoutMilliseconds: input.TimeoutMilliseconds,
		SuccessStatusCodes:  append([]int(nil), input.SuccessStatusCodes...),
	}
}

// applyWebhookProfileChannelRequest accepts the intentionally small fixed
// group-profile surface. URL/key material is a write-only secret envelope;
// it never shares the generic ConfigJSON escape hatch and never carries a
// business-supplied group or payload.
func (s *Service) applyWebhookProfileChannelRequest(ctx context.Context, item, existing *domain.Channel, request *facade.ChannelUpsertRequest) error {
	if item == nil || request == nil {
		return fmt.Errorf("webhook profile channel request is nil")
	}
	if strings.TrimSpace(request.ConfigJSON) != "" || strings.TrimSpace(request.MetadataJSON) != "" || strings.TrimSpace(request.RateLimitJSON) != "" || strings.TrimSpace(request.SecretPlain) != "" {
		return apperrors.Params("群机器人渠道只能使用受控字段，不能提交原始配置、扩展 JSON 或通用密钥")
	}

	config := domain.WebhookProfileConfig{}
	if request.WebhookProfileConfig != nil {
		config = webhookProfileDomainConfig(*request.WebhookProfileConfig)
	} else if existing != nil {
		parsed, err := domain.ParseWebhookProfileConfig(existing.ConfigJSON)
		if err != nil {
			return apperrors.Params("群机器人渠道配置无效")
		}
		config = parsed
	}
	normalized, err := domain.NormalizeWebhookProfileConfig(config)
	if err != nil {
		return apperrors.Params("群机器人渠道配置无效")
	}
	encodedConfig, err := domain.EncodeWebhookProfileConfig(normalized)
	if err != nil {
		return apperrors.Params("群机器人渠道配置无效")
	}
	item.ConfigJSON = encodedConfig
	item.MetadataJSON = ""
	item.RateLimitJSON = ""

	endpoint := strings.TrimSpace(request.WebhookURL)
	if endpoint == "" {
		if strings.TrimSpace(request.WebhookSigningSecret) != "" {
			return apperrors.Params("更新群机器人签名密钥时必须同时提供 Webhook 地址")
		}
		return nil
	}
	secret := domain.WebhookProfileSecret{EndpointURL: endpoint, SigningSecret: request.WebhookSigningSecret}
	normalizedSecret, err := domain.NormalizeWebhookProfileSecret(item.ChannelType, secret)
	if err != nil {
		return apperrors.Params("群机器人 Webhook 地址或签名密钥无效")
	}
	validator, ok := s.urls.(WebhookProfileURLValidator)
	if !ok || validator == nil {
		return fmt.Errorf("notification webhook profile URL validator is not configured")
	}
	if err := validator.ValidateWebhookProfileEndpoint(ctx, item.ChannelType, normalizedSecret.EndpointURL); err != nil {
		return err
	}
	encodedSecret, err := domain.EncodeWebhookProfileSecret(item.ChannelType, normalizedSecret)
	if err != nil {
		return apperrors.Params("群机器人 Webhook 地址或签名密钥无效")
	}
	request.SecretPlain = encodedSecret
	return nil
}

func webhookProfileDomainConfig(input facade.WebhookProfileConfig) domain.WebhookProfileConfig {
	return domain.WebhookProfileConfig{
		TimeoutMilliseconds: input.TimeoutMilliseconds,
		SuccessStatusCodes:  append([]int(nil), input.SuccessStatusCodes...),
	}
}

func httpConnectorUsesNoSecret(raw string) bool {
	config, err := domain.ParseHTTPConnectorConfig(raw)
	return err == nil && config.Authentication.Mode == domain.HTTPConnectorAuthNone
}

func (s *Service) ListChannels(ctx context.Context, query domain.ChannelQuery) (*facade.PageResult[facade.ChannelRecord], error) {
	query.ScopeID = strings.TrimSpace(s.scopeID)
	items, total, err := s.repo.ListChannels(ctx, query)
	if err != nil {
		return nil, err
	}
	records := make([]facade.ChannelRecord, 0, len(items))
	for _, item := range items {
		records = append(records, *mapChannel(item))
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	return &facade.PageResult[facade.ChannelRecord]{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

// channelBelongsToCurrentScope is the application-level authorization gate for
// channel configuration and dispatch. Empty legacy channel scope is accepted
// only by the local runtime until it is explicitly republished with a scope.
func (s *Service) channelBelongsToCurrentScope(channel *domain.Channel) bool {
	if channel == nil {
		return false
	}
	return s.configurationBelongsToCurrentScope(channel.ScopeID)
}

func (s *Service) templateBelongsToCurrentScope(template *domain.Template) bool {
	if template == nil {
		return false
	}
	return s.configurationBelongsToCurrentScope(template.ScopeID)
}

func (s *Service) sceneBindingBelongsToCurrentScope(binding *domain.SceneBinding) bool {
	if binding == nil {
		return false
	}
	return s.configurationBelongsToCurrentScope(binding.ScopeID)
}

func (s *Service) configurationBelongsToCurrentScope(configurationScope string) bool {
	configuredScope := strings.TrimSpace(s.scopeID)
	if configuredScope == "" {
		return true
	}
	configurationScope = strings.TrimSpace(configurationScope)
	if configurationScope == "" {
		return configuredScope == "local"
	}
	return configurationScope == configuredScope
}

func (s *Service) RelayOutbox(ctx context.Context, limit int) error {
	return s.relayOutbox(ctx, limit, true)
}

// RelaySelectedOutbox relays only exact durable events selected by the caller.
// Unlike RelayOutbox it never lists the module-wide ready queue, unknown
// events, or materialization tasks. The selection is re-read by event id and
// type before its fenced claim so unrelated notification work cannot be
// observed, claimed, or completed by a controlled operation.
func (s *Service) RelaySelectedOutbox(ctx context.Context, selections []domain.OutboxEventSelection) error {
	return s.relaySelectedOutbox(ctx, selections, true)
}

// relaySelectedOutbox relays only explicitly named Outbox events. The normal
// controlled-acceptance path forces local dispatch so it cannot touch a shared
// broker; integration-only callers may explicitly keep the broker path while
// retaining the same exact event identity boundary.
func (s *Service) relaySelectedOutbox(ctx context.Context, selections []domain.OutboxEventSelection, forceLocalDispatch bool) error {
	if s == nil || !s.relayMu.TryLock() {
		return nil
	}
	defer s.relayMu.Unlock()
	if len(selections) == 0 {
		return fmt.Errorf("at least one selected notification Outbox event is required")
	}

	seen := make(map[string]struct{}, len(selections))
	for _, selection := range selections {
		eventID := strings.TrimSpace(selection.EventID)
		eventType := strings.TrimSpace(selection.EventType)
		if eventID == "" || eventType == "" {
			return fmt.Errorf("selected notification Outbox event id and type are required")
		}
		if !knownNotificationOutboxEventType(eventType) {
			return fmt.Errorf("selected notification Outbox event type is not supported")
		}
		key := eventType + "\x00" + eventID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate selected notification Outbox event")
		}
		seen[key] = struct{}{}

		event, err := s.findReadyOutbox(ctx, eventID, eventType)
		if err != nil {
			return err
		}
		if event == nil {
			// Another normal relay may already own or have completed this exact
			// event. It is safe to leave it alone and let the caller observe its
			// own delivery's durable terminal state.
			continue
		}
		if event.EventID != eventID || event.EventType != eventType {
			return fmt.Errorf("selected notification Outbox event did not match its exact identity")
		}
		belongs, scopeErr := s.outboxEventBelongsToScope(ctx, *event)
		if scopeErr != nil {
			return scopeErr
		}
		if !belongs {
			return ErrNotificationScopeMismatch
		}
		if err := s.relayReadyOutboxEvent(ctx, *event, "notification-acceptance-relay", forceLocalDispatch); err != nil {
			return err
		}
	}
	return nil
}

func knownNotificationOutboxEventType(eventType string) bool {
	switch eventType {
	case domain.OutboxEventNotificationDispatch, domain.OutboxEventNotificationIntent, domain.OutboxEventNotificationInboxChanged:
		return true
	default:
		return false
	}
}

func (s *Service) findReadyOutbox(ctx context.Context, eventID, eventType string) (*domain.OutboxEvent, error) {
	if repo, ok := s.repo.(scopedOutboxRepository); ok && strings.TrimSpace(s.scopeID) != "" {
		return repo.FindReadyOutboxForScope(ctx, s.scopeID, eventID, eventType)
	}
	return s.repo.FindReadyOutbox(ctx, eventID, eventType)
}

func (s *Service) listReadyOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if repo, ok := s.repo.(scopedOutboxRepository); ok && strings.TrimSpace(s.scopeID) != "" {
		return repo.ListReadyOutboxForScope(ctx, s.scopeID, limit)
	}
	return s.repo.ListReadyOutbox(ctx, limit)
}

func (s *Service) listUnknownOutbox(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if repo, ok := s.repo.(scopedOutboxRepository); ok && strings.TrimSpace(s.scopeID) != "" {
		return repo.ListUnknownOutboxForScope(ctx, s.scopeID, limit)
	}
	return s.repo.ListUnknownOutbox(ctx, limit)
}

func (s *Service) outboxEventBelongsToScope(ctx context.Context, event domain.OutboxEvent) (bool, error) {
	configuredScope := strings.TrimSpace(s.scopeID)
	if configuredScope == "" {
		return true, nil
	}
	scopeID := strings.TrimSpace(event.ScopeID)
	if scopeID == "" {
		var err error
		switch event.EventType {
		case domain.OutboxEventNotificationDispatch:
			var message domain.DeliveryMessage
			if json.Unmarshal([]byte(event.Payload), &message) == nil && strings.TrimSpace(message.ScopeID) != "" {
				scopeID = strings.TrimSpace(message.ScopeID)
			} else if strings.TrimSpace(message.DeliveryID) != "" {
				delivery, findErr := s.repo.FindDeliveryByID(ctx, message.DeliveryID)
				if findErr != nil {
					return false, findErr
				}
				scopeID, err = s.deliveryScope(ctx, delivery)
			}
		case domain.OutboxEventNotificationIntent:
			var message domain.IntentMessage
			if json.Unmarshal([]byte(event.Payload), &message) == nil && strings.TrimSpace(message.ScopeID) != "" {
				scopeID = strings.TrimSpace(message.ScopeID)
			} else if message.NotificationID > 0 {
				item, findErr := s.repo.FindLogicalNotificationByID(ctx, message.NotificationID)
				if findErr != nil {
					return false, findErr
				}
				if item != nil {
					scopeID = strings.TrimSpace(item.ScopeID)
				}
			}
		case domain.OutboxEventNotificationInboxChanged:
			var intent domain.InboxChangedIntent
			if json.Unmarshal([]byte(event.Payload), &intent) == nil {
				scopeID = strings.TrimSpace(intent.ScopeID)
			}
		}
		if err != nil {
			return false, err
		}
	}
	if scopeID == "" {
		// The only safe compatibility rule for historically unscoped records is
		// local-only ownership. A Hub/Node never claims an ambiguous row.
		return configuredScope == "local", nil
	}
	return scopeID == configuredScope, nil
}

func (s *Service) deliveryScope(ctx context.Context, delivery *domain.Delivery) (string, error) {
	if delivery == nil {
		return "", nil
	}
	if delivery.ExternalTargetID != nil {
		target, err := s.repo.FindExternalTargetByID(ctx, *delivery.ExternalTargetID)
		if err != nil || target == nil {
			return "", err
		}
		return strings.TrimSpace(target.ScopeID), nil
	}
	if delivery.NotificationID != nil {
		item, err := s.repo.FindLogicalNotificationByID(ctx, *delivery.NotificationID)
		if err != nil || item == nil {
			return "", err
		}
		return strings.TrimSpace(item.ScopeID), nil
	}
	channel, err := s.repo.FindChannelByCode(ctx, delivery.ChannelCode)
	if err != nil || channel == nil {
		return "", err
	}
	if scopeID := strings.TrimSpace(channel.ScopeID); scopeID != "" {
		return scopeID, nil
	}
	return "local", nil
}

func (s *Service) relayOutbox(ctx context.Context, limit int, materialize bool) error {
	if s == nil || !s.relayMu.TryLock() {
		return nil
	}
	defer s.relayMu.Unlock()

	unknownEvents, err := s.listUnknownOutbox(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range unknownEvents {
		belongs, scopeErr := s.outboxEventBelongsToScope(ctx, event)
		if scopeErr != nil {
			return scopeErr
		}
		if !belongs {
			continue
		}
		lease, claimed, claimErr := s.repo.TryClaimOutbox(ctx, event.ID, event.EventType, "notification-outbox-relay")
		if claimErr != nil {
			return claimErr
		}
		if !claimed {
			continue
		}
		if _, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "DEAD", fmt.Sprintf("unsupported notification outbox event type %q", event.EventType), event.RetryCount+1, nil); markErr != nil {
			return markErr
		}
	}

	events, err := s.listReadyOutbox(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := s.relayReadyOutboxEvent(ctx, event, "notification-outbox-relay", false); err != nil {
			return err
		}
	}
	if materialize && s.audiences != nil {
		return s.MaterializePending(ctx, materializationTaskLimit)
	}
	return nil
}

func (s *Service) relayReadyOutboxEvent(ctx context.Context, event domain.OutboxEvent, worker string, forceLocalDispatch bool) error {
	belongs, scopeErr := s.outboxEventBelongsToScope(ctx, event)
	if scopeErr != nil {
		return scopeErr
	}
	if !belongs {
		// Another scope owns this durable work. Do not take a lease, mutate its
		// retry state, or publish it to the local RabbitMQ route.
		return nil
	}
	lease, claimed, err := s.repo.TryClaimOutbox(ctx, event.ID, event.EventType, worker)
	if err != nil || !claimed {
		return err
	}
	if event.EventType == domain.OutboxEventNotificationIntent {
		return s.relayIntent(ctx, event, lease.Token)
	}
	if event.EventType == domain.OutboxEventNotificationInboxChanged {
		return s.relayInboxChanged(ctx, event, lease.Token)
	}
	var message domain.DeliveryMessage
	if err := json.Unmarshal([]byte(event.Payload), &message); err != nil {
		if _, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "DEAD", err.Error(), event.RetryCount+1, nil); markErr != nil {
			return markErr
		}
		return nil
	}
	if forceLocalDispatch || s.broker == nil || !s.broker.Enabled() {
		return s.dispatchLocal(ctx, event, lease.Token, message)
	}
	if err := s.broker.PublishDispatch(ctx, message); err != nil {
		next := s.now().Add(backoff(event.RetryCount + 1))
		if _, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "FAILED", err.Error(), event.RetryCount+1, &next); markErr != nil {
			return markErr
		}
		return nil
	}
	_, err = s.repo.MarkOutbox(ctx, event.ID, event.EventType, lease.Token, "DONE", "", event.RetryCount, nil)
	return err
}

// relayInboxChanged validates and publishes the durable, content-free inbox
// intent after commit. Redis failure is deliberately best-effort: the durable
// inbox state remains readable through REST and this outbox item is completed
// instead of retrying a freshness hint forever.
func (s *Service) relayInboxChanged(ctx context.Context, event domain.OutboxEvent, leaseToken string) error {
	var intent domain.InboxChangedIntent
	if err := json.Unmarshal([]byte(event.Payload), &intent); err != nil || strings.TrimSpace(intent.ScopeID) == "" || intent.UserID <= 0 || intent.ChangeSequence <= 0 {
		if err == nil {
			err = fmt.Errorf("notification inbox changed intent is invalid")
		}
		_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DEAD", err.Error(), event.RetryCount+1, nil)
		return markErr
	}
	if s.realtime != nil {
		if publishErr := s.realtime.PublishInboxChanged(ctx, intent); publishErr != nil {
			s.logger().Warn("notification_inbox_realtime_publish_failed",
				zap.String("scopeId", intent.ScopeID),
				zap.Int64("userId", intent.UserID),
				zap.Int64("changeSequence", intent.ChangeSequence),
				zap.Error(publishErr),
			)
		}
	}
	_, err := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DONE", "", event.RetryCount, nil)
	return err
}

// relayIntent handles the internal, secret-free materialization wakeup. It is
// intentionally not published to RabbitMQ and never enters a channel driver.
func (s *Service) relayIntent(ctx context.Context, event domain.OutboxEvent, leaseToken string) error {
	var message domain.IntentMessage
	if err := json.Unmarshal([]byte(event.Payload), &message); err != nil || message.NotificationID <= 0 {
		if err == nil {
			err = fmt.Errorf("notification intent has an invalid notification id")
		}
		_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DEAD", err.Error(), event.RetryCount+1, nil)
		return markErr
	}
	item, err := s.repo.FindLogicalNotificationByID(ctx, message.NotificationID)
	if err != nil {
		next := s.now().Add(backoff(event.RetryCount + 1))
		_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "FAILED", err.Error(), event.RetryCount+1, &next)
		return markErr
	}
	if item == nil {
		_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DEAD", "logical notification does not exist", event.RetryCount+1, nil)
		return markErr
	}
	_, err = s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DONE", "", event.RetryCount, nil)
	return err
}

// dispatchLocal is a bounded DB-backed fallback worker: RelayOutbox processes
// at most its configured batch and serializes local execution per process.
// It deliberately does not create one goroutine per outbox event.
func (s *Service) dispatchLocal(ctx context.Context, event domain.OutboxEvent, leaseToken string, message domain.DeliveryMessage) error {
	if strings.TrimSpace(message.MessageID) == "" {
		message.MessageID = "notification:" + message.DeliveryID
	}
	s.logger().Warn("notification_outbox_mq_disabled_bounded_local_dispatch",
		zap.Int64("outboxId", event.ID),
		zap.String("eventId", event.EventID),
		zap.String("messageId", message.MessageID),
		zap.String("deliveryId", message.DeliveryID),
		zap.String("reason", "RabbitMQ 未启用，通知使用有界的数据库 Outbox relay 投递"),
	)
	err := s.dispatch(ctx, message.DeliveryID)
	if err == nil || isDeliveryAsyncHandled(err) {
		_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "DONE", "", event.RetryCount, nil)
		return markErr
	}
	next := s.now().Add(backoff(event.RetryCount + 1))
	_, markErr := s.repo.MarkOutbox(ctx, event.ID, event.EventType, leaseToken, "FAILED", err.Error(), event.RetryCount+1, &next)
	return markErr
}

func (s *Service) HandleDispatchMessage(ctx context.Context, message domain.DeliveryMessage) error {
	if strings.TrimSpace(message.MessageID) == "" {
		message.MessageID = "notification:" + message.DeliveryID
	}
	if configuredScope := strings.TrimSpace(s.scopeID); configuredScope != "" {
		if strings.TrimSpace(message.ScopeID) == "" && configuredScope == "local" {
			// Compatibility for a previously published legacy-local message.
			message.ScopeID = configuredScope
		}
		if strings.TrimSpace(message.ScopeID) != configuredScope {
			return permanentDispatchConsumeError{err: ErrNotificationScopeMismatch}
		}
	}
	lease, claimed, err := s.repo.BeginConsume(ctx, message.MessageID, "notification-dispatch", "notification-dispatch-consumer", message.DeliveryID)
	if err != nil || !claimed {
		return err
	}
	if err := s.dispatch(ctx, message.DeliveryID); err != nil {
		if isDeliveryAsyncHandled(err) {
			_, err = s.repo.MarkConsumed(ctx, message.MessageID, "notification-dispatch", lease.Token, message.DeliveryID)
			return err
		}
		_, _ = s.repo.MarkConsumeFailed(ctx, message.MessageID, "notification-dispatch", lease.Token, err.Error())
		if errors.Is(err, ErrNotificationScopeMismatch) {
			return permanentDispatchConsumeError{err: err}
		}
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) && appErr.Kind() == apperrors.KindNotFound {
			// A broker message cannot become deliverable later when its durable
			// delivery row does not exist. Requeueing it would create an
			// unbounded poison-message loop.
			return permanentDispatchConsumeError{err: err}
		}
		return err
	}
	_, err = s.repo.MarkConsumed(ctx, message.MessageID, "notification-dispatch", lease.Token, message.DeliveryID)
	return err
}

func (s *Service) dispatch(ctx context.Context, deliveryID string) error {
	delivery, err := s.repo.FindDeliveryByID(ctx, deliveryID)
	if err != nil {
		return err
	}
	if delivery == nil {
		return apperrors.NotFound("通知投递不存在")
	}
	if configuredScope := strings.TrimSpace(s.scopeID); configuredScope != "" {
		deliveryScope, scopeErr := s.deliveryScope(ctx, delivery)
		if scopeErr != nil {
			return scopeErr
		}
		if deliveryScope == "" || deliveryScope != configuredScope {
			return ErrNotificationScopeMismatch
		}
	}
	if delivery.Status == domain.DeliveryStatusSent || delivery.Status == domain.DeliveryStatusProviderAccepted || delivery.Status == domain.DeliveryStatusUnknown || delivery.Status == domain.DeliveryStatusCanceled || delivery.Status == domain.DeliveryStatusFailed {
		return nil
	}
	channel, err := s.repo.FindChannelByCode(ctx, delivery.ChannelCode)
	if err != nil {
		return err
	}
	if channel != nil && !s.channelBelongsToCurrentScope(channel) {
		return ErrNotificationScopeMismatch
	}
	claimed, err := s.repo.MarkDeliverySending(ctx, deliveryID)
	if err != nil || !claimed {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(delivery.SceneCode), domain.SceneChallengeOTP) {
		if err := s.hydrateChallengeOTPForDispatch(ctx, delivery); err != nil {
			return err
		}
	}
	if delivery.ExternalTargetID != nil {
		return s.dispatchExternal(ctx, delivery, channel)
	}
	if domain.IsStaticHTTPChannelType(delivery.ChannelType) {
		return s.dispatchStaticHTTP(ctx, delivery, channel)
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled {
		return s.failOrRetry(ctx, delivery, "通知渠道不可用")
	}
	if err := s.deliver(ctx, delivery, *channel); err != nil {
		return s.failOrRetry(ctx, delivery, err.Error())
	}
	return s.repo.MarkDeliverySent(ctx, deliveryID, s.now())
}

// hydrateChallengeOTPForDispatch loads the one-time code only after the
// worker has claimed the delivery and only into this in-memory delivery
// object. No plaintext is put back into the repository, outbox or logs.
func (s *Service) hydrateChallengeOTPForDispatch(ctx context.Context, delivery *domain.Delivery) error {
	if delivery == nil || !strings.EqualFold(strings.TrimSpace(delivery.SceneCode), domain.SceneChallengeOTP) {
		return nil
	}
	repo, err := s.deliveryDiagnosticsRepository()
	if err != nil {
		return s.failOrRetry(ctx, delivery, "SECRET_EPHEMERAL_UNAVAILABLE")
	}
	item, err := repo.FindDeliveryEphemeralContent(ctx, strings.TrimSpace(s.scopeID), delivery.DeliveryID)
	if err != nil {
		return s.failOrRetry(ctx, delivery, "SECRET_EPHEMERAL_UNAVAILABLE")
	}
	if item == nil {
		return s.finishChallengeOTPWithoutDispatch(ctx, delivery, "SECRET_EPHEMERAL_MISSING")
	}
	if !item.ExpiresAt.After(s.now()) {
		return s.finishChallengeOTPWithoutDispatch(ctx, delivery, domain.DeliveryDiagnosticResultExpired)
	}
	content, err := s.decryptEphemeralDeliveryContent(ctx, *item)
	if err != nil {
		return s.failOrRetry(ctx, delivery, "SECRET_EPHEMERAL_UNAVAILABLE")
	}
	delivery.PayloadJSON = ""
	delivery.RenderedSubject = content.Subject
	delivery.RenderedText = content.Text
	delivery.RenderedHTML = content.HTML
	delivery.RenderedMarkdown = content.Markdown
	return nil
}

// finishChallengeOTPWithoutDispatch is terminal by design: sending an
// expired or missing one-time secret after its intended validity window would
// be worse than a visible delivery failure. Returning the handled sentinel
// lets the consumer acknowledge the already-resolved outbox event.
func (s *Service) finishChallengeOTPWithoutDispatch(ctx context.Context, delivery *domain.Delivery, resultCode string) error {
	if delivery == nil {
		return fmt.Errorf("notification delivery is nil")
	}
	retryCount := delivery.MaxRetry + 1
	if retryCount < delivery.RetryCount+1 {
		retryCount = delivery.RetryCount + 1
	}
	if err := s.repo.MarkDeliveryFailed(ctx, delivery.DeliveryID, retryCount, strings.TrimSpace(resultCode)); err != nil {
		return err
	}
	return deliveryAsyncHandledError{err: fmt.Errorf("%s", strings.TrimSpace(resultCode))}
}

func (s *Service) deliver(ctx context.Context, delivery *domain.Delivery, channel domain.Channel) error {
	if s == nil || s.drivers == nil {
		return fmt.Errorf("notification driver registry is not configured")
	}
	driver := s.drivers.Driver(channel.ChannelType)
	if driver == nil {
		return fmt.Errorf("notification channel driver %s is not available", channel.ChannelType)
	}
	secretPlain, err := s.decryptSecret(ctx, channel)
	if err != nil {
		return err
	}
	var variables map[string]any
	_ = json.Unmarshal([]byte(delivery.PayloadJSON), &variables)
	return driver.Send(ctx, DriverMessage{
		Channel:     channel,
		SecretPlain: secretPlain,
		Target:      delivery.Target,
		Subject:     delivery.RenderedSubject,
		Text:        delivery.RenderedText,
		HTML:        delivery.RenderedHTML,
		Markdown:    delivery.RenderedMarkdown,
		Variables:   variables,
		DeliveryID:  delivery.DeliveryID,
		EventKey:    delivery.SceneCode,
		Priority:    strconv.Itoa(channel.Priority),
		TraceID:     delivery.TraceID,
	})
}

func (s *Service) failOrRetry(ctx context.Context, delivery *domain.Delivery, lastError string) error {
	return s.failOrRetryWithAttempt(ctx, delivery, lastError, nil)
}

// failOrRetryWithAttempt changes a delivery's retry state and records the
// corresponding external provider attempt in the same transaction. The nil
// attempt path preserves legacy V1 behavior.
func (s *Service) failOrRetryWithAttempt(ctx context.Context, delivery *domain.Delivery, lastError string, attempt *domain.DeliveryAttempt) error {
	if delivery == nil {
		return fmt.Errorf("notification delivery is nil")
	}
	if attempt != nil {
		lastError = stableExternalCode(lastError, "DELIVERY_FAILED")
	}
	retryCount := delivery.RetryCount + 1
	if retryCount > delivery.MaxRetry {
		if err := s.withinTx(ctx, func(txCtx context.Context) error {
			if attempt != nil {
				if err := s.repo.InsertDeliveryAttempt(txCtx, attempt); err != nil {
					return err
				}
			}
			return s.repo.MarkDeliveryFailed(txCtx, delivery.DeliveryID, retryCount, lastError)
		}); err != nil {
			return err
		}
		return deliveryAsyncHandledError{err: fmt.Errorf("%s", lastError)}
	}
	next := s.now().Add(backoff(retryCount))
	message := domain.DeliveryMessage{
		MessageID:  fmt.Sprintf("notification:%s:retry:%d", delivery.DeliveryID, retryCount),
		DeliveryID: delivery.DeliveryID,
		ScopeID:    s.scopeID,
	}
	outbox := &domain.OutboxEvent{
		ID:            s.nextID(),
		EventID:       message.MessageID,
		ScopeID:       s.scopeID,
		EventType:     domain.OutboxEventNotificationDispatch,
		AggregateType: domain.OutboxAggregateNotification,
		AggregateID:   delivery.DeliveryID,
		Payload:       mustJSON(message),
		Status:        "PENDING",
		RetryCount:    retryCount,
		NextRetryAt:   next,
	}
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if attempt != nil {
			if err := s.repo.InsertDeliveryAttempt(txCtx, attempt); err != nil {
				return err
			}
		}
		if err := s.repo.MarkDeliveryRetry(txCtx, delivery.DeliveryID, retryCount, next, lastError); err != nil {
			return err
		}
		return s.repo.AppendOutbox(txCtx, outbox)
	}); err != nil {
		return err
	}
	return deliveryAsyncHandledError{err: fmt.Errorf("%s", lastError)}
}

func (s *Service) decryptSecret(ctx context.Context, channel domain.Channel) (string, error) {
	if strings.TrimSpace(channel.SecretCiphertext) == "" {
		return "", nil
	}
	if s.secrets == nil {
		return "", fmt.Errorf("notification secret service is not configured")
	}
	return s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{
		CiphertextB64: channel.SecretCiphertext,
		EDEKB64:       channel.SecretEDEK,
		WrapKeyRef:    channel.SecretWrapKeyRef,
	})
}

func (s *Service) withinTx(ctx context.Context, fn func(context.Context) error) error {
	if s == nil || s.tx == nil || !s.tx.Enabled() {
		return fmt.Errorf("notification transaction is not configured")
	}
	if fn == nil {
		return fmt.Errorf("notification transaction callback is not configured")
	}
	return s.tx.WithinTransaction(ctx, fn)
}

func (s *Service) nextID() int64 {
	if s != nil && s.idGen != nil {
		return s.idGen.NextID()
	}
	return time.Now().UnixNano()
}

func (s *Service) logger() *zap.Logger {
	if s != nil && s.log != nil {
		return s.log
	}
	return zap.NewNop()
}

type deliveryAsyncHandledError struct {
	err error
}

func (e deliveryAsyncHandledError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e deliveryAsyncHandledError) Unwrap() error {
	return e.err
}

func isDeliveryAsyncHandled(err error) bool {
	var handled deliveryAsyncHandledError
	return errors.As(err, &handled)
}

func (s *Service) nextStringID() string {
	return fmt.Sprintf("%d", s.nextID())
}

type renderedTemplate struct {
	Subject  string
	Text     string
	HTML     string
	Markdown string
}

func renderTemplate(tpl domain.Template, variables map[string]any) (renderedTemplate, error) {
	return renderedTemplate{
		Subject:  executeTemplate(tpl.SubjectTemplate, variables),
		Text:     executeTemplate(tpl.TextTemplate, variables),
		HTML:     executeTemplate(tpl.HTMLTemplate, variables),
		Markdown: executeTemplate(tpl.MarkdownTemplate, variables),
	}, nil
}

func executeTemplate(pattern string, variables map[string]any) string {
	if strings.TrimSpace(pattern) == "" {
		return ""
	}
	tpl, err := template.New("notification").Parse(pattern)
	if err != nil {
		return pattern
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, variables); err != nil {
		return pattern
	}
	return buf.String()
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func maskTarget(target string) string {
	target = strings.TrimSpace(target)
	at := strings.Index(target, "@")
	if at > 1 {
		return target[:1] + "***" + target[at-1:]
	}
	if len(target) > 6 {
		return target[:3] + "***" + target[len(target)-3:]
	}
	return "***"
}

func sceneDisplayName(scene string) string {
	switch strings.TrimSpace(scene) {
	case "RESET_EMAIL":
		return "修改邮箱"
	case "RESET_PASSWORD":
		return "重置密码"
	case "LOGIN_UNLOCK":
		return "登录解锁"
	case "ACTIVE_USER":
		return "激活账户"
	default:
		return "安全验证"
	}
}

func backoff(retry int) time.Duration {
	if retry <= 0 {
		retry = 1
	}
	if retry > 6 {
		retry = 6
	}
	return time.Duration(1<<retry) * time.Minute
}

func choose(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func int64Ptr(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return &value
}

const currentRevisionBatchMaxIDs = 400

func normalizePage(current, pageSize int) (int, int) {
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return current, pageSize
}
