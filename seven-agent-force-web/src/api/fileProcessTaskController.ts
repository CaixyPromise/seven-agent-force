import { request } from './request';

/** 获取文件处理任务列表 GET /api/file-process-task */
export async function getFileProcessTasks(
    params?: {
        current?: number;
        pageSize?: number;
        status?: number;
        taskType?: string;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultPageFileProcessTask>(`/api/file-process-task`, {
        method: 'GET',
        params,
        ...(options || {}),
    });
}

/** 获取任务详情 GET /api/file-process-task/{id} */
export async function getFileProcessTask(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultFileProcessTask>(`/api/file-process-task/${id}`, {
        method: 'GET',
        ...(options || {}),
    });
}

/** 重试失败任务 POST /api/file-process-task/{id}/retry */
export async function retryFileProcessTask(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/file-process-task/${id}/retry`, {
        method: 'POST',
        ...(options || {}),
    });
}

/** 重放任务 POST /api/file-process-task/{id}/replay */
export async function replayFileProcessTask(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/file-process-task/${id}/replay`, {
        method: 'POST',
        ...(options || {}),
    });
}

/** 获取任务统计 GET /api/file-process-task/stats */
export async function getFileProcessTaskStats(
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultTaskStatsResponse>(`/api/file-process-task/stats`, {
        method: 'GET',
        ...(options || {}),
    });
}

/** 批量重试失败任务 POST /api/file-process-task/batch-retry */
export async function batchRetryTasks(
    ids: API.Int64[],
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/file-process-task/batch-retry`, {
        method: 'POST',
        data: { ids },
        ...(options || {}),
    });
}
