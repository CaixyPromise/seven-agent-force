import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

function module(path, imports = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { if (!(name in imports)) throw new Error(`Unexpected dependency ${name}`); return imports[name]; });
  return exports;
}
const executionAccess = module('../src/api/executionAccessContract.ts');
const delivery = module('../src/api/deliveryContract.ts');
const contract = module('../src/api/conversationContract.ts', { './executionAccessContract': executionAccess, './deliveryContract': delivery });
const { normalizeConversationPage, normalizeConversationItem, normalizeAnswers, validateAnswers, mergeConversation, decimal, conversationFeedback } = contract;
const { ConversationDrafts } = module('../src/app/workforce/tasks/conversationDrafts.ts');
const { consumeEventFrames } = module('../src/api/conversationStream.ts', { './conversationContract': contract });
const { presentTaskEvents } = module('../src/app/workforce/tasks/taskEventPresentation.ts');
const question = { key: 'direction', title: '选择方向', description: '', kind: 'SINGLE_CHOICE', required: true, allowFreeText: false, options: [{ key: 'a', label: '方向 A' }, { key: 'b', label: '方向 B' }] };
const item = (id = '9007199254740993', version = '1') => ({ id, version, type: 'QUESTION', createdAt: '2026-09-09T12:00:00Z', body: { interactionId: '9007199254740995', status: 'OPEN', version, isBlocking: true, questions: [structuredClone(question)] } });
const page = (items = [item()]) => ({ items, nextCursor: null, watermark: '9007199254740998', allowedActions: ['SEND_MESSAGE', 'ANSWER'] });

test('normalizers preserve BIGINT identities and versions without coercion', () => {
  assert.equal(normalizeConversationPage(page()).items[0].id, '9007199254740993');
  for (const value of [1, 9007199254740992, '01', ' 1', '-1', '1e2', '9223372036854775808', null, undefined]) assert.throws(() => decimal(value));
  assert.equal(decimal('0', true), '0');
  assert.throws(() => decimal('0'));
  assert.equal(decimal('9223372036854775807'), '9223372036854775807');
});

test('page fails closed on missing authority, bad cursor, oversized or duplicate items', () => {
  for (const patch of [{ allowedActions: undefined }, { watermark: 4 }, { nextCursor: '' }, { items: [item(), item()] }, { items: Array.from({ length: 51 }, (_, i) => item(String(i + 1))) }]) assert.throws(() => normalizeConversationPage({ ...page(), ...patch }));
  assert.deepEqual(normalizeConversationPage({ ...page(), allowedActions: ['APPROVE_EVERYTHING'] }).allowedActions, []);
});

test('unknown item type drops untrusted body and cannot become a question control', () => {
  const result = normalizeConversationItem({ ...item(), type: 'APPROVE', body: { html: '<button>grant</button>', token: 'secret' } });
  assert.equal(result.type, 'UNSUPPORTED');
  assert.equal(result.body, null);
});

test('structured questions reject malformed enums, options, requirements and versions', () => {
  for (const patch of [{ kind: 'PASSWORD' }, { required: 'true' }, { allowFreeText: null }, { options: [{ key: 'a', label: 'A' }, { key: 'a', label: 'B' }] }, { kind: 'TEXT' }]) {
    const value = item(); Object.assign(value.body.questions[0], patch); assert.throws(() => normalizeConversationItem(value));
  }
  for (const patch of [{ version: 2 }, { interactionId: 8 }, { status: 'GUESS' }, { isBlocking: 'true' }, { questions: [] }]) assert.throws(() => normalizeConversationItem({ ...item(), body: { ...item().body, ...patch } }));
  assert.equal(normalizeConversationItem({ ...item(), body: { ...item().body, isBlocking: false } }).body.isBlocking, false);
});

test('frozen public status/actions and safe response DTO keep saved distinct from delivered', () => {
  for (const status of ['OPEN', 'RESOLVED', 'EXPIRED', 'CANCELLED', 'SUPERSEDED']) assert.equal(normalizeConversationItem({ ...item(), body: { ...item().body, status } }).body.status, status);
  for (const status of ['ANSWERED', 'INVALIDATED']) assert.throws(() => normalizeConversationItem({ ...item(), body: { ...item().body, status } }));
  assert.deepEqual(normalizeConversationPage({ ...page(), allowedActions: ['RESPOND', 'ANSWER'] }).allowedActions, ['ANSWER']);
  const body = { ...item().body, status: 'RESOLVED', deliveryStatus: 'SAVED', response: { id: '8', answers: [{ questionKey: 'direction', selectedOptionKeys: ['a'], text: '' }], nativeId: 'private' }, attemptId: 'private' };
  const saved = normalizeConversationItem({ ...item(), body });
  assert.equal(saved.body.deliveryStatus, 'SAVED');
  assert.equal(saved.body.response.id, '8');
  assert.ok(!JSON.stringify(saved).includes('private'));
  assert.equal(normalizeConversationItem({ ...item(), body: { ...body, deliveryStatus: 'DELIVERED' } }).body.deliveryStatus, 'DELIVERED');
  assert.throws(() => normalizeConversationItem({ ...item(), body: { ...body, deliveryStatus: 'PENDING' } }));
});

test('UTF-8 bounded inputs reject oversize answers, malformed unicode and excessive questions', () => {
  assert.throws(() => contract.conversationText('中'.repeat(6000)));
  assert.throws(() => contract.conversationText('\ud800'));
  assert.throws(() => contract.conversationText('secret\0'));
  assert.throws(() => normalizeConversationItem({ ...item(), body: { ...item().body, questions: Array.from({ length: 4 }, (_, i) => ({ ...question, key: String(i) })) } }));
  assert.ok(validateAnswers([{ ...question, kind: 'TEXT', options: [] }], [{ questionKey: 'direction', selectedOptionKeys: [], text: '中'.repeat(6000) }]));
});

test('answer behavior covers required single, multiple, optional and free text', () => {
  assert.match(validateAnswers([question], []), /请回答/);
  const a = { questionKey: 'direction', selectedOptionKeys: ['a'], text: '' };
  assert.equal(validateAnswers([question], [a]), undefined);
  assert.ok(validateAnswers([question], [{ ...a, selectedOptionKeys: ['a', 'b'] }]));
  assert.ok(validateAnswers([question], [{ ...a, selectedOptionKeys: ['forged'] }]));
  assert.ok(validateAnswers([question], [{ ...a, text: 'not allowed' }]));
  assert.equal(validateAnswers([{ ...question, kind: 'MULTIPLE_CHOICE' }], [{ ...a, selectedOptionKeys: ['a', 'b'] }]), undefined);
  assert.equal(validateAnswers([{ ...question, allowFreeText: true }], [{ ...a, selectedOptionKeys: [], text: 'another direction' }]), undefined);
  assert.equal(validateAnswers([{ ...question, required: false }], []), undefined);
  assert.throws(() => normalizeAnswers([a, a]));
});

test('pagination and out-of-order updates keep one stable question and newer version', () => {
  const initial = [normalizeConversationItem(item('9007199254740995', '10'))];
  const merged = mergeConversation(initial, [normalizeConversationItem(item('9007199254740995', '2')), normalizeConversationItem(item('9007199254740993'))]);
  assert.deepEqual(merged.map((i) => i.id), ['9007199254740993', '9007199254740995']);
  assert.equal(merged[1].version, '10');
  const update = item('9007199254740995', '11'); update.body.status = 'RESOLVED';
  assert.equal(mergeConversation(merged, [normalizeConversationItem(update)])[1].body.status, 'RESOLVED');
});

test('drafts survive virtual remount, preserve failed submission key, and isolate versions/tasks', () => {
  const drafts = new ConversationDrafts(); drafts.setScope('user:org');
  const k = 'task:interaction:1';
  drafts.set(k, { text: 'draft', answers: [{ questionKey: 'direction', selectedOptionKeys: ['a'], text: '' }] });
  assert.equal(drafts.submission(k, 'body', () => 'key-1'), 'key-1');
  assert.equal(drafts.submission(k, 'body', () => 'must-not-be-used'), 'key-1');
  assert.equal(drafts.get(k).text, 'draft');
  assert.equal(drafts.get('task:interaction:2').text, '');
  assert.equal(drafts.get('other-task:interaction:1').text, '');
  assert.equal(drafts.submission(k, 'changed-body', () => 'key-2'), 'key-2');
  drafts.clear(k); assert.equal(drafts.get(k).text, '');
});

test('logout and organization changes erase private drafts across unmounted tasks', () => {
  const drafts = new ConversationDrafts(); drafts.setScope('user:org1'); drafts.set('task', { text: 'private', answers: [] });
  drafts.setScope('user:org1'); assert.equal(drafts.get('task').text, 'private');
  drafts.setScope('user:org2'); assert.equal(drafts.get('task').text, '');
  drafts.set('task', { text: 'private', answers: [] }); drafts.setScope(''); assert.equal(drafts.get('task').text, '');
});

test('SSE supports split frames, CRLF, duplicates and lossless reconnect cursors', () => {
  const first = consumeEventFrames('id: 9007199254740993\r\nevent: changed\r\ndata: {}\r\n', '0');
  assert.equal(first.after, '0');
  const second = consumeEventFrames(`${first.rest}\r\n`, first.after);
  assert.equal(second.after, '9007199254740993');
  assert.equal(consumeEventFrames('id: 2\ndata: {}\n\n: reconnect\n\n', second.after).after, second.after);
  assert.throws(() => consumeEventFrames('id: 1e30\n\n', '0'));
  assert.throws(() => consumeEventFrames('event: error\ndata: {"token":"private"}\n\n', '0'));
  assert.throws(() => consumeEventFrames('x'.repeat(1024 * 1024 + 1), '0'));
});

test('safe errors classify stable reason codes and never expose upstream messages', () => {
  const secret = 'token=secret /opt/private SQL stack http://internal';
  for (const error of [new Error(secret), { message: secret }, { payload: { data: { reasonCode: secret }, message: secret } }, null]) assert.equal(conversationFeedback(error, '回答提交'), '回答提交未完成，请重试。');
  assert.match(conversationFeedback({ payload: { data: { reasonCode: 'INTERACTION_STALE' }, message: secret } }), /草稿已保留/);
});

test('SSE change metadata locates old cards without interpreting event body as authority', () => {
  const parsed = consumeEventFrames('id: 99\nevent: conversation.updated\ndata: {"payload":{"itemId":"9007199254740993","allowedActions":["FORGED"]}}\n\n', '98');
  assert.deepEqual(parsed.itemIds, ['9007199254740993']);
  assert.equal(parsed.after, '99');
  assert.throws(() => consumeEventFrames('id: 99\nevent: conversation.updated\ndata: {"payload":{"itemId":9007199254740993}}\n\n', '98'));
});

test('run history hides conversation invalidations by default and groups them for technical diagnostics', () => {
  const events = [
    { sequence: 44, type: 'conversation.updated', createdAt: '2026-09-12T23:39:24Z', payload: { kind: 'ACTIVITY', itemId: '8000000000000000009', version: '1' } },
    { sequence: 43, type: 'conversation.updated', createdAt: '2026-09-12T23:39:16Z', payload: { kind: 'ACTIVITY', itemId: '8000000000000000008', version: '1' } },
    { sequence: 42, type: 'task.validating', createdAt: '2026-09-12T23:39:15Z', payload: {} },
    { sequence: 41, type: 'conversation.updated', createdAt: '2026-09-12T23:39:11Z', payload: { kind: 'ASSISTANT_MESSAGE', itemId: '8000000000000000007', version: '1' } },
  ];
  const business = presentTaskEvents(events, false);
  assert.deepEqual(business.map((row) => row.kind === 'event' && row.event.type), ['task.validating']);

  const technical = presentTaskEvents(events, true);
  assert.deepEqual(technical.map((row) => row.kind), ['conversation-sync', 'event', 'conversation-sync']);
  assert.equal(technical[0].summary, 'Agent 活动 2');
  assert.equal(technical[0].events.length, 2);
  assert.equal(technical[0].technicalFields[0].itemId, '8000000000000000009');
  assert.equal(technical[2].summary, 'Agent 消息 1');
});

test('POST adapters validate identities and preserve caller idempotency and expectedVersion', async () => {
  const calls = [];
  const api = module('../src/api/conversationController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: page() }; } },
    '@/store/auth': {}, '@/lib/auth/token': {}, '@/lib/auth/device': {}, './conversationContract': contract, './conversationStream': { consumeEventFrames },
  });
  await api.respondToInteraction('1', '9007199254740995', { expectedVersion: '9007199254740994', idempotencyKey: 'retry-key', answers: [{ questionKey: 'direction', selectedOptionKeys: ['a'], text: '' }] });
  assert.equal(calls[0][0], '/api/workforce/tasks/1/interactions/9007199254740995/responses');
  assert.equal(calls[0][1].data.expectedVersion, '9007199254740994');
  assert.equal(calls[0][1].data.idempotencyKey, 'retry-key');
  await assert.rejects(() => api.sendConversationMessage('1', ' ', 'key'));
  await assert.rejects(() => api.getConversation(1));
  await api.markConversationRead('1', '0');
  assert.deepEqual(calls[1][1].data, { watermark: '0' });
});

test('workspace structure preserves stable editor, automatic read and separated workflow results', () => {
  const source = readFileSync(new URL('../src/app/workforce/tasks/TaskConversation.tsx', import.meta.url), 'utf8');
  assert.ok(source.indexOf('<QuestionEditor') > source.indexOf('</VirtualList>'));
  assert.match(source, /document.visibilityState !== 'visible'/);
  assert.match(source, /event\.nativeEvent\.isComposing/);
  assert.match(source, /event\.shiftKey/);
  assert.match(source, /markConversationRead/);
  assert.match(source, /onResults/);
  assert.doesNotMatch(source, /ConversationBusinessCards/);
  assert.doesNotMatch(source, /dangerouslySetInnerHTML|localStorage|sessionStorage/);
  const detail = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  for (const component of ['TaskConversation', 'IndependentReviewWorkspace', 'CompletionCheckPanel', 'DeliveryWorkspace']) assert.equal((detail.match(new RegExp(`<${component}\\b`, 'g')) ?? []).length, 1);
  for (const label of ['工作流', '结果', '详情']) assert.match(detail, new RegExp(`label: '${label}'`));
  const checks = readFileSync(new URL('../src/app/workforce/tasks/CompletionCheckPanel.tsx', import.meta.url), 'utf8');
  assert.match(checks, /summary.availableActions.map/);
  assert.match(checks, /current.availableActions.includes\(action\)/);
});
