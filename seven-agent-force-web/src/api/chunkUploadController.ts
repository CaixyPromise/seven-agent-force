import { request } from './request';

/** 初始化分块上传 POST /api/chunk-upload/init */
export async function initChunkUpload(
    data: {
        fileName: string;
        fileSize: number;
        chunkSize?: number;
        fileSha256?: string;
        contentType?: string;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultChunkUploadInitResponse>(`/api/chunk-upload/init`, {
        method: 'POST',
        data,
        ...(options || {}),
    });
}

/** 上传分块 POST /api/chunk-upload/part */
export async function uploadChunkPart(
    uploadId: string,
    partNumber: number,
    chunk: Blob,
    chunkSha256?: string,
    options?: Parameters<typeof request>[1],
) {
    const formData = new FormData();
    formData.append('uploadId', uploadId);
    formData.append('partNumber', String(partNumber));
    formData.append('chunk', chunk);
    if (chunkSha256) {
        formData.append('chunkSha256', chunkSha256);
    }

    return request<API.ResultChunkPartResponse>(`/api/chunk-upload/part`, {
        method: 'POST',
        data: formData,
        headers: {
            'Content-Type': 'multipart/form-data',
        },
        ...(options || {}),
    });
}

/** 完成分块上传 POST /api/chunk-upload/complete */
export async function completeChunkUpload(
    data: {
        uploadId: string;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultUploadResult>(`/api/chunk-upload/complete`, {
        method: 'POST',
        data,
        ...(options || {}),
    });
}

/** 取消分块上传 POST /api/chunk-upload/abort */
export async function abortChunkUpload(
    uploadId: string,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/chunk-upload/abort`, {
        method: 'POST',
        params: { uploadId },
        ...(options || {}),
    });
}

/** 查询上传状态 GET /api/chunk-upload/status */
export async function getChunkUploadStatus(
    uploadId: string,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultChunkUploadStatusResponse>(`/api/chunk-upload/status`, {
        method: 'GET',
        params: { uploadId },
        ...(options || {}),
    });
}

/** 获取进行中的上传列表 GET /api/chunk-upload/active */
export async function getActiveUploads(
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultChunkUploadStatusList>(`/api/chunk-upload/active`, {
        method: 'GET',
        ...(options || {}),
    });
}

/** 获取断点续传信息 GET /api/chunk-upload/resume-info */
export async function getResumeInfo(
    uploadId: string,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultChunkUploadStatusResponse>(`/api/chunk-upload/resume-info`, {
        method: 'GET',
        params: { uploadId },
        ...(options || {}),
    });
}
