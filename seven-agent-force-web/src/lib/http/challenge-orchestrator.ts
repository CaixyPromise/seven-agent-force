import type {
  ChallengeProofHeaders,
  ChallengeRequiredPayload,
  ChallengeRetryCode,
} from '@/lib/http/types';

interface ChallengeResolutionTask {
  fingerprint: string;
  payload: ChallengeRequiredPayload;
  resolve: (proof: ChallengeProofHeaders) => void;
  reject: (reason?: unknown) => void;
}

interface ChallengeError extends Error {
  code: ChallengeRetryCode;
  cause?: unknown;
}

type ChallengePresenter = (payload: ChallengeRequiredPayload) => Promise<ChallengeProofHeaders>;

let presenter: ChallengePresenter | null = null;
let activeTask: ChallengeResolutionTask | null = null;
const pendingQueue: ChallengeResolutionTask[] = [];
const pendingByFingerprint = new Map<string, Promise<ChallengeProofHeaders>>();

function stableSerialize(value: unknown): string {
  if (value === null || value === undefined) {
    return '';
  }
  if (Array.isArray(value)) {
    return `[${value.map((item) => stableSerialize(item)).join(',')}]`;
  }
  if (value && typeof value === 'object') {
    const objectValue = value as Record<string, unknown>;
    const sortedKeys = Object.keys(objectValue).sort();
    return `{${sortedKeys
      .map((key) => `${JSON.stringify(key)}:${stableSerialize(objectValue[key])}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

function createChallengeError(
  code: ChallengeRetryCode,
  message: string,
  cause?: unknown,
): ChallengeError {
  const error = new Error(message) as ChallengeError;
  error.code = code;
  if (cause !== undefined) {
    error.cause = cause;
  }
  return error;
}

function normalizePresenterError(error: unknown): ChallengeError {
  if ((error as { code?: string })?.code === 'CHALLENGE_CANCELLED') {
    return createChallengeError('CHALLENGE_CANCELLED', '挑战已取消', error);
  }
  if (error instanceof Error && /cancel|取消/i.test(error.message)) {
    return createChallengeError('CHALLENGE_CANCELLED', '挑战已取消', error);
  }
  return createChallengeError('CHALLENGE_CANCELLED', '挑战已取消', error);
}

function removePendingPromise(fingerprint: string) {
  pendingByFingerprint.delete(fingerprint);
}

function processQueue() {
  if (activeTask) {
    return;
  }
  const task = pendingQueue.shift();
  if (!task) {
    return;
  }

  activeTask = task;
  const currentPresenter = presenter;
  if (!currentPresenter) {
    activeTask = null;
    task.reject(
      createChallengeError('CHALLENGE_PRESENTER_UNAVAILABLE', '全局挑战展示器未注册'),
    );
    processQueue();
    return;
  }

  currentPresenter(task.payload)
    .then((proofHeaders) => {
      task.resolve(proofHeaders);
    })
    .catch((error) => {
      task.reject(normalizePresenterError(error));
    })
    .finally(() => {
      if (activeTask?.fingerprint === task.fingerprint) {
        activeTask = null;
      }
      processQueue();
    });
}

export function buildRequestFingerprint(input: {
  method?: string;
  url?: string;
  params?: unknown;
  data?: unknown;
}) {
  const method = (input.method || 'GET').toUpperCase();
  const url = input.url || '';
  const params = stableSerialize(input.params);
  const data = stableSerialize(input.data);
  return `${method}|${url}|${params}|${data}`;
}

export function registerChallengePresenter(nextPresenter: ChallengePresenter | null) {
  presenter = nextPresenter;
  return () => {
    if (presenter === nextPresenter) {
      presenter = null;
    }
  };
}

export function clearChallengeQueue(reason = '挑战已取消') {
  const challengeError = createChallengeError('CHALLENGE_CANCELLED', reason);

  if (activeTask) {
    const current = activeTask;
    activeTask = null;
    current.reject(challengeError);
    removePendingPromise(current.fingerprint);
  }

  while (pendingQueue.length > 0) {
    const task = pendingQueue.shift();
    if (!task) {
      continue;
    }
    task.reject(challengeError);
    removePendingPromise(task.fingerprint);
  }
}

function acquireChallengeProof(
  fingerprint: string,
  payload: ChallengeRequiredPayload,
): Promise<ChallengeProofHeaders> {
  const existed = pendingByFingerprint.get(fingerprint);
  if (existed) {
    return existed;
  }

  const promise = new Promise<ChallengeProofHeaders>((resolve, reject) => {
    pendingQueue.push({
      fingerprint,
      payload,
      resolve,
      reject,
    });
    processQueue();
  }).finally(() => {
    removePendingPromise(fingerprint);
  });

  pendingByFingerprint.set(fingerprint, promise);
  return promise;
}

function readBusinessCode(error: unknown): number | string | undefined {
  const directCode = (error as { code?: number | string })?.code;
  if (typeof directCode === 'number' || typeof directCode === 'string') {
    return directCode;
  }

  const payloadCode = (error as { payload?: { code?: number | string } })?.payload?.code;
  if (typeof payloadCode === 'number' || typeof payloadCode === 'string') {
    return payloadCode;
  }

  const responseCode = (error as { response?: { data?: { code?: number | string } } })?.response?.data
    ?.code;
  if (typeof responseCode === 'number' || typeof responseCode === 'string') {
    return responseCode;
  }

  return undefined;
}

function createRetryExhaustedError(error: unknown): ChallengeError {
  return createChallengeError(
    'CHALLENGE_RETRY_EXHAUSTED',
    '挑战凭证已失效，请重新完成挑战',
    error,
  );
}

export async function resolveWithChallenge<T>(input: {
  fingerprint: string;
  payload: ChallengeRequiredPayload;
  executeRetry: (proofHeaders: ChallengeProofHeaders) => Promise<T>;
}) {
  const proofHeaders = await acquireChallengeProof(input.fingerprint, input.payload);
  try {
    return await input.executeRetry(proofHeaders);
  } catch (error) {
    if (readBusinessCode(error) === 40120) {
      throw createRetryExhaustedError(error);
    }
    throw error;
  }
}

export function isChallengeRetryError(
  error: unknown,
  code?: ChallengeRetryCode,
): error is ChallengeError {
  const retryCode = (error as { code?: string })?.code as ChallengeRetryCode | undefined;
  if (!retryCode) {
    return false;
  }
  if (!code) {
    return true;
  }
  return retryCode === code;
}
