'use client';

import type { ReactNode } from 'react';
import { Typography } from 'antd';
import { motion, useReducedMotion } from 'framer-motion';
import type { ObservabilityMetric } from '@/api/observabilityController';
import { formatNumber } from '@/app/system/observability/components/observabilityFormat';

const { Paragraph, Text, Title } = Typography;

const panelMotion = {
  initial: { opacity: 0, y: 16 },
  animate: { opacity: 1, y: 0 },
  transition: { duration: 0.32, ease: 'easeOut' },
} as const;

function getMetricAccent(index: number) {
  return ['#0284c7', '#0f766e', '#7c3aed', '#111827', '#d97706', '#475569'][index % 6];
}

function formatDisplayValue(value: string) {
  if (/^-?\d+(\.\d+)?$/.test(value)) {
    const numeric = Number(value);
    return formatNumber(numeric, Number.isInteger(numeric) ? 0 : 2);
  }
  return value;
}

export function StatusCapsule({
  label,
  tone,
}: {
  label: string;
  tone: {
    dot: string;
    glow: string;
  };
}) {
  return (
    <span
      style={{
        width: 'fit-content',
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: 32,
        padding: '0 12px',
        borderRadius: 999,
        border: `1px solid ${tone.dot}35`,
        background: tone.glow,
        color: '#0f172a',
        fontSize: 12,
        fontWeight: 700,
        lineHeight: 1,
        whiteSpace: 'nowrap',
      }}
    >
      {label}
    </span>
  );
}

export function KpiCard({ metric, index }: { metric: ObservabilityMetric; index: number }) {
  const accent = getMetricAccent(index);
  return (
    <div
      style={{
        display: 'grid',
        gap: 12,
        padding: '18px 20px',
        borderRadius: 26,
        border: '1px solid rgba(203, 213, 225, 0.78)',
        background: 'linear-gradient(180deg, rgba(255,255,255,0.98) 0%, rgba(248,250,252,0.98) 100%)',
        boxShadow: '0 18px 42px -34px rgba(14, 116, 144, 0.26)',
      }}
    >
      <div
        style={{
          width: 40,
          height: 4,
          borderRadius: 999,
          background: accent,
          boxShadow: `0 0 18px ${accent}40`,
        }}
      />
      <Text style={{ color: '#64748b', fontSize: 12 }}>{metric.label}</Text>
      <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, flexWrap: 'wrap' }}>
        <span
          style={{
            color: '#0f172a',
            fontSize: 28,
            lineHeight: 1,
            fontWeight: 700,
            letterSpacing: '-0.04em',
            fontFamily: '"DIN Alternate", "IBM Plex Sans", "PingFang SC", sans-serif',
            wordBreak: 'break-word',
          }}
        >
          {formatDisplayValue(metric.value)}
        </span>
        {metric.unit ? <Text style={{ color: '#475569', fontSize: 12 }}>{metric.unit}</Text> : null}
      </div>
      <Text style={{ color: '#475569', fontSize: 12 }}>{metric.trendLabel}</Text>
    </div>
  );
}

export function InsightCard({
  label,
  value,
  detail,
  accent,
}: {
  label: string;
  value: string;
  detail: string;
  accent: string;
}) {
  return (
    <div
      style={{
        display: 'grid',
        gap: 8,
        padding: '18px 18px 16px',
        borderRadius: 24,
        border: '1px solid rgba(191, 219, 254, 0.76)',
        background: 'linear-gradient(180deg, rgba(255,255,255,0.96) 0%, rgba(248,250,252,0.98) 100%)',
      }}
    >
      <div
        style={{
          width: 34,
          height: 4,
          borderRadius: 999,
          background: accent,
          boxShadow: `0 0 18px ${accent}40`,
        }}
      />
      <Text style={{ color: '#64748b', fontSize: 12 }}>{label}</Text>
        <Text
          style={{
            color: '#0f172a',
            fontSize: 22,
            lineHeight: 1.1,
            fontWeight: 700,
            letterSpacing: '-0.03em',
            fontFamily: '"DIN Alternate", "IBM Plex Sans", "PingFang SC", sans-serif',
            wordBreak: 'break-word',
          }}
        >
          {formatDisplayValue(value)}
        </Text>
      <Text style={{ color: '#475569', fontSize: 12 }}>{detail}</Text>
    </div>
  );
}

export function SectionShell({
  title,
  description,
  icon,
  children,
}: {
  title: string;
  description: string;
  icon: ReactNode;
  children: ReactNode;
}) {
  const prefersReducedMotion = useReducedMotion();

  return (
    <motion.section
      {...(prefersReducedMotion ? {} : panelMotion)}
      style={{
        display: 'grid',
        gap: 18,
        padding: '22px 22px 24px',
        borderRadius: 30,
        border: '1px solid rgba(191, 219, 254, 0.82)',
        background: 'linear-gradient(180deg, rgba(255,255,255,0.97) 0%, rgba(248,250,252,0.98) 100%)',
        boxShadow: '0 22px 52px -42px rgba(14, 116, 144, 0.22)',
      }}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', gap: 16, alignItems: 'flex-start' }}>
        <div style={{ display: 'grid', gap: 6 }}>
          <div style={{ display: 'inline-flex', alignItems: 'center', gap: 10 }}>
            <span
              style={{
                width: 34,
                height: 34,
                borderRadius: 16,
                display: 'grid',
                placeItems: 'center',
                background: 'linear-gradient(180deg, rgba(224, 242, 254, 0.96) 0%, rgba(239, 246, 255, 0.98) 100%)',
                color: '#0f172a',
              }}
            >
              {icon}
            </span>
            <Title level={4} style={{ margin: 0, color: '#0f172a' }}>
              {title}
            </Title>
          </div>
          <Paragraph style={{ marginBottom: 0, color: '#64748b' }}>{description}</Paragraph>
        </div>
      </div>
      {children}
    </motion.section>
  );
}
