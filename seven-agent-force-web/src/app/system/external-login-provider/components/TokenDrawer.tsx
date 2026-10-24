'use client';

import React, { useMemo, useState } from 'react';
import { Button, Drawer, Form, Input, Modal, Space, Table, Tag, Tooltip } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { FileTextOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons';
import type {
  ExternalLoginProviderRecord,
  ExternalOAuthTokenRecord,
  ExternalOAuthTokenRevokeRequest,
} from '@/api/externalLoginController';

export interface TokenDrawerProps {
  open: boolean;
  provider: ExternalLoginProviderRecord | null;
  canRevoke: boolean;
  loading?: boolean;
  records: ExternalOAuthTokenRecord[];
  onClose: () => void;
  onReload: () => Promise<void>;
  onRevoke: (tokenId: string | number, values: ExternalOAuthTokenRevokeRequest) => Promise<void>;
}

const TOKEN_STATUS_LABEL: Record<number, { text: string; color: string }> = {
  0: { text: '启用', color: 'green' },
  1: { text: '已撤销', color: 'default' },
  2: { text: '已过期', color: 'orange' },
  3: { text: '刷新失败', color: 'red' },
};

const SENSITIVE_KEY_PATTERN =
  /(access|refresh|id)?token|secret|authorizationcode|codeverifier|state|nonce|ciphertext|edek/i;

function formatDateTime(value?: string | null) {
  if (!value) {
    return '-';
  }
  return new Date(value).toLocaleString();
}

function renderStatus(status: number) {
  const meta = TOKEN_STATUS_LABEL[status] || TOKEN_STATUS_LABEL[0];
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

function redactValue(value: unknown): unknown {
  if (Array.isArray(value)) {
    return value.map(redactValue);
  }
  if (!value || typeof value !== 'object') {
    return value;
  }
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>).map(([key, item]) => [
      key,
      SENSITIVE_KEY_PATTERN.test(key.replace(/[_-]/g, '')) ? '[REDACTED]' : redactValue(item),
    ]),
  );
}

function redactMetadata(metadataJson?: string) {
  if (!metadataJson?.trim()) {
    return '{}';
  }
  try {
    return JSON.stringify(redactValue(JSON.parse(metadataJson)), null, 2);
  } catch {
    const sensitiveMetadataPattern =
      /("(?:(?:access|refresh|id)?token|secret|authorizationCode|codeVerifier|state|nonce)"\s*:\s*)"[^"]*"/gi;
    return metadataJson.replace(sensitiveMetadataPattern, '$1"[REDACTED]"');
  }
}

export default function TokenDrawer({
  open,
  provider,
  canRevoke,
  loading,
  records,
  onClose,
  onReload,
  onRevoke,
}: TokenDrawerProps) {
  const [revokeForm] = Form.useForm<ExternalOAuthTokenRevokeRequest>();
  const [revokeTarget, setRevokeTarget] = useState<ExternalOAuthTokenRecord | null>(null);
  const [metadataTarget, setMetadataTarget] = useState<ExternalOAuthTokenRecord | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const columns = useMemo<ColumnsType<ExternalOAuthTokenRecord>>(
    () => [
      {
        title: 'Token ID',
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
        title: 'Identity ID',
        dataIndex: 'identityId',
        width: 120,
      },
      {
        title: '用户ID',
        dataIndex: 'userId',
        width: 120,
      },
      {
        title: '用途',
        dataIndex: 'tokenPurpose',
        width: 140,
      },
      {
        title: 'Scopes',
        dataIndex: 'scopes',
        width: 220,
        render: (values: string[]) =>
          values?.length ? (
            <Space size={[4, 4]} wrap>
              {values.map((value) => (
                <Tag key={value}>{value}</Tag>
              ))}
            </Space>
          ) : (
            '-'
          ),
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (value: number) => renderStatus(value),
      },
      {
        title: 'Access过期',
        dataIndex: 'accessExpiresAt',
        width: 180,
        render: (value: string | null | undefined) => formatDateTime(value),
      },
      {
        title: 'Refresh过期',
        dataIndex: 'refreshExpiresAt',
        width: 180,
        render: (value: string | null | undefined) => formatDateTime(value),
      },
      {
        title: '最近刷新',
        dataIndex: 'lastRefreshAt',
        width: 180,
        render: (value: string | null | undefined) => formatDateTime(value),
      },
      {
        title: '版本',
        dataIndex: 'version',
        width: 80,
      },
      {
        title: '操作',
        key: 'action',
        fixed: 'right',
        width: 160,
        render: (_, record) => (
          <Space size="small">
            <Button
              type="link"
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => setMetadataTarget(record)}
            >
              元数据
            </Button>
            {canRevoke && record.status === 0 ? (
              <Button
                type="link"
                danger
                size="small"
                icon={<StopOutlined />}
                onClick={() => setRevokeTarget(record)}
              >
                撤销
              </Button>
            ) : null}
          </Space>
        ),
      },
    ],
    [canRevoke],
  );

  const handleRevoke = async () => {
    if (!revokeTarget) {
      return;
    }
    const values = await revokeForm.validateFields();
    setSubmitting(true);
    try {
      await onRevoke(revokeTarget.id, { reason: values.reason.trim() });
      setRevokeTarget(null);
      revokeForm.resetFields();
      await onReload();
    } finally {
      setSubmitting(false);
    }
  };

  const handleClose = () => {
    setRevokeTarget(null);
    setMetadataTarget(null);
    revokeForm.resetFields();
    onClose();
  };

  return (
    <Drawer
      title={provider ? `OAuth令牌 - ${provider.providerName}` : 'OAuth令牌'}
      width={1120}
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
        scroll={{ x: 1600 }}
        pagination={false}
      />

      <Modal
        title="撤销外部OAuth令牌"
        open={!!revokeTarget}
        onCancel={() => {
          setRevokeTarget(null);
          revokeForm.resetFields();
        }}
        onOk={handleRevoke}
        confirmLoading={submitting}
        okText="确认撤销"
        okButtonProps={{ danger: true }}
      >
        <Form form={revokeForm} layout="vertical">
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

      <Modal
        title="Token元数据"
        open={!!metadataTarget}
        footer={null}
        onCancel={() => setMetadataTarget(null)}
        width={720}
      >
        <Input.TextArea
          value={redactMetadata(metadataTarget?.metadataJson)}
          readOnly
          autoSize={{ minRows: 8, maxRows: 18 }}
        />
      </Modal>
    </Drawer>
  );
}
