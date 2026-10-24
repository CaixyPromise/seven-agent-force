'use client';

import type { ReactNode } from 'react';
import { AreaChartOutlined, WarningOutlined } from '@ant-design/icons';
import { Typography } from 'antd';
import type { ObservabilityEndpointInsight, ObservabilityPlatform } from '@/api/observabilityController';
import { ObservabilityTrendChart } from '@/app/system/observability/components/ObservabilityTrendChart';
import { InsightCard, SectionShell } from '@/app/system/observability/components/ObservabilitySurface';
import { formatDurationMs, formatNumber, formatPercent } from '@/app/system/observability/components/observabilityFormat';

const { Paragraph, Text } = Typography;

interface TrafficTabProps {
  platform: ObservabilityPlatform;
  isDesktop: boolean;
  isWide: boolean;
}

function resolveEndpointTone(severity: string) {
  switch (severity) {
    case 'critical':
      return { border: 'rgba(248, 113, 113, 0.36)', background: 'rgba(254, 242, 242, 0.92)', tag: '#dc2626' };
    case 'warning':
      return { border: 'rgba(251, 191, 36, 0.36)', background: 'rgba(255, 251, 235, 0.94)', tag: '#d97706' };
    default:
      return { border: 'rgba(125, 211, 252, 0.36)', background: 'rgba(240, 249, 255, 0.94)', tag: '#0284c7' };
  }
}

function buildTrafficSummary(platform: ObservabilityPlatform) {
  const trends = platform.trafficTrends || [];
  const latest = trends.at(-1);
  const hottestEndpoint = (platform.endpointInsights || [])[0];
  const maxRequestCount = Math.max(0, ...trends.map((item) => item.requestCount || 0));
  const maxQps = Math.max(0, ...trends.map((item) => item.qps || 0));
  const maxP95 = Math.max(0, ...trends.map((item) => item.p95LatencyMs || 0));
  const max5xx = Math.max(0, ...trends.map((item) => item.error5xxRate || 0));
  return {
    latestRequestCount: latest?.requestCount || 0,
    latestQps: latest?.qps || 0,
    maxRequestCount,
    maxQps,
    maxP95,
    max5xx,
    hottestEndpoint,
  };
}

function ChartSlot({
  title,
  description,
  chart,
}: {
  title: string;
  description: string;
  chart: ReactNode;
}) {
  return (
    <div
      style={{
        display: 'grid',
        gap: 12,
        padding: '16px 16px 18px',
        borderRadius: 24,
        border: '1px solid rgba(203, 213, 225, 0.78)',
        background: 'linear-gradient(180deg, rgba(255,255,255,0.96) 0%, rgba(248,250,252,0.98) 100%)',
      }}
    >
      <div style={{ display: 'grid', gap: 4 }}>
        <Text style={{ color: '#0f172a', fontSize: 16, fontWeight: 700 }}>{title}</Text>
        <Text style={{ color: '#64748b', fontSize: 12 }}>{description}</Text>
      </div>
      {chart}
    </div>
  );
}

function EndpointInsightList({ items }: { items: ObservabilityEndpointInsight[] }) {
  if (!items.length) {
    return (
      <div
        style={{
          padding: '20px 18px',
          borderRadius: 22,
          border: '1px dashed rgba(191, 219, 254, 0.8)',
          color: '#64748b',
        }}
      >
        当前窗口还没有足够的请求样本。
      </div>
    );
  }

  return (
    <div style={{ display: 'grid', gap: 12 }}>
      {items.map((item) => {
        const tone = resolveEndpointTone(item.severity);
        return (
          <div
            key={item.insightKey}
            style={{
              display: 'grid',
              gap: 12,
              padding: '16px 18px',
              borderRadius: 22,
              border: `1px solid ${tone.border}`,
              background: tone.background,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, flexWrap: 'wrap' }}>
              <div style={{ display: 'grid', gap: 6 }}>
                <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}>
                  <span
                    style={{
                      minHeight: 28,
                      padding: '0 10px',
                      borderRadius: 999,
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      background: `${tone.tag}18`,
                      color: tone.tag,
                      fontWeight: 700,
                      fontSize: 12,
                    }}
                  >
                    {item.severityLabel}
                  </span>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>
                    {item.method} {item.uri}
                  </Text>
                </div>
                <Paragraph style={{ marginBottom: 0, color: '#475569' }}>{item.summary}</Paragraph>
              </div>
              <div
                style={{
                  display: 'grid',
                  gridTemplateColumns: 'repeat(3, minmax(0, max-content))',
                  gap: 12,
                  justifyContent: 'end',
                }}
              >
                <div style={{ display: 'grid', gap: 4 }}>
                  <Text style={{ color: '#64748b', fontSize: 12 }}>平均</Text>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatDurationMs(item.averageLatencyMs)}</Text>
                </div>
                <div style={{ display: 'grid', gap: 4 }}>
                  <Text style={{ color: '#64748b', fontSize: 12 }}>峰值</Text>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatDurationMs(item.maxLatencyMs)}</Text>
                </div>
                <div style={{ display: 'grid', gap: 4 }}>
                  <Text style={{ color: '#64748b', fontSize: 12 }}>错误占比</Text>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatPercent(item.errorRate)}</Text>
                </div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export function TrafficTab({ platform, isDesktop, isWide }: TrafficTabProps) {
  const summary = buildTrafficSummary(platform);
  const chartColumns = isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr';

  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <SectionShell
        title="流量与延迟趋势"
        description="吞吐、QPS、延迟和错误率分开展示，避免不同量纲压扁在一张图里。"
        icon={<AreaChartOutlined />}
      >
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: isWide ? 'repeat(4, minmax(0, 1fr))' : isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
            gap: 12,
          }}
        >
          <InsightCard
            label="最近请求量"
            value={formatNumber(summary.latestRequestCount)}
            detail={`窗口峰值 ${formatNumber(summary.maxRequestCount)}`}
            accent="#0284c7"
          />
          <InsightCard
            label="最近 QPS"
            value={summary.latestQps.toFixed(2)}
            detail={`窗口峰值 ${summary.maxQps.toFixed(2)}`}
            accent="#111827"
          />
          <InsightCard
            label="窗口最高 P95"
            value={formatDurationMs(summary.maxP95)}
            detail="超过 500 ms 应优先看慢接口和下游依赖"
            accent="#0f766e"
          />
          <InsightCard
            label="窗口最高 5xx"
            value={formatPercent(summary.max5xx)}
            detail="持续超过 1% 就是明确异常信号"
            accent="#dc2626"
          />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: chartColumns, gap: 14 }}>
          <ChartSlot
            title="请求量趋势"
            description="用于识别是不是单纯放量，还是只有少量高延迟请求。"
            chart={
              <ObservabilityTrendChart
                points={platform.trafficTrends || []}
                series={[{ key: 'requestCount', label: '请求量', color: '#0284c7', fill: 'rgba(2, 132, 199, 0.10)' }]}
                yValueFormatter={(value) => formatNumber(Math.round(value))}
                height={220}
              />
            }
          />
          <ChartSlot
            title="QPS 趋势"
            description="观察瞬时压力变化，判断是不是某个时间点突然冲高。"
            chart={
              <ObservabilityTrendChart
                points={platform.trafficTrends || []}
                series={[{ key: 'qps', label: 'QPS', color: '#111827', fill: 'rgba(15, 23, 42, 0.08)' }]}
                yValueFormatter={(value) => value.toFixed(value >= 10 ? 1 : 2)}
                height={220}
              />
            }
          />
          <ChartSlot
            title="尾延迟趋势"
            description="把 P95 和 P99 放在一起，先看慢请求是持续性还是偶发尖刺。"
            chart={
              <ObservabilityTrendChart
                points={platform.trafficTrends || []}
                series={[
                  { key: 'p95LatencyMs', label: 'P95 延迟', color: '#0f766e', fill: 'rgba(15, 118, 110, 0.10)' },
                  { key: 'p99LatencyMs', label: 'P99 延迟', color: '#0f172a' },
                ]}
                yValueFormatter={(value) => formatDurationMs(value)}
                height={220}
              />
            }
          />
          <ChartSlot
            title="错误率趋势"
            description="区分 4xx 和 5xx，避免把业务拒绝和系统故障混为一谈。"
            chart={
              <ObservabilityTrendChart
                points={platform.trafficTrends || []}
                series={[
                  { key: 'error4xxRate', label: '4xx 占比', color: '#d97706', fill: 'rgba(217, 119, 6, 0.08)' },
                  { key: 'error5xxRate', label: '5xx 占比', color: '#dc2626' },
                ]}
                yValueFormatter={(value) => formatPercent(value)}
                height={220}
              />
            }
          />
        </div>
      </SectionShell>

      <SectionShell
        title="接口诊断"
        description="不只看曲线，把当前窗口里最慢、最重、最危险的接口直接摊开。"
        icon={<WarningOutlined />}
      >
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr',
            gap: 12,
          }}
        >
          <InsightCard
            label="当前热点接口"
            value={summary.hottestEndpoint ? `${summary.hottestEndpoint.method} ${summary.hottestEndpoint.uri}` : '暂无'}
            detail="按请求量、延迟和错误占比综合排序"
            accent="#d97706"
          />
          <InsightCard
            label="最严重结论"
            value={
              summary.max5xx >= 1
                ? '错误率异常'
                : summary.maxP95 >= 500
                  ? '尾延迟偏高'
                  : summary.maxQps >= 5
                    ? '流量抬升'
                    : '整体平稳'
            }
            detail="结合曲线和接口热点，先判定问题主要落在吞吐、延迟还是错误。"
            accent="#7c3aed"
          />
        </div>
        <EndpointInsightList items={platform.endpointInsights || []} />
      </SectionShell>
    </div>
  );
}
