'use client';

import React, { useCallback, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Result,
  Select,
  Space,
  Tabs,
  Tag,
  Typography,
  message,
} from 'antd';
import { EditableProTable, ProCard, ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ApiOutlined,
  ClockCircleOutlined,
  EditOutlined,
  FileTextOutlined,
  PlusOutlined,
  SendOutlined,
  TeamOutlined,
  SlidersOutlined,
} from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import {
  listNotificationChannels,
  saveNotificationChannel,
  testEnterpriseConnection,
  testStaticConnection,
  type NotificationChannel,
  type NotificationChannelType,
  type HTTPConnectorConfig,
  type WebhookProfileConfig,
  type ProviderParameterDescriptor,
  type ProviderParameterSetting,
} from '@/api/notificationController';
import { usePermissionFlags } from '@/hooks/auth';
import { NOTIFICATION_PERMISSIONS } from '@/lib/auth/permissionCodes';
import {
  enterpriseConnectionTestFeedback,
  staticConnectionTestFeedback,
  type EnterpriseConnectionFeedback,
} from '@/components/notification/enterpriseConnectionFeedback';
import { VersionedTemplateWorkspace } from '@/components/notification/VersionedTemplateWorkspace';
import { VersionedSceneWorkspace } from '@/components/notification/VersionedSceneWorkspace';
import { DeliveryDiagnosticsWorkspace } from '@/components/notification/DeliveryDiagnosticsWorkspace';
import { GroupRouteWorkspace } from '@/components/notification/GroupRouteWorkspace';

const CHANNEL_TYPE_OPTIONS: { value: NotificationChannelType; label: string; disabled?: boolean }[] = [
  { value: 'MOCK', label: 'Mock 调试' },
  { value: 'EMAIL', label: 'Email SMTP' },
  { value: 'FEISHU_APP', label: '飞书应用消息' },
  { value: 'WECOM_APP', label: '企业微信应用消息' },
  { value: 'HTTP_CONNECTOR', label: 'HTTP 连接' },
  { value: 'FEISHU_WEBHOOK', label: '飞书群机器人' },
  { value: 'WECOM_WEBHOOK', label: '企业微信群机器人' },
  { value: 'FEISHU', label: '飞书机器人（暂未开放）', disabled: true },
  { value: 'WECOM', label: '企业微信机器人（暂未开放）', disabled: true },
  { value: 'DINGTALK', label: '钉钉机器人', disabled: true },
  { value: 'WEBHOOK', label: '自定义 Webhook', disabled: true },
];

const HTTP_CONNECTOR_FIELD_OPTIONS = [
  { value: 'SUBJECT', label: '标题' },
  { value: 'TEXT', label: '正文' },
  { value: 'EVENT_KEY', label: '事件标识' },
  { value: 'CATEGORY', label: '分类' },
  { value: 'PRIORITY', label: '优先级' },
  { value: 'TRACE_ID', label: '追踪标识' },
  { value: 'DEEP_LINK', label: '站内链接' },
] as const;

const HTTP_CONNECTOR_HEADER_OPTIONS = [
  { value: 'Accept', label: 'Accept' },
  { value: 'X-Notification-Source', label: 'X-Notification-Source' },
  { value: 'X-Notification-Category', label: 'X-Notification-Category' },
  { value: 'X-Notification-Priority', label: 'X-Notification-Priority' },
] as const;

const HTTP_CONNECTOR_AUTH_OPTIONS = [
  { value: 'NONE', label: '不使用认证' },
  { value: 'BEARER', label: 'Bearer 密钥' },
  { value: 'BASIC', label: 'Basic 密钥' },
  { value: 'HMAC_SHA256', label: 'HMAC-SHA256 密钥' },
] as const;

const PROVIDER_PARAMETER_CATALOG: Partial<Record<NotificationChannelType, ProviderParameterDescriptor[]>> = {
  WECOM_APP: [
    {
      key: 'mentionedList',
      label: '提醒成员',
      valueType: 'stringList',
      maxItems: 100,
      maxValueBytes: 64,
      allowDefault: true,
    },
  ],
  FEISHU_APP: [],
};

type EnterpriseTestIdentityKind = 'FEISHU_OPEN_ID' | 'FEISHU_CHAT_ID' | 'WECOM_USERID';

type EnterpriseTestFormValues = {
  identityKind: EnterpriseTestIdentityKind;
  subject: string;
  text: string;
};

type StaticTestFormValues = {
  text: string;
};

type EnterpriseTestResultFeedback =
  | EnterpriseConnectionFeedback
  | {
      tone: 'error';
      title: string;
      detail?: string;
      guidance?: undefined;
    };

const STATUS_VALUE_ENUM = {
  0: { text: '启用', status: 'Success' },
  1: { text: '停用', status: 'Default' },
};

function prettyJson(value?: string, fallback = '{}') {
  if (!value) {
    return fallback;
  }
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

function defaultConfigJson(type: NotificationChannelType) {
  if (type === 'EMAIL') {
    return JSON.stringify(
      {
        host: 'smtp.example.com',
        port: 465,
        username: 'noreply@example.com',
        from: 'Seven <noreply@example.com>',
        useTls: true,
        startTls: false,
        timeout: '10s',
        skipVerify: false,
      },
      null,
      2,
    );
  }
  return JSON.stringify({ capturePrefix: 'notification:mock:capture' }, null, 2);
}

function isEnterpriseApplicationChannel(type?: string): type is 'FEISHU_APP' | 'WECOM_APP' {
  return type === 'FEISHU_APP' || type === 'WECOM_APP';
}

function isHTTPConnectorChannel(type?: string): type is 'HTTP_CONNECTOR' {
  return type === 'HTTP_CONNECTOR';
}

function isWebhookProfileChannel(type?: string): type is 'FEISHU_WEBHOOK' | 'WECOM_WEBHOOK' {
  return type === 'FEISHU_WEBHOOK' || type === 'WECOM_WEBHOOK';
}

function isStaticHTTPChannel(type?: string): type is 'HTTP_CONNECTOR' | 'FEISHU_WEBHOOK' | 'WECOM_WEBHOOK' {
  return isHTTPConnectorChannel(type) || isWebhookProfileChannel(type);
}

function defaultHTTPConnectorConfig(): HTTPConnectorConfig {
  return {
    endpointUrl: '',
    method: 'POST',
    authenticationMode: 'NONE',
    fieldMappings: [{ source: 'TEXT', target: 'message' }],
    headerAllowlist: [],
    idempotencyHeader: 'Idempotency-Key',
    timeoutMilliseconds: 5000,
    successStatusCodes: [],
  };
}

function defaultWebhookProfileConfig(): WebhookProfileConfig {
  return {
    timeoutMilliseconds: 5000,
    successStatusCodes: [],
  };
}

function defaultProviderConfig(type?: string) {
  if (type === 'FEISHU_APP') {
    return { feishuAppId: '' };
  }
  if (type === 'WECOM_APP') {
    return { weComCorpId: '', weComAgentId: '' };
  }
  return undefined;
}

function providerSettingValue(value: unknown) {
  if (Array.isArray(value)) {
    return value.map((item) => String(item)).join(', ');
  }
  return value == null ? '' : String(value);
}

type ProviderParameterRow = {
  key: string;
  label: string;
  enabled: boolean;
  defaultValueText: string;
  valueType: string;
  maxItems?: number;
};

function ProviderParameterEditor({
  catalog,
  values,
  onChange,
}: {
  catalog: ProviderParameterDescriptor[];
  values?: ProviderParameterSetting[];
  onChange: (next: ProviderParameterSetting[]) => void;
}) {
  const rows = useMemo<ProviderParameterRow[]>(
    () =>
      catalog.map((descriptor) => {
        const setting = values?.find((item) => item.key === descriptor.key);
        return {
          key: descriptor.key,
          label: descriptor.label,
          enabled: setting?.enabled ?? false,
          defaultValueText: providerSettingValue(setting?.defaultValue),
          valueType: descriptor.valueType,
          maxItems: descriptor.maxItems,
        };
      }),
    [catalog, values],
  );
  const columns = useMemo<ProColumns<ProviderParameterRow>[]>(
    () => [
      { title: '参数', dataIndex: 'label', editable: false, width: 150 },
      {
        title: '启用',
        dataIndex: 'enabled',
        valueType: 'switch',
        width: 100,
      },
      {
        title: '默认值',
        dataIndex: 'defaultValueText',
        fieldProps: {
          placeholder: '多个成员用逗号分隔',
        },
      },
    ],
    [],
  );
  const saveRows = useCallback(
    (nextRows: ProviderParameterRow[]) => {
      onChange(
        nextRows.map((item) => ({
          key: item.key,
          enabled: !!item.enabled,
          defaultValue: item.defaultValueText
            .split(',')
            .map((value) => value.trim())
            .filter(Boolean),
        })),
      );
    },
    [onChange],
  );
  if (!catalog.length) {
    return null;
  }
  return (
    <div style={{ marginTop: 8 }}>
      <Typography.Text type="secondary">调用方可按需传入；勾选后可设置默认值，未传时才使用默认值。</Typography.Text>
      <EditableProTable<ProviderParameterRow>
        rowKey="key"
        headerTitle={false}
        search={false}
        options={false}
        toolBarRender={false}
        recordCreatorProps={false}
        pagination={false}
        value={rows}
        onChange={(nextRows) => saveRows((nextRows || []) as ProviderParameterRow[])}
        columns={columns}
        editable={{
          type: 'multiple',
          editableKeys: rows.map((item) => item.key),
          onValuesChange: (_, nextRows) => saveRows(nextRows as ProviderParameterRow[]),
          actionRender: () => [],
        }}
      />
    </div>
  );
}

function normalizeRecordId<T extends { id?: string | number }>(values: T): T {
  if (!values.id) {
    return { ...values, id: undefined };
  }
  return { ...values, id: String(values.id) };
}

export default function NotificationCenterPage() {
  const channelActionRef = useRef<ActionType>(undefined);
  const [channelForm] = Form.useForm<NotificationChannel>();
  const [enterpriseTestForm] = Form.useForm<EnterpriseTestFormValues>();
  const [staticTestForm] = Form.useForm<StaticTestFormValues>();
  const [channelOpen, setChannelOpen] = useState(false);
  const [enterpriseTestOpen, setEnterpriseTestOpen] = useState(false);
  const [enterpriseTestChannel, setEnterpriseTestChannel] = useState<NotificationChannel | null>(null);
  const [enterpriseTestResultFeedback, setEnterpriseTestResultFeedback] = useState<EnterpriseTestResultFeedback | null>(null);
  const [staticTestOpen, setStaticTestOpen] = useState(false);
  const [staticTestChannel, setStaticTestChannel] = useState<NotificationChannel | null>(null);
  const [staticTestResultFeedback, setStaticTestResultFeedback] = useState<EnterpriseTestResultFeedback | null>(null);
  const channelType = Form.useWatch('channelType', channelForm);
  const channelID = Form.useWatch('id', channelForm);
  const providerParameterSettings = Form.useWatch('providerParameterSettings', channelForm);
  const httpConnectorAuthenticationMode = Form.useWatch(['httpConnectorConfig', 'authenticationMode'], channelForm) as
    | HTTPConnectorConfig['authenticationMode']
    | undefined;
  const enterpriseTestIdentityKind = Form.useWatch('identityKind', enterpriseTestForm);
  const enterpriseTestIsFeishuGroup =
    enterpriseTestChannel?.channelType === 'FEISHU_APP' && enterpriseTestIdentityKind === 'FEISHU_CHAT_ID';

  const permissions = usePermissionFlags({
    channelList: NOTIFICATION_PERMISSIONS.CHANNEL_LIST,
    channelEdit: NOTIFICATION_PERMISSIONS.CHANNEL_EDIT,
    templateList: NOTIFICATION_PERMISSIONS.TEMPLATE_LIST,
    templateEdit: NOTIFICATION_PERMISSIONS.TEMPLATE_EDIT,
    sceneList: NOTIFICATION_PERMISSIONS.SCENE_LIST,
    sceneEdit: NOTIFICATION_PERMISSIONS.SCENE_EDIT,
    deliveryList: NOTIFICATION_PERMISSIONS.DELIVERY_LIST,
    deliveryDiagnostic: NOTIFICATION_PERMISSIONS.DELIVERY_DIAGNOSTIC,
    test: NOTIFICATION_PERMISSIONS.TEST,
  });

  const saveChannelMutation = useMutation({
    mutationFn: saveNotificationChannel,
    onSuccess: () => {
      message.success('通知渠道已保存');
      setChannelOpen(false);
      channelActionRef.current?.reload();
    },
    onError: (error) => message.error((error as Error).message || '保存通知渠道失败'),
  });

  const enterpriseTestMutation = useMutation({
    mutationFn: testEnterpriseConnection,
    onSuccess: (result) => {
      const testChannelType = enterpriseTestChannel?.channelType;
      const feedback = enterpriseConnectionTestFeedback(
        result,
        isEnterpriseApplicationChannel(testChannelType) ? testChannelType : undefined,
      );
      if (feedback.tone === 'success') {
        message.success([feedback.title, feedback.detail].filter(Boolean).join('；'));
        setEnterpriseTestOpen(false);
        setEnterpriseTestResultFeedback(null);
        return;
      }
      setEnterpriseTestResultFeedback(feedback);
    },
    onError: (error) =>
      setEnterpriseTestResultFeedback({
        tone: 'error',
        title: '测试连接未通过',
        detail: (error as Error).message?.trim() || '请稍后再试',
      }),
  });

  const staticTestMutation = useMutation({
    mutationFn: testStaticConnection,
    onSuccess: (result) => setStaticTestResultFeedback(staticConnectionTestFeedback(result)),
    onError: (error) =>
      setStaticTestResultFeedback({
        tone: 'error',
        title: '测试连接未通过',
        detail: (error as Error).message?.trim() || '请稍后再试',
      }),
  });

  const openEnterpriseTestModal = useCallback(
    (channel: NotificationChannel) => {
      setEnterpriseTestChannel(channel);
      setEnterpriseTestResultFeedback(null);
      enterpriseTestForm.setFieldsValue({
        identityKind: channel.channelType === 'FEISHU_APP' ? 'FEISHU_OPEN_ID' : 'WECOM_USERID',
        subject: '',
        text: '这是一条连接测试消息。',
      });
      setEnterpriseTestOpen(true);
    },
    [enterpriseTestForm],
  );

  const openStaticTestModal = useCallback(
    (channel: NotificationChannel) => {
      setStaticTestChannel(channel);
      setStaticTestResultFeedback(null);
      staticTestForm.setFieldsValue({ text: '这是一条连接测试消息。' });
      setStaticTestOpen(true);
    },
    [staticTestForm],
  );

  const channelColumns: ProColumns<NotificationChannel>[] = [
    { title: '渠道编码', dataIndex: 'channelCode', width: 180, fixed: 'left' },
    { title: '渠道名称', dataIndex: 'channelName', width: 180 },
    {
      title: '渠道类型',
      dataIndex: 'channelType',
      width: 140,
      valueEnum: Object.fromEntries(CHANNEL_TYPE_OPTIONS.map((item) => [item.value, { text: item.label }])),
    },
    { title: '状态', dataIndex: 'status', width: 100, valueEnum: STATUS_VALUE_ENUM },
    { title: '优先级', dataIndex: 'priority', width: 100, search: false },
    {
      title: '密钥',
      dataIndex: 'secretConfigured',
      width: 100,
      search: false,
      render: (_, record) => <Tag color={record.secretConfigured ? 'green' : 'default'}>{record.secretConfigured ? '已配置' : '未配置'}</Tag>,
    },
    { title: '更新时间', dataIndex: 'updateTime', valueType: 'dateTime', width: 180, search: false },
    {
      title: '操作',
      valueType: 'option',
      fixed: 'right',
      width: 190,
      render: (_, record) => (
        <Space>
          {permissions.channelEdit && (
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => {
                channelForm.resetFields();
                channelForm.setFieldsValue({
                  ...record,
                  configJson: isStaticHTTPChannel(record.channelType) ? undefined : prettyJson(record.configJson),
                  metadataJson: isStaticHTTPChannel(record.channelType) ? undefined : record.metadataJson,
                  rateLimitJson: isStaticHTTPChannel(record.channelType) ? undefined : record.rateLimitJson,
                  providerConfig: record.providerConfig || defaultProviderConfig(record.channelType),
                  httpConnectorConfig: record.httpConnectorConfig || (isHTTPConnectorChannel(record.channelType) ? defaultHTTPConnectorConfig() : undefined),
                  webhookProfileConfig: record.webhookProfileConfig || (isWebhookProfileChannel(record.channelType) ? defaultWebhookProfileConfig() : undefined),
                  webhookUrl: '',
                  webhookSigningSecret: '',
                  providerParameterSettings: record.providerParameterSettings || [],
                  secretPlain: '',
                });
                setChannelOpen(true);
              }}
            >
              编辑
            </Button>
          )}
          {permissions.test && isEnterpriseApplicationChannel(record.channelType) && (
            <Button type="link" icon={<SendOutlined />} onClick={() => openEnterpriseTestModal(record)}>
              测试连接
            </Button>
          )}
          {permissions.test && isStaticHTTPChannel(record.channelType) && (
            <Button type="link" icon={<SendOutlined />} onClick={() => openStaticTestModal(record)}>
              测试连接
            </Button>
          )}
        </Space>
      ),
    },
  ];

  const channelWorkspace = (
    <ProTable<NotificationChannel>
      rowKey="channelCode"
      actionRef={channelActionRef}
      columns={channelColumns}
      scroll={{ x: 1100 }}
      request={async (params) => {
        const result = await listNotificationChannels(params);
        return { data: result.records, success: true, total: result.total };
      }}
      toolBarRender={() => [
        permissions.channelEdit && (
          <Button
            key="create"
            type="primary"
            icon={<PlusOutlined />}
            onClick={() => {
              channelForm.resetFields();
              channelForm.setFieldsValue({
                channelType: 'EMAIL',
                status: 0,
                priority: 100,
                configJson: defaultConfigJson('EMAIL'),
                providerConfig: undefined,
                httpConnectorConfig: undefined,
                webhookProfileConfig: undefined,
                webhookUrl: '',
                webhookSigningSecret: '',
                providerParameterSettings: [],
              });
              setChannelOpen(true);
            }}
          >
            新建渠道
          </Button>
        ),
      ]}
    />
  );

  const tabs = [
    ...(permissions.channelList ? [{
      key: 'group-routes',
      label: <Space size={6}><TeamOutlined />通知群</Space>,
      children: <GroupRouteWorkspace />,
    }] : []),
    ...(permissions.channelList ? [{
      key: 'channels',
      label: (
        <Space size={6}>
          <ApiOutlined />
          发送渠道
        </Space>
      ),
      children: channelWorkspace,
    }] : []),
    ...(permissions.templateList ? [{
      key: 'template-versions',
      label: (
        <Space size={6}>
          <FileTextOutlined />
          消息模板
        </Space>
      ),
      children: <VersionedTemplateWorkspace canEdit={permissions.templateEdit} />,
    }] : []),
    ...(permissions.sceneList ? [{
      key: 'scene-versions',
      label: (
        <Space size={6}>
          <SlidersOutlined />
          发送规则
        </Space>
      ),
      children: <VersionedSceneWorkspace canEdit={permissions.sceneEdit} />,
    }] : []),
    ...(permissions.deliveryList ? [{
      key: 'deliveries',
      label: (
        <Space size={6}>
          <ClockCircleOutlined />
          发送记录
        </Space>
      ),
      children: <DeliveryDiagnosticsWorkspace canDiagnose={permissions.deliveryDiagnostic} />,
    }] : []),
  ];

  return (
    <div style={{ padding: '8px 0 24px' }}>
      <ProCard variant="borderless" styles={{ body: { padding: '0 24px 24px' } }}>
        {tabs.length > 0 ? (
          <Tabs size="large" items={tabs} />
        ) : (
          <Result status="403" title="暂无权限" subTitle="请联系管理员开通通知中心查看权限。" />
        )}
      </ProCard>

      <Drawer
        title="通知渠道"
        size="large"
        open={channelOpen}
        onClose={() => setChannelOpen(false)}
        extra={
          <Button type="primary" loading={saveChannelMutation.isPending} onClick={() => channelForm.submit()}>
            保存
          </Button>
        }
      >
        <Form form={channelForm} layout="vertical" onFinish={(values) => saveChannelMutation.mutate(normalizeRecordId(values))}>
          <Form.Item name="id" hidden>
            <Input />
          </Form.Item>
          <Form.Item name="channelCode" label="渠道编码" rules={[{ required: true, message: '请输入渠道编码' }]}>
            <Input placeholder="email-default" />
          </Form.Item>
          <Form.Item name="channelName" label="渠道名称" rules={[{ required: true, message: '请输入渠道名称' }]}>
            <Input placeholder="默认邮件渠道" />
          </Form.Item>
          <Form.Item name="channelType" label="渠道类型" rules={[{ required: true }]}>
            <Select
              options={CHANNEL_TYPE_OPTIONS}
              onChange={(value: NotificationChannelType) => {
                if (isEnterpriseApplicationChannel(value)) {
                  channelForm.setFieldsValue({
                    channelType: value,
                    status: 0,
                    configJson: undefined,
                    metadataJson: undefined,
                    rateLimitJson: undefined,
                    providerConfig: defaultProviderConfig(value),
                    httpConnectorConfig: undefined,
                    webhookProfileConfig: undefined,
                    webhookUrl: '',
                    webhookSigningSecret: '',
                    providerParameterSettings: [],
                    secretPlain: '',
                  });
                  return;
                }
                if (isHTTPConnectorChannel(value)) {
                  channelForm.setFieldsValue({
                    channelType: value,
                    status: 0,
                    configJson: undefined,
                    metadataJson: undefined,
                    rateLimitJson: undefined,
                    providerConfig: undefined,
                    httpConnectorConfig: defaultHTTPConnectorConfig(),
                    webhookProfileConfig: undefined,
                    webhookUrl: '',
                    webhookSigningSecret: '',
                    providerParameterSettings: [],
                    secretPlain: '',
                  });
                  return;
                }
                if (isWebhookProfileChannel(value)) {
                  channelForm.setFieldsValue({
                    channelType: value,
                    status: 0,
                    configJson: undefined,
                    metadataJson: undefined,
                    rateLimitJson: undefined,
                    providerConfig: undefined,
                    httpConnectorConfig: undefined,
                    webhookProfileConfig: defaultWebhookProfileConfig(),
                    webhookUrl: '',
                    webhookSigningSecret: '',
                    providerParameterSettings: [],
                    secretPlain: '',
                  });
                  return;
                }
                channelForm.setFieldsValue({
                  channelType: value,
                  status: 0,
                  configJson: defaultConfigJson(value),
                  providerConfig: undefined,
                  httpConnectorConfig: undefined,
                  webhookProfileConfig: undefined,
                  webhookUrl: '',
                  webhookSigningSecret: '',
                  providerParameterSettings: [],
                  secretPlain: '',
                });
              }}
            />
          </Form.Item>
          <Space size={16}>
            <Form.Item name="status" label="状态" initialValue={0}>
              <Select
                style={{ width: 180 }}
                options={[
                  { value: 0, label: '启用' },
                  { value: 1, label: '停用' },
                ]}
              />
            </Form.Item>
            <Form.Item name="priority" label="优先级" initialValue={100}>
              <InputNumber min={1} max={9999} />
            </Form.Item>
          </Space>
          {isEnterpriseApplicationChannel(channelType) ? (
            <>
              {channelType === 'FEISHU_APP' ? (
                <Form.Item name={['providerConfig', 'feishuAppId']} label="App ID" rules={[{ required: true, message: '请输入 App ID' }]}>
                  <Input placeholder="cli_xxx" autoComplete="off" />
                </Form.Item>
              ) : (
                <Space size={16} style={{ width: '100%' }}>
                  <Form.Item
                    name={['providerConfig', 'weComCorpId']}
                    label="企业 ID"
                    rules={[{ required: true, message: '请输入企业 ID' }]}
                    style={{ flex: 1 }}
                  >
                    <Input placeholder="wwxxx" autoComplete="off" />
                  </Form.Item>
                  <Form.Item
                    name={['providerConfig', 'weComAgentId']}
                    label="应用 AgentId"
                    rules={[{ required: true, message: '请输入应用 AgentId' }]}
                    style={{ flex: 1 }}
                  >
                    <Input placeholder="1000001" autoComplete="off" />
                  </Form.Item>
                </Space>
              )}
              <Form.Item name="secretPlain" label="应用密钥">
                <Input.Password placeholder="新建时必填；编辑时留空保持原密钥" autoComplete="new-password" />
              </Form.Item>
              <Form.Item label="可选项">
                <ProviderParameterEditor
                  catalog={PROVIDER_PARAMETER_CATALOG[channelType] || []}
                  values={providerParameterSettings}
                  onChange={(next) => channelForm.setFieldValue('providerParameterSettings', next)}
                />
              </Form.Item>
            </>
          ) : isHTTPConnectorChannel(channelType) ? (
            <>
              <Alert
                type="info"
                showIcon
                message="连接会固定保存"
                description="业务模块只能选择这个连接。地址、认证和发送内容不会由业务调用方传入。"
                style={{ marginBottom: 16 }}
              />
              <Form.Item
                name={['httpConnectorConfig', 'endpointUrl']}
                label="接收地址"
                rules={[{ required: true, message: '请输入 HTTPS 接收地址' }, { type: 'url', message: '请输入有效的 HTTPS 地址' }]}
              >
                <Input placeholder="https://receiver.example/notifications" autoComplete="off" />
              </Form.Item>
              <Form.Item
                name={['httpConnectorConfig', 'egressPolicyRef']}
                label="受管网络策略"
                extra="留空仅允许公网 HTTPS；内网地址只能使用部署环境预先配置的策略名称。"
              >
                <Input placeholder="可选，例如 corp-orders" autoComplete="off" />
              </Form.Item>
              <Space size={16} style={{ width: '100%' }} wrap>
                <Form.Item
                  name={['httpConnectorConfig', 'authenticationMode']}
                  label="认证方式"
                  rules={[{ required: true, message: '请选择认证方式' }]}
                  style={{ minWidth: 240 }}
                >
                  <Select
                    options={[...HTTP_CONNECTOR_AUTH_OPTIONS]}
                    onChange={(value) => {
                      if (value === 'NONE') {
                        channelForm.setFieldValue('secretPlain', '');
                      }
                    }}
                  />
                </Form.Item>
                <Form.Item
                  name={['httpConnectorConfig', 'timeoutMilliseconds']}
                  label="等待时间"
                  rules={[{ required: true, message: '请输入等待时间' }]}
                  style={{ minWidth: 200 }}
                >
                  <InputNumber min={1000} max={30000} step={500} addonAfter="毫秒" style={{ width: '100%' }} />
                </Form.Item>
              </Space>
              {httpConnectorAuthenticationMode && httpConnectorAuthenticationMode !== 'NONE' ? (
                <Form.Item
                  name="secretPlain"
                  label="连接密钥"
                  extra="保存后不会再次显示；编辑时留空可保持原密钥。"
                  rules={channelID ? undefined : [{ required: true, message: '请配置连接密钥' }]}
                >
                  <Input.Password placeholder="输入新密钥" autoComplete="new-password" />
                </Form.Item>
              ) : null}
              <div style={{ marginBottom: 20 }}>
                <Typography.Text strong>发送内容</Typography.Text>
                <Typography.Paragraph type="secondary" style={{ margin: '4px 0 12px' }}>
                  只选择要发送的通知字段，并为它填写一个简单的输出名称。不能填写脚本、表达式或整段请求内容。
                </Typography.Paragraph>
                <Form.List name={['httpConnectorConfig', 'fieldMappings']}>
                  {(fields, { add, remove }) => (
                    <Space orientation="vertical" size={8} style={{ width: '100%' }}>
                      {fields.map((field, index) => (
                        <Space key={field.key} align="baseline" wrap>
                          <Form.Item
                            {...field}
                            name={[field.name, 'source']}
                            label={index === 0 ? '通知字段' : undefined}
                            rules={[{ required: true, message: '请选择通知字段' }]}
                          >
                            <Select style={{ width: 180 }} options={[...HTTP_CONNECTOR_FIELD_OPTIONS]} />
                          </Form.Item>
                          <Form.Item
                            {...field}
                            name={[field.name, 'target']}
                            label={index === 0 ? '输出名称' : undefined}
                            rules={[{ required: true, message: '请输入输出名称' }, { pattern: /^[A-Za-z0-9_][A-Za-z0-9_-]{0,63}$/, message: '仅支持字母、数字、下划线和短横线' }]}
                          >
                            <Input style={{ width: 220 }} placeholder="message" autoComplete="off" />
                          </Form.Item>
                          <Button type="link" danger onClick={() => remove(field.name)}>
                            删除
                          </Button>
                        </Space>
                      ))}
                      <Button type="dashed" onClick={() => add({ source: 'TEXT', target: 'message' })}>
                        添加字段
                      </Button>
                    </Space>
                  )}
                </Form.List>
              </div>
              <Form.Item name={['httpConnectorConfig', 'headerAllowlist']} label="附加头部">
                <Select mode="multiple" allowClear options={[...HTTP_CONNECTOR_HEADER_OPTIONS]} placeholder="可选" />
              </Form.Item>
              <Typography.Text type="secondary">
                请求固定为 POST；默认 2xx 视为成功。系统会写入幂等标识，地址和认证不会由业务调用方传入。
              </Typography.Text>
            </>
          ) : isWebhookProfileChannel(channelType) ? (
            <>
              <Alert
                type="info"
                showIcon
                message="消息会发送到保存的群机器人"
                description="业务模块只能选择这个连接，不会传入群、地址或原始消息格式。"
                style={{ marginBottom: 16 }}
              />
              <Form.Item
                name="webhookUrl"
                label="群机器人地址"
                extra={channelID ? '留空保持当前地址；如需更换地址，请重新填写。' : '保存后不会再次显示。'}
                rules={channelID ? undefined : [{ required: true, message: '请输入群机器人地址' }, { type: 'url', message: '请输入有效的 HTTPS 地址' }]}
              >
                <Input placeholder={channelType === 'FEISHU_WEBHOOK' ? 'https://open.feishu.cn/open-apis/bot/v2/hook/...' : 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...'} autoComplete="off" />
              </Form.Item>
              {channelType === 'FEISHU_WEBHOOK' ? (
                <Form.Item name="webhookSigningSecret" label="签名密钥（可选）" extra="更换签名密钥时，请同时填写群机器人地址。">
                  <Input.Password placeholder="留空表示不使用签名" autoComplete="new-password" />
                </Form.Item>
              ) : null}
              <Form.Item
                name={['webhookProfileConfig', 'timeoutMilliseconds']}
                label="等待时间"
                rules={[{ required: true, message: '请输入等待时间' }]}
              >
                <InputNumber min={1000} max={30000} step={500} addonAfter="毫秒" style={{ width: 240 }} />
              </Form.Item>
              <Typography.Text type="secondary">群、地址和消息格式由已保存的连接决定。</Typography.Text>
            </>
          ) : (
            <>
              <Form.Item name="configJson" label="渠道配置" rules={[{ required: true, message: '请输入渠道配置' }]}>
                <Input.TextArea rows={9} />
              </Form.Item>
              <Form.Item name="secretPlain" label="密钥/SMTP 授权码">
                <Input.Password placeholder="留空则保持原密钥不变" />
              </Form.Item>
              <Form.Item name="metadataJson" label="扩展信息">
                <Input.TextArea rows={3} placeholder="{}" />
              </Form.Item>
            </>
          )}
        </Form>
      </Drawer>

      <Modal
        title="测试企业应用连接"
        open={enterpriseTestOpen}
        onCancel={() => {
          setEnterpriseTestOpen(false);
          setEnterpriseTestResultFeedback(null);
        }}
        onOk={() => enterpriseTestForm.submit()}
        okText="发送测试"
        confirmLoading={enterpriseTestMutation.isPending}
      >
        <Typography.Paragraph type="secondary">
          只用于确认当前应用配置和接收对象是否可用，不会创建站内信或投递记录。
        </Typography.Paragraph>
        <Form
          form={enterpriseTestForm}
          layout="vertical"
          onValuesChange={() => setEnterpriseTestResultFeedback(null)}
          onFinish={(values) => {
            if (!enterpriseTestChannel || !isEnterpriseApplicationChannel(enterpriseTestChannel.channelType)) {
              setEnterpriseTestResultFeedback({
                tone: 'error',
                title: '测试连接未通过',
                detail: '请选择企业应用渠道',
              });
              return;
            }
            setEnterpriseTestResultFeedback(null);
            const identityKind =
              enterpriseTestChannel.channelType === 'FEISHU_APP'
                ? values.identityKind === 'FEISHU_CHAT_ID'
                  ? 'FEISHU_CHAT_ID'
                  : 'FEISHU_OPEN_ID'
                : 'WECOM_USERID';
            enterpriseTestMutation.mutate({
              connectionRef: enterpriseTestChannel.channelCode,
              identityKind,
              subject: values.subject.trim(),
              text: values.text?.trim(),
            });
          }}
        >
          {enterpriseTestChannel?.channelType === 'FEISHU_APP' && (
            <Form.Item name="identityKind" label="发送给" rules={[{ required: true }]}>
              <Select
                options={[
                  { value: 'FEISHU_OPEN_ID', label: '指定成员' },
                  { value: 'FEISHU_CHAT_ID', label: '指定群聊' },
                ]}
              />
            </Form.Item>
          )}
          <Form.Item
            name="subject"
            label={enterpriseTestIsFeishuGroup ? '群聊 Chat ID' : enterpriseTestChannel?.channelType === 'FEISHU_APP' ? '成员 Open ID' : '成员 UserID'}
            rules={[{ required: true, whitespace: true, message: '请输入接收对象标识' }]}
          >
            <Input
              autoComplete="off"
              placeholder={enterpriseTestIsFeishuGroup ? 'oc_xxx' : enterpriseTestChannel?.channelType === 'FEISHU_APP' ? 'ou_xxx' : 'member-id'}
            />
          </Form.Item>
          <Form.Item name="text" label="测试内容">
            <Input.TextArea rows={3} maxLength={640} showCount />
          </Form.Item>
        </Form>
        {enterpriseTestResultFeedback && (
          <Alert
            showIcon
            type={enterpriseTestResultFeedback.tone}
            message={enterpriseTestResultFeedback.title}
            description={
              enterpriseTestResultFeedback.detail || enterpriseTestResultFeedback.guidance ? (
                <Space orientation="vertical" size={8} style={{ width: '100%' }}>
                  {enterpriseTestResultFeedback.detail && <Typography.Text>{enterpriseTestResultFeedback.detail}</Typography.Text>}
                  {enterpriseTestResultFeedback.guidance && (
                    <div>
                      <Typography.Text strong>{enterpriseTestResultFeedback.guidance.summary}</Typography.Text>
                      <ol style={{ margin: '8px 0 0', paddingInlineStart: 20 }}>
                        {enterpriseTestResultFeedback.guidance.steps.map((step) => (
                          <li key={step}>{step}</li>
                        ))}
                      </ol>
                      {enterpriseTestResultFeedback.guidance.managementUrl && (
                        <Typography.Link
                          href={enterpriseTestResultFeedback.guidance.managementUrl}
                          rel="noreferrer"
                          target="_blank"
                        >
                          打开企业微信管理后台
                        </Typography.Link>
                      )}
                    </div>
                  )}
                </Space>
              ) : undefined
            }
            style={{ marginTop: 4 }}
          />
        )}
      </Modal>

      <Modal
        title={staticTestChannel?.channelType === 'HTTP_CONNECTOR' ? '测试 HTTP 连接' : '测试群机器人连接'}
        open={staticTestOpen}
        onCancel={() => {
          setStaticTestOpen(false);
          setStaticTestResultFeedback(null);
        }}
        onOk={() => staticTestForm.submit()}
        okText="发送测试"
        confirmLoading={staticTestMutation.isPending}
      >
        <Typography.Paragraph type="secondary">
          只用于确认当前连接是否可用，不会创建站内信或投递记录。
        </Typography.Paragraph>
        <Form
          form={staticTestForm}
          layout="vertical"
          onValuesChange={() => setStaticTestResultFeedback(null)}
          onFinish={(values) => {
            if (!staticTestChannel || !isStaticHTTPChannel(staticTestChannel.channelType)) {
              setStaticTestResultFeedback({
                tone: 'error',
                title: '测试连接未通过',
                detail: '请选择受控 HTTP 连接',
              });
              return;
            }
            setStaticTestResultFeedback(null);
            staticTestMutation.mutate({
              connectionRef: staticTestChannel.channelCode,
              text: values.text?.trim(),
            });
          }}
        >
          <Form.Item name="text" label="测试内容">
            <Input.TextArea rows={3} maxLength={640} showCount />
          </Form.Item>
        </Form>
        {staticTestResultFeedback && (
          <Alert
            showIcon
            type={staticTestResultFeedback.tone}
            message={staticTestResultFeedback.title}
            description={staticTestResultFeedback.detail || undefined}
            style={{ marginTop: 4 }}
          />
        )}
      </Modal>

    </div>
  );
}
