import { request } from './request';

/** 获取当前登录用户聚合信息 GET /auth/me */
export async function getCurrentUser(options?: Record<string, unknown>) {
  return request<API.ResultCurrentUserResponse>(`/api/auth/me`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取当前用户菜单权限 GET /auth/menus */
export async function getCurrentUserMenus(options?: Record<string, unknown>) {
  return request<API.ResultListMenuVO>(`/api/auth/menus`, {
    method: 'GET',
    ...(options || {}),
  });
}

/** 获取当前用户在指定模块下的权限 GET /auth/permissions */
export async function getUserPermissionsByModule(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getUserPermissionsParams,
  options?: Record<string, unknown>,
) {
  return request<API.ResultListString>(`/api/auth/permissions`, {
    method: 'GET',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 发起 step-up 挑战 POST /auth/step-up/challenge */
export async function createStepUpChallenge(
  body: API.StepUpChallengeRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultStepUpChallengeVO>(`/api/auth/step-up/challenge`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 校验 step-up 挑战 POST /auth/step-up/verify */
export async function verifyStepUp(
  body: API.StepUpVerifyRequest,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultStepUpTokenVO>(`/api/auth/step-up/verify`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}
