'use client';

import React, { useCallback, useMemo, useRef, useState } from 'react';
import { Alert, Button, Descriptions, Drawer, Form, Input, Modal, Select, Space, Tag, Typography, message } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { EditOutlined, PlusOutlined } from '@ant-design/icons';
import {
  createVersionedNotificationScene,
  createVersionedNotificationSceneDraft,
  getVersionedNotificationScene,
  getNotificationReadiness,
  listNotificationChannels,
  listVersionedNotificationScenes,
  listVersionedNotificationTemplates,
  publishVersionedNotificationScene,
  saveVersionedNotificationSceneDraft,
  stopVersionedNotificationScene,
  type NotificationChannel,
  type NotificationReadiness,
  type SceneReceiverKind,
  type SceneRevision,
  type SceneRevisionDraftInput,
  type VersionedNotificationScene,
  type VersionedNotificationTemplate,
} from '@/api/notificationController';

type SendingWayOption = {
  value: string;
  label: string;
  receiverKind: SceneReceiverKind;
  connectionRef?: string;
};

type EditorValues = {
  sceneCode: string;
  sceneName: string;
  templateRevisionId?: API.Int64;
  sendingWay?: string;
};

function sendingWayKey(receiverKind: SceneReceiverKind, connectionRef?: string) {
  return `${receiverKind}::${connectionRef || ''}`;
}

function sceneState(record: VersionedNotificationScene) {
  if (record.currentDraft) {
    return { color: 'blue', label: '草稿' };
  }
  if (record.currentPublished?.enabled === false) {
    return { color: 'default', label: '已停用' };
  }
  if (record.currentPublished) {
    return { color: 'green', label: '已发布' };
  }
  return { color: 'default', label: '未发布' };
}

function visibleRevision(item: VersionedNotificationScene | null): SceneRevision | undefined {
  return item?.currentDraft || item?.currentPublished;
}

function sceneDraft(item: VersionedNotificationScene | null): EditorValues {
  const revision = visibleRevision(item);
  if (!item || !revision) {
    return { sceneCode: '', sceneName: '', templateRevisionId: undefined, sendingWay: undefined };
  }
  return {
    sceneCode: item.sceneCode,
    sceneName: item.sceneName,
    templateRevisionId: revision.templateRevisionId,
    sendingWay: sendingWayKey(item.receiverKind, revision.connectionRef),
  };
}

function enabledConnection(channel: NotificationChannel) {
  return channel.status === 0;
}

function senderOptions(channels: NotificationChannel[]): SendingWayOption[] {
  const options: SendingWayOption[] = [
    { value: sendingWayKey('IN_APP'), label: '站内信', receiverKind: 'IN_APP' },
    { value: sendingWayKey('EMAIL_USER'), label: '用户已验证邮箱', receiverKind: 'EMAIL_USER' },
  ];
  for (const channel of channels.filter(enabledConnection)) {
    if (channel.channelType === 'FEISHU_APP') {
      options.push(
        { value: sendingWayKey('FEISHU_OPEN_ID', channel.channelCode), label: `飞书应用 · 指定成员（${channel.channelName}）`, receiverKind: 'FEISHU_OPEN_ID', connectionRef: channel.channelCode },
        { value: sendingWayKey('FEISHU_CHAT_ID', channel.channelCode), label: `飞书应用 · 指定群聊（${channel.channelName}）`, receiverKind: 'FEISHU_CHAT_ID', connectionRef: channel.channelCode },
      );
    }
    if (channel.channelType === 'WECOM_APP') {
      options.push({ value: sendingWayKey('WECOM_USERID', channel.channelCode), label: `企业微信应用 · 指定成员（${channel.channelName}）`, receiverKind: 'WECOM_USERID', connectionRef: channel.channelCode });
    }
    if (channel.channelType === 'FEISHU_WEBHOOK') {
      options.push({ value: sendingWayKey('FEISHU_WEBHOOK_GROUP', channel.channelCode), label: `飞书群 · ${channel.channelName}`, receiverKind: 'FEISHU_WEBHOOK_GROUP', connectionRef: channel.channelCode });
    }
    if (channel.channelType === 'WECOM_WEBHOOK') {
      options.push({ value: sendingWayKey('WECOM_WEBHOOK_GROUP', channel.channelCode), label: `企业微信群 · ${channel.channelName}`, receiverKind: 'WECOM_WEBHOOK_GROUP', connectionRef: channel.channelCode });
    }
    if (channel.channelType === 'HTTP_CONNECTOR') {
      options.push({ value: sendingWayKey('HTTP_CONNECTOR_FIXED', channel.channelCode), label: `固定连接 · ${channel.channelName}`, receiverKind: 'HTTP_CONNECTOR_FIXED', connectionRef: channel.channelCode });
    }
  }
  return options;
}

const RECEIVER_LABELS: Record<SceneReceiverKind, string> = {
  IN_APP: '站内信',
  EMAIL_USER: '用户邮箱',
  FEISHU_OPEN_ID: '飞书成员',
  FEISHU_CHAT_ID: '飞书群聊',
  WECOM_USERID: '企业微信成员',
  FEISHU_WEBHOOK_GROUP: '飞书群',
  WECOM_WEBHOOK_GROUP: '企业微信群',
  HTTP_CONNECTOR_FIXED: '固定连接',
  FIXED_CONNECTION: '固定连接（旧版）',
};

function capabilityStatus(available: boolean, ready?: boolean) {
  if (!available) return <Tag>未配置</Tag>;
  if (ready === false) return <Tag color="gold">需要完善</Tag>;
  return <Tag color="green">可用</Tag>;
}

/**
 * VersionedSceneWorkspace is intentionally a small configuration surface:
 * one template and one sending way. It keeps V1 scene bindings in their own
 * tab and never accepts raw JSON, a target, a route list, or fallback rules.
 */
export function VersionedSceneWorkspace({ canEdit }: { canEdit: boolean }) {
  const actionRef = useRef<ActionType>(undefined);
  const [editorForm] = Form.useForm<EditorValues>();
  const [editorOpen, setEditorOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [loadingOptions, setLoadingOptions] = useState(false);
  const [activeDefinition, setActiveDefinition] = useState<VersionedNotificationScene | null>(null);
  const [templates, setTemplates] = useState<VersionedNotificationTemplate[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [readinessOpen, setReadinessOpen] = useState(false);
  const [readinessLoading, setReadinessLoading] = useState(false);
  const [readiness, setReadiness] = useState<NotificationReadiness | null>(null);

  const isReadOnly = Boolean(activeDefinition && !activeDefinition.currentDraft);
  const publishedTemplates = useMemo(() => templates.filter((item) => item.currentPublished), [templates]);
  const templateOptions = useMemo(
    () =>
      publishedTemplates.map((item) => ({
        value: item.currentPublished!.id,
        label: `${item.templateName} · V${item.currentPublished!.revisionNo}`,
      })),
    [publishedTemplates],
  );
  const ways = useMemo(() => senderOptions(channels), [channels]);

  const loadOptions = useCallback(async () => {
    setLoadingOptions(true);
    try {
      const [templateResult, channelResult] = await Promise.all([
        listVersionedNotificationTemplates({ current: 1, pageSize: 200 }),
        listNotificationChannels({ current: 1, pageSize: 200 }),
      ]);
      setTemplates(templateResult.records);
      setChannels(channelResult.records);
    } catch (error) {
      message.error((error as Error).message || '加载可用模板和发送方式失败');
    } finally {
      setLoadingOptions(false);
    }
  }, []);

  const columns: ProColumns<VersionedNotificationScene>[] = [
      { title: '规则编码', dataIndex: 'sceneCode', width: 210 },
      { title: '名称', dataIndex: 'sceneName', width: 180 },
      {
        title: '状态',
        width: 100,
        render: (_, record) => {
          const state = sceneState(record);
          return <Tag color={state.color}>{state.label}</Tag>;
        },
      },
      { title: '发送方式', width: 180, render: (_, record) => RECEIVER_LABELS[record.receiverKind] || '通知渠道' },
      { title: '模板版本', width: 120, render: (_, record) => (visibleRevision(record) ? `V${visibleRevision(record)!.revisionNo}` : '—') },
      {
        title: '操作',
        width: 220,
        fixed: 'right',
        render: (_, record) => (
          <Space size={4}>
            <Button size="small" type="link" icon={<EditOutlined />} onClick={() => void openDefinition(record.sceneCode, record.receiverKind)}>
              查看
            </Button>
            <Button size="small" type="link" onClick={() => void checkReadiness(record.sceneCode)}>
              检查可用性
            </Button>
            {canEdit && record.currentPublished && !record.currentDraft && (
              <Button size="small" type="link" onClick={() => void createNextDraft(record)}>
                新建版本
              </Button>
            )}
            {canEdit && record.currentPublished?.enabled && !record.currentDraft && (
              <Button size="small" danger type="link" onClick={() => void stopScene(record)}>
                停用
              </Button>
            )}
          </Space>
        ),
      },
  ];

  function applyDefinition(item: VersionedNotificationScene) {
    setActiveDefinition(item);
    editorForm.setFieldsValue(sceneDraft(item));
  }

  async function openDefinition(sceneCode: string, receiverKind: SceneReceiverKind) {
    try {
      await loadOptions();
      applyDefinition(await getVersionedNotificationScene(sceneCode, receiverKind));
      setEditorOpen(true);
    } catch (error) {
      message.error((error as Error).message || '加载发送规则失败');
    }
  }

  async function checkReadiness(sceneCode: string) {
    setReadinessOpen(true);
    setReadiness(null);
    setReadinessLoading(true);
    try {
      setReadiness(await getNotificationReadiness(sceneCode));
    } catch (error) {
      message.error((error as Error).message || '检查通知可用性失败');
    } finally {
      setReadinessLoading(false);
    }
  }

  async function openCreate() {
    await loadOptions();
    setActiveDefinition(null);
    editorForm.resetFields();
    setEditorOpen(true);
  }

  function selectedWay(): SendingWayOption | undefined {
    const value = editorForm.getFieldValue('sendingWay');
    return ways.find((item) => item.value === value);
  }

  async function persistDraft(): Promise<VersionedNotificationScene | null> {
    const values = await editorForm.validateFields();
    const way = selectedWay();
    if (!way) {
      throw new Error('请选择发送方式');
    }
    const draft: SceneRevisionDraftInput = {
      sceneName: values.sceneName.trim(),
      receiverKind: way.receiverKind,
      templateRevisionId: String(values.templateRevisionId || ''),
      connectionRef: way.connectionRef,
      enabled: true,
    };
    if (!activeDefinition) {
      return createVersionedNotificationScene({ sceneCode: values.sceneCode.trim(), draft });
    }
    if (!activeDefinition.currentDraft) {
      throw new Error('已发布规则只能新建版本后修改');
    }
    return saveVersionedNotificationSceneDraft(activeDefinition.currentDraft.id, {
      expectedVersion: activeDefinition.currentDraft.revisionVersion,
      draft,
    });
  }

  async function saveDraft() {
    setSubmitting(true);
    try {
      const result = await persistDraft();
      if (result) {
        applyDefinition(result);
        message.success('草稿已保存');
        actionRef.current?.reload();
      }
    } catch (error) {
      message.error((error as Error).message || '保存草稿失败');
    } finally {
      setSubmitting(false);
    }
  }

  async function publishDraft() {
    setSubmitting(true);
    try {
      const saved = await persistDraft();
      const draft = saved?.currentDraft;
      if (!saved || !draft) {
        return;
      }
      const result = await publishVersionedNotificationScene(draft.id, draft.revisionVersion);
      applyDefinition(result);
      actionRef.current?.reload();
      message.success('发送规则已发布');
    } catch (error) {
      message.error((error as Error).message || '发布发送规则失败');
    } finally {
      setSubmitting(false);
    }
  }

  async function createNextDraft(record: VersionedNotificationScene) {
    setSubmitting(true);
    try {
      await loadOptions();
      const result = await createVersionedNotificationSceneDraft(record.sceneCode, record.receiverKind);
      applyDefinition(result);
      setEditorOpen(true);
      actionRef.current?.reload();
    } catch (error) {
      message.error((error as Error).message || '新建版本失败');
    } finally {
      setSubmitting(false);
    }
  }

  async function stopScene(record: VersionedNotificationScene) {
    setSubmitting(true);
    try {
      const result = await stopVersionedNotificationScene(record.sceneCode, record.receiverKind);
      if (activeDefinition?.id === record.id) {
        applyDefinition(result);
      }
      actionRef.current?.reload();
      message.success('发送规则已停用');
    } catch (error) {
      message.error((error as Error).message || '停用发送规则失败');
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      <ProTable<VersionedNotificationScene>
        rowKey={(record) => `${record.sceneCode}:${record.receiverKind}`}
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1000 }}
        request={async (params) => {
          const result = await listVersionedNotificationScenes(params);
          return { data: result.records, success: true, total: result.total };
        }}
        toolBarRender={() => [
          canEdit && (
            <Button key="create" type="primary" icon={<PlusOutlined />} onClick={() => void openCreate()}>
              新建规则
            </Button>
          ),
        ]}
      />

      <Drawer
        title={activeDefinition ? activeDefinition.sceneName : '新建发送规则'}
        size="large"
        open={editorOpen}
        onClose={() => setEditorOpen(false)}
        extra={
          !isReadOnly && canEdit ? (
            <Space>
              <Button loading={submitting} onClick={() => void saveDraft()}>保存草稿</Button>
              <Button type="primary" loading={submitting} onClick={() => void publishDraft()}>发布</Button>
            </Space>
          ) : activeDefinition && canEdit ? (
            <Button type="primary" loading={submitting} onClick={() => void createNextDraft(activeDefinition)}>新建版本</Button>
          ) : null
        }
      >
        {isReadOnly && (
          <div style={{ marginBottom: 16 }}>
            <Tag color={activeDefinition?.currentPublished?.enabled ? 'green' : 'default'}>
              {activeDefinition?.currentPublished?.enabled ? '已发布' : '已停用'}
            </Tag>
          </div>
        )}
        <Form form={editorForm} layout="vertical">
          <Typography.Text strong>基本信息</Typography.Text>
          <Space size={16} wrap style={{ display: 'flex', marginTop: 8 }}>
            <Form.Item name="sceneCode" label="规则编码" rules={[{ required: true, whitespace: true, message: '请输入规则编码' }]}>
              <Input disabled={Boolean(activeDefinition)} style={{ width: 280 }} placeholder="invoice_ready" />
            </Form.Item>
            <Form.Item name="sceneName" label="规则名称" rules={[{ required: true, whitespace: true, message: '请输入规则名称' }]}>
              <Input disabled={isReadOnly || !canEdit} style={{ width: 280 }} placeholder="账单已生成" />
            </Form.Item>
          </Space>
          <Typography.Text strong>发送设置</Typography.Text>
          <Form.Item name="templateRevisionId" label="模板" rules={[{ required: true, message: '请选择已发布模板' }]} style={{ marginTop: 8 }}>
            <Select disabled={isReadOnly || !canEdit} loading={loadingOptions} showSearch placeholder="选择已发布模板" options={templateOptions} optionFilterProp="label" />
          </Form.Item>
          <Form.Item name="sendingWay" label="发送方式" rules={[{ required: true, message: '请选择发送方式' }]}>
            <Select disabled={isReadOnly || !canEdit || Boolean(activeDefinition)} loading={loadingOptions} showSearch placeholder="选择一种发送方式" options={ways} optionFilterProp="label" />
          </Form.Item>
          {!isReadOnly && (
            <Typography.Paragraph type="secondary" style={{ marginTop: 8 }}>
              发布后，新的通知将按当前设置发送。
            </Typography.Paragraph>
          )}
        </Form>
      </Drawer>

      <Modal
        title="当前通知可用性"
        open={readinessOpen}
        footer={null}
        onCancel={() => setReadinessOpen(false)}
        loading={readinessLoading}
        destroyOnHidden
      >
        {readiness && (
          <Space orientation="vertical" size={16} style={{ width: '100%' }}>
            {!readiness.taskLink.available && (
              <Alert
                type="warning"
                showIcon
                title="外部消息暂时无法直接打开平台"
                description="消息正文仍会正常发送。管理员配置平台公开访问地址后，外部消息会自动显示跳转按钮。"
              />
            )}
            <Descriptions column={1} size="small" bordered>
              <Descriptions.Item label="站内信">{capabilityStatus(readiness.inApp.available)}</Descriptions.Item>
              <Descriptions.Item label="邮箱">
                {capabilityStatus(readiness.email.available, readiness.email.recipientReady)}
                {readiness.email.available && !readiness.email.recipientReady && <Typography.Text type="secondary"> 用户需要先验证邮箱</Typography.Text>}
              </Descriptions.Item>
              <Descriptions.Item label="飞书">
                {capabilityStatus(readiness.feishu.available, readiness.feishu.directReady || readiness.feishu.groupReady)}
                {readiness.feishu.boundRouteName && <Typography.Text type="secondary"> {readiness.feishu.boundRouteName}</Typography.Text>}
              </Descriptions.Item>
              <Descriptions.Item label="企业微信">
                {capabilityStatus(readiness.wecom.available, readiness.wecom.directReady || readiness.wecom.groupReady)}
                {readiness.wecom.boundRouteName && <Typography.Text type="secondary"> {readiness.wecom.boundRouteName}</Typography.Text>}
              </Descriptions.Item>
              <Descriptions.Item label="消息跳转">{capabilityStatus(readiness.taskLink.available)}</Descriptions.Item>
            </Descriptions>
          </Space>
        )}
      </Modal>

    </Space>
  );
}
