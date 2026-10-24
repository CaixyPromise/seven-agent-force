'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Empty, Grid, Segmented, Skeleton, Tabs, Typography } from 'antd';
import {
  ApartmentOutlined,
  AreaChartOutlined,
  ClockCircleOutlined,
  DashboardOutlined,
  DatabaseOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { motion, useReducedMotion } from 'framer-motion';
import { useQuery } from '@tanstack/react-query';
import type { ObservabilityOverview } from '@/api/observabilityController';
import { getObservabilityOverview } from '@/api/observabilityController';
import { KpiCard, StatusCapsule } from '@/app/system/observability/components/ObservabilitySurface';
import { resolveHealthTone } from '@/app/system/observability/components/observabilityTone';
import { formatDateTime } from '@/app/system/observability/components/observabilityFormat';
import { AlertsTab } from '@/app/system/observability/components/tabs/AlertsTab';
import { EventsTab } from '@/app/system/observability/components/tabs/EventsTab';
import { ExtensionsTab } from '@/app/system/observability/components/tabs/ExtensionsTab';
import { JvmTab } from '@/app/system/observability/components/tabs/JvmTab';
import { MiddlewareTab } from '@/app/system/observability/components/tabs/MiddlewareTab';
import { OverviewTab } from '@/app/system/observability/components/tabs/OverviewTab';
import { TrafficTab } from '@/app/system/observability/components/tabs/TrafficTab';

const { Paragraph, Text, Title } = Typography;

const RANGE_OPTIONS = [
  { label: '1h', value: '1h' },
  { label: '6h', value: '6h' },
  { label: '24h', value: '24h' },
  { label: '7d', value: '7d' },
] as const;

const VIEW_OPTIONS = [
  { key: 'overview', label: '总览', icon: <DashboardOutlined /> },
  { key: 'traffic', label: '流量诊断', icon: <AreaChartOutlined /> },
  { key: 'jvm', label: 'JVM', icon: <ClockCircleOutlined /> },
  { key: 'middleware', label: '中间件', icon: <DatabaseOutlined /> },
  { key: 'events', label: '事件与来源', icon: <ClockCircleOutlined /> },
  { key: 'alerts', label: '告警与异常', icon: <WarningOutlined /> },
  { key: 'extensions', label: '扩展面板', icon: <ApartmentOutlined /> },
] as const;

const DEFAULT_RANGE_KEY = '24h';
const DEFAULT_VIEW_KEY = 'overview';

function LoadingShell() {
  return (
    <div style={{ display: 'grid', gap: 18 }}>
      <Skeleton.Node active style={{ width: '100%', height: 150, borderRadius: 28 }} />
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, minmax(0, 1fr))', gap: 18 }}>
        {Array.from({ length: 3 }).map((_, index) => (
          <Skeleton.Node key={index} active style={{ width: '100%', height: 132, borderRadius: 26 }} />
        ))}
      </div>
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 18 }}>
        <Skeleton.Node active style={{ width: '100%', height: 360, borderRadius: 28 }} />
        <Skeleton.Node active style={{ width: '100%', height: 360, borderRadius: 28 }} />
      </div>
    </div>
  );
}

export default function ObservabilityPage() {
  const screens = Grid.useBreakpoint();
  const prefersReducedMotion = useReducedMotion();
  const isDesktop = !!screens.lg;
  const isWide = !!screens.xl;

  const [selectedPlatformKey, setSelectedPlatformKey] = useState(() => {
    if (typeof window === "undefined") {
      return '';
    }
    return new URLSearchParams(window.location.search).get('platform')?.trim() || '';
  });
  const [selectedRangeKey, setSelectedRangeKey] = useState(() => {
    if (typeof window === "undefined") {
      return DEFAULT_RANGE_KEY;
    }
    return new URLSearchParams(window.location.search).get('range')?.trim() || DEFAULT_RANGE_KEY;
  });
  const [selectedViewKey, setSelectedViewKey] = useState(() => {
    if (typeof window === "undefined") {
      return DEFAULT_VIEW_KEY;
    }
    return new URLSearchParams(window.location.search).get('view')?.trim() || DEFAULT_VIEW_KEY;
  });

  const syncSearchParams = useCallback((platformKey: string, rangeKey: string, viewKey: string) => {
    if (typeof window === 'undefined') {
      return;
    }
    const params = new URLSearchParams(window.location.search);
    if (platformKey) {
      params.set('platform', platformKey);
    } else {
      params.delete('platform');
    }
    params.set('range', rangeKey);
    params.set('view', viewKey);
    const nextQuery = params.toString();
    const nextUrl = nextQuery ? `${window.location.pathname}?${nextQuery}` : window.location.pathname;
    window.history.replaceState(window.history.state, '', nextUrl);
  }, []);

  const overviewQuery = useQuery({
    queryKey: ['observability', 'overview', selectedPlatformKey, selectedRangeKey],
    queryFn: () =>
      getObservabilityOverview({
        platform: selectedPlatformKey || undefined,
        range: selectedRangeKey,
      }),
    staleTime: 60 * 1000,
    refetchInterval: 60 * 1000,
  });

  const overview: ObservabilityOverview | undefined = overviewQuery.data?.data;
  const platforms = useMemo(() => overview?.platforms ?? [], [overview?.platforms]);
  const effectivePlatformKey = overview?.selectedPlatformKey || selectedPlatformKey;
  const effectiveRangeKey = overview?.rangeKey || selectedRangeKey;

  useEffect(() => {
    document.title = '可观测中心 - Seven Agent Force';
  }, []);

  useEffect(() => {
    if (!overview) {
      return;
    }
    syncSearchParams(effectivePlatformKey, effectiveRangeKey, selectedViewKey);
  }, [effectivePlatformKey, effectiveRangeKey, overview, selectedViewKey, syncSearchParams]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    const syncFromLocation = () => {
      const params = new URLSearchParams(window.location.search);
      setSelectedPlatformKey(params.get('platform')?.trim() || '');
      setSelectedRangeKey(params.get('range')?.trim() || DEFAULT_RANGE_KEY);
      setSelectedViewKey(params.get('view')?.trim() || DEFAULT_VIEW_KEY);
    };
    window.addEventListener('popstate', syncFromLocation);
    return () => window.removeEventListener('popstate', syncFromLocation);
  }, []);

  const selectedPlatform = useMemo(
    () =>
      platforms.find((item) =>
        effectivePlatformKey ? item.platformKey === effectivePlatformKey : item.platformKey === overview?.selectedPlatformKey,
      ) ?? platforms[0],
    [effectivePlatformKey, overview?.selectedPlatformKey, platforms],
  );
  const healthTone = resolveHealthTone(selectedPlatform?.status || 'healthy');

  const alertSummary = useMemo(() => {
    const alerts = selectedPlatform?.alerts || [];
    return {
      total: alerts.length,
      critical: alerts.filter((item) => item.severity === 'critical').length,
      watch: alerts.filter((item) => item.severity !== 'critical').length,
    };
  }, [selectedPlatform?.alerts]);

  const dependencySummary = useMemo(() => {
    const items = selectedPlatform?.dependencyMatrix || [];
    return {
      healthy: items.filter((item) => item.status === 'up').length,
      degraded: items.filter((item) => item.status !== 'up').length,
      total: items.length,
    };
  }, [selectedPlatform?.dependencyMatrix]);

  if (overviewQuery.isLoading) {
    return (
      <div style={{ padding: '24px 24px 48px' }}>
        <LoadingShell />
      </div>
    );
  }

  if (overviewQuery.isError) {
    return (
      <div style={{ padding: '24px 24px 48px' }}>
        <Alert
          type="error"
          showIcon
          title="统一可观测中心加载失败"
          description="请检查可观测聚合接口、权限配置以及后台原始观测数据访问边界。"
          action={
            <Button size="small" onClick={() => overviewQuery.refetch()}>
              重新加载
            </Button>
          }
        />
      </div>
    );
  }

  if (!overview || !selectedPlatform) {
    return (
      <div style={{ padding: '24px 24px 48px' }}>
        <Empty description="暂无可观测性数据" />
      </div>
    );
  }

  return (
    <motion.div
      {...(prefersReducedMotion
        ? {}
        : { initial: { opacity: 0, y: 12 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.34 } })}
      style={{
        minHeight: '100%',
        padding: '24px 24px 52px',
        color: '#0f172a',
        background:
          'linear-gradient(180deg, rgba(224, 242, 254, 0.72) 0%, rgba(240, 249, 255, 0.9) 32%, rgba(255,255,255,0.98) 100%)',
      }}
    >
      <div style={{ display: 'grid', gap: 18 }}>
        <motion.section
          {...(prefersReducedMotion
            ? {}
            : { initial: { opacity: 0, y: 16 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.32, ease: 'easeOut' } })}
          style={{
            display: 'grid',
            gap: 20,
            padding: '22px 24px',
            borderRadius: 32,
            border: '1px solid rgba(191, 219, 254, 0.88)',
            background: `
              radial-gradient(circle at 12% 16%, rgba(186, 230, 253, 0.46), transparent 24%),
              radial-gradient(circle at 88% 12%, rgba(219, 234, 254, 0.34), transparent 22%),
              linear-gradient(145deg, rgba(255,255,255,0.98) 0%, rgba(248,250,252,0.98) 48%, rgba(239,246,255,0.96) 100%)
            `,
            boxShadow: '0 24px 58px -44px rgba(14, 116, 144, 0.28)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 18, flexWrap: 'wrap' }}>
            <div style={{ display: 'grid', gap: 10, maxWidth: 680 }}>
              <div style={{ display: 'grid', gap: 6 }}>
                <Title
                  level={2}
                  style={{
                    margin: 0,
                    color: '#0f172a',
                    fontSize: 'clamp(28px, 4vw, 42px)',
                    lineHeight: 1.08,
                    letterSpacing: '-0.04em',
                    fontFamily: '"DIN Alternate", "IBM Plex Sans", "PingFang SC", sans-serif',
                  }}
                >
                  观测中心
                </Title>
                <Paragraph style={{ marginBottom: 0, color: '#64748b' }}>
                  当前展示 {selectedPlatform.platformName} 的观测数据。趋势、诊断和面板结构已经按未来多平台扩展预留了固定槽位。
                </Paragraph>
              </div>
            </div>

            <div style={{ display: 'grid', gap: 12, minWidth: isDesktop ? 360 : '100%' }}>
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  gap: 12,
                  flexWrap: 'wrap',
                }}
              >
                <div style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                  <span
                    style={{
                      width: 10,
                      height: 10,
                      borderRadius: '999px',
                      background: healthTone.dot,
                      boxShadow: `0 0 20px ${healthTone.glow}`,
                    }}
                  />
                  <Text style={{ color: '#0f172a', fontWeight: 600 }}>{healthTone.label}</Text>
                  <Text style={{ color: '#64748b' }}>{overview.timeRangeLabel}</Text>
                </div>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={() => overviewQuery.refetch()}
                  style={{
                    borderRadius: 999,
                    borderColor: 'rgba(15, 23, 42, 0.12)',
                    background: '#0f172a',
                    color: '#fff',
                  }}
                >
                  刷新
                </Button>
              </div>

              <div style={{ display: 'grid', gap: 10 }}>
                <Text style={{ color: '#64748b', fontSize: 12 }}>平台</Text>
                <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
                  {platforms.map((item) => (
                    <Button
                      key={item.platformKey}
                      type={item.platformKey === selectedPlatform.platformKey ? 'primary' : 'default'}
                      onClick={() => {
                        setSelectedPlatformKey(item.platformKey);
                        syncSearchParams(item.platformKey, effectiveRangeKey, selectedViewKey);
                      }}
                      style={{
                        borderRadius: 999,
                        height: 40,
                        paddingInline: 16,
                        ...(item.platformKey === selectedPlatform.platformKey
                          ? { background: '#0f172a', borderColor: '#0f172a' }
                          : { background: 'rgba(255,255,255,0.92)', borderColor: 'rgba(191, 219, 254, 0.8)' }),
                      }}
                    >
                      {item.platformName}
                    </Button>
                  ))}
                </div>
              </div>

              <div style={{ display: 'grid', gap: 10 }}>
                <Text style={{ color: '#64748b', fontSize: 12 }}>时间范围</Text>
                <Segmented
                  block
                  value={effectiveRangeKey}
                  options={RANGE_OPTIONS.map((item) => ({ label: item.label, value: item.value }))}
                  onChange={(value) => {
                    const nextValue = String(value);
                    setSelectedRangeKey(nextValue);
                    syncSearchParams(selectedPlatform.platformKey, nextValue, selectedViewKey);
                  }}
                />
              </div>
            </div>
          </div>

          <div
            style={{
              display: 'grid',
              gridTemplateColumns: isWide ? '1.4fr 1fr 1fr 1fr' : isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
              gap: 14,
            }}
          >
            <div
              style={{
                display: 'grid',
                gap: 10,
                padding: '18px 20px',
                borderRadius: 24,
                border: '1px solid rgba(191, 219, 254, 0.86)',
                background: 'rgba(255,255,255,0.84)',
              }}
            >
              <div style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
                <SafetyCertificateOutlined style={{ color: '#0284c7' }} />
                <Text style={{ color: '#0f172a', fontWeight: 600 }}>当前平台态势</Text>
              </div>
              <StatusCapsule label={healthTone.label} tone={healthTone} />
              <Text style={{ color: '#475569' }}>
                健康探针 Liveness {selectedPlatform.healthSummary?.livenessStatus || 'unknown'} · Readiness{' '}
                {selectedPlatform.healthSummary?.readinessStatus || 'unknown'}
              </Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>
                Uptime {selectedPlatform.healthSummary?.uptimeLabel || '未知'}
              </Text>
            </div>

            <div
              style={{
                display: 'grid',
                gap: 8,
                padding: '18px 20px',
                borderRadius: 24,
                border: '1px solid rgba(191, 219, 254, 0.86)',
                background: 'rgba(255,255,255,0.84)',
              }}
            >
              <Text style={{ color: '#64748b', fontSize: 12 }}>版本</Text>
              <Text style={{ color: '#0f172a', fontSize: 18, fontWeight: 700 }}>
                {selectedPlatform.healthSummary?.version || 'unknown'}
              </Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>
                commit {selectedPlatform.healthSummary?.gitCommit || 'unknown'}
              </Text>
            </div>

            <div
              style={{
                display: 'grid',
                gap: 8,
                padding: '18px 20px',
                borderRadius: 24,
                border: '1px solid rgba(191, 219, 254, 0.86)',
                background: 'rgba(255,255,255,0.84)',
              }}
            >
              <Text style={{ color: '#64748b', fontSize: 12 }}>最近刷新</Text>
              <Text style={{ color: '#0f172a', fontSize: 18, fontWeight: 700 }}>
                {formatDateTime(overview.generatedAt)}
              </Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>
                平台 {selectedPlatform.platformName}
              </Text>
            </div>

            <div
              style={{
                display: 'grid',
                gap: 8,
                padding: '18px 20px',
                borderRadius: 24,
                border: '1px solid rgba(191, 219, 254, 0.86)',
                background: 'rgba(255,255,255,0.84)',
              }}
            >
              <Text style={{ color: '#64748b', fontSize: 12 }}>观测窗口</Text>
              <Text style={{ color: '#0f172a', fontSize: 18, fontWeight: 700 }}>{overview.timeRangeLabel}</Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>平台切换与范围切换即时生效</Text>
            </div>
          </div>
        </motion.section>

        <motion.section
          {...(prefersReducedMotion
            ? {}
            : { initial: { opacity: 0, y: 16 }, animate: { opacity: 1, y: 0 }, transition: { duration: 0.32, ease: 'easeOut' } })}
          style={{
            display: 'grid',
            gridTemplateColumns: isWide ? 'repeat(6, minmax(0, 1fr))' : isDesktop ? 'repeat(3, minmax(0, 1fr))' : '1fr',
            gap: 14,
          }}
        >
          {overview.headlineMetrics.map((metric, index) => (
            <KpiCard key={metric.key} metric={metric} index={index} />
          ))}
        </motion.section>

        <Tabs
          activeKey={selectedViewKey}
          onChange={(nextKey) => {
            setSelectedViewKey(nextKey);
            syncSearchParams(selectedPlatform.platformKey, selectedRangeKey, nextKey);
          }}
          items={[
            {
              key: 'overview',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[0].icon}
                  {VIEW_OPTIONS[0].label}
                </span>
              ),
              children: (
                <OverviewTab
                  overview={overview}
                  platform={selectedPlatform}
                  isDesktop={isDesktop}
                  isWide={isWide}
                  healthTone={healthTone}
                  alertSummary={alertSummary}
                  dependencySummary={dependencySummary}
                />
              ),
            },
            {
              key: 'traffic',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[1].icon}
                  {VIEW_OPTIONS[1].label}
                </span>
              ),
              children: <TrafficTab platform={selectedPlatform} isDesktop={isDesktop} isWide={isWide} />,
            },
            {
              key: 'jvm',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[2].icon}
                  {VIEW_OPTIONS[2].label}
                </span>
              ),
              children: <JvmTab platform={selectedPlatform} isDesktop={isDesktop} isWide={isWide} />,
            },
            {
              key: 'middleware',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[3].icon}
                  {VIEW_OPTIONS[3].label}
                </span>
              ),
              children: <MiddlewareTab platform={selectedPlatform} isDesktop={isDesktop} isWide={isWide} />,
            },
            {
              key: 'events',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[4].icon}
                  {VIEW_OPTIONS[4].label}
                </span>
              ),
              children: <EventsTab platform={selectedPlatform} isWide={isWide} />,
            },
            {
              key: 'alerts',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[5].icon}
                  {VIEW_OPTIONS[5].label}
                </span>
              ),
              children: <AlertsTab platform={selectedPlatform} isDesktop={isDesktop} isWide={isWide} />,
            },
            {
              key: 'extensions',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {VIEW_OPTIONS[6].icon}
                  {VIEW_OPTIONS[6].label}
                </span>
              ),
              children: <ExtensionsTab platform={selectedPlatform} />,
            },
          ]}
        />
      </div>
    </motion.div>
  );
}
