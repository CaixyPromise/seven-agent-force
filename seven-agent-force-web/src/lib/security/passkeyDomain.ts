'use client';

function readCurrentHostName() {
  if (typeof window === 'undefined' || !window.location?.hostname) {
    return '';
  }
  return window.location.hostname.trim().toLowerCase();
}

export function shouldSwitchPasskeyToLocalhostDomain() {
  const hostName = readCurrentHostName();
  return hostName === '127.0.0.1' || hostName === '::1' || hostName === '[::1]';
}

export function buildPasskeyLocalhostUrl(
  nextSearchParams: Record<string, string | null | undefined>,
) {
  const currentUrl = new URL(window.location.href);
  currentUrl.hostname = 'localhost';
  Object.entries(nextSearchParams).forEach(([key, value]) => {
    const normalizedValue = typeof value === 'string' ? value.trim() : '';
    if (normalizedValue) {
      currentUrl.searchParams.set(key, normalizedValue);
      return;
    }
    currentUrl.searchParams.delete(key);
  });
  return currentUrl.toString();
}
