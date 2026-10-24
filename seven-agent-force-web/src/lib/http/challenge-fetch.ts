import {
  buildRequestFingerprint,
  isChallengeRetryError,
  resolveWithChallenge,
} from '@/lib/http/challenge-orchestrator';
import { createApiError } from '@/lib/http/error-semantics';
import type {
  ApiResponse,
  ChallengeProofHeaders,
  ChallengeRequiredPayload,
} from '@/lib/http/types';

interface ChallengeAwareFetchInit extends RequestInit {
  skipGlobalChallenge?: boolean;
  _challengeRetry?: boolean;
  _challengeFingerprint?: string;
}

function readChallengePayload(payload: ApiResponse<unknown>): ChallengeRequiredPayload | null {
  const challengePayload = payload?.data as ChallengeRequiredPayload | undefined;
  if (!challengePayload?.challengeIdentifier) {
    return null;
  }
  return challengePayload;
}

function resolveUrl(input: RequestInfo | URL): string {
  if (typeof input === 'string') {
    return input;
  }
  if (input instanceof URL) {
    return input.toString();
  }
  return input.url;
}

function normalizeBody(body: BodyInit | null | undefined): unknown {
  if (body === null || body === undefined) {
    return undefined;
  }

  if (typeof body === 'string') {
    try {
      return JSON.parse(body);
    } catch {
      return body;
    }
  }

  if (typeof URLSearchParams !== 'undefined' && body instanceof URLSearchParams) {
    return body.toString();
  }

  if (typeof FormData !== 'undefined' && body instanceof FormData) {
    const values: Record<string, string[]> = {};
    body.forEach((value, key) => {
      const normalized = typeof value === 'string' ? value : value.name;
      if (!values[key]) {
        values[key] = [];
      }
      values[key].push(normalized);
    });
    return values;
  }

  return String(body);
}

function buildProofHeaderBag(proofHeaders: ChallengeProofHeaders) {
  return {
    'Proof-Token': proofHeaders.proofToken,
    'Flow-Nonce': proofHeaders.flowNonce,
  } as Record<string, string>;
}

function mergeHeaders(baseHeaders: HeadersInit | undefined, patch: Record<string, string>) {
  const merged = new Headers(baseHeaders);
  Object.entries(patch).forEach(([key, value]) => {
    merged.set(key, value);
  });
  return merged;
}

async function parseResponsePayload<T>(response: Response): Promise<ApiResponse<T>> {
  const payload = (await response.json()) as ApiResponse<T>;
  if (!payload || typeof payload !== 'object' || typeof payload.code !== 'number') {
    throw new Error(`请求失败，HTTP ${response.status}`);
  }
  return payload;
}

export async function challengeAwareFetch<T>(
  input: RequestInfo | URL,
  init: ChallengeAwareFetchInit = {},
): Promise<ApiResponse<T>> {
  const method = (init.method || 'GET').toUpperCase();
  const url = resolveUrl(input);

  const response = await fetch(input, init);
  const payload = await parseResponsePayload<T>(response);

  if (payload.code === 0 || payload.code === 200) {
    return payload;
  }

  if (payload.code === 40120) {
    const challengePayload = readChallengePayload(payload as ApiResponse<unknown>);
    if (!challengePayload) {
      throw createApiError(payload, '需要完成挑战验证');
    }

    if (init.skipGlobalChallenge || init._challengeRetry) {
      throw createApiError(payload, '需要完成挑战验证');
    }

    const fingerprint =
      init._challengeFingerprint ||
      buildRequestFingerprint({
        method,
        url,
        data: normalizeBody(init.body),
      });

    try {
      return await resolveWithChallenge({
        fingerprint,
        payload: challengePayload,
        executeRetry: async (proofHeaders) => {
          return challengeAwareFetch<T>(input, {
            ...init,
            _challengeRetry: true,
            _challengeFingerprint: fingerprint,
            headers: mergeHeaders(init.headers, buildProofHeaderBag(proofHeaders)),
          });
        },
      });
    } catch (challengeError) {
      if (isChallengeRetryError(challengeError, 'CHALLENGE_PRESENTER_UNAVAILABLE')) {
        throw createApiError(payload, '需要完成挑战验证');
      }
      throw challengeError;
    }
  }

  throw createApiError(payload, '请求失败');
}
