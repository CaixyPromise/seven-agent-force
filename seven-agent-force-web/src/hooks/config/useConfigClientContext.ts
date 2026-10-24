import { createContext, useContext } from 'react';
import type { ConfigValueDTO } from '@/types/configClient';

export interface ConfigClientContextValue {
  getConfig: (key: string) => ConfigValueDTO | null;
  ensureConfig: (key: string) => void;
  refreshConfig: (key: string) => Promise<void>;
  refreshAll: () => void;
  isLoading: (key: string) => boolean;
  version: number;
}

export const ConfigClientContext = createContext<ConfigClientContextValue | null>(null);

export function useConfigClientContext(): ConfigClientContextValue {
  const context = useContext(ConfigClientContext);
  if (!context) {
    throw new Error('useConfigClientContext must be used within ConfigClientProvider');
  }
  return context;
}
