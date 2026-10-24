export type NetworkTarget = { host: string; protocol: 'http' | 'https'; port: number };
export type ExecutionOperationStatus =
  | 'WAITING_APPROVAL'
  | 'READY'
  | 'DISPATCHED'
  | 'RUNNING'
  | 'SUCCEEDED'
  | 'FAILED'
  | 'OUTCOME_UNKNOWN'
  | 'DENIED'
  | 'CANCELLED'
  | 'EXPIRED';
export type OperationDecisionAction = 'ALLOW_ONCE' | 'ALLOW_FOR_TASK' | 'DENY';
export type OperationDeliveryStatus = 'NONE' | 'SAVED' | 'DELIVERED_TO_ADAPTER' | 'NATIVE_CALL_RESOLVED';

export type ExecutionOperationSummary = {
  operationId: string;
  version: string;
  status: ExecutionOperationStatus;
  purpose: string;
  targets: NetworkTarget[];
  decisionAction?: OperationDecisionAction;
  resultDeliveryStatus?: OperationDeliveryStatus;
  agentContinued: boolean;
  result?: { exitCode?: number; summary: string };
};

export type ExecutionOperationDetail = ExecutionOperationSummary & {
  id: string;
  taskId: string;
  command: string;
  cwd: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type TaskNetworkGrant = {
  id: string;
  taskId: string;
  version: string;
  status: 'ACTIVE' | 'REVOKED' | 'EXPIRED';
  targets: NetworkTarget[];
  createdAt: string;
  expiresAt?: string;
  revokedAt?: string;
  actorUserId?: string;
  purpose?: string;
  lastOperation?: {
    status: ExecutionOperationStatus;
    resultSummary?: string;
    exitCode?: number;
    resultDeliveryStatus?: OperationDeliveryStatus;
    agentContinued: boolean;
    finishedAt?: string;
  };
};
export type ExecutionOperationReceipt = { operationId: string; grantId?: string; version: string; status: ExecutionOperationStatus; replayed: boolean };
export type TaskNetworkGrantReceipt = { grantId: string; version: string; status: TaskNetworkGrant['status']; replayed: boolean };

const operationStatuses: ExecutionOperationStatus[] = ['WAITING_APPROVAL', 'READY', 'DISPATCHED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'OUTCOME_UNKNOWN', 'DENIED', 'CANCELLED', 'EXPIRED'];
const decisionActions: OperationDecisionAction[] = ['ALLOW_ONCE', 'ALLOW_FOR_TASK', 'DENY'];
const deliveryStatuses: OperationDeliveryStatus[] = ['NONE', 'SAVED', 'DELIVERED_TO_ADAPTER', 'NATIVE_CALL_RESOLVED'];
const grantStatuses: TaskNetworkGrant['status'][] = ['ACTIVE', 'REVOKED', 'EXPIRED'];

export class ExecutionAccessContractError extends Error { constructor() { super('EXECUTION_ACCESS_INVALID'); } }
function invalid(): never { throw new ExecutionAccessContractError(); }
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
  return value as Record<string, unknown>;
}
function decimal(value: unknown): string {
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value) || value.length > 19 || (value.length === 19 && value > '9223372036854775807')) return invalid();
  return value;
}
function conversationText(value: unknown, max = 16384): string {
  if (typeof value !== 'string' || value.includes('\0') || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value) || new TextEncoder().encode(value).length > max) return invalid();
  return value;
}
function enumValue<T extends string>(value: unknown, allowed: readonly T[]): T {
  return typeof value === 'string' && allowed.includes(value as T) ? value as T : invalid();
}
function optionalEnum<T extends string>(value: unknown, allowed: readonly T[]): T | undefined {
  return value === undefined || value === null ? undefined : enumValue(value, allowed);
}
function date(value: unknown): string {
  const result = conversationText(value, 64);
  return /^\d{4}-\d{2}-\d{2}T/.test(result) && Number.isFinite(Date.parse(result)) ? result : invalid();
}
function optionalDate(value: unknown): string | undefined { return value === undefined || value === null ? undefined : date(value); }
function bool(value: unknown): boolean { return typeof value === 'boolean' ? value : invalid(); }
function integer(value: unknown): number {
  return typeof value === 'number' && Number.isSafeInteger(value) ? value : invalid();
}
function target(value: unknown): NetworkTarget {
  const data = record(value);
  const host = conversationText(data.host, 253).toLowerCase();
  if (!host || host.trim() !== host || /^\d+(?:\.\d+){3}$/.test(host) || !/^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(host)) invalid();
  const port = integer(data.port);
  if (port < 1 || port > 65535) invalid();
  return { host, protocol: enumValue(data.protocol, ['http', 'https'] as const), port };
}
function targets(value: unknown): NetworkTarget[] {
  if (!Array.isArray(value) || value.length < 1 || value.length > 8) return invalid();
  const result = value.map(target);
  if (new Set(result.map((item) => `${item.protocol}:${item.host}:${item.port}`)).size !== result.length) invalid();
  return result;
}
function result(value: unknown): ExecutionOperationSummary['result'] {
  if (value === undefined || value === null) return undefined;
  const data = record(value);
  return { ...(data.exitCode === undefined || data.exitCode === null ? {} : { exitCode: integer(data.exitCode) }), summary: conversationText(data.summary, 4096) };
}

export function normalizeExecutionOperationSummary(value: unknown): ExecutionOperationSummary {
  const data = record(value);
  const purpose = conversationText(data.purpose, 2048);
  if (!purpose.trim()) invalid();
  const hasFlatResult = (typeof data.resultSummary === 'string' && data.resultSummary.length > 0) || (data.exitCode !== undefined && data.exitCode !== null);
  const resultValue = data.result ?? (hasFlatResult ? { summary: data.resultSummary ?? '', exitCode: data.exitCode } : undefined);
  const resultDeliveryStatus = data.resultDeliveryStatus === '' ? undefined : optionalEnum(data.resultDeliveryStatus, deliveryStatuses);
  return {
    operationId: decimal(data.operationId),
    version: decimal(data.version),
    status: enumValue(data.status, operationStatuses),
    purpose,
    targets: targets(data.targets),
    decisionAction: optionalEnum(data.decisionAction, decisionActions),
    resultDeliveryStatus,
    // Missing evidence is false. Native call resolution proves only that the
    // App Server accepted the tool output, not that the Agent continued.
    agentContinued: data.agentContinued === undefined ? false : bool(data.agentContinued),
    result: result(resultValue),
  };
}

export function normalizeExecutionOperationDetail(value: unknown): ExecutionOperationDetail {
  const data = record(value);
  const summary = normalizeExecutionOperationSummary({ ...data, operationId: data.id ?? data.operationId });
  const cwd = conversationText(data.cwd, 1024);
  if (!cwd || cwd.startsWith('/') || cwd.split('/').some((part) => part === '..')) invalid();
  return {
    ...summary,
    id: decimal(data.id ?? data.operationId),
    taskId: decimal(data.taskId),
    command: conversationText(data.command, 16384),
    cwd,
    createdAt: date(data.createdAt),
    startedAt: optionalDate(data.startedAt),
    finishedAt: optionalDate(data.finishedAt),
  };
}

export function normalizeTaskNetworkGrant(value: unknown): TaskNetworkGrant {
  const data = record(value);
  const lastOperation = data.lastOperation === undefined || data.lastOperation === null ? undefined : (() => {
    const item = record(data.lastOperation);
    return {
      status: enumValue(item.status, operationStatuses),
      resultSummary: item.resultSummary === undefined || item.resultSummary === null ? undefined : conversationText(item.resultSummary, 4096),
      exitCode: item.exitCode === undefined || item.exitCode === null ? undefined : integer(item.exitCode),
      resultDeliveryStatus: optionalEnum(item.resultDeliveryStatus, deliveryStatuses),
      agentContinued: item.agentContinued === undefined ? false : bool(item.agentContinued),
      finishedAt: optionalDate(item.finishedAt),
    };
  })();
  return {
    id: decimal(data.id),
    taskId: decimal(data.taskId),
    version: decimal(data.version),
    status: enumValue(data.status, grantStatuses),
    targets: targets(data.targets),
    createdAt: date(data.createdAt),
    expiresAt: optionalDate(data.expiresAt),
    revokedAt: optionalDate(data.revokedAt),
    actorUserId: data.actorUserId === undefined || data.actorUserId === null ? undefined : decimal(data.actorUserId),
    purpose: data.purpose === undefined || data.purpose === null ? undefined : conversationText(data.purpose, 2048),
    lastOperation,
  };
}

export function normalizeTaskNetworkGrants(value: unknown): TaskNetworkGrant[] {
  const raw = Array.isArray(value) ? value : record(value).items;
  if (!Array.isArray(raw) || raw.length > 100) return invalid();
  const items = raw.map(normalizeTaskNetworkGrant);
  if (new Set(items.map((item) => item.id)).size !== items.length) invalid();
  return items;
}

export function normalizeExecutionOperationReceipt(value: unknown): ExecutionOperationReceipt {
  const data = record(value);
  return {
    operationId: decimal(data.operationId),
    grantId: data.grantId === undefined || data.grantId === null || data.grantId === '0' ? undefined : decimal(data.grantId),
    version: decimal(data.version),
    status: enumValue(data.status, operationStatuses),
    replayed: bool(data.replayed),
  };
}

export function normalizeTaskNetworkGrantReceipt(value: unknown): TaskNetworkGrantReceipt {
  const data = record(value);
  return { grantId: decimal(data.grantId), version: decimal(data.version), status: enumValue(data.status, grantStatuses), replayed: bool(data.replayed) };
}

export function executionAccessErrorCode(error: unknown): string {
  try {
    const outer = record(error);
    const payload = outer.payload ? record(outer.payload) : outer;
    const data = payload.data ? record(payload.data) : payload;
    return typeof data.reasonCode === 'string' ? data.reasonCode : '';
  } catch { return ''; }
}

export function executionAccessFeedback(error: unknown, operation = '操作'): string {
  const messages: Record<string, string> = {
    EXECUTION_OPERATION_NOT_FOUND: '这项访问请求不存在或暂不可访问。',
    EXECUTION_OPERATION_STALE: '访问请求已经变化，请查看最新状态。',
    EXECUTION_OPERATION_ACTION_NOT_ALLOWED: '当前状态不能执行这个决定，请查看最新状态。',
    EXECUTION_OPERATION_IDEMPOTENCY_CONFLICT: '这次提交与已保存的决定不一致，请查看最新状态。',
    EXECUTION_OPERATION_TARGET_DENIED: '请求的外部服务不在可访问范围内。',
    EXECUTION_OPERATION_OUTCOME_UNKNOWN: '无法确认命令是否完整执行，平台不会自动重试。',
    EXECUTION_OPERATION_FENCED: '这项访问请求已失效，请查看最新任务状态。',
    EXECUTION_OPERATION_UNAVAILABLE: '当前暂时无法执行网络操作，请稍后重试。',
    EXECUTION_OPERATION_INVALID: '访问请求内容不符合要求，请查看最新状态。',
    TASK_NETWORK_GRANT_STALE: '访问权限已经变化，请查看最新状态。',
    TASK_NETWORK_GRANT_REVOKED: '本任务的这项访问权限已经撤销。',
    TASK_NETWORK_GRANT_EXPIRED: '本任务的这项访问权限已经过期。',
  };
  return messages[executionAccessErrorCode(error)] ?? `${operation}未完成，请重试。`;
}
