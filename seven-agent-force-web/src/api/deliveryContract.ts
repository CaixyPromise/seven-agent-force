export type DeliveryAuthorization = 'AUTO_AFTER_GATES' | 'ASK_AFTER_GATES';
export type DeliveryMode = 'NONE' | 'NEW_BRANCH' | 'NEW_BRANCH_AND_DRAFT_REVIEW';
export type DeliveryDecisionAction = 'DELIVER' | 'SKIP_THIS_RESULT';
export type DeliveryConversationAction = DeliveryDecisionAction | 'RESTART_DELIVERY';
export type DeliveryConversationStatus =
  | 'SCHEDULED'
  | 'WAITING_FINALIZATION'
  | 'WAITING_CHECKS'
  | 'WAITING_REVIEW'
  | 'WAITING_USER_DECISION'
  | 'AUTO_DELIVERY_SCHEDULED'
  | 'PREPARING'
  | 'PUSHING'
  | 'VERIFYING_BRANCH'
  | 'CREATING_REVIEW'
  | 'VERIFYING_REVIEW'
  | 'DELIVERED'
  | 'DELIVERY_FAILED'
  | 'OUTCOME_UNKNOWN'
  | 'SKIPPED'
  | 'CANCELLED';

export type DeliveryConversationCard = {
  intentId: string;
  requestId?: string;
  version: string;
  intentVersion: string;
  requestVersion?: string;
  status: DeliveryConversationStatus;
  summary: string;
  deliveryMode: Exclude<DeliveryMode, 'NONE'>;
  authorization: DeliveryAuthorization;
  repositoryName: string;
  destinationBranch: string;
  resultCommit?: string;
  deliveryCommit?: string;
  pullRequestUrl?: string;
  remoteReadbackVerified?: boolean;
  reasonCode?: string;
  allowedActions: DeliveryConversationAction[];
};

export type DeliveryRequestStatus =
  | 'WAITING_APPROVAL' | 'MANUAL_ONLY' | 'READY' | 'DISPATCHED' | 'RUNNING'
  | 'CONFIRMING' | 'RETRY_WAIT' | 'SUCCEEDED' | 'FAILED' | 'MANUAL_REVIEW'
  | 'OUTCOME_UNKNOWN' | 'CANCELLING' | 'CANCELLED';

export type DeliveryRequest = {
  id: string;
  version: string;
  taskId: string;
  taskTurnId: string;
  checkpointId: string;
  actionType: 'GIT_PUSH' | 'CUSTOM_DELIVERY';
  projectId: string;
  baseBranch: string;
  destinationBranch: string;
  resultCommit: string;
  resultTree: string;
  selectionDigest: string;
  deliveryCommit: string;
  approvalMode: 'AFTER_APPROVAL' | 'ON_PASS' | 'MANUAL_ONLY';
  approvedBy?: string;
  approvedAt?: string;
  status: DeliveryRequestStatus;
  reasonCode?: string;
  createdAt: string;
  review?: { operationId?: string; status: string; providerType?: string; providerNumber?: string; webUrl?: string; reasonCode?: string };
};

export class DeliveryContractError extends Error { constructor() { super('DELIVERY_CONTRACT_INVALID'); } }
function invalid(): never { throw new DeliveryContractError(); }
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
  return value as Record<string, unknown>;
}
function decimal(value: unknown, optional = false): string | undefined {
  if (optional && (value === undefined || value === null || value === '0')) return undefined;
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value) || value.length > 19 || (value.length === 19 && value > '9223372036854775807')) return invalid();
  return value;
}
function text(value: unknown, max: number, optional = false): string | undefined {
  if (optional && (value === undefined || value === null || value === '')) return undefined;
  if (typeof value !== 'string' || value.includes('\0') || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value) || new TextEncoder().encode(value).length > max) return invalid();
  return value;
}
function enumValue<T extends string>(value: unknown, allowed: readonly T[]): T {
  return typeof value === 'string' && allowed.includes(value as T) ? value as T : invalid();
}
function date(value: unknown, optional = false): string | undefined {
  const result = text(value, 64, optional);
  if (result === undefined) return undefined;
  return /^\d{4}-\d{2}-\d{2}T/.test(result) && Number.isFinite(Date.parse(result)) ? result : invalid();
}
function digest(value: unknown, optional = false): string | undefined {
  const result = text(value, 64, optional);
  return result === undefined ? undefined : /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/i.test(result) ? result : invalid();
}
function safeURL(value: unknown, optional = false): string | undefined {
  const result = text(value, 2048, optional);
  if (result === undefined) return undefined;
  try {
    const parsed = new URL(result);
    if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) return invalid();
    return parsed.toString();
  } catch { return invalid(); }
}

const deliveryModes = ['NONE', 'NEW_BRANCH', 'NEW_BRANCH_AND_DRAFT_REVIEW'] as const;
const authorizations = ['AUTO_AFTER_GATES', 'ASK_AFTER_GATES'] as const;
const conversationStatuses: DeliveryConversationStatus[] = ['SCHEDULED', 'WAITING_FINALIZATION', 'WAITING_CHECKS', 'WAITING_REVIEW', 'WAITING_USER_DECISION', 'AUTO_DELIVERY_SCHEDULED', 'PREPARING', 'PUSHING', 'VERIFYING_BRANCH', 'CREATING_REVIEW', 'VERIFYING_REVIEW', 'DELIVERED', 'DELIVERY_FAILED', 'OUTCOME_UNKNOWN', 'SKIPPED', 'CANCELLED'];
const conversationActions: DeliveryConversationAction[] = ['DELIVER', 'SKIP_THIS_RESULT', 'RESTART_DELIVERY'];
const requestStatuses: DeliveryRequestStatus[] = ['WAITING_APPROVAL', 'MANUAL_ONLY', 'READY', 'DISPATCHED', 'RUNNING', 'CONFIRMING', 'RETRY_WAIT', 'SUCCEEDED', 'FAILED', 'MANUAL_REVIEW', 'OUTCOME_UNKNOWN', 'CANCELLING', 'CANCELLED'];

export function normalizeDeliveryAuthorization(value: unknown): DeliveryAuthorization {
  return enumValue(value, authorizations);
}

export function normalizeDeliveryConversationCard(value: unknown): DeliveryConversationCard {
  const data = record(value);
  const deliveryMode = enumValue(data.deliveryMode, deliveryModes);
  if (deliveryMode === 'NONE') return invalid();
  // Older persisted projections encoded an empty Go slice as null. Treat that
  // exact representation as no actions; every non-empty value stays strict.
  const rawActions = data.allowedActions === null ? [] : data.allowedActions;
  const actions = Array.isArray(rawActions) && rawActions.length <= 5
    ? rawActions.map((action) => enumValue(action, conversationActions))
    : invalid();
  if (new Set(actions).size !== actions.length) return invalid();
  const summary = text(data.summary, 2048)!;
  const repositoryName = text(data.repositoryName, 512)!;
  const destinationBranch = text(data.destinationBranch, 1024)!;
  if (!summary.trim() || !repositoryName.trim() || !destinationBranch.trim()) return invalid();
  const requestId = decimal(data.requestId, true);
	const requestVersion = decimal(data.requestVersion, true);
  const resultCommit = digest(data.resultCommit, true);
	if (actions.some((action) => action === 'DELIVER' || action === 'SKIP_THIS_RESULT') && (!requestId || !requestVersion)) return invalid();
  if (actions.includes('RESTART_DELIVERY') && !resultCommit) return invalid();
  return {
    intentId: decimal(data.intentId)!,
    requestId,
    version: decimal(data.version)!,
		intentVersion: decimal(data.intentVersion)!,
		requestVersion,
    status: enumValue(data.status, conversationStatuses),
    summary,
    deliveryMode,
    authorization: enumValue(data.authorization, authorizations),
    repositoryName,
    destinationBranch,
    resultCommit,
    deliveryCommit: digest(data.deliveryCommit, true),
    pullRequestUrl: safeURL(data.pullRequestUrl, true),
    remoteReadbackVerified: data.remoteReadbackVerified === undefined || data.remoteReadbackVerified === null ? undefined : typeof data.remoteReadbackVerified === 'boolean' ? data.remoteReadbackVerified : invalid(),
    reasonCode: text(data.reasonCode, 128, true),
    allowedActions: actions,
  };
}

export function normalizeDeliveryRequest(value: unknown): DeliveryRequest {
  const data = record(value);
  const review = data.review === undefined || data.review === null ? undefined : (() => {
    const item = record(data.review);
    return {
      operationId: decimal(item.operationId, true),
      status: enumValue(item.status, ['PENDING', 'WAITING_DEPENDENCY', 'WAITING_APPROVAL', 'READY', 'RUNNING', 'RETRY_WAIT', 'SUCCEEDED', 'FAILED', 'MANUAL_REVIEW', 'OUTCOME_UNKNOWN', 'CANCELLED'] as const),
      providerType: text(item.providerType, 64, true),
      providerNumber: text(item.providerNumber, 256, true),
      webUrl: safeURL(item.webUrl, true),
      reasonCode: text(item.reasonCode, 128, true),
    };
  })();
  return {
    id: decimal(data.id)!, version: decimal(data.version)!, taskId: decimal(data.taskId)!, taskTurnId: decimal(data.taskTurnId)!, checkpointId: decimal(data.checkpointId)!,
    actionType: enumValue(data.actionType, ['GIT_PUSH', 'CUSTOM_DELIVERY'] as const), projectId: decimal(data.projectId)!,
    baseBranch: text(data.baseBranch, 1024)!, destinationBranch: text(data.destinationBranch, 1024)!,
    resultCommit: digest(data.resultCommit)!, resultTree: digest(data.resultTree)!, selectionDigest: digest(data.selectionDigest)!, deliveryCommit: digest(data.deliveryCommit)!,
    approvalMode: enumValue(data.approvalMode, ['AFTER_APPROVAL', 'ON_PASS', 'MANUAL_ONLY'] as const), approvedBy: decimal(data.approvedBy, true), approvedAt: date(data.approvedAt, true),
    status: enumValue(data.status, requestStatuses), reasonCode: text(data.reasonCode, 128, true), createdAt: date(data.createdAt)!, review,
  };
}

export function normalizeDeliveryRequests(value: unknown): DeliveryRequest[] {
  if (!Array.isArray(value) || value.length > 100) return invalid();
  const items = value.map(normalizeDeliveryRequest);
  if (new Set(items.map((item) => item.id)).size !== items.length) return invalid();
  return items;
}

export function deliveryErrorCode(error: unknown): string {
  try {
    const root = record(error); const payload = root.payload ? record(root.payload) : root; const data = payload.data ? record(payload.data) : payload;
    const code = data.reasonCode;
    return typeof code === 'string' && /^[A-Z0-9_]{1,128}$/.test(code) ? code : '';
  } catch { return ''; }
}

export function deliveryFeedback(error: unknown, operation = '交付操作'): string {
  const messages: Record<string, string> = {
    DELIVERY_INTENT_NOT_AVAILABLE: '当前结果没有可用的交付请求。',
    DELIVERY_REQUEST_NOT_FOUND: '这项交付不存在或暂不可访问。',
    DELIVERY_REQUEST_STALE: '交付状态已经变化，已刷新当前结果。',
    DELIVERY_DECISION_ACTION_NOT_ALLOWED: '当前状态不能执行这项交付操作。',
    DELIVERY_DECISION_IDEMPOTENCY_CONFLICT: '这次提交与已保存的决定不一致，已刷新当前结果。',
    DELIVERY_TASK_AUTHORIZATION_INVALID: '任务的自动交付授权已失效，需要你重新确认。',
    DELIVERY_RESULT_NOT_CURRENT: '任务结果已经更新，请确认最新结果后再交付。',
    DELIVERY_REAUTHORIZATION_REQUIRED: '仓库授权已经变化，需要重新确认后继续。',
    DELIVERY_OUTCOME_UNKNOWN: '远端结果暂时无法确认，请先核对远端状态。',
  };
  return messages[deliveryErrorCode(error)] ?? `${operation}未完成，请重试。`;
}
