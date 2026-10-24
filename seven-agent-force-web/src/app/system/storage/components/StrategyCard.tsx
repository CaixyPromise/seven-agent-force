'use client';

import React from 'react';
import { Card, Tag, Button, Dropdown } from 'antd';
import {
    SettingOutlined,
    DeleteOutlined,
    HeartOutlined,
    StarOutlined,
    MoreOutlined,
    CloudOutlined,
    AliyunOutlined,
    AmazonOutlined,
    AppstoreOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';
import HealthStatus from './HealthStatus';

interface StrategyCardProps {
    strategy: API.StorageStrategy;
    canEditStrategy: boolean;
    canDeleteStrategy: boolean;
    onEdit: () => void;
    onDelete: () => void;
    onSetDefault: () => void;
    onHealthCheck: () => void;
}

// 获取云服务商图标
const getProviderIcon = (provider: string) => {
    switch (provider) {
        case 'ALIYUN_OSS':
            return <AliyunOutlined className="text-orange-500" />;
        case 'AWS_S3':
            return <AmazonOutlined className="text-yellow-600" />;
        case 'TENCENT_COS':
            return <CloudOutlined className="text-blue-500" />;
        default:
            return <AppstoreOutlined className="text-gray-500" />;
    }
};

// 获取云服务商名称
const getProviderName = (provider: string) => {
    const names: Record<string, string> = {
        'LOCAL': '本地存储',
        'ALIYUN_OSS': '阿里云 OSS',
        'AWS_S3': 'AWS S3 / MinIO',
        'TENCENT_COS': '腾讯云 COS',
    };
    return names[provider] || provider;
};

const StrategyCard: React.FC<StrategyCardProps> = ({
    strategy,
    canEditStrategy,
    canDeleteStrategy,
    onEdit,
    onDelete,
    onSetDefault,
    onHealthCheck,
}) => {
    const isDefault = strategy.isDefault;
    const isEnabled = strategy.isEnabled;
    const runState = (strategy.runState || '').toUpperCase();
    const stateTag =
        runState === 'ACTIVE'
            ? { color: 'success', label: '运行中' }
            : runState === 'DRAINING'
              ? { color: 'processing', label: '排空中' }
              : { color: isEnabled ? 'warning' : 'default', label: '已停用' };
    const updateTimeLabel = strategy.updateTime
        ? new Date(strategy.updateTime).toLocaleDateString()
        : '暂无';
    const deleteDisabledReason = isDefault
        ? '默认策略不可删除'
        : isEnabled
          ? '请先停用策略'
          : runState !== 'DISABLED'
            ? '仅 DISABLED 策略允许删除'
            : undefined;
    const menuItems = [
        ...(canEditStrategy
            ? [
                  {
                      key: 'edit',
                      icon: <SettingOutlined />,
                      label: '编辑配置',
                      onClick: onEdit,
                  },
                  {
                      key: 'health',
                      icon: <HeartOutlined />,
                      label: '健康检查',
                      onClick: onHealthCheck,
                  },
                  ...(!isDefault
                      ? [
                            {
                                key: 'default',
                                icon: <StarOutlined />,
                                label: '设为默认',
                                onClick: onSetDefault,
                            },
                        ]
                      : []),
              ]
            : [
                  {
                      key: 'health',
                      icon: <HeartOutlined />,
                      label: '健康检查',
                      onClick: onHealthCheck,
                  },
              ]),
        ...(canDeleteStrategy
            ? [
                  ...(canEditStrategy ? [{ type: 'divider' as const }] : []),
                  {
                      key: 'delete',
                      icon: <DeleteOutlined />,
                      label: deleteDisabledReason ? `删除（${deleteDisabledReason}）` : '删除',
                      danger: true,
                      disabled: Boolean(deleteDisabledReason),
                      onClick: onDelete,
                  },
              ]
            : []),
    ];

    return (
        <motion.div whileHover={{ y: -4 }} transition={{ duration: 0.2 }}>
            <Card
                className={`
          relative overflow-hidden transition-all duration-300
          ${runState !== 'ACTIVE' ? 'opacity-70' : ''}
          hover:shadow-lg
          border border-slate-200/80 bg-white/95 shadow-[0_20px_45px_-34px_rgba(15,23,42,0.18)] hover:-translate-y-0.5 hover:shadow-[0_24px_48px_-34px_rgba(15,23,42,0.2)]
        `}
                styles={{ body: { padding: '16px' } }}
            >
                {/* 头部 */}
                <div className="flex items-start gap-3 mb-4">
                    <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-slate-200 bg-slate-50 text-2xl">
                        {getProviderIcon(strategy.providerType || '')}
                    </div>
                    <div className="flex-1 min-w-0">
                        <div className="font-medium text-gray-800 truncate">
                            {strategy.strategyName}
                        </div>
                        <div className="text-sm text-gray-500">
                            {getProviderName(strategy.providerType || '')}
                        </div>
                    </div>
                    {menuItems.length > 0 ? (
                        <Dropdown
                            menu={{
                                items: menuItems,
                            }}
                            trigger={['click']}
                        >
                            <Button type="text" size="small" icon={<MoreOutlined />} />
                        </Dropdown>
                    ) : null}
                </div>

                {/* 健康状态 */}
                <div className="mb-4 flex items-center justify-between gap-2">
                    <HealthStatus status={strategy.healthStatus} />
                    {isDefault ? <Tag color="blue"><StarOutlined /> 默认</Tag> : null}
                </div>

                {/* 统计信息 */}
                <div className="grid grid-cols-2 gap-3 text-sm">
                    <div className="rounded-2xl border border-slate-200 bg-slate-50/70 p-2 text-center">
                        <div className="text-gray-500">优先级</div>
                        <div className="font-semibold text-gray-800">{strategy.priority || 0}</div>
                    </div>
                    <div className="rounded-2xl border border-slate-200 bg-slate-50/70 p-2 text-center">
                        <div className="text-gray-500">故障率</div>
                        <div className="font-semibold text-gray-800">
                            {strategy.failureRateThreshold || 10}%
                        </div>
                    </div>
                </div>

                {/* 状态标签 */}
                <div className="mt-4 flex items-center justify-between">
                    <Tag color={stateTag.color}>{stateTag.label}</Tag>
                    <span className="text-xs text-gray-400">
                        更新于 {updateTimeLabel}
                    </span>
                </div>
            </Card>
        </motion.div>
    );
};

export default StrategyCard;
