'use client';

import React, { useEffect } from 'react';
import { Modal, Form, Input, Radio, InputNumber } from 'antd';

type OrganizationFormValues = Pick<
  API.SysOrg,
  'name' | 'code' | 'status' | 'sortOrder' | 'leaderUserId'
>;

interface OrganizationModalProps {
  visible: boolean;
  mode: 'create' | 'edit';
  initialValues?: API.SysOrg;
  onOk: (values: OrganizationFormValues) => void;
  onCancel: () => void;
}

export const OrganizationModal: React.FC<OrganizationModalProps> = ({
  visible,
  mode,
  initialValues,
  onOk,
  onCancel,
}) => {
  const [form] = Form.useForm<OrganizationFormValues>();

  useEffect(() => {
    if (visible) {
      if (mode === 'edit' && initialValues) {
        form.setFieldsValue({
          name: initialValues.name,
          code: initialValues.code,
          status: initialValues.status,
          sortOrder: initialValues.sortOrder,
          leaderUserId: initialValues.leaderUserId,
        });
      } else {
        form.setFieldsValue({
          status: 0,
          sortOrder: 0,
        });
      }
    }
  }, [visible, mode, initialValues, form]);

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      onOk(values);
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
      title={mode === 'create' ? '新增组织' : '编辑组织'}
      open={visible}
      onOk={handleSubmit}
      onCancel={handleCancel}
      destroyOnHidden
      width={600}
      mask={{ closable: false }}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          status: 0,
          sortOrder: 0,
        }}
      >
        <Form.Item
          name="name"
          label="组织名称"
          rules={[
            { required: true, message: '请输入组织名称' },
            { max: 50, message: '组织名称最多50个字符' },
          ]}
        >
          <Input placeholder="请输入组织名称" />
        </Form.Item>

        <Form.Item
          name="code"
          label="组织编码"
          rules={[
            { required: true, message: '请输入组织编码' },
            { pattern: /^[A-Z_]+$/, message: '组织编码只能包含大写字母和下划线' },
            { max: 50, message: '组织编码最多50个字符' },
          ]}
        >
          <Input placeholder="请输入组织编码，如：ROOT" />
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
          name="leaderUserId"
          label="负责人用户ID"
        >
          <InputNumber
            min={0}
            placeholder="请输入负责人用户ID"
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item
          name="status"
          label="状态"
          rules={[{ required: true, message: '请选择状态' }]}
        >
          <Radio.Group>
            <Radio value={0}>启用</Radio>
            <Radio value={1}>禁用</Radio>
          </Radio.Group>
        </Form.Item>
      </Form>
    </Modal>
  );
};
