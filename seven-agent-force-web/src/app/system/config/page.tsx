'use client';

import React, { useState } from 'react';
import { Button, Modal, Tabs, message } from 'antd';
import {SettingOutlined, ThunderboltOutlined, BranchesOutlined} from '@ant-design/icons';
import { ConfigProvider } from './context/ConfigContext';
import { ConfigGroupSidebar } from './components/ConfigGroupSidebar';
import { ConfigItemList } from './components/ConfigItemList';
import { CreateConfigGroupModal } from './components/CreateConfigGroupModal';
import { PendingConfigList } from './components/PendingConfigList';
import { ConfigChangeHistory } from './components/ConfigChangeHistory';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { refreshSystemCache } from '@/api/configController';

/**
 * 配置管理主页面
 */
const ConfigManagePage: React.FC = () => {
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState('manage');
  const [selectedConfigId, setSelectedConfigId] = useState<API.Int64 | null>(null);
  const [selectedConfigKey, setSelectedConfigKey] = useState<string>('');
  const [refreshing, setRefreshing] = useState(false);
  const canRefreshCache = usePermissionAccess(CONFIG_PERMISSIONS.CACHE_REFRESH);

  const handleRefreshCache = () => {
    Modal.confirm({
      title: '刷新缓存',
      content: '刷新后，系统会重新读取最新设置，短时间内响应可能变慢。',
      okText: '刷新缓存',
      cancelText: '取消',
      onOk: async () => {
        setRefreshing(true);
        try {
          const result = await refreshSystemCache();
          message.success(result.state === 'PENDING' ? '刷新已提交，系统会自动完成同步' : '缓存已刷新');
        } catch (error) {
          if (typeof error === 'object' && error !== null && 'code' in error && (error as { code?: number }).code === 42900) {
            message.error('操作过于频繁，请稍后再试');
          } else {
            message.error('缓存刷新提交失败，请稍后重试');
          }
        } finally {
          setRefreshing(false);
        }
      },
    });
  };

  const handleViewHistory = (configId: API.Int64, configKey: string) => {
    setSelectedConfigId(configId);
    setSelectedConfigKey(configKey);
    setActiveTab('history');
  };

  const tabItems = [
    {
      key: 'manage',
      label: (
        <span className="flex h-full gap-1">
          <SettingOutlined />
          配置管理
        </span>
      ),
      children: (
        <div className="config-manage-workspace flex min-h-0 gap-4 overflow-hidden">
          <ConfigGroupSidebar onCreateClick={() => setIsModalOpen(true)} />
          <ConfigItemList onViewHistory={handleViewHistory} />
          <CreateConfigGroupModal open={isModalOpen} onCancel={() => setIsModalOpen(false)} />
        </div>
      ),
    },
    {
      key: 'pending',
      label: (
        <span className="flex h-full gap-1">
          <ThunderboltOutlined />
          待生效配置
        </span>
      ),
      children: <PendingConfigList />,
    },
    {
      key: 'history',
      label: (
        <span className="flex h-full gap-1">
          <BranchesOutlined />
          变更历史
        </span>
      ),
      children: selectedConfigId ? (
        <ConfigChangeHistory
          configId={selectedConfigId}
          configKey={selectedConfigKey}
          onClose={() => setActiveTab('manage')}
        />
      ) : (
        <div className="p-8 text-center text-gray-400">
          请从配置管理中选择一个配置项查看变更历史
        </div>
      ),
    },
  ];

  return (
    <ConfigProvider>
      <div
        className="config-manage-page flex min-h-[620px] flex-col overflow-hidden bg-gray-50 p-4"
        style={{ height: 'calc(100dvh - 120px)' }}
      >
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={tabItems}
          animated={false}
          className="config-manage-tabs"
          tabBarExtraContent={
            canRefreshCache ? (
              <Button loading={refreshing} onClick={handleRefreshCache}>
                刷新缓存
              </Button>
            ) : null
          }
        />
      </div>
    </ConfigProvider>
  );
};

export default ConfigManagePage;
