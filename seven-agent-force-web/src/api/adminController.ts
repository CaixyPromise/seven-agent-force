import { request } from './request';

/** 强制用户下线 管理员强制指定用户下线 POST /admin/kick/${param0} */
export async function kickUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.kickUserParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/admin/kick/${param0}`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 批量强制用户下线 管理员批量强制指定用户下线 POST /admin/kick/batch */
export async function batchKickUsers(body: API.Int64[], options?: Parameters<typeof request>[1]) {
  return request<API.ResultBatchLogoutResultVO>(`/api/admin/kick/batch`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 检查用户是否在线 检查指定用户是否在线 GET /admin/online/check/${param0} */
export async function checkUserOnline(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.checkUserOnlineParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/admin/online/check/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取在线用户数量 获取当前在线用户总数 GET /admin/online/count */
export async function getOnlineUserCount(options?: Parameters<typeof request>[1]) {
  const response = await request<API.Result<API.Int64>>(`/api/admin/online/count`, {
    method: 'GET',
    ...(options || {}),
  });
  return {
    ...response,
    data: response.data === undefined ? undefined : Number(response.data),
  } satisfies API.ResultInteger;
}

/** 获取在线用户统计信息 获取当前在线用户数量统计和分布情况 GET /admin/online/stats */
export async function getOnlineUserStats(options?: Parameters<typeof request>[1]) {
  return request<API.ResultOnlineUserStatsVO>(`/api/admin/online/stats`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取在线用户列表 分页获取当前在线用户列表，支持搜索过滤 GET /admin/online/users */
export async function getOnlineUsers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOnlineUsersParams,
  options?: Parameters<typeof request>[1],
) {
  const { pageSize, ...restParams } = params;
  return request<API.ResultPageOnlineUserVO>(`/api/admin/online/users`, {
    method: 'GET',
    params: {
      // current has a default value: 1
      current: '1',
      // size has a default value: 10
      size: '10',
      ...restParams,
      ...(pageSize !== undefined ? { size: pageSize } : {}),
    },
    ...(options || {}),
  });
}

/** 获取用户会话信息 获取指定用户的在线会话详细信息 GET /admin/online/users/${param0} */
export async function getUserSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserSessionParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultOnlineUserVO>(`/api/admin/online/users/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}
