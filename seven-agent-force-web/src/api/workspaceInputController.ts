import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };

export interface WorkspaceSourceRevision {
  id: API.Int64; sourceId: API.Int64; kind: 'GIT_REPOSITORY' | 'UPLOADED_ARCHIVE' | 'UPLOADED_DIRECTORY';
  projectId?: API.Int64; projectVersionId?: API.Int64; repositoryConfigDigest?: string; refType?: string; refName?: string; resolvedCommit?: string; resolvedTree?: string;
  sourceIdentityDigest?: string; repositoryBundleContentSha256?: string; repositoryBundleSizeBytes?: number;
  status: 'PENDING' | 'AVAILABLE' | 'FAILED'; reasonCode?: string;
}
export interface ReferenceAsset { id: API.Int64; displayName: string; contentType: string; contentSha256: string; sizeBytes: number; status: string }
export interface TaskInputView { taskId: API.Int64; taskTurnId: API.Int64; sourceKind: 'EMPTY_WORKSPACE' | 'SOURCE_REVISION'; workspaceSource: WorkspaceSourceRevision; references: ReferenceAsset[]; inputSnapshotDigest: string; repositoryBundleSizeBytes: number }
export interface GitPlanView { sourceProjectId?: API.Int64; projectId?: API.Int64; reviewBaseSourceRevisionId?: API.Int64; sourceRefType?: string; sourceRefName?: string; sourceCommit: string; reviewBaseBranch?: string; reviewBaseCommit?: string; resultBranch?: string; deliveryMode: 'NONE' | 'NEW_BRANCH' | 'NEW_BRANCH_AND_DRAFT_REVIEW'; deliveryAuthorization?: 'AUTO_AFTER_GATES' | 'ASK_AFTER_GATES'; fullProjectImport: boolean; previousDeliveryCommit?: string }

export async function createReferenceAsset(fileId: string, displayName: string): Promise<ReferenceAsset> {
  const response = await request<Envelope<ReferenceAsset>>('/api/workforce/reference-assets', { method: 'POST', data: { fileId, displayName, idempotencyKey: crypto.randomUUID() } }); return response.data;
}
export async function getTaskInput(taskId: API.Int64): Promise<TaskInputView> { const response = await request<Envelope<TaskInputView>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/input`, { method: 'GET' }); return response.data; }
export async function getTaskGitPlan(taskId: API.Int64): Promise<GitPlanView> { const response = await request<Envelope<GitPlanView>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/git-plan`, { method: 'GET' }); return response.data; }
