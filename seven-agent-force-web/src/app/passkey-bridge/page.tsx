'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Spin } from 'antd';
import { createPasskeyAssertionPayload, createPasskeyRegistrationPayload } from '@/lib/security/webauthn';
import {
  isPasskeyBridgeMessage,
  passkeyBridgeEvents,
  type PasskeyBridgeMode,
} from '@/lib/security/passkeyBridge';

type BridgeState = 'booting' | 'ready' | 'working' | 'failed';

interface BridgeRequestPayload {
  mode?: PasskeyBridgeMode;
  hints?: Record<string, unknown>;
  userName?: string;
  displayName?: string;
}

export default function PasskeyBridgePage() {
  const [state, setState] = useState<BridgeState>('booting');
  const [errorMessage, setErrorMessage] = useState('');
  const requestStartedRef = useRef(false);

  const allowedOrigin = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    const currentUrl = new URL(window.location.href);
    return currentUrl.searchParams.get('origin')?.trim() || '';
  }, []);
  const invalidEntry = typeof window !== 'undefined' && (!window.opener || !allowedOrigin);

  useEffect(() => {
    if (typeof window === 'undefined' || invalidEntry) {
      return;
    }

    const notifyReady = () => {
      window.opener?.postMessage({ type: passkeyBridgeEvents.ready }, allowedOrigin);
      setState('ready');
    };

    const handleMessage = async (event: MessageEvent<unknown>) => {
      if (event.origin !== allowedOrigin || !isPasskeyBridgeMessage(event.data, passkeyBridgeEvents.request)) {
        return;
      }
      if (requestStartedRef.current) {
        return;
      }
      const request = (event.data as { payload?: BridgeRequestPayload }).payload;
      if (!request?.mode) {
        return;
      }
      requestStartedRef.current = true;

      setState('working');
      setErrorMessage('');

      try {
        const payload = request.mode === 'registration'
          ? await createPasskeyRegistrationPayload(
            request.hints,
            request.userName || 'user',
            request.displayName,
          )
          : await createPasskeyAssertionPayload(request.hints);
        window.opener?.postMessage(
          {
            type: passkeyBridgeEvents.result,
            payload,
          },
          allowedOrigin,
        );
        window.close();
      } catch (error) {
        const message = (error as { message?: string })?.message || 'Passkey 验证失败';
        requestStartedRef.current = false;
        setState('failed');
        setErrorMessage(message);
        window.opener?.postMessage(
          {
            type: passkeyBridgeEvents.error,
            message,
          },
          allowedOrigin,
        );
      }
    };

    window.addEventListener('message', handleMessage);
    notifyReady();
    return () => {
      window.removeEventListener('message', handleMessage);
    };
  }, [allowedOrigin, invalidEntry]);

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
        background:
          'radial-gradient(circle at top, rgba(56,189,248,0.18), transparent 42%), linear-gradient(180deg, #f8fbff 0%, #eef7ff 100%)',
      }}
    >
      <div
        style={{
          width: 'min(420px, 100%)',
          borderRadius: 24,
          background: 'rgba(255,255,255,0.96)',
          border: '1px solid rgba(147,197,253,0.35)',
          boxShadow: '0 28px 72px rgba(14,165,233,0.14)',
          padding: 32,
          textAlign: 'center',
        }}
      >
        <div
          style={{
            width: 88,
            height: 88,
            borderRadius: '50%',
            margin: '0 auto 20px',
            background: 'linear-gradient(135deg, rgba(59,130,246,0.14), rgba(34,211,238,0.2))',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: '#0284c7',
            fontSize: 36,
            fontWeight: 700,
          }}
        >
          PK
        </div>
        <h1
          style={{
            margin: 0,
            fontSize: 28,
            fontWeight: 700,
            color: '#0f172a',
          }}
        >
          Passkey 验证
        </h1>
        <p
          style={{
            margin: '12px 0 0',
            fontSize: 15,
            lineHeight: 1.8,
            color: '#64748b',
          }}
        >
          {state === 'working'
            ? '请在系统弹层中完成 Passkey 验证。'
            : '桥接窗口已就绪，将在收到请求后立即调起系统验证。'}
        </p>

        <div style={{ marginTop: 24 }}>
          {(state === 'booting' || state === 'ready' || state === 'working') && (
            <Spin size="large" />
          )}
          {(state === 'failed' || invalidEntry) && (
            <Alert
              type="error"
              showIcon
              message={errorMessage || 'Passkey 桥接窗口未通过合法入口打开。'}
              style={{ textAlign: 'left', borderRadius: 14 }}
            />
          )}
        </div>
      </div>
    </div>
  );
}
