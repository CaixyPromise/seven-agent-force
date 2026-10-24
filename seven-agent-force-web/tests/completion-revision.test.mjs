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
const checks = compile('../src/api/completionCheckContract.ts');
const c = compile('../src/api/completionRevisionContract.ts', { './completionCheckContract': checks });
const taskId = '9007199254740993';
const base = { mode: 'COMMAND', command: 'pnpm test', instructions: '现有说明', referenceAssetIds: ['2'] };
const view = () => ({ expectedVersion: '9007199254740995', completionCheck: base, pending: null, allowedActions: ['REQUEST_REVISION'] });
const revision = () => ({ id: '9', taskId, sourceTaskTurnId: '3', sourceVersion: '9007199254740995', revisionNumber: '1', version: '1', status: 'PENDING', completionCheck: base, baseDigest: 'a'.repeat(64), checkDigest: 'b'.repeat(64), createdAt: '2026-09-09T12:00:00Z', replayed: false });

test('revision view uses independent authority, lossless versions, and safe projection', () => {
  const actual = c.normalizeRevisionView({ ...view(), attemptId: 'secret' }, taskId);
  assert.equal(actual.expectedVersion, '9007199254740995');
  assert.deepEqual(actual.allowedActions, ['REQUEST_REVISION']);
  assert.equal(actual.attemptId, undefined);
  assert.deepEqual(c.normalizeRevisionView({ ...view(), allowedActions: ['ADD_VERIFICATION', 'ANSWER', 'FUTURE'] }, taskId).allowedActions, []);
  for (const bad of [1, '01', '0', '9223372036854775808']) assert.throws(() => c.normalizeRevisionView({ ...view(), expectedVersion: bad }, taskId));
  for (const bad of [null, {}, [1]]) assert.throws(() => c.normalizeRevisionView({ ...view(), allowedActions: bad }, taskId));
  assert.throws(() => c.normalizeRevisionView({ ...view(), pending: revision() }, taskId));
});
test('PENDING cannot imply application; pending field cannot contain terminal revision', () => {
  const pending = c.normalizeCompletionRevision({ ...revision(), sourceAttemptId: 'secret', nativeId: 'private' }, taskId);
  assert.equal(pending.appliedTurnId, undefined);
  assert.equal(pending.sourceAttemptId, undefined);
  assert.throws(() => c.normalizeCompletionRevision({ ...revision(), status: 'APPLIED' }, taskId));
  const applied = { ...revision(), status: 'APPLIED', appliedTurnId: '12', appliedAt: '2026-09-09T12:01:00Z' };
  assert.equal(c.normalizeCompletionRevision(applied, taskId).appliedTurnId, '12');
  assert.throws(() => c.normalizeRevisionView({ ...view(), pending: applied, allowedActions: [] }, taskId));
  for (const patch of [{ taskId: '2' }, { replayed: 'true' }, { createdAt: 'bad' }, { appliedTurnId: '12' }, { status: 'DELIVERED' }, { checkDigest: 'secret' }]) assert.throws(() => c.normalizeCompletionRevision({ ...revision(), ...patch }, taskId));
});
test('append editor preserves frozen executable, text prefix and references', () => {
  const next = c.buildRevisionCheck(base, { command: 'replace!', mode: 'SCRIPT', instructions: ' 新说明 ', referenceAssetIds: ['4'] }, ['4']);
  assert.equal(next.command, 'pnpm test');
  assert.equal(next.mode, 'COMMAND');
  assert.equal(next.instructions, '现有说明\n新说明');
  assert.deepEqual(next.referenceAssetIds, ['2', '4']);
  assert.throws(() => c.buildRevisionCheck(base, {}, []));
  assert.throws(() => c.buildRevisionCheck(base, { instructions: '   ' }, []));
  assert.throws(() => c.buildRevisionCheck(base, { referenceAssetIds: ['99'] }, ['4']));
  assert.throws(() => c.buildRevisionCheck(base, { instructions: '中'.repeat(6000) }, []));
  assert.equal(c.buildRevisionCheck({ mode: 'NONE' }, { mode: 'SCRIPT', script: 'echo ok' }, []).script, 'echo ok');
});
test('reference candidates are bounded safe task input, never arbitrary IDs or uploads', () => {
  assert.deepEqual(c.normalizeRevisionReferences({ taskId, references: [{ id: '4', displayName: '<b>资料</b>', objectKey: 'secret' }] }, taskId), [{ id: '4', label: '<b>资料</b>' }]);
  for (const value of [{ taskId: '2', references: [] }, { taskId, references: null }, { taskId, references: [{ id: 4, displayName: 'x' }] }, { taskId, references: Array(65).fill({ id: '4', displayName: 'x' }) }]) assert.throws(() => c.normalizeRevisionReferences(value, taskId));
});
test('known revision errors are classified and unknown/raw errors never leak', () => {
  assert.equal(c.revisionFeedback({ payload: { data: { reasonCode: 'COMPLETION_CHECK_REVISION_UNAVAILABLE' } } }), '暂不支持调整后续检查。');
  for (const e of [new Error('secret /private SQL'), [], null, { details: [] }, { code: 'secret', message: 'secret' }]) assert.equal(c.revisionFeedback(e), '检查调整未完成，已保留输入，请重试。');
});
test('real route adapters normalize input/output and preserve exact retry payload', async () => {
  const calls = [];
  const api = compile('../src/api/completionRevisionController.ts', { './completionCheckContract': checks, './completionRevisionContract': c, './request': { request: async (url, opts) => { calls.push({ url, opts }); return { data: opts.method === 'GET' ? view() : revision() }; } } });
  await api.getCompletionRevisionView(taskId);
  const input = { expectedVersion: '9007199254740995', idempotencyKey: 'retry-key', completionCheck: base, nativeId: 'secret' };
  await api.requestCompletionRevision(taskId, input); await api.requestCompletionRevision(taskId, input);
  assert.equal(calls[0].url, `/api/workforce/tasks/${taskId}/completion-check-revisions`);
  assert.deepEqual(calls[1].opts.data, calls[2].opts.data);
  assert.equal(calls[1].opts.data.nativeId, undefined);
  await assert.rejects(api.requestCompletionRevision(1, input));
  assert.equal(calls.length, 3);
});
test('rendered initial revision capability is disabled unless server grants exact action', () => {
  const React = require('react'); const { renderToStaticMarkup } = require('react-dom/server');
  function render(data, error) {
    const component = compile('../src/app/workforce/tasks/CompletionRevisionPanel.tsx', {
      react: React, 'react/jsx-runtime': require('react/jsx-runtime'), antd: require('antd'),
      '@tanstack/react-query': { useQuery: () => ({ data, isError: !!error, error, isLoading: false }) },
      '@/store/auth': { useAuthStore: (select) => select({ user: { id: '1', primaryOrgId: '2' } }) },
      '@/api/workspaceInputController': {}, '@/api/completionRevisionController': {}, '@/api/completionRevisionContract': c, '@/api/completionCheckContract': checks,
    }).default;
    return renderToStaticMarkup(React.createElement(component, { taskId }));
  }
  assert.match(render({ ...view(), allowedActions: [] }), /disabled/);
  assert.doesNotMatch(render(view()), /disabled/);
  const pending = render({ ...view(), pending: revision(), allowedActions: [] });
  assert.match(pending, /等待应用/); assert.doesNotMatch(pending, /已应用/);
  const unavailable = render(undefined, { payload: { data: { reasonCode: 'COMPLETION_CHECK_REVISION_UNAVAILABLE' } }, message: 'secret' });
  assert.match(unavailable, /暂不支持/); assert.doesNotMatch(unavailable, /secret/);
});
test('form retains snapshot and draft, fences scope/stale state and reuses payload idempotency', () => {
  const source = readFileSync(new URL('../src/app/workforce/tasks/CompletionRevisionPanel.tsx', import.meta.url), 'utf8');
  assert.match(source, /key=\{`\$\{scope\}:\$\{taskId\}`\}/);
  assert.match(source, /current.expectedVersion !== snapshot.expectedVersion/);
  assert.match(source, /idempotency.current\?\.fingerprint !== fingerprint/);
  assert.match(source, /submitting.current/);
  assert.match(source, /preserve>/);
  assert.doesNotMatch(source, /ADD_VERIFICATION|createReferenceAsset|setFieldsValue|resetFields/);
});
