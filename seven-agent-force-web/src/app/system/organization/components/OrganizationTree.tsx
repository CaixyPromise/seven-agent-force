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

export interface OrganizationTreeNode extends DataNode {
  nodeData: API.SysOrg;
  children?: OrganizationTreeNode[];
}

interface OrganizationTreeProps {
  data: API.SysOrg[];
  loading: boolean;
  selectedKeys: React.Key[];
  onSelect: NonNullable<TreeProps<OrganizationTreeNode>['onSelect']>;
  onAdd: (parentId?: API.Int64) => void;
  onEdit: (org: API.SysOrg) => void;
  onDelete: (org: API.SysOrg) => void;
  onStatusChange: (org: API.SysOrg, status: number) => void;
  onMove: NonNullable<TreeProps<OrganizationTreeNode>['onDrop']>;
}

export const OrganizationTree: React.FC<OrganizationTreeProps> = ({
  data,
  loading,
  selectedKeys,
  onSelect,
  onAdd,
  onEdit,
  onDelete,
  onStatusChange,
  onMove,
}) => {
  // 转换组织数据为树形结构
  const convertToTreeData = (
    orgs: API.SysOrg[],
    parentKey = 'organization-root',
  ): OrganizationTreeNode[] => {
    return orgs.map((org, index) => {
      const key = org.id ?? `${parentKey}:${org.code ?? index}`;
      return {
      key,
      title: (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ApartmentOutlined style={{ color: '#1890ff' }} />
            <span style={{ fontWeight: 500 }}>{org.name}</span>
            <Tag color="blue">{org.code}</Tag>
            {org.status === 1 && (
              <Tag color="red">禁用</Tag>
            )}
          </div>
          <Dropdown
            menu={{
              items: [
                {
                  key: 'add',
                  label: '新增子组织',
                  icon: <PlusOutlined />,
                  onClick: () => onAdd(org.id),
                },
                {
                  key: 'edit',
                  label: '编辑',
                  icon: <EditOutlined />,
                  onClick: () => onEdit(org),
                },
                {
                  key: 'status',
                  label: org.status === 0 ? '禁用' : '启用',
                  icon: <EditOutlined />,
                  onClick: () => onStatusChange(org, org.status === 0 ? 1 : 0),
                },
                {
                  type: 'divider',
                },
                {
                  key: 'delete',
                  label: '删除',
                  icon: <DeleteOutlined />,
                  danger: true,
                  onClick: () => onDelete(org),
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
      children: org.children ? convertToTreeData(org.children, String(key)) : undefined,
      nodeData: org, // 保存原始数据
    };
    });
  };

  const treeData = convertToTreeData(data);

  const treeProps: TreeProps<OrganizationTreeNode> = {
    treeData,
    selectedKeys,
    onSelect,
    showLine: { showLeafIcon: false },
    blockNode: true,
    draggable: {
      icon: false,
    },
    onDrop: onMove,
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
          <div>暂无组织数据</div>
          <div style={{ fontSize: 12, marginTop: 8 }}>点击"新增组织"开始配置组织架构</div>
        </div>
        )}
      </Spin>
    </div>
  );
};
