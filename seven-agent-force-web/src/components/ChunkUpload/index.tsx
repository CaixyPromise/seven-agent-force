'use client';

import React, { useState, useRef } from 'react';
import { Upload, Progress, Button, message, Card } from 'antd';
import {
    CloudUploadOutlined,
    InboxOutlined,
    PauseCircleOutlined,
    PlayCircleOutlined
} from '@ant-design/icons';
import { motion, AnimatePresence } from 'framer-motion';
import type { RcFile } from 'antd/es/upload';
import { initChunkUpload, uploadChunkPart, completeChunkUpload, abortChunkUpload } from '@/api/chunkUploadController';
import { checkFileExist, uploadFileFaster } from '@/api/fileController';
import {
    buildFasterUploadInput,
    isAcceptedUploadResult,
    isExistingFile,
} from '@/api/uploadContract';
import { computeSha256Hex } from '@/utils/crypto';

const { Dragger } = Upload;
interface ChunkUploadProps {
    chunkSize?: number;
    onSuccess?: (fileId: API.Int64) => void;
    onError?: (error: Error) => void;
}

interface ChunkStatus {
    index: number;
    status: 'pending' | 'uploading' | 'success' | 'error';
}

function getUploadError(error: unknown): Error {
    return error instanceof Error ? error : new Error('未知上传错误');
}

const ChunkUpload: React.FC<ChunkUploadProps> = ({
    chunkSize = 5 * 1024 * 1024, // 5MB
    onSuccess,
    onError,
}) => {
    const [file, setFile] = useState<RcFile | null>(null);
    const [uploading, setUploading] = useState(false);
    const [paused, setPaused] = useState(false);
    const [progress, setProgress] = useState(0);
    const [chunks, setChunks] = useState<ChunkStatus[]>([]);
    const [uploadId, setUploadId] = useState<string>('');
    const [speed, setSpeed] = useState<string>('');

    const abortRef = useRef(false);
    const startTimeRef = useRef<number>(0);
    const uploadedBytesRef = useRef<number>(0);
    const pausedRef = useRef(false);

    const prepareUpload = async (targetFile: RcFile) => {
        const sha256 = await computeSha256Hex(targetFile);
        const res = await checkFileExist({
            sha256,
            fileSize: targetFile.size,
        });
        if (res.code !== 0 || !res.data) {
            throw new Error(res.message || '上传前检查失败');
        }
        return {
            check: res.data,
            sha256,
        };
    };

    const tryFasterUpload = async (
        targetFile: RcFile,
        sha256: string,
        check: API.CheckFileExistResponse,
    ) => {
        if (!isExistingFile(check)) {
            return null;
        }
        const res = await uploadFileFaster(buildFasterUploadInput({
            fileName: targetFile.name,
            contentType: targetFile.type,
            sha256,
            fileSize: targetFile.size,
        }));
        if (res.code !== 0 || !isAcceptedUploadResult(res.data)) {
            throw new Error(res.message || '文件秒传失败');
        }
        return res.data.fileId;
    };

    // 开始上传
    const handleUpload = async () => {
        if (!file) return;

        setUploading(true);
        setPaused(false);
        pausedRef.current = false;
        abortRef.current = false;
        startTimeRef.current = Date.now();
        uploadedBytesRef.current = 0;

        try {
            const { check, sha256 } = await prepareUpload(file);
            const fasterFileId = await tryFasterUpload(file, sha256, check);
            if (fasterFileId !== null) {
                message.success('秒传成功');
                onSuccess?.(fasterFileId);
                return;
            }
            // 1. 初始化上传
            const initRes = await initChunkUpload({
                fileName: file.name,
                fileSize: file.size,
                chunkSize,
                contentType: file.type,
                fileSha256: sha256,
            });

            if (initRes.code !== 0 || !initRes.data) {
                throw new Error(initRes.message || '初始化上传失败');
            }

            const { uploadId: uid, totalChunks } = initRes.data;
            setUploadId(uid);

            // 初始化分块状态
            const initialChunks: ChunkStatus[] = Array.from({ length: totalChunks }, (_, i) => ({
                index: i,
                status: 'pending',
            }));
            setChunks(initialChunks);

            // 2. 上传各分块
            for (let i = 0; i < totalChunks; i++) {
                // 检查是否暂停或取消
                if (abortRef.current) {
                    break;
                }

                while (pausedRef.current) {
                    await new Promise(resolve => setTimeout(resolve, 500));
                    if (abortRef.current) break;
                }

                // 更新分块状态为上传中
                setChunks(prev => prev.map((c, idx) =>
                    idx === i ? { ...c, status: 'uploading' } : c
                ));

                const start = i * chunkSize;
                const end = Math.min(start + chunkSize, file.size);
                const chunk = file.slice(start, end);

                try {
                    const partRes = await uploadChunkPart(uid, i + 1, chunk);

                    if (partRes.code === 0 && partRes.data?.uploaded) {
                        uploadedBytesRef.current += end - start;

                        // 更新分块状态为成功
                        setChunks(prev => prev.map((c, idx) =>
                            idx === i ? { ...c, status: 'success' } : c
                        ));

                        // 更新进度和速度
                        const progressPercent = Math.round(((i + 1) / totalChunks) * 100);
                        setProgress(progressPercent);

                        const elapsedTime = (Date.now() - startTimeRef.current) / 1000;
                        const speedBps = uploadedBytesRef.current / elapsedTime;
                        setSpeed(formatSpeed(speedBps));
                    } else {
                        throw new Error(`分块 ${i + 1} 上传失败`);
                    }
                } catch (error) {
                    // 更新分块状态为失败
                    setChunks(prev => prev.map((c, idx) =>
                        idx === i ? { ...c, status: 'error' } : c
                    ));
                    throw error;
                }
            }

            if (!abortRef.current) {
                // 3. 完成上传
                const completeRes = await completeChunkUpload({
                    uploadId: uid,
                });

                if (completeRes.code === 0 && isAcceptedUploadResult(completeRes.data)) {
                    message.success('上传成功');
                    onSuccess?.(completeRes.data.fileId);
                } else {
                    throw new Error(completeRes.message || '完成分块上传失败');
                }
            }
        } catch (error) {
            const uploadError = getUploadError(error);
            message.error(`上传失败: ${uploadError.message}`);
            onError?.(uploadError);
        } finally {
            setUploading(false);
        }
    };

    // 暂停/继续
    const togglePause = () => {
        const nextPaused = !pausedRef.current;
        pausedRef.current = nextPaused;
        setPaused(nextPaused);
    };

    // 取消上传
    const handleCancel = async () => {
        abortRef.current = true;

        if (uploadId) {
            try {
                await abortChunkUpload(uploadId);
            } catch (error) {
                console.error('Failed to abort upload:', error);
            }
        }

        setFile(null);
        setProgress(0);
        setChunks([]);
        setUploadId('');
        setUploading(false);
        setPaused(false);
    };

    // 格式化速度
    const formatSpeed = (bytesPerSecond: number): string => {
        if (bytesPerSecond < 1024) return `${bytesPerSecond.toFixed(0)} B/s`;
        if (bytesPerSecond < 1024 * 1024) return `${(bytesPerSecond / 1024).toFixed(1)} KB/s`;
        return `${(bytesPerSecond / 1024 / 1024).toFixed(1)} MB/s`;
    };

    // 格式化文件大小
    const formatSize = (bytes: number): string => {
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
        return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
    };

    // 渲染分块状态网格
    const renderChunkGrid = () => (
        <div className="mt-4 p-3 bg-gray-50 rounded-lg">
            <div className="text-xs text-gray-500 mb-2">分块状态</div>
            <div className="flex flex-wrap gap-1">
                {chunks.map((chunk) => (
                    <motion.div
                        key={chunk.index}
                        initial={{ scale: 0 }}
                        animate={{ scale: 1 }}
                        className={`w-3 h-3 rounded-sm ${chunk.status === 'pending' ? 'bg-gray-300' :
                            chunk.status === 'uploading' ? 'bg-blue-500 animate-pulse' :
                                chunk.status === 'success' ? 'bg-green-500' :
                                    'bg-red-500'
                            }`}
                        title={`分块 ${chunk.index + 1}: ${chunk.status}`}
                    />
                ))}
            </div>
        </div>
    );

    return (
        <Card className="chunk-upload-card">
            {!file ? (
                <Dragger
                    accept="*"
                    showUploadList={false}
                    beforeUpload={(f) => {
                        setFile(f);
                        return false;
                    }}
                >
                    <motion.div
                        whileHover={{ scale: 1.02 }}
                        className="py-8"
                    >
                        <p className="ant-upload-drag-icon">
                            <InboxOutlined className="text-4xl text-blue-400" />
                        </p>
                        <p className="ant-upload-text">点击或拖拽文件到此区域</p>
                        <p className="ant-upload-hint text-gray-400">
                            支持大文件分块上传，断点续传
                        </p>
                    </motion.div>
                </Dragger>
            ) : (
                <div>
                    {/* 文件信息 */}
                    <div className="flex items-center gap-3 p-3 bg-gray-50 rounded-lg mb-4">
                        <CloudUploadOutlined className="text-2xl text-blue-500" />
                        <div className="flex-1 min-w-0">
                            <div className="font-medium truncate">{file.name}</div>
                            <div className="text-sm text-gray-500">{formatSize(file.size)}</div>
                        </div>
                        {!uploading && (
                            <Button size="small" onClick={() => setFile(null)}>
                                更换
                            </Button>
                        )}
                    </div>

                    {/* 进度条 */}
                    <AnimatePresence>
                        {(uploading || progress > 0) && (
                            <motion.div
                                initial={{ opacity: 0, height: 0 }}
                                animate={{ opacity: 1, height: 'auto' }}
                                exit={{ opacity: 0, height: 0 }}
                            >
                                <Progress
                                    percent={progress}
                                    status={progress === 100 ? 'success' : 'active'}
                                    strokeColor={{
                                        '0%': '#1890ff',
                                        '100%': '#52c41a',
                                    }}
                                />
                                <div className="flex justify-between text-xs text-gray-500 mt-1">
                                    <span>{speed}</span>
                                    <span>{progress}%</span>
                                </div>
                            </motion.div>
                        )}
                    </AnimatePresence>

                    {/* 分块状态 */}
                    {chunks.length > 0 && renderChunkGrid()}

                    {/* 操作按钮 */}
                    <div className="flex justify-end gap-2 mt-4">
                        {uploading ? (
                            <>
                                <Button
                                    icon={paused ? <PlayCircleOutlined /> : <PauseCircleOutlined />}
                                    onClick={togglePause}
                                >
                                    {paused ? '继续' : '暂停'}
                                </Button>
                                <Button danger onClick={handleCancel}>
                                    取消
                                </Button>
                            </>
                        ) : (
                            <Button
                                type="primary"
                                icon={<CloudUploadOutlined />}
                                onClick={handleUpload}
                                disabled={progress === 100}
                                className="bg-gradient-to-r from-blue-500 to-blue-600"
                            >
                                开始上传
                            </Button>
                        )}
                    </div>
                </div>
            )}
        </Card>
    );
};

export default ChunkUpload;
