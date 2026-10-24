'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Checkbox,
  Dropdown,
  Empty,
  Input,
  Modal,
  Pagination,
  Popover,
  Skeleton,
  Space,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type { MenuProps } from 'antd';
import {
  DeleteOutlined,
  ExclamationCircleFilled,
  EyeOutlined,
  MoreOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
  StopOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  applyDockerContainerCleanup,
  deleteDockerContainer,
  getDockerContainer,
  getDockerContainers,
  previewDockerContainerCleanup,
  restartDockerContainer,
  startDockerContainer,
  stopDockerContainer,
  type DockerCleanupPreviewVO,
  type DockerContainerAction,
  type DockerContainerDetailView,
  type DockerContainerView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerStateTag, formatBytes } from '../../components/dockerConsole';
import {
  formatAbsoluteTime,
  formatContainerStateLabel,
  formatOperationTypeLabel,
  formatPort,
  formatRelativeTime,
  normalizeState,
  shortId,
} from '../../components/dockerFormat';
import { ContainerDetailDrawer } from '../../containers/components/ContainerDetailDrawer';

type BasicContainerAction = Extract<DockerContainerAction, 'start' | 'stop' | 'restart' | 'delete'>;

interface DockerContainersTabProps {
  refreshToken?: number;
  onOpenComposeProject?: (projectName: string) => void;
}

interface ContainerFilters {
  statuses: string[];
  keyword: string;
}

const DEFAULT_CONTAINER_FILTERS: ContainerFilters = {
  statuses: [],
  keyword: '',
};

function defaultContainerActions(state?: string): DockerContainerAction[] {
  const normalized = normalizeState(state);
  if (normalized === 'running' || normalized === 'restarting' || normalized === 'paused') {
    return ['stop', 'restart', 'logs', 'stats', 'inspect'];
  }
  if (normalized === 'exited' || normalized === 'created' || normalized === 'dead') {
    return ['start', 'logs', 'inspect', 'delete'];
  }
  return ['logs', 'inspect'];
}

function hasContainerAction(record: DockerContainerView, action: DockerContainerAction) {
  return (record.availableActions?.length ? record.availableActions : defaultContainerActions(record.state)).includes(action);
}

function matchesStatus(record: DockerContainerView, statuses: string[]) {
  if (!statuses.length) {
    return true;
  }
  return statuses.includes(normalizeState(record.state));
}

function matchesKeyword(record: DockerContainerView, keyword: string) {
  const normalizedKeyword = keyword.trim().toLowerCase();
  if (!normalizedKeyword) {
    return true;
  }
  return [record.id, record.name, record.image, record.imageId, record.composeProject, record.composeService]
    .filter(Boolean)
    .some((value) => String(value).toLowerCase().includes(normalizedKeyword));
}

export function DockerContainersTab({ refreshToken = 0, onOpenComposeProject }: DockerContainersTabProps) {
  const [containers, setContainers] = useState<DockerContainerView[]>([]);
  const [listLoading, setListLoading] = useState(false);
  const [filters, setFilters] = useState<ContainerFilters>(DEFAULT_CONTAINER_FILTERS);
  const [keywordInput, setKeywordInput] = useState(DEFAULT_CONTAINER_FILTERS.keyword);
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10 });
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [selectedContainerId, setSelectedContainerId] = useState<string>();
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detail, setDetail] = useState<DockerContainerDetailView | null>(null);
  const [detailInitialTab, setDetailInitialTab] = useState<'overview' | 'inspect' | 'stats' | 'logs'>('overview');
  const [rowOperationLoading, setRowOperationLoading] = useState<Record<string, string>>({});
  const [cleanupPreview, setCleanupPreview] = useState<DockerCleanupPreviewVO | null>(null);
  const [cleanupPreviewOpen, setCleanupPreviewOpen] = useState(false);
  const [cleanupPreviewLoading, setCleanupPreviewLoading] = useState(false);
  const [cleanupApplying, setCleanupApplying] = useState(false);
  const [cleanupConfirmText, setCleanupConfirmText] = useState('');

  const permissions = usePermissionFlags({
    canQuery: DOCKER_PERMISSIONS.CONTAINER_QUERY,
    canStart: DOCKER_PERMISSIONS.CONTAINER_START,
    canStop: DOCKER_PERMISSIONS.CONTAINER_STOP,
    canRestart: DOCKER_PERMISSIONS.CONTAINER_RESTART,
    canDelete: DOCKER_PERMISSIONS.CONTAINER_DELETE,
  });

  const loadContainers = useCallback(async (options?: { silent?: boolean }) => {
    if (!options?.silent) {
      setListLoading(true);
    }
    try {
      const response = await getDockerContainers({
        current: 1,
        size: 200,
        keyword: filters.keyword || undefined,
      });
      setContainers(response.data.records || []);
    } catch (error) {
      const messageText = (error as Error).message || '容器列表加载失败';
      message.error(messageText);
      setContainers([]);
    } finally {
      if (!options?.silent) {
        setListLoading(false);
      }
    }
  }, [filters.keyword]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadContainers();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadContainers, refreshToken]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setFilters((prev) => ({ ...prev, keyword: keywordInput.trim() }));
      setPagination((prev) => ({ ...prev, current: 1 }));
    }, 300);
    return () => window.clearTimeout(timer);
  }, [keywordInput]);

  const containerStatusCounts = useMemo(() => {
    const counts: Record<string, number> = {};
    containers.forEach((container) => {
      const state = normalizeState(container.state) || 'unknown';
      counts[state] = (counts[state] || 0) + 1;
    });
    return counts;
  }, [containers]);

  const filteredContainers = useMemo(
    () =>
      containers.filter(
        (record) =>
          matchesStatus(record, filters.statuses) &&
          matchesKeyword(record, filters.keyword),
      ),
    [containers, filters],
  );

  const pagedContainers = useMemo(() => {
    const start = (pagination.current - 1) * pagination.pageSize;
    return filteredContainers.slice(start, start + pagination.pageSize);
  }, [filteredContainers, pagination]);

  const openDetail = useCallback(async (record: DockerContainerView, initialTab: 'overview' | 'inspect' | 'stats' | 'logs' = 'overview') => {
    setSelectedContainerId(record.id);
    setDetailInitialTab(initialTab);
    setDetailOpen(true);
    setDetailLoading(true);
    try {
      const response = await getDockerContainer(record.id);
      setDetail(response.data);
    } catch (error) {
      message.error((error as Error).message || '获取容器详情失败');
      setDetail(null);
    } finally {
      setDetailLoading(false);
    }
  }, []);

  const refreshDetail = useCallback(async () => {
    if (!selectedContainerId) {
      return;
    }
    setDetailLoading(true);
    try {
      const response = await getDockerContainer(selectedContainerId);
      setDetail(response.data);
    } catch (error) {
      message.error((error as Error).message || '刷新容器详情失败');
    } finally {
      setDetailLoading(false);
    }
  }, [selectedContainerId]);

  const openComposeProject = useCallback((projectName?: string) => {
    if (!projectName) {
      message.warning('当前容器没有 Compose 项目信息');
      return;
    }
    onOpenComposeProject?.(projectName);
  }, [onOpenComposeProject]);

  const runContainerAction = useCallback(
    async (
      record: DockerContainerView,
      actionKey: BasicContainerAction,
      runner: () => Promise<{ data?: boolean }>,
      successText: string,
      failureText: string,
    ) => {
      setRowOperationLoading((prev) => ({ ...prev, [record.id]: actionKey }));
      try {
        const response = await runner();
        if (response.data === false) {
          throw new Error(failureText);
        }
        message.success(successText);
        await loadContainers({ silent: true });
        if (detailOpen && selectedContainerId === record.id) {
          if (actionKey === 'delete') {
            setDetailOpen(false);
            setDetail(null);
            setSelectedContainerId(undefined);
            return;
          }
          await refreshDetail();
        }
    } catch (error) {
      const messageText = (error as Error).message || failureText;
      message.error(messageText);
    } finally {
        setRowOperationLoading((prev) => {
          const next = { ...prev };
          delete next[record.id];
          return next;
        });
      }
    },
    [detailOpen, loadContainers, refreshDetail, selectedContainerId],
  );

  const handleDeleteContainer = useCallback(
    (record: DockerContainerView) => {
      const state = normalizeState(record.state);
      if (state === 'running' || state === 'restarting') {
        message.warning('容器仍在运行，请先停止容器后再删除。');
        return;
      }
      Modal.confirm({
        title: `删除容器 ${record.name}`,
        content: '此操作会删除容器且不可恢复，请确认当前容器已经不再使用。',
        okText: '删除',
        cancelText: '取消',
        okButtonProps: { danger: true },
        onOk: () =>
          runContainerAction(record, 'delete', () => deleteDockerContainer(record.id), '删除成功', '删除失败'),
      });
    },
    [runContainerAction],
  );

  const handlePreviewCleanup = async () => {
    if (!permissions.canDelete) {
      message.warning('当前账号没有容器清理权限');
      return;
    }
    setCleanupPreviewLoading(true);
    try {
      const response = await previewDockerContainerCleanup({});
      setCleanupPreview(response.data);
      setCleanupConfirmText('');
      setCleanupPreviewOpen(true);
    } catch (error) {
      const messageText = (error as Error).message || '预览容器清理影响失败';
      message.error(messageText);
    } finally {
      setCleanupPreviewLoading(false);
    }
  };

  const handleApplyCleanup = async () => {
    if (!cleanupPreview?.previewToken) {
      message.warning('请先预览影响后再执行清理。');
      return;
    }
    if (cleanupConfirmText !== 'DELETE') {
      message.warning('请输入 DELETE 确认执行清理。');
      return;
    }
    setCleanupApplying(true);
    try {
      const response = await applyDockerContainerCleanup({ previewToken: cleanupPreview.previewToken });
      const operationId = response.data?.operationId;
      message.success(operationId ? `清理操作已提交 #${operationId}` : '清理操作已提交');
      setCleanupPreviewOpen(false);
      setCleanupPreview(null);
      setCleanupConfirmText('');
      await loadContainers();
    } catch (error) {
      const messageText = (error as Error).message || '执行容器清理失败';
      message.error(messageText);
    } finally {
      setCleanupApplying(false);
    }
  };

  const resetFilters = () => {
    setFilters(DEFAULT_CONTAINER_FILTERS);
    setKeywordInput(DEFAULT_CONTAINER_FILTERS.keyword);
    setPagination({ current: 1, pageSize: 10 });
  };

  const cleanupRows = useMemo(() => {
    const affected = cleanupPreview?.affectedResources || [];
    return affected.map((id) => {
      const found = containers.find((container) => container.id === id || shortId(container.id) === id);
      return {
        id,
        name: found?.name || id,
        image: found?.image || '-',
        state: found?.state || '-',
        created: found?.created,
      };
    });
  }, [cleanupPreview, containers]);

  const columns = useMemo<ColumnsType<DockerContainerView>>(
    () => [
      {
        title: '容器名',
        dataIndex: 'name',
        width: 220,
        render: (_, record) => {
          const activeOperation = record.activeOperation;
          return (
            <button
              type="button"
              className="block max-w-[190px] text-left"
              onClick={(event) => {
                event.stopPropagation();
                void openDetail(record);
              }}
            >
              <Tooltip title={record.name}>
                <div className="truncate text-sm font-medium text-slate-900">{record.name || '-'}</div>
              </Tooltip>
              <div className="mt-1 text-xs text-slate-500">{shortId(record.id)}</div>
              {activeOperation ? (
                <Tag className="mt-1" color="processing">
                  {formatOperationTypeLabel(activeOperation.operationType)} #{activeOperation.operationId}
                </Tag>
              ) : null}
            </button>
          );
        },
      },
      {
        title: '镜像',
        dataIndex: 'image',
        width: 190,
        render: (_, record) => (
          <Tooltip title={record.image}>
            <div className="max-w-[170px] truncate text-sm text-slate-700">{record.image || '-'}</div>
          </Tooltip>
        ),
      },
      {
        title: '状态',
        dataIndex: 'state',
        width: 110,
        render: (_, record) => (
          <DockerStateTag state={record.state} label={formatContainerStateLabel(record.state)} />
        ),
      },
      {
        title: '端口',
        dataIndex: 'ports',
        width: 150,
        render: (_, record) => {
          const ports = record.ports || [];
          if (!ports.length) {
            return <span className="text-slate-400">-</span>;
          }
          const visiblePorts = ports.slice(0, 2);
          return (
            <Space size={4} wrap>
              {visiblePorts.map((port, index) => (
                <Tag key={`${record.id}-${index}`} className="m-0 rounded-md border-0 bg-blue-50 text-blue-600">
                  {formatPort(port)}
                </Tag>
              ))}
              {ports.length > 2 ? (
                <Popover
                  content={
                    <div className="space-y-1">
                      {ports.map((port, index) => (
                        <div key={`${record.id}-all-${index}`}>{formatPort(port)}</div>
                      ))}
                    </div>
                  }
                >
                  <Tag className="m-0 cursor-pointer">+{ports.length - 2}</Tag>
                </Popover>
              ) : null}
            </Space>
          );
        },
      },
      {
        title: '运行时长',
        dataIndex: 'created',
        width: 150,
        render: (_, record) => (
          <div>
            <div className="text-sm text-slate-700">{formatRelativeTime(record.created)}</div>
            <div className="mt-1 text-xs text-slate-400">{formatAbsoluteTime(record.created)}</div>
          </div>
        ),
      },
      {
        title: '编排',
        dataIndex: 'composeManaged',
        width: 150,
        render: (_, record) =>
          record.composeManaged ? (
            <button
              type="button"
              className="max-w-[130px] truncate text-left text-sm font-medium text-blue-600 hover:underline"
              onClick={(event) => {
                event.stopPropagation();
                openComposeProject(record.composeProject);
              }}
            >
              {record.composeProject || '编排项目'}
            </button>
          ) : (
            <Tag className="m-0">单容器</Tag>
          ),
      },
      {
        title: '操作',
        key: 'actions',
        width: 128,
        fixed: 'right',
        render: (_, record) => {
          const loadingAction = rowOperationLoading[record.id];
          const menuItems: MenuProps['items'] = [
            permissions.canQuery && hasContainerAction(record, 'inspect')
              ? {
                  key: 'detail',
                  label: '查看详情',
                  icon: <EyeOutlined />,
                }
              : null,
            permissions.canRestart && hasContainerAction(record, 'restart')
              ? {
                  key: 'restart',
                  label: '重启容器',
                  icon: <SyncOutlined />,
                }
              : null,
            permissions.canDelete && hasContainerAction(record, 'delete')
              ? {
                  key: 'delete',
                  label: '删除容器',
                  icon: <DeleteOutlined />,
                  danger: true,
                }
              : null,
          ].filter(Boolean) as MenuProps['items'];

          const onMenuClick: MenuProps['onClick'] = ({ key }) => {
            if (key === 'detail') {
              void openDetail(record);
              return;
            }
            if (key === 'restart') {
              void runContainerAction(record, 'restart', () => restartDockerContainer(record.id), '重启成功', '重启失败');
              return;
            }
            if (key === 'delete') {
              handleDeleteContainer(record);
            }
          };

          return (
            <Space size={4} onClick={(event) => event.stopPropagation()}>
              {permissions.canStart && hasContainerAction(record, 'start') ? (
                <Tooltip title="启动">
                  <Button
                    size="small"
                    type="text"
                    icon={<PlayCircleOutlined />}
                    loading={loadingAction === 'start'}
                    disabled={!!loadingAction && loadingAction !== 'start'}
                    onClick={() =>
                      void runContainerAction(record, 'start', () => startDockerContainer(record.id), '启动成功', '启动失败')
                    }
                  />
                </Tooltip>
              ) : null}
              {permissions.canStop && hasContainerAction(record, 'stop') ? (
                <Tooltip title="停止">
                  <Button
                    size="small"
                    type="text"
                    icon={<StopOutlined />}
                    loading={loadingAction === 'stop'}
                    disabled={!!loadingAction && loadingAction !== 'stop'}
                    onClick={() =>
                      void runContainerAction(record, 'stop', () => stopDockerContainer(record.id), '停止成功', '停止失败')
                    }
                  />
                </Tooltip>
              ) : null}
              <Dropdown menu={{ items: menuItems, onClick: onMenuClick }} trigger={['click']}>
                <Button
                  size="small"
                  type="text"
                  icon={<MoreOutlined />}
                  loading={loadingAction === 'restart' || loadingAction === 'delete'}
                  disabled={!!loadingAction && loadingAction !== 'restart' && loadingAction !== 'delete'}
                />
              </Dropdown>
            </Space>
          );
        },
      },
    ],
    [handleDeleteContainer, openComposeProject, openDetail, permissions, rowOperationLoading, runContainerAction],
  );

  const statusOptions = [
    { label: `运行中 (${containerStatusCounts.running || 0})`, value: 'running' },
    { label: `重启中 (${containerStatusCounts.restarting || 0})`, value: 'restarting' },
    { label: `已停止 (${containerStatusCounts.exited || 0})`, value: 'exited' },
    { label: `已暂停 (${containerStatusCounts.paused || 0})`, value: 'paused' },
    { label: `已创建 (${containerStatusCounts.created || 0})`, value: 'created' },
    { label: `异常 (${containerStatusCounts.dead || 0})`, value: 'dead' },
    { label: `删除中 (${containerStatusCounts.removing || 0})`, value: 'removing' },
    { label: `未知 (${containerStatusCounts.unknown || 0})`, value: 'unknown' },
  ];

  return (
    <div className="min-h-[calc(100vh-96px)] bg-[#f5f7fb] px-1 pb-6">
      <div className="space-y-4">
        <main className="min-w-0 space-y-4">
          <section className="rounded-2xl border border-[#e8edf5] bg-white px-4 py-4 shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <h1 className="m-0 text-2xl font-semibold tracking-normal text-slate-950">容器管理</h1>
              <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-end">
                <Input
                  allowClear
                  className="max-w-[420px]"
                  value={keywordInput}
                  prefix={<SearchOutlined className="text-slate-400" />}
                  placeholder="搜索容器名 / 镜像 / ID"
                  onChange={(event) => setKeywordInput(event.target.value)}
                  onPressEnter={(event) => {
                    const value = event.currentTarget.value.trim();
                    setFilters((prev) => ({ ...prev, keyword: value }));
                    setPagination((prev) => ({ ...prev, current: 1 }));
                  }}
                />
                <Button icon={<ReloadOutlined />} onClick={() => void loadContainers()}>
                  刷新
                </Button>
                <Button
                  icon={<DeleteOutlined />}
                  loading={cleanupPreviewLoading}
                  disabled={!permissions.canDelete}
                  onClick={() => void handlePreviewCleanup()}
                >
                  清理已停止容器
                </Button>
              </div>
            </div>
          </section>

          <section className="overflow-hidden rounded-2xl border border-[#e8edf5] bg-white shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
            <div className="grid min-h-[680px] lg:grid-cols-[190px_minmax(0,1fr)]">
              <aside className="border-b border-[#e8edf5] p-4 lg:border-b-0 lg:border-r">
                <div className="text-base font-semibold text-slate-900">筛选</div>

                <div className="mt-6 space-y-7">
                  <div>
                    <div className="mb-3 text-sm font-medium text-slate-700">状态</div>
                    <Checkbox.Group
                      className="flex flex-col gap-3"
                      value={filters.statuses}
                      onChange={(values) => {
                        setFilters((prev) => ({ ...prev, statuses: values.map(String) }));
                        setPagination((prev) => ({ ...prev, current: 1 }));
                      }}
                    >
                      {statusOptions.map((option) => (
                        <Checkbox key={option.value} value={option.value}>
                          {option.label}
                        </Checkbox>
                      ))}
                    </Checkbox.Group>
                  </div>
                </div>

                <Button className="mt-10 w-full" onClick={resetFilters}>
                  重置筛选
                </Button>
              </aside>

              <div className="min-w-0 p-4">
                <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div>
                    <div className="text-lg font-semibold text-slate-900">容器列表</div>
                    <div className="mt-1 text-sm text-slate-500">共 {filteredContainers.length} 个容器</div>
                  </div>
                </div>

                {listLoading && !containers.length ? (
                  <Skeleton active paragraph={{ rows: 10 }} />
                ) : filteredContainers.length ? (
                  <Table<DockerContainerView>
                    rowKey="id"
                    className="[&_.ant-table-row]:h-[66px]"
                    columns={columns}
                    dataSource={pagedContainers}
                    loading={listLoading}
                    pagination={false}
                    scroll={{ x: 1120 }}
                    rowSelection={{ selectedRowKeys, onChange: setSelectedRowKeys }}
                  />
                ) : (
                  <Empty
                    className="py-24"
                    description={
                      <div>
                        <div className="font-medium text-slate-900">暂无容器</div>
                        <div className="mt-1 text-sm text-slate-500">
                          当前主机未发现 Docker 容器，或筛选条件无匹配结果。
                        </div>
                      </div>
                    }
                  >
                    <Space>
                      <Button onClick={resetFilters}>重置筛选</Button>
                      <Button type="primary" onClick={() => void loadContainers()}>
                        刷新
                      </Button>
                    </Space>
                  </Empty>
                )}

                <div className="mt-4 flex justify-end">
                  <Pagination
                    current={pagination.current}
                    pageSize={pagination.pageSize}
                    total={filteredContainers.length}
                    showSizeChanger
                    showTotal={(count) => `共 ${count} 条`}
                    onChange={(current, pageSize) => setPagination({ current, pageSize })}
                  />
                </div>
              </div>
            </div>
          </section>
        </main>
      </div>

      <ContainerDetailDrawer
        open={detailOpen}
        loading={detailLoading}
        detail={detail}
        initialTab={detailInitialTab}
        onRefresh={() => void refreshDetail()}
        onClose={() => {
          setDetailOpen(false);
          setDetail(null);
          setSelectedContainerId(undefined);
          setDetailInitialTab('overview');
        }}
      />

      <Modal
        open={cleanupPreviewOpen}
        title={
          <Space>
            <ExclamationCircleFilled className="text-red-500" />
            停止容器清理确认
          </Space>
        }
        width={760}
        okText="确认清理"
        cancelText="取消"
        okButtonProps={{
          danger: true,
          disabled: cleanupConfirmText !== 'DELETE' || !cleanupPreview?.previewToken,
          loading: cleanupApplying,
        }}
        onCancel={() => {
          setCleanupPreviewOpen(false);
          setCleanupConfirmText('');
        }}
        onOk={() => void handleApplyCleanup()}
      >
        <div className="space-y-4">
          <div className="rounded-xl bg-red-50 px-4 py-3 text-sm text-slate-700">
            将清理 {cleanupRows.length} 个已停止容器，预计释放 {formatBytes(cleanupPreview?.estimatedBytes)}。
          </div>
          {cleanupPreview?.warning ? (
            <div className="rounded-xl bg-amber-50 px-4 py-3 text-sm text-amber-700">
              {cleanupPreview.warning}
            </div>
          ) : null}
          <Table
            size="small"
            rowKey="id"
            pagination={false}
            dataSource={cleanupRows}
            columns={[
              { title: '容器名', dataIndex: 'name' },
              { title: '镜像', dataIndex: 'image' },
              { title: '状态', dataIndex: 'state' },
              {
                title: '创建时间',
                dataIndex: 'created',
                render: (value) => formatAbsoluteTime(value),
              },
            ]}
          />
          <div>
            <div className="mb-2 text-sm text-slate-600">请输入 DELETE 确认执行清理</div>
            <Input value={cleanupConfirmText} onChange={(event) => setCleanupConfirmText(event.target.value)} />
          </div>
        </div>
      </Modal>
    </div>
  );
}

export default DockerContainersTab;
