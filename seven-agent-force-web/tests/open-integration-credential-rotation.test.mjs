import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const apiSource = await readFile(new URL('../src/api/openIntegrationController.ts', import.meta.url), 'utf8');
const pageSource = await readFile(new URL('../src/app/workforce/open-integrations/page.tsx', import.meta.url), 'utf8');

for (const [name, source] of [
  ['open integration API contract', apiSource],
  ['open integration page', pageSource],
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

function interfaceBody(name) {
  const match = apiSource.match(new RegExp(`export interface ${name} \\{([\\s\\S]*?)\\n\\}`));
  assert.ok(match, `${name} must exist`);
  return match[1];
}

test('application and creation contracts expose optional signing credentials with string ids', () => {
  const application = interfaceBody('OpenIntegrationApplication');
  assert.match(application, /apiCredential: IntegrationCredentialSummary/);
  assert.match(application, /signingCredential\?: IntegrationCredentialSummary/);
  assert.match(application, /apiKeyGrace\?: IntegrationCredentialGrace/);
  assert.match(application, /signingSecretGrace\?: IntegrationCredentialGrace/);
  assert.doesNotMatch(application, /^\s*credentialHint:/m);
  assert.doesNotMatch(application, /^\s*signingSecretHint:/m);

  const summary = interfaceBody('IntegrationCredentialSummary');
  const grace = interfaceBody('IntegrationCredentialGrace');
  const created = interfaceBody('CreatedOpenIntegration');
  assert.match(summary, /credentialId: API\.Int64/);
  assert.match(summary, /version: number/);
  assert.match(summary, /hint: string/);
  assert.match(grace, /credentialId: API\.Int64/);
  assert.match(created, /apiCredentialId: API\.Int64/);
  assert.match(created, /signingSecret\?: string/);
  assert.match(created, /signingCredentialId\?: API\.Int64/);
  assert.match(created, /apiCredentialVersion: number/);
  assert.match(created, /signingCredentialVersion\?: number/);
  assert.match(apiSource, /webhookUrl\?: string/);
});

test('both rotation calls use the frozen endpoints and preserve bigint ids as API.Int64', () => {
  const apiRotation = interfaceBody('APIKeyRotationResult');
  const signingRotation = interfaceBody('SigningSecretRotationResult');
  for (const contract of [apiRotation, signingRotation]) {
    assert.match(contract, /applicationId: API\.Int64/);
    assert.match(contract, /previousCredentialId: API\.Int64/);
    assert.match(contract, /previousValidUntil: string/);
    assert.match(contract, /createdAt: string/);
  }
  assert.match(apiRotation, /credentialId: API\.Int64/);
  assert.match(signingRotation, /signingCredentialId: API\.Int64/);
  assert.match(apiSource, /toApiIdParam\(id\)/);
  assert.match(apiSource, /\$\{[^}]*\}\/api-key\/rotate/);
  assert.match(apiSource, /\$\{[^}]*\}\/signing-secret\/rotate/);
  assert.equal(apiSource.includes('Number(id)'), false);
  assert.equal(apiSource.includes('parseInt(id'), false);
});

test('the page keeps webhook-only credentials and actions conditional', () => {
  assert.match(pageSource, /title: '凭证'/);
  assert.match(pageSource, /API v\{item\.apiCredential\.version\}/);
  assert.match(pageSource, /item\.signingCredential &&/);
  assert.match(pageSource, /hasWebhook\(application\)/);
  assert.match(pageSource, /hasWebhook\(item\)/);
  assert.match(pageSource, /<Dropdown/);
  assert.match(pageSource, />投递记录<\/Button>/);
  assert.match(pageSource, />更多<\/Button>/);
  for (const label of ['轮换 API Key', '轮换签名 Secret', '停用', '启用']) {
    assert.ok(pageSource.includes(label), `${label} must remain available`);
  }
  assert.equal(pageSource.includes('<Alert'), false, 'the compact operator page must not add a large alert');
});

test('copy facts keep optional webhook configuration and one-time secrets at their action surfaces', () => {
  // F-01: the page purpose remains in its title and compact subtitle.
  assert.ok(pageSource.includes('管理外部应用和任务回调'));
  // F-02: webhook state is visible in the row, without a permanent explanation banner.
  assert.ok(pageSource.includes('Webhook HTTPS 地址（可选）'));
  assert.ok(pageSource.includes('未配置'));
  // F-03: invalid optional input is repaired beside the field.
  assert.ok(pageSource.includes('地址最多 2048 个字符'));
  assert.ok(pageSource.includes('请输入 HTTPS 地址'));
  // F-04: generated secrets are disclosed only in the one-time result modal.
  assert.equal((pageSource.match(/关闭后无法再次查看/g) ?? []).length, 2);
  assert.match(pageSource, /credential\?\.signingSecret &&/);
  assert.ok(pageSource.includes('MCP 连接配置'));
  assert.ok(pageSource.includes('Authorization: Bearer ${SEVEN_WORKFORCE_API_KEY}'));
  assert.ok(pageSource.includes('Authorization: Bearer'));
});

test('rotation uses concise confirmation and globally serialized one-secret disclosure', () => {
  assert.ok(pageSource.includes('轮换 API Key？'));
  assert.ok(pageSource.includes('轮换签名 Secret？'));
  assert.ok(pageSource.includes('新 Key 立即生效，旧 Key 将在 15 分钟后失效。'));
  assert.ok(pageSource.includes('新 Secret 立即生效，Webhook 将在 24 小时内同时携带新旧签名。'));
  assert.equal(pageSource.includes('只会显示一次'), false, 'the one-time warning belongs only in the success modal');
  assert.match(pageSource, /okText: '轮换'/);
  assert.match(pageSource, /rotation\.secret/);
  assert.match(pageSource, /版本 v\{rotation\.credentialVersion\}/);
  assert.match(pageSource, /旧版本截止时间/);
  assert.equal((pageSource.match(/关闭后无法再次查看/g) ?? []).length, 2, 'create and rotation modals each use the one-time warning once');
  assert.equal((pageSource.match(/已保存，关闭/g) ?? []).length, 2, 'create and rotation modals each have one clear close action');
  assert.match(pageSource, /const secretOperationLocked = create\.isPending \|\| rotate\.isPending \|\| !!credential \|\| !!rotation/);
  assert.match(pageSource, /disabled: secretOperationLocked/);
  assert.match(pageSource, /rotate\.reset\(\)/);
  assert.match(pageSource, /create\.reset\(\)/);
  assert.match(pageSource, /rotationTriggerRef\.current\?\.focus\(\)/);
});
