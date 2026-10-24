'use client';

import React, { useMemo, useState } from 'react';
import { Modal, Spin, Tree, message } from 'antd';
import type { TreeProps } from 'antd';
import { useMutation, useQuery } from '@tanstack/react-query';
import { getPostList } from '@/api/sysPostController';
import { assignUserPosts } from '@/api/userController';
import { hasId, toApiIdList, toApiIdParam } from '@/lib/apiId';

interface UserPostAssignProps {
  visible: boolean;
  userId?: React.Key;
  userName?: string;
  currentPostIds: React.Key[];
  onOk: (postIds: React.Key[]) => void;
  onCancel: () => void;
}

export const UserPostAssign: React.FC<UserPostAssignProps> = ({
  visible,
  userId,
  userName,
  currentPostIds,
  onOk,
  onCancel,
}) => {
  const [selection, setSelection] = useState<{ sourceKey: string; keys: string[] }>({
    sourceKey: '',
    keys: [],
  });
  const initialCheckedKeys = useMemo(() => currentPostIds.map(String), [currentPostIds]);
  const sourceKey = `${visible}:${String(userId ?? '')}:${initialCheckedKeys.join(',')}`;
  const checkedKeys = selection.sourceKey === sourceKey ? selection.keys : initialCheckedKeys;

  const { data: postList, isLoading } = useQuery({
    queryKey: ['postList'],
    queryFn: () => getPostList(),
    enabled: visible,
  });

  const assignPostsMutation = useMutation({
    mutationFn: (values: { userId: React.Key; postIds: React.Key[] }) =>
      assignUserPosts(
        { id: toApiIdParam(values.userId) },
        {
          userId: toApiIdParam(values.userId),
          postIds: toApiIdList(values.postIds),
          ids: toApiIdList(values.postIds),
        },
      ),
    onSuccess: () => {
      message.success('岗位分配成功');
      onOk(checkedKeys);
    },
    onError: error => {
      message.error(error.message || '岗位分配失败');
    },
  });

  const handleOk = async () => {
    if (!hasId(userId)) {
      message.error('用户ID无效');
      return;
    }
    await assignPostsMutation.mutateAsync({ userId, postIds: checkedKeys });
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

  const treeData =
    postList?.data?.map((post: API.SysPost) => ({
      key: String(post.id),
      title: post.name || post.code || String(post.id),
    })) || [];

  return (
    <Modal
      title={`分配岗位 - ${userName}`}
      open={visible}
      onOk={handleOk}
      onCancel={handleCancel}
      confirmLoading={assignPostsMutation.isPending}
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
