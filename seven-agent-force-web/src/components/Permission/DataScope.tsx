import React from 'react';
import { useDataScope } from '@/hooks/auth';
import type { UserDataScope } from '@/lib/http/types';

export interface DataScopeProps {
	deptId?: API.Int64;
	userId?: API.Int64;
  children: React.ReactNode;
  fallback?: React.ReactNode;
  // 自定义数据权限检查
  customCheck?: (dataScope: UserDataScope) => boolean;
}

/**
 * 数据权限组件
 * 根据数据权限范围控制数据访问
 */
export const DataScope: React.FC<DataScopeProps> = ({
  deptId,
  userId,
  children,
  fallback = null,
  customCheck,
}) => {
  const { dataScope, canAccessDeptData, isPersonalOnly } = useDataScope();

  // 自定义检查优先
  if (customCheck) {
    const hasAccess = customCheck(dataScope);
    return hasAccess ? <>{children}</> : <>{fallback}</>;
  }

  // 部门数据权限检查
  if (deptId !== undefined) {
    const hasAccess = canAccessDeptData(deptId);
    return hasAccess ? <>{children}</> : <>{fallback}</>;
  }

  // 个人数据权限检查
  if (userId !== undefined && isPersonalOnly) {
		const hasAccess = dataScope.userId !== undefined && dataScope.userId === userId;
    return hasAccess ? <>{children}</> : <>{fallback}</>;
  }

  // 默认显示
  return <>{children}</>;
};

export default DataScope;
