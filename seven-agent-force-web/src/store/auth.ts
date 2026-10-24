import { create } from 'zustand';
import type { LoginUser } from '@/lib/http/types';
import { clearAuthModeHint, persistAuthModeHint } from '@/lib/auth/session-hint';
import { resolveAccessExpireAt } from '@/lib/auth/token';
import { clearStoredSsoRefreshToken } from '@/lib/auth/refresh-token';

export type AuthMode = 'sso' | null;

interface AuthState {
  user: LoginUser | null;
  accessToken: string | null;
  tokenType: string | null;
  accessExpireAt: number | null;
  authMode: AuthMode;
  setUser: (user: LoginUser | null) => void;
  applyAccessToken: (payload: {
    accessToken?: string | null;
    tokenType?: string | null;
    accessTtlSec?: number | null;
  }) => void;
  clearUser: () => void;
  clearSession: () => void;
}

export const useAuthStore = create<AuthState>((set) => ({
  user: null,
  accessToken: null,
  tokenType: null,
  accessExpireAt: null,
  authMode: null,
  setUser: (user) => set({ user }),
  applyAccessToken: (payload) => {
    const authMode = payload.accessToken ? 'sso' : null;
    persistAuthModeHint(authMode);
    set({
      accessToken: payload.accessToken ?? null,
      tokenType: payload.accessToken ? (payload.tokenType ?? 'Bearer') : null,
      accessExpireAt: payload.accessToken
        ? resolveAccessExpireAt(payload.accessToken, payload.accessTtlSec)
        : null,
      authMode,
    });
  },
  clearUser: () => set({ user: null }),
  clearSession: () => {
    clearAuthModeHint();
    clearStoredSsoRefreshToken();
    set({
      user: null,
      accessToken: null,
      tokenType: null,
      accessExpireAt: null,
      authMode: null,
    });
  },
}));
