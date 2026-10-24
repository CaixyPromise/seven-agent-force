import { request } from './request';

/** 此处后端没有提供注释 GET / */
export async function check(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: Record<string, string | undefined>,
  options?: Parameters<typeof request>[1],
) {
  return request<string>(`/api/`, {
    method: 'GET',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST / */
export async function receiveMessage(options?: Parameters<typeof request>[1]) {
  return request<unknown>(`/api/`, {
    method: 'POST',
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /setMenu */
export async function setMenu(options?: Parameters<typeof request>[1]) {
  return request<string>(`/api/setMenu`, {
    method: 'GET',
    ...(options || {}),
  });
}
