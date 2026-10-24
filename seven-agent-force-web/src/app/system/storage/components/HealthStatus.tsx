'use client';

import React from 'react';
import { Tag } from 'antd';
import {
    CheckCircleOutlined,
    ExclamationCircleOutlined,
    CloseCircleOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';

interface HealthStatusProps {
    status?: number;
}

const statusConfig: Record<number, {
    label: string;
    color: string;
    icon: React.ReactNode;
    bgColor: string;
    pulseColor: string;
}> = {
    0: {
        label: '异常',
        color: 'error',
        icon: <CloseCircleOutlined />,
        bgColor: 'bg-red-500',
        pulseColor: 'bg-red-400',
    },
    1: {
        label: '健康',
        color: 'success',
        icon: <CheckCircleOutlined />,
        bgColor: 'bg-green-500',
        pulseColor: 'bg-green-400',
    },
    2: {
        label: '降级',
        color: 'warning',
        icon: <ExclamationCircleOutlined />,
        bgColor: 'bg-yellow-500',
        pulseColor: 'bg-yellow-400',
    },
};

const HealthStatus: React.FC<HealthStatusProps> = ({ status = 1 }) => {
    const config = statusConfig[status] || statusConfig[1];

    return (
        <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg">
            {/* 脉冲指示器 */}
            <div className="relative">
                <motion.div
                    className={`w-3 h-3 rounded-full ${config.bgColor}`}
                    animate={{ scale: [1, 1.2, 1] }}
                    transition={{ duration: 2, repeat: Infinity }}
                />
                {status === 1 && (
                    <motion.div
                        className={`absolute inset-0 w-3 h-3 rounded-full ${config.pulseColor}`}
                        animate={{ scale: [1, 2], opacity: [0.5, 0] }}
                        transition={{ duration: 2, repeat: Infinity }}
                    />
                )}
            </div>

            {/* 状态文字 */}
            <div className="flex-1">
                <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-gray-700">健康状态</span>
                    <Tag color={config.color} className="m-0">
                        {config.icon} {config.label}
                    </Tag>
                </div>
            </div>
        </div>
    );
};

export default HealthStatus;
