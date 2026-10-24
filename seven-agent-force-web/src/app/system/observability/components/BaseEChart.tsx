'use client';

import { useEffect, useRef } from 'react';
import * as echarts from 'echarts/core';
import type { EChartsOption, SetOptionOpts } from 'echarts';
import { LineChart, ScatterChart } from 'echarts/charts';
import { GridComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([LineChart, ScatterChart, GridComponent, TooltipComponent, CanvasRenderer]);

interface BaseEChartProps {
  option: EChartsOption;
  height: number;
  settings?: SetOptionOpts;
}

export function BaseEChart({ option, height, settings }: BaseEChartProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<ReturnType<typeof echarts.init> | null>(null);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }

    const chart = echarts.init(node, undefined, {
      renderer: 'canvas',
    });
    chartRef.current = chart;

    const resizeObserver = new ResizeObserver(() => {
      chart.resize();
    });
    resizeObserver.observe(node);

    return () => {
      resizeObserver.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    const chart = chartRef.current;
    if (!chart) {
      return;
    }
    chart.setOption(option, settings ?? { notMerge: true });
    chart.resize();
  }, [option, settings]);

  return <div ref={containerRef} style={{ width: '100%', height }} />;
}
