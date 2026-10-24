const FIVE_MINUTES_MS = 5 * 60 * 1000;

function decodeBase64Url(value: string): string {
  const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
  const padded = normalized + '='.repeat((4 - (normalized.length % 4)) % 4);
  if (typeof window !== 'undefined' && typeof window.atob === 'function') {
    return window.atob(padded);
  }
  throw new Error('base64url decoding requires a browser runtime');
}

export function parseJwtPayload(token: string | null | undefined): Record<string, unknown> | null {
  if (!token) {
    return null;
  }
  const parts = token.split('.');
  if (parts.length < 2 || !parts[1]) {
    return null;
  }
  try {
    return JSON.parse(decodeBase64Url(parts[1])) as Record<string, unknown>;
  } catch {
    return null;
  }
}

export function parseJwtExpireAt(token: string | null | undefined): number | null {
  const payload = parseJwtPayload(token) as { exp?: number } | null;
  if (!payload?.exp || Number.isNaN(payload.exp)) {
    return null;
  }
  return payload.exp * 1000;
}

export function resolveAccessExpireAt(
  accessToken: string | null | undefined,
  accessTtlSec: number | null | undefined,
): number | null {
  if (typeof accessTtlSec === 'number' && !Number.isNaN(accessTtlSec)) {
    return Date.now() + accessTtlSec * 1000;
  }
  return parseJwtExpireAt(accessToken);
}

export function shouldRefreshAccessToken(accessExpireAt: number | null | undefined): boolean {
  if (typeof accessExpireAt !== 'number' || Number.isNaN(accessExpireAt)) {
    return false;
  }
  return accessExpireAt - Date.now() <= FIVE_MINUTES_MS;
}

export function buildAuthorizationHeader(
  accessToken: string | null | undefined,
  tokenType: string | null | undefined,
): string | null {
  if (!accessToken) {
    return null;
  }
  return `${tokenType ?? 'Bearer'} ${accessToken}`;
}

export function isSsoAccessToken(accessToken: string | null | undefined): boolean {
  const payload = parseJwtPayload(accessToken);
  return typeof payload?.iss === 'string' && payload.iss.includes('/sso');
}
