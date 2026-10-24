import type { RuntimeLogLine } from '@/lib/http/types';

export type ConsoleMode = 'realtime' | 'history';
export type LevelFilter = 'ALL' | 'INFO' | 'WARN' | 'ERROR' | 'DEBUG' | 'TRACE';

export interface ConsoleFilterState {
  level: LevelFilter;
  contentKw: string;
  loggerKw: string;
  threadKw: string;
  traceId: string;
  useRegex: boolean;
  copyN: number;
  startTime: string;
  endTime: string;
}

export interface StreamState {
  active: boolean;
  connecting: boolean;
  lastError?: string;
  lastHeartbeatAt?: number;
}

export interface ContextMenuState {
  visible: boolean;
  x: number;
  y: number;
  lineId?: string;
}

export interface LocalFilterResult {
  logs: RuntimeLogLine[];
  hasInvalidRegex: boolean;
}
