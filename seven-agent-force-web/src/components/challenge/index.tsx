'use client';

import React, { useEffect, useState } from 'react';
import { Button, Form, Modal } from 'antd';
import {
  createPasskeyAssertionPayload,
  createPasskeyRegistrationPayload,
} from '@/lib/security/webauthn';
import { runPasskeyInLocalhostBridge } from '@/lib/security/passkeyBridge';
import {
  shouldSwitchPasskeyToLocalhostDomain,
} from '@/lib/security/passkeyDomain';
import { MethodSelector } from './components/MethodSelector';
import { OtpForm } from './components/OtpForm';
import { PasswordForm } from './components/PasswordForm';
import { RecoveryCodeForm } from './components/RecoveryCodeForm';
import { PasskeyForm } from './components/PasskeyForm';
import { CaptchaForm } from './components/CaptchaForm';
import { FailureAlert } from './components/FailureAlert';
import { resolveStepLabel, resolveStepIcon } from './challenge.types';
import type { ChallengeFlowAdapter } from './challenge.types';

export type { ChallengeFlowAdapter };

interface GlobalChallengeModalProps {
  challengeFlow: ChallengeFlowAdapter;
  currentUserAccount?: string;
  onCancel: () => void;
}

/**
 * 多因素认证挑战弹窗
 *
 * 交互模式：
 *  - 若只有一种可用方式，直接展示对应验证表单
 *  - 若有多种可用方式，先展示方法选择器（列表即按钮，点击进入子表单）
 *  - 子表单顶部有"← 返回"可回到方法选择器
 */
export default function GlobalChallengeModal({
  challengeFlow,
  currentUserAccount,
  onCancel,
}: GlobalChallengeModalProps) {
  const [form] = Form.useForm();
  const [error, setError] = useState<string>();

  const step = challengeFlow.currentStep;
  const availableSteps = challengeFlow.availableSteps ?? [];
  const type = step?.challengeType;
  const hints = step?.userInterfaceHints ?? {};
  const latestResult = challengeFlow.lastResponse;
  const recommendedStepIdentifier = challengeFlow.challenge?.recommendedStepIdentifier;
  const resendCooldownSeconds =
    typeof hints.resendCooldownSeconds === 'number' ? hints.resendCooldownSeconds : 0;
  const [verificationCountdown, setVerificationCountdown] = useState(0);

  const hasMultipleSteps = availableSteps.length > 1;
  const isActive = challengeFlow.isActive;
  // 尚未选择具体步骤（显示选择器）
  const [showSelector, setShowSelector] = useState(false);

  // 每次 challenge 激活 / 步骤变化时，决定是否需要先展示选择器
  useEffect(() => {
    if (!isActive) return;
    form.resetFields();
    queueMicrotask(() => {
      setShowSelector(hasMultipleSteps && !step?.stepIdentifier);
      setError(undefined);
    });
    if (type === 'WEBAUTHN_PASSKEY_REGISTRATION') {
      form.setFieldValue('displayName', `My Passkey ${new Date().toLocaleDateString()}`);
    }
    // 自动激活：单步场景或已确定步骤时，若是 Passkey 断言则立刻触发
  }, [form, hasMultipleSteps, isActive, step?.stepIdentifier, type]);

  useEffect(() => {
    const seconds = latestResult?.cooldownSeconds ?? step?.cooldownSeconds ?? 0;
    setVerificationCountdown(seconds > 0 ? seconds : 0);
    if (seconds <= 0) return;
    const timer = window.setInterval(() => {
      setVerificationCountdown((current) => {
        if (current <= 1) {
          window.clearInterval(timer);
          return 0;
        }
        return current - 1;
      });
    }, 1000);
    return () => window.clearInterval(timer);
  }, [latestResult?.cooldownSeconds, step?.cooldownSeconds, step?.stepIdentifier]);

  // 选择器：用户点击某个方式
  const handleSelectStep = (stepIdentifier: string) => {
    challengeFlow.switchStep(stepIdentifier);
    setShowSelector(false);
    form.resetFields();
    setError(undefined);
  };

  const handleBack = () => {
    setShowSelector(true);
    setError(undefined);
  };

  const showFailureHint = Boolean(latestResult?.message);
  const remainingAttemptCount =
    latestResult?.remainingAttemptCount ?? step?.remainingAttemptCount;
  const cooldownSeconds = latestResult?.cooldownSeconds ?? step?.cooldownSeconds;

  // ── 提交逻辑（完整保留原有业务） ──────────────────────────────
  const handleSubmit = async () => {
    if (!step?.stepIdentifier || !type) return;
    setError(undefined);
    try {
      let payload: Record<string, unknown> | undefined;

      if (type === 'WEBAUTHN_PASSKEY_REGISTRATION') {
        const values = await form.validateFields(['displayName']);
        const passkeyPayload = shouldSwitchPasskeyToLocalhostDomain()
          ? await runPasskeyInLocalhostBridge({
            mode: 'registration',
            hints: step.userInterfaceHints,
            userName: currentUserAccount || 'user',
            displayName: values.displayName,
          })
          : await createPasskeyRegistrationPayload(
            step.userInterfaceHints,
            currentUserAccount || 'user',
            values.displayName,
          );
        payload = { ...passkeyPayload };
      } else if (type === 'WEBAUTHN_PASSKEY_ASSERTION') {
        const passkeyPayload = shouldSwitchPasskeyToLocalhostDomain()
          ? await runPasskeyInLocalhostBridge({
            mode: 'assertion',
            hints: step.userInterfaceHints,
          })
          : await createPasskeyAssertionPayload(step.userInterfaceHints);
        payload = { ...passkeyPayload };
      } else if (type === 'IMAGE_CAPTCHA') {
        const values = await form.validateFields(['captchaCode']);
        payload = { captchaCode: values.captchaCode?.trim() };
      } else if (type === 'PASSWORD_VERIFICATION') {
        const values = await form.validateFields(['password']);
        payload = { password: values.password, userAccount: currentUserAccount };
      } else if (
        type === 'TIME_BASED_ONE_TIME_PASSWORD' ||
        type === 'EMAIL_ONE_TIME_PASSWORD'
      ) {
        const values = await form.validateFields(['oneTimePassword']);
        payload = { oneTimePassword: values.oneTimePassword?.trim() };
      } else if (type === 'RECOVERY_CODE_VERIFICATION') {
        const values = await form.validateFields(['recoveryCode']);
        payload = { recoveryCode: values.recoveryCode?.trim() };
      }

      if (payload) {
        await challengeFlow.submitCurrentStep({ payload });
      }
    } catch (err) {
      const msg = (err as Error)?.message;
      if (msg) setError(msg);
    }
  };

  // ── 渲染各步骤表单内容 ────────────────────────────────────────
  const renderStepForm = () => {
    switch (type) {
      case 'IMAGE_CAPTCHA':
        return (
          <CaptchaForm
            form={form}
            codeImage={typeof hints.codeImage === 'string' ? hints.codeImage : undefined}
            busy={challengeFlow.busy}
            onRefresh={() => challengeFlow.refreshCurrentStep()}
          />
        );

      case 'PASSWORD_VERIFICATION':
        return <PasswordForm form={form} />;

      case 'TIME_BASED_ONE_TIME_PASSWORD':
        return (
          <OtpForm
            mode="totp"
            otpauthUrl={typeof hints.otpauthUrl === 'string' ? hints.otpauthUrl : undefined}
            secret={typeof hints.secret === 'string' ? hints.secret : undefined}
            busy={challengeFlow.busy}
            cooldownSeconds={typeof cooldownSeconds === 'number' ? cooldownSeconds : undefined}
          />
        );

      case 'EMAIL_ONE_TIME_PASSWORD':
        return (
          <OtpForm
            key={step?.stepIdentifier}
            mode="email"
            targetEmail={
              typeof hints.emailMasked === 'string'
                ? hints.emailMasked
                : typeof hints.targetEmail === 'string'
                  ? hints.targetEmail
                  : undefined
            }
            busy={challengeFlow.busy}
            cooldownSeconds={resendCooldownSeconds}
            onSendCode={async () => {
              setError(undefined);
              try {
                await challengeFlow.refreshCurrentStep();
                form.setFieldValue('oneTimePassword', undefined);
              } catch (err) {
                const message = (err as Error)?.message || '验证码发送失败，请稍后重试';
                setError(message);
                throw err;
              }
            }}
          />
        );

      case 'WEBAUTHN_PASSKEY_ASSERTION':
        return <PasskeyForm form={form} mode="assertion" busy={challengeFlow.busy} />;

      case 'WEBAUTHN_PASSKEY_REGISTRATION':
        return <PasskeyForm form={form} mode="registration" busy={challengeFlow.busy} />;

      case 'RECOVERY_CODE_VERIFICATION':
        return <RecoveryCodeForm form={form} />;

      default:
        return (
          <div style={{
            padding: '20px 0',
            textAlign: 'center',
            color: '#94a3b8',
            fontSize: 13,
          }}>
            未知验证步骤：{type}
          </div>
        );
    }
  };

  // ── 弹窗底部按钮 ──────────────────────────────────────────────
  const footerButtons = showSelector
    ? [
        <Button key="cancel" onClick={onCancel} disabled={challengeFlow.busy}>
          取消
        </Button>,
      ]
    : [
        hasMultipleSteps && (
          <Button
            key="back"
            onClick={handleBack}
            disabled={challengeFlow.busy}
            style={{ marginRight: 'auto' }}
          >
            ← 返回
          </Button>
        ),
        <Button key="cancel" onClick={onCancel} disabled={challengeFlow.busy}>
          取消
        </Button>,
        <Button
          key="confirm"
          type="primary"
          loading={challengeFlow.busy}
          disabled={challengeFlow.busy || verificationCountdown > 0}
          onClick={handleSubmit}
          style={{
            background: 'linear-gradient(90deg, #3b82f6 0%, #22d3ee 100%)',
            border: 'none',
            borderRadius: 8,
            fontWeight: 600,
          }}
        >
          {verificationCountdown > 0
            ? `${verificationCountdown} 秒后可重试`
            : type === 'WEBAUTHN_PASSKEY_ASSERTION'
              ? '调起验证'
              : '确认验证'}
        </Button>,
      ].filter(Boolean);

  // ── 弹窗标题 ──────────────────────────────────────────────────
  const modalTitle = showSelector ? (
    <span style={{ fontWeight: 700, fontSize: 16 }}>选择验证方式</span>
  ) : (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
      <span style={{ fontSize: 20 }}>{resolveStepIcon(type)}</span>
      <span style={{ fontWeight: 700, fontSize: 16 }}>{resolveStepLabel(type)}</span>
    </div>
  );

  return (
    <Modal
      title={modalTitle}
      open={challengeFlow.isActive}
      zIndex={5000}
      onCancel={onCancel}
      confirmLoading={challengeFlow.busy}
      destroyOnHidden
      footer={footerButtons}
      width={480}
      styles={{
        header: {
          borderBottom: '1px solid #f1f5f9',
          paddingBottom: 14,
          marginBottom: 0,
        },
        body: {
          paddingTop: 20,
        },
      }}
    >
      {/* 错误横幅 */}
      {error && (
        <div style={{
          background: 'rgba(239,68,68,0.08)',
          border: '1px solid rgba(239,68,68,0.22)',
          borderRadius: 8,
          padding: '9px 13px',
          marginBottom: 16,
          fontSize: 13,
          color: '#dc2626',
          display: 'flex',
          gap: 8,
          alignItems: 'flex-start',
        }}>
          <span>{error}</span>
        </div>
      )}

      {/* 验证失败提示（来自后端 lastResponse） */}
      {!error && showFailureHint && (
        <FailureAlert
          message={latestResult?.message}
          remainingAttemptCount={remainingAttemptCount}
          cooldownSeconds={cooldownSeconds}
          canSwitchMethod={latestResult?.canSwitchMethod}
        />
      )}

      {/* 主体内容：选择器 或 步骤表单 */}
      {showSelector ? (
        <MethodSelector
          steps={availableSteps}
          recommendedStepIdentifier={recommendedStepIdentifier}
          busy={challengeFlow.busy}
          onSelect={handleSelectStep}
        />
      ) : (
        <Form form={form} layout="vertical" style={{ margin: 0 }}>
          {renderStepForm()}
        </Form>
      )}
    </Modal>
  );
}
