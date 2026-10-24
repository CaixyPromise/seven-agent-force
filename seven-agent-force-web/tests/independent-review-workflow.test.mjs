import assert from 'node:assert/strict';
import { access, readFile } from 'node:fs/promises';
import test from 'node:test';

const read = (path) => readFile(new URL(path, import.meta.url), 'utf8');
const [reviewApi, feedback, workspace, projectForm, taskComposer, detailPage, routes, normalizers] = await Promise.all([
  read('../src/api/independentReviewController.ts'),
  read('../src/app/workforce/review/reviewFeedback.ts'),
  read('../src/app/workforce/review/IndependentReviewWorkspace.tsx'),
  read('../src/app/workforce/projects/_components/ProjectForm.tsx'),
  read('../src/app/workforce/projects/[projectId]/tasks/new/page.tsx'),
  read('../src/app/workforce/tasks/[taskId]/page.tsx'),
  read('../config/route.tsx'),
  read('../src/api/reviewNormalizers.ts'),
]);

test('Independent Review keeps entity IDs lossless and decisions safe', () => {
  assert.doesNotMatch(reviewApi, /\b(?:Number|parseInt|parseFloat)\s*\(/);
  assert.match(reviewApi, /id: API\.Int64/);
  assert.match(reviewApi, /taskTurnId: API\.Int64/);
  assert.match(reviewApi, /normalizeHumanDecisionResult/);
  assert.match(feedback, /payload\?\.data\?\.reasonCode/);
  assert.doesNotMatch(feedback, /\.message/);
});

test('Project owns one review prompt and Task owns one review toggle', () => {
  assert.match(projectForm, /name="reviewPrompt"/);
  assert.match(projectForm, /独立审查提示词/);
  assert.doesNotMatch(projectForm, /review\.enabled|ReviewProfilePicker|rubric|sandbox|providerAlias/i);
  assert.match(taskComposer, /name="reviewEnabled"/);
  assert.match(taskComposer, /reviewEnabled: true/);
  assert.doesNotMatch(taskComposer, /reviewFocus|reviewPrompt|reviewProfile|rubric/i);
});

test('review policy products and routes are removed', async () => {
  for (const path of [
    '../src/api/reviewPolicyController.ts',
    '../src/app/workforce/review/profiles/page.tsx',
    '../src/app/workforce/review/rubrics/page.tsx',
  ]) {
    await assert.rejects(access(new URL(path, import.meta.url)));
  }
  assert.doesNotMatch(routes, /\/workforce\/review\/(profiles|rubrics)/);
  assert.doesNotMatch(normalizers, /ReviewProfile|ReviewRubric|rubricDimensionKeys/);
});

test('review result and human closure remain available on Task details', () => {
  assert.match(workspace, /name="reason" label="授权原因"/);
  assert.match(workspace, /name="revisionGuidance"/);
  assert.match(workspace, /revisionGuidanceRequired/);
  assert.doesNotMatch(workspace, /error\.message|response\.data\.message/);
  assert.match(detailPage, /independentReviewOwnsClosure/);
  assert.match(detailPage, /<IndependentReviewWorkspace taskId=\{task\.id\}/);
});
