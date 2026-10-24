'use client';

import React, { useMemo } from 'react';
import { Descriptions, Drawer, Space, Table, Tag, Typography } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import type {
  ExternalLoginCapabilities,
  ExternalLoginProviderCapability,
  ExternalLoginProviderRecord,
} from '@/api/externalLoginController';

export interface CapabilitySummaryProps {
  open: boolean;
  provider: ExternalLoginProviderRecord | null;
  capabilities: ExternalLoginCapabilities | null;
  onClose: () => void;
}

function renderBool(value?: boolean, yes = '是', no = '否') {
  return <Tag color={value ? 'green' : 'default'}>{value ? yes : no}</Tag>;
}

function renderTags(values?: string[]) {
  if (!values?.length) {
    return '-';
  }
  return (
    <Space size={[4, 4]} wrap>
      {values.map((value) => (
        <Tag key={value}>{value}</Tag>
      ))}
    </Space>
  );
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return '-';
  }
  return new Date(value).toLocaleString();
}

export default function CapabilitySummary({
  open,
  provider,
  capabilities,
  onClose,
}: CapabilitySummaryProps) {
  const rows = useMemo(
    () =>
      Object.values(capabilities || {}).sort((left, right) =>
        left.providerCode.localeCompare(right.providerCode),
      ),
    [capabilities],
  );

  const columns = useMemo<ColumnsType<ExternalLoginProviderCapability>>(
    () => [
      {
        title: 'Provider',
        dataIndex: 'providerCode',
        width: 140,
        render: (value: string) => <Tag color="blue">{value}</Tag>,
      },
      {
        title: '显示名称',
        dataIndex: 'displayName',
        width: 160,
      },
      {
        title: '协议',
        dataIndex: 'protocolType',
        width: 120,
      },
      {
        title: '能力',
        dataIndex: 'capabilities',
        render: (values: string[]) => renderTags(values),
      },
      {
        title: '默认Scopes',
        dataIndex: 'defaultScopes',
        render: (values: string[]) => renderTags(values),
      },
    ],
    [],
  );

  return (
    <Drawer
      title={provider ? `能力摘要 - ${provider.providerName}` : '能力摘要'}
      width={880}
      open={open}
      onClose={onClose}
      destroyOnHidden
    >
      {provider ? (
        <>
          <Typography.Title level={5}>Provider配置</Typography.Title>
          <Descriptions column={2} size="small" bordered>
            <Descriptions.Item label="Provider编码">{provider.providerCode}</Descriptions.Item>
            <Descriptions.Item label="Provider名称">{provider.providerName}</Descriptions.Item>
            <Descriptions.Item label="协议类型">{provider.protocolType}</Descriptions.Item>
            <Descriptions.Item label="Client ID">{provider.clientId || '-'}</Descriptions.Item>
            <Descriptions.Item label="展示入口">
              {renderBool(provider.displayEnabled)}
            </Descriptions.Item>
            <Descriptions.Item label="允许登录">
              {renderBool(provider.loginEnabled)}
            </Descriptions.Item>
            <Descriptions.Item label="允许绑定">
              {renderBool(provider.bindEnabled)}
            </Descriptions.Item>
            <Descriptions.Item label="邮箱自动绑定">
              {renderBool(provider.emailAutoBindEnabled)}
            </Descriptions.Item>
            <Descriptions.Item label="自动创建账号">
              {renderBool(provider.accountAutoCreateEnabled)}
            </Descriptions.Item>
            <Descriptions.Item label="排序">{provider.sortOrder}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={provider.status === 0 ? 'green' : 'default'}>
                {provider.status === 0 ? '启用' : '停用'}
              </Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Scopes" span={2}>
              {renderTags(provider.scopes)}
            </Descriptions.Item>
            <Descriptions.Item label="Issuer" span={2}>
              {provider.issuer || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="授权端点" span={2}>
              {provider.authorizationEndpoint || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Token端点" span={2}>
              {provider.tokenEndpoint || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="Userinfo端点" span={2}>
              {provider.userinfoEndpoint || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="JWKS URI" span={2}>
              {provider.jwksUri || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="回调地址" span={2}>
              {provider.redirectUri || '-'}
            </Descriptions.Item>
            <Descriptions.Item label="更新时间" span={2}>
              {formatDateTime(provider.updateTime)}
            </Descriptions.Item>
          </Descriptions>
        </>
      ) : null}

      <Typography.Title level={5} className="mt-6">
        Provider能力目录
      </Typography.Title>
      <Table
        rowKey="providerCode"
        columns={columns}
        dataSource={rows}
        pagination={false}
        scroll={{ x: 820 }}
      />
    </Drawer>
  );
}
