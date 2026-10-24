import { CompletionCheckValidationError, completionCheckId, completionCheckText, normalizeCompletionCheck, type CompletionCheck } from './completionCheckContract';

export interface CompletionRevision {
  id: API.Int64; taskId: API.Int64; sourceTaskTurnId: API.Int64; sourceVersion: API.Int64;
  revisionNumber: API.Int64; version: API.Int64; status: 'PENDING' | 'APPLIED' | 'CANCELLED' | 'SUPERSEDED';
  completionCheck: CompletionCheck; baseDigest: string; checkDigest: string;
  sourceBaselineResultId?: API.Int64; appliedTurnId?: API.Int64; createdAt: string; appliedAt?: string; replayed: boolean;
}
export interface RevisionView { expectedVersion: API.Int64; completionCheck: CompletionCheck; pending: CompletionRevision | null; allowedActions: 'REQUEST_REVISION'[] }
export interface RevisionRequest { expectedVersion: API.Int64; idempotencyKey: string; completionCheck: CompletionCheck }
export interface RevisionDraft { instructions?: string; mode?: CompletionCheck['mode']; command?: string; script?: string; referenceAssetIds?: API.Int64[] }
const invalid = () => { throw new CompletionCheckValidationError('revision'); };
const record = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : invalid();
const digest = (value: unknown): string => typeof value === 'string' && /^[a-f0-9]{64}$/.test(value) ? value : invalid();
const date = (value: unknown): string => typeof value === 'string' && value.length <= 40 && /^\d{4}-\d\d-\d\dT/.test(value) && Number.isFinite(Date.parse(value)) ? value : invalid();

export function normalizeCompletionRevision(value: unknown, taskId: API.Int64): CompletionRevision {
  const r = record(value);
  if (r.taskId !== completionCheckId(taskId) || !['PENDING', 'APPLIED', 'CANCELLED', 'SUPERSEDED'].includes(r.status as string) || typeof r.replayed !== 'boolean') return invalid();
  const result: CompletionRevision = {
    id: completionCheckId(r.id), taskId, sourceTaskTurnId: completionCheckId(r.sourceTaskTurnId), sourceVersion: completionCheckId(r.sourceVersion),
    revisionNumber: completionCheckId(r.revisionNumber), version: completionCheckId(r.version), status: r.status as CompletionRevision['status'],
    completionCheck: normalizeCompletionCheck(r.completionCheck), baseDigest: digest(r.baseDigest), checkDigest: digest(r.checkDigest), createdAt: date(r.createdAt), replayed: r.replayed,
    ...(r.sourceBaselineResultId !== undefined ? { sourceBaselineResultId: completionCheckId(r.sourceBaselineResultId) } : {}),
    ...(r.appliedTurnId !== undefined ? { appliedTurnId: completionCheckId(r.appliedTurnId) } : {}),
    ...(r.appliedAt !== undefined ? { appliedAt: date(r.appliedAt) } : {}),
  };
  if (result.status === 'APPLIED' && (!result.appliedTurnId || !result.appliedAt)) return invalid();
  if (result.status === 'PENDING' && (result.appliedTurnId || result.appliedAt)) return invalid();
  return result;
}
export function normalizeRevisionView(value: unknown, taskId: API.Int64): RevisionView {
  const r = record(value);
  if (!Array.isArray(r.allowedActions) || r.allowedActions.length > 16 || r.allowedActions.some((a) => typeof a !== 'string')) return invalid();
  const pending = r.pending === null ? null : normalizeCompletionRevision(r.pending, taskId);
  if (pending && pending.status !== 'PENDING') return invalid();
  const allowedActions: RevisionView['allowedActions'] = r.allowedActions.includes('REQUEST_REVISION') ? ['REQUEST_REVISION'] : [];
  if (pending && allowedActions.length) return invalid();
  return { expectedVersion: completionCheckId(r.expectedVersion), completionCheck: normalizeCompletionCheck(r.completionCheck), pending, allowedActions };
}
export function normalizeRevisionRequest(value: unknown): RevisionRequest {
  const r = record(value);
  return { expectedVersion: completionCheckId(r.expectedVersion), idempotencyKey: completionCheckText(r.idempotencyKey, 'idempotencyKey', 128, true)!, completionCheck: normalizeCompletionCheck(r.completionCheck) };
}

// The editor authors additions, never editable copies of frozen executable/text.
export function buildRevisionCheck(base: CompletionCheck, draft: RevisionDraft, availableReferenceIds: API.Int64[]): CompletionCheck {
  const original = normalizeCompletionCheck(base);
  const append = (old: string | undefined, extra: string | undefined) => extra?.trim() ? [old, extra.trim()].filter(Boolean).join('\n') : old;
  const added = draft.referenceAssetIds ?? [];
  if (added.some((id) => !availableReferenceIds.includes(completionCheckId(id)))) return invalid();
  const result = normalizeCompletionCheck({ ...original,
    ...(original.mode === 'NONE' ? { mode: draft.mode ?? 'NONE', command: draft.command, script: draft.script } : {}),
    instructions: append(original.instructions, draft.instructions),
    referenceAssetIds: [...new Set([...(original.referenceAssetIds ?? []), ...added])],
  });
  if (JSON.stringify(result) === JSON.stringify(original)) return invalid();
  return result;
}

export function normalizeRevisionReferences(value: unknown, taskId: API.Int64): { id: API.Int64; label: string }[] {
  const r = record(value);
  if (r.taskId !== completionCheckId(taskId) || !Array.isArray(r.references) || r.references.length > 64) return invalid();
  const refs = r.references.map((value) => { const item = record(value); return { id: completionCheckId(item.id), label: completionCheckText(item.displayName, 'reference', 4096, true)! }; });
  if (new Set(refs.map((ref) => ref.id)).size !== refs.length) return invalid();
  return refs;
}

const feedback: Record<string, string> = {
  COMPLETION_CHECK_INVALID: '调整内容无效。请保留原有检查，并补充说明或资料。',
  COMPLETION_CHECK_CONFLICT: '已有调整或任务状态发生变化，请刷新状态后重试。',
  COMPLETION_CHECK_STALE: '任务已更新，请刷新并确认最新检查后再提交。',
  COMPLETION_CHECK_ACTION_NOT_ALLOWED: '当前任务不允许调整检查，请刷新状态。',
  COMPLETION_CHECK_IDEMPOTENCY_CONFLICT: '这次提交与此前的调整不一致，请刷新状态。',
  COMPLETION_CHECK_NOT_FOUND: '当前检查不可用，请刷新状态后重试。',
  COMPLETION_CHECK_REVISION_UNAVAILABLE: '暂不支持调整后续检查。',
};
export function revisionFeedback(error: unknown): string {
  const safeRecord = (value: unknown) => value && typeof value === 'object' && !Array.isArray(value) ? record(value) : {};
  const r = safeRecord(error);
  const details = safeRecord(r.details);
  const payload = safeRecord(r.payload);
  const data = safeRecord(payload.data);
  const code = r.reasonCode ?? details.reasonCode ?? data.reasonCode;
  return typeof code === 'string' && Object.hasOwn(feedback, code) ? feedback[code] : '检查调整未完成，已保留输入，请重试。';
}
