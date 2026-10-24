import {
  ArrowLeftOutlined,
  DeleteOutlined,
  DiffOutlined,
  FileAddOutlined,
  FolderOpenOutlined,
  SaveOutlined,
  SendOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  App,
  Badge,
  Button,
  Card,
  Checkbox,
  Drawer,
  Flex,
  Form,
  Grid,
  Input,
  InputNumber,
  Modal,
  Result,
  Select,
  Skeleton,
  Space,
  Table,
  Tabs,
  Tag,
  Tree,
  Typography,
  Upload,
  type TableColumnsType,
} from 'antd';
import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { listHarnessChannels } from '@/api/workforceHooks';
import {
  createResultProcessorFixtureRun,
  createResultProcessorRevision,
  getResultProcessor,
  getResultProcessorFixtureRun,
  importResultProcessorArchive,
  publishResultProcessorRevision,
  saveResultProcessorRevision,
  setDefaultResultProcessor,
  setResultProcessorEnvironment,
  validateResultProcessorRevision,
  type ResultProcessor,
  type ResultProcessorDecision,
  type ResultProcessorEnvironmentBinding,
  type ResultProcessorManifest,
  type ResultProcessorRevision,
} from '@/api/resultProcessorController';
import styles from '../processors.module.css';

const MonacoProcessorEditor = lazy(() => import('./MonacoProcessorEditor'));
const { Text, Title } = Typography;
const decisions: ResultProcessorDecision[] = ['PASS', 'REVISE', 'WAIT_USER', 'REJECT', 'MANUAL_REVIEW'];

export default function ResultProcessorEditorPage() {
  const { processorId = '' } = useParams();
  const navigate = useNavigate();
  const { message, modal } = App.useApp();
  const queryClient = useQueryClient();
  const screens = Grid.useBreakpoint();
  const [draft, setDraft] = useState<ResultProcessorRevision>();
  const [selectedPath, setSelectedPath] = useState('');
  const [dirty, setDirty] = useState(false);
  const [treeOpen, setTreeOpen] = useState(false);
  const [fixtureOpen, setFixtureOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [environmentOpen, setEnvironmentOpen] = useState(false);
  const [fixtureRunId, setFixtureRunId] = useState('');
  const [fixtureForm] = Form.useForm();
  const [environmentForm] = Form.useForm();
  const item = useQuery({ queryKey: ['workforce', 'result-processor', processorId], queryFn: () => getResultProcessor(processorId), enabled: Boolean(processorId) });
  const harnesses = useQuery({ queryKey: ['workforce', 'harness-channels'], queryFn: listHarnessChannels, enabled: fixtureOpen });
  const fixture = useQuery({
    queryKey: ['workforce', 'result-processor-fixture', fixtureRunId],
    queryFn: () => getResultProcessorFixtureRun(fixtureRunId),
    enabled: Boolean(fixtureRunId),
    refetchInterval: (query) => isFixtureTerminal(query.state.data?.status) ? false : 1_500,
  });

  useEffect(() => {
    const source = item.data?.currentDraft ?? releaseAsReadOnlyRevision(item.data?.latestRelease);
    if (source && !dirty) {
      const next = structuredClone(source);
      setDraft(next);
      setSelectedPath((current) => current || next.files[0]?.path || 'processor.py');
    }
  }, [item.data, dirty]);
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => { if (dirty) event.preventDefault(); };
    addEventListener('beforeunload', guard);
    return () => removeEventListener('beforeunload', guard);
  }, [dirty]);

  const save = useMutation({
    mutationFn: () => saveResultProcessorRevision(draft!),
    onSuccess: async (next) => {
      setDraft(next);
      setDirty(false);
      message.success('草稿已保存');
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'result-processor', processorId] });
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'result-processors'] });
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '保存失败；本地内容仍保留'),
  });
  const validate = useMutation({
    mutationFn: () => validateResultProcessorRevision(draft!.id),
    onSuccess: (validation) => {
      setDraft((current) => current ? { ...current, validation } : current);
      message[validation.valid ? 'success' : 'error'](validation.valid ? 'Validate 通过' : 'Validate 未通过');
    },
    onError: (error) => message.error(error instanceof Error ? error.message : 'Validate 失败'),
  });
  const importArchive = useMutation({
    mutationFn: (file: File) => importResultProcessorArchive(draft!, file),
    onSuccess: (next) => {
      setDraft(next);
      setDirty(false);
      setSelectedPath(next.files[0]?.path || 'processor.py');
      message.success('archive 已安全导入草稿；请重新 Validate');
    },
    onError: (error) => message.error(error instanceof Error ? error.message : 'archive 导入失败'),
  });
  const createRevision = useMutation({
    mutationFn: () => createResultProcessorRevision(processorId),
    onSuccess: (next) => {
      setDraft(next);
      setDirty(false);
      setSelectedPath(next.files[0]?.path || 'processor.py');
      message.success('已从不可变 Release 创建新的草稿');
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '创建草稿失败'),
  });
  const publish = useMutation({
    mutationFn: (values: { version: string }) => publishResultProcessorRevision(draft!.id, values.version),
    onSuccess: async (release) => {
      message.success(`已发布不可变 Release v${release.version}`);
      setPublishOpen(false);
      await item.refetch();
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'result-processors'] });
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '发布失败'),
  });
  const runFixture = useMutation({
    mutationFn: (values: { harnessReleaseId: string; evaluationContext: string; workspaceFiles: string; configOverrides: string; expectedDecision?: ResultProcessorDecision }) => createResultProcessorFixtureRun(draft!.id, {
      harnessReleaseId: values.harnessReleaseId,
      evaluationContext: parseJSONObject(values.evaluationContext, '评估上下文'),
      workspaceFiles: parseWorkspaceFiles(values.workspaceFiles),
      configOverrides: parseJSONObject(values.configOverrides, '配置覆盖'),
      expectedDecision: values.expectedDecision,
    }),
    onSuccess: (result) => {
      setFixtureRunId(result.runId);
      message.success('Fixture 已进入 Worker 专属后处理队列');
    },
    onError: (error) => message.error(error instanceof Error ? error.message : 'Fixture 创建失败'),
  });
  const setDefault = useMutation({
    mutationFn: (releaseId: string) => setDefaultResultProcessor(releaseId),
    onSuccess: async () => {
      message.success('默认 Result Processor 已更新；只影响新建任务');
      await queryClient.invalidateQueries({ queryKey: ['workforce', 'result-processors'] });
      await item.refetch();
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '设置默认失败'),
  });
  const bindEnvironment = useMutation({
    mutationFn: (values: Omit<ResultProcessorEnvironmentBinding, 'id' | 'tenantId' | 'processorReleaseId'>) => {
      const releaseId = item.data?.latestRelease?.id;
      if (!releaseId) throw new Error('请先发布 Release，再绑定环境变量');
      return setResultProcessorEnvironment(releaseId, values);
    },
    onSuccess: () => {
      message.success('环境绑定已保存；SECRET 只保存引用与版本，不显示值');
      setEnvironmentOpen(false);
      environmentForm.resetFields();
    },
    onError: (error) => message.error(error instanceof Error ? error.message : '保存环境绑定失败'),
  });

  const processorItem = item.data;
  const officialProcessor = processorItem?.scopeType === 'SYSTEM';
  const selected = draft?.files.find((file) => file.path === selectedPath);
  const readOnly = officialProcessor || draft?.status === 'PUBLISHED';
  const treeData = useMemo(() => (draft?.files ?? []).map((file) => ({ key: file.path, title: file.path, isLeaf: true })), [draft?.files]);
  const updateDraft = (updater: (current: ResultProcessorRevision) => ResultProcessorRevision) => {
    if (readOnly) return;
    setDraft((current) => current ? updater(current) : current);
    setDirty(true);
  };
  const addFile = () => modal.confirm({
    title: '新建 Python 文件',
    content: <Input id="new-result-processor-file" placeholder="checks/project.py" />,
    onOk: () => {
      const input = document.getElementById('new-result-processor-file') as HTMLInputElement | null;
      const path = input?.value.trim();
      if (!path || draft?.files.some((file) => file.path === path)) return Promise.reject(new Error('请输入未使用的相对路径'));
      if (!path.endsWith('.py')) return Promise.reject(new Error('Result Processor 仅允许 Python 文件'));
      updateDraft((current) => ({ ...current, files: [...current.files, { path, content: '', mode: 0o644 }] }));
      setSelectedPath(path);
    },
  });
  const renameFile = () => modal.confirm({
    title: `重命名 ${selectedPath}`,
    content: <Input id="rename-result-processor-file" defaultValue={selectedPath} />,
    onOk: () => {
      const input = document.getElementById('rename-result-processor-file') as HTMLInputElement | null;
      const path = input?.value.trim();
      if (!path || (path !== selectedPath && draft?.files.some((file) => file.path === path))) return Promise.reject(new Error('请输入未使用的相对路径'));
      if (!path.endsWith('.py')) return Promise.reject(new Error('Result Processor 仅允许 Python 文件'));
      updateDraft((current) => ({ ...current, files: current.files.map((file) => file.path === selectedPath ? { ...file, path } : file) }));
      setSelectedPath(path);
    },
  });
  const removeFile = () => {
    if (!draft || !selectedPath || selectedPath === 'processor.py') return;
    modal.confirm({
      title: `删除 ${selectedPath}？`,
      content: '发布前可以继续编辑草稿；processor.py 是固定入口，不能删除。',
      okButtonProps: { danger: true },
      onOk: () => {
        const files = draft.files.filter((file) => file.path !== selectedPath);
        updateDraft((current) => ({ ...current, files }));
        setSelectedPath(files[0]?.path || 'processor.py');
      },
    });
  };
  const fileTree = (
    <div className={styles.tree}>
      <Flex className={styles.treeActions} gap={4} wrap>
        <Button type="text" icon={<FileAddOutlined />} disabled={readOnly} onClick={addFile}>新建</Button>
        <Button type="text" disabled={readOnly || !selected} onClick={renameFile}>重命名</Button>
        <Button type="text" danger icon={<DeleteOutlined />} disabled={readOnly || !selected || selectedPath === 'processor.py'} onClick={removeFile} />
        <Upload accept=".gz,.tgz" showUploadList={false} disabled={readOnly || importArchive.isPending} beforeUpload={(file) => { importArchive.mutate(file as File); return Upload.LIST_IGNORE; }}>
          <Button type="text" icon={<UploadOutlined />} disabled={readOnly || importArchive.isPending}>导入</Button>
        </Upload>
      </Flex>
      <Tree blockNode selectedKeys={[selectedPath]} treeData={treeData} onSelect={(keys) => { if (keys[0]) { setSelectedPath(String(keys[0])); setTreeOpen(false); } }} />
    </div>
  );

  if (item.isLoading) return <div className={styles.page}><Card><Skeleton active paragraph={{ rows: 12 }} /></Card></div>;
  if (item.isError || !processorItem) return <div className={styles.page}><Result status="404" title="Result Processor 不存在或不可访问" extra={<Button onClick={() => navigate('/workforce/result-processors')}>返回列表</Button>} /></div>;
  if (!draft) return <div className={styles.page}><Card><Skeleton active paragraph={{ rows: 12 }} /></Card></div>;

  return <div className={styles.page}>
    <Button type="text" icon={<ArrowLeftOutlined />} onClick={() => { if (!dirty || confirm('存在未保存修改，确定离开？')) navigate('/workforce/result-processors'); }}>Result Processor / {processorItem.name}</Button>
    <Flex className={styles.editorHeader} justify="space-between" align="center" wrap gap={12}>
      <Space orientation="vertical" size={2}>
        <Flex align="center" gap={10} wrap><Title level={2}>{processorItem.name}</Title>{officialProcessor && <Tag color="gold">官方内置 Release</Tag>}<Badge status={readOnly ? 'default' : dirty ? 'warning' : 'success'} text={officialProcessor ? '官方只读' : readOnly ? '已发布 · 只读' : dirty ? '未保存' : '已保存'} /></Flex>
        <Text type="secondary">Revision {draft.revisionNumber} · Python {draft.manifest.runtime.requirement || '镜像固定'} · process(context) 为固定入口</Text>
      </Space>
      <Space wrap>
        {readOnly && !officialProcessor ? <Button type="primary" onClick={() => createRevision.mutate()} loading={createRevision.isPending}>创建新草稿</Button> : !readOnly ? <>
          <Button icon={<SaveOutlined />} type={dirty ? 'primary' : 'default'} disabled={!dirty} loading={save.isPending} onClick={() => save.mutate()}>保存</Button>
          <Button disabled={dirty} loading={validate.isPending} onClick={() => validate.mutate()}>Validate</Button>
          <Button disabled={dirty || !draft.validation?.valid} onClick={() => setFixtureOpen(true)}>Test</Button>
          <Button type="primary" icon={<SendOutlined />} disabled={dirty || !draft.validation?.valid} onClick={() => setPublishOpen(true)}>Publish</Button>
        </> : null}
      </Space>
    </Flex>
    {dirty && <Alert type="warning" showIcon title="存在未保存的修改" description="Validate、Fixture 和 Publish 始终针对服务器已保存的草稿；请先保存，避免测试与编辑内容不一致。" style={{ marginBottom: 16 }} />}
    {!draft.validation?.valid && !dirty && draft.validation?.errors?.length > 0 && <Alert type="error" showIcon title="Validate 未通过" description={draft.validation.errors.join('；')} style={{ marginBottom: 16 }} />}
    <Card styles={{ body: { paddingTop: 0 } }}>
      <Tabs className={styles.editorTabs} items={[
        {
          key: 'source', label: '源码', children: <div className={styles.source}>{fileTree}<div className={styles.editor}>{!screens.lg && <Button icon={<FolderOpenOutlined />} onClick={() => setTreeOpen(true)} style={{ position: 'absolute', zIndex: 2, margin: 8 }}>文件</Button>}{selected ? <Suspense fallback={<Skeleton active />}><MonacoProcessorEditor processorId={processorItem.id} path={selected.path} value={selected.content} readOnly={readOnly} onChange={(content) => updateDraft((current) => ({ ...current, files: current.files.map((file) => file.path === selected.path ? { ...file, content } : file) }))} /></Suspense> : <Result title="请选择或新建 Python 文件" />}</div></div>,
        },
        { key: 'manifest', label: 'Manifest', children: <ManifestForm manifest={draft.manifest} readOnly={readOnly} onChange={(manifest) => updateDraft((current) => ({ ...current, manifest }))} /> },
        { key: 'test', label: '测试', children: <FixtureTab draft={draft} fixture={fixture.data} official={officialProcessor} onOpen={() => setFixtureOpen(true)} /> },
        { key: 'versions', label: `版本 (${processorItem.releases?.length ?? 0})`, children: <VersionsTab item={processorItem} settingDefault={setDefault.isPending} onDefault={(releaseId) => setDefault.mutate(releaseId)} /> },
        { key: 'environment', label: '环境与审计', children: <EnvironmentTab manifest={draft.manifest} latestRelease={processorItem.latestRelease} onBind={() => setEnvironmentOpen(true)} /> },
      ]} />
    </Card>
    <Drawer title="Processor 文件" placement="left" open={treeOpen} onClose={() => setTreeOpen(false)} size="default">{fileTree}</Drawer>
    <Drawer title="精确 Harness Fixture" open={fixtureOpen} onClose={() => setFixtureOpen(false)} size={screens.md ? 'large' : 'default'}>
      <Alert type="info" showIcon title="无 Provider、无网络的镜像 Fixture" description="Fixture 仅在已准入的精确 Harness Image 内运行未发布草稿；不会运行 Agent，也不会使用真实 Provider。" style={{ marginBottom: 16 }} />
      <Form form={fixtureForm} layout="vertical" initialValues={{ evaluationContext: '{\n  "taskId": "fixture",\n  "attemptId": "fixture"\n}', workspaceFiles: '[]', configOverrides: '{}', expectedDecision: 'PASS' }} onFinish={(values) => runFixture.mutate(values)}>
        <Form.Item name="harnessReleaseId" label="Harness Release" rules={[{ required: true, message: '请选择已准入 Harness Release' }]}><Select loading={harnesses.isLoading} options={(harnesses.data ?? []).map((entry) => ({ value: entry.harnessReleaseId, label: `${entry.alias} · ${entry.harnessType} ${entry.harnessVersion} · ${shortDigest(entry.imageDigest)}` }))} /></Form.Item>
        <Form.Item name="expectedDecision" label="预期 decision"><Select allowClear options={decisions.map((decision) => ({ value: decision }))} /></Form.Item>
        <Form.Item name="evaluationContext" label="Evaluation Context JSON" rules={[{ required: true }]}><Input.TextArea rows={5} className={styles.mono} /></Form.Item>
        <Form.Item name="workspaceFiles" label="Workspace 文件 JSON" extra="最多 256 个文件、总计 8 MiB；数组元素必须包含 path 和 content。"><Input.TextArea rows={5} className={styles.mono} /></Form.Item>
        <Form.Item name="configOverrides" label="CONFIG 覆盖 JSON"><Input.TextArea rows={4} className={styles.mono} /></Form.Item>
        <Button type="primary" htmlType="submit" loading={runFixture.isPending}>提交 Fixture</Button>
      </Form>
      {fixtureRunId && <FixtureResultCard run={fixture.data} loading={fixture.isLoading || fixture.isFetching} />}
    </Drawer>
    <Modal title="发布不可变 Result Processor Release" open={publishOpen} footer={null} onCancel={() => setPublishOpen(false)}>
      <Alert type="warning" showIcon title="发布后不可修改" description="服务端会 canonicalize archive、回读校验 digest；Task 只冻结 Release 与 digest，不会跟随草稿或默认绑定漂移。" />
      <Form layout="vertical" style={{ marginTop: 18 }} onFinish={(values) => publish.mutate(values)}>
        <Form.Item name="version" label="语义版本" rules={[{ required: true }, { pattern: /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/, message: '例如 1.0.0' }]}><Input placeholder="1.0.0" /></Form.Item>
        <Flex justify="flex-end" gap={8}><Button onClick={() => setPublishOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={publish.isPending}>发布</Button></Flex>
      </Form>
    </Modal>
    <Modal title="绑定 Result Processor 环境" open={environmentOpen} footer={null} destroyOnHidden onCancel={() => setEnvironmentOpen(false)}>
      <Alert type="info" showIcon title="环境绑定面向已发布 Release" description="CONFIG 可以提供非敏感配置；SECRET 只引用 Framework Config 的 ref/version，在 Evaluation 开始时短暂注入，页面不会显示其值。" style={{ marginBottom: 16 }} />
      <Form form={environmentForm} layout="vertical" initialValues={{ type: 'CONFIG', phase: 'EVALUATE', taskOverrideAllowed: false, networkPolicy: 'NONE', configText: '{}' }} onFinish={(values) => bindEnvironment.mutate({ name: values.name, type: values.type, phase: values.phase, taskOverrideAllowed: values.taskOverrideAllowed, networkPolicy: values.networkPolicy, config: values.type === 'CONFIG' ? parseJSONObject(values.configText, 'CONFIG') : undefined, secretRef: values.type === 'SECRET' ? values.secretRef : undefined, secretVersion: values.type === 'SECRET' ? values.secretVersion : undefined })}>
        <Form.Item name="name" label="变量名" rules={[{ required: true }, { pattern: /^[A-Z_][A-Z0-9_]*$/, message: '使用大写字母、数字和下划线' }]}><Input placeholder="QUALITY_GATE_TOKEN" /></Form.Item>
        <Flex gap={12} wrap><Form.Item name="type" label="类型" style={{ flex: 1 }}><Select options={[{ value: 'CONFIG' }, { value: 'SECRET' }]} /></Form.Item><Form.Item name="phase" label="阶段" style={{ flex: 1 }}><Select options={[{ value: 'EVALUATE', label: 'EVALUATE（本能力域）' }]} /></Form.Item></Flex>
        <Form.Item noStyle shouldUpdate={(prev, current) => prev.type !== current.type}>{({ getFieldValue }) => getFieldValue('type') === 'SECRET' ? <><Form.Item name="secretRef" label="Framework Config 引用" rules={[{ required: true }]}><Input placeholder="group.key" /></Form.Item><Form.Item name="secretVersion" label="Secret Version" rules={[{ required: true }]}><Input placeholder="例如 12" /></Form.Item></> : <Form.Item name="configText" label="CONFIG JSON" rules={[{ required: true }]}><Input.TextArea rows={4} className={styles.mono} /></Form.Item>}</Form.Item>
        <Form.Item name="taskOverrideAllowed" valuePropName="checked"><Checkbox>允许 Task 参数覆盖</Checkbox></Form.Item>
        <Form.Item name="networkPolicy" label="网络策略"><Select options={[{ value: 'NONE', label: 'NONE（默认）' }]} /></Form.Item>
        <Flex justify="flex-end" gap={8}><Button onClick={() => setEnvironmentOpen(false)}>取消</Button><Button type="primary" htmlType="submit" loading={bindEnvironment.isPending} disabled={!processorItem.latestRelease}>保存绑定</Button></Flex>
      </Form>
    </Modal>
  </div>;
}

function ManifestForm({ manifest, readOnly, onChange }: { manifest: ResultProcessorManifest; readOnly: boolean; onChange: (manifest: ResultProcessorManifest) => void }) {
  const [parametersText, setParametersText] = useState(() => JSON.stringify(manifest.parametersSchema ?? { type: 'object', properties: {} }, null, 2));
  const [parametersError, setParametersError] = useState('');
  useEffect(() => { setParametersText(JSON.stringify(manifest.parametersSchema ?? { type: 'object', properties: {} }, null, 2)); }, [manifest.parametersSchema]);
  const update = (patch: Partial<ResultProcessorManifest>) => onChange({ ...manifest, ...patch });
  return <Card size="small" title="seven.result-processor" className={styles.formCard}>
    <Form layout="vertical" disabled={readOnly}>
      <Flex gap={16} wrap>
        <Form.Item label="名称" style={{ flex: '1 1 280px' }}><Input value={manifest.name} onChange={(event) => update({ name: event.target.value })} /></Form.Item>
        <Form.Item label="Python Runtime" style={{ flex: '0 1 240px' }}><Input value={manifest.runtime.requirement || '>=3.11 <3.14'} disabled /></Form.Item>
        <Form.Item label="最大自动修正轮次" style={{ flex: '0 1 190px' }}><InputNumber min={1} max={5} value={manifest.maxEvaluationCycles ?? 3} onChange={(value) => update({ maxEvaluationCycles: value ?? 3 })} /></Form.Item>
      </Flex>
      <Flex gap={16} wrap>
        <Form.Item label="Continuation Policy" style={{ flex: '1 1 260px' }}><Select value={manifest.continuationPolicy || 'CHECKPOINT_REQUIRED'} options={[{ value: 'CHECKPOINT_REQUIRED', label: 'CHECKPOINT_REQUIRED（固定）' }]} disabled /></Form.Item>
        <Form.Item label="Delivery Policy" style={{ flex: '1 1 260px' }}><Select value={manifest.deliveryPolicy || 'OPTIONAL'} options={[{ value: 'OPTIONAL', label: 'OPTIONAL · 不调用 deliver()' }, { value: 'DECLARED', label: 'DECLARED · Fixture 校验可选 deliver()' }]} onChange={(deliveryPolicy) => update({ deliveryPolicy, capabilities: deliveryPolicy === 'DECLARED' ? ['PROCESSOR_DELIVER'] : [] })} /></Form.Item>
      </Flex>
      <Form.Item label="Task 参数 Schema" validateStatus={parametersError ? 'error' : undefined} help={parametersError || '仅用于 Task Create 表单；必须为 JSON object。'}>
        <Input.TextArea value={parametersText} rows={8} className={styles.mono} onChange={(event) => {
          const value = event.target.value;
          setParametersText(value);
          try {
            const parsed = JSON.parse(value) as unknown;
            if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error('参数 Schema 必须是 JSON object');
            setParametersError('');
            update({ parametersSchema: parsed as Record<string, unknown> });
          } catch (error) {
            setParametersError(error instanceof Error ? error.message : '无效 JSON');
          }
        }} />
      </Form.Item>
      <Alert type="info" showIcon title="固定入口" description="processor.py 必须定义 process(context)。可选 deliver(context) 只在声明 DECLARED 后参与无网络 Fixture 校验，正式交付不在本能力域执行。" />
    </Form>
  </Card>;
}

function FixtureTab({ draft, fixture, official, onOpen }: { draft: ResultProcessorRevision; fixture?: Awaited<ReturnType<typeof getResultProcessorFixtureRun>>; official: boolean; onOpen: () => void }) {
  return <Space orientation="vertical" size={16} style={{ width: '100%' }}>
    <Alert type="info" showIcon title="精确镜像 Fixture" description={official ? '官方内置 Release 已作为平台资产完成准入，项目可直接使用当前标准规则。' : 'Validate 只检查资产与合同；Fixture 才会在已准入 Agent 运行环境中验证 Python 模块、SDK 和隔离边界。'} action={<Button disabled={official || !draft.validation?.valid || draft.status === 'PUBLISHED'} onClick={onOpen}>打开 Fixture</Button>} />
    {fixture && <FixtureResultCard run={fixture} loading={false} />}
  </Space>;
}

function releaseAsReadOnlyRevision(release?: NonNullable<ResultProcessor['latestRelease']>): ResultProcessorRevision | undefined {
  if (!release) return undefined;
  return {
    id: release.revisionId,
    processorId: release.processorId,
    tenantId: release.tenantId,
    scopeType: release.scopeType,
    revisionNumber: '1',
    draftVersion: '1',
    status: 'PUBLISHED',
    manifest: release.manifest,
    files: release.files,
    validation: { valid: true, errors: [], warnings: [] },
    createdAt: release.publishedAt,
    updatedAt: release.publishedAt,
  };
}

function FixtureResultCard({ run, loading }: { run?: Awaited<ReturnType<typeof getResultProcessorFixtureRun>>; loading: boolean }) {
  if (!run && loading) return <Skeleton active paragraph={{ rows: 4 }} />;
  if (!run) return null;
  const contract = run.resultContract ? JSON.stringify(run.resultContract, null, 2) : '';
  return <Card size="small" title="Fixture 结果" extra={<Tag color={isFixtureSucceeded(run.status) ? 'green' : isFixtureTerminal(run.status) ? 'red' : 'blue'}>{run.status}</Tag>}>
    <Space orientation="vertical" size={8} style={{ width: '100%' }}>
      <DescriptionsMini rows={[
        ['Run ID', run.id], ['Harness Digest', run.harnessImageDigest], ['Worker', run.workerId || '等待领取'], ['Reason Code', run.reasonCode || '—'],
      ]} />
      {run.diagnostics && <Alert type="warning" showIcon title="安全诊断" description={run.diagnostics} />}
      {contract && <pre className={styles.safePre}>{contract}</pre>}
    </Space>
  </Card>;
}

function VersionsTab({ item, settingDefault, onDefault }: { item: ResultProcessor; settingDefault: boolean; onDefault: (releaseId: string) => void }) {
  const columns: TableColumnsType<NonNullable<ResultProcessor['releases']>[number]> = [
    { title: 'Release', dataIndex: 'version', width: 130, render: (value: string) => <Tag color="green">v{value}</Tag> },
    { title: 'Digest', dataIndex: 'archiveSha256', width: 260, ellipsis: true, render: (value: string) => <Text code copyable>{shortDigest(value)}</Text> },
    { title: '发布时间', dataIndex: 'publishedAt', width: 190, render: (value: string) => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '—' },
    { title: '操作', width: 160, render: (_, release) => <Button type="link" loading={settingDefault} onClick={() => onDefault(release.id)}>设为 default</Button> },
  ];
  return <Space orientation="vertical" size={16} style={{ width: '100%' }}>
    <Alert type="info" showIcon icon={<DiffOutlined />} title="Release 不可变" description="default 只影响之后创建的 Task；已排队、重试或接管中的 Task 始终消费创建时冻结的 Processor Release 与 digest。" />
    <Table rowKey="id" size="small" columns={columns} dataSource={item.releases ?? []} pagination={false} scroll={{ x: 700 }} locale={{ emptyText: '尚未发布 Release' }} />
  </Space>;
}

function EnvironmentTab({ manifest, latestRelease, onBind }: { manifest: ResultProcessorManifest; latestRelease?: { id: string; version: string }; onBind: () => void }) {
  const columns: TableColumnsType<NonNullable<ResultProcessorManifest['environment']>[number]> = [
    { title: '变量名', dataIndex: 'name', width: 220, render: (value: string) => <Text code>{value}</Text> },
    { title: '类型', dataIndex: 'type', width: 120, render: (value: string) => <Tag color={value === 'SECRET' ? 'orange' : 'blue'}>{value}</Tag> },
    { title: '阶段', dataIndex: 'phase', width: 130, render: (value: string) => <Tag>{value}</Tag> },
    { title: 'Task 可覆盖', dataIndex: 'taskOverrideAllowed', width: 130, render: (value: boolean) => value ? '允许' : '不允许' },
    { title: 'Secret Ref', dataIndex: 'secretRef', width: 230, ellipsis: true, render: (value: string) => value || '—' },
  ];
  return <Space orientation="vertical" size={16} style={{ width: '100%' }}>
    <Alert type="info" showIcon title="CONFIG / SECRET 边界" description="SECRET 只以 Framework Config 的 ref/version 冻结。每轮 Evaluation 在 Agent 进程组确认停止后才短暂物化，处理完成即清理；秘密值不进入 Docker 环境、参数、日志、Web 或 Artifact。" action={<Button disabled={!latestRelease} onClick={onBind}>{latestRelease ? `绑定 v${latestRelease.version} 环境` : '先发布 Release'}</Button>} />
    <Table className={styles.environmentTable} rowKey="name" size="small" columns={columns} dataSource={manifest.environment ?? []} pagination={false} scroll={{ x: 760 }} locale={{ emptyText: '此 Manifest 未声明环境变量' }} />
  </Space>;
}

function DescriptionsMini({ rows }: { rows: Array<[string, string]> }) {
  return <Table size="small" pagination={false} showHeader={false} rowKey={([key]) => key} columns={[{ dataIndex: 0, width: 150, render: (value) => <Text type="secondary">{value}</Text> }, { dataIndex: 1, render: (value) => <Text code copyable>{value}</Text> }]} dataSource={rows} />;
}

function parseJSONObject(value: string, label: string): Record<string, unknown> {
  const parsed = JSON.parse(value) as unknown;
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(`${label}必须是 JSON object`);
  return parsed as Record<string, unknown>;
}

function parseWorkspaceFiles(value: string): Array<{ path: string; content: string }> {
  const parsed = JSON.parse(value) as unknown;
  if (!Array.isArray(parsed)) throw new Error('Workspace 文件必须是 JSON array');
  return parsed.map((item) => {
    if (!item || typeof item !== 'object' || typeof (item as { path?: unknown }).path !== 'string' || typeof (item as { content?: unknown }).content !== 'string') throw new Error('Workspace 文件必须包含 path 和 content');
    return { path: (item as { path: string }).path, content: (item as { content: string }).content };
  });
}

function isFixtureTerminal(status?: string) {
  return status === 'SUCCEEDED' || status === 'FAILED' || status === 'CANCELLED';
}

function isFixtureSucceeded(status?: string) {
  return status === 'SUCCEEDED' || status === 'PASSED';
}

function shortDigest(value?: string) {
  return value ? `${value.slice(0, 18)}…` : '—';
}
