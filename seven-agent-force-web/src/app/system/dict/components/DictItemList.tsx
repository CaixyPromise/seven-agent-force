'use client';

import React, { useState, useEffect } from 'react';
import {
  Button,
  Alert,
  Skeleton,
  Switch,
  Divider,
  Popconfirm,
  Input,
  InputNumber,
  Select,
  Tag,
  Dropdown,
  Pagination,
  Tooltip,
  message,
} from 'antd';
import {
  PlusOutlined,
  HolderOutlined,
  DeleteOutlined,
  DownOutlined,
  SafetyCertificateOutlined,
  BlockOutlined,
  BookOutlined,
  SaveOutlined,
  SearchOutlined,
  UndoOutlined,
  LockOutlined,
} from '@ant-design/icons';
import { CopyKeyButton } from '@/components/CopyKeyButton';
import { ActionEmptyState } from '@/components/empty-state/ActionEmptyState';
import { usePermissionAccess } from '@/hooks/auth';
import { DICT_PERMISSIONS } from '@/lib/auth/permissionCodes';

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
        className={`flex items-center justify-center min-w-[24px] h-6 px-1.5 rounded bg-gray-100 text-xs font-mono text-gray-500 transition-colors select-none ${
          disabled ? 'cursor-not-allowed opacity-50' : 'hover:bg-indigo-100 hover:text-indigo-600 cursor-pointer'
        }`}
      >
        {value}
      </div>
    </Tooltip>
  );
};
import { DndContext, closestCenter, DragEndEvent, PointerSensor, useSensor, useSensors } from '@dnd-kit/core';
import { SortableContext, verticalListSortingStrategy, useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { InlineTextEdit } from '@/components/InlineTextEdit';
import { useDictContext } from '../context/useDictContext';
import type { DictItem } from '@/types/dict';

interface SortableDictItemProps {
  item: DictItem;
  dictCode?: string;
  isNew?: boolean;
  onUpdate: (item: DictItem) => Promise<void>;
  onCreate: (item: DictItem) => Promise<void>;
  onDelete: () => void;
  onCancel: () => void;
  canEdit: boolean;
  canDelete: boolean;
}

const SortableDictItem: React.FC<SortableDictItemProps> = ({
  item,
  dictCode,
  isNew = false,
  onUpdate,
  onCreate,
  onDelete,
  onCancel,
  canEdit,
  canDelete,
}) => {
  const [draft, setDraft] = useState({ ...item });
  const [isDirty, setIsDirty] = useState(isNew);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  // 当 item 变化时更新 draft（排除本地修改）
  useEffect(() => {
    if (!isDirty) {
      setDraft({ ...item });
    }
  }, [item, isDirty]);

  const handleUpdate = (field: keyof DictItem, value: unknown) => {
    setDraft(prev => ({ ...prev, [field]: value }));
    setIsDirty(true);
    setSaveError(null);
  };

  const handleSave = async () => {
    if (!draft.itemLabel || !draft.itemValue) {
      message.error('名称和键值不能为空');
      return;
    }

    setIsSaving(true);
    try {
      if (isNew) {
        await onCreate(draft);
      } else {
        // 一次性提交所有变更（完整的item对象）
        await onUpdate(draft);
        setIsDirty(false);
      }
    } catch (error: unknown) {
      setSaveError(error instanceof Error && error.message ? error.message : '保存失败，请重试');
    } finally {
      setIsSaving(false);
    }
  };

  const handleCancel = () => {
    if (isNew) {
      onCancel();
    } else {
      setDraft({ ...item });
      setIsDirty(false);
    }
  };
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: item.id,
    disabled: !canEdit,
  });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={`
        group relative p-3 border rounded-lg hover:border-indigo-300 hover:shadow-sm transition-all bg-white
        flex items-stretch gap-3
        ${isNew ? 'border-indigo-300 shadow-md ring-1 ring-indigo-100' : 'border-gray-100'}
        ${isDragging ? 'opacity-40 border-dashed border-gray-400 bg-gray-50' : ''}
        ${item.status === 0 ? 'bg-gray-50/80 grayscale-[0.5]' : ''}
      `}
    >
      {canEdit ? (
        <div
          {...attributes}
          {...listeners}
          className="flex flex-col justify-center text-gray-300 cursor-move hover:text-indigo-500 px-1 border-r border-gray-50 mr-1"
        >
          <HolderOutlined className="text-lg" />
        </div>
      ) : null}

      <div className="flex-1 flex flex-col justify-center gap-2 min-w-0 py-0.5">
        {saveError ? <Alert type="error" showIcon title={saveError} /> : null}
        <div className="flex items-center flex-wrap gap-4">
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-gray-400 uppercase tracking-wider font-bold bg-gray-100 px-1.5 py-0.5 rounded select-none">
              名称
            </span>
            <div className="font-bold text-gray-700 text-sm hover:text-indigo-600 transition-colors">
              <InlineTextEdit
                value={draft.itemLabel}
                disabled={!canEdit}
                onChange={val => handleUpdate('itemLabel', val)}
                textClassName="px-1 -ml-1 rounded"
                placeholder={isNew ? "请输入名称" : undefined}
                autoSave={false}
              />
            </div>
          </div>
          <Divider orientation="vertical" className="bg-gray-200 h-4 m-0" />
          <div className="flex items-center gap-2">
            <Tooltip title={isNew ? '' : '字典值不可修改，如需修改请删除后重建'}>
              <span className="text-[10px] text-blue-400 uppercase tracking-wider font-bold bg-blue-50 px-1.5 py-0.5 rounded select-none">
                键值 {!isNew && <LockOutlined className="ml-1 text-gray-400" />}
              </span>
            </Tooltip>
            <div className="font-mono text-sm text-blue-600 font-medium">
              <InlineTextEdit
                value={draft.itemValue}
                disabled={!isNew || !canEdit}
                onChange={val => handleUpdate('itemValue', val)}
                textClassName={`px-1 -ml-1 rounded ${isNew ? 'hover:bg-blue-50 cursor-pointer' : 'cursor-not-allowed'}`}
                placeholder={isNew ? "请输入键值" : undefined}
                autoSave={false}
              />
            </div>
          </div>
          <Divider orientation="vertical" className="bg-gray-200 h-4 m-0" />
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-gray-400 uppercase tracking-wider font-bold bg-gray-100 px-1.5 py-0.5 rounded select-none">
              序号
            </span>
            <EditableSortOrder
              value={draft.sortOrder ?? 0}
              disabled={!canEdit}
              onChange={val => handleUpdate('sortOrder', val)}
            />
          </div>
          {!isNew && dictCode && draft.itemValue && (
            <>
              <Divider orientation="vertical" className="bg-gray-200 h-4 m-0" />
              <CopyKeyButton
                options={[
                  {
                    label: '字典编码 (dictCode)',
                    value: dictCode,
                    description: '使用 useDictValue(dictCode) 获取整个字典',
                  },
                  {
                    label: '字典值 (itemValue)',
                    value: draft.itemValue,
                    description: '当前字典项的值',
                  },
                  {
                    label: '获取标签示例',
                    value: `useDictLabel('${dictCode}', '${draft.itemValue}')`,
                    description: '根据值获取标签的Hook调用方式',
                  },
                ]}
              />
            </>
          )}
        </div>
        <div className="flex items-center gap-2 w-full">
          <span className="text-[10px] text-gray-400 select-none">描述:</span>
          <div className="flex-1 text-xs text-gray-500 bg-gray-50 px-2 py-1 rounded hover:bg-gray-100 transition-colors truncate">
            <InlineTextEdit
              value={draft.itemDesc || ''}
              disabled={!canEdit}
              placeholder="暂无描述信息..."
              onChange={val => handleUpdate('itemDesc', val)}
              textClassName="w-full block"
              autoSave={false}
            />
          </div>
        </div>
      </div>

      <div className="flex flex-col items-end justify-between pl-3 border-l border-gray-100 gap-2 min-w-[100px]">
        <div className="flex items-center gap-2" title="启用状态">
          <span className={`text-[10px] ${draft.status === 1 ? 'text-green-500' : 'text-gray-400'}`}>
            {draft.status === 1 ? '已启用' : '已禁用'}
          </span>
          <Switch
            size="small"
            disabled={!canEdit}
            checked={draft.status === 1}
            onChange={c => handleUpdate('status', c ? 1 : 0)}
          />
        </div>
        <div className="flex items-center gap-1">
          {isDirty && canEdit && (
            <>
              <Button
                size="small"
                onClick={handleCancel}
                icon={isNew ? <DeleteOutlined /> : <UndoOutlined />}
                disabled={isSaving}
              >
                {isNew ? '删除' : '取消'}
              </Button>
              <Button
                size="small"
                type="primary"
                onClick={handleSave}
                icon={<SaveOutlined />}
                loading={isSaving}
              >
                保存
              </Button>
            </>
          )}
          <Select
            size="small"
            allowClear
            className="w-20"
            placeholder="颜色"
            value={draft.colorToken}
            disabled={!canEdit}
            options={['gray', 'blue', 'pink', 'green', 'orange', 'red', 'purple'].map(value => ({ value, label: value }))}
            onChange={value => handleUpdate('colorToken', value)}
          />
          <Select
            size="small"
            allowClear
            className="w-20"
            placeholder="图标"
            value={draft.iconToken}
            disabled={!canEdit}
            options={['unknown', 'male', 'female', 'check', 'close', 'info'].map(value => ({ value, label: value }))}
            onChange={value => handleUpdate('iconToken', value)}
          />
          {!isNew && canDelete ? <Popconfirm
            title="删除此字典项?"
            description="删除后无法恢复，确认操作？"
            onConfirm={onDelete}
            okType="danger"
            placement="topRight"
          >
            <Button
              size="small"
              type="text"
              danger
              icon={<DeleteOutlined />}
              className="text-gray-400 hover:text-red-500 hover:bg-red-50"
            />
          </Popconfirm> : null}
        </div>
      </div>
    </div>
  );
};

export const DictItemList: React.FC = () => {
  const canAdd = usePermissionAccess(DICT_PERMISSIONS.ADD);
  const canEdit = usePermissionAccess(DICT_PERMISSIONS.EDIT);
  const canDelete = usePermissionAccess(DICT_PERMISSIONS.DELETE);
  const {
    selectedType,
    items,
    itemTotal,
    itemPageNum,
    itemPageSize,
    itemSearchTerm,
    loadingItems,
    fetchItems,
    setItemSearchTerm,
    setItemPageNum,
    setItemPageSize,
    handleUpdateType,
    handleCreateItem,
    handleUpdateItem,
    handleDeleteItem,
    handleMoveItem,
    addTempItem,
    removeTempItem,
  } = useDictContext();

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  );

  const handleDragEnd = (event: DragEndEvent) => {
    if (!canEdit) return;
    const { active, over } = event;
    if (!over || active.id === over.id) return;

    const movedItemId = String(active.id);
    const targetItemId = String(over.id);

    const oldIndex = items.findIndex(item => item.id === movedItemId);
    const newIndex = items.findIndex(item => item.id === targetItemId);

    if (oldIndex === -1 || newIndex === -1) return;

    // 模拟移动后的新数组，用于计算beforeId/afterId
    const newItems = [...items];
    const [movedItem] = newItems.splice(oldIndex, 1);
    newItems.splice(newIndex, 0, movedItem);

    // 按规范计算 beforeId 和 afterId
    let beforeId: API.Int64 | null = null;
    let afterId: API.Int64 | null = null;

    if (newIndex === 0) {
      // 移到最前：afterId = 新位置后一个元素
      afterId = newItems.length > 1 ? newItems[1].id : null;
    } else if (newIndex === newItems.length - 1) {
      // 移到最后：beforeId = 新位置前一个元素
      beforeId = newItems[newIndex - 1].id;
    } else {
      // 插到中间：beforeId = 前一个，afterId = 后一个
      beforeId = newItems[newIndex - 1].id;
      afterId = newItems[newIndex + 1].id;
    }

    // 调用 move 接口
    handleMoveItem(movedItemId, beforeId, afterId);
  };

  const handleAddItem = () => {
    if (!selectedType) return;
    // 创建本地临时ID（负十进制字符串，不会提交给后端）
    const tempId = `-${Date.now()}`;
    const maxSort = items.length > 0 ? Math.max(...items.map(i => i.sortOrder || 0), 0) + 1 : 1;
    const newItem: DictItem = {
      id: tempId,
      dictTypeId: selectedType.id,
      itemLabel: '',
      itemValue: '',
      status: 1,
      sortOrder: maxSort,
      colorToken: undefined,
      iconToken: undefined,
      presentationVersion: 1,
      version: '1',
      itemDesc: ''
    };
    // 添加到本地列表（虚拟的）
    addTempItem(newItem);
  };

  const handleSearchItems = () => {
    if (!selectedType) return;
    setItemPageNum(1);
    fetchItems(selectedType.id, { pageNum: 1, keyword: itemSearchTerm });
  };

  const clearItemSearch = () => {
    if (!selectedType) return;
    setItemSearchTerm('');
    setItemPageNum(1);
    fetchItems(selectedType.id, { pageNum: 1, keyword: '' });
  };

  if (!selectedType) {
    return (
      <div className="h-full flex items-center justify-center">
        <ActionEmptyState
          icon={<BookOutlined />}
          title="请选择左侧字典类型"
          description="选择一个字典类型后，这里会显示对应字典项。"
        />
      </div>
    );
  }

  return (
    <div className="flex-1 min-w-0 bg-white rounded-lg shadow-sm border border-gray-200 h-full flex flex-col overflow-hidden">
      <div className="px-6 py-4 border-b border-gray-100 bg-gray-50/30 space-y-3">
        <div className="flex min-w-0 justify-between gap-4 items-start">
          <div className="flex-1 min-w-0">
            <div className="flex min-w-0 flex-wrap items-center gap-3 mb-1">
              <h2 className="min-w-0 max-w-full text-xl font-bold text-gray-800 m-0">
                <InlineTextEdit
                  value={selectedType.dictName}
                  onChange={val => handleUpdateType(selectedType.id, 'dictName', String(val))}
                  textClassName="block max-w-full break-words"
                />
              </h2>
              <Dropdown
                menu={{
                  items: [
                    {
                      key: '1',
                      label: (
                        <span className="text-orange-500 font-medium">
                          <SafetyCertificateOutlined /> 系统内置
                        </span>
                      ),
                    },
                    {
                      key: '0',
                      label: (
                        <span className="text-blue-500 font-medium">
                          <BlockOutlined /> 业务字典
                        </span>
                      ),
                    },
                  ],
                  onClick: ({ key }) => {
                    if (canEdit) void handleUpdateType(selectedType.id, 'isSystem', parseInt(key));
                  },
                }}
                trigger={['click']}
              >
                <Tag
                  className={`${canEdit ? 'cursor-pointer hover:opacity-80' : 'cursor-not-allowed opacity-70'} select-none transition-all`}
                  color={selectedType.isSystem ? 'orange' : 'blue'}
                >
                  {selectedType.isSystem ? '系统内置' : '业务字典'}{' '}
                  <DownOutlined className="text-[10px] ml-1 opacity-70" />
                </Tag>
              </Dropdown>
            </div>
            <div className="flex min-w-0 flex-wrap items-center gap-4 text-gray-500 text-sm">
              <span className="font-mono bg-gray-100 px-1 rounded flex min-w-0 max-w-full items-center gap-1">
                <span className="min-w-0 break-all">{selectedType.dictCode}</span>
                <CopyKeyButton
                  size="small"
                  options={[
                    {
                      label: '字典编码 (dictCode)',
                      value: selectedType.dictCode,
                      description: '使用 useDictValue(dictCode) 获取字典',
                    },
                    {
                      label: 'Hook 调用示例',
                      value: `const items = useDictValue('${selectedType.dictCode}');`,
                      description: '完整的Hook调用代码',
                    },
                    {
                      label: '下拉选项示例',
                      value: `const options = useDictOptions('${selectedType.dictCode}');`,
                      description: '获取 {value, label} 格式的选项列表',
                    },
                  ]}
                />
              </span>
              <span className="flex min-w-0 items-center gap-1">
                <span className="shrink-0 opacity-60">描述:</span>
                <InlineTextEdit
                  value={selectedType.dictDesc || ''}
                  disabled={!canEdit}
                  placeholder="添加描述..."
                  onChange={val => handleUpdateType(selectedType.id, 'dictDesc', String(val))}
                  textClassName="block max-w-full break-words border-b border-dashed border-gray-300"
                />
              </span>
            </div>
          </div>
          {canAdd ? (
            <Button type="primary" icon={<PlusOutlined />} onClick={handleAddItem}>
              新增选项
            </Button>
          ) : null}
        </div>
        <div className="flex gap-3">
          <Input
            value={itemSearchTerm}
            placeholder="搜索选项名称/键值"
            allowClear
            onChange={e => setItemSearchTerm(e.target.value)}
            onPressEnter={handleSearchItems}
            prefix={<SearchOutlined className="text-gray-400" />}
          />
          <Button type="primary" onClick={handleSearchItems}>
            搜索
          </Button>
          {itemSearchTerm.trim() && <Button onClick={clearItemSearch}>清空筛选</Button>}
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden p-6 bg-white">
        {loadingItems ? (
          <Skeleton active />
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <SortableContext items={items.map(i => i.id || i.itemValue)} strategy={verticalListSortingStrategy}>
              <div className="space-y-3">
                {items.map(item => (
                  <SortableDictItem
                    key={item.id || `${selectedType?.id || 'dict'}-${item.itemValue}`}
                    item={item}
                    dictCode={selectedType?.dictCode}
                    isNew={item.id.startsWith('-')}
                    onUpdate={handleUpdateItem}
                    onCreate={handleCreateItem}
                    onDelete={() => handleDeleteItem(item.id)}
                    onCancel={() => removeTempItem(item.id)}
                    canEdit={item.id.startsWith('-') ? canAdd : canEdit}
                    canDelete={canDelete}
                  />
                ))}
              </div>
            </SortableContext>
          </DndContext>
        )}
        {!loadingItems && items.length === 0 && (
          <ActionEmptyState
            icon={<PlusOutlined />}
            title={itemSearchTerm.trim() ? '未找到匹配的字典项' : '暂无字典项'}
            description={
              itemSearchTerm.trim()
                ? '调整搜索关键字后重试。'
                : '点击右上角“新增选项”按钮，创建该类型下的第一个字典项。'
            }
            actionText={itemSearchTerm.trim() ? '清空筛选' : canAdd ? '新增选项' : undefined}
            onAction={itemSearchTerm.trim() ? clearItemSearch : canAdd ? handleAddItem : undefined}
          />
        )}
      </div>
      <div className="px-6 py-4 border-t border-gray-100 bg-white flex justify-end">
        <Pagination
          current={itemPageNum}
          pageSize={itemPageSize}
          total={itemTotal}
          showTotal={total => `共 ${total} 条`}
          showSizeChanger
          pageSizeOptions={[10, 20, 50, 100]}
          onChange={(pageNum, pageSize) => {
            setItemPageNum(pageNum);
            setItemPageSize(pageSize);
            fetchItems(selectedType.id, { pageNum, pageSize, keyword: itemSearchTerm });
          }}
          onShowSizeChange={(pageNum, pageSize) => {
            setItemPageNum(pageNum);
            setItemPageSize(pageSize);
            fetchItems(selectedType.id, { pageNum, pageSize, keyword: itemSearchTerm });
          }}
        />
      </div>
    </div>
  );
};
