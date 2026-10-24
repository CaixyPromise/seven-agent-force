'use client';

import { useEffect, useState } from 'react';
import { Alert, Modal, Space, Typography, message } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getPostRoleIds, replacePostRoleIds } from '@/api/postRoleController';
import { RoleSelector } from '@/components/Selectors/RoleSelector';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';

type ApiIdentifier = string | number;

interface PostRoleAssignProps {
  open: boolean;
  post?: API.SysPost;
  onClose: () => void;
}

export function PostRoleAssign({ open, post, onClose }: PostRoleAssignProps) {
  const queryClient = useQueryClient();
  const postId = post?.id as ApiIdentifier | undefined;
  const [roleIds, setRoleIds] = useState<ApiIdentifier[]>([]);
  const [draftReady, setDraftReady] = useState(false);
  const roleQuery = useQuery({
    queryKey: ['post-role-ids', postId],
    queryFn: async () => (await getPostRoleIds(postId!)).data ?? [],
    enabled: open && postId !== undefined,
  });

  useEffect(() => {
    if (!open || !roleQuery.data) return;
    const timer = window.setTimeout(() => {
      setRoleIds(roleQuery.data.map(String));
      setDraftReady(true);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open, roleQuery.data]);

  const mutation = useMutation({
    mutationFn: () => replacePostRoleIds(postId!, roleIds),
    onSuccess: async () => {
      message.success('岗位角色已更新');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['post-role-ids', postId] }),
        queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY }),
      ]);
      onClose();
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : '岗位角色更新失败');
    },
  });

  return (
    <Modal
      title={`分配角色 - ${post?.name || ''}`}
      open={open}
      onCancel={onClose}
      onOk={() => mutation.mutate()}
      okText="完成二次验证并保存"
      confirmLoading={mutation.isPending}
      destroyOnHidden
      width={680}
    >
      <Space direction="vertical" size={16} className="w-full">
        <Alert
          type="info"
          showIcon
          message="安全根不能通过岗位继承"
          description="列表已排除授权安全根；后端会再次校验，异常历史关系不会被静默保留。"
        />
        <Typography.Text>岗位角色</Typography.Text>
        <RoleSelector
          mode="multiple"
          value={roleIds}
          onChange={(value) => setRoleIds(Array.isArray(value) ? value : value === undefined ? [] : [value])}
          excludeAuthorizationRoot
          disabled={!draftReady || roleQuery.isLoading}
          style={{ width: '100%' }}
        />
        {roleQuery.isError ? <Alert type="error" showIcon message="岗位角色加载失败" /> : null}
      </Space>
    </Modal>
  );
}
