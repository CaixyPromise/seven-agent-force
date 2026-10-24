'use client';

import React, { useEffect, useRef, useState } from 'react';
import { Alert, Button, Card, Col, Drawer, Form, Input, Row, Space, Switch, Table, Tag, Typography, message } from 'antd';
import { CheckCircleFilled, ExportOutlined, SettingOutlined, WarningFilled } from '@ant-design/icons';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { getEnterpriseConnectionSetup, listEnterpriseConnections, listEnterpriseIdentities, saveEnterpriseConnection, type EnterpriseConnectionInput, type EnterpriseConnectionRecord, type EnterpriseIdentityRecord } from '@/api/externalLoginController';
import { FEISHU_DEVELOPER_CONSOLE_URL, FEISHU_REDIRECT_GUIDE_URL, WECOM_ADMIN_APPLICATIONS_URL, WECOM_MESSAGE_GUIDE_URL, WECOM_OAUTH_GUIDE_URL } from '@/lib/enterprise-identity-feedback';

const { Paragraph, Text, Title } = Typography;
type Provider = 'FEISHU' | 'WECOM';
const providerMeta: Record<Provider, { name: string; color: string }> = { FEISHU: { name: '飞书', color: '#3370ff' }, WECOM: { name: '企业微信', color: '#07c160' } };

type EnterpriseConnectionSaveFailure = {
  title: string;
  description: string;
  action?: 'OPEN_ACCOUNT_SECURITY';
};

function enterpriseConnectionSaveFeedback(error: unknown): EnterpriseConnectionSaveFailure {
  const errorMessage = error instanceof Error ? error.message.trim() : '';
  const payload = (error as { payload?: { message?: unknown; traceId?: unknown; data?: unknown } })?.payload;
  const payloadMessage = payload?.message;
  const serverMessage = typeof payloadMessage === 'string' ? payloadMessage.trim() : '';
  const traceId = typeof payload?.traceId === 'string' ? payload.traceId.trim() : '';
  const reasonCode = payload?.data && typeof payload.data === 'object' && typeof (payload.data as { reasonCode?: unknown }).reasonCode === 'string'
    ? String((payload.data as { reasonCode: string }).reasonCode).trim()
    : '';
  const withReference = (description: string) => traceId ? `${description} 参考编号：${traceId}` : description;

  if (
    errorMessage === '当前账号未绑定可用的二次验证方式，请先在账号设置绑定后再操作'
    || serverMessage === '当前主体没有可用的验证方式'
  ) {
    return {
      title: '需要先完成账号安全设置',
      description: '请先到账户设置的“账号安全”绑定身份验证器或通行密钥，再回来保存。已填写的连接信息会保留在当前页面。',
      action: 'OPEN_ACCOUNT_SECURITY',
    };
  }

  if (errorMessage === '连接检查未通过' || serverMessage === '连接检查未通过') {
    return {
      title: '连接检查未通过',
      description: '请确认应用 ID、应用密钥和应用权限配置正确，然后重新保存。',
    };
  }

  if (reasonCode === 'ENTERPRISE_CONNECTION_NOT_VERIFIED') {
    return {
      title: '应用凭证检查未通过',
      description: withReference('请确认 App ID 和应用密钥来自同一个飞书应用，并确认应用当前可用。已填写的内容会保留。'),
    };
  }

  if (reasonCode === 'ENTERPRISE_CONNECTION_WECOM_TRUSTED_IP_REQUIRED') {
	return {
		title: '需要配置企业可信 IP',
		description: withReference('企业微信已接受应用凭证，但拒绝了 Seven 服务端公网出口 IP。请在“应用管理 → 当前自建应用 → 开发者接口 → 企业可信 IP”中放行服务端出口 IP，然后重新保存并检查。'),
	};
  }

  if (reasonCode === 'ENTERPRISE_CONNECTION_WECOM_AGENT_MISMATCH') {
	return {
		title: 'Agent ID 与应用密钥不匹配',
		description: withReference('请从同一个企业微信自建应用复制 Agent ID 和 Secret，再重新保存并检查。'),
	};
  }

  if (reasonCode === 'ENTERPRISE_AUTHORIZATION_REDIRECT_INVALID') {
    return {
      title: '平台回调地址尚未就绪',
      description: withReference('请先配置平台公开访问地址，再把平台生成的 OAuth 回调地址添加到飞书开发者后台。'),
    };
  }

  if (errorMessage === '连接已在其他位置更新' || serverMessage === '连接已在其他位置更新') {
    return {
      title: '连接配置已经更新',
      description: '请关闭当前窗口，重新打开连接后再修改。',
    };
  }

  if (
    errorMessage === '连接已验证，但个人通知通道初始化失败，请稍后重试'
    || serverMessage === '连接已验证，但个人通知通道初始化失败，请稍后重试'
  ) {
    return {
      title: '连接已验证，但通知初始化失败',
      description: withReference('飞书应用凭证检查已经通过，但平台暂时无法创建个人通知通道。请稍后重试；如持续失败，请将参考编号提供给管理员。'),
    };
  }

  return {
    title: '暂时无法保存并检查连接',
    description: withReference(serverMessage || errorMessage || '系统处理连接时发生异常，已保留填写内容。请稍后重试。'),
  };
}

export default function EnterpriseConnectionsPage() {
  const queryClient = useQueryClient();
  const [form] = Form.useForm<EnterpriseConnectionInput>();
  const [editing, setEditing] = useState<{ provider: Provider; record?: EnterpriseConnectionRecord } | null>(null);
  const [saveFailure, setSaveFailure] = useState<EnterpriseConnectionSaveFailure | null>(null);
  const saveFailureRef = useRef<HTMLDivElement>(null);
  const { data = [], isLoading } = useQuery({ queryKey: ['enterprise-connections'], queryFn: listEnterpriseConnections });
  const { data: connectionSetup, isLoading: setupLoading } = useQuery({ queryKey: ['enterprise-connections', 'setup'], queryFn: getEnterpriseConnectionSetup });
  const { data: identities = [], isLoading: identitiesLoading } = useQuery({ queryKey: ['enterprise-identities'], queryFn: listEnterpriseIdentities });
  const saveMutation = useMutation({ mutationFn: saveEnterpriseConnection, onSuccess: async (record) => { setSaveFailure(null); message.success(record.provider === 'FEISHU' ? '应用凭证已验证，请继续完成飞书后台配置' : '应用凭证已验证，请继续完成企业微信可信域名和可见范围配置'); setEditing({ provider: record.provider, record }); form.setFieldsValue({ provider: record.provider, displayName: record.displayName, bindingEnabled: record.bindingEnabled, notificationDirectEnabled: record.notificationDirectEnabled, notificationMentionEnabled: record.notificationMentionEnabled, loginEnabled: record.loginEnabled, expectedVersion: record.version, clientSecret: undefined, appId: undefined, corpId: undefined, agentId: undefined }); await queryClient.invalidateQueries({ queryKey: ['enterprise-connections'] }); }, onError: (error: unknown) => setSaveFailure(enterpriseConnectionSaveFeedback(error)) });
  const open = (provider: Provider, record?: EnterpriseConnectionRecord) => { setSaveFailure(null); setEditing({ provider, record }); form.setFieldsValue({ provider, displayName: record?.displayName || providerMeta[provider].name, bindingEnabled: record?.bindingEnabled ?? true, notificationDirectEnabled: record?.notificationDirectEnabled ?? true, notificationMentionEnabled: record?.notificationMentionEnabled ?? true, loginEnabled: record?.loginEnabled ?? false, expectedVersion: record?.version }); };
  const close = () => { setSaveFailure(null); setEditing(null); };
  const submit = async () => {
    setSaveFailure(null);
    const values = await form.validateFields();
    saveMutation.mutate(values);
  };
  const callbackUrl = connectionSetup?.providers.find((item) => item.provider === editing?.provider)?.callbackUrl;
  const callbackIsLoopback = Boolean(callbackUrl && /^http:\/\/(?:127\.0\.0\.1|localhost)(?::\d+)?(?:\/|$)/i.test(callbackUrl));

  useEffect(() => {
    if (!saveFailure) return;
    const frame = window.requestAnimationFrame(() => {
      saveFailureRef.current?.scrollIntoView({ behavior: 'auto', block: 'start' });
      saveFailureRef.current?.focus({ preventScroll: true });
    });
    return () => window.cancelAnimationFrame(frame);
  }, [saveFailure]);

  return <div className="p-6 lg:p-8 max-w-[1320px] mx-auto">
    <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between sm:gap-6 mb-7"><div><Title level={2} className="!mb-2">企业连接</Title><Paragraph type="secondary" className="!mb-0">配置组织的飞书和企业微信应用，成员随后可在账户设置中绑定自己的企业账号。</Paragraph></div></div>
    <Row gutter={[20, 20]}>{(['FEISHU','WECOM'] as Provider[]).map((provider) => { const record=data.find((item)=>item.provider===provider);const meta=providerMeta[provider];const ready=record?.status==='ACTIVE'&&record.lastProbeStatus==='PASSED';return <Col xs={24} xl={12} key={provider}><Card loading={isLoading} className="h-full rounded-xl" title={<Space><span className="h-9 w-9 rounded-lg flex items-center justify-center font-semibold" style={{color:meta.color,background:`${meta.color}12`}}>{meta.name.slice(0,2)}</span><span>{meta.name}</span></Space>} extra={<Tag color={ready?'success':record?'warning':'default'} icon={ready?<CheckCircleFilled />:record?<WarningFilled />:undefined}>{ready?'已连接':record?'需要处理':'未配置'}</Tag>}>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-6"><div><Text type="secondary">个人消息</Text><div className="mt-1 font-medium">{ready&&record?.notificationDirectEnabled?'可用':'未启用'}</div></div><div><Text type="secondary">群内提醒</Text><div className="mt-1 font-medium">{ready&&record?.notificationMentionEnabled?'可用':'未启用'}</div></div></div>
      <div className="flex items-center justify-between gap-4"><Text type="secondary">最近检查：{record?.lastProbeAt?new Date(record.lastProbeAt).toLocaleString():'尚未检查'}</Text><Button icon={<SettingOutlined />} onClick={()=>open(provider,record)}>管理连接</Button></div>
    </Card></Col>})}</Row>
    <Card className="mt-6 rounded-xl" title="已绑定成员">
      <Table<EnterpriseIdentityRecord> rowKey="id" loading={identitiesLoading} pagination={false} dataSource={identities} scroll={{x:760}} columns={[
        {title:'成员',key:'user',render:(_,item)=><div><div className="font-medium">{item.userDisplayName||'组织成员'}</div><Text type="secondary">{item.provider==='FEISHU'?'飞书':'企业微信'}</Text></div>},
        {title:'个人消息',dataIndex:'directEnabled',render:(value:boolean)=><Tag color={value?'success':'default'}>{value?'可用':'已关闭'}</Tag>},
        {title:'群内提醒',dataIndex:'mentionEnabled',render:(value:boolean)=><Tag color={value?'success':'default'}>{value?'可用':'已关闭'}</Tag>},
        {title:'最近验证',dataIndex:'verifiedAt',render:(value:string)=>value?new Date(value).toLocaleString():'—'},
      ]} />
    </Card>
    <Drawer title={`管理${editing?providerMeta[editing.provider].name:''}连接`} size="large" open={Boolean(editing)} onClose={close} extra={<Space><Button onClick={close}>取消</Button><Button type="primary" loading={saveMutation.isPending} onClick={submit}>保存并检查</Button></Space>}>
      {saveFailure ? <div ref={saveFailureRef} tabIndex={-1} className="outline-none"><Alert className="mb-6" showIcon type="error" title={saveFailure.title} description={saveFailure.description} action={saveFailure.action === 'OPEN_ACCOUNT_SECURITY' ? <Button href="/account/settings" target="_blank" rel="noreferrer" size="small">打开账号安全</Button> : undefined} /></div> : null}
      <Form form={form} layout="vertical" requiredMark="optional"><Form.Item name="provider" hidden><Input /></Form.Item><Form.Item label="连接名称" name="displayName" rules={[{required:true,message:'请输入连接名称'}]}><Input maxLength={128} /></Form.Item>
        <Form.Item label="OAuth 回调地址">
          {setupLoading ? <Text type="secondary">正在读取回调地址…</Text> : callbackUrl ? <Text copyable={{ text: callbackUrl }} className="break-all">{callbackUrl}</Text> : <Alert showIcon type="warning" title="平台尚未配置公开访问地址，暂时无法生成回调地址。" />}
        </Form.Item>
        {editing?.provider === 'FEISHU' && callbackUrl ? <>
          <Alert className={callbackIsLoopback ? 'mb-3' : 'mb-6'} showIcon type={editing.record?.lastProbeStatus === 'PASSED' ? 'success' : 'info'} title={editing.record?.lastProbeStatus === 'PASSED' ? '应用凭证已验证，还需完成飞书后台配置' : '保存前先准备飞书后台配置'} description={<div className="space-y-3"><ol className="list-decimal pl-5 space-y-1"><li>在飞书开发者后台打开当前应用，进入“安全设置”。</li><li>把上方 OAuth 回调地址完整添加为“重定向 URL”，不要填到事件回调或 Webhook。</li><li>保存后发布应用版本，并确认使用成员在应用可用范围内。</li></ol><Text type="secondary">地址必须完全一致，包括协议、主机、端口和路径。本地环境中 127.0.0.1 与 localhost 不是同一个地址。</Text><Space wrap><Button type="primary" href={FEISHU_DEVELOPER_CONSOLE_URL} target="_blank" rel="noreferrer" icon={<ExportOutlined />}>打开飞书开发者后台</Button><Button href={FEISHU_REDIRECT_GUIDE_URL} target="_blank" rel="noreferrer">查看配置说明</Button></Space></div>} />
          {callbackIsLoopback ? <Alert className="mb-6" showIcon type="warning" title="这是本地回调地址" description="只能在运行 Seven Agent Force 的这台电脑上完成绑定。部署到其他环境前，请配置稳定的 HTTPS 公开地址，并同步更新飞书重定向 URL。" /> : null}
        </> : null}
        {editing?.provider === 'WECOM' && callbackUrl ? <>
          <Alert className={callbackIsLoopback ? 'mb-3' : 'mb-6'} showIcon type={editing.record?.lastProbeStatus === 'PASSED' ? 'success' : 'info'} title={editing.record?.lastProbeStatus === 'PASSED' ? '应用凭证已验证，还需完成企业微信后台配置' : '保存前先准备企业微信自建应用'} description={<div className="space-y-3"><ol className="list-decimal pl-5 space-y-1"><li>在企业微信管理后台选择“应用管理”，打开用于通知的自建应用。</li><li>在“网页授权及 JS-SDK”中配置上方回调地址对应的可信域名；域名和端口必须完全一致。</li><li>把测试成员加入应用可见范围，并使用该应用自己的 Secret 和 AgentID。</li></ol><Text type="secondary">这里配置的是企业自建应用，不是群机器人 Webhook。个人消息接收人必须在应用可见范围内。</Text><Space wrap><Button type="primary" href={WECOM_ADMIN_APPLICATIONS_URL} target="_blank" rel="noreferrer" icon={<ExportOutlined />}>打开企业微信应用管理</Button><Button href={WECOM_OAUTH_GUIDE_URL} target="_blank" rel="noreferrer">查看网页授权说明</Button><Button href={WECOM_MESSAGE_GUIDE_URL} target="_blank" rel="noreferrer">查看应用消息说明</Button></Space></div>} />
          {callbackIsLoopback ? <Alert className="mb-6" showIcon type="warning" title="真实企业微信绑定需要可访问的可信域名" description="当前回调使用本机地址，无法作为其他设备或企业微信客户端可访问的正式回调。请先配置稳定的 HTTPS 公开地址，再将对应域名和端口加入应用可信域名。" /> : null}
        </> : null}
        {editing?.provider==='FEISHU'?<Form.Item label="App ID" name="appId" rules={editing.record?[{}]:[{required:true,message:'请输入 App ID'}]}><Input placeholder={editing.record?.appIdHint||'cli_xxx'} /></Form.Item>:<><Form.Item label="企业 ID" name="corpId" rules={editing?.record?[{}]:[{required:true,message:'请输入企业 ID'}]}><Input placeholder={editing?.record?.corpIdHint||'wwxxxx'} /></Form.Item><Form.Item label="应用 Agent ID" name="agentId" rules={editing?.record?[{}]:[{required:true,message:'请输入该自建应用的 Agent ID'}]}><Input placeholder={editing?.record?.agentIdHint||'1000002'} /></Form.Item></>}
        <Form.Item label="应用密钥" name="clientSecret" rules={editing?.record?[{}]:[{required:true,message:'请输入应用密钥'}]} extra={editing?.record?'留空表示保持现有密钥。密钥不会回显。':'保存后密钥不会再次显示。'}><Input.Password autoComplete="new-password" /></Form.Item>
        <Card size="small" title="可用能力"><Form.Item name="bindingEnabled" valuePropName="checked" label="允许成员绑定"><Switch /></Form.Item><Form.Item name="notificationDirectEnabled" valuePropName="checked" label="个人消息"><Switch /></Form.Item><Form.Item name="notificationMentionEnabled" valuePropName="checked" label="群内提醒"><Switch /></Form.Item><Form.Item name="loginEnabled" valuePropName="checked" hidden><Switch /></Form.Item></Card><Form.Item name="expectedVersion" hidden><Input /></Form.Item>
      </Form>
    </Drawer>
  </div>;
}
