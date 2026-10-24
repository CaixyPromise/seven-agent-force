import { useRef, useEffect } from 'react';

/**
 * 自定义 Hook，用于检查组件是否仍然挂载
 * 解决 React Query 在组件卸载后仍在执行回调的问题
 */
export function useIsMounted() {
  const isMountedRef = useRef(true);

  useEffect(() => {
    isMountedRef.current = true;

    return () => {
      isMountedRef.current = false;
    };
  }, []);

  return isMountedRef;
}
