import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';
import { normalizeTaskConversationSummary, type TaskConversationSummary } from './taskSummaryContract';
import { normalizeTaskStatus, type TaskStatus } from './taskStatusContract';
import { normalizeTaskArtifacts } from './taskArtifactContract';

export type { TaskStatus } from './taskStatusContract';

export interface WorkforceTask extends TaskConversationSummary {
  id: API.Int64;
  tenantId: API.Int64;
  projectId: API.Int64;
  projectVersionId: API.Int64;
	selectedRuntimeReleaseId?: API.Int64;
	selectedRuntime?: {
		runtimeReleaseId: API.Int64;
		name: string;
		imageDigest: string;
	};
  name: string;
  tags: string[];
  /** Lossless Framework user ID. Resolve display fields through UserSelector. */
  createdBy: API.Int64;
	metadataVersion: API.Int64;
  inputKind?: 'EMPTY_WORKSPACE' | 'SOURCE_REVISION';
  referenceCount?: number;
  repositoryUrl: string;
  repositoryRef: string;
  resolvedBaseCommit?: string;
  instruction: string;
  resultProcessorReleaseId?: API.Int64;
  taskTurnId?: API.Int64;
  status: TaskStatus;
  priority: number;
  executionTimeoutSeconds: number;
  /** Server-confirmed Agent-active time, across replacement Attempts. */
  executionActiveMilliseconds?: API.Int64;
  lastProgressAt?: string;
  resultSummary?: string;
  latestAttemptId?: API.Int64;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  execution?: WorkforceExecutionView;
  independentReview?: {
    required: boolean;
    status?: string;
  };
  cancelRequestId?: API.Int64;
  cancelGeneration?: API.Int64;
  cancelRequestedBy?: string;
  cancelRequestedAt?: string;
  cancelReason?: string;
}

export interface WorkforceExecutionView {
  executionNodeId: API.Int64;
  nodeKey: string;
  nodeStatus: string;
  queueWaitReason?: string;
  executionRequirementDigest: string;
  assignmentId?: API.Int64;
  assignmentStatus?: string;
  workerId?: API.Int64;
  workerInstanceId?: string;
  hostBootId?: string;
  attemptId?: API.Int64;
  generation: API.Int64;
  containerName?: string;
  containerId?: string;
  executionIdentityDigest?: string;
  runtimeStatus?: string;
  ownerEpoch?: API.Int64;
  executionLeaseExpiresAt?: string;
  containerCreatedAt?: string;
  supervisorReadyAt?: string;
  runPermitIssuedAt?: string;
  harnessStartedAt?: string;
  deliveryAckedAt?: string;
  attemptStatus?: string;
  attemptNumber?: number;
  maxAttempts: number;
  retryCount: number;
  nextRetryAt?: string;
  lastRetryReasonCode?: string;
  recoveryPreviousStatus?: string;
  recoveryStartedAt?: string;
  reattachDeadline?: string;
  pendingWorkerInstanceId?: string;
  pendingOwnerEpoch?: API.Int64;
  reattachedAt?: string;
  watchdogDeadline?: string;
  terminalReasonCode?: string;
  resolved?: {
    runtimeReleaseId: API.Int64;
    runtimeAlias: string;
    runtimeImageDigest: string;
    adapterId: string;
    adapterVersion: string;
    adapterProtocol: string;
    adapterCapabilities: string[];
    hookConfigurationDigest?: string;
    hooks: Array<{
      releaseId: API.Int64;
      hookId: API.Int64;
      name: string;
      version: string;
    }>;
    modelCredentialId: API.Int64;
    modelCredentialName: string;
    modelProtocol: 'OPENAI' | 'ANTHROPIC';
    primaryModel: string;
    credentialVersion: API.Int64;
    workerImageContract: string;
    isolationProfile: string;
    selectionDigest: string;
    executionDeadlineSeconds: number;
  };
}

export type WorkerState = 'PROVISIONED' | 'RECONCILING' | 'ONLINE' | 'DRAINING' | 'DRAINED' | 'STALE' | 'OFFLINE' | 'DISABLED' | 'DECOMMISSIONED';
export interface WorkforceWorkerResourceSnapshot {
  logicalCpuCores?: number;
  cpuUsagePercent?: number;
  cpuUsageAvailable?: boolean;
  load1?: number;
  memoryTotalBytes?: number;
  memoryUsedBytes?: number;
  memoryAvailableBytes?: number;
  memoryUsagePercent?: number;
  workspaceDiskTotalBytes?: number;
  workspaceDiskUsedBytes?: number;
  workspaceDiskAvailableBytes?: number;
  workspaceDiskUsagePercent?: number;
}

export interface WorkforceWorker {
  id: API.Int64;
  name: string;
  description: string;
  desiredState: 'ENABLED' | 'DRAINING' | 'DISABLED';
  state: WorkerState;
  maxActiveAttempts: number;
  reportedMaxActiveAttempts: number;
  currentInstanceId?: string;
  hostBootId?: string;
  workerVersion?: string;
  workerRevision?: string;
  workerBuildDate?: string;
  tokenHint: string;
  observedRemoteIp?: string;
  reportedIp?: string;
  hostname?: string;
  primaryInterface?: string;
  primaryMac?: string;
  consumerState?: string;
  sessionState?: string;
  reconciliationDeadline?: string;
  dockerReady: boolean;
  activeAttempts: number;
  reservedAttempts: number;
  startingAttempts: number;
  runningAttempts: number;
  availableAttempts: number;
	 inventoryRefreshGeneration: API.Int64;
	 inventoryVersion: API.Int64;
  decommissionedAt?: string;
  lastHeartbeatAt?: string;
  resourceSnapshot?: WorkforceWorkerResourceSnapshot;
  interfaces?: Array<{ name: string; addresses: string[]; mac?: string }>;
  imageCount: number;
  pendingFinalizations: number;
  pendingArtifactUploads: number;
	 version: API.Int64;
  createdAt: string;
}

export interface WorkforceWorkerImage {
  imageDigest: string;
  imageRef: string;
  role: 'attempt' | 'provider-proxy';
  contractVersion: string;
  manifestDigest: string;
  manifest: Record<string, unknown>;
  capabilities: Record<string, unknown>;
  admissionStatus: 'ADMITTED' | 'REJECTED';
  admissionEvidence: Record<string, unknown>;
  inventoryVersion: API.Int64;
  presenceStatus: 'PRESENT' | 'MISSING' | 'DRIFTED';
  missingSince?: string;
  driftReason?: string;
}

export interface WorkforceAssignment {
  id: API.Int64;
  taskId: API.Int64;
  executionNodeId: API.Int64;
  workerInstanceId: string;
  generation: API.Int64;
  executionRequirementDigest: string;
  status: string;
  messageId: API.Int64;
  attemptId?: API.Int64;
  createdAt: string;
  claimedAt?: string;
  completedAt?: string;
}

export type WorkforceWorkerMetricRange = '1h' | '6h' | '24h' | '7d' | '30d';
export interface WorkforceWorkerMetricPoint {
  at: string;
  cpuUsagePercent: number | null;
  memoryUsagePercent: number | null;
  workspaceDiskUsagePercent: number | null;
  activeAttempts: number;
  maxActiveAttempts: number;
}
export interface WorkforceWorkerMetricSeries {
  from: string;
  to: string;
  stepSeconds: number;
  points: WorkforceWorkerMetricPoint[];
}

export async function listWorkforceWorkers(): Promise<WorkforceWorker[]> {
  const response = await request<DataEnvelope<WorkforceWorker[]>>('/api/workforce/workers', { method: 'GET' });
  return response.data ?? [];
}

export async function getWorkforceWorkerMetrics(range: WorkforceWorkerMetricRange, workerId?: API.Int64): Promise<WorkforceWorkerMetricSeries> {
  const prefix = workerId ? `/api/workforce/workers/${encodeURIComponent(toApiIdParam(workerId))}` : '/api/workforce/workers';
  const response = await request<DataEnvelope<WorkforceWorkerMetricSeries>>(`${prefix}/metrics?range=${encodeURIComponent(range)}`, { method: 'GET' });
  return response.data;
}

export interface WorkforceWorkerQueue {
  exchange: string;
  queue: string;
  routingKey: string;
  deadQueue: string;
  maxPrefetch: number;
  messageSchema: string;
  postprocessExchange: string;
  postprocessQueue: string;
  postprocessRoutingKey: string;
  postprocessDeadQueue: string;
}

export interface WorkforceRecoverableRuntime {
  taskId: API.Int64;
  attemptId: API.Int64;
  executionNodeId: API.Int64;
  assignmentId: API.Int64;
  generation: API.Int64;
  ownerEpoch: API.Int64;
  hostBootId: string;
  expectedContainerName: string;
  containerId: string;
  runtimeStatus: string;
  executionIdentityDigest: string;
  harnessImageDigest: string;
  hookBundleDigest: string;
  reattachDeadline: string;
}

export interface WorkforceReconciliation {
  required: boolean;
  deadline?: string;
  runtimes: WorkforceRecoverableRuntime[];
}

export async function getWorkforceWorker(id: API.Int64): Promise<{ worker: WorkforceWorker; images: WorkforceWorkerImage[]; queue: WorkforceWorkerQueue; reconciliation: WorkforceReconciliation }> {
  const response = await request<DataEnvelope<{ worker: WorkforceWorker; images: WorkforceWorkerImage[]; queue: WorkforceWorkerQueue; reconciliation: WorkforceReconciliation }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(id))}`, { method: 'GET' });
  return response.data;
}

export async function listWorkforceWorkerAssignments(id: API.Int64): Promise<WorkforceAssignment[]> {
  const response = await request<DataEnvelope<WorkforceAssignment[]>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(id))}/assignments`, { method: 'GET' });
  return response.data ?? [];
}

export async function createWorkforceWorker(input: { name: string; description?: string; maxActiveAttempts: number }): Promise<{ worker: WorkforceWorker; credential: { token: string; tokenHint: string } }> {
  const response = await request<DataEnvelope<{ worker: WorkforceWorker; credential: { token: string; tokenHint: string } }>>('/api/workforce/workers', { method: 'POST', data: input });
  return response.data;
}

export async function setWorkforceWorkerDesiredState(worker: WorkforceWorker, desiredState: 'ENABLED' | 'DRAINING' | 'DISABLED'): Promise<WorkforceWorker> {
	const response = await request<DataEnvelope<{ worker: WorkforceWorker }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(worker.id))}/desired-state`, { method: 'PUT', data: { desiredState, version: toApiIdParam(worker.version) } });
  return response.data.worker;
}

export async function updateWorkforceWorkerCapacity(worker: WorkforceWorker, maxActiveAttempts: number): Promise<WorkforceWorker> {
	const response = await request<DataEnvelope<{ worker: WorkforceWorker }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(worker.id))}/capacity`, {
		method: 'PUT',
		data: { maxActiveAttempts, version: toApiIdParam(worker.version) },
	});
	return response.data.worker;
}

export async function rotateWorkforceWorkerToken(id: API.Int64): Promise<{ token: string; tokenHint: string }> {
  const response = await request<DataEnvelope<{ credential: { token: string; tokenHint: string } }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(id))}/token-rotations`, { method: 'POST' });
  return response.data.credential;
}

export async function refreshWorkforceWorkerInventory(id: API.Int64): Promise<API.Int64> {
	const response = await request<DataEnvelope<{ refreshInventoryGeneration: API.Int64 }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(id))}/image-inventory-refreshes`, { method: 'POST' });
  return response.data.refreshInventoryGeneration;
}

export async function decommissionWorkforceWorker(worker: WorkforceWorker): Promise<{ decommissioned: boolean; purgedMessages: number }> {
	const response = await request<DataEnvelope<{ decommissioned: boolean; purgedMessages: number }>>(`/api/workforce/workers/${encodeURIComponent(toApiIdParam(worker.id))}/decommission`, { method: 'POST', data: { version: toApiIdParam(worker.version) } });
  return response.data;
}

export interface WorkforceEvent {
  id: API.Int64;
  taskId: API.Int64;
  attemptId?: API.Int64;
  type: string;
  sequence: number;
  payload: Record<string, unknown>;
  createdAt: string;
}

export interface WorkforceArtifact {
  id: API.Int64;
  taskId: API.Int64;
  attemptId: API.Int64;
  type: string;
  name: string;
  sha256: string;
  sizeBytes: number;
  mediaType: string;
  status: string;
  downloadUrl: string;
}

export interface WorkforceArtifactUploadAttempt {
  id: API.Int64;
  finalizationRunId: API.Int64;
  taskId: API.Int64;
  attemptId: API.Int64;
  artifactType: string;
  name: string;
  objectKey: string;
  sha256: string;
  sizeBytes: number;
  mediaType: string;
  attemptNumber: number;
  maxAttempts: number;
  status: string;
  nextRetryAt?: string;
  errorCode?: string;
  startedAt?: string;
  uploadedAt?: string;
  verifiedAt?: string;
}

export interface WorkforceFinalizationRun {
  id: API.Int64;
  taskId: API.Int64;
  attemptId: API.Int64;
  executionNodeId: API.Int64;
  generation: API.Int64;
  status: string;
  runNumber: number;
  maxRuns: number;
  workerId: API.Int64;
  workerInstanceId?: string;
  hostBootId: string;
  manifestDigest?: string;
  spoolIdentity?: string;
  nextRetryAt?: string;
  errorCode?: string;
  createdAt: string;
  startedAt?: string;
  frozenAt?: string;
  finishedAt?: string;
  artifactUploadAttempts: WorkforceArtifactUploadAttempt[];
}

export interface WorkforceRecoveryView {
  taskId: API.Int64;
  status: TaskStatus;
  execution?: WorkforceExecutionView;
  cancelRequestId?: API.Int64;
  cancelGeneration?: API.Int64;
  cancelRequestedBy?: string;
  cancelRequestedAt?: string;
  cancelReason?: string;
  finalizationRuns: WorkforceFinalizationRun[];
}

type DataEnvelope<T> = { data: T; replayed?: boolean };
export interface WorkforceTaskPage { items: WorkforceTask[]; nextCursor: API.Int64 | null }
export async function listWorkforceTasks(input: { query?: string; view?: string; from?: string; to?: string; cursor?: API.Int64; limit?: number } = {}): Promise<WorkforceTaskPage> {
  const response = await request<DataEnvelope<WorkforceTaskPage>>('/api/workforce/tasks', {
    method: 'GET',
    params: { ...(input.query ? { query: input.query } : {}), ...(input.view ? { view: input.view } : {}), ...(input.from ? { from: input.from } : {}), ...(input.to ? { to: input.to } : {}), ...(input.cursor ? { cursor: toApiIdParam(input.cursor) } : {}), limit: input.limit ?? 30 },
  });
  return { items: (response.data?.items ?? []).map((task) => ({ ...task, status: normalizeTaskStatus(task.status), ...normalizeTaskConversationSummary(task) })), nextCursor: response.data?.nextCursor ?? null };
}

export async function getWorkforceTask(id: API.Int64): Promise<WorkforceTask> {
  const response = await request<DataEnvelope<WorkforceTask>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(id))}`, { method: 'GET' });
  return { ...response.data, status: normalizeTaskStatus(response.data.status) };
}

export async function updateWorkforceTaskMetadata(id: API.Int64, input: { name: string; tags: string[]; expectedMetadataVersion: API.Int64 }): Promise<WorkforceTask> {
	const expectedMetadataVersion = toApiIdParam(input.expectedMetadataVersion);
  const response = await request<DataEnvelope<WorkforceTask>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(id))}/metadata`, {
    method: 'PATCH',
    data: { ...input, expectedMetadataVersion },
  });
  return response.data;
}

export interface WorkforceBacklogSummary {
  queuedTasks: number;
  oldestWaitSeconds: string | number;
  reasons: Array<{ reasonCode: string; count: number }>;
  compatibleWorkers: number;
  availableWorkers: number;
}

export async function getWorkforceBacklog() {
  const response = await request<DataEnvelope<WorkforceBacklogSummary>>('/api/workforce/backlog', { method: 'GET' });
  return response.data;
}

export type WorkforceModelCredentialStatus = 'ACTIVE' | 'DISABLED';
export type WorkforceModelProtocol = 'OPENAI' | 'ANTHROPIC';

export interface WorkforceModelCredential {
  id: API.Int64;
  name: string;
  protocol: WorkforceModelProtocol;
  baseUrl: string;
  models: string[];
  credentialConfigured: boolean;
  secretHint?: string;
  status: WorkforceModelCredentialStatus;
  version: API.Int64;
  lastDiscoveryStatus?: string;
  lastDiscoveryReasonCode?: string;
  lastDiscoveredAt?: string;
  createdAt?: string;
  updatedAt?: string;
  invocationCount: number;
  inputTokens: number;
  cachedInputTokens: number;
  outputTokens: number;
  totalTokens: number;
}

export interface WorkforceModelCredentialInput {
  name: string;
  protocol: WorkforceModelProtocol;
  baseUrl: string;
  apiKey: string;
  models: string[];
}

export interface WorkforceModelOption {
  credentialId: API.Int64;
  credentialName: string;
  protocol: WorkforceModelProtocol;
  model: string;
  label: string;
}

export async function listWorkforceModelCredentials(): Promise<WorkforceModelCredential[]> {
  const response = await request<DataEnvelope<WorkforceModelCredential[]>>('/api/workforce/model-credentials', { method: 'GET' });
  return response.data ?? [];
}

export async function listWorkforceModelOptions(): Promise<WorkforceModelOption[]> {
  const response = await request<DataEnvelope<WorkforceModelOption[]>>('/api/workforce/model-options', { method: 'GET' });
  return response.data ?? [];
}

export async function createWorkforceModelCredential(input: WorkforceModelCredentialInput): Promise<WorkforceModelCredential> {
  const response = await request<DataEnvelope<WorkforceModelCredential>>('/api/workforce/model-credentials', { method: 'POST', data: input });
  return response.data;
}

export async function updateWorkforceModelCredential(
  credential: WorkforceModelCredential,
  input: WorkforceModelCredentialInput,
): Promise<WorkforceModelCredential> {
  const response = await request<DataEnvelope<WorkforceModelCredential>>(`/api/workforce/model-credentials/${encodeURIComponent(toApiIdParam(credential.id))}`, {
    method: 'PUT', data: { ...input, expectedVersion: credential.version },
  });
  return response.data;
}

export async function setWorkforceModelCredentialStatus(credential: WorkforceModelCredential, status: WorkforceModelCredentialStatus): Promise<WorkforceModelCredential> {
  const response = await request<DataEnvelope<WorkforceModelCredential>>(`/api/workforce/model-credentials/${encodeURIComponent(toApiIdParam(credential.id))}/status`, {
    method: 'PUT', data: { status, expectedVersion: credential.version },
  });
  return response.data;
}

export async function deleteWorkforceModelCredential(credential: WorkforceModelCredential): Promise<{ deleted: boolean; retainedHistory: boolean }> {
  const response = await request<DataEnvelope<{ deleted: boolean; retainedHistory: boolean }>>(`/api/workforce/model-credentials/${encodeURIComponent(toApiIdParam(credential.id))}`, {
    method: 'DELETE', data: { expectedVersion: credential.version },
  });
  return response.data;
}

export async function discoverWorkforceModels(input: { credentialId?: API.Int64; protocol: WorkforceModelProtocol; baseUrl: string; apiKey: string }): Promise<string[]> {
  const response = await request<DataEnvelope<{ models: string[] }>>('/api/workforce/model-credentials/models:discover', {
    method: 'POST', data: input, timeout: 30_000,
  });
  return response.data.models ?? [];
}

export interface WorkforceModelInvocation {
  id: API.Int64;
  taskId: API.Int64;
  taskName: string;
  attemptId?: API.Int64;
  executionKind?: 'AGENT_ATTEMPT' | 'REVIEW_RUN';
  modelCredentialId?: API.Int64;
  credentialVersion?: API.Int64;
  protocol?: WorkforceModelProtocol;
  primaryModel?: string;
  requestedModel: string;
  responseModel?: string;
  isPrimaryModel: boolean;
  operation: 'responses.create' | 'responses.compact' | 'messages.create' | 'messages.count_tokens';
  status: 'AUTHORIZED' | 'STREAMING' | 'SUCCEEDED' | 'FAILED' | 'CANCELLED' | 'OUTCOME_UNKNOWN';
  statusCode?: number;
  durationMs: number;
  firstByteMs?: number;
  upstreamRequestId?: string;
  usage: { inputTokens?: number; cachedInputTokens?: number; cacheCreationInputTokens?: number; outputTokens?: number; reasoningOutputTokens?: number; totalTokens?: number };
  errorCode?: string;
  startedAt: string;
  finishedAt?: string;
}

export interface WorkforceModelAnalytics {
  days: 7 | 30 | 90;
  summary: {
    activeCredentialCount: number;
    modelCount: number;
    invocationCount: number;
    totalTokens: number;
  };
  timeline: Array<{ date: string; invocationCount: number; totalTokens: number }>;
  modelDistribution: Array<{ model: string; invocationCount: number }>;
  cacheHitRate?: number;
  p95DurationMs?: number;
  comparison: {
    invocationPercent?: number;
    tokenPercent?: number;
  };
}

export async function getWorkforceModelAnalytics(days: 7 | 30 | 90): Promise<WorkforceModelAnalytics> {
  const response = await request<DataEnvelope<WorkforceModelAnalytics>>('/api/workforce/model-credentials/analytics', {
    method: 'GET',
    params: { days },
  });
  return response.data;
}

export async function listTaskModelInvocations(taskId: API.Int64): Promise<WorkforceModelInvocation[]> {
  const response = await request<DataEnvelope<WorkforceModelInvocation[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/model-invocations`, { method: 'GET' });
  return response.data ?? [];
}

export async function listModelCredentialInvocations(credentialId: API.Int64): Promise<WorkforceModelInvocation[]> {
  const response = await request<DataEnvelope<WorkforceModelInvocation[]>>(`/api/workforce/model-credentials/${encodeURIComponent(toApiIdParam(credentialId))}/invocations`, { method: 'GET' });
  return response.data ?? [];
}

export async function cancelWorkforceTask(id: API.Int64): Promise<WorkforceTask> {
  const response = await request<DataEnvelope<{ task: WorkforceTask }>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(id))}/cancel`, { method: 'POST' });
	return response.data.task;
}

export async function getWorkforceTaskRecovery(id: API.Int64): Promise<WorkforceRecoveryView> {
  const response = await request<DataEnvelope<WorkforceRecoveryView>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(id))}/recovery`, { method: 'GET' });
  return response.data;
}

export async function retryWorkforceFinalization(id: API.Int64, reason = ''): Promise<WorkforceFinalizationRun> {
  const response = await request<DataEnvelope<WorkforceFinalizationRun>>(`/api/workforce/finalization-runs/${encodeURIComponent(toApiIdParam(id))}/retry`, { method: 'POST', data: { reason } });
  return response.data;
}

export async function retryWorkforceArtifactUpload(id: API.Int64, reason = ''): Promise<WorkforceArtifactUploadAttempt> {
  const response = await request<DataEnvelope<WorkforceArtifactUploadAttempt>>(`/api/workforce/artifact-upload-attempts/${encodeURIComponent(toApiIdParam(id))}/retry`, { method: 'POST', data: { reason } });
  return response.data;
}

export async function listWorkforceEvents(id: API.Int64): Promise<WorkforceEvent[]> {
  const response = await request<DataEnvelope<WorkforceEvent[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(id))}/events`, { method: 'GET' });
  return response.data ?? [];
}

export async function listWorkforceArtifacts(id: API.Int64): Promise<WorkforceArtifact[]> {
  const taskId = toApiIdParam(id);
  const response = await request<DataEnvelope<unknown>>(`/api/workforce/tasks/${encodeURIComponent(taskId)}/artifacts`, { method: 'GET' });
  return normalizeTaskArtifacts(response.data, taskId);
}

export async function downloadWorkforceArtifact(taskId: API.Int64, artifact: WorkforceArtifact) {
  const blob = await request<Blob>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/artifacts/${encodeURIComponent(toApiIdParam(artifact.id))}/download`, {
    method: 'GET',
    responseType: 'blob',
  });
  const href = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = href;
  anchor.download = artifact.name;
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  URL.revokeObjectURL(href);
}
