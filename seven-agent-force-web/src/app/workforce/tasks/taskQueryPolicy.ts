const terminalTaskStatuses = new Set(['SUCCEEDED', 'FAILED', 'CANCELLED', 'TIMED_OUT']);
const activeDeliveryStatuses = new Set(['READY', 'DISPATCHED', 'RUNNING', 'CONFIRMING', 'RETRY_WAIT', 'CANCELLING']);
const activeReviewStatuses = new Set(['PENDING_FINALIZATION', 'PENDING', 'ASSIGNED', 'RUNNING', 'VALIDATING_REPORT', 'REVISION_REQUIRED', 'RETRY_WAIT']);

export const ACTIVE_TASK_REFRESH_MS = 10_000;
export const ACTIVE_WORKFLOW_REFRESH_MS = 5_000;

// A terminal task stops polling. Its last status transition must still fetch
// the artifacts released by review instead of retaining PENDING_REVIEW data.
export function artifactQueryKey(taskId: string, taskStatus?: string): readonly string[] {
  return ['workforce', 'artifacts', taskId, taskStatus ?? ''];
}

// Refresh the Review snapshot when the Task changes state, including its final transition.
export function reviewQueryKey(taskId: string, taskStatus?: string): readonly string[] {
  return ['workforce', 'independent-review', taskId, taskStatus ?? ''];
}

export function isTaskTerminal(status?: string): boolean {
  return Boolean(status && terminalTaskStatuses.has(status));
}

export function taskRefreshInterval(status?: string): number | false {
  if (isTaskTerminal(status)) return false;
  return ACTIVE_TASK_REFRESH_MS;
}

export function completionRefreshInterval(status?: string, currentResult?: unknown): number | false {
  if (isTaskTerminal(status) || currentResult) return false;
  return ACTIVE_WORKFLOW_REFRESH_MS;
}

export function reviewRefreshInterval(status?: string, reportStatus?: string, required?: boolean): number | false {
  if (required === false || isTaskTerminal(status)) return false;
  return !reportStatus || activeReviewStatuses.has(reportStatus) ? ACTIVE_WORKFLOW_REFRESH_MS : false;
}

export function deliveryRefreshInterval(workspace: string, taskStatus: string | undefined, deliveries?: Array<{ status: string }>): number | false {
  if (workspace !== 'workflow' && workspace !== 'details') return false;
  if (deliveries?.some((delivery) => activeDeliveryStatuses.has(delivery.status))) return ACTIVE_WORKFLOW_REFRESH_MS;
  return isTaskTerminal(taskStatus) ? false : ACTIVE_TASK_REFRESH_MS;
}

export function notificationRefreshInterval(status?: string): number | false {
  return isTaskTerminal(status) ? false : ACTIVE_TASK_REFRESH_MS;
}
