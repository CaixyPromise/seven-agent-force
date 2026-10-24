import React from 'react';
import { usePermissionAccess } from '@/hooks/auth';
import type { PermissionMatchMode } from '@/lib/auth/permissions';

export interface HasPermissionProps {
  code?: string;
  codes?: string[];
  matchMode?: PermissionMatchMode;
  module?: string;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}

/**
 * 权限控制组件
 * 根据权限码控制子组件的显示/隐藏
 */
export const HasPermission: React.FC<HasPermissionProps> = ({
  code,
  codes,
  matchMode = 'any',
  children,
  fallback = null,
}) => {
  const hasPermission = usePermissionAccess(
    codes ? { permissions: codes, matchMode } : code,
  );

  if (!hasPermission) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
};

export default HasPermission;
