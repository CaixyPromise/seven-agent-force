import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const read = (path) => readFile(new URL(path, import.meta.url), 'utf8');
const compile = (source, jsx = ts.JsxEmit.React) => ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext, jsx } }).outputText;
const normalizers = await import(`data:text/javascript;base64,${Buffer.from(compile(await read('../src/api/reviewNormalizers.ts'))).toString('base64')}`);
const summary = { required: true, status: 'PENDING_FINALIZATION', availableActions: [], automaticRevisionCount: 0, automaticRevisionLimit: 0, noProgressStreak: 0, revisionGuidanceRequired: false };

test('actual pre-finalization summary loads without a fabricated ReviewRun', () => {
  const result = normalizers.normalizeIndependentReviewReport(summary);
  assert.equal(result.status, 'PENDING_FINALIZATION');
  assert.equal(result.verdict, undefined);
  assert.deepEqual(result.availableActions, []);
  assert.throws(() => normalizers.normalizeIndependentReviewReport({ ...summary, status: 'UNKNOWN_STATE' }));
});

test('summary-only state stays forbidden in the real Run contract', () => {
  const run = { id: '1', taskId: '2', taskTurnId: '3', status: 'PENDING', automaticRevisionNumber: 0, noProgressStreak: 0, createdAt: '2026-09-09T00:00:00Z' };
  assert.equal(normalizers.normalizeReviewRun(run).status, 'PENDING');
  assert.throws(() => normalizers.normalizeReviewRun({ ...run, status: 'PENDING_FINALIZATION' }), /reviewRun.status/);
});

test('pending display states a neutral fact without directing cancelled tasks to continue', async () => {
  const source = await read('../src/app/workforce/review/IndependentReviewWorkspace.tsx');
  assert.match(source, /report\.data\.status === 'PENDING_FINALIZATION'\) return <PendingFinalizationNotice/);
  const component = source.slice(source.indexOf('function PendingFinalizationNotice()'), source.indexOf('function ReviewStateTag('));
  const Notice = new Function('React', 'Card', 'Text', `${compile(component)}; return PendingFinalizationNotice;`)(React, ({ title, children }) => React.createElement('section', null, title, children), ({ children }) => React.createElement('span', null, children));
  const html = renderToStaticMarkup(React.createElement(Notice));
  assert.match(html, /独立审查尚未开始。/);
  assert.doesNotMatch(html, /请|下一步|先完成|无法加载|独立审查中|PENDING_FINALIZATION|<button/);
});

test('Task hero maps CHECK_UNAVAILABLE to safe Chinese', async () => {
  const source = await read('../src/app/workforce/tasks/[taskId]/page.tsx');
  const fn = source.slice(source.indexOf('function humanizeResultSummary('), source.indexOf('function RecoveryCard('));
  const humanize = new Function(`${compile(fn)}; return humanizeResultSummary;`)();
  assert.match(source, /humanizeResultSummary\(task\.resultSummary/);
  const html = renderToStaticMarkup(React.createElement('span', null, humanize('CHECK_UNAVAILABLE')));
  assert.match(html, /当前检查因命令或环境条件不足，未能完成。/);
  assert.doesNotMatch(html, /CHECK_UNAVAILABLE|请|下一步|确认/);
});
