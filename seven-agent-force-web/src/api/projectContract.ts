export type ProjectKind = 'CODEBASE' | 'GENERAL';
export type ProjectStatus = 'ACTIVE' | 'ARCHIVED';
export type ProjectDeliveryMode = 'NONE' | 'NEW_BRANCH' | 'NEW_BRANCH_AND_DRAFT_REVIEW';
export type ProjectDeliveryAuthorization = 'AUTO_AFTER_GATES' | 'ASK_AFTER_GATES';
export type ProjectTaskDeliveryPreference = 'PROJECT_DEFAULT' | 'ASK_AFTER_GATES' | 'SAVE_ONLY';
export type ProjectNotificationEvent = 'TASK_ACTION_REQUIRED' | 'TASK_INTERRUPTED' | 'TASK_COMPLETED';
export type ProjectRepositoryProvider = 'GITHUB' | 'GITLAB' | 'GITEA' | 'GENERIC_GIT';
export type ProjectRepositorySourceState = 'PREPARING' | 'READY' | 'FAILED';

export interface ProjectRepositoryView {
  displayUrl: string;
  webUrl?: string;
  provider: ProjectRepositoryProvider;
  defaultRefName: string;
  credentialConfigured: boolean;
  sourceState: ProjectRepositorySourceState;
  capabilities: {
    read: boolean;
    push: boolean;
    draftReview: boolean;
  };
  lastCheckedAt?: string;
  reasonCode?: string;
}

export interface ProjectCompletionCheck {
  mode: 'NONE' | 'COMMAND' | 'SCRIPT';
  command?: string;
  script?: string;
  instructions?: string;
  referenceAssetIds?: API.Int64[];
}

export interface ProjectHookRelease {
  releaseId: API.Int64;
  hookId: API.Int64;
  name: string;
  version: string;
  events: string[];
  failureMode: 'BLOCK' | 'OBSERVE';
}

export interface WorkforceProject {
  id: API.Int64;
  tenantId: API.Int64;
  projectKey: string;
  name: string;
  description: string;
  departmentId?: API.Int64;
  departmentName?: string;
  kind: ProjectKind;
  status: ProjectStatus;
  versionId: API.Int64;
  repository?: ProjectRepositoryView;
  techStack: string[];
  agentGuidance: string;
  completionCheck: ProjectCompletionCheck;
  resultProcessingSummary: string;
  reviewPrompt: string;
  hookReleaseIds: API.Int64[];
  hooks: ProjectHookRelease[];
  delivery: {
    mode: ProjectDeliveryMode;
    authorization?: ProjectDeliveryAuthorization;
    resultBranchPattern?: string;
  };
  notification: {
    enabled: boolean;
    events: ProjectNotificationEvent[];
    groupRouteId?: API.Int64;
    groupRouteName?: string;
  };
  taskCount: number;
  activeTaskCount: number;
  latestTaskAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface WorkforceProjectPage {
  items: WorkforceProject[];
  nextCursor: API.Int64 | null;
}

export interface ProjectTaskSummary {
  id: API.Int64;
  projectId: API.Int64;
  name: string;
  instruction: string;
  status: string;
  pendingActionCount: number;
  resultSummary?: string;
  createdAt: string;
  finishedAt?: string;
	selectedRuntimeReleaseId?: API.Int64;
}

export interface ProjectTaskPage {
  items: ProjectTaskSummary[];
  nextCursor: API.Int64 | null;
}

export interface ProjectDraft {
  projectKey?: string;
  name: string;
  description: string;
  departmentId?: API.Int64;
  kind: ProjectKind;
  repository?: { cloneUrl?: string; defaultRefName: string };
  techStack: string[];
  agentGuidance: string;
  completionCheck: ProjectCompletionCheck;
  reviewPrompt: string;
  hookReleaseIds: API.Int64[];
  delivery: {
    mode: ProjectDeliveryMode;
    authorization?: ProjectDeliveryAuthorization;
    resultBranchPattern?: string;
  };
  notification: {
    enabled: boolean;
    events: ProjectNotificationEvent[];
    groupRouteId?: API.Int64;
  };
}

export interface ProjectTaskCreateInput {
  instruction: string;
  modelSelection: {
    credentialId: API.Int64;
    model: string;
  };
	runtimeReleaseId: API.Int64;
  executionTimeoutSeconds?: number;
  name?: string;
  tags?: string[];
  referenceAssetIds?: API.Int64[];
  additionalCompletionCheck?: ProjectCompletionCheck | null;
  reviewEnabled?: boolean;
  deliveryPreference: ProjectTaskDeliveryPreference;
}

export interface ProjectRuntimeWorkerOption {
	workerId: API.Int64;
	name: string;
	state: string;
	reservedAttempts: number;
	startingAttempts: number;
	runningAttempts: number;
	availableAttempts: number;
}

export interface ProjectRuntimeOption {
	runtimeReleaseId: API.Int64;
	name: string;
	description: string;
	harnessType: string;
	harnessVersion: string;
	adapterVersion: string;
	imageRef: string;
	imageDigest: string;
	providerProtocols: string[];
	environmentCapabilities: string[];
	executionKinds: string[];
	hookEvents: string[];
	supportsIndependentReview: boolean;
	waitingTaskCount: number;
	availableSlots: number;
	recommended: boolean;
	workers: ProjectRuntimeWorkerOption[];
}

export interface ProjectRuntimeOptions {
	projectVersionId: API.Int64;
	items: ProjectRuntimeOption[];
}

export class ProjectContractError extends TypeError {
  readonly code = 'PROJECT_CONTRACT_INVALID';
  constructor(readonly field = 'response') {
    super('PROJECT_CONTRACT_INVALID');
  }
}

const maxInt64 = '9223372036854775807';
const projectStatuses = ['ACTIVE', 'ARCHIVED'] as const;
const projectKinds = ['CODEBASE', 'GENERAL'] as const;
const deliveryModes = ['NONE', 'NEW_BRANCH', 'NEW_BRANCH_AND_DRAFT_REVIEW'] as const;
const deliveryAuthorizations = ['AUTO_AFTER_GATES', 'ASK_AFTER_GATES'] as const;
const notificationEvents = ['TASK_ACTION_REQUIRED', 'TASK_INTERRUPTED', 'TASK_COMPLETED'] as const;
const repositoryProviders = ['GITHUB', 'GITLAB', 'GITEA', 'GENERIC_GIT'] as const;
const repositorySourceStates = ['PREPARING', 'READY', 'FAILED'] as const;

function record(value: unknown, field = 'response'): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new ProjectContractError(field);
  return value as Record<string, unknown>;
}

export function projectId(value: unknown, optional = false): API.Int64 | undefined {
  if (optional && (value === undefined || value === null || value === '')) return undefined;
  if (typeof value !== 'string' || !/^[1-9]\d*$/.test(value) || value.length > 19 || (value.length === 19 && value > maxInt64)) {
    throw new ProjectContractError('identity');
  }
  return value;
}

function boundedText(value: unknown, field: string, maxBytes: number, required = false): string {
  if (value === undefined || value === null) {
    if (required) throw new ProjectContractError(field);
    return '';
  }
  if (typeof value !== 'string' || new TextEncoder().encode(value).length > maxBytes || /[\uD800-\uDFFF]/u.test(value)) {
    throw new ProjectContractError(field);
  }
  if (required && !value.trim()) throw new ProjectContractError(field);
  return value;
}

function boundedStrings(value: unknown, field: string, limit: number, itemBytes = 256): string[] {
  if (!Array.isArray(value) || value.length > limit) throw new ProjectContractError(field);
  const items = value.map((item) => boundedText(item, field, itemBytes, true).trim());
  if (new Set(items.map((item) => item.toLocaleLowerCase())).size !== items.length) throw new ProjectContractError(field);
  return items;
}

function enumValue<T extends string>(value: unknown, values: readonly T[], field: string): T {
  if (typeof value !== 'string' || !values.includes(value as T)) throw new ProjectContractError(field);
  return value as T;
}

function bool(value: unknown, field: string): boolean {
  if (typeof value !== 'boolean') throw new ProjectContractError(field);
  return value;
}

function count(value: unknown, field: string): number {
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) throw new ProjectContractError(field);
  return value;
}

function timestamp(value: unknown, field: string, optional = false): string | undefined {
  if (optional && (value === undefined || value === null || value === '')) return undefined;
  const text = boundedText(value, field, 64, true);
  if (Number.isNaN(Date.parse(text))) throw new ProjectContractError(field);
  return text;
}

export function normalizeProjectCompletionCheck(value: unknown): ProjectCompletionCheck {
  if (value === undefined || value === null) return { mode: 'NONE' };
  const item = record(value, 'completionCheck');
  const mode = enumValue(item.mode, ['NONE', 'COMMAND', 'SCRIPT'] as const, 'completionCheck.mode');
  const result: ProjectCompletionCheck = { mode };
  if (mode === 'COMMAND') result.command = boundedText(item.command, 'completionCheck.command', 8192, true);
  if (mode === 'SCRIPT') result.script = boundedText(item.script, 'completionCheck.script', 65536, true);
  if (item.instructions !== undefined) result.instructions = boundedText(item.instructions, 'completionCheck.instructions', 16384);
  if (item.referenceAssetIds !== undefined) {
    if (!Array.isArray(item.referenceAssetIds) || item.referenceAssetIds.length > 64) throw new ProjectContractError('completionCheck.referenceAssetIds');
    result.referenceAssetIds = item.referenceAssetIds.map((id) => projectId(id)!);
  }
  return result;
}

export function normalizeProject(value: unknown): WorkforceProject {
  const item = record(value);
  const kind = enumValue(item.kind, projectKinds, 'kind');
  const repositoryValue = item.repository === undefined || item.repository === null
    ? undefined
    : record(item.repository, 'repository');
  if (kind === 'CODEBASE' && !repositoryValue) throw new ProjectContractError('repository');
  if (kind === 'GENERAL' && repositoryValue) throw new ProjectContractError('repository');

  const delivery = item.delivery === undefined ? {
    mode: item.deliveryMode,
    authorization: item.deliveryAuthorization,
    resultBranchPattern: item.resultBranchPattern,
  } : record(item.delivery, 'delivery');
  const notification = item.notification === undefined ? {
    enabled: item.notificationEnabled,
    events: item.notificationEvents,
    groupRouteId: item.notificationGroupRouteId,
  } : record(item.notification, 'notification');
  const mode = enumValue(delivery.mode, deliveryModes, 'delivery.mode');
  if (kind === 'GENERAL' && mode !== 'NONE') throw new ProjectContractError('delivery.mode');
  const authorization = delivery.authorization === undefined || delivery.authorization === ''
    ? undefined
    : enumValue(delivery.authorization, deliveryAuthorizations, 'delivery.authorization');
  if (mode !== 'NONE' && !authorization) throw new ProjectContractError('delivery.authorization');
  const enabled = bool(notification.enabled, 'notification.enabled');
  const events = boundedStrings(notification.events ?? [], 'notification.events', 3).map((event) => enumValue(event, notificationEvents, 'notification.events'));
	const hookReleaseSource = item.hookReleaseIds ?? [];
	const hookSource = item.hooks ?? [];
  if (!Array.isArray(hookReleaseSource) || hookReleaseSource.length > 64 || !Array.isArray(hookSource) || hookSource.length > 64) {
    throw new ProjectContractError('hooks');
  }
  const hookReleaseIds = hookReleaseSource.map((value) => projectId(value)!);
  if (new Set(hookReleaseIds).size !== hookReleaseIds.length) throw new ProjectContractError('hookReleaseIds');
  const hooks = hookSource.map((value) => {
    const hook = record(value, 'hooks');
    return {
      releaseId: projectId(hook.releaseId)!,
      hookId: projectId(hook.hookId)!,
      name: boundedText(hook.name, 'hooks.name', 128, true),
      version: boundedText(hook.version, 'hooks.version', 64, true),
      events: boundedStrings(hook.events ?? [], 'hooks.events', 16, 128),
      failureMode: enumValue(hook.failureMode, ['BLOCK', 'OBSERVE'] as const, 'hooks.failureMode'),
    };
  });

  return {
    id: projectId(item.id)!,
    tenantId: projectId(item.tenantId)!,
    projectKey: boundedText(item.projectKey, 'projectKey', 100, true),
    name: boundedText(item.name, 'name', 200, true),
    description: boundedText(item.description, 'description', 2000),
    departmentId: projectId(item.departmentId, true),
    departmentName: item.departmentName === undefined ? undefined : boundedText(item.departmentName, 'departmentName', 200),
    kind,
    status: enumValue(item.status, projectStatuses, 'status'),
    versionId: projectId(item.versionId)!,
    repository: repositoryValue ? normalizeProjectRepository(repositoryValue) : undefined,
    techStack: boundedStrings(item.techStack ?? [], 'techStack', 32),
    agentGuidance: boundedText(item.agentGuidance, 'agentGuidance', 16384),
    completionCheck: normalizeProjectCompletionCheck(item.completionCheck),
    resultProcessingSummary: item.resultProcessingSummary === undefined ? '' : boundedText(item.resultProcessingSummary, 'resultProcessingSummary', 1000),
    reviewPrompt: boundedText(item.reviewPrompt, 'reviewPrompt', 16384),
    hookReleaseIds,
    hooks,
    delivery: {
      mode,
      authorization,
      resultBranchPattern: delivery.resultBranchPattern === undefined ? undefined : boundedText(delivery.resultBranchPattern, 'delivery.resultBranchPattern', 500),
    },
    notification: {
      enabled,
      events,
      groupRouteId: projectId(notification.groupRouteId, true),
      groupRouteName: notification.groupRouteName === undefined ? undefined : boundedText(notification.groupRouteName, 'notification.groupRouteName', 200),
    },
    taskCount: count(item.taskCount ?? 0, 'taskCount'),
    activeTaskCount: count(item.activeTaskCount ?? 0, 'activeTaskCount'),
    latestTaskAt: timestamp(item.latestTaskAt, 'latestTaskAt', true),
    createdAt: timestamp(item.createdAt, 'createdAt')!,
    updatedAt: timestamp(item.updatedAt, 'updatedAt')!,
  };
}

export function normalizeProjectPage(value: unknown): WorkforceProjectPage {
  if (Array.isArray(value)) {
    if (value.length > 200) throw new ProjectContractError('items');
    const items = value.map(normalizeProject);
    if (new Set(items.map((project) => project.id)).size !== items.length) throw new ProjectContractError('items');
    return { items, nextCursor: null };
  }
  const item = record(value);
  if (!Array.isArray(item.items) || item.items.length > 50) throw new ProjectContractError('items');
  const items = item.items.map(normalizeProject);
  if (new Set(items.map((project) => project.id)).size !== items.length) throw new ProjectContractError('items');
  return { items, nextCursor: item.nextCursor === null || item.nextCursor === undefined ? null : projectId(item.nextCursor)! };
}

export function normalizeProjectTask(value: unknown): ProjectTaskSummary {
  const item = record(value);
  const instruction = boundedText(item.instruction, 'task.instruction', 32768, true);
  const name = boundedText(item.name, 'task.name', 200).trim() || instruction.split(/\r?\n/, 1)[0].slice(0, 100);
  return {
    id: projectId(item.id)!,
    projectId: projectId(item.projectId)!,
    name,
    instruction,
    status: boundedText(item.status, 'task.status', 64, true),
    pendingActionCount: count(item.pendingActionCount ?? 0, 'task.pendingActionCount'),
    resultSummary: item.resultSummary === undefined ? undefined : boundedText(item.resultSummary, 'task.resultSummary', 4096),
    createdAt: timestamp(item.createdAt, 'task.createdAt')!,
    finishedAt: timestamp(item.finishedAt, 'task.finishedAt', true),
		selectedRuntimeReleaseId: projectId(item.selectedRuntimeReleaseId, true),
  };
}

export function normalizeProjectRuntimeOptions(value: unknown): ProjectRuntimeOptions {
	const root = record(value, 'runtimeOptions');
	if (!Array.isArray(root.items) || root.items.length > 100) throw new ProjectContractError('runtimeOptions.items');
	const items = root.items.map((raw) => {
		const item = record(raw, 'runtimeOptions.items');
		if (!Array.isArray(item.workers) || item.workers.length > 256) throw new ProjectContractError('runtimeOptions.workers');
		return {
			runtimeReleaseId: projectId(item.runtimeReleaseId)!,
			name: boundedText(item.name, 'runtimeOptions.name', 256, true),
			description: boundedText(item.description, 'runtimeOptions.description', 1000),
			harnessType: boundedText(item.harnessType, 'runtimeOptions.harnessType', 64, true),
			harnessVersion: boundedText(item.harnessVersion, 'runtimeOptions.harnessVersion', 128, true),
			adapterVersion: boundedText(item.adapterVersion, 'runtimeOptions.adapterVersion', 128),
			imageRef: boundedText(item.imageRef, 'runtimeOptions.imageRef', 1024, true),
			imageDigest: boundedText(item.imageDigest, 'runtimeOptions.imageDigest', 71, true),
			providerProtocols: boundedStrings(item.providerProtocols ?? [], 'runtimeOptions.providerProtocols', 4, 32),
			environmentCapabilities: boundedStrings(item.environmentCapabilities ?? [], 'runtimeOptions.environmentCapabilities', 64, 128),
			executionKinds: boundedStrings(item.executionKinds ?? [], 'runtimeOptions.executionKinds', 8, 64),
			hookEvents: boundedStrings(item.hookEvents ?? [], 'runtimeOptions.hookEvents', 64, 128),
			supportsIndependentReview: bool(item.supportsIndependentReview, 'runtimeOptions.supportsIndependentReview'),
			waitingTaskCount: count(item.waitingTaskCount, 'runtimeOptions.waitingTaskCount'),
			availableSlots: count(item.availableSlots, 'runtimeOptions.availableSlots'),
			recommended: bool(item.recommended, 'runtimeOptions.recommended'),
			workers: item.workers.map((rawWorker) => {
				const worker = record(rawWorker, 'runtimeOptions.workers');
				return { workerId: projectId(worker.workerId)!, name: boundedText(worker.name, 'runtimeOptions.worker.name', 256, true), state: boundedText(worker.state, 'runtimeOptions.worker.state', 32, true), reservedAttempts: count(worker.reservedAttempts, 'runtimeOptions.worker.reservedAttempts'), startingAttempts: count(worker.startingAttempts, 'runtimeOptions.worker.startingAttempts'), runningAttempts: count(worker.runningAttempts, 'runtimeOptions.worker.runningAttempts'), availableAttempts: count(worker.availableAttempts, 'runtimeOptions.worker.availableAttempts') };
			}),
		} satisfies ProjectRuntimeOption;
	});
	return { projectVersionId: projectId(root.projectVersionId)!, items };
}

export function normalizeProjectTaskPage(value: unknown): ProjectTaskPage {
  const item = record(value);
  if (!Array.isArray(item.items) || item.items.length > 50) throw new ProjectContractError('items');
  const items = item.items.map(normalizeProjectTask);
  if (new Set(items.map((task) => task.id)).size !== items.length) throw new ProjectContractError('items');
  return { items, nextCursor: item.nextCursor === null || item.nextCursor === undefined ? null : projectId(item.nextCursor)! };
}

function normalizeProjectRepository(item: Record<string, unknown>): ProjectRepositoryView {
  const capabilities = record(item.capabilities, 'repository.capabilities');
  return {
    displayUrl: boundedText(item.displayUrl, 'repository.displayUrl', 4096, true),
    webUrl: item.webUrl === undefined || item.webUrl === '' ? undefined : boundedText(item.webUrl, 'repository.webUrl', 4096),
    provider: enumValue(item.provider, repositoryProviders, 'repository.provider'),
    defaultRefName: boundedText(item.defaultRefName, 'repository.defaultRefName', 500, true),
    credentialConfigured: bool(item.credentialConfigured, 'repository.credentialConfigured'),
    sourceState: enumValue(item.sourceState, repositorySourceStates, 'repository.sourceState'),
    capabilities: {
      read: bool(capabilities.read, 'repository.capabilities.read'),
      push: bool(capabilities.push, 'repository.capabilities.push'),
      draftReview: bool(capabilities.draftReview, 'repository.capabilities.draftReview'),
    },
    lastCheckedAt: timestamp(item.lastCheckedAt, 'repository.lastCheckedAt', true),
    reasonCode: item.reasonCode === undefined || item.reasonCode === '' ? undefined : boundedText(item.reasonCode, 'repository.reasonCode', 128),
  };
}

const projectFeedbackByCode: Record<string, string> = {
  PROJECT_NOT_FOUND: '项目不存在或你无权查看。',
  PROJECT_ARCHIVED: '项目已归档，不能再发起任务。',
  PROJECT_VERSION_CONFLICT: '项目配置已更新，请刷新后重试。',
  PROJECT_CONFIGURATION_INVALID: '项目配置不完整，请检查相关字段。',
  PROJECT_REPOSITORY_URL_INVALID: 'Git 克隆地址格式不正确，请使用 HTTP(S) 地址。',
  PROJECT_REPOSITORY_CREDENTIAL_REQUIRED: '该交付方式需要在克隆地址中提供用户名和访问令牌。',
  PROJECT_REPOSITORY_AUTHENTICATION_FAILED: '仓库身份验证未通过，请更换 Git 克隆地址。',
  PROJECT_REPOSITORY_NOT_FOUND: '无法读取该仓库，请检查地址和访问权限。',
  PROJECT_REPOSITORY_REF_NOT_FOUND: '默认分支不存在，请检查分支名称。',
  PROJECT_REPOSITORY_PUSH_UNAVAILABLE: '当前凭据不能推送结果分支。',
  PROJECT_REPOSITORY_REVIEW_UNAVAILABLE: '当前仓库或凭据不支持创建草稿评审。',
  PROJECT_REPOSITORY_PREPARATION_FAILED: '仓库暂未准备完成，请稍后重试。',
  PROJECT_REPOSITORY_NOT_READY: '项目代码仍在准备，请稍后重试。',
  PROJECT_SOURCE_NOT_READY: '代码正在准备，请稍后重试。',
  PROJECT_TASK_IDEMPOTENCY_CONFLICT: '这次提交与此前请求不一致，请重新发起。',
  TASK_RUNTIME_SELECTION_REQUIRED: '请选择本次任务的执行镜像。',
  TASK_RUNTIME_SELECTION_UNAVAILABLE: '所选执行镜像已不可用，已保留填写内容，请重新选择。',
  TASK_RUNTIME_SELECTION_INCOMPATIBLE: '所选执行镜像与当前模型协议或项目 Hook 不兼容，请重新选择。',
  TASK_EXECUTION_TIMEOUT_INVALID: '请输入有效的执行时长。',
};

function projectErrorCode(error: unknown): string {
  if (!error || typeof error !== 'object') return '';
  const item = error as { reasonCode?: unknown; details?: { reasonCode?: unknown }; payload?: { data?: { reasonCode?: unknown } } };
  const code = item.reasonCode ?? item.details?.reasonCode ?? item.payload?.data?.reasonCode;
  return typeof code === 'string' && Object.hasOwn(projectFeedbackByCode, code) ? code : '';
}

export function projectFeedback(error: unknown): string {
  return projectFeedbackByCode[projectErrorCode(error)] ?? '操作未完成，已保留填写内容，请重试。';
}
