'use client';

import React, { useEffect, useMemo } from 'react';
import { Alert, Checkbox, Drawer, Form, Input, InputNumber, Select, Space, Switch } from 'antd';
import type {
  SsoClientCapabilities,
  SsoClientCreateRequest,
  SsoClientDetail,
  SsoClientRecord,
  SsoClientType,
  SsoClientUpdateRequest,
} from '@/api/ssoController';

export interface ClientFormDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  capabilities: SsoClientCapabilities | null;
  initialValues?: SsoClientDetail | SsoClientRecord | null;
  confirmLoading?: boolean;
  onClose: () => void;
  onSubmit: (values: SsoClientCreateRequest | SsoClientUpdateRequest) => Promise<void>;
}

type ClientFormValues = SsoClientCreateRequest;

function uniqueValues(values: string[]) {
  return Array.from(new Set(values.filter(Boolean)));
}

function normalizeClientFormValues(
  values: ClientFormValues,
  mode: 'create' | 'edit',
): SsoClientCreateRequest | SsoClientUpdateRequest {
  const clientType = values.clientType || 'PUBLIC';
  const normalizedScopes = uniqueValues(['openid', ...(values.scopes || [])]);
  const grantTypes = uniqueValues(values.grantTypes?.length ? values.grantTypes : ['authorization_code']);
  return {
    ...(mode === 'create' ? { clientId: values.clientId?.trim() } : {}),
    clientName: values.clientName?.trim(),
    clientType,
    clientAuthMethod: clientType === 'PUBLIC' ? 'none' : 'client_secret_basic',
    grantTypes,
    scopes: normalizedScopes,
    requirePkce: clientType === 'PUBLIC' ? true : Boolean(values.requirePkce),
    requireConsent: Boolean(values.requireConsent),
    trustedFirstParty: Boolean(values.trustedFirstParty),
    accessTokenTtlSec: Number(values.accessTokenTtlSec || 900),
    refreshTokenTtlSec: Number(values.refreshTokenTtlSec || 2592000),
    metadataJson: values.metadataJson?.trim() || undefined,
  };
}

function toInitialFormValues(
  mode: 'create' | 'edit',
  initialValues?: SsoClientDetail | SsoClientRecord | null,
): Partial<ClientFormValues> {
  if (!initialValues) {
    return {
      clientType: 'PUBLIC',
      clientAuthMethod: 'none',
      grantTypes: ['authorization_code', 'refresh_token'],
      scopes: ['openid', 'profile', 'email'],
      requirePkce: true,
      requireConsent: false,
      trustedFirstParty: true,
      accessTokenTtlSec: 900,
      refreshTokenTtlSec: 2592000,
    };
  }

  return {
    ...(mode === 'create' ? { clientId: initialValues.clientId } : {}),
    clientName: initialValues.clientName,
    clientType: initialValues.clientType,
    clientAuthMethod: initialValues.clientAuthMethod,
    grantTypes: initialValues.grantTypes || ['authorization_code'],
    scopes: initialValues.scopes || ['openid'],
    requirePkce: initialValues.requirePkce,
    requireConsent: initialValues.requireConsent,
    trustedFirstParty: initialValues.trustedFirstParty,
    accessTokenTtlSec: initialValues.accessTokenTtlSec,
    refreshTokenTtlSec: initialValues.refreshTokenTtlSec,
    metadataJson: initialValues.metadataJson,
  };
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

const CLIENT_TYPE_LABEL: Record<string, string> = {
  PUBLIC: '公开客户端',
  CONFIDENTIAL: '保密客户端',
};

const AUTH_METHOD_LABEL: Record<string, string> = {
  none: '无需密钥',
  client_secret_basic: '使用客户端密钥',
};

const GRANT_TYPE_LABEL: Record<string, string> = {
  authorization_code: '授权码登录',
  refresh_token: '刷新登录状态',
};

const SCOPE_LABEL: Record<string, string> = {
  openid: '识别用户身份',
  profile: '读取基础资料',
  email: '读取邮箱',
  offline_access: '保持登录',
  'authorization.console': '访问管理控制台',
};

function valueLabel(value: string, labels: Record<string, string>) {
  return labels[value] || labels[value.toLowerCase()] || labels[value.toUpperCase()] || value;
}

export default function ClientFormDrawer({
  open,
  mode,
  capabilities,
  initialValues,
  confirmLoading,
  onClose,
  onSubmit,
}: ClientFormDrawerProps) {
  const [form] = Form.useForm<ClientFormValues>();
  const clientType = Form.useWatch('clientType', form) as SsoClientType | undefined;
  const selectedScopes = Form.useWatch('scopes', form) as string[] | undefined;

  useEffect(() => {
    if (open) {
      form.setFieldsValue(toInitialFormValues(mode, initialValues));
    } else {
      form.resetFields();
    }
  }, [form, initialValues, mode, open]);

  useEffect(() => {
    if (!open || !clientType) {
      return;
    }
    if (clientType === 'PUBLIC') {
      form.setFieldsValue({ clientAuthMethod: 'none', requirePkce: true });
      return;
    }
    form.setFieldsValue({ clientAuthMethod: 'client_secret_basic' });
  }, [clientType, form, open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (!selectedScopes?.includes('openid')) {
      form.setFieldsValue({ scopes: uniqueValues(['openid', ...(selectedScopes || [])]) });
    }
  }, [form, open, selectedScopes]);

  const clientTypeOptions = useMemo(
    () =>
      (capabilities?.clientTypes || []).map((value) => ({
        label: valueLabel(value, CLIENT_TYPE_LABEL),
        value,
      })),
    [capabilities?.clientTypes],
  );
  const authMethodOptions = useMemo(
    () =>
      (capabilities?.clientAuthMethods || []).map((value) => ({
        label: valueLabel(value, AUTH_METHOD_LABEL),
        value,
      })),
    [capabilities?.clientAuthMethods],
  );
  const grantOptions = useMemo(
    () =>
      (capabilities?.grantTypes || []).map((value) => ({
        label: valueLabel(value, GRANT_TYPE_LABEL),
        value,
      })),
    [capabilities?.grantTypes],
  );
  const scopeOptions = useMemo(
    () =>
      (capabilities?.scopes || [])
        .filter((scope) => scope.name)
        .map((scope) => ({
          label: valueLabel(scope.name, SCOPE_LABEL),
          value: scope.name,
          disabled: scope.required || scope.name === 'openid',
        })),
    [capabilities?.scopes],
  );

  const handleSubmit = async () => {
    const values = await form.validateFields();
    await onSubmit(normalizeClientFormValues(values, mode));
  };

  return (
    <Drawer
      title={mode === 'create' ? '新增 SSO 客户端' : '编辑 SSO 客户端'}
      width={640}
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
        <Alert type="warning" showIcon message="客户端能力尚未加载，暂不能编辑配置。" />
      ) : null}
      <Form form={form} layout="vertical" disabled={!capabilities || confirmLoading}>
        {mode === 'create' ? (
          <Form.Item
            label="客户端标识"
            name="clientId"
            rules={[
              { required: true, message: '请输入客户端标识' },
              {
                pattern: /^[a-zA-Z0-9._:-]{3,80}$/,
                message: '仅允许 3-80 位字母、数字、点、下划线、冒号或短横线',
              },
            ]}
          >
            <Input placeholder="internal-subsystem" />
          </Form.Item>
        ) : null}
        <Form.Item
          label="客户端名称"
          name="clientName"
          rules={[
            { required: true, message: '请输入客户端名称' },
            { max: 100, message: '客户端名称不能超过 100 个字符' },
          ]}
        >
          <Input placeholder="内部子系统" />
        </Form.Item>
        <Form.Item label="客户端类型" name="clientType" rules={[{ required: true }]}>
          <Select options={clientTypeOptions} />
        </Form.Item>
        <Form.Item label="校验方式" name="clientAuthMethod" rules={[{ required: true }]}>
          <Select options={authMethodOptions} disabled />
        </Form.Item>
        <Form.Item label="登录能力" name="grantTypes" rules={[{ required: true }]}>
          <Checkbox.Group options={grantOptions} />
        </Form.Item>
        <Form.Item label="可访问内容" name="scopes" rules={[{ required: true }]}>
          <Checkbox.Group options={scopeOptions} />
        </Form.Item>
        <Space direction="vertical" size="middle" className="w-full">
          <Form.Item label="要求浏览器安全校验" name="requirePkce" valuePropName="checked">
            <Switch disabled={clientType === 'PUBLIC'} />
          </Form.Item>
          <Form.Item label="需要用户授权确认" name="requireConsent" valuePropName="checked">
            <Switch />
          </Form.Item>
          <Form.Item label="内部可信客户端" name="trustedFirstParty" valuePropName="checked">
            <Switch />
          </Form.Item>
        </Space>
        <Form.Item
          label="访问令牌有效期（秒）"
          name="accessTokenTtlSec"
          rules={[
            { required: true, message: '请输入访问令牌有效期' },
            {
              type: 'number',
              min: 300,
              max: 7200,
              message: '访问令牌有效期必须在 300 到 7200 秒之间',
            },
          ]}
        >
          <InputNumber min={300} max={7200} className="w-full" />
        </Form.Item>
        <Form.Item
          label="刷新令牌有效期（秒）"
          name="refreshTokenTtlSec"
          rules={[
            { required: true, message: '请输入刷新令牌有效期' },
            {
              type: 'number',
              min: 3600,
              max: 7776000,
              message: '刷新令牌有效期必须在 3600 到 7776000 秒之间',
            },
          ]}
        >
          <InputNumber min={3600} max={7776000} className="w-full" />
        </Form.Item>
        <Form.Item label="附加配置 JSON" name="metadataJson" rules={[{ validator: validateJson }]}>
          <Input.TextArea rows={4} placeholder='{"owner":"security-team"}' />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
