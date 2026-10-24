import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const composer = await readFile(new URL('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx', import.meta.url), 'utf8');
const taskList = await readFile(new URL('../src/app/workforce/tasks/page.tsx', import.meta.url), 'utf8');

test('project-scoped task creation inherits source and accepts only task additions', () => {
  assert.match(composer, /createProjectTask\(projectId/);
  assert.match(composer, /instruction: values\.instruction\.trim\(\)/);
  assert.match(composer, /additionalCompletionCheck/);
  assert.match(composer, /reviewEnabled/);
	assert.match(composer, /listProjectRuntimeOptions/);
	assert.match(composer, /runtimeReleaseId: values\.runtimeReleaseId/);
	assert.match(composer, /queryKey: \['workforce', 'project-runtime-options', projectId, selectedCredentialId\]/);
	assert.doesNotMatch(composer, /refetchInterval/);
	assert.match(composer, /item\.recommended/);
	assert.match(composer, /刷新负载/);
	assert.match(composer, /role="radiogroup"/);
	assert.match(composer, /role="radio"/);
	assert.match(composer, /查看详情/);
	assert.match(composer, /event\.key !== 'Enter'.*event\.key !== ' '/s);
	assert.doesNotMatch(composer, /workerId.*setFieldValue|setFieldValue.*workerId/);
  assert.doesNotMatch(composer, /reviewFocus|reviewProfile|rubric/i);
  assert.match(composer, /deliveryPreference/);
  assert.doesNotMatch(composer, /TaskInputWizard|workspaceSourceRevisionId|repositoryConnectionId|providerAlias|providerModel|harnessAlias|hookBundleReleaseId|runtimeRequirements/);
});

test('global task entry selects a project instead of exposing a second create contract', () => {
  assert.match(taskList, /if \(createOnly\) return <ProjectPicker/);
  assert.match(taskList, /listProjects\(\{ status: 'ACTIVE'/);
  assert.match(taskList, /\/workforce\/projects\/\$\{encodeURIComponent\(project\.id\)\}\/tasks\/new/);
  assert.doesNotMatch(taskList, /createWorkforceTask|providerAlias|harnessAlias|resultProcessorSelection|hookSelection/);
});
