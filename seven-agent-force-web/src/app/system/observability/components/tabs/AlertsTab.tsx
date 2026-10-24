'use client';

import { SafetyCertificateOutlined } from '@ant-design/icons';
import type { ObservabilityPlatform } from '@/api/observabilityController';
import { AlertFeed } from '@/app/system/observability/components/AlertFeed';
import { InsightCard, SectionShell } from '@/app/system/observability/components/ObservabilitySurface';
import { Tag } from 'antd';

interface AlertsTabProps {
  platform: ObservabilityPlatform;
  isDesktop: boolean;
  isWide: boolean;
}

export function AlertsTab({ platform, isDesktop, isWide }: AlertsTabProps) {
  const alerts = platform.alerts || [];
  const critical = alerts.filter((item) => item.severity === 'critical').length;
  const watch = alerts.filter((item) => item.severity !== 'critical').length;
  const topReasonMap = new Map<string, number>();
  alerts.forEach((item) => {
    const key = item.reasonCode || item.eventType || 'unknown';
    topReasonMap.set(key, (topReasonMap.get(key) || 0) + 1);
  });
  const topReasons = [...topReasonMap.entries()]
    .sort((left, right) => right[1] - left[1])
    .slice(0, 3)
    .map(([key, count]) => ({ key, count }));

  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: isWide ? 'repeat(4, minmax(0, 1fr))' : isDesktop ? 'repeat(2, minmax(0, 1fr))' : '1fr',
          gap: 14,
        }}
      >
        <InsightCard label="异常总量" value={String(alerts.length)} detail="当前窗口所有告警与异常事件" accent="#111827" />
        <InsightCard label="高优先级" value={String(critical)} detail="需要立即处理" accent="#dc2626" />
        <InsightCard label="关注项" value={String(watch)} detail="建议继续跟踪" accent="#d97706" />
        <InsightCard
          label="主导原因"
          value={topReasons[0]?.key || '无'}
          detail={topReasons[0] ? `出现 ${topReasons[0].count} 次` : '当前窗口暂无异常原因'}
          accent="#0284c7"
        />
      </div>

      <SectionShell
        title="异常与告警流"
        description="把异常事件按严重程度和时间排开，先处理高优先级，再看是否持续重复。"
        icon={<SafetyCertificateOutlined />}
      >
        <div style={{ display: 'grid', gap: 12 }}>
          {topReasons.length ? (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>
              {topReasons.map((item) => (
                <Tag
                  key={item.key}
                  style={{
                    marginInlineEnd: 0,
                    borderRadius: 999,
                    paddingInline: 12,
                    paddingBlock: 5,
                    background: 'rgba(239, 246, 255, 0.98)',
                    border: '1px solid rgba(191, 219, 254, 0.8)',
                    color: '#0f172a',
                  }}
                >
                  {item.key} · {item.count}
                </Tag>
              ))}
            </div>
          ) : null}
          <AlertFeed items={alerts} />
        </div>
      </SectionShell>
    </div>
  );
}
