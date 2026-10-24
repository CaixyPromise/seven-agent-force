import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const apiSource = await readFile(new URL('../src/api/notificationController.ts', import.meta.url), 'utf8');
const pageSource = await readFile(new URL('../src/app/system/notification/page.tsx', import.meta.url), 'utf8');
const workspaceSource = await readFile(new URL('../src/components/notification/VersionedSceneWorkspace.tsx', import.meta.url), 'utf8');

for (const [name, source] of [
  ['notification API contract', apiSource],
  ['notification page', pageSource],
  ['versioned scene workspace', workspaceSource],
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

test('sending rules use the current rule workspace only', () => {
  assert.match(pageSource, /发送规则/);
  assert.match(pageSource, /<VersionedSceneWorkspace canEdit=\{permissions\.sceneEdit\} \/>/);
  assert.equal(pageSource.includes('历史发送规则'), false);
  assert.equal(workspaceSource.includes('导入历史规则'), false);
  assert.equal(workspaceSource.includes('legacySceneBindingId'), false);
  assert.match(apiSource, /scene-definitions/);
  assert.match(apiSource, /scene-revisions\/\$\{encodeURIComponent\(String\(revisionId\)\)\}\/publish/);
});

test('scene editor has exactly the small sending configuration surface', () => {
  assert.match(workspaceSource, /label="模板"/);
  assert.match(workspaceSource, /label="发送方式"/);
  assert.match(workspaceSource, /站内信/);
  assert.match(workspaceSource, /飞书应用 · 指定成员/);
  assert.match(workspaceSource, /飞书应用 · 指定群聊/);
  assert.match(workspaceSource, /企业微信应用 · 指定成员/);
  assert.match(workspaceSource, /固定连接 · /);
  for (const forbiddenField of ['metadataJson', 'priority', 'fallback', 'retryIntervalSeconds', 'maxRetry', 'variablesJson']) {
    assert.equal(workspaceSource.includes(`name="${forbiddenField}"`), false, `${forbiddenField} must not be an editor field`);
  }
  assert.equal(workspaceSource.includes('JSON.parse'), false);
  assert.equal(workspaceSource.includes('JSON.stringify'), false);
  assert.equal(workspaceSource.includes('webhookUrl'), false);
  assert.equal(workspaceSource.includes('secretPlain'), false);
  assert.equal(workspaceSource.includes('ExternalRecipient'), false);
});

test('published scenes are read-only and only offer the next safe lifecycle action', () => {
  assert.match(workspaceSource, /已发布/);
  assert.match(workspaceSource, /新建版本/);
  assert.match(workspaceSource, /停用/);
  assert.match(workspaceSource, /disabled=\{isReadOnly \|\| !canEdit\}/);
  assert.match(workspaceSource, /已停用/);
  assert.equal(workspaceSource.includes('新通知不会自动改走旧配置'), false);
});

test('scene editor has no legacy migration input', () => {
  assert.equal(workspaceSource.includes('legacySceneBindingId'), false);
  assert.equal(workspaceSource.includes('导入历史规则'), false);
  assert.equal(workspaceSource.includes('历史规则编号'), false);
});
