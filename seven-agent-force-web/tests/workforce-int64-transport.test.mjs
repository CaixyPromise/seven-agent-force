import assert from 'node:assert/strict';
import test from 'node:test';

import {
  parseJsonPreserveLargeIntegers,
  preserveLargeIntegerLiterals,
} from '../src/lib/lossless-json.mjs';

test('preserves Workforce Snowflake IDs beyond JavaScript safe integer range', () => {
  const parsed = parseJsonPreserveLargeIntegers(
    '{"taskId":9007199254740993,"attemptId":"9007199254740995","generation":"9007199254740999","ids":[9007199254740997]}',
    { 'content-type': 'application/json' },
  );
  assert.deepEqual(parsed, {
    taskId: '9007199254740993',
    attemptId: '9007199254740995',
    generation: '9007199254740999',
    ids: ['9007199254740997'],
  });
});

test('does not rewrite digits inside strings or non-JSON payloads', () => {
  assert.equal(
    preserveLargeIntegerLiterals('{"message":"id 9007199254740993"}'),
    '{"message":"id 9007199254740993"}',
  );
  assert.equal(parseJsonPreserveLargeIntegers('9007199254740993', { 'content-type': 'text/plain' }), '9007199254740993');
});

test('preserves Task Notification relation IDs while keeping the receipt opaque', () => {
  const parsed = parseJsonPreserveLargeIntegers(
    '{"id":2065424359060985001,"taskId":2065424359060984001,"taskTurnId":2065424359060984002,"sourceId":2065424359060985002,"subscriptionVersion":2065424359060985003,"factType":"TASK_ACTION_REQUIRED","status":"SUPPRESSED","notificationReceiptIdentity":"notification:receipt:opaque-9007199254740993"}',
    { 'content-type': 'application/json' },
  );
  assert.deepEqual(parsed, {
    id: '2065424359060985001',
    taskId: '2065424359060984001',
    taskTurnId: '2065424359060984002',
    sourceId: '2065424359060985002',
    subscriptionVersion: '2065424359060985003',
    factType: 'TASK_ACTION_REQUIRED',
    status: 'SUPPRESSED',
    notificationReceiptIdentity: 'notification:receipt:opaque-9007199254740993',
  });
});
