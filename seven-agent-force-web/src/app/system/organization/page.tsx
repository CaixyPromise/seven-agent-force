'use client';

import React, { useState } from 'react';
import { Card, Row, Col, Button, Modal, message, Spin, Tag } from 'antd';
import { PlusOutlined, ExclamationCircleOutlined } from '@ant-design/icons';
import { useQuery, useMutation } from '@tanstack/react-query';
import type { DataNode } from 'antd/es/tree';
import type { TreeProps } from 'antd';
import {
  getOrgTree,
  createOrg,
  updateOrg,
  deleteOrg,
  changeStatus,
  moveOrg
} from '@/api/sysOrgController';
import { OrganizationTree } from './components/OrganizationTree';
import { OrganizationModal } from './components/OrganizationModal';

type OrganizationFormValues = Pick<
  API.SysOrg,
  'name' | 'code' | 'status' | 'sortOrder' | 'leaderUserId'
>;

interface OrganizationTreeNode extends DataNode {
  nodeData?: API.SysOrg;
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export default function OrganizationManagementPage() {
  const [selectedOrg, setSelectedOrg] = useState<API.SysOrg | null>(null);
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [parentId, setParentId] = useState<API.Int64>();

  // 获取组织树
  const { data: orgTreeData, isLoading, refetch } = useQuery({
    queryKey: ['orgTree'],
    queryFn: () => getOrgTree(),
  });
  const orgTree = orgTreeData?.data ?? [];

  // 创建组织
  const createOrgMutation = useMutation({
    mutationFn: (body: Parameters<typeof createOrg>[0]) => createOrg(body),
    onSuccess: () => {
      message.success('组织创建成功');
      refetch();
      setModalVisible(false);
      setSelectedOrg(null);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '组织创建失败'));
    },
  });

  // 更新组织
  const updateOrgMutation = useMutation({
    mutationFn: (body: Parameters<typeof updateOrg>[0]) => updateOrg(body),
    onSuccess: () => {
      message.success('组织更新成功');
      refetch();
      setModalVisible(false);
      setSelectedOrg(null);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '组织更新失败'));
    },
  });

  // 删除组织
  const deleteOrgMutation = useMutation({
    mutationFn: (params: Parameters<typeof deleteOrg>[0]) => deleteOrg(params),
    onSuccess: () => {
      message.success('组织删除成功');
      refetch();
      setSelectedOrg(null);
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '组织删除失败'));
    },
  });

  // 切换状态
  const toggleStatusMutation = useMutation({
    mutationFn: (params: Parameters<typeof changeStatus>[0]) => changeStatus(params),
    onSuccess: () => {
      message.success('状态更新成功');
      refetch();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '状态更新失败'));
    },
  });

  // 移动组织
  const moveOrgMutation = useMutation({
    mutationFn: (params: Parameters<typeof moveOrg>[0]) => moveOrg(params),
    onSuccess: () => {
      message.success('组织移动成功');
      refetch();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '组织移动失败'));
    },
  });

  // 选择组织
  const handleSelect: NonNullable<TreeProps<OrganizationTreeNode>['onSelect']> = (keys, info) => {
    if (keys.length > 0) {
      const selectedNode = info.selectedNodes[0];
      const nodeData = selectedNode?.nodeData;
      if (nodeData) {
        setSelectedOrg(nodeData);
      } else {
        console.warn('无法获取选中节点的原始数据');
      }
    } else {
      setSelectedOrg(null);
    }
  };

  // 新增组织
  const handleAdd = (parentOrgId?: API.Int64) => {
    setModalMode('create');
    setParentId(parentOrgId);
    setSelectedOrg(null);
    setModalVisible(true);
  };

  // 编辑组织
  const handleEdit = (org: API.SysOrg) => {
    setModalMode('edit');
    setSelectedOrg(org);
    setParentId(undefined);
    setModalVisible(true);
  };

  // 删除组织
  const handleDelete = (org: API.SysOrg) => {
    Modal.confirm({
      title: '确认删除',
      icon: <ExclamationCircleOutlined />,
      content: (
        <div>
          <p>确定要删除组织 <strong>{org.name}</strong> 吗？</p>
          <p style={{ color: '#ff4d4f' }}>
            注意：删除后不可恢复，且会影响该组织下的所有子组织和用户！
          </p>
        </div>
      ),
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        if (org.id) {
          await deleteOrgMutation.mutateAsync({ id: org.id });
        }
      },
    });
  };

  // 状态变更
  const handleStatusChange = (org: API.SysOrg, status: number) => {
    const action = status === 0 ? '启用' : '禁用';
    Modal.confirm({
      title: `确认${action}`,
      icon: <ExclamationCircleOutlined />,
      content: `确定要${action}组织 "${org.name}" 吗？`,
      okText: `确认${action}`,
      okType: status === 1 ? 'danger' : 'primary',
      cancelText: '取消',
      onOk: async () => {
        if (org.id) {
          await toggleStatusMutation.mutateAsync({ id: org.id, status });
        }
      },
    });
  };

  // 移动组织
  const handleMove: NonNullable<TreeProps<OrganizationTreeNode>['onDrop']> = dragInfo => {
    const dragKey = String(dragInfo.dragNode.key);
    const dropKey = String(dragInfo.node.key);
    const { dropToGap } = dragInfo;

    if (dragKey === dropKey) {
      message.warning('不能移动到自身');
      return;
    }

    const newParentId = dropToGap ? '0' : dropKey;

    Modal.confirm({
      title: '确认移动',
      content: '确定要移动该组织吗？',
      onOk: async () => {
        await moveOrgMutation.mutateAsync({
          id: dragKey,
          newParentId: newParentId,
        });
      },
    });
  };

  // 关闭模态框
  const handleModalCancel = () => {
    setModalVisible(false);
    setSelectedOrg(null);
    setParentId(undefined);
  };

  // 处理模态框提交
  const handleModalOk = async (values: OrganizationFormValues) => {
    if (modalMode === 'create') {
      await createOrgMutation.mutateAsync({
        ...values,
        parentId: parentId || '0',
      });
    } else if (selectedOrg?.id) {
      await updateOrgMutation.mutateAsync({
        ...values,
        id: selectedOrg.id,
      });
    }
  };

  return (
    <>
      <Row gutter={[16, 16]}>
        {/* 组织树 */}
        <Col xs={24} lg={16}>
          <Card
            title="组织架构"
            extra={
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={() => handleAdd()}
              >
                新增组织
              </Button>
            }
            style={{ height: '700px', overflow: 'auto' }}
          >
            <Spin spinning={isLoading}>
              <OrganizationTree
                data={orgTree}
                loading={isLoading}
                selectedKeys={selectedOrg?.id ? [selectedOrg.id] : []}
                onSelect={handleSelect}
                onAdd={handleAdd}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onStatusChange={handleStatusChange}
                onMove={handleMove}
              />
            </Spin>
          </Card>
        </Col>

        {/* 组织详情 */}
        <Col xs={24} lg={8}>
          <Card title="组织详情" style={{ height: '700px' }}>
            {selectedOrg ? (
              <div style={{ padding: '16px 0' }}>
                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    组织名称：
                  </label>
                  <span>{selectedOrg.name}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    组织编码：
                  </label>
                  <span>{selectedOrg.code}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    状态：
                  </label>
                  <span style={{ color: selectedOrg.status === 0 ? '#52c41a' : '#ff4d4f' }}>
                    {selectedOrg.status === 0 ? '启用' : '禁用'}
                  </span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    排序：
                  </label>
                  <span>{selectedOrg.sortOrder}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    负责人：
                  </label>
                  <span>
                    {selectedOrg.leaderUserId ? (
                      <Tag color="blue">用户ID: {selectedOrg.leaderUserId}</Tag>
                    ) : (
                      '暂未设置'
                    )}
                  </span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    创建时间：
                  </label>
                  <span>{selectedOrg.createTime}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>
                    更新时间：
                  </label>
                  <span>{selectedOrg.updateTime}</span>
                </div>

                <div style={{ marginTop: 24, display: 'flex', gap: 8 }}>
                  <Button type="primary" onClick={() => handleEdit(selectedOrg)}>
                    编辑
                  </Button>
                  <Button danger onClick={() => handleDelete(selectedOrg)}>
                    删除
                  </Button>
                </div>
              </div>
            ) : (
              <div style={{ textAlign: 'center', padding: '60px 0', color: '#999' }}>
                请选择组织查看详情
              </div>
            )}
          </Card>
        </Col>
      </Row>

      {/* 新增/编辑模态框 */}
      <OrganizationModal
        visible={modalVisible}
        mode={modalMode}
        initialValues={selectedOrg || undefined}
        onOk={handleModalOk}
        onCancel={handleModalCancel}
      />
    </>
  );
}
