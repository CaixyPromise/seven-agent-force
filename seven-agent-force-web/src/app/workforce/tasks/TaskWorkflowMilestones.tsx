import { CheckCircleFilled, ClockCircleOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { Skeleton, Tag, Typography } from 'antd';
import type { CompletionCheckSummary } from '@/api/completionCheckContract';
import type { IndependentReviewReport } from '@/api/independentReviewController';

type ResultFocus = 'checks' | 'review';

export default function TaskWorkflowMilestones({ checks, review, taskStatus, loading, onOpen }: { checks?: CompletionCheckSummary; review?: IndependentReviewReport; taskStatus?: string; loading: boolean; onOpen: (focus: ResultFocus) => void }) {
  const current = checks?.currentResult;
  const report = review;
  const blocking = report?.findings?.filter((finding) => finding.severity === 'BLOCKING').length ?? 0;
  const reviewNotStarted = report?.status === 'PENDING_FINALIZATION';
  const executionEnded = ['FAILED', 'TIMED_OUT', 'CANCELLED'].includes(taskStatus ?? '');

  if (loading) return <Skeleton className="workforce-milestone-loading" active paragraph={{ rows: 1 }} />;
  return <div className="workforce-workflow-milestones" aria-label="验收进展">
    {current && <button type="button" className={`workforce-milestone is-${current.status.toLowerCase()}`} onClick={() => onOpen('checks')}>
      <span className="workforce-milestone-icon">{current.status === 'PASSED' ? <CheckCircleFilled /> : <ClockCircleOutlined />}</span>
      <span className="workforce-milestone-copy">
        <span><strong>完成检查</strong><Tag color={current.status === 'PASSED' ? 'success' : current.status === 'FAILED' ? 'error' : 'warning'}>{checkLabel(current.status)}</Tag></span>
        <Typography.Text type="secondary">{current.summary || checks?.reasonCode || '查看检查结果'}</Typography.Text>
      </span>
    </button>}
    {report?.required && <button type="button" className={`workforce-milestone is-${reviewTone(report)}`} onClick={() => onOpen('review')}>
      <span className="workforce-milestone-icon"><SafetyCertificateOutlined /></span>
      <span className="workforce-milestone-copy">
        <span><strong>独立审查</strong><Tag color={reviewNotStarted ? 'default' : reviewColor(report)}>{reviewNotStarted ? '尚未开始' : reviewLabel(report)}</Tag></span>
        <Typography.Text type="secondary">{reviewNotStarted ? (executionEnded ? '执行已结束，未进入独立审查' : '等待执行结果保存后审查') : `${report.score !== undefined ? `评分 ${report.score}` : '等待评分'} · ${blocking} 个阻断问题`}</Typography.Text>
      </span>
    </button>}
  </div>;
}

function checkLabel(status: string): string {
  return ({ PASSED: '通过', FAILED: '未通过', UNAVAILABLE: '待处理', NO_TESTS: '待处理', NOT_CONFIGURED: '未配置', TIMED_OUT: '超时' } as Record<string, string>)[status] ?? '进行中';
}

function reviewLabel(report: IndependentReviewReport): string {
  if (report.qualityOutcome === 'HUMAN_ACCEPTED') return '人工接受';
  if (report.status === 'CANCELLED') return '已取消';
  if (report.status === 'MANUAL_REVIEW') return '等待人工决定';
  if (report.verdict === 'PASS') return 'AI 通过';
  if (report.verdict === 'REJECT') return '拒绝';
  if (report.verdict === 'REVISE') return '修订中';
  return '进行中';
}

function reviewColor(report: IndependentReviewReport): string {
  if (report.qualityOutcome === 'HUMAN_ACCEPTED') return 'gold';
  if (report.status === 'CANCELLED') return 'default';
  if (report.status === 'MANUAL_REVIEW') return 'warning';
  if (report.verdict === 'PASS') return 'success';
  if (report.verdict === 'REJECT') return 'error';
  return 'processing';
}

function reviewTone(report: IndependentReviewReport): string {
  if (report.qualityOutcome === 'HUMAN_ACCEPTED') return 'active';
  if (report.status === 'CANCELLED' || report.status === 'MANUAL_REVIEW') return 'active';
  if (report.verdict === 'PASS') return 'passed';
  if (report.verdict === 'REJECT') return 'failed';
  return 'active';
}
