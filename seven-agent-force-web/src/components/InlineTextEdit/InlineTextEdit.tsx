'use client';

import React, { useState, useEffect, useRef } from 'react';
import { Input, Popconfirm } from 'antd';
import type { InputRef } from 'antd';

interface InlineTextEditProps {
  /** 当前值 */
  value: string | number;
  /** 值变化回调 */
  onChange: (value: string | number) => void;
  /** 自定义样式类名 */
  className?: string;
  /** 自定义样式 */
  style?: React.CSSProperties;
  /** 是否需要确认 */
  confirm?: boolean;
  /** 占位符 */
  placeholder?: string;
  /** 文本样式类名 */
  textClassName?: string;
  /** 确认提示信息 */
  confirmMessage?: string;
  /** 是否禁用 */
  disabled?: boolean;
  /** 是否自动保存（onBlur时），默认为true */
  autoSave?: boolean;
}

/**
 * 行内文本编辑组件
 * 支持点击编辑、回车保存、ESC取消
 */
export const InlineTextEdit: React.FC<InlineTextEditProps> = ({
  value,
  onChange,
  className,
  style,
  confirm = false,
  placeholder,
  textClassName,
  confirmMessage,
  disabled = false,
  autoSave = true,
}) => {
  const [editing, setEditing] = useState(false);
  const [tempValue, setTempValue] = useState(value);
  const inputRef = useRef<InputRef>(null);

  useEffect(() => {
    if (editing && inputRef.current) {
      inputRef.current.focus();
    }
  }, [editing]);

  useEffect(() => {
    setTempValue(value);
  }, [value]);

  const handleSave = () => {
    const finalValue = typeof tempValue === 'string' ? tempValue.trim() : tempValue;
    if (finalValue !== value) {
      onChange(finalValue);
    }
    setEditing(false);
  };

  const onInputChange = (newValue: string | number) => {
    setTempValue(newValue);
    // 如果 autoSave=false，实时更新本地状态但不提交
    if (!autoSave) {
      onChange(newValue);
    }
  };

  const onBlur = () => {
    if (autoSave) {
      handleSave();
    } else {
      // 不自动保存，只取消编辑状态，但保持已更新的值
      setEditing(false);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      if (autoSave) {
        handleSave();
      } else {
        // 不自动保存模式下，Enter 只取消编辑状态
        setEditing(false);
      }
    } else if (e.key === 'Escape') {
      // ESC 取消所有修改
      setTempValue(value);
      if (!autoSave) {
        onChange(value); // 恢复原始值
      }
      setEditing(false);
    }
  };

  if (editing && !disabled) {
    return (
      <Input
        ref={inputRef}
        value={tempValue}
        onChange={e => onInputChange(e.target.value)}
        onBlur={onBlur}
        onKeyDown={onKeyDown}
        className={className}
        style={{ ...style, margin: -5, width: '100%', minWidth: 60 }}
        size="small"
        onClick={e => e.stopPropagation()}
      />
    );
  }

  const textElement = (
    <span
      className={`cursor-text hover:bg-gray-100 px-1 rounded transition-colors duration-200 border border-transparent hover:border-gray-200 ${textClassName || ''} ${
        disabled ? 'cursor-not-allowed opacity-60 hover:border-transparent hover:bg-transparent' : ''
      }`}
      onClick={(e) => {
        if (disabled) return;
        if (!confirm) {
          e.stopPropagation();
          setEditing(true);
        }
      }}
      title={disabled ? "只读属性不可直接修改" : "点击编辑"}
    >
      {value || <span className="text-gray-400 italic">{placeholder || '未设置'}</span>}
    </span>
  );

  if (confirm && !disabled) {
    return (
      <Popconfirm
        title="关键信息修改"
        description={confirmMessage || "修改此字段可能影响系统运行逻辑。"}
        onConfirm={(e) => { e?.stopPropagation(); setEditing(true); }}
        onCancel={(e) => e?.stopPropagation()}
        okText="修改"
        okType='danger'
        cancelText="取消"
        placement="topLeft"
      >
        <span onClick={e => e.stopPropagation()}>{textElement}</span>
      </Popconfirm>
    );
  }

  return textElement;
};

export default InlineTextEdit;
