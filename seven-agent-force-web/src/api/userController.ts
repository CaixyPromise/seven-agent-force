import { request } from './request';

function toUserPageQuery(params: API.listUsersParams | API.getDataScopeUserListParams) {
  return {
    current: params.current,
    size: params.size ?? params.pageSize,
    username: params.username,
    nickname: params.nickname,
    status: params.status,
    orgId: params.orgId,
    deptId: params.deptId,
    postId: params.postId,
  };
}

/** 获取有数据权限的用户列表（适配 Go 的数据范围分页端点） GET /user/list/page */
export async function getDataScopeUserList(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getDataScopeUserListParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultPageUserVO>(`/api/user/list/page`, {
    method: 'GET',
    params: {
      ...toUserPageQuery(params),
      current: params.current ?? 1,
      size: params.size ?? params.pageSize ?? 10,
    },
    ...(options || {}),
  });
}

/** 删除用户 POST /user/delete/${param0} */
export async function deleteUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteUserParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/delete/${param0}`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取用户详细信息 GET /user/get/${param0} */
export async function getUserDetail(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserDetailParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultUserVO>(`/api/user/get/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 根据ID获取用户信息 GET /user/get/${param0} */
export async function getUserById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultUserVO>(`/api/user/get/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取当前登录用户信息 GET /user/get/login */
export async function getLoginUser(options?: Parameters<typeof request>[1]) {
  return request<API.ResultUserVO>(`/api/user/get/login`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 分页获取用户列表 GET /user/list */
export async function listUsers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listUsersParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultPageUserVO>(`/api/user/list/page`, {
    method: 'POST',
    params: {
      ...toUserPageQuery(params),
      current: params.current ?? 1,
      size: params.size ?? params.pageSize ?? 10,
    },
    ...(options || {}),
  });
}

/** 创建用户 POST /user/create */
export async function createUser(
  data: API.UserCreateRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/user/create`, {
    method: 'POST',
    data,
    ...(options || {}),
  });
}

/** 更新用户 POST /user/update */
export async function updateUser(
  data: API.UserUpdateRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/user/update`, {
    method: 'POST',
    data,
    ...(options || {}),
  });
}

/** 获取用户角色ID列表 GET /user/${param0}/roles */
export async function getUserRoleIds(
  params: API.getUserRoleIdsParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/user/${param0}/roles`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 分配用户角色 POST /user/${param0}/roles */
export async function assignUserRoles(
  params: API.getUserRoleIdsParams,
  data: API.UserRoleAssignRequest,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/${param0}/roles`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data,
    ...(options || {}),
  });
}

/** 获取用户组织ID列表 GET /user/${param0}/orgs */
export async function getUserOrgIds(
  params: API.getUserOrgIdsParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/user/${param0}/orgs`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 分配用户组织 POST /user/${param0}/orgs */
export async function assignUserOrgs(
  params: API.getUserOrgIdsParams,
  data: API.UserOrgAssignRequest,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/${param0}/orgs`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data,
    ...(options || {}),
  });
}

/** 获取用户部门ID列表 GET /user/${param0}/depts */
export async function getUserDeptIds(
  params: { id: string | number },
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/user/${param0}/depts`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 分配用户部门 POST /user/${param0}/depts */
export async function assignUserDepts(
  params: { id: string | number },
  data: {
    userId: API.Int64;
    deptIds?: API.Int64[];
    ids?: API.Int64[];
    primaryDeptId?: API.Int64;
    primaryId?: API.Int64;
  },
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/${param0}/depts`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data,
    ...(options || {}),
  });
}

/** 获取用户岗位ID列表 GET /user/${param0}/posts */
export async function getUserPostIds(
  params: { id: string | number },
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/user/${param0}/posts`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 分配用户岗位 POST /user/${param0}/posts */
export async function assignUserPosts(
  params: { id: string | number },
  data: {
    userId: API.Int64;
    postIds?: API.Int64[];
    ids?: API.Int64[];
    primaryPostId?: API.Int64;
    primaryId?: API.Int64;
  },
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/${param0}/posts`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data,
    ...(options || {}),
  });
}

/** 获取用户选项列表 GET /user/options */
export async function getUserOptions(
  params?: { keyword?: string; limit?: number; deptId?: API.Int64 },
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultListUserOptionVO>(`/api/user/options`, {
    method: 'GET',
    params: {
      limit: 50,
      ...params,
    },
    ...(options || {}),
  });
}

/** 根据关键词搜索用户 GET /user/search */
export async function searchUsers(
  params?: { keyword?: string; limit?: number; deptId?: API.Int64 },
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultListSimpleUserVO>(`/api/user/search`, {
    method: 'GET',
    params: {
      limit: 20,
      ...params,
    },
    ...(options || {}),
  });
}

/** 根据ID获取简单用户信息 GET /user/simple/{id} */
export async function getSimpleUserById(
  params: { id: API.Int64 },
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultSimpleUserVO>(`/api/user/simple/${params.id}`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 重置用户密码 POST /user/reset-password/${param0} */
export async function resetUserPassword(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resetUserPasswordParams,
  data?: { password?: string },
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/reset-password/${param0}`, {
    method: 'POST',
    headers: data
      ? {
          'Content-Type': 'application/json',
        }
      : undefined,
    params: { ...queryParams },
    data,
    ...(options || {}),
  });
}

/** 更新用户状态 POST /user/status/${param0} */
export async function updateUserStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateUserStatusParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/user/status/${param0}`, {
    method: 'POST',
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 测试接口 GET /user/test */
export async function test(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/user/test`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 测试任意角色权限 GET /user/test/any-role */
export async function testAnyRole(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/user/test/any-role`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 测试岗位权限 GET /user/test/post */
export async function testPost(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/user/test/post`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 测试角色权限 GET /user/test/role */
export async function testRole(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/user/test/role`, {
    method: 'GET',
    ...(options || {}),
  });
}
