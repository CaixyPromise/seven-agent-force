import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(
  new URL('../src/components/notification/enterpriseConnectionFeedback.ts', import.meta.url),
  'utf8',
);
const pageSource = await readFile(
  new URL('../src/app/system/notification/page.tsx', import.meta.url),
  'utf8',
);
const { outputText, diagnostics = [] } = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  reportDiagnostics: true,
});
assert.deepEqual(diagnostics, []);

const feedback = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('enterprise probe maps sanitized failure categories to direct messages', () => {
  assert.equal(
    feedback.enterpriseConnectionTestFeedback({ status: 'FAILED', failureClass: 'AUTHENTICATION', diagnostic: 'TOKEN_REJECTED' }).title,
    '应用凭据无效',
  );
  assert.equal(
    feedback.enterpriseConnectionTestFeedback({ status: 'FAILED', failureClass: 'INVALID_TARGET', diagnostic: 'INVALID_TARGET' }).title,
    '接收对象不可用或不可见',
  );
  assert.equal(
    feedback.enterpriseConnectionTestFeedback({ status: 'FAILED', failureClass: 'AUTHENTICATION', diagnostic: 'FEISHU_REJECTED' }).title,
    '飞书拒绝请求',
  );
  assert.equal(
    feedback.enterpriseConnectionTestFeedback({ status: 'FAILED', failureClass: 'PROVIDER_REJECTED', diagnostic: 'WECOM_REJECTED' }, 'WECOM_APP').title,
    '企业微信拒绝请求',
  );
});

test('enterprise probe shows the sanitized source message and keeps credentials and targets out', () => {
  const result = feedback.enterpriseConnectionTestFeedback({
    status: 'FAILED',
    failureClass: 'PROVIDER_REJECTED',
    diagnostic: 'FEISHU_REJECTED',
    providerReference: 'om_sensitive-provider-reference',
    providerError: {
      provider: 'FEISHU_APP',
      httpStatus: 400,
      code: '230001',
      message: 'The application bot is not in this chat.',
      logId: 'feishu-log-123',
    },
    warnings: [{ provider: 'FEISHU_APP', key: 'ignored', reason: 'raw provider response: token=secret' }],
  });

  assert.equal(result.title, '飞书拒绝请求');
  assert.equal(result.detail, 'The application bot is not in this chat.（230001）；部分可选项未使用');
  assert.equal(result.detail.includes('om_sensitive-provider-reference'), false);
  assert.equal(result.detail.includes('token=secret'), false);
  assert.equal(result.detail.includes('feishu-log-123'), false);
  assert.equal(feedback.enterpriseConnectionTestErrorMessage, '测试连接暂时无法完成，请稍后重试');
});

test('enterprise probe unwraps a response envelope instead of hiding the provider failure', () => {
  const result = feedback.enterpriseConnectionTestFeedback(
    {
      status: 200,
      data: {
        code: 0,
        data: {
          status: 'FAILED',
          failureClass: 'PROVIDER_REJECTED',
          diagnostic: 'WECOM_REJECTED',
          providerError: {
            provider: 'WECOM_APP',
            code: '60020',
            message: 'The provider rejected this request from the current network.',
          },
        },
      },
    },
    'WECOM_APP',
  );

  assert.equal(result.title, '请配置企业可信 IP');
  assert.equal(result.detail, 'The provider rejected this request from the current network.（60020）');
  assert.deepEqual(result.guidance, {
    title: '请配置企业可信 IP',
    summary: '需要添加的是通知服务访问企业微信时使用的公网出口 IP。',
    steps: [
      '不要填写成员 UserID、应用 AgentId、127.0.0.1 或前端地址。',
      '进入企业微信管理后台：应用管理 → 自建应用 → 当前应用 → 开发者接口 → 企业可信 IP。',
      '保存后回到这里重新发送测试。',
    ],
    managementUrl: 'https://work.weixin.qq.com/wework_admin/frame#apps',
  });
});

test('enterprise probe unwraps the serialized response emitted by the local proxy', () => {
  const result = feedback.enterpriseConnectionTestFeedback(
    JSON.stringify({
      code: 0,
      data: {
        status: 'FAILED',
        failureClass: 'PROVIDER_REJECTED',
        diagnostic: 'WECOM_REJECTED',
        providerError: {
          provider: 'WECOM_APP',
          code: '60020',
          message: 'The provider rejected this request from the current network.',
        },
      },
    }),
    'WECOM_APP',
  );

  assert.equal(result.title, '请配置企业可信 IP');
  assert.equal(result.detail, 'The provider rejected this request from the current network.（60020）');
});

test('enterprise probe keeps a non-accepted result in the test dialog instead of a global toast', () => {
  const mutationStart = pageSource.indexOf('const enterpriseTestMutation = useMutation');
  const mutationEnd = pageSource.indexOf('const staticTestMutation = useMutation');
  const mutationSource = pageSource.slice(mutationStart, mutationEnd);

  assert.ok(mutationStart >= 0 && mutationEnd > mutationStart);
  assert.match(mutationSource, /feedback\.tone === 'success'/);
  assert.match(mutationSource, /setEnterpriseTestResultFeedback\(/);
  assert.equal(mutationSource.includes('message.warning('), false);
  assert.equal(mutationSource.includes('message.error('), false);
  assert.match(pageSource, /onValuesChange=\{\(\) => setEnterpriseTestResultFeedback\(null\)\}/);
  assert.match(
    pageSource,
    /<Alert[\s\S]*type=\{enterpriseTestResultFeedback\.tone\}[\s\S]*message=\{enterpriseTestResultFeedback\.title\}[\s\S]*enterpriseTestResultFeedback\.guidance/,
  );
  assert.match(pageSource, /打开企业微信管理后台/);
});

test('static HTTP probe keeps a bounded source error in its own small dialog', () => {
  const rejected = feedback.staticConnectionTestFeedback({
    status: 'FAILED',
    failureClass: 'PROVIDER_REJECTED',
    providerError: {
      provider: 'HTTP_CONNECTOR',
      httpStatus: 400,
      code: 'INVALID_SIGNATURE',
      message: 'signature is invalid',
    },
  });
  assert.equal(rejected.title, '接收服务拒绝请求');
  assert.equal(rejected.detail, 'signature is invalid（INVALID_SIGNATURE）');
  assert.equal(feedback.staticConnectionTestFeedback({ status: 'PROVIDER_ACCEPTED' }).title, '连接正常');

  const mutationStart = pageSource.indexOf('const staticTestMutation = useMutation');
  const mutationEnd = pageSource.indexOf('const openEnterpriseTestModal = useCallback');
  const mutationSource = pageSource.slice(mutationStart, mutationEnd);
  assert.ok(mutationStart >= 0 && mutationEnd > mutationStart);
  assert.match(mutationSource, /setStaticTestResultFeedback\(staticConnectionTestFeedback\(result\)\)/);
  assert.equal(mutationSource.includes('message.success('), false);
  assert.equal(mutationSource.includes('message.warning('), false);
  assert.equal(mutationSource.includes('message.error('), false);
  assert.match(pageSource, /testStaticConnection/);
  assert.match(pageSource, /title=\{staticTestChannel\?\.channelType === 'HTTP_CONNECTOR'/);
  assert.match(pageSource, /onValuesChange=\{\(\) => setStaticTestResultFeedback\(null\)\}/);
  assert.match(pageSource, /message=\{staticTestResultFeedback\.title\}/);
});
