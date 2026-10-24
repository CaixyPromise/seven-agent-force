import { request } from '@/api/request';
import type { BatchLogoutResult, OnlineUserRecord, PageResult } from '@/types/admin';

interface ApiPayload<T> {
  code: number;
  data: T;
  message?: string;
}

export interface OnlineUserParams {
  page?: number;
  size?: number;
  [key: string]: string | number | boolean | readonly string[] | readonly number[] | undefined;
}

export async function fetchOnlineUsers(params: OnlineUserParams) {
  const response = await request<ApiPayload<PageResult<OnlineUserRecord>>>('/api/admin/onlineUsers', {
    method: 'GET',
    params,
  });
  return response.data;
}

export async function fetchOnlineUserStats() {
  const response = await request<ApiPayload<unknown>>('/api/admin/onlineUsers/stats', {
    method: 'GET',
  });
  return response.data;
}

export async function fetchUserSession(userId: API.Int64) {
  const response = await request<ApiPayload<OnlineUserRecord>>(`/api/admin/onlineUsers/${userId}/sessions`, {
    method: 'GET',
  });
  return response.data;
}

export async function forceLogout(userId: API.Int64) {
  const response = await request<ApiPayload<boolean>>('/api/admin/forceLogout', {
    method: 'POST',
    params: { userId },
  });
  return response.data;
}

export async function batchForceLogout(userIds: API.Int64[]) {
  const response = await request<ApiPayload<BatchLogoutResult>>('/api/admin/forceLogout/batch', {
    method: 'POST',
    data: userIds,
  });
  return response.data;
}

export async function banUser(params: { userId: API.Int64; banHours?: number }) {
  const response = await request<ApiPayload<boolean>>('/api/admin/ban', {
    method: 'POST',
    params,
  });
  return response.data;
}

export async function unbanUser(userId: API.Int64) {
  const response = await request<ApiPayload<boolean>>('/api/admin/unban', {
    method: 'POST',
    params: { userId },
  });
  return response.data;
}
