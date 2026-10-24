'use client';

import {
  CheckCircleOutlined,
  CopyOutlined,
  GithubOutlined,
  GoogleOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import { css } from '@emotion/css';
import { Alert, Button, ConfigProvider, Space, Spin, Typography, message } from 'antd';
import { AnimatePresence, motion } from 'framer-motion';
import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { completeExternalLoginCallback } from '@/api/externalLoginController';

const { Paragraph, Text, Title } = Typography;

const SUPPORTED_PROVIDERS: Record<string, { label: string; icon: React.ReactNode }> = {
  github: { label: 'GitHub', icon: <GithubOutlined /> },
  google: { label: 'Google', icon: <GoogleOutlined /> },
};

type LandingStage = 'processing' | 'success' | 'error';

type DiagnosticPayload = {
  code?: number;
  message?: string;
  traceId?: string;
  providerCode?: string;
  stage?: string;
  data?: unknown;
};

const processedCallbackKeys = new Set<string>();

type CallbackSnapshot = {
  code: string;
  error: string;
  issuer: string | null;
  providerCode: string;
  state: string;
};

let activeCallbackSnapshot: CallbackSnapshot | null = null;

function sanitizeProviderCode(value?: string) {
  return (value || '').trim().toLowerCase();
}

function readErrorPayload(error: unknown): Partial<DiagnosticPayload> {
  const candidate = error as {
    code?: number;
    message?: string;
    payload?: Partial<DiagnosticPayload>;
    response?: { data?: Partial<DiagnosticPayload> };
  };
  return candidate.payload || candidate.response?.data || candidate || {};
}

function buildDiagnostic(payload: Partial<DiagnosticPayload>, providerCode: string, stage: string): DiagnosticPayload {
  return {
    code: typeof payload.code === 'number' ? payload.code : undefined,
    message: payload.message || '外部登录回调失败',
    traceId: payload.traceId,
    providerCode,
    stage,
    data: payload.data,
  };
}

function stringifyDiagnostic(diagnostic: DiagnosticPayload | null) {
  if (!diagnostic) {
    return '';
  }
  return JSON.stringify(diagnostic, null, 2);
}

function clearSensitiveQuery() {
  if (typeof window === 'undefined') {
    return;
  }
  window.history.replaceState(null, document.title, `${window.location.pathname}${window.location.hash}`);
}

function providerIcon(providerCode: string) {
  return SUPPORTED_PROVIDERS[providerCode]?.icon || <SafetyCertificateOutlined />;
}

function readCallbackSnapshot(providerCode: string): CallbackSnapshot {
  const query = new URLSearchParams(window.location.search);
  const code = query.get('code')?.trim() || '';
  const state = query.get('state')?.trim() || '';
  const issuer = query.get('issuer');
  const error = query.get('error_description') || query.get('error') || '';
  if (code || state || error) {
    activeCallbackSnapshot = { code, error, issuer, providerCode, state };
    return activeCallbackSnapshot;
  }
  if (activeCallbackSnapshot?.providerCode === providerCode) {
    return activeCallbackSnapshot;
  }
  return { code: '', error: '', issuer: null, providerCode, state: '' };
}

export default function OAuthLandingPage() {
  const navigate = useNavigate();
  const params = useParams();
  const providerCode = sanitizeProviderCode(params.providerCode);
  const provider = SUPPORTED_PROVIDERS[providerCode];
  const [stage, setStage] = useState<LandingStage>('processing');
  const [statusText, setStatusText] = useState('正在建立安全登录通道...');
  const [diagnostic, setDiagnostic] = useState<DiagnosticPayload | null>(null);
  const handledRef = useRef(false);

  const diagnosticJson = useMemo(() => stringifyDiagnostic(diagnostic), [diagnostic]);

  const pageClassName = css({
    position: 'relative',
    minHeight: '100vh',
    overflow: 'hidden',
    background: '#f8fafc',
    padding: '16px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#0f172a',
    '.oauth-card': {
      width: '100%',
      maxWidth: '560px',
      position: 'relative',
      zIndex: 1,
      background: 'rgba(255,255,255,0.68)',
      backdropFilter: 'blur(24px)',
      WebkitBackdropFilter: 'blur(24px)',
      border: '1px solid rgba(255,255,255,0.5)',
      boxShadow: '0 24px 72px rgba(31, 38, 135, 0.12)',
      borderRadius: '28px',
      padding: '36px 36px 32px',
      '@media (max-width: 576px)': {
        padding: '28px 20px 24px',
        borderRadius: '24px',
      },
    },
    '.oauth-brand': {
      display: 'flex',
      gap: '14px',
      alignItems: 'center',
      marginBottom: '28px',
    },
    '.oauth-logo': {
      width: '48px',
      height: '48px',
      borderRadius: '14px',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(135deg, #3b82f6 0%, #22d3ee 100%)',
      color: '#fff',
      fontSize: '24px',
      boxShadow: '0 16px 36px rgba(14, 165, 233, 0.26)',
      flex: '0 0 auto',
    },
    '.oauth-title.ant-typography': {
      margin: 0,
      color: '#1e293b',
      fontSize: '28px',
      fontWeight: 700,
      lineHeight: 1.15,
    },
    '.oauth-subtitle.ant-typography': {
      color: '#64748b',
      fontSize: '14px',
      marginBottom: 0,
    },
    '.oauth-provider': {
      display: 'flex',
      justifyContent: 'center',
      marginBottom: '24px',
    },
    '.oauth-provider-icon': {
      position: 'relative',
      width: '76px',
      height: '76px',
      borderRadius: '24px',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'rgba(255,255,255,0.78)',
      border: '1px solid rgba(226,232,240,0.88)',
      boxShadow: '0 18px 48px rgba(14, 165, 233, 0.16)',
      color: '#1e293b',
      fontSize: '34px',
    },
    '.oauth-provider-icon::after': {
      content: '""',
      position: 'absolute',
      inset: '-8px',
      borderRadius: '30px',
      border: '1px solid rgba(34, 211, 238, 0.5)',
      animation: 'oauthPulse 1.8s ease-out infinite',
    },
    '@keyframes oauthPulse': {
      '0%': { transform: 'scale(0.92)', opacity: 0.78 },
      '100%': { transform: 'scale(1.18)', opacity: 0 },
    },
    '.oauth-status': {
      textAlign: 'center',
      marginBottom: '22px',
    },
    '.oauth-status-title.ant-typography': {
      marginBottom: '8px',
      color: '#1e293b',
      fontSize: '20px',
      fontWeight: 700,
    },
    '.oauth-status-text.ant-typography': {
      color: '#64748b',
      fontSize: '14px',
      marginBottom: 0,
    },
    '.oauth-spinner': {
      display: 'flex',
      justifyContent: 'center',
      marginBottom: '8px',
    },
    '.oauth-alert.ant-alert': {
      borderRadius: '16px',
      background: 'rgba(254, 242, 242, 0.88)',
      border: '1px solid rgba(254, 202, 202, 0.9)',
      marginBottom: '18px',
    },
    '.oauth-diagnostic': {
      maxHeight: '260px',
      overflow: 'auto',
      borderRadius: '16px',
      background: 'rgba(15, 23, 42, 0.94)',
      color: '#dbeafe',
      fontSize: '12px',
      lineHeight: 1.65,
      padding: '16px',
      marginBottom: '18px',
      whiteSpace: 'pre-wrap',
      wordBreak: 'break-word',
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
    },
    '.oauth-actions': {
      display: 'flex',
      justifyContent: 'center',
      flexWrap: 'wrap',
      gap: '12px',
    },
    '.oauth-primary.ant-btn': {
      minWidth: '136px',
      height: '44px',
      borderRadius: '14px',
      border: 'none',
      background: 'linear-gradient(90deg, #3b82f6 0%, #22d3ee 100%)',
      boxShadow: '0 16px 36px rgba(14, 165, 233, 0.22)',
    },
    '.oauth-secondary.ant-btn': {
      minWidth: '136px',
      height: '44px',
      borderRadius: '14px',
      background: 'rgba(255,255,255,0.82)',
      borderColor: 'rgba(203,213,225,0.9)',
    },
    '.oauth-grid-bg': {
      position: 'absolute',
      inset: 0,
      opacity: 0.48,
      backgroundImage:
        'linear-gradient(rgba(15,23,42,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(15,23,42,0.03) 1px, transparent 1px)',
      backgroundSize: '40px 40px',
      pointerEvents: 'none',
    },
  });

  const blobClassName = css({
    position: 'absolute',
    borderRadius: '999px',
    filter: 'blur(120px)',
    opacity: 0.22,
    mixBlendMode: 'multiply' as const,
    animation: 'oauthBlob 7s infinite',
    '@keyframes oauthBlob': {
      '0%': { transform: 'translate(0px, 0px) scale(1)' },
      '33%': { transform: 'translate(30px, -50px) scale(1.08)' },
      '66%': { transform: 'translate(-24px, 24px) scale(0.94)' },
      '100%': { transform: 'translate(0px, 0px) scale(1)' },
    },
  });

  useEffect(() => {
    const run = async () => {
      let callbackKey = '';
      try {
        if (!provider) {
          throw new Error('不支持的外部登录方式');
        }
        const callback = readCallbackSnapshot(providerCode);
        const { code, state, issuer } = callback;
        const protocolError = callback.error;
        callbackKey = `${providerCode}:${code}:${state}`;
        clearSensitiveQuery();

        if (protocolError) {
          throw new Error(protocolError);
        }
        if (!code || !state) {
          throw new Error('OAuth 回调参数不完整');
        }
        if (handledRef.current || processedCallbackKeys.has(callbackKey)) {
          return;
        }
        handledRef.current = true;
        processedCallbackKeys.add(callbackKey);

        setStatusText(`正在验证 ${provider.label} 账号...`);
        const result = await completeExternalLoginCallback(providerCode, { code, state, issuer });
        if (result?.outcome === 'BOUND') {
          setStage('success');
          setStatusText('账号绑定完成，正在返回账户设置...');
          window.setTimeout(() => {
            window.location.replace(result.redirectUrl || '/account/settings');
          }, 240);
          return;
        }
        if (!result?.authenticated || !result.redirectUrl) {
          throw new Error('外部登录结果不完整');
        }
        setStage('success');
        setStatusText('登录成功，正在跳转...');
        window.setTimeout(() => {
          window.location.replace(result.redirectUrl || '/');
        }, 360);
      } catch (error) {
        if (callbackKey) {
          processedCallbackKeys.delete(callbackKey);
        }
        const payload = readErrorPayload(error);
        const messageText = payload.message || (error as { message?: string })?.message || '外部登录回调失败';
        setStage('error');
        setStatusText('外部登录未完成');
        setDiagnostic(buildDiagnostic({ ...payload, message: messageText }, providerCode || 'unknown', 'callback'));
      }
    };

    void run();
  }, [provider, providerCode]);

  const copyDiagnostic = async () => {
    if (!diagnosticJson) {
      return;
    }
    await navigator.clipboard.writeText(diagnosticJson);
    message.success('诊断信息已复制');
  };

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#3b82f6',
          borderRadius: 16,
        },
      }}
    >
      <div className={pageClassName}>
        <div
          className={blobClassName}
          style={{
            width: '40vw',
            height: '40vw',
            minWidth: 280,
            minHeight: 280,
            top: '-10%',
            left: '-10%',
            background: 'rgba(59, 130, 246, 1)',
          }}
        />
        <div
          className={blobClassName}
          style={{
            width: '35vw',
            height: '35vw',
            minWidth: 260,
            minHeight: 260,
            top: '20%',
            right: '-10%',
            background: 'rgba(103, 232, 249, 1)',
            animationDelay: '2s',
          }}
        />
        <div className="oauth-grid-bg" />

        <motion.div
          className="oauth-card"
          initial={{ opacity: 0, y: 18, scale: 0.985 }}
          animate={{ opacity: 1, y: 0, scale: 1 }}
          transition={{ duration: 0.28, ease: 'easeOut' }}
        >
          <div className="oauth-brand">
            <div className="oauth-logo">
              <SafetyCertificateOutlined />
            </div>
            <div>
              <Title level={1} className="oauth-title">
                Seven
              </Title>
              <Paragraph className="oauth-subtitle">统一身份认证系统</Paragraph>
            </div>
          </div>

          <AnimatePresence mode="wait">
            {stage === 'processing' ? (
              <motion.div
                key="processing"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={{ duration: 0.22 }}
              >
                <div className="oauth-provider">
                  <div className="oauth-provider-icon">{providerIcon(providerCode)}</div>
                </div>
                <div className="oauth-status">
                  <Title level={2} className="oauth-status-title">
                    {provider?.label || 'OAuth'} 登录验证
                  </Title>
                  <Paragraph className="oauth-status-text">{statusText}</Paragraph>
                </div>
                <div className="oauth-spinner">
                  <Spin />
                </div>
              </motion.div>
            ) : null}

            {stage === 'success' ? (
              <motion.div
                key="success"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={{ duration: 0.22 }}
              >
                <div className="oauth-provider">
                  <div className="oauth-provider-icon">
                    <CheckCircleOutlined style={{ color: '#16a34a' }} />
                  </div>
                </div>
                <div className="oauth-status">
                  <Title level={2} className="oauth-status-title">
                    登录成功
                  </Title>
                  <Paragraph className="oauth-status-text">{statusText}</Paragraph>
                </div>
              </motion.div>
            ) : null}

            {stage === 'error' ? (
              <motion.div
                key="error"
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -10 }}
                transition={{ duration: 0.22 }}
              >
                <div className="oauth-provider">
                  <div className="oauth-provider-icon">
                    <WarningOutlined style={{ color: '#ef4444' }} />
                  </div>
                </div>
                <Alert
                  className="oauth-alert"
                  type="error"
                  showIcon
                  title="外部登录失败"
                  description={diagnostic?.message || '外部登录回调失败'}
                />
                {diagnosticJson ? <pre className="oauth-diagnostic">{diagnosticJson}</pre> : null}
                <div className="oauth-actions">
                  <Space wrap>
                    <Button
                      className="oauth-primary"
                      type="primary"
                      icon={<ReloadOutlined />}
                      onClick={() => navigate('/login', { replace: true })}
                    >
                      重新登录
                    </Button>
                    <Button
                      className="oauth-secondary"
                      icon={<CopyOutlined />}
                      onClick={() => void copyDiagnostic()}
                    >
                      复制诊断信息
                    </Button>
                  </Space>
                </div>
              </motion.div>
            ) : null}
          </AnimatePresence>

          <div style={{ marginTop: 24, textAlign: 'center' }}>
            <Text type="secondary">请勿关闭当前页面</Text>
          </div>
        </motion.div>
      </div>
    </ConfigProvider>
  );
}
