'use client';

import React, { useCallback, useEffect } from 'react';
import { Modal, Form, Input, Radio, InputNumber } from 'antd';
import { getDeptById, getDeptOptions } from '@/api/sysDeptController';
import { RemoteSelect } from '@/components/Selectors';
import type { RemoteSelectOption } from '@/components/Selectors';

const { TextArea } = Input;

interface PostModalProps {
  visible: boolean;
  mode: 'create' | 'edit';
  initialValues?: API.SysPost;
  onOk: (values: PostFormValues) => void;
  onCancel: () => void;
}

export interface PostFormValues {
  name: string;
  code: string;
  deptId: string;
  sortOrder: number;
  status: number;
  remark?: string;
}

export const PostModal: React.FC<PostModalProps> = ({
  visible,
  mode,
  initialValues,
  onOk,
  onCancel,
}) => {
  const [form] = Form.useForm<PostFormValues>();

  const buildDeptOption = useCallback(
    (dept: API.SysDept): RemoteSelectOption<API.SysDept> => ({
      value: String(dept.id),
      label: dept.name || `部门(${dept.id})`,
      data: dept,
    }),
    [],
  );

  const fetchDeptOptions = useCallback(
    async (keyword: string): Promise<RemoteSelectOption<API.SysDept>[]> => {
      const response = await getDeptOptions({
        keyword: keyword.trim() || undefined,
        status: 0,
        limit: 20,
      });
      return (response.data ?? [])
        .filter((dept) => dept?.id !== undefined && dept?.id !== null)
        .map(buildDeptOption);
    },
    [buildDeptOption],
  );

  const fetchDeptById = useCallback(
    async (deptId: string): Promise<RemoteSelectOption<API.SysDept> | null> => {
      if (!deptId) return null;
      const response = await getDeptById({ id: deptId } as unknown as API.getDeptByIdParams);
      const dept = response.data;
      return dept?.id !== undefined && dept?.id !== null ? buildDeptOption(dept) : null;
    },
    [buildDeptOption],
  );

  useEffect(() => {
    if (visible) {
      form.resetFields();
      if (mode === 'edit' && initialValues) {
        form.setFieldsValue({
          name: initialValues.name,
          code: initialValues.code,
          deptId:
            initialValues.deptId !== undefined && initialValues.deptId !== null
              ? String(initialValues.deptId)
              : undefined,
          status: initialValues.status,
          sortOrder: initialValues.sortOrder,
          remark: initialValues.remark,
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
      title={mode === 'create' ? '新增岗位' : '编辑岗位'}
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
          label="岗位名称"
          rules={[
            { required: true, message: '请输入岗位名称' },
            { max: 50, message: '岗位名称最多50个字符' },
          ]}
        >
          <Input placeholder="请输入岗位名称" />
        </Form.Item>

        <Form.Item
          name="deptId"
          label="所属部门"
          rules={[{ required: true, message: '请选择所属部门' }]}
        >
          <RemoteSelect<string>
            placeholder="搜索并选择所属部门"
            allowClear={false}
            disabled={mode === 'edit'}
            fetchOptions={fetchDeptOptions}
            fetchByValue={fetchDeptById}
            fetchOnDropdownOpen
            style={{ width: '100%' }}
          />
        </Form.Item>

        <Form.Item
          name="code"
          label="岗位编码"
          rules={[
            { required: true, message: '请输入岗位编码' },
            { pattern: /^[A-Z_]+$/, message: '岗位编码只能包含大写字母和下划线' },
            { max: 50, message: '岗位编码最多50个字符' },
          ]}
        >
          <Input placeholder="请输入岗位编码，如：POST_001" />
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
          name="status"
          label="状态"
          rules={[{ required: true, message: '请选择状态' }]}
        >
          <Radio.Group>
            <Radio value={0}>启用</Radio>
            <Radio value={1}>禁用</Radio>
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
    </Modal>
  );
};
