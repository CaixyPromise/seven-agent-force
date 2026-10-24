import { getOrCreateDeviceId } from '@/lib/auth/device';
import { resolveSafePostLoginTarget } from '@/lib/auth/navigation';

export interface PkceSession {
  state: string;
  nonce: string;
  codeVerifier: string;
  codeChallenge: string;
  redirectUri: string;
  postLoginRedirect?: string;
}

const PKCE_STORAGE_PREFIX = 'seven:oidc:pkce:';

function randomBase64Url(length = 64): string {
  const bytes = new Uint8Array(length);
  crypto.getRandomValues(bytes);
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('');
  return btoa(binary)
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/g, '')
    .slice(0, length);
}

async function sha256Base64Url(value: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value));
  const bytes = new Uint8Array(digest);
  const binary = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('');
  return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

export async function createPkceSession(redirectUri: string): Promise<PkceSession> {
  const codeVerifier = randomBase64Url(64);
  return {
    state: randomBase64Url(32),
    nonce: randomBase64Url(32),
    codeVerifier,
    codeChallenge: await sha256Base64Url(codeVerifier),
    redirectUri,
  };
}

export function persistPkceSession(clientId: string, session: PkceSession) {
  sessionStorage.setItem(`${PKCE_STORAGE_PREFIX}${clientId}`, JSON.stringify(session));
}

export function readPkceSession(clientId: string): PkceSession | null {
  const raw = sessionStorage.getItem(`${PKCE_STORAGE_PREFIX}${clientId}`);
  if (!raw) {
    return null;
  }
  try {
    return JSON.parse(raw) as PkceSession;
  } catch {
    return null;
  }
}

export function clearPkceSession(clientId: string) {
  sessionStorage.removeItem(`${PKCE_STORAGE_PREFIX}${clientId}`);
}

export function buildAuthorizationParams(clientId: string, session: PkceSession, scope: string) {
  return {
    clientId,
    redirectUri: session.redirectUri,
    scope,
    state: session.state,
    nonce: session.nonce,
    codeChallenge: session.codeChallenge,
  };
}

export function resolveOidcCallbackRedirectUri() {
  return `${window.location.origin}/oidc/callback/authorization-console`;
}

export function resolveLoginRedirectTarget() {
  const params = new URLSearchParams(window.location.search);
  return resolveSafePostLoginTarget(params.get('redirect'), '/');
}

export function buildDeviceHeaders() {
  return {
    'X-Device-Id': getOrCreateDeviceId(),
  };
}
