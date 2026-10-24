"use client";

import {
  LockOutlined,
} from '@ant-design/icons';
import { Typography } from 'antd';
import React from 'react';

const { Text, Title } = Typography;

interface LockedPanelProps {
  lockDescription?: string;
}

export default function LockedPanel(props: LockedPanelProps) {
  const {
    lockDescription,
  } = props;

  return (
    <div className="login-step-panel login-step-panel-centered">
      <div className="login-step-icon login-step-icon-danger">
        <LockOutlined />
      </div>
      <Title level={4} className="login-step-title">
        账号已临时冻结
      </Title>
      <Text className="login-step-description">
        系统检测到连续异常登录尝试。锁定期间不会继续发起动态验证码验证，请等待自动解锁后再重试，或联系管理员处理。
      </Text>
      {lockDescription ? (
        <div className="login-lock-meta">
          <Text className="login-lock-text">{lockDescription}</Text>
        </div>
      ) : null}
    </div>
  );
}
