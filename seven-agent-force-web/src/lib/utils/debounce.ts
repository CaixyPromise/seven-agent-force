/**
 * 防抖函数
 * @param func 要执行的函数
 * @param delay 延迟时间（毫秒）
 * @returns 防抖后的函数
 */
export type DebouncedFunction<Args extends unknown[]> = ((...args: Args) => void) & {
  cancel: () => void;
};

export function debounce<Args extends unknown[]>(
  func: (...args: Args) => unknown,
  delay: number,
): DebouncedFunction<Args> {
  let timeoutId: ReturnType<typeof setTimeout> | null = null;

  const debounced = (...args: Args) => {
    if (timeoutId) {
      clearTimeout(timeoutId);
    }

    timeoutId = setTimeout(() => {
      timeoutId = null;
      func(...args);
    }, delay);
  };

  debounced.cancel = () => {
    if (timeoutId) {
      clearTimeout(timeoutId);
      timeoutId = null;
    }
  };

  return debounced;
}
