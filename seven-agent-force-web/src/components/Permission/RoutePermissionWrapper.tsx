import React from 'react';
import { AuthGuard } from '@/components/Permission';

interface RoutePermissionWrapperProps {
  children: React.ReactNode;
  // 路由权限配置
  permission?: string;
  permissionModule?: string;
  roles?: string | string[];
  requireAllRoles?: boolean;
  requireAuth?: boolean;
}

/**
 * 路由权限包装器
 * 用于包装页面组件，进行路由级别的权限检查
 */
export const RoutePermissionWrapper: React.FC<RoutePermissionWrapperProps> = ({
  children,
  permission,
  permissionModule,
  roles,
  requireAllRoles = false,
  requireAuth = true,
}) => {
  return (
    <AuthGuard
      permission={permission}
      permissionModule={permissionModule}
      roles={roles}
      requireAllRoles={requireAllRoles}
      requireAuth={requireAuth}
      showDefaultFallback={true}
    >
      {children}
    </AuthGuard>
  );
};

export default RoutePermissionWrapper;
