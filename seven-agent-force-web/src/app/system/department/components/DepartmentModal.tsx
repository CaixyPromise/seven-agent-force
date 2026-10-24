'use client';

import React, { useEffect } from 'react';
import { Modal, Form, Input, Radio, InputNumber, Select } from 'antd';

type DepartmentFormValues = Pick<
  API.SysDept,
  'name' | 'code' | 'orgId' | 'status' | 'sortOrder' | 'leaderUserId'
>;

interface OrganizationSelectOption {
  value?: API.Int64;
  label: string;
}

interface DepartmentModalProps {
  visible: boolean;
  mode: 'create' | 'edit';
  initialValues?: API.SysDept;
  orgTree: API.SysOrg[];
  onOk: (values: DepartmentFormValues) => void;
  onCancel: () => void;
}

export const DepartmentModal: React.FC<DepartmentModalProps> = ({
  visible,
  mode,
  initialValues,
  orgTree,
  onOk,
  onCancel,
}) => {
  const [form] = Form.useForm<DepartmentFormValues>();

  useEffect(() => {
    if (visible) {
      if (mode === 'edit' && initialValues) {
        form.setFieldsValue({
          name: initialValues.name,
          code: initialValues.code,
          orgId: initialValues.orgId,
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

  // 转换组织树为Select数据格式
  const convertOrgToSelectData = (tree: API.SysOrg[]): OrganizationSelectOption[] => {
    const result: OrganizationSelectOption[] = [];
    const traverse = (orgs: API.SysOrg[], level = 0) => {
      orgs.forEach(org => {
        result.push({
          value: org.id,
          label: '  '.repeat(level) + org.name,
        });
        if (org.children) {
          traverse(org.children, level + 1);
        }
      });
    };
    traverse(tree);
    return result;
  };

  const orgSelectData = convertOrgToSelectData(orgTree);

  return (
    <Modal
      title={mode === 'create' ? '新增部门' : '编辑部门'}
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
          label="部门名称"
          rules={[
            { required: true, message: '请输入部门名称' },
            { max: 50, message: '部门名称最多50个字符' },
          ]}
        >
          <Input placeholder="请输入部门名称" />
        </Form.Item>

        <Form.Item
          name="code"
          label="部门编码"
          rules={[
            { required: true, message: '请输入部门编码' },
            { pattern: /^[A-Z_]+$/, message: '部门编码只能包含大写字母和下划线' },
            { max: 50, message: '部门编码最多50个字符' },
          ]}
        >
          <Input placeholder="请输入部门编码，如：DEPT_001" />
        </Form.Item>

        <Form.Item
          name="orgId"
          label="所属组织"
          rules={[{ required: true, message: '请选择所属组织' }]}
        >
          <Select
            placeholder="请选择所属组织"
            options={orgSelectData}
            showSearch
            filterOption={(input, option) =>
              (option?.label ?? '').toLowerCase().includes(input.toLowerCase())
            }
          />
        </Form.Item>

        <Form.Item
          name="leaderUserId"
          label="负责人"
        >
          <InputNumber
            min={0}
            placeholder="请输入负责人用户ID"
            style={{ width: '100%' }}
          />
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

      </Form>
    </Modal>
  );
};
