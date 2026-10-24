import { request } from './request';

export type PostMutationBody = Omit<API.SysPost, 'id' | 'deptId' | 'orgId'> & {
  id?: API.Int64;
  deptId: API.Int64;
  orgId?: API.Int64;
};

/** 分页查询岗位列表 GET /system/post/page */
export async function getPostPage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPostPageParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultIPageSysPost>(`/api/system/post/page`, {
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

/** 获取所有启用的岗位 GET /system/post/list */
export async function getPostList(options?: Parameters<typeof request>[1]) {
  return request<API.ResultListSysPost>(`/api/system/post/list`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 根据ID获取岗位详情 GET /system/post/${param0} */
export async function getPostById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPostByIdParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultSysPost>(`/api/system/post/${param0}`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 创建岗位 POST /system/post */
export async function createPost(body: PostMutationBody, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/system/post`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 更新岗位 PUT /system/post */
export async function updatePost(body: PostMutationBody, options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/system/post`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 删除岗位 DELETE /system/post/${param0} */
export async function deletePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deletePostParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/post/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 批量删除岗位 DELETE /system/post/batch */
export async function batchDeletePosts(body: API.Int64[], options?: Parameters<typeof request>[1]) {
  return request<API.ResultBoolean>(`/api/system/post/batch`, {
    method: 'DELETE',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 更改岗位状态 PUT /system/post/${param0}/status */
export async function changePostStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.changePostStatusParams,
  options?: Parameters<typeof request>[1],
) {
  const { id: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/system/post/${param0}/status`, {
    method: 'PUT',
    params: { ...queryParams },
    ...(options || {}),
  });
}
