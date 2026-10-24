'use client';

import React from 'react';
import { Card } from 'antd';
import {
    ClockCircleOutlined,
    SyncOutlined,
    CheckCircleOutlined,
    CloseCircleOutlined,
} from '@ant-design/icons';
import { motion } from 'framer-motion';

interface TaskStatsProps {
    stats: {
        pending: number;
        processing: number;
        completed: number;
        failed: number;
        pendingRetry?: number;
    } | null;
    onFilterChange: (status: number | undefined) => void;
}

const TaskStats: React.FC<TaskStatsProps> = ({ stats, onFilterChange }) => {
    const items = [
        {
            key: undefined,
            label: '全部任务',
            value:
                (stats?.pending || 0)
                + (stats?.processing || 0)
                + (stats?.completed || 0)
                + (stats?.failed || 0)
                + (stats?.pendingRetry || 0),
            icon: <SyncOutlined />,
            iconTone: 'text-slate-500',
        },
        {
            key: 0,
            label: '待处理',
            value: stats?.pending || 0,
            icon: <ClockCircleOutlined />,
            iconTone: 'text-sky-600',
        },
        {
            key: 1,
            label: '处理中',
            value: stats?.processing || 0,
            icon: <SyncOutlined spin />,
            iconTone: 'text-amber-600',
        },
        {
            key: 2,
            label: '已完成',
            value: stats?.completed || 0,
            icon: <CheckCircleOutlined />,
            iconTone: 'text-emerald-600',
        },
        {
            key: 3,
            label: '失败',
            value: stats?.failed || 0,
            icon: <CloseCircleOutlined />,
            iconTone: 'text-rose-600',
        },
        {
            key: 4,
            label: '待补偿',
            value: stats?.pendingRetry || 0,
            icon: <SyncOutlined />,
            iconTone: 'text-yellow-600',
        },
    ];

    return (
        <div className="mb-6 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
            {items.map((item, index) => (
                <motion.div
                    key={item.label}
                    initial={{ opacity: 0, scale: 0.9 }}
                    animate={{ opacity: 1, scale: 1 }}
                    transition={{ delay: index * 0.1 }}
                    whileHover={{ scale: 1.02 }}
                    onClick={() => onFilterChange(item.key)}
                    className="cursor-pointer"
                >
                    <Card
                        className="rounded-3xl border border-slate-200/80 bg-white/95 transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_24px_48px_-32px_rgba(15,23,42,0.18)]"
                        styles={{ body: { padding: '16px' } }}
                    >
                        <div className="flex items-center justify-between gap-3">
                            <div>
                                <div className="text-3xl font-bold text-slate-900">{item.value}</div>
                                <div className="mt-1 text-sm text-slate-500">{item.label}</div>
                            </div>
                            <div className={`flex h-11 w-11 items-center justify-center rounded-2xl border border-sky-100 bg-sky-50/70 text-2xl ${item.iconTone}`}>
                                {item.icon}
                            </div>
                        </div>
                    </Card>
                </motion.div>
            ))}
        </div>
    );
};

export default TaskStats;
