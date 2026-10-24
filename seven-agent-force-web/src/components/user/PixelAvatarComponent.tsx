'use client';

import React, { useMemo } from 'react';
import { Avatar } from 'antd';
import type { AvatarProps } from 'antd';
import { pixelAvatarDataUrl } from './pixelAvatarDataUrl';

export interface PixelAvatarProps extends Omit<AvatarProps, 'src'> {
  src?: string;
  seed?: string;
}

export default function PixelAvatar({ src, seed, ...props }: PixelAvatarProps) {
  const fallback = useMemo(() => pixelAvatarDataUrl(seed), [seed]);
  return <Avatar {...props} src={src || fallback} />;
}
