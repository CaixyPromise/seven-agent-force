'use client';

import React from 'react';
import { Tree, Button, Tag, Dropdown, Spin } from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  MoreOutlined,
  ApartmentOutlined
} from '@ant-design/icons';
import type { TreeProps } from 'antd';
import type { DataNode } from 'antd/es/tree';

export interface DepartmentTreeNode extends DataNode {
  nodeData: API.SysDept;
  children?: DepartmentTreeNode[];
}

export interface DepartmentTreeProps {
  data: API.SysDept[];
  loading: boolean;
  selectedKeys: React.Key[];
  onSelect: NonNullable<TreeProps<DepartmentTreeNode>['onSelect']>;
  onAdd: (parentId?: API.Int64) => void;
  onEdit: (dept: API.SysDept) => void;
  onDelete: (dept: API.SysDept) => void;
  onStatusChange: (dept: API.SysDept, status: number) => void;
}

export const DepartmentTree: React.FC<DepartmentTreeProps> = ({
  data,
  loading,
  selectedKeys,
  onSelect,
  onAdd,
  onEdit,
  onDelete,
  onStatusChange,
}) => {
  // 转换部门数据为树形结构
  const convertToTreeData = (
    depts: API.SysDept[],
    parentKey = 'department-root',
  ): DepartmentTreeNode[] => {
    return depts.map((dept, index) => {
      const key = dept.id ?? `${parentKey}:${dept.code ?? index}`;
      return {
      key,
      title: (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ApartmentOutlined style={{ color: '#13c2c2' }} />
            <span style={{ fontWeight: 500 }}>{dept.name}</span>
            <Tag color="cyan">{dept.code}</Tag>
            {dept.status === 1 && (
              <Tag color="red">禁用</Tag>
            )}
          </div>
          <Dropdown
            menu={{
              items: [
                {
                  key: 'add',
                  label: '新增子部门',
                  icon: <PlusOutlined />,
                  onClick: () => onAdd(dept.id),
                },
                {
                  key: 'edit',
                  label: '编辑',
                  icon: <EditOutlined />,
                  onClick: () => onEdit(dept),
                },
                {
                  key: 'status',
                  label: dept.status === 0 ? '禁用' : '启用',
                  icon: <EditOutlined />,
                  onClick: () => onStatusChange(dept, dept.status === 0 ? 1 : 0),
                },
                {
                  type: 'divider',
                },
                {
                  key: 'delete',
                  label: '删除',
                  icon: <DeleteOutlined />,
                  danger: true,
                  onClick: () => onDelete(dept),
                },
              ],
            }}
            trigger={['click']}
          >
            <Button
              type="text"
              size="small"
              icon={<MoreOutlined />}
              onClick={(e) => e.stopPropagation()}
            />
          </Dropdown>
        </div>
      ),
      children: dept.children ? convertToTreeData(dept.children, String(key)) : undefined,
      nodeData: dept, // 保存原始数据
    };
    });
  };

  const treeData = convertToTreeData(data);

  const treeProps: TreeProps<DepartmentTreeNode> = {
    treeData,
    selectedKeys,
    onSelect,
    showLine: { showLeafIcon: false },
    blockNode: true,
  };

  return (
    <div style={{ padding: '8px 0' }}>
      <Spin spinning={loading}>
        {treeData.length > 0 ? (
          <Tree {...treeProps} />
        ) : (
        <div style={{
          textAlign: 'center',
          padding: '40px 0',
          color: '#999',
          background: '#fafafa',
          borderRadius: 6,
          border: '1px dashed #d9d9d9'
        }}>
          <ApartmentOutlined style={{ fontSize: 48, marginBottom: 16, color: '#d9d9d9' }} />
          <div>暂无部门数据</div>
          <div style={{ fontSize: 12, marginTop: 8 }}>点击"新增部门"开始配置部门架构</div>
        </div>
        )}
      </Spin>
    </div>
  );
};
