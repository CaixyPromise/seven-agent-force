import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const read = (path) => readFileSync(new URL(path, import.meta.url), 'utf8');

test('each analytics page uses one aggregate query and no analytics polling', () => {
  const projects = read('../src/app/workforce/projects/page.tsx');
  const detail = read('../src/app/workforce/projects/[projectId]/page.tsx');
  const tasks = read('../src/app/workforce/tasks/page.tsx');
  assert.equal((projects.match(/getProjectListAnalytics\(/g) ?? []).length, 1);
  assert.equal((detail.match(/getProjectAnalytics\(/g) ?? []).length, 1);
  assert.equal((tasks.match(/getTaskAnalytics\(/g) ?? []).length, 1);
  for (const source of [projects, detail, tasks]) {
    const query = source.match(/const analytics\s*=\s*useQuery\([\s\S]*?\);/);
    assert.ok(query, 'analytics query is present');
    assert.doesNotMatch(query[0], /refetchInterval/);
  }
  assert.match(detail, /refetchInterval:activeTab==='work-items'\?5000:false/);
});

test('analytics drill-down writes server list filters into the URL', () => {
  const dashboard = read('../src/app/workforce/_components/AnalyticsDashboard.tsx');
  const tasks = read('../src/app/workforce/tasks/page.tsx');
  const detail = read('../src/app/workforce/projects/[projectId]/page.tsx');
  assert.match(dashboard, /onFilter\(\{view:/);
  assert.match(tasks, /copy\.set\('view'/);
  assert.match(tasks, /copy\.set\('from'/);
  assert.match(detail, /copy\.set\('tab','tasks'\)/);
});

test('shared chart preserves gaps and supports keyboard focus', () => {
  const chart = read('../src/app/workforce/_components/AnalyticsChart.tsx');
  const dashboard = read('../src/app/workforce/_components/AnalyticsDashboard.tsx');
  assert.match(chart, /ResizeObserver/);
  assert.match(chart, /tabIndex=\{0\}/);
  assert.match(dashboard, /connectNulls:false/);
  assert.match(dashboard, /showSymbol:points\.length<=3/);
});

test('list pages prioritize summary, items, then secondary insights', () => {
  const projects = read('../src/app/workforce/projects/page.tsx');
  const tasks = read('../src/app/workforce/tasks/page.tsx');
  assert.ok(projects.indexOf('<ProjectListSummary') < projects.indexOf('className={styles.projectListCard}'));
  assert.ok(projects.indexOf('className={styles.projectListCard}') < projects.indexOf('<ProjectListInsights'));
  assert.ok(tasks.indexOf('<TaskListSummary') < tasks.indexOf('className="workforce-table-card workforce-task-workbench"'));
  assert.ok(tasks.indexOf('className="workforce-table-card workforce-task-workbench"') < tasks.indexOf('<TaskListInsights'));
});

test('analytics charts stay visually subordinate to list content', () => {
  const chartStyles = read('../src/app/workforce/_components/AnalyticsChart.module.css');
  const dashboardStyles = read('../src/app/workforce/_components/AnalyticsDashboard.module.css');
  assert.match(chartStyles, /min-height:\s*220px/);
  assert.match(dashboardStyles, /min-height:\s*92px/);
});

test('task list uses an operational row workbench with responsive identity access', () => {
  const tasks = read('../src/app/workforce/tasks/page.tsx');
  const taskStyles = read('../src/app/workforce/tasks/workforce.css');
  assert.match(tasks, /function TaskListCard/);
  assert.match(tasks, /workforce-task-list-columns/);
  assert.match(tasks, /workforce-task-list-progress-title/);
  assert.match(tasks, /执行环境/);
  assert.match(tasks, /当前进展/);
  assert.match(tasks, /最后更新/);
  assert.match(tasks, /role="link"/);
  assert.match(tasks, /event\.key !== 'Enter' && event\.key !== ' '/);
  assert.match(tasks, /aria-label=\{`查看任务：\$\{task\.name\}`\}/);
  assert.match(taskStyles, /\.workforce-task-list-row\.is-attention/);
  assert.match(taskStyles, /@container \(max-width: 720px\)/);
  assert.match(taskStyles, /grid-template-columns:\s*1fr auto/);
  assert.match(taskStyles, /prefers-reduced-motion/);
});
