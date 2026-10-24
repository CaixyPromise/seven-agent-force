'use client';

import { Spin } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { Navigate } from 'react-router-dom';
import { useMemo } from 'react';
import { getCurrentUserMenus } from '@/api/authController';
import { useAuth } from '@/hooks/auth';
import { AUTH_MENUS_QUERY_KEY } from '@/hooks/useAuth';
import { useSafeRuntimeFeatures } from '@/hooks/useRuntimeFeatures';
import { buildRoutesFromMenuTree } from '@/lib/navigation/menuRoutes';
import { findFirstAccessiblePath } from '@/lib/navigation/routeAccess';

export default function SystemPage() {
  const { isAdmin, permissions } = useAuth();
  const runtimeFeaturesQuery = useSafeRuntimeFeatures();
  const runtimeFeatures = runtimeFeaturesQuery.safeData;
  const { data: menuResponse, isLoading } = useQuery({
    queryKey: AUTH_MENUS_QUERY_KEY,
    queryFn: () => getCurrentUserMenus({ skipAuthRedirect: true }),
    staleTime: 5 * 60 * 1000,
  });
  const dynamicRoutes = useMemo(
    () => buildRoutesFromMenuTree(menuResponse?.data ?? [], runtimeFeatures),
    [menuResponse?.data, runtimeFeatures],
  );
  const fallbackPath = useMemo(
    () => {
      const systemRoutes = dynamicRoutes.flatMap((route) => {
        if (route.path === '/system') {
          return route.routes ?? [];
        }
        return route.path.startsWith('/system/') ? [route] : [];
      });
      return (
        findFirstAccessiblePath(
          systemRoutes,
          { isAdmin, permissions },
        ) ?? '/'
      );
    },
    [dynamicRoutes, isAdmin, permissions],
  );

  if (isLoading || runtimeFeaturesQuery.isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 48 }}>
        <Spin />
      </div>
    );
  }

  return <Navigate to={fallbackPath} replace />;
}
