/**
 * 字典客户端 API
 * 提供给业务运行时使用的字典读取接口
 */
import { request } from './request';
import type { DictItemVO, DictBatchRequest, DictBatchResponse } from '@/types/dictClient';

/**
 * 按字典编码获取字典项列表
 * GET /api/dict-client/{dictCode}
 * @param dictCode 字典编码
 * @returns 字典项列表
 */
export async function getDictByCode(dictCode: string): Promise<DictItemVO[]> {
  const res = await request<API.Result<DictItemVO[]>>(`/api/dict-client/${encodeURIComponent(dictCode)}`, {
    method: 'GET',
  });
  return res.data ?? [];
}

/**
 * 批量获取字典
 * POST /api/dict-client/batch
 * @param dictCodes 字典编码列表
 * @param force 是否强制查询数据库（不走缓存）
 * @returns 批量字典响应
 */
export async function getDictBatch(dictCodes: string[], force: boolean = false): Promise<DictBatchResponse> {
  const res = await request<API.Result<DictBatchResponse>>(`/api/dict-client/batch`, {
    method: 'POST',
    data: { dictCodes, force } as DictBatchRequest,
  });
  return res.data ?? { record: {}, missing: [] };
}
