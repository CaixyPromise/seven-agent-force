'use client';

import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Divider,
  Empty,
  Input,
  InputNumber,
  Radio,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  message,
} from 'antd';
import {
  CopyOutlined,
  DeleteOutlined,
  PlusOutlined,
  ReloadOutlined,
  SafetyCertificateOutlined,
  SaveOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  getDockerComposeBuilderMetadata,
  getDockerNetworks,
  getDockerRegistries,
  getDockerRepositories,
  getDockerRepositoryTags,
  getDockerVolumes,
  getLocalDockerImages,
  previewDockerfileBuild,
  validateDockerComposeYaml,
  type DockerComposeBuilderMetadataView,
  type DockerComposeBuildFileCommand,
  type DockerComposeVisualDraftView,
  type DockerComposeVisualNetworkView,
  type DockerComposeVisualPortView,
  type DockerComposeVisualServiceView,
  type DockerComposeVisualVolumeMountView,
  type DockerComposeVisualVolumeView,
  type DockerImageView,
  type DockerKeyValueCommand,
  type DockerRemoteRegistryView,
  type DockerRemoteRepositoryView,
  type DockerRemoteTagsView,
  type DockerResourceView,
  type DockerfileBuildPreviewView,
} from '@/api/dockerController';

export interface ComposeVisualBuilderValue {
  visualDraft: DockerComposeVisualDraftView;
  composeYaml: string;
  buildFiles: DockerComposeBuildFileCommand[];
  yamlDirty?: boolean;
}

export interface ComposeVisualBuilderProps {
  projectName?: string;
  workingDir?: string;
  value?: Partial<ComposeVisualBuilderValue>;
  metadata?: DockerComposeBuilderMetadataView | null;
  readonly?: boolean;
  compact?: boolean;
  onChange?: (value: ComposeVisualBuilderValue) => void;
  onSave?: (value: ComposeVisualBuilderValue) => void;
}

type ImageSourceType = 'local' | 'remote' | 'manual' | 'dockerfile';
type ActiveEditorMode = 'visual' | 'yaml';

type ServiceDraft = DockerComposeVisualServiceView & {
  id: string;
  imageSource: ImageSourceType;
  localImage?: string;
  registryId?: API.Int64;
  remoteRepository?: string;
  remoteTag?: string;
  dockerfileContent?: string;
};

type VisualDraftState = Omit<DockerComposeVisualDraftView, 'services'> & { services: ServiceDraft[] };

const DEFAULT_DOCKERFILE = `FROM node:20-alpine\nWORKDIR /app\nCOPY package*.json ./\nRUN npm ci --omit=dev\nCOPY . .\nEXPOSE 3000\nCMD ["npm", "start"]`;

function uid(prefix: string) {
  return `${prefix}_${Date.now()}_${Math.random().toString(16).slice(2)}`;
}

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function toKeyValueArray(value?: DockerKeyValueCommand[] | Record<string, string>): DockerKeyValueCommand[] {
  if (!value) return [];
  if (Array.isArray(value)) return value.map((item) => ({ key: item.key || '', value: item.value || '' }));
  return Object.entries(value).map(([key, itemValue]) => ({ key, value: String(itemValue ?? '') }));
}

function normalizeService(service: Partial<DockerComposeVisualServiceView>, index: number): ServiceDraft {
  const hasBuild = !!service.build;
  const image = service.image || service.build?.context || '';
  return {
    id: uid('svc'),
    serviceName: service.serviceName || `service-${index + 1}`,
    imageSource: hasBuild ? 'dockerfile' : image ? 'manual' : 'manual',
    image: service.image || (hasBuild ? service.build?.context ? `${service.serviceName || 'service'}:latest` : '' : 'nginx:1.25-alpine'),
    containerName: service.containerName || '',
    ports: (service.ports || []).map((port) => ({ ...port, protocol: port.protocol || 'tcp' })),
    environment: toKeyValueArray(service.environment),
    volumes: service.volumes || [],
    networks: service.networks || [],
    dependsOn: service.dependsOn || [],
    restart: service.restart || 'unless-stopped',
    command: Array.isArray(service.command) ? service.command.join(' ') : service.command || '',
    workingDir: service.workingDir || '',
    user: service.user || '',
    build: service.build ? { ...service.build } : { context: `./${service.serviceName || 'service'}`, dockerfile: 'Dockerfile', args: {} },
    dockerfileContent: hasBuild ? DEFAULT_DOCKERFILE : undefined,
    healthcheck: service.healthcheck ? { ...service.healthcheck } : undefined,
    resources: service.resources ? { ...service.resources } : undefined,
    advanced: service.advanced ? { ...service.advanced } : undefined,
    unsupportedFields: service.unsupportedFields || [],
  };
}

function defaultService(name = 'web'): ServiceDraft {
  return normalizeService(
    {
      serviceName: name,
      image: 'nginx:1.25-alpine',
      restart: 'unless-stopped',
      ports: [{ hostIp: '0.0.0.0', hostPort: 80, containerPort: 80, protocol: 'tcp' }],
      networks: ['default'],
    },
    0,
  );
}

function defaultDraft(): VisualDraftState {
  return {
    version: '3.9',
    services: [defaultService('web')],
    networks: [{ name: 'default', driver: 'bridge' }],
    volumes: [],
  };
}

function normalizeDraft(value?: Partial<DockerComposeVisualDraftView>): VisualDraftState {
  if (!value || !Array.isArray(value.services) || !value.services.length) return defaultDraft();
  return {
    version: value.version || '3.9',
    services: value.services.map((service, index) => normalizeService(service, index)),
    networks: value.networks?.length ? value.networks : [{ name: 'default', driver: 'bridge' }],
    volumes: value.volumes || [],
  };
}

function scalar(value: unknown) {
  if (value === undefined || value === null || value === '') return '""';
  const text = String(value);
  if (/^(true|false|null|[0-9]+)$/.test(text)) return JSON.stringify(text);
  if (/^[a-zA-Z0-9_./:@${}-]+$/.test(text)) return text;
  return JSON.stringify(text);
}

function writeKeyValues(lines: string[], key: string, values?: DockerKeyValueCommand[], level = 2) {
  const list = (values || []).filter((item) => item.key?.trim());
  if (!list.length) return;
  lines.push(`${'  '.repeat(level)}${key}:`);
  list.forEach((item) => lines.push(`${'  '.repeat(level + 1)}${item.key!.trim()}: ${scalar(item.value || '')}`));
}

function composeDraftToYaml(draft: VisualDraftState) {
  const lines: string[] = [`version: ${scalar(draft.version || '3.9')}`, 'services:'];
  draft.services.forEach((service) => {
    const name = service.serviceName?.trim() || 'service';
    lines.push(`  ${name}:`);

    if (service.imageSource === 'dockerfile') {
      lines.push('    build:');
      lines.push(`      context: ${scalar(service.build?.context || '.')}`);
      if (service.build?.dockerfile) lines.push(`      dockerfile: ${scalar(service.build.dockerfile)}`);
      const args = service.build?.args || {};
      if (Object.keys(args).length) {
        lines.push('      args:');
        Object.entries(args).forEach(([key, value]) => lines.push(`        ${key}: ${scalar(value)}`));
      }
      if (service.image) lines.push(`    image: ${scalar(service.image)}`);
    } else if (service.image) {
      lines.push(`    image: ${scalar(service.image)}`);
    }

    if (service.containerName) lines.push(`    container_name: ${scalar(service.containerName)}`);
    if (service.restart) lines.push(`    restart: ${scalar(service.restart)}`);
    if (service.command) lines.push(`    command: ${scalar(Array.isArray(service.command) ? service.command.join(' ') : service.command)}`);
    if (service.workingDir) lines.push(`    working_dir: ${scalar(service.workingDir)}`);
    if (service.user) lines.push(`    user: ${scalar(service.user)}`);

    const ports = (service.ports || []).filter((port) => port.containerPort || port.hostPort);
    if (ports.length) {
      lines.push('    ports:');
      ports.forEach((port) => {
        const hostIp = port.hostIp ? `${port.hostIp}:` : '';
        const host = port.hostPort ? `${port.hostPort}:` : '';
        const container = port.containerPort || port.hostPort;
        const protocol = port.protocol && port.protocol !== 'tcp' ? `/${port.protocol}` : '';
        lines.push(`      - ${JSON.stringify(`${hostIp}${host}${container}${protocol}`)}`);
      });
    }

    writeKeyValues(lines, 'environment', service.environment, 2);

    const volumes = (service.volumes || []).filter((volume) => volume.source || volume.target);
    if (volumes.length) {
      lines.push('    volumes:');
      volumes.forEach((volume) => {
        const source = volume.source || '';
        const target = volume.target || '';
        const mode = volume.readOnly ? ':ro' : '';
        if (volume.type && volume.type !== 'bind') {
          lines.push('      - type: volume');
          if (source) lines.push(`        source: ${scalar(source)}`);
          if (target) lines.push(`        target: ${scalar(target)}`);
          if (volume.readOnly) lines.push('        read_only: true');
        } else {
          lines.push(`      - ${JSON.stringify(`${source}:${target}${mode}`)}`);
        }
      });
    }

    if (service.networks?.length) {
      lines.push('    networks:');
      service.networks.forEach((network) => lines.push(`      - ${network}`));
    }

    if (service.dependsOn?.length) {
      lines.push('    depends_on:');
      service.dependsOn.forEach((dependency) => lines.push(`      - ${dependency}`));
    }

    if (service.healthcheck) {
      const hc = service.healthcheck;
      if (hc.disable) {
        lines.push('    healthcheck:');
        lines.push('      disable: true');
      } else if (hc.test || hc.interval || hc.timeout || hc.retries || hc.startPeriod) {
        lines.push('    healthcheck:');
        if (hc.test) {
          const test = Array.isArray(hc.test) ? hc.test : ['CMD-SHELL', hc.test];
          lines.push(`      test: ${JSON.stringify(test)}`);
        }
        if (hc.interval) lines.push(`      interval: ${scalar(hc.interval)}`);
        if (hc.timeout) lines.push(`      timeout: ${scalar(hc.timeout)}`);
        if (hc.retries !== undefined) lines.push(`      retries: ${hc.retries}`);
        if (hc.startPeriod) lines.push(`      start_period: ${scalar(hc.startPeriod)}`);
      }
    }

    const res = service.resources;
    if (res?.cpus) lines.push(`    cpus: ${scalar(res.cpus)}`);
    if (res?.memory) lines.push(`    mem_limit: ${scalar(res.memory)}`);
    if (res?.memoryReservation) lines.push(`    mem_reservation: ${scalar(res.memoryReservation)}`);
    if (res?.pidsLimit !== undefined) lines.push(`    pids_limit: ${res.pidsLimit}`);

    const adv = service.advanced;
    if (adv?.privileged !== undefined) lines.push(`    privileged: ${adv.privileged ? 'true' : 'false'}`);
    if (adv?.networkMode) lines.push(`    network_mode: ${scalar(adv.networkMode)}`);
    if (adv?.pid) lines.push(`    pid: ${scalar(adv.pid)}`);
    if (adv?.ipc) lines.push(`    ipc: ${scalar(adv.ipc)}`);
    if (adv?.capAdd?.length) {
      lines.push('    cap_add:');
      adv.capAdd.forEach((item) => lines.push(`      - ${item}`));
    }
    if (adv?.capDrop?.length) {
      lines.push('    cap_drop:');
      adv.capDrop.forEach((item) => lines.push(`      - ${item}`));
    }
  });

  const networks = (draft.networks || []).filter((network) => network.name);
  if (networks.length) {
    lines.push('networks:');
    networks.forEach((network) => {
      lines.push(`  ${network.name}:`);
      if (network.external) lines.push('    external: true');
      else if (network.driver) lines.push(`    driver: ${scalar(network.driver)}`);
      writeKeyValues(lines, 'labels', network.labels, 2);
    });
  }

  const volumes = (draft.volumes || []).filter((volume) => volume.name);
  if (volumes.length) {
    lines.push('volumes:');
    volumes.forEach((volume) => {
      lines.push(`  ${volume.name}:`);
      if (volume.external) lines.push('    external: true');
      else if (volume.driver) lines.push(`    driver: ${scalar(volume.driver)}`);
      writeKeyValues(lines, 'labels', volume.labels, 2);
    });
  }

  return `${lines.join('\n')}\n`;
}

function serviceImage(service: ServiceDraft) {
  if (service.imageSource === 'local') return service.localImage || service.image || '';
  if (service.imageSource === 'remote') {
    const repo = service.remoteRepository || service.image || '';
    const tag = service.remoteTag || '';
    return repo && tag && !repo.includes(':') ? `${repo}:${tag}` : repo;
  }
  if (service.imageSource === 'dockerfile') return service.image || service.build?.context || `${service.serviceName}:latest`;
  return service.image || '';
}

function draftToBuildFiles(draft: VisualDraftState): DockerComposeBuildFileCommand[] {
  return draft.services
    .filter((service) => service.imageSource === 'dockerfile')
    .map((service) => ({
      serviceName: service.serviceName,
      context: service.build?.context || `./${service.serviceName}`,
      dockerfilePath: service.build?.dockerfile || 'Dockerfile',
      dockerfileContent: service.dockerfileContent || DEFAULT_DOCKERFILE,
      imageTag: service.image || `${service.serviceName}:latest`,
      buildArgs: Object.entries(service.build?.args || {}).map(([key, value]) => ({ key, value })),
    }));
}

function cleanVisualDraft(draft: VisualDraftState): DockerComposeVisualDraftView {
  return {
    version: draft.version,
    networks: draft.networks,
    volumes: draft.volumes,
    services: draft.services.map((service) => ({
      serviceName: service.serviceName,
      image: service.image,
      build: service.imageSource === 'dockerfile' ? service.build : undefined,
      containerName: service.containerName,
      ports: service.ports,
      environment: service.environment,
      volumes: service.volumes,
      networks: service.networks,
      dependsOn: service.dependsOn,
      restart: service.restart,
      command: service.command,
      workingDir: service.workingDir,
      user: service.user,
      healthcheck: service.healthcheck,
      resources: service.resources,
      advanced: service.advanced,
      unsupportedFields: service.unsupportedFields,
    })),
  };
}

function builderValueSignature(value?: Partial<ComposeVisualBuilderValue> | null) {
  if (!value) return '';
  return JSON.stringify({
    visualDraft: value.visualDraft,
    composeYaml: value.composeYaml,
    buildFiles: value.buildFiles,
    yamlDirty: value.yamlDirty,
  });
}

function localParseYamlToDraft(yaml: string, fallback: VisualDraftState): VisualDraftState {
  // A lightweight fallback parser. Backend visualDraft remains the source of truth when available.
  const lines = yaml.split(/\r?\n/);
  const services: ServiceDraft[] = [];
  let current: ServiceDraft | null = null;
  let listMode: 'ports' | 'environment' | 'volumes' | 'networks' | 'depends_on' | 'cap_add' | 'cap_drop' | null = null;
  for (const raw of lines) {
    const line = raw.replace(/\t/g, '  ');
    const svc = /^ {2}([A-Za-z0-9_.-]+):\s*$/.exec(line);
    if (svc && !['build', 'environment', 'ports', 'volumes', 'networks', 'depends_on', 'healthcheck'].includes(svc[1])) {
      current = normalizeService({ serviceName: svc[1], ports: [], environment: [], volumes: [], networks: [], dependsOn: [] }, services.length);
      services.push(current);
      listMode = null;
      continue;
    }
    if (!current) continue;
    const kv = /^ {4}([A-Za-z0-9_\-.]+):\s*(.*)$/.exec(line);
    if (kv) {
      const key = kv[1];
      const value = kv[2].replace(/^['"]|['"]$/g, '');
      listMode = null;
      if (key === 'image') current.image = value;
      else if (key === 'container_name') current.containerName = value;
      else if (key === 'restart') current.restart = value;
      else if (key === 'command') current.command = value;
      else if (key === 'working_dir') current.workingDir = value;
      else if (key === 'user') current.user = value;
      else if (key === 'network_mode') current.advanced = { ...(current.advanced || {}), networkMode: value };
      else if (key === 'privileged') current.advanced = { ...(current.advanced || {}), privileged: value === 'true' };
      else if (key === 'ports' || key === 'environment' || key === 'volumes' || key === 'networks' || key === 'depends_on' || key === 'cap_add' || key === 'cap_drop') listMode = key;
      else if (key === 'build') {
        current.imageSource = 'dockerfile';
        current.build = { context: value || './' };
      }
      continue;
    }
    const item = /^ {6}-\s*(.*)$/.exec(line);
    if (item && listMode) {
      const value = item[1].replace(/^['"]|['"]$/g, '');
      if (listMode === 'ports') {
        const [host, containerWithProtocol] = value.split(':').slice(-2);
        const [container, protocol] = (containerWithProtocol || host || '').split('/');
        current.ports = [...(current.ports || []), { hostPort: host ? Number(host) || host : undefined, containerPort: Number(container) || container, protocol: protocol || 'tcp' }];
      } else if (listMode === 'volumes') {
        const parts = value.split(':');
        current.volumes = [...(current.volumes || []), { source: parts[0], target: parts[1], readOnly: parts[2] === 'ro' }];
      } else if (listMode === 'networks') current.networks = [...(current.networks || []), value];
      else if (listMode === 'depends_on') current.dependsOn = [...(current.dependsOn || []), value];
      else if (listMode === 'cap_add') current.advanced = { ...(current.advanced || {}), capAdd: [...(current.advanced?.capAdd || []), value] };
      else if (listMode === 'cap_drop') current.advanced = { ...(current.advanced || {}), capDrop: [...(current.advanced?.capDrop || []), value] };
    }
  }
  return services.length ? { ...fallback, services } : fallback;
}

function CompactCode({ value, maxHeight = 280 }: { value: string; maxHeight?: number }) {
  return (
    <pre
      className="overflow-auto rounded-2xl border border-slate-800 bg-[#07111f] px-4 py-3 text-xs leading-6 text-slate-100"
      style={{ maxHeight }}
    >
      {value || '暂无内容'}
    </pre>
  );
}

function PairEditor({
  value,
  onChange,
  addText = '添加',
}: {
  value: DockerKeyValueCommand[];
  onChange: (value: DockerKeyValueCommand[]) => void;
  addText?: string;
}) {
  const rows = value.map((item, index) => ({ ...item, id: `${index}-${item.key || ''}` }));
  return (
    <div className="space-y-3">
      <div className="flex justify-end">
        <Button size="small" icon={<PlusOutlined />} onClick={() => onChange([...value, { key: '', value: '' }])}>
          {addText}
        </Button>
      </div>
      <Table
        size="small"
        rowKey="id"
        pagination={false}
        dataSource={rows}
        columns={[
          {
            title: '名称',
            render: (_, row, index) => <Input value={row.key} onChange={(e) => onChange(value.map((item, i) => (i === index ? { ...item, key: e.target.value } : item)))} />,
          },
          {
            title: '值',
            render: (_, row, index) => <Input value={row.value} onChange={(e) => onChange(value.map((item, i) => (i === index ? { ...item, value: e.target.value } : item)))} />,
          },
          {
            title: '操作',
            width: 70,
            render: (_, __, index) => <Button danger type="text" size="small" icon={<DeleteOutlined />} onClick={() => onChange(value.filter((_, i) => i !== index))} />,
          },
        ]}
      />
    </div>
  );
}

function splitWords(value?: string[]) {
  return (value || []).join(', ');
}

function joinWords(value: string) {
  return value.split(',').map((item) => item.trim()).filter(Boolean);
}

export function ComposeVisualBuilder({
  projectName,
  workingDir,
  value,
  metadata: metadataProp,
  readonly = false,
  compact = false,
  onChange,
  onSave,
}: ComposeVisualBuilderProps) {
  const [mode, setMode] = useState<ActiveEditorMode>('visual');
  const [draft, setDraft] = useState<VisualDraftState>(() => normalizeDraft(value?.visualDraft));
  const [yamlText, setYamlText] = useState(value?.composeYaml || composeDraftToYaml(normalizeDraft(value?.visualDraft)));
  const [yamlDirty, setYamlDirty] = useState(false);
  const [selectedServiceId, setSelectedServiceId] = useState<string>(() => draft.services[0]?.id || '');
  const [metadata, setMetadata] = useState<DockerComposeBuilderMetadataView | null>(metadataProp || null);
  const [localImages, setLocalImages] = useState<DockerImageView[]>([]);
  const [registries, setRegistries] = useState<DockerRemoteRegistryView[]>([]);
  const [repositories, setRepositories] = useState<DockerRemoteRepositoryView[]>([]);
  const [remoteTags, setRemoteTags] = useState<DockerRemoteTagsView | null>(null);
  const [dockerNetworks, setDockerNetworks] = useState<DockerResourceView[]>([]);
  const [dockerVolumes, setDockerVolumes] = useState<DockerResourceView[]>([]);
  const [dockerfilePreview, setDockerfilePreview] = useState<DockerfileBuildPreviewView | null>(null);
  const [syncingYaml, setSyncingYaml] = useState(false);
  const [validatingDockerfile, setValidatingDockerfile] = useState(false);
  const appliedValueSignatureRef = useRef('');
  const emittedValueSignatureRef = useRef('');

  useEffect(() => {
    const incomingSignature = builderValueSignature(value);
    if (!incomingSignature || incomingSignature === appliedValueSignatureRef.current) return;
    appliedValueSignatureRef.current = incomingSignature;

    if (value?.visualDraft) {
      const normalized = normalizeDraft(value.visualDraft);
      setDraft(normalized);
      setSelectedServiceId(normalized.services[0]?.id || '');
      setYamlText(value.composeYaml || composeDraftToYaml(normalized));
      setYamlDirty(false);
    } else if (value?.composeYaml) {
      setYamlText(value.composeYaml);
    }
  }, [value]);

  useEffect(() => {
    if (metadataProp) setMetadata(metadataProp);
  }, [metadataProp]);

  useEffect(() => {
    if (!metadataProp) {
      void getDockerComposeBuilderMetadata().then((res) => setMetadata(res.data)).catch(() => undefined);
    }
    void getLocalDockerImages({ current: 1, size: 100 }).then((res) => setLocalImages(res.data.records || [])).catch(() => undefined);
    void getDockerRegistries().then((res) => setRegistries(res.data || [])).catch(() => undefined);
    void getDockerNetworks({ current: 1, size: 100 }).then((res) => setDockerNetworks(res.data.records || [])).catch(() => undefined);
    void getDockerVolumes({ current: 1, size: 100 }).then((res) => setDockerVolumes(res.data.records || [])).catch(() => undefined);
  }, [metadataProp]);

  const generatedYaml = useMemo(() => composeDraftToYaml(draft), [draft]);
  const effectiveYaml = yamlDirty ? yamlText : generatedYaml;
  const buildFiles = useMemo(() => draftToBuildFiles(draft), [draft]);
  const cleanDraft = useMemo(() => cleanVisualDraft(draft), [draft]);
  const selectedService = draft.services.find((service) => service.id === selectedServiceId) || draft.services[0];

  useEffect(() => {
    if (!yamlDirty) setYamlText(generatedYaml);
    const nextValue: ComposeVisualBuilderValue = {
      visualDraft: cleanDraft,
      composeYaml: yamlDirty ? yamlText : generatedYaml,
      buildFiles,
      yamlDirty,
    };
    const nextSignature = builderValueSignature(nextValue);
    if (nextSignature === emittedValueSignatureRef.current) return;
    emittedValueSignatureRef.current = nextSignature;
    appliedValueSignatureRef.current = nextSignature;
    onChange?.(nextValue);
  }, [buildFiles, cleanDraft, generatedYaml, onChange, yamlDirty, yamlText]);

  const updateDraft = useCallback((updater: (current: VisualDraftState) => VisualDraftState) => {
    setYamlDirty(false);
    setDraft((current) => updater(clone(current)));
  }, []);

  const updateService = useCallback((serviceId: string, patch: Partial<ServiceDraft>) => {
    updateDraft((current) => ({
      ...current,
      services: current.services.map((service) => (service.id === serviceId ? { ...service, ...patch } : service)),
    }));
  }, [updateDraft]);

  const addService = () => {
    const name = `service-${draft.services.length + 1}`;
    const service = defaultService(name);
    updateDraft((current) => ({ ...current, services: [...current.services, service] }));
    setSelectedServiceId(service.id);
  };

  const syncYamlToVisual = async () => {
    setSyncingYaml(true);
    try {
      const response = await validateDockerComposeYaml({ projectName, workingDir, composeYaml: yamlText });
      if (!response.data.valid) {
        message.error(response.data.message || 'YAML 校验失败，无法同步到可视化');
        return;
      }
      const next = response.data.visualDraft
        ? normalizeDraft(response.data.visualDraft)
        : localParseYamlToDraft(yamlText, draft);
      setDraft(next);
      setSelectedServiceId(next.services[0]?.id || '');
      setYamlDirty(false);
      setYamlText(response.data.normalizedYaml || yamlText);
      message.success('已同步到可视化配置');
    } catch (error) {
      message.error((error as Error).message || '同步 YAML 失败');
    } finally {
      setSyncingYaml(false);
    }
  };

  const validateCurrentDockerfile = async () => {
    if (!selectedService) return;
    setValidatingDockerfile(true);
    try {
      const response = await previewDockerfileBuild({
        projectName,
        workingDir,
        serviceName: selectedService.serviceName,
        context: selectedService.build?.context || '.',
        dockerfilePath: selectedService.build?.dockerfile || 'Dockerfile',
        dockerfileContent: selectedService.dockerfileContent || DEFAULT_DOCKERFILE,
        imageTag: selectedService.image || `${selectedService.serviceName}:latest`,
        buildArgs: Object.entries(selectedService.build?.args || {}).map(([key, value]) => ({ key, value })),
      });
      setDockerfilePreview(response.data);
      message[response.data.valid ? 'success' : 'warning'](response.data.message || (response.data.valid ? 'Dockerfile 预览通过' : 'Dockerfile 预览存在问题'));
    } catch (error) {
      message.error((error as Error).message || 'Dockerfile 预览失败');
    } finally {
      setValidatingDockerfile(false);
    }
  };

  const refreshRepositories = async (registryId?: API.Int64, keyword?: string) => {
    if (!registryId) return;
    try {
      const response = await getDockerRepositories(registryId, { current: 1, size: 100, keyword });
      setRepositories(response.data.records || []);
    } catch (error) {
      message.error((error as Error).message || '加载远程仓库失败');
    }
  };

  const refreshTags = async (registryId?: API.Int64, repository?: string) => {
    if (!registryId || !repository) return;
    try {
      const response = await getDockerRepositoryTags(registryId, repository);
      setRemoteTags(response.data);
    } catch (error) {
      message.error((error as Error).message || '加载镜像标签失败');
    }
  };

  const addNetwork = () => updateDraft((current) => ({ ...current, networks: [...(current.networks || []), { name: `network-${(current.networks || []).length + 1}`, driver: 'bridge' }] }));
  const addVolume = () => updateDraft((current) => ({ ...current, volumes: [...(current.volumes || []), { name: `volume-${(current.volumes || []).length + 1}` }] }));

  const networkNames = (draft.networks || []).map((network) => network.name).filter(Boolean) as string[];
  const volumeNames = (draft.volumes || []).map((volume) => volume.name).filter(Boolean) as string[];

  const summary = useMemo(() => {
    const ports = draft.services.reduce((sum, service) => sum + (service.ports?.length || 0), 0);
    const envs = draft.services.reduce((sum, service) => sum + (service.environment?.length || 0), 0);
    const mounts = draft.services.reduce((sum, service) => sum + (service.volumes?.length || 0), 0);
    return { serviceCount: draft.services.length, ports, envs, mounts, networks: draft.networks?.length || 0, volumes: draft.volumes?.length || 0 };
  }, [draft]);

  const imageOptions = localImages.flatMap((image) => (image.repoTags?.length ? image.repoTags : [image.imageId]).map((tag) => ({ label: tag, value: tag })));

  const serviceList = (
    <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-3">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="text-sm font-semibold text-slate-950">服务列表</div>
          <div className="text-xs text-slate-400">点击服务进行配置</div>
        </div>
        {!readonly ? <Button size="small" icon={<PlusOutlined />} onClick={addService}>添加服务</Button> : null}
      </div>
      <div className={compact ? 'max-h-[560px] space-y-2 overflow-y-auto pr-1' : 'space-y-2'}>
        {draft.services.map((service) => (
          <button
            key={service.id}
            type="button"
            className={`w-full rounded-xl border px-3 py-3 text-left transition ${selectedService?.id === service.id ? 'border-blue-300 bg-blue-50/70' : 'border-slate-100 bg-white hover:border-blue-200 hover:bg-blue-50/30'}`}
            onClick={() => setSelectedServiceId(service.id)}
          >
            <div className="flex items-center justify-between gap-2">
              <div className="font-medium text-slate-950">{service.serviceName || '未命名服务'}</div>
              <Tag color="green" className="m-0">配置中</Tag>
            </div>
            <div className="mt-2 truncate text-xs text-slate-500">镜像：{serviceImage(service) || '-'}</div>
            <div className="mt-1 text-xs text-slate-500">端口：{(service.ports || []).map((p) => `${p.hostPort || p.containerPort}:${p.containerPort}`).join(', ') || '-'}</div>
          </button>
        ))}
      </div>
    </div>
  );

  const graph = (
    <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="text-sm font-semibold text-slate-950">服务拓扑</div>
          <div className="text-xs text-slate-400">根据 depends_on 展示依赖关系</div>
        </div>
        <Space size={4}>
          <Button size="small" icon={<ReloadOutlined />} onClick={() => setYamlDirty(false)}>刷新 YAML</Button>
        </Space>
      </div>
      <div className="relative min-h-[220px] rounded-2xl bg-slate-50 p-4">
        <div className={compact ? 'grid grid-cols-1 gap-4 lg:grid-cols-2' : 'grid grid-cols-2 gap-4 xl:grid-cols-3'}>
          {draft.services.map((service) => (
            <div key={service.id} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
              <div className="flex items-start justify-between gap-2">
                <div>
                  <div className="font-semibold text-slate-950">{service.serviceName}</div>
                  <div className="mt-1 text-xs text-slate-500">{serviceImage(service)}</div>
                </div>
                <Button size="small" type="text" onClick={() => setSelectedServiceId(service.id)}>编辑</Button>
              </div>
              <div className="mt-3 flex flex-wrap gap-1">
                {(service.ports || []).map((port, index) => <Tag key={index} color="blue">{port.hostPort || port.containerPort}:{port.containerPort}</Tag>)}
                {(service.dependsOn || []).map((dep) => <Tag key={dep}>依赖 {dep}</Tag>)}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );

  const selectedForm = selectedService ? (
    <div className="min-w-0 overflow-hidden rounded-2xl border border-slate-100 bg-white p-4">
      <Tabs
        size="small"
        tabBarGutter={compact ? 18 : undefined}
        items={[
          {
            key: 'base',
            label: '基础配置',
            children: (
              <div className="grid gap-4 lg:grid-cols-2">
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">服务名称</div>
                  <Input disabled={readonly} value={selectedService.serviceName} onChange={(e) => updateService(selectedService.id, { serviceName: e.target.value })} />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">镜像来源</div>
                  <Radio.Group
                    disabled={readonly}
                    value={selectedService.imageSource}
                    onChange={(e) => updateService(selectedService.id, { imageSource: e.target.value })}
                    options={[
                      { label: '本地镜像', value: 'local' },
                      { label: '远程镜像', value: 'remote' },
                      { label: '手动输入', value: 'manual' },
                      { label: 'Dockerfile 构建', value: 'dockerfile' },
                    ]}
                  />
                </div>
                {selectedService.imageSource === 'local' ? (
                  <div className="lg:col-span-2">
                    <div className="mb-2 text-sm font-medium text-slate-700">本地镜像</div>
                    <Select
                      showSearch
                      disabled={readonly}
                      className="w-full"
                      value={selectedService.localImage || selectedService.image}
                      options={imageOptions}
                      onChange={(image) => updateService(selectedService.id, { localImage: image, image })}
                    />
                  </div>
                ) : null}
                {selectedService.imageSource === 'remote' ? (
                  <>
                    <div>
                      <div className="mb-2 text-sm font-medium text-slate-700">Registry</div>
                      <Select
                        disabled={readonly}
                        className="w-full"
                        value={selectedService.registryId}
                        options={registries.map((registry) => ({ label: registry.name || registry.endpoint, value: registry.id }))}
                        onChange={(registryId) => {
                          updateService(selectedService.id, { registryId });
                          void refreshRepositories(registryId);
                        }}
                      />
                    </div>
                    <div>
                      <div className="mb-2 text-sm font-medium text-slate-700">Repository</div>
                      <Select
                        showSearch
                        disabled={readonly || !selectedService.registryId}
                        className="w-full"
                        value={selectedService.remoteRepository}
                        options={repositories.map((repo) => ({ label: repo.repository, value: repo.repository }))}
                        onSearch={(keyword) => void refreshRepositories(selectedService.registryId, keyword)}
                        onChange={(repository) => {
                          updateService(selectedService.id, { remoteRepository: repository, image: repository });
                          void refreshTags(selectedService.registryId, repository);
                        }}
                      />
                    </div>
                    <div className="lg:col-span-2">
                      <div className="mb-2 text-sm font-medium text-slate-700">Tag</div>
                      <Select
                        disabled={readonly || !selectedService.remoteRepository}
                        className="w-full"
                        value={selectedService.remoteTag}
                        options={(remoteTags?.tags || []).map((tag) => ({ label: tag, value: tag }))}
                        onChange={(tag) => updateService(selectedService.id, { remoteTag: tag, image: `${selectedService.remoteRepository}:${tag}` })}
                      />
                    </div>
                  </>
                ) : null}
                {selectedService.imageSource === 'manual' ? (
                  <div className="lg:col-span-2">
                    <div className="mb-2 text-sm font-medium text-slate-700">镜像地址</div>
                    <Input disabled={readonly} value={selectedService.image} placeholder="nginx:1.25-alpine" onChange={(e) => updateService(selectedService.id, { image: e.target.value })} />
                  </div>
                ) : null}
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">容器名</div>
                  <Input disabled={readonly} value={selectedService.containerName} onChange={(e) => updateService(selectedService.id, { containerName: e.target.value })} />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">重启策略</div>
                  <Select
                    disabled={readonly}
                    className="w-full"
                    value={selectedService.restart}
                    options={(metadata?.restartPolicies || ['no', 'always', 'unless-stopped', 'on-failure']).map((item) => ({ label: item, value: item }))}
                    onChange={(restart) => updateService(selectedService.id, { restart })}
                  />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">工作目录</div>
                  <Input disabled={readonly} value={selectedService.workingDir} onChange={(e) => updateService(selectedService.id, { workingDir: e.target.value })} />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">运行用户</div>
                  <Input disabled={readonly} value={selectedService.user} onChange={(e) => updateService(selectedService.id, { user: e.target.value })} />
                </div>
                <div className="lg:col-span-2">
                  <div className="mb-2 text-sm font-medium text-slate-700">启动命令</div>
                  <Input disabled={readonly} value={String(selectedService.command || '')} onChange={(e) => updateService(selectedService.id, { command: e.target.value })} />
                </div>
              </div>
            ),
          },
          {
            key: 'ports',
            label: '端口映射',
            children: (
              <div className="space-y-3">
                <div className="flex justify-end">
                  <Button disabled={readonly} size="small" icon={<PlusOutlined />} onClick={() => updateService(selectedService.id, { ports: [...(selectedService.ports || []), { hostIp: '0.0.0.0', hostPort: undefined, containerPort: undefined, protocol: 'tcp' }] })}>添加端口</Button>
                </div>
                <Table<DockerComposeVisualPortView & { rowId: string }>
                  size="small"
                  rowKey="rowId"
                  pagination={false}
                  dataSource={(selectedService.ports || []).map((row, index) => ({ ...row, rowId: String(index) }))}
                  columns={[
                    { title: '主机 IP', render: (_, row, index) => <Input disabled={readonly} value={row.hostIp} onChange={(e) => updateService(selectedService.id, { ports: (selectedService.ports || []).map((item, i) => (i === index ? { ...item, hostIp: e.target.value } : item)) })} /> },
                    { title: '主机端口', render: (_, row, index) => <Input disabled={readonly} value={row.hostPort} onChange={(e) => updateService(selectedService.id, { ports: (selectedService.ports || []).map((item, i) => (i === index ? { ...item, hostPort: e.target.value } : item)) })} /> },
                    { title: '容器端口', render: (_, row, index) => <Input disabled={readonly} value={row.containerPort} onChange={(e) => updateService(selectedService.id, { ports: (selectedService.ports || []).map((item, i) => (i === index ? { ...item, containerPort: e.target.value } : item)) })} /> },
                    { title: '协议', width: 110, render: (_, row, index) => <Select disabled={readonly} className="w-full" value={row.protocol || 'tcp'} options={[{ label: 'TCP', value: 'tcp' }, { label: 'UDP', value: 'udp' }]} onChange={(protocol) => updateService(selectedService.id, { ports: (selectedService.ports || []).map((item, i) => (i === index ? { ...item, protocol } : item)) })} /> },
                    { title: '操作', width: 70, render: (_, __, index) => <Button disabled={readonly} danger size="small" type="text" icon={<DeleteOutlined />} onClick={() => updateService(selectedService.id, { ports: (selectedService.ports || []).filter((_, i) => i !== index) })} /> },
                  ]}
                />
              </div>
            ),
          },
          {
            key: 'env',
            label: '环境变量',
            children: <PairEditor value={selectedService.environment || []} onChange={(environment) => updateService(selectedService.id, { environment })} addText="添加变量" />,
          },
          {
            key: 'volumes',
            label: '卷挂载',
            children: (
              <div className="space-y-3">
                <div className="flex justify-end">
                  <Button disabled={readonly} size="small" icon={<PlusOutlined />} onClick={() => updateService(selectedService.id, { volumes: [...(selectedService.volumes || []), { source: '', target: '', type: 'bind', readOnly: false }] })}>添加挂载</Button>
                </div>
                <Table<DockerComposeVisualVolumeMountView & { rowId: string }>
                  size="small"
                  rowKey="rowId"
                  pagination={false}
                  dataSource={(selectedService.volumes || []).map((row, index) => ({ ...row, rowId: String(index) }))}
                  columns={[
                    { title: '类型', width: 110, render: (_, row, index) => <Select disabled={readonly} className="w-full" value={row.type || 'bind'} options={[{ label: 'bind', value: 'bind' }, { label: 'volume', value: 'volume' }]} onChange={(type) => updateService(selectedService.id, { volumes: (selectedService.volumes || []).map((item, i) => (i === index ? { ...item, type } : item)) })} /> },
                    { title: '源路径/卷', render: (_, row, index) => row.type === 'volume' ? <Select disabled={readonly} className="w-full" value={row.source} options={[...volumeNames, ...dockerVolumes.map((v) => v.name)].filter(Boolean).map((name) => ({ label: name, value: name }))} onChange={(source) => updateService(selectedService.id, { volumes: (selectedService.volumes || []).map((item, i) => (i === index ? { ...item, source } : item)) })} /> : <Input disabled={readonly} value={row.source} onChange={(e) => updateService(selectedService.id, { volumes: (selectedService.volumes || []).map((item, i) => (i === index ? { ...item, source: e.target.value } : item)) })} /> },
                    { title: '容器路径', render: (_, row, index) => <Input disabled={readonly} value={row.target} onChange={(e) => updateService(selectedService.id, { volumes: (selectedService.volumes || []).map((item, i) => (i === index ? { ...item, target: e.target.value } : item)) })} /> },
                    { title: '只读', width: 80, render: (_, row, index) => <Checkbox disabled={readonly} checked={row.readOnly} onChange={(e) => updateService(selectedService.id, { volumes: (selectedService.volumes || []).map((item, i) => (i === index ? { ...item, readOnly: e.target.checked } : item)) })} /> },
                    { title: '操作', width: 70, render: (_, __, index) => <Button disabled={readonly} danger size="small" type="text" icon={<DeleteOutlined />} onClick={() => updateService(selectedService.id, { volumes: (selectedService.volumes || []).filter((_, i) => i !== index) })} /> },
                  ]}
                />
              </div>
            ),
          },
          {
            key: 'network',
            label: '网络配置',
            children: (
              <div className="space-y-4">
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">服务加入网络</div>
                  <Select
                    mode="multiple"
                    disabled={readonly}
                    className="w-full"
                    value={selectedService.networks || []}
                    options={[...networkNames, ...dockerNetworks.map((n) => n.name)].filter(Boolean).map((name) => ({ label: name, value: name }))}
                    onChange={(networks) => updateService(selectedService.id, { networks })}
                  />
                </div>
                <Divider className="my-2" />
                <div className="flex items-center justify-between">
                  <div className="text-sm font-semibold text-slate-950">项目网络</div>
                  <Button disabled={readonly} size="small" icon={<PlusOutlined />} onClick={addNetwork}>添加网络</Button>
                </div>
                <Table<DockerComposeVisualNetworkView & { rowId: string }>
                  size="small"
                  rowKey="rowId"
                  pagination={false}
                  dataSource={(draft.networks || []).map((row, index) => ({ ...row, rowId: String(index) }))}
                  columns={[
                    { title: '名称', render: (_, row, index) => <Input disabled={readonly} value={row.name} onChange={(e) => updateDraft((current) => ({ ...current, networks: (current.networks || []).map((item, i) => (i === index ? { ...item, name: e.target.value } : item)) }))} /> },
                    { title: 'Driver', width: 130, render: (_, row, index) => <Input disabled={readonly || row.external} value={row.driver} onChange={(e) => updateDraft((current) => ({ ...current, networks: (current.networks || []).map((item, i) => (i === index ? { ...item, driver: e.target.value } : item)) }))} /> },
                    { title: 'External', width: 90, render: (_, row, index) => <Checkbox disabled={readonly} checked={row.external} onChange={(e) => updateDraft((current) => ({ ...current, networks: (current.networks || []).map((item, i) => (i === index ? { ...item, external: e.target.checked } : item)) }))} /> },
                    { title: '操作', width: 70, render: (_, __, index) => <Button disabled={readonly} danger size="small" type="text" icon={<DeleteOutlined />} onClick={() => updateDraft((current) => ({ ...current, networks: (current.networks || []).filter((_, i) => i !== index) }))} /> },
                  ]}
                />
              </div>
            ),
          },
          {
            key: 'depends',
            label: '依赖关系',
            children: (
              <Select
                mode="multiple"
                disabled={readonly}
                className="w-full"
                value={selectedService.dependsOn || []}
                options={draft.services.filter((service) => service.id !== selectedService.id).map((service) => ({ label: service.serviceName, value: service.serviceName }))}
                onChange={(dependsOn) => updateService(selectedService.id, { dependsOn })}
              />
            ),
          },
          {
            key: 'healthcheck',
            label: '健康检查',
            children: (
              <div className="grid gap-4 lg:grid-cols-2">
                <div className="lg:col-span-2">
                  <Checkbox disabled={readonly} checked={selectedService.healthcheck?.disable} onChange={(e) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), disable: e.target.checked } })}>禁用健康检查</Checkbox>
                </div>
                <div className="lg:col-span-2">
                  <div className="mb-2 text-sm font-medium text-slate-700">检查命令</div>
                  <Input disabled={readonly || selectedService.healthcheck?.disable} value={Array.isArray(selectedService.healthcheck?.test) ? selectedService.healthcheck?.test.join(' ') : selectedService.healthcheck?.test} placeholder="curl -f http://localhost/health || exit 1" onChange={(e) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), test: e.target.value } })} />
                </div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">interval</div><Input disabled={readonly || selectedService.healthcheck?.disable} value={selectedService.healthcheck?.interval} placeholder={metadata?.healthcheckDefaults?.interval || '30s'} onChange={(e) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), interval: e.target.value } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">timeout</div><Input disabled={readonly || selectedService.healthcheck?.disable} value={selectedService.healthcheck?.timeout} placeholder={metadata?.healthcheckDefaults?.timeout || '5s'} onChange={(e) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), timeout: e.target.value } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">retries</div><InputNumber disabled={readonly || selectedService.healthcheck?.disable} className="w-full" value={selectedService.healthcheck?.retries} placeholder={metadata?.healthcheckDefaults?.retries === undefined ? undefined : String(metadata.healthcheckDefaults.retries)} onChange={(retries) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), retries: retries || undefined } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">start_period</div><Input disabled={readonly || selectedService.healthcheck?.disable} value={selectedService.healthcheck?.startPeriod} placeholder={metadata?.healthcheckDefaults?.startPeriod || '10s'} onChange={(e) => updateService(selectedService.id, { healthcheck: { ...(selectedService.healthcheck || {}), startPeriod: e.target.value } })} /></div>
              </div>
            ),
          },
          {
            key: 'resources',
            label: '资源限制',
            children: (
              <div className="grid gap-4 lg:grid-cols-2">
                <div><div className="mb-2 text-sm font-medium text-slate-700">CPU</div><Input disabled={readonly} value={selectedService.resources?.cpus} placeholder={metadata?.resourceLimitHints?.cpuExamples?.[0] || '1.0'} onChange={(e) => updateService(selectedService.id, { resources: { ...(selectedService.resources || {}), cpus: e.target.value } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">内存限制</div><Input disabled={readonly} value={selectedService.resources?.memory} placeholder={metadata?.resourceLimitHints?.memoryExamples?.[1] || '512M'} onChange={(e) => updateService(selectedService.id, { resources: { ...(selectedService.resources || {}), memory: e.target.value } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">内存保留</div><Input disabled={readonly} value={selectedService.resources?.memoryReservation} placeholder="256M" onChange={(e) => updateService(selectedService.id, { resources: { ...(selectedService.resources || {}), memoryReservation: e.target.value } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">PIDs 限制</div><InputNumber disabled={readonly} className="w-full" value={selectedService.resources?.pidsLimit} onChange={(pidsLimit) => updateService(selectedService.id, { resources: { ...(selectedService.resources || {}), pidsLimit: pidsLimit || undefined } })} /></div>
              </div>
            ),
          },
          {
            key: 'advanced',
            label: '高级选项',
            children: (
              <div className="grid gap-4 lg:grid-cols-2">
                <div className="lg:col-span-2">
                  <Alert showIcon type="warning" message="高级选项可能触发 Docker Safety Policy。建议仅在明确需要时配置。" />
                </div>
                <div className="lg:col-span-2"><Checkbox disabled={readonly} checked={selectedService.advanced?.privileged} onChange={(e) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), privileged: e.target.checked } })}>privileged</Checkbox></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">network_mode</div><Select allowClear disabled={readonly} className="w-full" value={selectedService.advanced?.networkMode} options={(metadata?.networkModes || ['bridge', 'host', 'none']).map((mode) => ({ label: mode, value: mode }))} onChange={(networkMode) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), networkMode } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">pid</div><Select allowClear disabled={readonly} className="w-full" value={selectedService.advanced?.pid} options={[{ label: 'host', value: 'host' }]} onChange={(pid) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), pid } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">ipc</div><Select allowClear disabled={readonly} className="w-full" value={selectedService.advanced?.ipc} options={[{ label: 'host', value: 'host' }]} onChange={(ipc) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), ipc } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">cap_add</div><Input disabled={readonly} value={splitWords(selectedService.advanced?.capAdd)} placeholder="NET_ADMIN, SYS_TIME" onChange={(e) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), capAdd: joinWords(e.target.value) } })} /></div>
                <div><div className="mb-2 text-sm font-medium text-slate-700">cap_drop</div><Input disabled={readonly} value={splitWords(selectedService.advanced?.capDrop)} placeholder="ALL" onChange={(e) => updateService(selectedService.id, { advanced: { ...(selectedService.advanced || {}), capDrop: joinWords(e.target.value) } })} /></div>
              </div>
            ),
          },
          {
            key: 'dockerfile',
            label: 'Dockerfile',
            disabled: selectedService.imageSource !== 'dockerfile',
            children: (
              <div className="space-y-4">
                <div className="grid gap-4 lg:grid-cols-3">
                  <div><div className="mb-2 text-sm font-medium text-slate-700">context</div><Input disabled={readonly} value={selectedService.build?.context} onChange={(e) => updateService(selectedService.id, { build: { ...(selectedService.build || {}), context: e.target.value } })} /></div>
                  <div><div className="mb-2 text-sm font-medium text-slate-700">Dockerfile 路径</div><Input disabled={readonly} value={selectedService.build?.dockerfile} onChange={(e) => updateService(selectedService.id, { build: { ...(selectedService.build || {}), dockerfile: e.target.value } })} /></div>
                  <div><div className="mb-2 text-sm font-medium text-slate-700">构建镜像 Tag</div><Input disabled={readonly} value={selectedService.image} onChange={(e) => updateService(selectedService.id, { image: e.target.value })} /></div>
                </div>
                <div>
                  <div className="mb-2 flex items-center justify-between">
                    <div className="text-sm font-medium text-slate-700">Dockerfile 内容</div>
                    <Button disabled={readonly} size="small" loading={validatingDockerfile} onClick={() => void validateCurrentDockerfile()}>预览 Dockerfile</Button>
                  </div>
                  <Input.TextArea disabled={readonly} value={selectedService.dockerfileContent || DEFAULT_DOCKERFILE} rows={10} className="font-mono text-xs" onChange={(e) => updateService(selectedService.id, { dockerfileContent: e.target.value })} />
                </div>
                <div>
                  <div className="mb-2 text-sm font-medium text-slate-700">Build Args</div>
                  <PairEditor value={Object.entries(selectedService.build?.args || {}).map(([key, value]) => ({ key, value }))} onChange={(args) => updateService(selectedService.id, { build: { ...(selectedService.build || {}), args: Object.fromEntries(args.filter((item) => item.key).map((item) => [item.key!, item.value || ''])) } })} addText="添加参数" />
                </div>
                {dockerfilePreview ? (
                  <Alert
                    showIcon
                    type={dockerfilePreview.valid ? 'success' : 'warning'}
                    message={dockerfilePreview.valid ? 'Dockerfile 预览通过' : 'Dockerfile 预览存在问题'}
                    description={dockerfilePreview.message || dockerfilePreview.resolvedDockerfilePath}
                  />
                ) : null}
              </div>
            ),
          },
        ]}
      />
    </div>
  ) : <Empty description="请选择服务" />;

  const previewPanel = (
    <div className="min-w-0 space-y-4">
      <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
        <div className="mb-3 flex items-center justify-between">
          <div className="font-semibold text-slate-950">实时预览</div>
          <Button size="small" icon={<SafetyCertificateOutlined />} onClick={() => void syncYamlToVisual()} loading={syncingYaml}>校验/同步</Button>
        </div>
        <div className={compact ? 'grid grid-cols-2 gap-2 text-center text-xs md:grid-cols-3' : 'grid grid-cols-3 gap-2 text-center text-xs'}>
          <div className="rounded-xl bg-blue-50 p-3 text-blue-600"><div className="text-lg font-semibold">{summary.serviceCount}</div><div>服务</div></div>
          <div className="rounded-xl bg-amber-50 p-3 text-amber-600"><div className="text-lg font-semibold">{summary.ports}</div><div>端口</div></div>
          <div className="rounded-xl bg-emerald-50 p-3 text-emerald-600"><div className="text-lg font-semibold">{summary.networks}</div><div>网络</div></div>
          <div className="rounded-xl bg-slate-50 p-3 text-slate-600"><div className="text-lg font-semibold">{summary.volumes}</div><div>卷</div></div>
          <div className="rounded-xl bg-slate-50 p-3 text-slate-600"><div className="text-lg font-semibold">{summary.envs}</div><div>环境变量</div></div>
          <div className="rounded-xl bg-slate-50 p-3 text-slate-600"><div className="text-lg font-semibold">{buildFiles.length}</div><div>Dockerfile</div></div>
        </div>
      </div>
      <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
        <div className="mb-3 flex items-center justify-between">
          <div className="font-semibold text-slate-950">YAML 预览</div>
          <Space>
            <Button size="small" icon={<CopyOutlined />} onClick={() => void navigator.clipboard.writeText(effectiveYaml).then(() => message.success('YAML 已复制'))}>复制</Button>
            {onSave ? <Button size="small" type="primary" icon={<SaveOutlined />} onClick={() => onSave({ visualDraft: cleanDraft, composeYaml: effectiveYaml, buildFiles, yamlDirty })}>保存</Button> : null}
          </Space>
        </div>
        <CompactCode value={effectiveYaml} maxHeight={compact ? 420 : 620} />
      </div>
    </div>
  );

  return (
    <div className="min-w-0 space-y-4">
      <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
        <Tabs
          activeKey={mode}
          onChange={(key) => setMode(key as ActiveEditorMode)}
          items={[
            { key: 'visual', label: '可视化设计' },
            { key: 'yaml', label: 'YAML 编辑' },
          ]}
        />
        <div className="shrink-0 text-xs text-slate-500">{yamlDirty ? 'YAML 有未同步修改' : '可视化与 YAML 已同步'}</div>
      </div>

      {mode === 'yaml' ? (
        <div className={compact ? 'grid min-w-0 gap-4' : 'grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_360px]'}>
          <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
            <div className="mb-3 flex items-center justify-between">
              <div className="font-semibold text-slate-950">Compose YAML</div>
              <Space>
                <Button icon={<SyncOutlined />} loading={syncingYaml} onClick={() => void syncYamlToVisual()}>同步到可视化</Button>
              </Space>
            </div>
            <Input.TextArea
              disabled={readonly}
              value={yamlText}
              rows={compact ? 18 : 26}
              className="font-mono text-xs"
              onChange={(e) => {
                setYamlDirty(true);
                setYamlText(e.target.value);
              }}
            />
          </div>
          {previewPanel}
        </div>
      ) : (
        <div className={compact ? 'grid min-w-0 gap-4 xl:grid-cols-[220px_minmax(0,1fr)]' : 'grid min-w-0 gap-4 xl:grid-cols-[260px_minmax(0,1fr)_360px]'}>
          {serviceList}
          <div className="min-w-0 space-y-4">
            {compact ? (
              <div className="grid min-w-0 gap-4 2xl:grid-cols-[minmax(0,1fr)_320px]">
                <div className="min-w-0 space-y-4">
                  {graph}
                  {selectedForm}
                </div>
                {previewPanel}
              </div>
            ) : (
              <>
                {graph}
                {selectedForm}
              </>
            )}
            <div className={compact ? 'grid gap-4' : 'grid gap-4 lg:grid-cols-2'}>
              <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
                <div className="mb-3 flex items-center justify-between">
                  <div className="font-semibold text-slate-950">项目命名卷</div>
                  <Button disabled={readonly} size="small" icon={<PlusOutlined />} onClick={addVolume}>添加卷</Button>
                </div>
                <Table<DockerComposeVisualVolumeView & { rowId: string }>
                  size="small"
                  rowKey="rowId"
                  pagination={false}
                  dataSource={(draft.volumes || []).map((row, index) => ({ ...row, rowId: String(index) }))}
                  columns={[
                    { title: '名称', render: (_, row, index) => <Input disabled={readonly} value={row.name} onChange={(e) => updateDraft((current) => ({ ...current, volumes: (current.volumes || []).map((item, i) => (i === index ? { ...item, name: e.target.value } : item)) }))} /> },
                    { title: 'External', width: 90, render: (_, row, index) => <Checkbox disabled={readonly} checked={row.external} onChange={(e) => updateDraft((current) => ({ ...current, volumes: (current.volumes || []).map((item, i) => (i === index ? { ...item, external: e.target.checked } : item)) }))} /> },
                    { title: '操作', width: 70, render: (_, __, index) => <Button disabled={readonly} danger size="small" type="text" icon={<DeleteOutlined />} onClick={() => updateDraft((current) => ({ ...current, volumes: (current.volumes || []).filter((_, i) => i !== index) }))} /> },
                  ]}
                />
              </div>
              <div className="min-w-0 rounded-2xl border border-slate-100 bg-white p-4">
                <div className="mb-3 font-semibold text-slate-950">辅助说明</div>
                <div className="space-y-2 text-sm leading-6 text-slate-500">
                  <p>网络、卷、资源限制、健康检查和高级选项都会实时映射到 Compose YAML。</p>
                  <p>最终风险判断以后端 Preview / Policy 为准，前端仅负责配置表达和同步。</p>
                  <p>Dockerfile 构建来源会生成 build 配置，并通过 buildFiles 提交给后端写入工作目录。</p>
                </div>
              </div>
            </div>
          </div>
          {compact ? null : previewPanel}
        </div>
      )}
    </div>
  );
}
