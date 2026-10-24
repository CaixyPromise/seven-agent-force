/**
 * 配置客户端 API
 * 提供给业务运行时使用的配置读取接口
 */
import { request } from './request';
import type { ConfigValueDTO, ConfigBatchRequest } from '@/types/configClient';

function requireData<T>(response: API.Result<T>, message: string): T {
  if (response.data === undefined) {
    throw new Error(message);
  }
  return response.data;
}

/**
 * 按配置键获取配置值
 * GET /api/config-client/{configKey}
 * @param configKey 配置键
 * @returns 配置值对象
 */
export async function getConfigByKey(configKey: string): Promise<ConfigValueDTO> {
  const res = await request<API.Result<ConfigValueDTO>>(`/api/config-client/${encodeURIComponent(configKey)}`, {
    method: 'GET',
  });
  return requireData(res, '配置响应缺少数据');
}

/**
 * 批量获取配置
 * POST /api/config-client/batch
 * @param configKeys 配置键列表
 * @returns 配置键到配置值的映射
 */
export async function getConfigBatch(configKeys: string[]): Promise<Record<string, ConfigValueDTO>> {
  const res = await request<API.Result<Record<string, ConfigValueDTO>>>(`/api/config-client/batch`, {
    method: 'POST',
    data: { configKeys } as ConfigBatchRequest,
  });
  return res.data ?? {};
}
