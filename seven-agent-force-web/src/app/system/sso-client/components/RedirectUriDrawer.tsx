'use client';

import React, { useEffect } from 'react';
import { Alert, Button, Drawer, Form, Input, Space, Tooltip } from 'antd';
import { DeleteOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import type {
  SsoClientRecord,
  SsoRedirectUriRecord,
  SsoRedirectUriUpdateRequest,
} from '@/api/ssoController';

export interface RedirectUriDrawerProps {
  open: boolean;
  client: SsoClientRecord | null;
  canEdit: boolean;
  loading?: boolean;
  saving?: boolean;
  records: SsoRedirectUriRecord[];
  onClose: () => void;
  onReload: () => Promise<void>;
  onSubmit: (values: SsoRedirectUriUpdateRequest) => Promise<void>;
}

function splitRedirectRecords(records: SsoRedirectUriRecord[]) {
  return {
    redirectUris: records.map((item) => item.redirectUri).filter(Boolean) as string[],
    postLogoutRedirectUris: records
      .map((item) => item.postLogoutRedirectUri)
      .filter(Boolean) as string[],
  };
}

function normalizeUris(values?: string[]) {
  return Array.from(new Set((values || []).map((item) => item.trim()).filter(Boolean)));
}

function validateRedirectUri(_: unknown, value?: string) {
  const next = value?.trim();
  if (!next) {
    return Promise.reject(new Error('请输入回调地址'));
  }
  if (next.includes('*')) {
    return Promise.reject(new Error('回调地址不允许使用通配符'));
  }
  try {
    const parsed = new URL(next);
    const isLocalhost = ['localhost', '127.0.0.1', '::1'].includes(parsed.hostname);
    if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && isLocalhost)) {
      return Promise.reject(new Error('仅开发环境允许 localhost HTTP 回调，生产环境必须使用 HTTPS'));
    }
    return Promise.resolve();
  } catch {
    return Promise.reject(new Error('请输入合法 URL'));
  }
}

export default function RedirectUriDrawer({
  open,
  client,
  canEdit,
  loading,
  saving,
  records,
  onClose,
  onReload,
  onSubmit,
}: RedirectUriDrawerProps) {
  const [form] = Form.useForm<SsoRedirectUriUpdateRequest>();

  useEffect(() => {
    if (open) {
      form.setFieldsValue(splitRedirectRecords(records));
    } else {
      form.resetFields();
    }
  }, [form, open, records]);

  const handleSubmit = async () => {
    const values = await form.validateFields();
    await onSubmit({
      redirectUris: normalizeUris(values.redirectUris),
      postLogoutRedirectUris: normalizeUris(values.postLogoutRedirectUris),
    });
  };

  return (
    <Drawer
      title={client ? `回调地址 - ${client.clientName}` : '回调地址'}
      width={720}
      open={open}
      onClose={onClose}
      loading={loading}
      destroyOnHidden
      extra={
        <Space>
          <Tooltip title="重新加载">
            <Button icon={<ReloadOutlined />} onClick={() => void onReload()} disabled={loading} />
          </Tooltip>
          {canEdit ? (
            <Button type="primary" onClick={handleSubmit} loading={saving}>
              保存
            </Button>
          ) : null}
        </Space>
      }
    >
      <Alert
        type="info"
        showIcon
        className="mb-4"
        message="生产环境回调地址必须使用 HTTPS；仅开发环境允许 localhost HTTP。"
      />
      <Form form={form} layout="vertical" disabled={!canEdit || saving}>
        <Form.List name="redirectUris">
          {(fields, { add, remove }) => (
            <Form.Item label="登录回调地址">
              <Space direction="vertical" className="w-full">
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" className="w-full">
                    <Form.Item
                      {...field}
                      rules={[{ validator: validateRedirectUri }]}
                      className="mb-0 flex-1"
                    >
                      <Input placeholder="https://subsystem.example.com/oauth/callback" />
                    </Form.Item>
                    <Tooltip title="删除">
                      <Button
                        icon={<DeleteOutlined />}
                        danger
                        onClick={() => remove(field.name)}
                        disabled={!canEdit}
                      />
                    </Tooltip>
                  </Space>
                ))}
                {canEdit ? (
                  <Button icon={<PlusOutlined />} onClick={() => add('')}>
                    添加登录回调地址
                  </Button>
                ) : null}
              </Space>
            </Form.Item>
          )}
        </Form.List>
        <Form.List name="postLogoutRedirectUris">
          {(fields, { add, remove }) => (
            <Form.Item label="登出回调地址">
              <Space direction="vertical" className="w-full">
                {fields.map((field) => (
                  <Space key={field.key} align="baseline" className="w-full">
                    <Form.Item
                      {...field}
                      rules={[{ validator: validateRedirectUri }]}
                      className="mb-0 flex-1"
                    >
                      <Input placeholder="https://subsystem.example.com/logout/callback" />
                    </Form.Item>
                    <Tooltip title="删除">
                      <Button
                        icon={<DeleteOutlined />}
                        danger
                        onClick={() => remove(field.name)}
                        disabled={!canEdit}
                      />
                    </Tooltip>
                  </Space>
                ))}
                {canEdit ? (
                  <Button icon={<PlusOutlined />} onClick={() => add('')}>
                    添加登出回调地址
                  </Button>
                ) : null}
              </Space>
            </Form.Item>
          )}
        </Form.List>
      </Form>
    </Drawer>
  );
}
