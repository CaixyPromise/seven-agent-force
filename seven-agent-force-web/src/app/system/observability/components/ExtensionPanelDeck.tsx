'use client';

import { Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityExtensionPanel } from '@/api/observabilityController';

const { Text } = Typography;

interface ExtensionPanelDeckProps {
  items: ObservabilityExtensionPanel[];
}

function resolveTone(status: string) {
  switch (status) {
    case 'critical':
      return {
        accent: '#dc2626',
        background: 'rgba(254, 242, 242, 0.98)',
      };
    case 'warning':
      return {
        accent: '#d97706',
        background: 'rgba(255, 251, 235, 0.98)',
      };
    default:
      return {
        accent: '#0284c7',
        background: 'rgba(239, 246, 255, 0.98)',
      };
  }
}

export function ExtensionPanelDeck({ items }: ExtensionPanelDeckProps) {
  const prefersReducedMotion = useReducedMotion();

  if (!items.length) {
    return null;
  }

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(260px, 1fr))',
        gap: 16,
      }}
    >
      {items.map((item, index) => {
        const tone = resolveTone(item.status);
        return (
          <motion.div
            key={item.panelKey}
            initial={prefersReducedMotion ? false : { opacity: 0, y: 12 }}
            animate={prefersReducedMotion ? undefined : { opacity: 1, y: 0 }}
            transition={prefersReducedMotion ? undefined : { duration: 0.3, delay: index * 0.04 }}
            style={{
              display: 'grid',
              gap: 16,
              padding: '20px 20px 18px',
              borderRadius: 24,
              border: '1px solid rgba(191, 219, 254, 0.82)',
              background: `linear-gradient(180deg, ${tone.background} 0%, rgba(255,255,255,0.98) 100%)`,
            }}
          >
            <div style={{ display: 'grid', gap: 6 }}>
              <div style={{ width: 42, height: 4, borderRadius: 999, background: tone.accent }} />
              <Text style={{ color: '#0f172a', fontSize: 15, fontWeight: 600 }}>{item.title}</Text>
              <Text style={{ color: '#64748b', fontSize: 12 }}>{item.description}</Text>
            </div>
            <div style={{ display: 'grid', gap: 12 }}>
              {item.metrics.map((metric) => (
                <div key={metric.key} style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                  <div style={{ display: 'grid', gap: 4 }}>
                    <Text style={{ color: '#334155', fontSize: 12 }}>{metric.label}</Text>
                    <Text style={{ color: '#64748b', fontSize: 11 }}>{metric.trendLabel}</Text>
                  </div>
                  <Text
                    style={{
                      color: '#0f172a',
                      fontSize: 18,
                      fontWeight: 700,
                      fontFamily: '"DIN Alternate", "IBM Plex Sans", "PingFang SC", sans-serif',
                    }}
                  >
                    {metric.value}
                  </Text>
                </div>
              ))}
            </div>
          </motion.div>
        );
      })}
    </div>
  );
}
