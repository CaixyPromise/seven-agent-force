'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Drawer, Empty, Grid, Progress, Space, Table, Tabs, Tag, Tooltip, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  CloseOutlined,
  CodeOutlined,
  FileTextOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  StopOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  updateDockerComposeProject,
  type DockerComposeProjectAction as BackendComposeProjectAction,
  type DockerComposePreviewView,
  type DockerComposeProjectDetailVO,
  type DockerComposeProjectSource,
  type DockerComposeProjectStatus,
  type DockerComposeServiceVO,
  type DockerContainerPortView,
  type DockerContainerView,
  type DockerOperationEventVO,
  type DockerOperationVO,
  type DockerPolicyViolationVO,
} from '@/api/dockerController';
import { DockerStateTag, formatBytes } from '../../components/dockerConsole';
import {
  formatComposeProjectStatusLabel,
  formatContainerStateLabel,
  formatOperationStatusLabel,
} from '../../components/dockerFormat';
import { ComposeVisualBuilder, type ComposeVisualBuilderValue } from './ComposeVisualBuilder';

export type ComposeProjectStatus = DockerComposeProjectStatus;
export type ComposeProjectAction = BackendComposeProjectAction | 'save';
export type ComposeServiceSummary = DockerComposeServiceVO;

export interface ComposeProjectSummary {
  projectId: string;
  projectName: string;
  source?: DockerComposeProjectSource;
  workingDir?: string;
  configFiles?: string[];
  composeFilePath?: string;
  fileManifest?: DockerComposeProjectDetailVO['fileManifest'];
  visualDraft?: DockerComposeProjectDetailVO['visualDraft'];
  serviceCount: number;
  containerCount: number;
  runningCount: number;
  exitedCount?: number;
  status: ComposeProjectStatus;
  warningCount?: number;
  violationCount?: number;
  safe?: boolean;
	lastOperationId?: API.Int64;
  lastOperationType?: string;
  lastOperationStatus?: string;
  lastOperationProgress?: number;
  lastOperationStage?: string;
  availableActions?: BackendComposeProjectAction[];
  activeOperation?: DockerOperationVO;
  createdAt?: string;
  updatedAt?: string;
  services?: ComposeServiceSummary[];
  containers?: DockerContainerView[];
}

interface ComposeProjectDrawerProps {
  open: boolean;
  project: ComposeProjectSummary | null;
  composeYaml: string;
  composeYamlLoading?: boolean;
  previewResult?: DockerComposePreviewView | null;
  operation?: DockerOperationVO | null;
  operationEvents?: DockerOperationEventVO[];
  psContainers?: DockerContainerView[] | null;
  actionLoading?: Partial<Record<ComposeProjectAction, boolean>>;
  onClose: () => void;
  onComposeYamlChange: (value: string) => void;
  onOpenContainerDetail?: (container: DockerContainerView) => void;
  onOpenContainerLogs?: (container: DockerContainerView) => void;
  onAction: (action: ComposeProjectAction, project: ComposeProjectSummary) => void;
  onProjectUpdated?: (detail: DockerComposeProjectDetailVO) => void;
}

function formatPort(port: DockerContainerPortView) {
  const protocol = port.type || 'tcp';
  if (port.publicPort) return `${port.publicPort}:${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
  return `${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
}

function shortId(id?: string) { return id ? id.slice(0, 12) : '-'; }

function actionColor(action?: string) { return action === 'DENY' ? 'red' : action === 'WARN' ? 'orange' : 'green'; }
function severityColor(severity?: string) {
  const normalized = (severity || '').toUpperCase();
  if (normalized === 'CRITICAL' || normalized === 'HIGH') return 'red';
  if (normalized === 'MEDIUM') return 'orange';
  if (normalized === 'LOW') return 'blue';
  return 'default';
}

function ProjectStatusTag({ status }: { status?: ComposeProjectStatus }) {
  if (status === 'running') return <Tag color="success">{formatComposeProjectStatusLabel(status)}</Tag>;
  if (status === 'degraded') return <Tag color="warning">{formatComposeProjectStatusLabel(status)}</Tag>;
  if (status === 'unknown') return <Tag>{formatComposeProjectStatusLabel(status)}</Tag>;
  return <Tag>{formatComposeProjectStatusLabel(status)}</Tag>;
}

function ProjectSourceTag({ source }: { source?: DockerComposeProjectSource }) {
  if (source === 'MANAGED') return <Tag color="blue">托管项目</Tag>;
  if (source === 'DISCOVERED') return <Tag color="gold">发现项目</Tag>;
  return <Tag>未知来源</Tag>;
}

function OperationStatusTag({ status }: { status?: string }) {
  const normalized = (status || '').toUpperCase();
  if (normalized === 'RUNNING') return <Tag color="processing">{formatOperationStatusLabel(status)}</Tag>;
  if (normalized === 'SUCCEEDED') return <Tag color="success">{formatOperationStatusLabel(status)}</Tag>;
  if (normalized === 'FAILED' || normalized === 'TIMEOUT') return <Tag color="error">{formatOperationStatusLabel(status)}</Tag>;
  if (normalized === 'CANCELLED') return <Tag color="warning">{formatOperationStatusLabel(status)}</Tag>;
  return <Tag>{formatOperationStatusLabel(status)}</Tag>;
}

function defaultProjectActions(project?: ComposeProjectSummary | null): BackendComposeProjectAction[] {
  if (!project) return [];
  const hasSpec = project.source === 'MANAGED';
  const actions: BackendComposeProjectAction[] = [];
  if (hasSpec) actions.push('preview', 'validate', 'edit');
  if (project.containerCount > 0) actions.push('logs', 'ps');
  if (project.activeOperation) return actions;
  if (!hasSpec) return actions;
  if (project.status === 'running' || project.status === 'degraded') {
    actions.push('down', 'restart');
  } else {
    actions.push('up');
  }
  return Array.from(new Set(actions));
}

function hasProjectAction(project: ComposeProjectSummary | null, action: BackendComposeProjectAction) {
  return (project?.availableActions?.length ? project.availableActions : defaultProjectActions(project)).includes(action);
}

function StatCard({ label, value, hint, tone }: { label: string; value: React.ReactNode; hint?: string; tone?: 'blue'|'green'|'orange'|'red'|'slate' }) {
  const toneClass = tone === 'green' ? 'text-emerald-600 bg-emerald-50 border-emerald-100' : tone === 'orange' ? 'text-amber-600 bg-amber-50 border-amber-100' : tone === 'red' ? 'text-red-600 bg-red-50 border-red-100' : tone === 'slate' ? 'text-slate-600 bg-slate-50 border-slate-100' : 'text-blue-600 bg-blue-50 border-blue-100';
  return <div className={`rounded-2xl border px-4 py-3 ${toneClass}`}><div className="text-xs opacity-80">{label}</div><div className="mt-2 text-2xl font-semibold leading-none">{value}</div>{hint ? <div className="mt-2 text-xs opacity-80">{hint}</div> : null}</div>;
}

function RiskCard({ item }: { item: DockerPolicyViolationVO }) {
  const danger = item.action === 'DENY';
  return (
    <div className={`rounded-2xl border px-4 py-3 ${danger ? 'border-red-100 bg-red-50/70' : 'border-amber-100 bg-amber-50/70'}`}>
      <div className="flex flex-wrap items-center gap-2">
        <Tag color={actionColor(item.action)} className="m-0">{item.action}</Tag>
        <Tag color={severityColor(item.severity)} className="m-0">{item.severity || '-'}</Tag>
        <span className="font-mono text-sm font-semibold text-slate-950">{item.code}</span>
      </div>
      <div className="mt-2 text-sm leading-6 text-slate-700">{item.message}</div>
      <div className="mt-3 grid gap-2 rounded-xl bg-white/70 p-3 text-xs text-slate-600 md:grid-cols-2">
        <div><span className="text-slate-400">field：</span><span className="font-mono break-all">{item.field || '-'}</span></div>
        <div><span className="text-slate-400">value：</span><span className="font-mono break-all">{item.value || '-'}</span></div>
        <div className="md:col-span-2"><span className="text-slate-400">建议：</span>{item.remediation || '-'}</div>
      </div>
    </div>
  );
}

function EventTimeline({ events }: { events: DockerOperationEventVO[] }) {
  if (!events.length) return <Empty description="暂无事件" />;
  return <div className="space-y-3">{events.slice(-16).map((event) => <div key={`${event.operationId}-${event.sequence}-${event.type}`} className="grid grid-cols-[92px_76px_1fr] gap-3 text-sm"><div className="text-xs text-slate-400">{event.occurredAt?.slice(11, 19) || '-'}</div><Tag className="m-0 justify-center" color={event.type === 'ERROR' ? 'red' : event.type === 'POLICY' ? 'orange' : event.type === 'RESULT' ? 'green' : 'blue'}>{event.type}</Tag><div className="min-w-0"><span className="mr-2 text-slate-500">{event.stage}</span>{event.message || '-'}</div></div>)}</div>;
}

function MiniCode({ value }: { value?: string }) {
  return <pre className="max-h-[520px] overflow-auto rounded-2xl bg-slate-950 px-4 py-3 text-xs leading-6 text-slate-100">{value || '暂无内容'}</pre>;
}

export function ComposeProjectDrawer({
  open,
  project,
  composeYaml,
  composeYamlLoading = false,
  previewResult,
  operation,
  operationEvents = [],
  psContainers,
  actionLoading = {},
  onClose,
  onComposeYamlChange,
  onOpenContainerDetail,
  onOpenContainerLogs,
  onAction,
  onProjectUpdated,
}: ComposeProjectDrawerProps) {
  const screens = Grid.useBreakpoint();
  const [builderValue, setBuilderValue] = useState<ComposeVisualBuilderValue | null>(null);
  const [savingVisual, setSavingVisual] = useState(false);
  const [activeTab, setActiveTab] = useState('services');
  const projectId = project?.projectId;
  const projectVisualDraft = project?.visualDraft;
  const projectServices = project?.services;

  useEffect(() => {
    setActiveTab('services');
  }, [projectId]);

  useEffect(() => {
    if (!projectId) return;
    const visualDraft = projectVisualDraft || { services: projectServices?.map((svc) => ({ serviceName: svc.serviceName, image: svc.image, ports: svc.ports?.map((p) => ({ hostIp: p.ip, hostPort: p.publicPort, containerPort: p.privatePort, protocol: p.type })) || [] })) || [] };
    setBuilderValue({ visualDraft, composeYaml, buildFiles: [] });
  }, [projectId, projectServices, projectVisualDraft, composeYaml]);

  const serviceRows = useMemo(() => project?.services || [], [project?.services]);
  const containerRows = useMemo(() => psContainers || project?.containers || [], [project?.containers, psContainers]);
  const risks = useMemo(() => [...(previewResult?.preview?.violations || []), ...(previewResult?.preview?.warnings || [])], [previewResult]);
  const readonly = project?.source !== 'MANAGED';

  const serviceColumns: ColumnsType<ComposeServiceSummary> = [
    { title: '服务名', dataIndex: 'serviceName', width: 150 },
    { title: '镜像', dataIndex: 'image', ellipsis: true, render: (value) => value || '-' },
    { title: '容器数', dataIndex: 'containerCount', width: 90 },
    { title: '状态', dataIndex: 'status', width: 110, render: (status) => <ProjectStatusTag status={status} /> },
    { title: '端口', width: 180, render: (_, record) => record.ports?.length ? <Space size={4} wrap>{record.ports.map((port, index) => <Tag key={index}>{formatPort(port)}</Tag>)}</Space> : '-' },
  ];

  const containerColumns: ColumnsType<DockerContainerView> = [
    { title: '容器名', dataIndex: 'name', width: 180, render: (_, record) => <button type="button" className="text-left" onClick={() => onOpenContainerDetail?.(record)}><div className="font-medium text-slate-900">{record.name}</div><div className="text-xs text-slate-400">{shortId(record.id)}</div></button> },
    { title: '镜像', dataIndex: 'image', ellipsis: true },
    { title: '状态', dataIndex: 'state', width: 120, render: (_, record) => <DockerStateTag state={record.state} label={formatContainerStateLabel(record.state)} /> },
    { title: '端口', width: 180, render: (_, record) => record.ports?.length ? <Space size={4} wrap>{record.ports.map((port, index) => <Tag key={index}>{formatPort(port)}</Tag>)}</Space> : '-' },
    { title: '操作', width: 110, fixed: 'right', render: (_, record) => <Space><Button size="small" onClick={() => onOpenContainerDetail?.(record)}>详情</Button><Button size="small" type="link" onClick={() => onOpenContainerLogs?.(record)}>日志</Button></Space> },
  ];

  const saveVisualConfig = async (value: ComposeVisualBuilderValue) => {
    if (!project?.projectId) return;
    if (project.source !== 'MANAGED') {
      message.warning('运行时发现项目暂不支持编辑，请先导入为托管项目。');
      return;
    }
    setSavingVisual(true);
    try {
      const response = await updateDockerComposeProject(project.projectId, {
        composeYaml: value.composeYaml,
        validateBeforeSave: true,
        writeFiles: true,
        buildFiles: value.buildFiles,
      });
      onComposeYamlChange(response.data.composeYaml || value.composeYaml);
      onProjectUpdated?.(response.data);
      message.success('Compose 可视化配置已保存');
    } catch (error) {
      message.error((error as Error).message || '保存 Compose 配置失败');
    } finally {
      setSavingVisual(false);
    }
  };

  const body = !project ? (
    <div className="flex h-full items-center justify-center"><Empty description="请选择 Compose 项目" /></div>
  ) : (
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-b border-slate-100 px-6 py-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <Tooltip title={project.projectName}><div className="truncate text-[20px] font-semibold text-slate-950">{project.projectName}</div></Tooltip>
              <ProjectStatusTag status={project.status} />
              <ProjectSourceTag source={project.source} />
            </div>
            <div className="mt-2 text-sm text-slate-500">项目目录：{project.workingDir || '-'} {project.composeFilePath ? ` / ${project.composeFilePath}` : ''}</div>
          </div>
          <Button type="text" icon={<CloseOutlined />} onClick={onClose} />
        </div>
        <div className="mt-5 flex flex-wrap gap-2">
          {hasProjectAction(project, 'up') ? <Button type="primary" icon={<PlayCircleOutlined />} loading={actionLoading.up} onClick={() => onAction('up', project)}>Up 启动</Button> : null}
          {hasProjectAction(project, 'down') ? <Button icon={<StopOutlined />} loading={actionLoading.down} onClick={() => onAction('down', project)}>Down 停止</Button> : null}
          {hasProjectAction(project, 'restart') ? <Button icon={<SyncOutlined />} loading={actionLoading.restart} onClick={() => onAction('restart', project)}>Restart 重启</Button> : null}
          {hasProjectAction(project, 'logs') ? <Button icon={<FileTextOutlined />} loading={actionLoading.logs} onClick={() => { setActiveTab('containers'); onAction('logs', project); }}>Logs 日志</Button> : null}
          {hasProjectAction(project, 'ps') ? <Button icon={<CodeOutlined />} loading={actionLoading.ps} onClick={() => onAction('ps', project)}>PS 容器状态</Button> : null}
          {hasProjectAction(project, 'preview') ? <Button icon={<SafetyCertificateOutlined />} loading={actionLoading.preview} onClick={() => onAction('preview', project)}>Preview</Button> : null}
          {hasProjectAction(project, 'validate') ? <Button icon={<ReloadOutlined />} loading={actionLoading.validate} onClick={() => onAction('validate', project)}>Validate</Button> : null}
        </div>
        {project.activeOperation ? <Alert className="mt-4" showIcon type="info" message={`当前有进行中的 Docker 操作：${project.activeOperation.operationType} #${project.activeOperation.operationId}`} /> : null}
        {readonly ? <Alert className="mt-4" showIcon type="warning" message="DISCOVERED 项目只支持查看运行态，不支持编辑配置或写入文件。" /> : null}
      </div>

      <Tabs
        activeKey={activeTab}
        onChange={setActiveTab}
        className="min-h-0 flex-1 overflow-hidden [&_.ant-tabs-content-holder]:min-h-0 [&_.ant-tabs-content-holder]:overflow-x-hidden [&_.ant-tabs-content-holder]:overflow-y-auto [&_.ant-tabs-content]:h-full [&_.ant-tabs-nav]:mb-4 [&_.ant-tabs-nav]:shrink-0 [&_.ant-tabs-nav]:px-6 [&_.ant-tabs-tab]:py-3 [&_.ant-tabs-tab-active_.ant-tabs-tab-btn]:font-semibold [&_.ant-tabs-tabpane]:min-h-full"
        tabBarGutter={28}
        items={[
          { key: 'services', label: '服务', children: <div className="space-y-5 px-6 pb-6"><div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><StatCard label="服务数" value={project.serviceCount} /><StatCard label="容器数" value={project.containerCount} hint={`${project.runningCount} 个运行中`} tone="green" /><StatCard label="端口映射" value={new Set(serviceRows.flatMap((service) => service.ports?.map(formatPort) || [])).size} /><StatCard label="策略风险" value={`${previewResult?.preview?.warnings?.length || project.warningCount || 0}/${previewResult?.preview?.violations?.length || project.violationCount || 0}`} tone="orange" /></div><Table rowKey="serviceName" columns={serviceColumns} dataSource={serviceRows} pagination={false} scroll={{ x: 760 }} /></div> },
          { key: 'containers', label: '容器', children: <div className="space-y-3 px-6 pb-6"><Alert showIcon type="info" message="项目日志按容器查看。选择具体容器后会打开容器详情抽屉，并使用 direct SSE 推送日志。" /><Table rowKey="id" columns={containerColumns} dataSource={containerRows} pagination={false} scroll={{ x: 880 }} /></div> },
          { key: 'config', label: '配置', children: <div className="min-w-0 px-6 pb-8">{composeYamlLoading ? <Empty description="配置加载中..." /> : builderValue ? <ComposeVisualBuilder projectName={project.projectName} workingDir={project.workingDir} value={builderValue} readonly={readonly || savingVisual} compact onChange={(value) => { setBuilderValue(value); onComposeYamlChange(value.composeYaml); }} onSave={saveVisualConfig} /> : <Empty description="暂无 Compose 配置" />}</div> },
          { key: 'risk', label: '风险', children: <div className="space-y-4 px-6 pb-6"><div className="grid grid-cols-2 gap-3 xl:grid-cols-4"><StatCard label="Safe" value={previewResult?.preview?.safe === false ? 'false' : 'true'} tone={previewResult?.preview?.safe === false ? 'red' : 'green'} /><StatCard label="Warnings" value={previewResult?.preview?.warnings?.length || 0} tone="orange" /><StatCard label="Violations" value={previewResult?.preview?.violations?.length || 0} tone="red" /><StatCard label="服务数" value={previewResult?.services?.length || project.serviceCount} /></div>{risks.length ? risks.map((risk) => <RiskCard key={`${risk.action}-${risk.code}-${risk.field}`} item={risk} />) : <Empty description="暂无风险结果，请先执行 Preview / Validate" />}</div> },
          { key: 'runtime', label: '运行态', children: <div className="space-y-5 px-6 pb-6">{operation ? <div className="rounded-2xl border border-blue-100 bg-blue-50/50 p-4"><div className="flex flex-wrap items-center justify-between gap-3"><Space><Tag color="blue">{operation.operationType}</Tag><OperationStatusTag status={operation.status} /><span className="text-sm text-slate-500">Operation ID: {operation.operationId}</span></Space><span className="text-sm text-slate-500">阶段：{operation.currentStage || '-'}</span></div><Progress className="mt-3" percent={operation.progress || 0} size="small" status={operation.status === 'FAILED' ? 'exception' : operation.status === 'SUCCEEDED' ? 'success' : 'active'} />{operation.errorSummary ? <Alert className="mt-3" type="error" showIcon message={operation.errorSummary} /> : null}</div> : <Empty description="暂无最近操作" />}<div className="rounded-2xl border border-slate-100 bg-white p-4"><div className="mb-3 font-semibold text-slate-950">事件时间线</div><EventTimeline events={operationEvents} /></div></div> },
          { key: 'files', label: '文件', children: <div className="px-6 pb-6"><Table rowKey="path" size="small" pagination={false} dataSource={project.fileManifest || []} columns={[{ title: '类型', dataIndex: 'type', render: (v) => <Tag>{v}</Tag> }, { title: '路径', dataIndex: 'path' }, { title: '服务', dataIndex: 'serviceName' }, { title: '大小', dataIndex: 'sizeBytes', render: (v) => formatBytes(v) }, { title: '更新时间', dataIndex: 'updateTime' }]} locale={{ emptyText: <Empty description="暂无文件清单" /> }} /><div className="mt-4"><div className="mb-2 font-semibold text-slate-950">当前 YAML</div><MiniCode value={composeYaml} /></div></div> },
        ]}
      />
    </div>
  );

  return (
    <Drawer
      open={open}
      width={screens.xl ? 980 : screens.md ? 760 : '100%'}
      placement="right"
      title={null}
      closable={false}
      mask={false}
      onClose={onClose}
      destroyOnHidden
      styles={{ body: { height: '100vh', padding: 0, background: '#fff', overflow: 'hidden' }, content: { height: '100vh', maxHeight: '100vh', borderTopLeftRadius: screens.md ? 18 : 0, borderBottomLeftRadius: screens.md ? 18 : 0, overflow: 'hidden', boxShadow: '0 18px 56px rgba(15,23,42,.16)' } }}
    >
      {body}
    </Drawer>
  );
}
