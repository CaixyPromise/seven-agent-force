import { useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Descriptions,
  Drawer,
  Form,
  Input,
  Modal,
  Pagination,
  Radio,
  Skeleton,
  Space,
  Table,
  Tabs,
  Tag,
  Tree,
  Typography,
  message,
} from 'antd';
import type { DataNode } from 'antd/es/tree';
import type { ColumnsType } from 'antd/es/table';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { getMenuTree, listPermissions } from '@/api/sysMenuController';
import { getDeptTree } from '@/api/sysDeptController';
import { getAllConfigGroups, getConfigs } from '@/api/configController';
import {
  commitRoleGrantBundle,
  getRoleGrantSnapshot,
  previewRoleGrantBundle,
} from '@/api/roleGrantController';
import type {
  RoleConfigScopeGrant,
  RoleGrantBundle,
  RoleGrantPreview,
} from '@/api/roleGrantController';
import type { ConfigGroup, ConfigItem } from '@/types/config';
import { AUTH_MENUS_QUERY_KEY, CURRENT_USER_QUERY_KEY } from '@/hooks/useAuth';

type KeyValue = string | number;

type ConfigRow = {
  key: string;
  groupCode: string;
  configKey?: string;
  label: string;
  kind: 'group' | 'config';
};

type ConfigFlags = { canRead: boolean; canWrite: boolean; canDelete: boolean };

interface RoleGrantDrawerProps {
  open: boolean;
  role?: API.RoleVO;
  readonly?: boolean;
  canQueryConfigScope?: boolean;
  canAssignConfigScope?: boolean;
  onClose: () => void;
  onCommitted?: () => void;
}

const dataScopeLabels: Record<number, string> = {
  1: '全部数据',
  2: '自定部门',
  3: '本部门',
  4: '本部门及以下',
  5: '仅本人',
};

function toMenuTree(items: API.MenuVO[] = []): DataNode[] {
  return items
    .filter((item) => item.id !== undefined)
    .map((item) => ({
      key: String(item.id),
      title: item.name || String(item.id),
      children: toMenuTree(item.children),
    }));
}

function toDeptTree(items: API.SysDept[] = []): DataNode[] {
  return items
    .filter((item) => item.id !== undefined)
    .map((item) => ({
      key: String(item.id),
      title: item.name ? `${item.name} (${item.code || item.id})` : String(item.id),
      children: toDeptTree(item.children),
    }));
}

function checkedKeys(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String);
  return ((value as { checked?: React.Key[] } | undefined)?.checked ?? []).map(String);
}

function configKey(groupCode: string, itemKey?: string) {
  return `${groupCode}::${itemKey || ''}`;
}

function countChanges(preview: RoleGrantPreview) {
  const changes = preview.changes;
  return (
    changes.addedMenuIds.length +
    changes.removedMenuIds.length +
    changes.addedPermissionIds.length +
    changes.removedPermissionIds.length +
    changes.addedDeptIds.length +
    changes.removedDeptIds.length +
    changes.addedConfigScopes.length +
    changes.removedConfigScopes.length +
    (changes.dataScopeFrom === changes.dataScopeTo ? 0 : 1)
  );
}

function newIdempotencyKey(roleId: KeyValue) {
  const suffix = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `role-grant-${roleId}-${suffix}`;
}

export function RoleGrantDrawer({
  open,
  role,
  readonly = false,
  canQueryConfigScope = false,
  canAssignConfigScope = false,
  onClose,
  onCommitted,
}: RoleGrantDrawerProps) {
  const queryClient = useQueryClient();
  const [menuIds, setMenuIds] = useState<string[]>([]);
  const [permissionIds, setPermissionIds] = useState<React.Key[]>([]);
  const [dataScope, setDataScope] = useState(5);
  const [deptIds, setDeptIds] = useState<string[]>([]);
  const [configFlags, setConfigFlags] = useState<Record<string, ConfigFlags>>({});
  const [reason, setReason] = useState('');
  const [permissionPage, setPermissionPage] = useState(1);
  const [permissionKeyword, setPermissionKeyword] = useState('');
  const [preview, setPreview] = useState<RoleGrantPreview>();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [draftReady, setDraftReady] = useState(false);
  const roleId = role?.id;

  const snapshotQuery = useQuery({
    queryKey: ['role-grant-snapshot', roleId],
    queryFn: async () => (await getRoleGrantSnapshot(roleId!)).data,
    enabled: open && roleId !== undefined,
  });
  const menuQuery = useQuery({
    queryKey: ['role-grant-menu-tree'],
    queryFn: async () => (await getMenuTree()).data ?? [],
    enabled: open,
  });
  const deptQuery = useQuery({
    queryKey: ['role-grant-dept-tree'],
    queryFn: async () => (await getDeptTree()).data ?? [],
    enabled: open,
  });
  const permissionQuery = useQuery({
    queryKey: ['role-grant-permissions', permissionPage, permissionKeyword],
    queryFn: async () => {
      const response = await listPermissions({
        current: permissionPage,
        size: 20,
        code: permissionKeyword || undefined,
        status: 0,
      });
      const data = response.data as unknown as { records?: API.PermissionVO[]; total?: number } | API.PermissionVO[] | undefined;
      return Array.isArray(data)
        ? { records: data, total: data.length }
        : { records: data?.records ?? [], total: Number(data?.total ?? 0) };
    },
    enabled: open,
  });
  const configQuery = useQuery({
    queryKey: ['role-grant-config-options'],
    queryFn: async () => {
      const groups = await getAllConfigGroups();
      const pairs = await Promise.all(
        groups.map(async (group) => [group.groupCode, (await getConfigs({ groupId: group.id, pageNum: 1, pageSize: 1000 })).records] as const),
      );
      return { groups, configsByGroup: Object.fromEntries(pairs) as Record<string, ConfigItem[]> };
    },
    enabled: open && canQueryConfigScope,
  });

  useEffect(() => {
    const snapshot = snapshotQuery.data;
    if (!open || !snapshot) return;
    const timer = window.setTimeout(() => {
      setMenuIds(snapshot.menuIds.map(String));
      setPermissionIds(snapshot.permissionIds.map(String));
      setDataScope(snapshot.dataScope);
      setDeptIds(snapshot.deptIds.map(String));
      const nextFlags: Record<string, ConfigFlags> = {};
      snapshot.configScopes.forEach((grant) => {
        nextFlags[configKey(grant.groupCode, grant.configKey)] = {
          canRead: grant.canRead === 1,
          canWrite: grant.canWrite === 1,
          canDelete: grant.canDelete === 1,
        };
      });
      setConfigFlags(nextFlags);
      setReason('');
      setDraftReady(true);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [open, snapshotQuery.data]);

  const configRows = useMemo<ConfigRow[]>(() => {
    const data = configQuery.data;
    const catalogRows = data ? data.groups.flatMap((group: ConfigGroup) => [
      { key: configKey(group.groupCode), groupCode: group.groupCode, label: group.groupName, kind: 'group' as const },
      ...(data.configsByGroup[group.groupCode] ?? []).map((item) => ({
        key: configKey(group.groupCode, item.configKey), groupCode: group.groupCode,
        configKey: item.configKey, label: item.configDesc || item.configKey, kind: 'config' as const,
      })),
    ]) : [];
    const known = new Set(catalogRows.map((row) => row.key));
    const snapshotRows = (snapshotQuery.data?.configScopes ?? [])
      .filter((grant) => !known.has(configKey(grant.groupCode, grant.configKey)))
      .map((grant) => ({
        key: configKey(grant.groupCode, grant.configKey),
        groupCode: grant.groupCode,
        configKey: grant.configKey,
        label: grant.configKey || grant.groupCode,
        kind: grant.configKey ? 'config' as const : 'group' as const,
      }));
    return [...catalogRows, ...snapshotRows];
  }, [configQuery.data, snapshotQuery.data?.configScopes]);

  const configColumns: ColumnsType<ConfigRow> = [
    {
      title: '配置范围',
      render: (_, row) => (
        <Space>
          <Tag color={row.kind === 'group' ? 'blue' : 'default'}>{row.kind === 'group' ? '分组' : '配置'}</Tag>
          <span className={row.kind === 'config' ? 'pl-4' : 'font-medium'}>{row.label}</span>
          <Typography.Text code>{row.configKey ? `${row.groupCode}.${row.configKey}` : row.groupCode}</Typography.Text>
        </Space>
      ),
    },
    ...(['canRead', 'canWrite', 'canDelete'] as const).map((field) => ({
      title: { canRead: '可读', canWrite: '可写', canDelete: '可删' }[field],
      width: 80,
      align: 'center' as const,
      render: (_: unknown, row: ConfigRow) => (
        <Checkbox
          disabled={rootReadonly || !canAssignConfigScope}
          checked={configFlags[row.key]?.[field] ?? false}
          onChange={(event) => {
            setConfigFlags((current) => {
              const flags = current[row.key] ?? { canRead: false, canWrite: false, canDelete: false };
              const next = { ...flags, [field]: event.target.checked };
              if ((field === 'canWrite' || field === 'canDelete') && event.target.checked) next.canRead = true;
              if (field === 'canRead' && !event.target.checked) {
                next.canWrite = false;
                next.canDelete = false;
              }
              return { ...current, [row.key]: next };
            });
          }}
        />
      ),
    })),
  ];

  const buildBundle = (idempotencyKey = ''): RoleGrantBundle => ({
    expectedRevision: String(snapshotQuery.data?.revision ?? 0),
    menuIds,
    permissionIds: permissionIds.map(String),
    dataScope,
    deptIds: dataScope === 2 ? deptIds : [],
    configScopes: configRows.flatMap<RoleConfigScopeGrant>((row) => {
      const flags = configFlags[row.key];
      if (!flags?.canRead && !flags?.canWrite && !flags?.canDelete) return [];
      return [{
        groupCode: row.groupCode,
        configKey: row.configKey,
        canRead: flags.canRead ? 1 : 0,
        canWrite: flags.canWrite ? 1 : 0,
        canDelete: flags.canDelete ? 1 : 0,
      }];
    }),
    reason: reason.trim(),
    idempotencyKey,
  });

  const handlePreview = async () => {
    if (!roleId) return;
    if (!reason.trim()) {
      message.warning('请填写本次授权变更原因');
      return;
    }
    try {
      const response = await previewRoleGrantBundle(roleId, buildBundle());
      if (!response.data) throw new Error('预览结果为空');
      setPreview(response.data);
      setPreviewOpen(true);
    } catch (error) {
      message.error(error instanceof Error ? error.message : '授权预览失败');
    }
  };

  const handleCommit = async () => {
    if (!roleId || !preview) return;
    setSubmitting(true);
    try {
      const response = await commitRoleGrantBundle(roleId, buildBundle(newIdempotencyKey(roleId)));
      message.success(response.data?.changed ? '角色授权已原子提交' : '角色授权没有变化');
      setPreviewOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['role-grant-snapshot', roleId] }),
        queryClient.invalidateQueries({ queryKey: AUTH_MENUS_QUERY_KEY }),
        queryClient.invalidateQueries({ queryKey: CURRENT_USER_QUERY_KEY }),
      ]);
      onCommitted?.();
      onClose();
    } catch (error) {
      const conflict = error as Error & { code?: number; payload?: { data?: { reasonCode?: string } } };
      if (conflict.code === 40900 || conflict.payload?.data?.reasonCode === 'ROLE_GRANT_REVISION_CONFLICT') {
        message.error('角色授权已被其他管理员更新。当前草稿仍保留，请重新加载后再预览。');
      } else {
        message.error(error instanceof Error ? error.message : '角色授权提交失败');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const loading = !draftReady || snapshotQuery.isLoading || menuQuery.isLoading || deptQuery.isLoading || (canQueryConfigScope && configQuery.isLoading);
  const rootReadonly = readonly || role?.authorizationRoot === true;

  return (
    <>
      <Drawer
        title={`${rootReadonly ? '查看' : '统一授权'} - ${role?.name || ''}`}
        open={open}
        onClose={onClose}
        size="large"
        destroyOnHidden
        extra={!rootReadonly ? <Button type="primary" onClick={handlePreview}>预览并提交</Button> : null}
      >
        {role?.authorizationRoot ? (
          <Alert className="mb-4" type="warning" showIcon title="安全根授权只读" description="安全根的菜单、权限、数据范围和配置范围由系统迁移维护，普通授权接口不能削弱。" />
        ) : null}
        {snapshotQuery.isError ? <Alert type="error" showIcon title="授权快照加载失败" /> : null}
        {loading ? <Skeleton active paragraph={{ rows: 12 }} /> : (
          <Tabs
            items={[
              {
                key: 'menus', label: `菜单 (${menuIds.length})`,
                children: <Tree checkable disabled={rootReadonly} defaultExpandAll treeData={toMenuTree(menuQuery.data)} checkedKeys={menuIds} onCheck={(keys) => setMenuIds(checkedKeys(keys))} />,
              },
              {
                key: 'permissions', label: `直接权限 (${permissionIds.length})`,
                children: (
                  <Space orientation="vertical" className="w-full">
                    <Input.Search allowClear placeholder="搜索权限标识" onSearch={(value) => { setPermissionKeyword(value.trim()); setPermissionPage(1); }} />
                    <Table<API.PermissionVO>
                      rowKey={(item) => String(item.id)} size="small" pagination={false}
                      dataSource={permissionQuery.data?.records ?? []}
                      rowSelection={{ selectedRowKeys: permissionIds, preserveSelectedRowKeys: true, onChange: setPermissionIds, getCheckboxProps: () => ({ disabled: rootReadonly }) }}
                      columns={[
                        { title: '权限标识', dataIndex: 'code', render: (value) => <Typography.Text code>{value}</Typography.Text> },
                        { title: '名称', dataIndex: 'name' },
                        { title: '资源', render: (_, item) => `${item.method || '-'} ${item.path || '-'}` },
                      ]}
                    />
                    <Pagination current={permissionPage} pageSize={20} total={permissionQuery.data?.total ?? 0} showSizeChanger={false} onChange={setPermissionPage} />
                  </Space>
                ),
              },
              {
                key: 'data-scope', label: `数据范围 (${dataScopeLabels[dataScope] || dataScope})`,
                children: (
                  <Space orientation="vertical" className="w-full" size="middle">
                    <Radio.Group
                      value={dataScope}
                      disabled={rootReadonly || role?.systemManaged === true}
                      onChange={(event) => setDataScope(event.target.value)}
                      options={Object.entries(dataScopeLabels).map(([value, label]) => ({ value: Number(value), label }))}
                    />
                    {role?.systemManaged ? <Alert type="info" showIcon title="SYSTEM 角色的数据范围类型由系统管理" /> : null}
                    {dataScope === 2 ? <Tree checkable disabled={rootReadonly} defaultExpandAll treeData={toDeptTree(deptQuery.data)} checkedKeys={deptIds} onCheck={(keys) => setDeptIds(checkedKeys(keys))} /> : <Alert type="info" showIcon title="当前范围不使用自定部门清单" />}
                  </Space>
                ),
              },
              {
                key: 'config-scopes', label: `配置范围 (${Object.values(configFlags).filter((item) => item.canRead || item.canWrite || item.canDelete).length})`,
                children: (
                  <Space orientation="vertical" className="w-full">
                    {!canQueryConfigScope ? <Alert type="info" showIcon title="当前账号只能查看快照中已有的配置范围" /> : null}
                    {canQueryConfigScope && !canAssignConfigScope ? <Alert type="info" showIcon title="当前账号没有配置范围委派权限，本页只读" /> : null}
                    <Table rowKey="key" size="small" pagination={false} scroll={{ y: 460 }} columns={configColumns} dataSource={configRows} />
                  </Space>
                ),
              },
            ]}
          />
        )}
        {!rootReadonly ? (
          <Form layout="vertical" className="mt-5">
            <Form.Item label="授权变更原因" required>
              <Input.TextArea value={reason} onChange={(event) => setReason(event.target.value)} maxLength={500} showCount rows={3} placeholder="说明本次授权变更的业务背景；该原因会进入操作审计。" />
            </Form.Item>
            <Typography.Text type="secondary">最终提交会对菜单、直接权限、数据范围、部门清单和配置范围执行一次事务；任一分区失败都不会部分生效。</Typography.Text>
          </Form>
        ) : null}
      </Drawer>

      <Modal title="角色授权变更预览" open={previewOpen} onCancel={() => setPreviewOpen(false)} onOk={handleCommit} okText="完成二次验证并提交" confirmLoading={submitting}>
        {preview ? (
          <Space orientation="vertical" className="w-full">
            <Alert type={preview.changed ? 'warning' : 'info'} showIcon title={preview.changed ? `将产生 ${countChanges(preview)} 组变更` : '授权内容没有变化'} description={`该角色通过直接授予和岗位继承共影响 ${preview.impactedUserCount} 名用户。提交时会再次校验 revision ${preview.revision}。`} />
            <Descriptions column={2} bordered size="small">
              <Descriptions.Item label="菜单">+{preview.changes.addedMenuIds.length} / -{preview.changes.removedMenuIds.length}</Descriptions.Item>
              <Descriptions.Item label="直接权限">+{preview.changes.addedPermissionIds.length} / -{preview.changes.removedPermissionIds.length}</Descriptions.Item>
              <Descriptions.Item label="部门">+{preview.changes.addedDeptIds.length} / -{preview.changes.removedDeptIds.length}</Descriptions.Item>
              <Descriptions.Item label="配置范围">+{preview.changes.addedConfigScopes.length} / -{preview.changes.removedConfigScopes.length}</Descriptions.Item>
              <Descriptions.Item label="数据范围" span={2}>{dataScopeLabels[preview.changes.dataScopeFrom]} → {dataScopeLabels[preview.changes.dataScopeTo]}</Descriptions.Item>
            </Descriptions>
          </Space>
        ) : null}
      </Modal>
    </>
  );
}
