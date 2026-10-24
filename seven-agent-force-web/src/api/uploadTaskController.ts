import { request } from './request';

/** 查询异步上传 receipt 状态；只有 CLEAN 终态返回 fileId。 */
export async function getUploadTaskStatus(
  taskId: string,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultUploadTaskStatus>(`/api/uploads/${encodeURIComponent(taskId)}/status`, {
    method: 'GET',
    ...(options || {}),
  });
}
