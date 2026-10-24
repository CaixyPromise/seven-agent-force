'use client';

import { ApartmentOutlined, DatabaseOutlined } from '@ant-design/icons';
import { Typography } from 'antd';
import type { ObservabilityMiddlewarePanel, ObservabilityPlatform } from '@/api/observabilityController';
import { DependencyMatrixPanel } from '@/app/system/observability/components/DependencyMatrixPanel';
import { SectionShell } from '@/app/system/observability/components/ObservabilitySurface';
import { formatBytes, formatNumber, toFiniteNumber } from '@/app/system/observability/components/observabilityFormat';

const { Text } = Typography;

interface MiddlewareTabProps {
  platform: ObservabilityPlatform;
  isDesktop: boolean;
  isWide: boolean;
}

function renderMetricValue(_panel: ObservabilityMiddlewarePanel, value: string, unit?: string) {
  if (unit === 'bytes') {
    return formatBytes(Number(value));
  }
  if (/^-?\d+(\.\d+)?$/.test(value)) {
    const numeric = toFiniteNumber(value);
    return unit ? `${formatNumber(numeric, Number.isInteger(numeric) ? 0 : 2)} ${unit}` : formatNumber(numeric, Number.isInteger(numeric) ? 0 : 2);
  }
  if (unit) {
    return `${value} ${unit}`;
  }
  return value;
}

function resolvePanelTone(status: string) {
  switch (status) {
    case 'warning':
      return {
        border: 'rgba(251, 191, 36, 0.34)',
        background: 'rgba(255, 251, 235, 0.96)',
      };
    case 'critical':
      return {
        border: 'rgba(248, 113, 113, 0.34)',
        background: 'rgba(254, 242, 242, 0.96)',
      };
    default:
      return {
        border: 'rgba(125, 211, 252, 0.34)',
        background: 'rgba(240, 249, 255, 0.96)',
      };
  }
}

export function MiddlewareTab({ platform, isDesktop, isWide }: MiddlewareTabProps) {
  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <SectionShell
        title="中间件健康矩阵"
        description="数据库、缓存、消息和存储先看状态，再往下看更细的统计。"
        icon={<DatabaseOutlined />}
      >
        <DependencyMatrixPanel items={platform.dependencyMatrix || []} />
      </SectionShell>

      <SectionShell
        title="中间件细项"
        description="把当前实例能拿到的 MySQL、Redis、RabbitMQ 和存储指标拆成单独面板。"
        icon={<ApartmentOutlined />}
      >
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr',
            gap: 14,
          }}
        >
          {(platform.middlewarePanels || []).map((panel) => {
            const tone = resolvePanelTone(panel.status);
            return (
              <div
                key={panel.panelKey}
                style={{
                  display: 'grid',
                  gap: 14,
                  padding: '18px 18px 16px',
                  borderRadius: 26,
                  border: `1px solid ${tone.border}`,
                  background: tone.background,
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 14, flexWrap: 'wrap' }}>
                  <div style={{ display: 'grid', gap: 6 }}>
                    <Text style={{ color: '#0f172a', fontSize: 18, fontWeight: 700 }}>{panel.title}</Text>
                    <Text style={{ color: '#475569' }}>{panel.description}</Text>
                  </div>
                  <span
                    style={{
                      minHeight: 28,
                      padding: '0 10px',
                      borderRadius: 999,
                      display: 'inline-flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      background: 'rgba(255,255,255,0.7)',
                      color: '#0f172a',
                      fontWeight: 700,
                      fontSize: 12,
                      lineHeight: 1,
                    }}
                  >
                    {panel.statusLabel}
                  </span>
                </div>

                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
                    gap: 12,
                  }}
                >
                  {panel.metrics.map((metric) => (
                    <div
                      key={`${panel.panelKey}-${metric.key}`}
                      style={{
                        display: 'grid',
                        gap: 4,
                        padding: '12px 14px',
                        borderRadius: 18,
                        background: 'rgba(255,255,255,0.72)',
                      }}
                    >
                      <Text style={{ color: '#64748b', fontSize: 12 }}>{metric.label}</Text>
                      <Text style={{ color: '#0f172a', fontWeight: 700 }}>
                        {renderMetricValue(panel, metric.value, metric.unit)}
                      </Text>
                      <Text style={{ color: '#475569', fontSize: 12 }}>{metric.trendLabel}</Text>
                    </div>
                  ))}
                </div>

                <div style={{ display: 'grid', gap: 8 }}>
                  {panel.detailLines.map((line, index) => (
                    <div
                      key={`${panel.panelKey}-detail-${index}`}
                      style={{
                        padding: '10px 12px',
                        borderRadius: 16,
                        background: 'rgba(255,255,255,0.68)',
                        color: '#475569',
                      }}
                    >
                      {line}
                    </div>
                  ))}
                </div>
              </div>
            );
          })}
        </div>
      </SectionShell>
    </div>
  );
}
