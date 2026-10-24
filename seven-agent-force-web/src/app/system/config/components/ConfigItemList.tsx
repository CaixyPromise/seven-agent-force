'use client';

import React from 'react';
import { Button, Input, Pagination, Select, Skeleton, Tag } from 'antd';
import { PlusOutlined, SearchOutlined, SettingOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { ActionEmptyState } from '@/components/empty-state/ActionEmptyState';
import { useConfigContext } from '../context/useConfigContext';
import { ConfigItemCard } from './ConfigItemCard';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';

interface ConfigItemListProps {
  onViewHistory?: (configId: API.Int64, configKey: string) => void;
}

export const ConfigItemList: React.FC<ConfigItemListProps> = ({ onViewHistory }) => {
  const hasCreatePermission = usePermissionAccess(CONFIG_PERMISSIONS.ADD);
  const {
    activeGroup,
    configs,
    loadingConfigs,
    configTotal,
    configPageNum,
    configPageSize,
    fetchConfigs,
    setConfigPageNum,
    setConfigPageSize,
    configSearchText,
    setConfigSearchText,
    configSearchType,
    setConfigSearchType,
    handleUpdateConfig,
    handleDeleteConfig,
    handleCreateConfig,
    addTempConfig,
    removeTempConfig,
  } = useConfigContext();

  const handleAddConfig = () => {
    if (!activeGroup) return;
    const tempId = `-${Date.now()}`;
    const now = dayjs().format('YYYY-MM-DD HH:mm:ss');
    const newConfig = {
      id: tempId,
      groupId: activeGroup.id,
      configKey: '',
      configValue: '',
      valueType: 'STRING' as const,
      configDesc: '',
      isSensitive: 0,
      isReadonly: 0,
      isEnabled: 1,
      effectType: 'realtime' as const,
      uiWidget: 'INPUT' as const,
      exposure: 'INTERNAL' as const,
      sensitivity: 'NORMAL' as const,
      schemaVersion: 1,
      version: '1',
      valuePresent: false,
      connected: false,
      consumerStatus: 'UNCONNECTED' as const,
      updateTime: now,
      createTime: now,
    };
    addTempConfig(newConfig);
  };

  if (!activeGroup) {
    return (
      <div className="h-full flex items-center justify-center">
        <ActionEmptyState
          icon={<SettingOutlined />}
          title="请选择左侧配置分组"
          description="选择一个配置分组后，这里会显示对应配置项列表。"
        />
      </div>
    );
  }

  const hasSearchFilter = configSearchText.trim().length > 0;
  const canCreateInGroup = hasCreatePermission && activeGroup.access?.canWrite !== false;

  const handleSearch = () => {
    setConfigPageNum(1);
    fetchConfigs({
      groupId: activeGroup.id,
      pageNum: 1,
      pageSize: configPageSize,
      forceKeyword: configSearchText.trim(),
      forceSearchType: configSearchType,
    });
  };

  const clearSearch = () => {
    setConfigSearchText('');
    setConfigSearchType('both');
    setConfigPageNum(1);
    fetchConfigs({
      groupId: activeGroup.id,
      pageNum: 1,
      pageSize: configPageSize,
      forceKeyword: '',
      forceSearchType: 'both',
    });
  };

  const handlePageChange = (pageNum: number, pageSize: number) => {
    setConfigPageNum(pageNum);
    setConfigPageSize(pageSize);
    fetchConfigs({ groupId: activeGroup.id, pageNum, pageSize });
  };

  return (
    <div className="flex-1 min-w-0 bg-white rounded-lg shadow-sm border border-gray-200 h-full flex flex-col overflow-hidden">
      <div className="px-6 py-4 border-b border-gray-100 space-y-3">
        <div className="flex min-w-0 items-center justify-between gap-4">
          <div className="flex min-w-0 flex-wrap items-center gap-3">
            <h2 className="min-w-0 break-words text-xl font-bold text-gray-800 m-0">{activeGroup.groupName}</h2>
            <Tag className="max-w-full">
              <span className="block max-w-full break-all">{activeGroup.groupCode}</span>
            </Tag>
          </div>
          {canCreateInGroup ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={handleAddConfig}>
              新增配置
            </Button>
          ) : null}
        </div>
        <div className="flex gap-3">
          <Select
            value={configSearchType}
            style={{ width: 140 }}
            onChange={value => {
              setConfigSearchType(value);
              setConfigPageNum(1);
              fetchConfigs({
                groupId: activeGroup.id,
                pageNum: 1,
                pageSize: configPageSize,
                forceKeyword: configSearchText.trim(),
                forceSearchType: value,
              });
            }}
            options={[
              { value: 'both', label: 'Label + Key' },
              { value: 'label', label: '仅 Label' },
              { value: 'key', label: '仅 Key' },
            ]}
          />
          <Input
            value={configSearchText}
            placeholder="搜索配置 Label / Key"
            allowClear
            onChange={e => setConfigSearchText(e.target.value)}
            onPressEnter={handleSearch}
            prefix={<SearchOutlined className="text-gray-400" />}
          />
          <Button type="primary" onClick={handleSearch}>
            搜索
          </Button>
          {hasSearchFilter && <Button onClick={clearSearch}>清空筛选</Button>}
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-6 bg-gray-50/30">
        {loadingConfigs ? (
          <Skeleton active paragraph={{ rows: 6 }} />
        ) : configs.length === 0 ? (
          <ActionEmptyState
            icon={<PlusOutlined />}
            title={hasSearchFilter ? '未找到匹配的配置项' : '该分组下暂无配置'}
            description={
              hasSearchFilter
                ? '调整搜索关键字或搜索维度后重试。'
                : '点击右上角“新增配置”按钮，创建当前分组的第一条配置。'
            }
            actionText={hasSearchFilter ? '清空筛选' : canCreateInGroup ? '新增配置' : undefined}
            onAction={hasSearchFilter ? clearSearch : canCreateInGroup ? handleAddConfig : undefined}
          />
        ) : (
          <div className="space-y-4 max-w-5xl mx-auto">
            {configs.map(config => (
              <ConfigItemCard
                key={config.id}
                config={config}
                groupCode={activeGroup.groupCode}
                isNew={config.id.startsWith('-')}
                onSave={handleUpdateConfig}
                onCreate={handleCreateConfig}
                onDelete={handleDeleteConfig}
                onCancel={removeTempConfig}
                onViewHistory={onViewHistory}
              />
            ))}
          </div>
        )}
      </div>

      <div className="px-6 py-4 border-t border-gray-100 bg-white flex justify-end">
        <Pagination
          current={configPageNum}
          pageSize={configPageSize}
          total={configTotal}
          showTotal={total => `共 ${total} 条`}
          showSizeChanger
          pageSizeOptions={[10, 20, 50, 100]}
          onChange={handlePageChange}
          onShowSizeChange={handlePageChange}
        />
      </div>
    </div>
  );
};
