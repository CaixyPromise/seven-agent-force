import { request } from '@/api/request';
import type {
  ApiResponse,
  ExternalLoginCallbackResult,
  ExternalLoginMethod,
} from '@/lib/http/types';

export type ExternalProviderStatus = 0 | 1;
export type ExternalIdentityStatus = 0 | 1 | 2;
export type ExternalOAuthTokenStatus = 0 | 1 | 2 | 3;

export interface EnterpriseIdentityRecord {
  id: string;
  userId: string;
  userDisplayName?: string;
  provider: 'FEISHU' | 'WECOM';
  displayHint: string;
  status: 'ACTIVE' | 'REVOKED' | 'DISABLED';
  identityVersion: string;
  directEnabled: boolean;
  mentionEnabled: boolean;
  loginEnabled: boolean;
  verifiedAt: string;
  revokedAt?: string;
}

export interface EnterpriseConnectionRecord {
  id: string;
  provider: 'FEISHU' | 'WECOM';
  displayName: string;
  appIdHint?: string;
  corpIdHint?: string;
  agentIdHint?: string;
  callbackUrl?: string;
  credentialConfigured: boolean;
  credentialVersion: string;
  bindingEnabled: boolean;
  notificationDirectEnabled: boolean;
  notificationMentionEnabled: boolean;
  loginEnabled: boolean;
  status: 'ACTIVE' | 'DISABLED';
  lastProbeStatus: string;
  lastProbeReasonCode?: string;
  lastProbeAt?: string;
  version: string;
}

export interface EnterpriseConnectionInput {
  provider: 'FEISHU' | 'WECOM';
  displayName: string;
  appId?: string;
  corpId?: string;
  agentId?: string;
  clientSecret?: string;
  bindingEnabled: boolean;
  notificationDirectEnabled: boolean;
  notificationMentionEnabled: boolean;
  loginEnabled: boolean;
  expectedVersion?: string;
}

export interface EnterpriseConnectionSetup {
  organizationCode: string;
  loginUrl?: string;
  providers: Array<{
    provider: 'FEISHU' | 'WECOM';
    callbackUrl?: string;
  }>;
}

export interface EnterpriseLoginOptions {
  organizationName: string;
  methods: Array<{ provider: 'FEISHU' | 'WECOM'; label: string }>;
}

export interface ExternalLoginProviderCapability {
  providerCode: string;
  displayName: string;
  protocolType: string;
  capabilities: string[];
  defaultScopes?: string[];
}

export type ExternalLoginCapabilities = Record<string, ExternalLoginProviderCapability>;

export interface ExternalLoginProviderQuery {
  keyword?: string;
  providerCode?: string;
  protocolType?: string;
  status?: number;
  current?: number;
  pageSize?: number;
}

export interface ExternalLoginProviderRecord {
  id?: string | number;
  providerCode: string;
  providerName: string;
  protocolType: string;
  issuer?: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  userinfoEndpoint?: string;
  jwksUri?: string;
  clientId: string;
  scopes: string[];
  redirectUri: string;
  displayName: string;
  icon?: string;
  sortOrder: number;
  displayEnabled: boolean;
  loginEnabled: boolean;
  bindEnabled: boolean;
  emailAutoBindEnabled: boolean;
  accountAutoCreateEnabled: boolean;
  status: ExternalProviderStatus;
  metadataJson?: string;
  methods?: ExternalLoginProviderMethod[];
  createTime?: string;
  updateTime?: string;
}

export interface ExternalLoginProviderMethod {
  providerCode: string;
  methodKey: string;
  displayName: string;
  icon?: string;
  sortOrder: number;
  enabled: boolean;
  metadataJson?: string;
}

export interface ExternalLoginProviderPage {
  records: ExternalLoginProviderRecord[];
  total: number;
  current: number;
  pageSize: number;
}

export interface ExternalLoginProviderCreateRequest {
  providerCode: string;
  providerName: string;
  protocolType: string;
  issuer?: string;
  authorizationEndpoint: string;
  tokenEndpoint: string;
  userinfoEndpoint?: string;
  jwksUri?: string;
  clientId: string;
  clientSecret?: string;
  scopes: string[];
  redirectUri: string;
  displayName: string;
  icon?: string;
  sortOrder: number;
  displayEnabled: boolean;
  loginEnabled: boolean;
  bindEnabled: boolean;
  emailAutoBindEnabled: boolean;
  accountAutoCreateEnabled: boolean;
  metadataJson?: string;
}

export type ExternalLoginProviderUpdateRequest = Omit<
  ExternalLoginProviderCreateRequest,
  'providerCode' | 'clientSecret'
>;

export interface ExternalLoginProviderStatusRequest {
  status: ExternalProviderStatus;
  reason: string;
  revokeActiveSessions?: boolean;
}

export interface ExternalLoginProviderSecretRotateRequest {
  clientSecret: string;
  reason: string;
}

export interface ExternalLoginProviderSecretRotateResponse {
  rotated?: boolean;
  secretHint?: string;
  clientSecret?: string;
}

export interface ExternalLoginIdentityQuery {
  providerCode?: string;
  userId?: string | number;
  status?: number;
  keyword?: string;
  current?: number;
  pageSize?: number;
}

export interface ExternalLoginIdentityRecord {
  id: string | number;
  providerCode: string;
  externalSubject: string;
  userId: string | number;
  externalLogin?: string;
  externalEmail?: string;
  emailVerified: boolean;
  displayName?: string;
  avatarUrl?: string;
  status: ExternalIdentityStatus;
  firstLinkedAt?: string;
  lastLoginAt?: string | null;
  lastVerifiedAt?: string | null;
  createTime?: string;
  updateTime?: string;
}

export interface ExternalLoginIdentityPage {
  records: ExternalLoginIdentityRecord[];
  total: number;
  current: number;
  pageSize: number;
}

export interface ExternalLoginIdentityStatusRequest {
  status: ExternalIdentityStatus;
  reason: string;
}

export interface CurrentExternalBinding {
  providerCode: string;
  displayName: string;
  icon?: string;
  bindEnabled: boolean;
  bound: boolean;
  identityId?: string | number;
  externalLogin?: string;
  externalEmail?: string;
  emailVerified: boolean;
  avatarUrl?: string;
  status?: ExternalIdentityStatus;
  lastLoginAt?: string | null;
  lastVerifiedAt?: string | null;
  bindUrl?: string;
  sortOrder: number;
}

export interface ExternalOAuthTokenQuery {
  providerCode?: string;
  identityId?: string | number;
  userId?: string | number;
  tokenPurpose?: string;
  status?: number;
  current?: number;
  pageSize?: number;
}

export interface ExternalOAuthTokenRecord {
  id: string | number;
  providerCode: string;
  identityId: string | number;
  userId: string | number;
  tokenPurpose: string;
  scopes: string[];
  scopeHash: string;
  accessExpiresAt?: string | null;
  refreshExpiresAt?: string | null;
  lastRefreshAt?: string | null;
  revokedAt?: string | null;
  status: ExternalOAuthTokenStatus;
  version: number;
  metadataJson?: string;
  createTime?: string;
  updateTime?: string;
}

export interface ExternalOAuthTokenPage {
  records: ExternalOAuthTokenRecord[];
  total: number;
  current: number;
  pageSize: number;
}

export interface ExternalOAuthTokenRevokeRequest {
  reason: string;
}

function unwrapApiData<T>(response: ApiResponse<T>, fallbackMessage: string): T {
  if (!response || typeof response !== 'object' || !('code' in response)) {
    return response as T;
  }
  if (response.code !== 0 && response.code !== 200) {
    throw new Error(response.message || fallbackMessage);
  }
  return response.data as T;
}

function unwrapExternalLoginMethodsResponse(
  response: ApiResponse<ExternalLoginMethod[]>,
): ExternalLoginMethod[] {
  return unwrapApiData(response, '外部登录接口返回失败') || [];
}

function normalizeArray<T>(value: T[] | null | undefined): T[];
function normalizeArray(value: unknown): unknown[];
function normalizeArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
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

function normalizeProviderStatus(value: unknown): ExternalProviderStatus {
  return normalizeNumber(value, 0) === 1 ? 1 : 0;
}

function normalizeIdentityStatus(value: unknown): ExternalIdentityStatus {
  const status = normalizeNumber(value, 0);
  return status === 1 || status === 2 ? status : 0;
}

function normalizeTokenStatus(value: unknown): ExternalOAuthTokenStatus {
  const status = normalizeNumber(value, 0);
  return status === 1 || status === 2 || status === 3 ? status : 0;
}

function normalizeTime(value: unknown): string | null | undefined {
  if (value === null) {
    return null;
  }
  if (value === undefined || value === '') {
    return undefined;
  }
  return String(value);
}

function normalizeProviderMethod(raw: unknown): ExternalLoginProviderMethod {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    providerCode: String(source.providerCode ?? ''),
    methodKey: String(source.methodKey ?? ''),
    displayName: String(source.displayName ?? ''),
    icon: source.icon ? String(source.icon) : undefined,
    sortOrder: normalizeNumber(source.sortOrder),
    enabled: normalizeBool(source.enabled),
    metadataJson: source.metadataJson ? String(source.metadataJson) : undefined,
  };
}

function normalizeProviderRecord(raw: unknown): ExternalLoginProviderRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    id: source.id as string | number | undefined,
    providerCode: String(source.providerCode ?? ''),
    providerName: String(source.providerName ?? ''),
    protocolType: String(source.protocolType ?? 'OAUTH2'),
    issuer: source.issuer ? String(source.issuer) : undefined,
    authorizationEndpoint: String(source.authorizationEndpoint ?? ''),
    tokenEndpoint: String(source.tokenEndpoint ?? ''),
    userinfoEndpoint: source.userinfoEndpoint ? String(source.userinfoEndpoint) : undefined,
    jwksUri: source.jwksUri ? String(source.jwksUri) : undefined,
    clientId: String(source.clientId ?? ''),
    scopes: normalizeStringArray(source.scopes),
    redirectUri: String(source.redirectUri ?? ''),
    displayName: String(source.displayName ?? ''),
    icon: source.icon ? String(source.icon) : undefined,
    sortOrder: normalizeNumber(source.sortOrder),
    displayEnabled: normalizeBool(source.displayEnabled),
    loginEnabled: normalizeBool(source.loginEnabled),
    bindEnabled: normalizeBool(source.bindEnabled),
    emailAutoBindEnabled: normalizeBool(source.emailAutoBindEnabled),
    accountAutoCreateEnabled: normalizeBool(source.accountAutoCreateEnabled),
    status: normalizeProviderStatus(source.status),
    metadataJson: source.metadataJson ? String(source.metadataJson) : undefined,
    methods: normalizeArray(source.methods).map(normalizeProviderMethod),
    createTime: normalizeTime(source.createTime) || undefined,
    updateTime: normalizeTime(source.updateTime) || undefined,
  };
}

function normalizeProviderPage(raw: unknown): ExternalLoginProviderPage {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    records: normalizeArray(source.records).map(normalizeProviderRecord),
    total: normalizeNumber(source.total),
    current: normalizeNumber(source.current, 1),
    pageSize: normalizeNumber(source.pageSize, 20),
  };
}

function normalizeCapabilities(raw: unknown): ExternalLoginCapabilities {
  const source = (raw || {}) as Record<string, unknown>;
  return Object.fromEntries(
    Object.entries(source).map(([key, value]) => {
      const item = (value || {}) as Record<string, unknown>;
      return [
        key,
        {
          providerCode: String(item.providerCode ?? key),
          displayName: String(item.displayName ?? key),
          protocolType: String(item.protocolType ?? ''),
          capabilities: normalizeStringArray(item.capabilities),
          defaultScopes: normalizeStringArray(item.defaultScopes),
        },
      ];
    }),
  );
}

function normalizeIdentityRecord(raw: unknown): ExternalLoginIdentityRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    id: (source.id ?? '') as string | number,
    providerCode: String(source.providerCode ?? ''),
    externalSubject: String(source.externalSubject ?? ''),
    userId: (source.userId ?? '') as string | number,
    externalLogin: source.externalLogin ? String(source.externalLogin) : undefined,
    externalEmail: source.externalEmail ? String(source.externalEmail) : undefined,
    emailVerified: normalizeBool(source.emailVerified),
    displayName: source.displayName ? String(source.displayName) : undefined,
    avatarUrl: source.avatarUrl ? String(source.avatarUrl) : undefined,
    status: normalizeIdentityStatus(source.status),
    firstLinkedAt: normalizeTime(source.firstLinkedAt) || undefined,
    lastLoginAt: normalizeTime(source.lastLoginAt),
    lastVerifiedAt: normalizeTime(source.lastVerifiedAt),
    createTime: normalizeTime(source.createTime) || undefined,
    updateTime: normalizeTime(source.updateTime) || undefined,
  };
}

function normalizeIdentityPage(raw: unknown): ExternalLoginIdentityPage {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    records: normalizeArray(source.records).map(normalizeIdentityRecord),
    total: normalizeNumber(source.total),
    current: normalizeNumber(source.current, 1),
    pageSize: normalizeNumber(source.pageSize, 20),
  };
}

function normalizeCurrentBinding(raw: unknown): CurrentExternalBinding {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    providerCode: String(source.providerCode ?? ''),
    displayName: String(source.displayName ?? ''),
    icon: source.icon ? String(source.icon) : undefined,
    bindEnabled: normalizeBool(source.bindEnabled),
    bound: normalizeBool(source.bound),
    identityId: source.identityId as string | number | undefined,
    externalLogin: source.externalLogin ? String(source.externalLogin) : undefined,
    externalEmail: source.externalEmail ? String(source.externalEmail) : undefined,
    emailVerified: normalizeBool(source.emailVerified),
    avatarUrl: source.avatarUrl ? String(source.avatarUrl) : undefined,
    status: source.status === undefined ? undefined : normalizeIdentityStatus(source.status),
    lastLoginAt: normalizeTime(source.lastLoginAt),
    lastVerifiedAt: normalizeTime(source.lastVerifiedAt),
    bindUrl: source.bindUrl ? String(source.bindUrl) : undefined,
    sortOrder: normalizeNumber(source.sortOrder),
  };
}

function normalizeTokenRecord(raw: unknown): ExternalOAuthTokenRecord {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    id: (source.id ?? '') as string | number,
    providerCode: String(source.providerCode ?? ''),
    identityId: (source.identityId ?? '') as string | number,
    userId: (source.userId ?? '') as string | number,
    tokenPurpose: String(source.tokenPurpose ?? ''),
    scopes: normalizeStringArray(source.scopes),
    scopeHash: String(source.scopeHash ?? ''),
    accessExpiresAt: normalizeTime(source.accessExpiresAt),
    refreshExpiresAt: normalizeTime(source.refreshExpiresAt),
    lastRefreshAt: normalizeTime(source.lastRefreshAt),
    revokedAt: normalizeTime(source.revokedAt),
    status: normalizeTokenStatus(source.status),
    version: normalizeNumber(source.version),
    metadataJson: source.metadataJson ? String(source.metadataJson) : undefined,
    createTime: normalizeTime(source.createTime) || undefined,
    updateTime: normalizeTime(source.updateTime) || undefined,
  };
}

function normalizeTokenPage(raw: unknown): ExternalOAuthTokenPage {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    records: normalizeArray(source.records).map(normalizeTokenRecord),
    total: normalizeNumber(source.total),
    current: normalizeNumber(source.current, 1),
    pageSize: normalizeNumber(source.pageSize, 20),
  };
}

export async function listExternalLoginMethods(loginTransactionId?: string) {
  const query = new URLSearchParams();
  const trimmedLoginTransactionId = loginTransactionId?.trim();
  if (trimmedLoginTransactionId) {
    query.set('loginTransactionId', trimmedLoginTransactionId);
  }
  const queryString = query.toString();
  const response = await request<ApiResponse<ExternalLoginMethod[]>>(
    `/api/login/external/providers${queryString ? `?${queryString}` : ''}`,
    {
      method: 'GET',
      skipAuthRefresh: true,
      skipAuthRedirect: true,
      skipGlobalChallenge: true,
    },
  );
  return unwrapExternalLoginMethodsResponse(response);
}

export async function completeExternalLoginCallback(
  providerCode: string,
  params: { code: string; state: string; issuer?: string | null },
): Promise<ExternalLoginCallbackResult> {
  const query = new URLSearchParams();
  query.set('code', params.code);
  query.set('state', params.state);
  if (params.issuer?.trim()) {
    query.set('issuer', params.issuer.trim());
  }
  const response = await request<ApiResponse<ExternalLoginCallbackResult>>(
    `/api/login/external/${encodeURIComponent(providerCode)}/callback?${query.toString()}`,
    {
      method: 'GET',
      skipAuthRefresh: true,
      skipAuthRedirect: true,
      skipGlobalChallenge: true,
    },
  );
  return unwrapApiData(response, '外部登录回调失败');
}

export async function getExternalLoginCapabilities(): Promise<ExternalLoginCapabilities> {
  const response = await request<ApiResponse<ExternalLoginCapabilities>>(
    '/api/external-login/admin/capabilities',
    {
      method: 'GET',
    },
  );
  return normalizeCapabilities(unwrapApiData(response, '外部登录能力接口返回失败'));
}

export async function listExternalLoginProviders(
  params: ExternalLoginProviderQuery,
): Promise<ExternalLoginProviderPage> {
  const response = await request<ApiResponse<ExternalLoginProviderPage>>(
    '/api/external-login/admin/providers',
    {
      method: 'GET',
      params,
    },
  );
  return normalizeProviderPage(unwrapApiData(response, '外部登录Provider列表接口返回失败'));
}

export async function getExternalLoginProvider(
  providerCode: string,
): Promise<ExternalLoginProviderRecord> {
  const response = await request<ApiResponse<ExternalLoginProviderRecord>>(
    `/api/external-login/admin/providers/${encodeURIComponent(providerCode)}`,
    {
      method: 'GET',
    },
  );
  return normalizeProviderRecord(unwrapApiData(response, '外部登录Provider详情接口返回失败'));
}

export async function createExternalLoginProvider(
  body: ExternalLoginProviderCreateRequest,
): Promise<ExternalLoginProviderRecord> {
  const response = await request<ApiResponse<ExternalLoginProviderRecord>>(
    '/api/external-login/admin/providers',
    {
      method: 'POST',
      data: body,
    },
  );
  return normalizeProviderRecord(unwrapApiData(response, '创建外部登录Provider失败'));
}

export async function updateExternalLoginProvider(
  providerCode: string,
  body: ExternalLoginProviderUpdateRequest,
): Promise<ExternalLoginProviderRecord> {
  const response = await request<ApiResponse<ExternalLoginProviderRecord>>(
    `/api/external-login/admin/providers/${encodeURIComponent(providerCode)}`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return normalizeProviderRecord(unwrapApiData(response, '更新外部登录Provider失败'));
}

export async function updateExternalLoginProviderStatus(
  providerCode: string,
  body: ExternalLoginProviderStatusRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<{ updated?: boolean }>>(
    `/api/external-login/admin/providers/${encodeURIComponent(providerCode)}/status`,
    {
      method: 'PUT',
      data: body,
    },
  );
  const result = unwrapApiData(response, '更新外部登录Provider状态失败');
  return result?.updated !== false;
}

export async function rotateExternalLoginProviderSecret(
  providerCode: string,
  body: ExternalLoginProviderSecretRotateRequest,
): Promise<ExternalLoginProviderSecretRotateResponse> {
  const response = await request<ApiResponse<ExternalLoginProviderSecretRotateResponse>>(
    `/api/external-login/admin/providers/${encodeURIComponent(providerCode)}/client-secret/rotate`,
    {
      method: 'POST',
      data: body,
    },
  );
  return unwrapApiData(response, '轮换外部登录Provider密钥失败') || {};
}

export async function listExternalLoginIdentities(
  params: ExternalLoginIdentityQuery,
): Promise<ExternalLoginIdentityPage> {
  const response = await request<ApiResponse<ExternalLoginIdentityPage>>(
    '/api/external-login/admin/identities',
    {
      method: 'GET',
      params,
    },
  );
  return normalizeIdentityPage(unwrapApiData(response, '外部登录身份绑定列表接口返回失败'));
}

export async function updateExternalLoginIdentityStatus(
  identityId: string | number,
  body: ExternalLoginIdentityStatusRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<{ updated?: boolean }>>(
    `/api/external-login/admin/identities/${encodeURIComponent(String(identityId))}/status`,
    {
      method: 'PUT',
      data: body,
    },
  );
  const result = unwrapApiData(response, '更新外部登录身份绑定状态失败');
  return result?.updated !== false;
}

export async function listCurrentExternalBindings(): Promise<CurrentExternalBinding[]> {
  const response = await request<ApiResponse<CurrentExternalBinding[]>>(
    '/api/external-login/me/bindings',
    { method: 'GET' },
  );
  return normalizeArray(unwrapApiData(response, '当前用户外部账号绑定列表接口返回失败')).map(
    normalizeCurrentBinding,
  );
}

export async function listExternalOAuthTokens(
  params: ExternalOAuthTokenQuery,
): Promise<ExternalOAuthTokenPage> {
  const response = await request<ApiResponse<ExternalOAuthTokenPage>>(
    '/api/external-login/admin/tokens',
    {
      method: 'GET',
      params,
    },
  );
  return normalizeTokenPage(unwrapApiData(response, '外部OAuth令牌列表接口返回失败'));
}

export async function revokeExternalOAuthToken(
  tokenId: string | number,
  body: ExternalOAuthTokenRevokeRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<{ revoked?: boolean }>>(
    `/api/external-login/admin/tokens/${encodeURIComponent(String(tokenId))}/revoke`,
    {
      method: 'POST',
      data: body,
    },
  );
  const result = unwrapApiData(response, '撤销外部OAuth令牌失败');
  return result?.revoked !== false;
}

export async function listMyEnterpriseIdentities(): Promise<EnterpriseIdentityRecord[]> {
  const response = await request<ApiResponse<EnterpriseIdentityRecord[]>>('/api/external-identities/me', { method: 'GET' });
  return normalizeArray(unwrapApiData(response, '企业账号列表加载失败'));
}

export async function startEnterpriseBinding(provider: 'FEISHU' | 'WECOM'): Promise<{ redirectUrl: string }> {
  const response = await request<ApiResponse<{ redirectUrl: string }>>(
    `/api/external-identities/me/${provider}/authorizations?redirectAfter=${encodeURIComponent('/account/settings')}`,
    { method: 'POST' },
  );
  return unwrapApiData(response, '无法开始企业账号绑定');
}

export async function startEnterpriseLogin(provider: 'FEISHU' | 'WECOM', organizationCode: string): Promise<{ redirectUrl: string }> {
  const response = await request<ApiResponse<{ redirectUrl: string }>>(
    `/api/login/enterprise/${provider}/authorizations`,
    { method: 'POST', data: { organizationCode, redirectAfter: '/' }, skipAuthRefresh: true, skipAuthRedirect: true, skipGlobalChallenge: true },
  );
  return unwrapApiData(response, '无法开始企业账号登录');
}

export async function getEnterpriseLoginOptions(organizationCode: string): Promise<EnterpriseLoginOptions> {
  const response = await request<ApiResponse<EnterpriseLoginOptions>>(
    `/api/login/enterprise/options?org=${encodeURIComponent(organizationCode)}`,
    { method: 'GET', skipAuthRefresh: true, skipAuthRedirect: true, skipGlobalChallenge: true },
  );
  return unwrapApiData(response, '组织登录入口不可用');
}

export async function completeEnterpriseBinding(provider: string, params: { code: string; state: string; error?: string }): Promise<{ outcome: string; purpose?: string; authenticated?: boolean; redirectAfter?: string }> {
  const query = new URLSearchParams({ code: params.code, state: params.state });
  if (params.error) query.set('error', params.error);
  const response = await request<ApiResponse<{ outcome: string; purpose?: string; authenticated?: boolean; redirectAfter?: string }>>(`/api/external-identities/oauth/callback/${encodeURIComponent(provider)}?${query.toString()}`, { method: 'GET', skipAuthRefresh: true, skipAuthRedirect: true, skipGlobalChallenge: true });
  return unwrapApiData(response, '企业账号绑定失败');
}

export async function updateEnterpriseNotificationSettings(identityId: string, body: { directEnabled: boolean; mentionEnabled: boolean; expectedIdentityVersion: string }): Promise<void> {
  await request<ApiResponse<{ updated: boolean }>>(`/api/external-identities/me/${encodeURIComponent(identityId)}/notification-settings`, { method: 'PUT', data: body });
}

export async function sendEnterpriseTestMessage(identityId: string): Promise<void> {
  await request<ApiResponse<{ sent: boolean }>>(`/api/external-identities/me/${encodeURIComponent(identityId)}/test-message`, { method: 'POST' });
}

export async function unbindEnterpriseIdentity(identityId: string, body: { expectedIdentityVersion: string; reason: string }): Promise<void> {
  await request<ApiResponse<{ unbound: boolean }>>(`/api/external-identities/me/${encodeURIComponent(identityId)}`, { method: 'DELETE', data: body });
}

export async function listEnterpriseConnections(): Promise<EnterpriseConnectionRecord[]> {
  const response = await request<ApiResponse<EnterpriseConnectionRecord[]>>('/api/system/enterprise-connections', { method: 'GET' });
  return normalizeArray(unwrapApiData(response, '企业连接列表加载失败'));
}

export async function getEnterpriseConnectionSetup(): Promise<EnterpriseConnectionSetup> {
  const response = await request<ApiResponse<EnterpriseConnectionSetup>>('/api/system/enterprise-connections/setup', { method: 'GET' });
  return unwrapApiData(response, '企业连接配置入口加载失败');
}

export async function listEnterpriseIdentities(): Promise<EnterpriseIdentityRecord[]> {
  const response = await request<ApiResponse<EnterpriseIdentityRecord[]>>('/api/system/enterprise-identities', { method: 'GET' });
  return normalizeArray(unwrapApiData(response, '已绑定成员加载失败'));
}

export async function saveEnterpriseConnection(body: EnterpriseConnectionInput): Promise<EnterpriseConnectionRecord> {
  const response = await request<ApiResponse<EnterpriseConnectionRecord>>('/api/system/enterprise-connections', { method: 'PUT', data: body });
  return unwrapApiData(response, '企业连接保存失败');
}

export async function setEnterpriseLoginAccess(identityId: string, body: { enabled: boolean; expectedIdentityVersion: string; reason: string }): Promise<void> {
  await request<ApiResponse<{ updated: boolean }>>(`/api/system/enterprise-identities/${encodeURIComponent(identityId)}/login-access`, { method: 'PUT', data: body });
}
