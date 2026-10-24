'use client';

import React, { useEffect, useMemo, useState } from 'react';
import { Alert, Button, Checkbox, Skeleton, Space, Table, Tag, message } from 'antd';
import type { ColumnsType } from 'antd/es/table';
import {
  assignRoleConfigScopes,
  getAllConfigGroups,
  getConfigs,
  getRoleConfigScopes,
} from '@/api/configController';
import type { ConfigGroup, ConfigItem, ConfigScopeGrant } from '@/types/config';

type ScopeRow = {
  key: string;
  groupCode: string;
  configKey?: string;
  label: string;
  rowType: 'group' | 'config';
};

type ScopeFlags = {
  canRead: boolean;
  canWrite: boolean;
  canDelete: boolean;
};

interface ConfigScopeAssignProps {
  role?: API.RoleVO;
  readonly?: boolean;
  onSaved?: () => void;
}

function scopeKey(groupCode: string, configKey?: string) {
  return `${groupCode}::${configKey ?? ''}`;
}

function grantToFlags(grant?: ConfigScopeGrant): ScopeFlags {
  return {
    canRead: grant?.canRead === 1,
    canWrite: grant?.canWrite === 1,
    canDelete: grant?.canDelete === 1,
  };
}

function hasAnyFlag(flags?: ScopeFlags) {
  return Boolean(flags?.canRead || flags?.canWrite || flags?.canDelete);
}

export const ConfigScopeAssign: React.FC<ConfigScopeAssignProps> = ({ role, readonly = false, onSaved }) => {
  const [groups, setGroups] = useState<ConfigGroup[]>([]);
  const [configsByGroup, setConfigsByGroup] = useState<Record<string, ConfigItem[]>>({});
  const [grants, setGrants] = useState<Record<string, ScopeFlags>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const roleId = role?.id;

  useEffect(() => {
    if (!roleId) return;
    let cancelled = false;
    Promise.all([getAllConfigGroups(), getRoleConfigScopes(roleId)])
      .then(async ([groupList, existingGrants]) => {
        const configPairs = await Promise.all(
          groupList.map(async group => {
            const page = await getConfigs({ groupId: group.id, pageNum: 1, pageSize: 1000 });
            return [group.groupCode, page.records ?? []] as const;
          }),
        );
        if (cancelled) return;
        const nextConfigsByGroup = Object.fromEntries(configPairs);
        const nextGrants: Record<string, ScopeFlags> = {};
        existingGrants.forEach(grant => {
          nextGrants[scopeKey(grant.groupCode, grant.configKey)] = grantToFlags(grant);
        });
        setGroups(groupList);
        setConfigsByGroup(nextConfigsByGroup);
        setGrants(nextGrants);
      })
      .catch((error: unknown) => {
        message.error((error as Error)?.message || '加载配置范围失败');
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [roleId]);

  const rows = useMemo<ScopeRow[]>(() => {
    return groups.flatMap(group => {
      const groupRow: ScopeRow = {
        key: scopeKey(group.groupCode),
        groupCode: group.groupCode,
        label: group.groupName,
        rowType: 'group',
      };
      const configRows = (configsByGroup[group.groupCode] ?? []).map(config => ({
        key: scopeKey(group.groupCode, config.configKey),
        groupCode: group.groupCode,
        configKey: config.configKey,
        label: config.configDesc || config.configKey,
        rowType: 'config' as const,
      }));
      return [groupRow, ...configRows];
    });
  }, [configsByGroup, groups]);

  const updateGrant = (row: ScopeRow, field: keyof ScopeFlags, checked: boolean) => {
    setGrants(prev => {
      const current = prev[row.key] ?? { canRead: false, canWrite: false, canDelete: false };
      const next = { ...current, [field]: checked };
      if ((field === 'canWrite' || field === 'canDelete') && checked) {
        next.canRead = true;
      }
      if (field === 'canRead' && !checked) {
        next.canWrite = false;
        next.canDelete = false;
      }
      return { ...prev, [row.key]: next };
    });
  };

  const columns: ColumnsType<ScopeRow> = [
    {
      title: '范围',
      dataIndex: 'label',
      render: (_, row) => (
        <Space>
          <Tag color={row.rowType === 'group' ? 'blue' : 'default'}>
            {row.rowType === 'group' ? '分组' : '配置'}
          </Tag>
          <span className={row.rowType === 'config' ? 'pl-5' : 'font-medium'}>{row.label}</span>
          <span className="font-mono text-xs text-gray-400">
            {row.configKey ? `${row.groupCode}.${row.configKey}` : row.groupCode}
          </span>
        </Space>
      ),
    },
    {
      title: '可读',
      width: 90,
      align: 'center',
      render: (_, row) => (
        <Checkbox
          disabled={readonly}
          checked={grants[row.key]?.canRead ?? false}
          onChange={event => updateGrant(row, 'canRead', event.target.checked)}
        />
      ),
    },
    {
      title: '可写',
      width: 90,
      align: 'center',
      render: (_, row) => (
        <Checkbox
          disabled={readonly}
          checked={grants[row.key]?.canWrite ?? false}
          onChange={event => updateGrant(row, 'canWrite', event.target.checked)}
        />
      ),
    },
    {
      title: '可删',
      width: 90,
      align: 'center',
      render: (_, row) => (
        <Checkbox
          disabled={readonly}
          checked={grants[row.key]?.canDelete ?? false}
          onChange={event => updateGrant(row, 'canDelete', event.target.checked)}
        />
      ),
    },
  ];

  const handleSave = async () => {
    if (!roleId) return;
    const payload = rows
      .map(row => ({ row, flags: grants[row.key] }))
      .filter(item => hasAnyFlag(item.flags))
      .map(({ row, flags }) => ({
        groupCode: row.groupCode,
        configKey: row.configKey,
        canRead: flags?.canRead ? 1 : 0,
        canWrite: flags?.canWrite ? 1 : 0,
        canDelete: flags?.canDelete ? 1 : 0,
    }));
    setSaving(true);
    try {
      await assignRoleConfigScopes(roleId, payload);
      message.success('配置范围已保存');
      onSaved?.();
    } catch (error: unknown) {
      message.error((error as Error)?.message || '保存配置范围失败');
    } finally {
      setSaving(false);
    }
  };

  if (!roleId) {
    return <Alert type="warning" showIcon message="请先选择角色" />;
  }

  return (
    <Space direction="vertical" className="w-full" size="middle">
      <Alert
        type="info"
        showIcon
        message="配置中心范围控制"
        description="角色拥有配置中心菜单权限后，仍只会看到这里授予的配置分组或配置项。可写/可删会自动隐含可读。"
      />
      {loading ? (
        <Skeleton active paragraph={{ rows: 8 }} />
      ) : (
        <Table
          rowKey="key"
          size="small"
          columns={columns}
          dataSource={rows}
          pagination={false}
          scroll={{ y: 420 }}
        />
      )}
      {!readonly ? (
        <div className="flex justify-end">
          <Button type="primary" loading={saving} onClick={handleSave}>
            保存配置范围
          </Button>
        </div>
      ) : null}
    </Space>
  );
};
