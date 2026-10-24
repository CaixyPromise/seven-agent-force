import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);
const { FormStore } = require('rc-field-form/lib/useForm');
const { HOOK_MARK } = require('rc-field-form/lib/FieldContext');
const source = readFileSync(new URL('../src/app/workforce/projects/_components/ProjectForm.tsx', import.meta.url), 'utf8');
const ast = ts.createSourceFile('ProjectForm.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
let submit;
let normalize;
function visit(node) {
  if (ts.isVariableDeclaration(node) && node.name.getText(ast) === 'submit') submit = node.parent.parent.getText(ast);
  if (ts.isFunctionDeclaration(node) && node.name?.text === 'normalizeDraft') normalize = node.getText(ast);
  ts.forEachChild(node, visit);
}
visit(ast);
assert.ok(submit && normalize, 'load the production submit callback and normalizer');
const compiled = ts.transpileModule(`export function bind(form: any, onSubmit: any, setReplacingRepository: any, canApproveDelivery: boolean) { ${submit} ${normalize} return submit; }`, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;
const exports = {};
new Function('exports', compiled)(exports);

function formWithCollapsedRules(initialValues) {
  const form = new FormStore(() => {}).getForm();
  const hooks = form.getInternalHooks(HOOK_MARK);
  hooks.setInitialValues(initialValues, true);
  // Only visible fields register; unopened Collapse panels have no entities.
  for (const name of [['name'], ['kind'], ['repository', 'cloneUrl']]) {
    hooks.registerField({
      props: {}, getNamePath: () => name, onStoreChange() {},
      isListField: () => false, isList: () => false,
      isFieldTouched: () => false, isFieldValidating: () => false,
      getMeta: () => ({ touched: false, validating: false, errors: [], warnings: [] }),
    });
  }
  return form;
}

function fixture() {
  return {
    name: ' Existing project ', description: '', kind: 'CODEBASE',
    repository: { cloneUrl: 'https://git.example.test/group/repo.git', defaultRefName: 'main' },
    techStack: ['Go'], agentGuidance: '', completionCheck: { mode: 'NONE' },
    reviewPrompt: 'Keep the existing independent review policy',
    hookReleaseIds: ['9007199254740997'],
    delivery: { mode: 'NEW_BRANCH_AND_DRAFT_REVIEW', authorization: 'ASK_AFTER_GATES', resultBranchPattern: 'codex/{task-id}' },
    notification: { enabled: true, events: ['TASK_COMPLETED'], groupRouteId: '9007199254740999' },
  };
}

test('saving without opening automation panels preserves their existing configuration', async () => {
  const initial = fixture();
  const form = formWithCollapsedRules(initial);
  const validated = await form.validateFields();
  assert.equal(validated.delivery, undefined, 'real form validation only returns registered fields');
  let saved;
  let replacement;
  const save = exports.bind(form, async value => { saved = value; }, value => { replacement = value; }, true);
  await save(validated);
  assert.equal(saved.name, 'Existing project');
  assert.equal(saved.departmentId, undefined);
  assert.deepEqual(saved.delivery, initial.delivery);
  assert.deepEqual(saved.notification, initial.notification);
  assert.deepEqual(saved.hookReleaseIds, initial.hookReleaseIds);
  assert.equal(saved.reviewPrompt, initial.reviewPrompt);
  assert.equal(saved.repository.defaultRefName, 'main');
  assert.equal(form.getFieldValue(['repository', 'cloneUrl']), undefined, 'clear sensitive input only after success');
  assert.equal(replacement, false);
});

test('failed save retains the draft and permission gating still prevents automatic delivery', async () => {
  const form = formWithCollapsedRules(fixture());
  form.setFieldValue(['delivery', 'authorization'], 'AUTO_AFTER_GATES');
  let sent;
  let cleared = false;
  const save = exports.bind(form, async value => { sent = value; throw new Error('save failed'); }, () => { cleared = true; }, false);
  await assert.rejects(() => save(form.getFieldsValue()), /save failed/);
  assert.equal(sent.delivery.authorization, 'ASK_AFTER_GATES');
  assert.equal(form.getFieldValue(['repository', 'cloneUrl']), 'https://git.example.test/group/repo.git');
  assert.equal(cleared, false);
});
