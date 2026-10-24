import type {
  DockerComposeProjectStatus,
  DockerContainerPortView,
  DockerContainerView,
} from '@/api/dockerController';

export function shortId(id?: string) {
  return id ? id.slice(0, 12) : '-';
}

function timestampMs(value?: number | string) {
  if (!value) {
    return 0;
  }
  if (typeof value === 'number') {
    return value * 1000;
  }
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function formatAbsoluteTime(timestamp?: number | string) {
  const ms = timestampMs(timestamp);
  if (!ms) {
    return '-';
  }
  return new Date(ms).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

export function formatRelativeTime(timestamp?: number | string) {
  const ms = timestampMs(timestamp);
  if (!ms) {
    return '-';
  }
  const diffSeconds = Math.max(0, Math.floor((Date.now() - ms) / 1000));
  if (diffSeconds < 60) {
    return '刚刚';
  }
  if (diffSeconds < 3600) {
    return `${Math.floor(diffSeconds / 60)} 分钟前`;
  }
  if (diffSeconds < 86400) {
    return `${Math.floor(diffSeconds / 3600)} 小时前`;
  }
  return `${Math.floor(diffSeconds / 86400)} 天前`;
}

export function formatPort(port: DockerContainerPortView) {
  const protocol = port.type || 'tcp';
  if (port.publicPort) {
    return `${port.publicPort}:${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
  }
  return `${port.privatePort ?? '-'}${protocol ? `/${protocol}` : ''}`;
}

export function normalizeState(state?: string) {
  return (state || '').toLowerCase();
}

const containerStateLabels: Record<string, string> = {
  running: '运行中',
  restarting: '重启中',
  exited: '已停止',
  paused: '已暂停',
  created: '已创建',
  dead: '异常',
  removing: '删除中',
  active: '运行中',
  inactive: '未运行',
  stopped: '已停止',
  pending: '等待中',
  failed: '失败',
  error: '错误',
  unknown: '未知',
};

export function formatContainerStateLabel(state?: string) {
  const normalized = normalizeState(state) || 'unknown';
  return containerStateLabels[normalized] || state || '未知';
}

export function formatComposeProjectStatusLabel(status?: string) {
  const normalized = normalizeState(status);
  if (normalized === 'running') {
    return '运行中';
  }
  if (normalized === 'degraded') {
    return '部分运行';
  }
  if (normalized === 'stopped') {
    return '已停止';
  }
  return '未知';
}

const operationStatusLabels: Record<string, string> = {
  PENDING: '等待中',
  RUNNING: '执行中',
  SUCCEEDED: '成功',
  FAILED: '失败',
  CANCELLED: '已取消',
  TIMEOUT: '超时',
};

export function formatOperationStatusLabel(status?: string) {
  return operationStatusLabels[(status || '').toUpperCase()] || status || '-';
}

const operationTypeLabels: Record<string, string> = {
  COMPOSE_UP: '编排启动',
  COMPOSE_DOWN: '编排停止',
  COMPOSE_RESTART: '编排重启',
  COMPOSE_LOGS: '编排日志',
  COMPOSE_IMPORT: '编排导入',
  COMPOSE_IMPORT_DISCOVERED: '发现项目导入',
  IMAGE_PULL: '镜像拉取',
  IMAGE_PUSH: '镜像推送',
  IMAGE_EXPORT: '镜像导出',
  IMAGE_DELETE: '镜像删除',
  IMAGE_CLEANUP: '镜像清理',
  REGISTRY_SYNC: '仓库同步',
  REGISTRY_DELETE: '仓库删除',
  CONTAINER_CREATE: '容器创建',
  CONTAINER_START: '容器启动',
  CONTAINER_STOP: '容器停止',
  CONTAINER_RESTART: '容器重启',
  CONTAINER_DELETE: '容器删除',
  CONTAINER_CLEANUP: '容器清理',
  NETWORK_CREATE: '网络创建',
  NETWORK_DELETE: '网络删除',
  NETWORK_CONNECT: '网络连接',
  NETWORK_DISCONNECT: '网络断开',
  NETWORK_PRUNE: '网络清理',
  VOLUME_CREATE: '存储卷创建',
  VOLUME_DELETE: '存储卷删除',
  VOLUME_PRUNE: '存储卷清理',
  DAEMON_CONFIG_UPDATE: '守护进程配置更新',
  DAEMON_RESTART: '守护进程重启',
};

export function formatOperationTypeLabel(operationType?: string) {
  return operationTypeLabels[(operationType || '').toUpperCase()] || operationType || '-';
}

const operationStageLabels: Record<string, string> = {
  pending: '等待中',
  running: '执行中',
  started: '已开始',
  complete: '完成',
  completed: '完成',
  done: '完成',
  success: '成功',
  succeeded: '成功',
  failed: '失败',
  error: '错误',
  cancelled: '已取消',
  canceled: '已取消',
  timeout: '超时',
  timed_out: '超时',
};

export function formatOperationStageLabel(stage?: string) {
  const normalized = normalizeState(stage).replace(/\s+/g, '_');
  return operationStageLabels[normalized] || stage || '-';
}

export function getProjectStatus(containers: DockerContainerView[]): DockerComposeProjectStatus {
  if (!containers.length) {
    return 'stopped';
  }
  const runningCount = containers.filter((container) => {
    const state = normalizeState(container.state);
    return state === 'running' || state === 'restarting';
  }).length;
  if (runningCount === containers.length) {
    return 'running';
  }
  if (runningCount > 0) {
    return 'degraded';
  }
  return 'stopped';
}
