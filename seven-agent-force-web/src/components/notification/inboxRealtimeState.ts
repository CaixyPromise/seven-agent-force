export interface InboxRealtimeHintDecision {
  duplicate: boolean;
  recentChangeTokens: string[];
  invalidatePreview: boolean;
  showNewUnreadPrompt: boolean;
}

export interface InboxReconnectDecision {
  changeToken: string;
  invalidatePreview: true;
  requestDelta: true;
  showNewUnreadPrompt: false;
}

// acceptInboxRealtimeHint keeps the decision that is safe to make from a
// content-free SSE hint in one small, testable place. Every accepted mailbox
// change can make a previously opened unread preview stale; only a newly
// created unread recipient is allowed to ask for the calm new-message prompt.
export function acceptInboxRealtimeHint(
  recentChangeTokens: string[],
  hint: { changeToken: string; newUnread: boolean },
): InboxRealtimeHintDecision {
  if (recentChangeTokens.includes(hint.changeToken)) {
    return {
      duplicate: true,
      recentChangeTokens,
      invalidatePreview: false,
      showNewUnreadPrompt: false,
    };
  }
  return {
    duplicate: false,
    recentChangeTokens: [...recentChangeTokens, hint.changeToken].slice(-32),
    invalidatePreview: true,
    showNewUnreadPrompt: hint.newUnread,
  };
}

// A fetch-SSE reconnect has no replay guarantee. The current count response
// carries only an opaque mailbox token, which is enough for an open message
// center to request its durable delta and for the bell to discard a stale
// preview. It is deliberately quiet: reconnecting never implies a new message.
export function createInboxReconnectDecision(changeToken: string): InboxReconnectDecision | null {
  const token = changeToken.trim();
  if (!token) {
    return null;
  }
  return {
    changeToken: token,
    invalidatePreview: true,
    requestDelta: true,
    showNewUnreadPrompt: false,
  };
}

// A count response started before a real SSE hint must not move the page back
// to an older opaque token. Tokens are intentionally not ordered in the
// browser, so the local notice sequence is the only safe tie-breaker.
export function shouldApplyReconnectDecision(startNoticeID: number, currentNoticeID: number) {
  return startNoticeID === currentNoticeID;
}
