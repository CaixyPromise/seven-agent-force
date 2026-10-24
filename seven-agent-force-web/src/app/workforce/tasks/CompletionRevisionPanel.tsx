import { useRef, useState } from 'react';
import { useMutation, useQuery } from '@tanstack/react-query';
import { Button, Form, Input, Radio, Select, Space, Typography } from 'antd';
import { useAuthStore } from '@/store/auth';
import { getTaskInput } from '@/api/workspaceInputController';
import { getCompletionRevisionView, requestCompletionRevision } from '@/api/completionRevisionController';
import { buildRevisionCheck, normalizeRevisionReferences, normalizeRevisionRequest, revisionFeedback, type CompletionRevision, type RevisionDraft, type RevisionView } from '@/api/completionRevisionContract';
import { completionCheckFieldFeedback } from '@/api/completionCheckContract';

export default function CompletionRevisionPanel({ taskId }: { taskId: API.Int64 }) {
  const scope = useAuthStore((s) => `${s.user?.id ?? ''}:${s.user?.primaryOrgId ?? ''}:${s.user?.authVersion ?? ''}`);
  return <RevisionWorkspace key={`${scope}:${taskId}`} taskId={taskId} scope={scope} />;
}

function RevisionWorkspace({ taskId, scope }: { taskId: API.Int64; scope: string }) {
  const query = useQuery({ queryKey: ['workforce', 'completion-revisions', scope, taskId], queryFn: () => getCompletionRevisionView(taskId), retry: false });
  const [snapshot, setSnapshot] = useState<RevisionView>();
  const [accepted, setAccepted] = useState<{ revision: CompletionRevision; viewUpdatedAt: number }>();
  const allowed = !query.isError && (!accepted || query.dataUpdatedAt > accepted.viewUpdatedAt) && !!query.data?.allowedActions.includes('REQUEST_REVISION');
  const pending = query.data?.pending;
  return <Space orientation="vertical" style={{ width: '100%' }}>
    <Space wrap>
      <Button disabled={!allowed || !!snapshot} loading={query.isLoading} aria-describedby={`check-revisions-${taskId}`} onClick={() => setSnapshot(query.data)}>调整后续检查</Button>
      <Typography.Text type="secondary" id={`check-revisions-${taskId}`} role="status">
        {pending ? '检查调整已提交，等待应用。' : query.isError ? revisionFeedback(query.error) : query.isLoading ? '正在读取检查调整状态…' : accepted ? (accepted.revision.status === 'APPLIED' ? '检查调整已应用。' : '调整请求已受理。最新状态以任务进展为准。') : !allowed ? '当前暂不能调整后续检查。' : ''}
      </Typography.Text>
      {query.isError && <Button loading={query.isFetching} onClick={() => void query.refetch()}>重试</Button>}
    </Space>
    {snapshot && <RevisionForm taskId={taskId} scope={scope} snapshot={snapshot} current={query.data} unavailable={query.isError} refresh={async () => { const result = await query.refetch(); return result.isError ? undefined : result.data; }} adopt={setSnapshot} close={() => setSnapshot(undefined)} accepted={(revision) => { setAccepted({ revision, viewUpdatedAt: query.dataUpdatedAt }); setSnapshot(undefined); void query.refetch(); }} />}
  </Space>;
}

function RevisionForm({ taskId, scope, snapshot, current, unavailable, refresh, adopt, close, accepted }: {
  taskId: API.Int64; scope: string; snapshot: RevisionView; current?: RevisionView; unavailable: boolean;
  refresh: () => Promise<RevisionView | undefined>; adopt: (view: RevisionView) => void; close: () => void; accepted: (revision: CompletionRevision) => void;
}) {
  const [form] = Form.useForm<RevisionDraft>();
  const [busy, setBusy] = useState(false);
  const [validation, setValidation] = useState('');
  const submitting = useRef(false);
  const idempotency = useRef<{ fingerprint: string; key: string } | undefined>(undefined);
  const references = useQuery({ queryKey: ['workforce', 'revision-references', scope, taskId], queryFn: async () => normalizeRevisionReferences(await getTaskInput(taskId), taskId), retry: false });
  const base = snapshot.completionCheck;
  const stale = !current || current.expectedVersion !== snapshot.expectedVersion || JSON.stringify(current.completionCheck) !== JSON.stringify(base);
  const allowed = !unavailable && !stale && !current?.pending && !!current?.allowedActions.includes('REQUEST_REVISION');
  const mode = Form.useWatch('mode', form) ?? 'NONE';
  const mutation = useMutation({ mutationFn: (input: Parameters<typeof requestCompletionRevision>[1]) => requestCompletionRevision(taskId, input), onSuccess: accepted, onSettled: () => { submitting.current = false; } });
  const submit = (draft: RevisionDraft) => {
    if (!allowed || busy || submitting.current || ((draft.referenceAssetIds?.length ?? 0) > 0 && (!references.data || references.isError))) return;
    try {
      const completionCheck = buildRevisionCheck(base, draft, references.data?.map((ref) => ref.id) ?? []);
      const payload = { expectedVersion: snapshot.expectedVersion, completionCheck };
      const fingerprint = JSON.stringify(payload);
      if (idempotency.current?.fingerprint !== fingerprint) idempotency.current = { fingerprint, key: crypto.randomUUID() };
      const input = normalizeRevisionRequest({ ...payload, idempotencyKey: idempotency.current.key });
      setValidation(''); submitting.current = true; mutation.mutate(input);
    } catch (error) { setValidation(error && typeof error === 'object' && 'field' in error && error.field === 'revision' ? revisionFeedback(error) : completionCheckFieldFeedback(error)); }
  };
  const confirmLatest = async () => {
    setBusy(true);
    try { const latest = await refresh(); if (latest) { adopt(latest); mutation.reset(); setValidation(''); } }
    finally { setBusy(false); }
  };
  return <Form form={form} layout="vertical" onFinish={submit} disabled={busy || mutation.isPending} initialValues={{ mode: 'NONE' }} preserve>
    <Typography.Title level={5}>调整后续检查</Typography.Title>
    <Typography.Paragraph>保留现有检查，补充后续需要验证的内容。提交后等待任务应用。</Typography.Paragraph>
    <details><summary>查看现有检查</summary>
      <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{base.command ?? base.script ?? '无需额外命令'}</Typography.Paragraph>
      {base.instructions && <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{base.instructions}</Typography.Paragraph>}
      <Typography.Text>保留 {base.referenceAssetIds?.length ?? 0} 份检查资料</Typography.Text>
    </details>
    {base.mode === 'NONE' && <>
      <Form.Item name="mode" label="新增检查方式"><Radio.Group options={[{ value: 'NONE', label: '无需额外命令' }, { value: 'COMMAND', label: '运行命令' }, { value: 'SCRIPT', label: '运行脚本' }]} /></Form.Item>
      {mode === 'COMMAND' && <Form.Item name="command" label="检查命令" rules={[{ required: true, message: '请填写检查命令。' }]}><Input.TextArea rows={3} /></Form.Item>}
      {mode === 'SCRIPT' && <Form.Item name="script" label="检查脚本" rules={[{ required: true, message: '请填写检查脚本。' }]}><Input.TextArea rows={6} /></Form.Item>}
    </>}
    <Form.Item name="instructions" label="追加检查说明"><Input.TextArea rows={3} autoFocus /></Form.Item>
    <Form.Item name="referenceAssetIds" label="追加任务资料" extra="仅可选择任务已有资料，原有检查资料将保留。"><Select mode="multiple" loading={references.isLoading} disabled={busy || mutation.isPending || references.isError || !references.data} options={(references.data ?? []).filter((ref) => !base.referenceAssetIds?.includes(ref.id)).map((ref) => ({ value: ref.id, label: ref.label }))} optionFilterProp="label" /></Form.Item>
    {references.isError && <Space><Typography.Text type="danger" role="alert">任务资料暂时无法读取。</Typography.Text><Button onClick={() => void references.refetch()} loading={references.isFetching}>重试资料</Button></Space>}
    {(validation || mutation.isError || stale || unavailable) && <Typography.Paragraph type="danger" role="alert">{validation || (mutation.isError ? revisionFeedback(mutation.error) : '检查状态已变化或暂不可用，请刷新并确认。已填写内容会保留。')}</Typography.Paragraph>}
    <Space wrap>
      <Button type="primary" htmlType="submit" disabled={!allowed || busy} loading={mutation.isPending}>提交检查调整</Button>
      <Button disabled={mutation.isPending} loading={busy} onClick={() => void confirmLatest()}>刷新并确认最新检查</Button>
      <Button disabled={mutation.isPending || busy} onClick={close}>取消</Button>
    </Space>
  </Form>;
}
