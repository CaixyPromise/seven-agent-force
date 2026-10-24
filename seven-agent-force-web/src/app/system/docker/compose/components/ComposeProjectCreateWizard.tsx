'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Checkbox, Descriptions, Drawer, Empty, Input, Result, Space, Steps, Table, Tag, message } from 'antd';
import {
  ArrowLeftOutlined,
  ArrowRightOutlined,
  CheckCircleFilled,
  FolderOpenOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import {
  checkDockerComposeWorkspace,
  createDockerComposeProject,
  getDockerComposeBuilderMetadata,
  previewDockerComposeWithFiles,
  type DockerComposeBuilderMetadataView,
  type DockerPolicyViolationVO,
  type DockerComposePreviewView,
  type DockerComposeVisualDraftView,
  type DockerComposeWorkspaceCheckView,
} from '@/api/dockerController';
import { ComposeVisualBuilder, type ComposeVisualBuilderValue } from './ComposeVisualBuilder';

export interface ComposeProjectCreateWizardProps {
  open: boolean;
  onClose: () => void;
  onCreated: (projectId: string) => void;
}

interface ProjectBaseForm {
  projectName: string;
  workingDir: string;
  description?: string;
  autoUp: boolean;
  overwriteExisting: boolean;
}

function defaultVisualDraft(): DockerComposeVisualDraftView {
  return {
    version: '3.9',
    networks: [{ name: 'default', driver: 'bridge' }],
    volumes: [],
    services: [
      {
        serviceName: 'web',
        image: 'nginx:1.25-alpine',
        restart: 'unless-stopped',
        ports: [{ hostIp: '0.0.0.0', hostPort: 80, containerPort: 80, protocol: 'tcp' }],
        networks: ['default'],
      },
    ],
  };
}

function defaultYaml() {
  return `version: '3.9'\nservices:\n  web:\n    image: nginx:1.25-alpine\n    restart: unless-stopped\n    ports:\n      - "80:80"\n    networks:\n      - default\nnetworks:\n  default:\n    driver: bridge\n`;
}

export function ComposeProjectCreateWizard({ open, onClose, onCreated }: ComposeProjectCreateWizardProps) {
  const [current, setCurrent] = useState(0);
  const [metadata, setMetadata] = useState<DockerComposeBuilderMetadataView | null>(null);
  const [baseForm, setBaseForm] = useState<ProjectBaseForm>({
    projectName: 'my-web-stack',
    workingDir: 'data/docker-compose/my-web-stack',
    description: '',
    autoUp: false,
    overwriteExisting: false,
  });
  const [workspaceCheck, setWorkspaceCheck] = useState<DockerComposeWorkspaceCheckView | null>(null);
  const [checkingWorkspace, setCheckingWorkspace] = useState(false);
  const [builderValue, setBuilderValue] = useState<ComposeVisualBuilderValue>({
    visualDraft: defaultVisualDraft(),
    composeYaml: defaultYaml(),
    buildFiles: [],
  });
  const [preview, setPreview] = useState<DockerComposePreviewView | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [creating, setCreating] = useState(false);
  const [createdProjectId, setCreatedProjectId] = useState('');

  useEffect(() => {
    if (!open) return;
    void getDockerComposeBuilderMetadata()
      .then((res) => {
        setMetadata(res.data);
        setBaseForm((prev) => ({
          ...prev,
          workingDir: prev.workingDir || `${res.data.defaultWorkspaceRoot || res.data.workspaceRoots?.[0] || 'data/docker-compose'}/${prev.projectName}`,
        }));
      })
      .catch(() => undefined);
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const root = metadata?.defaultWorkspaceRoot || metadata?.workspaceRoots?.[0] || 'data/docker-compose';
    setBaseForm((prev) => ({ ...prev, workingDir: `${root}/${prev.projectName || 'my-web-stack'}` }));
  }, [baseForm.projectName, metadata?.defaultWorkspaceRoot, metadata?.workspaceRoots, open]);

  const canNextBase = baseForm.projectName.trim() && baseForm.workingDir.trim();

  const runWorkspaceCheck = async () => {
    if (!baseForm.workingDir.trim()) {
      message.warning('请先填写项目目录');
      return false;
    }
    setCheckingWorkspace(true);
    try {
      const response = await checkDockerComposeWorkspace({
        workingDir: baseForm.workingDir,
        createIfMissing: true,
        overwriteExistingCompose: baseForm.overwriteExisting,
      });
      setWorkspaceCheck(response.data);
      if (!response.data.valid) {
        message.error(response.data.message || '项目目录检查未通过');
        return false;
      }
      message.success('项目目录检查通过');
      return true;
    } catch (error) {
      message.error((error as Error).message || '项目目录检查失败');
      return false;
    } finally {
      setCheckingWorkspace(false);
    }
  };

  const runPreview = async () => {
    setPreviewing(true);
    try {
      const response = await previewDockerComposeWithFiles({
        projectName: baseForm.projectName,
        workingDir: baseForm.workingDir,
        composeYaml: builderValue.composeYaml,
        buildFiles: builderValue.buildFiles,
      });
      setPreview(response.data);
      message[response.data.validation.valid ? 'success' : 'warning'](
        response.data.validation.valid ? 'Preview 完成' : response.data.validation.message || 'Compose 校验未通过',
      );
      return response.data.validation.valid;
    } catch (error) {
      message.error((error as Error).message || 'Preview 失败');
      return false;
    } finally {
      setPreviewing(false);
    }
  };

  const createProject = async () => {
    setCreating(true);
    try {
      const checked = workspaceCheck?.valid || (await runWorkspaceCheck());
      if (!checked) return;
      const response = await createDockerComposeProject({
        projectName: baseForm.projectName,
        workingDir: baseForm.workingDir,
        description: baseForm.description,
        composeYaml: builderValue.composeYaml,
        writeFiles: true,
        overwriteExisting: baseForm.overwriteExisting,
        autoUp: baseForm.autoUp,
        buildFiles: builderValue.buildFiles,
      });
      setCreatedProjectId(response.data.projectId);
      setCurrent(4);
      message.success(response.data.operationId ? `项目已创建，Up 操作已提交 #${response.data.operationId}` : 'Compose 项目创建成功');
    } catch (error) {
      message.error((error as Error).message || '创建 Compose 项目失败');
    } finally {
      setCreating(false);
    }
  };

  const resetAndClose = () => {
    onClose();
    window.setTimeout(() => {
      setCurrent(0);
      setPreview(null);
      setCreatedProjectId('');
      setWorkspaceCheck(null);
    }, 200);
  };

  const previewRisks = useMemo(() => ({
    warnings: preview?.preview?.warnings?.length || 0,
    violations: preview?.preview?.violations?.length || 0,
    services: preview?.services?.length || builderValue.visualDraft.services.length,
  }), [builderValue.visualDraft.services.length, preview]);

  return (
    <Drawer
      open={open}
      width="96vw"
      title="新建 Compose 项目"
      onClose={resetAndClose}
      destroyOnHidden
      styles={{ body: { background: '#f6f8fc', padding: 20 } }}
    >
      <div className="space-y-4">
        <div className="rounded-2xl border border-slate-100 bg-white px-6 py-5">
          <Steps
            current={current}
            items={[
              { title: '基本信息', description: '项目名与工作目录' },
              { title: '服务配置', description: '可视化 / YAML' },
              { title: 'Preview & 风险检查', description: '服务清单与策略结果' },
              { title: '确认创建', description: '写入文件并创建项目' },
              { title: '创建成功', description: '进入项目详情' },
            ]}
          />
        </div>

        {current === 0 ? (
          <div className="rounded-2xl border border-slate-100 bg-white p-6">
            <div className="mb-5 text-lg font-semibold text-slate-950">基本信息</div>
            <div className="grid gap-5 lg:grid-cols-2">
              <div>
                <div className="mb-2 text-sm font-medium text-slate-700">项目名称</div>
                <Input value={baseForm.projectName} onChange={(e) => setBaseForm((prev) => ({ ...prev, projectName: e.target.value }))} />
              </div>
              <div>
                <div className="mb-2 text-sm font-medium text-slate-700">项目目录</div>
                <Input
                  value={baseForm.workingDir}
                  suffix={<FolderOpenOutlined />}
                  onChange={(e) => setBaseForm((prev) => ({ ...prev, workingDir: e.target.value }))}
                  onBlur={() => void runWorkspaceCheck()}
                />
              </div>
              <div className="lg:col-span-2">
                <div className="mb-2 text-sm font-medium text-slate-700">描述</div>
                <Input.TextArea rows={4} value={baseForm.description} onChange={(e) => setBaseForm((prev) => ({ ...prev, description: e.target.value }))} />
              </div>
              <div className="lg:col-span-2 flex flex-wrap gap-5">
                <Checkbox checked={baseForm.autoUp} onChange={(e) => setBaseForm((prev) => ({ ...prev, autoUp: e.target.checked }))}>创建后立即执行 Compose Up</Checkbox>
                <Checkbox checked={baseForm.overwriteExisting} onChange={(e) => setBaseForm((prev) => ({ ...prev, overwriteExisting: e.target.checked }))}>允许覆盖已存在 compose 文件</Checkbox>
              </div>
            </div>
            {workspaceCheck ? (
              <Alert
                className="mt-5"
                showIcon
                type={workspaceCheck.valid ? 'success' : 'error'}
                message={workspaceCheck.valid ? '项目目录可用' : '项目目录不可用'}
                description={workspaceCheck.message || `resolved: ${workspaceCheck.resolvedPath || '-'}`}
              />
            ) : null}
            <div className="mt-6 flex justify-end">
              <Space>
                <Button loading={checkingWorkspace} onClick={() => void runWorkspaceCheck()}>检查目录</Button>
                <Button type="primary" disabled={!canNextBase} onClick={async () => { const ok = await runWorkspaceCheck(); if (ok) setCurrent(1); }}>下一步</Button>
              </Space>
            </div>
          </div>
        ) : null}

        {current === 1 ? (
          <div className="rounded-2xl border border-slate-100 bg-white p-5">
            <ComposeVisualBuilder
              projectName={baseForm.projectName}
              workingDir={baseForm.workingDir}
              metadata={metadata}
              value={builderValue}
              onChange={setBuilderValue}
            />
            <div className="mt-5 flex justify-end gap-3">
              <Button icon={<ArrowLeftOutlined />} onClick={() => setCurrent(0)}>上一步</Button>
              <Button type="primary" icon={<ArrowRightOutlined />} onClick={async () => { const ok = await runPreview(); if (ok || preview) setCurrent(2); }}>Preview & 风险检查</Button>
            </div>
          </div>
        ) : null}

        {current === 2 ? (
          <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_420px]">
            <div className="rounded-2xl border border-slate-100 bg-white p-6">
              <div className="mb-4 flex items-center justify-between">
                <div className="text-lg font-semibold text-slate-950">Preview & 风险检查</div>
                <Button icon={<SafetyCertificateOutlined />} loading={previewing} onClick={() => void runPreview()}>重新检查</Button>
              </div>
              {preview ? (
                <div className="space-y-4">
                  <Alert showIcon type={preview.validation.valid ? 'success' : 'error'} message={preview.validation.valid ? 'Compose 校验通过' : 'Compose 校验失败'} description={preview.validation.message} />
                  <div className="grid gap-3 md:grid-cols-3">
                    <div className="rounded-2xl bg-blue-50 p-4 text-blue-600"><div className="text-2xl font-semibold">{previewRisks.services}</div><div>服务</div></div>
                    <div className="rounded-2xl bg-amber-50 p-4 text-amber-600"><div className="text-2xl font-semibold">{previewRisks.warnings}</div><div>Warnings</div></div>
                    <div className="rounded-2xl bg-red-50 p-4 text-red-600"><div className="text-2xl font-semibold">{previewRisks.violations}</div><div>Violations</div></div>
                  </div>
                  <Table
                    size="small"
                    rowKey="code"
                    pagination={false}
                    dataSource={[...(preview.preview?.violations || []), ...(preview.preview?.warnings || [])]}
                    columns={[
                      {
                        title: '级别',
                        render: (_, row: DockerPolicyViolationVO) => (
                          <Tag color={row.action === 'DENY' ? 'red' : 'orange'}>
                            {row.action}
                          </Tag>
                        ),
                      },
                      { title: 'Code', dataIndex: 'code' },
                      { title: '字段', dataIndex: 'field' },
                      { title: '建议', dataIndex: 'remediation' },
                    ]}
                    locale={{ emptyText: <Empty description="暂无风险" /> }}
                  />
                </div>
              ) : <Empty description="请先执行 Preview" />}
              <div className="mt-5 flex justify-end gap-3">
                <Button icon={<ArrowLeftOutlined />} onClick={() => setCurrent(1)}>上一步</Button>
                <Button type="primary" onClick={() => setCurrent(3)}>下一步</Button>
              </div>
            </div>
            <div className="rounded-2xl border border-slate-100 bg-white p-5">
              <div className="mb-3 font-semibold text-slate-950">Normalized YAML</div>
              <pre className="max-h-[620px] overflow-auto rounded-2xl bg-slate-950 p-4 text-xs leading-6 text-slate-100">{preview?.normalizedYaml || preview?.validation.normalizedYaml || builderValue.composeYaml}</pre>
            </div>
          </div>
        ) : null}

        {current === 3 ? (
          <div className="rounded-2xl border border-slate-100 bg-white p-6">
            <div className="mb-5 text-lg font-semibold text-slate-950">确认创建</div>
            <Descriptions bordered column={2} size="small">
              <Descriptions.Item label="项目名称">{baseForm.projectName}</Descriptions.Item>
              <Descriptions.Item label="项目目录">{baseForm.workingDir}</Descriptions.Item>
              <Descriptions.Item label="服务数">{builderValue.visualDraft.services.length}</Descriptions.Item>
              <Descriptions.Item label="Dockerfile 文件">{builderValue.buildFiles.length}</Descriptions.Item>
              <Descriptions.Item label="写入文件">是</Descriptions.Item>
              <Descriptions.Item label="创建后 Up">{baseForm.autoUp ? '是' : '否'}</Descriptions.Item>
            </Descriptions>
            <Alert className="mt-5" showIcon type="info" message="创建项目会写入 docker-compose.yaml，并按 Dockerfile 构建服务写入 Dockerfile / extraFiles。" />
            <div className="mt-6 flex justify-end gap-3">
              <Button icon={<ArrowLeftOutlined />} onClick={() => setCurrent(2)}>上一步</Button>
              <Button type="primary" loading={creating} onClick={() => void createProject()}>创建项目</Button>
            </div>
          </div>
        ) : null}

        {current === 4 ? (
          <div className="rounded-2xl border border-slate-100 bg-white p-6">
            <Result
              status="success"
              icon={<CheckCircleFilled className="text-green-500" />}
              title="Compose 项目创建成功"
              subTitle={`项目 ${baseForm.projectName} 已创建${baseForm.autoUp ? '，并已提交启动操作' : ''}。`}
              extra={[
                <Button key="detail" type="primary" onClick={() => onCreated(createdProjectId)}>进入项目详情</Button>,
                <Button key="list" onClick={resetAndClose}>返回项目列表</Button>,
              ]}
            />
          </div>
        ) : null}
      </div>
    </Drawer>
  );
}
