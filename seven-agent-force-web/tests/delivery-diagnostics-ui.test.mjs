import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const apiSource = await readFile(new URL('../src/api/notificationController.ts', import.meta.url), 'utf8');
const pageSource = await readFile(new URL('../src/app/system/notification/page.tsx', import.meta.url), 'utf8');
const workspaceSource = await readFile(new URL('../src/components/notification/DeliveryDiagnosticsWorkspace.tsx', import.meta.url), 'utf8');

for (const [name, source] of [
  ['notification API contract', apiSource],
  ['notification page', pageSource],
  ['delivery diagnostics workspace', workspaceSource],
]) {
  const { diagnostics = [] } = ts.transpileModule(source, {
    compilerOptions: {
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    reportDiagnostics: true,
  });
  assert.deepEqual(diagnostics, [], `${name} must remain syntactically valid`);
}

function notificationDeliveryInterface() {
  const match = apiSource.match(/export interface NotificationDelivery \{([\s\S]*?)\n\}/);
  assert.ok(match, 'normal delivery interface must exist');
  return match[1];
}

test('the first-level page uses the configured business-facing workspaces', () => {
  const directTabs = pageSource.match(/const tabs = \[([\s\S]*?)\n  \];/);
  assert.ok(directTabs, 'direct tab list must exist');
  assert.deepEqual(
    [...directTabs[1].matchAll(/key: '([^']+)'/g)].map((match) => match[1]),
    ['group-routes', 'channels', 'template-versions', 'scene-versions', 'deliveries'],
  );
  for (const label of ['通知群', '发送渠道', '消息模板', '发送规则', '发送记录']) {
    assert.match(directTabs[1], new RegExp(label));
  }
  for (const removedLabel of ['历史配置', '历史模板', '历史发送规则', '更多']) {
    assert.equal(pageSource.includes(removedLabel), false, `${removedLabel} must not appear in the page`);
  }
  assert.equal(pageSource.includes('模板、场景和投递记录集中在这里'), false);
  assert.equal(pageSource.includes('已发布的新模板和场景不会被旧配置改写'), false);
  assert.equal(pageSource.includes('<Collapse'), false);
});

test('normal delivery data cannot contain rendered content, raw targets, or payloads', () => {
  const normalDelivery = notificationDeliveryInterface();
  for (const forbidden of [
    'renderedSubject',
    'renderedText',
    'renderedHtml',
    'renderedMarkdown',
    'payloadJson',
    'target:',
    'lastError',
  ]) {
    assert.equal(normalDelivery.includes(forbidden), false, `normal delivery API must not expose ${forbidden}`);
  }
  assert.match(apiSource, /diagnostic-content/);
  assert.match(apiSource, /method: 'POST'/);
});

test('diagnostic plaintext stays in one modal and never joins a shared or persistent cache', () => {
  for (const forbidden of ['useQuery', 'useMutation', 'localStorage', 'sessionStorage', 'dangerouslySetInnerHTML']) {
    assert.equal(workspaceSource.includes(forbidden), false, `diagnostic workspace must not use ${forbidden}`);
  }
  assert.match(workspaceSource, /setContent\(null\)/);
  assert.match(workspaceSource, /destroyOnHidden/);
  assert.match(workspaceSource, /\[accountID, close\]/);
  assert.match(workspaceSource, /仅查看这一条记录/);
  assert.match(workspaceSource, /请说明用途并完成确认/);
  assert.match(workspaceSource, /短期内容，请勿复制或转发/);
});
