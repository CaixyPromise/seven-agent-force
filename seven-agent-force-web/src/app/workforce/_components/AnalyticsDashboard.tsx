import { CheckCircleOutlined, ClockCircleOutlined, ExclamationCircleOutlined, PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import { Button, Card, Flex, Segmented, Skeleton, Statistic, Typography } from 'antd';
import { useMemo } from 'react';
import type { ReactNode } from 'react';
import type { ProjectAnalytics, ProjectListAnalytics, TaskAnalytics } from '@/api/taskAnalyticsContract';
import { AnalyticsChart, ChartState } from './AnalyticsChart';
import { categoryAxis, chartGrid, chartLegend, localDay, tooltip, valueAxis } from './AnalyticsChartConfig';
import styles from './AnalyticsDashboard.module.css';

const { Text, Title } = Typography;
type QueryState<T> = { data?: T; isLoading: boolean; isError: boolean; isFetching: boolean; refetch: () => unknown };

function Shell({ title = '趋势与健康', range, ranges, onRange, query, children }: { title?: string; range: string; ranges: string[]; onRange: (value: string) => void; query: QueryState<unknown>; children: ReactNode }) {
  return <section className={styles.section}>
    <Flex className={styles.sectionHeader} align="center" justify="space-between" gap={12} wrap>
      <div><Title level={4}>{title}</Title><Text type="secondary">点击图表可查看对应任务</Text></div>
      <Flex gap={8} align="center"><Segmented size="small" value={range} options={ranges.map((value) => ({ label: value.replace('d', ' 天'), value }))} onChange={(value) => onRange(String(value))} /><Button size="small" aria-label="刷新分析" icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => query.refetch()} /></Flex>
    </Flex>
    {children}
  </section>;
}

function Summary({ loading = false, items }: { loading?: boolean; items: Array<{ label: string; value: number | string; tone?: 'attention' | 'success'; icon?: ReactNode }> }) {
  return <div className={styles.summaryGrid}>{items.map((item) => <Card key={item.label} className={`${styles.summaryCard} ${item.tone ? styles[item.tone] : ''}`}>
    {loading ? <Skeleton.Input active size="small" /> : <Flex align="center" gap={13}>
      {item.icon && <span className={styles.summaryIcon} aria-hidden="true">{item.icon}</span>}
      <Statistic title={item.label} value={item.value} />
    </Flex>}
  </Card>)}</div>;
}

export function ProjectListSummary({ query }: { query: QueryState<ProjectListAnalytics> }) {
  const data = query.data;
  return <Summary loading={query.isLoading} items={[
    { label: '进行中项目', value: data?.summary.activeProjectCount ?? '—' },
    { label: '范围内新增任务', value: data?.summary.createdTaskCount ?? '—' },
    { label: '成功任务', value: data?.summary.succeededTaskCount ?? '—', tone: 'success' },
    { label: '需关注项目', value: data?.summary.attentionProjectCount ?? '—', tone: 'attention' },
  ]} />;
}

export function TaskListSummary({ query }: { query: QueryState<TaskAnalytics> }) {
  const data = query.data;
  return <Summary loading={query.isLoading} items={[
    { label: '待处理', value: data?.summary.attention ?? '—', tone: 'attention', icon: <ExclamationCircleOutlined /> },
    { label: '执行中', value: data?.summary.running ?? '—', icon: <PlayCircleOutlined /> },
    { label: '等待中', value: data?.summary.waiting ?? '—', icon: <ClockCircleOutlined /> },
    { label: '范围内完成', value: data?.summary.succeeded ?? '—', tone: 'success', icon: <CheckCircleOutlined /> },
  ]} />;
}

function Panel({ title, subtitle, children, wide }: { title: string; subtitle?: string; children: ReactNode; wide?: boolean }) {
  return <Card className={`${styles.panel} ${wide ? styles.wide : ''}`} title={<div><strong>{title}</strong>{subtitle && <small>{subtitle}</small>}</div>}>{children}</Card>;
}

const trendColors = ['#1677ff', '#22a06b', '#ef5b5b', '#98a2b3'];
function taskTrendOption(points: Array<{ at: string; created: number; succeeded: number; failed: number; cancelled: number }>) {
  const series = [['创建', 'created'], ['成功', 'succeeded'], ['失败', 'failed'], ['取消', 'cancelled']] as const;
  return { color: trendColors, legend: chartLegend(), tooltip: tooltip(), grid: chartGrid(), xAxis: categoryAxis(points.map((item) => localDay(item.at))), yAxis: valueAxis(), series: series.map(([name, key]) => ({ name, type: 'bar', stack: 'tasks', barMaxWidth: 28, data: points.map((item) => item[key]), itemStyle: { borderRadius: key === 'cancelled' ? [4, 4, 0, 0] : 0 } })) };
}

export function ProjectListInsights({ range, onRange, query, onProject, onDay }: { range: '7d' | '30d' | '90d'; onRange: (value: '7d' | '30d' | '90d') => void; query: QueryState<ProjectListAnalytics>; onProject: (id: string, view: string) => void; onDay: (day: string, view: string) => void }) {
  const data=query.data; const trend=useMemo(()=>taskTrendOption(data?.taskTrend??[]),[data?.taskTrend]);
  const health=useMemo(()=>({ color:['#22a06b','#1677ff','#ef5b5b','#faad14'],legend:chartLegend(),tooltip:tooltip(),grid:{...chartGrid(),left:12},xAxis:valueAxis(),yAxis:{...categoryAxis((data?.projectHealth??[]).map(x=>x.name),true),inverse:true},series:[['成功','succeeded'],['进行中','active'],['失败','failed'],['需关注','attention']].map(([name,key])=>({name,type:'bar',stack:'health',barMaxWidth:24,data:(data?.projectHealth??[]).map(x=>x[key as keyof typeof x])}))}),[data?.projectHealth]);
  const empty=!!data&&data.summary.activeProjectCount===0&&data.summary.createdTaskCount===0&&data.summary.succeededTaskCount===0&&data.summary.attentionProjectCount===0&&data.taskTrend.length===0&&data.projectHealth.length===0;
  return <Shell range={range} ranges={['7d','30d','90d']} onRange={(v)=>onRange(v as typeof range)} query={query}><ChartState loading={query.isLoading} error={query.isError} empty={empty} onRetry={query.refetch}>{data&&<div className={styles.grid}><Panel title="项目任务趋势" subtitle="创建、成功、失败与取消" wide><AnalyticsChart ariaLabel="项目任务趋势" option={trend} onSelect={(raw)=>{const p=raw as {dataIndex?:number;seriesName?:string};const point=data.taskTrend[p.dataIndex??-1];if(point)onDay(point.at,p.seriesName==='失败'?'failed':p.seriesName==='成功'?'succeeded':'all')}} /></Panel><Panel title="项目健康" subtitle="最多显示 10 个需要关注的项目"><AnalyticsChart ariaLabel="项目健康对比" option={health} onSelect={(raw)=>{const p=raw as {dataIndex?:number;seriesName?:string};const item=data.projectHealth[p.dataIndex??-1];if(item)onProject(String(item.projectId),p.seriesName==='需关注'?'attention':p.seriesName==='失败'?'failed':'all')}} /></Panel></div>}</ChartState></Shell>;
}

const statusLabels: Record<string,string>={PENDING:'待开始',RUNNING:'执行中',WAITING:'等待处理',SUCCEEDED:'成功',FAILED:'失败',CANCELLED:'取消'};
const waitLabels: Record<string,string>={WAITING_USER_INPUT:'等待用户回答',WAITING_DELIVERY_DECISION:'等待交付决定',MANUAL_REVIEW:'人工处理',WAITING_AGENT_RUNTIME:'等待执行镜像',WAITING_WORKER_CAPACITY:'等待 Worker 容量',WORKER_DISK_PRESSURE:'Worker 磁盘压力',WAITING_REVIEW:'等待审查',RETRY_WAIT:'等待重试'};
export function TaskListInsights({range,onRange,query,onFilter}:{range:'7d'|'14d'|'30d';onRange:(v:'7d'|'14d'|'30d')=>void;query:QueryState<TaskAnalytics>;onFilter:(input:{view?:string;from?:string;to?:string})=>void}){
 const d=query.data;
 const activity=useMemo(()=>({color:['#1677ff','#22a06b','#722ed1'],legend:chartLegend(),tooltip:tooltip(),grid:chartGrid(),xAxis:categoryAxis((d?.activityTrend??[]).map(x=>localDay(x.at))),yAxis:valueAxis(),series:[{name:'创建',type:'bar',data:(d?.activityTrend??[]).map(x=>x.created),barMaxWidth:20},{name:'完成',type:'bar',data:(d?.activityTrend??[]).map(x=>x.terminal),barMaxWidth:20},{name:'积压',type:'line',data:(d?.activityTrend??[]).map(x=>x.backlog),smooth:false,showSymbol:(d?.activityTrend.length??0)<=3,lineStyle:{width:2.4}}]}),[d?.activityTrend]);
 const duration=durationOption(d?.durationTrend??[]);
 const empty=!!d&&d.summary.attention===0&&d.summary.running===0&&d.summary.waiting===0&&d.summary.succeeded===0&&d.activityTrend.length===0&&d.statusCounts.every(x=>x.count===0);
 return <Shell range={range} ranges={['7d','14d','30d']} onRange={v=>onRange(v as typeof range)} query={query}><ChartState loading={query.isLoading} error={query.isError} empty={empty} onRetry={query.refetch}>{d&&<div className={styles.grid}><Panel title="任务活动与积压" wide><AnalyticsChart ariaLabel="任务创建完成和积压趋势" option={activity} onSelect={(raw)=>{const p=raw as {dataIndex?:number;seriesName?:string};const point=d.activityTrend[p.dataIndex??-1];if(point)onFilter({view:p.seriesName==='完成'?'succeeded':'all',from:point.at,to:new Date(new Date(point.at).getTime()+86400000).toISOString()})}}/></Panel><Panel title="当前状态"><AnalyticsChart ariaLabel="任务状态分布" option={pieOption(d.statusCounts,statusLabels)} onSelect={(raw)=>{const p=raw as {name?:string};const code=Object.entries(statusLabels).find(([,v])=>v===p.name)?.[0];onFilter({view:code==='FAILED'?'failed':code==='SUCCEEDED'?'succeeded':code==='CANCELLED'?'cancelled':code==='WAITING'?'waiting':'active'})}}/></Panel><Panel title="等待时长"><AnalyticsChart ariaLabel="任务等待时长" option={barOption(d.waitingAgeBuckets.map(x=>({name:{LT_10M:'10 分钟内',M10_30:'10–30 分钟',M30_2H:'30 分钟–2 小时',GT_2H:'2 小时以上'}[x.code]??x.code,value:x.count})),'等待任务')}/></Panel><Panel title="等待原因"><AnalyticsChart ariaLabel="任务等待原因" option={barOption(d.waitingReasons.map(x=>({name:waitLabels[x.code]??x.code,value:x.count})),'任务数',true)} onSelect={()=>onFilter({view:'waiting'})}/></Panel><Panel title="完成耗时" subtitle="成功任务 P50 / P90" wide><AnalyticsChart ariaLabel="任务完成耗时趋势" option={duration}/></Panel></div>}</ChartState></Shell>
}

export function ProjectDashboard({range,onRange,query,onFilter}:{range:'7d'|'30d'|'90d';onRange:(v:'7d'|'30d'|'90d')=>void;query:QueryState<ProjectAnalytics>;onFilter:(input:{view?:string;from?:string;to?:string})=>void}){
 const d=query.data;const trend=useMemo(()=>taskTrendOption(d?.taskTrend??[]),[d?.taskTrend]);const stages=barOption((d?.stageHealth??[]).map(x=>({name:{RESULT_FROZEN:'结果冻结',COMPLETION_CHECK:'完成检查',INDEPENDENT_REVIEW:'独立审查',GIT_DELIVERY:'Git 交付'}[x.code]??x.code,value:x.reachedCount,eligible:x.eligibleCount})),'已闭环',true);const model=useMemo(()=>({color:['#1677ff','#22a06b','#722ed1'],legend:chartLegend(),tooltip:tooltip(),grid:chartGrid(),xAxis:categoryAxis((d?.modelUsageTrend??[]).map(x=>localDay(x.at)),false),yAxis:valueAxis(),series:[{name:'输入 Token',type:'line',data:(d?.modelUsageTrend??[]).map(x=>x.inputTokens),connectNulls:false},{name:'缓存 Token',type:'line',data:(d?.modelUsageTrend??[]).map(x=>x.cachedInputTokens),connectNulls:false},{name:'输出 Token',type:'line',data:(d?.modelUsageTrend??[]).map(x=>x.outputTokens),connectNulls:false}]}),[d?.modelUsageTrend]);
 const review=d?pieOption([{code:'DIRECT',count:d.reviewOutcomes.directPassed},{code:'REVISED',count:d.reviewOutcomes.passedAfterRevision},{code:'RISK',count:d.reviewOutcomes.acceptedRisk},{code:'REJECTED',count:d.reviewOutcomes.rejected},{code:'PROCESSING',count:d.reviewOutcomes.processing},{code:'DISABLED',count:d.reviewOutcomes.disabled}],{DIRECT:'直接通过',REVISED:'修订后通过',RISK:'人工接受风险',REJECTED:'拒绝',PROCESSING:'处理中',DISABLED:'未启用'}):{};
 const empty=!!d&&d.summary.createdTaskCount===0&&d.summary.successfulDeliveryCount===0&&d.summary.attentionTaskCount===0&&d.taskTrend.length===0&&d.stageHealth.length===0&&d.durationTrend.length===0&&d.waitingReasons.length===0&&d.modelUsageTrend.length===0;
 return <Shell title="项目分析" range={range} ranges={['7d','30d','90d']} onRange={v=>onRange(v as typeof range)} query={query}><ChartState loading={query.isLoading} error={query.isError} empty={empty} onRetry={query.refetch}>{d&&<><Summary items={[{label:'新增任务',value:d.summary.createdTaskCount},{label:'成功率',value:d.summary.successRate==null?'—':`${Math.round(d.summary.successRate*100)}%`,tone:'success'},{label:'成功交付',value:d.summary.successfulDeliveryCount},{label:'当前需关注',value:d.summary.attentionTaskCount,tone:'attention'}]}/><div className={styles.grid}><Panel title="交付趋势" wide><AnalyticsChart ariaLabel="项目任务交付趋势" option={trend} onSelect={(raw)=>{const p=raw as {dataIndex?:number;seriesName?:string};const point=d.taskTrend[p.dataIndex??-1];if(point)onFilter({view:p.seriesName==='失败'?'failed':p.seriesName==='成功'?'succeeded':'all',from:point.at,to:new Date(new Date(point.at).getTime()+86400000).toISOString()})}}/></Panel><Panel title="流程健康"><AnalyticsChart ariaLabel="项目流程健康" option={stages}/></Panel><Panel title="执行耗时"><AnalyticsChart ariaLabel="项目执行耗时" option={durationOption(d.durationTrend)}/></Panel><Panel title="等待原因"><AnalyticsChart ariaLabel="项目等待原因" option={barOption(d.waitingReasons.map(x=>({name:waitLabels[x.code]??x.code,value:x.count})),'任务数',true)} onSelect={()=>onFilter({view:'waiting'})}/></Panel><Panel title="独立审查结果"><AnalyticsChart ariaLabel="独立审查结果" option={review}/></Panel><Panel title="模型 Token 趋势" wide><AnalyticsChart ariaLabel="项目模型 Token 趋势" option={model}/></Panel></div></>}</ChartState></Shell>
}

function durationOption(points:Array<{at:string;p50Seconds:number|null;p90Seconds:number|null;sampleCount:number}>){return{color:['#1677ff','#722ed1'],legend:chartLegend(),tooltip:tooltip(),grid:chartGrid(),xAxis:categoryAxis(points.map(x=>localDay(x.at)),false),yAxis:valueAxis(),series:[{name:'P50',type:'line',data:points.map(x=>x.p50Seconds),connectNulls:false,showSymbol:points.length<=3},{name:'P90',type:'line',data:points.map(x=>x.p90Seconds),connectNulls:false,showSymbol:points.length<=3}]}}
function pieOption(items:Array<{code:string;count:number}>,labels:Record<string,string>){return{color:['#1677ff','#22a06b','#faad14','#ef5b5b','#98a2b3','#722ed1'],tooltip:{trigger:'item'},legend:{...chartLegend(),orient:'vertical',top:'center'},series:[{type:'pie',radius:['46%','72%'],center:['35%','52%'],label:{show:false},data:items.filter(x=>x.count>0).map(x=>({name:labels[x.code]??x.code,value:x.count}))}]}}
function barOption(items:Array<{name:string;value:number;eligible?:number}>,seriesName:string,horizontal=false){return{color:['#1677ff'],tooltip:{trigger:'axis',axisPointer:{type:'shadow'},formatter:(raw:unknown)=>{const p=(raw as Array<{dataIndex:number;value:number}>)[0];const item=items[p?.dataIndex];return item?`${item.name}<br/>${seriesName}：${item.value}${item.eligible!==undefined?` / ${item.eligible}`:''}`:''}},grid:chartGrid(),xAxis:horizontal?valueAxis():categoryAxis(items.map(x=>x.name)),yAxis:horizontal?categoryAxis(items.map(x=>x.name)):valueAxis(),series:[{name:seriesName,type:'bar',barMaxWidth:26,data:items.map(x=>x.value),itemStyle:{borderRadius:horizontal?[0,5,5,0]:[5,5,0,0]}}]}}
