'use client';

import React, { useMemo, useState } from 'react';
import { Modal, Spin, Tree, message } from 'antd';
import type { TreeProps } from 'antd';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { getRoleList } from '@/api/sysRoleController';
import { assignUserRoles } from '@/api/userController';
import { hasId, toApiIdList, toApiIdParam } from '@/lib/apiId';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';

interface UserRoleAssignProps {
  visible: boolean;
  userId?: React.Key;
  userName?: string;
  currentRoleIds: React.Key[];
  onOk: (roleIds: React.Key[]) => void;
  onCancel: () => void;
}

export const UserRoleAssign: React.FC<UserRoleAssignProps> = ({
  visible,
  userId,
  userName,
  currentRoleIds,
  onOk,
  onCancel,
}) => {
  const [selection, setSelection] = useState<{ sourceKey: string; keys: string[] }>({
    sourceKey: '',
    keys: [],
  });
  const initialCheckedKeys = useMemo(() => currentRoleIds.map(String), [currentRoleIds]);
  const sourceKey = `${visible}:${String(userId ?? '')}:${initialCheckedKeys.join(',')}`;
  const checkedKeys = selection.sourceKey === sourceKey ? selection.keys : initialCheckedKeys;
  const queryClient = useQueryClient();

  // 获取角色列表
  const { data: roleList, isLoading } = useQuery({
    queryKey: ['roleList'],
    queryFn: () => getRoleList(),
    enabled: visible,
  });

  // 分配角色
  const assignRolesMutation = useMutation({
    mutationFn: (values: { userId: React.Key; roleIds: React.Key[] }) =>
      assignUserRoles(
        { id: String(values.userId) },
        {
          userId: toApiIdParam(values.userId),
          roleIds: toApiIdList(values.roleIds),
        },
      ),
    onSuccess: () => {
      message.success('角色分配成功');
      void queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY });
      void queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });
      onOk(checkedKeys);
    },
    onError: error => {
      message.error(error.message || '角色分配失败');
    },
  });

  const handleOk = async () => {
    if (!hasId(userId)) {
      message.error('用户ID无效');
      return;
    }

    await assignRolesMutation.mutateAsync({
      userId,
      roleIds: checkedKeys,
    });
  };

  const handleCancel = () => {
    setSelection({ sourceKey: '', keys: [] });
    onCancel();
  };

  const handleCheck: TreeProps['onCheck'] = (nextCheckedKeys) => {
    const normalizedKeys = (
      Array.isArray(nextCheckedKeys) ? nextCheckedKeys : nextCheckedKeys.checked
    ).map((key) => String(key));
    setSelection({ sourceKey, keys: normalizedKeys });
  };

  // 转换角色数据为树形结构
  const treeData = roleList?.data?.map((role: API.RoleVO) => ({
    key: String(role.id),
    title: role.name,
    children: undefined,
  })) || [];

  return (
    <Modal
      title={`分配角色 - ${userName}`}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={assignRolesMutation.isPending}
      width={600}
      mask={{ closable: false }}
    >
      <div style={{ maxHeight: 400, overflow: 'auto' }}>
        <Spin spinning={isLoading}>
          <Tree
            checkable
            checkedKeys={checkedKeys}
            onCheck={handleCheck}
            treeData={treeData}
          />
        </Spin>
      </div>
    </Modal>
  );
};
