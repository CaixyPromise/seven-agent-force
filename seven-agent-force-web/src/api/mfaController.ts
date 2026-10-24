import { request } from './request';
import type { StartChallengeResponse } from './challengeController';

interface ResultEnvelope<T> {
  code: number;
  message?: string;
  data?: T;
  success?: boolean;
}

export const MFA_ACTIONS = {
  OTP_BIND: 'MFA_OTP_BIND',
  OTP_SWITCH: 'MFA_OTP_SWITCH',
  PASSKEY_BIND: 'MFA_PASSKEY_BIND',
  PASSKEY_SWITCH: 'MFA_PASSKEY_SWITCH',
  RECOVERY_VERIFY: 'MFA_RECOVERY_VERIFY',
  RECOVERY_CODES_REGENERATE: 'MFA_RECOVERY_CODES_REGENERATE',
  OTP_DELETE: 'MFA_OTP_DELETE',
  PASSKEY_DELETE: 'MFA_PASSKEY_DELETE',
} as const;

export type MfaBusinessAction = (typeof MFA_ACTIONS)[keyof typeof MFA_ACTIONS];

export interface MfaStatusResponse {
  subjectIdentifier?: string;
  otpBound?: boolean;
  passkeyBound?: boolean;
  availableRecoveryCodeCount?: number;
}

export interface MfaPasskeyVO {
  credentialIdentifier?: string;
  displayName?: string;
  aaguid?: string;
  transports?: string;
  createdAt?: string;
  lastUsedAt?: string;
}

export interface RegenerateRecoveryCodeResponse {
  subjectIdentifier?: string;
  batchIdentifier?: string;
  recoveryCodes?: string[];
  generatedAt?: string;
}

export interface MfaChallengeStartRequest {
  businessAction: MfaBusinessAction;
  flowNonce?: string;
  requestedTimeToLiveSeconds?: number;
  extensionContext?: Record<string, unknown>;
}

export async function getMfaStatus(options?: Record<string, unknown>) {
  return request<ResultEnvelope<MfaStatusResponse>>('/api/v1/mfa/status', {
    method: 'GET',
    ...(options || {}),
  });
}

export async function regenerateRecoveryCodes(options?: Record<string, unknown>) {
  return request<ResultEnvelope<RegenerateRecoveryCodeResponse>>('/api/v1/mfa/recovery-codes/regenerate', {
    method: 'POST',
    ...(options || {}),
  });
}

export async function listMfaPasskeys(options?: Record<string, unknown>) {
  return request<ResultEnvelope<MfaPasskeyVO[]>>('/api/v1/mfa/passkeys', {
    method: 'GET',
    ...(options || {}),
  });
}

export async function deleteMfaPasskey(
  credentialIdentifier: string,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<boolean>>(`/api/v1/mfa/passkeys/${credentialIdentifier}`, {
    method: 'DELETE',
    ...(options || {}),
  });
}

export async function deleteMfaOtpBinding(options?: Record<string, unknown>) {
  return request<ResultEnvelope<boolean>>('/api/v1/mfa/otp-binding', {
    method: 'DELETE',
    ...(options || {}),
  });
}

export async function startMfaChallenge(
  body: MfaChallengeStartRequest,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<StartChallengeResponse>>('/api/v1/mfa/challenges/start', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    skipGlobalChallenge: true,
    ...(options || {}),
  });
}
