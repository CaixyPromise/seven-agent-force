'use client';

import { createContext, useContext } from 'react';
import type { StartChallengeResponse } from '@/api/challengeController';
import { useMfaChallengeFlow, type StartMfaFlowOptions } from '@/hooks/useMfaChallengeFlow';

export interface GlobalChallengeContextValue {
  challengeFlow: ReturnType<typeof useMfaChallengeFlow>;
  startChallenge: (options: StartMfaFlowOptions) => Promise<StartChallengeResponse>;
  cancelChallenge: () => void;
}

export const GlobalChallengeContext = createContext<GlobalChallengeContextValue | null>(null);

function useGlobalChallengeContext() {
  const context = useContext(GlobalChallengeContext);
  if (!context) {
    throw new Error('GlobalChallenge hooks 必须在 GlobalChallengeProvider 内使用');
  }
  return context;
}

export function useGlobalChallengeFlow() {
  return useGlobalChallengeContext().challengeFlow;
}

export function useGlobalChallengeActions() {
  const context = useGlobalChallengeContext();
  return {
    startChallenge: context.startChallenge,
    cancelChallenge: context.cancelChallenge,
  };
}
