import { CheckCircleFilled, ClockCircleOutlined, ExclamationCircleFilled, FileTextOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Button, Card, Tag, Typography } from 'antd';
import type { CompletionCheckSummary } from '@/api/completionCheckContract';
import type { IndependentReviewReport } from '@/api/independentReviewController';
import type { DeliveryRequest } from '@/api/deliveryContract';
import type { WorkforceTask } from '@/api/workforceController';
import NetworkGrantsPanel from './NetworkGrantsPanel';

const terminal = new Set(['SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT']);

export default function TaskStatusRail({ task, deliveries, checks, review, latestStep, onOpenPending, onOpenDetails, onOpenResults }: {
  task: WorkforceTask;
  deliveries: DeliveryRequest[];
  checks?: CompletionCheckSummary;
  review?: IndependentReviewReport;
  latestStep: string;
  onOpenPending: () => void;
  onOpenDetails: () => void;
  onOpenResults?: () => void;
}) {
  const pending = task.pendingActionCount ?? 0;
  const state = railState(task.status, pending);
  const delivered = deliveries.some((item) => item.status === 'SUCCEEDED');
  const checkPassed = checks?.currentResult?.status === 'PASSED';
  const reviewPassed = review?.status === 'PASSED' && review.verdict === 'PASS';
  const reviewRequired = review?.required === true;
  const riskAccepted = task.status === 'SUCCEEDED' && review?.qualityOutcome === 'HUMAN_ACCEPTED';
  const reviewLabel = riskAccepted ? '人工接受' : reviewPassed ? 'AI 通过' : review?.status === 'MANUAL_REVIEW' ? '等待人工决定' : review?.status === 'CANCELLED' ? '审查已取消' : '审查结果';

  return <aside className="workforce-task-rail" aria-label="任务状态">
    <Card className="workforce-rail-card workforce-rail-status" title="当前状态">
      <div className={`workforce-rail-state is-${state.tone}`}>
        {state.tone === 'success' ? <CheckCircleFilled /> : state.tone === 'attention' ? <ExclamationCircleFilled /> : <ClockCircleOutlined />}
        <strong>{state.label}</strong>
      </div>
      {!terminal.has(task.status) && <div className="workforce-rail-latest"><Typography.Text type="secondary">最新步骤</Typography.Text><strong>{latestStep}</strong></div>}
      {terminal.has(task.status) && <div className="workforce-rail-facts">
        <Tag color={checkPassed ? 'success' : 'default'}>{checkPassed ? '检查通过' : '检查结果'}</Tag>
        {reviewRequired && <Tag icon={<SafetyCertificateOutlined />} color={riskAccepted ? 'gold' : reviewPassed ? 'success' : 'default'}>{reviewLabel}</Tag>}
        {delivered && <Tag color="success">已交付</Tag>}
      </div>}
    </Card>

    <Card className="workforce-rail-card workforce-rail-actions" title="待你处理" extra={pending > 0 ? <Tag color="orange">{pending}</Tag> : undefined}>
      <div className={`workforce-rail-count${pending > 0 ? ' has-items' : ''}`}>{pending}</div>
      {pending > 0 && <Button type="link" onClick={onOpenPending}>定位待办</Button>}
    </Card>

    {onOpenResults && <Button type="text" icon={<FileTextOutlined />} onClick={onOpenResults}>查看结果与报告</Button>}
    <NetworkGrantsPanel taskId={task.id} compact onManage={onOpenDetails} />
  </aside>;
}

function railState(status: WorkforceTask['status'], pending: number): { label: string; tone: 'active' | 'attention' | 'success' | 'muted' } {
  if (status === 'SUCCEEDED') return { label: '执行已完成', tone: 'success' };
  if (status === 'FAILED') return { label: '执行失败', tone: 'attention' };
  if (status === 'CANCELLED') return { label: '已取消', tone: 'muted' };
  if (status === 'TIMED_OUT') return { label: '已超时', tone: 'attention' };
  if (status === 'CANCELLING') return { label: '取消中', tone: 'muted' };
  if (status === 'INTERACTION_SUSPENDED') return { label: '等待你回答', tone: 'attention' };
  if (status === 'MANUAL_REVIEW' || pending > 0) return { label: '等待你的决定', tone: 'attention' };
  const labels: Partial<Record<WorkforceTask['status'], string>> = {
    CREATED: '任务已创建', QUEUED: '排队中', PREPARING: '准备执行环境',
    RUNNING: 'Agent 运行中', VALIDATING: '检查结果中', REVIEWING: '审查中',
    PACKAGING: '保存结果中', UPLOADING: '上传结果中', RETRY_WAIT: '等待执行重试',
    CANCELLING: '取消中',
  };
  return { label: labels[status] ?? '状态暂不可用', tone: 'active' };
}
