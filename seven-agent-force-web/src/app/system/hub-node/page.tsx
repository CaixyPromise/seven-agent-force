'use client';

import { useRef, useState } from 'react';
import { Button, Dropdown, Grid, Input, Modal, Result, Select, Space, Tag, Tooltip, Typography, message } from 'antd';
import type { MenuProps } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  CopyOutlined,
  EditOutlined,
  MoreOutlined,
  PlusOutlined,
  PoweroffOutlined,
  RadarChartOutlined,
  SettingOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  copyHubNode,
  canManageHubNode,
  createHubNode,
  getHubNode,
  listHubNodes,
  setHubNodeStatus,
  testHubNodeConnection,
  updateHubNode,
  type HubNodeRecord,
  type HubNodeStatusValue,
} from '@/api/hubNodeController';
import { usePermissionFlags } from '@/hooks/auth';
import { HUB_NODE_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { connectionStatusTag, formatTime, nodeStatusTag } from './constants';
import HubNodeDetailDrawer, { type HubNodeDetailPermissions } from './components/HubNodeDetailDrawer';
import HubNodeFormDrawer, { type HubNodeFormMode, type HubNodeFormValues } from './components/HubNodeFormDrawer';
import styles from './hubNode.module.css';

function showError(error: unknown, fallback: string) {
  message.error((error as Error)?.message || fallback);
}

export default function HubNodePage() {
  const actionRef = useRef<ActionType>(undefined);
  const queryClient = useQueryClient();
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.md;
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<HubNodeStatusValue | undefined>();
  const [savingNode, setSavingNode] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<HubNodeFormMode>('create');
  const [selectedNode, setSelectedNode] = useState<HubNodeRecord | null>(null);
  const [detailNode, setDetailNode] = useState<HubNodeRecord | null>(null);
  const [loadingNodeCode, setLoadingNodeCode] = useState<string | null>(null);

  const permissions = usePermissionFlags({
    canList: HUB_NODE_PERMISSIONS.LIST,
    canCreate: HUB_NODE_PERMISSIONS.ADD,
    canQuery: HUB_NODE_PERMISSIONS.QUERY,
    canEdit: HUB_NODE_PERMISSIONS.EDIT,
    canChangeStatus: HUB_NODE_PERMISSIONS.STATUS,
    canTest: HUB_NODE_PERMISSIONS.TEST,
    canListUsers: HUB_NODE_PERMISSIONS.USER_LIST,
    canQueryUser: HUB_NODE_PERMISSIONS.USER_QUERY,
    canChangeUserStatus: HUB_NODE_PERMISSIONS.USER_STATUS,
    canListSessions: HUB_NODE_PERMISSIONS.SESSION_LIST,
    canRevokeSessions: HUB_NODE_PERMISSIONS.SESSION_REVOKE,
    canQueryPolicy: HUB_NODE_PERMISSIONS.POLICY_QUERY,
    canApplyPolicy: HUB_NODE_PERMISSIONS.POLICY_APPLY,
    canQueryFederation: HUB_NODE_PERMISSIONS.FEDERATION_QUERY,
    canApplyFederation: HUB_NODE_PERMISSIONS.FEDERATION_APPLY,
  });

  const refreshRegistry = async () => {
    await queryClient.invalidateQueries({ queryKey: ['hub-nodes'] });
    actionRef.current?.reload();
  };

  const statusMutation = useMutation({
    mutationFn: ({ nodeCode, status }: { nodeCode: string; status: HubNodeStatusValue }) => setHubNodeStatus(nodeCode, status),
    onSuccess: async () => {
      message.success('Node 状态已更新');
      await refreshRegistry();
    },
    onError: (error) => showError(error, 'Node 状态更新失败'),
    onSettled: () => setLoadingNodeCode(null),
  });
  const testMutation = useMutation({
    mutationFn: testHubNodeConnection,
    onSuccess: async (health) => {
      message.success(`连接测试完成：${health.health}${health.version ? ` · ${health.version}` : ''}`);
      await refreshRegistry();
    },
    onError: (error) => showError(error, '连接测试失败'),
    onSettled: () => setLoadingNodeCode(null),
  });

  const loadNode = async (record: HubNodeRecord) => {
    setLoadingNodeCode(record.nodeCode);
    try {
      return await getHubNode(record.nodeCode);
    } catch (error) {
      showError(error, 'Node 详情加载失败');
      return null;
    } finally {
      setLoadingNodeCode(null);
    }
  };
  const openEdit = async (record: HubNodeRecord) => {
    if (!permissions.canQuery || !canManageHubNode(record)) return;
    const detail = await loadNode(record);
    if (!detail) return;
    if (!canManageHubNode(detail)) {
      message.error('Node 详情状态无法安全识别，已阻止编辑');
      return;
    }
    setSelectedNode(detail);
    setFormMode('edit');
    setFormOpen(true);
  };
  const openCopy = async (record: HubNodeRecord) => {
    if (!permissions.canQuery || !canManageHubNode(record)) return;
    const detail = await loadNode(record);
    if (!detail) return;
    if (!canManageHubNode(detail)) {
      message.error('Node 详情状态无法安全识别，已阻止复制');
      return;
    }
    setSelectedNode(detail);
    setFormMode('copy');
    setFormOpen(true);
  };
  const openDetail = (record: HubNodeRecord) => {
    if (permissions.canQuery && canManageHubNode(record)) {
      setDetailNode(record);
    }
  };
  const changeStatus = (record: HubNodeRecord) => {
    if (!canManageHubNode(record)) return;
    const nextStatus: HubNodeStatusValue = record.status === 0 ? 1 : 0;
    Modal.confirm({
      title: `${nextStatus === 0 ? '启用' : '停用'} ${record.nodeName}`,
      content: nextStatus === 1 ? '停用后 Hub 将拒绝发起新的管理操作，已有 Node 会话不会被撤销。' : '启用后 Hub 可以重新向该 Node 发起管理操作。',
      okText: nextStatus === 0 ? '启用' : '停用',
      okButtonProps: { danger: nextStatus === 1 },
      onOk: async () => {
        setLoadingNodeCode(record.nodeCode);
        await statusMutation.mutateAsync({ nodeCode: record.nodeCode, status: nextStatus });
      },
    });
  };

  const renderNodeActions = (row: HubNodeRecord, compact = false) => {
    if (!canManageHubNode(row)) return null;
    const menuItems: NonNullable<MenuProps['items']> = [];
    if (permissions.canEdit && permissions.canQuery) {
      menuItems.push({ key: 'edit', icon: <EditOutlined />, label: '编辑' });
    }
    if (permissions.canCreate && permissions.canQuery) {
      menuItems.push({ key: 'copy', icon: <CopyOutlined />, label: '复制为停用 Node' });
    }
    if (permissions.canTest) {
      menuItems.push({ key: 'test', icon: <RadarChartOutlined />, label: '测试连接' });
    }
    if (permissions.canChangeStatus) {
      menuItems.push({
        key: 'status',
        icon: <PoweroffOutlined />,
        label: row.status === 0 ? '停用' : '启用',
        danger: row.status === 0,
      });
    }
    return (
      <Space size={4}>
        {permissions.canQuery ? (
          <Tooltip title="管理 Node">
            <Button
              aria-label={`管理 ${row.nodeName}`}
              type="link"
              size="small"
              icon={<SettingOutlined />}
              onClick={() => openDetail(row)}
            >
              {compact ? null : '管理'}
            </Button>
          </Tooltip>
        ) : null}
        {menuItems.length ? (
          <Dropdown menu={{ items: menuItems, onClick: ({ key }) => {
            if (key === 'edit') void openEdit(row);
            if (key === 'copy') void openCopy(row);
            if (key === 'test') { setLoadingNodeCode(row.nodeCode); testMutation.mutate(row.nodeCode); }
            if (key === 'status') changeStatus(row);
          } }}>
            <Button aria-label={`${row.nodeName} 更多操作`} size="small" icon={<MoreOutlined />} loading={loadingNodeCode === row.nodeCode} />
          </Dropdown>
        ) : null}
      </Space>
    );
  };

  const submitNodeForm = async (values: HubNodeFormValues) => {
    setSavingNode(true);
    try {
      if (formMode === 'create') {
        await createHubNode(values);
        message.success('Node 已创建');
      } else if (formMode === 'edit' && selectedNode) {
        await updateHubNode(selectedNode.nodeCode, values);
        message.success('Node 已更新');
      } else if (formMode === 'copy' && selectedNode) {
        const copy = await copyHubNode(selectedNode.nodeCode, {
          nodeCode: values.nodeCode,
          nodeName: values.nodeName,
          managementBearer: values.managementBearer,
        });
        message.success(`${copy.nodeName || copy.nodeCode} 已复制并保持停用`);
      }
      setFormOpen(false);
      setSelectedNode(null);
      await refreshRegistry();
    } catch (error) {
      showError(error, formMode === 'copy' ? 'Node 复制失败' : formMode === 'edit' ? 'Node 更新失败' : 'Node 创建失败');
    } finally {
      setSavingNode(false);
    }
  };

  if (!permissions.canList) {
    return (
      <Result
        status="403"
        title="访问被拒绝"
        subTitle="您没有权限访问当前页面，请联系管理员为您分配角色或权限。"
        extra={
          <Button type="primary" onClick={() => window.history.back()}>
            返回上一页
          </Button>
        }
      />
    );
  }

  const columns: ProColumns<HubNodeRecord>[] = [
    {
      title: 'Node', dataIndex: 'nodeCode', width: 210,
      render: (_, row) => (
        <Space orientation="vertical" size={0}>
          {permissions.canQuery && canManageHubNode(row) ? (
            <Button type="link" className={styles.nodeLink} onClick={() => openDetail(row)}>{row.nodeName}</Button>
          ) : (
            <Typography.Text strong>{row.nodeName}</Typography.Text>
          )}
          <Typography.Text type="secondary" copyable>{row.nodeCode}</Typography.Text>
        </Space>
      ),
    },
    { title: '状态', dataIndex: 'status', width: 105, valueType: 'select', valueEnum: { 0: { text: '启用' }, 1: { text: '停用' } }, render: (_, row) => nodeStatusTag(row.status) },
    { title: '发现方式', dataIndex: 'discoveryType', width: 110, search: false, render: (_, row) => <Tag>{row.discoveryType}</Tag> },
    { title: '连接', dataIndex: 'connectionStatus', width: 110, search: false, render: (_, row) => connectionStatusTag(row.connectionStatus) },
    { title: '管理目标', key: 'target', width: 250, search: false, ellipsis: true, responsive: ['lg'], render: (_, row) => row.discoveryType === 'CONSUL' ? row.serviceName || '-' : row.managementBaseUrl || '-' },
    { title: '能力', dataIndex: 'capabilities', width: 210, search: false, responsive: ['xxl'], render: (_, row) => <Space size={[2, 4]} wrap>{row.capabilities.slice(0, 3).map((item) => <Tag key={item}>{item}</Tag>)}{row.capabilities.length > 3 ? <Tag>+{row.capabilities.length - 3}</Tag> : null}</Space> },
    { title: '最近健康', dataIndex: 'lastHealthyAt', width: 170, search: false, responsive: ['xxl'], render: (_, row) => formatTime(row.lastHealthyAt) },
    {
      title: '操作', key: 'actions', fixed: 'right', width: 180, search: false,
      render: (_, row) => renderNodeActions(row),
    },
  ];
  const mobileColumns: ProColumns<HubNodeRecord>[] = [
    {
      title: 'Node',
      key: 'mobileNode',
      width: 170,
      render: (_, row) => (
        <Space orientation="vertical" size={5}>
          {permissions.canQuery && canManageHubNode(row) ? (
            <Button type="link" className={styles.nodeLink} onClick={() => openDetail(row)}>{row.nodeName}</Button>
          ) : (
            <Typography.Text strong>{row.nodeName}</Typography.Text>
          )}
          <Typography.Text type="secondary">{row.nodeCode}</Typography.Text>
          <Space size={4}>{nodeStatusTag(row.status)}{connectionStatusTag(row.connectionStatus)}</Space>
        </Space>
      ),
    },
    { title: '操作', key: 'mobileActions', width: 100, render: (_, row) => renderNodeActions(row, true) },
  ];

  const detailPermissions: HubNodeDetailPermissions = permissions;
  return (
    <div className={styles.page} data-testid="hub-node-page">
      <div className={styles.registryControls}>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索 Node 编码或名称"
          value={keyword}
          onChange={(event) => setKeyword(event.target.value)}
          onPressEnter={() => actionRef.current?.reloadAndRest?.()}
        />
        <Select
          allowClear
          placeholder="全部状态"
          value={statusFilter}
          options={[{ label: '启用', value: 0 }, { label: '停用', value: 1 }]}
          onChange={setStatusFilter}
        />
        <Button type="primary" icon={<SearchOutlined />} onClick={() => actionRef.current?.reloadAndRest?.()}>查询</Button>
      </div>
      <ProTable<HubNodeRecord>
        rowKey="nodeCode"
        actionRef={actionRef}
        columns={isMobile ? mobileColumns : columns}
        cardBordered
        sticky
        scroll={{ x: isMobile ? 270 : 980 }}
        search={false}
        headerTitle="Node 注册表"
        toolBarRender={() => permissions.canCreate ? [
          <Tooltip key="create" title="注册一个可由 Hub 管理的 Node">
            <Button type="primary" icon={<PlusOutlined />} onClick={() => { setSelectedNode(null); setFormMode('create'); setFormOpen(true); }}>新增 Node</Button>
          </Tooltip>,
        ] : []}
        request={async (params) => {
          const result = await listHubNodes({
            current: params.current,
            size: params.pageSize,
            keyword: keyword.trim() || undefined,
            status: statusFilter,
          });
          return { data: result.records, total: result.total, success: true };
        }}
        pagination={{ defaultPageSize: 20, showSizeChanger: true }}
      />
      <HubNodeFormDrawer
        open={formOpen}
        mode={formMode}
        initialValues={selectedNode}
        loading={savingNode}
        onClose={() => { setFormOpen(false); setSelectedNode(null); }}
        onSubmit={submitNodeForm}
      />
      <HubNodeDetailDrawer open={Boolean(detailNode)} node={detailNode} permissions={detailPermissions} onClose={() => setDetailNode(null)} />
    </div>
  );
}
