import {
  ArrowRightOutlined,
  BranchesOutlined,
  CheckCircleFilled,
  ClockCircleOutlined,
  CodeOutlined,
  ExclamationCircleFilled,
  FileTextOutlined,
  PlusOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Alert, Badge, Button, Card, Empty, Flex, Input, Segmented, Skeleton, Space, Tag, Typography } from 'antd';
import { useMemo, useState } from 'react';
import { useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { listProjects } from '@/api/projectController';
import type { WorkforceProject } from '@/api/projectContract';
import { listWorkforceTasks, type TaskStatus, type WorkforceTask } from '@/api/workforceController';
import { getTaskAnalytics } from '@/api/taskAnalyticsController';
import { usePermissionAccess } from '@/hooks/auth';
import './workforce.css';
import { TaskListInsights, TaskListSummary } from '../_components/AnalyticsDashboard';

const { Paragraph, Text, Title } = Typography;

type TaskFilter = 'all' | 'active' | 'succeeded' | 'failed' | 'cancelled' | 'attention' | 'waiting';

const statusMeta: Record<TaskStatus, { label: string; color: string; terminal?: boolean }> = {
  CREATED: { label: '准备开始', color: 'default' },
  QUEUED: { label: '排队', color: 'gold' },
  PREPARING: { label: '准备环境', color: 'blue' },
  RUNNING: { label: '运行中', color: 'blue' },
  INTERACTION_SUSPENDED: { label: '等待你回答', color: 'gold' },
  VALIDATING: { label: '检查中', color: 'cyan' },
  PACKAGING: { label: '整理结果', color: 'geekblue' },
  UPLOADING: { label: '保存结果', color: 'purple' },
  RETRY_WAIT: { label: '等待重试', color: 'gold' },
  REVIEWING: { label: '审查中', color: 'cyan' },
  MANUAL_REVIEW: { label: '需要处理', color: 'orange' },
  SUCCEEDED: { label: '已完成', color: 'green', terminal: true },
  CANCELLING: { label: '取消中', color: 'orange' },
  CANCELLED: { label: '已取消', color: 'default', terminal: true },
  TIMED_OUT: { label: '已超时', color: 'red', terminal: true },
  FAILED: { label: '失败', color: 'red', terminal: true },
};

const filterOptions = [
  { label: '全部', value: 'all' },
  { label: '进行中', value: 'active' },
  { label: '成功', value: 'succeeded' },
  { label: '失败', value: 'failed' },
  { label: '需关注', value: 'attention' },
] satisfies Array<{ label: string; value: TaskFilter }>;

export default function WorkforceTasksPage({ createOnly = false, pendingOnly = false }: { createOnly?: boolean; pendingOnly?: boolean }) {
  if (createOnly) return <ProjectPicker />;
  return <TaskList pendingOnly={pendingOnly} />;
}

function TaskList({ pendingOnly }: { pendingOnly: boolean }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [params, setParams] = useSearchParams();
  const canCreate = usePermissionAccess('workforce:task:create');
  const filter = parseFilter(params.get('view') ?? params.get('status'));
  const search = params.get('query') ?? params.get('q') ?? '';
  const from=params.get('from')??undefined;const to=params.get('to')??undefined;
  const query = useInfiniteQuery({ queryKey: ['workforce','tasks',pendingOnly,filter,search,from,to], initialPageParam: undefined as string|undefined, queryFn:({pageParam})=>listWorkforceTasks({query:search||undefined,view:pendingOnly?'attention':filter,from,to,cursor:pageParam,limit:30}), getNextPageParam:(page)=>page.nextCursor??undefined });
  const tasks = useMemo(() => query.data?.pages.flatMap(page=>page.items) ?? [], [query.data]);
  const [analyticsRange,setAnalyticsRange]=useState<'7d'|'14d'|'30d'>('14d');
  const analytics=useQuery({queryKey:['workforce','tasks','analytics',analyticsRange],queryFn:()=>getTaskAnalytics(analyticsRange),enabled:!pendingOnly});
  const update = (next: { status?: TaskFilter; q?: string }) => {
    const copy = new URLSearchParams(params);
    if (next.status) {
      copy.delete('status');
      if (next.status === 'all') copy.delete('view');
      else copy.set('view', next.status);
    }
    if (next.q !== undefined) {
      copy.delete('q');
      if (next.q.trim()) copy.set('query', next.q);
      else copy.delete('query');
    }
    setParams(copy, { replace: true });
  };
  const open = (task: WorkforceTask) => navigate(`/workforce/tasks/${encodeURIComponent(task.id)}`, {
    state: { from: `${location.pathname}${location.search}` },
  });

  return <main className="workforce-page workforce-list-page">
    <Flex className="workforce-page-heading" justify="space-between" align="flex-end" wrap gap={16}>
      <Space orientation="vertical" size={2}>
        <Title level={2}>{pendingOnly ? '待我处理' : '任务'}</Title>
        <Text type="secondary">{pendingOnly ? '回答问题或确认任务下一步' : '查看当前组织的任务、进展和需要处理的事项'}</Text>
      </Space>
      {!pendingOnly && canCreate && <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/workforce/tasks/new')}>发起任务</Button>}
    </Flex>

    {!pendingOnly&&<TaskListSummary query={analytics}/>}

    <Card className="workforce-table-card workforce-task-workbench">
      <Flex className="workforce-toolbar" align="center" justify="space-between" wrap gap={12}>
        <Text strong className="workforce-list-title">{pendingOnly ? '待处理事项' : `任务列表 (${tasks.length})`}</Text>
        <Flex className="workforce-toolbar-controls" align="center" justify="end" wrap gap={10}>
          <Input className="workforce-search" allowClear prefix={<SearchOutlined />} value={search} placeholder="搜索任务" aria-label="搜索任务" onChange={(event) => update({ q: event.target.value })} />
          {!pendingOnly && <Segmented<TaskFilter> size="small" options={filterOptions} value={filter} onChange={(value) => update({ status: value })} />}
          <Button size="small" icon={<ReloadOutlined />} loading={query.isFetching} onClick={() => query.refetch()}>刷新</Button>
        </Flex>
      </Flex>

      {query.isLoading ? <Skeleton active paragraph={{ rows: 7 }} /> : query.isError ? (
        <Alert type="warning" showIcon title="任务暂时无法加载" action={<Button onClick={() => query.refetch()}>重试</Button>} />
      ) : tasks.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={search ? '没有匹配的任务' : pendingOnly ? '暂无待处理事项' : '还没有任务'}>
          {!pendingOnly && canCreate && !search && <Button type="primary" onClick={() => navigate('/workforce/tasks/new')}>发起第一个任务</Button>}
        </Empty>
      ) : <div className="workforce-task-list-table">
        <div className="workforce-task-list-columns" aria-hidden="true">
          <span>任务</span>
          <span>执行环境</span>
          <span>当前进展</span>
          <span>最后更新</span>
          <span>状态</span>
          <span />
        </div>
        <div className="workforce-task-list-rows">
          {tasks.map((task) => <TaskListCard key={task.id} task={task} onOpen={() => open(task)} />)}
        </div>
      </div>}
      {query.hasNextPage&&<Flex justify="center" style={{marginTop:16}}><Button loading={query.isFetchingNextPage} onClick={()=>query.fetchNextPage()}>加载更多</Button></Flex>}
    </Card>

    {!pendingOnly&&<TaskListInsights range={analyticsRange} onRange={setAnalyticsRange} query={analytics} onFilter={(value)=>{const copy=new URLSearchParams(params);if(value.view&&value.view!=='all')copy.set('view',value.view);else copy.delete('view');if(value.from)copy.set('from',value.from);else copy.delete('from');if(value.to)copy.set('to',value.to);else copy.delete('to');setParams(copy,{replace:true})}}/>}
  </main>;
}

function TaskListCard({ task, onOpen }: { task: WorkforceTask; onOpen: () => void }) {
  const pending = task.pendingActionCount ?? 0;
  const tone = taskTone(task.status, pending);
  const runtime = task.selectedRuntime?.name;
  const updateAt = task.finishedAt ?? task.startedAt ?? task.createdAt;
  const progress = task.activitySummary || taskProgressFallback(task.status, pending);
  return <article
    className={`workforce-task-list-row is-${tone}`}
    role="link"
    tabIndex={0}
    aria-label={`查看任务：${task.name}`}
    onClick={onOpen}
    onKeyDown={(event) => {
      if (event.key !== 'Enter' && event.key !== ' ') return;
      event.preventDefault();
      onOpen();
    }}
  >
    <div className="workforce-task-list-main">
      <span className="workforce-task-list-icon" aria-hidden="true">{taskIcon(tone)}</span>
      <div className="workforce-task-list-copy">
        <span className="workforce-task-list-title">{task.name}</span>
        <Paragraph className="workforce-task-list-goal" ellipsis={{ rows: 1, tooltip: task.instruction }}>
          {task.instruction}
        </Paragraph>
      </div>
    </div>
    <div className="workforce-task-list-environment" data-label="执行环境">
      <span className="workforce-task-list-kind"><CodeOutlined />{task.inputKind === 'EMPTY_WORKSPACE' ? '通用任务' : '代码任务'}</span>
      <span className="workforce-task-runtime">{runtime || '镜像信息未记录'}</span>
    </div>
    <div className="workforce-task-list-progress" data-label={pending > 0 ? '等待处理' : '当前进展'}>
      <span className="workforce-task-list-progress-title">
        {pending > 0 ? <><Badge status="warning" />待你处理 · {pending} 项</> : task.hasUnreadUpdates ? <><Badge color="blue" />有新进展</> : statusMeta[task.status].label}
      </span>
      <span className="workforce-task-list-progress-copy" title={progress}>{progress}</span>
    </div>
    <div className="workforce-task-list-update" data-label="最后更新" title={new Date(updateAt).toLocaleString()}>
      <strong>{relativeTime(updateAt)}</strong>
      <span>{formatCompactDate(updateAt)}</span>
    </div>
    <div className="workforce-task-list-state" data-label="状态">
      <StatusTag status={task.status} />
      {(task.hasUnreadUpdates || pending > 0) && <span className="workforce-task-list-signal">{pending > 0 ? '需要操作' : '未读更新'}</span>}
    </div>
    <span className="workforce-task-list-open" aria-hidden="true"><ArrowRightOutlined /></span>
  </article>;
}

function ProjectPicker() {
  const navigate = useNavigate();
  const canCreateProject = usePermissionAccess('workforce:task:create');
  const query = useInfiniteQuery({
    queryKey: ['workforce', 'projects', 'ACTIVE', 'task-start'],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => listProjects({ status: 'ACTIVE', cursor: pageParam, limit: 50 }),
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  });
  const projects = query.data?.pages.flatMap((page) => page.items) ?? [];
  return <main className="workforce-page workforce-new-task">
    <Space orientation="vertical" size={3}>
      <Title level={2}>选择项目</Title>
      <Text type="secondary">任务会使用项目的代码、检查和交付规则。</Text>
    </Space>
    {query.isLoading ? <Skeleton active paragraph={{ rows: 6 }} /> : query.isError ? (
      <Alert type="warning" showIcon title="项目暂时无法加载" action={<Button onClick={() => query.refetch()}>重试</Button>} />
    ) : projects.length === 0 ? (
      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有可用项目">
        {canCreateProject && <Button type="primary" onClick={() => navigate('/workforce/projects/new')}>新建项目</Button>}
      </Empty>
    ) : <div className="workforce-project-task-grid">
      {projects.map((project) => <ProjectChoice key={project.id} project={project} onOpen={() => navigate(`/workforce/projects/${encodeURIComponent(project.id)}/tasks/new`)} />)}
    </div>}
  </main>;
}

function ProjectChoice({ project, onOpen }: { project: WorkforceProject; onOpen: () => void }) {
  const icon = project.kind === 'CODEBASE' ? <BranchesOutlined /> : <FileTextOutlined />;
  return <button type="button" className="workforce-project-task-card" onClick={onOpen}>
    <Flex align="start" justify="space-between" gap={16}>
      <Space align="start">
        <span className="workforce-project-choice-icon">{icon}</span>
        <Space orientation="vertical" size={3}>
          <Text strong>{project.name}</Text>
          <Text type="secondary">{project.description || (project.kind === 'CODEBASE' ? project.repository?.displayUrl : '通用项目')}</Text>
          <Space size={[4, 4]} wrap>{project.techStack.slice(0, 4).map((item) => <Tag key={item} bordered={false}>{item}</Tag>)}</Space>
        </Space>
      </Space>
      <ArrowRightOutlined />
    </Flex>
  </button>;
}

function StatusTag({ status }: { status: TaskStatus }) {
  const meta = statusMeta[status];
  return <Tag color={meta.color} variant="filled">{meta.label}</Tag>;
}

function taskTone(status: TaskStatus, pending: number): 'attention' | 'success' | 'danger' | 'active' | 'muted' {
  if (pending > 0 || status === 'INTERACTION_SUSPENDED' || status === 'MANUAL_REVIEW') return 'attention';
  if (status === 'SUCCEEDED') return 'success';
  if (status === 'FAILED' || status === 'TIMED_OUT') return 'danger';
  if (status === 'CANCELLED') return 'muted';
  return 'active';
}

function taskIcon(tone: ReturnType<typeof taskTone>) {
  if (tone === 'success') return <CheckCircleFilled />;
  if (tone === 'attention' || tone === 'danger') return <ExclamationCircleFilled />;
  return <ClockCircleOutlined />;
}

function taskProgressFallback(status: TaskStatus, pending: number) {
  if (pending > 0) return '任务正在等待你的确认或输入';
  const messages: Partial<Record<TaskStatus, string>> = {
    CREATED: '任务已创建，等待进入执行队列', QUEUED: '正在等待可用执行资源', PREPARING: '正在准备代码和执行环境',
    RUNNING: 'Agent 正在执行任务', INTERACTION_SUSPENDED: 'Agent 正在等待你的回答', VALIDATING: '正在执行完成检查',
    PACKAGING: '正在整理任务结果', UPLOADING: '正在保存任务结果', RETRY_WAIT: '执行失败，等待自动重试', REVIEWING: '正在进行独立审查',
    MANUAL_REVIEW: '需要人工确认任务结果', SUCCEEDED: '任务已经完成', CANCELLING: '正在取消任务', CANCELLED: '任务已取消',
    TIMED_OUT: '任务执行超时', FAILED: '任务执行失败',
  };
  return messages[status] ?? statusMeta[status].label;
}

function formatCompactDate(value?: string) {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '';
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(date);
}

function parseFilter(value: string | null): TaskFilter {
  return value && ['active','succeeded','failed','cancelled','attention','waiting'].includes(value) ? value as TaskFilter : 'all';
}

function relativeTime(value?: string) {
  if (!value) return '刚刚';
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
  if (seconds < 60) return '刚刚';
  if (seconds < 3600) return `${Math.floor(seconds / 60)} 分钟前`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} 小时前`;
  return `${Math.floor(seconds / 86400)} 天前`;
}
