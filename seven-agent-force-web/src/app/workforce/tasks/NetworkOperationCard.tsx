import { CloudOutlined, FolderOutlined, SafetyCertificateOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Flex, Popconfirm, Space, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';
import { decideExecutionOperation, getExecutionOperation } from '@/api/executionAccessController';
import { executionAccessErrorCode, executionAccessFeedback, type ExecutionOperationSummary, type OperationDecisionAction } from '@/api/executionAccessContract';

const statusView: Record<ExecutionOperationSummary['status'], { label: string; color: string }> = {
  WAITING_APPROVAL: { label: '等待你决定', color: 'gold' },
  READY: { label: '已允许', color: 'blue' },
  DISPATCHED: { label: '正在准备执行环境', color: 'processing' },
  RUNNING: { label: '正在运行', color: 'processing' },
  SUCCEEDED: { label: '操作已完成', color: 'success' },
  FAILED: { label: '操作失败', color: 'error' },
  OUTCOME_UNKNOWN: { label: '执行结果未知', color: 'warning' },
  DENIED: { label: '已拒绝', color: 'default' },
  CANCELLED: { label: '已取消', color: 'default' },
  EXPIRED: { label: '已过期', color: 'default' },
};
const operationSubmissionKeys = new Map<string, string>();

function deliveryLabel(item: ExecutionOperationSummary): string | undefined {
  if (item.agentContinued) return 'Agent 已继续';
  if (item.resultDeliveryStatus === 'NATIVE_CALL_RESOLVED') return '结果已返回 Agent';
  if (item.resultDeliveryStatus === 'DELIVERED_TO_ADAPTER') return '结果已送达 Adapter';
  if (item.resultDeliveryStatus === 'SAVED') return '结果已保存，等待发送';
}

export default function NetworkOperationCard({ taskId, item, disabled, onChanged, onBusyChange }: { taskId: string; item: ExecutionOperationSummary; disabled: boolean; onBusyChange?: (busy: boolean) => void; onChanged: () => Promise<void> }) {
  const client = useQueryClient();
  const primary = useRef<HTMLButtonElement>(null);
  const pending = useRef(false);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [error, setError] = useState('');
  const detail = useQuery({
    queryKey: ['workforce', 'execution-operation', taskId, item.operationId, item.version],
    queryFn: ({ signal }) => getExecutionOperation(taskId, item.operationId, signal),
    enabled: detailsOpen,
    retry: false,
  });
  const mutation = useMutation({
    onMutate: () => onBusyChange?.(true),
    onSettled: () => { pending.current = false; onBusyChange?.(false); },
    mutationFn: (action: OperationDecisionAction) => {
      const fingerprint = `${taskId}:${item.operationId}:${item.version}:${action}`;
      let idempotencyKey = operationSubmissionKeys.get(fingerprint);
      if (!idempotencyKey) { idempotencyKey = crypto.randomUUID(); operationSubmissionKeys.set(fingerprint, idempotencyKey); }
      return decideExecutionOperation(taskId, item.operationId, { expectedVersion: item.version, action, reason: '', idempotencyKey });
    },
    onSuccess: async () => {
      setError('');
      for (const action of ['ALLOW_ONCE', 'ALLOW_FOR_TASK', 'DENY'] as const) operationSubmissionKeys.delete(`${taskId}:${item.operationId}:${item.version}:${action}`);
      await Promise.all([onChanged(), client.invalidateQueries({ queryKey: ['workforce', 'network-grants', taskId] })]);
      primary.current?.focus();
    },
    onError: async (cause) => {
      setError(executionAccessFeedback(cause, '访问决定'));
      if (['EXECUTION_OPERATION_STALE', 'EXECUTION_OPERATION_ACTION_NOT_ALLOWED', 'EXECUTION_OPERATION_NOT_FOUND'].includes(executionAccessErrorCode(cause))) await onChanged().catch(() => undefined);
      primary.current?.focus();
    },
  });
  const state = statusView[item.status];
  const decide = (action: OperationDecisionAction) => { if (!disabled && item.status === 'WAITING_APPROVAL' && !pending.current) { pending.current = true; mutation.mutate(action); } };
  return <>
    <div className="workforce-operation-heading">
      <Space size={8}><CloudOutlined aria-hidden /><Typography.Text strong>{item.status === 'WAITING_APPROVAL' ? '需要访问外部服务' : '外部服务访问'}</Typography.Text></Space>
      <Tag color={state.color}>{state.label}</Tag>
    </div>
    <div className="workforce-operation-facts">
      <div className="workforce-operation-fact"><span>用途</span><strong>{item.purpose}</strong></div>
      <div className="workforce-operation-fact"><span>外部服务</span><Flex wrap gap={6} className="workforce-operation-targets" aria-label="请求访问的服务">{item.targets.map((target) => <Tag key={`${target.protocol}:${target.host}:${target.port}`}>{target.host}:{target.port}</Tag>)}</Flex></div>
      <div className="workforce-operation-fact"><span>作用范围</span><strong><SafetyCertificateOutlined /> {item.decisionAction === 'ALLOW_FOR_TASK' ? '当前任务' : '这次操作'}</strong></div>
    </div>
    {item.status === 'WAITING_APPROVAL' && <Typography.Text type="secondary" className="workforce-operation-disclosure">该命令可以向上述服务发送工作区中的数据。</Typography.Text>}
    {item.result && <div className={`workforce-operation-result is-${item.status.toLowerCase()}`} role="status">
      <Typography.Text strong>{item.result.summary || state.label}</Typography.Text>
      {item.result.exitCode !== undefined && <Typography.Text type="secondary">退出码 {item.result.exitCode}</Typography.Text>}
    </div>}
    {deliveryLabel(item) && <Typography.Text className="workforce-operation-delivery" type="secondary">{deliveryLabel(item)}</Typography.Text>}
    <details className="workforce-operation-details" onToggle={(event) => setDetailsOpen(event.currentTarget.open)}>
      <summary>查看执行内容</summary>
      {detail.isLoading && <Typography.Text type="secondary">正在加载…</Typography.Text>}
      {detail.isError && <Alert type="warning" title={executionAccessFeedback(detail.error, '执行内容加载')} action={<Button size="small" onClick={() => void detail.refetch()}>重试</Button>} />}
      {detail.data && <div className="workforce-operation-command">
        <Typography.Text type="secondary"><FolderOutlined /> 工作目录</Typography.Text><Typography.Text code>{detail.data.cwd}</Typography.Text>
        <Typography.Text type="secondary">命令</Typography.Text><pre>{detail.data.command}</pre>
      </div>}
    </details>
    {error && <Alert className="workforce-operation-error" type="error" showIcon title={error} role="alert" />}
    {item.status === 'WAITING_APPROVAL' && <Flex wrap gap={8} className="workforce-operation-actions">
      <Button ref={primary} type="primary" loading={mutation.isPending && mutation.variables === 'ALLOW_ONCE'} disabled={disabled || mutation.isPending} onClick={() => decide('ALLOW_ONCE')}>允许这次</Button>
      <Popconfirm title="本任务后续也可访问这些服务？" description="仅覆盖上方列出的服务；任务结束后自动失效。" okText="本任务允许" cancelText="取消" onConfirm={() => decide('ALLOW_FOR_TASK')}>
        <Button icon={<SafetyCertificateOutlined />} loading={mutation.isPending && mutation.variables === 'ALLOW_FOR_TASK'} disabled={disabled || mutation.isPending}>本任务允许</Button>
      </Popconfirm>
      <Popconfirm title="拒绝这次访问？" description="拒绝结果会返回 Agent，由它选择其他方案。" okText="拒绝" cancelText="返回" onConfirm={() => decide('DENY')}>
        <Button type="text" danger loading={mutation.isPending && mutation.variables === 'DENY'} disabled={disabled || mutation.isPending}>拒绝</Button>
      </Popconfirm>
    </Flex>}
  </>;
}
