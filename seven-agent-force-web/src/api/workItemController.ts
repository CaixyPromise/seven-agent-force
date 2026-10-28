import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };
export type WorkPlan = {
  schema: string; title: string; summary: string; goals: string[]; nonGoals: string[];
  approach: string; affectedAreas: { path: string; reason: string }[];
  steps: { key: string; description: string }[]; acceptanceCriteria: string[]; risks: string[];
  openQuestions: { key: string; question: string; blocking: boolean; resolution?: string }[]; markdown: string;
};
export type WorkItem = {
  id: string; projectId: string; projectVersionId: string; name: string; instruction: string;
  status: string; version: string; reasonCode?: string;
  source: { kind: string; commit?: string; projectVersionId: string };
  runs: { id: string; stage: string; runNumber: number; taskId: string; status: string }[];
  currentPlan?: { id: string; revision: number; contentDigest: string; plan: WorkPlan; origin?: string };
  usage?: { calls?: string; knownTotalTokens?: string; unknownUsageInvocationCount?: string } | null;
  availableActions: string[];
};
type Selection = { modelSelection: { credentialId: string; model: string }; runtimeReleaseId: string };
export type CreateWorkItem = { instruction: string; name?: string; planSelection: Selection; implementationSelection: Selection; deliveryPreference: string; usagePolicy: { mode: 'OBSERVE' } };
const id = (value: string) => encodeURIComponent(toApiIdParam(value));
export async function createWorkItem(projectId: string, data: CreateWorkItem, key: string): Promise<WorkItem> {
  return (await request<Envelope<WorkItem>>(`/api/workforce/projects/${id(projectId)}/work-items`, { method: 'POST', headers: { 'Idempotency-Key': key }, data })).data;
}
export async function listWorkItems(projectId: string): Promise<WorkItem[]> {
  return (await request<Envelope<{ items: WorkItem[] }>>(`/api/workforce/projects/${id(projectId)}/work-items`, { method: 'GET' })).data.items;
}
export async function getWorkItem(workItemId: string): Promise<WorkItem> {
  return (await request<Envelope<WorkItem>>(`/api/workforce/work-items/${id(workItemId)}`, { method: 'GET' })).data;
}
export async function actOnWorkItem(workItemId: string, action: 'plan-decisions' | 'plan-edits' | 'plan-revisions' | 'cancel', data: unknown, key: string): Promise<WorkItem> {
  return (await request<Envelope<WorkItem>>(`/api/workforce/work-items/${id(workItemId)}/${action}`, { method: 'POST', headers: { 'Idempotency-Key': key }, data })).data;
}
