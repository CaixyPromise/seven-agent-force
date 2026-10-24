'use client';

import { ApartmentOutlined } from '@ant-design/icons';
import type { ObservabilityPlatform } from '@/api/observabilityController';
import { ExtensionPanelDeck } from '@/app/system/observability/components/ExtensionPanelDeck';
import { SectionShell } from '@/app/system/observability/components/ObservabilitySurface';

interface ExtensionsTabProps {
  platform: ObservabilityPlatform;
}

export function ExtensionsTab({ platform }: ExtensionsTabProps) {
  return (
    <div style={{ display: 'grid', gap: 18, paddingTop: 8 }}>
      <SectionShell
        title="扩展面板槽位"
        description="给未来平台或专项域预留挂载位，当前先展示平台侧已经聚合的扩展信号。"
        icon={<ApartmentOutlined />}
      >
        <ExtensionPanelDeck items={platform.extensionPanels || []} />
      </SectionShell>
    </div>
  );
}
