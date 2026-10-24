"use client";

import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  getLoginPasswordState,
  startLoginPasskey,
  submitLoginPassword,
  verifyLoginPasskey,
  verifyLoginTotp,
} from '@/api/loginController';
import { getOrCreateDeviceId } from '@/lib/auth/device';
import type {
  LoginPasskeyStartResult,
  LoginPasskeyVerifyResult,
  LoginPasswordState,
  LoginPasswordSubmitResult,
  LoginTotpVerifyResult,
} from '@/lib/http/types';
import {
  buildPasskeyLocalhostUrl as buildPasskeyLocalhostRedirectUrl,
  shouldSwitchPasskeyToLocalhostDomain,
} from '@/lib/security/passkeyDomain';
import { createPasskeyAssertionPayload } from '@/lib/security/webauthn';
import { isExpiredLoginSessionError } from '../lib/login-session';

export type LoginPageStage = 'password' | 'passkeyPrompt' | 'totp' | 'locked';

const DEFAULT_PASSWORD_STATE: LoginPasswordState = {
  canPasswordLogin: true,
  captchaRequired: false,
  totpRequired: false,
  locked: false,
  lockExpiresAt: null,
  unlockMethod: null,
  captcha: null,
};

type LoginErrorPayload = {
  code?: number;
  message?: string;
  error?: string;
  error_description?: string;
};

function readErrorPayload(error: unknown): LoginErrorPayload {
  const payload = (error as { payload?: LoginErrorPayload })?.payload;
  if (payload) {
    return payload;
  }
  const responsePayload = (error as { response?: { data?: LoginErrorPayload } })?.response?.data;
  if (responsePayload && typeof responsePayload === 'object') {
    return responsePayload;
  }
  return {};
}

function readErrorMessage(error: unknown, fallback: string) {
  const payload = readErrorPayload(error);
  const payloadMessage = payload.message || payload.error_description;
  const messageText = (error as { message?: string })?.message;
  return payloadMessage || messageText || fallback;
}

function isPasskeyPrimaryStageCompletedError(error: unknown) {
  const payload = readErrorPayload(error);
  const resolvedMessage = `${payload.message || ''} ${payload.error_description || ''}`.trim();
  return payload.error === 'authentication_stage_completed'
    || resolvedMessage.includes('主认证已完成')
    || resolvedMessage.includes('继续完成挑战');
}

function isPasskeyCancelledError(error: unknown) {
  const name = (error as { name?: string })?.name || '';
  const message = ((error as { message?: string })?.message || '').toLowerCase();
  return name === 'NotAllowedError'
    || message.includes('notallowederror')
    || message.includes('operation either timed out or was not allowed');
}

function mergePasswordState(
  source?: Partial<LoginPasswordState> | null,
  base: LoginPasswordState = DEFAULT_PASSWORD_STATE,
): LoginPasswordState {
  if (!source) {
    return { ...base };
  }
  return {
    canPasswordLogin: source.canPasswordLogin ?? base.canPasswordLogin,
    captchaRequired: source.captchaRequired ?? base.captchaRequired,
    totpRequired: source.totpRequired ?? base.totpRequired,
    locked: source.locked ?? base.locked,
    lockExpiresAt: normalizeLockExpiresAt(source.lockExpiresAt ?? base.lockExpiresAt ?? null),
    unlockMethod: source.unlockMethod ?? base.unlockMethod ?? null,
    captcha: source.captcha ?? base.captcha ?? null,
  };
}

function normalizeLockExpiresAt(value?: LoginPasswordState['lockExpiresAt']) {
  if (value === null || value === undefined || value === '') {
    return null;
  }
  const parsed = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
}

interface UseLoginPageStateOptions {
  loginTransactionId: string;
  loginContextId?: string;
  initialUserAccount?: string;
  onAuthenticated: (redirectUrl?: string | null) => void | Promise<void>;
  onRefreshLoginSession: () => Promise<{
    loginTransactionId: string;
    loginContextId: string;
  }>;
}

function buildPasskeyLocalhostUrl(loginTransactionId: string, userAccount: string) {
  return buildPasskeyLocalhostRedirectUrl({
    loginTransactionId,
    userAccount,
    deviceId: getOrCreateDeviceId(),
    passkeyAuto: '1',
  });
}

export function useLoginPageState(options: UseLoginPageStateOptions) {
  const {
    loginTransactionId,
    loginContextId,
    initialUserAccount,
    onAuthenticated,
    onRefreshLoginSession,
  } = options;
  const normalizedInitialUserAccount = (initialUserAccount || '').trim();
  const [stage, setStage] = useState<LoginPageStage>('password');
  const [currentAccount, setCurrentAccount] = useState(normalizedInitialUserAccount);
  const [rememberSession, setRememberSession] = useState(true);
  const [passwordState, setPasswordState] = useState<LoginPasswordState>(DEFAULT_PASSWORD_STATE);
  const [errorMessage, setErrorMessage] = useState('');
  const [checkingPasswordState, setCheckingPasswordState] = useState(false);
  const [refreshingCaptcha, setRefreshingCaptcha] = useState(false);
  const [submittingPassword, setSubmittingPassword] = useState(false);
  const [startingPasskey, setStartingPasskey] = useState(false);
  const [verifyingTotp, setVerifyingTotp] = useState(false);
  const lockExpiresAt = useMemo(
    () => passwordState.lockExpiresAt ?? null,
    [passwordState.lockExpiresAt],
  );

  const clearError = useCallback(() => {
    setErrorMessage('');
  }, []);

  useEffect(() => {
    if (!normalizedInitialUserAccount) {
      return;
    }
    setCurrentAccount((current) => current || normalizedInitialUserAccount);
  }, [normalizedInitialUserAccount]);

  const applyPasswordState = useCallback((nextState?: Partial<LoginPasswordState> | null) => {
    const mergedState = mergePasswordState(nextState, DEFAULT_PASSWORD_STATE);
    setPasswordState(mergedState);
    if (mergedState.locked) {
      setStage('locked');
      return;
    }
    if (mergedState.totpRequired) {
      setStage('totp');
      return;
    }
    setStage('password');
  }, []);

  const applyPasswordPreflightState = useCallback((
    nextState?: Partial<LoginPasswordState> | null,
  ) => {
    const mergedState = mergePasswordState(nextState, DEFAULT_PASSWORD_STATE);
    setPasswordState(mergedState);
    setStage(mergedState.locked ? 'locked' : 'password');
  }, []);

  const handleAuthenticated = useCallback(async (redirectUrl?: string | null) => {
    clearError();
    setPasswordState(DEFAULT_PASSWORD_STATE);
    setStage('password');
    await onAuthenticated(redirectUrl);
  }, [clearError, onAuthenticated]);

  const replaceExpiredLoginSession = useCallback(async (normalizedAccount: string) => {
    if (!loginTransactionId) {
      throw new Error('登录事务尚未就绪，请稍后重试');
    }
    clearError();
    setPasswordState(DEFAULT_PASSWORD_STATE);
    setStage('password');
    const nextSession = await onRefreshLoginSession();
    setCurrentAccount(normalizedAccount);
    return nextSession;
  }, [clearError, loginTransactionId, onRefreshLoginSession]);

  const ensureLoginTransactionForAccount = useCallback(async (normalizedAccount: string) => {
    const previousAccount = currentAccount.trim();
    if (!loginTransactionId) {
      throw new Error('登录事务尚未就绪，请稍后重试');
    }
    if (!previousAccount || previousAccount === normalizedAccount) {
      return { loginTransactionId, loginContextId: loginContextId || '' };
    }
    clearError();
    setPasswordState(DEFAULT_PASSWORD_STATE);
    setStage('password');
    return onRefreshLoginSession();
  }, [clearError, currentAccount, loginContextId, loginTransactionId, onRefreshLoginSession]);

  const checkPasswordState = useCallback(async (
    userAccount: string,
    options?: { refreshCaptcha?: boolean },
  ) => {
    const normalizedAccount = userAccount.trim();
    if (!loginTransactionId || !normalizedAccount) {
      setCurrentAccount(normalizedAccount);
      setPasswordState(DEFAULT_PASSWORD_STATE);
      setStage('password');
      return null;
    }
    setCheckingPasswordState(true);
    try {
      clearError();
      const activeSession = await ensureLoginTransactionForAccount(normalizedAccount);
      const response = await getLoginPasswordState({
        loginTransactionId: activeSession.loginTransactionId,
        loginContextId: activeSession.loginContextId || undefined,
        userAccount: normalizedAccount,
        refreshCaptcha: options?.refreshCaptcha,
      });
      const nextState = response.data;
      if (!nextState) {
        throw new Error('登录状态返回为空');
      }
      setCurrentAccount(normalizedAccount);
      applyPasswordPreflightState(nextState);
      return nextState;
    } catch (error) {
      if (isExpiredLoginSessionError(error)) {
        try {
          const refreshedSession = await replaceExpiredLoginSession(normalizedAccount);
          const refreshedResponse = await getLoginPasswordState({
            loginTransactionId: refreshedSession.loginTransactionId,
            loginContextId: refreshedSession.loginContextId,
            userAccount: normalizedAccount,
            refreshCaptcha: options?.refreshCaptcha,
          });
          const refreshedState = refreshedResponse.data;
          if (!refreshedState) {
            throw new Error('登录状态返回为空');
          }
          applyPasswordPreflightState(refreshedState);
          return refreshedState;
        } catch (refreshError) {
          const nextMessage = readErrorMessage(refreshError, '登录事务已失效，请稍后重试');
          setErrorMessage(nextMessage);
          return null;
        }
      }
      const nextMessage = readErrorMessage(error, '获取登录状态失败');
      setErrorMessage(nextMessage);
      return null;
    } finally {
      setCheckingPasswordState(false);
    }
  }, [
    applyPasswordPreflightState,
    clearError,
    ensureLoginTransactionForAccount,
    loginTransactionId,
    replaceExpiredLoginSession,
  ]);

  const refreshCaptcha = useCallback(async () => {
    if (!currentAccount) {
      return;
    }
    setRefreshingCaptcha(true);
    try {
      await checkPasswordState(currentAccount, { refreshCaptcha: true });
    } finally {
      setRefreshingCaptcha(false);
    }
  }, [checkPasswordState, currentAccount]);

  const applyPasswordSubmitResult = useCallback(async (
    result: LoginPasswordSubmitResult,
    request: { password: string; captchaCode?: string },
  ) => {
    if (result.authenticated) {
      await handleAuthenticated(result.redirectUrl);
      return;
    }
    if (result.captchaRequired && !result.totpRequired && !result.locked && request.password) {
      setErrorMessage(
        request.captchaCode
          ? '账号、密码或图形验证码错误，请重新输入。'
          : '请先完成图形验证码校验。',
      );
    }
    applyPasswordState(result);
  }, [applyPasswordState, handleAuthenticated]);

  const submitPassword = useCallback(async (values: {
    userAccount: string;
    password: string;
    captchaCode?: string;
  }) => {
    const normalizedAccount = values.userAccount.trim();
    if (!loginTransactionId) {
      setErrorMessage('登录事务尚未就绪，请稍后重试');
      return;
    }
    setSubmittingPassword(true);
    try {
      clearError();
      const activeSession = await ensureLoginTransactionForAccount(normalizedAccount);
      const response = await submitLoginPassword({
        loginTransactionId: activeSession.loginTransactionId,
        loginContextId: activeSession.loginContextId || undefined,
        userAccount: normalizedAccount,
        password: values.password,
        captchaCode: values.captchaCode?.trim() || undefined,
      });
      const result = response.data;
      if (!result) {
        throw new Error('登录结果返回为空');
      }
      setCurrentAccount(normalizedAccount);
      await applyPasswordSubmitResult(result, values);
    } catch (error) {
      if (isExpiredLoginSessionError(error)) {
        try {
          const refreshedSession = await replaceExpiredLoginSession(normalizedAccount);
          const refreshedResponse = await submitLoginPassword({
            loginTransactionId: refreshedSession.loginTransactionId,
            loginContextId: refreshedSession.loginContextId,
            userAccount: normalizedAccount,
            password: values.password,
            captchaCode: values.captchaCode?.trim() || undefined,
          });
          const refreshedResult = refreshedResponse.data;
          if (!refreshedResult) {
            throw new Error('登录结果返回为空');
          }
          await applyPasswordSubmitResult(refreshedResult, values);
          return;
        } catch (refreshError) {
          const nextMessage = readErrorMessage(refreshError, '登录事务已失效，请重新输入账号密码');
          setErrorMessage(nextMessage);
          return;
        }
      }
      const nextMessage = readErrorMessage(error, '登录失败');
      setErrorMessage(nextMessage);
    } finally {
      setSubmittingPassword(false);
    }
  }, [
    applyPasswordSubmitResult,
    clearError,
    ensureLoginTransactionForAccount,
    loginTransactionId,
    replaceExpiredLoginSession,
  ]);

  const executePasskeyFlow = useCallback(async (
    activeLoginTransactionId: string,
    activeLoginContextId: string,
    normalizedAccount: string,
  ) => {
    const startResponse = await startLoginPasskey({
      loginTransactionId: activeLoginTransactionId,
      loginContextId: activeLoginContextId || undefined,
      userAccount: normalizedAccount,
    });
    const startResult: LoginPasskeyStartResult | undefined = startResponse.data;
    if (!startResult?.stepIdentifier) {
      throw new Error('通行密钥挑战返回为空');
    }
    setStage('passkeyPrompt');
    const payload = await createPasskeyAssertionPayload(startResult.userInterfaceHints);
    const verifyResponse = await verifyLoginPasskey({
      loginTransactionId: activeLoginTransactionId,
      userAccount: normalizedAccount,
      credentialIdentifier: payload.credentialIdentifier,
      clientDataJSON: payload.clientDataJSON,
      authenticatorData: payload.authenticatorData,
      signature: payload.signature,
    });
    const verifyResult: LoginPasskeyVerifyResult | undefined = verifyResponse.data;
    if (!verifyResult) {
      throw new Error('通行密钥校验结果返回为空');
    }
    if (verifyResult.authenticated) {
      await handleAuthenticated(verifyResult.redirectUrl);
      return;
    }
    if (verifyResult.locked) {
      applyPasswordState({
        ...DEFAULT_PASSWORD_STATE,
        locked: true,
        canPasswordLogin: false,
        lockExpiresAt: verifyResult.lockExpiresAt,
        unlockMethod: null,
      });
      return;
    }
    setStage('password');
    setErrorMessage('通行密钥验证未通过，请重试或改用密码登录。');
  }, [applyPasswordState, handleAuthenticated]);

  const startPasskey = useCallback(async (userAccount: string) => {
    const normalizedAccount = userAccount.trim();
    if (!loginTransactionId) {
      setErrorMessage('登录事务尚未就绪，请稍后重试');
      return;
    }
    if (!normalizedAccount) {
      setErrorMessage('请输入账号后再使用通行密钥');
      return;
    }
    setStartingPasskey(true);
    try {
      clearError();
      const activeSession = await ensureLoginTransactionForAccount(normalizedAccount);
      setCurrentAccount(normalizedAccount);
      if (shouldSwitchPasskeyToLocalhostDomain()) {
        window.location.assign(buildPasskeyLocalhostUrl(activeSession.loginTransactionId, normalizedAccount));
        return;
      }
      await executePasskeyFlow(
        activeSession.loginTransactionId,
        activeSession.loginContextId,
        normalizedAccount,
      );
    } catch (error) {
      if (isPasskeyPrimaryStageCompletedError(error)) {
        try {
          const refreshedSession = await replaceExpiredLoginSession(normalizedAccount);
          await executePasskeyFlow(
            refreshedSession.loginTransactionId,
            refreshedSession.loginContextId,
            normalizedAccount,
          );
          return;
        } catch (refreshError) {
          const nextMessage = readErrorMessage(refreshError, '登录事务已刷新失败，请重试通行密钥');
          setStage('password');
          setErrorMessage(nextMessage);
          return;
        }
      }
      if (isExpiredLoginSessionError(error)) {
        try {
          const refreshedSession = await replaceExpiredLoginSession(normalizedAccount);
          await executePasskeyFlow(
            refreshedSession.loginTransactionId,
            refreshedSession.loginContextId,
            normalizedAccount,
          );
          return;
        } catch (refreshError) {
          const nextMessage = readErrorMessage(refreshError, '登录事务已失效，请重新点击通行密钥');
          setStage('password');
          setErrorMessage(nextMessage);
          return;
        }
      }
      if (isPasskeyCancelledError(error)) {
        setStage('password');
        setErrorMessage('已取消通行密钥验证，可再次点击重试。');
        return;
      }
      const nextMessage = readErrorMessage(error, '通行密钥验证失败');
      setStage('password');
      setErrorMessage(nextMessage);
    } finally {
      setStartingPasskey(false);
    }
  }, [
    clearError,
    ensureLoginTransactionForAccount,
    executePasskeyFlow,
    loginTransactionId,
    replaceExpiredLoginSession,
  ]);

  const submitTotp = useCallback(async (otpCode: string) => {
    if (!loginTransactionId || !currentAccount) {
      setErrorMessage('登录上下文已失效，请重新输入账号密码');
      return;
    }
    setVerifyingTotp(true);
    try {
      clearError();
      const response = await verifyLoginTotp({
        loginTransactionId,
        userAccount: currentAccount,
        otpCode: otpCode.trim(),
      });
      const result: LoginTotpVerifyResult | undefined = response.data;
      if (!result) {
        throw new Error('动态验证码校验结果返回为空');
      }
      if (result.authenticated) {
        await handleAuthenticated(result.redirectUrl);
        return;
      }
      if (result.locked) {
        applyPasswordState({
          ...DEFAULT_PASSWORD_STATE,
          locked: true,
          canPasswordLogin: false,
          lockExpiresAt: result.lockExpiresAt,
          unlockMethod: null,
        });
        return;
      }
      setErrorMessage('无效的动态验证码，请检查您的身份验证器应用。');
    } catch (error) {
      if (isExpiredLoginSessionError(error)) {
        try {
          const refreshedSession = await replaceExpiredLoginSession(currentAccount);
          const refreshedResponse = await getLoginPasswordState({
            loginTransactionId: refreshedSession.loginTransactionId,
            loginContextId: refreshedSession.loginContextId,
            userAccount: currentAccount,
          });
          const refreshedState = refreshedResponse.data;
          if (refreshedState) {
            applyPasswordPreflightState(refreshedState);
          }
          setErrorMessage('登录事务已刷新，请重新输入密码');
          return;
        } catch (refreshError) {
          const nextMessage = readErrorMessage(refreshError, '登录事务已失效，请重新输入账号密码');
          setErrorMessage(nextMessage);
          return;
        }
      }
      const nextMessage = readErrorMessage(error, '动态验证码校验失败');
      setErrorMessage(nextMessage);
    } finally {
      setVerifyingTotp(false);
    }
  }, [
    applyPasswordState,
    applyPasswordPreflightState,
    clearError,
    currentAccount,
    handleAuthenticated,
    loginTransactionId,
    replaceExpiredLoginSession,
  ]);

  const returnToLocked = useCallback(() => {
    clearError();
    setStage(passwordState.locked ? 'locked' : 'password');
  }, [clearError, passwordState.locked]);

  useEffect(() => {
    setStage('password');
    setPasswordState(DEFAULT_PASSWORD_STATE);
    setErrorMessage('');
  }, [loginTransactionId]);

  useEffect(() => {
    if (!currentAccount || !lockExpiresAt) {
      return;
    }
    if (stage === 'locked' && Number(lockExpiresAt) <= Date.now()) {
      void checkPasswordState(currentAccount);
    }
  }, [checkPasswordState, currentAccount, lockExpiresAt, stage]);

  return {
    stage,
    currentAccount,
    rememberSession,
    setRememberSession,
    passwordState,
    lockExpiresAt,
    errorMessage,
    checkingPasswordState,
    refreshingCaptcha,
    submittingPassword,
    startingPasskey,
    verifyingTotp,
    clearError,
    checkPasswordState,
    refreshCaptcha,
    submitPassword,
    startPasskey,
    submitTotp,
    returnToLocked,
  };
}
