import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

const source = await readFile(
  new URL('../src/components/notification/inboxPreviewState.ts', import.meta.url),
  'utf8',
);
const { outputText, diagnostics = [] } = ts.transpileModule(source, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  reportDiagnostics: true,
});
assert.deepEqual(diagnostics, []);

const previewState = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);

const preview = (recipientId, mailboxVersion = '1') => ({
  recipientId,
  title: `消息 ${recipientId}`,
  summary: '打开查看详情',
  mailboxVersion,
  createTime: '2026-07-22T12:00:00.000Z',
});

test('a successful empty preview is cached and does not loop while its generation stays current', () => {
  let cache = previewState.activateInboxPreviewAccount(previewState.emptyInboxPreviewCache(), 'A');
  const request = previewState.beginInboxPreviewLoad(cache, 'A');
  assert.equal(request.requestGeneration, 1);
  cache = previewState.resolveInboxPreviewLoad(request.next, {
    accountKey: 'A',
    requestGeneration: request.requestGeneration,
    expectedMailboxKey: null,
    mailboxKey: 'mailbox-A',
    records: [],
  });

  assert.equal(cache.previewState, 'empty');
  assert.equal(previewState.beginInboxPreviewLoad(cache, 'A').requestGeneration, null);
});

test('a new unread invalidates only its active account and a delayed A preview cannot populate B', () => {
  let cache = previewState.activateInboxPreviewAccount(previewState.emptyInboxPreviewCache(), 'A');
  const requestA = previewState.beginInboxPreviewLoad(cache, 'A');
  cache = previewState.resolveInboxPreviewLoad(requestA.next, {
    accountKey: 'A',
    requestGeneration: requestA.requestGeneration,
    expectedMailboxKey: null,
    mailboxKey: 'mailbox-A',
    records: [preview('A-1')],
  });
  cache = previewState.invalidateInboxPreview(cache, 'A');
  cache = previewState.activateInboxPreviewAccount(cache, 'B');

  const afterLateA = previewState.resolveInboxPreviewLoad(cache, {
    accountKey: 'A',
    requestGeneration: requestA.requestGeneration,
    expectedMailboxKey: 'mailbox-A',
    mailboxKey: 'mailbox-A',
    records: [preview('A-delayed')],
  });

  assert.equal(afterLateA.accountKey, 'B');
  assert.deepEqual(afterLateA.messagePopoverList, []);
  assert.equal(afterLateA.previewState, 'notLoaded');
});

test('marking the last preview item read creates one new generation that can refill the open preview', () => {
  let cache = previewState.activateInboxPreviewAccount(previewState.emptyInboxPreviewCache(), 'A');
  const request = previewState.beginInboxPreviewLoad(cache, 'A');
  cache = previewState.resolveInboxPreviewLoad(request.next, {
    accountKey: 'A',
    requestGeneration: request.requestGeneration,
    expectedMailboxKey: null,
    mailboxKey: 'mailbox-A',
    records: [preview('A-1')],
  });
  const previousGeneration = cache.requestGeneration;

  cache = previewState.removeInboxPreviewRecipient(cache, 'A', 'A-1');

  assert.equal(cache.previewState, 'notLoaded');
  assert.equal(cache.requestGeneration, previousGeneration + 1);
  assert.equal(previewState.beginInboxPreviewLoad(cache, 'A').requestGeneration, cache.requestGeneration);
});
