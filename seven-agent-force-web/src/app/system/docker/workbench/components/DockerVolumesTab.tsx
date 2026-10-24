'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Button,
  Descriptions,
  Drawer,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  DatabaseOutlined,
  DeleteOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
  ScissorOutlined,
} from '@ant-design/icons';
import {
  applyDockerVolumePrune,
  createDockerVolume,
  deleteDockerVolume,
  getDockerVolume,
  getDockerVolumes,
  previewDockerVolumePrune,
  type DockerResourcePrunePreview,
  type DockerResourceView,
  type DockerVolumeCreateRequest,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerEmptyState, DockerSurfaceCard, formatBytes } from '../../components/dockerConsole';
import { formatAbsoluteTime, shortId } from '../../components/dockerFormat';

interface KeyValueFormRow {
  key?: string;
  value?: string;
}

type VolumeFormValues = Omit<DockerVolumeCreateRequest, 'labels' | 'driverOpts'> & {
  labels?: KeyValueFormRow[];
  driverOptions?: KeyValueFormRow[];
};

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

interface DockerVolumesTabProps {
  refreshToken?: number;
}

export function DockerVolumesTab({ refreshToken = 0 }: DockerVolumesTabProps) {
  const [form] = Form.useForm<VolumeFormValues>();
  const [resources, setResources] = useState<DockerResourceView[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [detailOpen, setDetailOpen] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [selectedVolume, setSelectedVolume] = useState<DockerResourceView | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [pruneOpen, setPruneOpen] = useState(false);
  const [pruneLoading, setPruneLoading] = useState(false);
  const [prunePreview, setPrunePreview] = useState<DockerResourcePrunePreview | null>(null);
  const permissions = usePermissionFlags({
    canList: DOCKER_PERMISSIONS.VOLUME_LIST,
    canQuery: DOCKER_PERMISSIONS.VOLUME_QUERY,
    canCreate: DOCKER_PERMISSIONS.VOLUME_CREATE,
    canDelete: DOCKER_PERMISSIONS.VOLUME_DELETE,
    canPrune: DOCKER_PERMISSIONS.VOLUME_PRUNE,
  });

  const loadVolumes = useCallback(async () => {
    if (!permissions.canList) {
      setResources([]);
      return;
    }
    setLoading(true);
    try {
      const response = await getDockerVolumes({ current: 1, size: 500, keyword: keyword.trim() || undefined });
      setResources(response.data.records || []);
    } catch (error) {
      message.error((error as Error).message || '加载 Docker 存储卷失败');
      setResources([]);
    } finally {
      setLoading(false);
    }
  }, [keyword, permissions.canList]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadVolumes();
    }, 200);
    return () => window.clearTimeout(timer);
  }, [loadVolumes, refreshToken]);

  const openDetail = useCallback(
    async (volume: DockerResourceView) => {
      if (!permissions.canQuery) {
        return;
      }
      setSelectedVolume(volume);
      setDetailOpen(true);
      setDetailLoading(true);
      try {
        const response = await getDockerVolume(volume.name || volume.id);
        setSelectedVolume({
          ...response.data.resource,
          mountpoint: response.data.mountpoint || response.data.resource.mountpoint,
          options: response.data.options || response.data.resource.options,
          inspect: response.data.inspect,
        });
      } catch (error) {
        message.error((error as Error).message || '加载存储卷详情失败');
      } finally {
        setDetailLoading(false);
      }
    },
    [permissions.canQuery],
  );

  const handleCreate = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      await createDockerVolume({
        ...values,
        driver: values.driver || undefined,
        labels: keyValueMap(values.labels),
        driverOpts: keyValueMap(values.driverOptions),
      });
      message.success('存储卷已创建');
      setCreateOpen(false);
      form.resetFields();
      await loadVolumes();
    } finally {
      setSubmitting(false);
    }
  };

  const confirmDelete = useCallback(
    (volume: DockerResourceView) => {
      Modal.confirm({
        title: '删除 Docker 存储卷',
        content: `确认删除存储卷「${volume.name || volume.id}」？正在被容器使用的存储卷会被 Docker 拒绝删除。`,
        okText: '删除',
        okButtonProps: { danger: true },
        cancelText: '取消',
        onOk: async () => {
          await deleteDockerVolume(volume.name || volume.id);
          message.success('存储卷已删除');
          await loadVolumes();
        },
      });
    },
    [loadVolumes],
  );

  const loadPrunePreview = useCallback(async () => {
    setPruneLoading(true);
    try {
      const response = await previewDockerVolumePrune();
      setPrunePreview(response.data);
    } catch (error) {
      message.error((error as Error).message || '生成存储卷清理预览失败');
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
      const response = await applyDockerVolumePrune({ previewToken: prunePreview?.previewToken });
      message.success(`存储卷清理任务已提交 #${response.data.operationId}`);
      setPruneOpen(false);
      await loadVolumes();
    } finally {
      setSubmitting(false);
    }
  };

  const columns = useMemo<ColumnsType<DockerResourceView>>(
    () => [
      {
        title: '名称',
        dataIndex: 'name',
        width: 260,
        render: (_, record) => (
          <Button
            type="link"
            className="h-auto px-0 text-left"
            disabled={!permissions.canQuery}
            onClick={() => void openDetail(record)}
          >
            <div>
              <div className="font-semibold text-slate-900">{record.name || shortId(record.id)}</div>
              <div className="mt-1 text-xs text-slate-400">{record.id ? shortId(record.id) : '-'}</div>
            </div>
          </Button>
        ),
      },
      { title: '驱动', dataIndex: 'driver', width: 130, render: (value) => value || 'local' },
      {
        title: '挂载点',
        dataIndex: 'mountpoint',
        ellipsis: true,
        render: (value) => value || '-',
      },
      {
        title: '状态',
        dataIndex: 'dangling',
        width: 120,
        render: (value) => <Tag color={value ? 'warning' : 'success'}>{value ? '未使用' : '使用中'}</Tag>,
      },
      {
        title: '大小',
        dataIndex: 'sizeBytes',
        width: 130,
        render: (value) => formatBytes(value as number),
      },
      { title: '创建时间', dataIndex: 'createdAt', width: 180, render: (value) => formatAbsoluteTime(value as string) },
      {
        title: '操作',
        key: 'option',
        width: 140,
        fixed: 'right',
        render: (_, record) =>
          permissions.canDelete ? (
            <Button type="link" size="small" danger onClick={() => confirmDelete(record)}>
              删除
            </Button>
          ) : null,
      },
    ],
    [confirmDelete, openDetail, permissions.canDelete, permissions.canQuery],
  );

  if (!permissions.canList) {
    return <DockerEmptyState title="无存储卷权限" description="当前账号没有查看 Docker 存储卷的权限。" />;
  }

  return (
    <div className="space-y-5">
      <DockerSurfaceCard
        title="Docker 存储卷"
        description="查看持久化存储卷、创建新卷，并预览未使用卷的清理范围。"
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
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadVolumes()}>
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
                  form.setFieldsValue({ driver: 'local', labels: [], driverOptions: [] });
                  setCreateOpen(true);
                }}
              >
                创建存储卷
              </Button>
            ) : null}
          </Space>
        }
      >
        <Table<DockerResourceView>
          rowKey={(record) => record.name || record.id}
          columns={columns}
          dataSource={resources}
          loading={loading}
          scroll={{ x: 980 }}
          pagination={{ pageSize: 20, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
          locale={{
            emptyText: <DockerEmptyState title="暂无存储卷" description="当前 Docker daemon 未返回存储卷资源。" />,
          }}
        />
      </DockerSurfaceCard>

      <Drawer
        open={detailOpen}
        title="存储卷详情"
        styles={{ wrapper: { width: 820, maxWidth: '100vw' } }}
        loading={detailLoading}
        destroyOnHidden
        onClose={() => setDetailOpen(false)}
      >
        <Descriptions bordered column={1}>
          <Descriptions.Item label="名称">{selectedVolume?.name || '-'}</Descriptions.Item>
          <Descriptions.Item label="驱动">{selectedVolume?.driver || 'local'}</Descriptions.Item>
          <Descriptions.Item label="挂载点">{selectedVolume?.mountpoint || '-'}</Descriptions.Item>
          <Descriptions.Item label="大小">{formatBytes(selectedVolume?.sizeBytes)}</Descriptions.Item>
          <Descriptions.Item label="标签">{tagMap(selectedVolume?.labels)}</Descriptions.Item>
          <Descriptions.Item label="驱动选项">{tagMap(selectedVolume?.options)}</Descriptions.Item>
          <Descriptions.Item label="原始 inspect">
            <pre className="max-h-[320px] overflow-auto rounded bg-slate-950 p-3 text-xs text-slate-100">
              {JSON.stringify(selectedVolume?.inspect || selectedVolume || {}, null, 2)}
            </pre>
          </Descriptions.Item>
        </Descriptions>
      </Drawer>

      <Drawer
        open={createOpen}
        title="创建 Docker 存储卷"
        styles={{ wrapper: { width: 720, maxWidth: '100vw' } }}
        destroyOnHidden
        onClose={() => setCreateOpen(false)}
        extra={
          <Space>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" loading={submitting} icon={<DatabaseOutlined />} onClick={() => void handleCreate()}>
              创建
            </Button>
          </Space>
        }
      >
        <Form<VolumeFormValues> form={form} layout="vertical">
          <div className="grid gap-4 md:grid-cols-2">
            <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入存储卷名称' }]}>
              <Input placeholder="例如：app_data" />
            </Form.Item>
            <Form.Item label="驱动" name="driver">
              <Input placeholder="默认 local" />
            </Form.Item>
          </div>
          {(['labels', 'driverOptions'] as const).map((field) => (
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
        open={pruneOpen}
        title="存储卷清理预览"
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
          <Descriptions.Item label="将清理存储卷数">{prunePreview?.count ?? 0}</Descriptions.Item>
          <Descriptions.Item label="预计释放">{formatBytes(prunePreview?.reclaimBytes)}</Descriptions.Item>
          <Descriptions.Item label="候选存储卷">
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

export default DockerVolumesTab;
