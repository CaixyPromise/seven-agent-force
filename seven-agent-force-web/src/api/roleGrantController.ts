import { request } from './request';

export type RoleConfigScopeGrant = {
  groupCode: string;
  configKey?: string;
  canRead: number;
  canWrite: number;
  canDelete: number;
};

export type RoleGrantSnapshot = {
  role: API.RoleVO;
  revision: API.Int64;
  menuIds: API.Int64[];
  permissionIds: API.Int64[];
  dataScope: number;
  deptIds: API.Int64[];
  configScopes: RoleConfigScopeGrant[];
  impactedUserCount: number;
};

export type RoleGrantBundle = {
  expectedRevision: API.Int64;
  menuIds: API.Int64[];
  permissionIds: API.Int64[];
  dataScope: number;
  deptIds: API.Int64[];
  configScopes: RoleConfigScopeGrant[];
  reason: string;
  idempotencyKey: string;
};

export type RoleGrantChanges = {
  addedMenuIds: API.Int64[];
  removedMenuIds: API.Int64[];
  addedPermissionIds: API.Int64[];
  removedPermissionIds: API.Int64[];
  addedDeptIds: API.Int64[];
  removedDeptIds: API.Int64[];
  addedConfigScopes: RoleConfigScopeGrant[];
  removedConfigScopes: RoleConfigScopeGrant[];
  dataScopeFrom: number;
  dataScopeTo: number;
};

export type RoleGrantPreview = {
  roleId: API.Int64;
  revision: API.Int64;
  changed: boolean;
  impactedUserCount: number;
  changes: RoleGrantChanges;
};

export type RoleGrantCommit = {
  roleId: API.Int64;
  revision: API.Int64;
  changed: boolean;
  impactedUserCount: number;
  idempotentReplay: boolean;
};

type ResultEnvelope<T> = {
  code?: number;
  message?: string;
  data?: T;
};

export function getRoleGrantSnapshot(roleId: API.Int64) {
  return request<ResultEnvelope<RoleGrantSnapshot>>(`/api/system/role/${roleId}/grant-snapshot`, {
    method: 'GET',
  });
}

export function previewRoleGrantBundle(roleId: API.Int64, data: RoleGrantBundle) {
  return request<ResultEnvelope<RoleGrantPreview>>(`/api/system/role/${roleId}/grant-preview`, {
    method: 'POST',
    data,
  });
}

export function commitRoleGrantBundle(roleId: API.Int64, data: RoleGrantBundle) {
  return request<ResultEnvelope<RoleGrantCommit>>(`/api/system/role/${roleId}/grants`, {
    method: 'PUT',
    data,
  });
}
