import { useCallback, useMemo, useState } from 'react';
import {
  getChallenge,
  refreshChallenge,
  respondChallenge,
  type RespondChallengeResponse,
  type ChallengeStep,
  type StartChallengeResponse,
} from '@/api/challengeController';
import { startMfaChallenge, type MfaBusinessAction } from '@/api/mfaController';
import { resolveSemanticErrorMessage } from '@/lib/http/error-semantics';
import type { ApiResponse, ChallengeRequiredPayload } from '@/lib/http/types';

export interface StartMfaFlowOptions {
  action: MfaBusinessAction;
  extensionContext?: Record<string, unknown>;
  requestedTimeToLiveSeconds?: number;
  flowNonce?: string;
}

export interface SubmitCurrentStepOptions {
  payload: Record<string, unknown>;
  stepIdentifier?: string;
}

interface UseMfaChallengeFlowOptions {
  onPassed?: (result: {
    proofToken?: string;
    flowNonce?: string;
    response: RespondChallengeResponse;
  }) => void | Promise<void>;
}

function readErrorMessage(error: unknown, fallback: string): string {
  const payload = (error as { payload?: ApiResponse<unknown> })?.payload;
  if (payload) {
    return resolveSemanticErrorMessage(payload, fallback);
  }
  const message = (error as { message?: string })?.message;
  return message || fallback;
}

function readChallengeFailureData(error: unknown): RespondChallengeResponse | null {
  const payload = (error as { payload?: ApiResponse<unknown> })?.payload;
  const data = payload?.data;
  if (!data || typeof data !== 'object') {
    return null;
  }
  const candidate = data as RespondChallengeResponse;
  if (!candidate.challengeState) {
    return null;
  }
  return {
    ...candidate,
    message: readErrorMessage(error, '挑战验证失败'),
  };
}

export function useMfaChallengeFlow(options?: UseMfaChallengeFlowOptions) {
  const [challenge, setChallenge] = useState<StartChallengeResponse | null>(null);
  const [currentStepIdentifier, setCurrentStepIdentifier] = useState<string | null>(null);
  const [activeFlowNonce, setActiveFlowNonce] = useState<string | null>(null);
  const [lastResponse, setLastResponse] = useState<RespondChallengeResponse | null>(null);
  const [busy, setBusy] = useState(false);

  const inProgressSteps = useMemo<ChallengeStep[]>(() => {
    if (!challenge?.steps?.length) {
      return [];
    }
    return challenge.steps.filter((step) => step.stepState === 'IN_PROGRESS');
  }, [challenge]);

  const currentStep = useMemo<ChallengeStep | null>(() => {
    if (!challenge?.steps?.length) {
      return null;
    }
    if (currentStepIdentifier) {
      const matched = challenge.steps.find((step) => step.stepIdentifier === currentStepIdentifier);
      if (matched) {
        return matched;
      }
    }
    if (challenge.recommendedStepIdentifier) {
      const recommended = challenge.steps.find(
        (step) => step.stepIdentifier === challenge.recommendedStepIdentifier,
      );
      if (recommended) {
        return recommended;
      }
    }
    return inProgressSteps[0] ?? challenge.steps[0] ?? null;
  }, [challenge, currentStepIdentifier, inProgressSteps]);

  const reset = useCallback(() => {
    setChallenge(null);
    setCurrentStepIdentifier(null);
    setActiveFlowNonce(null);
    setLastResponse(null);
    setBusy(false);
  }, []);

  const applyChallengePayload = useCallback(
    (payload: ChallengeRequiredPayload | StartChallengeResponse | null, flowNonce?: string | null) => {
      if (!payload?.challengeIdentifier) {
        reset();
        return;
      }
      const nextChallenge = payload as StartChallengeResponse;
      setChallenge(nextChallenge);
      setCurrentStepIdentifier(
        nextChallenge.recommendedStepIdentifier
          ?? nextChallenge.steps?.[0]?.stepIdentifier
          ?? null,
      );
      if (flowNonce !== undefined) {
        setActiveFlowNonce(flowNonce);
      }
      setLastResponse(null);
    },
    [reset],
  );

  const start = useCallback(
    async (params: StartMfaFlowOptions) => {
      setBusy(true);
      try {
        const response = await startMfaChallenge({
          businessAction: params.action,
          extensionContext: params.extensionContext,
          requestedTimeToLiveSeconds: params.requestedTimeToLiveSeconds,
          flowNonce: params.flowNonce,
        });
        const data = response?.data ?? null;
        if (!data?.challengeIdentifier) {
          throw new Error('挑战发起失败，未返回挑战标识');
        }
        applyChallengePayload(data, params.flowNonce ?? null);
        return data;
      } finally {
        setBusy(false);
      }
    },
    [applyChallengePayload],
  );

  const openWithPayload = useCallback(
    (payload: ChallengeRequiredPayload) => {
      applyChallengePayload(payload, payload?.flowNonce ?? null);
    },
    [applyChallengePayload],
  );

  const switchStep = useCallback(
    (stepIdentifier: string) => {
      if (!stepIdentifier) {
        return;
      }
      const targetStep = inProgressSteps.find((step) => step.stepIdentifier === stepIdentifier);
      if (!targetStep) {
        return;
      }
      setCurrentStepIdentifier(stepIdentifier);
      setLastResponse(null);
    },
    [inProgressSteps],
  );

  const refreshCurrentStep = useCallback(async () => {
    if (!challenge?.challengeIdentifier || !currentStep?.stepIdentifier) {
      return null;
    }
    setBusy(true);
    try {
      const response = await refreshChallenge(challenge.challengeIdentifier, {
        stepIdentifier: currentStep.stepIdentifier,
      });
      const data = response?.data ?? null;
      if (data?.challengeIdentifier) {
        applyChallengePayload(data, activeFlowNonce);
      }
      return data;
    } catch (error) {
      const payloadCode = (error as { payload?: { code?: number } })?.payload?.code;
      if (payloadCode === 40400) {
        throw new Error('验证会话已失效，请关闭验证窗口后重新提交操作');
      }
      throw new Error(readErrorMessage(error, '验证码发送失败，请稍后重试'));
    } finally {
      setBusy(false);
    }
  }, [challenge, currentStep, activeFlowNonce, applyChallengePayload]);

  const submitCurrentStep = useCallback(
    async (params: SubmitCurrentStepOptions) => {
      if (!challenge?.challengeIdentifier) {
        throw new Error('挑战会话不存在，请先发起挑战');
      }
      const stepIdentifier = params.stepIdentifier || currentStep?.stepIdentifier;
      if (!stepIdentifier) {
        throw new Error('挑战步骤不存在，请刷新后重试');
      }

      setBusy(true);
      try {
        const response = await respondChallenge(challenge.challengeIdentifier, {
          stepIdentifier,
          payload: params.payload,
        });
        const data = response?.data;
        if (!data) {
          throw new Error('挑战响应失败，请稍后重试');
        }
        setLastResponse(data);

        if (data.challengeState === 'PASSED') {
          await options?.onPassed?.({
            proofToken: data.proofToken,
            flowNonce: activeFlowNonce ?? undefined,
            response: data,
          });
          reset();
          return data;
        }

        if (data.challengeState === 'FAILED' || data.challengeState === 'EXPIRED') {
          throw new Error('挑战已失效，请重新发起');
        }

        try {
          const sessionResponse = await getChallenge(challenge.challengeIdentifier);
          if (sessionResponse?.data?.challengeIdentifier) {
            applyChallengePayload(sessionResponse.data, activeFlowNonce);
          }
        } catch {
          // 忽略会话快照刷新失败，继续保留当前挑战状态。
        }

        if (data.nextStepIdentifier) {
          setCurrentStepIdentifier(data.nextStepIdentifier);
        } else if (data.recommendedStepIdentifier) {
          setCurrentStepIdentifier(data.recommendedStepIdentifier);
        }
        return data;
      } catch (error) {
        const payloadCode = (error as { payload?: { code?: number } })?.payload?.code;
        const payloadData = (error as { payload?: { data?: ChallengeRequiredPayload } })?.payload?.data;
        if (payloadCode === 40120 && payloadData?.challengeIdentifier) {
          openWithPayload(payloadData);
        }
        const failureData = readChallengeFailureData(error);
        if (failureData) {
          setLastResponse(failureData);
          if (failureData.nextStepIdentifier) {
            setCurrentStepIdentifier(failureData.nextStepIdentifier);
          } else if (failureData.recommendedStepIdentifier) {
            setCurrentStepIdentifier(failureData.recommendedStepIdentifier);
          }
          return failureData;
        }
        throw new Error(readErrorMessage(error, '挑战验证失败'));
      } finally {
        setBusy(false);
      }
    },
    [challenge, currentStep, options, activeFlowNonce, reset, openWithPayload, applyChallengePayload],
  );

  const isActive = Boolean(challenge?.challengeIdentifier);

  return {
    challenge,
    currentStep,
    availableSteps: inProgressSteps,
    flowNonce: activeFlowNonce,
    lastResponse,
    busy,
    isActive,
    start,
    openWithPayload,
    switchStep,
    refreshCurrentStep,
    submitCurrentStep,
    reset,
  };
}
