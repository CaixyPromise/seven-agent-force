'use client';

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { Alert, Button, Card, Input, InputNumber, Select, Space, Switch, Tag, Typography } from 'antd';
import { PauseCircleOutlined, PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import {
  openRuntimeLogStream,
  type RuntimeLogStreamHandle,
} from '@/api/runtimeLogController';
import type { RuntimeLogLine, RuntimeLogStreamRequest } from '@/lib/http/types';

interface RuntimeLogStreamCardProps {
  canStream: boolean;
}

interface StreamState {
  active: boolean;
  connecting: boolean;
  lastError?: string;
  lastHeartbeatAt?: number;
}

const MAX_BUFFERED_LOGS = 1000;

function getLevelColor(level?: string) {
  const normalized = (level || '').toUpperCase();
  if (normalized === 'ERROR') {
    return 'red';
  }
  if (normalized === 'WARN') {
    return 'orange';
  }
  if (normalized === 'DEBUG') {
    return 'blue';
  }
  if (normalized === 'TRACE') {
    return 'purple';
  }
  return 'green';
}

export function RuntimeLogStreamCard({ canStream }: RuntimeLogStreamCardProps) {
  const [streamState, setStreamState] = useState<StreamState>({ active: false, connecting: false });
  const [logs, setLogs] = useState<RuntimeLogLine[]>([]);
  const [autoScroll, setAutoScroll] = useState(true);
  const [request, setRequest] = useState<RuntimeLogStreamRequest>({
    lastN: 100,
  });
  const streamHandleRef = useRef<RuntimeLogStreamHandle | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const manualStopRef = useRef(false);
  const logContainerRef = useRef<HTMLDivElement | null>(null);

  const canStartStream = canStream && !streamState.active && !streamState.connecting;

  useEffect(() => () => {
    manualStopRef.current = true;
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
    streamHandleRef.current?.close();
    streamHandleRef.current = null;
  }, []);

  useEffect(() => {
    if (!autoScroll || !logContainerRef.current) {
      return;
    }
    logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
  }, [autoScroll, logs]);

  const streamStatusText = useMemo(() => {
    if (streamState.connecting) {
      return '连接中';
    }
    if (streamState.active) {
      return '实时中';
    }
    return '已停止';
  }, [streamState.active, streamState.connecting]);

  const startStream = () => {
    if (!canStream) {
      return;
    }
    clearReconnectTimer();
    manualStopRef.current = false;
    setStreamState((prev) => ({
      ...prev,
      active: false,
      connecting: true,
      lastError: undefined,
    }));

    const handle = openRuntimeLogStream(request, {
      onConnected: () => {
        setStreamState((prev) => ({
          ...prev,
          active: true,
          connecting: false,
          lastError: undefined,
        }));
      },
      onLog: (logLine) => {
        setLogs((prevLogs) => {
          const nextLogs = [...prevLogs, logLine];
          if (nextLogs.length > MAX_BUFFERED_LOGS) {
            return nextLogs.slice(nextLogs.length - MAX_BUFFERED_LOGS);
          }
          return nextLogs;
        });
        setStreamState((prev) => ({
          ...prev,
          active: true,
          connecting: false,
        }));
      },
      onHeartbeat: () => {
        setStreamState((prev) => ({
          ...prev,
          lastHeartbeatAt: Date.now(),
        }));
      },
      onError: (error) => {
        setStreamState((prev) => ({
          ...prev,
          active: false,
          connecting: false,
          lastError: error.message,
        }));
        scheduleReconnect();
      },
    });

    streamHandleRef.current = handle;
    void handle.done.then(() => {
      if (!manualStopRef.current) {
        setStreamState((prev) => ({
          ...prev,
          active: false,
          connecting: false,
        }));
        scheduleReconnect();
      }
    });
  };

  const stopStream = (silent = false) => {
    manualStopRef.current = true;
    clearReconnectTimer();
    streamHandleRef.current?.close();
    streamHandleRef.current = null;
    setStreamState((prev) => ({
      ...prev,
      active: false,
      connecting: false,
      lastError: silent ? prev.lastError : undefined,
    }));
  };

  const scheduleReconnect = () => {
    if (manualStopRef.current) {
      return;
    }
    clearReconnectTimer();
    reconnectTimerRef.current = window.setTimeout(() => {
      startStream();
    }, 2000);
  };

  const clearReconnectTimer = () => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  };

  return (
    <Card
      title="实时运行日志"
      size="small"
      extra={
        <Space size={8}>
          <Tag color={streamState.active ? 'green' : streamState.connecting ? 'processing' : 'default'}>
            {streamStatusText}
          </Tag>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => setLogs([])}
          >
            清空缓冲
          </Button>
          <Button
            type="primary"
            icon={<PlayCircleOutlined />}
            disabled={!canStartStream}
            onClick={startStream}
          >
            开始
          </Button>
          <Button
            icon={<PauseCircleOutlined />}
            disabled={!streamState.active && !streamState.connecting}
            onClick={() => stopStream(false)}
          >
            停止
          </Button>
        </Space>
      }
    >
      {!canStream && (
        <Alert
          showIcon
          type="warning"
          message="当前账号没有运行日志实时订阅权限（admin:runtime-log:stream）"
          style={{ marginBottom: 12 }}
        />
      )}
      {streamState.lastError && (
        <Alert
          showIcon
          type="error"
          message={`实时日志连接异常：${streamState.lastError}`}
          style={{ marginBottom: 12 }}
        />
      )}

      <Space wrap size={12} style={{ marginBottom: 12 }}>
        <Input
          style={{ width: 220 }}
          allowClear
          placeholder="关键字"
          value={request.keyword}
          onChange={(event) =>
            setRequest((prev) => ({ ...prev, keyword: event.target.value || undefined }))
          }
        />
        <Select
          style={{ width: 140 }}
          allowClear
          placeholder="日志级别"
          value={request.level}
          onChange={(value) => setRequest((prev) => ({ ...prev, level: value || undefined }))}
          options={[
            { label: 'ERROR', value: 'ERROR' },
            { label: 'WARN', value: 'WARN' },
            { label: 'INFO', value: 'INFO' },
            { label: 'DEBUG', value: 'DEBUG' },
            { label: 'TRACE', value: 'TRACE' },
          ]}
        />
        <Input
          style={{ width: 180 }}
          allowClear
          placeholder="Logger 名称"
          value={request.loggerName}
          onChange={(event) =>
            setRequest((prev) => ({ ...prev, loggerName: event.target.value || undefined }))
          }
        />
        <Input
          style={{ width: 180 }}
          allowClear
          placeholder="线程名称"
          value={request.threadName}
          onChange={(event) =>
            setRequest((prev) => ({ ...prev, threadName: event.target.value || undefined }))
          }
        />
        <InputNumber
          min={1}
          max={1000}
          value={request.lastN}
          onChange={(value) => setRequest((prev) => ({ ...prev, lastN: Number(value || 100) }))}
          addonBefore="预热"
          addonAfter="条"
        />
        <Space>
          <span>自动滚动</span>
          <Switch checked={autoScroll} onChange={setAutoScroll} />
        </Space>
      </Space>

      <div
        ref={logContainerRef}
        style={{
          maxHeight: 360,
          overflowY: 'auto',
          border: '1px solid #f0f0f0',
          borderRadius: 8,
          background: '#0f172a',
          padding: 12,
        }}
      >
        {logs.length === 0 ? (
          <Typography.Text style={{ color: '#94a3b8' }}>暂无实时日志</Typography.Text>
        ) : (
          logs.map((line) => (
            <div key={line.lineId} style={{ marginBottom: 8 }}>
              <Space size={8} align="start" wrap>
                <Typography.Text style={{ color: '#e2e8f0', fontFamily: 'Menlo, Monaco, monospace' }}>
                  {line.logTime || '-'}
                </Typography.Text>
                <Tag color={getLevelColor(line.level)}>{(line.level || 'INFO').toUpperCase()}</Tag>
                <Typography.Text style={{ color: '#7dd3fc', fontFamily: 'Menlo, Monaco, monospace' }}>
                  [{line.threadName || '-'}]
                </Typography.Text>
                <Typography.Text style={{ color: '#cbd5e1', fontFamily: 'Menlo, Monaco, monospace' }}>
                  {line.loggerName || '-'}
                </Typography.Text>
              </Space>
              <div style={{ color: '#f8fafc', fontFamily: 'Menlo, Monaco, monospace', whiteSpace: 'pre-wrap' }}>
                {line.message || '-'}
              </div>
            </div>
          ))
        )}
      </div>
    </Card>
  );
}
