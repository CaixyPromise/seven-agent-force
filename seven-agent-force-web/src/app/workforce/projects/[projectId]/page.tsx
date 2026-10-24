import {
  ArrowLeftOutlined,
  BranchesOutlined,
  CheckCircleOutlined,
  ClockCircleOutlined,
  EditOutlined,
  FileTextOutlined,
  MoreOutlined,
  PlusOutlined,
  SafetyCertificateOutlined,
  SendOutlined,
} from '@ant-design/icons';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  App,
  Button,
  Card,
  Descriptions,
  Drawer,
  Dropdown,
  Empty,
  Flex,
  Result,
  Skeleton,
  Space,
  Tabs,
  Tag,
  Typography,
  type MenuProps,
} from 'antd';
import { useState } from 'react';
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { archiveProject, getProject, listProjectTasks, updateProject } from '@/api/projectController';
import { getProjectAnalytics } from '@/api/taskAnalyticsController';
import { projectFeedback, type ProjectDraft, type ProjectTaskSummary, type WorkforceProject } from '@/api/projectContract';
import { usePermissionAccess } from '@/hooks/auth';
import ProjectForm from '../_components/ProjectForm';
import {
  completionCheckLabel,
  deliveryLabels,
  formatProjectTime,
  projectSourceLabel,
  projectTaskStatus,
  taskPrimaryText,
} from '../_components/projectPresentation';
import styles from '../projects.module.css';
import { ProjectDashboard } from '../../_components/AnalyticsDashboard';

const { Paragraph, Text, Title } = Typography;

export default function ProjectDetailPage() {
  const { projectId = '' } = useParams();
  const navigate = useNavigate();
  const [searchParams,setSearchParams]=useSearchParams();
  const queryClient = useQueryClient();
  const { message, modal } = App.useApp();
  const canCreateTask = usePermissionAccess('workforce:task:create');
  const canUpdateProject = usePermissionAccess('workforce:task:metadata:update');
  const [editing, setEditing] = useState(false);
  const [analyticsRange,setAnalyticsRange]=useState<'7d'|'30d'|'90d'>('30d');
  const activeTab=['overview','tasks','profile','rules'].includes(searchParams.get('tab')??'')?searchParams.get('tab')!:'overview';
  const taskView=searchParams.get('view')??undefined;const taskFrom=searchParams.get('from')??undefined;const taskTo=searchParams.get('to')??undefined;
  const project = useQuery({
    queryKey: ['workforce', 'project', projectId],
    queryFn: () => getProject(projectId),
    enabled: Boolean(projectId),
  });
  const tasks = useInfiniteQuery({
    queryKey: ['workforce', 'project', projectId, 'tasks',taskView,taskFrom,taskTo],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => listProjectTasks(projectId, { cursor: pageParam, limit: 20,view:taskView,from:taskFrom,to:taskTo }),
    getNextPageParam: (page) => page.nextCursor ?? undefined,
    enabled: Boolean(projectId),
  });
  const analytics=useQuery({queryKey:['workforce','project',projectId,'analytics',analyticsRange],queryFn:()=>getProjectAnalytics(projectId,analyticsRange),enabled:Boolean(projectId)});
  const update = useMutation({
    mutationFn: (input: ProjectDraft) => updateProject(projectId, project.data!.versionId, input),
    onSuccess: async (updated) => {
      update.reset();
      queryClient.setQueryData(['workforce', 'project', projectId], updated);
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'projects'] });
      setEditing(false);
      message.success('项目已更新');
    },
    onError: (error) => message.error(projectFeedback(error)),
  });
  const archive = useMutation({
    mutationFn: () => archiveProject(projectId, project.data!.versionId),
    onSuccess: async (updated) => {
      queryClient.setQueryData(['workforce', 'project', projectId], updated);
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'projects'] });
      message.success('项目已归档');
    },
    onError: (error) => message.error(projectFeedback(error)),
  });

  if (project.isLoading) return <main className={styles.page}><ProjectDetailSkeleton /></main>;
  if (!project.data || project.isError) return <main className={styles.page}><Result status="warning" title="项目暂时无法加载" extra={<Button onClick={() => project.refetch()}>重试</Button>} /></main>;

  const current = project.data;
  const taskCreatePath = `/workforce/projects/${encodeURIComponent(current.id)}/tasks/new`;
  const menuItems: MenuProps['items'] = current.status === 'ACTIVE' && canUpdateProject ? [{ key: 'archive', label: '归档项目' }] : [];

  const handleMenu: MenuProps['onClick'] = ({ key }) => {
    if (key !== 'archive') return;
    modal.confirm({
      title: '归档项目？',
      content: '归档后不能发起新任务，已有任务和结果仍可查看。',
      okText: '归档',
      okButtonProps: { danger: true },
      onOk: () => archive.mutateAsync(),
    });
  };

  return <main className={styles.page}>
    <section className={styles.projectHero}>
      <Flex align="start" justify="space-between" gap={20} wrap>
        <Flex className={styles.heroIdentity} align="start" gap={14}>
          <Button className={styles.backButton} aria-label="返回项目列表" type="text" icon={<ArrowLeftOutlined />} onClick={() => navigate('/workforce/projects')} />
          <Space orientation="vertical" size={6} className={styles.heroCopy}>
            <Flex align="center" gap={9} wrap>
              <Title level={2}>{current.name}</Title>
              {current.status === 'ARCHIVED' && <Tag>已归档</Tag>}
            </Flex>
            {current.description && <Paragraph className={styles.heroDescription}>{current.description}</Paragraph>}
            <Flex gap={8} align="center" wrap>
              <Tag icon={current.kind === 'CODEBASE' ? <BranchesOutlined /> : <FileTextOutlined />} bordered={false}>{projectSourceLabel(current)}</Tag>
              {current.techStack.slice(0, 4).map((stack) => <Tag key={stack} bordered={false}>{stack}</Tag>)}
            </Flex>
          </Space>
        </Flex>
        <Flex gap={8}>
          {current.status === 'ACTIVE' && canCreateTask && <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate(taskCreatePath)}>发起任务</Button>}
          {current.status === 'ACTIVE' && canUpdateProject && <Button icon={<EditOutlined />} onClick={() => setEditing(true)}>编辑</Button>}
          {menuItems.length > 0 && <Dropdown menu={{ items: menuItems, onClick: handleMenu }} trigger={['click']}><Button aria-label="更多项目操作" icon={<MoreOutlined />} /></Dropdown>}
        </Flex>
      </Flex>
      <div className={styles.heroStats}>
        <ProjectMetric label="进行中" value={current.activeTaskCount} />
        <ProjectMetric label="全部任务" value={current.taskCount} />
        <ProjectMetric label="最近任务" value={formatProjectTime(current.latestTaskAt)} />
      </div>
    </section>

    <Tabs
      className={styles.projectTabs}
      activeKey={activeTab}
      onChange={(tab)=>{const copy=new URLSearchParams(searchParams);if(tab==='overview')copy.delete('tab');else copy.set('tab',tab);setSearchParams(copy,{replace:true})}}
      items={[
        { key: 'overview', label: '概览', children: <ProjectDashboard range={analyticsRange} onRange={setAnalyticsRange} query={analytics} onFilter={(value)=>{const copy=new URLSearchParams(searchParams);copy.set('tab','tasks');if(value.view&&value.view!=='all')copy.set('view',value.view);else copy.delete('view');if(value.from)copy.set('from',value.from);else copy.delete('from');if(value.to)copy.set('to',value.to);else copy.delete('to');setSearchParams(copy,{replace:true})}} /> },
        { key: 'tasks', label: '任务', children: <ProjectTasks project={current} tasks={tasks.data?.pages.flatMap((page) => page.items) ?? []} loading={tasks.isLoading} error={tasks.isError} loadingMore={tasks.isFetchingNextPage} hasMore={Boolean(tasks.hasNextPage)} canCreate={canCreateTask} onRetry={() => tasks.refetch()} onLoadMore={() => tasks.fetchNextPage()} onCreate={() => navigate(taskCreatePath)} /> },
        { key: 'profile', label: '项目资料', children: <ProjectProfile project={current} /> },
        { key: 'rules', label: '自动化', children: <ProjectRules project={current} /> },
      ]}
    />

    <Drawer open={editing} title="编辑项目" size={760} onClose={() => setEditing(false)} destroyOnHidden>
      <ProjectForm initialProject={current} submitting={update.isPending} submitText="保存修改" onSubmit={(value) => update.mutateAsync(value)} onCancel={() => setEditing(false)} />
    </Drawer>
  </main>;
}

function ProjectMetric({ label, value, emphasis }: { label: string; value: string | number; emphasis?: boolean }) {
  return <div className={styles.metric}><Text type="secondary">{label}</Text><Text strong className={emphasis ? styles.metricEmphasis : undefined}>{value}</Text></div>;
}

function ProjectTasks({ project, tasks, loading, error, loadingMore, hasMore, canCreate, onRetry, onLoadMore, onCreate }: { project: WorkforceProject; tasks: ProjectTaskSummary[]; loading: boolean; error: boolean; loadingMore: boolean; hasMore: boolean; canCreate: boolean; onRetry: () => void; onLoadMore: () => void; onCreate: () => void }) {
  if (loading) return <Card className={styles.tabCard}><Skeleton active paragraph={{ rows: 6 }} /></Card>;
  if (error) return <Card className={styles.tabCard}><Result status="warning" title="任务暂时无法加载" extra={<Button onClick={onRetry}>重试</Button>} /></Card>;
  if (!tasks.length) return <Card className={styles.tabCard}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这个项目还没有任务">{project.status === 'ACTIVE' && canCreate && <Button type="primary" onClick={onCreate}>发起第一个任务</Button>}</Empty></Card>;
  return <>
    <div className={styles.taskList}>{tasks.map((task) => {
      const status = projectTaskStatus(task.status);
      return <Link className={styles.taskRow} key={task.id} to={`/workforce/tasks/${encodeURIComponent(task.id)}`}>
        <div className={styles.taskMain}>
          <Flex gap={8} align="center" wrap>
            <Text strong>{task.name}</Text>
            <Tag color={status.color} bordered={false}>{status.label}</Tag>
            {task.pendingActionCount > 0 && <Tag color="gold" bordered={false}>{task.pendingActionCount} 项待处理</Tag>}
          </Flex>
          <Text type="secondary" className={styles.taskSummary}>{taskPrimaryText(task)}</Text>
        </div>
        <Text type="secondary" className={styles.taskTime}><ClockCircleOutlined /> {formatProjectTime(task.finishedAt ?? task.createdAt)}</Text>
      </Link>;
    })}</div>
    {hasMore && <Flex className={styles.loadMore} justify="center"><Button loading={loadingMore} onClick={onLoadMore}>加载更多</Button></Flex>}
  </>;
}

function ProjectProfile({ project }: { project: WorkforceProject }) {
  return <div className={styles.detailGrid}>
    <Card className={styles.tabCard} title="基本信息">
      <Descriptions column={{ xs: 1, sm: 2 }} items={[
        { key: 'name', label: '项目名称', children: project.name, span: 2 },
        { key: 'department', label: '所属部门', children: project.departmentName || '未设置' },
        { key: 'kind', label: '项目类型', children: project.kind === 'CODEBASE' ? '代码项目' : '通用项目' },
        { key: 'source', label: '代码来源', children: projectSourceLabel(project), span: 2 },
      ]} />
    </Card>
    <Card className={styles.tabCard} title="技术栈">
      <Flex gap={8} wrap>{project.techStack.length ? project.techStack.map((stack) => <Tag key={stack}>{stack}</Tag>) : <Text type="secondary">未设置</Text>}</Flex>
    </Card>
    <Card className={`${styles.tabCard} ${styles.fullWidth}`} title="项目指引">
      <Paragraph className={styles.preservedText}>{project.agentGuidance || '未设置项目指引'}</Paragraph>
    </Card>
  </div>;
}

function ProjectRules({ project }: { project: WorkforceProject }) {
  const rules = [
    { key: 'check', icon: <CheckCircleOutlined />, title: '完成检查', value: completionCheckLabel(project), detail: project.completionCheck.instructions },
    { key: 'result', icon: <FileTextOutlined />, title: '结果处理', value: project.resultProcessingSummary || '保存任务结果' },
    { key: 'review', icon: <SafetyCertificateOutlined />, title: '独立审查提示词', value: project.reviewPrompt || '使用平台默认要求' },
    { key: 'hooks', icon: <SafetyCertificateOutlined />, title: 'Agent Hooks', value: project.hooks.length ? project.hooks.map((hook) => `${hook.name} v${hook.version}`).join(' → ') : '未配置', detail: project.hooks.length ? '按显示顺序执行' : undefined },
    { key: 'delivery', icon: <BranchesOutlined />, title: 'Git 交付', value: deliveryLabels[project.delivery.mode], detail: project.delivery.mode === 'NONE' ? undefined : project.delivery.authorization === 'AUTO_AFTER_GATES' ? '检查和审查通过后自动交付' : '完成后确认' },
    { key: 'notification', icon: <SendOutlined />, title: '通知', value: project.notification.enabled ? (project.notification.groupRouteName || '已启用') : '未启用' },
  ];
  return <div className={styles.ruleGrid}>{rules.map((rule) => <Card key={rule.key} className={styles.ruleCard}>
    <Flex gap={12} align="start">
      <span className={styles.ruleIcon}>{rule.icon}</span>
      <Space orientation="vertical" size={4} className={styles.ruleCopy}>
        <Text type="secondary">{rule.title}</Text>
        <Text strong className={styles.ruleValue}>{rule.value}</Text>
        {rule.detail && <Text type="secondary" className={styles.preservedText}>{rule.detail}</Text>}
      </Space>
    </Flex>
  </Card>)}</div>;
}

function ProjectDetailSkeleton() {
  return <><Card><Skeleton active avatar paragraph={{ rows: 3 }} /></Card><Card><Skeleton active paragraph={{ rows: 8 }} /></Card></>;
}
