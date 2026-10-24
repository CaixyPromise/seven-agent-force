'use client';

import {
  Alert,
  Avatar,
  Button,
  Empty,
  Skeleton,
  Tag,
  Tooltip,
  Typography,
} from 'antd';
import {
  ArrowRightOutlined,
  ClockCircleOutlined,
  FileSearchOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
} from '@ant-design/icons';
import { useQueries } from '@tanstack/react-query';
import { useEffect, useMemo, startTransition } from 'react';
import { useNavigate } from 'react-router-dom';
import { getOnlineUserStats } from '@/api/adminController';
import { getOperationLogs } from '@/api/operationLogController';
import { getCurrentUserSsoDevices, getCurrentUserSsoSessions } from '@/api/ssoController';
import { listUsers } from '@/api/userController';
import Footer from '@/components/Footer';
import { useAuth, usePermissionAccess } from '@/hooks/auth';
import { useAuthStore } from '@/store/auth';
import WorkforceWorkbench from './workforce/_components/WorkforceWorkbench';

const { Title, Paragraph, Text } = Typography;

const ADMIN_QUICK_LINKS = [
  {
    title: '用户与权限',
    description: '进入用户、角色、菜单和权限模型维护。',
    path: '/system/user',
  },
  {
    title: '在线会话',
    description: '处理在线用户、会话与设备下线。',
    path: '/system/online-user',
  },
  {
    title: '配置与字典',
    description: '统一维护运行配置、配置组与字典体系。',
    path: '/system/config',
  },
  {
    title: '文件与存储',
    description: '查看文件处理链路、任务状态和存储策略。',
    path: '/system/files',
  },
];

const PERSONAL_QUICK_LINKS = [
  {
    title: '个人安全设置',
    description: '管理密码、Passkey、OTP 与恢复码，维持账号安全基线。',
    path: '/account/settings',
  },
];

function formatCount(value?: number) {
  return new Intl.NumberFormat('zh-CN').format(value ?? 0);
}

function formatDateTime(value?: string | number) {
  if (!value) return '暂无';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return String(value);
  }
  return date.toLocaleString('zh-CN', {
    hour12: false,
  });
}

function getGreeting() {
  const hour = new Date().getHours();
  if (hour < 6) return '夜间巡检';
  if (hour < 11) return '早安，控制台已就绪';
  if (hour < 14) return '午间态势概览';
  if (hour < 18) return '下午好，系统运行稳定';
  return '晚间值守面板';
}

export default function HomePage() {
  const canViewWorkforce = usePermissionAccess('workforce:task:list');
  return canViewWorkforce ? <WorkforceWorkbench /> : <PersonalSecurityHome />;
}

function PersonalSecurityHome() {
  const navigate = useNavigate();
  const user = useAuthStore((state) => state.user);
  const { isAdmin } = useAuth();
  const canViewAdminWorkspace = isAdmin;
  const hasResolvedUser = Boolean(user?.id);

  useEffect(() => {
    document.title = '工作台 - Seven Agent Force';
  }, []);

  const [onlineStatsQuery, sessionsQuery, devicesQuery, usersQuery, operationLogsQuery] = useQueries({
    queries: [
      {
        queryKey: ['dashboard', 'online-stats'],
        queryFn: () => getOnlineUserStats(),
        staleTime: 60 * 1000,
        enabled: canViewAdminWorkspace,
      },
      {
        queryKey: ['dashboard', 'current-sessions'],
        queryFn: () => getCurrentUserSsoSessions(),
        staleTime: 60 * 1000,
        enabled: hasResolvedUser,
      },
      {
        queryKey: ['dashboard', 'current-devices'],
        queryFn: () => getCurrentUserSsoDevices(),
        staleTime: 60 * 1000,
        enabled: hasResolvedUser,
      },
      {
        queryKey: ['dashboard', 'users-total'],
        queryFn: () => listUsers({ current: 1, size: 1 }),
        staleTime: 60 * 1000,
        enabled: canViewAdminWorkspace,
      },
      {
        queryKey: ['dashboard', 'recent-operation-logs'],
        queryFn: () => getOperationLogs({ current: 1, size: 5 }),
        staleTime: 30 * 1000,
        enabled: canViewAdminWorkspace,
      },
    ],
  });

  const onlineStats = onlineStatsQuery.data?.data;
  const sessions = useMemo(() => sessionsQuery.data?.data ?? [], [sessionsQuery.data?.data]);
  const devices = useMemo(() => devicesQuery.data?.data ?? [], [devicesQuery.data?.data]);
  const usersPage = usersQuery.data?.data;
  const operationLogs = operationLogsQuery.data?.data?.records ?? [];

  const workspaceSummary = useMemo(() => {
    const sessionList = sessions ?? [];
    const deviceList = devices ?? [];
    const totalOnline = Number(onlineStats?.totalOnlineUsers ?? onlineStats?.totalOnline ?? 0);
    const todayLogin = Number(onlineStats?.todayLoginUsers ?? onlineStats?.todayLogin ?? 0);
    const adminUsers = Number(onlineStats?.adminUsers ?? 0);
    const activeUsers = Number(onlineStats?.activeUsers ?? 0);
    const totalUsers = usersPage?.total ?? 0;
    const totalSessions = sessionList.length;
    const totalDevices = deviceList.length;
    const currentDevice = deviceList.find((item) => item.currentDevice);

    return {
      totalOnline,
      todayLogin,
      adminUsers,
      activeUsers,
      totalUsers,
      totalSessions,
      totalDevices,
      currentDevice,
      currentPermissionCount: user?.permissions?.length ?? 0,
      currentRoleCount: user?.roleCodes?.length ?? 0,
    };
  }, [devices, onlineStats, sessions, user?.permissions?.length, user?.roleCodes?.length, usersPage?.total]);

  const healthSignals = useMemo(() => {
    const deviceActivity = Math.min(100, workspaceSummary.totalDevices * 18 + 28);
    const sessionActivity = Math.min(100, workspaceSummary.totalSessions * 15 + 20);
    const loginHeat =
      workspaceSummary.totalOnline > 0
        ? Math.min(100, Math.round((workspaceSummary.todayLogin / workspaceSummary.totalOnline) * 100))
        : 0;
    const permissionCoverage = Math.min(
      100,
      workspaceSummary.currentPermissionCount * 6 + workspaceSummary.currentRoleCount * 18,
    );

    return [
      {
        label: '设备信号',
        value: deviceActivity,
        tone: '#0ea5e9',
      },
      {
        label: '会话活跃度',
        value: sessionActivity,
        tone: '#38bdf8',
      },
      {
        label: canViewAdminWorkspace ? '今日登录热度' : '权限覆盖',
        value: canViewAdminWorkspace ? loginHeat : permissionCoverage,
        tone: '#7dd3fc',
      },
    ];
  }, [canViewAdminWorkspace, workspaceSummary]);

  const hasError = (canViewAdminWorkspace
    ? [
        onlineStatsQuery.isError,
        sessionsQuery.isError,
        devicesQuery.isError,
        usersQuery.isError,
        operationLogsQuery.isError,
      ]
    : [sessionsQuery.isError, devicesQuery.isError]
  ).some(Boolean);

  const isLoading = (canViewAdminWorkspace
    ? [
        onlineStatsQuery.isLoading,
        sessionsQuery.isLoading,
        devicesQuery.isLoading,
        usersQuery.isLoading,
        operationLogsQuery.isLoading,
      ]
    : [sessionsQuery.isLoading, devicesQuery.isLoading]
  ).some(Boolean);

  const statsCards = canViewAdminWorkspace
    ? [
        {
          label: '在线用户',
          value: workspaceSummary.totalOnline,
          hint: `今日登录 ${formatCount(workspaceSummary.todayLogin)}`,
          icon: <RadarChartOutlined />,
        },
        {
          label: '平台用户',
          value: workspaceSummary.totalUsers,
          hint: `管理员 ${formatCount(workspaceSummary.adminUsers)}`,
          icon: <TeamOutlined />,
        },
        {
          label: '我的会话',
          value: workspaceSummary.totalSessions,
          hint: `设备 ${formatCount(workspaceSummary.totalDevices)}`,
          icon: <SafetyCertificateOutlined />,
        },
        {
          label: '活跃峰值',
          value: Number(onlineStats?.peakOnline ?? workspaceSummary.activeUsers),
          hint: `活跃用户 ${formatCount(workspaceSummary.activeUsers)}`,
          icon: <ClockCircleOutlined />,
        },
      ]
    : [
        {
          label: '我的设备',
          value: workspaceSummary.totalDevices,
          hint: workspaceSummary.currentDevice ? '已识别当前设备' : '设备待识别',
          icon: <RadarChartOutlined />,
        },
        {
          label: '我的会话',
          value: workspaceSummary.totalSessions,
          hint: `最近活跃 ${formatDateTime(workspaceSummary.currentDevice?.lastActiveTime)}`,
          icon: <SafetyCertificateOutlined />,
        },
        {
          label: '角色数量',
          value: workspaceSummary.currentRoleCount,
          hint: user?.nickname || user?.username || '当前账号',
          icon: <TeamOutlined />,
        },
        {
          label: '权限点',
          value: workspaceSummary.currentPermissionCount,
          hint: '仅展示当前已授予权限',
          icon: <ClockCircleOutlined />,
        },
      ];

  const quickLinks = canViewAdminWorkspace ? ADMIN_QUICK_LINKS : PERSONAL_QUICK_LINKS;

  const handleNavigate = (path: string) => {
    startTransition(() => {
      navigate(path);
    });
  };

  return (
    <div
      className="dashboard-page"
      style={{
        width: '100%',
        minWidth: 0,
        minHeight: '100%',
        padding: '8px 24px 16px',
        background:
          'linear-gradient(180deg, rgba(224, 242, 254, 0.9) 0%, rgba(240, 249, 255, 0.94) 40%, rgba(255, 255, 255, 0.98) 100%)',
      }}
    >
      <div
        className="dashboard-shell"
        style={{
          width: '100%',
          minWidth: 0,
          maxWidth: 1440,
          margin: '0 auto',
          display: 'grid',
          gap: 22,
        }}
      >
        <section
          className="dashboard-surface dashboard-hero"
          style={{
            position: 'relative',
            overflow: 'hidden',
            borderRadius: 32,
            padding: 30,
            background:
              'linear-gradient(135deg, rgba(125, 211, 252, 0.98) 0%, rgba(186, 230, 253, 0.96) 52%, rgba(255, 255, 255, 0.98) 100%)',
            color: '#0f172a',
            boxShadow: '0 32px 72px -44px rgba(14, 116, 144, 0.2)',
            border: '1px solid rgba(186, 230, 253, 0.88)',
          }}
        >
          <div
            style={{
              position: 'absolute',
              width: 420,
              height: 420,
              borderRadius: '50%',
              top: -180,
              left: -90,
              background: 'radial-gradient(circle, rgba(255,255,255,0.26) 0%, rgba(255,255,255,0) 72%)',
              pointerEvents: 'none',
            }}
          />
          <div
            style={{
              position: 'absolute',
              width: 380,
              height: 380,
              borderRadius: '50%',
              right: -120,
              bottom: -180,
              background: 'radial-gradient(circle, rgba(191,219,254,0.28) 0%, rgba(255,255,255,0) 74%)',
              pointerEvents: 'none',
            }}
          />
          <div
            style={{
              position: 'absolute',
              inset: 0,
              background:
                'linear-gradient(90deg, rgba(255,255,255,0.08) 1px, transparent 1px), linear-gradient(rgba(255,255,255,0.05) 1px, transparent 1px)',
              backgroundSize: '26px 26px',
              opacity: 0.12,
              pointerEvents: 'none',
            }}
          />
          <div
            className="dashboard-hero-grid"
            style={{
              position: 'relative',
              display: 'grid',
              gridTemplateColumns: 'minmax(0, 1.45fr) minmax(280px, 0.9fr)',
              gap: 20,
            }}
          >
            <div>
              <Tag
                style={{
                  marginBottom: 16,
                  borderRadius: 999,
                  paddingInline: 12,
                  paddingBlock: 6,
                  border: '1px solid rgba(15, 23, 42, 0.1)',
                  background: 'rgba(255, 255, 255, 0.78)',
                  color: '#0f172a',
                  fontSize: 12,
                  letterSpacing: 1.2,
                  fontWeight: 700,
                }}
              >
                统一工作台
              </Tag>
              <Title
                level={1}
                style={{
                  margin: 0,
                  color: '#0f172a',
                  fontFamily: '"Iowan Old Style", "Palatino Linotype", Georgia, serif',
                  fontSize: 'clamp(2rem, 3vw, 3.15rem)',
                  lineHeight: 1.12,
                  letterSpacing: '-0.04em',
                  maxWidth: 560,
                }}
              >
                {getGreeting()}
              </Title>
              <Paragraph
                style={{
                  marginTop: 14,
                  marginBottom: 22,
                  maxWidth: 580,
                  color: 'rgba(15, 23, 42, 0.78)',
                  fontSize: 'clamp(0.98rem, 1.5vw, 1.0625rem)',
                  lineHeight: 1.82,
                  fontWeight: 500,
                }}
              >
                {canViewAdminWorkspace
                  ? '这是 Seven Agent Force 的运营工作台。你可以在这里快速掌握在线态势、账号安全、设备活跃度以及最近的系统操作，作为日常巡检和管理入口。'
                  : '这里展示当前账号的设备、会话与安全状态。若后续被授予管理角色，首页会自动切换为平台工作台视图。'}
              </Paragraph>

              <div
                className="dashboard-hero-actions"
                style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'center' }}
              >
                <div
                  className="dashboard-user-pill"
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 12,
                    padding: '10px 14px',
                    borderRadius: 20,
                    background: 'rgba(255, 255, 255, 0.84)',
                    border: '1px solid rgba(15, 23, 42, 0.08)',
                    backdropFilter: 'blur(8px)',
                    boxShadow: '0 18px 36px -28px rgba(15, 23, 42, 0.28)',
                  }}
                >
                  <Avatar size={44} src={user?.userAvatar || undefined}>
                    {(user?.nickname || user?.username || 'S').slice(0, 1)}
                  </Avatar>
                  <div>
                    <Text style={{ display: 'block', color: '#0f172a', fontWeight: 600 }}>
                      {user?.nickname || user?.username || '系统管理员'}
                    </Text>
                    <Text style={{ color: 'rgba(15, 23, 42, 0.7)' }}>
                      {workspaceSummary.currentRoleCount} 个角色 · {workspaceSummary.currentPermissionCount} 个权限点
                    </Text>
                  </div>
                </div>

                <Button
                  type="primary"
                  size="large"
                  icon={<ArrowRightOutlined />}
                  onClick={() =>
                    handleNavigate(canViewAdminWorkspace ? '/system/online-user' : '/account/settings')
                  }
                  style={{
                    borderRadius: 18,
                    height: 50,
                    paddingInline: 20,
                    background: 'linear-gradient(90deg, #3b82f6 0%, #22d3ee 100%)',
                    color: '#ffffff',
                    border: '1px solid rgba(56, 189, 248, 0.92)',
                    boxShadow: '0 16px 36px rgba(14, 165, 233, 0.26)',
                    fontWeight: 700,
                  }}
                >
                  {canViewAdminWorkspace ? '进入在线会话面板' : '进入个人安全设置'}
                </Button>
              </div>
            </div>

            <div
              className="dashboard-surface"
              style={{
                padding: 20,
                borderRadius: 24,
                background: 'rgba(255, 255, 255, 0.82)',
                border: '1px solid rgba(191, 219, 254, 0.82)',
                backdropFilter: 'blur(14px)',
                boxShadow: '0 24px 48px -34px rgba(14, 116, 144, 0.18)',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 14 }}>
                <Text style={{ color: '#0f172a', fontWeight: 700, fontSize: 15 }}>运行侧写</Text>
                <Tag color="default" style={{ color: '#0f172a', borderColor: '#dbeafe', background: '#f8fafc' }}>
                  实时
                </Tag>
              </div>
              <div style={{ display: 'grid', gap: 14 }}>
                {healthSignals.map((signal) => (
                  <div key={signal.label}>
                    <div
                      style={{
                        display: 'flex',
                        justifyContent: 'space-between',
                        marginBottom: 6,
                        color: '#0f172a',
                      }}
                    >
                      <span>{signal.label}</span>
                      <span>{signal.value}%</span>
                    </div>
                    <div
                      style={{
                        height: 10,
                        borderRadius: 999,
                        background: 'rgba(148, 163, 184, 0.16)',
                        overflow: 'hidden',
                      }}
                    >
                      <div
                        style={{
                          width: `${signal.value}%`,
                          height: '100%',
                          borderRadius: 999,
                          background: `linear-gradient(90deg, ${signal.tone}, #22d3ee)`,
                        }}
                      />
                    </div>
                  </div>
                ))}
              </div>
              <div
                style={{
                  marginTop: 18,
                  paddingTop: 18,
                  borderTop: '1px solid rgba(255,255,255,0.18)',
                  display: 'grid',
                  gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
                  gap: 12,
                }}
              >
                <div>
                  <Text style={{ display: 'block', color: '#64748b' }}>当前设备</Text>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>
                    {workspaceSummary.currentDevice?.deviceInfo || '未识别'}
                  </Text>
                </div>
                <div>
                  <Text style={{ display: 'block', color: '#64748b' }}>最近活跃</Text>
                  <Text style={{ color: '#0f172a', fontWeight: 700 }}>
                    {formatDateTime(workspaceSummary.currentDevice?.lastActiveTime)}
                  </Text>
                </div>
              </div>
            </div>
          </div>
        </section>

        {hasError && (
          <Alert
            type="warning"
            showIcon
            title="工作台部分数据加载失败"
            description={
              canViewAdminWorkspace
                ? '首页会继续展示已获取的数据，你可以稍后刷新页面，或进入具体模块查看详情。'
                : '个人工作台会继续展示已获取的数据，你可以稍后刷新页面，或进入个人中心查看安全设置。'
            }
          />
        )}

        <section
          className="dashboard-stats-grid"
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
            gap: 18,
          }}
        >
          {statsCards.map((item) => (
            <div
              key={item.label}
              className="dashboard-surface"
              style={{
                padding: 22,
                borderRadius: 26,
                background:
                  'linear-gradient(180deg, rgba(255,255,255,0.92) 0%, rgba(248,252,255,0.98) 100%)',
                border: '1px solid rgba(191,219,254,0.46)',
                boxShadow: '0 26px 64px -38px rgba(15, 23, 42, 0.18)',
                contentVisibility: 'auto',
              }}
            >
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ color: '#334155', fontWeight: 700, fontSize: 15 }}>{item.label}</Text>
                <div
                  style={{
                    width: 46,
                    height: 46,
                    borderRadius: 18,
                    display: 'grid',
                    placeItems: 'center',
                    color: '#ffffff',
                    background: 'linear-gradient(135deg, #2563eb 0%, #22d3ee 100%)',
                    border: '1px solid rgba(56, 189, 248, 0.72)',
                    boxShadow: '0 16px 30px -18px rgba(14, 165, 233, 0.34)',
                  }}
                >
                  {item.icon}
                </div>
              </div>
              {isLoading ? (
                <Skeleton active paragraph={false} style={{ marginTop: 18 }} />
              ) : (
                <>
                  <div
                    style={{
                      marginTop: 18,
                      fontSize: 38,
                      lineHeight: 1,
                      fontWeight: 700,
                      letterSpacing: '-0.05em',
                      color: '#0b1324',
                    }}
                  >
                    {formatCount(item.value as number)}
                  </div>
                  <Text style={{ marginTop: 12, display: 'block', color: '#64748b', fontSize: 14 }}>
                    {item.hint}
                  </Text>
                </>
              )}
            </div>
          ))}
        </section>

        <section
          className="dashboard-dual-grid"
          style={{
            display: 'grid',
            gridTemplateColumns: 'minmax(0, 1.15fr) minmax(300px, 0.85fr)',
            gap: 20,
          }}
        >
          <div
            className="dashboard-surface"
            style={{
              padding: 24,
              borderRadius: 28,
              background:
                'linear-gradient(180deg, rgba(255,255,255,0.92) 0%, rgba(248,252,255,0.98) 100%)',
              border: '1px solid rgba(191,219,254,0.44)',
              boxShadow: '0 28px 64px -40px rgba(15, 23, 42, 0.18)',
              contentVisibility: 'auto',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 22 }}>
              <div>
                <Title
                  level={3}
                  style={{
                    margin: 0,
                    fontFamily: '"Iowan Old Style", Georgia, serif',
                    color: '#0f172a',
                    letterSpacing: '-0.03em',
                  }}
                >
                  快捷调度
                </Title>
                <Text style={{ color: '#64748b', fontSize: 14 }}>
                  {canViewAdminWorkspace ? '把最常访问的系统能力集中到一屏。' : '保留当前账号最常用的安全入口。'}
                </Text>
              </div>
              <Button
                type="link"
                onClick={() => handleNavigate(canViewAdminWorkspace ? '/system/user' : '/account/settings')}
              >
                {canViewAdminWorkspace ? '打开系统管理' : '打开个人中心'}
              </Button>
            </div>
            <div
              className="dashboard-actions-grid"
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
                gap: 14,
              }}
            >
              {quickLinks.map((item) => (
                <button
                  key={item.path}
                  type="button"
                  className="dashboard-action-card"
                  onClick={() => handleNavigate(item.path)}
                  style={{
                    textAlign: 'left',
                    padding: 20,
                    borderRadius: 22,
                    border: '1px solid rgba(191,219,254,0.42)',
                    background:
                      'linear-gradient(180deg, rgba(248,252,255,0.98) 0%, rgba(255,255,255,1) 100%)',
                    boxShadow:
                      '0 18px 38px -30px rgba(15, 23, 42, 0.14), inset 0 1px 0 rgba(255,255,255,0.82)',
                    cursor: 'pointer',
                    gridColumn: quickLinks.length === 1 ? '1 / -1' : undefined,
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                    <Text style={{ display: 'block', fontWeight: 700, color: '#0f172a', marginBottom: 8, fontSize: 16 }}>
                      {item.title}
                    </Text>
                    <ArrowRightOutlined style={{ color: '#38bdf8', fontSize: 16, marginTop: 2 }} />
                  </div>
                  <Text style={{ color: '#64748b', lineHeight: 1.8, fontSize: 14 }}>{item.description}</Text>
                </button>
              ))}
            </div>
          </div>

          <div
            className="dashboard-surface"
            style={{
              padding: 24,
              borderRadius: 28,
              background:
                'linear-gradient(180deg, rgba(255,255,255,0.92) 0%, rgba(248,252,255,0.98) 100%)',
              border: '1px solid rgba(191,219,254,0.44)',
              boxShadow: '0 28px 64px -40px rgba(15, 23, 42, 0.16)',
              contentVisibility: 'auto',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 22 }}>
              <div>
                <Title
                  level={3}
                  style={{
                    margin: 0,
                    fontFamily: '"Iowan Old Style", Georgia, serif',
                    color: '#0f172a',
                    letterSpacing: '-0.03em',
                  }}
                >
                  安全席位
                </Title>
                <Text style={{ color: '#64748b', fontSize: 14 }}>查看当前账号和设备会话状态。</Text>
              </div>
              <Button
                type="link"
                onClick={() => handleNavigate(canViewAdminWorkspace ? '/system/online-user' : '/account/settings')}
              >
                {canViewAdminWorkspace ? '查看全部会话' : '查看安全设置'}
              </Button>
            </div>

            <div style={{ display: 'grid', gap: 14 }}>
              <div
                className="dashboard-surface"
                style={{
                  padding: 18,
                  borderRadius: 22,
                  background: 'rgba(255,255,255,0.98)',
                  color: '#0f172a',
                  border: '1px solid rgba(191,219,254,0.42)',
                  boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.86)',
                }}
              >
                <Text style={{ display: 'block', color: '#64748b' }}>当前设备簇</Text>
                <Text style={{ display: 'block', marginTop: 8, color: '#0f172a', fontSize: 20, fontWeight: 700 }}>
                  {workspaceSummary.currentDevice?.deviceInfo || '未识别设备'}
                </Text>
                <Text style={{ display: 'block', marginTop: 8, color: '#64748b' }}>
                  最近活动 {formatDateTime(workspaceSummary.currentDevice?.lastActiveTime)}
                </Text>
              </div>

              <div
                className="dashboard-security-grid"
                style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: 12 }}
              >
                <div
                  className="dashboard-surface"
                  style={{
                    padding: 16,
                    borderRadius: 20,
                    background: 'rgba(255,255,255,0.98)',
                    border: '1px solid rgba(191,219,254,0.34)',
                  }}
                >
                  <Text style={{ color: '#64748b' }}>我的设备数</Text>
                  <div style={{ marginTop: 10, fontSize: 28, fontWeight: 700, color: '#0f172a' }}>
                    {formatCount(workspaceSummary.totalDevices)}
                  </div>
                </div>
                <div
                  className="dashboard-surface"
                  style={{
                    padding: 16,
                    borderRadius: 20,
                    background: 'rgba(255,255,255,0.98)',
                    border: '1px solid rgba(191,219,254,0.34)',
                  }}
                >
                  <Text style={{ color: '#64748b' }}>我的会话数</Text>
                  <div style={{ marginTop: 10, fontSize: 28, fontWeight: 700, color: '#0f172a' }}>
                    {formatCount(workspaceSummary.totalSessions)}
                  </div>
                </div>
              </div>

              <div
                className="dashboard-surface"
                style={{
                  padding: 16,
                  borderRadius: 20,
                  background: 'rgba(255,255,255,0.98)',
                  border: '1px solid rgba(191,219,254,0.34)',
                }}
              >
                <Text style={{ color: '#64748b', display: 'block', marginBottom: 10 }}>近期会话</Text>
                {sessions.length === 0 ? (
                  <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="当前没有可展示的会话" />
                ) : (
                  <div style={{ display: 'grid', gap: 10 }}>
                    {sessions.slice(0, 3).map((session) => (
                      <div
                        key={session.sessionId}
                        className="dashboard-log-row"
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          gap: 12,
                          padding: '10px 12px',
                          borderRadius: 16,
                          border: '1px solid rgba(226,232,240,0.84)',
                          background: 'rgba(248,250,252,0.72)',
                        }}
                      >
                        <div>
                          <Text style={{ display: 'block', color: '#0f172a', fontWeight: 600 }}>
                            {session.deviceInfo || session.userAgent || '未知会话'}
                          </Text>
                          <Text style={{ color: '#64748b' }}>{formatDateTime(session.lastActiveTime)}</Text>
                        </div>
                        {session.currentSession ? <Tag color="blue">当前</Tag> : <Tag>历史</Tag>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </section>

        {canViewAdminWorkspace ? (
          <section
            className="dashboard-surface"
            style={{
              padding: 24,
              borderRadius: 28,
              background:
                'linear-gradient(180deg, rgba(255,255,255,0.92) 0%, rgba(248,252,255,0.98) 100%)',
              border: '1px solid rgba(191,219,254,0.44)',
              boxShadow: '0 28px 64px -40px rgba(15, 23, 42, 0.16)',
              contentVisibility: 'auto',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 22 }}>
              <div>
                <Title
                  level={3}
                  style={{
                    margin: 0,
                    fontFamily: '"Iowan Old Style", Georgia, serif',
                    color: '#0f172a',
                    letterSpacing: '-0.03em',
                  }}
                >
                  最近操作
                </Title>
                <Text style={{ color: '#64748b', fontSize: 14 }}>
                  观察平台最近发生了什么，快速发现异常操作和热点模块。
                </Text>
              </div>
              <Button icon={<FileSearchOutlined />} onClick={() => handleNavigate('/system/operation-log')}>
                查看完整日志
              </Button>
            </div>

            {operationLogsQuery.isLoading ? (
              <Skeleton active paragraph={{ rows: 4 }} />
            ) : operationLogs.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无操作日志" />
            ) : (
              <div style={{ display: 'grid', gap: 12 }}>
                {operationLogs.map((log) => (
                  <div
                    key={log.id}
                    className="dashboard-log-row dashboard-ops-row"
                    style={{
                      display: 'grid',
                      gridTemplateColumns: '180px minmax(0, 1fr) 120px',
                      gap: 16,
                      alignItems: 'center',
                      padding: 18,
                      borderRadius: 20,
                      background: 'rgba(255,255,255,0.96)',
                      border: '1px solid rgba(226,232,240,0.84)',
                      boxShadow: 'inset 0 1px 0 rgba(255,255,255,0.82)',
                    }}
                  >
                    <div>
                      <Text style={{ display: 'block', color: '#0f172a', fontWeight: 600 }}>
                        {log.nickName || log.userName || '系统'}
                      </Text>
                      <Text style={{ color: '#64748b' }}>{formatDateTime(log.operationTime || log.createTime)}</Text>
                    </div>
                    <div>
                      <Text style={{ display: 'block', color: '#0f172a', fontWeight: 600 }}>
                        {log.operationTypeLabel || log.operationTypeDesc || log.operationType || '未知操作'}
                      </Text>
                      <Tooltip title={log.requestUrl}>
                        <Text style={{ color: '#64748b' }}>
                          {log.operationDesc || log.requestUrl || log.methodName || '无描述'}
                        </Text>
                      </Tooltip>
                    </div>
                    <div style={{ textAlign: 'right' }}>
                      <Tag color={log.status === 1 ? 'success' : 'error'}>
                        {log.status === 1 ? '成功' : '失败'}
                      </Tag>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>
        ) : (
          <section
            className="dashboard-surface"
            style={{
              padding: 24,
              borderRadius: 28,
              background:
                'linear-gradient(180deg, rgba(255,255,255,0.92) 0%, rgba(248,252,255,0.98) 100%)',
              border: '1px solid rgba(191,219,254,0.44)',
              boxShadow: '0 28px 64px -40px rgba(15, 23, 42, 0.16)',
              contentVisibility: 'auto',
            }}
          >
            <Alert
              type="info"
              showIcon
              title="当前账号尚未授予后台管理角色"
              description="首页已自动切换为个人安全视图。你仍可以管理当前账号的密码、Passkey、OTP 与恢复码；获得管理角色后，这里会自动显示平台态势和最近操作。"
              action={
                <Button type="primary" size="small" onClick={() => handleNavigate('/account/settings')}>
                  打开个人中心
                </Button>
              }
            />
          </section>
        )}
        <Footer />
      </div>
    </div>
  );
}
