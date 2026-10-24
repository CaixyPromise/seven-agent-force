// 权限组件统一导出
export { default as HasPermission } from './HasPermission';
export { default as RoleGuard } from './RoleGuard';
export { default as AuthGuard } from './AuthGuard';
export { default as DataScope } from './DataScope';
export { default as RoutePermissionWrapper } from './RoutePermissionWrapper';

// 类型导出
export type { HasPermissionProps } from './HasPermission';
export type { RoleGuardProps } from './RoleGuard';
export type { AuthGuardProps } from './AuthGuard';
export type { DataScopeProps } from './DataScope';
