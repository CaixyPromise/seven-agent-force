'use client';

import React from 'react';
import { Button, Checkbox, Empty, Space, Tag, Tooltip, Typography } from 'antd';
import {
  CheckOutlined,
  CopyOutlined,
  ReloadOutlined,
  VerticalAlignBottomOutlined,
} from '@ant-design/icons';
import type { RuntimeLogLine } from '@/lib/http/types';
import styles from './RuntimeLogConsole.module.css';
import type { ConsoleMode, ContextMenuState, StreamState } from './runtimeLogConsole.types';

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringField(source: Record<string, unknown> | undefined, key: string) {
  const value = source?.[key];
  if (typeof value === 'string') {
    return value.trim();
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }
  return '';
}

function objectField(source: Record<string, unknown> | undefined, key: string) {
  const value = source?.[key];
  return isRecord(value) ? value : undefined;
}

function compactJson(value: unknown, maxLength = 120) {
  if (value === undefined || value === null) {
    return '';
  }
  const text = typeof value === 'string' ? value : JSON.stringify(value);
  return text.length > maxLength ? `${text.slice(0, maxLength)}...` : text;
}

function formatRuntimeLogSize(value: string) {
  const size = Number(value);
  if (!Number.isFinite(size) || size < 0) {
    return '';
  }
  if (size < 1024) {
    return `${size}B`;
  }
  return `${(size / 1024).toFixed(1)}KB`;
}

function buildRuntimeLogContent(line: RuntimeLogLine) {
  const source = line.source;
  const message = (line.message || '').trim() || '-';
  if (!source || Object.keys(source).length === 0) {
    return { title: message, detail: '' };
  }

  const method = stringField(source, 'method');
  const path = stringField(source, 'path');
  const status = stringField(source, 'status');
  const latency = stringField(source, 'latency_ms');
  const responseSize = formatRuntimeLogSize(stringField(source, 'response_size'));
  const rawQuery = stringField(source, 'raw_query');
  const clientIP = stringField(source, 'client_ip');
  const errorCode = stringField(source, 'error_code');
  const errorMessage = stringField(source, 'error_message');
  const payload = objectField(source, 'payload');
  const payloadQuery = objectField(payload, 'query');
  const requestBody = objectField(payload, 'body') || payload;
  const route = [method, path].filter(Boolean).join(' ');
  const querySummary = rawQuery || compactJson(payloadQuery, 180);
  const requestSummary = requestBody && Object.keys(requestBody).length > 0
    ? compactJson(requestBody, 180)
    : '';

  if (message === 'request_started') {
    return {
      title: route ? `${method || 'REQUEST'} ${path || '-'}` : message,
      detail: [
        'request_started',
        clientIP && `ip ${clientIP}`,
        querySummary && `query ${querySummary}`,
        requestSummary && `payload ${requestSummary}`,
      ].filter(Boolean).join(' · '),
    };
  }
  if (message === 'request_finished') {
    const statusText = status ? `${status}${errorMessage ? ` ${errorMessage}` : ''}` : '';
    return {
      title: route ? `${method || 'REQUEST'} ${path || '-'}` : message,
      detail: [
        'request_finished',
        statusText && `status ${statusText}`,
        errorCode && `error ${errorCode}`,
        latency && `${latency}ms`,
        responseSize,
      ].filter(Boolean).join(' · '),
    };
  }

  return {
    title: route ? `${message} · ${route}` : message,
    detail: [
      errorMessage,
      status && `status ${status}`,
      errorCode && `error ${errorCode}`,
      latency && `${latency}ms`,
      clientIP && `ip ${clientIP}`,
      querySummary && `query ${querySummary}`,
    ].filter(Boolean).join(' · '),
  };
}

function formatRuntimeLogSource(line: RuntimeLogLine) {
  const source = isRecord(line.source) ? line.source : {};
  const payload = {
    content: line.message || '',
    traceId: line.traceId || undefined,
    logger: line.loggerName || undefined,
    thread: line.threadName || undefined,
    file: line.fileName ? `${line.fileName}${line.lineNumber ? `:${line.lineNumber}` : ''}` : undefined,
    source,
  };
  return JSON.stringify(payload, null, 2);
}

function renderRuntimeLogSourceTooltip(line: RuntimeLogLine) {
  if (!line.source || Object.keys(line.source).length === 0) {
    return line.message || '-';
  }
  return (
    <div className={styles.sourceTooltipContent}>
      <div className={styles.sourceTooltipTitle}>源信息</div>
      <pre>{formatRuntimeLogSource(line)}</pre>
    </div>
  );
}

interface RuntimeLogConsoleViewerProps {
  mode: ConsoleMode;
  isLive: boolean;
  streamState: StreamState;
  historyLoading: boolean;
  filteredLogs: RuntimeLogLine[];
  autoScroll: boolean;
  selectedLogIds: Set<string>;
  copiedKey?: string;
  isBulkCopied: boolean;
  contextMenu: ContextMenuState;
  contextLine?: RuntimeLogLine;
  copyN: number;
  logContainerRef: React.RefObject<HTMLDivElement | null>;
  onScrollConsole: () => void;
  onHandleContextMenu: (event: React.MouseEvent<HTMLDivElement>, lineId?: string) => void;
  onDragStart: (event: React.MouseEvent<HTMLDivElement>, lineId?: string) => void;
  onDragEnter: (lineId?: string) => void;
  onHoverActionChange: (value: boolean) => void;
  onCopySingle: (line: RuntimeLogLine) => Promise<void>;
  onCopyContext: (line: RuntimeLogLine) => Promise<void>;
  onCopySelected: () => Promise<void>;
  onClearSelection: () => void;
  onFollowBottom: () => void;
  onCloseContextMenu: () => void;
  levelClassName: (level?: string) => string;
  levelTagColor: (level?: string) => string;
}

export function RuntimeLogConsoleViewer({
  mode,
  isLive,
  streamState,
  historyLoading,
  filteredLogs,
  autoScroll,
  selectedLogIds,
  copiedKey,
  isBulkCopied,
  contextMenu,
  contextLine,
  copyN,
  logContainerRef,
  onScrollConsole,
  onHandleContextMenu,
  onDragStart,
  onDragEnter,
  onHoverActionChange,
  onCopySingle,
  onCopyContext,
  onCopySelected,
  onClearSelection,
  onFollowBottom,
  onCloseContextMenu,
  levelClassName,
  levelTagColor,
}: RuntimeLogConsoleViewerProps) {
  const selectedCount = selectedLogIds.size;
  const scrollerRef = React.useRef<HTMLDivElement>(null);

  const isNearBottom = React.useCallback((container: HTMLDivElement, threshold = 24) => {
    return container.scrollHeight - container.scrollTop - container.clientHeight < threshold;
  }, []);

  const scrollToBottom = React.useCallback(() => {
    scrollerRef.current?.scrollTo({ top: scrollerRef.current.scrollHeight, behavior: 'smooth' });
  }, []);

  React.useEffect(() => {
    logContainerRef.current = scrollerRef.current;
  }, [logContainerRef]);

  React.useEffect(() => {
    if (mode !== 'realtime' || !autoScroll) {
      return;
    }
    const container = logContainerRef.current;
    if (container && !isNearBottom(container)) {
      return;
    }
    scrollToBottom();
  }, [autoScroll, isNearBottom, mode, scrollToBottom, logContainerRef]);

  return (
    <div className={styles.consoleWrap}>
      {mode === 'realtime' && isLive && streamState.active && <div className={styles.scanLine} />}
      <div ref={scrollerRef} className={styles.consoleScroller} onScroll={onScrollConsole}>
        {historyLoading ? (
          <div className={styles.emptyState}>
            <Space direction="vertical" align="center">
              <ReloadOutlined spin style={{ color: '#38bdf8' }} />
              <Typography.Text style={{ color: '#94a3b8' }}>正在加载历史日志...</Typography.Text>
            </Space>
          </div>
        ) : filteredLogs.length === 0 ? (
          <div className={styles.emptyState}>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={mode === 'history' ? '当前条件下没有历史日志' : '实时流暂无日志'}
            />
          </div>
        ) : (
          <div className={styles.logList}>
            {filteredLogs.map((line) => {
              const lineId = line.lineId;
              const selected = !!lineId && selectedLogIds.has(lineId);
              const singleCopied = copiedKey === `single-${lineId}`;
              const contextCopied = copiedKey === `context-${lineId}`;
              const content = buildRuntimeLogContent(line);
              return (
                <div
                  key={lineId || `${line.logTime}-${line.loggerName}-${line.message}`}
                  className={`${styles.logRow} ${levelClassName(line.level)} ${
                    selected ? styles.logRowSelected : ''
                  }`}
                  onContextMenu={(event) => onHandleContextMenu(event, lineId)}
                  onMouseEnter={() => onDragEnter(lineId)}
                >
                  <div className={styles.selectorCell} onMouseDown={(event) => onDragStart(event, lineId)}>
                    <Checkbox checked={selected} />
                  </div>
                  <div className={styles.logContent}>
                    <span className={styles.timestampCell}>{line.logTime || '-'}</span>
                    <span className={styles.levelCell}>
                      <Tag color={levelTagColor(line.level)} className={styles.levelTag}>
                      {(line.level || 'INFO').toUpperCase()}
                      </Tag>
                    </span>
                    <Tooltip title={line.traceId ? `Trace ID: ${line.traceId}` : '无 Trace ID'} mouseEnterDelay={0.25}>
                      <span className={styles.traceCell}>
                        {line.traceId ? `trace:${line.traceId.slice(0, 12)}` : 'trace:-'}
                      </span>
                    </Tooltip>
                    <Tooltip
                      title={renderRuntimeLogSourceTooltip(line)}
                      mouseEnterDelay={0.25}
                      placement="topLeft"
                      overlayClassName={styles.sourceTooltip}
                    >
                      <span
                        className={`${styles.messageCell} ${
                          (line.level || '').toUpperCase() === 'ERROR' ? styles.messageError : ''
                        }`}
                      >
                        <span className={styles.messageMain}>{content.title}</span>
                        {content.detail && <span className={styles.messageDetail}>{content.detail}</span>}
                      </span>
                    </Tooltip>
                  </div>
                  <div
                    className={styles.actionsPanel}
                    onMouseEnter={() => onHoverActionChange(true)}
                    onMouseLeave={() => onHoverActionChange(false)}
                  >
                    <Tooltip title={singleCopied ? '已复制' : '复制单行'}>
                      <Button
                        type="text"
                        size="small"
                        icon={singleCopied ? <CheckOutlined /> : <CopyOutlined />}
                        onClick={() => void onCopySingle(line)}
                      />
                    </Tooltip>
                    <Tooltip title={contextCopied ? '已复制' : `复制上下文 ±${copyN} 行`}>
                      <Button
                        type="text"
                        size="small"
                        icon={contextCopied ? <CheckOutlined /> : <VerticalAlignBottomOutlined />}
                        onClick={() => void onCopyContext(line)}
                      />
                    </Tooltip>
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>

      {!autoScroll && mode === 'realtime' && (
        <div className={styles.jumpBottomBtn}>
          <Button
            type="primary"
            size="small"
            onClick={() => {
              scrollToBottom();
              onFollowBottom();
            }}
          >
            跟随到底部
          </Button>
        </div>
      )}

      {selectedCount > 0 && (
        <div className={styles.floatingBar}>
          <Typography.Text style={{ color: '#cbd5e1' }}>
            已选择 <Typography.Text style={{ color: '#f8fafc' }}>{selectedCount}</Typography.Text> 条
          </Typography.Text>
          <Button
            type="primary"
            icon={isBulkCopied ? <CheckOutlined /> : <CopyOutlined />}
            onClick={() => void onCopySelected()}
          >
            {isBulkCopied ? '已复制' : '合并复制'}
          </Button>
          <Button onClick={onClearSelection}>取消</Button>
        </div>
      )}

      {contextMenu.visible && contextLine && (
        <div
          className={styles.contextMenu}
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onClick={(event) => event.stopPropagation()}
        >
          <div className={styles.contextMenuHeader}>快捷操作</div>
          <button
            type="button"
            className={styles.contextMenuItem}
            onClick={() => {
              void onCopySingle(contextLine);
              onCloseContextMenu();
            }}
          >
            复制当前行
          </button>
          <button
            type="button"
            className={styles.contextMenuItem}
            onClick={() => {
              void onCopyContext(contextLine);
              onCloseContextMenu();
            }}
          >
            复制上下文 ±{copyN} 行
          </button>
        </div>
      )}
    </div>
  );
}
