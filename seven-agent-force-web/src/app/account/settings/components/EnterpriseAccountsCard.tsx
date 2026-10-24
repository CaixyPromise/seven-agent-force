'use client';

import React, { useMemo, useState } from 'react';
import { Alert, Button, Card, Modal, Skeleton, Space, Switch, Tag, Typography, message } from 'antd';
import { BellOutlined, LinkOutlined, TeamOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  listMyEnterpriseIdentities,
  sendEnterpriseTestMessage,
  startEnterpriseBinding,
  unbindEnterpriseIdentity,
  updateEnterpriseNotificationSettings,
  type EnterpriseIdentityRecord,
} from '@/api/externalLoginController';
import { enterpriseBindingStartFeedback, type EnterpriseFailureFeedback } from '@/lib/enterprise-identity-feedback';

const { Text } = Typography;
type Provider = 'FEISHU' | 'WECOM';

const providers: Array<{ provider: Provider; name: string; tone: string }> = [
  { provider: 'FEISHU', name: '飞书', tone: '#3370ff' },
  { provider: 'WECOM', name: '企业微信', tone: '#07c160' },
];

function enterpriseErrorMessage(error: unknown, fallback: string) {
  const payload = (error as { payload?: { message?: unknown } } | null | undefined)?.payload;
  if (typeof payload?.message === 'string' && payload.message.trim()) return payload.message.trim();
  if (error instanceof Error && error.message.trim()) return error.message.trim();
  return fallback;
}

export default function EnterpriseAccountsCard() {
  const queryClient = useQueryClient();
  const [settings, setSettings] = useState<EnterpriseIdentityRecord | null>(null);
  const [directEnabled, setDirectEnabled] = useState(false);
  const [mentionEnabled, setMentionEnabled] = useState(false);
  const [bindFailure, setBindFailure] = useState<{ provider: Provider; feedback: EnterpriseFailureFeedback } | null>(null);
  const { data = [], isLoading } = useQuery({ queryKey: ['account-settings', 'enterprise-identities'], queryFn: listMyEnterpriseIdentities });
  const identities = useMemo(() => new Map(data.filter((item) => item.status === 'ACTIVE').map((item) => [item.provider, item])), [data]);

  const bindMutation = useMutation({
    mutationFn: startEnterpriseBinding,
    onMutate: (provider) => setBindFailure((current) => current?.provider === provider ? null : current),
    onSuccess: ({ redirectUrl }) => window.location.assign(redirectUrl),
    onError: (error: unknown, provider) => setBindFailure({ provider, feedback: enterpriseBindingStartFeedback(error) }),
  });
  const settingsMutation = useMutation({
    mutationFn: ({ item }: { item: EnterpriseIdentityRecord }) => updateEnterpriseNotificationSettings(item.id, { directEnabled, mentionEnabled, expectedIdentityVersion: item.identityVersion }),
    onSuccess: async () => { message.success('通知设置已更新'); setSettings(null); await queryClient.invalidateQueries({ queryKey: ['account-settings', 'enterprise-identities'] }); },
    onError: (error: unknown) => message.error(enterpriseErrorMessage(error, '通知设置保存失败，请稍后重试')),
  });
  const unbindMutation = useMutation({
    mutationFn: (item: EnterpriseIdentityRecord) => unbindEnterpriseIdentity(item.id, { expectedIdentityVersion: item.identityVersion, reason: '用户解除企业账号绑定' }),
    onSuccess: async () => { message.success('企业账号已解除绑定'); await queryClient.invalidateQueries({ queryKey: ['account-settings', 'enterprise-identities'] }); },
    onError: (error: unknown) => message.error(enterpriseErrorMessage(error, '暂时无法解除绑定，请稍后重试')),
  });
  const testMutation = useMutation({
    mutationFn: sendEnterpriseTestMessage,
    onSuccess: () => message.success('测试消息已发送'),
    onError: (error: unknown) => message.error(enterpriseErrorMessage(error, '测试消息发送失败，请联系管理员检查连接')),
  });

  const openSettings = (item: EnterpriseIdentityRecord) => {
    setDirectEnabled(item.directEnabled); setMentionEnabled(item.mentionEnabled); setSettings(item);
  };
  const confirmUnbind = (item: EnterpriseIdentityRecord, providerName: string) => Modal.confirm({
    title: `解除${providerName}账号？`,
    content: '解除后将停止个人消息和群内提醒。不会删除你的平台账号、任务和历史通知。',
    okText: '解除绑定', cancelText: '取消', okButtonProps: { danger: true },
    onOk: () => unbindMutation.mutateAsync(item),
  });

  return <>
    <Card title="企业账号" variant="borderless" className="shadow-sm rounded-xl">
      {isLoading ? <Skeleton active paragraph={{ rows: 3 }} /> : <div>
        {providers.map((provider, index) => {
          const identity = identities.get(provider.provider);
          return <div key={provider.provider} className={`flex items-center justify-between gap-5 py-4 max-sm:items-start max-sm:flex-col ${index ? 'border-t border-slate-100' : ''}`}>
            <div className="flex items-start gap-3 min-w-0">
              <div className="h-11 w-11 shrink-0 rounded-xl border flex items-center justify-center font-semibold" style={{ color: provider.tone, borderColor: `${provider.tone}40`, background: `${provider.tone}0d` }}>{provider.name.slice(0, 2)}</div>
              <div className="min-w-0">
                <Space wrap><span className="font-medium text-slate-900">{provider.name}</span>{identity ? <Tag color="success">已绑定</Tag> : <Tag>未绑定</Tag>}</Space>
                <div className="mt-1">{identity ? <Space wrap size={12}><Text type={identity.directEnabled ? undefined : 'secondary'}><BellOutlined /> {identity.directEnabled ? '可以接收个人消息' : '个人消息已关闭'}</Text><Text type={identity.mentionEnabled ? undefined : 'secondary'}><TeamOutlined /> {identity.mentionEnabled ? '可以在群消息中提醒我' : '群内提醒已关闭'}</Text></Space> : <Text type="secondary">绑定后可接收个人任务通知和群内提醒。</Text>}</div>
                {!identity && bindFailure?.provider === provider.provider ? <Alert className="mt-3" showIcon type="error" title={bindFailure.feedback.title} description={<Space direction="vertical" size={6}><span>{bindFailure.feedback.description}</span>{bindFailure.feedback.traceId ? <Text type="secondary" copyable={{ text: bindFailure.feedback.traceId }}>参考编号：{bindFailure.feedback.traceId}</Text> : null}</Space>} /> : null}
              </div>
            </div>
            <Space wrap className="max-sm:!w-full">{identity ? <>
              <Button loading={testMutation.isPending} onClick={() => testMutation.mutate(identity.id)}>发送测试消息</Button>
              <Button onClick={() => openSettings(identity)}>通知设置</Button>
              <Button danger loading={unbindMutation.isPending} onClick={() => confirmUnbind(identity, provider.name)}>解除绑定</Button>
            </> : <Button type="primary" icon={<LinkOutlined />} loading={bindMutation.isPending} onClick={() => bindMutation.mutate(provider.provider)}>绑定{provider.name}</Button>}</Space>
          </div>;
        })}
      </div>}
    </Card>
    <Modal title="通知设置" open={Boolean(settings)} okText="保存" cancelText="取消" confirmLoading={settingsMutation.isPending} onCancel={() => setSettings(null)} onOk={() => settings && settingsMutation.mutate({ item: settings })}>
      <div className="py-2 space-y-5">
        <div className="flex items-center justify-between gap-6"><div><div className="font-medium">接收个人消息</div><Text type="secondary">任务需要处理或完成时，发送给你的企业账号。</Text></div><Switch checked={directEnabled} onChange={setDirectEnabled} /></div>
        <div className="flex items-center justify-between gap-6"><div><div className="font-medium">在群消息中提醒我</div><Text type="secondary">群通知支持提醒个人后才会使用此设置。</Text></div><Switch checked={mentionEnabled} onChange={setMentionEnabled} /></div>
      </div>
    </Modal>
  </>;
}
