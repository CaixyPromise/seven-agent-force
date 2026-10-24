'use client';

import React, { useEffect, useRef } from 'react';
import {
    Button,
    Card,
    Col,
    Form,
    Input,
    Modal,
    Row,
    message,
    Tag,
    Typography,
} from 'antd';
import {
    UserOutlined,
    MailOutlined,
    PhoneOutlined,
    IdcardOutlined,
    CameraOutlined,
} from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
    getCurrentUserProfile,
    updateCurrentUserEmail,
    type UserSelfEmailUpdateRequest,
    type UserSelfProfileUpdateRequest,
    commitCurrentUserAvatar,
    updateCurrentUserProfile,
} from '@/api/userProfileController';
import { useAuthStore } from '@/store/auth';
import { isChallengeRetryError } from '@/lib/http/challenge-orchestrator';
import { checkFileExist, uploadFile } from '@/api/fileController';
import { computeSha256Hex } from '@/utils/crypto';
import PixelAvatar from '@/components/user/PixelAvatar';

const { Title, Text } = Typography;

type AvatarCommitResult = {
    userAvatar?: string;
    avatar?: string;
};

function getErrorMessage(error: unknown, fallback: string): string {
    return error instanceof Error && error.message ? error.message : fallback;
}

interface EmailChangeModalProps {
    open: boolean;
    initialEmail?: string;
    emailVersion: API.Int64;
    loading: boolean;
    onCancel: () => void;
    onSubmit: (payload: UserSelfEmailUpdateRequest) => void;
}

function EmailChangeModal({
    open,
    initialEmail,
    emailVersion,
    loading,
    onCancel,
    onSubmit,
}: EmailChangeModalProps) {
    const [form] = Form.useForm<UserSelfEmailUpdateRequest>();

    useEffect(() => {
        if (!open) {
            form.resetFields();
            return;
        }
        form.setFieldsValue({
            userEmail: initialEmail,
            expectedEmailVersion: emailVersion,
        });
    }, [emailVersion, form, initialEmail, open]);

    if (!open) {
        return null;
    }

    return (
        <Modal
            title="修改邮箱"
            open={open}
            onCancel={() => {
                if (loading) {
                    return;
                }
                onCancel();
            }}
            onOk={() => form.submit()}
            okText="提交修改"
            confirmLoading={loading}
            destroyOnHidden
        >
            <Form
                form={form}
                layout="vertical"
                onFinish={onSubmit}
                requiredMark={false}
            >
                <Form.Item name="expectedEmailVersion" hidden>
                    <Input />
                </Form.Item>
                <Form.Item
                    label="新邮箱"
                    name="userEmail"
                    rules={[
                        { required: true, message: '请输入新邮箱' },
                        { type: 'email', message: '请输入有效的邮箱地址' },
                    ]}
                >
                    <Input
                        prefix={<MailOutlined className="text-slate-400" />}
                        placeholder="请输入新的登录邮箱"
                        size="large"
                        className="rounded-lg"
                    />
                </Form.Item>
                <Text type="secondary" className="text-xs block">
                    为保护账号，先确认当前账号，再验证新邮箱。两步完成后才会生效。
                </Text>
            </Form>
        </Modal>
    );
}

export default function ProfileSection() {
    const queryClient = useQueryClient();
    const [form] = Form.useForm<UserSelfProfileUpdateRequest>();
    const authUser = useAuthStore((state) => state.user);
    const setAuthUser = useAuthStore((state) => state.setUser);
    const [emailModalOpen, setEmailModalOpen] = React.useState(false);
    const [avatarUploading, setAvatarUploading] = React.useState(false);
    const avatarInputRef = useRef<HTMLInputElement | null>(null);

    const { data: profileDataResponse, refetch } = useQuery({
        queryKey: ['account-settings', 'profile'],
        queryFn: () => getCurrentUserProfile(),
    });

    const profileData = profileDataResponse?.data;
    const profileEmail = profileData?.userEmail || profileData?.email || '';
    const profileEmailVersion = profileData?.emailVersion || '1';
    const profileEmailVerified = Boolean(profileData?.emailVerified);
    const profileAccount = profileData?.userAccount || profileData?.accountName || authUser?.username || '';
    const profilePhone = profileData?.userPhone || profileData?.phone || '';
    const profileIntro = profileData?.userProfile ?? profileData?.profile ?? '';

    useEffect(() => {
        if (profileData) {
            form.setFieldsValue({
                nickName: profileData.nickName,
                userPhone: profilePhone,
                userProfile: profileIntro,
            });
        }
    }, [profileData, profilePhone, profileIntro, form]);

    const updateProfileMutation = useMutation({
        mutationFn: (payload: UserSelfProfileUpdateRequest) => updateCurrentUserProfile(payload),
        onSuccess: async () => {
            message.success('个人资料已更新');
            await refetch();
            await queryClient.invalidateQueries({ queryKey: ['auth', 'currentUser'] });
            const values = form.getFieldsValue();
            setAuthUser({
                ...authUser,
                nickname: values.nickName ?? authUser?.nickname,
            });
        },
        onError: (error: Error) => {
            if (isChallengeRetryError(error, 'CHALLENGE_CANCELLED')) {
                return;
            }
            message.error(error.message || '保存失败，请稍后重试');
        },
    });

    const updateEmailMutation = useMutation({
        mutationFn: (payload: UserSelfEmailUpdateRequest) => updateCurrentUserEmail(payload),
        onSuccess: async () => {
            message.success('邮箱已更新');
            setEmailModalOpen(false);
            await refetch();
            await queryClient.invalidateQueries({ queryKey: ['auth', 'currentUser'] });
        },
        onError: (error: Error) => {
            if (isChallengeRetryError(error, 'CHALLENGE_CANCELLED')) {
                return;
            }
			if (
				error.message?.includes('PROFILE_EMAIL_VERSION_CONFLICT') ||
				error.message?.includes('其他位置更新')
			) {
                message.warning('邮箱信息已在其他位置更新，请刷新后重试');
                void refetch();
                return;
            }
            message.error(error.message || '邮箱修改失败，请稍后重试');
        },
    });

    const resolveFileId = (value: unknown): API.Int64 | undefined => {
        if (!value) {
            return undefined;
        }
        if (typeof value === 'string' && /^[1-9]\d*$/.test(value)) {
            return value;
        }
        if (typeof value === 'object') {
            const candidate = (value as Record<string, unknown>).fileId;
            return resolveFileId(candidate);
        }
        return undefined;
    };

    const handleAvatarFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        event.target.value = '';
        if (!file) {
            return;
        }
        if (!file.type.startsWith('image/')) {
            message.error('请选择图片文件');
            return;
        }
        setAvatarUploading(true);
        try {
            const sha256 = await computeSha256Hex(file);
            const checkResult = await checkFileExist({ sha256, size: file.size });
            if (checkResult.code !== 0) {
                throw new Error(checkResult.message || '头像上传前检查失败');
            }

            let fileId = resolveFileId(checkResult.data);
            if (!fileId) {
                const uploadResult = await uploadFile(file);
                if (uploadResult.code !== 0) {
                    throw new Error(uploadResult.message || '头像上传失败');
                }
                fileId = resolveFileId(uploadResult.data);
            }
            if (!fileId) {
                const latestCheck = await checkFileExist({ sha256, size: file.size });
                fileId = resolveFileId(latestCheck.data);
            }
            if (!fileId) {
                throw new Error('头像文件ID获取失败');
            }

            const commitResult = await commitCurrentUserAvatar({ fileId });
            if (commitResult.code !== 0) {
                throw new Error(commitResult.message || '头像提交失败');
            }
            const avatarData =
                commitResult.data && typeof commitResult.data === 'object'
                    ? (commitResult.data as AvatarCommitResult)
                    : undefined;
            const avatarUrl = avatarData?.userAvatar || avatarData?.avatar || profileData?.userAvatar;
            message.success('头像已更新');
            await refetch();
            await queryClient.invalidateQueries({ queryKey: ['auth', 'currentUser'] });
            setAuthUser({
                ...authUser,
                userAvatar: avatarUrl ?? authUser?.userAvatar,
                avatar: avatarUrl ?? authUser?.avatar,
            });
        } catch (error) {
            message.error(getErrorMessage(error, '头像上传失败'));
        } finally {
            setAvatarUploading(false);
        }
    };

    const accountEnabled =
        typeof profileData?.enabled === 'boolean' ? profileData.enabled : profileData?.status === 0;
    const avatarSeed = profileData?.userAccount || authUser?.username || profileData?.nickName || authUser?.nickname;

    return (
        <div className="animate-fade-in">
            {/* 这是一个模仿 Prototype 样式的 Profile Header */}
            <Card
                variant="borderless"
                className="shadow-sm rounded-xl overflow-hidden"
                styles={{ body: { padding: '24px' } }}
                style={{ marginBottom: 24 }}
            >
                <div className="flex flex-col sm:flex-row items-center gap-6">
                    <div className="relative">
                        <button
                            type="button"
                            className="relative group rounded-full focus:outline-none focus:ring-4 focus:ring-sky-100"
                            onClick={() => avatarInputRef.current?.click()}
                            aria-label="更换头像"
                        >
                            <PixelAvatar
                                size={100}
                                src={profileData?.userAvatar || authUser?.userAvatar || undefined}
                                seed={avatarSeed}
                                icon={<UserOutlined />}
                                className="border-4 border-white shadow-md bg-indigo-50 text-indigo-500"
                            />
                            <div className="absolute inset-0 rounded-full bg-slate-950/0 group-hover:bg-slate-950/45 group-focus-visible:bg-slate-950/45 transition-colors flex items-center justify-center">
                                <div className="opacity-0 group-hover:opacity-100 group-focus-visible:opacity-100 transition-opacity h-10 w-10 rounded-full bg-white/95 text-sky-600 shadow flex items-center justify-center">
                                    <CameraOutlined />
                                </div>
                            </div>
                            {avatarUploading ? (
                                <div className="absolute inset-0 rounded-full bg-white/60 flex items-center justify-center text-xs text-slate-700">
                                    上传中
                                </div>
                            ) : null}
                        </button>
                        <input
                            ref={avatarInputRef}
                            type="file"
                            accept="image/*"
                            className="hidden"
                            onChange={handleAvatarFileChange}
                        />
                    </div>
                    <div className="flex-1 text-center sm:text-left">
                        <Title level={3} style={{ marginBottom: 4 }}>
                            {profileData?.nickName || authUser?.nickname || 'User'}
                        </Title>
                        <Text type="secondary" className="block mb-2">
                            账号: {profileData?.userAccount || authUser?.username}
                        </Text>
                        <div className="flex flex-wrap justify-center sm:justify-start gap-2">
                            <Tag color={accountEnabled ? 'success' : 'error'}>
                                {accountEnabled ? '账号正常' : '账号异常'}
                            </Tag>
                            <Tag color="blue">系统用户</Tag>
                        </div>
                    </div>
                </div>
            </Card>

            <Card
                title={<span className="font-semibold text-lg">基本信息</span>}
                variant="borderless"
                className="shadow-sm rounded-xl"
            >
                <Form
                    form={form}
                    layout="vertical"
                    onFinish={(values) => updateProfileMutation.mutate(values)}
                    requiredMark={false}
                >
                    <Row gutter={[32, 20]}>
                        <Col xs={24} md={12}>
                            <Form.Item
                                label="用户昵称"
                                name="nickName"
                                rules={[{ required: true, message: '请输入用户昵称' }]}
                            >
                                <Input
                                    prefix={<UserOutlined className="text-slate-400" />}
                                    placeholder="请输入用户昵称"
                                    size="large"
                                    className="rounded-lg"
                                />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item label="登录账号">
                                <Input
                                    prefix={<IdcardOutlined className="text-slate-400" />}
                                    value={profileAccount}
                                    disabled
                                    size="large"
                                    className="rounded-lg bg-slate-50 text-slate-500"
                                />
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item label="电子邮箱">
                                <div className="space-y-2">
                                    <Input
                                        prefix={<MailOutlined className="text-slate-400" />}
                                        value={profileEmail}
                                        disabled
                                        size="large"
                                        className="rounded-lg bg-slate-50 text-slate-500"
                                    />
                                    <div className="flex items-center justify-between gap-3">
                                        <div className="flex items-center gap-2">
                                            <Tag color={profileEmailVerified ? 'green' : 'gold'}>
                                                {profileEmailVerified ? '已验证' : '未验证'}
                                            </Tag>
                                            <Text type="secondary" className="text-xs block">
                                                {profileEmailVerified ? '可用于接收邮件通知' : '验证后可接收邮件通知'}
                                            </Text>
                                        </div>
                                        <Button
                                            type="link"
                                            className="px-0"
                                            onClick={() => setEmailModalOpen(true)}
                                        >
                                            修改邮箱
                                        </Button>
                                    </div>
                                </div>
                            </Form.Item>
                        </Col>
                        <Col xs={24} md={12}>
                            <Form.Item label="手机号码" name="userPhone">
                                <Input
                                    prefix={<PhoneOutlined className="text-slate-400" />}
                                    placeholder="请输入手机号"
                                    maxLength={20}
                                    size="large"
                                    className="rounded-lg"
                                />
                            </Form.Item>
                            <Text type="secondary" className="text-xs -mt-3 mb-3 block">
                                修改手机号需要先完成安全挑战
                            </Text>
                        </Col>
                        <Col span={24}>
                            <Form.Item label="个人简介" name="userProfile">
                                <Input.TextArea
                                    rows={4}
                                    placeholder="介绍一下你自己..."
                                    maxLength={300}
                                    showCount
                                    className="rounded-lg"
                                />
                            </Form.Item>
                        </Col>
                    </Row>

                    <div className="flex justify-end pt-4 border-t border-slate-100 mt-4">
                        <Button
                            type="primary"
                            htmlType="submit"
                            size="large"
                            loading={updateProfileMutation.isPending}
                            className="px-8 rounded-lg font-medium"
                        >
                            保存更改
                        </Button>
                    </div>
                </Form>
            </Card>

            {emailModalOpen ? (
                <EmailChangeModal
                    open={emailModalOpen}
                    initialEmail={profileEmail}
                    emailVersion={profileEmailVersion}
                    loading={updateEmailMutation.isPending}
                    onCancel={() => setEmailModalOpen(false)}
                    onSubmit={(values) => updateEmailMutation.mutate(values)}
                />
            ) : null}
        </div>
    );
}
