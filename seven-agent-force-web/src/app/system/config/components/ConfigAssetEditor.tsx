'use client';

import React, { useMemo, useState } from 'react';
import { Alert, Button, Image, Space, Typography, Upload, message } from 'antd';
import { DeleteOutlined, EyeOutlined, FileTextOutlined, UploadOutlined } from '@ant-design/icons';
import type { UploadProps } from 'antd';
import { uploadFile } from '@/api/fileController';
import { isAcceptedUploadResult } from '@/api/uploadContract';
import { configAssetStablePathOrEmpty } from '@/lib/configAssets';

const { Text } = Typography;

type ConfigAssetType = 'IMAGE' | 'FILE';

interface ConfigAssetEditorProps {
  assetType: ConfigAssetType;
  stablePath?: string;
  pendingFileName?: string;
  clearRequested?: boolean;
  disabled?: boolean;
  onUploaded: (fileId: API.Int64, fileName: string) => void;
  onClear: () => void;
}

function validateSelectedFile(file: File, assetType: ConfigAssetType): string | null {
  const name = file.name.toLowerCase();
  if (assetType === 'IMAGE') {
    const supported = ['.png', '.jpg', '.jpeg', '.webp'];
    if (!supported.some(extension => name.endsWith(extension))) {
      return '图片仅支持 PNG、JPEG 或 WebP 格式';
    }
    if (file.size > 2 * 1024 * 1024) {
      return '图片不能超过 2MB';
    }
    return null;
  }
  const supported = ['.pdf', '.txt'];
  if (!supported.some(extension => name.endsWith(extension))) {
    return '文件仅支持 PDF 或 TXT 格式';
  }
  if (file.size > 10 * 1024 * 1024) {
    return '文件不能超过 10MB';
  }
  return null;
}

/**
 * Minimal configuration-asset interaction. An uploaded fileId exists only in
 * React state until the parent saves it atomically with the configuration;
 * this component never displays the identifier, a reference, or a temporary
 * preview URL.
 */
export const ConfigAssetEditor: React.FC<ConfigAssetEditorProps> = ({
  assetType,
  stablePath,
  pendingFileName,
  clearRequested = false,
  disabled = false,
  onUploaded,
  onClear,
}) => {
  const [uploading, setUploading] = useState(false);
  const safeStablePath = useMemo(() => configAssetStablePathOrEmpty(stablePath), [stablePath]);
  const accept = assetType === 'IMAGE' ? '.png,.jpg,.jpeg,.webp' : '.pdf,.txt';

  const beforeUpload: UploadProps['beforeUpload'] = async file => {
    if (disabled || uploading) {
      return Upload.LIST_IGNORE;
    }
    const validationError = validateSelectedFile(file, assetType);
    if (validationError) {
      message.error(validationError);
      return Upload.LIST_IGNORE;
    }
    setUploading(true);
    try {
      const result = await uploadFile(file);
      if (result.code !== 0 || !isAcceptedUploadResult(result.data)) {
        throw new Error(result.message || '上传结果缺少有效文件标识');
      }
      onUploaded(result.data.fileId, file.name);
      message.success('文件已上传，保存后才会替换当前资产');
    } catch (error: unknown) {
      message.error(error instanceof Error && error.message ? error.message : '上传失败，请重试');
    } finally {
      setUploading(false);
    }
    return Upload.LIST_IGNORE;
  };

  const openCurrentFile = () => {
    if (!safeStablePath) {
      return;
    }
    window.open(safeStablePath, '_blank', 'noopener,noreferrer');
  };

  const label = assetType === 'IMAGE' ? '上传图片' : '上传文件';

  return (
    <Space orientation="vertical" size="middle" className="w-full rounded border border-gray-100 bg-gray-50 p-3">
      {safeStablePath && !clearRequested ? (
        assetType === 'IMAGE' ? (
          <Space align="start" wrap>
            <Image
              aria-label="当前配置图片预览"
              src={safeStablePath}
              alt="当前配置图片"
              width={96}
              height={96}
              style={{ objectFit: 'contain', background: '#fff', borderRadius: 6 }}
            />
            <Text type="secondary">当前图片已保存。上传新文件后，点击保存才会替换。</Text>
          </Space>
        ) : (
          <Space wrap>
            <FileTextOutlined />
            <Text type="secondary">当前文件已保存。</Text>
            <Button size="small" icon={<EyeOutlined />} onClick={openCurrentFile}>
              打开当前文件
            </Button>
          </Space>
        )
      ) : clearRequested ? (
        <Alert type="warning" showIcon title="当前资产将在保存后清除" />
      ) : (
        <Text type="secondary">尚未绑定资产。</Text>
      )}

      {pendingFileName ? (
        <Alert type="info" showIcon title={`新文件已上传：${pendingFileName}；保存后生效`} />
      ) : null}

      <Space wrap>
        <Upload accept={accept} showUploadList={false} beforeUpload={beforeUpload} disabled={disabled || uploading}>
          <Button icon={<UploadOutlined />} loading={uploading} disabled={disabled}>
            {safeStablePath && !clearRequested ? `替换并${label}` : label}
          </Button>
        </Upload>
        {(safeStablePath || pendingFileName || clearRequested) ? (
          <Button danger icon={<DeleteOutlined />} disabled={disabled} onClick={onClear}>
            清除
          </Button>
        ) : null}
      </Space>
      <Text type="secondary" className="text-xs">
        {assetType === 'IMAGE' ? '仅 PNG、JPEG、WebP，最大 2MB。' : '仅 PDF、TXT，最大 10MB。'}
      </Text>
    </Space>
  );
};
