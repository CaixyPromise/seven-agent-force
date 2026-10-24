import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import test from 'node:test';
import ts from 'typescript';
const require = createRequire(import.meta.url);
const React = require('react');
const { renderToStaticMarkup } = require('react-dom/server');
const imports = { 'react/jsx-runtime': require('react/jsx-runtime'), antd: require('antd'), '@ant-design/icons': require('@ant-design/icons'), './NetworkGrantsPanel': { default: () => null } };
function component(name) {
  const js = ts.transpileModule(readFileSync(new URL(`../src/app/workforce/tasks/${name}.tsx`, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  const exports = {};
  new Function('exports', 'require', js)(exports, (name) => { assert.ok(name in imports, name); return imports[name]; });
  return exports.default;
}
const Milestones = component('TaskWorkflowMilestones');
test('review without a run distinguishes waiting execution from terminal execution', () => {
  for (const status of ['RUNNING','FAILED','TIMED_OUT','CANCELLED']) {
    const html = renderToStaticMarkup(React.createElement(Milestones, { taskStatus:status, review:{required:true,status:'PENDING_FINALIZATION'}, loading:false, onOpen() {} }));
    assert.match(html, /尚未开始/);
    assert.doesNotMatch(html, /进行中|等待评分|0 个阻断问题/);
    assert.match(html, status === 'RUNNING' ? /等待执行结果保存后审查/ : /执行已结束，未进入独立审查/);
  }
});
