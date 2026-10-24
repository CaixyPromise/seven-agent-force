import { request } from '@/api/request';
import type { ApiResponse, SsoRuntimeConfig } from '@/lib/http/types';
import { buildDeviceHeaders } from '@/lib/auth/oidc';
import { normalizeOidcTokenResponse } from '@/services/auth';

export interface AuthorizeRequest {
  clientId: string;
  redirectUri: string;
  scope: string;
  state: string;
  nonce: string;
  codeChallenge: string;
  codeChallengeMethod?: string;
  prompt?: string;
}

export interface AuthorizeResult {
  redirectRequired?: boolean;
  redirectUrl?: string;
}

export interface AuthorizeLoginResult {
  loginTransactionId?: string;
  expiresIn?: number;
  redirectUrl?: string | null;
}

export type SsoClientType = 'PUBLIC' | 'CONFIDENTIAL';
export type SsoClientAuthMethod = 'none' | 'client_secret_basic';
export type SsoClientStatus = 0 | 1;

export interface SsoScopeCapability {
  name: string;
  required?: boolean;
}

export interface SsoClientCapabilities {
  clientTypes: SsoClientType[];
  clientAuthMethods: SsoClientAuthMethod[];
  grantTypes: string[];
  scopes: SsoScopeCapability[];
  codeChallengeMethods: string[];
  signingAlgorithms: string[];
}

export interface SsoClientQuery {
  keyword?: string;
  status?: number;
  clientType?: string;
  current?: number;
  pageSize?: number;
}

export interface SsoClientRecord {
  id?: string | number;
  clientId: string;
  clientName: string;
  clientType: SsoClientType;
  clientAuthMethod: SsoClientAuthMethod;
  grantTypes: string[];
  scopes: string[];
  requirePkce: boolean;
  requireConsent: boolean;
  trustedFirstParty: boolean;
  accessTokenTtlSec: number;
  refreshTokenTtlSec: number;
  status: SsoClientStatus;
  metadataJson?: string;
  activeRedirectUriCount: number;
  activeSecretCount: number;
  createTime: string;
  updateTime: string;
}

export interface SsoClientPage {
  records: SsoClientRecord[];
  total: number;
  current: number;
  pageSize: number;
}

export interface SsoRedirectUriRecord {
  id?: string | number;
  clientId: string;
  redirectUri?: string;
  postLogoutRedirectUri?: string;
  status: SsoClientStatus;
  createTime: string;
  updateTime: string;
}

export interface SsoClientSecretRecord {
  secretId: string | number;
  secretHint?: string;
  status: SsoClientStatus;
  expiresAt?: string | null;
  createTime: string;
}

export interface SsoClientDetail extends SsoClientRecord {
  redirectUris?: SsoRedirectUriRecord[];
  secrets?: SsoClientSecretRecord[];
}

export interface SsoClientCreateRequest {
  clientId: string;
  clientName: string;
  clientType: SsoClientType;
  clientAuthMethod: SsoClientAuthMethod;
  grantTypes: string[];
  scopes: string[];
  requirePkce: boolean;
  requireConsent: boolean;
  trustedFirstParty: boolean;
  accessTokenTtlSec: number;
  refreshTokenTtlSec: number;
  metadataJson?: string;
}

export type SsoClientUpdateRequest = Omit<SsoClientCreateRequest, 'clientId'>;

export interface SsoClientStatusRequest {
  status: SsoClientStatus;
  reason?: string;
  revokeActiveSessions?: boolean;
}

export interface SsoRedirectUriUpdateRequest {
  redirectUris: string[];
  postLogoutRedirectUris?: string[];
}

export interface SsoClientSecretGenerateRequest {
  expiresInDays?: number;
  reason?: string;
}

export interface SsoClientSecretGenerateResponse {
  secretId: string | number;
  clientSecret: string;
  secretHint?: string;
  expiresAt?: string | null;
}

export interface SsoClientSecretStatusRequest {
  status: SsoClientStatus;
  reason?: string;
  allowNoActiveSecret?: boolean;
}

function readProtocolError(payload: unknown, fallback: string) {
  const source = payload as { error_description?: string; message?: string } | undefined;
  return source?.error_description || source?.message || fallback;
}

function unwrapApiData<T>(response: ApiResponse<T>): T {
  if (!response || typeof response !== 'object' || !('code' in response)) {
    return response as T;
  }
  if (response.code !== 0 && response.code !== 200) {
    throw new Error(response.message || 'SSO 客户端接口返回失败');
  }
  return response.data as T;
}

function normalizeStringArray(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).filter(Boolean);
  }
  if (typeof value !== 'string' || !value.trim()) {
    return [];
  }
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed.map((item) => String(item)).filter(Boolean) : [];
  } catch {
    return value
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean);
  }
}

function normalizeNumber(value: unknown, fallback = 0): number {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
}

function normalizeBool(value: unknown): boolean {
  return value === true || value === 1 || value === '1' || value === 'true';
}

function normalizeStatus(value: unknown): SsoClientStatus {
  return normalizeNumber(value, 1) === 0 ? 0 : 1;
}

function normalizeSsoClientRecord(raw: unknown): SsoClientRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    id: source.id as string | number | undefined,
    clientId: String(source.clientId ?? source.clientID ?? ''),
    clientName: String(source.clientName ?? ''),
    clientType: (String(source.clientType ?? 'PUBLIC') as SsoClientType),
    clientAuthMethod: (String(source.clientAuthMethod ?? 'none') as SsoClientAuthMethod),
    grantTypes: normalizeStringArray(source.grantTypes),
    scopes: normalizeStringArray(source.scopes),
    requirePkce: normalizeBool(source.requirePkce ?? source.requirePKCE),
    requireConsent: normalizeBool(source.requireConsent),
    trustedFirstParty: normalizeBool(source.trustedFirstParty),
    accessTokenTtlSec: normalizeNumber(source.accessTokenTtlSec),
    refreshTokenTtlSec: normalizeNumber(source.refreshTokenTtlSec),
    status: normalizeStatus(source.status),
    metadataJson: source.metadataJson ? String(source.metadataJson) : undefined,
    activeRedirectUriCount: normalizeNumber(source.activeRedirectUriCount),
    activeSecretCount: normalizeNumber(source.activeSecretCount),
    createTime: String(source.createTime ?? ''),
    updateTime: String(source.updateTime ?? ''),
  };
}

function normalizeSsoRedirectUriRecord(raw: unknown): SsoRedirectUriRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    id: source.id as string | number | undefined,
    clientId: String(source.clientId ?? ''),
    redirectUri: source.redirectUri ? String(source.redirectUri) : undefined,
    postLogoutRedirectUri: source.postLogoutRedirectUri
      ? String(source.postLogoutRedirectUri)
      : undefined,
    status: normalizeStatus(source.status),
    createTime: String(source.createTime ?? ''),
    updateTime: String(source.updateTime ?? ''),
  };
}

function normalizeSsoClientSecretRecord(raw: unknown): SsoClientSecretRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    secretId: (source.secretId ?? source.id ?? '') as string | number,
    secretHint: source.secretHint ? String(source.secretHint) : undefined,
    status: normalizeStatus(source.status),
    expiresAt: source.expiresAt === null || source.expiresAt === undefined
      ? null
      : String(source.expiresAt),
    createTime: String(source.createTime ?? ''),
  };
}

function normalizeSsoClientDetail(raw: unknown): SsoClientDetail {
  const source = (raw || {}) as Record<string, unknown>;
  const base = (source.clientAdminRecord || source.ClientAdminRecord || source) as Record<string, unknown>;
  return {
    ...normalizeSsoClientRecord(base),
    redirectUris: normalizeArray(source.redirectUris).map(normalizeSsoRedirectUriRecord),
    secrets: normalizeArray(source.secrets).map(normalizeSsoClientSecretRecord),
  };
}

function normalizeArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function normalizeSsoClientPage(raw: unknown): SsoClientPage {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    records: normalizeArray(source.records).map(normalizeSsoClientRecord),
    total: normalizeNumber(source.total),
    current: normalizeNumber(source.current, 1),
    pageSize: normalizeNumber(source.pageSize, 20),
  };
}

export async function authorizeSso(params: AuthorizeRequest): Promise<AuthorizeResult> {
  const url = new URL('/api/sso/oauth2/authorize', window.location.origin);
  url.searchParams.set('client_id', params.clientId);
  url.searchParams.set('response_type', 'code');
  url.searchParams.set('redirect_uri', params.redirectUri);
  url.searchParams.set('scope', params.scope);
  url.searchParams.set('state', params.state);
  url.searchParams.set('nonce', params.nonce);
  url.searchParams.set('code_challenge', params.codeChallenge);
  url.searchParams.set('code_challenge_method', params.codeChallengeMethod || 'S256');
  if (params.prompt) {
    url.searchParams.set('prompt', params.prompt);
  }

  const response = await fetch(url.toString(), {
    method: 'GET',
    credentials: 'include',
    redirect: 'manual',
    headers: {
      Accept: 'application/json',
      ...buildDeviceHeaders(),
    },
  });

  if (response.status === 302 || response.type === 'opaqueredirect') {
    const redirectUrl = response.headers.get('Location') || response.url;
    if (!redirectUrl) {
      throw new Error('SSO authorize 未返回有效回跳地址');
    }
    return {
      redirectRequired: true,
      redirectUrl,
    };
  }
  const payload = (await response.json().catch(() => ({}))) as Record<string, unknown>;
  throw new Error(readProtocolError(payload, `SSO authorize 失败: ${response.status}`));
}

export async function authorizeSsoLogin(params: AuthorizeRequest) {
  return request<ApiResponse<AuthorizeLoginResult>>('/api/sso/oauth2/authorize/login', {
    method: 'GET',
    params: {
      client_id: params.clientId,
      response_type: 'code',
      redirect_uri: params.redirectUri,
      scope: params.scope,
      state: params.state,
      nonce: params.nonce,
      code_challenge: params.codeChallenge,
      code_challenge_method: params.codeChallengeMethod || 'S256',
      prompt: params.prompt,
    },
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function exchangeOidcToken(body: Record<string, string>) {
  const form = new URLSearchParams();
  Object.entries(body).forEach(([key, value]) => {
    if (value) {
      form.set(key, value);
    }
  });
  const response = await fetch('/api/sso/oauth2/token', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      Accept: 'application/json',
      ...buildDeviceHeaders(),
    },
    body: form.toString(),
  });
  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(readProtocolError(payload, `SSO token 失败: ${response.status}`));
  }
  return normalizeOidcTokenResponse(payload);
}

export async function getSsoRuntimeConfigApi() {
  return request<ApiResponse<SsoRuntimeConfig>>('/api/sso/runtime/config', {
    method: 'GET',
    skipAuthRefresh: true,
    skipAuthRedirect: true,
  });
}

export async function revokeOidcToken(token: string, tokenTypeHint?: 'access_token' | 'refresh_token') {
  const form = new URLSearchParams();
  form.set('token', token);
  if (tokenTypeHint) {
    form.set('token_type_hint', tokenTypeHint);
  }
  const response = await fetch('/api/sso/oauth2/revoke', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
      Accept: 'application/json',
      ...buildDeviceHeaders(),
    },
    body: form.toString(),
  });
  if (!response.ok) {
    const payload = await response.json().catch(() => ({}));
    throw new Error(readProtocolError(payload, `SSO revoke 失败: ${response.status}`));
  }
}

export async function ssoLogout(accessToken?: string | null) {
  return request<ApiResponse<boolean>>('/api/sso/logout', {
    method: 'POST',
    headers: accessToken
      ? {
          Authorization: `Bearer ${accessToken}`,
        }
      : undefined,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function getCurrentUserSsoSessions(options?: Record<string, unknown>) {
  return request<API.ResultListUserSessionVO>('/api/sso/sessions', {
    method: 'GET',
    ...(options || {}),
  });
}

export async function revokeCurrentUserSsoSession(
  params: API.revokeCurrentUserSessionParams,
  options?: Record<string, unknown>,
) {
  const { sessionId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/sso/sessions/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function getCurrentUserSsoDevices(options?: Record<string, unknown>) {
  return request<API.ResultListUserDeviceVO>('/api/sso/devices', {
    method: 'GET',
    ...(options || {}),
  });
}

export async function revokeCurrentUserSsoDevice(
  params: API.revokeCurrentUserDeviceParams,
  options?: Record<string, unknown>,
) {
  const { deviceId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/sso/devices/${param0}`, {
    method: 'DELETE',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function logoutAllSsoSessions(options?: Record<string, unknown>) {
  return request<API.ResultBoolean>('/api/sso/logout-all', {
    method: 'POST',
    ...(options || {}),
  });
}

export async function listAdminUserSsoSessions(
  params: API.listAdminUserSessionsParams,
  options?: Record<string, unknown>,
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultListUserSessionVO>(`/api/sso/admin/users/${param0}/sessions`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function kickAdminUserSsoSession(
  params: API.kickAdminUserSessionParams,
  options?: Record<string, unknown>,
) {
  const { userId: param0, sessionId: param1, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/sso/admin/users/${param0}/sessions/${param1}/kick`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function kickAdminUserAllSsoSessions(
  params: API.kickAdminUserAllSessionsParams,
  options?: Record<string, unknown>,
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/sso/admin/users/${param0}/logout-all`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function listAdminUserSsoDevices(
  params: API.listAdminUserDevicesParams,
  options?: Record<string, unknown>,
) {
  const { userId: param0, ...queryParams } = params;
  return request<API.ResultListUserDeviceVO>(`/api/sso/admin/users/${param0}/devices`, {
    method: 'GET',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function kickAdminUserSsoDevice(
  params: API.kickAdminUserDeviceParams,
  options?: Record<string, unknown>,
) {
  const { userId: param0, deviceId: param1, ...queryParams } = params;
  return request<API.ResultBoolean>(`/api/sso/admin/users/${param0}/devices/${param1}/kick`, {
    method: 'POST',
    params: { ...queryParams },
    ...(options || {}),
  });
}

export async function getSsoClientCapabilities(): Promise<SsoClientCapabilities> {
  const response = await request<ApiResponse<SsoClientCapabilities>>(
    '/api/sso/admin/client-capabilities',
    {
      method: 'GET',
    },
  );
  return unwrapApiData(response);
}

export async function listSsoClients(params: SsoClientQuery): Promise<SsoClientPage> {
  const response = await request<ApiResponse<SsoClientPage>>('/api/sso/admin/clients', {
    method: 'GET',
    params,
  });
  return normalizeSsoClientPage(unwrapApiData(response));
}

export async function getSsoClient(clientId: string): Promise<SsoClientDetail> {
  const response = await request<ApiResponse<SsoClientDetail>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}`,
    {
      method: 'GET',
    },
  );
  return normalizeSsoClientDetail(unwrapApiData(response));
}

export async function createSsoClient(
  body: SsoClientCreateRequest,
): Promise<SsoClientDetail> {
  const response = await request<ApiResponse<SsoClientDetail>>('/api/sso/admin/clients', {
    method: 'POST',
    data: body,
  });
  return normalizeSsoClientDetail(unwrapApiData(response));
}

export async function updateSsoClient(
  clientId: string,
  body: SsoClientUpdateRequest,
): Promise<SsoClientDetail> {
  const response = await request<ApiResponse<SsoClientDetail>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return normalizeSsoClientDetail(unwrapApiData(response));
}

export async function updateSsoClientStatus(
  clientId: string,
  body: SsoClientStatusRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<boolean>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/status`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return unwrapApiData(response);
}

export async function listSsoClientRedirectUris(
  clientId: string,
): Promise<SsoRedirectUriRecord[]> {
  const response = await request<ApiResponse<SsoRedirectUriRecord[]>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/redirect-uris`,
    {
      method: 'GET',
    },
  );
  return normalizeArray(unwrapApiData(response)).map(normalizeSsoRedirectUriRecord);
}

export async function updateSsoClientRedirectUris(
  clientId: string,
  body: SsoRedirectUriUpdateRequest,
): Promise<SsoRedirectUriRecord[]> {
  const response = await request<ApiResponse<SsoRedirectUriRecord[]>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/redirect-uris`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return normalizeArray(unwrapApiData(response)).map(normalizeSsoRedirectUriRecord);
}

export async function listSsoClientSecrets(clientId: string): Promise<SsoClientSecretRecord[]> {
  const response = await request<ApiResponse<SsoClientSecretRecord[]>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/secrets`,
    {
      method: 'GET',
    },
  );
  return normalizeArray(unwrapApiData(response)).map(normalizeSsoClientSecretRecord);
}

export async function generateSsoClientSecret(
  clientId: string,
  body: SsoClientSecretGenerateRequest,
): Promise<SsoClientSecretGenerateResponse> {
  const response = await request<ApiResponse<SsoClientSecretGenerateResponse>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/secrets`,
    {
      method: 'POST',
      data: body,
    },
  );
  return unwrapApiData(response);
}

export async function disableSsoClientSecret(
  clientId: string,
  secretId: string | number,
  body: SsoClientSecretStatusRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<boolean>>(
    `/api/sso/admin/clients/${encodeURIComponent(clientId)}/secrets/${encodeURIComponent(String(secretId))}/status`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return unwrapApiData(response);
}
