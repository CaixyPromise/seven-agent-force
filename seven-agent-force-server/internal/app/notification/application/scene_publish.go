package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

// Publish keeps the established semantic Client path while making an explicit
// SceneCode strict: a selected versioned scene never falls back to caller
// content or an older scene configuration.
func (s *Service) Publish(ctx context.Context, request facade.PublishRequest) (*facade.PublishReceipt, error) {
	if strings.TrimSpace(request.SceneCode) == "" {
		return s.publishDirect(ctx, request)
	}
	return s.publishScene(ctx, request)
}

type scenePublishResolution struct {
	definition         *domain.SceneDefinition
	revision           *domain.SceneRevision
	templateDefinition *domain.TemplateDefinition
	templateRevision   *domain.TemplateRevision
	channel            *domain.Channel
	rendered           domain.RenderedTemplateRevision
	contentTier        string
	receiverKind       string
	disabled           bool
	snapshotID         *int64
}

type scenePublishPlan struct {
	effectiveAudience     facade.Audience
	externalInputs        []facade.ExternalRecipient
	preparedEmails        []preparedExternalRecipient
	recipientWarnings     []facade.ProviderParameterWarning
	staticRoutes          []facade.StaticRoute
	resolutions           map[string]*scenePublishResolution
	canonicalSubject      string
	canonicalContent      string
	primaryActionLabel    string
	disabledCount         int
	enterpriseAuthorities map[string]enterpriseAuthoritySnapshot
	groupExternal         []preparedExternalRecipient
	groupStatic           []preparedStaticRoute
}

type preparedGroupRouteMetadata struct {
	route            domain.GroupRoute
	mentionUserID    int64
	mentionAuthority *enterpriseAuthoritySnapshot
	warningCode      string
}

type enterpriseAuthoritySnapshot struct {
	organizationID         int64
	userID                 int64
	externalIdentityID     int64
	identityVersion        int64
	enterpriseConnectionID int64
	connectionVersion      int64
	connectionConfigDigest string
	authorityDigest        string
	grantType              string
}

// publishScene resolves published scene/template/connection records inside
// the acceptance transaction, renders strictly, persists snapshots, then uses
// the same durable target/outbox machinery as V1. It never re-resolves a
// retry against current configuration once an idempotency record exists.
func (s *Service) publishScene(ctx context.Context, request facade.PublishRequest) (*facade.PublishReceipt, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	if strings.TrimSpace(s.scopeID) == "" {
		return nil, fmt.Errorf("notification scope is not configured")
	}
	fingerprint, err := scenePublishFingerprint(request, s.scopeID)
	if err != nil {
		return nil, err
	}
	receipt := &facade.PublishReceipt{}
	var warnings []facade.ProviderParameterWarning
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repo.FindLogicalNotificationByIdempotency(txCtx, s.scopeID, strings.TrimSpace(request.EventKey), strings.TrimSpace(request.IdempotencyKey))
		if findErr != nil {
			return findErr
		}
		if existing != nil {
			return s.applyExistingSceneIdempotency(txCtx, existing, fingerprint, request, receipt)
		}

		plan, resolveErr := s.resolveScenePublishPlan(txCtx, request)
		if resolveErr != nil {
			return resolveErr
		}
		acceptedExternal := len(plan.externalInputs) + len(plan.preparedEmails) + len(plan.staticRoutes) + len(plan.groupExternal) + len(plan.groupStatic)
		if len(plan.effectiveAudience.UserIDs)+len(plan.effectiveAudience.RoleIDs) == 0 && acceptedExternal == 0 {
			if plan.disabledCount > 0 {
				return sceneDisabledError()
			}
			return apperrors.Params("通知至少需要一个站内信受众或第三方收件人")
		}

		itemRequest := request
		itemRequest.Audience = plan.effectiveAudience
		itemRequest.Title = plan.canonicalSubject
		itemRequest.Content = plan.canonicalContent
		item, audience, createErr := s.newLogicalNotificationBase(itemRequest, acceptedExternal)
		if createErr != nil {
			return createErr
		}
		item.RequestFingerprint = fingerprint
		if strings.TrimSpace(plan.primaryActionLabel) != "" && strings.TrimSpace(item.DeepLink) != "" {
			link, linkErr := domain.ParseInternalDeepLink(item.DeepLink)
			if linkErr != nil {
				return apperrors.Params(domain.ReasonNotificationDeepLinkInvalid)
			}
			action, actionErr := domain.NewPrimaryInternalAction(plan.primaryActionLabel, link)
			if actionErr != nil {
				return apperrors.Params(domain.ReasonNotificationActionInvalid)
			}
			item.ActionType = action.Type
			item.ActionLabelSnapshot = action.Label
			item.ActionStyle = action.Style
		} else {
			item.ActionType = ""
			item.ActionLabelSnapshot = ""
			item.ActionStyle = ""
		}

		created, createErr := s.repo.CreateLogicalNotification(txCtx, item)
		if createErr != nil {
			return createErr
		}
		if !created {
			existing, retryFindErr := s.repo.FindLogicalNotificationByIdempotency(txCtx, item.ScopeID, item.EventKey, item.IdempotencyKey)
			if retryFindErr != nil {
				return retryFindErr
			}
			if existing == nil {
				return fmt.Errorf("notification idempotency record disappeared")
			}
			return s.applyExistingSceneIdempotency(txCtx, existing, fingerprint, request, receipt)
		}

		if snapshotErr := s.insertScenePublishSnapshots(txCtx, item, plan, request.TemplateVariables); snapshotErr != nil {
			return snapshotErr
		}
		deferred := audience.HasDeferredAudience() || len(audience.UserIDs) > inlineRecipientLimit || isScheduled(item.ScheduleAt, s.now())
		if deferred {
			if err := s.createMaterializationTask(txCtx, item, audience); err != nil {
				return err
			}
		} else {
			recipients := s.newRecipients(item, audience.UserIDs)
			createdRecipients, insertErr := s.repo.InsertInboxRecipients(txCtx, recipients)
			if insertErr != nil {
				return insertErr
			}
			if err := s.appendInboxChangedIntents(txCtx, createdRecipients, true); err != nil {
				return err
			}
		}

		preparedExternal, preparedWarnings, prepareErr := s.prepareExternalRecipients(txCtx, plan.externalInputs)
		if prepareErr != nil {
			return prepareErr
		}
		if decorateErr := bindEnterpriseAuthoritySnapshots(preparedExternal, plan.enterpriseAuthorities); decorateErr != nil {
			return decorateErr
		}
		preparedExternal = append(preparedExternal, plan.preparedEmails...)
		preparedExternal = append(preparedExternal, plan.groupExternal...)
		preparedWarnings = append(preparedWarnings, plan.recipientWarnings...)
		preparedStatic, staticErr := s.prepareStaticRoutes(txCtx, plan.staticRoutes)
		if staticErr != nil {
			return staticErr
		}
		preparedStatic = append(preparedStatic, plan.groupStatic...)
		if err := s.createExternalTargetsAndDeliveriesWithScene(txCtx, item, preparedExternal, plan.externalRenders()); err != nil {
			return err
		}
		if err := s.createStaticRouteDeliveriesWithScene(txCtx, item, preparedStatic, plan.staticRenders()); err != nil {
			return err
		}

		intent := domain.IntentMessage{NotificationID: item.ID, ScopeID: item.ScopeID}
		if err := s.repo.AppendOutbox(txCtx, &domain.OutboxEvent{
			ID:            s.nextID(),
			EventID:       "notification-intent:" + item.NotificationID,
			ScopeID:       item.ScopeID,
			EventType:     domain.OutboxEventNotificationIntent,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   item.NotificationID,
			Payload:       mustJSON(intent),
			Status:        "PENDING",
		}); err != nil {
			return err
		}
		receipt.NotificationID = item.NotificationID
		receipt.Status = item.Status
		receipt.MaterializationStatus = materializationStatus(deferred, item.Status)
		warnings = preparedWarnings
		receipt.Warnings = append([]facade.ProviderParameterWarning(nil), preparedWarnings...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logProviderParameterWarnings(warnings)
	return receipt, nil
}

func (s *Service) resolveScenePublishPlan(ctx context.Context, request facade.PublishRequest) (*scenePublishPlan, error) {
	if len(request.UserAudiences) > 0 {
		var err error
		request, err = s.applyUserAudiencePreferences(ctx, request)
		if err != nil {
			return nil, err
		}
	}
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	plan := &scenePublishPlan{
		resolutions: make(map[string]*scenePublishResolution),
	}
	personalRecipients, authorities, err := s.resolveEnterprisePersonalRecipients(ctx, request)
	if err != nil {
		return nil, err
	}
	allExternalRecipients := append([]facade.ExternalRecipient(nil), request.ExternalRecipients...)
	allExternalRecipients = append(allExternalRecipients, personalRecipients...)
	plan.enterpriseAuthorities = authorities
	needed := make(map[string]struct{})
	requestedInApp := false
	for _, requestedKind := range request.ReceiverKinds {
		if requestedKind == facade.ReceiverKindFeishuGroup || requestedKind == facade.ReceiverKindWeComGroup {
			continue
		}
		kind, kindErr := domain.NormalizeSceneReceiverKind(string(requestedKind))
		if kindErr != nil || kind == domain.SceneReceiverKindFixedConnection {
			return nil, apperrors.Params("通知接收类型无效")
		}
		needed[kind] = struct{}{}
		if kind == domain.SceneReceiverKindInApp {
			requestedInApp = true
		}
	}
	if len(request.ReceiverKinds) == 0 && len(request.Audience.UserIDs)+len(request.Audience.RoleIDs) > 0 {
		needed[domain.SceneReceiverKindInApp] = struct{}{}
		requestedInApp = true
	}
	if len(request.GroupRouteIDs) > 0 {
		// Group deliveries reuse the published business template, while their
		// transport is selected only by the approved Group Route.
		needed[domain.SceneReceiverKindInApp] = struct{}{}
	}
	for _, recipient := range allExternalRecipients {
		kind, kindErr := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		if kindErr != nil {
			return nil, apperrors.Params(kindErr.Error())
		}
		if _, subjectErr := domain.NormalizeExternalTargetSubject(string(recipient.IdentityKind), recipient.Subject); subjectErr != nil {
			return nil, apperrors.Params("第三方目标标识无效")
		}
		needed[kind] = struct{}{}
	}
	if request.SendToConfiguredConnection {
		needed[domain.SceneReceiverKindFixedConnection] = struct{}{}
	}
	if requestedInApp && len(request.Audience.UserIDs)+len(request.Audience.RoleIDs) == 0 {
		return nil, apperrors.Params("站内信接收类型需要用户受众")
	}
	if _, requested := needed[domain.SceneReceiverKindEmailUser]; requested && len(request.Audience.UserIDs)+len(request.Audience.RoleIDs) == 0 {
		return nil, apperrors.Params("邮件接收类型需要用户受众")
	}
	if len(needed) == 0 {
		return nil, apperrors.Params("新版场景通知至少需要一个接收对象")
	}
	kinds := make([]string, 0, len(needed))
	for kind := range needed {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		resolution, resolveErr := s.resolveSceneReceiver(ctx, repo, strings.TrimSpace(request.SceneCode), kind, request.OrganizationID, request.TemplateVariables)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if resolution != nil {
			plan.resolutions[kind] = resolution
			if resolution.disabled {
				plan.disabledCount++
			}
		}
	}

	if resolution := plan.resolutions[domain.SceneReceiverKindInApp]; resolution != nil {
		if !resolution.disabled {
			if requestedInApp {
				plan.effectiveAudience = facade.Audience{UserIDs: usersForReceiverKind(request, facade.ReceiverKindInApp), RoleIDs: request.Audience.RoleIDs}
			}
			plan.canonicalSubject, plan.canonicalContent = renderedSceneContent(resolution.rendered)
			plan.primaryActionLabel = resolution.rendered.PrimaryActionLabel
		}
	} else if _, requested := needed[domain.SceneReceiverKindInApp]; requested {
		return nil, sceneNotPublishedError()
	}
	if _, requested := needed[domain.SceneReceiverKindEmailUser]; requested {
		resolution := plan.resolutions[domain.SceneReceiverKindEmailUser]
		switch {
		case resolution == nil:
			return nil, sceneNotPublishedError()
		case resolution.disabled:
			// A disabled receiver does not affect other accepted receivers.
		default:
			if len(request.Audience.RoleIDs) != 0 {
				return nil, apperrors.Params("邮件通知暂不支持角色受众")
			}
			prepared, warnings, prepareErr := s.prepareVerifiedEmailRecipients(ctx, usersForReceiverKind(request, facade.ReceiverKindEmailUser), resolution.channel)
			if prepareErr != nil {
				return nil, prepareErr
			}
			plan.preparedEmails = prepared
			plan.recipientWarnings = append(plan.recipientWarnings, warnings...)
		}
	}

	for _, recipient := range allExternalRecipients {
		kind, _ := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		resolution := plan.resolutions[kind]
		switch {
		case resolution == nil:
			return nil, sceneNotPublishedError()
		case resolution.disabled:
			// A deliberately disabled published scene never falls back to caller
			// content or a caller-selected connection. It creates no target/outbox.
			continue
		default:
			if strings.TrimSpace(recipient.ConnectionRef) != "" && strings.TrimSpace(recipient.ConnectionRef) != resolution.revision.ConnectionRef {
				return nil, apperrors.Params("第三方收件人连接与已发布场景发送方式不一致")
			}
			copy := recipient
			copy.ConnectionRef = resolution.revision.ConnectionRef
			plan.externalInputs = append(plan.externalInputs, copy)
		}
	}
	if request.SendToConfiguredConnection {
		resolution := plan.resolutions[domain.SceneReceiverKindFixedConnection]
		if resolution == nil {
			return nil, sceneNotPublishedError()
		}
		if !resolution.disabled {
			plan.staticRoutes = []facade.StaticRoute{{ConnectionRef: resolution.revision.ConnectionRef}}
		}
	}
	for _, kind := range []string{domain.SceneReceiverKindFeishuWebhookGroup, domain.SceneReceiverKindWeComWebhookGroup, domain.SceneReceiverKindHTTPConnectorFixed} {
		if _, requested := needed[kind]; !requested {
			continue
		}
		resolution := plan.resolutions[kind]
		if resolution == nil {
			return nil, sceneNotPublishedError()
		}
		if !resolution.disabled {
			plan.staticRoutes = append(plan.staticRoutes, facade.StaticRoute{ConnectionRef: resolution.revision.ConnectionRef})
		}
	}
	plan.staticRoutes = uniqueStaticRoutes(plan.staticRoutes)
	groupExternal, groupStatic, groupErr := s.resolveGroupRouteDeliveries(ctx, request)
	if groupErr != nil {
		return nil, groupErr
	}
	plan.groupExternal = groupExternal
	plan.groupStatic = groupStatic

	if strings.TrimSpace(plan.canonicalSubject) == "" || strings.TrimSpace(plan.canonicalContent) == "" {
		for _, kind := range kinds {
			if resolution := plan.resolutions[kind]; resolution != nil && !resolution.disabled {
				plan.canonicalSubject, plan.canonicalContent = renderedSceneContent(resolution.rendered)
				plan.primaryActionLabel = resolution.rendered.PrimaryActionLabel
				break
			}
		}
	}
	return plan, nil
}

func (s *Service) resolveEnterprisePersonalRecipients(ctx context.Context, request facade.PublishRequest) ([]facade.ExternalRecipient, map[string]enterpriseAuthoritySnapshot, error) {
	personalKinds := make(map[facade.ReceiverKind]struct{})
	for _, kind := range request.ReceiverKinds {
		switch kind {
		case facade.ReceiverKindFeishuOpenID, facade.ReceiverKindWeComUserID:
			personalKinds[kind] = struct{}{}
		}
	}
	if len(personalKinds) == 0 {
		return nil, nil, nil
	}
	if request.OrganizationID <= 0 || len(request.Audience.UserIDs) == 0 || len(request.Audience.RoleIDs) != 0 {
		return nil, nil, apperrors.Params("企业个人通知需要明确的组织和用户受众")
	}
	if len(request.ExternalRecipients) != 0 {
		return nil, nil, apperrors.Params("企业个人通知目标只能由平台身份绑定解析")
	}
	if s.enterpriseIdentities == nil {
		return nil, nil, apperrors.Operation("企业个人通知身份解析不可用")
	}
	result := make([]facade.ExternalRecipient, 0, len(personalKinds)*len(request.Audience.UserIDs))
	authorities := make(map[string]enterpriseAuthoritySnapshot, len(personalKinds)*len(request.Audience.UserIDs))
	for kind := range personalKinds {
		provider := "FEISHU"
		identityKind := facade.ExternalIdentityFeishuOpenID
		if kind == facade.ReceiverKindWeComUserID {
			provider = "WECOM"
			identityKind = facade.ExternalIdentityWeComUserID
		}
		for _, userID := range usersForReceiverKind(request, kind) {
			if userID <= 0 {
				return nil, nil, apperrors.Params("企业个人通知用户身份无效")
			}
			resolvedKind, subject, authorityDigest, connectionConfigDigest, identityID, identityVersion, connectionID, connectionVersion, err := s.enterpriseIdentities.ResolveEnterpriseNotificationAuthority(ctx, request.OrganizationID, userID, provider, "NOTIFICATION_DIRECT")
			if err != nil {
				return nil, nil, err
			}
			if !strings.EqualFold(resolvedKind, string(identityKind)) || strings.TrimSpace(subject) == "" {
				return nil, nil, apperrors.Operation("企业个人通知身份不匹配")
			}
			result = append(result, facade.ExternalRecipient{IdentityKind: identityKind, Subject: subject})
			authorities[enterpriseAuthorityKey(string(identityKind), subject)] = enterpriseAuthoritySnapshot{organizationID: request.OrganizationID, userID: userID, externalIdentityID: identityID, identityVersion: identityVersion, enterpriseConnectionID: connectionID, connectionVersion: connectionVersion, connectionConfigDigest: connectionConfigDigest, authorityDigest: authorityDigest, grantType: "NOTIFICATION_DIRECT"}
		}
	}
	return result, authorities, nil
}

func usersForReceiverKind(request facade.PublishRequest, kind facade.ReceiverKind) []int64 {
	if len(request.UserAudiences) == 0 {
		return append([]int64(nil), request.Audience.UserIDs...)
	}
	result := make([]int64, 0, len(request.UserAudiences))
	seen := map[int64]struct{}{}
	for _, plan := range request.UserAudiences {
		if slices.Contains(plan.ReceiverKinds, kind) {
			if _, ok := seen[plan.UserID]; !ok {
				seen[plan.UserID] = struct{}{}
				result = append(result, plan.UserID)
			}
		}
	}
	return result
}

func (s *Service) applyUserAudiencePreferences(ctx context.Context, request facade.PublishRequest) (facade.PublishRequest, error) {
	preservedReceiverKinds := make([]facade.ReceiverKind, 0, len(request.ReceiverKinds))
	for _, kind := range request.ReceiverKinds {
		if kind == facade.ReceiverKindFeishuGroup || kind == facade.ReceiverKindWeComGroup {
			if !slices.Contains(preservedReceiverKinds, kind) {
				preservedReceiverKinds = append(preservedReceiverKinds, kind)
			}
		}
	}
	seen := map[int64]struct{}{}
	request.Audience.UserIDs = nil
	request.ReceiverKinds = preservedReceiverKinds
	eventFamily := strings.ToUpper(strings.TrimSpace(request.EventKey))
	switch eventFamily {
	case "WORKFORCE.TASK.ACTION_REQUIRED":
		eventFamily = "TASK_ACTION_REQUIRED"
	case "WORKFORCE.TASK.INTERRUPTED":
		eventFamily = "TASK_INTERRUPTED"
	case "WORKFORCE.TASK.COMPLETED":
		eventFamily = "TASK_COMPLETED"
	default:
		if strings.Contains(strings.ToLower(request.SceneCode), "completed") {
			eventFamily = "TASK_COMPLETED"
		} else if strings.Contains(strings.ToLower(request.SceneCode), "interrupted") {
			eventFamily = "TASK_INTERRUPTED"
		} else {
			eventFamily = "TASK_ACTION_REQUIRED"
		}
	}
	for index := range request.UserAudiences {
		plan := &request.UserAudiences[index]
		if plan.UserID <= 0 {
			return request, apperrors.Params("通知用户身份无效")
		}
		if _, exists := seen[plan.UserID]; exists {
			return request, apperrors.Params("通知用户身份重复")
		}
		seen[plan.UserID] = struct{}{}
		filtered := make([]facade.ReceiverKind, 0, len(plan.ReceiverKinds))
		for _, kind := range plan.ReceiverKinds {
			channel := mapPreferenceChannel(kind)
			if channel == "" {
				return request, apperrors.Params("通知接收类型无效")
			}
			enabled, err := s.workPreferenceEnabled(ctx, request.OrganizationID, plan.UserID, eventFamily, channel)
			if err != nil {
				return request, err
			}
			if kind == facade.ReceiverKindInApp && plan.Role == facade.UserAudienceAttention {
				enabled = true
			}
			if enabled {
				filtered = append(filtered, kind)
				if !slices.Contains(request.ReceiverKinds, kind) {
					request.ReceiverKinds = append(request.ReceiverKinds, kind)
				}
			}
		}
		plan.ReceiverKinds = filtered
		if len(filtered) > 0 {
			request.Audience.UserIDs = append(request.Audience.UserIDs, plan.UserID)
		}
	}
	return request, nil
}

func mapPreferenceChannel(kind facade.ReceiverKind) string {
	switch kind {
	case facade.ReceiverKindInApp:
		return "IN_APP"
	case facade.ReceiverKindEmailUser:
		return "EMAIL_USER"
	case facade.ReceiverKindFeishuOpenID:
		return "FEISHU_DIRECT"
	case facade.ReceiverKindWeComUserID:
		return "WECOM_DIRECT"
	default:
		return ""
	}
}

func enterpriseAuthorityKey(identityKind, subject string) string {
	return strings.ToUpper(strings.TrimSpace(identityKind)) + "\x00" + strings.TrimSpace(subject)
}

func uniqueStaticRoutes(routes []facade.StaticRoute) []facade.StaticRoute {
	if len(routes) < 2 {
		return routes
	}
	seen := make(map[string]struct{}, len(routes))
	result := make([]facade.StaticRoute, 0, len(routes))
	for _, route := range routes {
		connectionRef := strings.TrimSpace(route.ConnectionRef)
		if connectionRef == "" {
			continue
		}
		if _, exists := seen[connectionRef]; exists {
			continue
		}
		seen[connectionRef] = struct{}{}
		result = append(result, facade.StaticRoute{ConnectionRef: connectionRef})
	}
	return result
}

// resolveSceneReceiver locks one configured scene identity so template and
// connection selection cannot move midway through acceptance. A missing or
// draft-only scene is represented by nil so the caller receives the explicit
// SCENE_NOT_PUBLISHED result.
func (s *Service) resolveSceneReceiver(ctx context.Context, repo domain.SceneRevisionRepository, sceneCode, receiverKind string, organizationID int64, values map[string]any) (*scenePublishResolution, error) {
	definition, err := repo.LockSceneDefinitionByCodeAndReceiverKind(ctx, s.sceneRevisionScopeID(), enterpriseSceneCode(sceneCode, receiverKind, organizationID), receiverKind)
	if err != nil {
		return nil, err
	}
	if definition == nil || definition.CurrentPublishedRevisionID == nil {
		return nil, nil
	}
	revision, err := repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
	if err != nil {
		return nil, err
	}
	if revision == nil || revision.State != domain.SceneRevisionStatePublished {
		return nil, domain.ErrSceneRevisionConflict
	}
	templateDefinition, templateRevision, err := s.loadSceneTemplateRevision(ctx, revision.TemplateRevisionID)
	if err != nil {
		return nil, err
	}
	resolution := &scenePublishResolution{
		definition:         definition,
		revision:           revision,
		templateDefinition: templateDefinition,
		templateRevision:   templateRevision,
		receiverKind:       receiverKind,
		disabled:           !revision.Enabled,
	}
	if resolution.disabled {
		return resolution, nil
	}
	if err := s.validateSceneConnectionAtAcceptance(ctx, definition, revision); err != nil {
		return nil, err
	}
	if strings.TrimSpace(revision.ConnectionRef) != "" {
		channel, findErr := s.repo.FindChannelByCode(ctx, revision.ConnectionRef)
		if findErr != nil {
			return nil, findErr
		}
		resolution.channel = channel
	}
	variables, err := domain.TemplateVariablesFromJSON(templateRevision.VariableSchemaJSON)
	if err != nil {
		return nil, apperrors.Operation("已发布模板变量配置无效")
	}
	resolution.contentTier = domain.ContentTierForTemplateVariables(variables)
	renderValues, err := sceneTemplateValues(sceneCode, variables, values)
	if err != nil {
		return nil, err
	}
	rendered, err := domain.RenderTemplateRevision(domain.TemplateRevisionDraft{
		TemplateName:               templateDefinition.TemplateName,
		Locale:                     templateDefinition.Locale,
		SubjectTemplate:            templateRevision.SubjectTemplate,
		TextTemplate:               templateRevision.TextTemplate,
		HTMLTemplate:               templateRevision.HTMLTemplate,
		MarkdownTemplate:           templateRevision.MarkdownTemplate,
		PrimaryActionLabelTemplate: templateRevision.PrimaryActionLabelTemplate,
		Variables:                  variables,
	}, renderValues)
	if err != nil {
		return nil, apperrors.Params("场景模板变量校验或渲染失败")
	}
	resolution.rendered = rendered
	return resolution, nil
}

func enterpriseSceneCode(sceneCode, receiverKind string, organizationID int64) string {
	if organizationID > 0 && (receiverKind == domain.SceneReceiverKindFeishuOpenID || receiverKind == domain.SceneReceiverKindWeComUserID) {
		return fmt.Sprintf("org_%d_%s", organizationID, strings.TrimSpace(sceneCode))
	}
	return strings.TrimSpace(sceneCode)
}

var workforceSceneVariableAllowlist = map[string]struct{}{
	"taskName": {}, "turnNumber": {}, "statusLabel": {}, "summary": {}, "reasonLabel": {}, "occurredAt": {},
}

func sceneTemplateValues(sceneCode string, schema []domain.TemplateVariable, values map[string]any) (map[string]any, error) {
	if !strings.HasPrefix(strings.TrimSpace(sceneCode), "workforce_") {
		return values, nil
	}
	for name := range values {
		if _, allowed := workforceSceneVariableAllowlist[name]; !allowed {
			return nil, apperrors.Params("场景模板变量不在允许范围内")
		}
	}
	selected := make(map[string]any, len(schema))
	for _, variable := range schema {
		if value, found := values[variable.Name]; found {
			selected[variable.Name] = value
		}
	}
	return selected, nil
}

func (s *Service) loadSceneTemplateRevision(ctx context.Context, revisionID int64) (*domain.TemplateDefinition, *domain.TemplateRevision, error) {
	templateRepo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, nil, err
	}
	revision, err := templateRepo.FindTemplateRevisionByID(ctx, revisionID)
	if err != nil {
		return nil, nil, err
	}
	if revision == nil || (revision.State != domain.TemplateRevisionStatePublished && revision.State != domain.TemplateRevisionStateSuperseded) {
		return nil, nil, apperrors.Operation("场景引用的模板版本不可用")
	}
	definition, err := templateRepo.FindTemplateDefinitionByID(ctx, revision.TemplateDefinitionID)
	if err != nil {
		return nil, nil, err
	}
	if !s.templateDefinitionBelongsToCurrentScope(definition) {
		return nil, nil, apperrors.Operation("场景引用的模板不属于当前作用域")
	}
	return definition, revision, nil
}

func (s *Service) validateSceneConnectionAtAcceptance(ctx context.Context, definition *domain.SceneDefinition, revision *domain.SceneRevision) error {
	if definition == nil || revision == nil {
		return domain.ErrSceneRevisionNotFound
	}
	if definition.ReceiverKind == domain.SceneReceiverKindInApp {
		return domain.ValidateSceneConnection(definition.ReceiverKind, nil, revision.ConnectionRef)
	}
	channel, err := s.repo.FindChannelByCode(ctx, revision.ConnectionRef)
	if err != nil {
		return err
	}
	if channel == nil || !s.channelBelongsToCurrentScope(channel) || channel.Status != domain.ChannelStatusEnabled {
		return apperrors.Operation("场景发送方式不可用")
	}
	if err := domain.ValidateSceneConnection(definition.ReceiverKind, channel, revision.ConnectionRef); err != nil {
		return apperrors.Operation("场景发送方式与接收对象不匹配")
	}
	if err := s.validateSceneChannelReadiness(*channel, true); err != nil {
		return err
	}
	if domain.SceneConnectionDigest(channel) != revision.ConnectionDigest {
		return apperrors.Operation("场景发送方式已变更，请新建场景版本")
	}
	return nil
}

func (s *Service) insertScenePublishSnapshots(ctx context.Context, notification *domain.LogicalNotification, plan *scenePublishPlan, values map[string]any) error {
	if notification == nil || plan == nil {
		return fmt.Errorf("notification scene acceptance state is invalid")
	}
	variableDigest, err := sceneVariablesDigest(values)
	if err != nil {
		return apperrors.Params("场景模板变量格式无效")
	}
	kinds := make([]string, 0, len(plan.resolutions))
	for kind := range plan.resolutions {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	for _, kind := range kinds {
		resolution := plan.resolutions[kind]
		if resolution == nil || resolution.definition == nil || resolution.revision == nil || resolution.templateDefinition == nil || resolution.templateRevision == nil {
			continue
		}
		resolutionValue := domain.SceneSnapshotResolutionAccepted
		renderedDigest := sceneRenderedDigest(resolution.rendered)
		if resolution.disabled {
			resolutionValue = domain.SceneSnapshotResolutionDisabled
			renderedDigest = digest("scene-disabled", resolution.templateRevision.ContentDigest)
		}
		snapshotID := s.nextID()
		snapshot := &domain.SceneSnapshot{
			ID:                    snapshotID,
			NotificationID:        notification.ID,
			ScopeID:               notification.ScopeID,
			SceneCode:             resolution.definition.SceneCode,
			ReceiverKind:          resolution.receiverKind,
			SceneDefinitionID:     resolution.definition.ID,
			SceneRevisionID:       resolution.revision.ID,
			TemplateDefinitionID:  resolution.templateDefinition.ID,
			TemplateRevisionID:    resolution.templateRevision.ID,
			ConnectionRef:         resolution.revision.ConnectionRef,
			ConnectionDigest:      resolution.revision.ConnectionDigest,
			TemplateContentDigest: resolution.templateRevision.ContentDigest,
			RenderedDigest:        renderedDigest,
			VariableDigest:        variableDigest,
			Resolution:            resolutionValue,
		}
		if err := repo.InsertSceneSnapshot(ctx, snapshot); err != nil {
			return err
		}
		resolution.snapshotID = int64Ptr(snapshotID)
	}
	return nil
}

func (p *scenePublishPlan) externalRenders() map[string]sceneDeliveryRender {
	if p == nil {
		return nil
	}
	result := make(map[string]sceneDeliveryRender)
	for _, resolution := range p.resolutions {
		if resolution == nil || resolution.disabled || resolution.revision == nil || resolution.templateDefinition == nil || resolution.receiverKind == domain.SceneReceiverKindInApp || resolution.receiverKind == domain.SceneReceiverKindFixedConnection {
			continue
		}
		identityKind := strings.ToUpper(resolution.receiverKind)
		if resolution.receiverKind == domain.SceneReceiverKindEmailUser {
			identityKind = domain.ExternalIdentityEmailAddress
		}
		result[identityKind+"\x00"+strings.TrimSpace(resolution.revision.ConnectionRef)] = sceneDeliveryRender{
			SceneCode:          resolution.definition.SceneCode,
			TemplateCode:       resolution.templateDefinition.TemplateCode,
			RenderedSubject:    normalizedRenderedSubject(resolution.rendered.Subject),
			RenderedText:       resolution.rendered.Text,
			RenderedHTML:       resolution.rendered.HTML,
			RenderedMarkdown:   resolution.rendered.Markdown,
			PrimaryActionLabel: resolution.rendered.PrimaryActionLabel,
			ContentTier:        resolution.contentTier,
			SceneSnapshotID:    resolution.snapshotID,
		}
	}
	return result
}

func (p *scenePublishPlan) staticRenders() map[string]sceneDeliveryRender {
	if p == nil {
		return nil
	}
	result := make(map[string]sceneDeliveryRender)
	for _, kind := range []string{domain.SceneReceiverKindFixedConnection, domain.SceneReceiverKindFeishuWebhookGroup, domain.SceneReceiverKindWeComWebhookGroup, domain.SceneReceiverKindHTTPConnectorFixed} {
		resolution := p.resolutions[kind]
		if resolution == nil || resolution.disabled || resolution.definition == nil || resolution.templateDefinition == nil || resolution.revision == nil {
			continue
		}
		result[strings.TrimSpace(resolution.revision.ConnectionRef)] = sceneDeliveryRender{
			SceneCode: resolution.definition.SceneCode, TemplateCode: resolution.templateDefinition.TemplateCode,
			RenderedSubject: normalizedRenderedSubject(resolution.rendered.Subject), RenderedText: resolution.rendered.Text,
			RenderedHTML: resolution.rendered.HTML, RenderedMarkdown: resolution.rendered.Markdown,
			PrimaryActionLabel: resolution.rendered.PrimaryActionLabel, ContentTier: resolution.contentTier, SceneSnapshotID: resolution.snapshotID,
		}
	}
	return result
}

func sceneExternalInputKey(identityKind, connectionRef string) string {
	return strings.ToUpper(strings.TrimSpace(identityKind)) + "\x00" + strings.TrimSpace(connectionRef)
}

func renderedSceneContent(rendered domain.RenderedTemplateRevision) (string, string) {
	return normalizedRenderedSubject(rendered.Subject), choose(strings.TrimSpace(rendered.Text), strings.TrimSpace(rendered.Markdown), strings.TrimSpace(rendered.HTML))
}

func normalizedRenderedSubject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "通知"
	}
	return value
}

func (s *Service) applyExistingSceneIdempotency(ctx context.Context, existing *domain.LogicalNotification, fingerprint string, request facade.PublishRequest, receipt *facade.PublishReceipt) error {
	if existing == nil || receipt == nil || existing.RequestFingerprint != fingerprint {
		return idempotencyConflict()
	}
	snapshotsRepo, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	snapshots, err := snapshotsRepo.ListSceneSnapshotsByNotificationID(ctx, existing.ID)
	if err != nil {
		return err
	}
	byKind := make(map[string]domain.SceneSnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byKind[snapshot.ReceiverKind] = snapshot
	}
	for _, recipient := range request.ExternalRecipients {
		kind, kindErr := domain.SceneReceiverKindForExternalIdentity(string(recipient.IdentityKind))
		if kindErr != nil {
			return apperrors.Params(kindErr.Error())
		}
		if strings.TrimSpace(recipient.ConnectionRef) == "" {
			continue
		}
		if snapshot, found := byKind[kind]; found && strings.TrimSpace(snapshot.ConnectionRef) != "" && strings.TrimSpace(recipient.ConnectionRef) != strings.TrimSpace(snapshot.ConnectionRef) {
			return idempotencyConflict()
		}
	}
	incoming := &domain.LogicalNotification{RequestFingerprint: fingerprint}
	return applyIdempotentReceipt(existing, incoming, receipt)
}

func sceneNotPublishedError() error {
	return apperrors.ObjectState("场景尚未发布").WithDetails(map[string]string{"reasonCode": "SCENE_NOT_PUBLISHED"})
}

func sceneDisabledError() error {
	return apperrors.ObjectState("场景已停用").WithDetails(map[string]string{"reasonCode": "SCENE_DISABLED"})
}

func scenePublishFingerprint(request facade.PublishRequest, scopeID string) (string, error) {
	audience, err := domain.NormalizeOptionalAudience(request.Audience.UserIDs, request.Audience.RoleIDs)
	if err != nil {
		return "", apperrors.Params(err.Error())
	}
	type target struct {
		IdentityKind       string `json:"identityKind"`
		SubjectDigest      string `json:"subjectDigest"`
		ProviderParamsJSON string `json:"providerParamsJson"`
	}
	targets := make([]target, 0, len(request.ExternalRecipients))
	for _, recipient := range request.ExternalRecipients {
		identityKind := strings.ToUpper(strings.TrimSpace(string(recipient.IdentityKind)))
		subject, normalizeErr := domain.NormalizeExternalTargetSubject(identityKind, recipient.Subject)
		if normalizeErr != nil {
			return "", apperrors.Params("第三方目标标识无效")
		}
		params, encodeErr := json.Marshal(recipient.ProviderParams)
		if encodeErr != nil {
			return "", apperrors.Params("第三方可选参数格式无效")
		}
		subjectSum := sha256.Sum256([]byte(identityKind + "\x00" + subject))
		targets = append(targets, target{IdentityKind: identityKind, SubjectDigest: hex.EncodeToString(subjectSum[:]), ProviderParamsJSON: string(params)})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].IdentityKind != targets[j].IdentityKind {
			return targets[i].IdentityKind < targets[j].IdentityKind
		}
		if targets[i].SubjectDigest != targets[j].SubjectDigest {
			return targets[i].SubjectDigest < targets[j].SubjectDigest
		}
		return targets[i].ProviderParamsJSON < targets[j].ProviderParamsJSON
	})
	variableDigest, err := sceneVariablesDigest(request.TemplateVariables)
	if err != nil {
		return "", apperrors.Params("场景模板变量格式无效")
	}
	canonical := struct {
		ScopeID                    string                    `json:"scopeId"`
		OrganizationID             int64                     `json:"organizationId,string,omitempty"`
		EventKey                   string                    `json:"eventKey"`
		IdempotencyKey             string                    `json:"idempotencyKey"`
		SceneCode                  string                    `json:"sceneCode"`
		Audience                   domain.AudienceSnapshot   `json:"audience"`
		Targets                    []target                  `json:"targets"`
		SendToConfiguredConnection bool                      `json:"sendToConfiguredConnection"`
		ReceiverKinds              []string                  `json:"receiverKinds"`
		GroupRouteIDs              []int64                   `json:"groupRouteIds"`
		MentionUserIDs             []int64                   `json:"mentionUserIds"`
		UserAudiences              []facade.UserAudiencePlan `json:"userAudiences"`
		TemplateVariablesDigest    string                    `json:"templateVariablesDigest"`
		Category                   string                    `json:"category"`
		Priority                   string                    `json:"priority"`
		Mandatory                  bool                      `json:"mandatory"`
		DeepLink                   string                    `json:"deepLink"`
		ScheduleAt                 string                    `json:"scheduleAt"`
		ExpiresAt                  string                    `json:"expiresAt"`
	}{
		ScopeID: scopeID, OrganizationID: request.OrganizationID, EventKey: strings.TrimSpace(request.EventKey), IdempotencyKey: strings.TrimSpace(request.IdempotencyKey), SceneCode: strings.TrimSpace(request.SceneCode), Audience: audience,
		Targets: targets, SendToConfiguredConnection: request.SendToConfiguredConnection, TemplateVariablesDigest: variableDigest, UserAudiences: append([]facade.UserAudiencePlan(nil), request.UserAudiences...),
		Category: strings.ToUpper(defaultNotificationCategory(request.Category)), Priority: strings.ToUpper(defaultNotificationPriority(request.Priority)), Mandatory: request.Mandatory,
		DeepLink: strings.TrimSpace(request.DeepLink),
	}
	for _, kind := range request.ReceiverKinds {
		canonical.ReceiverKinds = append(canonical.ReceiverKinds, strings.ToUpper(strings.TrimSpace(string(kind))))
	}
	sort.Strings(canonical.ReceiverKinds)
	canonical.GroupRouteIDs = append([]int64(nil), request.GroupRouteIDs...)
	canonical.MentionUserIDs = append([]int64(nil), request.MentionUserIDs...)
	sort.Slice(canonical.GroupRouteIDs, func(i, j int) bool { return canonical.GroupRouteIDs[i] < canonical.GroupRouteIDs[j] })
	sort.Slice(canonical.MentionUserIDs, func(i, j int) bool { return canonical.MentionUserIDs[i] < canonical.MentionUserIDs[j] })
	if request.ScheduleAt != nil {
		canonical.ScheduleAt = request.ScheduleAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	if request.ExpiresAt != nil {
		canonical.ExpiresAt = request.ExpiresAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sceneVariablesDigest(values map[string]any) (string, error) {
	if values == nil {
		values = map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func sceneRenderedDigest(rendered domain.RenderedTemplateRevision) string {
	return digest(rendered.Subject, rendered.Text, rendered.HTML, rendered.Markdown, rendered.PrimaryActionLabel)
}
