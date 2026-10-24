'use client';

import React from 'react';
import { WarningOutlined } from '@ant-design/icons';

interface FailureAlertProps {
  message?: string;
  remainingAttemptCount?: number | null;
  cooldownSeconds?: number | null;
  canSwitchMethod?: boolean;
}

function resolveFailureCopy(message?: string, cooldownSeconds?: number | null) {
  if (typeof cooldownSeconds === 'number' && cooldownSeconds > 0) {
    return {
      title: message || '验证请求过于频繁',
      description: '请等待冷却时间结束后再试。',
    };
  }
  return {
    title: message || '验证失败',
    description: '请检查输入后重试。',
  };
}

export function FailureAlert({
  message,
  remainingAttemptCount,
  cooldownSeconds,
  canSwitchMethod,
}: FailureAlertProps) {
  const copy = resolveFailureCopy(message, cooldownSeconds);

  return (
    <div style={{
      background: 'rgba(239,68,68,0.08)',
      border: '1px solid rgba(239,68,68,0.25)',
      borderRadius: 10,
      padding: '10px 14px',
      marginBottom: 16,
      display: 'flex',
      gap: 10,
      alignItems: 'flex-start',
    }}>
      <WarningOutlined style={{ fontSize: 16, flexShrink: 0, color: '#ea580c' }} />
      <div style={{ fontSize: 13, color: '#dc2626', lineHeight: 1.6 }}>
        <div style={{ fontWeight: 600, marginBottom: 2 }}>{copy.title}</div>
        {typeof remainingAttemptCount === 'number' && (
          <div>剩余尝试次数：<strong>{remainingAttemptCount}</strong></div>
        )}
        {typeof cooldownSeconds === 'number' && cooldownSeconds > 0 && (
          <div>冷却时间：<strong>{cooldownSeconds}</strong> 秒后可重试</div>
        )}
        {canSwitchMethod && (
          <div style={{ color: '#b45309' }}>你可以切换到其他验证方式继续操作。</div>
        )}
        {typeof remainingAttemptCount !== 'number' && !cooldownSeconds && !canSwitchMethod && (
          <div>{copy.description}</div>
        )}
      </div>
    </div>
  );
}
