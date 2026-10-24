import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };

export interface TaskTurn {
  id: API.Int64;
  taskId: API.Int64;
  tenantId: API.Int64;
  turnNumber: number;
  instruction: string;
  status: 'ACTIVE' | 'INTERACTION_SUSPENDED' | 'SUCCEEDED' | 'REVIEWING' | 'MANUAL_REVIEW';
  sourceTurnId?: API.Int64;
  sourceCheckpointId?: API.Int64;
  executionSnapshotDigest: string;
  continuationLineageId: API.Int64;
  /** Lossless Framework user ID. */
  createdBy: API.Int64;
  resultCommit?: string;
  resultTree?: string;
  resultSummary?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface ContinuationCheckpointDetail {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  attemptId: API.Int64;
  decision: 'PASS' | 'WAIT_USER' | 'MANUAL_REVIEW';
  status: string;
  checkpointIdentity: string;
  manifestSha256: string;
  expiresAt: string;
  approvedBy?: string;
  approvedAt?: string;
  approvalReason?: string;
  consumedByTurnId?: API.Int64;
  consumedAt?: string;
}

export interface ResultMaterializationRun {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  sourceCheckpointId: API.Int64;
  outputId?: API.Int64;
  kind: 'DECLARED_OUTPUT' | 'ZIP' | 'TAR_GZ' | 'PATCH' | 'GIT_BUNDLE';
  status: 'PENDING' | 'RUNNING' | 'RETRY_WAIT' | 'FROZEN' | 'AVAILABLE' | 'FAILED' | 'CANCELLED';
  runNumber: number;
  maxRuns: number;
  artifactId?: API.Int64;
  artifactSha256?: string;
  artifactSizeBytes?: number;
  reasonCode?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  nextRetryAt?: string;
}

export async function listTaskTurns(taskId: API.Int64): Promise<TaskTurn[]> {
  const response = await request<Envelope<TaskTurn[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/turns`, { method: 'GET' });
  return response.data ?? [];
}

export async function createNextTaskTurn(taskId: API.Int64, input: {
  sourceCheckpointId: API.Int64;
  expectedCurrentTurnId: API.Int64;
  instruction: string;
  idempotencyKey: string;
}): Promise<TaskTurn> {
  const response = await request<Envelope<TaskTurn>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/turns`, { method: 'POST', data: input });
  return response.data;
}

export async function approveContinuationCheckpoint(checkpointId: API.Int64, reason: string): Promise<ContinuationCheckpointDetail> {
  const response = await request<Envelope<ContinuationCheckpointDetail>>(`/api/workforce/continuation-checkpoints/${encodeURIComponent(toApiIdParam(checkpointId))}/approvals`, { method: 'POST', data: { reason } });
  return response.data;
}

export async function createResultMaterialization(taskId: API.Int64, input: {
  taskTurnId: API.Int64;
  kind: ResultMaterializationRun['kind'];
  outputId?: API.Int64;
  idempotencyKey: string;
}): Promise<ResultMaterializationRun> {
  const response = await request<Envelope<ResultMaterializationRun>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/result-materializations`, { method: 'POST', data: input });
  return response.data;
}

export async function listResultMaterializations(taskId: API.Int64): Promise<ResultMaterializationRun[]> {
  const response = await request<Envelope<ResultMaterializationRun[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/result-materializations`, { method: 'GET' });
  return response.data ?? [];
}

export async function getResultMaterialization(runId: API.Int64): Promise<ResultMaterializationRun> {
  const response = await request<Envelope<ResultMaterializationRun>>(`/api/workforce/result-materialization-runs/${encodeURIComponent(toApiIdParam(runId))}`, { method: 'GET' });
  return response.data;
}

export async function retryResultMaterialization(runId: API.Int64, reason: string): Promise<void> {
  await request(`/api/workforce/result-materialization-runs/${encodeURIComponent(toApiIdParam(runId))}/retry`, { method: 'POST', data: { reason } });
}
