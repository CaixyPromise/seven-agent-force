'use client';

import React from 'react';
import { Form, Input } from 'antd';
import { KeyOutlined } from '@ant-design/icons';

interface RecoveryCodeFormProps {
  form: ReturnType<typeof Form.useForm>[0];
}

export const RecoveryCodeForm: React.FC<RecoveryCodeFormProps> = () => {
  return (
    <div>
      <div style={{
        background: 'rgba(245,158,11,0.07)',
        border: '1px solid rgba(245,158,11,0.20)',
        borderRadius: 10,
        padding: '12px 14px',
        marginBottom: 16,
      }}>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 8,
          marginBottom: 6,
        }}>
          <KeyOutlined style={{ fontSize: 16, color: '#b45309' }} />
          <span style={{ fontSize: 13, fontWeight: 600, color: '#92400e' }}>使用账号恢复码</span>
        </div>
        <ul style={{
          margin: 0,
          paddingLeft: 18,
          fontSize: 12,
          color: '#78350f',
          lineHeight: 1.8,
        }}>
          <li>恢复码是注册时生成的一次性密钥</li>
          <li>每个恢复码只能使用一次</li>
          <li>格式通常为 <code style={{ fontFamily: 'monospace', letterSpacing: '0.05em' }}>XXXX-XXXX-XXXX</code></li>
        </ul>
      </div>
      <Form.Item
        name="recoveryCode"
        rules={[{ required: true, message: '请输入恢复码' }]}
        style={{ marginBottom: 0 }}
      >
        <Input
          size="large"
          placeholder="XXXX-XXXX-XXXX"
          style={{
            borderRadius: 10,
            fontFamily: "'JetBrains Mono', 'SFMono-Regular', Menlo, monospace",
            fontSize: 15,
            letterSpacing: '0.08em',
          }}
        />
      </Form.Item>
    </div>
  );
};
