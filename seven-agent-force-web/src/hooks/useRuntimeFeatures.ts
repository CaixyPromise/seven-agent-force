import { useQuery } from '@tanstack/react-query';

import {
  getRuntimeFeatures,
  SAFE_RUNTIME_FEATURES,
} from '@/api/runtimeFeaturesController';

export const RUNTIME_FEATURES_QUERY_KEY = ['runtime-features'] as const;

export function useRuntimeFeatures(options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: RUNTIME_FEATURES_QUERY_KEY,
    queryFn: getRuntimeFeatures,
    enabled: options?.enabled ?? true,
    staleTime: 60 * 1000,
    retry: 0,
  });
}

export function useSafeRuntimeFeatures(options?: { enabled?: boolean }) {
  const query = useRuntimeFeatures(options);
  return {
    ...query,
    safeData: query.data ?? SAFE_RUNTIME_FEATURES,
  };
}
