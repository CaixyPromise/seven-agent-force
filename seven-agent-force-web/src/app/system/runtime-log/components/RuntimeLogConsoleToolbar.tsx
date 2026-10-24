'use client';

import React from 'react';
import { Alert, Button, Card, Checkbox, Input, InputNumber, Segmented, Select, Space, Tag, Typography } from 'antd';
import {
  AlignLeftOutlined,
  ClearOutlined,
  CodeOutlined,
  FieldTimeOutlined,
  LinkOutlined,
  PauseCircleOutlined,
  PlayCircleOutlined,
  ReloadOutlined,
  SearchOutlined,
} from '@ant-design/icons';
import styles from './RuntimeLogConsole.module.css';
import type { ConsoleFilterState, ConsoleMode, LevelFilter, StreamState } from './runtimeLogConsole.types';

interface RuntimeLogConsoleToolbarProps {
  canStream: boolean;
  mode: ConsoleMode;
  isLive: boolean;
  streamState: StreamState;
  historyLoading: boolean;
  historyError?: string;
  hasInvalidRegex: boolean;
  filter: ConsoleFilterState;
  onModeChange: (value: ConsoleMode) => void;
  onFilterChange: (key: keyof ConsoleFilterState, value: ConsoleFilterState[keyof ConsoleFilterState]) => void;
  onStartLive: () => void;
  onPauseLive: () => void;
  onRefreshHistory: () => void;
  onClearPanel: () => void;
  clampCopyN: (value?: number | null) => number;
}

function streamStatusText(streamState: StreamState, isLive: boolean) {
  if (!isLive) {
    return '已暂停';
  }
  if (streamState.connecting) {
    return '连接中';
  }
  if (streamState.active) {
    return '实时中';
  }
  return '待连接';
}

export function RuntimeLogConsoleToolbar({
  canStream,
  mode,
  isLive,
  streamState,
  historyLoading,
  historyError,
  hasInvalidRegex,
  filter,
  onModeChange,
  onFilterChange,
  onStartLive,
  onPauseLive,
  onRefreshHistory,
  onClearPanel,
  clampCopyN,
}: RuntimeLogConsoleToolbarProps) {
  return (
    <>
      <Card className={styles.toolbarCard} styles={{ body: { padding: 16 } }}>
        {!canStream && mode === 'realtime' && (
          <Alert
            showIcon
            type="warning"
            message="当前账号没有实时订阅权限（admin:runtime-log:stream），可切换到历史回溯模式。"
          />
        )}
        {streamState.lastError && mode === 'realtime' && (
          <Alert showIcon type="error" message={`实时连接异常：${streamState.lastError}`} />
        )}
        {historyError && mode === 'history' && (
          <Alert showIcon type="error" message={`历史查询失败：${historyError}`} />
        )}
        {hasInvalidRegex && (
          <Alert showIcon type="warning" message="检测到非法正则，已自动忽略该条件，页面不会中断。" />
        )}

        <div className={styles.toolbarRow}>
          <span className={styles.toolbarLabel}>级别</span>
          <Select<LevelFilter>
            className={styles.searchSelect}
            value={filter.level}
            onChange={(value) => onFilterChange('level', value)}
            options={[
              { value: 'ALL', label: '全部' },
              { value: 'INFO', label: 'INFO' },
              { value: 'WARN', label: 'WARN' },
              { value: 'ERROR', label: 'ERROR' },
              { value: 'DEBUG', label: 'DEBUG' },
              { value: 'TRACE', label: 'TRACE' },
            ]}
          />
          <Input
            className={styles.searchInput}
            allowClear
            prefix={<SearchOutlined />}
            placeholder="内容 (Content)"
            value={filter.contentKw}
            onChange={(event) => onFilterChange('contentKw', event.target.value)}
          />
          <Input
            className={`${styles.searchInputSmall} ${styles.searchInput}`}
            allowClear
            prefix={<CodeOutlined />}
            placeholder="类名 (Logger)"
            value={filter.loggerKw}
            onChange={(event) => onFilterChange('loggerKw', event.target.value)}
          />
          <Input
            className={`${styles.searchInputSmall} ${styles.searchInput}`}
            allowClear
            prefix={<AlignLeftOutlined />}
            placeholder="线程 (Thread)"
            value={filter.threadKw}
            onChange={(event) => onFilterChange('threadKw', event.target.value)}
          />
          <Input
            className={`${styles.searchInputSmall} ${styles.searchInput}`}
            allowClear
            prefix={<LinkOutlined />}
            placeholder="Trace ID"
            value={filter.traceId}
            onChange={(event) => onFilterChange('traceId', event.target.value)}
          />
          <Checkbox
            checked={filter.useRegex}
            onChange={(event) => onFilterChange('useRegex', event.target.checked)}
            style={{ color: '#334155' }}
          >
            Regex
          </Checkbox>
          <Space size={10} className={styles.modeSwitchShell} style={{ marginLeft: 'auto' }}>
            <Segmented
              value={mode}
              onChange={(value) => {
                if (value === 'realtime' || value === 'history') {
                  onModeChange(value);
                }
              }}
              options={[
                { label: '实时追踪', value: 'realtime' },
                { label: '历史回溯', value: 'history' },
              ]}
            />
            <Tag color={streamState.active ? 'success' : streamState.connecting ? 'processing' : 'default'}>
              {streamStatusText(streamState, isLive)}
            </Tag>
          </Space>
        </div>

        <div className={styles.toolbarRow}>
          <span className={styles.toolbarLabel}>上下文复制 ±N</span>
          <InputNumber
            className={styles.copyInput}
            min={1}
            max={100}
            value={filter.copyN}
            onChange={(value) => onFilterChange('copyN', clampCopyN(value))}
          />
          <Typography.Text type="secondary">最大 100</Typography.Text>

          {mode === 'history' && (
            <Space size={8}>
              <FieldTimeOutlined style={{ color: '#94a3b8' }} />
              <input
                className={styles.timeInput}
                type="datetime-local"
                value={filter.startTime}
                onChange={(event) => onFilterChange('startTime', event.target.value)}
              />
              <span style={{ color: '#64748b' }}>至</span>
              <input
                className={styles.timeInput}
                type="datetime-local"
                value={filter.endTime}
                onChange={(event) => onFilterChange('endTime', event.target.value)}
              />
            </Space>
          )}

          <Space size={8} style={{ marginLeft: 'auto' }}>
            {mode === 'history' ? (
              <Button icon={<ReloadOutlined />} onClick={onRefreshHistory} loading={historyLoading}>
                刷新
              </Button>
            ) : (
              <>
                <Button type="primary" icon={<PlayCircleOutlined />} disabled={!canStream} onClick={onStartLive}>
                  开始
                </Button>
                <Button icon={<PauseCircleOutlined />} onClick={onPauseLive}>
                  暂停
                </Button>
              </>
            )}
            <Button icon={<ClearOutlined />} onClick={onClearPanel}>
              清空
            </Button>
          </Space>
        </div>
      </Card>
    </>
  );
}
