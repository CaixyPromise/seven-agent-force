'use client';

import React from 'react';
import { Button, Tag, Space, Tooltip } from 'antd';
import { EyeOutlined } from '@ant-design/icons';
import type { ProColumns } from '@ant-design/pro-components';

interface OperationLogColumnsProps {
  handleViewDetail: (record: API.OperationLogVO) => void;
  operationTypeOptions: API.OperationTypeOption[];
  canViewDetail: boolean;
}

export const OperationLogColumns = ({
  handleViewDetail,
  operationTypeOptions,
  canViewDetail,
}: OperationLogColumnsProps): ProColumns<API.OperationLogVO>[] => {
  const operationTypeValueEnum = Object.fromEntries(
    operationTypeOptions.map((option) => [option.value, { text: option.label }]),
  );

  const getTypeColor = (type: string) => {
    if (type.includes('CREATE')) return 'green';
    if (type.includes('UPDATE')) return 'blue';
    if (type.includes('DELETE')) return 'red';
    if (type.includes('LOGIN')) return 'cyan';
    if (type.includes('LOGOUT')) return 'orange';
    return 'default';
  };

  return [
  {
    title: 'ID',
    dataIndex: 'id',
    key: 'id',
    width: 80,
    search: false,
  },
  {
    title: '操作用户',
    dataIndex: 'userName',
    key: 'userName',
    width: 120,
    render: (_, record) => (
      <div>
        <div style={{ fontWeight: 500 }}>{record.userName}</div>
        {record.userId && (
          <div style={{ fontSize: 12, color: '#999' }}>ID: {record.userId}</div>
        )}
      </div>
    ),
  },
  {
    title: '操作类型',
    dataIndex: 'operationType',
    key: 'operationType',
    width: 120,
    valueEnum: operationTypeValueEnum,
    render: (_, record) => {
      const type = record.operationType || '';
      const label = record.operationTypeLabel || record.operationTypeDesc || operationTypeValueEnum[type]?.text || type || '未分类操作';
      return (
        <Tag color={getTypeColor(type)}>{label}</Tag>
      );
    },
  },
  {
    title: '操作描述',
    dataIndex: 'operationDesc',
    key: 'operationDesc',
    ellipsis: true,
    width: 200,
    render: (_, record) => {
      const desc = record.operationDesc;
      return (
        <Tooltip title={desc}>
          <span>{desc || '-'}</span>
        </Tooltip>
      );
    },
  },
  {
    title: '请求方法',
    dataIndex: 'requestMethod',
    key: 'requestMethod',
    width: 100,
    search: false,
    render: (_, record) => {
      const method = record.requestMethod;
      const getMethodColor = (m?: string) => {
        switch (m?.toUpperCase()) {
          case 'GET': return 'blue';
          case 'POST': return 'green';
          case 'PUT': return 'orange';
          case 'DELETE': return 'red';
          case 'PATCH': return 'purple';
          default: return 'default';
        }
      };
      return (
        <Tag color={getMethodColor(method)}>
          {method || '-'}
        </Tag>
      );
    },
  },
  {
    title: '请求URL',
    dataIndex: 'requestUrl',
    key: 'requestUrl',
    ellipsis: true,
    width: 200,
    search: false,
    render: (_, record) => {
      const url = record.requestUrl;
      return (
        <Tooltip title={url}>
          <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{url || '-'}</span>
        </Tooltip>
      );
    },
  },
  {
    title: 'IP地址',
    dataIndex: 'requestIp',
    key: 'requestIp',
    width: 120,
    render: (_, record) => {
      const ip = record.requestIp;
      return <Tag color="geekblue">{ip || '-'}</Tag>;
    },
  },
  {
    title: '浏览器',
    dataIndex: 'browser',
    key: 'browser',
    width: 120,
    ellipsis: true,
    search: false,
    render: (_, record) => (
      <Tooltip title={record.browser}>
        <span>{record.browser || '-'}</span>
      </Tooltip>
    ),
  },
  {
    title: '操作系统',
    dataIndex: 'os',
    key: 'os',
    width: 120,
    ellipsis: true,
    search: false,
    render: (_, record) => (
      <Tooltip title={record.os}>
        <span>{record.os || '-'}</span>
      </Tooltip>
    ),
  },
  {
    title: '执行时间',
    dataIndex: 'executionTime',
    key: 'executionTime',
    width: 100,
    search: false,
    render: (_, record) => {
      const duration = Number(record.executionTime ?? 0);
      const getDurationColor = (ms: number) => {
        if (ms < 100) return 'green';
        if (ms < 500) return 'blue';
        if (ms < 1000) return 'orange';
        return 'red';
      };
      return (
        <Tag color={getDurationColor(duration)}>
          {duration}ms
        </Tag>
      );
    },
  },
  {
    title: '状态',
    dataIndex: 'status',
    key: 'status',
    width: 80,
    search: false,
    render: (_, record) => (
      <Tag color={record.status === 1 ? 'green' : 'red'}>
        {record.status === 1 ? '成功' : '失败'}
      </Tag>
    ),
  },
  {
    title: '操作时间',
    dataIndex: 'operationTime',
    key: 'operationTime',
    valueType: 'dateTime',
    search: false,
    width: 180,
    render: (_, record) => record.operationTime || record.createTime || '-',
  },
  {
    title: '操作',
    key: 'action',
    fixed: 'right',
    width: 100,
    search: false,
    render: (_, record) => (
      <Space size="small">
        {canViewDetail ? (
          <Button
            type="link"
            size="small"
            icon={<EyeOutlined />}
            onClick={() => handleViewDetail(record)}
          >
            详情
          </Button>
        ) : (
          <span className="text-slate-400">-</span>
        )}
      </Space>
    ),
  },
  ];
};
