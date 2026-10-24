'use client';

import React, { useRef, useState } from 'react';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType } from '@ant-design/pro-components';
import {
  Alert,
  Button,
  Dropdown,
  Input,
  message,
  Modal,
  Popconfirm,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { DownOutlined, ReloadOutlined, UserDeleteOutlined } from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import { batchKickUsers, getOnlineUsers, getUserSession } from '@/api/adminController';
import { createStepUpChallenge, verifyStepUp } from '@/api/authController';
import {
  kickAdminUserAllSsoSessions,
  kickAdminUserSsoDevice,
  listAdminUserSsoDevices,
} from '@/api/ssoController';
import { useOnlineUserColumns } from './components/OnlineUserColumns';
import { OnlineUserStats } from './components/OnlineUserStats';
import { SessionDetailModal } from './components/SessionDetailModal';
import { usePermissionFlags } from '@/hooks/auth';
import { ADMIN_PERMISSIONS } from '@/lib/auth/permissionCodes';

type ProtectedAction = (stepUpToken?: string) => Promise<void>;

const { Text } = Typography;

function parseErrorMessage(error: unknown) {
  if (!error) {
    return '操作失败';
  }
  if (typeof error === 'string') {
    return error === 'STEP_UP_CANCELLED' ? '已取消二次验证' : error;
  }
  const payload = error as { message?: string };
  if (payload.message === 'STEP_UP_CANCELLED') {
    return '已取消二次验证';
  }
  return payload.message || '操作失败';
}

function isStepUpRequiredError(error: unknown) {
  const messageText = parseErrorMessage(error);
  return messageText.includes('STEP_UP_REQUIRED');
}

function formatDateTime(value?: string) {
  if (!value) {
    return '-';
  }
  const time = new Date(value);
  if (Number.isNaN(time.getTime())) {
    return value;
  }
  return time.toLocaleString();
}

function isValidUserId(value?: API.Int64): value is API.Int64 {
  return typeof value === 'string' && /^[1-9]\d*$/.test(value);
}

export default function OnlineUserPage() {
  const actionRef = useRef<ActionType | undefined>(undefined);
  const stepUpResolveRef = useRef<((value: string) => void) | null>(null);
  const stepUpRejectRef = useRef<((reason?: unknown) => void) | null>(null);

  const [selectedRows, setSelectedRows] = useState<API.OnlineUserVO[]>([]);
  const [statsRefreshTrigger, setStatsRefreshTrigger] = useState(0);
  const [sessionDetailVisible, setSessionDetailVisible] = useState(false);
  const [currentUserSession, setCurrentUserSession] = useState<API.OnlineUserVO | null>(null);

  const [deviceModalVisible, setDeviceModalVisible] = useState(false);
  const [deviceTargetUser, setDeviceTargetUser] = useState<API.OnlineUserVO | null>(null);
  const [devicesLoading, setDevicesLoading] = useState(false);
  const [devices, setDevices] = useState<API.UserDeviceVO[]>([]);

  const [stepUpVisible, setStepUpVisible] = useState(false);
  const [stepUpSubmitting, setStepUpSubmitting] = useState(false);
  const [stepUpScene, setStepUpScene] = useState('');
  const [stepUpChallengeId, setStepUpChallengeId] = useState('');
  const [stepUpPassword, setStepUpPassword] = useState('');
  const deviceRiskControlsSupported = false;
  const { canViewOnline, canKickOnline } = usePermissionFlags({
    canViewOnline: ADMIN_PERMISSIONS.ONLINE_VIEW,
    canKickOnline: ADMIN_PERMISSIONS.ONLINE_KICK,
  });

  const batchKickMutation = useMutation({
    mutationFn: (body: Parameters<typeof batchKickUsers>[0]) => batchKickUsers(body),
    onSuccess: () => {
      message.success('批量下线操作完成');
      actionRef.current?.reload();
      setSelectedRows([]);
      setStatsRefreshTrigger((prev) => prev + 1);
    },
    onError: (error: unknown) => {
      message.error(parseErrorMessage(error));
    },
  });

  const closeStepUpModal = () => {
    setStepUpVisible(false);
    setStepUpSubmitting(false);
    setStepUpScene('');
    setStepUpChallengeId('');
    setStepUpPassword('');
    stepUpResolveRef.current = null;
    stepUpRejectRef.current = null;
  };

  const openStepUpModal = async (scene: string): Promise<string> => {
    const challengeResponse = await createStepUpChallenge({ scene });
    const challengeId = challengeResponse?.data?.challengeId;
    if (!challengeId) {
      throw new Error('无法创建 step-up 挑战');
    }

    setStepUpScene(scene);
    setStepUpChallengeId(challengeId);
    setStepUpPassword('');
    setStepUpVisible(true);

    return new Promise<string>((resolve, reject) => {
      stepUpResolveRef.current = resolve;
      stepUpRejectRef.current = reject;
    });
  };

  const confirmStepUp = async () => {
    if (!stepUpChallengeId) {
      message.error('step-up 挑战不存在，请重试');
      return;
    }
    if (!stepUpPassword.trim()) {
      message.warning('请输入当前账号密码');
      return;
    }
    setStepUpSubmitting(true);
    try {
      const response = await verifyStepUp({
        challengeId: stepUpChallengeId,
        password: stepUpPassword,
      });
      const token = response?.data?.stepUpToken;
      if (!token) {
        throw new Error('step-up 令牌为空');
      }
      stepUpResolveRef.current?.(token);
      closeStepUpModal();
      message.success('二次验证通过');
    } catch (error) {
      setStepUpSubmitting(false);
      message.error(parseErrorMessage(error));
    }
  };

  const runProtectedAction = async (scene: string, action: ProtectedAction) => {
    try {
      await action(undefined);
      return;
    } catch (error) {
      if (!isStepUpRequiredError(error)) {
        throw error;
      }
    }

    const stepUpToken = await openStepUpModal(scene);
    await action(stepUpToken);
  };

  const withStepUpHeaders = (token?: string) => {
    if (!token) {
      return {};
    }
    return {
      headers: {
        'X-Step-Up-Token': token,
      },
    };
  };

  const loadUserDevices = async (userId: API.Int64) => {
    setDevicesLoading(true);
    try {
      const deviceResp = await listAdminUserSsoDevices({ userId });
      setDevices(deviceResp?.data || []);
    } catch (error) {
      message.error(parseErrorMessage(error));
    } finally {
      setDevicesLoading(false);
    }
  };

  const handleForceLogout = async (record: API.OnlineUserVO) => {
    const userId = record.userId;
    if (!isValidUserId(userId)) {
      message.error('用户ID无效');
      return;
    }
    try {
      await runProtectedAction('admin-user-kick', async (stepUpToken) => {
        await kickAdminUserAllSsoSessions(
          { userId },
          withStepUpHeaders(stepUpToken),
        );
      });
      message.success('用户已强制下线');
      actionRef.current?.reload();
      setStatsRefreshTrigger((prev) => prev + 1);
    } catch (error) {
      message.error(parseErrorMessage(error));
    }
  };

  const handleManageDevices = async (record: API.OnlineUserVO) => {
    const userId = record.userId;
    if (!isValidUserId(userId)) {
      message.error('用户ID无效');
      return;
    }
    setDeviceTargetUser(record);
    setDeviceModalVisible(true);
    await loadUserDevices(userId);
  };

  const handleKickDevice = async (deviceId?: string) => {
    const userId = deviceTargetUser?.userId;
    if (!isValidUserId(userId) || !deviceId) {
      return;
    }
    try {
      await runProtectedAction('admin-device-kick', async (stepUpToken) => {
        await kickAdminUserSsoDevice(
          {
            userId,
            deviceId,
          },
          withStepUpHeaders(stepUpToken),
        );
      });
      message.success('设备会话已踢出');
      await loadUserDevices(userId);
      actionRef.current?.reload();
      setStatsRefreshTrigger((prev) => prev + 1);
    } catch (error) {
      message.error(parseErrorMessage(error));
    }
  };

  const handleViewSession = async (record: API.OnlineUserVO) => {
    if (!record.userId) {
      message.error('用户ID无效');
      return;
    }

    try {
      const response = await getUserSession({ userId: record.userId });
      if (response?.code === 0 || response?.code === 200) {
        setCurrentUserSession(response.data || record);
      } else {
        setCurrentUserSession(record);
      }
      setSessionDetailVisible(true);
    } catch {
      message.warning('获取会话详情失败，已显示基础信息');
      setCurrentUserSession(record);
      setSessionDetailVisible(true);
    }
  };

  const handleBatchForceLogout = async () => {
    if (selectedRows.length === 0) {
      message.warning('请选择要下线的用户');
      return;
    }

    const protectedUsers = selectedRows.filter(
      (user) => user.isCurrentSession || (user.userRole || '').toUpperCase().includes('ADMIN'),
    );

    if (protectedUsers.length > 0) {
      message.warning('选择的用户中包含当前用户或管理员，无法执行批量下线');
      return;
    }

    Modal.confirm({
      title: '批量强制下线',
      content: `确定要强制 ${selectedRows.length} 个用户下线吗？`,
      onOk: async () => {
        const userIds = selectedRows.map((user) => user.userId!).filter((id) => id);
        await batchKickMutation.mutateAsync(userIds);
      },
    });
  };

  const handleRefresh = () => {
    actionRef.current?.reload();
    setStatsRefreshTrigger((prev) => prev + 1);
    message.success('数据已刷新');
  };

  const columns = useOnlineUserColumns({
    handleForceLogout,
    handleViewSession,
    handleManageDevices,
    canViewOnline,
    canKickOnline,
  });

  const dropdownItems = canKickOnline
    ? [
        {
          key: 'batchLogout',
          label: '批量强制下线',
          icon: <UserDeleteOutlined />,
          disabled: selectedRows.length === 0,
        },
      ]
    : [];

  const requestData = async (params: Record<string, unknown>) => {
    try {
      const response = await getOnlineUsers({
        current: Number(params.current) || 1,
        pageSize: Number(params.pageSize) || 10,
        userName: (params.userName as string) || (params.username as string),
        loginIp: (params.loginIp as string) || undefined,
        browser: (params.browser as string) || undefined,
        os: (params.os as string) || undefined,
      });

      const responseData = response?.data;
      const records = responseData?.records || [];
      const total = responseData?.total || 0;
      return {
        success: response?.code === 0 || response?.code === 200,
        data: records,
        total,
      };
    } catch (error) {
      console.error('获取在线用户列表失败:', error);
      return {
        success: false,
        data: [],
        total: 0,
      };
    }
  };

  const deviceColumns: ColumnsType<API.UserDeviceVO> = [
    {
      title: '设备ID',
      dataIndex: 'deviceId',
      key: 'deviceId',
      render: (value: string) => <Text code>{value || '-'}</Text>,
    },
    {
      title: '设备信息',
      dataIndex: 'deviceInfo',
      key: 'deviceInfo',
      render: (value: string) => value || '-',
    },
    {
      title: '会话数',
      dataIndex: 'sessionCount',
      key: 'sessionCount',
      width: 90,
      render: (value: number) => value ?? 0,
    },
    {
      title: '最近活跃',
      dataIndex: 'lastActiveTime',
      key: 'lastActiveTime',
      render: (value: string) => formatDateTime(value),
    },
    {
      title: 'IP样本',
      dataIndex: 'ipSamples',
      key: 'ipSamples',
      render: (value?: string[]) => (value && value.length > 0 ? value.join(', ') : '-'),
    },
    {
      title: '状态',
      key: 'status',
      width: 120,
      render: (_, record) => {
        const tags: React.ReactNode[] = [];
        if (record.currentDevice) {
          tags.push(
            <Tag color="blue" key="current">
              当前设备
            </Tag>,
          );
        }
        if (tags.length === 0) {
          tags.push(
            <Tag color="green" key="normal">
              正常
            </Tag>,
          );
        }
        return <Space size={4}>{tags}</Space>;
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 140,
      render: (_, record) => (
        <Space size={8}>
          <Popconfirm
            title="确认踢出该设备全部会话？"
            onConfirm={() => handleKickDevice(record.deviceId)}
          >
            <Button type="link" danger size="small">
              踢出
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <OnlineUserStats refreshTrigger={statsRefreshTrigger} />

      <ProTable<API.OnlineUserVO>
        headerTitle="在线用户列表"
        actionRef={actionRef}
        rowKey="userId"
        search={{
          labelWidth: 120,
          collapsed: false,
          collapseRender: false,
        }}
        toolBarRender={() => [
          <Button key="refresh" icon={<ReloadOutlined />} onClick={handleRefresh}>
            刷新
          </Button>,
          ...(dropdownItems.length > 0
            ? [
                <Dropdown
                  key="batch"
                  menu={{
                    items: dropdownItems,
                    onClick: ({ key }) => {
                      if (key === 'batchLogout') {
                        handleBatchForceLogout();
                      }
                    },
                  }}
                  disabled={selectedRows.length === 0}
                >
                  <Button>
                    批量操作 <DownOutlined />
                  </Button>
                </Dropdown>,
              ]
            : []),
        ]}
        request={requestData}
        columns={columns}
        rowSelection={
          canKickOnline
            ? {
                type: 'checkbox',
                selectedRowKeys: selectedRows.map((row) => row.userId!).filter(Boolean),
                onChange: (_, rows) => {
                  setSelectedRows(rows);
                },
                getCheckboxProps: (record) => ({
                  disabled: record.isCurrentSession || (record.userRole || '').toUpperCase().includes('ADMIN'),
                }),
              }
            : undefined
        }
        pagination={{
          defaultPageSize: 10,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
        }}
        tableAlertRender={({ selectedRowKeys, onCleanSelected }) => (
          <Space size={24}>
            <span>
              已选择 <a style={{ fontWeight: 600 }}>{selectedRowKeys.length}</a> 项
              <a style={{ marginLeft: 8 }} onClick={onCleanSelected}>
                取消选择
              </a>
            </span>
          </Space>
        )}
        tableAlertOptionRender={() =>
          canKickOnline ? (
            <Space size={16}>
              <a onClick={handleBatchForceLogout}>批量强制下线</a>
            </Space>
          ) : null
        }
        options={{
          density: true,
          fullScreen: true,
          reload: true,
          setting: true,
        }}
        scroll={{ x: 1400 }}
      />

      <SessionDetailModal
        visible={sessionDetailVisible}
        userSession={currentUserSession}
        onCancel={() => {
          setSessionDetailVisible(false);
          setCurrentUserSession(null);
        }}
      />

      <Modal
        title={`设备治理 - ${deviceTargetUser?.username || ''}`}
        open={deviceModalVisible}
        width={1100}
        onCancel={() => {
          setDeviceModalVisible(false);
          setDeviceTargetUser(null);
          setDevices([]);
        }}
        footer={null}
      >
        <Space style={{ marginBottom: 16 }} wrap>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => {
              if (deviceTargetUser?.userId) {
                const userId = deviceTargetUser.userId;
                if (isValidUserId(userId)) {
                  void loadUserDevices(userId);
                }
              }
            }}
          >
            刷新设备
          </Button>
        </Space>

        {!deviceRiskControlsSupported ? (
          <Alert
            style={{ marginBottom: 16 }}
            type="info"
            showIcon
            message="设备冻结与设备并发上限已从 authorization-app 主链移除"
            description="当前系统已切到统一 SSO 会话权威面，管理员可以继续查看设备和踢出设备会话；设备冻结、设备并发上限这类统一风控能力会收敛到 SSO 中心后再恢复。"
          />
        ) : null}

        <Table<API.UserDeviceVO>
          rowKey={(record) => record.deviceId || ''}
          loading={devicesLoading}
          columns={deviceColumns}
          dataSource={devices}
          pagination={false}
          locale={{ emptyText: '暂无设备会话数据' }}
          scroll={{ x: 1000 }}
        />
      </Modal>

      <Modal
        title="管理员二次验证"
        open={stepUpVisible}
        okText="验证"
        cancelText="取消"
        confirmLoading={stepUpSubmitting}
        onOk={confirmStepUp}
        onCancel={() => {
          stepUpRejectRef.current?.(new Error('STEP_UP_CANCELLED'));
          closeStepUpModal();
        }}
      >
        <Space orientation="vertical" style={{ width: '100%' }}>
          <Text type="secondary">当前操作需要二次验证（scene: {stepUpScene || 'default'}）</Text>
          <Input.Password
            autoFocus
            value={stepUpPassword}
            placeholder="请输入当前登录账号密码"
            onChange={(event) => setStepUpPassword(event.target.value)}
            onPressEnter={confirmStepUp}
          />
        </Space>
      </Modal>
    </div>
  );
}
