import type {
  ProjectDeliveryMode,
  ProjectTaskSummary,
  WorkforceProject,
} from '@/api/projectContract';

export const deliveryLabels: Record<ProjectDeliveryMode, string> = {
  NONE: '仅保存平台结果',
  NEW_BRANCH: '推送结果分支',
  NEW_BRANCH_AND_DRAFT_REVIEW: '结果分支 + 草稿评审',
};

export function projectSourceLabel(project: WorkforceProject): string {
  if (project.kind === 'GENERAL') return '空白工作区';
  return `${project.repository?.displayUrl ?? '代码仓库'} · ${project.repository?.defaultRefName ?? '默认分支'}`;
}

export function completionCheckLabel(project: WorkforceProject): string {
  if (project.completionCheck.mode === 'COMMAND') return project.completionCheck.command || '命令检查';
  if (project.completionCheck.mode === 'SCRIPT') return '项目检查脚本';
  return '未配置额外检查';
}

export function projectTaskStatus(status: string): { label: string; color: string } {
  const normalized = status.trim().toUpperCase();
  if (normalized === 'SUCCEEDED') return { label: '已完成', color: 'success' };
  if (normalized === 'FAILED' || normalized === 'CANCELLED') return { label: normalized === 'FAILED' ? '未完成' : '已取消', color: 'default' };
  if (normalized.includes('WAIT') || normalized.includes('SUSPEND') || normalized === 'MANUAL_REVIEW') return { label: '待处理', color: 'gold' };
  if (normalized === 'PENDING' || normalized === 'QUEUED') return { label: '排队中', color: 'blue' };
  return { label: '进行中', color: 'processing' };
}

export function taskPrimaryText(task: ProjectTaskSummary): string {
  return task.resultSummary?.trim() || task.instruction;
}

export function formatProjectTime(value?: string): string {
  if (!value) return '暂无任务';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date);
}
