'use client';

import React from 'react';
import { Form, Input, message, Modal } from 'antd';
import { useMutation } from '@tanstack/react-query';
import {
    changeCurrentUserPassword,
    type UserSelfPasswordChangeRequest,
} from '@/api/userProfileController';

interface PasswordChangeModalProps {
    open: boolean;
    onCancel: () => void;
    onSuccess: () => void;
}

export default function PasswordChangeModal({
    open,
    onCancel,
    onSuccess,
}: PasswordChangeModalProps) {
    const [form] = Form.useForm<UserSelfPasswordChangeRequest>();

    const changePasswordMutation = useMutation({
        mutationFn: (payload: UserSelfPasswordChangeRequest) => changeCurrentUserPassword(payload),
        onSuccess: () => {
            message.success('密码修改成功，请重新登录');
            form.resetFields();
            onSuccess();
        },
        onError: (error: Error) => {
            message.error(error.message || '密码修改失败');
        },
    });

    const handleOk = () => {
        form.validateFields().then((values) => {
            changePasswordMutation.mutate(values);
        });
    };

    return (
        <Modal
            title="修改登录密码"
            open={open}
            onOk={handleOk}
            onCancel={() => {
                form.resetFields();
                onCancel();
            }}
            confirmLoading={changePasswordMutation.isPending}
            okText="确认修改"
            cancelText="取消"
            destroyOnHidden
            forceRender
        >
            <Form
                form={form}
                layout="vertical"
                preserve={false}
            >
                <Form.Item
                    label="当前密码"
                    name="oldPassword"
                    rules={[{ required: true, message: '请输入当前密码' }]}
                >
                    <Input.Password placeholder="请输入当前密码" autoComplete="current-password" />
                </Form.Item>
                <Form.Item
                    label="新密码"
                    name="newPassword"
                    rules={[
                        { required: true, message: '请输入新密码' },
                        { min: 6, message: '密码长度至少为6位' },
                    ]}
                >
                    <Input.Password placeholder="请输入新密码" autoComplete="new-password" />
                </Form.Item>
                <Form.Item
                    label="确认新密码"
                    name="confirmPassword"
                    dependencies={['newPassword']}
                    rules={[
                        { required: true, message: '请确认新密码' },
                        ({ getFieldValue }) => ({
                            validator(_, value) {
                                if (!value || getFieldValue('newPassword') === value) {
                                    return Promise.resolve();
                                }
                                return Promise.reject(new Error('两次输入的新密码不一致'));
                            },
                        }),
                    ]}
                >
                    <Input.Password placeholder="请再次输入新密码" autoComplete="new-password" />
                </Form.Item>
            </Form>
        </Modal>
    );
}
