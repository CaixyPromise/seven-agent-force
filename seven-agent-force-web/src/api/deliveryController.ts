import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';
import { normalizeDeliveryRequest, normalizeDeliveryRequests, type DeliveryDecisionAction, type DeliveryRequest } from './deliveryContract';

export type { DeliveryRequest } from './deliveryContract';

type Envelope<T> = { data: T };

export type DeliveryTransport = 'HTTPS' | 'SSH' | 'LOCAL_BARE';
export type ApprovalPolicy = 'AFTER_APPROVAL' | 'ON_PASS' | 'MANUAL_ONLY';

export interface GitDeliveryTarget {
  id: API.Int64;
  tenantId: API.Int64;
  name: string;
  transport: DeliveryTransport;
  host?: string;
  port?: number;
  repositoryPath: string;
  credentialRef?: string;
  credentialVersion?: string;
  sshKnownHostFingerprint?: string;
  egressPolicyDigest: string;
  commitAuthorName: string;
  commitAuthorEmail: string;
  status: 'ENABLED' | 'DISABLED';
  configDigest: string;
  lastProbeStatus?: string;
  lastProbeReasonCode?: string;
  lastProbedAt?: string;
}

export type GitDeliveryTargetInput = Omit<GitDeliveryTarget, 'id' | 'tenantId' | 'configDigest' | 'lastProbeStatus' | 'lastProbeReasonCode' | 'lastProbedAt'> & { tlsPolicy?: { mode?: 'SYSTEM_STRICT' } };

export async function listGitDeliveryTargets(): Promise<GitDeliveryTarget[]> {
  const response = await request<Envelope<GitDeliveryTarget[]>>('/api/workforce/git-delivery-targets', { method: 'GET' });
  return response.data ?? [];
}
export async function createGitDeliveryTarget(input: GitDeliveryTargetInput): Promise<GitDeliveryTarget> {
  const response = await request<Envelope<GitDeliveryTarget>>('/api/workforce/git-delivery-targets', { method: 'POST', data: input });
  return response.data;
}
export async function updateGitDeliveryTarget(id: API.Int64, input: GitDeliveryTargetInput): Promise<GitDeliveryTarget> {
  const response = await request<Envelope<GitDeliveryTarget>>(`/api/workforce/git-delivery-targets/${encodeURIComponent(toApiIdParam(id))}`, { method: 'PUT', data: input });
  return response.data;
}
export async function probeGitDeliveryTarget(id: API.Int64): Promise<GitDeliveryTarget> {
  const response = await request<Envelope<GitDeliveryTarget>>(`/api/workforce/git-delivery-targets/${encodeURIComponent(toApiIdParam(id))}/probe`, { method: 'POST' });
  return response.data;
}
export async function setGitDeliveryTargetStatus(id: API.Int64, status: 'ENABLED' | 'DISABLED'): Promise<GitDeliveryTarget> {
  const response = await request<Envelope<GitDeliveryTarget>>(`/api/workforce/git-delivery-targets/${encodeURIComponent(toApiIdParam(id))}/status`, { method: 'PUT', data: { status } });
  return response.data;
}
export async function listTaskDeliveries(taskId: API.Int64): Promise<DeliveryRequest[]> {
  const response = await request<Envelope<unknown>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/deliveries`, { method: 'GET' });
  return normalizeDeliveryRequests(response.data ?? []);
}
export async function cancelDelivery(requestId: API.Int64): Promise<DeliveryRequest> {
  const response = await request<Envelope<DeliveryRequest>>(`/api/workforce/delivery-requests/${encodeURIComponent(toApiIdParam(requestId))}/cancel`, { method: 'POST' });
  return normalizeDeliveryRequest(response.data);
}
export async function retryDelivery(requestId: API.Int64): Promise<DeliveryRequest> {
  const response = await request<Envelope<DeliveryRequest>>(`/api/workforce/delivery-requests/${encodeURIComponent(toApiIdParam(requestId))}/retry`, { method: 'POST' });
  return normalizeDeliveryRequest(response.data);
}
export async function reconcileDelivery(requestId: API.Int64): Promise<DeliveryRequest> {
  const response = await request<Envelope<DeliveryRequest>>(`/api/workforce/delivery-requests/${encodeURIComponent(toApiIdParam(requestId))}/reconcile`, { method: 'POST' });
  return normalizeDeliveryRequest(response.data);
}

export async function reconcileReview(operationId: API.Int64): Promise<{ operationId: API.Int64; status: string; reasonCode?: string }> {
  const response = await request<Envelope<{ operationId: API.Int64; status: string; reasonCode?: string }>>(`/api/workforce/review-operations/${encodeURIComponent(toApiIdParam(operationId))}/reconcile`, { method: 'POST' });
  return response.data;
}
export async function reauthorizeDelivery(requestId: API.Int64, reason: string, projectVersionId?: API.Int64): Promise<DeliveryRequest> {
  const response = await request<Envelope<DeliveryRequest>>(`/api/workforce/delivery-requests/${encodeURIComponent(toApiIdParam(requestId))}/reauthorizations`, { method: 'POST', data: { reason, ...(projectVersionId ? { projectVersionId: toApiIdParam(projectVersionId) } : {}) } });
  return normalizeDeliveryRequest(response.data);
}
export async function reauthorizeReview(operationId: API.Int64, reason: string): Promise<DeliveryRequest> {
  const response = await request<Envelope<DeliveryRequest>>(`/api/workforce/review-operations/${encodeURIComponent(toApiIdParam(operationId))}/reauthorizations`, { method: 'POST', data: { reason } });
  return normalizeDeliveryRequest(response.data);
}

export async function decideTaskDelivery(taskId: API.Int64, requestId: API.Int64, input: { expectedVersion: API.Int64; action: DeliveryDecisionAction; reason?: string; idempotencyKey: string }): Promise<DeliveryRequest> {
  const response = await request<Envelope<unknown>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/delivery-requests/${encodeURIComponent(toApiIdParam(requestId))}/decisions`, {
    method: 'POST',
    data: { expectedVersion: toApiIdParam(input.expectedVersion), action: input.action, reason: input.reason?.trim() ?? '', idempotencyKey: input.idempotencyKey },
  });
  return normalizeDeliveryRequest(response.data);
}

export async function restartTaskDelivery(taskId: API.Int64, input: { resultCommit: string; idempotencyKey: string }): Promise<DeliveryRequest> {
  if (!/^[a-f0-9]{40}$/i.test(input.resultCommit)) throw new Error('DELIVERY_CONTRACT_INVALID');
  const response = await request<Envelope<unknown>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/delivery-requests`, {
    method: 'POST', data: input,
  });
  return normalizeDeliveryRequest(response.data);
}
