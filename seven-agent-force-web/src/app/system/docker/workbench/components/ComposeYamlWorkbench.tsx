'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Empty,
  Form,
  Input,
  Progress,
  Skeleton,
  Space,
  Table,
  Tag,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CheckCircleOutlined,
  CloudUploadOutlined,
  FileSearchOutlined,
  PlayCircleOutlined,
  PlusOutlined,
  ReloadOutlined,
  SaveOutlined,
  StopOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  createDockerComposeProject,
  downDockerComposeProject,
  getDockerComposeProject,
  getDockerComposeProjectPS,
  getDockerComposeProjects,
  getDockerContainer,
  importDockerComposeDiscoveredProject,
  previewDockerComposeProject,
  restartDockerComposeProject,
  upDockerComposeProject,
  updateDockerComposeProject,
  validateDockerComposeProject,
  type DockerComposePreviewView,
  type DockerComposeProjectCreateRequest,
  type DockerComposeProjectDetailVO,
  type DockerComposeProjectStatus,
  type DockerComposeProjectSummaryVO,
  type DockerContainerDetailView,
  type DockerContainerView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { ContainerDetailDrawer } from '../../containers/components/ContainerDetailDrawer';
import { DockerStateTag } from '../../components/dockerConsole';
import {
  formatAbsoluteTime,
  formatComposeProjectStatusLabel,
  formatContainerStateLabel,
  formatOperationTypeLabel,
  normalizeState,
} from '../../components/dockerFormat';

type ComposeAction = 'save' | 'validate' | 'preview' | 'ps' | 'up' | 'down' | 'restart' | 'import';

const DEFAULT_COMPOSE_YAML = `services:
  app:
    image: nginx:latest
    ports:
      - "8080:80"
`;

function projectStatusColor(status?: DockerComposeProjectStatus) {
  if (status === 'running') {
    return 'success';
  }
  if (status === 'degraded') {
    return 'warning';
  }
  if (status === 'stopped') {
    return 'default';
  }
  return 'processing';
}

function matchProject(project: DockerComposeProjectSummaryVO, key: string) {
  const normalized = key.trim();
  if (!normalized) {
    return false;
  }
  return project.projectId === normalized || project.projectName === normalized;
}

interface ComposeYamlWorkbenchProps {
  refreshToken?: number;
  requestedProject?: string;
}

export function ComposeYamlWorkbench({ refreshToken = 0, requestedProject }: ComposeYamlWorkbenchProps) {
  const [projects, setProjects] = useState<DockerComposeProjectSummaryVO[]>([]);
  const [projectsLoading, setProjectsLoading] = useState(false);
  const [selectedProjectId, setSelectedProjectId] = useState('');
  const [selectedProject, setSelectedProject] = useState<DockerComposeProjectDetailVO | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [composeYaml, setComposeYaml] = useState('');
  const [preview, setPreview] = useState<DockerComposePreviewView | null>(null);
  const [containers, setContainers] = useState<DockerContainerView[]>([]);
  const [actionLoading, setActionLoading] = useState<Partial<Record<ComposeAction, boolean>>>({});
  const [createOpen, setCreateOpen] = useState(false);
  const [createSubmitting, setCreateSubmitting] = useState(false);
  const [containerDetailOpen, setContainerDetailOpen] = useState(false);
  const [containerDetailLoading, setContainerDetailLoading] = useState(false);
  const [containerDetail, setContainerDetail] = useState<DockerContainerDetailView | null>(null);
  const [containerDetailInitialTab, setContainerDetailInitialTab] = useState<'overview' | 'inspect' | 'stats' | 'logs'>('overview');
  const [form] = Form.useForm<DockerComposeProjectCreateRequest>();
  const permissions = usePermissionFlags({
    canList: DOCKER_PERMISSIONS.COMPOSE_PROJECT_LIST,
    canCreate: DOCKER_PERMISSIONS.COMPOSE_PROJECT_CREATE,
    canQuery: DOCKER_PERMISSIONS.COMPOSE_PROJECT_QUERY,
    canUpdate: DOCKER_PERMISSIONS.COMPOSE_PROJECT_UPDATE,
    canValidate: DOCKER_PERMISSIONS.COMPOSE_VALIDATE,
    canUp: DOCKER_PERMISSIONS.COMPOSE_UP,
    canContainerLogs: DOCKER_PERMISSIONS.CONTAINER_LOGS,
  });

  const selectedSummary = useMemo(
    () => projects.find((project) => project.projectId === selectedProjectId),
    [projects, selectedProjectId],
  );

  const filteredProjects = useMemo(() => {
    const normalized = keyword.trim().toLowerCase();
    if (!normalized) {
      return projects;
    }
    return projects.filter((project) =>
      [project.projectId, project.projectName, project.workingDir, ...(project.configFiles || [])]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(normalized)),
    );
  }, [keyword, projects]);

  const loadProjects = useCallback(async () => {
    if (!permissions.canList) {
      setProjects([]);
      setSelectedProjectId('');
      return;
    }
    setProjectsLoading(true);
    try {
      const response = await getDockerComposeProjects({ current: 1, size: 200 });
      const rows = response.data.records || [];
      setProjects(rows);
      setSelectedProjectId((current) => {
        if (current && rows.some((project) => project.projectId === current)) {
          return current;
        }
        if (requestedProject) {
          return rows.find((project) => matchProject(project, requestedProject))?.projectId || rows[0]?.projectId || '';
        }
        return rows[0]?.projectId || '';
      });
    } catch (error) {
      message.error((error as Error).message || '加载 Compose 项目失败');
      setProjects([]);
    } finally {
      setProjectsLoading(false);
    }
  }, [permissions.canList, requestedProject]);

  const loadProjectDetail = useCallback(async (projectId: string) => {
    if (!projectId) {
      setSelectedProject(null);
      setComposeYaml('');
      setContainers([]);
      setPreview(null);
      return;
    }
    if (!permissions.canQuery) {
      setSelectedProject(null);
      setComposeYaml('');
      setContainers([]);
      setPreview(null);
      return;
    }
    setDetailLoading(true);
    try {
      const response = await getDockerComposeProject(projectId);
      const detail = response.data;
      setSelectedProject(detail);
      setComposeYaml(detail.composeYaml || detail.normalizedYaml || '');
      setContainers(detail.containers || []);
      setPreview(detail.preview || detail.validation ? {
        preview: detail.preview || { safe: true },
        validation: detail.validation || { valid: true },
        services: detail.services?.map((service) => service.serviceName),
        normalizedYaml: detail.normalizedYaml,
      } : null);
    } catch (error) {
      message.error((error as Error).message || '加载 Compose 项目详情失败');
      setSelectedProject(null);
    } finally {
      setDetailLoading(false);
    }
  }, [permissions.canQuery]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadProjects();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadProjects, refreshToken]);

  useEffect(() => {
    if (!requestedProject || !projects.length) {
      return;
    }
    const matched = projects.find((project) => matchProject(project, requestedProject));
    if (matched) {
      setSelectedProjectId(matched.projectId);
    }
  }, [projects, requestedProject]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadProjectDetail(selectedProjectId);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadProjectDetail, refreshToken, selectedProjectId]);

  const withActionLoading = async (action: ComposeAction, runner: () => Promise<void>) => {
    setActionLoading((prev) => ({ ...prev, [action]: true }));
    try {
      await runner();
    } finally {
      setActionLoading((prev) => ({ ...prev, [action]: false }));
    }
  };

  const requireManagedProject = () => {
    if (!selectedProject) {
      message.warning('请先选择 Compose 项目');
      return false;
    }
    if (selectedProject.source !== 'MANAGED') {
      message.warning('发现型 Compose 项目没有持久化 YAML，暂不支持该动作');
      return false;
    }
    return true;
  };

  const handleSave = () =>
    withActionLoading('save', async () => {
      if (!requireManagedProject() || !selectedProject) {
        return;
      }
      if (!permissions.canUpdate) {
        message.warning('当前账号没有更新 Compose 项目的权限');
        return;
      }
      const response = await updateDockerComposeProject(selectedProject.projectId, {
        composeYaml,
        validateBeforeSave: true,
        writeFiles: true,
      });
      message.success('Compose YAML 已保存');
      setSelectedProject(response.data);
      setComposeYaml(response.data.composeYaml || composeYaml);
      await loadProjects();
    });

  const handleValidate = () =>
    withActionLoading('validate', async () => {
      if (!requireManagedProject() || !selectedProject) {
        return;
      }
      if (!permissions.canValidate) {
        message.warning('当前账号没有校验 Compose 项目的权限');
        return;
      }
      const response = await validateDockerComposeProject(selectedProject.projectId);
      setPreview(response.data);
      message[response.data.validation.valid ? 'success' : 'warning'](
        response.data.validation.valid ? 'Compose 校验通过' : 'Compose 校验存在问题',
      );
    });

  const handlePreview = () =>
    withActionLoading('preview', async () => {
      if (!requireManagedProject() || !selectedProject) {
        return;
      }
      if (!permissions.canValidate) {
        message.warning('当前账号没有预览 Compose 项目的权限');
        return;
      }
      const response = await previewDockerComposeProject(selectedProject.projectId);
      setPreview(response.data);
      message[response.data.preview.safe ? 'success' : 'warning'](
        response.data.preview.safe ? '预览通过，未发现阻断风险' : '预览完成，存在策略风险',
      );
    });

  const handleImportDiscovered = () =>
    withActionLoading('import', async () => {
      if (!selectedProject) {
        message.warning('请先选择 Compose 项目');
        return;
      }
      if (!permissions.canCreate) {
        message.warning('当前账号没有导入 Compose 项目的权限');
        return;
      }
      if (selectedProject.source === 'MANAGED') {
        message.info('当前项目已由平台托管');
        return;
      }
      const response = await importDockerComposeDiscoveredProject({
        projectId: selectedProject.projectId,
        projectName: selectedProject.projectName,
      });
      message.success('发现型 Compose 项目已导入托管');
      await loadProjects();
      setSelectedProjectId(response.data.projectId || selectedProject.projectId);
      setSelectedProject(response.data);
      setComposeYaml(response.data.composeYaml || response.data.normalizedYaml || composeYaml);
    });

  const handlePS = () =>
    withActionLoading('ps', async () => {
      if (!selectedProject) {
        message.warning('请先选择 Compose 项目');
        return;
      }
      if (!permissions.canValidate) {
        message.warning('当前账号没有查询 Compose 运行态的权限');
        return;
      }
      if (selectedProject.source !== 'MANAGED') {
        setContainers(selectedProject.containers || []);
        return;
      }
      const response = await getDockerComposeProjectPS(selectedProject.projectId);
      setContainers(response.data.containers || []);
      message.success(`PS 查询完成，共 ${response.data.containers?.length || 0} 个容器`);
    });

  const handleRuntimeAction = (action: Extract<ComposeAction, 'up' | 'down' | 'restart'>) =>
    withActionLoading(action, async () => {
      if (!requireManagedProject() || !selectedProject) {
        return;
      }
      if (!permissions.canUp) {
        message.warning('当前账号没有执行 Compose 运行操作的权限');
        return;
      }
      const response =
        action === 'up'
          ? await upDockerComposeProject(selectedProject.projectId)
          : action === 'down'
            ? await downDockerComposeProject(selectedProject.projectId)
            : await restartDockerComposeProject(selectedProject.projectId);
      message.success(`${formatOperationTypeLabel(response.data.operationType)} 已提交 #${response.data.operationId}`);
      await loadProjects();
      await loadProjectDetail(selectedProject.projectId);
    });

  const openContainerDetail = async (
    container: DockerContainerView,
    initialTab: 'overview' | 'inspect' | 'stats' | 'logs' = 'overview',
  ) => {
    setContainerDetailOpen(true);
    setContainerDetailInitialTab(initialTab);
    setContainerDetailLoading(true);
    try {
      const response = await getDockerContainer(container.id);
      setContainerDetail(response.data);
    } catch (error) {
      message.error((error as Error).message || '加载容器详情失败');
    } finally {
      setContainerDetailLoading(false);
    }
  };

  const handleCreate = async (values: DockerComposeProjectCreateRequest) => {
    if (!permissions.canCreate) {
      message.warning('当前账号没有创建 Compose 项目的权限');
      return;
    }
    setCreateSubmitting(true);
    try {
      const response = await createDockerComposeProject({
        ...values,
        writeFiles: true,
        overwriteExisting: false,
        autoUp: false,
      });
      message.success('Compose 项目已创建');
      setCreateOpen(false);
      form.resetFields();
      await loadProjects();
      setSelectedProjectId(response.data.projectId);
    } finally {
      setCreateSubmitting(false);
    }
  };

  const containerColumns: ColumnsType<DockerContainerView> = [
    {
      title: '容器',
      dataIndex: 'name',
      render: (_, record) => (
        <Button type="link" className="px-0" onClick={() => void openContainerDetail(record)}>
          {record.name || record.id}
        </Button>
      ),
    },
    { title: '镜像', dataIndex: 'image', ellipsis: true },
    {
      title: '状态',
      dataIndex: 'state',
      width: 120,
      render: (_, record) => (
        <DockerStateTag state={record.state} label={formatContainerStateLabel(record.state)} />
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: 130,
      render: (_, record) => (
        <Space size={4}>
          {permissions.canContainerLogs ? (
            <Button type="link" className="px-0" onClick={() => void openContainerDetail(record, 'logs')}>
              日志
            </Button>
          ) : null}
          <Button type="link" className="px-0" onClick={() => void openContainerDetail(record, 'stats')}>
            状态
          </Button>
        </Space>
      ),
    },
  ];

  const runningCount = containers.filter((container) => {
    const state = normalizeState(container.state);
    return state === 'running' || state === 'restarting';
  }).length;
  const isManaged = selectedProject?.source === 'MANAGED';
  const sourceLabel = isManaged ? '托管' : '发现';

  return (
    <div className="grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
      <section className="rounded-2xl border border-[#e8edf5] bg-white p-4 shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
        <div className="flex items-center justify-between gap-3">
          <div>
            <div className="text-lg font-semibold text-slate-950">编排项目</div>
            <div className="mt-1 text-sm text-slate-500">选择项目后直接编辑 docker-compose.yaml。</div>
          </div>
          {permissions.canCreate ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
              创建
            </Button>
          ) : null}
        </div>
        <Input.Search
          className="mt-4"
          allowClear
          placeholder="搜索项目 / 目录"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
        />

        <div className="mt-4 min-h-[520px]">
          {projectsLoading ? (
            <Skeleton active paragraph={{ rows: 10 }} />
          ) : filteredProjects.length ? (
            <div className="space-y-2">
              {filteredProjects.map((project) => (
                <button
                  key={project.projectId}
                  type="button"
                  className={`w-full rounded-xl border px-3 py-3 text-left transition ${
                    selectedProjectId === project.projectId
                      ? 'border-blue-200 bg-blue-50'
                      : 'border-transparent hover:border-slate-200 hover:bg-slate-50'
                  }`}
                  onClick={() => setSelectedProjectId(project.projectId)}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-sm font-semibold text-slate-900">{project.projectName}</span>
                    <Tag color={projectStatusColor(project.status)}>
                      {formatComposeProjectStatusLabel(project.status)}
                    </Tag>
                  </div>
                  <div className="mt-2 truncate text-xs text-slate-500">
                    {project.workingDir || project.configFiles?.join(', ') || project.source}
                  </div>
                  <div className="mt-2 text-xs text-slate-500">
                    {project.runningCount}/{project.containerCount} 运行中
                  </div>
                </button>
              ))}
            </div>
          ) : (
            <Empty className="py-20" description="暂无 Compose 项目" />
          )}
        </div>
      </section>

      <section className="min-w-0 rounded-2xl border border-[#e8edf5] bg-white p-4 shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
        {detailLoading ? (
          <Skeleton active paragraph={{ rows: 14 }} />
        ) : selectedProject ? (
          <div className="space-y-4">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div>
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="m-0 text-xl font-semibold text-slate-950">{selectedProject.projectName}</h2>
                  <Tag color={projectStatusColor(selectedSummary?.status)}>
                    {formatComposeProjectStatusLabel(selectedSummary?.status)}
                  </Tag>
                  <Tag>{sourceLabel}</Tag>
                  {preview?.validation.valid ? <Tag icon={<CheckCircleOutlined />} color="success">校验通过</Tag> : null}
                </div>
                <div className="mt-2 text-sm text-slate-500">
                  {selectedProject.composeFilePath || selectedProject.workingDir || 'Compose 文件路径暂未返回'}
                </div>
                {selectedSummary?.activeOperation ? (
                  <div className="mt-3 max-w-xl">
                    <div className="mb-1 text-xs text-slate-500">
                      当前任务：{formatOperationTypeLabel(selectedSummary.activeOperation.operationType)} #{selectedSummary.activeOperation.operationId}
                    </div>
                    <Progress percent={selectedSummary.activeOperation.progress || 0} size="small" />
                  </div>
                ) : null}
              </div>
              <Space wrap>
                <Button icon={<ReloadOutlined />} onClick={() => void loadProjectDetail(selectedProject.projectId)}>
                  刷新
                </Button>
                <Button icon={<FileSearchOutlined />} loading={actionLoading.preview} disabled={!isManaged || !permissions.canValidate} onClick={handlePreview}>
                  预览
                </Button>
                <Button icon={<CheckCircleOutlined />} loading={actionLoading.validate} disabled={!isManaged || !permissions.canValidate} onClick={handleValidate}>
                  校验
                </Button>
                <Button icon={<SaveOutlined />} loading={actionLoading.save} disabled={!isManaged || !permissions.canUpdate} onClick={handleSave}>
                  保存
                </Button>
                {!isManaged && permissions.canCreate ? (
                  <Button
                    type="primary"
                    icon={<CloudUploadOutlined />}
                    loading={actionLoading.import}
                    onClick={handleImportDiscovered}
                  >
                    导入托管
                  </Button>
                ) : null}
              </Space>
            </div>

            {!isManaged ? (
              <Alert
                type="info"
                showIcon
                message="发现型项目可导入托管"
                description="导入后会根据 Docker labels 中的 working_dir 与 config_files 托管原 compose 文件；平台不会复制一份新的 compose 文件。"
              />
            ) : null}

            <div className="grid gap-3 md:grid-cols-4">
              <div className="rounded-xl bg-slate-50 px-4 py-3">
                <div className="text-xs text-slate-500">服务</div>
                <div className="mt-2 text-2xl font-semibold text-slate-900">{selectedProject.services?.length || 0}</div>
              </div>
              <div className="rounded-xl bg-slate-50 px-4 py-3">
                <div className="text-xs text-slate-500">容器</div>
                <div className="mt-2 text-2xl font-semibold text-slate-900">{containers.length}</div>
              </div>
              <div className="rounded-xl bg-slate-50 px-4 py-3">
                <div className="text-xs text-slate-500">运行中</div>
                <div className="mt-2 text-2xl font-semibold text-emerald-600">{runningCount}</div>
              </div>
              <div className="rounded-xl bg-slate-50 px-4 py-3">
                <div className="text-xs text-slate-500">更新时间</div>
                <div className="mt-2 text-sm font-medium text-slate-900">{formatAbsoluteTime(selectedSummary?.updatedAt)}</div>
              </div>
            </div>

            <div className="flex flex-wrap items-center gap-2">
              <Button type="primary" icon={<PlayCircleOutlined />} loading={actionLoading.up} disabled={!isManaged || !permissions.canUp} onClick={() => void handleRuntimeAction('up')}>
                启动
              </Button>
              <Button icon={<StopOutlined />} loading={actionLoading.down} disabled={!isManaged || !permissions.canUp} onClick={() => void handleRuntimeAction('down')}>
                停止
              </Button>
              <Button icon={<SyncOutlined />} loading={actionLoading.restart} disabled={!isManaged || !permissions.canUp} onClick={() => void handleRuntimeAction('restart')}>
                重启
              </Button>
              <Button icon={<CloudUploadOutlined />} loading={actionLoading.ps} disabled={!permissions.canValidate} onClick={handlePS}>
                PS
              </Button>
            </div>

            <div className="rounded-xl border border-slate-200 bg-[#1f2430]">
              <div className="flex items-center justify-between border-b border-white/10 px-4 py-3">
                <span className="text-sm font-medium text-slate-100">docker-compose.yaml</span>
                {!isManaged ? <Tag>只读</Tag> : null}
              </div>
              <Input.TextArea
                className="!border-0 !bg-[#1f2430] !font-mono !text-[13px] !leading-6 !text-slate-100 focus:!shadow-none"
                value={composeYaml}
                disabled={!isManaged || !permissions.canUpdate}
                autoSize={{ minRows: 18, maxRows: 34 }}
                onChange={(event) => setComposeYaml(event.target.value)}
              />
            </div>

            {preview ? (
              <div className="rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600">
                <Space wrap>
                  <Tag color={preview.validation.valid ? 'success' : 'error'}>
                    校验：{preview.validation.valid ? '通过' : '失败'}
                  </Tag>
                  <Tag color={preview.preview.safe ? 'success' : 'warning'}>
                    策略：{preview.preview.safe ? '安全' : '有风险'}
                  </Tag>
                  <span>服务：{preview.services?.join(', ') || '-'}</span>
                </Space>
              </div>
            ) : null}

            <Table<DockerContainerView>
              rowKey="id"
              columns={containerColumns}
              dataSource={containers}
              pagination={false}
              scroll={{ x: 780 }}
              locale={{ emptyText: '该项目暂无容器，执行 PS 或启动后刷新。' }}
            />
          </div>
        ) : (
          <Empty className="py-32" description="请选择或创建 Compose 项目" />
        )}
      </section>

      <Drawer
        open={createOpen}
        title="创建 Compose 项目"
        width={760}
        destroyOnClose
        onClose={() => setCreateOpen(false)}
        extra={
          <Button type="primary" loading={createSubmitting} onClick={() => form.submit()}>
            创建
          </Button>
        }
      >
        <Form<DockerComposeProjectCreateRequest>
          form={form}
          layout="vertical"
          initialValues={{
            composeYaml: DEFAULT_COMPOSE_YAML,
            writeFiles: true,
            overwriteExisting: false,
            autoUp: false,
          }}
          onFinish={handleCreate}
        >
          <Form.Item name="projectName" label="项目名" rules={[{ required: true, message: '请输入项目名' }]}>
            <Input placeholder="例如 postgres" />
          </Form.Item>
          <Form.Item name="workingDir" label="工作目录">
            <Input placeholder="可选，例如 /opt/seven/docker/postgres" />
          </Form.Item>
          <Form.Item name="description" label="描述">
            <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} />
          </Form.Item>
          <Form.Item name="composeYaml" label="docker-compose.yaml" rules={[{ required: true, message: '请输入 Compose YAML' }]}>
            <Input.TextArea className="font-mono" autoSize={{ minRows: 16, maxRows: 28 }} />
          </Form.Item>
        </Form>
      </Drawer>

      <ContainerDetailDrawer
        open={containerDetailOpen}
        loading={containerDetailLoading}
        detail={containerDetail}
        initialTab={containerDetailInitialTab}
        onRefresh={() => {
          if (containerDetail?.container) {
            void openContainerDetail(containerDetail.container, containerDetailInitialTab);
          }
        }}
        onClose={() => {
          setContainerDetailOpen(false);
          setContainerDetail(null);
          setContainerDetailInitialTab('overview');
        }}
      />
    </div>
  );
}

export default ComposeYamlWorkbench;
