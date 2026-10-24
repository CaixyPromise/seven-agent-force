'use client';

import React, { useState } from 'react';
import { Card, Button, Input, InputNumber, Pagination, Tag, Popconfirm, Tooltip } from 'antd';
import { PlusOutlined, SearchOutlined, DeleteOutlined, HolderOutlined, BookOutlined } from '@ant-design/icons';
import { DndContext, closestCenter, DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { useSortable, SortableContext, verticalListSortingStrategy } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { InlineTextEdit } from '@/components/InlineTextEdit';
import { ActionEmptyState } from '@/components/empty-state/ActionEmptyState';
import { usePermissionAccess } from '@/hooks/auth';
import { DICT_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { useDictContext } from '../context/useDictContext';
import type { DictType } from '@/types/dict';

// 可编辑的排序序号组件
const EditableSortOrder: React.FC<{
  value: number;
  onChange: (val: number) => void;
  disabled?: boolean;
}> = ({ value, onChange, disabled }) => {
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
        onClick={e => e.stopPropagation()}
      />
    );
  }

  return (
    <Tooltip title={disabled ? '无字典编辑权限' : '点击修改排序'} placement="top">
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

interface DictTypeSidebarProps {
  onCreateClick: () => void;
}

interface SortableItemProps {
  type: DictType;
  isSelected: boolean;
  onSelect: () => void;
  onDelete: (e?: React.MouseEvent<HTMLElement>) => void;
  onUpdateField: <K extends keyof DictType>(field: K, value: DictType[K]) => void;
  canEdit: boolean;
  canDelete: boolean;
}

const SortableTypeItem: React.FC<SortableItemProps> = ({
  type,
  isSelected,
  onSelect,
  onDelete,
  onUpdateField,
  canEdit,
  canDelete,
}) => {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: type.id,
    disabled: !canEdit,
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
        ${isSelected ? 'bg-indigo-50 border-indigo-500' : 'hover:bg-gray-50 border-transparent'}
      `}
      onClick={onSelect}
    >
      {canEdit ? (
        <div
          {...attributes}
          {...listeners}
          className="mt-1 text-gray-300 cursor-move hover:text-gray-500"
        >
          <HolderOutlined />
        </div>
      ) : null}
      <div className="flex-1 min-w-0 overflow-hidden">
        <div className="flex justify-between items-start">
          <div className="flex-1 min-w-0 overflow-hidden pr-6">
            <div className={`text-sm font-medium mb-1 flex min-w-0 items-center gap-2 ${isSelected ? 'text-indigo-700' : 'text-gray-700'}`}>
              <InlineTextEdit
                value={type.dictName}
                disabled={!canEdit}
                onChange={val => onUpdateField('dictName', String(val))}
                textClassName="block max-w-full truncate"
              />
              <EditableSortOrder
                value={type.sortOrder ?? 0}
                disabled={!canEdit}
                onChange={val => onUpdateField('sortOrder', Number(val))}
              />
            </div>
            <div className="text-xs text-gray-400 font-mono flex min-w-0 items-center gap-1">
              <Tag
                color={type.isSystem === 1 ? 'gold' : 'blue'}
                className="mr-0"
                style={{ fontSize: 9, padding: '0 4px', lineHeight: '14px', height: '16px' }}
              >
                {type.isSystem === 1 ? '系统' : '业务'}
              </Tag>
              <InlineTextEdit
                value={type.dictCode}
                disabled={!canEdit}
                confirm={true}
                onChange={val => onUpdateField('dictCode', String(val))}
                textClassName="block max-w-full truncate hover:text-indigo-600 cursor-pointer"
              />
            </div>
          </div>
          {canDelete ? <div
            className="absolute right-2 top-3 opacity-0 group-hover:opacity-100 transition-opacity"
            onClick={e => e.stopPropagation()}
          >
            <Popconfirm
              title="确认删除?"
              onConfirm={onDelete}
              okType="danger"
              disabled={type.isSystem === 1}
            >
              <Button
                type="text"
                danger
                size="small"
                icon={<DeleteOutlined />}
                disabled={type.isSystem === 1}
                className="bg-white/80 backdrop-blur"
              />
            </Popconfirm>
          </div> : null}
        </div>
      </div>
    </div>
  );
};

export const DictTypeSidebar: React.FC<DictTypeSidebarProps> = ({ onCreateClick }) => {
  const canAdd = usePermissionAccess(DICT_PERMISSIONS.ADD);
  const canEdit = usePermissionAccess(DICT_PERMISSIONS.EDIT);
  const canDelete = usePermissionAccess(DICT_PERMISSIONS.DELETE);
  const {
    types,
    typeTotal,
    typePageNum,
    typePageSize,
    groupedTypes,
    selectedType,
    setSelectedType,
    fetchTypes,
    setTypePageNum,
    setTypePageSize,
    handleUpdateType,
    handleDeleteType,
    handleMoveType,
    searchTerm,
    setSearchTerm,
  } = useDictContext();
  const hasAnyType = types.length > 0;

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    if (!canEdit) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    // 找到移动前后的索引
    const oldIndex = types.findIndex(t => t.id === active.id);
    const newIndex = types.findIndex(t => t.id === over.id);
    if (oldIndex === -1 || newIndex === -1) return;

    // 模拟移动后的数组，计算beforeId/afterId
    const newTypes = [...types];
    const [movedType] = newTypes.splice(oldIndex, 1);
    newTypes.splice(newIndex, 0, movedType);

    let beforeId: API.Int64 | null = null;
    let afterId: API.Int64 | null = null;

    if (newIndex === 0) {
      // 移到最前：afterId = 新位置后一个元素
      afterId = newTypes.length > 1 ? newTypes[1].id : null;
    } else if (newIndex === newTypes.length - 1) {
      // 移到最后：beforeId = 新位置前一个元素
      beforeId = newTypes[newIndex - 1].id;
    } else {
      // 插到中间：beforeId = 前一个，afterId = 后一个
      beforeId = newTypes[newIndex - 1].id;
      afterId = newTypes[newIndex + 1].id;
    }

    handleMoveType(String(active.id), beforeId, afterId);
  };

  return (
    <Card
      className="w-80 h-full flex flex-col shadow-sm border-gray-200"
      styles={{ body: { padding: 0, height: '100%', display: 'flex', flexDirection: 'column' } }}
    >
      <div className="p-4 border-b border-gray-100 bg-white z-10">
        <div className="flex justify-between items-center mb-3">
          <span className="font-bold text-gray-700">字典集合</span>
          {canAdd ? (
            <Button
              type="primary"
              shape="circle"
              size="small"
              icon={<PlusOutlined />}
              onClick={onCreateClick}
            />
          ) : null}
        </div>
        <Input
          prefix={<SearchOutlined className="text-gray-400" />}
          placeholder="搜索字典名称/编码"
          variant="filled"
          value={searchTerm}
          onChange={e => setSearchTerm(e.target.value)}
          onPressEnter={() => {
            setTypePageNum(1);
            fetchTypes({ pageNum: 1, keyword: searchTerm });
          }}
          allowClear
          onClear={() => {
            setSearchTerm('');
            setTypePageNum(1);
            fetchTypes({ pageNum: 1, keyword: '' });
          }}
        />
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-2 scrollbar-hide">
        <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
          {Object.entries(groupedTypes).map(([moduleName, list]) => (
            <div key={moduleName} className="mb-4 rounded-lg">
              <div className="px-3 py-1 flex items-center justify-between text-xs font-bold text-gray-400 uppercase tracking-wider mb-1">
                <div className="flex-1">
                  <InlineTextEdit
                    value={moduleName}
                    disabled={!canEdit}
                    confirm={true}
                    confirmMessage="修改模块名会将该模块下所有字典迁移至新模块。"
                    onChange={val => {
                      // 批量更新模块名
                      list.forEach(type => {
                        handleUpdateType(type.id, 'module', String(val));
                      });
                    }}
                    textClassName="hover:text-indigo-600 cursor-pointer"
                  />{' '}
                  模块
                </div>
              </div>
              <SortableContext items={list.map(t => t.id || t.dictCode)} strategy={verticalListSortingStrategy}>
                <div className="space-y-1">
                  {list.map(type => (
                    <SortableTypeItem
                      key={type.id || type.dictCode}
                      type={type}
                      isSelected={selectedType?.id === type.id}
                      onSelect={() => setSelectedType(type)}
                      onDelete={(e) => {
                        e?.stopPropagation();
                        if (type.isSystem === 1) {
                          return;
                        }
                        handleDeleteType(type.id);
                      }}
                      onUpdateField={(field, value) => handleUpdateType(type.id, field, value)}
                      canEdit={canEdit}
                      canDelete={canDelete && type.isSystem !== 1}
                    />
                  ))}
                </div>
              </SortableContext>
            </div>
          ))}
          {Object.keys(groupedTypes).length === 0 && (
            <ActionEmptyState
              icon={<BookOutlined />}
              title={hasAnyType ? '没有匹配的字典类型' : '暂无字典类型'}
              description={
                hasAnyType
                  ? '当前搜索条件没有命中字典类型，请调整关键词。'
                  : '点击右上角“+”创建第一个字典类型。'
              }
              actionText={hasAnyType ? '清空搜索' : canAdd ? '新建字典类型' : undefined}
              onAction={
                hasAnyType
                  ? () => {
                      setSearchTerm('');
                      setTypePageNum(1);
                      fetchTypes({ pageNum: 1, keyword: '' });
                    }
                  : canAdd ? onCreateClick : undefined
              }
            />
          )}
        </DndContext>
      </div>
      <div className="border-t border-gray-100 px-3 py-2 bg-white">
        <Pagination
          size="small"
          current={typePageNum}
          pageSize={typePageSize}
          total={typeTotal}
          showSizeChanger
          pageSizeOptions={[10, 20, 50]}
          showTotal={total => `共 ${total} 个`}
          onChange={(pageNum, pageSize) => {
            setTypePageNum(pageNum);
            setTypePageSize(pageSize);
            fetchTypes({ pageNum, pageSize, keyword: searchTerm });
          }}
        />
      </div>
    </Card>
  );
};
