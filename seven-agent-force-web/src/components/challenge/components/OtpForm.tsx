'use client';

import React, { useEffect, useRef, useState } from 'react';
import { Form, Input, QRCode, Alert, Typography } from 'antd';

interface OtpFormProps {
  mode: 'totp' | 'email';
  /** Email OTP 才需要：目标邮箱（脱敏后） */
  targetEmail?: string;
  /** TOTP 注册场景：otpauth URL */
  otpauthUrl?: string;
  /** TOTP 注册场景：密钥 */
  secret?: string;
  busy: boolean;
  /** Email 模式：发送验证码的回调，返回冷却秒数 */
  onSendCode?: () => Promise<void>;
  /** 冷却时间（秒），外部控制 */
  cooldownSeconds?: number;
}

const COOLDOWN_DURATION = 60;

export function OtpForm({
  mode,
  targetEmail,
  otpauthUrl,
  secret,
  busy,
  onSendCode,
  cooldownSeconds: externalCooldown,
}: OtpFormProps) {
  const [countdown, setCountdown] = useState(0);
  const [sending, setSending] = useState(false);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // 外部触发冷却（后端返回 cooldownSeconds 时）
  useEffect(() => {
    if (typeof externalCooldown === 'number' && externalCooldown > 0) {
      setCountdown(externalCooldown);
    }
  }, [externalCooldown]);

  useEffect(() => {
    if (countdown > 0) {
      timerRef.current = setInterval(() => {
        setCountdown((prev) => {
          if (prev <= 1) {
            clearInterval(timerRef.current!);
            return 0;
          }
          return prev - 1;
        });
      }, 1000);
    }
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [countdown]);

  const handleSend = async () => {
    if (!onSendCode || sending || countdown > 0) return;
    setSending(true);
    try {
      await onSendCode();
      setCountdown(COOLDOWN_DURATION);
    } catch {
      // The parent challenge modal owns the actionable error message. Keep the
      // resend action available instead of starting a false success cooldown.
    } finally {
      setSending(false);
    }
  };

  const otpInputStyle: React.CSSProperties = {
    letterSpacing: '0.25em',
    fontSize: 22,
    fontFamily: "'JetBrains Mono', 'SFMono-Regular', Menlo, monospace",
    fontWeight: 700,
    textAlign: 'center',
  };

  return (
    <div>
      {/* TOTP 注册场景：二维码 + 密钥 */}
      {mode === 'totp' && otpauthUrl && (
        <div style={{ marginBottom: 20 }}>
          <div style={{
            display: 'flex',
            justifyContent: 'center',
            marginBottom: 12,
          }}>
            <div style={{
              background: '#fff',
              borderRadius: 12,
              padding: 12,
              border: '1px solid #e2e8f0',
              boxShadow: '0 2px 8px rgba(15,23,42,0.07)',
            }}>
              <QRCode value={otpauthUrl} size={160} />
            </div>
          </div>
          <p style={{ fontSize: 12, color: '#94a3b8', textAlign: 'center', marginBottom: 8 }}>
            用 Authenticator App 扫描二维码
          </p>
          {secret && (
            <Alert
              style={{ borderRadius: 8 }}
              message={
                <span style={{ fontSize: 12 }}>
                  无法扫码？手动添加密钥：
                  <Typography.Text copyable style={{ fontSize: 12, fontFamily: 'monospace' }}>
                    {String(secret)}
                  </Typography.Text>
                </span>
              }
              type="info"
            />
          )}
        </div>
      )}

      {/* Email 提示 */}
      {mode === 'email' && (
        <div style={{
          background: 'rgba(14,165,233,0.06)',
          border: '1px solid rgba(56,189,248,0.16)',
          borderRadius: 10,
          padding: '10px 14px',
          marginBottom: 16,
          fontSize: 13,
          color: '#475569',
          textAlign: 'center',
        }}>
          {targetEmail
            ? <>验证码已发送至 <strong style={{ color: '#0284c7' }}>{targetEmail}</strong></>
            : '验证码已发送至你的绑定邮箱'}
        </div>
      )}

      {/* OTP 输入 */}
      <Form.Item
        className="challenge-otp-form-item"
        name="oneTimePassword"
        rules={[{ required: true, len: 6, message: '请输入 6 位验证码' }]}
        style={{ marginBottom: mode === 'email' && onSendCode ? 8 : 0 }}
      >
        <Input.OTP
          className="challenge-otp-input"
          length={6}
          size="large"
          inputMode="numeric"
          style={{ ...otpInputStyle, width: 'auto' }}
          formatter={(v) => v.replace(/\D/g, '')}
        />
      </Form.Item>

      {/* Email 模式：获取验证码按钮 */}
      {mode === 'email' && onSendCode && (
        <div style={{ textAlign: 'center' }}>
          <button
            type="button"
            disabled={busy || sending || countdown > 0}
            onClick={handleSend}
            style={{
              background: 'none',
              border: 'none',
              cursor: busy || sending || countdown > 0 ? 'not-allowed' : 'pointer',
              fontSize: 13,
              color: countdown > 0 ? '#94a3b8' : '#0284c7',
              fontWeight: 500,
              padding: '4px 0',
              opacity: busy || sending ? 0.65 : 1,
            }}
          >
            {sending ? '发送中…' : countdown > 0 ? `${countdown} 秒后重新发送` : '获取验证码'}
          </button>
        </div>
      )}
    </div>
  );
}
