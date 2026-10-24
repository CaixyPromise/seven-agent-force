import { useEffect } from 'react';
import { Alert, Button, Checkbox, Drawer, Form, Input, Select, Space } from 'antd';
import { SaveOutlined } from '@ant-design/icons';
import type {
  HubDiscoveryTypeValue,
  HubNodeRecord,
  HubNodeStatusValue,
} from '../controllerContract';
import {
  isHubIssuerLocked,
  isValidHubIssuer,
  isValidHubNodeCode,
  isValidManagementBaseUrl,
  utf8ByteLength,
} from '../controllerContract';
import { KNOWN_NODE_CAPABILITIES } from '../constants';

export type HubNodeFormMode = 'create' | 'edit' | 'copy';

export interface HubNodeFormValues {
  nodeCode: string;
  nodeName: string;
  status: HubNodeStatusValue;
  discoveryType: HubDiscoveryTypeValue;
  serviceName?: string;
  managementBaseUrl?: string;
  hubIssuer: string;
  managementBearer?: string;
  capabilities: string[];
}

interface Props {
  open: boolean;
  mode: HubNodeFormMode;
  initialValues: HubNodeRecord | null;
  loading: boolean;
  onClose: () => void;
  onSubmit: (values: HubNodeFormValues) => Promise<void>;
}

function initialFormValues(mode: HubNodeFormMode, node: HubNodeRecord | null): HubNodeFormValues {
  if (mode === 'copy') {
    return {
      nodeCode: node ? `${node.nodeCode}-copy` : '',
      nodeName: node ? `${node.nodeName} 副本` : '',
      status: 1,
      discoveryType:
        node?.discoveryType === 'STATIC' || node?.discoveryType === 'CONSUL'
          ? node.discoveryType
          : 'STATIC',
      hubIssuer: node?.hubIssuer || '',
      capabilities: node?.capabilities || [],
    };
  }
  return {
    nodeCode: node?.nodeCode || '',
    nodeName: node?.nodeName || '',
    status: node?.status === 0 || node?.status === 1 ? node.status : 1,
    discoveryType:
      node?.discoveryType === 'STATIC' || node?.discoveryType === 'CONSUL'
        ? node.discoveryType
        : 'STATIC',
    serviceName: node?.serviceName,
    managementBaseUrl: node?.managementBaseUrl,
    hubIssuer: node?.hubIssuer || '',
    capabilities: node?.capabilities || [],
  };
}

export default function HubNodeFormDrawer({
  open,
  mode,
  initialValues,
  loading,
  onClose,
  onSubmit,
}: Props) {
  const [form] = Form.useForm<HubNodeFormValues>();
  const discoveryType = Form.useWatch('discoveryType', form);

  useEffect(() => {
    if (open) {
      form.setFieldsValue(initialFormValues(mode, initialValues));
    } else {
      form.resetFields();
    }
  }, [form, initialValues, mode, open]);

  const copyMode = mode === 'copy';
  const issuerLocked = mode === 'edit' && isHubIssuerLocked(initialValues);
  const title = mode === 'create' ? '新增 Node' : mode === 'edit' ? '编辑 Node' : '复制 Node';

  return (
    <Drawer
      open={open}
      title={title}
      size="min(100vw, 560px)"
      destroyOnHidden
      maskClosable={!loading}
      onClose={onClose}
      extra={
        <Space>
          <Button onClick={onClose} disabled={loading}>取消</Button>
          <Button
            type="primary"
            icon={<SaveOutlined />}
            loading={loading}
            onClick={() => form.submit()}
          >
            {copyMode ? '创建副本' : '保存'}
          </Button>
        </Space>
      }
    >
      {copyMode ? (
        <Alert
          showIcon
          type="info"
          title="副本将以停用状态创建"
          description="连接状态、版本和错误记录不会复制。管理 Bearer 仅在此处写入，不会回显。"
          style={{ marginBottom: 20 }}
        />
      ) : null}
      <Form
        form={form}
        layout="vertical"
        requiredMark="optional"
        disabled={loading}
        onFinish={async (values) => {
          await onSubmit({ ...values, status: copyMode ? 1 : values.status });
          form.setFieldValue('managementBearer', undefined);
        }}
      >
        <Form.Item
          label="Node 编码"
          name="nodeCode"
          rules={[
            { required: true, message: '请输入 Node 编码' },
            { validator: (_, value) => isValidHubNodeCode(value || '') ? Promise.resolve() : Promise.reject(new Error('使用 2-60 位小写字母、数字、点、下划线或短横线，首位须为字母或数字')) },
          ]}
        >
          <Input autoComplete="off" disabled={mode === 'edit'} placeholder="singapore-node-01" />
        </Form.Item>
        <Form.Item
          label="Node 名称"
          name="nodeName"
          htmlFor="hub-node-display-name"
          rules={[
            { required: true },
            { validator: (_, value) => !value || Array.from(value).length <= 128 ? Promise.resolve() : Promise.reject(new Error('Node 名称不能超过 128 字符')) },
          ]}
        >
          <Input id="hub-node-display-name" placeholder="新加坡生产节点" />
        </Form.Item>

        {!copyMode ? (
          <Form.Item label="运行状态" name="status" rules={[{ required: true }]}>
            <Select options={[{ label: '启用', value: 0 }, { label: '停用', value: 1 }]} />
          </Form.Item>
        ) : null}

        {!copyMode ? (
          <>
            <Form.Item label="发现方式" name="discoveryType" rules={[{ required: true }]}>
              <Select
                options={[
                  { label: 'Static 静态地址', value: 'STATIC' },
                  { label: 'Consul 服务发现', value: 'CONSUL' },
                ]}
              />
            </Form.Item>
            {discoveryType === 'CONSUL' ? (
              <Form.Item
                label="Consul 服务名"
                name="serviceName"
                rules={[
                  { required: true },
                  { validator: (_, value) => !value || utf8ByteLength(value.trim()) <= 128 ? Promise.resolve() : Promise.reject(new Error('Consul 服务名不能超过 128 字节')) },
                ]}
              >
                <Input placeholder="seven-system-node" />
              </Form.Item>
            ) : (
              <Form.Item
                label="管理地址"
                name="managementBaseUrl"
                rules={[
                  { required: true },
                  { validator: (_, value) => isValidManagementBaseUrl(value || '') ? Promise.resolve() : Promise.reject(new Error('请输入带显式端口且不含凭据、查询、片段或非根路径的 HTTP(S) 地址')) },
                ]}
              >
                <Input placeholder="https://node.example.com:9443" />
              </Form.Item>
            )}
            <Form.Item
              label="Hub Issuer"
              name="hubIssuer"
              rules={[
                { required: true },
                { validator: (_, value) => isValidHubIssuer(value || '') ? Promise.resolve() : Promise.reject(new Error('请输入不含凭据、查询或片段的 HTTPS Issuer 地址')) },
              ]}
              extra={issuerLocked ? '连接激活后 Hub Issuer 已永久锁定。' : undefined}
            >
              <Input disabled={issuerLocked} placeholder="https://hub.example.com/api/sso" />
            </Form.Item>
            <Form.Item label="可管理能力" name="capabilities">
              <Checkbox.Group options={KNOWN_NODE_CAPABILITIES} />
            </Form.Item>
          </>
        ) : null}

        <Form.Item
          label={mode === 'edit' ? '替换管理 Bearer' : '管理 Bearer'}
          name="managementBearer"
          preserve={false}
          rules={[
            { validator: (_, value) => !value || utf8ByteLength(value) <= 8192 ? Promise.resolve() : Promise.reject(new Error('管理 Bearer 不能超过 8192 字节')) },
          ]}
          extra={mode === 'edit' ? '留空表示保持现有密钥。该值提交后不会回显。' : '写入后不可查看，请使用受控凭据。'}
        >
          <Input.Password
            data-testid="management-bearer-input"
            autoComplete="new-password"
            maxLength={8192}
            placeholder={mode === 'edit' ? '仅在需要轮换时填写' : '输入 Node 管理凭据'}
          />
        </Form.Item>
      </Form>
    </Drawer>
  );
}
