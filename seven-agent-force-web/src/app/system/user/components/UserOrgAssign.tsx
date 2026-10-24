'use client';

import React, { useMemo, useState } from 'react';
import { Modal, Spin, Tree, message } from 'antd';
import type { TreeProps } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useQuery, useMutation } from '@tanstack/react-query';
import { getOrgTree } from '@/api/sysOrgController';
import { assignUserOrgs } from '@/api/userController';
import { hasId, toApiIdList, toApiIdParam } from '@/lib/apiId';

interface UserOrgAssignProps {
  visible: boolean;
  userId?: React.Key;
  userName?: string;
  currentOrgIds: React.Key[];
  onOk: (orgIds: React.Key[]) => void;
  onCancel: () => void;
}

export const UserOrgAssign: React.FC<UserOrgAssignProps> = ({
  visible,
  userId,
  userName,
  currentOrgIds,
  onOk,
  onCancel,
}) => {
  const [selection, setSelection] = useState<{ sourceKey: string; keys: string[] }>({
    sourceKey: '',
    keys: [],
  });
  const initialCheckedKeys = useMemo(() => currentOrgIds.map(String), [currentOrgIds]);
  const sourceKey = `${visible}:${String(userId ?? '')}:${initialCheckedKeys.join(',')}`;
  const checkedKeys = selection.sourceKey === sourceKey ? selection.keys : initialCheckedKeys;

  // 获取组织树
  const { data: orgTree, isLoading } = useQuery({
    queryKey: ['orgTree'],
    queryFn: () => getOrgTree(),
    enabled: visible,
  });

  // 分配组织
  const assignOrgsMutation = useMutation({
    mutationFn: (values: { userId: React.Key; orgIds: React.Key[] }) =>
      assignUserOrgs(
        { id: String(values.userId) },
        {
          userId: toApiIdParam(values.userId),
          orgIds: toApiIdList(values.orgIds),
        },
      ),
    onSuccess: () => {
      message.success('组织分配成功');
      onOk(checkedKeys);
    },
    onError: error => {
      message.error(error.message || '组织分配失败');
    },
  });

  const handleOk = async () => {
    if (!hasId(userId)) {
      message.error('用户ID无效');
      return;
    }

    await assignOrgsMutation.mutateAsync({
      userId,
      orgIds: checkedKeys,
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

  // 转换组织数据为树形结构
  const convertToTreeData = (orgs: API.SysOrg[]): DataNode[] => {
    return orgs.map(org => ({
      key: String(org.id),
      title: org.name,
      children: org.children ? convertToTreeData(org.children) : undefined,
    }));
  };

  const treeData = orgTree?.data ? convertToTreeData(orgTree.data) : [];

  return (
    <Modal
      title={`分配组织 - ${userName}`}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={assignOrgsMutation.isPending}
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
