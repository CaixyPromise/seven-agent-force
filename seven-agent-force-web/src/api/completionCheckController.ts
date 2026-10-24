import { request } from './request';
import { completionCheckId, normalizeCompletionCheckDecision, normalizeCompletionCheckDecisionResult, normalizeCompletionCheckSummary, type CompletionCheckDecision } from './completionCheckContract';

const taskPath = (taskId: API.Int64) => `/api/workforce/tasks/${encodeURIComponent(completionCheckId(taskId))}`;

export async function getTaskCompletionCheck(taskId: API.Int64) {
  const response = await request<{ data: unknown }>(`${taskPath(taskId)}/completion-check`, { method: 'GET' });
  return normalizeCompletionCheckSummary(response.data, taskId);
}

export async function decideTaskCompletionCheck(taskId: API.Int64, input: CompletionCheckDecision) {
  const data = normalizeCompletionCheckDecision(input);
  const response = await request<{ data: unknown }>(`${taskPath(taskId)}/completion-check-decisions`, { method: 'POST', data });
  return normalizeCompletionCheckDecisionResult(response.data);
}
