import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const root = new URL('../', import.meta.url);

async function source(path) {
  return readFile(new URL(path, root), 'utf8');
}

test('UserSelector uses bounded non-admin selector APIs and no sensitive user fields', async () => {
  const [selector, controller] = await Promise.all([
    source('src/components/Selectors/UserSelector.tsx'),
    source('src/api/userController.ts'),
  ]);

  assert.match(selector, /import \{ getUserOptions, searchUsers \} from '@\/api\/userController'/);
  assert.doesNotMatch(selector, /\blistUsers\b|\/user\/list\/page/);
  assert.doesNotMatch(selector, /userEmail|userPhone/);
  assert.match(selector, /type SelectorValue = API\.Int64/);
  assert.match(selector, /OPTION_LIMIT = 20/);
  assert.match(controller, /getUserOptions\([\s\S]*?deptId\?: API\.Int64[\s\S]*?`\/api\/user\/options`/);
  assert.match(controller, /searchUsers\([\s\S]*?deptId\?: API\.Int64[\s\S]*?`\/api\/user\/search`/);
  assert.match(controller, /getSimpleUserById\([\s\S]*?id: API\.Int64[\s\S]*?`\/api\/user\/simple\/\$\{params\.id\}`/);
});
