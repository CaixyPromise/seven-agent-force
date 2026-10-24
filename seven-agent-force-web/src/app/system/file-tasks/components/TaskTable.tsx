'use client';

import React from 'react';
import { Table, Tag, Button, Tooltip, Progress } from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import {
    ReloadOutlined,
    FileImageOutlined,
    CompressOutlined,
    RedoOutlined
} from '@ant-design/icons';
import dayjs from 'dayjs';

interface TaskTableProps {
    dataSource: API.FileProcessTask[];
    loading: boolean;
    canRetryTasks: boolean;
    pagination: TablePaginationConfig;
    selectedRowKeys: React.Key[];
    onSelectChange: (keys: React.Key[]) => void;
    onPaginationChange: (pagination: TablePaginationConfig) => void;
    onRetry: (id: API.Int64) => void;
    onReplay: (id: API.Int64) => void;
}

const statusConfig: Record<number, { label: string; color: string }> = {
    0: { label: '待处理', color: 'default' },
    1: { label: '处理中', color: 'processing' },
    2: { label: '已完成', color: 'success' },
    3: { label: '失败', color: 'error' },
    4: { label: '待补偿', color: 'warning' },
};

const taskTypeConfig: Record<string, { label: string; icon: React.ReactNode }> = {
    'THUMBNAIL': { label: '缩略图生成', icon: <FileImageOutlined /> },
    'COMPRESS': { label: '文件压缩', icon: <CompressOutlined /> },
};

const TaskTable: React.FC<TaskTableProps> = ({
    dataSource,
    loading,
    canRetryTasks,
    pagination,
    selectedRowKeys,
    onSelectChange,
    onPaginationChange,
    onRetry,
    onReplay,
}) => {
    const columns: ColumnsType<API.FileProcessTask> = [
        {
            title: '任务ID',
            dataIndex: 'id',
            key: 'id',
            width: 80,
            render: (id) => <span className="font-mono text-xs">#{id}</span>,
        },
        {
            title: '任务类型',
            dataIndex: 'taskType',
            key: 'taskType',
            width: 130,
            render: (type) => {
                const config = taskTypeConfig[type] || { label: type, icon: null };
                return (
                    <div className="flex items-center gap-2">
                        {config.icon}
                        <span>{config.label}</span>
                    </div>
                );
            },
        },
        {
            title: '文件ID',
            dataIndex: 'fileId',
            key: 'fileId',
            width: 80,
            render: (id) => <span className="text-blue-500">#{id}</span>,
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: 100,
            render: (status) => {
                const config = statusConfig[status] || statusConfig[0];
                return <Tag color={config.color}>{config.label}</Tag>;
            },
        },
        {
            title: '重试次数',
            dataIndex: 'retryCount',
            key: 'retryCount',
            width: 100,
            render: (count) => (
                <div className="flex items-center gap-2">
                    <Progress
                        percent={((count || 0) / 3) * 100}
                        steps={3}
                        size="small"
                        showInfo={false}
                        strokeColor={count >= 3 ? '#ff4d4f' : '#1890ff'}
                    />
                    <span className="text-xs text-gray-500">{count || 0}/3</span>
                </div>
            ),
        },
        {
            title: '尝试',
            dataIndex: 'attempt',
            key: 'attempt',
            width: 80,
            render: (attempt) => attempt ?? 0,
        },
        {
            title: '创建时间',
            dataIndex: 'createTime',
            key: 'createTime',
            width: 160,
            render: (time) => dayjs(time).format('YYYY-MM-DD HH:mm:ss'),
        },
        {
            title: '完成时间',
            dataIndex: 'finishTime',
            key: 'finishTime',
            width: 160,
            render: (time) => (time ? dayjs(time).format('YYYY-MM-DD HH:mm:ss') : '-'),
        },
        {
            title: '错误信息',
            dataIndex: 'errorMsg',
            key: 'errorMsg',
            width: 200,
            ellipsis: { showTitle: false },
            render: (msg) => (
                msg ? (
                    <Tooltip title={msg}>
                        <span className="text-red-500 cursor-help">{msg}</span>
                    </Tooltip>
                ) : '-'
            ),
        },
        {
            title: '操作',
            key: 'action',
            width: 100,
            fixed: 'right',
            render: (_, record) => (
                canRetryTasks ? (
                    <div className="flex items-center gap-1">
                        <Button
                            type="link"
                            size="small"
                            icon={<ReloadOutlined />}
                            onClick={() => onRetry(record.id!)}
                            disabled={record.status === 1 || record.status === 2}
                        >
                            重试
                        </Button>
                        <Button
                            type="link"
                            size="small"
                            icon={<RedoOutlined />}
                            onClick={() => onReplay(record.id!)}
                        >
                            重放
                        </Button>
                    </div>
                ) : (
                    <span className="text-slate-400">-</span>
                )
            ),
        },
    ];

    return (
        <Table
            rowKey="id"
            columns={columns}
            dataSource={dataSource}
            loading={loading}
            pagination={{
                ...pagination,
                showSizeChanger: true,
                showQuickJumper: true,
                showTotal: (total) => `共 ${total} 个任务`,
            }}
            rowSelection={
                canRetryTasks
                    ? {
                          selectedRowKeys,
                          onChange: onSelectChange,
                          getCheckboxProps: (record) => ({
                              disabled: record.status === 1 || record.status === 2,
                          }),
                      }
                    : undefined
            }
            onChange={(p) => onPaginationChange(p)}
            scroll={{ x: 1200 }}
        />
    );
};

export default TaskTable;
