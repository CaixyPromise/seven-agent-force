import { request } from './request';

export type TemporaryPermission = {
  userId: API.Int64;
  permissionCode: string;
  permissionName?: string;
  type: number;
  expireAt?: string;
  source?: string;
  reason?: string;
  grantedBy?: API.Int64;
  grantedAt?: string;
  updatedAt?: string;
  status: 'ACTIVE' | 'EXPIRED' | 'PERMANENT';
};

type ResultEnvelope<T> = {
  code?: number;
  message?: string;
  data?: T;
};

export type TemporaryPermissionGrantRequest = {
  userId: API.Int64;
  permissionCode: string;
  expireAt?: string;
  source?: string;
  reason: string;
};

export type TemporaryPermissionUpdateRequest = {
  userId: API.Int64;
  permissionCode: string;
  expireAt?: string;
  reason: string;
};

export function getUserTemporaryPermissions(userId: API.Int64) {
  return request<ResultEnvelope<TemporaryPermission[]>>(`/api/admin/temp-permission/user/${userId}`, {
    method: 'GET',
  });
}

export function grantTemporaryPermission(data: TemporaryPermissionGrantRequest) {
  return request<ResultEnvelope<boolean>>('/api/admin/temp-permission/grant', {
    method: 'POST',
    data,
  });
}

export function extendTemporaryPermission(data: TemporaryPermissionUpdateRequest) {
  return request<ResultEnvelope<boolean>>('/api/admin/temp-permission/extend', {
    method: 'POST',
    data,
  });
}

export function revokeTemporaryPermission(data: TemporaryPermissionUpdateRequest) {
  return request<ResultEnvelope<boolean>>('/api/admin/temp-permission/revoke', {
    method: 'POST',
    data,
  });
}

export function cleanupExpiredTemporaryPermissions() {
  return request<ResultEnvelope<boolean>>('/api/admin/temp-permission/cleanup', {
    method: 'POST',
  });
}

export function getTemporaryPermissionStatistics() {
  return request<ResultEnvelope<{ totalActive: number; temporary: number; permanent: number; expiringSoon: number }>>(
    '/api/admin/temp-permission/stats',
    { method: 'GET' },
  );
}
