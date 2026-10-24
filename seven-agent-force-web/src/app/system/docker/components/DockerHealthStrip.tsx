'use client';

import { Button, Space, Tag, Tooltip } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';

type DockerDaemonStatus = 'checking' | 'healthy' | 'error';

interface DockerHealthStripProps {
  daemonStatus: DockerDaemonStatus;
  containerCount: number;
  permissions: {
    canQuery?: boolean;
    canStart?: boolean;
    canStop?: boolean;
    canRestart?: boolean;
    canDelete?: boolean;
  };
  lastError?: string;
  onRefresh?: () => void;
}

function daemonTag(status: DockerDaemonStatus) {
  if (status === 'checking') {
    return <Tag color="processing">检查中</Tag>;
  }
  if (status === 'healthy') {
    return <Tag color="success">Docker 可用</Tag>;
  }
  return <Tag color="error">Docker 不可用</Tag>;
}

function permissionSummary(permissions: DockerHealthStripProps['permissions']) {
  const enabled = [
    permissions.canQuery ? '查看' : '',
    permissions.canStart ? '启动' : '',
    permissions.canStop ? '停止' : '',
    permissions.canRestart ? '重启' : '',
    permissions.canDelete ? '删除' : '',
  ].filter(Boolean);
  return enabled.length ? enabled.join(' / ') : '暂无容器操作权限';
}

export function DockerHealthStrip({
  daemonStatus,
  containerCount,
  permissions,
  lastError,
  onRefresh,
}: DockerHealthStripProps) {
  return (
    <section className="rounded-2xl border border-[#dce7f5] bg-white px-4 py-3 shadow-[0_8px_24px_rgba(15,23,42,0.04)]">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <Space size={8} wrap>
          {daemonTag(daemonStatus)}
          <Tag color="blue">容器 {containerCount}</Tag>
        </Space>
        <Space size={8} wrap>
          {lastError ? (
            <Tooltip title={lastError}>
              <span className="max-w-[520px] truncate text-sm text-rose-600">最近错误：{lastError}</span>
            </Tooltip>
          ) : (
            <Tooltip title={permissionSummary(permissions)}>
              <span className="text-sm text-slate-500">容器权限：{permissionSummary(permissions)}</span>
            </Tooltip>
          )}
          {onRefresh ? (
            <Button size="small" icon={<ReloadOutlined />} onClick={onRefresh}>
              刷新
            </Button>
          ) : null}
        </Space>
      </div>
    </section>
  );
}
