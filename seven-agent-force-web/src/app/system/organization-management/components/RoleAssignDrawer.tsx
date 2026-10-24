import React, { useState, useCallback } from 'react';
import {
  Drawer,
  Transfer,
  Space,
  Button,
  Input,
  Spin,
  message,
  Card,
  Tag,
  Typography,
  Divider,
  Empty,
  Alert,
} from 'antd';
import {
  SafetyOutlined,
  SearchOutlined,
  IdcardOutlined,
  UserOutlined,
} from '@ant-design/icons';
import type { TransferItem } from 'antd/es/transfer';
import type { Key } from 'react';
import { useOrganizationManagement } from '../hooks/useOrganizationManagement';
import type { PostEntity } from '../hooks/useOrganizationManagement';
import { getPostRoles, assignRolesToPost } from '@/api/sysPostRoleController';

const { Title, Text } = Typography;

interface RoleAssignDrawerProps {
  visible: boolean;
  post: PostEntity | null;
  onClose: () => void;
  onSuccess?: () => void;
}

interface RoleItem extends TransferItem {
  key: string;
  title: string;
  description?: string;
  code?: string;
  status?: number;
  authorizationRoot?: boolean;
}

async function fetchPostAssignedRoles(postId: API.Int64): Promise<API.Int64[]> {
  try {
    const response = await getPostRoles({ postId });
    return response.data || [];
  } catch (error) {
    console.error('获取岗位已分配角色失败:', error);
    message.error('获取岗位已分配角色失败');
    return [];
  }
}

/**
 * 岗位角色分配抽屉
 */
const RoleAssignDrawer: React.FC<RoleAssignDrawerProps> = ({
  visible,
  post,
  onClose,
  onSuccess,
}) => {
  const [targetKeys, setTargetKeys] = useState<string[]>([]);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);
  const [allRoles, setAllRoles] = useState<RoleItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [searchValue, setSearchValue] = useState('');
  const [blockedRootRoleIds, setBlockedRootRoleIds] = useState<API.Int64[]>([]);

  const { fetchRoles } = useOrganizationManagement({
    onError: () => {
      message.error('操作失败');
    },
  });

  // 获取所有角色列表
  const loadRoles = useCallback(async () => {
    if (!post) return;

    setLoading(true);
    try {
      // 并发获取所有角色和当前岗位已分配的角色
      const [rolesResponse, assignedRolesResponse] = await Promise.all([
        fetchRoles({ size: 1000 }), // 获取大量数据，实际应该支持搜索和分页
        fetchPostAssignedRoles(post.id), // TODO: 实现获取岗位已分配角色的API
      ]);

      const rootRoleIds = new Set(
        rolesResponse.data
          .filter((role: API.RoleVO) => role.authorizationRoot && role.id)
          .map((role: API.RoleVO) => String(role.id)),
      );
      const roles: RoleItem[] = rolesResponse.data
        .filter((role: API.RoleVO) => !role.authorizationRoot && role.id && role.id !== '0')
        .map((role: API.RoleVO) => ({
          key: String(role.id),
          title: role.name ?? '',
          description: role.remark,
          code: role.code,
          status: role.status,
          authorizationRoot: role.authorizationRoot,
        }));
      const historicalRootRelations = assignedRolesResponse.filter((id) =>
        rootRoleIds.has(id),
      );

      setAllRoles(roles);
      setBlockedRootRoleIds(historicalRootRelations);
      setTargetKeys(
        assignedRolesResponse
          .filter((id) => !rootRoleIds.has(id)),
      );
    } catch {
      message.error('获取角色数据失败');
    } finally {
      setLoading(false);
    }
  }, [post, fetchRoles]);

  // 保存岗位角色分配的API调用
  const savePostRoles = async (postId: API.Int64, roleIds: API.Int64[]): Promise<void> => {
    try {
      const response = await assignRolesToPost({ postId }, roleIds);
      if (!response.data) {
        throw new Error('保存岗位角色分配失败');
      }
    } catch (error) {
      console.error('保存岗位角色分配失败:', error);
      throw error;
    }
  };

  // 处理角色转移
  const handleChange = (newTargetKeys: Key[]) => {
    setTargetKeys(newTargetKeys.map(String));
  };

  // 处理选中状态变化
  const handleSelectChange = (sourceSelectedKeys: Key[], targetSelectedKeys: Key[]) => {
    setSelectedKeys([...sourceSelectedKeys, ...targetSelectedKeys].map(String));
  };

  // 保存角色分配
  const handleSave = async () => {
    if (!post || saving) return; // 防止重复提交
    if (blockedRootRoleIds.length > 0) {
      message.error('检测到历史安全根岗位关系，请先按升级 preflight 清单人工处理');
      return;
    }

    setSaving(true);
    try {
      await savePostRoles(String(post.id), targetKeys);

      message.success('角色分配保存成功');

      // 先关闭抽屉，再触发刷新
      onClose();

      // 延迟触发刷新，确保抽屉已关闭
      setTimeout(() => {
        onSuccess?.();
      }, 100);

    } catch (error) {
      console.error('保存岗位角色分配失败:', error);
      message.error('保存失败: ' + (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  // 重置状态
  const handleReset = () => {
    setTargetKeys([]);
    setSelectedKeys([]);
    loadRoles(); // 重新加载原始数据
  };

  // 搜索过滤
  const getFilteredRoles = () => {
    if (!searchValue) return allRoles;

    const keyword = searchValue.toLowerCase();
    return allRoles.filter(role =>
      role.title.toLowerCase().includes(keyword) ||
      (role.code && role.code.toLowerCase().includes(keyword)) ||
      (role.description && role.description.toLowerCase().includes(keyword))
    );
  };

  // 自定义渲染Transfer项
  const renderItem = (item: RoleItem) => {
    const customLabel = (
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <div>
          <div style={{ fontWeight: 500 }}>{item.title}</div>
          {item.code && (
            <Tag color="blue" style={{ marginTop: 4 }}>
              {item.code}
            </Tag>
          )}
          {item.description && (
            <div style={{ fontSize: '12px', color: '#666', marginTop: 2 }}>
              {item.description.length > 30
                ? `${item.description.substring(0, 30)}...`
                : item.description
              }
            </div>
          )}
        </div>
        <Tag color={item.status === 0 ? 'green' : 'red'}>
          {item.status === 0 ? '正常' : '停用'}
        </Tag>
      </div>
    );

    return {
      label: customLabel,
      value: item.title,
    };
  };

  return (
    <Drawer
      title={
        <Space>
          <SafetyOutlined style={{ color: '#fa8c16' }} />
          <span>岗位角色管理</span>
        </Space>
      }
      placement="right"
      size={800}
      open={visible}
      onClose={onClose}
      afterOpenChange={(open) => {
        if (open && post) {
          void loadRoles();
        } else if (!open) {
          setSearchValue('');
          setSelectedKeys([]);
        }
      }}
      extra={
        <Space>
          <Button onClick={handleReset}>重置</Button>
          <Button type="primary" onClick={handleSave} loading={saving} disabled={blockedRootRoleIds.length > 0}>
            保存
          </Button>
        </Space>
      }
    >
      {post && (
        <>
          {/* 岗位信息卡片 */}
          <Card size="small" style={{ marginBottom: 16 }}>
            <Space>
              <IdcardOutlined style={{ color: '#fa8c16' }} />
              <div>
                <Title level={5} style={{ margin: 0 }}>
                  {post.name}
                </Title>
                <Space size={8}>
                  <Text type="secondary">编码: {post.code}</Text>
                  <Tag color={post.status === 0 ? 'green' : 'red'}>
                    {post.status === 0 ? '正常' : '停用'}
                  </Tag>
                </Space>
              </div>
            </Space>
          </Card>

          <Divider />

          <Alert
            type={blockedRootRoleIds.length > 0 ? 'error' : 'info'}
            showIcon
            style={{ marginBottom: 16 }}
            message="安全根只能直接授予用户，不能通过岗位继承"
            description={
              blockedRootRoleIds.length > 0
                ? '当前岗位存在历史安全根关系，已阻止保存。请先运行并按 rbac-root-preflight 输出清单完成迁移。'
                : '安全根已从可选角色中排除，后端也会拒绝相关写入。'
            }
          />

          {/* 搜索框 */}
          <div style={{ marginBottom: 16 }}>
            <Input
              placeholder="搜索角色名称、编码或描述"
              prefix={<SearchOutlined />}
              value={searchValue}
              onChange={(e) => setSearchValue(e.target.value)}
              allowClear
            />
          </div>

          {/* 角色分配Transfer */}
          <Spin spinning={loading}>
            {allRoles.length > 0 ? (
              <Transfer
                dataSource={getFilteredRoles()}
                targetKeys={targetKeys}
                selectedKeys={selectedKeys}
                onChange={handleChange}
                onSelectChange={handleSelectChange}
                render={renderItem}
                titles={['可选角色', '已分配角色']}
                operations={['分配', '移除']}
                listStyle={{
                  width: 350,
                  height: 500,
                }}
                showSearch={false} // 使用自定义搜索
                locale={{
                  itemUnit: '项',
                  itemsUnit: '项',
                  notFoundContent: '暂无数据',
                  searchPlaceholder: '请输入搜索内容',
                }}
              />
            ) : (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description="暂无角色数据"
                style={{ marginTop: 100 }}
              />
            )}
          </Spin>

          {/* 统计信息 */}
          {allRoles.length > 0 && (
            <div style={{ marginTop: 16, padding: 16, background: '#fafafa', borderRadius: 6 }}>
              <Space split={<Divider orientation="vertical" />}>
                <span>
                  <UserOutlined style={{ marginRight: 4 }} />
                  总角色数: <strong>{allRoles.length}</strong>
                </span>
                <span>
                  已分配: <strong style={{ color: '#52c41a' }}>{targetKeys.length}</strong>
                </span>
                <span>
                  可分配: <strong style={{ color: '#1890ff' }}>{allRoles.length - targetKeys.length}</strong>
                </span>
              </Space>
            </div>
          )}
        </>
      )}
    </Drawer>
  );
};

export default RoleAssignDrawer;
