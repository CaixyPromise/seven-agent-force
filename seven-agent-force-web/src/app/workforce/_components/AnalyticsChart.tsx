import { Empty, Result, Skeleton } from 'antd';
import { useEffect, useRef } from 'react';
import type { ReactNode } from 'react';
import styles from './AnalyticsChart.module.css';
import { echarts } from './AnalyticsChartConfig';

export function AnalyticsChart({ option, ariaLabel, onSelect, className }: { option: echarts.EChartsCoreOption; ariaLabel: string; onSelect?: (params: unknown) => void; className?: string }) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!root.current) return;
    const chart = echarts.init(root.current, undefined, { renderer: 'canvas' });
    chart.setOption({ ...option, animation: !window.matchMedia('(prefers-reduced-motion: reduce)').matches });
    if (onSelect) chart.on('click', onSelect);
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(root.current);
    return () => { observer.disconnect(); chart.dispose(); };
  }, [onSelect, option]);
  return <div ref={root} className={className ? `${styles.chart} ${className}` : styles.chart} role="img" aria-label={ariaLabel} tabIndex={0} />;
}

export function ChartState({ loading, error, empty, onRetry, children }: { loading: boolean; error: boolean; empty: boolean; onRetry: () => void; children: ReactNode }) {
  if (loading) return <Skeleton active paragraph={{ rows: 5 }} />;
  if (error) return <Result className={styles.state} status="warning" title="分析数据暂时无法加载" extra={<button type="button" onClick={onRetry}>重试</button>} />;
  if (empty) return <Empty className={styles.state} image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前范围暂无数据" />;
  return children;
}
