import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import ts from 'typescript';

const require = createRequire(import.meta.url);

function compile(path, imports = {}) {
  const source = readFileSync(new URL(path, import.meta.url), 'utf8');
  const js = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => {
    if (!(name in imports)) throw new Error(`Unexpected dependency: ${name}`);
    return imports[name];
  });
  return exports;
}

const contract = compile('../src/api/projectContract.ts');

test('project contract preserves identities and exposes business configuration only', () => {
  const project = contract.normalizeProject({
    id: '9007199254740993',
    tenantId: '9007199254740995',
    projectKey: 'seven-workforce',
    name: 'Seven Agent Force',
    description: 'Agent 开发平台',
    departmentId: '9007199254740997',
    kind: 'CODEBASE',
    status: 'ACTIVE',
    versionId: '7',
    repository: {
      displayUrl: 'https://github.com/CaixyPromise/seven-workforce.git',
      provider: 'GITHUB',
      defaultRefName: 'main',
      credentialConfigured: true,
      sourceState: 'READY',
      capabilities: { read: true, push: true, draftReview: true },
      lastCheckedAt: '2026-09-13T00:30:00Z',
      reasonCode: '',
      cloneUrl: 'https://user:secret@github.com/CaixyPromise/seven-workforce.git',
    },
    techStack: ['TypeScript', 'Go'],
    agentGuidance: '保持改动聚焦。',
    completionCheck: { mode: 'COMMAND', command: 'make check' },
    resultProcessingSummary: '保存代码、检查结论与可下载结果',
    reviewPrompt: '回归与权限边界',
    delivery: {
      mode: 'NEW_BRANCH_AND_DRAFT_REVIEW',
      authorization: 'AUTO_AFTER_GATES',
      resultBranchPattern: 'codex/{task-id}',
    },
    notification: {
      enabled: true,
      events: ['TASK_ACTION_REQUIRED', 'TASK_COMPLETED'],
      groupRouteId: '9007199254741001',
      groupRouteName: '研发群',
    },
    taskCount: 12,
    activeTaskCount: 2,
    createdAt: '2026-09-13T00:00:00Z',
    updatedAt: '2026-09-13T01:00:00Z',
  });

  assert.equal(project.id, '9007199254740993');
  assert.equal(project.repository.displayUrl, 'https://github.com/CaixyPromise/seven-workforce.git');
  assert.equal(project.repository.provider, 'GITHUB');
  assert.equal(project.repository.credentialConfigured, true);
  assert.deepEqual(project.repository.capabilities, { read: true, push: true, draftReview: true });
  assert.equal('cloneUrl' in project.repository, false);
  assert.equal(project.notification.groupRouteId, '9007199254741001');
  assert.deepEqual(project.techStack, ['TypeScript', 'Go']);
  assert.equal('providerAlias' in project, false);
  assert.equal('harnessAlias' in project, false);
});

test('project contract rejects lossy or malformed identity and inconsistent project kind', () => {
  assert.throws(() => contract.projectId(9007199254740993));
  assert.throws(() => contract.projectId('01'));
  assert.throws(() => contract.normalizeProject({
    id: '1', tenantId: '2', projectKey: 'general', name: 'General', description: '',
    kind: 'GENERAL', status: 'ACTIVE', versionId: '1',
    repository: { displayUrl: 'https://example.com/repo.git', provider: 'GENERIC_GIT', defaultRefName: 'main', credentialConfigured: false, sourceState: 'READY', capabilities: { read: true, push: false, draftReview: false } },
    techStack: [], agentGuidance: '', completionCheck: { mode: 'NONE' },
    resultProcessingSummary: '', reviewPrompt: '',
    delivery: { mode: 'NONE' }, notification: { enabled: false, events: [] },
    taskCount: 0, activeTaskCount: 0, pendingActionCount: 0,
    createdAt: '2026-09-13T00:00:00Z', updatedAt: '2026-09-13T00:00:00Z',
  }));
});

test('project contract rejects the deleted Repository Connection read model', () => {
  assert.throws(() => contract.normalizeProject({
    id: '21', tenantId: '22', projectKey: 'flat-project', name: 'Flat Project',
    kind: 'CODEBASE', status: 'ACTIVE', versionId: '3',
    repositoryConnectionId: '23', defaultRefName: 'main',
    techStack: ['Go'], completionCheck: null,
    reviewEnabled: false, reviewFocus: '', deliveryMode: 'NONE',
    notificationEnabled: false, notificationEvents: null,
    createdAt: '2026-09-13T00:00:00Z', updatedAt: '2026-09-13T00:00:00Z',
    resultProcessorReleaseId: '24', reviewProfileId: '25', completionCheckDigest: 'hidden',
  }));
});

test('project controller uses project-scoped endpoints and explicit idempotency', async () => {
  const calls = [];
  const request = async (path, options) => {
    calls.push({ path, options });
    if (path.includes('/tasks') && options.method === 'POST') {
      return { data: { task: { id: '11', projectId: '9', name: '修复分页', instruction: '修复分页', status: 'PENDING', pendingActionCount: 0, createdAt: '2026-09-13T00:00:00Z' }, replayed: false } };
    }
    return { data: { items: [], nextCursor: null } };
  };
  const controller = compile('../src/api/projectController.ts', {
    './request': { request },
    '@/lib/apiId': { toApiIdParam: (value) => String(value) },
    './projectContract': contract,
  });

  await controller.listProjects({ query: 'seven', status: 'ACTIVE', limit: 20 });
  await controller.listProjectTasks('9', { limit: 20 });
  const created = await controller.createProjectTask('9', {
    instruction: '修复分页',
    name: '修复分页',
    tags: ['bugfix'],
    referenceAssetIds: [],
    deliveryPreference: 'PROJECT_DEFAULT',
    reviewEnabled: true,
    modelSelection: { credentialId: '17', model: 'gpt-seven' }, runtimeReleaseId: '9007199254740993', executionTimeoutSeconds: 7200,
    providerAlias: 'must-not-leave-the-browser',
    harnessAlias: 'must-not-leave-the-browser',
  }, 'task-key-1');

  assert.equal(calls[0].path, '/api/workforce/projects');
  assert.deepEqual(calls[0].options.params, { query: 'seven', status: 'ACTIVE', limit: 20 });
  assert.equal(calls[1].path, '/api/workforce/projects/9/tasks');
  assert.equal(calls[2].path, '/api/workforce/projects/9/tasks');
  assert.equal(calls[2].options.headers['Idempotency-Key'], 'task-key-1');
  assert.equal(created.task.id, '11');
  assert.equal(created.replayed, false);
  assert.deepEqual(calls[2].options.data.modelSelection, { credentialId: '17', model: 'gpt-seven' });
	assert.equal(calls[2].options.data.runtimeReleaseId, '9007199254740993');
  assert.equal(calls[2].options.data.executionTimeoutSeconds, 7200);
  for (const forbidden of ['providerAlias', 'providerModel', 'harnessAlias', 'hookSelection', 'resultProcessorSelection', 'harnessTimeoutSeconds']) {
    assert.equal(forbidden in calls[2].options.data, false);
  }
  await controller.createProjectTask('9', {
    instruction: '不限时任务',
    modelSelection: { credentialId: '17', model: 'gpt-seven' },
    runtimeReleaseId: '9007199254740993',
  }, 'task-key-2');
  assert.equal('executionTimeoutSeconds' in calls[3].options.data, false);
});

test('runtime options use one project aggregate request and preserve decimal identities', async () => {
  const calls = [];
  const request = async (path, options) => {
    calls.push({ path, options });
    return { data: {
      projectVersionId: '9007199254740993',
      items: [{
        runtimeReleaseId: '9007199254740995', name: 'Codex Node 22', description: 'Node task image',
        harnessType: 'codex', harnessVersion: '0.147.0', adapterVersion: '0.147.0',
        imageRef: 'seven/codex@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        imageDigest: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
        providerProtocols: ['OPENAI'], environmentCapabilities: ['language.node', 'package-manager.pnpm'],
        executionKinds: ['AGENT_ATTEMPT', 'REVIEW_RUN'], hookEvents: ['codex.PreToolUse'],
        supportsIndependentReview: true, waitingTaskCount: 2, availableSlots: 1, recommended: true,
        workers: [{ workerId: '9007199254740997', name: 'worker-dev-01', state: 'ONLINE', reservedAttempts: 0, startingAttempts: 1, runningAttempts: 2, availableAttempts: 1 }],
      }],
    } };
  };
  const controller = compile('../src/api/projectController.ts', {
    './request': { request }, '@/lib/apiId': { toApiIdParam: String }, './projectContract': contract,
  });

  const result = await controller.listProjectRuntimeOptions('9007199254740991', '9007199254740989');

  assert.equal(calls.length, 1);
  assert.equal(calls[0].path, '/api/workforce/projects/9007199254740991/runtime-options');
  assert.deepEqual(calls[0].options.params, { modelCredentialId: '9007199254740989' });
  assert.equal(result.projectVersionId, '9007199254740993');
  assert.equal(result.items[0].runtimeReleaseId, '9007199254740995');
  assert.equal(result.items[0].workers[0].workerId, '9007199254740997');
});

test('project list delegates filtering and pagination to the server', async () => {
  const calls = [];
  const serverItem = {
    id: '9007199254740993', tenantId: '2', projectKey: 'backend-only', name: '后端返回项目', description: '',
    kind: 'GENERAL', status: 'ARCHIVED', versionId: '1', techStack: [],
    agentGuidance: '', completionCheck: { mode: 'NONE' }, resultProcessingSummary: '生成可读结果',
    reviewPrompt: '', delivery: { mode: 'NONE' },
    notification: { enabled: false, events: [] }, taskCount: 0, activeTaskCount: 0,
    createdAt: '2026-09-13T00:00:00Z', updatedAt: '2026-09-13T00:00:00Z',
  };
  const request = async (path, options) => {
    calls.push({ path, options });
    return { data: { items: [serverItem], nextCursor: '9007199254740000' } };
  };
  const controller = compile('../src/api/projectController.ts', {
    './request': { request },
    '@/lib/apiId': { toApiIdParam: (value) => String(value) },
    './projectContract': contract,
  });

  const page = await controller.listProjects({ query: 'seven', status: 'ACTIVE', cursor: '9007199254740999', limit: 20 });

  assert.deepEqual(calls[0].options.params, {
    query: 'seven', status: 'ACTIVE', cursor: '9007199254740999', limit: 20,
  });
  assert.equal(page.items.length, 1, 'the browser must not re-filter a server page');
  assert.equal(page.nextCursor, '9007199254740000');
});

test('project create serializes only business fields and uses the server branch contract', async () => {
  const calls = [];
  const responseProject = {
    id: '31', tenantId: '32', projectKey: 'project-31', name: 'Project 31', description: '',
    kind: 'CODEBASE', status: 'ACTIVE', versionId: '1',
    repository: { displayUrl: 'https://github.com/example/project.git', provider: 'GITHUB', defaultRefName: 'main', credentialConfigured: true, sourceState: 'PREPARING', capabilities: { read: true, push: true, draftReview: true }, lastCheckedAt: '2026-09-13T00:00:00Z', reasonCode: '' },
    techStack: [],
    completionCheck: { mode: 'NONE' }, reviewPrompt: '',
    delivery: { mode: 'NEW_BRANCH', authorization: 'AUTO_AFTER_GATES', resultBranchPattern: 'codex/{task-id}' },
    notification: { enabled: false, events: [] }, taskCount: 0, activeTaskCount: 0,
    createdAt: '2026-09-13T00:00:00Z', updatedAt: '2026-09-13T00:00:00Z',
  };
  const request = async (path, options) => {
    calls.push({ path, options });
    return { data: responseProject };
  };
  const controller = compile('../src/api/projectController.ts', {
    './request': { request },
    '@/lib/apiId': { toApiIdParam: (value) => String(value) },
    './projectContract': contract,
  });
  await controller.createProject({
    projectKey: 'project-31', name: ' Project 31 ', description: '', kind: 'CODEBASE',
    repository: { cloneUrl: ' https://user:token@github.com/example/project.git ', defaultRefName: ' main ' },
    techStack: [], agentGuidance: '', completionCheck: { mode: 'NONE' },
    reviewPrompt: '',
    delivery: { mode: 'NEW_BRANCH', authorization: 'AUTO_AFTER_GATES', resultBranchPattern: ' codex/{task-id} ' },
    notification: { enabled: false, events: [] },
    providerAlias: 'must-not-leave-the-browser', harnessTimeoutSeconds: 999,
  }, 'project-key-1');

  assert.equal(calls[0].path, '/api/workforce/projects');
  assert.equal(calls[0].options.headers['Idempotency-Key'], 'project-key-1');
  assert.equal(calls[0].options.data.name, 'Project 31');
  assert.deepEqual(Object.keys(calls[0].options.data.repository).sort(), ['cloneUrl', 'defaultRefName']);
  assert.equal(calls[0].options.data.repository.cloneUrl, 'https://user:token@github.com/example/project.git');
  assert.equal(calls[0].options.data.repository.defaultRefName, 'main');
  assert.equal(calls[0].options.data.delivery.branchPattern, 'codex/{task-id}');
  assert.equal('resultBranchPattern' in calls[0].options.data.delivery, false);
  assert.equal('providerAlias' in calls[0].options.data, false);
  assert.equal('harnessTimeoutSeconds' in calls[0].options.data, false);
});

test('project update preserves the current repository without replaying credentials', async () => {
  const calls = [];
  const request = async (path, options) => {
    calls.push({ path, options });
    return { data: {
      id: '31', tenantId: '32', projectKey: 'project-31', name: 'Project 31', description: '', kind: 'CODEBASE', status: 'ACTIVE', versionId: '2',
      repository: { displayUrl: 'https://github.com/example/project.git', provider: 'GITHUB', defaultRefName: 'develop', credentialConfigured: true, sourceState: 'READY', capabilities: { read: true, push: true, draftReview: true } },
      techStack: [], agentGuidance: '', completionCheck: { mode: 'NONE' }, resultProcessingSummary: '', reviewPrompt: '', delivery: { mode: 'NONE' }, notification: { enabled: false, events: [] }, taskCount: 0, activeTaskCount: 0,
      createdAt: '2026-09-13T00:00:00Z', updatedAt: '2026-09-13T00:00:00Z',
    } };
  };
  const controller = compile('../src/api/projectController.ts', {
    './request': { request }, '@/lib/apiId': { toApiIdParam: String }, './projectContract': contract,
  });
  await controller.updateProject('31', '1', {
    name: 'Project 31', description: '', kind: 'CODEBASE', repository: { defaultRefName: ' develop ' },
    techStack: [], agentGuidance: '', completionCheck: { mode: 'NONE' }, reviewPrompt: '', delivery: { mode: 'NONE' }, notification: { enabled: false, events: [] },
  });
  assert.deepEqual(calls[0].options.data.repository, { defaultRefName: 'develop' });
  assert.equal(JSON.stringify(calls[0]).includes('token'), false);
});

test('safe project feedback never echoes a raw backend message', () => {
  assert.equal(contract.projectFeedback({ reasonCode: 'PROJECT_ARCHIVED' }), '项目已归档，不能再发起任务。');
	assert.equal(contract.projectFeedback({ payload: { data: { reasonCode: 'TASK_RUNTIME_SELECTION_UNAVAILABLE' } } }), '所选执行镜像已不可用，已保留填写内容，请重新选择。');
  assert.equal(contract.projectFeedback({ message: 'postgres password leaked' }), '操作未完成，已保留填写内容，请重试。');
});
