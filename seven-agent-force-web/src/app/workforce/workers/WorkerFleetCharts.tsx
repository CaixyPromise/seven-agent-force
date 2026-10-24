import { useMemo } from 'react';
import type { WorkforceWorkerMetricSeries } from '@/api/workforceController';
import { AnalyticsChart } from '@/app/workforce/_components/AnalyticsChart';
import { echarts } from '@/app/workforce/_components/AnalyticsChartConfig';
import styles from './workers.module.css';

export function WorkerResourceChart({ series, className }: { series: WorkforceWorkerMetricSeries; className?: string }) {
  const data = useMemo(() => fillTimeline(series), [series]);
  const option = useMemo<echarts.EChartsCoreOption>(() => ({
    color: ['#1677ff', '#fa8c16', '#52c41a'], legend: chartLegend(), tooltip: percentTooltip(), grid: chartGrid(),
    xAxis: timeAxis(data.map((point) => point.label)), yAxis: percentAxis(),
    series: [lineSeries('CPU', data.map((point) => point.cpu), '#1677ff'), lineSeries('内存', data.map((point) => point.memory), '#fa8c16'), lineSeries('磁盘', data.map((point) => point.disk), '#52c41a')],
  }), [data]);
  return <AnalyticsChart className={className} ariaLabel="Worker 资源历史趋势图" option={option} />;
}

export function WorkerSlotChart({ series, className }: { series: WorkforceWorkerMetricSeries; className?: string }) {
  const data = useMemo(() => fillTimeline(series), [series]);
  const summary = useMemo(() => {
    const latest = [...data].reverse().find((point) => point.active !== null && point.maximum !== null);
    const peak = data.reduce((maximum, point) => Math.max(maximum, point.active ?? 0), 0);
    const active = latest?.active ?? 0; const maximum = latest?.maximum ?? 0;
    return { active, maximum, peak, utilization: maximum > 0 ? Math.round((active / maximum) * 100) : 0 };
  }, [data]);
  const option = useMemo<echarts.EChartsCoreOption>(() => ({
    color: ['#1677ff', '#94a3b8'], legend: chartLegend(),
    tooltip: { trigger: 'axis', axisPointer: { type: 'line' }, backgroundColor: 'rgba(15,23,42,.92)', borderWidth: 0, textStyle: { color: '#fff' } },
    grid: chartGrid(), xAxis: timeAxis(data.map((point) => point.label)),
    yAxis: { type: 'value', min: 0, minInterval: 1, axisLabel: { color: '#8b95a5' }, splitLine: { lineStyle: { color: '#edf1f6', type: 'dashed' } } },
    series: [
      { name: '使用中', type: 'bar', data: data.map((point) => point.active), barMaxWidth: 18, showBackground: true, backgroundStyle: { color: '#f1f5f9', borderRadius: 4 }, itemStyle: { color: '#1677ff', borderRadius: [4, 4, 0, 0] } },
      { name: '最大槽位', type: 'line', data: data.map((point) => point.maximum), step: 'end', showSymbol: false, connectNulls: false, lineStyle: { width: 1.5, type: 'dashed', color: '#94a3b8' }, itemStyle: { color: '#94a3b8' } },
    ],
  }), [data]);
  return <div className={className}>
    <div className={styles.slotSummary} aria-label={`当前使用 ${summary.active}，最大槽位 ${summary.maximum}，峰值 ${summary.peak}`}>
      <span><small>当前槽位</small><strong>{summary.active}<i>/</i>{summary.maximum}</strong></span>
      <span><small>使用峰值</small><strong>{summary.peak}</strong></span>
      <span><small>当前利用率</small><strong>{summary.utilization}<i>%</i></strong></span>
    </div>
    <AnalyticsChart className={styles.slotChartCanvas} ariaLabel="Worker 槽位历史趋势图" option={option} />
  </div>;
}

function fillTimeline(series: WorkforceWorkerMetricSeries) {
  const byTime = new Map(series.points.map((point) => [new Date(point.at).getTime(), point]));
  const sampleTimes = [...byTime.keys()].filter(Number.isFinite);
  const result: Array<{ label: string; cpu: number | null; memory: number | null; disk: number | null; active: number | null; maximum: number | null }> = [];
  if (sampleTimes.length === 0) return result;
  const from = Math.min(...sampleTimes); const to = Math.max(...sampleTimes); const step = series.stepSeconds * 1000;
  for (let at = from; at <= to; at += step) {
    const point = byTime.get(at);
    result.push({ label: formatTime(at, series.stepSeconds), cpu: point?.cpuUsagePercent ?? null, memory: point?.memoryUsagePercent ?? null, disk: point?.workspaceDiskUsagePercent ?? null, active: point?.activeAttempts ?? null, maximum: point?.maxActiveAttempts ?? null });
  }
  return result;
}

function formatTime(at: number, step: number) {
  const date = new Date(at);
  return step >= 1800 ? date.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
function chartLegend() { return { top: 0, right: 0, itemWidth: 9, itemHeight: 9, textStyle: { color: '#667085', fontSize: 12 } }; }
function chartGrid() { return { left: 8, right: 14, top: 34, bottom: 6, containLabel: true }; }
function timeAxis(data: string[]) { return { type: 'category', boundaryGap: false, data, axisTick: { show: false }, axisLine: { lineStyle: { color: '#e8edf4' } }, axisLabel: { color: '#8b95a5', hideOverlap: true, fontSize: 11 } }; }
function percentAxis() { return { type: 'value', min: 0, max: 100, axisLabel: { color: '#8b95a5', formatter: '{value}%' }, splitLine: { lineStyle: { color: '#edf1f6', type: 'dashed' } } }; }
function percentTooltip() { return { trigger: 'axis', axisPointer: { type: 'line', lineStyle: { color: '#cbd5e1', type: 'dashed' } }, backgroundColor: 'rgba(15,23,42,.92)', borderWidth: 0, textStyle: { color: '#fff' }, valueFormatter: (value: unknown) => value == null ? '无数据' : `${Number(value).toFixed(1)}%` }; }
function lineSeries(name: string, data: Array<number | null>, color: string, area = true) {
  const visiblePoints = data.filter((value) => value !== null).length;
  return { name, type: 'line', data, connectNulls: false, smooth: false, showSymbol: visiblePoints <= 3, symbolSize: 6, lineStyle: { width: 2.2, color }, itemStyle: { color }, areaStyle: area ? { color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [{ offset: 0, color: `${color}24` }, { offset: 1, color: `${color}02` }]) } : undefined };
}
