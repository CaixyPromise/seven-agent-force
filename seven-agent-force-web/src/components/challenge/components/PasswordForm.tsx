'use client';

import React from 'react';
import { Form, Input } from 'antd';
import { KeyOutlined } from '@ant-design/icons';

interface PasswordFormProps {
  form: ReturnType<typeof Form.useForm>[0];
}

export const PasswordForm: React.FC<PasswordFormProps> = () => {
  return (
    <div>
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 10,
        background: 'rgba(14,165,233,0.06)',
        border: '1px solid rgba(56,189,248,0.18)',
        borderRadius: 10,
        padding: '10px 14px',
        marginBottom: 16,
      }}>
        <KeyOutlined style={{ fontSize: 18, color: '#0284c7' }} />
        <span style={{ fontSize: 13, color: '#475569' }}>请输入当前账号的登录密码以确认身份</span>
      </div>
      <Form.Item
        name="password"
        rules={[{ required: true, message: '请输入密码' }]}
        style={{ marginBottom: 0 }}
      >
        <Input.Password
          size="large"
          placeholder="请输入登录密码"
          style={{ borderRadius: 10 }}
        />
      </Form.Item>
    </div>
  );
};
