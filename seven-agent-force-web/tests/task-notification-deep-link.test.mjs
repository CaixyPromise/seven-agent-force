import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

async function transpileModule(path) {
  const source = await readFile(new URL(path, import.meta.url), 'utf8');
  const { outputText, diagnostics = [] } = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    reportDiagnostics: true,
  });
  assert.deepEqual(diagnostics, []);
  return outputText;
}

const apiIdOutput = await transpileModule('../src/lib/apiId.ts');
const apiIdUrl = `data:text/javascript;base64,${Buffer.from(apiIdOutput).toString('base64')}`;
const taskDeepLinkSource = await readFile(
  new URL('../src/app/workforce/tasks/taskDeepLink.ts', import.meta.url),
  'utf8',
);
const linkedTaskDeepLinkSource = taskDeepLinkSource.replace(
  "from '@/lib/apiId'",
  `from '${apiIdUrl}'`,
);
const { outputText: taskDeepLinkOutput, diagnostics: taskDeepLinkDiagnostics = [] } = ts.transpileModule(
  linkedTaskDeepLinkSource,
  {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    reportDiagnostics: true,
  },
);
assert.deepEqual(taskDeepLinkDiagnostics, []);
const taskDeepLink = await import(
  `data:text/javascript;base64,${Buffer.from(taskDeepLinkOutput).toString('base64')}`
);

test('builds a lossless task notification deep link', () => {
  const taskId = '2065424359060983808';
  const turnId = '2065424359060983812';
  assert.equal(
    taskDeepLink.buildTaskDeepLink(taskId, turnId, 'evaluation'),
    `/workforce/tasks/${taskId}?turn=${turnId}&focus=evaluation`,
  );
  assert.equal(
    taskDeepLink.buildTaskDeepLink(taskId, turnId, 'result'),
    `/workforce/tasks/${taskId}?turn=${turnId}&focus=result`,
  );
});

test('parses positive decimal turn identity and an allowlisted focus without Number coercion', () => {
  const turnId = '2065424359060983812';
  assert.deepEqual(
    taskDeepLink.parseTaskDetailLocation(new URLSearchParams(`turn=${turnId}&focus=evaluation`)),
    { turnId, focus: 'evaluation', deliveryId: undefined, reviewId: undefined },
  );
  assert.equal(Number(turnId).toString() === turnId, false);
});

test('rejects malformed turn identities and unsupported focus values', () => {
  for (const invalidTurn of ['', '0', '-1', '1.5', '1e3', 'tsk_1', ' 1', '1 ', '+1', '9223372036854775808']) {
    const parsed = taskDeepLink.parseTaskDetailLocation(
      new URLSearchParams({ turn: invalidTurn, focus: 'evaluation' }),
    );
    assert.equal(parsed.turnId, undefined, invalidTurn);
  }
  for (const invalidFocus of ['', 'Evaluation', '../result', 'billing']) {
    const parsed = taskDeepLink.parseTaskDetailLocation(
      new URLSearchParams({ turn: '2065424359060983812', focus: invalidFocus }),
    );
    assert.equal(parsed.focus, undefined, invalidFocus);
  }
  for (const validFocus of ['continuation', 'delivery']) {
    const parsed = taskDeepLink.parseTaskDetailLocation(
      new URLSearchParams({ turn: '2065424359060983812', focus: validFocus }),
    );
    assert.equal(parsed.focus, validFocus);
  }
  assert.equal(
    taskDeepLink.buildTaskDeepLink('2065424359060983808', '2065424359060983812', 'delivery'),
    '/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=delivery',
  );
});

test('rejects unsafe numeric values before they reach a task route', () => {
  assert.throws(
    () => taskDeepLink.buildTaskDeepLink(9007199254740992, '2065424359060983812', 'result'),
    /lossless decimal string/,
  );
});
