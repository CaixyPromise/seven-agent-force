import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const api = readFileSync(new URL('../src/api/externalLoginController.ts', import.meta.url), 'utf8');
const account = readFileSync(new URL('../src/app/account/settings/components/EnterpriseAccountsCard.tsx', import.meta.url), 'utf8');
const admin = readFileSync(new URL('../src/app/system/enterprise-connections/page.tsx', import.meta.url), 'utf8');
const callback = readFileSync(new URL('../src/app/(auth)/external-identities/oauth/callback/[provider]/page.tsx', import.meta.url), 'utf8');
const login = readFileSync(new URL('../src/app/(auth)/login/page.tsx', import.meta.url), 'utf8');
const feedback = readFileSync(new URL('../src/lib/enterprise-identity-feedback.ts', import.meta.url), 'utf8');
const externalLoginAdmin = readFileSync(new URL('../src/app/system/external-login-provider/page.tsx', import.meta.url), 'utf8');
const authRoutes = readFileSync(new URL('../src/lib/auth/routes.ts', import.meta.url), 'utf8');

test('enterprise identity IDs remain decimal strings at the Web boundary', () => {
  for (const field of ['id', 'userId', 'identityVersion', 'credentialVersion', 'version']) {
    assert.match(api, new RegExp(`${field}[^;]*string`));
  }
  for (const source of [api, account, admin, callback, login]) {
    assert.doesNotMatch(source, /(?:Number|parseInt)\s*\([^)]*(?:identity|connection|user).*\)/i);
  }
});

test('ordinary enterprise-account screens use product language and hide identity internals', () => {
  assert.match(account, /企业账号/);
  assert.match(account, /接收个人消息/);
  assert.match(account, /群消息中提醒我/);
  assert.doesNotMatch(account, /可以用于登录|暂不可用于登录/);
  for (const source of [account, callback, login]) {
    assert.doesNotMatch(source, /ExternalIdentity|Subject digest|Open ID|PKCE|Nonce|OAuth Token|Identity Version/);
  }
});

test('enterprise login remains hidden while its product flow is unpublished', () => {
  assert.doesNotMatch(login, /getEnterpriseLoginOptions|startEnterpriseLogin|enterpriseOptions\.methods\.map/);
  assert.match(api, /\/api\/login\/enterprise\/options/);
  assert.match(api, /\/api\/login\/enterprise\/\$\{provider\}\/authorizations/);
  assert.match(api, /\/api\/external-identities\/oauth\/callback\/\$\{encodeURIComponent\(provider\)\}/);
  assert.doesNotMatch(api, /\/api\/login\/enterprise\/\$\{encodeURIComponent\(provider\)\}\/callback/);
  assert.doesNotMatch(login, /企业账号登录|组织代码|使用飞书登录|使用企业微信登录/);
});

test('binding callback is purpose aware and never calls a binding login success', () => {
  assert.match(callback, /result\.authenticated \? 'login' : 'bind'/);
  assert.match(callback, /purpose==='login'\?'登录成功':'绑定完成'/);
});

test('enterprise OAuth callbacks are public without exposing enterprise login', () => {
  assert.match(authRoutes, /'\/external-identities\/oauth\/callback\/'/);
  assert.doesNotMatch(authRoutes, /'\/login\/enterprise\/'/);
});

test('binding callback shows the safe server reason and trace reference', () => {
  assert.match(callback, /enterpriseBindingFeedback\(error, params\.provider\)/);
  assert.match(feedback, /case 'WECOM':[\s\S]*return '企业微信'/);
  assert.match(feedback, /`\$\{providerLabel\}授权没有完成`/);
  assert.match(feedback, /case 'ENTERPRISE_AUTHORIZATION_PROVIDER_REJECTED'/);
  assert.match(feedback, /应用 Agent ID 和应用密钥来自同一个企业微信自建应用/);
  assert.match(feedback, /payload\?\.traceId/);
  assert.match(feedback, /readReasonCode\(payload\)/);
  assert.match(callback, /参考编号：\{failure\.traceId\}/);
  assert.doesNotMatch(callback, /subTitle="授权已失效，请重新开始。"/);
});

test('binding callback consumes an OAuth state only once across development remounts', () => {
  assert.match(callback, /const callbackRequests = new Map/);
  assert.match(callback, /completeEnterpriseBindingOnce/);
  assert.match(callback, /callbackRequests\.get\(key\)/);
  assert.match(callback, /crypto\.subtle\.digest\('SHA-256'/);
  assert.doesNotMatch(callback, /callbackRequests\.set\([^,]+,\s*\{[^}]*state/);
});

test('enterprise binding surfaces the safe server error instead of hiding it behind a generic toast', () => {
  assert.match(account, /onError:\s*\(error:\s*unknown\)\s*=>\s*message\.error\(enterpriseErrorMessage\(error,/);
  assert.match(account, /error instanceof Error\s*&&\s*error\.message\.trim\(\)/);
  assert.doesNotMatch(account, /onError:\s*\(\)\s*=>\s*message\.error\('暂时无法开始绑定，请稍后重试'\)/);
});

test('enterprise login is absent from ordinary and administration surfaces', () => {
  assert.doesNotMatch(login, /企业账号登录|组织代码|使用飞书登录|使用企业微信登录/);
  assert.doesNotMatch(admin, /connectionSetup\.loginUrl|打开组织登录页|企业登录使用组织专属入口|setEnterpriseLoginAccess/);
  assert.doesNotMatch(externalLoginAdmin, /组织企业登录|与企业连接自动联动|listEnterpriseConnections|listEnterpriseIdentities/);
});

test('new enterprise connections obtain callback URLs from platform setup instead of an existing record', () => {
  assert.match(api, /getEnterpriseConnectionSetup/);
  assert.match(api, /\/api\/system\/enterprise-connections\/setup/);
  assert.match(admin, /queryFn:\s*getEnterpriseConnectionSetup/);
  assert.match(admin, /connectionSetup\?\.providers\.find\(\(item\)\s*=>\s*item\.provider\s*===\s*editing\?\.provider\)\?\.callbackUrl/);
  assert.doesNotMatch(admin, /editing\?\.record\?\.callbackUrl\s*\?/);
});

test('enterprise connection save failures remain visible beside the form', () => {
  assert.match(admin, /const \[saveFailure, setSaveFailure\] = useState/);
  assert.match(admin, /onError:\s*\(error:\s*unknown\)\s*=>\s*setSaveFailure\(enterpriseConnectionSaveFeedback\(error\)\)/);
  assert.match(admin, /saveFailure\s*\?\s*<div[\s\S]*<Alert[\s\S]*type="error"[\s\S]*title=\{saveFailure\.title\}/);
  assert.match(admin, /description=\{saveFailure\.description\}/);
  assert.match(admin, /saveFailureRef\.current\?\.scrollIntoView\(\{\s*behavior:\s*'auto',\s*block:\s*'start'\s*\}\)/);
  assert.match(admin, /ref=\{saveFailureRef\}[\s\S]*tabIndex=\{-1\}/);
  assert.match(admin, /description:\s*withReference\(serverMessage\s*\|\|\s*errorMessage\s*\|\|\s*'系统处理连接时发生异常，已保留填写内容。请稍后重试。'\)/);
  assert.match(admin, /payload\?:\s*\{\s*message\?: unknown;\s*traceId\?: unknown;\s*data\?: unknown\s*\}/);
  assert.match(admin, /参考编号：\$\{traceId\}/);
  assert.doesNotMatch(admin, /description=\{\(saveMutation\.error as Error\)\.message\}/);
});
