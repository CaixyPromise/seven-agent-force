'use client';

import React, { useEffect, useRef, useState } from 'react';
import { Alert, Button, Input, Space } from 'antd';
import { DeleteOutlined, PlusOutlined } from '@ant-design/icons';

interface ControlledJsonEditorProps {
  value: string;
  disabled?: boolean;
  onChange: (value: string) => void;
  onValidationChange?: (error: string | null) => void;
  onDraftChange?: () => void;
}

type Row = { id: number; key: string; value: string };

function decodeRows(value: string, nextID: () => number): { rows: Row[]; error?: string } {
  try {
    const parsed = JSON.parse(value || '{}') as unknown;
    if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
      return { rows: [], error: '受控 JSON 仅支持一层字符串键值对象' };
    }
    const entries = Object.entries(parsed as Record<string, unknown>);
    if (entries.some(([, item]) => typeof item !== 'string')) {
      return {
        rows: [],
        error: '受控 JSON 仅支持一层字符串键值对象；数字、布尔、数组和嵌套对象必须通过专用契约管理',
      };
    }
    return {
      rows: entries.map(([key, item]) => ({ id: nextID(), key, value: item as string })),
    };
  } catch {
    return { rows: [], error: '现有值不是合法 JSON，必须先由后端修复后才能编辑' };
  }
}

function validateRows(rows: Row[]): string | null {
  const keys = rows.map(row => row.key.trim());
  if (keys.some(key => !key)) {
    return 'JSON 字段名不能为空';
  }
  if (new Set(keys).size !== keys.length) {
    return 'JSON 字段名不能重复';
  }
  return null;
}

function encodeRows(rows: Row[]): string {
  return JSON.stringify(
    Object.fromEntries(rows.map(row => [row.key.trim(), row.value])),
  );
}

export const ControlledJsonEditor: React.FC<ControlledJsonEditorProps> = ({
  value,
  disabled,
  onChange,
  onValidationChange,
  onDraftChange,
}) => {
  const [initial] = useState(() => {
    let initialID = 0;
    return decodeRows(value, () => ++initialID);
  });
  const nextIDRef = useRef(initial.rows.length + 1);
  const nextID = () => nextIDRef.current++;
  const [rows, setRows] = useState<Row[]>(initial.rows);
  const [decodeError, setDecodeError] = useState<string | undefined>(initial.error);
  const lastEmittedValue = useRef<string | null>(null);

  useEffect(() => {
    if (value === lastEmittedValue.current) {
      lastEmittedValue.current = null;
      return;
    }
    const decoded = decodeRows(value, nextID);
    setRows(decoded.rows);
    setDecodeError(decoded.error);
    onValidationChange?.(decoded.error ?? validateRows(decoded.rows));
  }, [value, onValidationChange]);

  const commitRows = (nextRows: Row[]) => {
    setRows(nextRows);
    onDraftChange?.();
    const validationError = validateRows(nextRows);
    onValidationChange?.(validationError);
    if (!validationError) {
      const encoded = encodeRows(nextRows);
      lastEmittedValue.current = encoded;
      onChange(encoded);
    }
  };

  if (decodeError) {
    return <Alert type="error" showIcon title={decodeError} />;
  }

  const update = (index: number, field: 'key' | 'value', next: string) => {
    commitRows(rows.map((row, rowIndex) =>
      rowIndex === index ? { ...row, [field]: next } : row,
    ));
  };

  const validationError = validateRows(rows);

  return (
    <Space orientation="vertical" className="w-full">
      {rows.map((row, index) => (
        <Space.Compact key={row.id} className="w-full">
          <Input
            aria-label={`JSON key ${index + 1}`}
            value={row.key}
            status={!row.key.trim() ? 'error' : undefined}
            disabled={disabled}
            placeholder="键"
            onChange={event => update(index, 'key', event.target.value)}
          />
          <Input
            aria-label={`JSON value ${index + 1}`}
            value={row.value}
            disabled={disabled}
            placeholder="文本值"
            onChange={event => update(index, 'value', event.target.value)}
          />
          <Button
            aria-label={`删除 JSON 字段 ${index + 1}`}
            disabled={disabled}
            icon={<DeleteOutlined />}
            onClick={() => commitRows(rows.filter((_, rowIndex) => rowIndex !== index))}
          />
        </Space.Compact>
      ))}
      {validationError ? <Alert type="warning" showIcon title={validationError} /> : null}
      <Button
        disabled={disabled}
        icon={<PlusOutlined />}
        onClick={() => commitRows([...rows, { id: nextID(), key: '', value: '' }])}
      >
        添加字段
      </Button>
    </Space>
  );
};
