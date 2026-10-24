"use client";

import {
  GlobalOutlined,
  GithubOutlined,
  KeyOutlined,
  LoadingOutlined,
  LockOutlined,
  MoreOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
} from '@ant-design/icons';
import { Button, Checkbox, Dropdown, Form, Input, Typography } from 'antd';
import type { MenuProps } from 'antd';
import React, { useEffect } from 'react';

import type { LoginCaptcha, PlatformLoginMethod } from '@/lib/http/types';

const { Text } = Typography;

function normalizeCaptchaImage(source?: string | null) {
  if (!source) {
    return '';
  }
  const normalizedSource = source.trim();
  if (
    normalizedSource.startsWith('data:')
    || normalizedSource.startsWith('http://')
    || normalizedSource.startsWith('https://')
    || normalizedSource.startsWith('/')
  ) {
    return normalizedSource;
  }
  if (normalizedSource.startsWith('<svg')) {
    return `data:image/svg+xml;utf8,${encodeURIComponent(normalizedSource)}`;
  }
  if (normalizedSource.startsWith('PHN2Zy') || normalizedSource.startsWith('PD94bWwg')) {
    return `data:image/svg+xml;base64,${normalizedSource}`;
  }
  return `data:image/png;base64,${normalizedSource}`;
}

interface PasswordPanelProps {
  initialAccount?: string;
  checkingPasswordState: boolean;
  refreshingCaptcha: boolean;
  submittingPassword: boolean;
  startingPasskey: boolean;
  rememberSession: boolean;
  captchaRequired: boolean;
  captcha?: LoginCaptcha | null;
  passwordLoginEnabled: boolean;
  passkeyLoginEnabled: boolean;
  externalLoginMethods: PlatformLoginMethod[];
  loadingLoginOptions: boolean;
  registerEnabled: boolean;
  onRememberSessionChange: (checked: boolean) => void;
  onAccountBlur: (userAccount: string) => void | Promise<void>;
  onSubmit: (values: {
    userAccount: string;
    password: string;
    captchaCode?: string;
  }) => void | Promise<void>;
  onPasskeyLogin: (userAccount: string) => void | Promise<void>;
  onExternalLogin: (method: PlatformLoginMethod) => void | Promise<void>;
  onRefreshCaptcha: () => void | Promise<void>;
  onRegisterClick: (userAccount?: string) => void;
}

function renderExternalLoginIcon(providerCode: string) {
  const normalizedProviderCode = providerCode.trim().toLowerCase();
  if (normalizedProviderCode === 'github') {
    return <GithubOutlined />;
  }
  return <GlobalOutlined />;
}

interface PasswordlessAction {
  key: string;
  label: string;
  icon: React.ReactNode;
  className?: string;
  loading?: boolean;
  disabled?: boolean;
  onClick: () => void;
}

export default function PasswordPanel(props: PasswordPanelProps) {
  const {
    initialAccount,
    checkingPasswordState,
    refreshingCaptcha,
    submittingPassword,
    startingPasskey,
    rememberSession,
    captchaRequired,
    captcha,
    passwordLoginEnabled,
    passkeyLoginEnabled,
    externalLoginMethods,
    loadingLoginOptions,
    registerEnabled,
    onRememberSessionChange,
    onAccountBlur,
    onSubmit,
    onPasskeyLogin,
    onExternalLogin,
    onRefreshCaptcha,
    onRegisterClick,
  } = props;
  const [form] = Form.useForm();

  const passkeyAction: PasswordlessAction | null = passkeyLoginEnabled
    ? {
        key: 'passkey',
        label: '通行密钥',
        icon: <KeyOutlined />,
        loading: startingPasskey,
        onClick: () => {
          void (async () => {
            const values = await form.validateFields(['userAccount']);
            await onPasskeyLogin(values.userAccount?.trim() ?? '');
          })().catch(() => {
            // 账号校验失败时保持由表单自身提示。
          });
        },
      }
    : null;

  const loadingAction: PasswordlessAction | null = loadingLoginOptions && externalLoginMethods.length === 0
    ? {
        key: 'loading',
        label: '正在加载登录方式',
        icon: <LoadingOutlined />,
        className: 'login-secondary-button-muted',
        loading: true,
        disabled: true,
        onClick: () => {},
      }
    : null;

  const passwordlessActions = [
    passkeyAction,
    loadingAction,
    ...externalLoginMethods.map((method): PasswordlessAction => ({
      key: `${method.providerCode}-${method.sortOrder}`,
      label: method.displayName,
      icon: renderExternalLoginIcon(method.providerCode),
      className: 'login-secondary-button-muted',
      onClick: () => {
        void onExternalLogin(method);
      },
    })),
  ].filter((action): action is PasswordlessAction => Boolean(action));

  const visiblePasswordlessActions = passwordlessActions.length > 4
    ? passwordlessActions.slice(0, 3)
    : passwordlessActions;
  const overflowPasswordlessActions = passwordlessActions.length > 4
    ? passwordlessActions.slice(3)
    : [];
  const visiblePasswordlessCount = visiblePasswordlessActions.length
    + (overflowPasswordlessActions.length > 0 ? 1 : 0);
  const compactPasswordless = visiblePasswordlessCount >= 3;
  const passwordlessGridClassName = [
    'login-secondary-grid',
    `login-secondary-grid-count-${Math.min(visiblePasswordlessCount, 4)}`,
    compactPasswordless ? 'login-secondary-grid-compact' : '',
  ].filter(Boolean).join(' ');
  const overflowMenuItems: MenuProps['items'] = overflowPasswordlessActions.map((action) => ({
    key: action.key,
    icon: action.icon,
    label: action.label,
    disabled: action.disabled,
    onClick: action.onClick,
  }));

  useEffect(() => {
    form.setFieldsValue({
      userAccount: initialAccount,
    });
  }, [form, initialAccount]);

  useEffect(() => {
    if (!captchaRequired) {
      form.setFieldValue('captchaCode', undefined);
    }
  }, [captchaRequired, form]);

  useEffect(() => {
    form.setFieldValue('captchaCode', undefined);
  }, [captcha?.challengeIdentifier, captcha?.stepIdentifier, captcha?.imageBase64, form]);

  return (
    <Form
      form={form}
      layout="vertical"
      requiredMark={false}
      onFinish={(values) => {
        void onSubmit({
          userAccount: values.userAccount?.trim() ?? '',
          password: values.password ?? '',
          captchaCode: values.captchaCode?.trim() || undefined,
        });
      }}
      className="login-password-form"
    >
      <Form.Item
        name="userAccount"
        className="login-no-margin"
        rules={[
          { required: true, message: '请输入账号' },
          { min: 3, message: '账号长度至少 3 位' },
        ]}
      >
        <Input
          size="large"
          className="login-affix-input"
          prefix={<UserOutlined />}
          suffix={checkingPasswordState ? <LoadingOutlined /> : null}
          placeholder="管理员账号"
          autoComplete="username"
          autoCapitalize="none"
          autoCorrect="off"
          onBlur={(event) => {
            void onAccountBlur(event.target.value);
          }}
        />
      </Form.Item>

      {passwordLoginEnabled ? (
        <Form.Item
          name="password"
          className="login-no-margin"
          rules={[
            { required: true, message: '请输入密码' },
            { min: 1, message: '请输入密码' },
          ]}
        >
          <Input.Password
            size="large"
            className="login-affix-input"
            prefix={<LockOutlined />}
            placeholder="登录密码"
            autoComplete="current-password"
          />
        </Form.Item>
      ) : null}

      {passwordLoginEnabled && captchaRequired ? (
        <div className="login-captcha-row">
          <Form.Item
            name="captchaCode"
            className="login-no-margin login-captcha-field"
            rules={[{ required: true, message: '请输入图形验证码' }]}
          >
            <Input
              size="large"
              className="login-affix-input"
              prefix={<SafetyCertificateOutlined />}
              placeholder="请输入验证码"
              autoComplete="off"
            />
          </Form.Item>
          <Button
            type="default"
            className={`login-captcha-button ${refreshingCaptcha ? 'login-captcha-button-refreshing' : ''}`}
            disabled={refreshingCaptcha}
            onClick={() => {
              form.setFieldValue('captchaCode', undefined);
              void onRefreshCaptcha();
            }}
            title="点击刷新验证码"
            aria-busy={refreshingCaptcha}
          >
            {captcha?.imageBase64 ? (
              <img
                key={`${captcha.challengeIdentifier || ''}:${captcha.stepIdentifier || ''}`}
                src={normalizeCaptchaImage(captcha.imageBase64)}
                alt="验证码"
                className="login-captcha-image"
              />
            ) : (
              <span className="login-captcha-fallback">刷新验证码</span>
            )}
            <span className="login-captcha-overlay">
              <ReloadOutlined spin={refreshingCaptcha} />
            </span>
          </Button>
        </div>
      ) : null}

      {passwordLoginEnabled ? (
        <div className="login-form-options">
          <Checkbox
            checked={rememberSession}
            className="login-remember"
            onChange={(event) => {
              onRememberSessionChange(event.target.checked);
            }}
          >
            保持登录 (SSO)
          </Checkbox>
        </div>
      ) : null}

      {passwordLoginEnabled ? (
        <Button
          htmlType="submit"
          type="primary"
          loading={submittingPassword}
          className="login-primary-button"
        >
          安全登录
        </Button>
      ) : null}

      {registerEnabled ? (
        <div className="login-register-prompt">
          <Text className="login-register-prompt-text">没有账号？</Text>
          <Button
            type="link"
            className="login-register-link"
            onClick={() => {
              const values = form.getFieldsValue(['userAccount']);
              onRegisterClick(values.userAccount?.trim());
            }}
          >
            创建账号
          </Button>
        </div>
      ) : null}

      {passwordlessActions.length > 0 ? (
        <div className="login-secondary-section">
          <div className={passwordlessGridClassName}>
            {visiblePasswordlessActions.map((action) => (
              <Button
                key={action.key}
                size="large"
                className={`login-secondary-button ${action.className ?? ''}`}
                icon={action.icon}
                loading={action.loading}
                disabled={action.disabled}
                aria-label={compactPasswordless ? action.label : undefined}
                title={compactPasswordless ? action.label : undefined}
                onClick={action.onClick}
              >
                {compactPasswordless ? null : action.label}
              </Button>
            ))}
            {overflowPasswordlessActions.length > 0 ? (
              <Dropdown menu={{ items: overflowMenuItems }} trigger={['click']} placement="bottomRight">
                <Button
                  size="large"
                  className="login-secondary-button login-secondary-button-muted"
                  icon={<MoreOutlined />}
                  aria-label="更多登录方式"
                  title="更多登录方式"
                />
              </Dropdown>
            ) : null}
          </div>
        </div>
      ) : null}
    </Form>
  );
}
