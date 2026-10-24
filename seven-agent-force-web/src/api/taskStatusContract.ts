/** Durable Task statuses only. Runtime PAUSING is not checkpoint confirmation. */
const taskStatuses = ['CREATED', 'QUEUED', 'PREPARING', 'RUNNING', 'INTERACTION_SUSPENDED', 'VALIDATING', 'PACKAGING', 'UPLOADING', 'RETRY_WAIT', 'REVIEWING', 'MANUAL_REVIEW', 'SUCCEEDED', 'CANCELLING', 'CANCELLED', 'TIMED_OUT', 'FAILED'] as const;
export type TaskStatus = typeof taskStatuses[number];
export function normalizeTaskStatus(value: unknown): TaskStatus {
  if (typeof value !== 'string' || !(taskStatuses as readonly string[]).includes(value)) throw new Error('TASK_STATUS_INVALID');
  return value as TaskStatus;
}

/** Only the durable server transition attests a confirmed suspend checkpoint. */
export function savedInteractionProgress(status: TaskStatus): string | undefined {
  return status === 'INTERACTION_SUSPENDED' ? '进度已保存' : undefined;
}
