'use client';

import React, { useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Descriptions,
  Drawer,
  Empty,
  Input,
  Select,
  Space,
  Spin,
  Statistic,
  Table,
  Tag,
  Typography,
} from 'antd';
import { useQuery } from '@tanstack/react-query';
import {
  fetchEffectiveAccess,
  fetchPermissionExplanation,
  type ApiIdentifier,
  type EffectivePermission,
  type PermissionGrantChain,
} from '@/services/admin/accessExplain';

const { Text, Paragraph } = Typography;

const scopeLabels: Record<string, string> = {
  ALL: '全部数据',
  CUSTOM: '指定部门',
  DEPT: '本部门',
  DEPT_AND_CHILD: '本部门及下级部门',
  SELF: '仅本人',
  NONE: '无业务数据',
};

const sourceLabels: Record<string, string> = {
  DIRECT_USER: '用户直授角色',
  POST: '岗位继承角色',
  ROLE_DIRECT: '角色直接权限',
  MENU_DERIVED: '菜单派生权限',
  TEMPORARY: '用户临时权限',
};

const reasonLabels: Record<string, string> = {
  AUTHORIZATION_ROOT_BYPASS: '安全根全局放行',
  DIRECT_ROLE_PERMISSION: '来自用户直授角色',
  POST_ROLE_PERMISSION: '来自岗位继承角色',
  MENU_DERIVED_PERMISSION: '来自角色菜单',
  TEMPORARY_PERMISSION_ACTIVE: '有效的临时权限',
  WILDCARD_PERMISSION_MATCH: '由通配权限匹配',
  USER_INACTIVE: '用户已停用',
  PERMISSION_NOT_GRANTED: '没有任何有效授权来源',
  ROLE_DISABLED: '授权角色已停用',
  PERMISSION_DISABLED: '权限资源已停用',
  MENU_DISABLED: '授权菜单已停用',
  TEMPORARY_PERMISSION_EXPIRED: '临时权限已过期',
  FEATURE_DISABLED: '所属功能当前不可用',
};

interface EffectiveAccessTabProps {
  userId: ApiIdentifier;
  active: boolean;
  canExplain: boolean;
}

export const EffectiveAccessTab: React.FC<EffectiveAccessTabProps> = ({
  userId,
  active,
  canExplain,
}) => {
  const [current, setCurrent] = useState(1);
  const [size, setSize] = useState(10);
  const [keyword, setKeyword] = useState('');
  const [sourceType, setSourceType] = useState<string>();
  const [effective, setEffective] = useState<boolean>();
  const [explainCode, setExplainCode] = useState<string>();

  const accessQuery = useQuery({
    queryKey: ['userEffectiveAccess', userId, current, size, keyword, sourceType, effective],
    queryFn: () => fetchEffectiveAccess(userId, { current, size, keyword, sourceType, effective }),
    enabled: active,
  });
  const explanationQuery = useQuery({
    queryKey: ['userPermissionExplanation', userId, explainCode],
    queryFn: () => fetchPermissionExplanation(userId, explainCode || ''),
    enabled: Boolean(explainCode),
  });

  const access = accessQuery.data;
  const columns = useMemo(
    () => [
      {
        title: '权限标识',
        dataIndex: 'permissionCode',
        key: 'permissionCode',
        render: (value: string, record: EffectivePermission) => (
          <Space orientation="vertical" size={0}>
            <Text code copyable>{value}</Text>
            <Text type="secondary">{record.permissionName || '未命名权限'}</Text>
          </Space>
        ),
      },
      {
        title: '状态',
        key: 'effective',
        width: 130,
        render: (_: unknown, record: EffectivePermission) => (
          <Space orientation="vertical" size={2}>
            <Tag color={record.effective ? 'success' : 'error'}>
              {record.effective ? '当前有效' : '当前无效'}
            </Tag>
            {record.featureCode && !record.featureEnabled ? <Tag color="warning">Feature 已过滤</Tag> : null}
          </Space>
        ),
      },
      {
        title: '来源数',
        key: 'sourceCount',
        width: 90,
        render: (_: unknown, record: EffectivePermission) => record.grants.length,
      },
      {
        title: '操作',
        key: 'actions',
        width: 90,
        render: (_: unknown, record: EffectivePermission) =>
          canExplain ? (
            <Button type="link" size="small" onClick={() => setExplainCode(record.permissionCode)}>
              解释
            </Button>
          ) : null,
      },
    ],
    [canExplain],
  );

  if (accessQuery.isLoading && !access) {
    return <Spin description="正在计算有效权限" />;
  }
  if (accessQuery.isError) {
    return <Alert type="error" showIcon title="有效权限加载失败" description="请确认权限或稍后重试。" />;
  }
  if (!access) {
    return <Empty description="暂无有效权限信息" />;
  }

  return (
    <Space orientation="vertical" size={16} style={{ width: '100%' }}>
      {access.authorizationRoot ? (
        <Alert
          type="warning"
          showIcon
          title="该用户是授权安全根"
          description="安全根由系统稳定身份识别，对权限检查拥有全局旁路；下方列表只展示已配置的授权关系。"
        />
      ) : null}

      <Space wrap size={12}>
        <Card size="small"><Statistic title="当前有效" value={access.permissionSummary.effectiveCount} /></Card>
        <Card size="small"><Statistic title="Feature 过滤" value={access.permissionSummary.filteredCount} /></Card>
        <Card size="small"><Statistic title="临时权限" value={access.permissionSummary.temporaryCount} /></Card>
      </Space>

      <Card size="small" title="最终数据范围">
        <Descriptions size="small" column={2}>
          <Descriptions.Item label="范围">
            <Tag color={access.dataScope.scopeType === 'ALL' ? 'green' : 'blue'}>
              {scopeLabels[access.dataScope.scopeType] || access.dataScope.scopeType}
            </Tag>
          </Descriptions.Item>
          <Descriptions.Item label="可见部门数">{access.dataScope.deptIds.length}</Descriptions.Item>
          <Descriptions.Item label="可见组织数">{access.dataScope.orgIds.length}</Descriptions.Item>
          <Descriptions.Item label="贡献角色">
            <Space wrap>
              {access.dataScope.contributors.map((item) => (
                <Tag key={String(item.roleId)} color={item.winning ? 'green' : 'default'}>
                  {item.roleCode} · {scopeLabels[item.declaredScopeType] || item.declaredScopeType}
                </Tag>
              ))}
            </Space>
          </Descriptions.Item>
        </Descriptions>
        <Paragraph type="secondary" style={{ margin: '12px 0 0' }}>
          此处用于解释界面可见范围，实际数据访问始终以后端校验为准。
        </Paragraph>
      </Card>

      <Card size="small" title="角色来源">
        <Space wrap>
          {access.roleSources.map((role, index) => (
            <Tag key={`${String(role.roleId)}-${role.roleAssignmentSource}-${String(role.post?.postId || index)}`} color={role.roleStatus === 0 ? 'blue' : 'error'}>
              {role.roleName}（{role.roleCode}） · {sourceLabels[role.roleAssignmentSource]}
              {role.post ? ` · ${role.post.postName}` : ''}
            </Tag>
          ))}
        </Space>
      </Card>

      <Space wrap>
        <Input.Search
          allowClear
          placeholder="搜索权限标识或名称"
          style={{ width: 280 }}
          onSearch={(value) => { setCurrent(1); setKeyword(value.trim()); }}
        />
        <Select
          allowClear
          placeholder="授权来源"
          style={{ width: 180 }}
          value={sourceType}
          onChange={(value) => { setCurrent(1); setSourceType(value); }}
          options={[
            { value: 'ROLE_DIRECT', label: '角色直接权限' },
            { value: 'MENU_DERIVED', label: '菜单派生权限' },
            { value: 'TEMPORARY', label: '临时权限' },
            { value: 'DIRECT_USER', label: '用户直授角色' },
            { value: 'POST', label: '岗位继承角色' },
          ]}
        />
        <Select
          allowClear
          placeholder="有效状态"
          style={{ width: 140 }}
          value={effective}
          onChange={(value) => { setCurrent(1); setEffective(value); }}
          options={[{ value: true, label: '当前有效' }, { value: false, label: '当前无效' }]}
        />
      </Space>

      <Table<EffectivePermission>
        rowKey={(record) => record.permissionCode}
        size="small"
        columns={columns}
        dataSource={access.permissions.records}
        loading={accessQuery.isFetching}
        pagination={{
          current: access.permissions.current,
          pageSize: access.permissions.size,
          total: access.permissions.total,
          showSizeChanger: true,
          onChange: (nextCurrent, nextSize) => { setCurrent(nextCurrent); setSize(nextSize); },
        }}
        expandable={{ expandedRowRender: (record) => <GrantChains chains={record.grants} /> }}
      />

      <Drawer
        title={`权限解释：${explainCode || ''}`}
        open={Boolean(explainCode)}
        size="large"
        onClose={() => setExplainCode(undefined)}
        destroyOnHidden
      >
        {explanationQuery.isLoading ? <Spin /> : explanationQuery.data ? (
          <Space orientation="vertical" size={16} style={{ width: '100%' }}>
            <Alert
              type={explanationQuery.data.decision === 'ALLOW' ? 'success' : 'error'}
              showIcon
              title={explanationQuery.data.decision === 'ALLOW' ? '允许访问' : '拒绝访问'}
              description={reasonLabels[explanationQuery.data.reasonCode] || explanationQuery.data.reasonCode}
            />
            {explanationQuery.data.matchedPermissionCodes.length > 0 ? (
              <Descriptions size="small" column={1} bordered>
                <Descriptions.Item label="匹配权限">
                  {explanationQuery.data.matchedPermissionCodes.map((code) => <Tag key={code}>{code}</Tag>)}
                </Descriptions.Item>
                <Descriptions.Item label="评估时间">{explanationQuery.data.evaluatedAt}</Descriptions.Item>
              </Descriptions>
            ) : null}
            <GrantChains chains={explanationQuery.data.chains} />
          </Space>
        ) : explanationQuery.isError ? <Alert type="error" title="权限解释加载失败" /> : null}
      </Drawer>
    </Space>
  );
};

const GrantChains: React.FC<{ chains: PermissionGrantChain[] }> = ({ chains }) => {
  if (chains.length === 0) {
    return <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有匹配的授权链" />;
  }
  return (
    <Space orientation="vertical" size={8} style={{ width: '100%' }}>
      {chains.map((chain, index) => (
        <Card key={`${chain.permissionGrantSource}-${String(chain.roleId || '')}-${String(chain.menuId || '')}-${index}`} size="small">
          <Space wrap>
            <Tag color={chain.active ? 'success' : 'error'}>{chain.active ? '有效' : '无效'}</Tag>
            <Tag>{sourceLabels[chain.permissionGrantSource] || chain.permissionGrantSource}</Tag>
            {chain.roleCode ? <Text>角色：{chain.roleName || chain.roleCode}（{chain.roleCode}）</Text> : null}
            {chain.post ? <Text>岗位：{chain.post.postName}</Text> : null}
            {chain.menuPath ? <Text>菜单：{chain.menuPath}</Text> : null}
            {chain.expireAt ? <Text>过期：{chain.expireAt}</Text> : null}
          </Space>
          <div><Text type={chain.active ? 'secondary' : 'danger'}>{reasonLabels[chain.reasonCode] || chain.reasonCode}</Text></div>
        </Card>
      ))}
    </Space>
  );
};
