'use client';

import { PropsWithChildren, useEffect, useMemo, useRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Spin } from 'antd';
import { useCurrentUser } from '@/hooks/useAuth';
import { buildLoginRedirectUrl, buildSetupRedirectUrl } from '@/lib/auth/navigation';
import { requiresAuth } from '@/lib/auth/routes';
import { InitialStateContext } from '@/components/providers/InitialStateContext';
import { useAuthStore } from '@/store/auth';
import { readAuthModeHint } from '@/lib/auth/session-hint';
import { getSetupStatusApi } from '@/api/setupController';
import { useState } from 'react';

export default function InitialStateProvider({ children }: PropsWithChildren) {
  const location = useLocation();
  const navigate = useNavigate();
  const pathname = location.pathname;
  const authRequired = requiresAuth(pathname);
  const storeUser = useAuthStore((state) => state.user);
  const accessToken = useAuthStore((state) => state.accessToken);
  const { data: queriedUser, isFetched, isLoading, isFetching } = useCurrentUser(authRequired);
  const isClient = typeof window !== 'undefined';
  const authModeHint = isClient ? readAuthModeHint() : null;
  const currentUser = queriedUser ?? storeUser ?? null;
  const [checkingSetup, setCheckingSetup] = useState(false);
  const protectedReturnTargetRef = useRef<string | null>(null);
  const waitingForAuthenticatedBootstrap =
    authRequired &&
    !queriedUser &&
    (!isFetched || isLoading || isFetching || !!accessToken || !!authModeHint);

  useEffect(() => {
    if (currentUser) {
      protectedReturnTargetRef.current = null;
    }
  }, [currentUser]);

  useEffect(() => {
    let cancelled = false;
    if (!isClient) return;
    if (!authRequired) return;
    if (waitingForAuthenticatedBootstrap) return;
    if (currentUser) return;
    const protectedReturnTarget =
      protectedReturnTargetRef.current ??
      `${location.pathname}${location.search}${location.hash}`;
    protectedReturnTargetRef.current = protectedReturnTarget;
    void Promise.resolve().then(async () => {
      if (cancelled) {
        return;
      }
      setCheckingSetup(true);
      try {
        const response = await getSetupStatusApi();
        if (cancelled) {
          return;
        }
        if (!response.data?.initialized) {
          navigate(buildSetupRedirectUrl(protectedReturnTarget), { replace: true });
          return;
        }
        navigate(buildLoginRedirectUrl(protectedReturnTarget), { replace: true });
      } catch {
        if (cancelled) {
          return;
        }
        navigate(buildLoginRedirectUrl(protectedReturnTarget), { replace: true });
      } finally {
        if (!cancelled) {
          setCheckingSetup(false);
        }
      }
    });
    return () => {
      cancelled = true;
    };
  }, [
    isClient,
    authRequired,
    waitingForAuthenticatedBootstrap,
    currentUser,
    location.hash,
    location.pathname,
    location.search,
    navigate,
  ]);

  const value = useMemo(
    () => ({
      currentUser,
      loading: authRequired && (waitingForAuthenticatedBootstrap || checkingSetup) && isClient,
    }),
    [authRequired, checkingSetup, currentUser, waitingForAuthenticatedBootstrap, isClient],
  );

  // 服务端渲染时不显示加载状态，避免水合不匹配
  if (!isClient || value.loading) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100vh',
        }}
      >
        <Spin size="large" />
      </div>
    );
  }

  return (
    <InitialStateContext.Provider value={value}>
      {children}
    </InitialStateContext.Provider>
  );
}
