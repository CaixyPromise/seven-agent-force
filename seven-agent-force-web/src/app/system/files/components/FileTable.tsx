'use client';

import React from 'react';
import { Table, Tag, Button, Dropdown, Tooltip, Image, Space, message } from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import {
    EyeOutlined,
    DownloadOutlined,
    DeleteOutlined,
    LinkOutlined,
    MoreOutlined,
    FileImageOutlined,
    FileTextOutlined,
    VideoCameraOutlined,
    FileOutlined,
    FilePdfOutlined,
    FileZipOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';
import dayjs from 'dayjs';

interface FileTableProps {
    dataSource: API.FileInfo[];
    loading: boolean;
    canQueryFiles: boolean;
    canDeleteFiles: boolean;
    pagination: TablePaginationConfig;
    selectedRowKeys: React.Key[];
    onSelectChange: (keys: React.Key[]) => void;
    onPaginationChange: (pagination: TablePaginationConfig) => void;
    onPreview: (file: API.FileInfo) => void;
    onViewReferences: (file: API.FileInfo) => void;
    onDownload: (file: API.FileInfo) => void;
    onDelete: (id: API.Int64) => void;
}

// 文件类型图标映射
const getFileIcon = (fileType: string) => {
    if (fileType?.startsWith('image')) return <FileImageOutlined className="text-purple-500" />;
    if (fileType?.startsWith('video')) return <VideoCameraOutlined className="text-blue-500" />;
    if (fileType?.includes('pdf')) return <FilePdfOutlined className="text-red-500" />;
    if (fileType?.includes('zip') || fileType?.includes('rar')) return <FileZipOutlined className="text-yellow-500" />;
    if (fileType?.includes('text') || fileType?.includes('document')) return <FileTextOutlined className="text-green-500" />;
    return <FileOutlined className="text-gray-500" />;
};

// 格式化文件大小
const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

const FileTable: React.FC<FileTableProps> = ({
    dataSource,
    loading,
    canQueryFiles,
    canDeleteFiles,
    pagination,
    selectedRowKeys,
    onSelectChange,
    onPaginationChange,
    onPreview,
    onViewReferences,
    onDownload,
    onDelete,
}) => {
    const columns: ColumnsType<API.FileInfo> = [
        {
            title: '文件',
            dataIndex: 'fileName',
            key: 'fileName',
            width: 300,
            render: (_, record) => (
                <div className="flex items-center gap-3">
                    {/* 缩略图或图标 */}
                    <div className="w-10 h-10 rounded-lg bg-gray-100 flex items-center justify-center overflow-hidden">
                        {record.contentType?.startsWith('image') && record.fileUrl ? (
                            <Image
                                src={record.fileUrl}
                                alt={record.fileName}
                                className="w-full h-full object-cover"
                                preview={false}
                                fallback="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
                            />
                        ) : (
                            <span className="text-xl">{getFileIcon(record.contentType || '')}</span>
                        )}
                    </div>

                    {/* 文件信息 */}
                    <div className="flex-1 min-w-0">
                        <Tooltip title={record.fileName}>
                            <div className="font-medium text-gray-800 truncate">{record.fileName}</div>
                        </Tooltip>
                        <div className="text-xs text-gray-400">{record.fileSha256?.slice(0, 16)}...</div>
                    </div>
                </div>
            ),
        },
        {
            title: '类型',
            dataIndex: 'contentType',
            key: 'contentType',
            width: 100,
            render: (type) => {
                const shortType = type?.split('/')[1]?.toUpperCase() || 'UNKNOWN';
                return (
                    <Tag color="blue" className="rounded-full">
                        {shortType}
                    </Tag>
                );
            },
        },
        {
            title: '大小',
            dataIndex: 'fileSize',
            key: 'fileSize',
            width: 100,
            render: (size) => formatFileSize(size || 0),
        },
        {
            title: '引用',
            dataIndex: 'referenceCount',
            key: 'referenceCount',
            width: 80,
            render: (count, record) => (
                <motion.div whileHover={{ scale: 1.1 }}>
                    {canQueryFiles ? (
                        <Button
                            type="link"
                            size="small"
                            onClick={() => onViewReferences(record)}
                            className="font-semibold"
                        >
                            {typeof count === 'number' ? count : '查看'}
                        </Button>
                    ) : (
                        <span className="font-semibold text-slate-400">
                            {typeof count === 'number' ? count : '-'}
                        </span>
                    )}
                </motion.div>
            ),
        },
        {
            title: '上传时间',
            dataIndex: 'createTime',
            key: 'createTime',
            width: 160,
            render: (time) => dayjs(time).format('YYYY-MM-DD HH:mm'),
        },
        {
            title: '状态',
            dataIndex: 'status',
            key: 'status',
            width: 100,
            render: (status) => {
                const statusValue = typeof status === 'string' ? status : '';
                const map: Record<string, { color: string; label: string }> = {
                    AVAILABLE: { color: 'success', label: '可用' },
                    QUARANTINED: { color: 'warning', label: '隔离' },
                    DELETED: { color: 'default', label: '已删除' },
                    BROKEN: { color: 'error', label: '损坏' },
                };
                const info = map[statusValue] || { color: 'default', label: statusValue || '未知' };
                return (
                    <Tag color={info.color}>
                        {info.label}
                    </Tag>
                );
            },
        },
        {
            title: '扫描',
            dataIndex: 'scanStatus',
            key: 'scanStatus',
            width: 100,
            render: (status) => {
                const statusValue = typeof status === 'string' ? status : '';
                const map: Record<string, { color: string; label: string }> = {
                    PENDING: { color: 'warning', label: '待扫描' },
                    CLEAN: { color: 'success', label: '安全' },
                    INFECTED: { color: 'error', label: '感染' },
                    MIME_REJECTED: { color: 'error', label: 'MIME拒绝' },
                    DLP_REJECTED: { color: 'error', label: 'DLP拒绝' },
                    SCAN_TIMEOUT: { color: 'warning', label: '扫描超时' },
                    ERROR: { color: 'default', label: '异常' },
                };
                const info = map[statusValue] || { color: 'default', label: statusValue || '未知' };
                return (
                    <Tag color={info.color}>
                        {info.label}
                    </Tag>
                );
            },
        },
        {
            title: '完整性',
            dataIndex: 'integrityStatus',
            key: 'integrityStatus',
            width: 120,
            render: (status) => {
                const map: Record<string, { color: string; label: string }> = {
                    VERIFIED: { color: 'success', label: '已校验' },
                    PENDING: { color: 'warning', label: '待校验' },
                    HASH_MISMATCH: { color: 'error', label: '哈希不匹配' },
                    CRC_MISMATCH: { color: 'error', label: 'CRC不匹配' },
                    ERROR: { color: 'default', label: '异常' },
                };
                const info = map[String(status)] || { color: 'default', label: status || '-' };
                return <Tag color={info.color}>{info.label}</Tag>;
            },
        },
        {
            title: '分发',
            dataIndex: 'distributionMode',
            key: 'distributionMode',
            width: 90,
            render: (mode) => mode || '-',
        },
        {
            title: '操作',
            key: 'action',
            width: 120,
            fixed: 'right',
            render: (_, record) => (
                <Space>
                    {canQueryFiles ? (
                        <>
                            <Tooltip title="预览">
                                <Button
                                    type="text"
                                    size="small"
                                    icon={<EyeOutlined />}
                                    onClick={() => onPreview(record)}
                                />
                            </Tooltip>
                            <Tooltip title="下载">
                                <Button
                                    type="text"
                                    size="small"
                                    icon={<DownloadOutlined />}
                                    onClick={() => onDownload(record)}
                                />
                            </Tooltip>
                        </>
                    ) : null}
                    {canQueryFiles || canDeleteFiles ? (
                        <Dropdown
                            menu={{
                                items: [
                                    ...(canQueryFiles
                                        ? [
                                            {
                                                key: 'copyLink',
                                                icon: <LinkOutlined />,
                                                label: '复制链接',
                                                onClick: () => {
                                                    if (!record.fileUrl) {
                                                        message.warning('暂无可复制的链接');
                                                        return;
                                                    }
                                                    navigator.clipboard.writeText(record.fileUrl);
                                                },
                                            },
                                          ]
                                        : []),
                                    ...(canQueryFiles && canDeleteFiles ? [{ type: 'divider' as const }] : []),
                                    ...(canDeleteFiles
                                        ? [
                                            {
                                                key: 'delete',
                                                icon: <DeleteOutlined />,
                                                label: '删除',
                                                danger: true,
                                                onClick: () => onDelete(record.id!),
                                            },
                                          ]
                                        : []),
                                ],
                            }}
                            trigger={['click']}
                        >
                            <Button type="text" size="small" icon={<MoreOutlined />} />
                        </Dropdown>
                    ) : null}
                </Space>
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
                showTotal: (total) => `共 ${total} 个文件`,
            }}
            rowSelection={{
                selectedRowKeys,
                onChange: onSelectChange,
            }}
            onChange={(p) => onPaginationChange(p)}
            scroll={{ x: 1000 }}
            className="file-table"
        />
    );
};

export default FileTable;
