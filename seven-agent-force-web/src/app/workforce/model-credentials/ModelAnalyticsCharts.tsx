import { LineChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';
import { useEffect, useRef } from 'react';

echarts.use([LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

type ChartProps = {
  option: echarts.EChartsCoreOption;
  ariaLabel: string;
  className?: string;
};

function Chart({ option, ariaLabel, className }: ChartProps) {
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!root.current) return;
    const chart = echarts.init(root.current, undefined, { renderer: 'canvas' });
    const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    chart.setOption({ ...option, animation: !reduceMotion });
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(root.current);
    return () => {
      observer.disconnect();
      chart.dispose();
    };
  }, [option]);

  return <div ref={root} className={className} role="img" aria-label={ariaLabel} />;
}

export function ModelTrendChart({ points, className }: { points: Array<{ date: string; invocationCount: number; totalTokens: number }>; className?: string }) {
  const labels = points.map((item) => item.date.slice(5).replace('-', '/'));
  return <Chart
    className={className}
    ariaLabel="调用次数与 Token 用量趋势图"
    option={{
      color: ['#1677ff', '#22c3dd'],
      tooltip: { trigger: 'axis', backgroundColor: 'rgba(15,23,42,.92)', borderWidth: 0, textStyle: { color: '#fff' } },
      legend: { top: 0, right: 0, itemWidth: 18, itemHeight: 8, textStyle: { color: '#64748b' }, data: ['调用次数', 'Token 用量'] },
      grid: { left: 12, right: 14, top: 38, bottom: 4, containLabel: true },
      xAxis: { type: 'category', boundaryGap: false, data: labels, axisTick: { show: false }, axisLine: { lineStyle: { color: '#e5eaf1' } }, axisLabel: { color: '#8b95a5', hideOverlap: true } },
      yAxis: [
        { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#edf1f6', type: 'dashed' } }, axisLabel: { color: '#8b95a5' } },
        { type: 'value', splitLine: { show: false }, axisLabel: { color: '#8b95a5', formatter: (value: number) => formatCompact(value) } },
      ],
      series: [
        { name: '调用次数', type: 'line', smooth: true, symbol: 'circle', symbolSize: 6, lineStyle: { width: 3 }, areaStyle: { color: 'rgba(22,119,255,.10)' }, data: points.map((item) => item.invocationCount) },
        { name: 'Token 用量', type: 'line', yAxisIndex: 1, smooth: true, symbol: 'circle', symbolSize: 5, lineStyle: { width: 2 }, areaStyle: { color: 'rgba(34,195,221,.08)' }, data: points.map((item) => item.totalTokens) },
      ],
    }}
  />;
}

export function ModelDistributionChart({ data, className }: { data: Array<{ model: string; invocationCount: number }>; className?: string }) {
  return <Chart
    className={className}
    ariaLabel="模型调用分布图"
    option={{
      color: ['#1677ff', '#22c3dd', '#7357e8', '#6bdaae', '#f6bd5b', '#f4777f', '#93a5bd', '#b7c5d8'],
      tooltip: { trigger: 'item', formatter: '{b}<br/>{c} 次 · {d}%', backgroundColor: 'rgba(15,23,42,.92)', borderWidth: 0, textStyle: { color: '#fff' } },
      legend: { type: 'scroll', orient: 'vertical', right: 0, top: 'middle', width: 130, itemWidth: 9, itemHeight: 9, textStyle: { color: '#64748b', width: 112, overflow: 'truncate' } },
      series: [{ type: 'pie', radius: ['55%', '76%'], center: ['34%', '52%'], avoidLabelOverlap: true, label: { show: false }, itemStyle: { borderColor: '#fff', borderWidth: 3, borderRadius: 5 }, data: data.map((item) => ({ name: item.model, value: item.invocationCount })) }],
    }}
  />;
}

export function LatencySparkline({ values, className }: { values: number[]; className?: string }) {
  return <Chart
    className={className}
    ariaLabel="P95 延迟变化"
    option={{
      grid: { left: 0, right: 0, top: 4, bottom: 0 },
      xAxis: { type: 'category', show: false, data: values.map((_, index) => index) },
      yAxis: { type: 'value', show: false, min: 'dataMin' },
      series: [{ type: 'line', smooth: true, showSymbol: false, lineStyle: { width: 2, color: '#7357e8' }, areaStyle: { color: 'rgba(115,87,232,.10)' }, data: values }],
    }}
  />;
}

function formatCompact(value: number): string {
  return new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}
