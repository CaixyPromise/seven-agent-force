import { request } from '@/api/request';
import { toApiIdParam } from '@/lib/apiId';

interface ApiPayload<T> {
  code: number;
  data: T;
  message?: string;
}

export async function checkOrgCode(code: string, excludeId?: API.Int64) {
  try {
    const response = await request<ApiPayload<boolean>>('/api/org/checkCode', {
      method: 'GET',
		params: excludeId !== undefined ? { code, excludeId: toApiIdParam(excludeId) } : { code },
    });
    return Boolean(response.data);
  } catch {
    return false;
  }
}
