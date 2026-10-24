"use client";

import { Button, Spin, Typography } from 'antd';
import { KeyOutlined, LeftOutlined } from '@ant-design/icons';
import React from 'react';

const { Paragraph, Text, Title } = Typography;

interface PasskeyPanelProps {
  currentAccount?: string;
  startingPasskey: boolean;
  onBack: () => void;
}

export default function PasskeyPanel(props: PasskeyPanelProps) {
  const { currentAccount, startingPasskey, onBack } = props;

  return (
    <div className="login-step-panel login-step-panel-centered">
      <div className="login-back-row" style={{ width: '100%', justifyContent: 'flex-start' }}>
        <Button
          type="text"
          className="login-back-button"
          icon={<LeftOutlined />}
          onClick={onBack}
          disabled={startingPasskey}
        >
          返回密码登录
        </Button>
      </div>
      <div className="login-step-intro">
        <div className="login-step-icon login-step-icon-neutral">
          <KeyOutlined />
        </div>
        <Title level={3} className="login-step-title">
          正在唤起通行密钥
        </Title>
        <Paragraph className="login-step-description">
          请在当前设备上完成指纹、面容或系统 PIN 验证。
          {currentAccount ? ` 当前账号：${currentAccount}` : ''}
        </Paragraph>
      </div>
      <div className="login-passkey-progress">
        <Spin size="large" />
        <Text className="login-passkey-progress-text">
          浏览器会弹出系统安全提示。若未看到，请检查设备认证弹窗或稍后重试。
        </Text>
      </div>
    </div>
  );
}
