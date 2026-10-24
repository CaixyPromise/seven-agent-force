'use client';

import { useEffect, useMemo } from 'react';
import { useConfigClientContext } from './config/useConfigClientContext';
import {
  ConfigValueDTO,
  ConfigOptions,
  ConfigResult,
  buildConfigResult,
  isValidConfigDTO
} from '@/types/configClient';

/**
 * 获取配置值的Hook（支持泛型），返回包装对象 ConfigResult
 *
 * 使用示例：
 * ```tsx
 * // 基本使用（返回 ConfigResult<string>）
 * const appNameResult = useConfigValue('app.name');
 * const appName = appNameResult?.value; // 解析后的值
 * const appNameType = appNameResult?.type; // 值类型
 *
 * // 指定类型为number
 * const timeoutResult = useConfigValue<number>('order.timeout.minutes');
 * const timeout = timeoutResult?.value; // number 类型
 *
 * // JSON类型配置（指定对象类型）
 * interface FeatureConfig {
 *   enabled: boolean;
 *   maxUsers: number;
 * }
 * const featureResult = useConfigValue<FeatureConfig>('feature.config');
 * const featureConfig = featureResult?.value;
 *
 * // 使用 groupCode.key 格式
 * const configResult = useConfigValue<string>('SEVEN.title');
 *
 * // 使用 {groupCode, key} 选项
 * const groupConfigResult = useConfigValue<number>({ groupCode: 'order', key: 'timeout' });
 * ```
 *
 * @param keyOrOptions 配置键 或 {groupCode, key} 选项
 * @returns ConfigResult<T> 包装对象 或 null（加载中/未找到）
 */
export function useConfigValue<T = string>(
  keyOrOptions: string | ConfigOptions
): ConfigResult<T> | null {
  const { ensureConfig, getConfig, version } = useConfigClientContext();

  // 解析配置键
  const configKey = typeof keyOrOptions === 'string'
    ? keyOrOptions
    : `${keyOrOptions.groupCode}.${keyOrOptions.key}`;

  useEffect(() => {
    ensureConfig(configKey);
  }, [configKey, ensureConfig, version]);

  return useMemo(() => {
    // version 是 Provider 缓存的失效信号；读取它确保批量请求完成后重新取快照。
    void version;
    const dto = getConfig(configKey);
    if (dto && isValidConfigDTO(dto)) {
      return buildConfigResult<T>(dto);
    }
    return null;
  }, [configKey, getConfig, version]);
}

/**
 * 获取配置值的Hook（向后兼容），仅返回解析后的值
 *
 * 使用示例：
 * ```tsx
 * const appName = useConfigValueOnly('app.name'); // string | null
 * const timeout = useConfigValueOnly<number>('order.timeout'); // number | null
 * ```
 *
 * @param keyOrOptions 配置键 或 {groupCode, key} 选项
 * @returns 解析后的配置值（泛型类型T）或null（加载中/未找到）
 */
export function useConfigValueOnly<T = string>(
  keyOrOptions: string | ConfigOptions
): T | null {
  const result = useConfigValue<T>(keyOrOptions);
  return result?.value ?? null;
}

/**
 * 获取配置原始DTO的Hook
 * 适用于需要获取完整配置信息（包括类型）的场景
 *
 * @param key 配置键
 * @returns 配置值DTO或null
 */
export function useConfigDTO(key: string): ConfigValueDTO | null {
  const { ensureConfig, getConfig, version } = useConfigClientContext();

  useEffect(() => {
    ensureConfig(key);
  }, [ensureConfig, key, version]);

  return useMemo(() => {
    // version 是 Provider 缓存的失效信号；读取它确保批量请求完成后重新取快照。
    void version;
    const result = getConfig(key);
    if (result && isValidConfigDTO(result)) {
      return result;
    }
    return null;
  }, [getConfig, key, version]);
}

/**
 * 获取配置加载状态的Hook
 *
 * @param key 配置键
 * @returns 是否正在加载
 */
export function useConfigLoading(key: string): boolean {
  const { isLoading } = useConfigClientContext();
  return isLoading(key);
}

export default useConfigValue;
