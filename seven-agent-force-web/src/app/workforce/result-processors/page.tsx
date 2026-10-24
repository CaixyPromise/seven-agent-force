import { CodeOutlined, PlusOutlined, SearchOutlined, UploadOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, App, Badge, Button, Card, Flex, Form, Input, Modal, Segmented, Select, Space, Table, Tag, Typography, Upload, type TableColumnsType } from 'antd';
import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  createEmptyProcessorDraft,
  createResultProcessor,
  importResultProcessorArchive,
  listResultProcessors,
  type ResultProcessor,
} from '@/api/resultProcessorController';
import styles from './processors.module.css';

const { Text, Title } = Typography;
type Category = '全部' | '通用' | 'Node/npm' | 'Go' | '报告输出' | 'Git Delivery' | '其他';

export default function ResultProcessorsPage() {
  const { message } = App.useApp();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [category, setCategory] = useState<Category>('全部');
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const [form] = Form.useForm();
  const processors = useQuery({ queryKey: ['workforce', 'result-processors'], queryFn: listResultProcessors });
  const create = useMutation({
    mutationFn: async (values: { name: string; category: string; description?: string; archive?: { fileList?: Array<{ originFileObj?: File }> } }) => {
      const archive = values.archive?.fileList?.[0]?.originFileObj;
      const item = await createResultProcessor(createEmptyProcessorDraft(values.name, values.category, values.description));
      if (!archive) return item;
      if (!item.currentDraft) throw new Error('新建草稿未返回，无法导入 archive');
      await importResultProcessorArchive(item.currentDraft, archive);
      return item;
    },
    onSuccess: async (item) => {
      message.success('Result Processor 草稿已创建');
      setOpen(false);
      form.resetFields();
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'result-processors'] });
      navigate(`/workforce/result-processors/${encodeURIComponent(item.id)}`);
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '创建失败'),
  });
  const visible = useMemo(() => (processors.data ?? []).filter((item) => {
    const categoryMatches = category === '全部' || item.category === category || (category === '其他' && !['通用', 'Node/npm', 'Go', '报告输出', 'Git Delivery'].includes(item.category));
    const haystack = `${item.name} ${item.category} ${item.description}`.toLowerCase();
    return categoryMatches && (!query.trim() || haystack.includes(query.trim().toLowerCase()));
  }), [category, processors.data, query]);

  const columns: TableColumnsType<ResultProcessor> = [
    {
      title: 'Result Processor', dataIndex: 'name', width: 310,
      render: (_, item) => <Space orientation="vertical" size={2} className={styles.nameCell}>
        <Space wrap><Text strong>{item.name}</Text>{item.scopeType === 'SYSTEM' && <Tag color="gold">官方内置</Tag>}{item.defaultBound && <Tag color="blue">默认 default</Tag>}</Space>
        <Text type="secondary" className={styles.mono}>{item.id}</Text>
      </Space>,
    },
    { title: '分类', dataIndex: 'category', width: 130, render: (value: string) => <Tag>{value}</Tag> },
    {
      title: '草稿状态', width: 140,
      render: (_, item) => item.currentDraft
        ? <Badge status={item.currentDraft.validation?.valid ? 'success' : 'processing'} text={item.currentDraft.validation?.valid ? '已验证' : `Draft ${item.currentDraft.revisionNumber}`} />
        : <Text type="secondary">无草稿</Text>,
    },
    {
      title: '最新 Release', width: 160,
      render: (_, item) => item.latestRelease
        ? <Space orientation="vertical" size={1}><Tag color="green">v{item.latestRelease.version}</Tag><Text className={styles.mono} type="secondary" ellipsis={{ tooltip: item.latestRelease.archiveSha256 }}>{shortDigest(item.latestRelease.archiveSha256)}</Text></Space>
        : <Text type="secondary">尚未发布</Text>,
    },
    { title: '说明', dataIndex: 'description', width: 280, ellipsis: true, responsive: ['lg'], render: (value: string) => <Text type="secondary" ellipsis={{ tooltip: value }} className={styles.description}>{value || '—'}</Text> },
    { title: '更新时间', dataIndex: 'updatedAt', width: 170, responsive: ['xl'], render: (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false }) },
  ];

  return (
    <div className={styles.page}>
      <Flex justify="space-between" align="flex-end" wrap gap={16} className={styles.heading}>
        <Space orientation="vertical" size={2}>
          <Title level={2}>Result Processor</Title>
          <Text type="secondary">将验收逻辑资产化；处理器决定 PASS、自动修正或人工处理，可信结果仍由平台独立复算。</Text>
        </Space>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>新建 Processor</Button>
      </Flex>
      <Card className={styles.card} styles={{ body: { padding: 0 } }}>
        <Flex className={styles.toolbar} justify="space-between" wrap gap={12}>
          <Segmented<Category> value={category} onChange={setCategory} options={['全部', '通用', 'Node/npm', 'Go', '报告输出', 'Git Delivery', '其他']} />
          <Input allowClear prefix={<SearchOutlined />} className={styles.search} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索 Processor、分类或说明" />
        </Flex>
        <Table
          rowKey="id"
          loading={processors.isLoading}
          columns={columns}
          dataSource={visible}
          pagination={{ pageSize: 20, showTotal: (total) => `共 ${total} 个` }}
          scroll={{ x: 960 }}
          onRow={(item) => ({ onClick: () => navigate(`/workforce/result-processors/${encodeURIComponent(item.id)}`), style: { cursor: 'pointer' } })}
        />
      </Card>
      <Modal title="新建 Result Processor" open={open} footer={null} destroyOnHidden onCancel={() => setOpen(false)}>
        <Form form={form} layout="vertical" initialValues={{ category: '通用' }} onFinish={(values) => create.mutate(values)}>
          <Alert type="info" showIcon title="Python 处理器" description="固定入口为 processor.py 的 process(context)。平台 SDK、镜像内模块和受保护 bootstrap 由精确 Harness Image 提供。" style={{ marginBottom: 16 }} />
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }, { max: 128 }]}><Input prefix={<CodeOutlined />} placeholder="quality-gate-python" /></Form.Item>
          <Form.Item name="category" label="分类"><Select options={['通用', 'Node/npm', 'Go', '报告输出', 'Git Delivery', '其他'].map((value) => ({ value }))} /></Form.Item>
          <Form.Item name="description" label="说明"><Input.TextArea rows={3} maxLength={500} showCount /></Form.Item>
          <Form.Item name="archive" label="从 archive 初始化（可选）"><Upload accept=".gz,.tgz" beforeUpload={() => false} maxCount={1}><Button icon={<UploadOutlined />}>选择 .tar.gz</Button></Upload></Form.Item>
          <Flex justify="flex-end" gap={8}><Button onClick={() => setOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={create.isPending}>创建并编辑</Button></Flex>
        </Form>
      </Modal>
    </div>
  );
}

function shortDigest(value: string) {
  return value ? `${value.slice(0, 18)}…` : '—';
}
