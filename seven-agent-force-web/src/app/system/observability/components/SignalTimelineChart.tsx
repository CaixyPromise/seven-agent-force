'use client';

import { Empty, Typography } from 'antd';
import { useReducedMotion } from 'framer-motion';
import type { EChartsOption } from 'echarts';
import { BaseEChart } from '@/app/system/observability/components/BaseEChart';
import type { ObservabilityTimelinePoint } from '@/api/observabilityController';

const { Text } = Typography;

interface SignalTimelineChartProps {
  points: ObservabilityTimelinePoint[];
}

type SeriesConfig = {
  key: 'loginSuccessCount' | 'loginFailureCount';
  label: string;
  color: string;
  fill?: string;
};

const SERIES: SeriesConfig[] = [
  {
    key: 'loginSuccessCount',
    label: '成功事件',
    color: '#0284c7',
    fill: 'rgba(2, 132, 199, 0.12)',
  },
  {
    key: 'loginFailureCount',
    label: '失败事件',
    color: '#dc2626',
  },
];

export function SignalTimelineChart({ points }: SignalTimelineChartProps) {
  const prefersReducedMotion = useReducedMotion();

  if (!points.length) {
    return (
      <div style={{ minHeight: 280, display: 'grid', placeItems: 'center' }}>
        <Empty description="当前窗口暂无趋势数据" />
      </div>
    );
  }

  const option: EChartsOption = {
    animation: !prefersReducedMotion,
    animationDuration: 380,
    animationEasing: 'cubicOut',
    grid: {
      left: 12,
      right: 18,
      top: 22,
      bottom: 48,
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
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: points.map((point) => point.bucketLabel),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: '#64748b',
        fontSize: 12,
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
      },
      splitLine: {
        lineStyle: {
          color: 'rgba(148, 163, 184, 0.22)',
          type: 'dashed',
        },
      },
    },
    series: [
      ...SERIES.map((series, index) => ({
        name: series.label,
        type: 'line' as const,
        smooth: 0.26,
        showSymbol: false,
        symbol: 'circle',
        lineStyle: {
          color: series.color,
          width: index === 0 ? 3.5 : 2.5,
          cap: 'round' as const,
          join: 'round' as const,
        },
        areaStyle: series.fill
          ? {
              color: series.fill,
            }
          : undefined,
        emphasis: {
          focus: 'series' as const,
        },
        data: points.map((point) => point[series.key] || 0),
      })),
      {
        name: '风险事件',
        type: 'scatter' as const,
        symbolSize: 8,
        itemStyle: {
          color: '#d97706',
          borderColor: 'rgba(255,255,255,0.92)',
          borderWidth: 1.5,
          shadowBlur: 12,
          shadowColor: 'rgba(217, 119, 6, 0.24)',
        },
        data: points.map((point, index) => [index, point.riskEventCount || 0]),
      },
    ],
  };

  return (
    <div style={{ display: 'grid', gap: 18 }}>
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          gap: 12,
          alignItems: 'center',
        }}
      >
        {SERIES.map((series) => (
          <div key={series.key} style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <span
              style={{
                width: 10,
                height: 10,
                borderRadius: '999px',
                background: series.color,
                boxShadow: `0 0 18px ${series.color}55`,
              }}
            />
            <Text style={{ color: '#334155' }}>{series.label}</Text>
          </div>
        ))}
        <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <span
            style={{
              width: 10,
              height: 10,
              borderRadius: '999px',
              background: '#d97706',
              boxShadow: '0 0 18px rgba(217, 119, 6, 0.24)',
            }}
          />
          <Text style={{ color: '#334155' }}>风险事件</Text>
        </div>
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
        <BaseEChart option={option} height={300} />
      </div>
    </div>
  );
}
