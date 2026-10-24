'use client';

import React, { useEffect, useRef, useState } from 'react';
import {
  Button,
  Checkbox,
  Dropdown,
  Form,
  Input,
  Modal,
  Space,
  Tag,
  Tooltip,
  message,
} from 'antd';
import type { MenuProps } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  ApiOutlined,
  EditOutlined,
  IdcardOutlined,
  KeyOutlined,
  MoreOutlined,
  PlusOutlined,
  PoweroffOutlined,
  SafetyOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  createExternalLoginProvider,
  getExternalLoginCapabilities,
  getExternalLoginProvider,
  listExternalLoginIdentities,
  listExternalLoginProviders,
  listExternalOAuthTokens,
  revokeExternalOAuthToken,
  rotateExternalLoginProviderSecret,
  updateExternalLoginIdentityStatus,
  updateExternalLoginProvider,
  updateExternalLoginProviderStatus,
  type ExternalLoginCapabilities,
  type ExternalLoginIdentityRecord,
  type ExternalLoginProviderCreateRequest,
  type ExternalLoginProviderRecord,
  type ExternalLoginProviderStatusRequest,
  type ExternalLoginProviderUpdateRequest,
  type ExternalOAuthTokenRecord,
} from '@/api/externalLoginController';
import { usePermissionFlags } from '@/hooks/auth';
import { EXTERNAL_LOGIN_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';
import CapabilitySummary from './components/CapabilitySummary';
import IdentityBindingDrawer from './components/IdentityBindingDrawer';
import ProviderFormDrawer from './components/ProviderFormDrawer';
import TokenDrawer from './components/TokenDrawer';

type DrawerMode = 'create' | 'edit';

interface StatusFormValues {
  reason: string;
  revokeActiveSessions?: boolean;
}

interface RotateSecretFormValues {
  clientSecret: string;
  reason: string;
}

const PROVIDER_STATUS_LABEL: Record<number, { text: string; color: string }> = {
  0: { text: '启用', color: 'green' },
  1: { text: '停用', color: 'default' },
};

function renderStatusTag(status: number) {
  const meta = PROVIDER_STATUS_LABEL[status] || PROVIDER_STATUS_LABEL[0];
  return <Tag color={meta.color}>{meta.text}</Tag>;
}

function renderBoolTag(value: boolean, yes = '是', no = '否') {
  return <Tag color={value ? 'blue' : 'default'}>{value ? yes : no}</Tag>;
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return '-';
  }
  return new Date(value).toLocaleString();
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

function capabilitiesToValueEnum(capabilities?: ExternalLoginCapabilities | null) {
  return Object.fromEntries(
    Object.values(capabilities || {}).map((item) => [
      item.protocolType,
      { text: item.protocolType },
    ]),
  );
}

export default function ExternalLoginProviderPage() {
  const actionRef = useRef<ActionType>(undefined);
  const [form] = Form.useForm<StatusFormValues>();
  const [rotateForm] = Form.useForm<RotateSecretFormValues>();

  const [formOpen, setFormOpen] = useState(false);
  const [formMode, setFormMode] = useState<DrawerMode>('create');
  const [selectedProvider, setSelectedProvider] = useState<ExternalLoginProviderRecord | null>(
    null,
  );
  const [detailProvider, setDetailProvider] = useState<ExternalLoginProviderRecord | null>(null);
  const [summaryOpen, setSummaryOpen] = useState(false);
  const [identityOpen, setIdentityOpen] = useState(false);
  const [tokenOpen, setTokenOpen] = useState(false);
  const [identityRecords, setIdentityRecords] = useState<ExternalLoginIdentityRecord[]>([]);
  const [tokenRecords, setTokenRecords] = useState<ExternalOAuthTokenRecord[]>([]);
  const [identityLoading, setIdentityLoading] = useState(false);
  const [tokenLoading, setTokenLoading] = useState(false);
  const [statusTarget, setStatusTarget] = useState<ExternalLoginProviderRecord | null>(null);
  const [rotateTarget, setRotateTarget] = useState<ExternalLoginProviderRecord | null>(null);
  const [statusUpdatingProviderCode, setStatusUpdatingProviderCode] = useState<string | null>(null);

  const permissions = usePermissionFlags({
    canCreate: EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_ADD,
    canQuery: EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_QUERY,
    canEdit: EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_EDIT,
    canChangeStatus: EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_STATUS,
    canRotateSecret: EXTERNAL_LOGIN_PERMISSIONS.PROVIDER_SECRET_ROTATE,
    canListIdentities: EXTERNAL_LOGIN_PERMISSIONS.IDENTITY_LIST,
    canDisableIdentity: EXTERNAL_LOGIN_PERMISSIONS.IDENTITY_STATUS,
    canListTokens: EXTERNAL_LOGIN_PERMISSIONS.TOKEN_LIST,
    canRevokeToken: EXTERNAL_LOGIN_PERMISSIONS.TOKEN_REVOKE,
  });

  const capabilitiesQuery = useQuery<ExternalLoginCapabilities>({
    queryKey: ['external-login-capabilities'],
    queryFn: getExternalLoginCapabilities,
  });
  const createMutation = useMutation({
    mutationFn: createExternalLoginProvider,
    onSuccess: () => {
      message.success('外部登录Provider已创建');
      setFormOpen(false);
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '创建外部登录Provider失败'),
  });

  const updateMutation = useMutation({
    mutationFn: ({
      providerCode,
      body,
    }: {
      providerCode: string;
      body: ExternalLoginProviderUpdateRequest;
    }) => updateExternalLoginProvider(providerCode, body),
    onSuccess: () => {
      message.success('外部登录Provider已更新');
      setFormOpen(false);
      setSelectedProvider(null);
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '更新外部登录Provider失败'),
  });

  const statusMutation = useMutation({
    mutationFn: ({
      providerCode,
      body,
    }: {
      providerCode: string;
      body: ExternalLoginProviderStatusRequest;
    }) => updateExternalLoginProviderStatus(providerCode, body),
    onSuccess: () => {
      message.success('外部登录Provider状态已更新');
      setStatusTarget(null);
      form.resetFields();
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '更新外部登录Provider状态失败'),
    onSettled: () => setStatusUpdatingProviderCode(null),
  });

  const rotateSecretMutation = useMutation({
    mutationFn: ({
      providerCode,
      body,
    }: {
      providerCode: string;
      body: RotateSecretFormValues;
    }) => rotateExternalLoginProviderSecret(providerCode, body),
    onSuccess: (result) => {
      const hint = result.secretHint ? `，提示：${result.secretHint}` : '';
      message.success(`Provider密钥已轮换${hint}。明文密钥未在管理台展示。`);
      setRotateTarget(null);
      rotateForm.resetFields();
      actionRef.current?.reload();
    },
    onError: (error) => showError(error, '轮换Provider密钥失败'),
  });

  const identityStatusMutation = useMutation({
    mutationFn: ({
      identityId,
      reason,
    }: {
      identityId: string | number;
      reason: string;
    }) =>
      updateExternalLoginIdentityStatus(identityId, {
        status: 1,
        reason,
      }),
    onSuccess: () => {
      message.success('外部身份绑定已禁用');
    },
    onError: (error) => showError(error, '禁用外部身份绑定失败'),
  });

  const tokenRevokeMutation = useMutation({
    mutationFn: ({ tokenId, reason }: { tokenId: string | number; reason: string }) =>
      revokeExternalOAuthToken(tokenId, { reason }),
    onSuccess: () => {
      message.success('外部OAuth令牌已撤销');
    },
    onError: (error) => showError(error, '撤销外部OAuth令牌失败'),
  });

  useEffect(() => {
    if (!statusTarget) {
      form.resetFields();
      return;
    }
    const disabling = statusTarget.status === 0;
    form.setFieldsValue({
      reason: '',
      revokeActiveSessions: disabling,
    });
  }, [form, statusTarget]);

  const loadProviderDetail = async (record: ExternalLoginProviderRecord) => {
    const detail = await getExternalLoginProvider(record.providerCode);
    setDetailProvider(detail);
    return detail;
  };

  const loadIdentities = async (record = selectedProvider) => {
    if (!record) {
      return;
    }
    setIdentityLoading(true);
    try {
      const page = await listExternalLoginIdentities({
        providerCode: record.providerCode,
        current: 1,
        pageSize: 100,
      });
      setIdentityRecords(page.records);
    } catch (error) {
      showError(error, '加载外部身份绑定失败');
    } finally {
      setIdentityLoading(false);
    }
  };

  const loadTokens = async (record = selectedProvider) => {
    if (!record) {
      return;
    }
    setTokenLoading(true);
    try {
      const page = await listExternalOAuthTokens({
        providerCode: record.providerCode,
        current: 1,
        pageSize: 100,
      });
      setTokenRecords(page.records);
    } catch (error) {
      showError(error, '加载外部OAuth令牌失败');
    } finally {
      setTokenLoading(false);
    }
  };

  const openCreate = () => {
    setFormMode('create');
    setSelectedProvider(null);
    setDetailProvider(null);
    setFormOpen(true);
  };

  const openEdit = async (record: ExternalLoginProviderRecord) => {
    try {
      const detail = await loadProviderDetail(record);
      setSelectedProvider(detail);
      setFormMode('edit');
      setFormOpen(true);
    } catch (error) {
      showError(error, '获取Provider详情失败');
    }
  };

  const openSummary = async (record: ExternalLoginProviderRecord) => {
    try {
      await loadProviderDetail(record);
      setSummaryOpen(true);
    } catch (error) {
      showError(error, '获取Provider能力摘要失败');
    }
  };

  const openIdentities = async (record: ExternalLoginProviderRecord) => {
    setSelectedProvider(record);
    setIdentityOpen(true);
    await loadIdentities(record);
  };

  const openTokens = async (record: ExternalLoginProviderRecord) => {
    setSelectedProvider(record);
    setTokenOpen(true);
    await loadTokens(record);
  };

  const handleFormSubmit = async (
    values: ExternalLoginProviderCreateRequest | ExternalLoginProviderUpdateRequest,
  ) => {
    if (formMode === 'create') {
      await createMutation.mutateAsync(values as ExternalLoginProviderCreateRequest);
      return;
    }
    if (!selectedProvider) {
      message.error('未选择Provider');
      return;
    }
    await updateMutation.mutateAsync({
      providerCode: selectedProvider.providerCode,
      body: values as ExternalLoginProviderUpdateRequest,
    });
  };

  const handleStatusSubmit = async () => {
    if (!statusTarget) {
      return;
    }
    const values = await form.validateFields();
    const nextStatus = statusTarget.status === 0 ? 1 : 0;
    setStatusUpdatingProviderCode(statusTarget.providerCode);
    await statusMutation.mutateAsync({
      providerCode: statusTarget.providerCode,
      body: {
        status: nextStatus,
        reason: values.reason.trim(),
        revokeActiveSessions: nextStatus === 1 ? values.revokeActiveSessions !== false : false,
      },
    });
  };

  const handleRotateSecret = async () => {
    if (!rotateTarget) {
      return;
    }
    const values = await rotateForm.validateFields();
    await rotateSecretMutation.mutateAsync({
      providerCode: rotateTarget.providerCode,
      body: {
        clientSecret: values.clientSecret.trim(),
        reason: values.reason.trim(),
      },
    });
  };

  const columns: ProColumns<ExternalLoginProviderRecord>[] = [
    {
      title: 'Provider编码',
      dataIndex: 'providerCode',
      copyable: true,
      width: 160,
      render: (_, record) => <Tag color="blue">{record.providerCode}</Tag>,
    },
    {
      title: 'Provider名称',
      dataIndex: 'providerName',
      width: 180,
      search: false,
    },
    {
      title: '协议类型',
      dataIndex: 'protocolType',
      width: 120,
      valueType: 'select',
      valueEnum: capabilitiesToValueEnum(capabilitiesQuery.data),
    },
    {
      title: '展示',
      dataIndex: 'displayEnabled',
      width: 90,
      search: false,
      render: (_, record) => renderBoolTag(record.displayEnabled),
    },
    {
      title: '登录',
      dataIndex: 'loginEnabled',
      width: 90,
      search: false,
      render: (_, record) => renderBoolTag(record.loginEnabled),
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
      title: '排序',
      dataIndex: 'sortOrder',
      width: 90,
      search: false,
    },
    {
      title: '更新时间',
      dataIndex: 'updateTime',
      valueType: 'dateTime',
      width: 180,
      search: false,
      render: (_, record) => formatDateTime(record.updateTime),
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 240,
      search: false,
      render: (_, record) => {
        const items: MenuProps['items'] = [
          permissions.canQuery
            ? {
                key: 'summary',
                icon: <ApiOutlined />,
                label: '能力摘要',
              }
            : null,
          permissions.canEdit
            ? {
                key: 'edit',
                icon: <EditOutlined />,
                label: '编辑',
              }
            : null,
          permissions.canListIdentities
            ? {
                key: 'identities',
                icon: <IdcardOutlined />,
                label: '身份绑定',
              }
            : null,
          permissions.canListTokens
            ? {
                key: 'tokens',
                icon: <SafetyOutlined />,
                label: 'OAuth令牌',
              }
            : null,
          permissions.canRotateSecret
            ? {
                key: 'rotate',
                icon: <KeyOutlined />,
                label: '轮换密钥',
                danger: true,
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
                  if (key === 'identities') void openIdentities(record);
                  if (key === 'tokens') void openTokens(record);
                  if (key === 'rotate') {
                    setRotateTarget(record);
                    rotateForm.resetFields();
                  }
                  if (key === 'status') setStatusTarget(record);
                },
              }}
            >
              <Button
                size="small"
                icon={<MoreOutlined />}
                loading={statusUpdatingProviderCode === record.providerCode}
              />
            </Dropdown>
          </Space>
        );
      },
    },
  ];

  return (
    <div className="min-h-full bg-[#f6f8fb] px-6 py-6">
      <ProTable<ExternalLoginProviderRecord>
        rowKey="providerCode"
        actionRef={actionRef}
        columns={columns}
        search={{ labelWidth: 100 }}
        scroll={{ x: 1380 }}
        cardBordered
        headerTitle="外部登录Provider"
        toolBarRender={() => [
          permissions.canCreate ? (
            <Tooltip
              key="create"
              title={capabilitiesQuery.isLoading ? '正在加载外部登录能力' : undefined}
            >
              <Button
                type="primary"
                icon={<PlusOutlined />}
                onClick={openCreate}
                disabled={!capabilitiesQuery.data}
              >
                新增Provider
              </Button>
            </Tooltip>
          ) : null,
        ]}
        request={async (params) => {
          const page = await listExternalLoginProviders({
            keyword: typeof params.providerCode === 'string' ? params.providerCode : undefined,
            status:
              typeof params.status === 'string'
                ? Number(params.status)
                : typeof params.status === 'number'
                  ? params.status
                  : undefined,
            protocolType:
              typeof params.protocolType === 'string' ? params.protocolType : undefined,
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

      <ProviderFormDrawer
        open={formOpen}
        mode={formMode}
        capabilities={capabilitiesQuery.data || null}
        initialValues={formMode === 'edit' ? selectedProvider : null}
        confirmLoading={createMutation.isPending || updateMutation.isPending}
        onClose={() => setFormOpen(false)}
        onSubmit={handleFormSubmit}
      />

      <CapabilitySummary
        open={summaryOpen}
        provider={detailProvider}
        capabilities={capabilitiesQuery.data || null}
        onClose={() => setSummaryOpen(false)}
      />

      <IdentityBindingDrawer
        open={identityOpen}
        provider={selectedProvider}
        canDisable={permissions.canDisableIdentity}
        loading={identityLoading}
        records={identityRecords}
        onClose={() => setIdentityOpen(false)}
        onReload={() => loadIdentities()}
        onDisable={async (identityId, values) => {
          await identityStatusMutation.mutateAsync({ identityId, reason: values.reason });
        }}
      />

      <TokenDrawer
        open={tokenOpen}
        provider={selectedProvider}
        canRevoke={permissions.canRevokeToken}
        loading={tokenLoading}
        records={tokenRecords}
        onClose={() => setTokenOpen(false)}
        onReload={() => loadTokens()}
        onRevoke={async (tokenId, values) => {
          await tokenRevokeMutation.mutateAsync({ tokenId, reason: values.reason });
        }}
      />

      <Modal
        title={statusTarget?.status === 0 ? '停用外部登录Provider' : '启用外部登录Provider'}
        open={!!statusTarget}
        onCancel={() => setStatusTarget(null)}
        onOk={handleStatusSubmit}
        confirmLoading={statusMutation.isPending}
        okText={statusTarget?.status === 0 ? '确认停用' : '确认启用'}
        okButtonProps={{ danger: statusTarget?.status === 0 }}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            label="操作原因"
            name="reason"
            rules={[
              { required: true, message: '请输入操作原因' },
              { max: 200, message: '操作原因不能超过 200 个字符' },
            ]}
          >
            <Input.TextArea rows={3} placeholder="用于审计追踪" />
          </Form.Item>
          {statusTarget?.status === 0 ? (
            <Form.Item name="revokeActiveSessions" valuePropName="checked">
              <Checkbox>同时撤销该Provider的活跃会话</Checkbox>
            </Form.Item>
          ) : null}
        </Form>
      </Modal>

      <Modal
        title="轮换Provider Client Secret"
        open={!!rotateTarget}
        onCancel={() => setRotateTarget(null)}
        onOk={handleRotateSecret}
        confirmLoading={rotateSecretMutation.isPending}
        okText="确认轮换"
        okButtonProps={{ danger: true }}
      >
        <Form form={rotateForm} layout="vertical">
          <Form.Item
            label="新的Client Secret"
            name="clientSecret"
            rules={[
              { required: true, message: '请输入新的Client Secret' },
              { max: 2048, message: 'Client Secret不能超过 2048 个字符' },
            ]}
          >
            <Input.Password placeholder="提交后不在管理台展示明文" autoComplete="new-password" />
          </Form.Item>
          <Form.Item
            label="操作原因"
            name="reason"
            rules={[
              { required: true, message: '请输入操作原因' },
              { max: 200, message: '操作原因不能超过 200 个字符' },
            ]}
          >
            <Input.TextArea rows={3} placeholder="用于审计追踪" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
}
