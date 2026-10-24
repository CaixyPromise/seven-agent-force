/**
 * 字典管理 API
 */
import { request } from './request';
import type {
  DictType,
  DictItem,
  CreateDictTypeRequest,
  UpdateDictTypeRequest,
  DictTypeQuery,
  CreateDictItemRequest,
  UpdateDictItemRequest,
  BatchUpdateSortRequest,
  DictItemQuery,
} from '@/types/dict';

/**
 * ========== 字典类型管理 (/dict-type) ==========
 */

/**
 * 分页查询字典类型列表
 * GET /api/dict-type/types
 */
export async function getDictTypePage(params: DictTypeQuery): Promise<API.PageResult<DictType>> {
  const res = await request<API.Result<API.PageResult<DictType>>>(`/api/dict-type/types`, {
    method: 'GET',
    params,
  });
  return res.data ?? { records: [], total: 0 };
}

/**
 * 获取字典类型详情
 * GET /api/dict-type/{id}
 */
export async function getDictTypeById(id: API.Int64) {
  return request<API.Result<DictType>>(`/api/dict-type/${id}`, {
    method: 'GET',
  });
}

/**
 * 创建字典类型
 * POST /api/dict-type/add
 */
export async function createDictType(data: CreateDictTypeRequest) {
  return request<API.Result<API.Int64>>(`/api/dict-type/add`, {
    method: 'POST',
    data,
  });
}

/**
 * 更新字典类型
 * POST /api/dict-type/update
 */
export async function updateDictType(data: UpdateDictTypeRequest) {
  return request<API.Result<boolean>>(`/api/dict-type/update`, {
    method: 'POST',
    data,
  });
}

/**
 * 删除字典类型
 * POST /api/dict-type/delete
 */
export async function deleteDictType(id: API.Int64) {
  return request<API.Result<boolean>>(`/api/dict-type/delete`, {
    method: 'POST',
    params: { id },
  });
}

/**
 * 修改字典类型状态
 * POST /api/dict-type/status
 */
export async function changeDictTypeStatus(id: API.Int64, status: number) {
  return request<API.Result<boolean>>(`/api/dict-type/status`, {
    method: 'POST',
    params: { id, status },
  });
}

/**
 * 移动字典类型到指定位置
 * POST /api/dict-type/{id}/move
 * 按照拖拽排序规范，使用 beforeId/afterId 描述目标位置
 */
export async function moveDictType(
  id: API.Int64,
  beforeId?: API.Int64 | null,
  afterId?: API.Int64 | null,
) {
  return request<API.Result<boolean>>(`/api/dict-type/${id}/move`, {
    method: 'POST',
    data: { beforeId, afterId },
  });
}

/**
 * ========== 字典项管理 (/dict) ==========
 */

/**
 * 获取字典项列表
 * GET /api/dict/{typeId}/items
 */
export async function getDictItems(params: DictItemQuery): Promise<DictItem[]> {
  const { dictTypeId, ...rest } = params;
  const res = await request<API.Result<DictItem[]>>(`/api/dict/${dictTypeId}/items`, {
    method: 'GET',
    params: rest,
  });
  return res.data ?? [];
}

/**
 * 创建字典项
 * POST /api/dict/{typeId}/items
 */
export async function createDictItem(data: CreateDictItemRequest) {
  const { dictTypeId, ...rest } = data;
  return request<API.Result<API.Int64>>(`/api/dict/${dictTypeId}/items`, {
    method: 'POST',
    data: rest,
  });
}

/**
 * 更新字典项
 * POST /api/dict/items/update
 */
export async function updateDictItem(data: UpdateDictItemRequest) {
  return request<API.Result<boolean>>(`/api/dict/items/update`, {
    method: 'POST',
    data,
  });
}

/**
 * 删除字典项
 * POST /api/dict/items/delete
 */
export async function deleteDictItem(id: API.Int64) {
  return request<API.Result<boolean>>(`/api/dict/items/delete`, {
    method: 'POST',
    params: { id },
  });
}

/**
 * 修改字典项状态
 * POST /api/dict/items/status
 */
export async function changeDictItemStatus(id: API.Int64, status: number) {
  return request<API.Result<boolean>>(`/api/dict/items/status`, {
    method: 'POST',
    params: { id, status },
  });
}

/**
 * 批量更新字典项排序（保留向后兼容）
 * POST /api/dict/items/sort?typeId=xxx
 */
export async function batchUpdateDictItemSort(data: BatchUpdateSortRequest) {
  return request<API.Result<boolean>>(`/api/dict/items/sort`, {
    method: 'POST',
    data: {
      items: data.items,
    },
    params: { typeId: data.typeId },
  });
}

/**
 * 移动字典项到指定位置
 * POST /api/dict/{typeId}/items/{itemId}/move
 * 按照拖拽排序规范，使用 beforeId/afterId 描述目标位置
 */
export async function moveDictItem(
  typeId: API.Int64,
  itemId: API.Int64,
  beforeId?: API.Int64 | null,
  afterId?: API.Int64 | null,
) {
  return request<API.Result<boolean>>(`/api/dict/${typeId}/items/${itemId}/move`, {
    method: 'POST',
    data: { beforeId, afterId },
  });
}
