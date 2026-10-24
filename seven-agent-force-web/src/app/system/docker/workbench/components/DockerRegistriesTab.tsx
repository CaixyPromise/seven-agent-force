'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Button, Drawer, Modal, Space, Table, Tag, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  createDockerRegistry,
  deleteDockerRegistry,
  getDockerRegistries,
  syncDockerRegistry,
  testDockerRegistry,
  updateDockerRegistry,
  type DockerRegistryConnectionTestView,
  type DockerRemoteRegistryCommand,
  type DockerRemoteRegistryView,
} from '@/api/dockerController';
import { HasPermission } from '@/components/Permission/HasPermission';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerEmptyState, DockerSurfaceCard } from '../../components/dockerConsole';
import { RegistryConfigDrawer } from '../../images/components/RegistryConfigDrawer';
import { RemoteImageBrowser } from '../../images/components/RemoteImageBrowser';

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '-';
}

function authLabel(authType?: string) {
  const labels: Record<string, string> = {
    ANONYMOUS: '匿名',
    BASIC: 'Basic',
    TOKEN_CHALLENGE: 'Bearer Challenge',
  };
  return labels[authType || ''] || authType || '-';
}

function isRegistryEnabled(registry?: DockerRemoteRegistryView | null) {
  return !!registry && registry.status !== 1;
}

interface DockerRegistriesTabProps {
  refreshToken?: number;
}

export function DockerRegistriesTab({ refreshToken = 0 }: DockerRegistriesTabProps) {
  const [registries, setRegistries] = useState<DockerRemoteRegistryView[]>([]);
  const [loading, setLoading] = useState(false);
  const [browserRegistryId, setBrowserRegistryId] = useState<API.Int64>();
  const [browserOpen, setBrowserOpen] = useState(false);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [drawerSubmitting, setDrawerSubmitting] = useState(false);
  const [editingRegistry, setEditingRegistry] = useState<DockerRemoteRegistryView | null>(null);
  const permissions = usePermissionFlags({
    canListRegistry: DOCKER_PERMISSIONS.REGISTRY_LIST,
    canCreateRegistry: DOCKER_PERMISSIONS.REGISTRY_CREATE,
    canUpdateRegistry: DOCKER_PERMISSIONS.REGISTRY_UPDATE,
    canTestRegistry: DOCKER_PERMISSIONS.REGISTRY_TEST,
    canDeleteRegistry: DOCKER_PERMISSIONS.REGISTRY_DELETE,
    canSyncRegistry: DOCKER_PERMISSIONS.REGISTRY_SYNC,
  });

  const browserRegistry = useMemo(
    () => registries.find((registry) => registry.id === browserRegistryId && isRegistryEnabled(registry)),
    [browserRegistryId, registries],
  );

  const loadRegistries = useCallback(async () => {
    if (!permissions.canListRegistry) {
      setRegistries([]);
      setBrowserRegistryId(undefined);
      setBrowserOpen(false);
      return;
    }
    setLoading(true);
    try {
      const response = await getDockerRegistries();
      const rows = response.data || [];
      const enabledRows = rows.filter(isRegistryEnabled);
      setRegistries(rows);
      setBrowserRegistryId((prev) => {
        if (!prev || enabledRows.some((registry) => registry.id === prev)) {
          return prev;
        }
        setBrowserOpen(false);
        return undefined;
      });
    } catch (error) {
      message.error((error as Error).message || '获取镜像源配置失败');
    } finally {
      setLoading(false);
    }
  }, [permissions.canListRegistry]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadRegistries();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadRegistries, refreshToken]);

  const openCreateDrawer = useCallback(() => {
    setEditingRegistry(null);
    setDrawerOpen(true);
  }, []);

  const openEditDrawer = useCallback((registry: DockerRemoteRegistryView) => {
    setEditingRegistry(registry);
    setDrawerOpen(true);
  }, []);

  const handleSaveRegistry = async (values: DockerRemoteRegistryCommand) => {
    setDrawerSubmitting(true);
    try {
      if (editingRegistry) {
        await updateDockerRegistry(editingRegistry.id, values);
        message.success('镜像源配置已更新');
      } else {
        await createDockerRegistry(values);
        message.success('镜像源配置已创建');
      }
      setDrawerOpen(false);
      setEditingRegistry(null);
      await loadRegistries();
    } finally {
      setDrawerSubmitting(false);
    }
  };

  const handleTestRegistry = useCallback(async (registry: DockerRemoteRegistryView): Promise<DockerRegistryConnectionTestView | null> => {
    try {
      const response = await testDockerRegistry(registry.id);
      message[response.data.success ? 'success' : 'warning'](response.data.message);
      return response.data;
    } catch (error) {
      message.error((error as Error).message || '测试连接失败');
      return null;
    }
  }, []);

  const handleSyncRegistry = useCallback(async (registry: DockerRemoteRegistryView) => {
    try {
      const response = await syncDockerRegistry(registry.id);
      message.success(`仓库同步任务已提交 #${response.data.operationId}`);
    } catch (error) {
      message.error((error as Error).message || '同步仓库失败');
    }
  }, []);

  const openBrowserDrawer = useCallback((registry: DockerRemoteRegistryView) => {
    if (!isRegistryEnabled(registry)) {
      message.warning('当前镜像源已停用，启用后才能浏览远程仓库。');
      return;
    }
    setBrowserRegistryId(registry.id);
    setBrowserOpen(true);
  }, []);

  const confirmDeleteRegistry = useCallback(
    (registry: DockerRemoteRegistryView) => {
      Modal.confirm({
        title: '删除本地仓库配置',
        content: `确认删除「${registry.name}」的本地 registry 配置？此操作只删除本系统保存的连接配置，不会删除远端仓库、镜像标签、manifest、blob 或任何远端镜像数据。`,
        okText: '删除本地配置',
        okButtonProps: { danger: true },
        cancelText: '取消',
        onOk: async () => {
          await deleteDockerRegistry(registry.id);
          message.success('本地仓库配置已删除');
          if (browserRegistryId === registry.id) {
            setBrowserRegistryId(undefined);
            setBrowserOpen(false);
          }
          await loadRegistries();
        },
      });
    },
    [browserRegistryId, loadRegistries],
  );

  const columns = useMemo<ColumnsType<DockerRemoteRegistryView>>(() => [
    {
      title: '名称',
      dataIndex: 'name',
      ellipsis: true,
      render: (value: string) => <span className="font-medium text-slate-900">{value}</span>,
    },
    { title: '编码', dataIndex: 'code', width: 160, ellipsis: true },
    { title: '服务地址', dataIndex: 'endpoint', ellipsis: true },
    {
      title: '认证',
      dataIndex: 'authType',
      width: 150,
      render: (value: string, record) => (
        <Space size={4} wrap>
          <Tag color={value === 'ANONYMOUS' ? 'default' : 'blue'}>{authLabel(value)}</Tag>
          {record.secretConfigured ? <Tag color="green">已配置密钥</Tag> : null}
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (value: number | undefined, record) => (
        <Space size={4} wrap>
          <Tag color={value === 1 ? 'default' : 'success'}>{value === 1 ? '停用' : '启用'}</Tag>
          {record.defaultRegistry ? <Tag color="processing">默认</Tag> : null}
        </Space>
      ),
    },
    { title: '更新时间', dataIndex: 'updateTime', width: 180, render: formatTime },
    {
      title: '操作',
      key: 'option',
      width: 230,
      fixed: 'right',
      render: (_, record) => (
        <Space size={4} wrap>
          <Button
            type="link"
            size="small"
            onClick={() => openBrowserDrawer(record)}
          >
            浏览
          </Button>
          {permissions.canTestRegistry ? (
            <Button
              type="link"
              size="small"
              icon={<SafetyOutlined />}
              onClick={() => void handleTestRegistry(record)}
            >
              测试
            </Button>
          ) : null}
          {permissions.canSyncRegistry && isRegistryEnabled(record) ? (
            <Button
              type="link"
              size="small"
              icon={<SyncOutlined />}
              onClick={() => void handleSyncRegistry(record)}
            >
              同步
            </Button>
          ) : null}
          {permissions.canUpdateRegistry ? (
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => openEditDrawer(record)}
            >
              编辑
            </Button>
          ) : null}
          {permissions.canDeleteRegistry ? (
            <Button
              type="link"
              size="small"
              danger
              icon={<DeleteOutlined />}
              onClick={() => confirmDeleteRegistry(record)}
            >
              删除
            </Button>
          ) : null}
        </Space>
      ),
    },
  ], [
    confirmDeleteRegistry,
    handleSyncRegistry,
    handleTestRegistry,
    openBrowserDrawer,
    openEditDrawer,
    permissions.canDeleteRegistry,
    permissions.canSyncRegistry,
    permissions.canTestRegistry,
    permissions.canUpdateRegistry,
  ]);

  if (!permissions.canListRegistry) {
    return (
      <DockerEmptyState
        title="无镜像源权限"
        description="当前账号没有查看镜像源的权限。"
      />
    );
  }

  return (
    <div className="space-y-5">
      <DockerSurfaceCard
        title="镜像源"
        description="管理远程 registry 配置，并从这里浏览仓库、测试连接或拉取镜像到本地。"
        compact
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadRegistries()}>
              刷新
            </Button>
            <HasPermission code={DOCKER_PERMISSIONS.REGISTRY_CREATE}>
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreateDrawer}>
                新增镜像源
              </Button>
            </HasPermission>
          </Space>
        }
      >
        <Table<DockerRemoteRegistryView>
          rowKey="id"
          size="middle"
          loading={loading}
          dataSource={registries}
          columns={columns}
          pagination={false}
          scroll={{ x: 1120 }}
          rowClassName={(record) => (browserOpen && record.id === browserRegistry?.id ? 'bg-sky-50/70' : '')}
          locale={{
            emptyText: (
              <DockerEmptyState
                title="暂无镜像源"
                description="新增一个 registry 后，就可以浏览远程仓库并拉取镜像。"
                action={
                  permissions.canCreateRegistry ? (
                    <Button type="primary" icon={<PlusOutlined />} onClick={openCreateDrawer}>
                      新增镜像源
                    </Button>
                  ) : null
                }
              />
            ),
          }}
        />
      </DockerSurfaceCard>

      <Drawer
        open={browserOpen}
        title={browserRegistry ? `浏览远程仓库 · ${browserRegistry.name}` : '浏览远程仓库'}
        width={1120}
        destroyOnHidden
        onClose={() => setBrowserOpen(false)}
      >
        {browserRegistry ? (
        <RemoteImageBrowser
          registry={browserRegistry}
          onRefreshRegistries={loadRegistries}
          onEditRegistry={openEditDrawer}
          onTestRegistry={handleTestRegistry}
        />
        ) : (
          <DockerEmptyState title="没有可浏览的镜像源" description="请选择一个启用状态的镜像源后再浏览。" />
        )}
      </Drawer>

      <RegistryConfigDrawer
        open={drawerOpen}
        loading={drawerSubmitting}
        registry={editingRegistry}
        onClose={() => {
          setDrawerOpen(false);
          setEditingRegistry(null);
        }}
        onSubmit={handleSaveRegistry}
      />
    </div>
  );
}

export default DockerRegistriesTab;
