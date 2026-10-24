import { request } from './request';

/** 查询所有正常状态的组织 GET /org/active */
export async function getActiveOrgs(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListSysOrg>(`/api/org/active`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 启用/禁用组织 POST /org/changeStatus */
export async function changeStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.changeStatusParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/org/changeStatus`, {
    method: 'POST',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 检查组织编码是否存在 GET /org/checkCode */
export async function checkCodeExists(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.checkCodeExistsParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/org/checkCode`, {
    method: 'GET',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 根据父ID查询子组织 GET /org/children/${param0} */
export async function getChildrenByParentId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getChildrenByParentIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { parentId: param0, ...queryParams } = params;
  return request<API.ResultListSysOrg>(`/api/org/children/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 创建组织 POST /org/create */
export async function createOrg(body: API.SysOrg, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/org/create`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 删除组织 POST /org/delete/${param0} */
export async function deleteOrg(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteOrgParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/org/delete/${param0}`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 根据ID查询组织 GET /org/get/${param0} */
export async function getOrgById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOrgByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultSysOrg>(`/api/org/get/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 根据编码查询组织 GET /org/getByCode/${param0} */
export async function getOrgByCode(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOrgByCodeParams,
  options?: Parameters<typeof request>[1],
) {
  const { code: param0, ...queryParams } = params;
  return request<API.ResultSysOrg>(`/api/org/getByCode/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 根据用户ID查询所属组织 GET /org/getByUserId/${param0} */
export async function getOrgByUserId(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getOrgByUserIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultSysOrg>(`/api/org/getByUserId/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 移动组织 POST /org/move */
export async function moveOrg(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.moveOrgParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/org/move`, {
    method: 'POST',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 查询组织树 GET /org/tree */
export async function getOrgTree(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListSysOrg>(`/api/org/tree`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 更新组织 POST /org/update */
export async function updateOrg(body: API.SysOrg, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/org/update`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}
