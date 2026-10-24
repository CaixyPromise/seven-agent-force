export type UnknownState = 'UNKNOWN';
export type HubNodeStatusValue = 0 | 1;
export type HubNodeStatus = HubNodeStatusValue | UnknownState;
export type HubUserStatusValue = 0 | 1 | 2;
export type HubUserStatus = HubUserStatusValue | UnknownState;
export type HubSessionStatus = 'ACTIVE' | 'EXPIRED' | 'REVOKED' | UnknownState;
export type HubConnectionStatus = 'PENDING' | 'ACTIVE' | 'ERROR' | UnknownState;
export type HubDiscoveryTypeValue = 'STATIC' | 'CONSUL';
export type HubDiscoveryType = HubDiscoveryTypeValue | UnknownState;
export type ManagedLoginMethodType = 'PASSWORD' | 'PASSKEY' | 'EXTERNAL_OAUTH';

export const MANAGED_LOGIN_METHOD_TYPES: ManagedLoginMethodType[] = [
  'PASSWORD',
  'PASSKEY',
  'EXTERNAL_OAUTH',
];

export interface HubApiResponse<T> {
  code: number;
  message?: string;
  data: T;
}

export interface HubNodeRecord {
  nodeCode: string;
  nodeName: string;
  status: HubNodeStatus;
  discoveryType: HubDiscoveryType;
  serviceName?: string;
  managementBaseUrl?: string;
  hubIssuer: string;
  oidcClientId?: string;
  capabilities: string[];
  connectionStatus: HubConnectionStatus;
  connectionVersion?: string;
  issuerLockedAt?: string;
  lastConnectionError?: string;
  lastConnectionTraceId?: string;
  lastHealthyAt?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface HubNodePage {
  current: number;
  size: number;
  total: number;
  records: HubNodeRecord[];
}

export interface HubNodeHealth {
  nodeCode: string;
  version: string;
  capabilities: string[];
  health: string;
  traceId?: string;
}

export interface HubUserRecord {
  userId: string;
  username: string;
  nickname: string;
  emailMasked?: string;
  phoneMasked?: string;
  status: HubUserStatus;
  createdAt?: string;
  updatedAt?: string;
}

export interface HubUserPage {
  current: number;
  size: number;
  total: number;
  records: HubUserRecord[];
}

export interface HubSessionRecord {
  sessionRef: string;
  clientId: string;
  loginMethod?: string;
  loginAt?: string;
  lastAccessAt?: string;
  expiresAt?: string;
  status: HubSessionStatus;
}

export interface HubSessionPage {
  current: number;
  size: number;
  total: number;
  records: HubSessionRecord[];
}

export interface HubLoginMethod {
  methodType: ManagedLoginMethodType | UnknownState;
  providerCode?: string;
  displayName: string;
  icon?: string;
  sortOrder: number;
  displayEnabled: boolean;
  loginEnabled: boolean;
}

export interface HubSourceRule {
  matchType: string;
  matchValue: string;
  priority: number;
  status: HubNodeStatus;
}

export interface HubLoginPolicy {
  platformCode: string;
  status: HubNodeStatus;
  allowAutoRegister: boolean;
  allowFormRegister: boolean;
  loginMethods: HubLoginMethod[];
  sourceRules: HubSourceRule[];
}

export interface HubFederationStatus {
  nodeCode: string;
  oidcClientId?: string;
  connectionStatus: HubConnectionStatus;
  connectionVersion?: string;
  lastConnectionError?: string;
  lastConnectionTraceId?: string;
}

export interface UserStatusCommand {
  status: HubUserStatusValue;
  reason: string;
}

export interface SessionRevokeCommand {
  all: boolean;
  sessionRefs?: string[];
  reason: string;
}

export interface RequestDescriptor<T> {
  url: string;
  method: 'POST' | 'PUT';
  data: T;
  headers: Record<string, string>;
  _challengeFingerprint: string;
}

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function list(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function text(value: unknown): string {
  return value === undefined || value === null ? '' : String(value);
}

function optionalText(value: unknown): string | undefined {
  const normalized = text(value).trim();
  return normalized || undefined;
}

function numberValue(value: unknown, fallback = 0): number {
  const normalized = Number(value);
  return Number.isFinite(normalized) ? normalized : fallback;
}

function boolValue(value: unknown): boolean {
  return value === true || value === 1 || value === '1' || value === 'true';
}

function nodeStatus(value: unknown): HubNodeStatus {
  return value === 0 || value === 1 ? value : 'UNKNOWN';
}

function userStatus(value: unknown): HubUserStatus {
  return value === 0 || value === 1 || value === 2 ? value : 'UNKNOWN';
}

function connectionStatus(value: unknown): HubConnectionStatus {
  return value === 'PENDING' || value === 'ACTIVE' || value === 'ERROR' ? value : 'UNKNOWN';
}

function discoveryType(value: unknown): HubDiscoveryType {
  return value === 'STATIC' || value === 'CONSUL' ? value : 'UNKNOWN';
}

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

export function runeLength(value: string): number {
  return Array.from(value).length;
}

export function isValidHubNodeCode(value: string): boolean {
  const normalized = value.trim();
  return normalized.length >= 2
    && normalized.length <= 60
    && /^[a-z0-9][a-z0-9._-]+$/.test(normalized);
}

function parseAbsoluteUrl(value: string): URL | null {
  try {
    const normalized = value.trim();
    const parsed = new URL(normalized);
    return parsed.host ? parsed : null;
  } catch {
    return null;
  }
}

export function isValidManagementBaseUrl(value: string): boolean {
  const parsed = parseAbsoluteUrl(value);
  return Boolean(
    parsed
      && utf8ByteLength(value.trim()) <= 2048
      && (parsed.protocol === 'http:' || parsed.protocol === 'https:')
      && parsed.port
      && !parsed.username
      && !parsed.password
      && !parsed.search
      && !parsed.hash
      && (parsed.pathname === '' || parsed.pathname === '/'),
  );
}

export function isValidHubIssuer(value: string): boolean {
  const normalized = value.trim();
  const parsed = parseAbsoluteUrl(normalized);
  return Boolean(
    parsed
      && utf8ByteLength(normalized) <= 512
      && parsed.protocol === 'https:'
      && !parsed.username
      && !parsed.password
      && !parsed.search
      && !parsed.hash,
  );
}

export function isValidFederationRedirectUri(value: string): boolean {
  const normalized = value.trim();
  const parsed = parseAbsoluteUrl(normalized);
  return Boolean(
    parsed
      && utf8ByteLength(normalized) <= 2048
      && parsed.protocol === 'https:'
      && !parsed.hash,
  );
}

function canonicalChallengeValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonicalChallengeValue);
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, childValue]) => [key, canonicalChallengeValue(childValue)]),
    );
  }
  return value;
}

async function sha256Hex(value: unknown): Promise<string> {
  const bytes = new TextEncoder().encode(JSON.stringify(canonicalChallengeValue(value)));
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
}

async function redactChallengeValue(value: unknown, key = ''): Promise<unknown> {
  if (key === 'sessionRefs' || key === 'managementBearer') {
    return {
      redacted: true,
      count: Array.isArray(value) ? value.length : value ? 1 : 0,
      sha256: await sha256Hex(value),
    };
  }
  if (Array.isArray(value)) return Promise.all(value.map((item) => redactChallengeValue(item)));
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      await Promise.all(
        Object.entries(value as Record<string, unknown>)
          .sort(([left], [right]) => left.localeCompare(right))
          .map(async ([childKey, childValue]) => [
            childKey,
            await redactChallengeValue(childValue, childKey),
          ] as const),
      ),
    );
  }
  return value;
}

export async function buildHubChallengeFingerprint(
  method: string,
  url: string,
  data?: unknown,
): Promise<string> {
  return `${method.toUpperCase()}|${url}|${JSON.stringify(await redactChallengeValue(data))}`;
}

export function isManagedLoginMethodType(value: unknown): value is ManagedLoginMethodType {
  return MANAGED_LOGIN_METHOD_TYPES.some((methodType) => methodType === value);
}

export function normalizeCapabilities(value: unknown): string[] {
  let source = value;
  if (typeof value === 'string') {
    try {
      source = JSON.parse(value);
    } catch {
      source = [];
    }
  }
  return [...new Set(list(source).map((item) => text(item).trim()).filter(Boolean))];
}

export function normalizeHubNode(raw: unknown): HubNodeRecord {
  const source = record(raw);
  return {
    nodeCode: text(source.nodeCode),
    nodeName: text(source.nodeName),
    status: nodeStatus(source.status),
    discoveryType: discoveryType(source.discoveryType),
    serviceName: optionalText(source.serviceName),
    managementBaseUrl: optionalText(source.managementBaseUrl),
    hubIssuer: text(source.hubIssuer),
    oidcClientId: optionalText(source.oidcClientId),
    capabilities: normalizeCapabilities(source.capabilitiesJson ?? source.capabilities),
    connectionStatus: connectionStatus(source.connectionStatus),
    connectionVersion: optionalText(source.connectionVersion),
    issuerLockedAt: optionalText(source.issuerLockedAt),
    lastConnectionError: optionalText(source.lastConnectionError),
    lastConnectionTraceId: optionalText(source.lastConnectionTraceId),
    lastHealthyAt: optionalText(source.lastHealthyAt),
    createdAt: optionalText(source.createdAt),
    updatedAt: optionalText(source.updatedAt),
  };
}

export function normalizeHubNodePage(raw: unknown): HubNodePage {
  const source = record(raw);
  return {
    current: numberValue(source.current, 1),
    size: numberValue(source.size, 20),
    total: numberValue(source.total),
    records: list(source.records).map(normalizeHubNode),
  };
}

export function normalizeHubNodeHealth(raw: unknown): HubNodeHealth {
  const source = record(raw);
  return {
    nodeCode: text(source.nodeCode),
    version: text(source.version),
    capabilities: normalizeCapabilities(source.capabilities),
    health: text(source.health).toUpperCase() || 'UNKNOWN',
    traceId: optionalText(source.traceId),
  };
}

export function normalizeHubUser(raw: unknown): HubUserRecord {
  const source = record(raw);
  return {
    userId: text(source.userId),
    username: text(source.username),
    nickname: text(source.nickname),
    emailMasked: optionalText(source.emailMasked),
    phoneMasked: optionalText(source.phoneMasked),
    status: userStatus(source.status),
    createdAt: optionalText(source.createdAt),
    updatedAt: optionalText(source.updatedAt),
  };
}

export function normalizeHubUserPage(raw: unknown): HubUserPage {
  const source = record(raw);
  return {
    current: numberValue(source.current, 1),
    size: numberValue(source.size, 20),
    total: numberValue(source.total),
    records: list(source.records).map(normalizeHubUser),
  };
}

export function normalizeHubSessionPage(raw: unknown): HubSessionPage {
  const source = record(raw);
  return {
    current: numberValue(source.current, 1),
    size: numberValue(source.size, 20),
    total: numberValue(source.total),
    records: list(source.records).map((item) => {
      const session = record(item);
      const status: HubSessionStatus =
        session.status === 'ACTIVE' || session.status === 'EXPIRED' || session.status === 'REVOKED'
          ? session.status
          : 'UNKNOWN';
      return {
        sessionRef: text(session.sessionRef),
        clientId: text(session.clientId),
        loginMethod: optionalText(session.loginMethod),
        loginAt: optionalText(session.loginAt),
        lastAccessAt: optionalText(session.lastAccessAt),
        expiresAt: optionalText(session.expiresAt),
        status,
      };
    }),
  };
}

export function normalizeHubLoginPolicy(raw: unknown): HubLoginPolicy {
  const source = record(raw);
  return {
    platformCode: text(source.platformCode),
    status: nodeStatus(source.status),
    allowAutoRegister: boolValue(source.allowAutoRegister),
    allowFormRegister: boolValue(source.allowFormRegister),
    loginMethods: list(source.loginMethods).map((item) => {
      const method = record(item);
      return {
        methodType: isManagedLoginMethodType(method.methodType) ? method.methodType : 'UNKNOWN',
        providerCode: optionalText(method.providerCode),
        displayName: text(method.displayName),
        icon: optionalText(method.icon),
        sortOrder: numberValue(method.sortOrder),
        displayEnabled: boolValue(method.displayEnabled),
        loginEnabled: boolValue(method.loginEnabled),
      };
    }),
    sourceRules: list(source.sourceRules).map((item) => {
      const rule = record(item);
      return {
        matchType: text(rule.matchType).toUpperCase(),
        matchValue: text(rule.matchValue),
        priority: numberValue(rule.priority),
        status: nodeStatus(rule.status),
      };
    }),
  };
}

export function normalizeHubFederationStatus(raw: unknown): HubFederationStatus {
  const source = record(raw);
  return {
    nodeCode: text(source.nodeCode),
    oidcClientId: optionalText(source.oidcClientId),
    connectionStatus: connectionStatus(source.connectionStatus),
    connectionVersion: optionalText(source.connectionVersion),
    lastConnectionError: optionalText(source.lastConnectionError),
    lastConnectionTraceId: optionalText(source.lastConnectionTraceId),
  };
}

export function deriveExistingProviderOptions(policy?: HubLoginPolicy | null) {
  const providerCodes = (policy?.loginMethods || [])
    .map((method) => method.providerCode?.trim())
    .filter((providerCode): providerCode is string => Boolean(providerCode));
  return [...new Set(providerCodes)].sort().map((providerCode) => ({
    label: providerCode,
    value: providerCode,
  }));
}

export function isHubIssuerLocked(node?: HubNodeRecord | null): boolean {
  return Boolean(node?.issuerLockedAt) || node?.connectionStatus === 'ACTIVE';
}

export function canManageHubNode(node?: HubNodeRecord | null): boolean {
  return Boolean(
    node
      && node.status !== 'UNKNOWN'
      && node.discoveryType !== 'UNKNOWN'
      && node.connectionStatus !== 'UNKNOWN',
  );
}

export function canManageHubUser(user?: HubUserRecord | null): boolean {
  return Boolean(user && user.status !== 'UNKNOWN');
}

export function canApplyHubLoginPolicy(policy?: HubLoginPolicy | null): boolean {
  return Boolean(
    policy
      && policy.status !== 'UNKNOWN'
      && policy.loginMethods.every(
        (method) =>
          method.methodType !== 'UNKNOWN'
          && (method.methodType !== 'EXTERNAL_OAUTH' || Boolean(method.providerCode)),
      )
      && policy.sourceRules.every((rule) => rule.status !== 'UNKNOWN'),
  );
}

export function unwrapHubResponse<T>(response: HubApiResponse<T>, fallbackMessage: string): T {
  if (!response || typeof response !== 'object' || response.code !== 0) {
    throw new Error(response?.message || fallbackMessage);
  }
  return response.data;
}

export function freshHubIdempotencyKey(): string {
  const uuid = globalThis.crypto?.randomUUID?.();
  if (uuid) {
    return `hub-ui-${uuid}`;
  }
  return `hub-ui-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function commandHeaders() {
  return { 'Idempotency-Key': freshHubIdempotencyKey() };
}

function nodeUserPath(nodeCode: string, userId: string) {
  return `/api/system/hub/nodes/${encodeURIComponent(nodeCode)}/users/${encodeURIComponent(userId)}`;
}

export async function buildUserStatusRequest(
  nodeCode: string,
  userId: string,
  data: UserStatusCommand,
): Promise<RequestDescriptor<UserStatusCommand>> {
  const url = `${nodeUserPath(nodeCode, userId)}/status`;
  return {
    url,
    method: 'PUT',
    data,
    headers: commandHeaders(),
    _challengeFingerprint: await buildHubChallengeFingerprint('PUT', url, data),
  };
}

export async function buildSessionRevokeRequest(
  nodeCode: string,
  userId: string,
  data: SessionRevokeCommand,
): Promise<RequestDescriptor<SessionRevokeCommand>> {
  const url = `${nodeUserPath(nodeCode, userId)}/sessions/revoke`;
  return {
    url,
    method: 'POST',
    data,
    headers: commandHeaders(),
    _challengeFingerprint: await buildHubChallengeFingerprint('POST', url, data),
  };
}

export async function buildReasonedRequest<T>(url: string, data: T): Promise<RequestDescriptor<T>> {
  return {
    url,
    method: 'POST',
    data,
    headers: commandHeaders(),
    _challengeFingerprint: await buildHubChallengeFingerprint('POST', url, data),
  };
}
