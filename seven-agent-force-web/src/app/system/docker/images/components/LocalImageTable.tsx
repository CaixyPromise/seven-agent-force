'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AutoComplete,
  Button,
  Drawer,
  Form,
  Input,
  Modal,
  Select,
  Spin,
  Table,
  Tag,
  message,
} from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  DeleteOutlined,
  DownloadOutlined,
  FileSearchOutlined,
  PushpinOutlined,
  ReloadOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import {
  deleteLocalDockerImage,
  getLocalDockerImage,
  getLocalDockerImageContainers,
  getLocalDockerImages,
  getDockerRepositories,
  pullDockerImage,
  pushDockerImage,
  tagDockerImage,
  type DockerContainerUsageView,
  type DockerImageDetailView,
  type DockerImagePullCommand,
  type DockerImagePushCommand,
  type DockerImageTagCommand,
  type DockerImageView,
  type DockerRemoteRegistryView,
} from '@/api/dockerController';
import { HasPermission } from '@/components/Permission/HasPermission';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerStateTag, DockerSurfaceCard, formatBytes } from '../../components/dockerConsole';
import { ImageDetailDrawer } from './ImageDetailDrawer';
import { ImageStartupDrawer } from './ImageStartupDrawer';

interface LocalImageTableProps {
  refreshToken?: number;
  registries: DockerRemoteRegistryView[];
}

interface TagModalValues {
  sourceImage: string;
  registryId?: API.Int64;
  targetRepositoryPath?: string;
  targetTag: string;
}

interface PushModalValues {
  sourceImage: string;
  registryId?: API.Int64;
  targetRepositoryPath?: string;
  targetTag?: string;
}

function stripRegistryProtocol(endpoint?: string) {
  if (!endpoint) {
    return '';
  }
  return endpoint.replace(/^https?:\/\//, '').replace(/\/+$/, '');
}

function extractRepositoryPath(sourceImage?: string) {
  if (!sourceImage) {
    return '';
  }
  const withoutDigest = sourceImage.split('@')[0];
  const lastColon = withoutDigest.lastIndexOf(':');
  const lastSlash = withoutDigest.lastIndexOf('/');
  if (lastColon > lastSlash) {
    return withoutDigest.slice(0, lastColon);
  }
  return withoutDigest;
}

function extractTag(sourceImage?: string) {
  if (!sourceImage) {
    return 'latest';
  }
  const withoutDigest = sourceImage.split('@')[0];
  const lastColon = withoutDigest.lastIndexOf(':');
  const lastSlash = withoutDigest.lastIndexOf('/');
  if (lastColon > lastSlash) {
    return withoutDigest.slice(lastColon + 1);
  }
  return 'latest';
}

export function LocalImageTable({ refreshToken = 0, registries }: LocalImageTableProps) {
  const actionRef = useRef<ActionType>(undefined);
  const permissions = usePermissionFlags({
    canTag: DOCKER_PERMISSIONS.IMAGE_TAG,
    canPush: DOCKER_PERMISSIONS.IMAGE_PUSH,
    canDelete: DOCKER_PERMISSIONS.IMAGE_DELETE,
    canStartup: DOCKER_PERMISSIONS.IMAGE_STARTUP_PREVIEW,
  });
  const [detailOpen, setDetailOpen] = useState(false);
  const [detail, setDetail] = useState<DockerImageDetailView | null>(null);
  const [usageOpen, setUsageOpen] = useState(false);
  const [usageRows, setUsageRows] = useState<DockerContainerUsageView[]>([]);
  const [usageTitle, setUsageTitle] = useState<string>();
  const [pullOpen, setPullOpen] = useState(false);
  const [tagOpen, setTagOpen] = useState(false);
  const [pushOpen, setPushOpen] = useState(false);
  const [startupOpen, setStartupOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [currentImage, setCurrentImage] = useState<DockerImageView | null>(null);
  const [pullForm] = Form.useForm<DockerImagePullCommand>();
  const [tagForm] = Form.useForm<TagModalValues>();
  const [pushForm] = Form.useForm<PushModalValues>();
  const [tagRepoOptions, setTagRepoOptions] = useState<Array<{ value: string }>>([]);
  const [pushRepoOptions, setPushRepoOptions] = useState<Array<{ value: string }>>([]);
  const [tagRepoLoading, setTagRepoLoading] = useState(false);
  const [pushRepoLoading, setPushRepoLoading] = useState(false);

  const refresh = () => actionRef.current?.reload();

  useEffect(() => {
    if (!refreshToken) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      actionRef.current?.reload();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [refreshToken]);

  const formatTime = (timestamp?: number) => (timestamp ? new Date(timestamp * 1000).toLocaleString() : '-');

  const buildRepositoryOptions = useCallback((image?: DockerImageView, registryId?: API.Int64) => {
    const options = new Set<string>();
    image?.repoTags?.forEach((item) => {
      const path = extractRepositoryPath(item);
      if (path) {
        options.add(path);
      }
    });
    const registry = registries.find((item) => item.id === registryId);
    const endpoint = stripRegistryProtocol(registry?.endpoint);
    return Array.from(options).map((value) => ({
      value: endpoint && !value.startsWith(endpoint) ? value : value.replace(new RegExp(`^${endpoint}/?`), ''),
    }));
  }, [registries]);

  const loadRemoteRepositories = useCallback(async (
    registryId: API.Int64 | undefined,
    image: DockerImageView | null,
    setter: (options: Array<{ value: string }>) => void,
    setLoading: (loading: boolean) => void,
  ) => {
    const fallback = buildRepositoryOptions(image ?? undefined, registryId);
    if (!registryId) {
      setter(fallback);
      return;
    }
    setLoading(true);
    try {
      const response = await getDockerRepositories(registryId, { current: 1, size: 100 });
      const remoteOptions = (response.data.records || []).map((item) => ({ value: item.repository }));
      const merged = new Map<string, { value: string }>();
      [...fallback, ...remoteOptions].forEach((item) => merged.set(item.value, item));
      setter(Array.from(merged.values()));
    } catch {
      setter(fallback);
    } finally {
      setLoading(false);
    }
  }, [buildRepositoryOptions]);

  const resolveTargetRepository = (pathValue: string, registryId?: API.Int64) => {
    const trimmed = pathValue.trim();
    if (!trimmed) {
      return '';
    }
    if (/^[\w.-]+(?::\d+)?\//.test(trimmed)) {
      return trimmed;
    }
    const registry = registries.find((item) => item.id === registryId);
    const endpoint = stripRegistryProtocol(registry?.endpoint);
    return endpoint ? `${endpoint}/${trimmed}` : trimmed;
  };

  const columns = useMemo<ProColumns<DockerImageView>[]>(() => [
    {
      title: '镜像 ID',
      dataIndex: 'imageId',
      ellipsis: true,
      width: 220,
      search: false,
    },
    {
      title: '镜像标签',
      dataIndex: 'repoTags',
      render: (_, record) =>
        record.repoTags?.length
          ? record.repoTags.map((tag) => (
              <Tag color="blue" key={tag}>
                {tag}
              </Tag>
            ))
          : <Tag>无标签</Tag>,
    },
    {
      title: '镜像摘要',
      dataIndex: 'repoDigests',
      search: false,
      render: (_, record) =>
        record.repoDigests?.length
          ? record.repoDigests.map((digest) => (
              <div className="mb-1 break-all" key={digest}>
                <Tag color="geekblue">{digest}</Tag>
              </div>
            ))
          : '-',
    },
    {
      title: '大小',
      dataIndex: 'size',
      search: false,
      width: 120,
      render: (_, record) => formatBytes(record.size),
    },
    {
      title: '引用容器数',
      dataIndex: 'usedByContainerCount',
      search: false,
      width: 120,
    },
    {
      title: '创建时间',
      dataIndex: 'created',
      search: false,
      width: 180,
      renderText: (_, record) => formatTime(record.created),
    },
    {
      title: '关键词',
      dataIndex: 'keyword',
      hideInTable: true,
      fieldProps: { placeholder: '按 tag / digest / imageId 搜索' },
    },
    {
      title: '操作',
      key: 'option',
      fixed: 'right',
      width: 172,
      valueType: 'option',
      render: (_, record) => (
        <div className="grid w-[144px] grid-cols-2 gap-x-2 gap-y-1">
          <HasPermission code={DOCKER_PERMISSIONS.IMAGE_QUERY}>
            <Button
              type="link"
              size="small"
              className="w-full min-w-0 justify-start px-0"
              icon={<FileSearchOutlined />}
              onClick={async () => {
                const response = await getLocalDockerImage(record.imageId);
                setDetail(response.data);
                setDetailOpen(true);
              }}
            >
              详情
            </Button>
          </HasPermission>
          <HasPermission code={DOCKER_PERMISSIONS.IMAGE_CONTAINERS}>
            <Button
              type="link"
              size="small"
              className="w-full min-w-0 justify-start px-0"
              onClick={async () => {
                const response = await getLocalDockerImageContainers(record.imageId);
                setUsageRows(response.data || []);
                setUsageTitle(record.imageId);
                setUsageOpen(true);
              }}
            >
              引用容器
            </Button>
          </HasPermission>
          {permissions.canStartup ? (
            <Button
              type="link"
              size="small"
              className="w-full min-w-0 justify-start px-0"
              onClick={() => {
                setCurrentImage(record);
                setStartupOpen(true);
              }}
            >
              启动容器
            </Button>
          ) : null}
          {permissions.canTag ? (
            <Button
              type="link"
              size="small"
              className="w-full min-w-0 justify-start px-0"
              icon={<PushpinOutlined />}
              onClick={() => {
                const sourceImage = record.repoTags?.[0] || record.imageId;
                const registryId = registries[0]?.id;
                setCurrentImage(record);
                tagForm.setFieldsValue({
                  sourceImage,
                  registryId,
                  targetRepositoryPath: extractRepositoryPath(sourceImage),
                  targetTag: extractTag(sourceImage),
                });
                void loadRemoteRepositories(registryId, record, setTagRepoOptions, setTagRepoLoading);
                setTagOpen(true);
              }}
            >
              打标签
            </Button>
          ) : null}
          {permissions.canPush ? (
            <Button
              type="link"
              size="small"
              className="w-full min-w-0 justify-start px-0"
              icon={<UploadOutlined />}
              onClick={() => {
                const sourceImage = record.repoTags?.[0] || record.imageId;
                const registryId = registries[0]?.id;
                setCurrentImage(record);
                pushForm.setFieldsValue({
                  sourceImage,
                  registryId,
                  targetRepositoryPath: extractRepositoryPath(sourceImage),
                  targetTag: extractTag(sourceImage),
                });
                void loadRemoteRepositories(registryId, record, setPushRepoOptions, setPushRepoLoading);
                setPushOpen(true);
              }}
            >
              推送
            </Button>
          ) : null}
          {permissions.canDelete ? (
            <Button
              type="link"
              size="small"
              danger
              className="w-full min-w-0 justify-start px-0"
              icon={<DeleteOutlined />}
              onClick={() =>
                void Modal.confirm({
                  title: `确定删除镜像 ${record.imageId} 吗？`,
                  okText: '删除',
                  cancelText: '取消',
                  okButtonProps: { danger: true },
                  onOk: async () => {
                    const response = await deleteLocalDockerImage(record.imageId);
                    message.success(`镜像删除操作已提交 #${response.data.operationId}`);
                    refresh();
                  },
                })
              }
            >
              删除
            </Button>
          ) : null}
        </div>
      ),
    },
  ], [
    loadRemoteRepositories,
    permissions.canDelete,
    permissions.canPush,
    permissions.canStartup,
    permissions.canTag,
    pushForm,
    registries,
    tagForm,
  ]);

  const submitPull = async () => {
    const values = await pullForm.validateFields();
    setSubmitting(true);
    try {
      const response = await pullDockerImage(values);
      message.success(`镜像拉取操作已提交 #${response.data.operationId}`);
      setPullOpen(false);
      pullForm.resetFields();
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const submitTag = async () => {
    const values = await tagForm.validateFields();
    setSubmitting(true);
    try {
      const payload: DockerImageTagCommand = {
        sourceImage: values.sourceImage,
        targetRepository: resolveTargetRepository(values.targetRepositoryPath || '', values.registryId),
        targetTag: values.targetTag,
      };
      const response = await tagDockerImage(payload);
      message.success(`镜像打标操作已提交 #${response.data.operationId}`);
      setTagOpen(false);
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  const submitPush = async () => {
    const values = await pushForm.validateFields();
    setSubmitting(true);
    try {
      const payload: DockerImagePushCommand = {
        sourceImage: values.sourceImage,
        registryId: values.registryId,
        targetRepository: values.targetRepositoryPath
          ? resolveTargetRepository(values.targetRepositoryPath, values.registryId)
          : undefined,
        targetTag: values.targetTag,
      };
      const response = await pushDockerImage(payload);
      message.success(`镜像推送操作已提交 #${response.data.operationId}`);
      setPushOpen(false);
      refresh();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="space-y-5">
      <DockerSurfaceCard title="本地镜像清单" compact>
        <ProTable<DockerImageView>
          rowKey="imageId"
          actionRef={actionRef}
          columns={columns}
          scroll={{ x: 1500 }}
          search={{
            labelWidth: 100,
            collapsed: false,
            collapseRender: false,
          }}
          options={false}
          toolBarRender={() => [
            <HasPermission code={DOCKER_PERMISSIONS.IMAGE_PULL} key="pull">
              <Button
                type="primary"
                icon={<DownloadOutlined />}
                onClick={() => {
                  pullForm.resetFields();
                  setPullOpen(true);
                }}
              >
                拉取镜像
              </Button>
            </HasPermission>,
            <Button key="refresh" icon={<ReloadOutlined />} onClick={refresh}>
              刷新
            </Button>,
          ]}
          request={async (params) => {
            try {
              const response = await getLocalDockerImages({
                current: Number(params.current || 1),
                size: Number(params.pageSize || 10),
                keyword: params.keyword as string | undefined,
              });
              return {
                data: response.data.records || [],
                total: response.data.total || 0,
                success: true,
              };
            } catch (error) {
              message.error((error as Error).message || '获取本地镜像失败');
              return {
                data: [],
                total: 0,
                success: false,
              };
            }
          }}
          pagination={{
            pageSize: 10,
            showSizeChanger: true,
          }}
          tableAlertRender={false}
        />
      </DockerSurfaceCard>

      <ImageDetailDrawer
        open={detailOpen}
        detail={detail}
        onClose={() => {
          setDetailOpen(false);
          setDetail(null);
        }}
      />

      <ImageStartupDrawer
        open={startupOpen}
        image={currentImage}
        onClose={() => {
          setStartupOpen(false);
          setCurrentImage(null);
        }}
        onStarted={refresh}
      />

      <Drawer
        open={usageOpen}
        size="large"
        title={usageTitle ? `引用容器 · ${usageTitle}` : '引用容器'}
        onClose={() => setUsageOpen(false)}
        destroyOnHidden
      >
        <Table<DockerContainerUsageView>
          rowKey="id"
          dataSource={usageRows}
          pagination={false}
          scroll={{ x: 960 }}
          columns={[
            { title: '容器 ID', dataIndex: 'id', render: (value) => <div className="break-all">{value}</div> },
            { title: '容器名称', dataIndex: 'name', ellipsis: true },
            { title: '镜像', dataIndex: 'image', ellipsis: true },
            { title: '状态', dataIndex: 'state', render: (value) => <DockerStateTag state={value} label={value || '-'} /> },
            { title: '状态描述', dataIndex: 'status', ellipsis: true },
          ]}
        />
      </Drawer>

      <Modal
        open={pullOpen}
        title="拉取镜像"
        forceRender
        onCancel={() => setPullOpen(false)}
        onOk={submitPull}
        confirmLoading={submitting}
        destroyOnHidden
        okText="开始拉取"
        cancelText="取消"
      >
        <Form<DockerImagePullCommand> form={pullForm} layout="vertical">
          <Form.Item label="仓库" name="repository" rules={[{ required: true, message: '请输入仓库名' }]}>
            <Input placeholder="例如：library/nginx" />
          </Form.Item>
          <Form.Item label="标签" name="tag">
            <Input placeholder="例如：latest" />
          </Form.Item>
          <Form.Item label="使用的仓库配置" name="registryId">
            <Select
              allowClear
              placeholder="可选，选择远程仓库配置"
              options={registries.map((item) => ({ label: item.name, value: item.id }))}
            />
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        open={tagOpen}
        title={currentImage ? `镜像打标签 · ${currentImage.imageId}` : '镜像打标签'}
        forceRender
        onCancel={() => setTagOpen(false)}
        onOk={submitTag}
        confirmLoading={submitting}
        destroyOnHidden
        okText="保存标签"
        cancelText="取消"
        width={860}
      >
        <Form<TagModalValues> form={tagForm} layout="vertical">
          <div className="space-y-4">
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="mb-4 text-sm font-medium text-slate-800">源镜像</div>
              <Form.Item label="源镜像标签" name="sourceImage" rules={[{ required: true, message: '请选择源镜像' }]} className="mb-0">
                <Select
                  options={(currentImage?.repoTags?.length ? currentImage.repoTags : [currentImage?.imageId || ''])
                    .filter(Boolean)
                    .map((item) => ({ label: item, value: item }))}
                />
              </Form.Item>
            </div>

            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="mb-4 text-sm font-medium text-slate-800">目标镜像</div>
              <div className="grid gap-4">
                {registries.length ? (
                  <Form.Item
                    label="目标镜像源"
                    name="registryId"
                    rules={[{ required: true, message: '请选择目标镜像源' }]}
                  >
                    <Select
                      allowClear
                      placeholder="选择镜像源"
                      options={registries.map((item) => ({
                        label: `${item.name} · ${stripRegistryProtocol(item.endpoint) || item.endpoint}`,
                        value: item.id,
                      }))}
                      onChange={(value) => {
                        void loadRemoteRepositories(value, currentImage, setTagRepoOptions, setTagRepoLoading);
                      }}
                    />
                  </Form.Item>
                ) : null}
                <Form.Item label="目标标签" name="targetTag" rules={[{ required: true, message: '请输入目标标签' }]}>
                  <Input placeholder="例如：latest / 0.5 / release-20260421" />
                </Form.Item>
                <Form.Item
                  label="仓库路径"
                  name="targetRepositoryPath"
                  rules={[{ required: true, message: '请输入目标仓库路径' }]}
                  extra={registries.length ? '优先从已有仓库中选择，也支持直接输入新路径。这里只填仓库路径，不带标签。' : '未配置镜像源时，可直接输入仓库路径。这里只填仓库路径，不带标签。'}
                >
                  <AutoComplete
                    showSearch
                    notFoundContent={tagRepoLoading ? <Spin size="small" /> : null}
                    options={tagRepoOptions.map((item) => ({ label: item.value, value: item.value }))}
                    filterOption={(inputValue, option) => (option?.value || '').toLowerCase().includes(inputValue.toLowerCase())}
                    placeholder={tagRepoLoading ? '正在加载仓库列表…' : '例如：demo/hello-world'}
                  />
                  </Form.Item>
              </div>
            </div>

            <Form.Item shouldUpdate noStyle>
              {({ getFieldValue }) => {
                const registryId = getFieldValue('registryId');
                const repo = getFieldValue('targetRepositoryPath');
                const tag = getFieldValue('targetTag');
                const preview = repo ? `${resolveTargetRepository(repo, registryId)}${tag ? `:${tag}` : ''}` : '-';
                return (
                  <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
                    完整标签预览：<span className="break-all font-medium text-slate-900">{preview}</span>
                  </div>
                );
              }}
            </Form.Item>
          </div>
        </Form>
      </Modal>

      <Modal
        open={pushOpen}
        title={currentImage ? `推送镜像 · ${currentImage.imageId}` : '推送镜像'}
        forceRender
        onCancel={() => setPushOpen(false)}
        onOk={submitPush}
        confirmLoading={submitting}
        destroyOnHidden
        okText="开始推送"
        cancelText="取消"
        width={860}
      >
        <Form<PushModalValues> form={pushForm} layout="vertical">
          <div className="space-y-4">
            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="mb-4 text-sm font-medium text-slate-800">源镜像</div>
              <Form.Item label="源镜像标签" name="sourceImage" rules={[{ required: true, message: '请选择源镜像' }]} className="mb-0">
                <Select
                  options={(currentImage?.repoTags?.length ? currentImage.repoTags : [currentImage?.imageId || ''])
                    .filter(Boolean)
                    .map((item) => ({ label: item, value: item }))}
                />
              </Form.Item>
            </div>

            <div className="rounded-2xl border border-slate-200 p-4">
              <div className="mb-4 text-sm font-medium text-slate-800">推送目标</div>
              <div className="grid gap-4">
                {registries.length ? (
                  <Form.Item
                    label="目标镜像源"
                    name="registryId"
                    rules={[{ required: true, message: '请选择目标镜像源' }]}
                  >
                    <Select
                      allowClear
                      placeholder="选择推送目标镜像源"
                      options={registries.map((item) => ({
                        label: `${item.name} · ${stripRegistryProtocol(item.endpoint) || item.endpoint}`,
                        value: item.id,
                      }))}
                      onChange={(value) => {
                        void loadRemoteRepositories(value, currentImage, setPushRepoOptions, setPushRepoLoading);
                      }}
                    />
                  </Form.Item>
                ) : null}
                <Form.Item label="目标标签" name="targetTag">
                  <Input placeholder="例如：latest / 0.5 / plan" />
                </Form.Item>
                <Form.Item
                  label="仓库路径"
                  name="targetRepositoryPath"
                  extra={registries.length ? '优先从已有仓库中选择；留空则沿用当前镜像名。这里只填仓库路径，不带标签。' : '未配置镜像源时可直接推到已登录仓库；留空则沿用当前镜像名。这里只填仓库路径，不带标签。'}
                >
                  <AutoComplete
                    showSearch
                    notFoundContent={pushRepoLoading ? <Spin size="small" /> : null}
                    options={pushRepoOptions.map((item) => ({ label: item.value, value: item.value }))}
                    filterOption={(inputValue, option) => (option?.value || '').toLowerCase().includes(inputValue.toLowerCase())}
                    placeholder={pushRepoLoading ? '正在加载仓库列表…' : '例如：demo/hello-world'}
                  />
                </Form.Item>
              </div>
            </div>

            <Form.Item shouldUpdate noStyle>
              {({ getFieldValue }) => {
                const registryId = getFieldValue('registryId');
                const repo =
                  getFieldValue('targetRepositoryPath') ||
                  extractRepositoryPath(getFieldValue('sourceImage'));
                const tag = getFieldValue('targetTag') || extractTag(getFieldValue('sourceImage'));
                const preview = repo ? `${resolveTargetRepository(repo, registryId)}${tag ? `:${tag}` : ''}` : '-';
                return (
                  <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
                    推送目标预览：<span className="break-all font-medium text-slate-900">{preview}</span>
                  </div>
                );
              }}
            </Form.Item>
          </div>
        </Form>
      </Modal>
    </div>
  );
}
