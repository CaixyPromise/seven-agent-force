import React from 'react';
import {
  KeyOutlined,
  LockOutlined,
  MailOutlined,
  SafetyCertificateOutlined,
  PictureOutlined,
  FieldTimeOutlined,
} from '@ant-design/icons';
import type { ChallengeStep, ChallengeRequiredPayload } from '@/lib/http/types';

export type { ChallengeStep, ChallengeRequiredPayload };

export interface ChallengeFlowAdapter {
  challenge: ChallengeRequiredPayload | null;
  currentStep: ChallengeStep | null;
  availableSteps: ChallengeStep[];
  flowNonce?: string | null;
  lastResponse?: {
    message?: string;
    remainingAttemptCount?: number;
    cooldownSeconds?: number;
    canSwitchMethod?: boolean;
  } | null;
  busy: boolean;
  isActive: boolean;
  switchStep: (stepIdentifier: string) => void;
  refreshCurrentStep: () => Promise<unknown>;
  submitCurrentStep: (params: {
    payload: Record<string, unknown>;
    stepIdentifier?: string;
  }) => Promise<unknown>;
  reset: () => void;
}

export function resolveStepLabel(challengeType?: string): string {
  switch (challengeType) {
    case 'IMAGE_CAPTCHA':
      return '图形验证码';
    case 'PASSWORD_VERIFICATION':
      return '登录密码';
    case 'TIME_BASED_ONE_TIME_PASSWORD':
      return '身份验证器 OTP';
    case 'EMAIL_ONE_TIME_PASSWORD':
      return '邮箱验证码';
    case 'WEBAUTHN_PASSKEY_ASSERTION':
      return 'Passkey 验证';
    case 'WEBAUTHN_PASSKEY_REGISTRATION':
      return 'Passkey 绑定';
    case 'RECOVERY_CODE_VERIFICATION':
      return '账号恢复码';
    default:
      return challengeType || '未知方式';
  }
}

function createStepIcon(Icon: typeof KeyOutlined) {
  return React.createElement(Icon, {
    style: {
      fontSize: 18,
      color: '#0284c7',
    },
  });
}

export function resolveStepIcon(challengeType?: string) {
  switch (challengeType) {
    case 'IMAGE_CAPTCHA':
      return createStepIcon(PictureOutlined);
    case 'PASSWORD_VERIFICATION':
      return createStepIcon(KeyOutlined);
    case 'TIME_BASED_ONE_TIME_PASSWORD':
      return createStepIcon(FieldTimeOutlined);
    case 'EMAIL_ONE_TIME_PASSWORD':
      return createStepIcon(MailOutlined);
    case 'WEBAUTHN_PASSKEY_ASSERTION':
      return createStepIcon(SafetyCertificateOutlined);
    case 'WEBAUTHN_PASSKEY_REGISTRATION':
      return createStepIcon(SafetyCertificateOutlined);
    case 'RECOVERY_CODE_VERIFICATION':
      return createStepIcon(KeyOutlined);
    default:
      return createStepIcon(LockOutlined);
  }
}
