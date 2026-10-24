import dayjs from 'dayjs';

/**
 * 格式化日期时间
 * @param date 日期字符串或 Date 对象
 * @param format 格式，默认 'YYYY-MM-DD HH:mm:ss'
 * @returns 格式化后的日期字符串
 */
export function formatDate(
  date: string | Date | null | undefined,
  format: string = 'YYYY-MM-DD HH:mm:ss'
): string {
  if (!date) return '-';
  return dayjs(date).format(format);
}
