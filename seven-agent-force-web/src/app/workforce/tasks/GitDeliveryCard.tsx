import { BranchesOutlined, CheckCircleOutlined, CloudUploadOutlined } from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import { Alert, Button, Collapse, Flex, Popconfirm, Space, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';
import { decideTaskDelivery, restartTaskDelivery } from '@/api/deliveryController';
import { deliveryErrorCode, deliveryFeedback, type DeliveryConversationCard as DeliveryCard, type DeliveryDecisionAction } from '@/api/deliveryContract';

const states: Record<DeliveryCard['status'], { label: string; color: string }> = {
  SCHEDULED: { label: 'Agent 已准备交付', color: 'blue' },
  WAITING_FINALIZATION: { label: '等待任务结果冻结', color: 'blue' },
  WAITING_CHECKS: { label: '等待完成检查', color: 'cyan' },
  WAITING_REVIEW: { label: '等待独立审查', color: 'cyan' },
  WAITING_USER_DECISION: { label: '等待你决定', color: 'gold' },
  AUTO_DELIVERY_SCHEDULED: { label: '已安排自动交付', color: 'blue' },
  PREPARING: { label: '正在准备交付', color: 'processing' },
  PUSHING: { label: '正在推送分支', color: 'processing' },
  VERIFYING_BRANCH: { label: '正在核验远端分支', color: 'processing' },
  CREATING_REVIEW: { label: '正在创建草稿评审', color: 'processing' },
  VERIFYING_REVIEW: { label: '正在核验草稿评审', color: 'processing' },
  DELIVERED: { label: '交付成功', color: 'success' },
  DELIVERY_FAILED: { label: '交付失败', color: 'error' },
  OUTCOME_UNKNOWN: { label: '交付结果待确认', color: 'warning' },
  SKIPPED: { label: '本次不交付', color: 'default' },
  CANCELLED: { label: '已取消', color: 'default' },
};
const keys = new Map<string, string>();

function submissionKey(fingerprint: string) {
  const current = keys.get(fingerprint);
  if (current) return current;
  const created = crypto.randomUUID(); keys.set(fingerprint, created); return created;
}

export default function GitDeliveryCard({ taskId, item, disabled, pageActions, onChanged, onDetails, onBusyChange }: { taskId: string; item: DeliveryCard; disabled: boolean; pageActions: string[]; onBusyChange?: (busy: boolean) => void; onChanged: () => Promise<void>; onDetails: () => void }) {
  const primary = useRef<HTMLButtonElement>(null);
  const pending = useRef(false);
  const [error, setError] = useState('');
  const mutation = useMutation({
    onMutate: () => onBusyChange?.(true),
    onSettled: () => { pending.current = false; onBusyChange?.(false); },
    mutationFn: async (action: DeliveryDecisionAction | 'RESTART_DELIVERY') => {
      const fingerprint = `${taskId}:${item.requestId ?? item.intentId}:${item.version}:${action}`;
      const idempotencyKey = submissionKey(fingerprint);
      if (action === 'RESTART_DELIVERY') {
        if (!item.resultCommit) throw new Error('DELIVERY_CONTRACT_INVALID');
        return restartTaskDelivery(taskId, { resultCommit: item.resultCommit, idempotencyKey });
      }
      if (!item.requestId) throw new Error('DELIVERY_CONTRACT_INVALID');
		if (!item.requestVersion) throw new Error('DELIVERY_CONTRACT_INVALID');
		return decideTaskDelivery(taskId, item.requestId, { expectedVersion: item.requestVersion, action, reason: '', idempotencyKey });
    },
    onSuccess: async (_, action) => {
      setError(''); keys.delete(`${taskId}:${item.requestId ?? item.intentId}:${item.version}:${action}`);
      await onChanged(); primary.current?.focus();
    },
    onError: async (cause) => {
      setError(deliveryFeedback(cause));
      if (['DELIVERY_REQUEST_STALE', 'DELIVERY_REQUEST_NOT_FOUND', 'DELIVERY_DECISION_ACTION_NOT_ALLOWED', 'DELIVERY_RESULT_NOT_CURRENT'].includes(deliveryErrorCode(cause))) await onChanged().catch(() => undefined);
      primary.current?.focus();
    },
  });
  const state = states[item.status];
  const can = (action: DeliveryCard['allowedActions'][number]) => item.allowedActions.includes(action) && pageActions.includes(action === 'RESTART_DELIVERY' ? 'RESTART_DELIVERY' : 'DECIDE_DELIVERY') && !disabled && !mutation.isPending;
  const decide = (action: DeliveryDecisionAction | 'RESTART_DELIVERY') => {
    if (!pending.current && can(action)) { pending.current = true; mutation.mutate(action); }
  };
  return <>
    <Flex className="workforce-git-delivery-heading" align="center" justify="space-between" gap={12} wrap>
      <Space size={8}><CloudUploadOutlined aria-hidden /><Typography.Text strong>Git 交付</Typography.Text></Space>
      <Tag color={state.color}>{state.label}</Tag>
    </Flex>
    <Typography.Paragraph className="workforce-git-delivery-summary">{item.summary}</Typography.Paragraph>
    <Flex className="workforce-git-delivery-target" gap={8} wrap>
      <Tag icon={<BranchesOutlined />}>{item.repositoryName}</Tag>
      <Tag>{item.destinationBranch}</Tag>
      <Tag>{item.deliveryMode === 'NEW_BRANCH_AND_DRAFT_REVIEW' ? '分支 + 草稿评审' : '结果分支'}</Tag>
      {item.deliveryCommit && <Tag>{item.deliveryCommit.slice(0, 7)}</Tag>}
      {item.remoteReadbackVerified && <Tag color="success">远端已核验</Tag>}
    </Flex>
    {item.authorization === 'AUTO_AFTER_GATES' && ['SCHEDULED', 'WAITING_FINALIZATION', 'WAITING_CHECKS', 'WAITING_REVIEW', 'AUTO_DELIVERY_SCHEDULED'].includes(item.status) && <Typography.Text type="secondary">检查和审查通过后，平台会自动交付。</Typography.Text>}
    {item.status === 'OUTCOME_UNKNOWN' && <Alert type="warning" showIcon title="远端结果尚未确认" description="请先通过交付详情核对远端状态，平台不会直接重复推送。" />}
    {item.status === 'DELIVERED' && <div className="workforce-git-delivery-result" role="status"><CheckCircleOutlined /><Typography.Text strong>交付成功</Typography.Text></div>}
    {item.pullRequestUrl && <Button type="primary" href={item.pullRequestUrl} target="_blank" rel="noreferrer">打开草稿评审</Button>}
    {(item.resultCommit || item.deliveryCommit || item.requestId || item.reasonCode) && <Collapse ghost size="small" items={[{ key: 'details', label: '查看交付信息', children: <Space orientation="vertical" size={6}>
      {item.resultCommit && <Typography.Text code copyable>{`结果 ${item.resultCommit}`}</Typography.Text>}
      {item.deliveryCommit && <Typography.Text code copyable>{`交付 ${item.deliveryCommit}`}</Typography.Text>}
      {item.reasonCode && <Typography.Text type="secondary">{deliveryFeedback({ reasonCode: item.reasonCode }, '交付')}</Typography.Text>}
      <Button type="link" onClick={onDetails} style={{ paddingInline: 0 }}>查看完整交付记录</Button>
    </Space> }]} />}
    {error && <Alert className="workforce-git-delivery-error" type="error" showIcon title={error} role="alert" />}
    {item.status === 'WAITING_USER_DECISION' && <Flex className="workforce-git-delivery-actions" gap={8} wrap>
      <Popconfirm title="交付当前结果？" description="平台会使用已冻结的仓库、分支和凭据执行，并在远端回读确认。" okText="交付" cancelText="返回" onConfirm={() => decide('DELIVER')}>
        <Button ref={primary} type="primary" loading={mutation.isPending && mutation.variables === 'DELIVER'} disabled={!can('DELIVER')}>交付</Button>
      </Popconfirm>
      <Popconfirm title="本次不交付？" description="当前结果会保留；需要时可以从这张卡重新发起交付。" okText="本次不交付" cancelText="返回" onConfirm={() => decide('SKIP_THIS_RESULT')}>
        <Button loading={mutation.isPending && mutation.variables === 'SKIP_THIS_RESULT'} disabled={!can('SKIP_THIS_RESULT')}>本次不交付</Button>
      </Popconfirm>
    </Flex>}
    {item.allowedActions.includes('RESTART_DELIVERY') && <Button ref={primary} type="primary" ghost loading={mutation.isPending} disabled={!can('RESTART_DELIVERY')} onClick={() => decide('RESTART_DELIVERY')}>重新发起交付（保留执行结果）</Button>}
  </>;
}
