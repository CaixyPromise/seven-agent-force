'use client';

import {
  ArrowRightOutlined,
  InboxOutlined,
  RollbackOutlined,
} from '@ant-design/icons';
import { Alert, Button, Empty, Skeleton, Tabs, Typography, message } from 'antd';
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useLocation, useNavigate } from 'react-router-dom';
import {
  archiveInboxRecipient,
  getInboxDetail,
  listInbox,
  listInboxChanges,
  markInboxRead,
  markInboxUnread,
  restoreInboxRecipient,
  type InboxDetail,
  type InboxListItem,
} from '@/api/inboxController';
import {
  mergeInboxRecords,
  shouldClearExpandedRecipient,
} from '@/components/notification/inboxChangeState';
import { INBOX_QUERY_ROOT, useNotificationInbox } from '@/components/notification/notificationInboxContext';
import { isTaskDeepLink, safeInternalPath } from '@/lib/safeInternalPath';

type MailboxView = 'inbox' | 'archived';

interface MailboxPageState {
  key: string;
  records: InboxListItem[];
  nextPageCursor?: string;
  changeToken: string;
}

const PAGE_SIZE = 20;

function formatMessageTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function MessageListSkeleton() {
  return (
    <div style={{ display: 'grid', gap: 12 }} aria-label="正在加载消息">
      {[0, 1, 2, 3].map((index) => (
        <div key={index} style={{ padding: '16px 2px', borderBottom: '1px solid rgba(15, 23, 42, 0.08)' }}>
          <Skeleton.Input active size="small" style={{ width: index % 2 ? '46%' : '62%' }} />
          <div style={{ height: 8 }} />
          <Skeleton.Input active size="small" style={{ width: '22%' }} />
        </div>
      ))}
    </div>
  );
}

export default function NotificationsPage() {
  const {
    accountKey,
    mailboxKey,
    lastNotice,
    unreadCount,
    unreadCountLoading,
    refreshUnreadCount,
    removePreviewRecipient,
    isCurrentSession,
  } = useNotificationInbox();
  const queryClient = useQueryClient();
  const location = useLocation();
  const navigate = useNavigate();
  const [view, setView] = useState<MailboxView>('inbox');
  const [pageState, setPageState] = useState<MailboxPageState | null>(null);
  const [expandedRecipient, setExpandedRecipient] = useState<{
    accountKey: string;
    recipientId: string;
  } | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);
  const [mutatingRecipientId, setMutatingRecipientId] = useState<string | null>(null);
  const pageStateRef = useRef<MailboxPageState | null>(null);
  const deltaRunRef = useRef(0);
  const markedReadRef = useRef(new Set<string>());
  const archived = view === 'archived';
  const currentPageKey = mailboxKey ? `${mailboxKey}:${view}` : null;
  const expandedRecipientId = expandedRecipient?.accountKey === accountKey
    ? expandedRecipient.recipientId
    : null;
  const requestedRecipientId = useMemo(
    () => new URLSearchParams(location.search).get('message')?.trim() || null,
    [location.search],
  );

  const setCurrentPageState = useCallback((update: MailboxPageState | null | ((current: MailboxPageState | null) => MailboxPageState | null)) => {
    setPageState((current) => {
      const next = typeof update === 'function' ? update(current) : update;
      pageStateRef.current = next;
      return next;
    });
  }, []);

  // Keep every visible value account-bound before a new account can paint. The
  // account tag on expandedRecipient also prevents a stale A detail request
  // from being enabled while B's layout reset is being applied.
  useLayoutEffect(() => {
    deltaRunRef.current += 1;
    markedReadRef.current.clear();
    setMutatingRecipientId(null);
    setExpandedRecipient(null);
    setCurrentPageState(null);
  }, [accountKey, setCurrentPageState]);

  const inboxPageQuery = useQuery({
    queryKey: mailboxKey
      ? [...INBOX_QUERY_ROOT, 'mailbox', mailboxKey, 'list', view]
      : [...INBOX_QUERY_ROOT, 'mailbox', 'unavailable', 'list', view],
    queryFn: ({ signal }) => listInbox({ archived, pageSize: PAGE_SIZE }, signal),
    enabled: Boolean(mailboxKey),
    staleTime: 10_000,
  });
  const refetchInboxPage = inboxPageQuery.refetch;

  useEffect(() => {
    if (!currentPageKey || !inboxPageQuery.data) {
      return;
    }
    const data = inboxPageQuery.data;
    setCurrentPageState({
      key: currentPageKey,
      records: data.records,
      nextPageCursor: data.nextPageCursor,
      changeToken: data.changeToken,
    });
    setExpandedRecipient(null);
    markedReadRef.current.clear();
  }, [currentPageKey, inboxPageQuery.data, setCurrentPageState]);

  useEffect(() => {
    if (!currentPageKey) {
      setCurrentPageState(null);
      setExpandedRecipient(null);
    }
  }, [currentPageKey, setCurrentPageState]);

  useEffect(() => {
    if (!requestedRecipientId || pageState?.key !== currentPageKey) {
      return;
    }
    if (accountKey && pageState.records.some((item) => item.recipientId === requestedRecipientId)) {
      setExpandedRecipient({ accountKey, recipientId: requestedRecipientId });
    }
  }, [accountKey, currentPageKey, pageState, requestedRecipientId]);

  const expandedDetailQuery = useQuery({
    queryKey: expandedRecipientId && mailboxKey
      ? [...INBOX_QUERY_ROOT, 'mailbox', mailboxKey, 'detail', expandedRecipientId]
      : [...INBOX_QUERY_ROOT, 'mailbox', 'unavailable', 'detail'],
    queryFn: ({ signal }) => getInboxDetail(expandedRecipientId ?? '', signal),
    enabled: Boolean(expandedRecipientId && mailboxKey),
    staleTime: 0,
  });

  const applyMutation = useCallback(async (
    action: 'read' | 'unread' | 'archive' | 'restore',
    item: InboxListItem | InboxDetail,
  ) => {
    if (!mailboxKey || !currentPageKey || !isCurrentSession()) {
      return;
    }
    setMutatingRecipientId(item.recipientId);
    try {
      const request = { expectedMailboxVersion: item.mailboxVersion };
      const updated = action === 'read'
        ? await markInboxRead(item.recipientId, request)
        : action === 'unread'
          ? await markInboxUnread(item.recipientId, request)
          : action === 'archive'
            ? await archiveInboxRecipient(item.recipientId, request)
            : await restoreInboxRecipient(item.recipientId, request);
      if (!isCurrentSession()) {
        return;
      }
      if (action === 'read') {
        removePreviewRecipient(updated.recipientId);
      }
      setCurrentPageState((current) => {
        if (!current || current.key !== currentPageKey) {
          return current;
        }
        return {
          ...current,
          records: mergeInboxRecords(current.records, [updated], [], archived),
        };
      });
      queryClient.setQueryData<InboxDetail>(
        [...INBOX_QUERY_ROOT, 'mailbox', mailboxKey, 'detail', updated.recipientId],
        (previous) => previous ? { ...previous, ...updated } : previous,
      );
      if ((action === 'archive' && !archived) || (action === 'restore' && archived)) {
        setExpandedRecipient(null);
      }
      await refreshUnreadCount();
    } catch (error) {
      if (!isCurrentSession()) {
        return;
      }
      message.error(error instanceof Error ? error.message : '更新消息失败');
      await refetchInboxPage();
    } finally {
      if (isCurrentSession()) {
        setMutatingRecipientId(null);
      }
    }
  }, [
    archived,
    currentPageKey,
    mailboxKey,
    queryClient,
    refetchInboxPage,
    refreshUnreadCount,
    removePreviewRecipient,
    setCurrentPageState,
    isCurrentSession,
  ]);

  useEffect(() => {
    const detail = expandedDetailQuery.data;
    if (!detail || detail.readAt || markedReadRef.current.has(detail.recipientId)) {
      return;
    }
    markedReadRef.current.add(detail.recipientId);
    void applyMutation('read', detail);
  }, [applyMutation, expandedDetailQuery.data]);

  useEffect(() => {
    if (!lastNotice || !currentPageKey || !mailboxKey || !pageStateRef.current || !isCurrentSession()) {
      return;
    }
    const runID = ++deltaRunRef.current;
    const controller = new AbortController();
    const sync = async () => {
      const current = pageStateRef.current;
      if (!current || current.key !== currentPageKey || !current.changeToken) {
        return;
      }
      let afterChangeToken = current.changeToken;
      let hasMore = true;
      while (hasMore && !controller.signal.aborted && runID === deltaRunRef.current && isCurrentSession()) {
        const changes = await listInboxChanges({
          afterChangeToken,
          untilChangeToken: lastNotice.changeToken,
          limit: 50,
        }, controller.signal);
        if (!isCurrentSession()) {
          return;
        }
        if (changes.resyncRequired || (changes.mailboxKey && changes.mailboxKey !== mailboxKey)) {
          await refetchInboxPage();
          return;
        }
        const removedRecipientIds = changes.removedRecipientIds ?? [];
        if (removedRecipientIds.length > 0) {
          setExpandedRecipient((current) => (
            shouldClearExpandedRecipient(current, accountKey, removedRecipientIds) ? null : current
          ));
          for (const recipientId of removedRecipientIds) {
            queryClient.removeQueries({
              queryKey: [...INBOX_QUERY_ROOT, 'mailbox', mailboxKey, 'detail', recipientId],
              exact: true,
            });
          }
        }
        setCurrentPageState((previous) => {
          if (!previous || previous.key !== currentPageKey) {
            return previous;
          }
          return {
            ...previous,
            records: mergeInboxRecords(previous.records, changes.upserts, removedRecipientIds, archived),
            changeToken: changes.targetChangeToken || changes.nextChangeToken || previous.changeToken,
          };
        });
        hasMore = changes.hasMore && Boolean(changes.nextChangeToken);
        afterChangeToken = changes.nextChangeToken || afterChangeToken;
      }
    };
    void sync().catch(() => {
      if (!controller.signal.aborted && isCurrentSession()) {
        void refetchInboxPage();
      }
    });
    return () => controller.abort();
  }, [
    accountKey,
    archived,
    currentPageKey,
    isCurrentSession,
    lastNotice,
    mailboxKey,
    pageState?.key,
    queryClient,
    refetchInboxPage,
    setCurrentPageState,
  ]);

  const loadMore = async () => {
    const current = pageStateRef.current;
    if (!current?.nextPageCursor || current.key !== currentPageKey || loadingMore || !isCurrentSession()) {
      return;
    }
    setLoadingMore(true);
    try {
      const next = await listInbox({ archived, pageCursor: current.nextPageCursor, pageSize: PAGE_SIZE });
      if (!isCurrentSession()) {
        return;
      }
      setCurrentPageState((previous) => {
        if (!previous || previous.key !== current.key) {
          return previous;
        }
        return {
          ...previous,
          records: mergeInboxRecords(previous.records, next.records, [], archived),
          nextPageCursor: next.nextPageCursor,
          changeToken: next.changeToken,
        };
      });
    } catch (error) {
      if (!isCurrentSession()) {
        return;
      }
      message.error(error instanceof Error ? error.message : '加载消息失败');
    } finally {
      if (isCurrentSession()) {
        setLoadingMore(false);
      }
    }
  };

  const records = pageState?.key === currentPageKey ? pageState.records : [];
  const activeDetail = expandedDetailQuery.data;
  const headerText = useMemo(() => {
    if (view === 'archived') {
      return '已归档';
    }
    return unreadCount > 0 ? `${unreadCount} 条未读` : '全部已读';
  }, [unreadCount, view]);

  if (!accountKey) {
    return null;
  }

  return (
    <div style={{ maxWidth: 760, margin: '0 auto', padding: '16px 0 40px' }}>
      <div style={{ display: 'flex', alignItems: 'baseline', justifyContent: 'space-between', gap: 16, marginBottom: 18 }}>
        <div>
          <Typography.Title level={2} style={{ margin: 0, color: '#1d1d1f' }}>消息</Typography.Title>
          <Typography.Text type="secondary">{headerText}</Typography.Text>
        </div>
        <Button size="small" onClick={() => void inboxPageQuery.refetch()} loading={inboxPageQuery.isFetching}>
          刷新
        </Button>
      </div>

      <Tabs
        activeKey={view}
        onChange={(key) => setView(key as MailboxView)}
        items={[
          { key: 'inbox', label: '消息' },
          { key: 'archived', label: '已归档' },
        ]}
      />

      {!mailboxKey && (unreadCountLoading || !inboxPageQuery.isError) ? (
        <MessageListSkeleton />
      ) : inboxPageQuery.isError ? (
        <Alert
          type="error"
          showIcon
          title="消息加载失败"
          action={<Button size="small" onClick={() => void inboxPageQuery.refetch()}>重试</Button>}
        />
      ) : inboxPageQuery.isLoading || (pageState?.key !== currentPageKey && !inboxPageQuery.data) ? (
        <MessageListSkeleton />
      ) : records.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description={archived ? '没有已归档消息' : '暂无消息'}
          style={{ margin: '56px 0' }}
        />
      ) : (
        <div role="list">
          {records.map((item) => {
            const expanded = expandedRecipientId === item.recipientId;
            const detail = expanded && activeDetail?.recipientId === item.recipientId ? activeDetail : null;
            const detailDeepLink = detail?.deepLink;
            const primaryAction = detail?.primaryAction;
            const actionLink = safeInternalPath(primaryAction?.link) ? primaryAction?.link : detailDeepLink;
            const actionLabel = primaryAction?.label?.trim() || (isTaskDeepLink(actionLink) ? '查看任务' : '前往');
            return (
              <div key={item.recipientId} role="listitem" style={{ display: 'block', padding: 0, border: 0 }}>
                <button
                  type="button"
                  onClick={() => {
                    setExpandedRecipient(expanded || !accountKey ? null : { accountKey, recipientId: item.recipientId });
                    if (expanded && requestedRecipientId === item.recipientId) {
                      navigate('/notifications', { replace: true });
                    }
                  }}
                  aria-expanded={expanded}
                  style={{
                    width: '100%',
                    display: 'grid',
                    gap: 6,
                    textAlign: 'left',
                    padding: '16px 4px',
                    border: 0,
                    borderBottom: '1px solid rgba(15, 23, 42, 0.08)',
                    background: 'transparent',
                    cursor: 'pointer',
                  }}
                >
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 14 }}>
                    <Typography.Text strong={!item.readAt} style={{ color: '#1d1d1f' }}>
                      {item.title}
                    </Typography.Text>
                    <Typography.Text type="secondary" style={{ flexShrink: 0, fontSize: 12 }}>
                      {formatMessageTime(item.createTime)}
                    </Typography.Text>
                  </div>
                  <Typography.Text type="secondary" style={{ fontSize: 13 }}>
                    {item.summary || '打开查看详情'}
                  </Typography.Text>
                </button>

                {expanded && (
                  <div style={{ padding: '4px 4px 18px', borderBottom: '1px solid rgba(15, 23, 42, 0.08)' }}>
                    {expandedDetailQuery.isLoading ? (
                      <MessageListSkeleton />
                    ) : expandedDetailQuery.isError ? (
                      <Alert
                        type="error"
                        showIcon
                        title="消息详情加载失败"
                        action={<Button size="small" onClick={() => void expandedDetailQuery.refetch()}>重试</Button>}
                      />
                    ) : detail ? (
                      <>
                        <Typography.Paragraph style={{ margin: '8px 0 14px', whiteSpace: 'pre-wrap', color: '#334155' }}>
                          {detail.content}
                        </Typography.Paragraph>
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                          {detail.readAt && (
                            <Button
                              size="small"
                              loading={mutatingRecipientId === detail.recipientId}
                              onClick={() => void applyMutation('unread', detail)}
                            >
                              标为未读
                            </Button>
                          )}
                          {detail.archivedAt ? (
                            <Button
                              size="small"
                              icon={<RollbackOutlined />}
                              loading={mutatingRecipientId === detail.recipientId}
                              onClick={() => void applyMutation('restore', detail)}
                            >
                              恢复
                            </Button>
                          ) : (
                            <Button
                              size="small"
                              icon={<InboxOutlined />}
                              loading={mutatingRecipientId === detail.recipientId}
                              onClick={() => void applyMutation('archive', detail)}
                            >
                              归档
                            </Button>
                          )}
                          {safeInternalPath(actionLink) && (
                            <Button
                              size="small"
                              type={primaryAction ? 'primary' : 'link'}
                              icon={<ArrowRightOutlined />}
                              onClick={() => navigate(actionLink)}
                            >
                              {actionLabel}
                            </Button>
                          )}
                        </div>
                      </>
                    ) : null}
                  </div>
                )}
              </div>
            );
          })}
          {pageState?.key === currentPageKey && pageState.nextPageCursor && (
            <div style={{ paddingTop: 18, textAlign: 'center' }}>
              <Button loading={loadingMore} onClick={() => void loadMore()}>
                加载更多
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
