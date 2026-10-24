import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

async function loadTypeScriptModule(relativePath) {
  const source = await readFile(new URL(relativePath, import.meta.url), 'utf8');
  const { outputText, diagnostics = [] } = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
    reportDiagnostics: true,
  });
  assert.deepEqual(diagnostics, []);
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`);
}

const realtimeState = await loadTypeScriptModule('../src/components/notification/inboxRealtimeState.ts');
const inboxChangeState = await loadTypeScriptModule('../src/components/notification/inboxChangeState.ts');

const record = (recipientId, archivedAt = undefined) => ({
  recipientId,
  title: `消息 ${recipientId}`,
  summary: '打开查看详情',
  archivedAt,
  mailboxVersion: '1',
  createTime: '2026-07-23T10:00:00.000Z',
  updateTime: '2026-07-23T10:00:00.000Z',
});

test('every accepted mailbox hint invalidates cached preview, while only new unread asks for a prompt', () => {
  const ordinaryChange = realtimeState.acceptInboxRealtimeHint([], {
    changeToken: 'opaque-change-2',
    newUnread: false,
  });
  assert.equal(ordinaryChange.duplicate, false);
  assert.equal(ordinaryChange.invalidatePreview, true);
  assert.equal(ordinaryChange.showNewUnreadPrompt, false);

  const newUnread = realtimeState.acceptInboxRealtimeHint(ordinaryChange.recentChangeTokens, {
    changeToken: 'opaque-change-3',
    newUnread: true,
  });
  assert.equal(newUnread.invalidatePreview, true);
  assert.equal(newUnread.showNewUnreadPrompt, true);

  const duplicate = realtimeState.acceptInboxRealtimeHint(newUnread.recentChangeTokens, {
    changeToken: 'opaque-change-3',
    newUnread: true,
  });
  assert.equal(duplicate.duplicate, true);
  assert.equal(duplicate.invalidatePreview, false);
  assert.equal(duplicate.showNewUnreadPrompt, false);
});

test('reconnect treats the current mailbox token as a quiet resync trigger', () => {
  const reconnect = realtimeState.createInboxReconnectDecision('opaque-current-change');
  assert.deepEqual(reconnect, {
    changeToken: 'opaque-current-change',
    invalidatePreview: true,
    requestDelta: true,
    showNewUnreadPrompt: false,
  });
  assert.equal(realtimeState.createInboxReconnectDecision(''), null);
});

test('an older reconnect count cannot replace a later accepted mailbox hint', () => {
  assert.equal(realtimeState.shouldApplyReconnectDecision(7, 7), true);
  assert.equal(realtimeState.shouldApplyReconnectDecision(7, 8), false);
});

test('expiry removal converges the opened list and clears an opened detail without reintroducing content', () => {
  const changed = inboxChangeState.mergeInboxRecords(
    [record('nrc-expired'), record('nrc-still-visible')],
    [],
    ['nrc-expired'],
    false,
  );
  assert.deepEqual(changed.map((item) => item.recipientId), ['nrc-still-visible']);
  assert.equal(inboxChangeState.shouldClearExpandedRecipient(
    { accountKey: 'A', recipientId: 'nrc-expired' },
    'A',
    ['nrc-expired'],
  ), true);
  assert.equal(inboxChangeState.shouldClearExpandedRecipient(
    { accountKey: 'B', recipientId: 'nrc-expired' },
    'A',
    ['nrc-expired'],
  ), false);
});
