import { request } from './request';

export interface RespondChallengeRequest {
  stepIdentifier?: string;
  payload?: Record<string, unknown>;
}

export interface RespondChallengeResponse {
  challengeState?: string;
  proofToken?: string;
  nextStepIdentifier?: string;
  message?: string;
  remainingAttemptCount?: number;
  cooldownSeconds?: number;
  canSwitchMethod?: boolean;
  recommendedStepIdentifier?: string;
}

export interface ChallengeStep {
  stepIdentifier?: string;
  challengeType?: string;
  stepPurpose?: string;
  stepState?: string;
  remainingAttemptCount?: number;
  cooldownSeconds?: number;
  switchable?: boolean;
  userInterfaceHints?: Record<string, unknown>;
}

export interface StartChallengeResponse {
  challengeIdentifier?: string;
  expiresAt?: string;
  steps?: ChallengeStep[];
  challengeState?: string;
  effectiveTimeToLiveSeconds?: number;
  requiredAssuranceLevel?: string;
  resolvedAssuranceLevel?: string;
  recommendedStepIdentifier?: string;
  actualChallengeTypeNames?: string[];
}

export async function respondChallenge(
  challengeIdentifier: string,
  body: RespondChallengeRequest,
  options?: Record<string, unknown>,
) {
  return request<{ code: number; message?: string; data?: RespondChallengeResponse }>(
    `/api/v1/challenges/${challengeIdentifier}/respond`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      skipGlobalChallenge: true,
      ...(options || {}),
    },
  );
}

export async function getChallenge(
  challengeIdentifier: string,
  options?: Record<string, unknown>,
) {
  return request<{ code: number; message?: string; data?: StartChallengeResponse }>(
    `/api/v1/challenges/${challengeIdentifier}`,
    {
      method: 'GET',
      skipGlobalChallenge: true,
      ...(options || {}),
    },
  );
}

export async function refreshChallenge(
  challengeIdentifier: string,
  body: { stepIdentifier?: string },
  options?: Record<string, unknown>,
) {
  return request<{ code: number; message?: string; data?: StartChallengeResponse }>(
    `/api/v1/challenges/${challengeIdentifier}/refresh`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      skipGlobalChallenge: true,
      ...(options || {}),
    },
  );
}
