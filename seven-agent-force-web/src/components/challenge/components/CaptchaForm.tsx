'use client';

import React from 'react';
import { Form, Input } from 'antd';

interface CaptchaFormProps {
  form: ReturnType<typeof Form.useForm>[0];
  codeImage?: string;
  busy: boolean;
  onRefresh: () => void;
}

function normalizeCaptchaImage(raw?: string): string {
  if (!raw) return '';
  return raw.startsWith('data:') ? raw : `data:image/jpeg;base64,${raw}`;
}

export function CaptchaForm({ codeImage, busy, onRefresh }: CaptchaFormProps) {
  return (
    <div>
      <div style={{
        background: 'rgba(14,165,233,0.05)',
        border: '1px solid rgba(56,189,248,0.14)',
        borderRadius: 10,
        padding: '10px 14px',
        marginBottom: 16,
        fontSize: 13,
        color: '#475569',
        textAlign: 'center',
      }}>
        请输入图片中显示的验证码
      </div>
      <div style={{ display: 'flex', gap: 10, alignItems: 'stretch', marginBottom: 0 }}>
        <Form.Item
          name="captchaCode"
          rules={[{ required: true, message: '请输入验证码' }]}
          style={{ flex: 1, marginBottom: 0 }}
        >
          <Input
            size="large"
            placeholder="输入验证码"
            disabled={busy}
            style={{ borderRadius: 10 }}
          />
        </Form.Item>
        <button
          type="button"
          title="点击刷新验证码"
          disabled={busy}
          onClick={onRefresh}
          style={{
            flexShrink: 0,
            width: 112,
            height: 40,
            borderRadius: 10,
            border: '1.5px solid #e2e8f0',
            overflow: 'hidden',
            background: '#f1f5f9',
            cursor: busy ? 'not-allowed' : 'pointer',
            padding: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            transition: 'border-color 0.18s',
          }}
          onMouseEnter={(e) => {
            (e.currentTarget as HTMLElement).style.borderColor = '#38bdf8';
          }}
          onMouseLeave={(e) => {
            (e.currentTarget as HTMLElement).style.borderColor = '#e2e8f0';
          }}
        >
          {codeImage ? (
            <img
              src={normalizeCaptchaImage(codeImage)}
              alt="captcha"
              style={{ width: '100%', height: '100%', objectFit: 'cover', display: 'block' }}
            />
          ) : (
            <span style={{ fontSize: 12, color: '#94a3b8' }}>点击获取</span>
          )}
        </button>
      </div>
      <div style={{ fontSize: 11, color: '#94a3b8', marginTop: 4, textAlign: 'right' }}>
        看不清？点击图片刷新
      </div>
    </div>
  );
}
