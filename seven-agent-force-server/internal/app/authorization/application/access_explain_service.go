package application

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type AccessExplainRepository interface {
	FindAccessUser(ctx context.Context, userID int64) (*domain.AccessUserRecord, error)
	ListAccessRoleSources(ctx context.Context, userID int64) ([]domain.AccessRoleSourceRecord, error)
	ListAccessGrantRecords(ctx context.Context, userID int64) ([]domain.AccessGrantRecord, error)
	ListAccessRoleDeptRecords(ctx context.Context, roleIDs []int64) ([]domain.AccessRoleDeptRecord, error)
	ListAccessMemberships(ctx context.Context, userID int64) ([]domain.AccessMembershipRecord, error)
	ListDeptIDsByHierarchies(ctx context.Context, hierarchies []string) (map[string][]int64, error)
	ListAllMenus(ctx context.Context) ([]domain.MenuRecord, error)
}

type AccessExplainService struct {
	repository  AccessExplainRepository
	domain      *domain.Service
	features    features.Set
	now         func() time.Time
	snapshotter store.Snapshotter
}

func NewAccessExplainService(repository AccessExplainRepository, domainService *domain.Service, featureSet features.Set, snapshotters ...store.Snapshotter) *AccessExplainService {
	service := &AccessExplainService{
		repository: repository,
		domain:     domainService,
		features:   featureSet,
		now:        func() time.Time { return time.Now().UTC() },
	}
	if len(snapshotters) > 0 {
		service.snapshotter = snapshotters[0]
	}
	return service
}

func (s *AccessExplainService) GetEffectiveAccess(ctx context.Context, userID int64, query authorizationfacade.EffectiveAccessQuery) (*authorizationfacade.EffectiveAccessVO, error) {
	snapshot, err := s.loadSnapshot(ctx, userID)
	if err != nil {
		return nil, err
	}
	permissions := filterEffectivePermissions(snapshot.permissions, query)
	current, size := normalizeAccessPage(query.Current, query.Size)
	total := int64(len(permissions))
	start := int((current - 1) * size)
	if start > len(permissions) {
		start = len(permissions)
	}
	end := start + int(size)
	if end > len(permissions) {
		end = len(permissions)
	}
	return &authorizationfacade.EffectiveAccessVO{
		UserID:            snapshot.user.UserID,
		Username:          snapshot.user.Username,
		Status:            snapshot.user.Status,
		AuthorizationRoot: snapshot.authorizationRoot,
		RoleSources:       snapshot.roleSources,
		DataScope:         snapshot.dataScope,
		PermissionSummary: snapshot.summary,
		Permissions: authorizationfacade.EffectivePermissionPageVO{
			Current: current,
			Size:    size,
			Total:   total,
			Records: append([]authorizationfacade.EffectivePermissionVO{}, permissions[start:end]...),
		},
	}, nil
}

func (s *AccessExplainService) ExplainPermission(ctx context.Context, userID int64, permissionCode string) (*authorizationfacade.PermissionExplainVO, error) {
	permissionCode = strings.TrimSpace(permissionCode)
	if permissionCode == "" {
		return nil, apperrors.Params("permissionCode不能为空")
	}
	if len(permissionCode) > 100 {
		return nil, apperrors.Params("permissionCode长度不能超过100")
	}
	snapshot, err := s.loadSnapshot(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := &authorizationfacade.PermissionExplainVO{
		UserID:                 userID,
		PermissionCode:         permissionCode,
		Decision:               authorizationfacade.AccessDecisionDeny,
		ReasonCode:             "PERMISSION_NOT_GRANTED",
		MatchedPermissionCodes: []string{},
		Chains:                 []authorizationfacade.PermissionGrantChainVO{},
		EvaluatedAt:            s.now(),
	}
	if snapshot.user.Status != 0 {
		result.ReasonCode = "USER_INACTIVE"
		return result, nil
	}
	if snapshot.authorizationRoot {
		result.Decision = authorizationfacade.AccessDecisionAllow
		result.ReasonCode = "AUTHORIZATION_ROOT_BYPASS"
		return result, nil
	}

	matched := make([]authorizationfacade.EffectivePermissionVO, 0)
	for _, permission := range snapshot.permissions {
		if securitycontext.PermissionMatches(permission.PermissionCode, permissionCode) {
			matched = append(matched, permission)
			result.MatchedPermissionCodes = append(result.MatchedPermissionCodes, permission.PermissionCode)
			result.Chains = append(result.Chains, permission.Grants...)
			if result.Feature == nil && permission.FeatureCode != "" {
				result.Feature = &authorizationfacade.PermissionFeatureVO{Code: permission.FeatureCode, Enabled: permission.FeatureEnabled}
			}
		}
	}
	result.MatchedPermissionCodes = uniqueSortedStrings(result.MatchedPermissionCodes)
	if len(matched) == 0 {
		return result, nil
	}
	for _, permission := range matched {
		if !permission.Effective {
			continue
		}
		result.Decision = authorizationfacade.AccessDecisionAllow
		if permission.PermissionCode != permissionCode {
			result.ReasonCode = "WILDCARD_PERMISSION_MATCH"
		} else {
			result.ReasonCode = primaryAllowReason(permission.Grants)
		}
		return result, nil
	}
	result.ReasonCode = primaryDenyReason(result.Chains)
	return result, nil
}

type accessSnapshot struct {
	user              *domain.AccessUserRecord
	authorizationRoot bool
	roleSources       []authorizationfacade.AccessRoleSourceVO
	dataScope         authorizationfacade.EffectiveDataScopeVO
	permissions       []authorizationfacade.EffectivePermissionVO
	summary           authorizationfacade.PermissionSummaryVO
}

func (s *AccessExplainService) loadSnapshot(ctx context.Context, userID int64) (*accessSnapshot, error) {
	if userID <= 0 {
		return nil, apperrors.Params("用户ID不能为空")
	}
	if s.snapshotter == nil || !s.snapshotter.Enabled() {
		return nil, apperrors.System("访问解释一致性快照能力未配置")
	}
	var snapshot *accessSnapshot
	err := s.snapshotter.WithinReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		var loadErr error
		snapshot, loadErr = s.loadSnapshotQueries(snapshotCtx, userID)
		return loadErr
	})
	return snapshot, err
}

func (s *AccessExplainService) loadSnapshotQueries(ctx context.Context, userID int64) (*accessSnapshot, error) {
	user, err := s.repository.FindAccessUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, apperrors.NotFound("用户不存在")
	}
	roleRecords, err := s.repository.ListAccessRoleSources(ctx, userID)
	if err != nil {
		return nil, err
	}
	grantRecords, err := s.repository.ListAccessGrantRecords(ctx, userID)
	if err != nil {
		return nil, err
	}
	roleIDs := make([]int64, 0, len(roleRecords))
	for _, role := range roleRecords {
		roleIDs = append(roleIDs, role.RoleID)
	}
	roleDepts, err := s.repository.ListAccessRoleDeptRecords(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	memberships, err := s.repository.ListAccessMemberships(ctx, userID)
	if err != nil {
		return nil, err
	}
	hierarchies := make([]string, 0)
	for _, membership := range memberships {
		if membership.Kind == "DEPT" && strings.TrimSpace(membership.Hierarchy) != "" {
			hierarchies = append(hierarchies, membership.Hierarchy)
		}
	}
	descendants, err := s.repository.ListDeptIDsByHierarchies(ctx, uniqueSortedStrings(hierarchies))
	if err != nil {
		return nil, err
	}
	menus, err := s.repository.ListAllMenus(ctx)
	if err != nil {
		return nil, err
	}

	roleSources, authorizationRoot := buildRoleSources(roleRecords)
	dataScope := s.buildDataScope(userID, roleRecords, roleDepts, memberships, descendants)
	permissions, summary := s.buildPermissions(grantRecords, menus)
	return &accessSnapshot{
		user:              user,
		authorizationRoot: authorizationRoot,
		roleSources:       roleSources,
		dataScope:         dataScope,
		permissions:       permissions,
		summary:           summary,
	}, nil
}

func buildRoleSources(records []domain.AccessRoleSourceRecord) ([]authorizationfacade.AccessRoleSourceVO, bool) {
	result := make([]authorizationfacade.AccessRoleSourceVO, 0, len(records))
	authorizationRoot := false
	for _, record := range records {
		item := authorizationfacade.AccessRoleSourceVO{
			RoleID:               record.RoleID,
			RoleCode:             record.RoleCode,
			RoleName:             record.RoleName,
			RoleStatus:           record.RoleStatus,
			DeclaredDataScope:    dataScopeTypeForRole(record.RoleDataScope),
			RoleAssignmentSource: record.AssignmentSource,
		}
		if record.AssignmentSource == "POST" {
			item.Post = postSource(record.PostID, record.PostCode, record.PostName, record.PostDeptID, record.PostOrgID)
		}
		if record.RoleStatus == 0 && record.AssignmentSource == "DIRECT_USER" && record.RoleSystemKey == domain.AuthorizationRootSystemKey {
			authorizationRoot = true
		}
		result = append(result, item)
	}
	return result, authorizationRoot
}

func (s *AccessExplainService) buildDataScope(userID int64, sourceRecords []domain.AccessRoleSourceRecord, roleDepts []domain.AccessRoleDeptRecord, memberships []domain.AccessMembershipRecord, descendants map[string][]int64) authorizationfacade.EffectiveDataScopeVO {
	roleByID := make(map[int64]domain.RoleRecord)
	roleDeptMap := make(map[int64][]int64)
	for _, row := range roleDepts {
		roleDeptMap[row.RoleID] = append(roleDeptMap[row.RoleID], row.DeptID)
	}
	deptIDs := []int64{}
	orgIDs := []int64{}
	deptHierarchy := make(map[int64]string)
	for _, membership := range memberships {
		switch membership.Kind {
		case "ORG":
			orgIDs = append(orgIDs, membership.ID)
		case "DEPT":
			deptIDs = append(deptIDs, membership.ID)
			deptHierarchy[membership.ID] = membership.Hierarchy
		}
	}
	for _, source := range sourceRecords {
		if source.RoleStatus != 0 {
			continue
		}
		if _, exists := roleByID[source.RoleID]; !exists {
			roleByID[source.RoleID] = domain.RoleRecord{
				RoleID: source.RoleID, Code: source.RoleCode, Name: source.RoleName,
				SystemKey: source.RoleSystemKey, Status: source.RoleStatus, DataScope: source.RoleDataScope,
			}
		}
	}
	roles := make([]domain.RoleRecord, 0, len(roleByID))
	customDeptIDs := []int64{}
	for _, role := range roleByID {
		roles = append(roles, role)
		customDeptIDs = append(customDeptIDs, roleDeptMap[role.RoleID]...)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].RoleID < roles[j].RoleID })
	effective := s.domain.EffectiveDataScope(roles, customDeptIDs, deptIDs, orgIDs, deptHierarchy, descendants)
	best := 99
	for _, role := range roles {
		if role.DataScope >= 1 && role.DataScope <= 5 && role.DataScope < best {
			best = role.DataScope
		}
	}
	contributors := make([]authorizationfacade.DataScopeContributorVO, 0, len(roles))
	for _, role := range roles {
		contributorDepts := []int64{}
		switch role.DataScope {
		case 2:
			contributorDepts = roleDeptMap[role.RoleID]
		case 3, 5:
			contributorDepts = deptIDs
		case 4:
			for _, deptID := range deptIDs {
				contributorDepts = append(contributorDepts, descendants[deptHierarchy[deptID]]...)
			}
		}
		contributors = append(contributors, authorizationfacade.DataScopeContributorVO{
			RoleID: role.RoleID, RoleCode: role.Code, DeclaredScopeType: dataScopeTypeForRole(role.DataScope),
			Winning: role.DataScope == best, DeptIDs: uniqueSortedInt64(contributorDepts),
		})
	}
	return authorizationfacade.EffectiveDataScopeVO{
		UserID: userID, ScopeType: string(effective.ScopeType),
		DeptIDs: uniqueSortedInt64(effective.DeptIDs), OrgIDs: uniqueSortedInt64(effective.OrgIDs), Contributors: contributors,
	}
}

type permissionAggregate struct {
	vo        authorizationfacade.EffectivePermissionVO
	temporary bool
	chainKeys map[string]struct{}
}

func (s *AccessExplainService) buildPermissions(records []domain.AccessGrantRecord, menus []domain.MenuRecord) ([]authorizationfacade.EffectivePermissionVO, authorizationfacade.PermissionSummaryVO) {
	menuPaths := buildMenuPaths(menus)
	aggregates := make(map[string]*permissionAggregate)
	now := s.now()
	for _, record := range records {
		code := strings.TrimSpace(record.PermissionCode)
		if code == "" {
			continue
		}
		aggregate := aggregates[code]
		if aggregate == nil {
			aggregate = &permissionAggregate{vo: authorizationfacade.EffectivePermissionVO{
				PermissionID: record.PermissionID, PermissionCode: code, PermissionName: record.PermissionName,
				FeatureCode: strings.TrimSpace(record.FeatureCode), FeatureEnabled: true,
				Grants: []authorizationfacade.PermissionGrantChainVO{},
			}, chainKeys: make(map[string]struct{})}
			aggregates[code] = aggregate
		}
		if aggregate.vo.PermissionID == 0 && record.PermissionID > 0 {
			aggregate.vo.PermissionID = record.PermissionID
		}
		if aggregate.vo.PermissionName == "" {
			aggregate.vo.PermissionName = record.PermissionName
		}
		if aggregate.vo.FeatureCode == "" && strings.TrimSpace(record.FeatureCode) != "" {
			aggregate.vo.FeatureCode = strings.TrimSpace(record.FeatureCode)
		}
		featureEnabled := s.featureEnabled(record.FeatureCode)
		chain := grantChain(record, menuPaths[record.MenuID], now, featureEnabled)
		key := chainKey(chain)
		if _, exists := aggregate.chainKeys[key]; exists {
			continue
		}
		aggregate.chainKeys[key] = struct{}{}
		aggregate.vo.Grants = append(aggregate.vo.Grants, chain)
		aggregate.vo.Effective = aggregate.vo.Effective || chain.Active
		if record.GrantSource == "TEMPORARY" && record.PermissionType == 1 {
			aggregate.temporary = true
		}
	}
	permissions := make([]authorizationfacade.EffectivePermissionVO, 0, len(aggregates))
	summary := authorizationfacade.PermissionSummaryVO{}
	for _, aggregate := range aggregates {
		aggregate.vo.FeatureEnabled = s.featureEnabled(aggregate.vo.FeatureCode)
		sort.SliceStable(aggregate.vo.Grants, func(i, j int) bool {
			return allowChainPriority(aggregate.vo.Grants[i]) < allowChainPriority(aggregate.vo.Grants[j])
		})
		permissions = append(permissions, aggregate.vo)
		if aggregate.vo.Effective {
			summary.EffectiveCount++
		} else if !aggregate.vo.FeatureEnabled {
			summary.FilteredCount++
		}
		if aggregate.temporary {
			summary.TemporaryCount++
		}
	}
	sort.Slice(permissions, func(i, j int) bool { return permissions[i].PermissionCode < permissions[j].PermissionCode })
	return permissions, summary
}

func (s *AccessExplainService) featureEnabled(featureCode string) bool {
	code := features.Code(strings.TrimSpace(featureCode))
	return code == "" || s.features == nil || s.features.Enabled(code)
}

func grantChain(record domain.AccessGrantRecord, menuPath string, now time.Time, featureEnabled bool) authorizationfacade.PermissionGrantChainVO {
	chain := authorizationfacade.PermissionGrantChainVO{
		PermissionGrantSource: record.GrantSource,
		RoleID:                record.RoleID, RoleCode: record.RoleCode, RoleName: record.RoleName,
		RoleAssignmentSource: record.AssignmentSource,
		MenuID:               record.MenuID, MenuName: record.MenuName, MenuPath: menuPath,
		GrantedBy: record.GrantedBy, Source: record.Source, ExpireAt: record.ExpireAt,
		Active: true,
	}
	if record.AssignmentSource == "POST" {
		chain.Post = postSource(record.PostID, record.PostCode, record.PostName, record.PostDeptID, record.PostOrgID)
	}
	switch {
	case record.GrantSource != "TEMPORARY" && record.RoleStatus != 0:
		chain.Active, chain.ReasonCode = false, "ROLE_DISABLED"
	case record.PermissionStatus != 0:
		chain.Active, chain.ReasonCode = false, "PERMISSION_DISABLED"
	case record.GrantSource == "MENU_DERIVED" && record.MenuStatus != 0:
		chain.Active, chain.ReasonCode = false, "MENU_DISABLED"
	case record.GrantSource == "TEMPORARY" && record.PermissionType == 1 && record.ExpireAt != nil && !record.ExpireAt.After(now):
		chain.Active, chain.ReasonCode = false, "TEMPORARY_PERMISSION_EXPIRED"
	case !featureEnabled:
		chain.Active, chain.ReasonCode = false, "FEATURE_DISABLED"
	default:
		chain.ReasonCode = allowReason(chain)
	}
	return chain
}

func filterEffectivePermissions(records []authorizationfacade.EffectivePermissionVO, query authorizationfacade.EffectiveAccessQuery) []authorizationfacade.EffectivePermissionVO {
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	sourceType := strings.ToUpper(strings.TrimSpace(query.SourceType))
	result := make([]authorizationfacade.EffectivePermissionVO, 0, len(records))
	for _, record := range records {
		if keyword != "" && !strings.Contains(strings.ToLower(record.PermissionCode+" "+record.PermissionName), keyword) {
			continue
		}
		if query.Effective != nil && record.Effective != *query.Effective {
			continue
		}
		if sourceType != "" {
			matched := false
			for _, chain := range record.Grants {
				if strings.EqualFold(chain.PermissionGrantSource, sourceType) || strings.EqualFold(chain.RoleAssignmentSource, sourceType) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		result = append(result, record)
	}
	return result
}

func buildMenuPaths(menus []domain.MenuRecord) map[int64]string {
	byID := make(map[int64]domain.MenuRecord, len(menus))
	for _, menu := range menus {
		byID[menu.MenuID] = menu
	}
	result := make(map[int64]string, len(menus))
	for _, menu := range menus {
		names := []string{}
		seen := map[int64]struct{}{}
		current := menu
		for current.MenuID > 0 {
			if _, exists := seen[current.MenuID]; exists {
				break
			}
			seen[current.MenuID] = struct{}{}
			if strings.TrimSpace(current.Name) != "" {
				names = append(names, strings.TrimSpace(current.Name))
			}
			parent, exists := byID[current.ParentID]
			if !exists {
				break
			}
			current = parent
		}
		for left, right := 0, len(names)-1; left < right; left, right = left+1, right-1 {
			names[left], names[right] = names[right], names[left]
		}
		result[menu.MenuID] = strings.Join(names, " / ")
	}
	return result
}

func primaryAllowReason(chains []authorizationfacade.PermissionGrantChainVO) string {
	bestPriority := 99
	reason := "PERMISSION_NOT_GRANTED"
	for _, chain := range chains {
		if !chain.Active {
			continue
		}
		if priority := allowChainPriority(chain); priority < bestPriority {
			bestPriority = priority
			reason = allowReason(chain)
		}
	}
	return reason
}

func allowChainPriority(chain authorizationfacade.PermissionGrantChainVO) int {
	switch {
	case chain.PermissionGrantSource == "TEMPORARY":
		return 1
	case chain.PermissionGrantSource == "ROLE_DIRECT" && chain.RoleAssignmentSource == "DIRECT_USER":
		return 2
	case chain.PermissionGrantSource == "ROLE_DIRECT" && chain.RoleAssignmentSource == "POST":
		return 3
	case chain.PermissionGrantSource == "MENU_DERIVED" && chain.RoleAssignmentSource == "DIRECT_USER":
		return 4
	case chain.PermissionGrantSource == "MENU_DERIVED" && chain.RoleAssignmentSource == "POST":
		return 5
	default:
		return 10
	}
}

func allowReason(chain authorizationfacade.PermissionGrantChainVO) string {
	switch {
	case chain.PermissionGrantSource == "TEMPORARY":
		return "TEMPORARY_PERMISSION_ACTIVE"
	case chain.PermissionGrantSource == "MENU_DERIVED":
		return "MENU_DERIVED_PERMISSION"
	case chain.RoleAssignmentSource == "POST":
		return "POST_ROLE_PERMISSION"
	default:
		return "DIRECT_ROLE_PERMISSION"
	}
}

func primaryDenyReason(chains []authorizationfacade.PermissionGrantChainVO) string {
	priority := map[string]int{
		"FEATURE_DISABLED": 1, "TEMPORARY_PERMISSION_EXPIRED": 2, "ROLE_DISABLED": 3,
		"PERMISSION_DISABLED": 4, "MENU_DISABLED": 5,
	}
	best := 99
	reason := "PERMISSION_NOT_GRANTED"
	for _, chain := range chains {
		if value, ok := priority[chain.ReasonCode]; ok && value < best {
			best, reason = value, chain.ReasonCode
		}
	}
	return reason
}

func dataScopeTypeForRole(value int) string {
	switch value {
	case 1:
		return string(securitycontext.DataScopeAll)
	case 2:
		return string(securitycontext.DataScopeCustom)
	case 3:
		return string(securitycontext.DataScopeDept)
	case 4:
		return string(securitycontext.DataScopeDeptAndChild)
	case 5:
		return string(securitycontext.DataScopeSelf)
	default:
		return string(securitycontext.DataScopeNone)
	}
}

func postSource(id int64, code, name string, deptID, orgID int64) *authorizationfacade.AccessPostSourceVO {
	return &authorizationfacade.AccessPostSourceVO{PostID: id, PostCode: code, PostName: name, DeptID: deptID, OrgID: orgID}
}

func chainKey(chain authorizationfacade.PermissionGrantChainVO) string {
	postID := int64(0)
	if chain.Post != nil {
		postID = chain.Post.PostID
	}
	return strings.Join([]string{
		chain.PermissionGrantSource, chain.RoleAssignmentSource, chain.RoleCode,
		accessInt64String(chain.RoleID), accessInt64String(chain.MenuID), accessInt64String(chain.GrantedBy),
		accessInt64String(postID),
	}, "|")
}

func accessInt64String(value int64) string {
	return strconv.FormatInt(value, 10)
}

func uniqueSortedInt64(values []int64) []int64 {
	set := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			if _, exists := set[value]; !exists {
				set[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueSortedStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := set[value]; !exists {
			set[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func normalizeAccessPage(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return current, size
}
