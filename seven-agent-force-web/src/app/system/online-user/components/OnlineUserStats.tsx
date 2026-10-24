'use client';

import React from 'react';
import { Card, Row, Col, Statistic } from 'antd';
import { UserOutlined, TeamOutlined, GlobalOutlined, ClockCircleOutlined } from '@ant-design/icons';
import { useQuery } from '@tanstack/react-query';
import { getOnlineUserStats } from '@/api/adminController';

interface OnlineUserStatsProps {
  refreshTrigger: number;
}

export const OnlineUserStats: React.FC<OnlineUserStatsProps> = ({ refreshTrigger }) => {
  // 获取在线用户统计
  const { data: statsData, isLoading } = useQuery({
    queryKey: ['onlineUserStats', refreshTrigger],
    queryFn: () => getOnlineUserStats(),
  });

  const stats = statsData?.data || {
    totalOnline: 0,
    totalOnlineUsers: 0,
    activeUsers: 0,
    todayLogin: 0,
    peakOnline: 0,
  };

  const totalOnline = stats.totalOnline ?? stats.totalOnlineUsers ?? 0;

  return (
    <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
      <Col xs={24} sm={12} lg={6}>
        <Card>
          <Statistic
            title="在线用户"
            value={totalOnline}
            prefix={<UserOutlined />}
            loading={isLoading}
          />
        </Card>
      </Col>

      <Col xs={24} sm={12} lg={6}>
        <Card>
          <Statistic
            title="活跃用户"
            value={stats.activeUsers}
            prefix={<TeamOutlined />}
            styles={{ content: { color: '#3f8600' } }}
            loading={isLoading}
          />
        </Card>
      </Col>

      <Col xs={24} sm={12} lg={6}>
        <Card>
          <Statistic
            title="今日登录"
            value={stats.todayLogin}
            prefix={<GlobalOutlined />}
            styles={{ content: { color: '#1890ff' } }}
            loading={isLoading}
          />
        </Card>
      </Col>

      <Col xs={24} sm={12} lg={6}>
        <Card>
          <Statistic
            title="峰值在线"
            value={stats.peakOnline}
            prefix={<ClockCircleOutlined />}
            styles={{ content: { color: '#722ed1' } }}
            loading={isLoading}
          />
        </Card>
      </Col>
    </Row>
  );
};
