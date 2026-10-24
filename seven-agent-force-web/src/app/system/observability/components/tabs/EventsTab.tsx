'use client';

import { ApartmentOutlined, AreaChartOutlined, ClockCircleOutlined } from '@ant-design/icons';
import type { ObservabilityPlatform } from '@/api/observabilityController';
import { ClientActivityPanel } from '@/app/system/observability/components/ClientActivityPanel';
import { EventMixPanel } from '@/app/system/observability/components/EventMixPanel';
import { ObservabilityTrendChart } from '@/app/system/observability/components/ObservabilityTrendChart';
import { SectionShell } from '@/app/system/observability/components/ObservabilitySurface';

interface EventsTabProps {
  platform: ObservabilityPlatform;
  isWide: boolean;
}

function hasTokenOutput(points: ObservabilityPlatform['timeline']) {
  return (points || []).some((point) => (point.tokenIssuedCount || 0) > 0);
}

export function EventsTab({ platform, isWide }: EventsTabProps) {
  const timeline = platform.timeline || [];

  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <SectionShell
        title="事件分布"
        description="先看当前窗口内各类平台事件的密度和占比，再判断变化主要来自哪里。"
        icon={<AreaChartOutlined />}
      >
        <EventMixPanel items={platform.eventShares || []} />
      </SectionShell>

      <div style={{ display: 'grid', gridTemplateColumns: isWide ? 'repeat(2, minmax(0, 1fr))' : '1fr', gap: 18 }}>
        <SectionShell
          title="认证结果趋势"
          description="成功、失败和风险放在一起，先看是不是登录失败抬高了风险信号。"
          icon={<ClockCircleOutlined />}
        >
          <ObservabilityTrendChart
            points={timeline}
            series={[
              { key: 'loginSuccessCount', label: '成功事件', color: '#0284c7', fill: 'rgba(2, 132, 199, 0.12)' },
              { key: 'loginFailureCount', label: '失败事件', color: '#dc2626' },
              { key: 'riskEventCount', label: '风险事件', color: '#d97706' },
            ]}
            yValueFormatter={(value) => `${Math.round(value)}`}
            height={260}
          />
        </SectionShell>

        {hasTokenOutput(timeline) ? (
          <SectionShell
            title="协议产出趋势"
            description="把授权码与令牌链路单独看，避免产出峰值把结果类事件压扁。"
            icon={<AreaChartOutlined />}
          >
            <ObservabilityTrendChart
              points={timeline}
              series={[{ key: 'tokenIssuedCount', label: '令牌产出', color: '#0f766e', fill: 'rgba(15, 118, 110, 0.12)' }]}
              yValueFormatter={(value) => `${Math.round(value)}`}
              height={260}
            />
          </SectionShell>
        ) : null}
      </div>

      <SectionShell
        title="来源活跃度"
        description="把来源或接入端单独摊开，观察活跃度、失败量和最近活动时间。"
        icon={<ApartmentOutlined />}
      >
        <ClientActivityPanel items={platform.topClients || []} />
      </SectionShell>
    </div>
  );
}
