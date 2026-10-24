import { request } from './request';

/** 分页查询操作日志 分页查询系统操作日志，支持多条件过滤 GET /admin/logs/operation */
export async function getOperationLogs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOperationLogsParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultPageSysOperationLog>(`/api/admin/logs/operation`, {
    method: 'GET',
    params: {
      // current has a default value: 1
      current: '1',
      // size has a default value: 10
      size: '10',

      ...params,
    },
    ...(options || {}),
  });
}

/** 查询操作日志详情 根据ID查询操作日志的详细信息 GET /admin/logs/operation/${param0} */
export async function getOperationLogById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOperationLogByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultSysOperationLog>(`/api/admin/logs/operation/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 清理过期日志 清理指定天数前的操作日志 POST /admin/logs/operation/clean */
export async function cleanExpiredLogs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cleanExpiredLogsParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultInteger>(`/api/admin/logs/operation/clean`, {
    method: 'POST',
    params: {
      // days has a default value: 30
      days: '30',
      ...params,
    },
    ...(options || {}),
  });
}

/** 按时间范围删除日志 删除指定时间范围内的操作日志 POST /admin/logs/operation/deleteByTimeRange */
export async function deleteLogsByTimeRange(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteLogsByTimeRangeParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultInteger>(`/api/admin/logs/operation/deleteByTimeRange`, {
    method: 'POST',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 导出操作日志 导出操作日志到Excel文件 GET /admin/logs/operation/export */
export async function exportOperationLogs(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.exportOperationLogsParams,
  options?: Parameters<typeof request>[1],
) {
  return request<Blob>(`/api/admin/logs/operation/export`, {
    method: 'GET',
    params: {
      ...params,
    },
    responseType: 'blob',
    ...(options || {}),
  });
}

/** 获取我的操作日志 分页查询当前用户的操作日志 GET /admin/logs/operation/my */
export async function getMyOperationLogPage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMyOperationLogPageParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultIPageOperationLogVO>(`/api/admin/logs/operation/my`, {
    method: 'GET',
    params: {
      // current has a default value: 1
      current: '1',
      // size has a default value: 10
      size: '10',

      ...params,
    },
    ...(options || {}),
  });
}

/** 获取操作类型列表 获取可用于筛选的操作类型及其服务端显示名称 GET /admin/logs/operation/types */
export async function getOperationTypes(options?: Parameters<typeof request>[1]) {
  return request<API.ResultOperationTypeOptionArray>(`/api/admin/logs/operation/types`, {
    method: 'GET',
    ...(options || {}),
  });
}
