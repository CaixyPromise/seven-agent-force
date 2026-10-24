'use client';

import { Empty, Tag, Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityAlert } from '@/api/observabilityController';

const { Paragraph, Text } = Typography;

interface AlertFeedProps {
  items: ObservabilityAlert[];
}

function formatDateTime(value?: string) {
  if (!value) {
    return '刚刚';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

function resolveTagColor(severity: string) {
  if (severity === 'critical') {
    return '#dc2626';
  }
  return '#d97706';
}

export function AlertFeed({ items }: AlertFeedProps) {
  const prefersReducedMotion = useReducedMotion();

  if (!items.length) {
    return <Empty description="当前窗口暂无告警" />;
  }

  return (
    <div style={{ display: 'grid', gap: 14 }}>
      {items.map((item, index) => (
        <motion.div
          key={
            item.id != null
              ? `alert-${item.id}`
              : `${item.eventType}-${item.createdAt ?? 'na'}-${item.clientId ?? 'na'}-${item.reasonCode ?? 'na'}-${index}`
          }
          initial={prefersReducedMotion ? false : { opacity: 0, y: 12 }}
          animate={prefersReducedMotion ? undefined : { opacity: 1, y: 0 }}
          transition={prefersReducedMotion ? undefined : { duration: 0.28, delay: index * 0.05 }}
          style={{
            padding: '16px 18px',
            borderRadius: 22,
            border: `1px solid ${resolveTagColor(item.severity)}28`,
            background:
              item.severity === 'critical'
                ? 'linear-gradient(180deg, rgba(254, 242, 242, 0.98) 0%, rgba(255, 255, 255, 0.98) 100%)'
                : 'linear-gradient(180deg, rgba(255, 251, 235, 0.98) 0%, rgba(255, 255, 255, 0.98) 100%)',
            boxShadow:
              item.severity === 'critical'
                ? '0 18px 36px rgba(255, 72, 72, 0.08)'
                : '0 18px 36px rgba(255, 196, 87, 0.06)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
            <Tag
              variant="filled"
              style={{
                marginInlineEnd: 0,
                borderRadius: 999,
                background: `${resolveTagColor(item.severity)}20`,
                color: resolveTagColor(item.severity),
                fontWeight: 600,
              }}
            >
              {item.severity === 'critical' ? '高优先级' : '关注'}
            </Tag>
            <Text style={{ color: '#64748b', fontSize: 12 }}>
              {formatDateTime(item.createdAt)}
            </Text>
          </div>

          <div style={{ display: 'grid', gap: 6, marginTop: 12 }}>
            <Text style={{ color: '#0f172a', fontSize: 16, fontWeight: 600 }}>
              {item.title}
            </Text>
            <Paragraph style={{ marginBottom: 0, color: '#475569' }}>
              {item.summary}
            </Paragraph>
          </div>

          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10, marginTop: 12 }}>
            {item.clientName ? (
              <Text style={{ color: '#334155', fontSize: 12 }}>
                客户端 {item.clientName}
              </Text>
            ) : null}
            {item.reasonCode ? (
              <Text
                style={{
                  color: '#64748b',
                  fontSize: 12,
                  fontFamily: 'IBM Plex Mono, SFMono-Regular, ui-monospace, monospace',
                }}
              >
                {item.reasonCode}
              </Text>
            ) : null}
          </div>
        </motion.div>
      ))}
    </div>
  );
}
