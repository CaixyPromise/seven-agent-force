'use client';

import React, { useState } from 'react';
import { Card, Button, Input, Pagination, Popconfirm, InputNumber, Tooltip } from 'antd';
import { PlusOutlined, SearchOutlined, DeleteOutlined, HolderOutlined, SettingOutlined } from '@ant-design/icons';
import { DndContext, closestCenter, DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { InlineTextEdit } from '@/components/InlineTextEdit';
import { ActionEmptyState } from '@/components/empty-state/ActionEmptyState';
import { useConfigContext } from '../context/useConfigContext';
import type { ConfigGroup } from '@/types/config';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';

// 可编辑的排序序号组件
const EditableSortOrder: React.FC<{
  value: number;
  disabled?: boolean;
  onChange: (val: number) => void;
}> = ({ value, disabled = false, onChange }) => {
  const [editing, setEditing] = useState(false);
  const [tempValue, setTempValue] = useState(value);

  const handleConfirm = () => {
    if (tempValue !== value && tempValue >= 0) {
      onChange(tempValue);
    }
    setEditing(false);
  };

  if (editing) {
    return (
      <InputNumber
        size="small"
        min={0}
        value={tempValue}
        onChange={v => setTempValue(v ?? 0)}
        onBlur={handleConfirm}
        onPressEnter={handleConfirm}
        autoFocus
        className="w-12 text-xs"
        controls={false}
      />
    );
  }

  return (
    <Tooltip title="点击修改排序" placement="top">
      <div
        onClick={e => {
          e.stopPropagation();
          if (disabled) return;
          setTempValue(value);
          setEditing(true);
        }}
        className={`flex items-center justify-center w-6 h-6 rounded bg-gray-100 text-xs font-mono text-gray-500 transition-colors select-none ${
          disabled ? 'cursor-not-allowed opacity-50' : 'hover:bg-indigo-100 hover:text-indigo-600 cursor-pointer'
        }`}
      >
        {value}
      </div>
    </Tooltip>
  );
};

interface ConfigGroupSidebarProps {
  onCreateClick: () => void;
}

interface SortableItemProps {
  group: ConfigGroup;
  isActive: boolean;
  onSelect: () => void;
  onDelete: (e: React.MouseEvent) => void;
  onUpdateField: (field: keyof ConfigGroup, value: unknown) => void;
  canWrite: boolean;
  canDelete: boolean;
}

const SortableGroupItem: React.FC<SortableItemProps> = ({
  group,
  isActive,
  onSelect,
  onDelete,
  onUpdateField,
  canWrite,
  canDelete,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: group.id,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`
        group relative px-2 py-3 rounded-lg transition-colors border-l-4 cursor-pointer flex gap-2
        ${isActive ? 'bg-indigo-50 border-indigo-500' : 'hover:bg-gray-50 border-transparent'}
      `}
      onClick={onSelect}
    >
      <div
        {...(canWrite ? attributes : {})}
        {...(canWrite ? listeners : {})}
        className={`mt-1 text-gray-300 ${canWrite ? 'cursor-move hover:text-gray-500' : 'cursor-not-allowed opacity-40'}`}
        title="按住拖动以调整顺序"
      >
        <HolderOutlined />
      </div>

      <div className="flex-1 min-w-0 overflow-hidden">
        <div className="flex justify-between items-start">
          <div className="flex-1 pr-6 overflow-hidden">
            <div className={`text-sm font-medium mb-1 flex min-w-0 items-center gap-2 ${isActive ? 'text-indigo-700' : 'text-gray-700'}`}>
              <InlineTextEdit
                value={group.groupName}
                disabled={!canWrite}
                onChange={val => onUpdateField('groupName', val)}
                textClassName="block max-w-full truncate"
              />
              <EditableSortOrder
                value={group.sortOrder ?? 0}
                disabled={!canWrite}
                onChange={val => onUpdateField('sortOrder', val)}
              />
            </div>
            <div className="text-xs text-gray-400 font-mono min-w-0">
              <InlineTextEdit
                value={group.groupCode}
                disabled={!canWrite}
                confirm={true}
                confirmMessage="修改分组编码(Group Code)是高风险操作，请确保后端逻辑兼容。"
                onChange={val => onUpdateField('groupCode', val)}
                textClassName="block max-w-full truncate hover:text-indigo-600 cursor-pointer"
              />
            </div>
          </div>
          {canDelete ? (
            <div
              className="absolute right-2 top-3 opacity-0 group-hover:opacity-100 transition-opacity"
              onClick={e => e.stopPropagation()}
            >
              <Popconfirm
                title="删除分组?"
                onConfirm={(e) => {
                  if (e) {
                    e.stopPropagation();
                    onDelete(e as React.MouseEvent);
                  }
                }}
                okType="danger"
              >
                <Button type="text" danger size="small" icon={<DeleteOutlined />} className="bg-white/80 backdrop-blur" />
              </Popconfirm>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
};

export const ConfigGroupSidebar: React.FC<ConfigGroupSidebarProps> = ({ onCreateClick }) => {
  const canCreateGroup = usePermissionAccess(CONFIG_PERMISSIONS.GROUP_ADD);
  const canEditGroup = usePermissionAccess(CONFIG_PERMISSIONS.GROUP_EDIT);
  const canDeleteGroup = usePermissionAccess(CONFIG_PERMISSIONS.GROUP_DELETE);
  const {
    groupedGroups,
    groupTotal,
    groupPageNum,
    groupPageSize,
    activeGroup,
    setActiveGroup,
    fetchGroups,
    setGroupPageNum,
    setGroupPageSize,
    handleUpdateGroup,
    handleDeleteGroup,
    searchTerm,
    setSearchTerm,
    handleMoveGroup,
    groups,
  } = useConfigContext();
  const hasAnyGroup = groups.length > 0;

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const oldIndex = groups.findIndex(g => g.id === active.id);
    const newIndex = groups.findIndex(g => g.id === over.id);

    if (oldIndex === -1 || newIndex === -1) return;

    // 按规范计算 beforeId 和 afterId
    const movedId = String(active.id);
    const movedGroupForAccess = groups.find(g => g.id === movedId);
    if (!canEditGroup || movedGroupForAccess?.access?.canWrite === false) return;
    let beforeId: API.Int64 | null = null;
    let afterId: API.Int64 | null = null;

    // 模拟移动后的新数组
    const newGroups = [...groups];
    const [movedGroup] = newGroups.splice(oldIndex, 1);
    newGroups.splice(newIndex, 0, movedGroup);

    // 根据新位置计算 beforeId 和 afterId
    if (newIndex === 0) {
      // 移到最前：afterId = 新位置后一个元素
      afterId = newGroups.length > 1 ? newGroups[1].id : null;
    } else if (newIndex === newGroups.length - 1) {
      // 移到最后：beforeId = 新位置前一个元素
      beforeId = newGroups[newIndex - 1].id;
    } else {
      // 插到中间：beforeId = 前一个，afterId = 后一个
      beforeId = newGroups[newIndex - 1].id;
      afterId = newGroups[newIndex + 1].id;
    }

    handleMoveGroup(movedId, beforeId, afterId);
  };

  return (
    <Card
      className="w-80 h-full flex flex-col shadow-sm border-gray-200"
      styles={{ body: { padding: 0, height: '100%', display: 'flex', flexDirection: 'column' } }}
    >
      <div className="p-4 border-b border-gray-100 bg-white z-10">
        <div className="flex justify-between items-center mb-3">
          <span className="font-bold text-gray-700">配置分组</span>
          {canCreateGroup ? (
            <Button type="primary" shape="circle" size="small" icon={<PlusOutlined />} onClick={onCreateClick} />
          ) : null}
        </div>
        <Input
          prefix={<SearchOutlined className="text-gray-400" />}
          placeholder="搜索配置分组..."
          variant="filled"
          value={searchTerm}
          onChange={e => setSearchTerm(e.target.value)}
          onPressEnter={() => {
            setGroupPageNum(1);
            fetchGroups({ pageNum: 1, keyword: searchTerm });
          }}
          allowClear
        />
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-2 scrollbar-hide">
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          {Object.entries(groupedGroups).map(([moduleName, list]) => (
            <div key={moduleName} className="mb-4 rounded-lg">
              <div className="px-3 py-1 flex items-center justify-between group/header">
                <div className="text-xs font-bold text-gray-400 uppercase tracking-wider mb-1 flex-1">
                  <InlineTextEdit
                    value={moduleName}
                    confirm={true}
                    confirmMessage="修改模块名会自动合并相同模块下的分组。"
                    onChange={val => {
                      // 批量更新模块名
                      list.forEach(group => {
                        if (canEditGroup && group.access?.canWrite !== false) {
                          handleUpdateGroup(group.id, 'module', val);
                        }
                      });
                    }}
                    disabled={!canEditGroup || list.every(group => group.access?.canWrite === false)}
                    textClassName="hover:text-indigo-600 cursor-pointer"
                  />{' '}
                  模块
                </div>
              </div>

              <SortableContext items={list.map(g => g.id)} strategy={verticalListSortingStrategy}>
                <div className="space-y-1 px-1 pb-1">
                  {list.map(group => (
                    <SortableGroupItem
                      key={group.id}
                      group={group}
                      isActive={activeGroup?.id === group.id}
                      onSelect={() => setActiveGroup(group)}
                      onDelete={e => {
                        e.stopPropagation();
                        handleDeleteGroup(group.id);
                      }}
                      onUpdateField={(field, value) => handleUpdateGroup(group.id, field, value)}
                      canWrite={canEditGroup && group.access?.canWrite !== false}
                      canDelete={canDeleteGroup && group.access?.canDelete !== false}
                    />
                  ))}
                </div>
              </SortableContext>
            </div>
          ))}
        </DndContext>
        {Object.keys(groupedGroups).length === 0 && (
          <ActionEmptyState
            icon={<SettingOutlined />}
            title={hasAnyGroup ? '没有匹配的配置分组' : '暂无配置分组'}
            description={
              hasAnyGroup
                ? '当前搜索条件没有命中分组，请调整关键词。'
                : '点击右上角“+”创建第一个配置分组。'
            }
            actionText={hasAnyGroup ? '清空搜索' : canCreateGroup ? '新建分组' : undefined}
            onAction={
              hasAnyGroup
                ? () => {
                    setSearchTerm('');
                    setGroupPageNum(1);
                    fetchGroups({ pageNum: 1, keyword: '' });
                  }
                : canCreateGroup ? onCreateClick : undefined
            }
          />
        )}
      </div>
      <div className="border-t border-gray-100 px-3 py-2 bg-white">
        <Pagination
          size="small"
          current={groupPageNum}
          pageSize={groupPageSize}
          total={groupTotal}
          showSizeChanger
          pageSizeOptions={[10, 20, 50]}
          showTotal={total => `共 ${total} 个`}
          onChange={(pageNum, pageSize) => {
            setGroupPageNum(pageNum);
            setGroupPageSize(pageSize);
            fetchGroups({ pageNum, pageSize, keyword: searchTerm });
          }}
        />
      </div>
    </Card>
  );
};
