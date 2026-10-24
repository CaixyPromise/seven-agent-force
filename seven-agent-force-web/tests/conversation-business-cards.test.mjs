import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import ts from 'typescript';
const require = createRequire(import.meta.url);
const source = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');
function compile(path, imports = {}) {
  const js = ts.transpileModule(source(path), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { if (!(name in imports)) throw new Error(`Unexpected dependency ${name}`); return imports[name]; });
  return exports;
}
const checks = compile('../src/api/completionCheckContract.ts');
const artifacts = compile('../src/api/taskArtifactContract.ts', { './completionCheckContract': checks });
const taskId = '9007199254740993';
const artifact = () => ({ id: '9007199254740995', taskId, attemptId: '7', type: 'OUTPUT', name: '结果说明.md', sha256: 'a'.repeat(64), sizeBytes: 123, mediaType: 'text/markdown', status: 'AVAILABLE', downloadUrl: 'https://untrusted.invalid/secret', metadata: { raw: 'secret' } });

test('three business slots render once in a tabbed accessible outcome group', () => {
  const React = require('react');
  const { renderToStaticMarkup } = require('react-dom/server');
  const Cards = compile('../src/app/workforce/tasks/ConversationBusinessCards.tsx', { react: React, antd: require('antd'), 'react/jsx-runtime': require('react/jsx-runtime') }).default;
  const html = renderToStaticMarkup(React.createElement(Cards, {
    checks: React.createElement('section', { 'data-existing': 'checks' }, '真实检查面板'),
    review: React.createElement('section', { 'data-existing': 'review' }, '真实审查面板'),
    result: React.createElement('section', { 'data-existing': 'result' }, '<script>结果</script>'),
  }));
  assert.equal((html.match(/role="tab"/g) ?? []).length, 3);
  for (const title of ['完成检查', '独立审查', '结果与下载']) assert.match(html, new RegExp(title));
  assert.equal((html.match(/data-existing="checks"/g) ?? []).length, 1);
  assert.equal((html.match(/data-existing="review"|data-existing="result"/g) ?? []).length, 0);
  assert.doesNotMatch(html, /<script>|role="button"/);
});
test('existing check/review/artifact panels are exclusively in the results tab, not workflow or technical details', () => {
  const detail = source('../src/app/workforce/tasks/[taskId]/page.tsx');
  const workflow = detail.slice(detail.indexOf("workspaceTab === 'workflow'"), detail.indexOf("workspaceTab === 'results'"));
  const results = detail.slice(detail.indexOf("workspaceTab === 'results'"), detail.indexOf('<div ref={detailsRef}'));
  const technical = detail.slice(detail.indexOf('<div ref={detailsRef}'), detail.indexOf('</div>\n    </div>\n  );'));
  for (const name of ['CompletionCheckPanel', 'IndependentReviewWorkspace', 'ArtifactsTab']) {
    assert.equal((detail.match(new RegExp(`<${name}\\b`, 'g')) ?? []).length, 1);
    assert.match(results, new RegExp(`<${name}\\b`));
    assert.doesNotMatch(workflow, new RegExp(`<${name}\\b`));
    assert.doesNotMatch(technical, new RegExp(`<${name}\\b`));
  }
  assert.match(results, /artifacts=\{artifactsQuery.data \?\? \[\]\}/);
  assert.match(results, /StatusTag status=\{task.status\}/);
  assert.match(technical, /OverviewTab|TaskInputSummary/);
  assert.doesNotMatch(workflow, /type: ['"]RESULT|mergeConversation|mock|fixture/);
});
test('business cards stay outside virtual history; historical result links select the results workspace', () => {
  const chat = source('../src/app/workforce/tasks/TaskConversation.tsx');
  assert.doesNotMatch(chat, /ConversationBusinessCards/);
  assert.match(chat, /item.type === 'RESULT'.*onResults/s);
  const detail = source('../src/app/workforce/tasks/[taskId]/page.tsx');
  assert.match(detail, /<ConversationBusinessCards/);
  assert.match(detail, /params.set\('tab', 'results'\)/);
  const cards = source('../src/app/workforce/tasks/ConversationBusinessCards.tsx');
  assert.doesNotMatch(cards, /useQuery|fetch\(|getConversation|allowedActions/);
  assert.match(cards, /activeKey=\{active\}/);
  assert.match(cards, /if \(focus\) setActive\(focus\)/);
});
test('artifact DTO projects safe bounded fields, decimal IDs and a same-origin authenticated download path', () => {
  const item = artifacts.normalizeTaskArtifacts([artifact()], taskId)[0];
  assert.equal(item.id, '9007199254740995');
  assert.equal(item.downloadUrl, `/api/workforce/tasks/${taskId}/artifacts/${item.id}/download`);
  assert.equal(item.metadata, undefined);
  for (const patch of [{ id: 4 }, { taskId: '8' }, { attemptId: '01' }, { sizeBytes: Number.MAX_SAFE_INTEGER + 1 }, { sizeBytes: -1 }, { sha256: 'bad' }, { name: 'secret\0' }, { name: {} }]) assert.throws(() => artifacts.normalizeTaskArtifacts([{ ...artifact(), ...patch }], taskId));
  assert.throws(() => artifacts.normalizeTaskArtifacts(null, taskId));
  assert.throws(() => artifacts.normalizeTaskArtifacts([artifact(), artifact()], taskId));
});
test('artifact read uses existing single API and unknown states cannot enable downloads', async () => {
  const calls = [];
  const api = compile('../src/api/workforceController.ts', {
    './request': { request: async (...args) => { calls.push(args); return { data: [artifact()] }; } },
    '@/lib/apiId': { toApiIdParam: checks.completionCheckId }, './completionCheckContract': checks, './taskSummaryContract': {}, './taskStatusContract': {}, './taskArtifactContract': artifacts,
  });
  const items = await api.listWorkforceArtifacts(taskId);
  assert.equal(calls.length, 1);
  assert.equal(calls[0][0], `/api/workforce/tasks/${taskId}/artifacts`);
  assert.equal(items[0].id, artifact().id);
  const detail = source('../src/app/workforce/tasks/[taskId]/page.tsx');
  assert.match(detail, /artifact.status !== 'AVAILABLE'/);
  assert.match(detail, /if \(error\) return .*结果文件暂时无法读取/);
  assert.match(detail, /catch \{ setDownloadError\('文件下载未完成，请重试。'\)/);
  assert.doesNotMatch(detail.slice(detail.indexOf('function ArtifactsTab'), detail.indexOf('function StatusTag')), /window.open|artifact.downloadUrl|error.message/);
});

test('public int64 byte-count strings normalize without coercing IDs or accepting unsafe quantities', () => {
  for (const value of ['0', '20', '81', '1065', '9007199254740991']) {
    const item = artifacts.normalizeTaskArtifacts([{ ...artifact(), sizeBytes: value }], taskId)[0];
    assert.equal(item.sizeBytes, Number(value));
    assert.equal(item.id, artifact().id);
    assert.equal(typeof item.id, 'string');
  }
  for (const value of ['9007199254740992', '9223372036854775807', '-1', '01', '+1', '1.0', '1e3', ' 20', '', null, true, {}, 0.5, Infinity]) {
    assert.throws(() => artifacts.normalizeTaskArtifacts([{ ...artifact(), sizeBytes: value }], taskId));
  }
});
