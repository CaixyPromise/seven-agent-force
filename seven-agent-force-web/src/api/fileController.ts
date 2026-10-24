import { request } from './request';

/** 秒传前检查文件是否已存在 GET /file/check */
export async function checkFileExist(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.checkFileExistParams,
  options?: Parameters<typeof request>[1],
) {
  const { sha256, fileSize, size } = params;
  return request<API.ResultCheckFileExistResponse>(`/api/file/check`, {
    method: 'GET',
    params: {
      sha256,
      fileSize: fileSize ?? size,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /file/download */
export async function downloadFileById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.downloadFileByIdParams,
  options?: Parameters<typeof request>[1],
) {
  return request<Blob>(`/api/file/download`, {
    method: 'GET',
    responseType: 'blob',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 获取文件下载地址 GET /uploads/files/${param0}/download-url */
export async function getFileDownloadUrl(
  param0: API.Int64,
  options?: Parameters<typeof request>[1],
) {
  return request<{
    code: number;
    message?: string;
    data?: {
      url?: string;
      downloadUrl?: string;
    };
  }>(`/api/uploads/files/${param0}/download-url`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 普通上传文件；终态只返回 fileId POST /file/upload */
export async function uploadFile(
  file: File,
  options?: Parameters<typeof request>[1],
) {
  const formData = new FormData();
  formData.append('file', file);

  return request<API.ResultUploadResult>(`/api/file/upload`, {
    method: 'POST',
    data: formData,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /file/upload/faster */
export async function uploadFileFaster(
  body: API.UploadFileRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultUploadResult>(`/api/file/upload/faster`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}
