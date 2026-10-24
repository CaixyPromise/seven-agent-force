import { request } from './request';
import { completionCheckId } from './completionCheckContract';
import { normalizeCompletionRevision, normalizeRevisionRequest, normalizeRevisionView, type RevisionRequest } from './completionRevisionContract';

const path = (taskId: API.Int64) => `/api/workforce/tasks/${encodeURIComponent(completionCheckId(taskId))}/completion-check-revisions`;
export async function getCompletionRevisionView(taskId: API.Int64) {
  const response = await request<{ data: unknown }>(path(taskId), { method: 'GET' });
  return normalizeRevisionView(response.data, taskId);
}
export async function requestCompletionRevision(taskId: API.Int64, input: RevisionRequest) {
  const response = await request<{ data: unknown }>(path(taskId), { method: 'POST', data: normalizeRevisionRequest(input) });
  return normalizeCompletionRevision(response.data, taskId);
}
