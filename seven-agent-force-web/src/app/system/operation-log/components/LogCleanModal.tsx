'use client';

import React from 'react';
import { Modal, Form, DatePicker, Select, Button, message, Alert } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import { useMutation } from '@tanstack/react-query';
import { cleanExpiredLogs } from '@/api/operationLogController';

const { RangePicker } = DatePicker;
const { Option } = Select;

interface LogCleanModalProps {
  visible: boolean;
  onCancel: () => void;
  onSuccess: () => void;
  operationTypeOptions: API.OperationTypeOption[];
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

export const LogCleanModal: React.FC<LogCleanModalProps> = ({
  visible,
  onCancel,
  onSuccess,
  operationTypeOptions,
}) => {
  const [form] = Form.useForm();

  // 清理日志
  const cleanLogsMutation = useMutation({
    mutationFn: (params: Parameters<typeof cleanExpiredLogs>[0]) =>
      cleanExpiredLogs(params),
    onSuccess: () => {
      message.success('日志清理成功');
      form.resetFields();
      onSuccess();
      onCancel();
    },
    onError: (error: unknown) => {
      message.error(getErrorMessage(error, '日志清理失败'));
    },
  });

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      await cleanLogsMutation.mutateAsync(values);
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
      title="清理操作日志"
      open={visible}
      onCancel={handleCancel}
      footer={[
        <Button key="cancel" onClick={handleCancel}>
          取消
        </Button>,
        <Button
          key="clean"
          type="primary"
          danger
          icon={<ExclamationCircleOutlined />}
          loading={cleanLogsMutation.isPending}
          onClick={handleSubmit}
        >
          确认清理
        </Button>,
      ]}
      width={600}
      mask={{ closable: false }}
    >
      <Alert
        title="警告"
        description="清理操作不可恢复，请谨慎操作！建议先备份重要日志数据。"
        type="warning"
        showIcon
        style={{ marginBottom: 16 }}
      />

      <Form
        form={form}
        layout="vertical"
        initialValues={{
          cleanType: 'date',
        }}
      >
        <Form.Item
          name="cleanType"
          label="清理方式"
          rules={[{ required: true, message: '请选择清理方式' }]}
        >
          <Select>
            <Option value="date">按时间范围清理</Option>
            <Option value="type">按操作类型清理</Option>
            <Option value="user">按用户清理</Option>
            <Option value="all">清理所有日志</Option>
          </Select>
        </Form.Item>

        <Form.Item
          noStyle
          shouldUpdate={(prevValues, currentValues) =>
            prevValues.cleanType !== currentValues.cleanType
          }
        >
          {({ getFieldValue }) => {
            const cleanType = getFieldValue('cleanType');

            if (cleanType === 'date') {
              return (
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
              );
            }

            if (cleanType === 'type') {
              return (
                <Form.Item
                  name="operationType"
                  label="操作类型"
                  rules={[{ required: true, message: '请选择操作类型' }]}
                >
                  <Select
                    placeholder="请选择要清理的操作类型"
                    options={operationTypeOptions}
                  />
                </Form.Item>
              );
            }

            if (cleanType === 'user') {
              return (
                <Form.Item
                  name="userName"
                  label="操作用户"
                  rules={[{ required: true, message: '请输入操作用户名' }]}
                >
                  <Select
                    placeholder="请输入要清理的用户名"
                    showSearch
                    filterOption={(input, option) =>
                      String(option?.label ?? '').toLowerCase().includes(input.toLowerCase())
                    }
                  />
                </Form.Item>
              );
            }

            return null;
          }}
        </Form.Item>

        <Form.Item
          name="confirmText"
          label="确认清理"
          rules={[
            { required: true, message: '请输入确认文本' },
            {
              validator: (_, value) => {
                if (value === 'DELETE') {
                  return Promise.resolve();
                }
                return Promise.reject(new Error('请输入 "DELETE" 确认清理操作'));
              },
            },
          ]}
        >
          <Select
            placeholder='请输入 "DELETE" 确认清理操作'
            showSearch
            filterOption={(input, option) =>
              String(option?.label ?? '').toLowerCase().includes(input.toLowerCase())
            }
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};
