'use client';

import type { ReactNode } from 'react';
import { ClockCircleOutlined, DatabaseOutlined } from '@ant-design/icons';
import { Typography } from 'antd';
import type { ObservabilityPlatform, ObservabilityResourceTrendPoint } from '@/api/observabilityController';
import { ObservabilityTrendChart } from '@/app/system/observability/components/ObservabilityTrendChart';
import { InsightCard, SectionShell } from '@/app/system/observability/components/ObservabilitySurface';
import {
  formatBytes,
  formatBytesInUnit,
  formatBytesPerSecond,
  formatDurationMs,
  formatNumber,
  formatPercent,
  resolveByteUnit,
} from '@/app/system/observability/components/observabilityFormat';

const { Text } = Typography;

interface JvmTabProps {
  platform: ObservabilityPlatform;
  isDesktop: boolean;
  isWide: boolean;
}

function resolveHeapWatermark(usedBytes?: number, maxBytes?: number) {
  const denominator = Math.max(maxBytes || 0, 1);
  return ((usedBytes || 0) / denominator) * 100;
}

function resolveJvmHint(platform: ObservabilityPlatform) {
  const jvm = platform.jvmSnapshot;
  const heapWatermark = resolveHeapWatermark(jvm?.heapUsedBytes, jvm?.heapMaxBytes);
  if ((jvm?.fullGcCount || 0) > 0 || (jvm?.fullGcPauseMaxMs || 0) >= 500) {
    return '已经出现 Full GC 或长暂停，优先检查老年代膨胀、大对象分配和缓存上界。';
  }
  if (heapWatermark >= 85) {
    return 'Heap 水位已经接近上限，先排查热点对象、缓存命中和批量加载路径。';
  }
  if ((jvm?.youngGcCount || 0) >= 20) {
    return 'Young GC 频率偏高，说明短生命周期对象分配比较密集，建议结合接口热点一起看。';
  }
  return '当前 JVM 水位平稳，可以继续结合慢接口和中间件波动一起判断压力来源。';
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

function getLastPoint(points: ObservabilityResourceTrendPoint[]) {
  return points.at(-1);
}

function hasNetworkMetrics(points: ObservabilityResourceTrendPoint[]) {
  return points.some((point) => point.networkMetricsAvailable);
}

function hasDiskIoMetrics(points: ObservabilityResourceTrendPoint[]) {
  return points.some((point) => point.diskIoMetricsAvailable);
}

export function JvmTab({ platform, isDesktop, isWide }: JvmTabProps) {
  const points = platform.resourceTrends || [];
  const lastPoint = getLastPoint(points);
  const jvm = platform.jvmSnapshot;
  const heapWatermark = resolveHeapWatermark(jvm?.heapUsedBytes, jvm?.heapMaxBytes);
  const jvmHint = resolveJvmHint(platform);

  const maxMemoryBytes = Math.max(
    0,
    ...points.flatMap((point) => [
      Number(point.heapUsedBytes || 0),
      Number(point.nonHeapUsedBytes || 0),
      Number(point.metaspaceUsedBytes || 0),
    ]),
  );
  const memoryUnit = resolveByteUnit(maxMemoryBytes);
  const maxNetworkBytes = Math.max(
    0,
    ...points.flatMap((point) => [
      Number(point.networkReceiveBytesPerSecond || 0),
      Number(point.networkTransmitBytesPerSecond || 0),
    ]),
  );
  const networkUnit = resolveByteUnit(maxNetworkBytes);
  const maxDiskIoBytes = Math.max(
    0,
    ...points.flatMap((point) => [
      Number(point.diskReadBytesPerSecond || 0),
      Number(point.diskWriteBytesPerSecond || 0),
    ]),
  );
  const diskIoUnit = resolveByteUnit(maxDiskIoBytes);

  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <SectionShell
        title="JVM 细项"
        description="把 CPU、堆、GC、线程和文件句柄拆开，先判断瓶颈是算力、内存还是回收。"
        icon={<DatabaseOutlined />}
      >
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: isWide ? 'repeat(4, minmax(0, 1fr))' : isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
            gap: 12,
          }}
        >
          <InsightCard
            label="系统 CPU"
            value={formatPercent(jvm?.systemCpuUsagePercent || 0)}
            detail={`进程 CPU ${formatPercent(jvm?.processCpuUsagePercent || 0)}`}
            accent="#0284c7"
          />
          <InsightCard
            label="Heap 水位"
            value={formatPercent(heapWatermark)}
            detail={`已用 ${formatBytes(jvm?.heapUsedBytes)} / 上限 ${formatBytes(jvm?.heapMaxBytes)}`}
            accent="#111827"
          />
          <InsightCard
            label="Full GC"
            value={formatNumber(jvm?.fullGcCount || 0)}
            detail={`累计 ${formatDurationMs(jvm?.fullGcPauseTimeMs || 0)} · 峰值 ${formatDurationMs(jvm?.fullGcPauseMaxMs || 0)}`}
            accent="#dc2626"
          />
          <InsightCard
            label="文件句柄"
            value={formatNumber(jvm?.openFileDescriptorCount || 0)}
            detail={`上限 ${formatNumber(jvm?.maxFileDescriptorCount || 0)}`}
            accent="#475569"
          />
          <InsightCard
            label="活跃线程"
            value={formatNumber(jvm?.liveThreadCount || 0)}
            detail={`守护线程 ${formatNumber(jvm?.daemonThreadCount || 0)} · 峰值 ${formatNumber(jvm?.peakThreadCount || 0)}`}
            accent="#7c3aed"
          />
          <InsightCard
            label="Young GC"
            value={formatNumber(jvm?.youngGcCount || 0)}
            detail={`累计 ${formatDurationMs(jvm?.youngGcPauseTimeMs || 0)} · 总 GC ${formatNumber(jvm?.gcPauseCount || 0)}`}
            accent="#d97706"
          />
          <InsightCard
            label="磁盘总量"
            value={formatBytes(jvm?.diskTotalBytes)}
            detail={`剩余 ${formatBytes(jvm?.diskFreeBytes)}`}
            accent="#0f766e"
          />
          <div
            style={{
              display: 'grid',
              gap: 8,
              padding: '18px 18px 16px',
              borderRadius: 24,
              border: '1px solid rgba(191, 219, 254, 0.76)',
              background: 'linear-gradient(180deg, rgba(255,255,255,0.96) 0%, rgba(248,250,252,0.98) 100%)',
            }}
          >
            <Text style={{ color: '#64748b', fontSize: 12 }}>诊断建议</Text>
            <Text style={{ color: '#0f172a', fontWeight: 700 }}>{jvmHint}</Text>
            <Text style={{ color: '#475569', fontSize: 12 }}>下面的趋势图只展示当前实例真实采到的资源数据。</Text>
          </div>
        </div>
      </SectionShell>

      <SectionShell
        title="资源趋势"
        description="把 CPU、内存、网络带宽和磁盘 IO 分成同量纲图表，避免趋势被压扁。"
        icon={<ClockCircleOutlined />}
      >
        <div style={{ display: 'grid', gridTemplateColumns: isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr', gap: 14 }}>
          <ChartSlot
            title="CPU 与磁盘占用"
            description="百分比视角，先判断是不是算力吃紧还是磁盘空间逼近阈值。"
            chart={
              <ObservabilityTrendChart
                points={points}
                series={[
                  { key: 'cpuUsagePercent', label: '系统 CPU', color: '#0284c7', fill: 'rgba(2, 132, 199, 0.10)' },
                  { key: 'processCpuUsagePercent', label: '进程 CPU', color: '#111827' },
                  { key: 'diskUsagePercent', label: '磁盘占用', color: '#0f766e' },
                ]}
                yValueFormatter={(value) => formatPercent(value)}
                height={232}
              />
            }
          />

          <ChartSlot
            title="内存趋势"
            description="堆、非堆和 Metaspace 按同一单位展示，先判断是真涨还是 GC 抖动。"
            chart={
              <ObservabilityTrendChart
                points={points}
                series={[
                  { key: 'heapUsedBytes', label: 'Heap 已用', color: '#111827', fill: 'rgba(15, 23, 42, 0.08)' },
                  { key: 'nonHeapUsedBytes', label: 'Non-Heap', color: '#0284c7' },
                  { key: 'metaspaceUsedBytes', label: 'Metaspace', color: '#0f766e' },
                ]}
                yValueFormatter={(value) => formatBytesInUnit(value, memoryUnit)}
                height={232}
              />
            }
          />

          {hasNetworkMetrics(points) ? (
            <ChartSlot
              title="网络吞吐"
              description={`网卡真实吞吐，统一按 ${networkUnit}/s 展示，用于识别带宽抬升。`}
              chart={
                <div style={{ display: 'grid', gap: 12 }}>
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
                      gap: 12,
                    }}
                  >
                    <InsightCard
                      label="最近接收"
                      value={formatBytesPerSecond(lastPoint?.networkReceiveBytesPerSecond)}
                      detail="最近一个采样桶的平均入站吞吐"
                      accent="#0284c7"
                    />
                    <InsightCard
                      label="最近发送"
                      value={formatBytesPerSecond(lastPoint?.networkTransmitBytesPerSecond)}
                      detail="最近一个采样桶的平均出站吞吐"
                      accent="#0f766e"
                    />
                  </div>
                  <ObservabilityTrendChart
                    points={points}
                    series={[
                      {
                        key: 'networkReceiveBytesPerSecond',
                        label: '接收吞吐',
                        color: '#0284c7',
                        fill: 'rgba(2, 132, 199, 0.10)',
                      },
                      { key: 'networkTransmitBytesPerSecond', label: '发送吞吐', color: '#0f766e' },
                    ]}
                    yValueFormatter={(value) => `${formatBytesInUnit(value, networkUnit)}/s`}
                    height={232}
                  />
                </div>
              }
            />
          ) : null}

          {hasDiskIoMetrics(points) ? (
            <ChartSlot
              title="磁盘吞吐"
              description={`磁盘真实读写吞吐，统一按 ${diskIoUnit}/s 展示，用于识别刷盘或读放大。`}
              chart={
                <div style={{ display: 'grid', gap: 12 }}>
                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns: isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
                      gap: 12,
                    }}
                  >
                    <InsightCard
                      label="最近读取"
                      value={formatBytesPerSecond(lastPoint?.diskReadBytesPerSecond)}
                      detail="最近一个采样桶的平均读吞吐"
                      accent="#111827"
                    />
                    <InsightCard
                      label="最近写入"
                      value={formatBytesPerSecond(lastPoint?.diskWriteBytesPerSecond)}
                      detail="最近一个采样桶的平均写吞吐"
                      accent="#d97706"
                    />
                  </div>
                  <ObservabilityTrendChart
                    points={points}
                    series={[
                      {
                        key: 'diskReadBytesPerSecond',
                        label: '读取吞吐',
                        color: '#111827',
                        fill: 'rgba(15, 23, 42, 0.08)',
                      },
                      { key: 'diskWriteBytesPerSecond', label: '写入吞吐', color: '#d97706' },
                    ]}
                    yValueFormatter={(value) => `${formatBytesInUnit(value, diskIoUnit)}/s`}
                    height={232}
                  />
                </div>
              }
            />
          ) : null}
        </div>
      </SectionShell>
    </div>
  );
}
