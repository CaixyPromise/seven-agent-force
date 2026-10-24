import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/lib/safeInternalPath.ts', import.meta.url), 'utf8');
const { outputText, diagnostics = [] } = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  reportDiagnostics: true,
});
assert.deepEqual(diagnostics, []);
const paths = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

test('allows exact internal task notification links', () => {
  const value = '/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation';
  assert.equal(paths.safeInternalPath(value), true);
  assert.equal(paths.isTaskDeepLink(value), true);
});

test('keeps other safe inbox links internal without calling them task links', () => {
  assert.equal(paths.safeInternalPath('/notifications?message=receipt_1'), true);
  assert.equal(paths.isTaskDeepLink('/notifications?message=receipt_1'), false);
});

test('rejects absolute, protocol-relative, encoded, traversal and control-character links', () => {
  for (const value of [
    'https://evil.example/workforce/tasks/1',
    '//evil.example/workforce/tasks/1',
    '/workforce\\tasks\\1',
    '/workforce/tasks/%31',
    '/workforce/tasks/1/../2',
    '/workforce/tasks/1\n?turn=2',
    'javascript:alert(1)',
  ]) {
    assert.equal(paths.safeInternalPath(value), false, value);
    assert.equal(paths.isTaskDeepLink(value), false, value);
  }
});

test('only labels positive decimal task routes as task links', () => {
  for (const value of [
    '/workforce/tasks/0?turn=1&focus=result',
    '/workforce/tasks/-1?turn=1&focus=result',
    '/workforce/tasks/tsk_1?turn=1&focus=result',
    '/workforce/tasks/1/extra?turn=1&focus=result',
  ]) {
    assert.equal(paths.isTaskDeepLink(value), false, value);
  }
});
