'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { Alert, Button, Drawer, Input, Select, Space, Tag, message } from 'antd';
import { ClearOutlined, DisconnectOutlined, PlayCircleOutlined, SendOutlined } from '@ant-design/icons';
import { dockerContainerTerminalWsUrl } from '@/api/dockerController';
import { formatContainerStateLabel, normalizeState, shortId } from './dockerFormat';

type ShellType = '/bin/sh' | '/bin/bash';
type TerminalStatus = 'idle' | 'connecting' | 'connected' | 'closed' | 'error';

interface DockerTerminalDrawerProps {
  open: boolean;
  containerId?: string;
  containerName?: string;
  containerState?: string;
  canUse?: boolean;
  onClose: () => void;
}

const MAX_TERMINAL_LINES = 1000;

function canOpenTerminal(state?: string) {
  const normalized = normalizeState(state);
  return normalized === 'running' || normalized === 'restarting';
}

function appendOutput(current: string[], incoming: string) {
  const lines = incoming.split(/\r?\n/);
  return [...current, ...lines].slice(-MAX_TERMINAL_LINES);
}

export function DockerTerminalDrawer({
  open,
  containerId,
  containerName,
  containerState,
  canUse = false,
  onClose,
}: DockerTerminalDrawerProps) {
  const [shell, setShell] = useState<ShellType>('/bin/sh');
  const [status, setStatus] = useState<TerminalStatus>('idle');
  const [errorText, setErrorText] = useState('');
  const [output, setOutput] = useState<string[]>([]);
  const [command, setCommand] = useState('');
  const socketRef = useRef<WebSocket | null>(null);

  const stateAllowsTerminal = canOpenTerminal(containerState);
  const terminalEnabled = Boolean(containerId && canUse && stateAllowsTerminal);

  const disconnect = useCallback((nextStatus: TerminalStatus = 'closed') => {
    socketRef.current?.close();
    socketRef.current = null;
    setStatus((current) => (current === 'idle' ? current : nextStatus));
  }, []);

  const closeSocketSilently = useCallback(() => {
    socketRef.current?.close();
    socketRef.current = null;
  }, []);

  const pushOutput = useCallback((value: string) => {
    setOutput((current) => appendOutput(current, value));
  }, []);

  const connect = useCallback(() => {
    if (!containerId) {
      setErrorText('缺少容器 ID，无法打开终端。');
      return;
    }
    if (!terminalEnabled) {
      setErrorText(!canUse ? '当前账号没有容器终端权限。' : '只有运行中或重启中的容器可以打开终端。');
      return;
    }

    disconnect('closed');
    setStatus('connecting');
    setErrorText('');
    setOutput([]);

    try {
      const socket = new WebSocket(dockerContainerTerminalWsUrl(containerId, { shell, rows: 32, cols: 120 }));
      socketRef.current = socket;
      socket.onopen = () => {
        setStatus('connected');
        pushOutput(`已连接 ${shell}`);
      };
      socket.onmessage = (event) => {
        if (typeof event.data === 'string') {
          pushOutput(event.data);
          return;
        }
        if (event.data instanceof Blob) {
          void event.data.text().then(pushOutput);
        }
      };
      socket.onerror = () => {
        setErrorText('终端 WebSocket 连接失败，请确认后端终端接口已启用。');
        setStatus('error');
      };
      socket.onclose = (event) => {
        if (socketRef.current === socket) {
          socketRef.current = null;
        }
        if (event.code !== 1000 && event.code !== 1005) {
          setErrorText(event.reason || `终端连接已断开，关闭码 ${event.code}`);
          setStatus('error');
          return;
        }
        setStatus('closed');
      };
    } catch (error) {
      setErrorText((error as Error).message || '终端连接创建失败');
      setStatus('error');
    }
  }, [canUse, containerId, disconnect, pushOutput, shell, terminalEnabled]);

  useEffect(() => {
    if (!open) {
      closeSocketSilently();
    }
    return closeSocketSilently;
  }, [closeSocketSilently, open]);

  const sendCommand = () => {
    const socket = socketRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN) {
      message.warning('终端尚未连接');
      return;
    }
    socket.send(`${command}\n`);
    setCommand('');
  };

  const statusLabel = {
    idle: '未连接',
    connecting: '连接中',
    connected: '已连接',
    closed: '已断开',
    error: '连接失败',
  }[status];

  return (
    <Drawer
      open={open}
      width={760}
      title={
        <Space size={8} wrap>
          <span>容器终端</span>
          <Tag>{containerName || shortId(containerId)}</Tag>
          <Tag>{formatContainerStateLabel(containerState)}</Tag>
          <Tag color={status === 'connected' ? 'processing' : status === 'error' ? 'error' : 'default'}>{statusLabel}</Tag>
        </Space>
      }
      destroyOnHidden
      onClose={() => {
        disconnect('closed');
        setCommand('');
        setErrorText('');
        setOutput([]);
        onClose();
      }}
      extra={
        <Space>
          <Select<ShellType>
            value={shell}
            disabled={status === 'connected' || status === 'connecting'}
            options={[
              { label: '/bin/sh', value: '/bin/sh' },
              { label: '/bin/bash', value: '/bin/bash' },
            ]}
            onChange={setShell}
          />
          {status === 'connected' ? (
            <Button icon={<DisconnectOutlined />} onClick={() => disconnect('closed')}>
              断开
            </Button>
          ) : (
            <Button type="primary" icon={<PlayCircleOutlined />} disabled={!terminalEnabled} loading={status === 'connecting'} onClick={connect}>
              连接
            </Button>
          )}
        </Space>
      }
    >
      <div className="space-y-3">
        {!terminalEnabled ? (
          <Alert
            type="warning"
            showIcon
            message={!canUse ? '当前账号没有容器终端权限' : '当前容器状态不可打开终端'}
            description={!canUse ? '需要容器终端权限后才能连接 WebSocket 终端。' : '仅 running/restarting 状态支持打开终端。'}
          />
        ) : null}
        {errorText ? <Alert type="error" showIcon message="终端连接异常" description={errorText} /> : null}

        <div className="rounded-2xl bg-slate-950 px-4 py-3 font-mono text-[12px] leading-6 text-slate-100">
          <div className="h-[420px] overflow-auto whitespace-pre-wrap break-all">
            {output.length ? output.join('\n') : '点击连接后打开容器终端'}
          </div>
        </div>

        <div className="flex gap-2">
          <Input
            value={command}
            disabled={status !== 'connected'}
            placeholder="输入命令后回车发送"
            onChange={(event) => setCommand(event.target.value)}
            onPressEnter={sendCommand}
          />
          <Button icon={<SendOutlined />} disabled={status !== 'connected'} onClick={sendCommand}>
            发送
          </Button>
          <Button icon={<ClearOutlined />} onClick={() => setOutput([])}>
            清空
          </Button>
        </div>
      </div>
    </Drawer>
  );
}
