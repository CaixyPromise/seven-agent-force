'use client';

import React, { forwardRef, useImperativeHandle } from 'react';
import { Alert, Form, Input, Select, Radio, InputNumber } from 'antd';

const { Option } = Select;
const { TextArea } = Input;

export interface RoleFormRef {
  validateFields: () => Promise<API.RoleCommandDTO>;
  resetFields: () => void;
}

interface RoleFormProps {
  initialValues?: API.RoleVO;
}

export const RoleForm = forwardRef<RoleFormRef, RoleFormProps>(({ initialValues }, ref) => {
  const [form] = Form.useForm();
  const isSystemRole = initialValues?.type === 'SYSTEM';
  const isEditing = initialValues?.id !== undefined;

  useImperativeHandle(ref, () => ({
    validateFields: () => form.validateFields(),
    resetFields: () => form.resetFields(),
  }));

  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={{
        type: 'CUSTOM',
        status: 0,
        dataScope: 5,
        sortOrder: 0,
        ...initialValues,
      }}
    >
      <Form.Item name="id" hidden>
        <Input />
      </Form.Item>

      <Form.Item
        name="name"
        label="角色名称"
        rules={[
          { required: true, message: '请输入角色名称' },
          { max: 50, message: '角色名称最多50个字符' },
        ]}
      >
        <Input placeholder="请输入角色名称" />
      </Form.Item>

      <Form.Item
        name="code"
        label="角色编码"
        rules={[
          { required: true, message: '请输入角色编码' },
          { pattern: /^[A-Z_]+$/, message: '角色编码只能包含大写字母和下划线' },
          { max: 50, message: '角色编码最多50个字符' },
        ]}
      >
        <Input
          disabled={isSystemRole}
          placeholder={isSystemRole ? 'SYSTEM 角色编码受保护' : '请输入角色编码，如：ADMIN'}
        />
      </Form.Item>

      <Form.Item
        name="type"
        label="角色类型"
        rules={[{ required: true, message: '请选择角色类型' }]}
      >
        <Select disabled={isSystemRole} placeholder="请选择角色类型">
          {isSystemRole ? <Option value="SYSTEM">系统角色</Option> : null}
          <Option value="BUSINESS">业务角色</Option>
          <Option value="CUSTOM">自定义角色</Option>
        </Select>
      </Form.Item>

      <Form.Item
        name="dataScope"
        label="数据权限"
        rules={[{ required: true, message: '请选择数据权限' }]}
      >
        <Select
          disabled={isEditing}
          placeholder={isEditing ? '请通过统一授权维护数据范围' : '请选择数据权限'}
        >
          <Option value={1}>全部数据</Option>
          <Option value={2}>自定数据</Option>
          <Option value={3}>本部门数据</Option>
          <Option value={4}>本部门及以下</Option>
          <Option value={5}>仅本人数据</Option>
        </Select>
      </Form.Item>

      {isEditing && !isSystemRole ? (
        <Alert
          type="info"
          showIcon
          title="数据范围由统一授权维护"
          description="编辑角色基本信息不会修改数据范围、部门、菜单、权限或配置范围。"
        />
      ) : null}

      <Form.Item
        name="status"
        label="状态"
        rules={[{ required: true, message: '请选择状态' }]}
      >
        <Radio.Group disabled={isSystemRole}>
          <Radio value={0}>启用</Radio>
          <Radio value={1}>
            禁用
          </Radio>
        </Radio.Group>
      </Form.Item>

      {isSystemRole ? (
        <Alert
          type="info"
          showIcon
          title="SYSTEM 角色由系统维护"
          description="名称、备注、排序可维护，其余安全属性由系统管理。"
        />
      ) : null}

      <Form.Item
        name="sortOrder"
        label="排序"
        rules={[{ required: true, message: '请输入排序值' }]}
      >
        <InputNumber
          min={0}
          max={9999}
          placeholder="请输入排序值"
          style={{ width: '100%' }}
        />
      </Form.Item>

      <Form.Item
        name="remark"
        label="描述"
      >
        <TextArea
          placeholder="请输入角色描述"
          rows={3}
          maxLength={200}
          showCount
        />
      </Form.Item>
    </Form>
  );
});

RoleForm.displayName = 'RoleForm';
