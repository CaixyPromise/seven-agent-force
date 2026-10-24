package application

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

const stepUpActionRBACCommitRoleGrants = "RBAC_COMMIT_ROLE_GRANTS"

// AdvanceRoleGrantRevision advances a legacy role-grant writer inside the caller's transaction.
func (s *Service) AdvanceRoleGrantRevision(ctx context.Context, roleID int64, operatorID int64) error {
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		role, err := s.repository.LockRoleGrant(txCtx, roleID)
		if err != nil {
			return err
		}
		if role == nil {
			return apperrors.NotFound("角色不存在")
		}
		return s.repository.UpdateRoleGrantRevision(txCtx, roleID, role.GrantRevision, role.GrantRevision+1, operatorID)
	})
}

type preparedRoleGrant struct {
	requestedMenuIDs        []int64
	requestedPermissionIDs  []int64
	requestedDeptIDs        []int64
	configScopes            []authorizationfacade.RoleConfigScopeGrantVO
	nextMenuIDs             []int64
	nextDirectPermissionIDs []int64
	derivedPermissionIDs    []int64
}

// GetRoleGrantSnapshot returns the current editable role grant state and revision.
func (s *Service) GetRoleGrantSnapshot(ctx context.Context, roleID int64) (*authorizationfacade.RoleGrantSnapshotVO, error) {
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
	return s.buildRoleGrantSnapshot(ctx, *role)
}

// PreviewRoleGrantBundle validates and previews a complete role grant replacement.
func (s *Service) PreviewRoleGrantBundle(ctx context.Context, command authorizationfacade.PreviewRoleGrantBundleCommand) (*authorizationfacade.RoleGrantPreviewVO, error) {
	if err := validateRoleGrantBundleRequest(command.RoleGrantBundleRequest, false); err != nil {
		return nil, err
	}
	role, err := s.repository.FindRoleByID(ctx, command.RoleID)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, apperrors.NotFound("角色不存在")
	}
	if role.GrantRevision != command.ExpectedRevision {
		return nil, roleGrantRevisionConflict(role.GrantRevision)
	}
	if role.IsAuthorizationRoot() {
		return nil, apperrors.Operation("授权安全根的授权关系由系统管理")
	}
	prepared, err := s.prepareRoleGrant(ctx, *role, command.RoleGrantBundleRequest, command.OperatorID)
	if err != nil {
		return nil, err
	}
	snapshot, err := s.buildRoleGrantSnapshot(ctx, *role)
	if err != nil {
		return nil, err
	}
	return buildRoleGrantPreview(snapshot, command.RoleGrantBundleRequest.DataScope, prepared), nil
}

// CommitRoleGrantBundle atomically replaces every role grant partition at one expected revision.
func (s *Service) CommitRoleGrantBundle(ctx context.Context, command authorizationfacade.CommitRoleGrantBundleCommand) (*authorizationfacade.RoleGrantCommitVO, error) {
	if err := validateRoleGrantBundleRequest(command.RoleGrantBundleRequest, true); err != nil {
		return nil, err
	}
	requestHash, err := authorizationfacade.RoleGrantRequestHash(command.RoleGrantBundleRequest)
	if err != nil {
		return nil, err
	}
	observedMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, command.RoleID)
	if err != nil {
		return nil, err
	}
	observedDerivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, uniqueInt64(append(append([]int64{}, observedMenuIDs...), []int64(command.MenuIDs)...)))
	if err != nil {
		return nil, err
	}
	observedDirectPermissions, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(ctx, []int64{command.RoleID})
	if err != nil {
		return nil, err
	}
	guardPermissionIDs := uniqueInt64(append(append(append([]int64{}, []int64(command.PermissionIDs)...), observedDerivedPermissionIDs...), observedDirectPermissions[command.RoleID]...))
	var result *authorizationfacade.RoleGrantCommitVO
	var impactedUserCount int
	err = s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if _, err := s.lockPermissionParents(txCtx, guardPermissionIDs); err != nil {
			return err
		}
		role, _, err := s.lockRoleMenuMutation(txCtx, command.RoleID, []int64(command.MenuIDs))
		if err != nil {
			return err
		}
		previous, err := s.repository.FindRoleGrantRequest(txCtx, command.RoleID, command.IdempotencyKey)
		if err != nil {
			return err
		}
		if previous != nil {
			if previous.RequestHash != requestHash {
				return apperrors.ObjectState("幂等键已用于其他角色授权请求").WithDetails(map[string]any{
					"reasonCode": "ROLE_GRANT_IDEMPOTENCY_KEY_REUSED",
				})
			}
			result = &authorizationfacade.RoleGrantCommitVO{
				RoleID: command.RoleID, Revision: previous.ResultRevision,
				Changed: previous.Changed == 1, ImpactedUserCount: previous.ImpactedUserCount,
				IdempotentReplay: true,
			}
			return nil
		}
		if role.GrantRevision != command.ExpectedRevision {
			return roleGrantRevisionConflict(role.GrantRevision)
		}
		if role.IsAuthorizationRoot() {
			return apperrors.Operation("授权安全根的授权关系由系统管理")
		}
		prepared, err := s.prepareRoleGrant(txCtx, *role, command.RoleGrantBundleRequest, command.OperatorID)
		if err != nil {
			return err
		}
		if !idsWithinGuard(prepared.nextDirectPermissionIDs, guardPermissionIDs) || !idsWithinGuard(prepared.derivedPermissionIDs, guardPermissionIDs) {
			return apperrors.ObjectState("角色权限关系已并发变化，请重试")
		}
		binding, err := authorizationfacade.RoleGrantOperationBinding(command.RoleID, command.RoleGrantBundleRequest)
		if err != nil {
			return err
		}
		if err := stepup.Require(command.StepUpProof, stepUpActionRBACCommitRoleGrants, binding); err != nil {
			return err
		}
		snapshot, err := s.buildRoleGrantSnapshot(txCtx, *role)
		if err != nil {
			return err
		}
		preview := buildRoleGrantPreview(snapshot, command.DataScope, prepared)
		impactedUserCount, err = s.repository.CountUserIDsByRoleID(txCtx, command.RoleID)
		if err != nil {
			return err
		}
		nextRevision := role.GrantRevision
		if preview.Changed {
			if err := s.repository.ReplaceRolePermissions(txCtx, command.RoleID, prepared.nextDirectPermissionIDs, prepared.derivedPermissionIDs, prepared.nextMenuIDs, command.OperatorID, s.nextID); err != nil {
				return err
			}
			if err := s.repository.ReplaceRoleDepts(txCtx, command.RoleID, prepared.requestedDeptIDs, command.OperatorID, s.nextID); err != nil {
				return err
			}
			if role.DataScope != command.DataScope {
				if err := s.repository.UpdateRoleGrantDataScope(txCtx, command.RoleID, command.DataScope, command.OperatorID); err != nil {
					return err
				}
			}
			if err := s.configScopes.ReplaceRoleConfigScopes(txCtx, command.RoleID, prepared.configScopes, command.OperatorID); err != nil {
				return err
			}
			nextRevision++
			if err := s.repository.UpdateRoleGrantRevision(txCtx, command.RoleID, role.GrantRevision, nextRevision, command.OperatorID); err != nil {
				return err
			}
		}
		record := domain.RoleGrantRequestRecord{
			RoleID: command.RoleID, IdempotencyKey: command.IdempotencyKey, RequestHash: requestHash,
			ResultRevision: nextRevision, ImpactedUserCount: impactedUserCount, Changed: boolInt(preview.Changed),
		}
		if err := s.repository.CreateRoleGrantRequest(txCtx, record, command.OperatorID); err != nil {
			return err
		}
		result = &authorizationfacade.RoleGrantCommitVO{
			RoleID: command.RoleID, Revision: nextRevision, Changed: preview.Changed,
			ImpactedUserCount: impactedUserCount, IdempotentReplay: false,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) buildRoleGrantSnapshot(ctx context.Context, role domain.RoleRecord) (*authorizationfacade.RoleGrantSnapshotVO, error) {
	menuIDs, err := s.repository.ListRoleMenuIDs(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	menuIDs, err = s.activeMenuIDs(ctx, menuIDs)
	if err != nil {
		return nil, err
	}
	directByRole, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(ctx, []int64{role.RoleID})
	if err != nil {
		return nil, err
	}
	permissionIDs, err := s.activePermissionIDs(ctx, directByRole[role.RoleID])
	if err != nil {
		return nil, err
	}
	deptIDs, err := s.repository.ListDeptIDsByRoleID(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	if s.configScopes == nil {
		return nil, apperrors.System("角色授权配置范围端口未配置")
	}
	configScopes, err := s.configScopes.ListRoleConfigScopes(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	impactedUserCount, err := s.repository.CountUserIDsByRoleID(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	return &authorizationfacade.RoleGrantSnapshotVO{
		Role: toRoleVO(role), Revision: role.GrantRevision,
		MenuIDs: sortedIDs(menuIDs), PermissionIDs: sortedIDs(permissionIDs),
		DataScope: role.DataScope, DeptIDs: sortedIDs(deptIDs), ConfigScopes: normalizeConfigScopeOrder(configScopes),
		ImpactedUserCount: impactedUserCount,
	}, nil
}

func (s *Service) prepareRoleGrant(ctx context.Context, role domain.RoleRecord, request authorizationfacade.RoleGrantBundleRequest, operatorID int64) (*preparedRoleGrant, error) {
	if role.IsSystem() && role.DataScope != request.DataScope {
		return nil, apperrors.Operation("SYSTEM角色的数据范围类型由系统管理")
	}
	menuIDs := sortedIDs([]int64(request.MenuIDs))
	permissionIDs := sortedIDs([]int64(request.PermissionIDs))
	deptIDs := sortedIDs([]int64(request.DeptIDs))
	if request.DataScope != 2 && len(deptIDs) > 0 {
		return nil, apperrors.Params("只有自定数据权限角色可以绑定部门范围")
	}
	if err := s.ensureMenusExist(ctx, menuIDs); err != nil {
		return nil, err
	}
	if err := s.ensurePermissionsExist(ctx, permissionIDs); err != nil {
		return nil, err
	}
	if err := s.ensureDeptsExist(ctx, deptIDs); err != nil {
		return nil, err
	}
	if err := s.ensureMenuFeaturesEnabled(ctx, menuIDs); err != nil {
		return nil, err
	}
	if err := s.ensurePermissionFeaturesEnabled(ctx, permissionIDs); err != nil {
		return nil, err
	}
	menuPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, menuIDs)
	if err != nil {
		return nil, err
	}
	menuPermissionCodes, err := s.repository.ListMenuPermissionCodes(ctx, menuIDs)
	if err != nil {
		return nil, err
	}
	if err := s.ensureOperatorCanGrantPermissionIDs(ctx, operatorID, uniqueInt64(append(append([]int64{}, permissionIDs...), menuPermissionIDs...))); err != nil {
		return nil, err
	}
	if err := s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, menuPermissionCodes); err != nil {
		return nil, err
	}
	existingMenuIDs, err := s.repository.ListRoleMenuIDs(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	preservedMenus, err := s.unavailableMenuIDs(ctx, existingMenuIDs)
	if err != nil {
		return nil, err
	}
	directByRole, err := s.repository.ListDirectRolePermissionIDsByRoleIDs(ctx, []int64{role.RoleID})
	if err != nil {
		return nil, err
	}
	preservedPermissions, err := s.unavailablePermissionIDs(ctx, directByRole[role.RoleID])
	if err != nil {
		return nil, err
	}
	nextMenuIDs := sortedIDs(append(append([]int64{}, menuIDs...), preservedMenus...))
	nextDirectPermissionIDs := sortedIDs(append(append([]int64{}, permissionIDs...), preservedPermissions...))
	derivedPermissionIDs, err := s.repository.ListMenuPermissionIDs(ctx, nextMenuIDs)
	if err != nil {
		return nil, err
	}
	if s.configScopes == nil {
		return nil, apperrors.System("角色授权配置范围端口未配置")
	}
	configScopes, err := s.configScopes.NormalizeRoleConfigScopes(ctx, request.ConfigScopes)
	if err != nil {
		return nil, err
	}
	existingConfigScopes, err := s.configScopes.ListRoleConfigScopes(ctx, role.RoleID)
	if err != nil {
		return nil, err
	}
	addedConfigScopes, removedConfigScopes := configScopeDiff(existingConfigScopes, configScopes)
	if len(addedConfigScopes)+len(removedConfigScopes) > 0 {
		if err := s.ensureOperatorCanGrantPermissionCodes(ctx, operatorID, []string{"system:config:scope:assign"}); err != nil {
			return nil, err
		}
	}
	return &preparedRoleGrant{
		requestedMenuIDs: menuIDs, requestedPermissionIDs: permissionIDs, requestedDeptIDs: deptIDs,
		configScopes: normalizeConfigScopeOrder(configScopes), nextMenuIDs: nextMenuIDs,
		nextDirectPermissionIDs: nextDirectPermissionIDs, derivedPermissionIDs: sortedIDs(derivedPermissionIDs),
	}, nil
}

func (s *Service) activeMenuIDs(ctx context.Context, ids []int64) ([]int64, error) {
	all, err := s.repository.ListAllMenus(ctx)
	if err != nil {
		return nil, err
	}
	requested := idSet(ids)
	result := make([]int64, 0, len(ids))
	for _, item := range all {
		if _, ok := requested[item.MenuID]; ok && item.Status == 0 && s.featureEnabled(item.FeatureCode) {
			result = append(result, item.MenuID)
		}
	}
	return sortedIDs(result), nil
}

func (s *Service) unavailableMenuIDs(ctx context.Context, ids []int64) ([]int64, error) {
	active, err := s.activeMenuIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return subtractIDs(sortedIDs(ids), active), nil
}

func (s *Service) activePermissionIDs(ctx context.Context, ids []int64) ([]int64, error) {
	records, err := s.repository.ListPermissionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	result := make([]int64, 0, len(records))
	for _, item := range records {
		if item.Status == 0 && s.featureEnabled(item.FeatureCode) {
			result = append(result, item.PermissionID)
		}
	}
	return sortedIDs(result), nil
}

func (s *Service) unavailablePermissionIDs(ctx context.Context, ids []int64) ([]int64, error) {
	active, err := s.activePermissionIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return subtractIDs(sortedIDs(ids), active), nil
}

func buildRoleGrantPreview(snapshot *authorizationfacade.RoleGrantSnapshotVO, dataScope int, prepared *preparedRoleGrant) *authorizationfacade.RoleGrantPreviewVO {
	changes := authorizationfacade.RoleGrantChangeSetVO{
		AddedMenuIDs: subtractIDs(prepared.requestedMenuIDs, snapshot.MenuIDs), RemovedMenuIDs: subtractIDs(snapshot.MenuIDs, prepared.requestedMenuIDs),
		AddedPermissionIDs: subtractIDs(prepared.requestedPermissionIDs, snapshot.PermissionIDs), RemovedPermissionIDs: subtractIDs(snapshot.PermissionIDs, prepared.requestedPermissionIDs),
		AddedDeptIDs: subtractIDs(prepared.requestedDeptIDs, snapshot.DeptIDs), RemovedDeptIDs: subtractIDs(snapshot.DeptIDs, prepared.requestedDeptIDs),
		DataScopeFrom: snapshot.DataScope, DataScopeTo: dataScope,
	}
	changes.AddedConfigScopes, changes.RemovedConfigScopes = configScopeDiff(snapshot.ConfigScopes, prepared.configScopes)
	changed := len(changes.AddedMenuIDs)+len(changes.RemovedMenuIDs)+len(changes.AddedPermissionIDs)+len(changes.RemovedPermissionIDs)+len(changes.AddedDeptIDs)+len(changes.RemovedDeptIDs)+len(changes.AddedConfigScopes)+len(changes.RemovedConfigScopes) > 0 || changes.DataScopeFrom != changes.DataScopeTo
	return &authorizationfacade.RoleGrantPreviewVO{RoleID: snapshot.Role.RoleID, Revision: snapshot.Revision, Changed: changed, ImpactedUserCount: snapshot.ImpactedUserCount, Changes: changes}
}

func validateRoleGrantBundleRequest(request authorizationfacade.RoleGrantBundleRequest, commit bool) error {
	if request.ExpectedRevision < 0 {
		return apperrors.Params("expectedRevision不能小于0")
	}
	if request.DataScope < 1 || request.DataScope > 5 {
		return apperrors.Params("dataScope必须为1到5")
	}
	if commit {
		if strings.TrimSpace(request.IdempotencyKey) == "" || len(request.IdempotencyKey) > 128 {
			return apperrors.Params("idempotencyKey不能为空且不能超过128个字符")
		}
		if strings.TrimSpace(request.Reason) == "" || len([]rune(request.Reason)) > 500 {
			return apperrors.Params("授权变更原因不能为空且不能超过500个字符")
		}
	}
	return nil
}

func roleGrantRevisionConflict(currentRevision int64) error {
	return apperrors.ObjectState("角色授权已被其他管理员更新，请重新加载后预览").WithDetails(map[string]any{
		"reasonCode": "ROLE_GRANT_REVISION_CONFLICT", "currentRevision": currentRevision,
	})
}

func normalizeConfigScopeOrder(values []authorizationfacade.RoleConfigScopeGrantVO) []authorizationfacade.RoleConfigScopeGrantVO {
	result := append([]authorizationfacade.RoleConfigScopeGrantVO(nil), values...)
	for index := range result {
		result[index].GroupCode = strings.TrimSpace(result[index].GroupCode)
		result[index].ConfigKey = strings.TrimSpace(result[index].ConfigKey)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].GroupCode+"\x00"+result[i].ConfigKey, result[j].GroupCode+"\x00"+result[j].ConfigKey
		if left != right {
			return left < right
		}
		if result[i].CanRead != result[j].CanRead {
			return result[i].CanRead < result[j].CanRead
		}
		if result[i].CanWrite != result[j].CanWrite {
			return result[i].CanWrite < result[j].CanWrite
		}
		return result[i].CanDelete < result[j].CanDelete
	})
	if result == nil {
		return []authorizationfacade.RoleConfigScopeGrantVO{}
	}
	return result
}

func configScopeDiff(before, after []authorizationfacade.RoleConfigScopeGrantVO) ([]authorizationfacade.RoleConfigScopeGrantVO, []authorizationfacade.RoleConfigScopeGrantVO) {
	key := func(item authorizationfacade.RoleConfigScopeGrantVO) string {
		payload, _ := json.Marshal(item)
		return string(payload)
	}
	beforeSet, afterSet := map[string]authorizationfacade.RoleConfigScopeGrantVO{}, map[string]authorizationfacade.RoleConfigScopeGrantVO{}
	for _, item := range normalizeConfigScopeOrder(before) {
		beforeSet[key(item)] = item
	}
	for _, item := range normalizeConfigScopeOrder(after) {
		afterSet[key(item)] = item
	}
	added, removed := []authorizationfacade.RoleConfigScopeGrantVO{}, []authorizationfacade.RoleConfigScopeGrantVO{}
	for itemKey, item := range afterSet {
		if _, ok := beforeSet[itemKey]; !ok {
			added = append(added, item)
		}
	}
	for itemKey, item := range beforeSet {
		if _, ok := afterSet[itemKey]; !ok {
			removed = append(removed, item)
		}
	}
	return normalizeConfigScopeOrder(added), normalizeConfigScopeOrder(removed)
}

func sortedIDs(values []int64) []int64 {
	result := uniqueInt64(values)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	if result == nil {
		return []int64{}
	}
	return result
}

func subtractIDs(left, right []int64) []int64 {
	rightSet := idSet(right)
	result := make([]int64, 0)
	for _, id := range sortedIDs(left) {
		if _, ok := rightSet[id]; !ok {
			result = append(result, id)
		}
	}
	return result
}

func idSet(values []int64) map[int64]struct{} {
	result := make(map[int64]struct{}, len(values))
	for _, id := range values {
		if id > 0 {
			result[id] = struct{}{}
		}
	}
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
