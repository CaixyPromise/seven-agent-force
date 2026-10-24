"use client";

import {
  Alert,
  Button,
  ConfigProvider,
  Spin,
  Typography,
} from 'antd';
import { css } from '@emotion/css';
import { AnimatePresence, motion } from 'framer-motion';
import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { SafetyCertificateOutlined } from '@ant-design/icons';

import PasswordPanel from './components/PasswordPanel';
import RegisterPanel from './components/RegisterPanel';
import PasskeyPanel from './components/PasskeyPanel';
import TotpPanel from './components/TotpPanel';
import LockedPanel from './components/LockedPanel';
import { useLoginPageState } from './hooks/useLoginPageState';
import { getPlatformLoginOptions } from '@/api/platformController';
import { getLoginRegisterState, sendLoginRegisterEmailCode, submitLoginRegister } from '@/api/loginController';
import { authorizeSsoLogin } from '@/api/ssoController';
import { getSetupStatusApi } from '@/api/setupController';
import {
  buildAuthorizationParams,
  createPkceSession,
  persistPkceSession,
  resolveLoginRedirectTarget,
  resolveOidcCallbackRedirectUri,
} from '@/lib/auth/oidc';
import { buildSetupRedirectUrl } from '@/lib/auth/navigation';
import { getSsoRuntimeConfig } from '@/lib/auth/sso-runtime';
import { syncDeviceId } from '@/lib/auth/device';
import { useConfigValue } from '@/hooks/config';
import { configAssetStablePathOrEmpty } from '@/lib/configAssets';
import type {
  LoginCaptcha,
  PlatformLoginOptions,
} from '@/lib/http/types';
import { refreshLoginSession as resolveRefreshedLoginSession } from './lib/login-session';

const { Paragraph, Text, Title } = Typography;

const EMPTY_LOGIN_OPTIONS: PlatformLoginOptions = {
  loginContextId: '',
  platformName: 'Seven',
  brand: {
    title: 'Seven',
    subtitle: '统一身份认证系统',
  },
  methods: [],
};

type BootstrapResult =
  | {
      clientId: string;
      loginTransactionId: string;
    }
  | {
      clientId: string;
      redirectUrl: string;
    };

let activeBootstrapPromise: Promise<BootstrapResult> | null = null;
let activeBootstrapKey: string | null = null;

function parseAuthorizeParamsFromContinueUrl(continueUrl: string) {
  const parsedUrl = new URL(continueUrl);
  const searchParams = parsedUrl.searchParams;
  const clientId = searchParams.get('client_id')?.trim() || '';
  const redirectUri = searchParams.get('redirect_uri')?.trim() || '';
  const scope = searchParams.get('scope')?.trim() || '';
  const state = searchParams.get('state')?.trim() || '';
  const nonce = searchParams.get('nonce')?.trim() || '';
  const codeChallenge = searchParams.get('code_challenge')?.trim() || '';
  const codeChallengeMethod = searchParams.get('code_challenge_method')?.trim() || 'S256';
  const prompt = searchParams.get('prompt')?.trim() || '';
  if (!clientId || !redirectUri || !scope || !state || !codeChallenge) {
    throw new Error('授权参数不完整，无法恢复登录事务');
  }
  return {
    clientId,
    redirectUri,
    scope,
    state,
    nonce,
    codeChallenge,
    codeChallengeMethod,
    prompt,
  };
}

function runLoginBootstrap(): Promise<BootstrapResult> {
  const bootstrapKey = `${window.location.pathname}?${window.location.search}`;
  if (activeBootstrapPromise && activeBootstrapKey === bootstrapKey) {
    return activeBootstrapPromise;
  }
  activeBootstrapKey = bootstrapKey;
  activeBootstrapPromise = (async () => {
    const runtimeConfig = await getSsoRuntimeConfig();
    if (!runtimeConfig.enabled || !runtimeConfig.frontendPrimaryEnabled) {
      throw new Error('登录服务尚未启用');
    }
    const clientId = runtimeConfig.defaultFirstPartyClientId || 'authorization-console';
    const pkceSession = await createPkceSession(resolveOidcCallbackRedirectUri());
    pkceSession.postLoginRedirect = resolveLoginRedirectTarget();
    persistPkceSession(clientId, pkceSession);
    const authorizeResponse = await authorizeSsoLogin(
      buildAuthorizationParams(clientId, pkceSession, 'openid profile email offline_access'),
    );
    const authorizeResult = authorizeResponse.data;
    if (!authorizeResult) {
      throw new Error('登录服务未返回有效的登录事务');
    }
    if (authorizeResult.redirectUrl) {
      return {
        clientId,
        redirectUrl: authorizeResult.redirectUrl,
      };
    }
    if (authorizeResult.loginTransactionId) {
      return {
        clientId,
        loginTransactionId: authorizeResult.loginTransactionId,
      };
    }
    throw new Error('登录服务未返回有效的登录事务');
  })().finally(() => {
    activeBootstrapPromise = null;
    activeBootstrapKey = null;
  });
  return activeBootstrapPromise;
}

function normalizeTimestampMs(ms?: number | string | null) {
  if (ms === null || ms === undefined || ms === '') {
    return null;
  }
  const parsed = typeof ms === 'number' ? ms : Number(ms);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

function formatDateTime(ms?: number | string | null) {
  const timestamp = normalizeTimestampMs(ms);
  if (!timestamp) {
    return '';
  }
  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  const pad = (value: number) => value.toString().padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function formatLockCountdown(now: number, ms?: number | string | null) {
  const timestamp = normalizeTimestampMs(ms);
  if (!timestamp || timestamp <= now) {
    return '';
  }
  const remainingSeconds = Math.floor((timestamp - now) / 1000);
  const hours = Math.floor(remainingSeconds / 3600);
  const minutes = Math.floor((remainingSeconds % 3600) / 60);
  const seconds = remainingSeconds % 60;
  return `预计 ${formatDateTime(timestamp)} 自动解锁，剩余 ${hours} 小时 ${minutes} 分 ${seconds} 秒`;
}

function resolveExternalLoginUrl(loginUrl?: string | null) {
  if (!loginUrl?.trim()) {
    throw new Error('外部登录地址未配置');
  }
  const target = new URL(loginUrl, window.location.origin);
  if (target.origin !== window.location.origin) {
    throw new Error('外部登录地址不合法');
  }
  return `${target.pathname}${target.search}${target.hash}`;
}

function isSameOriginExternalLoginUrl(loginUrl?: string | null) {
  if (typeof window === 'undefined') {
    return false;
  }
  try {
    resolveExternalLoginUrl(loginUrl);
    return true;
  } catch {
    return false;
  }
}

type ApiErrorPayload = {
  code?: number;
  message?: string;
};

function readApiErrorPayload(error: unknown): ApiErrorPayload {
  const payload = (error as { payload?: ApiErrorPayload })?.payload;
  if (payload) {
    return payload;
  }
  const responsePayload = (error as { response?: { data?: ApiErrorPayload } })?.response?.data;
  if (responsePayload && typeof responsePayload === 'object') {
    return responsePayload;
  }
  return {};
}

function isLoginContextExpiredError(error: unknown) {
  const payload = readApiErrorPayload(error);
  const message = payload.message || (error as { message?: string })?.message || '';
  return payload.code === 40900
    || (payload.code === 40300 && message.includes('登录事务已失效'))
    || message.includes('登录上下文已失效')
    || message.includes('登录事务已失效')
    || message.includes('登录事务已完成或已失效');
}

export default function LoginPage() {
  const navigate = useNavigate();
  const [loginTransactionId, setLoginTransactionId] = useState('');
  const [loginClientId, setLoginClientId] = useState('');
  const [loginOptionsRedirectUrl, setLoginOptionsRedirectUrl] = useState('');
  const [bootstrapping, setBootstrapping] = useState(true);
  const [bootstrapError, setBootstrapError] = useState('');
  const [now, setNow] = useState(() => Date.now());
  const [passkeyAutoStarted, setPasskeyAutoStarted] = useState(false);
  const [loginOptions, setLoginOptions] = useState<PlatformLoginOptions>(EMPTY_LOGIN_OPTIONS);
  const [loadingLoginOptions, setLoadingLoginOptions] = useState(false);
  const [loginOptionsError, setLoginOptionsError] = useState('');
  const [loginOptionsReloadKey, setLoginOptionsReloadKey] = useState(0);
  const registerLoginTransactionIdRef = useRef('');
  const registerLoginContextIdRef = useRef('');
  const [authPanel, setAuthPanel] = useState<'password' | 'register'>('password');
  const [registerInitialAccount, setRegisterInitialAccount] = useState('');
  const [registerCaptcha, setRegisterCaptcha] = useState<LoginCaptcha | null>(null);
  const [registerError, setRegisterError] = useState('');
  const [registerInfo, setRegisterInfo] = useState('');
  const [registerSuccessMessage, setRegisterSuccessMessage] = useState('');
  const [registerSessionResetKey, setRegisterSessionResetKey] = useState(0);
  const loginLogo = useConfigValue<string>('SEVEN_FRONTEND_METADATA.loginLogo');
  const loginLogoPath = configAssetStablePathOrEmpty(loginLogo?.value);
  const [loadingRegisterState, setLoadingRegisterState] = useState(false);
  const [submittingRegister, setSubmittingRegister] = useState(false);
  const hostedLoginTransactionId = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return new URLSearchParams(window.location.search).get('loginTransactionId')?.trim() || '';
  }, []);
  const hostedContinueUrl = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return new URLSearchParams(window.location.search).get('continue')?.trim() || '';
  }, []);
  const hostedRedirectUrl = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return new URLSearchParams(window.location.search).get('redirect')?.trim() || '';
  }, []);
  const hostedClientId = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    const searchParams = new URLSearchParams(window.location.search);
    return searchParams.get('clientId')?.trim() || searchParams.get('client_id')?.trim() || '';
  }, []);
  const hostedUserAccount = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return new URLSearchParams(window.location.search).get('userAccount')?.trim() || '';
  }, []);
  const hostedDeviceId = useMemo(() => {
    if (typeof window === 'undefined') {
      return '';
    }
    return new URLSearchParams(window.location.search).get('deviceId')?.trim() || '';
  }, []);
  const hostedPasskeyAuto = useMemo(() => {
    if (typeof window === 'undefined') {
      return false;
    }
    const rawValue = new URLSearchParams(window.location.search).get('passkeyAuto')?.trim().toLowerCase();
    return rawValue === '1' || rawValue === 'true';
  }, []);

  const replaceHostedLoginTransactionInUrl = useCallback((nextLoginTransactionId: string) => {
    if (!nextLoginTransactionId || typeof window === 'undefined') {
      return;
    }
    const currentUrl = new URL(window.location.href);
    currentUrl.searchParams.set('loginTransactionId', nextLoginTransactionId);
    window.history.replaceState(window.history.state, '', currentUrl.toString());
  }, []);

  const followBrowserRedirect = useCallback((redirectUrl?: string | null) => {
    if (!redirectUrl) {
      const target = resolveLoginRedirectTarget();
      if (target.startsWith('http://') || target.startsWith('https://')) {
        window.location.assign(target);
        return;
      }
      navigate(target, { replace: true });
      return;
    }
    window.location.assign(redirectUrl);
  }, [navigate]);

  const refreshLoginTransaction = useCallback(async () => {
    if (hostedContinueUrl) {
      const authorizeParams = parseAuthorizeParamsFromContinueUrl(hostedContinueUrl);
      const response = await authorizeSsoLogin({
        clientId: authorizeParams.clientId,
        redirectUri: authorizeParams.redirectUri,
        scope: authorizeParams.scope,
        state: authorizeParams.state,
        nonce: authorizeParams.nonce,
        codeChallenge: authorizeParams.codeChallenge,
        codeChallengeMethod: authorizeParams.codeChallengeMethod,
        prompt: authorizeParams.prompt,
      });
      const authorizeResult = response.data;
      if (!authorizeResult) {
        throw new Error('登录服务未返回有效的登录事务');
      }
      if (authorizeResult.redirectUrl) {
        followBrowserRedirect(authorizeResult.redirectUrl);
        return '';
      }
      if (!authorizeResult.loginTransactionId) {
        throw new Error('登录服务未返回有效的登录事务');
      }
      setLoginClientId(authorizeParams.clientId);
      setLoginTransactionId(authorizeResult.loginTransactionId);
      setLoginOptionsRedirectUrl(authorizeParams.redirectUri);
      replaceHostedLoginTransactionInUrl(authorizeResult.loginTransactionId);
      return authorizeResult.loginTransactionId;
    }
    const bootstrapResult = await runLoginBootstrap();
    if ('redirectUrl' in bootstrapResult) {
      followBrowserRedirect(bootstrapResult.redirectUrl);
      return '';
    }
    setLoginClientId(bootstrapResult.clientId);
    setLoginTransactionId(bootstrapResult.loginTransactionId);
    setLoginOptionsRedirectUrl(resolveOidcCallbackRedirectUri());
    return bootstrapResult.loginTransactionId;
  }, [followBrowserRedirect, hostedContinueUrl, replaceHostedLoginTransactionInUrl]);

  const refreshLoginSession = useCallback(async () => resolveRefreshedLoginSession({
    refreshTransaction: refreshLoginTransaction,
    resolveLoginOptions: async (refreshedLoginTransactionId) => {
      const refreshedLoginOptions = await getPlatformLoginOptions({
        redirect: loginOptionsRedirectUrl || resolveOidcCallbackRedirectUri(),
        clientId: loginClientId || hostedClientId,
        loginTransactionId: refreshedLoginTransactionId,
      });
      setLoginOptions(refreshedLoginOptions);
      setLoginOptionsError('');
      return refreshedLoginOptions;
    },
  }), [
    hostedClientId,
    loginClientId,
    loginOptionsRedirectUrl,
    refreshLoginTransaction,
  ]);

  const bootstrapLogin = useCallback(async () => {
    setBootstrapping(true);
    setBootstrapError('');
    if (hostedLoginTransactionId) {
      setLoginClientId(hostedClientId);
      setLoginTransactionId(hostedLoginTransactionId);
      setLoginOptionsRedirectUrl(
        hostedRedirectUrl.includes('/oidc/callback/')
          ? hostedRedirectUrl
          : resolveOidcCallbackRedirectUri(),
      );
      setBootstrapping(false);
      return;
    }
    const setupStatusResponse = await getSetupStatusApi().catch(() => null);
    if (setupStatusResponse && !setupStatusResponse.data?.initialized) {
      navigate(buildSetupRedirectUrl(window.location.href), { replace: true });
      return;
    }
    try {
      const bootstrapResult = await runLoginBootstrap();
      if ('redirectUrl' in bootstrapResult) {
        followBrowserRedirect(bootstrapResult.redirectUrl);
        return;
      }
      setLoginClientId(bootstrapResult.clientId);
      setLoginTransactionId(bootstrapResult.loginTransactionId);
      setLoginOptionsRedirectUrl(resolveOidcCallbackRedirectUri());
    } catch (error) {
      const nextMessage = (error as { message?: string })?.message || '登录初始化失败';
      setBootstrapError(nextMessage);
    } finally {
      setBootstrapping(false);
    }
  }, [followBrowserRedirect, hostedClientId, hostedLoginTransactionId, hostedRedirectUrl, navigate]);

  const reloadLoginOptions = useCallback(() => {
    setLoginOptionsError('');
    setLoginOptionsReloadKey((current) => current + 1);
  }, []);

  useEffect(() => {
    document.title = '登录 - Seven Agent Force';
    let cancelled = false;
    void Promise.resolve().then(() => {
      if (!cancelled) {
        void bootstrapLogin();
      }
    });
    return () => {
      cancelled = true;
    };
  }, [bootstrapLogin]);

  useEffect(() => {
    let cancelled = false;
    void Promise.resolve().then(async () => {
      if (!loginTransactionId) {
        if (cancelled) {
          return;
        }
        setLoginOptions(EMPTY_LOGIN_OPTIONS);
        setLoadingLoginOptions(false);
        setLoginOptionsError('');
        return;
      }
      setLoginOptions(EMPTY_LOGIN_OPTIONS);
      setLoadingLoginOptions(true);
      try {
        const options = await getPlatformLoginOptions({
          redirect: loginOptionsRedirectUrl || resolveOidcCallbackRedirectUri(),
          clientId: loginClientId || hostedClientId,
          loginTransactionId,
        });
        if (!cancelled) {
          setLoginOptions(options);
          setLoginOptionsError('');
        }
      } catch (error) {
        if (!cancelled) {
          setLoginOptions(EMPTY_LOGIN_OPTIONS);
          setLoginOptionsError(
            (error as { message?: string })?.message || '平台登录配置不可用，请重试',
          );
        }
      } finally {
        if (!cancelled) {
          setLoadingLoginOptions(false);
        }
      }
    });
    return () => {
      cancelled = true;
    };
  }, [
    hostedClientId,
    loginClientId,
    loginOptionsRedirectUrl,
    loginOptionsReloadKey,
    loginTransactionId,
  ]);

  useEffect(() => {
    if (!hostedDeviceId) {
      return;
    }
    syncDeviceId(hostedDeviceId);
  }, [hostedDeviceId]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);
    return () => {
      window.clearInterval(timer);
    };
  }, []);

  const loginState = useLoginPageState({
    loginTransactionId,
    loginContextId: loginOptions.loginContextId,
    initialUserAccount: hostedUserAccount,
    onAuthenticated: async (redirectUrl) => {
      followBrowserRedirect(redirectUrl);
    },
    onRefreshLoginSession: refreshLoginSession,
  });
  const {
    checkPasswordState,
    currentAccount: loginCurrentAccount,
    stage: loginStage,
    startingPasskey,
    startPasskey,
  } = loginState;

  const lockDescription = useMemo(
    () => formatLockCountdown(now, loginState.lockExpiresAt),
    [loginState.lockExpiresAt, now],
  );
  const passwordLoginEnabled = useMemo(
    () => loginOptions.methods.some((method) => method.methodType === 'PASSWORD'),
    [loginOptions.methods],
  );
  const passkeyLoginEnabled = useMemo(
    () => loginOptions.methods.some((method) => method.methodType === 'PASSKEY'),
    [loginOptions.methods],
  );
  const externalLoginMethods = useMemo(
    () => loginOptions.methods.filter(
      (method) => method.methodType === 'EXTERNAL_OAUTH'
        && isSameOriginExternalLoginUrl(method.loginUrl),
    ),
    [loginOptions.methods],
  );
  const formRegisterEnabled = loginOptions.registration?.formRegisterEnabled === true;
  const loginOptionsUnavailable = Boolean(loginOptionsError);
  const loginAlertMessage = (authPanel === 'register' ? registerError : '')
    || loginState.errorMessage
    || loginOptionsError;
  const registerInfoMessage = authPanel === 'register' && !loginAlertMessage ? registerInfo : '';
  const brandTitle = loginOptions.brand.title || loginOptions.platformName || 'Seven';
  const brandSubtitle = loginOptions.brand.subtitle || '统一身份认证系统';

  useEffect(() => {
    registerLoginTransactionIdRef.current = loginTransactionId;
    registerLoginContextIdRef.current = loginOptions.loginContextId || '';
  }, [loginOptions.loginContextId, loginTransactionId]);

  const refreshRegisterContext = useCallback(async (userAccount: string) => {
    const normalizedAccount = userAccount.trim();
    if (!normalizedAccount) {
      setRegisterCaptcha(null);
      return null;
    }
    setRegisterError('');
    setRegisterInfo('验证信息已过期，正在自动刷新...');
    const refreshedLoginTransactionId = await refreshLoginTransaction();
    if (!refreshedLoginTransactionId) {
      throw new Error('登录事务刷新失败，请重新打开登录页');
    }
    const refreshedLoginOptions = await getPlatformLoginOptions({
      redirect: loginOptionsRedirectUrl || resolveOidcCallbackRedirectUri(),
      clientId: loginClientId || hostedClientId,
      loginTransactionId: refreshedLoginTransactionId,
    });
    setLoginOptions(refreshedLoginOptions);
    registerLoginTransactionIdRef.current = refreshedLoginTransactionId;
    registerLoginContextIdRef.current = refreshedLoginOptions.loginContextId || '';
    setLoginOptionsError('');
    if (refreshedLoginOptions.registration?.formRegisterEnabled !== true) {
      throw new Error('当前平台未开放注册');
    }
    if (!refreshedLoginOptions.loginContextId) {
      throw new Error('平台登录上下文刷新失败，请重试');
    }
    const stateResponse = await getLoginRegisterState({
      loginTransactionId: refreshedLoginTransactionId,
      loginContextId: refreshedLoginOptions.loginContextId,
      userAccount: normalizedAccount,
    });
    setRegisterCaptcha(stateResponse.data?.captcha ?? null);
    setRegisterSessionResetKey((current) => current + 1);
    setRegisterInfo('已刷新验证信息，请输入新的图形验证码');
    return {
      loginTransactionId: refreshedLoginTransactionId,
      loginContextId: refreshedLoginOptions.loginContextId,
    };
  }, [
    hostedClientId,
    loginClientId,
    loginOptionsRedirectUrl,
    refreshLoginTransaction,
  ]);

  const loadRegisterState = useCallback(async (userAccount: string) => {
    const normalizedAccount = userAccount.trim();
    const activeLoginTransactionId = registerLoginTransactionIdRef.current || loginTransactionId;
    const activeLoginContextId = registerLoginContextIdRef.current || loginOptions.loginContextId;
    if (!normalizedAccount || !activeLoginTransactionId || !activeLoginContextId) {
      setRegisterCaptcha(null);
      return null;
    }
    setLoadingRegisterState(true);
    try {
      const response = await getLoginRegisterState({
        loginTransactionId: activeLoginTransactionId,
        loginContextId: activeLoginContextId,
        userAccount: normalizedAccount,
      });
      setRegisterCaptcha(response.data?.captcha ?? null);
      setRegisterError('');
      setRegisterInfo('');
      return response.data ?? null;
    } catch (error) {
      if (isLoginContextExpiredError(error)) {
        try {
          await refreshRegisterContext(normalizedAccount);
        } catch (refreshError) {
          setRegisterCaptcha(null);
          setRegisterError(
            (refreshError as { message?: string })?.message || '注册会话刷新失败，请重新打开登录页',
          );
        }
        return null;
      }
      setRegisterCaptcha(null);
      setRegisterError((error as { message?: string })?.message || '注册配置不可用，请重新登录');
      return null;
    } finally {
      setLoadingRegisterState(false);
    }
  }, [loginOptions.loginContextId, loginTransactionId, refreshRegisterContext]);

  const openRegisterPanel = useCallback((userAccount?: string) => {
    const normalizedAccount = userAccount?.trim() || loginCurrentAccount || '';
    setRegisterInitialAccount(normalizedAccount);
    setRegisterCaptcha(null);
    setRegisterError('');
    setRegisterInfo('');
    setRegisterSuccessMessage('');
    setAuthPanel('register');
    if (normalizedAccount) {
      void loadRegisterState(normalizedAccount);
    }
  }, [loadRegisterState, loginCurrentAccount]);

  const closeRegisterPanel = useCallback(() => {
    setAuthPanel('password');
    setRegisterCaptcha(null);
    setRegisterError('');
    setRegisterInfo('');
  }, []);

  const submitRegister = useCallback(async (values: {
    userAccount: string;
    userName: string;
    userEmail: string;
    password: string;
    confirmPassword: string;
    emailCode: string;
  }) => {
    const activeLoginTransactionId = registerLoginTransactionIdRef.current || loginTransactionId;
    const activeLoginContextId = registerLoginContextIdRef.current || loginOptions.loginContextId;
    if (!activeLoginTransactionId || !activeLoginContextId) {
      setRegisterError('登录事务尚未就绪，请稍后重试');
      return;
    }
    setSubmittingRegister(true);
    try {
      setRegisterError('');
      setRegisterInfo('');
      const response = await submitLoginRegister({
        loginTransactionId: activeLoginTransactionId,
        loginContextId: activeLoginContextId,
        ...values,
      });
      const result = response.data;
      if (result?.registered) {
        setRegisterInitialAccount(values.userAccount);
        setRegisterSuccessMessage('注册成功，请使用新账号登录');
        setAuthPanel('password');
        setRegisterCaptcha(null);
        void checkPasswordState(values.userAccount);
        return;
      }
      setRegisterCaptcha(result?.captcha ?? null);
      setRegisterError(result?.message || '注册失败，请检查后重试');
    } catch (error) {
      if (isLoginContextExpiredError(error)) {
        try {
          await refreshRegisterContext(values.userAccount);
        } catch (refreshError) {
          setRegisterCaptcha(null);
          setRegisterError(
            (refreshError as { message?: string })?.message || '注册会话刷新失败，请重新打开登录页',
          );
        }
        return;
      }
      setRegisterError((error as { message?: string })?.message || '注册失败，请稍后重试');
      void loadRegisterState(values.userAccount);
    } finally {
      setSubmittingRegister(false);
    }
  }, [
    checkPasswordState,
    loadRegisterState,
    loginOptions.loginContextId,
    loginTransactionId,
    refreshRegisterContext,
  ]);

  const sendRegisterEmailCode = useCallback(async (values: {
    userAccount: string;
    userEmail: string;
    captchaCode: string;
  }) => {
    const activeLoginTransactionId = registerLoginTransactionIdRef.current || loginTransactionId;
    const activeLoginContextId = registerLoginContextIdRef.current || loginOptions.loginContextId;
    if (!activeLoginTransactionId || !activeLoginContextId) {
      setRegisterError('登录事务尚未就绪，请稍后重试');
      return null;
    }
    try {
      setRegisterError('');
      setRegisterInfo('');
      const response = await sendLoginRegisterEmailCode({
        loginTransactionId: activeLoginTransactionId,
        loginContextId: activeLoginContextId,
        ...values,
      });
      const result = response.data ?? null;
      setRegisterCaptcha(result?.captcha ?? null);
      if (!result?.sent) {
        setRegisterError(result?.message || '验证码发送失败，请检查后重试');
      }
      return result;
    } catch (error) {
      if (isLoginContextExpiredError(error)) {
        try {
          await refreshRegisterContext(values.userAccount);
        } catch (refreshError) {
          setRegisterCaptcha(null);
          setRegisterError(
            (refreshError as { message?: string })?.message || '注册会话刷新失败，请重新打开登录页',
          );
        }
        return null;
      }
      setRegisterError((error as { message?: string })?.message || '验证码发送失败，请稍后重试');
      void loadRegisterState(values.userAccount);
      return null;
    }
  }, [
    loadRegisterState,
    loginOptions.loginContextId,
    loginTransactionId,
    refreshRegisterContext,
  ]);

  useEffect(() => {
    if (!loadingLoginOptions && !formRegisterEnabled) {
      let cancelled = false;
      void Promise.resolve().then(() => {
        if (cancelled) {
          return;
        }
        setAuthPanel('password');
        setRegisterCaptcha(null);
        setRegisterError('');
      });
      return () => {
        cancelled = true;
      };
    }
    return undefined;
  }, [formRegisterEnabled, loadingLoginOptions]);

  useEffect(() => {
    let cancelled = false;
    void Promise.resolve().then(() => {
      if (cancelled || !hostedPasskeyAuto || passkeyAutoStarted) {
        return;
      }
      if (
        !loginTransactionId
        || !hostedUserAccount
        || bootstrapping
        || bootstrapError
        || loadingLoginOptions
        || loginOptionsUnavailable
        || !passkeyLoginEnabled
      ) {
        return;
      }
      if (loginStage !== 'password' || startingPasskey) {
        return;
      }
      setPasskeyAutoStarted(true);
      const currentUrl = new URL(window.location.href);
      currentUrl.searchParams.delete('passkeyAuto');
      window.history.replaceState(window.history.state, '', currentUrl.toString());
      void startPasskey(hostedUserAccount);
    });
    return () => {
      cancelled = true;
    };
  }, [
    bootstrapError,
    bootstrapping,
    hostedPasskeyAuto,
    hostedUserAccount,
    loadingLoginOptions,
    loginOptionsUnavailable,
    loginStage,
    loginTransactionId,
    passkeyAutoStarted,
    passkeyLoginEnabled,
    startPasskey,
    startingPasskey,
  ]);
  const pageClassName = css({
    position: 'relative',
    minHeight: '100vh',
    overflow: 'hidden',
    background: '#f8fafc',
    padding: '16px',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    color: '#0f172a',
    '.login-grid': {
      width: '100%',
      maxWidth: '1280px',
      margin: '0 auto',
      display: 'flex',
      justifyContent: 'center',
      position: 'relative',
      zIndex: 1,
    },
    '.login-card': {
      width: '100%',
      maxWidth: '440px',
      background: 'rgba(255,255,255,0.6)',
      backdropFilter: 'blur(24px)',
      WebkitBackdropFilter: 'blur(24px)',
      border: '1px solid rgba(255,255,255,0.45)',
      boxShadow: '0 8px 32px rgba(31, 38, 135, 0.07)',
      borderRadius: '28px',
      padding: '32px 32px 28px',
      position: 'relative',
      '@media (min-width: 1024px)': {
        marginLeft: 'auto',
        marginRight: '96px',
      },
      '@media (min-width: 1280px)': {
        marginRight: '128px',
      },
      '@media (max-width: 576px)': {
        padding: '28px 20px 24px',
        borderRadius: '24px',
      },
    },
    '.login-card-register': {
      maxWidth: '640px',
      padding: '28px 30px 26px',
      '@media (min-width: 1024px)': {
        marginRight: '80px',
      },
      '@media (max-width: 760px)': {
        maxWidth: '440px',
      },
    },
    '.login-card-register .login-header': {
      marginBottom: '22px',
    },
    '.login-card-register .login-logo': {
      width: '44px',
      height: '44px',
      borderRadius: '13px',
      fontSize: '22px',
      marginBottom: '12px',
      boxShadow: '0 14px 30px rgba(14, 165, 233, 0.22)',
    },
    '.login-card-register .login-title.ant-typography': {
      fontSize: '30px',
      marginBottom: '4px',
    },
    '.login-logo': {
      width: '48px',
      height: '48px',
      borderRadius: '14px',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: 'linear-gradient(135deg, #3b82f6 0%, #22d3ee 100%)',
      color: '#fff',
      fontSize: '24px',
      boxShadow: '0 16px 36px rgba(14, 165, 233, 0.26)',
      marginBottom: '16px',
    },
    '.login-logo img': {
      width: '100%',
      height: '100%',
      borderRadius: 'inherit',
      objectFit: 'cover',
    },
    '.login-header': {
      marginBottom: '28px',
    },
    '.login-title.ant-typography': {
      marginBottom: '6px',
      color: '#1e293b',
      fontSize: '32px',
      fontWeight: 700,
      lineHeight: 1.15,
    },
    '.login-subtitle.ant-typography': {
      color: '#64748b',
      fontSize: '14px',
      marginBottom: 0,
    },
    '.login-alert.ant-alert': {
      borderRadius: '14px',
      border: '1px solid rgba(254, 226, 226, 0.95)',
      background: 'rgba(254, 242, 242, 0.88)',
      marginBottom: '24px',
    },
    '.login-alert .ant-alert-message': {
      fontWeight: 500,
    },
    '.login-info-alert.ant-alert': {
      borderRadius: '14px',
      border: '1px solid rgba(125, 211, 252, 0.9)',
      background: 'rgba(239, 246, 255, 0.9)',
      marginBottom: '24px',
    },
    '.login-info-alert .ant-alert-message': {
      fontWeight: 500,
      color: '#075985',
    },
    '.login-success-alert.ant-alert': {
      borderRadius: '14px',
      border: '1px solid rgba(187, 247, 208, 0.95)',
      background: 'rgba(240, 253, 244, 0.9)',
      marginBottom: '24px',
    },
    '.login-success-alert .ant-alert-message': {
      fontWeight: 500,
      color: '#166534',
    },
    '.login-password-form': {
      display: 'flex',
      flexDirection: 'column',
      gap: '18px',
    },
    '.login-register-grid': {
      display: 'grid',
      gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
      gap: '14px 16px',
      '@media (max-width: 760px)': {
        gridTemplateColumns: '1fr',
      },
    },
    '.login-register-grid-full': {
      gridColumn: '1 / -1',
    },
    '.login-no-margin': {
      marginBottom: 0,
    },
    '.login-affix-input.ant-input-affix-wrapper': {
      minHeight: '50px',
      borderRadius: '16px',
      background: 'rgba(255,255,255,0.55)',
      border: '1px solid rgba(226,232,240,0.8)',
      boxShadow: '0 2px 8px rgba(148,163,184,0.12)',
      paddingInline: '14px',
      transition: 'all 0.2s ease',
    },
    '.login-affix-input.ant-input-affix-wrapper .ant-input': {
      background: 'transparent',
      fontSize: '15px',
      color: '#1e293b',
    },
    '.login-affix-input.ant-input-affix-wrapper .ant-input-prefix': {
      color: '#94a3b8',
      marginRight: '10px',
      fontSize: '17px',
    },
    '.login-affix-input.ant-input-affix-wrapper:hover': {
      borderColor: 'rgba(59, 130, 246, 0.42)',
    },
    '.login-affix-input.ant-input-affix-wrapper.ant-input-affix-wrapper-focused': {
      borderColor: 'rgba(59, 130, 246, 0.72)',
      boxShadow: '0 0 0 4px rgba(56, 189, 248, 0.14)',
    },
    '.login-plain-input.ant-input': {
      minHeight: '54px',
      borderRadius: '16px',
      background: 'rgba(255,255,255,0.55)',
      border: '1px solid rgba(226,232,240,0.8)',
      boxShadow: '0 2px 8px rgba(148,163,184,0.12)',
      fontSize: '24px',
      fontFamily: 'ui-monospace, SFMono-Regular, SFMono-Regular, Menlo, monospace',
      letterSpacing: '0.45em',
      textAlign: 'center',
      paddingInline: '16px',
    },
    '.login-plain-input.ant-input:focus, .login-plain-input.ant-input-focused': {
      borderColor: 'rgba(59, 130, 246, 0.72)',
      boxShadow: '0 0 0 4px rgba(56, 189, 248, 0.14)',
    },
    '.login-form-options': {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'space-between',
      minHeight: '24px',
    },
    '.login-remember.ant-checkbox-wrapper': {
      color: '#64748b',
      fontSize: '14px',
    },
    '.login-primary-button.ant-btn': {
      width: '100%',
      height: '52px',
      borderRadius: '16px',
      border: 'none',
      background: 'linear-gradient(90deg, #3b82f6 0%, #22d3ee 100%)',
      boxShadow: '0 16px 36px rgba(14, 165, 233, 0.26)',
      fontSize: '15px',
      fontWeight: 600,
      transition: 'all 0.2s ease',
    },
    '.login-primary-button.ant-btn:hover': {
      background: 'linear-gradient(90deg, #2563eb 0%, #06b6d4 100%)',
      transform: 'translateY(-1px)',
      boxShadow: '0 18px 38px rgba(14, 165, 233, 0.28)',
    },
    '.login-primary-button.ant-btn:disabled, .login-primary-button.ant-btn.ant-btn-loading': {
      transform: 'none',
      boxShadow: '0 12px 28px rgba(14, 165, 233, 0.16)',
    },
    '.login-register-prompt': {
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: '4px',
      minHeight: '28px',
      marginTop: '-6px',
    },
    '.login-register-prompt-text.ant-typography': {
      color: '#94a3b8',
      fontSize: '13px',
      fontWeight: 500,
    },
    '.login-register-link.ant-btn': {
      height: '26px',
      paddingInline: 0,
      color: '#0284c7',
      fontWeight: 600,
      fontSize: '13px',
      alignSelf: 'center',
    },
    '.login-register-link.ant-btn:hover': {
      color: '#0369a1',
    },
    '.login-secondary-section': {
      marginTop: '4px',
      paddingTop: '18px',
      paddingBottom: '2px',
      borderTop: '1px solid rgba(226,232,240,0.65)',
    },
    '.login-secondary-note.ant-typography': {
      display: 'block',
      textAlign: 'center',
      color: '#94a3b8',
      fontSize: '12px',
      marginBottom: '16px',
    },
    '.login-secondary-grid': {
      display: 'grid',
      gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
      gap: '10px',
      width: '100%',
    },
    '.login-secondary-grid-count-1': {
      gridTemplateColumns: '1fr',
    },
    '.login-secondary-grid-count-3': {
      gridTemplateColumns: 'repeat(3, minmax(0, 1fr))',
    },
    '.login-secondary-grid-count-4': {
      gridTemplateColumns: 'repeat(4, minmax(0, 1fr))',
    },
    '.login-secondary-button.ant-btn': {
      width: '100%',
      height: '44px',
      borderRadius: '14px',
      border: '1px solid rgba(125, 211, 252, 0.55)',
      background: 'rgba(239, 246, 255, 0.85)',
      color: '#0369a1',
      fontWeight: 600,
      boxShadow: 'none',
    },
    '.login-secondary-grid-count-1 .login-secondary-button.ant-btn': {
      width: '100%',
      height: '48px',
    },
    '.login-secondary-grid-compact .login-secondary-button.ant-btn': {
      minWidth: 0,
      paddingInline: 0,
      fontSize: '20px',
    },
    '.login-secondary-grid-compact .login-secondary-button.ant-btn .ant-btn-icon': {
      marginInlineEnd: 0,
    },
    '.login-secondary-button.login-secondary-button-muted.ant-btn': {
      borderColor: 'rgba(226,232,240,0.85)',
      background: 'rgba(255,255,255,0.58)',
      color: '#475569',
    },
    '.login-secondary-button.ant-btn:disabled': {
      opacity: 0.92,
      color: 'rgba(71, 85, 105, 0.9)',
    },
    '.login-captcha-row': {
      display: 'flex',
      gap: '10px',
      alignItems: 'stretch',
      gridColumn: '1 / -1',
      '@media (max-width: 576px)': {
        flexDirection: 'column',
      },
    },
    '.login-register-inline-row': {
      minWidth: 0,
    },
    '.login-captcha-field': {
      flex: 1,
      minWidth: 0,
    },
    '.login-captcha-button.ant-btn': {
      width: '170px',
      flex: '0 0 170px',
      height: '50px',
      borderRadius: '16px',
      border: '1px solid rgba(125, 211, 252, 0.7)',
      background: 'rgba(255,255,255,0.58)',
      position: 'relative',
      overflow: 'hidden',
      padding: '4px',
      '@media (max-width: 576px)': {
        width: '100%',
        flex: '0 0 50px',
      },
    },
    '.login-captcha-image': {
      width: '100%',
      height: '100%',
      objectFit: 'cover',
      borderRadius: '12px',
      display: 'block',
    },
    '.login-captcha-fallback': {
      color: '#0369a1',
      fontSize: '12px',
      fontWeight: 600,
    },
    '.login-captcha-overlay': {
      position: 'absolute',
      inset: 0,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      color: '#0284c7',
      background: 'rgba(255,255,255,0)',
      opacity: 0,
      transition: 'all 0.2s ease',
      fontSize: '16px',
    },
    '.login-captcha-button.ant-btn:hover .login-captcha-overlay': {
      opacity: 1,
      background: 'rgba(255,255,255,0.34)',
    },
    '.login-captcha-button-refreshing.ant-btn .login-captcha-image': {
      opacity: 0.62,
    },
    '.login-captcha-button-refreshing.ant-btn .login-captcha-overlay': {
      opacity: 1,
      background: 'rgba(255,255,255,0.48)',
    },
    '.login-email-send-row': {
      display: 'flex',
      gap: '10px',
      alignItems: 'stretch',
      gridColumn: '1 / -1',
      '@media (max-width: 576px)': {
        flexDirection: 'column',
      },
    },
    '.login-email-send-field': {
      flex: 1,
      minWidth: 0,
    },
    '.login-email-verify-row': {
      display: 'flex',
      gap: '10px',
      alignItems: 'stretch',
      gridColumn: '1 / -1',
      '@media (max-width: 576px)': {
        flexDirection: 'column',
      },
    },
    '.login-email-verify-field': {
      flex: 1,
      minWidth: 0,
    },
    '.login-email-code-button.ant-btn': {
      width: '112px',
      height: '50px',
      borderRadius: '16px',
      border: '1px solid rgba(125, 211, 252, 0.72)',
      background: 'rgba(255,255,255,0.58)',
      color: '#0369a1',
      fontSize: '13px',
      fontWeight: 700,
      paddingInline: '10px',
      boxShadow: '0 2px 8px rgba(14, 165, 233, 0.08)',
      '@media (max-width: 576px)': {
        width: '100%',
      },
    },
    '.login-email-code-button.ant-btn:disabled': {
      color: '#64748b',
      background: 'rgba(241, 245, 249, 0.8)',
      borderColor: 'rgba(203, 213, 225, 0.9)',
    },
    '.login-email-code-button-wide.ant-btn': {
      width: '142px',
      '@media (max-width: 576px)': {
        width: '100%',
      },
    },
    '.login-step-panel': {
      display: 'flex',
      flexDirection: 'column',
      gap: '20px',
    },
    '.login-step-panel-centered': {
      alignItems: 'center',
      textAlign: 'center',
      padding: '16px 0 8px',
    },
    '.login-back-row': {
      display: 'flex',
      alignItems: 'center',
      minHeight: '24px',
      marginTop: '-2px',
      marginBottom: '-2px',
    },
    '.login-back-button.ant-btn': {
      paddingInline: '4px',
      color: '#475569',
      fontWeight: 500,
    },
    '.login-card-register .login-back-button.ant-btn': {
      height: '32px',
      color: '#334155',
      fontSize: '14px',
      fontWeight: 600,
    },
    '.login-step-intro': {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      textAlign: 'center',
      gap: '8px',
      marginBottom: '4px',
    },
    '.login-step-icon': {
      width: '64px',
      height: '64px',
      borderRadius: '50%',
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
      fontSize: '30px',
    },
    '.login-step-icon-neutral': {
      background: '#f1f5f9',
      color: '#0ea5e9',
    },
    '.login-step-icon-danger': {
      background: '#fee2e2',
      color: '#dc2626',
      marginBottom: '4px',
    },
    '.login-step-title.ant-typography': {
      marginBottom: 0,
      color: '#1e293b',
      fontSize: '18px',
      fontWeight: 700,
    },
    '.login-step-description.ant-typography': {
      color: '#64748b',
      fontSize: '13px',
      lineHeight: 1.75,
      maxWidth: '340px',
      marginBottom: 0,
    },
    '.login-ghost-button.ant-btn': {
      width: '100%',
      height: '48px',
      borderRadius: '16px',
      border: '1px solid rgba(226,232,240,0.9)',
      background: 'rgba(255,255,255,0.7)',
      color: '#334155',
      fontWeight: 600,
    },
    '.login-lock-meta': {
      width: '100%',
      padding: '12px 14px',
      borderRadius: '14px',
      background: 'rgba(248,250,252,0.9)',
      border: '1px solid rgba(226,232,240,0.85)',
    },
    '.login-lock-text.ant-typography': {
      display: 'block',
      color: '#64748b',
      fontSize: '12px',
      textAlign: 'center',
      marginBottom: 0,
    },
    '.login-bootstrap': {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '280px',
      textAlign: 'center',
      gap: '16px',
    },
    '.login-bootstrap-text.ant-typography': {
      color: '#64748b',
      marginBottom: 0,
    },
    '.login-passkey-progress': {
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      gap: '18px',
      padding: '8px 18px 0',
    },
    '.login-passkey-progress-text.ant-typography': {
      color: '#64748b',
      fontSize: '13px',
      lineHeight: 1.75,
      textAlign: 'center',
      marginBottom: 0,
      maxWidth: '320px',
    },
    '.login-bootstrap-actions': {
      marginTop: '8px',
    },
    '@media (max-width: 1024px)': {
      '.login-grid': {
        justifyContent: 'center',
      },
      '.login-card': {
        marginRight: 0,
      },
    },
  });

  const blobClassName = css({
    position: 'absolute',
    borderRadius: '999px',
    filter: 'blur(120px)',
    opacity: 0.22,
    mixBlendMode: 'multiply' as const,
    animation: 'loginBlob 7s infinite',
    '@keyframes loginBlob': {
      '0%': {
        transform: 'translate(0px, 0px) scale(1)',
      },
      '33%': {
        transform: 'translate(30px, -50px) scale(1.08)',
      },
      '66%': {
        transform: 'translate(-24px, 24px) scale(0.94)',
      },
      '100%': {
        transform: 'translate(0px, 0px) scale(1)',
      },
    },
  });

  const panelMotion = {
    initial: { opacity: 0, x: 16, scale: 0.985 },
    animate: { opacity: 1, x: 0, scale: 1 },
    exit: { opacity: 0, x: -16, scale: 0.985 },
    transition: { duration: 0.26, ease: 'easeOut' as const },
  };

  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: '#3b82f6',
          borderRadius: 16,
        },
      }}
    >
      <div className={pageClassName}>
        <div
          className={blobClassName}
          style={{
            width: '40vw',
            height: '40vw',
            minWidth: 280,
            minHeight: 280,
            top: '-10%',
            left: '-10%',
            background: 'rgba(59, 130, 246, 1)',
          }}
        />
        <div
          className={blobClassName}
          style={{
            width: '35vw',
            height: '35vw',
            minWidth: 260,
            minHeight: 260,
            top: '20%',
            right: '-10%',
            background: 'rgba(103, 232, 249, 1)',
            animationDelay: '2s',
          }}
        />
        <div
          className={blobClassName}
          style={{
            width: '50vw',
            height: '50vw',
            minWidth: 340,
            minHeight: 340,
            bottom: '-20%',
            left: '20%',
            background: 'rgba(125, 211, 252, 1)',
            animationDelay: '4s',
          }}
        />
        <div
          style={{
            position: 'absolute',
            inset: 0,
            opacity: 0.48,
            backgroundImage:
              'linear-gradient(rgba(15,23,42,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(15,23,42,0.03) 1px, transparent 1px)',
            backgroundSize: '40px 40px',
            pointerEvents: 'none',
          }}
        />

        <div className="login-grid">
          <div className={`login-card ${authPanel === 'register' ? 'login-card-register' : ''}`}>
            <div className="login-header">
              <div className="login-logo">
                {loginLogoPath ? (
                  <img src={loginLogoPath} alt={brandTitle} referrerPolicy="no-referrer" />
                ) : (
                  <SafetyCertificateOutlined />
                )}
              </div>
              <Title level={1} className="login-title">
                {brandTitle}
              </Title>
              <Paragraph className="login-subtitle">
                {brandSubtitle}
              </Paragraph>
            </div>

            {bootstrapError ? (
              <>
                <Alert
                  type="error"
                  showIcon
                  className="login-alert"
                  title={bootstrapError}
                />
                <div className="login-bootstrap-actions">
                  <Button
                    type="primary"
                    className="login-primary-button"
                    onClick={() => {
                      void bootstrapLogin();
                    }}
                  >
                    重新初始化登录
                  </Button>
                </div>
              </>
            ) : null}

            {!bootstrapError && loginAlertMessage ? (
              <Alert
                type="error"
                showIcon
                className="login-alert"
                title={loginAlertMessage}
              />
            ) : null}

            {!bootstrapError && registerInfoMessage ? (
              <Alert
                type="info"
                showIcon
                className="login-info-alert"
                title={registerInfoMessage}
              />
            ) : null}

            {!bootstrapError && !loginAlertMessage && registerSuccessMessage ? (
              <Alert
                type="success"
                showIcon
                className="login-success-alert"
                title={registerSuccessMessage}
              />
            ) : null}

            {bootstrapping ? (
              <div className="login-bootstrap">
                <Spin size="large" />
                <Text className="login-bootstrap-text">正在准备统一登录上下文...</Text>
              </div>
            ) : null}

            {!bootstrapping && !bootstrapError && loginOptionsUnavailable ? (
              <div className="login-bootstrap">
                <Text className="login-bootstrap-text">平台登录配置不可用，请稍后重试。</Text>
                <Button
                  type="primary"
                  className="login-primary-button"
                  loading={loadingLoginOptions}
                  onClick={reloadLoginOptions}
                >
                  重新加载登录配置
                </Button>
              </div>
            ) : null}

            {!bootstrapping && !bootstrapError && !loginOptionsUnavailable ? (
              <AnimatePresence mode="wait">
                {loginState.stage === 'password' && authPanel === 'password' ? (
                  <motion.div key="password" {...panelMotion}>
                    <PasswordPanel
                      initialAccount={loginState.currentAccount || registerInitialAccount}
                      checkingPasswordState={loginState.checkingPasswordState}
                      refreshingCaptcha={loginState.refreshingCaptcha}
                      submittingPassword={loginState.submittingPassword}
                      startingPasskey={loginState.startingPasskey}
                      rememberSession={loginState.rememberSession}
                      captchaRequired={loginState.passwordState.captchaRequired}
                      captcha={loginState.passwordState.captcha}
                      passwordLoginEnabled={passwordLoginEnabled}
                      passkeyLoginEnabled={passkeyLoginEnabled}
                      externalLoginMethods={externalLoginMethods}
                      loadingLoginOptions={loadingLoginOptions}
                      registerEnabled={formRegisterEnabled}
                      onRememberSessionChange={loginState.setRememberSession}
                      onAccountBlur={(account) => {
                        void loginState.checkPasswordState(account);
                      }}
                      onSubmit={(values) => {
                        void loginState.submitPassword(values);
                      }}
                      onPasskeyLogin={(userAccount) => {
                        void loginState.startPasskey(userAccount);
                      }}
                      onExternalLogin={(method) => {
                        try {
                          const loginUrl = resolveExternalLoginUrl(method.loginUrl);
                          window.location.href = loginUrl;
                        } catch (error) {
                          setLoginOptionsError(
                            (error as { message?: string })?.message || '外部登录地址不合法',
                          );
                        }
                      }}
                      onRefreshCaptcha={() => {
                        void loginState.refreshCaptcha();
                      }}
                      onRegisterClick={openRegisterPanel}
                    />
                  </motion.div>
                ) : null}

                {authPanel === 'register' ? (
                  <motion.div key="register" {...panelMotion}>
                    <RegisterPanel
                      initialAccount={registerInitialAccount || loginState.currentAccount}
                      loadingState={loadingRegisterState}
                      submitting={submittingRegister}
                      captcha={registerCaptcha}
                      sessionResetKey={registerSessionResetKey}
                      onBack={closeRegisterPanel}
                      onAccountBlur={(account) => {
                        void loadRegisterState(account);
                      }}
                      onRefreshCaptcha={(account) => {
                        void loadRegisterState(account);
                      }}
                      onSubmit={(values) => {
                        void submitRegister(values);
                      }}
                      onSendEmailCode={sendRegisterEmailCode}
                    />
                  </motion.div>
                ) : null}

                {loginState.stage === 'passkeyPrompt' ? (
                  <motion.div key="passkeyPrompt" {...panelMotion}>
                    <PasskeyPanel
                      currentAccount={loginState.currentAccount}
                      startingPasskey={loginState.startingPasskey}
                      onBack={() => {
                        loginState.clearError();
                        loginState.returnToLocked();
                      }}
                    />
                  </motion.div>
                ) : null}

                {loginState.stage === 'totp' ? (
                  <motion.div key="totp" {...panelMotion}>
                    <TotpPanel
                      verifyingTotp={loginState.verifyingTotp}
                      onBack={loginState.returnToLocked}
                      onSubmit={(otpCode) => {
                        void loginState.submitTotp(otpCode);
                      }}
                    />
                  </motion.div>
                ) : null}

                {loginState.stage === 'locked' ? (
                  <motion.div key="locked" {...panelMotion}>
                    <LockedPanel
                      lockDescription={lockDescription}
                    />
                  </motion.div>
                ) : null}
              </AnimatePresence>
            ) : null}
          </div>
        </div>
      </div>
    </ConfigProvider>
  );
}
