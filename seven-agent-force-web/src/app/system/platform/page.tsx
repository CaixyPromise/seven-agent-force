'use client';

import React, { useRef, useState } from 'react';
import { Button, Dropdown, Input, Modal, Space, Tag, Tooltip, message } from 'antd';
import type { MenuProps } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  CopyOutlined,
  EditOutlined,
  InfoCircleOutlined,
  MoreOutlined,
  PlusOutlined,
  PoweroffOutlined,
  SafetyOutlined,
} from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import {
  createPlatformAdmin,
  getPlatformAdmin,
  listPlatformAdmins,
  updatePlatformAdmin,
  updatePlatformAdminDefaultRoles,
  updatePlatformAdminLoginMethods,
  updatePlatformAdminSourceRules,
  updatePlatformAdminStatus,
  type PlatformAdminCreateRequest,
  type PlatformAdminRecord,
  type PlatformAdminStatusRequest,
  type PlatformAdminUpdateRequest,
} from '@/api/platformController';
import { usePermissionFlags } from '@/hooks/auth';
import { PLATFORM_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';
import PlatformFormDrawer, {
  type PlatformFormSubmitValues,
} from './components/PlatformFormDrawer';

type DrawerMode = 'create' | 'edit';

const PLATFORM_TYPE_META: Record<string, { label: string; color: string; description: string }> = {
  ADMIN: {
    label: '管理后台',
    color: 'geekblue',
    description: '面向系统管理员和运营人员，通常拥有较高权限和完整后台菜单。',
  },
  CONSOLE: {
    label: '控制台',
    color: 'blue',
    description: '面向内部或租户管理员，承载平台配置、账号安全和日常运维入口。',
  },
  PORTAL: {
    label: '门户站点',
    color: 'cyan',
    description: '面向普通用户或业务用户，通常只开放轻量功能和自助服务。',
  },
  BUSINESS: {
    label: '业务平台',
    color: 'green',
    description: '面向具体业务系统或多平台后台，用于配置独立登录方式、来源规则和默认权限。',
  },
  API: {
    label: '开放 API',
    color: 'purple',
    description: '面向程序化调用或第三方系统接入，通常按 Client 和来源规则识别。',
  },
};

function renderStatusTag(status: number) {
  return (
    <Tag color={status === 0 ? 'green' : 'default'}>{status === 0 ? '启用' : '停用'}</Tag>
  );
}

function renderCountTag(count: number, color = 'blue') {
  return <Tag color={count > 0 ? color : 'default'}>{count}</Tag>;
}

function renderPlatformTypeTag(platformType?: string) {
  const normalized = (platformType || 'CONSOLE').toUpperCase();
  const meta = PLATFORM_TYPE_META[normalized] || {
    label: normalized || '-',
    color: 'default',
    description: '自定义平台类型，用于后续扩展多平台后台管理场景。',
  };
  return (
    <Tooltip title={meta.description}>
      <Tag color={meta.color}>
        {meta.label}
        <InfoCircleOutlined style={{ marginLeft: 4 }} />
      </Tag>
    </Tooltip>
  );
}

function isChallengeCancelled(error: unknown) {
  return isChallengeRetryError(error, 'CHALLENGE_CANCELLED');
}

function showError(error: unknown, fallback: string) {
  if (isChallengeCancelled(error)) {
    return;
  }
  message.error((error as { message?: string })?.message || fallback);
}

function askReason(title: string, placeholder: string): Promise<string | null> {
  return new Promise((resolve) => {
    let reason = '';
    Modal.confirm({
      title,
      icon: <SafetyOutlined />,
      content: (
        <Input.TextArea
          rows={3}
          placeholder={placeholder}
          onChange={(event) => {
            reason = event.target.value;
          }}
        />
      ),
      okText: '确认',
      cancelText: '取消',
      onOk: () => {
        const normalized = reason.trim();
        if (!normalized) {
          message.warning('请输入操作原因');
          return Promise.reject(new Error('reason required'));
        }
        resolve(normalized);
        return Promise.resolve();
      },
      onCancel: () => resolve(null),
    });
  });
}

function buildCopyPlatformCode(platformCode: string) {
  const suffix = Date.now().toString(36).slice(-6);
  const base = `${platformCode}-copy`.toLowerCase().replace(/[^a-z0-9-]/g, '-');
  return `${base.slice(0, Math.max(1, 63 - suffix.length))}${suffix}`;
}

function buildBaseCreateRequest(values: PlatformFormSubmitValues): PlatformAdminCreateRequest {
  return {
    platformCode: values.platformCode || '',
    platformName: values.platformName,
    platformType: values.platformType,
    description: values.description,
    defaultRedirectUrl: values.defaultRedirectUrl,
    allowAutoRegister: values.allowAutoRegister,
    isDefault: values.isDefault,
    defaultDeptId: values.defaultDeptId,
    brandJson: values.brandJson,
    settingsJson: values.settingsJson,
    reason: values.reason,
    stepUpProof: values.stepUpProof,
  };
}

function buildBaseUpdateRequest(values: PlatformFormSubmitValues): PlatformAdminUpdateRequest {
  return {
    platformName: values.platformName,
    platformType: values.platformType,
    description: values.description,
    defaultRedirectUrl: values.defaultRedirectUrl,
    allowAutoRegister: values.allowAutoRegister,
    isDefault: values.isDefault,
    defaultDeptId: values.defaultDeptId,
    brandJson: values.brandJson,
    settingsJson: values.settingsJson,
    reason: values.reason,
    stepUpProof: values.stepUpProof,
  };
}

export default function PlatformManagementPage() {
  const actionRef = useRef<ActionType | undefined>(undefined);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<DrawerMode>('create');
  const [selectedPlatform, setSelectedPlatform] = useState<PlatformAdminRecord | null>(null);
  const [statusUpdatingPlatformCode, setStatusUpdatingPlatformCode] = useState<string | null>(null);
  const [createDisabledAfterSave, setCreateDisabledAfterSave] = useState(false);

  const permissions = usePermissionFlags({
    canCreate: PLATFORM_PERMISSIONS.ADD,
    canQuery: PLATFORM_PERMISSIONS.QUERY,
    canEdit: PLATFORM_PERMISSIONS.EDIT,
    canChangeStatus: PLATFORM_PERMISSIONS.STATUS,
    canEditLoginMethods: PLATFORM_PERMISSIONS.LOGIN_METHODS,
    canEditSourceRules: PLATFORM_PERMISSIONS.SOURCE_RULES,
    canEditDefaultRoles: PLATFORM_PERMISSIONS.DEFAULT_ROLES,
  });

  const createMutation = useMutation({
    mutationFn: createPlatformAdmin,
    onError: (error) => showError(error, '创建平台失败'),
  });

  const updateMutation = useMutation({
    mutationFn: ({
      platformCode,
      body,
    }: {
      platformCode: string;
      body: PlatformAdminUpdateRequest;
    }) => updatePlatformAdmin(platformCode, body),
    onError: (error) => showError(error, '更新平台失败'),
  });

  const statusMutation = useMutation({
    mutationFn: ({
      platformCode,
      body,
    }: {
      platformCode: string;
      body: PlatformAdminStatusRequest;
    }) => updatePlatformAdminStatus(platformCode, body),
    onSuccess: () => {
      message.success('平台状态已更新');
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '更新平台状态失败'),
    onSettled: () => setStatusUpdatingPlatformCode(null),
  });

  const saveConfiguration = async (platformCode: string, values: PlatformFormSubmitValues) => {
    const common = {
      reason: values.reason,
      stepUpProof: values.stepUpProof,
    };
    if (permissions.canEditLoginMethods) {
      await updatePlatformAdminLoginMethods(platformCode, {
        methods: values.loginMethods,
        ...common,
      });
    }
    if (permissions.canEditSourceRules) {
      await updatePlatformAdminSourceRules(platformCode, {
        rules: values.sourceRules,
        ...common,
      });
    }
    if (permissions.canEditDefaultRoles && values.allowAutoRegister) {
      await updatePlatformAdminDefaultRoles(platformCode, {
        roleIds: values.defaultRoleIds,
        roles: values.defaultRoles,
        ...common,
      });
    }
  };

  const loadPlatformDetail = async (record: PlatformAdminRecord) => {
    const detail = await getPlatformAdmin(record.platformCode);
    setSelectedPlatform(detail);
    return detail;
  };

  const openCreate = () => {
    setFormMode('create');
    setSelectedPlatform(null);
    setCreateDisabledAfterSave(false);
    setFormOpen(true);
  };

  const openEdit = async (record: PlatformAdminRecord) => {
    try {
      await loadPlatformDetail(record);
      setFormMode('edit');
      setCreateDisabledAfterSave(false);
      setFormOpen(true);
    } catch (error) {
      showError(error, '获取平台详情失败');
    }
  };

  const openCopy = async (record: PlatformAdminRecord) => {
    try {
      const detail = await getPlatformAdmin(record.platformCode);
      setSelectedPlatform({
        ...detail,
        platformCode: buildCopyPlatformCode(detail.platformCode || record.platformCode),
        platformName: `${detail.platformName || record.platformName} 副本`,
        isDefault: false,
        status: 1,
      });
      setFormMode('create');
      setCreateDisabledAfterSave(true);
      setFormOpen(true);
    } catch (error) {
      showError(error, '复制平台配置失败');
    }
  };

  const handleFormSubmit = async (values: PlatformFormSubmitValues) => {
    if (formMode === 'create') {
      const createBody = buildBaseCreateRequest(values);
      if (createDisabledAfterSave) {
        createBody.status = 1;
      }
      const created = await createMutation.mutateAsync(createBody);
      const platformCode = created.platformCode || values.platformCode || '';
      await saveConfiguration(platformCode, values);
      message.success(createDisabledAfterSave ? '平台已复制并保持停用' : '平台已创建');
      setFormOpen(false);
      setSelectedPlatform(null);
      setCreateDisabledAfterSave(false);
      actionRef.current?.reload();
      return;
    }
    if (!selectedPlatform) {
      message.error('未选择平台');
      return;
    }
    await updateMutation.mutateAsync({
      platformCode: selectedPlatform.platformCode,
      body: buildBaseUpdateRequest(values),
    });
    await saveConfiguration(selectedPlatform.platformCode, values);
    message.success('平台已更新');
    setFormOpen(false);
    setSelectedPlatform(null);
    actionRef.current?.reload();
  };

  const handleStatusChange = async (record: PlatformAdminRecord) => {
    const nextStatus = record.status === 0 ? 1 : 0;
    const reason = await askReason(
      nextStatus === 0 ? '启用平台' : '停用平台',
      nextStatus === 0 ? '请输入启用原因' : '请输入停用原因',
    );
    if (!reason) {
      return;
    }
    setStatusUpdatingPlatformCode(record.platformCode);
    statusMutation.mutate({
      platformCode: record.platformCode,
      body: {
        status: nextStatus,
        reason,
        stepUpProof: '',
      },
    });
  };

  const columns: ProColumns<PlatformAdminRecord>[] = [
    {
      title: '平台编码',
      dataIndex: 'platformCode',
      copyable: true,
      width: 180,
      fixed: 'left',
      render: (_, record) => <Tag color="blue">{record.platformCode}</Tag>,
    },
    {
      title: '平台名称',
      dataIndex: 'platformName',
      width: 180,
      fixed: 'left',
      search: false,
    },
    {
      title: (
        <Tooltip title="平台类型用于区分不同后台、门户和 API 接入方的登录策略、默认权限和入口展示方式。">
          <Space size={4}>
            平台类型
            <InfoCircleOutlined />
          </Space>
        </Tooltip>
      ),
      dataIndex: 'platformType',
      width: 140,
      valueType: 'select',
      valueEnum: Object.fromEntries(
        Object.entries(PLATFORM_TYPE_META).map(([value, meta]) => [value, { text: meta.label }]),
      ),
      render: (_, record) => renderPlatformTypeTag(record.platformType),
    },
    {
      title: '默认跳转地址',
      dataIndex: 'defaultRedirectUrl',
      width: 220,
      search: false,
      ellipsis: true,
      renderText: (_, record) => record.defaultRedirectUrl || '-',
    },
    {
      title: '登录方式',
      dataIndex: 'loginMethods',
      width: 100,
      search: false,
      render: (_, record) => renderCountTag(record.loginMethods?.length || 0),
    },
    {
      title: '来源规则',
      dataIndex: 'sourceRules',
      width: 100,
      search: false,
      render: (_, record) => renderCountTag(record.sourceRules?.length || 0, 'cyan'),
    },
    {
      title: '默认角色',
      dataIndex: 'defaultRoles',
      width: 100,
      search: false,
      render: (_, record) =>
        renderCountTag(
          (record.defaultRoleIds?.length || 0) + (record.defaultRoles?.length || 0),
          'purple',
        ),
    },
    {
      title: '自动注册',
      dataIndex: 'allowAutoRegister',
      width: 100,
      search: false,
      render: (_, record) => (
        <Tag color={record.allowAutoRegister ? 'green' : 'default'}>
          {record.allowAutoRegister ? '允许' : '关闭'}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      fixed: 'right',
      valueType: 'select',
      valueEnum: {
        0: { text: '启用' },
        1: { text: '停用' },
      },
      render: (_, record) => renderStatusTag(record.status),
    },
    {
      title: '默认平台',
      dataIndex: 'isDefault',
      width: 100,
      search: false,
      render: (_, record) => (
        <Tag color={record.isDefault ? 'blue' : 'default'}>{record.isDefault ? '是' : '否'}</Tag>
      ),
    },
    {
      title: '更新时间',
      dataIndex: 'updateTime',
      valueType: 'dateTime',
      width: 180,
      search: false,
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 190,
      search: false,
      render: (_, record) => {
        const items: MenuProps['items'] = [
          permissions.canEdit
            ? {
                key: 'edit',
                icon: <EditOutlined />,
                label: '编辑',
              }
            : null,
          permissions.canCreate
            ? {
                key: 'copy',
                icon: <CopyOutlined />,
                label: '复制为新平台',
              }
            : null,
          permissions.canChangeStatus
            ? {
                key: 'status',
                icon: <PoweroffOutlined />,
                label:
                  record.isDefault && record.status === 0
                    ? '默认平台不可停用'
                    : record.status === 0
                      ? '停用'
                      : '启用',
                danger: record.status === 0,
                disabled: record.isDefault && record.status === 0,
              }
            : null,
        ].filter(Boolean) as MenuProps['items'];

        if (!items?.length) {
          return null;
        }

        return (
          <Space size="small">
            {permissions.canEdit ? (
              <Button type="link" size="small" onClick={() => void openEdit(record)}>
                编辑
              </Button>
            ) : null}
            <Dropdown
              menu={{
                items,
                onClick: ({ key }) => {
                  if (key === 'edit') void openEdit(record);
                  if (key === 'copy') void openCopy(record);
                  if (key === 'status' && !(record.isDefault && record.status === 0)) {
                    void handleStatusChange(record);
                  }
                },
              }}
            >
              <Button
                size="small"
                icon={<MoreOutlined />}
                loading={statusUpdatingPlatformCode === record.platformCode}
              />
            </Dropdown>
          </Space>
        );
      },
    },
  ];

  return (
    <div className="min-h-full bg-[#f6f8fb] px-6 py-6">
      <ProTable<PlatformAdminRecord>
        rowKey="platformCode"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 100 }}
        scroll={{ x: 1600 }}
        sticky
        cardBordered
        headerTitle="平台管理"
        toolBarRender={() => [
          permissions.canCreate ? (
            <Tooltip key="create" title="新增平台">
              <Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>
                新增平台
              </Button>
            </Tooltip>
          ) : null,
        ]}
        request={async (params) => {
          const page = await listPlatformAdmins({
            keyword: typeof params.platformCode === 'string' ? params.platformCode : undefined,
            platformType:
              typeof params.platformType === 'string' ? params.platformType : undefined,
            status:
              typeof params.status === 'string'
                ? Number(params.status)
                : typeof params.status === 'number'
                  ? params.status
                  : undefined,
            current: params.current,
            pageSize: params.pageSize,
          });
          return {
            data: page.records,
            success: true,
            total: page.total,
          };
        }}
      />

        <PlatformFormDrawer
          open={formOpen}
          mode={formMode}
          initialValues={selectedPlatform}
          confirmLoading={createMutation.isPending || updateMutation.isPending}
          canEditLoginMethods={permissions.canEditLoginMethods}
          canEditSourceRules={permissions.canEditSourceRules}
          canEditDefaultRoles={permissions.canEditDefaultRoles}
          onClose={() => {
            setFormOpen(false);
            setCreateDisabledAfterSave(false);
          }}
          onSubmit={handleFormSubmit}
        />
    </div>
  );
}
