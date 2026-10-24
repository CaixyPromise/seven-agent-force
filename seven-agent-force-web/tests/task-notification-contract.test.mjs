import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const apiSource = await readFile(
  new URL('../src/api/taskNotificationController.ts', import.meta.url),
  'utf8',
);
const taskApiSource = await readFile(
  new URL('../src/api/projectController.ts', import.meta.url),
  'utf8',
);
const createPageSource = await readFile(
  new URL('../src/app/workforce/projects/_components/ProjectForm.tsx', import.meta.url),
  'utf8',
);
const detailPageSource = await readFile(
  new URL('../src/app/workforce/tasks/[taskId]/page.tsx', import.meta.url),
  'utf8',
);
const inboxPageSource = await readFile(
  new URL('../src/app/notifications/page.tsx', import.meta.url),
  'utf8',
);

test('Task Notification internal identities use lossless API.Int64 strings', () => {
  for (const field of [
    'id',
    'tenantId',
    'taskId',
    'subscriberUserId',
    'version',
    'taskTurnId',
    'sourceId',
  ]) {
    assert.match(apiSource, new RegExp(`\\b${field}: API\\.Int64;`), field);
  }
  assert.doesNotMatch(apiSource, /\b(?:id|tenantId|taskId|subscriberUserId|version|taskTurnId|sourceId): number;/);
  assert.doesNotMatch(apiSource, /Number\(|parseInt\(|parseFloat\(/);
  assert.match(apiSource, /notificationReceiptIdentity\?: string;/);
});

test('Project owns notification policy and task creation cannot select recipients', () => {
  assert.match(taskApiSource, /notification: input\.notification\.enabled/);
  assert.match(taskApiSource, /events: \[\.\.\.input\.notification\.events\]/);
  assert.match(taskApiSource, /groupRouteId: toApiIdParam/);
  assert.match(apiSource, /kind: 'IN_APP' \| 'EMAIL_USER' \| 'FEISHU_DIRECT' \| 'WECOM_DIRECT' \| 'GROUP_ROUTE'/);
  assert.doesNotMatch(taskApiSource, /actionAssigneeUserId|staticRouteRefs|recipientUserId/);
  assert.match(createPageSource, /项目通知/);
  assert.match(createPageSource, /需要处理/);
  assert.match(createPageSource, /任务中断/);
  assert.match(createPageSource, /任务完成/);
});

test('Task Notification create, update and subscription response share the frozen event values', () => {
  for (const event of [
    'TASK_ACTION_REQUIRED',
    'TASK_INTERRUPTED',
    'TASK_COMPLETED',
  ]) {
    const quotedEvent = new RegExp(`['\"]${event}['\"]`);
    assert.match(apiSource, quotedEvent, `API event ${event}`);
    assert.match(createPageSource, quotedEvent, `create event ${event}`);
    assert.match(detailPageSource, quotedEvent, `subscription event ${event}`);
  }

  assert.match(apiSource, /events: TaskNotificationEvent\[\];/);
  assert.match(apiSource, /channels: TaskNotificationChannel\[\];/);
  assert.doesNotMatch(
    `${apiSource}\n${taskApiSource}\n${createPageSource}\n${detailPageSource}`,
    /['\"](?:ACTION_REQUIRED|INTERRUPTED|COMPLETED)['\"]/,
  );
});

test('Task Notification record uses SUPPRESSED rather than the retired SKIPPED status', () => {
  assert.match(apiSource, /['\"]SUPPRESSED['\"]/);
  assert.match(detailPageSource, /SUPPRESSED: \{ label: '未发送'/);
  assert.doesNotMatch(`${apiSource}\n${detailPageSource}`, /['\"]SKIPPED['\"]/);
});

test('Task Detail consumes turn and focus and renders per-turn notification targets', () => {
  assert.match(detailPageSource, /parseTaskDetailLocation\(searchParams\)/);
  assert.match(detailPageSource, /data-task-turn-id=\{turn\.id\}/);
  assert.match(detailPageSource, /data-task-focus="evaluation"/);
  assert.match(detailPageSource, /data-task-focus="result"/);
  assert.match(detailPageSource, /TaskNotificationCard task=\{task\}/);
  assert.doesNotMatch(detailPageSource, /Number\([^)]*(?:turn|notification|subscription|record)/i);
});

test('Inbox reuses the shared internal-path guard and labels task actions clearly', () => {
  assert.match(inboxPageSource, /isTaskDeepLink, safeInternalPath/);
  assert.match(inboxPageSource, /isTaskDeepLink\(actionLink\) \? '查看任务' : '前往'/);
  assert.doesNotMatch(inboxPageSource, /function safeInternalPath/);
});
