import {
  ArrowLeftOutlined,
  CheckCircleOutlined,
  PaperClipOutlined,
	ReloadOutlined,
  SafetyCertificateOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  App,
  Button,
  Card,
  Checkbox,
  Collapse,
  Flex,
  Form,
  Input,
  InputNumber,
  Result,
  Segmented,
  Select,
  Skeleton,
  Space,
  Switch,
  Tag,
  Typography,
  Upload,
} from 'antd';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { createProjectTask, getProject, listProjectRuntimeOptions } from '@/api/projectController';
import { projectFeedback, type ProjectRuntimeOption, type ProjectTaskCreateInput, type ProjectTaskDeliveryPreference } from '@/api/projectContract';
import { listWorkforceModelOptions } from '@/api/workforceController';
import { createReferenceAsset, type ReferenceAsset } from '@/api/workspaceInputController';
import { uploadTaskInput } from '@/app/workforce/tasks/taskInputUpload';
import { deliveryLabels, projectSourceLabel } from '../../../_components/projectPresentation';
import styles from '../../../projects.module.css';

const { Text, Title } = Typography;

type TaskFormValues = {
  instruction: string;
  name?: string;
  tags?: string[];
  reviewEnabled?: boolean;
  modelSelection?: string;
  runtimeReleaseId?: API.Int64;
  timeLimitMode?: 'UNLIMITED' | 'CUSTOM';
  timeLimitValue?: number;
  timeLimitUnit?: 'MINUTES' | 'HOURS' | 'DAYS';
  saveOnly?: boolean;
  confirmDelivery?: boolean;
  checkEnabled?: boolean;
  checkMode?: 'COMMAND' | 'SCRIPT';
  checkCommand?: string;
  checkScript?: string;
  checkInstructions?: string;
};

export default function NewProjectTaskPage() {
  const { projectId = '' } = useParams();
  const navigate = useNavigate();
  const { message } = App.useApp();
  const [form] = Form.useForm<TaskFormValues>();
  const idempotencyKey = useRef(`web-project-task:${crypto.randomUUID()}`);
  const [references, setReferences] = useState<ReferenceAsset[]>([]);
  const [uploadingReference, setUploadingReference] = useState(false);
  const project = useQuery({ queryKey: ['workforce', 'project', projectId], queryFn: () => getProject(projectId), enabled: Boolean(projectId) });
  const modelOptions = useQuery({ queryKey: ['workforce', 'model-options'], queryFn: listWorkforceModelOptions });
	const selectedModel = Form.useWatch('modelSelection', form);
	const selectedCredentialId = useMemo(() => {
		const [credentialId] = selectedModel?.split('\u0000') ?? [];
		return credentialId || '';
	}, [selectedModel]);
	const runtimeOptions = useQuery({
		queryKey: ['workforce', 'project-runtime-options', projectId, selectedCredentialId],
		queryFn: () => listProjectRuntimeOptions(projectId, selectedCredentialId),
		enabled: Boolean(projectId && selectedCredentialId),
		staleTime: 0,
		refetchOnWindowFocus: false,
	});
  const checkEnabled = Form.useWatch('checkEnabled', form) ?? false;
  const checkMode = Form.useWatch('checkMode', form) ?? 'COMMAND';
  const saveOnly = Form.useWatch('saveOnly', form) ?? false;
	const reviewEnabled = Form.useWatch('reviewEnabled', form) ?? true;
	const timeLimitMode = Form.useWatch('timeLimitMode', form) ?? 'UNLIMITED';
	const timeLimitUnit = Form.useWatch('timeLimitUnit', form) ?? 'HOURS';
	const selectedRuntimeReleaseId = Form.useWatch('runtimeReleaseId', form);
  const create = useMutation({
    mutationFn: (input: ProjectTaskCreateInput) => createProjectTask(projectId, input, idempotencyKey.current),
    onSuccess: ({ task }) => {
      message.success('任务已发起');
      navigate(`/workforce/tasks/${encodeURIComponent(task.id)}`, { replace: true, state: { from: `/workforce/projects/${encodeURIComponent(projectId)}` } });
    },
		onError: (error) => {
			message.error(projectFeedback(error));
			if (runtimeSelectionExpired(error)) {
				form.setFieldValue('runtimeReleaseId', undefined);
				void runtimeOptions.refetch();
			}
		},
  });

  useEffect(() => {
    if (modelOptions.data?.length === 1 && !form.getFieldValue('modelSelection')) {
      form.setFieldValue('modelSelection', `${modelOptions.data[0].credentialId}\u0000${modelOptions.data[0].model}`);
    }
  }, [form, modelOptions.data]);

	useEffect(() => {
		const items = runtimeOptions.data?.items ?? [];
		const current = form.getFieldValue('runtimeReleaseId');
		if (current && items.some((item) => item.runtimeReleaseId === current)) return;
		form.setFieldValue('runtimeReleaseId', items.find((item) => item.recommended)?.runtimeReleaseId ?? items[0]?.runtimeReleaseId);
	}, [form, runtimeOptions.data]);

  const submit = (values: TaskFormValues) => {
    const selected = modelOptions.data?.find((item) => `${item.credentialId}\u0000${item.model}` === values.modelSelection);
    if (!selected) {
      message.error('请选择工作模型');
      return Promise.resolve();
    }
		if (!values.runtimeReleaseId) {
			message.error('请选择执行镜像');
			return Promise.resolve();
		}
    return create.mutateAsync({
    instruction: values.instruction.trim(),
    modelSelection: { credentialId: selected.credentialId, model: selected.model },
		runtimeReleaseId: values.runtimeReleaseId,
    ...(values.timeLimitMode === 'CUSTOM' && values.timeLimitValue ? { executionTimeoutSeconds: values.timeLimitValue * timeUnitSeconds(values.timeLimitUnit) } : {}),
    ...(values.name?.trim() ? { name: values.name.trim() } : {}),
    tags: values.tags ?? [],
    referenceAssetIds: references.map((reference) => reference.id),
    additionalCompletionCheck: values.checkEnabled ? {
      mode: values.checkMode ?? 'COMMAND',
      ...(values.checkMode === 'SCRIPT' ? { script: values.checkScript?.trim() } : { command: values.checkCommand?.trim() }),
      ...(values.checkInstructions?.trim() ? { instructions: values.checkInstructions.trim() } : {}),
    } : null,
    reviewEnabled: values.reviewEnabled ?? true,
    deliveryPreference: deliveryPreference(values),
    });
  };

  const addReference = async (file: File) => {
    setUploadingReference(true);
    try {
      const fileId = await uploadTaskInput(file);
      const reference = await createReferenceAsset(fileId, file.name);
      setReferences((current) => [...current, reference]);
      message.success('资料已添加');
    } catch {
      message.error('资料上传失败，请重试');
    } finally {
      setUploadingReference(false);
    }
    return false;
  };

  const projectSummary = useMemo(() => project.data ? [
    projectSourceLabel(project.data),
    project.data.techStack.slice(0, 3).join(' · '),
    deliveryLabels[project.data.delivery.mode],
  ].filter(Boolean) : [], [project.data]);

  if (project.isLoading) return <main className={styles.composePage}><Card><Skeleton active paragraph={{ rows: 8 }} /></Card></main>;
  if (!project.data || project.isError) return <main className={styles.composePage}><Result status="warning" title="项目暂时无法加载" extra={<Button onClick={() => project.refetch()}>重试</Button>} /></main>;
  if (project.data.status === 'ARCHIVED') return <main className={styles.composePage}><Result status="info" title="项目已归档" extra={<Button onClick={() => navigate(`/workforce/projects/${encodeURIComponent(projectId)}`)}>返回项目</Button>} /></main>;

  return <main className={styles.composePage}>
    <Flex className={styles.composeHeader} align="start" gap={14}>
      <Button aria-label="返回项目" type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate(`/workforce/projects/${encodeURIComponent(projectId)}`)} />
      <Space orientation="vertical" size={6} className={styles.composeHeaderCopy}>
        <Text type="secondary">{project.data.name}</Text>
        <Title level={2}>发起任务</Title>
        <Flex gap={6} wrap>{projectSummary.map((item) => <Tag key={item} bordered={false}>{item}</Tag>)}</Flex>
      </Space>
    </Flex>

    <Form<TaskFormValues>
      form={form}
      layout="vertical"
      initialValues={{ tags: [], reviewEnabled: true, checkEnabled: false, checkMode: 'COMMAND', saveOnly: false, confirmDelivery: false, timeLimitMode: 'UNLIMITED', timeLimitUnit: 'HOURS' }}
      onFinish={submit}
      requiredMark={false}
    >
      <Card className={styles.composerCard}>
        <Form.Item name="instruction" label={<span className={styles.promptLabel}>你想完成什么？</span>} rules={[{ required: true, message: '请说明任务目标' }, { max: 20000, message: '任务目标过长，请精简后重试' }]}>
          <Input.TextArea
            className={styles.goalInput}
            autoFocus
            autoSize={{ minRows: 7, maxRows: 18 }}
            placeholder="描述目标、范围和期望结果"
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || event.key !== 'Enter' || (!event.metaKey && !event.ctrlKey)) return;
              event.preventDefault();
              form.submit();
            }}
          />
        </Form.Item>

        <Form.Item name="modelSelection" label="工作模型" rules={[{ required: true, message: '请选择工作模型' }]}>
          <Select
            loading={modelOptions.isLoading}
            placeholder={modelOptions.data?.length ? '选择本次任务使用的模型' : '请先配置可用的模型密钥'}
            options={(modelOptions.data ?? []).map((item) => ({ value: `${item.credentialId}\u0000${item.model}`, label: item.label }))}
            showSearch
            optionFilterProp="label"
          />
        </Form.Item>

		<Form.Item name="runtimeReleaseId" hidden rules={[{ required: true, message: '请选择执行镜像' }]}><Input /></Form.Item>
		<section className={styles.runtimeSection} aria-labelledby="runtime-section-title">
			<Flex align="center" justify="space-between" gap={12}>
				<div><Text id="runtime-section-title" strong>执行镜像</Text><br /><Text type="secondary">选择本次 Agent 的工具环境</Text></div>
				<Button size="small" type="text" icon={<ReloadOutlined />} loading={runtimeOptions.isFetching} disabled={!selectedCredentialId} onClick={() => runtimeOptions.refetch()}>刷新负载</Button>
			</Flex>
			{!selectedCredentialId ? <div className={styles.runtimeEmpty}>选择工作模型后显示可用镜像</div> : runtimeOptions.isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : runtimeOptions.data?.items.length ? <div className={styles.runtimeGrid} role="radiogroup" aria-label="执行镜像">
				{runtimeOptions.data.items.map((item) => <RuntimeCard key={item.runtimeReleaseId} item={item} selected={selectedRuntimeReleaseId === item.runtimeReleaseId} reviewEnabled={reviewEnabled} onSelect={() => form.setFieldValue('runtimeReleaseId', item.runtimeReleaseId)} />)}
			</div> : <div className={styles.runtimeEmpty}>{runtimeOptions.isError ? '执行镜像暂时无法加载，请刷新重试' : '当前模型协议或项目 Hook 没有可用执行镜像'}</div>}
		</section>

        <section className={styles.timeLimitSection} aria-labelledby="time-limit-title">
          <Text id="time-limit-title" strong>Agent 执行时限</Text>
          <Form.Item name="timeLimitMode" className={styles.timeLimitChoice}>
            <Segmented aria-label="Agent 执行时限" options={[{ label: '不限时', value: 'UNLIMITED' }, { label: '自定义', value: 'CUSTOM' }]} />
          </Form.Item>
          {timeLimitMode === 'CUSTOM' && <>
            <Flex gap={8} align="start" wrap>
              <Form.Item name="timeLimitValue" label="时长" rules={[{ required: true, message: '请输入执行时长' }, { type: 'number', min: 1, max: Math.floor(2147483647 / timeUnitSeconds(timeLimitUnit)), message: '请输入有效的正整数时长' }, { validator: async (_, value) => { if (value !== undefined && value !== null && !Number.isInteger(value)) throw new Error('请输入整数时长'); } }]}>
                <InputNumber min={1} max={Math.floor(2147483647 / timeUnitSeconds(timeLimitUnit))} precision={0} style={{ width: 100 }} aria-label="执行时长" />
              </Form.Item>
              <Form.Item name="timeLimitUnit" label="单位">
                <Select style={{ width: 96 }} options={[{ label: '分钟', value: 'MINUTES' }, { label: '小时', value: 'HOURS' }, { label: '天', value: 'DAYS' }]} aria-label="执行时长单位" />
              </Form.Item>
            </Flex>
            <Text type="secondary">Agent 执行和基础检查计入；排队不计，等待回答暂停后不计。</Text>
          </>}
        </section>

        <Flex className={styles.referenceRow} align="center" gap={10} wrap>
          <Upload showUploadList={false} beforeUpload={addReference} disabled={uploadingReference}>
            <Button icon={<PaperClipOutlined />} loading={uploadingReference}>添加资料</Button>
          </Upload>
          {references.map((reference) => <Tag key={reference.id} closable onClose={() => setReferences((current) => current.filter((item) => item.id !== reference.id))}>{reference.displayName}</Tag>)}
        </Flex>

        {project.data.hooks.length > 0 && <div className={styles.inheritedHooks}>
          <Text type="secondary">Agent Hooks</Text>
          <Flex gap={6} wrap>{project.data.hooks.map((hook, index) => <Tag key={hook.releaseId} bordered={false}>{index + 1}. {hook.name} v{hook.version}</Tag>)}</Flex>
        </div>}

        <Collapse className={styles.composeOptions} ghost items={[{
          key: 'options',
          label: '补充设置',
          children: <>
            <Flex gap={16} wrap>
              <Form.Item className={styles.flexField} name="name" label="任务名称" rules={[{ max: 100, message: '最多 100 个字符' }]}><Input placeholder="留空则根据目标生成" /></Form.Item>
              <Form.Item className={styles.flexField} name="tags" label="标签"><SelectTags /></Form.Item>
            </Flex>
            <Form.Item name="reviewEnabled" label={<Space><SafetyCertificateOutlined />独立审查</Space>} valuePropName="checked">
              <Switch />
            </Form.Item>
            <Form.Item name="checkEnabled" valuePropName="checked"><Checkbox><CheckCircleOutlined /> 补充完成检查</Checkbox></Form.Item>
            {checkEnabled && <div className={styles.supplementCheck}>
              <Form.Item name="checkMode" label="检查方式"><Segmented options={[{ label: '命令', value: 'COMMAND' }, { label: '脚本', value: 'SCRIPT' }]} /></Form.Item>
              {checkMode === 'COMMAND' ? <Form.Item name="checkCommand" label="检查命令" rules={[{ required: true, message: '请输入检查命令' }]}><Input placeholder="pnpm test" /></Form.Item> : <Form.Item name="checkScript" label="检查脚本" rules={[{ required: true, message: '请输入检查脚本' }]}><Input.TextArea className={styles.codeInput} rows={7} spellCheck={false} /></Form.Item>}
              <Form.Item name="checkInstructions" label="验收说明"><Input.TextArea rows={2} placeholder="说明这项检查要确认什么" /></Form.Item>
            </div>}
            {project.data.delivery.mode !== 'NONE' && <Flex className={styles.deliveryOverrides} gap={20} wrap>
              <Form.Item name="saveOnly" valuePropName="checked"><Checkbox onChange={(event) => event.target.checked && form.setFieldValue('confirmDelivery', false)}>本次只保存结果</Checkbox></Form.Item>
              <Form.Item name="confirmDelivery" valuePropName="checked"><Checkbox disabled={saveOnly} onChange={(event) => event.target.checked && form.setFieldValue('saveOnly', false)}>完成后让我确认交付</Checkbox></Form.Item>
            </Flex>}
          </>,
        }]} />

        <Flex className={styles.composerFooter} align="center" justify="space-between" gap={16} wrap>
          <Text type="secondary"><PaperClipOutlined /> 项目资料和规则已自动带入</Text>
          <Space>
            <Button onClick={() => navigate(`/workforce/projects/${encodeURIComponent(projectId)}`)}>取消</Button>
            <Button type="primary" htmlType="submit" loading={create.isPending} disabled={uploadingReference || modelOptions.isLoading || !modelOptions.data?.length || runtimeOptions.isLoading || !runtimeOptions.data?.items.length || !selectedRuntimeReleaseId}>开始任务</Button>
          </Space>
        </Flex>
      </Card>
    </Form>
  </main>;
}

function RuntimeCard({ item, selected, reviewEnabled, onSelect }: { item: ProjectRuntimeOption; selected: boolean; reviewEnabled: boolean; onSelect: () => void }) {
	const capabilities = item.environmentCapabilities.slice(0, 5).map((value) => capabilityLabel(value));
	return <div role="radio" tabIndex={0} aria-checked={selected} className={`${styles.runtimeCard} ${selected ? styles.runtimeCardSelected : ''}`} onClick={onSelect} onKeyDown={(event) => {
		if (event.key !== 'Enter' && event.key !== ' ') return;
		event.preventDefault();
		onSelect();
	}}>
		<Flex align="start" justify="space-between" gap={12}>
			<div className={styles.runtimeIdentity}><Text strong>{item.name}</Text><Text type="secondary">{item.harnessType} {item.harnessVersion} · {item.providerProtocols.join(' / ')}</Text></div>
			{item.recommended && <Tag color="blue" bordered={false}>推荐</Tag>}
		</Flex>
		<Flex gap={6} wrap className={styles.runtimeCapabilities}>{capabilities.map((label) => <Tag key={label} bordered={false}>{label}</Tag>)}</Flex>
		<Text type="secondary">支持 Agent · {item.supportsIndependentReview ? '支持独立审查' : reviewEnabled ? '独立审查将使用其他兼容镜像' : '本次未开启独立审查'} · {item.hookEvents.length} 类 Hook 事件</Text>
		<div className={styles.runtimeWorkers}>{item.workers.map((worker) => <Flex key={worker.workerId} justify="space-between" gap={12}><Text>{worker.name}</Text><Text type={worker.availableAttempts > 0 ? 'secondary' : 'warning'}>运行 {worker.runningAttempts + worker.startingAttempts + worker.reservedAttempts} · 可用 {worker.availableAttempts}</Text></Flex>)}</div>
		<Flex justify="space-between" gap={12} className={styles.runtimeFooter}><Text type="secondary">等待 {item.waitingTaskCount} 个任务</Text><Text type={item.availableSlots > 0 ? 'success' : 'warning'}>可用 {item.availableSlots} 个槽位</Text></Flex>
		<details className={styles.runtimeDetails} onClick={(event) => event.stopPropagation()} onKeyDown={(event) => event.stopPropagation()}>
			<summary>查看详情</summary>
			{item.description && <Text type="secondary">{item.description}</Text>}
			<dl>
				<div><dt>Adapter</dt><dd>{item.adapterVersion || '—'}</dd></div>
				<div><dt>镜像</dt><dd title={item.imageRef}>{item.imageRef}</dd></div>
				<div><dt>能力</dt><dd>{item.environmentCapabilities.map(capabilityLabel).join(' · ') || '—'}</dd></div>
			</dl>
		</details>
	</div>;
}

function capabilityLabel(value: string) {
	return ({ 'language.node': 'Node.js', 'language.go': 'Go', 'language.java': 'Java', 'language.python': 'Python', 'package-manager.pnpm': 'pnpm', 'package-manager.npm': 'npm', 'build-tool.maven': 'Maven', 'build-tool.gradle': 'Gradle', 'shell.posix': 'POSIX Shell', 'browser.chromium': 'Chromium' } as Record<string, string>)[value] ?? value;
}

function timeUnitSeconds(unit?: TaskFormValues['timeLimitUnit']): number {
  if (unit === 'DAYS') return 86400;
  if (unit === 'MINUTES') return 60;
  return 3600;
}

function runtimeSelectionExpired(error: unknown) {
	if (!error || typeof error !== 'object') return false;
	const value = error as { reasonCode?: unknown; details?: { reasonCode?: unknown }; payload?: { data?: { reasonCode?: unknown } } };
	const code = value.reasonCode ?? value.details?.reasonCode ?? value.payload?.data?.reasonCode;
	return code === 'TASK_RUNTIME_SELECTION_UNAVAILABLE' || code === 'TASK_RUNTIME_SELECTION_INCOMPATIBLE';
}

function SelectTags() {
  return <Select mode="tags" tokenSeparators={[',']} placeholder="例如：bugfix" options={[{ label: '功能', value: 'feature' }, { label: '修复', value: 'bugfix' }, { label: '优化', value: 'improvement' }]} />;
}

function deliveryPreference(values: TaskFormValues): ProjectTaskDeliveryPreference {
  if (values.saveOnly) return 'SAVE_ONLY';
  if (values.confirmDelivery) return 'ASK_AFTER_GATES';
  return 'PROJECT_DEFAULT';
}
