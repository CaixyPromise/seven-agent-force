import React from 'react';
import { Result, Button } from 'antd';
import { useAuth, usePermission } from '@/hooks/auth';

interface SimpleAuthGuardProps {
  children: React.ReactNode;
  permission?: string;
}

/**
 * 测试修复后的权限组件
 */
export const SimpleAuthGuard: React.FC<SimpleAuthGuardProps> = ({
  children,
  permission,
}) => {
  console.log('=== SimpleAuthGuard 渲染开始 ===', { permission });

  // 使用修复后的 hooks
  const { isAuthenticated, user, isAdmin } = useAuth();
  const hasPermission = usePermission(permission);

  console.log('修复后的 hooks 结果:', {
    isAuthenticated,
    hasUser: !!user,
    isAdmin,
    permission,
    hasPermission
  });

  // 权限检查逻辑
  if (!isAuthenticated) {
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

  if (permission && !hasPermission) {
    console.warn('权限检查失败，拒绝访问:', {
      permission,
      hasPermission,
      userPermissions: user?.permissions,
      userRoles: user?.userRole
    });
    return (
      <Result
        status="403"
        title="访问被拒绝"
        subTitle={`您没有权限访问此页面 (需要权限: ${permission})，请联系管理员`}
        extra={
          <Button type="primary" onClick={() => window.history.back()}>
            返回上一页
          </Button>
        }
      />
    );
  }

  console.log('SimpleAuthGuard: 允许访问');
  return <>{children}</>;
};

export default SimpleAuthGuard;
