'use client';

import React, { ReactNode } from 'react';
import { ConfigClientProvider } from '@/hooks/config/ConfigClientContext';
import { DictClientProvider } from '@/hooks/dict/DictClientContext';
import RuntimeBrandAssets from './RuntimeBrandAssets';

/**
 * 客户端数据Provider
 * 整合配置和字典的Context Provider，提供全局数据缓存和批量请求合并功能
 *
 * 使用后可在任意组件中使用以下Hook：
 * - useConfigValue<T>(key) - 获取配置值（支持泛型）
 * - useDictValue<T>(code) - 获取字典项列表（支持泛型）
 * - useDictOptions(code) - 获取字典选项（{value, label}格式）
 * - useDictLabel(code, value) - 获取字典标签
 */
export const ClientDataProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  return (
    <ConfigClientProvider>
      <RuntimeBrandAssets />
      <DictClientProvider>
        {children}
      </DictClientProvider>
    </ConfigClientProvider>
  );
};

export default ClientDataProvider;
