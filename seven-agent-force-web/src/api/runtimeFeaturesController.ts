import { request } from '@/api/request';
import type {
  ApiResponse,
  RuntimeFeatures,
} from '@/lib/http/types';
import {
  SAFE_RUNTIME_FEATURES,
  normalizeRuntimeFeatures,
} from '@/lib/navigation/runtimeFeaturePolicy';

export const RUNTIME_FEATURES_ENDPOINT = '/api/system/features/runtime';

function unwrapApiData<T>(response: ApiResponse<T>, fallbackMessage: string): T {
  if (!response || typeof response !== 'object' || !('code' in response)) {
    return response as T;
  }
  if (response.code !== 0 && response.code !== 200) {
    throw new Error(response.message || fallbackMessage);
  }
  return response.data as T;
}

export { SAFE_RUNTIME_FEATURES, normalizeRuntimeFeatures };

export async function getRuntimeFeatures(): Promise<RuntimeFeatures> {
  const response = await request<ApiResponse<unknown>>(RUNTIME_FEATURES_ENDPOINT, {
    method: 'GET',
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
  return normalizeRuntimeFeatures(unwrapApiData(response, '运行特性接口返回失败'));
}
