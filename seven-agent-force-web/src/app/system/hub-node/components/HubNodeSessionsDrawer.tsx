import { useMemo, useState } from 'react';
import { Button, Descriptions, Drawer, Empty, Space, Table, Typography, message } from 'antd';
import { CheckSquareOutlined, BorderOutlined, StopOutlined } from '@ant-design/icons';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  getHubNodeUser,
  listHubNodeSessions,
  revokeHubNodeSessions,
  type HubSessionRecord,
  type HubUserRecord,
} from '@/api/hubNodeController';
import { formatTime, sessionStatusTag, userStatusTag } from '../constants';
import { requestActionReason } from '../reason';

interface Props {
  open: boolean;
  nodeCode: string;
  user: HubUserRecord | null;
  canQueryUser: boolean;
  canListSessions: boolean;
  canRevokeSessions: boolean;
  onClose: () => void;
}

export default function HubNodeSessionsDrawer({
  open,
  nodeCode,
  user,
  canQueryUser,
  canListSessions,
  canRevokeSessions,
  onClose,
}: Props) {
  const queryClient = useQueryClient();
  const [page, setPage] = useState({ current: 1, size: 10 });
  const [selectedRefs, setSelectedRefs] = useState<string[]>([]);
  const [revoking, setRevoking] = useState(false);
  const userId = user?.userId || '';
  const queryPrefix = useMemo(() => ['hub-node', nodeCode, 'user', userId], [nodeCode, userId]);

  const detailQuery = useQuery({
    queryKey: [...queryPrefix, 'detail'],
    queryFn: () => getHubNodeUser(nodeCode, userId),
    enabled: open && Boolean(userId) && canQueryUser,
    retry: 0,
  });
  const sessionsQuery = useQuery({
    queryKey: [...queryPrefix, 'sessions', page.current, page.size],
    queryFn: () => listHubNodeSessions(nodeCode, userId, page),
    enabled: open && Boolean(userId) && canListSessions,
    retry: 0,
  });

  const runRevoke = async (all: boolean) => {
    if (!all && selectedRefs.length === 0) return;
    const reason = await requestActionReason(
      all ? '撤销该用户全部会话' : `撤销选中的 ${selectedRefs.length} 个会话`,
      '请输入审计原因，例如：账号风险处置',
    );
    if (!reason) return;
    setRevoking(true);
    try {
      await revokeHubNodeSessions(nodeCode, userId, {
        all,
        sessionRefs: all ? undefined : selectedRefs,
        reason,
      });
      setSelectedRefs([]);
      message.success('会话撤销指令已提交');
      await queryClient.invalidateQueries({ queryKey: [...queryPrefix, 'sessions'] });
    } catch (error) {
      message.error((error as Error)?.message || '会话撤销失败');
    } finally {
      setRevoking(false);
    }
  };

  const rows = (sessionsQuery.data?.records || []).map((session, index) => ({
    ...session,
    localKey: `${page.current}-${index}`,
  }));
  const displayedUser = detailQuery.data || user;

  const toggleSession = (sessionRef: string) => {
    setSelectedRefs((current) =>
      current.includes(sessionRef)
        ? current.filter((item) => item !== sessionRef)
        : [...current, sessionRef],
    );
  };

  const closeAndClear = () => {
    setSelectedRefs([]);
    setPage({ current: 1, size: 10 });
    queryClient.removeQueries({ queryKey: queryPrefix });
    onClose();
  };

  return (
    <Drawer
      open={open}
      title="用户与会话"
      size="min(100vw, 760px)"
      destroyOnHidden
      onClose={closeAndClear}
      afterOpenChange={(visible) => {
        if (!visible) queryClient.removeQueries({ queryKey: queryPrefix });
      }}
      extra={
        canRevokeSessions && canListSessions ? (
          <Space>
            <Button
              danger
              icon={<StopOutlined />}
              disabled={selectedRefs.length === 0}
              loading={revoking}
              onClick={() => void runRevoke(false)}
            >
              撤销选中
            </Button>
            <Button
              danger
              type="primary"
              loading={revoking}
              onClick={() => void runRevoke(true)}
            >
              撤销全部
            </Button>
          </Space>
        ) : null
      }
    >
      {displayedUser ? (
        <Descriptions column={{ xs: 1, sm: 2 }} size="small" style={{ marginBottom: 24 }}>
          <Descriptions.Item label="用户">{displayedUser.nickname || displayedUser.username}</Descriptions.Item>
          <Descriptions.Item label="状态">{userStatusTag(displayedUser.status)}</Descriptions.Item>
          <Descriptions.Item label="用户名">{displayedUser.username}</Descriptions.Item>
          <Descriptions.Item label="用户 ID">
            <Typography.Text copyable>{displayedUser.userId}</Typography.Text>
          </Descriptions.Item>
          <Descriptions.Item label="邮箱">{displayedUser.emailMasked || '-'}</Descriptions.Item>
          <Descriptions.Item label="手机">{displayedUser.phoneMasked || '-'}</Descriptions.Item>
        </Descriptions>
      ) : null}

      {canListSessions ? (
        <Table<HubSessionRecord & { localKey: string }>
          data-testid="hub-session-table"
          rowKey="localKey"
          size="small"
          loading={sessionsQuery.isLoading}
          dataSource={rows}
          scroll={{ x: 720 }}
          columns={[
            ...(canRevokeSessions
              ? [{
                  title: '选择',
                  key: 'selection',
                  width: 60,
                  render: (_: unknown, row: HubSessionRecord) => {
                    const selected = selectedRefs.includes(row.sessionRef);
                    return (
                      <Button
                        type="text"
                        icon={selected ? <CheckSquareOutlined /> : <BorderOutlined />}
                        aria-label={`${selected ? '取消选择' : '选择'}会话 ${row.clientId}`}
                        aria-pressed={selected}
                        disabled={row.status !== 'ACTIVE'}
                        onClick={() => toggleSession(row.sessionRef)}
                        style={{ width: 32, height: 32 }}
                      />
                    );
                  },
                }]
              : []),
            { title: '客户端', dataIndex: 'clientId', width: 180, ellipsis: true },
            { title: '登录方式', dataIndex: 'loginMethod', width: 110, render: (value) => value || '-' },
            { title: '状态', dataIndex: 'status', width: 90, render: (value) => sessionStatusTag(value) },
            { title: '登录时间', dataIndex: 'loginAt', width: 170, render: formatTime },
            { title: '最近访问', dataIndex: 'lastAccessAt', width: 170, render: formatTime },
          ]}
          pagination={{
            current: page.current,
            pageSize: page.size,
            total: sessionsQuery.data?.total || 0,
            showSizeChanger: true,
            onChange: (current, size) => {
              setSelectedRefs([]);
              setPage({ current, size });
            },
          }}
        />
      ) : (
        <Empty description="无会话查看权限" />
      )}
    </Drawer>
  );
}
