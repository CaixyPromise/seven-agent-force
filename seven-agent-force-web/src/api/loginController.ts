import { request } from '@/api/request';
import type {
  ApiResponse,
  LoginPasskeyStartRequest,
  LoginPasskeyStartResult,
  LoginPasskeyVerifyRequest,
  LoginPasskeyVerifyResult,
  LoginPasswordState,
  LoginPasswordStateRequest,
  LoginPasswordSubmitRequest,
  LoginPasswordSubmitResult,
  LoginRegisterEmailCodeRequest,
  LoginRegisterEmailCodeResult,
  LoginRegisterState,
  LoginRegisterStateRequest,
  LoginRegisterSubmitRequest,
  LoginRegisterSubmitResult,
  LoginTotpVerifyRequest,
  LoginTotpVerifyResult,
} from '@/lib/http/types';

export async function getLoginPasswordState(body: LoginPasswordStateRequest) {
  return request<ApiResponse<LoginPasswordState>>('/api/login/password/state', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function submitLoginPassword(body: LoginPasswordSubmitRequest) {
  return request<ApiResponse<LoginPasswordSubmitResult>>('/api/login/password', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function getLoginRegisterState(body: LoginRegisterStateRequest) {
  return request<ApiResponse<LoginRegisterState>>('/api/login/register/state', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function submitLoginRegister(body: LoginRegisterSubmitRequest) {
  return request<ApiResponse<LoginRegisterSubmitResult>>('/api/login/register', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function sendLoginRegisterEmailCode(body: LoginRegisterEmailCodeRequest) {
  return request<ApiResponse<LoginRegisterEmailCodeResult>>('/api/login/register/email-code', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function startLoginPasskey(body: LoginPasskeyStartRequest) {
  return request<ApiResponse<LoginPasskeyStartResult>>('/api/login/passkey/start', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function verifyLoginPasskey(body: LoginPasskeyVerifyRequest) {
  return request<ApiResponse<LoginPasskeyVerifyResult>>('/api/login/passkey/verify', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}

export async function verifyLoginTotp(body: LoginTotpVerifyRequest) {
  return request<ApiResponse<LoginTotpVerifyResult>>('/api/login/totp/verify', {
    method: 'POST',
    data: body,
    skipAuthRefresh: true,
    skipAuthRedirect: true,
    skipGlobalChallenge: true,
  });
}
