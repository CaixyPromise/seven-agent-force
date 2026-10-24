import { refreshAccessToken, request } from '@/api/request';
import { useAuthStore } from '@/store/auth';
import { getOrCreateDeviceId } from '@/lib/auth/device';
import { buildAuthorizationHeader, shouldRefreshAccessToken } from '@/lib/auth/token';
import type {
  ApiResponse,
  RuntimeLogLine,
  RuntimeLogPageData,
  RuntimeLogPageRequest,
  RuntimeLogStreamRequest,
} from '@/lib/http/types';

export interface RuntimeLogStreamHandle {
  close: () => void;
  done: Promise<void>;
}

export interface RuntimeLogStreamCallbacks {
  onLog: (logLine: RuntimeLogLine) => void;
  onError?: (error: Error) => void;
  onHeartbeat?: () => void;
  onConnected?: () => void;
}

export async function getRuntimeLogPage(params: RuntimeLogPageRequest) {
  return request<ApiResponse<RuntimeLogPageData>>('/api/admin/runtime-logs/page', {
    method: 'GET',
    params,
  });
}

async function resolveRuntimeLogAuthorizationHeader(forceRefresh = false) {
  const authState = useAuthStore.getState();
  const needsRefresh =
    forceRefresh
    || !authState.accessToken
    || shouldRefreshAccessToken(authState.accessExpireAt);

  if (needsRefresh) {
    await refreshAccessToken();
  }

  const nextAuthState = useAuthStore.getState();
  return buildAuthorizationHeader(nextAuthState.accessToken, nextAuthState.tokenType);
}

async function openRuntimeLogStreamResponse(url: URL, abortController: AbortController) {
  let unauthorizedRetried = false;

  while (true) {
    if (abortController.signal.aborted) {
      throw new DOMException('The operation was aborted.', 'AbortError');
    }

    const authorizationHeader = await resolveRuntimeLogAuthorizationHeader(unauthorizedRetried);
    const headers: Record<string, string> = {
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
      'X-Device-Id': getOrCreateDeviceId(),
    };
    if (authorizationHeader) {
      headers.Authorization = authorizationHeader;
    }

    const response = await fetch(url.toString(), {
      method: 'GET',
      headers,
      credentials: 'include',
      cache: 'no-store',
      signal: abortController.signal,
    });

    if ((response.status === 401 || response.status === 403) && !unauthorizedRetried) {
      unauthorizedRetried = true;
      continue;
    }

    return response;
  }
}

export function openRuntimeLogStream(
  params: RuntimeLogStreamRequest,
  callbacks: RuntimeLogStreamCallbacks,
): RuntimeLogStreamHandle {
  const abortController = new AbortController();
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  const done = (async () => {
    const url = new URL('/api/admin/runtime-logs/stream', window.location.origin);
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && String(value).trim() !== '') {
        url.searchParams.set(key, String(value));
      }
    });

    const response = await openRuntimeLogStreamResponse(url, abortController);
    if (!response.ok || !response.body) {
      throw new Error(`运行日志实时流连接失败: ${response.status}`);
    }

    reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8');
    let buffer = '';
    try {
      while (true) {
        const { value, done: streamDone } = await reader.read();
        if (streamDone) {
          break;
        }
        buffer += decoder.decode(value, { stream: true });
        const parsed = consumeSseChunks(buffer, callbacks);
        buffer = parsed.rest;
      }
    } finally {
      try {
        await reader.cancel();
      } catch {
        // Ignore reader close errors caused by client aborts.
      }
      try {
        reader.releaseLock();
      } catch {
        // Ignore double-release when the stream is already closed.
      }
    }
  })().catch((error: unknown) => {
    if (abortController.signal.aborted) {
      return;
    }
    const runtimeError = error instanceof Error ? error : new Error(String(error));
    callbacks.onError?.(runtimeError);
  });

  return {
    close: () => {
      void reader?.cancel().catch(() => undefined);
      abortController.abort();
    },
    done,
  };
}

function consumeSseChunks(rawBuffer: string, callbacks: RuntimeLogStreamCallbacks) {
  let rest = rawBuffer;
  while (true) {
    const lflfIndex = rest.indexOf('\n\n');
    const crlfcrlfIndex = rest.indexOf('\r\n\r\n');
    const hasLfLf = lflfIndex >= 0;
    const hasCrLfCrLf = crlfcrlfIndex >= 0;
    if (!hasLfLf && !hasCrLfCrLf) {
      break;
    }

    const useCrLf = hasCrLfCrLf && (!hasLfLf || crlfcrlfIndex < lflfIndex);
    const splitIndex = useCrLf ? crlfcrlfIndex : lflfIndex;
    const delimiterLength = useCrLf ? 4 : 2;
    const chunk = rest.slice(0, splitIndex);
    rest = rest.slice(splitIndex + delimiterLength);
    handleSseChunk(chunk, callbacks);
  }
  return { rest };
}

function handleSseChunk(chunk: string, callbacks: RuntimeLogStreamCallbacks) {
  const lines = chunk
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0 && !line.startsWith(':'));
  if (!lines.length) {
    return;
  }

  let eventType = 'message';
  const dataParts: string[] = [];
  lines.forEach((line) => {
    if (line.startsWith('event:')) {
      eventType = line.slice('event:'.length).trim();
      return;
    }
    if (line.startsWith('data:')) {
      dataParts.push(line.slice('data:'.length).trimStart());
    }
  });
  if (!dataParts.length) {
    return;
  }
  const payload = dataParts.join('\n').trim();
  if (!payload) {
    return;
  }

  if (eventType === 'connected') {
    callbacks.onConnected?.();
    return;
  }
  if (eventType === 'heartbeat') {
    callbacks.onHeartbeat?.();
    return;
  }
  if (eventType !== 'log') {
    return;
  }

  try {
    const parsed = JSON.parse(payload) as ApiResponse<RuntimeLogLine> | RuntimeLogLine;
    const line =
      parsed && typeof parsed === 'object' && 'code' in parsed
        ? parsed.code === 0
          ? parsed.data
          : undefined
        : parsed;
    if (line && typeof line === 'object' && line.lineId) {
      callbacks.onLog(line);
    }
  } catch (error) {
    callbacks.onError?.(error instanceof Error ? error : new Error(String(error)));
  }
}
