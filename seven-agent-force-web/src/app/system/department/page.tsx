'use client';

import React, { useState } from 'react';
import { Card, Row, Col, Button, Modal, message, Spin, Tag } from 'antd';
import { PlusOutlined, ExclamationCircleOutlined, UserOutlined, TeamOutlined, SafetyOutlined } from '@ant-design/icons';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
  getDeptTree,
  createDept,
  updateDept,
  deleteDept
} from '@/api/sysDeptController';
import { getOrgTree } from '@/api/sysOrgController';
import { DepartmentTree } from './components/DepartmentTree';
import type { DepartmentTreeProps } from './components/DepartmentTree';
import { DepartmentModal } from './components/DepartmentModal';

type DepartmentFormValues = Pick<
  API.SysDept,
  'name' | 'code' | 'orgId' | 'status' | 'sortOrder' | 'leaderUserId'
>;

export default function DepartmentManagementPage() {
  const [selectedDept, setSelectedDept] = useState<API.SysDept | null>(null);
  const [modalVisible, setModalVisible] = useState(false);
  const [modalMode, setModalMode] = useState<'create' | 'edit'>('create');
  const [parentId, setParentId] = useState<API.Int64>();
  const [postAssignVisible, setPostAssignVisible] = useState(false);
  const [roleAssignVisible, setRoleAssignVisible] = useState(false);

  // 获取部门树
  const { data: deptTreeData, isLoading: deptLoading, refetch: refetchDept } = useQuery({
    queryKey: ['deptTree'],
    queryFn: () => getDeptTree(),
  });

  // 获取组织树
  const { data: orgTreeData } = useQuery({
    queryKey: ['orgTree'],
    queryFn: () => getOrgTree(),
  });

  const deptTree = deptTreeData?.data ?? [];
  const orgTree = orgTreeData?.data ?? [];

  // 创建部门
  const createDeptMutation = useMutation({
    mutationFn: (body: Parameters<typeof createDept>[0]) => createDept(body),
    onSuccess: () => {
      message.success('部门创建成功');
      refetchDept();
      setModalVisible(false);
      setSelectedDept(null);
    },
    onError: error => {
      message.error(error.message || '部门创建失败');
    },
  });

  // 更新部门
  const updateDeptMutation = useMutation({
    mutationFn: (body: Parameters<typeof updateDept>[0]) => updateDept(body),
    onSuccess: () => {
      message.success('部门更新成功');
      refetchDept();
      setModalVisible(false);
      setSelectedDept(null);
    },
    onError: error => {
      message.error(error.message || '部门更新失败');
    },
  });

  // 删除部门
  const deleteDeptMutation = useMutation({
    mutationFn: (params: Parameters<typeof deleteDept>[0]) => deleteDept(params),
    onSuccess: () => {
      message.success('部门删除成功');
      refetchDept();
      setSelectedDept(null);
    },
    onError: error => {
      message.error(error.message || '部门删除失败');
    },
  });

  // 选择部门
  const handleSelect: DepartmentTreeProps['onSelect'] = (keys, info) => {
    if (keys.length > 0) {
      const nodeData = info.node.nodeData;
      if (nodeData) {
        setSelectedDept((prev) => (prev?.id === nodeData.id ? prev : nodeData));
      } else {
        console.warn('无法获取选中节点的原始数据');
      }
    } else {
      if (selectedDept !== null) setSelectedDept(null);
    }
  };

  // 新增部门
  const handleAdd = (parentDeptId?: API.Int64) => {
    setModalMode('create');
    setParentId(parentDeptId);
    setSelectedDept(null);
    setModalVisible(true);
  };

  // 编辑部门
  const handleEdit = (dept: API.SysDept) => {
    setModalMode('edit');
    setSelectedDept(dept);
    setParentId(undefined);
    setModalVisible(true);
  };

  // 删除部门
  const handleDelete = (dept: API.SysDept) => {
    Modal.confirm({
      title: '确认删除',
      icon: <ExclamationCircleOutlined />,
      content: (
        <div>
          <p>确定要删除部门 <strong>{dept.name}</strong> 吗？</p>
          <p style={{ color: '#ff4d4f' }}>
            注意：删除后不可恢复，且会影响该部门下的所有子部门和用户！
          </p>
        </div>
      ),
      okText: '确认删除',
      okType: 'danger',
      cancelText: '取消',
      onOk: async () => {
        if (dept.id) {
          await deleteDeptMutation.mutateAsync({ id: dept.id });
        }
      },
    });
  };

  // 状态变更（预留）
  const handleStatusChange = () => {
    message.info('部门状态变更功能暂未开放');
  };

  // 分配岗位
  const handleAssignPosts = () => {
    if (!selectedDept) {
      message.warning('请先选择部门');
      return;
    }
    setPostAssignVisible(true);
  };

  // 分配角色
  const handleAssignRoles = () => {
    if (!selectedDept) {
      message.warning('请先选择部门');
      return;
    }
    setRoleAssignVisible(true);
  };

  // 关闭模态框
  const handleModalCancel = () => {
    setModalVisible(false);
    setSelectedDept(null);
    setParentId(undefined);
  };

  // 处理模态框提交
  const handleModalOk = async (values: DepartmentFormValues) => {
    if (modalMode === 'create') {
      await createDeptMutation.mutateAsync({
        ...values,
        parentId: parentId || '0',
      });
    } else if (selectedDept?.id) {
      await updateDeptMutation.mutateAsync({
        ...values,
        id: selectedDept.id,
      });
    }
  };

  // 根据组织ID获取组织名称
  const getOrgName = (orgId?: API.Int64) => {
    if (!orgId || !orgTree.length) return '-';

    const findOrgName = (tree: API.SysOrg[]): string => {
      for (const org of tree) {
        if (org.id === orgId) return org.name || '-';
        if (org.children) {
          const found = findOrgName(org.children);
          if (found !== '-') return found;
        }
      }
      return '-';
    };

    return findOrgName(orgTree);
  };

  return (
    <>
      <Row gutter={[16, 16]}>
        {/* 部门树 */}
        <Col xs={24} lg={16}>
          <Card
            title="部门架构"
            extra={
              <Button type="primary" icon={<PlusOutlined />} onClick={() => handleAdd()}>
                新增部门
              </Button>
            }
            style={{ height: '700px', overflow: 'auto' }}
          >
            <Spin spinning={deptLoading}>
              <DepartmentTree
                data={deptTree}
                loading={deptLoading}
                selectedKeys={selectedDept?.id ? [selectedDept.id] : []}
                onSelect={handleSelect}
                onAdd={handleAdd}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onStatusChange={handleStatusChange}
              />
            </Spin>
          </Card>
        </Col>

        {/* 部门详情 */}
        <Col xs={24} lg={8}>
          <Card title="部门详情" style={{ height: '700px' }}>
            {selectedDept ? (
              <div style={{ padding: '16px 0' }}>
                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>部门名称：</label>
                  <span>{selectedDept.name}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>部门编码：</label>
                  <span>{selectedDept.code}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>所属组织：</label>
                  <span>{getOrgName(selectedDept.orgId)}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>状态：</label>
                  <Tag color={selectedDept.status === 0 ? 'green' : 'red'}>
                    {selectedDept.status === 0 ? '启用' : '禁用'}
                  </Tag>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>负责人：</label>
                  <span>
                    {selectedDept.leaderUserId ? (
                      <Tag icon={<UserOutlined />} color="blue">用户ID: {selectedDept.leaderUserId}</Tag>
                    ) : (
                      '暂未设置'
                    )}
                  </span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>排序：</label>
                  <span>{selectedDept.sortOrder}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>创建时间：</label>
                  <span>{selectedDept.createTime}</span>
                </div>

                <div style={{ marginBottom: 16 }}>
                  <label style={{ fontWeight: 500, display: 'block', marginBottom: 4 }}>更新时间：</label>
                  <span>{selectedDept.updateTime}</span>
                </div>

                <div style={{ marginTop: 24, display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                  <Button type="primary" onClick={() => handleEdit(selectedDept)}>编辑</Button>
                  <Button danger onClick={() => handleDelete(selectedDept)}>删除</Button>
                  <Button
                    icon={<TeamOutlined />}
                    onClick={handleAssignPosts}
                    style={{ marginTop: 8 }}
                  >
                    分配岗位
                  </Button>
                  <Button
                    icon={<SafetyOutlined />}
                    onClick={handleAssignRoles}
                    style={{ marginTop: 8 }}
                  >
                    分配角色
                  </Button>
                </div>
              </div>
            ) : (
              <div style={{ textAlign: 'center', padding: '60px 0', color: '#999' }}>
                请选择部门查看详情
              </div>
            )}
          </Card>
        </Col>
      </Row>

      {/* 新增/编辑模态框 */}
      <DepartmentModal
        visible={modalVisible}
        mode={modalMode}
        initialValues={selectedDept || undefined}
        orgTree={orgTree}
        onOk={handleModalOk}
        onCancel={handleModalCancel}
      />

      {/* 分配岗位模态框 */}
      <Modal
        title={`分配岗位 - ${selectedDept?.name}`}
        open={postAssignVisible}
        onCancel={() => setPostAssignVisible(false)}
        footer={null}
        width={600}
      >
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
          <TeamOutlined style={{ fontSize: 48, marginBottom: 16 }} />
          <p>部门岗位分配功能开发中...</p>
          <p>该功能将允许为部门分配相关岗位</p>
        </div>
      </Modal>

      {/* 分配角色模态框 */}
      <Modal
        title={`分配角色 - ${selectedDept?.name}`}
        open={roleAssignVisible}
        onCancel={() => setRoleAssignVisible(false)}
        footer={null}
        width={600}
      >
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
          <SafetyOutlined style={{ fontSize: 48, marginBottom: 16 }} />
          <p>部门角色分配功能开发中...</p>
          <p>该功能将允许为部门分配相关角色</p>
        </div>
      </Modal>
    </>
  );
}
