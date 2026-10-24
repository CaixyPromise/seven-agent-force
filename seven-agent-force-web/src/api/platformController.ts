import { request } from '@/api/request';
import type {
  ApiResponse,
  PlatformLoginMetadata,
  PlatformLoginMethod,
  PlatformLoginOptions,
} from '@/lib/http/types';

export const PLATFORM_LOGIN_OPTIONS_ENDPOINT = '/api/platform/public/login-options';
export const PLATFORM_LOGIN_OPTIONS_COMPAT_ENDPOINT = '/api/platform/login-options';
export const PLATFORM_ADMIN_ENDPOINT = '/api/platform/admin/platforms';

export type PlatformAdminStatus = 0 | 1;

export interface PlatformAdminQuery {
  keyword?: string;
  platformCode?: string;
  platformType?: string;
  status?: number;
  current?: number;
  pageSize?: number;
}

export interface PlatformAdminLoginMethod {
  methodType: PlatformLoginMethod['methodType'] | string;
  providerCode?: string;
  displayName: string;
  icon?: string | null;
  sortOrder: number;
  displayEnabled?: boolean;
  loginEnabled?: boolean;
  enabled: boolean;
  metadataJson?: string;
}

export interface PlatformAdminSourceRule {
  matchType?: string;
  matchValue?: string;
  priority?: number;
  status?: PlatformAdminStatus;
  metadataJson?: string;
}

export interface PlatformAdminDefaultRole {
  roleId?: API.Int64;
  roleCode?: string;
  roleName?: string;
  scene?: string;
  autoAssignEnabled?: boolean;
  enabled?: boolean;
}

export interface PlatformAdminRecord {
  id?: API.Int64;
  platformCode: string;
  platformName: string;
  platformType?: string;
  description?: string;
  defaultRedirectUrl?: string;
  allowAutoRegister?: boolean;
  allowFormRegister?: boolean;
  isDefault?: boolean;
  defaultDeptId?: API.Int64;
  brandJson?: string;
  settingsJson?: string;
  status: PlatformAdminStatus;
  loginMethods: PlatformAdminLoginMethod[];
  sourceRules: PlatformAdminSourceRule[];
  defaultRoles: PlatformAdminDefaultRole[];
  defaultRoleIds?: API.Int64[];
  createTime?: string;
  updateTime?: string;
}

export interface PlatformAdminPage {
  records: PlatformAdminRecord[];
  total: number;
  current: number;
  pageSize: number;
}

export interface PlatformAdminBaseRequest {
  platformName: string;
  platformType?: string;
  description?: string;
  defaultRedirectUrl?: string;
  allowAutoRegister?: boolean;
  allowFormRegister?: boolean;
  isDefault?: boolean;
  defaultDeptId?: API.Int64;
  brandJson?: string;
  settingsJson?: string;
  reason: string;
  stepUpProof?: string;
}

export interface PlatformAdminCreateRequest extends PlatformAdminBaseRequest {
  platformCode: string;
  status?: PlatformAdminStatus;
}

export type PlatformAdminUpdateRequest = PlatformAdminBaseRequest;

export interface PlatformAdminStatusRequest {
  status: PlatformAdminStatus;
  reason: string;
  stepUpProof?: string;
}

export interface PlatformAdminLoginMethodsRequest {
  methods: PlatformAdminLoginMethod[];
  reason: string;
  stepUpProof?: string;
}

export interface PlatformAdminSourceRulesRequest {
  rules: PlatformAdminSourceRule[];
  reason: string;
  stepUpProof?: string;
}

export interface PlatformAdminDefaultRolesRequest {
  roleIds?: API.Int64[];
  roles?: PlatformAdminDefaultRole[];
  reason: string;
  stepUpProof?: string;
}

export interface PlatformLoginOptionsQuery {
  redirect?: string;
  clientId?: string;
  loginTransactionId?: string;
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

function normalizeArray(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function normalizeNumber(value: unknown, fallback = 0): number {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
}

function normalizeInt64(value: unknown): API.Int64 | undefined {
  if (value === undefined || value === null) return undefined;
  const normalized = String(value);
  return /^\d+$/.test(normalized) && normalized !== '0' ? normalized : undefined;
}

function normalizeBool(value: unknown): boolean {
  return value === true || value === 1 || value === '1' || value === 'true';
}

function normalizeStatus(value: unknown): PlatformAdminStatus {
  return normalizeNumber(value, 0) === 1 ? 1 : 0;
}

function normalizeJsonText(value: unknown): string | undefined {
  if (value === undefined || value === null || value === '') {
    return undefined;
  }
  if (typeof value === 'string') {
    return value;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function normalizeAdminLoginMethod(raw: unknown): PlatformAdminLoginMethod {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    methodType: String(source.methodType ?? source.type ?? 'PASSWORD').toUpperCase(),
    providerCode: source.providerCode ? String(source.providerCode) : undefined,
    displayName: String(source.displayName ?? source.name ?? ''),
    icon: source.icon === null ? null : source.icon ? String(source.icon) : undefined,
    sortOrder: normalizeNumber(source.sortOrder),
    displayEnabled:
      source.displayEnabled === undefined ? undefined : normalizeBool(source.displayEnabled),
    loginEnabled: source.loginEnabled === undefined ? undefined : normalizeBool(source.loginEnabled),
    enabled:
      source.enabled === undefined
        ? source.displayEnabled === undefined && source.loginEnabled === undefined
          ? true
          : normalizeBool(source.displayEnabled) && normalizeBool(source.loginEnabled)
        : normalizeBool(source.enabled),
    metadataJson: normalizeJsonText(source.metadataJson ?? source.metadata),
  };
}

function normalizeSourceRule(raw: unknown): PlatformAdminSourceRule {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    matchType: source.matchType ? String(source.matchType) : undefined,
    matchValue: source.matchValue ? String(source.matchValue) : undefined,
    priority: source.priority === undefined ? undefined : normalizeNumber(source.priority),
    status: normalizeStatus(source.status),
    metadataJson: normalizeJsonText(source.metadataJson ?? source.metadata),
  };
}

function normalizeDefaultRole(raw: unknown): PlatformAdminDefaultRole {
  const source = (raw || {}) as Record<string, unknown>;
  return {
    roleId: normalizeInt64(source.roleId),
    roleCode: source.roleCode ? String(source.roleCode) : undefined,
    roleName: source.roleName ? String(source.roleName) : undefined,
    scene: source.scene ? String(source.scene) : undefined,
    autoAssignEnabled:
      source.autoAssignEnabled === undefined ? undefined : normalizeBool(source.autoAssignEnabled),
    enabled: source.enabled === undefined ? true : normalizeBool(source.enabled),
  };
}

function normalizeDefaultRoleIds(raw: unknown): API.Int64[] {
  return normalizeArray(raw)
    .map((item) => {
      if (typeof item === 'object' && item !== null) {
        return normalizeInt64((item as Record<string, unknown>).roleId);
      }
      return normalizeInt64(item);
    })
    .filter((item): item is API.Int64 => Boolean(item));
}

function normalizePlatformAdminRecord(raw: unknown): PlatformAdminRecord {
  const source = (raw || {}) as Record<string, unknown>;
  const base = (source.platformAdminRecord || source.PlatformAdminRecord || source) as Record<
    string,
    unknown
  >;
  const defaultRoles = normalizeArray(
    base.defaultRoles ?? source.defaultRoles ?? base.roles ?? source.roles,
  ).map(normalizeDefaultRole);
  return {
    id: normalizeInt64(base.id),
    platformCode: String(base.platformCode ?? base.code ?? ''),
    platformName: String(base.platformName ?? base.name ?? base.displayName ?? ''),
    platformType: base.platformType ? String(base.platformType) : undefined,
    description: base.description ? String(base.description) : undefined,
    defaultRedirectUrl: base.defaultRedirectUrl ? String(base.defaultRedirectUrl) : undefined,
    allowAutoRegister: normalizeBool(base.allowAutoRegister),
    allowFormRegister: normalizeBool(base.allowFormRegister),
    isDefault: normalizeBool(base.isDefault),
    defaultDeptId: normalizeInt64(base.defaultDeptId),
    brandJson: normalizeJsonText(base.brandJson ?? base.brand),
    settingsJson: normalizeJsonText(base.settingsJson ?? base.settings),
    status: normalizeStatus(base.status),
    loginMethods: normalizeArray(base.loginMethods ?? source.loginMethods ?? base.methods)
      .map(normalizeAdminLoginMethod),
    sourceRules: normalizeArray(base.sourceRules ?? source.sourceRules ?? base.rules)
      .map(normalizeSourceRule),
    defaultRoles,
    defaultRoleIds: normalizeDefaultRoleIds(base.defaultRoleIds ?? source.defaultRoleIds)
      .concat(normalizeDefaultRoleIds(defaultRoles)),
    createTime: base.createTime ? String(base.createTime) : undefined,
    updateTime: base.updateTime ? String(base.updateTime) : undefined,
  };
}

function serializeLoginMethods(methods: PlatformAdminLoginMethod[]) {
  return methods.map((item) => ({
    methodType: String(item.methodType || 'PASSWORD').trim().toUpperCase(),
    providerCode: item.providerCode?.trim() || undefined,
    displayName: item.displayName?.trim() || '',
    icon: item.icon?.trim() || undefined,
    sortOrder: normalizeNumber(item.sortOrder),
    displayEnabled: item.displayEnabled ?? item.enabled !== false,
    loginEnabled: item.loginEnabled ?? item.enabled !== false,
    metadataJson: item.metadataJson?.trim() || undefined,
  }));
}

function serializeSourceRules(rules: PlatformAdminSourceRule[]) {
  return rules.map((item) => ({
    matchType: item.matchType?.trim().toUpperCase(),
    matchValue: item.matchValue?.trim(),
    priority: normalizeNumber(item.priority),
    status: item.status ?? 0,
    metadataJson: item.metadataJson?.trim() || undefined,
  }));
}

function serializeDefaultRoles(body: PlatformAdminDefaultRolesRequest) {
  const roles = [
    ...(body.roles || []),
    ...(body.roleIds || []).map((roleId) => ({ roleId, autoAssignEnabled: true })),
  ];
  return roles
    .map((item) => ({
      roleId: normalizeInt64(item.roleId),
      autoAssignEnabled: true,
    }))
    .filter((item): item is { roleId: API.Int64; autoAssignEnabled: boolean } => Boolean(item.roleId));
}

function extractMutationAfter(raw: unknown): Record<string, unknown> | null {
  if (!raw || typeof raw !== 'object') {
    return null;
  }
  const source = raw as Record<string, unknown>;
  if (source.after && typeof source.after === 'object') {
    return source.after as Record<string, unknown>;
  }
  return source;
}

function normalizePlatformAdminPage(raw: unknown): PlatformAdminPage {
  const source = (raw || {}) as Record<string, unknown>;
  const records = normalizeArray(source.records ?? source.list ?? source.platforms);
  return {
    records: records.map(normalizePlatformAdminRecord),
    total: normalizeNumber(source.total ?? records.length),
    current: normalizeNumber(source.current ?? source.pageNum, 1),
    pageSize: normalizeNumber(source.pageSize, 20),
  };
}

function normalizeLoginMethod(raw: unknown): PlatformLoginMethod | null {
  const source = (raw || {}) as Record<string, unknown>;
  const methodType = String(source.methodType || '').toUpperCase();
  if (methodType !== 'PASSWORD' && methodType !== 'PASSKEY' && methodType !== 'EXTERNAL_OAUTH') {
    return null;
  }
  return {
    methodType,
    providerCode: String(source.providerCode ?? ''),
    displayName: String(source.displayName ?? ''),
    icon: source.icon ? String(source.icon) : undefined,
    sortOrder: normalizeNumber(source.sortOrder),
    loginUrl: source.loginUrl ? String(source.loginUrl) : undefined,
  };
}

function normalizeMetadata(raw: unknown): PlatformLoginMetadata | undefined {
  if (!raw || typeof raw !== 'object') {
    return undefined;
  }
  const source = raw as Record<string, unknown>;
  const metadata: PlatformLoginMetadata = {};
  if (source.platformCode) {
    metadata.platformCode = String(source.platformCode);
  }
  if (source.platformType) {
    metadata.platformType = String(source.platformType);
  }
  if (source.displayName) {
    metadata.displayName = String(source.displayName);
  }
  if (source.supportUrl) {
    metadata.supportUrl = String(source.supportUrl);
  }
  return Object.keys(metadata).length > 0 ? metadata : undefined;
}

function normalizeLoginOptions(raw: unknown): PlatformLoginOptions {
  const source = (raw || {}) as Record<string, unknown>;
  const brand = ((source.brand || {}) as Record<string, unknown>);
  const methods = Array.isArray(source.methods) ? source.methods : [];
  const registration = ((source.registration || {}) as Record<string, unknown>);
  return {
    loginContextId: String(source.loginContextId ?? ''),
    platformName: String(source.platformName ?? ''),
    brand: {
      title: String(brand.title ?? source.platformName ?? 'Seven'),
      subtitle: String(brand.subtitle ?? '统一身份认证系统'),
      theme: brand.theme ? String(brand.theme) : undefined,
    },
    registration: {
      formRegisterEnabled: normalizeBool(registration.formRegisterEnabled),
      requireCaptcha: registration.requireCaptcha === undefined
        ? true
        : normalizeBool(registration.requireCaptcha),
      requiredFields: normalizeArray(registration.requiredFields).map((item) => String(item)),
    },
    metadata: normalizeMetadata(source.metadata),
    methods: methods
      .map(normalizeLoginMethod)
      .filter((method): method is PlatformLoginMethod => Boolean(method))
      .sort((first, second) => first.sortOrder - second.sortOrder),
  };
}

function buildLoginOptionsParams(query: PlatformLoginOptionsQuery) {
  return {
    redirect: query.redirect?.trim() || undefined,
    clientId: query.clientId?.trim() || undefined,
    loginTransactionId: query.loginTransactionId?.trim() || undefined,
  };
}

async function requestLoginOptions(
  endpoint: string,
  query: PlatformLoginOptionsQuery,
): Promise<PlatformLoginOptions> {
  const response = await request<ApiResponse<PlatformLoginOptions>>(endpoint, {
    method: 'GET',
    params: buildLoginOptionsParams(query),
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
  return normalizeLoginOptions(unwrapApiData(response, '登录方式接口返回失败'));
}

export async function getPlatformLoginOptions(query: PlatformLoginOptionsQuery = {}) {
  try {
    return await requestLoginOptions(PLATFORM_LOGIN_OPTIONS_ENDPOINT, query);
  } catch (primaryError) {
    try {
      return await requestLoginOptions(PLATFORM_LOGIN_OPTIONS_COMPAT_ENDPOINT, query);
    } catch {
      throw primaryError;
    }
  }
}

export async function listPlatformAdmins(params: PlatformAdminQuery): Promise<PlatformAdminPage> {
  const response = await request<ApiResponse<PlatformAdminPage>>(PLATFORM_ADMIN_ENDPOINT, {
    method: 'GET',
    params,
  });
  return normalizePlatformAdminPage(unwrapApiData(response, '平台列表接口返回失败'));
}

export async function getPlatformAdmin(platformCode: string): Promise<PlatformAdminRecord> {
  const response = await request<ApiResponse<PlatformAdminRecord>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}`,
    { method: 'GET' },
  );
  return normalizePlatformAdminRecord(unwrapApiData(response, '平台详情接口返回失败'));
}

export async function createPlatformAdmin(
  body: PlatformAdminCreateRequest,
): Promise<PlatformAdminRecord> {
  const response = await request<ApiResponse<PlatformAdminRecord>>(PLATFORM_ADMIN_ENDPOINT, {
    method: 'POST',
    data: body,
  });
  return normalizePlatformAdminRecord(unwrapApiData(response, '创建平台失败'));
}

export async function updatePlatformAdmin(
  platformCode: string,
  body: PlatformAdminUpdateRequest,
): Promise<PlatformAdminRecord> {
  const response = await request<ApiResponse<PlatformAdminRecord>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}`,
    {
      method: 'PUT',
      data: body,
    },
  );
  return normalizePlatformAdminRecord(unwrapApiData(response, '更新平台失败'));
}

export async function updatePlatformAdminStatus(
  platformCode: string,
  body: PlatformAdminStatusRequest,
): Promise<boolean> {
  const response = await request<ApiResponse<boolean | { updated?: boolean }>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}/status`,
    {
      method: 'PUT',
      data: body,
    },
  );
  const result = unwrapApiData(response, '更新平台状态失败');
  return typeof result === 'boolean' ? result : result?.updated !== false;
}

export async function updatePlatformAdminLoginMethods(
  platformCode: string,
  body: PlatformAdminLoginMethodsRequest,
): Promise<PlatformAdminLoginMethod[]> {
  const response = await request<ApiResponse<PlatformAdminLoginMethod[]>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}/login-methods`,
    {
      method: 'PUT',
      data: { ...body, methods: serializeLoginMethods(body.methods) },
    },
  );
  const result = unwrapApiData(response, '保存登录方式失败');
  const after = extractMutationAfter(result);
  return normalizeArray(after?.loginMethods ?? result).map(normalizeAdminLoginMethod);
}

export async function updatePlatformAdminSourceRules(
  platformCode: string,
  body: PlatformAdminSourceRulesRequest,
): Promise<PlatformAdminSourceRule[]> {
  const response = await request<ApiResponse<PlatformAdminSourceRule[]>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}/source-rules`,
    {
      method: 'PUT',
      data: { ...body, rules: serializeSourceRules(body.rules) },
    },
  );
  const result = unwrapApiData(response, '保存来源规则失败');
  const after = extractMutationAfter(result);
  return normalizeArray(after?.sourceRules ?? result).map(normalizeSourceRule);
}

export async function updatePlatformAdminDefaultRoles(
  platformCode: string,
  body: PlatformAdminDefaultRolesRequest,
): Promise<PlatformAdminDefaultRole[]> {
  const response = await request<ApiResponse<PlatformAdminDefaultRole[]>>(
    `${PLATFORM_ADMIN_ENDPOINT}/${encodeURIComponent(platformCode)}/default-roles`,
    {
      method: 'PUT',
      data: { reason: body.reason, roles: serializeDefaultRoles(body) },
    },
  );
  const result = unwrapApiData(response, '保存默认角色失败');
  const after = extractMutationAfter(result);
  return normalizeArray(after?.defaultRoles ?? result).map(normalizeDefaultRole);
}
