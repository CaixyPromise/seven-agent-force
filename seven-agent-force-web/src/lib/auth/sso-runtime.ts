import type { SsoRuntimeConfig } from '@/lib/http/types';

const DEFAULT_RUNTIME_CONFIG: SsoRuntimeConfig = {
  enabled: false,
  frontendPrimaryEnabled: false,
  resourceServerEnabled: false,
  issuer: '',
  defaultFirstPartyClientId: 'authorization-console',
};

let runtimeConfigPromise: Promise<SsoRuntimeConfig> | null = null;
let cachedRuntimeConfig: SsoRuntimeConfig | null = null;

function normalizeRuntimeConfig(raw: unknown): SsoRuntimeConfig {
  if (raw && typeof raw === 'object' && 'data' in (raw as Record<string, unknown>)) {
    return normalizeRuntimeConfig((raw as { data?: unknown }).data);
  }
  if (!raw || typeof raw !== 'object') {
    return DEFAULT_RUNTIME_CONFIG;
  }
  const source = raw as Record<string, unknown>;
  return {
    enabled: Boolean(source.enabled),
    frontendPrimaryEnabled: Boolean(source.frontendPrimaryEnabled),
    resourceServerEnabled: Boolean(source.resourceServerEnabled),
    issuer: typeof source.issuer === 'string' ? source.issuer : '',
    defaultFirstPartyClientId:
      typeof source.defaultFirstPartyClientId === 'string' && source.defaultFirstPartyClientId
        ? source.defaultFirstPartyClientId
        : 'authorization-console',
  };
}

export function clearSsoRuntimeConfigCache() {
  runtimeConfigPromise = null;
  cachedRuntimeConfig = null;
}

export async function getSsoRuntimeConfig(): Promise<SsoRuntimeConfig> {
  if (cachedRuntimeConfig) {
    return cachedRuntimeConfig;
  }
  if (runtimeConfigPromise) {
    return runtimeConfigPromise;
  }
  runtimeConfigPromise = fetch('/api/sso/runtime/config', {
    method: 'GET',
    credentials: 'include',
    headers: {
      Accept: 'application/json',
    },
  })
    .then(async (response) => {
      if (!response.ok) {
        throw new Error(`Failed to load SSO runtime config: ${response.status}`);
      }
      return normalizeRuntimeConfig(await response.json());
    })
    .then((config) => {
      cachedRuntimeConfig = config;
      return config;
    })
    .catch(() => DEFAULT_RUNTIME_CONFIG)
    .finally(() => {
      runtimeConfigPromise = null;
    });
  return runtimeConfigPromise;
}

export function getDefaultSsoRuntimeConfig(): SsoRuntimeConfig {
  return DEFAULT_RUNTIME_CONFIG;
}
