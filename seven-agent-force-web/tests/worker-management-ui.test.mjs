import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const page = readFileSync(new URL('../src/app/workforce/workers/page.tsx', import.meta.url), 'utf8');
const charts = readFileSync(new URL('../src/app/workforce/workers/WorkerFleetCharts.tsx', import.meta.url), 'utf8');
const chartComponent = readFileSync(new URL('../src/app/workforce/_components/AnalyticsChart.tsx', import.meta.url), 'utf8');
const chartConfig = readFileSync(new URL('../src/app/workforce/_components/AnalyticsChartConfig.ts', import.meta.url), 'utf8');
const controller = readFileSync(new URL('../src/api/workforceController.ts', import.meta.url), 'utf8');

test('worker management uses summary charts and compact interactive cards', () => {
  assert.match(page, /Worker 集群概览/);
  assert.match(page, /WorkerResourceChart/);
  assert.match(page, /WorkerSlotChart/);
  assert.match(page, /className=\{styles\.workerGrid\}/);
  assert.match(page, /className=\{`\$\{styles\.workerCard\}/);
  assert.match(page, /WorkerFact icon=\{<DatabaseOutlined \/>\} label="主机地址"/);
  assert.match(page, /className=\{styles\.cardCapacity\}/);
  assert.doesNotMatch(page, /className=\{styles\.workerCardFooter\}/);
  assert.match(charts, /import \{ AnalyticsChart \} from '@\/app\/workforce\/_components\/AnalyticsChart'/);
  assert.match(charts, /<AnalyticsChart[^>]*option=\{option\}/);
  assert.match(chartConfig, /echarts\/core/);
  assert.match(chartComponent, /import \{ echarts \} from '\.\/AnalyticsChartConfig'/);
  assert.match(chartComponent, /echarts\.init\(root\.current/);
  assert.match(chartComponent, /new ResizeObserver\(\(\) => chart\.resize\(\)\)/);
  assert.match(chartComponent, /observer\.observe\(root\.current\)/);
  assert.match(chartComponent, /return \(\) => \{ observer\.disconnect\(\); chart\.dispose\(\); \}/);
});

test('worker detail opens in a responsive drawer with focused tabs', () => {
  assert.match(page, /width="min\(620px, 100vw\)"/);
  assert.match(page, /label: '详情'/);
  assert.match(page, /label: `任务/);
  assert.match(page, /label: `镜像/);
  assert.match(page, /label: '配置'/);
});

test('worker images separate schedulable runtimes, platform components, and blocked local images', () => {
  assert.match(page, /image\.role === 'attempt' && image\.admissionStatus === 'ADMITTED' && image\.presenceStatus === 'PRESENT'/);
  assert.match(page, /image\.role === 'provider-proxy' && image\.presenceStatus === 'PRESENT'/);
  assert.match(page, /image\.role === 'attempt' && image\.admissionStatus !== 'ADMITTED' && image\.presenceStatus === 'PRESENT'/);
  assert.match(page, /其他本地镜像/);
  assert.match(page, /未加入当前 Worker 允许列表/);
  assert.match(page, /镜像未通过运行环境校验/);
  assert.match(page, /title=\{`\$\{schedulableImages\.length\} 个可调度执行镜像`\}/);
  assert.match(page, /label: `镜像 \(\$\{schedulableImages\.length\}\)`/);
  assert.doesNotMatch(page, /admissionEvidence\?\.diagnostic/);
});

test('maximum concurrency is editable through the worker capacity endpoint', () => {
  assert.match(page, /CapacityEditor/);
  assert.match(page, /Worker 上限/);
  assert.match(page, /updateWorkforceWorkerCapacity/);
  assert.match(controller, /\/capacity/);
  assert.match(controller, /maxActiveAttempts, version: toApiIdParam\(worker\.version\)/);
});

test('historical metrics use one fleet query and one on-demand drawer query', () => {
  assert.match(page, /queryKey: \['workforce', 'workers'\]/);
  assert.match(page, /queryKey: \['workforce', 'worker-metrics', 'fleet', metricRange\]/);
  assert.match(page, /queryKey: \['workforce', 'worker-metrics', selectedId, metricRange\]/);
  assert.match(page, /enabled: !!selectedId/);
  assert.match(page, /staleTime: 60_000/);
  assert.equal((page.match(/refetchOnWindowFocus: false/g) || []).length, 2);
  assert.equal((page.match(/refetchOnReconnect: false/g) || []).length, 2);
  assert.match(controller, /\/metrics\?range=/);
  assert.match(charts, /connectNulls: false/);
  assert.match(charts, /Math\.min\(\.\.\.sampleTimes\)/);
  assert.match(charts, /Math\.max\(\.\.\.sampleTimes\)/);
  assert.doesNotMatch(charts, /new Date\(series\.(from|to)\)/);
  assert.match(charts, /type: 'bar'/);
  assert.match(charts, /当前槽位/);
  assert.match(charts, /当前利用率/);
  assert.doesNotMatch(charts, /useState/);
  assert.doesNotMatch(page, /worker-metrics[^\n]*refetchInterval/);
  assert.doesNotMatch(page, /visibleWorkers\.map\([^)]*getWorkforceWorker/s);
  assert.doesNotMatch(page, /workerItems\.map\([^)]*useQuery/s);
});
