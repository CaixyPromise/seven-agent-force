'use client';

import React, { useMemo, useState } from 'react';
import { Modal, Spin, Tree, message } from 'antd';
import type { TreeProps } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useMutation, useQuery } from '@tanstack/react-query';
import { getDeptTree } from '@/api/sysDeptController';
import { assignUserDepts } from '@/api/userController';
import { hasId, toApiIdList, toApiIdParam } from '@/lib/apiId';

interface UserDeptAssignProps {
  visible: boolean;
  userId?: React.Key;
  userName?: string;
  currentDeptIds: React.Key[];
  onOk: (deptIds: React.Key[]) => void;
  onCancel: () => void;
}

export const UserDeptAssign: React.FC<UserDeptAssignProps> = ({
  visible,
  userId,
  userName,
  currentDeptIds,
  onOk,
  onCancel,
}) => {
  const [selection, setSelection] = useState<{ sourceKey: string; keys: string[] }>({
    sourceKey: '',
    keys: [],
  });
  const initialCheckedKeys = useMemo(() => currentDeptIds.map(String), [currentDeptIds]);
  const sourceKey = `${visible}:${String(userId ?? '')}:${initialCheckedKeys.join(',')}`;
  const checkedKeys = selection.sourceKey === sourceKey ? selection.keys : initialCheckedKeys;

  const { data: deptTree, isLoading } = useQuery({
    queryKey: ['deptTree'],
    queryFn: () => getDeptTree(),
    enabled: visible,
  });

  const assignDeptsMutation = useMutation({
    mutationFn: (values: { userId: React.Key; deptIds: React.Key[] }) =>
      assignUserDepts(
        { id: toApiIdParam(values.userId) },
        {
          userId: toApiIdParam(values.userId),
          deptIds: toApiIdList(values.deptIds),
          ids: toApiIdList(values.deptIds),
        },
      ),
    onSuccess: () => {
      message.success('部门分配成功');
      onOk(checkedKeys);
    },
    onError: error => {
      message.error(error.message || '部门分配失败');
    },
  });

  const handleOk = async () => {
    if (!hasId(userId)) {
      message.error('用户ID无效');
      return;
    }
    await assignDeptsMutation.mutateAsync({ userId, deptIds: checkedKeys });
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

  const convertToTreeData = (depts: API.SysDept[] = []): DataNode[] =>
    depts.map((dept) => ({
      key: String(dept.id),
      title: dept.name,
      children: dept.children ? convertToTreeData(dept.children) : undefined,
    }));

  return (
    <Modal
      title={`分配部门 - ${userName}`}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={assignDeptsMutation.isPending}
      width={600}
      mask={{ closable: false }}
    >
      <div style={{ maxHeight: 400, overflow: 'auto' }}>
        <Spin spinning={isLoading}>
          <Tree
            checkable
            checkedKeys={checkedKeys}
            onCheck={handleCheck}
            treeData={convertToTreeData(deptTree?.data || [])}
          />
        </Spin>
      </div>
    </Modal>
  );
};
