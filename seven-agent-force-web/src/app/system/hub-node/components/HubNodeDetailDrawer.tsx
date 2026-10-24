import { useMemo, useState } from 'react';
import { Alert, Button, Descriptions, Drawer, Space, Spin, Tabs, Tag, Typography } from 'antd';
import type { TabsProps } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { canManageHubNode, getHubNode, type HubNodeRecord } from '@/api/hubNodeController';
import { connectionStatusTag, formatTime, nodeStatusTag } from '../constants';
import HubNodeFederationTab from './HubNodeFederationTab';
import HubNodePolicyTab from './HubNodePolicyTab';
import HubNodeUsersTab from './HubNodeUsersTab';
import styles from '../hubNode.module.css';

export interface HubNodeDetailPermissions {
  canListUsers: boolean;
  canQueryUser: boolean;
  canChangeUserStatus: boolean;
  canListSessions: boolean;
  canRevokeSessions: boolean;
  canQueryPolicy: boolean;
  canApplyPolicy: boolean;
  canQueryFederation: boolean;
  canApplyFederation: boolean;
}

interface Props {
  open: boolean;
  node: HubNodeRecord | null;
  permissions: HubNodeDetailPermissions;
  onClose: () => void;
}

export default function HubNodeDetailDrawer({ open, node, permissions, onClose }: Props) {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState('overview');
  const nodeCode = node?.nodeCode || '';
  const queryPrefix = useMemo(() => ['hub-node', nodeCode], [nodeCode]);
  const detailQuery = useQuery({
    queryKey: [...queryPrefix, 'detail'],
    queryFn: () => getHubNode(nodeCode),
    enabled: open && Boolean(nodeCode),
    retry: 0,
  });

  const closeAndClear = () => {
    queryClient.removeQueries({ queryKey: queryPrefix });
    setActiveTab('overview');
    onClose();
  };
  const detail = detailQuery.data || node;
  const manageable = canManageHubNode(detail);

  const overview = detail ? (
    <div data-testid="hub-node-overview">
      <Descriptions size="small" column={{ xs: 1, sm: 2 }}>
        <Descriptions.Item label="Node 编码">
          <Typography.Text copyable>{detail.nodeCode}</Typography.Text>
        </Descriptions.Item>
        <Descriptions.Item label="运行状态">{nodeStatusTag(detail.status)}</Descriptions.Item>
        <Descriptions.Item label="发现方式">{detail.discoveryType}</Descriptions.Item>
        <Descriptions.Item label="连接状态">{connectionStatusTag(detail.connectionStatus)}</Descriptions.Item>
        <Descriptions.Item label={detail.discoveryType === 'CONSUL' ? '服务名' : '管理地址'} span={{ xs: 1, sm: 2 }}>
          {detail.discoveryType === 'CONSUL' ? detail.serviceName || '-' : detail.managementBaseUrl || '-'}
        </Descriptions.Item>
        <Descriptions.Item label="Hub Issuer" span={{ xs: 1, sm: 2 }}>{detail.hubIssuer || '-'}</Descriptions.Item>
        <Descriptions.Item label="连接版本">{detail.connectionVersion || '-'}</Descriptions.Item>
        <Descriptions.Item label="最近健康时间">{formatTime(detail.lastHealthyAt)}</Descriptions.Item>
        <Descriptions.Item label="Issuer 锁定时间">{formatTime(detail.issuerLockedAt)}</Descriptions.Item>
        <Descriptions.Item label="更新时间">{formatTime(detail.updatedAt)}</Descriptions.Item>
      </Descriptions>
      <div className={styles.capabilityStrip}>
        <Typography.Text type="secondary">可管理能力</Typography.Text>
        <Space size={[4, 6]} wrap>
          {detail.capabilities.length ? detail.capabilities.map((item) => <Tag key={item}>{item}</Tag>) : <span>-</span>}
        </Space>
      </div>
      {detail.lastConnectionError ? (
        <Alert
          type="error"
          showIcon
          title="最近连接异常"
          description={
            <Space orientation="vertical" size={2}>
              <span>{detail.lastConnectionError}</span>
              {detail.lastConnectionTraceId ? <Typography.Text type="secondary">Trace ID: {detail.lastConnectionTraceId}</Typography.Text> : null}
            </Space>
          }
        />
      ) : null}
    </div>
  ) : null;

  const items: TabsProps['items'] = [{ key: 'overview', label: '概览', children: overview }];
  if (manageable && permissions.canListUsers) {
    items.push({
      key: 'users',
      label: '用户',
      children: <HubNodeUsersTab nodeCode={nodeCode} canQueryUser={permissions.canQueryUser} canChangeStatus={permissions.canChangeUserStatus} canListSessions={permissions.canListSessions} canRevokeSessions={permissions.canRevokeSessions} />,
    });
  }
  if (manageable && permissions.canQueryPolicy) {
    items.push({ key: 'policy', label: '登录策略', children: <HubNodePolicyTab nodeCode={nodeCode} canApply={permissions.canApplyPolicy} /> });
  }
  if (manageable && permissions.canQueryFederation) {
    items.push({ key: 'federation', label: '联邦连接', children: <HubNodeFederationTab nodeCode={nodeCode} nodeName={detail?.nodeName || node?.nodeName || nodeCode} canProvision={permissions.canApplyFederation} /> });
  }

  return (
    <Drawer
      open={open}
      title={detail ? `${detail.nodeName} · Node 管理` : 'Node 管理'}
      size="min(100vw, 960px)"
      destroyOnHidden
      onClose={closeAndClear}
      afterOpenChange={(visible) => {
        if (!visible) queryClient.removeQueries({ queryKey: queryPrefix });
      }}
      extra={<Button aria-label="刷新 Node 详情" icon={<ReloadOutlined />} loading={detailQuery.isFetching} onClick={() => detailQuery.refetch()} />}
    >
      {detailQuery.isLoading ? <div className={styles.centered}><Spin /></div> : null}
      {detailQuery.isError ? <Alert type="error" showIcon title={(detailQuery.error as Error)?.message || 'Node 详情加载失败'} /> : null}
      {detail && !manageable ? (
        <Alert
          type="warning"
          showIcon
          title="Node 返回了未知状态"
          description="为避免在协议漂移时执行错误操作，远端管理标签已关闭。"
          style={{ marginBottom: 16 }}
        />
      ) : null}
      {detail ? <Tabs activeKey={activeTab} onChange={setActiveTab} destroyOnHidden items={items} /> : null}
    </Drawer>
  );
}
