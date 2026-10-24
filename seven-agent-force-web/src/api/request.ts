import axios, { AxiosError, AxiosRequestConfig, AxiosRequestHeaders, AxiosResponse } from 'axios';
import { getSsoRuntimeConfig } from '@/lib/auth/sso-runtime';
import { buildLoginRedirectUrl } from '@/lib/auth/navigation';
import { useAuthStore } from '@/store/auth';
import { getOrCreateDeviceId } from '@/lib/auth/device';
import { readAuthModeHint } from '@/lib/auth/session-hint';
import { buildAuthorizationHeader, shouldRefreshAccessToken } from '@/lib/auth/token';
import {
  buildRequestFingerprint,
  isChallengeRetryError,
  resolveWithChallenge,
} from '@/lib/http/challenge-orchestrator';
import { createApiError } from '@/lib/http/error-semantics';
import type {
  ApiResponse,
  ChallengeProofHeaders,
  ChallengeRequiredPayload,
  RefreshResponse,
} from '@/lib/http/types';
import { parseJsonPreserveLargeIntegers } from '@/lib/lossless-json.mjs';

export type RequestOptions = AxiosRequestConfig & {
  method?: 'GET' | 'POST' | 'PUT' | 'DELETE' | 'PATCH';
  data?: unknown;
  params?: unknown;
  headers?: Record<string, string>;
  responseType?: 'json' | 'blob' | 'arraybuffer' | 'document' | 'text' | 'stream';
  skipGlobalChallenge?: boolean;
  skipAuthRefresh?: boolean;
  skipAuthRedirect?: boolean;
  _challengeFingerprint?: string;
};

type RetryRequestConfig = AxiosRequestConfig & {
  _authRetry?: boolean;
  _skipAuthRefresh?: boolean;
  _challengeRetry?: boolean;
  _challengeFingerprint?: string;
  skipGlobalChallenge?: boolean;
  skipAuthRefresh?: boolean;
  skipAuthRedirect?: boolean;
};

function readChallengePayload(payload: ApiResponse<unknown>): ChallengeRequiredPayload | null {
  const challengePayload = payload?.data as ChallengeRequiredPayload | undefined;
  if (!challengePayload?.challengeIdentifier) {
    return null;
  }
  return challengePayload;
}

function buildProofHeaderBag(proofHeaders: ChallengeProofHeaders) {
  return {
    'Proof-Token': proofHeaders.proofToken,
    'Flow-Nonce': proofHeaders.flowNonce,
  } as Record<string, string>;
}

function redirectToLogin() {
  if (typeof window === 'undefined') {
    return;
  }
  window.location.href = buildLoginRedirectUrl(window.location.href);
}

function traceAuthRedirect(reason: string, detail: Record<string, unknown>) {
  if (typeof window === 'undefined') {
    return;
  }
  console.warn('[auth-redirect-trace]', {
    reason,
    href: window.location.href,
    ...detail,
  });
}

function isRefreshRequest(url?: string) {
  return typeof url === 'string' && url.includes('/sso/oauth2/token');
}

function canAttemptAuthRefresh() {
  const state = useAuthStore.getState();
  return !!state.accessToken || readAuthModeHint() === 'sso';
}

function getAuthorizationHeader() {
  const state = useAuthStore.getState();
  return buildAuthorizationHeader(state.accessToken, state.tokenType);
}

function clearSession() {
  useAuthStore.getState().clearSession();
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function applyRefreshedAccessToken(data: RefreshResponse | undefined) {
  if (!data?.accessToken) {
    throw new Error('无法刷新登录状态');
  }
  useAuthStore.getState().applyAccessToken(data);
}

function normalizeOidcRefreshPayload(raw: unknown): RefreshResponse {
  if (!raw || typeof raw !== 'object') {
    return {};
  }
  const source = raw as Record<string, unknown>;
  return {
    accessToken:
      (source.accessToken as string | undefined) ?? (source.access_token as string | undefined),
    tokenType:
      (source.tokenType as string | undefined) ?? (source.token_type as string | undefined),
    accessTtlSec:
      (source.accessTtlSec as number | undefined) ?? (source.expires_in as number | undefined),
    refreshToken:
      (source.refreshToken as string | undefined) ?? (source.refresh_token as string | undefined),
  };
}

function isRetryableSsoRefreshConflict(error: unknown) {
  if (!axios.isAxiosError(error)) {
    return false;
  }
  return error.response?.status === 409
    && (error.response?.data as { error?: string } | undefined)?.error === 'concurrent_request';
}

const apiClient = axios.create({
  timeout: 10000,
  withCredentials: true,
  transformResponse: [(data, headers) => parseJsonPreserveLargeIntegers(data, headers)],
});

const refreshClient = axios.create({
  timeout: 10000,
  withCredentials: true,
  transformResponse: [(data, headers) => parseJsonPreserveLargeIntegers(data, headers)],
});

let refreshPromise: Promise<void> | null = null;

async function refreshAccessTokenViaSso(clientId: string) {
  for (let attempt = 0; attempt < 3; attempt += 1) {
    try {
      const body = new URLSearchParams();
      body.set('grant_type', 'refresh_token');
      body.set('client_id', clientId);
      const { data } = await refreshClient.post<Record<string, unknown>>(
        '/api/sso/oauth2/token',
        body.toString(),
        {
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'X-Device-Id': getOrCreateDeviceId(),
          },
          skipAuthRefresh: true,
        } as AxiosRequestConfig,
      );
      applyRefreshedAccessToken(normalizeOidcRefreshPayload(data));
      return;
    } catch (error) {
      const shouldRetry = attempt < 2 && isRetryableSsoRefreshConflict(error);
      if (!shouldRetry) {
        throw error;
      }
      await sleep(120 * (attempt + 1));
    }
  }
}

export async function refreshAccessToken() {
  if (refreshPromise) {
    return refreshPromise;
  }

  refreshPromise = getSsoRuntimeConfig()
    .then(async (runtimeConfig) => {
      if (!runtimeConfig.enabled || !runtimeConfig.frontendPrimaryEnabled) {
        throw new Error('登录服务未启用');
      }
      await refreshAccessTokenViaSso(runtimeConfig.defaultFirstPartyClientId || 'authorization-console');
    })
    .catch((error) => {
      clearSession();
      throw error;
    })
    .finally(() => {
      refreshPromise = null;
    });

  return refreshPromise;
}

apiClient.interceptors.request.use(async (config) => {
  const requestConfig = config as RetryRequestConfig;
  const requestUrl = requestConfig.url;
  const skipAuthRefresh = requestConfig.skipAuthRefresh || requestConfig._skipAuthRefresh;
  const authState = useAuthStore.getState();

  if (!skipAuthRefresh && !isRefreshRequest(requestUrl)) {
    const shouldRefresh =
      !!authState.accessToken && shouldRefreshAccessToken(authState.accessExpireAt);
    if (shouldRefresh) {
      await refreshAccessToken();
    }
  }

  if (!config.headers) {
    config.headers = {} as AxiosRequestHeaders;
  }

  const headerBag = config.headers as unknown as Record<string, string>;
  headerBag['X-Device-Id'] = getOrCreateDeviceId();

  const authHeader = getAuthorizationHeader();
  if (authHeader) {
    headerBag.Authorization = authHeader;
  }

  const requestMethod = (config.method || 'GET').toUpperCase();
  const isFormData = typeof FormData !== 'undefined' && config.data instanceof FormData;
  if (requestMethod !== 'GET' && !config.headers?.['Content-Type'] && !isFormData) {
    headerBag['Content-Type'] = 'application/json';
  }

  return config;
});

apiClient.interceptors.response.use(
  async (response: AxiosResponse) => {
    if (response.config.responseType === 'blob') {
      return response.data;
    }

    // Some proxy/runtime combinations hand Axios a JSON payload as text even
    // when the response is declared as JSON. Re-run the shared parser here so
    // callers never receive a serialized { code, data } envelope.
    const payload = parseJsonPreserveLargeIntegers(
      response.data,
      response.headers,
    ) as ApiResponse<unknown>;
    if (!payload || typeof payload !== 'object' || !('code' in payload)) {
      return payload;
    }

    const code = Number(payload.code);
    if (code === 0 || code === 200) {
      return payload;
    }

    if (code === 40100) {
      const originalConfig = (response.config || {}) as RetryRequestConfig;
      const skipAuthRefresh = originalConfig.skipAuthRefresh || originalConfig._skipAuthRefresh;
      const canRetry =
        !skipAuthRefresh &&
        !originalConfig._authRetry &&
        !isRefreshRequest(originalConfig.url) &&
        canAttemptAuthRefresh();

      if (canRetry) {
        originalConfig._authRetry = true;
        await refreshAccessToken();
        const authHeader = getAuthorizationHeader();
        if (authHeader) {
          const headers = (originalConfig.headers || {}) as Record<string, string>;
          headers.Authorization = authHeader;
          headers['X-Device-Id'] = getOrCreateDeviceId();
          originalConfig.headers = headers;
        }
        return apiClient.request(originalConfig);
      }

      if (!originalConfig.skipAuthRedirect) {
        traceAuthRedirect('business-40100', {
          url: originalConfig.url,
          status: response.status,
          code,
          message: payload.message,
        });
        clearSession();
        redirectToLogin();
      }
      throw createApiError(payload, '请先登录');
    }

    if (code === 40120) {
      const originalConfig = (response.config || {}) as RetryRequestConfig;
      const challengePayload = readChallengePayload(payload);
      if (!challengePayload) {
        throw createApiError(payload, '需要完成挑战验证');
      }

      if (originalConfig.skipGlobalChallenge || originalConfig._challengeRetry) {
        throw createApiError(payload, '需要完成挑战验证');
      }

      const fingerprint =
        originalConfig._challengeFingerprint ||
        buildRequestFingerprint({
          method: originalConfig.method,
          url: originalConfig.url,
          params: originalConfig.params,
          data: originalConfig.data,
        });

      try {
        return await resolveWithChallenge({
          fingerprint,
          payload: challengePayload,
          executeRetry: async (proofHeaders) => {
            const retryConfig: RetryRequestConfig = {
              ...originalConfig,
              _challengeRetry: true,
              _challengeFingerprint: fingerprint,
              headers: {
                ...((originalConfig.headers || {}) as Record<string, string>),
                ...buildProofHeaderBag(proofHeaders),
                'X-Device-Id': getOrCreateDeviceId(),
              },
            };
            return apiClient.request(retryConfig);
          },
        });
      } catch (challengeError) {
        if (isChallengeRetryError(challengeError, 'CHALLENGE_PRESENTER_UNAVAILABLE')) {
          throw createApiError(payload, '需要完成挑战验证');
        }
        throw challengeError;
      }
    }

    if (
      code === 40101 ||
      code === 40310 ||
      code === 40400 ||
      code === 40900 ||
      code === 40000 ||
      code === 50001
    ) {
      throw createApiError(payload, '请求失败');
    }

    throw createApiError(payload, '请求失败');
  },
  async (error: AxiosError) => {
    const responseStatus = error.response?.status;
    const originalConfig = ((error.config || {}) as RetryRequestConfig);
    const skipAuthRefresh = originalConfig.skipAuthRefresh || originalConfig._skipAuthRefresh;

    const canRetry =
      responseStatus === 401 &&
      !skipAuthRefresh &&
      !originalConfig._authRetry &&
      !isRefreshRequest(originalConfig.url) &&
      canAttemptAuthRefresh();

    if (canRetry) {
      originalConfig._authRetry = true;
      await refreshAccessToken();
      const authHeader = getAuthorizationHeader();
      if (authHeader) {
        const headers = (originalConfig.headers || {}) as Record<string, string>;
        headers.Authorization = authHeader;
        headers['X-Device-Id'] = getOrCreateDeviceId();
        originalConfig.headers = headers;
      }
      return apiClient.request(originalConfig);
    }

    if (responseStatus === 401) {
      if (!originalConfig.skipAuthRedirect) {
        traceAuthRedirect('http-401', {
          url: originalConfig.url,
          status: responseStatus,
          message: error.message,
        });
        clearSession();
        redirectToLogin();
      }
    }

    const payload = error.response?.data;
    if (payload && typeof payload === 'object') {
      return Promise.reject(createApiError(payload as ApiResponse<unknown>, error.message || '请求失败'));
    }
    return Promise.reject(error);
  },
);

/**
 * 统一的请求函数，替换 @umijs/max 的 request
 */
export async function request<T = unknown>(url: string, options: RequestOptions = {}): Promise<T> {
  const {
    method = 'GET',
    data,
    params,
    headers = {},
    responseType = 'json',
    skipGlobalChallenge = false,
    skipAuthRefresh = false,
    skipAuthRedirect = false,
    ...restOptions
  } = options;

  const response = await apiClient.request({
    url,
    method,
    data,
    params,
    headers,
    responseType,
    skipGlobalChallenge,
    skipAuthRefresh,
    skipAuthRedirect,
    ...restOptions,
  } as RetryRequestConfig);

  return response as T;
}
