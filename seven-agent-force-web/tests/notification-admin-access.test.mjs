import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const pageSource = await readFile(
  new URL('../src/app/system/notification/page.tsx', import.meta.url),
  'utf8',
);
const menuSource = await readFile(
  new URL('../src/lib/navigation/menuRoutes.tsx', import.meta.url),
  'utf8',
);

test('each notification workspace requires its matching list permission', () => {
  for (const [flag, permission] of [
    ['channelList', 'CHANNEL_LIST'],
    ['templateList', 'TEMPLATE_LIST'],
    ['sceneList', 'SCENE_LIST'],
    ['deliveryList', 'DELIVERY_LIST'],
  ]) {
    assert.match(
      pageSource,
      new RegExp(`${flag}:\\s*NOTIFICATION_PERMISSIONS\\.${permission}`),
    );
    assert.match(pageSource, new RegExp(`permissions\\.${flag}\\s*\\?`));
  }
  assert.match(pageSource, /tabs\.length > 0/);
  assert.match(pageSource, /status="403"/);
});

test('the notification route accepts any matching list permission', () => {
  assert.match(menuSource, /path === '\/system\/notification'/);
  for (const permission of [
    'CHANNEL_LIST',
    'TEMPLATE_LIST',
    'SCENE_LIST',
    'DELIVERY_LIST',
  ]) {
    assert.match(menuSource, new RegExp(`NOTIFICATION_PERMISSIONS\\.${permission}`));
  }
  assert.match(
    menuSource,
    /permissionMatchMode: path === '\/system\/notification' \? 'any' : undefined/,
  );
});
