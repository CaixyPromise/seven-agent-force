import { request } from '@/api/request';

export type ApiIdentifier = number | string;
export type AccessDecision = 'ALLOW' | 'DENY';
export type PermissionGrantSource = 'ROLE_DIRECT' | 'MENU_DERIVED' | 'TEMPORARY';
export type RoleAssignmentSource = 'DIRECT_USER' | 'POST';

interface ApiPayload<T> {
  code: number;
  data: T;
  message?: string;
}

export interface AccessPostSource {
  postId: ApiIdentifier;
  postCode: string;
  postName: string;
  deptId: ApiIdentifier;
  orgId: ApiIdentifier;
}

export interface AccessRoleSource {
  roleId: ApiIdentifier;
  roleCode: string;
  roleName: string;
  roleStatus: number;
  declaredDataScope: string;
  roleAssignmentSource: RoleAssignmentSource;
  post?: AccessPostSource;
}

export interface DataScopeContributor {
  roleId: ApiIdentifier;
  roleCode: string;
  declaredScopeType: string;
  winning: boolean;
  deptIds: ApiIdentifier[];
}

export interface EffectiveDataScope {
  userId: ApiIdentifier;
  scopeType: string;
  deptIds: ApiIdentifier[];
  orgIds: ApiIdentifier[];
  contributors: DataScopeContributor[];
}

export interface PermissionGrantChain {
  permissionGrantSource: PermissionGrantSource;
  roleId?: ApiIdentifier;
  roleCode?: string;
  roleName?: string;
  roleAssignmentSource?: RoleAssignmentSource;
  post?: AccessPostSource;
  menuId?: ApiIdentifier;
  menuName?: string;
  menuPath?: string;
  grantedBy?: ApiIdentifier;
  source?: string;
  expireAt?: string;
  active: boolean;
  reasonCode: string;
}

export interface EffectivePermission {
  permissionId: ApiIdentifier;
  permissionCode: string;
  permissionName: string;
  effective: boolean;
  featureCode?: string;
  featureEnabled: boolean;
  grants: PermissionGrantChain[];
}

export interface EffectiveAccess {
  userId: ApiIdentifier;
  username: string;
  status: number;
  authorizationRoot: boolean;
  roleSources: AccessRoleSource[];
  dataScope: EffectiveDataScope;
  permissionSummary: {
    effectiveCount: number;
    filteredCount: number;
    temporaryCount: number;
  };
  permissions: {
    current: number;
    size: number;
    total: number;
    records: EffectivePermission[];
  };
}

export interface PermissionExplanation {
  userId: ApiIdentifier;
  permissionCode: string;
  decision: AccessDecision;
  reasonCode: string;
  matchedPermissionCodes: string[];
  chains: PermissionGrantChain[];
  feature?: { code: string; enabled: boolean };
  evaluatedAt: string;
}

export interface EffectiveAccessParams {
  current?: number;
  size?: number;
  keyword?: string;
  sourceType?: string;
  effective?: boolean;
}

export async function fetchEffectiveAccess(userId: ApiIdentifier, params: EffectiveAccessParams) {
  const response = await request<ApiPayload<EffectiveAccess>>(
    `/api/system/user/${encodeURIComponent(String(userId))}/effective-access`,
    { method: 'GET', params },
  );
  const data = response.data;
  return {
    ...data,
    roleSources: data.roleSources ?? [],
    dataScope: {
      ...data.dataScope,
      deptIds: data.dataScope?.deptIds ?? [],
      orgIds: data.dataScope?.orgIds ?? [],
      contributors: data.dataScope?.contributors ?? [],
    },
    permissionSummary: {
      effectiveCount: Number(data.permissionSummary?.effectiveCount ?? 0),
      filteredCount: Number(data.permissionSummary?.filteredCount ?? 0),
      temporaryCount: Number(data.permissionSummary?.temporaryCount ?? 0),
    },
    permissions: {
      current: Number(data.permissions?.current ?? 1),
      size: Number(data.permissions?.size ?? params.size ?? 20),
      total: Number(data.permissions?.total ?? 0),
      records: data.permissions?.records ?? [],
    },
  } satisfies EffectiveAccess;
}

export async function fetchPermissionExplanation(userId: ApiIdentifier, permissionCode: string) {
  const response = await request<ApiPayload<PermissionExplanation>>(
    `/api/system/user/${encodeURIComponent(String(userId))}/access-explain`,
    { method: 'GET', params: { permissionCode } },
  );
  return {
    ...response.data,
    matchedPermissionCodes: response.data.matchedPermissionCodes ?? [],
    chains: response.data.chains ?? [],
  } satisfies PermissionExplanation;
}
