import React, { useState, useEffect, useMemo } from 'react';
import { Tree, Input, Empty, Spin, Card, Tag } from 'antd';
import { SearchOutlined, ApiOutlined, MenuOutlined, BugOutlined } from '@ant-design/icons';
import type { TreeProps } from 'antd/es/tree';
import type { DataNode } from 'antd/es/tree';

export interface PermissionNode {
  id: API.Int64;
  permissionName: string;
  permissionKey: string;
  type: 'API' | 'MENU' | 'BUTTON';
  httpMethod?: string;
  path?: string;
  parentId: API.Int64;
  children?: PermissionNode[];
  createTime: Date;
  updateTime: Date;
}

interface PermissionTreeSelectorProps {
  value?: API.Int64 | API.Int64[];
  onChange?: (
    value: API.Int64 | API.Int64[] | undefined,
    permissions?: PermissionNode | PermissionNode[],
  ) => void;
  checkable?: boolean;
  selectable?: boolean;
  multiple?: boolean;
  showSearch?: boolean;
  disabled?: boolean;
  // 过滤选项
  filterByType?: ('API' | 'MENU' | 'BUTTON')[];
  filterByMethod?: string[];
  excludePermissions?: API.Int64[];
  // 自定义数据源
  dataSource?: PermissionNode[];
  onLoad?: () => Promise<PermissionNode[]>;
  // 显示选项
  showType?: boolean;
  showMethod?: boolean;
  showPath?: boolean;
  expandAll?: boolean;
  defaultExpandedKeys?: React.Key[];
  height?: number;
  style?: React.CSSProperties;
  className?: string;
}

const permissionTypeIcons: Record<PermissionNode['type'], React.ReactNode> = {
  API: <ApiOutlined style={{ color: '#1890ff' }} />,
  MENU: <MenuOutlined style={{ color: '#52c41a' }} />,
  BUTTON: <BugOutlined style={{ color: '#faad14' }} />,
};

const methodColors: Record<string, string> = {
  GET: 'blue',
  POST: 'green',
  PUT: 'orange',
  DELETE: 'red',
  PATCH: 'purple',
};

function getAllPermissionKeys(nodes: PermissionNode[]): string[] {
  return nodes.flatMap(node => [String(node.id), ...getAllPermissionKeys(node.children ?? [])]);
}

function filterPermissionNodes(
  nodes: PermissionNode[],
  filterByType: PermissionTreeSelectorProps['filterByType'],
  filterByMethod: string[] | undefined,
  excludedIds: Set<API.Int64>,
): PermissionNode[] {
  return nodes.flatMap(node => {
    if (
      excludedIds.has(node.id) ||
      (filterByType && !filterByType.includes(node.type)) ||
      (filterByMethod && node.httpMethod && !filterByMethod.includes(node.httpMethod))
    ) {
      return [];
    }
    return [{
      ...node,
      children: filterPermissionNodes(
        node.children ?? [],
        filterByType,
        filterByMethod,
        excludedIds,
      ),
    }];
  });
}

function searchPermissionNodes(
  nodes: PermissionNode[],
  keyword: string,
  ancestorMatches = false,
): PermissionNode[] {
  const normalizedKeyword = keyword.trim().toLowerCase();
  if (!normalizedKeyword) return nodes;
  return nodes.flatMap(node => {
    const matches =
      ancestorMatches ||
      node.permissionName.toLowerCase().includes(normalizedKeyword) ||
      node.permissionKey.toLowerCase().includes(normalizedKeyword) ||
      Boolean(node.path?.toLowerCase().includes(normalizedKeyword));
    const children = searchPermissionNodes(node.children ?? [], normalizedKeyword, matches);
    return matches || children.length > 0 ? [{ ...node, children }] : [];
  });
}

function findPermission(nodes: PermissionNode[], id: API.Int64): PermissionNode | undefined {
  for (const node of nodes) {
    if (node.id === id) return node;
    const child = findPermission(node.children ?? [], id);
    if (child) return child;
  }
  return undefined;
}

function buildPermissionTreeData(
  nodes: PermissionNode[],
  options: {
    disabled: boolean;
    showType: boolean;
    showMethod: boolean;
    showPath: boolean;
  },
): DataNode[] {
  return nodes.map(node => ({
    key: String(node.id),
    title: (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '2px 0' }}>
        {options.showType && permissionTypeIcons[node.type]}
        <span style={{ fontWeight: 500 }}>{node.permissionName}</span>
        <span style={{ color: '#666', fontSize: 12 }}>({node.permissionKey})</span>
        {options.showMethod && node.httpMethod && (
          <Tag color={methodColors[node.httpMethod]}>{node.httpMethod}</Tag>
        )}
        {options.showPath && node.path && (
          <span style={{ color: '#999', fontSize: 11 }}>{node.path}</span>
        )}
      </div>
    ),
    children: buildPermissionTreeData(node.children ?? [], options),
    disabled: options.disabled,
  }));
}

/**
 * 权限树选择器组件
 * 支持树形展示、搜索、多选等功能
 */
export const PermissionTreeSelector: React.FC<PermissionTreeSelectorProps> = ({
  value,
  onChange,
  checkable = true,
  selectable = false,
  multiple = true,
  showSearch = true,
  disabled = false,
  filterByType,
  filterByMethod,
  excludePermissions = [],
  dataSource,
  onLoad,
  showType = true,
  showMethod = true,
  showPath = true,
  expandAll = false,
  defaultExpandedKeys = [],
  height = 400,
  style,
  className,
}) => {
  const [loadResult, setLoadResult] = useState<{
    loader: PermissionTreeSelectorProps['onLoad'];
    data: PermissionNode[];
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
          console.error('加载权限数据失败:', error);
          setLoadResult({ loader: onLoad, data: [] });
        }
      });
    return () => {
      cancelled = true;
    };
  }, [dataSource, loadResult.loader, onLoad]);

  const loading =
    dataSource === undefined && onLoad !== undefined && loadResult.loader !== onLoad;
  const permissions = useMemo(
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

  const processedPermissions = useMemo(() => {
    const filtered = filterPermissionNodes(
      permissions,
      filterByType,
      filterByMethod,
      new Set(excludePermissions),
    );
    return searchPermissionNodes(filtered, searchValue);
  }, [permissions, searchValue, filterByType, filterByMethod, excludePermissions]);

  const treeData = useMemo(
    () =>
      buildPermissionTreeData(processedPermissions, {
        disabled,
        showType,
        showMethod,
        showPath,
      }),
    [processedPermissions, disabled, showType, showMethod, showPath],
  );
  const defaultKeys = useMemo(
    () => (expandAll ? getAllPermissionKeys(processedPermissions) : defaultExpandedKeys),
    [expandAll, processedPermissions, defaultExpandedKeys],
  );
  const effectiveExpandedKeys = expandedKeys ?? defaultKeys;

  const handleCheck: NonNullable<TreeProps['onCheck']> = checkedKeysValue => {
    const keys = Array.isArray(checkedKeysValue) ? checkedKeysValue : checkedKeysValue.checked;
    setSelection({ sourceKey: selectionSourceKey, checkedKeys: keys, selectedKeys });
    const checkedIds = keys.map(key => String(key) as API.Int64);
    const selectedPermissions = checkedIds
      .map(id => findPermission(permissions, id))
      .filter((node): node is PermissionNode => node !== undefined);
    onChange?.(checkedIds, selectedPermissions);
  };

  const handleSelect: NonNullable<TreeProps['onSelect']> = selectedKeysValue => {
    setSelection({
      sourceKey: selectionSourceKey,
      checkedKeys,
      selectedKeys: selectedKeysValue,
    });
    if (selectedKeysValue.length > 0) {
      const selectedId = String(selectedKeysValue[0]) as API.Int64;
      onChange?.(selectedId, findPermission(permissions, selectedId));
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
      const allKeys = getAllPermissionKeys(processedPermissions);
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
          placeholder="搜索权限..."
          prefix={<SearchOutlined />}
          value={searchValue}
          onChange={handleSearchChange}
          style={{ marginBottom: 12 }}
          allowClear
        />
      )}

      {processedPermissions.length > 0 ? (
        <Tree {...treeProps} />
      ) : (
        <Empty description="暂无权限数据" style={{ margin: '20px 0' }} />
      )}
    </Card>
  );
};

export default PermissionTreeSelector;
