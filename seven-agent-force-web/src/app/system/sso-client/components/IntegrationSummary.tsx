'use client';

import React, { useMemo } from 'react';
import { Descriptions, Drawer, Empty, Space, Tag, Typography } from 'antd';
import type {
  SsoClientCapabilities,
  SsoClientDetail,
  SsoClientRecord,
} from '@/api/ssoController';
import styles from './IntegrationSummary.module.css';

export interface IntegrationSummaryProps {
  open: boolean;
  client: SsoClientDetail | SsoClientRecord | null;
  capabilities: SsoClientCapabilities | null;
  onClose: () => void;
}

const { Text } = Typography;

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

const SIGNING_ALGORITHM_LABEL: Record<string, string> = {
  RS256: 'RSA 签名',
};

function buildSsoEndpoints() {
  const base = typeof window === 'undefined' ? '' : `${window.location.origin}/api/sso`;
  return {
    issuer: base,
    discoveryUrl: `${base}/.well-known/openid-configuration`,
    authorizationEndpoint: `${base}/oauth2/authorize`,
    tokenEndpoint: `${base}/oauth2/token`,
    userinfoEndpoint: `${base}/oauth2/userinfo`,
    jwksUri: `${base}/.well-known/jwks.json`,
    revocationEndpoint: `${base}/oauth2/revoke`,
  };
}

function valueLabel(value: string, labels?: Record<string, string>) {
  return labels?.[value] || labels?.[value.toLowerCase()] || labels?.[value.toUpperCase()] || value;
}

function TagList({ values, labels }: { values?: string[]; labels?: Record<string, string> }) {
  if (!values?.length) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Space size={[4, 4]} wrap>
      {values.map((value) => (
        <Tag key={value}>{valueLabel(value, labels)}</Tag>
      ))}
    </Space>
  );
}

interface ScopeCatalogItem {
  name?: string;
  description?: string;
}

function parseScopeCatalog(metadataJson?: string): ScopeCatalogItem[] {
  if (!metadataJson) {
    return [];
  }
  try {
    const metadata = JSON.parse(metadataJson) as { scopeCatalog?: ScopeCatalogItem[] };
    return Array.isArray(metadata.scopeCatalog)
      ? metadata.scopeCatalog.filter((item) => item?.name)
      : [];
  } catch {
    return [];
  }
}

function ScopeCatalog({ values }: { values: ScopeCatalogItem[] }) {
  if (!values.length) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Space direction="vertical" size={4}>
      {values.map((item) => (
        <Space key={item.name} size={8} wrap>
          <Tag>{valueLabel(item.name || '', SCOPE_LABEL)}</Tag>
          {item.description ? <Text type="secondary">{item.description}</Text> : null}
        </Space>
      ))}
    </Space>
  );
}

function CopyText({ value }: { value: string }) {
  if (!value) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Text copyable={{ text: value }} className={styles.copyText}>
      {value}
    </Text>
  );
}

function statusTag(status?: number) {
  return <Tag color={status === 0 ? 'green' : 'default'}>{status === 0 ? '启用' : '停用'}</Tag>;
}

function boolTag(value?: boolean, yesColor = 'blue') {
  return <Tag color={value ? yesColor : 'default'}>{value ? '是' : '否'}</Tag>;
}

function formatTtl(seconds?: number) {
  const value = Number(seconds || 0);
  if (!Number.isFinite(value) || value <= 0) {
    return '-';
  }
  if (value % 86400 === 0) {
    return `${value / 86400} 天 (${value}s)`;
  }
  if (value % 3600 === 0) {
    return `${value / 3600} 小时 (${value}s)`;
  }
  if (value % 60 === 0) {
    return `${value / 60} 分钟 (${value}s)`;
  }
  return `${value}s`;
}

function formatTime(value?: string) {
  return value ? value.replace('T', ' ').replace('Z', '') : '-';
}

function RedirectUriList({ client }: { client: SsoClientDetail | SsoClientRecord }) {
  const detail = client as SsoClientDetail;
  const redirectUris = detail.redirectUris || [];
  if (!redirectUris.length) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Space direction="vertical" size={4} className="w-full">
      {redirectUris.map((item) => (
        <Space key={`${item.id ?? item.redirectUri ?? item.postLogoutRedirectUri}`} wrap>
          {item.redirectUri ? <Tag color="blue">登录回调</Tag> : null}
          {item.postLogoutRedirectUri ? <Tag color="purple">退出回调</Tag> : null}
          <CopyText value={item.redirectUri || item.postLogoutRedirectUri || ''} />
          {statusTag(item.status)}
        </Space>
      ))}
    </Space>
  );
}

function SecretSummary({ client }: { client: SsoClientDetail | SsoClientRecord }) {
  const detail = client as SsoClientDetail;
  const secrets = detail.secrets || [];
  if (!secrets.length) {
    return <Text type="secondary">-</Text>;
  }
  return (
    <Space direction="vertical" size={4}>
      {secrets.map((item) => (
        <Space key={String(item.secretId)} wrap>
          <Tag>{item.secretHint || `#${item.secretId}`}</Tag>
          {statusTag(item.status)}
          <Text type="secondary">过期时间：{item.expiresAt || '永不过期'}</Text>
        </Space>
      ))}
    </Space>
  );
}

export default function IntegrationSummary({
  open,
  client,
  capabilities,
  onClose,
}: IntegrationSummaryProps) {
  const endpoints = useMemo(() => buildSsoEndpoints(), []);
  const scopeCatalog = useMemo(() => parseScopeCatalog(client?.metadataJson), [client?.metadataJson]);
  const descriptionProps = {
    bordered: true,
    column: 1,
    size: 'small' as const,
    className: styles.summaryDescriptions,
  };

  return (
    <Drawer
      title="接入摘要"
      width="min(920px, calc(100vw - 32px))"
      rootClassName={styles.summaryDrawer}
      open={open}
      onClose={onClose}
      destroyOnHidden
    >
      {!client ? (
        <Empty description="请选择客户端" />
      ) : (
        <Space direction="vertical" size="large" className="w-full">
          <Descriptions {...descriptionProps} title="客户端">
            <Descriptions.Item label="客户端标识">
              <CopyText value={client.clientId} />
            </Descriptions.Item>
            <Descriptions.Item label="客户端名称">{client.clientName}</Descriptions.Item>
            <Descriptions.Item label="客户端类型">
              {valueLabel(client.clientType, CLIENT_TYPE_LABEL)}
            </Descriptions.Item>
            <Descriptions.Item label="校验方式">
              {valueLabel(client.clientAuthMethod, AUTH_METHOD_LABEL)}
            </Descriptions.Item>
            <Descriptions.Item label="登录能力">
              <TagList values={client.grantTypes} labels={GRANT_TYPE_LABEL} />
            </Descriptions.Item>
            <Descriptions.Item label="可访问内容">
              <TagList values={client.scopes} labels={SCOPE_LABEL} />
            </Descriptions.Item>
            <Descriptions.Item label="要求浏览器安全校验">
              {boolTag(client.requirePkce, 'green')}
            </Descriptions.Item>
            <Descriptions.Item label="需要授权确认">
              {boolTag(client.requireConsent, 'gold')}
            </Descriptions.Item>
            <Descriptions.Item label="内部可信客户端">
              {boolTag(client.trustedFirstParty)}
            </Descriptions.Item>
            <Descriptions.Item label="访问令牌有效期">
              {formatTtl(client.accessTokenTtlSec)}
            </Descriptions.Item>
            <Descriptions.Item label="刷新令牌有效期">
              {formatTtl(client.refreshTokenTtlSec)}
            </Descriptions.Item>
            <Descriptions.Item label="状态">{statusTag(client.status)}</Descriptions.Item>
            <Descriptions.Item label="有效回调数">{client.activeRedirectUriCount ?? 0}</Descriptions.Item>
            <Descriptions.Item label="有效密钥数">{client.activeSecretCount ?? 0}</Descriptions.Item>
            <Descriptions.Item label="创建时间">{formatTime(client.createTime)}</Descriptions.Item>
            <Descriptions.Item label="更新时间">{formatTime(client.updateTime)}</Descriptions.Item>
            {scopeCatalog.length ? (
              <Descriptions.Item label="可访问内容说明">
                <ScopeCatalog values={scopeCatalog} />
              </Descriptions.Item>
            ) : null}
            <Descriptions.Item label="令牌签名方式">
              <TagList values={capabilities?.signingAlgorithms} labels={SIGNING_ALGORITHM_LABEL} />
            </Descriptions.Item>
          </Descriptions>

          <Descriptions {...descriptionProps} title="回调地址与密钥">
            <Descriptions.Item label="回调地址">
              <RedirectUriList client={client} />
            </Descriptions.Item>
            <Descriptions.Item label="客户端密钥">
              <SecretSummary client={client} />
            </Descriptions.Item>
          </Descriptions>

          <Descriptions {...descriptionProps} title="认证服务地址">
            <Descriptions.Item label="签发方地址">
              <CopyText value={endpoints.issuer} />
            </Descriptions.Item>
            <Descriptions.Item label="服务配置">
              <CopyText value={endpoints.discoveryUrl} />
            </Descriptions.Item>
            <Descriptions.Item label="发起授权">
              <CopyText value={endpoints.authorizationEndpoint} />
            </Descriptions.Item>
            <Descriptions.Item label="换取令牌">
              <CopyText value={endpoints.tokenEndpoint} />
            </Descriptions.Item>
            <Descriptions.Item label="读取用户信息">
              <CopyText value={endpoints.userinfoEndpoint} />
            </Descriptions.Item>
            <Descriptions.Item label="公钥地址">
              <CopyText value={endpoints.jwksUri} />
            </Descriptions.Item>
            <Descriptions.Item label="注销令牌">
              <CopyText value={endpoints.revocationEndpoint} />
            </Descriptions.Item>
          </Descriptions>
        </Space>
      )}
    </Drawer>
  );
}
