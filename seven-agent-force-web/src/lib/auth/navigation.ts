import { isPublicRoute } from '@/lib/auth/routes';

function toRelativeTarget(url: URL) {
  return `${url.pathname}${url.search}${url.hash}` || '/';
}

export function resolveSafePostLoginTarget(raw?: string | null, fallback = '/') {
  if (typeof window === 'undefined') {
    return fallback;
  }
  if (!raw) {
    return fallback;
  }

  try {
    const url = new URL(raw, window.location.origin);
    if (url.origin !== window.location.origin) {
      return fallback;
    }
    if (isPublicRoute(url.pathname)) {
      return fallback;
    }
    return toRelativeTarget(url);
  } catch {
    if (!raw.startsWith('/')) {
      return fallback;
    }
    const cleanPath = raw.split(/[?#]/)[0];
    return isPublicRoute(cleanPath) ? fallback : raw;
  }
}

export function buildLoginRedirectUrl(currentHref?: string | null, fallback = '/') {
  if (typeof window === 'undefined') {
    return '/login';
  }

  const target = resolveSafePostLoginTarget(currentHref || window.location.href, fallback);
  if (!target || target === '/login') {
    return '/login';
  }

  const params = new URLSearchParams();
  params.set('redirect', new URL(target, window.location.origin).toString());
  return `/login?${params.toString()}`;
}

export function buildSetupRedirectUrl(currentHref?: string | null, fallback = '/') {
  if (typeof window === 'undefined') {
    return '/setup';
  }

  const target = resolveSafePostLoginTarget(currentHref || window.location.href, fallback);
  if (!target || target === '/setup') {
    return '/setup';
  }

  const params = new URLSearchParams();
  params.set('redirect', new URL(target, window.location.origin).toString());
  return `/setup?${params.toString()}`;
}
