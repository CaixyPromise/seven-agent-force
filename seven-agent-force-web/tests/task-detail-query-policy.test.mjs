import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';
import { QueryClient, QueryObserver } from '@tanstack/react-query';

function compile(path) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { throw new Error(`Unexpected dependency: ${name}`); });
  return exports;
}

const policy = compile('../src/app/workforce/tasks/taskQueryPolicy.ts');

test('review completion fetches available artifacts before terminal polling stops', async () => {
  const client = new QueryClient({ defaultOptions: { queries: { retry:false, gcTime:0 } } });
  let requests = 0;
  let artifactStatus = 'PENDING_REVIEW';
  const queryFn = async () => { requests++; return [{ status:artifactStatus }]; };
  const observer = new QueryObserver(client, { queryKey:policy.artifactQueryKey('9007199254740993','REVIEWING'), queryFn, refetchInterval:false });
  const stop = observer.subscribe(() => {});
  try {
    await observer.refetch();
    assert.equal(observer.getCurrentResult().data[0].status, 'PENDING_REVIEW');
    const before = requests;
    artifactStatus = 'AVAILABLE';
    await new Promise((resolve,reject) => {
      const timer = setTimeout(() => { unsubscribe(); reject(new Error('terminal artifact state did not refresh')); },1000);
      const unsubscribe = observer.subscribe((result) => {
        if (result.data?.[0].status === 'AVAILABLE') { clearTimeout(timer); unsubscribe(); resolve(); }
      });
      observer.setOptions({ queryKey:policy.artifactQueryKey('9007199254740993','SUCCEEDED'), queryFn, refetchInterval:policy.taskRefreshInterval('SUCCEEDED') });
    });
    assert.equal(requests,before+1);
    assert.equal(observer.getCurrentResult().data[0].status,'AVAILABLE');
  } finally { stop(); client.clear(); }
});

test('terminal tasks stop every periodic detail refresh', () => {
  for (const status of ['SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT']) {
    assert.equal(policy.taskRefreshInterval(status), false);
    assert.equal(policy.completionRefreshInterval(status), false);
    assert.equal(policy.reviewRefreshInterval(status), false);
    assert.equal(policy.deliveryRefreshInterval('workflow', status, []), false);
    assert.equal(policy.notificationRefreshInterval(status), false);
  }
});

test('active delivery polling is scoped to visible workflow or details and stops after convergence', () => {
  assert.equal(policy.deliveryRefreshInterval('workflow', 'RUNNING', [{ status: 'RUNNING' }]), 5_000);
  assert.equal(policy.deliveryRefreshInterval('details', 'SUCCEEDED', [{ status: 'CONFIRMING' }]), 5_000);
  assert.equal(policy.deliveryRefreshInterval('results', 'RUNNING', [{ status: 'RUNNING' }]), false);
  assert.equal(policy.deliveryRefreshInterval('workflow', 'SUCCEEDED', [{ status: 'SUCCEEDED' }]), false);
  assert.equal(policy.deliveryRefreshInterval('workflow', 'SUCCEEDED', [{ status: 'OUTCOME_UNKNOWN' }]), false);
});

test('completed checks and reviews stop their own polling before the page unmounts', () => {
  assert.equal(policy.completionRefreshInterval('VALIDATING', { status: 'PASSED' }), false);
  assert.equal(policy.completionRefreshInterval('VALIDATING'), 5_000);
  assert.equal(policy.reviewRefreshInterval('REVIEWING', 'RUNNING', true), 5_000);
  assert.equal(policy.reviewRefreshInterval('MANUAL_REVIEW', 'MANUAL_REVIEW', true), false);
  assert.equal(policy.reviewRefreshInterval('REVIEWING', 'PASSED', true), false);
  assert.equal(policy.reviewRefreshInterval('RUNNING', undefined, false), false);
});

test('task detail mounts expensive technical data only for the details workspace', () => {
  const page = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  assert.match(page, /workspaceTab === 'details' && <div ref=\{detailsRef\}/);
  assert.doesNotMatch(page, /hidden=\{workspaceTab !== 'details'\}/);
  for (const key of ['events', 'recovery', 'task-model-invocations', 'task-result-evaluations', 'task-result-outputs', 'task-result-materializations']) {
    const offset = page.indexOf(`'${key}'`);
    assert.ok(offset >= 0, `missing ${key} query`);
    assert.match(page.slice(offset, offset + 500), /workspaceTab === 'details'/, `${key} is not details-scoped`);
  }
  const invocations = page.indexOf("'task-model-invocations'");
  assert.match(page.slice(invocations, invocations + 650), /engineeringOpen/, 'model invocations must wait for the engineering details disclosure');
  assert.doesNotMatch(page, /refetchInterval:\s*[23]_000/);
});

test('workflow summary has one query owner instead of duplicate component polling', () => {
  const rail = readFileSync(new URL('../src/app/workforce/tasks/TaskStatusRail.tsx', import.meta.url), 'utf8');
  const milestones = readFileSync(new URL('../src/app/workforce/tasks/TaskWorkflowMilestones.tsx', import.meta.url), 'utf8');
  assert.doesNotMatch(rail, /useQuery|getTaskCompletionCheck|getIndependentReview/);
  assert.doesNotMatch(milestones, /useQuery|getTaskCompletionCheck|getIndependentReview/);
});

test('conversation refreshes only changed historical cards during live updates', () => {
  const conversation = readFileSync(new URL('../src/app/workforce/tasks/TaskConversation.tsx', import.meta.url), 'utf8');
  assert.match(conversation, /itemIds\.length\s*\? itemIds/);
  assert.match(conversation, /if \(changedItems\.some\(\(item\) => item\.type === 'DELIVERY'\)\) onDeliveryChange/);
  assert.doesNotMatch(conversation, /new Set\(\[\.\.\.itemIds, \.\.\.itemsRef\.current\.filter/);
});

test('delivery changes refresh the terminal task pending summary as well as delivery details', () => {
  const page = readFileSync(new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url), 'utf8');
  const callback = page.slice(page.indexOf('const refreshDeliveryFromConversation'), page.indexOf('const cancelMutation'));
  assert.match(callback, /queryKey: \['workforce', 'task-deliveries', decodedTaskId\]/);
  assert.match(callback, /queryKey: \['workforce', 'task', decodedTaskId\]/);
});
