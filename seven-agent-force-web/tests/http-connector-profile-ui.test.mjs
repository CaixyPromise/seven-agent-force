import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const apiSource = await readFile(new URL('../src/api/notificationController.ts', import.meta.url), 'utf8');
const pageSource = await readFile(new URL('../src/app/system/notification/page.tsx', import.meta.url), 'utf8');

for (const [name, source] of [
  ['notification API contract', apiSource],
  ['notification channel page', pageSource],
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

test('group profile form has only fixed, write-only connection inputs', () => {
  assert.match(apiSource, /webhookProfileConfig\?: WebhookProfileConfig/);
  assert.match(apiSource, /webhookUrl\?: string/);
  assert.match(apiSource, /webhookSigningSecret\?: string/);
  assert.match(pageSource, /name="webhookUrl"/);
  assert.match(pageSource, /name="webhookSigningSecret"/);
  assert.match(pageSource, /<Input\.Password placeholder="留空表示不使用签名"/);
  assert.match(pageSource, /isWebhookProfileChannel\(channelType\)/);
  assert.match(pageSource, /群、地址和消息格式由已保存的连接决定/);
});

test('HTTP and group profile connections are selectable and the retired direct test-send API is absent', () => {
  assert.match(pageSource, /\{ value: 'HTTP_CONNECTOR', label: 'HTTP 连接' \}/);
  assert.match(pageSource, /\{ value: 'FEISHU_WEBHOOK', label: '飞书群机器人' \}/);
  assert.match(pageSource, /\{ value: 'WECOM_WEBHOOK', label: '企业微信群机器人' \}/);
  assert.match(pageSource, /isEnterpriseApplicationChannel\(record\.channelType\)/);
  assert.match(pageSource, /isStaticHTTPChannel\(record\.channelType\)/);
  assert.equal(apiSource.includes('/test-send'), false);
  assert.equal(apiSource.includes('testSendNotification'), false);
  assert.equal(pageSource.includes("mTLS（后续投递支持）"), false);
  assert.equal(apiSource.includes("| 'MTLS'"), false);
});

test('HTTP and group profiles have a separate non-persistent connection check', () => {
  assert.match(apiSource, /export interface StaticConnectionTestRequest/);
  assert.match(apiSource, /channels\/test-static-connection/);
  assert.match(pageSource, /isStaticHTTPChannel\(record\.channelType\)/);
  assert.match(pageSource, /openStaticTestModal\(record\)/);
  assert.match(pageSource, /不会创建站内信或投递记录/);
});
