'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  Modal,
  Select,
  Space,
  Switch,
  Table,
  Tag,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DeleteOutlined,
  LinkOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ScissorOutlined,
} from '@ant-design/icons';
import {
  applyDockerNetworkPrune,
  connectDockerNetwork,
  createDockerNetwork,
  deleteDockerNetwork,
  disconnectDockerNetwork,
  getDockerContainers,
  getDockerNetwork,
  getDockerNetworks,
  previewDockerNetworkPrune,
  type DockerContainerView,
  type DockerNetworkConnectRequest,
  type DockerNetworkCreateRequest,
  type DockerNetworkDisconnectRequest,
  type DockerResourcePrunePreview,
  type DockerResourceView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerEmptyState, DockerSurfaceCard, formatBytes } from '../../components/dockerConsole';
import { formatAbsoluteTime, shortId } from '../../components/dockerFormat';

interface KeyValueFormRow {
  key?: string;
  value?: string;
}

type NetworkFormValues = Omit<DockerNetworkCreateRequest, 'labels' | 'options'> & {
  labels?: KeyValueFormRow[];
  options?: KeyValueFormRow[];
};
type NetworkConnectionMode = 'connect' | 'disconnect';

interface NetworkConnectionValues {
  containerId: string;
  aliasesText?: string;
}

function keyValueMap(rows?: KeyValueFormRow[]) {
  return (rows || []).reduce<Record<string, string>>((acc, item) => {
    const key = item.key?.trim();
    if (key) {
      acc[key] = item.value?.trim() || '';
    }
    return acc;
  }, {});
}

function tagMap(map?: Record<string, string>) {
  const entries = Object.entries(map || {});
  if (!entries.length) {
    return <span className="text-slate-400">-</span>;
  }
  return (
    <Space size={4} wrap>
      {entries.map(([key, value]) => (
        <Tag key={key} className="m-0">
          {key}={value}
        </Tag>
      ))}
    </Space>
  );
}

function containersText(network?: DockerResourceView | null) {
  const entries = Object.entries(network?.containers || {});
  if (!entries.length) {
    return <span className="text-slate-400">暂无容器连接</span>;
  }
  return (
    <div className="space-y-2">
      {entries.map(([id, container]) => (
        <div key={id} className="rounded border border-slate-100 bg-slate-50 px-3 py-2 text-sm">
          <div className="font-medium text-slate-900">{container.name || shortId(id)}</div>
          <div className="mt-1 text-xs text-slate-500">
            {shortId(id)}
            {container.ipv4Address ? ` · ${container.ipv4Address}` : ''}
            {container.ipv6Address ? ` · ${container.ipv6Address}` : ''}
          </div>
        </div>
      ))}
    </div>
  );
}

interface DockerNetworksTabProps {
  refreshToken?: number;
}

export function DockerNetworksTab({ refreshToken = 0 }: DockerNetworksTabProps) {
  const [form] = Form.useForm<NetworkFormValues>();
  const [connectionForm] = Form.useForm<NetworkConnectionValues>();
  const [resources, setResources] = useState<DockerResourceView[]>([]);
  const [containers, setContainers] = useState<DockerContainerView[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [selectedNetwork, setSelectedNetwork] = useState<DockerResourceView | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [connectionOpen, setConnectionOpen] = useState(false);
  const [connectionMode, setConnectionMode] = useState<NetworkConnectionMode>('connect');
  const [pruneOpen, setPruneOpen] = useState(false);
  const [pruneLoading, setPruneLoading] = useState(false);
  const [prunePreview, setPrunePreview] = useState<DockerResourcePrunePreview | null>(null);
  const permissions = usePermissionFlags({
    canList: DOCKER_PERMISSIONS.NETWORK_LIST,
    canQuery: DOCKER_PERMISSIONS.NETWORK_QUERY,
    canCreate: DOCKER_PERMISSIONS.NETWORK_CREATE,
    canConnect: DOCKER_PERMISSIONS.NETWORK_CONNECT,
    canDisconnect: DOCKER_PERMISSIONS.NETWORK_DISCONNECT,
    canDelete: DOCKER_PERMISSIONS.NETWORK_DELETE,
    canPrune: DOCKER_PERMISSIONS.NETWORK_PRUNE,
  });

  const loadNetworks = useCallback(async () => {
    if (!permissions.canList) {
      setResources([]);
      return;
    }
    setLoading(true);
    try {
      const response = await getDockerNetworks({ current: 1, size: 500, keyword: keyword.trim() || undefined });
      setResources(response.data.records || []);
    } catch (error) {
      message.error((error as Error).message || '加载 Docker 网络失败');
      setResources([]);
    } finally {
      setLoading(false);
    }
  }, [keyword, permissions.canList]);

  const loadContainers = useCallback(async () => {
    try {
      const response = await getDockerContainers({ current: 1, size: 500 });
      setContainers(response.data.records || []);
    } catch {
      setContainers([]);
    }
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadNetworks();
    }, 200);
    return () => window.clearTimeout(timer);
  }, [loadNetworks, refreshToken]);

  const openDetail = useCallback(
    async (network: DockerResourceView) => {
      if (!permissions.canQuery) {
        return;
      }
      setSelectedNetwork(network);
      setDetailOpen(true);
      setDetailLoading(true);
      try {
        const response = await getDockerNetwork(network.id || network.name);
        setSelectedNetwork({
          ...response.data.resource,
          containers: response.data.containers || response.data.resource.containers,
          options: response.data.options || response.data.resource.options,
          inspect: response.data.inspect,
        });
      } catch (error) {
        message.error((error as Error).message || '加载网络详情失败');
      } finally {
        setDetailLoading(false);
      }
    },
    [permissions.canQuery],
  );

  const openConnectionDrawer = useCallback(
    (network: DockerResourceView, mode: NetworkConnectionMode) => {
      setSelectedNetwork(network);
      setConnectionMode(mode);
      connectionForm.resetFields();
      setConnectionOpen(true);
      void loadContainers();
    },
    [connectionForm, loadContainers],
  );

  const handleCreate = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      await createDockerNetwork({
        ...values,
        driver: values.driver || undefined,
        labels: keyValueMap(values.labels),
        options: keyValueMap(values.options),
      });
      message.success('网络已创建');
      setCreateOpen(false);
      form.resetFields();
      await loadNetworks();
    } finally {
      setSubmitting(false);
    }
  };

  const handleConnection = async () => {
    if (!selectedNetwork) {
      return;
    }
    const values = await connectionForm.validateFields();
    const connectData: DockerNetworkConnectRequest = {
      containerId: values.containerId,
      aliases: values.aliasesText
        ?.split(/[\n,]/)
        .map((item) => item.trim())
        .filter(Boolean),
    };
    setSubmitting(true);
    try {
      if (connectionMode === 'connect') {
        await connectDockerNetwork(selectedNetwork.id || selectedNetwork.name, connectData);
        message.success('容器已连接到网络');
      } else {
        const disconnectData: DockerNetworkDisconnectRequest = {
          containerId: connectData.containerId,
          force: false,
        };
        await disconnectDockerNetwork(selectedNetwork.id || selectedNetwork.name, disconnectData);
        message.success('容器已从网络断开');
      }
      setConnectionOpen(false);
      await loadNetworks();
    } finally {
      setSubmitting(false);
    }
  };

  const confirmDelete = useCallback(
    (network: DockerResourceView) => {
      Modal.confirm({
        title: '删除 Docker 网络',
        content: `确认删除网络「${network.name || network.id}」？已连接容器或系统网络可能会被 Docker 拒绝删除。`,
        okText: '删除',
        okButtonProps: { danger: true },
        cancelText: '取消',
        onOk: async () => {
          await deleteDockerNetwork(network.id || network.name);
          message.success('网络已删除');
          await loadNetworks();
        },
      });
    },
    [loadNetworks],
  );

  const loadPrunePreview = useCallback(async () => {
    setPruneLoading(true);
    try {
      const response = await previewDockerNetworkPrune();
      setPrunePreview(response.data);
    } catch (error) {
      message.error((error as Error).message || '生成网络清理预览失败');
      setPrunePreview(null);
    } finally {
      setPruneLoading(false);
    }
  }, []);

  const openPruneDrawer = useCallback(() => {
    setPruneOpen(true);
    void loadPrunePreview();
  }, [loadPrunePreview]);

  const applyPrune = async () => {
    setSubmitting(true);
    try {
      const response = await applyDockerNetworkPrune({ previewToken: prunePreview?.previewToken });
      message.success(`网络清理任务已提交 #${response.data.operationId}`);
      setPruneOpen(false);
      await loadNetworks();
    } finally {
      setSubmitting(false);
    }
  };

  const columns = useMemo<ColumnsType<DockerResourceView>>(
    () => [
      {
        title: '名称',
        dataIndex: 'name',
        width: 240,
        render: (_, record) => (
          <Button
            type="link"
            className="h-auto px-0 text-left"
            disabled={!permissions.canQuery}
            onClick={() => void openDetail(record)}
          >
            <div>
              <div className="font-semibold text-slate-900">{record.name || shortId(record.id)}</div>
              <div className="mt-1 text-xs text-slate-400">{shortId(record.id)}</div>
            </div>
          </Button>
        ),
      },
      { title: '驱动', dataIndex: 'driver', width: 120, render: (value) => value || '-' },
      { title: '作用域', dataIndex: 'scope', width: 110, render: (value) => value || '-' },
      {
        title: '属性',
        key: 'flags',
        width: 220,
        render: (_, record) => (
          <Space size={4} wrap>
            {record.internal ? <Tag color="warning">内部</Tag> : null}
            {record.attachable ? <Tag color="processing">可连接</Tag> : null}
            {record.ingress ? <Tag color="purple">Ingress</Tag> : null}
            {record.ipv6 ? <Tag color="cyan">IPv6</Tag> : null}
            {!record.internal && !record.attachable && !record.ingress && !record.ipv6 ? <span className="text-slate-400">-</span> : null}
          </Space>
        ),
      },
      {
        title: '容器',
        dataIndex: 'containers',
        width: 100,
        render: (_, record) => Object.keys(record.containers || {}).length || '-',
      },
      { title: '创建时间', dataIndex: 'createdAt', width: 180, render: (value) => formatAbsoluteTime(value as string) },
      {
        title: '操作',
        key: 'option',
        width: 260,
        fixed: 'right',
        render: (_, record) => (
          <Space size={4} wrap>
            {permissions.canConnect ? (
              <Button type="link" size="small" onClick={() => openConnectionDrawer(record, 'connect')}>
                连接
              </Button>
            ) : null}
            {permissions.canDisconnect ? (
              <Button type="link" size="small" onClick={() => openConnectionDrawer(record, 'disconnect')}>
                断开
              </Button>
            ) : null}
            {permissions.canDelete ? (
              <Button type="link" size="small" danger onClick={() => confirmDelete(record)}>
                删除
              </Button>
            ) : null}
          </Space>
        ),
      },
    ],
    [confirmDelete, openConnectionDrawer, openDetail, permissions],
  );

  if (!permissions.canList) {
    return <DockerEmptyState title="无网络权限" description="当前账号没有查看 Docker 网络的权限。" />;
  }

  return (
    <div className="space-y-5">
      <DockerSurfaceCard
        title="Docker 网络"
        description="查看网络、创建自定义网络，并将容器连接或断开到指定网络。"
        compact
        extra={
          <Space wrap>
            <Input
              allowClear
              className="w-[260px]"
              prefix={<SearchOutlined className="text-slate-400" />}
              placeholder="搜索名称 / ID"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
            />
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadNetworks()}>
              刷新
            </Button>
            {permissions.canPrune ? (
              <Button icon={<ScissorOutlined />} onClick={openPruneDrawer}>
                清理预览
              </Button>
            ) : null}
            {permissions.canCreate ? (
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => {
                  form.setFieldsValue({ driver: 'bridge', labels: [], options: [] });
                  setCreateOpen(true);
                }}
              >
                创建网络
              </Button>
            ) : null}
          </Space>
        }
      >
        <Table<DockerResourceView>
          rowKey="id"
          columns={columns}
          dataSource={resources}
          loading={loading}
          scroll={{ x: 1120 }}
          pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
          locale={{
            emptyText: <DockerEmptyState title="暂无网络" description="当前 Docker daemon 未返回网络资源。" />,
          }}
        />
      </DockerSurfaceCard>

      <Drawer
        open={detailOpen}
        title="网络详情"
        styles={{ wrapper: { width: 860, maxWidth: '100vw' } }}
        loading={detailLoading}
        destroyOnHidden
        onClose={() => setDetailOpen(false)}
      >
        <Descriptions bordered column={1}>
          <Descriptions.Item label="名称">{selectedNetwork?.name || '-'}</Descriptions.Item>
          <Descriptions.Item label="ID">{selectedNetwork?.id || '-'}</Descriptions.Item>
          <Descriptions.Item label="驱动">{selectedNetwork?.driver || '-'}</Descriptions.Item>
          <Descriptions.Item label="作用域">{selectedNetwork?.scope || '-'}</Descriptions.Item>
          <Descriptions.Item label="标签">{tagMap(selectedNetwork?.labels)}</Descriptions.Item>
          <Descriptions.Item label="选项">{tagMap(selectedNetwork?.options)}</Descriptions.Item>
          <Descriptions.Item label="连接容器">{containersText(selectedNetwork)}</Descriptions.Item>
          <Descriptions.Item label="原始 inspect">
            <pre className="max-h-[320px] overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-100">
              {JSON.stringify(selectedNetwork?.inspect || selectedNetwork || {}, null, 2)}
            </pre>
          </Descriptions.Item>
        </Descriptions>
      </Drawer>

      <Drawer
        open={createOpen}
        title="创建 Docker 网络"
        styles={{ wrapper: { width: 760, maxWidth: '100vw' } }}
        destroyOnHidden
        onClose={() => setCreateOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" loading={submitting} onClick={() => void handleCreate()}>
              创建
            </Button>
          </Space>
        }
      >
        <Form<NetworkFormValues> form={form} layout="vertical">
          <div className="grid gap-4 md:grid-cols-2">
            <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入网络名称' }]}>
              <Input placeholder="例如：app_backend" />
            </Form.Item>
            <Form.Item label="驱动" name="driver">
              <Select
                options={[
                  { label: 'bridge', value: 'bridge' },
                  { label: 'overlay', value: 'overlay' },
                  { label: 'macvlan', value: 'macvlan' },
                  { label: 'host', value: 'host' },
                  { label: 'none', value: 'none' },
                ]}
              />
            </Form.Item>
          </div>
          <div className="grid gap-4 md:grid-cols-3">
            <Form.Item label="内部网络" name="internal" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item label="允许手动连接" name="attachable" valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item label="启用 IPv6" name="enableIpv6" valuePropName="checked">
              <Switch />
            </Form.Item>
          </div>
          {(['labels', 'options'] as const).map((field) => (
            <Form.List key={field} name={field}>
              {(fields, { add, remove }) => (
                <div className="mb-5">
                  <div className="mb-2 flex items-center justify-between">
                    <span className="text-sm font-medium text-slate-700">{field === 'labels' ? '标签' : '驱动选项'}</span>
                    <Button size="small" icon={<PlusOutlined />} onClick={() => add({})}>
                      添加
                    </Button>
                  </div>
                  <div className="space-y-2">
                    {fields.map(({ key, name }) => (
                      <Space key={key} align="baseline" className="w-full">
                        <Form.Item name={[name, 'key']} className="mb-0 flex-1">
                          <Input placeholder="key" />
                        </Form.Item>
                        <Form.Item name={[name, 'value']} className="mb-0 flex-1">
                          <Input placeholder="value" />
                        </Form.Item>
                        <Button type="text" danger icon={<MinusCircleOutlined />} onClick={() => remove(name)} />
                      </Space>
                    ))}
                  </div>
                </div>
              )}
            </Form.List>
          ))}
        </Form>
      </Drawer>

      <Drawer
        open={connectionOpen}
        title={connectionMode === 'connect' ? '连接容器到网络' : '从网络断开容器'}
        styles={{ wrapper: { width: 640, maxWidth: '100vw' } }}
        destroyOnHidden
        onClose={() => setConnectionOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setConnectionOpen(false)}>取消</Button>
            <Button type="primary" loading={submitting} icon={<LinkOutlined />} onClick={() => void handleConnection()}>
              {connectionMode === 'connect' ? '连接' : '断开'}
            </Button>
          </Space>
        }
      >
        <Form<NetworkConnectionValues> form={connectionForm} layout="vertical">
          <Form.Item label="网络">{selectedNetwork?.name || selectedNetwork?.id || '-'}</Form.Item>
          <Form.Item label="容器" name="containerId" rules={[{ required: true, message: '请选择或输入容器 ID' }]}>
            <Select
              showSearch
              placeholder="选择容器或输入容器 ID"
              optionFilterProp="label"
              options={containers.map((container) => ({
                label: `${container.name || shortId(container.id)} · ${shortId(container.id)}`,
                value: container.id,
              }))}
            />
          </Form.Item>
          {connectionMode === 'connect' ? (
            <Form.Item label="网络别名" name="aliasesText" tooltip="多个别名可用换行或英文逗号分隔">
              <Input.TextArea autoSize={{ minRows: 3, maxRows: 6 }} placeholder="可选，例如：api, backend" />
            </Form.Item>
          ) : null}
        </Form>
      </Drawer>

      <Drawer
        open={pruneOpen}
        title="网络清理预览"
        styles={{ wrapper: { width: 680, maxWidth: '100vw' } }}
        loading={pruneLoading}
        destroyOnHidden
        onClose={() => setPruneOpen(false)}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} loading={pruneLoading} onClick={() => void loadPrunePreview()}>
              重新预览
            </Button>
            {permissions.canPrune ? (
              <Button danger icon={<DeleteOutlined />} loading={submitting} disabled={!prunePreview?.count} onClick={() => void applyPrune()}>
                执行清理
              </Button>
            ) : null}
          </Space>
        }
      >
        <Descriptions bordered column={1}>
          <Descriptions.Item label="将清理网络数">{prunePreview?.count ?? 0}</Descriptions.Item>
          <Descriptions.Item label="预计释放">{formatBytes(prunePreview?.reclaimBytes)}</Descriptions.Item>
          <Descriptions.Item label="候选网络">
            {prunePreview?.resourceIds?.length ? (
              <Space size={4} wrap>
                {prunePreview.resourceIds.map((id) => (
                  <Tag key={id}>{id}</Tag>
                ))}
              </Space>
            ) : (
              '无'
            )}
          </Descriptions.Item>
          {prunePreview?.warning ? <Descriptions.Item label="提示">{prunePreview.warning}</Descriptions.Item> : null}
        </Descriptions>
      </Drawer>
    </div>
  );
}

export default DockerNetworksTab;
