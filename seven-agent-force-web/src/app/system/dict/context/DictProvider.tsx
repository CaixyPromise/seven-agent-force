'use client';

import React, { useState, useCallback, useEffect, useMemo } from 'react';
import { message } from 'antd';
import type { DictType, DictItem, CreateDictTypeRequest } from '@/types/dict';
import {
  getDictTypePage,
  createDictType,
  updateDictType,
  deleteDictType,
  moveDictType,
  getDictItems,
  createDictItem,
  updateDictItem,
  deleteDictItem,
  moveDictItem,
} from '@/api/dictController';
import {
  DictContext,
  type DictContextType,
} from './useDictContext';

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

export const DictProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [types, setTypes] = useState<DictType[]>([]);
  const [typeTotal, setTypeTotal] = useState(0);
  const [typePageNum, setTypePageNum] = useState(1);
  const [typePageSize, setTypePageSize] = useState(20);
  const [selectedType, setSelectedType] = useState<DictType | null>(null);
  const [loadingTypes, setLoadingTypes] = useState(false);
  const [items, setItems] = useState<DictItem[]>([]);
  const [itemTotal, setItemTotal] = useState(0);
  const [itemPageNum, setItemPageNum] = useState(1);
  const [itemPageSize, setItemPageSize] = useState(20);
  const [itemSearchTerm, setItemSearchTerm] = useState('');
  const [loadingItems, setLoadingItems] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');

  // 获取字典类型列表
  const fetchTypes = useCallback(async (params?: { pageNum?: number; pageSize?: number; keyword?: string }) => {
    setLoadingTypes(true);
    try {
      const pageNum = params?.pageNum ?? typePageNum;
      const pageSize = params?.pageSize ?? typePageSize;
      const keyword = (params?.keyword ?? searchTerm).trim();
      const res = await getDictTypePage({ pageNum, pageSize, keyword });
      const list = res.records ?? res.list ?? [];
      const sortedTypes = list.sort((a: DictType, b: DictType) => (a.sortOrder || 0) - (b.sortOrder || 0));
      setTypes(sortedTypes);
      setTypeTotal(Number(res?.total || 0));
      setTypePageNum(Number(res?.current || pageNum));
      setTypePageSize(Number(res?.size || pageSize));
      setSelectedType(current => {
        if (!current) return sortedTypes[0] ?? null;
        return sortedTypes.find(item => item.id === current.id) ?? null;
      });
    } catch (error) {
      message.error(getErrorMessage(error, '获取字典类型失败'));
    } finally {
      setLoadingTypes(false);
    }
  }, [searchTerm, typePageNum, typePageSize]);

  // 获取字典项列表
  const fetchItems = useCallback(async (
    typeId: API.Int64,
    params?: { pageNum?: number; pageSize?: number; keyword?: string },
  ) => {
    if (!typeId) return;
    setLoadingItems(true);
    try {
      const pageNum = params?.pageNum ?? itemPageNum;
      const pageSize = params?.pageSize ?? itemPageSize;
      const keyword = (params?.keyword ?? itemSearchTerm).trim();
      const list = await getDictItems({ dictTypeId: typeId, keyword });
      const sortedItems = (list || []).sort((a: DictItem, b: DictItem) => (a.sortOrder || 0) - (b.sortOrder || 0));
      const start = (pageNum - 1) * pageSize;
      setItems(sortedItems.slice(start, start + pageSize));
      setItemTotal(sortedItems.length);
      setItemPageNum(pageNum);
      setItemPageSize(pageSize);
    } catch (error) {
      message.error(getErrorMessage(error, '获取字典项失败'));
    } finally {
      setLoadingItems(false);
    }
  }, [itemPageNum, itemPageSize, itemSearchTerm]);

  // 创建字典类型
  const handleCreateType = useCallback(async (values: CreateDictTypeRequest) => {
    try {
      await createDictType(values);
      message.success('字典类型创建成功');
      await fetchTypes();
    } catch (error) {
      message.error(getErrorMessage(error, '创建失败'));
      throw error;
    }
  }, [fetchTypes]);

  // 更新字典类型
  const handleUpdateType = useCallback(async <K extends keyof DictType,>(
    id: API.Int64,
    field: K,
    value: DictType[K],
  ) => {
    try {
      const current = types.find(item => item.id === id);
      await updateDictType({ id, version: current?.version, [field]: value });
      message.success('更新成功');
      await fetchTypes();
    } catch (error) {
      message.error(getErrorMessage(error, '更新失败'));
      throw error;
    }
  }, [fetchTypes, types]);

  // 删除字典类型
  const handleDeleteType = useCallback(async (id: API.Int64) => {
    try {
      await deleteDictType(id);
      message.success('删除成功');
      if (selectedType?.id === id) {
        setSelectedType(null);
      }
      await fetchTypes();
    } catch (error) {
      message.error(getErrorMessage(error, '删除失败'));
      throw error;
    }
  }, [fetchTypes, selectedType]);

  // 移动字典类型到指定位置（按规范使用beforeId/afterId）
  const handleMoveType = useCallback(async (
    id: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => {
    try {
      await moveDictType(id, beforeId, afterId);
      message.success('排序已更新');
      // 重新加载字典类型列表以获取最新顺序
      await fetchTypes();
    } catch (error) {
      message.error(getErrorMessage(error, '排序更新失败'));
      throw error;
    }
  }, [fetchTypes]);

  // 添加临时字典项（本地虚拟）
  const addTempItem = useCallback((item: DictItem) => {
    setItems(prev => [item, ...prev]);
  }, []);

  // 移除临时字典项
  const removeTempItem = useCallback((id: API.Int64) => {
    setItems(prev => prev.filter(item => item.id !== id));
  }, []);

  // 创建字典项（提交到后端）
  const handleCreateItem = useCallback(async (item: DictItem) => {
    try {
      await createDictItem({
        dictTypeId: item.dictTypeId,
        itemValue: item.itemValue,
        itemLabel: item.itemLabel,
        itemDesc: item.itemDesc,
        colorToken: item.colorToken,
        iconToken: item.iconToken,
        sortOrder: item.sortOrder,
      });
      message.success('字典项创建成功');
      if (selectedType) {
        await fetchItems(selectedType.id, { pageNum: 1 });
      }
    } catch (error) {
      message.error(getErrorMessage(error, '创建失败'));
      throw error;
    }
  }, [selectedType, fetchItems]);

  // 更新字典项（一次性提交所有变更字段）
  const handleUpdateItem = useCallback(async (item: DictItem) => {
    try {
      // 一次性提交所有字段更新
      await updateDictItem({
        id: item.id,
        itemLabel: item.itemLabel,
        itemDesc: item.itemDesc,
        sortOrder: item.sortOrder,
        status: item.status,
        colorToken: item.colorToken,
        iconToken: item.iconToken,
        version: item.version,
      });
      message.success('更新成功');
      if (selectedType) {
        await fetchItems(selectedType.id, { pageNum: itemPageNum });
      }
    } catch (error) {
      message.error(getErrorMessage(error, '更新失败'));
      throw error;
    }
  }, [fetchItems, itemPageNum, selectedType]);

  // 删除字典项
  const handleDeleteItem = useCallback(async (id: API.Int64) => {
    try {
      await deleteDictItem(id);
      message.success('删除成功');
      setItems(prev => prev.filter(item => item.id !== id));
    } catch (error) {
      message.error(getErrorMessage(error, '删除失败'));
      throw error;
    }
  }, []);

  // 移动字典项到指定位置（按规范使用beforeId/afterId）
  const handleMoveItem = useCallback(async (
    itemId: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => {
    if (!selectedType) return;
    try {
      await moveDictItem(selectedType.id, itemId, beforeId, afterId);
      message.success('排序已更新');
      // 重新加载字典项列表以获取最新顺序
      await fetchItems(selectedType.id, { pageNum: itemPageNum });
    } catch (error) {
      message.error(getErrorMessage(error, '排序更新失败'));
      throw error;
    }
  }, [selectedType, fetchItems, itemPageNum]);

  // 分组字典类型
  const groupedTypes = useMemo(() => {
    const groups: Record<string, DictType[]> = {};
    types.forEach(t => {
      const mod = t.module || 'other';
      if (!groups[mod]) groups[mod] = [];
      groups[mod].push(t);
    });
    return groups;
  }, [types]);

  // 初始加载
  useEffect(() => {
    fetchTypes();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 选中类型时加载字典项
  useEffect(() => {
    if (selectedType?.id) {
      setItemPageNum(1);
      fetchItems(selectedType.id, { pageNum: 1 });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedType?.id]);

  const value: DictContextType = {
    types,
    typeTotal,
    typePageNum,
    typePageSize,
    selectedType,
    loadingTypes,
    setSelectedType,
    fetchTypes,
    setTypePageNum,
    setTypePageSize,
    handleCreateType,
    handleUpdateType,
    handleDeleteType,
    handleMoveType,
    items,
    itemTotal,
    itemPageNum,
    itemPageSize,
    itemSearchTerm,
    loadingItems,
    fetchItems,
    setItemSearchTerm,
    setItemPageNum,
    setItemPageSize,
    addTempItem,
    removeTempItem,
    handleCreateItem,
    handleUpdateItem,
    handleDeleteItem,
    handleMoveItem,
    searchTerm,
    setSearchTerm,
    groupedTypes,
  };

  return <DictContext.Provider value={value}>{children}</DictContext.Provider>;
};
