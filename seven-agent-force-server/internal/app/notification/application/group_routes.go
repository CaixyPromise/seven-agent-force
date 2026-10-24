package application

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

type groupRouteReader interface {
	ListEnabledGroupRoutes(context.Context, int64, int64, string) ([]domain.GroupRoute, error)
}

type publicationStatusReader interface {
	ResolvePublicationStatus(context.Context, int64, string, int64) ([]domain.PublicationChannelStatus, error)
}

type groupRouteAcceptanceRepository interface {
	FindGroupRoutesByIDs(context.Context, int64, []int64) ([]domain.GroupRoute, error)
	FindChannelByID(context.Context, int64) (*domain.Channel, error)
}

type groupRouteManagementRepository interface {
	groupRouteAcceptanceRepository
	ListGroupRoutes(context.Context, int64) ([]domain.GroupRoute, error)
	FindGroupRouteByID(context.Context, int64, int64) (*domain.GroupRoute, error)
	InsertGroupRoute(context.Context, domain.GroupRoute) error
	UpdateGroupRoute(context.Context, domain.GroupRoute, int64) (bool, error)
	SetChannelOrganizationOwnership(context.Context, int64, int64) error
	GroupRouteImpact(context.Context, int64, int64) (int64, int64, int64, int64, int64, int64, error)
	DeleteGroupRoute(context.Context, int64, int64) (bool, error)
	InsertGroupRouteCandidate(context.Context, domain.GroupRouteCandidate) error
	ConsumeGroupRouteCandidate(context.Context, int64, string, time.Time) (*domain.GroupRouteCandidate, error)
}

func (s *Service) DiscoverFeishuGroupCandidates(ctx context.Context, organizationID, connectionID int64) ([]facade.GroupCandidate, error) {
	resolver, ok := s.enterpriseIdentities.(EnterpriseGroupDiscoveryResolver)
	if !ok || s.secrets == nil || s.digester == nil {
		return nil, apperrors.Operation("飞书应用群发现暂不可用")
	}
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	groups, err := resolver.DiscoverEnterpriseGroups(ctx, organizationID, connectionID)
	if err != nil {
		return nil, err
	}
	result := make([]facade.GroupCandidate, 0, len(groups))
	for _, group := range groups {
		raw := make([]byte, 32)
		if _, err = rand.Read(raw); err != nil {
			return nil, err
		}
		token := base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(token))
		encrypted, encErr := s.secrets.EncryptString(ctx, group.Target)
		if encErr != nil {
			return nil, encErr
		}
		digest, _, digestErr := s.digester.Digest(ctx, "", s.scopeID, fmt.Sprintf("enterprise-connection-%d", group.ConnectionID), domain.ExternalIdentityFeishuChatID, group.Target)
		if digestErr != nil {
			return nil, digestErr
		}
		now := s.now().UTC()
		item := domain.GroupRouteCandidate{ID: s.nextID(), TokenHash: hex.EncodeToString(sum[:]), OrganizationID: organizationID, ConnectionID: group.ConnectionID, ConnectionVersion: group.ConnectionVersion, ConnectionConfigDigest: group.ConnectionConfigDigest, DisplayName: group.DisplayName, Description: group.Description, TargetCiphertext: encrypted.CiphertextB64, TargetEDEK: encrypted.EDEKB64, TargetWrapKeyRef: encrypted.WrapKeyRef, TargetDigest: digest, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now}
		if err = repo.InsertGroupRouteCandidate(ctx, item); err != nil {
			return nil, err
		}
		result = append(result, facade.GroupCandidate{CandidateToken: token, DisplayName: group.DisplayName, Description: group.Description})
	}
	return result, nil
}

func (s *Service) ListGroupRoutes(ctx context.Context, organizationID int64) ([]facade.GroupRouteRecord, error) {
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	items, err := repo.ListGroupRoutes(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	result := make([]facade.GroupRouteRecord, 0, len(items))
	for _, item := range items {
		impact, _ := s.GetGroupRouteImpact(ctx, organizationID, item.ID)
		result = append(result, mapGroupRouteRecord(item, impact))
	}
	return result, nil
}

func (s *Service) GetGroupRoute(ctx context.Context, organizationID, routeID int64) (*facade.GroupRouteRecord, error) {
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	item, err := repo.FindGroupRouteByID(ctx, organizationID, routeID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, domain.ErrGroupRouteNotFound
	}
	impact, _ := s.GetGroupRouteImpact(ctx, organizationID, routeID)
	mapped := mapGroupRouteRecord(*item, impact)
	return &mapped, nil
}

func (s *Service) SaveGroupRoute(ctx context.Context, organizationID, actorID int64, req facade.GroupRouteSaveRequest) (*facade.GroupRouteRecord, error) {
	if organizationID <= 0 || actorID <= 0 {
		return nil, domain.ErrGroupRouteInvalid
	}
	provider := domain.GroupRouteProvider(strings.ToUpper(strings.TrimSpace(req.Provider)))
	transport := domain.GroupRouteTransport(strings.ToUpper(strings.TrimSpace(req.Transport)))
	if !provider.Valid() || !transport.Valid() {
		return nil, domain.ErrGroupRouteInvalid
	}
	permission := strings.TrimSpace(req.RequiredPermissionCode)
	if permission == "" {
		permission = "workforce:task:create"
	}
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	var saved domain.GroupRoute
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		var existing *domain.GroupRoute
		if req.ID > 0 {
			existing, err = repo.FindGroupRouteByID(txCtx, organizationID, req.ID)
			if err != nil {
				return err
			}
			if existing == nil {
				return domain.ErrGroupRouteNotFound
			}
			if req.ExpectedVersion != existing.RouteVersion {
				return domain.ErrGroupRouteDrift
			}
			if existing.Transport != transport || existing.Provider != provider {
				return domain.ErrGroupRouteInvalid
			}
		}
		routeID := req.ID
		if routeID <= 0 {
			routeID = s.nextID()
		}
		channelID := int64(0)
		if existing != nil {
			channelID = existing.DeliveryChannelID
		} else {
			channelID = s.nextID()
		}
		channelType := domain.ChannelTypeFeishuWebhook
		if transport == domain.GroupRouteTransportWeComWebhook {
			channelType = domain.ChannelTypeWeComWebhook
		} else if transport == domain.GroupRouteTransportFeishuAppChat {
			channelType = domain.ChannelTypeFeishuApp
		}
		channelCode := fmt.Sprintf("group-route-%d", routeID)
		channelReq := facade.ChannelUpsertRequest{ID: channelID, ChannelCode: channelCode, ChannelName: strings.TrimSpace(req.Name), ChannelType: channelType, Status: domain.ChannelStatusEnabled, Priority: 100, WebhookProfileConfig: &facade.WebhookProfileConfig{TimeoutMilliseconds: 5000, SuccessStatusCodes: []int{200}}, WebhookURL: strings.TrimSpace(req.WebhookURL), WebhookSigningSecret: strings.TrimSpace(req.WebhookSigningSecret)}
		var candidate *domain.GroupRouteCandidate
		if transport == domain.GroupRouteTransportFeishuAppChat {
			if strings.TrimSpace(req.CandidateToken) == "" || req.EnterpriseConnectionID <= 0 {
				return domain.ErrGroupRouteTargetInvalid
			}
			decodedToken, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimSpace(req.CandidateToken))
			if decodeErr != nil || len(decodedToken) != 32 {
				return domain.ErrGroupRouteDiscoveryTampered
			}
			sum := sha256.Sum256([]byte(strings.TrimSpace(req.CandidateToken)))
			candidate, err = repo.ConsumeGroupRouteCandidate(txCtx, organizationID, hex.EncodeToString(sum[:]), s.now().UTC())
			if err != nil {
				return err
			}
			if candidate == nil || candidate.ConnectionID != req.EnterpriseConnectionID {
				return domain.ErrGroupRouteDiscoveryExpired
			}
			organization, connection := organizationID, candidate.ConnectionID
			appChannel := &domain.Channel{ID: channelID, ChannelCode: channelCode, ChannelName: candidate.DisplayName, ChannelType: domain.ChannelTypeFeishuApp, ScopeID: s.scopeID, ScopeType: "ORGANIZATION", OrganizationID: &organization, EnterpriseConnectionID: &connection, EnterpriseConnectionVersion: candidate.ConnectionVersion, EnterpriseConnectionDigest: candidate.ConnectionConfigDigest, Status: domain.ChannelStatusEnabled, Priority: 100, ConfigJSON: `{}`, RateLimitJSON: `{}`, MetadataJSON: `{"credentialOwner":"ENTERPRISE_IDENTITY","routeType":"GROUP"}`, CreatorID: int64Ptr(actorID), UpdaterID: int64Ptr(actorID)}
			if existing != nil {
				existingChannel, findErr := repo.FindChannelByID(txCtx, channelID)
				if findErr != nil {
					return findErr
				}
				if existingChannel != nil {
					appChannel.CreatorID = existingChannel.CreatorID
				}
			}
			if saveErr := s.repo.UpsertChannel(txCtx, appChannel); saveErr != nil {
				return saveErr
			}
		}
		if transport != domain.GroupRouteTransportFeishuAppChat {
			channel, saveErr := s.UpsertChannel(txCtx, channelReq, actorID)
			if saveErr != nil {
				return saveErr
			}
			channelID = channel.ID
		}
		if ownErr := repo.SetChannelOrganizationOwnership(txCtx, channelID, organizationID); ownErr != nil {
			return ownErr
		}
		now := s.now().UTC()
		item := domain.GroupRoute{ID: routeID, OrganizationID: organizationID, DisplayName: strings.TrimSpace(req.Name), Provider: provider, Transport: transport, DeliveryChannelID: channelID, RequiredPermissionCode: permission, Status: domain.GroupRouteDraft, RouteVersion: 1, ChannelVersion: 1, Capabilities: []string{"TASK_ACTION_REQUIRED", "TASK_COMPLETED", "MENTION", "PRIMARY_ACTION"}, LastProbeStatus: "NOT_PROBED", CreatorID: actorID, UpdaterID: actorID, CreatedAt: now, UpdatedAt: now}
		if existing != nil {
			item = *existing
			item.DisplayName = strings.TrimSpace(req.Name)
			item.RequiredPermissionCode = permission
			item.Status = domain.GroupRouteDraft
			item.LastProbeStatus = "NOT_PROBED"
			item.LastProbeReasonCode = ""
			item.LastProbeAt = nil
			item.RouteVersion = existing.RouteVersion + 1
			item.ChannelVersion = existing.ChannelVersion + 1
			item.UpdaterID = actorID
			item.UpdatedAt = now
		}
		if candidate != nil {
			item.DisplayName = candidate.DisplayName
			item.TargetCiphertext = candidate.TargetCiphertext
			item.TargetEDEK = candidate.TargetEDEK
			item.TargetWrapKeyRef = candidate.TargetWrapKeyRef
			item.TargetDigest = candidate.TargetDigest
			item.ConnectionVersion = candidate.ConnectionVersion
		}
		item.ConfigDigest = domain.GroupRouteConfigDigest(item)
		if validateErr := item.Validate(); validateErr != nil {
			return validateErr
		}
		if existing == nil {
			if insertErr := repo.InsertGroupRoute(txCtx, item); insertErr != nil {
				return insertErr
			}
		} else {
			ok, updateErr := repo.UpdateGroupRoute(txCtx, item, existing.RouteVersion)
			if updateErr != nil {
				return updateErr
			}
			if !ok {
				return domain.ErrGroupRouteDrift
			}
		}
		saved = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	mapped := mapGroupRouteRecord(saved, nil)
	return &mapped, nil
}

func (s *Service) ProbeGroupRoute(ctx context.Context, organizationID, actorID, routeID, expectedVersion int64) (*facade.GroupRouteRecord, error) {
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	item, err := repo.FindGroupRouteByID(ctx, organizationID, routeID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, domain.ErrGroupRouteNotFound
	}
	if item.RouteVersion != expectedVersion {
		return nil, domain.ErrGroupRouteDrift
	}
	channel, err := repo.FindChannelByID(ctx, item.DeliveryChannelID)
	if err != nil {
		return nil, err
	}
	passed := false
	reason := "PROBE_FAILED"
	if channel != nil {
		if item.Transport == domain.GroupRouteTransportFeishuAppChat && s.secrets != nil {
			target, decryptErr := s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{CiphertextB64: item.TargetCiphertext, EDEKB64: item.TargetEDEK, WrapKeyRef: item.TargetWrapKeyRef})
			if decryptErr == nil {
				enterpriseResult, enterpriseErr := s.TestEnterpriseConnection(ctx, facade.EnterpriseConnectionTestRequest{ConnectionRef: channel.ChannelCode, IdentityKind: facade.ExternalIdentityFeishuChatID, Subject: target, Text: "Seven Agent Force 通知群连接测试"})
				if enterpriseErr == nil && enterpriseResult != nil && enterpriseResult.Status == domain.DriverResultProviderAccepted {
					passed = true
					reason = ""
				} else {
					reason = groupRouteProbeReason(enterpriseResultStatus(enterpriseResult), enterpriseResultFailureClass(enterpriseResult), enterpriseResultDiagnostic(enterpriseResult), enterpriseResultProviderError(enterpriseResult), enterpriseErr)
				}
			} else {
				reason = "GROUP_TARGET_DECRYPT_FAILED"
			}
		} else {
			result, probeErr := s.TestStaticConnection(ctx, facade.StaticConnectionTestRequest{ConnectionRef: channel.ChannelCode, Text: "Seven Agent Force 通知群连接测试"})
			if probeErr == nil && result != nil && result.Status == domain.DriverResultProviderAccepted {
				passed = true
				reason = ""
			} else if result != nil {
				reason = groupRouteProbeReason(result.Status, result.FailureClass, result.Diagnostic, result.ProviderError, probeErr)
			} else {
				reason = groupRouteProbeReason("", "", "", nil, probeErr)
			}
		}
	}
	previous := item.RouteVersion
	item.MarkProbe(passed, reason, s.now())
	item.UpdaterID = actorID
	item.ConfigDigest = domain.GroupRouteConfigDigest(*item)
	ok, err := repo.UpdateGroupRoute(ctx, *item, previous)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrGroupRouteDrift
	}
	mapped := mapGroupRouteRecord(*item, nil)
	return &mapped, nil
}

func enterpriseResultStatus(result *facade.EnterpriseConnectionTestResult) string {
	if result == nil {
		return ""
	}
	return result.Status
}

func enterpriseResultFailureClass(result *facade.EnterpriseConnectionTestResult) string {
	if result == nil {
		return ""
	}
	return result.FailureClass
}

func enterpriseResultDiagnostic(result *facade.EnterpriseConnectionTestResult) string {
	if result == nil {
		return ""
	}
	return result.Diagnostic
}

func enterpriseResultProviderError(result *facade.EnterpriseConnectionTestResult) *facade.ProviderError {
	if result == nil {
		return nil
	}
	return result.ProviderError
}

// groupRouteProbeReason deliberately persists only bounded diagnostic codes.
// Raw provider messages, targets and credentials never enter the route record.
func groupRouteProbeReason(status, failureClass, diagnostic string, providerError *facade.ProviderError, probeErr error) string {
	if probeErr != nil {
		return "PROBE_REQUEST_FAILED"
	}
	if providerError != nil {
		switch strings.TrimSpace(providerError.Code) {
		case "230001", "230002", "230003":
			return "FEISHU_GROUP_UNAVAILABLE"
		}
	}
	switch strings.ToUpper(strings.TrimSpace(failureClass)) {
	case "INVALID_TARGET":
		return "GROUP_TARGET_INVALID"
	case "AUTHENTICATION":
		return "CONNECTION_AUTHENTICATION_FAILED"
	case "THROTTLED":
		return "PROVIDER_THROTTLED"
	}
	if strings.EqualFold(strings.TrimSpace(status), domain.DriverResultUnknown) || strings.Contains(strings.ToUpper(strings.TrimSpace(diagnostic)), "UNCONFIRMED") {
		return "PROVIDER_OUTCOME_UNKNOWN"
	}
	return "PROBE_FAILED"
}

func (s *Service) SetGroupRouteState(ctx context.Context, organizationID, actorID, routeID int64, req facade.GroupRouteStateRequest, action string) (*facade.GroupRouteRecord, error) {
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	item, err := repo.FindGroupRouteByID(ctx, organizationID, routeID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, domain.ErrGroupRouteNotFound
	}
	if item.RouteVersion != req.ExpectedVersion {
		return nil, domain.ErrGroupRouteDrift
	}
	previous := item.RouteVersion
	switch strings.ToUpper(action) {
	case "ENABLE":
		err = item.Enable(s.now())
	case "DISABLE":
		err = item.Disable(s.now())
	case "RETIRE":
		item.Retire(s.now())
	default:
		err = domain.ErrGroupRouteInvalid
	}
	if err != nil {
		return nil, err
	}
	item.UpdaterID = actorID
	ok, err := repo.UpdateGroupRoute(ctx, *item, previous)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrGroupRouteDrift
	}
	mapped := mapGroupRouteRecord(*item, nil)
	return &mapped, nil
}

func (s *Service) GetGroupRouteImpact(ctx context.Context, organizationID, routeID int64) (*facade.GroupRouteImpact, error) {
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return nil, err
	}
	a, b, c, d, e, f, err := repo.GroupRouteImpact(ctx, organizationID, routeID)
	if err != nil {
		return nil, err
	}
	return &facade.GroupRouteImpact{ActiveTasks: a, ResumableTasks: b, Subscriptions: c, PendingRecords: d, PendingDeliveries: e, HistoricalReferences: f, CanDelete: a+b+c+d+e+f == 0}, nil
}

func (s *Service) DeleteGroupRoute(ctx context.Context, organizationID, routeID int64) error {
	impact, err := s.GetGroupRouteImpact(ctx, organizationID, routeID)
	if err != nil {
		return err
	}
	if !impact.CanDelete {
		return domain.ErrGroupRouteInUse
	}
	repo, err := s.groupRouteManagementRepository()
	if err != nil {
		return err
	}
	ok := false
	err = s.withinTx(ctx, func(txCtx context.Context) error {
		var deleteErr error
		ok, deleteErr = repo.DeleteGroupRoute(txCtx, organizationID, routeID)
		return deleteErr
	})
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrGroupRouteInvalid
	}
	return nil
}

func (s *Service) groupRouteManagementRepository() (groupRouteManagementRepository, error) {
	repo, ok := s.repo.(groupRouteManagementRepository)
	if !ok {
		return nil, fmt.Errorf("notification group route management repository is not configured")
	}
	return repo, nil
}

func mapGroupRouteRecord(item domain.GroupRoute, impact *facade.GroupRouteImpact) facade.GroupRouteRecord {
	count := int64(0)
	if impact != nil {
		count = impact.Subscriptions
	}
	return facade.GroupRouteRecord{ID: item.ID, Name: item.DisplayName, Provider: string(item.Provider), Transport: string(item.Transport), RequiredPermissionCode: item.RequiredPermissionCode, Status: string(item.Status), Version: item.RouteVersion, LastProbeStatus: item.LastProbeStatus, LastProbeReasonCode: item.LastProbeReasonCode, LastProbeAt: item.LastProbeAt, ReferencedTaskCount: count, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func (s *Service) resolveGroupRouteDeliveries(ctx context.Context, request facade.PublishRequest) ([]preparedExternalRecipient, []preparedStaticRoute, error) {
	if len(request.GroupRouteIDs) == 0 {
		if hasGroupReceiverKind(request.ReceiverKinds) {
			return nil, nil, apperrors.Params(domain.ErrGroupRouteInvalid.Error())
		}
		return nil, nil, nil
	}
	if request.OrganizationID <= 0 || len(request.GroupRouteIDs) > 3 {
		return nil, nil, apperrors.Params(domain.ErrGroupRouteLimitExceeded.Error())
	}
	if len(request.MentionUserIDs) > 1 {
		return nil, nil, apperrors.Params(domain.ErrMentionTargetInvalid.Error())
	}
	seen := make(map[int64]struct{}, len(request.GroupRouteIDs))
	ids := append([]int64(nil), request.GroupRouteIDs...)
	for _, id := range ids {
		if id <= 0 {
			return nil, nil, apperrors.Params(domain.ErrGroupRouteInvalid.Error())
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, nil, apperrors.Params(domain.ErrGroupRouteInvalid.Error())
		}
		seen[id] = struct{}{}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	repo, ok := s.repo.(groupRouteAcceptanceRepository)
	if !ok {
		return nil, nil, fmt.Errorf("notification group route repository is not configured")
	}
	routes, err := repo.FindGroupRoutesByIDs(ctx, request.OrganizationID, ids)
	if err != nil {
		return nil, nil, err
	}
	if len(routes) != len(ids) {
		return nil, nil, apperrors.ObjectState("组织通知群不存在或不属于当前组织").WithDetails(map[string]string{"reasonCode": domain.ErrGroupRouteNotFound.Error()})
	}
	mentionUserID := int64(0)
	if len(request.MentionUserIDs) == 1 {
		mentionUserID = request.MentionUserIDs[0]
		if mentionUserID <= 0 {
			return nil, nil, apperrors.Params(domain.ErrMentionTargetInvalid.Error())
		}
	}
	var external []preparedExternalRecipient
	var static []preparedStaticRoute
	for _, route := range routes {
		if route.OrganizationID != request.OrganizationID {
			return nil, nil, apperrors.PermissionDenied(domain.ErrGroupRouteCrossOrganization.Error())
		}
		if route.Status != domain.GroupRouteEnabled || route.ConfigDigest != domain.GroupRouteConfigDigest(route) {
			return nil, nil, apperrors.ObjectState("组织通知群当前不可用").WithDetails(map[string]string{"reasonCode": domain.ErrGroupRouteNotEnabled.Error()})
		}
		if !requestAllowsGroupProvider(request.ReceiverKinds, route.Provider) {
			return nil, nil, apperrors.Params(domain.ErrMentionProviderMismatch.Error())
		}
		channel, findErr := repo.FindChannelByID(ctx, route.DeliveryChannelID)
		if findErr != nil {
			return nil, nil, findErr
		}
		if channel == nil || channel.Status != domain.ChannelStatusEnabled || !s.channelBelongsToCurrentScope(channel) || channel.ID != route.DeliveryChannelID {
			return nil, nil, apperrors.ObjectState("组织通知群发送连接不可用").WithDetails(map[string]string{"reasonCode": domain.ErrGroupRouteDrift.Error()})
		}
		if channel.OrganizationID == nil || *channel.OrganizationID != request.OrganizationID {
			return nil, nil, apperrors.PermissionDenied(domain.ErrGroupRouteCrossOrganization.Error())
		}
		metadata := &preparedGroupRouteMetadata{route: route, mentionUserID: mentionUserID}
		if mentionUserID > 0 {
			provider := string(route.Provider)
			if s.enterpriseIdentities == nil {
				metadata.warningCode = domain.ErrMentionIdentityNotAvailable.Error()
			} else {
				kind, _, authorityDigest, connectionDigest, identityID, identityVersion, connectionID, connectionVersion, authorityErr := s.enterpriseIdentities.ResolveEnterpriseNotificationAuthority(ctx, request.OrganizationID, mentionUserID, provider, "NOTIFICATION_MENTION")
				if authorityErr != nil || strings.TrimSpace(kind) == "" || strings.TrimSpace(authorityDigest) == "" {
					metadata.warningCode = domain.ErrMentionIdentityNotAvailable.Error()
				} else {
					metadata.mentionAuthority = &enterpriseAuthoritySnapshot{organizationID: request.OrganizationID, userID: mentionUserID, externalIdentityID: identityID, identityVersion: identityVersion, enterpriseConnectionID: connectionID, connectionVersion: connectionVersion, connectionConfigDigest: connectionDigest, authorityDigest: authorityDigest, grantType: "NOTIFICATION_MENTION"}
				}
			}
		}
		switch route.Transport {
		case domain.GroupRouteTransportFeishuAppChat:
			if channel.ChannelType != domain.ChannelTypeFeishuApp || s.secrets == nil || s.digester == nil {
				return nil, nil, apperrors.Operation(domain.ErrGroupRouteDrift.Error())
			}
			target, decryptErr := s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{CiphertextB64: route.TargetCiphertext, EDEKB64: route.TargetEDEK, WrapKeyRef: route.TargetWrapKeyRef})
			if decryptErr != nil || strings.TrimSpace(target) == "" {
				return nil, nil, apperrors.Operation(domain.ErrGroupRouteTargetInvalid.Error())
			}
			targetDigest, targetDigestKeyRef, digestErr := s.digester.Digest(ctx, "", s.scopeID, channel.ChannelCode, domain.ExternalIdentityFeishuChatID, target)
			if digestErr != nil {
				return nil, nil, digestErr
			}
			external = append(external, preparedExternalRecipient{channel: *channel, identityKind: domain.ExternalIdentityFeishuChatID, subject: target, subjectDigest: targetDigest, subjectDigestKeyRef: targetDigestKeyRef, providerParams: map[string]any{}, providedParams: map[string]any{}, providerParamsJSON: "{}", authority: &enterpriseAuthoritySnapshot{organizationID: request.OrganizationID, enterpriseConnectionID: valueOrZero(channel.EnterpriseConnectionID), connectionVersion: route.ConnectionVersion, connectionConfigDigest: channel.EnterpriseConnectionDigest, grantType: "GROUP_ROUTE"}, groupRoute: metadata})
		case domain.GroupRouteTransportFeishuWebhook:
			if channel.ChannelType != domain.ChannelTypeFeishuWebhook {
				return nil, nil, apperrors.Operation(domain.ErrGroupRouteDrift.Error())
			}
			static = append(static, preparedStaticRoute{channel: *channel, groupRoute: metadata})
		case domain.GroupRouteTransportWeComWebhook:
			if channel.ChannelType != domain.ChannelTypeWeComWebhook {
				return nil, nil, apperrors.Operation(domain.ErrGroupRouteDrift.Error())
			}
			static = append(static, preparedStaticRoute{channel: *channel, groupRoute: metadata})
		default:
			return nil, nil, apperrors.Params(domain.ErrGroupRouteUnsupported.Error())
		}
	}
	return external, static, nil
}

func hasGroupReceiverKind(kinds []facade.ReceiverKind) bool {
	for _, kind := range kinds {
		if kind == facade.ReceiverKindFeishuGroup || kind == facade.ReceiverKindWeComGroup {
			return true
		}
	}
	return false
}

func requestAllowsGroupProvider(kinds []facade.ReceiverKind, provider domain.GroupRouteProvider) bool {
	want := facade.ReceiverKindFeishuGroup
	if provider == domain.GroupRouteProviderWeCom {
		want = facade.ReceiverKindWeComGroup
	}
	for _, kind := range kinds {
		if kind == want {
			return true
		}
	}
	return false
}

func valueOrZero(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func (s *Service) newGroupRouteSnapshot(notification *domain.LogicalNotification, delivery domain.Delivery, metadata preparedGroupRouteMetadata) domain.GroupRouteSnapshot {
	route := metadata.route
	item := domain.GroupRouteSnapshot{ID: s.nextID(), NotificationIdentity: notification.NotificationID, DeliveryIdentity: delivery.DeliveryID, RouteID: route.ID, OrganizationID: route.OrganizationID, RouteVersion: route.RouteVersion, Provider: route.Provider, Transport: route.Transport, RouteConfigDigest: route.ConfigDigest, DeliveryChannelID: route.DeliveryChannelID, ChannelVersion: route.ChannelVersion, EnterpriseConnectionVersion: route.ConnectionVersion, TargetCiphertext: route.TargetCiphertext, TargetEDEK: route.TargetEDEK, TargetWrapKeyRef: route.TargetWrapKeyRef, TargetDigest: route.TargetDigest, Presentation: delivery.Presentation, ActionURLSnapshot: delivery.ActionURLSnapshot, MentionRequested: metadata.mentionUserID > 0, MentionUserID: metadata.mentionUserID, WarningCode: metadata.warningCode, RenderedPayloadDigest: delivery.RenderedPayloadDigest, CreatedAt: s.now().UTC()}
	if metadata.mentionAuthority != nil {
		item.MentionIdentityID = metadata.mentionAuthority.externalIdentityID
		item.MentionIdentityVersion = metadata.mentionAuthority.identityVersion
		item.MentionAuthorityDigest = metadata.mentionAuthority.authorityDigest
	}
	return item
}

func (s *Service) resolveGroupRouteDispatch(ctx context.Context, delivery *domain.Delivery, channel *domain.Channel) (*domain.DriverMention, bool, error) {
	if delivery == nil {
		return nil, false, nil
	}
	repo, ok := s.repo.(groupRouteSnapshotRepository)
	if !ok {
		return nil, false, nil
	}
	snapshot, err := repo.FindGroupRouteSnapshotByDeliveryID(ctx, delivery.DeliveryID)
	if err != nil || snapshot == nil {
		return nil, false, err
	}
	acceptance, ok := s.repo.(groupRouteAcceptanceRepository)
	if !ok {
		return nil, true, fmt.Errorf("notification group route repository is not configured")
	}
	routes, err := acceptance.FindGroupRoutesByIDs(ctx, snapshot.OrganizationID, []int64{snapshot.RouteID})
	if err != nil {
		return nil, true, err
	}
	// A route rotation only affects deliveries accepted after the rotation. Existing
	// deliveries keep using their immutable route/channel/target snapshot, but a
	// route that is explicitly disabled or retired fences all future provider calls.
	if len(routes) != 1 || routes[0].Status != domain.GroupRouteEnabled || channel == nil || channel.ID != snapshot.DeliveryChannelID || channel.Status != domain.ChannelStatusEnabled {
		return nil, true, nil
	}
	if !snapshot.MentionRequested || snapshot.WarningCode == domain.ErrMentionIdentityNotAvailable.Error() || snapshot.MentionUserID <= 0 {
		return nil, false, nil
	}
	if s.enterpriseIdentities == nil {
		_ = repo.UpdateGroupRouteSnapshotWarning(ctx, delivery.DeliveryID, domain.ErrMentionIdentityNotAvailable.Error())
		return nil, false, nil
	}
	kind, subject, authorityDigest, _, identityID, identityVersion, _, _, authorityErr := s.enterpriseIdentities.ResolveEnterpriseNotificationAuthority(ctx, snapshot.OrganizationID, snapshot.MentionUserID, string(snapshot.Provider), "NOTIFICATION_MENTION")
	expectedKind := domain.ExternalIdentityFeishuOpenID
	if snapshot.Provider == domain.GroupRouteProviderWeCom {
		expectedKind = domain.ExternalIdentityWeComUserID
	}
	if authorityErr != nil || !strings.EqualFold(kind, expectedKind) || identityID != snapshot.MentionIdentityID || identityVersion != snapshot.MentionIdentityVersion || authorityDigest != snapshot.MentionAuthorityDigest || strings.TrimSpace(subject) == "" {
		_ = repo.UpdateGroupRouteSnapshotWarning(ctx, delivery.DeliveryID, domain.ErrMentionIdentityNotAvailable.Error())
		return nil, false, nil
	}
	return &domain.DriverMention{Provider: string(snapshot.Provider), Subject: subject, Label: "相关处理人"}, false, nil
}

func (s *Service) ListAvailableGroupRoutes(ctx context.Context, organizationID, userID int64, sceneCode string) ([]facade.GroupRouteOption, error) {
	if organizationID <= 0 || userID <= 0 || strings.TrimSpace(sceneCode) == "" {
		return nil, domain.ErrGroupRouteInvalid
	}
	reader, ok := s.repo.(groupRouteReader)
	if !ok {
		return []facade.GroupRouteOption{}, nil
	}
	items, err := reader.ListEnabledGroupRoutes(ctx, organizationID, userID, sceneCode)
	if err != nil {
		return nil, err
	}
	result := make([]facade.GroupRouteOption, 0, len(items))
	for _, item := range items {
		if s.permissions == nil {
			continue
		}
		allowed, permissionErr := s.permissions.HasPermission(ctx, userID, item.RequiredPermissionCode)
		if permissionErr != nil {
			return nil, permissionErr
		}
		if !allowed {
			continue
		}
		result = append(result, facade.GroupRouteOption{ID: item.ID, Name: item.DisplayName, Provider: string(item.Provider), MentionReady: item.Provider.Valid()})
	}
	return result, nil
}

func (s *Service) ResolvePublicationStatus(ctx context.Context, organizationID int64, receiptIdentity string, viewerUserID int64) (*facade.PublicationStatus, error) {
	if organizationID <= 0 || viewerUserID <= 0 || strings.TrimSpace(receiptIdentity) == "" {
		return nil, domain.ErrGroupRouteInvalid
	}
	reader, ok := s.repo.(publicationStatusReader)
	if !ok {
		return &facade.PublicationStatus{Channels: []facade.PublicationChannelStatus{}}, nil
	}
	items, err := reader.ResolvePublicationStatus(ctx, organizationID, strings.TrimSpace(receiptIdentity), viewerUserID)
	if err != nil {
		return nil, err
	}
	result := make([]facade.PublicationChannelStatus, 0, len(items))
	for _, item := range items {
		result = append(result, facade.PublicationChannelStatus{ChannelKind: item.ChannelKind, DisplayName: item.DisplayName, State: item.State, MentionState: item.MentionState, SafeReasonCode: item.SafeReasonCode, AcceptedAt: item.AcceptedAt, UpdatedAt: item.UpdatedAt})
	}
	return &facade.PublicationStatus{Channels: result}, nil
}
