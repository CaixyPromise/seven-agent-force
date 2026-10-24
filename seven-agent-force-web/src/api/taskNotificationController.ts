import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };

export type TaskNotificationEvent =
  | 'TASK_ACTION_REQUIRED'
  | 'TASK_INTERRUPTED'
  | 'TASK_COMPLETED';

export interface TaskNotificationChannel {
  kind: 'IN_APP' | 'EMAIL_USER' | 'FEISHU_DIRECT' | 'WECOM_DIRECT' | 'GROUP_ROUTE';
  routeId?: API.Int64;
}

export interface TaskNotificationCreateSettings {
  enabled: boolean;
  events: TaskNotificationEvent[];
  channels: TaskNotificationChannel[];
}

export interface TaskNotificationSubscription {
  id: API.Int64;
  tenantId: API.Int64;
  taskId: API.Int64;
  subscriberUserId: API.Int64;
  events: TaskNotificationEvent[];
  channels: TaskNotificationChannel[];
  enabled: boolean;
  version: API.Int64;
  createdAt: string;
  updatedAt: string;
}

export type TaskNotificationFactType =
  | 'TASK_ACTION_REQUIRED'
  | 'TASK_INTERRUPTED'
  | 'TASK_COMPLETED';

export type TaskNotificationFocus = 'evaluation' | 'result' | 'continuation' | 'delivery';

export type TaskNotificationRecordStatus =
  | 'PENDING'
  | 'PROCESSING'
  | 'RETRY_WAIT'
  | 'PUBLISHED'
  | 'SUPPRESSED'
  | 'FAILED';

export interface TaskNotificationRecord {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  factType: TaskNotificationFactType;
  sourceType: string;
  sourceId: API.Int64;
  reasonCode: string;
  focus: TaskNotificationFocus;
  deepLink?: string;
  status: TaskNotificationRecordStatus;
  warningCode?: string;
  /** Opaque receipt returned by Notification Center. It is not a database ID. */
  notificationReceiptIdentity?: string;
  createdAt: string;
  publishedAt?: string;
  updatedAt: string;
  channels?: Array<{
    kind: TaskNotificationChannel['kind'];
    routeId?: API.Int64;
    mentionUserId?: API.Int64;
  }>;
  deliveries?: Array<{
    channelKind: string;
    displayName: string;
    state: 'QUEUED' | 'DELIVERED' | 'FAILED' | 'NEEDS_ATTENTION' | 'SUPPRESSED' | 'OUTCOME_UNKNOWN';
    mentionState: 'MENTIONED' | 'NOT_REQUESTED' | 'NOT_AVAILABLE';
    safeReasonCode?: string;
    acceptedAt?: string;
    updatedAt: string;
  }>;
}

export interface TaskNotificationGroupRoute {
  id: API.Int64;
  name: string;
  provider: 'FEISHU' | 'WECOM';
  mentionReady: boolean;
}

export interface TaskNotificationReadiness {
  inApp: {
    available: boolean;
    reasonCode?: string;
  };
  email: { available: boolean; reasonCode?: string };
  feishu: { available: boolean; directReady?: boolean; mentionReady?: boolean; reasonCode?: string };
  wecom: { available: boolean; directReady?: boolean; mentionReady?: boolean; reasonCode?: string };
  groupRoutes: TaskNotificationGroupRoute[];
}

export interface UpdateTaskNotificationSubscriptionInput {
  enabled: boolean;
  events: TaskNotificationEvent[];
  channels: TaskNotificationChannel[];
  expectedVersion: API.Int64;
}

export async function getTaskNotificationReadiness(): Promise<TaskNotificationReadiness> {
  const response = await request<Envelope<TaskNotificationReadiness>>('/api/workforce/task-notifications/readiness', {
    method: 'GET',
  });
  return response.data;
}

export async function getTaskNotificationSubscription(taskId: API.Int64): Promise<TaskNotificationSubscription> {
  const response = await request<Envelope<TaskNotificationSubscription>>(
    `/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/notification-subscription`,
    { method: 'GET' },
  );
  return response.data;
}

export async function updateTaskNotificationSubscription(
  taskId: API.Int64,
  input: UpdateTaskNotificationSubscriptionInput,
): Promise<TaskNotificationSubscription> {
  const response = await request<Envelope<TaskNotificationSubscription>>(
    `/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/notification-subscription`,
    {
      method: 'PUT',
      data: {
        ...input,
        expectedVersion: toApiIdParam(input.expectedVersion),
        channels: input.channels.map((channel) => ({
          kind: channel.kind,
          ...(channel.routeId ? { routeId: toApiIdParam(channel.routeId) } : {}),
        })),
      },
    },
  );
  return response.data;
}

export async function listTaskNotificationRecords(taskId: API.Int64): Promise<TaskNotificationRecord[]> {
  const response = await request<Envelope<TaskNotificationRecord[]>>(
    `/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/notification-records`,
    { method: 'GET' },
  );
  return response.data ?? [];
}
