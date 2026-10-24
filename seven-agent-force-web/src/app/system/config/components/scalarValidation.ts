import type { ConfigValueType, ScalarValidation } from '@/types/config';

export function validateScalarValidation(
  valueType: ConfigValueType,
  validation?: ScalarValidation,
): string | null {
  if (!validation) {
    return valueType === 'ENUM' || valueType === 'MULTI_ENUM'
      ? '枚举类型必须配置至少一个选项'
      : null;
  }
  if (
    validation.minLength !== undefined &&
    validation.maxLength !== undefined &&
    validation.minLength > validation.maxLength
  ) {
    return '最小长度不能大于最大长度';
  }
  if (
    validation.minValue !== undefined &&
    validation.maxValue !== undefined &&
    validation.minValue > validation.maxValue
  ) {
    return '最小值不能大于最大值';
  }
  if (
    validation.maxItems !== undefined &&
    (validation.maxItems < 1 || validation.maxItems > 100)
  ) {
    return '最大选项数必须在 1 到 100 之间';
  }
  if (valueType === 'ENUM' || valueType === 'MULTI_ENUM') {
    const options = validation.options?.map(item => item.trim()).filter(Boolean) ?? [];
    if (options.length === 0) return '枚举类型必须配置至少一个选项';
    if (new Set(options).size !== options.length) return '枚举选项不能重复';
  }
  return null;
}
