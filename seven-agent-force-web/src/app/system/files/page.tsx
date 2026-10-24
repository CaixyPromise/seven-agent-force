'use client';

import React, { useState } from 'react';
import { Card, Button, Space, message, Modal, Table, Tag, Typography } from 'antd';
import {
    FileOutlined,
    UploadOutlined,
    DeleteOutlined,
    ReloadOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';
import FileTable from './components/FileTable';
import FileFilter from './components/FileFilter';
import UploadModal from './components/UploadModal';
import FilePreview from './components/FilePreview';
import ReferenceDrawer from './components/ReferenceDrawer';
import { useFileManage } from './hooks/useFileManage';
import { HasPermission } from '@/components/Permission';
import { usePermissionFlags } from '@/hooks/auth';
import { FILE_PERMISSIONS } from '@/lib/auth/permissionCodes';
import type { FileBatchDeleteItem, FileBatchDeleteResult } from '@/api/fileManageController';

/**
 * 文件管理中心
 */
const FileManagePage: React.FC = () => {
    const [uploadModalOpen, setUploadModalOpen] = useState(false);
    const [previewFile, setPreviewFile] = useState<API.FileInfo | null>(null);
    const [referenceFile, setReferenceFile] = useState<API.FileInfo | null>(null);
    const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
    const [deleteFeedback, setDeleteFeedback] = useState<FileBatchDeleteResult | null>(null);

    const {
        fileList,
        loading,
        pagination,
        filters,
        stats,
        setFilters,
        setPagination,
        refreshList,
        deleteFiles,
        downloadFile,
        resolveFileUrl,
        resolveFileUrls,
    } = useFileManage();
    const { canQueryFiles, canDeleteFiles } = usePermissionFlags({
        canQueryFiles: FILE_PERMISSIONS.QUERY,
        canDeleteFiles: FILE_PERMISSIONS.DELETE,
    });

    const handlePreview = async (file: API.FileInfo) => {
        const fileUrl = await resolveFileUrl(file);
        setPreviewFile({
            ...file,
            fileUrl: fileUrl || file.fileUrl,
        });
        if (!fileUrl && !file.fileUrl) {
            message.warning('暂无可预览的链接');
        }
    };

    const presentDeleteFeedback = (result: FileBatchDeleteResult) => {
        if (result.outcome === 'FULL_SUCCESS') {
            return;
        }
        setDeleteFeedback(result);
    };

    const handleSingleDelete = async (id: API.Int64) => {
        const result = await deleteFiles([id]);
        presentDeleteFeedback(result);
    };

    const handleResolveVisibleUrls = async () => {
        const count = await resolveFileUrls(fileList);
        message.success(`已解析 ${count} 个可用链接`);
    };

    // 批量删除
    const handleBatchDelete = async () => {
        if (selectedRowKeys.length === 0) {
            message.warning('请先选择要删除的文件');
            return;
        }

        Modal.confirm({
            title: '确认删除',
            content: `确定要删除选中的 ${selectedRowKeys.length} 个文件吗？此操作不可恢复。`,
            okText: '删除',
            okType: 'danger',
            cancelText: '取消',
            onOk: async () => {
                const result = await deleteFiles(selectedRowKeys.map(String));
                if (result.deletedIds.length > 0) {
                    setSelectedRowKeys(prev =>
                        prev.filter(key => !result.deletedIds.includes(String(key))),
                    );
                }
                presentDeleteFeedback(result);
            },
        });
    };

    // 渲染统计卡片
    const renderStats = () => (
        <motion.div
            initial={{ opacity: 0, y: -20 }}
            animate={{ opacity: 1, y: 0 }}
            className="grid grid-cols-1 gap-4 mb-6 sm:grid-cols-2 xl:grid-cols-4"
        >
            {[
                { label: '总文件数', value: stats?.totalCount || 0, icon: <FileOutlined className="text-sky-600" /> },
                { label: '总大小', value: stats?.totalSizeFormatted || '0 KB', icon: <UploadOutlined className="text-sky-600" /> },
                { label: '图片', value: stats?.imageCount || 0, icon: <FileOutlined className="text-sky-600" /> },
                { label: '文档', value: stats?.docCount || 0, icon: <FileOutlined className="text-sky-600" /> },
            ].map((item, index) => (
                <motion.div
                    key={item.label}
                    initial={{ opacity: 0, scale: 0.9 }}
                    animate={{ opacity: 1, scale: 1 }}
                    transition={{ delay: index * 0.1 }}
                >
                    <Card
                        className="rounded-3xl border border-slate-200/80 bg-white/95 text-center shadow-[0_20px_45px_-32px_rgba(15,23,42,0.22)] transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_28px_55px_-34px_rgba(14,116,144,0.24)]"
                        styles={{ body: { padding: '16px' } }}
                    >
                        <div className="mb-3 flex items-center justify-center">
                            <div className="flex h-11 w-11 items-center justify-center rounded-2xl border border-sky-100 bg-sky-50/80">
                                {item.icon}
                            </div>
                        </div>
                        <div className="text-3xl font-bold tracking-tight text-slate-900">
                            {item.value}
                        </div>
                        <div className="mt-1 text-sm font-medium text-slate-500">{item.label}</div>
                    </Card>
                </motion.div>
            ))}
        </motion.div>
    );

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
                        <FileOutlined className="text-lg" />
                    </div>
                    <div>
                        <h1 className="text-xl font-semibold text-slate-900">文件列表</h1>
                        <p className="text-sm text-slate-500">集中查看系统中的文件资源与引用情况</p>
                    </div>
                </div>

                <Space>
                    <Button
                        icon={<ReloadOutlined />}
                        onClick={refreshList}
                        loading={loading}
                    >
                        刷新
                    </Button>
                    <HasPermission code={FILE_PERMISSIONS.QUERY}>
                        <Button
                            onClick={handleResolveVisibleUrls}
                            disabled={!fileList.length}
                            loading={loading}
                        >
                            解析链接
                        </Button>
                    </HasPermission>
                    <HasPermission code={FILE_PERMISSIONS.DELETE}>
                        {selectedRowKeys.length > 0 ? (
                            <Button
                                danger
                                icon={<DeleteOutlined />}
                                onClick={handleBatchDelete}
                            >
                                删除 ({selectedRowKeys.length})
                            </Button>
                        ) : null}
                    </HasPermission>
                    <Button
                        type="primary"
                        icon={<UploadOutlined />}
                        onClick={() => setUploadModalOpen(true)}

                    >
                        上传文件
                    </Button>
                </Space>
            </motion.div>

            {/* 统计卡片 */}
            {renderStats()}

            {/* 筛选区域 */}
            <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.2 }}
                className="mb-8"
            >
                <FileFilter
                    filters={filters}
                    onChange={setFilters}
                    onReset={() => setFilters({})}
                />
            </motion.div>

            {/* 文件表格 */}
            <motion.div
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.3 }}
            >
                <Card className="rounded-3xl border border-slate-200/80 bg-white/95 shadow-[0_20px_45px_-34px_rgba(15,23,42,0.2)]" styles={{ body: { padding: 0 } }}>
                    <FileTable
                        dataSource={fileList}
                        loading={loading}
                        canQueryFiles={canQueryFiles}
                        canDeleteFiles={canDeleteFiles}
                        pagination={pagination}
                        selectedRowKeys={selectedRowKeys}
                        onSelectChange={setSelectedRowKeys}
                        onPaginationChange={setPagination}
                        onPreview={handlePreview}
                        onViewReferences={setReferenceFile}
                        onDownload={downloadFile}
                        onDelete={handleSingleDelete}
                    />
                </Card>
            </motion.div>

            {/* 上传弹窗 */}
            <UploadModal
                open={uploadModalOpen}
                onClose={() => setUploadModalOpen(false)}
                onSuccess={() => {
                    setUploadModalOpen(false);
                    refreshList();
                }}
            />

            {/* 文件预览 */}
            <FilePreview
                file={previewFile}
                onClose={() => setPreviewFile(null)}
            />

            {/* 引用抽屉 */}
            <ReferenceDrawer
                file={referenceFile}
                onClose={() => setReferenceFile(null)}
            />

            <Modal
                open={!!deleteFeedback}
                title={deleteFeedback?.outcome === 'PARTIAL_SUCCESS' ? '批量删除部分成功' : '删除未完成'}
                onCancel={() => setDeleteFeedback(null)}
                onOk={() => setDeleteFeedback(null)}
                width={760}
                okText="我知道了"
                cancelButtonProps={{ style: { display: 'none' } }}
            >
                {deleteFeedback ? (
                    <div className="space-y-4">
                        <Typography.Paragraph className="mb-0 text-slate-600">
                            本次共请求删除 {deleteFeedback.requestedCount} 个文件，成功删除 {deleteFeedback.deletedCount} 个，
                            跳过 {deleteFeedback.skippedCount} 个。
                        </Typography.Paragraph>
                        <div className="flex gap-2">
                            <Tag color={deleteFeedback.outcome === 'PARTIAL_SUCCESS' ? 'gold' : 'red'}>
                                {deleteFeedback.outcome}
                            </Tag>
                            <Tag color="green">删除成功 {deleteFeedback.deletedCount}</Tag>
                            <Tag color="default">跳过 {deleteFeedback.skippedCount}</Tag>
                        </div>
                        <Table<FileBatchDeleteItem>
                            size="small"
                            pagination={false}
                            rowKey={(record, index) => `${record.fileId ?? 'unknown'}-${record.reason ?? 'unknown'}-${index ?? 0}`}
                            dataSource={deleteFeedback.skippedItems || []}
                            columns={[
                                {
                                    title: '文件 ID',
                                    dataIndex: 'fileId',
                                    key: 'fileId',
                                    width: 160,
                                    render: (value) => value ?? '-',
                                },
                                {
                                    title: '原因',
                                    dataIndex: 'reason',
                                    key: 'reason',
                                    width: 180,
                                    render: (value) => <Tag color="orange">{value || 'UNKNOWN'}</Tag>,
                                },
                                {
                                    title: '说明',
                                    dataIndex: 'message',
                                    key: 'message',
                                    render: (value) => value || '-',
                                },
                            ]}
                            locale={{ emptyText: '没有跳过明细' }}
                        />
                    </div>
                ) : null}
            </Modal>
        </div>
    );
};

export default FileManagePage;
