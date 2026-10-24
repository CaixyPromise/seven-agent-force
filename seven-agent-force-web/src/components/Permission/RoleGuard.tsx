import React from 'react';
import { useRole } from '@/hooks/auth';

export interface RoleGuardProps {
  roles: string | string[];
  requireAll?: boolean;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}

/**
 * 角色守卫组件
 * 根据角色控制子组件的显示/隐藏
 */
export const RoleGuard: React.FC<RoleGuardProps> = ({
  roles,
  requireAll = false,
  children,
  fallback = null,
}) => {
  const hasRole = useRole(roles, requireAll);

  if (!hasRole) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
};

export default RoleGuard;
