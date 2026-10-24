import {
  ArrowRightOutlined,
  BranchesOutlined,
  ClockCircleOutlined,
  FileTextOutlined,
  PlusOutlined,
} from '@ant-design/icons';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { Button, Card, Empty, Flex, Input, Result, Segmented, Skeleton, Space, Tag, Typography } from 'antd';
import { useDeferredValue, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { listProjects } from '@/api/projectController';
import { getProjectListAnalytics } from '@/api/taskAnalyticsController';
import type { ProjectStatus, WorkforceProject } from '@/api/projectContract';
import { usePermissionAccess } from '@/hooks/auth';
import {
  formatProjectTime,
  projectSourceLabel,
} from './_components/projectPresentation';
import styles from './projects.module.css';
import { ProjectListInsights, ProjectListSummary } from '../_components/AnalyticsDashboard';

const { Text, Title } = Typography;

export default function ProjectsPage() {
  const navigate = useNavigate();
  const canCreate = usePermissionAccess('workforce:task:create');
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<ProjectStatus>('ACTIVE');
  const [analyticsRange, setAnalyticsRange] = useState<'7d' | '30d' | '90d'>('30d');
  const deferredSearch = useDeferredValue(search.trim());
  const query = useInfiniteQuery({
    queryKey: ['workforce', 'projects', status, deferredSearch],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => listProjects({ query: deferredSearch || undefined, status, cursor: pageParam, limit: 20 }),
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  });
  const projects = useMemo(() => query.data?.pages.flatMap((page) => page.items) ?? [], [query.data]);
  const analytics = useQuery({ queryKey: ['workforce','projects','analytics',analyticsRange], queryFn:()=>getProjectListAnalytics(analyticsRange), enabled: status==='ACTIVE' });

  return <main className={styles.page}>
    <Flex className={styles.pageHeader} align="end" justify="space-between" gap={20} wrap>
      <Space orientation="vertical" size={3}>
        <Title level={2}>项目</Title>
        <Text type="secondary">从项目发起任务，代码和完成规则会自动带入。</Text>
      </Space>
      {canCreate && <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/workforce/projects/new')}>新建项目</Button>}
    </Flex>

    {status === 'ACTIVE' && <ProjectListSummary query={analytics} />}

    <Card className={styles.projectListCard}>
      <Flex className={styles.listToolbar} align="center" justify="space-between" gap={16} wrap>
        <Flex align="center" gap={14} wrap>
          <Text strong className={styles.listTitle}>项目列表</Text>
          <Segmented<ProjectStatus>
            size="small"
            value={status}
            onChange={setStatus}
            options={[{ label: '进行中', value: 'ACTIVE' }, { label: '已归档', value: 'ARCHIVED' }]}
          />
        </Flex>
        <Flex className={styles.searchGroup} align="center" gap={12} wrap>
          <Input.Search allowClear value={search} onChange={(event) => setSearch(event.target.value)} placeholder="搜索项目" />
        </Flex>
      </Flex>

      {query.isLoading ? <ProjectGridSkeleton /> : query.isError ? <Result status="warning" title="项目暂时无法加载" extra={<Button onClick={() => query.refetch()}>重试</Button>} /> : projects.length > 0 ? <>
        <div className={styles.projectGrid}>
          {projects.map((project) => <ProjectCard key={project.id} project={project} onOpen={() => navigate(`/workforce/projects/${encodeURIComponent(project.id)}`)} />)}
        </div>
        {query.hasNextPage && <Flex className={styles.loadMore} justify="center"><Button loading={query.isFetchingNextPage} onClick={() => query.fetchNextPage()}>加载更多</Button></Flex>}
      </> : <Empty className={styles.emptyState} image={Empty.PRESENTED_IMAGE_SIMPLE} description={deferredSearch ? '没有匹配的项目' : status === 'ARCHIVED' ? '没有已归档项目' : '还没有项目'}>
        {canCreate && status === 'ACTIVE' && !deferredSearch && <Button type="primary" onClick={() => navigate('/workforce/projects/new')}>新建第一个项目</Button>}
      </Empty>}
    </Card>

    {status === 'ACTIVE' && <ProjectListInsights range={analyticsRange} onRange={setAnalyticsRange} query={analytics} onProject={(id,view)=>navigate(`/workforce/projects/${encodeURIComponent(id)}?tab=tasks&view=${view}`)} onDay={(day,view)=>navigate(`/workforce/tasks?view=${view}&from=${encodeURIComponent(day)}&to=${encodeURIComponent(new Date(new Date(day).getTime()+86400000).toISOString())}`)} />}
  </main>;
}

function ProjectCard({ project, onOpen }: { project: WorkforceProject; onOpen: () => void }) {
  const sourceIcon = project.kind === 'CODEBASE' ? <BranchesOutlined /> : <FileTextOutlined />;
  return <button type="button" className={styles.projectCard} onClick={onOpen}>
    <Flex align="start" justify="space-between" gap={16}>
      <div className={styles.projectIdentity}>
        <Flex align="center" gap={9}>
          <span className={styles.projectIcon}>{sourceIcon}</span>
          <Title level={4}>{project.name}</Title>
        </Flex>
        <Text type="secondary" className={styles.projectDescription}>{project.description || '暂无项目说明'}</Text>
      </div>
      <ArrowRightOutlined className={styles.openIcon} />
    </Flex>
    <Flex className={styles.projectSource} gap={7} align="center">
      {sourceIcon}
      <Text ellipsis={{ tooltip: projectSourceLabel(project) }}>{projectSourceLabel(project)}</Text>
    </Flex>
    <Flex className={styles.stackRow} gap={6} wrap>
      {project.techStack.slice(0, 4).map((stack) => <Tag key={stack} bordered={false}>{stack}</Tag>)}
      {project.techStack.length > 4 && <Tag bordered={false}>+{project.techStack.length - 4}</Tag>}
    </Flex>
    <Flex className={styles.projectStats} justify="space-between" gap={16}>
      <Space size={16}>
        <Text type="secondary"><strong>{project.activeTaskCount}</strong> 进行中</Text>
        <Text type="secondary"><strong>{project.taskCount}</strong> 个任务</Text>
      </Space>
      <Text type="secondary"><ClockCircleOutlined /> {formatProjectTime(project.latestTaskAt)}</Text>
    </Flex>
  </button>;
}

function ProjectGridSkeleton() {
  return <div className={styles.projectGrid}>{[1, 2, 3, 4].map((item) => <Card key={item} className={styles.projectSkeleton}><Skeleton active paragraph={{ rows: 4 }} /></Card>)}</div>;
}
