'use client';

import React, { useCallback, useEffect, useMemo, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import GlobalChallengeModal from '@/components/challenge/GlobalChallengeModal';
import {
  clearChallengeQueue,
  registerChallengePresenter,
} from '@/lib/http/challenge-orchestrator';
import type {
  ChallengeProofHeaders,
  ChallengeRetryCode,
  ChallengeRequiredPayload,
} from '@/lib/http/types';
import { useMfaChallengeFlow, type StartMfaFlowOptions } from '@/hooks/useMfaChallengeFlow';
import { useAuthStore } from '@/store/auth';
import {
  GlobalChallengeContext,
  type GlobalChallengeContextValue,
} from './globalChallengeContext';

interface ChallengePresenterPending {
  resolve: (value: ChallengeProofHeaders) => void;
  reject: (reason?: unknown) => void;
}

interface ChallengeRetryError extends Error {
  code: ChallengeRetryCode;
  cause?: unknown;
}

function createChallengeRetryError(
  code: ChallengeRetryCode,
  message: string,
  cause?: unknown,
): ChallengeRetryError {
  const error = new Error(message) as ChallengeRetryError;
  error.code = code;
  if (cause !== undefined) {
    error.cause = cause;
  }
  return error;
}

export default function GlobalChallengeProvider({ children }: { children: React.ReactNode }) {
  const queryClient = useQueryClient();
  const currentUserAccount = useAuthStore((state) => state.user?.username);
  const pendingPresenterRef = useRef<ChallengePresenterPending | null>(null);

  const challengeFlow = useMfaChallengeFlow({
    onPassed: async ({ proofToken, flowNonce }) => {
      const pending = pendingPresenterRef.current;
      pendingPresenterRef.current = null;

      if (pending) {
        if (proofToken && flowNonce) {
          pending.resolve({
            proofToken,
            flowNonce,
          });
        } else {
          pending.reject(
            createChallengeRetryError(
              'CHALLENGE_RETRY_EXHAUSTED',
              '挑战通过后未返回重试凭证，请重新发起挑战',
            ),
          );
        }
      }

      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['account-settings'] }),
        queryClient.invalidateQueries({ queryKey: ['account-settings', 'mfa-status'] }),
        queryClient.invalidateQueries({ queryKey: ['account-settings', 'passkeys'] }),
      ]);
    },
  });

  const cancelChallenge = useCallback(() => {
    const pending = pendingPresenterRef.current;
    pendingPresenterRef.current = null;
    challengeFlow.reset();
    if (!pending) {
      return;
    }
    pending.reject(createChallengeRetryError('CHALLENGE_CANCELLED', '挑战已取消'));
  }, [challengeFlow]);

  const startChallenge = useCallback(
    async (options: StartMfaFlowOptions) => {
      return challengeFlow.start(options);
    },
    [challengeFlow],
  );

  const openWithPayload = challengeFlow.openWithPayload;

  useEffect(() => {
    const unregister = registerChallengePresenter((payload: ChallengeRequiredPayload) => {
      if (!payload?.challengeIdentifier) {
        return Promise.reject(
          createChallengeRetryError('CHALLENGE_RETRY_EXHAUSTED', '挑战载荷不完整，无法继续'),
        );
      }

      return new Promise<ChallengeProofHeaders>((resolve, reject) => {
        pendingPresenterRef.current = {
          resolve,
          reject,
        };
        openWithPayload(payload);
      });
    });

    return () => {
      unregister();
      clearChallengeQueue();
      const pending = pendingPresenterRef.current;
      pendingPresenterRef.current = null;
      if (pending) {
        pending.reject(createChallengeRetryError('CHALLENGE_CANCELLED', '挑战展示器已关闭'));
      }
    };
  }, [openWithPayload]);

  const value = useMemo<GlobalChallengeContextValue>(() => {
    return {
      challengeFlow,
      startChallenge,
      cancelChallenge,
    };
  }, [cancelChallenge, challengeFlow, startChallenge]);

  return (
    <GlobalChallengeContext.Provider value={value}>
      {children}
      <GlobalChallengeModal
        challengeFlow={challengeFlow}
        currentUserAccount={currentUserAccount}
        onCancel={cancelChallenge}
      />
    </GlobalChallengeContext.Provider>
  );
}
