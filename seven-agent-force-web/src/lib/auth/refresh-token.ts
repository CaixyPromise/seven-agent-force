const SSO_REFRESH_TOKEN_KEY = 'seven:sso:refresh-token';

function canUseSessionStorage() {
  return typeof window !== 'undefined' && typeof window.sessionStorage !== 'undefined';
}

export function clearStoredSsoRefreshToken() {
  if (!canUseSessionStorage()) {
    return;
  }
  window.sessionStorage.removeItem(SSO_REFRESH_TOKEN_KEY);
}
