'use client';

import React from 'react';
import { Alert, Button, Card, Skeleton, Switch, Typography, message } from 'antd';
import { BellOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listMyWorkNotificationPreferences,
  getNotificationReadiness,
  updateMyWorkNotificationPreference,
  type WorkNotificationChannelKind,
  type WorkNotificationEventFamily,
} from '@/api/notificationController';

const events: Array<{ key: WorkNotificationEventFamily; label: string; help: string }> = [
  { key: 'TASK_ACTION_REQUIRED', label: '需要我处理', help: '任务需要补充信息、确认或交付处理时通知。' },
  { key: 'TASK_INTERRUPTED', label: '执行中断', help: '任务最终无法自动恢复时通知。' },
  { key: 'TASK_COMPLETED', label: '任务完成', help: '当前轮结果可信保存后通知。' },
];
const channels: Array<{ key: WorkNotificationChannelKind; label: string }> = [
  { key: 'IN_APP', label: '站内信' }, { key: 'EMAIL_USER', label: '邮箱' },
  { key: 'FEISHU_DIRECT', label: '飞书' }, { key: 'WECOM_DIRECT', label: '企业微信' },
];

export default function WorkNotificationsCard() {
  const queryClient = useQueryClient();
  const query = useQuery({ queryKey: ['notification', 'work-preferences'], queryFn: listMyWorkNotificationPreferences });
  const readiness = useQuery({ queryKey: ['notification', 'readiness', 'workforce_task_action_required'], queryFn: () => getNotificationReadiness('workforce_task_action_required') });
  const mutation = useMutation({
    mutationFn: ({ event, channel, enabled, version }: { event: WorkNotificationEventFamily; channel: WorkNotificationChannelKind; enabled: boolean; version: API.Int64 }) => updateMyWorkNotificationPreference(event, channel, enabled, version),
    onSuccess: async () => { message.success('工作通知设置已保存'); await queryClient.invalidateQueries({ queryKey: ['notification', 'work-preferences'] }); },
    onError: () => message.error('设置已在其他位置更新，请重新加载后再试'),
  });
  if (query.isLoading) return <Card title="工作通知"><Skeleton active paragraph={{ rows: 4 }} /></Card>;
  const rows = query.data ?? [];
  const current = (event: WorkNotificationEventFamily, channel: WorkNotificationChannelKind) => rows.find((item) => item.eventFamily === event && item.channelKind === channel);
  const available = (channel: WorkNotificationChannelKind) => {
    if (channel === 'IN_APP') return readiness.data?.inApp.available !== false;
    if (channel === 'EMAIL_USER') return readiness.data?.email.recipientReady === true;
    if (channel === 'FEISHU_DIRECT') return readiness.data?.feishu.directReady === true;
    return readiness.data?.wecom.directReady === true;
  };
  return <Card title={<span><BellOutlined style={{ color: '#1677ff', marginRight: 8 }} />工作通知</span>} variant="borderless" className="shadow-sm rounded-xl">
    <Typography.Paragraph type="secondary">选择个人渠道接收任务变化。任务明确选择的组织通知群不受这里影响。</Typography.Paragraph>
    {query.isError && <Alert type="error" showIcon message="暂时无法加载工作通知设置" style={{ marginBottom: 16 }} />}
    <div style={{ display: 'grid', gap: 12 }}>
      {events.map((event) => <div key={event.key} style={{ border: '1px solid #f0f0f0', borderRadius: 10, padding: 16 }}>
        <div style={{ marginBottom: 12 }}><Typography.Text strong>{event.label}</Typography.Text><br /><Typography.Text type="secondary">{event.help}</Typography.Text></div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(120px,1fr))', gap: 12 }}>
          {channels.map((channel) => { const item = current(event.key, channel.key); const enabled = item?.enabled ?? true; const ready = available(channel.key); return <label key={channel.key} style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}><span>{channel.label}{!ready && <Typography.Text type="secondary"> · 暂不可用</Typography.Text>}</span><Switch aria-label={`${event.label}-${channel.label}`} checked={enabled} disabled={!ready} loading={mutation.isPending && mutation.variables?.event === event.key && mutation.variables?.channel === channel.key} onChange={(checked) => mutation.mutate({ event: event.key, channel: channel.key, enabled: checked, version: item?.version ?? '0' })} /></label>; })}
        </div>
      </div>)}
    </div>
    {[readiness.data?.email.recipientReady, readiness.data?.feishu.directReady, readiness.data?.wecom.directReady].some((item) => item === false) && <Button type="link" href="#enterprise-accounts" style={{ paddingInline: 0, marginTop: 12 }}>验证邮箱或绑定企业账号</Button>}
  </Card>;
}
