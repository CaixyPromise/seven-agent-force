'use client';

import { useEffect, useMemo } from 'react';
import { useDictClientContext } from './dict/useDictClientContext';
import type { DictItemVO, DictOptions, DictResult } from '@/types/dictClient';
import {
  getDictLabel,
  getDictItemByValue,
  parseDictItemExt,
  buildDictResult,
  toDictOptions
} from '@/types/dictClient';

/**
 * 获取字典值的Hook（支持泛型），返回包装对象 DictResult
 *
 * 使用示例：
 * ```tsx
 * // 基本使用（返回 DictResult<DictItemVO[]>）
 * const genderResult = useDictValue('GENDER');
 * const genderItems = genderResult?.items; // 字典项数组
 * const genderValue = genderResult?.value; // 同 items
 * const dictCode = genderResult?.dictCode; // 'GENDER'
 *
 * // 使用 {id} 选项
 * const dictResult = useDictValue({ id: 123 });
 * ```
 *
 * @param codeOrOptions 字典编码 或 {id} 选项
 * @returns DictResult 包装对象 或 null（加载中/未找到）
 */
export function useDictValue<T = DictItemVO[]>(
  codeOrOptions: string | DictOptions
): DictResult<T> | null {
  const { ensureDict, getDict, version } = useDictClientContext();

  // 解析字典编码
  const dictCode = typeof codeOrOptions === 'string'
    ? codeOrOptions
    : `dict:${codeOrOptions.id}`;

  useEffect(() => {
    ensureDict(dictCode);
  }, [dictCode, ensureDict, version]);

  return useMemo(() => {
    // version 是 Provider 缓存的失效信号；读取它确保批量请求完成后重新取快照。
    void version;
    const items = getDict(dictCode);
    if (items) {
      return buildDictResult<T>(dictCode, items);
    }
    return null;
  }, [dictCode, getDict, version]);
}

/**
 * 获取字典项列表的Hook（向后兼容），仅返回字典项数组
 *
 * 使用示例：
 * ```tsx
 * const genders = useDictValueOnly('GENDER'); // DictItemVO[] | null
 * ```
 *
 * @param codeOrOptions 字典编码 或 {id} 选项
 * @returns 字典项列表（DictItemVO[]）或null（加载中/未找到）
 */
export function useDictValueOnly(
  codeOrOptions: string | DictOptions
): DictItemVO[] | null {
  const result = useDictValue<DictItemVO[]>(codeOrOptions);
  return result?.items ?? null;
}

/**
 * 获取字典选项列表的Hook，返回包装对象 DictResult
 * 适用于下拉选择器等组件
 *
 * 使用示例：
 * ```tsx
 * const genderOptionsResult = useDictOptions('GENDER');
 * const options = genderOptionsResult?.value; // { value: string; label: string }[]
 * ```
 *
 * @param code 字典编码
 * @returns DictResult<{value, label}[]> 或 null
 */
export function useDictOptions(code: string): DictResult<{ value: string; label: string }[]> | null {
  const result = useDictValue<DictItemVO[]>(code);

  if (!result) return null;

  const options = toDictOptions(result.items);
  return buildDictResult(code, result.items, () => options);
}

/**
 * 获取字典选项列表的Hook（向后兼容），仅返回选项数组
 *
 * @param code 字典编码
 * @returns 选项列表 或 null
 */
export function useDictOptionsOnly(code: string): { value: string; label: string }[] | null {
  const result = useDictOptions(code);
  return result?.value ?? null;
}

/**
 * 根据字典编码和值获取标签的Hook
 *
 * 使用示例：
 * ```tsx
 * const genderLabel = useDictLabel('GENDER', '1'); // 返回 "男"
 * ```
 *
 * @param code 字典编码
 * @param value 字典值
 * @returns 字典标签 或 原始值（未找到时）
 */
export function useDictLabel(code: string, value: string): string {
  const result = useDictValue<DictItemVO[]>(code);

  if (!result?.items) return value;

  return getDictLabel(result.items, value);
}

/**
 * 根据字典编码和值获取字典项的Hook
 *
 * @param code 字典编码
 * @param value 字典值
 * @returns 字典项 或 undefined
 */
export function useDictItem(code: string, value: string): DictItemVO | undefined {
  const result = useDictValue<DictItemVO[]>(code);

  if (!result?.items) return undefined;

  return getDictItemByValue(result.items, value);
}

/**
 * 获取字典项扩展属性的Hook
 *
 * 使用示例：
 * ```tsx
 * interface GenderExt { color: string; icon: string }
 * const ext = useDictItemExt<GenderExt>('GENDER', '1');
 * // ext = { color: 'blue', icon: 'man' }
 * ```
 *
 * @param code 字典编码
 * @param value 字典值
 * @returns 扩展属性对象 或 null
 */
export function useDictItemExt<T extends Record<string, unknown>>(
  code: string,
  value: string
): T | null {
  const item = useDictItem(code, value);

  if (!item) return null;

  return parseDictItemExt<T>(item);
}

/**
 * 获取字典加载状态的Hook
 *
 * @param code 字典编码
 * @returns 是否正在加载
 */
export function useDictLoading(code: string): boolean {
  const { isLoading } = useDictClientContext();
  return isLoading(code);
}

export default useDictValue;
