'use client';

import type { DictItemVO } from '@/types/dictClient';

export const USER_GENDER_DICT_CODE = 'gender';

export const normalizeUserGenderValue = (value: unknown): number | null => {
  if (value === null || value === undefined) {
    return null;
  }

  const raw = String(value).trim();
  if (!raw) {
    return null;
  }

  const upperValue = raw.toUpperCase();
  switch (upperValue) {
    case '0':
    case 'UNKNOWN':
      return 0;
    case '1':
    case 'MALE':
      return 1;
    case '2':
    case 'FEMALE':
      return 2;
    default: {
      const parsed = Number(raw);
      return Number.isFinite(parsed) ? parsed : null;
    }
  }
};

export const buildUserGenderOptions = (items?: DictItemVO[] | null) => {
  const options = new Map<number, { value: number; label: string }>();

  (items ?? []).forEach((item) => {
    const normalizedValue = normalizeUserGenderValue(item.itemValue);
    if (normalizedValue === null) {
      return;
    }
    if (!options.has(normalizedValue)) {
      options.set(normalizedValue, {
        value: normalizedValue,
        label: item.itemLabel,
      });
    }
  });

  return Array.from(options.values()).sort((left, right) => left.value - right.value);
};

export const buildUserGenderLabelMap = (items?: DictItemVO[] | null) => {
  const labelMap = new Map<number, string>();

  buildUserGenderOptions(items).forEach((option) => {
    labelMap.set(option.value, option.label);
  });

  return labelMap;
};

export const findUserGenderDictItem = (
  items: DictItemVO[] | null | undefined,
  value: number | null | undefined,
): DictItemVO | undefined => {
  if (value === null || value === undefined) {
    return undefined;
  }

  return (items ?? []).find(
    (item) => normalizeUserGenderValue(item.itemValue) === Number(value),
  );
};

export const resolveUserGenderLabel = (
  items: DictItemVO[] | null | undefined,
  value: number | null | undefined,
): string => {
  const dictItem = findUserGenderDictItem(items, value);
  if (dictItem?.itemLabel) {
    return dictItem.itemLabel;
  }

  switch (Number(value)) {
    case 1:
      return '男';
    case 2:
      return '女';
    default:
      return '未知';
  }
};
