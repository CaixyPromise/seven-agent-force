import { create } from 'zustand';
import type { InboxPreviewItem } from '@/api/inboxController';
import {
  activateInboxPreviewAccount,
  beginInboxPreviewLoad,
  emptyInboxPreviewCache,
  failInboxPreviewLoad,
  invalidateInboxPreview,
  removeInboxPreviewRecipient,
  resolveInboxPreviewLoad,
  type InboxPreviewCache,
  type InboxPreviewLoadState,
} from '@/components/notification/inboxPreviewState';

export type { InboxPreviewLoadState } from '@/components/notification/inboxPreviewState';

interface NotificationInboxState {
  accountKey: string | null;
  mailboxKey: string | null;
  messagePopoverList: InboxPreviewItem[];
  previewLoading: boolean;
  previewLoaded: boolean;
  previewState: InboxPreviewLoadState;
  requestGeneration: number;
  activateAccount: (accountKey: string | null) => void;
  setMailboxKey: (accountKey: string, mailboxKey: string) => void;
  beginPreviewLoad: (accountKey: string) => number | null;
  resolvePreviewLoad: (
    accountKey: string,
    requestGeneration: number,
    expectedMailboxKey: string | null,
    mailboxKey: string,
    records: InboxPreviewItem[],
  ) => void;
  failPreviewLoad: (accountKey: string, requestGeneration: number) => void;
  invalidatePreview: (accountKey: string) => void;
  removePreviewRecipient: (accountKey: string, recipientId: string) => void;
}

// This is intentionally in-memory only. A message preview belongs to the
// current login session and must not survive logout or a different account.
export const useNotificationInboxStore = create<NotificationInboxState>((set, get) => ({
  ...toStorePatch(emptyInboxPreviewCache()),
  activateAccount: (accountKey) => {
    const current = get();
    const cache = toPreviewCache(current);
    const next = activateInboxPreviewAccount(cache, accountKey);
    if (next === cache) {
      return;
    }
    set(toStorePatch(next));
  },
  setMailboxKey: (accountKey, mailboxKey) => {
    if (!mailboxKey || get().accountKey !== accountKey) {
      return;
    }
    set({ mailboxKey });
  },
  beginPreviewLoad: (accountKey) => {
    const current = get();
    const result = beginInboxPreviewLoad(toPreviewCache(current), accountKey);
    if (result.requestGeneration === null) {
      return null;
    }
    set(toStorePatch(result.next));
    return result.requestGeneration;
  },
  resolvePreviewLoad: (accountKey, requestGeneration, expectedMailboxKey, mailboxKey, records) => {
    const current = get();
    const cache = toPreviewCache(current);
    const next = resolveInboxPreviewLoad(cache, {
      accountKey,
      requestGeneration,
      expectedMailboxKey,
      mailboxKey,
      records,
    });
    if (next === cache) {
      return;
    }
    set(toStorePatch(next));
  },
  failPreviewLoad: (accountKey, requestGeneration) => {
    const current = get();
    const cache = toPreviewCache(current);
    const next = failInboxPreviewLoad(cache, accountKey, requestGeneration);
    if (next === cache) {
      return;
    }
    set(toStorePatch(next));
  },
  invalidatePreview: (accountKey) => {
    const current = get();
    const cache = toPreviewCache(current);
    const next = invalidateInboxPreview(cache, accountKey);
    if (next === cache) {
      return;
    }
    set(toStorePatch(next));
  },
  removePreviewRecipient: (accountKey, recipientId) => {
    const current = get();
    const cache = toPreviewCache(current);
    const next = removeInboxPreviewRecipient(cache, accountKey, recipientId);
    if (next === cache) {
      return;
    }
    set(toStorePatch(next));
  },
}));

function toPreviewCache(state: Pick<
  NotificationInboxState,
  'accountKey' | 'mailboxKey' | 'messagePopoverList' | 'previewState' | 'requestGeneration'
>): InboxPreviewCache {
  return {
    accountKey: state.accountKey,
    mailboxKey: state.mailboxKey,
    messagePopoverList: state.messagePopoverList,
    previewState: state.previewState,
    requestGeneration: state.requestGeneration,
  };
}

function toStorePatch(cache: InboxPreviewCache) {
  return {
    ...cache,
    previewLoading: cache.previewState === 'loading',
    previewLoaded: cache.previewState === 'ready' || cache.previewState === 'empty',
  };
}
