import type { ReactNode } from 'react';
import { Badge, Tag } from 'antd';
import type {
  HubConnectionStatus,
  HubNodeStatus,
  HubSessionStatus,
  HubUserStatus,
} from './controllerContract';

export const KNOWN_NODE_CAPABILITIES = [
  { label: '用户管理', value: 'users' },
  { label: '会话管理', value: 'sessions' },
  { label: '登录策略', value: 'login-policy' },
  { label: 'Hub 联邦连接', value: 'hub-connection' },
];

export const USER_STATUS_OPTIONS = [
  { label: '正常', value: 0 },
  { label: '已禁用', value: 1 },
  { label: '待审核', value: 2 },
];

export function nodeStatusTag(status: HubNodeStatus): ReactNode {
  if (status === 'UNKNOWN') {
    return <Badge status="warning" text="未知" />;
  }
  return <Badge status={status === 0 ? 'success' : 'default'} text={status === 0 ? '启用' : '停用'} />;
}

export function userStatusTag(status: HubUserStatus): ReactNode {
  if (status === 'UNKNOWN') {
    return <Tag color="warning">未知</Tag>;
  }
  const meta = {
    0: { color: 'green', label: '正常' },
    1: { color: 'default', label: '已禁用' },
    2: { color: 'gold', label: '待审核' },
  }[status];
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export function connectionStatusTag(status: HubConnectionStatus): ReactNode {
  if (status === 'UNKNOWN') {
    return <Tag color="warning">未知</Tag>;
  }
  const meta = {
    PENDING: { color: 'processing', label: '待连接' },
    ACTIVE: { color: 'success', label: '已连接' },
    ERROR: { color: 'error', label: '异常' },
  }[status];
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export function sessionStatusTag(status: HubSessionStatus): ReactNode {
  if (status === 'UNKNOWN') {
    return <Tag color="warning">未知</Tag>;
  }
  const meta = {
    ACTIVE: { color: 'green', label: '活跃' },
    EXPIRED: { color: 'default', label: '已过期' },
    REVOKED: { color: 'red', label: '已撤销' },
  }[status];
  return <Tag color={meta.color}>{meta.label}</Tag>;
}

export function formatTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
