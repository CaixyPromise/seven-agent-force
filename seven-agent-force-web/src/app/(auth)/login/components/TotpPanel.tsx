"use client";

import {
  ArrowLeftOutlined,
  KeyOutlined,
  MobileOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import { Button, Form, Input, Typography } from 'antd';
import React from 'react';

const { Text, Title } = Typography;

interface TotpPanelProps {
  verifyingTotp: boolean;
  onBack: () => void;
  onSubmit: (otpCode: string) => void | Promise<void>;
}

export default function TotpPanel(props: TotpPanelProps) {
  const { verifyingTotp, onBack, onSubmit } = props;
  const [form] = Form.useForm();

  return (
    <div className="login-step-panel">
      <div className="login-back-row">
        <Button
          type="text"
          className="login-back-button"
          icon={<ArrowLeftOutlined />}
          onClick={onBack}
        >
          返回修改密码
        </Button>
      </div>

      <div className="login-step-intro">
        <div className="login-step-icon login-step-icon-neutral">
          <MobileOutlined />
        </div>
        <Title level={4} className="login-step-title">
          需要动态验证码
        </Title>
        <Text className="login-step-description">
          请输入身份验证器中的 6 位动态验证码以继续登录
        </Text>
      </div>

      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        onFinish={(values) => {
          void onSubmit(values.otpCode ?? '');
        }}
        className="login-password-form"
      >
        <Form.Item
          name="otpCode"
          className="login-no-margin"
          rules={[
            { required: true, message: '请输入动态验证码' },
            { len: 6, message: '请输入 6 位动态验证码' },
          ]}
        >
          <Input
            size="large"
            className="login-plain-input"
            prefix={<KeyOutlined />}
            placeholder="------"
            maxLength={6}
            autoComplete="one-time-code"
            autoFocus
          />
        </Form.Item>

        <Button
          htmlType="submit"
          type="primary"
          className="login-primary-button"
          loading={verifyingTotp}
          icon={verifyingTotp ? <ReloadOutlined spin /> : undefined}
        >
          完成验证并登录
        </Button>
      </Form>
    </div>
  );
}
