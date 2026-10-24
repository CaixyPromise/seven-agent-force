'use client';

import React, { useState, useRef } from 'react';
import { Alert, Button, Modal, message, Tag, Space, Dropdown } from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  SafetyOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { ProTable } from '@ant-design/pro-components';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useIsMounted } from '@/hooks/useIsMounted';
import {
  getRolePage,
  createRole,
  updateRole,
  deleteRole,
  getRoleSecurityStatus,
} from '@/api/sysRoleController';
import { RoleForm } from './components/RoleForm';
import type { RoleFormRef } from './components/RoleForm';
import { RoleGrantDrawer } from './components/RoleGrantDrawer';
import { usePermissionFlags } from '@/hooks/auth';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { CONFIG_PERMISSIONS, ROLE_PERMISSIONS } from '@/lib/auth/permissionCodes';

function getErrorMessage(error: unknown, fallback: string) {
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

export default function RoleManagementPage() {
  const queryClient = useQueryClient();
  const {
    canCreateRole,
    canQueryRole,
    canEditRole,
    canDeleteRole,
    canGrantRole,
    canQueryConfigScope,
    canAssignConfigScope,
  } = usePermissionFlags({
    canQueryRole: ROLE_PERMISSIONS.QUERY,
    canCreateRole: ROLE_PERMISSIONS.ADD,
    canEditRole: ROLE_PERMISSIONS.EDIT,
    canDeleteRole: ROLE_PERMISSIONS.REMOVE,
    canGrantRole: ROLE_PERMISSIONS.GRANT,
    canQueryConfigScope: CONFIG_PERMISSIONS.SCOPE_QUERY,
    canAssignConfigScope: CONFIG_PERMISSIONS.SCOPE_ASSIGN,
  });

  // 基础状态
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [currentRole, setCurrentRole] = useState<API.RoleVO | undefined>();
  const [grantVisible, setGrantVisible] = useState(false);
  const [selectedRoleForGrant, setSelectedRoleForGrant] = useState<API.RoleVO | undefined>();
  const actionRef = useRef<ActionType>(undefined);
  const formRef = useRef<RoleFormRef>(null);
  const isMountedRef = useIsMounted();
  const { data: securityStatus } = useQuery({
    queryKey: ['role-security-status'],
    queryFn: async () => (await getRoleSecurityStatus()).data,
    staleTime: 30_000,
    retry: false,
    enabled: canQueryRole,
  });
  const invalidateAuthAuthorization = () => {
    void queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY });
    void queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });
  };

  // 创建角色
  const createRoleMutation = useMutation({
    mutationFn: (body: Parameters<typeof createRole>[0]) => createRole(body),
    onSuccess: () => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.success('角色创建成功');
      invalidateAuthAuthorization();
      actionRef.current?.reload();
      setModalVisible(false);
      setCurrentRole(undefined);
    },
    onError: (error: unknown) => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.error(getErrorMessage(error, '角色创建失败'));
    },
  });

  // 更新角色
  const updateRoleMutation = useMutation({
    mutationFn: (body: Parameters<typeof updateRole>[0]) => updateRole(body),
    onSuccess: () => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.success('角色更新成功');
      invalidateAuthAuthorization();
      actionRef.current?.reload();
      setModalVisible(false);
      setCurrentRole(undefined);
    },
    onError: (error: unknown) => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.error(getErrorMessage(error, '角色更新失败'));
    },
  });

  // 删除角色
  const deleteRoleMutation = useMutation({
    mutationFn: (params: Parameters<typeof deleteRole>[0]) => deleteRole(params),
    onSuccess: () => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.success('角色删除成功');
      invalidateAuthAuthorization();
      actionRef.current?.reload();
    },
    onError: (error: unknown) => {
      if (!isMountedRef.current) return; // 组件已卸载
      message.error(getErrorMessage(error, '角色删除失败'));
    },
  });

  // 表格列定义
  const columns: ProColumns<API.RoleVO>[] = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 80,
      search: false,
    },
    {
      title: '角色名称',
      dataIndex: 'name',
      key: 'name',
      render: (text) => <span style={{ fontWeight: 500 }}>{text}</span>,
    },
    {
      title: '角色编码',
      dataIndex: 'code',
      key: 'code',
      render: (text) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: '角色类型',
      dataIndex: 'type',
      key: 'type',
      valueEnum: {
        SYSTEM: { text: '系统角色', status: 'Error' },
        BUSINESS: { text: '业务角色', status: 'Processing' },
        CUSTOM: { text: '自定义角色', status: 'Success' },
      },
      width: 120,
    },
    {
      title: '数据权限',
      dataIndex: 'dataScope',
      key: 'dataScope',
      search: false,
      width: 120,
      render: (_, record) => {
        const dataScope = record.dataScope;
        const scopeMap = {
          1: { text: '全部数据', color: 'red' },
          2: { text: '自定数据', color: 'orange' },
          3: { text: '本部门数据', color: 'blue' },
          4: { text: '本部门及以下', color: 'cyan' },
          5: { text: '仅本人数据', color: 'green' },
        };
        const scope = scopeMap[dataScope as keyof typeof scopeMap] || { text: '未知', color: 'default' };
        return <Tag color={scope.color}>{scope.text}</Tag>;
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      valueEnum: {
        0: { text: '启用', status: 'Success' },
        1: { text: '禁用', status: 'Error' },
      },
      width: 100,
    },
    {
      title: '排序',
      dataIndex: 'sortOrder',
      key: 'sortOrder',
      search: false,
      width: 80,
    },
    {
      title: '描述',
      dataIndex: 'remark',
      key: 'remark',
      ellipsis: true,
      search: false,
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      valueType: 'dateTime',
      search: false,
      width: 160,
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 170,
      search: false,
      render: (_, record) => {
        const moreItems = [];

        if (canGrantRole) {
          moreItems.push({
            key: 'atomic-grant',
            icon: <SafetyOutlined />,
            label: record.authorizationRoot ? '查看系统授权' : '统一授权',
            onClick: () => handleGrant(record),
          });
        }

        if (canDeleteRole) {
          if (moreItems.length > 0) {
            moreItems.push({
              type: 'divider' as const,
            });
          }

          if (record.type === 'SYSTEM') {
            moreItems.push({
              key: 'delete-protected',
              icon: <DeleteOutlined />,
              label: 'SYSTEM 角色不可删除',
              disabled: true,
            });
          } else {
            moreItems.push({
              key: 'delete',
              icon: <DeleteOutlined />,
              label: '删除',
              danger: true,
              onClick: () =>
                Modal.confirm({
                  title: '删除角色',
                  content: `确定要删除角色 "${record.name}" 吗？删除后不可恢复！`,
                  okText: '确定删除',
                  cancelText: '取消',
                  okButtonProps: { danger: true },
                  onOk: () => handleDelete(record),
                }),
            });
          }
        }

        return (
          <Space size="small" wrap={false}>
            {canEditRole ? (
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={() => handleEdit(record)}
              >
                编辑
              </Button>
            ) : null}

            {moreItems.length > 0 ? (
              <Dropdown menu={{ items: moreItems }} trigger={['click']}>
                <Button type="link" size="small" icon={<MoreOutlined />}>
                  更多
                </Button>
              </Dropdown>
            ) : null}
          </Space>
        );
      },
    },
  ];

  // 新增角色
  const handleAdd = () => {
    setModalMode('create');
    setCurrentRole(undefined);
    setModalVisible(true);
  };

  // 编辑角色
  const handleEdit = (record: API.RoleVO) => {
    setModalMode('edit');
    setCurrentRole(record);
    setModalVisible(true);
  };

  // 删除角色
  const handleDelete = async (record: API.RoleVO) => {
    if (record.id) {
      await deleteRoleMutation.mutateAsync({ id: record.id });
    }
  };

  const handleGrant = (record: API.RoleVO) => {
    setSelectedRoleForGrant(record);
    setGrantVisible(true);
  };

  // 提交表单
  const handleSubmit = async () => {
    try {
      const values = await formRef.current?.validateFields();
      if (!values) return;

      if (modalMode === 'create') {
        await createRoleMutation.mutateAsync(values);
      } else if (currentRole?.id) {
        const protectedValues = {
          dataScope: currentRole.dataScope,
          ...(currentRole.type === 'SYSTEM'
            ? {
              code: currentRole.code,
              type: currentRole.type,
              status: currentRole.status,
              }
            : {}),
        };
        await updateRoleMutation.mutateAsync({
          ...values,
          ...protectedValues,
          id: currentRole.id,
        });
      }
    } catch (error) {
      console.error('表单验证失败:', error);
    }
  };

  // 关闭模态框
  const handleCancel = () => {
    setModalVisible(false);
    setCurrentRole(undefined);
    formRef.current?.resetFields();
  };

  return (
    <>
      {securityStatus?.health === 'LOW_REDUNDANCY' ? (
        <Alert
          type="warning"
          showIcon
          style={{ marginBottom: 16 }}
          title="安全根管理员冗余不足"
          description={`当前有 ${securityStatus.activeDirectAdmins ?? 0} 名有效直接管理员，建议至少保留 ${securityStatus.recommendedMinimum ?? 2} 名。此告警不阻断日常管理操作。`}
        />
      ) : null}
      <ProTable<API.RoleVO>
        headerTitle="角色列表"
        actionRef={actionRef}
        rowKey="id"
        search={{
          labelWidth: 120,
          collapsed: false,
        }}
        toolBarRender={() =>
          canCreateRole
            ? [
                <Button
                  key="add"
                  type="primary"
                  icon={<PlusOutlined />}
                  onClick={handleAdd}
                >
                  新增角色
                </Button>,
              ]
            : []
        }
        request={async (params) => {
          try {
            const result = await getRolePage({
              current: Number(params.current || 1),
              size: Number(params.pageSize || 10),
              name: params.name ? String(params.name) : undefined,
              status: params.status === undefined ? undefined : Number(params.status),
            });

            return {
              data: result.data?.records ?? [],
              success: result.code === 200 || result.code === 0,
              total: result.data?.total ?? 0,
            };
          } catch (error) {
            console.error('获取角色列表失败:', error);
            return {
              data: [],
              success: false,
              total: 0,
            };
          }
        }}
        columns={columns}
        pagination={{
          defaultPageSize: 10,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total, range) =>
            `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
        }}
        scroll={{ x: 1200 }}
      />

      {/* 新增/编辑角色模态框 */}
      <Modal
        title={modalMode === 'create' ? '新增角色' : '编辑角色'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={handleCancel}
        confirmLoading={createRoleMutation.isPending || updateRoleMutation.isPending}
        destroyOnHidden
        width={600}
        mask={{ closable: false }}
      >
        <RoleForm
          ref={formRef}
          initialValues={currentRole}
        />
      </Modal>

      <RoleGrantDrawer
        open={grantVisible}
        role={selectedRoleForGrant}
        readonly={selectedRoleForGrant?.authorizationRoot === true}
        canQueryConfigScope={canQueryConfigScope}
        canAssignConfigScope={canAssignConfigScope}
        onClose={() => {
          setGrantVisible(false);
          setSelectedRoleForGrant(undefined);
        }}
        onCommitted={() => {
          invalidateAuthAuthorization();
          actionRef.current?.reload();
        }}
      />
    </>
  );
}
