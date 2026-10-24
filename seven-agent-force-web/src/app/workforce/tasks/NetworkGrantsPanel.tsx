import { GlobalOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Card, Empty, Popconfirm, Skeleton, Space, Tag, Typography } from 'antd';
import { useRef, useState } from 'react';
import { listTaskNetworkGrants, revokeTaskNetworkGrant } from '@/api/executionAccessController';
import { executionAccessErrorCode, executionAccessFeedback, type TaskNetworkGrant } from '@/api/executionAccessContract';
import CreatorIdentity from '../components/CreatorIdentity';

const status: Record<TaskNetworkGrant['status'], { label: string; color: string }> = {
  ACTIVE: { label: '可访问', color: 'green' },
  REVOKED: { label: '已撤销', color: 'default' },
  EXPIRED: { label: '已过期', color: 'default' },
};

export default function NetworkGrantsPanel({ taskId, compact = false, onManage }: { taskId: string; compact?: boolean; onManage?: () => void }) {
  const client = useQueryClient();
  const [error, setError] = useState('');
  const keys = useRef(new Map<string, string>());
  const query = useQuery({ queryKey: ['workforce', 'network-grants', taskId], queryFn: ({ signal }) => listTaskNetworkGrants(taskId, signal), retry: false });
  const revoke = useMutation({
    mutationFn: (grant: TaskNetworkGrant) => {
      const fingerprint = `${grant.id}:${grant.version}`;
      let key = keys.current.get(fingerprint);
      if (!key) { key = crypto.randomUUID(); keys.current.set(fingerprint, key); }
      return revokeTaskNetworkGrant(taskId, grant.id, grant.version, key);
    },
    onSuccess: async (_, grant) => {
      setError('');
      keys.current.delete(`${grant.id}:${grant.version}`);
      await Promise.all([client.invalidateQueries({ queryKey: ['workforce', 'network-grants', taskId] }), client.invalidateQueries({ queryKey: ['workforce', 'conversation', taskId] })]);
    },
    onError: async (cause) => {
      setError(executionAccessFeedback(cause, '撤销访问权限'));
      if (['TASK_NETWORK_GRANT_STALE', 'TASK_NETWORK_GRANT_REVOKED', 'TASK_NETWORK_GRANT_EXPIRED'].includes(executionAccessErrorCode(cause))) await query.refetch();
    },
  });
  const visible = compact ? query.data?.slice(0, 2) : query.data;
  return <Card className={`workforce-network-grants${compact ? ' is-compact workforce-rail-card' : ''}`} title={compact ? '本次权限' : <Space><GlobalOutlined />本任务可访问的服务</Space>} extra={compact && onManage ? <Button type="link" size="small" onClick={onManage}>管理</Button> : undefined}>
    {query.isLoading ? <Skeleton active paragraph={{ rows: 2 }} /> : query.isError ? <Alert type="warning" showIcon title={executionAccessFeedback(query.error, '访问权限加载')} action={<Button size="small" onClick={() => void query.refetch()}>重试</Button>} /> : !query.data?.length ? compact ? <div className="workforce-rail-count">0</div> : <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有任务范围访问权限" /> : <Space orientation="vertical" size={12} className="workforce-network-grant-list">
      {visible?.map((grant) => <div key={grant.id} className="workforce-network-grant">
        <div className="workforce-network-grant-main">
          <Space wrap size={[6, 6]}>{grant.targets.map((target) => <Tag key={`${target.protocol}:${target.host}:${target.port}`}>{target.protocol.toUpperCase()} · {target.host}:{target.port}</Tag>)}</Space>
          {grant.purpose && <Typography.Text type="secondary">用途：{grant.purpose}</Typography.Text>}
          {grant.actorUserId && <Typography.Text type="secondary"><Space size={4}>决定人：<CreatorIdentity userId={grant.actorUserId} /> · {formatGrantTime(grant.createdAt)}</Space></Typography.Text>}
          <Typography.Text type="secondary">{grant.expiresAt ? `有效至 ${formatGrantTime(grant.expiresAt)}` : '任务结束时失效'}</Typography.Text>
          {grant.lastOperation && <Typography.Text className="workforce-network-grant-result" type="secondary">最近结果：{operationResult(grant.lastOperation)}</Typography.Text>}
        </div>
        <Space><Tag color={status[grant.status].color}>{status[grant.status].label}</Tag>{!compact && grant.status === 'ACTIVE' && <Popconfirm title="撤销本任务的访问权限？" description="新的访问会被阻止，正在使用的连接也会停止。" okText="撤销" cancelText="保留" onConfirm={() => revoke.mutate(grant)}><Button danger size="small" loading={revoke.isPending && revoke.variables?.id === grant.id}>撤销</Button></Popconfirm>}</Space>
      </div>)}
    </Space>}
    {error && <Alert className="workforce-operation-error" type="error" showIcon title={error} role="alert" />}
  </Card>;
}

function formatGrantTime(value: string): string {
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }).format(new Date(value));
}

function operationResult(value: NonNullable<TaskNetworkGrant['lastOperation']>): string {
  const statusLabel = ({ SUCCEEDED: '成功', FAILED: '失败', OUTCOME_UNKNOWN: '待确认', DENIED: '已拒绝', CANCELLED: '已取消' } as Record<string, string>)[value.status] ?? value.status;
  const delivered = value.agentContinued ? 'Agent 已继续' : value.resultDeliveryStatus === 'NATIVE_CALL_RESOLVED' ? '已返回 Agent' : '结果已保存';
  return `${statusLabel}${value.exitCode !== undefined ? ` · 退出码 ${value.exitCode}` : ''} · ${delivered}`;
}
