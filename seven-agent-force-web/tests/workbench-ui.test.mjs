import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('root page dispatches by task list permission and preserves the personal home', () => {
  const page = read('../src/app/page.tsx');
  assert.match(page, /usePermissionAccess\('workforce:task:list'\)/);
  assert.match(page, /canViewWorkforce \? <WorkforceWorkbench \/> : <PersonalSecurityHome \/>/);
});

test('workbench uses one aggregate query without focus refresh or polling', () => {
  const workbench = read('../src/app/workforce/_components/WorkforceWorkbench.tsx');
  assert.equal((workbench.match(/getWorkforceWorkbench\(/g) ?? []).length, 1);
  assert.match(workbench, /queryKey: \['workforce', 'workbench', range\]/);
  assert.match(workbench, /staleTime: 60_000/);
  assert.match(workbench, /refetchOnWindowFocus: false/);
  assert.doesNotMatch(workbench, /refetchInterval/);
});

test('workbench keeps IDs as strings and provides exact deep links', () => {
  const contract = read('../src/api/taskAnalyticsContract.ts');
  const workbench = read('../src/app/workforce/_components/WorkforceWorkbench.tsx');
  assert.match(contract, /taskId: API\.Int64/);
  assert.match(contract, /workerId: API\.Int64/);
  assert.match(workbench, /String\(item\.objectId/);
  assert.match(workbench, /focus=delivery/);
  assert.match(workbench, /item=/);
});

test('workbench implements approved desktop and narrow-screen hierarchy', () => {
  const workbench = read('../src/app/workforce/_components/WorkforceWorkbench.tsx');
  const styles = read('../src/app/workforce/_components/WorkforceWorkbench.module.css');
  for (const label of ['待我处理', '执行中任务', '排队任务', '今日完成', '进行中的工作', '需要关注', '任务活动', '执行资源', '最近交付']) {
    assert.match(workbench, new RegExp(label));
  }
  assert.match(styles, /grid-template-columns: repeat\(4/);
  assert.match(styles, /grid-template-columns: minmax\(0, 2fr\) minmax\(350px, 1fr\)/);
  assert.match(workbench, /className=\{styles\.activityChartFrame\}/);
  assert.match(styles, /\.activityChartFrame \{ min-width: 0; padding: 0 12px; overflow: hidden; \}/);
  assert.doesNotMatch(styles, /\.activityChart[^\n]*margin:/);
  assert.match(styles, /@media \(max-width: 767px\)/);
  assert.match(styles, /prefers-reduced-motion/);
  assert.match(styles, /focus-visible/);
});

test('first-load skeleton preserves the complete workbench layout and permission-dependent actions', () => {
  const workbench = read('../src/app/workforce/_components/WorkforceWorkbench.tsx');
  const styles = read('../src/app/workforce/_components/WorkforceWorkbench.module.css');
  assert.match(workbench, /WorkbenchSkeleton canCreate=\{canCreate\} canViewModelUsage=\{canViewModelUsage\}/);
  for (const section of ['skeletonStat', 'skeletonTaskList', 'skeletonAttentionList', 'skeletonChart', 'skeletonResourceBody', 'skeletonDeliveryRow']) {
    assert.match(workbench, new RegExp(`styles\\.${section}`));
    assert.match(styles, new RegExp(`\\.${section}\\s*\\{`));
  }
  assert.match(workbench, /Header canCreate=\{canCreate\} navigate=\{navigate\}/);
  assert.match(workbench, /!canViewModelUsage \? styles\.singleBottom : ''/);
  assert.match(styles, /@media \(max-width: 767px\)/);
});
