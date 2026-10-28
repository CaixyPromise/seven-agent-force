import {
  ArrowLeftOutlined,
  BellOutlined,
  CheckCircleFilled,
  ClockCircleOutlined,
  CloudDownloadOutlined,
  CodeOutlined,
  FileTextOutlined,
  ReloadOutlined,
  RedoOutlined,
  SendOutlined,
  SafetyCertificateOutlined,
  CloudUploadOutlined,
  BranchesOutlined,
  FolderOpenOutlined,
  EditOutlined,
  MoreOutlined,
  StopOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  App as AntdApp,
  Button,
  Card,
  Checkbox,
  Col,
  Collapse,
  Descriptions,
  Empty,
  Flex,
  Input,
  Form,
  Modal,
  Popconfirm,
  Result,
  Row,
  Select,
  Skeleton,
  Spin,
  Space,
  Switch,
  Steps,
  Progress,
  Table,
  Tabs,
  Tag,
  Timeline,
  Typography,
  type TableColumnsType,
} from 'antd';
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type MutableRefObject } from 'react';
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom';
import {
  cancelWorkforceTask,
  downloadWorkforceArtifact,
  getWorkforceTask,
  updateWorkforceTaskMetadata,
  getWorkforceTaskRecovery,
  listWorkforceArtifacts,
  listWorkforceEvents,
  listTaskModelInvocations,
  retryWorkforceArtifactUpload,
  retryWorkforceFinalization,
  type TaskStatus,
  type WorkforceArtifact,
  type WorkforceArtifactUploadAttempt,
  type WorkforceEvent,
  type WorkforceFinalizationRun,
  type WorkforceModelInvocation,
  type WorkforceRecoveryView,
  type WorkforceTask,
} from '@/api/workforceController';
import CreatorIdentity from '../../components/CreatorIdentity';
import {
  listTaskContinuationCheckpoints,
  listTaskResultEvaluations,
  listTaskResultOutputs,
  type ContinuationCheckpoint,
  type ResultEvaluationCycle,
  type ResultOutput,
} from '@/api/resultProcessorController';
import {
  approveContinuationCheckpoint,
  createNextTaskTurn,
  createResultMaterialization,
  listResultMaterializations,
  listTaskTurns,
  retryResultMaterialization,
  type ResultMaterializationRun,
  type TaskTurn,
} from '@/api/continuationController';
import { cancelDelivery, decideTaskDelivery, listTaskDeliveries, reauthorizeDelivery, reconcileDelivery, reconcileReview, retryDelivery, type DeliveryRequest } from '@/api/deliveryController';
import { getTaskGitPlan, getTaskInput, type GitPlanView, type TaskInputView } from '@/api/workspaceInputController';
import {
  getTaskNotificationReadiness,
  getTaskNotificationSubscription,
  listTaskNotificationRecords,
  updateTaskNotificationSubscription,
  type TaskNotificationChannel,
  type TaskNotificationEvent,
  type TaskNotificationRecord,
} from '@/api/taskNotificationController';
import { buildTaskDeepLink, parseTaskDetailLocation, type TaskDetailFocus } from '../taskDeepLink';
import '../workforce.css';
import IndependentReviewWorkspace from '../../review/IndependentReviewWorkspace';
import CompletionCheckPanel from '../CompletionCheckPanel';
import TaskConversation from '../TaskConversation';
import NetworkGrantsPanel from '../NetworkGrantsPanel';
import ConversationBusinessCards from '../ConversationBusinessCards';
import TaskStatusRail from '../TaskStatusRail';
import TaskWorkflowMilestones from '../TaskWorkflowMilestones';
import { presentTaskEvents } from '../taskEventPresentation';
import { savedInteractionProgress } from '@/api/taskStatusContract';
import { getProject } from '@/api/projectController';
import { getTaskCompletionCheck } from '@/api/completionCheckController';
import { getIndependentReview } from '@/api/independentReviewController';
import { artifactQueryKey, completionRefreshInterval, deliveryRefreshInterval, notificationRefreshInterval, reviewQueryKey, reviewRefreshInterval, taskRefreshInterval } from '../taskQueryPolicy';

const { Text, Title } = Typography;

type DetailTab = 'overview' | 'activity';
type WorkspaceTab = 'workflow' | 'results' | 'details';

const statusMeta: Record<TaskStatus, { label: string; color: string; terminal?: boolean }> = {
  CREATED: { label: '准备开始', color: 'default' },
  QUEUED: { label: '排队中', color: 'gold' },
  PREPARING: { label: '准备工作区', color: 'blue' },
  RUNNING: { label: '执行中', color: 'blue' },
  INTERACTION_SUSPENDED: { label: '等待你回答', color: 'gold' },
  VALIDATING: { label: '验证中', color: 'cyan' },
  PACKAGING: { label: '打包中', color: 'geekblue' },
  UPLOADING: { label: '上传中', color: 'purple' },
  RETRY_WAIT: { label: '等待重新执行', color: 'gold' },
  REVIEWING: { label: '处理中', color: 'cyan' },
  MANUAL_REVIEW: { label: '需要人工处理', color: 'orange', terminal: true },
  SUCCEEDED: { label: '成功', color: 'green', terminal: true },
  CANCELLING: { label: '取消中', color: 'orange' },
  CANCELLED: { label: '已取消', color: 'default', terminal: true },
  TIMED_OUT: { label: '已超时', color: 'red', terminal: true },
  FAILED: { label: '失败', color: 'red', terminal: true },
};

const stages = [
  { label: '准备工作区', statuses: ['PREPARING'] as TaskStatus[] },
  { label: 'Agent 执行', statuses: ['RUNNING'] as TaskStatus[] },
  { label: '结果验收', statuses: ['VALIDATING', 'REVIEWING'] as TaskStatus[] },
  { label: '打包制品', statuses: ['PACKAGING', 'UPLOADING'] as TaskStatus[] },
  { label: '完成', statuses: ['SUCCEEDED'] as TaskStatus[] },
];

export default function WorkforceTaskDetailPage() {
  const { message } = AntdApp.useApp();
  const { taskId = '' } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const taskDetailLocation = useMemo(() => parseTaskDetailLocation(searchParams), [searchParams]);
  const workspaceTab = parseWorkspaceTab(searchParams, taskDetailLocation);
  const detailTab = parseDetailTab(searchParams.get('detailTab') ?? (searchParams.get('tab') === 'activity' ? 'activity' : null));
  const resultFocus = parseResultFocus(searchParams.get('result'));
  const decodedTaskId = decodeURIComponent(taskId);
  const [continueOpen, setContinueOpen] = useState(false);
  const [approvalOpen, setApprovalOpen] = useState(false);
  const [engineeringOpen, setEngineeringOpen] = useState(false);
  const detailsRef = useRef<HTMLDivElement>(null);
  const taskMenuRef = useRef<HTMLDetailsElement>(null);
  const selectWorkspace = (value: WorkspaceTab, focus?: TaskDetailFocus) => {
    const params = new URLSearchParams(searchParams);
    params.set('tab', value);
    params.delete('detailTab');
    if (value !== 'results') params.delete('result');
    if (focus) params.set('focus', focus);
    else params.delete('focus');
    setSearchParams(params, { replace: true });
  };
  const openResults = (focus: 'checks' | 'review' | 'result' = 'checks') => {
    const params = new URLSearchParams(searchParams);
    params.set('tab', 'results');
    params.set('result', focus);
    params.delete('detailTab');
    params.delete('focus');
    setSearchParams(params, { replace: true });
  };
  const openDetails = (focus?: TaskDetailFocus) => {
    selectWorkspace('details', focus);
    requestAnimationFrame(() => detailsRef.current?.scrollIntoView({ block: 'start' }));
  };
	const [metadataOpen, setMetadataOpen] = useState(false);
  const [continueForm] = Form.useForm<{ instruction: string }>();
  const [approvalForm] = Form.useForm<{ reason: string }>();
	const [metadataForm] = Form.useForm<{ name: string; tags: string[] }>();

  const taskQuery = useQuery({
    queryKey: ['workforce', 'task', decodedTaskId],
    queryFn: () => getWorkforceTask(decodedTaskId),
    enabled: Boolean(decodedTaskId),
    refetchInterval: (state) => taskRefreshInterval(state.state.data?.status),
  });
	const workflowChecksQuery = useQuery({
		queryKey: ['workforce', 'completion-check', decodedTaskId],
		queryFn: () => getTaskCompletionCheck(decodedTaskId),
		enabled: Boolean(decodedTaskId && workspaceTab === 'workflow'),
		refetchInterval: (state) => completionRefreshInterval(taskQuery.data?.status, state.state.data?.currentResult),
	});
	const workflowReviewQuery = useQuery({
		queryKey: reviewQueryKey(decodedTaskId, taskQuery.data?.status),
		queryFn: () => getIndependentReview(decodedTaskId),
		enabled: Boolean(decodedTaskId && workspaceTab === 'workflow'),
		refetchInterval: (state) => reviewRefreshInterval(taskQuery.data?.status, state.state.data?.status, state.state.data?.required),
	});
	const metadataMutation = useMutation({
		mutationFn: (value: { name: string; tags: string[] }) => {
			if (!taskQuery.data?.metadataVersion) throw new Error('任务信息尚未加载完成');
			return updateWorkforceTaskMetadata(decodedTaskId, { ...value, expectedMetadataVersion: taskQuery.data.metadataVersion });
		},
		onSuccess: async () => { message.success('任务信息已更新'); setMetadataOpen(false); await Promise.all([taskQuery.refetch(), queryClient.invalidateQueries({ queryKey:['workforce','tasks'] })]); },
		onError: () => message.error('任务信息更新失败，请刷新后重试'),
	});
  const eventsQuery = useQuery({
    queryKey: ['workforce', 'events', decodedTaskId],
    queryFn: () => listWorkforceEvents(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'details'),
    staleTime: 30_000,
  });
  const artifactsQuery = useQuery({
    queryKey: artifactQueryKey(decodedTaskId, taskQuery.data?.status),
    queryFn: () => listWorkforceArtifacts(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'results'),
    refetchInterval: () => taskRefreshInterval(taskQuery.data?.status),
  });
  const recoveryQuery = useQuery({
    queryKey: ['workforce', 'recovery', decodedTaskId],
    queryFn: () => getWorkforceTaskRecovery(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'details'),
    staleTime: 30_000,
  });
  const providerInvocationsQuery = useQuery({
    queryKey: ['workforce', 'task-model-invocations', decodedTaskId],
    queryFn: () => listTaskModelInvocations(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'details' && detailTab === 'overview' && engineeringOpen && taskQuery.data?.execution?.resolved?.modelCredentialId),
    staleTime: 30_000,
  });
  const evaluationsQuery = useQuery({
    queryKey: ['workforce', 'task-result-evaluations', decodedTaskId],
    queryFn: () => listTaskResultEvaluations(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'details' && taskQuery.data?.resultProcessorReleaseId),
    staleTime: 30_000,
  });
  const checkpointsQuery = useQuery({
    queryKey: ['workforce', 'task-continuation-checkpoints', decodedTaskId],
    queryFn: () => listTaskContinuationCheckpoints(decodedTaskId),
    enabled: Boolean(decodedTaskId && taskQuery.data?.resultProcessorReleaseId),
    refetchInterval: () => taskRefreshInterval(taskQuery.data?.status),
  });
  const turnsQuery = useQuery({
    queryKey: ['workforce', 'task-turns', decodedTaskId],
    queryFn: () => listTaskTurns(decodedTaskId),
    enabled: Boolean(decodedTaskId && taskQuery.data?.resultProcessorReleaseId),
    refetchInterval: () => taskRefreshInterval(taskQuery.data?.status),
  });
  const outputsQuery = useQuery({
    queryKey: ['workforce', 'task-result-outputs', decodedTaskId],
    queryFn: () => listTaskResultOutputs(decodedTaskId),
    enabled: Boolean(decodedTaskId && workspaceTab === 'details' && taskQuery.data?.resultProcessorReleaseId),
    staleTime: 30_000,
  });
	const materializationsQuery = useQuery({
		queryKey: ['workforce', 'task-result-materializations', decodedTaskId],
		queryFn: () => listResultMaterializations(decodedTaskId),
		enabled: Boolean(decodedTaskId && workspaceTab === 'details' && taskQuery.data?.resultProcessorReleaseId),
		staleTime: 30_000,
	});
  const deliveriesQuery = useQuery({
    queryKey: ['workforce', 'task-deliveries', decodedTaskId],
    queryFn: () => listTaskDeliveries(decodedTaskId),
    enabled: Boolean(decodedTaskId && (workspaceTab === 'workflow' || workspaceTab === 'details')),
    refetchInterval: (state) => deliveryRefreshInterval(workspaceTab, taskQuery.data?.status, state.state.data),
  });
  const taskInputQuery = useQuery({
    queryKey: ['workforce', 'task-input', decodedTaskId],
    queryFn: () => getTaskInput(decodedTaskId),
    enabled: Boolean(decodedTaskId),
    staleTime: Infinity,
  });
  const gitPlanQuery = useQuery({
    queryKey: ['workforce', 'task-git-plan', decodedTaskId],
    queryFn: () => getTaskGitPlan(decodedTaskId),
    enabled: Boolean(decodedTaskId),
    staleTime: Infinity,
  });
  const projectQuery = useQuery({
    queryKey: ['workforce', 'project', taskQuery.data?.projectId],
    queryFn: () => getProject(taskQuery.data!.projectId),
    enabled: Boolean(taskQuery.data?.projectId),
    staleTime: 30_000,
  });
	const refreshDeliveryFromConversation = useCallback(() => {
		void queryClient.invalidateQueries({ queryKey: ['workforce', 'task-deliveries', decodedTaskId] });
		void queryClient.invalidateQueries({ queryKey: ['workforce', 'task', decodedTaskId] });
	}, [decodedTaskId, queryClient]);
  const cancelMutation = useMutation({
    mutationFn: cancelWorkforceTask,
    onSuccess: async () => {
      message.success('取消请求已提交');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['workforce', 'task', decodedTaskId] }),
        queryClient.invalidateQueries({ queryKey: ['workforce', 'tasks'] }),
      ]);
    },
		onError: () => message.error('取消任务失败，请刷新后重试'),
  });
  const retryFinalizationMutation = useMutation({
    mutationFn: (id: string) => retryWorkforceFinalization(id),
    onSuccess: async () => {
      message.success('已重新排入收尾队列，不会重新运行 Agent');
      await Promise.all([recoveryQuery.refetch(), taskQuery.refetch()]);
    },
		onError: () => message.error('收尾重试失败，请稍后重试'),
  });
  const retryUploadMutation = useMutation({
    mutationFn: (id: string) => retryWorkforceArtifactUpload(id),
    onSuccess: async () => {
      message.success('已只重试失败的制品上传');
      await Promise.all([recoveryQuery.refetch(), taskQuery.refetch()]);
    },
		onError: () => message.error('制品上传重试失败，请稍后重试'),
  });
  const currentTurn = turnsQuery.data?.at(-1);
  const currentCheckpoint = [...(checkpointsQuery.data ?? [])].reverse().find((item) => item.taskTurnId === currentTurn?.id && item.status === 'AVAILABLE');
  const independentReviewOwnsClosure = Boolean(taskQuery.data?.independentReview?.required && ['REVIEWING', 'MANUAL_REVIEW'].includes(taskQuery.data.status));
  const canContinueCurrentTurn = Boolean(taskQuery.data?.status !== 'INTERACTION_SUSPENDED' && currentTurn && currentCheckpoint && !independentReviewOwnsClosure && ['SUCCEEDED', 'REVIEWING', 'MANUAL_REVIEW'].includes(currentTurn.status));
  const openContinuationDialog = () => {
    if (currentTurn?.status === 'MANUAL_REVIEW') setApprovalOpen(true);
    else setContinueOpen(true);
  };
  const createTurnMutation = useMutation({
    mutationFn: (instruction: string) => {
      if (!canContinueCurrentTurn || !currentTurn || !currentCheckpoint) throw new Error('当前结果还没有准备好，暂时不能继续');
      return createNextTaskTurn(decodedTaskId, {
        sourceCheckpointId: currentCheckpoint.id,
        expectedCurrentTurnId: currentTurn.id,
        instruction,
        idempotencyKey: `continue:${currentTurn.id}:${crypto.randomUUID()}`,
      });
    },
    onSuccess: async () => {
      message.success('新一轮要求已提交，将从上次结果继续');
      setContinueOpen(false);
      continueForm.resetFields();
      await Promise.all([turnsQuery.refetch(), taskQuery.refetch(), checkpointsQuery.refetch()]);
    },
		onError: () => message.error('继续任务失败，请刷新后重试'),
  });
  const approveMutation = useMutation({
    mutationFn: (reason: string) => {
      if (!currentCheckpoint) throw new Error('没有可审批的结果');
      return approveContinuationCheckpoint(currentCheckpoint.id, reason);
    },
    onSuccess: async () => {
      message.success('已批准继续处理');
      setApprovalOpen(false);
      approvalForm.resetFields();
      setContinueOpen(true);
      await checkpointsQuery.refetch();
    },
		onError: () => message.error('审批失败，请刷新后重试'),
  });
  const materializeMutation = useMutation({
    mutationFn: (input: { kind: ResultMaterializationRun['kind']; outputId?: string }) => {
      if (!currentTurn) throw new Error('当前轮次不存在');
      return createResultMaterialization(decodedTaskId, { taskTurnId: currentTurn.id, ...input, idempotencyKey: `output:${currentTurn.id}:${input.kind}:${input.outputId ?? 'builtin'}:${crypto.randomUUID()}` });
    },
    onSuccess: async () => {
      message.success('正在后台生成，完成后会出现在验收产物中');
		await Promise.all([outputsQuery.refetch(), materializationsQuery.refetch(), artifactsQuery.refetch()]);
    },
		onError: () => message.error('生成输出失败，请稍后重试'),
  });
	const retryMaterializationMutation = useMutation({
		mutationFn: (runId: string) => retryResultMaterialization(runId, '用户重新生成输出'),
		onSuccess: async () => {
			message.success('已重新开始生成，不会再次运行 Agent');
			await materializationsQuery.refetch();
		},
		onError: () => message.error('重新生成失败，请稍后重试'),
	});
	const approveDeliveryMutation = useMutation({ mutationFn: (item: DeliveryRequest) => decideTaskDelivery(decodedTaskId, item.id, { expectedVersion: item.version, action: 'DELIVER', reason: '用户确认交付冻结结果', idempotencyKey: `delivery:${item.id}:deliver:${crypto.randomUUID()}` }), onSuccess: async () => { message.success('已批准，平台将安全推送并回读确认'); await deliveriesQuery.refetch(); }, onError: () => message.error('批准失败，请刷新后重试') });
	const cancelDeliveryMutation = useMutation({ mutationFn: cancelDelivery, onSuccess: async () => { message.success('取消请求已提交'); await deliveriesQuery.refetch(); }, onError: () => message.error('取消失败，请刷新后重试') });
	const retryDeliveryMutation = useMutation({ mutationFn: retryDelivery, onSuccess: async () => { message.success('已重新确认交付，不会重新运行 Agent'); await deliveriesQuery.refetch(); }, onError: () => message.error('重试失败，请刷新后重试') });
	const reconcileDeliveryMutation = useMutation({ mutationFn: reconcileDelivery, onSuccess: async () => { message.success('正在只读核对远端状态，不会重复推送'); await deliveriesQuery.refetch(); }, onError: () => message.error('远端状态核对失败，请稍后重试') });
	const reconcileReviewMutation = useMutation({ mutationFn: reconcileReview, onSuccess: async () => { message.success('已只读核对远端评审状态'); await deliveriesQuery.refetch(); }, onError: () => message.error('远端评审状态核对失败，请稍后重试') });
	const reauthorizeDeliveryMutation = useMutation({ mutationFn: ({ id, projectVersionId }: { id: API.Int64; projectVersionId: API.Int64 }) => reauthorizeDelivery(id, '使用已更新的项目仓库凭据，重新确认原结果的交付', projectVersionId), onSuccess: async () => { message.success('请再次批准；平台会先核对远端，再决定是否推送'); await deliveriesQuery.refetch(); }, onError: () => message.error('重新授权失败，请刷新后重试') });

  const backTarget = useMemo(() => {
    const from = (location.state as { from?: unknown } | null)?.from;
    return typeof from === 'string' && from.startsWith('/workforce/tasks') && !from.startsWith('//')
      ? from
      : '/workforce/tasks';
  }, [location.state]);
  const eventWorkerId = useMemo(() => {
    const event = (eventsQuery.data ?? []).find((item) => typeof item.payload.workerId === 'string');
    return typeof event?.payload.workerId === 'string' ? event.payload.workerId : '等待分配';
  }, [eventsQuery.data]);

  if (taskQuery.isLoading) {
    return <div className="workforce-page"><Card><Skeleton active paragraph={{ rows: 12 }} /></Card></div>;
  }
  if (taskQuery.isError || !taskQuery.data) {
    return (
      <div className="workforce-page">
        <Card>
          <Result
            status="404"
            title="任务不存在或不可访问"
            subTitle="请确认任务 ID，或返回任务列表重新选择。"
            extra={<Button type="primary" onClick={() => navigate('/workforce/tasks')}>返回任务列表</Button>}
          />
        </Card>
      </div>
    );
  }

  const task = taskQuery.data;
  const terminal = Boolean(statusMeta[task.status].terminal);
  const taskWaiting = task.status === 'INTERACTION_SUSPENDED' || Boolean(task.execution?.queueWaitReason);
  const progressAt = task.lastProgressAt || task.startedAt;
  const staleProgress = !taskWaiting && ['RUNNING', 'VALIDATING'].includes(task.status) && Boolean(progressAt) && Date.now() - new Date(progressAt!).getTime() > 30 * 60_000;
  const activeMinutes = task.executionActiveMilliseconds === undefined ? null : Number(task.executionActiveMilliseconds) / 60_000;

  return (
    <div className="workforce-page workforce-detail-page">
      <header className="workforce-task-header">
        <Button className="workforce-task-back" type="text" shape="circle" icon={<ArrowLeftOutlined />} aria-label="返回任务列表" onClick={() => navigate(backTarget)} />
        <div className="workforce-task-heading">
          <Space wrap size={8} className="workforce-task-heading-status">
            <StatusTag status={task.status} verbose />
            {currentTurn && <Tag>第 {currentTurn.turnNumber} 轮</Tag>}
            {task.tags?.slice(0, 3).map((tag) => <Tag key={tag}>{tag}</Tag>)}
          </Space>
          <Title level={2}>{task.name}</Title>
          <Space wrap separator={<span className="workforce-meta-dot">·</span>} className="workforce-task-meta">
            <Link className="workforce-task-project-link" to={`/workforce/projects/${encodeURIComponent(task.projectId)}`}><FolderOpenOutlined /> {projectQuery.data?.name ?? '所属项目'}</Link>
            <Text><CodeOutlined /> {sourceLabel(taskInputQuery.data)}</Text>
            {taskInputQuery.data?.workspaceSource.refName && <Text>{taskInputQuery.data.workspaceSource.refName}</Text>}
            <CreatorIdentity userId={task.createdBy} />
            <Text type="secondary">{formatTime(task.createdAt)}</Text>
          </Space>
        </div>
        <details ref={taskMenuRef} className="workforce-task-menu" onKeyDown={(event) => {
          if (event.key !== 'Escape' || !taskMenuRef.current?.open) return;
          event.preventDefault();
          taskMenuRef.current.open = false;
          taskMenuRef.current.querySelector<HTMLElement>('summary')?.focus();
        }}>
          <summary aria-label="任务操作"><MoreOutlined /></summary>
          <div className="workforce-task-menu-popover">
            <Button type="text" block icon={<EditOutlined />} onClick={() => { metadataForm.setFieldsValue({ name:task.name, tags:task.tags ?? [] }); setMetadataOpen(true); }}>编辑信息</Button>
            {task.resultProcessorReleaseId && task.status !== 'INTERACTION_SUSPENDED' && !independentReviewOwnsClosure && <Button type="text" block icon={<SendOutlined />} disabled={!canContinueCurrentTurn} loading={createTurnMutation.isPending} onClick={openContinuationDialog}>继续完善</Button>}
            {!terminal && task.status !== 'CANCELLING' && <Popconfirm title="取消这项工作？" description="当前执行会停止，已保存记录会保留。" onConfirm={() => cancelMutation.mutate(task.id)}><Button type="text" danger block icon={<StopOutlined />} loading={cancelMutation.isPending}>取消任务</Button></Popconfirm>}
          </div>
        </details>
      </header>

      {!terminal && <div className="workforce-task-progress-summary" aria-label="任务运行状态">
        <Text>活跃执行：{activeMinutes === null ? '暂无记录' : `${activeMinutes < 1 ? '<1' : Math.floor(activeMinutes)} 分钟`}</Text>
        <Text>最近进展：{task.lastProgressAt ? formatTime(task.lastProgressAt) : '暂无可确认的执行进展'}</Text>
        <Text>当前等待：{task.execution?.queueWaitReason ? waitReasonLabel(task.execution.queueWaitReason) : task.status === 'INTERACTION_SUSPENDED' ? '用户回答' : '无'}</Text>
      </div>}
      {staleProgress && <Alert className="workforce-task-stale-progress" type="warning" showIcon title="超过 30 分钟没有新的执行进展，请检查运行记录。" action={<Popconfirm title="取消这项工作？" description="当前执行会停止，已保存记录会保留。" onConfirm={() => cancelMutation.mutate(task.id)}><Button danger size="small" loading={cancelMutation.isPending}>取消任务</Button></Popconfirm>} />}

      <Tabs
        className="workforce-task-primary-tabs"
        activeKey={workspaceTab}
        onChange={(key) => selectWorkspace(key as WorkspaceTab)}
        items={[
          { key: 'workflow', label: '工作流' },
          { key: 'results', label: '结果' },
          { key: 'details', label: '详情' },
        ]}
      />

      <div className="workforce-workspace-grid" hidden={!(workspaceTab === 'workflow')}>
        <main className="workforce-workflow-column">
          <TaskConversation
            visible={workspaceTab === 'workflow'}
            reviewPending={task.status === 'MANUAL_REVIEW' && !!workflowReviewQuery.data?.availableActions.length}
            taskId={task.id}
            goal={task.instruction}
            onDeliveryChange={refreshDeliveryFromConversation}
            onResults={() => openResults('result')}
            onReview={() => openResults('review')}
            onDetails={() => openDetails()}
            milestones={<TaskWorkflowMilestones taskStatus={task.status} checks={workflowChecksQuery.data} review={workflowReviewQuery.data} loading={workflowChecksQuery.isLoading && workflowReviewQuery.isLoading} onOpen={openResults} />}
          />
        </main>
        <TaskStatusRail
          onOpenResults={() => openResults('result')}
          task={task}
          deliveries={deliveriesQuery.data ?? []}
          checks={workflowChecksQuery.data}
          review={workflowReviewQuery.data}
          latestStep={savedInteractionProgress(task.status) || currentStageHint(task.status)}
          onOpenPending={() => {
            const holder = document.querySelector<HTMLElement>('.workforce-chat-conversation .rc-virtual-list-holder');
            holder?.scrollTo({ top: holder.scrollHeight, behavior: 'smooth' });
          }}
          onOpenDetails={() => openDetails('delivery')}
        />
      </div>

      {workspaceTab === 'results' && <div className="workforce-results-workspace">
        <ConversationBusinessCards
          focus={resultFocus}
          checks={<CompletionCheckPanel key={task.id} taskId={task.id} taskStatus={task.status} />}
          review={<IndependentReviewWorkspace taskId={task.id} taskStatus={task.status} compact />}
          result={<Card title="任务结果" extra={<StatusTag status={task.status} />} data-visual-surface="task-result-card"><ArtifactsTab key={task.id} taskId={task.id} artifacts={artifactsQuery.data ?? []} loading={artifactsQuery.isLoading} error={artifactsQuery.isError} onRetry={() => void artifactsQuery.refetch()} /></Card>}
        />
      </div>}

      {workspaceTab === 'details' && <div ref={detailsRef} className="workforce-task-technical">

      <TaskInputSummary input={taskInputQuery.data} gitPlan={gitPlanQuery.data} loading={taskInputQuery.isLoading || gitPlanQuery.isLoading} />

      <NetworkGrantsPanel taskId={task.id} />

      {task.resultProcessorReleaseId ? (
        <ContinuationWorkspace
          task={task}
          turns={turnsQuery.data ?? []}
          turnsLoading={turnsQuery.isLoading}
          evaluations={evaluationsQuery.data ?? []}
          checkpoints={checkpointsQuery.data ?? []}
          outputs={outputsQuery.data ?? []}
          outputsLoading={outputsQuery.isLoading}
          materializations={materializationsQuery.data ?? []}
          materializationsLoading={materializationsQuery.isLoading}
          continuing={createTurnMutation.isPending}
          materializing={materializeMutation.isPending || retryMaterializationMutation.isPending}
          onContinue={openContinuationDialog}
          onMaterialize={(kind, outputId) => materializeMutation.mutate({ kind, outputId })}
          onRetryMaterialization={(runId) => retryMaterializationMutation.mutate(runId)}
          deepLinkTurnId={taskDetailLocation.turnId}
          deepLinkFocus={taskDetailLocation.focus}
        />
      ) : <ExecutionProgress task={task} />}

      <TaskNotificationCard task={task} turns={turnsQuery.data ?? []} />

      <DeliveryWorkspace
        deliveries={deliveriesQuery.data ?? []}
        loading={deliveriesQuery.isLoading}
        mutating={approveDeliveryMutation.isPending || cancelDeliveryMutation.isPending || retryDeliveryMutation.isPending || reconcileDeliveryMutation.isPending || reconcileReviewMutation.isPending || reauthorizeDeliveryMutation.isPending}
        onApprove={(item) => approveDeliveryMutation.mutate(item)}
        onCancel={(id) => cancelDeliveryMutation.mutate(id)}
        onRetry={(id) => retryDeliveryMutation.mutate(id)}
        onReconcile={(id) => reconcileDeliveryMutation.mutate(id)}
        onReconcileReview={(id) => reconcileReviewMutation.mutate(id)}
        onReauthorize={(id, projectVersionId) => { if (projectVersionId) reauthorizeDeliveryMutation.mutate({ id, projectVersionId }); }}
        replacementVersionId={projectQuery.data?.versionId}
        focusedDeliveryId={taskDetailLocation.deliveryId}
        focusedReviewId={taskDetailLocation.reviewId}
      />

	  <Modal title="编辑任务信息" open={metadataOpen} onCancel={() => setMetadataOpen(false)} onOk={() => metadataForm.submit()} confirmLoading={metadataMutation.isPending} okText="保存" cancelText="取消" destroyOnHidden>
		<Form form={metadataForm} layout="vertical" onFinish={(value) => metadataMutation.mutate(value)}>
		  <Form.Item name="name" label="任务名称" rules={[{ required:true, message:'请输入任务名称' }, { max:120, message:'名称最多 120 个字符' }]}><Input maxLength={120} /></Form.Item>
		  <Form.Item name="tags" label="标签" rules={[{ validator: async (_, value?: string[]) => { if ((value?.length ?? 0) > 10) throw new Error('最多添加 10 个标签'); } }]}><Select mode="tags" maxCount={10} tokenSeparators={[',']} placeholder="输入后回车" /></Form.Item>
		</Form>
	  </Modal>

      <Modal className="workforce-continue-modal" width={760} title="回复并继续" open={continueOpen} onCancel={() => setContinueOpen(false)} onOk={() => continueForm.submit()} confirmLoading={createTurnMutation.isPending} okText="发送并继续" cancelText="取消" destroyOnHidden>
        <Row gutter={[20, 20]}>
          <Col xs={24} md={16}>
            <Alert type="info" showIcon title="从当前结果接着做" description="平台会恢复代码和工作会话，上一轮的结果会完整保留。" style={{ marginBottom: 16 }} />
            <Form form={continueForm} layout="vertical" onFinish={(value) => createTurnMutation.mutate(value.instruction)}>
              <Form.Item name="instruction" label="你的回复与要求" rules={[{ required: true, message: '请描述需要继续完成的内容' }, { max: 16384, message: '内容过长' }]}>
                <Input.TextArea autoFocus rows={7} placeholder="请描述你的反馈，或者下一步希望补充的内容……" showCount maxLength={16384} />
              </Form.Item>
              <Space wrap size={[8, 8]}>
                <Text type="secondary">快捷填写</Text>
                {['继续优化细节', '补充更多测试', '完善使用说明', '调整实现方案'].map((value) => (
                  <Button key={value} size="small" onClick={() => continueForm.setFieldValue('instruction', value)}>{value}</Button>
                ))}
              </Space>
            </Form>
          </Col>
          <Col xs={24} md={8}>
            <Card size="small" className="workforce-continue-context" title="当前结果概览">
              <Space orientation="vertical" size={12}>
                <div><Text type="secondary">已完成轮次</Text><Title level={4}>{currentTurn?.turnNumber ?? 1}</Title></div>
                <div><Text type="secondary">本轮结果</Text><div><StatusTag status={task.status} /></div></div>
                <div><Text type="secondary">可用输出</Text><div><Text strong>{outputsQuery.data?.filter((item) => item.taskTurnId === currentTurn?.id).length ?? 0} 项</Text></div></div>
              </Space>
            </Card>
          </Col>
        </Row>
      </Modal>
      <Modal title="批准继续处理" open={approvalOpen} onCancel={() => setApprovalOpen(false)} onOk={() => approvalForm.submit()} confirmLoading={approveMutation.isPending} okText="批准" cancelText="取消" destroyOnHidden>
        <Form form={approvalForm} layout="vertical" onFinish={(value) => approveMutation.mutate(value.reason)}>
          <Form.Item name="reason" label="审批说明" rules={[{ required: true, message: '请填写审批说明' }, { max: 2000 }]}><Input.TextArea rows={4} placeholder="说明为什么允许基于这份结果继续。" /></Form.Item>
        </Form>
      </Modal>

      <Card className="workforce-detail-tabs" styles={{ body: { paddingTop: 0 } }}>
        <Tabs
          activeKey={detailTab}
          onChange={(key) => {
            const params = new URLSearchParams(searchParams);
            params.set('tab', 'details');
            if (key === 'overview') params.delete('detailTab');
            else params.set('detailTab', key);
            setSearchParams(params, { replace: true });
          }}
          items={[
            {
              key: 'overview',
              label: '更多详情',
              children: (
                <Collapse
                  className="workforce-engineering-details"
                  onChange={(keys) => setEngineeringOpen(Array.isArray(keys) ? keys.includes('engineering') : keys === 'engineering')}
                  items={[{
                    key: 'engineering',
                    label: '工程与审计信息',
                    children: (
                      <OverviewTab
                        task={task}
                        events={eventsQuery.data ?? []}
                        workerId={task.execution?.workerId || eventWorkerId}
                        recovery={recoveryQuery.data}
                        recoveryLoading={recoveryQuery.isLoading}
                        providerInvocations={providerInvocationsQuery.data ?? []}
                        providerInvocationsLoading={providerInvocationsQuery.isLoading}
                        evaluations={evaluationsQuery.data ?? []}
                        evaluationsLoading={evaluationsQuery.isLoading}
                        checkpoints={checkpointsQuery.data ?? []}
                        checkpointsLoading={checkpointsQuery.isLoading}
                        retryingFinalization={retryFinalizationMutation.isPending}
                        retryingUpload={retryUploadMutation.isPending}
                        onRetryFinalization={(id) => retryFinalizationMutation.mutate(id)}
                        onRetryUpload={(id) => retryUploadMutation.mutate(id)}
                      />
                    ),
                  }]}
                />
              ),
            },
            {
              key: 'activity',
              label: `运行记录 (${eventsQuery.data?.length ?? 0})`,
              children: (
                <ActivityTab
                  events={eventsQuery.data ?? []}
                  loading={eventsQuery.isLoading}
                  onRefresh={() => eventsQuery.refetch()}
                />
              ),
            },
          ]}
        />
      </Card>
      </div>}
    </div>
  );
}

const taskNotificationEventOptions: Array<{ label: string; value: TaskNotificationEvent }> = [
  { label: '需要我处理', value: 'TASK_ACTION_REQUIRED' },
  { label: '执行中断', value: 'TASK_INTERRUPTED' },
  { label: '当前轮完成', value: 'TASK_COMPLETED' },
];

function TaskNotificationCard({ task, turns }: { task: WorkforceTask; turns: TaskTurn[] }) {
  const { message } = AntdApp.useApp();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsForm] = Form.useForm<{ enabled: boolean; events: TaskNotificationEvent[]; channelKeys: string[] }>();
  const settingsEnabled = Form.useWatch('enabled', settingsForm) !== false;
  const subscriptionKey = ['workforce', 'task-notification-subscription', task.id] as const;
  const subscriptionQuery = useQuery({
    queryKey: subscriptionKey,
    queryFn: () => getTaskNotificationSubscription(task.id),
    staleTime: 30_000,
  });
  const readinessQuery = useQuery({
    queryKey: ['workforce', 'task-notification-readiness'],
    queryFn: getTaskNotificationReadiness,
    enabled: settingsOpen,
  });
  const recordsQuery = useQuery({
    queryKey: ['workforce', 'task-notification-records', task.id],
    queryFn: () => listTaskNotificationRecords(task.id),
    refetchInterval: () => notificationRefreshInterval(task.status),
  });
  const updateMutation = useMutation({
    mutationFn: (values: { enabled: boolean; events: TaskNotificationEvent[]; channelKeys: string[] }) => {
      const subscription = subscriptionQuery.data;
      if (!subscription) throw new Error('通知设置尚未加载完成');
      return updateTaskNotificationSubscription(task.id, {
        enabled: values.enabled,
        events: values.events,
        channels: notificationChannelsFromKeys(values.channelKeys),
        expectedVersion: subscription.version,
      });
    },
    onSuccess: async (subscription) => {
      queryClient.setQueryData(subscriptionKey, subscription);
      message.success('任务通知已更新');
      setSettingsOpen(false);
      updateMutation.reset();
    },
  });
  const openSettings = () => {
    const subscription = subscriptionQuery.data;
    if (!subscription) return;
    updateMutation.reset();
    settingsForm.setFieldsValue({ enabled: subscription.enabled, events: subscription.events, channelKeys: notificationChannelKeys(subscription.channels) });
    setSettingsOpen(true);
  };
  const reloadSettings = async () => {
    const result = await subscriptionQuery.refetch();
    if (result.data) settingsForm.setFieldsValue({ enabled: result.data.enabled, events: result.data.events, channelKeys: notificationChannelKeys(result.data.channels) });
    updateMutation.reset();
  };

  if (subscriptionQuery.isLoading) {
    return <Card title={<Space><BellOutlined style={{ color: '#1677ff' }} />任务通知</Space>}><Skeleton active paragraph={{ rows: 3 }} /></Card>;
  }
  if (subscriptionQuery.isError || !subscriptionQuery.data) {
    return (
      <Card title={<Space><BellOutlined style={{ color: '#1677ff' }} />任务通知</Space>}>
        <Alert
          type="warning"
          showIcon
          title="通知设置暂时无法加载"
          action={<Button size="small" onClick={() => void subscriptionQuery.refetch()}>重试</Button>}
        />
      </Card>
    );
  }

  const subscription = subscriptionQuery.data;
  const recentRecords = [...(recordsQuery.data ?? [])]
    .sort((left, right) => Date.parse(right.createdAt) - Date.parse(left.createdAt))
    .slice(0, 3);

  return (
    <Card
      title={<Space><BellOutlined style={{ color: '#1677ff' }} />任务通知</Space>}
      extra={<Button type="link" onClick={openSettings}>管理</Button>}
    >
      <Space orientation="vertical" size={16} style={{ width: '100%' }}>
        <Descriptions
          size="small"
          column={{ xs: 1, sm: 3 }}
          items={[
            { key: 'recipient', label: '接收人', children: <CreatorIdentity userId={subscription.subscriberUserId} /> },
            { key: 'events', label: '通知事件', children: subscription.enabled ? notificationEventSummary(subscription.events) : '已关闭' },
            { key: 'channel', label: '发送到', children: subscription.enabled ? <Space wrap>{subscription.channels.map((channel) => <Tag color="blue" key={`${channel.kind}:${channel.routeId ?? ''}`}>{notificationChannelLabel(channel, readinessQuery.data?.groupRoutes)}</Tag>)}</Space> : <Tag>未发送</Tag> },
          ]}
        />
        <div>
          <Flex align="center" justify="space-between" gap={12} style={{ marginBottom: 8 }}>
            <Text strong>最近通知</Text>
            {recordsQuery.isFetching && <Spin size="small" />}
          </Flex>
          {recordsQuery.isError ? (
            <Alert type="warning" showIcon title="最近通知暂时无法加载" action={<Button size="small" onClick={() => void recordsQuery.refetch()}>重试</Button>} />
          ) : recentRecords.length ? (
            <div role="list" aria-label="最近通知">
              {recentRecords.map((record, index) => (
                <Flex
                  key={`${record.id || record.sourceId}:${record.taskTurnId}:${record.factType}:${index}`}
                  role="listitem"
                  align="center"
                  justify="space-between"
                  gap={16}
                  style={{
                    minHeight: 56,
                    padding: '8px 0',
                    borderBottom: index === recentRecords.length - 1 ? 0 : '1px solid rgba(5, 5, 5, 0.06)',
                  }}
                >
                  <div style={{ minWidth: 0 }}>
                    <Space wrap size={8}>
                      <Text>{notificationRecordTitle(record)}</Text>
                      <TaskNotificationStatusTag status={record.status} />
                    </Space>
                    <div>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        {formatTime(record.publishedAt || record.updatedAt || record.createdAt)} · {notificationTurnLabel(record, turns)}
                      </Text>
                    </div>
                    {(record.deliveries ?? []).length > 0 && <Space wrap size={[4, 4]} style={{ marginTop: 6 }}>
                      {(record.deliveries ?? []).map((delivery, deliveryIndex) => <Tag key={`${delivery.channelKind}:${delivery.displayName}:${deliveryIndex}`} color={delivery.state === 'DELIVERED' ? 'green' : delivery.state === 'QUEUED' ? 'blue' : delivery.state === 'OUTCOME_UNKNOWN' ? 'orange' : 'default'}>
                        {delivery.displayName} · {delivery.state === 'DELIVERED' ? '已送达' : delivery.state === 'QUEUED' ? '投递中' : delivery.state === 'OUTCOME_UNKNOWN' ? '结果待确认' : delivery.state === 'SUPPRESSED' ? '未发送' : '需要处理'}{delivery.mentionState === 'MENTIONED' ? ' · 已提醒处理人' : delivery.mentionState === 'NOT_AVAILABLE' ? ' · 未能提醒个人' : ''}
                      </Tag>)}
                    </Space>}
                  </div>
                  <Button type="link" size="small" onClick={() => navigate(record.deepLink || buildTaskDeepLink(task.id, record.taskTurnId, record.focus))}>
                    {notificationActionLabel(record)}
                  </Button>
                </Flex>
              ))}
            </div>
          ) : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="任务有需要处理或完成时，会在这里留下记录" />}
        </div>
      </Space>

      <Modal
        title="管理任务通知"
        open={settingsOpen}
        onCancel={() => { setSettingsOpen(false); updateMutation.reset(); }}
        onOk={() => settingsForm.submit()}
        confirmLoading={updateMutation.isPending}
        okText="保存"
        cancelText="取消"
        destroyOnHidden
      >
        <Form form={settingsForm} layout="vertical" onFinish={(values) => updateMutation.mutate(values)}>
          {updateMutation.isError && (
            <Alert
              type="warning"
              showIcon
              title="通知设置未保存"
              description="请稍后重试，或重新加载最新设置。"
              action={<Button size="small" onClick={() => void reloadSettings()}>重新加载</Button>}
              style={{ marginBottom: 16 }}
            />
          )}
          <Form.Item name="enabled" label="接收任务通知" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item
            name="events"
            label="通知事件"
            rules={[{
              validator: async (_, value?: TaskNotificationEvent[]) => {
                if (settingsEnabled && !value?.length) throw new Error('请至少选择一个通知事件');
              },
            }]}
          >
            <Checkbox.Group disabled={!settingsEnabled} options={taskNotificationEventOptions} />
          </Form.Item>
          <Form.Item name="channelKeys" label="发送到" rules={[{ required: settingsEnabled, message: '请至少选择一个通知渠道' }]}>
            <Checkbox.Group disabled={!settingsEnabled} style={{ width: '100%' }}>
              <Space orientation="vertical" size={8}>
                <Checkbox value="IN_APP">站内信</Checkbox>
                <Checkbox value="EMAIL_USER" disabled={!readinessQuery.data?.email.available}>邮箱</Checkbox>
                <Checkbox value="FEISHU_DIRECT" disabled={!readinessQuery.data?.feishu.directReady}>个人飞书</Checkbox>
                <Checkbox value="WECOM_DIRECT" disabled={!readinessQuery.data?.wecom.directReady}>个人企业微信</Checkbox>
                {(readinessQuery.data?.groupRoutes ?? []).map((route) => (
                  <Checkbox key={route.id} value={`GROUP_ROUTE:${route.id}`}>{route.name}</Checkbox>
                ))}
              </Space>
            </Checkbox.Group>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}

function ContinuationWorkspace({ task, turns, turnsLoading, evaluations, checkpoints, outputs, outputsLoading, materializations, materializationsLoading, continuing, materializing, onContinue, onMaterialize, onRetryMaterialization, deepLinkTurnId, deepLinkFocus }: {
  task: WorkforceTask;
  turns: TaskTurn[];
  turnsLoading: boolean;
  evaluations: ResultEvaluationCycle[];
  checkpoints: ContinuationCheckpoint[];
  outputs: ResultOutput[];
  outputsLoading: boolean;
	materializations: ResultMaterializationRun[];
	materializationsLoading: boolean;
  continuing: boolean;
  materializing: boolean;
  onContinue: () => void;
  onMaterialize: (kind: ResultMaterializationRun['kind'], outputId?: string) => void;
	onRetryMaterialization: (runId: string) => void;
  deepLinkTurnId?: API.Int64;
  deepLinkFocus?: TaskDetailFocus;
}) {
  const currentTurn = turns.at(-1);
  const currentCheckpoint = [...checkpoints].reverse().find((item) => item.taskTurnId === currentTurn?.id && item.status === 'AVAILABLE');
  const independentReviewOwnsClosure = Boolean(task.independentReview?.required && ['REVIEWING', 'MANUAL_REVIEW'].includes(task.status));
  const canContinue = Boolean(task.status !== 'INTERACTION_SUSPENDED' && currentTurn && currentCheckpoint && !independentReviewOwnsClosure && ['SUCCEEDED', 'REVIEWING', 'MANUAL_REVIEW'].includes(currentTurn.status));
  const currentEvaluations = evaluations.filter((item) => item.taskTurnId === currentTurn?.id);
  const progress = task.status === 'SUCCEEDED' ? 100 : ({ QUEUED: 12, PREPARING: 25, RUNNING: 48, VALIDATING: 68, PACKAGING: 82, UPLOADING: 92, REVIEWING: 100, MANUAL_REVIEW: 100 } as Record<string, number>)[task.status] ?? 35;
  const latestDecision = currentEvaluations.at(-1)?.decision;
  const currentOutputs = outputs.filter((item) => item.taskTurnId === currentTurn?.id);
	const currentMaterializations = materializations.filter((item) => item.taskTurnId === currentTurn?.id);
	const deepLinkTurnUnavailable = Boolean(
		deepLinkTurnId
		&& deepLinkFocus
		&& !turnsLoading
		&& !turns.some((item) => item.id === deepLinkTurnId),
	);
	const materializationFor = (kind: ResultMaterializationRun['kind'], outputId?: string) => currentMaterializations.find((item) => item.kind === kind && (item.outputId ?? '') === (outputId ?? ''));
	const patchRun = materializationFor('PATCH');
	const bundleRun = materializationFor('GIT_BUNDLE');
  const evaluationTargets = useRef(new Map<API.Int64, HTMLDivElement>());
  const resultTargets = useRef(new Map<API.Int64, HTMLDivElement>());
  const continuationTargets = useRef(new Map<API.Int64, HTMLDivElement>());
  const focusedLocation = useRef('');

  useEffect(() => {
    if (!deepLinkTurnId || !deepLinkFocus || turnsLoading) {
      if (!deepLinkTurnId || !deepLinkFocus) focusedLocation.current = '';
      return;
    }
    const locationKey = `${deepLinkTurnId}:${deepLinkFocus}`;
    if (focusedLocation.current === locationKey) return;
    if (deepLinkFocus === 'delivery') return;
    const targets = deepLinkFocus === 'evaluation' ? evaluationTargets : deepLinkFocus === 'continuation' ? continuationTargets : resultTargets;
    const target = targets.current.get(deepLinkTurnId);
    if (!target) return;
    focusedLocation.current = locationKey;
    const frame = window.requestAnimationFrame(() => {
      const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
      target.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'center' });
      target.focus({ preventScroll: true });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [deepLinkFocus, deepLinkTurnId, turnsLoading, turns, evaluations, checkpoints]);

  const registerTarget = (
    targets: MutableRefObject<Map<API.Int64, HTMLDivElement>>,
    turnId: API.Int64,
  ) => (node: HTMLDivElement | null) => {
    if (node) targets.current.set(turnId, node);
    else targets.current.delete(turnId);
  };

  return (
    <Card className="workforce-continuation-shell" styles={{ body: { padding: 0 } }}>
	  {deepLinkTurnUnavailable && (
		<Alert
		  type="warning"
		  showIcon
		  title="该轮次不可用"
		  description="这条通知指向的任务轮次不存在，或不属于当前任务。你仍可以查看当前任务的最新进展。"
		  style={{ margin: 16, marginBottom: 0 }}
		/>
	  )}
      <Row gutter={0}>
        <Col xs={24} lg={15} className="workforce-turn-column">
          <Flex align="center" justify="space-between" gap={12} wrap className="workforce-section-heading">
            <div><Title level={4}>工作进展</Title><Text type="secondary">每一轮都会保留结果，继续修改也不会覆盖过去。</Text></div>
            <Tag color="blue">共 {turns.length || 1} 轮</Tag>
          </Flex>
          {turnsLoading ? <Skeleton active paragraph={{ rows: 7 }} /> : (
            <Timeline
              className="workforce-turn-timeline"
              items={(turns.length ? turns : [{ id: 'pending', turnNumber: 1, instruction: task.instruction, status: 'ACTIVE', taskId: task.id } as TaskTurn]).map((turn) => {
                const cycles = evaluations.filter((item) => item.taskTurnId === turn.id);
                const decision = cycles.at(-1)?.decision;
                const checkpoint = checkpoints.find((item) => item.taskTurnId === turn.id && item.status === 'AVAILABLE');
                const latestEvaluation = cycles.at(-1);
                const questions = evaluationQuestions(latestEvaluation?.contract);
                const active = turn.id === currentTurn?.id;
                const activeStep = continuationStep(task.status);
                const evaluationFocused = deepLinkTurnId === turn.id && deepLinkFocus === 'evaluation';
                const resultFocused = deepLinkTurnId === turn.id && deepLinkFocus === 'result';
                const continuationFocused = deepLinkTurnId === turn.id && deepLinkFocus === 'continuation';
                return {
                  color: turn.status === 'SUCCEEDED' ? 'green' : active ? 'blue' : 'gray',
                  icon: turn.status === 'SUCCEEDED' ? <CheckCircleFilled /> : active ? <ClockCircleOutlined /> : undefined,
                  content: (
                    <div ref={registerTarget(continuationTargets, turn.id)} tabIndex={-1} data-task-turn-id={turn.id} data-task-focus="continuation" style={deepLinkTargetStyle(continuationFocused)}>
                    <Card size="small" className={`workforce-turn-card${active ? ' is-active' : ''}`}>
                      <Flex justify="space-between" align="flex-start" gap={12} wrap>
                        <Space orientation="vertical" size={6} className="workforce-turn-copy">
                          <Space wrap><Text strong>第 {turn.turnNumber} 轮</Text><Tag color={turnStatusColor(turn.status)}>{turnStatusLabel(turn.status)}</Tag>{decision && <Tag color={decisionColor(decision)}>{decisionLabel(decision)}</Tag>}</Space>
                          <Text className="workforce-turn-instruction">{turn.instruction}</Text>
                          {cycles.length > 1 && <Text type="secondary">自动检查并修正了 {cycles.length - 1} 次</Text>}
                          {latestEvaluation && (
                            <div
                              ref={registerTarget(evaluationTargets, turn.id)}
                              tabIndex={-1}
                              data-task-turn-id={turn.id}
                              data-task-focus="evaluation"
                              style={deepLinkTargetStyle(evaluationFocused)}
                            >
                              <Space orientation="vertical" size={4}>
                                <Text type="secondary">本轮检查</Text>
                                {latestEvaluation.message && <Typography.Paragraph style={{ marginBottom: 0, whiteSpace: 'pre-wrap' }}>{latestEvaluation.message}</Typography.Paragraph>}
                                {questions.map((question) => <Text key={question}>• {question}</Text>)}
                              </Space>
                            </div>
                          )}
                          {(checkpoint || turn.resultSummary) && (
                            <div
                              ref={registerTarget(resultTargets, turn.id)}
                              tabIndex={-1}
                              data-task-turn-id={turn.id}
                              data-task-focus="result"
                              style={deepLinkTargetStyle(resultFocused)}
                            >
                              <Space orientation="vertical" size={3}>
                                {turn.resultSummary && <Text>{humanizeResultSummary(turn.resultSummary)}</Text>}
                                {checkpoint && <Text type="secondary">结果已安全保存 · {formatTime(checkpoint.expiresAt)} 前可继续</Text>}
                              </Space>
                            </div>
                          )}
                          {active && turn.status === 'ACTIVE' && (
                            <Steps
                              className="workforce-turn-steps"
                              orientation="vertical"
                              size="small"
                              current={activeStep}
                              items={[
                                { title: turn.sourceCheckpointId ? '恢复上次工作' : '准备工作', content: turn.sourceCheckpointId ? '正在恢复代码和工作上下文' : '正在准备本轮工作环境' },
                                { title: '继续处理', content: '根据你的要求完善结果' },
                                { title: '结果验收', content: '自动检查结果并在需要时继续修正' },
                                { title: '保存结果', content: '安全保存本轮结果，随时可以继续' },
                              ]}
                            />
                          )}
                        </Space>
                        <Text type="secondary">{formatTime(turn.finishedAt || turn.startedAt || turn.createdAt)}</Text>
                      </Flex>
                    </Card></div>
                  ),
                };
              })}
            />
          )}
          <Card size="small" className="workforce-next-turn-card">
            <Flex align="center" justify="space-between" gap={12} wrap>
              <Space orientation="vertical" size={1}><Text strong>后续轮次</Text><Text type="secondary">{independentReviewOwnsClosure ? '当前轮由独立审查流程处理，请使用上方审查工作区中的可用动作。' : canContinue ? '可以基于当前结果继续提出要求。' : '当前轮完成并保存后，可以继续提出要求。'}</Text></Space>
              {!independentReviewOwnsClosure && task.status !== 'INTERACTION_SUSPENDED' && <Button type="link" icon={<SendOutlined />} disabled={!canContinue} loading={continuing} onClick={onContinue}>
                {currentTurn?.status === 'REVIEWING' ? '回复并继续' : currentTurn?.status === 'MANUAL_REVIEW' ? '审批后继续' : '继续完善'}
              </Button>}
            </Flex>
          </Card>
        </Col>
        <Col xs={24} lg={9} className="workforce-current-column">
          <Card variant="borderless" className="workforce-current-card">
            <Flex align="center" justify="space-between" gap={20} wrap>
              {task.status !== 'INTERACTION_SUSPENDED' && <Progress type="circle" percent={progress} size={116} strokeWidth={9} strokeColor={{ '0%': '#1677ff', '100%': '#37c6ea' }} railColor="#e9f2ff" format={(value) => <div><Text strong className="workforce-progress-number">{value}%</Text><br /><Text type="secondary" className="workforce-progress-caption">整体进度</Text></div>} />}
              <Space orientation="vertical" size={10} className="workforce-current-content">
                <div><Text type="secondary">当前进度</Text><Title level={3}>{statusMeta[task.status].label}</Title></div>
                <div><Text type="secondary">当前轮次</Text><br /><Text strong>{currentTurn ? `第 ${currentTurn.turnNumber} 轮` : '第 1 轮'}</Text></div>
                <Text type="secondary">{savedInteractionProgress(task.status) || (latestDecision ? `最近检查：${decisionLabel(latestDecision)}` : currentStageHint(task.status))}</Text>
              </Space>
            </Flex>
          </Card>
          <div className="workforce-output-panel">
			<Flex justify="space-between" align="center"><Text strong>可用结果</Text>{(outputsLoading || materializationsLoading) && <Spin size="small" />}</Flex>
            <Space orientation="vertical" size={10} className="workforce-output-list">
              {currentOutputs.map((output) => (
                <Card size="small" key={output.id} className="workforce-output-item">
                  <Flex align="center" justify="space-between" gap={12}>
                    <Space orientation="vertical" size={1}><Text strong>{output.name}</Text><Text type="secondary">{output.storage === 'ON_DEMAND' ? '需要时生成' : '已随结果生成'}</Text></Space>
					<MaterializationAction
						run={materializationFor(output.outputKind === 'ARCHIVE' ? (output.archiveFormat || 'ZIP') : 'DECLARED_OUTPUT', output.id)}
						available={output.status === 'AVAILABLE'}
						loading={materializing}
						onCreate={() => onMaterialize(output.outputKind === 'ARCHIVE' ? (output.archiveFormat || 'ZIP') : 'DECLARED_OUTPUT', output.id)}
						onRetry={onRetryMaterialization}
					/>
                  </Flex>
                </Card>
              ))}
              {task.status === 'SUCCEEDED' && <>
				<MaterializationButton label="生成代码差异（Patch）" run={patchRun} loading={materializing} onCreate={() => onMaterialize('PATCH')} onRetry={onRetryMaterialization} />
				<MaterializationButton label="生成 Git Bundle" run={bundleRun} loading={materializing} onCreate={() => onMaterialize('GIT_BUNDLE')} onRetry={onRetryMaterialization} />
              </>}
              {!currentOutputs.length && task.status !== 'SUCCEEDED' && <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="任务完成后，可在这里生成需要的结果" />}
            </Space>
          </div>
        </Col>
      </Row>
    </Card>
  );
}

function MaterializationAction({ run, available, loading, onCreate, onRetry }: { run?: ResultMaterializationRun; available: boolean; loading: boolean; onCreate: () => void; onRetry: (runId: string) => void }) {
	if (available || run?.status === 'AVAILABLE') return <Tag color="green">已准备</Tag>;
	if (run?.status === 'FAILED') return <Button type="link" danger loading={loading} onClick={() => onRetry(run.id)}>重新生成</Button>;
	if (run) return <Tag icon={['PENDING', 'RUNNING', 'RETRY_WAIT', 'FROZEN'].includes(run.status) ? <Spin size="small" /> : undefined} color={materializationColor(run.status)}>{materializationLabel(run.status)}</Tag>;
	return <Button type="link" loading={loading} onClick={onCreate}>生成</Button>;
}

function TaskInputSummary({ input, gitPlan, loading }: { input?: TaskInputView; gitPlan?: GitPlanView; loading: boolean }) {
  if (loading) return <Card title="任务材料"><Skeleton active paragraph={{ rows: 2 }} /></Card>;
  if (!input) return null;
  const source = input.workspaceSource;
  const delivery = gitPlan?.deliveryMode === 'NEW_BRANCH_AND_DRAFT_REVIEW' ? '推送新分支并创建 Draft PR/MR' : gitPlan?.deliveryMode === 'NEW_BRANCH' ? '推送到新分支' : '仅保留平台结果';
  return <Card title={<Space><FolderOpenOutlined style={{ color: '#1677ff' }} />任务材料</Space>}>
    <Row gutter={[24, 20]}>
      <Col xs={24} lg={14}>
        <Descriptions size="small" column={{ xs: 1, sm: 2 }} items={[
          { key: 'source', label: '代码来源', children: sourceLabel(input) },
		  { key: 'ref', label: '来源版本', children: input.sourceKind === 'EMPTY_WORKSPACE' ? '从空白开始' : source.refName || '上传时的固定版本' },
		  ...(input.sourceKind === 'EMPTY_WORKSPACE' ? [] : [{ key: 'commit', label: '固定提交', children: <TechnicalValue value={source.resolvedCommit || '—'} copyable={Boolean(source.resolvedCommit)} /> }]),
          { key: 'references', label: '参考资料', children: `${input.references.length} 项（只读）` },
        ]} />
      </Col>
      <Col xs={24} lg={10}>
        <Flex vertical gap={8} className="workforce-input-delivery-summary">
          <Space><BranchesOutlined style={{ color: '#1677ff' }} /><Text strong>{delivery}</Text></Space>
          {gitPlan?.resultBranch && <Text type="secondary" ellipsis={{ tooltip: gitPlan.resultBranch }}>结果分支：{gitPlan.resultBranch}</Text>}
          {gitPlan?.reviewBaseBranch && <Text type="secondary">合并到：{gitPlan.reviewBaseBranch}</Text>}
          {gitPlan?.deliveryMode !== 'NONE' && <Text type="secondary">{gitPlan?.deliveryAuthorization === 'ASK_AFTER_GATES' ? '结果准备好后由你确认交付' : gitPlan?.deliveryAuthorization === 'AUTO_AFTER_GATES' ? '检查和审查通过后自动交付' : '交付时机暂不可用'}</Text>}
          {gitPlan?.previousDeliveryCommit && <Text type="secondary">本轮会接在上一轮已确认的交付提交之后</Text>}
        </Flex>
      </Col>
    </Row>
  </Card>;
}

function sourceLabel(input?: TaskInputView) {
  if (!input) return '正在读取代码来源';
	if (input.sourceKind === 'EMPTY_WORKSPACE') return '未提供起始代码';
  switch (input.workspaceSource.kind) {
    case 'GIT_REPOSITORY': return 'Git 仓库';
    case 'UPLOADED_DIRECTORY': return '浏览器上传的代码目录';
    case 'UPLOADED_ARCHIVE': return '上传的代码压缩包';
    default: return '已冻结的代码';
  }
}

function DeliveryWorkspace({ deliveries, loading, mutating, onApprove, onCancel, onRetry, onReconcile, onReconcileReview, onReauthorize, replacementVersionId, focusedDeliveryId, focusedReviewId }: {
  deliveries: DeliveryRequest[];
  loading: boolean;
  mutating: boolean;
  onApprove: (item: DeliveryRequest) => void;
  onCancel: (id: string) => void;
  onRetry: (id: string) => void;
  onReconcile: (id: string) => void;
  onReconcileReview: (id: string) => void;
  onReauthorize: (id: string, projectVersionId?: API.Int64) => void;
  replacementVersionId?: API.Int64;
  focusedDeliveryId?: API.Int64;
  focusedReviewId?: API.Int64;
}) {
  const deliveryTargets = useRef(new Map<API.Int64, HTMLDivElement>());
  const focusedLocation = useRef('');
  const selectedDelivery = focusedDeliveryId
    ? deliveries.find((item) => item.id === focusedDeliveryId)
    : focusedReviewId
      ? deliveries.find((item) => item.review?.operationId === focusedReviewId)
      : undefined;
  useEffect(() => {
    if (!selectedDelivery) return;
    const key = `${selectedDelivery.id}:${focusedReviewId ?? ''}`;
    if (focusedLocation.current === key) return;
    const target = deliveryTargets.current.get(selectedDelivery.id);
    if (!target) return;
    focusedLocation.current = key;
    const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches ?? false;
    target.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'center' });
    target.focus({ preventScroll: true });
  }, [focusedReviewId, selectedDelivery]);
  if (loading) return <Card title="结果交付"><Skeleton active paragraph={{ rows: 3 }} /></Card>;
  if (!deliveries.length) return null;
  return <Card className="workforce-delivery-card" title={<Space><CloudUploadOutlined style={{ color: '#1677ff' }} />结果交付</Space>} extra={<Text type="secondary">任务结果与交付状态相互独立</Text>}>
    <Space orientation="vertical" size={14} style={{ width: '100%' }}>
      {(focusedDeliveryId || focusedReviewId) && !selectedDelivery && <Alert type="warning" showIcon title="这条交付记录不可用" description="通知指向的交付不属于当前任务，已安全返回任务详情。" />}
      {deliveries.map((item) => <div key={item.id} ref={(node) => { if (node) deliveryTargets.current.set(item.id, node); else deliveryTargets.current.delete(item.id); }} tabIndex={-1} className={selectedDelivery?.id === item.id ? 'workforce-focus-target' : undefined}><Card size="small" className="workforce-delivery-item">
        <Flex justify="space-between" align="center" gap={16} wrap>
          <Space orientation="vertical" size={5} style={{ minWidth: 0 }}>
            <Space wrap><Text strong>{item.actionType === 'GIT_PUSH' ? '交付到 Git 仓库' : '自定义交付'}</Text><Tag color={deliveryColor(item.status)}>{deliveryLabel(item.status)}</Tag></Space>
            <Text type="secondary" ellipsis={{ tooltip: item.destinationBranch }}>{item.actionType === 'GIT_PUSH' ? `目标分支：${item.destinationBranch}` : '由已批准的处理器执行受控交付'}</Text>
            {item.reasonCode && <Text type="secondary">{deliveryReasonLabel(item.reasonCode)}</Text>}
            {item.review && <Space wrap size={6}>
              <Tag color={deliveryColor(item.review.status)}>{reviewLabel(item.review.status)}</Tag>
              {item.review.webUrl ? <Button type="link" size="small" href={item.review.webUrl} target="_blank" rel="noreferrer" style={{ paddingInline: 0 }}>打开 Draft PR/MR</Button> : <Text type="secondary">分支确认后由平台创建 Draft PR/MR</Text>}
            </Space>}
          </Space>
          <Space wrap>
            {item.status === 'WAITING_APPROVAL' && <Popconfirm title="批准并交付这份结果？" description="平台将使用冻结结果、固定分支规则和已绑定凭证执行。" onConfirm={() => onApprove(item)}><Button type="primary" loading={mutating}>批准并推送</Button></Popconfirm>}
            {['FAILED', 'MANUAL_REVIEW'].includes(item.status) && <Button loading={mutating} onClick={() => onRetry(item.id)}>重新确认</Button>}
            {item.status === 'OUTCOME_UNKNOWN' && <Button type="primary" ghost loading={mutating} onClick={() => onReconcile(item.id)}>重新读取远端状态</Button>}
            {item.status === 'CONFIRMING' && <Button loading={mutating} onClick={() => onReconcile(item.id)}>检查并恢复交付</Button>}
            {item.review?.status === 'MANUAL_REVIEW' && item.review.operationId && <Button type="primary" ghost loading={mutating} onClick={() => onReconcileReview(item.review!.operationId!)}>重新读取评审状态</Button>}
            {['FAILED', 'MANUAL_REVIEW'].includes(item.status) || item.reasonCode === 'DELIVERY_REAUTHORIZATION_REQUIRED' ? <Popconfirm title="重新确认这份结果的交付？" description={<Space orientation="vertical"><Text>使用项目中已更新的仓库凭据，保留原结果、仓库与分支。再次批准后先核对远端。</Text><Text type="secondary">项目版本：{replacementVersionId ?? '请先刷新项目'}</Text></Space>} onConfirm={() => onReauthorize(item.id, replacementVersionId)}><Button loading={mutating} disabled={!replacementVersionId}>重新授权后再试</Button></Popconfirm> : null}
            {['WAITING_APPROVAL', 'READY', 'DISPATCHED', 'RUNNING', 'CONFIRMING'].includes(item.status) && <Popconfirm title="取消交付？" description="如果 Push 已经生效，平台会保留成功事实，不会删除远端分支。" onConfirm={() => onCancel(item.id)}><Button danger type="text" loading={mutating}>取消</Button></Popconfirm>}
          </Space>
        </Flex>
        <Collapse ghost size="small" items={[{ key: 'engineering', label: '工程详情', children: <Descriptions size="small" column={{ xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }} items={[{ key: 'result', label: '可信结果', children: <TechnicalValue value={item.resultCommit} copyable /> }, { key: 'tree', label: '结果目录树', children: <TechnicalValue value={item.resultTree} copyable /> }, { key: 'delivery', label: '交付提交', children: <TechnicalValue value={item.deliveryCommit} copyable /> }, { key: 'selection', label: '文件选择身份', children: <TechnicalValue value={item.selectionDigest} copyable /> }, { key: 'request', label: '交付请求', children: <TechnicalValue value={item.id} copyable /> }, { key: 'created', label: '创建时间', children: formatTime(item.createdAt) }]} /> }]} />
      </Card></div>)}
    </Space>
  </Card>;
}

function deliveryLabel(status: string) {
  return ({ WAITING_APPROVAL: '等待批准', MANUAL_ONLY: '等待发起', READY: '准备交付', DISPATCHED: '准备交付', RUNNING: '正在推送', CONFIRMING: '正在确认', RETRY_WAIT: '等待重试', SUCCEEDED: '交付成功', FAILED: '交付失败', MANUAL_REVIEW: '需要处理', OUTCOME_UNKNOWN: '正在核对远端', CANCELLING: '正在取消', CANCELLED: '已取消' } as Record<string, string>)[status] ?? '状态暂不可用';
}
function deliveryColor(status: string) {
  if (status === 'SUCCEEDED') return 'green';
  if (status === 'FAILED') return 'red';
  if (status === 'MANUAL_REVIEW' || status === 'OUTCOME_UNKNOWN' || status === 'WAITING_APPROVAL') return 'orange';
  if (status === 'CANCELLED') return 'default';
  return 'blue';
}
function deliveryReasonLabel(reason: string) {
  return ({ REMOTE_REF_VERIFIED: '远端分支与交付提交一致，已确认。', REMOTE_REF_CHANGED: '远端分支已变化，平台没有执行 Push。', REMOTE_BASE_CHANGED: '远端基线发生变化，需要重新确认。', CANCEL_AFTER_EFFECT: '取消到达前交付已生效，远端结果已确认。', DELIVERY_RETRY_EXHAUSTED: '安全重试次数已用完，需要工程师处理。', CUSTOM_DELIVERY_REQUIRES_VERIFIER: '自定义交付已返回候选结果，等待独立核验。', DELIVERY_REAUTHORIZATION_REQUIRED: '仓库授权已经变化，需要重新确认后继续。' } as Record<string, string>)[reason] ?? '交付需要处理，请查看当前状态。';
}
function reviewLabel(status: string) {
  return ({ PENDING: '正在准备 Draft PR/MR', WAITING_DEPENDENCY: '等待分支交付', WAITING_APPROVAL: '等待确认', READY: '准备创建 Draft PR/MR', RUNNING: '正在创建 Draft PR/MR', RETRY_WAIT: '等待重试创建', SUCCEEDED: 'Draft PR/MR 已创建', FAILED: '创建 Draft PR/MR 失败', MANUAL_REVIEW: '创建 PR/MR 需要处理', OUTCOME_UNKNOWN: '正在核对 Draft PR/MR', CANCELLED: '已取消' } as Record<string, string>)[status] ?? '评审状态暂不可用';
}

function MaterializationButton({ label, run, loading, onCreate, onRetry }: { label: string; run?: ResultMaterializationRun; loading: boolean; onCreate: () => void; onRetry: (runId: string) => void }) {
	if (!run) return <Button block loading={loading} onClick={onCreate}>{label}</Button>;
	if (run.status === 'FAILED') return <Button block danger loading={loading} onClick={() => onRetry(run.id)}>{label} · 重新生成</Button>;
	return <Button block disabled icon={['PENDING', 'RUNNING', 'RETRY_WAIT', 'FROZEN'].includes(run.status) ? <Spin size="small" /> : <CheckCircleFilled />}>{label} · {materializationLabel(run.status)}</Button>;
}

function materializationLabel(status: ResultMaterializationRun['status']) {
	return ({ PENDING: '等待生成', RUNNING: '正在生成', RETRY_WAIT: '准备重试', FROZEN: '正在上传', AVAILABLE: '已准备', FAILED: '生成失败', CANCELLED: '已取消' } as const)[status];
}

function materializationColor(status: ResultMaterializationRun['status']) {
	if (status === 'AVAILABLE') return 'green';
	if (status === 'FAILED') return 'red';
	if (status === 'CANCELLED') return 'default';
	return 'blue';
}

function ExecutionProgress({ task }: { task: WorkforceTask }) {
  if (task.status === 'INTERACTION_SUSPENDED') return <Card className="workforce-progress-card"><Text>等待你回答 · 进度已保存</Text></Card>;
  const stageIndex = stages.findIndex((stage) => stage.statuses.includes(task.status));
  const current = task.status === 'SUCCEEDED' ? stages.length - 1 : Math.max(0, stageIndex);
  const stopped = ['FAILED', 'TIMED_OUT', 'CANCELLED', 'MANUAL_REVIEW', 'REVIEWING'].includes(task.status);

  return (
    <Card className="workforce-progress-card" aria-label="任务执行进度">
      <Steps
        current={current}
        status={stopped ? 'error' : task.status === 'SUCCEEDED' ? 'finish' : task.status === 'QUEUED' ? 'wait' : 'process'}
        responsive
        items={stages.map((stage, index) => ({
          title: stage.label,
          content: task.status === 'SUCCEEDED' || index < current
            ? '已完成'
            : index === current && !stopped
              ? elapsed(task)
              : stopped
                ? '未执行'
                : '待开始',
        }))}
      />
    </Card>
  );
}

function OverviewTab({ task, events, workerId, recovery, recoveryLoading, providerInvocations, providerInvocationsLoading, evaluations, evaluationsLoading, checkpoints, checkpointsLoading, retryingFinalization, retryingUpload, onRetryFinalization, onRetryUpload }: {
  task: WorkforceTask;
  events: WorkforceEvent[];
  workerId: string;
  recovery?: WorkforceRecoveryView;
  recoveryLoading: boolean;
  providerInvocations: WorkforceModelInvocation[];
  providerInvocationsLoading: boolean;
  evaluations: ResultEvaluationCycle[];
  evaluationsLoading: boolean;
  checkpoints: ContinuationCheckpoint[];
  checkpointsLoading: boolean;
  retryingFinalization: boolean;
  retryingUpload: boolean;
  onRetryFinalization: (id: string) => void;
  onRetryUpload: (id: string) => void;
}) {
  const execution = task.execution;
  const resolved = execution?.resolved;
  const activeMinutes = task.executionActiveMilliseconds === undefined ? null : Number(task.executionActiveMilliseconds) / 60_000;
  const executionItems = [
    { key: 'worker', label: 'Worker', children: workerId },
    { key: 'runtime', label: 'Agent Runtime', children: resolved ? <Space wrap><Tag>{resolved.runtimeAlias}</Tag><Tag>{resolved.adapterId}</Tag></Space> : <Text type="secondary">等待调度</Text> },
    { key: 'execution-timeout', label: 'Agent 执行时限', children: formatExecutionTimeout(task.executionTimeoutSeconds) },
    { key: 'active-duration', label: '已确认活跃执行', children: activeMinutes === null ? '暂无记录' : `${activeMinutes < 1 ? '<1' : Math.floor(activeMinutes)} 分钟` },
    { key: 'latest-progress', label: '最近一次执行进展', children: task.lastProgressAt ? formatTime(task.lastProgressAt) : '暂无可确认的执行进展' },
    { key: 'current-wait', label: '当前等待', children: execution?.queueWaitReason ? waitReasonLabel(execution.queueWaitReason) : task.status === 'INTERACTION_SUSPENDED' ? '等待用户回答' : '无' },
    { key: 'attempt', label: 'Attempt', children: <Text code>{task.latestAttemptId ? short(task.latestAttemptId) : '等待调度'}</Text> },
    { key: 'workspace', label: 'Workspace', children: <Text code copyable>{`/workspace/${task.id}`}</Text> },
  ];
  const sourceItems = [
    { key: 'repository', label: 'Repository', children: repositoryName(task.repositoryUrl) },
    { key: 'ref', label: 'Ref', children: <Tag>{task.repositoryRef}</Tag> },
    { key: 'commit', label: 'Base commit', children: <Text code>{task.resolvedBaseCommit || '等待解析'}</Text> },
    { key: 'processor', label: 'Result Processor', children: task.resultProcessorReleaseId ? <TechnicalValue value={task.resultProcessorReleaseId} copyable /> : <Text type="secondary">未配置</Text> },
  ];

  return (
    <Space orientation="vertical" size={16} className="workforce-tab-content">
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={12}>
          <Card size="small" title="执行信息">
            <Descriptions column={1} size="small" bordered items={executionItems} />
          </Card>
        </Col>
        <Col xs={24} xl={12}>
          <Card size="small" title="代码基线">
            <Descriptions column={1} size="small" bordered items={sourceItems} />
          </Card>
        </Col>
      </Row>
      {execution && (
        <Card size="small" title="调度与 Runtime 身份">
          <Descriptions
            className="workforce-technical-descriptions"
            column={{ xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }}
            layout="vertical"
            size="small"
            bordered
            items={[
              { key: 'node', label: '执行节点（ExecutionNode）', children: <TechnicalValue value={execution.executionNodeId} copyable /> },
              { key: 'node-status', label: '节点状态 / Generation', children: <Space wrap><Tag>{execution.nodeStatus}</Tag><Text>g{execution.generation}</Text><Text type="secondary">owner {execution.ownerEpoch || '—'}</Text></Space> },
              { key: 'assignment', label: '任务分配（Assignment）', children: execution.assignmentId ? <TechnicalValue value={execution.assignmentId} copyable /> : '等待分配' },
              { key: 'assignment-status', label: '分配状态', children: <Tag>{execution.assignmentStatus || 'QUEUED'}</Tag> },
              { key: 'worker-instance', label: 'Worker / 进程实例', children: <Space orientation="vertical" size={1} className="workforce-technical-stack"><TechnicalValue value={execution.workerId} /><TechnicalValue value={execution.workerInstanceId} secondary /></Space> },
              { key: 'host-boot', label: '主机启动标识', children: <TechnicalValue value={execution.hostBootId} /> },
              { key: 'requirement', label: '执行要求摘要（Requirement Digest）', span: 'filled' as const, children: <TechnicalValue value={execution.executionRequirementDigest} copyable /> },
              { key: 'runtime', label: '容器 Runtime', children: <Space wrap><Tag>{execution.runtimeStatus || '未创建'}</Tag><TechnicalValue value={execution.containerName} /></Space> },
              { key: 'container', label: '容器 ID', children: <TechnicalValue value={execution.containerId} copyable /> },
              { key: 'identity', label: '执行身份摘要', span: 'filled' as const, children: <TechnicalValue value={execution.executionIdentityDigest} copyable /> },
              { key: 'lease', label: '执行租约到期', children: formatTime(execution.executionLeaseExpiresAt) },
              { key: 'wait', label: '等待原因', children: execution.queueWaitReason ? <Tag color="gold">{waitReasonLabel(execution.queueWaitReason)}</Tag> : '—' },
				{ key: 'selected-runtime', label: '选择的执行镜像', children: task.selectedRuntime ? <Space wrap><Tag color="blue">{task.selectedRuntime.name}</Tag><TechnicalValue value={task.selectedRuntime.runtimeReleaseId} copyable /></Space> : task.selectedRuntimeReleaseId ? <TechnicalValue value={task.selectedRuntimeReleaseId} copyable /> : '—' },
              ...(resolved ? [
                { key: 'runtime-selection', label: '实际 Runtime', children: <Space wrap><Tag>{resolved.runtimeAlias}</Tag><TechnicalValue value={resolved.runtimeReleaseId} copyable /></Space> },
                { key: 'runtime-image', label: 'Runtime 镜像摘要', span: 'filled' as const, children: <TechnicalValue value={resolved.runtimeImageDigest} copyable /> },
                { key: 'adapter-selection', label: 'Adapter', children: <Space wrap><Tag>{resolved.adapterId}</Tag><Tag>{resolved.adapterVersion}</Tag><Tag>{resolved.adapterProtocol}</Tag></Space> },
                { key: 'model-selection', label: '模型密钥 / 工作模型', children: <Space wrap><Tag>{resolved.modelCredentialName}</Tag><Tag>{resolved.primaryModel}</Tag><Tag>{resolved.modelProtocol}</Tag></Space> },
                { key: 'credential-version', label: '调用凭据版本', children: <TechnicalValue value={resolved.credentialVersion} copyable /> },
                { key: 'selection', label: '调度选择摘要', span: 'filled' as const, children: <TechnicalValue value={resolved.selectionDigest} copyable /> },
              ] : []),
            ]}
          />
        </Card>
      )}
      {resolved && <HookExecutionCard hooks={resolved.hooks ?? []} digest={resolved.hookConfigurationDigest} events={events} />}
      {task.resultProcessorReleaseId && <ResultEvaluationCard evaluations={evaluations} loading={evaluationsLoading} checkpoints={checkpoints} checkpointsLoading={checkpointsLoading} />}
      <RecoveryCard
        task={task}
        recovery={recovery}
        loading={recoveryLoading}
        retryingFinalization={retryingFinalization}
        retryingUpload={retryingUpload}
        onRetryFinalization={onRetryFinalization}
        onRetryUpload={onRetryUpload}
      />
      {resolved?.modelCredentialId && <ModelInvocationCard execution={resolved} invocations={providerInvocations} loading={providerInvocationsLoading} />}
      {execution && (
        <Card size="small" title="启动门禁与消息确认">
          <Timeline
            items={[
              { label: 'ContainerCreated', at: execution.containerCreatedAt, text: '容器身份已由 Server 接受；Task 仍处于 PREPARING' },
              { label: 'SupervisorReady', at: execution.supervisorReadyAt, text: 'Supervisor v2 已就绪，尚未运行 Hook 或 Harness' },
              { label: 'RunPermit', at: execution.runPermitIssuedAt, text: 'Server 签发一次性运行许可和 runNonce' },
              { label: 'HARNESS_STARTED', at: execution.harnessStartedAt, text: 'Harness 已实际启动，Server 原子写入 RUNNING' },
              { label: 'RabbitMQ ACK', at: execution.deliveryAckedAt, text: 'Assignment Delivery 已确认，不再占用 Broker 消息' },
            ].map((item) => ({
              color: item.at ? 'green' : 'gray',
              content: <Space orientation="vertical" size={1}><Space wrap><Text strong>{item.label}</Text><Text type="secondary">{formatTime(item.at)}</Text></Space><Text type="secondary">{item.text}</Text></Space>,
            }))}
          />
        </Card>
      )}
      {task.resultSummary && (
        <Alert
          type={task.status === 'SUCCEEDED' ? 'success' : task.status === 'FAILED' ? 'error' : 'info'}
          showIcon
          title="执行摘要"
          description={humanizeResultSummary(task.resultSummary)}
        />
      )}
      <Alert
        type="info"
        showIcon
        icon={<SafetyCertificateOutlined />}
        title="执行安全边界"
        description="隔离容器 · 无控制端口 · Finalizer 独立校验"
      />
    </Space>
  );
}

function HookExecutionCard({ hooks, digest, events }: {
  hooks: NonNullable<NonNullable<WorkforceTask['execution']>['resolved']>['hooks'];
  digest?: string;
  events: WorkforceEvent[];
}) {
  if (hooks.length === 0) return null;
  const loaded = events.some((event) => event.type === 'hook.configuration.loaded' && (!digest || event.payload.configurationDigest === digest));
  const invocations = events.filter((event) => event.type === 'hook.allow' || event.type === 'hook.deny' || event.type === 'hook.failed');
  const stats = new Map<string, { total: number; denied: number; failed: number }>();
  for (const event of invocations) {
    const releaseId = typeof event.payload.packageReleaseId === 'string' ? event.payload.packageReleaseId : '';
    const value = stats.get(releaseId) ?? { total: 0, denied: 0, failed: 0 };
    value.total += 1;
    if (event.type === 'hook.deny') value.denied += 1;
    if (event.type === 'hook.failed') value.failed += 1;
    stats.set(releaseId, value);
  }
  return (
    <Card size="small" title="Agent Hooks" extra={<Tag color={loaded ? 'green' : 'gold'}>{loaded ? '已加载' : '等待加载'}</Tag>}>
      <Space orientation="vertical" size={12} className="workforce-tab-content">
        {hooks.map((hook, index) => {
          const value = stats.get(hook.releaseId) ?? { total: 0, denied: 0, failed: 0 };
          const outcome = value.failed > 0 ? `失败 ${value.failed} 次` : value.denied > 0 ? `拦截 ${value.denied} 次` : value.total > 0 ? '全部通过' : '尚未调用';
          return (
            <Flex key={hook.releaseId} justify="space-between" align="center" gap={12} wrap>
              <Space size={8} wrap><Tag>{index + 1}</Tag><Text strong>{hook.name}</Text><Text type="secondary">v{hook.version}</Text></Space>
              <Text type={value.failed > 0 || value.denied > 0 ? undefined : 'secondary'}>执行 {value.total} 次 · {outcome}</Text>
            </Flex>
          );
        })}
        <Collapse size="small" ghost items={[{ key: 'identity', label: '加载信息', children: <TechnicalValue value={digest} copyable /> }]} />
      </Space>
    </Card>
  );
}

function ModelInvocationCard({ execution, invocations, loading }: { execution: NonNullable<WorkforceTask['execution']>['resolved']; invocations: WorkforceModelInvocation[]; loading: boolean }) {
  return (
    <Card size="small" title="模型调用" extra={<Text type="secondary">按实际请求模型统计</Text>}>
      <Descriptions
        size="small"
        bordered
        column={{ xs: 1, sm: 1, md: 2, lg: 2 }}
        style={{ marginBottom: 16 }}
        items={[
          { key: 'credential', label: '模型密钥', children: <Space><Text>{execution?.modelCredentialName || '—'}</Text><TechnicalValue value={execution?.modelCredentialId} copyable /></Space> },
          { key: 'protocol', label: '协议', children: <Tag>{execution?.modelProtocol || '—'}</Tag> },
          { key: 'model', label: '工作模型', children: <Tag>{execution?.primaryModel || '—'}</Tag> },
          { key: 'adapter', label: 'Agent Adapter', span: 'filled' as const, children: <Space wrap><Tag>{execution?.adapterId || '—'}</Tag><Tag>{execution?.adapterVersion || '—'}</Tag></Space> },
        ]}
      />
      <Table<WorkforceModelInvocation>
        rowKey="id"
        size="small"
        loading={loading}
        dataSource={invocations}
        pagination={false}
        scroll={{ x: 920 }}
        locale={{ emptyText: '尚无模型调用事实' }}
        columns={[
          { title: '调用 ID', dataIndex: 'id', width: 210, ellipsis: true, render: (value) => <TechnicalValue value={value} copyable /> },
          { title: '实际模型', dataIndex: 'requestedModel', width: 190, ellipsis: true },
          { title: '用途', key: 'modelKind', width: 110, render: (_, item) => item.isPrimaryModel ? '主模型' : '辅助调用' },
          { title: '操作', dataIndex: 'operation', width: 150 },
          { title: '状态', dataIndex: 'status', width: 120, render: (value) => <Tag color={value === 'SUCCEEDED' ? 'green' : value === 'FAILED' ? 'red' : value === 'CANCELLED' ? 'default' : 'blue'}>{providerInvocationStatusLabel(value)}</Tag> },
          { title: '首字节 / 总耗时', key: 'latency', width: 150, render: (_, item) => `${item.firstByteMs ?? '—'} / ${item.durationMs ?? '—'} ms` },
          { title: '输入 / 缓存 / 输出 / 总计', key: 'usage', width: 250, render: (_, item) => item.usage?.totalTokens == null ? <Text type="secondary">上游未返回</Text> : `${item.usage.inputTokens ?? '—'} / ${item.usage.cachedInputTokens ?? '—'} / ${item.usage.outputTokens ?? '—'} / ${item.usage.totalTokens}` },
          { title: '结果', key: 'result', width: 180, render: (_, item) => item.errorCode ? <Tag color="red">{item.errorCode}</Tag> : (item.statusCode ?? '—') },
        ]}
      />
    </Card>
  );
}

function ResultEvaluationCard({ evaluations, loading, checkpoints, checkpointsLoading }: {
  evaluations: ResultEvaluationCycle[];
  loading: boolean;
  checkpoints: ContinuationCheckpoint[];
  checkpointsLoading: boolean;
}) {
  const columns: TableColumnsType<ResultEvaluationCycle> = [
    { title: '轮次', dataIndex: 'cycleNumber', width: 92, render: (value: number) => `Cycle ${value}` },
    { title: 'Decision', dataIndex: 'decision', width: 150, render: (value: string) => <Tag color={decisionColor(value)}>{decisionLabel(value)}</Tag> },
    { title: '原因', dataIndex: 'reasonCode', width: 230, ellipsis: true, render: (value: string) => <Text code>{value || '—'}</Text> },
    { title: '处理器消息', dataIndex: 'message', width: 300, ellipsis: true, render: (value: string) => <Text ellipsis={{ tooltip: value }}>{value || '—'}</Text> },
    { title: '状态', dataIndex: 'status', width: 120, render: (value: string) => <Tag>{value}</Tag> },
    { title: '时间', dataIndex: 'persistedAt', width: 180, render: (value: string) => formatTime(value) },
  ];
  const checkpointColumns: TableColumnsType<ContinuationCheckpoint> = [
    { title: 'Checkpoint', dataIndex: 'id', width: 210, ellipsis: true, render: (value: string) => <TechnicalValue value={value} copyable /> },
    { title: '来自 Cycle', dataIndex: 'evaluationCycleId', width: 170, ellipsis: true, render: (value: string) => <TechnicalValue value={value} /> },
    { title: 'Decision', dataIndex: 'decision', width: 140, render: (value: string) => <Tag color={decisionColor(value)}>{decisionLabel(value)}</Tag> },
    { title: '状态', dataIndex: 'status', width: 120, render: (value: string) => <Tag color={value === 'AVAILABLE' ? 'green' : 'default'}>{value}</Tag> },
    { title: '到期时间', dataIndex: 'expiresAt', width: 180, render: (value: string) => formatTime(value) },
  ];
  return <Card size="small" title="Result Processing 与自动修正" extra={<Text type="secondary">同一 Attempt / Session 内完成</Text>}>
    <Space orientation="vertical" size={16} className="workforce-recovery-content">
      <Alert type="info" showIcon title="每一次检查都有记录" description="需要修正时会继续同一工作会话；通过前不会生成最终结果。等待回复或人工处理时，会先安全保存代码和会话，之后可从同一个任务继续。" />
      <Table<ResultEvaluationCycle> rowKey="id" size="small" loading={loading} columns={columns} dataSource={evaluations} pagination={false} scroll={{ x: 980 }} locale={{ emptyText: 'Agent round 结束后将产生 Result EvaluationCycle' }} />
      <div>
        <Flex justify="space-between" align="center" wrap gap={8}><Text strong>可继续的保存点</Text><Text type="secondary">用于恢复代码和工作会话</Text></Flex>
        <Table<ContinuationCheckpoint> rowKey="id" size="small" loading={checkpointsLoading} columns={checkpointColumns} dataSource={checkpoints} pagination={false} scroll={{ x: 820 }} locale={{ emptyText: 'PASS、WAIT_USER 或 MANUAL_REVIEW 后会冻结内部 Checkpoint' }} />
      </div>
    </Space>
  </Card>;
}

function decisionLabel(value: string) {
  return ({ PASS: '通过', REVISE: '需要修正', WAIT_USER: '等待用户', REJECT: '拒绝', MANUAL_REVIEW: '人工处理' } as Record<string, string>)[value] ?? value;
}

function decisionColor(value: string) {
  if (value === 'PASS') return 'green';
  if (value === 'REVISE') return 'blue';
  if (value === 'REJECT') return 'red';
  return 'orange';
}

function notificationEventSummary(events: TaskNotificationEvent[]) {
  const labels = new Map(taskNotificationEventOptions.map((option) => [option.value, option.label]));
  return events.map((event) => labels.get(event) ?? event).join('、') || '未选择';
}

function notificationChannelsFromKeys(keys: string[]): TaskNotificationChannel[] {
  return (keys?.length ? keys : ['IN_APP']).map((key) => key.startsWith('GROUP_ROUTE:')
    ? { kind: 'GROUP_ROUTE', routeId: key.slice('GROUP_ROUTE:'.length) }
    : { kind: key as Exclude<TaskNotificationChannel['kind'], 'GROUP_ROUTE'> });
}

function notificationChannelKeys(channels: TaskNotificationChannel[]) {
  return channels.map((channel) => channel.kind === 'GROUP_ROUTE' ? `GROUP_ROUTE:${channel.routeId}` : channel.kind);
}

function notificationChannelLabel(channel: TaskNotificationChannel, routes?: Array<{ id: API.Int64; name: string }>) {
  if (channel.kind === 'GROUP_ROUTE') return routes?.find((route) => route.id === channel.routeId)?.name ?? '组织通知群';
  return ({ IN_APP: '站内信', EMAIL_USER: '邮箱', FEISHU_DIRECT: '个人飞书', WECOM_DIRECT: '个人企业微信' } as const)[channel.kind];
}

function notificationRecordTitle(record: TaskNotificationRecord) {
  if (record.reasonCode === 'DELIVERY_WAITING_APPROVAL') return '交付结果等待批准';
  if (record.reasonCode === 'DELIVERY_OUTCOME_UNKNOWN') return '交付结果需要确认';
  if (record.sourceType === 'DELIVERY_REQUEST' || record.sourceType === 'REVIEW_OPERATION') return '交付需要人工处理';
  if (record.reasonCode === 'PROCESSOR_WAIT_USER') return '任务需要补充信息';
  if (record.reasonCode === 'PROCESSOR_MANUAL_REVIEW' || record.reasonCode === 'FINALIZATION_RETRY_EXHAUSTED' || record.reasonCode === 'ARTIFACT_UPLOAD_RETRY_EXHAUSTED') return '任务需要人工处理';
  return ({
    TASK_ACTION_REQUIRED: '任务需要你处理',
    TASK_INTERRUPTED: '任务执行已中断',
    TASK_COMPLETED: '当前轮已经完成',
  } as Record<TaskNotificationRecord['factType'], string>)[record.factType];
}

function notificationActionLabel(record: TaskNotificationRecord) {
  if (record.focus === 'delivery' && record.reasonCode === 'DELIVERY_WAITING_APPROVAL') return '确认交付';
  if (record.focus === 'result') return record.factType === 'TASK_COMPLETED' ? '查看结果' : '去处理';
  if (record.factType === 'TASK_ACTION_REQUIRED') return '去处理';
  return '查看任务';
}

function notificationTurnLabel(record: TaskNotificationRecord, turns: TaskTurn[]) {
  const turn = turns.find((item) => item.id === record.taskTurnId);
  return turn ? `第 ${turn.turnNumber} 轮` : '任务进展';
}

function TaskNotificationStatusTag({ status }: { status: TaskNotificationRecord['status'] }) {
  const metadata = ({
    PENDING: { label: '等待发送', color: 'blue' },
    PROCESSING: { label: '发送中', color: 'blue' },
    RETRY_WAIT: { label: '准备重试', color: 'gold' },
    PUBLISHED: { label: '已送达', color: 'green' },
    SUPPRESSED: { label: '未发送', color: 'default' },
    FAILED: { label: '发送失败', color: 'red' },
  } as const)[status] ?? { label: '状态更新中', color: 'default' };
  return <Tag color={metadata.color}>{metadata.label}</Tag>;
}

function evaluationQuestions(contract?: Record<string, unknown>) {
  const raw = contract?.questions;
  if (!Array.isArray(raw)) return [];
  return [...new Set(raw.flatMap((item) => {
    if (!item || typeof item !== 'object') return [];
    const question = (item as { question?: unknown }).question;
    return typeof question === 'string' && question.trim() ? [question.trim()] : [];
  }))];
}

function deepLinkTargetStyle(focused: boolean): CSSProperties {
  return {
    borderRadius: 8,
    margin: -8,
    padding: 8,
    outline: focused ? '3px solid rgba(22, 119, 255, 0.28)' : '3px solid transparent',
    outlineOffset: 2,
    background: focused ? 'rgba(22, 119, 255, 0.06)' : 'transparent',
  };
}

function turnStatusLabel(value: string) {
  return ({ ACTIVE: '进行中', INTERACTION_SUSPENDED: '等待你回答', SUCCEEDED: '已完成', REVIEWING: '等你回复', MANUAL_REVIEW: '待审批' } as Record<string, string>)[value] ?? value;
}

function turnStatusColor(value: string) {
  if (value === 'SUCCEEDED') return 'green';
  if (value === 'ACTIVE') return 'blue';
  return 'orange';
}

function currentStageHint(value: TaskStatus) {
  if (value === 'INTERACTION_SUSPENDED') return '进度已保存';
  return ({ QUEUED: '正在寻找合适的执行资源', PREPARING: '正在恢复代码和工作环境', RUNNING: '正在完成本轮要求', VALIDATING: '正在检查结果', PACKAGING: '正在保存可信结果', UPLOADING: '正在准备可下载内容', REVIEWING: '正在处理任务结果', MANUAL_REVIEW: '需要人工决定', SUCCEEDED: '本轮已经完成，可以继续完善' } as Partial<Record<TaskStatus, string>>)[value] ?? '正在处理';
}

function humanizeResultSummary(summary?: string) {
  const value = summary?.trim();
  if (!value) return '';
  const known = {
    CHECK_UNAVAILABLE: '当前检查因命令或环境条件不足，未能完成。',
    'Finalization and artifact verification passed': '结果已完成复核，所需文件已经准备好。',
    'Independent review passed': '独立审查已通过，结果可以交付。',
    'Independent review requires manual action': '独立审查需要人工决定。',
    'Human accepted independent review risk': '已人工接受当前风险，结果可以交付。',
    INDEPENDENT_REVIEW_REJECTED: 'AI Reviewer 拒绝当前结果，需要人工决定。',
    INDEPENDENT_REVIEW_REPORT_INVALID: '审查报告未通过可信校验，需要人工决定。',
    INDEPENDENT_REVIEW_RETRY_EXHAUSTED: '审查运行重试已耗尽，需要人工决定。',
    INDEPENDENT_REVIEW_REVISION_LIMIT_EXHAUSTED: '自动修订额度已耗尽，需要人工决定。',
    INDEPENDENT_REVIEW_NO_MATERIAL_PROGRESS: '连续审查未发现实质进展，需要人工决定。',
  } as Record<string, string>;
  if (known[value]) return known[value];
  return value;
}

function RecoveryCard({ task, recovery, loading, retryingFinalization, retryingUpload, onRetryFinalization, onRetryUpload }: {
  task: WorkforceTask;
  recovery?: WorkforceRecoveryView;
  loading: boolean;
  retryingFinalization: boolean;
  retryingUpload: boolean;
  onRetryFinalization: (id: string) => void;
  onRetryUpload: (id: string) => void;
}) {
  if (loading) return <Card size="small" title="恢复与重试"><Skeleton active paragraph={{ rows: 4 }} /></Card>;
  const execution = recovery?.execution ?? task.execution;
  const runs = recovery?.finalizationRuns ?? [];
  const finalizationColumns: TableColumnsType<WorkforceFinalizationRun> = [
    { title: '收尾任务', dataIndex: 'id', width: 190, ellipsis: true, render: (value: string) => <TechnicalValue value={value} copyable /> },
    { title: '状态', dataIndex: 'status', width: 120, render: (value: string) => <Tag color={postprocessColor(value)}>{postprocessLabel(value)}</Tag> },
    { title: '次数', width: 92, render: (_, item) => `${item.runNumber} / ${item.maxRuns}` },
    { title: '错误原因', dataIndex: 'errorCode', width: 220, ellipsis: true, render: (value: string) => value || '—' },
    {
      title: '操作', width: 110, fixed: 'right',
      render: (_, item) => item.status === 'FAILED' && item.errorCode !== 'FINALIZATION_SOURCE_LOST'
        ? <Button type="link" icon={<RedoOutlined />} loading={retryingFinalization} onClick={() => onRetryFinalization(item.id)}>重试收尾</Button>
        : '—',
    },
  ];
  return (
    <Card size="small" title="恢复与重试" extra={<Text type="secondary">后处理重试不会重新运行 Agent</Text>}>
      <Space orientation="vertical" size={16} className="workforce-recovery-content">
        <Descriptions
          bordered
          layout="vertical"
          size="small"
          column={{ xs: 1, sm: 1, md: 2, lg: 2, xl: 2, xxl: 2 }}
          className="workforce-technical-descriptions"
          items={[
            { key: 'runtime', label: '运行环境状态', children: <Tag color={execution?.runtimeStatus === 'RUNNING' ? 'green' : execution?.runtimeStatus === 'RECOVERING' ? 'gold' : 'default'}>{runtimeLabel(execution?.runtimeStatus)}</Tag> },
            { key: 'attempt', label: 'Agent 执行次数', children: `${execution?.attemptNumber || Math.max(1, (execution?.retryCount ?? 0) + 1)} / ${execution?.maxAttempts || 3}` },
            { key: 'owner', label: '当前 Owner', children: <Space orientation="vertical" size={1}><TechnicalValue value={execution?.workerInstanceId} /><Text type="secondary">ownerEpoch {execution?.ownerEpoch || '—'}</Text></Space> },
            { key: 'lease', label: '执行租约 / Watchdog', children: <Space orientation="vertical" size={1}><Text>{formatTime(execution?.executionLeaseExpiresAt)}</Text><Text type="secondary">Watchdog：{formatTime(execution?.watchdogDeadline)}</Text></Space> },
            { key: 'recovery', label: '接管窗口', children: execution?.reattachDeadline ? <Space orientation="vertical" size={1}><Tag color="gold">{execution?.pendingWorkerInstanceId ? '正在接管' : '等待接管'}</Tag><Text type="secondary">截止 {formatTime(execution.reattachDeadline)}</Text></Space> : '—' },
            { key: 'retry', label: '重新执行', children: execution?.nextRetryAt ? <Space orientation="vertical" size={1}><Tag>等待重新执行</Tag><Text type="secondary">{execution.lastRetryReasonCode || '基础设施故障'} · {formatTime(execution.nextRetryAt)}</Text></Space> : execution?.terminalReasonCode || '—' },
            { key: 'cancel', label: '取消请求', span: 'filled' as const, children: recovery?.cancelRequestId ? <Space wrap><Tag color="orange">正在取消</Tag><TechnicalValue value={recovery.cancelRequestId} /><Text type="secondary">{formatTime(recovery.cancelRequestedAt)} · {recovery.cancelReason || '用户取消'}</Text></Space> : '—' },
          ]}
        />
        <Table<WorkforceFinalizationRun>
          rowKey="id"
          size="small"
          pagination={false}
          tableLayout="fixed"
          scroll={{ x: 820 }}
          dataSource={runs}
          columns={finalizationColumns}
          expandable={{
            expandedRowRender: (run) => (
              <Table<WorkforceArtifactUploadAttempt>
                rowKey="id"
                size="small"
                pagination={false}
                scroll={{ x: 760 }}
                dataSource={run.artifactUploadAttempts}
                columns={[
                  { title: '制品', dataIndex: 'name', width: 180, ellipsis: true },
                  { title: '状态', dataIndex: 'status', width: 110, render: (value: string) => <Tag color={postprocessColor(value)}>{postprocessLabel(value)}</Tag> },
                  { title: '上传次数', width: 100, render: (_: unknown, item: { attemptNumber: number; maxAttempts: number }) => `${item.attemptNumber} / ${item.maxAttempts}` },
                  { title: 'SHA-256', dataIndex: 'sha256', width: 230, ellipsis: true, render: (value: string) => <TechnicalValue value={value} copyable /> },
                  { title: '错误原因', dataIndex: 'errorCode', width: 180, ellipsis: true, render: (value: string) => value || '—' },
                  { title: '操作', width: 110, fixed: 'right', render: (_: unknown, item: { id: string; status: string }) => item.status === 'FAILED' ? <Button type="link" icon={<RedoOutlined />} loading={retryingUpload} onClick={() => onRetryUpload(item.id)}>重试上传</Button> : '—' },
                ]}
              />
            ),
            rowExpandable: (run) => run.artifactUploadAttempts.length > 0,
          }}
          locale={{ emptyText: 'Agent 执行完成后才会创建收尾任务' }}
        />
      </Space>
    </Card>
  );
}

function TechnicalValue({ value, copyable = false, secondary = false }: { value?: string; copyable?: boolean; secondary?: boolean }) {
  const text = value?.trim() || '—';
  return <Text code copyable={copyable && text !== '—' ? { text } : false} type={secondary ? 'secondary' : undefined} className="workforce-technical-value">{text}</Text>;
}

function providerInvocationStatusLabel(status: string) {
  return ({ AUTHORIZED: '已授权', STREAMING: '响应中', SUCCEEDED: '成功', FAILED: '失败', CANCELLED: '已取消', OUTCOME_UNKNOWN: '结果未知' } as Record<string, string>)[status] ?? status;
}

function formatExecutionTimeout(seconds?: number): string {
  if (!seconds || seconds <= 0) return '不限时';
  if (seconds % 86400 === 0) return `${seconds / 86400} 天`;
  if (seconds % 3600 === 0) return `${seconds / 3600} 小时`;
  if (seconds % 60 === 0) return `${seconds / 60} 分钟`;
  return `${seconds} 秒`;
}

function formatTime(value?: string) {
  const raw = typeof value === 'string' ? value.trim() : '';
  if (!raw) return '等待中';

  // API timestamps are normally RFC 3339.  Keep a narrow compatibility path
  // for PostgreSQL-style offsets (for example `+0800`), and never render the
  // browser's unhelpful `Invalid Date` placeholder in an operator audit view.
  const normalized = raw
    .replace(' ', 'T')
    .replace(/([+-]\d{2})(\d{2})$/, '$1:$2');
  const timestamp = Date.parse(normalized);
  return Number.isNaN(timestamp)
    ? raw
    : new Date(timestamp).toLocaleString('zh-CN', { hour12: false });
}

function waitReasonLabel(value: string) {
  const labels: Record<string, string> = {
    NO_ONLINE_WORKER: '没有在线 Worker',
    NO_COMPATIBLE_WORKER: '没有能力匹配的 Worker',
    REQUIRED_IMAGE_UNAVAILABLE: '所需镜像不可用',
		WAITING_SELECTED_RUNTIME: '等待所选执行镜像可用',
    WAITING_WORKER_CAPACITY: '等待 Worker 可用槽位',
    WORKER_DRAINING: 'Worker 正在排空',
    WORKER_DISK_PRESSURE: 'Worker 工作区磁盘压力',
    ASSIGNMENT_RETRY_PENDING: '等待重新分配',
  };
  return labels[value] ?? value;
}

function runtimeLabel(value?: string) {
  const labels: Record<string, string> = {
    AUTHORIZED: '已授权创建', CONTAINER_CREATED: '容器已创建', SUPERVISOR_READY: 'Supervisor 已就绪',
    STARTING: '正在启动 Harness', RUNNING: '运行中', RECOVERING: '等待接管', REATTACHING: '正在接管',
    TERMINATING: '正在终止', RECONCILIATION_REQUIRED: '需要核对运行环境', LOST: '运行环境已丢失',
    FENCED: '旧 Owner 已隔离', SUCCEEDED: '已成功', FAILED: '失败', CANCELLED: '已取消', TIMED_OUT: '已超时',
  };
  return value ? (labels[value] ?? value) : '尚未创建';
}

function postprocessLabel(value: string) {
  const labels: Record<string, string> = {
    PENDING: '等待处理', RUNNING: '正在收尾', RETRY_WAIT: '等待重试', FROZEN: '制品已冻结',
    UPLOADING: '正在上传', VERIFYING: '正在校验', VERIFIED: '已校验', FAILED: '失败', CANCELLED: '已取消',
  };
  return labels[value] ?? value;
}

function postprocessColor(value: string) {
  if (value === 'VERIFIED' || value === 'FROZEN') return 'green';
  if (value === 'FAILED') return 'red';
  if (value === 'RETRY_WAIT') return 'gold';
  if (value === 'RUNNING' || value === 'UPLOADING' || value === 'VERIFYING') return 'blue';
  return 'default';
}

function ActivityTab({ events, loading, onRefresh }: { events: WorkforceEvent[]; loading: boolean; onRefresh: () => unknown }) {
  const [showTechnical, setShowTechnical] = useState(false);
  if (loading) return <Skeleton active paragraph={{ rows: 7 }} />;
  const rows = presentTaskEvents(events, showTechnical);

  return (
    <Space orientation="vertical" size={16} className="workforce-tab-content">
      <Flex align="center" justify="space-between" gap={12}>
        <Text type="secondary">关键执行记录</Text>
        <Space size={12}>
          <Flex align="center" gap={8}>
            <Text type="secondary">技术事件</Text>
            <Switch size="small" checked={showTechnical} onChange={setShowTechnical} aria-label="显示技术事件" />
          </Flex>
          <Button icon={<ReloadOutlined />} onClick={onRefresh}>刷新</Button>
        </Space>
      </Flex>
      {rows.length ? (
        <Timeline
          items={rows.map((row) => {
            if (row.kind === 'conversation-sync') {
              return {
                color: 'gray',
                content: (
                  <Space orientation="vertical" size={4} className="workforce-event">
                    <Flex wrap gap={8} align="center">
                      <Text strong>对话同步</Text>
                      <Tag>{row.events.length} 项</Tag>
                      <Text type="secondary">{new Date(row.createdAt).toLocaleString('zh-CN', { hour12: false })}</Text>
                    </Flex>
                    <Text type="secondary">{row.summary}</Text>
                    <Collapse size="small" ghost items={[{
                      key: row.key,
                      label: '技术字段',
                      children: <Input.TextArea className="workforce-payload" readOnly autoSize={{ minRows: 3, maxRows: 12 }} value={JSON.stringify(row.technicalFields, null, 2)} />,
                    }]} />
                  </Space>
                ),
              };
            }
            const event = row.event;
            return {
              color: event.type.endsWith('failed') ? 'red' : event.type.endsWith('succeeded') ? 'green' : 'blue',
              content: (
                <Space orientation="vertical" size={4} className="workforce-event">
                  <Flex wrap gap={8} align="center">
                    <Text strong>{eventLabel(event.type)}</Text>
                    <Tag>#{event.sequence}</Tag>
                    <Text type="secondary">{new Date(event.createdAt).toLocaleString('zh-CN', { hour12: false })}</Text>
                  </Flex>
                  <Text type="secondary" code>{event.type}</Text>
                  {hookEventSummary(event) && <Text>{hookEventSummary(event)}</Text>}
                  {Object.keys(event.payload ?? {}).length > 0 && (
                    <Collapse size="small" ghost items={[{
                      key: 'payload',
                      label: '技术字段',
                      children: <Input.TextArea className="workforce-payload" readOnly autoSize={{ minRows: 3, maxRows: 12 }} value={JSON.stringify(event.payload, null, 2)} />,
                    }]} />
                  )}
                </Space>
              ),
            };
          })}
        />
      ) : (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无关键执行记录" />
      )}
    </Space>
  );
}

function hookEventSummary(event: WorkforceEvent) {
  if (!event.type.startsWith('hook.')) return '';
  if (event.type === 'hook.configuration.loaded') {
    const hooks = Array.isArray(event.payload.hooks) ? event.payload.hooks : [];
    return hooks.length > 0 ? `已按顺序加载 ${hooks.length} 个 Hook` : 'Hook 配置已加载';
  }
  const name = typeof event.payload.hookName === 'string' ? event.payload.hookName : 'Hook';
  const version = typeof event.payload.packageVersion === 'string' ? ` v${event.payload.packageVersion}` : '';
  const trigger = typeof event.payload.event === 'string' ? event.payload.event : '未知触发动作';
  const reason = typeof event.payload.reasonCode === 'string' && event.payload.reasonCode ? ` · ${event.payload.reasonCode}` : '';
  const duration = typeof event.payload.durationMs === 'number' && event.payload.durationMs > 0 ? ` · ${event.payload.durationMs} ms` : '';
  return `${name}${version} · ${trigger}${reason}${duration}`;
}

function ArtifactsTab({ taskId, artifacts, loading, error, onRetry }: { taskId: string; artifacts: WorkforceArtifact[]; loading: boolean; error: boolean; onRetry: () => void }) {
  const [downloading, setDownloading] = useState<string>();
  const [downloadError, setDownloadError] = useState('');
  const download = async (artifact: WorkforceArtifact) => {
    if (downloading || error || artifact.status !== 'AVAILABLE') return;
    setDownloading(artifact.id); setDownloadError('');
    try { await downloadWorkforceArtifact(taskId, artifact); }
    catch { setDownloadError('文件下载未完成，请重试。'); }
    finally { setDownloading(undefined); }
  };
  const columns: TableColumnsType<WorkforceArtifact> = [
    {
      title: '文件',
      dataIndex: 'name',
      key: 'name',
      width: 280,
      render: (name: string) => <Space><FileTextOutlined /><Text strong style={{ overflowWrap: 'anywhere' }}>{name}</Text></Space>,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 120,
      render: (type: string) => <Tag>{type}</Tag>,
    },
    {
      title: '大小',
      dataIndex: 'sizeBytes',
      key: 'size',
      width: 100,
      render: (size: number) => formatBytes(size),
    },
    {
      title: 'SHA-256',
      dataIndex: 'sha256',
      key: 'sha256',
      width: 220,
      render: (sha256: string) => <Text code copyable>{sha256.slice(0, 16)}…</Text>,
    },
    {
      title: '操作',
      key: 'action',
      width: 110,
      align: 'right',
      render: (_, artifact) => (
        <Button icon={<CloudDownloadOutlined />} loading={downloading === artifact.id} disabled={!!downloading || artifact.status !== 'AVAILABLE'} onClick={() => void download(artifact)}>
          {artifact.status === 'AVAILABLE' ? '下载' : '暂不可下载'}
        </Button>
      ),
    },
  ];

  if (loading) return <Skeleton active paragraph={{ rows: 6 }} />;
  if (error) return <Space wrap><Text type="danger" role="alert">结果文件暂时无法读取。</Text><Button onClick={onRetry}>重试</Button></Space>;
  if (!artifacts.length) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无可下载文件" />;
  }

  return (
    <Space orientation="vertical" size={16} className="workforce-tab-content">
      {downloadError && <Text type="danger" role="alert">{downloadError}</Text>}
      <Table<WorkforceArtifact>
        rowKey="id"
        columns={columns}
        dataSource={artifacts}
        pagination={false}
        scroll={{ x: 760 }}
      />
    </Space>
  );
}

function StatusTag({ status, verbose = false }: { status: TaskStatus; verbose?: boolean }) {
  const meta = statusMeta[status];
  return <Tag color={meta.color}>{verbose ? meta.label : meta.label}</Tag>;
}

function continuationStep(status: TaskStatus) {
  if (status === 'INTERACTION_SUSPENDED') return 1;
  if (status === 'CREATED' || status === 'QUEUED' || status === 'PREPARING') return 0;
  if (status === 'RUNNING' || status === 'RETRY_WAIT') return 1;
  if (status === 'VALIDATING' || status === 'REVIEWING' || status === 'MANUAL_REVIEW') return 2;
  return 3;
}

function parseDetailTab(value: string | null): DetailTab {
  return value === 'activity' ? value : 'overview';
}

function parseWorkspaceTab(searchParams: Pick<URLSearchParams, 'get'>, location: ReturnType<typeof parseTaskDetailLocation>): WorkspaceTab {
  const value = searchParams.get('tab');
  if (value === 'workflow' || value === 'results' || value === 'details') return value;
  if (value === 'activity' || location.focus === 'continuation' || location.focus === 'delivery') return 'details';
  if (location.focus === 'evaluation' || location.focus === 'result') return 'results';
  return 'workflow';
}

function parseResultFocus(value: string | null): 'checks' | 'review' | 'result' {
  return value === 'review' || value === 'result' ? value : 'checks';
}

function repositoryName(value: string) {
  return value.replace(/\/$/, '').split('/').pop()?.replace(/\.git$/, '') || value;
}

function short(value?: string) {
  return value ? value.slice(0, 12) : '—';
}

function elapsed(task: WorkforceTask) {
  if (!task.startedAt) return task.status === 'QUEUED' ? '等待调度' : '进行中';
  const end = task.finishedAt ? new Date(task.finishedAt).getTime() : Date.now();
  const total = Math.max(0, Math.floor((end - new Date(task.startedAt).getTime()) / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return [hours, minutes, seconds].map((part) => String(part).padStart(2, '0')).join(':');
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`;
}

function eventLabel(type: string) {
  const labels: Record<string, string> = {
    'task.created': '任务已创建',
    'task.queued': '已进入调度队列',
    'attempt.preparing': '准备隔离工作区',
    'attempt.running': 'Agent 开始执行',
    'attempt.completion_proposed': '收到 Completion Proposal',
    'task.validating': '执行确定性验证',
    'result.evaluation_ready': 'Result Processor 已准备验收',
    'result.evaluation_persisted': 'Result EvaluationCycle 已持久化',
    'result.revise': '同 Session 自动修正',
    'result.pass': 'Result Processor 通过',
    'result.wait_user': '等待用户处理',
    'result.manual_review': '转人工处理',
    'task.packaging': 'Finalizer 打包制品',
    'task.uploading': '上传验收产物',
    'task.succeeded': '任务完成',
    'task.failed': '任务失败',
    'task.cancelled': '任务已取消',
    'hook.configuration.loaded': 'Agent Hooks 已加载',
    'hook.started': 'Hook 开始执行',
    'hook.allow': 'Hook 已放行',
    'hook.deny': 'Hook 已拦截',
    'hook.failed': 'Hook 执行失败',
  };
  return labels[type] ?? type.replaceAll('.', ' / ');
}
