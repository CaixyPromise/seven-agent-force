'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Progress, Space, Tag, message } from 'antd';
import type { EChartsOption } from 'echarts';
import { ReloadOutlined } from '@ant-design/icons';
import {
  getDockerComposeProjects,
  getDockerContainers,
  getDockerNetworks,
  getDockerRegistries,
  getDockerVolumes,
  getLatestDockerOperation,
  getLocalDockerImages,
  type DockerComposeProjectSummaryVO,
  type DockerContainerView,
  type DockerImageView,
  type DockerLatestOperationView,
  type DockerRemoteRegistryView,
  type DockerResourceView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerMetricCards, DockerSurfaceCard } from '../../components/dockerConsole';
import {
  formatContainerStateLabel,
  formatOperationStatusLabel,
  formatOperationTypeLabel,
  normalizeState,
} from '../../components/dockerFormat';
import { DockerWorkbenchChart } from './DockerWorkbenchChart';

interface OverviewSnapshot {
  containers: DockerContainerView[];
  images: DockerImageView[];
  composeProjects: DockerComposeProjectSummaryVO[];
  networks: DockerResourceView[];
  volumes: DockerResourceView[];
  registries: DockerRemoteRegistryView[];
  latestOperation?: DockerLatestOperationView;
}

const emptySnapshot: OverviewSnapshot = {
  containers: [],
  images: [],
  composeProjects: [],
  networks: [],
  volumes: [],
  registries: [],
};

interface DockerOverviewTabProps {
  refreshToken?: number;
  onOpenOperation?: (operationId: API.Int64) => void;
}

export function DockerOverviewTab({ refreshToken = 0, onOpenOperation }: DockerOverviewTabProps) {
  const [snapshot, setSnapshot] = useState<OverviewSnapshot>(emptySnapshot);
  const [loading, setLoading] = useState(false);
  const [lastError, setLastError] = useState('');
  const permissions = usePermissionFlags({
    canQuery: DOCKER_PERMISSIONS.CONTAINER_QUERY,
    canStart: DOCKER_PERMISSIONS.CONTAINER_START,
    canStop: DOCKER_PERMISSIONS.CONTAINER_STOP,
    canRestart: DOCKER_PERMISSIONS.CONTAINER_RESTART,
    canDelete: DOCKER_PERMISSIONS.CONTAINER_DELETE,
    canImages: DOCKER_PERMISSIONS.IMAGE_LIST,
    canRegistries: DOCKER_PERMISSIONS.REGISTRY_LIST,
    canCompose: DOCKER_PERMISSIONS.COMPOSE_PROJECT_LIST,
    canNetworks: DOCKER_PERMISSIONS.NETWORK_LIST,
    canVolumes: DOCKER_PERMISSIONS.VOLUME_LIST,
    canOperations: DOCKER_PERMISSIONS.OPERATION_LIST,
  });

  const canQuery = permissions.canQuery;
  const canImages = permissions.canImages;
  const canCompose = permissions.canCompose;
  const canRegistries = permissions.canRegistries;
  const canNetworks = permissions.canNetworks;
  const canVolumes = permissions.canVolumes;
  const canOperations = permissions.canOperations;

  const loadSnapshot = useCallback(async () => {
    setLoading(true);
    const next: OverviewSnapshot = { ...emptySnapshot };
    const errors: string[] = [];

    const run = async <T,>(enabled: boolean, task: () => Promise<T>, apply: (value: T) => void) => {
      if (!enabled) {
        return;
      }
      try {
        apply(await task());
      } catch (error) {
        errors.push((error as Error).message);
      }
    };

    await Promise.all([
      run(canQuery, async () => getDockerContainers({ current: 1, size: 500 }), (res) => {
        next.containers = res.data.records || [];
      }),
      run(canImages, async () => getLocalDockerImages({ current: 1, size: 500 }), (res) => {
        next.images = res.data.records || [];
      }),
      run(canCompose, async () => getDockerComposeProjects({ current: 1, size: 500 }), (res) => {
        next.composeProjects = res.data.records || [];
      }),
      run(canNetworks, async () => getDockerNetworks({ current: 1, size: 500 }), (res) => {
        next.networks = res.data.records || [];
      }),
      run(canVolumes, async () => getDockerVolumes({ current: 1, size: 500 }), (res) => {
        next.volumes = res.data.records || [];
      }),
      run(canRegistries, getDockerRegistries, (res) => {
        next.registries = res.data || [];
      }),
      run(canOperations, () => getLatestDockerOperation({}), (res) => {
        next.latestOperation = res.data;
      }),
    ]);

    setSnapshot(next);
    setLastError(errors[0] || next.latestOperation?.operation?.errorSummary || '');
    if (errors[0]) {
      message.warning(errors[0]);
    }
    setLoading(false);
  }, [canCompose, canImages, canNetworks, canOperations, canQuery, canRegistries, canVolumes]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadSnapshot();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadSnapshot, refreshToken]);

  const containerStateCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    snapshot.containers.forEach((container) => {
      const state = normalizeState(container.state) || 'unknown';
      counts[state] = (counts[state] || 0) + 1;
    });
    return counts;
  }, [snapshot.containers]);

  const runningContainers = (containerStateCounts.running || 0) + (containerStateCounts.restarting || 0);
  const stoppedContainers = (containerStateCounts.exited || 0) + (containerStateCounts.created || 0) + (containerStateCounts.dead || 0);
  const usedImages = snapshot.images.filter((image) => image.usedByContainerCount > 0).length;
  const enabledRegistries = snapshot.registries.filter((registry) => registry.status !== 1).length;

  const statusChartOption = useMemo<EChartsOption>(() => ({
    tooltip: { trigger: 'item' },
    legend: { bottom: 0 },
    series: [
      {
        type: 'pie',
        radius: ['46%', '70%'],
        data: Object.entries(containerStateCounts).map(([name, value]) => ({
          name: formatContainerStateLabel(name),
          value,
        })),
      },
    ],
  }), [containerStateCounts]);

  const resourceChartOption = useMemo<EChartsOption>(() => ({
    tooltip: { trigger: 'axis' },
    grid: { left: 28, right: 16, top: 24, bottom: 28 },
    xAxis: { type: 'category', data: ['容器', '编排', '镜像', '网络', '存储卷', '仓库'] },
    yAxis: { type: 'value', minInterval: 1 },
    series: [
      {
        type: 'bar',
        barWidth: 26,
        data: [
          snapshot.containers.length,
          snapshot.composeProjects.length,
          snapshot.images.length,
          snapshot.networks.length,
          snapshot.volumes.length,
          snapshot.registries.length,
        ],
      },
    ],
  }), [snapshot]);

  const latestOperation = snapshot.latestOperation?.operation;

  return (
    <div className="space-y-4">
      <DockerMetricCards
        columns={4}
        items={[
          { label: '容器', value: snapshot.containers.length, hint: `${runningContainers} 运行中 / ${stoppedContainers} 已停止` },
          { label: '编排', value: snapshot.composeProjects.length, hint: 'Compose 项目' },
          { label: '镜像', value: snapshot.images.length, hint: `${usedImages} 已使用` },
          { label: '仓库', value: snapshot.registries.length, hint: `${enabledRegistries} 已启用` },
          { label: '网络', value: snapshot.networks.length, hint: '可创建 / 连接 / 清理' },
          { label: '存储卷', value: snapshot.volumes.length, hint: '可创建 / 删除 / 清理' },
          {
            label: '最近任务',
            value: latestOperation ? (
              <button
                type="button"
                className="max-w-full truncate text-left text-2xl font-semibold tracking-normal text-slate-950 hover:text-blue-600"
                onClick={() => onOpenOperation?.(latestOperation.operationId)}
              >
                {formatOperationStatusLabel(latestOperation.status)}
              </button>
            ) : '0',
            hint: latestOperation
              ? (
                <button
                  type="button"
                  className="max-w-full truncate text-left text-sm text-slate-500 hover:text-blue-600"
                  onClick={() => onOpenOperation?.(latestOperation.operationId)}
                  title={`#${latestOperation.operationId} · ${formatOperationTypeLabel(latestOperation.operationType)}`}
                >
                  #{latestOperation.operationId} · {formatOperationTypeLabel(latestOperation.operationType)}
                </button>
              )
              : '暂无任务',
          },
          { label: '最近错误', value: lastError ? '1' : '0', hint: lastError || '暂无错误' },
        ]}
      />

      <div className="grid gap-4 xl:grid-cols-2">
        <DockerSurfaceCard
          title="容器状态分布"
          description="根据当前容器列表派生，不额外新增后端健康接口。"
          extra={<Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadSnapshot()}>刷新</Button>}
        >
          <DockerWorkbenchChart option={statusChartOption} />
        </DockerSurfaceCard>
        <DockerSurfaceCard title="资源数量概览" description="容器、编排、镜像、网络、存储卷和仓库的当前数量。">
          <DockerWorkbenchChart option={resourceChartOption} />
        </DockerSurfaceCard>
      </div>

      {latestOperation ? (
        <DockerSurfaceCard title="最近长任务" description="普通容器启动、停止、重启、删除已同步执行，不作为主路径任务。">
          <Space orientation="vertical" className="w-full">
            <Space wrap>
              <Tag>{formatOperationTypeLabel(latestOperation.operationType)}</Tag>
              <Tag color={latestOperation.status === 'SUCCEEDED' ? 'success' : latestOperation.status === 'FAILED' ? 'error' : 'processing'}>
                {formatOperationStatusLabel(latestOperation.status)}
              </Tag>
              <span className="text-sm text-slate-500">{latestOperation.targetName || latestOperation.targetId || '-'}</span>
            </Space>
            <Progress percent={latestOperation.progress || 0} size="small" />
          </Space>
        </DockerSurfaceCard>
      ) : null}
    </div>
  );
}

export default DockerOverviewTab;
