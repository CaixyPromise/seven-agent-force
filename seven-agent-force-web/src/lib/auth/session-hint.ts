export type AuthModeHint = 'sso';

const AUTH_MODE_HINT_KEY = 'seven:auth:mode';

function canUseStorage() {
  return typeof window !== 'undefined' && typeof window.localStorage !== 'undefined';
}

export function readAuthModeHint(): AuthModeHint | null {
  if (!canUseStorage()) {
    return null;
  }
  const value = window.localStorage.getItem(AUTH_MODE_HINT_KEY);
  return value === 'sso' ? value : null;
}

export function persistAuthModeHint(mode: AuthModeHint | null | undefined) {
  if (!canUseStorage()) {
    return;
  }
  if (!mode) {
    window.localStorage.removeItem(AUTH_MODE_HINT_KEY);
    return;
  }
  window.localStorage.setItem(AUTH_MODE_HINT_KEY, mode);
}

export function clearAuthModeHint() {
  if (!canUseStorage()) {
    return;
  }
  window.localStorage.removeItem(AUTH_MODE_HINT_KEY);
}
