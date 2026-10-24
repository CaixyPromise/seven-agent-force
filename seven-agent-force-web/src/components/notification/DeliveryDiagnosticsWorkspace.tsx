'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Form, Input, Modal, Select, Tag, Typography } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  listNotificationDeliveries,
  readNotificationDeliveryDiagnosticContent,
  type DeliveryDiagnosticContent,
  type DeliveryDiagnosticContentRequest,
  type NotificationDelivery,
} from '@/api/notificationController';
import { useAuthStore } from '@/store/auth';

const DELIVERY_STATUS_META: Record<string, { label: string; color: string }> = {
  PENDING: { label: '待发送', color: 'blue' },
  SENDING: { label: '发送中', color: 'processing' },
  SENT: { label: '已发送', color: 'green' },
  PROVIDER_ACCEPTED: { label: '平台已受理', color: 'green' },
  UNKNOWN: { label: '结果待确认', color: 'gold' },
  FAILED: { label: '发送失败', color: 'red' },
  CANCELED: { label: '已取消', color: 'default' },
};

const reasonOptions = [
  { value: 'INCIDENT', label: '处理故障' },
  { value: 'CUSTOMER_SUPPORT', label: '处理反馈' },
  { value: 'SECURITY_REVIEW', label: '安全核查' },
  { value: 'OTHER', label: '其他已批准事项' },
] as const;

type DiagnosticFormValues = DeliveryDiagnosticContentRequest;

/**
 * The regular table contains only safe delivery facts. Content is fetched
 * directly for one selected row, kept in component state only, then removed
 * on close or when the signed-in account changes.
 */
export function DeliveryDiagnosticsWorkspace({ canDiagnose }: { canDiagnose: boolean }) {
  const actionRef = useRef<ActionType>(undefined);
  const accountID = useAuthStore((state) => state.user?.id);
  const [form] = Form.useForm<DiagnosticFormValues>();
  const [selected, setSelected] = useState<NotificationDelivery | null>(null);
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState<DeliveryDiagnosticContent | null>(null);
  const [errorMessage, setErrorMessage] = useState('');
  const [loading, setLoading] = useState(false);

  const clearContent = useCallback(() => {
    setContent(null);
    setErrorMessage('');
    setLoading(false);
  }, []);

  const close = useCallback(() => {
    clearContent();
    form.resetFields();
    setSelected(null);
    setOpen(false);
  }, [clearContent, form]);

  // An account switch must not leave the previous account's plaintext in an
  // open modal or a component-held state value.
  useEffect(() => {
    close();
  }, [accountID, close]);

  const columns = useMemo<ProColumns<NotificationDelivery>[]>(
    () => [
      { title: '记录编号', dataIndex: 'deliveryId', width: 220, fixed: 'left' },
      { title: '发送规则', dataIndex: 'sceneCode', width: 150 },
      { title: '发送方式', dataIndex: 'channelCode', width: 170 },
      { title: '模板', dataIndex: 'templateCode', width: 190 },
      { title: '接收对象', dataIndex: 'targetMasked', width: 150, search: false },
      {
        title: '状态',
        dataIndex: 'status',
        width: 120,
        render: (_, record) => {
          const meta = DELIVERY_STATUS_META[record.status] || { label: record.status, color: 'default' };
          return <Tag color={meta.color}>{meta.label}</Tag>;
        },
      },
      { title: '尝试次数', dataIndex: 'retryCount', width: 100, search: false },
      { title: '提示', dataIndex: 'failureMessage', width: 180, search: false, ellipsis: true },
      { title: '创建时间', dataIndex: 'createTime', valueType: 'dateTime', width: 180, search: false },
      {
        title: '操作',
        valueType: 'option',
        fixed: 'right',
        width: 112,
        render: (_, record) =>
          canDiagnose ? (
            <Button
              type="link"
              onClick={() => {
                clearContent();
                form.setFieldsValue({ reasonCode: 'INCIDENT', ticketReference: undefined });
                setSelected(record);
                setOpen(true);
              }}
            >
              查看内容
            </Button>
          ) : null,
      },
    ],
    [canDiagnose, clearContent, form],
  );

  const requestContent = useCallback(async () => {
    if (!selected) {
      return;
    }
    let values: DiagnosticFormValues;
    try {
      values = await form.validateFields();
    } catch {
      return;
    }
    clearContent();
    setLoading(true);
    try {
      // Do not use React Query here: this response must not enter a shared
      // cache, persistence plugin, or global account state.
      const next = await readNotificationDeliveryDiagnosticContent(selected.deliveryId, values);
      setContent(next);
    } catch (error) {
      setErrorMessage(error instanceof Error ? error.message : '暂时无法查看内容');
    } finally {
      setLoading(false);
    }
  }, [clearContent, form, selected]);

  return (
    <>
      <ProTable<NotificationDelivery>
        rowKey="deliveryId"
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1500 }}
        request={async (params) => {
          const result = await listNotificationDeliveries(params);
          return { data: result.records, success: true, total: result.total };
        }}
        toolBarRender={false}
      />

      <Modal
        title="消息内容"
        open={open}
        destroyOnHidden
        onCancel={close}
        footer={
          <>
            <Button onClick={close}>关闭</Button>
            <Button type="primary" loading={loading} onClick={() => void requestContent()}>
              确认查看
            </Button>
          </>
        }
      >
        <Typography.Paragraph type="secondary">
          仅查看这一条记录。请说明用途并完成确认；关闭后不会保留内容。
        </Typography.Paragraph>
        <Form form={form} layout="vertical">
          <Form.Item name="reasonCode" label="查看原因" rules={[{ required: true, message: '请选择查看原因' }]}>
            <Select options={[...reasonOptions]} />
          </Form.Item>
          <Form.Item name="ticketReference" label="工单或事件编号">
            <Input maxLength={128} placeholder="可选" autoComplete="off" />
          </Form.Item>
        </Form>
        {errorMessage ? <Alert type="error" showIcon message={errorMessage} style={{ marginTop: 12 }} /> : null}
        {content ? (
          <div style={{ marginTop: 16 }}>
            {content.expiresAt ? <Alert type="warning" showIcon message="这是短期内容，请勿复制或转发。" /> : null}
            {content.subject ? <Typography.Title level={5}>{content.subject}</Typography.Title> : null}
            <Typography.Paragraph style={{ whiteSpace: 'pre-wrap', marginBottom: 0 }}>
              {content.text || '暂无可显示内容'}
            </Typography.Paragraph>
          </div>
        ) : null}
      </Modal>
    </>
  );
}
