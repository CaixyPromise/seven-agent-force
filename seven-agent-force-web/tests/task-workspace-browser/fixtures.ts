// Test-only in-memory transport. No API, Provider, Git or authentication calls.
import { normalizeConversationPage, type Answer, type ConversationItem } from '../../src/api/conversationContract';
import type { OperationDecisionAction } from '../../src/api/executionAccessContract';
import type { DeliveryDecisionAction } from '../../src/api/deliveryContract';
export const scenario = new URLSearchParams(location.search).get('scenario') ?? 'question';
const date = '2026-10-05T10:00:00Z';
const base = { id: '9007199254741050', version: '1', createdAt: date };
const question = { ...base, type: 'QUESTION', body: { interactionId: '9007199254741051', status: 'OPEN', version: '1', isBlocking: true, deliveryStatus: 'NONE', questions: [{ key: 'direction', title: '这次交付优先验证哪个操作路径？', description: '请按真实用户需要选择；你也可以补充具体约束。', kind: 'SINGLE_CHOICE', required: true, allowFreeText: true, options: [{key:'result',label:'先验证结果和报告入口'}, {key:'interaction',label:'先验证问题回答与任务补充'}] }] } };
const operation = { ...base, type: 'OPERATION', body: { operationId:'9007199254741052', version:'1', status:'WAITING_APPROVAL', purpose:'读取依赖服务提供的验收资料，用于生成任务结果', targets:[{protocol:'https',host:'documentation-with-a-long-service-name.example.test',port:443}], agentContinued:false, resultDeliveryStatus:'NONE' } };
const delivery = { ...base, type:'DELIVERY', body:{ intentId:'9007199254741053', requestId:'9007199254741054', version:'1', intentVersion:'1', requestVersion:'1', status:'WAITING_USER_DECISION', summary:'交付已保存且通过独立审查的结果', deliveryMode:'NEW_BRANCH_AND_DRAFT_REVIEW', authorization:'ASK_AFTER_GATES', repositoryName:'验收项目 / workforce-integration-contract', destinationBranch:'codex/workforce-contract-with-a-long-branch-name', resultCommit:'a'.repeat(40), allowedActions:['DELIVER','SKIP_THIS_RESULT'] } };
let watermark = 1;
let unavailable = scenario === 'unavailable';
let rejectNext = false;
let items: ConversationItem[] = normalizeConversationPage({
  items: scenario === 'empty' ? [] : [
    ...Array.from({length:12}, (_,i)=>({id:String(100+i),version:'1',createdAt:date,type:i%2?'USER_MESSAGE':'ASSISTANT_MESSAGE',body:{text:i%2?'请保留已有结果与审查记录，按照当前要求继续。':'正在核对执行记录与可交付结果。完成后可通过结果与报告入口查看。'}})),
    ...(scenario === 'question' ? [question] : scenario === 'operation' ? [operation] : scenario === 'delivery' ? [delivery] : [{...base,type:'RESULT',body:{text:scenario==='failed'?'当前执行未能完成，已保存的记录可以在详情和报告中查看。':'结果已保存，可以查看完成检查和独立审查报告。'}}]),
  ], nextCursor:null, watermark:'1', allowedActions:[]
}).items;
const watchers = new Set<()=>Promise<void>>();
function changed(){watermark++;for(const cb of watchers)void cb().catch(()=>undefined);}
function guarded(){if(rejectNext){rejectNext=false;throw {payload:{data:{reasonCode:'INTERACTION_UNAVAILABLE'}}};}}
export function toggleFailure(){unavailable=!unavailable;changed();}
export function failNext(){rejectNext=true;}
export function closeQuestion(){items=items.map(i=>i.type==='QUESTION'?{...i,version:'2',body:{...i.body,version:'2',status:'CANCELLED'}}:i);changed();}
export function fixturePendingCount(){return scenario==='review'?1:items.filter(i=>(i.type==='QUESTION'&&i.body.status==='OPEN')||(i.type==='OPERATION'&&i.body.status==='WAITING_APPROVAL')||(i.type==='DELIVERY'&&i.body.status==='WAITING_USER_DECISION')).length;}
export function fixtureTaskStatus(){return scenario==='question'&&fixturePendingCount()>0?'INTERACTION_SUSPENDED':scenario==='review'?'MANUAL_REVIEW':['success','delivery'].includes(scenario)?'SUCCEEDED':scenario==='failed'?'FAILED':'RUNNING';}
export function observeFixture(cb:()=>void){const listener=async()=>cb();watchers.add(listener);return()=>{watchers.delete(listener);};}
export async function getConversation(){if(unavailable)throw Error('fixture transport unavailable');return normalizeConversationPage({items:structuredClone(items),nextCursor:null,watermark:String(watermark),allowedActions:['SUCCEEDED','FAILED'].includes(fixtureTaskStatus())?(scenario==='delivery'?['DECIDE_DELIVERY']:[]):['SEND_MESSAGE','ANSWER','DECIDE_OPERATION','DECIDE_DELIVERY']});}
export async function getConversationItem(_task:string,id:string){const item=items.find(i=>i.id===id);if(!item)throw Error('fixture item missing');return structuredClone(item);}
export async function markConversationRead(){return;}
export function watchConversation(_task:string,refresh:()=>Promise<void>){watchers.add(refresh);return()=>watchers.delete(refresh);}
export async function sendConversationMessage(_task:string,text:string){guarded();items=[...items,{id:(BigInt('9007199254741056')+BigInt(watermark)).toString(),version:'1',createdAt:date,type:'USER_MESSAGE',body:{text}}];changed();}
export async function respondToInteraction(_task:string,_id:string,request:{answers:Answer[]}){guarded();items=items.map(i=>i.type==='QUESTION'?{...i,version:'2',body:{...i.body,version:'2',status:'RESOLVED',response:{id:'9007199254741055',answers:request.answers}}}:i);changed();}
export async function listTaskNetworkGrants(){return [];}
export async function revokeTaskNetworkGrant(){return;}
export async function getExecutionOperation(){return {...operation.body,id:operation.body.operationId,taskId:'9007199254740993',command:'curl https://documentation.example.test/acceptance',cwd:'/workspace'};}
export async function decideExecutionOperation(_task:string,_id:string,request:{action:OperationDecisionAction}){guarded();items=items.map(i=>i.type==='OPERATION'?{...i,version:'2',body:{...i.body,version:'2',status:request.action==='DENY'?'DENIED':'READY',decisionAction:request.action,result:{summary:request.action==='DENY'?'已拒绝访问，结果将返回 Agent':'当前请求已允许'},resultDeliveryStatus:'SAVED'}}:i);changed();}
export async function decideTaskDelivery(_task:string,_id:string,request:{action:DeliveryDecisionAction}){guarded();items=items.map(i=>i.type==='DELIVERY'?{...i,version:'2',body:{...i.body,version:'2',status:request.action==='DELIVER'?'PREPARING':'SKIPPED',allowedActions:[]}}:i);changed();}
export async function restartTaskDelivery(){return;}
