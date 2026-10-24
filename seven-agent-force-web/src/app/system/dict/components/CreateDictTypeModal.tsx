'use client';

import React, { useMemo } from 'react';
import { Modal, Form, Input, AutoComplete, Checkbox } from 'antd';
import type { CreateDictTypeRequest } from '@/types/dict';
import { useDictContext } from '../context/useDictContext';

const { TextArea } = Input;

interface CreateDictTypeModalProps {
  open: boolean;
  onCancel: () => void;
}

export const CreateDictTypeModal: React.FC<CreateDictTypeModalProps> = ({ open, onCancel }) => {
  const [form] = Form.useForm();
  const { types, handleCreateType } = useDictContext();

  const uniqueModules = useMemo(() => {
    const mods = new Set(types.map(t => t.module));
    return Array.from(mods).map(m => ({ value: m }));
  }, [types]);

  const handleOk = async () => {
    try {
      const values = await form.validateFields();
      // 转换 isSystem 从 boolean 到 0/1
      const requestData = {
        ...values,
        isSystem: values.isSystem ? 1 : 0
      };
      await handleCreateType(requestData as CreateDictTypeRequest);
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
    <Modal title="新建字典集合" open={open} onCancel={handleCancel} onOk={handleOk} destroyOnHidden>
      <Form
        form={form}
        layout="vertical"
        initialValues={{ isSystem: false }}
      >
        <Form.Item
          name="dictName"
          label="字典名称"
          rules={[{ required: true, message: '请输入字典名称' }]}
        >
          <Input placeholder="例如：用户性别" />
        </Form.Item>
        <Form.Item
          name="dictCode"
          label="字典编码 (唯一)"
          rules={[
            { required: true, message: '请输入字典编码' },
            { pattern: /^[a-zA-Z0-9_]+$/, message: '只能包含字母、数字和下划线' },
          ]}
        >
          <Input placeholder="例如：sys_user_sex" />
        </Form.Item>
        <Form.Item
          name="module"
          label="所属模块"
          rules={[{ required: true, message: '请选择或输入模块' }]}
          tooltip="可以直接输入新模块名称来创建新模块"
        >
          <AutoComplete options={uniqueModules} placeholder="选择或输入模块" />
        </Form.Item>
        <Form.Item name="dictDesc" label="描述">
          <TextArea rows={2} placeholder="描述字典的用途" />
        </Form.Item>
        <Form.Item name="isSystem" valuePropName="checked">
          <Checkbox>设为系统内置 (不可随意删除)</Checkbox>
        </Form.Item>
      </Form>
    </Modal>
  );
};
