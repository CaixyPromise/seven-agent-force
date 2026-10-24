'use client';

import React, { useEffect, useMemo } from 'react';
import { Alert, Drawer, Form, Input, InputNumber, Select, Space, Switch } from 'antd';
import type {
  ExternalLoginCapabilities,
  ExternalLoginProviderCreateRequest,
  ExternalLoginProviderRecord,
  ExternalLoginProviderUpdateRequest,
} from '@/api/externalLoginController';

export interface ProviderFormDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  capabilities: ExternalLoginCapabilities | null;
  initialValues?: ExternalLoginProviderRecord | null;
  confirmLoading?: boolean;
  onClose: () => void;
  onSubmit: (
    values: ExternalLoginProviderCreateRequest | ExternalLoginProviderUpdateRequest,
  ) => Promise<void>;
}

type ProviderFormValues = ExternalLoginProviderCreateRequest;

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.map((item) => item.trim()).filter(Boolean)));
}

function validateJson(_: unknown, value?: string) {
  const trimmed = value?.trim();
  if (!trimmed) {
    return Promise.resolve();
  }
  try {
    JSON.parse(trimmed);
    return Promise.resolve();
  } catch {
    return Promise.reject(new Error('请输入合法 JSON'));
  }
}

function toInitialValues(
  mode: 'create' | 'edit',
  initialValues?: ExternalLoginProviderRecord | null,
): Partial<ProviderFormValues> {
  if (!initialValues) {
    return {
      protocolType: 'OAUTH2',
      scopes: ['openid', 'profile', 'email'],
      sortOrder: 0,
      displayEnabled: true,
      loginEnabled: true,
      bindEnabled: true,
      emailAutoBindEnabled: false,
      accountAutoCreateEnabled: false,
    };
  }

  return {
    ...(mode === 'create' ? { providerCode: initialValues.providerCode } : {}),
    providerName: initialValues.providerName,
    protocolType: initialValues.protocolType,
    issuer: initialValues.issuer,
    authorizationEndpoint: initialValues.authorizationEndpoint,
    tokenEndpoint: initialValues.tokenEndpoint,
    userinfoEndpoint: initialValues.userinfoEndpoint,
    jwksUri: initialValues.jwksUri,
    clientId: initialValues.clientId,
    scopes: initialValues.scopes,
    redirectUri: initialValues.redirectUri,
    displayName: initialValues.displayName,
    icon: initialValues.icon,
    sortOrder: initialValues.sortOrder,
    displayEnabled: initialValues.displayEnabled,
    loginEnabled: initialValues.loginEnabled,
    bindEnabled: initialValues.bindEnabled,
    emailAutoBindEnabled: initialValues.emailAutoBindEnabled,
    accountAutoCreateEnabled: initialValues.accountAutoCreateEnabled,
    metadataJson: initialValues.metadataJson,
  };
}

function normalizeValues(values: ProviderFormValues, mode: 'create' | 'edit') {
  const result = {
    ...(mode === 'create' ? { providerCode: values.providerCode?.trim().toLowerCase() } : {}),
    providerName: values.providerName?.trim(),
    protocolType: values.protocolType?.trim() || 'OAUTH2',
    issuer: values.issuer?.trim() || undefined,
    authorizationEndpoint: values.authorizationEndpoint?.trim(),
    tokenEndpoint: values.tokenEndpoint?.trim(),
    userinfoEndpoint: values.userinfoEndpoint?.trim() || undefined,
    jwksUri: values.jwksUri?.trim() || undefined,
    clientId: values.clientId?.trim(),
    ...(mode === 'create' ? { clientSecret: values.clientSecret?.trim() || undefined } : {}),
    scopes: uniqueValues(values.scopes || []),
    redirectUri: values.redirectUri?.trim(),
    displayName: values.displayName?.trim(),
    icon: values.icon?.trim() || undefined,
    sortOrder: Number(values.sortOrder || 0),
    displayEnabled: Boolean(values.displayEnabled),
    loginEnabled: Boolean(values.loginEnabled),
    bindEnabled: Boolean(values.bindEnabled),
    emailAutoBindEnabled: Boolean(values.emailAutoBindEnabled),
    accountAutoCreateEnabled: Boolean(values.accountAutoCreateEnabled),
    metadataJson: values.metadataJson?.trim() || undefined,
  };
  return result;
}

export default function ProviderFormDrawer({
  open,
  mode,
  capabilities,
  initialValues,
  confirmLoading,
  onClose,
  onSubmit,
}: ProviderFormDrawerProps) {
  const [form] = Form.useForm<ProviderFormValues>();

  useEffect(() => {
    if (open) {
      form.setFieldsValue(toInitialValues(mode, initialValues));
    } else {
      form.resetFields();
    }
  }, [form, initialValues, mode, open]);

  const protocolOptions = useMemo(() => {
    const protocols = uniqueValues(
      Object.values(capabilities || {}).map((item) => item.protocolType || 'OAUTH2'),
    );
    return (protocols.length ? protocols : ['OAUTH2', 'OIDC']).map((value) => ({
      label: value,
      value,
    }));
  }, [capabilities]);

  const scopeOptions = useMemo(() => {
    const scopes = uniqueValues(
      Object.values(capabilities || {}).flatMap((item) => item.defaultScopes || []),
    );
    return scopes.map((scope) => ({ label: scope, value: scope }));
  }, [capabilities]);

  const handleSubmit = async () => {
    const values = await form.validateFields();
    await onSubmit(normalizeValues(values, mode));
  };

  return (
    <Drawer
      title={mode === 'create' ? '新增外部登录Provider' : '编辑外部登录Provider'}
      width={720}
      open={open}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space>
          <a onClick={onClose}>取消</a>
          <a onClick={handleSubmit} aria-disabled={confirmLoading}>
            {confirmLoading ? '保存中...' : '保存'}
          </a>
        </Space>
      }
    >
      {!capabilities ? (
        <Alert type="warning" showIcon message="外部登录能力尚未加载，暂不能编辑配置。" />
      ) : null}
      <Form form={form} layout="vertical" disabled={!capabilities || confirmLoading}>
        {mode === 'create' ? (
          <Form.Item
            label="Provider编码"
            name="providerCode"
            rules={[
              { required: true, message: '请输入Provider编码' },
              {
                pattern: /^[a-z0-9][a-z0-9._:-]{1,62}[a-z0-9]$/,
                message: '仅允许 3-64 位小写字母、数字、点、下划线、冒号或短横线',
              },
            ]}
          >
            <Input placeholder="github" />
          </Form.Item>
        ) : null}
        <Form.Item
          label="Provider名称"
          name="providerName"
          rules={[
            { required: true, message: '请输入Provider名称' },
            { max: 100, message: 'Provider名称不能超过 100 个字符' },
          ]}
        >
          <Input placeholder="GitHub OAuth" />
        </Form.Item>
        <Form.Item label="协议类型" name="protocolType" rules={[{ required: true }]}>
          <Select options={protocolOptions} />
        </Form.Item>
        <Form.Item label="Issuer" name="issuer">
          <Input placeholder="https://accounts.google.com" />
        </Form.Item>
        <Form.Item
          label="授权端点"
          name="authorizationEndpoint"
          rules={[{ required: true, message: '请输入授权端点' }]}
        >
          <Input placeholder="https://github.com/login/oauth/authorize" />
        </Form.Item>
        <Form.Item
          label="Token端点"
          name="tokenEndpoint"
          rules={[{ required: true, message: '请输入Token端点' }]}
        >
          <Input placeholder="https://github.com/login/oauth/access_token" />
        </Form.Item>
        <Form.Item label="Userinfo端点" name="userinfoEndpoint">
          <Input placeholder="https://api.github.com/user" />
        </Form.Item>
        <Form.Item label="JWKS URI" name="jwksUri">
          <Input placeholder="https://www.googleapis.com/oauth2/v3/certs" />
        </Form.Item>
        <Form.Item
          label="Client ID"
          name="clientId"
          rules={[{ required: true, message: '请输入Client ID' }]}
        >
          <Input placeholder="OAuth client id" />
        </Form.Item>
        {mode === 'create' ? (
          <Form.Item label="Client Secret" name="clientSecret">
            <Input.Password placeholder="创建后不再展示明文" autoComplete="new-password" />
          </Form.Item>
        ) : null}
        <Form.Item label="Scopes" name="scopes" rules={[{ required: true }]}>
          <Select mode="tags" tokenSeparators={[',', ' ']} options={scopeOptions} />
        </Form.Item>
        <Form.Item
          label="回调地址"
          name="redirectUri"
          rules={[{ required: true, message: '请输入回调地址' }]}
        >
          <Input placeholder="https://console.example.com/oauth/landing/github" />
        </Form.Item>
        <Form.Item
          label="登录页显示名称"
          name="displayName"
          rules={[{ required: true, message: '请输入登录页显示名称' }]}
        >
          <Input placeholder="GitHub" />
        </Form.Item>
        <Form.Item label="图标" name="icon">
          <Input placeholder="github" />
        </Form.Item>
        <Form.Item label="排序" name="sortOrder" rules={[{ required: true }]}>
          <InputNumber className="w-full" min={0} max={9999} />
        </Form.Item>
        <Space direction="vertical" size="middle" className="w-full">
          <Form.Item label="展示入口" name="displayEnabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="允许登录" name="loginEnabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="允许绑定" name="bindEnabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="允许邮箱自动绑定" name="emailAutoBindEnabled" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="允许自动创建账号" name="accountAutoCreateEnabled" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Space>
        <Form.Item label="附加配置 JSON" name="metadataJson" rules={[{ validator: validateJson }]}>
          <Input.TextArea rows={4} placeholder='{"tenant":"default"}' />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
