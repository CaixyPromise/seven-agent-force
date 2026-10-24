package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

const (
	externalRecipientLimit   = 100
	externalDeliveryMaxRetry = 3
)

// preparedExternalRecipient contains only data already validated against an
// operator-configured connection. Its Subject is held in memory only until it
// is encrypted into the immutable target snapshot.
type preparedExternalRecipient struct {
	channel             domain.Channel
	identityKind        string
	subject             string
	subjectDigest       string
	subjectDigestKeyRef string
	providerParams      map[string]any
	providedParams      map[string]any
	providerParamsJSON  string
	authority           *enterpriseAuthoritySnapshot
	groupRoute          *preparedGroupRouteMetadata
}

func bindEnterpriseAuthoritySnapshots(items []preparedExternalRecipient, authorities map[string]enterpriseAuthoritySnapshot) error {
	for index := range items {
		snapshot, ok := authorities[enterpriseAuthorityKey(items[index].identityKind, items[index].subject)]
		if !ok {
			continue
		}
		channel := items[index].channel
		if snapshot.organizationID <= 0 || channel.ScopeType != "ORGANIZATION" || channel.OrganizationID == nil || *channel.OrganizationID != snapshot.organizationID || snapshot.enterpriseConnectionID <= 0 || channel.EnterpriseConnectionID == nil || *channel.EnterpriseConnectionID != snapshot.enterpriseConnectionID || channel.EnterpriseConnectionVersion != snapshot.connectionVersion || channel.EnterpriseConnectionDigest == "" || channel.EnterpriseConnectionDigest != snapshot.connectionConfigDigest {
			return apperrors.Operation("企业个人通知连接与身份授权不一致")
		}
		items[index].authority = &snapshot
	}
	return nil
}

func (s *Service) prepareExternalRecipients(ctx context.Context, recipients []facade.ExternalRecipient) ([]preparedExternalRecipient, []facade.ProviderParameterWarning, error) {
	if len(recipients) == 0 {
		return []preparedExternalRecipient{}, []facade.ProviderParameterWarning{}, nil
	}
	if len(recipients) > externalRecipientLimit {
		return nil, nil, apperrors.Params("第三方收件人数量超过单次上限")
	}
	if s == nil || s.repo == nil || s.digester == nil {
		return nil, nil, fmt.Errorf("notification external target security is not configured")
	}
	prepared := make([]preparedExternalRecipient, 0, len(recipients))
	warnings := make([]facade.ProviderParameterWarning, 0)
	seen := make(map[string]string, len(recipients))
	connectionRefs := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		connectionRefs = append(connectionRefs, recipient.ConnectionRef)
	}
	channels, err := s.loadChannelsByCodes(ctx, connectionRefs)
	if err != nil {
		return nil, nil, err
	}
	for _, recipient := range recipients {
		connectionRef := strings.TrimSpace(recipient.ConnectionRef)
		identityKind := strings.ToUpper(strings.TrimSpace(string(recipient.IdentityKind)))
		subject := strings.TrimSpace(recipient.Subject)
		if connectionRef == "" || identityKind == "" || subject == "" {
			return nil, nil, apperrors.Params("第三方收件人缺少连接、身份类型或目标标识")
		}
		if len(connectionRef) > 64 {
			return nil, nil, apperrors.Params("第三方收件人字段长度无效")
		}
		channel, found := channels[connectionRef]
		if !found || channel.Status != domain.ChannelStatusEnabled || !domain.IsEnterpriseApplicationChannelType(channel.ChannelType) {
			return nil, nil, apperrors.Operation("企业应用连接不可用")
		}
		if strings.TrimSpace(channel.ScopeID) == "" || channel.ScopeID != s.scopeID {
			return nil, nil, apperrors.Operation("企业应用连接不属于当前作用域")
		}
		if !domain.SupportsEnterpriseApplicationIdentityKind(channel.ChannelType, identityKind) {
			return nil, nil, apperrors.Params("第三方目标身份类型与企业应用连接不匹配")
		}
		subject, err = domain.NormalizeExternalTargetSubject(identityKind, subject)
		if err != nil {
			return nil, nil, apperrors.Params("第三方目标标识无效")
		}
		if channel.EnterpriseConnectionID == nil {
			if _, err := domain.ParseEnterpriseApplicationConfig(channel.ChannelType, channel.ConfigJSON); err != nil {
				return nil, nil, apperrors.Operation("企业应用连接配置不完整")
			}
			if strings.TrimSpace(channel.SecretCiphertext) == "" || strings.TrimSpace(channel.SecretEDEK) == "" || strings.TrimSpace(channel.SecretWrapKeyRef) == "" {
				return nil, nil, apperrors.Operation("企业应用连接密钥未配置")
			}
		}
		settings, err := domain.ParseProviderParameterSettings(channel.MetadataJSON)
		if err != nil {
			return nil, nil, apperrors.Operation("企业应用参数配置无效")
		}
		settings, err = domain.NormalizeProviderParameterSettings(channel.ChannelType, settings)
		if err != nil {
			return nil, nil, apperrors.Operation("企业应用参数配置无效")
		}
		resolved, provided, parameterWarnings := domain.ResolveProviderParameters(channel.ChannelType, settings, recipient.ProviderParams)
		for _, warning := range parameterWarnings {
			warnings = append(warnings, facade.ProviderParameterWarning{Provider: warning.Provider, Key: warning.Key, Reason: warning.Reason})
		}
		paramsJSON, err := domain.CanonicalProviderParamsJSON(resolved)
		if err != nil {
			return nil, nil, fmt.Errorf("encode enterprise application parameters: %w", err)
		}
		subjectDigest, subjectDigestKeyRef, err := s.digester.Digest(ctx, "", s.scopeID, connectionRef, identityKind, subject)
		if err != nil || strings.TrimSpace(subjectDigest) == "" || strings.TrimSpace(subjectDigestKeyRef) == "" {
			return nil, nil, fmt.Errorf("digest enterprise application target: %w", err)
		}
		duplicateKey := strings.Join([]string{connectionRef, identityKind, subjectDigest}, "\x00")
		if previousParams, exists := seen[duplicateKey]; exists {
			if previousParams != paramsJSON {
				return nil, nil, apperrors.Params("同一第三方目标不能携带冲突的可选投递参数")
			}
			continue
		}
		seen[duplicateKey] = paramsJSON
		prepared = append(prepared, preparedExternalRecipient{
			channel:             channel,
			identityKind:        identityKind,
			subject:             subject,
			subjectDigest:       subjectDigest,
			subjectDigestKeyRef: subjectDigestKeyRef,
			providerParams:      cloneProviderParams(resolved),
			providedParams:      cloneProviderParams(provided),
			providerParamsJSON:  paramsJSON,
		})
	}
	sort.Slice(prepared, func(i, j int) bool {
		if prepared[i].channel.ChannelCode != prepared[j].channel.ChannelCode {
			return prepared[i].channel.ChannelCode < prepared[j].channel.ChannelCode
		}
		if prepared[i].identityKind != prepared[j].identityKind {
			return prepared[i].identityKind < prepared[j].identityKind
		}
		return prepared[i].subjectDigest < prepared[j].subjectDigest
	})
	return prepared, warnings, nil
}

func (s *Service) prepareVerifiedEmailRecipients(ctx context.Context, userIDs []int64, channel *domain.Channel) ([]preparedExternalRecipient, []facade.ProviderParameterWarning, error) {
	if len(userIDs) == 0 {
		return nil, nil, nil
	}
	if channel == nil || channel.Status != domain.ChannelStatusEnabled || !strings.EqualFold(channel.ChannelType, domain.ChannelTypeEmail) || !s.channelBelongsToCurrentScope(channel) {
		return nil, nil, apperrors.Operation("邮件发送方式不可用")
	}
	if s.verifiedEmails == nil || s.digester == nil {
		return nil, nil, fmt.Errorf("verified email recipient resolution is not configured")
	}
	seen := make(map[string]struct{}, len(userIDs))
	items := make([]preparedExternalRecipient, 0, len(userIDs))
	warnings := make([]facade.ProviderParameterWarning, 0)
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		email, ready, err := s.verifiedEmails.ResolveVerifiedNotificationEmail(ctx, userID)
		if err != nil {
			return nil, nil, err
		}
		if !ready {
			warnings = append(warnings, facade.ProviderParameterWarning{Provider: domain.ChannelTypeEmail, Reason: domain.ReasonNotificationRecipientNotVerified})
			continue
		}
		email, err = domain.NormalizeEmailTarget(email)
		if err != nil {
			return nil, nil, fmt.Errorf("verified email resolver returned an invalid target")
		}
		digest, keyRef, err := s.digester.Digest(ctx, "", s.scopeID, channel.ChannelCode, domain.ExternalIdentityEmailAddress, email)
		if err != nil || strings.TrimSpace(digest) == "" || strings.TrimSpace(keyRef) == "" {
			return nil, nil, fmt.Errorf("digest verified email target: %w", err)
		}
		if _, duplicate := seen[digest]; duplicate {
			continue
		}
		seen[digest] = struct{}{}
		items = append(items, preparedExternalRecipient{
			channel: channelClone(*channel), identityKind: domain.ExternalIdentityEmailAddress,
			subject: email, subjectDigest: digest, subjectDigestKeyRef: keyRef,
			providerParams: map[string]any{}, providedParams: map[string]any{}, providerParamsJSON: "{}",
		})
	}
	return items, warnings, nil
}

func channelClone(value domain.Channel) domain.Channel { return value }

func (s *Service) applyExistingDirectIdempotency(ctx context.Context, existing, incoming *domain.LogicalNotification, audience domain.AudienceSnapshot, recipients []facade.ExternalRecipient, staticRoutes []facade.StaticRoute, receipt *facade.PublishReceipt) ([]facade.ProviderParameterWarning, error) {
	if existing == nil || incoming == nil || receipt == nil {
		return nil, fmt.Errorf("notification idempotency state is invalid")
	}
	targets, err := s.repo.ListExternalTargetsByNotificationID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	warnings, err := s.matchExistingExternalTargets(ctx, targets, recipients)
	if err != nil {
		return nil, err
	}
	routes, err := s.matchExistingStaticRoutes(ctx, existing, staticRoutes)
	if err != nil {
		return nil, err
	}
	fingerprints := make([]domain.ExternalRecipientFingerprint, 0, len(targets))
	for _, target := range targets {
		fingerprints = append(fingerprints, domain.ExternalRecipientFingerprint{
			ConnectionRef:      target.ConnectionRef,
			ProviderCode:       target.ProviderCode,
			IdentityKind:       target.IdentityKind,
			SubjectDigest:      target.SubjectDigest,
			ProviderParamsJSON: normalizedProviderParamsJSON(target.ProviderParamsJSON),
		})
	}
	comparison := *incoming
	comparison.RequestFingerprint = domain.CanonicalFingerprintWithStaticRoutes(comparison, audience, fingerprints, routes)
	if err := applyIdempotentReceipt(existing, &comparison, receipt); err != nil {
		return nil, err
	}
	return warnings, nil
}

func (s *Service) matchExistingExternalTargets(ctx context.Context, targets []domain.ExternalTarget, recipients []facade.ExternalRecipient) ([]facade.ProviderParameterWarning, error) {
	if len(recipients) > externalRecipientLimit {
		return nil, apperrors.Params("第三方收件人数量超过单次上限")
	}
	if len(targets) == 0 && len(recipients) == 0 {
		return []facade.ProviderParameterWarning{}, nil
	}
	if s == nil || s.digester == nil {
		return nil, fmt.Errorf("notification external target security is not configured")
	}
	matched := make(map[int64]struct{}, len(targets))
	incomingSeen := make(map[int64]string, len(recipients))
	warnings := make([]facade.ProviderParameterWarning, 0)
	for _, recipient := range recipients {
		connectionRef := strings.TrimSpace(recipient.ConnectionRef)
		identityKind := strings.ToUpper(strings.TrimSpace(string(recipient.IdentityKind)))
		subject := strings.TrimSpace(recipient.Subject)
		if connectionRef == "" || identityKind == "" || subject == "" || len(connectionRef) > 64 {
			return nil, apperrors.Params("第三方收件人缺少连接、身份类型或目标标识")
		}
		normalizedSubject, normalizeErr := domain.NormalizeExternalTargetSubject(identityKind, subject)
		if normalizeErr != nil {
			return nil, apperrors.Params("第三方目标标识无效")
		}
		subject = normalizedSubject
		var matchedTarget *domain.ExternalTarget
		for index := range targets {
			target := &targets[index]
			if target.ConnectionRef != connectionRef || target.IdentityKind != identityKind || !domain.SupportsEnterpriseApplicationIdentityKind(target.ProviderCode, identityKind) {
				continue
			}
			digest, _, digestErr := s.digester.Digest(ctx, target.SubjectDigestKeyRef, target.ScopeID, connectionRef, identityKind, subject)
			if digestErr != nil {
				return nil, fmt.Errorf("digest idempotent enterprise application target: %w", digestErr)
			}
			if digest == target.SubjectDigest {
				matchedTarget = target
				break
			}
		}
		if matchedTarget == nil {
			return nil, idempotencyConflict()
		}
		_, provided, itemWarnings := domain.ResolveProviderParameters(matchedTarget.ProviderCode, nil, recipient.ProviderParams)
		for _, warning := range itemWarnings {
			warnings = append(warnings, facade.ProviderParameterWarning{Provider: warning.Provider, Key: warning.Key, Reason: warning.Reason})
		}
		snapshot, parseErr := domain.ParseProviderParamsJSON(matchedTarget.ProviderParamsJSON)
		if parseErr != nil {
			return nil, fmt.Errorf("parse idempotent enterprise application target snapshot: %w", parseErr)
		}
		for key, supplied := range provided {
			stored, exists := snapshot[key]
			if !exists || !providerParamValueEqual(stored, supplied) {
				return nil, idempotencyConflict()
			}
		}
		providedJSON, encodeErr := domain.CanonicalProviderParamsJSON(provided)
		if encodeErr != nil {
			return nil, encodeErr
		}
		if prior, duplicate := incomingSeen[matchedTarget.ID]; duplicate {
			if prior != providedJSON {
				return nil, apperrors.Params("同一第三方目标不能携带冲突的可选投递参数")
			}
			continue
		}
		incomingSeen[matchedTarget.ID] = providedJSON
		matched[matchedTarget.ID] = struct{}{}
	}
	if len(matched) != len(targets) {
		return nil, idempotencyConflict()
	}
	return warnings, nil
}

func normalizedProviderParamsJSON(raw string) string {
	values, err := domain.ParseProviderParamsJSON(raw)
	if err != nil {
		return "{}"
	}
	encoded, err := domain.CanonicalProviderParamsJSON(values)
	if err != nil {
		return "{}"
	}
	return encoded
}

func providerParamValueEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}

func idempotencyConflict() error {
	return apperrors.ObjectState("幂等键与既有请求不一致").WithDetails(map[string]string{"reasonCode": "IDEMPOTENCY_CONFLICT"})
}

func (s *Service) externalRecipientFingerprints(items []preparedExternalRecipient) []domain.ExternalRecipientFingerprint {
	result := make([]domain.ExternalRecipientFingerprint, 0, len(items))
	for _, item := range items {
		result = append(result, domain.ExternalRecipientFingerprint{
			ConnectionRef:      item.channel.ChannelCode,
			ProviderCode:       item.channel.ChannelType,
			IdentityKind:       item.identityKind,
			SubjectDigest:      item.subjectDigest,
			ProviderParamsJSON: item.providerParamsJSON,
		})
	}
	return result
}

// sceneDeliveryRender is the immutable per-receiver rendering accepted from a
// G6.2 scene. It is intentionally delivery-local because one semantic Publish
// request may include an inbox and an external target with different approved
// templates; a global logical-notification body must not overwrite either.
type sceneDeliveryRender struct {
	SceneCode          string
	TemplateCode       string
	RenderedSubject    string
	RenderedText       string
	RenderedHTML       string
	RenderedMarkdown   string
	PrimaryActionLabel string
	ContentTier        string
	SceneSnapshotID    *int64
}

func (s *Service) createExternalTargetsAndDeliveries(ctx context.Context, notification *domain.LogicalNotification, items []preparedExternalRecipient) error {
	return s.createExternalTargetsAndDeliveriesWithScene(ctx, notification, items, nil)
}

// createExternalTargetsAndDeliveriesWithScene persists the same encrypted G4
// target snapshot as the legacy path, with an optional G6.2 render override.
// The override has no plaintext target, connection secret or provider body.
func (s *Service) createExternalTargetsAndDeliveriesWithScene(ctx context.Context, notification *domain.LogicalNotification, items []preparedExternalRecipient, renders map[string]sceneDeliveryRender) error {
	if len(items) == 0 {
		return nil
	}
	if len(items) > externalRecipientLimit {
		return apperrors.Params("第三方收件人数量超过单次上限")
	}
	if notification == nil || notification.ID <= 0 {
		return fmt.Errorf("notification external target has no logical notification")
	}
	if s == nil || s.secrets == nil || s.repo == nil {
		return fmt.Errorf("notification external delivery is not configured")
	}
	deliveryBatch, ok := s.repo.(deliveryBatchRepository)
	if !ok {
		return fmt.Errorf("notification delivery batch repository is not configured")
	}
	outboxBatch, ok := s.repo.(outboxBatchRepository)
	if !ok {
		return fmt.Errorf("notification outbox batch repository is not configured")
	}
	targets := make([]domain.ExternalTarget, 0, len(items))
	for _, item := range items {
		encrypted, err := s.secrets.EncryptString(ctx, item.subject)
		if err != nil {
			return fmt.Errorf("encrypt enterprise application target: %w", err)
		}
		if strings.TrimSpace(encrypted.CiphertextB64) == "" || strings.TrimSpace(encrypted.EDEKB64) == "" || strings.TrimSpace(encrypted.WrapKeyRef) == "" {
			return fmt.Errorf("encrypt enterprise application target returned an incomplete envelope")
		}
		targets = append(targets, domain.ExternalTarget{
			ID:                  s.nextID(),
			ExternalTargetID:    "net_" + s.nextStringID(),
			NotificationID:      notification.ID,
			ScopeID:             notification.ScopeID,
			ConnectionRef:       item.channel.ChannelCode,
			ProviderCode:        item.channel.ChannelType,
			IdentityKind:        item.identityKind,
			SubjectCiphertext:   encrypted.CiphertextB64,
			SubjectEDEK:         encrypted.EDEKB64,
			SubjectWrapKeyRef:   encrypted.WrapKeyRef,
			SubjectDigest:       item.subjectDigest,
			SubjectDigestKeyRef: item.subjectDigestKeyRef,
			ProviderParamsJSON:  item.providerParamsJSON,
		})
		if item.authority != nil {
			target := &targets[len(targets)-1]
			target.OrganizationID = item.authority.organizationID
			target.UserID = item.authority.userID
			target.ExternalIdentityID = item.authority.externalIdentityID
			target.ExternalIdentityVersion = item.authority.identityVersion
			target.EnterpriseConnectionID = item.authority.enterpriseConnectionID
			target.EnterpriseConnectionVersion = item.authority.connectionVersion
			target.AuthorityDigest = item.authority.authorityDigest
			target.GrantType = item.authority.grantType
		}
	}
	if err := s.repo.InsertExternalTargets(ctx, targets); err != nil {
		return err
	}
	deliveries := make([]domain.Delivery, 0, len(targets))
	groupSnapshots := make([]domain.GroupRouteSnapshot, 0)
	outboxEvents := make([]domain.OutboxEvent, 0, len(targets))
	for index, target := range targets {
		connection := items[index].channel
		render, hasRender := renders[sceneExternalRenderKey(items[index])]
		deliveryID := "ntf_ext_" + s.nextStringID()
		notificationID := notification.ID
		externalTargetID := target.ID
		createdAt := s.now()
		delivery := domain.Delivery{
			ID:               s.nextID(),
			DeliveryID:       deliveryID,
			RequestDigest:    digest("external", notification.NotificationID, target.ExternalTargetID, connection.ChannelCode),
			NotificationID:   &notificationID,
			ExternalTargetID: &externalTargetID,
			SceneCode:        externalDeliverySceneCode(notification.EventKey),
			ChannelCode:      connection.ChannelCode,
			ChannelType:      connection.ChannelType,
			TemplateCode:     "semantic-external",
			TargetMasked:     domain.MaskExternalSubject(items[index].subject),
			RenderedSubject:  notification.Title,
			// Persist only the bounded outbound rendering. The logical inbox body
			// remains on the notification/inbox side and must not be duplicated as
			// an unbounded third-party delivery payload.
			RenderedText: externalText(notification.Title),
			Status:       domain.DeliveryStatusPending,
			MaxRetry:     externalDeliveryMaxRetry,
			TraceID:      notification.TraceID,
			CreatorID:    notification.CreatorID,
			CreateTime:   createdAt,
			UpdateTime:   createdAt,
		}
		if target.OrganizationID > 0 {
			organizationID := target.OrganizationID
			delivery.OrganizationID = &organizationID
		}
		if hasRender {
			delivery.SceneCode = render.SceneCode
			delivery.TemplateCode = render.TemplateCode
			delivery.SceneSnapshotID = render.SceneSnapshotID
			delivery.RenderedSubject = render.RenderedSubject
			delivery.RenderedText = boundedExternalRenderedText(render.RenderedText, render.RenderedSubject)
			delivery.RenderedHTML = render.RenderedHTML
			delivery.RenderedMarkdown = render.RenderedMarkdown
			delivery.ContentTier = render.ContentTier
		}
		presentation, actionLabel, actionURL, renderedDigest, freezeErr := s.freezeDeliveryPresentation(notification, connection.ChannelType, render.PrimaryActionLabel)
		if freezeErr != nil {
			return freezeErr
		}
		delivery.Presentation = presentation
		delivery.ActionLabelSnapshot = actionLabel
		delivery.ActionURLSnapshot = actionURL
		mentionDigest := ""
		if items[index].groupRoute != nil && items[index].groupRoute.mentionAuthority != nil {
			mentionDigest = items[index].groupRoute.mentionAuthority.authorityDigest
		}
		delivery.RenderedPayloadDigest = digest(renderedDigest, delivery.RenderedSubject, delivery.RenderedText, delivery.RenderedHTML, delivery.RenderedMarkdown, mentionDigest)
		if items[index].groupRoute != nil {
			groupSnapshots = append(groupSnapshots, s.newGroupRouteSnapshot(notification, delivery, *items[index].groupRoute))
		}
		message := domain.DeliveryMessage{MessageID: "notification:" + deliveryID, DeliveryID: deliveryID, ScopeID: notification.ScopeID}
		deliveries = append(deliveries, delivery)
		outboxEvents = append(outboxEvents, domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       message.MessageID,
			ScopeID:       notification.ScopeID,
			EventType:     domain.OutboxEventNotificationDispatch,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   deliveryID,
			Payload:       mustJSON(message),
			Status:        "PENDING",
		})
	}
	if err := deliveryBatch.InsertDeliveries(ctx, deliveries); err != nil {
		return err
	}
	if len(groupSnapshots) > 0 {
		repo, ok := s.repo.(groupRouteSnapshotRepository)
		if !ok {
			return fmt.Errorf("notification group route snapshot repository is not configured")
		}
		if err := repo.InsertGroupRouteSnapshots(ctx, groupSnapshots); err != nil {
			return err
		}
	}
	return outboxBatch.AppendOutboxBatch(ctx, outboxEvents)
}

func sceneExternalRenderKey(item preparedExternalRecipient) string {
	return sceneExternalInputKey(item.identityKind, item.channel.ChannelCode)
}

func boundedExternalRenderedText(text, subject string) string {
	text = trimExternalRunes(text, externalTextLimit)
	if text != "" {
		return text
	}
	return externalText(subject)
}

func (s *Service) logProviderParameterWarnings(warnings []facade.ProviderParameterWarning) {
	for _, warning := range warnings {
		s.logger().Warn("notification_provider_parameter_ignored",
			zap.String("code", domain.ProviderParameterWarningIgnored),
			zap.String("provider", warning.Provider),
			zap.String("key", warning.Key),
			zap.String("reason", warning.Reason),
		)
	}
}

func cloneProviderParams(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		return map[string]any{}
	}
	return result
}

func externalDeliverySceneCode(eventKey string) string {
	eventKey = strings.TrimSpace(eventKey)
	if len(eventKey) <= 64 {
		return eventKey
	}
	return eventKey[:64]
}

func targetSecretValue(target *domain.ExternalTarget) secretvalueinfra.SecretValue {
	if target == nil {
		return secretvalueinfra.SecretValue{}
	}
	return secretvalueinfra.SecretValue{
		CiphertextB64: target.SubjectCiphertext,
		EDEKB64:       target.SubjectEDEK,
		WrapKeyRef:    target.SubjectWrapKeyRef,
	}
}
