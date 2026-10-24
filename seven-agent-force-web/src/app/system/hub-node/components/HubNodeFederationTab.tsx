import { useEffect, useMemo } from 'react';
import { Alert, Button, Descriptions, Form, Input, Space, Spin, Switch, Typography, message } from 'antd';
import { DeploymentUnitOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getHubNodeFederation, provisionHubNodeFederation } from '@/api/hubNodeController';
import { connectionStatusTag } from '../constants';
import { requestActionReason } from '../reason';
import { isValidFederationRedirectUri, runeLength, utf8ByteLength } from '../controllerContract';
import styles from '../hubNode.module.css';

interface Props {
  nodeCode: string;
  nodeName: string;
  canProvision: boolean;
}

interface ProvisionValues {
  connectionVersion: string;
  displayName: string;
  redirectUri: string;
  rotateSecret: boolean;
}

export default function HubNodeFederationTab({ nodeCode, nodeName, canProvision }: Props) {
  const [form] = Form.useForm<ProvisionValues>();
  const queryClient = useQueryClient();
  const queryKey = useMemo(() => ['hub-node', nodeCode, 'federation'], [nodeCode]);
  const federationQuery = useQuery({ queryKey, queryFn: () => getHubNodeFederation(nodeCode), retry: 0 });

  useEffect(() => {
    if (federationQuery.data) {
      form.setFieldsValue({
        connectionVersion: federationQuery.data.connectionVersion || '',
        displayName: nodeName,
        redirectUri: '',
        rotateSecret: false,
      });
    }
  }, [federationQuery.data, form, nodeName]);
  useEffect(() => () => {
    queryClient.removeQueries({ queryKey });
  }, [queryClient, queryKey]);

  const provisionMutation = useMutation({
    mutationFn: ({ values, reason }: { values: ProvisionValues; reason: string }) =>
      provisionHubNodeFederation(nodeCode, { ...values, reason }),
    onSuccess: async () => {
      message.success('联邦连接编排已提交');
      form.setFieldValue('rotateSecret', false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey }),
        queryClient.invalidateQueries({ queryKey: ['hub-node', nodeCode, 'detail'] }),
        queryClient.invalidateQueries({ queryKey: ['hub-nodes'] }),
      ]);
    },
    onError: (error) => message.error((error as Error)?.message || '联邦连接编排失败'),
  });

  const submit = async () => {
    const values = await form.validateFields();
    const reason = await requestActionReason(
      values.rotateSecret ? '编排连接并轮换 OIDC 密钥' : '编排 Node 联邦连接',
      '请输入本次连接变更的审计原因',
    );
    if (reason) await provisionMutation.mutateAsync({ values, reason });
  };

  if (federationQuery.isLoading) return <div className={styles.centered}><Spin /></div>;
  if (federationQuery.isError) {
    return <Alert type="error" showIcon title={(federationQuery.error as Error)?.message || '联邦状态加载失败'} action={<Button onClick={() => federationQuery.refetch()}>重试</Button>} />;
  }

  const federation = federationQuery.data!;
  const federationManageable = federation.connectionStatus !== 'UNKNOWN';
  const effectiveCanProvision = canProvision && federationManageable;
  return (
    <div data-testid="hub-federation-tab">
      {!federationManageable ? (
        <Alert
          type="warning"
          showIcon
          title="联邦连接状态未知"
          description="当前响应无法安全解释，连接编排已关闭。"
          style={{ marginBottom: 16 }}
        />
      ) : null}
      <Descriptions column={{ xs: 1, sm: 2 }} size="small" className={styles.statusDescriptions}>
        <Descriptions.Item label="连接状态">{connectionStatusTag(federation.connectionStatus)}</Descriptions.Item>
        <Descriptions.Item label="连接版本">{federation.connectionVersion || '-'}</Descriptions.Item>
        <Descriptions.Item label="OIDC Client ID">{federation.oidcClientId || '尚未分配'}</Descriptions.Item>
        <Descriptions.Item label="Node 编码">{federation.nodeCode}</Descriptions.Item>
      </Descriptions>
      {federation.lastConnectionError ? (
        <Alert
          type="error"
          showIcon
          title="最近一次连接编排失败"
          description={
            <Space orientation="vertical" size={4}>
              <span>{federation.lastConnectionError}</span>
              {federation.lastConnectionTraceId ? (
                <Typography.Text type="secondary" copyable>Trace ID: {federation.lastConnectionTraceId}</Typography.Text>
              ) : null}
            </Space>
          }
          style={{ marginBottom: 24 }}
        />
      ) : null}

      <div className={styles.sectionHeader}><div><strong>连接编排</strong><span>提交完整版本快照；密钥由 Hub 一次性安全下发，不在此界面显示</span></div></div>
      <Form form={form} layout="vertical" disabled={!effectiveCanProvision || provisionMutation.isPending}>
        <div className={styles.formGrid}>
          <Form.Item
            label="连接版本"
            name="connectionVersion"
            rules={[
              { required: true },
              { validator: (_, value) => !value || utf8ByteLength(value.trim()) <= 128 ? Promise.resolve() : Promise.reject(new Error('连接版本不能超过 128 字节')) },
            ]}
          >
            <Input placeholder="2026-07-12.1" />
          </Form.Item>
          <Form.Item
            label="显示名称"
            name="displayName"
            rules={[
              { required: true },
              { validator: (_, value) => !value || runeLength(value) <= 128 ? Promise.resolve() : Promise.reject(new Error('显示名称不能超过 128 字符')) },
            ]}
          >
            <Input placeholder="Seven Hub" />
          </Form.Item>
        </div>
        <Form.Item
          label="Node 回调地址"
          name="redirectUri"
          rules={[
            { required: true },
            { validator: (_, value) => isValidFederationRedirectUri(value || '') ? Promise.resolve() : Promise.reject(new Error('请输入不含片段的完整 HTTPS 回调地址')) },
          ]}
        >
          <Input placeholder="https://node.example.com/oidc/callback" />
        </Form.Item>
        <Form.Item label="轮换 OIDC 密钥" name="rotateSecret" valuePropName="checked" extra="仅在需要替换现有连接密钥时开启。密钥不会返回到浏览器。">
          <Switch />
        </Form.Item>
        {effectiveCanProvision ? (
          <Button type="primary" icon={<DeploymentUnitOutlined />} loading={provisionMutation.isPending} onClick={() => void submit()}>提交连接编排</Button>
        ) : null}
      </Form>
    </div>
  );
}
