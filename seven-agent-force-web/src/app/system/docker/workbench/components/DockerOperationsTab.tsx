'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Button, Drawer, Progress, Space, Table, Tag, Timeline, message } from 'antd';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import {
  cancelDockerOperation,
  dockerOperationStreamUrl,
  getDockerOperation,
  getDockerOperationEvents,
  getDockerOperations,
  retryDockerOperation,
  type DockerOperationEventVO,
  type DockerOperationStatus,
  type DockerOperationVO,
} from '@/api/dockerController';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { HasPermission } from '@/components/Permission/HasPermission';
import {
  formatOperationStageLabel,
  formatOperationStatusLabel,
  formatOperationTypeLabel,
} from '../../components/dockerFormat';

const statusColor: Record<DockerOperationStatus, string> = {
  PENDING: 'default',
  RUNNING: 'processing',
  SUCCEEDED: 'success',
  FAILED: 'error',
  CANCELLED: 'warning',
  TIMEOUT: 'error',
};

const longRunningOperationTypes = {
  COMPOSE_UP: { text: '编排启动' },
  COMPOSE_DOWN: { text: '编排停止' },
  COMPOSE_RESTART: { text: '编排重启' },
  IMAGE_PULL: { text: '镜像拉取' },
  IMAGE_PUSH: { text: '镜像推送' },
  IMAGE_EXPORT: { text: '镜像导出' },
  IMAGE_DELETE: { text: '镜像删除' },
  IMAGE_CLEANUP: { text: '镜像清理' },
  REGISTRY_SYNC: { text: '镜像源同步' },
  CONTAINER_CLEANUP: { text: '容器清理' },
  NETWORK_PRUNE: { text: '网络清理' },
  VOLUME_PRUNE: { text: '存储卷清理' },
  DAEMON_RESTART: { text: 'Docker 重启' },
};

function formatTime(value?: string) {
  return value ? new Date(value).toLocaleString() : '-';
}

interface DockerOperationsTabProps {
  refreshToken?: number;
  requestedOperationId?: API.Int64;
  requestedOperationToken?: number;
}

export function DockerOperationsTab({
  refreshToken = 0,
  requestedOperationId,
  requestedOperationToken,
}: DockerOperationsTabProps) {
  const actionRef = useRef<ActionType>(undefined);
  const eventSourceRef = useRef<EventSource | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detail, setDetail] = useState<DockerOperationVO | null>(null);
  const [activeOperationId, setActiveOperationId] = useState<API.Int64>();
  const [events, setEvents] = useState<DockerOperationEventVO[]>([]);
  const [loadingDetail, setLoadingDetail] = useState(false);

  const refresh = () => actionRef.current?.reload();

  const closeStream = useCallback(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = null;
  }, []);

  useEffect(() => () => closeStream(), [closeStream]);

  useEffect(() => {
    if (!refreshToken) {
      return undefined;
    }
    const timer = window.setTimeout(() => {
      actionRef.current?.reload();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [refreshToken]);

  const openDetail = useCallback(async (operationId: API.Int64) => {
    closeStream();
    setDrawerOpen(true);
    setActiveOperationId(operationId);
    setLoadingDetail(true);
    try {
      const [operationResponse, eventResponse] = await Promise.all([
        getDockerOperation(operationId),
        getDockerOperationEvents(operationId, { limit: 200 }),
      ]);
      setDetail(operationResponse.data);
      setEvents(eventResponse.data || []);
      const source = new EventSource(dockerOperationStreamUrl(operationId, eventResponse.data?.at(-1)?.sequence));
      eventSourceRef.current = source;
      source.addEventListener('progress', (event) => {
        setEvents((prev) => [...prev, JSON.parse((event as MessageEvent).data)]);
      });
      source.addEventListener('state', (event) => {
        setEvents((prev) => [...prev, JSON.parse((event as MessageEvent).data)]);
      });
      source.addEventListener('policy', (event) => {
        setEvents((prev) => [...prev, JSON.parse((event as MessageEvent).data)]);
      });
      source.addEventListener('error', (event) => {
        if ((event as MessageEvent).data) {
          setEvents((prev) => [...prev, JSON.parse((event as MessageEvent).data)]);
        }
      });
      source.addEventListener('result', (event) => {
        setEvents((prev) => [...prev, JSON.parse((event as MessageEvent).data)]);
      });
      source.addEventListener('done', (event) => {
        setDetail(JSON.parse((event as MessageEvent).data));
        source.close();
        eventSourceRef.current = null;
        refresh();
      });
    } catch (error) {
      message.error((error as Error).message || '获取 Docker 操作详情失败');
    } finally {
      setLoadingDetail(false);
    }
  }, [closeStream]);

  useEffect(() => {
    if (!requestedOperationId) {
      return;
    }
    void openDetail(requestedOperationId);
  }, [openDetail, requestedOperationId, requestedOperationToken]);

  const columns = useMemo<ProColumns<DockerOperationVO>[]>(() => [
    { title: '操作 ID', dataIndex: 'operationId', width: 190, search: false },
    {
      title: '类型',
      dataIndex: 'operationType',
      width: 180,
      valueType: 'select',
      valueEnum: longRunningOperationTypes,
      render: (_, record) => <Tag>{formatOperationTypeLabel(record.operationType)}</Tag>,
    },
    {
      title: '目标',
      dataIndex: 'targetName',
      width: 220,
      ellipsis: true,
      search: false,
      render: (_, record) => (
        <span className="block max-w-[200px] truncate" title={record.targetName || record.targetId || ''}>
          {record.targetName || record.targetId || '-'}
        </span>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      width: 130,
      valueType: 'select',
      valueEnum: {
        PENDING: { text: '等待中' },
        RUNNING: { text: '执行中' },
        SUCCEEDED: { text: '成功' },
        FAILED: { text: '失败' },
        CANCELLED: { text: '已取消' },
        TIMEOUT: { text: '超时' },
      },
      render: (_, record) => (
        <Tag color={statusColor[record.status]}>{formatOperationStatusLabel(record.status)}</Tag>
      ),
    },
    {
      title: '进度',
      dataIndex: 'progress',
      search: false,
      width: 220,
      render: (_, record) => <Progress percent={record.progress || 0} size="small" />,
    },
    {
      title: '阶段',
      dataIndex: 'currentStage',
      width: 170,
      ellipsis: true,
      search: false,
      render: (_, record) => (
        <span className="block max-w-[150px] truncate" title={formatOperationStageLabel(record.currentStage)}>
          {formatOperationStageLabel(record.currentStage)}
        </span>
      ),
    },
    { title: '创建时间', dataIndex: 'createTime', search: false, width: 210, renderText: formatTime },
    {
      title: '操作',
      valueType: 'option',
      width: 160,
      fixed: 'right',
      render: (_, record) => (
        <Space>
          <Button type="link" size="small" onClick={() => void openDetail(record.operationId)}>
            详情
          </Button>
          <HasPermission code={DOCKER_PERMISSIONS.OPERATION_CANCEL}>
            {['PENDING', 'RUNNING'].includes(record.status) ? (
              <Button
                type="link"
                size="small"
                danger
                onClick={async () => {
                  await cancelDockerOperation(record.operationId);
                  message.success('取消请求已提交');
                  refresh();
                }}
              >
                取消
              </Button>
            ) : null}
          </HasPermission>
          <HasPermission code={DOCKER_PERMISSIONS.OPERATION_RETRY}>
            {['FAILED', 'CANCELLED', 'TIMEOUT'].includes(record.status) ? (
              <Button
                type="link"
                size="small"
                onClick={async () => {
                  const response = await retryDockerOperation(record.operationId);
                  message.success(`重试操作已提交 #${response.data.operationId}`);
                  refresh();
                }}
              >
                重试
              </Button>
            ) : null}
          </HasPermission>
        </Space>
      ),
    },
  ], [openDetail]);

  return (
    <div className="space-y-4">
      <ProTable<DockerOperationVO>
        headerTitle="Docker 长任务与历史操作"
        rowKey="operationId"
        actionRef={actionRef}
        columns={columns}
        scroll={{ x: 1450 }}
        request={async (params) => {
          const response = await getDockerOperations({
            current: params.current,
            size: params.pageSize,
            status: params.status as DockerOperationStatus | undefined,
            operationType: params.operationType as string | undefined,
          });
          return {
            data: response.data.records,
            success: true,
            total: response.data.total,
          };
        }}
      />

      <Drawer
        title={
          activeOperationId || detail?.operationId
            ? `Docker 操作 #${activeOperationId || detail?.operationId}`
            : 'Docker 操作详情'
        }
        open={drawerOpen}
        size={720}
        destroyOnHidden
        onClose={() => {
          closeStream();
          setDrawerOpen(false);
          setActiveOperationId(undefined);
          setDetail(null);
          setEvents([]);
        }}
      >
        {detail ? (
          <div className="space-y-5">
            <Progress percent={detail.progress || 0} status={detail.status === 'FAILED' ? 'exception' : undefined} />
            <Table
              size="small"
              pagination={false}
              columns={[
                { title: '字段', dataIndex: 'key', width: 160 },
                { title: '值', dataIndex: 'value' },
              ]}
              dataSource={[
                { key: '状态', value: formatOperationStatusLabel(detail.status) },
                { key: '类型', value: formatOperationTypeLabel(detail.operationType) },
                { key: '目标', value: detail.targetName || '-' },
                { key: '阶段', value: formatOperationStageLabel(detail.currentStage) },
                { key: '错误', value: detail.errorSummary || '-' },
                { key: '创建时间', value: formatTime(detail.createTime) },
                { key: '开始时间', value: formatTime(detail.startedAt) },
                { key: '结束时间', value: formatTime(detail.finishedAt) },
              ]}
            />
            <Timeline
              pending={loadingDetail ? '加载中...' : false}
              items={events.map((event) => ({
                color: event.type === 'ERROR' ? 'red' : event.type === 'POLICY' ? 'orange' : 'blue',
                content: (
                  <div>
                    <div className="font-medium">{event.stage || event.type}</div>
                    <div className="text-slate-600 whitespace-pre-wrap">{event.message || '-'}</div>
                    <div className="text-xs text-slate-400">{formatTime(event.occurredAt)}</div>
                  </div>
                ),
              }))}
            />
          </div>
        ) : null}
      </Drawer>
    </div>
  );
}

export default DockerOperationsTab;
