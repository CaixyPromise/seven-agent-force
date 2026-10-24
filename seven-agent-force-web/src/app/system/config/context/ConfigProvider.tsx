'use client';

import React, { useState, useCallback, useEffect, useMemo } from 'react';
import { message } from 'antd';
import type {
  ConfigGroup,
  ConfigItem,
  ConfigPageResult,
  CreateConfigGroupRequest,
  UpdateConfigRequest,
} from '@/types/config';
import {
  getConfigGroupPage,
  createConfigGroup,
  updateConfigGroup,
  deleteConfigGroup,
  moveConfigGroup,
  getConfigs,
  createConfig,
  updateConfig,
  deleteConfig,
} from '@/api/configController';
import {
  ConfigContext,
  type ConfigContextType,
} from './useConfigContext';

export const ConfigProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [groups, setGroups] = useState<ConfigGroup[]>([]);
  const [groupTotal, setGroupTotal] = useState(0);
  const [groupPageNum, setGroupPageNum] = useState(1);
  const [groupPageSize, setGroupPageSize] = useState(20);
  const [activeGroup, setActiveGroup] = useState<ConfigGroup | null>(null);
  const [loadingGroups, setLoadingGroups] = useState(false);
  const [configs, setConfigs] = useState<ConfigItem[]>([]);
  const [configTotal, setConfigTotal] = useState(0);
  const [configPageNum, setConfigPageNum] = useState(1);
  const [configPageSize, setConfigPageSize] = useState(20);
  const [loadingConfigs, setLoadingConfigs] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [configSearchText, setConfigSearchText] = useState('');
  const [configSearchType, setConfigSearchType] = useState<'label' | 'key' | 'both'>('both');

  // 获取配置分组列表
  const fetchGroups = useCallback(async (params?: { pageNum?: number; pageSize?: number; keyword?: string }) => {
    setLoadingGroups(true);
    try {
      const pageNum = params?.pageNum ?? groupPageNum;
      const pageSize = params?.pageSize ?? groupPageSize;
      const keyword = (params?.keyword ?? searchTerm).trim();
      const pageData = await getConfigGroupPage({ pageNum, pageSize, keyword });
      const records = pageData?.records || pageData?.list || [];
      const sortedGroups = (records || []).sort((a: ConfigGroup, b: ConfigGroup) => (a.sortOrder || 0) - (b.sortOrder || 0));
      setGroups(sortedGroups);
      setGroupTotal(Number(pageData?.total || 0));
      setGroupPageNum(Number(pageData?.current || pageNum));
      setGroupPageSize(Number(pageData?.size || pageSize));
      // 只在没有选中分组且列表不为空时自动选中第一个
      if (sortedGroups.length > 0 && !activeGroup) {
        setActiveGroup(sortedGroups[0]);
      }
    } catch (error: unknown) {
      message.error((error as Error)?.message || '获取配置分组失败');
    } finally {
      setLoadingGroups(false);
    }
  }, [activeGroup, groupPageNum, groupPageSize, searchTerm]);

  // 获取配置项列表
  const fetchConfigs = useCallback(async (params?: {
    groupId?: API.Int64;
    pageNum?: number;
    pageSize?: number;
    autoFallbackWhenEmpty?: boolean;
    forceKeyword?: string;
    forceSearchType?: 'label' | 'key' | 'both';
  }) => {
    const groupId = params?.groupId ?? activeGroup?.id;
    if (!groupId) return;
    setLoadingConfigs(true);
    try {
      const pageNum = params?.pageNum ?? configPageNum;
      const pageSize = params?.pageSize ?? configPageSize;
      const searchText = (params?.forceKeyword ?? configSearchText).trim();
      const searchType = params?.forceSearchType ?? configSearchType;
      const pageData: ConfigPageResult = await getConfigs({
        groupId,
        pageNum,
        pageSize,
        searchText,
        searchType,
      });
      const pageRecords = pageData.records || [];
      const pageTotal = pageData.total || 0;
      setConfigs(pageRecords);
      setConfigTotal(pageTotal);
      setConfigPageNum(pageNum);
      setConfigPageSize(pageSize);

      if ((params?.autoFallbackWhenEmpty ?? true) && pageNum > 1 && pageRecords.length === 0 && pageTotal > 0) {
        await fetchConfigs({
          groupId,
          pageNum: pageNum - 1,
          pageSize,
          autoFallbackWhenEmpty: false,
          forceKeyword: searchText,
          forceSearchType: searchType,
        });
      }
    } catch (error: unknown) {
      message.error((error as Error)?.message || '获取配置项失败');
    } finally {
      setLoadingConfigs(false);
    }
  }, [activeGroup?.id, configPageNum, configPageSize, configSearchText, configSearchType]);

  // 创建配置分组
  const handleCreateGroup = useCallback(async (values: CreateConfigGroupRequest) => {
    try {
      await createConfigGroup(values);
      message.success('配置分组创建成功');
      await fetchGroups();
    } catch (error: unknown) {
      message.error((error as Error)?.message || '创建失败');
      throw error;
    }
  }, [fetchGroups]);

  // 更新配置分组
  const handleUpdateGroup = useCallback(async (id: API.Int64, field: keyof ConfigGroup, value: unknown) => {
    try {
      await updateConfigGroup({ id, [field]: value });
      message.success('更新成功');
      await fetchGroups();
      if (activeGroup?.id === id) {
        setActiveGroup({ ...activeGroup, [field]: value });
      }
    } catch (error: unknown) {
      message.error((error as Error)?.message || '更新失败');
      throw error;
    }
  }, [fetchGroups, activeGroup]);

  // 删除配置分组
  const handleDeleteGroup = useCallback(async (id: API.Int64) => {
    try {
      await deleteConfigGroup(id);
      message.success('删除成功');
      if (activeGroup?.id === id) {
        setActiveGroup(null);
      }
      await fetchGroups();
    } catch (error: unknown) {
      message.error((error as Error)?.message || '删除失败');
      throw error;
    }
  }, [fetchGroups, activeGroup]);

  // 移动分组到指定位置（按规范使用beforeId/afterId）
  const handleMoveGroup = useCallback(async (
    id: API.Int64,
    beforeId?: API.Int64 | null,
    afterId?: API.Int64 | null,
  ) => {
    try {
      await moveConfigGroup(id, beforeId, afterId);
      message.success('排序已更新');
      // 重新加载分组列表以获取最新顺序
      await fetchGroups();
    } catch (error: unknown) {
      message.error((error as Error)?.message || '排序更新失败');
      throw error;
    }
  }, [fetchGroups]);

  // 添加临时配置项（本地虚拟）
  const addTempConfig = useCallback((config: ConfigItem) => {
    setConfigs(prev => [config, ...prev]);
    setConfigTotal(prev => prev + 1);
  }, []);

  // 移除临时配置项
  const removeTempConfig = useCallback((id: API.Int64) => {
    setConfigs(prev => prev.filter(c => c.id !== id));
    setConfigTotal(prev => Math.max(prev - 1, 0));
  }, []);

  // 创建配置项（提交到后端）
  const handleCreateConfig = useCallback(async (config: ConfigItem & { assetFileId?: API.Int64 }) => {
    try {
      const createRequest = {
        groupId: config.groupId,
        configKey: config.configKey,
        valueType: config.valueType,
        configDesc: config.configDesc,
        isSensitive: config.isSensitive,
        isReadonly: config.isReadonly,
        effectType: config.effectType,
        uiWidget: config.uiWidget,
        validation: config.validation,
        exposure: config.exposure,
        sensitivity: config.sensitivity,
        schemaVersion: config.schemaVersion,
      };
      if (config.valueType !== 'IMAGE' && config.valueType !== 'FILE') {
        Object.assign(createRequest, { configValue: config.configValue });
      }
      if (config.assetFileId !== undefined) {
        Object.assign(createRequest, { assetFileId: config.assetFileId });
      }
      await createConfig(createRequest);
      message.success('配置项创建成功');
      await fetchConfigs({ groupId: activeGroup?.id ?? config.groupId, pageNum: 1 });
    } catch (error: unknown) {
      message.error((error as Error)?.message || '创建失败');
      throw error;
    }
  }, [activeGroup?.id, fetchConfigs]);

  // 更新配置项
  const handleUpdateConfig = useCallback(async (config: Partial<ConfigItem> & Pick<ConfigItem, 'id'> & {
    assetFileId?: API.Int64;
    clearAsset?: boolean;
  }) => {
    try {
      const updateRequest: UpdateConfigRequest = {
        id: config.id,
      };
      if (config.configKey !== undefined) updateRequest.configKey = config.configKey;
      if (config.configValue !== undefined) updateRequest.configValue = config.configValue;
      if (config.configDesc !== undefined) updateRequest.configDesc = config.configDesc;
      if (config.isEnabled !== undefined) updateRequest.isEnabled = config.isEnabled;
      if (config.effectType !== undefined) updateRequest.effectType = config.effectType;
      if (config.valueType !== undefined) updateRequest.valueType = config.valueType;
      if (config.isSensitive !== undefined) updateRequest.isSensitive = config.isSensitive;
      if (config.isReadonly !== undefined) updateRequest.isReadonly = config.isReadonly;
      if (config.uiWidget !== undefined) updateRequest.uiWidget = config.uiWidget;
      if (config.validation !== undefined) updateRequest.validation = config.validation;
      if (config.exposure !== undefined) updateRequest.exposure = config.exposure;
      if (config.sensitivity !== undefined) updateRequest.sensitivity = config.sensitivity;
      if (config.schemaVersion !== undefined) updateRequest.schemaVersion = config.schemaVersion;
      if (config.version !== undefined) updateRequest.version = config.version;
      if (config.assetFileId !== undefined) updateRequest.assetFileId = config.assetFileId;
      if (config.clearAsset === true) updateRequest.clearAsset = true;
      await updateConfig(updateRequest);
      message.success('配置已保存');
      await fetchConfigs({
        groupId: activeGroup?.id,
        pageNum: configPageNum,
        pageSize: configPageSize,
      });
    } catch (error: unknown) {
      message.error((error as Error)?.message || '保存失败');
      throw error;
    }
  }, [activeGroup?.id, configPageNum, configPageSize, fetchConfigs]);

  // 删除配置项
  const handleDeleteConfig = useCallback(async (id: API.Int64) => {
    try {
      await deleteConfig(id);
      message.success('删除成功');
      await fetchConfigs({ pageNum: configPageNum });
    } catch (error: unknown) {
      message.error((error as Error)?.message || '删除失败');
      throw error;
    }
  }, [configPageNum, fetchConfigs]);

  // 分组配置分组
  const groupedGroups = useMemo(() => {
    const map: Record<string, ConfigGroup[]> = {};
    groups.forEach(g => {
      const mod = g.module || 'other';
      if (!map[mod]) map[mod] = [];
      map[mod].push(g);
    });
    return map;
  }, [groups]);

  // 初始加载
  useEffect(() => {
    fetchGroups();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // 选中分组时加载配置项
  useEffect(() => {
    if (activeGroup?.id) {
      setConfigPageNum(1);
      fetchConfigs({ groupId: activeGroup.id, pageNum: 1 });
    } else {
      setConfigs([]);
      setConfigTotal(0);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeGroup?.id]);

  const value: ConfigContextType = {
    groups,
    groupTotal,
    groupPageNum,
    groupPageSize,
    activeGroup,
    loadingGroups,
    setActiveGroup,
    fetchGroups,
    setGroupPageNum,
    setGroupPageSize,
    handleCreateGroup,
    handleUpdateGroup,
    handleDeleteGroup,
    handleMoveGroup,
    configs,
    configTotal,
    configPageNum,
    configPageSize,
    loadingConfigs,
    fetchConfigs,
    addTempConfig,
    removeTempConfig,
    handleCreateConfig,
    handleUpdateConfig,
    handleDeleteConfig,
    searchTerm,
    setSearchTerm,
    groupedGroups,
    configSearchText,
    setConfigSearchText,
    configSearchType,
    setConfigSearchType,
    setConfigPageNum,
    setConfigPageSize,
  };

  return <ConfigContext.Provider value={value}>{children}</ConfigContext.Provider>;
};
