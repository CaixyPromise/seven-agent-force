'use client';

import { useEffect, useRef } from 'react';
import * as echarts from 'echarts/core';
import type { EChartsOption } from 'echarts';
import { BarChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([BarChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

interface DockerWorkbenchChartProps {
  option: EChartsOption;
  height?: number;
}

export function DockerWorkbenchChart({ option, height = 260 }: DockerWorkbenchChartProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const chartRef = useRef<ReturnType<typeof echarts.init> | null>(null);

  useEffect(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }
    const chart = echarts.init(node, undefined, { renderer: 'canvas' });
    chartRef.current = chart;
    return () => {
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    chartRef.current?.setOption(option, true);
  }, [option]);

  useEffect(() => {
    const handleResize = () => chartRef.current?.resize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  return <div ref={containerRef} style={{ height, width: '100%' }} />;
}
