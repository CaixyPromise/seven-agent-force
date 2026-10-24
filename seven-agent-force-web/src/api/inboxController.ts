import { refreshAccessToken, request } from '@/api/request';
import { getOrCreateDeviceId } from '@/lib/auth/device';
import { buildAuthorizationHeader, shouldRefreshAccessToken } from '@/lib/auth/token';
import type { ApiResponse } from '@/lib/http/types';
import { useAuthStore } from '@/store/auth';

const BASE = '/api/notification/inbox';

export interface InboxListItem {
  recipientId: string;
  title: string;
  summary?: string;
  firstSeenAt?: string;
  readAt?: string;
  archivedAt?: string;
  mailboxVersion: string;
  createTime: string;
  updateTime: string;
}

export interface InboxDetail extends InboxListItem {
  content: string;
  deepLink?: string;
  primaryAction?: {
    type: 'OPEN_INTERNAL';
    label: string;
    link: string;
    style: 'PRIMARY';
  };
}

export interface InboxPreviewItem {
  recipientId: string;
  title: string;
  summary?: string;
  mailboxVersion: string;
  createTime: string;
}

export interface InboxPage {
  records: InboxListItem[];
  nextPageCursor?: string;
  changeToken: string;
}

export interface InboxPreview {
  records: InboxPreviewItem[];
  mailboxKey: string;
  changeToken: string;
}

export interface InboxChanges {
  upserts: InboxListItem[];
  removedRecipientIds: string[];
  mailboxKey?: string;
  unreadCount: number;
  nextChangeToken?: string;
  targetChangeToken?: string;
  hasMore: boolean;
  resyncRequired: boolean;
  serverTime: string;
}

export interface InboxUnreadCount {
  mailboxKey: string;
  unreadCount: number;
  changeToken: string;
}

export interface InboxRealtimeHint {
  changeToken: string;
  newUnread: boolean;
}

export interface InboxMutationRequest {
  expectedMailboxVersion?: string;
}

export interface InboxListParams {
  archived?: boolean;
  pageCursor?: string;
  pageSize?: number;
}

export interface InboxChangeParams {
  afterChangeToken?: string;
  untilChangeToken?: string;
  limit?: number;
}

export interface InboxStreamCallbacks {
  onHint: (hint: InboxRealtimeHint) => void;
  onConnected?: () => void;
  onHeartbeat?: () => void;
  onError?: (error: Error) => void;
}

export interface InboxStreamHandle {
  close: () => void;
  done: Promise<void>;
}

function unwrap<T>(response: ApiResponse<T>, fallback: string): T {
  if (!response || typeof response !== 'object' || !('code' in response)) {
    return response as T;
  }
  if (response.code !== 0 && response.code !== 200) {
    throw new Error(response.message || fallback);
  }
  return response.data as T;
}

export async function getInboxUnreadCount(signal?: AbortSignal) {
  return unwrap(
    await request<ApiResponse<InboxUnreadCount>>(`${BASE}/unread-count`, { signal }),
    '读取未读消息失败',
  );
}

export async function getInboxPreview(signal?: AbortSignal) {
  return unwrap(
    await request<ApiResponse<InboxPreview>>(`${BASE}/unread-preview`, {
      params: { limit: 5 },
      signal,
    }),
    '读取消息预览失败',
  );
}

export async function listInbox(params: InboxListParams, signal?: AbortSignal) {
  return unwrap(
    await request<ApiResponse<InboxPage>>(`${BASE}`, { params, signal }),
    '读取消息列表失败',
  );
}

export async function getInboxDetail(recipientId: string, signal?: AbortSignal) {
  return unwrap(
    await request<ApiResponse<InboxDetail>>(`${BASE}/${encodeURIComponent(recipientId)}`, { signal }),
    '读取消息详情失败',
  );
}

export async function listInboxChanges(params: InboxChangeParams, signal?: AbortSignal) {
  return unwrap(
    await request<ApiResponse<InboxChanges>>(`${BASE}/changes`, { params, signal }),
    '刷新消息失败',
  );
}

export async function markInboxRead(
  recipientId: string,
  body: InboxMutationRequest = {},
  signal?: AbortSignal,
) {
  return mutateInboxRecipient(recipientId, 'read', body, signal);
}

export async function markInboxUnread(
  recipientId: string,
  body: InboxMutationRequest = {},
  signal?: AbortSignal,
) {
  return mutateInboxRecipient(recipientId, 'unread', body, signal);
}

export async function archiveInboxRecipient(
  recipientId: string,
  body: InboxMutationRequest = {},
  signal?: AbortSignal,
) {
  return mutateInboxRecipient(recipientId, 'archive', body, signal);
}

export async function restoreInboxRecipient(
  recipientId: string,
  body: InboxMutationRequest = {},
  signal?: AbortSignal,
) {
  return mutateInboxRecipient(recipientId, 'restore', body, signal);
}

async function mutateInboxRecipient(
  recipientId: string,
  action: 'read' | 'unread' | 'archive' | 'restore',
  body: InboxMutationRequest,
  signal?: AbortSignal,
) {
  return unwrap(
    await request<ApiResponse<InboxListItem>>(`${BASE}/${encodeURIComponent(recipientId)}/${action}`, {
      method: 'POST',
      data: body,
      signal,
    }),
    '更新消息状态失败',
  );
}

async function resolveInboxAuthorizationHeader(forceRefresh = false) {
  const authState = useAuthStore.getState();
  if (forceRefresh || !authState.accessToken || shouldRefreshAccessToken(authState.accessExpireAt)) {
    await refreshAccessToken();
  }
  const refreshed = useAuthStore.getState();
  return buildAuthorizationHeader(refreshed.accessToken, refreshed.tokenType);
}

async function openInboxStreamResponse(url: URL, abortController: AbortController) {
  let unauthorizedRetried = false;
  while (true) {
    if (abortController.signal.aborted) {
      throw new DOMException('The operation was aborted.', 'AbortError');
    }
    const authorization = await resolveInboxAuthorizationHeader(unauthorizedRetried);
    const headers: Record<string, string> = {
      Accept: 'text/event-stream',
      'Cache-Control': 'no-cache',
      'X-Device-Id': getOrCreateDeviceId(),
    };
    if (authorization) {
      headers.Authorization = authorization;
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

// openInboxChangedStream uses fetch rather than EventSource so the existing
// bearer-token refresh flow stays intact. The parser accepts only the compact
// notification.changed payload; no message content is present on this stream.
export function openInboxChangedStream(callbacks: InboxStreamCallbacks): InboxStreamHandle {
  const abortController = new AbortController();
  let reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  const done = (async () => {
    const url = new URL(`${BASE}/stream`, window.location.origin);
    const response = await openInboxStreamResponse(url, abortController);
    if (!response.ok || !response.body) {
      throw new Error(`消息提醒连接失败: ${response.status}`);
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
        const parsed = consumeInboxSseChunks(buffer, callbacks);
        buffer = parsed.rest;
      }
    } finally {
      try {
        await reader.cancel();
      } catch {
        // The browser may already have closed the reader.
      }
      try {
        reader.releaseLock();
      } catch {
        // Ignore a double release after an abort.
      }
    }
  })().catch((error: unknown) => {
    if (abortController.signal.aborted) {
      return;
    }
    callbacks.onError?.(error instanceof Error ? error : new Error(String(error)));
  });

  return {
    close: () => {
      void reader?.cancel().catch(() => undefined);
      abortController.abort();
    },
    done,
  };
}

function consumeInboxSseChunks(rawBuffer: string, callbacks: InboxStreamCallbacks) {
  let rest = rawBuffer;
  while (true) {
    const lf = rest.indexOf('\n\n');
    const crlf = rest.indexOf('\r\n\r\n');
    if (lf < 0 && crlf < 0) {
      break;
    }
    const useCrLf = crlf >= 0 && (lf < 0 || crlf < lf);
    const splitAt = useCrLf ? crlf : lf;
    const delimiterLength = useCrLf ? 4 : 2;
    const chunk = rest.slice(0, splitAt);
    rest = rest.slice(splitAt + delimiterLength);
    consumeInboxSseChunk(chunk, callbacks);
  }
  return { rest };
}

function consumeInboxSseChunk(chunk: string, callbacks: InboxStreamCallbacks) {
  const lines = chunk
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith(':'));
  if (!lines.length) {
    return;
  }
  let eventType = 'message';
  const data: string[] = [];
  for (const line of lines) {
    if (line.startsWith('event:')) {
      eventType = line.slice('event:'.length).trim();
    } else if (line.startsWith('data:')) {
      data.push(line.slice('data:'.length).trimStart());
    }
  }
  if (eventType === 'connected') {
    callbacks.onConnected?.();
    return;
  }
  if (eventType === 'heartbeat') {
    callbacks.onHeartbeat?.();
    return;
  }
  if (eventType !== 'notification.changed' || data.length === 0) {
    return;
  }
  try {
    const parsed = JSON.parse(data.join('\n')) as Partial<InboxRealtimeHint>;
    if (typeof parsed.changeToken !== 'string' || !parsed.changeToken || typeof parsed.newUnread !== 'boolean') {
      throw new Error('消息提醒数据格式错误');
    }
    callbacks.onHint({ changeToken: parsed.changeToken, newUnread: parsed.newUnread });
  } catch (error) {
    callbacks.onError?.(error instanceof Error ? error : new Error(String(error)));
  }
}
