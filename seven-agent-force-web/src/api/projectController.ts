import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';
import {
  normalizeProject,
  normalizeProjectPage,
  normalizeProjectTask,
  normalizeProjectTaskPage,
  normalizeProjectCompletionCheck,
	normalizeProjectRuntimeOptions,
  projectId,
  type ProjectDraft,
  type ProjectTaskCreateInput,
  type ProjectTaskPage,
	type ProjectRuntimeOptions,
  type WorkforceProject,
  type WorkforceProjectPage,
} from './projectContract';

type Envelope<T> = { data: T; replayed?: boolean };

type ProjectDraftWire = Omit<ProjectDraft, 'delivery'> & {
  delivery: {
    mode: ProjectDraft['delivery']['mode'];
    authorization?: ProjectDraft['delivery']['authorization'];
    branchPattern?: string;
  };
};

const id = (value: API.Int64) => encodeURIComponent(toApiIdParam(value));

export async function listProjects(input: {
  query?: string;
  status?: 'ACTIVE' | 'ARCHIVED';
  cursor?: API.Int64;
  limit?: number;
} = {}): Promise<WorkforceProjectPage> {
  const response = await request<Envelope<unknown>>('/api/workforce/projects', {
    method: 'GET',
    params: {
      ...(input.query?.trim() ? { query: input.query.trim() } : {}),
      ...(input.status ? { status: input.status } : {}),
      ...(input.cursor ? { cursor: toApiIdParam(input.cursor) } : {}),
      limit: input.limit ?? 20,
    },
  });
  return normalizeProjectPage(response.data);
}

export async function getProject(projectID: API.Int64): Promise<WorkforceProject> {
  const response = await request<Envelope<unknown>>(`/api/workforce/projects/${id(projectID)}`, { method: 'GET' });
  return normalizeProject(response.data);
}

export async function createProject(input: ProjectDraft, idempotencyKey = `web-project:${crypto.randomUUID()}`): Promise<WorkforceProject> {
  const response = await request<Envelope<unknown>>('/api/workforce/projects', {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    data: projectDraftPayload(input),
  });
  return normalizeProject(response.data);
}

export async function updateProject(projectID: API.Int64, expectedVersionId: API.Int64, input: ProjectDraft): Promise<WorkforceProject> {
  const response = await request<Envelope<unknown>>(`/api/workforce/projects/${id(projectID)}`, {
    method: 'PUT',
    data: { ...projectDraftPayload(input), expectedVersionId: projectId(expectedVersionId) },
  });
  return normalizeProject(response.data);
}

export async function archiveProject(projectID: API.Int64, expectedVersionId: API.Int64, idempotencyKey = `web-project-archive:${crypto.randomUUID()}`): Promise<WorkforceProject> {
  const response = await request<Envelope<unknown>>(`/api/workforce/projects/${id(projectID)}/archive`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    data: { expectedVersionId: projectId(expectedVersionId) },
  });
  return normalizeProject(response.data);
}

export async function listProjectTasks(projectID: API.Int64, input: { query?: string; view?: string; from?: string; to?: string; cursor?: API.Int64; limit?: number } = {}): Promise<ProjectTaskPage> {
  const response = await request<Envelope<unknown>>(`/api/workforce/projects/${id(projectID)}/tasks`, {
    method: 'GET',
    params: {
      ...(input.cursor ? { cursor: toApiIdParam(input.cursor) } : {}),
      ...(input.query ? { query: input.query } : {}),
      ...(input.view ? { view: input.view } : {}),
      ...(input.from ? { from: input.from } : {}),
      ...(input.to ? { to: input.to } : {}),
      limit: input.limit ?? 20,
    },
  });
  return normalizeProjectTaskPage(response.data);
}

export async function listProjectRuntimeOptions(projectID: API.Int64, modelCredentialID: API.Int64): Promise<ProjectRuntimeOptions> {
	const response = await request<Envelope<unknown>>(`/api/workforce/projects/${id(projectID)}/runtime-options`, {
		method: 'GET',
		params: { modelCredentialId: toApiIdParam(modelCredentialID) },
	});
	return normalizeProjectRuntimeOptions(response.data);
}

export async function createProjectTask(projectID: API.Int64, input: ProjectTaskCreateInput, idempotencyKey = `web-project-task:${crypto.randomUUID()}`): Promise<{ task: ReturnType<typeof normalizeProjectTask>; replayed: boolean }> {
  const response = await request<Envelope<{ task: unknown; replayed?: boolean }>>(`/api/workforce/projects/${id(projectID)}/tasks`, {
    method: 'POST',
    headers: { 'Idempotency-Key': idempotencyKey },
    data: projectTaskPayload(input),
  });
  return {
    task: normalizeProjectTask(response.data.task),
    replayed: Boolean(response.data.replayed ?? response.replayed),
  };
}

function projectDraftPayload(input: ProjectDraft): ProjectDraftWire {
  const repository = input.kind === 'CODEBASE' && input.repository ? {
    ...(input.repository.cloneUrl?.trim() ? { cloneUrl: input.repository.cloneUrl.trim() } : {}),
    defaultRefName: input.repository.defaultRefName.trim(),
  } : undefined;
  const completionCheck = normalizeProjectCompletionCheck(input.completionCheck);
  const delivery = input.kind === 'GENERAL' || input.delivery.mode === 'NONE'
    ? { mode: 'NONE' as const }
    : {
        mode: input.delivery.mode,
        authorization: input.delivery.authorization,
        branchPattern: input.delivery.resultBranchPattern?.trim(),
      };
  return {
    ...(input.projectKey?.trim() ? { projectKey: input.projectKey.trim() } : {}),
    name: input.name.trim(),
    description: input.description.trim(),
    ...(input.departmentId ? { departmentId: toApiIdParam(input.departmentId) } : {}),
    kind: input.kind,
    ...(repository ? { repository } : {}),
    techStack: input.techStack.map((item) => item.trim()).filter(Boolean),
    agentGuidance: input.agentGuidance.trim(),
    completionCheck,
    reviewPrompt: input.reviewPrompt.trim(),
    hookReleaseIds: (input.hookReleaseIds ?? []).map(toApiIdParam),
    delivery,
    notification: input.notification.enabled ? {
      enabled: true,
      events: [...input.notification.events],
      ...(input.notification.groupRouteId ? { groupRouteId: toApiIdParam(input.notification.groupRouteId) } : {}),
    } : { enabled: false, events: [] },
  };
}

function projectTaskPayload(input: ProjectTaskCreateInput): ProjectTaskCreateInput {
  return {
    instruction: input.instruction.trim(),
    modelSelection: {
      credentialId: toApiIdParam(input.modelSelection.credentialId),
      model: input.modelSelection.model.trim(),
    },
		runtimeReleaseId: toApiIdParam(input.runtimeReleaseId),
    ...(input.executionTimeoutSeconds !== undefined ? { executionTimeoutSeconds: input.executionTimeoutSeconds } : {}),
    ...(input.name?.trim() ? { name: input.name.trim() } : {}),
    tags: (input.tags ?? []).map((item) => item.trim()).filter(Boolean),
    referenceAssetIds: (input.referenceAssetIds ?? []).map(toApiIdParam),
    additionalCompletionCheck: input.additionalCompletionCheck ? normalizeProjectCompletionCheck(input.additionalCompletionCheck) : null,
    reviewEnabled: input.reviewEnabled ?? true,
    deliveryPreference: input.deliveryPreference,
  };
}
