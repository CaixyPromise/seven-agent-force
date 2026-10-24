'use client';

import React, { useMemo, useRef, useState } from 'react';
import { Button, Modal, message, Tag, Space, Popconfirm, Tree, Card } from 'antd';
import type { DataNode } from 'antd/es/tree';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  FolderOutlined,
  FileOutlined,
  AppstoreOutlined,
  ReloadOutlined,
  EyeInvisibleOutlined,
} from '@ant-design/icons';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  getMenuTree,
  getMenuById,
  createMenu,
  updateMenu,
  deleteMenu
} from '@/api/sysMenuController';
import { MenuForm } from './components/MenuForm';
import type { MenuFormRef } from './components/MenuForm';
import { usePermissionFlags } from '@/hooks/auth';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { MENU_PERMISSIONS } from '@/lib/auth/permissionCodes';

function getAllKeys(tree: API.MenuVO[]): React.Key[] {
  const keys: React.Key[] = [];
  const traverse = (nodes: API.MenuVO[]) => {
    nodes.forEach(node => {
      if (node.id) keys.push(node.id);
      if (node.children) traverse(node.children);
    });
  };
  traverse(tree);
  return keys;
}

export default function MenuManagementPage() {
  const queryClient = useQueryClient();
  const { canCreateMenu, canEditMenu, canDeleteMenu } = usePermissionFlags({
    canCreateMenu: MENU_PERMISSIONS.ADD,
    canEditMenu: MENU_PERMISSIONS.EDIT,
    canDeleteMenu: MENU_PERMISSIONS.REMOVE,
  });

  // 基础状态
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [currentMenu, setCurrentMenu] = useState<API.MenuVO | undefined>();
  const [expandedKeys, setExpandedKeys] = useState<React.Key[]>();
  const [selectedKeys, setSelectedKeys] = useState<React.Key[]>([]);
  const formRef = useRef<MenuFormRef>(null);
  const invalidateAuthAuthorization = () => {
    void queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY });
    void queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });
  };

    // 获取菜单树
  const { data: menuTreeData, isLoading, refetch } = useQuery({
    queryKey: ['menuTree'],
    queryFn: () => getMenuTree(),
  });

  const menuTree = useMemo(() => menuTreeData?.data ?? [], [menuTreeData?.data]);
  const allMenuKeys = useMemo(() => getAllKeys(menuTree), [menuTree]);
  const effectiveExpandedKeys = expandedKeys ?? allMenuKeys;

  // 创建菜单
  const createMenuMutation = useMutation({
    mutationFn: (body: Parameters<typeof createMenu>[0]) => createMenu(body),
    onSuccess: () => {
      message.success('菜单创建成功');
      invalidateAuthAuthorization();
      refetch();
      setModalVisible(false);
      setCurrentMenu(undefined);
    },
    onError: error => {
      message.error(error.message || '菜单创建失败');
    },
  });

  // 更新菜单
  const updateMenuMutation = useMutation({
    mutationFn: (body: Parameters<typeof updateMenu>[0]) => updateMenu(body),
    onSuccess: () => {
      message.success('菜单更新成功');
      invalidateAuthAuthorization();
      refetch();
      setModalVisible(false);
      setCurrentMenu(undefined);
    },
    onError: error => {
      message.error(error.message || '菜单更新失败');
    },
  });

  // 删除菜单
  const deleteMenuMutation = useMutation({
    mutationFn: (params: Parameters<typeof deleteMenu>[0]) => deleteMenu(params),
    onSuccess: () => {
      message.success('菜单删除成功');
      invalidateAuthAuthorization();
      refetch();
    },
    onError: error => {
      message.error(error.message || '菜单删除失败');
    },
  });

  // 渲染菜单树节点
  const renderTreeNode = (node: API.MenuVO) => {
    const getMenuTypeIcon = (type?: string) => {
      switch (type) {
        case 'M': return <FolderOutlined style={{ color: '#1890ff' }} />;
        case 'C': return <AppstoreOutlined style={{ color: '#52c41a' }} />;
        case 'F': return <FileOutlined style={{ color: '#faad14' }} />;
        default: return <FileOutlined style={{ color: '#999' }} />;
      }
    };

    const getMenuTypeText = (type?: string) => {
      switch (type) {
        case 'M': return '目录';
        case 'C': return '菜单';
        case 'F': return '按钮';
        default: return '未知';
      }
    };

    const getMenuTypeColor = (type?: string) => {
      switch (type) {
        case 'M': return 'blue';
        case 'C': return 'green';
        case 'F': return 'orange';
        default: return 'default';
      }
    };

    return (
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '4px 0',
        width: '100%'
      }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 8, flex: 1 }}>
          {getMenuTypeIcon(node.type)}
          <span style={{ fontWeight: 500 }}>{node.name}</span>
          <Tag color={getMenuTypeColor(node.type)}>
            {getMenuTypeText(node.type)}
          </Tag>

          {/* 路由地址 */}
          {node.path && (
            <Tag color="cyan">
              {node.path}
            </Tag>
          )}

          {/* 权限标识 */}
          {node.permission && (
            <Tag color="purple">
              {node.permission}
            </Tag>
          )}

          {/* 状态标签 */}
          {node.visible === 1 && (
            <Tag icon={<EyeInvisibleOutlined />} color="default">隐藏</Tag>
          )}
          {node.status === 1 && (
            <Tag color="red">禁用</Tag>
          )}
        </div>

        <Space size="small" onClick={(e) => e.stopPropagation()}>
          {canCreateMenu ? (
            <Button
              type="text"
              size="small"
              icon={<PlusOutlined />}
              onClick={(e) => {
                e.stopPropagation();
                handleAddChild(node);
              }}
              title="添加子菜单"
            />
          ) : null}

          {canEditMenu ? (
            <Button
              type="text"
              size="small"
              icon={<EditOutlined />}
              onClick={(e) => {
                e.stopPropagation();
                handleEdit(node);
              }}
              title="编辑"
            />
          ) : null}

          {canDeleteMenu ? (
            <Popconfirm
              title="删除菜单"
              description={
                <div>
                  <div>确定要删除菜单 <strong>&quot;{node.name}&quot;</strong> 吗？</div>
                  <div style={{ color: '#ff4d4f', fontSize: 12, marginTop: 4 }}>
                    删除后不可恢复，且会删除所有子菜单！
                  </div>
                </div>
              }
              onConfirm={(e) => {
                e?.stopPropagation();
                handleDelete(node);
              }}
              okText="确定删除"
              okType="danger"
              cancelText="取消"
            >
              <Button
                type="text"
                size="small"
                danger
                icon={<DeleteOutlined />}
                title="删除"
                onClick={(e) => e.stopPropagation()}
              />
            </Popconfirm>
          ) : null}
        </Space>
      </div>
    );
  };

  // 转换菜单树为Tree组件数据格式
  const convertToTreeData = (tree: API.MenuVO[], parentKey = 'menu-root'): DataNode[] => {
    return tree.map((node, index) => {
      const key = node.id ?? `${parentKey}:${node.name ?? node.path ?? index}`;
      return {
      key,
      title: renderTreeNode(node),
      children: node.children ? convertToTreeData(node.children, String(key)) : undefined,
    };
    });
  };

  // 新增根菜单
  const handleAdd = () => {
    setModalMode('create');
    setCurrentMenu(undefined);
    setModalVisible(true);
  };

  // 添加子菜单
  const handleAddChild = (parentNode: API.MenuVO) => {
    setModalMode('create');
    setCurrentMenu({
      parentId: parentNode.id,
    });
    setModalVisible(true);
  };

  // 编辑菜单
  const handleEdit = async (record: API.MenuVO) => {
    if (!record.id) return;

    try {
      const detail = await getMenuById({ id: record.id });
      setModalMode('edit');
      setCurrentMenu(detail?.data || undefined);
      setModalVisible(true);
    } catch (error) {
      console.error('获取菜单详情失败:', error);
    }
  };

  // 删除菜单
  const handleDelete = async (record: API.MenuVO) => {
    if (record.id) {
      await deleteMenuMutation.mutateAsync({ id: record.id });
    }
  };

  // 提交表单
  const handleSubmit = async () => {
    try {
      const values = await formRef.current?.validateFields();
      if (!values) return;

      if (modalMode === 'create') {
        await createMenuMutation.mutateAsync(values);
      } else if (currentMenu?.id) {
        await updateMenuMutation.mutateAsync({
          ...values,
          id: currentMenu.id,
        });
      }
    } catch (error) {
      console.error('表单验证失败:', error);
    }
  };

  // 关闭模态框
  const handleCancel = () => {
    setModalVisible(false);
    setCurrentMenu(undefined);
    formRef.current?.resetFields();
  };

  // 处理树节点选择
  const handleSelect = (selectedKeysValue: React.Key[]) => {
    setSelectedKeys(selectedKeysValue);
  };

  // 处理树节点展开
  const handleExpand = (expandedKeysValue: React.Key[]) => {
    setExpandedKeys(expandedKeysValue);
  };

  // 全部展开/收起
  const handleExpandAll = () => {
    if (effectiveExpandedKeys.length === 0) {
      setExpandedKeys(allMenuKeys);
    } else {
      setExpandedKeys([]);
    }
  };

  const treeData = convertToTreeData(menuTree);

  return (
    <>
      <Card>
        <div style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16
        }}>
          <h3 style={{ margin: 0 }}>菜单树</h3>
          <Space>
            <Button
              type="default"
              icon={<ReloadOutlined />}
              onClick={handleExpandAll}
            >
              {effectiveExpandedKeys.length === 0 ? '展开全部' : '收起全部'}
            </Button>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => refetch()}
              loading={isLoading}
            >
              刷新
            </Button>
            {canCreateMenu ? (
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={handleAdd}
              >
                新增菜单
              </Button>
            ) : null}
          </Space>
        </div>

        {treeData.length > 0 ? (
          <div style={{
            background: '#fafafa',
            padding: 16,
            borderRadius: 6,
            border: '1px solid #d9d9d9',
            minHeight: 400
          }}>
            <Tree
              showLine={{ showLeafIcon: false }}
              selectedKeys={selectedKeys}
              expandedKeys={effectiveExpandedKeys}
              treeData={treeData}
              onSelect={handleSelect}
              onExpand={handleExpand}
              blockNode
            />
          </div>
        ) : (
          <div style={{
            textAlign: 'center',
            padding: 60,
            color: '#999',
            background: '#fafafa',
            borderRadius: 6,
            border: '1px dashed #d9d9d9'
          }}>
            <AppstoreOutlined style={{ fontSize: 48, color: '#d9d9d9', marginBottom: 16 }} />
            <div>暂无菜单数据</div>
            <div style={{ fontSize: 12, marginTop: 8 }}>点击"新增菜单"开始配置系统菜单</div>
          </div>
        )}
      </Card>

      {/* 新增/编辑菜单模态框 */}
      <Modal
        title={modalMode === 'create' ? '新增菜单' : '编辑菜单'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={handleCancel}
        confirmLoading={createMenuMutation.isPending || updateMenuMutation.isPending}
        destroyOnHidden
        width={700}
        mask={{ closable: false }}
      >
        <MenuForm
          ref={formRef}
          initialValues={currentMenu}
          menuTree={menuTree}
        />
      </Modal>
    </>
  );
}
