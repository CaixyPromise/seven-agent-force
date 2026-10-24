'use client';

import React, { useRef, useState } from 'react';
import { Button, Modal, message, Popconfirm, Space, Tag } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  listPermissions,
  createPermission,
  updatePermission,
  deletePermission,
  getPermission,
} from '@/api/sysMenuController';
import { PermissionForm } from './components/PermissionForm';
import type { PermissionFormRef } from './components/PermissionForm';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { usePermissionFlags } from '@/hooks/auth';
import { PERMISSION_PERMISSIONS } from '@/lib/auth/permissionCodes';

export default function PermissionManagementPage() {
  const queryClient = useQueryClient();
  const { canCreatePermission, canEditPermission, canDeletePermission } = usePermissionFlags({
    canCreatePermission: PERMISSION_PERMISSIONS.ADD,
    canEditPermission: PERMISSION_PERMISSIONS.EDIT,
    canDeletePermission: PERMISSION_PERMISSIONS.REMOVE,
  });
  const actionRef = useRef<ActionType>(undefined);
  const formRef = useRef<PermissionFormRef>(null);
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [currentPermission, setCurrentPermission] = useState<API.PermissionVO | undefined>();
  const invalidateAuthAuthorization = () => {
    void queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY });
    void queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });
  };
  const reloadPermissionTable = () => {
    try {
      void actionRef.current?.reload?.();
    } catch (error) {
      console.warn('刷新权限列表失败:', error);
    }
  };

  const createPermissionMutation = useMutation({
    mutationFn: (body: Parameters<typeof createPermission>[0]) => createPermission(body),
    onSuccess: () => {
      message.success('权限创建成功');
      invalidateAuthAuthorization();
      setModalVisible(false);
      setCurrentPermission(undefined);
      formRef.current?.resetFields();
      reloadPermissionTable();
    },
    onError: error => {
      message.error(error.message || '权限创建失败');
    },
  });

  const updatePermissionMutation = useMutation({
    mutationFn: ({ id, values }: { id: API.Int64; values: API.PermissionCommandDTO }) =>
      updatePermission({ permissionId: id }, values),
    onSuccess: () => {
      message.success('权限更新成功');
      invalidateAuthAuthorization();
      setModalVisible(false);
      setCurrentPermission(undefined);
      formRef.current?.resetFields();
      reloadPermissionTable();
    },
    onError: error => {
      message.error(error.message || '权限更新失败');
    },
  });

  const deletePermissionMutation = useMutation({
    mutationFn: (params: Parameters<typeof deletePermission>[0]) => deletePermission(params),
    onSuccess: () => {
      message.success('权限删除成功');
      invalidateAuthAuthorization();
      reloadPermissionTable();
    },
    onError: error => {
      message.error(error.message || '权限删除失败');
    },
  });

  const handleAdd = () => {
    setModalMode('create');
    setCurrentPermission(undefined);
    setModalVisible(true);
  };

  const handleEdit = async (record: API.PermissionVO) => {
    if (!record.id) return;
    try {
      const detail = await getPermission({ permissionId: record.id });
      setModalMode('edit');
      setCurrentPermission(detail?.data || record);
      setModalVisible(true);
    } catch {
      message.error('获取权限详情失败');
    }
  };

  const handleDelete = async (record: API.PermissionVO) => {
    if (!record.id) return;
    await deletePermissionMutation.mutateAsync({ permissionId: record.id });
  };

  const handleSubmit = async () => {
    try {
      const values = await formRef.current?.validateFields();
      if (!values) return;

      if (modalMode === 'create') {
        await createPermissionMutation.mutateAsync(values);
      } else if (currentPermission?.id) {
        await updatePermissionMutation.mutateAsync({
          id: currentPermission.id,
          values,
        });
      }
    } catch (error) {
      console.error('表单验证失败:', error);
    }
  };

  const columns: ProColumns<API.PermissionVO>[] = [
    {
      title: '权限标识',
      dataIndex: 'code',
      key: 'code',
      width: 200,
      render: (text) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      width: 140,
      search: false,
    },
    {
      title: '资源类型',
      dataIndex: 'resourceType',
      key: 'resourceType',
      width: 120,
    },
    {
      title: '方法',
      dataIndex: 'method',
      key: 'method',
      width: 100,
    },
    {
      title: '路径',
      dataIndex: 'path',
      key: 'path',
      ellipsis: true,
      width: 220,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 80,
      render: (_, record) => (
        <Tag color={record.status === 0 ? 'green' : 'red'}>
          {record.status === 0 ? '启用' : '停用'}
        </Tag>
      ),
    },
    {
      title: '描述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      width: 200,
      search: false,
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      valueType: 'dateTime',
      width: 180,
      search: false,
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 140,
      search: false,
      render: (_, record) => {
        if (!canEditPermission && !canDeletePermission) {
          return null;
        }

        return (
          <Space size="small">
            {canEditPermission ? (
              <Button
                type="link"
                size="small"
                icon={<EditOutlined />}
                onClick={() => handleEdit(record)}
              >
                编辑
              </Button>
            ) : null}
            {canDeletePermission ? (
              <Popconfirm
                title="删除权限"
                description={`确定要删除权限 "${record.name}" 吗？删除后不可恢复！`}
                onConfirm={() => handleDelete(record)}
                okText="确定删除"
                okType="danger"
                cancelText="取消"
              >
                <Button type="link" size="small" danger icon={<DeleteOutlined />}>
                  删除
                </Button>
              </Popconfirm>
            ) : null}
          </Space>
        );
      },
    },
  ];

  return (
    <>
      <ProTable<API.PermissionVO>
        headerTitle="权限资源列表"
        actionRef={actionRef}
        rowKey="id"
        search={{
          labelWidth: 120,
          collapsed: false,
        }}
        toolBarRender={() =>
          canCreatePermission
            ? [
                <Button key="add" type="primary" icon={<PlusOutlined />} onClick={handleAdd}>
                  新增权限
                </Button>,
              ]
            : []
        }
        request={async (params) => {
          try {
            const result = await listPermissions({
              current: params.current,
              pageSize: params.pageSize,
              size: params.pageSize,
              code: params.code,
              name: params.name,
              resourceType: params.resourceType,
              method: params.method,
              path: params.path,
              status: params.status,
            });

            if (result.data) {
              const data = result.data;
              return {
                data,
                success: result.code === 0 || result.code === 200,
                total: data.length,
              };
            }

            return { data: [], success: false, total: 0 };
          } catch (error) {
            console.error('获取权限列表失败:', error);
            return { data: [], success: false, total: 0 };
          }
        }}
        columns={columns}
        pagination={{
          defaultPageSize: 10,
          showSizeChanger: true,
          showQuickJumper: true,
        }}
        scroll={{ x: 1200 }}
      />

      <Modal
        title={modalMode === 'create' ? '新增权限' : '编辑权限'}
        open={modalVisible}
        onOk={handleSubmit}
        onCancel={() => {
          setModalVisible(false);
          setCurrentPermission(undefined);
          formRef.current?.resetFields();
        }}
        confirmLoading={createPermissionMutation.isPending || updatePermissionMutation.isPending}
        destroyOnHidden
        width={600}
        mask={{ closable: false }}
      >
        <PermissionForm ref={formRef} initialValues={currentPermission} />
      </Modal>
    </>
  );
}
