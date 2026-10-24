import { request } from './request';

/** 获取存储策略列表 GET /api/storage-strategy */
export async function getStorageStrategies(
    params?: {
        current?: number;
        pageSize?: number;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultPageStorageStrategy>(`/api/storage-strategy`, {
        method: 'GET',
        params,
        ...(options || {}),
    });
}

/** 获取存储策略详情 GET /api/storage-strategy/{id} */
export async function getStorageStrategy(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultStorageStrategy>(`/api/storage-strategy/${id}`, {
        method: 'GET',
        ...(options || {}),
    });
}

/** 创建存储策略 POST /api/storage-strategy */
export async function createStorageStrategy(
    data: {
        strategyName: string;
        providerType: string;
        configJson: string;
        isDefault?: boolean;
        priority?: number;
        failureRateThreshold?: number;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultInt64>(`/api/storage-strategy`, {
        method: 'POST',
        data,
        ...(options || {}),
    });
}

/** 更新存储策略 PUT /api/storage-strategy/{id} */
export async function updateStorageStrategy(
    id: API.Int64,
    data: {
        strategyName?: string;
        configJson?: string;
        isDefault?: boolean;
        isEnabled?: boolean;
        priority?: number;
        failureRateThreshold?: number;
    },
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/storage-strategy/${id}`, {
        method: 'PUT',
        data,
        ...(options || {}),
    });
}

/** 删除存储策略 DELETE /api/storage-strategy/{id} */
export async function deleteStorageStrategy(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/storage-strategy/${id}`, {
        method: 'DELETE',
        ...(options || {}),
    });
}

/** 设为默认策略 PUT /api/storage-strategy/{id}/default */
export async function setDefaultStrategy(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultBoolean>(`/api/storage-strategy/${id}/default`, {
        method: 'PUT',
        ...(options || {}),
    });
}

/** 健康检查 GET /api/storage-strategy/{id}/health */
export async function checkStorageHealth(
    id: API.Int64,
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultHealthCheckResponse>(`/api/storage-strategy/${id}/health`, {
        method: 'GET',
        ...(options || {}),
    });
}

/** 获取支持的存储类型 GET /api/storage-strategy/providers */
export async function getStorageProviders(
    options?: Parameters<typeof request>[1],
) {
    return request<API.ResultStringList>(`/api/storage-strategy/providers`, {
        method: 'GET',
        ...(options || {}),
    });
}
