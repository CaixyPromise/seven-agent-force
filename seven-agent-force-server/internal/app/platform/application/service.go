package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const (
	loginContextCachePrefix          = "platform:login-context:"
	loginContextTTL                  = 10 * time.Minute
	provisioningAuthorityCachePrefix = "platform:provisioning-authority:"
	provisioningAuthorityTTL         = 10 * time.Minute
	loginMethodMaxCount              = 100
	sourceRuleMaxCount               = 200
	defaultRoleMaxCount              = 3

	StepUpActionPlatformCreate              = "PLATFORM_CREATE"
	StepUpActionPlatformUpdate              = "PLATFORM_UPDATE"
	StepUpActionPlatformStatusChange        = "PLATFORM_STATUS_CHANGE"
	StepUpActionPlatformLoginMethodsReplace = "PLATFORM_LOGIN_METHODS_REPLACE"
	StepUpActionPlatformSourceRulesReplace  = "PLATFORM_SOURCE_RULES_REPLACE"
	StepUpActionPlatformDefaultRolesReplace = "PLATFORM_DEFAULT_ROLES_REPLACE"
)

var platformCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)

var platformBrandThemes = map[string]struct{}{
	"blue-cyan": {},
	"light":     {},
	"dark":      {},
}

// platformBrandDocument is deliberately a finite text-and-token projection.
// In particular it has no logo URL field: image authority belongs exclusively
// to the typed CONFIG_ASSET configuration keys.
type platformBrandDocument struct {
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

type cachePort interface {
	Get(ctx context.Context, cacheKey string, dest any) (bool, error)
	GetDel(ctx context.Context, cacheKey string, dest any) (bool, error)
	Set(ctx context.Context, cacheKey string, value any, ttl time.Duration) error
	CompareAndDelete(ctx context.Context, cacheKey string, expected any) (bool, error)
}

type platformDetailBatchRepository interface {
	ListLoginMethodsByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.LoginMethod, error)
	ListSourceRulesByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.SourceRule, error)
	ListDefaultRoleRecordsByPlatformCodes(ctx context.Context, platformCodes []string) ([]domain.DefaultRole, error)
}

// Service orchestrates platform policy lookup and exposes facade contracts.
type Service struct {
	repo       domain.Repository
	cache      cachePort
	idGen      *xid.Generator
	transactor store.Transactor
	authz      ssofacade.AuthorizationSessionFacade
	sessions   ssofacade.SessionFacade
	now        func() time.Time
}

func NewService(repo domain.Repository, cache cachePort, idGen *xid.Generator, transactors ...store.Transactor) *Service {
	var transactor store.Transactor
	if len(transactors) > 0 {
		transactor = transactors[0]
	}
	return &Service{repo: repo, cache: cache, idGen: idGen, transactor: transactor, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) BindAuthorizationSessions(authz ssofacade.AuthorizationSessionFacade) {
	if s == nil {
		return
	}
	s.authz = authz
}

func (s *Service) BindSessions(sessions ssofacade.SessionFacade) {
	if s == nil {
		return
	}
	s.sessions = sessions
}

func BuildPlatformCreateOperationBinding(platformCode string) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|create"
}

func BuildPlatformUpdateOperationBinding(platformCode string) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|update"
}

func BuildPlatformStatusOperationBinding(platformCode string, status int) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|status:" + strconv.Itoa(status)
}

func BuildPlatformLoginMethodsOperationBinding(platformCode string) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|login-methods"
}

func BuildPlatformSourceRulesOperationBinding(platformCode string) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|source-rules"
}

func BuildPlatformDefaultRolesOperationBinding(platformCode string) string {
	return "platform:" + strings.TrimSpace(platformCode) + "|default-roles"
}

func (s *Service) ResolveLoginOptions(ctx context.Context, request platformfacade.ResolvePlatformRequest) (*platformfacade.LoginOptionResult, error) {
	trustedSource, err := s.resolveTrustedSource(ctx, request)
	if err != nil {
		return nil, err
	}
	platformCode, err := s.resolvePlatformCode(ctx, request, trustedSource)
	if err != nil {
		return nil, err
	}
	methods, err := s.repo.ListLoginMethods(ctx, platformCode)
	if err != nil {
		return nil, err
	}
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return nil, err
	}
	platform := findPlatform(platforms, platformCode)
	if platform == nil {
		return nil, apperrors.NotFound("平台不存在或已停用")
	}
	loginContext, err := s.issueLoginContext(ctx, *platform, request, domain.AuthorityPresentation)
	if err != nil {
		return nil, err
	}
	visible := domain.VisibleLoginMethods(methods)
	records := make([]platformfacade.LoginMethodRecord, 0, len(visible))
	for _, method := range visible {
		records = append(records, mapLoginMethod(method, loginContext.ID, trustedSource.ClientID, request.LoginTransactionID, trustedSource.RedirectURL))
	}
	return &platformfacade.LoginOptionResult{
		LoginContextID: loginContext.ID,
		PlatformCode:   platform.PlatformCode,
		PlatformName:   platform.PlatformName,
		Brand:          mapBrand(*platform),
		Registration:   mapRegistrationOptions(*platform),
		Methods:        records,
	}, nil
}

func (s *Service) ResolvePlatformCode(ctx context.Context, request platformfacade.ResolvePlatformRequest) (string, error) {
	trustedSource, err := s.resolveTrustedSource(ctx, request)
	if err != nil {
		return "", err
	}
	return s.resolvePlatformCode(ctx, request, trustedSource)
}

func (s *Service) resolvePlatformCode(ctx context.Context, request platformfacade.ResolvePlatformRequest, trustedSource platformfacade.TrustedSource) (string, error) {
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return "", err
	}
	if len(platforms) == 0 {
		return "", apperrors.NotFound("暂无可用平台")
	}
	rules, err := s.repo.ListActiveSourceRules(ctx)
	if err != nil {
		return "", err
	}
	bindings, err := s.repo.ListActiveSSOClientBindings(ctx)
	if err != nil {
		return "", err
	}
	rules = append(rules, sourceRulesFromBindings(bindings)...)
	source := domain.RequestSource{
		ClientID:         strings.TrimSpace(trustedSource.ClientID),
		RedirectURL:      strings.TrimSpace(trustedSource.RedirectURL),
		Host:             strings.TrimSpace(trustedSource.Host),
		Origin:           strings.TrimSpace(trustedSource.Origin),
		Referer:          strings.TrimSpace(trustedSource.Referer),
		ExplicitCodeHint: strings.TrimSpace(request.ExplicitCode),
	}
	if source.Host == "" {
		source.Host = ""
	}
	if code, ok := domain.MatchPlatformCode(rules, source); ok {
		if findPlatform(platforms, code) != nil {
			return code, nil
		}
		matchedPlatform, findErr := s.repo.FindPlatform(ctx, code)
		if findErr != nil {
			return "", findErr
		}
		if matchedPlatform != nil && matchedPlatform.Status == domain.StatusDisabled {
			return "", apperrors.Forbidden("平台已禁用")
		}
	}
	defaultPlatform, err := s.repo.FindDefaultPlatform(ctx)
	if err != nil {
		return "", err
	}
	if defaultPlatform != nil {
		if platformRequiresTrustedSource(*defaultPlatform) {
			return "", apperrors.Forbidden("无法识别可信登录来源")
		}
		return defaultPlatform.PlatformCode, nil
	}
	return "", apperrors.NotFound("无法识别登录平台")
}

func (s *Service) resolveTrustedSource(ctx context.Context, request platformfacade.ResolvePlatformRequest) (platformfacade.TrustedSource, error) {
	trusted := platformfacade.TrustedSource{
		Host:    strings.TrimSpace(request.TrustedSource.Host),
		Origin:  strings.TrimSpace(request.TrustedSource.Origin),
		Referer: strings.TrimSpace(request.TrustedSource.Referer),
	}
	loginTransactionID := strings.TrimSpace(request.LoginTransactionID)
	if loginTransactionID == "" {
		return trusted, nil
	}
	if s == nil || s.authz == nil {
		return trusted, apperrors.Forbidden("登录事务不可校验，请重新登录")
	}
	session, err := s.authz.GetAuthorizationSession(ctx, loginTransactionID)
	if err != nil {
		return trusted, err
	}
	if session == nil {
		return trusted, apperrors.Forbidden("登录事务已失效，请重新登录")
	}
	clientID := strings.TrimSpace(session.ClientID)
	redirectURI := strings.TrimSpace(session.RedirectURI)
	if candidate := firstNonBlank(request.ClientID, request.TrustedSource.ClientID); candidate != "" && candidate != clientID {
		return trusted, apperrors.Forbidden("登录事务客户端不匹配")
	}
	if candidate := firstNonBlank(request.RedirectURL, request.TrustedSource.RedirectURL); candidate != "" && candidate != redirectURI {
		return trusted, apperrors.Forbidden("登录事务回调地址不匹配")
	}
	trusted.ClientID = clientID
	trusted.RedirectURL = redirectURI
	return trusted, nil
}

func (s *Service) ValidateLoginContext(ctx context.Context, loginContextID string, request platformfacade.ResolvePlatformRequest) (*platformfacade.LoginContextValidation, error) {
	loginContextID = strings.TrimSpace(loginContextID)
	if loginContextID == "" {
		return nil, apperrors.Params("loginContextId不能为空")
	}
	if s.cache == nil {
		return nil, apperrors.System("平台登录上下文缓存未配置")
	}
	var payload loginContextPayload
	hit, err := s.cache.Get(ctx, loginContextCacheKey(loginContextID), &payload)
	if err != nil {
		if errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
			return nil, apperrors.System("平台登录上下文缓存不可用")
		}
		return nil, fmt.Errorf("get platform login context: %w", err)
	}
	if !hit || payload.ExpiresAt.Before(s.now()) {
		return nil, apperrors.ObjectState("登录上下文已失效，请重新登录")
	}
	trustedSource, err := s.resolveTrustedSource(ctx, request)
	if err != nil {
		return nil, err
	}
	request.TrustedSource = trustedSource
	if fingerprint := sourceFingerprint(request); payload.SourceFingerprint != "" && fingerprint != "" && payload.SourceFingerprint != fingerprint {
		return nil, apperrors.Forbidden("登录上下文来源不匹配")
	}
	if err := validateBoundLoginContext(payload, request); err != nil {
		return nil, err
	}
	return &platformfacade.LoginContextValidation{
		LoginContextID:       loginContextID,
		PlatformCode:         payload.PlatformCode,
		Authority:            payload.Authority,
		SourceFingerprint:    payload.SourceFingerprint,
		ProvisioningEligible: payload.ProvisioningEligible,
	}, nil
}

func (s *Service) IssueProvisioningAuthority(ctx context.Context, loginContextID string, request platformfacade.ResolvePlatformRequest) (*platformfacade.ProvisioningAuthority, error) {
	validation, err := s.ValidateLoginContext(ctx, loginContextID, request)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(validation.Authority, domain.AuthorityPresentation) {
		return nil, apperrors.Forbidden("登录上下文授权类型无效")
	}
	if !validation.ProvisioningEligible {
		return nil, apperrors.Forbidden("登录上下文不具备注册链接授权")
	}
	if s.cache == nil {
		return nil, apperrors.System("平台注册授权缓存未配置")
	}
	if s.idGen == nil {
		return nil, apperrors.System("平台注册授权ID生成器未配置")
	}
	authority := platformfacade.ProvisioningAuthority{
		AuthorityID:    "plprov_" + strconv.FormatInt(s.idGen.NextID(), 10),
		LoginContextID: validation.LoginContextID,
		PlatformCode:   validation.PlatformCode,
		Authority:      domain.AuthorityProvisioning,
	}
	payload := provisioningAuthorityPayload{
		LoginContextID: validation.LoginContextID,
		PlatformCode:   validation.PlatformCode,
		Authority:      domain.AuthorityProvisioning,
		ExpiresAt:      s.now().Add(provisioningAuthorityTTL),
	}
	var contextPayload loginContextPayload
	hit, err := s.cache.GetDel(ctx, loginContextCacheKey(validation.LoginContextID), &contextPayload)
	if err != nil {
		if errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
			return nil, apperrors.System("平台登录上下文缓存不可用")
		}
		return nil, fmt.Errorf("consume platform login context: %w", err)
	}
	if !hit || contextPayload.ExpiresAt.Before(s.now()) {
		return nil, apperrors.Forbidden("登录上下文已失效，请重新登录")
	}
	if !contextPayload.ProvisioningEligible ||
		!strings.EqualFold(contextPayload.PlatformCode, validation.PlatformCode) ||
		!strings.EqualFold(contextPayload.Authority, validation.Authority) ||
		(contextPayload.SourceFingerprint != "" && contextPayload.SourceFingerprint != validation.SourceFingerprint) {
		return nil, apperrors.Forbidden("登录上下文来源不匹配")
	}
	if err := s.cache.Set(ctx, provisioningAuthorityCacheKey(authority.AuthorityID), payload, provisioningAuthorityTTL); err != nil {
		if errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
			return nil, apperrors.System("平台注册授权缓存不可用")
		}
		return nil, fmt.Errorf("set platform provisioning authority: %w", err)
	}
	return &authority, nil
}

func validateBoundLoginContext(payload loginContextPayload, request platformfacade.ResolvePlatformRequest) error {
	if !requiredBoundValueMatches(payload.LoginTransactionID, request.LoginTransactionID) {
		return apperrors.Forbidden("登录上下文事务不匹配")
	}
	if !optionalBoundValueMatches(payload.ClientID, request.TrustedSource.ClientID) {
		return apperrors.Forbidden("登录上下文客户端不匹配")
	}
	if !optionalBoundValueMatches(payload.RedirectURL, request.TrustedSource.RedirectURL) {
		return apperrors.Forbidden("登录上下文回调地址不匹配")
	}
	return nil
}

func requiredBoundValueMatches(bound, provided string) bool {
	bound = strings.TrimSpace(bound)
	provided = strings.TrimSpace(provided)
	if bound == "" && provided == "" {
		return true
	}
	if bound == "" || provided == "" {
		return false
	}
	return bound == provided
}

func optionalBoundValueMatches(bound, provided string) bool {
	bound = strings.TrimSpace(bound)
	provided = strings.TrimSpace(provided)
	if bound == "" && provided == "" {
		return true
	}
	if bound == "" || provided == "" {
		return false
	}
	return bound == provided
}

func (s *Service) GetProvisioningPolicy(ctx context.Context, authority platformfacade.ProvisioningAuthority) (*platformfacade.ProvisioningPolicy, error) {
	if authority.Authority != domain.AuthorityProvisioning || strings.TrimSpace(authority.AuthorityID) == "" {
		return nil, apperrors.Forbidden("平台注册授权无效")
	}
	if s.cache == nil {
		return nil, apperrors.System("平台注册授权缓存未配置")
	}
	var payload provisioningAuthorityPayload
	hit, err := s.cache.GetDel(ctx, provisioningAuthorityCacheKey(authority.AuthorityID), &payload)
	if err != nil {
		if errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
			return nil, apperrors.System("平台注册授权缓存不可用")
		}
		return nil, fmt.Errorf("consume platform provisioning authority: %w", err)
	}
	if !hit || payload.ExpiresAt.Before(s.now()) {
		return nil, apperrors.Forbidden("平台注册授权已失效")
	}
	if !strings.EqualFold(payload.Authority, domain.AuthorityProvisioning) {
		return nil, apperrors.Forbidden("平台注册授权类型无效")
	}
	if strings.TrimSpace(authority.LoginContextID) != "" && strings.TrimSpace(authority.LoginContextID) != strings.TrimSpace(payload.LoginContextID) {
		return nil, apperrors.Forbidden("平台注册授权上下文不匹配")
	}
	if strings.TrimSpace(authority.PlatformCode) != "" && !strings.EqualFold(strings.TrimSpace(authority.PlatformCode), strings.TrimSpace(payload.PlatformCode)) {
		return nil, apperrors.Forbidden("平台注册授权平台不匹配")
	}
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return nil, err
	}
	platform := findPlatform(platforms, payload.PlatformCode)
	if platform == nil {
		return nil, apperrors.NotFound("平台不存在或已停用")
	}
	if !platform.AllowAutoRegister {
		return &platformfacade.ProvisioningPolicy{
			PlatformCode:        platform.PlatformCode,
			AllowAutoRegister:   false,
			AllowFormRegister:   platform.AllowFormRegister,
			DefaultRoleIDs:      []int64{},
			DefaultRoleMaxCount: defaultRoleMaxCount,
		}, nil
	}
	return s.buildRegistrationPolicy(ctx, *platform, true, platform.AllowFormRegister)
}

func (s *Service) GetFormRegistrationPolicy(ctx context.Context, platformCode string) (*platformfacade.ProvisioningPolicy, error) {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return nil, err
	}
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return nil, err
	}
	platform := findPlatform(platforms, code)
	if platform == nil {
		return nil, apperrors.NotFound("平台不存在或已停用")
	}
	if !platform.AllowFormRegister {
		return &platformfacade.ProvisioningPolicy{
			PlatformCode:        platform.PlatformCode,
			AllowAutoRegister:   platform.AllowAutoRegister,
			AllowFormRegister:   false,
			DefaultRoleIDs:      []int64{},
			DefaultRoleMaxCount: defaultRoleMaxCount,
		}, nil
	}
	return s.buildRegistrationPolicy(ctx, *platform, platform.AllowAutoRegister, true)
}

func (s *Service) buildRegistrationPolicy(ctx context.Context, platform domain.Platform, allowAutoRegister, allowFormRegister bool) (*platformfacade.ProvisioningPolicy, error) {
	var roles []domain.DefaultRole
	loadRoles := func(readCtx context.Context) error {
		var err error
		roles, err = s.repo.ListDefaultRoles(readCtx, platform.PlatformCode, defaultRoleMaxCount)
		return err
	}
	snapshotter, ok := s.transactor.(store.Snapshotter)
	if !ok || !snapshotter.Enabled() {
		return nil, apperrors.System("平台默认角色一致性快照能力未配置")
	}
	err := snapshotter.WithinReadOnlySnapshot(ctx, loadRoles)
	if err != nil {
		return nil, err
	}
	if len(roles) > defaultRoleMaxCount {
		return nil, apperrors.ObjectState("平台默认角色数量超过限制")
	}
	roleIDs := make([]int64, 0, len(roles))
	for _, role := range roles {
		if role.RoleID > 0 && role.AutoAssignEnabled && role.Status == domain.StatusActive {
			roleIDs = append(roleIDs, role.RoleID)
		}
	}
	defaultOrgID, defaultPostIDs := defaultRegistrationSettingsFromSettings(platform.SettingsJSON)
	return &platformfacade.ProvisioningPolicy{
		PlatformCode:        platform.PlatformCode,
		AllowAutoRegister:   allowAutoRegister,
		AllowFormRegister:   allowFormRegister,
		DefaultOrgID:        defaultOrgID,
		DefaultDeptID:       platform.DefaultDeptID,
		DefaultPostIDs:      defaultPostIDs,
		DefaultRoleIDs:      roleIDs,
		DefaultRoleMaxCount: defaultRoleMaxCount,
	}, nil
}

func (s *Service) RequireLoginMethod(ctx context.Context, platformCode, methodType, providerCode string) error {
	methods, err := s.repo.ListLoginMethods(ctx, platformCode)
	if err != nil {
		return err
	}
	methodType = strings.ToUpper(strings.TrimSpace(methodType))
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	for _, method := range methods {
		if !method.DisplayEnabled || !method.LoginEnabled {
			continue
		}
		if strings.EqualFold(method.MethodType, methodType) && strings.EqualFold(method.ProviderCode, providerCode) {
			return nil
		}
	}
	return apperrors.Forbidden("当前平台不允许该登录方式")
}

func (s *Service) ListPlatforms(ctx context.Context, query platformfacade.PlatformQuery) (*platformfacade.PlatformPage, error) {
	current, pageSize := normalizePage(query.Current, query.PageSize)
	batchRepo, ok := s.repo.(platformDetailBatchRepository)
	if !ok {
		return nil, apperrors.System("平台详情批量仓储能力未配置")
	}
	snapshotter, ok := s.transactor.(store.Snapshotter)
	if !ok || !snapshotter.Enabled() {
		return nil, apperrors.System("平台列表一致性快照能力未配置")
	}
	var (
		items   []domain.Platform
		total   int64
		methods []domain.LoginMethod
		rules   []domain.SourceRule
		roles   []domain.DefaultRole
	)
	err := snapshotter.WithinReadOnlySnapshot(ctx, func(readCtx context.Context) error {
		var readErr error
		items, total, readErr = s.repo.ListPlatforms(readCtx, domain.PlatformQuery{
			Keyword:      strings.TrimSpace(query.Keyword),
			PlatformCode: strings.TrimSpace(query.PlatformCode),
			PlatformType: strings.TrimSpace(query.PlatformType),
			Status:       query.Status,
			Current:      current,
			PageSize:     pageSize,
		})
		if readErr != nil || len(items) == 0 {
			return readErr
		}
		platformCodes := make([]string, 0, len(items))
		for _, item := range items {
			platformCodes = append(platformCodes, item.PlatformCode)
		}
		if methods, readErr = batchRepo.ListLoginMethodsByPlatformCodes(readCtx, platformCodes); readErr != nil {
			return readErr
		}
		if rules, readErr = batchRepo.ListSourceRulesByPlatformCodes(readCtx, platformCodes); readErr != nil {
			return readErr
		}
		roles, readErr = batchRepo.ListDefaultRoleRecordsByPlatformCodes(readCtx, platformCodes)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	methodsByPlatform := groupLoginMethodsByPlatform(methods)
	rulesByPlatform := groupSourceRulesByPlatform(rules)
	rolesByPlatform := groupDefaultRolesByPlatform(roles)
	records := make([]platformfacade.PlatformDetail, 0, len(items))
	for _, item := range items {
		records = append(records, mapPlatformDetail(
			item,
			methodsByPlatform[item.PlatformCode],
			rulesByPlatform[item.PlatformCode],
			rolesByPlatform[item.PlatformCode],
		))
	}
	return &platformfacade.PlatformPage{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

func groupLoginMethodsByPlatform(items []domain.LoginMethod) map[string][]domain.LoginMethod {
	result := make(map[string][]domain.LoginMethod)
	for _, item := range items {
		result[item.PlatformCode] = append(result[item.PlatformCode], item)
	}
	return result
}

func groupSourceRulesByPlatform(items []domain.SourceRule) map[string][]domain.SourceRule {
	result := make(map[string][]domain.SourceRule)
	for _, item := range items {
		result[item.PlatformCode] = append(result[item.PlatformCode], item)
	}
	return result
}

func groupDefaultRolesByPlatform(items []domain.DefaultRole) map[string][]domain.DefaultRole {
	result := make(map[string][]domain.DefaultRole)
	for _, item := range items {
		result[item.PlatformCode] = append(result[item.PlatformCode], item)
	}
	return result
}

func (s *Service) GetPlatform(ctx context.Context, platformCode string) (*platformfacade.PlatformDetail, error) {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return nil, err
	}
	platform, err := s.repo.FindPlatform(ctx, code)
	if err != nil {
		return nil, err
	}
	if platform == nil {
		return nil, apperrors.NotFound("平台不存在")
	}
	methods, err := s.repo.ListLoginMethods(ctx, code)
	if err != nil {
		return nil, err
	}
	rules, err := s.repo.ListSourceRules(ctx, code)
	if err != nil {
		return nil, err
	}
	roles, err := s.repo.ListDefaultRoleRecords(ctx, code)
	if err != nil {
		return nil, err
	}
	detail := mapPlatformDetail(*platform, methods, rules, roles)
	return &detail, nil
}

// GetManagedLoginPolicy returns the safe subset of the local default policy.
func (s *Service) GetManagedLoginPolicy(ctx context.Context) (*platformfacade.ManagedLoginPolicy, error) {
	platform, err := s.repo.FindManagedDefaultPlatform(ctx)
	if err != nil {
		return nil, err
	}
	if platform == nil {
		return nil, apperrors.NotFound("默认平台不存在")
	}
	methods, err := s.repo.ListManagedLoginMethods(ctx, platform.PlatformCode)
	if err != nil {
		return nil, err
	}
	rules, err := s.repo.ListManagedSourceRules(ctx, platform.PlatformCode)
	if err != nil {
		return nil, err
	}
	result := &platformfacade.ManagedLoginPolicy{
		PlatformCode:      platform.PlatformCode,
		Status:            platform.Status,
		AllowAutoRegister: platform.AllowAutoRegister,
		AllowFormRegister: platform.AllowFormRegister,
		LoginMethods:      make([]platformfacade.ManagedLoginMethod, 0, len(methods)),
		SourceRules:       make([]platformfacade.ManagedSourceRule, 0, len(rules)),
	}
	for _, method := range methods {
		result.LoginMethods = append(result.LoginMethods, platformfacade.ManagedLoginMethod{
			MethodType: method.MethodType, ProviderCode: method.ProviderCode, DisplayName: method.DisplayName,
			Icon: method.Icon, SortOrder: method.SortOrder, DisplayEnabled: method.DisplayEnabled, LoginEnabled: method.LoginEnabled,
		})
	}
	for _, rule := range rules {
		result.SourceRules = append(result.SourceRules, platformfacade.ManagedSourceRule{
			MatchType: rule.MatchType, MatchValue: rule.MatchValue, Priority: rule.Priority, Status: rule.Status,
		})
	}
	return result, nil
}

// ApplyManagedLoginPolicy validates and applies one complete safe snapshot.
func (s *Service) ApplyManagedLoginPolicy(ctx context.Context, command platformfacade.ApplyManagedLoginPolicyCommand) (int64, error) {
	code, err := normalizePlatformCode(command.PlatformCode)
	if err != nil {
		return 0, err
	}
	if err := validateLoginMethodCount(len(command.LoginMethods)); err != nil {
		return 0, err
	}
	if err := validateSourceRuleCount(len(command.SourceRules)); err != nil {
		return 0, err
	}
	status, err := normalizeStatus(command.Status)
	if err != nil {
		return 0, err
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return 0, apperrors.ServiceUnavailable("登录策略事务能力不可用")
	}
	var changed int64
	err = s.withTransaction(ctx, func(txCtx context.Context) error {
		current, err := s.repo.FindManagedDefaultPlatformForUpdate(txCtx)
		if err != nil {
			return err
		}
		if current == nil {
			return apperrors.NotFound("默认平台不存在")
		}
		if !strings.EqualFold(code, current.PlatformCode) {
			return apperrors.ObjectState("策略平台与本地默认平台不匹配")
		}
		methodRequests := make([]platformfacade.LoginMethodSaveRequest, 0, len(command.LoginMethods))
		for _, method := range command.LoginMethods {
			methodRequests = append(methodRequests, platformfacade.LoginMethodSaveRequest{MethodType: method.MethodType, ProviderCode: method.ProviderCode, DisplayName: method.DisplayName, Icon: method.Icon, SortOrder: method.SortOrder, DisplayEnabled: method.DisplayEnabled, LoginEnabled: method.LoginEnabled})
		}
		methods, err := s.normalizeManagedLoginMethods(txCtx, code, methodRequests)
		if err != nil {
			return err
		}
		ruleRequests := make([]platformfacade.SourceRuleSaveRequest, 0, len(command.SourceRules))
		for _, rule := range command.SourceRules {
			ruleRequests = append(ruleRequests, platformfacade.SourceRuleSaveRequest{MatchType: rule.MatchType, MatchValue: rule.MatchValue, Priority: rule.Priority, Status: rule.Status})
		}
		rules, err := normalizeSourceRules(code, ruleRequests)
		if err != nil {
			return err
		}
		beforeMethods, err := s.repo.ListManagedLoginMethodsForUpdate(txCtx, code)
		if err != nil {
			return err
		}
		beforeRules, err := s.repo.ListManagedSourceRulesForUpdate(txCtx, code)
		if err != nil {
			return err
		}
		preserveManagedLoginMethodMetadata(beforeMethods, methods)
		preserveManagedSourceRuleMetadata(beforeRules, rules)
		methodsChanged := !managedLoginMethodsEqual(beforeMethods, methods)
		rulesChanged := !managedSourceRulesEqual(beforeRules, rules)
		disabledMethods := disabledLoginMethods(beforeMethods, methods)
		updated := *current
		updated.AllowAutoRegister = command.AllowAutoRegister
		updated.AllowFormRegister = command.AllowFormRegister
		platformFieldsChanged := current.AllowAutoRegister != updated.AllowAutoRegister || current.AllowFormRegister != updated.AllowFormRegister
		statusChanged := current.Status != status
		platformDisabled := current.Status != domain.StatusDisabled && status == domain.StatusDisabled
		if !platformFieldsChanged && !statusChanged && !methodsChanged && !rulesChanged {
			return nil
		}
		changed = 1
		if platformFieldsChanged {
			if err := s.repo.UpdatePlatform(txCtx, updated, 0); err != nil {
				return err
			}
		}
		if statusChanged {
			if err := s.repo.UpdatePlatformStatus(txCtx, code, status, 0); err != nil {
				return err
			}
		}
		if methodsChanged {
			if err := s.repo.ReplaceLoginMethods(txCtx, code, methods, 0); err != nil {
				return err
			}
		}
		if rulesChanged {
			if err := s.repo.ReplaceSourceRules(txCtx, code, rules, 0); err != nil {
				return err
			}
		}
		if platformDisabled {
			if s.sessions == nil {
				return apperrors.ServiceUnavailable("SSO会话撤销能力不可用")
			}
			if _, err := s.sessions.RevokeSessionsByPlatformCode(txCtx, code); err != nil {
				return err
			}
			return nil
		}
		if len(disabledMethods) > 0 && s.sessions == nil {
			return apperrors.ServiceUnavailable("SSO会话撤销能力不可用")
		}
		for _, method := range disabledMethods {
			if _, err := s.sessions.RevokeSessionsByPlatformLoginMethod(txCtx, code, method.MethodType, method.ProviderCode); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

func (s *Service) CreatePlatform(ctx context.Context, actorID int64, req platformfacade.PlatformSaveRequest, proof stepup.ProofMetadata) (*platformfacade.PlatformDetail, error) {
	platform, err := platformFromSaveRequest(req)
	if err != nil {
		return nil, err
	}
	if platformRequiresProof(platform) {
		if strings.TrimSpace(req.Reason) == "" {
			return nil, apperrors.Params("reason不能为空")
		}
		if err := stepup.Require(proof, StepUpActionPlatformCreate, BuildPlatformCreateOperationBinding(platform.PlatformCode)); err != nil {
			return nil, err
		}
	}
	if err := s.repo.InsertPlatform(ctx, platform, actorID); err != nil {
		return nil, err
	}
	return s.GetPlatform(ctx, platform.PlatformCode)
}

func (s *Service) UpdatePlatform(ctx context.Context, actorID int64, platformCode string, req platformfacade.PlatformSaveRequest, proof stepup.ProofMetadata) (*platformfacade.PlatformDetail, error) {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return nil, err
	}
	before, err := s.repo.FindPlatform(ctx, code)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, apperrors.NotFound("平台不存在")
	}
	platform, err := platformFromSaveRequest(req)
	if err != nil {
		return nil, err
	}
	platform.PlatformCode = code
	platform.Status = before.Status
	if sensitivePlatformChange(*before, platform) {
		if strings.TrimSpace(req.Reason) == "" {
			return nil, apperrors.Params("reason不能为空")
		}
		if err := stepup.Require(proof, StepUpActionPlatformUpdate, BuildPlatformUpdateOperationBinding(code)); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdatePlatform(ctx, platform, actorID); err != nil {
		return nil, err
	}
	return s.GetPlatform(ctx, code)
}

func (s *Service) UpdatePlatformStatus(ctx context.Context, actorID int64, platformCode string, req platformfacade.PlatformStatusRequest, proof stepup.ProofMetadata) error {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return err
	}
	status, err := normalizeStatus(req.Status)
	if err != nil {
		return err
	}
	if strings.TrimSpace(req.Reason) == "" {
		return apperrors.Params("reason不能为空")
	}
	if err := stepup.Require(proof, StepUpActionPlatformStatusChange, BuildPlatformStatusOperationBinding(code, status)); err != nil {
		return err
	}
	if status == domain.StatusDisabled {
		if err := s.ensureCanDisablePlatform(ctx, code); err != nil {
			return err
		}
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdatePlatformStatus(txCtx, code, status, actorID); err != nil {
			return err
		}
		if status == domain.StatusDisabled && s.sessions == nil {
			return apperrors.System("SSO会话撤销能力未配置")
		}
		if status == domain.StatusDisabled {
			if _, err := s.sessions.RevokeSessionsByPlatformCode(txCtx, code); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ReplaceLoginMethods(ctx context.Context, actorID int64, platformCode string, methods []platformfacade.LoginMethodSaveRequest, proof stepup.ProofMetadata) error {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding(code)); err != nil {
		return err
	}
	if err := validateLoginMethodCount(len(methods)); err != nil {
		return err
	}
	if _, err := s.requirePlatform(ctx, code); err != nil {
		return err
	}
	before, err := s.repo.ListLoginMethods(ctx, code)
	if err != nil {
		return err
	}
	items, err := s.normalizeLoginMethods(ctx, code, methods)
	if err != nil {
		return err
	}
	disabledMethods := disabledLoginMethods(before, items)
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		if err := s.repo.ReplaceLoginMethods(txCtx, code, items, actorID); err != nil {
			return err
		}
		if len(disabledMethods) == 0 {
			return nil
		}
		if s.sessions == nil {
			return apperrors.System("SSO会话撤销能力未配置")
		}
		for _, method := range disabledMethods {
			if _, err := s.sessions.RevokeSessionsByPlatformLoginMethod(txCtx, code, method.MethodType, method.ProviderCode); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ReplaceSourceRules(ctx context.Context, actorID int64, platformCode string, rules []platformfacade.SourceRuleSaveRequest, proof stepup.ProofMetadata) error {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionPlatformSourceRulesReplace, BuildPlatformSourceRulesOperationBinding(code)); err != nil {
		return err
	}
	if err := validateSourceRuleCount(len(rules)); err != nil {
		return err
	}
	if _, err := s.requirePlatform(ctx, code); err != nil {
		return err
	}
	items, err := normalizeSourceRules(code, rules)
	if err != nil {
		return err
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		return s.repo.ReplaceSourceRules(txCtx, code, items, actorID)
	})
}

func (s *Service) ReplaceDefaultRoles(ctx context.Context, actorID int64, platformCode string, roles []platformfacade.DefaultRoleSaveRequest, proof stepup.ProofMetadata) error {
	code, err := normalizePlatformCode(platformCode)
	if err != nil {
		return err
	}
	if err := stepup.Require(proof, StepUpActionPlatformDefaultRolesReplace, BuildPlatformDefaultRolesOperationBinding(code)); err != nil {
		return err
	}
	if err := validateDefaultRoleCount(len(roles)); err != nil {
		return err
	}
	if _, err := s.requirePlatform(ctx, code); err != nil {
		return err
	}
	items, err := s.normalizeDefaultRoles(ctx, code, roles)
	if err != nil {
		return err
	}
	return s.withTransaction(ctx, func(txCtx context.Context) error {
		return s.repo.ReplaceDefaultRoles(txCtx, code, items, actorID)
	})
}

func (s *Service) requirePlatform(ctx context.Context, platformCode string) (*domain.Platform, error) {
	platform, err := s.repo.FindPlatform(ctx, platformCode)
	if err != nil {
		return nil, err
	}
	if platform == nil {
		return nil, apperrors.NotFound("平台不存在")
	}
	return platform, nil
}

func (s *Service) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor != nil && s.transactor.Enabled() {
		return s.transactor.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}

func mapLoginMethod(method domain.LoginMethod, loginContextID, clientID, loginTransactionID, redirectURL string) platformfacade.LoginMethodRecord {
	loginURL := ""
	if strings.EqualFold(method.MethodType, domain.MethodExternalOAuth) && method.ProviderCode != "" {
		query := url.Values{}
		if strings.TrimSpace(loginContextID) != "" {
			query.Set("loginContextId", strings.TrimSpace(loginContextID))
		}
		if strings.TrimSpace(clientID) != "" {
			query.Set("clientId", strings.TrimSpace(clientID))
		}
		if strings.TrimSpace(loginTransactionID) != "" {
			query.Set("loginTransactionId", strings.TrimSpace(loginTransactionID))
		}
		if strings.TrimSpace(redirectURL) != "" {
			query.Set("redirectAfterLogin", strings.TrimSpace(redirectURL))
		}
		loginURL = fmt.Sprintf("/api/login/external/%s/start", url.PathEscape(method.ProviderCode))
		if encoded := query.Encode(); encoded != "" {
			loginURL += "?" + encoded
		}
	}
	return platformfacade.LoginMethodRecord{
		MethodType:   method.MethodType,
		ProviderCode: method.ProviderCode,
		DisplayName:  method.DisplayName,
		Icon:         method.Icon,
		SortOrder:    method.SortOrder,
		LoginURL:     loginURL,
	}
}

func mapPlatformDetail(platform domain.Platform, methods []domain.LoginMethod, rules []domain.SourceRule, roles []domain.DefaultRole) platformfacade.PlatformDetail {
	return platformfacade.PlatformDetail{
		ID:                 platform.ID,
		PlatformCode:       platform.PlatformCode,
		PlatformName:       platform.PlatformName,
		PlatformType:       platform.PlatformType,
		Description:        platform.Description,
		DefaultRedirectURL: platform.DefaultRedirectURL,
		AllowAutoRegister:  platform.AllowAutoRegister,
		AllowFormRegister:  platform.AllowFormRegister,
		IsDefault:          platform.IsDefault,
		DefaultDeptID:      platform.DefaultDeptID,
		BrandJSON:          platform.BrandJSON,
		SettingsJSON:       platform.SettingsJSON,
		Status:             platform.Status,
		LoginMethods:       mapLoginMethodDetails(methods),
		SourceRules:        mapSourceRuleRecords(rules),
		DefaultRoles:       mapDefaultRoleRecords(roles),
		CreateTime:         platform.CreateTime,
		UpdateTime:         platform.UpdateTime,
	}
}

func mapLoginMethodDetails(methods []domain.LoginMethod) []platformfacade.LoginMethodDetail {
	result := make([]platformfacade.LoginMethodDetail, 0, len(methods))
	for _, method := range methods {
		result = append(result, platformfacade.LoginMethodDetail{
			ID:             method.ID,
			MethodType:     method.MethodType,
			ProviderCode:   method.ProviderCode,
			DisplayName:    method.DisplayName,
			Icon:           method.Icon,
			SortOrder:      method.SortOrder,
			DisplayEnabled: method.DisplayEnabled,
			LoginEnabled:   method.LoginEnabled,
			MetadataJSON:   method.MetadataJSON,
		})
	}
	return result
}

func mapSourceRuleRecords(rules []domain.SourceRule) []platformfacade.SourceRuleRecord {
	result := make([]platformfacade.SourceRuleRecord, 0, len(rules))
	for _, rule := range rules {
		result = append(result, platformfacade.SourceRuleRecord{
			ID:           rule.ID,
			MatchType:    rule.MatchType,
			MatchValue:   rule.MatchValue,
			Priority:     rule.Priority,
			Status:       rule.Status,
			MetadataJSON: rule.MetadataJSON,
		})
	}
	return result
}

func mapDefaultRoleRecords(roles []domain.DefaultRole) []platformfacade.DefaultRoleRecord {
	result := make([]platformfacade.DefaultRoleRecord, 0, len(roles))
	for _, role := range roles {
		result = append(result, platformfacade.DefaultRoleRecord{
			ID:                role.ID,
			RoleID:            role.RoleID,
			AutoAssignEnabled: role.AutoAssignEnabled,
			Status:            role.Status,
		})
	}
	return result
}

func (s *Service) issueLoginContext(ctx context.Context, platform domain.Platform, request platformfacade.ResolvePlatformRequest, authority string) (*domain.LoginContext, error) {
	if s.cache == nil {
		return nil, apperrors.System("平台登录上下文缓存未配置")
	}
	if s.idGen == nil {
		return nil, apperrors.System("平台登录上下文ID生成器未配置")
	}
	now := s.now()
	trustedSource, err := s.resolveTrustedSource(ctx, request)
	if err != nil {
		return nil, err
	}
	request.TrustedSource = trustedSource
	item := domain.LoginContext{
		ID:                 "plctx_" + strconv.FormatInt(s.idGen.NextID(), 10),
		PlatformCode:       platform.PlatformCode,
		Authority:          authority,
		ClientID:           strings.TrimSpace(trustedSource.ClientID),
		LoginTransactionID: strings.TrimSpace(request.LoginTransactionID),
		RedirectURL:        strings.TrimSpace(trustedSource.RedirectURL),
		SourceFingerprint:  sourceFingerprint(request),
		ExpiresAt:          now.Add(loginContextTTL),
	}
	provisioningEligible := item.LoginTransactionID != "" && item.ClientID != "" && item.RedirectURL != ""
	payload := loginContextPayload{
		PlatformCode:         item.PlatformCode,
		Authority:            item.Authority,
		ClientID:             item.ClientID,
		LoginTransactionID:   item.LoginTransactionID,
		RedirectURL:          item.RedirectURL,
		SourceFingerprint:    item.SourceFingerprint,
		ProvisioningEligible: provisioningEligible,
		ExpiresAt:            item.ExpiresAt,
	}
	if err := s.cache.Set(ctx, loginContextCacheKey(item.ID), payload, loginContextTTL); err != nil {
		if errors.Is(err, cacheinfra.ErrCacheLayerUnsupported) {
			return nil, apperrors.System("平台登录上下文缓存不可用")
		}
		return nil, fmt.Errorf("set platform login context: %w", err)
	}
	return &item, nil
}

type loginContextPayload struct {
	PlatformCode         string    `json:"platformCode"`
	Authority            string    `json:"authority"`
	ClientID             string    `json:"clientId,omitempty"`
	LoginTransactionID   string    `json:"loginTransactionId,omitempty"`
	RedirectURL          string    `json:"redirectUrl,omitempty"`
	SourceFingerprint    string    `json:"sourceFingerprint,omitempty"`
	ProvisioningEligible bool      `json:"provisioningEligible"`
	ExpiresAt            time.Time `json:"expiresAt"`
}

type provisioningAuthorityPayload struct {
	LoginContextID string    `json:"loginContextId"`
	PlatformCode   string    `json:"platformCode"`
	Authority      string    `json:"authority"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

func loginContextCacheKey(loginContextID string) string {
	return loginContextCachePrefix + strings.TrimSpace(loginContextID)
}

func provisioningAuthorityCacheKey(authorityID string) string {
	return provisioningAuthorityCachePrefix + strings.TrimSpace(authorityID)
}

func sourceFingerprint(request platformfacade.ResolvePlatformRequest) string {
	parts := []string{
		strings.TrimSpace(request.TrustedSource.ClientID),
		strings.TrimSpace(request.TrustedSource.RedirectURL),
		strings.TrimSpace(request.TrustedSource.Host),
	}
	return strings.Join(parts, "\n")
}

func mapBrand(platform domain.Platform) platformfacade.PlatformBrand {
	brand := platformfacade.PlatformBrand{
		Title:    firstNonBlank(platform.PlatformName, "Seven"),
		Subtitle: "统一身份认证系统",
		Theme:    "blue-cyan",
	}
	if strings.TrimSpace(platform.BrandJSON) == "" {
		return brand
	}
	// The read projection intentionally ignores unknown historical fields. This
	// keeps a legacy logoUrl from becoming a presentation authority even before
	// an upgraded database has run the retirement migration.
	var parsed platformBrandDocument
	if err := json.Unmarshal([]byte(platform.BrandJSON), &parsed); err != nil {
		return brand
	}
	if strings.TrimSpace(parsed.Title) == "" {
		parsed.Title = brand.Title
	}
	if strings.TrimSpace(parsed.Subtitle) == "" {
		parsed.Subtitle = brand.Subtitle
	}
	parsed.Title = strings.TrimSpace(parsed.Title)
	parsed.Subtitle = strings.TrimSpace(parsed.Subtitle)
	parsed.Theme = strings.ToLower(strings.TrimSpace(parsed.Theme))
	if parsed.Theme == "" || !isPlatformBrandTheme(parsed.Theme) {
		parsed.Theme = brand.Theme
	}
	return platformfacade.PlatformBrand{
		Title:    parsed.Title,
		Subtitle: parsed.Subtitle,
		Theme:    parsed.Theme,
	}
}

func mapRegistrationOptions(platform domain.Platform) platformfacade.RegistrationOptions {
	if !platform.AllowFormRegister {
		return platformfacade.RegistrationOptions{
			FormRegisterEnabled: false,
			RequireCaptcha:      true,
			RequiredFields:      []string{},
		}
	}
	return platformfacade.RegistrationOptions{
		FormRegisterEnabled: true,
		RequireCaptcha:      true,
		RequiredFields:      []string{"userAccount", "userName", "userEmail", "password"},
	}
}

func sourceRulesFromBindings(bindings []domain.SSOClientBinding) []domain.SourceRule {
	rules := make([]domain.SourceRule, 0, len(bindings))
	for _, binding := range bindings {
		rules = append(rules, domain.SourceRule{
			PlatformCode: binding.PlatformCode,
			MatchType:    domain.MatchClientID,
			MatchValue:   binding.ClientID,
			Priority:     1000,
			Status:       domain.StatusActive,
		})
	}
	return rules
}

func findPlatform(platforms []domain.Platform, code string) *domain.Platform {
	for i := range platforms {
		if strings.EqualFold(platforms[i].PlatformCode, code) {
			return &platforms[i]
		}
	}
	return nil
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

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

func normalizePlatformCode(value string) (string, error) {
	code := strings.ToLower(strings.TrimSpace(value))
	if len(code) < 2 || len(code) > 64 || !platformCodePattern.MatchString(code) {
		return "", apperrors.Params("platformCode格式错误")
	}
	return code, nil
}

func normalizeStatus(status int) (int, error) {
	if status != domain.StatusActive && status != domain.StatusDisabled {
		return 0, apperrors.Params("status格式错误")
	}
	return status, nil
}

func platformFromSaveRequest(req platformfacade.PlatformSaveRequest) (domain.Platform, error) {
	code, err := normalizePlatformCode(req.PlatformCode)
	if err != nil {
		return domain.Platform{}, err
	}
	name := strings.TrimSpace(req.PlatformName)
	if name == "" || len([]rune(name)) > 128 {
		return domain.Platform{}, apperrors.Params("platformName长度错误")
	}
	status := domain.StatusActive
	if req.Status != nil {
		normalizedStatus, statusErr := normalizeStatus(*req.Status)
		if statusErr != nil {
			return domain.Platform{}, statusErr
		}
		status = normalizedStatus
	}
	if req.IsDefault && status == domain.StatusDisabled {
		return domain.Platform{}, apperrors.Operation("默认平台不允许停用")
	}
	brandJSON, err := normalizePlatformBrandJSON(req.BrandJSON)
	if err != nil {
		return domain.Platform{}, err
	}
	return domain.Platform{
		PlatformCode:       code,
		PlatformName:       name,
		PlatformType:       firstNonBlank(strings.ToUpper(strings.TrimSpace(req.PlatformType)), "ADMIN"),
		Description:        strings.TrimSpace(req.Description),
		DefaultRedirectURL: strings.TrimSpace(req.DefaultRedirectURL),
		AllowAutoRegister:  req.AllowAutoRegister,
		AllowFormRegister:  req.AllowFormRegister,
		IsDefault:          req.IsDefault,
		DefaultDeptID:      req.DefaultDeptID,
		BrandJSON:          brandJSON,
		SettingsJSON:       strings.TrimSpace(req.SettingsJSON),
		Status:             status,
	}, nil
}

// normalizePlatformBrandJSON accepts only the persisted text fields and a
// finite theme token. It rejects logoUrl and every other unknown field so a
// direct management HTTP caller cannot retain an external/blob/data/file URL
// as a parallel login image source.
func normalizePlatformBrandJSON(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var parsed platformBrandDocument
	if err := decoder.Decode(&parsed); err != nil {
		return "", apperrors.Params("brandJson仅支持标题、副标题和有限主题")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", apperrors.Params("brandJson必须是单个JSON对象")
	}
	parsed.Title = strings.TrimSpace(parsed.Title)
	parsed.Subtitle = strings.TrimSpace(parsed.Subtitle)
	parsed.Theme = strings.ToLower(strings.TrimSpace(parsed.Theme))
	if len([]rune(parsed.Title)) > 128 || len([]rune(parsed.Subtitle)) > 256 {
		return "", apperrors.Params("brandJson文本长度错误")
	}
	if parsed.Theme != "" && !isPlatformBrandTheme(parsed.Theme) {
		return "", apperrors.Params("brandJson主题不受支持")
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return "", apperrors.System("brandJson序列化失败")
	}
	return string(encoded), nil
}

func isPlatformBrandTheme(value string) bool {
	_, ok := platformBrandThemes[strings.ToLower(strings.TrimSpace(value))]
	return ok
}

func platformRequiresProof(platform domain.Platform) bool {
	return platform.AllowAutoRegister || platform.AllowFormRegister || strings.TrimSpace(platform.DefaultRedirectURL) != "" || platform.DefaultDeptID != nil || platform.IsDefault || strings.TrimSpace(platform.SettingsJSON) != ""
}

func sensitivePlatformChange(before, after domain.Platform) bool {
	if before.AllowAutoRegister != after.AllowAutoRegister || before.AllowFormRegister != after.AllowFormRegister || before.IsDefault != after.IsDefault {
		return true
	}
	if strings.TrimSpace(before.DefaultRedirectURL) != strings.TrimSpace(after.DefaultRedirectURL) {
		return true
	}
	if (before.DefaultDeptID == nil) != (after.DefaultDeptID == nil) {
		return true
	}
	if before.DefaultDeptID != nil && after.DefaultDeptID != nil && *before.DefaultDeptID != *after.DefaultDeptID {
		return true
	}
	return strings.TrimSpace(before.SettingsJSON) != strings.TrimSpace(after.SettingsJSON)
}

func platformRequiresTrustedSource(platform domain.Platform) bool {
	settings := strings.TrimSpace(platform.SettingsJSON)
	if settings == "" {
		return false
	}
	var parsed struct {
		RequireTrustedSource bool   `json:"requireTrustedSource"`
		SourcePolicy         string `json:"sourcePolicy"`
	}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		return false
	}
	if parsed.RequireTrustedSource {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(parsed.SourcePolicy), "STRICT_MATCH")
}

func defaultRegistrationSettingsFromSettings(settings string) (*int64, []int64) {
	settings = strings.TrimSpace(settings)
	if settings == "" {
		return nil, nil
	}
	var parsed struct {
		DefaultOrgID   *int64  `json:"defaultOrgId"`
		DefaultPostIDs []int64 `json:"defaultPostIds"`
	}
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		return nil, nil
	}
	var defaultOrgID *int64
	if parsed.DefaultOrgID != nil && *parsed.DefaultOrgID > 0 {
		value := *parsed.DefaultOrgID
		defaultOrgID = &value
	}
	return defaultOrgID, uniquePositiveInt64s(parsed.DefaultPostIDs)
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func (s *Service) ensureCanDisablePlatform(ctx context.Context, platformCode string) error {
	platforms, err := s.repo.ListActivePlatforms(ctx)
	if err != nil {
		return err
	}
	activeOthers := 0
	var target *domain.Platform
	for i := range platforms {
		if strings.EqualFold(platforms[i].PlatformCode, platformCode) {
			target = &platforms[i]
			continue
		}
		activeOthers++
	}
	if target != nil && target.IsDefault {
		return apperrors.Operation("默认平台不允许停用")
	}
	if activeOthers == 0 {
		return apperrors.Operation("至少保留一个启用平台")
	}
	return nil
}

func (s *Service) normalizeLoginMethods(ctx context.Context, platformCode string, requests []platformfacade.LoginMethodSaveRequest) ([]domain.LoginMethod, error) {
	return s.normalizeLoginMethodsWithProviderLookup(ctx, platformCode, requests, s.repo.ListAvailableExternalProviderCodes)
}

func (s *Service) normalizeManagedLoginMethods(ctx context.Context, platformCode string, requests []platformfacade.LoginMethodSaveRequest) ([]domain.LoginMethod, error) {
	return s.normalizeLoginMethodsWithProviderLookup(ctx, platformCode, requests, s.repo.ListManagedExternalProviderCodes)
}

func (s *Service) normalizeLoginMethodsWithProviderLookup(ctx context.Context, platformCode string, requests []platformfacade.LoginMethodSaveRequest, listProviders func(context.Context, []string) ([]string, error)) ([]domain.LoginMethod, error) {
	if err := validateLoginMethodCount(len(requests)); err != nil {
		return nil, err
	}
	result := make([]domain.LoginMethod, 0, len(requests))
	seen := map[string]struct{}{}
	providerCodes := make([]string, 0, len(requests))
	seenProviders := make(map[string]struct{}, len(requests))
	for _, req := range requests {
		methodType := strings.ToUpper(strings.TrimSpace(req.MethodType))
		if methodType != domain.MethodPassword && methodType != domain.MethodPasskey && methodType != domain.MethodExternalOAuth {
			return nil, apperrors.Params("methodType格式错误")
		}
		providerCode := strings.ToLower(strings.TrimSpace(req.ProviderCode))
		if methodType == domain.MethodExternalOAuth {
			if providerCode == "" {
				return nil, apperrors.Params("providerCode不能为空")
			}
			if _, ok := seenProviders[providerCode]; !ok {
				seenProviders[providerCode] = struct{}{}
				providerCodes = append(providerCodes, providerCode)
			}
		} else {
			providerCode = ""
		}
		key := methodType + "\x00" + providerCode
		if _, ok := seen[key]; ok {
			return nil, apperrors.Params("登录方式重复")
		}
		seen[key] = struct{}{}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" || len([]rune(displayName)) > 128 {
			return nil, apperrors.Params("displayName长度错误")
		}
		result = append(result, domain.LoginMethod{
			PlatformCode:   platformCode,
			MethodType:     methodType,
			ProviderCode:   providerCode,
			DisplayName:    displayName,
			Icon:           strings.TrimSpace(req.Icon),
			SortOrder:      req.SortOrder,
			DisplayEnabled: req.DisplayEnabled,
			LoginEnabled:   req.LoginEnabled,
			MetadataJSON:   strings.TrimSpace(req.MetadataJSON),
		})
	}
	if len(providerCodes) == 0 {
		return result, nil
	}
	existingCodes, err := listProviders(ctx, providerCodes)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]struct{}, len(existingCodes))
	for _, code := range existingCodes {
		existing[strings.ToLower(strings.TrimSpace(code))] = struct{}{}
	}
	for _, code := range providerCodes {
		if _, ok := existing[code]; !ok {
			return nil, apperrors.Params("外部登录provider不存在")
		}
	}
	return result, nil
}

func validateLoginMethodCount(count int) error {
	if count > loginMethodMaxCount {
		return apperrors.Params("登录方式数量超过限制")
	}
	return nil
}

func validateSourceRuleCount(count int) error {
	if count > sourceRuleMaxCount {
		return apperrors.Params("来源规则数量超过限制")
	}
	return nil
}

func validateDefaultRoleCount(count int) error {
	if count > defaultRoleMaxCount {
		return apperrors.Operation("平台默认角色数量超过限制")
	}
	return nil
}

func disabledLoginMethods(before, after []domain.LoginMethod) []domain.LoginMethod {
	activeAfter := make(map[string]struct{}, len(after))
	for _, method := range after {
		if method.LoginEnabled {
			activeAfter[loginMethodKey(method)] = struct{}{}
		}
	}
	disabled := make([]domain.LoginMethod, 0)
	seen := map[string]struct{}{}
	for _, method := range before {
		if !method.LoginEnabled {
			continue
		}
		key := loginMethodKey(method)
		if _, stillActive := activeAfter[key]; stillActive {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		disabled = append(disabled, domain.LoginMethod{
			MethodType:   strings.ToUpper(strings.TrimSpace(method.MethodType)),
			ProviderCode: strings.ToLower(strings.TrimSpace(method.ProviderCode)),
		})
	}
	return disabled
}

func loginMethodKey(method domain.LoginMethod) string {
	methodType := strings.ToUpper(strings.TrimSpace(method.MethodType))
	providerCode := strings.ToLower(strings.TrimSpace(method.ProviderCode))
	if methodType != domain.MethodExternalOAuth {
		providerCode = ""
	}
	return methodType + "\x00" + providerCode
}

func preserveManagedLoginMethodMetadata(before, after []domain.LoginMethod) {
	metadata := make(map[string]string, len(before))
	for _, method := range before {
		metadata[loginMethodKey(method)] = method.MetadataJSON
	}
	for index := range after {
		if value, ok := metadata[loginMethodKey(after[index])]; ok {
			after[index].MetadataJSON = value
		}
	}
}

func preserveManagedSourceRuleMetadata(before, after []domain.SourceRule) {
	metadata := make(map[string]string, len(before))
	for _, rule := range before {
		metadata[sourceRuleKey(rule)] = rule.MetadataJSON
	}
	for index := range after {
		if value, ok := metadata[sourceRuleKey(after[index])]; ok {
			after[index].MetadataJSON = value
		}
	}
}

func managedLoginMethodsEqual(before, after []domain.LoginMethod) bool {
	if len(before) != len(after) {
		return false
	}
	current := make(map[string]domain.LoginMethod, len(before))
	for _, method := range before {
		current[loginMethodKey(method)] = method
	}
	for _, method := range after {
		existing, ok := current[loginMethodKey(method)]
		if !ok || existing.DisplayName != method.DisplayName || existing.Icon != method.Icon || existing.SortOrder != method.SortOrder || existing.DisplayEnabled != method.DisplayEnabled || existing.LoginEnabled != method.LoginEnabled || existing.MetadataJSON != method.MetadataJSON {
			return false
		}
	}
	return true
}

func managedSourceRulesEqual(before, after []domain.SourceRule) bool {
	if len(before) != len(after) {
		return false
	}
	current := make(map[string]domain.SourceRule, len(before))
	for _, rule := range before {
		current[sourceRuleKey(rule)] = rule
	}
	for _, rule := range after {
		existing, ok := current[sourceRuleKey(rule)]
		if !ok || existing.Priority != rule.Priority || existing.Status != rule.Status || existing.MetadataJSON != rule.MetadataJSON {
			return false
		}
	}
	return true
}

func sourceRuleKey(rule domain.SourceRule) string {
	return strings.ToUpper(strings.TrimSpace(rule.MatchType)) + "\x00" + strings.TrimSpace(rule.MatchValue)
}

func normalizeSourceRules(platformCode string, requests []platformfacade.SourceRuleSaveRequest) ([]domain.SourceRule, error) {
	if err := validateSourceRuleCount(len(requests)); err != nil {
		return nil, err
	}
	result := make([]domain.SourceRule, 0, len(requests))
	seen := map[string]struct{}{}
	for _, req := range requests {
		matchType := strings.ToUpper(strings.TrimSpace(req.MatchType))
		switch matchType {
		case domain.MatchClientID, domain.MatchRedirectHost, domain.MatchRedirectPrefix, domain.MatchHost, domain.MatchOrigin, domain.MatchRefererHost:
		default:
			return nil, apperrors.Params("matchType格式错误")
		}
		matchValue := strings.TrimSpace(req.MatchValue)
		if matchValue == "" || len([]rune(matchValue)) > 1024 {
			return nil, apperrors.Params("matchValue长度错误")
		}
		status, err := normalizeStatus(req.Status)
		if err != nil {
			return nil, err
		}
		key := matchType + "\x00" + matchValue
		if _, ok := seen[key]; ok {
			return nil, apperrors.Params("来源规则重复")
		}
		seen[key] = struct{}{}
		result = append(result, domain.SourceRule{
			PlatformCode: platformCode,
			MatchType:    matchType,
			MatchValue:   matchValue,
			Priority:     req.Priority,
			Status:       status,
			MetadataJSON: strings.TrimSpace(req.MetadataJSON),
		})
	}
	return result, nil
}

func (s *Service) normalizeDefaultRoles(ctx context.Context, platformCode string, requests []platformfacade.DefaultRoleSaveRequest) ([]domain.DefaultRole, error) {
	if err := validateDefaultRoleCount(len(requests)); err != nil {
		return nil, err
	}
	roleIDs := make([]int64, 0, len(requests))
	seen := map[int64]struct{}{}
	for _, req := range requests {
		if req.RoleID <= 0 {
			return nil, apperrors.Params("roleId格式错误")
		}
		if !req.AutoAssignEnabled {
			return nil, apperrors.Operation("默认角色必须允许自动分配")
		}
		if _, ok := seen[req.RoleID]; ok {
			continue
		}
		seen[req.RoleID] = struct{}{}
		roleIDs = append(roleIDs, req.RoleID)
	}
	safety, err := s.repo.ValidateDefaultRoles(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	safetyByID := make(map[int64]domain.RoleSafety, len(safety))
	for _, item := range safety {
		safetyByID[item.RoleID] = item
	}
	result := make([]domain.DefaultRole, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		item, ok := safetyByID[roleID]
		if !ok || !item.Exists || !item.Active || !item.AutoAssignable {
			return nil, apperrors.Operation("默认角色不可自动分配")
		}
		if hasHighRiskPermission(item.PermissionCodes) || hasHighRiskPermission(item.MenuPermissions) {
			return nil, apperrors.Operation("默认角色包含高风险权限，禁止自动分配")
		}
		result = append(result, domain.DefaultRole{
			PlatformCode:      platformCode,
			RoleID:            roleID,
			AutoAssignEnabled: true,
			Status:            domain.StatusActive,
		})
	}
	return result, nil
}

func hasHighRiskPermission(codes []string) bool {
	for _, code := range codes {
		normalized := strings.ToLower(strings.TrimSpace(code))
		if normalized == "*" || normalized == "system:*" || normalized == "admin:*" {
			return true
		}
		if strings.HasPrefix(normalized, "system:") || strings.HasPrefix(normalized, "admin:") {
			return true
		}
	}
	return false
}
