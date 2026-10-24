import React, { useState, useEffect, useMemo } from 'react';
import { Tree, Input, Card, Empty, Spin, Tag, Tooltip } from 'antd';
import { SearchOutlined, TeamOutlined, BankOutlined, UserOutlined } from '@ant-design/icons';
import type { TreeProps } from 'antd/es/tree';
import type { DataNode } from 'antd/es/tree';

export interface OrganizationNode {
  id: API.Int64;
  name: string;
  code: string;
  type: 'ORG' | 'DEPT';
  parentId: API.Int64;
  leader?: string;
  phone?: string;
  email?: string;
  address?: string;
  status: number;
  orderNum: number;
  children?: OrganizationNode[];
  userCount?: number; // 用户数量
  createTime: Date;
  updateTime: Date;
}

interface OrganizationTreeSelectorProps {
  value?: API.Int64 | API.Int64[];
  onChange?: (
    value: API.Int64 | API.Int64[] | undefined,
    nodes?: OrganizationNode | OrganizationNode[],
  ) => void;
  checkable?: boolean;
  selectable?: boolean;
  multiple?: boolean;
  showSearch?: boolean;
  disabled?: boolean;
  // 过滤选项
  filterByType?: ('ORG' | 'DEPT')[];
  filterByStatus?: number;
  excludeNodes?: API.Int64[];
  // 自定义数据源
  dataSource?: OrganizationNode[];
  onLoad?: () => Promise<OrganizationNode[]>;
  // 显示选项
  showType?: boolean;
  showLeader?: boolean;
  showUserCount?: boolean;
  showStatus?: boolean;
  expandAll?: boolean;
  defaultExpandedKeys?: React.Key[];
  height?: number;
  style?: React.CSSProperties;
  className?: string;
}

const organizationTypeIcons: Record<OrganizationNode['type'], React.ReactNode> = {
  ORG: <BankOutlined style={{ color: '#1890ff' }} />,
  DEPT: <TeamOutlined style={{ color: '#52c41a' }} />,
};

function getAllOrganizationKeys(nodes: OrganizationNode[]): string[] {
  return nodes.flatMap(node => [
    String(node.id),
    ...getAllOrganizationKeys(node.children ?? []),
  ]);
}

function filterOrganizationNodes(
  nodes: OrganizationNode[],
  filterByType: OrganizationTreeSelectorProps['filterByType'],
  filterByStatus: number | undefined,
  excludedIds: Set<API.Int64>,
): OrganizationNode[] {
  return nodes.flatMap(node => {
    if (
      excludedIds.has(node.id) ||
      (filterByType && !filterByType.includes(node.type)) ||
      (filterByStatus !== undefined && node.status !== filterByStatus)
    ) {
      return [];
    }
    return [{
      ...node,
      children: filterOrganizationNodes(
        node.children ?? [],
        filterByType,
        filterByStatus,
        excludedIds,
      ),
    }];
  });
}

function searchOrganizationNodes(
  nodes: OrganizationNode[],
  keyword: string,
  ancestorMatches = false,
): OrganizationNode[] {
  const normalizedKeyword = keyword.trim().toLowerCase();
  if (!normalizedKeyword) return nodes;

  return nodes.flatMap(node => {
    const matches =
      ancestorMatches ||
      node.name.toLowerCase().includes(normalizedKeyword) ||
      node.code.toLowerCase().includes(normalizedKeyword) ||
      Boolean(node.leader?.toLowerCase().includes(normalizedKeyword));
    const children = searchOrganizationNodes(node.children ?? [], normalizedKeyword, matches);
    return matches || children.length > 0 ? [{ ...node, children }] : [];
  });
}

function findOrganization(nodes: OrganizationNode[], id: API.Int64): OrganizationNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    const child = findOrganization(node.children ?? [], id);
    if (child) return child;
  }
  return undefined;
}

function buildOrganizationTreeData(
  nodes: OrganizationNode[],
  options: {
    disabled: boolean;
    showType: boolean;
    showStatus: boolean;
    showUserCount: boolean;
    showLeader: boolean;
  },
): DataNode[] {
  return nodes.map(node => {
    const nodeTitle = (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '2px 0' }}>
        {options.showType && organizationTypeIcons[node.type]}
        <span style={{ fontWeight: 500 }}>{node.name}</span>
        <span style={{ color: '#666', fontSize: 12 }}>({node.code})</span>
        {options.showStatus && (
          <Tag color={node.status === 1 ? 'green' : 'red'}>
            {node.status === 1 ? '启用' : '禁用'}
          </Tag>
        )}
        {options.showUserCount && node.userCount !== undefined && (
          <Tag color="blue" icon={<UserOutlined />}>
            {node.userCount}
          </Tag>
        )}
        {options.showLeader && node.leader && (
          <span style={{ color: '#999', fontSize: 11 }}>负责人: {node.leader}</span>
        )}
      </div>
    );
    const title =
      node.phone || node.email || node.address ? (
        <Tooltip
          title={
            <div>
              {node.phone && <div>电话: {node.phone}</div>}
              {node.email && <div>邮箱: {node.email}</div>}
              {node.address && <div>地址: {node.address}</div>}
            </div>
          }
          placement="right"
        >
          {nodeTitle}
        </Tooltip>
      ) : (
        nodeTitle
      );
    return {
      key: String(node.id),
      title,
      children: buildOrganizationTreeData(node.children ?? [], options),
      disabled: options.disabled,
    };
  });
}

/**
 * 组织部门树选择器组件
 * 支持组织架构树形展示、搜索、多选等功能
 */
export const OrganizationTreeSelector: React.FC<OrganizationTreeSelectorProps> = ({
  value,
  onChange,
  checkable = true,
  selectable = false,
  multiple = true,
  showSearch = true,
  disabled = false,
  filterByType,
  filterByStatus,
  excludeNodes = [],
  dataSource,
  onLoad,
  showType = true,
  showLeader = true,
  showUserCount = true,
  showStatus = false,
  expandAll = false,
  defaultExpandedKeys = [],
  height = 400,
  style,
  className,
}) => {
  const [loadResult, setLoadResult] = useState<{
    loader: OrganizationTreeSelectorProps['onLoad'];
    data: OrganizationNode[];
  }>({ loader: undefined, data: [] });
  const [searchValue, setSearchValue] = useState('');
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>();
  const [selection, setSelection] = useState<{
    sourceKey: string;
    checkedKeys: React.Key[];
    selectedKeys: React.Key[];
  }>({ sourceKey: '', checkedKeys: [], selectedKeys: [] });

  // 初始化数据
  useEffect(() => {
    if (dataSource !== undefined || !onLoad || loadResult.loader === onLoad) return;
    let cancelled = false;
    void onLoad()
      .then(data => {
        if (!cancelled) setLoadResult({ loader: onLoad, data });
      })
      .catch(error => {
        if (!cancelled) {
          console.error('加载组织数据失败:', error);
          setLoadResult({ loader: onLoad, data: [] });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [dataSource, loadResult.loader, onLoad]);

  const loading =
    dataSource === undefined && onLoad !== undefined && loadResult.loader !== onLoad;
  const organizations = useMemo(
    () => dataSource ?? (loadResult.loader === onLoad ? loadResult.data : []),
    [dataSource, loadResult.data, loadResult.loader, onLoad],
  );
  const valueKeys = useMemo(
    () => (value === undefined ? [] : (Array.isArray(value) ? value : [value]).map(String)),
    [value],
  );
  const selectionSourceKey = `${checkable}:${selectable}:${multiple}:${valueKeys.join(',')}`;
  const checkedKeys =
    selection.sourceKey === selectionSourceKey ? selection.checkedKeys : checkable ? valueKeys : [];
  const selectedKeys =
    selection.sourceKey === selectionSourceKey ? selection.selectedKeys : selectable ? valueKeys : [];

  const processedOrganizations = useMemo(() => {
    const filtered = filterOrganizationNodes(
      organizations,
      filterByType,
      filterByStatus,
      new Set(excludeNodes),
    );
    return searchOrganizationNodes(filtered, searchValue);
  }, [organizations, searchValue, filterByType, filterByStatus, excludeNodes]);

  const treeData = useMemo(
    () =>
      buildOrganizationTreeData(processedOrganizations, {
        disabled,
        showType,
        showStatus,
        showUserCount,
        showLeader,
      }),
    [processedOrganizations, disabled, showType, showStatus, showUserCount, showLeader],
  );
  const defaultKeys = useMemo(
    () => (expandAll ? getAllOrganizationKeys(processedOrganizations) : defaultExpandedKeys),
    [expandAll, processedOrganizations, defaultExpandedKeys],
  );
  const effectiveExpandedKeys = expandedKeys ?? defaultKeys;

  const handleCheck: NonNullable<TreeProps['onCheck']> = checkedKeysValue => {
    const keys = Array.isArray(checkedKeysValue) ? checkedKeysValue : checkedKeysValue.checked;
    setSelection({ sourceKey: selectionSourceKey, checkedKeys: keys, selectedKeys });

    const checkedIds = keys.map(key => String(key) as API.Int64);
    const selectedOrganizations = checkedIds
      .map(id => findOrganization(organizations, id))
      .filter((node): node is OrganizationNode => node !== undefined);
    onChange?.(checkedIds, selectedOrganizations);
  };

  const handleSelect: NonNullable<TreeProps['onSelect']> = selectedKeysValue => {
    setSelection({
      sourceKey: selectionSourceKey,
      checkedKeys,
      selectedKeys: selectedKeysValue,
    });
    if (selectedKeysValue.length > 0) {
      const selectedId = String(selectedKeysValue[0]) as API.Int64;
      onChange?.(selectedId, findOrganization(organizations, selectedId));
    } else {
      onChange?.(undefined, undefined);
    }
  };

  // 处理展开变化
  const handleExpand = (expandedKeysValue: React.Key[]) => {
    setExpandedKeys(expandedKeysValue);
  };

  // 搜索框变化
  const handleSearchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const keyword = e.target.value;
    setSearchValue(keyword);

    // 如果有搜索关键词，展开所有匹配的节点
    if (keyword) {
      const allKeys = getAllOrganizationKeys(processedOrganizations);
      setExpandedKeys(allKeys);
    }
  };

  const treeProps: TreeProps = {
    treeData,
    checkable: checkable && multiple,
    selectable: selectable && !checkable,
    multiple: selectable && multiple,
    checkedKeys: checkable ? checkedKeys : undefined,
    selectedKeys: selectable ? selectedKeys : undefined,
    expandedKeys: effectiveExpandedKeys,
    onCheck: checkable ? handleCheck : undefined,
    onSelect: selectable ? handleSelect : undefined,
    onExpand: handleExpand,
    height,
    style,
    className,
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 20 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <Card size="small" style={style} className={className}>
      {showSearch && (
        <Input
          placeholder="搜索组织/部门..."
          prefix={<SearchOutlined />}
          value={searchValue}
          onChange={handleSearchChange}
          style={{ marginBottom: 12 }}
          allowClear
        />
      )}

      {processedOrganizations.length > 0 ? (
        <Tree {...treeProps} />
      ) : (
        <Empty description="暂无组织数据" style={{ margin: '20px 0' }} />
      )}
    </Card>
  );
};

export default OrganizationTreeSelector;
