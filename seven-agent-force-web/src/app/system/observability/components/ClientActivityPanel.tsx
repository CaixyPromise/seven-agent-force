'use client';

import { Empty, Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityClientActivity } from '@/api/observabilityController';
import { formatNumber, toFiniteNumber } from '@/app/system/observability/components/observabilityFormat';

const { Text } = Typography;

interface ClientActivityPanelProps {
  items: ObservabilityClientActivity[];
}

function formatDateTime(value?: string) {
  if (!value) {
    return '暂无活动';
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

export function ClientActivityPanel({ items }: ClientActivityPanelProps) {
  const prefersReducedMotion = useReducedMotion();

  if (!items.length) {
    return <Empty description="暂无活跃来源数据" style={{ minHeight: 180, display: 'grid', placeItems: 'center' }} />;
  }

  const normalizedItems = items.map((item) => ({
    ...item,
    eventCount: toFiniteNumber(item.eventCount),
    failureCount: toFiniteNumber(item.failureCount),
  }));
  const max = Math.max(...normalizedItems.map((item) => item.eventCount), 1);
  const totalFailures = normalizedItems.reduce((sum, item) => sum + item.failureCount, 0);
  const latestActivityAt = items
    .map((item) => item.lastActivityAt)
    .filter(Boolean)
    .sort()
    .at(-1);

  return (
    <div style={{ display: 'grid', gap: 16 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
          gap: 12,
        }}
      >
        <div
          style={{
            display: 'grid',
            gap: 4,
            padding: '12px 14px',
            borderRadius: 18,
            background: 'rgba(240, 249, 255, 0.92)',
          }}
        >
          <Text style={{ color: '#64748b', fontSize: 12 }}>活跃来源</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatNumber(items.length)}</Text>
        </div>
        <div
          style={{
            display: 'grid',
            gap: 4,
            padding: '12px 14px',
            borderRadius: 18,
            background: 'rgba(248, 250, 252, 0.92)',
          }}
        >
          <Text style={{ color: '#64748b', fontSize: 12 }}>失败总数</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatNumber(totalFailures)}</Text>
        </div>
        <div
          style={{
            display: 'grid',
            gap: 4,
            padding: '12px 14px',
            borderRadius: 18,
            background: 'rgba(248, 250, 252, 0.92)',
          }}
        >
          <Text style={{ color: '#64748b', fontSize: 12 }}>最近活动</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatDateTime(latestActivityAt)}</Text>
        </div>
      </div>

      {normalizedItems.map((item, index) => (
        <motion.div
          key={item.clientId || item.clientName || `client-${index}`}
          initial={prefersReducedMotion ? false : { opacity: 0, x: 12 }}
          animate={prefersReducedMotion ? undefined : { opacity: 1, x: 0 }}
          transition={prefersReducedMotion ? undefined : { duration: 0.3, delay: index * 0.04 }}
          style={{
            display: 'grid',
            gap: 10,
            padding: '14px 16px',
            borderRadius: 18,
            background: 'rgba(255,255,255,0.72)',
            border: '1px solid rgba(191, 219, 254, 0.58)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'baseline' }}>
            <div style={{ display: 'grid', gap: 4 }}>
              <Text style={{ color: '#0f172a' }}>{item.clientName || item.clientId}</Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>{item.clientId}</Text>
            </div>
            <Text
              style={{
                color: '#334155',
                fontFamily: 'IBM Plex Mono, SFMono-Regular, ui-monospace, monospace',
              }}
            >
              {formatNumber(item.eventCount)}
            </Text>
          </div>

          <div
            style={{
              width: '100%',
              height: 8,
              borderRadius: 999,
              background: 'rgba(148, 163, 184, 0.18)',
              overflow: 'hidden',
            }}
          >
            <motion.div
              initial={prefersReducedMotion ? false : { width: 0 }}
              animate={{ width: `${(item.eventCount / max) * 100}%` }}
              transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.42, delay: index * 0.05 }}
              style={{
                height: '100%',
                borderRadius: 999,
                background: 'linear-gradient(90deg, rgba(2, 132, 199, 0.92) 0%, rgba(15, 23, 42, 0.92) 100%)',
              }}
            />
          </div>

          <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16 }}>
            <Text style={{ color: '#64748b', fontSize: 12 }}>
              最近活动 {formatDateTime(item.lastActivityAt)}
            </Text>
            <Text style={{ color: item.failureCount > 0 ? '#dc2626' : '#64748b', fontSize: 12 }}>
              失败 {formatNumber(item.failureCount)}
            </Text>
          </div>
        </motion.div>
      ))}
    </div>
  );
}
