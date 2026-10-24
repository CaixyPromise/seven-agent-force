import { Navigate, createBrowserRouter } from 'react-router-dom';
import { buildAutoRoutes } from '@/autoRoutes';
import RuntimeNotFound from '@/components/RuntimeNotFound';
import type { RuntimeFeatures } from '@/lib/http/types';
import {
  buildPublicRuntimeRouteManifest,
  buildRuntimeRouteManifest,
} from '@/lib/navigation/runtimeRouteManifest';

const runtimeRoutePending = <div style={{ minHeight: '100vh' }} aria-busy="true" />;

export function createPublicRuntimeRouter() {
  return createBrowserRouter([
    { path: '/user/login', element: <Navigate to="/login" replace /> },
    ...buildPublicRuntimeRouteManifest(buildAutoRoutes()),
    { path: '*', element: runtimeRoutePending },
  ]);
}

export function createRuntimeRouter(features: RuntimeFeatures) {
  return createBrowserRouter([
    { path: '/user/login', element: <Navigate to="/login" replace /> },
    ...buildRuntimeRouteManifest(features, buildAutoRoutes()),
    { path: '*', element: <RuntimeNotFound /> },
  ]);
}
