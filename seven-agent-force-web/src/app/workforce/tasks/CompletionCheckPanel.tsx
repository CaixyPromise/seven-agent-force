import { useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { App, Button, Card, Col, Form, Input, Row, Space, Tag, Typography } from 'antd';
import { getTaskCompletionCheck, decideTaskCompletionCheck } from '@/api/completionCheckController';
import { completionCheckErrorCode, completionCheckFeedback, completionCheckFieldFeedback, completionCheckText, normalizeCompletionCheck, normalizeCompletionCheckDecision, type CompletionCheck, type CompletionCheckAction, type CompletionCheckDecision, type CompletionCheckEvidence, type CompletionCheckSummary } from '@/api/completionCheckContract';
import { getTaskInput } from '@/api/workspaceInputController';
import { usePermissionAccess } from '@/hooks/auth';
import CompletionCheckFields from './CompletionCheckFields';
import CompletionRevisionPanel from './CompletionRevisionPanel';
import { completionRefreshInterval } from './taskQueryPolicy';

const actionLabels: Record<CompletionCheckAction, string> = { CONTINUE_WITH_LIMITATIONS: '保留限制并继续', AUTHORIZE_REPAIR: '授权修复', ADD_VERIFICATION: '补充验证' };
const statusLabels = { PASSED: '检查通过', FAILED: '检查未通过', UNAVAILABLE: '无法验证', NO_TESTS: '没有可运行的测试', NOT_CONFIGURED: '未配置自动检查', TIMED_OUT: '检查超时' };
type Draft = { reason: string; repairScope?: string; completionCheck?: CompletionCheck };

export default function CompletionCheckPanel({ taskId, taskStatus }: { taskId: API.Int64; taskStatus?: string }) {
  const query = useQuery({
    queryKey: ['workforce', 'completion-check', taskId],
    queryFn: () => getTaskCompletionCheck(taskId),
    refetchInterval: (state) => completionRefreshInterval(taskStatus, state.state.data?.currentResult),
  });
  const [selection, setSelection] = useState<{ action: CompletionCheckAction; snapshot: CompletionCheckSummary }>();
  const mayContinue = usePermissionAccess('workforce:task:continue');
  if (query.isLoading) return <Card title="完成检查" loading />;
  if (!query.data) return <Card title="完成检查"><Space orientation="vertical"><CompletionRevisionPanel taskId={taskId} /><Space><Typography.Text role="status">完成检查暂时无法读取。</Typography.Text><Button loading={query.isFetching} onClick={() => void query.refetch()}>重试</Button></Space></Space></Card>;
  const summary = query.data;
  return <Card title="完成检查" data-visual-surface="completion-check-panel">
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <CompletionRevisionPanel taskId={taskId} />
      <Row gutter={[16, 12]}><Col xs={24} sm={12}><EvidenceStatus title="起始代码" evidence={summary.baseline} /></Col><Col xs={24} sm={12}><EvidenceStatus title="当前结果" evidence={summary.currentResult} /></Col></Row>
      {summary.reasonCode === 'CHECK_DEFINITION_CHANGED'
        ? <Typography.Text role="status">检查定义发生变化，请补充验证或授权修复。</Typography.Text>
        : summary.reasonCode === 'CHECK_DEFINITION_UNAVAILABLE'
          ? <Typography.Text type="secondary" role="status">检查定义来自较早版本，当前仅展示已保存的检查结果。</Typography.Text>
        : summary.baseline && !['PASSED', 'NOT_CONFIGURED'].includes(summary.baseline.status) && <Typography.Text type="secondary">起始代码的检查存在限制，请结合当前结果决定下一步。</Typography.Text>}
      {query.isError && <Typography.Text type="danger" role="alert">最新检查状态读取失败，请重试后再提交决定。</Typography.Text>}
      {mayContinue && <Space wrap>{summary.availableActions.map((action) => <Button key={action} disabled={!!selection || query.isError} onClick={() => setSelection({ action, snapshot: summary })}>{actionLabels[action]}</Button>)}</Space>}
      {selection && <CompletionDecisionForm key={taskId} taskId={taskId} action={selection.action} snapshot={selection.snapshot} current={summary} unavailable={query.isError || !mayContinue} refresh={() => query.refetch()} adopt={(snapshot) => setSelection({ ...selection, snapshot })} close={() => setSelection(undefined)} />}
    </Space>
  </Card>;
}

function EvidenceStatus({ title, evidence }: { title: string; evidence: CompletionCheckEvidence | null }) {
  return <Space orientation="vertical" size={4}><Typography.Text strong>{title}</Typography.Text><Tag color={evidence?.status === 'PASSED' ? 'success' : evidence && evidence.status !== 'NOT_CONFIGURED' ? 'warning' : 'default'}>{evidence ? statusLabels[evidence.status] : '尚无检查结果'}</Tag></Space>;
}

function CompletionDecisionForm({ taskId, action, snapshot, current, unavailable, refresh, adopt, close }: {
  taskId: API.Int64; action: CompletionCheckAction; snapshot: CompletionCheckSummary; current: CompletionCheckSummary; unavailable: boolean;
  refresh: () => Promise<{ data?: CompletionCheckSummary; isError: boolean }>; adopt: (snapshot: CompletionCheckSummary) => void; close: () => void;
}) {
  const [form] = Form.useForm<Draft>();
  const { message } = App.useApp();
  const client = useQueryClient();
  const [uploadBusy, setUploadBusy] = useState(false);
  const [refreshBusy, setRefreshBusy] = useState(false);
  const submitting = useRef(false);
  const idempotency = useRef<{ fingerprint: string; key: string } | undefined>(undefined);
  const references = useQuery({ queryKey: ['workforce', 'task-input', taskId], queryFn: () => getTaskInput(taskId), enabled: action === 'ADD_VERIFICATION' });
  const stale = current.version !== snapshot.version || current.currentResult?.id !== snapshot.currentResult?.id;
  const allowed = !unavailable && !stale && current.checkpointReady && current.availableActions.includes(action);
  const mutation = useMutation({
    mutationFn: (input: CompletionCheckDecision) => decideTaskCompletionCheck(taskId, input),
    onSuccess: async () => {
      message.success('决定已提交');
      close();
      await client.invalidateQueries({ queryKey: ['workforce'] });
    },
    onSettled: () => { submitting.current = false; },
  });
  const refreshRequired = ['COMPLETION_CHECK_STALE', 'COMPLETION_CHECK_IDEMPOTENCY_CONFLICT', 'COMPLETION_CHECK_ACTION_NOT_ALLOWED'].includes(completionCheckErrorCode(mutation.error));
  const submit = (values: Draft) => {
    if (!allowed || refreshRequired || uploadBusy || submitting.current || !snapshot.currentResult) return;
    const payload = { resultId: snapshot.currentResult.id, expectedVersion: snapshot.version, action, reason: values.reason, ...(action === 'AUTHORIZE_REPAIR' ? { repairScope: values.repairScope } : {}), ...(action === 'ADD_VERIFICATION' ? { completionCheck: values.completionCheck } : {}) };
    const fingerprint = JSON.stringify(payload);
    if (idempotency.current?.fingerprint !== fingerprint) idempotency.current = { fingerprint, key: crypto.randomUUID() };
    try {
      const input = normalizeCompletionCheckDecision({ ...payload, idempotencyKey: idempotency.current.key });
      submitting.current = true; mutation.mutate(input);
    } catch (error) { form.setFields([{ name: action === 'ADD_VERIFICATION' ? 'completionCheck' : 'reason', errors: [completionCheckFieldFeedback(error)] }]); }
  };
  const confirmLatest = async () => {
    setRefreshBusy(true);
    try { const result = await refresh(); if (!result.isError && result.data) { adopt(result.data); mutation.reset(); } }
    finally { setRefreshBusy(false); }
  };
  return <Form form={form} layout="vertical" onFinish={submit} disabled={mutation.isPending || refreshBusy} preserve>
    <Typography.Title level={5}>{actionLabels[action]}</Typography.Title>
    {action === 'CONTINUE_WITH_LIMITATIONS' && <Typography.Paragraph>继续时保留本次检查限制，后续结果仍需通过必需的验收。</Typography.Paragraph>}
    <Form.Item name="reason" label="决定原因" extra="用于记录本次决定。" rules={[{ validator: async (_, value) => { try { completionCheckText(value, 'reason', 2000, true); } catch { throw new Error('请填写决定原因，最多 2000 UTF-8 字节。'); } } }]}><Input.TextArea rows={3} autoFocus /></Form.Item>
    {action === 'AUTHORIZE_REPAIR' && <Form.Item name="repairScope" label="授权修复范围" extra="说明允许修改的内容与边界，作为本次修复要求。" rules={[{ validator: async (_, value) => { try { completionCheckText(value, 'repairScope', 20000, true); } catch { throw new Error('请明确修复范围，最多 20000 UTF-8 字节。'); } } }]}><Input.TextArea rows={4} /></Form.Item>}
    {action === 'ADD_VERIFICATION' && <Form.Item name="completionCheck" label="补充检查" rules={[{ validator: async (_, value) => { try { const input = normalizeCompletionCheck(value); if (input.mode === 'NONE') throw new Error(); } catch (error) { throw new Error(value?.mode === 'NONE' ? '请选择检查命令或脚本。' : completionCheckFieldFeedback(error)); } } }]}><CompletionCheckFields disabled={mutation.isPending || refreshBusy} references={references.data?.references} onBusyChange={setUploadBusy} /></Form.Item>}
    {(stale || unavailable || mutation.isError) && <Space wrap style={{ display: 'flex', marginBottom: 16 }}><Typography.Text type="danger" role="alert">{stale ? '检查结果已更新。请确认最新结果，已填写的内容会保留。' : mutation.isError ? completionCheckFeedback(mutation.error) : '暂时无法提交，请刷新状态后重试。'}</Typography.Text><Button loading={refreshBusy} disabled={mutation.isPending || uploadBusy} onClick={() => void confirmLatest()}>刷新并确认最新结果</Button></Space>}
    <Space wrap><Button type="primary" htmlType="submit" loading={mutation.isPending} disabled={!allowed || refreshRequired || uploadBusy || refreshBusy}>确认{actionLabels[action]}</Button><Button disabled={mutation.isPending || uploadBusy || refreshBusy} onClick={close}>取消</Button></Space>
  </Form>;
}
