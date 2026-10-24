import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const source = await readFile(
  new URL('../src/components/notification/GroupRouteWorkspace.tsx', import.meta.url),
  'utf8',
);

test('Feishu application group discovery distinguishes provider failure from an empty group list', () => {
  assert.match(source, /candidates\.isError/);
  assert.match(source, /飞书群加载失败/);
  assert.match(source, /加载失败，请查看下方提示/);
  assert.match(source, /未发现应用可见群/);
  assert.match(source, /candidates\.refetch\(\)/);
});

test('Feishu application group discovery gives actionable provider configuration guidance', () => {
  assert.match(source, /im:chat:readonly/);
  assert.match(source, /打开飞书权限管理/);
  assert.match(source, /https:\/\/open\.feishu\.cn\/app/);
  assert.match(source, /candidateManagementUrl/);
  assert.match(source, /将应用机器人加入目标群/);
});
