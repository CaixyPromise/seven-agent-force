export interface CompletionCheck {
  mode: 'NONE' | 'COMMAND' | 'SCRIPT';
  command?: string;
  script?: string;
  instructions?: string;
  referenceAssetIds?: API.Int64[];
}

export const completionCheckLimits = { command: 8192, script: 65536, instructions: 16384, referenceAssetIds: 64 } as const;

export class CompletionCheckValidationError extends TypeError {
  readonly code = 'COMPLETION_CHECK_INVALID';
  constructor(readonly field: string) { super('COMPLETION_CHECK_INVALID'); }
}

export function completionCheckId(value: unknown): API.Int64 {
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value) || value.length > 19 || (value.length === 19 && value > '9223372036854775807')) throw new CompletionCheckValidationError('identity');
  return value;
}

export function completionCheckText(value: unknown, field: string, maxBytes: number, required = false): string | undefined {
  if (value === undefined && !required) return undefined;
  if (typeof value !== 'string' || new TextEncoder().encode(value).length > maxBytes || /[\uD800-\uDFFF]/u.test(value) || (required && !value.trim())) throw new CompletionCheckValidationError(field);
  const hasControl = Array.from(value.replace(/\r\n/g, '\n')).some((character) => {
    const code = character.charCodeAt(0);
    return (code < 32 && code !== 9 && code !== 10) || (code >= 127 && code <= 159);
  });
  if (hasControl) throw new CompletionCheckValidationError(field);
  return value;
}

export function normalizeCompletionCheck(value: unknown): CompletionCheck {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new CompletionCheckValidationError('mode');
  const item = value as Record<string, unknown>;
  if (item.mode !== 'NONE' && item.mode !== 'COMMAND' && item.mode !== 'SCRIPT') throw new CompletionCheckValidationError('mode');
  const command = item.mode === 'COMMAND' ? completionCheckText(item.command, 'command', completionCheckLimits.command, true) : undefined;
  const script = item.mode === 'SCRIPT' ? completionCheckText(item.script, 'script', completionCheckLimits.script, true) : undefined;
  const instructions = completionCheckText(item.instructions, 'instructions', completionCheckLimits.instructions);
  const refs = item.referenceAssetIds === undefined ? [] : item.referenceAssetIds;
  if (!Array.isArray(refs) || refs.length > completionCheckLimits.referenceAssetIds) throw new CompletionCheckValidationError('referenceAssetIds');
  const referenceAssetIds = refs.map(completionCheckId);
  if (new Set(referenceAssetIds).size !== referenceAssetIds.length) throw new CompletionCheckValidationError('referenceAssetIds');
  // Inactive editor drafts stay in the form but never become executable input.
  return { mode: item.mode, ...(item.mode === 'COMMAND' ? { command } : {}), ...(item.mode === 'SCRIPT' ? { script } : {}), ...(instructions !== undefined ? { instructions } : {}), referenceAssetIds };
}

const safeFeedback: Record<string, string> = {
  COMPLETION_CHECK_INVALID: '完成检查内容无效，请检查填写内容和资料后重试。',
  COMPLETION_CHECK_NOT_FOUND: '当前完成检查不可用，请刷新状态后重试。',
  COMPLETION_CHECK_STALE: '完成检查已更新，请刷新并确认最新结果后再提交。',
  COMPLETION_CHECK_ACTION_NOT_ALLOWED: '当前结果不允许此操作，请刷新后选择可用操作。',
  COMPLETION_CHECK_IDEMPOTENCY_CONFLICT: '这次提交与此前的决定不一致，请刷新并确认最新状态。',
};

export function completionCheckErrorCode(error: unknown): string {
  if (!error || typeof error !== 'object') return '';
  const item = error as { reasonCode?: unknown; details?: { reasonCode?: unknown }; payload?: { data?: { reasonCode?: unknown } } };
  const code = item.reasonCode ?? item.details?.reasonCode ?? item.payload?.data?.reasonCode;
  return typeof code === 'string' && Object.hasOwn(safeFeedback, code) ? code : '';
}

export function completionCheckFeedback(error: unknown): string {
  return safeFeedback[completionCheckErrorCode(error)] ?? '完成检查操作未完成，已保留输入，请重试。';
}

export function completionCheckFieldFeedback(error: unknown): string {
  if (!(error instanceof CompletionCheckValidationError)) return completionCheckFeedback(error);
  const labels: Record<string, string> = { command: '检查命令不能为空，且最多 8 KiB。', script: '检查脚本不能为空，且最多 64 KiB。', instructions: '检查说明最多 16 KiB。', referenceAssetIds: '最多关联 64 份资料，且不能重复。', identity: '资料标识无效，请重新添加资料。', mode: '请选择检查方式。' };
  return labels[error.field] ?? completionCheckFeedback(error);
}

export const completionCheckActions = ['CONTINUE_WITH_LIMITATIONS', 'AUTHORIZE_REPAIR', 'ADD_VERIFICATION'] as const;
export type CompletionCheckAction = typeof completionCheckActions[number];
export type CompletionCheckStatus = 'PASSED' | 'FAILED' | 'UNAVAILABLE' | 'NO_TESTS' | 'NOT_CONFIGURED' | 'TIMED_OUT';
export interface CompletionCheckEvidence {
  id: API.Int64; taskId: API.Int64; taskTurnId: API.Int64; attemptId: API.Int64;
  phase: 'BASELINE' | 'CURRENT'; status: CompletionCheckStatus; checkDigest: string; summary: string;
}
export interface CompletionCheckSummary {
  reasonCode?: 'CHECK_DEFINITION_CHANGED' | 'CHECK_DEFINITION_UNAVAILABLE';
  input: CompletionCheck | null; digest: string; baseline: CompletionCheckEvidence | null; currentResult: CompletionCheckEvidence | null;
  availableActions: CompletionCheckAction[]; checkpointReady: boolean; version: API.Int64;
  sourceCheckpointId: API.Int64; expectedCurrentTurnId: API.Int64;
}
export interface CompletionCheckDecision {
  resultId: API.Int64; expectedVersion: API.Int64; action: CompletionCheckAction; reason: string; idempotencyKey: string;
  repairScope?: string; completionCheck?: CompletionCheck;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new CompletionCheckValidationError('response');
  return value as Record<string, unknown>;
}

function digest(value: unknown, optional = false): string {
  if (optional && value === '') return '';
  if (typeof value !== 'string' || !/^[a-f0-9]{64}$/.test(value)) throw new CompletionCheckValidationError('response');
  return value;
}

function normalizeEvidence(value: unknown, phase: CompletionCheckEvidence['phase'], taskId: API.Int64): CompletionCheckEvidence | null {
  if (value === null) return null;
  const item = record(value);
  const statuses = ['PASSED', 'FAILED', 'UNAVAILABLE', 'NO_TESTS', 'NOT_CONFIGURED', 'TIMED_OUT'];
  if (item.phase !== phase || typeof item.status !== 'string' || !statuses.includes(item.status) || item.taskId !== taskId || typeof item.summary !== 'string') throw new CompletionCheckValidationError('response');
  return { id: completionCheckId(item.id), taskId: completionCheckId(item.taskId), taskTurnId: completionCheckId(item.taskTurnId), attemptId: completionCheckId(item.attemptId), phase, status: item.status as CompletionCheckStatus, checkDigest: digest(item.checkDigest), summary: completionCheckText(item.summary, 'response', 16384)! };
}

export function normalizeCompletionCheckSummary(value: unknown, taskId: API.Int64): CompletionCheckSummary {
  completionCheckId(taskId);
  const item = record(value);
  if (typeof item.checkpointReady !== 'boolean' || !Array.isArray(item.availableActions) || item.availableActions.length > 3 || item.availableActions.some((action) => !completionCheckActions.includes(action)) || new Set(item.availableActions).size !== item.availableActions.length) throw new CompletionCheckValidationError('response');
  if (item.reasonCode !== undefined && item.reasonCode !== '' && !['CHECK_DEFINITION_CHANGED', 'CHECK_DEFINITION_UNAVAILABLE'].includes(item.reasonCode as string)) throw new CompletionCheckValidationError('response');
  if (item.reasonCode === 'CHECK_DEFINITION_CHANGED' && item.availableActions.includes('CONTINUE_WITH_LIMITATIONS')) throw new CompletionCheckValidationError('response');
  const currentResult = normalizeEvidence(item.currentResult, 'CURRENT', taskId);
  const sourceCheckpointId = item.sourceCheckpointId === '0' ? '0' : completionCheckId(item.sourceCheckpointId);
  if (item.availableActions.length && (!item.checkpointReady || sourceCheckpointId === '0' || !currentResult)) throw new CompletionCheckValidationError('response');
  return {
    ...(['CHECK_DEFINITION_CHANGED', 'CHECK_DEFINITION_UNAVAILABLE'].includes(item.reasonCode as string) ? { reasonCode: item.reasonCode as CompletionCheckSummary['reasonCode'] } : {}),
    input: item.input === null ? null : normalizeCompletionCheck(item.input), digest: digest(item.digest, item.input === null),
    baseline: normalizeEvidence(item.baseline, 'BASELINE', taskId), currentResult, availableActions: item.availableActions,
    checkpointReady: item.checkpointReady, version: completionCheckId(item.version), sourceCheckpointId, expectedCurrentTurnId: completionCheckId(item.expectedCurrentTurnId),
  };
}

export function normalizeCompletionCheckDecision(value: unknown): CompletionCheckDecision {
  const item = record(value);
  if (!completionCheckActions.includes(item.action as CompletionCheckAction)) throw new CompletionCheckValidationError('action');
  const result: CompletionCheckDecision = {
    resultId: completionCheckId(item.resultId), expectedVersion: completionCheckId(item.expectedVersion), action: item.action as CompletionCheckAction,
    reason: completionCheckText(item.reason, 'reason', 2000, true)!, idempotencyKey: completionCheckText(item.idempotencyKey, 'idempotencyKey', 128, true)!,
  };
  if (result.action === 'AUTHORIZE_REPAIR') result.repairScope = completionCheckText(item.repairScope, 'repairScope', 20000, true);
  else if (item.repairScope !== undefined) throw new CompletionCheckValidationError('repairScope');
  if (result.action === 'ADD_VERIFICATION') {
    result.completionCheck = normalizeCompletionCheck(item.completionCheck);
    if (result.completionCheck.mode === 'NONE') throw new CompletionCheckValidationError('mode');
  } else if (item.completionCheck !== undefined) throw new CompletionCheckValidationError('mode');
  return result;
}

export function normalizeCompletionCheckDecisionResult(value: unknown): { taskTurnId: API.Int64; decisionId: API.Int64; replayed: boolean } {
  const item = record(value);
  if (typeof item.replayed !== 'boolean') throw new CompletionCheckValidationError('response');
  return { taskTurnId: completionCheckId(item.taskTurnId), decisionId: completionCheckId(item.decisionId), replayed: item.replayed };
}
