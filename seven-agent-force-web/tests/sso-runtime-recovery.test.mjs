import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import ts from 'typescript';

function runtime(fetch) {
  const source = readFileSync(new URL('../src/lib/auth/sso-runtime.ts', import.meta.url), 'utf8');
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
  const exports = {};
  new Function('exports', 'fetch', js)(exports, fetch);
  return exports;
}
const enabled = { enabled: true, frontendPrimaryEnabled: true, resourceServerEnabled: true };

test('login retries a failed runtime request after the API recovers', async () => {
  let calls = 0;
  const api = runtime(async () => ++calls === 1
    ? { ok: false, status: 503 }
    : { ok: true, json: async () => ({ data: enabled }) });
  assert.equal((await api.getSsoRuntimeConfig()).enabled, false);
  assert.equal((await api.getSsoRuntimeConfig()).frontendPrimaryEnabled, true);
  assert.equal(calls, 2);
});

test('concurrent failed bootstraps share a request and later retry', async () => {
  let calls = 0;
  const api = runtime(async () => {
    if (++calls === 1) throw new Error('API restarting');
    return { ok: true, json: async () => enabled };
  });
  const results = await Promise.all([api.getSsoRuntimeConfig(), api.getSsoRuntimeConfig()]);
  assert.deepEqual(results.map((v) => v.enabled), [false, false]);
  assert.equal(calls, 1);
  assert.equal((await api.getSsoRuntimeConfig()).enabled, true);
  assert.equal(calls, 2);
});

test('a successful disabled configuration remains authoritative and cached', async () => {
  let calls = 0;
  const api = runtime(async () => {
    calls++;
    return { ok: true, json: async () => ({ data: { enabled: false, frontendPrimaryEnabled: false } }) };
  });
  assert.equal((await api.getSsoRuntimeConfig()).enabled, false);
  assert.equal((await api.getSsoRuntimeConfig()).enabled, false);
  assert.equal(calls, 1);
});
