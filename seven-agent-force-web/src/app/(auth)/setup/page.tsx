"use client";

import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Card, ConfigProvider, Descriptions, Form, Input, Spin, Typography, message } from 'antd';
import { LockOutlined, SafetyCertificateOutlined, UserOutlined } from '@ant-design/icons';
import { css } from '@emotion/css';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';

import { createSetupOwnerApi, getSetupStatusApi } from '@/api/setupController';
import type { SetupOwnerRequest, SetupOwnerResult, SetupStatus } from '@/lib/http/types';
import { buildLoginRedirectUrl, resolveSafePostLoginTarget } from '@/lib/auth/navigation';
import { CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { useAuthStore } from '@/store/auth';
import { fetchCurrentUser, normalizeCurrentUser } from '@/services/auth';

const { Paragraph, Text, Title } = Typography;

const PASSWORD_POLICY_MESSAGE = '密码需为 8-20 位，并同时包含大写字母、小写字母和数字';
const PASSWORD_POLICY_PATTERN = /^(?=.*[a-z])(?=.*[A-Z])(?=.*\d).{8,20}$/;

function readSetupRedirectTarget() {
  if (typeof window === 'undefined') {
    return '/';
  }
  const redirect = new URLSearchParams(window.location.search).get('redirect');
  return resolveSafePostLoginTarget(redirect, '/');
}

function toLoginUser(owner: SetupOwnerResult) {
  return normalizeCurrentUser({
    id: owner.id,
    username: owner.username,
    nickname: owner.nickname,
    userAvatar: owner.userAvatar,
    permissions: owner.permissions,
    roleCodes: owner.roleCodes,
  });
}

export default function SetupPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const applyAccessToken = useAuthStore((state) => state.applyAccessToken);
  const setUser = useAuthStore((state) => state.setUser);
  const accessToken = useAuthStore((state) => state.accessToken);
  const currentUser = useAuthStore((state) => state.user);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [statusError, setStatusError] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const pageClassName = css({
    minHeight: '100vh',
    background: 'linear-gradient(180deg, #eef3f8 0%, #f7f9fc 100%)',
  });

  const shellClassName = css({
    minHeight: '100vh',
    display: 'flex',
    justifyContent: 'center',
    alignItems: 'center',
    padding: '32px 48px',
    '@media (max-width: 900px)': {
      justifyContent: 'center',
      padding: '20px 16px',
    },
  });

  const panelWrapperClassName = css({
    width: 'min(560px, 100%)',
  });

  const setupCardClassName = css({
    borderRadius: 20,
    border: '1px solid #dbe4ee',
    background: '#ffffff',
    boxShadow: '0 20px 48px rgba(15, 23, 42, 0.08)',
  });

  const redirectTarget = useMemo(() => readSetupRedirectTarget(), []);

  const redirectAfterSetup = useCallback((target: string) => {
    if (target.startsWith('http://') || target.startsWith('https://')) {
      window.location.assign(target);
      return;
    }
    navigate(target, { replace: true });
  }, [navigate]);

  const loadSetupStatus = useCallback(async () => {
    setStatusLoading(true);
    setStatusError('');
    try {
      const response = await getSetupStatusApi();
      const nextStatus = response.data ?? null;
      setSetupStatus(nextStatus);
      if (nextStatus?.initialized) {
        if (currentUser || accessToken) {
          redirectAfterSetup(redirectTarget);
          return;
        }
        navigate(buildLoginRedirectUrl(redirectTarget), { replace: true });
      }
    } catch (error) {
      setStatusError((error as { message?: string })?.message || '获取初始化状态失败');
    } finally {
      setStatusLoading(false);
    }
  }, [accessToken, currentUser, navigate, redirectAfterSetup, redirectTarget]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadSetupStatus();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadSetupStatus]);

  const handleSubmit = async (values: SetupOwnerRequest) => {
    if (!setupStatus?.setupToken) {
      message.error('初始化校验已失效，正在重新加载状态');
      await loadSetupStatus();
      return;
    }
    setSubmitting(true);
    try {
      const response = await createSetupOwnerApi(values, setupStatus.setupToken);
      const owner = response.data;
      if (!owner?.accessToken) {
        throw new Error('初始化成功，但未返回访问令牌');
      }
      applyAccessToken({
        accessToken: owner.accessToken,
        tokenType: owner.tokenType,
        accessTtlSec: owner.accessTtlSec,
      });
      try {
        const currentUserResponse = await fetchCurrentUser({ skipAuthRedirect: true });
        setUser(currentUserResponse?.data ? normalizeCurrentUser(currentUserResponse.data) : toLoginUser(owner));
      } catch {
        setUser(toLoginUser(owner));
      }
      queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });
      message.success('初始化完成，正在进入系统');
      redirectAfterSetup(redirectTarget);
    } catch (error) {
      const nextMessage = (error as { message?: string })?.message || '初始化失败，请稍后重试';
      message.error(nextMessage);
      const code = (error as { code?: number; payload?: { code?: number } })?.code
        ?? (error as { payload?: { code?: number } })?.payload?.code;
      if (code === 40101 || code === 50001) {
        await loadSetupStatus();
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#1677ff',
          colorInfo: '#1677ff',
          colorBgElevated: '#ffffff',
          colorTextBase: '#1f2937',
          colorBorderSecondary: '#dbe4ee',
          fontFamily: '"PingFang SC", "Microsoft YaHei", sans-serif',
        },
      }}
    >
      <div className={pageClassName}>
        <div className={shellClassName}>
          <div className={panelWrapperClassName}>
            <Card className={setupCardClassName} styles={{ body: { padding: 32 } }}>
              {statusLoading ? (
                <div style={{ minHeight: 480, display: 'grid', placeItems: 'center' }}>
                  <Spin size="large" description="正在检查初始化状态..." />
                </div>
              ) : (
                <div style={{ display: 'grid', gap: 24 }}>
                  <div>
                    <Text style={{ color: '#1677ff', fontWeight: 600 }}>
                      Seven 平台初始化
                    </Text>
                    <Title level={2} style={{ color: '#111827', marginTop: 8, marginBottom: 8 }}>
                      创建第一个超级管理员
                    </Title>
                    <Paragraph style={{ color: '#4b5563', marginBottom: 0 }}>
                      这是空环境的首次引导。初始化完成后，系统会绑定配置的安全根角色，并为当前浏览器写入访问会话。
                    </Paragraph>
                  </div>

                  <Descriptions
                    size="small"
                    column={1}
                    styles={{
                      label: { color: '#6b7280', width: 96 },
                      content: { color: '#111827' },
                    }}
                    items={[
                      { key: 'version', label: '应用版本', children: setupStatus?.appVersion || 'dev' },
                      { key: 'commit', label: '构建提交', children: setupStatus?.appCommit || 'dev' },
                      { key: 'startTime', label: '启动时间', children: setupStatus?.startTime || '-' },
                      { key: 'loginEnabled', label: '登录入口', children: setupStatus?.loginEnabled ? '已启用' : '初始化前关闭' },
                    ]}
                  />

                  <Alert
                    type="info"
                    showIcon
                    icon={<SafetyCertificateOutlined />}
                    title="初始化说明"
                    description="当前只允许创建首个安全根管理员。初始化完成后，setupToken 会立即失效，后续访问会引导到登录页。"
                  />

                  {statusError ? (
                    <Alert type="error" showIcon title="初始化状态获取失败" description={statusError} />
                  ) : null}

                  <Form<SetupOwnerRequest>
                    layout="vertical"
                    onFinish={handleSubmit}
                    initialValues={{
                      username: 'owner',
                      nickname: 'Owner',
                    }}
                    requiredMark={false}
                  >
                    <Form.Item
                      name="username"
                      label={<span style={{ color: '#374151' }}>用户名</span>}
                      rules={[{ required: true, message: '请输入用户名' }]}
                    >
                      <Input prefix={<UserOutlined />} size="large" autoComplete="username" />
                    </Form.Item>
                    <Form.Item
                      name="nickname"
                      label={<span style={{ color: '#374151' }}>昵称</span>}
                      rules={[{ required: true, message: '请输入昵称' }]}
                    >
                      <Input size="large" autoComplete="nickname" />
                    </Form.Item>
                    <Form.Item
                      name="password"
                      label={<span style={{ color: '#374151' }}>密码</span>}
                      extra={PASSWORD_POLICY_MESSAGE}
                      rules={[
                        { required: true, message: '请输入密码' },
                        { pattern: PASSWORD_POLICY_PATTERN, message: PASSWORD_POLICY_MESSAGE },
                      ]}
                    >
                      <Input.Password
                        prefix={<LockOutlined />}
                        size="large"
                        autoComplete="new-password"
                        placeholder="例如 Owner123"
                      />
                    </Form.Item>
                    <Form.Item
                      name="confirmPassword"
                      label={<span style={{ color: '#374151' }}>确认密码</span>}
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
                      <Input.Password prefix={<LockOutlined />} size="large" autoComplete="new-password" />
                    </Form.Item>
                    <Button
                      type="primary"
                      htmlType="submit"
                      size="large"
                      loading={submitting}
                      block
                      style={{ marginTop: 8, height: 48 }}
                    >
                      创建超级管理员并进入系统
                    </Button>
                  </Form>
                </div>
              )}
            </Card>
          </div>
        </div>
      </div>
    </ConfigProvider>
  );
}
