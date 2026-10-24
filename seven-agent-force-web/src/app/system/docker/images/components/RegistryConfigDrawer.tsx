'use client';

import { Button, Drawer, Form, Input, InputNumber, Select, Space, Switch } from 'antd';
import { useEffect } from 'react';
import type { DockerRemoteRegistryCommand, DockerRemoteRegistryView } from '@/api/dockerController';

interface RegistryConfigDrawerProps {
  open: boolean;
  loading: boolean;
  registry?: DockerRemoteRegistryView | null;
  onClose: () => void;
  onSubmit: (values: DockerRemoteRegistryCommand) => Promise<void>;
}

const defaultValues: DockerRemoteRegistryCommand = {
  name: '',
  code: '',
  registryType: 'REGISTRY',
  endpoint: '',
  apiBaseUrl: '',
  authType: 'ANONYMOUS',
  username: '',
  password: '',
  namespaceWhitelistJson: '',
  tlsEnabled: true,
  insecureSkipVerify: false,
  defaultRegistry: false,
  status: 0,
  description: '',
  sort: 0,
};

function normalizeAuthType(
  value: string | undefined,
): DockerRemoteRegistryCommand['authType'] {
  if (value === 'BASIC' || value === 'TOKEN_CHALLENGE') {
    return value;
  }
  return 'ANONYMOUS';
}

export function RegistryConfigDrawer({
  open,
  loading,
  registry,
  onClose,
  onSubmit,
}: RegistryConfigDrawerProps) {
  const [form] = Form.useForm<DockerRemoteRegistryCommand>();
  const authType = Form.useWatch('authType', form);

  useEffect(() => {
    if (!open) {
      return;
    }
    form.setFieldsValue({
      ...defaultValues,
      ...registry,
      registryType: 'REGISTRY',
      authType: normalizeAuthType(registry?.authType),
      password: '',
    });
  }, [form, open, registry]);

  const handleFinish = async () => {
    const values = await form.validateFields();
    await onSubmit({
      ...defaultValues,
      ...values,
      registryType: 'REGISTRY',
      password: values.password || undefined,
    });
    form.resetFields();
  };

  return (
    <Drawer
      open={open}
      styles={{ wrapper: { width: 920, maxWidth: '100vw' } }}
      title={registry ? '编辑镜像源' : '新增镜像源'}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button type="primary" loading={loading} onClick={handleFinish}>
            保存
          </Button>
        </Space>
      }
    >
      <Form<DockerRemoteRegistryCommand> form={form} layout="vertical">
        <div className="space-y-6">
            <section>
              <div className="mb-3 text-sm font-medium text-slate-700">连接信息</div>
              <div className="grid gap-4 md:grid-cols-2">
                <Form.Item label="名称" name="name" rules={[{ required: true, message: '请输入名称' }]}>
                  <Input placeholder="例如：本地测试仓库" />
                </Form.Item>
                <Form.Item label="编码" name="code" rules={[{ required: true, message: '请输入编码' }]}>
                  <Input placeholder="例如：LOCAL_REGISTRY" />
                </Form.Item>
              </div>
              <div className="grid gap-4 md:grid-cols-2">
                <Form.Item label="服务地址" name="endpoint" rules={[{ required: true, message: '请输入服务地址' }]}>
                  <Input placeholder="例如：http://127.0.0.1:5001" />
                </Form.Item>
                <Form.Item label="API 根路径" name="apiBaseUrl">
                  <Input placeholder="留空默认使用服务地址 + /v2" />
                </Form.Item>
              </div>
              <Form.Item label="描述" name="description">
                <Input.TextArea autoSize={{ minRows: 2, maxRows: 4 }} placeholder="填写这个 registry 的用途说明" />
              </Form.Item>
            </section>

            <section>
              <div className="mb-3 text-sm font-medium text-slate-700">认证与命名空间</div>
              <div className="grid gap-4 md:grid-cols-2">
                <Form.Item label="认证方式" name="authType">
                  <Select
                    options={[
                      { label: '匿名', value: 'ANONYMOUS' },
                      { label: 'Basic 认证', value: 'BASIC' },
                      { label: 'Bearer Challenge', value: 'TOKEN_CHALLENGE' },
                    ]}
                  />
                </Form.Item>
                <Form.Item label="命名空间白名单" name="namespaceWhitelistJson">
                  <Input.TextArea
                    autoSize={{ minRows: 3, maxRows: 5 }}
                    placeholder='可选，JSON 数组，例如 ["demo", "prod/team-a"]'
                  />
                </Form.Item>
              </div>
              {authType === 'BASIC' || authType === 'TOKEN_CHALLENGE' ? (
                <div className="grid gap-4 md:grid-cols-2">
                  <Form.Item
                    label={authType === 'TOKEN_CHALLENGE' ? 'Robot 用户名' : '用户名'}
                    name="username"
                    rules={[{ required: true, message: '请输入用户名' }]}
                  >
                    <Input placeholder={authType === 'TOKEN_CHALLENGE' ? '请输入只读 robot account 用户名' : '请输入仓库用户名'} />
                  </Form.Item>
                  <Form.Item
                    label={
                      registry?.secretConfigured
                        ? `${authType === 'TOKEN_CHALLENGE' ? 'Robot 密码' : '密码'}（留空表示保持不变）`
                        : authType === 'TOKEN_CHALLENGE'
                          ? 'Robot 密码'
                          : '密码'
                    }
                    name="password"
                    rules={registry?.secretConfigured ? [] : [{ required: true, message: '请输入密码' }]}
                  >
                    <Input.Password placeholder={registry?.secretConfigured ? '不修改密码可留空' : '请输入密码'} />
                  </Form.Item>
                </div>
              ) : null}
              {authType === 'TOKEN_CHALLENGE' ? (
                <div className="grid gap-4 md:grid-cols-2">
                  <Form.Item label="Token Realm" name="tokenRealm">
                    <Input placeholder="可留空。测试连接成功后会自动回填并保存" />
                  </Form.Item>
                  <Form.Item label="Token Service" name="tokenService">
                    <Input placeholder="可留空。测试连接成功后会自动回填并保存" />
                  </Form.Item>
                </div>
              ) : null}
            </section>

            <section>
              <div className="mb-3 text-sm font-medium text-slate-700">安全与策略</div>
              <div className="grid gap-4 md:grid-cols-2">
                <Form.Item label="排序" name="sort">
                  <InputNumber min={0} precision={0} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item label="启用状态" name="status" tooltip="0 表示启用，1 表示停用">
                  <Select
                    options={[
                      { label: '启用', value: 0 },
                      { label: '停用', value: 1 },
                    ]}
                  />
                </Form.Item>
              </div>
              <div className="grid gap-4 md:grid-cols-3">
                <Form.Item label="启用 TLS" name="tlsEnabled" valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item label="跳过证书校验" name="insecureSkipVerify" valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item label="设为默认仓库" name="defaultRegistry" valuePropName="checked">
                  <Switch />
                </Form.Item>
              </div>
            </section>
        </div>
      </Form>
    </Drawer>
  );
}
