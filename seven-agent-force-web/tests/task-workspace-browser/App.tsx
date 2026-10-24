import { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { ConfigProvider, Button, Select, Space, Tag } from 'antd';
import TaskConversation from '../../src/app/workforce/tasks/TaskConversation';
import TaskStatusRail from '../../src/app/workforce/tasks/TaskStatusRail';
import TaskWorkflowMilestones from '../../src/app/workforce/tasks/TaskWorkflowMilestones';
import type { WorkforceTask } from '../../src/api/workforceController';
import type { IndependentReviewReport } from '../../src/api/independentReviewController';
import { scenario, closeQuestion, failNext, toggleFailure, observeFixture, fixtureTaskStatus, fixturePendingCount } from './fixtures';
import '../../src/app/workforce/tasks/workforce.css';
const client = new QueryClient({defaultOptions:{queries:{retry:false},mutations:{retry:false}}});
const params = new URLSearchParams(location.search);
const embedded = params.has('embedded');
const cases = ['question','operation','delivery','review','success','failed','empty','unavailable'];
export default function App(){
  const [tab,setTab]=useState('workflow');
  const [note,setNote]=useState('');
  const [width,setWidth]=useState(1200);
  const [,setRevision]=useState(0);
  useEffect(()=>observeFixture(()=>setRevision(value=>value+1)),[]);
  const status=fixtureTaskStatus();
  const task = {id:'9007199254740993',status,pendingActionCount:fixturePendingCount()} as WorkforceTask;
  const completed = ['success','delivery'].includes(scenario);
  const review = {required:true,status:scenario==='review'?'MANUAL_REVIEW':completed?'PASSED':'PENDING_FINALIZATION',verdict:completed?'PASS':undefined,findings:[],score:completed?95:undefined,automaticRevisionCount:0,automaticRevisionLimit:1,noProgressStreak:0,availableActions:scenario==='review'?['ACCEPT_RISK']:[],revisionGuidanceRequired:false} as IndependentReviewReport;
  const results=()=>{setTab('results');setNote('查看结果与报告');};
  return <ConfigProvider theme={{cssVar:{key:"workforce-fixture"},token:{colorPrimary:'#1677ff'}}}>
    {!embedded?<div style={{padding:16,background:'#f5f7fb',minHeight:'100vh',boxSizing:'border-box'}}>
      <Space wrap><Tag color="purple">组件验收数据 · 不连接业务 API</Tag><Select aria-label="验收状态" value={scenario} options={cases.map(value=>({value,label:value}))} onChange={value=>{location.href=`?scenario=${value}`;}}/><Button onClick={()=>setWidth(1200)}>1200px</Button><Button onClick={()=>setWidth(390)}>390px</Button><span>{width} CSS px · 简体中文 · 浅色</span></Space>
      <iframe title="任务工作区验收" src={`?embedded=1&scenario=${scenario}`} style={{display:'block',width,height:width===390?780:850,marginTop:12,border:'1px solid #ddd',maxWidth:'none'}} />
    </div>:<div className="workforce-detail-page" style={{padding:16,background:'#f5f7fb',boxSizing:'border-box'}}>
      <h2 style={{fontSize:20,margin:'0 0 12px'}}>Workforce 任务工作区验收</h2>
      <Space wrap style={{marginBottom:12}}><Button onClick={()=>setTab('workflow')}>任务对话</Button><Button onClick={results}>结果与报告</Button><Button onClick={closeQuestion}>模拟问题关闭</Button><Button onClick={failNext}>下次提交失败</Button><Button onClick={toggleFailure}>切换加载失败</Button></Space>
      <div className="workforce-workspace-grid" hidden={tab!=='workflow'}>
        <main className="workforce-workflow-column"><TaskConversation taskId={task.id} goal="核对真实用户操作、保存结果，并根据明确授权处理后续交付。" visible={tab==='workflow'} reviewPending={scenario==='review'} onReview={results} onResults={results} onDetails={()=>setNote('打开任务详情')} milestones={<TaskWorkflowMilestones taskStatus={status} review={review} loading={false} onOpen={results}/>}/></main>
        <TaskStatusRail task={task} deliveries={[]} review={review} latestStep="等待当前操作完成" onOpenPending={()=>setNote('定位待办')} onOpenDetails={()=>setNote('打开任务详情')} onOpenResults={results}/>
      </div>
      {tab==='results'&&<section aria-label="结果与报告"><h3>结果与报告</h3><p>验收入口已打开；此处没有生成或读取业务结果。</p></section>}
      {note&&<p role="status">{note}</p>}
    </div>}
  </ConfigProvider>;
}
createRoot(document.getElementById('root')!).render(<BrowserRouter><QueryClientProvider client={client}><App/></QueryClientProvider></BrowserRouter>);
