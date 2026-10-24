import { conversationText, decimal, record, ConversationContractError } from './conversationContract';

/** Optional server batch projection. Missing fields mean unavailable, never an inferred zero. */
export type TaskConversationSummary = {
  pendingActionCount?: number;
  hasUnreadUpdates?: boolean;
  activitySummary?: string;
  firstPendingActionKind?: 'CONVERSATION_ITEM' | 'COMPLETION_CHECK' | 'DELIVERY_REQUEST';
  firstPendingActionId?: string;
};

export function normalizeTaskConversationSummary(value: unknown): TaskConversationSummary {
  const data = record(value);
  const summary: TaskConversationSummary = {};
  if (data.pendingActionCount !== undefined) {
    if (typeof data.pendingActionCount !== 'number' || !Number.isSafeInteger(data.pendingActionCount) || data.pendingActionCount < 0) throw new ConversationContractError();
    summary.pendingActionCount = data.pendingActionCount;
  }
  if (data.hasUnreadUpdates !== undefined) {
    if (typeof data.hasUnreadUpdates !== 'boolean') throw new ConversationContractError();
    summary.hasUnreadUpdates = data.hasUnreadUpdates;
  }
  if (data.activitySummary !== undefined) summary.activitySummary = conversationText(data.activitySummary, 4096);
  if (data.firstPendingActionKind !== undefined) {
    if (data.firstPendingActionKind !== 'CONVERSATION_ITEM' && data.firstPendingActionKind !== 'COMPLETION_CHECK' && data.firstPendingActionKind !== 'DELIVERY_REQUEST') throw new ConversationContractError();
    summary.firstPendingActionKind = data.firstPendingActionKind;
  }
  if (data.firstPendingActionId !== undefined && data.firstPendingActionId !== null) summary.firstPendingActionId = decimal(data.firstPendingActionId);
  if (data.firstPendingActionId === null) summary.firstPendingActionId = undefined;
  if ((summary.firstPendingActionKind === undefined) !== (summary.firstPendingActionId === undefined)) throw new ConversationContractError();
  if (summary.firstPendingActionId && summary.pendingActionCount === 0) throw new ConversationContractError();
  return summary;
}

export function pendingItemLink(taskId: string, summary: TaskConversationSummary): string | undefined {
  if (!summary.firstPendingActionKind || !summary.firstPendingActionId || summary.pendingActionCount === undefined || summary.pendingActionCount <= 0) return undefined;
  const task = decimal(taskId);
  const action = decimal(summary.firstPendingActionId);
  if (summary.firstPendingActionKind === 'COMPLETION_CHECK') return `/workforce/tasks/${task}?tab=overview&pending=completion-check&checkpoint=${action}`;
  if (summary.firstPendingActionKind === 'DELIVERY_REQUEST') return `/workforce/tasks/${task}?focus=delivery&delivery=${action}`;
  return `/workforce/tasks/${task}?item=${action}`;
}
