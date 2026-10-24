'use client';

import React, { useState, useRef } from 'react';
import { Button, Input, Modal, message } from 'antd';
import { PlusOutlined, DeleteOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType } from '@ant-design/pro-components';
import { useMutation } from '@tanstack/react-query';
import {
  listUsers,
  deleteUser,
  resetUserPassword,
  updateUserStatus,
  getUserDetail,
  getUserRoleIds,
  getUserOrgIds,
  getUserDeptIds,
  getUserPostIds,
} from '@/api/userController';
import { getRoleList } from '@/api/sysRoleController';
import { getOrgTree } from '@/api/sysOrgController';
import { UserManagementColumns } from './components/UserManagementColumns';
import { CreateUserModal } from './components/CreateUserModal';
import { EditUserModal } from './components/EditUserModal';
import { UserRoleAssign } from './components/UserRoleAssign';
import { UserOrgAssign } from './components/UserOrgAssign';
import { UserDeptAssign } from './components/UserDeptAssign';
import { UserPostAssign } from './components/UserPostAssign';
import { UserDetailModal } from './components/UserDetailModal';
import { useDictValueOnly } from '@/hooks/useDictValue';
import { buildUserGenderLabelMap, USER_GENDER_DICT_CODE } from '@/lib/userGender';
import { hasId, toApiIdList, toApiIdParam } from '@/lib/apiId';
import { usePermissionFlags } from '@/hooks/auth';
import { USER_PERMISSIONS } from '@/lib/auth/permissionCodes';

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

const flattenOrgTree = (orgs: API.SysOrg[] = [], target = new Map<string, string>()) => {
  orgs.forEach((org) => {
    const id = String(org.id ?? '');
    if (id && id !== '0' && org.name) {
      target.set(id, org.name);
    }
    if (org.children?.length) {
      flattenOrgTree(org.children, target);
    }
  });
  return target;
};

const getErrorMessage = (error: unknown, fallback: string) =>
  error instanceof Error && error.message ? error.message : fallback;

export default function UserManagementPage() {
  const genderItems = useDictValueOnly(USER_GENDER_DICT_CODE);
  const genderLabelMap = buildUserGenderLabelMap(genderItems);
  const {
    canCreateUser,
    canViewUserDetail,
    canUpdateUser,
    canDeleteUser,
    canChangeUserStatus,
    canResetUserPassword,
  } = usePermissionFlags({
    canCreateUser: USER_PERMISSIONS.CREATE,
    canViewUserDetail: USER_PERMISSIONS.QUERY,
    canUpdateUser: USER_PERMISSIONS.UPDATE,
    canDeleteUser: USER_PERMISSIONS.DELETE,
    canChangeUserStatus: USER_PERMISSIONS.STATUS,
    canResetUserPassword: USER_PERMISSIONS.RESET_PASSWORD,
  });

  // 基础状态
  const [createModalVisible, setCreateModalVisible] = useState<boolean>(false);
  const [updateModalVisible, setUpdateModalVisible] = useState<boolean>(false);
  const [currentRow, setCurrentRow] = useState<API.UserVO>({});
  const actionRef = useRef<ActionType>(null);

  // RBAC相关状态
  const [roleAssignVisible, setRoleAssignVisible] = useState(false);
  const [orgAssignVisible, setOrgAssignVisible] = useState(false);
  const [deptAssignVisible, setDeptAssignVisible] = useState(false);
  const [postAssignVisible, setPostAssignVisible] = useState(false);
  const [selectedUser, setSelectedUser] = useState<API.UserVO | null>(null);
  const [currentRoleIds, setCurrentRoleIds] = useState<React.Key[]>([]);
  const [currentOrgIds, setCurrentOrgIds] = useState<React.Key[]>([]);
  const [currentDeptIds, setCurrentDeptIds] = useState<React.Key[]>([]);
  const [currentPostIds, setCurrentPostIds] = useState<React.Key[]>([]);

  // 用户详情Modal状态
  const [userDetailVisible, setUserDetailVisible] = useState(false);
  const [selectedUserId, setSelectedUserId] = useState<React.Key | undefined>();

  // 批量操作
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);

    // 删除用户
  const deleteUserMutation = useMutation({
    mutationFn: (params: Parameters<typeof deleteUser>[0]) => deleteUser(params),
    onSuccess: () => {
      message.success('删除成功');
      actionRef.current?.reload();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '删除失败'));
    },
  });

  // 批量删除用户
  const batchDeleteMutation = useMutation({
    mutationFn: async (ids: React.Key[]) => {
      // 批量删除逻辑，暂时使用单个删除
      for (const id of ids) {
        await deleteUser({ id: String(id) });
      }
    },
    onSuccess: () => {
      message.success('批量删除成功');
      actionRef.current?.reload();
      setSelectedRowKeys([]);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '批量删除失败'));
    },
  });

  // 重置密码
  const resetPasswordMutation = useMutation({
    mutationFn: (values: { id: API.Int64; password: string }) =>
      resetUserPassword({ id: toApiIdParam(values.id) }, { password: values.password }),
    onSuccess: () => {
      message.success('密码重置成功');
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '密码重置失败'));
    },
  });

  // 更改用户状态
  const changeStatusMutation = useMutation({
    mutationFn: (params: Parameters<typeof updateUserStatus>[0]) => updateUserStatus(params),
    onSuccess: () => {
      message.success('状态更新成功');
      actionRef.current?.reload();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '状态更新失败'));
    },
  });

  // 删除用户
  const handleDelete = async (row: API.UserVO) => {
    if (!hasId(row.id)) return;
    await deleteUserMutation.mutateAsync({ id: String(row.id) });
  };

  // 批量删除
  const handleBatchDelete = () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请选择要删除的用户');
      return;
    }

    Modal.confirm({
      title: '确认批量删除',
      icon: <ExclamationCircleOutlined />,
      content: `确定要删除选中的 ${selectedRowKeys.length} 个用户吗？删除后不可恢复！`,
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        const ids = toApiIdList(selectedRowKeys);
        await batchDeleteMutation.mutateAsync(ids);
      },
    });
  };

  // 重置密码
  const handleResetPassword = (record: API.UserVO) => {
    if (!hasId(record.id)) return;
    let password = '';
    Modal.confirm({
      title: '重置密码',
      content: (
        <div className="space-y-3">
          <div>请输入新的临时密码。提交后该用户现有会话会失效。</div>
          <Input.Password
            placeholder="请输入新密码"
            onChange={(event) => {
              password = event.target.value;
            }}
          />
        </div>
      ),
      okText: '确认重置',
      cancelText: '取消',
      onOk: async () => {
        if (!password.trim()) {
          message.error('新密码不能为空');
          return Promise.reject();
        }
        await resetPasswordMutation.mutateAsync({ id: record.id!, password: password.trim() });
      },
    });
  };

  // 更改用户状态
  const handleChangeStatus = async (record: API.UserVO, newStatus: number) => {
    if (!hasId(record.id)) return;
    await changeStatusMutation.mutateAsync({
      id: toApiIdParam(record.id),
      status: newStatus,
    });
  };

  const buildUserAssignmentInfo = async (record: API.UserVO) => {
    if (!hasId(record.id)) {
      return { user: record, roleIds: [], orgIds: [], deptIds: [], postIds: [] };
    }

    const userId = String(record.id);

    let userInfo: API.UserVO = record;
    const [detailResult, roleIdsResult, orgIdsResult, deptIdsResult, postIdsResult] = await Promise.allSettled([
      getUserDetail({ id: userId }),
      getUserRoleIds({ id: userId }),
      getUserOrgIds({ id: userId }),
      getUserDeptIds({ id: userId }),
      getUserPostIds({ id: userId }),
    ]);

    if (detailResult.status === 'fulfilled') {
      userInfo = detailResult.value?.data || userInfo;
    }

    if (
      roleIdsResult.status === 'rejected' ||
      orgIdsResult.status === 'rejected' ||
      deptIdsResult.status === 'rejected' ||
      postIdsResult.status === 'rejected'
    ) {
      message.error('获取用户权限信息失败');
    }

    const roleIds =
      roleIdsResult.status === 'fulfilled'
        ? (((roleIdsResult.value?.data as unknown[]) ?? []).filter(hasId) as React.Key[])
        : [];

    const orgIds =
      orgIdsResult.status === 'fulfilled'
        ? (((orgIdsResult.value?.data as unknown[]) ?? []).filter(hasId) as React.Key[])
        : [];

    const deptIds =
      deptIdsResult.status === 'fulfilled'
        ? (((deptIdsResult.value?.data as unknown[]) ?? []).filter(hasId) as React.Key[])
        : [];

    const postIds =
      postIdsResult.status === 'fulfilled'
        ? (((postIdsResult.value?.data as unknown[]) ?? []).filter(hasId) as React.Key[])
        : [];

    return { user: userInfo as API.UserVO, roleIds, orgIds, deptIds, postIds };
  };

  // 分配角色
  const handleAssignRoles = async (record: API.UserVO) => {
    const info = await buildUserAssignmentInfo(record);
    setSelectedUser(info.user);
    setCurrentRoleIds(info.roleIds);
    setRoleAssignVisible(true);
  };

  // 分配组织
  const handleAssignOrgs = async (record: API.UserVO) => {
    const info = await buildUserAssignmentInfo(record);
    setSelectedUser(info.user);
    setCurrentOrgIds(info.orgIds);
    setOrgAssignVisible(true);
  };

  const handleAssignDepts = async (record: API.UserVO) => {
    const info = await buildUserAssignmentInfo(record);
    setSelectedUser(info.user);
    setCurrentDeptIds(info.deptIds);
    setDeptAssignVisible(true);
  };

  const handleAssignPosts = async (record: API.UserVO) => {
    const info = await buildUserAssignmentInfo(record);
    setSelectedUser(info.user);
    setCurrentPostIds(info.postIds);
    setPostAssignVisible(true);
  };

  // 显示用户详情
  const handleShowUserDetail = (record: API.UserVO) => {
    if (hasId(record.id)) {
      setSelectedUserId(record.id);
      setUserDetailVisible(true);
    }
  };

  // 表格行选择
  const rowSelection = {
    selectedRowKeys,
    onChange: (keys: React.Key[]) => {
      setSelectedRowKeys(keys);
    },
    getCheckboxProps: (record: API.UserVO) => ({
      name: record.nickname || record.username,
    }),
  };

  // 表格列配置
  const columns = UserManagementColumns({
    genderLabelMap,
    setCurrentRow,
    setUpdateModalVisible,
    canViewUserDetail,
    canUpdateUser,
    canDeleteUser,
    canChangeUserStatus,
    canResetUserPassword,
    handleDelete,
    onResetPassword: handleResetPassword,
    onAssignRoles: handleAssignRoles,
    onAssignOrgs: handleAssignOrgs,
    onAssignDepts: handleAssignDepts,
    onAssignPosts: handleAssignPosts,
    onChangeStatus: handleChangeStatus,
    onShowDetail: handleShowUserDetail,
  });

  const enrichAssignmentDisplay = async (records: API.UserVO[]) => {
    if (!records.length) {
      return records;
    }

    const [roleListResult, orgTreeResult] = await Promise.allSettled([getRoleList(), getOrgTree()]);
    const roleNameMap = new Map<string, string>();
    if (roleListResult.status === 'fulfilled') {
      (roleListResult.value?.data || []).forEach((role) => {
        const roleId = String(role.id ?? '');
        if (roleId && roleId !== '0' && role.name) {
          roleNameMap.set(roleId, role.name);
        }
      });
    }

    const orgNameMap =
      orgTreeResult.status === 'fulfilled'
        ? flattenOrgTree(orgTreeResult.value?.data || [])
        : new Map<string, string>();

    const enriched = await Promise.all(
      records.map(async (record) => {
        const currentRoleNames = normalizeDisplayList(record.allRoleNames);
        const currentOrgNames = normalizeDisplayList(record.allOrgNames);

        if ((!record.id || currentRoleNames.length > 0) && currentOrgNames.length > 0) {
          return record;
        }

        const [roleIdsResult, orgIdsResult] = await Promise.allSettled([
          record.id && currentRoleNames.length === 0 ? getUserRoleIds({ id: record.id }) : Promise.resolve(undefined),
          record.id && currentOrgNames.length === 0 ? getUserOrgIds({ id: record.id }) : Promise.resolve(undefined),
        ]);

        const roleNames =
          currentRoleNames.length > 0
            ? currentRoleNames
            : roleIdsResult.status === 'fulfilled'
              ? (((roleIdsResult.value?.data as unknown[]) ?? [])
                  .map((id) => roleNameMap.get(String(id)))
                  .filter((name): name is string => Boolean(name)))
              : [];

        const orgNames =
          currentOrgNames.length > 0
            ? currentOrgNames
            : orgIdsResult.status === 'fulfilled'
              ? (((orgIdsResult.value?.data as unknown[]) ?? [])
                  .map((id) => orgNameMap.get(String(id)))
                  .filter((name): name is string => Boolean(name)))
              : [];

        return {
          ...record,
          allRoleNames: roleNames,
          allOrgNames: orgNames,
          fallbackRoleNames: roleNames,
          fallbackOrgNames: orgNames,
        };
      }),
    );

    return enriched;
  };

  return (
    <><ProTable<API.UserVO>
      headerTitle="用户列表"
      actionRef={actionRef}
      rowKey="id"
      search={{
        labelWidth: 120,
        collapsed: false,
      }}
      toolBarRender={() => {
        const items = [];

        if (canCreateUser) {
          items.push(
            <Button
              key="add"
              type="primary"
              icon={<PlusOutlined />}
              onClick={() => setCreateModalVisible(true)}
            >
              新增用户
            </Button>,
          );
        }

        if (canDeleteUser) {
          items.push(
            <Button
              key="batchDelete"
              danger
              icon={<DeleteOutlined />}
              disabled={selectedRowKeys.length === 0}
              onClick={handleBatchDelete}
            >
              批量删除
            </Button>,
          );
        }

        return items;
      }}
      request={async (params, sort, filter) => {
        try {
          const sortField = Object.keys(sort)?.[0];
          const sortOrder = sort?.[sortField] ?? undefined;

          const result = await listUsers({
            ...params,
            sortField,
            sortOrder,
            ...filter,
          });

          // 处理 API 响应格式，确保返回 ProTable 期望的格式
          if (result && result.data) {
            const records = await enrichAssignmentDisplay(result.data.records || []);
            return {
              data: records,
              success: result.code === 0,
              total: result.data.total || 0,
            };
          }

          return {
            data: [],
            success: false,
            total: 0,
          };
        } catch (error) {
          console.error('获取用户列表失败:', error);
          return {
            data: [],
            success: false,
            total: 0,
          };
        }
      }}
      columns={columns}
      rowSelection={rowSelection}
      pagination={{
        defaultPageSize: 10,
        showSizeChanger: true,
        showQuickJumper: true,
        showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
      }}
      scroll={{x: 1400}}/><CreateUserModal
      visible={createModalVisible}
      onOk={() => {
        setCreateModalVisible(false);
        actionRef.current?.reload();
      }}
      onCancel={() => setCreateModalVisible(false)}/>

      <EditUserModal
        visible={updateModalVisible}
        userData={currentRow}
        onOk={() => {
          setUpdateModalVisible(false);
          setCurrentRow({});
          actionRef.current?.reload();
        }}
        onCancel={() => {
          setUpdateModalVisible(false);
          setCurrentRow({});
        }}
      />

      {/* 角色分配模态框 */}
      <UserRoleAssign
        visible={roleAssignVisible}
        userId={selectedUser?.id}
        userName={selectedUser?.nickname || selectedUser?.username}
        currentRoleIds={currentRoleIds}
        onOk={() => {
          setRoleAssignVisible(false);
          setSelectedUser(null);
          setCurrentRoleIds([]);
          actionRef.current?.reload();
        }}
        onCancel={() => {
          setRoleAssignVisible(false);
          setSelectedUser(null);
          setCurrentRoleIds([]);
        }}
      />

      {/* 组织分配模态框 */}
      <UserOrgAssign
        visible={orgAssignVisible}
        userId={selectedUser?.id}
        userName={selectedUser?.nickname || selectedUser?.username}
        currentOrgIds={currentOrgIds}
        onOk={() => {
          setOrgAssignVisible(false);
          setSelectedUser(null);
          setCurrentOrgIds([]);
          actionRef.current?.reload();
        }}
        onCancel={() => {
          setOrgAssignVisible(false);
          setSelectedUser(null);
          setCurrentOrgIds([]);
        }}
      />

      {/* 部门分配模态框 */}
      <UserDeptAssign
        visible={deptAssignVisible}
        userId={selectedUser?.id}
        userName={selectedUser?.nickname || selectedUser?.username}
        currentDeptIds={currentDeptIds}
        onOk={() => {
          setDeptAssignVisible(false);
          setSelectedUser(null);
          setCurrentDeptIds([]);
          actionRef.current?.reload();
        }}
        onCancel={() => {
          setDeptAssignVisible(false);
          setSelectedUser(null);
          setCurrentDeptIds([]);
        }}
      />

      {/* 岗位分配模态框 */}
      <UserPostAssign
        visible={postAssignVisible}
        userId={selectedUser?.id}
        userName={selectedUser?.nickname || selectedUser?.username}
        currentPostIds={currentPostIds}
        onOk={() => {
          setPostAssignVisible(false);
          setSelectedUser(null);
          setCurrentPostIds([]);
          actionRef.current?.reload();
        }}
        onCancel={() => {
          setPostAssignVisible(false);
          setSelectedUser(null);
          setCurrentPostIds([]);
        }}
      />

      {/* 用户详情Modal */}
      <UserDetailModal
        visible={userDetailVisible}
        userId={selectedUserId}
        onCancel={() => {
          setUserDetailVisible(false);
          setSelectedUserId(undefined);
        }}
      />
    </>
  );
}
