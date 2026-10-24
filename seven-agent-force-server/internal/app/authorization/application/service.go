package application

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const (
	authAudienceService                = "authorization-app"
	authIssuerService                  = "authorization-app"
	authorizationRoleSetMax            = 1000
	authorizationDerivedRelationSetMax = 10000
	authorizationAffectedUserPageSize  = 200

	stepUpActionRBACAssignUserRoles       = "RBAC_ASSIGN_USER_ROLES"
	stepUpActionRBACAssignRolePermissions = "RBAC_ASSIGN_ROLE_PERMISSIONS"
	stepUpActionRBACAssignRoleMenus       = "RBAC_ASSIGN_ROLE_MENUS"
	stepUpActionRBACAssignRoleDepts       = "RBAC_ASSIGN_ROLE_DEPTS"
	stepUpActionRBACAssignMenuPermissions = "RBAC_ASSIGN_MENU_PERMISSIONS"
	stepUpActionRBACGrantTempPermission   = "RBAC_GRANT_TEMP_PERMISSION"
	stepUpActionRBACRevokeTempPermission  = "RBAC_REVOKE_TEMP_PERMISSION"
	stepUpActionRBACExtendTempPermission  = "RBAC_EXTEND_TEMP_PERMISSION"
)

var authorizationRootCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{2,49}$`)

type Repository interface {
	FindUserAggregate(ctx context.Context, userID int64) (*domain.UserAggregate, error)
	ListUserRoles(ctx context.Context, userID int64) ([]domain.RoleRecord, error)
	ListUserPermissions(ctx context.Context, userID int64) ([]domain.PermissionRecord, error)
	ListUserMenus(ctx context.Context, userID int64) ([]domain.MenuRecord, error)
	ListUserOrganizations(ctx context.Context, userID int64) ([]domain.OrgRecord, error)
	ListUserDepartments(ctx context.Context, userID int64) ([]domain.DeptRecord, error)
	ListUserPosts(ctx context.Context, userID int64) ([]domain.PostRecord, error)
	ListRoleDeptIDs(ctx context.Context, roleIDs []int64) ([]int64, error)
	ListDeptHierarchyMap(ctx context.Context, deptIDs []int64) (map[int64]string, error)
	ListDeptIDsByHierarchies(ctx context.Context, hierarchies []string) (map[string][]int64, error)
	PageRoles(ctx context.Context, query authorizationfacade.RolePageQuery) ([]domain.RoleRecord, int64, error)
	ListRoleList(ctx context.Context) ([]domain.RoleRecord, error)
	FindRoleByID(ctx context.Context, roleID int64) (*domain.RoleRecord, error)
	LockRoleGrant(ctx context.Context, roleID int64) (*domain.RoleRecord, error)
	LockRoleGrants(ctx context.Context, roleIDs []int64) ([]domain.RoleRecord, error)
	TouchRoleGrantGuards(ctx context.Context, roleIDs []int64) error
	FindRoleGrantRequest(ctx context.Context, roleID int64, idempotencyKey string) (*domain.RoleGrantRequestRecord, error)
	CreateRoleGrantRequest(ctx context.Context, record domain.RoleGrantRequestRecord, operatorID int64) error
	UpdateRoleGrantDataScope(ctx context.Context, roleID int64, dataScope int, operatorID int64) error
	UpdateRoleGrantRevision(ctx context.Context, roleID, expectedRevision, nextRevision, operatorID int64) error
	UpdateRoleGrantRevisions(ctx context.Context, roles []domain.RoleRecord, operatorID int64) error
	CountRoleCodeExcludingID(ctx context.Context, roleID int64, code string) (int, error)
	CountRolesByIDs(ctx context.Context, roleIDs []int64) (int, error)
	CountAuthorizationRootRolesByIDs(ctx context.Context, roleIDs []int64) (int, error)
	LockSuperAdminInvariant(ctx context.Context, targetUserID int64) (domain.SuperAdminInvariantSnapshot, error)
	GetAuthorizationRootSecuritySnapshot(ctx context.Context) (*domain.AuthorizationRootSecuritySnapshot, error)
	BootstrapAuthorizationRoot(ctx context.Context, code, name string, initializedAt time.Time) (*domain.AuthorizationRootBootstrapResult, error)
	LockAuthorizationCreationGuard(ctx context.Context) error
	CountDeptsByIDs(ctx context.Context, deptIDs []int64) (int, error)
	ListPermissionCodesByRoleIDs(ctx context.Context, roleIDs []int64) ([]string, error)
	CreateRole(ctx context.Context, record domain.RoleRecord, operatorID int64) error
	UpdateRole(ctx context.Context, record domain.RoleRecord, operatorID int64) error
	DeleteRole(ctx context.Context, roleID int64, operatorID int64) error
	CountUserRoleReferences(ctx context.Context, roleID int64) (int, error)
	CountPostRoleReferences(ctx context.Context, roleID int64) (int, error)
	CountUserIDsByRoleID(ctx context.Context, roleID int64) (int, error)
	ListUserIDsByRoleIDsPage(ctx context.Context, roleIDs []int64, afterUserID int64, limit int) ([]int64, error)
	ListAllMenus(ctx context.Context) ([]domain.MenuRecord, error)
	ListMenus(ctx context.Context, enabledOnly bool) ([]domain.MenuRecord, error)
	FindMenuByID(ctx context.Context, menuID int64) (*domain.MenuRecord, error)
	LockMenuGrants(ctx context.Context, menuIDs []int64) ([]domain.MenuRecord, error)
	TouchMenuGrantGuards(ctx context.Context, menuIDs []int64) error
	CountMenuPermissionExcludingID(ctx context.Context, menuID int64, permission string) (int, error)
	CountMenuChildren(ctx context.Context, menuID int64) (int, error)
	CountRoleMenuReferences(ctx context.Context, menuID int64) (int, error)
	CountMenusByIDs(ctx context.Context, menuIDs []int64) (int, error)
	ListMenuPermissionCodes(ctx context.Context, menuIDs []int64) ([]string, error)
	CreateMenu(ctx context.Context, record domain.MenuRecord, operatorID int64) error
	UpdateMenu(ctx context.Context, record domain.MenuRecord, operatorID int64) error
	DeleteMenu(ctx context.Context, menuID int64, operatorID int64) error
	DeleteMenuPermissionsByMenuID(ctx context.Context, menuID int64) error
	ListRoleMenuIDs(ctx context.Context, roleID int64) ([]int64, error)
	ListRoleMenuIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error)
	ListDirectRolePermissionIDsByRoleIDs(ctx context.Context, roleIDs []int64) (map[int64][]int64, error)
	ListMenuPermissionIDs(ctx context.Context, menuIDs []int64) ([]int64, error)
	ListMenuPermissionIDsByMenuIDs(ctx context.Context, menuIDs []int64) (map[int64][]int64, error)
	ListPermissions(ctx context.Context, query authorizationfacade.PermissionQuery) ([]domain.PermissionRecord, error)
	PagePermissions(ctx context.Context, query authorizationfacade.PermissionPageQuery, filterFeatures bool, enabledFeatureCodes []string) ([]domain.PermissionRecord, int64, error)
	ListPermissionCodesByIDs(ctx context.Context, permissionIDs []int64) (map[int64]string, error)
	ListPermissionsByIDs(ctx context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error)
	FindPermissionByID(ctx context.Context, permissionID int64) (*domain.PermissionRecord, error)
	LockPermissionGrants(ctx context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error)
	TouchPermissionGrantGuards(ctx context.Context, permissionIDs []int64) error
	CountPermissionCodeExcludingID(ctx context.Context, permissionID int64, code string) (int, error)
	CountPermissionsByIDs(ctx context.Context, permissionIDs []int64) (int, error)
	CreatePermission(ctx context.Context, record domain.PermissionRecord, operatorID int64) error
	UpdatePermission(ctx context.Context, record domain.PermissionRecord, operatorID int64) error
	DeletePermission(ctx context.Context, permissionID int64, operatorID int64) error
	SoftDeleteUserPermissionsByPermissionID(ctx context.Context, permissionID int64, operatorID int64) error
	ListUserIDsByPermissionIDPage(ctx context.Context, permissionID, afterUserID int64, limit int) ([]int64, error)
	ListRoleIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error)
	ListMenuIDsByPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error)
	ListRoleIDsByMenuID(ctx context.Context, menuID int64) ([]int64, error)
	ListDeptIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error)
	ReplaceRoleDepts(ctx context.Context, roleID int64, deptIDs []int64, operatorID int64, nextID func() int64) error
	ReplaceMenuPermissions(ctx context.Context, menuID int64, permissionIDs []int64, operatorID int64, nextID func() int64) error
	ReplaceDerivedRolePermissionsBatch(ctx context.Context, assignments []domain.RolePermissionAssignment, operatorID int64, nextID func() int64) error
	ReplaceUserRoles(ctx context.Context, userID int64, roleIDs []int64, operatorID int64, nextID func() int64) error
	ReplaceRolePermissions(ctx context.Context, roleID int64, directPermissionIDs, menuPermissionIDs, menuIDs []int64, operatorID int64, nextID func() int64) error
	GrantTemporaryPermission(ctx context.Context, userID int64, permissionCode string, expireAt *time.Time, source, reason string, grantedBy int64, nextID func() int64) error
	RevokeTemporaryPermission(ctx context.Context, userID int64, permissionCode string) error
	ExtendTemporaryPermission(ctx context.Context, userID int64, permissionCode string, expireAt *time.Time, reason string) error
	ListUserTemporaryPermissions(ctx context.Context, userID int64) ([]domain.TemporaryPermissionRecord, error)
	ListExpiredTemporaryPermissionUserIDsPage(ctx context.Context, afterUserID int64, limit int) ([]int64, error)
	CleanupExpiredTemporaryPermissionsByUserIDs(ctx context.Context, userIDs []int64) error
	TemporaryPermissionStats(ctx context.Context) (*domain.TemporaryPermissionStats, error)
	ListPermissionCodes(ctx context.Context) ([]string, error)
	FindPermissionCodeByID(ctx context.Context, permissionID int64) (string, error)
	FindPermissionIDByCode(ctx context.Context, permissionCode string) (int64, error)
}

type Service struct {
	cfg           config.AuthorizationConfig
	cache         cacheinfra.Manager
	transactor    store.Transactor
	repository    Repository
	domain        *domain.Service
	idGen         *xid.Generator
	ssoTokens     ssofacade.TokenFacade
	ssoSessions   ssofacade.SessionFacade
	challenges    challengefacade.ChallengeInternalFacade
	proof         challengefacade.ProofTokenVerifier
	features      features.Set
	configScopes  authorizationfacade.RoleGrantConfigScopePort
	invalidations cachegovernancefacade.InvalidationRegistrar
}

type authorizationReadSnapshotContextKey struct{}

// BindRoleGrantConfigScopes binds the config module to the authorization-owned role grant policy.
func (s *Service) BindRoleGrantConfigScopes(port authorizationfacade.RoleGrantConfigScopePort) {
	if s != nil {
		s.configScopes = port
	}
}

// BindCacheInvalidations installs the governed, durable cache invalidation
// Facade after the cache-governance module has been composed. The service
// remains source-authoritative when governance is disabled; it never falls
// back to best-effort local deletion as a correctness mechanism.
func (s *Service) BindCacheInvalidations(registrar cachegovernancefacade.InvalidationRegistrar) {
	if s != nil {
		s.invalidations = registrar
	}
}

func NewService(
	cfg config.AuthorizationConfig,
	cache cacheinfra.Manager,
	transactor store.Transactor,
	repository Repository,
	domainService *domain.Service,
	idGen *xid.Generator,
	ssoTokens ssofacade.TokenFacade,
	ssoSessions ssofacade.SessionFacade,
	challenges challengefacade.ChallengeInternalFacade,
	proof challengefacade.ProofTokenVerifier,
	featureSets ...features.Set,
) *Service {
	var featureSet features.Set
	if len(featureSets) > 0 {
		featureSet = featureSets[0]
	}
	return &Service{
		cfg:         cfg,
		cache:       cache,
		transactor:  transactor,
		repository:  repository,
		domain:      domainService,
		idGen:       idGen,
		ssoTokens:   ssoTokens,
		ssoSessions: ssoSessions,
		challenges:  challenges,
		proof:       proof,
		features:    featureSet,
	}
}

func (s *Service) BuildContextFromAccessToken(ctx context.Context, accessToken string, source string) (*securitycontext.UserContext, error) {
	if s.ssoTokens == nil {
		return nil, apperrors.System("authorization sso token facade未配置")
	}
	principal, err := s.ssoTokens.ValidateAccessToken(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return nil, err
	}
	return s.BuildUserContext(ctx, principal.UserID, principal.SessionID, principal.IssuedAt, principal.ExpiresAt, source)
}

func (s *Service) BuildContextFromSession(ctx context.Context, sessionID string, source string) (*securitycontext.UserContext, error) {
	if s.ssoSessions == nil {
		return nil, apperrors.System("authorization sso session facade未配置")
	}
	session, err := s.ssoSessions.ResolveActiveSessionRecord(ctx, strings.TrimSpace(sessionID))
	if err != nil {
		return nil, err
	}
	if session == nil || session.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	return s.BuildUserContext(ctx, session.UserID, session.SessionID, session.LoginAt, session.ExpiresAt, source)
}

func (s *Service) BuildUserContext(ctx context.Context, userID int64, sessionID string, issuedAt, expiresAt *time.Time, source string) (*securitycontext.UserContext, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	snapshotActive, _ := ctx.Value(authorizationReadSnapshotContextKey{}).(bool)
	if !snapshotActive {
		snapshotter, ok := s.transactor.(store.Snapshotter)
		if !ok || !snapshotter.Enabled() {
			return nil, apperrors.System("授权上下文一致性快照能力未配置")
		}
		var userContext *securitycontext.UserContext
		err := snapshotter.WithinReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
			snapshotCtx = context.WithValue(snapshotCtx, authorizationReadSnapshotContextKey{}, true)
			var loadErr error
			userContext, loadErr = s.loadAuthorizationContextSnapshot(snapshotCtx, userID)
			return loadErr
		})
		if err != nil {
			return nil, err
		}
		return withAuthorizationSession(userContext, sessionID, issuedAt, expiresAt, source), nil
	}
	return s.buildAuthorizationContextSource(ctx, userID)
}

// loadAuthorizationContextSnapshot runs inside the source's consistent
// read-only snapshot. A currently active temporary grant makes this request
// source-only: authorization may never outlive the grant's expiry through a
// cache TTL.
func (s *Service) loadAuthorizationContextSnapshot(ctx context.Context, userID int64) (*securitycontext.UserContext, error) {
	governed, request, enabled := s.authorizationContextCacheRequest(userID)
	if !enabled {
		return s.buildAuthorizationContextSource(ctx, userID)
	}
	var cached securitycontext.UserContext
	_, err := governed.GetOrLoadClassifiedWithPreflight(ctx, request, &cached, func(preflightCtx context.Context) (bool, error) {
		if eligibilityErr := s.requireAuthorizationUserAvailable(preflightCtx, userID); eligibilityErr != nil {
			return false, eligibilityErr
		}
		activeTemporaryGrant, grantErr := s.hasActiveTemporaryPermission(preflightCtx, userID)
		return !activeTemporaryGrant, grantErr
	}, func(loadCtx context.Context) (cachepolicy.CacheableValue, error) {
		loaded, loadErr := s.buildAuthorizationContextSource(loadCtx, userID)
		if loadErr != nil {
			return cachepolicy.CacheableValue{}, loadErr
		}
		return cachepolicy.CacheableValue{Value: loaded, Cacheable: true}, nil
	})
	if err == nil && cached.UserID > 0 {
		return &cached, nil
	}
	// Cache/Governance uncertainty must never decide authorization. The read
	// snapshot is still open, so reload the authority instead of accepting an
	// older L1/L2 candidate.
	return s.buildAuthorizationContextSource(ctx, userID)
}

func (s *Service) buildAuthorizationContextSource(ctx context.Context, userID int64) (*securitycontext.UserContext, error) {

	aggregate, err := s.repository.FindUserAggregate(ctx, userID)
	if err != nil {
		return nil, err
	}
	if aggregate == nil {
		return nil, apperrors.Unauthorized("当前用户不存在或已失效")
	}
	if !aggregate.Enabled || aggregate.Locked {
		return nil, apperrors.Unauthorized("当前用户不存在或已失效")
	}
	roles, err := s.repository.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	permissions, err := s.repository.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	permissionCodes := s.enabledPermissionCodes(permissions)
	orgs, err := s.repository.ListUserOrganizations(ctx, userID)
	if err != nil {
		return nil, err
	}
	depts, err := s.repository.ListUserDepartments(ctx, userID)
	if err != nil {
		return nil, err
	}
	posts, err := s.repository.ListUserPosts(ctx, userID)
	if err != nil {
		return nil, err
	}
	roleIDs := make([]int64, 0, len(roles))
	roleCodes := make([]string, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.RoleID)
		roleCodes = append(roleCodes, role.Code)
	}
	customDeptIDs, err := s.repository.ListRoleDeptIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	deptIDs := make([]int64, 0, len(depts))
	deptHierarchies := make(map[int64]string, len(depts))
	for _, dept := range depts {
		deptIDs = append(deptIDs, dept.DeptID)
		deptHierarchies[dept.DeptID] = dept.Hierarchy
	}
	allDeptIDsByHierarchy, err := s.repository.ListDeptIDsByHierarchies(ctx, valuesOfMap(deptHierarchies))
	if err != nil {
		return nil, err
	}
	orgIDs := make([]int64, 0, len(orgs))
	postIDs := make([]int64, 0, len(posts))
	postCodes := make([]string, 0, len(posts))
	for _, org := range orgs {
		orgIDs = append(orgIDs, org.OrgID)
	}
	for _, post := range posts {
		postIDs = append(postIDs, post.PostID)
		postCodes = append(postCodes, post.Code)
	}
	dataScope := s.domain.EffectiveDataScope(roles, customDeptIDs, deptIDs, orgIDs, deptHierarchies, allDeptIDsByHierarchy)
	userContext := &securitycontext.UserContext{
		UserID:           aggregate.UserID,
		Username:         aggregate.Username,
		Nickname:         aggregate.Nickname,
		RoleIDs:          uniqueInt64(roleIDs),
		Roles:            s.domain.NormalizeCodes(roleCodes),
		Permissions:      s.domain.NormalizeCodes(permissionCodes),
		PrimaryOrgID:     aggregate.PrimaryOrgID,
		OrgIDs:           uniqueInt64(orgIDs),
		PrimaryDeptID:    aggregate.PrimaryDeptID,
		DeptIDs:          uniqueInt64(deptIDs),
		PostIDs:          uniqueInt64(postIDs),
		PostCodes:        s.domain.NormalizeCodes(postCodes),
		DataScopeDeptIDs: uniqueInt64(dataScope.DeptIDs),
		DataScopeOrgIDs:  uniqueInt64(dataScope.OrgIDs),
		DataScopeType:    dataScope.ScopeType,
		AuthVersion:      0,
		SessionVersion:   1,
		IsAdmin:          s.domain.IsAdmin(roles),
		IsAnonymous:      false,
	}
	userContext.AuthVersion = authorizationSnapshotVersion(userContext)
	return userContext, nil
}

func (s *Service) GetLoginUser(ctx context.Context, request authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	if request.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	return s.GetUserVO(ctx, request.UserID)
}

func (s *Service) GetLoginUserPermitNull(ctx context.Context, request authorizationfacade.RequestScope) (*authorizationfacade.UserVO, error) {
	if request.UserID <= 0 {
		return nil, nil
	}
	return s.GetUserVO(ctx, request.UserID)
}

func (s *Service) GetLoginUserID(ctx context.Context, request authorizationfacade.RequestScope) (int64, error) {
	if !s.IsLogin(ctx, request) {
		return 0, apperrors.Unauthorized("未登录")
	}
	return request.UserID, nil
}

func (s *Service) GetLoginUsername(ctx context.Context, request authorizationfacade.RequestScope) (string, error) {
	if !s.IsLogin(ctx, request) {
		return "", apperrors.Unauthorized("未登录")
	}
	if strings.TrimSpace(request.Username) != "" {
		return strings.TrimSpace(request.Username), nil
	}
	user, err := s.GetUserVO(ctx, request.UserID)
	if err != nil || user == nil {
		return "", err
	}
	return user.Username, nil
}

func (s *Service) IsLogin(ctx context.Context, request authorizationfacade.RequestScope) bool {
	_ = ctx
	return request.UserID > 0
}

func (s *Service) IsAdmin(ctx context.Context, request authorizationfacade.RequestScope) bool {
	_ = ctx
	if request.UserID <= 0 {
		return false
	}
	user, err := s.GetUserVO(context.Background(), request.UserID)
	return err == nil && user != nil && user.IsAdmin
}

func (s *Service) IsAuthorizationRootUser(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	roles, err := s.repository.ListUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	return s.domain.IsAdmin(roles), nil
}

func (s *Service) IsCurrentUser(ctx context.Context, request authorizationfacade.RequestScope, userID int64) bool {
	_ = ctx
	return request.UserID > 0 && request.UserID == userID
}

func (s *Service) IsAdminOrCurrentUser(ctx context.Context, request authorizationfacade.RequestScope, userID int64) bool {
	return s.IsCurrentUser(ctx, request, userID) || s.IsAdmin(ctx, request)
}

func (s *Service) GetUserVO(ctx context.Context, userID int64) (*authorizationfacade.UserVO, error) {
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-facade")
	if err != nil {
		return nil, err
	}
	aggregate, err := s.repository.FindUserAggregate(ctx, userID)
	if err != nil || aggregate == nil {
		return nil, err
	}
	orgs, _ := s.repository.ListUserOrganizations(ctx, userID)
	depts, _ := s.repository.ListUserDepartments(ctx, userID)
	posts, _ := s.repository.ListUserPosts(ctx, userID)
	roles, _ := s.repository.ListUserRoles(ctx, userID)
	roleNames := make([]string, 0, len(roles))
	roleCodes := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Name)
		roleCodes = append(roleCodes, role.Code)
	}
	orgCodes, orgNames := extractOrgValues(orgs)
	deptCodes, deptNames := extractDeptValues(depts)
	postCodes, postNames := extractPostValues(posts)
	return &authorizationfacade.UserVO{
		UserID:        aggregate.UserID,
		Username:      aggregate.Username,
		Nickname:      aggregate.Nickname,
		Avatar:        aggregate.Avatar,
		Email:         aggregate.Email,
		Phone:         aggregate.Phone,
		IsAdmin:       userContext.IsAdmin,
		RoleCodes:     s.domain.NormalizeCodes(roleCodes),
		RoleNames:     s.domain.NormalizeCodes(roleNames),
		Permissions:   userContext.Permissions,
		OrgIDs:        userContext.OrgIDs,
		OrgCodes:      orgCodes,
		OrgNames:      orgNames,
		DeptIDs:       userContext.DeptIDs,
		DeptCodes:     deptCodes,
		DeptNames:     deptNames,
		PostIDs:       userContext.PostIDs,
		PostCodes:     postCodes,
		PostNames:     postNames,
		PrimaryOrgID:  aggregate.PrimaryOrgID,
		PrimaryDeptID: aggregate.PrimaryDeptID,
		PrimaryPostID: aggregate.PrimaryPostID,
	}, nil
}

func (s *Service) RefreshUserPermissionCache(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	// Deprecated ABI shim. DG6.1 authorization snapshots are generation-gated;
	// a caller-local delete has no cross-instance correctness meaning and must
	// not be mistaken for an authorization revocation protocol.
	_ = ctx
	return nil
}

func (s *Service) GetUserPermissionsByModule(ctx context.Context, request authorizationfacade.RequestScope, module string) ([]string, error) {
	if request.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	permissions, err := s.repository.ListUserPermissions(ctx, request.UserID)
	if err != nil {
		return nil, err
	}
	return s.domain.FilterPermissionsByModule(s.enabledPermissionCodes(permissions), module), nil
}

func (s *Service) GetUserPermissions(ctx context.Context, userID int64) ([]string, error) {
	items, err := s.repository.ListUserPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.domain.NormalizeCodes(s.enabledPermissionCodes(items)), nil
}

func (s *Service) ResolvePermissionCode(ctx context.Context, permissionID int64) (string, error) {
	if permissionID <= 0 {
		return "", apperrors.Params("permissionId不能为空")
	}
	return s.repository.FindPermissionCodeByID(ctx, permissionID)
}

func (s *Service) GetUserRoles(ctx context.Context, userID int64) ([]string, error) {
	items, err := s.repository.ListUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, item.Code)
	}
	return s.domain.NormalizeCodes(values), nil
}

func (s *Service) HasPermission(ctx context.Context, userID int64, permission string) (bool, error) {
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-facade")
	if err != nil {
		return false, err
	}
	if userContext.IsAdmin {
		return true, nil
	}
	permission = strings.TrimSpace(permission)
	for _, item := range userContext.Permissions {
		if permissionMatches(item, permission) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) HasRole(ctx context.Context, userID int64, role string) (bool, error) {
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-facade")
	if err != nil {
		return false, err
	}
	role = strings.TrimSpace(role)
	for _, item := range userContext.Roles {
		if item == role {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) GetUserDataScope(ctx context.Context, userID int64) (*authorizationfacade.UserDataScopeVO, error) {
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-facade")
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.UserDataScopeVO{
		UserID:    userID,
		DeptIDs:   userContext.DataScopeDeptIDs,
		OrgIDs:    userContext.DataScopeOrgIDs,
		ScopeType: string(userContext.DataScopeType),
	}, nil
}

func (s *Service) ValidatePostRoleAssignment(ctx context.Context, userID, postID, roleID int64) (bool, error) {
	return s.ValidatePostRoleAssignments(ctx, userID, postID, []int64{roleID})
}

func (s *Service) ValidatePostRoleAssignments(ctx context.Context, userID, postID int64, roleIDs []int64) (bool, error) {
	_ = postID
	roleIDs = uniqueInt64(roleIDs)
	if userID <= 0 || len(roleIDs) == 0 || len(roleIDs) > authorizationRoleSetMax {
		return false, nil
	}
	count, err := s.repository.CountRolesByIDs(ctx, roleIDs)
	if err != nil || count != len(roleIDs) {
		return false, err
	}
	rootCount, err := s.repository.CountAuthorizationRootRolesByIDs(ctx, roleIDs)
	if err != nil || rootCount > 0 {
		return false, err
	}
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-facade")
	if err != nil {
		return false, err
	}
	if userContext.IsAdmin {
		return true, nil
	}
	roles, err := s.repository.ListUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	allowed := make(map[int64]struct{}, len(roles))
	for _, item := range roles {
		allowed[item.RoleID] = struct{}{}
	}
	for _, roleID := range roleIDs {
		if _, ok := allowed[roleID]; !ok {
			return false, nil
		}
	}
	return true, nil
}

// LockAndValidatePostRoleAssignments validates active non-root role parents
// under ascending role guards inside the caller's consistent transaction.
func (s *Service) LockAndValidatePostRoleAssignments(ctx context.Context, userID, postID int64, roleIDs []int64) (bool, error) {
	ids := uniqueInt64(roleIDs)
	if userID <= 0 || postID <= 0 || len(ids) == 0 || len(ids) > authorizationRoleSetMax {
		return false, nil
	}
	if !store.InConsistentTransaction(ctx) {
		return false, apperrors.System("岗位角色父记录校验必须运行在一致性事务内")
	}
	if err := s.repository.LockAuthorizationCreationGuard(ctx); err != nil {
		return false, err
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	roles, err := s.repository.LockRoleGrants(ctx, ids)
	if err != nil {
		return false, err
	}
	if len(roles) != len(ids) {
		return false, nil
	}
	for index, role := range roles {
		if role.RoleID != ids[index] || role.Status != 0 || role.IsAuthorizationRoot() {
			return false, nil
		}
	}
	if err := s.repository.TouchRoleGrantGuards(ctx, ids); err != nil {
		return false, err
	}
	userContext, err := s.BuildUserContext(ctx, userID, "", nil, nil, "auth-post-role-guard")
	if err != nil {
		return false, err
	}
	if userContext.IsAdmin {
		return true, nil
	}
	currentRoles, err := s.repository.ListUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}
	allowed := make(map[int64]struct{}, len(currentRoles))
	for _, role := range currentRoles {
		allowed[role.RoleID] = struct{}{}
	}
	for _, roleID := range ids {
		if _, ok := allowed[roleID]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func (s *Service) ValidateUserPostAssignment(ctx context.Context, userID, postID int64) (bool, error) {
	posts, err := s.repository.ListUserPosts(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, item := range posts {
		if item.PostID == postID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) PageRoles(ctx context.Context, query authorizationfacade.RolePageQuery) (*authorizationfacade.RolePageVO, error) {
	current, size := normalizePage(query.Current, query.Size)
	query.Current = current
	query.Size = size
	records, total, err := s.repository.PageRoles(ctx, query)
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.RolePageVO{
		Records: toRoleVOs(records),
		Total:   total,
		Size:    size,
		Current: current,
	}, nil
}

func (s *Service) GetRoleList(ctx context.Context) ([]authorizationfacade.RoleVO, error) {
	items, err := s.repository.ListRoleList(ctx)
	if err != nil {
		return nil, err
	}
	return toRoleVOs(items), nil
}

func (s *Service) GetRole(ctx context.Context, roleID int64) (*authorizationfacade.RoleVO, error) {
	if roleID <= 0 {
		return nil, apperrors.Params("roleId不能为空")
	}
	record, err := s.repository.FindRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperrors.NotFound("角色不存在")
	}
	vo := toRoleVO(*record)
	return &vo, nil
}

func (s *Service) GetRootSecurityStatus(ctx context.Context) (*authorizationfacade.RoleSecurityStatusVO, error) {
	snapshot, err := s.repository.GetAuthorizationRootSecuritySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot == nil || !snapshot.Role.IsAuthorizationRoot() {
		return nil, apperrors.System("授权安全根不存在")
	}
	result := &authorizationfacade.RoleSecurityStatusVO{
		RootRoleID:         snapshot.Role.RoleID,
		RootRoleCode:       snapshot.Role.Code,
		ActiveDirectAdmins: snapshot.ActiveUserCount,
		MinimumRequired:    1,
		RecommendedMinimum: 2,
		Health:             "HEALTHY",
		Warnings:           []string{},
	}
	if snapshot.ActiveUserCount < result.RecommendedMinimum {
		result.Health = "LOW_REDUNDANCY"
		result.Warnings = []string{"ROOT_ADMIN_LOW_REDUNDANCY"}
	}
	return result, nil
}

func (s *Service) BootstrapAuthorizationRoot(ctx context.Context, command authorizationfacade.BootstrapAuthorizationRootCommand) (*authorizationfacade.BootstrapAuthorizationRootResult, error) {
	code := strings.TrimSpace(command.Code)
	name := strings.TrimSpace(command.Name)
	if !authorizationRootCodePattern.MatchString(code) {
		return nil, apperrors.Params("超级管理员角色编码格式无效")
	}
	if name == "" {
		return nil, apperrors.Params("超级管理员角色名称不能为空")
	}
	result, err := s.repository.BootstrapAuthorizationRoot(ctx, code, name, command.InitializedAt.UTC())
	if err != nil {
		return nil, err
	}
	if result == nil || !result.Role.IsAuthorizationRoot() {
		return nil, apperrors.System("授权安全根不存在")
	}
	return &authorizationfacade.BootstrapAuthorizationRootResult{
		Role:               toRoleVO(result.Role),
		AlreadyInitialized: result.AlreadyInitialized,
	}, nil
}

func (s *Service) CreateRole(ctx context.Context, command authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	record, err := s.buildRoleRecord(command, true)
	if err != nil {
		return nil, err
	}
	if record.IsSystem() {
		return nil, apperrors.Operation("普通管理接口不能创建SYSTEM角色")
	}
	record.RoleID = s.nextID()
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		if err := s.ensureRoleCodeUnique(txCtx, record.RoleID, record.Code); err != nil {
			return err
		}
		return s.repository.CreateRole(txCtx, record, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, record.RoleID)
}

func (s *Service) UpdateRole(ctx context.Context, command authorizationfacade.RoleCommand) (*authorizationfacade.RoleVO, error) {
	roleID := command.ID
	if roleID <= 0 {
		return nil, apperrors.Params("角色ID不能为空")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		existing, err := s.repository.LockRoleGrant(txCtx, roleID)
		if err != nil {
			return err
		}
		if existing == nil {
			return apperrors.NotFound("角色不存在")
		}
		record, err := s.buildRoleRecord(command, false)
		if err != nil {
			return err
		}
		record.RoleID = roleID
		record.SystemKey = existing.SystemKey
		if existing.DataScope != record.DataScope {
			return apperrors.Operation("角色数据范围请通过统一授权接口修改")
		}
		if violation := existing.ProtectedMutation(record); violation != domain.RoleProtectionNone {
			return systemRoleProtectionError(violation)
		}
		if existing.IsActiveSuperAdmin() && !record.IsActiveSuperAdmin() {
			snapshot, err := s.repository.LockSuperAdminInvariant(txCtx, 0)
			if err != nil {
				return err
			}
			if snapshot.WouldRemoveActiveRole(*existing, record) {
				return lastSuperAdminError()
			}
		}
		if err := s.ensureRoleCodeUnique(txCtx, record.RoleID, record.Code); err != nil {
			return err
		}
		return s.repository.UpdateRole(txCtx, record, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetRole(ctx, roleID)
}

func (s *Service) DeleteRole(ctx context.Context, roleID int64, operatorID int64) error {
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		existing, _, err := s.lockRoleMenuMutation(txCtx, roleID, nil)
		if err != nil {
			return err
		}
		if existing.IsSystem() {
			return apperrors.Operation("SYSTEM角色不可删除")
		}
		if existing.IsActiveSuperAdmin() {
			snapshot, err := s.repository.LockSuperAdminInvariant(txCtx, 0)
			if err != nil {
				return err
			}
			deleted := *existing
			deleted.Status = 1
			if snapshot.WouldRemoveActiveRole(*existing, deleted) {
				return lastSuperAdminError()
			}
		}
		userCount, err := s.repository.CountUserRoleReferences(txCtx, roleID)
		if err != nil {
			return err
		}
		if userCount > 0 {
			return apperrors.Operation("该角色已分配给用户，无法删除")
		}
		postCount, err := s.repository.CountPostRoleReferences(txCtx, roleID)
		if err != nil {
			return err
		}
		if postCount > 0 {
			return apperrors.Operation("该角色已分配给岗位，无法删除")
		}
		return s.repository.DeleteRole(txCtx, roleID, operatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetRoleDeptIDs(ctx context.Context, roleID int64) (*authorizationfacade.RoleDeptIDsVO, error) {
	if roleID <= 0 {
		return nil, apperrors.Params("roleId不能为空")
	}
	role, err := s.repository.FindRoleByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, apperrors.NotFound("角色不存在")
	}
	deptIDs, err := s.repository.ListDeptIDsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.RoleDeptIDsVO{RoleID: roleID, DeptIDs: uniqueInt64(deptIDs)}, nil
}

func (s *Service) AssignRoleDepts(ctx context.Context, command authorizationfacade.AssignRoleDeptsCommand) error {
	roleID := command.RoleID
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	deptIDs := uniqueInt64([]int64(command.DeptIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignRoleDepts, roleDeptAssignmentBinding(roleID, deptIDs)); err != nil {
		return err
	}
	role, err := s.repository.FindRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return apperrors.NotFound("角色不存在")
	}
	if role.IsAuthorizationRoot() {
		return apperrors.Operation("授权安全根的数据范围由系统管理")
	}
	if role.DataScope != 2 && len(deptIDs) > 0 {
		return apperrors.Params("只有自定数据权限角色可以绑定部门范围")
	}
	if err := s.ensureDeptsExist(ctx, deptIDs); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		locked, err := s.repository.LockRoleGrant(txCtx, roleID)
		if err != nil {
			return err
		}
		if locked == nil {
			return apperrors.NotFound("角色不存在")
		}
		if locked.IsAuthorizationRoot() {
			return apperrors.Operation("授权安全根的数据范围由系统管理")
		}
		if locked.DataScope != 2 && len(deptIDs) > 0 {
			return apperrors.Params("只有自定数据权限角色可以绑定部门范围")
		}
		if err := s.repository.ReplaceRoleDepts(txCtx, roleID, deptIDs, command.OperatorID, s.nextID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevision(txCtx, roleID, locked.GrantRevision, locked.GrantRevision+1, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetRoleMenuIDs(ctx context.Context, roleID int64) ([]int64, error) {
	if roleID <= 0 {
		return nil, apperrors.Params("roleId不能为空")
	}
	selectedIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return nil, err
	}
	menus, err := s.repository.ListAllMenus(ctx)
	if err != nil {
		return nil, err
	}
	return intersectIDs(selectedIDs, menuIDs(s.enabledMenus(menus))), nil
}

func (s *Service) GetRoleMenuTree(ctx context.Context, roleID int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	allMenus, err := s.repository.ListAllMenus(ctx)
	if err != nil {
		return nil, err
	}
	allMenus = s.enabledMenus(allMenus)
	selectedIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return nil, err
	}
	selected := make(map[int64]struct{}, len(selectedIDs))
	for _, item := range selectedIDs {
		selected[item] = struct{}{}
	}
	nodes := make([]authorizationfacade.MenuTreeNodeVO, 0, len(allMenus))
	for _, item := range allMenus {
		_, checked := selected[item.MenuID]
		nodes = append(nodes, authorizationfacade.MenuTreeNodeVO{
			ID:          item.MenuID,
			MenuID:      item.MenuID,
			ParentID:    item.ParentID,
			SortOrder:   item.SortOrder,
			Name:        item.Name,
			Path:        item.Path,
			Component:   item.Component,
			Type:        item.Type,
			Permission:  item.Permission,
			FeatureCode: item.FeatureCode,
			Icon:        item.Icon,
			Status:      item.Status,
			Visible:     item.Visible,
			IsFrame:     item.IsFrame,
			IsCache:     item.IsCache,
			Remark:      item.Remark,
			Checked:     checked,
			CreateTime:  item.CreateTime,
			UpdateTime:  item.UpdateTime,
		})
	}
	return buildMenuTree(nodes), nil
}

func (s *Service) lockRoleMenuMutation(ctx context.Context, roleID int64, requestedMenuIDs []int64) (*domain.RoleRecord, []int64, error) {
	observedMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}
	guardMenuIDs := uniqueInt64(append(append([]int64{}, observedMenuIDs...), requestedMenuIDs...))
	if len(guardMenuIDs) > authorizationRoleSetMax {
		return nil, nil, apperrors.Operation("角色菜单关系数量超过单次授权上限")
	}
	sort.Slice(guardMenuIDs, func(i, j int) bool { return guardMenuIDs[i] < guardMenuIDs[j] })
	lockedMenus, err := s.repository.LockMenuGrants(ctx, guardMenuIDs)
	if err != nil {
		return nil, nil, err
	}
	if len(lockedMenus) != len(guardMenuIDs) {
		return nil, nil, apperrors.ObjectState("角色菜单关系已变化，请重试")
	}
	if err := s.repository.TouchMenuGrantGuards(ctx, guardMenuIDs); err != nil {
		return nil, nil, err
	}
	lockedRole, err := s.repository.LockRoleGrant(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}
	if lockedRole == nil {
		return nil, nil, apperrors.NotFound("角色不存在")
	}
	currentMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return nil, nil, err
	}
	guarded := make(map[int64]struct{}, len(guardMenuIDs))
	for _, menuID := range guardMenuIDs {
		guarded[menuID] = struct{}{}
	}
	for _, menuID := range uniqueInt64(currentMenuIDs) {
		if _, ok := guarded[menuID]; !ok {
			return nil, nil, apperrors.ObjectState("角色菜单关系已并发变化，请重试")
		}
	}
	return lockedRole, uniqueInt64(currentMenuIDs), nil
}

func (s *Service) AssignRoleMenus(ctx context.Context, command authorizationfacade.AssignRoleMenusCommand) error {
	roleID := command.RoleID
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	normalizedMenuIDs := uniqueInt64([]int64(command.MenuIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignRoleMenus, roleMenuAssignmentBinding(roleID, normalizedMenuIDs)); err != nil {
		return err
	}
	role, err := s.repository.FindRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return apperrors.NotFound("角色不存在")
	}
	if role.IsAuthorizationRoot() {
		return apperrors.Operation("授权安全根的菜单与权限由系统管理")
	}
	if err := s.ensureMenusExist(ctx, normalizedMenuIDs); err != nil {
		return err
	}
	if err := s.ensureMenuFeaturesEnabled(ctx, normalizedMenuIDs); err != nil {
		return err
	}
	menuPermissionCodes, err := s.repository.ListMenuPermissionCodes(ctx, normalizedMenuIDs)
	if err != nil {
		return err
	}
	requestedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, normalizedMenuIDs)
	if err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionIDs(ctx, command.OperatorID, requestedPermissionIDs); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, menuPermissionCodes); err != nil {
		return err
	}
	observedMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return err
	}
	observedDerivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, uniqueInt64(append(append([]int64{}, observedMenuIDs...), normalizedMenuIDs...)))
	if err != nil {
		return err
	}
	observedDirectPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(ctx, []int64{roleID})
	if err != nil {
		return err
	}
	guardPermissionIDs := uniqueInt64(append(append([]int64{}, observedDerivedPermissionIDs...), observedDirectPermissions[roleID]...))
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		lockedPermissions, err := s.lockPermissionParents(txCtx, guardPermissionIDs)
		if err != nil {
			return err
		}
		locked, existingMenuIDs, err := s.lockRoleMenuMutation(txCtx, roleID, normalizedMenuIDs)
		if err != nil {
			return err
		}
		if err := s.ensureMenuFeaturesEnabled(txCtx, normalizedMenuIDs); err != nil {
			return err
		}
		currentMenuPermissionIDs, err := s.repository.ListMenuPermissionIDs(txCtx, normalizedMenuIDs)
		if err != nil {
			return err
		}
		currentMenuPermissionCodes, err := s.repository.ListMenuPermissionCodes(txCtx, normalizedMenuIDs)
		if err != nil {
			return err
		}
		if err := s.validateLockedPermissionGrant(txCtx, command.OperatorID, lockedPermissions, currentMenuPermissionIDs); err != nil {
			return err
		}
		if err := s.ensureOperatorCanGrantPermissionCodes(txCtx, command.OperatorID, currentMenuPermissionCodes); err != nil {
			return err
		}
		directPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(txCtx, []int64{roleID})
		if err != nil {
			return err
		}
		disabledMenuIDs, err := s.disabledMenuIDs(txCtx, existingMenuIDs)
		if err != nil {
			return err
		}
		nextMenuIDs := uniqueInt64(append(append([]int64{}, normalizedMenuIDs...), disabledMenuIDs...))
		derivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(txCtx, nextMenuIDs)
		if err != nil {
			return err
		}
		if !idsWithinGuard(derivedPermissionIDs, guardPermissionIDs) || !idsWithinGuard(directPermissions[roleID], guardPermissionIDs) {
			return apperrors.ObjectState("角色权限关系已并发变化，请重试")
		}
		if err := s.repository.ReplaceRolePermissions(txCtx, roleID, directPermissions[roleID], derivedPermissionIDs, nextMenuIDs, command.OperatorID, s.nextID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevision(txCtx, roleID, locked.GrantRevision, locked.GrantRevision+1, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) AssignRolePermissions(ctx context.Context, command authorizationfacade.AssignRolePermissionsCommand) error {
	roleID := command.RoleID
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	menuIDs := uniqueInt64([]int64(command.MenuIDs))
	permissionIDs := uniqueInt64([]int64(command.PermissionIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignRolePermissions, rolePermissionAssignmentBinding(roleID, permissionIDs, menuIDs)); err != nil {
		return err
	}
	role, err := s.repository.FindRoleByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role == nil {
		return apperrors.NotFound("角色不存在")
	}
	if role.IsAuthorizationRoot() {
		return apperrors.Operation("授权安全根的菜单与权限由系统管理")
	}
	if err := s.ensureMenusExist(ctx, menuIDs); err != nil {
		return err
	}
	if err := s.ensurePermissionsExist(ctx, permissionIDs); err != nil {
		return err
	}
	if err := s.ensureMenuFeaturesEnabled(ctx, menuIDs); err != nil {
		return err
	}
	if err := s.ensurePermissionFeaturesEnabled(ctx, permissionIDs); err != nil {
		return err
	}
	menuPermissionCodes, err := s.repository.ListMenuPermissionCodes(ctx, menuIDs)
	if err != nil {
		return err
	}
	requestedMenuPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, menuIDs)
	if err != nil {
		return err
	}
	allPermissionIDs := uniqueInt64(append(append([]int64{}, permissionIDs...), requestedMenuPermissionIDs...))
	if err := s.ensureOperatorCanGrantPermissionIDs(ctx, command.OperatorID, allPermissionIDs); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, menuPermissionCodes); err != nil {
		return err
	}
	observedMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, roleID)
	if err != nil {
		return err
	}
	observedDerivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, uniqueInt64(append(append([]int64{}, observedMenuIDs...), menuIDs...)))
	if err != nil {
		return err
	}
	observedDirectPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(ctx, []int64{roleID})
	if err != nil {
		return err
	}
	guardPermissionIDs := uniqueInt64(append(append(append([]int64{}, permissionIDs...), observedDerivedPermissionIDs...), observedDirectPermissions[roleID]...))
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		lockedPermissions, err := s.lockPermissionParents(txCtx, guardPermissionIDs)
		if err != nil {
			return err
		}
		locked, existingMenuIDs, err := s.lockRoleMenuMutation(txCtx, roleID, menuIDs)
		if err != nil {
			return err
		}
		if err := s.ensureMenuFeaturesEnabled(txCtx, menuIDs); err != nil {
			return err
		}
		currentRequestedMenuPermissionIDs, err := s.repository.ListMenuPermissionIDs(txCtx, menuIDs)
		if err != nil {
			return err
		}
		currentMenuPermissionCodes, err := s.repository.ListMenuPermissionCodes(txCtx, menuIDs)
		if err != nil {
			return err
		}
		currentRequestedPermissionIDs := uniqueInt64(append(append([]int64{}, permissionIDs...), currentRequestedMenuPermissionIDs...))
		if err := s.validateLockedPermissionGrant(txCtx, command.OperatorID, lockedPermissions, currentRequestedPermissionIDs); err != nil {
			return err
		}
		if err := s.ensureOperatorCanGrantPermissionCodes(txCtx, command.OperatorID, currentMenuPermissionCodes); err != nil {
			return err
		}
		disabledMenuIDs, err := s.disabledMenuIDs(txCtx, existingMenuIDs)
		if err != nil {
			return err
		}
		nextMenuIDs := uniqueInt64(append(append([]int64{}, menuIDs...), disabledMenuIDs...))
		derivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(txCtx, nextMenuIDs)
		if err != nil {
			return err
		}
		directPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(txCtx, []int64{roleID})
		if err != nil {
			return err
		}
		if !idsWithinGuard(derivedPermissionIDs, guardPermissionIDs) || !idsWithinGuard(directPermissions[roleID], guardPermissionIDs) {
			return apperrors.ObjectState("角色权限关系已并发变化，请重试")
		}
		disabledDirectPermissionIDs, err := s.disabledPermissionIDs(txCtx, directPermissions[roleID])
		if err != nil {
			return err
		}
		nextDirectPermissionIDs := uniqueInt64(append(append([]int64{}, permissionIDs...), disabledDirectPermissionIDs...))
		if err := s.repository.ReplaceRolePermissions(txCtx, roleID, nextDirectPermissionIDs, derivedPermissionIDs, nextMenuIDs, command.OperatorID, s.nextID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevision(txCtx, roleID, locked.GrantRevision, locked.GrantRevision+1, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) AssignUserRoles(ctx context.Context, command authorizationfacade.AssignUserRolesCommand) error {
	userID := command.UserID
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	roleIDs := uniqueInt64([]int64(command.RoleIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignUserRoles, userRoleAssignmentBinding(userID, roleIDs)); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.assignUserRolesWithinPolicy(txCtx, userID, roleIDs, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) AssignCreatedUserRoles(ctx context.Context, command authorizationfacade.AssignCreatedUserRolesCommand) error {
	userID := command.UserID
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	roleIDs := uniqueInt64([]int64(command.RoleIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignUserRoles, createdUserRoleAssignmentBinding(command.Username, roleIDs)); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.assignUserRolesWithinPolicy(txCtx, userID, roleIDs, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) ValidateCreatedUserRoles(ctx context.Context, command authorizationfacade.AssignCreatedUserRolesCommand) error {
	roleIDs := uniqueInt64([]int64(command.RoleIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignUserRoles, createdUserRoleAssignmentBinding(command.Username, roleIDs)); err != nil {
		return err
	}
	if err := s.ensureRolesExist(ctx, roleIDs); err != nil {
		return err
	}
	return s.ensureOperatorCanAssignUserRoles(ctx, command.OperatorID, roleIDs)
}

func (s *Service) AssignProvisionedUserRoles(ctx context.Context, command authorizationfacade.AssignProvisionedUserRolesCommand) error {
	userID := command.UserID
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	roleIDs := uniqueInt64([]int64(command.RoleIDs))
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		roles, err := s.lockRoleParents(txCtx, roleIDs)
		if err != nil {
			return err
		}
		for _, role := range roles {
			if role.IsAuthorizationRoot() {
				return apperrors.Forbidden("自动配置不能分配超级管理员角色")
			}
		}
		return s.repository.ReplaceUserRoles(txCtx, userID, roleIDs, 0, s.nextID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) BootstrapOwnerRoles(ctx context.Context, command authorizationfacade.BootstrapOwnerRolesCommand) error {
	userID := command.UserID
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	roleIDs := uniqueInt64([]int64(command.RoleIDs))
	if len(roleIDs) == 0 {
		return apperrors.Params("roleIds不能为空")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		roles, err := s.lockRoleParents(txCtx, roleIDs)
		if err != nil {
			return err
		}
		adminRole := false
		for _, role := range roles {
			if role.IsAuthorizationRoot() {
				adminRole = true
				break
			}
		}
		if !adminRole {
			return apperrors.Params("初始化Owner必须包含超级管理员角色")
		}
		if _, err := s.repository.LockSuperAdminInvariant(txCtx, userID); err != nil {
			return err
		}
		return s.repository.ReplaceUserRoles(txCtx, userID, roleIDs, command.OperatorID, s.nextID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GuardUserDeactivation(ctx context.Context, userID int64) error {
	if userID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	snapshot, err := s.repository.LockSuperAdminInvariant(ctx, userID)
	if err != nil {
		return err
	}
	if snapshot.WouldRemoveLastUser(false) {
		return lastSuperAdminError()
	}
	return nil
}

func (s *Service) assignUserRolesWithinPolicy(ctx context.Context, userID int64, roleIDs []int64, operatorID int64) error {
	roles, err := s.lockRoleParents(ctx, roleIDs)
	if err != nil {
		return err
	}
	if err := s.ensureOperatorCanAssignUserRoles(ctx, operatorID, roleIDs); err != nil {
		return err
	}
	nextHasSuperAdmin := false
	for _, role := range roles {
		if role.IsAuthorizationRoot() {
			nextHasSuperAdmin = true
			break
		}
	}
	snapshot, err := s.repository.LockSuperAdminInvariant(ctx, userID)
	if err != nil {
		return err
	}
	if snapshot.WouldRemoveLastUser(nextHasSuperAdmin) {
		return lastSuperAdminError()
	}
	return s.repository.ReplaceUserRoles(ctx, userID, roleIDs, operatorID, s.nextID)
}

func (s *Service) lockRoleParents(ctx context.Context, roleIDs []int64) ([]domain.RoleRecord, error) {
	ids := uniqueInt64(roleIDs)
	if len(ids) == 0 {
		return []domain.RoleRecord{}, nil
	}
	if len(ids) > authorizationRoleSetMax {
		return nil, apperrors.Params("角色数量超过单次授权上限")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	roles, err := s.repository.LockRoleGrants(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(roles) != len(ids) {
		return nil, apperrors.Params("存在无效角色ID")
	}
	for index, role := range roles {
		if role.RoleID != ids[index] || role.Status != 0 {
			return nil, apperrors.Params("存在无效角色ID")
		}
	}
	if err := s.repository.TouchRoleGrantGuards(ctx, ids); err != nil {
		return nil, err
	}
	return roles, nil
}

func (s *Service) lockPermissionParents(ctx context.Context, permissionIDs []int64) ([]domain.PermissionRecord, error) {
	ids := uniqueInt64(permissionIDs)
	if len(ids) == 0 {
		return []domain.PermissionRecord{}, nil
	}
	if len(ids) > authorizationRoleSetMax {
		return nil, apperrors.Params("权限数量超过单次授权上限")
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	permissions, err := s.repository.LockPermissionGrants(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(permissions) != len(ids) {
		return nil, apperrors.Params("存在无效权限ID")
	}
	for index, permission := range permissions {
		if permission.PermissionID != ids[index] {
			return nil, apperrors.Params("存在无效权限ID")
		}
	}
	if err := s.repository.TouchPermissionGrantGuards(ctx, ids); err != nil {
		return nil, err
	}
	return permissions, nil
}

func idsWithinGuard(ids, guardIDs []int64) bool {
	guarded := make(map[int64]struct{}, len(guardIDs))
	for _, id := range uniqueInt64(guardIDs) {
		guarded[id] = struct{}{}
	}
	for _, id := range uniqueInt64(ids) {
		if _, ok := guarded[id]; !ok {
			return false
		}
	}
	return true
}

func (s *Service) validateLockedPermissionGrant(ctx context.Context, operatorID int64, locked []domain.PermissionRecord, requestedIDs []int64) error {
	byID := make(map[int64]domain.PermissionRecord, len(locked))
	for _, permission := range locked {
		byID[permission.PermissionID] = permission
	}
	codes := make([]string, 0, len(requestedIDs))
	for _, permissionID := range uniqueInt64(requestedIDs) {
		permission, ok := byID[permissionID]
		if !ok || permission.Status != 0 || !s.featureEnabled(permission.FeatureCode) {
			return apperrors.ObjectState("权限状态已变化，请重试")
		}
		codes = append(codes, permission.Code)
	}
	return s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, codes)
}

func (s *Service) GetMenuTree(ctx context.Context, enabledOnly bool) ([]authorizationfacade.MenuTreeNodeVO, error) {
	records, err := s.repository.ListMenus(ctx, enabledOnly)
	if err != nil {
		return nil, err
	}
	return buildMenuTree(toMenuVOs(s.enabledMenus(records))), nil
}

func (s *Service) GetMenu(ctx context.Context, menuID int64) (*authorizationfacade.MenuTreeNodeVO, error) {
	if menuID <= 0 {
		return nil, apperrors.Params("menuId不能为空")
	}
	record, err := s.repository.FindMenuByID(ctx, menuID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperrors.NotFound("菜单不存在")
	}
	vo := toMenuVO(*record)
	return &vo, nil
}

func (s *Service) CreateMenu(ctx context.Context, command authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	record, err := s.buildMenuRecord(ctx, command, true)
	if err != nil {
		return nil, err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{record.Permission}); err != nil {
		return nil, err
	}
	record.MenuID = s.nextID()
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		if err := s.ensureMenuPermissionUnique(txCtx, record.MenuID, record.Permission); err != nil {
			return err
		}
		return s.repository.CreateMenu(txCtx, record, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetMenu(ctx, record.MenuID)
}

func (s *Service) UpdateMenu(ctx context.Context, command authorizationfacade.MenuCommand) (*authorizationfacade.MenuTreeNodeVO, error) {
	menuID := command.ID
	if menuID <= 0 {
		return nil, apperrors.Params("菜单ID不能为空")
	}
	if command.ParentID == menuID {
		return nil, apperrors.Params("父菜单不能是自身")
	}
	if existing, err := s.repository.FindMenuByID(ctx, menuID); err != nil {
		return nil, err
	} else if existing == nil {
		return nil, apperrors.NotFound("菜单不存在")
	}
	record, err := s.buildMenuRecord(ctx, command, false)
	if err != nil {
		return nil, err
	}
	record.MenuID = menuID
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{record.Permission}); err != nil {
		return nil, err
	}
	if err := s.ensureMenuPermissionUnique(ctx, record.MenuID, record.Permission); err != nil {
		return nil, err
	}
	var affectedRoles []int64
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		menus, err := s.repository.LockMenuGrants(txCtx, []int64{record.MenuID})
		if err != nil {
			return err
		}
		if len(menus) != 1 || menus[0].MenuID != record.MenuID {
			return apperrors.NotFound("菜单不存在")
		}
		if err := s.repository.TouchMenuGrantGuards(txCtx, []int64{record.MenuID}); err != nil {
			return err
		}
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		if err := s.ensureMenuPermissionUnique(txCtx, record.MenuID, record.Permission); err != nil {
			return err
		}
		affectedRoles, err = s.repository.ListRoleIDsByMenuID(txCtx, record.MenuID)
		if err != nil {
			return err
		}
		affectedRoles = uniqueInt64(affectedRoles)
		if len(affectedRoles) > authorizationRoleSetMax {
			return apperrors.Operation("菜单关联角色数量超过单次更新上限")
		}
		sort.Slice(affectedRoles, func(i, j int) bool { return affectedRoles[i] < affectedRoles[j] })
		roles, err := s.repository.LockRoleGrants(txCtx, affectedRoles)
		if err != nil {
			return err
		}
		if len(roles) != len(affectedRoles) {
			return apperrors.ObjectState("菜单关联角色已变化，请重试")
		}
		if err := s.repository.UpdateMenu(txCtx, record, command.OperatorID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevisions(txCtx, roles, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetMenu(ctx, record.MenuID)
}

func (s *Service) DeleteMenu(ctx context.Context, menuID int64, operatorID int64) error {
	if menuID <= 0 {
		return apperrors.Params("menuId不能为空")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		menus, err := s.repository.LockMenuGrants(txCtx, []int64{menuID})
		if err != nil {
			return err
		}
		if len(menus) != 1 || menus[0].MenuID != menuID {
			return apperrors.NotFound("菜单不存在")
		}
		if !s.featureEnabled(menus[0].FeatureCode) {
			return apperrors.Forbidden("不能为未启用功能的菜单分配权限")
		}
		if err := s.repository.TouchMenuGrantGuards(txCtx, []int64{menuID}); err != nil {
			return err
		}
		children, err := s.repository.CountMenuChildren(txCtx, menuID)
		if err != nil {
			return err
		}
		if children > 0 {
			return apperrors.Operation("存在子菜单，无法删除")
		}
		roleRefs, err := s.repository.CountRoleMenuReferences(txCtx, menuID)
		if err != nil {
			return err
		}
		if roleRefs > 0 {
			return apperrors.Operation("该菜单已分配给角色，无法删除")
		}
		if err := s.repository.DeleteMenuPermissionsByMenuID(txCtx, menuID); err != nil {
			return err
		}
		return s.repository.DeleteMenu(txCtx, menuID, operatorID)
	})
}

func (s *Service) ListPermissions(ctx context.Context, query authorizationfacade.PermissionQuery) ([]authorizationfacade.PermissionVO, error) {
	records, err := s.repository.ListPermissions(ctx, query)
	if err != nil {
		return nil, err
	}
	return toPermissionVOs(s.enabledPermissions(records)), nil
}

func (s *Service) PagePermissions(ctx context.Context, query authorizationfacade.PermissionPageQuery) (*authorizationfacade.PermissionPageVO, error) {
	current, size := normalizePage(query.Current, query.Size)
	query.Current = current
	query.Size = size
	filterFeatures := s.features != nil
	var enabledFeatureCodes []string
	if filterFeatures {
		enabledFeatureCodes = s.features.EnabledCodes()
	}
	records, total, err := s.repository.PagePermissions(ctx, query, filterFeatures, enabledFeatureCodes)
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.PermissionPageVO{
		Records: toPermissionVOs(records),
		Total:   total,
		Size:    size,
		Current: current,
	}, nil
}

func (s *Service) GetPermission(ctx context.Context, permissionID int64) (*authorizationfacade.PermissionVO, error) {
	if permissionID <= 0 {
		return nil, apperrors.Params("permissionId不能为空")
	}
	record, err := s.repository.FindPermissionByID(ctx, permissionID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperrors.NotFound("权限不存在")
	}
	vo := toPermissionVO(*record)
	return &vo, nil
}

func (s *Service) CreatePermission(ctx context.Context, command authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	record, err := s.buildPermissionRecord(command, true)
	if err != nil {
		return nil, err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{record.Code}); err != nil {
		return nil, err
	}
	record.PermissionID = s.nextID()
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		if err := s.ensurePermissionCodeUnique(txCtx, record.PermissionID, record.Code); err != nil {
			return err
		}
		return s.repository.CreatePermission(txCtx, record, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetPermission(ctx, record.PermissionID)
}

func (s *Service) UpdatePermission(ctx context.Context, permissionID int64, command authorizationfacade.PermissionCommand) (*authorizationfacade.PermissionVO, error) {
	if permissionID <= 0 {
		return nil, apperrors.Params("permissionId不能为空")
	}
	if existing, err := s.repository.FindPermissionByID(ctx, permissionID); err != nil {
		return nil, err
	} else if existing == nil {
		return nil, apperrors.NotFound("权限不存在")
	}
	record, err := s.buildPermissionRecord(command, false)
	if err != nil {
		return nil, err
	}
	record.PermissionID = permissionID
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{record.Code}); err != nil {
		return nil, err
	}
	if err := s.ensurePermissionCodeUnique(ctx, record.PermissionID, record.Code); err != nil {
		return nil, err
	}
	var affectedRoles []int64
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		permissions, err := s.lockPermissionParents(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		if len(permissions) != 1 {
			return apperrors.NotFound("权限不存在")
		}
		if err := s.repository.LockAuthorizationCreationGuard(txCtx); err != nil {
			return err
		}
		if err := s.ensurePermissionCodeUnique(txCtx, record.PermissionID, record.Code); err != nil {
			return err
		}
		affectedRoles, err = s.repository.ListRoleIDsByPermissionIDs(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		affectedRoles = uniqueInt64(affectedRoles)
		if len(affectedRoles) > authorizationRoleSetMax {
			return apperrors.Operation("权限关联角色数量超过单次更新上限")
		}
		sort.Slice(affectedRoles, func(i, j int) bool { return affectedRoles[i] < affectedRoles[j] })
		roles, err := s.repository.LockRoleGrants(txCtx, affectedRoles)
		if err != nil {
			return err
		}
		if len(roles) != len(affectedRoles) {
			return apperrors.ObjectState("权限关联角色已变化，请重试")
		}
		if err := s.repository.UpdatePermission(txCtx, record, command.OperatorID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevisions(txCtx, roles, command.OperatorID)
	}); err != nil {
		return nil, err
	}
	return s.GetPermission(ctx, permissionID)
}

func (s *Service) DeletePermission(ctx context.Context, permissionID int64, operatorID int64) error {
	if permissionID <= 0 {
		return apperrors.Params("permissionId不能为空")
	}
	var affectedRoles []int64
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if _, err := s.lockPermissionParents(txCtx, []int64{permissionID}); err != nil {
			return err
		}
		menuIDs, err := s.repository.ListMenuIDsByPermissionIDs(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		sort.Slice(menuIDs, func(i, j int) bool { return menuIDs[i] < menuIDs[j] })
		menus, err := s.repository.LockMenuGrants(txCtx, menuIDs)
		if err != nil {
			return err
		}
		if len(menus) != len(menuIDs) {
			return apperrors.ObjectState("权限关联菜单已变化，请重试")
		}
		if err := s.repository.TouchMenuGrantGuards(txCtx, menuIDs); err != nil {
			return err
		}
		affectedRoles, err = s.repository.ListRoleIDsByPermissionIDs(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		affectedRoles = uniqueInt64(affectedRoles)
		if len(affectedRoles) > authorizationRoleSetMax {
			return apperrors.Operation("权限关联角色数量超过单次删除上限")
		}
		sort.Slice(affectedRoles, func(i, j int) bool { return affectedRoles[i] < affectedRoles[j] })
		roles, err := s.repository.LockRoleGrants(txCtx, affectedRoles)
		if err != nil {
			return err
		}
		if len(roles) != len(affectedRoles) {
			return apperrors.ObjectState("权限关联角色已变化，请重试")
		}
		if err := s.repository.SoftDeleteUserPermissionsByPermissionID(txCtx, permissionID, operatorID); err != nil {
			return err
		}
		if err := s.repository.DeletePermission(txCtx, permissionID, operatorID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevisions(txCtx, roles, operatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetMenuPermissionIDs(ctx context.Context, menuID int64) ([]int64, error) {
	if menuID <= 0 {
		return nil, apperrors.Params("menuId不能为空")
	}
	permissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, []int64{menuID})
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListPermissionsByIDs(ctx, permissionIDs)
	if err != nil {
		return nil, err
	}
	return intersectIDs(permissionIDs, permissionIDsFromRecords(s.enabledPermissions(records))), nil
}

func (s *Service) BindMenuPermissions(ctx context.Context, command authorizationfacade.MenuPermissionAssignCommand) error {
	menuID := command.MenuID
	if menuID <= 0 {
		return apperrors.Params("menuId不能为空")
	}
	permissionIDs := uniqueInt64([]int64(command.PermissionIDs))
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignMenuPermissions, menuPermissionAssignmentBinding(menuID, permissionIDs)); err != nil {
		return err
	}
	menu, err := s.repository.FindMenuByID(ctx, menuID)
	if err != nil {
		return err
	}
	if menu == nil {
		return apperrors.NotFound("菜单不存在")
	}
	if !s.featureEnabled(menu.FeatureCode) {
		return apperrors.Forbidden("不能为未启用功能的菜单分配权限")
	}
	if err := s.ensurePermissionsExist(ctx, permissionIDs); err != nil {
		return err
	}
	if err := s.ensurePermissionFeaturesEnabled(ctx, permissionIDs); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionIDs(ctx, command.OperatorID, permissionIDs); err != nil {
		return err
	}
	observedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, []int64{menuID})
	if err != nil {
		return err
	}
	guardPermissionIDs := uniqueInt64(append(append([]int64{}, permissionIDs...), observedPermissionIDs...))
	var affectedRoles []int64
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		lockedPermissions, err := s.lockPermissionParents(txCtx, guardPermissionIDs)
		if err != nil {
			return err
		}
		if err := s.validateLockedPermissionGrant(txCtx, command.OperatorID, lockedPermissions, permissionIDs); err != nil {
			return err
		}
		menus, err := s.repository.LockMenuGrants(txCtx, []int64{menuID})
		if err != nil {
			return err
		}
		if len(menus) != 1 || menus[0].MenuID != menuID {
			return apperrors.NotFound("菜单不存在")
		}
		if err := s.repository.TouchMenuGrantGuards(txCtx, []int64{menuID}); err != nil {
			return err
		}
		affectedRoles, err = s.repository.ListRoleIDsByMenuID(txCtx, menuID)
		if err != nil {
			return err
		}
		affectedRoles = uniqueInt64(affectedRoles)
		if len(affectedRoles) > authorizationRoleSetMax {
			return apperrors.Operation("菜单关联角色数量超过单次授权上限")
		}
		lockedRoles := append([]int64(nil), affectedRoles...)
		sort.Slice(lockedRoles, func(i, j int) bool { return lockedRoles[i] < lockedRoles[j] })
		roles, err := s.repository.LockRoleGrants(txCtx, lockedRoles)
		if err != nil {
			return err
		}
		if len(roles) != len(lockedRoles) {
			return apperrors.NotFound("关联角色不存在")
		}
		roleRevisions := make(map[int64]int64, len(lockedRoles))
		for _, role := range roles {
			if role.RoleID <= 0 {
				return apperrors.NotFound("关联角色不存在")
			}
			roleRevisions[role.RoleID] = role.GrantRevision
		}
		for _, roleID := range lockedRoles {
			if _, ok := roleRevisions[roleID]; !ok {
				return apperrors.NotFound("关联角色不存在")
			}
		}
		roleMenus, err := s.repository.ListRoleMenuIDsByRoleIDs(txCtx, lockedRoles)
		if err != nil {
			return err
		}
		directPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(txCtx, lockedRoles)
		if err != nil {
			return err
		}
		allMenuIDs := []int64{menuID}
		roleMenuRelations := 0
		for _, ids := range roleMenus {
			roleMenuRelations += len(uniqueInt64(ids))
			if roleMenuRelations > authorizationDerivedRelationSetMax {
				return apperrors.Operation("角色菜单关系数量超过单次派生授权上限")
			}
			allMenuIDs = append(allMenuIDs, ids...)
		}
		allMenuIDs = uniqueInt64(allMenuIDs)
		if len(allMenuIDs) > authorizationRoleSetMax {
			return apperrors.Operation("派生授权菜单数量超过单次上限")
		}
		menuPermissions, err := s.repository.ListMenuPermissionIDsByMenuIDs(txCtx, allMenuIDs)
		if err != nil {
			return err
		}
		guardedPermissions := make(map[int64]struct{}, len(guardPermissionIDs))
		for _, permissionID := range guardPermissionIDs {
			guardedPermissions[permissionID] = struct{}{}
		}
		for _, permissionID := range uniqueInt64(menuPermissions[menuID]) {
			if _, ok := guardedPermissions[permissionID]; !ok {
				return apperrors.ObjectState("菜单权限关系已并发变化，请重试")
			}
		}
		menuPermissionRelations := 0
		for _, ids := range menuPermissions {
			menuPermissionRelations += len(uniqueInt64(ids))
			if menuPermissionRelations > authorizationDerivedRelationSetMax {
				return apperrors.Operation("菜单权限关系数量超过单次派生授权上限")
			}
		}
		existingPermissionIDs := menuPermissions[menuID]
		disabledPermissionIDs, err := s.disabledPermissionIDs(txCtx, existingPermissionIDs)
		if err != nil {
			return err
		}
		nextPermissionIDs := uniqueInt64(append(append([]int64{}, permissionIDs...), disabledPermissionIDs...))
		menuPermissions[menuID] = nextPermissionIDs
		assignments := make([]domain.RolePermissionAssignment, 0, len(lockedRoles))
		derivedRelations := 0
		for _, roleID := range lockedRoles {
			menuIDs := uniqueInt64(roleMenus[roleID])
			rolePermissionIDs := make([]int64, 0)
			for _, item := range menuIDs {
				rolePermissionIDs = append(rolePermissionIDs, menuPermissions[item]...)
			}
			rolePermissionIDs = uniqueInt64(rolePermissionIDs)
			derivedRelations += len(rolePermissionIDs)
			if derivedRelations > authorizationDerivedRelationSetMax {
				return apperrors.Operation("角色派生权限关系数量超过单次更新上限")
			}
			assignments = append(assignments, domain.RolePermissionAssignment{
				RoleID:              roleID,
				DirectPermissionIDs: uniqueInt64(directPermissions[roleID]),
				MenuPermissionIDs:   rolePermissionIDs,
			})
		}
		if err := s.repository.ReplaceMenuPermissions(txCtx, menuID, nextPermissionIDs, command.OperatorID, s.nextID); err != nil {
			return err
		}
		if err := s.repository.ReplaceDerivedRolePermissionsBatch(txCtx, assignments, command.OperatorID, s.nextID); err != nil {
			return err
		}
		return s.repository.UpdateRoleGrantRevisions(txCtx, roles, command.OperatorID)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GrantTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionGrantCommand) error {
	if command.UserID <= 0 || strings.TrimSpace(command.PermissionCode) == "" {
		return apperrors.Params("userId和permissionCode不能为空")
	}
	permissionCode := strings.TrimSpace(command.PermissionCode)
	if strings.TrimSpace(command.Reason) == "" {
		return apperrors.Params("临时权限授予原因不能为空")
	}
	binding := authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACGrantTempPermission, command.UserID, permissionCode, command.ExpireAt, command.Reason)
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACGrantTempPermission, binding); err != nil {
		return err
	}
	if err := s.ensurePermissionCodeExists(ctx, permissionCode); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.GrantedBy, []string{permissionCode}); err != nil {
		return err
	}
	permissionID, err := s.repository.FindPermissionIDByCode(ctx, permissionCode)
	if err != nil {
		return err
	}
	if permissionID <= 0 {
		return apperrors.Params("权限编码不存在")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		permissions, err := s.lockPermissionParents(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		if len(permissions) != 1 || strings.TrimSpace(permissions[0].Code) != permissionCode || permissions[0].Status != 0 {
			return apperrors.ObjectState("权限状态已变化，请重试")
		}
		if err := s.ensureOperatorCanGrantPermissionCodes(txCtx, command.GrantedBy, []string{permissions[0].Code}); err != nil {
			return err
		}
		if err := s.repository.GrantTemporaryPermission(txCtx, command.UserID, permissionCode, command.ExpireAt, command.Source, command.Reason, command.GrantedBy, s.nextID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) RevokeTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error {
	if command.UserID <= 0 || strings.TrimSpace(command.PermissionCode) == "" {
		return apperrors.Params("userId和permissionCode不能为空")
	}
	permissionCode := strings.TrimSpace(command.PermissionCode)
	if strings.TrimSpace(command.Reason) == "" {
		return apperrors.Params("临时权限撤销原因不能为空")
	}
	binding := authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACRevokeTempPermission, command.UserID, permissionCode, nil, command.Reason)
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACRevokeTempPermission, binding); err != nil {
		return err
	}
	if err := s.ensurePermissionCodeExists(ctx, permissionCode); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{permissionCode}); err != nil {
		return err
	}
	permissionID, err := s.repository.FindPermissionIDByCode(ctx, permissionCode)
	if err != nil {
		return err
	}
	if permissionID <= 0 {
		return apperrors.Params("权限编码不存在")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		permissions, err := s.lockPermissionParents(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		if len(permissions) != 1 || strings.TrimSpace(permissions[0].Code) != permissionCode {
			return apperrors.ObjectState("权限状态已变化，请重试")
		}
		if err := s.ensureOperatorCanGrantPermissionCodes(txCtx, command.OperatorID, []string{permissions[0].Code}); err != nil {
			return err
		}
		if err := s.repository.RevokeTemporaryPermission(txCtx, command.UserID, permissionCode); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) ExtendTemporaryPermission(ctx context.Context, command authorizationfacade.TemporaryPermissionUpdateCommand) error {
	if command.UserID <= 0 || strings.TrimSpace(command.PermissionCode) == "" {
		return apperrors.Params("userId和permissionCode不能为空")
	}
	permissionCode := strings.TrimSpace(command.PermissionCode)
	if strings.TrimSpace(command.Reason) == "" {
		return apperrors.Params("临时权限延期原因不能为空")
	}
	binding := authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACExtendTempPermission, command.UserID, permissionCode, command.ExpireAt, command.Reason)
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACExtendTempPermission, binding); err != nil {
		return err
	}
	if err := s.ensurePermissionCodeExists(ctx, permissionCode); err != nil {
		return err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, command.OperatorID, []string{permissionCode}); err != nil {
		return err
	}
	permissionID, err := s.repository.FindPermissionIDByCode(ctx, permissionCode)
	if err != nil {
		return err
	}
	if permissionID <= 0 {
		return apperrors.Params("权限编码不存在")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		permissions, err := s.lockPermissionParents(txCtx, []int64{permissionID})
		if err != nil {
			return err
		}
		if len(permissions) != 1 || strings.TrimSpace(permissions[0].Code) != permissionCode || permissions[0].Status != 0 {
			return apperrors.ObjectState("权限状态已变化，请重试")
		}
		if err := s.ensureOperatorCanGrantPermissionCodes(txCtx, command.OperatorID, []string{permissions[0].Code}); err != nil {
			return err
		}
		if err := s.repository.ExtendTemporaryPermission(txCtx, command.UserID, permissionCode, command.ExpireAt, command.Reason); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func temporaryPermissionStatus(item domain.TemporaryPermissionRecord, now time.Time) string {
	if item.Type != 1 || item.ExpireAt == nil {
		return "PERMANENT"
	}
	if !item.ExpireAt.After(now) {
		return "EXPIRED"
	}
	return "ACTIVE"
}

func (s *Service) CleanupExpiredTemporaryPermissions(ctx context.Context) error {
	var afterUserID int64
	for {
		var affectedUsers []int64
		if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
			var err error
			affectedUsers, err = s.repository.ListExpiredTemporaryPermissionUserIDsPage(txCtx, afterUserID, authorizationAffectedUserPageSize)
			if err != nil {
				return err
			}
			return s.repository.CleanupExpiredTemporaryPermissionsByUserIDs(txCtx, affectedUsers)
		}); err != nil {
			return err
		}
		if len(affectedUsers) == 0 {
			return nil
		}
		afterUserID = affectedUsers[len(affectedUsers)-1]
		if len(affectedUsers) < authorizationAffectedUserPageSize {
			return nil
		}
	}
}

func (s *Service) ListUserTemporaryPermissions(ctx context.Context, userID int64) ([]authorizationfacade.TemporaryPermissionVO, error) {
	items, err := s.repository.ListUserTemporaryPermissions(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	result := make([]authorizationfacade.TemporaryPermissionVO, 0, len(items))
	for _, item := range items {
		result = append(result, authorizationfacade.TemporaryPermissionVO{
			UserID:         item.UserID,
			PermissionCode: item.PermissionCode,
			PermissionName: item.PermissionName,
			Type:           item.Type,
			ExpireAt:       item.ExpireAt,
			Source:         item.Source,
			Reason:         item.Reason,
			GrantedBy:      item.GrantedBy,
			GrantedAt:      item.GrantedAt,
			UpdatedAt:      item.UpdatedAt,
			Status:         temporaryPermissionStatus(item, now),
		})
	}
	return result, nil
}

func (s *Service) TemporaryPermissionStats(ctx context.Context) (*authorizationfacade.TemporaryPermissionStatsVO, error) {
	stats, err := s.repository.TemporaryPermissionStats(ctx)
	if err != nil || stats == nil {
		return nil, err
	}
	return &authorizationfacade.TemporaryPermissionStatsVO{
		TotalActive:  stats.TotalActive,
		Temporary:    stats.Temporary,
		Permanent:    stats.Permanent,
		ExpiringSoon: stats.ExpiringSoon,
	}, nil
}

func (s *Service) GetCurrentUserMenus(ctx context.Context, userID int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	if userID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	governed, request, enabled := s.authorizationMenuCacheRequest(userID)
	if !enabled {
		return s.getCurrentUserMenusSource(ctx, userID)
	}
	snapshotter, ok := s.transactor.(store.Snapshotter)
	if !ok || !snapshotter.Enabled() {
		return nil, apperrors.System("授权菜单一致性快照能力未配置")
	}
	var menus []authorizationfacade.MenuTreeNodeVO
	err := snapshotter.WithinReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		var cached []authorizationfacade.MenuTreeNodeVO
		_, cacheErr := governed.GetOrLoadClassifiedWithPreflight(snapshotCtx, request, &cached, func(preflightCtx context.Context) (bool, error) {
			if eligibilityErr := s.requireAuthorizationUserAvailable(preflightCtx, userID); eligibilityErr != nil {
				return false, eligibilityErr
			}
			activeTemporaryGrant, grantErr := s.hasActiveTemporaryPermission(preflightCtx, userID)
			return !activeTemporaryGrant, grantErr
		}, func(loadCtx context.Context) (cachepolicy.CacheableValue, error) {
			loaded, loadErr := s.getCurrentUserMenusSource(loadCtx, userID)
			if loadErr != nil {
				return cachepolicy.CacheableValue{}, loadErr
			}
			return cachepolicy.CacheableValue{Value: loaded, Cacheable: true}, nil
		})
		if cacheErr == nil {
			menus = cached
			return nil
		}
		// A failed cache candidate cannot be trusted. The outer source snapshot
		// keeps the fallback coherent without permitting an old menu projection.
		var loadErr error
		menus, loadErr = s.getCurrentUserMenusSource(snapshotCtx, userID)
		return loadErr
	})
	if err != nil {
		return nil, err
	}
	return menus, nil
}

func (s *Service) getCurrentUserMenusSource(ctx context.Context, userID int64) ([]authorizationfacade.MenuTreeNodeVO, error) {
	if err := s.requireAuthorizationUserAvailable(ctx, userID); err != nil {
		return nil, err
	}
	items, err := s.repository.ListUserMenus(ctx, userID)
	if err != nil {
		return nil, err
	}
	items = s.enabledMenus(items)
	nodes := make([]authorizationfacade.MenuTreeNodeVO, 0, len(items))
	for _, item := range items {
		nodes = append(nodes, authorizationfacade.MenuTreeNodeVO{
			ID:          item.MenuID,
			MenuID:      item.MenuID,
			ParentID:    item.ParentID,
			SortOrder:   item.SortOrder,
			Name:        item.Name,
			Path:        item.Path,
			Component:   item.Component,
			Type:        item.Type,
			Permission:  item.Permission,
			FeatureCode: item.FeatureCode,
			Icon:        item.Icon,
			Status:      item.Status,
			Visible:     item.Visible,
			IsFrame:     item.IsFrame,
			IsCache:     item.IsCache,
			Remark:      item.Remark,
			Checked:     true,
			CreateTime:  item.CreateTime,
			UpdateTime:  item.UpdateTime,
		})
	}
	return pruneEmptyMenuContainers(buildMenuTree(nodes)), nil
}

func (s *Service) CreateStepUpChallenge(ctx context.Context, request authorizationfacade.RequestScope, command authorizationfacade.StepUpChallengeRequest) (*authorizationfacade.StepUpChallengeVO, error) {
	if request.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	if s.challenges == nil {
		return nil, apperrors.System("challenge facade未配置")
	}
	flowNonce := strings.TrimSpace(command.FlowNonce)
	if flowNonce == "" {
		flowNonce = fmt.Sprintf("auth-step-up-%d", time.Now().UTC().UnixNano())
	}
	result, err := s.challenges.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         authIssuerService,
		AudienceServiceNames:       []string{authAudienceService},
		BusinessAction:             strings.TrimSpace(command.BusinessAction),
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "USER", SubjectValue: buildSubjectIdentifier(request.UserID)},
		SubjectIdentifier:          buildSubjectIdentifier(request.UserID),
		FlowNonce:                  flowNonce,
		RequestedTimeToLiveSeconds: command.RequestedTTLSeconds,
		IdempotencyKey:             fmt.Sprintf("%s|%d|%s", strings.TrimSpace(command.BusinessAction), request.UserID, flowNonce),
		RiskContext: &challengefacade.RiskContext{
			IPAddress:        request.IPAddress,
			UserAgent:        request.UserAgent,
			DeviceIdentifier: request.DeviceID,
			TenantIdentifier: request.TenantID,
		},
		ExpectedChallengeTypes: command.ExpectedChallengeTypes,
		ExtensionContext: map[string]any{
			"operationBinding": strings.TrimSpace(command.OperationBinding),
			"targetEmail":      strings.TrimSpace(command.TargetEmail),
		},
	})
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.StepUpChallengeVO{
		ChallengeIdentifier:        result.ChallengeIdentifier,
		ChallengeState:             result.ChallengeState,
		EffectiveTimeToLiveSeconds: result.EffectiveTimeToLiveSeconds,
		RequiredAssuranceLevel:     result.RequiredAssuranceLevel,
		ResolvedAssuranceLevel:     result.ResolvedAssuranceLevel,
		RecommendedStepIdentifier:  result.RecommendedStepIdentifier,
		ActualChallengeTypeNames:   result.ActualChallengeTypeNames,
		Steps:                      toChallengeSteps(result.Steps),
	}, nil
}

func (s *Service) VerifyStepUp(ctx context.Context, request authorizationfacade.RequestScope, command authorizationfacade.StepUpVerifyRequest) (*authorizationfacade.StepUpTokenVO, error) {
	if request.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录")
	}
	if s.proof == nil {
		return nil, apperrors.System("challenge proof verifier未配置")
	}
	claims, err := s.proof.VerifyProofToken(ctx, challengefacade.ProofTokenVerifyRequest{
		ProofToken:          strings.TrimSpace(command.ProofToken),
		AudienceServiceName: authAudienceService,
		BusinessAction:      strings.TrimSpace(command.BusinessAction),
		FlowNonce:           strings.TrimSpace(command.FlowNonce),
		SubjectIdentifier:   buildSubjectIdentifier(request.UserID),
		OperationBinding:    strings.TrimSpace(command.OperationBinding),
		ConsumeOnce:         command.ConsumeOnce,
	})
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.StepUpTokenVO{
		ProofToken:                strings.TrimSpace(command.ProofToken),
		ChallengeID:               claims.ChallengeIdentifier,
		TokenUniqueIdentifier:     claims.TokenUniqueIdentifier,
		BusinessAction:            claims.BusinessAction,
		FlowNonce:                 claims.FlowNonce,
		OperationBinding:          claims.OperationBinding,
		AuthenticationMethodNames: append([]string(nil), claims.AuthenticationMethodNames...),
		IssuedAt:                  claims.IssuedAt,
		ExpiresAt:                 claims.ExpiresAt,
	}, nil
}

func (s *Service) ValidateStepUpToken(ctx context.Context, request authorizationfacade.RequestScope, command authorizationfacade.StepUpValidateRequest) (bool, error) {
	_, err := s.VerifyStepUp(ctx, request, authorizationfacade.StepUpVerifyRequest(command))
	if err != nil {
		return false, err
	}
	return true, nil
}

// authorizationContextCacheRequest returns only the reviewed governed-cache
// surface. In particular, authorization.cache.enabled and its legacy raw key
// are intentionally not consulted: if the durable invalidation registrar,
// catalog, or governed layer is absent, callers load the authority instead.
func (s *Service) authorizationContextCacheRequest(userID int64) (cacheinfra.GovernedCache, cachepolicy.ReadRequest, bool) {
	return s.authorizationSnapshotCacheRequest(userID, cachepolicy.AuthorizationContextReadRequest)
}

func (s *Service) authorizationMenuCacheRequest(userID int64) (cacheinfra.GovernedCache, cachepolicy.ReadRequest, bool) {
	return s.authorizationSnapshotCacheRequest(userID, cachepolicy.AuthorizationMenuReadRequest)
}

func (s *Service) authorizationSnapshotCacheRequest(userID int64, requestFactory func(int64, string) (cachepolicy.ReadRequest, bool)) (cacheinfra.GovernedCache, cachepolicy.ReadRequest, bool) {
	if s == nil || s.invalidations == nil || !s.invalidations.Enabled() || s.cache == nil || requestFactory == nil {
		return nil, cachepolicy.ReadRequest{}, false
	}
	governed, ok := s.cache.(cacheinfra.GovernedCache)
	if !ok || governed == nil {
		return nil, cachepolicy.ReadRequest{}, false
	}
	request, ok := requestFactory(userID, s.authorizationFeatureFingerprint())
	if !ok {
		return nil, cachepolicy.ReadRequest{}, false
	}
	return governed, request, true
}

func (s *Service) authorizationFeatureFingerprint() string {
	if s == nil || s.features == nil {
		return cachepolicy.EventDigest("authorization-features:none")
	}
	return cachepolicy.EventDigest("authorization-features:" + strings.Join(s.features.EnabledCodes(), "\x00"))
}

// requireAuthorizationUserAvailable is deliberately a source read performed
// before every governed context/menu candidate. A lock or disable commit can
// therefore never be hidden behind a previously warm L1/L2 authorization
// result, even while relay/fanout delivery is delayed.
func (s *Service) requireAuthorizationUserAvailable(ctx context.Context, userID int64) error {
	aggregate, err := s.repository.FindUserAggregate(ctx, userID)
	if err != nil {
		return err
	}
	if aggregate == nil || !aggregate.Enabled || aggregate.Locked {
		return apperrors.Unauthorized("当前用户不存在或已失效")
	}
	return nil
}

func (s *Service) hasActiveTemporaryPermission(ctx context.Context, userID int64) (bool, error) {
	items, err := s.repository.ListUserTemporaryPermissions(ctx, userID)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	for _, item := range items {
		if item.Type == 0 {
			continue
		}
		// A malformed temporary grant without an expiry is treated as active.
		// Bypassing cache in that case is conservative and cannot extend access.
		if item.ExpireAt == nil || item.ExpireAt.UTC().After(now) {
			return true, nil
		}
	}
	return false, nil
}

// withAuthorizationSession deliberately runs after a governed context
// snapshot is selected. Session IDs, bearer/cookie validity timestamps, and
// request source therefore never enter the reusable L1/L2 value.
func withAuthorizationSession(snapshot *securitycontext.UserContext, sessionID string, issuedAt, expiresAt *time.Time, source string) *securitycontext.UserContext {
	if snapshot == nil {
		return nil
	}
	result := *snapshot
	result.SessionID = strings.TrimSpace(sessionID)
	result.IssuedAtEpoch = 0
	result.ExpireAtEpoch = 0
	result.Source = strings.TrimSpace(source)
	if issuedAt != nil {
		result.IssuedAtEpoch = issuedAt.UTC().Unix()
	}
	if expiresAt != nil {
		result.ExpireAtEpoch = expiresAt.UTC().Unix()
	}
	return &result
}

// authorizationSnapshotVersion is a deterministic representation of the
// authority result. The cache's Redis generation is the cross-instance
// revision in the key; this compact value is only response metadata and never
// a cache key, outbox field, token, or authorization input.
func authorizationSnapshotVersion(user *securitycontext.UserContext) int64 {
	if user == nil {
		return 0
	}
	material := strings.Join([]string{
		strconv.FormatInt(user.UserID, 10),
		strings.TrimSpace(user.Username),
		strings.TrimSpace(user.Nickname),
		canonicalInt64s(user.RoleIDs),
		canonicalStrings(user.Roles),
		canonicalStrings(user.Permissions),
		strconv.FormatInt(user.PrimaryOrgID, 10),
		canonicalInt64s(user.OrgIDs),
		strconv.FormatInt(user.PrimaryDeptID, 10),
		canonicalInt64s(user.DeptIDs),
		canonicalInt64s(user.PostIDs),
		canonicalStrings(user.PostCodes),
		canonicalInt64s(user.DataScopeDeptIDs),
		canonicalInt64s(user.DataScopeOrgIDs),
		string(user.DataScopeType),
		strconv.FormatBool(user.IsAdmin),
	}, "\x00")
	value, err := strconv.ParseInt(cachepolicy.EventDigest("authorization-snapshot:" + material)[:15], 16, 64)
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

func canonicalInt64s(values []int64) string {
	items := append([]int64(nil), values...)
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	parts := make([]string, 0, len(items))
	for _, value := range items {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func canonicalStrings(values []string) string {
	items := append([]string(nil), values...)
	sort.Strings(items)
	return strings.Join(items, ",")
}

func (s *Service) withTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("授权安全写一致性事务能力未配置")
	}
	consistent, ok := s.transactor.(store.ConsistentTransactor)
	if !ok {
		return apperrors.System("授权安全写一致性事务能力未配置")
	}
	const serializationRetryAttempts = 3
	var err error
	for attempt := 0; attempt < serializationRetryAttempts; attempt++ {
		err = consistent.WithinConsistentTransaction(ctx, fn)
		if err == nil || !store.IsSerializationFailure(err) {
			return err
		}
	}
	return err
}

// withAuthorizationInvalidationTx is the write-side registry boundary for
// every mutation that can alter a user's authorization context or visible
// menu tree. It invalidates bounded class generations, never discovers or
// enumerates affected users, and therefore cannot rely on local cache deletes
// for correctness. Config-scope policy remains authoritative and is not
// admitted into this cache; its role mutation is nevertheless covered because
// the same role grant transaction can change effective authorization.
func (s *Service) withAuthorizationInvalidationTx(ctx context.Context, fn func(context.Context) error) error {
	return cachegovernancefacade.RunInvalidatedMutationClasses(ctx, s.withTransaction, s.transactor, s.invalidations, []cachepolicy.DataClass{
		cachepolicy.DataClassAuthorizationContext,
		cachepolicy.DataClassAuthorizationMenus,
	}, func(txCtx context.Context) (bool, error) {
		if err := fn(txCtx); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *Service) buildRoleRecord(command authorizationfacade.RoleCommand, creating bool) (domain.RoleRecord, error) {
	name := strings.TrimSpace(command.Name)
	code := strings.TrimSpace(command.Code)
	if name == "" || code == "" {
		return domain.RoleRecord{}, apperrors.Params("角色名称和编码不能为空")
	}
	if len([]rune(name)) > 30 {
		return domain.RoleRecord{}, apperrors.Params("角色名称最多30个字符")
	}
	if len([]rune(code)) > 100 {
		return domain.RoleRecord{}, apperrors.Params("角色编码最多100个字符")
	}
	status := 0
	if command.Status != nil {
		status = *command.Status
	}
	dataScope := 1
	if command.DataScope != nil {
		dataScope = *command.DataScope
	}
	sortOrder := 0
	if command.SortOrder != nil {
		sortOrder = *command.SortOrder
	} else if command.Sort != nil {
		sortOrder = *command.Sort
	}
	roleType := roleTypeCode(command.Type)
	if creating && strings.TrimSpace(command.Type) == "" {
		roleType = 3
	}
	return domain.RoleRecord{Name: name, Code: code, Type: roleType, Status: status, DataScope: dataScope, SortOrder: sortOrder, Remark: command.Remark}, nil
}

func (s *Service) ensureRoleCodeUnique(ctx context.Context, roleID int64, code string) error {
	count, err := s.repository.CountRoleCodeExcludingID(ctx, roleID, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperrors.Operation("角色编码已存在")
	}
	return nil
}

func systemRoleProtectionError(violation domain.RoleProtectionViolation) error {
	switch violation {
	case domain.RoleProtectionCode:
		return apperrors.Operation("SYSTEM角色编码不可修改")
	case domain.RoleProtectionType:
		return apperrors.Operation("SYSTEM角色类型不可修改")
	case domain.RoleProtectionStatus:
		return apperrors.Operation("SYSTEM角色不可禁用")
	default:
		return apperrors.Operation("SYSTEM角色受保护")
	}
}

func lastSuperAdminError() error {
	return apperrors.Operation("必须保留至少一个有效超级管理员")
}

func (s *Service) ensureRolesExist(ctx context.Context, roleIDs []int64) error {
	ids := uniqueInt64(roleIDs)
	if len(ids) == 0 {
		return nil
	}
	count, err := s.repository.CountRolesByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return apperrors.Params("存在无效角色ID")
	}
	return nil
}

func (s *Service) ensureDeptsExist(ctx context.Context, deptIDs []int64) error {
	ids := uniqueInt64(deptIDs)
	if len(ids) == 0 {
		return nil
	}
	count, err := s.repository.CountDeptsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return apperrors.Params("存在无效部门ID")
	}
	return nil
}

func (s *Service) ensureMenusExist(ctx context.Context, menuIDs []int64) error {
	ids := uniqueInt64(menuIDs)
	if len(ids) == 0 {
		return nil
	}
	count, err := s.repository.CountMenusByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return apperrors.Params("存在无效菜单ID")
	}
	return nil
}

func (s *Service) featureEnabled(featureCode string) bool {
	code := features.Code(strings.TrimSpace(featureCode))
	return code == "" || s.features == nil || s.features.Enabled(code)
}

func (s *Service) enabledMenus(records []domain.MenuRecord) []domain.MenuRecord {
	result := make([]domain.MenuRecord, 0, len(records))
	for _, record := range records {
		if s.featureEnabled(record.FeatureCode) {
			result = append(result, record)
		}
	}
	return result
}

func (s *Service) enabledPermissions(records []domain.PermissionRecord) []domain.PermissionRecord {
	result := make([]domain.PermissionRecord, 0, len(records))
	for _, record := range records {
		if s.featureEnabled(record.FeatureCode) {
			result = append(result, record)
		}
	}
	return result
}

func (s *Service) enabledPermissionCodes(records []domain.PermissionRecord) []string {
	result := make([]string, 0, len(records))
	for _, record := range records {
		if s.featureEnabled(record.FeatureCode) {
			result = append(result, record.Code)
		}
	}
	return result
}

func (s *Service) ensureMenuFeaturesEnabled(ctx context.Context, menuIDs []int64) error {
	disabled, err := s.disabledMenuIDs(ctx, menuIDs)
	if err != nil {
		return err
	}
	if len(disabled) > 0 {
		return apperrors.Forbidden("不能分配未启用功能的菜单")
	}
	return nil
}

func (s *Service) disabledMenuIDs(ctx context.Context, menuIDs []int64) ([]int64, error) {
	ids := uniqueInt64(menuIDs)
	if len(ids) == 0 || s.features == nil {
		return []int64{}, nil
	}
	records, err := s.repository.ListAllMenus(ctx)
	if err != nil {
		return nil, err
	}
	featureByID := make(map[int64]string, len(records))
	for _, record := range records {
		featureByID[record.MenuID] = record.FeatureCode
	}
	disabled := make([]int64, 0)
	for _, id := range ids {
		if featureCode, ok := featureByID[id]; ok && !s.featureEnabled(featureCode) {
			disabled = append(disabled, id)
		}
	}
	return disabled, nil
}

func (s *Service) ensurePermissionFeaturesEnabled(ctx context.Context, permissionIDs []int64) error {
	disabled, err := s.disabledPermissionIDs(ctx, permissionIDs)
	if err != nil {
		return err
	}
	if len(disabled) > 0 {
		return apperrors.Forbidden("不能分配未启用功能的权限")
	}
	return nil
}

func (s *Service) disabledPermissionIDs(ctx context.Context, permissionIDs []int64) ([]int64, error) {
	ids := uniqueInt64(permissionIDs)
	if len(ids) == 0 || s.features == nil {
		return []int64{}, nil
	}
	records, err := s.repository.ListPermissionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	featureByID := make(map[int64]string, len(records))
	for _, record := range records {
		featureByID[record.PermissionID] = record.FeatureCode
	}
	disabled := make([]int64, 0)
	for _, id := range ids {
		if featureCode, ok := featureByID[id]; ok && !s.featureEnabled(featureCode) {
			disabled = append(disabled, id)
		}
	}
	return disabled, nil
}

func (s *Service) ensurePermissionsExist(ctx context.Context, permissionIDs []int64) error {
	ids := uniqueInt64(permissionIDs)
	if len(ids) == 0 {
		return nil
	}
	count, err := s.repository.CountPermissionsByIDs(ctx, ids)
	if err != nil {
		return err
	}
	if count != len(ids) {
		return apperrors.Params("存在无效权限ID")
	}
	return nil
}

func (s *Service) ensurePermissionCodeExists(ctx context.Context, permissionCode string) error {
	code := strings.TrimSpace(permissionCode)
	if code == "" {
		return apperrors.Params("permissionCode不能为空")
	}
	records, err := s.repository.ListPermissions(ctx, authorizationfacade.PermissionQuery{Code: code})
	if err != nil {
		return err
	}
	for _, item := range records {
		if strings.TrimSpace(item.Code) == code {
			if !s.featureEnabled(item.FeatureCode) {
				return apperrors.Forbidden("不能分配未启用功能的权限")
			}
			return nil
		}
	}
	return apperrors.Params("权限编码不存在")
}

func (s *Service) ensureOperatorCanAssignUserRoles(ctx context.Context, operatorID int64, roleIDs []int64) error {
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, []string{"system:user-role:assign"}); err != nil {
		return err
	}
	adminRole, err := s.roleIDsContainAdminRole(ctx, roleIDs)
	if err != nil {
		return err
	}
	if adminRole {
		if operatorID <= 0 {
			return apperrors.Forbidden("不能分配超级管理员角色")
		}
		roles, err := s.repository.ListUserRoles(ctx, operatorID)
		if err != nil {
			return err
		}
		if !s.domain.IsAdmin(roles) {
			return apperrors.Forbidden("不能分配超级管理员角色")
		}
	}
	codes, err := s.repository.ListPermissionCodesByRoleIDs(ctx, roleIDs)
	if err != nil {
		return err
	}
	return s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, codes)
}

func (s *Service) roleIDsContainAdminRole(ctx context.Context, roleIDs []int64) (bool, error) {
	count, err := s.repository.CountAuthorizationRootRolesByIDs(ctx, uniqueInt64(roleIDs))
	return count > 0, err
}

func (s *Service) ensureOperatorCanGrantPermissionIDs(ctx context.Context, operatorID int64, permissionIDs []int64) error {
	if operatorID <= 0 {
		return nil
	}
	codes, err := s.permissionCodesByIDs(ctx, permissionIDs)
	if err != nil {
		return err
	}
	return s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, codes)
}

func (s *Service) permissionCodesByIDs(ctx context.Context, permissionIDs []int64) ([]string, error) {
	ids := uniqueInt64(permissionIDs)
	if len(ids) == 0 {
		return []string{}, nil
	}
	codeByID, err := s.repository.ListPermissionCodesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(ids))
	for _, id := range ids {
		code, ok := codeByID[id]
		if !ok || code == "" {
			return nil, apperrors.Params("存在无效权限ID")
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func (s *Service) ensureOperatorCanGrantPermissionCodes(ctx context.Context, operatorID int64, permissionCodes []string) error {
	if operatorID <= 0 {
		return nil
	}
	codes := normalizePermissionCodes(permissionCodes)
	if len(codes) == 0 {
		return nil
	}
	userContext, err := s.BuildUserContext(ctx, operatorID, "", nil, nil, "auth-rbac-grant")
	if err != nil {
		return err
	}
	if userContext.IsAdmin {
		return nil
	}
	for _, code := range codes {
		if !permissionCodeSetAllows(userContext.Permissions, code) {
			return apperrors.Forbidden("不能授予当前用户未持有的权限")
		}
	}
	return nil
}

func (s *Service) buildMenuRecord(ctx context.Context, command authorizationfacade.MenuCommand, creating bool) (domain.MenuRecord, error) {
	name := strings.TrimSpace(command.Name)
	menuType := strings.TrimSpace(command.Type)
	if name == "" || menuType == "" {
		return domain.MenuRecord{}, apperrors.Params("菜单名称和类型不能为空")
	}
	parentID := command.ParentID
	if parentID > 0 {
		parent, err := s.repository.FindMenuByID(ctx, parentID)
		if err != nil {
			return domain.MenuRecord{}, err
		}
		if parent == nil {
			return domain.MenuRecord{}, apperrors.Params("父菜单不存在")
		}
	}
	status := 0
	if command.Status != nil {
		status = *command.Status
	}
	sortOrder := 0
	if command.SortOrder != nil {
		sortOrder = *command.SortOrder
	}
	visible := 0
	if command.Visible != nil {
		visible = *command.Visible
	}
	isFrame := 0
	if command.IsFrame != nil {
		isFrame = *command.IsFrame
	}
	isCache := 0
	if command.IsCache != nil {
		isCache = *command.IsCache
	}
	featureCode := features.Code(strings.TrimSpace(command.FeatureCode))
	if featureCode != "" && !features.Known(featureCode) {
		return domain.MenuRecord{}, apperrors.Params("功能能力编码无效")
	}
	_ = creating
	return domain.MenuRecord{
		ParentID: parentID, SortOrder: sortOrder, Name: name, Path: strings.TrimSpace(command.Path),
		Component: strings.TrimSpace(command.Component), Type: menuType, Permission: strings.TrimSpace(command.Permission),
		FeatureCode: string(featureCode), Icon: strings.TrimSpace(command.Icon), Status: status,
		Visible: visible, IsFrame: isFrame, IsCache: isCache,
		Remark: command.Remark,
	}, nil
}

func (s *Service) ensureMenuPermissionUnique(ctx context.Context, menuID int64, permission string) error {
	count, err := s.repository.CountMenuPermissionExcludingID(ctx, menuID, permission)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperrors.Operation("权限标识已存在")
	}
	return nil
}

func (s *Service) buildPermissionRecord(command authorizationfacade.PermissionCommand, creating bool) (domain.PermissionRecord, error) {
	code := strings.TrimSpace(command.Code)
	name := strings.TrimSpace(command.Name)
	if code == "" || name == "" {
		return domain.PermissionRecord{}, apperrors.Params("权限编码和名称不能为空")
	}
	status := 0
	if command.Status != nil {
		status = *command.Status
	}
	resourceType := strings.TrimSpace(command.ResourceType)
	if resourceType == "" {
		resourceType = "API"
	}
	featureCode := features.Code(strings.TrimSpace(command.FeatureCode))
	if featureCode != "" && !features.Known(featureCode) {
		return domain.PermissionRecord{}, apperrors.Params("功能能力编码无效")
	}
	_ = creating
	return domain.PermissionRecord{Code: code, FeatureCode: string(featureCode), Name: name, ResourceType: resourceType, Method: strings.TrimSpace(command.Method), Path: strings.TrimSpace(command.Path), Status: status, Description: command.Description}, nil
}

func (s *Service) ensurePermissionCodeUnique(ctx context.Context, permissionID int64, code string) error {
	count, err := s.repository.CountPermissionCodeExcludingID(ctx, permissionID, code)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperrors.Operation("权限编码已存在")
	}
	return nil
}

func (s *Service) nextID() int64 {
	if s.idGen == nil {
		return time.Now().UTC().UnixNano()
	}
	return s.idGen.NextID()
}

func buildSubjectIdentifier(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

func toChallengeSteps(values []challengefacade.ChallengeStepVO) []authorizationfacade.ChallengeStepVO {
	result := make([]authorizationfacade.ChallengeStepVO, 0, len(values))
	for _, item := range values {
		result = append(result, authorizationfacade.ChallengeStepVO{
			StepIdentifier:        item.StepIdentifier,
			ChallengeType:         item.ChallengeType,
			StepPurpose:           item.StepPurpose,
			StepState:             item.StepState,
			RemainingAttemptCount: item.RemainingAttemptCount,
			CooldownSeconds:       item.CooldownSeconds,
			Switchable:            item.Switchable,
			UserInterfaceHints:    item.UserInterfaceHints,
		})
	}
	return result
}

func extractOrgValues(values []domain.OrgRecord) ([]string, []string) {
	codes := make([]string, 0, len(values))
	names := make([]string, 0, len(values))
	for _, item := range values {
		codes = append(codes, item.Code)
		names = append(names, item.Name)
	}
	return uniqueString(codes), uniqueString(names)
}

func extractDeptValues(values []domain.DeptRecord) ([]string, []string) {
	codes := make([]string, 0, len(values))
	names := make([]string, 0, len(values))
	for _, item := range values {
		codes = append(codes, item.Code)
		names = append(names, item.Name)
	}
	return uniqueString(codes), uniqueString(names)
}

func extractPostValues(values []domain.PostRecord) ([]string, []string) {
	codes := make([]string, 0, len(values))
	names := make([]string, 0, len(values))
	for _, item := range values {
		codes = append(codes, item.Code)
		names = append(names, item.Name)
	}
	return uniqueString(codes), uniqueString(names)
}

func valuesOfMap(values map[int64]string) []string {
	result := make([]string, 0, len(values))
	for _, item := range values {
		if strings.TrimSpace(item) != "" {
			result = append(result, item)
		}
	}
	return uniqueString(result)
}

func uniqueString(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := set[trimmed]; ok {
			continue
		}
		set[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func uniqueInt64(values []int64) []int64 {
	set := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, item := range values {
		if item <= 0 {
			continue
		}
		if _, ok := set[item]; ok {
			continue
		}
		set[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func intersectIDs(values, allowed []int64) []int64 {
	allowedSet := make(map[int64]struct{}, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = struct{}{}
	}
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, id := range values {
		if _, ok := allowedSet[id]; !ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func menuIDs(records []domain.MenuRecord) []int64 {
	result := make([]int64, 0, len(records))
	for _, record := range records {
		result = append(result, record.MenuID)
	}
	return result
}

func permissionIDsFromRecords(records []domain.PermissionRecord) []int64 {
	result := make([]int64, 0, len(records))
	for _, record := range records {
		result = append(result, record.PermissionID)
	}
	return result
}

func toRoleVO(record domain.RoleRecord) authorizationfacade.RoleVO {
	return authorizationfacade.RoleVO{
		ID:                record.RoleID,
		RoleID:            record.RoleID,
		Name:              record.Name,
		Code:              record.Code,
		Type:              roleTypeName(record.Type),
		Status:            record.Status,
		DataScope:         record.DataScope,
		SortOrder:         record.SortOrder,
		Remark:            record.Remark,
		CreateTime:        record.CreateTime,
		UpdateTime:        record.UpdateTime,
		SystemManaged:     record.IsSystem(),
		AuthorizationRoot: record.IsAuthorizationRoot(),
		GrantRevision:     record.GrantRevision,
	}
}

func toRoleVOs(records []domain.RoleRecord) []authorizationfacade.RoleVO {
	result := make([]authorizationfacade.RoleVO, 0, len(records))
	for _, record := range records {
		result = append(result, toRoleVO(record))
	}
	return result
}

func toMenuVO(record domain.MenuRecord) authorizationfacade.MenuTreeNodeVO {
	return authorizationfacade.MenuTreeNodeVO{
		ID:          record.MenuID,
		MenuID:      record.MenuID,
		ParentID:    record.ParentID,
		SortOrder:   record.SortOrder,
		Name:        record.Name,
		Path:        record.Path,
		Component:   record.Component,
		Type:        record.Type,
		Permission:  record.Permission,
		FeatureCode: record.FeatureCode,
		Icon:        record.Icon,
		Status:      record.Status,
		Visible:     record.Visible,
		IsFrame:     record.IsFrame,
		IsCache:     record.IsCache,
		Remark:      record.Remark,
		CreateTime:  record.CreateTime,
		UpdateTime:  record.UpdateTime,
	}
}

func toMenuVOs(records []domain.MenuRecord) []authorizationfacade.MenuTreeNodeVO {
	result := make([]authorizationfacade.MenuTreeNodeVO, 0, len(records))
	for _, record := range records {
		result = append(result, toMenuVO(record))
	}
	return result
}

func toPermissionVO(record domain.PermissionRecord) authorizationfacade.PermissionVO {
	return authorizationfacade.PermissionVO{
		ID:           record.PermissionID,
		Code:         record.Code,
		FeatureCode:  record.FeatureCode,
		Name:         record.Name,
		ResourceType: record.ResourceType,
		Method:       record.Method,
		Path:         record.Path,
		Status:       record.Status,
		Description:  record.Description,
		CreateTime:   record.CreateTime,
		UpdateTime:   record.UpdateTime,
	}
}

func toPermissionVOs(records []domain.PermissionRecord) []authorizationfacade.PermissionVO {
	result := make([]authorizationfacade.PermissionVO, 0, len(records))
	for _, record := range records {
		result = append(result, toPermissionVO(record))
	}
	return result
}

func roleTypeCode(value string) int {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "SYSTEM":
		return 1
	case "BUSINESS":
		return 2
	case "CUSTOM", "":
		return 3
	default:
		return 3
	}
}

func roleTypeName(value int) string {
	switch value {
	case 1:
		return "SYSTEM"
	case 2:
		return "BUSINESS"
	case 3:
		return "CUSTOM"
	default:
		return "CUSTOM"
	}
}

func normalizePage(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 200 {
		size = 200
	}
	return current, size
}

func permissionMatches(candidate string, permission string) bool {
	candidate = strings.TrimSpace(candidate)
	permission = strings.TrimSpace(permission)
	if candidate == "" || permission == "" {
		return false
	}
	if candidate == "*" || candidate == permission {
		return true
	}
	if strings.HasSuffix(candidate, ":*") {
		prefix := strings.TrimSuffix(candidate, "*")
		return strings.HasPrefix(permission, prefix)
	}
	return false
}

func normalizePermissionCodes(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		code := strings.TrimSpace(value)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	return result
}

func permissionCodeSetAllows(held []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return false
	}
	for _, candidate := range held {
		if permissionMatches(candidate, requested) {
			return true
		}
	}
	return false
}

func buildMenuTree(values []authorizationfacade.MenuTreeNodeVO) []authorizationfacade.MenuTreeNodeVO {
	nodes := make(map[int64]authorizationfacade.MenuTreeNodeVO, len(values))
	order := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		value.Children = nil
		nodes[value.MenuID] = value
		order = append(order, value.MenuID)
	}
	children := make(map[int64][]int64, len(values))
	rootIDs := make([]int64, 0)
	for _, id := range order {
		node := nodes[id]
		if node.ParentID <= 0 {
			rootIDs = append(rootIDs, id)
			continue
		}
		if _, ok := nodes[node.ParentID]; !ok {
			rootIDs = append(rootIDs, id)
			continue
		}
		children[node.ParentID] = append(children[node.ParentID], id)
	}
	var materialize func(id int64) authorizationfacade.MenuTreeNodeVO
	materialize = func(id int64) authorizationfacade.MenuTreeNodeVO {
		node := nodes[id]
		if _, ok := seen[id]; ok {
			node.Children = nil
			return node
		}
		seen[id] = struct{}{}
		node.Children = make([]authorizationfacade.MenuTreeNodeVO, 0, len(children[id]))
		for _, childID := range children[id] {
			node.Children = append(node.Children, materialize(childID))
		}
		delete(seen, id)
		return node
	}
	result := make([]authorizationfacade.MenuTreeNodeVO, 0, len(rootIDs))
	for _, id := range rootIDs {
		result = append(result, materialize(id))
	}
	return result
}

func pruneEmptyMenuContainers(values []authorizationfacade.MenuTreeNodeVO) []authorizationfacade.MenuTreeNodeVO {
	result := make([]authorizationfacade.MenuTreeNodeVO, 0, len(values))
	for _, value := range values {
		value.Children = pruneEmptyMenuContainers(value.Children)
		if strings.EqualFold(strings.TrimSpace(value.Type), "M") && len(value.Children) == 0 {
			continue
		}
		result = append(result, value)
	}
	return result
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func userRoleAssignmentBinding(userID int64, roleIDs []int64) string {
	return fmt.Sprintf("user:%d|roles:%s", userID, joinSortedIDs(roleIDs))
}

func createdUserRoleAssignmentBinding(username string, roleIDs []int64) string {
	return fmt.Sprintf("user:create:%s|roles:%s", strings.TrimSpace(username), joinSortedIDs(roleIDs))
}

func rolePermissionAssignmentBinding(roleID int64, permissionIDs, menuIDs []int64) string {
	return fmt.Sprintf("role:%d|permissions:%s|menus:%s", roleID, joinSortedIDs(permissionIDs), joinSortedIDs(menuIDs))
}

func roleMenuAssignmentBinding(roleID int64, menuIDs []int64) string {
	return fmt.Sprintf("role:%d|menus:%s", roleID, joinSortedIDs(menuIDs))
}

func roleDeptAssignmentBinding(roleID int64, deptIDs []int64) string {
	return fmt.Sprintf("role:%d|depts:%s", roleID, joinSortedIDs(deptIDs))
}

func menuPermissionAssignmentBinding(menuID int64, permissionIDs []int64) string {
	return fmt.Sprintf("menu:%d|permissions:%s", menuID, joinSortedIDs(permissionIDs))
}

func joinSortedIDs(ids []int64) string {
	normalized := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, 0, len(normalized))
	for _, id := range normalized {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}
