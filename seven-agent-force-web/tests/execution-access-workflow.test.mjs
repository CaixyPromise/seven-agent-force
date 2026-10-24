import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

function compile(path, imports = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { if (!(name in imports)) throw new Error(`Unexpected dependency: ${name}`); return imports[name]; });
  return exports;
}

const access = compile('../src/api/executionAccessContract.ts');
const delivery = compile('../src/api/deliveryContract.ts');
const conversation = compile('../src/api/conversationContract.ts', { './executionAccessContract': access, './deliveryContract': delivery });

const summary = (patch = {}) => ({
  operationId: '9007199254740995', version: '3', status: 'WAITING_APPROVAL', purpose: '安装锁定依赖',
  targets: [{ host: 'registry.npmjs.org', protocol: 'https', port: 443 }], agentContinued: false, ...patch,
});
const detail = (patch = {}) => ({ ...summary(), id: '9007199254740995', taskId: '9007199254740993', command: 'pnpm install --frozen-lockfile', cwd: '.', createdAt: '2026-09-11T12:00:00Z', ...patch });

test('operation conversation items preserve only the safe projection', () => {
  const item = conversation.normalizeConversationItem({ id: '8', version: '2', type: 'OPERATION', createdAt: '2026-09-11T12:00:00Z', body: { ...summary(), command: 'secret command', workerToken: 'private' } });
  assert.equal(item.type, 'OPERATION');
  assert.deepEqual(item.body.targets, [{ host: 'registry.npmjs.org', protocol: 'https', port: 443 }]);
  assert.ok(!JSON.stringify(item).includes('secret command'));
  assert.ok(!JSON.stringify(item).includes('private'));
  assert.deepEqual(conversation.normalizeConversationPage({ items: [item], nextCursor: null, watermark: '8', allowedActions: ['DECIDE_OPERATION', 'FORGED'] }).allowedActions, ['DECIDE_OPERATION']);
});

test('operation and grant normalizers reject imprecise IDs, unsafe targets and unknown states', () => {
  assert.equal(access.normalizeExecutionOperationDetail(detail()).id, '9007199254740995');
  for (const patch of [
    { id: 9007199254740995 },
    { status: 'SUCCESS' },
    { cwd: '../private' },
    { targets: [{ host: '127.0.0.1', protocol: 'https', port: 443 }] },
    { targets: [{ host: 'registry.npmjs.org', protocol: 'tcp', port: 443 }] },
    { targets: [{ host: 'registry.npmjs.org', protocol: 'https', port: 70000 }] },
  ]) assert.throws(() => access.normalizeExecutionOperationDetail(detail(patch)));
  const grant = access.normalizeTaskNetworkGrant({
    id: '1', taskId: '2', version: '1', status: 'ACTIVE',
    targets: [{ host: 'registry.npmjs.org', protocol: 'https', port: 443 }],
    createdAt: '2026-09-11T12:00:00Z', actorUserId: '9007199254740993', purpose: '安装锁定依赖',
    lastOperation: { status: 'SUCCEEDED', resultSummary: '依赖已安装', resultDeliveryStatus: 'NATIVE_CALL_RESOLVED', exitCode: 0, agentContinued: true, finishedAt: '2026-09-11T12:05:00Z' },
  });
  assert.equal(grant.actorUserId, '9007199254740993');
  assert.equal(grant.lastOperation.agentContinued, true);
  assert.throws(() => access.normalizeTaskNetworkGrant({ id: '1', taskId: '2', version: '1', status: 'ACTIVE', targets: [], createdAt: '2026-09-11T12:00:00Z' }));
  assert.throws(() => access.normalizeTaskNetworkGrant({ ...grant, actorUserId: 9007199254740993 }));
});

test('result delivery states remain distinct from operation and agent completion', () => {
  assert.equal(access.normalizeExecutionOperationSummary(summary({ status: 'SUCCEEDED', resultDeliveryStatus: 'SAVED', result: { exitCode: 0, summary: '依赖已安装' } })).agentContinued, false);
  assert.equal(access.normalizeExecutionOperationSummary(summary({ status: 'SUCCEEDED', resultDeliveryStatus: 'DELIVERED_TO_ADAPTER' })).resultDeliveryStatus, 'DELIVERED_TO_ADAPTER');
  assert.equal(access.normalizeExecutionOperationSummary(summary({ status: 'SUCCEEDED', resultDeliveryStatus: 'NATIVE_CALL_RESOLVED', agentContinued: undefined })).agentContinued, false);
  assert.equal(access.normalizeExecutionOperationSummary(summary({ status: 'SUCCEEDED', resultDeliveryStatus: 'NATIVE_CALL_RESOLVED', agentContinued: true })).agentContinued, true);
  assert.throws(() => access.normalizeExecutionOperationSummary(summary({ resultDeliveryStatus: 'DONE' })));
  assert.deepEqual(access.normalizeExecutionOperationReceipt({ operationId: '9', grantId: '0', version: '2', status: 'READY', replayed: true }), { operationId: '9', grantId: undefined, version: '2', status: 'READY', replayed: true });
  assert.deepEqual(access.normalizeTaskNetworkGrantReceipt({ grantId: '7', version: '3', status: 'REVOKED', replayed: false }), { grantId: '7', version: '3', status: 'REVOKED', replayed: false });
});

test('operation card never infers Agent continuation from native resolution', () => {
  const card = readFileSync(new URL('../src/app/workforce/tasks/NetworkOperationCard.tsx', import.meta.url), 'utf8');
  assert.match(card, /if \(item\.agentContinued\) return 'Agent 已继续'/);
  assert.match(card, /NATIVE_CALL_RESOLVED'\) return '结果已返回 Agent'/);
  assert.match(card, /DELIVERED_TO_ADAPTER'\) return '结果已送达 Adapter'/);
  assert.match(card, /SAVED'\) return '结果已保存，等待发送'/);
  assert.doesNotMatch(card, /agentContinued\s*\|\|\s*item\.resultDeliveryStatus/);
});

test('known errors are actionable and unknown payloads never render raw messages', () => {
  assert.match(access.executionAccessFeedback({ payload: { data: { reasonCode: 'EXECUTION_OPERATION_STALE' } } }), /已经变化/);
  const secret = 'token=secret SQL stack /opt/private';
  for (const error of [new Error(secret), { message: secret }, { payload: { data: { reasonCode: secret }, message: secret } }]) {
    const feedback = access.executionAccessFeedback(error, '访问决定');
    assert.equal(feedback, '访问决定未完成，请重试。');
    assert.ok(!feedback.includes(secret));
  }
});

test('controllers send exact decimal strings, actions and idempotency identities', async () => {
  const calls = [];
  const controller = compile('../src/api/executionAccessController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: { operationId: '9007199254740995', version: '4', status: 'READY', replayed: false } }; } },
    './conversationContract': conversation,
    './executionAccessContract': access,
  });
  await controller.decideExecutionOperation('9007199254740993', '9007199254740995', { expectedVersion: '3', action: 'ALLOW_ONCE', reason: '', idempotencyKey: 'retry-key' });
  assert.equal(calls[0][0], '/api/workforce/tasks/9007199254740993/execution-operations/9007199254740995/decisions');
  assert.deepEqual(calls[0][1].data, { expectedVersion: '3', action: 'ALLOW_ONCE', reason: '', idempotencyKey: 'retry-key' });
  await assert.rejects(() => controller.decideExecutionOperation('1', '2', { expectedVersion: '1', action: 'FORGED', reason: '', idempotencyKey: 'key' }));
});

test('chat and detail UI expose the three decisions, lazy command detail and grant revoke without a new page', () => {
  const card = readFileSync(new URL('../src/app/workforce/tasks/NetworkOperationCard.tsx', import.meta.url), 'utf8');
  for (const label of ['允许这次', '本任务允许', '拒绝', '查看执行内容', '结果已送达 Adapter', '结果已返回 Agent', 'Agent 已继续', '可以向上述服务发送工作区中的数据']) assert.match(card, new RegExp(label));
  assert.match(card, /enabled: detailsOpen/);
  assert.doesNotMatch(card, /dangerouslySetInnerHTML|error\.message|response\.data/);
  const grants = readFileSync(new URL('../src/app/workforce/tasks/NetworkGrantsPanel.tsx', import.meta.url), 'utf8');
  assert.match(grants, /本任务可访问的服务/);
  assert.match(grants, /revokeTaskNetworkGrant/);
  const detailPage = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  assert.equal((detailPage.match(/<NetworkGrantsPanel\b/g) ?? []).length, 1);
});
