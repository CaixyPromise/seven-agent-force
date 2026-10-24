import type { InboxPreviewItem } from '@/api/inboxController';

export type InboxPreviewLoadState = 'notLoaded' | 'loading' | 'ready' | 'empty' | 'error';

export interface InboxPreviewCache {
  accountKey: string | null;
  mailboxKey: string | null;
  messagePopoverList: InboxPreviewItem[];
  previewState: InboxPreviewLoadState;
  requestGeneration: number;
}

export function emptyInboxPreviewCache(
  accountKey: string | null = null,
  requestGeneration = 0,
): InboxPreviewCache {
  return {
    accountKey,
    mailboxKey: null,
    messagePopoverList: [],
    previewState: 'notLoaded',
    requestGeneration,
  };
}

export function activateInboxPreviewAccount(
  current: InboxPreviewCache,
  accountKey: string | null,
): InboxPreviewCache {
  if (current.accountKey === accountKey) {
    return current;
  }
  return emptyInboxPreviewCache(accountKey, current.requestGeneration + 1);
}

export function beginInboxPreviewLoad(
  current: InboxPreviewCache,
  accountKey: string,
): { next: InboxPreviewCache; requestGeneration: number | null } {
  if (
    current.accountKey !== accountKey
    || current.previewState === 'loading'
    || current.previewState === 'ready'
    || current.previewState === 'empty'
    || current.messagePopoverList.length > 0
  ) {
    return { next: current, requestGeneration: null };
  }
  return {
    next: { ...current, previewState: 'loading' },
    requestGeneration: current.requestGeneration,
  };
}

export function resolveInboxPreviewLoad(
  current: InboxPreviewCache,
  request: {
    accountKey: string;
    requestGeneration: number;
    expectedMailboxKey: string | null;
    mailboxKey: string;
    records: InboxPreviewItem[];
  },
): InboxPreviewCache {
  if (
    current.accountKey !== request.accountKey
    || current.requestGeneration !== request.requestGeneration
    || (request.expectedMailboxKey !== null && current.mailboxKey !== request.expectedMailboxKey)
    || (current.mailboxKey !== null && request.mailboxKey !== current.mailboxKey)
  ) {
    return current;
  }
  return {
    ...current,
    mailboxKey: request.mailboxKey || current.mailboxKey,
    messagePopoverList: request.records,
    previewState: request.records.length > 0 ? 'ready' : 'empty',
  };
}

export function failInboxPreviewLoad(
  current: InboxPreviewCache,
  accountKey: string,
  requestGeneration: number,
): InboxPreviewCache {
  if (current.accountKey !== accountKey || current.requestGeneration !== requestGeneration) {
    return current;
  }
  return { ...current, previewState: 'error' };
}

export function invalidateInboxPreview(
  current: InboxPreviewCache,
  accountKey: string,
): InboxPreviewCache {
  if (current.accountKey !== accountKey) {
    return current;
  }
  return {
    ...current,
    messagePopoverList: [],
    previewState: 'notLoaded',
    requestGeneration: current.requestGeneration + 1,
  };
}

export function removeInboxPreviewRecipient(
  current: InboxPreviewCache,
  accountKey: string,
  recipientId: string,
): InboxPreviewCache {
  if (current.accountKey !== accountKey) {
    return current;
  }
  const records = current.messagePopoverList.filter((item) => item.recipientId !== recipientId);
  if (records.length > 0) {
    return { ...current, messagePopoverList: records };
  }
  return invalidateInboxPreview({ ...current, messagePopoverList: [] }, accountKey);
}
