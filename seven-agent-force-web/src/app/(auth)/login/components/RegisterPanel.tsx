"use client";

import {
  ArrowLeftOutlined,
  ClockCircleOutlined,
  MailOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  UserOutlined,
  LockOutlined,
} from '@ant-design/icons';
import { Button, Form, Input, message } from 'antd';
import React, { useEffect, useState } from 'react';

import type { LoginCaptcha, LoginRegisterEmailCodeResult } from '@/lib/http/types';

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

interface RegisterPanelProps {
  initialAccount?: string;
  loadingState: boolean;
  submitting: boolean;
  captcha?: LoginCaptcha | null;
  sessionResetKey?: number;
  onBack: () => void;
  onAccountBlur: (userAccount: string) => void | Promise<void>;
  onRefreshCaptcha: (userAccount: string) => void | Promise<void>;
  onSubmit: (values: {
    userAccount: string;
    userName: string;
    userEmail: string;
    password: string;
    confirmPassword: string;
    emailCode: string;
  }) => void | Promise<void>;
  onSendEmailCode: (values: {
    userAccount: string;
    userEmail: string;
    captchaCode: string;
  }) => Promise<LoginRegisterEmailCodeResult | null>;
}

export default function RegisterPanel(props: RegisterPanelProps) {
  const {
    initialAccount,
    loadingState,
    submitting,
    captcha,
    sessionResetKey,
    onBack,
    onAccountBlur,
    onRefreshCaptcha,
    onSubmit,
    onSendEmailCode,
  } = props;
  const [form] = Form.useForm();
  const [sendingEmailCode, setSendingEmailCode] = useState(false);
  const [emailCountdown, setEmailCountdown] = useState(0);
  const [emailCodeSent, setEmailCodeSent] = useState(false);

  const focusFirstErrorField = (error: unknown) => {
    const errorFields = (error as { errorFields?: Array<{ name?: Array<string | number> }> })
      ?.errorFields;
    const firstFieldName = errorFields?.[0]?.name?.[0];
    if (typeof firstFieldName === 'string') {
      form.getFieldInstance(firstFieldName)?.focus?.();
    }
  };

  useEffect(() => {
    form.setFieldsValue({ userAccount: initialAccount });
  }, [form, initialAccount]);

  useEffect(() => {
    form.setFieldValue('captchaCode', undefined);
  }, [captcha?.challengeIdentifier, captcha?.stepIdentifier, form]);

  useEffect(() => {
    if (!captcha?.challengeIdentifier) {
      return;
    }
    setEmailCodeSent(false);
    setEmailCountdown(0);
    form.setFieldValue('emailCode', undefined);
  }, [captcha?.challengeIdentifier, form]);

  useEffect(() => {
    setEmailCodeSent(false);
    setEmailCountdown(0);
    form.setFieldValue('captchaCode', undefined);
    form.setFieldValue('emailCode', undefined);
  }, [form, sessionResetKey]);

  useEffect(() => {
    if (emailCountdown <= 0) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      setEmailCountdown((value) => Math.max(0, value - 1));
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [emailCountdown]);

  return (
    <Form
      form={form}
      layout="vertical"
      requiredMark={false}
      className="login-password-form"
      onFinish={(values) => {
        void onSubmit({
          userAccount: values.userAccount?.trim() ?? '',
          userName: values.userName?.trim() ?? '',
          userEmail: values.userEmail?.trim() ?? '',
          password: values.password ?? '',
          confirmPassword: values.confirmPassword ?? '',
          emailCode: values.emailCode?.trim() ?? '',
        });
      }}
    >
      <div className="login-back-row">
        <Button
          type="text"
          className="login-back-button"
          icon={<ArrowLeftOutlined />}
          onClick={onBack}
        >
          返回登录
        </Button>
      </div>

      <div className="login-register-grid">
        <Form.Item
          name="userAccount"
          className="login-no-margin"
          rules={[
            { required: true, message: '请输入账号' },
            { pattern: /^[a-zA-Z][a-zA-Z0-9]{4,15}$/, message: '账号需字母开头，5-16 位字母数字' },
          ]}
        >
          <Input
            size="large"
            className="login-affix-input"
            prefix={<UserOutlined />}
            placeholder="登录账号"
            autoComplete="username"
            autoCapitalize="none"
            autoCorrect="off"
            onChange={() => {
              if (emailCodeSent) {
                setEmailCodeSent(false);
                setEmailCountdown(0);
                form.setFieldValue('emailCode', undefined);
              }
            }}
            onBlur={(event) => {
              void onAccountBlur(event.target.value);
            }}
          />
        </Form.Item>

        <Form.Item
          name="userName"
          className="login-no-margin"
          rules={[
            { required: true, message: '请输入昵称' },
            { max: 30, message: '昵称不能超过 30 个字符' },
          ]}
        >
          <Input
            size="large"
            className="login-affix-input"
            prefix={<UserOutlined />}
            placeholder="显示昵称"
            autoComplete="nickname"
          />
        </Form.Item>

        <div className="login-email-send-row login-register-inline-row">
          <Form.Item
            name="userEmail"
            className="login-no-margin login-email-send-field"
            rules={[
              { required: true, message: '请输入邮箱' },
              { type: 'email', message: '请输入有效邮箱' },
            ]}
          >
            <Input
              size="large"
              className="login-affix-input"
              prefix={<MailOutlined />}
              placeholder="用于接收验证码的邮箱"
              autoComplete="email"
              onChange={() => {
                if (emailCodeSent) {
                  setEmailCodeSent(false);
                  setEmailCountdown(0);
                  form.setFieldValue('emailCode', undefined);
                }
              }}
            />
          </Form.Item>
          {!emailCodeSent ? (
            <Button
              type="default"
              className="login-email-code-button"
              loading={sendingEmailCode}
              onClick={() => {
                void (async () => {
                  const rawValues = form.getFieldsValue(['userAccount', 'userEmail', 'captchaCode']);
                  if (!String(rawValues.userAccount ?? '').trim()) {
                    message.warning('请先输入登录账号');
                    return;
                  } else if (!String(rawValues.userEmail ?? '').trim()) {
                    message.warning('请先输入邮箱');
                    return;
                  } else if (!String(rawValues.captchaCode ?? '').trim()) {
                    message.warning('请输入图形验证码');
                    return;
                  }
                  const values = await form.validateFields(['userAccount', 'userEmail', 'captchaCode']);
                  setSendingEmailCode(true);
                  try {
                    const result = await onSendEmailCode({
                      userAccount: values.userAccount?.trim() ?? '',
                      userEmail: values.userEmail?.trim() ?? '',
                      captchaCode: values.captchaCode?.trim() ?? '',
                    });
                    if (result?.sent) {
                      setEmailCodeSent(true);
                      setEmailCountdown(result.cooldownSeconds || 60);
                    }
                  } finally {
                    setSendingEmailCode(false);
                  }
                })().catch((error) => {
                  setSendingEmailCode(false);
                  focusFirstErrorField(error);
                  if (String(form.getFieldValue('captchaCode') ?? '').trim()) {
                    message.warning('请检查注册信息');
                  }
                });
              }}
            >
              发送验证码
            </Button>
          ) : null}
        </div>

        {!emailCodeSent ? (
          <div className="login-captcha-row login-register-inline-row">
            <Form.Item
              name="captchaCode"
              className="login-no-margin login-captcha-field"
              rules={[{ required: true, message: '请输入图形验证码' }]}
            >
              <Input
                size="large"
                className="login-affix-input"
                prefix={<SafetyCertificateOutlined />}
                placeholder="图形验证码"
                autoComplete="off"
              />
            </Form.Item>
            <Button
              type="default"
              className="login-captcha-button"
              loading={loadingState}
              onClick={() => {
                void (async () => {
                  const values = await form.validateFields(['userAccount']);
                  await onRefreshCaptcha(values.userAccount?.trim() ?? '');
                })().catch(() => {
                  // 账号校验失败时保持由表单自身提示。
                });
              }}
              title="点击刷新验证码"
            >
              {captcha?.imageBase64 ? (
                <img
                  key={`${captcha.challengeIdentifier || ''}:${captcha.stepIdentifier || ''}`}
                  src={normalizeCaptchaImage(captcha.imageBase64)}
                  alt="验证码"
                  className="login-captcha-image"
                />
              ) : (
                <span className="login-captcha-fallback">刷新</span>
              )}
              <span className="login-captcha-overlay">
                <ReloadOutlined />
              </span>
            </Button>
          </div>
        ) : (
          <div className="login-email-verify-row login-register-inline-row">
            <Form.Item
              name="emailCode"
              className="login-no-margin login-email-verify-field"
              rules={[
                { required: true, message: '请输入邮箱验证码' },
                { len: 6, message: '请输入 6 位邮箱验证码' },
              ]}
            >
              <Input
                size="large"
                className="login-affix-input"
                prefix={<SafetyCertificateOutlined />}
                placeholder="邮箱验证码"
                autoComplete="one-time-code"
              />
            </Form.Item>
            <Button
              type="default"
              className="login-email-code-button login-email-code-button-wide"
              disabled={emailCountdown > 0}
              icon={emailCountdown > 0 ? <ClockCircleOutlined /> : <ReloadOutlined />}
              onClick={() => {
                setEmailCodeSent(false);
                setEmailCountdown(0);
                form.setFieldValue('emailCode', undefined);
                void (async () => {
                  const values = await form.validateFields(['userAccount']);
                  await onRefreshCaptcha(values.userAccount?.trim() ?? '');
                })().catch(() => {
                  // 账号校验失败时保持由表单自身提示。
                });
              }}
            >
              {emailCountdown > 0 ? `${emailCountdown}s 后重发` : '重新发送'}
            </Button>
          </div>
        )}

        <Form.Item
          name="password"
          className="login-no-margin"
          rules={[
            { required: true, message: '请输入密码' },
            { pattern: /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).{8,20}$/, message: '密码需 8-20 位，包含大小写字母和数字' },
          ]}
        >
          <Input.Password
            size="large"
            className="login-affix-input"
            prefix={<LockOutlined />}
            placeholder="登录密码"
            autoComplete="new-password"
          />
        </Form.Item>

        <Form.Item
          name="confirmPassword"
          className="login-no-margin"
          dependencies={['password']}
          rules={[
            { required: true, message: '请再次输入密码' },
            ({ getFieldValue }) => ({
              validator(_, value) {
                if (!value || getFieldValue('password') === value) {
                  return Promise.resolve();
                }
                return Promise.reject(new Error('两次输入的密码不一致'));
              },
            }),
          ]}
        >
          <Input.Password
            size="large"
            className="login-affix-input"
            prefix={<LockOutlined />}
            placeholder="确认密码"
            autoComplete="new-password"
          />
        </Form.Item>
      </div>

      <Button
        htmlType="submit"
        type="primary"
        loading={submitting}
        className="login-primary-button"
      >
        创建账号
      </Button>
    </Form>
  );
}
