import { request } from './request';
import { decimal, record } from './conversationContract';
import {
  normalizeExecutionOperationDetail,
  normalizeExecutionOperationReceipt,
  normalizeTaskNetworkGrantReceipt,
  normalizeTaskNetworkGrants,
  type ExecutionOperationDetail,
  type OperationDecisionAction,
  type TaskNetworkGrant,
} from './executionAccessContract';

const base = (taskId: string) => `/api/workforce/tasks/${decimal(taskId)}`;

export async function getExecutionOperation(taskId: string, operationId: string, signal?: AbortSignal): Promise<ExecutionOperationDetail> {
  const response = await request<{ data: unknown }>(`${base(taskId)}/execution-operations/${decimal(operationId)}`, { signal });
  const detail = normalizeExecutionOperationDetail(response.data);
  if (detail.taskId !== taskId || detail.id !== operationId) throw new Error('EXECUTION_OPERATION_INVALID');
  return detail;
}

export async function decideExecutionOperation(taskId: string, operationId: string, input: { expectedVersion: string; action: OperationDecisionAction; reason: string; idempotencyKey: string }) {
  const value = record(input);
  if (!['ALLOW_ONCE', 'ALLOW_FOR_TASK', 'DENY'].includes(String(value.action)) || typeof value.reason !== 'string' || typeof value.idempotencyKey !== 'string' || !value.idempotencyKey) throw new Error('EXECUTION_OPERATION_INVALID');
  const response = await request<{ data: unknown }>(`${base(taskId)}/execution-operations/${decimal(operationId)}/decisions`, { method: 'POST', data: { expectedVersion: decimal(value.expectedVersion), action: value.action, reason: value.reason, idempotencyKey: value.idempotencyKey } });
  const receipt = normalizeExecutionOperationReceipt(response.data);
  if (receipt.operationId !== operationId) throw new Error('EXECUTION_OPERATION_INVALID');
  return receipt;
}

export async function listTaskNetworkGrants(taskId: string, signal?: AbortSignal): Promise<TaskNetworkGrant[]> {
  const response = await request<{ data: unknown }>(`${base(taskId)}/network-grants`, { signal });
  const grants = normalizeTaskNetworkGrants(response.data);
  if (grants.some((grant) => grant.taskId !== taskId)) throw new Error('TASK_NETWORK_GRANT_INVALID');
  return grants;
}

export async function revokeTaskNetworkGrant(taskId: string, grantId: string, expectedVersion: string, idempotencyKey: string) {
  if (!idempotencyKey) throw new Error('TASK_NETWORK_GRANT_INVALID');
  const response = await request<{ data: unknown }>(`${base(taskId)}/network-grants/${decimal(grantId)}/revoke`, { method: 'POST', data: { expectedVersion: decimal(expectedVersion), idempotencyKey } });
  const receipt = normalizeTaskNetworkGrantReceipt(response.data);
  if (receipt.grantId !== grantId) throw new Error('TASK_NETWORK_GRANT_INVALID');
  return receipt;
}
