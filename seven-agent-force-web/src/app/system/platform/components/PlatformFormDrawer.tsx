'use client';

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Card,
  Collapse,
  Drawer,
  Form,
  Input,
  InputNumber,
  Modal,
  Select,
  Space,
  Switch,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import { DeleteOutlined, InfoCircleOutlined, PlusOutlined } from '@ant-design/icons';
import type {
  PlatformAdminDefaultRole,
  PlatformAdminLoginMethod,
  PlatformAdminRecord,
  PlatformAdminSourceRule,
} from '@/api/platformController';
import { listExternalLoginProviders, type ExternalLoginProviderRecord } from '@/api/externalLoginController';
import { getDeptOptions } from '@/api/sysDeptController';
import { getActiveOrgs } from '@/api/sysOrgController';
import { getPostList } from '@/api/sysPostController';
import { getRoleList } from '@/api/sysRoleController';

const { Text } = Typography;

export interface PlatformFormSubmitValues {
  platformCode?: string;
  platformName: string;
  platformType?: string;
  description?: string;
  defaultRedirectUrl?: string;
  allowAutoRegister?: boolean;
  allowFormRegister?: boolean;
  isDefault?: boolean;
  defaultOrgId?: API.Int64;
  defaultDeptId?: API.Int64;
  brandJson?: string;
  settingsJson?: string;
  loginMethods: PlatformAdminLoginMethod[];
  sourceRules: PlatformAdminSourceRule[];
  defaultRoleIds: API.Int64[];
  defaultRoles: PlatformAdminDefaultRole[];
  reason: string;
  stepUpProof: string;
}

export interface PlatformFormDrawerProps {
  open: boolean;
  mode: 'create' | 'edit';
  initialValues?: PlatformAdminRecord | null;
  confirmLoading?: boolean;
  canEditLoginMethods?: boolean;
  canEditSourceRules?: boolean;
  canEditDefaultRoles?: boolean;
  onClose: () => void;
  onSubmit: (values: PlatformFormSubmitValues) => Promise<void>;
}

interface PlatformFormValues {
  platformCode?: string;
  platformName?: string;
  platformType?: string;
  description?: string;
  defaultRedirectUrl?: string;
  allowAutoRegister?: boolean;
  allowFormRegister?: boolean;
  isDefault?: boolean;
  defaultOrgId?: API.Int64;
  defaultDeptId?: API.Int64;
  brandTitle?: string;
  brandSubtitle?: string;
  brandTheme?: string;
  supportUrl?: string;
  loginPrompt?: string;
  defaultPostIds?: API.Int64[];
  loginMethods?: PlatformAdminLoginMethod[];
  sourceRules?: PlatformAdminSourceRule[];
  defaultRoleIds?: API.Int64[];
}

interface SimpleOption {
  label: string;
  value: API.Int64;
  disabled?: boolean;
  orgId?: API.Int64;
  deptId?: API.Int64;
}

const LOGIN_METHOD_OPTIONS = [
  { label: '账号密码', value: 'PASSWORD' },
  { label: 'Passkey', value: 'PASSKEY' },
  { label: '外部 OAuth', value: 'EXTERNAL_OAUTH' },
];

const SOURCE_RULE_OPTIONS = [
  { label: 'SSO Client ID', value: 'CLIENT_ID' },
  { label: '访问 Host', value: 'HOST' },
  { label: 'Origin', value: 'ORIGIN' },
  { label: 'Referer Host', value: 'REFERER_HOST' },
  { label: 'Redirect Host', value: 'REDIRECT_HOST' },
  { label: 'Redirect 前缀', value: 'REDIRECT_PREFIX' },
];

const PLATFORM_TYPE_META: Record<string, { label: string; description: string }> = {
  ADMIN: {
    label: '管理后台',
    description: '面向系统管理员和运营人员，通常拥有较高权限和完整后台菜单。',
  },
  CONSOLE: {
    label: '控制台',
    description: '面向内部或租户管理员，承载平台配置、账号安全和日常运维入口。',
  },
  PORTAL: {
    label: '门户站点',
    description: '面向普通用户或业务用户，通常只开放轻量功能和自助服务。',
  },
  BUSINESS: {
    label: '业务平台',
    description: '面向具体业务系统或多平台后台，用于配置独立登录方式、来源规则和默认权限。',
  },
  API: {
    label: '开放 API',
    description: '面向程序化调用或第三方系统接入，通常按 Client 和来源规则识别。',
  },
};

const PLATFORM_TYPE_OPTIONS = Object.entries(PLATFORM_TYPE_META).map(([value, meta]) => ({
  value,
  label: (
    <Tooltip title={meta.description}>
      <Space size={4}>
        {meta.label}
        <InfoCircleOutlined />
      </Space>
    </Tooltip>
  ),
}));

const BRAND_THEME_OPTIONS = [
  { label: '蓝青默认', value: 'blue-cyan' },
  { label: '简洁浅色', value: 'light' },
  { label: '深色控制台', value: 'dark' },
];

function parseJsonRecord(value?: string): Record<string, unknown> {
  if (!value?.trim()) {
    return {};
  }
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function stringValue(value: unknown) {
  return typeof value === 'string' ? value : '';
}

function buildJsonText(value: Record<string, unknown>) {
  const compact = Object.fromEntries(
    Object.entries(value).filter(([, item]) => item !== undefined && item !== null && item !== ''),
  );
  return Object.keys(compact).length ? JSON.stringify(compact, null, 2) : undefined;
}

function normalizeLoginMethods(values?: PlatformAdminLoginMethod[]) {
  return (values || [])
    .map((item, index) => {
      const methodType = String(item.methodType || 'PASSWORD').trim().toUpperCase();
      const enabled = item.enabled !== false;
      return {
        methodType,
        providerCode: methodType === 'EXTERNAL_OAUTH' ? item.providerCode?.trim() || undefined : undefined,
        displayName: item.displayName?.trim() || '',
        icon: item.icon?.trim() || undefined,
        sortOrder: Number.isFinite(Number(item.sortOrder)) ? Number(item.sortOrder) : index,
        displayEnabled: item.displayEnabled ?? enabled,
        loginEnabled: item.loginEnabled ?? enabled,
        enabled,
        metadataJson: item.metadataJson?.trim() || undefined,
      };
    })
    .filter((item) => item.methodType && item.displayName);
}

function normalizeSourceRules(values?: PlatformAdminSourceRule[]) {
  return (values || [])
    .map((item, index) => ({
      matchType: item.matchType?.trim().toUpperCase(),
      matchValue: item.matchValue?.trim(),
      priority: Number.isFinite(Number(item.priority)) ? Number(item.priority) : (index + 1) * 10,
      status: item.status ?? 0,
      metadataJson: item.metadataJson?.trim() || undefined,
    }))
    .filter((item) => item.matchType && item.matchValue);
}

function normalizeRoleIds(values?: API.Int64[]) {
  return (values || []).filter((item) =>
    item !== '0' && item.length > 0 && [...item].every((char) => char >= '0' && char <= '9'),
  );
}

function normalizeProviderOptions(records: ExternalLoginProviderRecord[]) {
  return records
    .slice()
    .sort((first, second) => first.sortOrder - second.sortOrder)
    .map((provider) => ({
      label: `${provider.displayName || provider.providerName} (${provider.providerCode})`,
      value: provider.providerCode,
      provider,
    }));
}

function unwrapListResponse(response: unknown): unknown[] {
  const source = response as Record<string, unknown> | undefined;
  const data = source?.data as Record<string, unknown> | unknown[] | undefined;
  if (Array.isArray(data)) {
    return data;
  }
  if (data && typeof data === 'object') {
    const record = data as Record<string, unknown>;
    if (Array.isArray(record.records)) return record.records;
    if (Array.isArray(record.list)) return record.list;
  }
  if (Array.isArray(source?.records)) return source.records;
  if (Array.isArray(source?.list)) return source.list;
  return [];
}

function flattenTreeOptions(values: unknown[]): Record<string, unknown>[] {
  const result: Record<string, unknown>[] = [];
  const visit = (items: unknown[], prefix = '') => {
    items.forEach((item) => {
      if (!item || typeof item !== 'object') {
        return;
      }
      const source = item as Record<string, unknown>;
      const name =
        stringValue(source.orgName) ||
        stringValue(source.deptName) ||
        stringValue(source.postName) ||
        stringValue(source.roleName) ||
        stringValue(source.name);
      const currentPrefix = name ? (prefix ? `${prefix} / ${name}` : name) : prefix;
      result.push({ ...source, displayPath: currentPrefix });
      if (Array.isArray(source.children)) {
        visit(source.children, currentPrefix);
      }
    });
  };
  visit(values);
  return result;
}

function normalizeOrgOptions(response: unknown): SimpleOption[] {
  return flattenTreeOptions(unwrapListResponse(response)).map((item) => {
    const source = item as Record<string, unknown>;
    const orgId = String(source.orgId ?? source.id ?? '');
    const orgName =
      stringValue(source.displayPath) ||
      stringValue(source.orgName) ||
      stringValue(source.name) ||
      `组织 ${orgId}`;
    const orgKey = stringValue(source.orgCode) || stringValue(source.code);
    const status = Number(source.status ?? 0);
    return {
      value: orgId,
      label: orgKey ? `${orgName} (${orgKey})` : orgName,
      disabled: status !== 0,
    };
  }).filter((item) => /^\d+$/.test(item.value) && item.value !== '0');
}

function normalizeRoleOptions(response: unknown): SimpleOption[] {
  return flattenTreeOptions(unwrapListResponse(response)).map((item) => {
    const source = item as Record<string, unknown>;
    const roleId = String(source.roleId ?? source.id ?? '');
    const roleName = stringValue(source.roleName) || stringValue(source.name) || `角色 ${roleId}`;
    const roleKey = stringValue(source.roleKey) || stringValue(source.roleCode) || stringValue(source.code);
    const status = Number(source.status ?? 0);
    return {
      value: roleId,
      label: roleKey ? `${roleName} (${roleKey})` : roleName,
      disabled: status !== 0,
    };
  }).filter((item) => /^\d+$/.test(item.value) && item.value !== '0');
}

function normalizeDeptOptions(response: unknown): SimpleOption[] {
  return flattenTreeOptions(unwrapListResponse(response)).map((item) => {
    const source = item as Record<string, unknown>;
    const deptId = String(source.deptId ?? source.id ?? '');
    const deptName =
      stringValue(source.displayPath) ||
      stringValue(source.deptName) ||
      stringValue(source.name) ||
      `部门 ${deptId}`;
    const deptKey = stringValue(source.deptCode) || stringValue(source.code);
    const status = Number(source.status ?? 0);
    return {
      value: deptId,
      label: deptKey ? `${deptName} (${deptKey})` : deptName,
      disabled: status !== 0,
      orgId: String(source.orgId ?? ''),
    };
  }).filter((item) => /^\d+$/.test(item.value) && item.value !== '0');
}

function normalizePostOptions(response: unknown): SimpleOption[] {
  return flattenTreeOptions(unwrapListResponse(response)).map((item) => {
    const source = item as Record<string, unknown>;
    const postId = String(source.postId ?? source.id ?? '');
    const postName = stringValue(source.postName) || stringValue(source.name) || `岗位 ${postId}`;
    const postKey = stringValue(source.postCode) || stringValue(source.code);
    const status = Number(source.status ?? 0);
    return {
      value: postId,
      label: postKey ? `${postName} (${postKey})` : postName,
      disabled: status !== 0,
      orgId: String(source.orgId ?? ''),
      deptId: String(source.deptId ?? ''),
    };
  }).filter((item) => /^\d+$/.test(item.value) && item.value !== '0');
}

function toInitialValues(
  mode: 'create' | 'edit',
  initialValues?: PlatformAdminRecord | null,
): PlatformFormValues {
  if (!initialValues) {
    return {
      platformType: 'CONSOLE',
      allowAutoRegister: false,
      allowFormRegister: false,
      isDefault: false,
      brandTitle: 'Seven',
      brandSubtitle: '统一身份认证系统',
      brandTheme: 'blue-cyan',
      loginMethods: [
        {
          methodType: 'PASSWORD',
          displayName: '账号密码',
          sortOrder: 0,
          displayEnabled: true,
          loginEnabled: true,
          enabled: true,
        },
      ],
      sourceRules: [],
      defaultRoleIds: [],
    };
  }

  const brand = parseJsonRecord(initialValues.brandJson);
  const settings = parseJsonRecord(initialValues.settingsJson);
  const defaultPostIds = Array.isArray(settings.defaultPostIds)
    ? settings.defaultPostIds.map(String).filter((item) => /^\d+$/.test(item) && item !== '0')
    : [];
  const defaultOrgId = String(settings.defaultOrgId ?? '');
  return {
    ...(mode === 'create' ? { platformCode: initialValues.platformCode } : {}),
    platformName: initialValues.platformName,
    platformType: initialValues.platformType || 'CONSOLE',
    description: initialValues.description,
    defaultRedirectUrl: initialValues.defaultRedirectUrl,
    allowAutoRegister: initialValues.allowAutoRegister,
    allowFormRegister: initialValues.allowFormRegister,
    isDefault: initialValues.isDefault,
    defaultOrgId: /^\d+$/.test(defaultOrgId) && defaultOrgId !== '0' ? defaultOrgId : undefined,
    defaultDeptId: initialValues.defaultDeptId,
    brandTitle: stringValue(brand.title) || initialValues.platformName || 'Seven',
    brandSubtitle: stringValue(brand.subtitle) || '统一身份认证系统',
    brandTheme: stringValue(brand.theme) || 'blue-cyan',
    supportUrl: stringValue(settings.supportUrl),
    loginPrompt: stringValue(settings.loginPrompt),
    defaultPostIds,
    loginMethods: initialValues.loginMethods?.length
      ? initialValues.loginMethods
      : [
          {
            methodType: 'PASSWORD',
            displayName: '账号密码',
            sortOrder: 0,
            displayEnabled: true,
            loginEnabled: true,
            enabled: true,
          },
        ],
    sourceRules: initialValues.sourceRules || [],
    defaultRoleIds: initialValues.defaultRoleIds || [],
  };
}

function buildChangeSummary(mode: 'create' | 'edit', values: PlatformFormValues) {
  const methods = normalizeLoginMethods(values.loginMethods);
  const rules = normalizeSourceRules(values.sourceRules);
  const roles = normalizeRoleIds(values.defaultRoleIds);
  const posts = normalizeRoleIds(values.defaultPostIds);
  const autoRegisterEnabled = values.allowAutoRegister === true;
  const formRegisterEnabled = values.allowFormRegister === true;
  const summary = [
    mode === 'create' ? '创建新平台' : '更新平台配置',
    `登录方式 ${methods.length} 个`,
    `来源规则 ${rules.length} 条`,
    autoRegisterEnabled ? '允许外部账号自动注册' : '禁止外部账号自动注册',
    formRegisterEnabled ? '允许表单注册' : '禁止表单注册',
  ];
  if (autoRegisterEnabled || formRegisterEnabled) {
    if (values.defaultOrgId) {
      summary.push('包含默认组织');
    }
    if (values.defaultDeptId) {
      summary.push('包含默认部门');
    }
    summary.push(`默认岗位 ${posts.length} 个`);
    summary.push(`默认角色 ${roles.length} 个`);
  }
  if (methods.some((item) => item.methodType === 'EXTERNAL_OAUTH')) {
    summary.push('包含外部 OAuth 登录方式');
  }
  return summary;
}

function ProviderHint({ provider }: { provider?: ExternalLoginProviderRecord }) {
  if (!provider) {
    return null;
  }
  const unhealthy = provider.status !== 0 || !provider.loginEnabled;
  return (
    <Alert
      type={unhealthy ? 'warning' : 'info'}
      showIcon
      message={unhealthy ? '该 Provider 当前不可直接登录' : 'Provider 配置已联动'}
      description={
        <Space size={[6, 6]} wrap>
          <Tag color={provider.status === 0 ? 'green' : 'default'}>
            {provider.status === 0 ? 'Provider 启用' : 'Provider 停用'}
          </Tag>
          <Tag color={provider.loginEnabled ? 'green' : 'default'}>
            {provider.loginEnabled ? '允许登录' : '登录禁用'}
          </Tag>
          <Tag color={provider.bindEnabled ? 'blue' : 'default'}>
            {provider.bindEnabled ? '允许绑定' : '绑定禁用'}
          </Tag>
          <Text type="secondary">{provider.redirectUri}</Text>
        </Space>
      }
    />
  );
}

export default function PlatformFormDrawer({
  open,
  mode,
  initialValues,
  confirmLoading,
  canEditLoginMethods = true,
  canEditSourceRules = true,
  canEditDefaultRoles = true,
  onClose,
  onSubmit,
}: PlatformFormDrawerProps) {
  const [form] = Form.useForm<PlatformFormValues>();
  const [providers, setProviders] = useState<ExternalLoginProviderRecord[]>([]);
  const [roleOptions, setRoleOptions] = useState<SimpleOption[]>([]);
  const [orgOptions, setOrgOptions] = useState<SimpleOption[]>([]);
  const [deptOptions, setDeptOptions] = useState<SimpleOption[]>([]);
  const [postOptions, setPostOptions] = useState<SimpleOption[]>([]);
  const [loadingProviders, setLoadingProviders] = useState(false);
  const [loadingRoles, setLoadingRoles] = useState(false);
  const [loadingOrgs, setLoadingOrgs] = useState(false);
  const [loadingDepts, setLoadingDepts] = useState(false);
  const [loadingPosts, setLoadingPosts] = useState(false);
  const watchedValues = Form.useWatch([], form) || {};
  const autoRegisterEnabled = watchedValues.allowAutoRegister === true;
  const formRegisterEnabled = watchedValues.allowFormRegister === true;
  const registrationPolicyEnabled = autoRegisterEnabled || formRegisterEnabled;
  const selectedOrgId = watchedValues.defaultOrgId;
  const selectedDeptId = watchedValues.defaultDeptId;
  const selectedPostIds = useMemo(() => normalizeRoleIds(watchedValues.defaultPostIds), [watchedValues.defaultPostIds]);
  const providerOptions = useMemo(() => normalizeProviderOptions(providers), [providers]);
  const providerMap = useMemo(
    () => new Map(providers.map((provider) => [provider.providerCode, provider])),
    [providers],
  );
  const filteredDeptOptions = useMemo(
    () =>
      deptOptions.filter((option) => {
        if (!selectedOrgId || !option.orgId) {
          return true;
        }
        return option.orgId === selectedOrgId || option.value === selectedDeptId;
      }),
    [deptOptions, selectedDeptId, selectedOrgId],
  );
  const filteredPostOptions = useMemo(
    () =>
      postOptions.filter((option) => {
        if (selectedDeptId && option.deptId) {
          return option.deptId === selectedDeptId || selectedPostIds.includes(option.value);
        }
        if (selectedOrgId && option.orgId) {
          return option.orgId === selectedOrgId || selectedPostIds.includes(option.value);
        }
        return true;
      }),
    [postOptions, selectedDeptId, selectedOrgId, selectedPostIds],
  );

  const loadReferenceData = useCallback(async (isCurrent: () => boolean = () => true) => {
    setLoadingProviders(true);
    setLoadingRoles(true);
    setLoadingOrgs(true);
    setLoadingDepts(true);
    setLoadingPosts(true);
    const [providerResult, roleResult, orgResult, deptResult, postResult] = await Promise.allSettled([
      listExternalLoginProviders({ current: 1, pageSize: 100 }),
      getRoleList(),
      getActiveOrgs(),
      getDeptOptions({ status: 0, limit: 100 }),
      getPostList(),
    ]);
    if (!isCurrent()) {
      return;
    }
    if (providerResult.status === 'fulfilled') {
      setProviders(providerResult.value.records);
    } else {
      message.warning('外部登录 Provider 加载失败');
    }
    if (roleResult.status === 'fulfilled') {
      setRoleOptions(normalizeRoleOptions(roleResult.value));
    } else {
      message.warning('默认角色选项加载失败');
    }
    if (orgResult.status === 'fulfilled') {
      setOrgOptions(normalizeOrgOptions(orgResult.value));
    } else {
      message.warning('组织选项加载失败');
    }
    if (deptResult.status === 'fulfilled') {
      setDeptOptions(normalizeDeptOptions(deptResult.value));
    } else {
      message.warning('部门选项加载失败');
    }
    if (postResult.status === 'fulfilled') {
      setPostOptions(normalizePostOptions(postResult.value));
    } else {
      message.warning('岗位选项加载失败');
    }
    setLoadingProviders(false);
    setLoadingRoles(false);
    setLoadingOrgs(false);
    setLoadingDepts(false);
    setLoadingPosts(false);
  }, []);

  useEffect(() => {
    if (open) {
      form.setFieldsValue(toInitialValues(mode, initialValues));
      let active = true;
      const timer = window.setTimeout(() => {
        void loadReferenceData(() => active);
      }, 0);
      return () => {
        active = false;
        window.clearTimeout(timer);
      };
    } else {
      form.resetFields();
    }
    return undefined;
  }, [form, initialValues, loadReferenceData, mode, open]);

  const previewBrandJson = useMemo(
    () => buildJsonText({
      title: watchedValues.brandTitle,
      subtitle: watchedValues.brandSubtitle,
      theme: watchedValues.brandTheme,
    }),
    [
      watchedValues.brandSubtitle,
      watchedValues.brandTheme,
      watchedValues.brandTitle,
    ],
  );

  const previewSettingsJson = useMemo(
    () => buildJsonText({
      supportUrl: watchedValues.supportUrl,
      loginPrompt: watchedValues.loginPrompt,
      defaultOrgId:
        autoRegisterEnabled && watchedValues.defaultOrgId
          ? String(watchedValues.defaultOrgId)
          : undefined,
      defaultPostIds: autoRegisterEnabled ? normalizeRoleIds(watchedValues.defaultPostIds) : undefined,
    }),
    [
      autoRegisterEnabled,
      watchedValues.defaultOrgId,
      watchedValues.defaultPostIds,
      watchedValues.loginPrompt,
      watchedValues.supportUrl,
    ],
  );

  const confirmAndSubmit = async (values: PlatformFormValues) => {
    const summary = buildChangeSummary(mode, values);
    let reason = '';
    await new Promise<void>((resolve, reject) => {
      Modal.confirm({
        title: mode === 'create' ? '确认创建平台' : '确认保存平台变更',
        width: 560,
        content: (
            <Space orientation="vertical" className="w-full" size="middle">
            <Alert
              type="info"
              showIcon
              message="保存后会立即影响该平台的登录入口和默认注册策略。"
            />
            <Space size={[6, 6]} wrap>
              {summary.map((item) => (
                <Tag key={item}>{item}</Tag>
              ))}
            </Space>
            <Input.TextArea
              rows={4}
              placeholder="请输入本次变更原因"
              onChange={(event) => {
                reason = event.target.value;
              }}
            />
          </Space>
        ),
        okText: '确认保存',
        cancelText: '取消',
        onOk: () => {
          if (!reason.trim()) {
            message.warning('请输入操作原因');
            return Promise.reject(new Error('reason required'));
          }
          resolve();
          return Promise.resolve();
        },
        onCancel: () => reject(new Error('cancelled')),
      });
    });

    const allowAutoRegister = values.allowAutoRegister === true;
    const allowFormRegister = values.allowFormRegister === true;
    const registrationPolicyEnabled = allowAutoRegister || allowFormRegister;
    const roleIds = registrationPolicyEnabled ? normalizeRoleIds(values.defaultRoleIds) : [];
    await onSubmit({
      ...(mode === 'create' ? { platformCode: values.platformCode?.trim().toLowerCase() } : {}),
      platformName: values.platformName?.trim() || '',
      platformType: values.platformType?.trim() || undefined,
      description: values.description?.trim() || undefined,
      defaultRedirectUrl: values.defaultRedirectUrl?.trim() || undefined,
      allowAutoRegister,
      allowFormRegister,
      isDefault: values.isDefault === true,
      defaultDeptId:
        registrationPolicyEnabled && values.defaultDeptId
          ? String(values.defaultDeptId)
          : undefined,
      brandJson: previewBrandJson,
      settingsJson: previewSettingsJson,
      loginMethods: canEditLoginMethods ? normalizeLoginMethods(values.loginMethods) : [],
      sourceRules: canEditSourceRules ? normalizeSourceRules(values.sourceRules) : [],
      defaultRoleIds: canEditDefaultRoles ? roleIds : [],
      defaultRoles: canEditDefaultRoles
        ? roleIds.map((roleId) => ({ roleId, autoAssignEnabled: true }))
        : [],
      reason: reason.trim(),
      stepUpProof: '',
    });
  };

  const handleSubmit = async () => {
    const values = await form.validateFields();
    try {
      await confirmAndSubmit(values);
    } catch (error) {
      if ((error as { message?: string })?.message !== 'cancelled') {
        throw error;
      }
    }
  };

  return (
    <Drawer
      title={mode === 'create' ? '新增平台' : '编辑平台'}
      size="large"
      open={open}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space>
          <Button type="link" onClick={onClose}>
            取消
          </Button>
          <Button type="primary" loading={confirmLoading} onClick={handleSubmit}>
            保存变更
          </Button>
        </Space>
      }
    >
      <Form form={form} layout="vertical" disabled={confirmLoading}>
        <Space orientation="vertical" className="w-full" size={18}>
          <Card title="基础信息" variant="borderless">
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              {mode === 'create' ? (
                <Form.Item
                  label="平台编码"
                  name="platformCode"
                  rules={[
                    { required: true, message: '请输入平台编码' },
                    {
                      pattern: /^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$/,
                      message: '仅允许 2-64 位小写字母、数字或短横线',
                    },
                  ]}
                >
                  <Input placeholder="authorization-console" />
                </Form.Item>
              ) : null}
              <Form.Item
                label="平台名称"
                name="platformName"
                rules={[
                  { required: true, message: '请输入平台名称' },
                  { max: 100, message: '平台名称不能超过 100 个字符' },
                ]}
              >
                <Input placeholder="Seven 管理控制台" />
              </Form.Item>
              <Form.Item label="平台类型" name="platformType" rules={[{ required: true, message: '请选择平台类型' }]}>
                <Select options={PLATFORM_TYPE_OPTIONS} />
              </Form.Item>
              <Form.Item
                label="默认跳转地址"
                name="defaultRedirectUrl"
                rules={[{ type: 'url', warningOnly: true, message: '建议填写完整 URL' }]}
              >
                <Input placeholder="http://127.0.0.1:5291/" />
              </Form.Item>
              <Form.Item label="平台状态策略">
                <Space size="large">
	                  <Form.Item name="allowAutoRegister" valuePropName="checked" noStyle>
	                    <Switch
	                      checkedChildren="外部登录自动注册"
	                      unCheckedChildren="禁止外部自动注册"
	                      onChange={(checked) => {
	                        if (!checked && form.getFieldValue('allowFormRegister') !== true) {
	                          form.setFieldsValue({
	                            defaultOrgId: undefined,
	                            defaultDeptId: undefined,
                            defaultPostIds: [],
                            defaultRoleIds: [],
                          });
                        }
	                      }}
	                    />
	                  </Form.Item>
	                  <Form.Item name="allowFormRegister" valuePropName="checked" noStyle>
	                    <Switch
	                      checkedChildren="允许表单注册"
	                      unCheckedChildren="禁止表单注册"
	                      onChange={(checked) => {
	                        if (!checked && form.getFieldValue('allowAutoRegister') !== true) {
	                          form.setFieldsValue({
	                            defaultOrgId: undefined,
	                            defaultDeptId: undefined,
	                            defaultPostIds: [],
	                            defaultRoleIds: [],
	                          });
	                        }
	                      }}
	                    />
	                  </Form.Item>
	                  <Form.Item name="isDefault" valuePropName="checked" noStyle>
	                    <Switch checkedChildren="默认平台" unCheckedChildren="非默认平台" />
	                  </Form.Item>
                </Space>
              </Form.Item>
            </div>
            <Form.Item label="描述" name="description">
              <Input.TextArea rows={3} placeholder="说明该平台面向的后台或业务系统" />
            </Form.Item>
          </Card>

          <Card
            title="登录方式"
            extra={<Text type="secondary">OAuth 类型会自动联动已配置 Provider</Text>}
            variant="borderless"
          >
            {!canEditLoginMethods ? (
              <Alert type="warning" showIcon message="当前账号没有登录方式配置权限。" />
            ) : null}
            <Form.List name="loginMethods">
              {(fields, { add, remove }) => (
                <Space orientation="vertical" className="w-full" size="middle">
                  {fields.map((field) => {
                    const { key, ...restField } = field;
                    const method: Partial<PlatformAdminLoginMethod> =
                      watchedValues.loginMethods?.[field.name] || {};
                    const methodType = String(method.methodType || 'PASSWORD').toUpperCase();
                    const selectedProvider = providerMap.get(method.providerCode || '');
                    return (
                      <Card
                        key={key}
                        size="small"
                        title={`登录方式 ${field.name + 1}`}
                        extra={
                          <Button
                            danger
                            type="text"
                            icon={<DeleteOutlined />}
                            onClick={() => remove(field.name)}
                          />
                        }
                      >
                        <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
                          <Form.Item
                            {...restField}
                            label="类型"
                            name={[field.name, 'methodType']}
                            rules={[{ required: true, message: '请选择类型' }]}
                          >
                            <Select
                              options={LOGIN_METHOD_OPTIONS}
                              onChange={(value) => {
                                const nextMethods = [...(form.getFieldValue('loginMethods') || [])];
                                const current = nextMethods[field.name] || {};
                                nextMethods[field.name] = {
                                  ...current,
                                  methodType: value,
                                  providerCode: value === 'EXTERNAL_OAUTH' ? current.providerCode : undefined,
                                  displayName:
                                    value === 'PASSWORD'
                                      ? '账号密码'
                                      : value === 'PASSKEY'
                                        ? 'Passkey'
                                        : current.displayName || '外部登录',
                                };
                                form.setFieldValue('loginMethods', nextMethods);
                              }}
                            />
                          </Form.Item>
                          <Form.Item
                            {...restField}
                            label="显示名"
                            name={[field.name, 'displayName']}
                            rules={[{ required: true, message: '请输入显示名' }]}
                          >
                            <Input placeholder="账号密码" />
                          </Form.Item>
                          <Form.Item {...restField} label="排序" name={[field.name, 'sortOrder']}>
                            <InputNumber className="w-full" min={0} max={9999} />
                          </Form.Item>
                          <Form.Item label="开关">
                            <Space>
                              <Form.Item
                                {...restField}
                                name={[field.name, 'displayEnabled']}
                                valuePropName="checked"
                                noStyle
                              >
                                <Switch checkedChildren="展示" unCheckedChildren="隐藏" />
                              </Form.Item>
                              <Form.Item
                                {...restField}
                                name={[field.name, 'loginEnabled']}
                                valuePropName="checked"
                                noStyle
                              >
                                <Switch checkedChildren="可登录" unCheckedChildren="禁用" />
                              </Form.Item>
                            </Space>
                          </Form.Item>
                        </div>
                        {methodType === 'EXTERNAL_OAUTH' ? (
                          <Space orientation="vertical" className="w-full" size="middle">
                            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                              <Form.Item
                                {...restField}
                                label="OAuth Provider"
                                name={[field.name, 'providerCode']}
                                rules={[{ required: true, message: '请选择 OAuth Provider' }]}
                              >
                                <Select
                                  showSearch
                                  loading={loadingProviders}
                                  placeholder="选择已配置的 GitHub / Google Provider"
                                  options={providerOptions}
                                  optionFilterProp="label"
                                  onChange={(providerCode) => {
                                    const provider = providerMap.get(providerCode);
                                    const nextMethods = [...(form.getFieldValue('loginMethods') || [])];
                                    nextMethods[field.name] = {
                                      ...(nextMethods[field.name] || {}),
                                      methodType: 'EXTERNAL_OAUTH',
                                      providerCode,
                                      displayName: provider?.displayName || provider?.providerName || providerCode,
                                      icon: provider?.icon || providerCode,
                                      displayEnabled: provider?.displayEnabled ?? true,
                                      loginEnabled: provider?.loginEnabled ?? true,
                                      enabled: provider?.loginEnabled ?? true,
                                    };
                                    form.setFieldValue('loginMethods', nextMethods);
                                  }}
                                />
                              </Form.Item>
                              <Form.Item {...restField} label="图标" name={[field.name, 'icon']}>
                                <Input placeholder="github / google" />
                              </Form.Item>
                            </div>
                            <ProviderHint provider={selectedProvider} />
                          </Space>
                        ) : null}
                      </Card>
                    );
                  })}
                  <Button
                    type="dashed"
                    block
                    icon={<PlusOutlined />}
                    disabled={!canEditLoginMethods}
                    onClick={() =>
                      add({
                        methodType: 'PASSWORD',
                        displayName: '账号密码',
                        sortOrder: fields.length,
                        displayEnabled: true,
                        loginEnabled: true,
                        enabled: true,
                      })
                    }
                  >
                    添加登录方式
                  </Button>
                </Space>
              )}
            </Form.List>
          </Card>

          <Card title="来源规则" variant="borderless">
            {!canEditSourceRules ? (
              <Alert type="warning" showIcon message="当前账号没有来源规则配置权限。" />
            ) : null}
            <Form.List name="sourceRules">
              {(fields, { add, remove }) => (
                <Space orientation="vertical" className="w-full" size="middle">
                  {fields.map((field) => {
                    const { key, ...restField } = field;
                    return (
                      <div key={key} className="grid grid-cols-1 gap-4 lg:grid-cols-[1.2fr_2fr_120px_120px_48px]">
                        <Form.Item
                          {...restField}
                          label="来源类型"
                          name={[field.name, 'matchType']}
                          rules={[{ required: true, message: '请选择来源类型' }]}
                        >
                          <Select options={SOURCE_RULE_OPTIONS} />
                        </Form.Item>
                        <Form.Item
                          {...restField}
                          label="匹配值"
                          name={[field.name, 'matchValue']}
                          rules={[{ required: true, message: '请输入匹配值' }]}
                        >
                          <Input placeholder="authorization-console / 127.0.0.1:5291" />
                        </Form.Item>
                        <Form.Item {...restField} label="优先级" name={[field.name, 'priority']}>
                          <InputNumber className="w-full" min={0} max={9999} />
                        </Form.Item>
                        <Form.Item {...restField} label="状态" name={[field.name, 'status']}>
                          <Select
                            options={[
                              { label: '启用', value: 0 },
                              { label: '停用', value: 1 },
                            ]}
                          />
                        </Form.Item>
                        <Form.Item label=" ">
                          <Button danger type="text" icon={<DeleteOutlined />} onClick={() => remove(field.name)} />
                        </Form.Item>
                      </div>
                    );
                  })}
                  <Button
                    type="dashed"
                    block
                    icon={<PlusOutlined />}
                    disabled={!canEditSourceRules}
                    onClick={() =>
                      add({
                        matchType: 'CLIENT_ID',
                        matchValue: '',
                        priority: (fields.length + 1) * 10,
                        status: 0,
                      })
                    }
                  >
                    添加来源规则
                  </Button>
                </Space>
              )}
            </Form.List>
          </Card>

	          {registrationPolicyEnabled ? (
	            <Card title="默认注册策略" variant="borderless">
              {!canEditDefaultRoles ? (
                <Alert type="warning" showIcon message="当前账号没有默认角色配置权限。" />
              ) : null}
	              <Alert
	                type="info"
	                showIcon
	                message="开启外部登录自动注册或表单注册后，默认组织、部门、岗位和角色会在首次创建用户时生效。"
	                style={{ marginBottom: 16 }}
	              />
              <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                <Form.Item label="默认组织" name="defaultOrgId">
                  <Select
                    allowClear
                    showSearch
                    loading={loadingOrgs}
                    placeholder="请选择首次注册归属组织"
                    options={orgOptions}
                    optionFilterProp="label"
                    onChange={() => {
                      form.setFieldsValue({ defaultDeptId: undefined, defaultPostIds: [] });
                    }}
                  />
                </Form.Item>
                <Form.Item label="默认部门" name="defaultDeptId">
                  <Select
                    allowClear
                    showSearch
                    loading={loadingDepts}
                    placeholder="请选择首次注册归属部门"
                    options={filteredDeptOptions}
                    optionFilterProp="label"
                    onChange={() => {
                      form.setFieldValue('defaultPostIds', []);
                    }}
                  />
                </Form.Item>
                <Form.Item label="默认岗位" name="defaultPostIds">
                  <Select
                    mode="multiple"
                    allowClear
                    showSearch
                    loading={loadingPosts}
                    placeholder="请选择首次注册授予的岗位"
                    options={filteredPostOptions}
                    optionFilterProp="label"
                  />
                </Form.Item>
                <Form.Item
                  label="默认角色"
                  name="defaultRoleIds"
                  rules={[
                    {
	                      validator: (_, value) => {
	                        if (!registrationPolicyEnabled || !canEditDefaultRoles || normalizeRoleIds(value).length > 0) {
	                          return Promise.resolve();
                        }
                        return Promise.reject(new Error('请选择注册后授予的默认角色'));
                      },
                    },
                  ]}
                >
                  <Select
                    mode="multiple"
                    allowClear
                    showSearch
                    loading={loadingRoles}
                    placeholder="请选择首次注册授予的角色"
                    options={roleOptions}
                    optionFilterProp="label"
                    disabled={!canEditDefaultRoles}
                  />
                </Form.Item>
              </div>
            </Card>
          ) : null}

          <Card title="登录页品牌" variant="borderless">
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              <Form.Item label="品牌标题" name="brandTitle">
                <Input placeholder="Seven" />
              </Form.Item>
              <Form.Item label="副标题" name="brandSubtitle">
                <Input placeholder="统一身份认证系统" />
              </Form.Item>
              <Alert
                type="info"
                showIcon
                message="登录 Logo 由“系统配置”中的受控图片资产管理"
                description="这里不接受 URL、blob/data 地址或文件路径。上传、替换和清除请使用前端元数据分组的 loginLogo 配置项。"
              />
              <Form.Item label="主题" name="brandTheme">
                <Select options={BRAND_THEME_OPTIONS} />
              </Form.Item>
              <Form.Item label="支持链接" name="supportUrl">
                <Input placeholder="https://example.com/help" />
              </Form.Item>
              <Form.Item label="登录页提示" name="loginPrompt">
                <Input placeholder="如需访问权限，请联系平台管理员" />
              </Form.Item>
            </div>
            <Collapse
              ghost
              items={[
                {
                  key: 'json-preview',
                  label: '最终配置预览',
                  children: (
                    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
                      <Input.TextArea
                        readOnly
                        value={previewBrandJson || '{}'}
                        rows={8}
                        styles={{ textarea: { fontFamily: 'monospace' } }}
                      />
                      <Input.TextArea
                        readOnly
                        value={previewSettingsJson || '{}'}
                        rows={8}
                        styles={{ textarea: { fontFamily: 'monospace' } }}
                      />
                    </div>
                  ),
                },
              ]}
            />
          </Card>
        </Space>
      </Form>
    </Drawer>
  );
}
