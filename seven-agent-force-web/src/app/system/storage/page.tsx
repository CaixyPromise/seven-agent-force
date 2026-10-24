'use client';

import React, { useState, useEffect } from 'react';
import { Button, Space, message, Modal, Empty, Spin } from 'antd';
import {
    PlusOutlined,
    ReloadOutlined,
    CloudServerOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';
import StrategyCard from './components/StrategyCard';
import StrategyForm from './components/StrategyForm';
import {
    getStorageStrategies,
    getStorageStrategy,
    deleteStorageStrategy,
    setDefaultStrategy,
    checkStorageHealth
} from '@/api/storageStrategyController';
import { HasPermission } from '@/components/Permission';
import { usePermissionFlags } from '@/hooks/auth';
import { FILE_PERMISSIONS } from '@/lib/auth/permissionCodes';

/**
 * 存储策略管理页面
 */
const StorageStrategyPage: React.FC = () => {
    const [strategies, setStrategies] = useState<API.StorageStrategy[]>([]);
    const [loading, setLoading] = useState(false);
    const [formOpen, setFormOpen] = useState(false);
    const [editingStrategy, setEditingStrategy] = useState<API.StorageStrategy | null>(null);
    const { canEditStrategy, canDeleteStrategy } = usePermissionFlags({
        canEditStrategy: FILE_PERMISSIONS.STORAGE_EDIT,
        canDeleteStrategy: FILE_PERMISSIONS.STORAGE_DELETE,
    });

    const extractErrorMessage = (error: unknown, fallback: string) => {
        const maybeMessage = (error as { info?: { errorMessage?: string }; response?: { data?: { message?: string } }; message?: string }) || {};
        return maybeMessage.info?.errorMessage
            || maybeMessage.response?.data?.message
            || maybeMessage.message
            || fallback;
    };

    // 获取策略列表
    const fetchStrategies = async () => {
        setLoading(true);
        try {
            const res = await getStorageStrategies();
            if (res.code === 0) {
                setStrategies(res.data?.records ?? res.data?.list ?? []);
            }
        } catch {
            message.error('获取策略列表失败');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        fetchStrategies();
    }, []);

    // 删除策略
    const handleDelete = (id: API.Int64) => {
        Modal.confirm({
            title: '确认删除',
            content: '确定要删除该存储策略吗？删除后不可恢复。',
            okText: '删除',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
                try {
                    const res = await deleteStorageStrategy(id);
                    if (res.code === 0) {
                        message.success('删除成功');
                        fetchStrategies();
                    } else {
                        message.error(res.message || '删除失败');
                    }
                } catch (error) {
                    message.error(extractErrorMessage(error, '删除失败'));
                }
            },
        });
    };

    // 设为默认
    const handleSetDefault = async (id: API.Int64) => {
        try {
            const res = await setDefaultStrategy(id);
            if (res.code === 0) {
                message.success('已设为默认策略');
                fetchStrategies();
            }
        } catch {
            message.error('设置失败');
        }
    };

    // 健康检查
    const handleHealthCheck = async (id: API.Int64) => {
        try {
            const res = await checkStorageHealth(id);
            if (res.code === 0) {
                const { healthy, message: msg } = res.data || {};
                if (healthy) {
                    message.success('存储服务正常');
                } else {
                    message.warning(`存储异常: ${msg}`);
                }
                fetchStrategies();
            }
        } catch {
            message.error('健康检查失败');
        }
    };

    // 编辑策略
    const handleEdit = async (strategy: API.StorageStrategy) => {
        try {
            const res = await getStorageStrategy(strategy.id!);
            if (res.code === 0 && res.data) {
                setEditingStrategy(res.data);
            } else {
                setEditingStrategy(strategy);
                message.warning(res.message || '读取策略详情失败，已回退到列表快照');
            }
        } catch {
            setEditingStrategy(strategy);
            message.warning('读取策略详情失败，已回退到列表快照');
        }
        setFormOpen(true);
    };

    // 关闭表单
    const handleFormClose = () => {
        setFormOpen(false);
        setEditingStrategy(null);
    };

    // 表单提交成功
    const handleFormSuccess = () => {
        handleFormClose();
        fetchStrategies();
    };

    return (
        <div className="h-full bg-transparent p-4 md:p-6">
            {/* 页面标题 */}
            <motion.div
                initial={{ opacity: 0, x: -20 }}
                animate={{ opacity: 1, x: 0 }}
                className="flex items-center justify-between mb-6"
            >
                <div className="flex items-center gap-3">
                    <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-sky-100 bg-white text-sky-600 shadow-sm">
                        <CloudServerOutlined className="text-lg" />
                    </div>
                    <div>
                        <h1 className="text-xl font-semibold text-slate-900">存储策略</h1>
                        <p className="text-sm text-slate-500">配置和管理当前系统的存储策略</p>
                    </div>
                </div>

                <Space>
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={fetchStrategies}
                        loading={loading}
                    >
                        刷新
                    </Button>
                    <HasPermission code={FILE_PERMISSIONS.STORAGE_ADD}>
                        <Button
                            type="primary"
                            icon={<PlusOutlined />}
                            onClick={() => setFormOpen(true)}
                        >
                            添加策略
                        </Button>
                    </HasPermission>
                </Space>
            </motion.div>

            {/* 策略卡片网格 */}
            <Spin spinning={loading}>
                {strategies.length === 0 ? (
                    <motion.div
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        className="flex justify-center py-20"
                    >
                        <Empty
                            image={Empty.PRESENTED_IMAGE_SIMPLE}
                            description="暂无存储策略"
                        >
                            <HasPermission code={FILE_PERMISSIONS.STORAGE_ADD}>
                                <Button
                                    type="primary"
                                    icon={<PlusOutlined />}
                                    onClick={() => setFormOpen(true)}
                                >
                                    添加第一个策略
                                </Button>
                            </HasPermission>
                        </Empty>
                    </motion.div>
                ) : (
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                        {strategies.map((strategy, index) => (
                            <motion.div
                                key={strategy.id}
                                initial={{ opacity: 0, y: 20 }}
                                animate={{ opacity: 1, y: 0 }}
                                transition={{ delay: index * 0.1 }}
                            >
                                <StrategyCard
                                    strategy={strategy}
                                    canEditStrategy={canEditStrategy}
                                    canDeleteStrategy={canDeleteStrategy}
                                    onEdit={() => handleEdit(strategy)}
                                    onDelete={() => handleDelete(strategy.id!)}
                                    onSetDefault={() => handleSetDefault(strategy.id!)}
                                    onHealthCheck={() => handleHealthCheck(strategy.id!)}
                                />
                            </motion.div>
                        ))}
                    </div>
                )}
            </Spin>

            {/* 策略表单抽屉 */}
            <StrategyForm
                open={formOpen}
                strategy={editingStrategy}
                onClose={handleFormClose}
                onSuccess={handleFormSuccess}
            />
        </div>
    );
};

export default StorageStrategyPage;
