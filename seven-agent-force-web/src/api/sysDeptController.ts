import { request } from './request';

/** 更新部门 PUT /system/dept */
export async function updateDept(body: API.SysDept, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/system/dept`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 创建部门 POST /system/dept */
export async function createDept(body: API.SysDept, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/system/dept`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 根据ID获取部门详情 GET /system/dept/${param0} */
export async function getDeptById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getDeptByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultSysDept>(`/api/system/dept/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 删除部门 DELETE /system/dept/${param0} */
export async function deleteDept(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteDeptParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/dept/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 获取部门的所有子部门ID GET /system/dept/${param0}/children */
export async function getChildDeptIds(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getChildDeptIdsParams,
  options?: Parameters<typeof request>[1],
) {
  const { deptId: param0, ...queryParams } = params;
  return request<API.ResultListLong>(`/api/system/dept/${param0}/children`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 管理员或HR专用接口 GET /system/dept/admin-or-hr */
export async function adminOrHrOnly(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/system/dept/admin-or-hr`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 复合权限测试 GET /system/dept/complex-permission */
export async function complexPermission(options?: Parameters<typeof request>[1]) {
  return request<API.ResultString>(`/api/system/dept/complex-permission`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取部门树 GET /system/dept/tree */
export async function getDeptTree(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListSysDept>(`/api/system/dept/tree`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取启用的部门树 GET /system/dept/tree/enabled */
export async function getEnabledDeptTree(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListSysDept>(`/api/system/dept/tree/enabled`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 搜索部门选项 GET /system/dept/options */
export async function getDeptOptions(
  params?: { keyword?: string; orgId?: API.Int64; status?: number; limit?: number },
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultListSysDept>(`/api/system/dept/options`, {
    method: 'GET',
    params: {
      limit: 20,
      ...params,
    },
    ...(options || {}),
  });
}
