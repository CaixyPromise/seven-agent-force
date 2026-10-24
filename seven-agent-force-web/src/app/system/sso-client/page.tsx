'use client';

import React, { useRef, useState } from 'react';
import { Button, Dropdown, Input, Modal, Space, Tag, Tooltip, message } from 'antd';
import type { MenuProps } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ApiOutlined,
  EditOutlined,
  KeyOutlined,
  LinkOutlined,
  MoreOutlined,
  PlusOutlined,
  PoweroffOutlined,
  SafetyOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  createSsoClient,
  disableSsoClientSecret,
  generateSsoClientSecret,
  getSsoClient,
  getSsoClientCapabilities,
  listSsoClientRedirectUris,
  listSsoClients,
  listSsoClientSecrets,
  updateSsoClient,
  updateSsoClientRedirectUris,
  updateSsoClientStatus,
  type SsoClientCapabilities,
  type SsoClientCreateRequest,
  type SsoClientDetail,
  type SsoClientRecord,
  type SsoClientSecretGenerateRequest,
  type SsoClientSecretRecord,
  type SsoClientSecretStatusRequest,
  type SsoClientStatusRequest,
  type SsoClientUpdateRequest,
  type SsoRedirectUriRecord,
  type SsoRedirectUriUpdateRequest,
} from '@/api/ssoController';
import { usePermissionFlags } from '@/hooks/auth';
import { SSO_CLIENT_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';
import ClientFormDrawer from './components/ClientFormDrawer';
import IntegrationSummary from './components/IntegrationSummary';
import RedirectUriDrawer from './components/RedirectUriDrawer';
import SecretDrawer from './components/SecretDrawer';

type DrawerMode = 'create' | 'edit';

function renderStatusTag(status: number) {
  return (
    <Tag color={status === 0 ? 'green' : 'default'}>{status === 0 ? '启用' : '停用'}</Tag>
  );
}

function renderBoolTag(value: boolean, yes = '是', no = '否') {
  return <Tag color={value ? 'blue' : 'default'}>{value ? yes : no}</Tag>;
}

const CLIENT_TYPE_LABEL: Record<string, string> = {
  PUBLIC: '公开客户端',
  CONFIDENTIAL: '保密客户端',
};

const AUTH_METHOD_LABEL: Record<string, string> = {
  none: '无需密钥',
  client_secret_basic: '使用客户端密钥',
};

const GRANT_TYPE_LABEL: Record<string, string> = {
  authorization_code: '授权码登录',
  refresh_token: '刷新登录状态',
};

const SCOPE_LABEL: Record<string, string> = {
  openid: '识别用户身份',
  profile: '读取基础资料',
  email: '读取邮箱',
  offline_access: '保持登录',
  'authorization.console': '访问管理控制台',
};

function valueLabel(value: string, labels?: Record<string, string>) {
  return labels?.[value] || labels?.[value.toLowerCase()] || labels?.[value.toUpperCase()] || value;
}

function renderTagList(values?: string[], labels?: Record<string, string>) {
  if (!values?.length) {
    return '-';
  }
  return (
    <Space size={[4, 4]} wrap>
      {values.map((value) => (
        <Tag key={value}>{valueLabel(value, labels)}</Tag>
      ))}
    </Space>
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

export default function SsoClientPage() {
  const actionRef = useRef<ActionType>(undefined);
  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<DrawerMode>('create');
  const [selectedClient, setSelectedClient] = useState<SsoClientRecord | null>(null);
  const [detailClient, setDetailClient] = useState<SsoClientDetail | null>(null);
  const [summaryOpen, setSummaryOpen] = useState(false);
  const [redirectOpen, setRedirectOpen] = useState(false);
  const [secretOpen, setSecretOpen] = useState(false);
  const [redirectRecords, setRedirectRecords] = useState<SsoRedirectUriRecord[]>([]);
  const [secretRecords, setSecretRecords] = useState<SsoClientSecretRecord[]>([]);
  const [redirectLoading, setRedirectLoading] = useState(false);
  const [secretLoading, setSecretLoading] = useState(false);
  const [statusUpdatingClientId, setStatusUpdatingClientId] = useState<string | null>(null);

  const permissions = usePermissionFlags({
    canCreate: SSO_CLIENT_PERMISSIONS.ADD,
    canQuery: SSO_CLIENT_PERMISSIONS.QUERY,
    canEdit: SSO_CLIENT_PERMISSIONS.EDIT,
    canChangeStatus: SSO_CLIENT_PERMISSIONS.STATUS,
    canListRedirects: SSO_CLIENT_PERMISSIONS.REDIRECT_LIST,
    canEditRedirects: SSO_CLIENT_PERMISSIONS.REDIRECT_EDIT,
    canListSecrets: SSO_CLIENT_PERMISSIONS.SECRET_LIST,
    canGenerateSecret: SSO_CLIENT_PERMISSIONS.SECRET_GENERATE,
    canDisableSecret: SSO_CLIENT_PERMISSIONS.SECRET_DISABLE,
  });

  const capabilitiesQuery = useQuery<SsoClientCapabilities>({
    queryKey: ['sso-client-capabilities'],
    queryFn: getSsoClientCapabilities,
  });

  const createMutation = useMutation({
    mutationFn: createSsoClient,
    onSuccess: () => {
      message.success('SSO 客户端已创建');
      setFormOpen(false);
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '创建 SSO 客户端失败'),
  });

  const updateMutation = useMutation({
    mutationFn: ({ clientId, body }: { clientId: string; body: SsoClientUpdateRequest }) =>
      updateSsoClient(clientId, body),
    onSuccess: () => {
      message.success('SSO 客户端已更新');
      setFormOpen(false);
      setSelectedClient(null);
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '更新 SSO 客户端失败'),
  });

  const statusMutation = useMutation({
    mutationFn: ({ clientId, body }: { clientId: string; body: SsoClientStatusRequest }) =>
      updateSsoClientStatus(clientId, body),
    onSuccess: () => {
      message.success('SSO 客户端状态已更新');
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '更新 SSO 客户端状态失败'),
    onSettled: () => setStatusUpdatingClientId(null),
  });

  const redirectMutation = useMutation({
    mutationFn: ({ clientId, body }: { clientId: string; body: SsoRedirectUriUpdateRequest }) =>
      updateSsoClientRedirectUris(clientId, body),
    onSuccess: (records) => {
      message.success('回调地址已保存');
      setRedirectRecords(records);
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '保存回调地址失败'),
  });

  const loadClientDetail = async (record: SsoClientRecord) => {
    const detail = await getSsoClient(record.clientId);
    setDetailClient(detail);
    return detail;
  };

  const loadRedirects = async (record = selectedClient) => {
    if (!record) {
      return;
    }
    setRedirectLoading(true);
    try {
      const records = await listSsoClientRedirectUris(record.clientId);
      setRedirectRecords(records);
    } catch (error) {
      showError(error, '加载回调地址失败');
    } finally {
      setRedirectLoading(false);
    }
  };

  const loadSecrets = async (record = selectedClient) => {
    if (!record) {
      return;
    }
    setSecretLoading(true);
    try {
      const records = await listSsoClientSecrets(record.clientId);
      setSecretRecords(records);
    } catch (error) {
      showError(error, '加载客户端密钥失败');
    } finally {
      setSecretLoading(false);
    }
  };

  const openCreate = () => {
    setFormMode('create');
    setSelectedClient(null);
    setDetailClient(null);
    setFormOpen(true);
  };

  const openEdit = async (record: SsoClientRecord) => {
    try {
      const detail = await loadClientDetail(record);
      setSelectedClient(detail);
      setFormMode('edit');
      setFormOpen(true);
    } catch (error) {
      showError(error, '获取客户端详情失败');
    }
  };

  const openSummary = async (record: SsoClientRecord) => {
    try {
      await loadClientDetail(record);
      setSummaryOpen(true);
    } catch (error) {
      showError(error, '获取接入摘要失败');
    }
  };

  const openRedirects = async (record: SsoClientRecord) => {
    setSelectedClient(record);
    setRedirectOpen(true);
    await loadRedirects(record);
  };

  const openSecrets = async (record: SsoClientRecord) => {
    setSelectedClient(record);
    setSecretOpen(true);
    await loadSecrets(record);
  };

  const handleStatusChange = async (record: SsoClientRecord) => {
    const nextStatus = record.status === 0 ? 1 : 0;
    const reason = await askReason(
      nextStatus === 0 ? '启用 SSO 客户端' : '停用 SSO 客户端',
      nextStatus === 0 ? '请输入启用原因' : '请输入停用原因',
    );
    if (!reason) {
      return;
    }
    setStatusUpdatingClientId(record.clientId);
    statusMutation.mutate({
      clientId: record.clientId,
      body: {
        status: nextStatus,
        reason,
        revokeActiveSessions: nextStatus === 1,
      },
    });
  };

  const handleFormSubmit = async (values: SsoClientCreateRequest | SsoClientUpdateRequest) => {
    if (formMode === 'create') {
      await createMutation.mutateAsync(values as SsoClientCreateRequest);
      return;
    }
    if (!selectedClient) {
      message.error('未选择客户端');
      return;
    }
    await updateMutation.mutateAsync({
      clientId: selectedClient.clientId,
      body: values as SsoClientUpdateRequest,
    });
  };

  const columns: ProColumns<SsoClientRecord>[] = [
    {
      title: '客户端标识',
      dataIndex: 'clientId',
      copyable: true,
      width: 220,
      render: (_, record) => <Tag color="blue">{record.clientId}</Tag>,
    },
    {
      title: '客户端名称',
      dataIndex: 'clientName',
      width: 180,
      search: false,
    },
    {
      title: '类型',
      dataIndex: 'clientType',
      width: 130,
      valueType: 'select',
      valueEnum: Object.fromEntries(
        (capabilitiesQuery.data?.clientTypes || []).map((item) => [
          item,
          { text: valueLabel(item, CLIENT_TYPE_LABEL) },
        ]),
      ),
      render: (_, record) => valueLabel(record.clientType, CLIENT_TYPE_LABEL),
    },
    {
      title: '校验方式',
      dataIndex: 'clientAuthMethod',
      width: 170,
      search: false,
      render: (_, record) => valueLabel(record.clientAuthMethod, AUTH_METHOD_LABEL),
    },
    {
      title: '登录能力',
      dataIndex: 'grantTypes',
      width: 220,
      search: false,
      render: (_, record) => renderTagList(record.grantTypes, GRANT_TYPE_LABEL),
    },
    {
      title: '可访问内容',
      dataIndex: 'scopes',
      width: 220,
      search: false,
      render: (_, record) => renderTagList(record.scopes, SCOPE_LABEL),
    },
    {
      title: '安全校验',
      dataIndex: 'requirePkce',
      width: 110,
      search: false,
      render: (_, record) => renderBoolTag(record.requirePkce),
    },
    {
      title: '可信',
      dataIndex: 'trustedFirstParty',
      width: 90,
      search: false,
      render: (_, record) => renderBoolTag(record.trustedFirstParty),
    },
    {
      title: '回调',
      dataIndex: 'activeRedirectUriCount',
      width: 80,
      search: false,
    },
    {
      title: '密钥',
      dataIndex: 'activeSecretCount',
      width: 80,
      search: false,
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      valueType: 'select',
      valueEnum: {
        0: { text: '启用' },
        1: { text: '停用' },
      },
      render: (_, record) => renderStatusTag(record.status),
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
      width: 210,
      search: false,
      render: (_, record) => {
        const items: MenuProps['items'] = [
          permissions.canQuery
            ? {
                key: 'summary',
                icon: <ApiOutlined />,
                label: '接入摘要',
              }
            : null,
          permissions.canEdit
            ? {
                key: 'edit',
                icon: <EditOutlined />,
                label: '编辑',
              }
            : null,
          permissions.canListRedirects
            ? {
                key: 'redirect',
                icon: <LinkOutlined />,
                label: '回调地址',
              }
            : null,
          permissions.canListSecrets && record.clientType === 'CONFIDENTIAL'
            ? {
                key: 'secret',
                icon: <KeyOutlined />,
                label: '密钥',
              }
            : null,
          permissions.canChangeStatus
            ? {
                key: 'status',
                icon: <PoweroffOutlined />,
                label: record.status === 0 ? '停用' : '启用',
                danger: record.status === 0,
              }
            : null,
        ].filter(Boolean) as MenuProps['items'];

        if (!items?.length) {
          return null;
        }

        return (
          <Space size="small">
            {permissions.canQuery ? (
              <Button type="link" size="small" onClick={() => void openSummary(record)}>
                详情
              </Button>
            ) : null}
            {permissions.canEdit ? (
              <Button type="link" size="small" onClick={() => void openEdit(record)}>
                编辑
              </Button>
            ) : null}
            <Dropdown
              menu={{
                items,
                onClick: ({ key }) => {
                  if (key === 'summary') void openSummary(record);
                  if (key === 'edit') void openEdit(record);
                  if (key === 'redirect') void openRedirects(record);
                  if (key === 'secret') void openSecrets(record);
                  if (key === 'status') void handleStatusChange(record);
                },
              }}
            >
              <Button
                size="small"
                icon={<MoreOutlined />}
                loading={statusUpdatingClientId === record.clientId}
              />
            </Dropdown>
          </Space>
        );
      },
    },
  ];

  return (
    <div className="min-h-full bg-[#f6f8fb] px-6 py-6">
      <ProTable<SsoClientRecord>
        rowKey="clientId"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 100 }}
        scroll={{ x: 1680 }}
        cardBordered
        headerTitle="统一登录接入"
        toolBarRender={() => [
          permissions.canCreate ? (
            <Tooltip
              key="create"
              title={capabilitiesQuery.isLoading ? '正在加载客户端能力' : undefined}
            >
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={openCreate}
                disabled={!capabilitiesQuery.data}
              >
                新增客户端
              </Button>
            </Tooltip>
          ) : null,
        ]}
        request={async (params) => {
          const page = await listSsoClients({
            keyword: typeof params.clientId === 'string' ? params.clientId : undefined,
            status:
              typeof params.status === 'string'
                ? Number(params.status)
                : typeof params.status === 'number'
                  ? params.status
                  : undefined,
            clientType: typeof params.clientType === 'string' ? params.clientType : undefined,
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

      <ClientFormDrawer
        open={formOpen}
        mode={formMode}
        capabilities={capabilitiesQuery.data || null}
        initialValues={formMode === 'edit' ? selectedClient : null}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        onClose={() => setFormOpen(false)}
        onSubmit={handleFormSubmit}
      />

      <IntegrationSummary
        open={summaryOpen}
        client={detailClient}
        capabilities={capabilitiesQuery.data || null}
        onClose={() => setSummaryOpen(false)}
      />

      <RedirectUriDrawer
        open={redirectOpen}
        client={selectedClient}
        canEdit={permissions.canEditRedirects}
        loading={redirectLoading}
        saving={redirectMutation.isPending}
        records={redirectRecords}
        onClose={() => setRedirectOpen(false)}
        onReload={() => loadRedirects()}
        onSubmit={async (body) => {
          if (!selectedClient) {
            return;
          }
          await redirectMutation.mutateAsync({ clientId: selectedClient.clientId, body });
        }}
      />

      <SecretDrawer
        open={secretOpen}
        client={selectedClient}
        canDisable={permissions.canDisableSecret && selectedClient?.clientType === 'CONFIDENTIAL'}
        canGenerate={permissions.canGenerateSecret && selectedClient?.clientType === 'CONFIDENTIAL'}
        loading={secretLoading}
        records={secretRecords}
        onClose={() => setSecretOpen(false)}
        onReload={() => loadSecrets()}
        onGenerate={(body: SsoClientSecretGenerateRequest) => {
          if (!selectedClient) {
            return Promise.reject(new Error('未选择客户端'));
          }
          return generateSsoClientSecret(selectedClient.clientId, body);
        }}
        onDisable={async (
          secretId: string | number,
          body: SsoClientSecretStatusRequest,
        ) => {
          if (!selectedClient) {
            throw new Error('未选择客户端');
          }
          await disableSsoClientSecret(selectedClient.clientId, secretId, body);
        }}
      />
    </div>
  );
}
