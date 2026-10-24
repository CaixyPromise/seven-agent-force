import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Modal, Form, Input, Select, InputNumber, TreeSelect, Avatar } from 'antd';
import { BankOutlined, ApartmentOutlined, IdcardOutlined } from '@ant-design/icons';
import type {
  DepartmentEntity,
  EntityType,
  OrganizationEntity,
  PostEntity,
} from '../hooks/useOrganizationManagement';
import { RemoteSelect } from '@/components/Selectors';
import type { RemoteSelectOption } from '@/components/Selectors';
import { searchUsers, getSimpleUserById } from '@/api/userController';
import { getDeptOptions, getDeptById } from '@/api/sysDeptController';

const { Option } = Select;
const { TextArea } = Input;

interface EntityModalProps {
  visible: boolean;
  mode: 'create' | 'edit';
  entity: Partial<EntityType> | null;
  organizationOptions?: OrganizationEntity[]; // 组织选项（用于部门选择父组织）
  onCancel: () => void;
  onSubmit: (values: Partial<EntityType>) => Promise<void>;
}

interface EntityFormValues {
  id?: API.Int64;
  type?: EntityType['type'];
  name?: string;
  code?: string;
  status?: number;
  sortOrder?: number;
  remark?: string;
  parentId?: API.Int64;
  orgId?: API.Int64;
  deptId?: API.Int64;
  leaderUserId?: API.Int64;
}

function entityOrgId(entity: Partial<EntityType> | null): API.Int64 | undefined {
  if (entity?.type === 'dept' || entity?.type === 'post') {
    return entity.orgId;
  }
  return undefined;
}

function optionalId(value: API.Int64 | undefined): API.Int64 | undefined {
  return value && value !== '0' ? String(value) : undefined;
}

/**
 * 实体编辑弹窗 - 支持组织/部门/岗位的新增和编辑
 */
const EntityModal: React.FC<EntityModalProps> = ({
  visible,
  mode,
  entity,
  organizationOptions = [],
  onCancel,
  onSubmit,
}) => {
  const [form] = Form.useForm<EntityFormValues>();
  const [submitting, setSubmitting] = useState(false);
  const watchedOrgId = Form.useWatch('orgId', form);
  const initialOrgIdRef = useRef<string | undefined>(undefined);

  const currentOrgId = useMemo(() => {
    if (!entity) return undefined;
    const orgId = entityOrgId(entity);
    return orgId !== undefined && orgId !== null ? String(orgId) : undefined;
  }, [entity]);

  const entityId = useMemo(() => {
    if (!entity?.id) return undefined;
    return String(entity.id);
  }, [entity]);

  const normalizedOrganizations = useMemo(() => {
    return (organizationOptions || []).map(org => ({
      ...org,
      id: String(org.id),
    }));
  }, [organizationOptions]);

  const effectiveOrgId = useMemo(() => {
    if (watchedOrgId !== undefined && watchedOrgId !== null && watchedOrgId !== '') {
      return String(watchedOrgId);
    }
    const orgId = entityOrgId(entity);
    if (orgId !== undefined) {
      return String(orgId);
    }
    return currentOrgId;
  }, [watchedOrgId, entity, currentOrgId]);

  useEffect(() => {
    if (entity?.type === 'dept') {
      const rawOrgId = entity.orgId;
      initialOrgIdRef.current = rawOrgId !== undefined && rawOrgId !== null ? String(rawOrgId) : undefined;
    } else if (!visible) {
      initialOrgIdRef.current = undefined;
    }
  }, [entity, visible]);

  useEffect(() => {
    if (entity?.type !== 'dept') return;

    if (watchedOrgId === undefined || watchedOrgId === null || watchedOrgId === '') {
      form.setFieldsValue({ parentId: undefined });
      return;
    }

    const orgIdValue = String(watchedOrgId);
    if (initialOrgIdRef.current !== undefined && initialOrgIdRef.current !== orgIdValue) {
      form.setFieldsValue({ parentId: undefined });
    }
    initialOrgIdRef.current = orgIdValue;
  }, [watchedOrgId, entity?.type, form]);

  const fetchUserOptions = useCallback(async (keyword: string) => {
    if (!keyword?.trim()) {
      return [] as RemoteSelectOption<API.SimpleUserVO>[];
    }

    try {
      const response = await searchUsers({ keyword: keyword.trim(), limit: 20 });
      const users = (response.data || []).filter(user => user?.id !== undefined && user?.id !== null);
      return users.map((user) => ({
        value: String(user.id),
        label: user.nickName || user.username || `用户(${user.id})`,
        data: user,
      }));
    } catch (error) {
      console.error('搜索用户失败:', error);
      return [];
    }
  }, []);

  const fetchUserById = useCallback(async (
    userId: string,
  ): Promise<RemoteSelectOption<API.SimpleUserVO> | null> => {
    if (!userId) return null;
    try {
      const response = await getSimpleUserById({ id: userId });
      const user = response.data;
      if (!user || user.id === undefined || user.id === null) return null;
      return {
        value: String(user.id),
        label: user.nickName || user.username || `用户(${user.id})`,
        data: user,
      };
    } catch (error) {
      console.error('根据ID获取用户失败:', error);
      return null;
    }
  }, []);

  const renderUserOption = useCallback((option: RemoteSelectOption<API.SimpleUserVO>) => {
    const user = option.data;
    if (!user) {
      return option.label;
    }

    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <Avatar
          size={24}
          src={user.avatar}
          style={{ flexShrink: 0 }}
        >
          {(user.nickName || user.username || '').slice(0, 1).toUpperCase() || 'U'}
        </Avatar>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div
            style={{
              fontWeight: 500,
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {user.nickName || user.username || option.label}
          </div>
          {user.username && (
            <div
              style={{
                fontSize: 12,
                color: '#888',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              @{user.username}
            </div>
          )}
        </div>
      </div>
    );
  }, []);

  const renderDeptOption = useCallback((option: RemoteSelectOption<API.SysDept>) => {
    const dept = option.data;
    return (
      <div style={{ display: 'flex', flexDirection: 'column' }}>
        <span style={{ fontWeight: 500 }}>{option.label}</span>
        {dept?.code && (
          <span style={{ fontSize: 12, color: '#888' }}>编码: {dept.code}</span>
        )}
      </div>
    );
  }, []);

  const buildDeptOption = useCallback((dept: API.SysDept): RemoteSelectOption<API.SysDept> => ({
    value: String(dept.id),
    label: dept.name || `部门(${dept.id})`,
    data: dept,
  }), []);

  const fetchDeptOptionsBase = useCallback(
    async (keyword: string, targetOrgId?: string, excludeIds: string[] = []) => {
      if (!targetOrgId) {
        return [] as RemoteSelectOption<API.SysDept>[];
      }

      try {
        const response = await getDeptOptions({
          keyword: keyword?.trim(),
          orgId: targetOrgId,
          status: 0,
          limit: 20,
        });

        const excludeSet = new Set(excludeIds);
        const depts = (response.data || []).filter((dept) =>
          dept && dept.id !== undefined && dept.id !== null && !excludeSet.has(String(dept.id)),
        );

        return depts.map((dept) => buildDeptOption(dept));
      } catch (error) {
        console.error('搜索部门失败:', error);
        return [];
      }
    },
    [buildDeptOption],
  );

  const fetchParentDeptOptions = useCallback(
    async (keyword: string) =>
      fetchDeptOptionsBase(keyword, effectiveOrgId, entityId ? [entityId] : []),
    [fetchDeptOptionsBase, effectiveOrgId, entityId],
  );

  const fetchPostDeptOptions = useCallback(
    async (keyword: string) => fetchDeptOptionsBase(keyword, currentOrgId),
    [fetchDeptOptionsBase, currentOrgId],
  );

  const fetchDeptOptionById = useCallback(async (deptId: string) => {
    if (!deptId) return null;
    try {
      const response = await getDeptById({ id: deptId });
      const dept = response.data;
      if (!dept || dept.id === undefined || dept.id === null) return null;
      return buildDeptOption(dept);
    } catch (error) {
      console.error('根据ID获取部门失败:', error);
      return null;
    }
  }, [buildDeptOption]);

  // 获取实体类型的显示名称和图标
  const getEntityInfo = (type?: string) => {
    switch (type) {
      case 'org':
        return { name: '组织', icon: <BankOutlined />, color: '#1890ff' };
      case 'dept':
        return { name: '部门', icon: <ApartmentOutlined />, color: '#52c41a' };
      case 'post':
        return { name: '岗位', icon: <IdcardOutlined />, color: '#fa8c16' };
      default:
        return { name: '实体', icon: null, color: '#666' };
    }
  };

  const entityInfo = getEntityInfo(entity?.type);

  // 表单初始化
  useEffect(() => {
    console.log('EntityModal useEffect 触发:', { visible, entity, mode });
    if (visible && entity) {
      console.log('设置表单值:', entity);
      const nextValues: EntityFormValues = {
        ...entity,
        orgId: entityOrgId(entity),
        sortOrder: entity.sortOrder ?? 0,
        status: entity.status ?? 0,
      };
      if (entity.type === 'org') {
        nextValues.parentId = optionalId(entity.parentId);
      } else if (entity.type === 'dept') {
        nextValues.parentId = optionalId(entity.parentId);
        nextValues.leaderUserId = entity.leaderUserId;
      } else if (entity.type === 'post') {
        nextValues.deptId = entity.deptId || undefined;
      }
      form.setFieldsValue(nextValues);
    } else if (visible) {
      console.log('重置表单');
      form.resetFields();
      // 设置默认值
      form.setFieldsValue({
        status: 0,
        sortOrder: 0,
      });
    }
  }, [visible, entity, form, mode]);

  const handleSubmit = async () => {
    if (submitting) return; // 防止重复提交

    try {
      setSubmitting(true);
      const values = await form.validateFields();
      if (entity?.type === 'org') {
        const payload: Partial<OrganizationEntity> = {
          ...entity,
          ...values,
          type: 'org',
          parentId: values.parentId || '0',
        };
        await onSubmit(payload);
      } else if (entity?.type === 'dept') {
        const payload: Partial<DepartmentEntity> = {
          ...entity,
          ...values,
          type: 'dept',
          orgId: values.orgId,
          parentId: values.parentId || '0',
        };
        await onSubmit(payload);
      } else if (entity?.type === 'post') {
        const payload: Partial<PostEntity> = {
          ...entity,
          ...values,
          type: 'post',
          orgId: values.orgId,
          deptId: values.deptId,
        };
        await onSubmit(payload);
      }

      // 提交成功后重置表单
      form.resetFields();

      // Modal会在父组件的onSubmit中关闭，这里不需要额外操作
    } catch (error) {
      console.error('提交表单失败:', error);
      // 表单验证失败，不需要特殊处理
    } finally {
      setSubmitting(false);
    }
  };

  const renderFormItems = () => {
    if (!entity?.type) return null;

    const commonFields = (
      <>
        <Form.Item
          label="名称"
          name="name"
          rules={[
            { required: true, message: `请输入${entityInfo.name}名称` },
            { max: 50, message: '名称长度不能超过50个字符' },
          ]}
        >
          <Input placeholder={`请输入${entityInfo.name}名称`} />
        </Form.Item>

        <Form.Item
          label="编码"
          name="code"
          rules={[
            { max: 64, message: '编码长度不能超过64个字符' },
            { pattern: /^[a-zA-Z0-9_-]*$/, message: '编码只能包含字母、数字、下划线和横线' },
          ]}
        >
          <Input placeholder={`请输入${entityInfo.name}编码`} />
        </Form.Item>

        <Form.Item
          label="状态"
          name="status"
          rules={[{ required: true, message: '请选择状态' }]}
        >
          <Select>
            <Option value={0}>正常</Option>
            <Option value={1}>停用</Option>
          </Select>
        </Form.Item>

        <Form.Item
          label="排序"
          name="sortOrder"
          rules={[{ required: true, message: '请输入排序值' }]}
        >
          <InputNumber min={0} max={9999} precision={0} style={{ width: '100%' }} />
        </Form.Item>

        <Form.Item label="备注" name="remark">
          <TextArea
            rows={3}
            maxLength={500}
            placeholder="请输入备注信息"
            showCount
          />
        </Form.Item>
      </>
    );

    switch (entity.type) {
      case 'org':
        return (
          <>
            {/* 组织可以选择父组织 */}
            <Form.Item label="父组织" name="parentId">
              <TreeSelect
                placeholder="选择父组织（不选择则为顶级组织）"
                allowClear
                treeDefaultExpandAll
                treeData={normalizedOrganizations.map(org => ({
                  title: org.name,
                  value: org.id,
                  key: org.id,
                  disabled: mode === 'edit' && entityId !== undefined && org.id === entityId,
                }))}
              />
            </Form.Item>

            {commonFields}

            <Form.Item label="负责人" name="leaderUserId">
              <RemoteSelect<string, API.SimpleUserVO>
                placeholder="搜索并选择负责人"
                allowClear
                fetchOptions={fetchUserOptions}
                fetchByValue={fetchUserById}
                optionRender={renderUserOption}
                fetchOnDropdownOpen={false}
                style={{ width: '100%' }}
              />
            </Form.Item>
          </>
        );

      case 'dept':
        return (
          <>
            <Form.Item
              label="所属组织"
              name="orgId"
              rules={[{ required: true, message: '请选择所属组织' }]}
            >
              <Select placeholder="请选择所属组织" disabled={mode === 'edit'}>
                {normalizedOrganizations.map(org => (
                  <Option key={org.id} value={org.id}>
                    {org.name}
                  </Option>
                ))}
              </Select>
            </Form.Item>

            <Form.Item label="父部门" name="parentId">
              <RemoteSelect<string, API.SysDept>
                placeholder={effectiveOrgId ? '搜索并选择父部门（留空为顶级部门）' : '请先选择所属组织'}
                allowClear
                disabled={!effectiveOrgId}
                fetchOptions={fetchParentDeptOptions}
                fetchByValue={fetchDeptOptionById}
                optionRender={renderDeptOption}
                fetchOnDropdownOpen={!!effectiveOrgId}
                style={{ width: '100%' }}
              />
            </Form.Item>

            {commonFields}

            <Form.Item label="负责人" name="leaderUserId">
              <RemoteSelect<string, API.SimpleUserVO>
                placeholder="搜索并选择负责人"
                allowClear
                fetchOptions={fetchUserOptions}
                fetchByValue={fetchUserById}
                optionRender={renderUserOption}
                fetchOnDropdownOpen={false}
                style={{ width: '100%' }}
              />
            </Form.Item>
          </>
        );

      case 'post':
        return (
          <>
            <Form.Item
              label="所属组织"
              name="orgId"
              rules={[{ required: true, message: '请选择所属组织' }]}
            >
              <Select placeholder="请选择所属组织" disabled>
                {normalizedOrganizations.map(org => (
                  <Option key={org.id} value={org.id}>
                    {org.name}
                  </Option>
                ))}
              </Select>
            </Form.Item>

            <Form.Item
              label="所属部门"
              name="deptId"
              rules={[{ required: true, message: '请选择所属部门' }]}
            >
              <RemoteSelect<string, API.SysDept>
                placeholder="请选择所属部门"
                disabled={mode === 'edit'}
                allowClear={false}
                fetchOptions={fetchPostDeptOptions}
                fetchByValue={fetchDeptOptionById}
                optionRender={renderDeptOption}
                fetchOnDropdownOpen
                style={{ width: '100%' }}
              />
            </Form.Item>

            {/* 岗位编码是必填的 */}
            <Form.Item
              label="岗位编码"
              name="code"
              rules={[
                { required: true, message: '请输入岗位编码' },
                { max: 64, message: '编码长度不能超过64个字符' },
                { pattern: /^[a-zA-Z0-9_-]+$/, message: '编码只能包含字母、数字、下划线和横线' },
              ]}
            >
              <Input placeholder="请输入岗位编码" />
            </Form.Item>

            <Form.Item
              label="岗位名称"
              name="name"
              rules={[
                { required: true, message: '请输入岗位名称' },
                { max: 50, message: '名称长度不能超过50个字符' },
              ]}
            >
              <Input placeholder="请输入岗位名称" />
            </Form.Item>

            <Form.Item
              label="状态"
              name="status"
              rules={[{ required: true, message: '请选择状态' }]}
            >
              <Select>
                <Option value={0}>正常</Option>
                <Option value={1}>停用</Option>
              </Select>
            </Form.Item>

            <Form.Item
              label="排序"
              name="sortOrder"
              rules={[{ required: true, message: '请输入排序值' }]}
            >
              <InputNumber min={0} max={9999} precision={0} style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item label="备注" name="remark">
              <TextArea
                rows={3}
                maxLength={500}
                placeholder="请输入岗位描述"
                showCount
              />
            </Form.Item>
          </>
        );

      default:
        return null;
    }
  };

  return (
    <Modal
      title={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <span style={{ color: entityInfo.color }}>{entityInfo.icon}</span>
          <span>
            {mode === 'create' ? '新建' : '编辑'}{entityInfo.name}
          </span>
        </div>
      }
      open={visible}
      width={600}
      onCancel={onCancel}
      onOk={handleSubmit}
      okText="确定"
      cancelText="取消"
      confirmLoading={submitting}
      destroyOnHidden
    >
      <Form
        form={form}
        layout="vertical"
        preserve={false}
      >
        {renderFormItems()}
      </Form>
    </Modal>
  );
};

export default EntityModal;
