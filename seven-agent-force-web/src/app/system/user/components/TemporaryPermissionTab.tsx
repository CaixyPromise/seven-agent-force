'use client';

import { useState } from 'react';
import {
  Alert,
  Button,
  DatePicker,
  Empty,
  Form,
  Input,
  Modal,
  Space,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { PlusOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import dayjs, { type Dayjs } from 'dayjs';
import { PermissionSelector } from '@/components/Selectors/PermissionSelector';
import type { PermissionOption } from '@/components/Selectors/PermissionSelector';
import {
  extendTemporaryPermission,
  getUserTemporaryPermissions,
  grantTemporaryPermission,
  revokeTemporaryPermission,
  type TemporaryPermission,
} from '@/api/temporaryPermissionController';

type ActionMode = 'grant' | 'extend' | 'revoke';

interface ActionFormValues {
  permissionId?: string | number;
  expireAt?: Dayjs;
  source?: string;
  reason: string;
}

interface TemporaryPermissionTabProps {
  userId: API.Int64;
  active: boolean;
  canGrant: boolean;
  canExtend: boolean;
  canRevoke: boolean;
}

const statusLabels: Record<TemporaryPermission['status'], { text: string; color: string }> = {
  ACTIVE: { text: '当前有效', color: 'success' },
  EXPIRED: { text: '已过期', color: 'warning' },
  PERMANENT: { text: '永久关系', color: 'blue' },
};

export function TemporaryPermissionTab({
  userId,
  active,
  canGrant,
  canExtend,
  canRevoke,
}: TemporaryPermissionTabProps) {
  const queryClient = useQueryClient();
  const [form] = Form.useForm<ActionFormValues>();
  const [mode, setMode] = useState<ActionMode>();
  const [selected, setSelected] = useState<TemporaryPermission>();
  const [selectedPermission, setSelectedPermission] = useState<PermissionOption>();

  const permissionsQuery = useQuery({
    queryKey: ['temporary-permissions', userId],
    queryFn: async () => (await getUserTemporaryPermissions(userId)).data ?? [],
    enabled: active,
  });

  const refreshAuthorizationViews = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['temporary-permissions', userId] }),
      queryClient.invalidateQueries({ queryKey: ['userEffectiveAccess', userId] }),
      queryClient.invalidateQueries({ queryKey: ['userPermissionExplanation', userId] }),
    ]);
  };

  const mutation = useMutation({
    mutationFn: async (values: ActionFormValues) => {
      const reason = values.reason.trim();
      if (mode === 'grant') {
        if (!selectedPermission) throw new Error('请选择权限');
        if (!values.expireAt) throw new Error('请选择失效时间');
        return grantTemporaryPermission({
          userId,
          permissionCode: selectedPermission.code,
          expireAt: values.expireAt.toISOString(),
          source: values.source?.trim() || 'ADMIN_CONSOLE',
          reason,
        });
      }
      if (!selected) throw new Error('未选择临时权限');
      if (mode === 'extend') {
        if (!values.expireAt) throw new Error('请选择新的失效时间');
        return extendTemporaryPermission({
          userId,
          permissionCode: selected.permissionCode,
          expireAt: values.expireAt.toISOString(),
          reason,
        });
      }
      return revokeTemporaryPermission({
        userId,
        permissionCode: selected.permissionCode,
        reason,
      });
    },
    onSuccess: () => {
      message.success(mode === 'grant' ? '临时权限已授予' : mode === 'extend' ? '临时权限已延期' : '临时权限已撤销');
      setMode(undefined);
      setSelected(undefined);
      setSelectedPermission(undefined);
      form.resetFields();
      void refreshAuthorizationViews();
    },
    onError: (error: unknown) => {
      message.error(error instanceof Error ? error.message : '临时权限操作失败');
    },
  });

  const openAction = (nextMode: ActionMode, permission?: TemporaryPermission) => {
    setMode(nextMode);
    setSelected(permission);
    setSelectedPermission(undefined);
    form.resetFields();
    if (nextMode === 'extend' && permission?.expireAt) {
      form.setFieldValue('expireAt', dayjs(permission.expireAt).add(1, 'day'));
    }
  };

  const columns: ColumnsType<TemporaryPermission> = [
    {
      title: '权限',
      dataIndex: 'permissionCode',
      render: (code: string, record) => (
        <Space orientation="vertical" size={0}>
          <Typography.Text code>{code}</Typography.Text>
          <Typography.Text type="secondary">{record.permissionName || '未命名权限'}</Typography.Text>
        </Space>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 110,
      render: (status: TemporaryPermission['status']) => <Tag color={statusLabels[status].color}>{statusLabels[status].text}</Tag>,
    },
    {
      title: '失效时间',
      dataIndex: 'expireAt',
      width: 180,
      render: (value?: string) => value ? dayjs(value).format('YYYY-MM-DD HH:mm:ss') : '永久',
    },
    { title: '来源', dataIndex: 'source', width: 150, render: (value?: string) => value || '-' },
    { title: '授权原因', dataIndex: 'reason', ellipsis: true, render: (value?: string) => value || '-' },
    {
      title: '操作',
      width: 140,
      render: (_, record) => (
        <Space size={4}>
          {canExtend && record.status !== 'PERMANENT' ? <Button type="link" size="small" onClick={() => openAction('extend', record)}>延期</Button> : null}
          {canRevoke ? <Button type="link" danger size="small" onClick={() => openAction('revoke', record)}>撤销</Button> : null}
        </Space>
      ),
    },
  ];

  if (permissionsQuery.isError) {
    return <Alert type="error" showIcon title="临时权限加载失败" description="请确认查询权限或稍后重试。" />;
  }

  return (
    <Space orientation="vertical" size={16} className="w-full">
      <Alert
        type="info"
        showIcon
        title="临时权限是用户直授关系"
        description="授予、延期和撤销均需二次验证并记录原因；最终访问仍受用户状态、权限状态和 Feature 状态约束。"
      />
      <div>
        {canGrant ? <Button type="primary" icon={<PlusOutlined />} onClick={() => openAction('grant')}>授予临时权限</Button> : null}
      </div>
      <Table<TemporaryPermission>
        rowKey={(record) => `${record.permissionCode}-${record.grantedAt || ''}`}
        size="small"
        loading={permissionsQuery.isLoading}
        dataSource={permissionsQuery.data ?? []}
        columns={columns}
        pagination={{ pageSize: 10, showSizeChanger: true }}
        locale={{ emptyText: <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无临时权限关系" /> }}
      />

      <Modal
        title={mode === 'grant' ? '授予临时权限' : mode === 'extend' ? `延期：${selected?.permissionCode || ''}` : `撤销：${selected?.permissionCode || ''}`}
        open={Boolean(mode)}
        confirmLoading={mutation.isPending}
        okText={mode === 'revoke' ? '完成二次验证并撤销' : '完成二次验证并提交'}
        okButtonProps={{ danger: mode === 'revoke' }}
        onCancel={() => {
          setMode(undefined);
          setSelected(undefined);
          setSelectedPermission(undefined);
          form.resetFields();
        }}
        onOk={() => form.validateFields().then((values) => mutation.mutate(values))}
        forceRender
      >
        <Form form={form} layout="vertical">
          {mode === 'grant' ? (
            <>
              <Form.Item name="permissionId" label="权限" rules={[{ required: true, message: '请选择权限' }]}>
                <PermissionSelector
                  value={form.getFieldValue('permissionId')}
                  onChange={(value, permission) => {
                    form.setFieldValue('permissionId', value);
                    setSelectedPermission(permission);
                  }}
                  style={{ width: '100%' }}
                />
              </Form.Item>
              <Form.Item name="source" label="来源" initialValue="ADMIN_CONSOLE">
                <Input maxLength={100} placeholder="如：工单、值班、应急处理" />
              </Form.Item>
            </>
          ) : null}
          {mode !== 'revoke' ? (
            <Form.Item name="expireAt" label={mode === 'extend' ? '新的失效时间' : '失效时间'} rules={[{ required: true, message: '请选择失效时间' }]}>
              <DatePicker
                showTime
                className="w-full"
                disabledDate={(current) => current.endOf('day').isBefore(dayjs())}
              />
            </Form.Item>
          ) : null}
          <Form.Item
            name="reason"
            label={mode === 'revoke' ? '撤销原因' : mode === 'extend' ? '延期原因' : '授权原因'}
            rules={[{ required: true, whitespace: true, message: '请填写操作原因' }, { max: 500, message: '原因最多500个字符' }]}
          >
            <Input.TextArea rows={3} maxLength={500} showCount />
          </Form.Item>
        </Form>
      </Modal>
    </Space>
  );
}
