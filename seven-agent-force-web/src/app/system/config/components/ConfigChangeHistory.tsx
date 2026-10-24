'use client';

import React, { useCallback, useEffect, useState } from 'react';
import { Table, Tag, message, Button, Input, Modal } from 'antd';
import {RollbackOutlined, ReloadOutlined, BranchesOutlined} from '@ant-design/icons';
import {
  getConfigChangeHistory,
  rollbackConfigChange,
  getOperationChain
} from '@/api/configController';
import type { ConfigChangeLog } from '@/types/config';
import { formatDate } from '@/utils/date';
import { usePermissionAccess } from '@/hooks/auth';
import { CONFIG_PERMISSIONS } from '@/lib/auth/permissionCodes';

interface ConfigChangeHistoryProps {
  configId: API.Int64;
  configKey?: string;
  onClose?: () => void;
}

function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

/**
 * 配置变更历史组件
 */
export const ConfigChangeHistory: React.FC<ConfigChangeHistoryProps> = ({
  configId,
  configKey,
  onClose,
}) => {
  const canRollbackConfig = usePermissionAccess(CONFIG_PERMISSIONS.ROLLBACK);
  const [history, setHistory] = useState<ConfigChangeLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [rollbackReason, setRollbackReason] = useState('');
  const [rollbackModalVisible, setRollbackModalVisible] = useState(false);
  const [selectedLogId, setSelectedLogId] = useState<API.Int64 | null>(null);
  const [operationChain, setOperationChain] = useState<ConfigChangeLog[]>([]);
  const [chainModalVisible, setChainModalVisible] = useState(false);
  const [chainLoading, setChainLoading] = useState(false);

  const fetchHistory = useCallback(async () => {
    setLoading(true);
    try {
      const res = await getConfigChangeHistory(configId, 50);
      setHistory(res || []);
    } catch (error) {
      message.error(getErrorMessage(error, '获取变更历史失败'));
    } finally {
      setLoading(false);
    }
  }, [configId]);

  useEffect(() => {
    if (configId) {
      fetchHistory();
    }
  }, [configId, fetchHistory]);

  const handleRollback = async () => {
    if (!selectedLogId) return;

    try {
      await rollbackConfigChange(selectedLogId, rollbackReason);
      message.success('回滚成功');
      setRollbackModalVisible(false);
      setRollbackReason('');
      setSelectedLogId(null);
      await fetchHistory();
      onClose?.();
    } catch (error) {
      message.error(getErrorMessage(error, '回滚失败'));
    }
  };

  const openRollbackModal = (logId: API.Int64) => {
    setSelectedLogId(logId);
    setRollbackModalVisible(true);
  };

  const handleViewOperationChain = async (logId: API.Int64) => {
    setChainLoading(true);
    setChainModalVisible(true);
    try {
      const chain = await getOperationChain(logId);
      setOperationChain(chain || []);
    } catch (error) {
      message.error(getErrorMessage(error, '获取操作链失败'));
    } finally {
      setChainLoading(false);
    }
  };

  const getOperationTypeTag = (operationType: string) => {
    const typeMap: Record<string, { color: string; text: string }> = {
      CREATE: { color: 'green', text: '创建' },
      UPDATE: { color: 'blue', text: '更新' },
      DELETE: { color: 'red', text: '删除' },
      APPLY: { color: 'cyan', text: '应用' },
      ROLLBACK: { color: 'orange', text: '回滚' },
    };
    const type = typeMap[operationType] || { color: 'default', text: operationType };
    return <Tag color={type.color}>{type.text}</Tag>;
  };

  const getStatusTag = (status: string) => {
    switch (status) {
      case 'pending':
        return <Tag color="orange">待生效</Tag>;
      case 'applied':
        return <Tag color="green">已生效</Tag>;
      case 'rolled_back':
        return <Tag color="red">已回滚</Tag>;
      default:
        return <Tag>{status}</Tag>;
    }
  };

  const getEffectTypeTag = (effectType: string) => {
    return effectType === 'realtime' ? (
      <Tag color="green">即时生效</Tag>
    ) : (
      <Tag color="orange">重启生效</Tag>
    );
  };

  const columns = [
    {
      title: '操作类型',
      dataIndex: 'operationType',
      key: 'operationType',
      width: 100,
      fixed: 'left' as const,
      render: (operationType: string) => getOperationTypeTag(operationType),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status: string) => getStatusTag(status),
    },
    {
      title: '生效方式',
      dataIndex: 'effectType',
      key: 'effectType',
      width: 100,
      render: (effectType: string) => getEffectTypeTag(effectType),
    },
    {
      title: '旧值',
      dataIndex: 'oldValue',
      key: 'oldValue',
      width: 200,
      ellipsis: true,
      render: (text: string) => (
        <span className="text-gray-600">{text || '-'}</span>
      ),
    },
    {
      title: '新值',
      dataIndex: 'newValue',
      key: 'newValue',
      width: 200,
      ellipsis: true,
      render: (text: string) => (
        <span className="text-blue-600">{text}</span>
      ),
    },
    {
      title: '操作人',
      dataIndex: 'operatorName',
      key: 'operatorName',
      width: 120,
      render: (text: string) => text || '-',
    },
    {
      title: '操作时间',
      dataIndex: 'operationTime',
      key: 'operationTime',
      width: 180,
      render: (text: string) => formatDate(text),
    },
    {
      title: '操作原因',
      dataIndex: 'operationReason',
      key: 'operationReason',
      width: 200,
      ellipsis: true,
      render: (text: string) => text || '-',
    },
    {
      title: '应用时间',
      dataIndex: 'appliedTime',
      key: 'appliedTime',
      width: 180,
      render: (text: string) => text ? formatDate(text) : '-',
    },
    {
      title: '操作',
      key: 'action',
      width: 150,
      fixed: 'right' as const,
      render: (_: unknown, record: ConfigChangeLog) => (
        <div>
          {(record.parentLogId || record.relatedLogId) && (
            <Button
              type="link"
              size="small"
              icon={<BranchesOutlined />}
              onClick={() => handleViewOperationChain(record.id)}
            >
              操作链
            </Button>
          )}
          <Button
            type="link"
            size="small"
            icon={<RollbackOutlined />}
            disabled={!canRollbackConfig || record.status !== 'applied' || record.operationType !== 'UPDATE'}
            onClick={() => openRollbackModal(record.id)}
          >
            回滚
          </Button>
        </div>
      ),
    },
  ];

  return (
    <div className="p-4">
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2">
          <BranchesOutlined className="text-blue-500" />
          <h2 className="text-lg font-semibold">
            配置变更历史
            {configKey && (
              <span className="text-sm text-gray-500 ml-2">
                ({configKey})
              </span>
            )}
          </h2>
        </div>
        <Button
          icon={<ReloadOutlined />}
          onClick={fetchHistory}
          loading={loading}
        >
          刷新
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={history}
        loading={loading}
        rowKey="id"
        pagination={{ pageSize: 20 }}
        locale={{ emptyText: '暂无变更历史' }}
        scroll={{ x: 1500 }}
      />

      <Modal
        title="回滚配置变更"
        open={rollbackModalVisible}
        onOk={handleRollback}
        onCancel={() => {
          setRollbackModalVisible(false);
          setRollbackReason('');
          setSelectedLogId(null);
        }}
        okText="确定回滚"
        cancelText="取消"
      >
        <div className="py-4">
          <p className="mb-2">确定要回滚此配置变更吗？回滚后将恢复到变更前的值。</p>
          <Input.TextArea
            placeholder="请输入回滚原因（可选）"
            value={rollbackReason}
            onChange={(e) => setRollbackReason(e.target.value)}
            rows={3}
          />
        </div>
      </Modal>

      <Modal
        title="操作链"
        open={chainModalVisible}
        onCancel={() => {
          setChainModalVisible(false);
          setOperationChain([]);
        }}
        footer={null}
        width={1000}
      >
        <Table
          columns={columns.filter(col => col.key !== 'action').map(col => ({
            ...col,
            fixed: col.key === 'operationType' ? 'left' as const : undefined,
          }))}
          dataSource={operationChain}
          loading={chainLoading}
          rowKey="id"
          pagination={false}
          locale={{ emptyText: '暂无操作链数据' }}
          scroll={{ x: 1300 }}
        />
      </Modal>
    </div>
  );
};
