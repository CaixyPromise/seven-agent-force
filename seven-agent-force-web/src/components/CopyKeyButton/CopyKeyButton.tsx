'use client';

import React, { useState } from 'react';
import { Tooltip, Dropdown, message, Modal, Input } from 'antd';
import { CopyOutlined, CheckOutlined, KeyOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';

interface CopyOption {
  label: string;
  value: string;
  description?: string;
}

interface CopyKeyButtonProps {
  /** 可复制的选项列表 */
  options: CopyOption[];
  /** 按钮大小 */
  size?: 'small' | 'default';
  /** 自定义类名 */
  className?: string;
}

/**
 * 复制 Key 按钮组件
 * 用于在配置项和字典项中提供复制请求 key 的功能
 */
export const CopyKeyButton: React.FC<CopyKeyButtonProps> = ({
  options,
  size = 'small',
  className = '',
}) => {
  const [copied, setCopied] = useState(false);
  const [showManualCopy, setShowManualCopy] = useState<string | null>(null);
  const handleCopy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      message.success({ content: '已复制到剪贴板', duration: 1.5 });
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // 复制失败，显示手动复制对话框
      console.error('复制失败, 请手动复制');
      setShowManualCopy(value);
    }
  };

  const menuItems: MenuProps['items'] = options.map((opt, index) => ({
    key: index.toString(),
    label: (
      <div
        className="flex flex-col py-1 min-w-[200px]"
        onClick={(e) => {
          e.stopPropagation();
          handleCopy(opt.value);
        }}
      >
        <div className="flex items-center justify-between gap-2" >
          <span className="text-xs text-gray-500">{opt.label}</span>
          <Tooltip title="点击复制">
            <span
              className="text-xs text-indigo-500 hover:text-indigo-700 cursor-pointer flex items-center gap-1"
              onClick={(e) => {
                e.stopPropagation();
                handleCopy(opt.value);
              }}
            >
              <CopyOutlined /> 复制
            </span>
          </Tooltip>
        </div>
        <code className="text-xs font-mono bg-gray-100 px-2 py-1 rounded mt-1 text-gray-700 break-all select-all">
          {opt.value}
        </code>
        {opt.description && (
          <span className="text-[10px] text-gray-400 mt-0.5">{opt.description}</span>
        )}
      </div>
    ),
  }));

  return (
    <>
      <Dropdown
        menu={{ items: menuItems }}
        trigger={['click']}
        placement="bottomRight"
      >
        <Tooltip title="复制请求 Key">
          <div
            className={`
              inline-flex items-center justify-center cursor-pointer transition-all
              ${size === 'small' ? 'w-6 h-6 text-xs' : 'w-8 h-8 text-sm'}
              rounded bg-gray-50 hover:bg-indigo-50 text-gray-400 hover:text-indigo-600
              border border-transparent hover:border-indigo-200
              ${className}
            `}
          >
            {copied ? (
              <CheckOutlined className="text-green-500" />
            ) : (
              <KeyOutlined />
            )}
          </div>
        </Tooltip>
      </Dropdown>

      {/* 手动复制对话框 */}
      <Modal
        title="复制失败"
        open={!!showManualCopy}
        onCancel={() => setShowManualCopy(null)}
        footer={null}
        width={400}
      >
        <div className="space-y-3">
          <p className="text-gray-500 text-sm">
            自动复制失败，请手动选择下方内容并复制 (Ctrl+C / Cmd+C)：
          </p>
          <Input.TextArea
            value={showManualCopy || ''}
            readOnly
            autoSize={{ minRows: 2, maxRows: 4 }}
            className="font-mono text-sm"
            onFocus={(e) => e.target.select()}
          />
        </div>
      </Modal>
    </>
  );
};

export default CopyKeyButton;
