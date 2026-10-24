'use client';

import React from 'react';
import { Modal, Descriptions, Tag, Typography } from 'antd';
import { useQuery } from '@tanstack/react-query';
import { getOperationLogById } from '@/api/operationLogController';

const { Text } = Typography;

interface OperationLogDetailModalProps {
  visible: boolean;
  operationLog?: API.OperationLogVO | null;
  onCancel: () => void;
}

type CompatibleOperationLog = API.OperationLogVO & {
  method?: string;
  url?: string;
  ip?: string;
  description?: string;
  duration?: number;
  requestParams?: string;
  requestParam?: string;
  params?: string;
  responseResult?: string;
  result?: string;
  errorMessage?: string;
};

export const OperationLogDetailModal: React.FC<OperationLogDetailModalProps> = ({
  visible,
  operationLog,
  onCancel,
}) => {
  // 获取操作日志详情
  const { data: logDetail, isLoading } = useQuery({
    queryKey: ['operationLogDetail', operationLog?.id],
    queryFn: () => getOperationLogById({ id: operationLog?.id ?? '' }),
    enabled: visible && !!operationLog?.id,
  });

  const log = (logDetail?.data || operationLog) as CompatibleOperationLog | null | undefined;

  const getMethodColor = (method: string) => {
    switch (method?.toUpperCase()) {
      case 'GET': return 'blue';
      case 'POST': return 'green';
      case 'PUT': return 'orange';
      case 'DELETE': return 'red';
      case 'PATCH': return 'purple';
      default: return 'default';
    }
  };

  const getStatusColor = (status: number) => {
    return status === 1 ? 'green' : 'red';
  };

  const getDurationColor = (ms: number) => {
    if (ms < 100) return 'green';
    if (ms < 500) return 'blue';
    if (ms < 1000) return 'orange';
    return 'red';
  };

  const method = log?.requestMethod || log?.method || '';
  const url = log?.requestUrl || log?.url || '';
  const ip = log?.requestIp || log?.ip || '';
  const desc = log?.operationDesc || log?.description || '';
  const duration = Number(log?.executionTime ?? log?.duration ?? 0);
  const operationTime = log?.operationTime || log?.createTime || '';
  const requestParams =
    log?.requestParams ||
    log?.requestParam ||
    log?.params ||
    '';
  const responseResult =
    log?.responseResult ||
    log?.result ||
    '';

  return (
    <Modal
      title="操作日志详情"
      open={visible}
      onCancel={onCancel}
      footer={null}
      width={800}
      mask={{ closable: false }}
    >
      {isLoading ? (
        <div style={{ textAlign: 'center', padding: '40px 0' }}>
          加载中...
        </div>
      ) : log ? (
        <Descriptions column={2} bordered>
          <Descriptions.Item label="日志ID">
            {log.id}
          </Descriptions.Item>

          <Descriptions.Item label="操作用户">
            <div>
              <div style={{ fontWeight: 500 }}>{log.userName}</div>
              {log.userId && (
                <div style={{ fontSize: 12, color: '#999' }}>ID: {log.userId}</div>
              )}
            </div>
          </Descriptions.Item>

          <Descriptions.Item label="操作类型">
            <Tag color="blue">{log.operationTypeLabel || log.operationTypeDesc || log.operationType || '-'}</Tag>
          </Descriptions.Item>

          <Descriptions.Item label="操作描述">
            {desc || '-'}
          </Descriptions.Item>

          <Descriptions.Item label="请求方法">
            <Tag color={getMethodColor(method)}>
              {method || '-'}
            </Tag>
          </Descriptions.Item>

          <Descriptions.Item label="请求URL">
            <Text code>{url || '-'}</Text>
          </Descriptions.Item>

          <Descriptions.Item label="IP地址">
            <Tag color="geekblue">{ip || '-'}</Tag>
          </Descriptions.Item>

          <Descriptions.Item label="浏览器">
            {log.browser}
          </Descriptions.Item>

          <Descriptions.Item label="操作系统">
            {log.os}
          </Descriptions.Item>

          <Descriptions.Item label="执行时间">
            <Tag color={getDurationColor(duration)}>
              {duration}ms
            </Tag>
          </Descriptions.Item>

          <Descriptions.Item label="状态">
            <Tag color={getStatusColor(log.status || 0)}>
              {log.status === 1 ? '成功' : '失败'}
            </Tag>
          </Descriptions.Item>

          <Descriptions.Item label="操作时间">
            {operationTime || '-'}
          </Descriptions.Item>

          <Descriptions.Item label="请求参数" span={2}>
            <pre style={{
              background: '#f5f5f5',
              padding: 12,
              borderRadius: 4,
              maxHeight: 200,
              overflow: 'auto',
              fontSize: 12
            }}>
              {requestParams || '无'}
            </pre>
          </Descriptions.Item>

          <Descriptions.Item label="返回结果" span={2}>
            <pre style={{
              background: '#f5f5f5',
              padding: 12,
              borderRadius: 4,
              maxHeight: 200,
              overflow: 'auto',
              fontSize: 12
            }}>
              {responseResult || '无'}
            </pre>
          </Descriptions.Item>

          <Descriptions.Item label="错误信息" span={2}>
            {log.errorMsg || log.errorMessage ? (
              <Text type="danger">{log.errorMsg || log.errorMessage}</Text>
            ) : (
              '无'
            )}
          </Descriptions.Item>
        </Descriptions>
      ) : (
        <div style={{ textAlign: 'center', padding: '40px 0', color: '#999' }}>
          日志信息不存在
        </div>
      )}
    </Modal>
  );
};
