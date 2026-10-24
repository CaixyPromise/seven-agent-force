import type { AppRoute } from 'config/route';
import { hasPermissions, type PermissionMatchMode } from '@/lib/auth/permissions';

export interface RouteAccessContext {
  isAdmin?: boolean;
  permissions?: string[];
}

function canAccessLegacyRule(route: AppRoute, context: RouteAccessContext) {
  if (!route.access) {
    return true;
  }

  if (route.access === 'canAdmin') {
    return Boolean(context.isAdmin);
  }

  return true;
}

export function canAccessRoute(route: AppRoute, context: RouteAccessContext) {
  if (!canAccessLegacyRule(route, context)) {
    return false;
  }

  if (!route.requiredPermissions?.length) {
    return true;
  }

  return hasPermissions(
    context.permissions,
    route.requiredPermissions,
    route.permissionMatchMode as PermissionMatchMode | undefined,
  );
}

export function filterRoutesByAccess(
  routes: AppRoute[],
  context: RouteAccessContext,
  parentKey = 'root',
): AppRoute[] {
  return routes.reduce<AppRoute[]>((result, route, index) => {
    const routeKey = route.key ?? `${parentKey}-${index}-${route.path}`;
    const filteredChildren = route.routes
      ? filterRoutesByAccess(route.routes, context, routeKey)
      : route.routes;
    const routeAccessible = canAccessRoute(route, context);
    const hasAccessibleChildren = Boolean(filteredChildren?.length);
    const isEmptyGroupRoute = Boolean(route.routes?.length)
      && !hasAccessibleChildren
      && !route.component
      && !route.redirect;

    if ((!routeAccessible && !hasAccessibleChildren) || isEmptyGroupRoute) {
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

export function findRouteChain(
  pathname: string,
  routes: AppRoute[],
  chain: AppRoute[] = [],
): AppRoute[] | null {
  for (const route of routes) {
    const nextChain = [...chain, route];
    if (route.path === pathname) {
      return nextChain;
    }
    if (route.routes) {
      const matched = findRouteChain(pathname, route.routes, nextChain);
      if (matched) {
        return matched;
      }
    }
  }

  return null;
}

function findFirstPathFromRoute(route: AppRoute): string | null {
  if (route.routes?.length) {
    for (const childRoute of route.routes) {
      const childPath = findFirstPathFromRoute(childRoute);
      if (childPath) {
        return childPath;
      }
    }
  }

  if (route.redirect) {
    return route.redirect;
  }

  if (!route.hideInMenu && route.path && route.path !== '/') {
    return route.path;
  }

  return null;
}

export function findFirstAccessiblePath(routes: AppRoute[], context: RouteAccessContext) {
  const accessibleRoutes = filterRoutesByAccess(routes, context);
  for (const route of accessibleRoutes) {
    const accessiblePath = findFirstPathFromRoute(route);
    if (accessiblePath) {
      return accessiblePath;
    }
  }
  return null;
}
