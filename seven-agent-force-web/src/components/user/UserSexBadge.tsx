'use client';

import { UserOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import { useDictValueOnly } from '@/hooks/useDictValue';
import { parseDictItemExt } from '@/types/dictClient';
import React from 'react';
import {
  findUserGenderDictItem,
  resolveUserGenderLabel,
  USER_GENDER_DICT_CODE,
} from '@/lib/userGender';

const COLOR_MAP: Record<string, string> = {
  blue: 'processing',
  pink: 'magenta',
  gray: 'default',
};

export default function UserSexBadge({ value }: { value?: number | null }) {
  const genderItems = useDictValueOnly(USER_GENDER_DICT_CODE);
  if (value === undefined || value === null) return null;
  const dictItem = findUserGenderDictItem(genderItems, value);
  const label = resolveUserGenderLabel(genderItems, value);
  const ext = dictItem ? parseDictItemExt<{ color?: string }>(dictItem) : null;
  const color = ext?.color ? (COLOR_MAP[ext.color] ?? 'default') : 'default';
  return (
    <Tag color={color} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, marginInlineEnd: 0 }}>
      <UserOutlined />
      <span>{label}</span>
    </Tag>
  );
}
