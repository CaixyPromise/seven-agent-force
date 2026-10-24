import {
  BranchesOutlined,
  CheckCircleOutlined,
  FileTextOutlined,
  HolderOutlined,
  LinkOutlined,
  SafetyCertificateOutlined,
  SendOutlined,
} from '@ant-design/icons';
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core';
import { SortableContext, arrayMove, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { useQuery } from '@tanstack/react-query';
import {
  Button,
  Card,
  Checkbox,
  Collapse,
  Flex,
  Form,
  Input,
  Radio,
  Select,
  Space,
  Switch,
  Tag,
  Typography,
} from 'antd';
import { useMemo, useState, type ReactNode } from 'react';
import { getDeptOptions } from '@/api/sysDeptController';
import { listNotificationGroupRoutes } from '@/api/notificationController';
import { listHookReleases, type HookRelease } from '@/api/workforceHooks';
import {
  type ProjectDraft,
  type ProjectRepositoryProvider,
  type ProjectRepositorySourceState,
  type WorkforceProject,
} from '@/api/projectContract';
import RemoteSelect from '@/components/Selectors/RemoteSelect';
import { usePermissionAccess } from '@/hooks/auth';
import styles from '../projects.module.css';

const { Text, Title } = Typography;

type ProjectFormValues = ProjectDraft;

const defaultValues: ProjectFormValues = {
  name: '',
  description: '',
  kind: 'CODEBASE',
  repository: { cloneUrl: '', defaultRefName: '' },
  techStack: [],
  agentGuidance: '',
  completionCheck: { mode: 'NONE' },
  reviewPrompt: '',
  hookReleaseIds: [],
  delivery: {
    mode: 'NONE',
    authorization: 'AUTO_AFTER_GATES',
    resultBranchPattern: 'codex/{task-id}',
  },
  notification: { enabled: false, events: ['TASK_ACTION_REQUIRED', 'TASK_COMPLETED'] },
};

export interface ProjectFormProps {
  initialProject?: WorkforceProject;
  submitting?: boolean;
  submitText?: string;
  onSubmit: (value: ProjectDraft) => void | Promise<unknown>;
  onCancel?: () => void;
}

export default function ProjectForm({ initialProject, submitting, submitText = '保存项目', onSubmit, onCancel }: ProjectFormProps) {
  const canApproveDelivery = usePermissionAccess('workforce:delivery:approve');
  const [form] = Form.useForm<ProjectFormValues>();
  const [replacingRepository, setReplacingRepository] = useState(false);
  const kind = Form.useWatch('kind', form) ?? 'CODEBASE';
  const checkMode = Form.useWatch(['completionCheck', 'mode'], form) ?? 'NONE';
  const reviewPrompt = Form.useWatch('reviewPrompt', { form, preserve: true }) ?? '';
  const hookReleaseIds = Form.useWatch('hookReleaseIds', { form, preserve: true }) ?? [];
  const deliveryMode = Form.useWatch(['delivery', 'mode'], { form, preserve: true }) ?? 'NONE';
  const notificationEnabled = Form.useWatch(['notification', 'enabled'], { form, preserve: true }) ?? false;
  const groupRoutes = useQuery({ queryKey: ['notification', 'group-routes', 'project-picker'], queryFn: listNotificationGroupRoutes });
  const hookReleases = useQuery({ queryKey: ['workforce', 'hook-releases'], queryFn: listHookReleases });
  const hookSensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
  const initialValues = useMemo(() => initialProject ? projectToDraft(initialProject) : defaultValues, [initialProject]);
  const existingRepository = initialProject?.repository;
  const showCloneUrl = !existingRepository || replacingRepository;

  const submit = async () => {
    // Unopened Collapse panels have no registered fields, but their saved
    // configuration is still part of the draft held by the form store.
    const value = normalizeDraft(form.getFieldsValue(true));
    if (value.delivery.mode !== 'NONE' && !canApproveDelivery) value.delivery.authorization = 'ASK_AFTER_GATES';
    await onSubmit(value);
    form.setFieldValue(['repository', 'cloneUrl'], undefined);
    setReplacingRepository(false);
  };

  const cancelRepositoryReplacement = () => {
    form.setFieldValue(['repository', 'cloneUrl'], undefined);
    setReplacingRepository(false);
  };

  const selectedHooks = hookReleaseIds
    .map((releaseId) => hookReleases.data?.find((release) => release.id === releaseId))
    .filter((release): release is HookRelease => Boolean(release));
  const reorderHooks = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    const from = hookReleaseIds.indexOf(String(active.id));
    const to = hookReleaseIds.indexOf(String(over.id));
    if (from >= 0 && to >= 0) form.setFieldValue('hookReleaseIds', arrayMove(hookReleaseIds, from, to));
  };

  return <Form<ProjectFormValues>
    className={styles.projectForm}
    form={form}
    layout="vertical"
    initialValues={initialValues}
    onFinish={submit}
    requiredMark="optional"
    scrollToFirstError={{ behavior: 'smooth', block: 'center' }}
  >
    <div className={styles.formGrid}>
      <div className={styles.formMain}>
        <Card className={styles.formSection}>
          <Space orientation="vertical" size={2} className={styles.sectionHeading}>
            <Title level={4}>项目资料</Title>
            <Text type="secondary">任务会自动继承这里的代码和上下文。</Text>
          </Space>
          <Form.Item name="kind" label="项目类型" rules={[{ required: true }]}>
            <Radio.Group className={styles.kindPicker} onChange={(event) => {
              if (event.target.value === 'GENERAL') form.setFieldValue(['repository', 'cloneUrl'], undefined);
            }}>
              <Radio.Button value="CODEBASE"><BranchesOutlined /> 代码项目</Radio.Button>
              <Radio.Button value="GENERAL"><FileTextOutlined /> 通用项目</Radio.Button>
            </Radio.Group>
          </Form.Item>
          <Flex gap={16} wrap>
            <Form.Item className={styles.flexField} name="name" label="项目名称" rules={[{ required: true, message: '请输入项目名称' }, { max: 100, message: '最多 100 个字符' }]}>
              <Input autoFocus placeholder="例如：聚合查询平台" />
            </Form.Item>
            <Form.Item className={styles.flexField} name="departmentId" label="所属部门">
              <RemoteSelect<string, API.SysDept>
                allowClear
                showSearch
                placeholder="选择部门"
                fetchOptions={async (search) => {
                  const result = await getDeptOptions({ keyword: search, status: 0, limit: 20 });
                  return (result.data ?? []).filter((item) => item.id && item.name).map((item) => ({ value: String(item.id), label: item.name!, data: item }));
                }}
              />
            </Form.Item>
          </Flex>
          <Form.Item name="description" label="项目说明" rules={[{ max: 500, message: '最多 500 个字符' }]}>
            <Input.TextArea rows={3} placeholder="这个项目解决什么问题" />
          </Form.Item>

          {kind === 'CODEBASE' && <div className={styles.repositoryFields}>
            {existingRepository && <div className={styles.repositorySummary}>
              <div className={styles.repositorySummaryMain}>
                <Text type="secondary">当前仓库</Text>
                <Text strong className={styles.repositoryAddress}>{existingRepository.displayUrl}</Text>
                <Flex gap={6} wrap>
                  <Tag bordered={false}>{repositoryProviderLabel(existingRepository.provider)}</Tag>
                  <Tag bordered={false} color={repositoryStateColor(existingRepository.sourceState)}>{repositoryStateLabel(existingRepository.sourceState)}</Tag>
                  {existingRepository.credentialConfigured && <Tag bordered={false} color="success">已配置凭据</Tag>}
                </Flex>
              </div>
              {!replacingRepository && <Button icon={<LinkOutlined />} onClick={() => setReplacingRepository(true)}>更换地址</Button>}
            </div>}
            {showCloneUrl && <Form.Item
              className={styles.cloneUrlField}
              name={['repository', 'cloneUrl']}
              label={existingRepository ? '新 Git 克隆地址' : 'Git 克隆地址'}
              extra="支持携带用户名和访问令牌的 HTTP(S) 地址。保存后不再显示原始地址。"
              rules={[
                { required: true, message: '请输入 Git 克隆地址' },
                { validator: validateCloneUrl },
              ]}
            >
              <Input.Password
                autoComplete="off"
                visibilityToggle
                placeholder="https://user:token@git.example.com/org/repo.git"
              />
            </Form.Item>}
            <Flex gap={12} align="end" wrap>
              <Form.Item className={styles.repositoryField} name={['repository', 'defaultRefName']} label="默认分支（可选）">
                <Input placeholder="留空则使用仓库默认分支" />
              </Form.Item>
              {existingRepository && replacingRepository && <Form.Item className={styles.replaceAction}>
                <Button onClick={cancelRepositoryReplacement}>取消更换</Button>
              </Form.Item>}
            </Flex>
          </div>}

          <Form.Item name="techStack" label="技术栈">
            <Select mode="tags" tokenSeparators={[',']} placeholder="例如：TypeScript、React、Go" options={commonTechStack.map((value) => ({ value, label: value }))} />
          </Form.Item>
          <Form.Item name="agentGuidance" label="项目指引" rules={[{ max: 8000, message: '内容过长，请精简后保存' }]}>
            <Input.TextArea rows={5} placeholder="例如：遵循现有目录结构；优先复用公共组件。" />
          </Form.Item>
        </Card>

        <Card className={styles.formSection}>
          <Space orientation="vertical" size={2} className={styles.sectionHeading}>
            <Title level={4}>自动化</Title>
            <Text type="secondary">每个任务默认执行，可在任务中补充检查。</Text>
          </Space>
          <Collapse
            className={styles.ruleCollapse}
            defaultActiveKey={['check']}
            items={[
              {
                key: 'check',
                label: <RuleLabel icon={<CheckCircleOutlined />} title="完成检查" value={checkSummary(checkMode, Form.useWatch(['completionCheck', 'command'], form))} />,
                children: <>
                  <Form.Item name={['completionCheck', 'mode']} label="检查方式">
                    <Radio.Group optionType="button" buttonStyle="solid" options={[{ label: '不额外配置', value: 'NONE' }, { label: '命令', value: 'COMMAND' }, { label: '脚本', value: 'SCRIPT' }]} />
                  </Form.Item>
                  {checkMode === 'COMMAND' && <Form.Item name={['completionCheck', 'command']} label="检查命令" rules={[{ required: true, message: '请输入检查命令' }]}><Input placeholder="make check" /></Form.Item>}
                  {checkMode === 'SCRIPT' && <Form.Item name={['completionCheck', 'script']} label="检查脚本" rules={[{ required: true, message: '请输入检查脚本' }]}><Input.TextArea className={styles.codeInput} rows={8} spellCheck={false} placeholder="#!/bin/sh" /></Form.Item>}
                  <Form.Item name={['completionCheck', 'instructions']} label="验收说明"><Input.TextArea rows={3} placeholder="需要额外确认的结果" /></Form.Item>
                </>,
              },
              {
                key: 'hooks',
                label: <RuleLabel icon={<SafetyCertificateOutlined />} title="Agent Hooks" value={selectedHooks.length ? `${selectedHooks.length} 个` : '未配置'} />,
                children: <>
                  <Form.Item name="hookReleaseIds" label="选择已发布 Hook">
                    <Select
                      mode="multiple"
                      loading={hookReleases.isLoading}
                      placeholder="添加 Hook"
                      optionFilterProp="label"
                      options={(hookReleases.data ?? []).map((release) => ({
                        value: release.id,
                        label: `${release.manifest.name} · v${release.version}`,
                      }))}
                    />
                  </Form.Item>
                  {selectedHooks.length > 0 && <DndContext sensors={hookSensors} collisionDetection={closestCenter} onDragEnd={reorderHooks}>
                    <SortableContext items={hookReleaseIds} strategy={verticalListSortingStrategy}>
                      <div className={styles.hookOrderList}>
                        {selectedHooks.map((release) => <SortableHook key={release.id} release={release} />)}
                      </div>
                    </SortableContext>
                  </DndContext>}
                </>,
              },
              {
                key: 'review',
                label: <RuleLabel icon={<SafetyCertificateOutlined />} title="独立审查提示词" value={reviewPrompt.trim() ? '已配置' : '平台默认'} />,
                children: <Form.Item
                  name="reviewPrompt"
                  label="独立审查提示词"
                  extra="启用独立审查的任务会使用这段提示词。"
                  rules={[{ max: 8000, message: '内容过长，请精简后保存' }]}
                >
                  <Input.TextArea rows={4} placeholder="例如：重点检查回归风险、权限边界和数据一致性。" />
                </Form.Item>,
              },
              ...(kind === 'CODEBASE' ? [{
                key: 'delivery',
                label: <RuleLabel icon={<BranchesOutlined />} title="Git 交付" value={deliveryLabel(deliveryMode)} />,
                children: <>
                  <Form.Item name={['delivery', 'mode']} label="交付方式">
                    <Radio.Group onChange={(event) => {
                      if (event.target.value !== 'NONE' && !canApproveDelivery) form.setFieldValue(['delivery', 'authorization'], 'ASK_AFTER_GATES');
                    }} options={[{ label: '仅保存平台结果', value: 'NONE' }, { label: '推送结果分支', value: 'NEW_BRANCH' }, { label: '结果分支 + 草稿评审', value: 'NEW_BRANCH_AND_DRAFT_REVIEW' }]} />
                  </Form.Item>
                  {deliveryMode !== 'NONE' && <>
                    <Form.Item name={['delivery', 'authorization']} label="交付时机"><Radio.Group options={[{ label: '检查和审查通过后自动交付', value: 'AUTO_AFTER_GATES', disabled: !canApproveDelivery }, { label: '完成后让我确认', value: 'ASK_AFTER_GATES' }]} /></Form.Item>
                    <Form.Item name={['delivery', 'resultBranchPattern']} label="结果分支"><Input placeholder="codex/{task-id}" /></Form.Item>
                  </>}
                </>,
              }] : []),
              {
                key: 'notification',
                label: <RuleLabel icon={<SendOutlined />} title="通知" value={notificationEnabled ? '已启用' : '未启用'} />,
                children: <>
                  <Form.Item name={['notification', 'enabled']} label="项目通知" valuePropName="checked"><Switch /></Form.Item>
                  {notificationEnabled && <>
                    <Form.Item name={['notification', 'groupRouteId']} label="通知群" rules={[{ required: true, message: '请选择通知群' }]}>
                      <Select loading={groupRoutes.isLoading} placeholder="选择通知群" options={(groupRoutes.data ?? []).filter((route) => route.status === 'ENABLED').map((route) => ({ value: route.id, label: route.name }))} />
                    </Form.Item>
                    <Form.Item name={['notification', 'events']} label="通知时机">
                      <Checkbox.Group options={[{ label: '需要处理', value: 'TASK_ACTION_REQUIRED' }, { label: '任务中断', value: 'TASK_INTERRUPTED' }, { label: '任务完成', value: 'TASK_COMPLETED' }]} />
                    </Form.Item>
                  </>}
                </>,
              },
            ]}
          />
        </Card>
      </div>

      <aside className={styles.formAside}>
        <Card className={styles.summaryCard}>
          <Text type="secondary">任务将自动使用</Text>
          <Title level={4}>{Form.useWatch('name', form)?.trim() || '当前项目'}</Title>
          <Space orientation="vertical" size={10} className={styles.summaryList}>
            <SummaryLine label="代码" value={kind === 'GENERAL' ? '空白工作区' : existingRepository?.displayUrl || '保存时验证仓库'} />
            <SummaryLine label="检查" value={checkSummary(checkMode, Form.useWatch(['completionCheck', 'command'], form))} />
            <SummaryLine label="审查" value={reviewPrompt.trim() ? '使用项目提示词' : '平台默认'} />
            <SummaryLine label="Hooks" value={selectedHooks.length ? `${selectedHooks.length} 个，按项目顺序执行` : '未配置'} />
            <SummaryLine label="交付" value={kind === 'GENERAL' ? '保存结果' : deliveryLabel(deliveryMode)} />
          </Space>
        </Card>
      </aside>
    </div>

    <Flex className={styles.formActions} justify="end" gap={10}>
      {onCancel && <Button onClick={onCancel}>取消</Button>}
      <Button type="primary" htmlType="submit" loading={submitting}>{submitText}</Button>
    </Flex>
  </Form>;
}

function RuleLabel({ icon, title, value }: { icon: ReactNode; title: string; value: string }) {
  return <Flex className={styles.ruleLabel} align="center" justify="space-between" gap={12}><Space>{icon}<Text strong>{title}</Text></Space><Tag bordered={false}>{value}</Tag></Flex>;
}

function SummaryLine({ label, value }: { label: string; value: string }) {
  return <Flex justify="space-between" gap={12}><Text type="secondary">{label}</Text><Text className={styles.summaryValue}>{value}</Text></Flex>;
}

function checkSummary(mode: ProjectFormValues['completionCheck']['mode'], command?: string): string {
  if (mode === 'COMMAND') return command?.trim() || '命令检查';
  if (mode === 'SCRIPT') return '检查脚本';
  return '项目默认';
}

function deliveryLabel(mode: ProjectFormValues['delivery']['mode']): string {
  if (mode === 'NEW_BRANCH_AND_DRAFT_REVIEW') return '分支 + 草稿评审';
  if (mode === 'NEW_BRANCH') return '结果分支';
  return '保存结果';
}

function projectToDraft(project: WorkforceProject): ProjectDraft {
  return {
    projectKey: project.projectKey,
    name: project.name,
    description: project.description,
    departmentId: project.departmentId,
    kind: project.kind,
    repository: project.repository ? { defaultRefName: project.repository.defaultRefName } : undefined,
    techStack: project.techStack,
    agentGuidance: project.agentGuidance,
    completionCheck: project.completionCheck,
    reviewPrompt: project.reviewPrompt,
    hookReleaseIds: project.hookReleaseIds,
    delivery: project.delivery,
    notification: { enabled: project.notification.enabled, events: project.notification.events, groupRouteId: project.notification.groupRouteId },
  };
}

function normalizeDraft(values: ProjectFormValues): ProjectDraft {
  const kind = values.kind;
  const delivery = kind === 'GENERAL' || values.delivery.mode === 'NONE'
    ? { mode: 'NONE' as const }
    : values.delivery;
  return {
    ...values,
    name: values.name.trim(),
    description: values.description?.trim() ?? '',
    repository: kind === 'CODEBASE' ? {
      ...(values.repository?.cloneUrl?.trim() ? { cloneUrl: values.repository.cloneUrl.trim() } : {}),
      defaultRefName: values.repository?.defaultRefName?.trim() ?? '',
    } : undefined,
    techStack: (values.techStack ?? []).map((item) => item.trim()).filter(Boolean),
    agentGuidance: values.agentGuidance?.trim() ?? '',
    completionCheck: values.completionCheck ?? { mode: 'NONE' },
    reviewPrompt: values.reviewPrompt?.trim() ?? '',
    hookReleaseIds: values.hookReleaseIds ?? [],
    delivery,
    notification: values.notification?.enabled ? values.notification : { enabled: false, events: [] },
  };
}

function SortableHook({ release }: { release: HookRelease }) {
  const sortable = useSortable({ id: release.id });
  const events = release.manifest.entrypoints.map((entry) => entry.event);
  const failureMode = release.manifest.entrypoints.some((entry) => entry.failureMode === 'BLOCK') ? '拦截失败动作' : '仅记录';
  return <div
    ref={sortable.setNodeRef}
    className={styles.hookOrderItem}
    style={{ transform: CSS.Transform.toString(sortable.transform), transition: sortable.transition }}
  >
    <button type="button" className={styles.hookDragHandle} aria-label={`调整 ${release.manifest.name} 的顺序`} {...sortable.attributes} {...sortable.listeners}>
      <HolderOutlined />
    </button>
    <div className={styles.hookOrderContent}>
      <Flex align="center" gap={8} wrap><Text strong>{release.manifest.name}</Text><Tag bordered={false}>v{release.version}</Tag><Tag bordered={false}>{failureMode}</Tag></Flex>
      <Text type="secondary">{events.map(hookEventLabel).join(' · ')}</Text>
    </div>
  </div>;
}

function hookEventLabel(event: string): string {
  if (event === 'attempt.prepare') return '启动前';
  if (event === 'attempt.propose') return '提交结果前';
  if (event === 'attempt.end') return '结束时';
  if (event === 'codex.PreToolUse') return '工具执行前';
  if (event === 'codex.PostToolUse') return '工具执行后';
  return event;
}

const commonTechStack = ['TypeScript', 'React', 'Vue', 'Node.js', 'Go', 'Python', 'Java', 'PostgreSQL', 'Redis'];

async function validateCloneUrl(_: unknown, value?: string): Promise<void> {
  if (!value?.trim()) return;
  try {
    const url = new URL(value.trim());
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.hostname || !url.pathname || url.pathname === '/') throw new Error('invalid');
  } catch {
    throw new Error('请输入完整的 HTTP(S) Git 克隆地址');
  }
}

function repositoryProviderLabel(provider: ProjectRepositoryProvider): string {
  if (provider === 'GITHUB') return 'GitHub';
  if (provider === 'GITLAB') return 'GitLab';
  if (provider === 'GITEA') return 'Gitea';
  return 'Git';
}

function repositoryStateLabel(state: ProjectRepositorySourceState): string {
  if (state === 'READY') return '代码已准备';
  if (state === 'FAILED') return '准备未完成';
  return '正在准备';
}

function repositoryStateColor(state: ProjectRepositorySourceState): 'success' | 'error' | 'processing' {
  if (state === 'READY') return 'success';
  if (state === 'FAILED') return 'error';
  return 'processing';
}
