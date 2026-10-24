import { request } from './request';

/** 查询授权安全根冗余状态 GET /system/role/security-status */
export async function getRoleSecurityStatus(options?: Parameters<typeof request>[1]) {
  return request<API.ResultRoleSecurityStatusVO>(`/api/system/role/security-status`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 更新角色 PUT /system/role */
export async function updateRole(body: API.RoleCommandDTO, options?: Parameters<typeof request>[1]) {
  return request<API.ResultRoleVO>(`/api/system/role`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 创建角色 POST /system/role */
export async function createRole(body: API.RoleCommandDTO, options?: Parameters<typeof request>[1]) {
  return request<API.ResultRoleVO>(`/api/system/role`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 根据ID获取角色详情 GET /system/role/${param0} */
export async function getRoleById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRoleByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultRoleVO>(`/api/system/role/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除角色 DELETE /system/role/${param0} */
export async function deleteRole(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteRoleParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/role/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取角色的菜单权限ID列表 GET /system/role/${param0}/menus */
export async function getRoleMenuIds(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRoleMenuIdsParams,
  options?: Parameters<typeof request>[1],
) {
  const { roleId: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/system/role/${param0}/menus`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 为角色分配菜单权限 POST /system/role/${param0}/menus */
export async function assignRoleMenus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.assignRoleMenusParams,
  body: API.Int64[],
  options?: Parameters<typeof request>[1],
) {
  const { roleId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/role/${param0}/menus`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

export type RoleDeptIdsVO = {
  roleId: API.Int64;
  deptIds: API.Int64[];
};

export type AssignRoleDeptsRequest = {
  roleId: API.Int64;
  deptIds: API.Int64[];
};

/** 获取角色自定义数据范围部门 GET /system/role/${param0}/depts */
export async function getRoleDeptIds(roleId: API.Int64, options?: Parameters<typeof request>[1]) {
  return request<API.Result<RoleDeptIdsVO>>(`/api/system/role/${roleId}/depts`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 为角色分配自定义数据范围部门 POST /system/role/depts/assign */
export async function assignRoleDepts(
  body: AssignRoleDeptsRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/system/role/depts/assign`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 获取所有启用的角色 GET /system/role/list */
export async function getRoleList(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListRoleVO>(`/api/system/role/list`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 分页查询角色列表 GET /system/role/page */
export async function getRolePage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getRolePageParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultIPageRoleVO>(`/api/system/role/page`, {
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
