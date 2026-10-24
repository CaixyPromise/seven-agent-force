'use client';

import { Empty, Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityEventShare } from '@/api/observabilityController';
import { formatNumber, formatPercent, toFiniteNumber } from '@/app/system/observability/components/observabilityFormat';

const { Text } = Typography;

interface EventMixPanelProps {
  items: ObservabilityEventShare[];
}

const TONES = ['#0284c7', '#0f766e', '#7c3aed', '#dc2626', '#d97706', '#475569'];

export function EventMixPanel({ items }: EventMixPanelProps) {
  const prefersReducedMotion = useReducedMotion();

  if (!items.length) {
    return <Empty description="暂无事件分布数据" style={{ minHeight: 180, display: 'grid', placeItems: 'center' }} />;
  }

  const normalizedItems = items.map((item) => ({
    ...item,
    count: toFiniteNumber(item.count),
  }));
  const max = Math.max(...normalizedItems.map((item) => item.count), 1);
  const total = normalizedItems.reduce((sum, item) => sum + item.count, 0);
  const topItem = normalizedItems[0];

  return (
    <div style={{ display: 'grid', gap: 18 }}>
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
          <Text style={{ color: '#64748b', fontSize: 12 }}>总事件量</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>{formatNumber(total)}</Text>
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
          <Text style={{ color: '#64748b', fontSize: 12 }}>主导事件</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>{topItem?.eventName || '无'}</Text>
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
          <Text style={{ color: '#64748b', fontSize: 12 }}>主导占比</Text>
          <Text style={{ color: '#0f172a', fontWeight: 700 }}>
            {topItem ? formatPercent((topItem.count / Math.max(total, 1)) * 100) : '0%'}
          </Text>
        </div>
      </div>

      {normalizedItems.map((item, index) => (
        <div
          key={item.eventKey}
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
            <Text style={{ color: '#0f172a' }}>{item.eventName}</Text>
            <div style={{ display: 'inline-flex', alignItems: 'baseline', gap: 10 }}>
              <Text
                style={{
                  color: '#475569',
                  fontFamily: 'IBM Plex Mono, SFMono-Regular, ui-monospace, monospace',
                }}
              >
                {formatNumber(item.count)}
              </Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>
                {formatPercent((item.count / Math.max(total, 1)) * 100)}
              </Text>
            </div>
          </div>
          <div
            style={{
              width: '100%',
              height: 10,
              borderRadius: 999,
              background: 'rgba(148, 163, 184, 0.18)',
              overflow: 'hidden',
            }}
          >
            <motion.div
              initial={prefersReducedMotion ? false : { width: 0 }}
              animate={{ width: `${(item.count / max) * 100}%` }}
              transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.45, delay: index * 0.05 }}
              style={{
                height: '100%',
                borderRadius: 999,
                background: `linear-gradient(90deg, ${TONES[index % TONES.length]} 0%, rgba(14,165,233,0.18) 180%)`,
                boxShadow: `0 0 24px ${TONES[index % TONES.length]}25`,
              }}
            />
          </div>
          <Text style={{ color: '#64748b', fontSize: 12 }}>
            当前窗口内占比 {formatPercent((item.count / Math.max(total, 1)) * 100)}，用于判断事件变化主要来自哪一类。
          </Text>
        </div>
      ))}
    </div>
  );
}
