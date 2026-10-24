'use client';

import { useEffect } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { useAuthStore } from '@/store/auth';
import { CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';
import { buildLoginRedirectUrl } from '@/lib/auth/navigation';
import { subscribeAuthSessionEvents } from '@/lib/auth/session-sync';
import { requiresAuth } from '@/lib/auth/routes';

export default function AuthSessionSyncBridge() {
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const clearSession = useAuthStore((state) => state.clearSession);

  useEffect(() => {
    return subscribeAuthSessionEvents((event) => {
      if (event.type !== 'logout') {
        return;
      }
      clearSession();
      queryClient.setQueryData(CURRENT_USER_QUERY_KEY, null);
      queryClient.removeQueries({ queryKey: CURRENT_USER_QUERY_KEY });
      if (requiresAuth(location.pathname)) {
        navigate(buildLoginRedirectUrl(window.location.href), { replace: true });
      }
    });
  }, [clearSession, location.pathname, navigate, queryClient]);

  return null;
}
