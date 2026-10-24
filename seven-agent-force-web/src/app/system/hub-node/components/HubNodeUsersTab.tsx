import { useEffect, useMemo, useState } from 'react';
import { Button, Dropdown, Input, Select, Space, Table, message } from 'antd';
import { EyeOutlined, MoreOutlined, SearchOutlined } from '@ant-design/icons';
import type { MenuProps } from 'antd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  canManageHubUser,
  listHubNodeUsers,
  setHubNodeUserStatus,
  type HubUserRecord,
  type HubUserStatusValue,
} from '@/api/hubNodeController';
import { formatTime, USER_STATUS_OPTIONS, userStatusTag } from '../constants';
import { requestActionReason } from '../reason';
import HubNodeSessionsDrawer from './HubNodeSessionsDrawer';
import styles from '../hubNode.module.css';

interface Props {
  nodeCode: string;
  canQueryUser: boolean;
  canChangeStatus: boolean;
  canListSessions: boolean;
  canRevokeSessions: boolean;
}

export default function HubNodeUsersTab({
  nodeCode,
  canQueryUser,
  canChangeStatus,
  canListSessions,
  canRevokeSessions,
}: Props) {
  const queryClient = useQueryClient();
  const [keywordInput, setKeywordInput] = useState('');
  const [query, setQuery] = useState<{ current: number; size: number; keyword?: string; status?: HubUserStatusValue }>({ current: 1, size: 10 });
  const [selectedUser, setSelectedUser] = useState<HubUserRecord | null>(null);
  const usersKey = useMemo(() => ['hub-node', nodeCode, 'users'], [nodeCode]);

  const usersQuery = useQuery({
    queryKey: [...usersKey, query],
    queryFn: () => listHubNodeUsers(nodeCode, query),
    retry: 0,
  });

  useEffect(() => () => {
    queryClient.removeQueries({ queryKey: usersKey });
  }, [queryClient, usersKey]);

  const statusMutation = useMutation({
    mutationFn: ({ user, status, reason }: { user: HubUserRecord; status: HubUserStatusValue; reason: string }) =>
      setHubNodeUserStatus(nodeCode, user.userId, { status, reason }),
    onSuccess: async () => {
      message.success('用户状态已更新');
      await queryClient.invalidateQueries({ queryKey: usersKey });
    },
    onError: (error) => message.error((error as Error)?.message || '用户状态更新失败'),
  });

  const changeStatus = async (user: HubUserRecord, status: HubUserStatusValue) => {
    if (!canManageHubUser(user)) return;
    const label = USER_STATUS_OPTIONS.find((item) => item.value === status)?.label || String(status);
    const reason = await requestActionReason(
      `将 ${user.username} 状态调整为“${label}”`,
      '请输入本次状态调整的审计原因',
    );
    if (reason) await statusMutation.mutateAsync({ user, status, reason });
  };

  return (
    <div data-testid="hub-users-tab">
      <div className={styles.tabToolbar}>
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="搜索用户名、昵称或联系方式"
          value={keywordInput}
          onChange={(event) => setKeywordInput(event.target.value)}
          onPressEnter={() => setQuery((current) => ({ ...current, current: 1, keyword: keywordInput.trim() || undefined }))}
        />
        <Select
          allowClear
          placeholder="用户状态"
          options={USER_STATUS_OPTIONS}
          onChange={(status) => setQuery((current) => ({ ...current, current: 1, status }))}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={() => setQuery((current) => ({ ...current, current: 1, keyword: keywordInput.trim() || undefined }))}
        >
          查询
        </Button>
      </div>
      <Table<HubUserRecord>
        rowKey="userId"
        size="small"
        loading={usersQuery.isLoading}
        dataSource={usersQuery.data?.records || []}
        scroll={{ x: 760 }}
        columns={[
          { title: '用户', dataIndex: 'username', width: 160, render: (_, row) => row.nickname || row.username },
          { title: '用户名', dataIndex: 'username', width: 140 },
          { title: '联系方式', key: 'contact', width: 180, render: (_, row) => row.emailMasked || row.phoneMasked || '-' },
          { title: '状态', dataIndex: 'status', width: 90, render: (value) => userStatusTag(value) },
          { title: '更新时间', dataIndex: 'updatedAt', width: 170, render: formatTime },
          {
            title: '操作', key: 'actions', fixed: 'right', width: 128,
            render: (_, row) => {
              const manageable = canManageHubUser(row);
              const statusItems: MenuProps['items'] = canChangeStatus && manageable
                ? USER_STATUS_OPTIONS.filter((item) => item.value !== row.status).map((item) => ({ key: String(item.value), label: `设为${item.label}` }))
                : [];
              return (
                <Space size={4}>
                  {manageable && (canQueryUser || canListSessions) ? (
                    <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => setSelectedUser(row)}>查看</Button>
                  ) : null}
                  {statusItems.length ? (
                    <Dropdown menu={{ items: statusItems, onClick: ({ key }) => void changeStatus(row, Number(key) as HubUserStatusValue) }}>
                      <Button aria-label="用户状态操作" size="small" icon={<MoreOutlined />} loading={statusMutation.isPending} />
                    </Dropdown>
                  ) : null}
                </Space>
              );
            },
          },
        ]}
        pagination={{
          current: query.current,
          pageSize: query.size,
          total: usersQuery.data?.total || 0,
          showSizeChanger: true,
          onChange: (current, size) => setQuery((value) => ({ ...value, current, size })),
        }}
      />
      <HubNodeSessionsDrawer
        open={Boolean(selectedUser)}
        nodeCode={nodeCode}
        user={selectedUser}
        canQueryUser={canQueryUser}
        canListSessions={canListSessions}
        canRevokeSessions={canRevokeSessions}
        onClose={() => setSelectedUser(null)}
      />
    </div>
  );
}
