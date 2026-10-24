import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const read = path => readFileSync(new URL(`../${path}`, import.meta.url), 'utf8');

test('configuration UI exposes the approved DC2A scalar and DC2B asset types and uses a controlled JSON editor', () => {
  const options = read('src/app/system/config/const.d.ts');
  const card = read('src/app/system/config/components/ConfigItemCard.tsx');
  const controlledJson = read('src/app/system/config/components/ControlledJsonEditor.tsx');

  for (const approved of [
    'STRING', 'TEXT', 'INTEGER', 'DECIMAL', 'BOOLEAN', 'ENUM', 'MULTI_ENUM',
    'DATE', 'DATETIME', 'DURATION', 'COLOR', 'JSON', 'IMAGE', 'FILE',
  ]) {
    assert.match(options, new RegExp(`value: [\"']${approved}[\"']`), `missing approved type ${approved}`);
  }
  for (const forbidden of ['CONFIG_ASSET', 'URL', 'HTML', 'SCRIPT', 'fileId', 'logoUrl']) {
    assert.equal(options.includes(forbidden), false, `type selector exposes ${forbidden}`);
  }
  assert.match(card, /<ControlledJsonEditor/);
  assert.doesNotMatch(card, /valueType === ['"]json['"]/);
  assert.doesNotMatch(controlledJson, /dangerouslySetInnerHTML|style\s*=|href\s*=|src\s*=/);
});

test('client wire parser has no JSON-to-string fallback', () => {
  const contract = read('src/types/configClient.ts');
  assert.match(contract, /value: unknown/);
  assert.match(contract, /case 'JSON'/);
  assert.doesNotMatch(contract, /JSON\.parse\(value\)/);
  assert.doesNotMatch(contract, /catch\s*\{\s*return value/);
});

test('config and dictionary caches bind account scope and authorization generation', () => {
  const configProvider = read('src/hooks/config/ConfigClientProvider.tsx');
  const dictProvider = read('src/hooks/dict/DictClientProvider.tsx');
  for (const source of [configProvider, dictProvider]) {
    assert.match(source, /cacheIdentityRef/);
    assert.match(source, /generationRef/);
  }
  const identity = read('src/hooks/config/cacheIdentity.ts');
  assert.match(identity, /user\.primaryOrgId/);
  assert.match(identity, /org:/);
  assert.match(identity, /user\.authVersion/);
  assert.match(identity, /account=.*scope=.*authz=/s);
});

test('management UI labels unconnected consumers and uses finite dictionary tokens', () => {
  const configCard = read('src/app/system/config/components/ConfigItemCard.tsx');
  const dictItems = read('src/app/system/dict/components/DictItemList.tsx');
  assert.match(configCard, /未连接/);
  assert.match(dictItems, /\['gray', 'blue', 'pink', 'green', 'orange', 'red', 'purple'\]/);
  assert.match(dictItems, /\['unknown', 'male', 'female', 'check', 'close', 'info'\]/);
  assert.doesNotMatch(dictItems, /扩展属性 \(JSON\)|JSON\.parse/);
});

test('controlled JSON rejects non-string values instead of coercing them', () => {
  const controlledJson = read('src/app/system/config/components/ControlledJsonEditor.tsx');
  assert.match(controlledJson, /typeof item !== 'string'/);
  assert.match(controlledJson, /数字、布尔、数组和嵌套对象必须通过专用契约管理/);
  assert.doesNotMatch(controlledJson, /JSON\.stringify\(item\)/);
});

test('management editors preserve invalid local drafts and submit structured validation', () => {
  const controlledJson = read('src/app/system/config/components/ControlledJsonEditor.tsx');
  const card = read('src/app/system/config/components/ConfigItemCard.tsx');
  const validation = read('src/app/system/config/components/ScalarValidationEditor.tsx');
  assert.match(controlledJson, /setRows\(nextRows\)/);
  assert.match(controlledJson, /JSON 字段名不能为空/);
  assert.match(controlledJson, /JSON 字段名不能重复/);
  assert.match(controlledJson, /onDraftChange/);
  assert.match(validation, /mode="tags"/);
  for (const field of ['required', 'minLength', 'maxLength', 'minValue', 'maxValue', 'maxItems', 'options']) {
    assert.match(validation, new RegExp(field));
  }
  assert.match(card, /await onSave\(updateData\)/);
  assert.match(card, /setError\(saveError instanceof Error/);
});

test('successful saves refetch versions and dictionary mutations are permission gated', () => {
  const configProvider = read('src/app/system/config/context/ConfigProvider.tsx');
  const dictProvider = read('src/app/system/dict/context/DictProvider.tsx');
  const dictItems = read('src/app/system/dict/components/DictItemList.tsx');
  const dictTypes = read('src/app/system/dict/components/DictTypeSidebar.tsx');
  assert.match(configProvider, /await updateConfig\(updateRequest\)[\s\S]*await fetchConfigs/);
  assert.match(dictProvider, /await updateDictItem[\s\S]*await fetchItems/);
  assert.match(dictProvider, /version: current\?\.version/);
  for (const source of [dictItems, dictTypes]) {
    assert.match(source, /DICT_PERMISSIONS\.ADD/);
    assert.match(source, /DICT_PERMISSIONS\.EDIT/);
    assert.match(source, /DICT_PERMISSIONS\.DELETE/);
  }
});

test('connected layout consumers apply title short title and finite theme tokens', () => {
  const layout = read('src/components/layout/index.tsx');
  assert.match(layout, /SEVEN_FRONTEND_METADATA\.title/);
  assert.match(layout, /SEVEN_FRONTEND_METADATA\.shortTitle/);
  assert.match(layout, /SEVEN_FRONTEND_METADATA\.themePrimaryColor/);
  assert.match(layout, /THEME_PRESET_COLORS/);
  assert.match(layout, /<ConfigProvider theme=\{\{ token: \{ colorPrimary: primaryColor \} \}\}>/);
  assert.match(layout, /menuHeaderRender=/);
  assert.match(layout, /collapsed \?/);
  assert.match(layout, /resolvedShortTitle/);
  assert.doesNotMatch(layout, /dangerouslySetInnerHTML/);
});
