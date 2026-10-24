import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const source = readFileSync(
  new URL('../src/components/notification/GroupRouteWorkspace.tsx', import.meta.url),
  'utf8',
);

assert.doesNotMatch(source, /message\.success\(variables\.action === 'probe' \? '连接测试完成'/);
assert.match(source, /result\.lastProbeStatus === 'PASSED'/);
assert.match(source, /测试消息已被发送方接受，请到群内确认/);
assert.match(source, /FEISHU_GROUP_UNAVAILABLE/);
assert.match(source, /应用机器人已加入该群/);
assert.match(source, /route\.lastProbeStatus === 'FAILED'/);

console.log('group route probe feedback assertions passed');
