import { request } from './request';

/** 获取岗位已分配的角色ID列表 GET /system/post/${param0}/roles */
export async function getPostRoles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPostRolesParams,
  options?: Parameters<typeof request>[1],
) {
  const { postId: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/system/post/${param0}/roles`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 为岗位分配角色 POST /system/post/${param0}/roles */
export async function assignRolesToPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.assignRolesToPostParams,
  body: API.Int64[],
  options?: Parameters<typeof request>[1],
) {
  const { postId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/post/${param0}/roles`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 移除岗位的所有角色分配 DELETE /system/post/${param0}/roles */
export async function removeAllPostRoles(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.removeAllPostRolesParams,
  options?: Parameters<typeof request>[1],
) {
  const { postId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/post/${param0}/roles`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 根据角色ID获取拥有该角色的岗位ID列表 GET /system/post/role/${param0}/posts */
export async function getPostsByRoleId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPostsByRoleIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { roleId: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/system/post/role/${param0}/posts`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}
