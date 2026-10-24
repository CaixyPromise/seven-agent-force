'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Card, Button, Space, message } from 'antd';
import {
    ReloadOutlined,
    ThunderboltOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';
import TaskTable from './components/TaskTable';
import TaskStats from './components/TaskStats';
import {
    getFileProcessTasks,
    getFileProcessTaskStats,
    retryFileProcessTask,
    batchRetryTasks,
    replayFileProcessTask
} from '@/api/fileProcessTaskController';
import { usePermissionFlags } from '@/hooks/auth';
import { FILE_PERMISSIONS } from '@/lib/auth/permissionCodes';

/**
 * 文件处理任务监控页面
 */
const FileTasksPage: React.FC = () => {
    const [tasks, setTasks] = useState<API.FileProcessTask[]>([]);
    const [stats, setStats] = useState<{
        pending: number;
        processing: number;
        completed: number;
        failed: number;
        pendingRetry: number;
    } | null>(null);
    const [loading, setLoading] = useState(false);
    const [statusFilter, setStatusFilter] = useState<number | undefined>();
    const [pagination, setPagination] = useState({ current: 1, pageSize: 10, total: 0 });
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const current = pagination.current ?? 1;
    const pageSize = pagination.pageSize ?? 10;
    const { canRetryTasks } = usePermissionFlags({
        canRetryTasks: FILE_PERMISSIONS.TASK_RETRY,
    });

    // 获取任务列表
    const fetchTasks = useCallback(async () => {
        setLoading(true);
        try {
            const res = await getFileProcessTasks({
                current,
                pageSize,
                status: statusFilter,
            });
            if (res.code === 0) {
                setTasks(res.data?.records || []);
                setPagination(prev => ({ ...prev, total: res.data?.total || 0 }));
            }
        } catch {
            message.error('获取任务列表失败');
        } finally {
            setLoading(false);
        }
    }, [current, pageSize, statusFilter]);

    // 获取统计数据
    const fetchStats = useCallback(async () => {
        try {
            const res = await getFileProcessTaskStats();
            if (res.code === 0) {
                setStats(normalizeTaskStats(res.data));
            }
        } catch (error) {
            console.error('Failed to fetch stats:', error);
        }
    }, []);

    useEffect(() => {
        fetchTasks();
        fetchStats();
    }, [fetchTasks, fetchStats]);

    // 重试单个任务
    const handleRetry = async (id: API.Int64) => {
        try {
            const res = await retryFileProcessTask(id);
            if (res.code === 0) {
                message.success('已重新加入处理队列');
                fetchTasks();
                fetchStats();
            }
        } catch {
            message.error('重试失败');
        }
    };

    // 批量重试
    const handleBatchRetry = async () => {
        if (selectedRowKeys.length === 0) {
            message.warning('请先选择任务');
            return;
        }

        try {
            const res = await batchRetryTasks(selectedRowKeys.map(String));
            if (res.code === 0) {
                message.success(`${selectedRowKeys.length} 个任务已重新加入队列`);
                setSelectedRowKeys([]);
                fetchTasks();
                fetchStats();
            }
        } catch {
            message.error('批量重试失败');
        }
    };

    // 刷新
    const handleRefresh = () => {
        fetchTasks();
        fetchStats();
    };

    const handleReplay = async (id: API.Int64) => {
        try {
            const res = await replayFileProcessTask(id);
            if (res.code === 0) {
                message.success('任务已重放并入队');
                fetchTasks();
                fetchStats();
            }
        } catch {
            message.error('重放失败');
        }
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
                        <ThunderboltOutlined className="text-lg" />
                    </div>
                    <div>
                        <h1 className="text-xl font-semibold text-slate-900">任务处理</h1>
                        <p className="text-sm text-slate-500">查看文件异步处理链路与重试状态</p>
                    </div>
                </div>

                <Space>
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={handleRefresh}
                        loading={loading}
                    >
                        刷新
                    </Button>
                    {canRetryTasks && selectedRowKeys.length > 0 && (
                        <Button
                            type="primary"
                            onClick={handleBatchRetry}

                        >
                            批量重试 ({selectedRowKeys.length})
                        </Button>
                    )}
                </Space>
            </motion.div>

            {/* 统计卡片 */}
            <TaskStats stats={stats} onFilterChange={setStatusFilter} />

            {/* 任务表格 */}
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.2 }}
            >
                <Card className="rounded-3xl border border-slate-200/80 bg-white/95 shadow-[0_20px_45px_-34px_rgba(15,23,42,0.2)]" styles={{ body: { padding: 0 } }}>
                    <TaskTable
                        dataSource={tasks}
                        loading={loading}
                        canRetryTasks={canRetryTasks}
                        pagination={pagination}
                        selectedRowKeys={selectedRowKeys}
                        onSelectChange={setSelectedRowKeys}
                        onPaginationChange={(p) => setPagination(prev => ({ ...prev, ...p }))}
                        onRetry={handleRetry}
                        onReplay={handleReplay}
                    />
                </Card>
            </motion.div>
        </div>
    );
};

export default FileTasksPage;

function normalizeTaskStats(stats?: API.TaskStatsResponse) {
    if (!stats) {
        return null;
    }
    return {
        pending: toNumber(stats.pending),
        processing: toNumber(stats.processing),
        completed: toNumber(stats.completed),
        failed: toNumber(stats.failed),
        pendingRetry: toNumber(stats.pendingRetry),
    };
}

function toNumber(value: number | string | undefined | null): number {
    if (typeof value === 'number') {
        return Number.isFinite(value) ? value : 0;
    }
    if (typeof value === 'string') {
        const parsed = Number(value);
        return Number.isFinite(parsed) ? parsed : 0;
    }
    return 0;
}
