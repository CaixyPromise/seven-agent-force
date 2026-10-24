'use client';

import React, { useMemo, useRef } from 'react';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { Button, Space, Tag, Typography } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import { getRuntimeLogPage } from '@/api/runtimeLogController';
import type { RuntimeLogLine } from '@/lib/http/types';
import { RuntimeLogStreamCard } from '@/app/system/operation-log/components/RuntimeLogStreamCard';

interface RuntimeLogPanelProps {
  canStream: boolean;
}

function normalizeDateRange(value: unknown) {
  const toDateTimeString = (input: unknown) => {
    if (!input) {
      return undefined;
    }
    if (typeof input === 'string') {
      return input;
    }
    if (dayjs.isDayjs(input)) {
      return input.format('YYYY-MM-DD HH:mm:ss');
    }
    return String(input);
  };
  if (!Array.isArray(value) || value.length !== 2) {
    return { startTime: undefined, endTime: undefined };
  }
  const startTime = toDateTimeString(value[0]);
  const endTime = toDateTimeString(value[1]);
  return { startTime, endTime };
}

function getLevelColor(level?: string) {
  const normalized = (level || '').toUpperCase();
  if (normalized === 'ERROR') {
    return 'red';
  }
  if (normalized === 'WARN') {
    return 'orange';
  }
  if (normalized === 'DEBUG') {
    return 'blue';
  }
  if (normalized === 'TRACE') {
    return 'purple';
  }
  return 'green';
}

export function RuntimeLogPanel({ canStream }: RuntimeLogPanelProps) {
  const actionRef = useRef<ActionType>(undefined);

  const columns = useMemo<ProColumns<RuntimeLogLine>[]>(
    () => [
      {
        title: '时间',
        dataIndex: 'logTime',
        width: 190,
        valueType: 'dateTime',
        search: false,
      },
      {
        title: '级别',
        dataIndex: 'level',
        width: 110,
        valueType: 'select',
        valueEnum: {
          ERROR: { text: 'ERROR', status: 'Error' },
          WARN: { text: 'WARN', status: 'Warning' },
          INFO: { text: 'INFO', status: 'Success' },
          DEBUG: { text: 'DEBUG', status: 'Processing' },
          TRACE: { text: 'TRACE', status: 'Default' },
        },
        render: (_, record) => (
          <Tag color={getLevelColor(record.level)}>{(record.level || 'INFO').toUpperCase()}</Tag>
        ),
      },
      {
        title: '线程',
        dataIndex: 'threadName',
        width: 180,
      },
      {
        title: 'Logger',
        dataIndex: 'loggerName',
        width: 260,
        ellipsis: true,
      },
      {
        title: '日志内容',
        dataIndex: 'message',
        search: false,
        ellipsis: true,
        render: (_, record) => (
          <Typography.Text
            style={{ fontFamily: 'Menlo, Monaco, monospace', whiteSpace: 'pre-wrap' }}
          >
            {record.message || '-'}
          </Typography.Text>
        ),
      },
      {
        title: '文件',
        dataIndex: 'fileName',
        width: 220,
        search: false,
      },
      {
        title: '关键字',
        dataIndex: 'keyword',
        hideInTable: true,
        fieldProps: {
          allowClear: true,
          placeholder: '按日志正文、线程、Logger 检索',
        },
      },
      {
        title: '时间范围',
        dataIndex: 'timeRange',
        valueType: 'dateTimeRange',
        hideInTable: true,
      },
    ],
    [],
  );

  const requestRuntimeLogData = async (params: Record<string, unknown>) => {
    try {
      const { startTime, endTime } = normalizeDateRange(params.timeRange);
      const response = await getRuntimeLogPage({
        current: Number(params.current || 1),
        size: Number(params.pageSize || 20),
        keyword: params.keyword ? String(params.keyword) : undefined,
        level: params.level ? String(params.level) : undefined,
        loggerName: params.loggerName ? String(params.loggerName) : undefined,
        threadName: params.threadName ? String(params.threadName) : undefined,
        startTime,
        endTime,
      });

      if (response?.code !== 0 || !response.data) {
        return {
          success: false,
          data: [],
          total: 0,
        };
      }

      return {
        success: true,
        data: response.data.records || [],
        total: response.data.total || 0,
      };
    } catch (error) {
      console.error('查询运行日志失败:', error);
      return {
        success: false,
        data: [],
        total: 0,
      };
    }
  };

  return (
    <Space direction="vertical" size={16} style={{ width: '100%' }}>
      <ProTable<RuntimeLogLine>
        headerTitle="应用运行日志"
        actionRef={actionRef}
        rowKey="lineId"
        columns={columns}
        request={requestRuntimeLogData}
        search={{
          labelWidth: 110,
          collapsed: false,
          collapseRender: false,
        }}
        dateFormatter="string"
        scroll={{ x: 1400 }}
        toolBarRender={() => [
          <Button
            key="reload-runtime-log"
            icon={<ReloadOutlined />}
            onClick={() => actionRef.current?.reload()}
          >
            刷新
          </Button>,
        ]}
        pagination={{
          defaultPageSize: 20,
          showSizeChanger: true,
          showQuickJumper: true,
          showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
        }}
        options={{
          density: true,
          fullScreen: true,
          reload: true,
          setting: true,
        }}
      />

      <RuntimeLogStreamCard canStream={canStream} />
    </Space>
  );
}
