'use client';

import React, { forwardRef, useImperativeHandle } from 'react';
import { Form, Input, Select, Radio } from 'antd';

const { Option } = Select;
const { TextArea } = Input;

export interface PermissionFormRef {
  validateFields: () => Promise<API.PermissionVO>;
  resetFields: () => void;
}

interface PermissionFormProps {
  initialValues?: API.PermissionVO;
}

export const PermissionForm = forwardRef<PermissionFormRef, PermissionFormProps>(({ initialValues }, ref) => {
  const [form] = Form.useForm<API.PermissionVO>();
  const resourceType = Form.useWatch('resourceType', form);

  useImperativeHandle(ref, () => ({
    validateFields: () => form.validateFields(),
    resetFields: () => form.resetFields(),
  }));

  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={{
        status: 0,
        ...initialValues,
      }}
    >
      <Form.Item name="id" hidden>
        <Input />
      </Form.Item>

      <Form.Item
        name="name"
        label="权限名称"
        rules={[
          { required: true, message: '请输入权限名称' },
          { max: 50, message: '权限名称最多50个字符' },
        ]}
      >
        <Input placeholder="请输入权限名称" />
      </Form.Item>

      <Form.Item
        name="code"
        label="权限标识"
        rules={[
          { required: true, message: '请输入权限标识' },
          { max: 100, message: '权限标识最多100个字符' },
        ]}
      >
        <Input placeholder="请输入权限标识，如：system:user:list" />
      </Form.Item>

      <Form.Item
        name="resourceType"
        label="资源类型"
        rules={[{ required: true, message: '请选择资源类型' }]}
      >
        <Select placeholder="请选择资源类型">
          <Option value="API">API</Option>
          <Option value="BUTTON">按钮</Option>
          <Option value="TOPIC">主题</Option>
        </Select>
      </Form.Item>

      {resourceType === 'API' && (
        <>
          <Form.Item
            name="method"
            label="请求方法"
            rules={[{ required: true, message: '请选择请求方法' }]}
          >
            <Select placeholder="请选择请求方法">
              <Option value="GET">GET</Option>
              <Option value="POST">POST</Option>
              <Option value="PUT">PUT</Option>
              <Option value="DELETE">DELETE</Option>
              <Option value="PATCH">PATCH</Option>
            </Select>
          </Form.Item>

          <Form.Item
            name="path"
            label="资源路径"
            rules={[
              { required: true, message: '请输入资源路径' },
              { max: 200, message: '资源路径最多200个字符' },
            ]}
          >
            <Input placeholder="请输入资源路径，如：/system/user" />
          </Form.Item>
        </>
      )}

      <Form.Item
        name="status"
        label="状态"
        rules={[{ required: true, message: '请选择权限状态' }]}
      >
        <Radio.Group>
          <Radio value={0}>启用</Radio>
          <Radio value={1}>停用</Radio>
        </Radio.Group>
      </Form.Item>

      <Form.Item
        name="description"
        label="描述"
      >
        <TextArea
          placeholder="请输入描述"
          rows={3}
          maxLength={200}
          showCount
        />
      </Form.Item>
    </Form>
  );
});

PermissionForm.displayName = 'PermissionForm';
