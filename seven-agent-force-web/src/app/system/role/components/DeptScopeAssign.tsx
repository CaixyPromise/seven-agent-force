'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Skeleton, Space, Tree, message } from 'antd';
import type { DataNode } from 'antd/es/tree';
import { getDeptTree } from '@/api/sysDeptController';
import { assignRoleDepts, getRoleDeptIds } from '@/api/sysRoleController';

interface DeptScopeAssignProps {
  role?: API.RoleVO;
  onSaved?: () => void;
}

function toTreeData(items: API.SysDept[] | undefined): DataNode[] {
  return (items ?? [])
    .filter(item => item.id !== undefined)
    .map(item => ({
      key: String(item.id),
      title: item.name ? `${item.name} (${item.code ?? item.id})` : String(item.id),
      children: toTreeData(item.children),
    }));
}

function normalizeCheckedKeys(keys: unknown): string[] {
  if (Array.isArray(keys)) {
    return keys.map(String);
  }
  const checked = (keys as { checked?: React.Key[] } | undefined)?.checked;
  return (checked ?? []).map(String);
}

export const DeptScopeAssign: React.FC<DeptScopeAssignProps> = ({ role, onSaved }) => {
  const [deptTree, setDeptTree] = useState<API.SysDept[]>([]);
  const [checkedKeys, setCheckedKeys] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const roleId = role?.id;
  const isCustomScope = role?.dataScope === 2;

  const treeData = useMemo(() => toTreeData(deptTree), [deptTree]);

  useEffect(() => {
    if (!roleId) return;
    let cancelled = false;
    Promise.all([getDeptTree(), getRoleDeptIds(roleId)])
      .then(([deptResult, scopeResult]) => {
        if (cancelled) return;
        setDeptTree(deptResult.data ?? []);
        setCheckedKeys((scopeResult.data?.deptIds ?? []).map(String));
      })
      .catch((error: unknown) => {
        message.error((error as Error)?.message || '加载部门范围失败');
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [roleId]);

  const handleSave = async () => {
    if (!roleId) return;
    setSaving(true);
    try {
      await assignRoleDepts({
        roleId,
        deptIds: checkedKeys.filter((id) => /^\d+$/.test(id) && id !== '0'),
      });
      message.success('部门范围已保存');
      onSaved?.();
    } catch (error: unknown) {
      message.error((error as Error)?.message || '保存部门范围失败');
    } finally {
      setSaving(false);
    }
  };

  if (!roleId) {
    return <Alert type="warning" showIcon message="请先选择角色" />;
  }

  return (
    <Space direction="vertical" className="w-full" size="middle">
      {!isCustomScope ? (
        <Alert
          type="info"
          showIcon
          message="当前角色不是自定数据权限"
          description="只有 dataScope=CUSTOM 的角色会使用这里选择的部门范围。"
        />
      ) : null}
      {loading ? (
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : (
        <Tree
          checkable
          defaultExpandAll
          treeData={treeData}
          checkedKeys={checkedKeys}
          onCheck={keys => setCheckedKeys(normalizeCheckedKeys(keys))}
        />
      )}
      <div className="flex justify-end">
        <Button type="primary" loading={saving} onClick={handleSave}>
          保存部门范围
        </Button>
      </div>
    </Space>
  );
};
