import type { InboxListItem } from '@/api/inboxController';

function inCurrentView(item: InboxListItem, archived: boolean) {
  return archived ? Boolean(item.archivedAt) : !item.archivedAt;
}

// mergeInboxRecords applies one authoritative delta page to the open view.
// Removal markers intentionally contain only recipient IDs; an expired item
// never needs another card or detail payload to disappear from the screen.
export function mergeInboxRecords(
  records: InboxListItem[],
  upserts: InboxListItem[],
  removedRecipientIds: string[],
  archived: boolean,
) {
  const byRecipient = new Map(records.map((item) => [item.recipientId, item]));
  for (const recipientId of removedRecipientIds) {
    byRecipient.delete(recipientId);
  }
  for (const item of upserts) {
    if (inCurrentView(item, archived)) {
      byRecipient.set(item.recipientId, item);
    } else {
      byRecipient.delete(item.recipientId);
    }
  }
  return [...byRecipient.values()].sort((left, right) => {
    const leftTime = new Date(left.createTime).getTime();
    const rightTime = new Date(right.createTime).getTime();
    return rightTime - leftTime || right.recipientId.localeCompare(left.recipientId);
  });
}

export function shouldClearExpandedRecipient(
  expandedRecipient: { accountKey: string; recipientId: string } | null,
  accountKey: string | null,
  removedRecipientIds: string[],
) {
  return Boolean(
    expandedRecipient
      && expandedRecipient.accountKey === accountKey
      && removedRecipientIds.includes(expandedRecipient.recipientId),
  );
}
