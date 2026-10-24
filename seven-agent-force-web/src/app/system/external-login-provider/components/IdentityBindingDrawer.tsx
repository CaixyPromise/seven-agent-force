'use client';

import React, { useMemo, useState } from 'react';
import { Button, Drawer, Form, Input, Modal, Space, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { ReloadOutlined, StopOutlined } from '@ant-design/icons';
import type {
  ExternalLoginIdentityRecord,
  ExternalLoginIdentityStatusRequest,
  ExternalLoginProviderRecord,
} from '@/api/externalLoginController';

export interface IdentityBindingDrawerProps {
  open: boolean;
  provider: ExternalLoginProviderRecord | null;
  canDisable: boolean;
  loading?: boolean;
  records: ExternalLoginIdentityRecord[];
  onClose: () => void;
  onReload: () => Promise<void>;
  onDisable: (
    identityId: string | number,
    values: ExternalLoginIdentityStatusRequest,
  ) => Promise<void>;
}

const IDENTITY_STATUS_LABEL: Record<number, { text: string; color: string }> = {
  0: { text: '启用', color: 'green' },
  1: { text: '禁用', color: 'default' },
  2: { text: '已解绑', color: 'orange' },
};

function formatDateTime(value?: string | null) {
  if (!value) {
    return '-';
  }
  return new Date(value).toLocaleString();
}

function renderStatus(status: number) {
  const meta = IDENTITY_STATUS_LABEL[status] || IDENTITY_STATUS_LABEL[0];
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

export default function IdentityBindingDrawer({
  open,
  provider,
  canDisable,
  loading,
  records,
  onClose,
  onReload,
  onDisable,
}: IdentityBindingDrawerProps) {
  const [disableForm] = Form.useForm<ExternalLoginIdentityStatusRequest>();
  const [disableTarget, setDisableTarget] = useState<ExternalLoginIdentityRecord | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const columns = useMemo<ColumnsType<ExternalLoginIdentityRecord>>(
    () => [
      {
        title: 'Identity ID',
        dataIndex: 'id',
        width: 120,
      },
      {
        title: 'Provider',
        dataIndex: 'providerCode',
        width: 120,
        render: (value: string) => <Tag color="blue">{value}</Tag>,
      },
      {
        title: '用户ID',
        dataIndex: 'userId',
        width: 120,
      },
      {
        title: '外部账号',
        dataIndex: 'externalLogin',
        width: 180,
        render: (value: string | undefined, record) => value || record.displayName || '-',
      },
      {
        title: '外部邮箱',
        dataIndex: 'externalEmail',
        width: 220,
        render: (value: string | undefined, record) => (
          <Space size="small">
            <span>{value || '-'}</span>
            {value ? (
              <Tag color={record.emailVerified ? 'green' : 'default'}>
                {record.emailVerified ? '已验证' : '未验证'}
              </Tag>
            ) : null}
          </Space>
        ),
      },
      {
        title: '外部Subject',
        dataIndex: 'externalSubject',
        ellipsis: true,
        width: 240,
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (value: number) => renderStatus(value),
      },
      {
        title: '最近登录',
        dataIndex: 'lastLoginAt',
        width: 180,
        render: (value: string | null | undefined) => formatDateTime(value),
      },
      {
        title: '更新时间',
        dataIndex: 'updateTime',
        width: 180,
        render: (value: string | undefined) => formatDateTime(value),
      },
      {
        title: '操作',
        key: 'action',
        fixed: 'right',
        width: 110,
        render: (_, record) =>
          canDisable && record.status === 0 ? (
            <Button
              type="link"
              danger
              size="small"
              icon={<StopOutlined />}
              onClick={() => setDisableTarget(record)}
            >
              禁用
            </Button>
          ) : null,
      },
    ],
    [canDisable],
  );

  const handleDisable = async () => {
    if (!disableTarget) {
      return;
    }
    const values = await disableForm.validateFields();
    setSubmitting(true);
    try {
      await onDisable(disableTarget.id, {
        status: 1,
        reason: values.reason.trim(),
      });
      setDisableTarget(null);
      disableForm.resetFields();
      await onReload();
    } finally {
      setSubmitting(false);
    }
  };

  const handleClose = () => {
    setDisableTarget(null);
    disableForm.resetFields();
    onClose();
  };

  return (
    <Drawer
      title={provider ? `身份绑定 - ${provider.providerName}` : '身份绑定'}
      width={1040}
      open={open}
      onClose={handleClose}
      destroyOnHidden
      extra={
        <Tooltip title="重新加载">
          <Button icon={<ReloadOutlined />} onClick={() => void onReload()} disabled={loading} />
        </Tooltip>
      }
    >
      <Table
        rowKey={(record) => String(record.id)}
        loading={loading}
        columns={columns}
        dataSource={records}
        scroll={{ x: 1380 }}
        pagination={false}
      />

      <Modal
        title="禁用外部身份绑定"
        open={!!disableTarget}
        onCancel={() => {
          setDisableTarget(null);
          disableForm.resetFields();
        }}
        onOk={handleDisable}
        confirmLoading={submitting}
        okText="确认禁用"
        okButtonProps={{ danger: true }}
      >
        <Form form={disableForm} layout="vertical">
          <Form.Item
            label="操作原因"
            name="reason"
            rules={[
              { required: true, message: '请输入操作原因' },
              { max: 200, message: '操作原因不能超过 200 个字符' },
            ]}
          >
            <Input.TextArea rows={3} placeholder="用于审计追踪" />
          </Form.Item>
        </Form>
      </Modal>
    </Drawer>
  );
}
