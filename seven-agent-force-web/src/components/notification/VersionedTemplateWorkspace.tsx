'use client';

import React, { useMemo, useRef, useState } from 'react';
import { Button, Drawer, Form, Input, InputNumber, Modal, Space, Switch, Tag, Typography, message } from 'antd';
import { EditableProTable, ProTable } from '@ant-design/pro-components';
import type { ActionType, ProColumns } from '@ant-design/pro-components';
import { EyeOutlined, PlusOutlined } from '@ant-design/icons';
import {
  createVersionedNotificationTemplate,
  createVersionedNotificationTemplateDraft,
  getVersionedNotificationTemplate,
  listVersionedNotificationTemplates,
  previewVersionedNotificationTemplate,
  publishVersionedNotificationTemplate,
  saveVersionedNotificationTemplateDraft,
  type TemplateRevision,
  type TemplateRevisionDraftInput,
  type TemplateRevisionPreviewResponse,
  type TemplateRevisionVariable,
  type TemplateVariableType,
  type VersionedNotificationTemplate,
} from '@/api/notificationController';

type EditableVariable = TemplateRevisionVariable & { key: string };
type PreviewValue = {
  key: string;
  name: string;
  type: TemplateVariableType;
  required: boolean;
  value?: string | number | boolean;
};

type EditorValues = TemplateRevisionDraftInput & {
  templateCode: string;
};

const VARIABLE_TYPE_OPTIONS = [
  { value: 'STRING', label: '文本' },
  { value: 'NUMBER', label: '数字' },
  { value: 'BOOLEAN', label: '是/否' },
  { value: 'DATETIME', label: '时间' },
];

const VARIABLE_CLASSIFICATION_OPTIONS = [
  { value: 'PUBLIC', label: '公开' },
  { value: 'SENSITIVE', label: '敏感' },
];

function rowKey(variable: Partial<TemplateRevisionVariable>, index: number) {
  return `${variable.name || 'variable'}-${index}`;
}

function draftFromDefinition(item: VersionedNotificationTemplate, selectedRevision?: TemplateRevision): EditorValues {
	const revision = selectedRevision || item.currentDraft || item.currentPublished || item.revisions?.[0];
	return {
		templateCode: item.templateCode,
		templateName: item.templateName,
		locale: item.locale || 'zh-CN',
		subjectTemplate: revision?.subjectTemplate || '',
		textTemplate: revision?.textTemplate || '',
		htmlTemplate: revision?.htmlTemplate || '',
		markdownTemplate: revision?.markdownTemplate || '',
		primaryActionLabelTemplate: revision?.primaryActionLabelTemplate || '',
		variables: (revision?.variables || []).map((variable, index) => ({ ...variable, key: rowKey(variable, index) })),
	};
}

function defaultEditorValues(): EditorValues {
	return {
		templateCode: '',
		templateName: '',
		locale: 'zh-CN',
		subjectTemplate: '',
		textTemplate: '',
		htmlTemplate: '',
		markdownTemplate: '',
		primaryActionLabelTemplate: '',
		variables: [],
	};
}

function normalizeVariableRows(rows: EditableVariable[] | undefined): TemplateRevisionVariable[] {
	return (rows || []).map((variable) => {
		const type = variable.type || 'STRING';
		const classification = variable.classification || 'PUBLIC';
		return {
			name: variable.name?.trim(),
			type,
			required: Boolean(variable.required),
			classification,
			maxLength: type === 'STRING' ? Number(variable.maxLength || 0) : undefined,
			sampleValue: classification === 'SENSITIVE' ? undefined : normalizeValue(type, variable.sampleValue),
		};
	});
}

function normalizeValue(type: TemplateVariableType, value: unknown): string | number | boolean | undefined {
	if (value === undefined || value === null || value === '') {
		return undefined;
	}
	if (type === 'NUMBER') {
		return typeof value === 'number' ? value : Number(value);
	}
	if (type === 'BOOLEAN') {
		return value === true || value === 'true';
	}
	return String(value);
}

function renderValueInput(type: TemplateVariableType, value?: unknown, onChange?: (next: unknown) => void) {
	if (type === 'NUMBER') {
		return <InputNumber style={{ width: '100%' }} value={typeof value === 'number' ? value : undefined} onChange={onChange} />;
	}
	if (type === 'BOOLEAN') {
		return <Switch checked={value === true || value === 'true'} onChange={onChange} />;
	}
	return <Input value={value == null ? '' : String(value)} onChange={(event) => onChange?.(event.target.value)} placeholder={type === 'DATETIME' ? '2026-07-27T10:00:00Z' : '示例值'} />;
}

function revisionLabel(revision?: TemplateRevision) {
	return revision ? `v${revision.revisionNo}` : '—';
}

function variableFromRenderContext(context: unknown) {
	if (!context || typeof context !== 'object') {
		return undefined;
	}
	return (context as { record?: Partial<EditableVariable> }).record;
}

export function VersionedTemplateWorkspace({ canEdit }: { canEdit: boolean }) {
	const actionRef = useRef<ActionType>(undefined);
	const [editorForm] = Form.useForm<EditorValues>();
	const [editorOpen, setEditorOpen] = useState(false);
	const [previewOpen, setPreviewOpen] = useState(false);
	const [activeDefinition, setActiveDefinition] = useState<VersionedNotificationTemplate | null>(null);
	const [viewingRevision, setViewingRevision] = useState<TemplateRevision | null>(null);
	const [previewValues, setPreviewValues] = useState<PreviewValue[]>([]);
	const [previewResult, setPreviewResult] = useState<TemplateRevisionPreviewResponse | null>(null);
	const [submitting, setSubmitting] = useState(false);
	const [previewing, setPreviewing] = useState(false);
	const variables = (Form.useWatch('variables', editorForm) || []) as EditableVariable[];
	const activeDraft = activeDefinition?.currentDraft;
	const activePublished = activeDefinition?.currentPublished;
	const currentRevision = activeDraft || activePublished;
	const displayedRevision = viewingRevision || currentRevision;
	const viewingHistoricalRevision = Boolean(viewingRevision && viewingRevision.id !== currentRevision?.id);
	const readOnly = Boolean(activeDefinition && displayedRevision && (!activeDraft || viewingHistoricalRevision));
	const revisionHistory = activeDefinition?.revisions || [];

	const variableColumns = useMemo<ProColumns<EditableVariable>[]>(
		() => [
			{
				title: '名称',
				dataIndex: 'name',
				formItemProps: { rules: [{ required: true, whitespace: true, message: '请输入变量名称' }] },
			},
			{
				title: '类型',
				dataIndex: 'type',
				valueType: 'select',
				valueEnum: Object.fromEntries(VARIABLE_TYPE_OPTIONS.map((item) => [item.value, { text: item.label }])),
			},
			{
				title: '必填',
				dataIndex: 'required',
				valueType: 'switch',
				render: (_, record) => (record.required ? '是' : '否'),
			},
			{
				title: '最大长度',
				dataIndex: 'maxLength',
				render: (_, record) => (record.type === 'STRING' ? record.maxLength || '—' : '—'),
				renderFormItem: (_: unknown, context: unknown) =>
					variableFromRenderContext(context)?.type === 'STRING' ? <InputNumber min={1} max={4096} style={{ width: '100%' }} placeholder="1-4096" /> : <Typography.Text type="secondary">仅文本</Typography.Text>,
			},
			{
				title: '分类',
				dataIndex: 'classification',
				valueType: 'select',
				valueEnum: Object.fromEntries(VARIABLE_CLASSIFICATION_OPTIONS.map((item) => [item.value, { text: item.label }])),
			},
			{
				title: '示例',
				dataIndex: 'sampleValue',
				render: (_, record) => (record.classification === 'SENSITIVE' ? '不保存' : String(record.sampleValue ?? '—')),
				renderFormItem: (_: unknown, context: unknown) => {
					const record = variableFromRenderContext(context);
					if (record?.classification === 'SENSITIVE') {
						return <Typography.Text type="secondary">敏感变量不保存示例</Typography.Text>;
					}
					return renderValueInput((record?.type || 'STRING') as TemplateVariableType);
				},
			},
		],
		[],
	);

	const previewColumns = useMemo<ProColumns<PreviewValue>[]>(
		() => [
			{ title: '变量', dataIndex: 'name', editable: false },
			{ title: '类型', dataIndex: 'type', editable: false, valueEnum: Object.fromEntries(VARIABLE_TYPE_OPTIONS.map((item) => [item.value, { text: item.label }])) },
			{ title: '必填', dataIndex: 'required', editable: false, render: (_, record) => (record.required ? '是' : '否') },
			{
				title: '本次预览值',
				dataIndex: 'value',
				renderFormItem: (_: unknown, context: unknown) =>
					renderValueInput((variableFromRenderContext(context)?.type || 'STRING') as TemplateVariableType),
			},
		],
		[],
	);

	const columns: ProColumns<VersionedNotificationTemplate>[] = [
			{ title: '模板编码', dataIndex: 'templateCode', width: 220, fixed: 'left' },
			{ title: '模板名称', dataIndex: 'templateName', width: 180 },
			{ title: '草稿', width: 90, search: false, render: (_, record) => (record.currentDraft ? <Tag color="blue">{revisionLabel(record.currentDraft)}</Tag> : '—') },
			{ title: '已发布', width: 100, search: false, render: (_, record) => (record.currentPublished ? <Tag color="green">{revisionLabel(record.currentPublished)}</Tag> : '—') },
			{ title: '更新时间', dataIndex: 'updateTime', valueType: 'dateTime', width: 180, search: false },
			{
				title: '操作',
				valueType: 'option',
				fixed: 'right',
				width: 180,
				render: (_, record) => (
					<Space>
						<Button type="link" icon={<EyeOutlined />} onClick={() => void openDefinition(record.templateCode)}>
							查看
						</Button>
						{canEdit && !record.currentDraft && record.currentPublished && (
							<Button type="link" onClick={() => void createNextDraft(record.templateCode)}>
								新建版本
							</Button>
						)}
					</Space>
				),
			},
	];

	function applyDefinition(item: VersionedNotificationTemplate) {
		setActiveDefinition(item);
		setViewingRevision(null);
		editorForm.setFieldsValue(draftFromDefinition(item));
		setEditorOpen(true);
	}

	function showRevision(revision: TemplateRevision) {
		if (!activeDefinition) {
			return;
		}
		if (revision.id === currentRevision?.id) {
			setViewingRevision(null);
			editorForm.setFieldsValue(draftFromDefinition(activeDefinition));
			return;
		}
		setViewingRevision(revision);
		editorForm.setFieldsValue(draftFromDefinition(activeDefinition, revision));
	}

	async function openDefinition(templateCode: string) {
		try {
			applyDefinition(await getVersionedNotificationTemplate(templateCode));
		} catch (error) {
			message.error((error as Error).message || '加载模板详情失败');
		}
	}

	function openCreate() {
		setActiveDefinition(null);
		setViewingRevision(null);
		editorForm.setFieldsValue(defaultEditorValues());
		setEditorOpen(true);
	}

	async function createNextDraft(templateCode: string) {
		setSubmitting(true);
		try {
			const result = await createVersionedNotificationTemplateDraft(templateCode);
			message.success('已新建草稿版本');
			applyDefinition(result);
			actionRef.current?.reload();
		} catch (error) {
			message.error((error as Error).message || '新建模板版本失败');
		} finally {
			setSubmitting(false);
		}
	}

	async function persistDraft(): Promise<VersionedNotificationTemplate | null> {
		const values = await editorForm.validateFields();
		const draft: TemplateRevisionDraftInput = {
			templateName: values.templateName.trim(),
			locale: values.locale?.trim() || 'zh-CN',
			subjectTemplate: values.subjectTemplate || '',
			textTemplate: values.textTemplate || '',
			htmlTemplate: values.htmlTemplate || '',
			markdownTemplate: values.markdownTemplate || '',
			primaryActionLabelTemplate: values.primaryActionLabelTemplate?.trim() || '',
			variables: normalizeVariableRows(values.variables as EditableVariable[]),
		};
		if (!activeDefinition) {
			return createVersionedNotificationTemplate({ templateCode: values.templateCode.trim(), draft });
		}
		if (!activeDraft) {
			message.info('已发布版本不可修改，请先新建版本');
			return null;
		}
		return saveVersionedNotificationTemplateDraft(activeDraft.id, { expectedVersion: activeDraft.revisionVersion, draft });
	}

	async function saveDraft() {
		setSubmitting(true);
		try {
			const result = await persistDraft();
			if (!result) {
				return;
			}
			message.success('草稿已保存');
			applyDefinition(result);
			actionRef.current?.reload();
		} catch (error) {
			message.error((error as Error).message || '保存草稿失败');
		} finally {
			setSubmitting(false);
		}
	}

	function openPreview() {
		const rows = normalizeVariableRows(variables).map((variable, index) => ({
			key: rowKey(variable, index),
			name: variable.name,
			type: variable.type,
			required: variable.required,
			value: variable.classification === 'SENSITIVE' ? undefined : normalizeValue(variable.type, variable.sampleValue),
		}));
		setPreviewValues(rows);
		setPreviewResult(null);
		setPreviewOpen(true);
	}

	async function runPreview() {
		setPreviewing(true);
		try {
			const values = await editorForm.validateFields();
			const draft: TemplateRevisionDraftInput = {
				templateName: values.templateName.trim(),
				locale: values.locale?.trim() || 'zh-CN',
				subjectTemplate: values.subjectTemplate || '',
				textTemplate: values.textTemplate || '',
				htmlTemplate: values.htmlTemplate || '',
				markdownTemplate: values.markdownTemplate || '',
				primaryActionLabelTemplate: values.primaryActionLabelTemplate?.trim() || '',
				variables: normalizeVariableRows(values.variables as EditableVariable[]),
			};
			const requestValues: Record<string, unknown> = {};
			for (const row of previewValues) {
				const value = normalizeValue(row.type, row.value);
				if (value !== undefined) {
					requestValues[row.name] = value;
				}
			}
			setPreviewResult(await previewVersionedNotificationTemplate({ draft, variables: requestValues }));
		} catch (error) {
			message.error((error as Error).message || '预览失败');
		} finally {
			setPreviewing(false);
		}
	}

	async function publishDraft() {
		setSubmitting(true);
		try {
			const saved = await persistDraft();
			const draft = saved?.currentDraft;
			if (!saved || !draft) {
				return;
			}
			const published = await publishVersionedNotificationTemplate(draft.id, draft.revisionVersion);
			message.success('模板已发布');
			applyDefinition(published);
			actionRef.current?.reload();
		} catch (error) {
			message.error((error as Error).message || '发布模板失败');
		} finally {
			setSubmitting(false);
		}
	}

	return (
		<Space orientation="vertical" size={16} style={{ width: '100%' }}>
			<ProTable<VersionedNotificationTemplate>
				rowKey="templateCode"
				actionRef={actionRef}
				columns={columns}
				scroll={{ x: 1100 }}
				request={async (params) => {
					const result = await listVersionedNotificationTemplates(params);
					return { data: result.records, success: true, total: result.total };
				}}
				toolBarRender={() => [
					canEdit && (
						<Button key="create" type="primary" icon={<PlusOutlined />} onClick={openCreate}>
							新建模板
						</Button>
					),
				]}
			/>

			<Drawer
				title={activeDefinition ? activeDefinition.templateName : '新建消息模板'}
				size="large"
				open={editorOpen}
				onClose={() => setEditorOpen(false)}
				extra={
					!readOnly && canEdit ? (
						<Space>
							<Button onClick={openPreview}>预览</Button>
							<Button loading={submitting} onClick={() => void saveDraft()}>
								保存草稿
							</Button>
							<Button type="primary" loading={submitting} onClick={() => void publishDraft()}>
								发布
							</Button>
						</Space>
			) : activeDefinition && canEdit && !activeDraft ? (
				<Button type="primary" loading={submitting} onClick={() => void createNextDraft(activeDefinition.templateCode)}>
							新建版本
						</Button>
					) : null
				}
			>
				{revisionHistory.length > 1 && (
					<Space wrap size={[8, 8]} style={{ marginBottom: 16 }}>
						<Typography.Text type="secondary">版本记录</Typography.Text>
						{revisionHistory.map((revision) => (
							<Button
								key={revision.id}
								size="small"
								type={displayedRevision?.id === revision.id ? 'primary' : 'default'}
								onClick={() => showRevision(revision)}
							>
								{revisionLabel(revision)}
							</Button>
						))}
						{viewingHistoricalRevision && currentRevision && (
							<Button size="small" onClick={() => showRevision(currentRevision)}>
								返回当前版本
							</Button>
						)}
					</Space>
				)}
				{readOnly && (
					<div style={{ marginBottom: 16 }}>
						<Tag color={viewingHistoricalRevision ? 'default' : 'green'}>
							{viewingHistoricalRevision ? '历史版本' : '已发布'}
						</Tag>
					</div>
				)}
				<Form form={editorForm} layout="vertical">
					<Space size={16} wrap>
						<Form.Item name="templateCode" label="模板编码" rules={[{ required: true, whitespace: true, message: '请输入模板编码' }]}>
							<Input disabled={Boolean(activeDefinition)} style={{ width: 280 }} placeholder="account_notice" />
						</Form.Item>
						<Form.Item name="templateName" label="模板名称" rules={[{ required: true, whitespace: true, message: '请输入模板名称' }]}>
							<Input disabled={readOnly || !canEdit} style={{ width: 280 }} placeholder="账户提醒" />
						</Form.Item>
						<Form.Item name="locale" label="语言" rules={[{ required: true, whitespace: true }]}>
							<Input disabled={readOnly || !canEdit} style={{ width: 140 }} placeholder="zh-CN" />
						</Form.Item>
					</Space>
					<Form.Item name="subjectTemplate" label="消息标题">
						<Input disabled={readOnly || !canEdit} placeholder="{{.title}}" />
					</Form.Item>
					<Form.Item name="textTemplate" label="消息内容">
						<Input.TextArea disabled={readOnly || !canEdit} rows={5} placeholder="填写消息内容" />
					</Form.Item>
					<Form.Item name="htmlTemplate" label="HTML 内容">
						<Input.TextArea disabled={readOnly || !canEdit} rows={4} />
					</Form.Item>
					<Form.Item name="markdownTemplate" label="Markdown 内容">
						<Input.TextArea disabled={readOnly || !canEdit} rows={4} />
					</Form.Item>
					<Form.Item
						name="primaryActionLabelTemplate"
						label="按钮文字（可选）"
						extra="例如“查看任务”。点击后打开对应页面，跳转地址由系统生成。"
						rules={[{ max: 32, message: '按钮文字最多 32 个字符' }]}
					>
						<Input disabled={readOnly || !canEdit} maxLength={32} placeholder="查看任务" />
					</Form.Item>
					<Typography.Text strong>变量</Typography.Text>
					<Typography.Paragraph type="secondary" style={{ marginTop: 4 }}>
						在内容中使用 {'{{.变量名}}'} 插入变量。
					</Typography.Paragraph>
					<EditableProTable<EditableVariable>
						rowKey="key"
						value={variables}
						onChange={(rows) => editorForm.setFieldValue('variables', rows || [])}
						columns={variableColumns}
						search={false}
						options={false}
						toolBarRender={false}
						pagination={false}
						recordCreatorProps={
							readOnly || !canEdit
								? false
								: {
									record: () => ({ key: `variable-${Date.now()}`, name: '', type: 'STRING', required: false, maxLength: 256, classification: 'PUBLIC' }),
								}
						}
						editable={
							readOnly || !canEdit
								? undefined
								: {
									type: 'multiple',
									editableKeys: variables.map((item) => item.key),
									actionRender: () => [],
									onValuesChange: (_, rows) => editorForm.setFieldValue('variables', rows || []),
								}
						}
					/>
				</Form>
			</Drawer>

			<Modal
				title="预览模板"
				open={previewOpen}
				onCancel={() => setPreviewOpen(false)}
				onOk={() => void runPreview()}
				okText="生成预览"
				confirmLoading={previewing}
				width={720}
			>
				<Typography.Paragraph type="secondary">预览只显示效果，不会发送消息。</Typography.Paragraph>
				<EditableProTable<PreviewValue>
					rowKey="key"
					value={previewValues}
					onChange={(rows) => setPreviewValues((rows || []) as PreviewValue[])}
					columns={previewColumns}
					search={false}
					options={false}
					toolBarRender={false}
					pagination={false}
					recordCreatorProps={false}
					editable={{ type: 'multiple', editableKeys: previewValues.map((item) => item.key), actionRender: () => [], onValuesChange: (_, rows) => setPreviewValues(rows as PreviewValue[]) }}
				/>
				{previewResult && (
					<Space orientation="vertical" size={8} style={{ width: '100%', marginTop: 16 }}>
						<Typography.Text strong>标题</Typography.Text>
						<Typography.Paragraph>{previewResult.subject || '—'}</Typography.Paragraph>
						{previewResult.text && <><Typography.Text strong>文本</Typography.Text><Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{previewResult.text}</Typography.Paragraph></>}
						{previewResult.markdown && <><Typography.Text strong>Markdown</Typography.Text><Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{previewResult.markdown}</Typography.Paragraph></>}
						{previewResult.html && <><Typography.Text strong>HTML 源内容</Typography.Text><Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{previewResult.html}</Typography.Paragraph></>}
						{previewResult.primaryActionLabel && <><Typography.Text strong>按钮</Typography.Text><Typography.Paragraph>{previewResult.primaryActionLabel}</Typography.Paragraph></>}
					</Space>
				)}
			</Modal>

		</Space>
	);
}
