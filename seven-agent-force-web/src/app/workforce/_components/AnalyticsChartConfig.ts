import { BarChart, LineChart, PieChart } from 'echarts/charts';
import { GridComponent, LegendComponent, TooltipComponent } from 'echarts/components';
import * as echarts from 'echarts/core';
import { CanvasRenderer } from 'echarts/renderers';

echarts.use([BarChart, LineChart, PieChart, GridComponent, LegendComponent, TooltipComponent, CanvasRenderer]);

export { echarts };

export function chartGrid() { return { left: 8, right: 16, top: 38, bottom: 8, containLabel: true }; }
export function chartLegend() { return { top: 0, right: 0, itemWidth: 9, itemHeight: 9, textStyle: { color: '#667085', fontSize: 12 } }; }
export function categoryAxis(values: string[], boundaryGap = true) { return { type: 'category', data: values, boundaryGap, axisTick: { show: false }, axisLine: { lineStyle: { color: '#e7edf5' } }, axisLabel: { color: '#8792a5', hideOverlap: true, fontSize: 11 } }; }
export function valueAxis(formatter?: string) { return { type: 'value', min: 0, minInterval: 1, axisLabel: { color: '#8792a5', formatter }, splitLine: { lineStyle: { color: '#edf1f6', type: 'dashed' } } }; }
export function tooltip() { return { trigger: 'axis', axisPointer: { type: 'shadow' }, backgroundColor: 'rgba(15,23,42,.94)', borderWidth: 0, textStyle: { color: '#fff' } }; }
export function localDay(value: string) { return new Date(value).toLocaleDateString([], { month: '2-digit', day: '2-digit' }); }
