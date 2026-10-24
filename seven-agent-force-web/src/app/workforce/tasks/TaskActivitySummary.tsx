import { Badge, Button, Space, Tag, Typography } from 'antd';
import type { TaskConversationSummary } from '@/api/taskSummaryContract';
import { pendingItemLink } from '@/api/taskSummaryContract';

/** Pure presentation: summaries arrive with the task list, with no per-card query. */
export default function TaskActivitySummary({ taskId, summary, onOpen, showAction = true }: { taskId: string; summary: TaskConversationSummary; onOpen: (url: string) => void; showAction?: boolean }) {
  const pending = summary.pendingActionCount !== undefined && summary.pendingActionCount > 0;
  const link = pendingItemLink(taskId, summary);
  if (!summary.activitySummary && !pending && !summary.hasUnreadUpdates) return null;
  return <Space orientation="vertical" size={4} className="workforce-task-activity">
    {summary.activitySummary && <Typography.Text type="secondary" className="workforce-task-activity-text">{summary.activitySummary}</Typography.Text>}
    <Space wrap size={[8, 4]}>
      {pending && <Tag color="gold">待你处理 · {summary.pendingActionCount} 项</Tag>}
      {summary.hasUnreadUpdates === true && <Space size={6}><Badge color="blue" /><Typography.Text className="workforce-task-unread">新更新</Typography.Text></Space>}
      {showAction && link && <Button type="link" size="small" onKeyDown={(event) => event.stopPropagation()} onClick={(event) => { event.stopPropagation(); onOpen(link); }}>查看待处理</Button>}
    </Space>
  </Space>;
}
