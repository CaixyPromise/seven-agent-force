import { CheckCircleFilled, ClockCircleOutlined, SafetyCertificateOutlined, StopOutlined, ToolOutlined } from '@ant-design/icons';
import VirtualList from '@rc-component/virtual-list';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, App, Button, Card, Col, Descriptions, Empty, Flex, Form, Input, Modal, Row, Space, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';
import { decideIndependentReview, getIndependentReview, getIndependentReviewRun, listIndependentReviewRuns, type IndependentReviewReport, type ReviewDecisionAction, type ReviewRunView } from '@/api/independentReviewController';
import { reviewErrorCode, reviewFeedback } from './reviewFeedback';
import './independentReview.css';
import { reviewRefreshInterval } from '../tasks/taskQueryPolicy';

const { Text, Title, Paragraph } = Typography;
const decisionCopy: Record<ReviewDecisionAction, { title: string; submit: string; tone?: 'danger' }> = {
  AUTHORIZE_ONE_REVISION: { title: '授权一次修订', submit: '验证并授权' },
  ACCEPT_RISK: { title: '接受当前结果', submit: '验证并接受风险' },
  CANCEL_TASK: { title: '取消任务', submit: '验证并取消', tone: 'danger' },
};

export default function IndependentReviewWorkspace({ taskId, taskStatus, compact = false }: { taskId: API.Int64; taskStatus?: string; compact?: boolean }) {
  const report = useQuery({
    queryKey: ['workforce', 'independent-review', taskId],
    queryFn: () => getIndependentReview(taskId),
    refetchInterval: (state) => reviewRefreshInterval(taskStatus, state.state.data?.status, state.state.data?.required),
  });
  const runs = useInfiniteQuery({
    // A task can create its first ReviewRun after this page has mounted, and
    // automatic revision can create another one without a route change.  The
    // first implementation only loaded the history once, so an initially
    // empty page stayed empty even while the explicit review summary had
    // advanced.  Key the read by the authoritative summary facts so the
    // history is refreshed when a new cycle or terminal decision appears.
    queryKey: ['workforce', 'independent-review-runs', taskId, report.data?.status, report.data?.reportDigest, report.data?.automaticRevisionCount, report.data?.noProgressStreak],
    queryFn: ({ pageParam }) => listIndependentReviewRuns(taskId, pageParam), initialPageParam: undefined as API.Int64 | undefined,
    getNextPageParam: (page) => page.nextCursor || undefined,
  });
  const items = runs.data?.pages.flatMap((page) => page.items) ?? [];
  if (report.isLoading) return <Card className="review-workspace"><Flex align="center" gap={10}><ClockCircleOutlined spin /><Text>正在读取独立审查状态…</Text></Flex></Card>;
  if (report.isError) return <Alert type="warning" showIcon title="独立审查状态暂时无法加载" description="任务其他信息仍可查看。" action={<Button onClick={() => void report.refetch()}>重试</Button>} />;
  if (!report.data?.required) return compact ? <Card title="独立审查"><Text>本任务未启用独立审查。</Text></Card> : null;
  if (report.data.status === 'PENDING_FINALIZATION') return <PendingFinalizationNotice />;
  return <section className="review-workspace">
    <Flex className="review-workspace-heading" justify="space-between" align="flex-end" wrap gap={12}>
      <div><Title level={3}><SafetyCertificateOutlined /> 独立审查</Title><Text type="secondary">审查结论、修订过程和人工决定均保留为独立事实</Text></div>
      <ReviewStateTag report={report.data} />
    </Flex>
    <Row gutter={[20, 20]} align="top" data-visual-surface="independent-review-task">
      <Col xs={24} lg={compact ? 24 : 16}><ReviewEvidence report={report.data} runs={items} onLoadMore={() => void runs.fetchNextPage()} hasMore={runs.hasNextPage} loadingMore={runs.isFetchingNextPage} /></Col>
      <Col xs={24} lg={compact ? 24 : 8}><ReviewActions taskId={taskId} report={report.data} currentRun={items[0]} /></Col>
    </Row>
  </section>;
}

function PendingFinalizationNotice() {
  return <Card className="review-workspace" title="独立审查"><Text>独立审查尚未开始。</Text></Card>;
}

function ReviewStateTag({ report }: { report: IndependentReviewReport }) {
  const meta = report.qualityOutcome === 'HUMAN_ACCEPTED' ? ['人工接受当前风险', 'gold'] : report.verdict === 'PASS' ? ['AI 已通过', 'green'] : report.stopReason === 'INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS' ? ['无实质进展', 'orange'] : report.stopReason === 'INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED' ? ['修订额度耗尽', 'orange'] : report.status === 'MANUAL_REVIEW' ? ['需要人工决定', 'orange'] : report.status === 'REVISION_REQUIRED' ? ['自动修订中', 'blue'] : report.verdict === 'REJECT' ? ['AI 拒绝', 'red'] : report.verdict === 'REVISE' ? ['AI 要求修订', 'blue'] : report.status === 'CANCELLED' ? ['已取消', 'default'] : ['独立审查中', 'processing'];
  return <Tag color={meta[1]} className="review-state-tag">{meta[0]}</Tag>;
}

function ReviewEvidence({ report, runs, onLoadMore, hasMore, loadingMore }: { report: IndependentReviewReport; runs: ReviewRunView[]; onLoadMore: () => void; hasMore?: boolean; loadingMore?: boolean }) {
  const [evidenceView, setEvidenceView] = useState<'findings' | 'tests' | 'limitations'>('findings');
  const [detailRunId, setDetailRunId] = useState<API.Int64>();
  const detail = useQuery({ queryKey: ['workforce', 'independent-review-run', detailRunId], queryFn: () => getIndependentReviewRun(detailRunId!), enabled: Boolean(detailRunId) });
  const verdictLabel = report.verdict === 'PASS' ? '通过' : report.verdict === 'REJECT' ? '拒绝' : report.verdict === 'REVISE' ? '需要修订' : noTrustedReportLabel(report.stopReason);
  return <Card className="review-main-card">
    <div className="review-facts" aria-label="独立审查摘要">
      <ReviewFact label="AI 结论" value={verdictLabel} tone={report.verdict === 'PASS' ? 'success' : report.verdict === 'REJECT' ? 'danger' : 'warning'} />
      <ReviewFact label="质量结论" value={report.qualityOutcome === 'HUMAN_ACCEPTED' ? '人工接受' : report.qualityOutcome === 'INDEPENDENT_REVIEW_PASSED' ? 'AI 已通过' : '尚未关闭'} />
      <ReviewFact label="自动修订" value={`${report.automaticRevisionCount} / ${report.automaticRevisionLimit}`} />
      <ReviewFact label="停止原因" value={report.stopReason ? stopReason(report.stopReason) : '—'} />
    </div>
    <Paragraph className="review-summary">{report.summary || '当前没有可展示的可信报告摘要。'}</Paragraph>
    <div className="review-section-tabs" role="tablist" aria-label="审查证据">
      <button type="button" role="tab" aria-selected={evidenceView === 'findings'} className={evidenceView === 'findings' ? 'is-active' : ''} onClick={() => setEvidenceView('findings')}>问题 {(report.findings ?? []).length}</button>
      <button type="button" role="tab" aria-selected={evidenceView === 'tests'} className={evidenceView === 'tests' ? 'is-active' : ''} onClick={() => setEvidenceView('tests')}>验证 {(report.reviewTests ?? []).length}</button>
      <button type="button" role="tab" aria-selected={evidenceView === 'limitations'} className={evidenceView === 'limitations' ? 'is-active' : ''} onClick={() => setEvidenceView('limitations')}>限制 {(report.limitations ?? []).length}</button>
      <button type="button" onClick={() => document.getElementById(`review-history-${report.reportDigest || 'current'}`)?.scrollIntoView({ block: 'nearest' })}>审查历史</button>
    </div>
    {evidenceView === 'findings' && <section className="review-section"><Flex justify="space-between" align="center"><Title level={4}>发现的问题</Title><Text type="secondary">{(report.findings ?? []).length} 项</Text></Flex>
      {(report.findings ?? []).length ? <div className="review-finding-list">{(report.findings ?? []).map((finding) => <article key={finding.id || `${finding.category}:${finding.evidence?.path}:${finding.evidence?.line}`} className={`review-finding is-${finding.severity.toLowerCase()}`}><Flex justify="space-between" gap={12} wrap><Space wrap><Tag color={finding.severity === 'BLOCKING' ? 'red' : 'orange'}>{finding.severity}</Tag><Tag>{finding.category}</Tag></Space>{finding.evidence?.path && <Text code>{finding.evidence.path}{finding.evidence.line ? `:${finding.evidence.line}` : ''}</Text>}</Flex><Title level={5}>{finding.title}</Title><Paragraph>{finding.message}</Paragraph>{finding.reproduction && <Text type="secondary" className="review-finding-detail">复现：{finding.reproduction}</Text>}{finding.preconditions && <Text type="secondary" className="review-finding-detail">前置条件：{finding.preconditions}</Text>}{finding.acceptanceChecks?.length ? <Text type="secondary" className="review-finding-detail">验收检查：{finding.acceptanceChecks.join('、')}</Text> : null}{finding.suggestedDirection && <Text type="secondary" className="review-finding-detail">建议：{finding.suggestedDirection}</Text>}</article>)}</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有需要展示的问题" />}
    </section>}
    {evidenceView === 'tests' && <section className="review-section"><EvidenceList title="审查测试" empty="没有审查测试摘要" items={(report.reviewTests ?? []).map((test, index) => <div className="review-compact-evidence" key={`${test.command}:${index}`}><Tag color={test.status === 'PASS' || test.status === 'PASSED' ? 'green' : 'orange'}>{test.status}</Tag><Text code>{test.command || '受限测试'}</Text><Text type="secondary">{test.summary}</Text></div>)} /></section>}
    {evidenceView === 'limitations' && <section className="review-section"><EvidenceList title="复现限制" empty="没有复现限制" items={(report.limitations ?? []).map((item, index) => <Alert key={`${item.code}:${index}`} type="warning" showIcon title={item.code || '限制'} description={item.summary} />)} /></section>}
    <section className="review-history" id={`review-history-${report.reportDigest || 'current'}`}><Flex justify="space-between" align="center"><div><Title level={4}>审查周期</Title><Text type="secondary">按时间倒序，共显示已加载的周期</Text></div>{hasMore && <Button type="link" loading={loadingMore} onClick={onLoadMore}>加载更早周期</Button>}</Flex>{runs.length ? <VirtualList className="review-cycle-list" data={runs} height={390} itemHeight={122} itemKey={(run) => run.id}>{(run) => <button type="button" className="review-cycle review-cycle-button" onClick={() => setDetailRunId(run.id)}><Flex justify="space-between" align="center"><Text strong>第 {run.automaticRevisionNumber + 1} 轮</Text>{run.verdict === 'PASS' ? <CheckCircleFilled className="is-success" /> : run.status === 'CANCELLED' ? <StopOutlined /> : <ToolOutlined />}</Flex><Flex justify="space-between" align="center" gap={8}><Tag color={run.verdict === 'PASS' ? 'green' : run.verdict === 'REJECT' ? 'red' : 'orange'}>{run.verdict || run.status}</Tag><Text type="secondary" ellipsis>{run.resultTree}</Text></Flex><Text type="secondary">{run.humanDecision ? `人工决定：${decisionLabel(run.humanDecision.action)}` : run.revisionKind || '保留审查事实'}</Text><Text type="secondary">{formatTime(run.finishedAt || run.createdAt)} · ReviewRun {run.id}</Text></button>}</VirtualList> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="审查周期尚未产生" />}<Modal title="审查周期详情" open={Boolean(detailRunId)} footer={null} onCancel={() => setDetailRunId(undefined)} destroyOnHidden>{detail.isLoading ? <Flex align="center" gap={8}><ClockCircleOutlined spin /><Text>正在读取周期详情…</Text></Flex> : detail.isError || !detail.data ? <Alert type="warning" showIcon title="周期详情暂时无法加载" description="请关闭后重试。" /> : <RunDetail run={detail.data} />}</Modal></section>
  </Card>;
}

function RunDetail({ run }: { run: ReviewRunView }) {
  return <Space orientation="vertical" size={12} style={{ width: '100%' }}><Descriptions size="small" column={1} items={[{ key: 'status', label: '状态', children: `${run.status}${run.verdict ? ` · ${run.verdict}` : ''}` }, { key: 'turn', label: 'TaskTurn', children: run.taskTurnId }, { key: 'tree', label: 'ResultTree', children: run.resultTree }, { key: 'revision', label: '修订类型', children: run.revisionKind || '初始审查' }, { key: 'stop', label: '停止原因', children: run.stopReason ? stopReason(run.stopReason) : '—' }, { key: 'time', label: '时间', children: formatTime(run.finishedAt || run.createdAt) }]} /><Paragraph>{run.report?.summary || '本周期没有可展示的报告摘要。'}</Paragraph>{run.report?.reportDigest && <Text className="review-admin-digest">报告摘要：{run.report.reportDigest}</Text>}{run.humanDecision && <Alert type="info" showIcon title={`人工决定：${decisionLabel(run.humanDecision.action)}`} description={run.humanDecision.revisionGuidance ? `修订建议：${run.humanDecision.revisionGuidance}` : '已记录安全审计事实。'} />}</Space>;
}

function ReviewFact({ label, value, tone }: { label: string; value: string; tone?: 'success' | 'warning' | 'danger' }) { return <div className="review-fact"><Text type="secondary">{label}</Text><strong className={tone ? `is-${tone}` : undefined}>{value}</strong></div>; }

function EvidenceList({ title, empty, items }: { title: string; empty: string; items: React.ReactNode[] }) { return <Card title={title}>{items.length ? <Space orientation="vertical" size={10} style={{ width: '100%' }}>{items}</Space> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={empty} />}</Card>; }

function ReviewActions({ taskId, report, currentRun }: { taskId: API.Int64; report: IndependentReviewReport; currentRun?: ReviewRunView }) {
  const [action, setAction] = useState<ReviewDecisionAction>();
  const buttonRef = useRef<HTMLElement>(null);
  const last = currentRun?.humanDecision;
  return <Card className="review-action-card" title="下一步">
    <Space orientation="vertical" size={12} style={{ width: '100%' }}>
      <Text type="secondary">选择如何处理当前审查结果。</Text>
      {report.availableActions.length ? report.availableActions.map((item) => <Button key={item} block type={item === 'AUTHORIZE_ONE_REVISION' ? 'primary' : 'default'} danger={item === 'CANCEL_TASK'} onClick={(event) => { buttonRef.current = event.currentTarget; setAction(item); }}>{decisionLabel(item)}</Button>) : <Alert type="info" showIcon title="当前无需人工操作" description="页面会随审查进展自动刷新。" />}
      <Descriptions size="small" column={1} items={[{ key: 'impact', label: '交付状态', children: report.qualityOutcome || report.verdict === 'PASS' ? '已解除审查阻断' : '审查通过或人工确认前保持锁定' }, { key: 'digest', label: '报告摘要', children: report.reportDigest ? `${report.reportDigest.slice(0, 12)}…` : '尚无可信报告' }]} />
      {report.availableActions.includes('ACCEPT_RISK') && <Alert className="review-risk-note" type="warning" showIcon={false} title={`接受后将允许交付；AI 结论仍保留为“${report.verdict === 'REJECT' ? '拒绝' : '需要修订'}”。`} />}
      <Text type="secondary" className="review-audit-note">所有人工决定都需要二次验证并填写原因。</Text>
      {last && <div className="review-last-decision"><Text type="secondary">最近人工决定</Text><div><Text strong>{decisionLabel(last.action)}</Text></div><Text type="secondary">{formatTime(last.decidedAt)}</Text>{last.revisionGuidance && <Paragraph ellipsis={{ rows: 3 }}>修订建议：{last.revisionGuidance}</Paragraph>}</div>}
    </Space>
    {action && currentRun && <ReviewDecisionDialog key={`${currentRun.id}:${action}`} open action={action} taskId={taskId} run={currentRun} report={report} onClose={() => { setAction(undefined); window.requestAnimationFrame(() => buttonRef.current?.focus()); }} />}
  </Card>;
}

function ReviewDecisionDialog({ open, action, taskId, run, report, onClose }: { open: boolean; action: ReviewDecisionAction; taskId: API.Int64; run: ReviewRunView; report: IndependentReviewReport; onClose: () => void }) {
  const { message } = App.useApp(); const [form] = Form.useForm<{ reason: string; revisionGuidance?: string }>(); const client = useQueryClient(); const copy = decisionCopy[action];
  const idempotencyKey = useRef(`review-decision:${run.id}:${action}:${crypto.randomUUID()}`);
  const mutation = useMutation({ mutationFn: (values: { reason: string; revisionGuidance?: string }) => decideIndependentReview(run.id, { action, reason: values.reason, revisionGuidance: values.revisionGuidance, idempotencyKey: idempotencyKey.current }), onSuccess: async () => { message.success(action === 'AUTHORIZE_ONE_REVISION' ? '已授权一次修订' : action === 'ACCEPT_RISK' ? '已接受当前风险' : '任务已取消'); await Promise.all([client.invalidateQueries({ queryKey: ['workforce', 'independent-review', taskId] }), client.invalidateQueries({ queryKey: ['workforce', 'independent-review-runs', taskId] }), client.invalidateQueries({ queryKey: ['workforce', 'task', taskId] })]); onClose(); }, onError: (error) => { const code = reviewErrorCode(error); if (code.includes('GUIDANCE')) form.setFields([{ name: 'revisionGuidance', errors: [reviewFeedback(error)] }]); else if (code.includes('REASON')) form.setFields([{ name: 'reason', errors: [reviewFeedback(error)] }]); } });
  const guidanceRequired = action === 'AUTHORIZE_ONE_REVISION' && report.revisionGuidanceRequired;
  return <Modal open={open} title={copy.title} onCancel={onClose} okText={copy.submit} okButtonProps={{ danger: copy.tone === 'danger' }} confirmLoading={mutation.isPending} onOk={() => form.submit()} destroyOnHidden>
    <Space orientation="vertical" size={14} style={{ width: '100%' }}>
      {action === 'AUTHORIZE_ONE_REVISION' && <Alert type="info" showIcon title="本次只创建一个修订 Turn" description="不会重置自动修订额度；修订结果仍需重新接受独立审查。" />}
      {action === 'ACCEPT_RISK' && <Alert type="warning" showIcon title={`AI 结论仍保持 ${report.verdict}`} description="确认后质量结论记为人工接受，Artifact 与 Delivery 将解锁；不会改写为 AI 审查通过。" />}
      {action === 'CANCEL_TASK' && <Alert type="warning" showIcon title="任务将停止" description="已产生的审查证据会保留，迟到结果不会恢复任务。" />}
      {mutation.isError && !reviewErrorCode(mutation.error).includes('GUIDANCE') && !reviewErrorCode(mutation.error).includes('REASON') && <Alert type="error" showIcon title={reviewFeedback(mutation.error)} action={reviewErrorCode(mutation.error) === 'INDEPENDENT_REVIEW_STALE' ? <Button size="small" onClick={() => void client.invalidateQueries({ queryKey: ['workforce', 'independent-review', taskId] })}>刷新状态</Button> : undefined} />}
      <Form form={form} layout="vertical" onFinish={(values) => mutation.mutate(values)} preserve>
        <Form.Item name="reason" label="授权原因" rules={[{ required: true, message: '请填写本次高风险操作的授权原因' }, { max: 2000 }]}><Input.TextArea autoFocus rows={3} maxLength={2000} showCount placeholder="说明为什么批准本次操作；该内容只用于审计，不会交给 Agent。" /></Form.Item>
        {action === 'AUTHORIZE_ONE_REVISION' && <Form.Item name="revisionGuidance" label={`补充修订建议${guidanceRequired ? '（必填）' : ''}`} extra={guidanceRequired ? '当前没有可信审查报告，请明确下一次修订方向。' : '可补充实施约束；不会覆盖 AI 报告和冻结政策。'} rules={[{ required: guidanceRequired, message: '请填写明确的修订建议' }]}><Input.TextArea rows={4} maxLength={4000} showCount placeholder="例如：保持 API 兼容，只处理分页边界与空值问题。" /></Form.Item>}
      </Form>
    </Space>
  </Modal>;
}

function decisionLabel(action: string) { return ({ ACCEPT_RISK: '接受当前结果', AUTHORIZE_ONE_REVISION: '授权一次修订', CANCEL_TASK: '取消任务' } as Record<string, string>)[action] ?? action; }
function stopReason(value: string) { return ({ INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED: '自动修订额度已耗尽', INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS: '连续审查未发现实质进展', INDEPENDENT_REVIEW_REJECTED: 'AI Reviewer 拒绝当前结果', INDEPENDENT_REVIEW_REPORT_INVALID: '审查报告未通过可信校验', INDEPENDENT_REVIEW_RETRY_EXHAUSTED: '审查运行重试已耗尽' } as Record<string, string>)[value] ?? '需要人工处理'; }
function noTrustedReportLabel(value?: string) { return value === 'INDEPENDENT_REVIEW_RETRY_EXHAUSTED' || value === 'INDEPENDENT_REVIEW_REPORT_INVALID' || value === 'INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE' ? '审查失败，需要人工决定' : '审查中'; }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—'; }
