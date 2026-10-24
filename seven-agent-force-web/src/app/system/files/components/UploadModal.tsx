'use client';

import React, { useState } from 'react';
import { Modal, Upload, Progress, Button, message } from 'antd';
import {
    InboxOutlined,
    CloudUploadOutlined
} from '@ant-design/icons';
import type {RcFile, UploadFile, UploadProps} from 'antd/es/upload';
import { motion, AnimatePresence } from 'framer-motion';
import { checkFileExist, uploadFile, uploadFileFaster } from '@/api/fileController';
import { initChunkUpload, uploadChunkPart, completeChunkUpload } from '@/api/chunkUploadController';
import {
    buildFasterUploadInput,
    isAcceptedUploadResult,
    isExistingFile,
} from '@/api/uploadContract';
import { computeSha256Hex } from '@/utils/crypto';

const { Dragger } = Upload;

interface UploadModalProps {
    open: boolean;
    onClose: () => void;
    onSuccess: () => void;
}

function getErrorMessage(error: unknown): string {
    return error instanceof Error && error.message ? error.message : '未知错误';
}

const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB per chunk
const LARGE_FILE_THRESHOLD = 10 * 1024 * 1024; // 10MB threshold for chunked upload

const UploadModal: React.FC<UploadModalProps> = ({ open, onClose, onSuccess }) => {
    const [fileList, setFileList] = useState<UploadFile[]>([]);
    const [uploading, setUploading] = useState(false);
    const [uploadProgress, setUploadProgress] = useState<Record<string, number>>({});

    const prepareUpload = async (file: File) => {
        const sha256 = await computeSha256Hex(file);
        const res = await checkFileExist({
            sha256,
            fileSize: file.size,
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
        file: File,
        sha256: string,
        check: API.CheckFileExistResponse,
    ) => {
        if (!isExistingFile(check)) {
            return false;
        }
        const res = await uploadFileFaster(buildFasterUploadInput({
            fileName: file.name,
            contentType: file.type,
            sha256,
            fileSize: file.size,
        }));
        if (res.code !== 0 || !isAcceptedUploadResult(res.data)) {
            throw new Error(res.message || '文件秒传失败');
        }
        return true;
    };

    // 普通上传
    const handleNormalUpload = async (file: File) => {
        try {
            const { check, sha256 } = await prepareUpload(file);
            const fasterOk = await tryFasterUpload(file, sha256, check);
            if (fasterOk) {
                return true;
            }
            const res = await uploadFile(file);
            if (res.code === 0 && isAcceptedUploadResult(res.data)) {
                return true;
            }
            throw new Error(res.message || '上传结果缺少有效文件标识');
        } catch (error) {
            message.error(`上传失败: ${getErrorMessage(error)}`);
            return false;
        }
    };

    // 分块上传
    const handleChunkUpload = async (file: File) => {
        try {
            const { check, sha256 } = await prepareUpload(file);
            const fasterOk = await tryFasterUpload(file, sha256, check);
            if (fasterOk) {
                return true;
            }
            // 1. 初始化上传
            const initRes = await initChunkUpload({
                fileName: file.name,
                fileSize: file.size,
                chunkSize: CHUNK_SIZE,
                contentType: file.type,
                fileSha256: sha256,
            });

            if (initRes.code !== 0 || !initRes.data) {
                throw new Error(initRes.message || '初始化上传失败');
            }

            const { uploadId, totalChunks } = initRes.data;
            // 2. 上传各分块
            for (let i = 0; i < totalChunks; i++) {
                const start = i * CHUNK_SIZE;
                const end = Math.min(start + CHUNK_SIZE, file.size);
                const chunk = file.slice(start, end);

                const partRes = await uploadChunkPart(uploadId, i + 1, chunk);

                if (partRes.code === 0 && partRes.data?.uploaded) {
                    // 更新进度
                    const progress = Math.round(((i + 1) / totalChunks) * 100);
                    setUploadProgress(prev => ({ ...prev, [file.name]: progress }));
                } else {
                    throw new Error(`分块 ${i + 1} 上传失败`);
                }
            }

            // 3. 完成上传
            const completeRes = await completeChunkUpload({
                uploadId,
            });

            if (completeRes.code !== 0 || !completeRes.data) {
                throw new Error(completeRes.message || '完成分块上传失败');
            }

            return true;
        } catch (error) {
            message.error(`上传失败: ${getErrorMessage(error)}`);
            return false;
        }
    };

    // 处理上传
    const handleUpload = async () => {
        if (fileList.length === 0) {
            message.warning('请先选择文件');
            return;
        }

        setUploading(true);
        let successCount = 0;

        for (const uploadFile of fileList) {
            const file = (uploadFile.originFileObj ?? (uploadFile as unknown as RcFile)) as File;
            if (!file || !(file instanceof File)) {
              console.log(uploadFile);
              message.error('不是file格式');
              continue;
            }

            // 根据文件大小选择上传方式
            const success = file.size > LARGE_FILE_THRESHOLD
                ? await handleChunkUpload(file)
                : await handleNormalUpload(file);

            if (success) successCount++;
        }

        setUploading(false);

        if (successCount === fileList.length) {
            message.success(`${successCount} 个文件上传成功`);
            setFileList([]);
            setUploadProgress({});
            onSuccess();
        } else {
            message.warning(`${successCount}/${fileList.length} 个文件上传成功`);
        }
    };

    const uploadProps: UploadProps = {
        multiple: true,
        fileList,
        beforeUpload: () => false,
        onChange: (info) => {
            setFileList(info.fileList);
        },
        onRemove: (file) => {
            setFileList(prev => prev.filter(f => f.uid !== file.uid));
        },
    };

    return (
        <Modal
            title={
                <div className="flex items-center gap-2">
                    <CloudUploadOutlined className="text-blue-500" />
                    <span>上传文件</span>
                </div>
            }
            open={open}
            onCancel={() => {
                if (!uploading) {
                    setFileList([]);
                    setUploadProgress({});
                    onClose();
                }
            }}
            width={600}
            footer={[
                <Button key="cancel" onClick={onClose} disabled={uploading}>
                    取消
                </Button>,
                <Button
                    key="upload"
                    type="primary"
                    onClick={handleUpload}
                    loading={uploading}
                    disabled={fileList.length === 0}
                    className="bg-gradient-to-r from-blue-500 to-blue-600"
                >
                    {uploading ? '上传中...' : `上传 (${fileList.length})`}
                </Button>
            ]}
        >
            {/* 资产暂存说明 */}
            <div className="flex items-center gap-4 mb-4 p-3 bg-gray-50 rounded-lg">
                <div className="flex items-center gap-2">
                    <span className="text-gray-500 text-sm">资产状态：</span>
                    <span className="text-sm font-medium text-gray-700">未绑定暂存</span>
                </div>
                <span className="text-gray-500 text-sm">图片衍生处理由服务端策略执行</span>
            </div>

            {/* 拖拽上传区域 */}
            <Dragger {...uploadProps} disabled={uploading}>
                <motion.div
                    whileHover={{ scale: 1.02 }}
                    className="py-8"
                >
                    <p className="ant-upload-drag-icon">
                        <InboxOutlined className="text-4xl text-blue-400" />
                    </p>
                    <p className="ant-upload-text">点击或拖拽文件到此区域上传</p>
                    <p className="ant-upload-hint text-gray-400">
                        上传后进入资产暂存区，不会自动绑定任何业务记录
                    </p>
                </motion.div>
            </Dragger>

            {/* 上传进度 */}
            <AnimatePresence>
                {uploading && Object.keys(uploadProgress).length > 0 && (
                    <motion.div
                        initial={{ opacity: 0, height: 0 }}
                        animate={{ opacity: 1, height: 'auto' }}
                        exit={{ opacity: 0, height: 0 }}
                        className="mt-4 space-y-2"
                    >
                        {Object.entries(uploadProgress).map(([name, progress]) => (
                            <div key={name} className="flex items-center gap-3">
                                <span className="text-sm truncate w-40">{name}</span>
                                <Progress
                                    percent={progress}
                                    size="small"
                                    className="flex-1"
                                    status={progress === 100 ? 'success' : 'active'}
                                />
                            </div>
                        ))}
                    </motion.div>
                )}
            </AnimatePresence>
        </Modal>
    );
};

export default UploadModal;
