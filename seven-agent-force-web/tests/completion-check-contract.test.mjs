import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/api/completionCheckContract.ts', import.meta.url), 'utf8');
const { outputText, diagnostics } = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 }, reportDiagnostics: true });
assert.deepEqual(diagnostics, []);
const contract = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
const { normalizeCompletionCheck, completionCheckId, completionCheckFeedback, completionCheckFieldFeedback } = contract;

test('optional material stays per task and removed review fields are ignored', () => {
  assert.deepEqual(normalizeCompletionCheck({ mode: 'NONE', command: 'draft', script: 'draft', instructions: '验收说明', referenceAssetIds: ['9007199254740993'], reviewFocus: '边界', profileId: '2' }), {
    mode: 'NONE', instructions: '验收说明', referenceAssetIds: ['9007199254740993'],
  });
  assert.deepEqual(normalizeCompletionCheck({ mode: 'COMMAND', command: ' pnpm test\n', script: 'draft' }), { mode: 'COMMAND', command: ' pnpm test\n', referenceAssetIds: [] });
  assert.deepEqual(normalizeCompletionCheck({ mode: 'SCRIPT', script: '#!/bin/sh\nexit 0' }), { mode: 'SCRIPT', script: '#!/bin/sh\nexit 0', referenceAssetIds: [] });
  assert.deepEqual(normalizeCompletionCheck({ mode: 'NONE', script: 'x'.repeat(65537), command: 'draft' }), { mode: 'NONE', referenceAssetIds: [] });
});

test('completion input validates required mode and executable, types and NUL without exposing values', () => {
  for (const value of [null, [], {}, { mode: 'SHELL' }, { mode: 'COMMAND', command: ' ' }, { mode: 'SCRIPT', script: '' }, { mode: 'NONE', instructions: 12 }, { mode: 'NONE', instructions: 'secret\0' }, { mode: 'NONE', instructions: '\ud800' }, { mode: 'NONE', instructions: '\u001b[31m' }, { mode: 'NONE', referenceAssetIds: null }]) {
    assert.throws(() => normalizeCompletionCheck(value), { code: 'COMPLETION_CHECK_INVALID', message: 'COMPLETION_CHECK_INVALID' });
  }
});

test('GET and POST adapters use approved routes, whitelist request fields and normalize responses', async () => {
  const apiSource = await readFile(new URL('../src/api/completionCheckController.ts', import.meta.url), 'utf8');
  const apiJS = ts.transpileModule(apiSource, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText;
  const calls = [];
  const request = async (path, options) => { calls.push({ path, options }); return { data: options.method === 'GET' ? summary() : { taskTurnId: '9007199254740997', decisionId: '9007199254740999', replayed: false } }; };
  const load = new Function('request', 'contract', `${apiJS.replace(/import \{ request \} from ['"]\.\/request['"];?/, '').replace(/import \{([^}]+)\} from ['"]\.\/completionCheckContract['"];?/, 'const {$1} = contract;').replace(/export /g, '')}\nreturn { getTaskCompletionCheck, decideTaskCompletionCheck };`);
  const api = load(request, contract);
  await api.getTaskCompletionCheck(taskId);
  const input = { resultId, expectedVersion: '9007199254740998', action: 'AUTHORIZE_REPAIR', reason: 'audit only', repairScope: 'repair boundary', idempotencyKey: 'retry-key', instruction: 'do not forward' };
  await api.decideTaskCompletionCheck(taskId, input);
  await api.decideTaskCompletionCheck(taskId, input);
  assert.equal(calls[0].path, `/api/workforce/tasks/${taskId}/completion-check`);
  assert.equal(calls[1].path, `/api/workforce/tasks/${taskId}/completion-check-decisions`);
  assert.deepEqual(calls[1].options.data, calls[2].options.data);
  assert.equal(calls[1].options.data.instruction, undefined);
  assert.equal(calls[1].options.data.reason, 'audit only');
  assert.equal(calls[1].options.data.repairScope, 'repair boundary');
  await assert.rejects(api.decideTaskCompletionCheck(1, input));
  assert.equal(calls.length, 3);
});

test('limits count UTF-8 bytes for multilingual content and accept exact boundaries', () => {
  for (const [field, max] of Object.entries(contract.completionCheckLimits).filter(([key]) => key !== 'referenceAssetIds')) {
    const mode = field === 'command' ? 'COMMAND' : field === 'script' ? 'SCRIPT' : 'NONE';
    const exact = '中'.repeat(Math.floor(max / 3)) + 'a'.repeat(max % 3);
    assert.equal(normalizeCompletionCheck({ mode, [field]: exact })[field], exact);
    assert.throws(() => normalizeCompletionCheck({ mode, [field]: exact + 'a' }));
  }
});

test('reference IDs stay strict decimal int64 strings and lists are bounded', () => {
  for (const id of ['1', '9007199254740993', '9223372036854775807']) assert.equal(completionCheckId(id), id);
  for (const id of [1, 9007199254740992, '0', '-1', '01', ' 1', '1e2', '9223372036854775808', {}, null]) assert.throws(() => completionCheckId(id));
  const referenceAssetIds = Array.from({ length: 64 }, (_, i) => String(i + 1));
  assert.equal(normalizeCompletionCheck({ mode: 'NONE', referenceAssetIds }).referenceAssetIds.length, 64);
  for (const refs of [[...referenceAssetIds, '65'], ['1', '1'], [1], '1']) assert.throws(() => normalizeCompletionCheck({ mode: 'NONE', referenceAssetIds: refs }));
});

test('known errors use only safe codes; hostile error details never become copy', () => {
  const secret = 'secret-token /private/host trace SQL SELECT';
  for (const code of ['COMPLETION_CHECK_INVALID', 'COMPLETION_CHECK_NOT_FOUND', 'COMPLETION_CHECK_STALE', 'COMPLETION_CHECK_ACTION_NOT_ALLOWED', 'COMPLETION_CHECK_IDEMPOTENCY_CONFLICT']) {
    const expected = completionCheckFeedback({ reasonCode: code });
    assert.notEqual(expected, completionCheckFeedback({}));
    assert.equal(completionCheckFeedback({ payload: { data: { reasonCode: code, message: secret } }, message: secret }), expected);
    assert.equal(completionCheckFeedback({ payload: { code, message: secret }, message: secret }), completionCheckFeedback({}));
  }
  for (const value of [new Error(secret), { code: secret, message: secret }, { reasonCode: { toString: () => secret } }]) {
    assert.doesNotMatch(completionCheckFeedback(value), /secret|private|SQL/);
    assert.doesNotMatch(completionCheckFieldFeedback(value), /secret|private|SQL/);
  }
});

const taskId = '9007199254740993';
const resultId = '9007199254740995';
const evidence = (phase) => ({ id: resultId, taskId, taskTurnId: '9007199254740997', attemptId: '9007199254740999', phase, status: 'FAILED', checkDigest: 'a'.repeat(64), summary: '检查未通过', observation: { raw: 'secret' } });
const summary = () => ({ input: { mode: 'COMMAND', command: 'pnpm test' }, digest: 'a'.repeat(64), baseline: evidence('BASELINE'), currentResult: evidence('CURRENT'), availableActions: [...contract.completionCheckActions], checkpointReady: true, version: '9007199254740998', sourceCheckpointId: '9007199254740996', expectedCurrentTurnId: '9007199254740997' });

test('summary normalizes authoritative enums, IDs, null states and projects only safe fields', () => {
  const actual = contract.normalizeCompletionCheckSummary(summary(), taskId);
  assert.equal(actual.version, '9007199254740998');
  assert.equal(actual.currentResult.id, resultId);
  assert.equal(actual.currentResult.observation, undefined);
  for (const status of ['PASSED', 'FAILED', 'UNAVAILABLE', 'NO_TESTS', 'NOT_CONFIGURED', 'TIMED_OUT']) {
    const value = summary(); value.currentResult.status = status;
    assert.equal(contract.normalizeCompletionCheckSummary(value, taskId).currentResult.status, status);
  }
  const empty = { ...summary(), input: null, digest: '', baseline: null, currentResult: null, availableActions: [], checkpointReady: false, sourceCheckpointId: '0' };
  assert.equal(contract.normalizeCompletionCheckSummary(empty, taskId).currentResult, null);
});

test('malformed or mismatched summary cannot invent actions or a trusted result', () => {
  for (const patch of [{ version: 1 }, { currentResult: undefined }, { currentResult: { ...evidence('CURRENT'), taskId: '5' } }, { currentResult: { ...evidence('CURRENT'), status: 'PASS' } }, { baseline: evidence('CURRENT') }, { digest: 'raw-body' }, { availableActions: ['BYPASS'] }, { availableActions: ['AUTHORIZE_REPAIR', 'AUTHORIZE_REPAIR'] }, { checkpointReady: false }, { sourceCheckpointId: '0' }]) {
    assert.throws(() => contract.normalizeCompletionCheckSummary({ ...summary(), ...patch }, taskId));
  }
});

test('definition drift preserves PASSED evidence but never permits continuing with limitations', () => {
  const value = { ...summary(), reasonCode: 'CHECK_DEFINITION_CHANGED', currentResult: { ...evidence('CURRENT'), status: 'PASSED' }, availableActions: ['AUTHORIZE_REPAIR', 'ADD_VERIFICATION'] };
  const actual = contract.normalizeCompletionCheckSummary(value, taskId);
  assert.equal(actual.reasonCode, 'CHECK_DEFINITION_CHANGED');
  assert.equal(actual.currentResult.status, 'PASSED');
  assert.deepEqual(actual.availableActions, ['AUTHORIZE_REPAIR', 'ADD_VERIFICATION']);
  assert.throws(() => contract.normalizeCompletionCheckSummary({ ...value, availableActions: ['CONTINUE_WITH_LIMITATIONS'] }, taskId));
  for (const reasonCode of ['raw secret-token /private/path', null, 1]) assert.throws(() => contract.normalizeCompletionCheckSummary({ ...value, reasonCode }, taskId));
  assert.equal(contract.normalizeCompletionCheckSummary({ ...summary(), reasonCode: '' }, taskId).reasonCode, undefined);
});

test('obsolete definitions remain read-only while persisted evidence stays visible', () => {
  const value = { ...summary(), input: null, reasonCode: 'CHECK_DEFINITION_UNAVAILABLE', availableActions: [] };
  const actual = contract.normalizeCompletionCheckSummary(value, taskId);
  assert.equal(actual.input, null);
  assert.equal(actual.reasonCode, 'CHECK_DEFINITION_UNAVAILABLE');
  assert.equal(actual.currentResult.status, 'FAILED');
  assert.deepEqual(actual.availableActions, []);
});

test('decision retains result fencing and audit reason separately from Agent repair scope', () => {
  const base = { resultId, expectedVersion: '9007199254740998', reason: '审计原因', idempotencyKey: 'one-logical-request' };
  const input = { ...base, action: 'AUTHORIZE_REPAIR', repairScope: '只修复分页边界', instruction: 'never forward', sourceCheckpointId: '9' };
  assert.deepEqual(contract.normalizeCompletionCheckDecision(input), { ...base, action: 'AUTHORIZE_REPAIR', repairScope: '只修复分页边界' });
  assert.deepEqual(contract.normalizeCompletionCheckDecision({ ...base, action: 'CONTINUE_WITH_LIMITATIONS' }), { ...base, action: 'CONTINUE_WITH_LIMITATIONS' });
  assert.equal(contract.normalizeCompletionCheckDecision({ ...base, action: 'ADD_VERIFICATION', completionCheck: { mode: 'SCRIPT', script: 'exit 0' } }).completionCheck.script, 'exit 0');
  for (const patch of [{ action: 'AUTHORIZE_REPAIR' }, { action: 'AUTHORIZE_REPAIR', repairScope: ' ' }, { action: 'ADD_VERIFICATION', completionCheck: { mode: 'NONE' } }, { action: 'CONTINUE_WITH_LIMITATIONS', completionCheck: { mode: 'COMMAND', command: 'test' } }, { action: 'CONTINUE_WITH_LIMITATIONS', repairScope: 'repair' }, { action: 'CONTINUE_WITH_LIMITATIONS', resultId: 1 }, { action: 'CONTINUE_WITH_LIMITATIONS', expectedVersion: '0' }, { action: 'CONTINUE_WITH_LIMITATIONS', reason: '中'.repeat(667) }]) {
    assert.throws(() => contract.normalizeCompletionCheckDecision({ ...base, ...patch }));
  }
});

test('decision response requires lossless decision and turn identities plus boolean replay', () => {
  const response = { taskTurnId: '9007199254740997', decisionId: '9007199254740999', replayed: true };
  assert.deepEqual(contract.normalizeCompletionCheckDecisionResult({ ...response, debug: 'secret' }), response);
  for (const patch of [{ taskTurnId: 1 }, { decisionId: undefined }, { replayed: 'false' }]) assert.throws(() => contract.normalizeCompletionCheckDecisionResult({ ...response, ...patch }));
});
