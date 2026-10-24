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

const contract = compile('../src/api/deliveryContract.ts');
const deliveryBody = (patch = {}) => ({
  intentId: '9007199254740993', requestId: '9007199254740994', version: '3', intentVersion: '3', requestVersion: '1', status: 'WAITING_USER_DECISION',
  summary: '分页边界修复及回归检查已完成', deliveryMode: 'NEW_BRANCH_AND_DRAFT_REVIEW', authorization: 'ASK_AFTER_GATES',
  repositoryName: 'team/project', destinationBranch: 'codex/task-1', allowedActions: ['DELIVER', 'SKIP_THIS_RESULT'], ...patch,
});
const requestBody = (patch = {}) => ({
  id: '9007199254740994', version: '3', taskId: '9007199254740993', taskTurnId: '9007199254740995', checkpointId: '9007199254740996',
  actionType: 'GIT_PUSH', projectId: '9007199254740997', baseBranch: 'main', destinationBranch: 'codex/task-1',
  resultCommit: 'a'.repeat(40), resultTree: 'b'.repeat(40), selectionDigest: 'c'.repeat(64), deliveryCommit: 'd'.repeat(40),
  approvalMode: 'AFTER_APPROVAL', status: 'WAITING_APPROVAL', createdAt: '2026-09-12T12:00:00Z', ...patch,
});

test('delivery conversation projection is strict, lossless and drops no unsafe URL into the UI', () => {
  const item = contract.normalizeDeliveryConversationCard(deliveryBody({ pullRequestUrl: 'https://example.test/pr/1', remoteReadbackVerified: false }));
  assert.equal(item.intentId, '9007199254740993');
	assert.equal(item.intentVersion, '3');
	assert.equal(item.requestVersion, '1');
  assert.equal(item.pullRequestUrl, 'https://example.test/pr/1');
  for (const patch of [
    { intentId: 9007199254740993 }, { intentId: '01' }, { deliveryMode: 'NONE' }, { authorization: 'ALWAYS' },
    { allowedActions: ['DELIVER', 'FORCE_PUSH'] }, { pullRequestUrl: 'javascript:alert(1)' }, { pullRequestUrl: 'https://user:secret@example.test/pr/1' },
    { summary: '中'.repeat(800) }, { repositoryName: '' }, { requestId: undefined },
    { allowedActions: ['RESTART_DELIVERY'], resultCommit: undefined }, { deliveryCommit: 'a'.repeat(41) },
  ]) assert.throws(() => contract.normalizeDeliveryConversationCard(deliveryBody(patch)));
});

test('historical delivery cards normalize null actions as an empty safe action set', () => {
  const item = contract.normalizeDeliveryConversationCard(deliveryBody({ allowedActions: null }));
  assert.deepEqual(item.allowedActions, []);
});

test('delivery request normalizer accepts current delivery states and rejects raw or malformed identities', () => {
  assert.equal(contract.normalizeDeliveryRequest(requestBody()).status, 'WAITING_APPROVAL');
  assert.equal(contract.normalizeDeliveryRequest(requestBody()).projectId, '9007199254740997');
  assert.throws(() => contract.normalizeDeliveryRequest(requestBody({ projectId: undefined, targetId: '9007199254740997' })));
  assert.throws(() => contract.normalizeDeliveryRequest(requestBody({ projectId: 9007199254740997 })));
  assert.equal(contract.normalizeDeliveryRequests([requestBody()]).length, 1);
  assert.throws(() => contract.normalizeDeliveryRequests([requestBody(), requestBody()]));
  assert.throws(() => contract.normalizeDeliveryRequest(requestBody({ taskId: 1 })));
  assert.throws(() => contract.normalizeDeliveryRequest(requestBody({ status: 'PUSHED_MAYBE' })));
});

test('stable delivery feedback never exposes upstream messages or unknown reason text', () => {
  const secret = 'token=secret SQL stack /opt/private';
  assert.match(contract.deliveryFeedback({ payload: { data: { reasonCode: 'DELIVERY_REQUEST_STALE' }, message: secret } }), /已经变化/);
  assert.equal(contract.deliveryFeedback({ payload: { data: { reasonCode: 'UPSTREAM_' + secret }, message: secret } }), '交付操作未完成，请重试。');
  assert.equal(contract.deliveryFeedback(new Error(secret)), '交付操作未完成，请重试。');
});

test('confirming delivery exposes the existing guarded readback recovery action', () => {
  const page = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  assert.match(page, /item\.status === 'CONFIRMING'[^\n]+onReconcile\(item\.id\)[^\n]+检查并恢复交付/);
});

test('task-scoped API preserves expected version and idempotency without accepting Git targets', async () => {
  const calls = [];
  const api = compile('../src/api/deliveryController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: requestBody() }; } },
    '@/lib/apiId': { toApiIdParam(value) { if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) throw new Error('bad id'); return value; } },
    './deliveryContract': contract,
  });
  await api.decideTaskDelivery('1', '2', { expectedVersion: '3', action: 'DELIVER', reason: '', idempotencyKey: 'same-key' });
  assert.equal(calls[0][0], '/api/workforce/tasks/1/delivery-requests/2/decisions');
  assert.deepEqual(calls[0][1].data, { expectedVersion: '3', action: 'DELIVER', reason: '', idempotencyKey: 'same-key' });
  await api.restartTaskDelivery('1', { resultCommit: 'a'.repeat(40), idempotencyKey: 'restart-key' });
  assert.deepEqual(calls[1][1].data, { resultCommit: 'a'.repeat(40), idempotencyKey: 'restart-key' });
  await assert.rejects(() => api.restartTaskDelivery('1', { resultCommit: 'not-a-commit', idempotencyKey: 'x' }));
});

test('conversation supports one delivery card and allowlisted delivery actions', () => {
  const execution = compile('../src/api/executionAccessContract.ts');
  const conversation = compile('../src/api/conversationContract.ts', { './executionAccessContract': execution, './deliveryContract': contract });
  const normalized = conversation.normalizeConversationItem({ id: '8', version: '1', type: 'DELIVERY', createdAt: '2026-09-12T12:00:00Z', body: deliveryBody() });
  assert.equal(normalized.type, 'DELIVERY');
  assert.deepEqual(conversation.normalizeConversationPage({ items: [normalized], nextCursor: null, watermark: '8', allowedActions: ['DECIDE_DELIVERY', 'RESTART_DELIVERY', 'FORCE_PUSH'] }).allowedActions, ['DECIDE_DELIVERY', 'RESTART_DELIVERY']);
});

test('project delivery defaults automatic delivery but visibly fences users without approval permission', () => {
  const projectForm = readFileSync(new URL('../src/app/workforce/projects/_components/ProjectForm.tsx', import.meta.url), 'utf8');
  const taskComposer = readFileSync(new URL('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx', import.meta.url), 'utf8');
  const card = readFileSync(new URL('../src/app/workforce/tasks/GitDeliveryCard.tsx', import.meta.url), 'utf8');
  assert.match(projectForm, /workforce:delivery:approve/);
  assert.match(projectForm, /authorization: 'AUTO_AFTER_GATES'/);
  assert.match(projectForm, /disabled: !canApproveDelivery/);
  assert.match(projectForm, /value\.delivery\.authorization = 'ASK_AFTER_GATES'/);
  assert.match(taskComposer, /PROJECT_DEFAULT/);
  assert.doesNotMatch(taskComposer, /\b(?:remote|credential|refspec)\b|forcePush|force push/i);
  assert.match(card, /本次不交付/);
  assert.match(card, /重新发起交付/);
	assert.match(card, /expectedVersion: item\.requestVersion/);
  assert.doesNotMatch(card, /error\.message|response\.data|dangerouslySetInnerHTML|credential|force push/i);
});


test('reauthorization keeps a confirmation gate and explicitly identifies the replacement ProjectVersion', () => {
  const page = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  assert.match(page, /Popconfirm title="重新确认这份结果的交付？"[^\n]+onConfirm=\{\(\) => onReauthorize\(item\.id, replacementVersionId\)\}/);
  assert.match(page, /保留原结果、仓库与分支/);
  assert.match(page, /项目版本：/);
  assert.match(page, /replacementVersionId=\{projectQuery\.data\?\.versionId\}/);
  assert.match(page, /请再次批准；平台会先核对远端/);
  assert.doesNotMatch(page, /已冻结新的凭证授权|绑定了新的仓库凭证版本/);
});

test('reauthorization submits only the confirmed string ProjectVersion reference and reason', async () => {
  const calls = [];
  const api = compile('../src/api/deliveryController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: requestBody() }; } },
    '@/lib/apiId': { toApiIdParam(value) { if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value)) throw new Error('bad id'); return value; } },
    './deliveryContract': contract,
  });
  await api.reauthorizeDelivery('9007199254740994', 'confirmed replacement', '9007199254740997');
  assert.deepEqual(calls[0][1].data, { reason: 'confirmed replacement', projectVersionId: '9007199254740997' });
  await assert.rejects(() => api.reauthorizeDelivery('9007199254740994', 'replacement', 9007199254740997));
  assert.equal(calls.length, 1);
});
