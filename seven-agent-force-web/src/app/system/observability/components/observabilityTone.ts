'use client';

export function resolveHealthTone(status: string) {
  switch (status) {
    case 'critical':
    case 'down':
    case 'out_of_service':
      return {
        dot: '#dc2626',
        glow: 'rgba(220, 38, 38, 0.2)',
        label: '需要立即处理',
      };
    case 'warning':
      return {
        dot: '#d97706',
        glow: 'rgba(217, 119, 6, 0.2)',
        label: '需要关注',
      };
    default:
      return {
        dot: '#0f766e',
        glow: 'rgba(15, 118, 110, 0.2)',
        label: '运行平稳',
      };
  }
}
