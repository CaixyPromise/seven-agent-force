'use client';

import React, { useMemo, useRef, useState } from 'react';
import { ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { useQuery } from '@tanstack/react-query';
import { Alert, Button, Dropdown, Empty, message, Space, Tag, Tabs, Tooltip } from 'antd';
import {
  ReloadOutlined,
  ClearOutlined,
  DownOutlined,
  FileSearchOutlined,
  UserOutlined,
  SettingOutlined,
  SafetyOutlined,
  AuditOutlined,
} from '@ant-design/icons';
import { getOperationLogs, getMyOperationLogPage, getOperationTypes } from '@/api/operationLogController';
import { getAuditLogs } from '@/api/configController';
import { getObservabilityOverview, type ObservabilityAlert } from '@/api/observabilityController';
import { OperationLogColumns } from './components/OperationLogColumns';
import { OperationLogDetailModal } from './components/OperationLogDetailModal';
import { ExportLogModal } from './components/ExportLogModal';
import { LogCleanModal } from './components/LogCleanModal';
import { usePermissionFlags } from '@/hooks/auth';
import { ADMIN_PERMISSIONS } from '@/lib/auth/permissionCodes';
import type { ConfigChangeLog } from '@/types/config';

type LogCenterTabKey = 'operation' | 'configAudit' | 'securityAudit';

interface SecurityAlertRow extends Omit<ObservabilityAlert, 'id'> {
  id: string;
  platformKey: string;
  platformName: string;
}

function normalizeDateRange(value: unknown) {
  if (Array.isArray(value) && value.length === 2) {
    const startTime = value[0] ? String(value[0]) : undefined;
    const endTime = value[1] ? String(value[1]) : undefined;
    return { startTime, endTime };
  }
  return { startTime: undefined, endTime: undefined };
}

function filterByKeyword(values: string[], keyword?: string) {
  const normalizedKeyword = keyword?.trim().toLowerCase();
  if (!normalizedKeyword) {
    return true;
  }
  return values.some((value) => value.toLowerCase().includes(normalizedKeyword));
}

function sortByOperationTimeDesc(a?: string, b?: string) {
  const aTime = a ? new Date(a).getTime() : 0;
  const bTime = b ? new Date(b).getTime() : 0;
  return bTime - aTime;
}

function securitySeverityLabel(severity?: string) {
  switch ((severity || '').toUpperCase()) {
    case 'HIGH':
      return '高';
    case 'MEDIUM':
      return '中';
    default:
      return '低';
  }
}

export default function OperationLogPage() {
  const actionRef = useRef<ActionType>(undefined);
  const configAuditActionRef = useRef<ActionType>(undefined);
  const securityAuditActionRef = useRef<ActionType>(undefined);
  const [activeCenterTab, setActiveCenterTab] = useState<LogCenterTabKey>('operation');
  const [activeTab, setActiveTab] = useState<string>('all');
  const {
    canViewLogDetail,
    canExportLogs,
    canCleanLogs,
    canViewConfigAudit,
    canViewSecurityAudit,
  } =
    usePermissionFlags({
    canViewLogDetail: ADMIN_PERMISSIONS.LOG_VIEW,
    canExportLogs: ADMIN_PERMISSIONS.LOG_EXPORT,
    canCleanLogs: ADMIN_PERMISSIONS.LOG_CLEAN,
    canViewConfigAudit: 'system:config:query',
    canViewSecurityAudit: ADMIN_PERMISSIONS.OBSERVABILITY_VIEW,
  });

  // 详情模态框状态
  const [detailVisible, setDetailVisible] = useState(false);
  const [currentLogDetail, setCurrentLogDetail] = useState<API.OperationLogVO | null>(null);

  // 导出模态框状态
  const [exportVisible, setExportVisible] = useState(false);

  // 清理模态框状态
  const [cleanVisible, setCleanVisible] = useState(false);

  // 查看详情
  const handleViewDetail = (record: API.OperationLogVO) => {
    setCurrentLogDetail(record);
    setDetailVisible(true);
  };

  // 刷新数据
  const handleRefresh = () => {
    actionRef.current?.reload();
    message.success('数据已刷新');
  };

  // 处理清理成功
  const handleCleanSuccess = () => {
    actionRef.current?.reload();
  };

  const operationTypeOptionsQuery = useQuery({
    queryKey: ['operation-log-types'],
    queryFn: () => getOperationTypes(),
    enabled: canViewLogDetail,
    staleTime: 5 * 60 * 1000,
  });
  const operationTypeOptions = useMemo<API.OperationTypeOption[]>(
    () => operationTypeOptionsQuery.data?.data ?? [],
    [operationTypeOptionsQuery.data?.data],
  );

  const columns = OperationLogColumns({
    handleViewDetail,
    operationTypeOptions,
    canViewDetail: canViewLogDetail,
  });

  // 管理员操作下拉菜单
  const adminDropdownItems = [
    ...(canCleanLogs
      ? [
          {
            key: 'clean',
            label: '清理日志',
            icon: <ClearOutlined />,
            danger: true,
          },
        ]
      : []),
  ];

  // 处理下拉菜单点击
  const handleMenuClick = ({ key }: { key: string }) => {
    switch (key) {
      case 'export':
        setExportVisible(true);
        break;
      case 'clean':
        setCleanVisible(true);
        break;
    }
  };

  const configAuditColumns = useMemo<ProColumns<ConfigChangeLog>[]>(
    () => [
      {
        title: '日志ID',
        dataIndex: 'id',
        width: 90,
        search: false,
      },
      {
        title: '配置键',
        dataIndex: 'configKey',
        width: 220,
        copyable: true,
        ellipsis: true,
      },
      {
        title: '操作类型',
        dataIndex: 'operationType',
        width: 120,
        valueType: 'select',
        valueEnum: {
          CREATE: { text: '创建', status: 'Success' },
          UPDATE: { text: '更新', status: 'Processing' },
          DELETE: { text: '删除', status: 'Error' },
          APPLY: { text: '应用', status: 'Default' },
          ROLLBACK: { text: '回滚', status: 'Warning' },
        },
      },
      {
        title: '状态',
        dataIndex: 'status',
        width: 130,
        valueType: 'select',
        valueEnum: {
          PENDING: { text: '待生效', status: 'Processing' },
          APPLIED: { text: '已生效', status: 'Success' },
          ROLLED_BACK: { text: '已回滚', status: 'Warning' },
        },
      },
      {
        title: '操作人',
        dataIndex: 'operatorName',
        width: 140,
      },
      {
        title: '操作说明',
        dataIndex: 'operationReason',
        search: false,
        ellipsis: true,
        render: (_, record) => (
          <Tooltip title={record.operationReason}>
            <span>{record.operationReason || '-'}</span>
          </Tooltip>
        ),
      },
      {
        title: '操作时间',
        dataIndex: 'operationTime',
        valueType: 'dateTime',
        width: 190,
        search: false,
      },
      {
        title: '关键字',
        dataIndex: 'keyword',
        hideInTable: true,
        fieldProps: {
          placeholder: '按配置键/操作人/说明搜索',
          allowClear: true,
        },
      },
      {
        title: '时间范围',
        dataIndex: 'operationTimeRange',
        valueType: 'dateTimeRange',
        hideInTable: true,
      },
    ],
    [],
  );

  const securityAlertColumns = useMemo<ProColumns<SecurityAlertRow>[]>(
    () => [
      {
        title: '时间',
        dataIndex: 'createdAt',
        width: 190,
        valueType: 'dateTime',
        sorter: true,
      },
      {
        title: '级别',
        dataIndex: 'severity',
        width: 120,
        valueType: 'select',
        valueEnum: {
          HIGH: { text: '高', status: 'Error' },
          MEDIUM: { text: '中', status: 'Warning' },
          LOW: { text: '低', status: 'Success' },
        },
        render: (_, record) => {
          const severity = (record.severity || 'LOW').toUpperCase();
          const color = severity === 'HIGH' ? 'red' : severity === 'MEDIUM' ? 'orange' : 'green';
          return <Tag color={color}>{securitySeverityLabel(severity)}</Tag>;
        },
      },
      {
        title: '事件类型',
        dataIndex: 'eventType',
        width: 180,
        render: (_, record) => record.title || record.eventType || '-',
      },
      {
        title: '事件标题',
        dataIndex: 'title',
        width: 220,
        ellipsis: true,
      },
      {
        title: '摘要',
        dataIndex: 'summary',
        search: false,
        ellipsis: true,
      },
      {
        title: '客户端',
        dataIndex: 'clientName',
        width: 220,
        search: false,
        render: (_, record) => record.clientName || record.clientId || '-',
      },
      {
        title: '来源平台',
        dataIndex: 'platformName',
        width: 180,
        search: false,
      },
      {
        title: '关键字',
        dataIndex: 'keyword',
        hideInTable: true,
        fieldProps: {
          placeholder: '按事件类型/标题/摘要/客户端搜索',
          allowClear: true,
        },
      },
    ],
    [],
  );

  const logCenterTabs = useMemo(() => {
    const items: Array<{ key: LogCenterTabKey; label: React.ReactNode }> = [];
    if (canViewLogDetail) {
      items.push({
        key: 'operation',
        label: (
          <span>
            <FileSearchOutlined />
            操作日志
          </span>
        ),
      });
    }
    if (canViewConfigAudit) {
      items.push({
        key: 'configAudit',
        label: (
          <span>
            <AuditOutlined />
            配置审计
          </span>
        ),
      });
    }
    if (canViewSecurityAudit) {
      items.push({
        key: 'securityAudit',
        label: (
          <span>
            <SafetyOutlined />
            安全事件
          </span>
        ),
      });
    }
    return items;
  }, [canViewConfigAudit, canViewLogDetail, canViewSecurityAudit]);

  const requestConfigAuditData = async (params: Record<string, unknown>) => {
    try {
      const { startTime, endTime } = normalizeDateRange(params.operationTimeRange);
      const records = await getAuditLogs({
        operationType: params.operationType ? String(params.operationType) : undefined,
        status: params.status ? String(params.status) : undefined,
        startTime,
        endTime,
        limit: 500,
      });
      const keyword = params.keyword ? String(params.keyword) : '';
      const filteredRecords = records
        .filter((item) =>
          filterByKeyword(
            [item.configKey || '', item.operatorName || '', item.operationReason || ''],
            keyword,
          ),
        )
        .sort((a, b) => sortByOperationTimeDesc(a.operationTime, b.operationTime));

      const current = Number(params.current || 1);
      const pageSize = Number(params.pageSize || 10);
      const start = (current - 1) * pageSize;
      const end = start + pageSize;

      return {
        success: true,
        data: filteredRecords.slice(start, end),
        total: filteredRecords.length,
      };
    } catch (error) {
      console.error('查询配置审计失败:', error);
      return {
        success: false,
        data: [],
        total: 0,
      };
    }
  };

  const requestSecurityAlertData = async (params: Record<string, unknown>) => {
    try {
      const response = await getObservabilityOverview();
      const platforms = response?.data?.platforms || [];
      const records = platforms.flatMap((platform) =>
        (platform.alerts || []).map((alert, index) => ({
          ...alert,
          id: `${platform.platformKey}-${alert.id ?? index}`,
          platformKey: platform.platformKey,
          platformName: platform.platformName,
        })),
      ) as SecurityAlertRow[];

      const keyword = params.keyword ? String(params.keyword) : '';
      const severity = params.severity ? String(params.severity) : '';
      const eventType = params.eventType ? String(params.eventType) : '';

      const filteredRecords = records
        .filter((item) => (severity ? item.severity === severity : true))
        .filter((item) => (eventType ? item.eventType === eventType : true))
        .filter((item) =>
          filterByKeyword(
            [item.eventType || '', item.title || '', item.summary || '', item.clientName || item.clientId || ''],
            keyword,
          ),
        )
        .sort((a, b) => sortByOperationTimeDesc(a.createdAt, b.createdAt));

      const current = Number(params.current || 1);
      const pageSize = Number(params.pageSize || 10);
      const start = (current - 1) * pageSize;
      const end = start + pageSize;

      return {
        success: true,
        data: filteredRecords.slice(start, end),
        total: filteredRecords.length,
      };
    } catch (error) {
      console.error('查询安全事件失败:', error);
      return {
        success: false,
        data: [],
        total: 0,
      };
    }
  };

  // 通用的请求处理函数
  const requestData = async (params: Record<string, unknown>) => {
    try {
      let response;

      if (activeTab === 'my') {
        // 我的操作日志
        response = await getMyOperationLogPage({
          current: Number(params.current || 1),
          size: Number(params.pageSize || 10),
          operationType: params.operationType ? String(params.operationType) : undefined,
        });
      } else {
        // 所有操作日志
        response = await getOperationLogs({
          current: Number(params.current || 1),
          size: Number(params.pageSize || 10),
          operationType: params.operationType ? String(params.operationType) : undefined,
          username: params.userName ? String(params.userName) : undefined,
          startTime: params.startTime ? String(params.startTime) : undefined,
          endTime: params.endTime ? String(params.endTime) : undefined,
        });
      }

      return {
        success: response.code === 200 || response.code === 0,
        data: response.data?.records ?? [],
        total: response.data?.total ?? 0,
      };
    } catch (error) {
      console.error('获取操作日志失败:', error);
      return {
        success: false,
        data: [],
        total: 0,
      };
    }
  };

  const handleCenterTabChange = (key: string) => {
    const nextTab = key as LogCenterTabKey;
    setActiveCenterTab(nextTab);
    if (nextTab === 'operation') {
      actionRef.current?.reload();
    }
    if (nextTab === 'configAudit') {
      configAuditActionRef.current?.reload();
    }
    if (nextTab === 'securityAudit') {
      securityAuditActionRef.current?.reload();
    }
  };

  if (logCenterTabs.length === 0) {
    return (
      <Empty
        description="当前账号没有日志查看权限"
        image={Empty.PRESENTED_IMAGE_SIMPLE}
      />
    );
  }

  return (
    <>
      <Tabs activeKey={activeCenterTab} onChange={handleCenterTabChange} items={logCenterTabs} />

      {activeCenterTab === 'operation' && (
        <>
          <Tabs
            activeKey={activeTab}
            onChange={setActiveTab}
            items={[
              {
                key: 'all',
                label: (
                  <span>
                    <SettingOutlined />
                    所有日志
                  </span>
                ),
              },
              {
                key: 'my',
                label: (
                  <span>
                    <UserOutlined />
                    我的日志
                  </span>
                ),
              },
            ]}
          />

          <ProTable<API.OperationLogVO>
            headerTitle={activeTab === 'my' ? '我的操作日志' : '系统操作日志'}
            actionRef={actionRef}
            rowKey="id"
            search={{
              labelWidth: 120,
              collapsed: false,
              collapseRender: false,
            }}
            scroll={{ x: 1200 }}
            toolBarRender={() => [
              <Button
                key="refresh"
                icon={<ReloadOutlined />}
                onClick={handleRefresh}
              >
                刷新
              </Button>,
              ...(canExportLogs
                ? [
                    <Button
                      key="export"
                      icon={<FileSearchOutlined />}
                      onClick={() => setExportVisible(true)}
                    >
                      导出日志
                    </Button>,
                  ]
                : []),
              ...(activeTab === 'all' && adminDropdownItems.length > 0
                ? [
                    <Dropdown
                      key="admin"
                      menu={{
                        items: adminDropdownItems,
                        onClick: handleMenuClick,
                      }}
                    >
                      <Button>
                        管理操作 <DownOutlined />
                      </Button>
                    </Dropdown>,
                  ]
                : []),
            ]}
            request={requestData}
            columns={columns.filter((col) => {
              if (activeTab === 'my' && (col.dataIndex === 'userName' || col.dataIndex === 'userId')) {
                return false;
              }
              return true;
            })}
            pagination={{
              defaultPageSize: 10,
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
            dateFormatter="string"
          />
        </>
      )}

      {activeCenterTab === 'configAudit' && (
        <ProTable<ConfigChangeLog>
          headerTitle="配置审计日志"
          actionRef={configAuditActionRef}
          rowKey="id"
          columns={configAuditColumns}
          request={requestConfigAuditData}
          search={{
            labelWidth: 110,
            collapsed: false,
            collapseRender: false,
          }}
          dateFormatter="string"
          pagination={{
            defaultPageSize: 10,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
          }}
          toolBarRender={() => [
            <Button
              key="reload-config-audit"
              icon={<ReloadOutlined />}
              onClick={() => configAuditActionRef.current?.reload()}
            >
              刷新
            </Button>,
          ]}
        />
      )}

      {activeCenterTab === 'securityAudit' && (
        <Space direction="vertical" size={16} style={{ width: '100%' }}>
          <Alert
            type="info"
            showIcon
            message="安全事件来自统一登录可观测性聚合数据，支持按关键字与级别检索。"
          />
          <ProTable<SecurityAlertRow>
            headerTitle="安全事件审计"
            actionRef={securityAuditActionRef}
            rowKey="id"
            columns={securityAlertColumns}
            request={requestSecurityAlertData}
            search={{
              labelWidth: 110,
              collapsed: false,
              collapseRender: false,
            }}
            dateFormatter="string"
            pagination={{
              defaultPageSize: 10,
              showSizeChanger: true,
              showQuickJumper: true,
              showTotal: (total, range) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
            }}
            toolBarRender={() => [
              <Button
                key="reload-security-audit"
                icon={<ReloadOutlined />}
                onClick={() => securityAuditActionRef.current?.reload()}
              >
                刷新
              </Button>,
            ]}
          />
        </Space>
      )}

      {/* 操作日志详情模态框 */}
      <OperationLogDetailModal
        visible={detailVisible}
        operationLog={currentLogDetail}
        onCancel={() => {
          setDetailVisible(false);
          setCurrentLogDetail(null);
        }}
      />

      {/* 导出日志模态框 */}
      <ExportLogModal
        visible={exportVisible}
        onCancel={() => setExportVisible(false)}
        operationTypeOptions={operationTypeOptions}
      />

      {/* 清理日志模态框 */}
      <LogCleanModal
        visible={cleanVisible}
        onCancel={() => setCleanVisible(false)}
        onSuccess={handleCleanSuccess}
        operationTypeOptions={operationTypeOptions}
      />
    </>
  );
}
