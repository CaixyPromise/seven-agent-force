import { createContext, useContext } from 'react';
import type { DictItemVO } from '@/types/dictClient';

export interface DictClientContextValue {
  getDict: (code: string) => DictItemVO[] | null;
  ensureDict: (code: string) => void;
  refreshDict: (code: string) => Promise<void>;
  refreshAll: () => void;
  isLoading: (code: string) => boolean;
  version: number;
}

export const DictClientContext = createContext<DictClientContextValue | null>(null);

export function useDictClientContext(): DictClientContextValue {
  const context = useContext(DictClientContext);
  if (!context) {
    throw new Error('useDictClientContext must be used within DictClientProvider');
  }
  return context;
}
