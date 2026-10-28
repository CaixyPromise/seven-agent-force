import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, App, Button, Card, Checkbox, Divider, Flex, Input, Modal, Space, Steps, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { actOnWorkItem, getWorkItem, type WorkItem, type WorkPlan } from '@/api/workItemController';
import { projectFeedback } from '@/api/projectContract';
import styles from '../../projects/projects.module.css';

const statusLabel: Record<string, string> = { PLANNING: '正在生成方案', AWAITING_PLAN_APPROVAL: '等待你确认方案', IMPLEMENTING: '正在实施', NEEDS_ATTENTION: '需要处理', SUCCEEDED: '实施任务已完成', CANCELLING: '正在停止', CANCELLED: '已停止' };
const taskStatusLabel: Record<string, string> = { CREATED: '准备开始', QUEUED: '排队中', PREPARING: '准备工作区', RUNNING: '执行中', INTERACTION_SUSPENDED: '等待回答', VALIDATING: '检查中', PACKAGING: '保存结果中', UPLOADING: '上传中', RETRY_WAIT: '等待重试', REVIEWING: '审查中', MANUAL_REVIEW: '需要人工处理', SUCCEEDED: '已完成', CANCELLING: '正在取消', CANCELLED: '已取消', TIMED_OUT: '已超时', FAILED: '失败' };
type PlanGuard = { expectedVersion: string; planRevisionId: string; contentDigest: string };
export default function WorkItemPage() {
  const { workItemId = '' } = useParams(); const { message, modal } = App.useApp(); const client = useQueryClient();
  const [editing, setEditing] = useState(false); const [draft, setDraft] = useState<WorkPlan | null>(null);
  const [revising, setRevising] = useState(false); const [feedback, setFeedback] = useState('');
  const [editGuard, setEditGuard] = useState<PlanGuard | null>(null); const [revisionGuard, setRevisionGuard] = useState<PlanGuard | null>(null);
  const keys = useRef<Record<string, string>>({});
  const query = useQuery({ queryKey: ['workforce', 'work-item', workItemId], queryFn: () => getWorkItem(workItemId), enabled: Boolean(workItemId), refetchInterval: 5000 });
  const action = useMutation({ mutationFn: ({ kind, data, identity }: { kind: 'plan-decisions' | 'plan-edits' | 'plan-revisions' | 'cancel'; data: unknown; identity: string }) => {
    keys.current[identity] ??= `web-work-item-action:${crypto.randomUUID()}`;
    return actOnWorkItem(workItemId, kind, data, keys.current[identity]);
  }, onSuccess: (item, variables) => { delete keys.current[variables.identity]; client.setQueryData(['workforce', 'work-item', workItemId], item); setEditing(false); setRevising(false); setFeedback(''); message.success('操作已保存'); }, onError: (e) => { message.error(projectFeedback(e)); void query.refetch(); } });
  const item = query.data;
  if (!item) return <main className={`${styles.page} workforce-work-item-page`}><Card>{query.isError ? <Alert type="error" message="工作单暂时无法加载" action={<Button onClick={() => query.refetch()}>重试</Button>} /> : '正在加载工作单…'}</Card></main>;
  const plan = item.currentPlan; const allowed = (s: string) => item.availableActions.includes(s);
  const guard = { expectedVersion: item.version, planRevisionId: plan?.id ?? '0', contentDigest: plan?.contentDigest ?? '' };
  const current = item.status === 'PLANNING' ? 0 : item.status === 'AWAITING_PLAN_APPROVAL' ? 1 : item.status === 'IMPLEMENTING' ? 2 : item.status === 'SUCCEEDED' ? 3 : 0;
  const submit = (kind: 'plan-decisions' | 'plan-edits' | 'plan-revisions' | 'cancel', data: unknown, captured = guard) => action.mutateAsync({ kind, data, identity: `${kind}:${captured.expectedVersion}:${captured.planRevisionId}` });
  const edit = () => { if (plan) { setEditGuard(guard); setDraft(structuredClone(plan.plan)); setEditing(true); } };
  const saveEdit = () => {
    if (!draft || !editGuard) return Promise.reject(new Error('missing plan'));
    const clean = (values: string[] = []) => values.map((x) => x.trim()).filter(Boolean);
    const updated = { ...draft, goals: clean(draft.goals), nonGoals: clean(draft.nonGoals), acceptanceCriteria: clean(draft.acceptanceCriteria), risks: clean(draft.risks) };
    if (!updated.title.trim() || !updated.approach.trim() || !updated.goals.length || !updated.steps.length || !updated.acceptanceCriteria.length) { message.error('请填写方案标题、方法、目标、步骤和验收标准'); return Promise.reject(new Error('invalid plan')); }
    return submit('plan-edits', { ...editGuard, plan: updated }, editGuard);
  };
  const approve = () => modal.confirm({ title: '确认方案并开始实现？', content: '实现任务会使用当前方案与原代码基点，沿用项目已有的检查和交付授权。确认方案不会新增外部访问或 Git 权限。', okText: '确认并开始实现', onOk: () => submit('plan-decisions', { ...guard, decision: 'APPROVE' }) });
  const cancel = () => modal.confirm({ title: '停止工作单？', content: '正在执行的任务会收到停止请求，完成停止后更新状态。', okText: '停止', okButtonProps: { danger: true }, onOk: () => submit('cancel', { expectedVersion: item.version }) });
  return <main className={`${styles.page} workforce-work-item-page`}><Space orientation="vertical" size={20} style={{ width: '100%' }}>
    <Link to={`/workforce/projects/${encodeURIComponent(item.projectId)}`}>← 返回项目</Link>
    <div><Typography.Title level={2}>{item.name}</Typography.Title><Tag color={item.status === 'SUCCEEDED' ? 'green' : item.status === 'NEEDS_ATTENTION' ? 'orange' : item.status === 'CANCELLED' ? 'default' : 'blue'}>{statusLabel[item.status] ?? '状态暂不可用'}</Tag><Typography.Paragraph>{item.instruction}</Typography.Paragraph></div>
    <Card><Steps responsive current={current} status={item.status === 'SUCCEEDED' ? 'finish' : undefined} items={[{ title: '生成方案' }, { title: '人工确认' }, { title: '实施' }, { title: '检查与结果' }]} /></Card>
    {item.status === 'NEEDS_ATTENTION' && <Alert type="warning" showIcon message="执行需要处理" description={`原因：${item.reasonCode ?? '请打开执行任务查看详情'}`} />}
    <Card><Flex align="center" justify="space-between" gap={12} wrap><Typography.Title level={4} style={{ margin:0 }}>{plan ? `方案 v${plan.revision}` : '方案'}</Typography.Title><Space wrap>{allowed('EDIT_PLAN') && <Button onClick={edit}>直接编辑</Button>}{allowed('REVISE_PLAN') && <Button onClick={() => { setRevisionGuard(guard); setRevising(true); }}>提出修改</Button>}{allowed('APPROVE_PLAN') && <Button type="primary" loading={action.isPending} onClick={approve}>确认并开始实现</Button>}</Space></Flex><Divider />
      {plan ? <PlanContent item={item} /> : <Typography.Text type="secondary">方案生成后将在这里展示，确认前不会启动实现任务。</Typography.Text>}
    </Card>
    <Card title="执行任务与结果"><Space orientation="vertical" style={{ width: '100%' }}>{item.runs.map((run) => <div key={run.id}><Tag>{run.stage === 'PLAN' ? '方案' : '实施'} #{run.runNumber}</Tag><Link to={`/workforce/tasks/${encodeURIComponent(run.taskId)}`}>查看执行任务、提问、检查与结果</Link> <Typography.Text type="secondary">{taskStatusLabel[run.status] ?? '状态暂不可用'}</Typography.Text></div>)}<Typography.Text type="secondary">执行成功与 Git 交付状态分别以任务详情中的检查、Review 和交付记录为准。</Typography.Text></Space></Card>
    <Card title="消耗观察">{item.usage ? <Typography.Text>调用 {item.usage.calls ?? '—'} 次 · 已知总 Token {item.usage.knownTotalTokens ?? '—'} · 未知用量 {item.usage.unknownUsageInvocationCount ?? '—'} 次 · 费用未估算</Typography.Text> : <Typography.Text type="secondary">暂无可查看的用量信息</Typography.Text>}<div><Typography.Text type="secondary">仅记录消耗，不因费用中断执行。</Typography.Text></div></Card>
    {allowed('CANCEL') && <Button danger onClick={cancel}>停止工作单</Button>}
    <Modal title="直接编辑方案" open={editing} onCancel={() => setEditing(false)} onOk={saveEdit} okText="保存新版本" confirmLoading={action.isPending} width={760} destroyOnHidden><Typography.Paragraph type="secondary">保存后会生成新版本，不调用模型。每行填写一条目标或验收标准。</Typography.Paragraph>{draft && <PlanEditor value={draft} onChange={setDraft} />}</Modal>
    <Modal title="提出修改意见" open={revising} onCancel={() => setRevising(false)} onOk={() => revisionGuard ? submit('plan-revisions', { ...revisionGuard, feedback }, revisionGuard) : Promise.reject(new Error('missing plan version'))} okText="重新生成方案" okButtonProps={{ disabled: !feedback.trim() }} confirmLoading={action.isPending} destroyOnHidden><Typography.Paragraph type="secondary">将创建新的方案执行任务，可能产生模型调用；现有方案和历史会保留。</Typography.Paragraph><Input.TextArea value={feedback} onChange={(e) => setFeedback(e.target.value)} autoSize={{ minRows: 4, maxRows: 10 }} /></Modal>
  </Space></main>;
}
function PlanEditor({ value, onChange }: { value: WorkPlan; onChange: (plan: WorkPlan) => void }) {
  const text = (field: 'title' | 'summary' | 'approach' | 'markdown', label: string, multiline = false) => <label style={{ display:'block' }}><Typography.Text strong>{label}</Typography.Text>{multiline ? <Input.TextArea value={value[field]} onChange={(e) => onChange({ ...value, [field]:e.target.value })} autoSize={{ minRows:2,maxRows:8 }} /> : <Input value={value[field]} onChange={(e) => onChange({ ...value, [field]:e.target.value })} />}</label>;
  const lines = (field: 'goals' | 'nonGoals' | 'acceptanceCriteria' | 'risks', label: string) => <label style={{ display:'block' }}><Typography.Text strong>{label}</Typography.Text><Input.TextArea value={(value[field] ?? []).join('\n')} onChange={(e) => onChange({ ...value, [field]:e.target.value.split('\n') })} autoSize={{ minRows:2,maxRows:8 }} /></label>;
  return <Space orientation="vertical" size={14} style={{ width:'100%' }}>
    {text('title','方案标题')}{text('summary','摘要',true)}{lines('goals','目标')}{lines('nonGoals','不包含')}{text('approach','实施方法',true)}
    <section><Typography.Text strong>预计涉及区域</Typography.Text>{(value.affectedAreas ?? []).map((area, n) => <Space.Compact key={n} style={{ width:'100%', marginTop:8 }}><Input aria-label={`区域 ${n+1} 路径`} value={area.path} onChange={(e) => onChange({ ...value, affectedAreas:value.affectedAreas.map((x,i)=>i===n?{...x,path:e.target.value}:x) })} placeholder="路径" /><Input aria-label={`区域 ${n+1} 原因`} value={area.reason} onChange={(e) => onChange({ ...value, affectedAreas:value.affectedAreas.map((x,i)=>i===n?{...x,reason:e.target.value}:x) })} placeholder="原因" /><Button onClick={() => onChange({ ...value, affectedAreas:value.affectedAreas.filter((_,i)=>i!==n) })}>删除</Button></Space.Compact>)}<Button type="dashed" style={{ marginTop:8 }} onClick={() => onChange({ ...value, affectedAreas:[...(value.affectedAreas ?? []),{path:'',reason:''}] })}>添加区域</Button></section>
    <section><Typography.Text strong>实施步骤</Typography.Text>{(value.steps ?? []).map((step,n) => <Space.Compact key={step.key} style={{ width:'100%', marginTop:8 }}><Input aria-label={`步骤 ${n+1}`} value={step.description} onChange={(e) => onChange({ ...value, steps:value.steps.map((x,i)=>i===n?{...x,description:e.target.value}:x) })} /><Button onClick={() => onChange({ ...value, steps:value.steps.filter((_,i)=>i!==n) })}>删除</Button></Space.Compact>)}<Button type="dashed" style={{ marginTop:8 }} onClick={() => onChange({ ...value, steps:[...(value.steps ?? []),{key:crypto.randomUUID(),description:''}] })}>添加步骤</Button></section>
    {lines('acceptanceCriteria','验收标准')}{lines('risks','风险')}
    <section><Typography.Text strong>待决定事项</Typography.Text>{(value.openQuestions ?? []).map((question,n) => <Card key={question.key} size="small" style={{ marginTop:8 }}><Space orientation="vertical" style={{ width:'100%' }}><Input aria-label={`问题 ${n+1}`} value={question.question} onChange={(e) => onChange({ ...value, openQuestions:value.openQuestions.map((x,i)=>i===n?{...x,question:e.target.value}:x) })} placeholder="问题" /><Input.TextArea aria-label={`问题 ${n+1} 的决定`} value={question.resolution ?? ''} onChange={(e) => onChange({ ...value, openQuestions:value.openQuestions.map((x,i)=>i===n?{...x,resolution:e.target.value}:x) })} placeholder="填写决定或答案" /><Checkbox checked={question.blocking} onChange={(e) => onChange({ ...value, openQuestions:value.openQuestions.map((x,i)=>i===n?{...x,blocking:e.target.checked}:x) })}>必须解决后才能确认</Checkbox><Button onClick={() => onChange({ ...value, openQuestions:value.openQuestions.filter((_,i)=>i!==n) })}>删除问题</Button></Space></Card>)}<Button type="dashed" style={{ marginTop:8 }} onClick={() => onChange({ ...value, openQuestions:[...(value.openQuestions ?? []),{key:crypto.randomUUID(),question:'',blocking:true}] })}>添加问题</Button></section>
    {text('markdown','方案补充',true)}
  </Space>;
}
function PlanContent({ item }: { item: WorkItem }) {
  const revision = item.currentPlan!; const p = revision.plan;
  const list = (title: string, items: string[] = []) => items.length > 0 && <section><Typography.Title level={5}>{title}</Typography.Title><ul>{items.map((x, n) => <li key={`${n}-${x}`}>{x}</li>)}</ul></section>;
  return <Space orientation="vertical" style={{ width: '100%' }}><Typography.Title level={4}>{p.title}</Typography.Title><Typography.Paragraph>{p.summary}</Typography.Paragraph><Typography.Paragraph>{p.approach}</Typography.Paragraph>{list('目标', p.goals)}{list('不包含', p.nonGoals)}{list('验收标准', p.acceptanceCriteria)}{list('风险', p.risks)}
    {(p.affectedAreas ?? []).length > 0 && <section><Typography.Title level={5}>预计涉及</Typography.Title>{p.affectedAreas.map((x, n) => <div key={n}><Typography.Text code>{x.path}</Typography.Text> {x.reason}</div>)}</section>}
    <section><Typography.Title level={5}>实施步骤</Typography.Title><ol>{(p.steps ?? []).map((x) => <li key={x.key}>{x.description}</li>)}</ol></section>
    {(p.openQuestions ?? []).length > 0 && <section><Typography.Title level={5}>待决定事项</Typography.Title>{p.openQuestions.map((x) => <div key={x.key}><Tag color={x.blocking && !x.resolution ? 'orange' : 'default'}>{x.blocking && !x.resolution ? '待解决' : '已记录'}</Tag>{x.question}{x.resolution && `：${x.resolution}`}</div>)}</section>}
    {p.markdown && <section><Typography.Title level={5}>方案补充</Typography.Title><Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{p.markdown}</Typography.Paragraph></section>}
    <Typography.Text type="secondary" style={{ overflowWrap:'anywhere' }}>项目版本 {item.projectVersionId} · 代码基点 {item.source.kind === 'EMPTY_WORKSPACE' ? '空工作区' : item.source.commit || '未提供'} · 摘要 {revision.contentDigest} {revision.origin === 'USER_AUTHORED' ? '· 人工修订' : ''}</Typography.Text>
  </Space>;
}
