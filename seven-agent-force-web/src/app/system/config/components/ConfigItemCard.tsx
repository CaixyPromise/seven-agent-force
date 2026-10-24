'use client';

import React, { useCallback, useState, useEffect } from 'react';
import {
  Button,
  Input,
  InputNumber,
  Select,
  Switch,
  DatePicker,
  ColorPicker,
  Tag,
  Alert,
  Popconfirm,
  Dropdown,
  Checkbox,
  Tooltip,
  message,
} from 'antd';
import {
  SaveOutlined,
  UndoOutlined,
  DeleteOutlined,
  SettingOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  ThunderboltOutlined,
  PoweroffOutlined,
  LockOutlined,
  ClockCircleOutlined,
  SafetyCertificateOutlined,
  HistoryOutlined,
} from '@ant-design/icons';
import { motion, AnimatePresence } from 'framer-motion';
import dayjs from 'dayjs';
import { InlineTextEdit } from '@/components/InlineTextEdit';
import { CopyKeyButton } from '@/components/CopyKeyButton';
import type { ConfigItem } from '@/types/config';
import { VALUE_TYPE_OPTION } from '@/app/system/config/const.d';
import { revealSensitiveConfigValue } from '@/api/configController';
import { buildRevealKeyContext, decryptSensitiveValue } from '@/utils/rsaSensitiveReveal';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { ControlledJsonEditor } from './ControlledJsonEditor';
import { ScalarValidationEditor } from './ScalarValidationEditor';
import { validateScalarValidation } from './scalarValidation';
import { ConfigAssetEditor } from './ConfigAssetEditor';

const { TextArea } = Input;

interface ConfigItemCardProps {
  config: ConfigItem;
  groupCode?: string;
  isNew?: boolean;
  onSave: (config: Partial<ConfigItem> & Pick<ConfigItem, 'id'> & {
    assetFileId?: API.Int64;
    clearAsset?: boolean;
  }) => Promise<void>;
  onCreate: (config: ConfigItem & { assetFileId?: API.Int64 }) => Promise<void>;
  onDelete: (id: API.Int64) => void;
  onCancel: (id: API.Int64) => void;
  onViewHistory?: (configId: API.Int64, configKey: string) => void;
}

function isConfigAssetValueType(valueType: ConfigItem['valueType']): valueType is 'IMAGE' | 'FILE' {
  return valueType === 'IMAGE' || valueType === 'FILE';
}

export const ConfigItemCard: React.FC<ConfigItemCardProps> = ({ config, groupCode, isNew = false, onSave, onCreate, onDelete, onCancel, onViewHistory }) => {
  const hasEditPermission = usePermissionAccess(CONFIG_PERMISSIONS.EDIT);
  const hasDeletePermission = usePermissionAccess(CONFIG_PERMISSIONS.DELETE);
  const hasSensitivePermission = usePermissionAccess(CONFIG_PERMISSIONS.SENSITIVE);
  const [draft, setDraft] = useState({ ...config });
  const [isDirty, setIsDirty] = useState(isNew);
  const [error, setError] = useState<string | null>(null);
  const [showSensitive, setShowSensitive] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [sensitiveLoading, setSensitiveLoading] = useState(false);
  const [sensitiveValueLoaded, setSensitiveValueLoaded] = useState(false);
  const [jsonDraftError, setJsonDraftError] = useState<string | null>(null);
  const [assetFileId, setAssetFileId] = useState<API.Int64>();
  const [assetFileName, setAssetFileName] = useState<string>();
  const [clearAsset, setClearAsset] = useState(false);

  useEffect(() => {
    setDraft({ ...config });
    setIsDirty(isNew);
    setError(null);
    setShowSensitive(false);
    setSensitiveValueLoaded(false);
    setJsonDraftError(null);
    setAssetFileId(undefined);
    setAssetFileName(undefined);
    setClearAsset(false);
  }, [config, isNew]);

  const updateField = (field: keyof ConfigItem, value: unknown) => {
    setDraft(prev => ({ ...prev, [field]: value }));
    setIsDirty(true);
    if (error) setError(null);
  };

  const handleTypeChange = (newType: ConfigItem['valueType']) => {
    const wasAsset = isConfigAssetValueType(draft.valueType);
    const nextIsAsset = isConfigAssetValueType(newType);
    let newValue = draft.configValue;
    if (newType === 'JSON') {
      newValue = '{}';
    } else if (newType === 'BOOLEAN') {
      newValue = 'false';
    } else if (newType === 'MULTI_ENUM') {
      newValue = '[]';
    }
    if (wasAsset || nextIsAsset) {
      newValue = '';
    }
    const widgetByType: Record<ConfigItem['valueType'], ConfigItem['uiWidget']> = {
      STRING: 'INPUT',
      TEXT: 'TEXTAREA',
      INTEGER: 'INPUT_NUMBER',
      DECIMAL: 'INPUT_NUMBER',
      BOOLEAN: 'SWITCH',
      ENUM: 'SELECT',
      MULTI_ENUM: 'MULTI_SELECT',
      DATE: 'DATE_PICKER',
      DATETIME: 'DATETIME_PICKER',
      DURATION: 'DURATION_INPUT',
      COLOR: 'COLOR_PICKER',
      JSON: 'CONTROLLED_JSON',
      IMAGE: 'IMAGE_UPLOAD',
      FILE: 'FILE_UPLOAD',
    };
    setAssetFileId(undefined);
    setAssetFileName(undefined);
    setClearAsset(wasAsset && !nextIsAsset);
    setDraft(prev => ({
      ...prev,
      valueType: newType,
      uiWidget: widgetByType[newType],
      configValue: newValue,
      effectType: nextIsAsset ? 'realtime' : prev.effectType,
      sensitivity: nextIsAsset ? 'NORMAL' : prev.sensitivity,
      isSensitive: nextIsAsset ? 0 : prev.isSensitive,
      validation: newType === 'ENUM' || newType === 'MULTI_ENUM'
        ? { ...prev.validation, options: prev.validation?.options ?? [] }
        : nextIsAsset
          ? prev.validation?.required ? { required: true } : undefined
        : prev.validation
          ? { ...prev.validation, options: undefined, maxItems: undefined }
          : undefined,
    }));
    setIsDirty(true);
  };

  const handleSubmit = async () => {
    if (!draft.configKey) return setError('Key 不能为空');
    if (!draft.configDesc) return setError('配置描述不能为空');
    if (draft.valueType === 'JSON' && jsonDraftError) return setError(jsonDraftError);
    const validationError = validateScalarValidation(draft.valueType, draft.validation);
    if (validationError) return setError(validationError);
    const isAsset = isConfigAssetValueType(draft.valueType);
    const needsNewAsset = isAsset && (
      isNew
      || !isConfigAssetValueType(config.valueType)
      || config.valueType !== draft.valueType
    );
    if (needsNewAsset && !assetFileId) {
      return setError('请先上传符合要求的文件，再保存配置资产');
    }
    const sensitiveValueHidden = !isNew && draft.sensitivity !== 'NORMAL' && !sensitiveValueLoaded;
    if (!sensitiveValueHidden && draft.valueType === 'INTEGER') {
      if (draft.configValue === '' || isNaN(Number(draft.configValue))) return setError('请输入有效的数字');
    }

    setIsSaving(true);
    try {
      if (isNew) {
        // 新建模式：调用 onCreate
        await onCreate({ ...draft, ...(assetFileId ? { assetFileId } : {}) });
        // 创建成功后不需要设置 isDirty，因为组件会被重新加载
      } else {
        const updateData: Partial<ConfigItem> & Pick<ConfigItem, 'id'> & {
          assetFileId?: API.Int64;
          clearAsset?: boolean;
        } = {
          id: draft.id,
          groupId: draft.groupId,
          configKey: draft.configKey,
          valueType: draft.valueType,
          configDesc: draft.configDesc,
          isSensitive: draft.isSensitive,
          isReadonly: draft.isReadonly,
          isEnabled: draft.isEnabled,
          effectType: draft.effectType,
          uiWidget: draft.uiWidget,
          validation: draft.validation,
          exposure: draft.exposure,
          sensitivity: draft.sensitivity,
          schemaVersion: draft.schemaVersion,
          version: draft.version,
        };
        if (!isAsset && !sensitiveValueHidden) {
          updateData.configValue = draft.configValue;
        }
        if (assetFileId) updateData.assetFileId = assetFileId;
        if (clearAsset) updateData.clearAsset = true;
        await onSave(updateData);
        setIsDirty(false);
      }
    } catch (saveError: unknown) {
      setError(saveError instanceof Error && saveError.message ? saveError.message : '保存失败，请重试');
    } finally {
      setIsSaving(false);
    }
  };

  const handleJSONValidationChange = useCallback((nextError: string | null) => {
    setJsonDraftError(nextError);
  }, []);

  const handleAssetUploaded = (fileId: API.Int64, fileName: string) => {
    setAssetFileId(fileId);
    setAssetFileName(fileName);
    setClearAsset(false);
    setIsDirty(true);
    setError(null);
  };

  const handleClearAsset = () => {
    setAssetFileId(undefined);
    setAssetFileName(undefined);
    setClearAsset(!isNew && isConfigAssetValueType(config.valueType));
    setIsDirty(true);
    setError(null);
  };

  const handleCancel = () => {
    if (isNew) {
      // 新建模式取消：删除临时项
      onCancel(config.id);
    } else {
      // 编辑模式取消：恢复原值
      setDraft({ ...config });
      setIsDirty(false);
      setError(null);
    }
  };

  const fetchSensitivePlainValue = async (): Promise<string | null> => {
    if (draft.sensitivity === 'SECRET') {
      message.error('SECRET 配置为 write-only，只能替换，不能读取');
      return null;
    }
    if (isNew) {
      return draft.configValue;
    }
    setSensitiveLoading(true);
    try {
      const { privateKey, obfuscatedClientPublicKey } = await buildRevealKeyContext();
      const response = await revealSensitiveConfigValue(config.id, obfuscatedClientPublicKey);
      const plainValue = await decryptSensitiveValue(response.encryptedValue, privateKey);
      setDraft(prev => ({ ...prev, configValue: plainValue }));
      setSensitiveValueLoaded(true);
      return plainValue;
    } catch (err: unknown) {
      message.error((err as Error)?.message || '敏感值读取失败');
      return null;
    } finally {
      setSensitiveLoading(false);
    }
  };

  const handleRevealSensitive = async () => {
    const value = await fetchSensitivePlainValue();
    if (value === null) {
      return;
    }
    setShowSensitive(true);
  };

  const handleCopySensitive = async () => {
    const value = await fetchSensitivePlainValue();
    if (value === null) {
      return;
    }
    try {
      await navigator.clipboard.writeText(value);
      message.success('敏感值已复制到剪贴板');
    } catch {
      message.error('复制失败，请手动复制');
    }
  };

  const renderEditor = () => {
    const { valueType, configValue, sensitivity } = draft;
    const isLocked = isEditLocked;

    if (isConfigAssetValueType(valueType)) {
      return (
        <ConfigAssetEditor
          assetType={valueType}
          stablePath={configValue}
          pendingFileName={assetFileName}
          clearRequested={clearAsset}
          disabled={isLocked}
          onUploaded={handleAssetUploaded}
          onClear={handleClearAsset}
        />
      );
    }

    if (sensitivity !== 'NORMAL' && !showSensitive) {
      return (
        <div className="bg-gray-50 border border-gray-200 rounded p-3 flex items-center justify-between">
          <span className="text-gray-400 font-mono">******************</span>
          {hasSensitivePermission && sensitivity === 'SENSITIVE' ? (
            <div className="flex items-center gap-2">
              <Button type="link" size="small" icon={<EyeOutlined />} loading={sensitiveLoading} onClick={handleRevealSensitive}>
                显示明文
              </Button>
              <Button type="link" size="small" loading={sensitiveLoading} onClick={handleCopySensitive}>
                复制 value
              </Button>
            </div>
          ) : sensitivity === 'SECRET' && canEditConfig ? (
            <Button type="link" size="small" onClick={() => {
              setDraft(prev => ({ ...prev, configValue: '' }));
              setSensitiveValueLoaded(true);
              setShowSensitive(true);
            }}>
              替换密文
            </Button>
          ) : null}
        </div>
      );
    }
    if (valueType === 'BOOLEAN') {
      return (
        <div className="py-2">
          <Switch
            disabled={isLocked}
            checked={configValue === 'true'}
            checkedChildren="True"
            unCheckedChildren="False"
            onChange={checked => updateField('configValue', String(checked))}
          />
        </div>
      );
    }
    if (valueType === 'JSON') {
      return (
        <ControlledJsonEditor
          disabled={isLocked}
          value={configValue}
          onChange={value => updateField('configValue', value)}
          onValidationChange={handleJSONValidationChange}
          onDraftChange={() => {
            setIsDirty(true);
            setError(null);
          }}
        />
      );
    }
    if (valueType === 'TEXT') {
      return <TextArea disabled={isLocked} value={configValue} onChange={event => updateField('configValue', event.target.value)} autoSize={{ minRows: 3, maxRows: 8 }} />;
    }
    if (valueType === 'INTEGER' || valueType === 'DECIMAL') {
      return (
        <InputNumber
          className="w-full"
          disabled={isLocked}
          precision={valueType === 'INTEGER' ? 0 : undefined}
          value={configValue === '' ? null : Number(configValue)}
          min={draft.validation?.minValue}
          max={draft.validation?.maxValue}
          onChange={value => updateField('configValue', value === null ? '' : String(value))}
        />
      );
    }
    if (valueType === 'ENUM') {
      return <Select className="w-full" disabled={isLocked} value={configValue || undefined} options={(draft.validation?.options ?? []).map(value => ({ value, label: value }))} onChange={value => updateField('configValue', value)} />;
    }
    if (valueType === 'MULTI_ENUM') {
      let selected: string[] = [];
      try {
        const parsed = JSON.parse(configValue || '[]') as unknown;
        if (!Array.isArray(parsed) || !parsed.every(item => typeof item === 'string')) throw new Error('invalid');
        selected = parsed;
      } catch {
        return <Alert type="error" showIcon title="现有多选枚举值不符合严格数组契约" />;
      }
      return <Select mode="multiple" className="w-full" disabled={isLocked} value={selected} options={(draft.validation?.options ?? []).map(value => ({ value, label: value }))} onChange={value => updateField('configValue', JSON.stringify(value))} />;
    }
    if (valueType === 'DATE' || valueType === 'DATETIME') {
      return <DatePicker className="w-full" showTime={valueType === 'DATETIME'} disabled={isLocked} value={configValue ? dayjs(configValue) : null} onChange={value => updateField('configValue', value ? (valueType === 'DATE' ? value.format('YYYY-MM-DD') : value.toISOString()) : '')} />;
    }
    if (valueType === 'COLOR') {
      return <ColorPicker disabled={isLocked} value={configValue || '#1677FF'} onChange={value => updateField('configValue', value.toHexString().toUpperCase())} />;
    }
    return (
      <div className="relative">
        <Input
          disabled={isLocked}
          value={configValue}
          type="text"
          onChange={e => updateField('configValue', e.target.value)}
          placeholder={valueType === 'DURATION' ? '例如 30s、5m、2h' : '请输入值'}
          suffix={
            sensitivity !== 'NORMAL' ? (
              <EyeInvisibleOutlined
                className="text-gray-400 cursor-pointer hover:text-indigo-500"
                onClick={() => setShowSensitive(false)}
              />
            ) : null
          }
        />
      </div>
    );
  };

  const canWriteByScope = isNew || config.access?.canWrite !== false;
  const canDeleteByScope = config.access?.canDelete !== false;
  const canEditConfig = isNew || (hasEditPermission && canWriteByScope);
  const canDeleteConfig = !isNew && draft.isReadonly !== 1 && hasDeletePermission && canDeleteByScope;
  const isEditLocked = draft.isReadonly === 1 || !canEditConfig;
  const isLocked = isEditLocked;

  return (
    <motion.div
      layout
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      className={`bg-white border rounded-lg p-0 shadow-sm transition-all duration-300 group mb-4 relative overflow-hidden ${
        isDirty ? 'border-indigo-300 shadow-md ring-1 ring-indigo-100' : 'border-gray-100 hover:border-indigo-100'
      } ${draft.isEnabled === 0 ? 'bg-gray-50/50' : ''}`}
    >
      {isLocked && (
        <div className="absolute top-2 right-12 opacity-10 pointer-events-none text-6xl text-gray-400 rotate-12">
          <SafetyCertificateOutlined />
        </div>
      )}
      <div
        className={`flex items-start justify-between px-4 py-3 border-b border-gray-50 rounded-t-lg ${
          draft.isEnabled === 0 ? 'bg-gray-100' : 'bg-gray-50/30'
        }`}
      >
        <div className="flex flex-col gap-1 flex-1 min-w-0 mr-4">
          <div className="flex items-center gap-3">
            <Tooltip title={draft.isEnabled === 1 ? '当前启用' : '当前禁用'}>
              <Popconfirm
                title={draft.isEnabled === 1 ? '确认禁用此配置?' : '确认启用此配置?'}
                onConfirm={() => updateField('isEnabled', draft.isEnabled === 1 ? 0 : 1)}
                okText="确定"
                cancelText="取消"
                disabled={isLocked}
              >
                <Switch
                  size="small"
                  checked={draft.isEnabled === 1}
                  disabled={isLocked}
                  className={draft.isEnabled === 0 ? 'bg-gray-300' : ''}
                />
              </Popconfirm>
            </Tooltip>
            <div className={`font-bold text-base truncate max-w-[60%] ${draft.isEnabled === 0 ? 'text-gray-400 line-through' : 'text-gray-700'}`}>
              <InlineTextEdit
                value={draft.configDesc || ''}
                placeholder="配置描述"
                disabled={isLocked}
                onChange={val => updateField('configDesc', val)}
                textClassName="hover:bg-gray-100 px-1 -ml-1 rounded"
              />
            </div>
            <div className="flex gap-1">
              {draft.isReadonly === 1 && (
                <Tag color="default" className="mr-0">
                  <LockOutlined /> 系统只读
                </Tag>
              )}
              {!canWriteByScope && (
                <Tag color="blue" className="mr-0">
                  只读范围
                </Tag>
              )}
              {draft.effectType === 'restart' && (
                <Tag color="warning" className="mr-0">
                  <PoweroffOutlined /> 重启生效
                </Tag>
              )}
              {draft.effectType === 'realtime' && (
                <Tag color="success" className="mr-0">
                  <ThunderboltOutlined /> 即时生效
                </Tag>
              )}
              <Tag color={draft.connected ? 'success' : 'default'}>
                {draft.connected ? '已连接' : '未连接'}
              </Tag>
            </div>
            {!isNew && draft.configKey && (
              <CopyKeyButton
                options={[
                  {
                    label: '配置 Key',
                    value: draft.configKey,
                    description: '使用 useConfigValue(key) 获取',
                  },
                  ...(groupCode ? [{
                    label: '完整路径 (GroupCode.Key)',
                    value: `${groupCode}.${draft.configKey}`,
                    description: '使用 useConfigValue({ groupCode, key }) 获取',
                  }] : []),
                ]}
              />
            )}
          </div>
          <div className="text-xs text-gray-400 font-mono flex items-center gap-2 mt-1">
            <span className="bg-gray-100 px-1.5 py-0.5 rounded border border-gray-200">
              <span className="select-none text-gray-400 mr-1">KEY:</span>
              <InlineTextEdit
                value={draft.configKey}
                disabled={isLocked}
                confirm={true}
                confirmMessage="修改 Key 会导致代码中的引用失效，请确认。"
                onChange={val => updateField('configKey', val)}
                textClassName={`font-bold ${isLocked ? 'text-gray-500' : 'text-indigo-600 hover:underline cursor-pointer'}`}
              />
            </span>

            <Select
              value={draft.valueType}
              onChange={handleTypeChange}
              disabled={isLocked}
              size="small"
              variant="borderless"
              popupMatchSelectWidth={false}
              className="text-xs w-auto min-w-[70px] bg-white border border-gray-200 rounded hover:border-indigo-300 transition-colors"
              options={VALUE_TYPE_OPTION}
            />
          </div>
        </div>
        <div className="flex items-center gap-2 self-start mt-1 z-10">
          <AnimatePresence>
            {isDirty ? (
              <motion.div
                initial={{ opacity: 0, x: 10 }}
                animate={{ opacity: 1, x: 0 }}
                exit={{ opacity: 0, x: 10 }}
                className="flex items-center gap-2"
              >
                <Button size="small" onClick={handleCancel} icon={<UndoOutlined />} disabled={isSaving}>
                  {isNew ? '删除' : '取消'}
                </Button>
                <Button
                  size="small"
                  type="primary"
                  onClick={handleSubmit}
                  icon={<SaveOutlined />}
                  loading={isSaving}
                  disabled={!canEditConfig}
                >
                  保存
                </Button>
              </motion.div>
            ) : (
              !isNew && (
                <div className="flex items-center gap-1">
                  {onViewHistory && (
                    <Tooltip title="查看变更历史">
                      <Button
                        type="text"
                        size="small"
                        icon={<HistoryOutlined />}
                        onClick={() => onViewHistory(config.id, config.configKey)}
                        className="opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-blue-500"
                      />
                    </Tooltip>
                  )}
                  {canDeleteConfig ? (
                    <Popconfirm title="确认删除?" onConfirm={() => onDelete(config.id)} okType="danger">
                      <Button
                        type="text"
                        danger
                        size="small"
                        icon={<DeleteOutlined />}
                        className="opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-red-500"
                      />
                    </Popconfirm>
                  ) : null}
                </div>
              )
            )}
          </AnimatePresence>
        </div>
      </div>
      <div className="p-4 relative">
        {error && <Alert title={error} type="error" showIcon className="mb-3" />}
        {renderEditor()}
        <div className="mt-3">
          <ScalarValidationEditor
            valueType={draft.valueType}
            value={draft.validation}
            disabled={isLocked}
            onChange={validation => updateField('validation', validation)}
          />
        </div>
      </div>
      <div className="px-4 py-2 bg-gray-50/50 border-t border-gray-100 flex items-center justify-between text-xs text-gray-500">
        <div className="flex items-center gap-4">
          {isConfigAssetValueType(draft.valueType) ? (
            <span className="flex items-center gap-1">
              生效方式: <span className="font-medium text-gray-700">即时</span>
            </span>
          ) : (
            <Dropdown
              disabled={isEditLocked}
              menu={{
                items: [
                  {
                    key: 'realtime',
                    label: '即时生效 (Realtime)',
                    icon: <ThunderboltOutlined className="text-green-500" />,
                  },
                  { key: 'restart', label: '重启生效 (Restart)', icon: <PoweroffOutlined className="text-orange-500" /> },
                ],
                onClick: ({ key }) => updateField('effectType', key),
              }}
              trigger={['click']}
            >
              <span className={`cursor-pointer hover:text-indigo-600 flex items-center gap-1 ${isEditLocked ? 'cursor-not-allowed opacity-50' : ''}`}>
                生效方式: <span className="font-medium text-gray-700">{draft.effectType === 'realtime' ? '即时' : '重启'}</span>{' '}
                <SettingOutlined />
              </span>
            </Dropdown>
          )}
          <div className="flex gap-2">
            <Select
              size="small"
              disabled={isLocked}
              value={draft.exposure}
              options={[
                { value: 'INTERNAL', label: '仅内部' },
                { value: 'AUTHENTICATED', label: '登录可读' },
                { value: 'PUBLIC', label: '匿名可读' },
              ]}
              onChange={value => updateField('exposure', value)}
            />
            {isConfigAssetValueType(draft.valueType) ? (
              <Tag>普通资产</Tag>
            ) : (
              <Select
                size="small"
                disabled={isLocked}
                value={draft.sensitivity}
                options={[
                  { value: 'NORMAL', label: '普通' },
                  { value: 'SENSITIVE', label: '敏感可揭示' },
                  { value: 'SECRET', label: '密文只写' },
                ]}
                onChange={value => {
                  updateField('sensitivity', value);
                  updateField('isSensitive', value === 'NORMAL' ? 0 : 1);
                }}
              />
            )}
            <Tooltip title="只读配置禁止修改 Key 和 Value">
              <Popconfirm
                title={draft.isReadonly === 1 ? '解锁只读保护?' : '开启只读保护?'}
                onConfirm={() => updateField('isReadonly', draft.isReadonly === 1 ? 0 : 1)}
                okText="确定"
                cancelText="取消"
              >
                <Checkbox checked={draft.isReadonly === 1} disabled={isEditLocked} className={draft.isReadonly ? 'text-orange-600' : ''}>
                  只读保护
                </Checkbox>
              </Popconfirm>
            </Tooltip>
          </div>
        </div>
        <div className="flex items-center gap-1 opacity-70" title={`更新时间: ${draft.updateTime}`}>
          <ClockCircleOutlined /> {dayjs(draft.updateTime).format('MM-DD HH:mm')}
        </div>
      </div>
    </motion.div>
  );
};
