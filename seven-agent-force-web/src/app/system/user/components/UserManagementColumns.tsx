'use client';

import React from 'react';
import { Button, Tag, Space, Avatar, Dropdown, Modal } from 'antd';
import {
  EditOutlined,
  DeleteOutlined,
  KeyOutlined,
  TeamOutlined,
  ApartmentOutlined,
  UserOutlined,
  EyeOutlined,
  MoreOutlined,
  PartitionOutlined,
  IdcardOutlined,
} from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';
import {
  getUserStatusLabel,
  USER_STATUS_DISABLED,
  USER_STATUS_OPTIONS,
} from '@/lib/userStatus';

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

const resolveUserDisplayList = (record: API.UserVO, primaryKey: 'allRoleNames' | 'allOrgNames') => {
  const fallbackKey = primaryKey === 'allRoleNames' ? 'fallbackRoleNames' : 'fallbackOrgNames';
  const primary = normalizeDisplayList(record?.[primaryKey]);
  if (primary.length > 0) {
    return primary;
  }
  return normalizeDisplayList((record as API.UserVO & Record<string, unknown>)?.[fallbackKey]);
};

interface UserManagementColumnsProps {
  genderLabelMap: Map<number, string>;
  setCurrentRow: (row: API.UserVO) => void;
  setUpdateModalVisible: (visible: boolean) => void;
  canViewUserDetail: boolean;
  canUpdateUser: boolean;
  canDeleteUser: boolean;
  canChangeUserStatus: boolean;
  canResetUserPassword: boolean;
  handleDelete: (row: API.UserVO) => void;
  onResetPassword: (record: API.UserVO) => void;
  onAssignRoles: (record: API.UserVO) => void;
  onAssignOrgs: (record: API.UserVO) => void;
  onAssignDepts: (record: API.UserVO) => void;
  onAssignPosts: (record: API.UserVO) => void;
  onChangeStatus: (record: API.UserVO, status: number) => void | Promise<void>;
  onShowDetail: (record: API.UserVO) => void;
}

export const UserManagementColumns = ({
  genderLabelMap,
  setCurrentRow,
  setUpdateModalVisible,
  canViewUserDetail,
  canUpdateUser,
  canDeleteUser,
  canChangeUserStatus,
  canResetUserPassword,
  handleDelete,
  onResetPassword,
  onAssignRoles,
  onAssignOrgs,
  onAssignDepts,
  onAssignPosts,
  onChangeStatus,
  onShowDetail,
}: UserManagementColumnsProps): ProColumns<API.UserVO>[] => [
  {
    title: 'ID',
    dataIndex: 'id',
    key: 'id',
    width: 60,
    search: false,
  },
  {
    title: '头像',
    dataIndex: 'avatar',
    key: 'avatar',
    width: 60,
    search: false,
    render: (_, record) => (
      <Avatar
        src={record.avatar}
        icon={<UserOutlined />}
        size="small"
        style={{ backgroundColor: '#1890ff' }}
      >
        {record.nickname?.charAt(0) || record.username?.charAt(0)}
      </Avatar>
    ),
  },
  {
    title: '用户名',
    dataIndex: 'username',
    key: 'username',
    width: 100,
    render: (_, record) => <span style={{ fontWeight: 500 }}>{record.username}</span>,
  },
  {
    title: '昵称',
    dataIndex: 'nickname',
    key: 'nickname',
    width: 100,
  },
  {
    title: '邮箱',
    dataIndex: 'email',
    key: 'email',
    width: 140,
    ellipsis: true,
  },
  {
    title: '手机号',
    dataIndex: 'userPhone',
    key: 'userPhone',
    width: 110,
  },
  {
    title: '性别',
    dataIndex: 'userGender',
    key: 'userGender',
    width: 60,
    render: (_, record) => genderLabelMap.get(Number(record.userGender ?? 0)) ?? '未知',
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 80,
    valueEnum: Object.fromEntries(
      USER_STATUS_OPTIONS.map(({ value, label, proStatus }) => [
        String(value),
        { text: label, status: proStatus },
      ]),
    ),
  },
  {
    title: '角色',
    dataIndex: 'allRoleNames',
    key: 'allRoleNames',
    width: 120,
    search: false,
    render: (_, record) => {
      const roleNames = resolveUserDisplayList(record, 'allRoleNames');
      if (roleNames.length === 0) {
        return <Tag color="default">未分配</Tag>;
      }
      return (
        <Space wrap>
          {roleNames.slice(0, 2).map((role, index) => (
            <Tag key={index} color="blue">
              {role}
            </Tag>
          ))}
          {roleNames.length > 2 && (
            <Tag color="default">
              +{roleNames.length - 2}
            </Tag>
          )}
        </Space>
      );
    },
  },
  {
    title: '组织',
    dataIndex: 'allOrgNames',
    key: 'allOrgNames',
    width: 120,
    search: false,
    render: (_, record) => {
      const orgNames = resolveUserDisplayList(record, 'allOrgNames');
      if (orgNames.length === 0) {
        return <Tag color="default">未分配</Tag>;
      }
      return (
        <Space wrap>
          {orgNames.slice(0, 2).map((org, index) => (
            <Tag key={index} color="green">
              {org}
            </Tag>
          ))}
          {orgNames.length > 2 && (
            <Tag color="default">
              +{orgNames.length - 2}
            </Tag>
          )}
        </Space>
      );
    },
  },
  {
    title: '创建时间',
    dataIndex: 'createTime',
    key: 'createTime',
    valueType: 'dateTime',
    search: false,
    width: 160,
  },
  {
    title: '操作',
    key: 'action',
    fixed: 'right',
    width: 210,
    search: false,
    render: (_, record) => {
      const moreItems = [];

      if (canResetUserPassword) {
        moreItems.push({
          key: 'reset-password',
          icon: <KeyOutlined />,
          label: '重置密码',
          onClick: () => onResetPassword(record),
        });
      }

      if (canUpdateUser) {
        moreItems.push({
          key: 'assign-roles',
          icon: <TeamOutlined />,
          label: '分配角色',
          onClick: () => onAssignRoles(record),
        });
        moreItems.push({
          key: 'assign-orgs',
          icon: <ApartmentOutlined />,
          label: '分配组织',
          onClick: () => onAssignOrgs(record),
        });
        moreItems.push({
          key: 'assign-depts',
          icon: <PartitionOutlined />,
          label: '分配部门',
          onClick: () => onAssignDepts(record),
        });
        moreItems.push({
          key: 'assign-posts',
          icon: <IdcardOutlined />,
          label: '分配岗位',
          onClick: () => onAssignPosts(record),
        });
      }

      if (canChangeUserStatus || canDeleteUser) {
        if (moreItems.length > 0) {
          moreItems.push({
            type: 'divider' as const,
          });
        }

        if (canChangeUserStatus) {
          USER_STATUS_OPTIONS.filter((option) => option.value !== record.status).forEach((option) => {
            moreItems.push({
              key: `status-${option.value}`,
              label: `设为${option.label}`,
              danger: option.value === USER_STATUS_DISABLED,
              onClick: () =>
                Modal.confirm({
                  title: `设为${option.label}`,
                  content: `确定要将用户 "${record.nickname || record.username}" 状态从 ${getUserStatusLabel(record.status)} 改为 ${option.label} 吗？`,
                  okText: '确定',
                  cancelText: '取消',
                  okButtonProps: option.value === USER_STATUS_DISABLED ? { danger: true } : undefined,
                  onOk: () => onChangeStatus(record, option.value),
                }),
            });
          });
        }

        if (canDeleteUser) {
          moreItems.push({
            key: 'delete',
            icon: <DeleteOutlined />,
            label: '删除',
            danger: true,
            onClick: () =>
              Modal.confirm({
                title: '删除用户',
                content: '确定要删除该用户吗？删除后不可恢复！',
                okText: '确定删除',
                cancelText: '取消',
                okButtonProps: { danger: true },
                onOk: () => handleDelete(record),
              }),
          });
        }
      }

      return (
        <Space size="small" wrap={false}>
          {canViewUserDetail ? (
            <Button
              type="link"
              size="small"
              icon={<EyeOutlined />}
              onClick={() => onShowDetail(record)}
            >
              详情
            </Button>
          ) : null}

          {canUpdateUser ? (
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => {
                setCurrentRow(record);
                setUpdateModalVisible(true);
              }}
            >
              编辑
            </Button>
          ) : null}

          {moreItems.length > 0 ? (
            <Dropdown menu={{ items: moreItems }} trigger={['click']}>
              <Button type="link" size="small" icon={<MoreOutlined />}>
                更多
              </Button>
            </Dropdown>
          ) : null}
        </Space>
      );
    },
  },
];
