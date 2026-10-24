export const PUBLIC_ROUTES = new Set<string>([
  '/login',
  '/setup',
  '/passkey-bridge',
  '/oidc/callback/authorization-console',
]);

const PUBLIC_ROUTE_PREFIXES = [
  '/oauth/landing/',
  '/external-identities/oauth/callback/',
];

export function isPublicRoute(pathname: string | null | undefined) {
  if (!pathname) return false;
  const cleanPath = pathname.split('?')[0];
  return PUBLIC_ROUTES.has(cleanPath) || PUBLIC_ROUTE_PREFIXES.some((prefix) => cleanPath.startsWith(prefix));
}

export function requiresAuth(pathname: string | null | undefined) {
  return !isPublicRoute(pathname);
}
