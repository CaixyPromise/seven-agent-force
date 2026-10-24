'use client';

import React from 'react';
import { Modal, Form, DatePicker, Select, Button, message } from 'antd';
import { DownloadOutlined } from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import { exportOperationLogs } from '@/api/operationLogController';

const { RangePicker } = DatePicker;
const { Option } = Select;

interface ExportLogModalProps {
  visible: boolean;
  onCancel: () => void;
  operationTypeOptions: API.OperationTypeOption[];
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export const ExportLogModal: React.FC<ExportLogModalProps> = ({
  visible,
  onCancel,
  operationTypeOptions,
}) => {
  const [form] = Form.useForm();

  // 导出日志
  const exportLogsMutation = useMutation({
    mutationFn: (params: Parameters<typeof exportOperationLogs>[0]) =>
      exportOperationLogs(params),
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `operation_logs_${new Date().toISOString().replace(/[:.]/g, '-')}.xlsx`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      message.success('日志导出成功');
      form.resetFields();
      onCancel();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '日志导出失败'));
    },
  });

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      const [start, end] = values.dateRange ?? [];
      await exportLogsMutation.mutateAsync({
        operationType: values.operationType,
        startTime: start?.format?.('YYYY-MM-DD HH:mm:ss'),
        endTime: end?.format?.('YYYY-MM-DD HH:mm:ss'),
      });
    } catch (error) {
      console.error('表单验证失败:', error);
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  return (
    <Modal
      title="导出操作日志"
      open={visible}
      onCancel={handleCancel}
      footer={[
        <Button key="cancel" onClick={handleCancel}>
          取消
        </Button>,
        <Button
          key="export"
          type="primary"
          icon={<DownloadOutlined />}
          loading={exportLogsMutation.isPending}
          onClick={handleSubmit}
        >
          导出
        </Button>,
      ]}
      width={600}
      mask={{ closable: false }}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          format: 'excel',
        }}
      >
        <Form.Item
          name="dateRange"
          label="时间范围"
          rules={[{ required: true, message: '请选择时间范围' }]}
        >
          <RangePicker
            showTime
            format="YYYY-MM-DD HH:mm:ss"
            placeholder={['开始时间', '结束时间']}
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item
          name="operationType"
          label="操作类型"
        >
          <Select
            placeholder="请选择操作类型（不选择则导出所有类型）"
            allowClear
            options={operationTypeOptions}
          />
        </Form.Item>

        <Form.Item
          name="userName"
          label="操作用户"
        >
          <Select
            placeholder="请输入操作用户名（不输入则导出所有用户）"
            allowClear
            showSearch
            filterOption={(input, option) =>
              String(option?.label ?? '').toLowerCase().includes(input.toLowerCase())
            }
          />
        </Form.Item>

        <Form.Item
          name="format"
          label="导出格式"
          rules={[{ required: true, message: '请选择导出格式' }]}
        >
          <Select>
            <Option value="excel">Excel (.xlsx)</Option>
            <Option value="csv">CSV (.csv)</Option>
            <Option value="pdf">PDF (.pdf)</Option>
          </Select>
        </Form.Item>
      </Form>
    </Modal>
  );
};
