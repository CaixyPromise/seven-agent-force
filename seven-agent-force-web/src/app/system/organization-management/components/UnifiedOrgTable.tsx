import React, { useState, useCallback, useEffect } from 'react';
import {
  Table,
  Button,
  Space,
  message,
  Popconfirm,
  Badge,
  Input,
  Select,
  Form,
  Row,
  Col,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  SafetyOutlined,
  BankOutlined,
  ApartmentOutlined,
  IdcardOutlined,
  DownOutlined,
  RightOutlined,
  DragOutlined, ReloadOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
// import { DndProvider, useDrag, useDrop } from 'react-dnd';
// import { HTML5Backend } from 'react-dnd-html5-backend';
import { HasPermission } from '@/components/Permission';
import { useOrganizationManagement } from '../hooks/useOrganizationManagement';
import type { EntityType, OrganizationEntity, PostEntity } from '../hooks/useOrganizationManagement';
import EntityModal from './EntityModal';
import RoleAssignDrawer from './RoleAssignDrawer';

// 统一的表格行数据类型
interface UnifiedTableRow {
  id: API.Int64;
  name: string;
  code?: string;
  status: number;
  sortOrder?: number;
  createTime: string;
  updateTime?: string;
  remark?: string;
  type: 'org' | 'dept' | 'post';
  level: number; // 层级深度，用于缩进显示
  parentId?: API.Int64;
  parentName?: string;
  hasChildren?: boolean;
  children?: UnifiedTableRow[];
  uniqueKey: string; // 全局唯一键
  orgId?: API.Int64;
  deptId?: API.Int64;
  roleCount?: number;
}

// 展开状态管理
interface ExpandState {
  loading: boolean;
  children: UnifiedTableRow[];
  total: number;
  current: number;
  pageSize: number;
  hasMore: boolean;
  error?: string;
}

interface ParentContext {
  orgId?: API.Int64;
  parentId?: API.Int64;
  deptId?: API.Int64;
}

/**
 * 统一的组织管理表格组件
 * 支持组织/部门/岗位的层级展开、CRUD、拖拽排序
 */
const UnifiedOrgTable: React.FC = () => {
  // 状态管理
  const [dataSource, setDataSource] = useState<UnifiedTableRow[]>([]);
  const [expandedRowKeys, setExpandedRowKeys] = useState<readonly React.Key[]>([]);
  const [expandStates, setExpandStates] = useState<Map<string, ExpandState>>(new Map());
  const [loading, setLoading] = useState(false);

  // 弹窗和抽屉状态
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingEntity, setEditingEntity] = useState<Partial<EntityType> | null>(null);
  const [editMode, setEditMode] = useState<'create' | 'edit'>('create');
  const [roleAssignDrawerVisible, setRoleAssignDrawerVisible] = useState(false);
  const [roleAssignPost, setRoleAssignPost] = useState<PostEntity | null>(null);

  // 筛选条件
  const [filterParams, setFilterParams] = useState({
    keyword: '',
    status: undefined as number | undefined,
  });

  // Modal选项状态
  const [orgOptions, setOrgOptions] = useState<OrganizationEntity[]>([]);

  // 使用hooks
  const {
    loading: apiLoading,
    fetchOrganizations,
    fetchDepartments,
    fetchPosts,
    createEntity,
    updateEntity,
    deleteEntity,
  } = useOrganizationManagement();

  // 生成缓存键
  const generateCacheKey = useCallback((type: string, parentContext: ParentContext): string => {
    switch (type) {
      case 'org':
        return `org-${JSON.stringify(parentContext)}`;
      case 'dept':
        return `dept-${parentContext.orgId}-${parentContext.parentId || 0}`;
      case 'post':
        return `post-${parentContext.orgId}-${parentContext.deptId}`;
      default:
        return `${type}-${JSON.stringify(parentContext)}`;
    }
  }, []);

  // 加载组织列表
  const loadOrganizations = useCallback(async () => {
    setLoading(true);
    try {
      const result = await fetchOrganizations(filterParams);
      const orgRows: UnifiedTableRow[] = result.data.map(org => ({
        ...org,
        level: 0,
        hasChildren: true,
        parentName: '顶级组织',
        uniqueKey: `org-${org.id}`,
      }));
      setDataSource(orgRows);
      setOrgOptions(result.data); // 供 Modal 使用
    } catch (error) {
      console.error('加载组织列表失败:', error);
      message.error('加载组织列表失败');
    } finally {
      setLoading(false);
    }
  }, [fetchOrganizations, filterParams]);

  // 加载子数据（部门或岗位）
  const loadChildren = useCallback(async (
    parentRow: UnifiedTableRow,
    type: 'dept' | 'post',
    page: number = 1
  ) => {
    const cacheKey = generateCacheKey(type, {
      orgId: parentRow.orgId || parentRow.id,
      parentId: type === 'dept' ? (parentRow.type === 'org' ? '0' : parentRow.id) : parentRow.id,
      deptId: type === 'post' ? parentRow.id : undefined,
    });

    setExpandStates(prev => new Map(prev).set(cacheKey, {
      loading: true,
      children: [],
      total: 0,
      current: page,
      pageSize: 10,
      hasMore: false,
    }));

    try {
      let result;
      if (type === 'dept') {
        result = await fetchDepartments({
          orgId: String(parentRow.orgId || parentRow.id),
          parentId: parentRow.type === 'org' ? '0' : String(parentRow.id),
          current: page,
          size: 10,
          ...filterParams,
        });
      } else {
        result = await fetchPosts({
          orgId: String(parentRow.orgId || parentRow.id),
          deptId: String(parentRow.id),
          current: page,
          size: 10,
          ...filterParams,
        });
      }

      const children: UnifiedTableRow[] = result.data.map((item) => ({
        ...item,
        level: parentRow.level + 1,
        parentId: parentRow.id,
        parentName: parentRow.name,
        hasChildren: type === 'dept', // 部门可以展开，岗位不能展开
        uniqueKey: `${type}-${item.id}`,
      }));

      // 添加调试日志
      console.log(`加载${type === 'dept' ? '部门' : '岗位'}数据:`, {
        parentRow: parentRow.name,
        type,
        result: result.data,
        children: children,
        cacheKey
      });

      setExpandStates(prev => new Map(prev).set(cacheKey, {
        loading: false,
        children,
        total: result.total,
        current: result.current,
        pageSize: result.size,
        hasMore: result.data.length >= 10,
      }));
    } catch (error) {
      console.error(`加载${type === 'dept' ? '部门' : '岗位'}失败:`, error);
      setExpandStates(prev => new Map(prev).set(cacheKey, {
        loading: false,
        children: [],
        total: 0,
        current: page,
        pageSize: 10,
        hasMore: false,
        error: `加载${type === 'dept' ? '部门' : '岗位'}失败`,
      }));
    }
  }, [fetchDepartments, fetchPosts, filterParams, generateCacheKey]);

  // 处理行展开
  const handleExpand = useCallback((expanded: boolean, record: UnifiedTableRow) => {
    if (!expanded) {
      setExpandedRowKeys(prev => prev.filter(key => key !== record.uniqueKey));
      return;
    }

    setExpandedRowKeys(prev => [...prev, record.uniqueKey]);

    // 根据记录类型加载子数据
    if (record.type === 'org') {
      // 组织展开时加载部门
      loadChildren(record, 'dept');
    } else if (record.type === 'dept') {
      // 部门展开时同时加载子部门和岗位
      loadChildren(record, 'dept'); // 子部门
      loadChildren(record, 'post'); // 岗位
    }
  }, [loadChildren]);

  // 处理新建
  const handleCreate = useCallback(async (
    type: EntityType['type'],
    parentContext?: ParentContext,
  ) => {
    setEditMode('create');

    setEditingEntity({
      type,
      ...parentContext,
    } as Partial<EntityType>);
    setEditModalVisible(true);
  }, []);

  // 处理新建子项
  const handleCreateChild = useCallback(async (parentRow: UnifiedTableRow, type: 'dept' | 'post') => {
    setEditMode('create');

    setEditingEntity({
      type,
      orgId: parentRow.orgId || parentRow.id,
      parentId: type === 'dept' ? (parentRow.type === 'org' ? '0' : parentRow.id) : undefined,
      deptId: type === 'post' ? parentRow.id : undefined,
    } as Partial<EntityType>);
    setEditModalVisible(true);
  }, []);

  // 处理编辑
  const handleEdit = useCallback(async (record: EntityType) => {
    console.log('开始编辑实体:', record);
    setEditMode('edit');

    setEditingEntity(record);
    setEditModalVisible(true);
    console.log('编辑状态设置完成:', { editMode: 'edit', editingEntity: record });
  }, []);

  // 处理角色分配
  const handleRoleAssign = useCallback((record: PostEntity) => {
    setRoleAssignPost(record);
    setRoleAssignDrawerVisible(true);
  }, []);

  // 局部刷新数据（保持展开状态）
  // UnifiedOrgTable.tsx
  const refreshData = useCallback(async () => {
    try {
      const result = await fetchOrganizations(filterParams);
      const orgRows: UnifiedTableRow[] = result.data.map(org => ({
        ...org,
        level: 0,
        hasChildren: true,
        parentName: '顶级组织',
        uniqueKey: `org-${org.id}`,
      }));
      setDataSource(orgRows);

      // 合并更新 expandStates —— 先准备新 Map，一次 set
      const newMap = new Map(expandStates);

      const tasks: Promise<void>[] = [];
      for (const [cacheKey, expandState] of expandStates.entries()) {
        if (expandState.children.length === 0) continue;

        if (cacheKey.startsWith('dept-')) {
          const [, orgIdStr, parentIdStr] = cacheKey.split('-');
          const orgId = orgIdStr;
          const parentId = parentIdStr;
          tasks.push((async () => {
            const deptResult = await fetchDepartments({ orgId, parentId, current: 1, size: 10, ...filterParams });
            const children: UnifiedTableRow[] = deptResult.data.map((item) => ({
              ...item,
              level: parentId === '0' ? 1 : 2,
              parentId,
              hasChildren: true,
              uniqueKey: `dept-${item.id}`,
            }));
            newMap.set(cacheKey, { ...expandState, children, total: deptResult.total });
          })());
        }

        if (cacheKey.startsWith('post-')) {
          const [, orgIdStr, deptIdStr] = cacheKey.split('-');
          const orgId = orgIdStr;
          const deptId = deptIdStr;
          tasks.push((async () => {
            const postResult = await fetchPosts({ orgId, deptId, current: 1, size: 10, ...filterParams });
            const children: UnifiedTableRow[] = postResult.data.map((item) => ({
              ...item,
              level: 2,
              parentId: deptId,
              hasChildren: false,
              uniqueKey: `post-${item.id}`,
            }));
            newMap.set(cacheKey, { ...expandState, children, total: postResult.total });
          })());
        }
      }

      await Promise.all(tasks);
      setExpandStates(newMap); // 一次性 set
    } catch {
      message.error('刷新数据失败');
    }
  }, [fetchOrganizations, fetchDepartments, fetchPosts, filterParams, expandStates]);


  // 处理删除
  const handleDelete = useCallback(async (record: EntityType) => {
    try {
      await deleteEntity(record);
      message.success('删除成功');
      await refreshData();
    } catch (error) {
      message.error('删除失败: ' + (error as Error).message);
    }
  }, [deleteEntity, refreshData]);


  // 处理实体保存
  const handleEntitySave = useCallback(async (values: Partial<EntityType>) => {
    try {
      if (editMode === 'create') {
        await createEntity(values);
        message.success('创建成功');
      } else {
        await updateEntity(values as EntityType);
        message.success('更新成功');
      }
      // 先关弹窗，再刷新
      setEditModalVisible(false);
      setEditingEntity(null);
      await refreshData();
    } catch (error) {
      message.error('操作失败: ' + (error as Error).message);
    }
  }, [editMode, createEntity, updateEntity, refreshData]);


  // 统一的表格列配置
  const getUnifiedColumns = useCallback((): ColumnsType<UnifiedTableRow> => [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => {
        const icon = record.type === 'org' ? <BankOutlined /> :
                    record.type === 'dept' ? <ApartmentOutlined /> :
                    <IdcardOutlined />;

        const indent = record.level * 20;
        const typePrefix = record.type === 'org' ? '组织:' :
                          record.type === 'dept' ? '部门:' : '岗位:';

        return (
          <div style={{ paddingLeft: indent }}>
            <Space>
              {icon}
              <span style={{ fontWeight: record.level === 0 ? 500 : 400 }}>
                <span style={{ color: '#666', fontSize: '12px' }}>{typePrefix}</span>
                {text}
              </span>
              {record.code && (
                <Badge
                  count={record.code}
                  color={record.type === 'org' ? '#1890ff' :
                         record.type === 'dept' ? '#52c41a' : '#fa8c16'}
                />
              )}
              {record.type === 'post' && record.roleCount && record.roleCount > 0 && (
                <Badge count={`${record.roleCount}角色`} color="#f50" />
              )}
            </Space>
          </div>
        );
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (status: number) => (
        <Badge status={status === 0 ? 'success' : 'error'} text={status === 0 ? '正常' : '停用'} />
      ),
    },
    {
      title: '排序',
      dataIndex: 'sortOrder',
      key: 'sortOrder',
      width: 80,
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      width: 160,
    },
    {
      title: '操作',
      key: 'action',
      width: 250,
      render: (_, record) => (
        <Space size="small">
          {/* 新建子项按钮 */}
          {record.type === 'org' && (
            <HasPermission code="system:dept:add">
              <Button
                type="link"
                size="small"
                icon={<PlusOutlined />}
                onClick={() => handleCreateChild(record, 'dept')}
              >
                新建部门
              </Button>
            </HasPermission>
          )}
          {record.type === 'dept' && (
            <>
              <HasPermission code="system:dept:add">
                <Button
                  type="link"
                  size="small"
                  icon={<PlusOutlined />}
                  onClick={() => handleCreateChild(record, 'dept')}
                >
                  新建子部门
                </Button>
              </HasPermission>
              <HasPermission code="system:post:create">
                <Button
                  type="link"
                  size="small"
                  icon={<PlusOutlined />}
                  onClick={() => handleCreateChild(record, 'post')}
                >
                  新建岗位
                </Button>
              </HasPermission>
            </>
          )}

          {/* 角色分配按钮 */}
          {record.type === 'post' && (
            <HasPermission code="system:post:role">
              <Button
                type="link"
                size="small"
                icon={<SafetyOutlined />}
                onClick={() => handleRoleAssign(record as PostEntity)}
              >
                角色
              </Button>
            </HasPermission>
          )}

          {/* 编辑按钮 */}
          <HasPermission code={record.type === 'dept' ? 'system:dept:edit' : `system:${record.type}:update`}>
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record as EntityType)}
            >
              编辑
            </Button>
          </HasPermission>

          {/* 删除按钮 */}
          <HasPermission code={record.type === 'dept' ? 'system:dept:remove' : `system:${record.type}:delete`}>
            <Popconfirm
              title="确定删除？"
              onConfirm={() => {
                console.log('删除确认被点击:', record);
                handleDelete(record as EntityType);
              }}
              okText="确定"
              cancelText="取消"
            >
              <Button
                type="link"
                size="small"
                danger
                icon={<DeleteOutlined />}
                onClick={() => console.log('删除按钮被点击:', record)}
              >
                删除
              </Button>
            </Popconfirm>
          </HasPermission>

          {/* 拖拽按钮 */}
          {record.level > 0 && (
            <Button
              type="link"
              size="small"
              icon={<DragOutlined />}
              style={{ cursor: 'move' }}
            >
              拖拽
            </Button>
          )}
        </Space>
      ),
    },
  ], [handleCreateChild, handleRoleAssign, handleEdit, handleDelete]);

  // 渲染展开行内容
  const expandedRowRender = useCallback((record: UnifiedTableRow) => {
    const deptChildren: UnifiedTableRow[] = [];
    const postChildren: UnifiedTableRow[] = [];

    // 获取部门子数据
    if (record.type === 'org' || record.type === 'dept') {
      const deptCacheKey = generateCacheKey('dept', {
        orgId: record.orgId || record.id,
        parentId: record.type === 'org' ? '0' : record.id,
      });
      const deptState = expandStates.get(deptCacheKey);
      if (deptState && !deptState.loading && !deptState.error) {
        deptChildren.push(...deptState.children);
      }
    }

    // 获取岗位子数据
    if (record.type === 'dept') {
      const postCacheKey = generateCacheKey('post', {
        orgId: record.orgId || record.id,
        deptId: record.id,
      });
      const postState = expandStates.get(postCacheKey);
      console.log('获取岗位数据:', {
        record: record.name,
        postCacheKey,
        postState,
        postChildren: postState?.children
      });
      if (postState && !postState.loading && !postState.error) {
        postChildren.push(...postState.children);
      }
    }

    // 如果没有子数据，显示空状态
    if (deptChildren.length === 0 && postChildren.length === 0) {
      return (
        <div style={{ padding: 16, textAlign: 'center', color: '#999' }}>
          暂无数据
        </div>
      );
    }

    return (
      <div>
        {/* 子部门表格 */}
        {deptChildren.length > 0 && (
          <div style={{
            marginBottom: 16,
            marginLeft: 20,
            paddingLeft: 16,
            borderLeft: '3px solid #1890ff',
            backgroundColor: '#f0f8ff'
          }}>
            <div style={{
              marginBottom: 8,
              fontWeight: 'bold',
              color: '#1890ff',
              fontSize: '14px'
            }}>
              📁 {record.name} 的 子部门 ({deptChildren.length})
            </div>
            <Table
              dataSource={deptChildren}
              columns={getUnifiedColumns()}
              rowKey="uniqueKey"
              pagination={false}
              showHeader={true}
              size="small"
              expandable={{
                childrenColumnName:"__ignore_children",
                expandedRowKeys: expandedRowKeys,
                onExpand: handleExpand,
                expandedRowRender: expandedRowRender,
                rowExpandable: (childRecord) => childRecord.type === 'dept',
              }}
            />
          </div>
        )}

        {/* 岗位表格 */}
        {postChildren.length > 0 && (
          <div style={{
            marginLeft: 20,
            paddingLeft: 16,
            borderLeft: '3px solid #52c41a',
            backgroundColor: '#f6ffed'
          }}>
            <div style={{
              marginBottom: 8,
              fontWeight: 'bold',
              color: '#52c41a',
              fontSize: '14px'
            }}>
              👥 {record.name} 的 岗位 ({postChildren.length})
            </div>
            <Table
              dataSource={postChildren}
              columns={getUnifiedColumns()}
              rowKey="uniqueKey"
              pagination={false}
              showHeader={true}
              size="small"
            />
          </div>
        )}
      </div>
    );
  }, [expandedRowKeys, expandStates, generateCacheKey, getUnifiedColumns, handleExpand]);

  // 初始化数据
  useEffect(() => {
    loadOrganizations();
  }, [loadOrganizations]);

  return (
    <div>
        {/* 搜索和操作栏 */}
        <Row gutter={16} style={{ marginBottom: 16 }}>
          <Col flex="auto">
            <Form layout="inline" onFinish={setFilterParams}>
              <Form.Item name="keyword">
                <Input placeholder="搜索名称或编码" allowClear />
              </Form.Item>
              <Form.Item name="status">
                <Select placeholder="状态" allowClear style={{ width: 100 }}>
                  <Select.Option value={0}>正常</Select.Option>
                  <Select.Option value={1}>停用</Select.Option>
                </Select>
              </Form.Item>
              <Form.Item>
                <Button type="primary" htmlType="submit">
                  搜索
                </Button>
              </Form.Item>
            </Form>
          </Col>
          <Col flex="none">
            <Space>
              <Button icon={<ReloadOutlined />} onClick={refreshData} loading={loading}>
                刷新
              </Button>
              <HasPermission code="system:org:create">
                <Button type="primary" icon={<PlusOutlined />} onClick={() => handleCreate('org')}>
                  新建组织
                </Button>
              </HasPermission>
            </Space>
          </Col>
        </Row>

        {/* 统一表格 */}
        <Table
          rowKey="uniqueKey"
          dataSource={dataSource}
          columns={getUnifiedColumns()}

          loading={loading || apiLoading}
          expandable={{
            childrenColumnName:"__ignore_children",
            onExpand: handleExpand,
            expandedRowKeys: expandedRowKeys,
            expandedRowRender: expandedRowRender,
            expandIcon: ({ expanded, onExpand, record }) => (
              expanded ? (
                <DownOutlined onClick={e => onExpand(record, e)} />
              ) : (
                <RightOutlined onClick={e => onExpand(record, e)} />
              )
            ),
            rowExpandable: (record) => record.type === 'org' || record.type === 'dept',
          }}
          pagination={{
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
          }}
        />

        {/* 实体编辑弹窗 */}
        <EntityModal
          visible={editModalVisible}
          mode={editMode}
          entity={editingEntity}
          organizationOptions={orgOptions}
          onCancel={() => {
            setEditModalVisible(false);
            setEditingEntity(null);
          }}
          onSubmit={handleEntitySave}
        />

        {/* 角色分配抽屉 */}
        <RoleAssignDrawer
          visible={roleAssignDrawerVisible}
          post={roleAssignPost}
          onClose={() => {
            setRoleAssignDrawerVisible(false);
            setRoleAssignPost(null);
          }}
          onSuccess={() => {
            // 角色分配成功后局部刷新数据，保持展开状态
            refreshData();
          }}
        />
      </div>
  );
};


export default UnifiedOrgTable;
