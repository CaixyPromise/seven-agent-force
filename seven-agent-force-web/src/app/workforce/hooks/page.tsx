import { CodeOutlined, PlusOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { App, Badge, Button, Card, Flex, Form, Input, Modal, Segmented, Select, Space, Table, Tag, Typography, Upload, type TableColumnsType } from 'antd';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { createHookPackage, importHookArchive, listHookPackages, type HookPackage, type HookRuntime } from '@/api/workforceHooks';
import styles from './hooks.module.css';

const { Text, Title } = Typography;
type Category = '全部' | '安全策略' | '生命周期' | '质量门禁' | '其他';

export default function ManagedHooksPage() {
  const { message } = App.useApp();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [category, setCategory] = useState<Category>('全部');
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();
  const packages = useQuery({ queryKey: ['workforce', 'hooks'], queryFn: listHookPackages });
  const create = useMutation({
    mutationFn: async (values: { name: string; category: string; description?: string; runtime: HookRuntime; archive?: { fileList?: Array<{ originFileObj?: File }> } }) => {
      const item = await createHookPackage(values);
      const archive = values.archive?.fileList?.[0]?.originFileObj;
      return archive ? importHookArchive(item, archive) : item;
    },
    onSuccess: async (item) => { message.success('Hook 草稿已创建'); setOpen(false); form.resetFields(); await queryClient.invalidateQueries({ queryKey: ['workforce', 'hooks'] }); navigate(`/workforce/hooks/${item.id}`); },
    onError: (error) => message.error(error instanceof Error ? error.message : '创建失败'),
  });
  const visible = useMemo(() => (packages.data ?? []).filter((item) => (category === '全部' || item.category === category) && (!query.trim() || `${item.name} ${item.description}`.toLowerCase().includes(query.trim().toLowerCase()))), [category, packages.data, query]);
  const columns: TableColumnsType<HookPackage> = [
    { title: 'Hook 名称', dataIndex: 'name', width: 320, render: (_, item) => <Space direction="vertical" size={2} className={styles.nameCell}><Text strong>{item.name}</Text><Text type="secondary">{item.description || '—'}</Text></Space> },
    { title: '分类', dataIndex: 'category', width: 120, render: (value) => <Tag>{value}</Tag> },
    { title: 'Runtime', width: 110, render: (_, item) => <Tag color={item.manifest.runtime.kind === 'node' ? 'blue' : 'default'}>{item.manifest.runtime.kind === 'node' ? 'Node 22' : 'Shell'}</Tag> },
    { title: '草稿', width: 120, render: (_, item) => <Badge status={item.validation?.valid ? 'success' : 'processing'} text={item.validation?.valid ? '已验证' : `Revision ${item.revision}`} /> },
    { title: '最新发布', dataIndex: 'latestVersion', width: 120, render: (value) => value ? <Tag color="green">v{value}</Tag> : <Text type="secondary">未发布</Text> },
    { title: '更新时间', dataIndex: 'updatedAt', width: 180, responsive: ['lg'], render: (value) => new Date(value).toLocaleString('zh-CN', { hour12: false }) },
  ];
  return <div className={styles.page}>
    <Flex justify="space-between" align="flex-end" wrap gap={16} className={styles.heading}>
      <Space direction="vertical" size={2}><Title level={2}>Hook 管理</Title><Text type="secondary">编辑、测试和发布 Agent Hook</Text></Space>
      <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>新建 Hook</Button>
    </Flex>
    <Card className={styles.card} styles={{ body: { padding: 0 } }}>
      <Flex className={styles.toolbar} justify="space-between" wrap gap={12}>
        <Segmented<Category> value={category} onChange={setCategory} options={['全部', '安全策略', '生命周期', '质量门禁', '其他']} />
        <Input allowClear prefix={<SearchOutlined />} className={styles.search} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 Hook" />
      </Flex>
      <Table rowKey="id" loading={packages.isLoading} columns={columns} dataSource={visible} pagination={{ pageSize: 20, showTotal: (total) => `共 ${total} 个` }} scroll={{ x: 880 }} onRow={(item) => ({ onClick: () => navigate(`/workforce/hooks/${item.id}`), style: { cursor: 'pointer' } })} />
    </Card>
    <Modal title="新建 Hook" open={open} footer={null} destroyOnHidden onCancel={() => setOpen(false)}>
      <Form form={form} layout="vertical" initialValues={{ category: '安全策略', runtime: 'node' }} onFinish={(values) => create.mutate(values)}>
        <Form.Item name="name" label="名称" rules={[{ required: true }, { max: 128 }]}><Input prefix={<CodeOutlined />} placeholder="seven.demo.command-guard" /></Form.Item>
        <Flex gap={12}><Form.Item name="category" label="分类" style={{ flex: 1 }}><Select options={['安全策略', '生命周期', '质量门禁', '其他'].map((value) => ({ value }))} /></Form.Item><Form.Item name="runtime" label="Runtime" style={{ flex: 1 }}><Select options={[{ value: 'node', label: 'Node 22（镜像固定）' }, { value: 'shell', label: 'Shell' }]} /></Form.Item></Flex>
        <Form.Item name="description" label="说明"><Input.TextArea rows={3} maxLength={500} showCount /></Form.Item>
        <Form.Item name="archive" label="从 archive 初始化（可选）"><Upload accept=".gz,.tgz" beforeUpload={() => false} maxCount={1}><Button icon={<UploadOutlined />}>选择 .tar.gz</Button></Upload></Form.Item>
        <Flex justify="flex-end" gap={8}><Button onClick={() => setOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={create.isPending}>创建并编辑</Button></Flex>
      </Form>
    </Modal>
  </div>;
}
