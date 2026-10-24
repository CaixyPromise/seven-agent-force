'use client';

import { Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityDependencyHealth } from '@/api/observabilityController';
import { formatBytes } from '@/app/system/observability/components/observabilityFormat';

const { Text } = Typography;

interface DependencyMatrixPanelProps {
  items: ObservabilityDependencyHealth[];
}

function formatDependencyDetail(detail?: string) {
  if (!detail) {
    return '未提供附加说明';
  }
  return detail
    .split(' · ')
    .map((segment) => {
      const [rawKey, rawValue] = segment.split('=');
      if (!rawKey || rawValue === undefined) {
        return segment;
      }
      const key = rawKey.trim();
      const numericValue = Number(rawValue);
      if (['total', 'free', 'threshold', 'usableSpace', 'totalSpace', 'freeSpace'].includes(key) && Number.isFinite(numericValue)) {
        const label = key === 'total' || key === 'totalSpace' ? '总量' : key === 'free' || key === 'freeSpace' ? '剩余' : key === 'threshold' ? '阈值' : '可用';
        return `${label} ${formatBytes(numericValue)}`;
      }
      if (key === 'version') {
        return `版本 ${rawValue}`;
      }
      if (key === 'database') {
        return `数据库 ${rawValue}`;
      }
      if (key === 'validationQuery') {
        return `校验 ${rawValue}`;
      }
      return segment;
    })
    .join(' · ');
}

function resolveStatusTone(status: string) {
  switch (status) {
    case 'down':
      return {
        dot: '#dc2626',
        bg: 'rgba(254, 242, 242, 0.98)',
        border: 'rgba(252, 165, 165, 0.5)',
      };
    case 'out_of_service':
      return {
        dot: '#d97706',
        bg: 'rgba(255, 251, 235, 0.98)',
        border: 'rgba(251, 191, 36, 0.46)',
      };
    case 'up':
      return {
        dot: '#0f766e',
        bg: 'rgba(240, 253, 250, 0.98)  ',
        border: 'rgba(45, 212, 191, 0.42)',
      };
    default:
      return {
        dot: '#475569',
        bg: 'rgba(248, 250, 252, 0.98)',
        border: 'rgba(148, 163, 184, 0.38)',
      };
  }
}

export function DependencyMatrixPanel({ items }: DependencyMatrixPanelProps) {
  const prefersReducedMotion = useReducedMotion();

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
        gap: 14,
      }}
    >
      {items.map((item, index) => {
        const tone = resolveStatusTone(item.status);
        return (
          <motion.div
            key={item.dependencyKey}
            initial={prefersReducedMotion ? false : { opacity: 0, y: 12 }}
            animate={prefersReducedMotion ? undefined : { opacity: 1, y: 0 }}
            transition={prefersReducedMotion ? undefined : { duration: 0.3, delay: index * 0.04 }}
            style={{
              display: 'grid',
              gap: 10,
              padding: '18px 18px 16px',
              borderRadius: 24,
              background: tone.bg,
              border: `1px solid ${tone.border}`,
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
              <Text style={{ color: '#0f172a', fontSize: 15, fontWeight: 600 }}>{item.dependencyName}</Text>
              <div style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                <span
                  style={{
                    width: 10,
                    height: 10,
                    borderRadius: '999px',
                    background: tone.dot,
                    boxShadow: `0 0 16px ${tone.dot}40`,
                  }}
                />
                <Text style={{ color: '#334155', fontSize: 12 }}>{item.statusLabel}</Text>
              </div>
            </div>
            <Text style={{ color: '#64748b', fontSize: 12, wordBreak: 'break-word' }}>
              {formatDependencyDetail(item.detail)}
            </Text>
          </motion.div>
        );
      })}
    </div>
  );
}
