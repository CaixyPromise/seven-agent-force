'use client';

import { createContext, useContext } from 'react';
import type { LoginUser } from '@/lib/http/types';

export interface InitialState {
  currentUser: LoginUser | null;
  loading: boolean;
}

export const InitialStateContext = createContext<InitialState | undefined>(undefined);

export function useInitialState() {
  const ctx = useContext(InitialStateContext);
  if (!ctx) {
    throw new Error('useInitialState must be used within InitialStateProvider');
  }
  return ctx;
}
