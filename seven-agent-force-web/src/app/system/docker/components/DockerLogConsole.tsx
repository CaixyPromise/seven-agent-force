'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Checkbox, Input, InputNumber, Space, Switch, Tag, message } from 'antd';
import { ClearOutlined, CopyOutlined, PauseCircleOutlined, PlayCircleOutlined, ReloadOutlined } from '@ant-design/icons';
import VirtualList from '@rc-component/virtual-list';
import {
  dockerContainerLogsStreamUrl,
  getDockerContainerLogs,
  type DockerContainerLogsQuery,
} from '@/api/dockerController';

const MAX_LOG_LINES = 5000;
const LOG_LIST_HEIGHT = 360;

interface DockerLogConsoleProps {
  containerId?: string;
  active?: boolean;
}

interface LogLine {
  id: number;
  text: string;
}

function appendBounded(current: LogLine[], incoming: string[], nextId: number) {
  const lines = incoming
    .filter((line) => line.length > 0)
    .map((line, index) => ({ id: nextId + index, text: line }));
  return [...current, ...lines].slice(-MAX_LOG_LINES);
}

function splitLogText(value?: string) {
  return (value || '').split(/\r?\n/).filter((line) => line.length > 0);
}

export function DockerLogConsole({ containerId, active = true }: DockerLogConsoleProps) {
  const [tail, setTail] = useState<number>(200);
  const [since, setSince] = useState('');
  const [until, setUntil] = useState('');
  const [grep, setGrep] = useState('');
  const [timestamps, setTimestamps] = useState(false);
  const [follow, setFollow] = useState(true);
  const [paused, setPaused] = useState(false);
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState<'idle' | 'connecting' | 'streaming' | 'closed' | 'error'>('idle');
  const [errorText, setErrorText] = useState('');
  const [lines, setLines] = useState<LogLine[]>([]);
  const nextLineIdRef = useRef(1);
  const eventSourceRef = useRef<EventSource | null>(null);

  const query = useMemo<DockerContainerLogsQuery>(
    () => ({
      tail,
      since: since.trim() || undefined,
      until: until.trim() || undefined,
      timestamps,
      grep: grep.trim() || undefined,
      follow,
    }),
    [follow, grep, since, tail, timestamps, until],
  );

  const closeStream = useCallback((nextStatus: typeof status = 'closed') => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
    setLoading(false);
    setStatus((current) => (current === 'idle' ? current : nextStatus));
  }, []);

  const closeStreamSilently = useCallback(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
  }, []);

  const appendLines = useCallback((incoming: string[]) => {
    if (!incoming.length) {
      return;
    }
    setLines((current) => {
      const next = appendBounded(current, incoming, nextLineIdRef.current);
      nextLineIdRef.current += incoming.length;
      return next;
    });
  }, []);

  const loadHistory = useCallback(async () => {
    if (!containerId) {
      return;
    }
    closeStream('closed');
    setLoading(true);
    setStatus('connecting');
    setErrorText('');
    try {
      const response = await getDockerContainerLogs(containerId, { ...query, follow: false });
      const incoming = splitLogText(response.data);
      nextLineIdRef.current = 1;
      setLines(appendBounded([], incoming, nextLineIdRef.current));
      nextLineIdRef.current += incoming.length;
      setStatus('closed');
    } catch (error) {
      setErrorText((error as Error).message || '容器日志加载失败');
      setStatus('error');
    } finally {
      setLoading(false);
    }
  }, [closeStream, containerId, query]);

  const openStream = useCallback(() => {
    if (!containerId || paused) {
      return;
    }
    closeStream('closed');
    setLoading(true);
    setStatus('connecting');
    setErrorText('');

    const source = new EventSource(dockerContainerLogsStreamUrl(containerId, query));
    eventSourceRef.current = source;
    source.onopen = () => {
      setLoading(false);
      setStatus('streaming');
    };
    source.addEventListener('log', (event) => {
      const data = JSON.parse((event as MessageEvent).data) as { line?: string };
      appendLines(data.line ? [data.line] : []);
    });
    source.addEventListener('error', (event) => {
      const data = (event as MessageEvent).data;
      const messageText = data
        ? (JSON.parse(data) as { message?: string }).message
        : '容器日志流连接失败';
      setErrorText(messageText || '容器日志流连接失败');
      closeStream('error');
    });
    source.addEventListener('done', () => closeStream('closed'));
    source.onerror = () => {
      setErrorText('容器日志流连接失败');
      closeStream('error');
    };
  }, [appendLines, closeStream, containerId, paused, query]);

  const reload = useCallback(() => {
    setLines([]);
    nextLineIdRef.current = 1;
    if (follow && !paused) {
      openStream();
      return;
    }
    void loadHistory();
  }, [follow, loadHistory, openStream, paused]);

  useEffect(() => {
    if (!active || !containerId) {
      closeStreamSilently();
      return undefined;
    }
    const timer = window.setTimeout(reload, 0);
    return () => {
      window.clearTimeout(timer);
      closeStreamSilently();
    };
  }, [active, closeStreamSilently, containerId, reload]);

  useEffect(() => closeStreamSilently, [closeStreamSilently]);

  const copyLogs = () => {
    const text = lines.map((line) => line.text).join('\n');
    if (!text) {
      message.warning('暂无日志可复制');
      return;
    }
    void navigator.clipboard.writeText(text).then(
      () => message.success('日志已复制'),
      () => message.error('复制失败'),
    );
  };

  const statusLabel = {
    idle: '未连接',
    connecting: '连接中',
    streaming: '实时推送',
    closed: '已断开',
    error: '连接失败',
  }[status];

  return (
    <div className="space-y-3 px-5 pb-5">
      <div className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-3">
        <div className="grid gap-3 lg:grid-cols-[100px_1fr_1fr]">
          <InputNumber
            min={1}
            max={5000}
            value={tail}
            addonBefore="行数"
            onChange={(value) => setTail(Number(value) || 200)}
          />
          <Input allowClear placeholder="since，例如 2026-06-11T10:00:00Z" value={since} onChange={(event) => setSince(event.target.value)} />
          <Input allowClear placeholder="until，例如 2026-06-11T11:00:00Z" value={until} onChange={(event) => setUntil(event.target.value)} />
        </div>
        <div className="mt-3 flex flex-wrap items-center justify-between gap-3">
          <Input
            allowClear
            className="max-w-[320px]"
            placeholder="grep 过滤关键字"
            value={grep}
            onChange={(event) => setGrep(event.target.value)}
            onPressEnter={reload}
          />
          <Space wrap>
            <Checkbox checked={timestamps} onChange={(event) => setTimestamps(event.target.checked)}>
              时间戳
            </Checkbox>
            <span className="text-sm text-slate-500">跟随</span>
            <Switch size="small" checked={follow} onChange={setFollow} />
            <Tag color={status === 'error' ? 'error' : status === 'streaming' ? 'processing' : 'default'}>{statusLabel}</Tag>
          </Space>
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm text-slate-500">最多保留 {MAX_LOG_LINES} 行，当前 {lines.length} 行</span>
        <Space wrap>
          <Button
            size="small"
            icon={paused ? <PlayCircleOutlined /> : <PauseCircleOutlined />}
            onClick={() => {
              setPaused((current) => {
                const next = !current;
                if (!next && follow) {
                  window.setTimeout(openStream, 0);
                } else {
                  closeStream('closed');
                }
                return next;
              });
            }}
          >
            {paused ? '继续' : '暂停'}
          </Button>
          <Button size="small" loading={loading} icon={<ReloadOutlined />} onClick={reload}>
            刷新
          </Button>
          <Button size="small" icon={<ClearOutlined />} onClick={() => setLines([])}>
            清空
          </Button>
          <Button size="small" icon={<CopyOutlined />} onClick={copyLogs}>
            复制
          </Button>
        </Space>
      </div>

      <div className="rounded-2xl bg-slate-950 px-3 py-3 text-[12px] leading-6 text-slate-100">
        {errorText ? <div className="mb-2 rounded-lg bg-red-500/10 px-3 py-2 text-red-200">{errorText}</div> : null}
        {lines.length ? (
          <VirtualList<LogLine> data={lines} height={LOG_LIST_HEIGHT} itemHeight={24} itemKey="id">
            {(line) => <div className="whitespace-pre-wrap break-all font-mono">{line.text}</div>}
          </VirtualList>
        ) : (
          <div className="min-h-[180px] py-16 text-center text-slate-400">
            {loading ? '日志连接中...' : '暂无日志'}
          </div>
        )}
      </div>
    </div>
  );
}
