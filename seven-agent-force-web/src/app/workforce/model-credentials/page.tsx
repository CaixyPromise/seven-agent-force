import { ApiOutlined, AppstoreOutlined, BarChartOutlined, DeleteOutlined, EditOutlined, EyeInvisibleOutlined, EyeOutlined, FallOutlined, FileTextOutlined, KeyOutlined, PauseCircleOutlined, PlayCircleOutlined, PlusOutlined, ReloadOutlined, RiseOutlined, SearchOutlined } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { UseMutationResult } from '@tanstack/react-query';
import { App, Badge, Button, Card, Drawer, Empty, Flex, Form, Input, Modal, Popconfirm, Segmented, Select, Skeleton, Space, Table, Tag, Tooltip, Typography } from 'antd';
import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import {
  createWorkforceModelCredential, deleteWorkforceModelCredential, discoverWorkforceModels, getWorkforceModelAnalytics,
  listModelCredentialInvocations, listWorkforceModelCredentials, setWorkforceModelCredentialStatus,
  updateWorkforceModelCredential, type WorkforceModelAnalytics, type WorkforceModelCredential,
  type WorkforceModelCredentialInput, type WorkforceModelInvocation,
} from '@/api/workforceController';
import '../tasks/workforce.css';
import { ModelDistributionChart, ModelTrendChart } from './ModelAnalyticsCharts';
import styles from './modelCredentials.module.css';

const { Text, Title } = Typography;
type EditorState = { mode: 'create' | 'edit'; credential?: WorkforceModelCredential } | null;
type AnalyticsDays = 7 | 30 | 90;

export default function WorkforceModelCredentialsPage() {
  const { message } = App.useApp();
  const queryClient = useQueryClient();
  const [editor, setEditor] = useState<EditorState>(null);
  const [usageCredential, setUsageCredential] = useState<WorkforceModelCredential | null>(null);
  const [showKey, setShowKey] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [protocol, setProtocol] = useState<'ALL' | 'OPENAI' | 'ANTHROPIC'>('ALL');
  const [analyticsDays, setAnalyticsDays] = useState<AnalyticsDays>(30);
  const [form] = Form.useForm<WorkforceModelCredentialInput>();
  const credentials = useQuery({ queryKey: ['workforce', 'model-credentials'], queryFn: listWorkforceModelCredentials });
  const analytics = useQuery({ queryKey: ['workforce', 'model-credential-analytics', analyticsDays], queryFn: () => getWorkforceModelAnalytics(analyticsDays) });
  const invocations = useQuery({
    queryKey: ['workforce', 'model-credential-invocations', usageCredential?.id],
    queryFn: () => listModelCredentialInvocations(usageCredential!.id),
    enabled: Boolean(usageCredential),
  });
  const visibleCredentials = useMemo(() => {
    const normalized = keyword.trim().toLowerCase();
    return (credentials.data ?? []).filter((item) => (protocol === 'ALL' || item.protocol === protocol)
      && (!normalized || [item.name, item.baseUrl, ...item.models].some((value) => value.toLowerCase().includes(normalized))));
  }, [credentials.data, keyword, protocol]);
  const refresh = async () => Promise.all([
    queryClient.invalidateQueries({ queryKey: ['workforce', 'model-credentials'] }),
    queryClient.invalidateQueries({ queryKey: ['workforce', 'model-credential-analytics'] }),
    queryClient.invalidateQueries({ queryKey: ['workforce', 'model-options'] }),
  ]);

  const save = useMutation({
    mutationFn: async () => {
      const values = await form.validateFields();
      return editor?.mode === 'edit' && editor.credential
        ? updateWorkforceModelCredential(editor.credential, values)
        : createWorkforceModelCredential(values);
    },
    onSuccess: async () => {
      message.success(editor?.mode === 'edit' ? '模型密钥已更新' : '模型密钥已创建');
      form.resetFields(); setEditor(null); setShowKey(false); save.reset(); discover.reset(); await refresh();
    },
    onError: () => message.error('保存失败，请检查地址、密钥和模型列表'),
  });
  const discover = useMutation({
    mutationFn: async () => {
      const values = await form.validateFields(['protocol', 'baseUrl', 'apiKey']);
      return discoverWorkforceModels({ ...(editor?.credential ? { credentialId: editor.credential.id } : {}), protocol: values.protocol, baseUrl: values.baseUrl, apiKey: values.apiKey ?? '' });
    },
    onSuccess: (models) => { form.setFieldValue('models', models); message.success(`已获取 ${models.length} 个模型`); },
    onError: () => message.warning('未获取到模型目录，可手动填写'),
  });
  const status = useMutation({
    mutationFn: ({ credential, next }: { credential: WorkforceModelCredential; next: 'ACTIVE' | 'DISABLED' }) => setWorkforceModelCredentialStatus(credential, next),
    onSuccess: async (_, variables) => { message.success(variables.next === 'ACTIVE' ? '模型密钥已启用' : '模型密钥已停用'); await refresh(); },
    onError: () => message.error('状态更新失败'),
  });
  const remove = useMutation({
    mutationFn: deleteWorkforceModelCredential,
    onSuccess: async (result, credential) => {
      if (usageCredential?.id === credential.id) setUsageCredential(null);
      message.success(result.retainedHistory ? '模型密钥已删除，历史调用记录已保留' : '模型密钥已删除');
      await refresh();
    },
    onError: () => message.error('删除失败，请刷新后重试'),
  });
  const closeEditor = () => { setEditor(null); setShowKey(false); form.resetFields(); save.reset(); discover.reset(); };
  const openCreate = () => { form.setFieldsValue({ name: '', protocol: 'OPENAI', baseUrl: 'https://api.openai.com/v1', apiKey: '', models: [] }); setEditor({ mode: 'create' }); };
  const openEdit = (credential: WorkforceModelCredential) => { form.setFieldsValue({ name: credential.name, protocol: credential.protocol, baseUrl: credential.baseUrl, apiKey: '', models: credential.models }); setEditor({ mode: 'edit', credential }); };

  return <div className={`workforce-page workforce-list-page ${styles.page}`}>
    <Flex className="workforce-page-heading" justify="space-between" align="flex-end" wrap gap={16}>
      <Space orientation="vertical" size={2}><Title level={2}>模型中心</Title><Text type="secondary">管理模型密钥、可用模型与调用用量</Text></Space>
      <Space><Button icon={<ReloadOutlined />} loading={credentials.isFetching || analytics.isFetching} onClick={refresh}>刷新</Button><Button type="primary" icon={<PlusOutlined />} onClick={openCreate}>新增模型密钥</Button></Space>
    </Flex>

    <OverviewStrip loading={analytics.isLoading} summary={analytics.data?.summary} />

    <section className={styles.section} aria-labelledby="credential-heading">
      <Flex className={styles.sectionHeading} justify="space-between" align="center" wrap gap={12}>
        <div><Title level={4} id="credential-heading">模型密钥</Title><Text type="secondary">调用地址、凭据与模型目录</Text></div>
        <Flex gap={8} wrap className={styles.filters}>
          <Input allowClear prefix={<SearchOutlined />} value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜索密钥名称或地址" />
          <Select value={protocol} onChange={setProtocol} options={[{ value: 'ALL', label: '全部协议' }, { value: 'OPENAI', label: 'OpenAI' }, { value: 'ANTHROPIC', label: 'Anthropic' }]} />
        </Flex>
      </Flex>
      {credentials.isLoading ? <div className={styles.cardGrid}>{[0, 1].map((item) => <Card key={item} loading className={styles.credentialCard} />)}</div>
        : credentials.isError ? <EmptyState description="模型密钥暂时无法加载" onRetry={() => credentials.refetch()} />
          : visibleCredentials.length ? <div className={styles.cardGrid}>{visibleCredentials.map((item) => <CredentialCard key={String(item.id)} item={item} status={status} remove={remove} onEdit={openEdit} onUsage={setUsageCredential} />)}</div>
            : <EmptyState description={credentials.data?.length ? '没有匹配的模型密钥' : '还没有模型密钥'} />}
    </section>

    <section className={styles.section} aria-labelledby="analytics-heading">
      <Flex className={styles.sectionHeading} justify="space-between" align="center" wrap gap={12}>
        <div><Title level={4} id="analytics-heading">调用分析</Title><Text type="secondary">调用趋势、模型分布与运行效率</Text></div>
        <Segmented<AnalyticsDays> value={analyticsDays} onChange={setAnalyticsDays} options={[{ label: '近 7 天', value: 7 }, { label: '近 30 天', value: 30 }, { label: '近 90 天', value: 90 }]} />
      </Flex>
      <AnalyticsPanel data={analytics.data} loading={analytics.isLoading} error={analytics.isError} onRetry={() => analytics.refetch()} />
    </section>

    <Modal title={editor?.mode === 'edit' ? '编辑模型密钥' : '新增模型密钥'} open={Boolean(editor)} footer={null} width={680} destroyOnHidden onCancel={closeEditor}>
      <Form form={form} layout="vertical" onFinish={() => save.mutate()} requiredMark={false}>
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入名称' }]}><Input placeholder="APIKEY.FUN" /></Form.Item>
        <Form.Item name="protocol" label="协议" rules={[{ required: true }]}><Select options={[{ value: 'OPENAI', label: 'OpenAI' }, { value: 'ANTHROPIC', label: 'Anthropic' }]} /></Form.Item>
        <Form.Item name="apiKey" label="API Key" rules={editor?.mode === 'create' ? [{ required: true, message: '请输入 API Key' }] : undefined} extra={editor?.mode === 'edit' ? '留空表示保留当前密钥' : undefined}>
          <Input type={showKey ? 'text' : 'password'} autoComplete="new-password" suffix={<Button type="text" size="small" aria-label={showKey ? '隐藏密钥' : '显示密钥'} icon={showKey ? <EyeInvisibleOutlined /> : <EyeOutlined />} onClick={() => setShowKey((value) => !value)} />} />
        </Form.Item>
        <Form.Item name="baseUrl" label="基础地址" rules={[{ required: true, message: '请输入基础地址' }, { type: 'url', message: '请输入有效地址' }]}><Input placeholder="https://api.example.com/v1" /></Form.Item>
        <Flex justify="space-between" align="center" className={styles.modelFieldHeading}><span>模型列表</span><Button size="small" type="text" icon={<ReloadOutlined />} loading={discover.isPending} onClick={() => discover.mutate()}>从上游获取</Button></Flex>
        <Form.Item name="models"><Select style={{ width: '100%' }} mode="tags" tokenSeparators={[',', '\n']} open={false} placeholder="每行一个模型，也可以用逗号分隔" /></Form.Item>
        <Flex justify="flex-end" gap={8}><Button onClick={closeEditor}>取消</Button><Button type="primary" htmlType="submit" loading={save.isPending}>保存</Button></Flex>
      </Form>
    </Modal>

    <Drawer title={usageCredential ? `${usageCredential.name} · 调用记录` : '调用记录'} open={Boolean(usageCredential)} width={Math.min(980, typeof window === 'undefined' ? 980 : window.innerWidth)} onClose={() => setUsageCredential(null)}>
      <InvocationTable data={invocations.data ?? []} loading={invocations.isLoading} />
    </Drawer>
  </div>;
}

function OverviewStrip({ loading, summary }: { loading: boolean; summary?: WorkforceModelAnalytics['summary'] }) {
  const items = [
    { label: '可用密钥', value: summary?.activeCredentialCount ?? 0, icon: <KeyOutlined /> },
    { label: '可选模型', value: summary?.modelCount ?? 0, icon: <AppstoreOutlined /> },
    { label: '本月调用', value: formatNumber(summary?.invocationCount ?? 0), icon: <BarChartOutlined /> },
    { label: 'Token 用量', value: formatCompact(summary?.totalTokens ?? 0), icon: <FileTextOutlined /> },
  ];
  return <Card className={styles.overviewCard}><div className={styles.overviewGrid}>{items.map((item) => <div key={item.label} className={styles.overviewItem}><span className={styles.overviewIcon}>{item.icon}</span><div><Text type="secondary">{item.label}</Text>{loading ? <Skeleton.Input active size="small" /> : <strong>{item.value}</strong>}</div></div>)}</div></Card>;
}

function CredentialCard({ item, status, remove, onEdit, onUsage }: { item: WorkforceModelCredential; status: UseMutationResult<WorkforceModelCredential, Error, { credential: WorkforceModelCredential; next: 'ACTIVE' | 'DISABLED' }>; remove: UseMutationResult<{ deleted: boolean; retainedHistory: boolean }, Error, WorkforceModelCredential>; onEdit: (item: WorkforceModelCredential) => void; onUsage: (item: WorkforceModelCredential) => void }) {
  return <Card className={styles.credentialCard}>
    <Flex justify="space-between" align="flex-start" gap={14}><Flex gap={12} align="center" className={styles.identity}><div className={`${styles.protocolIcon} ${item.protocol === 'ANTHROPIC' ? styles.anthropicIcon : ''}`}><ApiOutlined /></div><Space orientation="vertical" size={1} className={styles.identityText}><Title level={4} ellipsis={{ tooltip: item.name }}>{item.name}</Title><Text type="secondary">{item.credentialConfigured ? item.secretHint : '未配置密钥'}</Text></Space></Flex><Badge status={item.status === 'ACTIVE' ? 'success' : 'default'} text={item.status === 'ACTIVE' ? '已启用' : '已停用'} /></Flex>
    <Flex className={styles.tags} gap={8} wrap><Tag>{item.protocol === 'OPENAI' ? 'OpenAI' : 'Anthropic'}</Tag><Tag>{item.models.length} 个模型</Tag>{item.models[0] ? <Tag className={styles.modelTag}>{item.models[0]}</Tag> : null}</Flex>
    <div className={styles.endpoint}><Text type="secondary" className={styles.sectionLabel}>接口地址</Text><Text className={styles.baseUrl} ellipsis={{ tooltip: item.baseUrl }}>{item.baseUrl}</Text></div>
    <div className={styles.metrics}><div><Text type="secondary">调用量</Text><Text strong>{formatNumber(item.invocationCount)}</Text></div><div><Text type="secondary">Token</Text><Text strong>{formatCompact(item.totalTokens)}</Text></div><div><Text type="secondary">更新时间</Text><Text strong className={styles.updatedAt}>{formatDate(item.updatedAt)}</Text></div></div>
    <Flex className={styles.footer} align="center" gap={8}><Button className={styles.pressable} type="primary" icon={<EditOutlined />} onClick={() => onEdit(item)}>编辑配置</Button><Button className={styles.pressable} onClick={() => onUsage(item)}>调用记录</Button><Popconfirm title={item.status === 'ACTIVE' ? '停用这个模型密钥？' : '启用这个模型密钥？'} onConfirm={() => status.mutate({ credential: item, next: item.status === 'ACTIVE' ? 'DISABLED' : 'ACTIVE' })}><Button danger={item.status === 'ACTIVE'} className={styles.pressable} icon={item.status === 'ACTIVE' ? <PauseCircleOutlined /> : <PlayCircleOutlined />} loading={status.isPending && status.variables?.credential.id === item.id}>{item.status === 'ACTIVE' ? '停用' : '启用'}</Button></Popconfirm><Popconfirm title={`删除 ${item.name}？`} description="删除后不能再用于新任务，已有调用记录会继续保留。" okText="删除" cancelText="取消" okButtonProps={{ danger: true }} onConfirm={() => remove.mutate(item)}><Tooltip title="删除密钥"><Button danger size="small" className={`${styles.pressable} ${styles.deleteIcon}`} aria-label={`删除 ${item.name}`} icon={<DeleteOutlined />} loading={remove.isPending && remove.variables?.id === item.id} /></Tooltip></Popconfirm></Flex>
  </Card>;
}

function AnalyticsPanel({ data, loading, error, onRetry }: { data?: WorkforceModelAnalytics; loading: boolean; error: boolean; onRetry: () => void }) {
  if (loading) return <div className={styles.analyticsGrid}><Card className={styles.chartCard}><Skeleton active /></Card><Card className={styles.chartCard}><Skeleton active /></Card></div>;
  if (error || !data) return <EmptyState description="调用分析暂时无法加载" onRetry={onRetry} />;
  const hasInvocations = data.timeline.some((item) => item.invocationCount > 0);
  return <div className={styles.analyticsGrid}>
    <Card className={`${styles.chartCard} ${styles.trendCard}`}><Flex justify="space-between" align="flex-start" gap={16} wrap><div><Title level={5}>调用与 Token 趋势</Title><Text type="secondary">按天统计</Text></div><Flex gap={10} wrap><Comparison label="调用" value={data.comparison.invocationPercent} /><Comparison label="Token" value={data.comparison.tokenPercent} /></Flex></Flex>{hasInvocations ? <ModelTrendChart points={data.timeline} className={styles.trendChart} /> : <ChartEmpty />}</Card>
    <Card className={styles.chartCard}><Title level={5}>模型调用分布</Title>{data.modelDistribution.length ? <div className={styles.distributionWrap}><ModelDistributionChart data={data.modelDistribution} className={styles.distributionChart} /><div className={styles.donutTotal}><strong>{formatNumber(data.modelDistribution.reduce((total, item) => total + item.invocationCount, 0))}</strong><span>总调用</span></div></div> : <ChartEmpty />}<div className={styles.efficiencyGrid}><div className={styles.efficiencyItem}><div className={styles.rateRing} style={{ background: `conic-gradient(#22c3dd 0 ${data.cacheHitRate ?? 0}%, #edf2f7 ${data.cacheHitRate ?? 0}% 100%)` }}><span>{data.cacheHitRate == null ? '—' : `${data.cacheHitRate}%`}</span></div><div><Text type="secondary">缓存命中率</Text><strong>{data.cacheHitRate == null ? '暂无数据' : '缓存 Token'}</strong></div></div><div className={styles.efficiencyItem}><div className={styles.latencyGlyph}><span /></div><div><Text type="secondary">P95 延迟</Text><strong>{data.p95DurationMs == null ? '—' : formatDuration(data.p95DurationMs)}</strong></div></div></div></Card>
  </div>;
}

function Comparison({ label, value }: { label: string; value?: number }) { const positive = (value ?? 0) >= 0; return <span className={`${styles.comparison} ${value == null ? styles.neutral : positive ? styles.positive : styles.negative}`}>{value == null ? null : positive ? <RiseOutlined /> : <FallOutlined />}{label} {value == null ? '暂无对比' : `${Math.abs(value)}%`}</span>; }
function ChartEmpty() { return <div className={styles.chartEmpty}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无调用数据" /></div>; }
function EmptyState({ description, onRetry }: { description: string; onRetry?: () => void }) { return <Card className={styles.emptyCard}><Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={description}>{onRetry ? <Button onClick={onRetry}>重试</Button> : null}</Empty></Card>; }

function InvocationTable({ data, loading }: { data: WorkforceModelInvocation[]; loading: boolean }) {
  return <Table rowKey="id" dataSource={data} loading={loading} pagination={{ pageSize: 20 }} scroll={{ x: 1300 }} columns={[
    { title: '任务', key: 'task', width: 220, render: (_, item) => <Space orientation="vertical" size={0}><Link to={`/workforce/tasks/${encodeURIComponent(item.taskId)}`}>{item.taskName.trim() || '未命名任务'}</Link><Text type="secondary">{item.taskId}</Text></Space> },
    { title: '实际模型', dataIndex: 'requestedModel', width: 190, ellipsis: true },
    { title: '用途', key: 'kind', width: 120, render: (_, item) => item.executionKind === 'REVIEW_RUN' ? '独立审查' : item.isPrimaryModel ? '主模型' : '辅助调用' },
    { title: '状态', dataIndex: 'status', width: 120, render: (value) => <Tag color={value === 'SUCCEEDED' ? 'green' : value === 'FAILED' ? 'red' : 'blue'}>{value}</Tag> },
    { title: '首字节 / 总耗时', key: 'latency', width: 160, render: (_, item) => `${item.firstByteMs ?? '—'} / ${item.durationMs ?? '—'} ms` },
    { title: '输入 / 缓存 / 输出 / 总计', key: 'usage', width: 250, render: (_, item) => [item.usage.inputTokens, item.usage.cachedInputTokens, item.usage.outputTokens, item.usage.totalTokens].map((value) => value ?? '—').join(' / ') },
    { title: '时间', dataIndex: 'startedAt', width: 180, render: (value) => value ? new Date(value).toLocaleString() : '—' },
  ]} />;
}

function formatNumber(value: number): string { return new Intl.NumberFormat('zh-CN').format(value); }
function formatCompact(value: number): string { return new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value); }
function formatDuration(value: number): string { return value >= 1000 ? `${(value / 1000).toFixed(value >= 10_000 ? 1 : 2)} s` : `${value} ms`; }
function formatDate(value?: string): string { return value ? new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'; }
