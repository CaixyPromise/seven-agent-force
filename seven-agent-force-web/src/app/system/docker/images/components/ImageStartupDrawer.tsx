'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { EditableProTable } from '@ant-design/pro-components';
import type { ProColumns } from '@ant-design/pro-components';
import {
  AutoComplete,
  Button,
  Checkbox,
  Drawer,
  Form,
  Input,
  InputNumber,
  Select,
  Space,
  Spin,
  Switch,
  Tabs,
  message,
} from 'antd';
import { parse as parseYaml, stringify as stringifyYaml } from 'yaml';
import {
  createDockerContainerFromImage,
  getDockerRegistries,
  getDockerImageStartupPreview,
  getDockerRepositories,
  getLocalDockerImages,
  previewDockerCompose,
  upDockerCompose,
  type DockerComposeUpRequest,
  type DockerContainerCreateRequest,
  type DockerImageStartupPreview,
  type DockerImageView,
  type DockerKeyValueCommand,
  type DockerPortBindingCommand,
  type DockerResourceLimitCommand,
  type DockerVolumeBindingCommand,
} from '@/api/dockerController';

interface ImageStartupDrawerProps {
  open: boolean;
  image: DockerImageView | null;
  onClose: () => void;
  onStarted?: () => void;
}

interface ComposeServiceForm {
  key: string;
  name: string;
  image?: string;
  containerName?: string;
  entrypoint?: string[];
  command?: string[];
  environment: DockerKeyValueCommand[];
  portBindings: DockerPortBindingCommand[];
  volumeBindings: DockerVolumeBindingCommand[];
  labels: DockerKeyValueCommand[];
  workingDir?: string;
  user?: string;
  networkMode?: string;
  privileged?: boolean;
  capAdd?: string[];
  capDrop?: string[];
  restartPolicy?: string;
  tty?: boolean;
  stdinOpen?: boolean;
  publishAllPorts?: boolean;
  resourceLimits?: DockerResourceLimitCommand;
  dependsOn?: string[];
}

interface ComposeFormModel {
  projectName: string;
  services: ComposeServiceForm[];
}

type EditableRow<T> = T & { _rowKey: string };

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function toPairs(values?: DockerKeyValueCommand[]) {
  return values?.length ? values : [{ key: '', value: '' }];
}

function toPorts(values?: DockerPortBindingCommand[]) {
  return values?.length ? values : [{ hostIp: '', hostPort: undefined, containerPort: undefined, protocol: 'tcp' }];
}

function toVolumes(values?: DockerVolumeBindingCommand[]) {
  return values?.length ? values : [{ source: '', target: '', type: 'bind', readOnly: false }];
}

function normalizeStringArray(values?: string[]) {
  return values?.filter(Boolean) || [];
}

function normalizeKeyValueList(values?: DockerKeyValueCommand[]) {
  return (values || []).filter((item) => item?.key);
}

function normalizePortList(values?: DockerPortBindingCommand[]) {
  return (values || []).filter((item) => item?.containerPort);
}

function normalizeVolumeList(values?: DockerVolumeBindingCommand[]) {
  return (values || []).filter((item) => item?.source && item?.target);
}

function buildSingleDefaults(data: DockerImageStartupPreview): DockerContainerCreateRequest {
  return {
    imageId: data.imageId,
    imageReference: data.imageReference,
    containerName: data.defaultContainerName,
    entrypoint: data.entrypoint,
    command: data.command,
    environment: toPairs(data.environment),
    portBindings: toPorts(data.portBindings),
    volumeBindings: toVolumes(data.volumeBindings),
    labels: toPairs(data.labels),
    workingDir: data.workingDir,
    user: data.user,
    networkMode: '',
    privileged: false,
    capAdd: [],
    capDrop: [],
    restartPolicy: 'always',
    tty: !!data.tty,
    stdinOpen: !!data.stdinOpen,
    publishAllPorts: !!data.publishAllPorts,
    autoRemove: false,
    resourceLimits: {},
  };
}

function composeServiceFromSingle(
  values: DockerContainerCreateRequest,
  preview: DockerImageStartupPreview | null,
): ComposeServiceForm {
  const imageReference = preview?.imageReference || values.imageReference || values.imageId || preview?.imageId;
  return {
    key: preview?.defaultServiceName || 'app',
    name: preview?.defaultServiceName || 'app',
    image: imageReference,
    containerName: values.containerName,
    entrypoint: normalizeStringArray(values.entrypoint),
    command: normalizeStringArray(values.command),
    environment: toPairs(values.environment),
    portBindings: toPorts(values.portBindings),
    volumeBindings: toVolumes(values.volumeBindings),
    labels: toPairs(values.labels),
    workingDir: values.workingDir,
    user: values.user,
    networkMode: values.networkMode,
    privileged: !!values.privileged,
    capAdd: normalizeStringArray(values.capAdd),
    capDrop: normalizeStringArray(values.capDrop),
    restartPolicy: values.restartPolicy || 'always',
    tty: !!values.tty,
    stdinOpen: !!values.stdinOpen,
    publishAllPorts: !!values.publishAllPorts,
    resourceLimits: values.resourceLimits || {},
    dependsOn: [],
  };
}

function buildComposeModel(
  values: DockerContainerCreateRequest,
  preview: DockerImageStartupPreview | null,
): ComposeFormModel {
  return {
    projectName: preview?.defaultProjectName || 'docker-ui-project',
    services: [composeServiceFromSingle(values, preview)],
  };
}

function parseMemoryToMb(value: unknown) {
  if (value === null || value === undefined || value === '') {
    return undefined;
  }
  if (typeof value === 'number') {
    return Math.round(value / 1024 / 1024);
  }
  const text = String(value).trim().toLowerCase();
  if (!text) {
    return undefined;
  }
  const numeric = parseFloat(text);
  if (!Number.isFinite(numeric)) {
    return undefined;
  }
  if (text.endsWith('g') || text.endsWith('gb')) {
    return Math.round(numeric * 1024);
  }
  if (text.endsWith('m') || text.endsWith('mb')) {
    return Math.round(numeric);
  }
  if (text.endsWith('k') || text.endsWith('kb')) {
    return Math.round(numeric / 1024);
  }
  return Math.round(numeric / 1024 / 1024);
}

function parseEnvironment(value: unknown): DockerKeyValueCommand[] {
  if (Array.isArray(value)) {
    return value.map((item) => {
      const [key, ...rest] = String(item).split('=');
      return { key, value: rest.join('=') };
    });
  }
  if (value && typeof value === 'object') {
    return Object.entries(value as Record<string, unknown>).map(([key, item]) => ({
      key,
      value: item == null ? '' : String(item),
    }));
  }
  return [{ key: '', value: '' }];
}

function parseLabels(value: unknown) {
  return parseEnvironment(value);
}

function parsePorts(value: unknown): DockerPortBindingCommand[] {
  if (!Array.isArray(value) || !value.length) {
    return [{ hostIp: '', hostPort: undefined, containerPort: undefined, protocol: 'tcp' }];
  }
  return value.map((item) => {
    const text = String(item);
    const protocolSplit = text.split('/');
    const binding = protocolSplit[0];
    const protocol = protocolSplit[1] || 'tcp';
    const parts = binding.split(':');
    if (parts.length === 3) {
      return {
        hostIp: parts[0],
        hostPort: Number(parts[1]),
        containerPort: Number(parts[2]),
        protocol,
      };
    }
    if (parts.length === 2) {
      return {
        hostIp: '',
        hostPort: Number(parts[0]),
        containerPort: Number(parts[1]),
        protocol,
      };
    }
    return {
      hostIp: '',
      hostPort: undefined,
      containerPort: Number(parts[0]),
      protocol,
    };
  });
}

function parseVolumes(value: unknown): DockerVolumeBindingCommand[] {
  if (!Array.isArray(value) || !value.length) {
    return [{ source: '', target: '', type: 'bind', readOnly: false }];
  }
  return value.map((item) => {
    const parts = String(item).split(':');
    return {
      source: parts[0] || '',
      target: parts[1] || '',
      type: 'bind',
      readOnly: parts[2] === 'ro',
    };
  });
}

function composeModelToYaml(model: ComposeFormModel) {
  const services = model.services.reduce<Record<string, Record<string, unknown>>>((accumulator, service) => {
    const serviceName = service.name || service.key || 'app';
    const payload: Record<string, unknown> = {
      image: service.image,
    };
    if (service.containerName) {
      payload.container_name = service.containerName;
    }
    if (service.entrypoint?.length) {
      payload.entrypoint = service.entrypoint.filter(Boolean);
    }
    if (service.command?.length) {
      payload.command = service.command.filter(Boolean);
    }
    if (normalizeKeyValueList(service.environment).length) {
      payload.environment = Object.fromEntries(
        normalizeKeyValueList(service.environment).map((item) => [item.key!, item.value || '']),
      );
    }
    if (normalizePortList(service.portBindings).length) {
      payload.ports = normalizePortList(service.portBindings).map((item) => {
        const prefix = item.hostIp ? `${item.hostIp}:` : '';
        const host = item.hostPort ? `${item.hostPort}:` : '';
        const protocol = item.protocol && item.protocol !== 'tcp' ? `/${item.protocol}` : '';
        return `${prefix}${host}${item.containerPort}${protocol}`;
      });
    }
    if (normalizeVolumeList(service.volumeBindings).length) {
      payload.volumes = normalizeVolumeList(service.volumeBindings).map(
        (item) => `${item.source}:${item.target}${item.readOnly ? ':ro' : ''}`,
      );
    }
    if (normalizeKeyValueList(service.labels).length) {
      payload.labels = Object.fromEntries(
        normalizeKeyValueList(service.labels).map((item) => [item.key!, item.value || '']),
      );
    }
    if (service.workingDir) {
      payload.working_dir = service.workingDir;
    }
    if (service.user) {
      payload.user = service.user;
    }
    if (service.networkMode) {
      payload.network_mode = service.networkMode;
    }
    if (service.privileged) {
      payload.privileged = true;
    }
    if (service.restartPolicy) {
      payload.restart = service.restartPolicy;
    }
    if (service.capAdd?.length) {
      payload.cap_add = service.capAdd;
    }
    if (service.capDrop?.length) {
      payload.cap_drop = service.capDrop;
    }
    if (service.tty) {
      payload.tty = true;
    }
    if (service.stdinOpen) {
      payload.stdin_open = true;
    }
    if (service.dependsOn?.length) {
      payload.depends_on = service.dependsOn.filter((item) => item && item !== serviceName);
    }
    const limits: Record<string, unknown> = {};
    if (service.resourceLimits?.cpus) {
      limits.cpus = String(service.resourceLimits.cpus);
    }
    if (service.resourceLimits?.memoryMb) {
      limits.memory = `${service.resourceLimits.memoryMb}m`;
    }
    if (Object.keys(limits).length) {
      payload.deploy = { resources: { limits } };
    }
    accumulator[serviceName] = payload;
    return accumulator;
  }, {});

  return stringifyYaml({
    name: model.projectName || 'docker-ui-project',
    services,
  });
}

function yamlToComposeModel(yaml: string, preview: DockerImageStartupPreview | null): ComposeFormModel {
  const parsed = parseYaml(yaml) as Record<string, unknown>;
  const services = parsed?.services && typeof parsed.services === 'object' ? (parsed.services as Record<string, unknown>) : {};
  const entries = Object.entries(services);
  if (!entries.length) {
    return {
      projectName: String(parsed?.name || preview?.defaultProjectName || 'docker-ui-project'),
      services: [composeServiceFromSingle(buildSingleDefaults(preview || { imageId: '', environment: [], portBindings: [], volumeBindings: [], labels: [] }), preview)],
    };
  }
  return {
    projectName: String(parsed?.name || preview?.defaultProjectName || 'docker-ui-project'),
    services: entries.map(([serviceName, raw]) => {
      const service = isRecord(raw) ? raw : {};
      const deploy = isRecord(service.deploy) ? service.deploy : {};
      const resources = isRecord(deploy.resources) ? deploy.resources : {};
      const limits = isRecord(resources.limits) ? resources.limits : {};
      return {
        key: serviceName,
        name: serviceName,
        image: service.image ? String(service.image) : (preview?.imageReference || preview?.imageId || ''),
        containerName: service.container_name ? String(service.container_name) : '',
        entrypoint: Array.isArray(service.entrypoint) ? service.entrypoint.map(String) : [],
        command: Array.isArray(service.command) ? service.command.map(String) : [],
        environment: parseEnvironment(service.environment),
        portBindings: parsePorts(service.ports),
        volumeBindings: parseVolumes(service.volumes),
        labels: parseLabels(service.labels),
        workingDir: service.working_dir ? String(service.working_dir) : '',
        user: service.user ? String(service.user) : '',
        networkMode: service.network_mode ? String(service.network_mode) : '',
        privileged: !!service.privileged,
        capAdd: Array.isArray(service.cap_add) ? service.cap_add.map(String) : [],
        capDrop: Array.isArray(service.cap_drop) ? service.cap_drop.map(String) : [],
        restartPolicy: service.restart ? String(service.restart) : 'always',
        tty: !!service.tty,
        stdinOpen: !!service.stdin_open,
        publishAllPorts: false,
        resourceLimits: {
          cpus: limits?.cpus ? Number(limits.cpus) : undefined,
          memoryMb: parseMemoryToMb(limits?.memory),
        },
        dependsOn: Array.isArray(service.depends_on)
          ? service.depends_on.map(String)
          : service.depends_on && typeof service.depends_on === 'object'
            ? Object.keys(service.depends_on as Record<string, unknown>)
            : [],
      } satisfies ComposeServiceForm;
    }),
  };
}

function emptyComposeService(preview: DockerImageStartupPreview | null, index: number): ComposeServiceForm {
  const name = index === 0 ? preview?.defaultServiceName || 'app' : `service-${index + 1}`;
  return {
    key: `${name}-${Date.now()}`,
    name,
    image: preview?.imageReference || preview?.imageId,
    environment: [{ key: '', value: '' }],
    portBindings: [{ hostIp: '', hostPort: undefined, containerPort: undefined, protocol: 'tcp' }],
    volumeBindings: [{ source: '', target: '', type: 'bind', readOnly: false }],
    labels: [{ key: '', value: '' }],
    restartPolicy: 'always',
    capAdd: [],
    capDrop: [],
    resourceLimits: {},
    dependsOn: [],
  };
}

function parseSuggestedComposeYaml(
  yaml: string | undefined,
  preview: DockerImageStartupPreview,
  fallback: ComposeFormModel,
) {
  if (!yaml?.trim()) {
    return fallback;
  }
  try {
    return yamlToComposeModel(yaml, preview);
  } catch {
    return fallback;
  }
}

function toEditableRows<T extends object>(values: T[] | undefined, prefix: string) {
  return (values || []).map((item, index) => ({
    ...item,
    _rowKey: `${prefix}-${index}`,
  })) as EditableRow<T>[];
}

function stripEditableRows<T extends object>(rows: readonly EditableRow<T>[]) {
  return rows.map((row) => {
    const { _rowKey, ...value } = row;
    void _rowKey;
    return value;
  });
}

function KeyValueListEditor({
  title,
  description,
  values,
  onChange,
  addLabel,
  keyLabel,
  keyPlaceholder,
  valueLabel,
  valuePlaceholder,
}: {
  title: string;
  description?: string;
  values: DockerKeyValueCommand[];
  onChange: (values: DockerKeyValueCommand[]) => void;
  addLabel: string;
  keyLabel?: string;
  keyPlaceholder: string;
  valueLabel?: string;
  valuePlaceholder: string;
}) {
  const rows = useMemo(() => toEditableRows(values, title), [title, values]);
  const columns = useMemo<ProColumns<EditableRow<DockerKeyValueCommand>>[]>(
    () => [
      {
        title: keyLabel || '键',
        dataIndex: 'key',
        formItemProps: {
          rules: [{ required: true, whitespace: true, message: `请输入${keyLabel || '键'}` }],
        },
        fieldProps: {
          placeholder: keyPlaceholder,
        },
      },
      {
        title: valueLabel || '值',
        dataIndex: 'value',
        fieldProps: {
          placeholder: valuePlaceholder,
        },
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 96,
        editable: false,
        render: (_, record) => [
          <Button
            key="delete"
            type="link"
            danger
            onClick={() => onChange(stripEditableRows(rows.filter((item) => item._rowKey !== record._rowKey)))}
          >
            删除
          </Button>,
        ],
      },
    ],
    [keyLabel, keyPlaceholder, onChange, rows, valueLabel, valuePlaceholder],
  );
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <div className="font-medium text-slate-800">{title}</div>
          {description ? <div className="text-xs text-slate-500">{description}</div> : null}
        </div>
        <Button onClick={() => onChange([...(values || []), { key: '', value: '' }])}>{addLabel}</Button>
      </div>
      <EditableProTable<EditableRow<DockerKeyValueCommand>>
        rowKey="_rowKey"
        headerTitle={false}
        search={false}
        options={false}
        toolBarRender={false}
        recordCreatorProps={false}
        pagination={false}
        scroll={{ x: 720 }}
        value={rows}
        onChange={(nextRows) => onChange(stripEditableRows(nextRows))}
        columns={columns}
        editable={{
          type: 'multiple',
          editableKeys: rows.map((item) => item._rowKey),
          onValuesChange: (_, nextRows) => onChange(stripEditableRows(nextRows)),
          actionRender: () => [],
        }}
      />
    </div>
  );
}

function PortBindingEditor({
  values,
  onChange,
  serviceName,
}: {
  values: DockerPortBindingCommand[];
  onChange: (values: DockerPortBindingCommand[]) => void;
  serviceName: string;
}) {
  const rows = useMemo(() => toEditableRows(values, `${serviceName}-ports`), [serviceName, values]);
  const columns = useMemo<ProColumns<EditableRow<DockerPortBindingCommand>>[]>(
    () => [
      {
        title: '宿主机 IP',
        dataIndex: 'hostIp',
        fieldProps: {
          placeholder: '可留空',
        },
      },
      {
        title: '宿主机端口',
        dataIndex: 'hostPort',
        valueType: 'digit',
        fieldProps: {
          min: 1,
          precision: 0,
          placeholder: '随机分配可留空',
        },
      },
      {
        title: '容器端口',
        dataIndex: 'containerPort',
        valueType: 'digit',
        formItemProps: {
          rules: [{ required: true, message: '请输入容器端口' }],
        },
        fieldProps: {
          min: 1,
          precision: 0,
          placeholder: '必填',
        },
      },
      {
        title: '协议',
        dataIndex: 'protocol',
        valueType: 'select',
        initialValue: 'tcp',
        fieldProps: {
          options: [
            { label: 'tcp', value: 'tcp' },
            { label: 'udp', value: 'udp' },
          ],
        },
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 96,
        editable: false,
        render: (_, record) => [
          <Button
            key="delete"
            type="link"
            danger
            onClick={() => onChange(stripEditableRows(rows.filter((item) => item._rowKey !== record._rowKey)))}
          >
            删除
          </Button>,
        ],
      },
    ],
    [onChange, rows],
  );
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <div className="font-medium text-slate-800">端口映射</div>
          <div className="text-xs text-slate-500">将写入 `services.{serviceName}.ports`，直接对应 Compose 端口映射。</div>
        </div>
        <Button onClick={() => onChange([...(values || []), { hostIp: '', hostPort: undefined, containerPort: undefined, protocol: 'tcp' }])}>
          新增端口
        </Button>
      </div>
      <EditableProTable<EditableRow<DockerPortBindingCommand>>
        rowKey="_rowKey"
        headerTitle={false}
        search={false}
        options={false}
        toolBarRender={false}
        recordCreatorProps={false}
        pagination={false}
        scroll={{ x: 860 }}
        value={rows}
        onChange={(nextRows) => onChange(stripEditableRows(nextRows))}
        columns={columns}
        editable={{
          type: 'multiple',
          editableKeys: rows.map((item) => item._rowKey),
          onValuesChange: (_, nextRows) => onChange(stripEditableRows(nextRows)),
          actionRender: () => [],
        }}
      />
    </div>
  );
}

function VolumeBindingEditor({
  values,
  onChange,
  serviceName,
}: {
  values: DockerVolumeBindingCommand[];
  onChange: (values: DockerVolumeBindingCommand[]) => void;
  serviceName: string;
}) {
  const rows = useMemo(() => toEditableRows(values, `${serviceName}-volumes`), [serviceName, values]);
  const columns = useMemo<ProColumns<EditableRow<DockerVolumeBindingCommand>>[]>(
    () => [
      {
        title: '宿主机路径',
        dataIndex: 'source',
        formItemProps: {
          rules: [{ required: true, whitespace: true, message: '请输入宿主机路径' }],
        },
        fieldProps: {
          placeholder: '例如 /data/app',
        },
      },
      {
        title: '容器路径',
        dataIndex: 'target',
        formItemProps: {
          rules: [{ required: true, whitespace: true, message: '请输入容器路径' }],
        },
        fieldProps: {
          placeholder: '例如 /var/www/html',
        },
      },
      {
        title: '类型',
        dataIndex: 'type',
        valueType: 'select',
        initialValue: 'bind',
        fieldProps: {
          options: [
            { label: 'bind', value: 'bind' },
            { label: 'volume', value: 'volume' },
          ],
        },
      },
      {
        title: '只读',
        dataIndex: 'readOnly',
        valueType: 'switch',
        width: 96,
        initialValue: false,
        fieldProps: {
          size: 'small',
        },
        render: (_, record) => (record.readOnly ? '只读' : '读写'),
      },
      {
        title: '操作',
        dataIndex: 'actions',
        width: 96,
        editable: false,
        render: (_, record) => [
          <Button
            key="delete"
            type="link"
            danger
            onClick={() => onChange(stripEditableRows(rows.filter((item) => item._rowKey !== record._rowKey)))}
          >
            删除
          </Button>,
        ],
      },
    ],
    [onChange, rows],
  );
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <div className="font-medium text-slate-800">卷 / 目录映射</div>
          <div className="text-xs text-slate-500">将写入 `services.{serviceName}.volumes`，直接表达挂载来源与目标。</div>
        </div>
        <Button onClick={() => onChange([...(values || []), { source: '', target: '', type: 'bind', readOnly: false }])}>
          新增映射
        </Button>
      </div>
      <EditableProTable<EditableRow<DockerVolumeBindingCommand>>
        rowKey="_rowKey"
        headerTitle={false}
        search={false}
        options={false}
        toolBarRender={false}
        recordCreatorProps={false}
        pagination={false}
        scroll={{ x: 920 }}
        value={rows}
        onChange={(nextRows) => onChange(stripEditableRows(nextRows))}
        columns={columns}
        editable={{
          type: 'multiple',
          editableKeys: rows.map((item) => item._rowKey),
          onValuesChange: (_, nextRows) => onChange(stripEditableRows(nextRows)),
          actionRender: () => [],
        }}
      />
    </div>
  );
}

export function ImageStartupDrawer({ open, image, onClose, onStarted }: ImageStartupDrawerProps) {
  const [loading, setLoading] = useState(false);
  const [preview, setPreview] = useState<DockerImageStartupPreview | null>(null);
  const [mode, setMode] = useState<'single' | 'compose'>('compose');
  const [composeEditorMode, setComposeEditorMode] = useState<'form' | 'yaml'>('form');
  const [composeYaml, setComposeYaml] = useState('');
  const [composeNormalizedYaml, setComposeNormalizedYaml] = useState('');
  const [composeForm, setComposeForm] = useState<ComposeFormModel>({ projectName: '', services: [] });
  const [activeServiceKey, setActiveServiceKey] = useState('');
  const [imageReferenceOptions, setImageReferenceOptions] = useState<
    { label: string; options: { label: string; value: string }[] }[]
  >([]);
  const [imageReferenceLoading, setImageReferenceLoading] = useState(false);
  const [imageReferenceKeyword, setImageReferenceKeyword] = useState('');
  const [singleForm] = Form.useForm<DockerContainerCreateRequest>();

  const applyPreview = useCallback((data: DockerImageStartupPreview) => {
    const singleDefaults = buildSingleDefaults(data);
    singleForm.setFieldsValue(singleDefaults);
    const fallbackComposeForm = buildComposeModel(singleDefaults, data);
    const nextComposeForm = parseSuggestedComposeYaml(data.suggestedComposeYaml, data, fallbackComposeForm);
    setComposeForm(nextComposeForm);
    setActiveServiceKey(nextComposeForm.services[0]?.key || '');
    setComposeYaml(data.suggestedComposeYaml || composeModelToYaml(nextComposeForm));
    setComposeNormalizedYaml('');
  }, [singleForm]);

  useEffect(() => {
    if (!open || !image) {
      return;
    }
    const timer = window.setTimeout(() => {
      setLoading(true);
      void getDockerImageStartupPreview(image.imageId)
        .then((response) => {
          const data = response.data;
          setPreview(data);
          applyPreview(data);
        })
        .catch((error) => {
          message.error((error as Error).message || '读取镜像启动配置失败');
        })
        .finally(() => setLoading(false));
    }, 0);
    return () => window.clearTimeout(timer);
  }, [applyPreview, image, open]);

  useEffect(() => {
    if (!open) {
      return;
    }
    let cancelled = false;
    const loadImageReferenceOptions = async () => {
      setImageReferenceLoading(true);
      try {
        const localResponse = await getLocalDockerImages({ current: 1, size: 200 });
        const localRefMap = new Map<string, { label: string; value: string }>();
        (localResponse.data.records || []).forEach((localImage) => {
          (localImage.repoTags || []).forEach((tag) => {
            if (!tag || localRefMap.has(tag)) {
              return;
            }
            localRefMap.set(tag, {
              value: tag,
              label: `本地镜像 · ${tag}`,
            });
          });
        });

        const registriesResponse = await getDockerRegistries();
        const remoteResults = await Promise.allSettled(
          (registriesResponse.data || []).map(async (registry) => {
            const repositoriesResponse = await getDockerRepositories(registry.id, { current: 1, size: 200 });
            return {
              registry,
              repositories: repositoriesResponse.data.records || [],
            };
          }),
        );

        const remoteRefMap = new Map<string, { label: string; value: string }>();
        remoteResults.forEach((item) => {
          if (item.status !== 'fulfilled') {
            return;
          }
          item.value.repositories.forEach((repository) => {
            const value = repository.repository;
            if (!value || remoteRefMap.has(value)) {
              return;
            }
            remoteRefMap.set(value, {
              value,
              label: `${item.value.registry.name} · ${value}`,
            });
          });
        });

        if (cancelled) {
          return;
        }

        const nextOptions: { label: string; options: { label: string; value: string }[] }[] = [];
        if (localRefMap.size) {
          nextOptions.push({
            label: '本地镜像',
            options: Array.from(localRefMap.values()).sort((left, right) => left.value.localeCompare(right.value)),
          });
        }
        if (remoteRefMap.size) {
          nextOptions.push({
            label: '远程镜像源',
            options: Array.from(remoteRefMap.values()).sort((left, right) => left.value.localeCompare(right.value)),
          });
        }
        setImageReferenceOptions(nextOptions);
      } catch (error) {
        if (!cancelled) {
          message.warning((error as Error).message || '加载镜像候选失败，仍可手动输入镜像引用');
          setImageReferenceOptions([]);
        }
      } finally {
        if (!cancelled) {
          setImageReferenceLoading(false);
        }
      }
    };
    void loadImageReferenceOptions();
    return () => {
      cancelled = true;
    };
  }, [open]);

  useEffect(() => {
    if (!composeForm.services.length) {
      if (activeServiceKey) {
        const timer = window.setTimeout(() => setActiveServiceKey(''), 0);
        return () => window.clearTimeout(timer);
      }
      return;
    }
    if (!composeForm.services.some((service) => service.key === activeServiceKey)) {
      const timer = window.setTimeout(() => setActiveServiceKey(composeForm.services[0]?.key || ''), 0);
      return () => window.clearTimeout(timer);
    }
    return undefined;
  }, [activeServiceKey, composeForm.services]);

  const getSinglePayload = async () => {
    const validated = await singleForm.validateFields();
    const merged = {
      ...singleForm.getFieldsValue(true),
      ...validated,
    } as DockerContainerCreateRequest;
    merged.imageId = merged.imageId || preview?.imageId;
    merged.imageReference = merged.imageReference || preview?.imageReference;
    return merged;
  };

  const syncComposeFromSingle = async () => {
    const singleValues = await getSinglePayload();
    const nextComposeForm = buildComposeModel(singleValues, preview);
    setComposeForm(nextComposeForm);
    setActiveServiceKey(nextComposeForm.services[0]?.key || '');
    setComposeYaml(composeModelToYaml(nextComposeForm));
    setComposeNormalizedYaml('');
    setMode('compose');
    setComposeEditorMode('form');
  };

  const syncYamlFromComposeForm = () => {
    const nextYaml = composeModelToYaml(composeForm);
    setComposeYaml(nextYaml);
    return nextYaml;
  };

  const syncComposeFormFromYaml = () => {
    try {
      const nextComposeForm = yamlToComposeModel(composeYaml, preview);
      setComposeForm(nextComposeForm);
      setActiveServiceKey(nextComposeForm.services[0]?.key || '');
      setComposeNormalizedYaml('');
      message.success('已按 YAML 解析为表单配置');
      return true;
    } catch (error) {
      message.error((error as Error).message || 'YAML 解析失败');
      return false;
    }
  };

  const handleSingleStart = async () => {
    const values = await getSinglePayload();
    setLoading(true);
    try {
      const response = await createDockerContainerFromImage(values);
      message.success(`容器创建操作已提交 #${response.data.operationId}`);
      onStarted?.();
      onClose();
    } finally {
      setLoading(false);
    }
  };

  const handleComposeValidate = async () => {
    const yaml = composeEditorMode === 'form' ? syncYamlFromComposeForm() : composeYaml;
    const payload: DockerComposeUpRequest = { projectName: composeForm.projectName, composeYaml: yaml };
    const response = await previewDockerCompose(payload);
    if (response.data.validation.valid) {
      message.success('Compose 配置校验通过');
      const normalizedYaml = response.data.normalizedYaml?.trim();
      setComposeNormalizedYaml(normalizedYaml && normalizedYaml !== yaml.trim() ? normalizedYaml : '');
      if (response.data.preview.warnings?.length) {
        message.warning(`检测到 ${response.data.preview.warnings.length} 条 Docker 安全策略告警`);
      }
      return;
    }
    message.error(response.data.validation.message || 'Compose 配置校验失败');
  };

  const handleComposeStart = async () => {
    const yaml = composeEditorMode === 'form' ? syncYamlFromComposeForm() : composeYaml;
    setLoading(true);
    try {
      const response = await upDockerCompose({ projectName: composeForm.projectName, composeYaml: yaml });
      message.success(`Compose 编排操作已提交 #${response.data.operationId}`);
      onStarted?.();
      onClose();
    } finally {
      setLoading(false);
    }
  };

  const exportCompose = () => {
    const yaml = composeEditorMode === 'form' ? syncYamlFromComposeForm() : composeYaml;
    const blob = new Blob([yaml], { type: 'text/yaml;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = `${composeForm.projectName || preview?.defaultProjectName || 'docker-compose'}.yaml`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const activeServiceIndex = useMemo(
    () => composeForm.services.findIndex((service) => service.key === activeServiceKey),
    [activeServiceKey, composeForm.services],
  );
  const activeService = activeServiceIndex >= 0 ? composeForm.services[activeServiceIndex] : composeForm.services[0];
  const activeServiceName = activeService?.name || activeService?.key || 'service';
  const filteredImageReferenceOptions = useMemo(() => {
    const keyword = imageReferenceKeyword.trim().toLowerCase();
    if (!keyword) {
      return imageReferenceOptions;
    }
    return imageReferenceOptions
      .map((group) => ({
        ...group,
        options: group.options.filter((option) => option.value.toLowerCase().includes(keyword)),
      }))
      .filter((group) => group.options.length > 0);
  }, [imageReferenceKeyword, imageReferenceOptions]);
  const dependsOnOptions = useMemo(
    () =>
      composeForm.services
        .filter((service) => service.key !== activeService?.key)
        .map((service) => ({
          label: service.name || service.key,
          value: service.name || service.key,
        })),
    [activeService?.key, composeForm.services],
  );

  const updateComposeService = (updater: (service: ComposeServiceForm) => ComposeServiceForm) => {
    if (!activeService) {
      return;
    }
    setComposeNormalizedYaml('');
    setComposeForm((current) => ({
      ...current,
      services: current.services.map((service) => (service.key === activeService.key ? updater(service) : service)),
    }));
  };

  const composeServiceTabs = composeForm.services.map((service) => ({
    key: service.key,
    label: service.name || service.key,
    children: null,
  }));

  const renderComposeServiceEditor = () => {
    if (!activeService) {
      return null;
    }
    return (
      <Tabs
        items={[
          {
            key: 'basic',
            label: '基础配置',
            children: (
              <div className="max-w-5xl space-y-4 rounded-2xl border border-slate-200 p-4">
                <div className="grid gap-4">
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">服务名</div>
                  <Input
                    value={activeService.name}
                    onChange={(event) =>
                      updateComposeService((service) => ({ ...service, name: event.target.value || service.name }))
                    }
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">镜像引用</div>
                  <AutoComplete
                    className="w-full"
                    options={filteredImageReferenceOptions}
                    value={activeService.image}
                    onChange={(value) => updateComposeService((service) => ({ ...service, image: value }))}
                    onSearch={setImageReferenceKeyword}
                    onFocus={() => setImageReferenceKeyword('')}
                    placeholder="可直接输入，也可从本地镜像或远程镜像源选择"
                    filterOption={false}
                  >
                    <Input suffix={imageReferenceLoading ? '加载候选中' : undefined} />
                  </AutoComplete>
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">容器名称</div>
                  <Input
                    value={activeService.containerName}
                    onChange={(event) =>
                      updateComposeService((service) => ({ ...service, containerName: event.target.value }))
                    }
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">网络模式</div>
                  <Input
                    value={activeService.networkMode}
                    onChange={(event) =>
                      updateComposeService((service) => ({ ...service, networkMode: event.target.value }))
                    }
                    placeholder="bridge / host / 自定义网络"
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">工作目录</div>
                  <Input
                    value={activeService.workingDir}
                    onChange={(event) =>
                      updateComposeService((service) => ({ ...service, workingDir: event.target.value }))
                    }
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">运行用户</div>
                  <Input
                    value={activeService.user}
                    onChange={(event) => updateComposeService((service) => ({ ...service, user: event.target.value }))}
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">Entrypoint</div>
                  <Select
                    className="w-full"
                    mode="tags"
                    value={activeService.entrypoint}
                    tokenSeparators={[',']}
                    placeholder="每项一个参数，回车确认"
                    onChange={(value) => updateComposeService((service) => ({ ...service, entrypoint: value }))}
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">Command</div>
                  <Select
                    className="w-full"
                    mode="tags"
                    value={activeService.command}
                    tokenSeparators={[',']}
                    placeholder="每项一个参数，回车确认"
                    onChange={(value) => updateComposeService((service) => ({ ...service, command: value }))}
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">重启策略</div>
                  <Select
                    className="w-full"
                    value={activeService.restartPolicy}
                    options={[
                      { label: 'always', value: 'always' },
                      { label: 'unless-stopped', value: 'unless-stopped' },
                      { label: 'on-failure', value: 'on-failure' },
                      { label: 'none', value: 'none' },
                    ]}
                    onChange={(value) => updateComposeService((service) => ({ ...service, restartPolicy: value }))}
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">依赖服务</div>
                  <Select
                    className="w-full"
                    mode="multiple"
                    value={(activeService.dependsOn || []).filter((item) => item && item !== activeServiceName)}
                    options={dependsOnOptions}
                    onChange={(value) =>
                      updateComposeService((service) => ({
                        ...service,
                        dependsOn: Array.from(new Set((value || []).filter((item) => item && item !== (service.name || service.key)))),
                      }))
                    }
                    placeholder="从当前编排内的其他服务中选择"
                  />
                </div>
              </div>
              </div>
            ),
          },
          {
            key: 'mapping',
            label: '端口 / 存储 / 环境',
            children: (
              <div className="max-w-5xl space-y-5 rounded-2xl border border-slate-200 p-4">
                <KeyValueListEditor
                  title="环境变量"
                  description={`将写入 services.${activeServiceName}.environment`}
                  values={activeService.environment}
                  onChange={(value) => updateComposeService((service) => ({ ...service, environment: value }))}
                  addLabel="新增环境变量"
                  keyLabel="环境变量名"
                  keyPlaceholder="变量名"
                  valueLabel="环境变量值"
                  valuePlaceholder="变量值"
                />
                <PortBindingEditor
                  values={activeService.portBindings}
                  onChange={(value) => updateComposeService((service) => ({ ...service, portBindings: value }))}
                  serviceName={activeServiceName}
                />
                <VolumeBindingEditor
                  values={activeService.volumeBindings}
                  onChange={(value) => updateComposeService((service) => ({ ...service, volumeBindings: value }))}
                  serviceName={activeServiceName}
                />
                <KeyValueListEditor
                  title="标签"
                  description={`将写入 services.${activeServiceName}.labels`}
                  values={activeService.labels}
                  onChange={(value) => updateComposeService((service) => ({ ...service, labels: value }))}
                  addLabel="新增标签"
                  keyLabel="标签键"
                  keyPlaceholder="标签键"
                  valueLabel="标签值"
                  valuePlaceholder="标签值"
                />
              </div>
            ),
          },
          {
            key: 'runtime',
            label: '资源 / 安全',
            children: (
              <div className="max-w-5xl space-y-5 rounded-2xl border border-slate-200 p-4">
                <div className="grid gap-4 md:grid-cols-2">
                  <div>
                    <div className="mb-2 text-sm font-medium text-slate-700">CPU</div>
                    <InputNumber
                      className="w-full"
                      min={0}
                      step={0.1}
                      value={activeService.resourceLimits?.cpus}
                      onChange={(value) =>
                        updateComposeService((service) => ({
                          ...service,
                          resourceLimits: { ...(service.resourceLimits || {}), cpus: value == null ? undefined : Number(value) },
                        }))
                      }
                    />
                  </div>
                  <div>
                    <div className="mb-2 text-sm font-medium text-slate-700">内存(MB)</div>
                    <InputNumber
                      className="w-full"
                      min={0}
                      value={activeService.resourceLimits?.memoryMb}
                      onChange={(value) =>
                        updateComposeService((service) => ({
                          ...service,
                          resourceLimits: { ...(service.resourceLimits || {}), memoryMb: value == null ? undefined : Number(value) },
                        }))
                      }
                    />
                  </div>
                </div>
                <div className="grid gap-4">
                  <div>
                    <div className="mb-2 text-sm font-medium text-slate-700">Cap Add</div>
                    <Select
                      className="w-full"
                      mode="tags"
                      value={activeService.capAdd}
                      tokenSeparators={[',']}
                      placeholder="输入 capability，回车确认"
                      onChange={(value) => updateComposeService((service) => ({ ...service, capAdd: value }))}
                    />
                  </div>
                  <div>
                    <div className="mb-2 text-sm font-medium text-slate-700">Cap Drop</div>
                    <Select
                      className="w-full"
                      mode="tags"
                      value={activeService.capDrop}
                      tokenSeparators={[',']}
                      placeholder="输入 capability，回车确认"
                      onChange={(value) => updateComposeService((service) => ({ ...service, capDrop: value }))}
                    />
                  </div>
                </div>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="flex items-center justify-between rounded-xl border border-slate-200 px-4 py-3">
                    <span className="text-sm font-medium text-slate-700">Privileged</span>
                    <Switch
                      checked={!!activeService.privileged}
                      onChange={(checked) => updateComposeService((service) => ({ ...service, privileged: checked }))}
                    />
                  </div>
                  <div className="flex items-center justify-between rounded-xl border border-slate-200 px-4 py-3">
                    <span className="text-sm font-medium text-slate-700">TTY</span>
                    <Switch
                      checked={!!activeService.tty}
                      onChange={(checked) => updateComposeService((service) => ({ ...service, tty: checked }))}
                    />
                  </div>
                  <div className="flex items-center justify-between rounded-xl border border-slate-200 px-4 py-3">
                    <span className="text-sm font-medium text-slate-700">STDIN</span>
                    <Switch
                      checked={!!activeService.stdinOpen}
                      onChange={(checked) => updateComposeService((service) => ({ ...service, stdinOpen: checked }))}
                    />
                  </div>
                  <div className="flex items-center justify-between rounded-xl border border-slate-200 px-4 py-3">
                    <span className="text-sm font-medium text-slate-700">全部映射端口</span>
                    <Switch
                      checked={!!activeService.publishAllPorts}
                      onChange={(checked) => updateComposeService((service) => ({ ...service, publishAllPorts: checked }))}
                    />
                  </div>
                </div>
              </div>
            ),
          },
        ]}
      />
    );
  };

  const tabItems = [
      {
        key: 'single',
        label: '单容器',
        forceRender: true,
        children: (
          <Form<DockerContainerCreateRequest> form={singleForm} layout="vertical" className="mt-4">
            <Tabs
              items={[
                {
                  key: 'basic',
                  label: '基础配置',
                  children: (
                    <div className="max-w-5xl grid gap-4 rounded-2xl border border-slate-200 p-4 md:grid-cols-2">
                      <Form.Item label="镜像引用" name="imageReference">
                        <Input disabled />
                      </Form.Item>
                      <Form.Item label="容器名称" name="containerName">
                        <Input />
                      </Form.Item>
                      <Form.Item label="工作目录" name="workingDir">
                        <Input />
                      </Form.Item>
                      <Form.Item label="运行用户" name="user">
                        <Input />
                      </Form.Item>
                      <Form.Item label="网络模式" name="networkMode">
                        <Input placeholder="bridge / host / 自定义网络" />
                      </Form.Item>
                      <Form.Item label="重启策略" name="restartPolicy">
                        <Select
                          className="w-full"
                          options={[
                            { label: 'always', value: 'always' },
                            { label: 'unless-stopped', value: 'unless-stopped' },
                            { label: 'on-failure', value: 'on-failure' },
                            { label: 'none', value: 'none' },
                          ]}
                        />
                      </Form.Item>
                      <Form.Item label="Entrypoint" name="entrypoint">
                        <Select className="w-full" mode="tags" tokenSeparators={[',']} placeholder="每项一个参数，回车确认" />
                      </Form.Item>
                      <Form.Item label="Command" name="command">
                        <Select className="w-full" mode="tags" tokenSeparators={[',']} placeholder="每项一个参数，回车确认" />
                      </Form.Item>
                    </div>
                  ),
                },
                {
                  key: 'env',
                  label: '端口 / 存储 / 环境',
                  children: (
                    <div className="space-y-5 rounded-2xl border border-slate-200 p-4">
                      <Form.List name="environment">
                        {(fields, { add, remove }) => (
                          <div className="space-y-3">
                            <div className="flex items-center justify-between">
                              <div className="font-medium">环境变量</div>
                              <Button onClick={() => add({ key: '', value: '' })}>新增环境变量</Button>
                            </div>
                            <div className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs font-medium text-slate-500 lg:grid-cols-[1fr_1fr_auto]">
                              <div>环境变量名</div>
                              <div>变量值</div>
                              <div className="text-right">操作</div>
                            </div>
                            {fields.map((field) => (
                              <div
                                className="grid gap-3 rounded-xl border border-slate-200 px-3 py-3 lg:grid-cols-[1fr_1fr_auto]"
                                key={field.key}
                              >
                                <Form.Item name={[field.name, 'key']} noStyle>
                                  <Input placeholder="变量名" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'value']} noStyle>
                                  <Input placeholder="变量值" />
                                </Form.Item>
                                <Button onClick={() => remove(field.name)}>删除</Button>
                              </div>
                            ))}
                          </div>
                        )}
                      </Form.List>
                      <Form.List name="portBindings">
                        {(fields, { add, remove }) => (
                          <div className="space-y-3">
                            <div className="flex items-center justify-between">
                              <div className="font-medium">端口映射</div>
                              <Button onClick={() => add({ protocol: 'tcp' })}>新增端口</Button>
                            </div>
                            <div className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs font-medium text-slate-500 lg:grid-cols-[1fr_1fr_1fr_120px_auto]">
                              <div>宿主机 IP</div>
                              <div>宿主机端口</div>
                              <div>容器端口</div>
                              <div>协议</div>
                              <div className="text-right">操作</div>
                            </div>
                            {fields.map((field) => (
                              <div
                                className="grid gap-3 rounded-xl border border-slate-200 px-3 py-3 lg:grid-cols-[1fr_1fr_1fr_120px_auto]"
                                key={field.key}
                              >
                                <Form.Item name={[field.name, 'hostIp']} noStyle>
                                  <Input placeholder="宿主机 IP" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'hostPort']} noStyle>
                                  <InputNumber className="w-full" placeholder="宿主机端口" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'containerPort']} noStyle>
                                  <InputNumber className="w-full" placeholder="容器端口" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'protocol']} noStyle>
                                  <Select className="w-full" options={[{ label: 'tcp', value: 'tcp' }, { label: 'udp', value: 'udp' }]} />
                                </Form.Item>
                                <Button onClick={() => remove(field.name)}>删除</Button>
                              </div>
                            ))}
                          </div>
                        )}
                      </Form.List>
                      <Form.List name="volumeBindings">
                        {(fields, { add, remove }) => (
                          <div className="space-y-3">
                            <div className="flex items-center justify-between">
                              <div className="font-medium">卷 / 目录映射</div>
                              <Button onClick={() => add({ type: 'bind', readOnly: false })}>新增映射</Button>
                            </div>
                            <div className="grid gap-3 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-xs font-medium text-slate-500 lg:grid-cols-[1fr_1fr_120px_100px_auto]">
                              <div>宿主机路径</div>
                              <div>容器路径</div>
                              <div>类型</div>
                              <div>只读</div>
                              <div className="text-right">操作</div>
                            </div>
                            {fields.map((field) => (
                              <div
                                className="grid gap-3 rounded-xl border border-slate-200 px-3 py-3 lg:grid-cols-[1fr_1fr_120px_100px_auto]"
                                key={field.key}
                              >
                                <Form.Item name={[field.name, 'source']} noStyle>
                                  <Input placeholder="宿主机路径" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'target']} noStyle>
                                  <Input placeholder="容器路径" />
                                </Form.Item>
                                <Form.Item name={[field.name, 'type']} noStyle>
                                  <Select className="w-full" options={[{ label: 'bind', value: 'bind' }, { label: 'volume', value: 'volume' }]} />
                                </Form.Item>
                                <Form.Item name={[field.name, 'readOnly']} valuePropName="checked" noStyle>
                                  <Checkbox>只读</Checkbox>
                                </Form.Item>
                                <Button onClick={() => remove(field.name)}>删除</Button>
                              </div>
                            ))}
                          </div>
                        )}
                      </Form.List>
                    </div>
                  ),
                },
                {
                  key: 'runtime',
                  label: '资源 / 安全',
                  children: (
                    <div className="max-w-5xl space-y-5 rounded-2xl border border-slate-200 p-4">
                      <div className="grid gap-4">
                        <Form.Item label="CPU" name={['resourceLimits', 'cpus']}>
                          <InputNumber className="w-full" min={0} step={0.1} />
                        </Form.Item>
                        <Form.Item label="内存(MB)" name={['resourceLimits', 'memoryMb']}>
                          <InputNumber className="w-full" min={0} />
                        </Form.Item>
                        <Form.Item label="Swap(MB)" name={['resourceLimits', 'memorySwapMb']}>
                          <InputNumber className="w-full" min={0} />
                        </Form.Item>
                        <Form.Item label="Pids 限制" name={['resourceLimits', 'pidsLimit']}>
                          <InputNumber className="w-full" min={0} />
                        </Form.Item>
                      </div>
                      <div className="grid gap-4 md:grid-cols-2">
                        <Form.Item label="Cap Add" name="capAdd">
                          <Select className="w-full" mode="tags" tokenSeparators={[',']} placeholder="输入 capability，回车确认" />
                        </Form.Item>
                        <Form.Item label="Cap Drop" name="capDrop">
                          <Select className="w-full" mode="tags" tokenSeparators={[',']} placeholder="输入 capability，回车确认" />
                        </Form.Item>
                      </div>
                      <div className="grid gap-4 md:grid-cols-2">
                        <Form.Item label="Privileged" name="privileged" valuePropName="checked">
                          <Switch />
                        </Form.Item>
                        <Form.Item label="TTY" name="tty" valuePropName="checked">
                          <Switch />
                        </Form.Item>
                        <Form.Item label="STDIN" name="stdinOpen" valuePropName="checked">
                          <Switch />
                        </Form.Item>
                        <Form.Item label="全部映射端口" name="publishAllPorts" valuePropName="checked">
                          <Switch />
                        </Form.Item>
                      </div>
                    </div>
                  ),
                },
              ]}
            />
          </Form>
        ),
      },
      {
        key: 'compose',
        label: 'Compose 编排',
        forceRender: true,
        children: (
          <div className="mt-4 space-y-4">
            <Tabs
              activeKey={composeEditorMode}
              onChange={(key) => {
                if (key === 'yaml') {
                  syncYamlFromComposeForm();
                } else if (key === 'form') {
                  if (!syncComposeFormFromYaml()) {
                    return;
                  }
                }
                setComposeEditorMode(key as 'form' | 'yaml');
              }}
              tabBarExtraContent={
                <Space wrap>
                  <Button onClick={() => void syncComposeFromSingle()}>从单容器生成</Button>
                  <Button onClick={() => void handleComposeValidate()}>校验</Button>
                  <Button onClick={exportCompose}>仅导出 compose</Button>
                </Space>
              }
              items={[
                {
                  key: 'form',
                  label: '可视化配置',
                  children: (
                    <div className="space-y-4 pt-2">
                      <div className="max-w-5xl rounded-2xl border border-slate-200 p-4">
                        <div className="max-w-[360px]">
                          <div className="mb-2 text-sm font-medium text-slate-700">项目名称</div>
                          <Input
                            value={composeForm.projectName}
                            onChange={(event) => {
                              setComposeNormalizedYaml('');
                              setComposeForm((current) => ({ ...current, projectName: event.target.value }));
                            }}
                          />
                        </div>
                      </div>
                      <Tabs
                        type="editable-card"
                        activeKey={activeServiceKey}
                        onChange={setActiveServiceKey}
                        onEdit={(targetKey, action) => {
                          if (action === 'add') {
                            const next = [...composeForm.services, emptyComposeService(preview, composeForm.services.length)];
                            setComposeNormalizedYaml('');
                            setComposeForm((current) => ({ ...current, services: next }));
                            setActiveServiceKey(next[next.length - 1].key);
                            return;
                          }
                          if (action === 'remove' && composeForm.services.length > 1) {
                            const next = composeForm.services.filter((service) => service.key !== targetKey);
                            setComposeNormalizedYaml('');
                            setComposeForm((current) => ({ ...current, services: next }));
                            setActiveServiceKey(next[0]?.key || '');
                          }
                        }}
                        items={composeServiceTabs.map((item) => ({ ...item, closable: composeForm.services.length > 1 }))}
                      />
                      {renderComposeServiceEditor()}
                    </div>
                  ),
                },
                {
                  key: 'yaml',
                  label: 'Raw YAML',
                  children: (
                    <div className="space-y-4 pt-2">
                      <Input.TextArea
                        value={composeYaml}
                        onChange={(event) => {
                          setComposeYaml(event.target.value);
                          setComposeNormalizedYaml('');
                        }}
                        autoSize={{ minRows: 18, maxRows: 28 }}
                        placeholder="在这里编辑 docker-compose.yaml"
                        className="max-w-5xl font-mono"
                      />
                      {composeNormalizedYaml ? (
                        <div className="max-w-5xl rounded-2xl border border-slate-200 bg-slate-50 p-4">
                          <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
                            <div className="space-y-1">
                              <div className="text-sm font-medium text-slate-800">规范化结果</div>
                              <div className="text-xs text-slate-500">
                                校验结果不会自动覆盖原始 YAML。只有点击“应用规范化结果”才会替换编辑区内容。
                              </div>
                            </div>
                            <Space wrap>
                              <Button
                                onClick={() => {
                                  setComposeYaml(composeNormalizedYaml);
                                  setComposeNormalizedYaml('');
                                }}
                              >
                                应用规范化结果
                              </Button>
                              <Button onClick={() => setComposeNormalizedYaml('')}>关闭结果</Button>
                            </Space>
                          </div>
                          <Input.TextArea
                            readOnly
                            value={composeNormalizedYaml}
                            autoSize={{ minRows: 10, maxRows: 18 }}
                            className="mt-4 font-mono"
                          />
                        </div>
                      ) : null}
                    </div>
                  ),
                },
              ]}
            />
          </div>
        ),
      },
    ];

  return (
    <Drawer
      open={open}
      forceRender
      styles={{ wrapper: { width: 1120, maxWidth: '100vw' } }}
      title={image ? `基于镜像启动容器 · ${image.repoTags?.[0] || image.imageId}` : '基于镜像启动容器'}
      onClose={onClose}
      destroyOnHidden
      extra={
        <Space>
          <Button onClick={onClose}>取消</Button>
          {mode === 'single' ? (
            <Button type="primary" loading={loading} onClick={() => void handleSingleStart()}>
              启动容器
            </Button>
          ) : (
            <Button type="primary" loading={loading} onClick={() => void handleComposeStart()}>
              启动 Compose
            </Button>
          )}
        </Space>
      }
    >
      {loading || (!preview && !!image) ? (
        <div className="flex min-h-[420px] items-center justify-center">
          <Spin size="large" />
        </div>
      ) : (
        <Tabs activeKey={mode} items={tabItems} onChange={(key) => setMode(key as 'single' | 'compose')} />
      )}
    </Drawer>
  );
}
