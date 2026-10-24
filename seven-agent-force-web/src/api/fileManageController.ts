import { request } from './request';

export interface FileBatchDeleteItem {
  fileId?: API.Int64;
  reason?: string;
  message?: string;
}

export interface FileBatchDeleteResult {
  success: boolean;
  outcome: 'FULL_SUCCESS' | 'PARTIAL_SUCCESS' | 'FULL_FAILED' | string;
  requestedCount: number;
  deletedCount: number;
  skippedCount: number;
  deletedIds: API.Int64[];
  skippedItems: FileBatchDeleteItem[];
}

/** 获取文件列表 GET /api/file-manage/list */
export async function getFileList(
  params: {
    current?: number;
    pageSize?: number;
    fileName?: string;
    fileType?: string;
    bizType?: number;
    startTime?: string;
    endTime?: string;
  },
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultPageFileInfo>(`/api/file-manage/list`, {
    method: 'GET',
    params,
    ...(options || {}),
  });
}

/** 获取文件详情 GET /api/file-manage/{id} */
export async function getFileDetail(
  id: API.Int64,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultFileInfo>(`/api/file-manage/${id}`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取文件引用列表 GET /api/file-manage/{id}/references */
export async function getFileReferences(
  id: API.Int64,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultFileReferenceList>(`/api/file-manage/${id}/references`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 更新文件引用访问级别 PUT /api/file-manage/references/{id}/access-level */
export async function updateReferenceAccessLevel(
  id: API.Int64,
  accessLevel: number,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/file-manage/references/${id}/access-level`, {
    method: 'PUT',
    data: { accessLevel },
    ...(options || {}),
  });
}

/** 删除文件 DELETE /api/file-manage/{id} */
export async function deleteFile(
  id: API.Int64,
  options?: Parameters<typeof request>[1],
) {
  return request<API.Result<FileBatchDeleteResult>>(`/api/file-manage/${id}`, {
    method: 'DELETE',
    ...(options || {}),
  });
}

/** 批量删除文件 DELETE /api/file-manage/batch */
export async function batchDeleteFiles(
  ids: API.Int64[],
  options?: Parameters<typeof request>[1],
) {
  return request<API.Result<FileBatchDeleteResult>>(`/api/file-manage/batch`, {
    method: 'DELETE',
    data: { ids },
    ...(options || {}),
  });
}

/** 获取文件统计 GET /api/file-manage/stats */
export async function getFileStats(
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultFileStats>(`/api/file-manage/stats`, {
    method: 'GET',
    ...(options || {}),
  });
}
