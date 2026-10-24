import { request } from './request';

export interface PageResult<T> {
  records: T[];
  total: number;
  size: number;
  current: number;
}

export interface Result<T> {
  code: number;
  data: T;
  message?: string;
  success?: boolean;
}

export type DockerContainerAction = 'start' | 'stop' | 'restart' | 'delete' | 'logs' | 'stats' | 'inspect';
export type DockerComposeProjectAction = 'up' | 'down' | 'restart' | 'logs' | 'ps' | 'preview' | 'validate' | 'edit';

export interface DockerContainerPortView {
  privatePort?: number;
  publicPort?: number;
  type?: string;
  ip?: string;
}

export interface DockerContainerView {
  id: string;
  name: string;
  image: string;
  imageId: string;
  state?: string;
  status?: string;
  created?: number;
  ports?: DockerContainerPortView[];
  labels?: Record<string, string>;
  restartCount?: number;
  composeManaged?: boolean;
  composeProject?: string;
  composeService?: string;
  composeConfigFiles?: string;
  composeWorkingDir?: string;
  availableActions?: DockerContainerAction[];
  activeOperation?: DockerOperationVO;
}

export interface DockerContainerDetailView {
  container: DockerContainerView;
  inspect: Record<string, unknown>;
  composeYaml?: string;
}

export interface DockerContainerUsageView {
  id: string;
  name: string;
  image?: string;
  state?: string;
  status?: string;
}

export interface DockerImageView {
  imageId: string;
  repoTags: string[];
  repoDigests: string[];
  size?: number;
  created?: number;
  labels?: Record<string, string>;
  usedByContainerCount: number;
}

export interface DockerImageDetailView {
  imageId: string;
  repoTags: string[];
  repoDigests: string[];
  size?: number;
  created?: number;
  labels?: Record<string, string>;
  usedByContainerCount: number;
  inspect: Record<string, unknown>;
}

export interface DockerImagePullCommand {
  repository: string;
  tag?: string;
  registryId?: API.Int64;
}

export interface DockerImageTagCommand {
  sourceImage: string;
  targetRepository: string;
  targetTag: string;
}

export interface DockerImagePushCommand {
  sourceImage: string;
  targetRepository?: string;
  targetTag?: string;
  registryId?: API.Int64;
}

export interface DockerRemoteRegistryView {
  id: API.Int64;
  name: string;
  code: string;
  registryType: string;
  endpoint: string;
  apiBaseUrl?: string;
  authType: string;
  username?: string;
  tokenRealm?: string;
  tokenService?: string;
  credentialId?: API.Int64;
  namespaceWhitelistJson?: string;
  tlsEnabled?: boolean;
  insecureSkipVerify?: boolean;
  defaultRegistry?: boolean;
  status?: number;
  description?: string;
  sort?: number;
  secretConfigured?: boolean;
  secretHint?: string;
  createTime?: string;
  updateTime?: string;
}

export interface DockerRemoteRegistryCommand {
  name: string;
  code: string;
  registryType: 'REGISTRY';
  endpoint: string;
  apiBaseUrl?: string;
  authType: 'ANONYMOUS' | 'BASIC' | 'TOKEN_CHALLENGE';
  username?: string;
  password?: string;
  tokenRealm?: string;
  tokenService?: string;
  credentialId?: API.Int64;
  namespaceWhitelistJson?: string;
  tlsEnabled?: boolean;
  insecureSkipVerify?: boolean;
  defaultRegistry?: boolean;
  status?: number;
  description?: string;
  sort?: number;
}

export interface DockerRegistryConnectionTestView {
  success: boolean;
  message: string;
  serverHeader?: string;
  registryVersion?: string;
  tokenRealm?: string;
  tokenService?: string;
}

export interface DockerRemoteRepositoryView {
  repository: string;
}

export interface DockerRemoteTagsView {
  repository: string;
  tags: string[];
}

export interface DockerRemoteManifestView {
  repository: string;
  reference: string;
  digest?: string;
  mediaType?: string;
  size?: number;
  schemaVersion?: number;
  os?: string;
  architecture?: string;
  variant?: string;
  created?: string;
  layerCount?: number;
  childManifestCount?: number;
  payload?: Record<string, unknown>;
}

export interface DockerRemoteImagePullRequest {
  registryId: API.Int64;
  repository: string;
  tag: string;
}

export interface DockerKeyValueCommand {
  key?: string;
  value?: string;
}

export interface DockerPortBindingCommand {
  hostIp?: string;
  hostPort?: number;
  containerPort?: number;
  protocol?: string;
}

export interface DockerVolumeBindingCommand {
  source?: string;
  target?: string;
  type?: string;
  readOnly?: boolean;
}

export interface DockerResourceLimitCommand {
  cpus?: number;
  memoryMb?: number;
  memorySwapMb?: number;
  pidsLimit?: number;
}

export interface DockerImageStartupPreview {
  imageId: string;
  imageReference?: string;
  defaultContainerName?: string;
  defaultServiceName?: string;
  defaultProjectName?: string;
  os?: string;
  architecture?: string;
  workingDir?: string;
  user?: string;
  entrypoint?: string[];
  command?: string[];
  environment?: DockerKeyValueCommand[];
  portBindings?: DockerPortBindingCommand[];
  volumeBindings?: DockerVolumeBindingCommand[];
  labels?: DockerKeyValueCommand[];
  tty?: boolean;
  stdinOpen?: boolean;
  publishAllPorts?: boolean;
  suggestedComposeYaml?: string;
}

export interface DockerContainerCreateRequest {
  imageId?: string;
  imageReference?: string;
  containerName?: string;
  entrypoint?: string[];
  command?: string[];
  environment?: DockerKeyValueCommand[];
  portBindings?: DockerPortBindingCommand[];
  volumeBindings?: DockerVolumeBindingCommand[];
  labels?: DockerKeyValueCommand[];
  workingDir?: string;
  user?: string;
  networkMode?: string;
  privileged?: boolean;
  capAdd?: string[];
  capDrop?: string[];
  restartPolicy?: string;
  restartMaxRetryCount?: number;
  tty?: boolean;
  stdinOpen?: boolean;
  publishAllPorts?: boolean;
  autoRemove?: boolean;
  resourceLimits?: DockerResourceLimitCommand;
}

export interface DockerComposeUpRequest {
  projectName?: string;
  composeYaml: string;
}

export type DockerComposeProjectSource = 'MANAGED' | 'DISCOVERED';

export type DockerComposeProjectStatus = 'running' | 'degraded' | 'stopped' | 'unknown';

export interface DockerComposeServiceVO {
  serviceName: string;
  image?: string;
  containerCount: number;
  runningCount: number;
  exitedCount?: number;
  status: DockerComposeProjectStatus;
  ports?: DockerContainerPortView[];
  containers?: DockerContainerView[];
  warningCount?: number;
  violationCount?: number;
}

export interface DockerComposeProjectSummaryVO {
  projectId: string;
  projectName: string;
  source: DockerComposeProjectSource;
  workingDir?: string;
  configFiles?: string[];
  serviceCount: number;
  containerCount: number;
  runningCount: number;
  exitedCount: number;
  status: DockerComposeProjectStatus;
  warningCount?: number;
  violationCount?: number;
  safe?: boolean;
  lastOperationId?: API.Int64;
  lastOperationType?: string;
  lastOperationStatus?: DockerOperationStatus;
  lastOperationProgress?: number;
  lastOperationStage?: string;
  availableActions?: DockerComposeProjectAction[];
  activeOperation?: DockerOperationVO;
  createdAt?: string;
  updatedAt?: string;
}

export interface DockerComposeProjectDetailVO {
  projectId: string;
  projectName: string;
  source: DockerComposeProjectSource;
  workingDir?: string;
  configFiles?: string[];
  composeFilePath?: string;
  fileManifest?: DockerComposeProjectFileManifestView[];
  composeYaml?: string;
  normalizedYaml?: string;
  visualDraft?: DockerComposeVisualDraftView;
  services?: DockerComposeServiceVO[];
  containers?: DockerContainerView[];
  preview?: DockerPreviewVO;
  validation?: DockerComposeValidationView;
  lastOperation?: DockerOperationVO;
  activeOperation?: DockerOperationVO;
  availableActions?: DockerComposeProjectAction[];
  recentEvents?: DockerOperationEventVO[];
}

export interface DockerActionNotAllowedVO {
  targetType: string;
  targetId?: string;
  currentState?: string;
  requestedAction: string;
  availableActions?: string[];
  message: string;
}

export interface DockerComposeProjectFileManifestView {
  path: string;
  type: 'compose' | 'dockerfile' | 'extra';
  serviceName?: string;
  sizeBytes?: number;
  sha256?: string;
  updateTime?: string;
}

export interface DockerComposeProjectFileCommand {
  path: string;
  content: string;
  encoding?: 'utf-8' | 'base64';
  overwrite?: boolean;
}

export interface DockerComposeBuildFileCommand {
  serviceName: string;
  context: string;
  dockerfilePath?: string;
  dockerfileContent?: string;
  imageTag?: string;
  buildArgs?: DockerKeyValueCommand[];
  extraFiles?: DockerComposeProjectFileCommand[];
}

export interface DockerComposeProjectCreateRequest {
  projectName: string;
  workingDir?: string;
  description?: string;
  composeYaml: string;
  writeFiles?: boolean;
  overwriteExisting?: boolean;
  autoUp?: boolean;
  buildFiles?: DockerComposeBuildFileCommand[];
}

export interface DockerComposeProjectCreateResult {
  projectId: string;
  projectName: string;
  operationId?: API.Int64;
}

export interface DockerComposeImportDiscoveredRequest {
  projectId: string;
  projectName?: string;
}

export interface DockerComposeProjectUpdateRequest {
  composeYaml: string;
  validateBeforeSave?: boolean;
  writeFiles?: boolean;
  buildFiles?: DockerComposeBuildFileCommand[];
}

export interface DockerComposeWorkspaceCheckRequest {
  workingDir: string;
  createIfMissing?: boolean;
  overwriteExistingCompose?: boolean;
}

export interface DockerComposeWorkspaceCheckView {
  valid: boolean;
  exists: boolean;
  canCreate: boolean;
  canWrite: boolean;
  allowedRoot: boolean;
  composeFileExists: boolean;
  resolvedPath?: string;
  message?: string;
  warnings?: string[];
}

export interface DockerComposeUnsupportedFieldView {
  path: string;
  value?: unknown;
  reason?: string;
}

export interface DockerComposeYamlValidateRequest {
  projectName?: string;
  workingDir?: string;
  composeYaml: string;
}

export interface DockerComposeYamlValidateView {
  valid: boolean;
  message?: string;
  normalizedYaml?: string;
  services?: string[];
  networks?: string[];
  volumes?: string[];
  unsupportedFields?: DockerComposeUnsupportedFieldView[];
  warnings?: DockerPolicyViolationVO[];
  visualDraft?: DockerComposeVisualDraftView;
}

export interface DockerComposeBuilderMetadataView {
  workspaceRoots: string[];
  defaultWorkspaceRoot?: string;
  defaultFileName: string;
  maxComposeBytes: number;
  maxDockerfileBytes: number;
  maxExtraFilesBytes: number;
  allowedProjectFileSuffixes: string[];
  restartPolicies: string[];
  networkModes: string[];
  supportedServiceFields: string[];
  defaultService: {
    restart?: string;
    networkMode?: string;
  };
  healthcheckDefaults: {
    interval?: string;
    timeout?: string;
    retries?: number;
    startPeriod?: string;
  };
  resourceLimitHints: {
    cpuExamples: string[];
    memoryExamples: string[];
  };
}

export interface DockerComposeVisualDraftView {
  version?: string;
  services: DockerComposeVisualServiceView[];
  networks?: DockerComposeVisualNetworkView[];
  volumes?: DockerComposeVisualVolumeView[];
}

export interface DockerComposeVisualNetworkView {
  name?: string;
  driver?: string;
  external?: boolean;
  labels?: DockerKeyValueCommand[];
}

export interface DockerComposeVisualVolumeView {
  name?: string;
  driver?: string;
  external?: boolean;
  labels?: DockerKeyValueCommand[];
}

export interface DockerComposeVisualServiceView {
  serviceName: string;
  image?: string;
  build?: DockerComposeVisualBuildView;
  containerName?: string;
  ports?: DockerComposeVisualPortView[];
  environment?: DockerKeyValueCommand[];
  volumes?: DockerComposeVisualVolumeMountView[];
  networks?: string[];
  dependsOn?: string[];
  restart?: string;
  command?: string | string[];
  workingDir?: string;
  user?: string;
  healthcheck?: DockerComposeVisualHealthcheckView;
  resources?: DockerComposeVisualResourcesView;
  advanced?: DockerComposeVisualAdvancedView;
  unsupportedFields?: DockerComposeUnsupportedFieldView[];
}

export interface DockerComposeVisualBuildView {
  context?: string;
  dockerfile?: string;
  args?: Record<string, string>;
}

export interface DockerComposeVisualPortView {
  hostIp?: string;
  hostPort?: number | string;
  containerPort?: number | string;
  protocol?: string;
}

export interface DockerComposeVisualVolumeMountView {
  source?: string;
  target?: string;
  type?: string;
  readOnly?: boolean;
}

export interface DockerComposeVisualHealthcheckView {
  test?: string | string[];
  interval?: string;
  timeout?: string;
  retries?: number;
  startPeriod?: string;
  disable?: boolean;
}

export interface DockerComposeVisualResourcesView {
  cpus?: string;
  memory?: string;
  memoryReservation?: string;
  pidsLimit?: number;
}

export interface DockerComposeVisualAdvancedView {
  privileged?: boolean;
  networkMode?: string;
  pid?: string;
  ipc?: string;
  capAdd?: string[];
  capDrop?: string[];
}

export interface DockerfileBuildPreviewRequest {
  projectName?: string;
  workingDir?: string;
  serviceName: string;
  context: string;
  dockerfilePath?: string;
  dockerfileContent?: string;
  imageTag?: string;
  buildArgs?: DockerKeyValueCommand[];
}

export interface DockerfileBuildPreviewView {
  valid: boolean;
  message?: string;
  resolvedContext?: string;
  resolvedDockerfilePath?: string;
  imageTag?: string;
  warnings?: DockerPolicyViolationVO[];
  violations?: DockerPolicyViolationVO[];
}

export interface DockerComposePreviewWithFilesRequest {
  projectName?: string;
  workingDir?: string;
  composeYaml: string;
  buildFiles?: DockerComposeBuildFileCommand[];
}

export interface DockerComposeValidationView {
  valid: boolean;
  message?: string;
  normalizedYaml?: string;
}

export interface DockerComposeExportView {
  containerId: string;
  composeManaged?: boolean;
  composeProject?: string;
  composeService?: string;
  composeYaml?: string;
  source?: string;
}

export type DockerOperationStatus =
  | 'PENDING'
  | 'RUNNING'
  | 'SUCCEEDED'
  | 'FAILED'
  | 'CANCELLED'
  | 'TIMEOUT';

export interface DockerOperationAcceptedVO {
  operationId: API.Int64;
  operationType: string;
  targetType: string;
  targetId?: string;
  targetName?: string;
  status: DockerOperationStatus;
}

export interface DockerOperationVO extends DockerOperationAcceptedVO {
  progress: number;
  currentStage?: string;
  startedAt?: string;
  finishedAt?: string;
  timeoutAt?: string;
  actor?: {
    userId?: API.Int64;
    username?: string;
    isAdmin?: boolean;
    permissions?: string[];
  };
  retryOf?: number | string;
  errorSummary?: string;
  result?: Record<string, unknown>;
  createTime?: string;
  updateTime?: string;
}

export interface DockerOperationEventVO {
  eventId: API.Int64;
  operationId: API.Int64;
  sequence: number;
  type: 'STATE' | 'PROGRESS' | 'LOG' | 'POLICY' | 'RESULT' | 'ERROR';
  stage?: string;
  percent?: number;
  message?: string;
  payload?: Record<string, unknown>;
  occurredAt: string;
}

export interface DockerPolicyViolationVO {
  code: string;
  severity: string;
  action: 'WARN' | 'DENY' | 'ALLOW';
  message: string;
  field?: string;
  value?: string;
  remediation?: string;
}

export interface DockerPreviewVO {
  safe: boolean;
  violations?: DockerPolicyViolationVO[];
  warnings?: DockerPolicyViolationVO[];
  affectedResources?: string[];
  normalizedSpec?: unknown;
}

export interface DockerComposePreviewView {
  preview: DockerPreviewVO;
  validation: DockerComposeValidationView;
  services?: string[];
  normalizedYaml?: string;
}

export interface DockerLatestOperationView {
  operation?: DockerOperationVO;
  events?: DockerOperationEventVO[];
}

export interface DockerResourceView {
  id: string;
  name: string;
  driver?: string;
  scope?: string;
  createdAt?: string;
  labels?: Record<string, string>;
  dangling?: boolean;
  sizeBytes?: number;
  description?: string;
  internal?: boolean;
  attachable?: boolean;
  ingress?: boolean;
  ipv6?: boolean;
  mountpoint?: string;
  options?: Record<string, string>;
  containers?: Record<string, DockerNetworkContainerView>;
  inspect?: Record<string, unknown>;
}

export interface DockerNetworkContainerView {
  name?: string;
  endpointId?: string;
  macAddress?: string;
  ipv4Address?: string;
  ipv6Address?: string;
}

export interface DockerResourceKeyValueCommand {
  key?: string;
  value?: string;
}

export interface DockerNetworkCreateRequest {
  name: string;
  driver?: string;
  internal?: boolean;
  attachable?: boolean;
  enableIpv6?: boolean;
  labels?: Record<string, string>;
  options?: Record<string, string>;
}

export interface DockerNetworkConnectRequest {
  containerId: string;
  aliases?: string[];
}

export interface DockerNetworkDisconnectRequest {
  containerId: string;
  force?: boolean;
}

export interface DockerNetworkDetailView {
  resource: DockerResourceView;
  inspect: Record<string, unknown>;
  containers?: Record<string, DockerNetworkContainerView>;
  options?: Record<string, string>;
}

export interface DockerVolumeCreateRequest {
  name: string;
  driver?: string;
  labels?: Record<string, string>;
  driverOpts?: Record<string, string>;
}

export interface DockerVolumeDetailView {
  resource: DockerResourceView;
  inspect: Record<string, unknown>;
  mountpoint?: string;
  options?: Record<string, string>;
  status?: Record<string, unknown>;
}

export interface DockerResourcePrunePreview {
  count: number;
  resourceIds?: string[];
  reclaimBytes?: number;
  previewToken?: string;
  warning?: string;
}

export interface DockerDaemonConfigView {
  supported: boolean;
  supportReason?: string;
  platform?: string;
  rootless?: boolean;
  configPath?: string;
  editable?: Record<string, unknown>;
  readonly?: Record<string, unknown>;
  raw?: Record<string, unknown>;
  editableKeys?: string[];
  requiresRestart?: boolean;
}

export interface DockerDaemonConfigUpdateRequest {
  editable: Record<string, unknown>;
}

export interface DockerDaemonConfigValidateView {
  valid: boolean;
  message?: string;
  keys?: string[];
}

export interface DockerCleanupPreviewRequest {
  untilHours?: number;
}

export interface DockerCleanupApplyRequest {
  previewToken?: string;
  untilHours?: number;
}

export interface DockerCleanupPreviewVO {
  previewToken: string;
  resourceType: string;
  affectedResources?: string[];
  estimatedBytes?: number;
  warning?: string;
}

export interface DockerContainerLogsQuery {
  tail?: number;
  since?: string;
  until?: string;
  timestamps?: boolean;
  grep?: string;
  follow?: boolean;
}

function encodeRepositoryPath(repository: string) {
  return repository
    .split('/')
    .map((segment) => encodeURIComponent(segment))
    .join('/');
}

export async function getDockerContainers(params: {
  current?: number;
  size?: number;
  keyword?: string;
  state?: string;
}) {
  return request<Result<PageResult<DockerContainerView>>>('/api/admin/docker/containers', {
    method: 'GET',
    params,
  });
}

export async function getDockerContainer(id: string) {
  return request<Result<DockerContainerDetailView>>(`/api/admin/docker/containers/${id}`, {
    method: 'GET',
  });
}

export async function getDockerContainerLogs(id: string, params?: DockerContainerLogsQuery) {
  return request<Result<string>>(`/api/admin/docker/containers/${id}/logs`, {
    method: 'GET',
    params,
  });
}

export function dockerContainerLogsStreamUrl(id: string, params?: DockerContainerLogsQuery) {
  const query = new URLSearchParams();
  if (params?.tail) {
    query.set('tail', String(params.tail));
  }
  if (params?.since) {
    query.set('since', params.since);
  }
  if (params?.until) {
    query.set('until', params.until);
  }
  if (params?.timestamps !== undefined) {
    query.set('timestamps', String(params.timestamps));
  }
  if (params?.grep) {
    query.set('grep', params.grep);
  }
  if (params?.follow !== undefined) {
    query.set('follow', String(params.follow));
  }
  const suffix = query.toString();
  return `/api/admin/docker/containers/${encodeURIComponent(id)}/logs/stream${suffix ? `?${suffix}` : ''}`;
}

export function dockerContainerTerminalWsUrl(id: string, params?: { shell?: '/bin/sh' | '/bin/bash'; rows?: number; cols?: number }) {
  const query = new URLSearchParams();
  if (params?.shell) {
    query.set('shell', params.shell);
  }
  if (params?.rows) {
    query.set('rows', String(params.rows));
  }
  if (params?.cols) {
    query.set('cols', String(params.cols));
  }
  const suffix = query.toString();
  const path = `/api/admin/docker/containers/${encodeURIComponent(id)}/terminal${suffix ? `?${suffix}` : ''}`;
  if (typeof window === 'undefined') {
    return path;
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${window.location.host}${path}`;
}

export async function getDockerContainerStats(id: string) {
  return request<Result<Record<string, unknown>>>(`/api/admin/docker/containers/${id}/stats`, {
    method: 'GET',
  });
}

export async function startDockerContainer(id: string) {
  return request<Result<boolean>>(`/api/admin/docker/containers/${id}/start`, {
    method: 'POST',
  });
}

export async function stopDockerContainer(id: string) {
  return request<Result<boolean>>(`/api/admin/docker/containers/${id}/stop`, {
    method: 'POST',
  });
}

export async function restartDockerContainer(id: string) {
  return request<Result<boolean>>(`/api/admin/docker/containers/${id}/restart`, {
    method: 'POST',
  });
}

export async function deleteDockerContainer(id: string) {
  return request<Result<boolean>>(`/api/admin/docker/containers/${id}`, {
    method: 'DELETE',
  });
}

export async function exportDockerContainerCompose(id: string) {
  return request<Result<DockerOperationAcceptedVO>>(`/api/admin/docker/containers/${id}/compose-export`, {
    method: 'GET',
  });
}

export async function getLocalDockerImages(params: {
  current?: number;
  size?: number;
  keyword?: string;
}) {
  return request<Result<PageResult<DockerImageView>>>('/api/admin/docker/images/local', {
    method: 'GET',
    params,
  });
}

export async function getLocalDockerImage(id: string) {
  return request<Result<DockerImageDetailView>>(`/api/admin/docker/images/local/${id}`, {
    method: 'GET',
  });
}

export async function getLocalDockerImageContainers(id: string) {
  return request<Result<DockerContainerUsageView[]>>(`/api/admin/docker/images/local/${id}/containers`, {
    method: 'GET',
  });
}

export async function pullDockerImage(data: DockerImagePullCommand) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/images/pull', {
    method: 'POST',
    data,
  });
}

export async function tagDockerImage(data: DockerImageTagCommand) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/images/tag', {
    method: 'POST',
    data,
  });
}

export async function pushDockerImage(data: DockerImagePushCommand) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/images/push', {
    method: 'POST',
    data,
  });
}

export async function deleteLocalDockerImage(id: string) {
  return request<Result<DockerOperationAcceptedVO>>(`/api/admin/docker/images/local/${id}`, {
    method: 'DELETE',
  });
}

export async function exportLocalDockerImage(id: string) {
  return request<Result<DockerOperationAcceptedVO>>(`/api/admin/docker/images/local/${id}/export`, {
    method: 'GET',
  });
}

export async function pullRemoteDockerImage(data: DockerRemoteImagePullRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/images/remote/pull', {
    method: 'POST',
    data,
  });
}

export async function getDockerImageStartupPreview(id: string) {
  return request<Result<DockerImageStartupPreview>>(`/api/admin/docker/images/local/${id}/startup-preview`, {
    method: 'POST',
  });
}

export async function checkDockerComposeWorkspace(data: DockerComposeWorkspaceCheckRequest) {
  return request<Result<DockerComposeWorkspaceCheckView>>('/api/admin/docker/compose/workspace/check', {
    method: 'POST',
    data,
  });
}

export async function getDockerComposeBuilderMetadata() {
  return request<Result<DockerComposeBuilderMetadataView>>('/api/admin/docker/compose/builder/metadata', {
    method: 'GET',
  });
}

export async function validateDockerComposeYaml(data: DockerComposeYamlValidateRequest) {
  return request<Result<DockerComposeYamlValidateView>>('/api/admin/docker/compose/yaml/validate', {
    method: 'POST',
    data,
  });
}

export async function previewDockerfileBuild(data: DockerfileBuildPreviewRequest) {
  return request<Result<DockerfileBuildPreviewView>>('/api/admin/docker/compose/dockerfile/preview', {
    method: 'POST',
    data,
  });
}

export async function previewDockerComposeWithFiles(data: DockerComposePreviewWithFilesRequest) {
  return request<Result<DockerComposePreviewView>>('/api/admin/docker/compose/preview-with-files', {
    method: 'POST',
    data,
  });
}

export async function createDockerContainerFromImage(data: DockerContainerCreateRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/containers/create-from-image', {
    method: 'POST',
    data,
  });
}

export async function validateDockerCompose(data: DockerComposeUpRequest) {
  return request<Result<DockerComposePreviewView>>('/api/admin/docker/compose/validate', {
    method: 'POST',
    data,
  });
}

export async function upDockerCompose(data: DockerComposeUpRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/compose/up', {
    method: 'POST',
    data,
  });
}

export async function previewDockerCompose(data: DockerComposeUpRequest) {
  return request<Result<DockerComposePreviewView>>('/api/admin/docker/compose/preview', {
    method: 'POST',
    data,
  });
}

export async function getDockerComposeProjects(params: {
  current?: number;
  size?: number;
  keyword?: string;
  status?: DockerComposeProjectStatus;
}) {
  return request<Result<PageResult<DockerComposeProjectSummaryVO>>>('/api/admin/docker/compose/projects', {
    method: 'GET',
    params,
  });
}

export async function getDockerComposeProject(projectId: string) {
  return request<Result<DockerComposeProjectDetailVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}`,
    { method: 'GET' },
  );
}

export async function createDockerComposeProject(data: DockerComposeProjectCreateRequest) {
  return request<Result<DockerComposeProjectCreateResult>>('/api/admin/docker/compose/projects', {
    method: 'POST',
    data,
  });
}

export async function importDockerComposeDiscoveredProject(data: DockerComposeImportDiscoveredRequest) {
  return request<Result<DockerComposeProjectDetailVO>>('/api/admin/docker/compose/projects/import-discovered', {
    method: 'POST',
    data,
  });
}

export async function updateDockerComposeProject(projectId: string, data: DockerComposeProjectUpdateRequest) {
  return request<Result<DockerComposeProjectDetailVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/compose`,
    { method: 'PUT', data },
  );
}

export async function previewDockerComposeProject(projectId: string) {
  return request<Result<DockerComposePreviewView>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/preview`,
    { method: 'POST' },
  );
}

export async function validateDockerComposeProject(projectId: string) {
  return request<Result<DockerComposePreviewView>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/validate`,
    { method: 'POST' },
  );
}

export async function upDockerComposeProject(projectId: string) {
  return request<Result<DockerOperationAcceptedVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/up`,
    { method: 'POST' },
  );
}

export async function downDockerComposeProject(projectId: string) {
  return request<Result<DockerOperationAcceptedVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/down`,
    { method: 'POST' },
  );
}

export async function restartDockerComposeProject(projectId: string) {
  return request<Result<DockerOperationAcceptedVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/restart`,
    { method: 'POST' },
  );
}

export async function getDockerComposeProjectPS(projectId: string) {
  return request<Result<{ projectId?: string; projectName?: string; containers?: DockerContainerView[]; services?: DockerComposeServiceVO[] }>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/ps`,
    { method: 'POST' },
  );
}

export async function getDockerComposeProjectLogs(projectId: string, tail?: number) {
  return request<Result<DockerOperationAcceptedVO>>(
    `/api/admin/docker/compose/projects/${encodeURIComponent(projectId)}/logs`,
    { method: 'POST', params: { tail } },
  );
}

export async function downDockerCompose(data: DockerComposeUpRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/compose/down', {
    method: 'POST',
    data,
  });
}

export async function restartDockerCompose(data: DockerComposeUpRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/compose/restart', {
    method: 'POST',
    data,
  });
}

export async function getDockerComposePS(data: DockerComposeUpRequest) {
  return request<Result<{ projectName?: string; containers?: DockerContainerView[] }>>(
    '/api/admin/docker/compose/ps',
    { method: 'POST', data },
  );
}

export async function getDockerComposeLogs(data: DockerComposeUpRequest, tail?: number) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/compose/logs', {
    method: 'POST',
    params: { tail },
    data,
  });
}

export async function getDockerRegistries() {
  return request<Result<DockerRemoteRegistryView[]>>('/api/admin/docker/registries', {
    method: 'GET',
  });
}

export async function getDockerRegistry(id: API.Int64) {
  return request<Result<DockerRemoteRegistryView>>(`/api/admin/docker/registries/${id}`, {
    method: 'GET',
  });
}

export async function createDockerRegistry(data: DockerRemoteRegistryCommand) {
  return request<Result<number | string>>('/api/admin/docker/registries', {
    method: 'POST',
    data,
  });
}

export async function updateDockerRegistry(id: API.Int64, data: DockerRemoteRegistryCommand) {
  return request<Result<boolean>>(`/api/admin/docker/registries/${id}`, {
    method: 'PUT',
    data,
  });
}

export async function testDockerRegistry(id: API.Int64) {
  return request<Result<DockerRegistryConnectionTestView>>(`/api/admin/docker/registries/${id}/test`, {
    method: 'POST',
  });
}

export async function syncDockerRegistry(id: API.Int64) {
  return request<Result<DockerOperationAcceptedVO>>(`/api/admin/docker/registries/${id}/sync`, {
    method: 'POST',
  });
}

export async function deleteDockerRegistry(id: API.Int64) {
  return request<Result<boolean>>(`/api/admin/docker/registries/${id}`, {
    method: 'DELETE',
  });
}

export async function getDockerRepositories(id: API.Int64, params: {
  current?: number;
  size?: number;
  keyword?: string;
}) {
  return request<Result<PageResult<DockerRemoteRepositoryView>>>(`/api/admin/docker/registries/${id}/repositories`, {
    method: 'GET',
    params,
  });
}

export async function getDockerRepositoryTags(id: API.Int64, repository: string) {
  return request<Result<DockerRemoteTagsView>>(
    `/api/admin/docker/registries/${id}/repositories/${encodeRepositoryPath(repository)}/tags`,
    { method: 'GET' },
  );
}

export async function getDockerRepositoryManifest(
  id: API.Int64,
  repository: string,
  reference: string,
) {
  return request<Result<DockerRemoteManifestView>>(
    `/api/admin/docker/registries/${id}/repositories/${encodeRepositoryPath(repository)}/manifests/${encodeURIComponent(reference)}`,
    { method: 'GET' },
  );
}

export async function getDockerVolumes(params: { current?: number; size?: number; keyword?: string }) {
  return request<Result<PageResult<DockerResourceView>>>('/api/admin/docker/volumes', {
    method: 'GET',
    params,
  });
}

export async function getDockerVolume(name: string) {
  return request<Result<DockerVolumeDetailView>>(`/api/admin/docker/volumes/${encodeURIComponent(name)}`, {
    method: 'GET',
  });
}

export async function createDockerVolume(data: DockerVolumeCreateRequest) {
  return request<Result<DockerResourceView>>('/api/admin/docker/volumes', {
    method: 'POST',
    data,
  });
}

export async function deleteDockerVolume(name: string) {
  return request<Result<boolean>>(`/api/admin/docker/volumes/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
}

export async function previewDockerVolumePrune() {
  return request<Result<DockerResourcePrunePreview>>('/api/admin/docker/volumes/prune/preview', {
    method: 'POST',
  });
}

export async function applyDockerVolumePrune(data?: { previewToken?: string }) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/volumes/prune/apply', {
    method: 'POST',
    data,
  });
}

export async function getDockerNetworks(params: { current?: number; size?: number; keyword?: string }) {
  return request<Result<PageResult<DockerResourceView>>>('/api/admin/docker/networks', {
    method: 'GET',
    params,
  });
}

export async function getDockerNetwork(id: string) {
  return request<Result<DockerNetworkDetailView>>(`/api/admin/docker/networks/${encodeURIComponent(id)}`, {
    method: 'GET',
  });
}

export async function createDockerNetwork(data: DockerNetworkCreateRequest) {
  return request<Result<DockerResourceView>>('/api/admin/docker/networks', {
    method: 'POST',
    data,
  });
}

export async function deleteDockerNetwork(id: string) {
  return request<Result<boolean>>(`/api/admin/docker/networks/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  });
}

export async function connectDockerNetwork(id: string, data: DockerNetworkConnectRequest) {
  return request<Result<boolean>>(`/api/admin/docker/networks/${encodeURIComponent(id)}/connect`, {
    method: 'POST',
    data,
  });
}

export async function disconnectDockerNetwork(id: string, data: DockerNetworkDisconnectRequest) {
  return request<Result<boolean>>(`/api/admin/docker/networks/${encodeURIComponent(id)}/disconnect`, {
    method: 'POST',
    data,
  });
}

export async function previewDockerNetworkPrune() {
  return request<Result<DockerResourcePrunePreview>>('/api/admin/docker/networks/prune/preview', {
    method: 'POST',
  });
}

export async function applyDockerNetworkPrune(data?: { previewToken?: string }) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/networks/prune/apply', {
    method: 'POST',
    data,
  });
}

export async function getDockerDaemonConfig() {
  return request<Result<DockerDaemonConfigView>>('/api/admin/docker/daemon/config', {
    method: 'GET',
  });
}

export async function validateDockerDaemonConfig(data: DockerDaemonConfigUpdateRequest) {
  return request<Result<DockerDaemonConfigValidateView>>('/api/admin/docker/daemon/config/validate', {
    method: 'POST',
    data,
  });
}

export async function saveDockerDaemonConfig(data: DockerDaemonConfigUpdateRequest) {
  return request<Result<DockerDaemonConfigView>>('/api/admin/docker/daemon/config', {
    method: 'PUT',
    data,
  });
}

export async function restartDockerDaemon() {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/daemon/restart', {
    method: 'POST',
  });
}

export async function previewDockerImageCleanup(data: DockerCleanupPreviewRequest = {}) {
  return request<Result<DockerCleanupPreviewVO>>('/api/admin/docker/images/cleanup/preview', {
    method: 'POST',
    data,
  });
}

export async function applyDockerImageCleanup(data: DockerCleanupApplyRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/images/cleanup/apply', {
    method: 'POST',
    data,
  });
}

export async function previewDockerContainerCleanup(data: DockerCleanupPreviewRequest = {}) {
  return request<Result<DockerCleanupPreviewVO>>('/api/admin/docker/containers/cleanup/preview', {
    method: 'POST',
    data,
  });
}

export async function applyDockerContainerCleanup(data: DockerCleanupApplyRequest) {
  return request<Result<DockerOperationAcceptedVO>>('/api/admin/docker/containers/cleanup/apply', {
    method: 'POST',
    data,
  });
}

export async function getDockerOperations(params: {
  current?: number;
  size?: number;
  status?: DockerOperationStatus;
  operationType?: string;
}) {
  return request<Result<PageResult<DockerOperationVO>>>('/api/admin/docker/operations', {
    method: 'GET',
    params,
  });
}

export async function getDockerOperation(operationId: API.Int64) {
  return request<Result<DockerOperationVO>>(`/api/admin/docker/operations/${operationId}`, {
    method: 'GET',
  });
}

export async function getDockerOperationEvents(operationId: API.Int64, params?: {
  afterSequence?: number;
  limit?: number;
}) {
  return request<Result<DockerOperationEventVO[]>>(
    `/api/admin/docker/operations/${operationId}/events`,
    { method: 'GET', params },
  );
}

export async function getLatestDockerOperation(params: {
  targetType?: 'compose' | 'container' | 'image' | 'registry';
  targetName?: string;
  targetId?: string;
  operationType?: string;
}) {
  return request<Result<DockerLatestOperationView>>('/api/admin/docker/operations/latest', {
    method: 'GET',
    params,
  });
}

export async function cancelDockerOperation(operationId: API.Int64) {
  return request<Result<boolean>>(`/api/admin/docker/operations/${operationId}/cancel`, {
    method: 'POST',
  });
}

export async function retryDockerOperation(operationId: API.Int64) {
  return request<Result<DockerOperationAcceptedVO>>(
    `/api/admin/docker/operations/${operationId}/retry`,
    { method: 'POST' },
  );
}

export function dockerOperationStreamUrl(operationId: API.Int64, afterSequence?: number) {
  const params = afterSequence ? `?afterSequence=${afterSequence}` : '';
  return `/api/admin/docker/operations/${operationId}/stream${params}`;
}
