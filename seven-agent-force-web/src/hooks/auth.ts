/**
 * 认证和权限 hooks 的适配层
 * 兼容旧版 Permission 组件的导入
 */

import { hasPermissions, type PermissionMatchMode } from '@/lib/auth/permissions';
import { useAuthorization as useAuthorizationOriginal } from './useAuth';

// 导出原始的 useAuthorization
export { useAuthorization } from './useAuth';

// 导出别名 useAuth
export const useAuth = () => {
  const { user, isLoggedIn, hasPermission, hasRole } = useAuthorizationOriginal();
  const isAdmin = Boolean(user?.isAdmin);

  return {
    user,
    isAuthenticated: isLoggedIn,
    isAdmin,
    permissions: user?.permissions || [],
    roles: user?.roleCodes || [],
    hasPermission,
    hasRole,
  };
};

export type PermissionRequirement =
  | string
  | string[]
  | {
      permissions: string[];
      matchMode?: PermissionMatchMode;
    };

function normalizePermissionRequirement(requirement?: PermissionRequirement) {
  if (!requirement) {
    return {
      permissions: [] as string[],
      matchMode: 'any' as PermissionMatchMode,
    };
  }

  if (typeof requirement === 'string') {
    return {
      permissions: [requirement],
      matchMode: 'any' as PermissionMatchMode,
    };
  }

  if (Array.isArray(requirement)) {
    return {
      permissions: requirement,
      matchMode: 'any' as PermissionMatchMode,
    };
  }

  return {
    permissions: requirement.permissions,
    matchMode: requirement.matchMode ?? 'any',
  };
}

export function usePermissionAccess(requirement?: PermissionRequirement): boolean {
  const { user } = useAuthorizationOriginal();
  const { permissions, matchMode } = normalizePermissionRequirement(requirement);
  return hasPermissions(user?.permissions, permissions, matchMode);
}

export function usePermissionFlags<T extends Record<string, PermissionRequirement>>(
  requirements: T,
): { [K in keyof T]: boolean } {
  const { user } = useAuthorizationOriginal();
  const grantedPermissions = user?.permissions;
  const flags = {} as { [K in keyof T]: boolean };

  (Object.keys(requirements) as Array<keyof T>).forEach((key) => {
    const { permissions, matchMode } = normalizePermissionRequirement(requirements[key]);
    flags[key] = hasPermissions(grantedPermissions, permissions, matchMode);
  });

  return flags;
}

// 权限检查 hook
export function usePermission(permission?: string, module?: string): boolean {
  void module;
  return usePermissionAccess(permission);
}

// 角色检查 hook
export function useRole(roles?: string | string[], requireAll = false): boolean {
  const { hasRole } = useAuthorizationOriginal();
  if (!roles || (Array.isArray(roles) && roles.length === 0)) return true;

  const roleList = Array.isArray(roles) ? roles : [roles];
  if (requireAll) {
    return roleList.every((role) => hasRole(role));
  }
  return roleList.some((role) => hasRole(role));
}

// 数据权限 hook
export function useDataScope() {
  const { user } = useAuthorizationOriginal();
  const dataScope = user?.dataScope ?? {
		userId: user?.id,
    deptIds: [],
    orgIds: [],
    scopeType: 'NONE' as const,
  };
  const scopeType = dataScope.scopeType;
  const visibleDeptIds = dataScope.deptIds;
  const visibleOrgIds = dataScope.orgIds;
  const hasGlobalDataAccess = scopeType === 'ALL';
  const isSelfOnly = scopeType === 'SELF';

	const canAccessDeptData = (deptId: API.Int64): boolean => {
    if (!user) return false;
    if (hasGlobalDataAccess) return true;
    if (!['CUSTOM', 'DEPT', 'DEPT_AND_CHILD'].includes(scopeType)) return false;
    const target = String(deptId);
    return visibleDeptIds.some((item) => String(item) === target);
  };

  const descriptions = {
    ALL: '全部数据权限',
    CUSTOM: '指定部门数据权限',
    DEPT: '本部门数据权限',
    DEPT_AND_CHILD: '本部门及下级部门数据权限',
    SELF: '仅本人数据权限',
    NONE: '无业务数据权限',
  } as const;
  const description = descriptions[scopeType];

  return {
    dataScope,
    scopeType,
    hasGlobalDataAccess,
    isSelfOnly,
    isPersonalOnly: isSelfOnly,
    visibleDeptIds,
    visibleOrgIds,
    canAccessDeptData,
    description,
    getDataScopeDesc: () => description,
  };
}
