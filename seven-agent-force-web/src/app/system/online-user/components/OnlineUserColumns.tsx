'use client';

import React from 'react';
import { Button, Tag, Space, Avatar, Tooltip } from 'antd';
import { EyeOutlined, UserDeleteOutlined, UserOutlined, MobileOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';

interface OnlineUserColumnsProps {
  handleForceLogout: (record: API.OnlineUserVO) => void;
  handleViewSession: (record: API.OnlineUserVO) => void;
  handleManageDevices: (record: API.OnlineUserVO) => void;
  canViewOnline: boolean;
  canKickOnline: boolean;
}

function toText(value: React.ReactNode) {
  if (typeof value === 'string' || typeof value === 'number') {
    return String(value);
  }
  return '';
}

function formatTimestamp(value: React.ReactNode) {
  const text = toText(value);
  if (!text) {
    return '-';
  }
  const timestamp = Number(text);
  if (Number.isNaN(timestamp)) {
    return '-';
  }
  const date = new Date(timestamp);
  return `${date.getMonth() + 1}/${date.getDate()} ${date.getHours()}:${date
    .getMinutes()
    .toString()
    .padStart(2, '0')}`;
}

export function useOnlineUserColumns({
  handleForceLogout,
  handleViewSession,
  handleManageDevices,
  canViewOnline,
  canKickOnline,
}: OnlineUserColumnsProps): ProColumns<API.OnlineUserVO>[] {
  return [
  {
    title: '用户信息',
    dataIndex: 'username',
    key: 'username',
    width: 140,
    render: (dom, record: API.OnlineUserVO) => (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <Avatar
          src={record.avatar}
          icon={<UserOutlined />}
          size="small"
          style={{ backgroundColor: '#1890ff' }}
        >
          {toText(dom)?.charAt(0)}
        </Avatar>
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 500, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {toText(dom)}
          </div>
          {record.nickname && (
            <div style={{ fontSize: 12, color: '#999', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {record.nickname}
            </div>
          )}
        </div>
      </div>
    ),
  },
  {
    title: '登录IP',
    dataIndex: 'loginIp',
    key: 'loginIp',
    width: 100,
    render: (dom) => (
      <Tag color="geekblue">{toText(dom) || '-'}</Tag>
    ),
  },
  {
    title: '登录地点',
    dataIndex: 'loginAddress',
    key: 'loginAddress',
    width: 100,
    ellipsis: true,
    search: false,
    render: (dom) => (
      <Tooltip title={toText(dom)}>
        <span>{toText(dom) || '-'}</span>
      </Tooltip>
    ),
  },
  {
    title: '浏览器',
    dataIndex: 'browser',
    key: 'browser',
    width: 80,
    ellipsis: true,
    search: false,
    render: (dom) => (
      <Tooltip title={toText(dom)}>
        <span>{toText(dom) || '-'}</span>
      </Tooltip>
    ),
  },
  {
    title: '操作系统',
    dataIndex: 'os',
    key: 'os',
    width: 80,
    ellipsis: true,
    search: false,
    render: (dom) => (
      <Tooltip title={toText(dom)}>
        <span>{toText(dom) || '-'}</span>
      </Tooltip>
    ),
  },
  {
    title: '登录时间',
    dataIndex: 'loginTime',
    key: 'loginTime',
    search: false,
    width: 160,
    render: (dom) => formatTimestamp(dom),
  },
  {
    title: '最后活动',
    dataIndex: 'lastActiveTime',
    key: 'lastActiveTime',
    search: false,
    width: 160,
    render: (dom) => formatTimestamp(dom),
  },
  {
    title: '在线时长',
    dataIndex: 'loginTime',
    key: 'onlineDuration',
    width: 100,
    search: false,
    render: (dom) => {
      const loginTime = toText(dom);
      if (!loginTime) {
        return '-';
      }
      const loginTimestamp = Number(loginTime);
      if (Number.isNaN(loginTimestamp)) {
        return '-';
      }
      const now = Date.now();
      const duration = Math.floor((now - loginTimestamp) / 1000);
      const hours = Math.floor(duration / 3600);
      const minutes = Math.floor((duration % 3600) / 60);
      if (hours > 0) {
        return `${hours}时${minutes}分`;
      }
      return `${minutes}分`;
    },
  },
  {
    title: '状态',
    dataIndex: 'isCurrentSession',
    key: 'status',
    width: 80,
    search: false,
    render: (dom) => {
      const isCurrentSession = Boolean(dom);
      if (isCurrentSession) {
        return <Tag color="blue">当前会话</Tag>;
      }
      return <Tag color="green">在线</Tag>;
    },
  },
  {
    title: '操作',
    key: 'action',
    fixed: 'right',
    width: 180,
    search: false,
    render: (_, record) => (
      <Space size="small">
        {canViewOnline ? (
          <Button
            type="link"
            size="small"
            icon={<MobileOutlined />}
            onClick={() => handleManageDevices(record)}
          >
            设备
          </Button>
        ) : null}
        {canViewOnline ? (
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => handleViewSession(record)}
          >
            详情
          </Button>
        ) : null}
        {canKickOnline && !record.isCurrentSession && (
          <Button
            type="link"
            size="small"
            danger
            icon={<UserDeleteOutlined />}
            onClick={() => handleForceLogout(record)}
          >
            下线
          </Button>
        )}
        {!canViewOnline && !canKickOnline ? <span className="text-slate-400">-</span> : null}
      </Space>
    ),
  },
  ];
}
