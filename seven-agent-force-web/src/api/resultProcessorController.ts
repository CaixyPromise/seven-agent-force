import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };

export type ResultProcessorDecision = 'PASS' | 'REVISE' | 'WAIT_USER' | 'REJECT' | 'MANUAL_REVIEW';

export interface ResultProcessorSourceFile {
  path: string;
  content: string;
  mode?: number;
}

export interface ResultProcessorOutput {
  name: string;
  path: string;
  storage: 'IMMEDIATE' | 'ON_DEMAND';
  mediaType?: string;
}

export interface ResultProcessorManifest {
  schemaVersion: 'seven.result-processor';
  name: string;
  version?: string;
  category?: string;
  runtime: { kind: 'python'; requirement?: string };
  parametersSchema?: Record<string, unknown>;
  environment?: Array<{
    name: string;
    type: 'CONFIG' | 'SECRET';
    phase: 'EVALUATE' | 'DELIVER';
    required?: boolean;
    taskOverrideAllowed?: boolean;
    defaultValue?: unknown;
    secretRef?: string;
  }>;
  maxEvaluationCycles?: number;
  continuationPolicy?: 'CHECKPOINT_REQUIRED';
  deliveryPolicy?: 'OPTIONAL' | 'DECLARED';
  capabilities?: string[];
}

export interface ResultProcessorValidation {
  valid: boolean;
  errors: string[];
  warnings: string[];
}

export interface ResultProcessorRevision {
  id: API.Int64;
  processorId: API.Int64;
  tenantId: API.Int64;
  scopeType: 'SYSTEM' | 'TENANT';
  revisionNumber: API.Int64;
  draftVersion: API.Int64;
  status: 'DRAFT' | 'PUBLISHED';
  manifest: ResultProcessorManifest;
  files: ResultProcessorSourceFile[];
  validation: ResultProcessorValidation;
  createdAt: string;
  updatedAt: string;
}

export interface ResultProcessorRelease {
  id: API.Int64;
  processorId: API.Int64;
  tenantId: API.Int64;
  scopeType: 'SYSTEM' | 'TENANT';
  revisionId: API.Int64;
  version: string;
  manifest: ResultProcessorManifest;
  files: ResultProcessorSourceFile[];
  archiveObjectKey?: string;
  archiveSha256: string;
  archiveSizeBytes: number;
  publishedAt: string;
	// These bindings belong to the current tenant even when the Release is an
	// official SYSTEM asset. SECRET values are never included; only their
	// reference/version metadata may be displayed.
	environmentBindings?: ResultProcessorEnvironmentBinding[];
}

export interface ResultProcessor {
  id: API.Int64;
  tenantId: API.Int64;
  scopeType: 'SYSTEM' | 'TENANT';
  name: string;
  category: string;
  description: string;
  defaultBound?: boolean;
  latestRelease?: ResultProcessorRelease;
  currentDraft?: ResultProcessorRevision;
  releases?: ResultProcessorRelease[];
  createdAt: string;
  updatedAt: string;
}

export interface ResultProcessorFixtureRun {
  id: API.Int64;
  tenantId: API.Int64;
  revisionId: API.Int64;
  harnessReleaseId: API.Int64;
  harnessImageDigest: string;
  evaluationContext: Record<string, unknown>;
  workspaceFiles: Array<{ path: string; content: string }>;
  configOverrides: Record<string, unknown>;
  expectedDecision?: ResultProcessorDecision;
  status: string;
  workerId?: API.Int64;
  workerInstanceId?: string;
  generation: API.Int64;
  resultContract?: Record<string, unknown>;
  resultContractSha256?: string;
  reasonCode?: string;
  diagnostics?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface ResultProcessorEnvironmentBinding {
  id: API.Int64;
  tenantId: API.Int64;
  processorReleaseId: API.Int64;
  name: string;
  type: 'CONFIG' | 'SECRET';
  phase: 'EVALUATE' | 'DELIVER';
  config?: Record<string, unknown>;
  secretRef?: string;
  secretVersion?: string;
  taskOverrideAllowed: boolean;
  networkPolicy: string;
}

export interface ResultEvaluationCycle {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  executionNodeId: API.Int64;
  attemptId: API.Int64;
  cycleNumber: number;
  sessionDigest?: string;
  candidateTreeDigest?: string;
  findingsDigest?: string;
  contractDigest: string;
  decision: ResultProcessorDecision;
  reasonCode: string;
  message?: string;
  contract: Record<string, unknown>;
  status: string;
  startedAt: string;
  persistedAt?: string;
  completedAt?: string;
}

export interface ContinuationCheckpoint {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  attemptId: API.Int64;
  checkpointIdentity: string;
  evaluationCycleId: API.Int64;
  decision: 'PASS' | 'WAIT_USER' | 'MANUAL_REVIEW';
  manifestSha256: string;
  status: string;
  expiresAt: string;
  createdAt: string;
  availableAt?: string;
}

export interface ResultOutput {
  id: API.Int64;
  taskId: API.Int64;
  taskTurnId: API.Int64;
  evaluationCycleId: API.Int64;
  name: string;
  outputKind?: 'PATH' | 'ARCHIVE';
  archiveFormat?: 'ZIP' | 'TAR_GZ';
  storage: 'IMMEDIATE' | 'ON_DEMAND';
  status: 'DECLARED' | 'FROZEN' | 'AVAILABLE';
  artifactId?: API.Int64;
  sha256?: string;
  sizeBytes?: number;
  createdAt: string;
}

const defaultManifest = (name: string): ResultProcessorManifest => ({
  schemaVersion: 'seven.result-processor',
  name,
  runtime: { kind: 'python', requirement: '>=3.11 <3.14' },
  parametersSchema: { type: 'object', properties: {}, additionalProperties: false },
  environment: [],
  maxEvaluationCycles: 3,
  continuationPolicy: 'CHECKPOINT_REQUIRED',
  deliveryPolicy: 'OPTIONAL',
  capabilities: [],
});

export function createEmptyProcessorDraft(name: string, category: string, description = '') {
  const manifest = defaultManifest(name);
  const source = [
    'from seven_workforce_runtime.processor import pass_result',
    '',
    '',
    'def process(context):',
    '    return pass_result(message="验收通过。")',
    '',
  ].join('\n');
  return { name, category, description, manifest, files: [{ path: 'processor.py', content: source, mode: 0o644 }] };
}

export async function listResultProcessors(): Promise<ResultProcessor[]> {
  const response = await request<Envelope<ResultProcessor[]>>('/api/workforce/result-processors', { method: 'GET' });
  return response.data ?? [];
}

export async function getResultProcessor(id: API.Int64): Promise<ResultProcessor> {
  const response = await request<Envelope<ResultProcessor>>(`/api/workforce/result-processors/${encodeURIComponent(toApiIdParam(id))}`, { method: 'GET' });
  return response.data;
}

export async function createResultProcessor(input: { name: string; category: string; description?: string; manifest: ResultProcessorManifest; files: ResultProcessorSourceFile[] }): Promise<ResultProcessor> {
  const response = await request<Envelope<ResultProcessor>>('/api/workforce/result-processors', { method: 'POST', data: input });
  return response.data;
}

export async function createResultProcessorRevision(processorId: API.Int64): Promise<ResultProcessorRevision> {
  const response = await request<Envelope<ResultProcessorRevision>>(`/api/workforce/result-processors/${encodeURIComponent(toApiIdParam(processorId))}/revisions`, { method: 'POST' });
  return response.data;
}

export async function saveResultProcessorRevision(revision: ResultProcessorRevision): Promise<ResultProcessorRevision> {
  const response = await request<Envelope<ResultProcessorRevision>>(`/api/workforce/result-processor-revisions/${encodeURIComponent(toApiIdParam(revision.id))}`, {
    method: 'PUT',
    data: { expectedRevision: revision.draftVersion, manifest: revision.manifest, files: revision.files },
  });
  return response.data;
}

export async function importResultProcessorArchive(revision: ResultProcessorRevision, archive: File): Promise<ResultProcessorRevision> {
  const response = await request<Envelope<ResultProcessorRevision>>(`/api/workforce/result-processor-revisions/${encodeURIComponent(toApiIdParam(revision.id))}/archive-import`, {
    method: 'POST',
    params: { expectedRevision: revision.draftVersion },
    headers: { 'Content-Type': 'application/gzip' },
    data: archive,
  });
  return response.data;
}

export async function validateResultProcessorRevision(revisionId: API.Int64): Promise<ResultProcessorValidation> {
  const response = await request<Envelope<ResultProcessorValidation>>(`/api/workforce/result-processor-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/validate`, { method: 'POST' });
  return response.data;
}

export async function createResultProcessorFixtureRun(revisionId: API.Int64, input: {
  harnessReleaseId: API.Int64;
  evaluationContext: Record<string, unknown>;
  workspaceFiles: Array<{ path: string; content: string }>;
  configOverrides?: Record<string, unknown>;
  expectedDecision?: ResultProcessorDecision;
}): Promise<{ runId: API.Int64; status: string }> {
  const response = await request<Envelope<{ runId: API.Int64; status: string }>>(`/api/workforce/result-processor-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/fixture-runs`, {
    method: 'POST', data: { ...input, configOverrides: input.configOverrides ?? {} },
  });
  return response.data;
}

export async function getResultProcessorFixtureRun(runId: API.Int64): Promise<ResultProcessorFixtureRun> {
  const response = await request<Envelope<ResultProcessorFixtureRun>>(`/api/workforce/result-processor-fixture-runs/${encodeURIComponent(toApiIdParam(runId))}`, { method: 'GET' });
  return response.data;
}

export async function publishResultProcessorRevision(revisionId: API.Int64, version: string): Promise<ResultProcessorRelease> {
  const response = await request<Envelope<ResultProcessorRelease>>(`/api/workforce/result-processor-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/publish`, { method: 'POST', data: { version } });
  return response.data;
}

export async function setDefaultResultProcessor(releaseId: API.Int64): Promise<void> {
  await request('/api/workforce/result-processor-bindings/default', { method: 'PUT', data: { processorReleaseId: releaseId } });
}

export async function setResultProcessorEnvironment(releaseId: API.Int64, input: Omit<ResultProcessorEnvironmentBinding, 'id' | 'tenantId' | 'processorReleaseId'>): Promise<ResultProcessorEnvironmentBinding> {
  const response = await request<Envelope<ResultProcessorEnvironmentBinding>>(`/api/workforce/result-processor-bindings/${encodeURIComponent(toApiIdParam(releaseId))}/environment`, { method: 'PUT', data: input });
  return response.data;
}

export async function listTaskResultEvaluations(taskId: API.Int64): Promise<ResultEvaluationCycle[]> {
  const response = await request<Envelope<ResultEvaluationCycle[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/evaluations`, { method: 'GET' });
  return response.data ?? [];
}

export async function listTaskContinuationCheckpoints(taskId: API.Int64): Promise<ContinuationCheckpoint[]> {
  const response = await request<Envelope<ContinuationCheckpoint[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/continuations`, { method: 'GET' });
  return response.data ?? [];
}

export async function listTaskResultOutputs(taskId: API.Int64): Promise<ResultOutput[]> {
  const response = await request<Envelope<ResultOutput[]>>(`/api/workforce/tasks/${encodeURIComponent(toApiIdParam(taskId))}/result-outputs`, { method: 'GET' });
  return response.data ?? [];
}
