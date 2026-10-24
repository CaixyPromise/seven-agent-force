'use client';

import { Empty, Typography } from 'antd';
import { useReducedMotion } from 'framer-motion';
import type { EChartsOption } from 'echarts';
import { BaseEChart } from '@/app/system/observability/components/BaseEChart';

const { Text } = Typography;

type TrendPoint = {
  bucketLabel: string;
};

export interface TrendSeries<T extends TrendPoint> {
  key: Extract<keyof T, string>;
  label: string;
  color: string;
  fill?: string;
}

interface ObservabilityTrendChartProps<T extends TrendPoint> {
  points: T[];
  series: TrendSeries<T>[];
  yValueFormatter?: (value: number) => string;
  height?: number;
}

export function ObservabilityTrendChart<T extends TrendPoint>({
  points,
  series,
  yValueFormatter,
  height = 300,
}: ObservabilityTrendChartProps<T>) {
  const prefersReducedMotion = useReducedMotion();

  if (!points.length) {
    return (
      <div style={{ minHeight: 280, display: 'grid', placeItems: 'center' }}>
        <Empty description="当前窗口暂无趋势数据" />
      </div>
    );
  }

  const axisLabels = points.map((point) => String(point.bucketLabel || ''));
  const option: EChartsOption = {
    animation: !prefersReducedMotion,
    animationDuration: 380,
    animationEasing: 'cubicOut',
    grid: {
      left: 12,
      right: 18,
      top: 20,
      bottom: 46,
      containLabel: true,
    },
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(255,255,255,0.98)',
      borderColor: 'rgba(191, 219, 254, 0.95)',
      borderWidth: 1,
      textStyle: {
        color: '#0f172a',
        fontFamily: 'IBM Plex Sans, PingFang SC, sans-serif',
      },
      extraCssText: 'box-shadow: 0 18px 40px -28px rgba(14,116,144,0.28); border-radius: 16px;',
      valueFormatter: yValueFormatter
        ? (value) =>
            yValueFormatter(Number(Array.isArray(value) ? value[0] : value))
        : undefined,
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: axisLabels,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: '#64748b',
        fontSize: 11,
        fontFamily: 'IBM Plex Mono, SFMono-Regular, ui-monospace, monospace',
        margin: 18,
      },
    },
    yAxis: {
      type: 'value',
      min: 0,
      splitNumber: 4,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: 'rgba(71, 85, 105, 0.78)',
        fontSize: 11,
        fontFamily: 'IBM Plex Mono, SFMono-Regular, ui-monospace, monospace',
        formatter: (value: number) => (yValueFormatter ? yValueFormatter(value) : String(Math.round(value))),
        margin: 14,
      },
      splitLine: {
        lineStyle: {
          color: 'rgba(148, 163, 184, 0.22)',
          type: 'dashed',
        },
      },
    },
    series: series.map((item, index) => ({
      name: item.label,
      type: 'line',
      smooth: 0.26,
      showSymbol: false,
      symbol: 'circle',
      lineStyle: {
        color: item.color,
        width: index === 0 ? 3.5 : 2.5,
        cap: 'round',
        join: 'round',
      },
      areaStyle: item.fill
        ? {
            color: item.fill,
          }
        : undefined,
      emphasis: {
        focus: 'series',
      },
      data: points.map((point) => {
        const rawValue = point[item.key];
        const numeric = typeof rawValue === 'number' ? rawValue : Number(rawValue || 0);
        return Number.isFinite(numeric) ? numeric : 0;
      }),
    })),
  };

  return (
    <div style={{ display: 'grid', gap: 18 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
        {series.map((item) => (
          <div key={item.key} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <span
              style={{
                width: 10,
                height: 10,
                borderRadius: '999px',
                background: item.color,
                boxShadow: `0 0 18px ${item.color}50`,
              }}
            />
            <Text style={{ color: '#334155' }}>{item.label}</Text>
          </div>
        ))}
      </div>

      <div
        style={{
          borderRadius: 28,
          overflow: 'hidden',
          border: '1px solid rgba(203, 213, 225, 0.82)',
          background:
            'linear-gradient(180deg, rgba(248, 250, 252, 0.96) 0%, rgba(241, 245, 249, 0.98) 100%)',
        }}
      >
        <BaseEChart option={option} height={height} />
      </div>
    </div>
  );
}
