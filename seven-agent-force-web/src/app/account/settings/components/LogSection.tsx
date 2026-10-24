'use client';

import React, { useState } from 'react';
import { Card, Table, Tag } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { useQuery } from '@tanstack/react-query';
import { getMyOperationLogPage } from '@/api/operationLogController';

interface OperationLogRow {
    id?: string | number;
    operationType?: string;
    operationTypeDesc?: string;
    operationTypeLabel?: string;
    operationDesc?: string;
    requestMethod?: string;
    requestUrl?: string;
    requestIp?: string;
    status?: number;
    operationTime?: string;
    createTime?: string;
}

interface OperationLogPageData {
    records: OperationLogRow[];
    total: number;
}

function formatDateTime(raw?: string): string {
    if (!raw) return '-';
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return raw;
    const pad = (v: number) => (v < 10 ? `0${v}` : v);
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export default function LogSection() {
    const [pagination, setPagination] = useState({ current: 1, pageSize: 10 });

    const { data, isLoading } = useQuery({
        queryKey: ['account-settings', 'my-logs', pagination.current, pagination.pageSize],
        queryFn: async (): Promise<OperationLogPageData> => {
            const response = await getMyOperationLogPage({
                current: pagination.current,
                size: pagination.pageSize,
            });
            const resData = response.data;
            return {
                records: Array.isArray(resData?.records) ? resData.records : [],
                total: Number(resData?.total ?? 0),
            };
        },
    });

    const columns: ColumnsType<OperationLogRow> = [
        {
            title: '操作时间',
            dataIndex: 'operationTime',
            key: 'operationTime',
            width: '32%',
            render: (_, record) => formatDateTime(record.operationTime || record.createTime),
        },
        {
            title: '操作类型',
            dataIndex: 'operationType',
            key: 'operationType',
            width: '28%',
            render: (_, record) => <Tag>{record.operationTypeLabel || record.operationTypeDesc || record.operationType || '未知操作'}</Tag>,
        },
        {
            title: 'IP 地址',
            dataIndex: 'requestIp',
            key: 'requestIp',
            width: '25%',
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: '15%',
            render: (val) => (
                <Tag color={val === 1 ? 'success' : 'error'}>
                    {val === 1 ? '成功' : '失败'}
                </Tag>
            ),
        },
    ];

    return (
        <Card
            variant="borderless"
            className="shadow-sm rounded-xl overflow-hidden"
            title="我的操作日志"
        >
            <Table<OperationLogRow>
                rowKey="id"
                columns={columns}
                dataSource={data?.records || []}
                loading={isLoading}
                tableLayout="fixed"
                pagination={{
                    current: pagination.current,
                    pageSize: pagination.pageSize,
                    total: data?.total || 0,
                    showSizeChanger: true,
                    onChange: (current, pageSize) => setPagination({ current, pageSize }),
                }}
            />
        </Card>
    );
}
