'use client';

import React, {
  useState,
  useCallback,
  useEffect,
  useRef,
  useMemo,
  ReactNode,
} from 'react';
import { getDictBatch } from '@/api/dictClientController';
import type { DictItemVO } from '@/types/dictClient';
import { useAuthStore } from '@/store/auth';
import { buildConfigCacheIdentity } from '@/hooks/config/cacheIdentity';
import {
  DictClientContext,
  type DictClientContextValue,
} from './useDictClientContext';

/**
 * 字典客户端Provider
 * 提供字典缓存和批量请求合并功能
 */
export const DictClientProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const user = useAuthStore(state => state.user);
  const cacheIdentity = useMemo(() => buildConfigCacheIdentity(user), [user]);
  const cacheIdentityRef = useRef(cacheIdentity);
  // 使用 ref 存储缓存，避免函数引用变化导致无限循环
  const cacheRef = useRef<Map<string, DictItemVO[]>>(new Map());
  // 使用 ref 存储加载状态
  const loadingRef = useRef<Set<string>>(new Set());
  // 每次全量刷新都会推进代数，使之前在途请求的结果失效。
  const generationRef = useRef(0);
  // 版本号，用于触发订阅者重新渲染
  const [version, setVersion] = useState(0);

  // 待请求队列
  const pendingCodesRef = useRef<Set<string>>(new Set());
  // 批量请求定时器
  const batchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // 请求中标记（防止重复请求）
  const isFetchingRef = useRef(false);
  // 初始为 true，确保子组件首次 effect 早于 Provider effect 时也能正常排队。
  const mountedRef = useRef(true);

  // 批量请求间隔（毫秒）
  const BATCH_DELAY = 50;

  /**
   * 触发版本更新
   */
  const triggerUpdate = useCallback(() => {
    setVersion(v => v + 1);
  }, []);

  /**
   * 执行批量请求
   */
  const executeBatch = useCallback(async () => {
    if (!mountedRef.current || isFetchingRef.current) return;

    const codes = Array.from(pendingCodesRef.current);
    if (codes.length === 0) return;
    const requestGeneration = generationRef.current;
    const requestIdentity = cacheIdentityRef.current;

    // 标记请求中
    isFetchingRef.current = true;
    // 清空待请求队列
    pendingCodesRef.current.clear();
    batchTimerRef.current = null;

    // 标记正在加载
    codes.forEach(code => loadingRef.current.add(code));

    try {
      const response = await getDictBatch(codes);
      if (!mountedRef.current || generationRef.current !== requestGeneration || cacheIdentityRef.current !== requestIdentity) return;

      // 更新缓存
      Object.entries(response.record || {}).forEach(([code, items]) => {
        cacheRef.current.set(`${requestIdentity}|${code}`, items);
      });
      // 对于missing的字典，设置为空数组（避免重复请求）
      (response.missing || []).forEach(code => {
        cacheRef.current.set(`${requestIdentity}|${code}`, []);
      });

      // 对于既不在 record 也不在 missing 中的 code，标记为空数组（避免无限循环请求）
      codes.forEach(code => {
        const scopedCode = `${requestIdentity}|${code}`;
        if (!cacheRef.current.has(scopedCode)) {
          cacheRef.current.set(scopedCode, []);
        }
      });

    } catch (error) {
      if (!mountedRef.current || generationRef.current !== requestGeneration || cacheIdentityRef.current !== requestIdentity) return;
      console.error('批量获取字典失败:', error);
      // 请求失败时也要标记，避免无限重试
      codes.forEach(code => {
        const scopedCode = `${requestIdentity}|${code}`;
        if (!cacheRef.current.has(scopedCode)) {
          cacheRef.current.set(scopedCode, []);
        }
      });
    } finally {
      // 移除加载状态
      codes.forEach(code => loadingRef.current.delete(code));
      isFetchingRef.current = false;
      if (!mountedRef.current) {
        pendingCodesRef.current.clear();
      } else {
        triggerUpdate();

        // 请求期间新加入的 code 留给下一批，避免并发窗口导致永久滞留。
        if (pendingCodesRef.current.size > 0) {
          if (batchTimerRef.current) {
            clearTimeout(batchTimerRef.current);
          }
          batchTimerRef.current = setTimeout(() => {
            batchTimerRef.current = null;
            void executeBatch();
          }, BATCH_DELAY);
        }
      }
    }
  }, [triggerUpdate]);

  /**
   * 调度批量请求
   */
  const scheduleBatch = useCallback(() => {
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
    }
    batchTimerRef.current = setTimeout(() => {
      batchTimerRef.current = null;
      void executeBatch();
    }, BATCH_DELAY);
  }, [executeBatch]);

  /**
   * 获取字典项列表
   * 只读取当前缓存快照，不在 render 阶段调度请求。
   * 注意：此函数引用稳定，不依赖 state
   */
  const getDict = useCallback((code: string): DictItemVO[] | null => {
    return cacheRef.current.get(`${cacheIdentityRef.current}|${code}`) ?? null;
  }, []);

  /**
   * 在组件提交后确保字典已进入请求队列。
   */
  const ensureDict = useCallback((code: string): void => {
    if (cacheRef.current.has(`${cacheIdentityRef.current}|${code}`)) return;

    if (!loadingRef.current.has(code) && !pendingCodesRef.current.has(code)) {
      pendingCodesRef.current.add(code);
      triggerUpdate();
      scheduleBatch();
    }
  }, [scheduleBatch, triggerUpdate]);

  /**
   * 刷新指定字典
   */
  const refreshDict = useCallback(async (code: string): Promise<void> => {
    // 从缓存中移除
    cacheRef.current.delete(`${cacheIdentityRef.current}|${code}`);

    // 加入待请求队列
    pendingCodesRef.current.add(code);

    // 立即执行
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }

    await executeBatch();
  }, [executeBatch]);

  /**
   * 刷新所有字典缓存
   */
  const refreshAll = useCallback(() => {
    generationRef.current += 1;
    cacheRef.current.clear();
    pendingCodesRef.current.clear();
    loadingRef.current.clear();
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }
    triggerUpdate();
  }, [triggerUpdate]);

  /**
   * 检查字典是否正在加载
   * 注意：此函数引用稳定，不依赖 state
   */
  const isLoading = useCallback((code: string): boolean => {
    return loadingRef.current.has(code) || pendingCodesRef.current.has(code);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    if (pendingCodesRef.current.size > 0 && !isFetchingRef.current) {
      scheduleBatch();
    }

    return () => {
      mountedRef.current = false;
      if (batchTimerRef.current) {
        clearTimeout(batchTimerRef.current);
        batchTimerRef.current = null;
      }
    };
  }, [scheduleBatch]);

  useEffect(() => {
    if (cacheIdentityRef.current === cacheIdentity) return;
    cacheIdentityRef.current = cacheIdentity;
    generationRef.current += 1;
    cacheRef.current.clear();
    pendingCodesRef.current.clear();
    loadingRef.current.clear();
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }
    triggerUpdate();
  }, [cacheIdentity, triggerUpdate]);

  // 使用稳定的 value 对象
  const value: DictClientContextValue = {
    getDict,
    ensureDict,
    refreshDict,
    refreshAll,
    isLoading,
    version,
  };

  return (
    <DictClientContext.Provider value={value}>
      {children}
    </DictClientContext.Provider>
  );
};
