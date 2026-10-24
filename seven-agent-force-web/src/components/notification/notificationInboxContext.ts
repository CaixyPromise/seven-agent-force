import { createContext, useContext } from 'react';
import type { InboxPreviewItem, InboxRealtimeHint, InboxUnreadCount } from '@/api/inboxController';

export const INBOX_QUERY_ROOT = ['notification', 'inbox'] as const;

export const unreadCountQueryKey = (accountKey: string) =>
  [...INBOX_QUERY_ROOT, 'unread-count', accountKey] as const;

export interface InboxRealtimeNotice extends InboxRealtimeHint {
  /** Client-local monotonic render key, not a database or API identity. */
  id: number;
  /** Internal account binding; it is never received from the API. */
  accountKey: string;
}

export interface NotificationInboxContextValue {
  accountKey: string | null;
  mailboxKey: string | null;
  unreadCount: number;
  unreadCountLoading: boolean;
  lastNotice: InboxRealtimeNotice | null;
  messagePopoverList: InboxPreviewItem[];
  previewLoading: boolean;
  previewLoaded: boolean;
  previewGeneration: number;
  /**
   * Returns false as soon as logout or an account change makes a callback
   * stale. Callers use this before applying an async result to visible state.
   */
  isCurrentSession: () => boolean;
  refreshUnreadCount: () => Promise<InboxUnreadCount | null>;
  loadPreview: () => Promise<void>;
  removePreviewRecipient: (recipientId: string) => void;
}

export const NotificationInboxContext = createContext<NotificationInboxContextValue | null>(null);

export function useNotificationInbox() {
  const value = useContext(NotificationInboxContext);
  if (!value) {
    throw new Error('useNotificationInbox must be used within NotificationRealtimeProvider');
  }
  return value;
}
