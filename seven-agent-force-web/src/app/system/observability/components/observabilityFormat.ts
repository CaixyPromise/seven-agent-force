'use client';

export function formatDateTime(value?: string) {
  if (!value) {
    return '刚刚';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

export function toFiniteNumber(value?: number | string | null) {
  const numeric = typeof value === 'number' ? value : Number(value ?? 0);
  return Number.isFinite(numeric) ? numeric : 0;
}

export function formatNumber(value: number, fractionDigits = 0) {
  return new Intl.NumberFormat('zh-CN', {
    maximumFractionDigits: fractionDigits,
    minimumFractionDigits: fractionDigits,
  }).format(value);
}

export function formatPercent(value: number, fractionDigits = 1) {
  return `${formatNumber(value, fractionDigits)}%`;
}

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const;

export function resolveByteUnit(value?: number) {
  const safeValue = toFiniteNumber(value);
  let unitIndex = 0;
  let displayValue = safeValue;
  while (displayValue >= 1024 && unitIndex < BYTE_UNITS.length - 1) {
    displayValue /= 1024;
    unitIndex += 1;
  }
  return BYTE_UNITS[unitIndex];
}

export function formatBytesInUnit(
  value: number | undefined,
  unit = resolveByteUnit(value),
): string {
  const safeValue = toFiniteNumber(value);
  const targetIndex = BYTE_UNITS.indexOf(unit);
  if (targetIndex < 0) {
    return formatBytes(value);
  }
  const divisor = 1024 ** targetIndex;
  const displayValue = divisor <= 0 ? 0 : safeValue / divisor;
  const fractionDigits = displayValue >= 100 ? 0 : displayValue >= 10 ? 1 : 2;
  return `${formatNumber(displayValue, fractionDigits)} ${unit}`;
}

export function formatBytes(value?: number): string {
  const safeValue = toFiniteNumber(value);
  if (!Number.isFinite(safeValue) || safeValue <= 0) {
    return '0 B';
  }
  return formatBytesInUnit(safeValue, resolveByteUnit(safeValue));
}

export function formatBytesPerSecond(value?: number) {
  return `${formatBytes(value)}/s`;
}

export function formatDurationMs(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return '0 ms';
  }
  if (value >= 1000) {
    return `${formatNumber(value / 1000, value >= 10_000 ? 1 : 2)} s`;
  }
  return `${formatNumber(value, value >= 100 ? 0 : 1)} ms`;
}
