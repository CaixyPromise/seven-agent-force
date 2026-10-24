import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const source = await readFile(
  new URL('../src/components/layout/index.tsx', import.meta.url),
  'utf8',
);

test('user area keeps notification and GitHub actions together without an empty action bar', () => {
  assert.equal(source.includes('InfoCircleFilled'), false);
  assert.equal(source.includes('QuestionCircleFilled'), false);
  assert.match(source, /width:\s*'100%'/);
  assert.match(source, /marginLeft:\s*collapsed\s*\?\s*0\s*:\s*'auto'/);
  assert.match(source, /<NotificationBell\s*\/>[\s\S]*?icon=\{<GithubFilled\s*\/>\}/);
  assert.match(source, /shape="circle"[\s\S]*?size="small"[\s\S]*?icon=\{<GithubFilled\s*\/>\}/);
  assert.match(source, /actionsRender=\{\(\)\s*=>\s*\[\]\}/);
});
