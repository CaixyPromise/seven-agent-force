import { request } from './request';

/** 更新菜单 PUT /system/menu */
export async function updateMenu(body: API.MenuCommandDTO, options?: Parameters<typeof request>[1]) {
  return request<API.ResultMenuVO>(`/api/system/menu`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 创建菜单 POST /system/menu */
export async function createMenu(body: API.MenuCommandDTO, options?: Parameters<typeof request>[1]) {
  return request<API.ResultMenuVO>(`/api/system/menu`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 根据ID获取菜单详情 GET /system/menu/${param0} */
export async function getMenuById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMenuByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultMenuVO>(`/api/system/menu/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除菜单 DELETE /system/menu/${param0} */
export async function deleteMenu(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteMenuParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/menu/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取菜单绑定的权限ID列表 GET /system/menu/${param0}/permissions */
export async function getMenuPermissionIds(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getMenuPermissionIdsParams,
  options?: Parameters<typeof request>[1],
) {
  const { menuId: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/system/menu/${param0}/permissions`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 绑定菜单与权限资源 POST /system/menu/${param0}/permissions */
export async function bindMenuPermissions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.bindMenuPermissionsParams,
  body: API.MenuPermissionAssignDTO,
  options?: Parameters<typeof request>[1],
) {
  const { menuId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/menu/${param0}/permissions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 查询权限资源列表 GET /system/menu/permissions */
export async function listPermissions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listPermissionsParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultListPermissionVO>(`/api/system/menu/permissions`, {
    method: 'GET',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 创建权限资源 POST /system/menu/permissions */
export async function createPermission(
  body: API.PermissionCommandDTO,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultPermissionVO>(`/api/system/menu/permissions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 获取权限资源详情 GET /system/menu/permissions/${param0} */
export async function getPermission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPermissionParams,
  options?: Parameters<typeof request>[1],
) {
  const { permissionId: param0, ...queryParams } = params;
  return request<API.ResultPermissionVO>(`/api/system/menu/permissions/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 更新权限资源 PUT /system/menu/permissions/${param0} */
export async function updatePermission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updatePermissionParams,
  body: API.PermissionCommandDTO,
  options?: Parameters<typeof request>[1],
) {
  const { permissionId: param0, ...queryParams } = params;
  return request<API.ResultPermissionVO>(`/api/system/menu/permissions/${param0}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 删除权限资源 DELETE /system/menu/permissions/${param0} */
export async function deletePermission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deletePermissionParams,
  options?: Parameters<typeof request>[1],
) {
  const { permissionId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/menu/permissions/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取菜单树 GET /system/menu/tree */
export async function getMenuTree(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListMenuVO>(`/api/system/menu/tree`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取启用的菜单树 GET /system/menu/tree/enabled */
export async function getEnabledMenuTree(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListMenuVO>(`/api/system/menu/tree/enabled`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取用户菜单权限 GET /system/menu/user/${param0} */
export async function getUserMenus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserMenusParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultListMenuVO>(`/api/system/menu/user/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取用户权限标识列表 GET /system/menu/user/${param0}/permissions */
export async function getUserPermissions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserPermissionsParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultListString>(`/api/system/menu/user/${param0}/permissions`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}
