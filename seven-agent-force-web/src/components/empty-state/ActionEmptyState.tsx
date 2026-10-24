'use client';

import React from 'react';
import { Button } from 'antd';

interface ActionEmptyStateProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  actionText?: string;
  onAction?: () => void;
  secondaryActionText?: string;
  onSecondaryAction?: () => void;
}

export const ActionEmptyState: React.FC<ActionEmptyStateProps> = ({
  icon,
  title,
  description,
  actionText,
  onAction,
  secondaryActionText,
  onSecondaryAction,
}) => (
  <div className="flex flex-col items-center justify-center py-16 px-6 text-center">
    <div className="w-20 h-20 rounded-2xl bg-gradient-to-br from-slate-100 to-slate-200 flex items-center justify-center text-4xl text-slate-400 mb-5">
      {icon}
    </div>
    <h3 className="text-base font-semibold text-slate-700 mb-2">{title}</h3>
    <p className="text-sm text-slate-500 mb-5 max-w-md">{description}</p>
    <div className="flex items-center gap-3">
      {actionText && onAction && (
        <Button type="primary" onClick={onAction}>
          {actionText}
        </Button>
      )}
      {secondaryActionText && onSecondaryAction && (
        <Button onClick={onSecondaryAction}>{secondaryActionText}</Button>
      )}
    </div>
  </div>
);
