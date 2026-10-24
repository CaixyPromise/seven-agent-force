import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const page = readFileSync(new URL('../src/app/workforce/model-credentials/page.tsx', import.meta.url), 'utf8');
const controller = readFileSync(new URL('../src/api/workforceController.ts', import.meta.url), 'utf8');
const taskComposer = readFileSync(new URL('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx', import.meta.url), 'utf8');

test('model credential editor contains only the key, protocol, address and catalog product contract', () => {
  assert.match(page, /模型密钥/);
  assert.match(page, /OpenAI/);
  assert.match(page, /Anthropic/);
  assert.match(page, /从上游获取/);
  for (const removed of ['能力探测', '执行角色', '路由优先级', '最大并发', '默认模型', '允许模型']) {
    assert.doesNotMatch(page, new RegExp(removed));
  }
});

test('model credentials render as responsive cards without a page-level table scroller', () => {
  assert.match(page, /模型中心/);
  assert.match(page, /className=\{styles\.cardGrid\}/);
  assert.match(page, /className=\{styles\.credentialCard\}/);
  assert.match(page, /编辑配置/);
  assert.match(page, /deleteWorkforceModelCredential/);
  assert.match(page, /删除后不能再用于新任务，已有调用记录会继续保留/);
  assert.doesNotMatch(page, /scroll=\{\{ x: 1050 \}\}/);
});

test('model center loads one aggregate analytics query and renders real ECharts views', () => {
  const charts = readFileSync(new URL('../src/app/workforce/model-credentials/ModelAnalyticsCharts.tsx', import.meta.url), 'utf8');
  assert.match(page, /getWorkforceModelAnalytics/);
  assert.match(page, /model-credential-analytics/);
  assert.match(page, /近 7 天/);
  assert.match(page, /近 30 天/);
  assert.match(page, /近 90 天/);
  assert.match(page, /ModelTrendChart/);
  assert.match(page, /ModelDistributionChart/);
  assert.match(charts, /echarts\/core/);
  assert.match(charts, /ResizeObserver/);
  assert.doesNotMatch(page, /credentials\.data\.map\([^)]*listModelCredentialInvocations/s);
});

test('plaintext API keys are not submitted as React Query mutation variables or stored locally', () => {
  assert.match(page, /mutationFn: async \(\) => \{\s*const values = await form\.validateFields\(\)/);
  assert.match(page, /onFinish=\{\(\) => save\.mutate\(\)\}/);
  assert.doesNotMatch(page, /save\.mutate\(values\)/);
  assert.doesNotMatch(page + controller, /localStorage|sessionStorage|URLSearchParams\([^)]*apiKey/);
  assert.match(page, /autoComplete="new-password"/);
  assert.match(page, /form\.resetFields\(\)/);
});

test('task composer selects an explicit credential and model without infrastructure controls', () => {
  assert.match(taskComposer, /listWorkforceModelOptions/);
  assert.match(taskComposer, /modelSelection: \{ credentialId: selected\.credentialId, model: selected\.model \}/);
  assert.match(taskComposer, /modelOptions\.data\?\.length === 1/);
  for (const forbidden of ['providerAlias', 'harnessAlias', 'baseUrl', 'apiKey']) {
    assert.doesNotMatch(taskComposer, new RegExp(forbidden));
  }
  // Worker identity is allowed only as a key for the read-only capacity list.
  assert.match(taskComposer, /item\.workers\.map\(\(worker\) => <Flex key=\{worker\.workerId\}/);
  assert.doesNotMatch(taskComposer.replaceAll('key={worker.workerId}', ''), /\bworkerId\b/);
});

test('model invocation history identifies and links its task without extra row requests', () => {
  assert.match(controller, /taskName: string/);
  assert.match(page, /title: '任务'/);
  assert.match(page, /item\.taskName\.trim\(\) \|\| '未命名任务'/);
  assert.match(page, /to=\{`\/workforce\/tasks\/\$\{encodeURIComponent\(item\.taskId\)\}`\}/);
  assert.doesNotMatch(page, /data\.map\([^)]*getWorkforceTask/s);
});
