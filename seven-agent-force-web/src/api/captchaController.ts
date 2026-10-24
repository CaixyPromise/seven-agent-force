import { request } from './request';

/** 创建验证码 创建验证码图片和ID GET /captcha/get */
export async function getCaptchaUsingGet1(options?: Parameters<typeof request>[1]) {
  return request<API.ResultCaptchaVO>('/api/captcha/get', {
    method: 'GET',
    ...(options || {}),
  });
}


/** 验证验证码 验证用户输入的验证码是否正确 POST /captcha/verify */
export async function verifyCaptcha(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.verifyCaptchaParams,
  options?: Parameters<typeof request>[1],
) {
  return request<API.ResultBoolean>(`/api/captcha/verify`, {
    method: 'POST',
    params: {
      ...params,
    },
    ...(options || {}),
  });
}
