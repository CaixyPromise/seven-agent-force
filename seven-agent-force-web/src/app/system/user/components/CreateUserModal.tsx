'use client';

import React from 'react';
import { Modal, Form, Input, Radio, message } from 'antd';
import { useMutation } from '@tanstack/react-query';
import { createUser } from '@/api/userController';
import { useDictValueOnly } from '@/hooks/useDictValue';
import { buildUserGenderOptions, USER_GENDER_DICT_CODE } from '@/lib/userGender';
import { USER_STATUS_OPTIONS } from '@/lib/userStatus';

interface CreateUserModalProps {
  visible: boolean;
  onOk: () => void;
  onCancel: () => void;
}

export const CreateUserModal: React.FC<CreateUserModalProps> = ({
  visible,
  onOk,
  onCancel,
}) => {
  const [form] = Form.useForm();
  const genderItems = useDictValueOnly(USER_GENDER_DICT_CODE);
  const genderOptions = buildUserGenderOptions(genderItems);

  const createUserMutation = useMutation({
    mutationFn: (values: API.UserCreateRequest) => createUser(values),
    onSuccess: () => {
      message.success('用户创建成功');
      form.resetFields();
      onOk();
    },
    onError: (error: Error) => {
      message.error(error.message || '用户创建失败');
    },
  });

  const handleSubmit = async () => {
    try {
      const values = await form.validateFields();
      await createUserMutation.mutateAsync(values);
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
      title="新增用户"
      open={visible}
      onOk={handleSubmit}
      onCancel={handleCancel}
      confirmLoading={createUserMutation.isPending}
      destroyOnHidden
      width={600}
      mask={{ closable: false }}
    >
      <Form
        form={form}
        layout="vertical"
        initialValues={{
          userGender: 0,
          status: 0,
        }}
      >
        <Form.Item
          name="username"
          label="用户名"
          rules={[
            { required: true, message: '请输入用户名' },
            { min: 4, message: '用户名至少4个字符' },
            { max: 20, message: '用户名最多20个字符' },
            { pattern: /^[a-zA-Z0-9_]+$/, message: '用户名只能包含字母、数字和下划线' },
          ]}
        >
          <Input placeholder="请输入用户名" />
        </Form.Item>

        <Form.Item
          name="nickname"
          label="昵称"
          rules={[
            { required: true, message: '请输入昵称' },
            { max: 50, message: '昵称最多50个字符' },
          ]}
        >
          <Input placeholder="请输入昵称" />
        </Form.Item>

        <Form.Item
          name="email"
          label="邮箱"
          rules={[
            { required: true, message: '请输入邮箱' },
            { type: 'email', message: '请输入有效的邮箱地址' },
          ]}
        >
          <Input placeholder="请输入邮箱" />
        </Form.Item>

        <Form.Item
          name="userPhone"
          label="手机号"
          rules={[
            { required: true, message: '请输入手机号' },
            { pattern: /^1[3-9]\d{9}$/, message: '请输入有效的手机号' },
          ]}
        >
          <Input placeholder="请输入手机号" />
        </Form.Item>

        <Form.Item
          name="password"
          label="密码"
          rules={[
            { required: true, message: '请输入密码' },
            { min: 8, message: '密码至少8个字符' },
            { max: 20, message: '密码最多20个字符' },
          ]}
        >
          <Input.Password placeholder="请输入密码" />
        </Form.Item>

        <Form.Item
          name="userGender"
          label="性别"
          rules={[{ required: true, message: '请选择性别' }]}
        >
          <Radio.Group>
            {genderOptions.map((option) => (
              <Radio key={option.value} value={option.value}>
                {option.label}
              </Radio>
            ))}
          </Radio.Group>
        </Form.Item>

        <Form.Item
          name="status"
          label="状态"
          rules={[{ required: true, message: '请选择状态' }]}
        >
          <Radio.Group>
            {USER_STATUS_OPTIONS.map((option) => (
              <Radio key={option.value} value={option.value}>
                {option.label}
              </Radio>
            ))}
          </Radio.Group>
        </Form.Item>

      </Form>
    </Modal>
  );
};
