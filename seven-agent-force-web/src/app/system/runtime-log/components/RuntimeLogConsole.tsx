'use client';

import React, {
  startTransition,
  useCallback,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import dayjs from 'dayjs';
import {
  getRuntimeLogPage,
  openRuntimeLogStream,
  type RuntimeLogStreamHandle,
} from '@/api/runtimeLogController';
import type { RuntimeLogLine, RuntimeLogPageRequest, RuntimeLogStreamRequest } from '@/lib/http/types';
import styles from './RuntimeLogConsole.module.css';
import { RuntimeLogConsoleToolbar } from './RuntimeLogConsoleToolbar';
import { RuntimeLogConsoleViewer } from './RuntimeLogConsoleViewer';
import type {
  ConsoleFilterState,
  ConsoleMode,
  ContextMenuState,
  LocalFilterResult,
  StreamState,
} from './runtimeLogConsole.types';

interface RuntimeLogConsoleProps {
  canStream: boolean;
}

interface TextMatcher {
  test: (text?: string | null) => boolean;
  invalid: boolean;
}

interface RealtimeLogMatcher {
  hasInvalidRegex: boolean;
  matches: (line: RuntimeLogLine) => boolean;
}

const HISTORY_PAGE_SIZE = 500;
const STREAM_MAX_BUFFER = 2000;
const STREAM_FLUSH_INTERVAL_MS = 150;

interface RuntimeLogPageEnvelope {
  code?: number;
  message?: string;
  data?: {
    records?: RuntimeLogLine[];
    [key: string]: unknown;
  } | null;
  records?: RuntimeLogLine[];
}

function tryParseJsonPayload(raw: unknown): unknown {
  if (typeof raw !== 'string') {
    return raw;
  }
  const trimmed = raw.trim();
  if (!trimmed || (!trimmed.startsWith('{') && !trimmed.startsWith('['))) {
    return raw;
  }
  try {
    return JSON.parse(trimmed);
  } catch {
    return raw;
  }
}

function toDatetimeLocal(value: dayjs.ConfigType) {
  return dayjs(value).format('YYYY-MM-DDTHH:mm');
}

function toBackendDateTime(value?: string) {
  if (!value) {
    return undefined;
  }
  const parsed = dayjs(value);
  if (!parsed.isValid()) {
    return undefined;
  }
  return parsed.format('YYYY-MM-DD HH:mm:ss');
}

function clampCopyN(value?: number | null) {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return 10;
  }
  return Math.max(1, Math.min(100, Math.floor(value)));
}

function levelClassName(level?: string) {
  const normalized = (level || '').toUpperCase();
  if (normalized === 'ERROR') {
    return styles.levelError;
  }
  if (normalized === 'WARN') {
    return styles.levelWarn;
  }
  if (normalized === 'INFO') {
    return styles.levelInfo;
  }
  if (normalized === 'DEBUG') {
    return styles.levelDebug;
  }
  if (normalized === 'TRACE') {
    return styles.levelTrace;
  }
  return styles.levelDefault;
}

function levelTagColor(level?: string) {
  const normalized = (level || '').toUpperCase();
  if (normalized === 'ERROR') {
    return 'error';
  }
  if (normalized === 'WARN') {
    return 'warning';
  }
  if (normalized === 'DEBUG') {
    return 'processing';
  }
  if (normalized === 'TRACE') {
    return 'default';
  }
  return 'success';
}

function normalizeText(text?: string | null) {
  return (text || '').trim().toLowerCase();
}

function createTextMatcher(keyword: string, useRegex: boolean): TextMatcher {
  const trimmed = keyword.trim();
  if (!trimmed) {
    return { test: () => true, invalid: false };
  }
  if (!useRegex) {
    const normalized = trimmed.toLowerCase();
    return {
      invalid: false,
      test: (text?: string | null) => normalizeText(text).includes(normalized),
    };
  }
  try {
    const regex = new RegExp(trimmed, 'i');
    return {
      invalid: false,
      test: (text?: string | null) => regex.test(text || ''),
    };
  } catch {
    return { test: () => true, invalid: true };
  }
}

function filterLogsLocally(
  sourceLogs: RuntimeLogLine[],
  filter: ConsoleFilterState,
  mode: ConsoleMode,
): LocalFilterResult {
  const level = filter.level === 'ALL' ? '' : filter.level;
  const contentMatcher = createTextMatcher(filter.contentKw, filter.useRegex);
  const loggerMatcher = createTextMatcher(filter.loggerKw, filter.useRegex);
  const threadMatcher = createTextMatcher(filter.threadKw, filter.useRegex);
  const traceMatcher = createTextMatcher(filter.traceId, filter.useRegex);
  const start = filter.startTime ? dayjs(filter.startTime) : null;
  const end = filter.endTime ? dayjs(filter.endTime) : null;

  const logs = sourceLogs.filter((line) => {
    if (level && (line.level || '').toUpperCase() !== level) {
      return false;
    }
    if (mode === 'history' && line.logTime) {
      const lineTime = dayjs(line.logTime);
      if (start?.isValid() && lineTime.isBefore(start)) {
        return false;
      }
      if (end?.isValid() && lineTime.isAfter(end)) {
        return false;
      }
    }
    if (!contentMatcher.test(line.message)) {
      return false;
    }
    if (!loggerMatcher.test(line.loggerName)) {
      return false;
    }
    if (!threadMatcher.test(line.threadName)) {
      return false;
    }
    if (!traceMatcher.test(line.traceId)) {
      return false;
    }
    return true;
  });

  return {
    logs,
    hasInvalidRegex: contentMatcher.invalid || loggerMatcher.invalid || threadMatcher.invalid || traceMatcher.invalid,
  };
}

function buildRealtimeMatcher(filter: ConsoleFilterState): RealtimeLogMatcher {
  const level = filter.level === 'ALL' ? '' : filter.level;
  const contentMatcher = createTextMatcher(filter.contentKw, filter.useRegex);
  const loggerMatcher = createTextMatcher(filter.loggerKw, filter.useRegex);
  const threadMatcher = createTextMatcher(filter.threadKw, filter.useRegex);
  const traceMatcher = createTextMatcher(filter.traceId, filter.useRegex);
  return {
    hasInvalidRegex: contentMatcher.invalid || loggerMatcher.invalid || threadMatcher.invalid || traceMatcher.invalid,
    matches: (line: RuntimeLogLine) => {
      if (level && (line.level || '').toUpperCase() !== level) {
        return false;
      }
      return (
        contentMatcher.test(line.message) &&
        loggerMatcher.test(line.loggerName) &&
        threadMatcher.test(line.threadName) &&
        traceMatcher.test(line.traceId)
      );
    },
  };
}

function clampStreamBuffer(lines: RuntimeLogLine[]) {
  if (lines.length <= STREAM_MAX_BUFFER) {
    return lines;
  }
  return lines.slice(lines.length - STREAM_MAX_BUFFER);
}

function normalizeHistoryResponse(raw: unknown) {
  const payload = (tryParseJsonPayload(raw) || {}) as RuntimeLogPageEnvelope;
  const parsedData = tryParseJsonPayload(payload.data);
  if (typeof payload.code === 'number') {
    if (payload.code !== 0 || !parsedData) {
      return {
        ok: false as const,
        message: payload.message || '查询运行日志失败',
        records: [] as RuntimeLogLine[],
      };
    }
    if (Array.isArray(parsedData)) {
      return {
        ok: true as const,
        message: '',
        records: parsedData,
      };
    }
    return {
      ok: true as const,
      message: '',
      records:
        typeof parsedData === 'object' && parsedData !== null && Array.isArray((parsedData as { records?: RuntimeLogLine[] }).records)
          ? (parsedData as { records: RuntimeLogLine[] }).records
          : [],
    };
  }
  if (Array.isArray(payload.records)) {
    return {
      ok: true as const,
      message: '',
      records: payload.records,
    };
  }
  return {
    ok: false as const,
    message: '查询运行日志失败：返回结构不符合预期',
    records: [] as RuntimeLogLine[],
  };
}

function isNearBottom(container: HTMLDivElement, threshold = 24) {
  return container.scrollHeight - container.scrollTop - container.clientHeight < threshold;
}

function formatLogLine(line: RuntimeLogLine) {
  const level = ((line.level || 'INFO').toUpperCase() + '     ').slice(0, 5);
  const timestamp = line.logTime || '-';
  const thread = line.threadName || '-';
  const logger = line.loggerName || '-';
  const trace = line.traceId ? ` trace=${line.traceId}` : '';
  const message = line.message || '';
  const source = line.source && Object.keys(line.source).length > 0
    ? ` source=${JSON.stringify(line.source)}`
    : '';
  return `${timestamp} ${level} [${thread}] ${logger}${trace} : ${message}${source}`;
}

function shouldAutoReconnectStream(error?: Error) {
  const normalizedMessage = (error?.message || '').toLowerCase();
  if (!normalizedMessage) {
    return true;
  }
  return !(
    normalizedMessage.includes('401')
    || normalizedMessage.includes('403')
    || normalizedMessage.includes('未登录')
    || normalizedMessage.includes('无权限')
    || normalizedMessage.includes('登录状态已失效')
  );
}

async function copyText(text: string) {
  if (!text) {
    return;
  }
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const textArea = document.createElement('textarea');
  textArea.value = text;
  document.body.appendChild(textArea);
  textArea.select();
  document.execCommand('copy');
  document.body.removeChild(textArea);
}

function useDebouncedValue<T>(value: T, delay = 400) {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedValue(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return debouncedValue;
}

export function RuntimeLogConsole({ canStream }: RuntimeLogConsoleProps) {
  const now = dayjs();
  const [mode, setMode] = useState<ConsoleMode>('realtime');
  const [isLive, setIsLive] = useState(true);
  const [autoScroll, setAutoScroll] = useState(true);
  const [isHoveringAction, setIsHoveringAction] = useState(false);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState<string>();
  const [streamState, setStreamState] = useState<StreamState>({ active: false, connecting: false });
  const [streamRetrySeed, setStreamRetrySeed] = useState(0);
  const [streamLogs, setStreamLogs] = useState<RuntimeLogLine[]>([]);
  const [historyLogs, setHistoryLogs] = useState<RuntimeLogLine[]>([]);
  const [filter, setFilter] = useState<ConsoleFilterState>({
    level: 'ALL',
    contentKw: '',
    loggerKw: '',
    threadKw: '',
    traceId: '',
    useRegex: false,
    copyN: 10,
    startTime: toDatetimeLocal(now.subtract(1, 'hour')),
    endTime: toDatetimeLocal(now),
  });
  const [selectedLogIds, setSelectedLogIds] = useState<Set<string>>(new Set());
  const [copiedKey, setCopiedKey] = useState<string>();
  const [contextMenu, setContextMenu] = useState<ContextMenuState>({ visible: false, x: 0, y: 0 });
  const [isBulkCopied, setIsBulkCopied] = useState(false);

  const dragStateRef = useRef({ dragging: false, selecting: true });
  const logContainerRef = useRef<HTMLDivElement | null>(null);
  const streamHandleRef = useRef<RuntimeLogStreamHandle | null>(null);
  const reconnectTimerRef = useRef<number | null>(null);
  const manualStopRef = useRef(false);
  const autoScrollRef = useRef(autoScroll);
  const modeRef = useRef(mode);
  const isLiveRef = useRef(isLive);
  const canStreamRef = useRef(canStream);
  const pendingStreamLogsRef = useRef<RuntimeLogLine[]>([]);
  const streamFlushTimerRef = useRef<number | null>(null);

  const deferredFilter = useDeferredValue(filter);
  const debouncedFilter = useDebouncedValue(deferredFilter, 400);
  const realtimeMatcher = useMemo(
    () => buildRealtimeMatcher(debouncedFilter),
    [debouncedFilter],
  );
  const realtimeFilteredLogs = useMemo(
    () => streamLogs.filter((line) => realtimeMatcher.matches(line)),
    [realtimeMatcher, streamLogs],
  );

  const historyFilterResult = useMemo(
    () => filterLogsLocally(historyLogs, debouncedFilter, 'history'),
    [historyLogs, debouncedFilter],
  );
  const filteredLogs = mode === 'history' ? historyFilterResult.logs : realtimeFilteredLogs;
  const hasInvalidRegex =
    mode === 'history' ? historyFilterResult.hasInvalidRegex : realtimeMatcher.hasInvalidRegex;
  const selectedRows = useMemo(
    () => filteredLogs.filter((line) => line.lineId && selectedLogIds.has(line.lineId)),
    [filteredLogs, selectedLogIds],
  );
  const contextLine = useMemo(
    () => filteredLogs.find((line) => line.lineId === contextMenu.lineId),
    [filteredLogs, contextMenu.lineId],
  );

  const closeContextMenu = useCallback(() => {
    setContextMenu((previous) => ({ ...previous, visible: false }));
  }, []);

  const clearReconnectTimer = useCallback(() => {
    if (reconnectTimerRef.current !== null) {
      window.clearTimeout(reconnectTimerRef.current);
      reconnectTimerRef.current = null;
    }
  }, []);

  const closeStreamHandle = useCallback(() => {
    streamHandleRef.current?.close();
    streamHandleRef.current = null;
  }, []);

  const clearStreamFlushTimer = useCallback(() => {
    if (streamFlushTimerRef.current !== null) {
      window.clearTimeout(streamFlushTimerRef.current);
      streamFlushTimerRef.current = null;
    }
  }, []);

  useEffect(() => {
    autoScrollRef.current = autoScroll;
  }, [autoScroll]);

  useEffect(() => {
    modeRef.current = mode;
  }, [mode]);

  useEffect(() => {
    isLiveRef.current = isLive;
  }, [isLive]);

  useEffect(() => {
    canStreamRef.current = canStream;
  }, [canStream]);

  const flushPendingStreamLogs = useCallback(() => {
    streamFlushTimerRef.current = null;
    const batch = pendingStreamLogsRef.current;
    if (!batch.length) {
      return;
    }
    const container = logContainerRef.current;
    const inRealtimeMode = modeRef.current === 'realtime';
    const shouldAutoScroll = autoScrollRef.current;
    const shouldFollow = inRealtimeMode && shouldAutoScroll && !!container && isNearBottom(container);
    if (inRealtimeMode && shouldAutoScroll && container && !shouldFollow) {
      setAutoScroll(false);
    }
    pendingStreamLogsRef.current = [];
    startTransition(() => {
      setStreamLogs((previous) => clampStreamBuffer([...previous, ...batch]));
    });
  }, [setAutoScroll, setStreamLogs]);

  const scheduleStreamFlush = useCallback(() => {
    if (streamFlushTimerRef.current !== null) {
      return;
    }
    streamFlushTimerRef.current = window.setTimeout(flushPendingStreamLogs, STREAM_FLUSH_INTERVAL_MS);
  }, [flushPendingStreamLogs]);

  const scheduleReconnect = useCallback(() => {
    if (
      manualStopRef.current ||
      modeRef.current !== 'realtime' ||
      !isLiveRef.current ||
      !canStreamRef.current
    ) {
      return;
    }
    clearReconnectTimer();
    reconnectTimerRef.current = window.setTimeout(() => {
      setStreamRetrySeed((value) => value + 1);
    }, 2000);
  }, [clearReconnectTimer, setStreamRetrySeed]);

  useEffect(() => {
    const handleMouseUp = () => {
      dragStateRef.current.dragging = false;
    };
    const handleGlobalClick = () => {
      if (contextMenu.visible) {
        closeContextMenu();
      }
    };
    const handleEsc = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        closeContextMenu();
      }
    };
    window.addEventListener('mouseup', handleMouseUp);
    window.addEventListener('click', handleGlobalClick);
    window.addEventListener('keydown', handleEsc);
    return () => {
      window.removeEventListener('mouseup', handleMouseUp);
      window.removeEventListener('click', handleGlobalClick);
      window.removeEventListener('keydown', handleEsc);
    };
  }, [closeContextMenu, contextMenu.visible]);

  useEffect(() => {
    if (mode !== 'history') {
      return;
    }
    let cancelled = false;

    const requestPayload: RuntimeLogPageRequest = {
      current: 1,
      size: HISTORY_PAGE_SIZE,
      level: debouncedFilter.level === 'ALL' ? undefined : debouncedFilter.level,
      contentKeyword: debouncedFilter.contentKw || undefined,
      loggerName: debouncedFilter.loggerKw || undefined,
      threadName: debouncedFilter.threadKw || undefined,
      traceId: debouncedFilter.traceId || undefined,
      useRegex: debouncedFilter.useRegex,
      startTime: toBackendDateTime(debouncedFilter.startTime),
      endTime: toBackendDateTime(debouncedFilter.endTime),
    };

    const requestTimer = window.setTimeout(() => {
      setHistoryLoading(true);
      setHistoryError(undefined);
      void getRuntimeLogPage(requestPayload)
        .then((response) => {
          if (cancelled) {
            return;
          }
          const normalizedResponse = normalizeHistoryResponse(response);
          if (!normalizedResponse.ok) {
            setHistoryError(normalizedResponse.message);
            startTransition(() => setHistoryLogs([]));
            return;
          }
          const sorted = [...normalizedResponse.records].sort((left, right) =>
            dayjs(left.logTime).valueOf() - dayjs(right.logTime).valueOf(),
          );
          startTransition(() => setHistoryLogs(sorted));
        })
        .catch((error: unknown) => {
          if (cancelled) {
            return;
          }
          setHistoryError(error instanceof Error ? error.message : '查询运行日志失败');
          startTransition(() => setHistoryLogs([]));
        })
        .finally(() => {
          if (!cancelled) {
            setHistoryLoading(false);
          }
        });
    }, 0);

    return () => {
      cancelled = true;
      window.clearTimeout(requestTimer);
    };
  }, [debouncedFilter, mode]);

  useEffect(() => {
    clearReconnectTimer();
    closeStreamHandle();
    const resetStateTimer = window.setTimeout(() => {
      setStreamState((previous) => ({
        ...previous,
        active: false,
        connecting: false,
        lastError: undefined,
      }));
    }, 0);

    if (mode !== 'realtime' || !isLive || !canStream) {
      manualStopRef.current = mode !== 'realtime' || !isLive;
      return () => window.clearTimeout(resetStateTimer);
    }

    manualStopRef.current = false;
    let active = true;
    const connectingStateTimer = window.setTimeout(() => {
      if (!active) {
        return;
      }
      setStreamState((previous) => ({
        ...previous,
        active: false,
        connecting: true,
        lastError: undefined,
      }));
    }, 0);

    const requestPayload: RuntimeLogStreamRequest = {
      level: debouncedFilter.level === 'ALL' ? undefined : debouncedFilter.level,
      contentKeyword: debouncedFilter.contentKw || undefined,
      loggerName: debouncedFilter.loggerKw || undefined,
      threadName: debouncedFilter.threadKw || undefined,
      traceId: debouncedFilter.traceId || undefined,
      useRegex: debouncedFilter.useRegex,
      lastN: 120,
    };

    let currentHandle: RuntimeLogStreamHandle | null = null;
    const isCurrentStream = () => active && currentHandle !== null && streamHandleRef.current === currentHandle;

    const streamHandle = openRuntimeLogStream(requestPayload, {
      onConnected: () => {
        if (!isCurrentStream()) {
          return;
        }
        setStreamState((previous) => ({
          ...previous,
          active: true,
          connecting: false,
          lastError: undefined,
        }));
      },
      onHeartbeat: () => {
        if (!isCurrentStream()) {
          return;
        }
        setStreamState((previous) => ({ ...previous, lastHeartbeatAt: Date.now() }));
      },
      onLog: (line) => {
        if (!isCurrentStream()) {
          return;
        }
        pendingStreamLogsRef.current.push(line);
        scheduleStreamFlush();
        setStreamState((previous) => ({
          ...previous,
          active: true,
          connecting: false,
          lastError: undefined,
        }));
      },
      onError: (error) => {
        if (!isCurrentStream()) {
          return;
        }
        flushPendingStreamLogs();
        const shouldReconnect = shouldAutoReconnectStream(error);
        if (!shouldReconnect) {
          manualStopRef.current = true;
          setIsLive(false);
        }
        setStreamState((previous) => ({
          ...previous,
          active: false,
          connecting: false,
          lastError: error.message,
        }));
        if (shouldReconnect) {
          scheduleReconnect();
        }
      },
    });

    currentHandle = streamHandle;
    streamHandleRef.current = streamHandle;
    void streamHandle.done.then(() => {
      if (!isCurrentStream()) {
        return;
      }
      if (!manualStopRef.current) {
        setStreamState((previous) => ({
          ...previous,
          active: false,
          connecting: false,
        }));
        scheduleReconnect();
      }
    });

    return () => {
      active = false;
      manualStopRef.current = true;
      window.clearTimeout(resetStateTimer);
      window.clearTimeout(connectingStateTimer);
      flushPendingStreamLogs();
      if (streamHandleRef.current === streamHandle) {
        streamHandleRef.current = null;
      }
      streamHandle.close();
    };
  }, [
    clearReconnectTimer,
    closeStreamHandle,
    debouncedFilter.contentKw,
    debouncedFilter.level,
    debouncedFilter.loggerKw,
    debouncedFilter.traceId,
    debouncedFilter.threadKw,
    debouncedFilter.useRegex,
    flushPendingStreamLogs,
    isLive,
    mode,
    canStream,
    scheduleReconnect,
    scheduleStreamFlush,
    streamRetrySeed,
  ]);

  useEffect(() => {
    return () => {
      manualStopRef.current = true;
      clearReconnectTimer();
      clearStreamFlushTimer();
      pendingStreamLogsRef.current = [];
      closeStreamHandle();
    };
  }, [clearReconnectTimer, clearStreamFlushTimer, closeStreamHandle]);

  useEffect(() => {
    if (mode !== 'realtime' || !autoScroll || isHoveringAction || !logContainerRef.current) {
      return;
    }
    const container = logContainerRef.current;
    container.scrollTop = container.scrollHeight;
  }, [autoScroll, filteredLogs, isHoveringAction, mode]);

  useEffect(() => {
    const container = logContainerRef.current;
    if (!container) {
      return;
    }
    const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
    if (container.scrollTop > maxScrollTop) {
      container.scrollTop = maxScrollTop;
    }
  }, [filteredLogs.length, mode]);

  const onModeChange = (value: ConsoleMode) => {
    setMode(value);
    closeContextMenu();
    setSelectedLogIds(new Set());
    setCopiedKey(undefined);
    setIsBulkCopied(false);
    setAutoScroll(value === 'realtime');
    if (value === 'history') {
      setIsLive(false);
      return;
    }
    setIsLive(true);
  };

  const onRefreshHistory = useCallback(() => {
    setFilter((previous) => ({ ...previous }));
  }, []);

  const onScrollConsole = () => {
    if (mode !== 'realtime') {
      return;
    }
    const target = logContainerRef.current;
    if (!target) {
      return;
    }
    const isAtBottom = isNearBottom(target);
    if (!isAtBottom) {
      setAutoScroll(false);
      return;
    }
    if (selectedLogIds.size === 0) {
      setAutoScroll(true);
    }
  };

  const onStartLive = () => {
    if (!canStream) {
      return;
    }
    setMode('realtime');
    setIsLive(true);
    setAutoScroll(true);
    clearReconnectTimer();
    setStreamRetrySeed((value) => value + 1);
  };

  const onPauseLive = () => {
    manualStopRef.current = true;
    clearReconnectTimer();
    flushPendingStreamLogs();
    closeStreamHandle();
    setIsLive(false);
    setStreamState((previous) => ({
      ...previous,
      active: false,
      connecting: false,
    }));
  };

  const onClearPanel = () => {
    if (mode === 'history') {
      setHistoryLogs([]);
      return;
    }
    pendingStreamLogsRef.current = [];
    clearStreamFlushTimer();
    setStreamLogs([]);
  };

  const updateFilter = (key: keyof ConsoleFilterState, value: ConsoleFilterState[keyof ConsoleFilterState]) => {
    setFilter((previous) => ({ ...previous, [key]: value }));
  };

  const handleCopySingle = async (line: RuntimeLogLine) => {
    await copyText(formatLogLine(line));
    setCopiedKey(`single-${line.lineId}`);
    window.setTimeout(() => setCopiedKey(undefined), 1600);
  };

  const handleCopyContext = async (line: RuntimeLogLine) => {
    const lineId = line.lineId;
    if (!lineId) {
      return;
    }
    const currentIndex = filteredLogs.findIndex((item) => item.lineId === lineId);
    if (currentIndex < 0) {
      return;
    }
    const copyN = clampCopyN(filter.copyN);
    const startIndex = Math.max(0, currentIndex - copyN);
    const endIndex = Math.min(filteredLogs.length, currentIndex + copyN + 1);
    const content = filteredLogs.slice(startIndex, endIndex).map(formatLogLine).join('\n');
    await copyText(content);
    setCopiedKey(`context-${lineId}`);
    window.setTimeout(() => setCopiedKey(undefined), 1600);
  };

  const handleCopySelected = async () => {
    if (!selectedRows.length) {
      return;
    }
    await copyText(selectedRows.map(formatLogLine).join('\n'));
    setIsBulkCopied(true);
    window.setTimeout(() => setIsBulkCopied(false), 1600);
  };

  const handleDragStart = (event: React.MouseEvent<HTMLDivElement>, lineId?: string) => {
    if (!lineId) {
      return;
    }
    event.preventDefault();
    setSelectedLogIds((previous) => {
      const next = new Set(previous);
      const selecting = !next.has(lineId);
      dragStateRef.current = { dragging: true, selecting };
      if (selecting) {
        next.add(lineId);
      } else {
        next.delete(lineId);
      }
      if (next.size > 0) {
        setAutoScroll(false);
      }
      return next;
    });
  };

  const handleDragEnter = (lineId?: string) => {
    if (!lineId || !dragStateRef.current.dragging) {
      return;
    }
    setSelectedLogIds((previous) => {
      const next = new Set(previous);
      if (dragStateRef.current.selecting) {
        next.add(lineId);
      } else {
        next.delete(lineId);
      }
      return next;
    });
  };

  const handleContextMenu = (event: React.MouseEvent<HTMLDivElement>, lineId?: string) => {
    if (!lineId) {
      return;
    }
    event.preventDefault();
    const menuWidth = 188;
    const menuHeight = 94;
    let x = event.clientX;
    let y = event.clientY;
    if (window.innerWidth - x < menuWidth) {
      x = x - menuWidth;
    }
    if (window.innerHeight - y < menuHeight) {
      y = y - menuHeight;
    }
    setContextMenu({ visible: true, x, y, lineId });
  };

  const handleFollowBottom = useCallback(() => {
    setAutoScroll(true);
  }, [setAutoScroll]);

  return (
    <div className={styles.container}>
      <RuntimeLogConsoleToolbar
        canStream={canStream}
        mode={mode}
        isLive={isLive}
        streamState={streamState}
        historyLoading={historyLoading}
        historyError={historyError}
        hasInvalidRegex={hasInvalidRegex}
        filter={filter}
        onModeChange={onModeChange}
        onFilterChange={updateFilter}
        onStartLive={onStartLive}
        onPauseLive={onPauseLive}
        onRefreshHistory={onRefreshHistory}
        onClearPanel={onClearPanel}
        clampCopyN={clampCopyN}
      />
      <RuntimeLogConsoleViewer
        mode={mode}
        isLive={isLive}
        streamState={streamState}
        historyLoading={historyLoading}
        filteredLogs={filteredLogs}
        autoScroll={autoScroll}
        selectedLogIds={selectedLogIds}
        copiedKey={copiedKey}
        isBulkCopied={isBulkCopied}
        contextMenu={contextMenu}
        contextLine={contextLine}
        copyN={clampCopyN(filter.copyN)}
        logContainerRef={logContainerRef}
        onScrollConsole={onScrollConsole}
        onHandleContextMenu={handleContextMenu}
        onDragStart={handleDragStart}
        onDragEnter={handleDragEnter}
        onHoverActionChange={setIsHoveringAction}
        onCopySingle={handleCopySingle}
        onCopyContext={handleCopyContext}
        onCopySelected={handleCopySelected}
        onClearSelection={() => setSelectedLogIds(new Set())}
        onFollowBottom={handleFollowBottom}
        onCloseContextMenu={closeContextMenu}
        levelClassName={levelClassName}
        levelTagColor={levelTagColor}
      />
    </div>
  );
}
