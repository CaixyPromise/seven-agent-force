import { useEffect, useMemo } from 'react';
import { Alert, Button, Form, Input, InputNumber, Select, Space, Spin, Switch, message } from 'antd';
import type { FormInstance } from 'antd';
import { DeleteOutlined, PlusOutlined, SaveOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  applyHubNodeLoginPolicy,
  canApplyHubLoginPolicy,
  deriveExistingProviderOptions,
  getHubNodeLoginPolicy,
  MANAGED_LOGIN_METHOD_TYPES,
  type HubLoginPolicy,
  type ManagedLoginMethodType,
} from '@/api/hubNodeController';
import { requestActionReason } from '../reason';
import styles from '../hubNode.module.css';

interface Props {
  nodeCode: string;
  canApply: boolean;
}

const METHOD_LABELS: Record<ManagedLoginMethodType, string> = {
  PASSWORD: '账号密码',
  PASSKEY: 'Passkey',
  EXTERNAL_OAUTH: '外部 OAuth',
};

const METHOD_TYPES = MANAGED_LOGIN_METHOD_TYPES.map((value) => ({
  label: METHOD_LABELS[value],
  value,
}));

const MATCH_TYPES = [
  { label: 'SSO Client ID', value: 'CLIENT_ID' },
  { label: '访问 Host', value: 'HOST' },
  { label: 'Origin', value: 'ORIGIN' },
  { label: 'Referer Host', value: 'REFERER_HOST' },
  { label: 'Redirect Host', value: 'REDIRECT_HOST' },
  { label: 'Redirect 前缀', value: 'REDIRECT_PREFIX' },
];

function ProviderSelectField({
  form,
  fieldName,
  options,
}: {
  form: FormInstance<HubLoginPolicy>;
  fieldName: number;
  options: Array<{ label: string; value: string }>;
}) {
  return (
    <Form.Item
      label="Provider"
      name={[fieldName, 'providerCode']}
      rules={[
        {
          validator: (_, value) =>
            form.getFieldValue(['loginMethods', fieldName, 'methodType']) === 'EXTERNAL_OAUTH'
              ? value
                ? Promise.resolve()
                : Promise.reject(new Error('请选择现有 Provider'))
              : value
                ? Promise.reject(new Error('仅外部 OAuth 登录方式可配置 Provider'))
                : Promise.resolve(),
        },
      ]}
    >
      <Select
        allowClear
        options={options}
        disabled={options.length === 0}
        placeholder={options.length ? '选择现有 Provider' : 'Node 未返回可复用 Provider'}
      />
    </Form.Item>
  );
}

export default function HubNodePolicyTab({ nodeCode, canApply }: Props) {
  const [form] = Form.useForm<HubLoginPolicy>();
  const queryClient = useQueryClient();
  const queryKey = useMemo(() => ['hub-node', nodeCode, 'login-policy'], [nodeCode]);
  const policyQuery = useQuery({
    queryKey,
    queryFn: () => getHubNodeLoginPolicy(nodeCode),
    retry: 0,
  });
  const providerOptions = useMemo(
    () => deriveExistingProviderOptions(policyQuery.data),
    [policyQuery.data],
  );
  const policyManageable = canApplyHubLoginPolicy(policyQuery.data);
  const effectiveCanApply = canApply && policyManageable;

  useEffect(() => {
    if (policyQuery.data) form.setFieldsValue(policyQuery.data);
  }, [form, policyQuery.data]);
  useEffect(() => () => {
    queryClient.removeQueries({ queryKey });
  }, [queryClient, queryKey]);

  const applyMutation = useMutation({
    mutationFn: ({ policy, reason }: { policy: HubLoginPolicy; reason: string }) =>
      applyHubNodeLoginPolicy(nodeCode, { ...policy, reason }),
    onSuccess: async () => {
      message.success('完整登录策略已应用');
      await queryClient.invalidateQueries({ queryKey });
    },
    onError: (error) => message.error((error as Error)?.message || '登录策略应用失败'),
  });

  const submit = async () => {
    const policy = await form.validateFields();
    const reason = await requestActionReason('应用完整登录策略', '请输入本次策略变更的审计原因');
    if (reason) await applyMutation.mutateAsync({ policy, reason });
  };

  if (policyQuery.isLoading) return <div className={styles.centered}><Spin /></div>;
  if (policyQuery.isError) {
    return <Alert type="error" showIcon title={(policyQuery.error as Error)?.message || '登录策略加载失败'} action={<Button onClick={() => policyQuery.refetch()}>重试</Button>} />;
  }

  return (
    <Form form={form} layout="vertical" disabled={!effectiveCanApply || applyMutation.isPending}>
      {!policyManageable ? (
        <Alert
          type="warning"
          showIcon
          title="策略包含未知或不完整状态"
          description="为避免把异常快照覆盖到 Node，当前策略仅可查看，不能应用。"
          style={{ marginBottom: 16 }}
        />
      ) : null}
      <div className={styles.formGrid}>
        <Form.Item label="平台编码" name="platformCode" rules={[{ required: true }, { max: 64 }]}>
          <Input placeholder="authorization-console" />
        </Form.Item>
        <Form.Item label="策略状态" name="status" rules={[{ required: true }]}>
          <Select options={[{ label: '启用', value: 0 }, { label: '停用', value: 1 }]} />
        </Form.Item>
        <Form.Item label="允许自动注册" name="allowAutoRegister" valuePropName="checked">
          <Switch />
        </Form.Item>
        <Form.Item label="允许表单注册" name="allowFormRegister" valuePropName="checked">
          <Switch />
        </Form.Item>
      </div>

      <div className={styles.sectionHeader}>
        <div><strong>登录方式</strong><span>定义该 Node 可展示与可执行的登录入口</span></div>
      </div>
      <Form.List name="loginMethods">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, ...field }) => (
              <div className={styles.repeatRow} key={key}>
                <Form.Item {...field} label="类型" name={[field.name, 'methodType']} rules={[{ required: true }]}>
                  <Select options={METHOD_TYPES} />
                </Form.Item>
                <Form.Item {...field} label="显示名称" name={[field.name, 'displayName']} rules={[{ required: true }, { max: 128 }]}>
                  <Input />
                </Form.Item>
                <ProviderSelectField form={form} fieldName={field.name} options={providerOptions} />
                <Form.Item {...field} label="图标" name={[field.name, 'icon']}>
                  <Input placeholder="可选图标名" />
                </Form.Item>
                <Form.Item {...field} label="排序" name={[field.name, 'sortOrder']} rules={[{ required: true }]}>
                  <InputNumber min={0} max={10000} />
                </Form.Item>
                <Form.Item {...field} label="展示" name={[field.name, 'displayEnabled']} valuePropName="checked">
                  <Switch />
                </Form.Item>
                <Form.Item {...field} label="允许登录" name={[field.name, 'loginEnabled']} valuePropName="checked">
                  <Switch />
                </Form.Item>
                {effectiveCanApply ? <Button aria-label="删除登录方式" danger type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} /> : null}
              </div>
            ))}
            {effectiveCanApply ? <Button block type="dashed" icon={<PlusOutlined />} onClick={() => add({ methodType: 'PASSWORD', displayName: '', sortOrder: fields.length, displayEnabled: true, loginEnabled: true })}>添加登录方式</Button> : null}
          </>
        )}
      </Form.List>

      <div className={styles.sectionHeader}>
        <div><strong>来源规则</strong><span>按明确来源匹配当前平台策略</span></div>
      </div>
      <Form.List name="sourceRules">
        {(fields, { add, remove }) => (
          <>
            {fields.map(({ key, ...field }) => (
              <div className={styles.repeatRowCompact} key={key}>
                <Form.Item {...field} label="匹配类型" name={[field.name, 'matchType']} rules={[{ required: true }]}>
                  <Select options={MATCH_TYPES} />
                </Form.Item>
                <Form.Item {...field} label="匹配值" name={[field.name, 'matchValue']} rules={[{ required: true }, { max: 1024 }]}>
                  <Input />
                </Form.Item>
                <Form.Item {...field} label="优先级" name={[field.name, 'priority']} rules={[{ required: true }]}>
                  <InputNumber min={0} max={10000} />
                </Form.Item>
                <Form.Item {...field} label="状态" name={[field.name, 'status']} rules={[{ required: true }]}>
                  <Select options={[{ label: '启用', value: 0 }, { label: '停用', value: 1 }]} />
                </Form.Item>
                {effectiveCanApply ? <Button aria-label="删除来源规则" danger type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} /> : null}
              </div>
            ))}
            {effectiveCanApply ? <Button block type="dashed" icon={<PlusOutlined />} onClick={() => add({ matchType: 'HOST', matchValue: '', priority: fields.length, status: 0 })}>添加来源规则</Button> : null}
          </>
        )}
      </Form.List>

      {effectiveCanApply ? (
        <Space className={styles.formActions}>
          <Button onClick={() => form.setFieldsValue(policyQuery.data!)}>还原</Button>
          <Button type="primary" icon={<SaveOutlined />} loading={applyMutation.isPending} onClick={() => void submit()}>应用完整策略</Button>
        </Space>
      ) : null}
    </Form>
  );
}
