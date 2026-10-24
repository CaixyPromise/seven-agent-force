'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Alert, Button, Descriptions, Form, Input, Select, Space, Switch, Tag, message } from 'antd';
import {
  CheckCircleOutlined,
  GlobalOutlined,
  ReloadOutlined,
  SaveOutlined,
  SyncOutlined,
} from '@ant-design/icons';
import {
  getDockerDaemonConfig,
  getDockerRegistries,
  restartDockerDaemon,
  saveDockerDaemonConfig,
  validateDockerDaemonConfig,
  type DockerDaemonConfigView,
  type DockerRemoteRegistryView,
} from '@/api/dockerController';
import { usePermissionFlags } from '@/hooks/auth';
import { DOCKER_PERMISSIONS } from '@/lib/auth/permissionCodes';
import { DockerEmptyState, DockerSurfaceCard } from '../../components/dockerConsole';

interface DockerConfigTabProps {
  refreshToken?: number;
  onOpenRegistries?: () => void;
}

interface ConfigFormValues {
  registryMirrorsText?: string;
  insecureRegistriesText?: string;
  logDriver?: string;
  logOptsText?: string;
  liveRestore?: boolean;
  ipv6?: boolean;
  iptables?: boolean;
  bip?: string;
}

const editableKeyLabels: Record<string, string> = {
  'registry-mirrors': '镜像加速地址',
  'insecure-registries': '非安全仓库',
  'log-driver': '日志驱动',
  'log-opts': '日志选项',
  'live-restore': 'Live Restore',
  ipv6: 'IPv6',
  iptables: 'iptables',
  bip: '网桥 CIDR',
};

function readStringList(value: unknown) {
  return Array.isArray(value) ? value.map(String).join('\n') : '';
}

function readStringMap(value: unknown) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return '';
  }
  return Object.entries(value as Record<string, unknown>)
    .map(([key, mapValue]) => `${key}=${String(mapValue)}`)
    .join('\n');
}

function parseLines(value?: string) {
  return (value || '')
    .split(/[\n,]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseKeyValueLines(value?: string) {
  const result: Record<string, string> = {};
  (value || '')
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
    .forEach((line) => {
      const index = line.indexOf('=');
      if (index > 0) {
        result[line.slice(0, index).trim()] = line.slice(index + 1).trim();
      }
    });
  return result;
}

function toFormValues(config?: DockerDaemonConfigView | null): ConfigFormValues {
  const editable = config?.editable || {};
  return {
    registryMirrorsText: readStringList(editable['registry-mirrors']),
    insecureRegistriesText: readStringList(editable['insecure-registries']),
    logDriver: typeof editable['log-driver'] === 'string' ? editable['log-driver'] : undefined,
    logOptsText: readStringMap(editable['log-opts']),
    liveRestore: typeof editable['live-restore'] === 'boolean' ? editable['live-restore'] : undefined,
    ipv6: typeof editable.ipv6 === 'boolean' ? editable.ipv6 : undefined,
    iptables: typeof editable.iptables === 'boolean' ? editable.iptables : undefined,
    bip: typeof editable.bip === 'string' ? editable.bip : undefined,
  };
}

function toEditablePayload(values: ConfigFormValues) {
  const editable: Record<string, unknown> = {
    'registry-mirrors': parseLines(values.registryMirrorsText),
    'insecure-registries': parseLines(values.insecureRegistriesText),
    'log-opts': parseKeyValueLines(values.logOptsText),
  };
  if (values.logDriver?.trim()) {
    editable['log-driver'] = values.logDriver.trim();
  }
  if (values.liveRestore !== undefined) {
    editable['live-restore'] = values.liveRestore;
  }
  if (values.ipv6 !== undefined) {
    editable.ipv6 = values.ipv6;
  }
  if (values.iptables !== undefined) {
    editable.iptables = values.iptables;
  }
  if (values.bip?.trim()) {
    editable.bip = values.bip.trim();
  }
  return editable;
}

function readonlyKeys(config?: DockerDaemonConfigView | null) {
  return Object.keys(config?.readonly || {}).sort();
}

export function DockerConfigTab({ refreshToken = 0, onOpenRegistries }: DockerConfigTabProps) {
  const [form] = Form.useForm<ConfigFormValues>();
  const [config, setConfig] = useState<DockerDaemonConfigView | null>(null);
  const [registries, setRegistries] = useState<DockerRemoteRegistryView[]>([]);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const permissions = usePermissionFlags({
    canQuery: DOCKER_PERMISSIONS.CONFIG_QUERY,
    canValidate: DOCKER_PERMISSIONS.CONFIG_VALIDATE,
    canUpdate: DOCKER_PERMISSIONS.CONFIG_UPDATE,
    canRestart: DOCKER_PERMISSIONS.CONFIG_RESTART,
    canRegistries: DOCKER_PERMISSIONS.REGISTRY_LIST,
  });

  const loadConfig = useCallback(async () => {
    if (!permissions.canQuery) {
      setConfig(null);
      return;
    }
    setLoading(true);
    try {
      const [configResponse, registryResponse] = await Promise.allSettled([
        getDockerDaemonConfig(),
        permissions.canRegistries ? getDockerRegistries() : Promise.resolve({ data: [] as DockerRemoteRegistryView[] }),
      ]);
      if (configResponse.status === 'fulfilled') {
        setConfig(configResponse.value.data);
        form.setFieldsValue(toFormValues(configResponse.value.data));
      } else {
        throw configResponse.reason;
      }
      if (registryResponse.status === 'fulfilled') {
        setRegistries(registryResponse.value.data || []);
      }
    } catch (error) {
      message.error((error as Error).message || '加载 Docker daemon 配置失败');
      setConfig(null);
    } finally {
      setLoading(false);
    }
  }, [form, permissions.canQuery, permissions.canRegistries]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void loadConfig();
    }, 0);
    return () => window.clearTimeout(timer);
  }, [loadConfig, refreshToken]);

  const registrySummary = useMemo(() => {
    const enabled = registries.filter((registry) => registry.status !== 1);
    return {
      total: registries.length,
      enabled: enabled.length,
      defaultRegistry: enabled.find((registry) => registry.defaultRegistry),
    };
  }, [registries]);

  const handleValidate = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const response = await validateDockerDaemonConfig({ editable: toEditablePayload(values) });
      if (response.data.valid) {
        message.success('配置校验通过');
      } else {
        message.warning(response.data.message || '配置校验未通过');
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleSave = async () => {
    const values = await form.validateFields();
    setSubmitting(true);
    try {
      const response = await saveDockerDaemonConfig({ editable: toEditablePayload(values) });
      setConfig(response.data);
      form.setFieldsValue(toFormValues(response.data));
      message.success('daemon 配置已保存');
    } finally {
      setSubmitting(false);
    }
  };

  const handleRestart = async () => {
    setSubmitting(true);
    try {
      const response = await restartDockerDaemon();
      message.success(`Docker 重启任务已提交 #${response.data.operationId}`);
    } finally {
      setSubmitting(false);
    }
  };

  if (!permissions.canQuery) {
    return <DockerEmptyState title="无配置权限" description="当前账号没有查看 Docker daemon 配置的权限。" />;
  }

  const supported = !!config?.supported;
  const readOnly = readonlyKeys(config);

  return (
    <div className="space-y-5">
      <DockerSurfaceCard
        title="Docker daemon 配置"
        description="读取 daemon.json，仅允许编辑安全 allowlist 内的字段；未开放字段会保持只读并在保存时保留。"
        compact
        extra={
          <Space wrap>
            <Button icon={<ReloadOutlined />} loading={loading} onClick={() => void loadConfig()}>
              刷新
            </Button>
            {permissions.canValidate && supported ? (
              <Button icon={<CheckCircleOutlined />} loading={submitting} onClick={() => void handleValidate()}>
                校验
              </Button>
            ) : null}
            {permissions.canUpdate && supported ? (
              <Button type="primary" icon={<SaveOutlined />} loading={submitting} onClick={() => void handleSave()}>
                保存
              </Button>
            ) : null}
            {permissions.canRestart && supported ? (
              <Button danger icon={<SyncOutlined />} loading={submitting} onClick={() => void handleRestart()}>
                重启 Docker
              </Button>
            ) : null}
          </Space>
        }
      >
        {!config ? (
          <DockerEmptyState title="暂无配置数据" description="请刷新后重试，或确认后端 Docker 配置接口可用。" />
        ) : (
          <div className="space-y-5">
            {!supported ? (
              <Alert
                type="warning"
                showIcon
                message="当前环境暂不支持在线编辑 daemon 配置"
                description={config.supportReason || '仅支持可可靠定位 daemon.json 的 rootful Linux Docker daemon。'}
              />
            ) : null}
            <Descriptions bordered column={1}>
              <Descriptions.Item label="支持状态">
                <Tag color={supported ? 'success' : 'warning'}>{supported ? '支持编辑' : '不支持编辑'}</Tag>
              </Descriptions.Item>
              <Descriptions.Item label="平台">{config.platform || '-'}</Descriptions.Item>
              <Descriptions.Item label="Rootless">{config.rootless ? '是' : '否'}</Descriptions.Item>
              <Descriptions.Item label="配置路径">{config.configPath || '-'}</Descriptions.Item>
              <Descriptions.Item label="保存后是否需要重启">{config.requiresRestart ? '需要' : '不需要'}</Descriptions.Item>
              <Descriptions.Item label="可编辑字段">
                <Space size={4} wrap>
                  {(config.editableKeys || Object.keys(editableKeyLabels)).map((key) => (
                    <Tag key={key}>{editableKeyLabels[key] || key}</Tag>
                  ))}
                </Space>
              </Descriptions.Item>
              <Descriptions.Item label="只读字段">
                {readOnly.length ? (
                  <Space size={4} wrap>
                    {readOnly.map((key) => (
                      <Tag key={key} color="default">
                        {key}
                      </Tag>
                    ))}
                  </Space>
                ) : (
                  '无'
                )}
              </Descriptions.Item>
            </Descriptions>
          </div>
        )}
      </DockerSurfaceCard>

      {config ? (
        <DockerSurfaceCard
          title="Allowlist 编辑"
          description="每次保存只提交下方字段；daemon.json 中其它字段由后端合并保留。"
          compact
        >
          <Form<ConfigFormValues> form={form} layout="vertical" disabled={!supported || submitting}>
            <div className="grid gap-5 lg:grid-cols-2">
              <Form.Item
                label="registry-mirrors"
                name="registryMirrorsText"
                tooltip="每行一个镜像加速地址，也可用英文逗号分隔。"
              >
                <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder="https://mirror.example.com" />
              </Form.Item>
              <Form.Item
                label="insecure-registries"
                name="insecureRegistriesText"
                tooltip="每行一个 registry host:port，也可用英文逗号分隔。"
              >
                <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder="127.0.0.1:5000" />
              </Form.Item>
            </div>
            <div className="grid gap-5 lg:grid-cols-2">
              <Form.Item label="log-driver" name="logDriver">
                <Select
                  allowClear
                  showSearch
                  placeholder="选择或输入日志驱动"
                  options={[
                    { label: 'json-file', value: 'json-file' },
                    { label: 'local', value: 'local' },
                    { label: 'journald', value: 'journald' },
                    { label: 'syslog', value: 'syslog' },
                    { label: 'none', value: 'none' },
                  ]}
                />
              </Form.Item>
              <Form.Item label="bip" name="bip" tooltip="Docker bridge IP，例如 172.18.0.1/16">
                <Input placeholder="172.18.0.1/16" />
              </Form.Item>
            </div>
            <Form.Item label="log-opts" name="logOptsText" tooltip="每行一个 key=value，例如 max-size=20m">
              <Input.TextArea autoSize={{ minRows: 4, maxRows: 8 }} placeholder={'max-size=20m\nmax-file=3'} />
            </Form.Item>
            <div className="grid gap-5 md:grid-cols-3">
              <Form.Item label="live-restore" name="liveRestore" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item label="ipv6" name="ipv6" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item label="iptables" name="iptables" valuePropName="checked">
                <Switch />
              </Form.Item>
            </div>
          </Form>
        </DockerSurfaceCard>
      ) : null}

      <DockerSurfaceCard
        title="仓库配置摘要"
        description="远程 registry 配置仍在仓库 tab 管理；daemon 镜像源只维护 registry-mirrors 和 insecure-registries。"
        compact
        extra={
          <Button icon={<GlobalOutlined />} onClick={onOpenRegistries}>
            管理仓库
          </Button>
        }
      >
        <Descriptions bordered column={1}>
          <Descriptions.Item label="仓库配置数量">{registrySummary.total}</Descriptions.Item>
          <Descriptions.Item label="启用数量">{registrySummary.enabled}</Descriptions.Item>
          <Descriptions.Item label="默认仓库">
            {registrySummary.defaultRegistry ? (
              <Space wrap>
                <Tag color="processing">{registrySummary.defaultRegistry.name}</Tag>
                <span>{registrySummary.defaultRegistry.endpoint}</span>
              </Space>
            ) : (
              '未设置'
            )}
          </Descriptions.Item>
        </Descriptions>
      </DockerSurfaceCard>
    </div>
  );
}

export default DockerConfigTab;
