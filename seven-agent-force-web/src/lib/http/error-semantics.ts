import type { ApiResponse } from './types';

type ErrorPayload<T> = Partial<ApiResponse<T>> & {
  error?: string;
  error_description?: string;
};

const CODE_FALLBACK_MESSAGES: Record<number, string> = {
  40000: '请求参数错误',
  40100: '请先登录',
  40101: '未授权',
  40120: '需要完成挑战验证',
  40300: '权限不足',
  40310: '数据范围不可访问',
  40400: '资源不存在',
  40900: '当前状态不可操作',
  42900: '请求过于频繁，请稍后重试',
  50000: '系统内部异常',
  50001: '操作失败',
};

const SERVER_MESSAGE_OVERRIDES: Record<string, string> = {
  当前主体没有可用的验证方式: '当前账号未绑定可用的二次验证方式，请先在账号设置绑定后再操作',
};

export function resolveSemanticErrorMessage<T>(
  payload: ErrorPayload<T> | undefined,
  fallbackMessage: string,
) {
  if (payload?.message) {
    return SERVER_MESSAGE_OVERRIDES[payload.message] || payload.message;
  }

  const code = typeof payload?.code === 'number' ? payload.code : undefined;
  const semanticMessage = code === undefined ? undefined : CODE_FALLBACK_MESSAGES[code];
  return semanticMessage || payload?.error_description || payload?.error || fallbackMessage;
}

export function createApiError<T>(payload: ErrorPayload<T>, fallbackMessage: string) {
  const error = new Error(resolveSemanticErrorMessage(payload, fallbackMessage)) as Error & {
    code?: number;
    payload?: ErrorPayload<T>;
  };
  error.code = typeof payload?.code === 'number' ? payload.code : undefined;
  error.payload = payload;
  return error;
}
