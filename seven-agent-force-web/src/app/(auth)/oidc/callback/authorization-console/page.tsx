'use client';

import { Alert, Spin, message } from 'antd';
import React, { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';

import { exchangeOidcToken, ssoLogout } from '@/api/ssoController';
import { CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { resolveSafePostLoginTarget } from '@/lib/auth/navigation';
import { clearPkceSession, readPkceSession } from '@/lib/auth/oidc';
import { parseJwtPayload } from '@/lib/auth/token';
import { useAuthStore } from '@/store/auth';
import { fetchCurrentUser, normalizeCurrentUser } from '@/services/auth';

const processedCallbackKeys = new Set<string>();

function validateNonce(idToken: string | undefined, expectedNonce: string) {
  if (!idToken) {
    throw new Error('OIDC id_token 缺失');
  }
  const payload = parseJwtPayload(idToken);
  if (payload?.nonce !== expectedNonce) {
    throw new Error('OIDC nonce 校验失败，请重新登录');
  }
}

export default function AuthorizationConsoleOidcCallbackPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const applyAccessToken = useAuthStore((state) => state.applyAccessToken);
  const setUser = useAuthStore((state) => state.setUser);
  const clearSession = useAuthStore((state) => state.clearSession);
  const [errorMessage, setErrorMessage] = useState<string>('');
  const handledRef = useRef(false);

  useEffect(() => {
    const run = async () => {
      const clientId = 'authorization-console';
      let callbackKey = '';
      let issuedAccessToken: string | null = null;
      try {
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        const state = params.get('state');
        const protocolError = params.get('error_description') || params.get('error');

        if (protocolError) {
          throw new Error(protocolError);
        }
        if (!code || !state) {
          throw new Error('OIDC 回调参数不完整');
        }
        callbackKey = `${clientId}:${code}:${state}`;
        if (handledRef.current || processedCallbackKeys.has(callbackKey)) {
          return;
        }
        handledRef.current = true;
        processedCallbackKeys.add(callbackKey);

        const pkceSession = readPkceSession(clientId);
        if (!pkceSession) {
          throw new Error('PKCE 会话不存在或已失效，请重新登录');
        }
        if (pkceSession.state !== state) {
          throw new Error('OIDC state 校验失败，请重新登录');
        }

        const tokenResult = await exchangeOidcToken({
          grant_type: 'authorization_code',
          client_id: clientId,
          code,
          redirect_uri: pkceSession.redirectUri,
          code_verifier: pkceSession.codeVerifier,
        });
        issuedAccessToken = tokenResult.accessToken ?? null;
        validateNonce(tokenResult.idToken, pkceSession.nonce);
        applyAccessToken(tokenResult);

        const currentUserResponse = await fetchCurrentUser({ skipAuthRedirect: true });
        setUser(currentUserResponse?.data ? normalizeCurrentUser(currentUserResponse.data) : null);
        queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY });

        clearPkceSession(clientId);
        const target = resolveSafePostLoginTarget(pkceSession.postLoginRedirect, '/');
        navigate(target, { replace: true });
      } catch (error) {
        if (issuedAccessToken) {
          try {
            await ssoLogout(issuedAccessToken);
          } catch {
            // ignore best-effort remote cleanup failures
          }
        }
        clearSession();
        clearPkceSession(clientId);
        if (callbackKey) {
          processedCallbackKeys.delete(callbackKey);
        }
        const nextMessage = (error as { message?: string })?.message || '登录回调失败';
        setErrorMessage(nextMessage);
        message.error(nextMessage);
      }
    };
    run();
  }, [applyAccessToken, clearSession, navigate, queryClient, setUser]);

  if (errorMessage) {
    return (
      <div className="flex min-h-screen items-center justify-center p-6">
        <Alert
          type="error"
          showIcon
          title="登录回调失败"
          description={errorMessage}
        />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <Spin size="large" description="正在完成登录..." />
    </div>
  );
}
