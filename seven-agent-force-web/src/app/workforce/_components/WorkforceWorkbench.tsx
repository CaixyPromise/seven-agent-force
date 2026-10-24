'use client';

import {
  AlertOutlined,
  ArrowRightOutlined,
  CheckCircleFilled,
  ClockCircleFilled,
  CloseCircleFilled,
  LoadingOutlined,
  PlusOutlined,
  ReloadOutlined,
  RobotOutlined,
  ThunderboltFilled,
} from '@ant-design/icons';
import { Button, Empty, Segmented, Skeleton, Tooltip } from 'antd';
import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { getWorkforceWorkbench } from '@/api/taskAnalyticsController';
import type { WorkbenchData } from '@/api/taskAnalyticsContract';
import { usePermissionAccess } from '@/hooks/auth';
import { AnalyticsChart } from './AnalyticsChart';
import { categoryAxis, chartGrid, chartLegend, localDay, tooltip, valueAxis } from './AnalyticsChartConfig';
import styles from './WorkforceWorkbench.module.css';

type Range = '7d' | '14d' | '30d';

const statusMeta: Record<string, { label: string; tone: string; progress: number }> = {
  CREATED: { label: '排队中', tone: 'violet', progress: 10 },
  QUEUED: { label: '排队中', tone: 'violet', progress: 12 },
  PREPARING: { label: '准备中', tone: 'blue', progress: 22 },
  RUNNING: { label: '执行中', tone: 'blue', progress: 58 },
  VALIDATING: { label: '完成检查', tone: 'green', progress: 82 },
  REVIEWING: { label: '等待审查', tone: 'orange', progress: 76 },
  PACKAGING: { label: '正在交付', tone: 'green', progress: 88 },
  UPLOADING: { label: '正在交付', tone: 'green', progress: 92 },
  INTERACTION_SUSPENDED: { label: '等待回答', tone: 'orange', progress: 48 },
  RETRY_WAIT: { label: '等待重试', tone: 'orange', progress: 35 },
  MANUAL_REVIEW: { label: '等待处理', tone: 'orange', progress: 78 },
};

const attentionMeta: Record<string, { title: string; tone: 'orange' | 'red'; icon: 'question' | 'error' }> = {
  WAITING_USER_INPUT: { title: '等待回答', tone: 'orange', icon: 'question' },
  WAITING_DELIVERY_DECISION: { title: '等待交付决定', tone: 'orange', icon: 'question' },
  MANUAL_REVIEW: { title: '独立审查待处理', tone: 'red', icon: 'error' },
  DELIVERY_FAILED: { title: '交付失败', tone: 'red', icon: 'error' },
  EXECUTION_FAILED: { title: '任务执行失败', tone: 'red', icon: 'error' },
};

const runningStatuses = new Set(['RUNNING', 'VALIDATING', 'REVIEWING', 'PACKAGING', 'UPLOADING', 'CANCELLING']);

const deliveryMeta: Record<string, { label: string; tone: 'success' | 'pending' | 'failed' }> = {
  SUCCEEDED: { label: '成功', tone: 'success' },
  WAITING_APPROVAL: { label: '待决定', tone: 'pending' },
  QUEUED: { label: '排队中', tone: 'pending' },
  RUNNING: { label: '交付中', tone: 'pending' },
  FAILED: { label: '失败', tone: 'failed' },
};

function formatCount(value: number | null | undefined) {
  return new Intl.NumberFormat('zh-CN', { notation: value && value >= 10000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value ?? 0);
}

function relativeTime(value: string) {
  const seconds = Math.max(0, Math.round((Date.now() - new Date(value).getTime()) / 1000));
  if (seconds < 60) return '刚刚';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
  return `${Math.floor(seconds / 86400)} 天前`;
}

function taskLink(taskId: API.Int64) {
  return `/workforce/tasks/${encodeURIComponent(String(taskId))}`;
}

function attentionLink(item: WorkbenchData['attentionItems'][number]) {
  const base = taskLink(item.taskId);
  if (item.objectKind === 'CONVERSATION_ITEM') return `${base}?item=${encodeURIComponent(String(item.objectId ?? ''))}`;
  if (item.objectKind === 'DELIVERY_REQUEST') return `${base}?focus=delivery&delivery=${encodeURIComponent(String(item.objectId ?? ''))}`;
  if (item.objectKind === 'REVIEW') return `${base}?tab=results&focus=review`;
  return base;
}

function StatCard({ icon, label, value, detail, tone, onClick }: { icon: ReactNode; label: string; value: number; detail: string; tone: string; onClick: () => void }) {
  return (
    <button type="button" className={styles.statCard} onClick={onClick}>
      <span className={`${styles.statIcon} ${styles[tone]}`}>{icon}</span>
      <span className={styles.statBody}><span>{label}</span><strong>{formatCount(value)}</strong><small>{detail}</small></span>
    </button>
  );
}

export default function WorkforceWorkbench() {
  const navigate = useNavigate();
  const canCreate = usePermissionAccess('workforce:task:create');
  const canViewModelUsage = usePermissionAccess('workforce:model-credential:usage');
  const [range, setRange] = useState<Range>('14d');
  const query = useQuery({
    queryKey: ['workforce', 'workbench', range],
    queryFn: () => getWorkforceWorkbench(range),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    placeholderData: keepPreviousData,
  });

  useEffect(() => { document.title = '工作台 - Seven Agent Force'; }, []);
  const data = query.data;
  const activityOption = useMemo(() => ({
    grid: chartGrid(), legend: chartLegend(), tooltip: tooltip(),
    xAxis: categoryAxis(data?.activityTrend.map((item) => localDay(item.at)) ?? []),
    yAxis: valueAxis(),
    series: [
      { name: '创建', type: 'bar', data: data?.activityTrend.map((item) => item.created) ?? [], itemStyle: { color: '#1677ff', borderRadius: [3, 3, 0, 0] }, barMaxWidth: 18 },
      { name: '完成', type: 'bar', data: data?.activityTrend.map((item) => item.completed) ?? [], itemStyle: { color: '#24bd7b', borderRadius: [3, 3, 0, 0] }, barMaxWidth: 18 },
      { name: '积压', type: 'line', data: data?.activityTrend.map((item) => item.backlog) ?? [], symbolSize: 6, itemStyle: { color: '#6c5ce7' }, lineStyle: { color: '#6c5ce7', width: 2 }, smooth: false },
    ],
  }), [data?.activityTrend]);
  const modelOption = useMemo(() => ({
    grid: { left: 3, right: 3, top: 6, bottom: 3 },
    xAxis: { type: 'category', show: false, data: data?.modelUsage?.trend.map((item) => item.at) ?? [] },
    yAxis: { type: 'value', show: false, min: 'dataMin' },
    tooltip: tooltip(),
    series: [{ type: 'line', data: data?.modelUsage?.trend.map((item) => item.totalTokens) ?? [], connectNulls: false, showSymbol: false, lineStyle: { color: '#1677ff', width: 2 }, areaStyle: { color: 'rgba(22,119,255,.08)' } }],
  }), [data?.modelUsage?.trend]);

  if (query.isLoading && !data) return <WorkbenchSkeleton canCreate={canCreate} canViewModelUsage={canViewModelUsage} navigate={navigate} />;
  if (query.isError && !data) {
    return <main className={styles.page}><Header canCreate={canCreate} navigate={navigate} /><div className={styles.errorPanel}><AlertOutlined /><h2>工作台暂时无法加载</h2><p>主操作仍可使用，您也可以重新加载工作台数据。</p><Button icon={<ReloadOutlined />} onClick={() => query.refetch()}>重新加载</Button></div></main>;
  }
  if (!data) return null;

  const capacity = data.resources.maximumSlots > 0 ? Math.round((data.resources.runningSlots / data.resources.maximumSlots) * 100) : 0;
  const runningProjectCount = new Set(
    data.activeTasks
      .filter((item) => runningStatuses.has(item.status))
      .map((item) => String(item.projectId)),
  ).size;
  return (
    <main className={styles.page} aria-busy={query.isFetching}>
      <Header canCreate={canCreate} navigate={navigate} />

      <section className={styles.stats} aria-label="工作概览">
        <StatCard icon={<AlertOutlined />} label="待我处理" value={data.summary.attentionCount} detail={`${data.attentionItems.filter((item) => item.reasonCode === 'WAITING_USER_INPUT').length} 个问题 · ${data.attentionItems.filter((item) => item.reasonCode === 'WAITING_DELIVERY_DECISION').length} 个交付决定`} tone="orange" onClick={() => navigate('/workforce/tasks?view=attention')} />
        <StatCard icon={<ThunderboltFilled />} label="执行中任务" value={data.summary.runningTaskCount} detail={runningProjectCount > 0 ? `分布在 ${runningProjectCount} 个项目` : '当前没有执行中的任务'} tone="blue" onClick={() => navigate('/workforce/tasks?view=active')} />
        <StatCard icon={<ClockCircleFilled />} label="排队任务" value={data.summary.queuedTaskCount} detail="等待可用槽位" tone="violet" onClick={() => navigate('/workforce/tasks?view=waiting')} />
        <StatCard icon={<CheckCircleFilled />} label="今日完成" value={data.summary.completedTodayCount} detail={data.summary.todaySuccessRate == null ? '今日暂无闭环任务' : `成功率 ${Math.round(data.summary.todaySuccessRate * 100)}%`} tone="green" onClick={() => navigate('/workforce/tasks?view=succeeded')} />
      </section>

      <section className={styles.primaryGrid}>
        <article className={styles.panel}>
          <div className={styles.panelHeader}><div><h2>进行中的工作</h2></div><button type="button" className={styles.linkButton} onClick={() => navigate('/workforce/tasks?view=active')}>查看全部任务 <ArrowRightOutlined /></button></div>
          {data.activeTasks.length ? <div className={styles.taskList}>{data.activeTasks.map((task) => {
            const meta = statusMeta[task.status] ?? { label: task.status, tone: 'blue', progress: 40 };
            return <button type="button" className={styles.taskRow} key={String(task.taskId)} onClick={() => navigate(taskLink(task.taskId))}>
              <span className={styles.taskIdentity}><strong>{task.name || '未命名任务'}</strong><small>{task.projectName}</small></span>
              <span className={`${styles.statusPill} ${styles[meta.tone]}`}>{meta.label}</span>
              <span className={styles.runtime}>{task.runtimeName || '执行镜像待分配'}</span>
              <span className={styles.progress}><i><b style={{ width: `${meta.progress}%` }} /></i></span>
              <span className={styles.taskTime}>{relativeTime(task.updatedAt)}</span><ArrowRightOutlined className={styles.rowArrow} />
            </button>;
          })}</div> : <Empty className={styles.compactEmpty} image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有进行中的任务" />}
        </article>

        <article className={styles.panel}>
          <div className={styles.panelHeader}><div><h2>需要关注</h2></div>{data.summary.attentionCount > 0 && <span className={styles.countBadge}>{data.summary.attentionCount} 项</span>}</div>
          {data.attentionItems.length ? <div className={styles.attentionList}>{data.attentionItems.map((item) => {
            const meta = attentionMeta[item.reasonCode] ?? { title: item.reasonCode, tone: 'red' as const, icon: 'error' as const };
            return <button type="button" className={`${styles.attentionRow} ${styles[meta.tone]}`} key={`${item.reasonCode}-${String(item.taskId)}`} onClick={() => navigate(attentionLink(item))}>
              <span className={styles.attentionIcon}>{meta.icon === 'question' ? '?' : '!'}</span><span className={styles.attentionIdentity}><strong>{meta.title}</strong><small>{item.projectName}</small></span><time>{relativeTime(item.occurredAt)}</time><b>{item.reasonCode === 'WAITING_USER_INPUT' ? '去处理' : '查看'}</b>
            </button>;
          })}</div> : <Empty className={styles.compactEmpty} image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有需要处理的事项" />}
        </article>
      </section>

      <section className={styles.analysisGrid}>
        <article className={`${styles.panel} ${styles.chartPanel}`}>
          <div className={styles.panelHeader}><div><h2>任务活动</h2><p>最近 {range === '7d' ? '7 天' : range === '14d' ? '14 天' : '30 天'}</p></div><div className={styles.chartActions}><Segmented size="small" value={range} options={[{ label: '7 天', value: '7d' }, { label: '14 天', value: '14d' }, { label: '30 天', value: '30d' }]} onChange={(value) => setRange(value as Range)} /><Tooltip title="刷新"><Button size="small" type="text" icon={<ReloadOutlined spin={query.isFetching} />} aria-label="刷新工作台" onClick={() => query.refetch()} /></Tooltip></div></div>
          <div className={styles.activityChartFrame}>
            <AnalyticsChart className={styles.activityChart} ariaLabel="任务创建、完成与积压趋势" option={activityOption} onSelect={() => navigate(`/workforce/tasks?from=${encodeURIComponent(data.from)}&to=${encodeURIComponent(data.to)}`)} />
          </div>
        </article>
        <article className={`${styles.panel} ${styles.resourcePanel}`}>
          <div className={styles.panelHeader}><div><h2>执行资源</h2></div><button type="button" className={styles.linkButton} onClick={() => navigate('/workforce/workers')}>查看 Worker <ArrowRightOutlined /></button></div>
          <div className={styles.resourceStats}><span><RobotOutlined /><small>在线 Worker</small><strong>{data.resources.onlineWorkerCount}</strong></span><span><ThunderboltFilled /><small>运行槽位</small><strong>{data.resources.runningSlots} / {data.resources.maximumSlots}</strong></span><span><ClockCircleFilled /><small>排队</small><strong>{data.resources.queuedTasks}</strong></span></div>
          <div className={styles.resourceBody}><div className={styles.capacityRing} style={{ '--capacity': `${capacity * 3.6}deg` } as CSSProperties}><span><strong>{capacity}%</strong><small>槽位使用率</small></span></div><div className={styles.workerList}><h3>Worker 列表</h3>{data.resources.workers.length ? data.resources.workers.map((worker) => <button type="button" key={String(worker.workerId)} onClick={() => navigate('/workforce/workers')}><i /> <span>{worker.name}</span><b>{worker.runningSlots} / {worker.maximumSlots}</b><em>{worker.runningSlots < worker.maximumSlots ? '运行中' : '已满'}</em></button>) : <p>暂无在线 Worker</p>}</div></div>
        </article>
      </section>

      <section className={`${styles.bottomGrid} ${!data.modelUsage ? styles.singleBottom : ''}`}>
        {data.modelUsage && <article className={`${styles.panel} ${styles.modelPanel}`}><div><h2>模型调用</h2><div className={styles.modelStats}><span><small>今日调用</small><strong>{formatCount(data.modelUsage.invocationCount)} 次</strong></span><span><small>总 Token</small><strong>{data.modelUsage.totalTokens == null ? '—' : formatCount(data.modelUsage.totalTokens)}</strong></span><span><small>缓存命中</small><strong>{data.modelUsage.cacheHitRate == null ? '—' : `${Math.round(data.modelUsage.cacheHitRate * 100)}%`}</strong></span></div></div><AnalyticsChart className={styles.modelChart} ariaLabel="模型 Token 使用趋势" option={modelOption} /></article>}
        <article className={`${styles.panel} ${styles.deliveryPanel}`}><div className={styles.panelHeader}><div><h2>最近交付</h2></div><button type="button" className={styles.linkButton} onClick={() => navigate('/workforce/tasks')}>查看全部 <ArrowRightOutlined /></button></div>{data.recentDeliveries.length ? <div className={styles.deliveryList}>{data.recentDeliveries.map((delivery) => {
          const meta = deliveryMeta[delivery.status] ?? { label: delivery.status, tone: 'pending' as const };
          return <button type="button" key={String(delivery.deliveryId)} onClick={() => navigate(`${taskLink(delivery.taskId)}?focus=delivery&delivery=${encodeURIComponent(String(delivery.deliveryId))}`)}>{meta.tone === 'success' ? <CheckCircleFilled /> : meta.tone === 'failed' ? <CloseCircleFilled /> : <ClockCircleFilled />}<strong>{delivery.taskName}</strong><span>{delivery.projectName}</span><b className={meta.tone === 'success' ? styles.deliverySuccess : meta.tone === 'failed' ? styles.deliveryFailed : styles.deliveryPending}>{meta.label}</b><time>{relativeTime(delivery.updatedAt)}</time></button>;
        })}</div> : <Empty className={styles.compactEmpty} image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无交付记录" />}</article>
      </section>
      {query.isFetching && <span className={styles.refreshing}><LoadingOutlined /> 正在刷新</span>}
    </main>
  );
}

function Header({ canCreate, navigate }: { canCreate: boolean; navigate: ReturnType<typeof useNavigate> }) {
  return <header className={styles.header}><div><h1>工作台</h1><p>查看正在进行的工作、待处理事项和平台运行状态</p></div>{canCreate && <div className={styles.headerActions}><Button onClick={() => navigate('/workforce/projects/new')}>新建项目</Button><Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/workforce/tasks/new')}>发起任务</Button></div>}</header>;
}

function WorkbenchSkeleton({ canCreate, canViewModelUsage, navigate }: { canCreate: boolean; canViewModelUsage: boolean; navigate: ReturnType<typeof useNavigate> }) {
  return (
    <main className={styles.page} aria-busy="true" aria-label="正在加载工作台">
      <Header canCreate={canCreate} navigate={navigate} />
      <section className={styles.stats} aria-hidden="true">
        {[0, 1, 2, 3].map((item) => <div className={`${styles.statCard} ${styles.skeletonStat}`} key={item}>
          <Skeleton.Avatar active shape="square" size={56} />
          <div className={styles.skeletonStatText}><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /></div>
        </div>)}
      </section>
      <section className={styles.primaryGrid} aria-hidden="true">
        <div className={`${styles.panel} ${styles.skeletonPanel}`}><SkeletonPanelHeading /><div className={styles.skeletonTaskList}>{[0, 1, 2, 3].map((item) => <div className={styles.skeletonTaskRow} key={item}><Skeleton.Input active size="small" /><Skeleton.Button active size="small" /><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /></div>)}</div></div>
        <div className={`${styles.panel} ${styles.skeletonPanel}`}><SkeletonPanelHeading /><div className={styles.skeletonAttentionList}>{[0, 1, 2].map((item) => <div className={styles.skeletonAttentionRow} key={item}><Skeleton.Avatar active size="small" /><Skeleton.Input active size="small" /><Skeleton.Button active size="small" /></div>)}</div></div>
      </section>
      <section className={styles.analysisGrid} aria-hidden="true">
        <div className={`${styles.panel} ${styles.chartPanel} ${styles.skeletonPanel}`}><SkeletonPanelHeading /><div className={styles.skeletonChart}><div className={styles.skeletonChartBars}>{[38, 60, 47, 74, 54, 86, 64, 48, 70, 56, 80, 62].map((height, item) => <i key={item} style={{ height: `${height}%` }} />)}</div></div></div>
        <div className={`${styles.panel} ${styles.resourcePanel} ${styles.skeletonPanel}`}><SkeletonPanelHeading /><div className={styles.skeletonResourceStats}><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /></div><div className={styles.skeletonResourceBody}><Skeleton.Avatar active shape="circle" size={104} /><div>{[0, 1].map((item) => <Skeleton.Input active size="small" key={item} />)}</div></div></div>
      </section>
      <section className={`${styles.bottomGrid} ${!canViewModelUsage ? styles.singleBottom : ''}`} aria-hidden="true">
        {canViewModelUsage && <div className={`${styles.panel} ${styles.modelPanel} ${styles.skeletonModelPanel}`}><div><SkeletonPanelHeading /><div className={styles.skeletonModelStats}><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /><Skeleton.Input active size="small" /></div></div><Skeleton.Input active block /></div>}
        <div className={`${styles.panel} ${styles.deliveryPanel} ${styles.skeletonPanel}`}><SkeletonPanelHeading />{[0, 1].map((item) => <div className={styles.skeletonDeliveryRow} key={item}><Skeleton.Avatar active size="small" /><Skeleton.Input active size="small" /><Skeleton.Button active size="small" /></div>)}</div>
      </section>
    </main>
  );
}

function SkeletonPanelHeading() {
  return <div className={styles.skeletonPanelHeading}><Skeleton.Input active size="small" /><Skeleton.Button active size="small" /></div>;
}
