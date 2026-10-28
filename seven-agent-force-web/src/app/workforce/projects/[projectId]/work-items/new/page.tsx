import { useMutation, useQuery } from '@tanstack/react-query';
import { App, Button, Card, Form, Input, Select, Space, Typography } from 'antd';
import { useEffect, useRef } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getProject, listProjectRuntimeOptions } from '@/api/projectController';
import { projectFeedback } from '@/api/projectContract';
import { listWorkforceModelOptions } from '@/api/workforceController';
import { createWorkItem } from '@/api/workItemController';
import styles from '../../../projects.module.css';

type Values = { instruction: string; name?: string; model: string; runtime: string; delivery: string };
export default function CreateWorkItemPage() {
  const { projectId = '' } = useParams();
  const navigate = useNavigate(); const { message } = App.useApp();
  const [form] = Form.useForm<Values>();
  const key = useRef(`web-work-item:${crypto.randomUUID()}`);
  const project = useQuery({ queryKey: ['workforce', 'project', projectId], queryFn: () => getProject(projectId), enabled: Boolean(projectId) });
  const models = useQuery({ queryKey: ['workforce', 'model-options'], queryFn: listWorkforceModelOptions });
  const model = Form.useWatch('model', form);
  const credential = model?.split('\u0000')[0] ?? '';
  const runtimes = useQuery({ queryKey: ['workforce', 'project-runtime-options', projectId, credential], queryFn: () => listProjectRuntimeOptions(projectId, credential), enabled: Boolean(projectId && credential) });
  useEffect(() => { if (models.data?.length === 1 && !form.getFieldValue('model')) form.setFieldValue('model', `${models.data[0].credentialId}\u0000${models.data[0].model}`); }, [models.data, form]);
  useEffect(() => { const options = runtimes.data?.items ?? []; const current = form.getFieldValue('runtime'); if (!options.some((x) => x.runtimeReleaseId === current)) form.setFieldValue('runtime', options.find((x) => x.recommended)?.runtimeReleaseId ?? options[0]?.runtimeReleaseId); }, [runtimes.data, form]);
  const create = useMutation({ mutationFn: (v: Values) => {
    const selected = models.data?.find((x) => `${x.credentialId}\u0000${x.model}` === v.model);
    if (!selected) throw new Error('请选择可用模型');
    const selection = { modelSelection: { credentialId: selected.credentialId, model: selected.model }, runtimeReleaseId: v.runtime };
    return createWorkItem(projectId, { instruction: v.instruction.trim(), ...(v.name?.trim() ? { name: v.name.trim() } : {}), planSelection: selection, implementationSelection: selection, deliveryPreference: v.delivery, usagePolicy: { mode: 'OBSERVE' } }, key.current);
  }, onSuccess: (item) => navigate(`/workforce/work-items/${encodeURIComponent(item.id)}`, { replace: true }), onError: (e) => message.error(projectFeedback(e)) });
  return <main className={`${styles.composePage} workforce-work-item-page`}><Space orientation="vertical" size={20} style={{ width: '100%' }}>
    <Button onClick={() => navigate(`/workforce/projects/${encodeURIComponent(projectId)}`)}>返回项目</Button>
    <div><Typography.Text type="secondary">{project.data?.name}</Typography.Text><Typography.Title level={2}>先出方案，再实施</Typography.Title><Typography.Paragraph type="secondary">Agent 会先调查并提交方案，只有你确认具体版本后才开始实现。</Typography.Paragraph></div>
    <Card><Form<Values> form={form} layout="vertical" initialValues={{ delivery: 'PROJECT_DEFAULT' }} onFinish={(v) => create.mutateAsync(v)}>
      <Form.Item name="instruction" label="你想完成什么？" rules={[{ required: true, whitespace: true }, { max: 20000 }]}><Input.TextArea autoFocus autoSize={{ minRows: 6, maxRows: 16 }} placeholder="描述目标、背景和期望结果" /></Form.Item>
      <Form.Item name="name" label="工作单名称（可选）"><Input maxLength={255} /></Form.Item>
      <Form.Item name="model" label="工作模型" rules={[{ required: true, message: '请选择模型' }]}><Select loading={models.isLoading} options={(models.data ?? []).map((x) => ({ value: `${x.credentialId}\u0000${x.model}`, label: x.label }))} /></Form.Item>
      <Form.Item name="runtime" label="执行环境" rules={[{ required: true, message: '请选择执行环境' }]}><Select loading={runtimes.isFetching} disabled={!credential} options={(runtimes.data?.items ?? []).map((x) => ({ value: x.runtimeReleaseId, label: x.name }))} /></Form.Item>
      <Form.Item name="delivery" label="实现后的交付"><Select options={[{ value: 'PROJECT_DEFAULT', label: '沿用项目现有授权' }, { value: 'ASK_AFTER_GATES', label: '交付前请我确认' }, { value: 'SAVE_ONLY', label: '只保存结果' }]} /></Form.Item>
      <Typography.Paragraph type="secondary">方案和实现使用上面选择的模型和环境。消耗只记录，不按费用中断任务。确认方案不会新增 Git 或网络授权。</Typography.Paragraph>
      <Button htmlType="submit" type="primary" loading={create.isPending} disabled={!project.data || project.data.status !== 'ACTIVE'}>生成方案</Button>
    </Form></Card>
  </Space></main>;
}
