export const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL ?? '/api';

export const API_TIMEOUT = Number(
  import.meta.env.VITE_API_TIMEOUT ?? 30000,
);

export const ROUTE_WITHOUT_LAYOUT = new Set<string>(['/login', '/setup', '/passkey-bridge']);
export const ROUTE_PREFIXES_WITHOUT_LAYOUT = ['/oidc/callback/', '/oauth/landing/'];

export function isRouteWithoutLayout(pathname: string | null | undefined) {
  if (!pathname) {
    return false;
  }
  if (ROUTE_WITHOUT_LAYOUT.has(pathname)) {
    return true;
  }
  return ROUTE_PREFIXES_WITHOUT_LAYOUT.some((prefix) => pathname.startsWith(prefix));
}

export const LOGIN_SUCCESS_REDIRECT = '/';
