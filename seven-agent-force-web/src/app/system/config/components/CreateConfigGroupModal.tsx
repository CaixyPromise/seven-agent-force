'use client';

import React, { useMemo } from 'react';
import { Modal, Form, Input, AutoComplete } from 'antd';
import type { CreateConfigGroupRequest } from '@/types/config';
import { useConfigContext } from '../context/useConfigContext';

interface CreateConfigGroupModalProps {
  open: boolean;
  onCancel: () => void;
}

export const CreateConfigGroupModal: React.FC<CreateConfigGroupModalProps> = ({ open, onCancel }) => {
  const [form] = Form.useForm();
  const { groups, handleCreateGroup } = useConfigContext();

  const uniqueModules = useMemo(() => {
    const mods = new Set(groups.map(g => g.module));
    return Array.from(mods).map(m => ({ value: m }));
  }, [groups]);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      await handleCreateGroup(values as CreateConfigGroupRequest);
      form.resetFields();
      onCancel();
    } catch {
      // 表单验证失败或创建失败
    }
  };

  const handleCancel = () => {
    form.resetFields();
    onCancel();
  };

  return (
    <Modal title="新建配置分组" open={open} onCancel={handleCancel} onOk={handleOk} destroyOnHidden>
      <Form form={form} layout="vertical">
        <Form.Item name="groupName" label="分组名称" rules={[{ required: true, message: '请输入分组名称' }]}>
          <Input placeholder="例如：基础设置" />
        </Form.Item>
        <Form.Item
          name="groupCode"
          label="分组编码 (唯一)"
          rules={[
            { required: true, message: '请输入分组编码' },
            { pattern: /^[a-zA-Z0-9_]+$/, message: '只能包含字母、数字和下划线' },
          ]}
        >
          <Input placeholder="例如：basic_config" />
        </Form.Item>
        <Form.Item
          name="module"
          label="所属模块"
          rules={[{ required: true, message: '请选择或输入模块' }]}
          tooltip="可以直接输入新模块名称来创建新模块"
        >
          <AutoComplete
            options={uniqueModules}
            placeholder="选择现有模块或输入新模块"
            filterOption={(inputValue, option) => option!.value.toUpperCase().indexOf(inputValue.toUpperCase()) !== -1}
          />
        </Form.Item>
      </Form>
    </Modal>
  );
};
