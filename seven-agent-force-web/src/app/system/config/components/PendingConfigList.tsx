'use client';

import React, { useEffect, useState } from 'react';
import { Button, Table, Tag, Popconfirm, message, Space } from 'antd';
import {
  ThunderboltOutlined,
  ReloadOutlined,
} from '@ant-design/icons';
import {
  getPendingConfigs,
  applyPendingConfigs,
} from '@/api/configController';
import type { PendingConfig } from '@/types/config';
import { formatDate } from '@/utils/date';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';

/**
 * 待生效配置列表组件
 */
export const PendingConfigList: React.FC = () => {
  const canApplyConfigs = usePermissionAccess(CONFIG_PERMISSIONS.APPLY);
  const [pendingConfigs, setPendingConfigs] = useState<PendingConfig[]>([]);
  const [loading, setLoading] = useState(false);
  const [applying, setApplying] = useState(false);

  const fetchPendingConfigs = async () => {
    setLoading(true);
    try {
      const res = await getPendingConfigs();
      setPendingConfigs(res || []);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '获取待生效配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchPendingConfigs();
  }, []);

  const handleApply = async () => {
    setApplying(true);
    try {
      const response = await applyPendingConfigs();
      const count = response.data ?? 0;
      message.success(`成功应用 ${count} 个配置变更`);
      await fetchPendingConfigs();
    } catch (error) {
      message.error(error instanceof Error ? error.message : '应用配置失败');
    } finally {
      setApplying(false);
    }
  };

  const columns = [
    {
      title: '配置键',
      dataIndex: 'configKey',
      key: 'configKey',
      width: 200,
      render: (text: string) => (
        <code className="text-sm bg-gray-100 px-2 py-1 rounded">{text}</code>
      ),
    },
    {
      title: '配置描述',
      dataIndex: 'configDesc',
      key: 'configDesc',
      ellipsis: true,
    },
    {
      title: '当前生效值',
      dataIndex: 'currentValue',
      key: 'currentValue',
      width: 200,
      ellipsis: true,
      render: (text: string) => (
        <span className="text-gray-600">{text || '-'}</span>
      ),
    },
    {
      title: '待生效值',
      dataIndex: 'pendingValue',
      key: 'pendingValue',
      width: 200,
      ellipsis: true,
      render: (text: string) => (
        <span className="text-orange-600 font-medium">{text}</span>
      ),
    },
    {
      title: '创建人',
      dataIndex: 'createdByName',
      key: 'createdByName',
      width: 120,
      render: (text: string) => text || '-',
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime',
      width: 180,
      render: (text: string) => formatDate(text),
    },
  ];

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <ThunderboltOutlined className="text-orange-500" />
          <h2 className="text-lg font-semibold">待生效配置</h2>
          <Tag color="orange">重启生效</Tag>
        </div>
        <Space>
          <Button
            icon={<ReloadOutlined />}
            onClick={fetchPendingConfigs}
            loading={loading}
          >
            刷新
          </Button>
          {canApplyConfigs ? (
            <Popconfirm
              title="确定要应用所有待生效配置吗？"
              description="这将立即应用当前可写范围内的待生效配置变更，系统启动时也会自动执行此操作。"
              onConfirm={handleApply}
              okText="确定"
              cancelText="取消"
            >
              <Button
                type="primary"
                icon={<ThunderboltOutlined />}
                loading={applying}
              >
                应用可写范围配置
              </Button>
            </Popconfirm>
          ) : null}
        </Space>
      </div>

      <Table
        columns={columns}
        dataSource={pendingConfigs}
        loading={loading}
        rowKey="logId"
        pagination={false}
        locale={{ emptyText: '暂无待生效配置' }}
      />
    </div>
  );
};
