import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const root = new URL('../', import.meta.url);

async function source(path) {
  return readFile(new URL(path, root), 'utf8');
}

function between(value, start, end) {
  const startIndex = value.indexOf(start);
  const endIndex = value.indexOf(end, startIndex);
  assert.notEqual(startIndex, -1);
  assert.notEqual(endIndex, -1);
  return value.slice(startIndex, endIndex);
}

test('configuration cache reads stay pure and requests start after commit', async () => {
  const [provider, hook] = await Promise.all([
    source('src/hooks/config/ConfigClientProvider.tsx'),
    source('src/hooks/useConfigValue.ts'),
  ]);

  assert.match(provider, /const getConfig = useCallback\([\s\S]*?cacheRef\.current\.get\(`\$\{cacheIdentityRef\.current\}\|\$\{key\}`\) \?\? null/);
  assert.match(hook, /useEffect\(\(\) => \{\s*ensureConfig\(configKey\)/);
  assert.match(hook, /\[configKey, ensureConfig, version\]/);
  assert.match(provider, /mountedRef\.current = true;[\s\S]*?pendingKeysRef\.current\.size > 0[\s\S]*?scheduleBatch\(\)/);
  assert.match(provider, /if \(!mountedRef\.current\) \{\s*pendingKeysRef\.current\.clear\(\);\s*\} else \{\s*triggerUpdate\(\)/);
  assert.doesNotMatch(between(provider, 'const getConfig', 'const ensureConfig'), /scheduleBatch/);
  assert.match(provider, /const requestGeneration = generationRef\.current/);
  assert.match(provider, /generationRef\.current !== requestGeneration/);
  assert.match(provider, /const refreshAll = useCallback\(\(\) => \{\s*generationRef\.current \+= 1;/);
});

test('dictionary cache reads stay pure and requests start after commit', async () => {
  const [provider, hook] = await Promise.all([
    source('src/hooks/dict/DictClientProvider.tsx'),
    source('src/hooks/useDictValue.ts'),
  ]);

  assert.match(provider, /const getDict = useCallback\([\s\S]*?cacheRef\.current\.get\(`\$\{cacheIdentityRef\.current\}\|\$\{code\}`\) \?\? null/);
  assert.match(hook, /useEffect\(\(\) => \{\s*ensureDict\(dictCode\)/);
  assert.match(hook, /\[dictCode, ensureDict, version\]/);
  assert.match(provider, /mountedRef\.current = true;[\s\S]*?pendingCodesRef\.current\.size > 0[\s\S]*?scheduleBatch\(\)/);
  assert.match(provider, /if \(!mountedRef\.current\) \{\s*pendingCodesRef\.current\.clear\(\);\s*\} else \{\s*triggerUpdate\(\)/);
  assert.doesNotMatch(between(provider, 'const getDict', 'const ensureDict'), /scheduleBatch/);
  assert.match(provider, /const requestGeneration = generationRef\.current/);
  assert.match(provider, /generationRef\.current !== requestGeneration/);
  assert.match(provider, /const refreshAll = useCallback\(\(\) => \{\s*generationRef\.current \+= 1;/);
});
