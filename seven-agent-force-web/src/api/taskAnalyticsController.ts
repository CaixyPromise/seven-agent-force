import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';
import type { ProjectAnalytics, ProjectListAnalytics, TaskAnalytics, WorkbenchData } from './taskAnalyticsContract';

type Envelope<T> = { data: T };

export async function getProjectListAnalytics(range: '7d' | '30d' | '90d') {
  return (await request<Envelope<ProjectListAnalytics>>('/api/workforce/projects/analytics', { method: 'GET', params: { range } })).data;
}

export async function getProjectAnalytics(projectId: API.Int64, range: '7d' | '30d' | '90d') {
  return (await request<Envelope<ProjectAnalytics>>(`/api/workforce/projects/${encodeURIComponent(toApiIdParam(projectId))}/analytics`, { method: 'GET', params: { range } })).data;
}

export async function getTaskAnalytics(range: '7d' | '14d' | '30d') {
  return (await request<Envelope<TaskAnalytics>>('/api/workforce/tasks/analytics', { method: 'GET', params: { range } })).data;
}

export async function getWorkforceWorkbench(range: '7d' | '14d' | '30d') {
  return (await request<Envelope<WorkbenchData>>('/api/workforce/workbench', { method: 'GET', params: { range } })).data;
}
