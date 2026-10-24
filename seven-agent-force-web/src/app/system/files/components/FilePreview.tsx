'use client';

import React from 'react';
import { Modal, Image, Button, Space } from 'antd';
import {
    DownloadOutlined,
    FileOutlined
} from '@ant-design/icons';
import { motion } from 'framer-motion';

interface FilePreviewProps {
    file: API.FileInfo | null;
    onClose: () => void;
}

const FilePreview: React.FC<FilePreviewProps> = ({ file, onClose }) => {
    if (!file) return null;

    const isImage = file.contentType?.startsWith('image');
    const isVideo = file.contentType?.startsWith('video');
    const isPdf = file.contentType?.includes('pdf');
    const fileName = file.fileName || file.fileInnerName || '未命名文件';
    const fileUrl = file.fileUrl;

    const renderContent = () => {
        if (isImage && fileUrl) {
            return (
                <motion.div
                    initial={{ opacity: 0, scale: 0.9 }}
                    animate={{ opacity: 1, scale: 1 }}
                    className="flex justify-center"
                >
                    <Image
                        src={fileUrl}
                        alt={fileName}
                        className="max-h-[60vh] rounded-lg"
                        fallback="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
                    />
                </motion.div>
            );
        }

        if (isVideo && fileUrl) {
            return (
                <motion.div
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    className="flex justify-center"
                >
                    <video
                        src={fileUrl}
                        controls
                        className="max-h-[60vh] rounded-lg"
                        style={{ maxWidth: '100%' }}
                    >
                        您的浏览器不支持视频播放
                    </video>
                </motion.div>
            );
        }

        if (isPdf && fileUrl) {
            return (
                <motion.div
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    className="h-[60vh]"
                >
                    <iframe
                        src={fileUrl}
                        className="w-full h-full rounded-lg border"
                        title={fileName}
                    />
                </motion.div>
            );
        }

        return (
            <motion.div
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                className="flex flex-col items-center justify-center py-20"
            >
                <FileOutlined className="text-6xl text-gray-300 mb-4" />
                <p className="text-gray-500 mb-4">该文件类型暂不支持预览</p>
                <Button
                    type="primary"
                    icon={<DownloadOutlined />}
                    onClick={() => fileUrl && window.open(fileUrl, '_blank')}
                    disabled={!fileUrl}
                >
                    下载文件
                </Button>
            </motion.div>
        );
    };

    return (
        <Modal
            title={
                <div className="flex items-center gap-2">
                    {isImage && <span className="text-purple-500">🖼️</span>}
                    {isVideo && <span className="text-blue-500">🎬</span>}
                    {isPdf && <span className="text-red-500">📄</span>}
                    <span className="truncate max-w-md">{fileName}</span>
                </div>
            }
            open={!!file}
            onCancel={onClose}
            width={800}
            footer={
                <Space>
                    <Button onClick={onClose}>关闭</Button>
                    <Button
                        type="primary"
                        icon={<DownloadOutlined />}
                        onClick={() => fileUrl && window.open(fileUrl, '_blank')}
                        disabled={!fileUrl}
                    >
                        下载
                    </Button>
                </Space>
            }
            centered
        >
            {renderContent()}

            {/* 文件信息 */}
            <div className="mt-4 p-4 bg-gray-50 rounded-lg">
                <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                        <span className="text-gray-500">文件大小：</span>
                        <span className="font-medium">{formatFileSize(file.fileSize || 0)}</span>
                    </div>
                    <div>
                        <span className="text-gray-500">文件类型：</span>
                        <span className="font-medium">{file.contentType}</span>
                    </div>
                    <div className="col-span-2">
                        <span className="text-gray-500">SHA256：</span>
                        <span className="font-mono text-xs">{file.fileSha256}</span>
                    </div>
                    <div className="col-span-2">
                        <span className="text-gray-500">CRC32C：</span>
                        <span className="font-mono text-xs">{file.fileCrc32c || '-'}</span>
                    </div>
                    <div>
                        <span className="text-gray-500">完整性：</span>
                        <span className="font-medium">{file.integrityStatus || '-'}</span>
                    </div>
                    <div>
                        <span className="text-gray-500">安全判定：</span>
                        <span className="font-medium">{file.securityVerdict || '-'}</span>
                    </div>
                </div>
            </div>
        </Modal>
    );
};

// 格式化文件大小
const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

export default FilePreview;
