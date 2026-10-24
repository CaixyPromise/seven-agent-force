export type AnalyticsRange = '7d' | '14d' | '30d' | '90d';

export interface TaskTrendPoint { at: string; created: number; succeeded: number; failed: number; cancelled: number }
export interface DurationPoint { at: string; sampleCount: number; p50Seconds: number | null; p90Seconds: number | null }
export interface CountPoint { code: string; count: number }

export interface ProjectListAnalytics {
  from: string; to: string;
  summary: { activeProjectCount: number; createdTaskCount: number; succeededTaskCount: number; attentionProjectCount: number };
  taskTrend: TaskTrendPoint[];
  projectHealth: Array<{ projectId: API.Int64; name: string; succeeded: number; failed: number; active: number; attention: number; latestActivity: string }>;
}

export interface ProjectAnalytics {
  from: string; to: string;
  summary: { createdTaskCount: number; successRate: number | null; successfulDeliveryCount: number; attentionTaskCount: number };
  taskTrend: TaskTrendPoint[];
  stageHealth: Array<{ code: string; eligibleCount: number; reachedCount: number; failedCount: number }>;
  durationTrend: DurationPoint[];
  reviewOutcomes: { disabled: number; directPassed: number; passedAfterRevision: number; acceptedRisk: number; rejected: number; processing: number };
  waitingReasons: CountPoint[];
  modelUsageTrend: Array<{ at: string; invocationCount: number; inputTokens: number | null; cachedInputTokens: number | null; cacheCreationInputTokens: number | null; outputTokens: number | null; reasoningOutputTokens: number | null; totalTokens: number | null; cacheHitRate: number | null }>;
}

export interface TaskAnalytics {
  from: string; to: string;
  summary: { attention: number; running: number; waiting: number; succeeded: number };
  activityTrend: Array<{ at: string; created: number; terminal: number; succeeded: number; failed: number; backlog: number }>;
  statusCounts: CountPoint[];
  waitingAgeBuckets: CountPoint[];
  waitingReasons: CountPoint[];
  durationTrend: DurationPoint[];
}

export interface WorkbenchData {
  from: string;
  to: string;
  summary: {
    attentionCount: number;
    runningTaskCount: number;
    queuedTaskCount: number;
    completedTodayCount: number;
    todaySuccessRate: number | null;
  };
  activeTasks: Array<{
    taskId: API.Int64;
    projectId: API.Int64;
    name: string;
    projectName: string;
    status: string;
    runtimeName: string;
    progressSummary: string;
    waitReasonCode?: string;
    updatedAt: string;
    startedAt?: string;
  }>;
  attentionItems: Array<{
    taskId: API.Int64;
    projectId: API.Int64;
    objectId?: API.Int64;
    taskName: string;
    projectName: string;
    reasonCode: string;
    objectKind: string;
    occurredAt: string;
  }>;
  activityTrend: Array<{ at: string; created: number; completed: number; backlog: number }>;
  resources: {
    onlineWorkerCount: number;
    runningSlots: number;
    maximumSlots: number;
    queuedTasks: number;
    workers: Array<{ workerId: API.Int64; name: string; runningSlots: number; maximumSlots: number }>;
  };
  modelUsage: null | {
    invocationCount: number;
    totalTokens: number | null;
    cacheHitRate: number | null;
    trend: Array<{ at: string; totalTokens: number | null }>;
  };
  recentDeliveries: Array<{
    deliveryId: API.Int64;
    taskId: API.Int64;
    projectId: API.Int64;
    taskName: string;
    projectName: string;
    status: string;
    updatedAt: string;
  }>;
}
