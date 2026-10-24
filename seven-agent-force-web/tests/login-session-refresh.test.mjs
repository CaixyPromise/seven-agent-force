import assert from 'node:assert/strict';
import test from 'node:test';

import {
  isExpiredLoginSessionError,
  refreshLoginSession,
} from '../src/app/(auth)/login/lib/login-session.js';

test('recognizes an expired login context as a refreshable session error', () => {
  assert.equal(isExpiredLoginSessionError({
    payload: {
      code: 40900,
      message: '登录上下文已失效，请重新登录',
    },
  }), true);
  assert.equal(isExpiredLoginSessionError({
    payload: {
      code: 40300,
      message: '登录上下文无效，请重新登录',
    },
  }), true);
});

test('refreshes the transaction and context as one session before retrying', async () => {
  const calls = [];
  const session = await refreshLoginSession({
    refreshTransaction: async () => {
      calls.push('transaction');
      return 'login-txn-new';
    },
    resolveLoginOptions: async (loginTransactionId) => {
      calls.push(`context:${loginTransactionId}`);
      return { loginContextId: 'login-context-new' };
    },
  });

  assert.deepEqual(calls, ['transaction', 'context:login-txn-new']);
  assert.deepEqual(session, {
    loginTransactionId: 'login-txn-new',
    loginContextId: 'login-context-new',
  });
});
