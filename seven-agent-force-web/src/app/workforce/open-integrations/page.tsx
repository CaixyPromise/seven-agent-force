import { MoreOutlined, PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  App as AntdApp,
  Badge,
  Button,
  Card,
  Collapse,
  Drawer,
  Dropdown,
  Empty,
  Flex,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  Tooltip,
  Typography,
  type MenuProps,
  type TableColumnsType,
} from 'antd';
import { useMemo, useRef, useState } from 'react';
import {
  createOpenIntegration,
  listOpenIntegrations,
  listWebhookDeliveries,
  rotateOpenIntegrationAPIKey,
  rotateOpenIntegrationSigningSecret,
  setOpenIntegrationEnabled,
  type CreatedOpenIntegration,
  type OpenIntegrationApplication,
  type WebhookDelivery,
  type WebhookDeliveryStatus,
} from '@/api/openIntegrationController';
import styles from './openIntegrations.module.css';

const { Paragraph, Text } = Typography;

type RotationKind = 'api-key' | 'signing-secret';

interface RotationDisclosure {
  kind: RotationKind;
  applicationId: API.Int64;
  credentialId: API.Int64;
  credentialVersion: number;
  secret: string;
  previousCredentialId: API.Int64;
  previousValidUntil: string;
}

const deliveryMeta: Record<WebhookDeliveryStatus, { label: string; badge?: 'success' | 'processing' | 'warning' | 'error' | 'default' }> = {
  PENDING: { label: '待投递', badge: 'default' },
  SENDING: { label: '投递中', badge: 'processing' },
  RETRY_WAIT: { label: '等待重试', badge: 'warning' },
  SUSPENDED: { label: '已暂停', badge: 'default' },
  DELIVERED: { label: '已送达', badge: 'success' },
  EXHAUSTED: { label: '已耗尽', badge: 'error' },
};

export default function OpenIntegrationsPage() {
  const { message, modal } = AntdApp.useApp();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [credential, setCredential] = useState<CreatedOpenIntegration>();
  const [rotation, setRotation] = useState<RotationDisclosure>();
  const [deliveryApplication, setDeliveryApplication] = useState<OpenIntegrationApplication>();
  const deliveryTriggerRef = useRef<HTMLElement | null>(null);
  const rotationTriggerRef = useRef<HTMLElement | null>(null);
  const [form] = Form.useForm<{ name: string; webhookUrl?: string }>();

  const applications = useQuery({ queryKey: ['workforce', 'open-integrations'], queryFn: listOpenIntegrations });
  const deliveries = useQuery({
    queryKey: ['workforce', 'open-integrations', deliveryApplication?.id, 'deliveries'],
    queryFn: () => listWebhookDeliveries(deliveryApplication!.id),
    enabled: !!deliveryApplication,
    refetchInterval: deliveryApplication ? 5_000 : false,
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['workforce', 'open-integrations'] });
  const create = useMutation({
    mutationFn: createOpenIntegration,
    onSuccess: async (result) => {
      setCreateOpen(false);
      setCredential(result);
      form.resetFields();
      await refresh();
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '创建应用失败'),
  });
  const operate = useMutation({
    mutationFn: ({ application, enabled }: { application: OpenIntegrationApplication; enabled: boolean }) => setOpenIntegrationEnabled(application.id, enabled),
    onSuccess: async () => { await refresh(); },
    onError: (error) => message.error(error instanceof Error ? error.message : '应用状态更新失败'),
  });
  const rotate = useMutation({
    mutationFn: async ({ application, kind }: { application: OpenIntegrationApplication; kind: RotationKind }) => {
      if (kind === 'api-key') {
        return { kind, result: await rotateOpenIntegrationAPIKey(application.id) } as const;
      }
      return { kind, result: await rotateOpenIntegrationSigningSecret(application.id) } as const;
    },
    onSuccess: async ({ kind, result }) => {
      setRotation(kind === 'api-key' ? {
        kind,
        applicationId: result.applicationId,
        credentialId: result.credentialId,
        credentialVersion: result.credentialVersion,
        secret: result.apiKey,
        previousCredentialId: result.previousCredentialId,
        previousValidUntil: result.previousValidUntil,
      } : {
        kind,
        applicationId: result.applicationId,
        credentialId: result.signingCredentialId,
        credentialVersion: result.signingCredentialVersion,
        secret: result.signingSecret,
        previousCredentialId: result.previousCredentialId,
        previousValidUntil: result.previousValidUntil,
      });
      await refresh();
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '凭证轮换失败'),
  });

  const secretOperationLocked = create.isPending || rotate.isPending || !!credential || !!rotation;

  const confirmRotation = (application: OpenIntegrationApplication, kind: RotationKind) => {
    modal.confirm({
      title: kind === 'api-key' ? '轮换 API Key？' : '轮换签名 Secret？',
      content: kind === 'api-key'
        ? '新 Key 立即生效，旧 Key 将在 15 分钟后失效。'
        : '新 Secret 立即生效，Webhook 将在 24 小时内同时携带新旧签名。',
      okText: '轮换',
      onOk: () => rotate.mutateAsync({ application, kind }),
    });
  };

  const actionItems = (application: OpenIntegrationApplication): MenuProps['items'] => {
    const items: MenuProps['items'] = [
      { key: 'rotate-api-key', label: '轮换 API Key', disabled: secretOperationLocked },
    ];
    if (hasWebhook(application)) {
      items.push({ key: 'rotate-signing-secret', label: '轮换签名 Secret', disabled: secretOperationLocked });
    }
    items.push(
      { type: 'divider' },
      {
        key: 'status',
        label: application.status === 'ENABLED' ? '停用' : '启用',
        danger: application.status === 'ENABLED',
        disabled: operate.isPending && operate.variables?.application.id === application.id,
      },
    );
    return items;
  };

  const handleAction = (application: OpenIntegrationApplication, key: string) => {
    if (key === 'rotate-api-key') {
      confirmRotation(application, 'api-key');
      return;
    }
    if (key === 'rotate-signing-secret') {
      confirmRotation(application, 'signing-secret');
      return;
    }
    if (key === 'status') confirmStatusChange(application);
  };

  const columns: TableColumnsType<OpenIntegrationApplication> = [
    {
      title: '应用', key: 'identity', width: 250, fixed: 'left',
      render: (_, item) => (
        <Space orientation="vertical" size={1} className={styles.cellMin}>
          <Text strong ellipsis={{ tooltip: item.name }}>{item.name}</Text>
          <Text className={styles.mono} type="secondary" ellipsis={{ tooltip: item.id }}>{item.id}</Text>
        </Space>
      ),
    },
    {
      title: 'Webhook 地址', dataIndex: 'webhookUrl', width: 360,
      render: (value: string) => value
        ? <Text className={styles.url} ellipsis={{ tooltip: value }}>{value}</Text>
        : <Text type="secondary">未配置</Text>,
    },
    {
      title: '凭证', key: 'credentials', width: 210,
      render: (_, item) => (
        <Space orientation="vertical" size={1} className={styles.cellMin}>
          <Text className={styles.credentialLine} ellipsis={{ tooltip: `API v${item.apiCredential.version} · ${item.apiCredential.hint}` }}>
            API v{item.apiCredential.version} · <span className={styles.monoInline}>{item.apiCredential.hint}</span>
          </Text>
          {item.signingCredential && (
            <Text className={styles.credentialLine} type="secondary" ellipsis={{ tooltip: `签名 v${item.signingCredential.version} · ${item.signingCredential.hint}` }}>
              签名 v{item.signingCredential.version} · <span className={styles.monoInline}>{item.signingCredential.hint}</span>
            </Text>
          )}
        </Space>
      ),
    },
    {
      title: '状态', dataIndex: 'status', width: 110,
      render: (status: string) => <Tag color={status === 'ENABLED' ? 'green' : 'default'}>{status === 'ENABLED' ? '已启用' : '已停用'}</Tag>,
    },
    {
      title: '最近投递', dataIndex: 'latestDeliveryStatus', width: 130,
      render: (status: WebhookDeliveryStatus | undefined, item) => !hasWebhook(item)
        ? <Text type="secondary">未配置</Text>
        : status ? <DeliveryStatus status={status} /> : <Text type="secondary">暂无记录</Text>,
    },
    {
      title: '创建时间', dataIndex: 'createdAt', width: 180,
      render: (value: string) => formatDateTime(value),
    },
    {
      title: '操作', key: 'actions', width: 170, fixed: 'right',
      render: (_, application) => (
        <Space size={4}>
          {hasWebhook(application) && (
            <Button
              type="link"
              ref={(node) => { if (deliveryApplication?.id === application.id) deliveryTriggerRef.current = node; }}
              onClick={(event) => { deliveryTriggerRef.current = event.currentTarget; setDeliveryApplication(application); }}
            >投递记录</Button>
          )}
          <Dropdown
            trigger={['click']}
            menu={{ items: actionItems(application), onClick: ({ key }) => handleAction(application, key) }}
          >
            <Button
              type="link"
              icon={<MoreOutlined />}
              loading={rotate.isPending && rotate.variables?.application.id === application.id}
              disabled={secretOperationLocked || (operate.isPending && operate.variables?.application.id === application.id)}
              onClick={(event) => { rotationTriggerRef.current = event.currentTarget; }}
            >更多</Button>
          </Dropdown>
        </Space>
      ),
    },
  ];

  const confirmStatusChange = (application: OpenIntegrationApplication) => {
    const enabled = application.status !== 'ENABLED';
    const webhookConfigured = hasWebhook(application);
    modal.confirm({
      title: enabled ? '启用这个应用？' : '停用这个应用？',
      content: enabled
        ? webhookConfigured ? 'API Key 恢复可用，待投递事件会恢复发送。' : 'API Key 将恢复可用。'
        : webhookConfigured ? 'API Key 将立即失效，待投递事件会暂停。' : 'API Key 将立即失效。',
      okText: enabled ? '启用' : '停用',
      okButtonProps: { danger: !enabled },
      onOk: () => operate.mutateAsync({ application, enabled }),
    });
  };

  const closeDeliveries = () => {
    setDeliveryApplication(undefined);
    window.setTimeout(() => deliveryTriggerRef.current?.focus(), 0);
  };

  const closeRotation = () => {
    setRotation(undefined);
    rotate.reset();
    window.setTimeout(() => rotationTriggerRef.current?.focus(), 0);
  };

  const closeCredential = () => {
    setCredential(undefined);
    create.reset();
  };

  return (
    <div className={styles.page}>
      <Flex justify="space-between" align="center" gap={16} wrap>
        <Text type="secondary">管理外部应用和任务回调</Text>
        <Button type="primary" icon={<PlusOutlined />} disabled={secretOperationLocked} onClick={() => setCreateOpen(true)}>创建应用</Button>
      </Flex>

      <Card styles={{ body: { padding: 0 } }}>
        <Table<OpenIntegrationApplication>
          rowKey="id"
          columns={columns}
          dataSource={applications.data ?? []}
          loading={applications.isLoading}
          pagination={false}
          scroll={{ x: 1390 }}
          locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="还没有外部应用" /> }}
        />
      </Card>

      <Modal title="创建应用" open={createOpen} footer={null} destroyOnHidden onCancel={() => setCreateOpen(false)}>
        <Form form={form} layout="vertical" onFinish={(values) => create.mutate(values)}>
          <Form.Item name="name" label="应用名称" rules={[{ required: true, message: '请输入应用名称' }, { max: 120, message: '名称最多 120 个字符' }]}>
            <Input autoFocus maxLength={120} placeholder="Jira Automation" />
          </Form.Item>
          <Form.Item
            name="webhookUrl"
            label="Webhook HTTPS 地址（可选）"
            rules={[
              { max: 2048, message: '地址最多 2048 个字符' },
              { type: 'url', warningOnly: false, message: '请输入有效地址' },
              { pattern: /^https:\/\//, warningOnly: false, message: '请输入 HTTPS 地址' },
            ]}
          >
            <Input maxLength={2048} placeholder="https://example.com/seven-webhook" />
          </Form.Item>
          <Flex justify="flex-end" gap={8}>
            <Button onClick={() => setCreateOpen(false)}>取消</Button>
            <Button type="primary" htmlType="submit" loading={create.isPending}>创建</Button>
          </Flex>
        </Form>
      </Modal>

      <CredentialModal credential={credential} onClose={closeCredential} />
      <RotationCredentialModal rotation={rotation} onClose={closeRotation} />

      <Drawer
        title="投递记录"
        open={!!deliveryApplication}
        size="min(920px, 100vw)"
        onClose={closeDeliveries}
        destroyOnHidden
      >
        {deliveryApplication && (
          <Space orientation="vertical" size={16} className={styles.drawerContent}>
            <Text type="secondary" ellipsis={{ tooltip: deliveryApplication.name }}>应用：{deliveryApplication.name}</Text>
            <Table<WebhookDelivery>
              size="small"
              rowKey="deliveryId"
              columns={deliveryColumns}
              dataSource={deliveries.data ?? []}
              loading={deliveries.isLoading || deliveries.isFetching}
              pagination={false}
              scroll={{ x: 1120 }}
              locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无投递记录" /> }}
            />
          </Space>
        )}
      </Drawer>
    </div>
  );
}

function RotationCredentialModal({ rotation, onClose }: { rotation?: RotationDisclosure; onClose: () => void }) {
  const secretLabel = rotation?.kind === 'api-key' ? 'API Key' : 'Signing Secret';
  return (
    <Modal
      title={rotation?.kind === 'api-key' ? '保存新 API Key' : '保存新签名 Secret'}
      open={!!rotation}
      mask={{ closable: false }}
      destroyOnHidden
      onCancel={onClose}
      footer={<Button type="primary" onClick={onClose}>已保存，关闭</Button>}
    >
      {rotation && (
        <>
          <Text type="secondary">关闭后无法再次查看</Text>
          <div className={styles.credentialBlock}>
            <Text strong>{secretLabel}</Text>
            <Paragraph copyable={{ text: rotation.secret }} className={styles.secret}>{rotation.secret}</Paragraph>
          </div>
          <Space orientation="vertical" size={2} className={styles.rotationMeta}>
            <Text>版本 v{rotation.credentialVersion}</Text>
            <Text type="secondary">旧版本截止时间 {formatDateTime(rotation.previousValidUntil)}</Text>
          </Space>
        </>
      )}
    </Modal>
  );
}

function CredentialModal({ credential, onClose }: { credential?: CreatedOpenIntegration; onClose: () => void }) {
  const curl = useMemo(() => credential ? [
    "curl -X POST 'https://your-seven.example/api/open/workforce/v1/tasks' \\",
    `  -H 'Authorization: Bearer ${credential.apiKey}' \\`,
    "  -H 'Idempotency-Key: external-task-001' \\",
    "  -H 'Content-Type: application/json' \\",
    `  --data '${JSON.stringify({ scenarioKey: 'mvp-empty-workspace', name: '外部任务名称', instruction: '需要执行的任务说明', externalCorrelationId: 'external-task-001' })}'`,
  ].join('\n') : '', [credential]);
  const mcpConfig = useMemo(() => credential ? [
    'mcpServers:',
    '  seven-agent-force:',
    '    url: https://your-seven.example/mcp',
    '    headers:',
    '      Authorization: Bearer ${SEVEN_WORKFORCE_API_KEY}',
  ].join('\n') : '', [credential]);
  const webhookConfigured = !!credential?.application.webhookUrl;
  return (
    <Modal
      title="保存接入凭证"
      open={!!credential}
      mask={{ closable: false }}
      onCancel={onClose}
      footer={<Button type="primary" onClick={onClose}>已保存，关闭</Button>}
    >
      <Text type="secondary">关闭后无法再次查看</Text>
      <div className={styles.credentialBlock}>
        <Text strong>API Key</Text>
        <Paragraph copyable={{ text: credential?.apiKey }} className={styles.secret}>{credential?.apiKey}</Paragraph>
      </div>
      {credential?.signingSecret && (
        <div className={styles.credentialBlock}>
          <Text strong>Signing Secret</Text>
          <Paragraph copyable={{ text: credential.signingSecret }} className={styles.secret}>{credential.signingSecret}</Paragraph>
        </div>
      )}
      <Collapse
        ghost
        size="small"
        items={[webhookConfigured
          ? { key: 'curl', label: '最小 curl 示例', children: <pre className={styles.code}>{curl}</pre> }
          : { key: 'mcp', label: 'MCP 连接配置', children: <pre className={styles.code}>{mcpConfig}</pre> }]}
      />
    </Modal>
  );
}

const deliveryColumns: TableColumnsType<WebhookDelivery> = [
  { title: 'Event ID', dataIndex: 'eventId', width: 190, fixed: 'left', render: (value: string) => <MonoValue value={value} /> },
  { title: 'Task ID', dataIndex: 'taskId', width: 190, render: (value: string) => <MonoValue value={value} /> },
  { title: '事件', dataIndex: 'eventType', width: 140, render: (value: string) => <Text ellipsis={{ tooltip: value }}>{value}</Text> },
  { title: '状态', dataIndex: 'status', width: 115, render: (status: WebhookDeliveryStatus) => <DeliveryStatus status={status} /> },
  { title: '尝试', dataIndex: 'attemptCount', width: 70 },
  { title: 'HTTP', dataIndex: 'lastHttpStatus', width: 75, render: (value?: number) => value ?? '—' },
  { title: '下次重试', dataIndex: 'nextAttemptAt', width: 180, render: (value?: string, item?: WebhookDelivery) => item?.status === 'RETRY_WAIT' && value ? formatDateTime(value) : '—' },
  { title: '更新时间', dataIndex: 'updatedAt', width: 180, fixed: 'right', render: (value: string) => formatDateTime(value) },
];

function DeliveryStatus({ status }: { status: WebhookDeliveryStatus }) {
  const meta = deliveryMeta[status] ?? { label: status, badge: 'default' as const };
  return <Badge status={meta.badge} text={meta.label} />;
}

function MonoValue({ value }: { value: string }) {
  return <Tooltip title={value}><Text className={styles.mono} ellipsis>{value}</Text></Tooltip>;
}

function formatDateTime(value?: string) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
}

function hasWebhook(application: OpenIntegrationApplication) {
  return application.webhookUrl.trim().length > 0;
}
