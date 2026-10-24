import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const providerSource = await readFile(
  new URL('../src/components/providers/InitialStateProvider.tsx', import.meta.url),
  'utf8',
);

test('unauthenticated protected deep links freeze path, query, and hash before async checks', () => {
  assert.match(providerSource, /protectedReturnTargetRef = useRef<string \| null>\(null\)/);
  assert.match(
    providerSource,
    /const protectedReturnTarget =\s*protectedReturnTargetRef\.current \?\?\s*`\$\{location\.pathname\}\$\{location\.search\}\$\{location\.hash\}`/,
  );
  assert.match(providerSource, /protectedReturnTargetRef\.current = protectedReturnTarget/);
  assert.match(
    providerSource,
    /buildLoginRedirectUrl\(protectedReturnTarget\)/,
  );
  assert.match(
    providerSource,
    /buildSetupRedirectUrl\(protectedReturnTarget\)/,
  );
  assert.equal(
    providerSource.includes('buildLoginRedirectUrl(window.location.href)'),
    false,
  );
});

test('a completed authentication clears the frozen target before another login cycle', () => {
  assert.match(
    providerSource,
    /if \(currentUser\) \{\s*protectedReturnTargetRef\.current = null;/,
  );
});
