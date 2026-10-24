import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const apiSource = await readFile(new URL('../src/api/notificationController.ts', import.meta.url), 'utf8');
const pageSource = await readFile(new URL('../src/app/system/notification/page.tsx', import.meta.url), 'utf8');
const workspaceSource = await readFile(new URL('../src/components/notification/VersionedTemplateWorkspace.tsx', import.meta.url), 'utf8');

for (const [name, source] of [
  ['notification API contract', apiSource],
  ['notification page', pageSource],
  ['versioned template workspace', workspaceSource],
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

test('message templates use the current template workspace only', () => {
  assert.match(pageSource, /消息模板/);
  assert.match(pageSource, /<VersionedTemplateWorkspace canEdit=\{permissions\.templateEdit\} \/>/);
  assert.equal(pageSource.includes('历史模板'), false);
  assert.equal(workspaceSource.includes('导入历史模板'), false);
  assert.equal(workspaceSource.includes('legacyTemplateCode'), false);
  assert.match(apiSource, /template-definitions/);
  assert.match(apiSource, /template-revisions\/preview/);
  assert.match(apiSource, /template-revisions\/\$\{encodeURIComponent\(String\(revisionId\)\)\}\/publish/);
});

test('versioned authoring uses structured editable rows and has no raw JSON input', () => {
  assert.match(workspaceSource, /EditableProTable<EditableVariable>/);
  assert.match(workspaceSource, /变量/);
  assert.match(workspaceSource, /在内容中使用 \{'\{\{\.变量名\}\}'\} 插入变量/);
  assert.equal(workspaceSource.includes('variablesJson'), false);
  assert.equal(workspaceSource.includes('JSON.parse'), false);
  assert.equal(workspaceSource.includes('JSON.stringify'), false);
  assert.equal(workspaceSource.includes('SECRET_EPHEMERAL'), false);
});

test('published and superseded revisions remain readable but never become editable', () => {
  assert.match(apiSource, /revisions\?: TemplateRevision\[\]/);
  assert.match(workspaceSource, /版本记录/);
  assert.match(workspaceSource, /历史版本/);
  assert.match(workspaceSource, /返回当前版本/);
  assert.match(workspaceSource, /viewingHistoricalRevision/);
});

test('preview stays a local editor action instead of entering delivery or inbox paths', () => {
  assert.match(workspaceSource, /previewVersionedNotificationTemplate/);
  assert.match(workspaceSource, /不会发送消息/);
  assert.equal(workspaceSource.includes('testSendNotification'), false);
  assert.equal(workspaceSource.includes('/notification/inbox'), false);
  assert.match(workspaceSource, /createVersionedNotificationTemplateDraft/);
});
