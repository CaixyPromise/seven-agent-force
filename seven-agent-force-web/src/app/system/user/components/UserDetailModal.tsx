'use client';

import React, { useState } from 'react';
import { Modal, Descriptions, Avatar, Tag, Space, Tabs } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { getUserDetail, getUserRoleIds, getUserOrgIds } from '@/api/userController';
import { getRoleList } from '@/api/sysRoleController';
import { getOrgTree } from '@/api/sysOrgController';
import { useDictValueOnly } from '@/hooks/useDictValue';
import { resolveUserGenderLabel, USER_GENDER_DICT_CODE } from '@/lib/userGender';
import { hasId, toApiIdParam, toIdString } from '@/lib/apiId';
import { getUserStatusColor, getUserStatusLabel } from '@/lib/userStatus';
import { usePermissionFlags } from '@/hooks/auth';
import { TEMPORARY_PERMISSION_PERMISSIONS, USER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { EffectiveAccessTab } from './EffectiveAccessTab';
import { TemporaryPermissionTab } from './TemporaryPermissionTab';

const normalizeDisplayList = (value: unknown): string[] => {
  if (Array.isArray(value)) {
    return value.map((item) => String(item).trim()).filter(Boolean);
  }
  if (typeof value === 'string') {
    return value
      .split(/[,\uff0c]/)
      .map((item) => item.trim())
      .filter(Boolean);
  }
  return [];
};

const resolveUserDisplayList = (
  user: API.UserVO | undefined,
  primaryKey: 'allRoleNames' | 'allOrgNames',
) => {
  const fallbackKey = primaryKey === 'allRoleNames' ? 'fallbackRoleNames' : 'fallbackOrgNames';
  const primary = normalizeDisplayList(user?.[primaryKey]);
  if (primary.length > 0) {
    return primary;
  }
  return normalizeDisplayList((user as API.UserVO & Record<string, unknown> | undefined)?.[fallbackKey]);
};

const flattenOrgTree = (orgs: API.SysOrg[] = [], target = new Map<string, string>()) => {
  orgs.forEach((org) => {
    if (hasId(org.id) && org.name) {
      target.set(toIdString(org.id), org.name);
    }
    if (org.children?.length) {
      flattenOrgTree(org.children, target);
    }
  });
  return target;
};

interface UserDetailModalProps {
  visible: boolean;
  userId?: React.Key;
  onCancel: () => void;
}

export const UserDetailModal: React.FC<UserDetailModalProps> = ({
  visible,
  userId,
  onCancel,
}) => {
  const [activeTab, setActiveTab] = useState('basic');
  const accessPermissions = usePermissionFlags({
    canQuery: USER_PERMISSIONS.ACCESS_QUERY,
    canExplain: USER_PERMISSIONS.ACCESS_EXPLAIN,
  });
  const temporaryPermissions = usePermissionFlags({
    canQuery: TEMPORARY_PERMISSION_PERMISSIONS.QUERY,
    canGrant: TEMPORARY_PERMISSION_PERMISSIONS.GRANT,
    canExtend: TEMPORARY_PERMISSION_PERMISSIONS.EXTEND,
    canRevoke: TEMPORARY_PERMISSION_PERMISSIONS.REVOKE,
  });

  // 获取用户详情
  const { data: userDetail, isLoading } = useQuery({
    queryKey: ['userDetail', userId],
    queryFn: async () => {
      const resolvedUserId = toApiIdParam(userId);
      const [detailResult, roleIdsResult, orgIdsResult, roleListResult, orgTreeResult] =
        await Promise.allSettled([
          getUserDetail({ id: String(resolvedUserId) }),
          getUserRoleIds({ id: String(resolvedUserId) }),
          getUserOrgIds({ id: String(resolvedUserId) }),
          getRoleList(),
          getOrgTree(),
        ]);

      if (detailResult.status !== 'fulfilled') {
        throw detailResult.reason;
      }

      const detailUser = detailResult.value?.data || {};
      const currentRoleNames = resolveUserDisplayList(detailUser, 'allRoleNames');
      const currentOrgNames = resolveUserDisplayList(detailUser, 'allOrgNames');

      const roleNameMap = new Map<string, string>();
      if (roleListResult.status === 'fulfilled') {
        (roleListResult.value?.data || []).forEach((role) => {
          if (hasId(role.id) && role.name) {
            roleNameMap.set(toIdString(role.id), role.name);
          }
        });
      }

      const orgNameMap =
        orgTreeResult.status === 'fulfilled'
          ? flattenOrgTree(orgTreeResult.value?.data || [])
          : new Map<string, string>();

      const roleNames =
        currentRoleNames.length > 0
          ? currentRoleNames
          : roleIdsResult.status === 'fulfilled'
          ? (((roleIdsResult.value?.data as unknown[]) ?? [])
              .filter(hasId)
              .map((id) => roleNameMap.get(toIdString(id)))
              .filter((name): name is string => Boolean(name)))
          : [];

      const orgNames =
        currentOrgNames.length > 0
          ? currentOrgNames
          : orgIdsResult.status === 'fulfilled'
          ? (((orgIdsResult.value?.data as unknown[]) ?? [])
              .filter(hasId)
              .map((id) => orgNameMap.get(toIdString(id)))
              .filter((name): name is string => Boolean(name)))
          : [];

      return {
        ...detailResult.value,
        data: {
          ...detailUser,
          allRoleNames: roleNames,
          allOrgNames: orgNames,
          fallbackRoleNames: roleNames,
          fallbackOrgNames: orgNames,
        },
      };
    },
    enabled: visible && !!userId,
  });

  const user = userDetail?.data;
  const roleNames = resolveUserDisplayList(user, 'allRoleNames');
  const orgNames = resolveUserDisplayList(user, 'allOrgNames');
  const genderItems = useDictValueOnly(USER_GENDER_DICT_CODE);
  const resolvedGenderLabel = resolveUserGenderLabel(genderItems, user?.userGender);

  const basicContent = isLoading ? (
    <div style={{ textAlign: 'center', padding: '40px 0' }}>加载中...</div>
  ) : user ? (
    <Descriptions column={2} bordered>
      <Descriptions.Item label="头像">
        <Avatar
          src={user.avatar}
          icon={<UserOutlined />}
          size="large"
          style={{ backgroundColor: '#1890ff' }}
        >
          {user.nickname?.charAt(0) || user.username?.charAt(0)}
        </Avatar>
      </Descriptions.Item>
      <Descriptions.Item label="用户ID">{user.id}</Descriptions.Item>
      <Descriptions.Item label="用户名">{user.username}</Descriptions.Item>
      <Descriptions.Item label="昵称">{user.nickname}</Descriptions.Item>
      <Descriptions.Item label="邮箱">{user.email}</Descriptions.Item>
      <Descriptions.Item label="手机号">{user.userPhone}</Descriptions.Item>
      <Descriptions.Item label="性别">{resolvedGenderLabel}</Descriptions.Item>
      <Descriptions.Item label="状态">
        <Tag color={getUserStatusColor(user.status)}>{getUserStatusLabel(user.status)}</Tag>
      </Descriptions.Item>
      <Descriptions.Item label="角色" span={2}>
        {roleNames.length > 0 ? (
          <Space wrap>
            {roleNames.map((role) => <Tag key={role} color="blue">{role}</Tag>)}
          </Space>
        ) : <Tag color="default">未分配</Tag>}
      </Descriptions.Item>
      <Descriptions.Item label="组织" span={2}>
        {orgNames.length > 0 ? (
          <Space wrap>
            {orgNames.map((org) => <Tag key={org} color="green">{org}</Tag>)}
          </Space>
        ) : <Tag color="default">未分配</Tag>}
      </Descriptions.Item>
      <Descriptions.Item label="创建时间">{user.createTime}</Descriptions.Item>
      <Descriptions.Item label="更新时间">{user.updateTime}</Descriptions.Item>
    </Descriptions>
  ) : (
    <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>用户信息不存在</div>
  );

  const tabs = [
    { key: 'basic', label: '基本信息', children: basicContent },
    ...(accessPermissions.canQuery && hasId(userId)
      ? [{
          key: 'effective-access',
          label: '有效权限',
          children: (
            <EffectiveAccessTab
              userId={toApiIdParam(userId)}
              active={visible && activeTab === 'effective-access'}
              canExplain={accessPermissions.canExplain}
            />
          ),
        }]
      : []),
    ...(temporaryPermissions.canQuery && hasId(userId)
      ? [{
          key: 'temporary-permissions',
          label: '临时权限',
          children: (
            <TemporaryPermissionTab
              userId={toIdString(userId)}
              active={visible && activeTab === 'temporary-permissions'}
              canGrant={temporaryPermissions.canGrant}
              canExtend={temporaryPermissions.canExtend}
              canRevoke={temporaryPermissions.canRevoke}
            />
          ),
        }]
      : []),
  ];

  return (
    <Modal
      title="用户详情"
      open={visible}
      onCancel={() => {
        setActiveTab('basic');
        onCancel();
      }}
      footer={null}
      width={1080}
      mask={{ closable: false }}
    >
      <Tabs activeKey={activeTab} onChange={setActiveTab} destroyOnHidden items={tabs} />
    </Modal>
  );
};
