'use client';

import React, { useState } from 'react';
import { Modal, Descriptions, Avatar, Tag, Typography } from 'antd';
import { UserOutlined } from '@ant-design/icons';

const { Text } = Typography;

interface SessionDetailModalProps {
  visible: boolean;
  userSession?: API.OnlineUserVO | null;
  onCancel: () => void;
}

export const SessionDetailModal: React.FC<SessionDetailModalProps> = ({
  visible,
  userSession,
  onCancel,
}) => {
  const [observedAt, setObservedAt] = useState(Date.now);

  const formatDuration = (duration: number) => {
    if (!duration) return '-';

    const hours = Math.floor(duration / 3600);
    const minutes = Math.floor((duration % 3600) / 60);
    const seconds = duration % 60;

    if (hours > 0) {
      return `${hours}时${minutes}分${seconds}秒`;
    } else if (minutes > 0) {
      return `${minutes}分${seconds}秒`;
    } else {
      return `${seconds}秒`;
    }
  };

  return (
    <Modal
      title="会话详情"
      open={visible}
      onCancel={onCancel}
      footer={null}
      width={800}
      mask={{ closable: false }}
      afterOpenChange={(open) => {
        if (open) {
          setObservedAt(Date.now());
        }
      }}
    >
      {userSession ? (
        <Descriptions column={2} bordered>
          <Descriptions.Item label="用户信息" span={2}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
              <Avatar
                src={userSession.avatar}
                icon={<UserOutlined />}
                size="large"
                style={{ backgroundColor: '#1890ff' }}
              >
                {(userSession.nickname || userSession.username || '').charAt(0)}
              </Avatar>
              <div>
                <div style={{ fontWeight: 500, fontSize: 16 }}>{userSession.username || '-'}</div>
                {userSession.nickname && (
                  <div style={{ fontSize: 12, color: '#999' }}>{userSession.nickname}</div>
                )}
              </div>
            </div>
          </Descriptions.Item>

          <Descriptions.Item label="用户ID">
            {userSession.userId}
          </Descriptions.Item>

          <Descriptions.Item label="用户角色">
            {userSession.userRole ? (
              <Tag color="blue">{userSession.userRole}</Tag>
            ) : (
              <Tag color="default">无角色</Tag>
            )}
          </Descriptions.Item>

          <Descriptions.Item label="登录IP">
            <Tag color="geekblue">{userSession.loginIp}</Tag>
          </Descriptions.Item>

          <Descriptions.Item label="登录地点">
            {userSession.loginAddress || '-'}
          </Descriptions.Item>

          <Descriptions.Item label="浏览器">
            {userSession.browser || '-'}
          </Descriptions.Item>

          <Descriptions.Item label="操作系统">
            {userSession.os || '-'}
          </Descriptions.Item>

          <Descriptions.Item label="登录时间">
            {userSession.loginTime ? new Date(userSession.loginTime).toLocaleString() : '-'}
          </Descriptions.Item>

          <Descriptions.Item label="最后活动时间">
            {userSession.lastActiveTime ? new Date(userSession.lastActiveTime).toLocaleString() : '-'}
          </Descriptions.Item>

          <Descriptions.Item label="在线时长">
            <Text code>
              {formatDuration(
                userSession.loginTime
                  ? Math.floor((observedAt - Number(userSession.loginTime)) / 1000)
                  : 0,
              )}
            </Text>
          </Descriptions.Item>

          <Descriptions.Item label="会话状态">
            {userSession.isCurrentSession ? (
              <Tag color="blue">当前会话</Tag>
            ) : (
              <Tag color="green">在线</Tag>
            )}
          </Descriptions.Item>

          <Descriptions.Item label="设备信息" span={2}>
            <div style={{ background: '#f5f5f5', padding: 12, borderRadius: 4 }}>
              <div><strong>User Agent:</strong></div>
              <Text code style={{ fontSize: 12 }}>
                {userSession.userAgent || '-'}
              </Text>
            </div>
          </Descriptions.Item>

          <Descriptions.Item label="会话ID" span={2}>
            <Text code>{userSession.tokenId || '-'}</Text>
          </Descriptions.Item>
        </Descriptions>
      ) : (
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
          会话信息不存在
        </div>
      )}
    </Modal>
  );
};
