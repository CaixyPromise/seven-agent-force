import { request, refreshAccessToken } from './request';
import { useAuthStore } from '@/store/auth';
import { buildAuthorizationHeader, shouldRefreshAccessToken } from '@/lib/auth/token';
import { getOrCreateDeviceId } from '@/lib/auth/device';
import { conversationText, decimal, normalizeAnswers, normalizeConversationItem, normalizeConversationPage, record } from './conversationContract';
import { consumeEventFrames } from './conversationStream';

const base = (taskId: string) => `/api/workforce/tasks/${decimal(taskId)}`;
export async function getConversation(taskId: string, before?: string, signal?: AbortSignal) {
  const response = await request<{ data: unknown }>(`${base(taskId)}/conversation`, { params: { before: before === undefined ? undefined : decimal(before), limit: 20 }, signal });
  return normalizeConversationPage(response.data);
}
export async function getConversationItem(taskId: string, itemId: string, signal?: AbortSignal) {
  const response = await request<{ data: unknown }>(`${base(taskId)}/conversation/${decimal(itemId)}`, { signal });
  const item = normalizeConversationItem(response.data);
  if (item.id !== itemId) throw new Error('CONVERSATION_INVALID');
  return item;
}
export async function sendConversationMessage(taskId: string, text: string, idempotencyKey: string) {
  conversationText(text);
  if (!text.trim() || !idempotencyKey) throw new Error('CONVERSATION_INVALID');
  await request(`${base(taskId)}/messages`, { method: 'POST', data: { text, idempotencyKey } });
}
export async function respondToInteraction(taskId: string, interactionId: string, input: unknown) {
  const value = record(input);
  if (typeof value.idempotencyKey !== 'string' || !value.idempotencyKey.length) throw new Error('CONVERSATION_INVALID');
  await request(`${base(taskId)}/interactions/${decimal(interactionId)}/responses`, { method: 'POST', data: { expectedVersion: decimal(value.expectedVersion), idempotencyKey: value.idempotencyKey, answers: normalizeAnswers(value.answers) } });
}
export async function markConversationRead(taskId: string, watermark: string) {
  await request(`${base(taskId)}/conversation/read`, { method: 'POST', data: { watermark: decimal(watermark, true) } });
}
/** Existing event endpoint. Server publishes snapshot watermark in the same event sequence. */
export function watchConversation(taskId: string, changed: (itemIds?: string[]) => Promise<void>, interrupted: () => void, initialAfter = '0'): () => void {
  const controller = new AbortController();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let after = decimal(initialAfter, true);
  let failures = 0;
  const connect = async () => {
    try {
      const state = useAuthStore.getState();
      if (state.accessToken && shouldRefreshAccessToken(state.accessExpireAt)) await refreshAccessToken();
      if (controller.signal.aborted) return;
      const auth = useAuthStore.getState();
      const header = buildAuthorizationHeader(auth.accessToken, auth.tokenType);
      const response = await fetch(`${base(taskId)}/events/stream?after=${after}`, { credentials: 'include', signal: controller.signal, headers: { Accept: 'text/event-stream', 'X-Device-Id': getOrCreateDeviceId(), ...(header ? { Authorization: header } : {}) } });
      if (response.status === 401 && auth.accessToken) { await refreshAccessToken(); throw new Error('STREAM_RECONNECT'); }
      if (!response.ok || !response.headers.get('content-type')?.startsWith('text/event-stream') || !response.body) throw new Error('STREAM_UNAVAILABLE');
      // Reconcile on EVERY reconnect, including old question cards outside the latest page.
      await changed();
      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      try {
        while (!controller.signal.aborted) {
          const { value, done } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const parsed = consumeEventFrames(buffer, after);
          buffer = parsed.rest;
          if (parsed.after !== after) { await changed(parsed.itemIds); after = parsed.after; }
        }
      } finally { await reader.cancel().catch(() => undefined); reader.releaseLock(); }
      failures = 0;
    } catch { if (!controller.signal.aborted) { failures += 1; interrupted(); } }
    if (!controller.signal.aborted) timer = setTimeout(() => { void connect(); }, Math.min(30000, 1000 * 2 ** Math.min(failures, 5)));
  };
  void connect();
  return () => { controller.abort(); clearTimeout(timer); };
}
