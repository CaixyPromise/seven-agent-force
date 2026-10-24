package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const (
	externalTextLimit          = 256
	externalTitleLimit         = 192
	feishuUUIDDeduplicationTTL = time.Hour
	externalDetailHint         = "请在系统中查看详情。"
)

var (
	providerErrorSecretPattern     = regexp.MustCompile(`(?i)\b(bearer\s+|(?:access[_-]?token|tenant[_-]?access[_-]?token|app[_-]?secret|corpsecret|authorization)\s*(?:=|:)\s*)[^\s,;]+`)
	providerErrorTargetPattern     = regexp.MustCompile(`\b(?:ou|oc)_[A-Za-z0-9_-]+\b`)
	providerErrorIdentifierPattern = regexp.MustCompile(`(?i)\b(?:open[_-]?id|receive[_-]?id|chat[_-]?id|user(?:[_-]?id)?|touser|invaliduser|unlicenseduser)\s*(?:=|:)\s*[^\s,;]+`)
	providerErrorIPPattern         = regexp.MustCompile(`(?i)\b((?:from\s+)?ip(?:\s*(?:=|:)\s*|\s+))(?:\[[^\]]+\]|[^\s,;]+)`)
	providerErrorURLPattern        = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)
)

// dispatchExternal sends one already materialized enterprise-member target.
// It never derives an external subject from a platform user or reads/writes
// inbox state. Provider replies are reduced to a small durable outcome before
// they reach delivery history.
func (s *Service) dispatchExternal(ctx context.Context, delivery *domain.Delivery, channel *domain.Channel) error {
	if delivery == nil || delivery.ExternalTargetID == nil || *delivery.ExternalTargetID <= 0 {
		return fmt.Errorf("notification external delivery is invalid")
	}
	target, err := s.repo.FindExternalTargetByID(ctx, *delivery.ExternalTargetID)
	if err != nil {
		return err
	}
	if target == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "TARGET_MISSING", Diagnostic: "TARGET_MISSING"})
	}
	if target.ScopeID != s.scopeID || target.NotificationID <= 0 || delivery.NotificationID == nil || *delivery.NotificationID != target.NotificationID {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "TARGET_SCOPE", Diagnostic: "TARGET_SCOPE"})
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled || channel.ScopeID != s.scopeID || channel.ChannelCode != target.ConnectionRef || channel.ChannelType != target.ProviderCode {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "CONNECTION_UNAVAILABLE", Diagnostic: "CONNECTION_UNAVAILABLE"})
	}
	if !domain.SupportsExternalDeliveryIdentityKind(channel.ChannelType, target.IdentityKind) {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "TARGET_KIND", Diagnostic: "TARGET_KIND"})
	}
	if domain.IsEnterpriseApplicationChannelType(channel.ChannelType) && channel.EnterpriseConnectionID == nil {
		if _, err := domain.ParseEnterpriseApplicationConfig(channel.ChannelType, channel.ConfigJSON); err != nil {
			return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "CONFIGURATION", Diagnostic: "CONFIGURATION"})
		}
	}
	if s.secrets == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "CONFIGURATION", Diagnostic: "CONFIGURATION"})
	}
	mention, suppressGroup, groupErr := s.resolveGroupRouteDispatch(ctx, delivery, channel)
	if groupErr != nil {
		return groupErr
	}
	if suppressGroup {
		return s.suppressExternalDelivery(ctx, delivery, domain.ErrGroupRouteNotEnabled.Error())
	}
	// Revalidate the frozen identity/grant/connection authority before
	// decrypting the external subject. A revoked identity must not even enter
	// the provider-attempt preparation path.
	if target.OrganizationID > 0 && target.GrantType != "GROUP_ROUTE" {
		if err := s.validateFrozenEnterpriseAuthority(ctx, target, channel); err != nil {
			return s.suppressExternalDelivery(ctx, delivery, "ENTERPRISE_NOTIFICATION_AUTHORITY_REVOKED")
		}
	}
	subject, err := s.secrets.DecryptString(ctx, targetSecretValue(target))
	if err != nil || strings.TrimSpace(subject) == "" {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "TARGET_DECRYPT", Diagnostic: "TARGET_DECRYPT"})
	}
	secretPlain := ""
	if target.OrganizationID > 0 {
		provider, appID, corpID, agentID, leasedSecret, _, expiresAt, err := s.enterpriseIdentities.AcquireEnterpriseApplicationCredential(ctx, target.OrganizationID, target.EnterpriseConnectionID, target.EnterpriseConnectionVersion, channel.EnterpriseConnectionDigest)
		if err != nil || !expiresAt.After(s.now()) || !strings.EqualFold(provider, providerForChannel(channel.ChannelType)) {
			return s.suppressExternalDelivery(ctx, delivery, "ENTERPRISE_CONNECTION_DRIFT")
		}
		configJSON, encodeErr := domain.EncodeEnterpriseApplicationConfig(channel.ChannelType, domain.EnterpriseApplicationConfig{AppID: appID, CorpID: corpID, AgentID: agentID})
		if encodeErr != nil {
			return s.suppressExternalDelivery(ctx, delivery, "ENTERPRISE_CONNECTION_DRIFT")
		}
		channelCopy := *channel
		channelCopy.ConfigJSON = configJSON
		channel = &channelCopy
		secretPlain = leasedSecret
	} else {
		secretPlain, err = s.decryptSecret(ctx, *channel)
		if err != nil || (strings.TrimSpace(secretPlain) == "" && channel.ChannelType != domain.ChannelTypeEmail) {
			return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "CONFIGURATION", Diagnostic: "CONFIGURATION"})
		}
	}
	params, err := domain.ParseProviderParamsJSON(target.ProviderParamsJSON)
	if err != nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "SNAPSHOT_INVALID", Diagnostic: "SNAPSHOT_INVALID"})
	}
	if s.drivers == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "DRIVER_UNAVAILABLE", Diagnostic: "DRIVER_UNAVAILABLE"})
	}
	driver, ok := s.drivers.Driver(channel.ChannelType).(ResultChannelDriver)
	if !ok || driver == nil {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "DRIVER_UNAVAILABLE", Diagnostic: "DRIVER_UNAVAILABLE"})
	}
	deepLink := ""
	if delivery.NotificationID != nil {
		notification, findErr := s.repo.FindLogicalNotificationByID(ctx, *delivery.NotificationID)
		if findErr != nil || notification == nil || notification.ScopeID != s.scopeID {
			return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "NOTIFICATION_INVALID", Diagnostic: "NOTIFICATION_INVALID"})
		}
		deepLink = notification.DeepLink
	}
	result, sendErr := driver.SendResult(ctx, DriverMessage{
		Channel:           *channel,
		SecretPlain:       secretPlain,
		IdentityKind:      target.IdentityKind,
		Target:            subject,
		Subject:           delivery.RenderedSubject,
		Text:              delivery.RenderedText,
		ProviderParams:    params,
		DeliveryID:        delivery.DeliveryID,
		DeepLink:          deepLink,
		PrimaryAction:     frozenPrimaryAction(delivery, deepLink),
		ExternalActionURL: delivery.ActionURLSnapshot,
		Presentation:      delivery.Presentation,
		Mention:           mention,
	})
	if sendErr != nil && strings.TrimSpace(result.Status) == "" {
		// A raw transport error cannot tell us whether the provider accepted the
		// request. Store no raw error and do not turn uncertainty into a replay.
		result = DriverResult{Status: DriverResultUnknown, FailureClass: "TRANSPORT", Diagnostic: "PROVIDER_REQUEST_UNCERTAIN", ProviderError: &ProviderError{Provider: channel.ChannelType, Message: sendErr.Error()}}
	}
	result = normalizeExternalDriverResult(result)
	switch result.Status {
	case DriverResultProviderAccepted:
		return s.finishExternalAccepted(ctx, delivery, result)
	case DriverResultUnknown:
		return s.finishExternalUnknown(ctx, delivery, result)
	default:
		if result.Retryable {
			if channel.ChannelType == domain.ChannelTypeFeishuApp && !s.feishuRetryIsWithinUUIDWindow(delivery) {
				return s.finishExternalUnknown(ctx, delivery, DriverResult{
					Status:       DriverResultUnknown,
					FailureClass: "AMBIGUOUS",
					Diagnostic:   "FEISHU_UUID_WINDOW_EXPIRED",
				})
			}
			return s.failOrRetryWithAttempt(ctx, delivery, result.Diagnostic, s.externalAttempt(delivery, result))
		}
		return s.finishExternalFailure(ctx, delivery, result)
	}
}

type deliveryCancelRepository interface {
	MarkDeliveryCanceled(ctx context.Context, deliveryID, reasonCode string) error
}

func (s *Service) suppressExternalDelivery(ctx context.Context, delivery *domain.Delivery, reasonCode string) error {
	repo, ok := s.repo.(deliveryCancelRepository)
	if !ok {
		return s.finishExternalFailure(ctx, delivery, DriverResult{Status: DriverResultFailed, FailureClass: "AUTHORITY", Diagnostic: reasonCode})
	}
	if err := repo.MarkDeliveryCanceled(ctx, delivery.DeliveryID, reasonCode); err != nil {
		return err
	}
	return deliveryAsyncHandledError{err: fmt.Errorf("%s", reasonCode)}
}

func (s *Service) validateFrozenEnterpriseAuthority(ctx context.Context, target *domain.ExternalTarget, channel *domain.Channel) error {
	if s.enterpriseIdentities == nil || target == nil || channel == nil || target.UserID <= 0 || target.ExternalIdentityID <= 0 || target.ExternalIdentityVersion <= 0 || target.EnterpriseConnectionID <= 0 || target.EnterpriseConnectionVersion <= 0 || target.AuthorityDigest == "" || target.GrantType == "" || channel.EnterpriseConnectionID == nil || *channel.EnterpriseConnectionID != target.EnterpriseConnectionID || channel.EnterpriseConnectionVersion != target.EnterpriseConnectionVersion {
		return fmt.Errorf("enterprise notification authority snapshot is invalid")
	}
	return s.enterpriseIdentities.ValidateEnterpriseNotificationAuthority(ctx, target.OrganizationID, target.UserID, target.ExternalIdentityID, target.ExternalIdentityVersion, target.EnterpriseConnectionID, target.EnterpriseConnectionVersion, target.AuthorityDigest, target.GrantType)
}

func providerForChannel(channelType string) string {
	if strings.EqualFold(channelType, domain.ChannelTypeWeComApp) {
		return "WECOM"
	}
	return "FEISHU"
}

func (s *Service) finishExternalAccepted(ctx context.Context, delivery *domain.Delivery, result DriverResult) error {
	return s.withinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.InsertDeliveryAttempt(txCtx, s.externalAttempt(delivery, result)); err != nil {
			return err
		}
		return s.repo.MarkDeliveryProviderAccepted(txCtx, delivery.DeliveryID, result.ProviderReference, s.now())
	})
}

func (s *Service) finishExternalUnknown(ctx context.Context, delivery *domain.Delivery, result DriverResult) error {
	return s.withinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.InsertDeliveryAttempt(txCtx, s.externalAttempt(delivery, result)); err != nil {
			return err
		}
		return s.repo.MarkDeliveryUnknown(txCtx, delivery.DeliveryID, result.Diagnostic)
	})
}

func (s *Service) finishExternalFailure(ctx context.Context, delivery *domain.Delivery, result DriverResult) error {
	result = normalizeExternalDriverResult(result)
	attempt := s.externalAttempt(delivery, result)
	retryCount := delivery.RetryCount + 1
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.InsertDeliveryAttempt(txCtx, attempt); err != nil {
			return err
		}
		return s.repo.MarkDeliveryFailed(txCtx, delivery.DeliveryID, retryCount, result.Diagnostic)
	}); err != nil {
		return err
	}
	return deliveryAsyncHandledError{err: fmt.Errorf("%s", result.Diagnostic)}
}

func (s *Service) externalAttempt(delivery *domain.Delivery, result DriverResult) *domain.DeliveryAttempt {
	attemptNo := 1
	if delivery != nil && delivery.RetryCount >= 0 {
		attemptNo = delivery.RetryCount + 1
	}
	return &domain.DeliveryAttempt{
		ID:                s.nextID(),
		AttemptID:         "nta_" + s.nextStringID(),
		DeliveryID:        delivery.DeliveryID,
		AttemptNo:         attemptNo,
		Status:            result.Status,
		FailureClass:      result.FailureClass,
		ProviderReference: result.ProviderReference,
		Diagnostic:        result.Diagnostic,
	}
}

func normalizeExternalDriverResult(result DriverResult) DriverResult {
	result.Status = strings.ToUpper(strings.TrimSpace(result.Status))
	switch result.Status {
	case DriverResultProviderAccepted, DriverResultUnknown, DriverResultFailed:
	default:
		result.Status = DriverResultUnknown
		result.Retryable = false
	}
	result.FailureClass = stableExternalCode(result.FailureClass, "PROVIDER")
	result.Diagnostic = stableExternalCode(result.Diagnostic, "PROVIDER_RESULT")
	result.ProviderReference = safeProviderReference(result.ProviderReference)
	result.ProviderError = normalizeProviderError(result.ProviderError)
	if result.Status != DriverResultFailed {
		result.Retryable = false
	}
	return result
}

func normalizeProviderError(value *ProviderError) *ProviderError {
	return domain.SanitizeProviderError(value)
}

// SanitizeProviderError applies the shared response boundary used by provider
// adapters before a DriverResult leaves the adapter. It is safe to call again
// at the application/API boundary.
func SanitizeProviderError(value *ProviderError) *ProviderError {
	return domain.SanitizeProviderError(value)
}

func safeProviderHTTPStatus(status int) int {
	if status < 100 || status > 599 {
		return 0
	}
	return status
}

func safeProviderErrorMessage(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = providerErrorURLPattern.ReplaceAllString(value, "[provider-url]")
	value = providerErrorSecretPattern.ReplaceAllString(value, "${1}[redacted]")
	value = providerErrorTargetPattern.ReplaceAllString(value, "[redacted]")
	value = providerErrorIPPattern.ReplaceAllString(value, "${1}[redacted]")
	value = providerErrorIdentifierPattern.ReplaceAllStringFunc(value, func(match string) string {
		if separator := strings.IndexAny(match, "=:"); separator >= 0 {
			return strings.TrimSpace(match[:separator+1]) + " [redacted]"
		}
		return "[redacted]"
	})
	return trimExternalRunes(value, externalTextLimit)
}

func facadeProviderError(value *ProviderError) *facade.ProviderError {
	if value == nil {
		return nil
	}
	return &facade.ProviderError{
		Provider:   value.Provider,
		HTTPStatus: value.HTTPStatus,
		Code:       value.Code,
		Message:    value.Message,
		LogID:      value.LogID,
	}
}

func stableExternalCode(value, fallback string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		return fallback
	}
	for _, character := range value {
		if character != '_' && !unicode.IsDigit(character) && (character < 'A' || character > 'Z') {
			return fallback
		}
	}
	return value
}

func safeProviderReference(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 191 {
		return ""
	}
	for _, character := range value {
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '_' && character != '-' && character != '.' && character != ':' {
			return ""
		}
	}
	return value
}

// externalText is the only production rendering used for an enterprise-member
// target. It intentionally never copies the logical inbox body: that body can
// be much longer and may carry data that is safe for the local inbox but not
// for a third-party application notification.
func externalText(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "通知"
	}
	return trimExternalRunes(title, externalTitleLimit) + "\n" + externalDetailHint
}

func trimExternalRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit-1]) + "…"
}

func externalProbeText(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		content = "连接测试消息"
	}
	return trimExternalRunes("连接测试\n"+content, externalTextLimit)
}

func (s *Service) feishuRetryIsWithinUUIDWindow(delivery *domain.Delivery) bool {
	if delivery == nil || delivery.CreateTime.IsZero() {
		// A zero creation timestamp is a repository invariant violation in real
		// dispatches. Treat it as ineligible rather than risking a duplicate send.
		return false
	}
	return s.now().Before(delivery.CreateTime.Add(feishuUUIDDeduplicationTTL))
}

// TestEnterpriseConnection performs a privileged, non-persistent probe of one
// saved enterprise application connection. It deliberately shares the same
// recipient validation, parameter resolution, guarded adapter, and normalized
// outcome path as asynchronous delivery without creating an inbox recipient or
// storing the supplied third-party subject.
func (s *Service) TestEnterpriseConnection(ctx context.Context, request facade.EnterpriseConnectionTestRequest) (*facade.EnterpriseConnectionTestResult, error) {
	if s == nil || s.repo == nil || s.drivers == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	prepared, warnings, err := s.prepareExternalRecipients(ctx, []facade.ExternalRecipient{{
		ConnectionRef:  request.ConnectionRef,
		IdentityKind:   request.IdentityKind,
		Subject:        request.Subject,
		ProviderParams: request.ProviderParams,
	}})
	if err != nil {
		return nil, err
	}
	if len(prepared) != 1 {
		return nil, fmt.Errorf("enterprise application probe target is invalid")
	}
	item := prepared[0]
	channel := item.channel
	secretPlain := ""
	if channel.EnterpriseConnectionID != nil {
		if channel.OrganizationID == nil || *channel.OrganizationID <= 0 || s.enterpriseIdentities == nil {
			return nil, apperrors.Operation("企业应用连接凭证不可用")
		}
		provider, appID, corpID, agentID, leasedSecret, _, expiresAt, acquireErr := s.enterpriseIdentities.AcquireEnterpriseApplicationCredential(ctx, *channel.OrganizationID, *channel.EnterpriseConnectionID, channel.EnterpriseConnectionVersion, channel.EnterpriseConnectionDigest)
		if acquireErr != nil || !expiresAt.After(s.now()) || !strings.EqualFold(provider, providerForChannel(channel.ChannelType)) || strings.TrimSpace(leasedSecret) == "" {
			return nil, apperrors.Operation("企业应用连接凭证不可用")
		}
		configJSON, encodeErr := domain.EncodeEnterpriseApplicationConfig(channel.ChannelType, domain.EnterpriseApplicationConfig{AppID: appID, CorpID: corpID, AgentID: agentID})
		if encodeErr != nil {
			return nil, apperrors.Operation("企业应用连接配置不可用")
		}
		channel.ConfigJSON = configJSON
		secretPlain = leasedSecret
	} else {
		secretPlain, err = s.decryptSecret(ctx, channel)
		if err != nil || strings.TrimSpace(secretPlain) == "" {
			return nil, apperrors.Operation("企业应用连接密钥不可用")
		}
	}
	driver, ok := s.drivers.Driver(channel.ChannelType).(ResultChannelDriver)
	if !ok || driver == nil {
		return nil, apperrors.Operation("企业应用连接暂不可测试")
	}
	text := strings.TrimSpace(request.Text)
	if text == "" {
		text = "连接测试消息"
	}
	result, sendErr := driver.SendResult(ctx, DriverMessage{
		Channel:        channel,
		SecretPlain:    secretPlain,
		IdentityKind:   item.identityKind,
		Target:         item.subject,
		Subject:        "连接测试",
		Text:           externalProbeText(text),
		ProviderParams: cloneProviderParams(item.providerParams),
		DeliveryID:     "probe-" + s.nextStringID(),
	})
	if sendErr != nil && strings.TrimSpace(result.Status) == "" {
		result = DriverResult{Status: DriverResultUnknown, FailureClass: "TRANSPORT", Diagnostic: "PROVIDER_REQUEST_UNCERTAIN", ProviderError: &ProviderError{Provider: item.channel.ChannelType, Message: sendErr.Error()}}
	}
	result = normalizeExternalDriverResult(result)
	s.logEnterpriseConnectionProbeFailure(channel.ChannelType, result)
	s.logProviderParameterWarnings(warnings)
	return &facade.EnterpriseConnectionTestResult{
		Status:            result.Status,
		FailureClass:      result.FailureClass,
		ProviderReference: result.ProviderReference,
		Diagnostic:        result.Diagnostic,
		ProviderError:     facadeProviderError(result.ProviderError),
		Warnings:          append([]facade.ProviderParameterWarning(nil), warnings...),
	}, nil
}

// logEnterpriseConnectionProbeFailure records only the normalized outcome of a
// failed, non-persistent connection probe. In particular, it deliberately
// excludes the supplied third-party subject, channel secret, token, and
// request payload so an operator can diagnose provider failures safely.
func (s *Service) logEnterpriseConnectionProbeFailure(provider string, result DriverResult) {
	s.logConnectionProbeFailure("notification_enterprise_connection_probe_failed", provider, result)
}

// logStaticConnectionProbeFailure retains only a normalized static connector
// outcome. It intentionally omits endpoint URLs, Authorization headers,
// request bodies and connection secrets.
func (s *Service) logStaticConnectionProbeFailure(provider string, result DriverResult) {
	s.logConnectionProbeFailure("notification_static_connection_probe_failed", provider, result)
}

func (s *Service) logConnectionProbeFailure(event, provider string, result DriverResult) {
	if result.Status == DriverResultProviderAccepted {
		return
	}
	fields := []zap.Field{
		zap.String("provider", stableExternalCode(provider, "PROVIDER")),
		zap.String("status", result.Status),
		zap.String("failureClass", result.FailureClass),
		zap.String("diagnostic", result.Diagnostic),
	}
	if detail := result.ProviderError; detail != nil {
		fields = append(fields,
			zap.Int("providerHTTPStatus", detail.HTTPStatus),
			zap.String("providerCode", detail.Code),
			zap.String("providerMessage", detail.Message),
			zap.String("providerLogId", detail.LogID),
		)
	}
	s.logger().Warn(event, fields...)
}
