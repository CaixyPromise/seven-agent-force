'use client';

import { useState, useEffect, useCallback } from 'react';
import { message } from 'antd';
import type { TablePaginationConfig } from 'antd/es/table';
import {
    getFileList,
    getFileStats,
    deleteFile,
    batchDeleteFiles,
    type FileBatchDeleteResult,
} from '@/api/fileManageController';
import { getFileDownloadUrl } from '@/api/fileController';

interface FileFilters {
    fileName?: string;
    fileType?: string;
    bizType?: number;
    startTime?: string;
    endTime?: string;
}

interface FileStats {
    totalCount: number;
    totalSize: number;
    totalSizeFormatted: string;
    imageCount: number;
    docCount: number;
    videoCount: number;
}

export function useFileManage() {
    const [fileList, setFileList] = useState<API.FileInfo[]>([]);
    const [loading, setLoading] = useState(false);
    const [filters, setFilters] = useState<FileFilters>({});
    const [stats, setStats] = useState<FileStats | null>(null);
    const [pagination, setPagination] = useState<TablePaginationConfig>({
        current: 1,
        pageSize: 10,
        total: 0,
    });
    const current = pagination.current ?? 1;
    const pageSize = pagination.pageSize ?? 10;

    // 获取文件列表
    const fetchFileList = useCallback(async () => {
        setLoading(true);
        try {
            const res = await getFileList({
                current,
                pageSize,
                ...filters,
            });

            if (res.code === 0) {
                const records = res.data?.records || [];
                setFileList(records.map(normalizeFileInfo));
                setPagination(prev => ({
                    ...prev,
                    total: res.data?.total || 0,
                }));
            }
        } catch (error) {
            console.error('Failed to fetch file list:', error);
            message.error('获取文件列表失败');
        } finally {
            setLoading(false);
        }
    }, [current, pageSize, filters]);

    // 获取统计数据
    const fetchStats = useCallback(async () => {
        try {
            const res = await getFileStats();
            if (res.code === 0 && res.data) {
                setStats(res.data);
            }
        } catch (error) {
            console.error('Failed to fetch stats:', error);
        }
    }, []);

    // 删除文件
    const deleteFiles = async (ids: API.Int64[]): Promise<FileBatchDeleteResult> => {
        try {
            const res = ids.length === 1
                ? await deleteFile(ids[0])
                : await batchDeleteFiles(ids);

            const result = res.data ?? {
                success: false,
                outcome: 'FULL_FAILED',
                requestedCount: ids.length,
                deletedCount: 0,
                skippedCount: ids.length,
                deletedIds: [],
                skippedItems: ids.map((id) => ({
                    fileId: id,
                    reason: 'DELETE_FAILED',
                    message: res.message || '删除失败',
                })),
            };

            if (res.code === 0) {
                if (result.outcome === 'FULL_SUCCESS') {
                    message.success('删除成功');
                }
                fetchFileList();
                fetchStats();
                return result;
            }
            throw new Error(res.message);
        } catch (error) {
            const errorMessage = resolveErrorMessage(error);
            message.error(`删除失败: ${errorMessage}`);
            return {
                success: false,
                outcome: 'FULL_FAILED',
                requestedCount: ids.length,
                deletedCount: 0,
                skippedCount: ids.length,
                deletedIds: [],
                skippedItems: ids.map((id) => ({
                    fileId: id,
                    reason: 'DELETE_FAILED',
                    message: errorMessage,
                })),
            };
        }
    };

    // 下载文件
    const resolveFileUrl = async (file: API.FileInfo): Promise<string | undefined> => {
        if (file.fileUrl) {
            return toAbsoluteFileUrl(file.fileUrl);
        }
        if (!file.id) {
            return undefined;
        }
        try {
            const res = await getFileDownloadUrl(file.id);
            const downloadUrl = res.data?.url;
            if (res.code === 0 && downloadUrl) {
                const absoluteUrl = toAbsoluteFileUrl(downloadUrl);
                setFileList(prev =>
                    prev.map(item =>
                        item.id === file.id ? { ...item, fileUrl: absoluteUrl } : item
                    )
                );
                return absoluteUrl;
            }
        } catch (error) {
            console.error('Failed to resolve file url:', error);
        }
        return undefined;
    };

    const resolveFileUrls = async (files: API.FileInfo[]) => {
        if (!files.length) {
            return 0;
        }
        let resolvedCount = 0;
        const results = await Promise.all(files.map(async (file) => {
            if (file.fileUrl) {
                return { id: file.id, url: toAbsoluteFileUrl(file.fileUrl) };
            }
            const url = await resolveFileUrl(file);
            if (url) {
                resolvedCount += 1;
            }
            return { id: file.id, url };
        }));

        setFileList(prev =>
            prev.map(item => {
                const hit = results.find(res => res.id === item.id);
                if (hit?.url) {
                    return { ...item, fileUrl: hit.url };
                }
                return item;
            })
        );
        return resolvedCount;
    };

    const downloadFile = async (file: API.FileInfo) => {
        try {
            const fileUrl = await resolveFileUrl(file);
            if (!fileUrl) {
                message.warning('未找到可用的下载链接');
                return;
            }
            const link = document.createElement('a');
            link.href = fileUrl;
            link.download = file.fileName || 'file';
            link.target = '_blank';
            document.body.appendChild(link);
            link.click();
            document.body.removeChild(link);
        } catch {
            message.error('下载失败');
        }
    };

    // 刷新列表
    const refreshList = () => {
        fetchFileList();
        fetchStats();
    };

    // 初始化加载
    useEffect(() => {
        fetchFileList();
        fetchStats();
    }, [fetchFileList, fetchStats]);

    // 筛选条件变化时重置页码
    useEffect(() => {
        setPagination(prev => ({ ...prev, current: 1 }));
    }, [filters]);

    return {
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
    };
}

function normalizeFileInfo(file: Partial<API.FileInfo> & Record<string, unknown>): API.FileInfo {
    return {
        ...file,
        fileUrl: toAbsoluteFileUrl(file.fileUrl),
        fileName: file.fileName || file.fileInnerName || file.storagePath || `file-${file.id ?? ''}`,
    } as API.FileInfo;
}

function toAbsoluteFileUrl(fileUrl?: string): string | undefined {
    if (!fileUrl) {
        return undefined;
    }
    if (typeof window === 'undefined') {
        return fileUrl;
    }
    try {
        return new URL(fileUrl, window.location.origin).toString();
    } catch {
        return fileUrl;
    }
}

function resolveErrorMessage(error: unknown): string {
    if (error instanceof Error && error.message) {
        return error.message;
    }
    return '未知错误';
}
