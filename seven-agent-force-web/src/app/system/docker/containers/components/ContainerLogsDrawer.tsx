'use client';

import { Button, Drawer, Empty, Skeleton, Space, message } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { DockerCodeBlock } from '../../components/dockerConsole';

interface ContainerLogsDrawerProps {
  open: boolean;
  loading: boolean;
  containerName?: string;
  logs: string;
  onClose: () => void;
}

export function ContainerLogsDrawer({
  open,
  loading,
  containerName,
  logs,
  onClose,
}: ContainerLogsDrawerProps) {
  const lineCount = logs ? logs.split('\n').length : 0;

  return (
    <Drawer
      open={open}
      size="large"
      title={containerName ? `容器日志 · ${containerName}` : '容器日志'}
      onClose={onClose}
      destroyOnHidden
      extra={
        logs ? (
          <Space>
            <Button
              icon={<CopyOutlined />}
              onClick={async () => {
                try {
                  await navigator.clipboard.writeText(logs);
                  message.success('日志已复制到剪贴板');
                } catch {
                  message.error('复制日志失败');
                }
              }}
            >
              复制日志
            </Button>
          </Space>
        ) : undefined
      }
    >
      {loading ? (
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : logs ? (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-3 text-sm text-slate-500">
            <span>{containerName || '当前容器'}</span>
            <span>共 {lineCount} 行</span>
            <span>{logs.length} 字符</span>
          </div>
          <DockerCodeBlock
            title="日志输出"
            description="内容来自 Docker daemon 当前日志读取结果。"
            value={logs}
          />
        </div>
      ) : (
        <Empty description="暂无日志内容" />
      )}
    </Drawer>
  );
}
