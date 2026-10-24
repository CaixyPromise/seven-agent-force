'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Drawer, Empty, Grid, Skeleton, Space, Tabs, Tag, Tooltip, message } from 'antd';
import {
  CloseOutlined,
  CopyOutlined,
  ReloadOutlined,
  CodeOutlined,
} from '@ant-design/icons';
import {
  getDockerContainerStats,
  type DockerContainerDetailView,
  type DockerContainerPortView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { formatBytes } from '../../components/dockerConsole';
import {
  formatContainerStateLabel,
  formatOperationTypeLabel,
  normalizeState,
} from '../../components/dockerFormat';
import { DockerLogConsole } from '../../components/DockerLogConsole';
import { DockerTerminalDrawer } from '../../components/DockerTerminalDrawer';

interface ContainerDetailDrawerProps {
  open: boolean;
  loading?: boolean;
  detail: DockerContainerDetailView | null;
  initialTab?: 'overview' | 'inspect' | 'stats' | 'logs';
  onClose: () => void;
  onRefresh?: () => void;
}

interface StatsPoint {
  timestamp: number;
  cpuPercent: number;
  memoryUsage: number;
  memoryLimit: number;
  memoryPercent: number;
  networkRxBytes: number;
  networkTxBytes: number;
  networkRxBytesPerSec: number;
  networkTxBytesPerSec: number;
}

interface RawNetworkCounters {
  rx_bytes?: number;
  tx_bytes?: number;
}

interface RawStatsShape {
  cpu_stats?: {
    cpu_usage?: {
      total_usage?: number;
      percpu_usage?: number[];
    };
    system_cpu_usage?: number;
    online_cpus?: number;
  };
  precpu_stats?: {
    cpu_usage?: {
      total_usage?: number;
    };
    system_cpu_usage?: number;
  };
  memory_stats?: {
    usage?: number;
    limit?: number;
  };
  networks?: Record<string, RawNetworkCounters>;
}

function shortId(id?: string) {
  return id ? id.slice(0, 12) : '-';
}

function formatTime(timestamp?: number) {
  if (!timestamp) {
    return '-';
  }
  return new Date(timestamp * 1000).toLocaleString();
}

function formatRelativeTime(timestamp?: number) {
  if (!timestamp) {
    return '-';
  }
  const diffSeconds = Math.max(0, Math.floor((Date.now() - timestamp * 1000) / 1000));
  if (diffSeconds < 60) {
    return '刚刚';
  }
  if (diffSeconds < 3600) {
    return `${Math.floor(diffSeconds / 60)} 分钟`;
  }
  if (diffSeconds < 86400) {
    return `${Math.floor(diffSeconds / 3600)} 小时`;
  }
  return `${Math.floor(diffSeconds / 86400)} 天`;
}

function formatPort(port: DockerContainerPortView) {
  const protocol = port.type || 'tcp';
  if (port.publicPort) {
    return `${port.publicPort}:${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
  }
  return `${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
}

function copyText(value: string, successText: string) {
  if (!value) {
    return;
  }
  void navigator.clipboard.writeText(value).then(
    () => message.success(successText),
    () => message.error('复制失败'),
  );
}

function toNumber(value: unknown) {
  const numeric = Number(value);
  return Number.isFinite(numeric) ? numeric : 0;
}

function parseStats(raw: Record<string, unknown>, previous?: StatsPoint): StatsPoint {
  const stats = raw as RawStatsShape;
  const cpuDelta =
    toNumber(stats.cpu_stats?.cpu_usage?.total_usage) -
    toNumber(stats.precpu_stats?.cpu_usage?.total_usage);
  const systemDelta =
    toNumber(stats.cpu_stats?.system_cpu_usage) -
    toNumber(stats.precpu_stats?.system_cpu_usage);
  const cpuCount =
    toNumber(stats.cpu_stats?.online_cpus) ||
    stats.cpu_stats?.cpu_usage?.percpu_usage?.length ||
    1;
  const cpuPercent = systemDelta > 0 && cpuDelta > 0 ? (cpuDelta / systemDelta) * cpuCount * 100 : 0;
  const memoryUsage = toNumber(stats.memory_stats?.usage);
  const memoryLimit = toNumber(stats.memory_stats?.limit);
  const memoryPercent = memoryLimit > 0 ? (memoryUsage / memoryLimit) * 100 : 0;
  const networks = Object.values(stats.networks || {});
  const rxBytes = networks.reduce((total, item) => total + toNumber(item.rx_bytes), 0);
  const txBytes = networks.reduce((total, item) => total + toNumber(item.tx_bytes), 0);
  const timestamp = Date.now();
  const elapsedSeconds = previous ? Math.max(1, (timestamp - previous.timestamp) / 1000) : 1;

  return {
    timestamp,
    cpuPercent,
    memoryUsage,
    memoryLimit,
    memoryPercent,
    networkRxBytes: rxBytes,
    networkTxBytes: txBytes,
    networkRxBytesPerSec: previous ? Math.max(0, (rxBytes - previous.networkRxBytes) / elapsedSeconds) : 0,
    networkTxBytesPerSec: previous ? Math.max(0, (txBytes - previous.networkTxBytes) / elapsedSeconds) : 0,
  };
}

function MiniTrend({ points, field, color }: { points: StatsPoint[]; field: keyof StatsPoint; color: string }) {
  const values = points.map((point) => Number(point[field]) || 0);
  const max = Math.max(...values, 1);
  const polyline = values
    .map((value, index) => {
      const x = values.length <= 1 ? 0 : (index / (values.length - 1)) * 120;
      const y = 42 - (value / max) * 34;
      return `${x},${y}`;
    })
    .join(' ');

  return (
    <svg className="mt-3 h-12 w-full" viewBox="0 0 120 48" preserveAspectRatio="none">
      <polyline fill="none" points={polyline} stroke={color} strokeWidth="2.4" />
    </svg>
  );
}

function MetricCard({
  label,
  value,
  hint,
  points,
  field,
  color,
}: {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  points: StatsPoint[];
  field: keyof StatsPoint;
  color: string;
}) {
  return (
    <div className="rounded-xl border border-slate-100 bg-white p-3 shadow-[0_6px_18px_rgba(15,23,42,0.04)]">
      <div className="text-[11px] text-slate-500">{label}</div>
      <div className="mt-1 text-[17px] font-semibold text-slate-950">{value}</div>
      {hint ? <div className="mt-1 text-xs text-slate-400">{hint}</div> : null}
      <MiniTrend points={points} field={field} color={color} />
    </div>
  );
}

function InfoRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[82px_minmax(0,1fr)] gap-4 py-[7px] text-[13px] leading-6">
      <div className="font-medium text-slate-500">{label}</div>
      <div className="min-w-0 text-slate-900">{children}</div>
    </div>
  );
}

function StatePill({ state }: { state?: string | null }) {
  const normalized = (state || '').toLowerCase();
  const isRunning = normalized === 'running' || normalized === 'restarting';
  const isError = normalized === 'dead' || normalized === 'failed' || normalized === 'error';
  const className = isRunning
    ? 'bg-blue-50 text-blue-600'
    : isError
      ? 'bg-red-50 text-red-600'
      : 'bg-slate-100 text-slate-600';
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1 text-[13px] font-medium ${className}`}>
      <span className={`inline-block h-2 w-2 rounded-full ${isRunning ? 'bg-blue-500' : isError ? 'bg-red-500' : 'bg-slate-400'}`} />
      {formatContainerStateLabel(state || undefined)}
    </span>
  );
}

export function ContainerDetailDrawer({
  open,
  loading = false,
  detail,
  initialTab = 'overview',
  onClose,
  onRefresh,
}: ContainerDetailDrawerProps) {
  const screens = Grid.useBreakpoint();
  const container = detail?.container;
  const [activeTab, setActiveTab] = useState('overview');
  const [statsPoints, setStatsPoints] = useState<StatsPoint[]>([]);
  const latestPointRef = useRef<StatsPoint | undefined>(undefined);
  const [statsLoading, setStatsLoading] = useState(false);
  const [terminalOpen, setTerminalOpen] = useState(false);
  const permissions = usePermissionFlags({
    canTerminal: DOCKER_PERMISSIONS.CONTAINER_TERMINAL,
    canLogs: DOCKER_PERMISSIONS.CONTAINER_LOGS,
  });

  useEffect(() => {
    setActiveTab(initialTab === 'logs' && !permissions.canLogs ? 'overview' : initialTab);
    setStatsPoints([]);
    latestPointRef.current = undefined;
    setTerminalOpen(false);
  }, [container?.id, initialTab, permissions.canLogs]);

  const loadStats = useCallback(async () => {
    if (!container?.id) {
      return;
    }
    setStatsLoading(true);
    try {
      const response = await getDockerContainerStats(container.id);
      const point = parseStats(response.data, latestPointRef.current);
      latestPointRef.current = point;
      setStatsPoints((prev) => [...prev, point].slice(-20));
    } catch (error) {
      message.error((error as Error).message || '获取容器 Stats 失败');
    } finally {
      setStatsLoading(false);
    }
  }, [container?.id]);

  useEffect(() => {
    if (!open || activeTab !== 'stats' || !container?.id) {
      return undefined;
    }
    void loadStats();
    const timer = window.setInterval(() => void loadStats(), 5000);
    return () => window.clearInterval(timer);
  }, [activeTab, container?.id, loadStats, open]);

  const inspectJson = useMemo(() => JSON.stringify(detail?.inspect || {}, null, 2), [detail?.inspect]);
  const lastStats = statsPoints[statsPoints.length - 1];
  const terminalAllowedByState = ['running', 'restarting'].includes(normalizeState(container?.state));
  const canOpenLogs = permissions.canLogs;
  const terminalDisabledReason = !permissions.canTerminal
    ? '当前账号没有容器终端权限'
    : !terminalAllowedByState
      ? '只有运行中或重启中的容器可以打开终端'
      : '';
  const body = loading ? (
    <Skeleton active paragraph={{ rows: 12 }} />
  ) : !detail || !container ? (
    <div className="flex h-full items-center justify-center">
      <Empty description="选择容器查看详情" />
    </div>
  ) : (
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-b border-slate-100 px-5 py-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex min-w-0 items-center gap-2">
              <Tooltip title={container.name}>
                <div className="truncate text-[19px] font-semibold leading-7 tracking-[-0.02em] text-slate-950">
                  {container.name}
                </div>
              </Tooltip>
              <StatePill state={container.state} />
            </div>
            <div className="mt-3 flex items-center gap-2 text-[13px] text-slate-500">
              <span>{shortId(container.id)}</span>
              <Button
                size="small"
                type="text"
                icon={<CopyOutlined />}
                onClick={() => copyText(container.id, '容器 ID 已复制')}
              />
              <Button size="small" type="text" icon={<ReloadOutlined />} onClick={onRefresh} />
              <Tooltip title={terminalDisabledReason || '打开终端'}>
                <Button
                  size="small"
                  type="text"
                  icon={<CodeOutlined />}
                  disabled={!permissions.canTerminal || !terminalAllowedByState}
                  onClick={() => setTerminalOpen(true)}
                />
              </Tooltip>
            </div>
          </div>
          <Button className="text-[20px]" size="small" type="text" icon={<CloseOutlined />} onClick={onClose} />
        </div>
        {container.activeOperation ? (
          <div className="mt-4 rounded-xl border border-blue-100 bg-blue-50 px-3 py-2 text-sm text-blue-700">
            当前操作：{formatOperationTypeLabel(container.activeOperation.operationType)} #{container.activeOperation.operationId}
            {container.activeOperation.currentStage ? ` / ${container.activeOperation.currentStage}` : ''}
          </div>
        ) : null}
      </div>

      <Tabs
        className="min-h-0 flex-1 [&_.ant-tabs-content-holder]:min-h-0 [&_.ant-tabs-content]:h-full [&_.ant-tabs-nav]:mb-4 [&_.ant-tabs-nav]:px-5 [&_.ant-tabs-tab]:py-3 [&_.ant-tabs-tab-btn]:text-[14px] [&_.ant-tabs-tab-active_.ant-tabs-tab-btn]:font-semibold"
        activeKey={activeTab}
        onChange={setActiveTab}
        tabBarGutter={30}
        items={[
          {
            key: 'overview',
            label: '基础信息',
            children: (
              <div className="px-5 pb-5">
                <InfoRow label="容器名称">{container.name}</InfoRow>
                <InfoRow label="容器 ID">
                  <div className="break-all leading-6">
                    {container.id}
                    <Button
                      className="ml-1"
                      size="small"
                      type="text"
                      icon={<CopyOutlined />}
                      onClick={() => copyText(container.id, '容器 ID 已复制')}
                    />
                  </div>
                </InfoRow>
                <InfoRow label="镜像">{container.image || '-'}</InfoRow>
                <InfoRow label="状态">
                  <StatePill state={container.state} />
                </InfoRow>
                <InfoRow label="创建时间">
                  {formatTime(container.created)}{' '}
                  <span className="text-slate-400">({formatRelativeTime(container.created)})</span>
                </InfoRow>
                <InfoRow label="运行时长">{formatRelativeTime(container.created)}</InfoRow>
                <InfoRow label="端口映射">
                  {container.ports?.length ? (
                    <Space size={4} wrap>
                      {container.ports.map((port, index) => (
                        <Tag className="m-0 rounded-lg border-0 bg-slate-100 px-2 py-0.5 text-[12px]" key={`${container.id}-${index}`}>
                          {formatPort(port)}
                        </Tag>
                      ))}
                    </Space>
                  ) : (
                    '-'
                  )}
                </InfoRow>
                <InfoRow label="Compose">
                  {container.composeManaged ? (
                    <Space direction="vertical" size={2}>
                      <Tag className="m-0 rounded-lg border-0 bg-blue-50 px-2 py-0.5 text-[12px] text-blue-600">Compose</Tag>
                      <span className="text-xs text-slate-500">
                        项目：{container.composeProject || '-'} / 服务：{container.composeService || '-'}
                      </span>
                    </Space>
                  ) : (
                    <Tag className="m-0 rounded-lg border-0 bg-slate-100 px-2 py-0.5 text-[12px]">单容器</Tag>
                  )}
                </InfoRow>
                <InfoRow label="重启次数">{container.restartCount ?? 0}</InfoRow>
                <InfoRow label="标签">
                  {container.labels && Object.keys(container.labels).length > 0 ? (
                    <div className="max-h-48 space-y-1 overflow-auto pr-1">
                      {Object.entries(container.labels).map(([key, value]) => (
                        <div className="grid grid-cols-[minmax(0,1fr)] gap-1 text-xs leading-5" key={key}>
                          <span className="inline-block max-w-full truncate rounded-lg bg-slate-100 px-2 py-0.5 text-slate-600" title={key}>
                            {key}
                          </span>
                          <span className="break-all text-slate-800">{value}</span>
                        </div>
                      ))}
                    </div>
                  ) : (
                    '-'
                  )}
                </InfoRow>
              </div>
            ),
          },
          {
            key: 'inspect',
            label: '检查信息',
            children: (
              <div className="space-y-3 px-5 pb-5">
                <div className="flex justify-end">
                  <Button size="small" icon={<CopyOutlined />} onClick={() => copyText(inspectJson, 'Inspect JSON 已复制')}>
                    复制 JSON
                  </Button>
                </div>
                <pre className="max-h-[calc(100vh-230px)] overflow-auto rounded-2xl bg-[#07111f] px-4 py-4 text-[12px] leading-6 text-slate-100 shadow-[0_12px_32px_rgba(2,6,23,0.18)]">
                  {inspectJson}
                </pre>
              </div>
            ),
          },
          {
            key: 'stats',
            label: '资源',
            children: (
              <div className="space-y-4 px-5 pb-5">
                <div className="flex items-center justify-between text-xs text-slate-500">
                  <span>实时采样，最近 20 个点</span>
                  <Button size="small" loading={statsLoading} icon={<ReloadOutlined />} onClick={() => void loadStats()}>
                    自动刷新：5s
                  </Button>
                </div>
                <div className="grid gap-3">
                  <MetricCard
                    label="CPU"
                    value={`${(lastStats?.cpuPercent || 0).toFixed(1)}%`}
                    points={statsPoints}
                    field="cpuPercent"
                    color="#3b82f6"
                  />
                  <MetricCard
                    label="内存"
                    value={formatBytes(lastStats?.memoryUsage)}
                    hint={`${(lastStats?.memoryPercent || 0).toFixed(1)}% / ${formatBytes(lastStats?.memoryLimit)}`}
                    points={statsPoints}
                    field="memoryPercent"
                    color="#22c55e"
                  />
                  <MetricCard
                    label="网络 I/O"
                    value={`${formatBytes(lastStats?.networkRxBytesPerSec)}/s`}
                    hint={`TX ${formatBytes(lastStats?.networkTxBytesPerSec)}/s`}
                    points={statsPoints}
                    field="networkRxBytesPerSec"
                    color="#f97316"
                  />
                </div>
              </div>
            ),
          },
          ...(canOpenLogs
            ? [
                {
                  key: 'logs',
                  label: '日志',
                  children: <DockerLogConsole containerId={container.id} active={open && activeTab === 'logs'} />,
                },
              ]
            : []),
        ]}
      />
      <DockerTerminalDrawer
        open={terminalOpen}
        containerId={container.id}
        containerName={container.name}
        containerState={container.state}
        canUse={permissions.canTerminal}
        onClose={() => setTerminalOpen(false)}
      />
    </div>
  );

  return (
    <Drawer
      open={open}
      width={screens.md ? 420 : '100%'}
      placement="right"
      title={null}
      closable={false}
      mask={false}
      onClose={onClose}
      destroyOnHidden
      styles={{
        body: { padding: 0, background: '#fff', overflow: 'hidden' },
        content: {
          borderTopLeftRadius: screens.md ? 18 : 0,
          borderBottomLeftRadius: screens.md ? 18 : 0,
          overflow: 'hidden',
          boxShadow: '0 18px 56px rgba(15, 23, 42, 0.16)',
        },
      }}
    >
      {body}
    </Drawer>
  );
}
