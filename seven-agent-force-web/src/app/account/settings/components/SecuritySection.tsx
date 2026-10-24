'use client';

import React, { useState } from 'react';
import {
  Button,
  Card,
  Space,
  Table,
  Tag,
  Typography,
  message,
  Popconfirm,
} from 'antd';
import {
  SafetyCertificateOutlined,
  KeyOutlined,
  MobileOutlined,
  LockOutlined,
  DeleteOutlined,
  PlusOutlined,
  SafetyOutlined, // For Recovery Codes fallback
  CheckCircleFilled,
  CloseCircleFilled,
} from '@ant-design/icons';
import { useMutation, useQuery } from '@tanstack/react-query';
import {
  getMfaStatus,
  listMfaPasskeys,
  MFA_ACTIONS,
  type MfaBusinessAction,
  type MfaPasskeyVO,
} from '@/api/mfaController';
import { getCurrentUserProfile } from '@/api/userProfileController';
import type { ColumnsType } from 'antd/es/table';
import PasswordChangeModal from './PasswordChangeModal';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';
import ExternalBindingsCard from './ExternalBindingsCard';
import EnterpriseAccountsCard from './EnterpriseAccountsCard';
import WorkNotificationsCard from './WorkNotificationsCard';

const { Text } = Typography;

interface SecuritySectionProps {
  onStartChallenge: (action: MfaBusinessAction, context?: Record<string, unknown>) => Promise<void>;
  onRegenerateRecoveryCodes: () => void;
  recoveryCodesLoading: boolean;
  onDeletePasskey: (credentialIdentifier: string) => Promise<void>;
  onDeleteOtpBinding: () => Promise<void>;
}

function extractErrorMessage(error: unknown, fallback: string) {
  return (error as { message?: string })?.message || fallback;
}

function isUserCancelledChallenge(error: unknown) {
  return isChallengeRetryError(error, 'CHALLENGE_CANCELLED');
}

function formatPasswordChangedAt(value?: string | null) {
  if (!value) {
    return '暂无修改记录';
  }
  const timestamp = new Date(value).getTime();
  if (!Number.isFinite(timestamp)) {
    return '暂无修改记录';
  }
  const diffMs = timestamp - Date.now();
  const absMs = Math.abs(diffMs);
  const units: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['year', 365 * 24 * 60 * 60 * 1000],
    ['month', 30 * 24 * 60 * 60 * 1000],
    ['day', 24 * 60 * 60 * 1000],
    ['hour', 60 * 60 * 1000],
    ['minute', 60 * 1000],
  ];
  const formatter = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' });
  for (const [unit, unitMs] of units) {
    if (absMs >= unitMs) {
      return `上次修改于 ${formatter.format(Math.round(diffMs / unitMs), unit)}`;
    }
  }
  return '刚刚修改';
}

export default function SecuritySection({
  onStartChallenge,
  onRegenerateRecoveryCodes,
  recoveryCodesLoading,
  onDeletePasskey,
  onDeleteOtpBinding,
}: SecuritySectionProps) {
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);

  const { data: profileResponse, refetch: refetchProfile } = useQuery({
    queryKey: ['account-settings', 'profile'],
    queryFn: () => getCurrentUserProfile(),
  });
  const passwordChangedAt = profileResponse?.data?.passwordChangedAt;

  // --- MFA Data Fetching ---
  const { data: mfaStatusResponse, refetch: refetchMfaStatus } = useQuery({
    queryKey: ['account-settings', 'mfa-status'],
    queryFn: () => getMfaStatus(),
  });
  const mfaStatus = mfaStatusResponse?.data;

  const { data: passkeyResponse, refetch: refetchPasskeys, isLoading: isLoadingPasskeys } = useQuery({
    queryKey: ['account-settings', 'passkeys'],
    queryFn: () => listMfaPasskeys(),
  });
  const passkeyList = (passkeyResponse?.data ?? []) as MfaPasskeyVO[];

  // --- Mutations ---
  const deletePasskeyMutation = useMutation({
    mutationFn: (credentialIdentifier: string) => onDeletePasskey(credentialIdentifier),
    onSuccess: async () => {
      await Promise.all([refetchPasskeys(), refetchMfaStatus()]);
    },
    onError: (error: unknown) => {
      if (isUserCancelledChallenge(error)) {
        return;
      }
      message.error(extractErrorMessage(error, '删除失败'));
    },
  });

  const deleteOtpBindingMutation = useMutation({
    mutationFn: () => onDeleteOtpBinding(),
    onSuccess: async () => {
      await refetchMfaStatus();
    },
    onError: (error: unknown) => {
      if (isUserCancelledChallenge(error)) {
        return;
      }
      message.error(extractErrorMessage(error, 'OTP 解绑失败'));
    },
  });

  // --- Columns ---
  const passkeyColumns: ColumnsType<MfaPasskeyVO> = [
    {
      title: '设备名称',
      dataIndex: 'displayName',
      key: 'displayName',
      render: (_, record) => (
        <span className="font-medium flex items-center gap-2">
          <MobileOutlined className="text-slate-400" />
          {record.displayName || record.credentialIdentifier || '未命名设备'}
        </span>
      ),
    },
    {
      title: '添加时间',
      dataIndex: 'createdAt',
      key: 'createdAt',
      responsive: ['md'],
      render: (val) => <span className="text-slate-500">{val ? new Date(val).toLocaleDateString() : '-'}</span>,
    },
    {
      title: '最近使用',
      dataIndex: 'lastUsedAt',
      key: 'lastUsedAt',
      render: (val) => <span className="text-slate-500">{val ? new Date(val).toLocaleDateString() : '从未'}</span>,
    },
    {
      title: '操作',
      key: 'action',
      width: 100,
      render: (_, record) => (
        <Popconfirm
          title="删除设备"
          description="确定要移除此 Passkey 吗？"
          onConfirm={() => {
            if (record.credentialIdentifier) {
              deletePasskeyMutation.mutate(record.credentialIdentifier);
            }
          }}
          okText="删除"
          cancelText="取消"
          okButtonProps={{ danger: true }}
        >
          <Button type="text" danger icon={<DeleteOutlined />} size="small">
                        删除
          </Button>
        </Popconfirm>
      ),
    },
  ];

  return (
    <div className="animate-fade-in">
      {/* 密码管理 Card (Refactored) */}
      <Card
        title="登录密码"
        variant="borderless"
        className="shadow-sm rounded-xl"
        style={{ marginBottom: 24 }}
        extra={
          <Text type="secondary" className="text-xs">
                        定期更换密码可以保护您的账号安全
          </Text>
        }
      >
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="p-3 bg-indigo-50 text-indigo-600 rounded-xl flex items-center justify-center">
              <LockOutlined style={{ fontSize: '20px' }} />
            </div>
            <div>
              <p className="text-sm font-medium text-slate-900 mb-0.5">
                {formatPasswordChangedAt(passwordChangedAt)}
              </p>
              <p className="text-xs text-slate-500 mb-0">建议设置包含数字和符号的高强度密码</p>
            </div>
          </div>
          <Button onClick={() => setPasswordModalOpen(true)}>修改密码</Button>
        </div>
      </Card>

      {/* MFA 概览 Dashboard (Refactored Styles to match prototype) */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-3" style={{ marginBottom: 24 }}>
        {/* OTP Status */}
        <div className={`p-5 rounded-xl border flex flex-col justify-between h-40 ${mfaStatus?.otpBound
          ? 'bg-emerald-50 border-emerald-100'
          : 'bg-slate-50 border-slate-200'
        }`}>
          <div className="flex justify-between items-start">
            <SafetyCertificateOutlined
              className={`text-2xl ${mfaStatus?.otpBound ? 'text-emerald-600' : 'text-slate-400'}`}
            />
            <Tag
              color={mfaStatus?.otpBound ? 'success' : 'default'}
              className="m-0 border-0 rounded-full "
              icon={mfaStatus?.otpBound ? <CheckCircleFilled /> : <CloseCircleFilled />}
            >
              {mfaStatus?.otpBound ? '已开启' : '未启用'}
            </Tag>
          </div>
          <div>
            <h4 className="font-semibold text-slate-900 text-base mb-1">身份验证器 (OTP)</h4>
            <p className="text-xs text-slate-500">推荐使用 Authenticator App 进行二次验证</p>
            <div className="mt-2">
              <Space size={10}>
                <Button
                  type="link"
                  className="p-0 h-auto text-xs"
                  onClick={() => onStartChallenge(mfaStatus?.otpBound ? MFA_ACTIONS.OTP_SWITCH : MFA_ACTIONS.OTP_BIND)}
                >
                  {mfaStatus?.otpBound ? '管理设备 >' : '立即绑定 >'}
                </Button>
                {mfaStatus?.otpBound && (
                  <Popconfirm
                    title="解绑 OTP"
                    description="解绑后将无法使用身份验证器动态码，请确认继续。"
                    okText="确认解绑"
                    cancelText="取消"
                    okButtonProps={{ danger: true, loading: deleteOtpBindingMutation.isPending }}
                    onConfirm={() => deleteOtpBindingMutation.mutate()}
                  >
                    <Button danger type="link" className="p-0 h-auto text-xs">
                      解绑
                    </Button>
                  </Popconfirm>
                )}
              </Space>
            </div>
          </div>
        </div>

        {/* Passkey Status */}
        <div className="p-5 rounded-xl border bg-indigo-50 border-indigo-100 flex flex-col justify-between h-40">
          <div className="flex justify-between items-start">
            <KeyOutlined className="text-2xl text-indigo-600" />
            <Tag color="geekblue" className="m-0 border-0 rounded-full px-2">
              {passkeyList?.length || 0} 个设备
            </Tag>
          </div>
          <div>
            <h4 className="font-semibold text-slate-900 text-base mb-1">通行密钥 (Passkey)</h4>
            <p className="text-xs text-slate-500">使用指纹或面容 ID 登录，无需输入密码</p>
            <div className="mt-2">
              <Button
                type="link"
                className="p-0 h-auto text-xs"
                onClick={() => onStartChallenge(mfaStatus?.passkeyBound ? MFA_ACTIONS.PASSKEY_SWITCH : MFA_ACTIONS.PASSKEY_BIND)}
              >
                                管理密钥 &gt;
              </Button>
            </div>
          </div>
        </div>

        {/* Recovery Codes Status */}
        <div className={`p-5 rounded-xl border flex flex-col justify-between h-40 ${Number(mfaStatus?.availableRecoveryCodeCount || 0) < 3
          ? 'bg-amber-50 border-amber-100'
          : 'bg-slate-50 border-slate-200'
        }`}>
          <div className="flex justify-between items-start">
            <SafetyOutlined
              className={`text-2xl ${Number(mfaStatus?.availableRecoveryCodeCount || 0) < 3 ? 'text-amber-600' : 'text-slate-400'}`}
            />
            <span className={`text-2xl font-bold ${Number(mfaStatus?.availableRecoveryCodeCount || 0) < 3 ? 'text-amber-600' : 'text-slate-700'}`}>
              {mfaStatus?.availableRecoveryCodeCount || 0}
            </span>
          </div>
          <div>
            <h4 className="font-semibold text-slate-900 text-base mb-1">备用恢复码</h4>
            <p className="text-xs text-slate-500">剩余可用次数，紧急情况下恢复访问权限</p>
          </div>
        </div>
      </div>

      <div style={{ marginBottom: 24 }}>
        <ExternalBindingsCard />
      </div>

      <div style={{ marginBottom: 24 }}>
        <EnterpriseAccountsCard />
      </div>

      <div style={{ marginBottom: 24 }}>
        <WorkNotificationsCard />
      </div>

      {/* MFA Detail Management Card */}
      <Card
        title="多因素认证管理 (MFA)"
        variant='borderless'
        className="shadow-sm rounded-xl"
      >
        {/* Passkey List */}
        <div className="mb-10 px-2">
          <div className="flex justify-between items-center mb-6">
            <h4 className="text-sm font-medium text-slate-900 flex items-center gap-2">
              <KeyOutlined /> 通行密钥列表
            </h4>
            <Button
              size="small"
              icon={<PlusOutlined />}
              onClick={() => onStartChallenge(mfaStatus?.passkeyBound ? MFA_ACTIONS.PASSKEY_SWITCH : MFA_ACTIONS.PASSKEY_BIND)}
            >
                            添加新设备
            </Button>
          </div>
          <Table<MfaPasskeyVO>
            rowKey={(record) => record.credentialIdentifier || Math.random().toString()}
            columns={passkeyColumns}
            dataSource={passkeyList}
            loading={isLoadingPasskeys}
            pagination={false}
            bordered
            size="small"
            locale={{ emptyText: '暂无已绑定的 Passkey 设备' }}
          />
        </div>

        {/* Recovery Codes Section */}
        <div className="border-t border-slate-100 pt-8 px-2">
          <div className="bg-slate-50 rounded-lg p-6 flex flex-col sm:flex-row items-center justify-between gap-4 border border-slate-200">
            <div>
              <h4 className="text-sm font-medium text-slate-900 flex items-center gap-2 mb-2">
                <SafetyOutlined className="text-amber-500" /> 紧急恢复码
              </h4>
              <p className="text-xs text-slate-500 max-w-xl leading-relaxed">
                                恢复码是您在丢失手机或无法使用 MFA 时的唯一登录方式。生成新代码会立即使旧代码失效，请谨慎操作。
              </p>
            </div>
            <Space>
              <Button onClick={() => onStartChallenge(MFA_ACTIONS.RECOVERY_VERIFY)}>验证</Button>
              <Button
                danger
                loading={recoveryCodesLoading}
                onClick={onRegenerateRecoveryCodes}
              >
                                重置恢复码
              </Button>
            </Space>
          </div>
        </div>
      </Card>

      {passwordModalOpen ? (
        <PasswordChangeModal
          open={passwordModalOpen}
          onCancel={() => setPasswordModalOpen(false)}
          onSuccess={() => {
            setPasswordModalOpen(false);
            void refetchProfile();
          }}
        />
      ) : null}
    </div>
  );
}
