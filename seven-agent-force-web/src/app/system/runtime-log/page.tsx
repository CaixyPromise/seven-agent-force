'use client';

import React from 'react';
import { Empty } from 'antd';
import { usePermissionFlags } from '@/hooks/auth';
import { ADMIN_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { RuntimeLogConsole } from '@/app/system/runtime-log/components/RuntimeLogConsole';

export default function RuntimeLogPage() {
  const { canViewRuntimeLog, canStreamRuntimeLog } = usePermissionFlags({
    canViewRuntimeLog: ADMIN_PERMISSIONS.RUNTIME_LOG_VIEW,
    canStreamRuntimeLog: ADMIN_PERMISSIONS.RUNTIME_LOG_STREAM,
  });

  if (!canViewRuntimeLog) {
    return <Empty description="当前账号没有应用运行日志查看权限" image={Empty.PRESENTED_IMAGE_SIMPLE} />;
  }

  return <RuntimeLogConsole canStream={canStreamRuntimeLog} />;
}
