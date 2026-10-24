'use client';

import React from 'react';
import { Checkbox, InputNumber, Select, Space } from 'antd';
import type { ConfigValueType, ScalarValidation } from '@/types/config';

interface ScalarValidationEditorProps {
  valueType: ConfigValueType;
  value?: ScalarValidation;
  disabled?: boolean;
  onChange: (value: ScalarValidation | undefined) => void;
}

function compactValidation(value: ScalarValidation): ScalarValidation | undefined {
  const compacted = Object.fromEntries(
    Object.entries(value).filter(([, item]) => item !== undefined),
  ) as ScalarValidation;
  if (compacted.options?.length === 0) {
    delete compacted.options;
  }
  return Object.keys(compacted).length > 0 ? compacted : undefined;
}

export const ScalarValidationEditor: React.FC<ScalarValidationEditorProps> = ({
  valueType,
  value,
  disabled,
  onChange,
}) => {
  const validation = value ?? {};
  const update = <K extends keyof ScalarValidation>(field: K, next: ScalarValidation[K]) => {
    onChange(compactValidation({ ...validation, [field]: next }));
  };
  const isEnum = valueType === 'ENUM' || valueType === 'MULTI_ENUM';

  if (valueType === 'IMAGE' || valueType === 'FILE') {
    return (
      <Space orientation="vertical" size="small" className="w-full rounded border border-gray-100 bg-gray-50 p-3">
        <Checkbox
          disabled={disabled}
          checked={validation.required === true}
          onChange={event => update('required', event.target.checked || undefined)}
        >
          必须绑定资产
        </Checkbox>
      </Space>
    );
  }

  return (
    <Space orientation="vertical" size="small" className="w-full rounded border border-gray-100 bg-gray-50 p-3">
      <Checkbox
        disabled={disabled}
        checked={validation.required === true}
        onChange={event => update('required', event.target.checked || undefined)}
      >
        必填
      </Checkbox>
      <Space wrap>
        <InputNumber
          aria-label="最小长度"
          min={0}
          disabled={disabled}
          placeholder="最小长度"
          value={validation.minLength}
          onChange={next => update('minLength', next ?? undefined)}
        />
        <InputNumber
          aria-label="最大长度"
          min={0}
          max={65536}
          disabled={disabled}
          placeholder="最大长度"
          value={validation.maxLength}
          onChange={next => update('maxLength', next ?? undefined)}
        />
        <InputNumber
          aria-label="最小值"
          disabled={disabled}
          placeholder="最小值"
          value={validation.minValue}
          onChange={next => update('minValue', next ?? undefined)}
        />
        <InputNumber
          aria-label="最大值"
          disabled={disabled}
          placeholder="最大值"
          value={validation.maxValue}
          onChange={next => update('maxValue', next ?? undefined)}
        />
        <InputNumber
          aria-label="最大选项数"
          min={1}
          max={100}
          disabled={disabled || valueType !== 'MULTI_ENUM'}
          placeholder="最大选项数"
          value={validation.maxItems}
          onChange={next => update('maxItems', next ?? undefined)}
        />
      </Space>
      {isEnum ? (
        <Select
          aria-label="枚举选项"
          mode="tags"
          tokenSeparators={[',']}
          disabled={disabled}
          value={validation.options ?? []}
          placeholder="输入选项后按回车；支持逗号分隔"
          onChange={options => update(
            'options',
            options.map(option => option.trim()).filter(Boolean),
          )}
          options={(validation.options ?? []).map(option => ({ value: option, label: option }))}
        />
      ) : null}
    </Space>
  );
};
