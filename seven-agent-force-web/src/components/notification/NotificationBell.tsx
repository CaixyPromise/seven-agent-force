'use client';

import { BellOutlined } from '@ant-design/icons';
import { Badge, Button, Empty, Popover, Skeleton, Typography, message } from 'antd';
import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { markInboxRead } from '@/api/inboxController';
import { useNotificationInbox } from '@/components/notification/notificationInboxContext';

function formatPreviewTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date);
}

function PreviewSkeleton() {
  return (
    <div style={{ display: 'grid', gap: 14, padding: '4px 0' }} aria-label="正在加载消息预览">
      {[0, 1, 2].map((index) => (
        <div key={index} style={{ display: 'grid', gap: 8 }}>
          <Skeleton.Input active size="small" style={{ width: index === 1 ? '58%' : '76%' }} />
          <Skeleton.Input active size="small" style={{ width: '42%' }} />
        </div>
      ))}
    </div>
  );
}

export default function NotificationBell() {
  const {
    accountKey,
    unreadCount,
    messagePopoverList,
    previewLoading,
    previewLoaded,
    previewGeneration,
    loadPreview,
    refreshUnreadCount,
    removePreviewRecipient,
    isCurrentSession,
  } = useNotificationInbox();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [markingRecipientId, setMarkingRecipientId] = useState<string | null>(null);
  const previewAttemptGenerationRef = useRef<number | null>(null);

  useLayoutEffect(() => {
    setOpen(false);
    previewAttemptGenerationRef.current = null;
  }, [accountKey]);

  useEffect(() => {
    if (
      !open
      || !isCurrentSession()
      || messagePopoverList.length > 0
      || previewLoading
      || previewLoaded
      || previewAttemptGenerationRef.current === previewGeneration
    ) {
      return;
    }
    previewAttemptGenerationRef.current = previewGeneration;
    void loadPreview();
  }, [
    loadPreview,
    messagePopoverList.length,
    open,
    previewGeneration,
    previewLoaded,
    previewLoading,
    isCurrentSession,
  ]);

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen && !isCurrentSession()) {
      return;
    }
    setOpen(nextOpen);
    if (!nextOpen) {
      previewAttemptGenerationRef.current = null;
    }
  };

  const openMessageCenter = (recipientId?: string) => {
    if (!isCurrentSession()) {
      return;
    }
    navigate(recipientId ? `/notifications?message=${encodeURIComponent(recipientId)}` : '/notifications');
    setOpen(false);
  };

  const handleMarkRead = async (recipientId: string, mailboxVersion: string) => {
    if (!isCurrentSession()) {
      return;
    }
    setMarkingRecipientId(recipientId);
    try {
      const updated = await markInboxRead(recipientId, { expectedMailboxVersion: mailboxVersion });
      if (!isCurrentSession()) {
        return;
      }
      removePreviewRecipient(updated.recipientId);
      await refreshUnreadCount();
    } catch (error) {
      if (!isCurrentSession()) {
        return;
      }
      message.error(error instanceof Error ? error.message : '更新消息失败');
    } finally {
      if (isCurrentSession()) {
        setMarkingRecipientId(null);
      }
    }
  };

  if (!accountKey) {
    return null;
  }

  const content = (
    <div style={{ width: 300 }}>
      <div style={{ marginBottom: 10, fontWeight: 600, color: '#1d1d1f' }}>未读消息</div>
      {previewLoading && messagePopoverList.length === 0 ? (
        <PreviewSkeleton />
      ) : messagePopoverList.length > 0 ? (
        <div style={{ display: 'grid', gap: 4 }}>
          {messagePopoverList.map((item) => (
            <div
              key={item.recipientId}
              style={{
                display: 'grid',
                gap: 4,
                padding: '9px 0',
                borderBottom: '1px solid rgba(15, 23, 42, 0.07)',
              }}
            >
              <button
                type="button"
                onClick={() => openMessageCenter(item.recipientId)}
                style={{
                  display: 'grid',
                  gap: 4,
                  padding: 0,
                  border: 0,
                  background: 'transparent',
                  color: 'inherit',
                  cursor: 'pointer',
                  textAlign: 'left',
                }}
              >
                <Typography.Text ellipsis={{ tooltip: item.title }} style={{ fontWeight: 500 }}>
                  {item.title}
                </Typography.Text>
                {item.summary && (
                  <Typography.Text ellipsis type="secondary" style={{ fontSize: 12 }}>
                    {item.summary}
                  </Typography.Text>
                )}
              </button>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                  {formatPreviewTime(item.createTime)}
                </Typography.Text>
                <Button
                  type="link"
                  size="small"
                  loading={markingRecipientId === item.recipientId}
                  onClick={() => void handleMarkRead(item.recipientId, item.mailboxVersion)}
                  style={{ height: 22, paddingInline: 0 }}
                >
                  标为已读
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="没有未读消息" style={{ margin: '18px 0' }} />
      )}
      <Button
        type="link"
        block
        onClick={() => openMessageCenter()}
        style={{ marginTop: 4, paddingInline: 0, textAlign: 'left' }}
      >
        查看全部
      </Button>
    </div>
  );

  return (
    <Popover
      content={content}
      trigger={['hover', 'focus']}
      placement="bottomRight"
      open={open}
      onOpenChange={handleOpenChange}
      mouseEnterDelay={0.16}
    >
      <Badge count={unreadCount} overflowCount={99} size="small" offset={[-1, 1]}>
        <Button
          type="text"
          shape="circle"
          size="small"
          icon={<BellOutlined />}
          aria-label={unreadCount > 0 ? `消息，${unreadCount} 条未读` : '消息'}
          onClick={() => openMessageCenter()}
          style={{ color: '#475569' }}
        />
      </Badge>
    </Popover>
  );
}
