'use client';

import React from 'react';
import { Form, Input } from 'antd';
import { KeyOutlined, SafetyCertificateOutlined } from '@ant-design/icons';

interface PasskeyFormProps {
  form: ReturnType<typeof Form.useForm>[0];
  /** registration = 绑定新 Passkey；assertion = 验证已有 Passkey */
  mode: 'registration' | 'assertion';
  busy: boolean;
}

export function PasskeyForm({ mode, busy }: PasskeyFormProps) {
  if (mode === 'assertion') {
    return (
      <div style={{ textAlign: 'center', padding: '8px 0' }}>
        <div style={{
          width: 80,
          height: 80,
          borderRadius: '50%',
          background: busy
            ? 'radial-gradient(circle, rgba(14,165,233,0.18) 0%, rgba(14,165,233,0.04) 100%)'
            : 'linear-gradient(135deg, #eff6ff 0%, #ecfeff 100%)',
          border: busy
            ? '2px solid rgba(14,165,233,0.5)'
            : '2px solid rgba(56,189,248,0.24)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          margin: '0 auto 16px',
          fontSize: 36,
          transition: 'all 0.3s ease',
          boxShadow: busy ? '0 0 0 8px rgba(14,165,233,0.08)' : 'none',
        }}>
          <SafetyCertificateOutlined style={{ color: '#0284c7' }} />
        </div>
        <p style={{ fontSize: 15, fontWeight: 600, color: '#1e293b', marginBottom: 6 }}>
          {busy ? '等待生物识别…' : '使用 Passkey 验证'}
        </p>
        <p style={{ fontSize: 13, color: '#64748b' }}>
          {busy
            ? '请在弹出的系统提示中完成指纹、面容或安全密钥验证'
            : '点击"确认验证"后，按系统提示完成 Passkey 验证'}
        </p>
        {busy && (
          <div style={{
            marginTop: 14,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            gap: 6,
            fontSize: 12,
            color: '#94a3b8',
          }}>
            <span style={{
              display: 'inline-block',
              width: 8,
              height: 8,
              borderRadius: '50%',
              background: '#0ea5e9',
              animation: 'pulse 1.2s infinite',
            }} />
            等待浏览器响应…
          </div>
        )}
      </div>
    );
  }

  // 注册模式
  return (
    <div>
      <div style={{
        background: 'linear-gradient(135deg, #eff6ff 0%, #ecfeff 100%)',
        border: '1px solid rgba(56,189,248,0.18)',
        borderRadius: 12,
        padding: '14px 16px',
        marginBottom: 18,
        textAlign: 'center',
      }}>
        <div style={{ fontSize: 28, marginBottom: 8, color: '#0284c7' }}>
          <KeyOutlined />
        </div>
        <p style={{ fontSize: 14, fontWeight: 600, color: '#0f172a', marginBottom: 4 }}>
          绑定新的 Passkey
        </p>
        <p style={{ fontSize: 12, color: '#0284c7' }}>
          Passkey 让你无需密码，使用指纹或面容即可登录
        </p>
      </div>
      <Form.Item
        name="displayName"
        label={<span style={{ fontSize: 13, fontWeight: 500 }}>设备名称</span>}
        rules={[{ required: true, message: '请输入设备名称' }]}
        style={{ marginBottom: 0 }}
      >
        <Input
          size="large"
          placeholder="例如：MacBook TouchID、iPhone 面容 ID"
          style={{ borderRadius: 10 }}
        />
      </Form.Item>
    </div>
  );
}
