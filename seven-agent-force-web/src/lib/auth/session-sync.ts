const AUTH_SESSION_SYNC_CHANNEL = 'seven-auth-session';
const AUTH_SESSION_SYNC_STORAGE_KEY = 'seven:auth:session:event';

export type AuthSessionSyncEvent =
  | {
      type: 'logout';
      at: number;
    };

function createChannel() {
  if (typeof window === 'undefined' || typeof BroadcastChannel === 'undefined') {
    return null;
  }
  return new BroadcastChannel(AUTH_SESSION_SYNC_CHANNEL);
}

export function broadcastAuthSessionEvent(event: AuthSessionSyncEvent) {
  if (typeof window === 'undefined') {
    return;
  }
  const payload = JSON.stringify(event);
  const channel = createChannel();
  try {
    channel?.postMessage(event);
  } finally {
    channel?.close();
  }
  window.localStorage.setItem(AUTH_SESSION_SYNC_STORAGE_KEY, payload);
  window.localStorage.removeItem(AUTH_SESSION_SYNC_STORAGE_KEY);
}

export function subscribeAuthSessionEvents(handler: (event: AuthSessionSyncEvent) => void) {
  if (typeof window === 'undefined') {
    return () => {};
  }

  const channel = createChannel();
  const handleChannelMessage = (messageEvent: MessageEvent<AuthSessionSyncEvent>) => {
    if (messageEvent.data?.type) {
      handler(messageEvent.data);
    }
  };
  const handleStorageEvent = (storageEvent: StorageEvent) => {
    if (storageEvent.key !== AUTH_SESSION_SYNC_STORAGE_KEY || !storageEvent.newValue) {
      return;
    }
    try {
      const event = JSON.parse(storageEvent.newValue) as AuthSessionSyncEvent;
      if (event?.type) {
        handler(event);
      }
    } catch {
      // ignore malformed cross-tab event payloads
    }
  };

  channel?.addEventListener('message', handleChannelMessage);
  window.addEventListener('storage', handleStorageEvent);

  return () => {
    channel?.removeEventListener('message', handleChannelMessage);
    channel?.close();
    window.removeEventListener('storage', handleStorageEvent);
  };
}
