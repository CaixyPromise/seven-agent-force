'use client';

import React, { useState } from 'react';
import {
  Menu,
  Typography,
  message,
  Modal,
  Button,
  List,
  Alert,
} from 'antd';
import {
  UserOutlined,
  SafetyCertificateOutlined,
  FileTextOutlined,
  LogoutOutlined,
} from '@ant-design/icons';
import ProfileSection from './components/ProfileSection';
import SecuritySection from './components/SecuritySection';
import LogSection from './components/LogSection';
import {
  regenerateRecoveryCodes,
  deleteMfaPasskey,
  deleteMfaOtpBinding,
  type MfaBusinessAction,
  type RegenerateRecoveryCodeResponse,
} from '@/api/mfaController';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useGlobalChallengeActions } from '@/components/providers/globalChallengeContext';
import { useLogoutMutation } from '@/hooks/useAuth';
import { useNavigate } from 'react-router-dom';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';

const { Title } = Typography;

function extractErrorMessage(error: unknown, fallback: string) {
  return (error as { message?: string })?.message || fallback;
}

function isUserCancelledChallenge(error: unknown) {
  return isChallengeRetryError(error, 'CHALLENGE_CANCELLED');
}

function AccountSettingsContent() {
  const [activeTab, setActiveTab] = useState('profile');
  const queryClient = useQueryClient();
  const { startChallenge } = useGlobalChallengeActions();
  const logoutMutation = useLogoutMutation();
  const navigate = useNavigate();

  // --- Recovery Codes Logic (Handled at top level due to Modal need) ---
  const [recoveryModalOpen, setRecoveryModalOpen] = useState(false);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);

  const regenerateRecoveryCodesMutation = useMutation({
    mutationFn: () => regenerateRecoveryCodes(),
    onSuccess: (res) => {
      const data = res.data as RegenerateRecoveryCodeResponse;
      if (data && data.recoveryCodes) {
        setRecoveryCodes(data.recoveryCodes);
        setRecoveryModalOpen(true);
      }
    },
    onError: (error: unknown) => {
      if (isUserCancelledChallenge(error)) {
        return;
      }
      message.error(extractErrorMessage(error, '重置失败'));
    },
  });

  const handleStartChallenge = async (action: MfaBusinessAction, context?: Record<string, unknown>) => {
    await startChallenge({ action, extensionContext: context });
  };

  const handleRegenerateRecoveryCodes = () => {
    regenerateRecoveryCodesMutation.mutate();
  };

  const handleDeletePasskey = async (credentialIdentifier: string) => {
    await deleteMfaPasskey(credentialIdentifier);
    message.success('Passkey 已删除');
    await queryClient.invalidateQueries({ queryKey: ['account-settings', 'passkeys'] });
    await queryClient.invalidateQueries({ queryKey: ['account-settings', 'mfa-status'] });
  };

  const handleDeleteOtpBinding = async () => {
    await deleteMfaOtpBinding();
    message.success('OTP 已解绑');
    await queryClient.invalidateQueries({ queryKey: ['account-settings', 'mfa-status'] });
  };

  const handleLogout = async () => {
    await logoutMutation.mutateAsync();
    message.success('已退出登录');
    navigate('/login', { replace: true });
  };

  const menuItems = [
    {
      key: 'profile',
      icon: <UserOutlined />,
      label: '个人资料',
    },
    {
      key: 'security',
      icon: <SafetyCertificateOutlined />,
      label: '账号安全',
    },
    {
      key: 'logs',
      icon: <FileTextOutlined />,
      label: '操作日志',
    },
  ];

  return (
    <div className="min-h-screen bg-slate-50">
      <div className="max-w-7xl mx-auto py-8 px-4 sm:px-6 lg:px-8">
        <div className="mb-6">
          <Title level={2}>账号设置</Title>
          <p className="text-slate-500">管理您的个人资料、安全设置与操作记录</p>
        </div>

        <div className="flex flex-col lg:flex-row gap-8 items-stretch">
          {/* Sidebar Navigation */}
          <div className="lg:w-64 flex-shrink-0 flex flex-col">
            <div className="bg-white rounded-xl shadow-sm border border-slate-100 overflow-hidden h-full">
              <Menu
                mode="inline"
                selectedKeys={[activeTab]}
                onClick={(e) => setActiveTab(e.key)}
                items={menuItems}
                className="border-none py-2"
                style={{ borderInlineEnd: 'none' }}
              />
              <div className="border-t border-slate-100 p-2 mt-2">
                <Button
                  type="text"
                  danger
                  block
                  icon={<LogoutOutlined />}
                  loading={logoutMutation.isPending}
                  onClick={handleLogout}
                >
                  退出登录
                </Button>
              </div>
            </div>
          </div>

          {/* Main Content */}
          <div className="flex-1 min-w-0">
            {activeTab === 'profile' && <ProfileSection />}
            {activeTab === 'security' && (
              <SecuritySection
                onStartChallenge={handleStartChallenge}
                onRegenerateRecoveryCodes={handleRegenerateRecoveryCodes}
                recoveryCodesLoading={regenerateRecoveryCodesMutation.isPending}
                onDeletePasskey={handleDeletePasskey}
                onDeleteOtpBinding={handleDeleteOtpBinding}
              />
            )}
            {activeTab === 'logs' && <LogSection />}
          </div>
        </div>
      </div>

      <Modal
        title="新的恢复码已生成"
        open={recoveryModalOpen}
        footer={[
          <Button key="ok" type="primary" onClick={() => setRecoveryModalOpen(false)}>
            我已保存，关闭
          </Button>
        ]}
        onCancel={() => setRecoveryModalOpen(false)}
      >
        <Alert
          title="警告"
          description="请立即保存下列代码。关闭窗口后将无法再次查看。"
          type="warning"
          showIcon
          className="mb-4"
        />
        <div className="max-h-60 overflow-y-auto bg-slate-50 p-4 rounded-lg border border-slate-200 font-mono">
          <List
            dataSource={recoveryCodes}
            renderItem={(code) => (
              <List.Item className="!py-2 !border-b-0">
                <Typography.Text copyable>{code}</Typography.Text>
              </List.Item>
            )}
          />
        </div>
      </Modal>
    </div>
  );
}

export default function AccountSettingsPage() {
  return <AccountSettingsContent />;
}
