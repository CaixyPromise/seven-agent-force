package application

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
)

func TestFeatureFilteringCoversAuthorizationReadModels(t *testing.T) {
	repo := newFeatureFilterRepo()
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
		features.PlatformControl: {},
	})

	permissions, err := service.GetUserPermissions(context.Background(), 1001)
	if err != nil {
		t.Fatalf("get user permissions: %v", err)
	}
	if !reflect.DeepEqual(permissions, []string{"system:platform:list", "system:user:list"}) {
		t.Fatalf("unexpected current permissions: %#v", permissions)
	}

	currentMenus, err := service.GetCurrentUserMenus(context.Background(), 1001)
	if err != nil {
		t.Fatalf("get current menus: %v", err)
	}
	if got := flattenMenuIDs(currentMenus); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("unexpected current menu ids: %#v", got)
	}
	if currentMenus[0].Children[0].FeatureCode != string(features.PlatformControl) {
		t.Fatalf("featureCode missing from menu DTO: %#v", currentMenus[0].Children[0])
	}

	menuTree, err := service.GetMenuTree(context.Background(), false)
	if err != nil {
		t.Fatalf("get menu tree: %v", err)
	}
	if got := flattenMenuIDs(menuTree); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("unexpected menu tree ids: %#v", got)
	}

	roleMenuIDs, err := service.GetRoleMenuIDs(context.Background(), 10)
	if err != nil {
		t.Fatalf("get role menu ids: %v", err)
	}
	if !reflect.DeepEqual(roleMenuIDs, []int64{1, 2}) {
		t.Fatalf("unexpected role menu ids: %#v", roleMenuIDs)
	}

	roleTree, err := service.GetRoleMenuTree(context.Background(), 10)
	if err != nil {
		t.Fatalf("get role menu tree: %v", err)
	}
	if got := flattenMenuIDs(roleTree); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("unexpected role menu tree ids: %#v", got)
	}

	permissionList, err := service.ListPermissions(context.Background(), authorizationfacade.PermissionQuery{})
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if got := permissionVOCodes(permissionList); !reflect.DeepEqual(got, []string{"system:user:list", "system:platform:list"}) {
		t.Fatalf("unexpected permission list: %#v", got)
	}
	if permissionList[1].FeatureCode != string(features.PlatformControl) {
		t.Fatalf("featureCode missing from permission DTO: %#v", permissionList[1])
	}

	page, err := service.PagePermissions(context.Background(), authorizationfacade.PermissionPageQuery{Current: 1, Size: 1})
	if err != nil {
		t.Fatalf("page permissions: %v", err)
	}
	if page.Total != 2 || len(page.Records) != 1 || page.Records[0].Code != "system:platform:list" {
		t.Fatalf("unexpected filtered page: %#v", page)
	}

	menuPermissionIDs, err := service.GetMenuPermissionIDs(context.Background(), 1)
	if err != nil {
		t.Fatalf("get menu permission ids: %v", err)
	}
	if !reflect.DeepEqual(menuPermissionIDs, []int64{101, 102}) {
		t.Fatalf("unexpected menu permission ids: %#v", menuPermissionIDs)
	}
}

func TestPagePermissionsUsesBoundedFeatureAwareRepositoryPagination(t *testing.T) {
	repo := newFeatureFilterRepo()
	repo.permissions = make([]domain.PermissionRecord, 0, 5000)
	expectedVisible := make([]domain.PermissionRecord, 0, 3334)
	for index := 0; index < 5000; index++ {
		record := domain.PermissionRecord{
			PermissionID: int64(index + 1),
			Code:         fmt.Sprintf("permission:%05d", index),
			Name:         fmt.Sprintf("Permission %05d", index),
		}
		switch index % 3 {
		case 1:
			record.FeatureCode = string(features.PlatformControl)
		case 2:
			record.FeatureCode = string(features.DockerAdmin)
		}
		repo.permissions = append(repo.permissions, record)
		if index%3 != 2 {
			expectedVisible = append(expectedVisible, record)
		}
	}
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
		features.PlatformControl: {},
	})

	page, err := service.PagePermissions(context.Background(), authorizationfacade.PermissionPageQuery{Current: 100, Size: 20})
	if err != nil {
		t.Fatalf("PagePermissions(): %v", err)
	}
	start := (100 - 1) * 20
	if page.Total != int64(len(expectedVisible)) || len(page.Records) != 20 {
		t.Fatalf("page total=%d records=%d, want total=%d records=20", page.Total, len(page.Records), len(expectedVisible))
	}
	if page.Records[0].Code != expectedVisible[start].Code {
		t.Fatalf("first page code=%q, want %q", page.Records[0].Code, expectedVisible[start].Code)
	}
	if repo.listPermissionCalls != 0 || repo.pagePermissionCalls != 1 {
		t.Fatalf("permission queries list=%d page=%d, want bounded page path 0/1", repo.listPermissionCalls, repo.pagePermissionCalls)
	}
	if !repo.pageFilterFeatures || !reflect.DeepEqual(repo.pageFeatureCodes, []string{string(features.PlatformControl)}) {
		t.Fatalf("feature filter enabled=%v codes=%v", repo.pageFilterFeatures, repo.pageFeatureCodes)
	}
}

func TestCurrentUserMenusPruneContainersWithoutRenderableChildren(t *testing.T) {
	repo := newFeatureFilterRepo()
	repo.menus = append(repo.menus,
		domain.MenuRecord{
			MenuID:    4,
			Name:      "Disabled feature group",
			Type:      "M",
			Component: "Layout",
		},
		domain.MenuRecord{
			MenuID:      5,
			ParentID:    4,
			Name:        "Docker",
			Type:        "C",
			FeatureCode: string(features.DockerAdmin),
		},
		domain.MenuRecord{
			MenuID:    6,
			Name:      "Empty group",
			Type:      "M",
			Component: "Layout",
		},
		domain.MenuRecord{
			MenuID: 7,
			Name:   "Duplicate empty root",
			Path:   "/system",
			Type:   "M",
		},
	)
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
		features.PlatformControl: {},
	})

	currentMenus, err := service.GetCurrentUserMenus(context.Background(), 1001)
	if err != nil {
		t.Fatalf("get current menus: %v", err)
	}
	if got := flattenMenuIDs(currentMenus); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("empty menu containers must not reach current navigation: %#v", got)
	}
}

func TestNilFeatureSetKeepsLegacyAuthorizationBehavior(t *testing.T) {
	repo := newFeatureFilterRepo()
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)

	permissions, err := service.GetUserPermissions(context.Background(), 1001)
	if err != nil {
		t.Fatalf("get user permissions: %v", err)
	}
	if len(permissions) != 3 {
		t.Fatalf("nil feature set should preserve legacy permissions, got %#v", permissions)
	}
	menus, err := service.GetMenuTree(context.Background(), false)
	if err != nil {
		t.Fatalf("get menu tree: %v", err)
	}
	if got := flattenMenuIDs(menus); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Fatalf("nil feature set should preserve legacy menus, got %#v", got)
	}
}

func TestGovernedAuthorizationFeatureFingerprintIsScopedByEnabledFeatures(t *testing.T) {
	legacy := NewService(nilConfig(), nil, nil, nil, nil, nil, nil, nil, nil, nil)
	platform := NewService(nilConfig(), nil, nil, nil, nil, nil, nil, nil, nil, nil, features.Set{
		features.PlatformControl: {},
	})
	local := NewService(nilConfig(), nil, nil, nil, nil, nil, nil, nil, nil, nil, features.Set{})

	if legacy.authorizationFeatureFingerprint() == "" {
		t.Fatal("nil feature set must produce an opaque governed-cache fingerprint")
	}
	if platform.authorizationFeatureFingerprint() == local.authorizationFeatureFingerprint() {
		t.Fatalf("different feature sets must not share governed authorization snapshots")
	}
}

func TestFeatureCodeWritesRejectUnknownCapabilities(t *testing.T) {
	service := NewService(nilConfig(), nil, nil, newFeatureFilterRepo(), domain.NewService(), nil, nil, nil, nil, nil, features.Set{})

	if _, err := service.buildMenuRecord(context.Background(), authorizationfacade.MenuCommand{
		Name: "Unknown feature menu", Type: "C", FeatureCode: "future.capability",
	}, true); apperrors.From(err) == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("unknown menu feature code must be rejected, got %v", err)
	}
	if _, err := service.buildPermissionRecord(authorizationfacade.PermissionCommand{
		Code: "future:test", Name: "Unknown feature permission", FeatureCode: "future.capability",
	}, true); apperrors.From(err) == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("unknown permission feature code must be rejected, got %v", err)
	}
}

func TestTemporaryPermissionMutationsRequireReasonAndBindProof(t *testing.T) {
	repo := newFeatureFilterRepo()
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	expireAt := time.Now().UTC().Add(2 * time.Hour)

	err := service.GrantTemporaryPermission(context.Background(), authorizationfacade.TemporaryPermissionGrantCommand{
		UserID: 1001, PermissionCode: "system:user:list", ExpireAt: &expireAt,
		StepUpProof: validStepUpProof(stepUpActionRBACGrantTempPermission, authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACGrantTempPermission, 1001, "system:user:list", &expireAt, "")),
	})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("missing grant reason must fail before persistence, got %v", err)
	}

	reason := "approved incident window"
	err = service.GrantTemporaryPermission(context.Background(), authorizationfacade.TemporaryPermissionGrantCommand{
		UserID: 1001, PermissionCode: "system:user:list", ExpireAt: &expireAt,
		Source: "SECURITY_INCIDENT", Reason: reason,
		StepUpProof: validStepUpProof(stepUpActionRBACGrantTempPermission, authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACGrantTempPermission, 1001, "system:user:list", &expireAt, reason)),
	})
	if err != nil {
		t.Fatalf("grant temporary permission: %v", err)
	}
	if !repo.tempGranted || repo.tempReason != reason || repo.tempSource != "SECURITY_INCIDENT" {
		t.Fatalf("temporary grant metadata was not persisted: %#v", repo)
	}

	extendReason := "incident still active"
	err = service.ExtendTemporaryPermission(context.Background(), authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID: 1001, PermissionCode: "system:user:list", ExpireAt: &expireAt, Reason: extendReason,
		StepUpProof: validStepUpProof(stepUpActionRBACExtendTempPermission, authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACExtendTempPermission, 1001, "system:user:list", &expireAt, extendReason)),
	})
	if err != nil || repo.tempReason != extendReason {
		t.Fatalf("extend temporary permission must persist the new reason, err=%v reason=%q", err, repo.tempReason)
	}

	revokeReason := "incident resolved"
	err = service.RevokeTemporaryPermission(context.Background(), authorizationfacade.TemporaryPermissionUpdateCommand{
		UserID: 1001, PermissionCode: "system:user:list", Reason: revokeReason,
		StepUpProof: validStepUpProof(stepUpActionRBACRevokeTempPermission, authorizationfacade.TemporaryPermissionOperationBinding(stepUpActionRBACRevokeTempPermission, 1001, "system:user:list", nil, revokeReason)),
	})
	if err != nil || !repo.tempRevoked {
		t.Fatalf("revoke temporary permission: err=%v revoked=%v", err, repo.tempRevoked)
	}
}

func TestTemporaryPermissionListExposesStableStatuses(t *testing.T) {
	now := time.Now().UTC()
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	repo := newFeatureFilterRepo()
	repo.tempItems = []domain.TemporaryPermissionRecord{
		{UserID: 1001, PermissionCode: "active", Type: 1, ExpireAt: &future},
		{UserID: 1001, PermissionCode: "expired", Type: 1, ExpireAt: &past},
		{UserID: 1001, PermissionCode: "permanent", Type: 0},
	}
	service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil)

	items, err := service.ListUserTemporaryPermissions(context.Background(), 1001)
	if err != nil {
		t.Fatalf("list temporary permissions: %v", err)
	}
	if got := []string{items[0].Status, items[1].Status, items[2].Status}; !reflect.DeepEqual(got, []string{"ACTIVE", "EXPIRED", "PERMANENT"}) {
		t.Fatalf("unexpected temporary permission statuses: %#v", got)
	}
}

func TestAssignmentsRejectDisabledFeatureResources(t *testing.T) {
	t.Run("menu", func(t *testing.T) {
		repo := newFeatureFilterRepo()
		service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
			features.PlatformControl: {},
		})

		err := service.AssignRoleMenus(context.Background(), authorizationfacade.AssignRoleMenusCommand{
			RoleID:      10,
			MenuIDs:     []int64{3},
			StepUpProof: validStepUpProof(stepUpActionRBACAssignRoleMenus, "role:10|menus:3"),
		})
		if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
			t.Fatalf("expected disabled menu assignment to be forbidden, got %v", err)
		}
		if repo.replaceCalled {
			t.Fatalf("disabled menu assignment must not replace relationships")
		}
	})

	t.Run("permission", func(t *testing.T) {
		repo := newFeatureFilterRepo()
		service := NewService(nilConfig(), nil, nil, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
			features.PlatformControl: {},
		})

		err := service.AssignRolePermissions(context.Background(), authorizationfacade.AssignRolePermissionsCommand{
			RoleID:        10,
			PermissionIDs: []int64{103},
			StepUpProof:   validStepUpProof(stepUpActionRBACAssignRolePermissions, "role:10|permissions:103|menus:"),
		})
		if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
			t.Fatalf("expected disabled permission assignment to be forbidden, got %v", err)
		}
		if repo.replaceCalled {
			t.Fatalf("disabled permission assignment must not replace relationships")
		}
	})
}

func TestRolePermissionAssignmentPreservesDisabledFeatureRelationships(t *testing.T) {
	repo := newFeatureFilterRepo()
	repo.roleMenuIDs = []int64{3}
	repo.directPermissionIDs = []int64{103}
	service := NewService(nilConfig(), nil, &serialTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil, features.Set{
		features.PlatformControl: {},
	})

	err := service.AssignRolePermissions(context.Background(), authorizationfacade.AssignRolePermissionsCommand{
		RoleID:        10,
		MenuIDs:       []int64{2},
		PermissionIDs: []int64{102},
		StepUpProof:   validStepUpProof(stepUpActionRBACAssignRolePermissions, "role:10|permissions:102|menus:2"),
	})
	if err != nil {
		t.Fatalf("assign role permissions: %v", err)
	}
	if !repo.replaceCalled {
		t.Fatalf("expected relationships to be replaced")
	}
	if !reflect.DeepEqual(repo.replacedMenuIDs, []int64{2, 3}) {
		t.Fatalf("disabled existing menu relationship was not preserved: %#v", repo.replacedMenuIDs)
	}
	if !reflect.DeepEqual(repo.replacedDirectPermissionIDs, []int64{102, 103}) {
		t.Fatalf("disabled existing direct permission was not preserved: %#v", repo.replacedDirectPermissionIDs)
	}
	if !reflect.DeepEqual(repo.replacedMenuPermissionIDs, []int64{102, 103}) {
		t.Fatalf("disabled menu-derived permission was not preserved: %#v", repo.replacedMenuPermissionIDs)
	}
}

type featureFilterRepo struct {
	Repository
	menus                       []domain.MenuRecord
	permissions                 []domain.PermissionRecord
	roleMenuIDs                 []int64
	directPermissionIDs         []int64
	replaceCalled               bool
	replacedMenuIDs             []int64
	replacedDirectPermissionIDs []int64
	replacedMenuPermissionIDs   []int64
	tempGranted                 bool
	tempRevoked                 bool
	tempSource                  string
	tempReason                  string
	tempItems                   []domain.TemporaryPermissionRecord
	listPermissionCalls         int
	pagePermissionCalls         int
	pageFilterFeatures          bool
	pageFeatureCodes            []string
}

func (r *featureFilterRepo) CountAuthorizationRootRolesByIDs(context.Context, []int64) (int, error) {
	return 0, nil
}

func newFeatureFilterRepo() *featureFilterRepo {
	return &featureFilterRepo{
		menus: []domain.MenuRecord{
			{MenuID: 1, Name: "System"},
			{MenuID: 2, ParentID: 1, Name: "Platform", FeatureCode: string(features.PlatformControl)},
			{MenuID: 3, ParentID: 1, Name: "Docker", FeatureCode: string(features.DockerAdmin)},
		},
		permissions: []domain.PermissionRecord{
			{PermissionID: 101, Code: "system:user:list", Name: "Users"},
			{PermissionID: 102, Code: "system:platform:list", Name: "Platforms", FeatureCode: string(features.PlatformControl)},
			{PermissionID: 103, Code: "admin:docker:container:list", Name: "Docker", FeatureCode: string(features.DockerAdmin)},
		},
		roleMenuIDs: []int64{1, 2, 3},
	}
}

func (r *featureFilterRepo) ListUserPermissions(context.Context, int64) ([]domain.PermissionRecord, error) {
	return append([]domain.PermissionRecord{}, r.permissions...), nil
}

func (*featureFilterRepo) FindUserAggregate(_ context.Context, userID int64) (*domain.UserAggregate, error) {
	return &domain.UserAggregate{UserID: userID, Username: "feature-filter", Enabled: true}, nil
}

func (r *featureFilterRepo) ListUserMenus(context.Context, int64) ([]domain.MenuRecord, error) {
	return append([]domain.MenuRecord{}, r.menus...), nil
}

func (r *featureFilterRepo) ListAllMenus(context.Context) ([]domain.MenuRecord, error) {
	return append([]domain.MenuRecord{}, r.menus...), nil
}

func (r *featureFilterRepo) ListMenus(context.Context, bool) ([]domain.MenuRecord, error) {
	return append([]domain.MenuRecord{}, r.menus...), nil
}

func (r *featureFilterRepo) ListRoleMenuIDs(context.Context, int64) ([]int64, error) {
	return append([]int64{}, r.roleMenuIDs...), nil
}

func (r *featureFilterRepo) ListPermissions(context.Context, authorizationfacade.PermissionQuery) ([]domain.PermissionRecord, error) {
	r.listPermissionCalls++
	return append([]domain.PermissionRecord{}, r.permissions...), nil
}

func (r *featureFilterRepo) PagePermissions(_ context.Context, query authorizationfacade.PermissionPageQuery, filterFeatures bool, enabledFeatureCodes []string) ([]domain.PermissionRecord, int64, error) {
	r.pagePermissionCalls++
	r.pageFilterFeatures = filterFeatures
	r.pageFeatureCodes = append([]string(nil), enabledFeatureCodes...)
	enabled := make(map[string]struct{}, len(enabledFeatureCodes))
	for _, code := range enabledFeatureCodes {
		enabled[code] = struct{}{}
	}
	filtered := make([]domain.PermissionRecord, 0, len(r.permissions))
	for _, record := range r.permissions {
		if filterFeatures && record.FeatureCode != "" {
			if _, ok := enabled[record.FeatureCode]; !ok {
				continue
			}
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Code != filtered[j].Code {
			return filtered[i].Code < filtered[j].Code
		}
		return filtered[i].PermissionID < filtered[j].PermissionID
	})
	current, size := normalizePage(query.Current, query.Size)
	start := int((current - 1) * size)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(size)
	if end > len(filtered) {
		end = len(filtered)
	}
	return append([]domain.PermissionRecord(nil), filtered[start:end]...), int64(len(filtered)), nil
}

func (r *featureFilterRepo) ListPermissionsByIDs(_ context.Context, ids []int64) ([]domain.PermissionRecord, error) {
	wanted := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	result := make([]domain.PermissionRecord, 0, len(ids))
	for _, record := range r.permissions {
		if _, ok := wanted[record.PermissionID]; ok {
			result = append(result, record)
		}
	}
	return result, nil
}

func (r *featureFilterRepo) LockPermissionGrants(_ context.Context, ids []int64) ([]domain.PermissionRecord, error) {
	return r.ListPermissionsByIDs(context.Background(), ids)
}

func (r *featureFilterRepo) FindPermissionIDByCode(_ context.Context, code string) (int64, error) {
	for _, permission := range r.permissions {
		if permission.Code == code {
			return permission.PermissionID, nil
		}
	}
	return 0, nil
}

func (r *featureFilterRepo) TouchPermissionGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *featureFilterRepo) ListMenuPermissionIDs(_ context.Context, menuIDs []int64) ([]int64, error) {
	result := make([]int64, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		switch menuID {
		case 1:
			result = append(result, 101, 102, 103)
		case 2:
			result = append(result, 102)
		case 3:
			result = append(result, 103)
		}
	}
	return uniqueInt64(result), nil
}

func (r *featureFilterRepo) FindRoleByID(context.Context, int64) (*domain.RoleRecord, error) {
	return &domain.RoleRecord{RoleID: 10}, nil
}

func (r *featureFilterRepo) LockRoleGrant(context.Context, int64) (*domain.RoleRecord, error) {
	return &domain.RoleRecord{RoleID: 10}, nil
}

func (r *featureFilterRepo) LockMenuGrants(_ context.Context, menuIDs []int64) ([]domain.MenuRecord, error) {
	result := make([]domain.MenuRecord, 0, len(menuIDs))
	for _, menuID := range menuIDs {
		result = append(result, domain.MenuRecord{MenuID: menuID})
	}
	return result, nil
}

func (r *featureFilterRepo) TouchMenuGrantGuards(context.Context, []int64) error {
	return nil
}

func (r *featureFilterRepo) ListUserIDsByRoleIDsPage(context.Context, []int64, int64, int) ([]int64, error) {
	return []int64{}, nil
}

func (r *featureFilterRepo) UpdateRoleGrantRevision(context.Context, int64, int64, int64, int64) error {
	return nil
}

func (r *featureFilterRepo) CountMenusByIDs(_ context.Context, ids []int64) (int, error) {
	return len(ids), nil
}

func (r *featureFilterRepo) CountPermissionsByIDs(_ context.Context, ids []int64) (int, error) {
	return len(ids), nil
}

func (r *featureFilterRepo) ListMenuPermissionCodes(context.Context, []int64) ([]string, error) {
	return []string{}, nil
}

func (r *featureFilterRepo) ListDirectRolePermissionIDsByRoleIDs(context.Context, []int64) (map[int64][]int64, error) {
	return map[int64][]int64{10: append([]int64{}, r.directPermissionIDs...)}, nil
}

func (r *featureFilterRepo) ListUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return []int64{}, nil
}

func (r *featureFilterRepo) ReplaceRolePermissions(_ context.Context, _ int64, directPermissionIDs, menuPermissionIDs, menuIDs []int64, _ int64, _ func() int64) error {
	r.replaceCalled = true
	r.replacedDirectPermissionIDs = append([]int64{}, directPermissionIDs...)
	r.replacedMenuPermissionIDs = append([]int64{}, menuPermissionIDs...)
	r.replacedMenuIDs = append([]int64{}, menuIDs...)
	return nil
}

func (r *featureFilterRepo) GrantTemporaryPermission(_ context.Context, _ int64, _ string, _ *time.Time, source, reason string, _ int64, _ func() int64) error {
	r.tempGranted = true
	r.tempSource = source
	r.tempReason = reason
	return nil
}

func (r *featureFilterRepo) ExtendTemporaryPermission(_ context.Context, _ int64, _ string, _ *time.Time, reason string) error {
	r.tempReason = reason
	return nil
}

func (r *featureFilterRepo) RevokeTemporaryPermission(context.Context, int64, string) error {
	r.tempRevoked = true
	return nil
}

func (r *featureFilterRepo) ListUserTemporaryPermissions(context.Context, int64) ([]domain.TemporaryPermissionRecord, error) {
	return append([]domain.TemporaryPermissionRecord(nil), r.tempItems...), nil
}

func flattenMenuIDs(nodes []authorizationfacade.MenuTreeNodeVO) []int64 {
	result := make([]int64, 0)
	var visit func([]authorizationfacade.MenuTreeNodeVO)
	visit = func(items []authorizationfacade.MenuTreeNodeVO) {
		for _, item := range items {
			result = append(result, item.MenuID)
			visit(item.Children)
		}
	}
	visit(nodes)
	return result
}

func permissionVOCodes(items []authorizationfacade.PermissionVO) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.Code)
	}
	return result
}
