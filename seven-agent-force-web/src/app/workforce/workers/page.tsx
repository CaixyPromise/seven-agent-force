import {
  ArrowRightOutlined,
  ClockCircleOutlined,
  CloudServerOutlined,
  CodeSandboxOutlined,
  DatabaseOutlined,
  DeleteOutlined,
  EditOutlined,
  HddOutlined,
  KeyOutlined,
  PauseCircleOutlined,
  PlusOutlined,
  PoweroffOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SearchOutlined,
  ThunderboltOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  App as AntdApp,
  Badge,
  Button,
  Card,
  Collapse,
  Descriptions,
  Drawer,
  Empty,
  Flex,
  Form,
  Input,
  InputNumber,
  Modal,
  Progress,
  Row,
  Segmented,
  Select,
  Col,
  Space,
  Statistic,
  Table,
  Tag,
  Tabs,
  Typography,
  type TableColumnsType,
} from 'antd';
import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import {
  createWorkforceWorker,
  decommissionWorkforceWorker,
  getWorkforceWorker,
  getWorkforceWorkerMetrics,
  listWorkforceWorkerAssignments,
  listWorkforceWorkers,
  rotateWorkforceWorkerToken,
  refreshWorkforceWorkerInventory,
  setWorkforceWorkerDesiredState,
  updateWorkforceWorkerCapacity,
  type WorkforceAssignment,
  type WorkforceWorker,
  type WorkforceWorkerImage,
  type WorkforceWorkerMetricRange,
  type WorkforceWorkerMetricSeries,
  type WorkforceWorkerQueue,
  type WorkforceReconciliation,
} from '@/api/workforceController';
import { WorkerResourceChart, WorkerSlotChart } from './WorkerFleetCharts';
import styles from './workers.module.css';

const { Paragraph, Text, Title } = Typography;

const stateMeta: Record<string, { label: string; color: string; status?: 'success' | 'processing' | 'warning' | 'error' | 'default' }> = {
  ONLINE: { label: '在线', color: 'green', status: 'success' },
  DRAINING: { label: '排空中', color: 'orange', status: 'warning' },
  DRAINED: { label: '已排空', color: 'cyan', status: 'default' },
  STALE: { label: '心跳过期', color: 'red', status: 'error' },
  OFFLINE: { label: '离线', color: 'default', status: 'default' },
  DISABLED: { label: '已停用', color: 'default', status: 'default' },
  PROVISIONED: { label: '待注册', color: 'blue', status: 'processing' },
  RECONCILING: { label: '等待接管', color: 'gold', status: 'warning' },
  DECOMMISSIONED: { label: '已退役', color: 'default', status: 'default' },
};

const consumerStateLabel: Record<string, string> = {
  STARTING: '启动中',
  CONSUMING: '消费中',
  STOPPED: '已停止',
  ERROR: '异常',
};

const assignmentStateLabel: Record<string, string> = {
  OFFERED: '待领取',
  CLAIMED: '已领取',
  COMPLETED: '已完成',
  EXPIRED: '已过期',
  RELEASED: '已释放',
  CANCELLED: '已取消',
  FAILED: '失败',
};

type Credential = { token: string; tokenHint: string };

export default function WorkforceWorkersPage() {
  const { message, modal } = AntdApp.useApp();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [createOpen, setCreateOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string>();
  const [credential, setCredential] = useState<Credential>();
  const [query, setQuery] = useState('');
  const [stateFilter, setStateFilter] = useState('ALL');
  const [metricRange, setMetricRange] = useState<WorkforceWorkerMetricRange>('24h');
  const [form] = Form.useForm<{ name: string; description?: string; maxActiveAttempts: number }>();
  const workers = useQuery({ queryKey: ['workforce', 'workers'], queryFn: listWorkforceWorkers, refetchInterval: 5_000 });
  const detail = useQuery({ queryKey: ['workforce', 'worker', selectedId], queryFn: () => getWorkforceWorker(selectedId!), enabled: !!selectedId, refetchInterval: 5_000 });
  const assignments = useQuery({ queryKey: ['workforce', 'worker', selectedId, 'assignments'], queryFn: () => listWorkforceWorkerAssignments(selectedId!), enabled: !!selectedId, refetchInterval: 5_000 });
  const fleetMetrics = useQuery({
    queryKey: ['workforce', 'worker-metrics', 'fleet', metricRange],
    queryFn: () => getWorkforceWorkerMetrics(metricRange),
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const selectedMetrics = useQuery({
    queryKey: ['workforce', 'worker-metrics', selectedId, metricRange],
    queryFn: () => getWorkforceWorkerMetrics(metricRange, selectedId!),
    enabled: !!selectedId,
    staleTime: 60_000,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const refresh = async () => {
    await queryClient.invalidateQueries({ queryKey: ['workforce', 'workers'] });
    if (selectedId) await queryClient.invalidateQueries({ queryKey: ['workforce', 'worker', selectedId] });
  };
  const create = useMutation({
    mutationFn: createWorkforceWorker,
    onSuccess: async (result) => {
      setCreateOpen(false);
      setCredential(result.credential);
      setSelectedId(result.worker.id);
      form.resetFields();
      await refresh();
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '创建 Worker 失败'),
  });
  const operate = useMutation({
    mutationFn: ({ worker, desiredState }: { worker: WorkforceWorker; desiredState: 'ENABLED' | 'DRAINING' | 'DISABLED' }) => setWorkforceWorkerDesiredState(worker, desiredState),
    onSuccess: refresh,
    onError: (error) => message.error(error instanceof Error ? error.message : 'Worker 状态更新失败'),
  });
  const updateCapacity = useMutation({
    mutationFn: ({ worker, maximum }: { worker: WorkforceWorker; maximum: number }) => updateWorkforceWorkerCapacity(worker, maximum),
    onSuccess: async () => {
      message.success('最大并发数已更新');
      await refresh();
    },
    onError: (error) => message.error(workerCapacityFeedback(error)),
  });
  const rotate = useMutation({
    mutationFn: rotateWorkforceWorkerToken,
    onSuccess: (next) => { setCredential(next); refresh(); },
    onError: (error) => message.error(error instanceof Error ? error.message : 'Token 轮换失败'),
  });
  const refreshInventory = useMutation({
    mutationFn: refreshWorkforceWorkerInventory,
    onSuccess: async () => { message.success('已通知 Worker 刷新本机协议镜像'); await refresh(); },
    onError: (error) => message.error(error instanceof Error ? error.message : '请求刷新镜像失败'),
  });
  const decommission = useMutation({
    mutationFn: decommissionWorkforceWorker,
    onSuccess: async (result) => { message.success(`Worker 已退役，主队列已清理（${result.purgedMessages} 条待处理通知）`); await refresh(); },
    onError: (error) => message.error(error instanceof Error ? error.message : 'Worker 退役失败'),
  });

  const worker = detail.data?.worker;
  const workerItems = useMemo(() => workers.data ?? [], [workers.data]);
  const summary = useMemo(() => fleetSummary(workerItems), [workerItems]);
  const visibleWorkers = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    return workerItems.filter((item) => {
      if (stateFilter !== 'ALL' && item.state !== stateFilter) return false;
      if (!normalized) return true;
      return [item.name, item.description, item.hostname, item.observedRemoteIp, item.reportedIp].some((value) => value?.toLowerCase().includes(normalized));
    });
  }, [query, stateFilter, workerItems]);
  const configSnippet = credential && worker ? `workforce:\n  worker:\n    id: ${worker.id}\n    serverBaseURL: http://127.0.0.1:9277\n    tokenFile: /secure/path/seven-worker.token\n    workspaceRoot: /var/lib/seven-worker/workspace\n    maxActiveAttempts: ${worker.maxActiveAttempts}` : '';

  return (
    <div className={styles.page}>
      <Flex justify="space-between" align="flex-end" gap={16} wrap>
        <Space orientation="vertical" size={2}>
          <Title level={2}>Worker 管理</Title>
          <Text type="secondary">查看执行节点、容量与任务负载</Text>
        </Space>
        <Space>
          <Button icon={<ReloadOutlined />} loading={workers.isFetching} onClick={() => workers.refetch()}>刷新</Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => { form.setFieldsValue({ maxActiveAttempts: 1 }); setCreateOpen(true); }}>创建 Worker</Button>
        </Space>
      </Flex>
      <section className={styles.summaryGrid} aria-label="Worker 集群概览">
        <FleetStat icon={<CloudServerOutlined />} label="在线 Worker" value={summary.online} tone="blue" />
        <FleetStat icon={<ThunderboltOutlined />} label="运行中" value={summary.running} tone="green" />
        <FleetStat icon={<DatabaseOutlined />} label="可用槽位" value={summary.available} tone="cyan" />
        <FleetStat icon={<HddOutlined />} label="平均负载" value={summary.load} suffix="%" tone="purple" />
      </section>

      <section className={styles.chartGrid} aria-label="Worker 运行分析">
        <Card className={styles.chartCard} title="资源趋势" extra={<MetricToolbar range={metricRange} loading={fleetMetrics.isFetching} onRange={setMetricRange} onRefresh={() => fleetMetrics.refetch()} />}>
          <MetricRegion query={fleetMetrics} render={(series) => <WorkerResourceChart series={series} className={styles.resourceChart} />} />
        </Card>
        <Card className={styles.chartCard} title="槽位趋势">
          <MetricRegion query={fleetMetrics} render={(series) => <WorkerSlotChart series={series} className={styles.capacityChart} />} />
        </Card>
      </section>

      <section className={styles.workerSection} aria-labelledby="worker-list-title">
        <Flex justify="space-between" align="center" gap={12} wrap>
          <Title id="worker-list-title" level={4}>Worker 列表 <Text type="secondary">({workerItems.length})</Text></Title>
          <Flex gap={10} wrap className={styles.filters}>
            <Input allowClear prefix={<SearchOutlined />} placeholder="搜索名称、主机或地址" value={query} onChange={(event) => setQuery(event.target.value)} />
            <Select
              className={styles.stateSelect}
              aria-label="筛选 Worker 状态"
              value={stateFilter}
              onChange={setStateFilter}
              options={[
                { value: 'ALL', label: '全部状态' },
                ...Object.entries(stateMeta).map(([value, meta]) => ({ value, label: meta.label })),
              ]}
            />
          </Flex>
        </Flex>
        {workers.isLoading ? <div className={styles.loadingGrid}><Card loading /><Card loading /></div> : visibleWorkers.length ? (
          <div className={styles.workerGrid}>
            {visibleWorkers.map((item) => <WorkerCard key={item.id} worker={item} selected={item.id === selectedId} onOpen={() => setSelectedId(item.id)} />)}
          </div>
        ) : <Card className={styles.emptyCard}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={workerItems.length ? '没有符合条件的 Worker' : '还没有 Worker'} /></Card>}
      </section>

      <Modal title="创建 Worker" open={createOpen} footer={null} destroyOnHidden onCancel={() => setCreateOpen(false)}>
        <Form form={form} layout="vertical" onFinish={(values) => create.mutate(values)}>
          <Form.Item name="name" label="名称" rules={[{ required: true }, { max: 120 }]}><Input placeholder="本地开发 Worker" /></Form.Item>
          <Form.Item name="description" label="说明" rules={[{ max: 1000 }]}><Input.TextArea rows={3} placeholder="所在机器或用途" /></Form.Item>
          <Form.Item name="maxActiveAttempts" label="最大并发" rules={[{ required: true }]}><InputNumber min={1} max={64} className={styles.fullWidth} /></Form.Item>
          <Flex justify="flex-end" gap={8}><Button onClick={() => setCreateOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={create.isPending}>创建</Button></Flex>
        </Form>
      </Modal>

      <Modal title="保存 Worker Token" open={!!credential} closable={false} mask={{ closable: false }} footer={<Button type="primary" onClick={() => setCredential(undefined)}>我已安全保存</Button>}>
        <Alert type="warning" showIcon title="关闭后无法再次查看" description="Token 只显示一次。保存为权限 0600 的独立文件；不要放进 YAML、Git、日志或容器环境。" />
        <Paragraph copyable={{ text: credential?.token }} className={styles.secret}>{credential?.token}</Paragraph>
        {configSnippet && <Paragraph><Text type="secondary">Worker 配置片段</Text><pre className={styles.code}>{configSnippet}</pre></Paragraph>}
      </Modal>

      <Drawer title={worker ? <DrawerTitle worker={worker} /> : 'Worker 详情'} open={!!selectedId} width="min(620px, 100vw)" onClose={() => setSelectedId(undefined)} destroyOnHidden extra={<Button icon={<ReloadOutlined />} onClick={() => detail.refetch()}>刷新</Button>}>
        {worker && <WorkerDetail worker={worker} images={detail.data?.images ?? []} assignments={assignments.data ?? []} queue={detail.data?.queue} reconciliation={detail.data?.reconciliation} metricRange={metricRange} metrics={selectedMetrics} updatingCapacity={updateCapacity.isPending} onUpdateCapacity={(maximum) => updateCapacity.mutateAsync({ worker, maximum })} onViewHistory={() => navigate(`/workforce/tasks?q=${encodeURIComponent(worker.id)}`)} onOperate={(desiredState) => operate.mutate({ worker, desiredState })} onRotate={() => modal.confirm({ title: '立即轮换 Worker Token？', content: '旧 Token 会立即失效。当前 Worker 需要更新 Token 文件后重新注册。', okText: '确认轮换', onOk: () => rotate.mutateAsync(worker.id) })} onRefreshInventory={() => refreshInventory.mutate(worker.id)} onDecommission={() => modal.confirm({ title: '退役这个 Worker？', content: '只允许已停用且没有有效 Assignment 或 Runtime 的 Worker 退役。主任务队列和 binding 会删除，DLQ 与全部历史执行记录保留。', okText: '确认退役', okButtonProps: { danger: true }, onOk: () => decommission.mutateAsync(worker) })} />}
      </Drawer>
    </div>
  );
}

function MetricToolbar({ range, loading, onRange, onRefresh }: { range: WorkforceWorkerMetricRange; loading: boolean; onRange: (value: WorkforceWorkerMetricRange) => void; onRefresh: () => unknown }) {
  return <Flex gap={8} align="center" wrap className={styles.metricToolbar}>
    <Segmented size="small" value={range} options={[{ value: '1h', label: '1 小时' }, { value: '6h', label: '6 小时' }, { value: '24h', label: '24 小时' }, { value: '7d', label: '7 天' }, { value: '30d', label: '30 天' }]} onChange={(value) => onRange(value as WorkforceWorkerMetricRange)} />
    <Button aria-label="刷新历史负载" size="small" type="text" icon={<ReloadOutlined spin={loading} />} onClick={onRefresh} />
  </Flex>;
}

type MetricQuery = { data?: WorkforceWorkerMetricSeries; isLoading: boolean; isError: boolean; refetch: () => unknown };
function MetricRegion({ query, render }: { query: MetricQuery; render: (series: WorkforceWorkerMetricSeries) => ReactNode }) {
  if (query.isLoading) return <div className={styles.metricState}><Card loading bordered={false} /></div>;
  if (query.isError) return <div className={styles.metricState}><Alert type="warning" showIcon title="负载趋势暂时无法加载" action={<Button size="small" onClick={() => query.refetch()}>重试</Button>} /></div>;
  if (!query.data?.points.length) return <div className={styles.metricState}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="这段时间还没有负载样本" /></div>;
  return render(query.data);
}

function FleetStat({ icon, label, value, suffix, tone }: { icon: ReactNode; label: string; value: number; suffix?: string; tone: 'blue' | 'green' | 'cyan' | 'purple' }) {
  return <Card className={styles.summaryCard}>
    <Flex align="center" gap={12}>
      <span className={`${styles.summaryIcon} ${styles[tone]}`}>{icon}</span>
      <div><Statistic title={label} value={value} suffix={suffix} /><Text className={styles.realtimeBadge}>实时状态</Text></div>
    </Flex>
  </Card>;
}

function WorkerCard({ worker, selected, onOpen }: { worker: WorkforceWorker; selected: boolean; onOpen: () => void }) {
  const resources = worker.resourceSnapshot ?? {};
  const running = worker.reservedAttempts + worker.startingAttempts + worker.runningAttempts;
  return <button type="button" className={`${styles.workerCard} ${selected ? styles.workerCardSelected : ''}`} onClick={onOpen} aria-label={`查看 ${worker.name} 详情`}>
    <div className={styles.workerCardHeader}>
      <strong className={styles.workerCardName}>{worker.name}</strong>
      <Flex align="center" gap={10}>
        <Badge status={stateMeta[worker.state]?.status ?? 'default'} text={stateMeta[worker.state]?.label ?? worker.state} />
        <Tag color={worker.consumerState === 'CONSUMING' ? 'green' : worker.consumerState === 'ERROR' ? 'red' : 'default'}>{consumerStateLabel[worker.consumerState ?? ''] ?? '已停止'}</Tag>
        <ArrowRightOutlined className={styles.cardArrow} />
      </Flex>
    </div>
    <div className={styles.workerCardBody}>
      <div className={styles.cardFacts}>
        <WorkerFact icon={<DatabaseOutlined />} label="主机地址" value={worker.observedRemoteIp || worker.reportedIp || worker.hostname || '等待连接'} />
        <WorkerFact icon={<ClockCircleOutlined />} label="心跳时间" value={relativeTime(worker.lastHeartbeatAt)} />
        <WorkerFact icon={<CodeSandboxOutlined />} label="版本" value={worker.workerVersion || '开发构建'} />
      </div>
      <div className={styles.cardRuntime}>
        <CompactMetric label="CPU" value={resources.cpuUsagePercent} available={resources.cpuUsageAvailable ?? resources.cpuUsagePercent !== undefined} />
        <CompactMetric label="内存" value={resources.memoryUsagePercent} />
        <CompactMetric label="磁盘" value={resources.workspaceDiskUsagePercent} />
        <div className={styles.cardCapacity}><SafetyCertificateOutlined /><Text>容量</Text><strong>{running} 运行 / {worker.availableAttempts} 可用</strong></div>
        <Flex gap={8} wrap className={styles.capabilityTags}>
          <Tag color={worker.dockerReady ? 'green' : 'red'}>{worker.dockerReady ? 'Docker' : 'Docker 未就绪'}</Tag>
          <Tag>{worker.imageCount} 个镜像</Tag>
        </Flex>
      </div>
    </div>
  </button>;
}

function WorkerFact({ icon, label, value }: { icon: ReactNode; label: string; value: string }) {
  return <div className={styles.workerFact}><span className={styles.workerFactIcon}>{icon}</span><div><Text type="secondary">{label}</Text><strong>{value}</strong></div></div>;
}

function CompactMetric({ label, value, available = value !== undefined }: { label: string; value?: number; available?: boolean }) {
  const safe = Math.max(0, Math.min(100, value ?? 0));
  return <div className={styles.compactMetric}>
    <Text className={styles.metricLabel}>{label}</Text>
    <Progress percent={available ? Math.round(safe) : 0} showInfo={false} size="small" strokeColor={safe >= 90 ? '#ff4d4f' : safe >= 75 ? '#faad14' : '#1677ff'} />
    <Text className={styles.metricValue}>{available ? `${Math.round(safe)}%` : '—'}</Text>
  </div>;
}

function DrawerTitle({ worker }: { worker: WorkforceWorker }) {
  return <Space orientation="vertical" size={4}>
    <Text strong className={styles.drawerTitle}>{worker.name}</Text>
    <Space size={6}><Badge status={stateMeta[worker.state]?.status ?? 'default'} text={stateMeta[worker.state]?.label ?? worker.state} /><Tag color={worker.consumerState === 'CONSUMING' ? 'green' : 'default'}>{consumerStateLabel[worker.consumerState ?? ''] ?? '已停止'}</Tag></Space>
  </Space>;
}

type WorkerDetailProps = {
  worker: WorkforceWorker;
  images: WorkforceWorkerImage[];
  assignments: WorkforceAssignment[];
  queue?: WorkforceWorkerQueue;
  reconciliation?: WorkforceReconciliation;
  metricRange: WorkforceWorkerMetricRange;
  metrics: MetricQuery;
  updatingCapacity: boolean;
  onUpdateCapacity: (maximum: number) => Promise<unknown>;
  onViewHistory: () => void;
  onOperate: (state: 'ENABLED' | 'DRAINING' | 'DISABLED') => void;
  onRotate: () => void;
  onRefreshInventory: () => void;
  onDecommission: () => void;
};

function WorkerDetail({ worker, images, assignments, queue, reconciliation, metricRange, metrics, updatingCapacity, onUpdateCapacity, onViewHistory, onOperate, onRotate, onRefreshInventory, onDecommission }: WorkerDetailProps) {
  const resources = worker.resourceSnapshot ?? {};
  const schedulableImages = images.filter((image) => image.role === 'attempt' && image.admissionStatus === 'ADMITTED' && image.presenceStatus === 'PRESENT');
  const platformImages = images.filter((image) => image.role === 'provider-proxy' && image.presenceStatus === 'PRESENT');
  const otherLocalImages = images.filter((image) => image.role === 'attempt' && image.admissionStatus !== 'ADMITTED' && image.presenceStatus === 'PRESENT');
  const assignmentColumns: TableColumnsType<WorkforceAssignment> = [
    { title: '任务（Task）', dataIndex: 'taskId', ellipsis: true, render: (value) => <Link className={`${styles.mono} ${styles.assignmentLink}`} to={`/workforce/tasks/${encodeURIComponent(value)}`}>{value}</Link> },
    { title: '状态', dataIndex: 'status', width: 110, render: (value) => <Tag>{assignmentStateLabel[value] ?? value}</Tag> },
    { title: '执行尝试（Attempt）', dataIndex: 'attemptId', ellipsis: true, render: (value, assignment) => value ? <Link className={`${styles.mono} ${styles.assignmentLink}`} to={`/workforce/tasks/${encodeURIComponent(assignment.taskId)}?tab=activity`}>{value}</Link> : <Text type="secondary">—</Text> },
  ];
  const overview = <Space orientation="vertical" size={16} className={styles.drawerContent}>
    {reconciliation?.required && (
      <Card size="small" title="运行环境接管" extra={<Tag color="gold">等待接管</Tag>}>
        <Text type="secondary">核对完成后继续接收任务。</Text>
        <Table className={styles.reconciliationTable} rowKey="attemptId" size="small" pagination={false} scroll={{ x: 640 }} dataSource={reconciliation.runtimes} columns={[
          { title: '任务', dataIndex: 'taskId', width: 190, render: (value: string) => <Link className={styles.assignmentLink} to={`/workforce/tasks/${encodeURIComponent(value)}`}>{value}</Link> },
          { title: '状态', dataIndex: 'runtimeStatus', width: 120, render: (value: string) => <Tag color="gold">{runtimeStateLabel(value)}</Tag> },
          { title: '接管截止', dataIndex: 'reattachDeadline', width: 180, render: (value: string) => value ? new Date(value).toLocaleString() : '—' },
        ]} />
      </Card>
    )}
    <Card size="small" title="运行状态">
      <Descriptions bordered size="small" column={2} className={styles.descriptions}>
        <Descriptions.Item label="主机地址">{worker.observedRemoteIp || worker.reportedIp || worker.hostname || '—'}</Descriptions.Item>
        <Descriptions.Item label="心跳时间">{relativeTime(worker.lastHeartbeatAt)}</Descriptions.Item>
        <Descriptions.Item label="Worker 版本">{worker.workerVersion || '开发构建'}</Descriptions.Item>
        <Descriptions.Item label="消费状态">{consumerStateLabel[worker.consumerState ?? ''] ?? '已停止'}</Descriptions.Item>
      </Descriptions>
      <CapacityEditor worker={worker} loading={updatingCapacity} onSave={onUpdateCapacity} />
    </Card>
    <Card size="small" title="资源使用">
      <Row gutter={[12, 12]}>
        <Col xs={24} md={8}><MetricBlock label="CPU" value={resources.cpuUsagePercent} available={resources.cpuUsageAvailable ?? resources.cpuUsagePercent !== undefined} detail={`${resources.logicalCpuCores ?? 0} 核 · Load ${formatLoad(resources.load1)}`} /></Col>
        <Col xs={24} md={8}><MetricBlock label="内存" value={resources.memoryUsagePercent} detail={formatUsage(resources.memoryUsedBytes, resources.memoryTotalBytes)} /></Col>
        <Col xs={24} md={8}><MetricBlock label="工作区磁盘" value={resources.workspaceDiskUsagePercent} detail={formatUsage(resources.workspaceDiskUsedBytes, resources.workspaceDiskTotalBytes)} /></Col>
      </Row>
    </Card>
    <Card size="small" title="负载趋势" extra={<Text type="secondary">最近 {metricRange === '24h' ? '24 小时' : metricRange}</Text>}>
      <MetricRegion query={metrics} render={(series) => <WorkerResourceChart series={series} className={styles.drawerMetricChart} />} />
    </Card>
    <Card size="small" title="任务容量">
      <Descriptions size="small" column={2} className={styles.compactDescriptions}>
        <Descriptions.Item label="已预留">{worker.reservedAttempts}</Descriptions.Item>
        <Descriptions.Item label="启动中">{worker.startingAttempts}</Descriptions.Item>
        <Descriptions.Item label="运行中">{worker.runningAttempts}</Descriptions.Item>
        <Descriptions.Item label="可用槽位">{worker.availableAttempts}</Descriptions.Item>
        <Descriptions.Item label="待处理收尾">{worker.pendingFinalizations}</Descriptions.Item>
        <Descriptions.Item label="待处理上传">{worker.pendingArtifactUploads}</Descriptions.Item>
      </Descriptions>
    </Card>
    <Card size="small" title={`${schedulableImages.length} 个可调度执行镜像`}>
      <div className={styles.imageSummaryList}>
        {schedulableImages.slice(0, 3).map((image) => (
          <div key={`${image.role}:${image.imageDigest}`} className={styles.imageSummaryItem}>
            <div><Text strong ellipsis>{image.imageRef}</Text><TechnicalValue value={image.imageDigest} /></div>
            <Tag color="green">已准入</Tag>
          </div>
        ))}
        {!schedulableImages.length && <Text type="secondary">暂无可调度执行镜像</Text>}
      </div>
    </Card>
    <Card size="small" title="连接信息">
      <Descriptions bordered size="small" column={2} className={styles.descriptions}>
        <Descriptions.Item label="连接地址">{worker.observedRemoteIp || '—'}</Descriptions.Item>
        <Descriptions.Item label="上报地址">{worker.reportedIp || '—'}</Descriptions.Item>
        <Descriptions.Item label="主接口">{worker.primaryInterface || '—'}</Descriptions.Item>
        <Descriptions.Item label="MAC 地址"><TechnicalValue value={worker.primaryMac} /></Descriptions.Item>
      </Descriptions>
    </Card>
  </Space>;

  const taskPanel = <Card size="small" title="最近任务分配" extra={<Button type="link" onClick={onViewHistory}>查看全部</Button>} styles={{ body: { padding: 0 } }}>
    <Table<WorkforceAssignment> rowKey="id" size="small" pagination={false} tableLayout="fixed" scroll={{ x: 560 }} dataSource={assignments} columns={assignmentColumns} locale={{ emptyText: '暂无任务分配' }} />
  </Card>;

  const admittedColumns: TableColumnsType<WorkforceWorkerImage> = [
    { title: '镜像', dataIndex: 'imageRef', ellipsis: true },
    { title: '状态', dataIndex: 'admissionStatus', width: 90, render: () => <Tag color="green">已准入</Tag> },
  ];
  const rejectedColumns: TableColumnsType<WorkforceWorkerImage> = [
    { title: '镜像', dataIndex: 'imageRef', ellipsis: true },
    { title: '状态', dataIndex: 'admissionStatus', width: 128, render: (_, image) => <ImageAdmissionStatus image={image} /> },
    { title: '原因', key: 'reason', width: 210, render: (_, image) => <Text type="secondary">{imageAdmissionReason(image)}</Text> },
  ];
  const imagePanel = <Space orientation="vertical" size={12} className={styles.drawerContent}>
    <Card
      size="small"
      title={`${schedulableImages.length} 个可调度执行镜像`}
      extra={<Button size="small" icon={<ReloadOutlined />} onClick={onRefreshInventory}>刷新镜像</Button>}
      styles={{ body: { padding: 0 } }}
    >
      <Table<WorkforceWorkerImage>
        rowKey="imageDigest"
        size="small"
        pagination={false}
        tableLayout="fixed"
        dataSource={schedulableImages}
        columns={admittedColumns}
        locale={{ emptyText: '暂无可调度执行镜像' }}
      />
    </Card>
    {platformImages.length > 0 && <Card size="small" title={`平台组件 (${platformImages.length})`} styles={{ body: { padding: 0 } }}>
      <Table<WorkforceWorkerImage>
        rowKey="imageDigest"
        size="small"
        pagination={false}
        tableLayout="fixed"
        dataSource={platformImages}
        columns={platformImages.some((image) => image.admissionStatus !== 'ADMITTED') ? rejectedColumns : admittedColumns}
      />
    </Card>}
    {otherLocalImages.length > 0 && <Collapse
      className={styles.localImageCollapse}
      items={[{
        key: 'other-local-images',
        label: <Flex align="center" gap={8}><Text strong>其他本地镜像</Text><Tag>{otherLocalImages.length}</Tag></Flex>,
        children: <Table<WorkforceWorkerImage>
          rowKey="imageDigest"
          size="small"
          pagination={false}
          tableLayout="fixed"
          dataSource={otherLocalImages}
          columns={rejectedColumns}
        />,
      }]}
    />}
  </Space>;

  const connectionPanel = <Space orientation="vertical" size={16} className={styles.drawerContent}>
    <Card size="small" title="身份与连接">
      <Descriptions bordered size="small" column={1} className={styles.descriptions}>
        <Descriptions.Item label="Worker ID"><TechnicalValue value={worker.id} copyable /></Descriptions.Item>
        <Descriptions.Item label="进程实例"><TechnicalValue value={worker.currentInstanceId} /></Descriptions.Item>
        <Descriptions.Item label="主机启动标识"><TechnicalValue value={worker.hostBootId} /></Descriptions.Item>
        <Descriptions.Item label="连接地址">{worker.observedRemoteIp || '—'}</Descriptions.Item>
        <Descriptions.Item label="上报地址">{worker.reportedIp || '—'}</Descriptions.Item>
        <Descriptions.Item label="主接口 / MAC"><Space wrap><Text>{worker.primaryInterface || '—'}</Text><TechnicalValue value={worker.primaryMac} copyable /></Space></Descriptions.Item>
        <Descriptions.Item label="Worker 版本">{worker.workerVersion || '—'}</Descriptions.Item>
        <Descriptions.Item label="构建提交"><TechnicalValue value={displayBuildRevision(worker.workerRevision)} /></Descriptions.Item>
      </Descriptions>
    </Card>
    <Card size="small" title="任务队列">
      <Descriptions bordered size="small" column={1} className={styles.descriptions}>
        <Descriptions.Item label="队列"><TechnicalValue value={queue?.queue} copyable /></Descriptions.Item>
        <Descriptions.Item label="路由键"><TechnicalValue value={queue?.routingKey} /></Descriptions.Item>
        <Descriptions.Item label="失败队列"><TechnicalValue value={queue?.deadQueue} /></Descriptions.Item>
        <Descriptions.Item label="消息合同 / Prefetch"><Space wrap><Tag>{queue?.messageSchema || '—'}</Tag><Text>{queue?.maxPrefetch ?? '—'}</Text></Space></Descriptions.Item>
      </Descriptions>
    </Card>
  </Space>;

  const configPanel = <Space orientation="vertical" size={16} className={styles.drawerContent}>
    <Flex gap={8} wrap className={styles.drawerActions}>
      {(worker.state === 'DRAINING' || worker.state === 'DRAINED') && <Button icon={<SafetyCertificateOutlined />} onClick={() => onOperate('ENABLED')}>恢复消费</Button>}
      {worker.state === 'ONLINE' && <Button icon={<PauseCircleOutlined />} onClick={() => onOperate('DRAINING')}>排空任务</Button>}
      {(worker.state === 'DRAINED' || worker.state === 'OFFLINE') && <Button danger icon={<PoweroffOutlined />} onClick={() => onOperate('DISABLED')}>停用</Button>}
      {worker.state !== 'DECOMMISSIONED' && <Button icon={<KeyOutlined />} onClick={onRotate}>轮换 Token</Button>}
      {worker.state === 'DISABLED' && <Button danger icon={<DeleteOutlined />} onClick={onDecommission}>退役 Worker</Button>}
    </Flex>
    {connectionPanel}
  </Space>;

  return <Space orientation="vertical" size={16} className={styles.drawerContent}>
    <Tabs items={[
      { key: 'overview', label: '详情', children: overview },
      { key: 'tasks', label: `任务 ${assignments.length ? `(${assignments.length})` : ''}`, children: taskPanel },
      { key: 'images', label: `镜像 (${schedulableImages.length})`, children: imagePanel },
      { key: 'connection', label: '配置', children: configPanel },
    ]} />
  </Space>;
}

function CapacityEditor({ worker, loading, onSave }: { worker: WorkforceWorker; loading: boolean; onSave: (maximum: number) => Promise<unknown> }) {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(worker.maxActiveAttempts);
  const reportedMaximum = worker.reportedMaxActiveAttempts || 64;
  useEffect(() => {
    if (!editing) setValue(worker.maxActiveAttempts);
  }, [editing, worker.maxActiveAttempts]);
  const cancel = () => { setValue(worker.maxActiveAttempts); setEditing(false); };
  return <div className={styles.capacityEditor}>
    <Flex justify="space-between" align="center" gap={12} wrap>
      <Flex align="center" gap={10}><Text type="secondary">最大并发数</Text><Text strong>{worker.maxActiveAttempts}</Text><Text type="secondary">Worker 上限 {reportedMaximum}</Text>{!editing && <Button type="text" size="small" icon={<EditOutlined />} onClick={() => setEditing(true)}>编辑</Button>}</Flex>
      {editing && <Form layout="inline" onFinish={() => onSave(value).then(() => setEditing(false)).catch(() => undefined)}>
        <Form.Item label="最大并发" className={styles.capacityField}><InputNumber min={1} max={reportedMaximum} value={value} onChange={(next) => setValue(next ?? 1)} /></Form.Item>
        <Form.Item><Space><Button onClick={cancel}>取消</Button><Button type="primary" htmlType="submit" loading={loading}>保存</Button></Space></Form.Item>
      </Form>}
    </Flex>
  </div>;
}

function ImageAdmissionStatus({ image }: { image: WorkforceWorkerImage }) {
  if (image.admissionStatus === 'ADMITTED') return <Tag color="green">已准入</Tag>;
  const reasonCode = imageAdmissionReasonCode(image);
  if (reasonCode === 'NOT_ALLOWED_BY_WORKER_POLICY') return <Tag>未加入允许列表</Tag>;
  if (reasonCode === 'IMAGE_ADMISSION_REJECTED') return <Tag color="red">准入失败</Tag>;
  return <Tag color="red">不可调度</Tag>;
}

function imageAdmissionReason(image: WorkforceWorkerImage) {
  if (image.admissionStatus === 'ADMITTED') return '—';
  const reasonCode = imageAdmissionReasonCode(image);
  if (reasonCode === 'NOT_ALLOWED_BY_WORKER_POLICY') return '未加入当前 Worker 允许列表';
  if (reasonCode === 'IMAGE_ADMISSION_REJECTED') return '镜像未通过运行环境校验';
  return '镜像未通过准入检查';
}

function imageAdmissionReasonCode(image: WorkforceWorkerImage) {
  return typeof image.admissionEvidence?.reasonCode === 'string' ? image.admissionEvidence.reasonCode : '';
}

function MetricBlock({ label, value, available = value !== undefined, detail }: { label: string; value?: number; available?: boolean; detail: string }) {
  const safe = Math.max(0, Math.min(100, value ?? 0));
  return (
    <div className={styles.metricCard}>
      <Statistic title={label} value={available ? Math.round(safe) : '—'} suffix={available ? '%' : undefined} />
      <Progress percent={Math.round(safe)} showInfo={false} strokeColor={safe >= 90 ? '#ff4d4f' : safe >= 75 ? '#faad14' : '#1677ff'} />
      <Text type="secondary" className={styles.metricDetail}>{available ? detail : '暂未采集'}</Text>
    </div>
  );
}

function TechnicalValue({ value, copyable = false }: { value?: string; copyable?: boolean }) {
  const text = value?.trim() || '—';
  return <Text copyable={copyable && text !== '—' ? { text } : false} className={`${styles.mono} ${styles.technicalValue}`}>{text}</Text>;
}

function formatBytes(value?: number) {
  if (value === undefined) return '—';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let size = value; let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
  return `${size.toFixed(index > 1 ? 1 : 0)} ${units[index]}`;
}

function formatUsage(used?: number, total?: number) {
  if (used === undefined || total === undefined) return '—';
  return `${formatBytes(used)} / ${formatBytes(total)}`;
}

function formatLoad(value?: number) {
  return value === undefined ? '—' : value.toFixed(2);
}

function displayBuildRevision(value?: string) {
  const revision = value?.trim();
  return revision && revision !== 'unknown' ? revision : '开发构建（未注入）';
}

function fleetSummary(workers: WorkforceWorker[]) {
  const online = workers.filter((worker) => worker.state === 'ONLINE').length;
  const running = workers.reduce((sum, worker) => sum + worker.runningAttempts, 0);
  const usedSlots = workers.reduce((sum, worker) => sum + worker.reservedAttempts + worker.startingAttempts + worker.runningAttempts, 0);
  const available = workers.reduce((sum, worker) => sum + worker.availableAttempts, 0);
  const totalSlots = usedSlots + available;
  return {
    online,
    running,
    usedSlots,
    available,
    totalSlots,
    load: totalSlots > 0 ? Math.round((usedSlots / totalSlots) * 100) : 0,
  };
}

function relativeTime(value?: string) {
  if (!value) return '暂无心跳';
  const time = new Date(value).getTime();
  if (!Number.isFinite(time)) return '时间未知';
  const seconds = Math.max(0, Math.round((Date.now() - time) / 1000));
  if (seconds < 10) return '刚刚';
  if (seconds < 60) return `${seconds} 秒前`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes} 分钟前`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} 小时前`;
  return `${Math.floor(hours / 24)} 天前`;
}

function workerCapacityFeedback(error: unknown) {
  const source = error as { reasonCode?: string; details?: { reasonCode?: string }; payload?: { data?: { reasonCode?: string } } };
  const reason = source?.reasonCode ?? source?.details?.reasonCode ?? source?.payload?.data?.reasonCode;
  if (reason === 'WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT') return '最大并发数不能超过 Worker 上报的能力';
  if (reason === 'WORKER_STATE_CONFLICT') return 'Worker 状态已变化，请刷新后重试';
  if (reason === 'WORKER_NOT_FOUND') return 'Worker 不存在或已退役';
  return '最大并发数更新失败，请刷新后重试';
}

function runtimeStateLabel(value: string) {
  const labels: Record<string, string> = {
    AUTHORIZED: '等待创建容器', CONTAINER_CREATED: '容器已创建', SUPERVISOR_READY: 'Supervisor 已就绪',
    STARTING: '正在启动', RUNNING: '运行中', RECOVERING: '等待接管', REATTACHING: '正在接管',
    RECONCILIATION_REQUIRED: '需要核对',
  };
  return labels[value] ?? value;
}
