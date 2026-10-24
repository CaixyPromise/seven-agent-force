import { normalizeExecutionOperationSummary, type ExecutionOperationSummary } from './executionAccessContract';
import { normalizeDeliveryConversationCard, type DeliveryConversationCard } from './deliveryContract';

/** Batch 3 wire boundary. Unknown bodies never become controls or rendered JSON. */
export type Answer = { questionKey: string; selectedOptionKeys: string[]; text: string };
export type Question = { key: string; title: string; description: string; kind: 'SINGLE_CHOICE' | 'MULTIPLE_CHOICE' | 'TEXT'; required: boolean; allowFreeText: boolean; options: { key: string; label: string }[] };
export type QuestionBody = { interactionId: string; status: string; version: string; isBlocking: boolean; questions: Question[]; response?: { id: string; answers: Answer[] }; deliveryStatus?: string };
export type ConversationItem = { id: string; version: string; createdAt: string } & (
  | { type: 'QUESTION'; body: QuestionBody }
  | { type: 'OPERATION'; body: ExecutionOperationSummary }
  | { type: 'DELIVERY'; body: DeliveryConversationCard }
  | { type: 'USER_MESSAGE' | 'ASSISTANT_MESSAGE' | 'ACTIVITY' | 'RESULT'; body: { text: string; deliveryStatus?: string; status?: string } }
  | { type: 'UNSUPPORTED'; body: null }
);
export type ConversationPage = { items: ConversationItem[]; nextCursor: string | null; watermark: string; allowedActions: string[] };
export class ConversationContractError extends Error { constructor() { super('CONVERSATION_INVALID'); } }
function invalid(): never { throw new ConversationContractError(); }
export function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return invalid();
  return value as Record<string, unknown>;
}
export function decimal(value: unknown, zero = false): string {
  if (typeof value !== 'string' || !(zero ? /^(0|[1-9]\d*)$/ : /^[1-9]\d*$/).test(value) || value.length > 19 || (value.length === 19 && value > '9223372036854775807')) return invalid();
  return value;
}
export function conversationText(value: unknown, max = 16384): string {
  if (typeof value !== 'string' || value.includes('\0') || /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/u.test(value) || new TextEncoder().encode(value).length > max) return invalid();
  return value;
}
const string = conversationText;
function key(value: unknown): string { const result = string(value, 128); return result.trim() === result && result.length ? result : invalid(); }
function label(value: unknown): string { const result = string(value, 512); return result.trim().length ? result : invalid(); }
function bool(value: unknown): boolean { return typeof value === 'boolean' ? value : invalid(); }
function array(value: unknown, max: number): unknown[] { return Array.isArray(value) && value.length <= max ? value : invalid(); }
function unique<T>(values: T[], identity: (value: T) => string): T[] { if (new Set(values.map(identity)).size !== values.length) return invalid(); return values; }
const deliveryStatuses = ['NONE', 'SAVED', 'DELIVERED'];
function delivery(value: unknown): string | undefined { if (value === undefined) return undefined; return typeof value === 'string' && deliveryStatuses.includes(value) ? value : invalid(); }
export function normalizeAnswers(value: unknown): Answer[] {
  const answers = unique(array(value, 3).map((value) => { const a = record(value); return { questionKey: key(a.questionKey), selectedOptionKeys: unique(array(a.selectedOptionKeys, 8).map(key), (v) => v), text: string(a.text) }; }), (a) => a.questionKey);
  conversationText(JSON.stringify(answers));
  return answers;
}
function question(value: unknown): Question {
  const q = record(value);
  if (q.kind !== 'SINGLE_CHOICE' && q.kind !== 'MULTIPLE_CHOICE' && q.kind !== 'TEXT') return invalid();
  const options = unique(array(q.options, 8).map((value) => { const o = record(value); return { key: key(o.key), label: label(o.label) }; }), (o) => o.key);
  if (q.kind === 'TEXT' ? options.length > 0 : options.length === 0) return invalid();
  return { key: key(q.key), title: label(q.title), description: string(q.description, 4096), kind: q.kind, required: bool(q.required), allowFreeText: bool(q.allowFreeText), options };
}
export function normalizeConversationItem(value: unknown): ConversationItem {
  const item = record(value);
  const base = { id: decimal(item.id), version: decimal(item.version), createdAt: string(item.createdAt, 64) };
  if (!/^\d{4}-\d{2}-\d{2}T/.test(base.createdAt) || !Number.isFinite(Date.parse(base.createdAt))) return invalid();
  const type = key(item.type);
  if (type === 'QUESTION') {
    const b = record(item.body);
    const status = key(b.status);
    if (!['OPEN', 'RESOLVED', 'EXPIRED', 'CANCELLED', 'SUPERSEDED'].includes(status)) return invalid();
    const questions = unique(array(b.questions, 3).map(question), (q) => q.key);
    if (!questions.length) return invalid();
    const body: QuestionBody = { interactionId: decimal(b.interactionId), version: decimal(b.version), status, isBlocking: bool(b.isBlocking), questions, deliveryStatus: delivery(b.deliveryStatus) };
    if (b.response !== undefined) { const response = record(b.response); body.response = { id: decimal(response.id), answers: normalizeAnswers(response.answers) }; if (validateAnswers(questions, body.response.answers)) return invalid(); }
    return { ...base, type, body };
  }
  if (type === 'OPERATION') return { ...base, type, body: normalizeExecutionOperationSummary(item.body) };
  if (type === 'DELIVERY') return { ...base, type, body: normalizeDeliveryConversationCard(item.body) };
  if (type === 'USER_MESSAGE' || type === 'ASSISTANT_MESSAGE' || type === 'ACTIVITY' || type === 'RESULT') {
    const b = record(item.body);
    const status = type === 'USER_MESSAGE' && b.status !== undefined ? key(b.status) : undefined;
    if (status !== undefined && !['OPEN', 'RESOLVED', 'EXPIRED', 'CANCELLED', 'SUPERSEDED'].includes(status)) return invalid();
    return { ...base, type, body: { text: string(b.text), deliveryStatus: delivery(b.deliveryStatus), ...(status !== undefined ? { status } : {}) } };
  }
  return { ...base, type: 'UNSUPPORTED', body: null };
}
export function normalizeConversationPage(value: unknown): ConversationPage {
  const p = record(value);
  return { items: unique(array(p.items, 50).map(normalizeConversationItem), (i) => i.id), nextCursor: p.nextCursor === null ? null : decimal(p.nextCursor), watermark: decimal(p.watermark, true), allowedActions: unique(array(p.allowedActions, 30).map(key), (v) => v).filter((a) => ['SEND_MESSAGE', 'ANSWER', 'DECIDE_OPERATION', 'DECIDE_DELIVERY', 'RESTART_DELIVERY'].includes(a)) };
}
export function compareDecimal(a: string, b: string): number { return a.length - b.length || a.localeCompare(b); }
export function mergeConversation(current: ConversationItem[], incoming: ConversationItem[]): ConversationItem[] {
  const items = new Map(current.map((i) => [i.id, i]));
  for (const item of incoming) { const old = items.get(item.id); if (!old || compareDecimal(item.version, old.version) > 0) items.set(item.id, item); }
  return [...items.values()].sort((a, b) => compareDecimal(a.id, b.id));
}
export function validateAnswers(questions: Question[], answers: Answer[]): string | undefined {
  try { normalizeAnswers(answers); } catch { return '回答内容无效或过长，请缩短后重试。'; }
  if (answers.some((a) => !questions.some((q) => q.key === a.questionKey))) return '回答与当前问题不匹配';
  for (const q of questions) {
    const a = answers.find((a) => a.questionKey === q.key);
    const choices = a?.selectedOptionKeys ?? [];
    const text = a?.text.trim() ?? '';
    if (q.required && !choices.length && !text) return `请回答：${q.title}`;
    if (new Set(choices).size !== choices.length || choices.some((k) => !q.options.some((o) => o.key === k)) || (q.kind !== 'MULTIPLE_CHOICE' && choices.length > 1) || (q.kind === 'TEXT' && choices.length) || (q.kind !== 'TEXT' && !q.allowFreeText && a?.text)) return `请检查：${q.title}`;
  }
}
export function conversationFeedback(error: unknown, operation = '对话加载'): string {
  let code: unknown;
  try { const e = record(error); const p = e.payload ? record(e.payload) : e; const data = p.data ? record(p.data) : p; code = data.reasonCode; } catch { /* Unknown payloads stay private. */ }
  const messages: Record<string, string> = { INTERACTION_INVALID: '提交内容不符合要求，请检查当前问题与回答。', INTERACTION_NOT_FOUND: '这项请求不存在或暂不可访问。', INTERACTION_STALE: '这项请求已经变化，请刷新查看当前请求。草稿已保留。', INTERACTION_IDEMPOTENCY_CONFLICT: '这次提交与已保存的内容不一致，请刷新查看当前请求。草稿已保留。', INTERACTION_ACTION_NOT_ALLOWED: '当前无法执行这项操作，请刷新查看当前请求。草稿已保留。', INTERACTION_UNAVAILABLE: '当前操作暂不可用，请稍后重试。', INTERACTION_FENCED: '这项请求已失效，请刷新查看当前请求。草稿已保留。', CAPABILITY_NOT_SUPPORTED: '当前环境暂不支持这项操作。', GRANT_REVOKED: '访问授权已撤销。', DELIVERY_RESULT_CHANGED: '待交付内容已变化，请重新确认。' };
  return typeof code === 'string' && messages[code] ? messages[code] : `${operation}未完成，请重试。`;
}
