import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const organizationSource = await readFile(
  new URL('../src/app/system/organization/page.tsx', import.meta.url),
  'utf8',
);
const configScopeSource = await readFile(
  new URL('../src/app/system/role/components/ConfigScopeAssign.tsx', import.meta.url),
  'utf8',
);
const configControllerSource = await readFile(
  new URL('../src/api/configController.ts', import.meta.url),
  'utf8',
);
const typingsSource = await readFile(
  new URL('../src/api/typings.d.ts', import.meta.url),
  'utf8',
);
const onlineUserSource = await readFile(
  new URL('../src/app/system/online-user/page.tsx', import.meta.url),
  'utf8',
);
const profileSource = await readFile(
  new URL('../src/app/account/settings/components/ProfileSection.tsx', import.meta.url),
  'utf8',
);
const userProfileControllerSource = await readFile(
  new URL('../src/api/userProfileController.ts', import.meta.url),
  'utf8',
);
const adminControllerSource = await readFile(
  new URL('../src/api/adminController.ts', import.meta.url),
  'utf8',
);
const storageControllerSource = await readFile(
  new URL('../src/api/storageStrategyController.ts', import.meta.url),
  'utf8',
);
const dictControllerSource = await readFile(
  new URL('../src/api/dictController.ts', import.meta.url),
  'utf8',
);
const dictClientTypesSource = await readFile(
  new URL('../src/types/dictClient.ts', import.meta.url),
  'utf8',
);
const organizationTreeSelectorSource = await readFile(
  new URL('../src/components/Selectors/OrganizationTreeSelector.tsx', import.meta.url),
  'utf8',
);
const permissionTreeSelectorSource = await readFile(
  new URL('../src/components/Selectors/PermissionTreeSelector.tsx', import.meta.url),
  'utf8',
);

const apiIdSource = await readFile(
  new URL('../src/lib/apiId.ts', import.meta.url),
  'utf8',
);
const { outputText: apiIdOutput, diagnostics: apiIdDiagnostics = [] } = ts.transpileModule(
  apiIdSource,
  {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    reportDiagnostics: true,
  },
);
assert.deepEqual(apiIdDiagnostics, []);
const apiId = await import(
  `data:text/javascript;base64,${Buffer.from(apiIdOutput).toString('base64')}`
);

test('organization status preserves snowflake IDs as decimal strings', () => {
  assert.doesNotMatch(organizationSource, /Number\(org\.id\)/);
  assert.match(organizationSource, /mutateAsync\(\{\s*id:\s*org\.id,\s*status\s*\}\)/);
  assert.match(
    typingsSource,
    /type changeStatusParams = \{[\s\S]*?id: Int64;[\s\S]*?status: number;/,
  );
});

test('role config scopes preserve snowflake IDs as decimal strings', () => {
  assert.doesNotMatch(configScopeSource, /Number\(roleId\)/);
  assert.match(configScopeSource, /getRoleConfigScopes\(roleId\)/);
  assert.match(configScopeSource, /assignRoleConfigScopes\(roleId, payload\)/);
  assert.match(
    configControllerSource,
    /getRoleConfigScopes\(roleId: API\.Int64\)/,
  );
  assert.match(
    configControllerSource,
    /assignRoleConfigScopes\(roleId: API\.Int64,/,
  );
});

test('large Snowflake IDs survive shared API ID normalization exactly', () => {
  const largeId = '2065424359060983808';

  assert.equal(apiId.toApiIdParam(largeId), largeId);
  assert.deepEqual(apiId.toApiIdList([largeId, '9007199254740993']), [
    largeId,
    '9007199254740993',
  ]);
  assert.equal(Number(largeId).toString() === largeId, false);
});

test('tree selectors preserve large Snowflake IDs as decimal strings', () => {
  const largeId = '2065424359060983808';
  const selectedKeys = [largeId];

  assert.deepEqual(selectedKeys.map(String), [largeId]);
  for (const source of [organizationTreeSelectorSource, permissionTreeSelectorSource]) {
    assert.match(source, /id: API\.Int64;/);
    assert.match(source, /parentId: API\.Int64;/);
    assert.match(source, /value\?: API\.Int64 \| API\.Int64\[\];/);
    assert.doesNotMatch(source, /Number\((?:key|selectedKeysValue\[0\])\)/);
    assert.match(source, /String\(key\) as API\.Int64/);
    assert.match(source, /String\(selectedKeysValue\[0\]\) as API\.Int64/);
  }
});

test('SSO and avatar actions never coerce Snowflake IDs through Number', () => {
  assert.doesNotMatch(onlineUserSource, /toSafeNumericUserId|Number\([^)]*userId/i);
  assert.match(onlineUserSource, /function isValidUserId\(value\?: API\.Int64\)/);
  assert.match(onlineUserSource, /loadUserDevices = async \(userId: API\.Int64\)/);

  assert.doesNotMatch(profileSource, /Number\([^)]*fileId|return Number\(value\)/);
  assert.match(profileSource, /resolveFileId = \(value: unknown\): API\.Int64 \| undefined/);
  assert.match(userProfileControllerSource, /fileId: API\.Int64/);
});

test('all identifier-shaped API declarations use Int64 instead of number', () => {
  const unsafeDeclarations = [
    ...typingsSource.matchAll(
      /\b(?:id|[A-Za-z][A-Za-z0-9]*(?:Id|IDs|Ids))\??:\s*number(?:\[\])?/g,
    ),
  ].map((match) => match[0]);

  assert.deepEqual(unsafeDeclarations, []);
  assert.match(dictClientTypesSource, /id: API\.Int64;/);
  assert.match(dictClientTypesSource, /dictTypeId: API\.Int64;/);
});

test('ID and metric result envelopes use separate aliases', () => {
  assert.match(typingsSource, /type ResultInt64 = Result<Int64>/);
  assert.match(typingsSource, /type ResultInteger = Result<number>/);
  assert.match(
    adminControllerSource,
    /request<API\.Result<API\.Int64>>\(`\/api\/admin\/online\/count`/,
  );
  assert.match(adminControllerSource, /data: response\.data === undefined \? undefined : Number\(response\.data\)/);
  assert.match(
    storageControllerSource,
    /request<API\.ResultInt64>\(`\/api\/storage-strategy`/,
  );
});

test('config and dict mutations declare the Go Result envelope', () => {
  assert.match(
    configControllerSource,
    /request<API\.Result<API\.Int64>>\(`\/api\/config-groups`/,
  );
  assert.match(
    configControllerSource,
    /request<API\.Result<boolean>>\(`\/api\/config\/update`/,
  );
  assert.match(
    dictControllerSource,
    /request<API\.Result<API\.Int64>>\(`\/api\/dict-type\/add`/,
  );
  assert.match(
    dictControllerSource,
    /request<API\.Result<boolean>>\(`\/api\/dict\/items\/update`/,
  );
});
