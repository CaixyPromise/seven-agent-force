'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Drawer, Tag, Spin, Empty, Button, Select, message } from 'antd';
import { LinkOutlined, ExportOutlined } from '@ant-design/icons';
import { motion } from 'framer-motion';
import { getFileReferences, updateReferenceAccessLevel } from '@/api/fileManageController';

interface ReferenceDrawerProps {
    file: API.FileInfo | null;
    onClose: () => void;
}

const ReferenceDrawer: React.FC<ReferenceDrawerProps> = ({ file, onClose }) => {
    const [loading, setLoading] = useState(false);
    const [references, setReferences] = useState<API.FileReference[]>([]);

    const fetchReferences = useCallback(async (isCurrent: () => boolean = () => true) => {
        if (!file?.id) return;

        setLoading(true);
        try {
            const res = await getFileReferences(file.id);
            if (isCurrent() && res.code === 0) {
                setReferences(res.data || []);
            }
        } catch (error) {
            if (isCurrent()) {
                console.error('Failed to fetch references:', error);
            }
        } finally {
            if (isCurrent()) {
                setLoading(false);
            }
        }
    }, [file?.id]);

    useEffect(() => {
        if (!file?.id) {
            return;
        }
        let active = true;
        const timer = window.setTimeout(() => {
            void fetchReferences(() => active);
        }, 0);
        return () => {
            active = false;
            window.clearTimeout(timer);
        };
    }, [fetchReferences, file?.id]);

    const handleAccessLevelChange = async (referenceId: API.Int64, accessLevel: number) => {
        try {
            const res = await updateReferenceAccessLevel(referenceId, accessLevel);
            if (res.code === 0) {
                await fetchReferences();
                message.success('访问级别已更新');
            } else {
                message.error(res.message || '更新失败');
            }
        } catch {
            message.error('更新失败');
        }
    };

    const getAccessLevel = (item: API.FileReference) => {
        if (typeof item.accessLevel === 'number') {
            return item.accessLevel;
        }
        return item.accessScope === 'PUBLIC' ? 1 : 0;
    };

    const getBizTypeColor = (bizType?: string) => {
        const colors: Record<string, string> = {
            'AVATAR': 'blue',
            'ATTACHMENT': 'green',
            'RESOURCE': 'orange',
            'POST': 'purple',
        };
        return bizType ? colors[bizType] || 'default' : 'default';
    };

    return (
        <Drawer
            title={
                <div className="flex items-center gap-2">
                    <LinkOutlined className="text-blue-500" />
                    <span>文件引用追踪</span>
                </div>
            }
            open={!!file}
            onClose={onClose}
            size={480}
            extra={
                <Tag color="blue">{references.length} 个引用</Tag>
            }
        >
            {file && (
                <div className="mb-4 p-3 bg-gray-50 rounded-lg">
                    <div className="font-medium text-gray-800 truncate">{file.fileName || file.fileInnerName}</div>
                    <div className="text-xs text-gray-400 mt-1">ID: {file.id}</div>
                </div>
            )}

            <Spin spinning={loading}>
                {references.length === 0 ? (
                    <Empty
                        description="暂无引用记录"
                        image={Empty.PRESENTED_IMAGE_SIMPLE}
                    />
                ) : (
                    <div className="space-y-3">
                        {references.map((item, index) => (
                            <motion.div
                                key={item.id}
                                initial={{ opacity: 0, x: -20 }}
                                animate={{ opacity: 1, x: 0 }}
                                transition={{ delay: index * 0.05 }}
                                className="rounded-xl border border-slate-200 bg-white px-4 py-3 shadow-sm transition-colors hover:bg-slate-50"
                            >
                                <div className="flex items-start justify-between gap-4">
                                    <div className="flex min-w-0 items-start gap-3">
                                        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-blue-50">
                                            <LinkOutlined className="text-blue-500" />
                                        </div>
                                        <div className="min-w-0">
                                            <div className="flex flex-wrap items-center gap-2">
                                                {item.bizType && (
                                                    <Tag color={getBizTypeColor(item.bizType)}>
                                                        {item.bizType}
                                                    </Tag>
                                                )}
                                                <span className="font-medium text-slate-800">
                                                    {item.displayName || '未命名引用'}
                                                </span>
                                            </div>
                                            <div className="mt-1 text-xs text-gray-400">
                                                业务ID: {item.bizId ?? '-'} · {item.createTime}
                                            </div>
                                        </div>
                                    </div>
                                    <div className="flex shrink-0 flex-col items-end gap-2">
                                        <Tag color={getAccessLevel(item) === 1 ? 'green' : 'default'}>
                                            {getAccessLevel(item) === 1 ? '公开' : '私有'}
                                        </Tag>
                                        <Select
                                            size="small"
                                            value={getAccessLevel(item)}
                                            onChange={(value) => handleAccessLevelChange(item.id, value)}
                                            options={[
                                                { label: '私有', value: 0 },
                                                { label: '公开', value: 1 },
                                            ]}
                                        />
                                        <Button
                                            type="link"
                                            size="small"
                                            icon={<ExportOutlined />}
                                            onClick={() => item.visitUrl && window.open(item.visitUrl, '_blank', 'noopener,noreferrer')}
                                            disabled={!item.visitUrl}
                                            className="px-0"
                                        >
                                            查看
                                        </Button>
                                    </div>
                                </div>
                            </motion.div>
                        ))}
                    </div>
                )}
            </Spin>
        </Drawer>
    );
};

export default ReferenceDrawer;
