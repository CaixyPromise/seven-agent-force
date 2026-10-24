'use client';

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type PropsWithChildren,
} from 'react';
import { notification } from 'antd';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  getInboxPreview,
  getInboxUnreadCount,
  openInboxChangedStream,
  type InboxRealtimeHint,
  type InboxUnreadCount,
} from '@/api/inboxController';
import { useAuthStore } from '@/store/auth';
import { useNotificationInboxStore } from '@/store/notificationInbox';
import {
  acceptInboxRealtimeHint,
  createInboxReconnectDecision,
  shouldApplyReconnectDecision,
} from '@/components/notification/inboxRealtimeState';
import {
  INBOX_QUERY_ROOT,
  NotificationInboxContext,
  type InboxRealtimeNotice,
  type NotificationInboxContextValue,
  unreadCountQueryKey,
} from '@/components/notification/notificationInboxContext';

function userAccountKey(user: ReturnType<typeof useAuthStore.getState>['user']) {
  if (!user?.id) {
    return null;
  }
  return String(user.id);
}

function sameActiveAccount(accountKey: string, authGeneration: number) {
  return userAccountKey(useAuthStore.getState().user) === accountKey
    && notificationAuthGeneration.current === authGeneration;
}

// This module-level ref lets fetch-SSE callbacks reject a late result after a
// logout or account switch even before React has finished unmounting effects.
const notificationAuthGeneration = { current: 0 };

export default function NotificationRealtimeProvider({ children }: PropsWithChildren) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const accountKey = userAccountKey(user);
  const lifecycleRef = useRef<{ accountKey: string | null | undefined; generation: number }>({
    accountKey: undefined,
    generation: 0,
  });
  if (lifecycleRef.current.accountKey !== accountKey) {
    lifecycleRef.current = {
      accountKey,
      generation: lifecycleRef.current.generation + 1,
    };
    notificationAuthGeneration.current = lifecycleRef.current.generation;
  }
  const sessionGeneration = lifecycleRef.current.generation;
  const activeStoreAccountKey = useNotificationInboxStore((state) => state.accountKey);
  const storedMailboxKey = useNotificationInboxStore((state) => state.mailboxKey);
  const messagePopoverList = useNotificationInboxStore((state) => state.messagePopoverList);
  const previewLoading = useNotificationInboxStore((state) => state.previewLoading);
  const previewLoaded = useNotificationInboxStore((state) => state.previewLoaded);
  const previewGeneration = useNotificationInboxStore((state) => state.requestGeneration);
  const [notificationApi, notificationHolder] = notification.useNotification({
    placement: 'topRight',
    maxCount: 1,
  });
  const notificationApiRef = useRef(notificationApi);
  notificationApiRef.current = notificationApi;
  const [lastNotice, setLastNotice] = useState<InboxRealtimeNotice | null>(null);
  const previewAbortRef = useRef<AbortController | null>(null);
  const promptTimerRef = useRef<number | null>(null);
  const promptKeyRef = useRef<string | null>(null);
  const noticeIDRef = useRef(0);
  const promptGenerationRef = useRef<number | null>(null);
  const recentHintTokensRef = useRef<string[]>([]);
  const observedUnreadCountRef = useRef<number | null>(null);
  const hintInvalidatedPreviewRef = useRef(false);
  const queryClientRef = useRef(queryClient);
  queryClientRef.current = queryClient;

  const unreadCountQuery = useQuery({
    queryKey: accountKey ? unreadCountQueryKey(accountKey) : [...INBOX_QUERY_ROOT, 'unread-count', 'anonymous'],
    queryFn: ({ signal }) => getInboxUnreadCount(signal),
    enabled: Boolean(accountKey),
    staleTime: 15_000,
    refetchInterval: accountKey ? 60_000 : false,
    retry: 1,
  });
  const refetchUnreadCountQuery = unreadCountQuery.refetch;

  const destroyPrompt = useCallback(() => {
    if (promptTimerRef.current !== null) {
      window.clearTimeout(promptTimerRef.current);
      promptTimerRef.current = null;
    }
    if (promptKeyRef.current) {
      notificationApiRef.current.destroy(promptKeyRef.current);
      promptKeyRef.current = null;
    }
  }, []);
  const destroyPromptRef = useRef(destroyPrompt);
  destroyPromptRef.current = destroyPrompt;

  const showPrompt = useCallback((
    kind: 'initial' | 'new',
    unreadCount?: number,
    mailboxKey?: string,
  ) => {
    if (!accountKey) {
      return;
    }
    const promptGeneration = notificationAuthGeneration.current;
    const key = `inbox-unread:${mailboxKey ?? accountKey}:${promptGeneration}`;
    promptKeyRef.current = key;
    notificationApiRef.current.open({
      key,
      title: kind === 'new' ? '你有新的未读消息' : `你有 ${unreadCount ?? 0} 条未读消息`,
      description: '查看消息中心',
      duration: 6,
      role: 'status',
      showProgress: true,
      pauseOnHover: true,
      onClick: () => {
        notificationApiRef.current.destroy(key);
        if (!sameActiveAccount(accountKey, promptGeneration)) {
          return;
        }
        navigate('/notifications');
      },
      style: {
        cursor: 'pointer',
        borderRadius: 14,
        boxShadow: '0 10px 30px rgba(15, 23, 42, 0.14)',
        border: '1px solid rgba(0, 122, 255, 0.14)',
      },
    });
  }, [accountKey, navigate]);

  // Account cleanup is layout-synchronous so an old Popover or toast cannot
  // become visible for the next account between React committing and painting.
  useLayoutEffect(() => {
    const nextGeneration = lifecycleRef.current.generation;
    previewAbortRef.current?.abort();
    previewAbortRef.current = null;
    destroyPromptRef.current();
    useNotificationInboxStore.getState().activateAccount(accountKey);
    setLastNotice(null);
    promptGenerationRef.current = null;
    recentHintTokensRef.current = [];
    observedUnreadCountRef.current = null;
    hintInvalidatedPreviewRef.current = false;
    void queryClientRef.current.cancelQueries({ queryKey: INBOX_QUERY_ROOT });
    queryClientRef.current.removeQueries({ queryKey: INBOX_QUERY_ROOT });
    if (!accountKey) {
      return undefined;
    }
    return () => {
      if (notificationAuthGeneration.current === nextGeneration) {
        previewAbortRef.current?.abort();
      }
    };
  }, [accountKey]);

  useEffect(() => {
    if (!accountKey || !unreadCountQuery.data || activeStoreAccountKey !== accountKey) {
      return;
    }
    useNotificationInboxStore.getState().setMailboxKey(accountKey, unreadCountQuery.data.mailboxKey);
  }, [accountKey, activeStoreAccountKey, unreadCountQuery.data]);

  useEffect(() => {
    if (!accountKey || !unreadCountQuery.isSuccess || promptGenerationRef.current === notificationAuthGeneration.current) {
      return;
    }
    promptGenerationRef.current = notificationAuthGeneration.current;
    if ((unreadCountQuery.data?.unreadCount ?? 0) > 0) {
      showPrompt(
        'initial',
        unreadCountQuery.data?.unreadCount,
        unreadCountQuery.data?.mailboxKey,
      );
    }
  }, [
    accountKey,
    showPrompt,
    unreadCountQuery.data?.mailboxKey,
    unreadCountQuery.data?.unreadCount,
    unreadCountQuery.isSuccess,
  ]);

  useEffect(() => {
    if (!accountKey || !unreadCountQuery.isSuccess || activeStoreAccountKey !== accountKey) {
      return;
    }
    const nextCount = unreadCountQuery.data?.unreadCount ?? 0;
    const previousCount = observedUnreadCountRef.current;
    observedUnreadCountRef.current = nextCount;
    const alreadyInvalidatedByHint = hintInvalidatedPreviewRef.current;
    hintInvalidatedPreviewRef.current = false;
    if (previousCount !== null && nextCount > previousCount && !alreadyInvalidatedByHint) {
      previewAbortRef.current?.abort();
      useNotificationInboxStore.getState().invalidatePreview(accountKey);
    }
  }, [
    accountKey,
    activeStoreAccountKey,
    unreadCountQuery.data?.unreadCount,
    unreadCountQuery.dataUpdatedAt,
    unreadCountQuery.isSuccess,
  ]);

  const refreshUnreadCount = useCallback(async (): Promise<InboxUnreadCount | null> => {
    if (!accountKey || !sameActiveAccount(accountKey, sessionGeneration)) {
      return null;
    }
    const result = await refetchUnreadCountQuery();
    if (!sameActiveAccount(accountKey, sessionGeneration)) {
      return null;
    }
    return result.isSuccess ? result.data ?? null : null;
  }, [accountKey, refetchUnreadCountQuery, sessionGeneration]);

  const handleHint = useCallback((hint: InboxRealtimeHint, streamGeneration: number) => {
    if (!accountKey || !sameActiveAccount(accountKey, streamGeneration)) {
      return;
    }
    const decision = acceptInboxRealtimeHint(recentHintTokensRef.current, hint);
    if (decision.duplicate) {
      return;
    }
    recentHintTokensRef.current = decision.recentChangeTokens;
    noticeIDRef.current += 1;
    setLastNotice({ ...hint, id: noticeIDRef.current, accountKey });
    if (decision.invalidatePreview) {
      hintInvalidatedPreviewRef.current = true;
      previewAbortRef.current?.abort();
      useNotificationInboxStore.getState().invalidatePreview(accountKey);
    }
    const refresh = refreshUnreadCount();
    if (!decision.showNewUnreadPrompt) {
      return;
    }
    void refresh.then((count) => {
      if (!sameActiveAccount(accountKey, streamGeneration) || promptTimerRef.current !== null) {
        return;
      }
      if (!count || count.unreadCount <= 0) {
        return;
      }
      promptTimerRef.current = window.setTimeout(() => {
        promptTimerRef.current = null;
        if (sameActiveAccount(accountKey, streamGeneration)) {
          showPrompt('new', undefined, count.mailboxKey);
        }
      }, 120);
    });
  }, [accountKey, refreshUnreadCount, showPrompt]);

  const recoverMailboxAfterReconnect = useCallback(
    async (streamGeneration: number) => {
      const noticeIDAtRefreshStart = noticeIDRef.current;
      const count = await refreshUnreadCount();
      if (
        !accountKey
        || !count
        || !sameActiveAccount(accountKey, streamGeneration)
        || !shouldApplyReconnectDecision(noticeIDAtRefreshStart, noticeIDRef.current)
      ) {
        return;
      }
      const decision = createInboxReconnectDecision(count.changeToken);
      if (!decision) {
        return;
      }
      hintInvalidatedPreviewRef.current = true;
      previewAbortRef.current?.abort();
      useNotificationInboxStore.getState().invalidatePreview(accountKey);
      noticeIDRef.current += 1;
      setLastNotice({
        changeToken: decision.changeToken,
        newUnread: decision.showNewUnreadPrompt,
        id: noticeIDRef.current,
        accountKey,
      });
    },
    [accountKey, refreshUnreadCount],
  );

  useEffect(() => {
    if (!accountKey) {
      return undefined;
    }
    const streamGeneration = notificationAuthGeneration.current;
    let disposed = false;
    let reconnectTimer: number | null = null;
    let retryDelay = 1000;
    let stream: ReturnType<typeof openInboxChangedStream> | null = null;

    const clearReconnect = () => {
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
    };
    const scheduleReconnect = () => {
      if (disposed || document.hidden || reconnectTimer !== null) {
        return;
      }
      reconnectTimer = window.setTimeout(() => {
        reconnectTimer = null;
        connect();
      }, retryDelay);
      retryDelay = Math.min(retryDelay * 2, 15_000);
    };
    const connect = () => {
      if (disposed || document.hidden || stream || !sameActiveAccount(accountKey, streamGeneration)) {
        return;
      }
      const openedStream = openInboxChangedStream({
        onConnected: () => {
          retryDelay = 1000;
          void recoverMailboxAfterReconnect(streamGeneration);
        },
        onHint: (hint) => handleHint(hint, streamGeneration),
        onError: () => undefined,
      });
      stream = openedStream;
      void openedStream.done.finally(() => {
        if (stream !== openedStream) {
          return;
        }
        stream = null;
        scheduleReconnect();
      });
    };
    const onVisibilityChange = () => {
      if (document.hidden) {
        clearReconnect();
        stream?.close();
        stream = null;
        return;
      }
      void refreshUnreadCount();
      connect();
    };

    document.addEventListener('visibilitychange', onVisibilityChange);
    connect();
    return () => {
      disposed = true;
      clearReconnect();
      document.removeEventListener('visibilitychange', onVisibilityChange);
      stream?.close();
    };
  }, [accountKey, handleHint, recoverMailboxAfterReconnect, refreshUnreadCount]);

  const loadPreview = useCallback(async () => {
    if (!accountKey || activeStoreAccountKey !== accountKey) {
      return;
    }
    const inboxState = useNotificationInboxStore.getState();
    const requestGeneration = inboxState.beginPreviewLoad(accountKey);
    if (requestGeneration === null) {
      return;
    }
    const expectedMailboxKey = inboxState.mailboxKey;
    const requestAuthGeneration = notificationAuthGeneration.current;
    previewAbortRef.current?.abort();
    const controller = new AbortController();
    previewAbortRef.current = controller;
    try {
      const preview = await getInboxPreview(controller.signal);
      if (!sameActiveAccount(accountKey, requestAuthGeneration)) {
        return;
      }
      useNotificationInboxStore.getState().resolvePreviewLoad(
        accountKey,
        requestGeneration,
        expectedMailboxKey,
        preview.mailboxKey,
        preview.records,
      );
    } catch {
      if (controller.signal.aborted) {
        return;
      }
      useNotificationInboxStore.getState().failPreviewLoad(accountKey, requestGeneration);
    } finally {
      if (previewAbortRef.current === controller) {
        previewAbortRef.current = null;
      }
    }
  }, [accountKey, activeStoreAccountKey]);

  const removePreviewRecipient = useCallback((recipientId: string) => {
    if (!accountKey) {
      return;
    }
    useNotificationInboxStore.getState().removePreviewRecipient(accountKey, recipientId);
  }, [accountKey]);

  const isCurrentSession = useCallback(
    () => Boolean(accountKey && sameActiveAccount(accountKey, sessionGeneration)),
    [accountKey, sessionGeneration],
  );

  const contextValue = useMemo<NotificationInboxContextValue>(() => ({
    accountKey,
    mailboxKey: unreadCountQuery.data?.mailboxKey ?? (activeStoreAccountKey === accountKey ? storedMailboxKey : null),
    unreadCount: unreadCountQuery.data?.unreadCount ?? 0,
    unreadCountLoading: unreadCountQuery.isLoading,
    lastNotice: lastNotice?.accountKey === accountKey ? lastNotice : null,
    messagePopoverList: activeStoreAccountKey === accountKey ? messagePopoverList : [],
    previewLoading: activeStoreAccountKey === accountKey && previewLoading,
    previewLoaded: activeStoreAccountKey === accountKey && previewLoaded,
    previewGeneration: activeStoreAccountKey === accountKey ? previewGeneration : 0,
    isCurrentSession,
    refreshUnreadCount,
    loadPreview,
    removePreviewRecipient,
  }), [
    accountKey,
    activeStoreAccountKey,
    lastNotice,
    isCurrentSession,
    loadPreview,
    messagePopoverList,
    previewGeneration,
    previewLoaded,
    previewLoading,
    refreshUnreadCount,
    removePreviewRecipient,
    storedMailboxKey,
    unreadCountQuery.data?.mailboxKey,
    unreadCountQuery.data?.unreadCount,
    unreadCountQuery.isLoading,
  ]);

  return (
    <NotificationInboxContext.Provider value={contextValue}>
      {notificationHolder}
      {children}
    </NotificationInboxContext.Provider>
  );
}
