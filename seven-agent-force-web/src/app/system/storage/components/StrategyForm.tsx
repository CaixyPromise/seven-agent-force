'use client';

import React, { useEffect, useState } from 'react';
import {
    Drawer,
    Form,
    Input,
    Select,
    Switch,
    InputNumber,
    Button,
    message,
    Space,
    Divider
} from 'antd';
import { SaveOutlined, CloudServerOutlined } from '@ant-design/icons';
import { motion } from 'framer-motion';
import {
    createStorageStrategy,
    updateStorageStrategy
} from '@/api/storageStrategyController';

interface StrategyFormProps {
    open: boolean;
    strategy: API.StorageStrategy | null;
    onClose: () => void;
    onSuccess: () => void;
}

type PathRules = Record<string, Record<string, string>>;
type StorageConfigValue = string | number | boolean | PathRules | null | undefined;
type StorageConfigValues = Record<string, StorageConfigValue>;

interface ProviderConfigField {
    name: string;
    label: string;
    placeholder: string;
    type?: 'text' | 'password';
}

interface FormValidationError {
    errorFields: Array<{ name: Array<string | number>; errors: string[] }>;
}

function isFormValidationError(error: unknown): error is FormValidationError {
    return (
        typeof error === 'object' &&
        error !== null &&
        'errorFields' in error &&
        Array.isArray((error as { errorFields?: unknown }).errorFields)
    );
}

function getErrorMessage(error: unknown, fallback: string): string {
    return error instanceof Error && error.message ? error.message : fallback;
}

const SECRET_MASK = '******';
const SECRET_FIELDS = new Set([
    'accessKeyId',
    'accessKeySecret',
    'secretId',
    'secretKey',
    'secretAccessKey',
]);

const DEFAULT_PATH_RULES: PathRules = {
    PUBLIC_STATIC: {
        PUBLIC: 'public-static/',
        LOGIN_USERS: '',
        DELEGATED: '',
        OWNER_ONLY: '',
    },
    PRIVATE_PREVIEW: {
        OWNER_ONLY: 'clean/preview/private/',
        DELEGATED: 'clean/preview/biz/',
        LOGIN_USERS: '',
        PUBLIC: '',
    },
    PRIVATE_DOWNLOAD: {
        OWNER_ONLY: 'clean/raw/private/',
        DELEGATED: 'clean/raw/biz/',
        LOGIN_USERS: '',
        PUBLIC: '',
    },
    FORBIDDEN: {
        ANY: 'staging/',
    },
};

const PATH_RULES_SCHEMA: Array<{ strategy: string; label: string; scopes: string[] }> = [
    { strategy: 'PUBLIC_STATIC', label: '稳定直链', scopes: ['PUBLIC', 'LOGIN_USERS', 'DELEGATED', 'OWNER_ONLY'] },
    { strategy: 'PRIVATE_PREVIEW', label: '私有预览', scopes: ['OWNER_ONLY', 'DELEGATED', 'LOGIN_USERS', 'PUBLIC'] },
    { strategy: 'PRIVATE_DOWNLOAD', label: '私有下载', scopes: ['OWNER_ONLY', 'DELEGATED', 'LOGIN_USERS', 'PUBLIC'] },
    { strategy: 'FORBIDDEN', label: '禁止访问', scopes: ['ANY'] },
];

const SCOPE_LABELS: Record<string, string> = {
    OWNER_ONLY: '仅拥有者',
    DELEGATED: '业务委托',
    LOGIN_USERS: '登录用户',
    PUBLIC: '所有人',
    ANY: '任意',
};

const mergePathRules = (base: PathRules, override?: PathRules): PathRules => {
    const result: PathRules = {};
    Object.keys(base).forEach((strategy) => {
        const baseScopes = base[strategy] || {};
        const overrideScopes = override?.[strategy] || {};
        result[strategy] = { ...baseScopes, ...overrideScopes };
    });
    if (override) {
        Object.keys(override).forEach((strategy) => {
            if (!result[strategy]) {
                result[strategy] = { ...override[strategy] };
            }
        });
    }
    return result;
};

const ensurePathRules = (config: StorageConfigValues): StorageConfigValues => {
    const pathRules = config.pathRules;
    return {
        ...config,
        pathRules: mergePathRules(
            DEFAULT_PATH_RULES,
            typeof pathRules === 'object' && pathRules !== null ? pathRules : undefined,
        ),
    };
};

const buildConfigPayload = (values: StorageConfigValues, isEdit: boolean) => {
    const { pathRules, ...rest } = values || {};
    const cleaned: StorageConfigValues = {};
    Object.entries(rest).forEach(([key, value]) => {
        if (isEdit && SECRET_FIELDS.has(key) && (value === SECRET_MASK || value === '')) {
            return;
        }
        if (value !== undefined && value !== null && value !== '') {
            cleaned[key] = value;
        }
    });
    return {
        ...cleaned,
        pathRules,
    };
};

const providerConfigs: Record<string, { label: string; fields: ProviderConfigField[] }> = {
    LOCAL: {
        label: '本地存储',
        fields: [
            { name: 'basePath', label: '存储路径', placeholder: '/data/files' },
            { name: 'urlPrefix', label: 'URL前缀', placeholder: 'http://localhost:8080/files' },
        ],
    },
    ALIYUN_OSS: {
        label: '阿里云 OSS',
        fields: [
            { name: 'endpoint', label: 'Endpoint', placeholder: 'oss-cn-hangzhou.aliyuncs.com' },
            { name: 'accessKeyId', label: 'AccessKey ID', placeholder: '' },
            { name: 'accessKeySecret', label: 'AccessKey Secret', placeholder: '', type: 'password' },
            { name: 'bucketName', label: 'Bucket名称', placeholder: 'my-bucket' },
            { name: 'customDomain', label: '自定义域名', placeholder: 'cdn.example.com (可选)' },
        ],
    },
    AWS_S3: {
        label: 'AWS S3 / MinIO',
        fields: [
            { name: 'endpoint', label: 'Endpoint', placeholder: 'https://filestore.example.com (MinIO 可填)' },
            { name: 'region', label: 'Region', placeholder: 'us-east-1' },
            { name: 'accessKeyId', label: 'Access Key ID', placeholder: '' },
            { name: 'secretAccessKey', label: 'Secret Access Key', placeholder: '', type: 'password' },
            { name: 'bucketName', label: 'Bucket名称', placeholder: 'my-bucket' },
            { name: 'customDomain', label: '自定义域名', placeholder: 'cdn.example.com (可选)' },
        ],
    },
    TENCENT_COS: {
        label: '腾讯云 COS',
        fields: [
            { name: 'region', label: 'Region', placeholder: 'ap-guangzhou' },
            { name: 'secretId', label: 'Secret ID', placeholder: '' },
            { name: 'secretKey', label: 'Secret Key', placeholder: '', type: 'password' },
            { name: 'bucketName', label: 'Bucket名称', placeholder: 'my-bucket-1234567890' },
            { name: 'customDomain', label: '自定义域名', placeholder: 'cdn.example.com (可选)' },
        ],
    },
};

const StrategyForm: React.FC<StrategyFormProps> = ({
    open,
    strategy,
    onClose,
    onSuccess,
}) => {
    const [form] = Form.useForm<API.StorageStrategy>();
    const [loading, setLoading] = useState(false);
    const [selectedProvider, setSelectedProvider] = useState<string>('');
    const [configForm] = Form.useForm<StorageConfigValues>();

    const isEdit = !!strategy;

    useEffect(() => {
        if (open) {
            if (strategy) {
                form.setFieldsValue({
                    ...strategy,
                });
                setSelectedProvider(strategy.providerType || '');

                // 解析配置JSON
                try {
                    const config = JSON.parse(strategy.configJson || '{}') as StorageConfigValues;
                    configForm.setFieldsValue(ensurePathRules(config));
                } catch (e) {
                    console.error('Failed to parse config:', e);
                }
            } else {
                form.resetFields();
                configForm.setFieldsValue(ensurePathRules({}));
                setSelectedProvider('');
            }
        }
    }, [open, strategy, form, configForm]);

    const handleSubmit = async () => {
        try {
            const values = await form.validateFields();
            const configValues = await configForm.validateFields();

            setLoading(true);

            const configJson = JSON.stringify(buildConfigPayload(configValues, isEdit));
            let res;
            if (isEdit) {
                res = await updateStorageStrategy(strategy!.id!, {
                    ...values,
                    configJson,
                });
            } else {
                const strategyName = values.strategyName?.trim();
                const providerType = values.providerType?.trim();
                if (!strategyName || !providerType) {
                    throw new Error('策略名称和存储提供商不能为空');
                }
                res = await createStorageStrategy({
                    ...values,
                    strategyName,
                    providerType,
                    configJson,
                });
            }

            if (res.code === 0) {
                message.success(isEdit ? '更新成功' : '创建成功');
                onSuccess();
            } else {
                throw new Error(res.message);
            }
        } catch (error) {
            if (isFormValidationError(error)) {
                return; // Form validation error
            }
            message.error(getErrorMessage(error, '操作失败'));
        } finally {
            setLoading(false);
        }
    };

    const renderConfigFields = () => {
        const config = providerConfigs[selectedProvider];
        if (!config) return null;

        return (
            <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: 'auto' }}
                exit={{ opacity: 0, height: 0 }}
            >
                <Divider titlePlacement="left" className="text-sm">
                    {config.label} 配置
                </Divider>
                    {config.fields.map((field) => (
                    <Form.Item
                        key={field.name}
                        name={field.name}
                        label={field.label}
                            rules={[{ required: !isEdit && !field.placeholder?.includes('可选') }]}
                    >
                        <Input
                            placeholder={field.placeholder}
                            type={field.type || 'text'}
                            autoComplete={field.type === 'password' ? 'new-password' : 'off'}
                            disabled={isEdit && SECRET_FIELDS.has(field.name)}
                        />
                    </Form.Item>
                ))}
            </motion.div>
        );
    };

    const renderPolicyFields = () => {
        return (
            <motion.div
                initial={{ opacity: 0, height: 0 }}
                animate={{ opacity: 1, height: 'auto' }}
                exit={{ opacity: 0, height: 0 }}
            >
                <Divider titlePlacement="left" className="text-sm">
                    访问路径 Policy
                </Divider>
                {PATH_RULES_SCHEMA.map((group) => (
                    <div key={group.strategy} className="mb-4">
                        <div className="text-xs text-gray-500 mb-2">{group.label}</div>
                        <div className="grid grid-cols-2 gap-3">
                            {group.scopes.map((scope) => (
                                <Form.Item
                                    key={`${group.strategy}-${scope}`}
                                    name={['pathRules', group.strategy, scope]}
                                    label={SCOPE_LABELS[scope] || scope}
                                >
                                    <Input placeholder="例如：clean/preview/private/" />
                                </Form.Item>
                            ))}
                        </div>
                    </div>
                ))}
            </motion.div>
        );
    };

    return (
        <Drawer
            title={
                <div className="flex items-center gap-2">
                    <CloudServerOutlined className="text-purple-500" />
                    <span>{isEdit ? '编辑存储策略' : '添加存储策略'}</span>
                </div>
            }
            open={open}
            onClose={onClose}
            size={480}
            extra={
                <Space>
                    <Button onClick={onClose}>取消</Button>
                    <Button
                        type="primary"
                        icon={<SaveOutlined />}
                        onClick={handleSubmit}
                        loading={loading}
                        className="bg-gradient-to-r from-purple-500 to-purple-600"
                    >
                        保存
                    </Button>
                </Space>
            }
        >
            <Form form={form} layout="vertical">
                <Form.Item
                    name="strategyName"
                    label="策略名称"
                    rules={[{ required: true, message: '请输入策略名称' }]}
                >
                    <Input placeholder="例如：阿里云OSS主存储" />
                </Form.Item>

                <Form.Item
                    name="providerType"
                    label="存储类型"
                    rules={[{ required: true, message: '请选择存储类型' }]}
                >
                    <Select
                        placeholder="选择存储类型"
                        onChange={(value) => {
                            setSelectedProvider(value);
                            configForm.resetFields();
                            configForm.setFieldsValue(ensurePathRules({}));
                        }}
                        disabled={isEdit}
                        options={Object.entries(providerConfigs).map(([key, value]) => ({
                            label: value.label,
                            value: key,
                        }))}
                    />
                </Form.Item>

                <div className="grid grid-cols-2 gap-4">
                    <Form.Item
                        name="priority"
                        label="优先级"
                        initialValue={0}
                    >
                        <InputNumber min={0} max={100} className="w-full" />
                    </Form.Item>

                    <Form.Item
                        name="failureRateThreshold"
                        label="故障率阈值(%)"
                        initialValue={10}
                    >
                        <InputNumber min={1} max={100} className="w-full" />
                    </Form.Item>
                </div>

                <div className="grid grid-cols-2 gap-4">
                    <Form.Item
                        name="isEnabled"
                        label="启用状态"
                        valuePropName="checked"
                        initialValue={true}
                    >
                        <Switch />
                    </Form.Item>

                    <Form.Item
                        name="isDefault"
                        label="设为默认"
                        valuePropName="checked"
                        initialValue={false}
                    >
                        <Switch />
                    </Form.Item>
                </div>
            </Form>

            {/* 动态配置字段 */}
            <Form form={configForm} layout="vertical">
                {selectedProvider && renderConfigFields()}
                {renderPolicyFields()}
            </Form>
        </Drawer>
    );
};

export default StrategyForm;
