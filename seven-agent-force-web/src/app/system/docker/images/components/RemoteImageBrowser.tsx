'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Input, Modal, Select, Space, Spin, Table, Tag, message } from 'antd';
import { EditOutlined, ReloadOutlined, SafetyOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import {
  getDockerRepositories,
  getDockerRepositoryManifest,
  getDockerRepositoryTags,
  pullRemoteDockerImage,
  type DockerRegistryConnectionTestView,
  type DockerRemoteManifestView,
  type DockerRemoteRegistryView,
  type DockerRemoteRepositoryView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerCodeBlock, DockerEmptyState, DockerSurfaceCard, formatBytes } from '../../components/dockerConsole';

interface RemoteImageBrowserProps {
  registry: DockerRemoteRegistryView;
  onRefreshRegistries: () => Promise<void> | void;
  onEditRegistry: (registry: DockerRemoteRegistryView) => void;
  onTestRegistry: (registry: DockerRemoteRegistryView) => Promise<DockerRegistryConnectionTestView | null>;
}

interface ManifestSummary {
  osText: string;
  archText: string;
  createdText: string;
  childCount: number | null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

function extractManifestSummary(manifest?: DockerRemoteManifestView | null): ManifestSummary {
  const payload = isRecord(manifest?.payload) ? manifest.payload : {};
  const annotations = isRecord(payload.annotations) ? payload.annotations : {};
  const config = isRecord(payload.config) ? payload.config : {};
  const history = Array.isArray(payload.history) ? payload.history : [];
  const firstHistory = isRecord(history[0]) ? history[0] : {};
  const manifestChildren = Array.isArray(payload.manifests)
    ? payload.manifests.filter((item): item is Record<string, unknown> => isRecord(item))
    : [];
  const layerChildren = Array.isArray(payload.layers) ? payload.layers : [];
  const platforms = manifestChildren
    .map((item) => (isRecord(item.platform) ? item.platform : null))
    .filter((item): item is Record<string, unknown> => !!item)
    .map((platform) => ({
      os: stringValue(platform.os),
      architecture: stringValue(platform.architecture),
      variant: stringValue(platform.variant),
    }));

  const osSet = new Set<string>();
  const archSet = new Set<string>();
  platforms.forEach((platform) => {
    if (platform.os) {
      osSet.add(platform.os);
    }
    if (platform.architecture) {
      archSet.add(platform.variant ? `${platform.architecture}/${platform.variant}` : platform.architecture);
    }
  });

  const createdRaw =
    stringValue(annotations['org.opencontainers.image.created']) ||
    stringValue(payload.created) ||
    stringValue(config.created) ||
    stringValue(firstHistory.created);

  return {
    osText: Array.from(osSet).join(', ') || '-',
    archText: Array.from(archSet).join(', ') || '-',
    createdText: createdRaw ? new Date(createdRaw).toLocaleString() : '-',
    childCount: manifestChildren.length || layerChildren.length || null,
  };
}

function manifestCacheKey(repository: string, tag: string) {
  return `${repository}@@${tag}`;
}

export function RemoteImageBrowser({
  registry,
  onEditRegistry,
  onTestRegistry,
}: RemoteImageBrowserProps) {
  const permissions = usePermissionFlags({
    canUpdateRegistry: DOCKER_PERMISSIONS.REGISTRY_UPDATE,
    canTestRegistry: DOCKER_PERMISSIONS.REGISTRY_TEST,
    canPullImage: DOCKER_PERMISSIONS.IMAGE_PULL,
  });

  const authLabelMap: Record<string, string> = {
    ANONYMOUS: '匿名访问',
    BASIC: 'Basic 认证',
  };

  const [repoLoading, setRepoLoading] = useState(false);
  const [repositories, setRepositories] = useState<DockerRemoteRepositoryView[]>([]);
  const [repoCurrent, setRepoCurrent] = useState(1);
  const [repoSize, setRepoSize] = useState(10);
  const [repoTotal, setRepoTotal] = useState(0);
  const [repoKeyword, setRepoKeyword] = useState('');

  const [expandedRepositoryKeys, setExpandedRepositoryKeys] = useState<string[]>([]);
  const [expandedTagKeys, setExpandedTagKeys] = useState<string[]>([]);
  const [tagsLoading, setTagsLoading] = useState<Record<string, boolean>>({});
  const [tagsByRepository, setTagsByRepository] = useState<Record<string, string[]>>({});
  const [manifestLoading, setManifestLoading] = useState<Record<string, boolean>>({});
  const [manifestByTag, setManifestByTag] = useState<Record<string, DockerRemoteManifestView | null>>({});
  const [connectionResult, setConnectionResult] = useState<DockerRegistryConnectionTestView | null>(null);
  const [pullModalOpen, setPullModalOpen] = useState(false);
  const [pulling, setPulling] = useState(false);
  const [pullRepository, setPullRepository] = useState<string>();
  const [pullTag, setPullTag] = useState<string>();

  const loadRepositories = useCallback(async (
    current = 1,
    size = repoSize,
    keyword = repoKeyword,
  ) => {
    setRepoLoading(true);
    try {
      const response = await getDockerRepositories(registry.id, {
        current,
        size,
        keyword: keyword || undefined,
      });
      setRepositories(response.data.records || []);
      setRepoTotal(response.data.total || 0);
      setRepoCurrent(current);
      setRepoSize(size);
    } catch (error) {
      message.error((error as Error).message || '获取镜像列表失败');
      setRepositories([]);
      setRepoTotal(0);
    } finally {
      setRepoLoading(false);
    }
  }, [registry.id, repoKeyword, repoSize]);
  const loadRepositoriesRef = useRef(loadRepositories);

  useEffect(() => {
    loadRepositoriesRef.current = loadRepositories;
  }, [loadRepositories]);

  const loadTags = async (repository: string) => {
    setTagsLoading((prev) => ({ ...prev, [repository]: true }));
    try {
      const response = await getDockerRepositoryTags(registry.id, repository);
      const tags = response.data.tags || [];
      setTagsByRepository((prev) => ({ ...prev, [repository]: tags }));
      return tags;
    } catch (error) {
      message.error((error as Error).message || '获取标签失败');
      setTagsByRepository((prev) => ({ ...prev, [repository]: [] }));
      return [];
    } finally {
      setTagsLoading((prev) => ({ ...prev, [repository]: false }));
    }
  };

  const loadManifest = async (repository: string, tag: string) => {
    const key = manifestCacheKey(repository, tag);
    setManifestLoading((prev) => ({ ...prev, [key]: true }));
    try {
      const response = await getDockerRepositoryManifest(registry.id, repository, tag);
      setManifestByTag((prev) => ({ ...prev, [key]: response.data }));
    } catch (error) {
      message.error((error as Error).message || '获取 manifest 失败');
      setManifestByTag((prev) => ({ ...prev, [key]: null }));
    } finally {
      setManifestLoading((prev) => ({ ...prev, [key]: false }));
    }
  };

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadRepositoriesRef.current(1);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [registry.id]);

  const handleRepositoryExpand = async (expanded: boolean, repository: string) => {
    if (!expanded) {
      setExpandedRepositoryKeys((prev) => prev.filter((item) => item !== repository));
      setExpandedTagKeys([]);
      return;
    }
    setExpandedRepositoryKeys([repository]);
    setExpandedTagKeys([]);
    if (!tagsByRepository[repository]) {
      await loadTags(repository);
    }
  };

  const handleTagExpand = async (expanded: boolean, repository: string, tag: string) => {
    const key = manifestCacheKey(repository, tag);
    if (!expanded) {
      setExpandedTagKeys((prev) => prev.filter((item) => item !== key));
      return;
    }
    setExpandedTagKeys([key]);
    if (!manifestByTag[key]) {
      await loadManifest(repository, tag);
    }
  };

  const tagColumns = (repository: string): ColumnsType<{ tag: string }> => [
    {
      title: '标签',
      dataIndex: 'tag',
      width: 180,
      render: (value: string) => <Tag color="blue">{value}</Tag>,
    },
    {
      title: '引用',
      key: 'reference',
      width: 420,
      ellipsis: true,
      render: (_, record) => {
        const reference = `${repository}:${record.tag}`;
        return (
          <div className="block min-w-0 max-w-full truncate whitespace-nowrap text-slate-700" title={reference}>
            {reference}
          </div>
        );
      },
    },
    {
      title: '系统 / 架构',
      key: 'platform',
      width: 220,
      render: (_, record) => {
        const summary = extractManifestSummary(manifestByTag[manifestCacheKey(repository, record.tag)]);
        return `${summary.osText} / ${summary.archText}`;
      },
    },
    {
      title: '创建时间',
      key: 'created',
      width: 180,
      render: (_, record) => extractManifestSummary(manifestByTag[manifestCacheKey(repository, record.tag)]).createdText,
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      fixed: 'right',
      render: (_, record) =>
        permissions.canPullImage ? (
          <Button
            type="link"
            size="small"
            onClick={() => void handlePull(repository, record.tag)}
          >
            拉取到本地
          </Button>
        ) : null,
    },
  ];

  const repositoryColumns: ColumnsType<DockerRemoteRepositoryView> = [
    {
      title: '镜像名',
      dataIndex: 'repository',
      ellipsis: true,
      render: (value: string) => <span className="block max-w-full truncate">{value}</span>,
    },
    {
      title: '标签数量',
      key: 'tagCount',
      width: 120,
      render: (_, record) =>
        tagsLoading[record.repository]
          ? '加载中'
          : tagsByRepository[record.repository]
            ? tagsByRepository[record.repository].length
            : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 140,
      fixed: 'right',
      render: (_, record) =>
        permissions.canPullImage ? (
          <Button
            type="link"
            size="small"
            onClick={() => openPullSelector(record.repository)}
          >
            拉取到本地
          </Button>
        ) : null,
    },
  ];

  const availablePullTags = useMemo(() => {
    if (!pullRepository) {
      return [];
    }
    return tagsByRepository[pullRepository] || [];
  }, [pullRepository, tagsByRepository]);

  const openPullSelector = async (repository: string) => {
    const tags = tagsByRepository[repository] || await loadTags(repository);
    if (tags.length === 1) {
      await handlePull(repository, tags[0]);
      return;
    }
    setPullRepository(repository);
    setPullTag(tags[0]);
    setPullModalOpen(true);
  };

  const handlePull = async (repository: string, tag?: string) => {
    const resolvedTag = tag || pullTag;
    if (!resolvedTag) {
      message.warning('请选择要拉取的标签');
      return;
    }
    setPulling(true);
    try {
      const response = await pullRemoteDockerImage({
        registryId: registry.id,
        repository,
        tag: resolvedTag,
      });
      message.success(`远程镜像拉取操作已提交 #${response.data.operationId}`);
      setPullModalOpen(false);
    } catch (error) {
      message.error((error as Error).message || '拉取镜像失败');
    } finally {
      setPulling(false);
    }
  };

  const renderManifestPanel = (repository: string, tag: string) => {
    const key = manifestCacheKey(repository, tag);
    if (manifestLoading[key]) {
      return (
        <div className="flex min-h-[160px] items-center justify-center">
          <Spin />
        </div>
      );
    }
    const manifest = manifestByTag[key];
    if (!manifest) {
      return <DockerEmptyState title="暂未读取 manifest" description="展开标签后会在这里显示 manifest 详情。" />;
    }
    const summary = extractManifestSummary(manifest);
    return (
      <div className="space-y-4 py-2">
        <div className="overflow-hidden rounded-2xl border border-slate-200">
          <table className="w-full table-fixed border-collapse text-sm">
            <tbody>
              <tr className="border-b border-slate-200">
                <th className="w-32 bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">镜像</th>
                <td className="px-4 py-3 break-all text-slate-900">{manifest.repository}</td>
                <th className="w-32 bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">标签</th>
                <td className="px-4 py-3 break-all text-slate-900">{tag}</td>
              </tr>
              <tr className="border-b border-slate-200">
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">Digest</th>
                <td className="px-4 py-3 break-all text-slate-900" colSpan={3}>
                  {manifest.digest || '-'}
                </td>
              </tr>
              <tr className="border-b border-slate-200">
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">媒体类型</th>
                <td className="px-4 py-3 break-all text-slate-900">{manifest.mediaType || '-'}</td>
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">大小</th>
                <td className="px-4 py-3 text-slate-900">{formatBytes(manifest.size)}</td>
              </tr>
              <tr className="border-b border-slate-200">
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">Schema</th>
                <td className="px-4 py-3 text-slate-900">{manifest.schemaVersion ?? '-'}</td>
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">适配系统</th>
                <td className="px-4 py-3 text-slate-900">{manifest.os || summary.osText}</td>
              </tr>
              <tr className="border-b border-slate-200">
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">系统架构</th>
                <td className="px-4 py-3 text-slate-900">{manifest.variant ? `${manifest.architecture || '-'} / ${manifest.variant}` : (manifest.architecture || summary.archText)}</td>
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">创建时间</th>
                <td className="px-4 py-3 text-slate-900">{manifest.created ? new Date(manifest.created).toLocaleString() : summary.createdText}</td>
              </tr>
              <tr>
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">
                  {manifest.mediaType?.includes('index') || manifest.mediaType?.includes('manifest.list') ? '子清单数' : '层数量'}
                </th>
                <td className="px-4 py-3 text-slate-900">{manifest.childManifestCount ?? manifest.layerCount ?? summary.childCount ?? '-'}</td>
                <th className="bg-slate-50 px-4 py-3 text-left font-medium text-slate-500">层数量</th>
                <td className="px-4 py-3 text-slate-900">{manifest.layerCount ?? '-'}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <DockerCodeBlock value={JSON.stringify(manifest.payload || {}, null, 2)} />
      </div>
    );
  };

  return (
    <DockerSurfaceCard
      title={registry.name}
      compact
      extra={
        <Space wrap>
          <Tag color={registry.authType === 'BASIC' ? 'blue' : 'default'}>{authLabelMap[registry.authType] || registry.authType}</Tag>
          {registry.defaultRegistry ? <Tag color="processing">默认</Tag> : null}
          <Button icon={<ReloadOutlined />} onClick={() => void loadRepositories(1, repoSize, repoKeyword)}>
            刷新镜像
          </Button>
          {permissions.canTestRegistry ? (
            <Button
              icon={<SafetyOutlined />}
              onClick={async () => {
                const result = await onTestRegistry(registry);
                setConnectionResult(result);
              }}
            >
              测试连接
            </Button>
          ) : null}
          {permissions.canUpdateRegistry ? (
            <Button icon={<EditOutlined />} onClick={() => onEditRegistry(registry)}>
              编辑配置
            </Button>
          ) : null}
        </Space>
      }
    >
      <div className="space-y-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0 space-y-1">
            <div className="truncate text-sm font-medium text-slate-700">{registry.endpoint}</div>
            <div className="truncate text-xs text-slate-400">{registry.code}</div>
          </div>
          <Input.Search
            allowClear
            className="w-full lg:w-80"
            placeholder="按镜像名搜索"
            onSearch={(value) => {
              setRepoKeyword(value);
              void loadRepositories(1, repoSize, value);
            }}
          />
        </div>

        {connectionResult ? (
          <div
            className={`rounded-xl border px-4 py-3 text-sm ${
              connectionResult.success ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-rose-200 bg-rose-50 text-rose-700'
            }`}
          >
            <div className="font-medium">{connectionResult.message}</div>
            {connectionResult.serverHeader || connectionResult.registryVersion ? (
              <div className="mt-1 break-all text-xs opacity-80">
                响应头：{connectionResult.serverHeader || '-'}，服务端：{connectionResult.registryVersion || '-'}
              </div>
            ) : null}
          </div>
        ) : null}

        <Table<DockerRemoteRepositoryView>
          rowKey="repository"
          size="middle"
          loading={repoLoading}
          dataSource={repositories}
          columns={repositoryColumns}
          pagination={{
            current: repoCurrent,
            pageSize: repoSize,
            total: repoTotal,
            showSizeChanger: true,
            onChange: (page, pageSize) => void loadRepositories(page, pageSize, repoKeyword),
          }}
          expandable={{
            expandedRowKeys: expandedRepositoryKeys,
            onExpand: (expanded, record) => void handleRepositoryExpand(expanded, record.repository),
            expandedRowRender: (record) => {
              const repoTags = tagsByRepository[record.repository] || [];
              return (
                <div className="space-y-3 py-2">
                  <div className="overflow-hidden rounded-2xl border border-slate-200 bg-slate-50">
                    <div className="border-b border-slate-200 px-4 py-3 text-sm font-medium text-slate-700">
                      标签列表 · {record.repository}
                    </div>
                    <Table<{ tag: string }>
                      rowKey={(tagRecord) => manifestCacheKey(record.repository, tagRecord.tag)}
                      size="small"
                      pagination={false}
                      loading={!!tagsLoading[record.repository]}
                      scroll={{ x: 980 }}
                      dataSource={repoTags.map((tag) => ({ tag }))}
                      columns={tagColumns(record.repository)}
                      expandable={{
                        expandedRowKeys: expandedTagKeys,
                        onExpand: (expanded, tagRecord) => void handleTagExpand(expanded, record.repository, tagRecord.tag),
                        expandedRowRender: (tagRecord) => renderManifestPanel(record.repository, tagRecord.tag),
                        expandRowByClick: true,
                      }}
                      onRow={(tagRecord) => ({
                        onClick: () => {
                          const key = manifestCacheKey(record.repository, tagRecord.tag);
                          const expanded = expandedTagKeys.includes(key);
                          void handleTagExpand(!expanded, record.repository, tagRecord.tag);
                        },
                      })}
                      locale={{
                        emptyText: <DockerEmptyState title="该镜像暂无标签" description="当前镜像还没有可浏览的 tag。" />,
                      }}
                    />
                  </div>
                </div>
              );
            },
          }}
          locale={{
            emptyText: <DockerEmptyState title="暂无镜像" description="当前镜像源还没有可浏览的镜像列表。" />,
          }}
        />

        <Modal
          open={pullModalOpen}
          title={pullRepository ? `拉取镜像 · ${pullRepository}` : '拉取镜像'}
          onCancel={() => setPullModalOpen(false)}
          onOk={() => void handlePull(pullRepository || '', pullTag)}
          confirmLoading={pulling}
          okText="拉取到本地"
          cancelText="取消"
          destroyOnHidden
        >
          <div className="space-y-4">
            <div className="text-sm text-slate-500">选择要从当前镜像源拉取到本地的标签。</div>
            <Select
              className="w-full"
              value={pullTag}
              options={availablePullTags.map((tag) => ({ label: tag, value: tag }))}
              onChange={setPullTag}
              placeholder="选择标签"
            />
          </div>
        </Modal>
      </div>
    </DockerSurfaceCard>
  );
}
