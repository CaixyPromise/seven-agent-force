'use client';

import React, {
  useState,
  useCallback,
  useEffect,
  useRef,
  useMemo,
  ReactNode,
} from 'react';
import { getConfigBatch } from '@/api/configClientController';
import type { ConfigValueDTO } from '@/types/configClient';
import { useAuthStore } from '@/store/auth';
import { buildConfigCacheIdentity } from './cacheIdentity';
import {
  ConfigClientContext,
  type ConfigClientContextValue,
} from './useConfigClientContext';

/**
 * 配置客户端Provider
 * 提供配置缓存和批量请求合并功能
 */
export const ConfigClientProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const user = useAuthStore(state => state.user);
  const cacheIdentity = useMemo(() => buildConfigCacheIdentity(user), [user]);
  const cacheIdentityRef = useRef(cacheIdentity);
  // 使用 ref 存储缓存，避免函数引用变化导致无限循环
  const cacheRef = useRef<Map<string, ConfigValueDTO>>(new Map());
  // 使用 ref 存储加载状态
  const loadingRef = useRef<Set<string>>(new Set());
  // 每次全量刷新都会推进代数，使之前在途请求的结果失效。
  const generationRef = useRef(0);
  // 版本号，用于触发订阅者重新渲染
  const [version, setVersion] = useState(0);

  // 待请求队列
  const pendingKeysRef = useRef<Set<string>>(new Set());
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

    const keys = Array.from(pendingKeysRef.current);
    if (keys.length === 0) return;
    const requestGeneration = generationRef.current;
    const requestIdentity = cacheIdentityRef.current;

    // 标记请求中
    isFetchingRef.current = true;
    // 清空待请求队列
    pendingKeysRef.current.clear();
    batchTimerRef.current = null;

    // 标记正在加载
    keys.forEach(key => loadingRef.current.add(key));

    try {
      const results = await getConfigBatch(keys);
      if (!mountedRef.current || generationRef.current !== requestGeneration || cacheIdentityRef.current !== requestIdentity) return;

      // 更新缓存（包含返回的数据）
      Object.entries(results).forEach(([key, value]) => {
        cacheRef.current.set(`${requestIdentity}|${key}`, value);
      });

      // 对于没有返回的 key，标记为空（避免无限循环请求）
      keys.forEach(key => {
        if (!results[key]) {
          // 标记为查询过但无结果（使用特殊的空对象）
          cacheRef.current.set(`${requestIdentity}|${key}`, { key, type: 'STRING', value: '', schemaVersion: 1, version: '0', _notFound: true });
        }
      });

    } catch (error) {
      if (!mountedRef.current || generationRef.current !== requestGeneration || cacheIdentityRef.current !== requestIdentity) return;
      console.error('批量获取配置失败:', error);
      // 请求失败时也要标记，避免无限重试
      keys.forEach(key => {
        const scopedKey = `${requestIdentity}|${key}`;
        if (!cacheRef.current.has(scopedKey)) {
          cacheRef.current.set(scopedKey, { key, type: 'STRING', value: '', schemaVersion: 1, version: '0', _error: true });
        }
      });
    } finally {
      // 移除加载状态
      keys.forEach(key => loadingRef.current.delete(key));
      isFetchingRef.current = false;
      if (!mountedRef.current) {
        pendingKeysRef.current.clear();
      } else {
        triggerUpdate();

        // 请求期间新加入的 key 留给下一批，避免并发窗口导致永久滞留。
        if (pendingKeysRef.current.size > 0) {
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
   * 获取配置值
   * 只读取当前缓存快照，不在 render 阶段调度请求。
   * 注意：此函数引用稳定，不依赖 state
   */
  const getConfig = useCallback((key: string): ConfigValueDTO | null => {
    return cacheRef.current.get(`${cacheIdentityRef.current}|${key}`) ?? null;
  }, []);

  /**
   * 在组件提交后确保配置已进入请求队列。
   */
  const ensureConfig = useCallback((key: string): void => {
    if (cacheRef.current.has(`${cacheIdentityRef.current}|${key}`)) return;

    if (!loadingRef.current.has(key) && !pendingKeysRef.current.has(key)) {
      pendingKeysRef.current.add(key);
      triggerUpdate();
      scheduleBatch();
    }
  }, [scheduleBatch, triggerUpdate]);

  /**
   * 刷新指定配置
   */
  const refreshConfig = useCallback(async (key: string): Promise<void> => {
    // 从缓存中移除
    cacheRef.current.delete(`${cacheIdentityRef.current}|${key}`);

    // 加入待请求队列
    pendingKeysRef.current.add(key);

    // 立即执行
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }

    await executeBatch();
  }, [executeBatch]);

  /**
   * 刷新所有配置缓存
   */
  const refreshAll = useCallback(() => {
    generationRef.current += 1;
    cacheRef.current.clear();
    pendingKeysRef.current.clear();
    loadingRef.current.clear();
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }
    triggerUpdate();
  }, [triggerUpdate]);

  /**
   * 检查配置是否正在加载
   * 注意：此函数引用稳定，不依赖 state
   */
  const isLoading = useCallback((key: string): boolean => {
    return loadingRef.current.has(key) || pendingKeysRef.current.has(key);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    if (pendingKeysRef.current.size > 0 && !isFetchingRef.current) {
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
    pendingKeysRef.current.clear();
    loadingRef.current.clear();
    if (batchTimerRef.current) {
      clearTimeout(batchTimerRef.current);
      batchTimerRef.current = null;
    }
    triggerUpdate();
  }, [cacheIdentity, triggerUpdate]);

  // 使用稳定的 value 对象
  const value: ConfigClientContextValue = {
    getConfig,
    ensureConfig,
    refreshConfig,
    refreshAll,
    isLoading,
    version,
  };

  return (
    <ConfigClientContext.Provider value={value}>
      {children}
    </ConfigClientContext.Provider>
  );
};
