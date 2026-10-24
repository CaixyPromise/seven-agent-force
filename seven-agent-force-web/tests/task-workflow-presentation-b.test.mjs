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
const Rail = component('TaskStatusRail');
const Milestones = component('TaskWorkflowMilestones');
const rail = (status, pendingActionCount = 0) => renderToStaticMarkup(React.createElement(Rail, { task: { id: '9007199254740993', status, pendingActionCount }, deliveries: [], latestStep: '最新执行记录', onOpenPending() {}, onOpenDetails() {} }));
const review = (report) => renderToStaticMarkup(React.createElement(Milestones, { review: { required: true, ...report }, loading: false, onOpen() {} }));
test('rendered task rail identifies every durable phase and keeps delivery separate', () => {
  const cases = { CREATED: '任务已创建', QUEUED: '排队中', PREPARING: '准备执行环境', RUNNING: 'Agent 运行中', INTERACTION_SUSPENDED: '等待你回答', VALIDATING: '检查结果中', REVIEWING: '审查中', PACKAGING: '保存结果中', UPLOADING: '上传结果中', RETRY_WAIT: '等待执行重试', MANUAL_REVIEW: '等待你的决定', CANCELLING: '取消中', CANCELLED: '已取消', FAILED: '执行失败', TIMED_OUT: '已超时', SUCCEEDED: '执行已完成' };
  for (const [status, label] of Object.entries(cases)) {
    const html = rail(status);
    assert.ok(html.includes(label), status);
    if (status !== 'RUNNING') assert.doesNotMatch(html, /Agent 运行中/);
    assert.doesNotMatch(html, /已交付/);
  }
  assert.match(rail('INTERACTION_SUSPENDED', 1), /等待你回答/);
  assert.match(rail('RUNNING', 1), /等待你的决定/);
});
test('rendered review milestones distinguish AI verdict, human acceptance and current decision', () => {
  assert.match(review({ verdict: 'PASS' }), /AI 通过/);
  assert.match(review({ verdict: 'REJECT', qualityOutcome: 'HUMAN_ACCEPTED' }), /人工接受/);
  assert.match(review({ verdict: 'REVISE', status: 'MANUAL_REVIEW' }), /等待人工决定/);
  const cancelled = review({ verdict: 'PASS', status: 'CANCELLED' });
  assert.match(cancelled, /已取消/);
  assert.doesNotMatch(cancelled, /AI 通过/);
});
test('pending delivery preserves terminal execution and cancellation labels', () => {
  for (const [status,label] of Object.entries({SUCCEEDED:'执行已完成',FAILED:'执行失败',CANCELLED:'已取消',TIMED_OUT:'已超时',CANCELLING:'取消中'})) {
    const html=rail(status,1);
    assert.match(html,new RegExp(label));
    assert.doesNotMatch(html,/等待你的决定/);
    assert.match(html,/定位待办/);
  }
});
test('rail ignores a stale AI verdict when the review currently needs a human decision', () => {
  for (const [status,label] of Object.entries({MANUAL_REVIEW:'等待人工决定',CANCELLED:'审查已取消',PASSED:'AI 通过'})) {
    const html=renderToStaticMarkup(React.createElement(Rail,{task:{id:'9007199254740993',status:'SUCCEEDED'},deliveries:[],review:{required:true,status,verdict:'PASS'},latestStep:'',onOpenPending(){},onOpenDetails(){}}));
    assert.match(html,new RegExp(label));
    if(status!=='PASSED')assert.doesNotMatch(html,/AI 通过|审查通过/);
  }
});
test('human acceptance retains the original AI review verdict', () => {
  const html=renderToStaticMarkup(React.createElement(Rail,{task:{id:'9007199254740993',status:'SUCCEEDED'},deliveries:[],review:{required:true,status:'MANUAL_REVIEW',verdict:'REJECT',qualityOutcome:'HUMAN_ACCEPTED'},latestStep:'',onOpenPending(){},onOpenDetails(){}}));
  assert.match(html,/人工接受/);
  assert.doesNotMatch(html,/AI 通过|等待人工决定/);
});
