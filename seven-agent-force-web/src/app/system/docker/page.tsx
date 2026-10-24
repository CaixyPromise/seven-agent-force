'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Tabs } from 'antd';
import {
  ApartmentOutlined,
  AppstoreOutlined,
  CodeOutlined,
  ContainerOutlined,
  DatabaseOutlined,
  DeploymentUnitOutlined,
  GlobalOutlined,
  RadarChartOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerOverviewTab } from './workbench/components/DockerOverviewTab';
import { DockerContainersTab } from './workbench/components/DockerContainersTab';
import { ComposeYamlWorkbench } from './workbench/components/ComposeYamlWorkbench';
import { DockerImagesTab } from './workbench/components/DockerImagesTab';
import { DockerNetworksTab } from './workbench/components/DockerNetworksTab';
import { DockerVolumesTab } from './workbench/components/DockerVolumesTab';
import { DockerRegistriesTab } from './workbench/components/DockerRegistriesTab';
import { DockerConfigTab } from './workbench/components/DockerConfigTab';
import { DockerOperationsTab } from './workbench/components/DockerOperationsTab';
import {
  DOCKER_WORKBENCH_TABS,
  isDockerWorkbenchTabKey,
  type DockerWorkbenchTabKey,
} from './workbench/types';

const DEFAULT_TAB: DockerWorkbenchTabKey = 'overview';
const EMPTY_REFRESH_TOKENS = Object.fromEntries(
  DOCKER_WORKBENCH_TABS.map((tab) => [tab, 0]),
) as Record<DockerWorkbenchTabKey, number>;

function readSearchState() {
  if (typeof window === 'undefined') {
    return { tab: DEFAULT_TAB, project: '' };
  }
  const params = new URLSearchParams(window.location.search);
  const rawTab = params.get('tab');
  return {
    tab: isDockerWorkbenchTabKey(rawTab) ? rawTab : DEFAULT_TAB,
    project: params.get('project')?.trim() || '',
  };
}

function tabLabel(icon: ReactNode, label: string) {
  return (
    <span className="inline-flex items-center gap-2">
      {icon}
      {label}
    </span>
  );
}

export default function DockerWorkbenchPage() {
  const initialState = useMemo(() => readSearchState(), []);
  const [activeTab, setActiveTab] = useState<DockerWorkbenchTabKey>(initialState.tab);
  const [requestedProject, setRequestedProject] = useState(initialState.project);
  const [requestedOperation, setRequestedOperation] = useState<{
    operationId: API.Int64;
    token: number;
  }>();
  const [refreshTokens, setRefreshTokens] = useState<Record<DockerWorkbenchTabKey, number>>(EMPTY_REFRESH_TOKENS);
  const permissions = usePermissionFlags({
    canContainers: DOCKER_PERMISSIONS.CONTAINER_LIST,
    canContainerQuery: DOCKER_PERMISSIONS.CONTAINER_QUERY,
    canCompose: DOCKER_PERMISSIONS.COMPOSE_PROJECT_LIST,
    canImages: DOCKER_PERMISSIONS.IMAGE_LIST,
    canNetworks: DOCKER_PERMISSIONS.NETWORK_LIST,
    canVolumes: DOCKER_PERMISSIONS.VOLUME_LIST,
    canRegistries: DOCKER_PERMISSIONS.REGISTRY_LIST,
    canConfig: DOCKER_PERMISSIONS.CONFIG_QUERY,
    canOperations: DOCKER_PERMISSIONS.OPERATION_LIST,
  });
  const canContainers = permissions.canContainers;
  const canContainerQuery = permissions.canContainerQuery;
  const canCompose = permissions.canCompose;
  const canImages = permissions.canImages;
  const canNetworks = permissions.canNetworks;
  const canVolumes = permissions.canVolumes;
  const canRegistries = permissions.canRegistries;
  const canConfig = permissions.canConfig;
  const canOperations = permissions.canOperations;

  const syncSearchParams = useCallback((tab: DockerWorkbenchTabKey, project?: string) => {
    if (typeof window === 'undefined') {
      return;
    }
    const params = new URLSearchParams(window.location.search);
    params.set('tab', tab);
    if (project) {
      params.set('project', project);
    } else {
      params.delete('project');
    }
    window.history.replaceState(window.history.state, '', `/system/docker?${params.toString()}`);
  }, []);

  const tabPermissions = useMemo<Record<DockerWorkbenchTabKey, boolean>>(
    () => ({
      overview: true,
      containers: canContainers || canContainerQuery,
      compose: canCompose,
      images: canImages,
      networks: canNetworks,
      volumes: canVolumes,
      registries: canRegistries,
      config: canConfig,
      operations: canOperations,
    }),
    [canCompose, canConfig, canContainerQuery, canContainers, canImages, canNetworks, canOperations, canRegistries, canVolumes],
  );

  const visibleTabs = useMemo(() => DOCKER_WORKBENCH_TABS.filter((tab) => tabPermissions[tab]), [tabPermissions]);
  const fallbackTab = visibleTabs[0] || DEFAULT_TAB;
  const effectiveActiveTab = visibleTabs.includes(activeTab) ? activeTab : fallbackTab;

  useEffect(() => {
    if (effectiveActiveTab !== activeTab) {
      syncSearchParams(
        effectiveActiveTab,
        effectiveActiveTab === 'compose' ? requestedProject : '',
      );
    }
  }, [activeTab, effectiveActiveTab, requestedProject, syncSearchParams]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }
    const syncFromLocation = () => {
      const next = readSearchState();
      setActiveTab(next.tab);
      setRequestedProject(next.project);
    };
    window.addEventListener('popstate', syncFromLocation);
    return () => window.removeEventListener('popstate', syncFromLocation);
  }, []);

  const switchTab = useCallback(
    (tab: DockerWorkbenchTabKey, project?: string) => {
      setActiveTab(tab);
      setRequestedProject(project || '');
      setRefreshTokens((prev) => ({ ...prev, [tab]: prev[tab] + 1 }));
      syncSearchParams(tab, tab === 'compose' ? project : '');
    },
    [syncSearchParams],
  );

  const items = [
    {
      key: 'overview',
      label: tabLabel(<RadarChartOutlined />, '概览'),
      children: (
        <DockerOverviewTab
          refreshToken={refreshTokens.overview}
          onOpenOperation={(operationId) => {
            setRequestedOperation((prev) => ({
              operationId,
              token: (prev?.token || 0) + 1,
            }));
            switchTab('operations');
          }}
        />
      ),
    },
    {
      key: 'containers',
      label: tabLabel(<ContainerOutlined />, '容器'),
      children: (
        <DockerContainersTab
          refreshToken={refreshTokens.containers}
          onOpenComposeProject={(projectName) => {
            switchTab('compose', projectName);
          }}
        />
      ),
    },
    {
      key: 'compose',
      label: tabLabel(<CodeOutlined />, '编排'),
      children: <ComposeYamlWorkbench refreshToken={refreshTokens.compose} requestedProject={requestedProject} />,
    },
    {
      key: 'images',
      label: tabLabel(<AppstoreOutlined />, '镜像'),
      children: <DockerImagesTab refreshToken={refreshTokens.images} />,
    },
    {
      key: 'networks',
      label: tabLabel(<ApartmentOutlined />, '网络'),
      children: <DockerNetworksTab refreshToken={refreshTokens.networks} />,
    },
    {
      key: 'volumes',
      label: tabLabel(<DatabaseOutlined />, '存储卷'),
      children: <DockerVolumesTab refreshToken={refreshTokens.volumes} />,
    },
    {
      key: 'registries',
      label: tabLabel(<GlobalOutlined />, '仓库'),
      children: <DockerRegistriesTab refreshToken={refreshTokens.registries} />,
    },
    {
      key: 'config',
      label: tabLabel(<SettingOutlined />, '配置'),
      children: <DockerConfigTab refreshToken={refreshTokens.config} onOpenRegistries={() => switchTab('registries')} />,
    },
    {
      key: 'operations',
      label: tabLabel(<DeploymentUnitOutlined />, '任务'),
      children: (
        <DockerOperationsTab
          refreshToken={refreshTokens.operations}
          requestedOperationId={requestedOperation?.operationId}
          requestedOperationToken={requestedOperation?.token}
        />
      ),
    },
  ].filter((item) => tabPermissions[item.key as DockerWorkbenchTabKey]);

  return (
    <div className="min-h-[calc(100vh-96px)] bg-[#f5f7fb] px-1 pb-6">
      <Tabs
        className="[&_.ant-tabs-nav]:rounded-2xl [&_.ant-tabs-nav]:border [&_.ant-tabs-nav]:border-[#e8edf5] [&_.ant-tabs-nav]:bg-white [&_.ant-tabs-nav]:px-4 [&_.ant-tabs-nav]:shadow-[0_8px_24px_rgba(15,23,42,0.04)]"
        activeKey={effectiveActiveTab}
        items={items}
        onChange={(key) => switchTab(key as DockerWorkbenchTabKey)}
      />
    </div>
  );
}
