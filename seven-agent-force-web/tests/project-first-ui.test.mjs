import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import test from 'node:test';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('Project-first routes expose projects and keep task details untouched', () => {
  const routes = read('../config/route.tsx');
  assert.match(routes, /path: '\/workbench',[\s\S]*?name: '工作台'/);
  assert.match(routes, /path: '\/system',[\s\S]*?name: '平台管理'/);
  assert.match(routes, /path: '\/workforce\/projects'/);
  assert.match(routes, /name: '项目'/);
  assert.match(routes, /path: '\/workforce\/projects\/:projectId'/);
  assert.match(routes, /path: '\/workforce\/projects\/:projectId\/tasks\/new'/);
  assert.match(routes, /name: '任务'/);
  assert.match(routes, /path: '\/workforce\/actions'/);
  assert.match(routes, /name: '待我处理'/);
  assert.doesNotMatch(routes, /workforce:project:(?:list|create|query)/);
  assert.doesNotMatch(routes, /\/workforce\/repositories|workforce:repository/);
  assert.equal(existsSync(new URL('../src/app/workforce/repositories/page.tsx', import.meta.url)), false);
  assert.equal(existsSync(new URL('../src/api/repositoryController.ts', import.meta.url)), false);
});

test('task execution limit is optional at creation and visible in task details', () => {
  const composer = read('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx');
  const detail = read('../src/app/workforce/tasks/[taskId]/page.tsx');
  const contract = read('../src/api/projectContract.ts');
  assert.match(composer, /timeLimitMode: 'UNLIMITED'/);
  assert.match(composer, /timeLimitMode === 'CUSTOM'[\s\S]*?executionTimeoutSeconds/);
  assert.match(composer, /不限时/);
  assert.match(composer, /自定义/);
  assert.match(detail, /Agent 执行时限/);
  assert.match(contract, /executionTimeoutSeconds\?: number/);
});

test('sidebar separates daily work from platform AI administration', () => {
  const taskSeed = read('../../seven-agent-force-server/migrations/postgres/20260824121000_workforce_rbac_seed.sql');
  const providerSeed = read('../../seven-agent-force-server/migrations/postgres/20260824123100_workforce_provider_rbac_seed.sql');
  const hookSeed = read('../../seven-agent-force-server/migrations/postgres/20260824124100_workforce_managed_hooks_rbac.sql');
  const processorSeed = read('../../seven-agent-force-server/migrations/postgres/20260825130000_workforce_worker_fleet_mvp.sql');
  const workerSeed = read('../../seven-agent-force-server/migrations/postgres/20260825131000_workforce_worker_fleet_rbac.sql');
  const integrationSeed = read('../../seven-agent-force-server/migrations/postgres/20260830101000_workforce_open_task_integration_rbac.sql');
  const menuRoutes = read('../src/lib/navigation/menuRoutes.tsx');

  assert.match(taskSeed, /'工作台'[\s\S]*?'\/workbench'[\s\S]*?'ProjectOutlined'[\s\S]*?'M'/);
  assert.match(taskSeed, /'AI 能力'[\s\S]*?'\/system\/workforce'[\s\S]*?'M'/);
  assert.match(taskSeed, /parent\.path = '\/workbench'/);
  for (const source of [providerSeed, hookSeed, processorSeed, workerSeed, integrationSeed]) {
    assert.match(source, /parent\.path\s*=\s*'\/system\/workforce'/);
  }
  for (const icon of ['ApiOutlined', 'BellOutlined', 'CodeOutlined', 'FileSearchOutlined', 'ProjectOutlined']) {
    assert.match(menuRoutes, new RegExp(`${icon}: <${icon} \\/>`));
  }
});

test('pending work reuses the task list without exposing task creation controls', () => {
  const actions = read('../src/app/workforce/actions/page.tsx');
  const tasks = read('../src/app/workforce/tasks/page.tsx');
  assert.match(actions, /pendingOnly/);
  assert.match(tasks, /待我处理/);
  assert.match(tasks, /pendingOnly\?'attention':filter/);
  assert.doesNotMatch(tasks, /pendingActionCount\s*\?\?\s*0\)\s*<=\s*0/);
  assert.match(tasks, /!pendingOnly\s*&&\s*canCreate/);
});

test('project detail has one task entry and four business tabs', () => {
  const detail = read('../src/app/workforce/projects/[projectId]/page.tsx');
  assert.match(detail, /任务/);
  assert.match(detail, /概览/);
  assert.match(detail, /项目资料/);
  assert.match(detail, /自动化/);
  assert.match(detail, /发起任务/);
  assert.match(detail, /\/workforce\/projects\/\$\{encodeURIComponent\(current\.id\)\}\/tasks\/new/);
  assert.match(detail, /useInfiniteQuery/);
  assert.match(detail, /getNextPageParam/);
  assert.match(detail, /加载更多/);
});

test('task details link back to the frozen project and do not report disabled review as passed', () => {
  const detail = read('../src/app/workforce/tasks/[taskId]/page.tsx');
  const rail = read('../src/app/workforce/tasks/TaskStatusRail.tsx');
  assert.match(detail, /getProject\(taskQuery\.data!\.projectId\)/);
  assert.match(detail, /\/workforce\/projects\/\$\{encodeURIComponent\(task\.projectId\)\}/);
  assert.match(detail, /所属项目/);
  assert.match(rail, /reviewRequired\s*&&/);
  assert.doesNotMatch(rail, /review\.data\?\.required\s*===\s*false/);
});

test('project forms and project-scoped task composer hide infrastructure selection', () => {
  const form = read('../src/app/workforce/projects/_components/ProjectForm.tsx');
  const composer = read('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx');
  const sources = [form, composer].join('\n');
  for (const forbidden of ['providerAlias', 'providerModel', 'ProviderPicker', 'HarnessPicker', 'harnessAlias', 'Hook Release', 'Processor Release', 'Review Release', 'harnessTimeoutSeconds']) {
    assert.doesNotMatch(sources, new RegExp(forbidden));
  }
  assert.match(sources, /你想完成什么/);
  assert.match(sources, /添加资料/);
  assert.match(sources, /补充完成检查/);
  assert.match(sources, /本次只保存结果/);
  assert.match(form, /<Input\.Password/);
  assert.match(form, /autoComplete="off"/);
  assert.match(form, /更换地址/);
  assert.match(form, /form\.setFieldValue\(\['repository', 'cloneUrl'\], undefined\)/);
  assert.doesNotMatch(form, /repositoryController|listRepositories|connectionId|inspectProjectRepository/);
  assert.doesNotMatch(composer, /repositoryController|listRepositories|connectionId|cloneUrl|选择.*仓库/);
	assert.doesNotMatch(form, /runtimeRequirements/);
	assert.match(composer, /执行镜像/);
	assert.match(composer, /runtimeReleaseId/);
});

test('successful Project mutations clear credential-bearing mutation state', () => {
  const create = read('../src/app/workforce/projects/new/page.tsx');
  const detail = read('../src/app/workforce/projects/[projectId]/page.tsx');
  assert.match(create, /onSuccess:[\s\S]*create\.reset\(\)/);
  assert.match(detail, /onSuccess:[\s\S]*update\.reset\(\)/);
});

test('project pages include responsive and reduced-motion layout contracts', () => {
  const styles = read('../src/app/workforce/projects/projects.module.css');
  assert.match(styles, /@media \(max-width: 760px\)/);
  assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
  assert.match(styles, /min-width:\s*0/);
  assert.match(styles, /overflow-wrap:\s*anywhere/);
});
