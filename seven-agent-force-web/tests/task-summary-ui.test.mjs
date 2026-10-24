import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
function compile(path, imports = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { if (!(name in imports)) throw new Error(`Unexpected dependency: ${name}`); return imports[name]; });
  return exports;
}
const executionAccess = compile('../src/api/executionAccessContract.ts');
const delivery = compile('../src/api/deliveryContract.ts');
const conversation = compile('../src/api/conversationContract.ts', { './executionAccessContract': executionAccess, './deliveryContract': delivery });
const contract = compile('../src/api/taskSummaryContract.ts', { './conversationContract': conversation });
const { normalizeTaskConversationSummary, pendingItemLink } = contract;
const taskStatus = compile('../src/api/taskStatusContract.ts');
const { default: Summary } = compile('../src/app/workforce/tasks/TaskActivitySummary.tsx', { 'react/jsx-runtime': require('react/jsx-runtime'), antd: require('antd'), '@/api/taskSummaryContract': contract });
const { renderToStaticMarkup } = require('react-dom/server');
const render = (summary) => renderToStaticMarkup(Summary({ taskId: '9007199254740993', summary, onOpen: () => {} }));

test('batch summary keeps omitted data unknown and lossless item IDs', () => {
  assert.deepEqual(normalizeTaskConversationSummary({ status: 'RUNNING' }), {});
  const summary = normalizeTaskConversationSummary({ pendingActionCount: 2, hasUnreadUpdates: true, activitySummary: '需要确认排序', firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '9007199254740995', private: 'never-copy' });
  assert.deepEqual(summary, { pendingActionCount: 2, hasUnreadUpdates: true, activitySummary: '需要确认排序', firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '9007199254740995' });
  assert.equal(pendingItemLink('9007199254740993', summary), '/workforce/tasks/9007199254740993?item=9007199254740995');
  assert.equal(pendingItemLink('9007199254740993', { pendingActionCount: 1, firstPendingActionKind: 'COMPLETION_CHECK', firstPendingActionId: '9007199254740997' }), '/workforce/tasks/9007199254740993?tab=overview&pending=completion-check&checkpoint=9007199254740997');
  assert.equal(pendingItemLink('9007199254740993', { pendingActionCount: 1, firstPendingActionKind: 'DELIVERY_REQUEST', firstPendingActionId: '9007199254740998' }), '/workforce/tasks/9007199254740993?focus=delivery&delivery=9007199254740998');
  assert.equal(pendingItemLink('1', { pendingActionCount: 0 }), undefined);
  assert.equal(pendingItemLink('1', { firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '2' }), undefined);
  assert.equal(normalizeTaskConversationSummary({ firstPendingActionId: null }).firstPendingActionId, undefined);
});

test('unsafe counts, IDs, booleans and bodies cannot become list actions', () => {
  for (const count of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1, '2', null, Infinity]) assert.throws(() => normalizeTaskConversationSummary({ pendingActionCount: count }));
  for (const id of [2, '0', '02', '9223372036854775808', '/private']) assert.throws(() => normalizeTaskConversationSummary({ firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: id }));
  for (const kind of ['QUESTION', 'OPERATION', '', 1]) assert.throws(() => normalizeTaskConversationSummary({ firstPendingActionKind: kind, firstPendingActionId: '2' }));
  for (const value of ['true', 1, null]) assert.throws(() => normalizeTaskConversationSummary({ hasUnreadUpdates: value }));
  assert.throws(() => normalizeTaskConversationSummary({ activitySummary: { token: 'private' } }));
  assert.throws(() => normalizeTaskConversationSummary({ activitySummary: '中'.repeat(1500) }));
  assert.throws(() => normalizeTaskConversationSummary({ pendingActionCount: 0, firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '2' }));
  assert.throws(() => normalizeTaskConversationSummary({ pendingActionCount: 1, firstPendingActionId: '2' }));
});

test('rendered list presentation treats pending and unread independently', () => {
  const pendingOnly = render({ pendingActionCount: 2, hasUnreadUpdates: false, firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '9' });
  assert.match(pendingOnly, /待你处理 · 2 项/);
  assert.match(pendingOnly, /查看待处理/);
  assert.doesNotMatch(pendingOnly, /新更新/);
  const unreadOnly = render({ pendingActionCount: 0, hasUnreadUpdates: true, activitySummary: '文档已完成' });
  assert.match(unreadOnly, /新更新/);
  assert.match(unreadOnly, /文档已完成/);
  assert.doesNotMatch(unreadOnly, /待你处理|查看待处理/);
  assert.equal(render({}), '');
  assert.match(render({ pendingActionCount: 1, hasUnreadUpdates: true }), /新更新/);
  assert.doesNotMatch(render({ pendingActionCount: 1, hasUnreadUpdates: true }), /查看待处理/);
});

test('server summary text is escaped in HTML and pending button opens one exact item', () => {
  const html = render({ activitySummary: '<script>unsafe()</script>' });
  assert.ok(!html.includes('<script>'));
  assert.match(html, /&lt;script&gt;/);
  let destination; let stopped = false;
  const element = Summary({ taskId: '1', summary: { pendingActionCount: 1, firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '9007199254740995' }, onOpen: (url) => { destination = url; } });
  function find(node) { if (!node || typeof node !== 'object') return undefined; if (node.props?.children === '查看待处理') return node; return [node.props?.children].flat(Infinity).map(find).find(Boolean); }
  const button = find(element);
  assert.ok(button);
  button.props.onClick({ stopPropagation() { stopped = true; } });
  assert.equal(stopped, true);
  assert.equal(destination, '/workforce/tasks/1?item=9007199254740995');
});

test('list adapter projects the batch response using exactly one list call, not one per card', async () => {
  const calls = [];
  const task = { id: '1', name: 'Task', status: 'INTERACTION_SUSPENDED', pendingActionCount: 3, hasUnreadUpdates: false, firstPendingActionKind: 'CONVERSATION_ITEM', firstPendingActionId: '9', activitySummary: '需要回答' };
  const api = compile('../src/api/workforceController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: { items: [task], nextCursor: null } }; } },
    '@/lib/apiId': {}, './completionCheckContract': {}, './taskSummaryContract': contract, './taskStatusContract': taskStatus, './taskArtifactContract': {},
  });
  const list = await api.listWorkforceTasks();
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], '/api/workforce/tasks');
  assert.equal(list.items[0].pendingActionCount, 3);
  assert.equal(list.items[0].firstPendingActionKind, 'CONVERSATION_ITEM');
  assert.equal(list.items[0].firstPendingActionId, '9');
  assert.equal(list.items[0].status, 'INTERACTION_SUSPENDED');
  const component = readFileSync(new URL('../src/app/workforce/tasks/TaskActivitySummary.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(component, /useQuery|fetch\(|getConversation/);
});

test('only durable confirmed suspend status reports saved progress and never aliases manual review', () => {
  assert.equal(taskStatus.normalizeTaskStatus('INTERACTION_SUSPENDED'), 'INTERACTION_SUSPENDED');
  assert.equal(taskStatus.savedInteractionProgress('INTERACTION_SUSPENDED'), '进度已保存');
  for (const status of ['RUNNING', 'MANUAL_REVIEW', 'PAUSING', 'SUSPENDED', 'SUCCEEDED']) assert.equal(taskStatus.savedInteractionProgress(status), undefined);
  for (const status of ['PAUSING', 'SUSPENDED', undefined, {}, 1]) assert.throws(() => taskStatus.normalizeTaskStatus(status));
  const pausing = conversation.normalizeConversationItem({ id: '1', version: '1', type: 'ACTIVITY', createdAt: '2026-09-09T12:00:00Z', body: { text: '执行已暂停，正在保存进度', phase: 'PAUSING', checkpointConfirmed: true } });
  assert.deepEqual(pausing.body, { text: '执行已暂停，正在保存进度', deliveryStatus: undefined });
  assert.equal(taskStatus.savedInteractionProgress('RUNNING'), undefined);
  for (const file of ['page.tsx', '[taskId]/page.tsx']) {
    const source = readFileSync(new URL(`../src/app/workforce/tasks/${file}`, import.meta.url), 'utf8');
    assert.match(source, /INTERACTION_SUSPENDED: \{ label: '等待你回答', color: 'gold' \}/);
    assert.doesNotMatch(source, /INTERACTION_SUSPENDED: \{[^}]*terminal: true/);
  }
});

test('runtime check revisions remain separate from result ADD_VERIFICATION', () => {
  const source = readFileSync(new URL('../src/app/workforce/tasks/CompletionCheckPanel.tsx', import.meta.url), 'utf8');
  assert.match(source, /<CompletionRevisionPanel taskId=\{taskId\}/);
  assert.match(source, /summary.availableActions.map/);
  assert.doesNotMatch(source, /mutationFn:.*completion-check-revisions/);
});

test('cancelled saved messages preserve business status separately from delivery facts', () => {
  const item = { id: '1', version: '2', type: 'USER_MESSAGE', createdAt: '2026-09-09T12:00:00Z', body: { text: '补充内容', status: 'CANCELLED', deliveryStatus: 'SAVED', nativeId: 'secret' } };
  assert.deepEqual(conversation.normalizeConversationItem(item).body, { text: '补充内容', status: 'CANCELLED', deliveryStatus: 'SAVED' });
  assert.equal(conversation.normalizeConversationItem({ ...item, body: { ...item.body, deliveryStatus: 'DELIVERED' } }).body.deliveryStatus, 'DELIVERED');
  assert.throws(() => conversation.normalizeConversationItem({ ...item, body: { ...item.body, status: 'ANSWERED' } }));
});
