'use client';

import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, App, Button, Card, Descriptions, Drawer, Empty, Flex, Form, Input, Modal, Popconfirm, Select, Skeleton, Space, Tag, Typography } from 'antd';
import { useState } from 'react';
import {
  changeNotificationGroupRoute,
  deleteNotificationGroupRoute,
  getNotificationGroupRouteImpact,
  listFeishuGroupCandidates,
  listNotificationGroupRoutes,
  saveNotificationGroupRoute,
  type NotificationGroupRoute,
} from '@/api/notificationController';
import { listEnterpriseConnections } from '@/api/externalLoginController';

const { Text, Title } = Typography;

type RouteForm = {
  name: string;
  transport: 'FEISHU_APP_CHAT' | 'FEISHU_WEBHOOK' | 'WECOM_WEBHOOK';
  requiredPermissionCode: string;
  webhookUrl?: string;
  webhookSigningSecret?: string;
  enterpriseConnectionId?: API.Int64;
  candidateToken?: string;
};

const statusText: Record<NotificationGroupRoute['status'], string> = {
  DRAFT: '待测试', VERIFIED: '测试通过', ENABLED: '已启用', DISABLED: '已停用', RETIRED: '已退役',
};

const probeFailureText: Record<string, string> = {
  FEISHU_GROUP_UNAVAILABLE: '飞书没有接受群消息。请确认应用机器人已加入该群，且该群仍在应用可见范围内。',
  GROUP_TARGET_INVALID: '群目标已失效，请删除此通知群后重新选择应用可见群。',
  CONNECTION_AUTHENTICATION_FAILED: '飞书应用凭证或发送权限不可用，请检查企业连接并发布最新应用版本。',
  PROVIDER_THROTTLED: '飞书暂时限制了请求频率，请稍后再试。',
  PROVIDER_OUTCOME_UNKNOWN: '飞书请求结果不确定，请先检查群内是否收到测试消息，避免立即重复发送。',
  GROUP_TARGET_DECRYPT_FAILED: '平台无法读取已保存的群目标，请删除此通知群后重新创建。',
  PROBE_REQUEST_FAILED: '平台未能发起群消息测试，请检查企业连接状态后重试。',
  PROBE_FAILED: '测试消息未被发送方接受，请检查企业连接、机器人入群状态和应用权限。',
};

export function GroupRouteWorkspace() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<NotificationGroupRoute | null | undefined>();
  const [impact, setImpact] = useState<{ route: NotificationGroupRoute; text: string; canDelete: boolean }>();
  const [form] = Form.useForm<RouteForm>();
  const transport = Form.useWatch('transport', form);
  const connectionId = Form.useWatch('enterpriseConnectionId', form);
  const routes = useQuery({ queryKey: ['notification', 'group-routes'], queryFn: listNotificationGroupRoutes });
  const connections = useQuery({ queryKey: ['enterprise-connections'], queryFn: listEnterpriseConnections, enabled: editing !== undefined && transport === 'FEISHU_APP_CHAT' });
  const candidates = useQuery({ queryKey: ['notification', 'feishu-group-candidates', connectionId], queryFn: () => listFeishuGroupCandidates(connectionId!), enabled: editing !== undefined && transport === 'FEISHU_APP_CHAT' && Boolean(connectionId) });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ['notification', 'group-routes'] });
  const save = useMutation({
    mutationFn: (values: RouteForm) => saveNotificationGroupRoute({
      ...values,
      id: editing?.id,
      expectedVersion: editing?.version,
      provider: values.transport === 'WECOM_WEBHOOK' ? 'WECOM' : 'FEISHU',
    }),
    onSuccess: async () => { message.success('通知群已保存，请先测试再启用'); setEditing(undefined); await refresh(); },
  });
  const change = useMutation({
    mutationFn: ({ route, action }: { route: NotificationGroupRoute; action: 'probe' | 'enable' | 'disable' | 'retire' }) => changeNotificationGroupRoute(route, action),
    onSuccess: async (result, variables) => {
      if (variables.action === 'probe') {
        if (result.lastProbeStatus === 'PASSED') message.success('测试消息已被发送方接受，请到群内确认');
        else message.error(probeFailureText[result.lastProbeReasonCode ?? ''] ?? probeFailureText.PROBE_FAILED);
      } else {
        message.success('通知群状态已更新');
      }
      await refresh();
    },
  });
  const remove = useMutation({ mutationFn: deleteNotificationGroupRoute, onSuccess: async () => { message.success('未使用的通知群已删除'); setImpact(undefined); await refresh(); } });
  const open = (route?: NotificationGroupRoute) => {
    setEditing(route ?? null);
    form.setFieldsValue(route ? {
      name: route.name,
      transport: route.transport,
      requiredPermissionCode: route.requiredPermissionCode,
    } : { transport: 'FEISHU_WEBHOOK', requiredPermissionCode: 'workforce:task:create' });
  };
  const inspect = async (route: NotificationGroupRoute) => {
    const value = await getNotificationGroupRouteImpact(route.id);
    setImpact({ route, canDelete: value.canDelete, text: `关联订阅 ${value.subscriptions} 个，待处理消息 ${value.pendingRecords} 条，历史引用 ${value.historicalReferences} 条。` });
  };

  const candidateErrorMessage = candidates.error instanceof Error
    ? candidates.error.message
    : '飞书应用可见群暂时无法加载，请稍后重试';
  const candidateErrorPayload = (candidates.error as (Error & { payload?: { data?: unknown } }) | null)?.payload?.data;
  const candidateManagementUrl = candidateErrorPayload
    && typeof candidateErrorPayload === 'object'
    && typeof (candidateErrorPayload as { managementUrl?: unknown }).managementUrl === 'string'
    ? (candidateErrorPayload as { managementUrl: string }).managementUrl
    : 'https://open.feishu.cn/app';

  if (routes.isLoading) return <Skeleton active paragraph={{ rows: 5 }} />;
  if (routes.isError) return <Alert type="warning" showIcon title="通知群暂时无法加载" action={<Button onClick={() => void routes.refetch()}>重试</Button>} />;
  const items = routes.data ?? [];
  return (
    <Space orientation="vertical" size={20} style={{ width: '100%' }}>
      <Flex justify="space-between" align="flex-start" gap={16} wrap>
        <div><Title level={4} style={{ margin: 0 }}>组织通知群</Title><Text type="secondary">管理员批准后，成员可以在创建任务时选择这些群。</Text></div>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => open()}>新建通知群</Button>
      </Flex>
      {items.length ? items.map((route) => (
        <Card key={route.id} size="small">
          <Flex justify="space-between" align="center" gap={16} wrap>
            <div style={{ minWidth: 220 }}><Space wrap><Text strong>{route.name}</Text><Tag color={route.status === 'ENABLED' ? 'green' : route.status === 'VERIFIED' ? 'blue' : 'default'}>{statusText[route.status]}</Tag></Space><div><Text type="secondary">{route.provider === 'FEISHU' ? '飞书' : '企业微信'} · 群机器人</Text></div>{route.lastProbeStatus === 'FAILED' && <div><Text type="danger">{probeFailureText[route.lastProbeReasonCode ?? ''] ?? probeFailureText.PROBE_FAILED}</Text></div>}</div>
            <Space wrap>
              <Button onClick={() => change.mutate({ route, action: 'probe' })}>测试连接</Button>
              {route.status === 'VERIFIED' && <Button type="primary" onClick={() => change.mutate({ route, action: 'enable' })}>启用</Button>}
              {route.status === 'ENABLED' && <Button onClick={() => change.mutate({ route, action: 'disable' })}>停用</Button>}
              <Button onClick={() => open(route)}>管理</Button>
              <Button danger icon={<DeleteOutlined />} onClick={() => void inspect(route)}>影响与删除</Button>
            </Space>
          </Flex>
        </Card>
      )) : <Empty description="还没有组织通知群" />}

      <Drawer title={editing?.id ? '管理通知群' : '新建通知群'} size="large" open={editing !== undefined} onClose={() => setEditing(undefined)} extra={<Button type="primary" loading={save.isPending} onClick={() => form.submit()}>保存</Button>} destroyOnHidden>
        <Form form={form} layout="vertical" onFinish={(values) => save.mutate(values)}>
          <Alert type="info" showIcon title="群地址和签名密钥写入后不会回显" style={{ marginBottom: 20 }} />
          <Form.Item name="name" label="名称" rules={[{ required: true }, { max: 120 }]}><Input placeholder="例如：研发通知群" /></Form.Item>
          <Form.Item name="transport" label="发送方式" rules={[{ required: true }]}><Select disabled={Boolean(editing?.id)} options={[{ value: 'FEISHU_APP_CHAT', label: '飞书应用群' }, { value: 'FEISHU_WEBHOOK', label: '飞书群机器人' }, { value: 'WECOM_WEBHOOK', label: '企业微信群机器人' }]} /></Form.Item>
          {transport === 'FEISHU_APP_CHAT' ? <>
            <Form.Item name="enterpriseConnectionId" label="飞书应用连接" rules={[{ required: true }]}><Select loading={connections.isLoading} options={(connections.data ?? []).filter((item) => item.provider === 'FEISHU' && item.status === 'ACTIVE').map((item) => ({ value: item.id, label: item.displayName }))} /></Form.Item>
            <Form.Item name="candidateToken" label="选择应用可见群" rules={[{ required: true }]} extra="群标识只在服务端加密保存，页面不会显示。">
              <Select
                loading={candidates.isFetching}
                status={candidates.isError ? 'error' : undefined}
                notFoundContent={candidates.isError ? '加载失败，请查看下方提示' : '未发现应用可见群'}
                options={(candidates.data ?? []).map((item) => ({ value: item.candidateToken, label: item.description ? `${item.displayName} · ${item.description}` : item.displayName }))}
              />
            </Form.Item>
            {candidates.isError && <Alert
              type="error"
              showIcon
              title="飞书群加载失败"
              description={<Space orientation="vertical" size={4}>
                <Text>{candidateErrorMessage}</Text>
                <Text type="secondary">如果提示权限不足，请开通 im:chat:readonly，发布应用新版本后再重新加载。</Text>
              </Space>}
              action={<Space wrap>
                <Button size="small" onClick={() => void candidates.refetch()}>重新加载</Button>
                <Button size="small" href={candidateManagementUrl} target="_blank" rel="noreferrer">打开飞书权限管理</Button>
              </Space>}
              style={{ marginBottom: 20 }}
            />}
            {!candidates.isFetching && !candidates.isError && connectionId && (candidates.data?.length ?? 0) === 0 && <Alert
              type="info"
              showIcon
              title="未发现应用可见群"
              description="请先在飞书中将应用机器人加入目标群，再重新加载。"
              action={<Button size="small" onClick={() => void candidates.refetch()}>重新加载</Button>}
              style={{ marginBottom: 20 }}
            />}
          </> : <>
            <Form.Item name="webhookUrl" label={editing?.id ? '新的群机器人地址（仅轮换时填写）' : '群机器人地址'} rules={editing?.id ? [] : [{ required: true }]}><Input.Password autoComplete="new-password" /></Form.Item>
            <Form.Item name="webhookSigningSecret" label="签名密钥（可选）"><Input.Password autoComplete="new-password" /></Form.Item>
          </>}
          <Form.Item name="requiredPermissionCode" label="谁可以使用" extra="拥有该权限的成员才能在创建任务时看到此通知群。" rules={[{ required: true }]}><Select options={[{ value: 'workforce:task:create', label: '可以创建任务的成员' }]} /></Form.Item>
          {save.isError && <Alert type="error" showIcon title="保存失败" description={save.error instanceof Error ? save.error.message : '请稍后重试'} />}
        </Form>
      </Drawer>

      <Modal title="通知群影响" open={Boolean(impact)} onCancel={() => setImpact(undefined)} footer={impact ? [<Button key="cancel" onClick={() => setImpact(undefined)}>关闭</Button>, impact.canDelete ? <Popconfirm key="delete" title="确定删除这个未使用的通知群？" onConfirm={() => remove.mutate(impact.route.id)}><Button danger loading={remove.isPending}>删除</Button></Popconfirm> : <Button key="retire" danger onClick={() => change.mutate({ route: impact.route, action: 'retire' })}>退役</Button>] : []}>
        {impact && <Descriptions column={1} items={[{ key: 'route', label: '通知群', children: impact.route.name }, { key: 'impact', label: '影响', children: impact.text }, { key: 'action', label: '可用操作', children: impact.canDelete ? '没有引用，可以删除。' : '已有历史引用，只能停用或退役。' }]} />}
      </Modal>
    </Space>
  );
}
