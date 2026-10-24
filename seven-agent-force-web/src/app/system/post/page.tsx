'use client';

import React, { useState, useRef } from 'react';
import {
  Button,
  Tag,
  Space,
  Modal,
  message,
  Dropdown,
} from 'antd';
import {
  PlusOutlined,
  EditOutlined,
  DeleteOutlined,
  ExclamationCircleOutlined,
  SafetyOutlined,
  MoreOutlined,
} from '@ant-design/icons';
import { ProTable } from '@ant-design/pro-components';
import type { ProColumns, ActionType } from '@ant-design/pro-components';
import { useMutation } from '@tanstack/react-query';
import {
  getPostPage,
  createPost,
  updatePost,
  deletePost,
  batchDeletePosts,
  changePostStatus,
} from '@/api/sysPostController';
import { PostModal, type PostFormValues } from './components/PostModal';
import { PostRoleAssign } from './components/PostRoleAssign';

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function PostManagementPage() {
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [currentPost, setCurrentPost] = useState<API.SysPost | undefined>();
  const [roleAssignVisible, setRoleAssignVisible] = useState(false);
  const [selectedPostForRole, setSelectedPostForRole] = useState<API.SysPost | undefined>();
  const actionRef = useRef<ActionType>(undefined);

    // 创建岗位
  const createPostMutation = useMutation({
    mutationFn: (body: Parameters<typeof createPost>[0]) => createPost(body),
    onSuccess: () => {
      message.success('岗位创建成功');
      actionRef.current?.reload();
      setModalVisible(false);
      setCurrentPost(undefined);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '岗位创建失败'));
    },
  });

  // 更新岗位
  const updatePostMutation = useMutation({
    mutationFn: (body: Parameters<typeof updatePost>[0]) => updatePost(body),
    onSuccess: () => {
      message.success('岗位更新成功');
      actionRef.current?.reload();
      setModalVisible(false);
      setCurrentPost(undefined);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '岗位更新失败'));
    },
  });

  // 删除岗位
  const deletePostMutation = useMutation({
    mutationFn: (params: Parameters<typeof deletePost>[0]) => deletePost(params),
    onSuccess: () => {
      message.success('岗位删除成功');
      actionRef.current?.reload();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '岗位删除失败'));
    },
  });

  // 批量删除岗位
  const batchDeleteMutation = useMutation({
    mutationFn: (body: Parameters<typeof batchDeletePosts>[0]) => batchDeletePosts(body),
    onSuccess: () => {
      message.success('批量删除成功');
      actionRef.current?.reload();
      setSelectedRowKeys([]);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '批量删除失败'));
    },
  });

  // 更改岗位状态
  const changeStatusMutation = useMutation({
    mutationFn: (params: Parameters<typeof changePostStatus>[0]) => changePostStatus(params),
    onSuccess: () => {
      message.success('状态更新成功');
      actionRef.current?.reload();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '状态更新失败'));
    },
  });

  // 表格列定义
  const columns: ProColumns<API.SysPost>[] = [
    {
      title: '岗位名称',
      dataIndex: 'name',
      key: 'name',
      render: (text) => <span style={{ fontWeight: 500 }}>{text}</span>,
    },
    {
      title: '岗位编码',
      dataIndex: 'code',
      key: 'code',
      render: (text) => <Tag color="blue">{text}</Tag>,
    },
    {
      title: '显示排序',
      dataIndex: 'sortOrder',
      key: 'sortOrder',
      sorter: true,
      width: 120,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (_, record) => (
        <Tag color={record.status === 0 ? 'green' : 'red'}>
          {record.status === 0 ? '启用' : '禁用'}
        </Tag>
      ),
      filters: [
        { text: '启用', value: 0 },
        { text: '禁用', value: 1 },
      ],
      width: 100,
    },
    {
      title: '备注',
      dataIndex: 'remark',
      key: 'remark',
      ellipsis: true,
      search: false,
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      search: false,
      width: 180,
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 170,
      search: false,
      render: (_, record) => {
        const moreItems = [
          {
            key: 'toggle-status',
            label: record.status === 0 ? '禁用' : '启用',
            danger: record.status === 0,
            onClick: () =>
              Modal.confirm({
                title: `确定要${record.status === 0 ? '禁用' : '启用'}该岗位吗？`,
                okText: '确定',
                cancelText: '取消',
                okButtonProps: record.status === 0 ? { danger: true } : undefined,
                onOk: () => handleStatusChange(record),
              }),
          },
          {
            key: 'assign-role',
            icon: <SafetyOutlined />,
            label: '分配角色',
            onClick: () => handleAssignRoles(record),
          },
          {
            type: 'divider' as const,
          },
          {
            key: 'delete',
            icon: <DeleteOutlined />,
            label: '删除',
            danger: true,
            onClick: () =>
              Modal.confirm({
                title: '确定要删除该岗位吗？',
                content: '删除后不可恢复，请谨慎操作',
                okText: '确定删除',
                cancelText: '取消',
                okButtonProps: { danger: true },
                onOk: () => handleDelete(record),
              }),
          },
        ];

        return (
          <Space size="small" wrap={false}>
            <Button
              type="link"
              size="small"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            >
              编辑
            </Button>

            <Dropdown menu={{ items: moreItems }} trigger={['click']}>
              <Button type="link" size="small" icon={<MoreOutlined />}>
                更多
              </Button>
            </Dropdown>
          </Space>
        );
      },
    },
  ];

  // 新增岗位
  const handleAdd = () => {
    setModalMode('create');
    setCurrentPost(undefined);
    setModalVisible(true);
  };

  // 编辑岗位
  const handleEdit = (record: API.SysPost) => {
    setModalMode('edit');
    setCurrentPost(record);
    setModalVisible(true);
  };

  // 删除岗位
  const handleDelete = async (record: API.SysPost) => {
    if (record.id) {
      await deletePostMutation.mutateAsync({ id: record.id });
    }
  };

  // 分配角色
  const handleAssignRoles = (record: API.SysPost) => {
    setSelectedPostForRole(record);
    setRoleAssignVisible(true);
  };

  // 批量删除
  const handleBatchDelete = async () => {
    if (selectedRowKeys.length === 0) {
      message.warning('请选择要删除的岗位');
      return;
    }

    Modal.confirm({
      title: '确认批量删除',
      icon: <ExclamationCircleOutlined />,
      content: `确定要删除选中的 ${selectedRowKeys.length} 个岗位吗？删除后不可恢复！`,
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        const ids = selectedRowKeys.map(String);
        await batchDeleteMutation.mutateAsync(ids);
      },
    });
  };

  // 状态变更
  const handleStatusChange = async (record: API.SysPost) => {
    if (record.id) {
      const newStatus = record.status === 0 ? 1 : 0;
      await changeStatusMutation.mutateAsync({ id: record.id, status: newStatus });
    }
  };

  // 关闭模态框
  const handleModalCancel = () => {
    setModalVisible(false);
    setCurrentPost(undefined);
  };

  // 处理模态框提交
  const handleModalOk = async (values: PostFormValues) => {
    if (modalMode === 'create') {
      await createPostMutation.mutateAsync(values);
    } else if (currentPost?.id) {
      await updatePostMutation.mutateAsync({
        ...values,
        id: currentPost.id,
      });
    }
  };

  // 表格行选择
  const rowSelection = {
    selectedRowKeys,
    onChange: (keys: React.Key[]) => {
      setSelectedRowKeys(keys);
    },
    getCheckboxProps: (record: API.SysPost) => ({
      name: record.name,
    }),
  };

  return (
    <>
      <ProTable<API.SysPost>
        actionRef={actionRef}
        rowKey="id"
        search={{
          labelWidth: 120,
          collapsed: false,
          collapseRender: false,
        }}
        toolBarRender={() => [
          <Button
            key="add"
            type="primary"
            icon={<PlusOutlined />}
            onClick={handleAdd}
          >
            新增岗位
          </Button>,
          <Button
            key="batchDelete"
            danger
            icon={<DeleteOutlined />}
            disabled={selectedRowKeys.length === 0}
            onClick={handleBatchDelete}
          >
            批量删除
          </Button>,
        ]}
        request={async (params) => {
          try {
            const result = await getPostPage({
              current: Number(params.current || 1),
              size: Number(params.pageSize || 10),
              name: params.name ? String(params.name) : undefined,
              code: params.code ? String(params.code) : undefined,
              status: params.status === undefined ? undefined : Number(params.status),
            });

            return {
              data: result.data?.records ?? [],
              success: result.code === 200 || result.code === 0,
              total: result.data?.total ?? 0,
            };
          } catch (error) {
            console.error('获取岗位列表失败:', error);
            return {
              data: [],
              total: 0,
              success: false,
            };
          }
        }}
        columns={columns}
        rowSelection={rowSelection}
        pagination={{
          defaultPageSize: 10,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total, range) =>
            `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
        }}
        scroll={{ x: 1000 }}
      />

      {/* 新增/编辑模态框 */}
      <PostModal
        visible={modalVisible}
        mode={modalMode}
        initialValues={currentPost}
        onOk={handleModalOk}
        onCancel={handleModalCancel}
      />

      <PostRoleAssign
        open={roleAssignVisible}
        post={selectedPostForRole}
        onClose={() => {
          setRoleAssignVisible(false);
          setSelectedPostForRole(undefined);
        }}
      />
    </>
  );
}
