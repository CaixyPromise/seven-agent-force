'use client';

import React, { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Space,
  Table,
  Tag,
  Tooltip,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { CopyOutlined, KeyOutlined, ReloadOutlined, StopOutlined } from '@ant-design/icons';
import type {
  SsoClientRecord,
  SsoClientSecretGenerateRequest,
  SsoClientSecretGenerateResponse,
  SsoClientSecretRecord,
  SsoClientSecretStatusRequest,
} from '@/api/ssoController';

export interface SecretDrawerProps {
  open: boolean;
  client: SsoClientRecord | null;
  canGenerate: boolean;
  canDisable: boolean;
  loading?: boolean;
  records: SsoClientSecretRecord[];
  onClose: () => void;
  onReload: () => Promise<void>;
  onGenerate: (
    values: SsoClientSecretGenerateRequest,
  ) => Promise<SsoClientSecretGenerateResponse>;
  onDisable: (secretId: string | number, values: SsoClientSecretStatusRequest) => Promise<void>;
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return '永不过期';
  }
  return new Date(value).toLocaleString();
}

export default function SecretDrawer({
  open,
  client,
  canGenerate,
  canDisable,
  loading,
  records,
  onClose,
  onReload,
  onGenerate,
  onDisable,
}: SecretDrawerProps) {
  const [generateForm] = Form.useForm<SsoClientSecretGenerateRequest>();
  const [disableForm] = Form.useForm<SsoClientSecretStatusRequest>();
  const [generateOpen, setGenerateOpen] = useState(false);
  const [disableTarget, setDisableTarget] = useState<SsoClientSecretRecord | null>(null);
  const [generating, setGenerating] = useState(false);
  const [disabling, setDisabling] = useState(false);
  const [generatedSecret, setGeneratedSecret] =
    useState<SsoClientSecretGenerateResponse | null>(null);

  const clearGeneratedSecret = () => {
    setGeneratedSecret(null);
  };

  useEffect(() => {
    if (!open) {
      clearGeneratedSecret();
      setGenerateOpen(false);
      setDisableTarget(null);
      generateForm.resetFields();
      disableForm.resetFields();
    }
  }, [disableForm, generateForm, open]);

  useEffect(() => {
    clearGeneratedSecret();
  }, [client?.clientId]);

  const columns = useMemo<ColumnsType<SsoClientSecretRecord>>(
    () => [
      {
        title: '密钥提示',
        dataIndex: 'secretHint',
        render: (value: string | undefined) => value || '-',
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 100,
        render: (status: number) => (
          <Tag color={status === 0 ? 'green' : 'default'}>
            {status === 0 ? '启用' : '停用'}
          </Tag>
        ),
      },
      {
        title: '过期时间',
        dataIndex: 'expiresAt',
        render: (value: string | null | undefined) => formatDateTime(value),
      },
      {
        title: '创建时间',
        dataIndex: 'createTime',
        render: (value: string) => formatDateTime(value),
      },
      {
        title: '操作',
        key: 'action',
        width: 120,
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

  const handleCopySecret = async () => {
    if (!generatedSecret?.clientSecret) {
      return;
    }
    await navigator.clipboard.writeText(generatedSecret.clientSecret);
    message.success('密钥已复制');
  };

  const handleGenerate = async () => {
    const values = await generateForm.validateFields();
    setGenerating(true);
    try {
      const result = await onGenerate(values);
      setGeneratedSecret(result);
      setGenerateOpen(false);
      generateForm.resetFields();
      await onReload();
    } finally {
      setGenerating(false);
    }
  };

  const handleDisable = async () => {
    if (!disableTarget) {
      return;
    }
    const values = await disableForm.validateFields();
    setDisabling(true);
    try {
      await onDisable(disableTarget.secretId, {
        status: 1,
        reason: values.reason,
        allowNoActiveSecret: Boolean(values.allowNoActiveSecret),
      });
      setDisableTarget(null);
      disableForm.resetFields();
      await onReload();
    } finally {
      setDisabling(false);
    }
  };

  return (
    <Drawer
      title={client ? `客户端密钥 - ${client.clientName}` : '客户端密钥'}
      width={760}
      open={open}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space>
          <Tooltip title="重新加载">
            <Button icon={<ReloadOutlined />} onClick={() => void onReload()} disabled={loading} />
          </Tooltip>
          {canGenerate ? (
            <Button type="primary" icon={<KeyOutlined />} onClick={() => setGenerateOpen(true)}>
              生成密钥
            </Button>
          ) : null}
        </Space>
      }
    >
      {generatedSecret ? (
        <div className="mb-4">
          <Alert
            type="warning"
            showIcon
            message="客户端密钥仅显示一次"
            description="关闭此提示后将无法再次查看明文，请立即复制到目标子系统的安全配置中。"
            action={
              <Space direction="vertical">
                <Button icon={<CopyOutlined />} onClick={handleCopySecret}>
                  复制
                </Button>
                <Button onClick={clearGeneratedSecret}>关闭</Button>
              </Space>
            }
          />
          <Input.TextArea
            className="mt-3"
            value={generatedSecret.clientSecret}
            readOnly
            autoSize={{ minRows: 2, maxRows: 4 }}
          />
        </div>
      ) : null}

      <Table
        rowKey={(record) => String(record.secretId)}
        loading={loading}
        columns={columns}
        dataSource={records}
        pagination={false}
      />

      <Modal
        title="生成客户端密钥"
        open={generateOpen}
        onCancel={() => setGenerateOpen(false)}
        onOk={handleGenerate}
        confirmLoading={generating}
        okText="生成"
      >
        <Form form={generateForm} layout="vertical">
          <Form.Item label="有效期（天）" name="expiresInDays">
            <InputNumber min={1} max={3650} className="w-full" placeholder="不填表示永不过期" />
          </Form.Item>
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
        title="禁用客户端密钥"
        open={!!disableTarget}
        onCancel={() => setDisableTarget(null)}
        onOk={handleDisable}
        confirmLoading={disabling}
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
          <Form.Item name="allowNoActiveSecret" valuePropName="checked">
            <Checkbox>允许停用后没有可用密钥</Checkbox>
          </Form.Item>
        </Form>
      </Modal>
    </Drawer>
  );
}
