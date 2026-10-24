'use client';

import React, { forwardRef, useImperativeHandle } from 'react';
import { Form, Input, Select, Radio, InputNumber, TreeSelect } from 'antd';

const { Option } = Select;
const { TextArea } = Input;

export interface MenuFormRef {
  validateFields: () => Promise<API.MenuVO>;
  resetFields: () => void;
}

interface MenuFormProps {
  initialValues?: API.MenuVO;
  menuTree: API.MenuVO[];
}

interface MenuTreeSelectNode {
  value?: API.Int64;
  title?: string;
  children?: MenuTreeSelectNode[];
}

export const MenuForm = forwardRef<MenuFormRef, MenuFormProps>(({ initialValues, menuTree }, ref) => {
  const [form] = Form.useForm<API.MenuVO>();

  useImperativeHandle(ref, () => ({
    validateFields: () => form.validateFields(),
    resetFields: () => form.resetFields(),
  }));

  // 转换菜单树为TreeSelect数据格式
  const convertToTreeSelectData = (tree: API.MenuVO[]): MenuTreeSelectNode[] => {
    return tree.map(node => ({
      value: node.id,
      title: node.name,
      children: node.children ? convertToTreeSelectData(node.children) : undefined,
    }));
  };

  const treeSelectData = convertToTreeSelectData(menuTree);

  return (
    <Form
      form={form}
      layout="vertical"
      initialValues={{
        type: 'C',
        status: 0,
        visible: 0,
        sortOrder: 0,
        ...initialValues,
      }}
    >
      <Form.Item name="id" hidden>
        <Input />
      </Form.Item>

      <Form.Item
        name="parentId"
        label="父级菜单"
      >
        <TreeSelect
          placeholder="请选择父级菜单"
          allowClear
          treeData={treeSelectData}
          style={{ width: '100%' }}
        />
      </Form.Item>

      <Form.Item
        name="name"
        label="菜单名称"
        rules={[
          { required: true, message: '请输入菜单名称' },
          { max: 50, message: '菜单名称最多50个字符' },
        ]}
      >
        <Input placeholder="请输入菜单名称" />
      </Form.Item>

      <Form.Item
        name="type"
        label="菜单类型"
        rules={[{ required: true, message: '请选择菜单类型' }]}
      >
        <Select placeholder="请选择菜单类型">
          <Option value="M">目录</Option>
          <Option value="C">菜单</Option>
          <Option value="F">按钮</Option>
        </Select>
      </Form.Item>

      <Form.Item
        name="path"
        label="路由地址"
        rules={[
          { max: 200, message: '路由地址最多200个字符' },
        ]}
      >
        <Input placeholder="请输入路由地址，如：/system/user" />
      </Form.Item>

      <Form.Item
        name="component"
        label="组件路径"
        rules={[
          { max: 200, message: '组件路径最多200个字符' },
        ]}
      >
        <Input placeholder="请输入组件路径，如：system/user/index" />
      </Form.Item>

      <Form.Item
        name="permission"
        label="权限标识"
        rules={[
          { max: 100, message: '权限标识最多100个字符' },
        ]}
      >
        <Input placeholder="请输入权限标识，如：system:user:list" />
      </Form.Item>

      <Form.Item
        name="icon"
        label="菜单图标"
        rules={[
          { max: 100, message: '菜单图标最多100个字符' },
        ]}
      >
        <Input placeholder="请输入菜单图标，如：UserOutlined" />
      </Form.Item>

      <Form.Item
        name="sortOrder"
        label="显示排序"
        rules={[{ required: true, message: '请输入显示排序' }]}
      >
        <InputNumber
          min={0}
          max={9999}
          placeholder="请输入显示排序"
          style={{ width: '100%' }}
        />
      </Form.Item>

      <Form.Item
        name="visible"
        label="是否显示"
        rules={[{ required: true, message: '请选择是否显示' }]}
      >
        <Radio.Group>
          <Radio value={0}>显示</Radio>
          <Radio value={1}>隐藏</Radio>
        </Radio.Group>
      </Form.Item>

      <Form.Item
        name="status"
        label="菜单状态"
        rules={[{ required: true, message: '请选择菜单状态' }]}
      >
        <Radio.Group>
          <Radio value={0}>正常</Radio>
          <Radio value={1}>停用</Radio>
        </Radio.Group>
      </Form.Item>

      <Form.Item
        name="remark"
        label="备注"
      >
        <TextArea
          placeholder="请输入备注"
          rows={3}
          maxLength={200}
          showCount
        />
      </Form.Item>
    </Form>
  );
});

MenuForm.displayName = 'MenuForm';
