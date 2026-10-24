'use client';

import React, { useEffect, useRef, useState } from 'react';
import { Button, Result, Space, Spin, Typography } from 'antd';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { completeEnterpriseBinding } from '@/api/externalLoginController';
import { enterpriseBindingFeedback, type EnterpriseFailureFeedback } from '@/lib/enterprise-identity-feedback';

type EnterpriseCallbackResult = { outcome: string; purpose?: string; authenticated?: boolean; redirectAfter?: string };

const callbackRequests = new Map<string, Promise<EnterpriseCallbackResult>>();

async function completeEnterpriseBindingOnce(provider: string, code: string, state: string, error: string) {
  const digest = await window.crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${provider}:${state}`));
  const key = Array.from(new Uint8Array(digest), (value) => value.toString(16).padStart(2, '0')).join('');
  const existing = callbackRequests.get(key);
  if (existing) return existing;
  const request = completeEnterpriseBinding(provider, { code, state, error });
  callbackRequests.set(key, request);
  window.setTimeout(() => callbackRequests.delete(key), 30_000);
  return request;
}

export default function EnterpriseIdentityCallbackPage() {
  const params = useParams<{ provider: string }>();
  const [query] = useSearchParams();
  const navigate = useNavigate();
  const consumed = useRef(false);
  const [state, setState] = useState<'loading'|'success'|'error'>('loading');
  const [purpose, setPurpose] = useState<'bind'|'login'>('bind');
  const [failure, setFailure] = useState<EnterpriseFailureFeedback>({ title: '授权没有完成', description: '请返回账户设置重新绑定。', action: 'RETURN_TO_SETTINGS' });

  useEffect(() => {
    if (consumed.current) return;
    consumed.current = true;
    const code=query.get('code')||'';const oauthState=query.get('state')||'';const oauthError=query.get('error')||'';
    window.history.replaceState({}, '', window.location.pathname);
    if ((!code && !oauthError) || !oauthState) { setFailure({ title: '授权信息不完整', description: '请返回账户设置，重新发起一次绑定。', action: 'RETURN_TO_SETTINGS' }); setState('error'); return; }
    completeEnterpriseBindingOnce(params.provider || '', code, oauthState, oauthError).then((result) => {
		if (result.outcome !== 'BOUND' && result.outcome !== 'AUTHENTICATED') throw new Error('authorization incomplete');
		setPurpose(result.authenticated ? 'login' : 'bind');
		setState('success'); window.setTimeout(()=>navigate(result.redirectAfter||(result.authenticated?'/':'/account/settings'),{replace:true}),300);
	}).catch((error: unknown) => {
		setFailure(enterpriseBindingFeedback(error, params.provider));
		setState('error');
	});
  }, [params.provider, query, navigate]);

  if (state==='loading') return <main className="min-h-screen flex flex-col items-center justify-center gap-4 bg-slate-50"><Spin size="large" /><div className="text-slate-700">正在验证企业账号…</div></main>;
  if (state==='success') return <main className="min-h-screen flex items-center justify-center bg-slate-50"><Result status="success" title={purpose==='login'?'登录成功':'绑定完成'} subTitle={purpose==='login'?'正在进入工作台。':'现在可以接收个人任务通知。'} extra={<Button type="primary" onClick={()=>navigate(purpose==='login'?'/':'/account/settings',{replace:true})}>{purpose==='login'?'进入工作台':'返回账户设置'}</Button>} /></main>;
  return <main className="min-h-screen flex items-center justify-center bg-slate-50 px-4"><Result status="error" title={failure.title} subTitle={failure.description} extra={<Space direction="vertical" size={12}><Button type="primary" onClick={()=>navigate('/account/settings',{replace:true})}>返回账户设置</Button>{failure.traceId ? <details className="text-left"><summary className="cursor-pointer text-sm text-slate-500">需要帮助？</summary><Typography.Text type="secondary" copyable={{ text: failure.traceId }}>参考编号：{failure.traceId}</Typography.Text></details> : null}</Space>} /></main>;
}
