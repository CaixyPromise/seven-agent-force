'use client';

import React from 'react';
import { resolveStepLabel, resolveStepIcon } from '../challenge.types';
import type { ChallengeStep } from '../challenge.types';

interface MethodSelectorProps {
  steps: ChallengeStep[];
  recommendedStepIdentifier?: string | null;
  busy: boolean;
  onSelect: (stepIdentifier: string) => void;
}

export function MethodSelector({
  steps,
  recommendedStepIdentifier,
  busy,
  onSelect,
}: MethodSelectorProps) {
  return (
    <div>
      <p style={{
        fontSize: 13,
        color: '#64748b',
        marginBottom: 14,
        textAlign: 'center',
      }}>
        请选择一种验证方式继续
      </p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {steps.map((step) => {
          const isRecommended = step.stepIdentifier === recommendedStepIdentifier;
          return (
            <button
              key={step.stepIdentifier}
              type="button"
              disabled={busy}
              onClick={() => step.stepIdentifier && onSelect(step.stepIdentifier)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 14,
                padding: '14px 16px',
                border: isRecommended
                  ? '1.5px solid #38bdf8'
                  : '1.5px solid #e2e8f0',
                borderRadius: 12,
                background: isRecommended
                  ? 'linear-gradient(135deg, #eff6ff 0%, #ecfeff 100%)'
                  : '#f8fafc',
                cursor: busy ? 'not-allowed' : 'pointer',
                opacity: busy ? 0.65 : 1,
                textAlign: 'left',
                width: '100%',
                transition: 'all 0.18s ease',
                boxShadow: isRecommended
                  ? '0 2px 12px rgba(14,165,233,0.13)'
                  : '0 1px 4px rgba(15,23,42,0.06)',
              }}
              onMouseEnter={(e) => {
                if (!busy) {
                  (e.currentTarget as HTMLElement).style.borderColor = '#38bdf8';
                  (e.currentTarget as HTMLElement).style.background =
                    'linear-gradient(135deg, #eff6ff 0%, #ecfeff 100%)';
                }
              }}
              onMouseLeave={(e) => {
                if (!isRecommended) {
                  (e.currentTarget as HTMLElement).style.borderColor = '#e2e8f0';
                  (e.currentTarget as HTMLElement).style.background = '#f8fafc';
                }
              }}
            >
              <span style={{
                fontSize: 24,
                lineHeight: 1,
                width: 40,
                height: 40,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                background: isRecommended ? 'rgba(14,165,233,0.12)' : 'rgba(100,116,139,0.08)',
                borderRadius: 10,
                flexShrink: 0,
              }}>
                {resolveStepIcon(step.challengeType)}
              </span>
              <div style={{ flex: 1, minWidth: 0 }}>
                <div style={{
                  fontWeight: 600,
                  fontSize: 14,
                  color: '#1e293b',
                  display: 'flex',
                  alignItems: 'center',
                  gap: 6,
                }}>
                  {resolveStepLabel(step.challengeType)}
                  {isRecommended && (
                    <span style={{
                      fontSize: 11,
                      fontWeight: 600,
                      color: '#0284c7',
                      background: 'rgba(14,165,233,0.12)',
                      borderRadius: 4,
                      padding: '1px 6px',
                      letterSpacing: '0.02em',
                    }}>
                      推荐
                    </span>
                  )}
                </div>
                <div style={{ fontSize: 12, color: '#94a3b8', marginTop: 2 }}>
                  {describeMethod(step.challengeType)}
                </div>
              </div>
              <span style={{ color: '#94a3b8', fontSize: 16, flexShrink: 0 }}>›</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

function describeMethod(challengeType?: string): string {
  switch (challengeType) {
    case 'IMAGE_CAPTCHA':
      return '输入图片中显示的文字或数字';
    case 'PASSWORD_VERIFICATION':
      return '使用你的账号登录密码验证';
    case 'TIME_BASED_ONE_TIME_PASSWORD':
      return '从 Authenticator App 获取 6 位动态码';
    case 'EMAIL_ONE_TIME_PASSWORD':
      return '向绑定邮箱发送 6 位一次性验证码';
    case 'WEBAUTHN_PASSKEY_ASSERTION':
      return '使用指纹、面容或安全密钥快速验证';
    case 'WEBAUTHN_PASSKEY_REGISTRATION':
      return '绑定新的 Passkey 到你的账号';
    case 'RECOVERY_CODE_VERIFICATION':
      return '使用账号注册时保存的恢复码';
    default:
      return '';
  }
}
