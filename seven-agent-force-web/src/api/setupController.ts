import { request } from '@/api/request';
import type { ApiResponse, SetupOwnerRequest, SetupOwnerResult, SetupStatus } from '@/lib/http/types';

export async function getSetupStatusApi() {
  return request<ApiResponse<SetupStatus>>('/api/setup/status', {
    method: 'GET',
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function createSetupOwnerApi(data: SetupOwnerRequest, setupToken: string) {
  return request<ApiResponse<SetupOwnerResult>>('/api/setup/owner', {
    method: 'POST',
    data,
    headers: {
      'X-Setup-Token': setupToken,
    },
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}
