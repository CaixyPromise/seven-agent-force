'use client';

/* eslint-disable react-refresh/only-export-components */

import React from 'react';
import { Empty, Tag } from 'antd';
import {
  CheckCircleFilled,
  ClockCircleFilled,
  ExclamationCircleFilled,
  PauseCircleFilled,
  StopFilled,
  SyncOutlined,
} from '@ant-design/icons';
import { formatContainerStateLabel } from './dockerFormat';

function cx(...classes: Array<string | false | null | undefined>) {
  return classes.filter(Boolean).join(' ');
}

export function formatBytes(size?: number | string | null) {
  if (size === null || size === undefined || size === '') {
    return '-';
  }
  const numericSize = Number(size);
  if (!Number.isFinite(numericSize)) {
    return '-';
  }
  if (numericSize < 1024) {
    return `${numericSize} B`;
  }
  if (numericSize < 1024 * 1024) {
    return `${(numericSize / 1024).toFixed(1)} KB`;
  }
  if (numericSize < 1024 * 1024 * 1024) {
    return `${(numericSize / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${(numericSize / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

export function formatCompactNumber(value?: number) {
  if (value === null || value === undefined) {
    return '-';
  }
  return new Intl.NumberFormat('zh-CN', {
    notation: 'compact',
    maximumFractionDigits: value >= 1000 ? 1 : 0,
  }).format(value);
}

interface DockerMetricItem {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
}

export function DockerMetricCards({ items, columns = 4 }: { items: DockerMetricItem[]; columns?: 2 | 3 | 4 }) {
  const gridClass =
    columns === 2
      ? 'md:grid-cols-2'
      : columns === 3
        ? 'md:grid-cols-2 xl:grid-cols-3'
        : 'md:grid-cols-2 xl:grid-cols-4';

  return (
    <div className={cx('grid gap-3', gridClass)}>
      {items.map((item) => (
        <div
          key={item.label}
          className="rounded-[24px] border border-sky-100 bg-white/85 px-5 py-4 shadow-[0_18px_50px_rgba(14,116,255,0.08)] backdrop-blur"
        >
          <div className="text-xs font-semibold uppercase tracking-[0.24em] text-sky-500">{item.label}</div>
          <div className="mt-3 text-3xl font-semibold tracking-tight text-slate-900">{item.value}</div>
          {item.hint ? <div className="mt-2 text-sm text-slate-500">{item.hint}</div> : null}
        </div>
      ))}
    </div>
  );
}

export function DockerPageHero({
  eyebrow,
  title,
  description,
  actions,
  stats,
}: {
  eyebrow: string;
  title: string;
  description: string;
  actions?: React.ReactNode;
  stats?: DockerMetricItem[];
}) {
  return (
    <section className="relative overflow-hidden rounded-[32px] border border-sky-100 bg-[radial-gradient(circle_at_top_left,_rgba(59,130,246,0.22),_transparent_38%),radial-gradient(circle_at_top_right,_rgba(6,182,212,0.15),_transparent_32%),linear-gradient(180deg,_#f8fbff_0%,_#eef6ff_52%,_#f8fbff_100%)] px-6 py-7 shadow-[0_28px_80px_rgba(14,116,255,0.10)] sm:px-8">
      <div className="absolute inset-y-0 right-0 hidden w-1/3 bg-[linear-gradient(135deg,rgba(255,255,255,0),rgba(191,219,254,0.4))] lg:block" />
      <div className="relative space-y-6">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
          <div className="max-w-3xl space-y-3">
            <div className="text-[11px] font-semibold uppercase tracking-[0.34em] text-sky-500">{eyebrow}</div>
            <div className="space-y-2">
              <h1 className="text-[34px] font-semibold tracking-[-0.04em] text-slate-950">{title}</h1>
              <p className="max-w-3xl text-[15px] leading-7 text-slate-600">{description}</p>
            </div>
          </div>
          {actions ? <div className="flex flex-wrap items-center gap-3">{actions}</div> : null}
        </div>
        {stats?.length ? <DockerMetricCards items={stats} /> : null}
      </div>
    </section>
  );
}

export function DockerSurfaceCard({
  title,
  description,
  extra,
  children,
  compact = false,
  className,
}: {
  title?: React.ReactNode;
  description?: React.ReactNode;
  extra?: React.ReactNode;
  children: React.ReactNode;
  compact?: boolean;
  className?: string;
}) {
  return (
    <section
      className={cx(
        'rounded-[30px] border border-sky-100 bg-white/95 shadow-[0_20px_70px_rgba(15,23,42,0.06)] backdrop-blur',
        className,
      )}
    >
      {title || description || extra ? (
        <div
          className={cx(
            'flex flex-col gap-4 border-b border-sky-100/80',
            compact ? 'px-5 py-4' : 'px-6 py-5',
            extra ? 'lg:flex-row lg:items-start lg:justify-between' : '',
          )}
        >
          <div className="space-y-1.5">
            {title ? <div className="text-lg font-semibold tracking-[-0.02em] text-slate-950">{title}</div> : null}
            {description ? <div className="text-sm leading-6 text-slate-500">{description}</div> : null}
          </div>
          {extra ? <div className="flex flex-wrap items-center gap-3">{extra}</div> : null}
        </div>
      ) : null}
      <div className={compact ? 'p-5' : 'p-6'}>{children}</div>
    </section>
  );
}

export function DockerCodeBlock({
  title,
  description,
  value,
  className,
}: {
  title?: React.ReactNode;
  description?: React.ReactNode;
  value: string;
  className?: string;
}) {
  return (
    <div className={cx('overflow-hidden rounded-[26px] border border-slate-800 bg-[#07111f] shadow-[0_18px_50px_rgba(2,6,23,0.24)]', className)}>
      {title || description ? (
        <div className="border-b border-slate-800/90 bg-[linear-gradient(90deg,rgba(14,165,233,0.18),rgba(8,47,73,0.38))] px-5 py-4">
          {title ? <div className="text-sm font-semibold text-slate-50">{title}</div> : null}
          {description ? <div className="mt-1 text-xs leading-6 text-slate-300">{description}</div> : null}
        </div>
      ) : null}
      <pre className="max-h-[68vh] overflow-auto px-5 py-4 text-xs leading-6 text-slate-100">{value}</pre>
    </div>
  );
}

export function DockerStateTag({
  state,
  label,
}: {
  state?: string | null;
  label?: React.ReactNode;
}) {
  const normalized = (state || '').toLowerCase();
  const text = label ?? formatContainerStateLabel(state || undefined);

  if (normalized === 'running' || normalized === 'active') {
    return (
      <Tag icon={<CheckCircleFilled />} color="processing">
        {text}
      </Tag>
    );
  }
  if (normalized === 'restarting') {
    return (
      <Tag icon={<SyncOutlined />} color="processing">
        {text}
      </Tag>
    );
  }
  if (normalized === 'paused') {
    return (
      <Tag icon={<PauseCircleFilled />} color="warning">
        {text}
      </Tag>
    );
  }
  if (normalized === 'created' || normalized === 'pending') {
    return (
      <Tag icon={<ClockCircleFilled />} color="default">
        {text}
      </Tag>
    );
  }
  if (normalized === 'failed' || normalized === 'error') {
    return (
      <Tag icon={<ExclamationCircleFilled />} color="error">
        {text}
      </Tag>
    );
  }
  if (normalized === 'exited' || normalized === 'inactive' || normalized === 'stopped') {
    return (
      <Tag icon={<StopFilled />} color="default">
        {text}
      </Tag>
    );
  }

  return <Tag color="default">{text}</Tag>;
}

export function DockerEmptyState({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="rounded-[28px] border border-dashed border-sky-200 bg-[linear-gradient(180deg,#f8fbff_0%,#eff6ff_100%)] p-8">
      <Empty
        description={
          <div className="space-y-2">
            <div className="text-base font-semibold text-slate-900">{title}</div>
            <div className="mx-auto max-w-md text-sm leading-6 text-slate-500">{description}</div>
          </div>
        }
      >
        {action}
      </Empty>
    </div>
  );
}
