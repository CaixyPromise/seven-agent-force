'use client';

import React, { useMemo, useState } from 'react';
import { Spin, Tree } from 'antd';
import type { TreeProps } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { useQuery } from '@tanstack/react-query';
import { getMenuTree } from '@/api/sysMenuController';
import { getRoleMenuIds } from '@/api/sysRoleController';

interface PermissionAssignProps {
  roleId?: API.Int64;
  onChange: (menuIds: string[]) => void;
}

function getAllKeys(tree: API.MenuVO[]): string[] {
  return tree.flatMap(node => [
    ...(node.id ? [String(node.id)] : []),
    ...getAllKeys(node.children ?? []),
  ]);
}

function convertToTreeData(tree: API.MenuVO[]): DataNode[] {
  return tree.map(node => ({
    key: String(node.id),
    title: node.name,
    children: convertToTreeData(node.children ?? []),
  }));
}

export const PermissionAssign: React.FC<PermissionAssignProps> = ({
  roleId,
  onChange,
}) => {
  const [selection, setSelection] = useState<{ sourceKey: string; keys: string[] }>({
    sourceKey: '',
    keys: [],
  });
  const [expandedKeys, setExpandedKeys] = useState<string[]>();

  // 获取菜单树
  const { data: menuTree, isLoading: menuLoading } = useQuery({
    queryKey: ['menuTree'],
    queryFn: () => getMenuTree(),
  });

  // 获取角色已分配的菜单ID
  const { data: roleMenuIds, isLoading: roleMenuLoading } = useQuery({
    queryKey: ['roleMenuIds', roleId],
    queryFn: () => getRoleMenuIds({ roleId: String(roleId) }),
    enabled: !!roleId,
  });

  const initialCheckedKeys = useMemo(
    () => (roleMenuIds?.data ?? []).map(String),
    [roleMenuIds?.data],
  );
  const sourceKey = `${roleId ?? ''}:${initialCheckedKeys.join(',')}`;
  const checkedKeys = selection.sourceKey === sourceKey ? selection.keys : initialCheckedKeys;
  const treeData = useMemo(() => convertToTreeData(menuTree?.data ?? []), [menuTree?.data]);
  const allKeys = useMemo(() => getAllKeys(menuTree?.data ?? []), [menuTree?.data]);
  const effectiveExpandedKeys = expandedKeys ?? allKeys;

  const handleCheck: TreeProps['onCheck'] = (checkedKeysValue) => {
    const nextCheckedKeys = (
      Array.isArray(checkedKeysValue) ? checkedKeysValue : checkedKeysValue.checked
    ).map((key) => String(key));
    setSelection({ sourceKey, keys: nextCheckedKeys });
    onChange(nextCheckedKeys);
  };

  const handleExpand: TreeProps['onExpand'] = (keys) => {
    setExpandedKeys(keys.map(String));
  };

  return (
    <div style={{ maxHeight: 500, overflow: 'auto' }}>
      <Spin spinning={menuLoading || roleMenuLoading}>
        <Tree
          checkable
          checkedKeys={checkedKeys}
          expandedKeys={effectiveExpandedKeys}
          onCheck={handleCheck}
          onExpand={handleExpand}
          treeData={treeData}
        />
      </Spin>
    </div>
  );
};
