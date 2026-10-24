'use client';

import { DashboardOutlined, DatabaseOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Tag, Typography } from 'antd';
import type { ObservabilityOverview, ObservabilityPlatform } from '@/api/observabilityController';
import { DependencyMatrixPanel } from '@/app/system/observability/components/DependencyMatrixPanel';
import { InsightCard, SectionShell, StatusCapsule } from '@/app/system/observability/components/ObservabilitySurface';
import { SignalTimelineChart } from '@/app/system/observability/components/SignalTimelineChart';
import { formatDateTime } from '@/app/system/observability/components/observabilityFormat';

const { Paragraph, Text, Title } = Typography;

interface OverviewTabProps {
  overview: ObservabilityOverview;
  platform: ObservabilityPlatform;
  isDesktop: boolean;
  isWide: boolean;
  healthTone: {
    dot: string;
    glow: string;
    label: string;
  };
  alertSummary: {
    total: number;
    critical: number;
    watch: number;
  };
  dependencySummary: {
    healthy: number;
    degraded: number;
    total: number;
  };
}

export function OverviewTab({
  overview,
  platform,
  isDesktop,
  isWide,
  healthTone,
  alertSummary,
  dependencySummary,
}: OverviewTabProps) {
  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: isDesktop ? '1.2fr 1fr' : '1fr',
          gap: 18,
          alignItems: 'start',
        }}
      >
        <SectionShell
          title="平台运行轮廓"
          description="先看当前平台在这个时间窗口里的运行态，再决定往哪个方向深挖。"
          icon={<DashboardOutlined />}
        >
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr',
              gap: 14,
            }}
          >
            <div
              style={{
                display: 'grid',
                gap: 12,
                padding: '18px 18px 16px',
                borderRadius: 24,
                border: '1px solid rgba(191, 219, 254, 0.76)',
                background: 'linear-gradient(180deg, rgba(255,255,255,0.96) 0%, rgba(248,250,252,0.98) 100%)',
              }}
            >
              <StatusCapsule label={healthTone.label} tone={healthTone} />
              <Title level={4} style={{ margin: 0, color: '#0f172a' }}>
                {platform.platformName}
              </Title>
              <Paragraph style={{ marginBottom: 0, color: '#475569' }}>
                {platform.description || '当前平台已经接入统一可观测中心，健康、依赖、事件和异常会在这里汇总展示。'}
              </Paragraph>
              <Text style={{ color: '#64748b', fontSize: 12 }}>
                健康探针 {platform.healthSummary?.overallStatusLabel || '未知'}
              </Text>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
                <Tag color="blue">Liveness {platform.healthSummary?.livenessStatus || 'unknown'}</Tag>
                <Tag color="cyan">Readiness {platform.healthSummary?.readinessStatus || 'unknown'}</Tag>
                <Tag color="default">更新于 {formatDateTime(overview.generatedAt)}</Tag>
              </div>
            </div>

            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
                gap: 12,
              }}
            >
              <InsightCard
                label="运行时长"
                value={platform.healthSummary?.uptimeLabel || '未知'}
                detail="当前实例持续在线时长"
                accent="#0284c7"
              />
              <InsightCard
                label="版本"
                value={platform.healthSummary?.version || 'unknown'}
                detail={`commit ${platform.healthSummary?.gitCommit || 'unknown'}`}
                accent="#111827"
              />
              <InsightCard
                label="告警总数"
                value={String(alertSummary.total)}
                detail={`高优先级 ${alertSummary.critical}，关注 ${alertSummary.watch}`}
                accent="#d97706"
              />
              <InsightCard
                label="依赖状态"
                value={`${dependencySummary.healthy}/${dependencySummary.total}`}
                detail={`异常或降级 ${dependencySummary.degraded}`}
                accent="#0f766e"
              />
              <InsightCard
                label="Docker 操作"
                value={String(overview.extra?.docker?.operationTotal ?? 0)}
                detail={`失败 ${overview.extra?.docker?.operationFailed ?? 0}，策略告警 ${overview.extra?.docker?.policyViolationTotal ?? 0}`}
                accent="#0891b2"
              />
              <InsightCard
                label="Docker Daemon"
                value={overview.extra?.docker?.daemonHealthy ? 'UP' : 'UNKNOWN'}
                detail={`镜像 ${overview.extra?.docker?.imageCount ?? 0}，容器 ${Object.values(overview.extra?.docker?.containerCountByState ?? {}).reduce((sum, item) => sum + item, 0)}`}
                accent="#2563eb"
              />
            </div>
          </div>
        </SectionShell>

        <SectionShell
          title="当前依赖状态"
          description="这一屏先看依赖是否稳定，异常就不用先钻趋势图。"
          icon={<DatabaseOutlined />}
        >
          <DependencyMatrixPanel items={platform.dependencyMatrix || []} />
        </SectionShell>
      </div>

      <SectionShell
        title="平台事件时间线"
        description="用一条时间线把关键事件和风险波动压在一起，先看变化，再决定细查哪里。"
        icon={<SafetyCertificateOutlined />}
      >
        <SignalTimelineChart points={platform.timeline || []} />
      </SectionShell>
    </div>
  );
}
