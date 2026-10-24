'use client';

import { useCallback, useEffect } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { refreshAccessToken } from '@/api/request';
import {
  fetchCurrentUser,
  normalizeCurrentUser,
} from '@/services/auth';
import { hasPermission as matchPermission } from '@/lib/auth/permissions';
import { broadcastAuthSessionEvent } from '@/lib/auth/session-sync';
import { readAuthModeHint } from '@/lib/auth/session-hint';
import { useAuthStore } from '@/store/auth';
import { ssoLogout } from '@/api/ssoController';

export const CURRENT_USER_QUERY_KEY = ['auth', 'currentUser'];
export const AUTH_MENUS_QUERY_KEY = ['auth', 'menus'];

export function useCurrentUser(enabled = true) {
  const setUser = useAuthStore((state) => state.setUser);
  const clearSession = useAuthStore((state) => state.clearSession);

  const shouldFetch = enabled && typeof window !== 'undefined';

  const query = useQuery({
    queryKey: CURRENT_USER_QUERY_KEY,
    queryFn: async () => {
      try {
        const authState = useAuthStore.getState();
        if (!authState.accessToken) {
          const authModeHint = readAuthModeHint();
          if (authModeHint !== 'sso') {
            return null;
          }
          await refreshAccessToken();
        }

        const { data } = await fetchCurrentUser({ skipAuthRedirect: true });
        const normalized = data ? normalizeCurrentUser(data) : null;
        setUser(normalized);
        return normalized;
      } catch (error) {
        clearSession();
        throw error;
      }
    },
    enabled: shouldFetch,
    retry: 0,
    staleTime: 5 * 60 * 1000,
  });

  useEffect(() => {
    if (!shouldFetch) {
      return;
    }
    if (query.data === undefined) {
      return;
    }
    setUser(query.data ?? null);
  }, [query.data, setUser, shouldFetch]);

  return query;
}

export function useLogoutMutation() {
  const queryClient = useQueryClient();
  const clearSession = useAuthStore((state) => state.clearSession);

  return useMutation({
    mutationKey: ['auth', 'logout'],
    mutationFn: async () => {
      let remoteSuccess = true;
      try {
        const { accessToken } = useAuthStore.getState();
        await ssoLogout(accessToken);
      } catch (error) {
        remoteSuccess = false;
        console.warn('Remote logout failed, local session has been cleared.', error);
      } finally {
        clearSession();
        broadcastAuthSessionEvent({
          type: 'logout',
          at: Date.now(),
        });
      }
      return { remoteSuccess };
    },
    onSuccess: () => {
      queryClient.removeQueries({ queryKey: CURRENT_USER_QUERY_KEY });
    },
  });
}

export function useAuthorization() {
  const user = useAuthStore((state) => state.user);

  const hasPermission = useCallback(
    (permission: string) => matchPermission(user?.permissions, permission),
    [user?.permissions],
  );

  const hasRole = useCallback(
    (role: string) => user?.roleCodes?.includes(role) ?? false,
    [user?.roleCodes],
  );

  return {
    user,
    isLoggedIn: !!user,
    hasPermission,
    hasRole,
  };
}
