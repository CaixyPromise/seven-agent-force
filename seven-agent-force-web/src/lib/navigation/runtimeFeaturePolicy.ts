import type { RouteObject } from 'react-router-dom';
import type { AppRoute } from 'config/route';
import type {
  RuntimeFeatureCode,
  RuntimeFeatures,
  RuntimePlatformMode,
} from '@/lib/http/types';

export const RUNTIME_FEATURE_CODES = {
  PLATFORM_CONTROL: 'platform.control',
  FEDERATION_HUB: 'federation.hub',
  FEDERATION_NODE: 'federation.node',
  DOCKER_ADMIN: 'docker.admin',
} as const satisfies Record<string, RuntimeFeatureCode>;

export const SAFE_RUNTIME_FEATURES: RuntimeFeatures = {
  features: {
    enabled: [],
  },
  platform: {
    mode: 'local',
    capabilities: {
      controlPlane: false,
      federatedHubLogin: false,
      nodeApi: false,
    },
  },
  docker: {
    enabled: false,
  },
  notification: {
    managedByPlatform: false,
  },
  runtimeLog: {
    managedByPlatform: false,
  },
};

export interface RuntimeRouteFeatureManifestEntry {
  path: string;
  featureCode: RuntimeFeatureCode | null;
}

export const RUNTIME_ROUTE_FEATURE_MANIFEST: readonly RuntimeRouteFeatureManifestEntry[] = [
  { path: '/system/platform', featureCode: RUNTIME_FEATURE_CODES.PLATFORM_CONTROL },
  { path: '/system/hub-node', featureCode: RUNTIME_FEATURE_CODES.FEDERATION_HUB },
  { path: '/system/docker', featureCode: RUNTIME_FEATURE_CODES.DOCKER_ADMIN },
  { path: '/system/external-login-provider', featureCode: null },
	{ path: '/system/enterprise-connections', featureCode: null },
];

export const PUBLIC_RUNTIME_ROUTE_PATHS = [
  '/login',
  '/setup',
  '/passkey-bridge',
  '/oidc/callback/authorization-console',
  '/oauth/landing/:providerCode',
	'/external-identities/oauth/callback/:provider',
];

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object';
}

function normalizeBool(value: unknown, fallback = false): boolean {
  if (value === undefined || value === null || value === '') {
    return fallback;
  }
  return value === true || value === 1 || value === '1' || value === 'true';
}

function isRuntimePlatformMode(value: unknown): value is RuntimePlatformMode {
  return value === 'local' || value === 'hub' || value === 'node';
}

function normalizeEnabledFeatureCodes(value: unknown): string[] {
  if (!Array.isArray(value) || value.some((item) => typeof item !== 'string')) {
    throw new Error('运行特性接口返回异常');
  }
  return [...new Set(value.map((item) => item.trim()).filter(Boolean))];
}

function readLegacyEnabledFeatureCodes(source: Record<string, unknown>): string[] {
  const platformSource = source.platform;
  if (!isRecord(platformSource) || !isRuntimePlatformMode(platformSource.mode)) {
    throw new Error('运行配置接口返回异常');
  }
  const capabilities = platformSource.capabilities;
  if (
    !isRecord(capabilities)
    || typeof capabilities.controlPlane !== 'boolean'
    || typeof capabilities.federatedHubLogin !== 'boolean'
    || typeof capabilities.nodeApi !== 'boolean'
  ) {
    throw new Error('运行配置接口返回异常');
  }

  const enabled: string[] = [];
  if (capabilities.controlPlane) enabled.push(RUNTIME_FEATURE_CODES.PLATFORM_CONTROL);
  if (capabilities.federatedHubLogin) enabled.push(RUNTIME_FEATURE_CODES.FEDERATION_HUB);
  if (capabilities.nodeApi) enabled.push(RUNTIME_FEATURE_CODES.FEDERATION_NODE);
  if (isRecord(source.docker) && normalizeBool(source.docker.enabled)) {
    enabled.push(RUNTIME_FEATURE_CODES.DOCKER_ADMIN);
  }
  return enabled;
}

export function normalizeRuntimeFeatures(raw: unknown): RuntimeFeatures {
  if (!isRecord(raw)) {
    throw new Error('运行配置接口返回异常');
  }

  const featuresSource = raw.features;
  const enabled = featuresSource === undefined
    ? readLegacyEnabledFeatureCodes(raw)
    : isRecord(featuresSource)
      ? normalizeEnabledFeatureCodes(featuresSource.enabled)
      : normalizeEnabledFeatureCodes(undefined);
  const enabledSet = new Set(enabled);
  const platformSource = isRecord(raw.platform) ? raw.platform : {};
  const notificationSource = isRecord(raw.notification) ? raw.notification : {};
  const runtimeLogSource = isRecord(raw.runtimeLog) ? raw.runtimeLog : {};

  return {
    features: { enabled },
    platform: {
      mode: isRuntimePlatformMode(platformSource.mode) ? platformSource.mode : 'local',
      capabilities: {
        controlPlane: enabledSet.has(RUNTIME_FEATURE_CODES.PLATFORM_CONTROL),
        federatedHubLogin: enabledSet.has(RUNTIME_FEATURE_CODES.FEDERATION_HUB),
        nodeApi: enabledSet.has(RUNTIME_FEATURE_CODES.FEDERATION_NODE),
      },
    },
    docker: {
      enabled: enabledSet.has(RUNTIME_FEATURE_CODES.DOCKER_ADMIN),
    },
    notification: {
      managedByPlatform: normalizeBool(notificationSource.managedByPlatform),
    },
    runtimeLog: {
      managedByPlatform: normalizeBool(runtimeLogSource.managedByPlatform),
    },
  };
}

function normalizePath(path?: string) {
  return (path || '').split(/[?#]/, 1)[0];
}

function matchesManifestPath(path: string, manifestPath: string) {
  return path === manifestPath || path.startsWith(`${manifestPath}/`);
}

export function findRuntimeRouteFeature(path?: string) {
  const normalizedPath = normalizePath(path);
  return RUNTIME_ROUTE_FEATURE_MANIFEST.find((entry) =>
    matchesManifestPath(normalizedPath, entry.path),
  );
}

export function isRuntimeFeatureEnabled(
  features: RuntimeFeatures | null | undefined,
  featureCode: string,
) {
  return features?.features.enabled.includes(featureCode) === true;
}

export function isPlatformManagementEnabled(features?: RuntimeFeatures | null) {
  return isRuntimeFeatureEnabled(features, RUNTIME_FEATURE_CODES.PLATFORM_CONTROL);
}

export function isDockerEnabled(features?: RuntimeFeatures | null) {
  return isRuntimeFeatureEnabled(features, RUNTIME_FEATURE_CODES.DOCKER_ADMIN);
}

export function isPathAllowedByRuntimeFeatures(
  path: string | undefined,
  features?: RuntimeFeatures | null,
  declaredFeatureCode?: string,
) {
  const normalizedPath = normalizePath(path);
  if (!normalizedPath || /^https?:\/\//i.test(normalizedPath)) {
    return true;
  }

  const manifestEntry = findRuntimeRouteFeature(normalizedPath);
  if (manifestEntry?.featureCode === null) {
    return true;
  }
  const featureCode = manifestEntry?.featureCode || declaredFeatureCode?.trim();
  return featureCode ? isRuntimeFeatureEnabled(features, featureCode) : true;
}

export function filterRoutesByRuntimeFeatures(
  routes: AppRoute[],
  features?: RuntimeFeatures | null,
  parentKey = 'runtime-root',
): AppRoute[] {
  return routes.reduce<AppRoute[]>((result, route, index) => {
    const routeKey = route.key ?? `${parentKey}-${index}-${route.path}`;
    const filteredChildren = route.routes
      ? filterRoutesByRuntimeFeatures(route.routes, features, routeKey)
      : route.routes;
    const routeAllowed = isPathAllowedByRuntimeFeatures(
      route.path,
      features,
      route.featureCode,
    );
    const hasAllowedChildren = Boolean(filteredChildren?.length);
    const isEmptyGroupRoute = Boolean(route.routes?.length)
      && !hasAllowedChildren
      && !route.component
      && !route.redirect;

    if ((!routeAllowed && !hasAllowedChildren) || isEmptyGroupRoute) {
      return result;
    }

    result.push({
      ...route,
      key: routeKey,
      routes: filteredChildren,
    });
    return result;
  }, []);
}

function resolveRoutePath(parentPath: string, routePath?: string) {
  if (!routePath) return parentPath;
  if (routePath.startsWith('/')) return routePath;
  if (!parentPath || parentPath === '/') return `/${routePath}`;
  return `${parentPath}/${routePath}`;
}

export function buildRuntimeRouteManifest(
  features: RuntimeFeatures,
  routes: RouteObject[],
  parentPath = '',
): RouteObject[] {
  return routes.flatMap((route) => {
    const fullPath = resolveRoutePath(parentPath, route.path);
    if (!isPathAllowedByRuntimeFeatures(fullPath, features)) {
      return [];
    }
    const children = route.children
      ? buildRuntimeRouteManifest(features, route.children, fullPath)
      : undefined;
    if (route.children && children?.length === 0 && !route.index && !route.element) {
      return [];
    }
    return [route.children ? ({ ...route, children } as RouteObject) : route];
  });
}

export function buildPublicRuntimeRouteManifest(
  routes: RouteObject[],
  parentPath = '',
): RouteObject[] {
  return routes.flatMap((route) => {
    const fullPath = resolveRoutePath(parentPath, route.path);
    const children = route.children
      ? buildPublicRuntimeRouteManifest(route.children, fullPath)
      : undefined;
    const isPublicRoute = PUBLIC_RUNTIME_ROUTE_PATHS.includes(fullPath);
    const isPublicIndexRoute = Boolean(route.index) && PUBLIC_RUNTIME_ROUTE_PATHS.includes(parentPath);
    if (!isPublicRoute && !isPublicIndexRoute && !children?.length) {
      return [];
    }
    return [route.children ? ({ ...route, children } as RouteObject) : route];
  });
}
