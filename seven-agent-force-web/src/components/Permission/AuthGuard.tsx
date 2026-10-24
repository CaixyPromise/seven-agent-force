import React from 'react';
import { Result, Button } from 'antd';
import { useAuth, usePermission, useRole } from '@/hooks/auth';

export interface AuthGuardProps {
  children: React.ReactNode;
  // 权限要求
  permission?: string;
  permissionModule?: string;
  // 角色要求
  roles?: string | string[];
  requireAllRoles?: boolean;
  // 登录要求
  requireAuth?: boolean;
  // 自定义权限检查函数
  customCheck?: () => boolean;
  // 无权限时的展示
  fallback?: React.ReactNode;
  // 显示默认的无权限页面
  showDefaultFallback?: boolean;
}

/**
 * 综合权限守卫组件
 * 支持登录检查、权限检查、角色检查和自定义检查
 */
export const AuthGuard: React.FC<AuthGuardProps> = ({
  children,
  permission,
  permissionModule,
  roles,
  requireAllRoles = false,
  requireAuth = true,
  customCheck,
  fallback,
  showDefaultFallback = true,
}) => {
  const { isAuthenticated } = useAuth();
  const hasPermission = usePermission(permission, permissionModule);
  const hasRole = useRole(roles, requireAllRoles);

  const customCheckResult = customCheck ? customCheck() : true;

  // 检查登录状态
  if (requireAuth && !isAuthenticated) {
    if (fallback) return <>{fallback}</>;
    if (!showDefaultFallback) return null;

    return (
      <Result
        status="403"
        title="请先登录"
        subTitle="您需要登录后才能访问此页面"
        extra={
          <Button type="primary" onClick={() => window.location.href = '/login'}>
            去登录
          </Button>
        }
      />
    );
  }

  // 检查权限
  if (!hasPermission || !hasRole || !customCheckResult) {
    if (fallback) return <>{fallback}</>;
    if (!showDefaultFallback) return null;

    return (
      <Result
        status="403"
        title="访问被拒绝"
        subTitle="您没有权限访问此页面，请联系管理员"
        extra={
          <Button type="primary" onClick={() => window.history.back()}>
            返回上一页
          </Button>
        }
      />
    );
  }

  return <>{children}</>;
};

export default AuthGuard;
