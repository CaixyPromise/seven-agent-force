import { request } from './request';

interface ResultEnvelope<T> {
  code: number;
  message?: string;
  data?: T;
  success?: boolean;
}

export interface UserSelfProfileVO {
  userId?: API.Int64;
  userAccount?: string;
  accountName?: string;
  nickName?: string;
  userEmail?: string;
  email?: string;
  userPhone?: string;
  phone?: string;
  userProfile?: string;
  profile?: string;
  userAvatar?: string;
  status?: number;
  enabled?: boolean;
  passwordChangedAt?: string;
  emailVerified?: boolean;
  emailVerifiedAt?: string;
  emailVersion?: API.Int64;
}

export interface UserSelfProfileUpdateRequest {
  nickName?: string;
  userPhone?: string;
  userProfile?: string;
}

export interface UserSelfEmailUpdateRequest {
  userEmail: string;
  expectedEmailVersion: API.Int64;
}

export interface UserSelfPasswordChangeRequest {
  oldPassword: string;
  newPassword: string;
  confirmPassword: string;
}

export interface UserAvatarCommitRequest {
  fileId: API.Int64;
}

export async function getCurrentUserProfile(options?: Record<string, unknown>) {
  return request<ResultEnvelope<UserSelfProfileVO>>('/api/user/profile/me', {
    method: 'GET',
    ...(options || {}),
  });
}

export async function updateCurrentUserProfile(
  body: UserSelfProfileUpdateRequest,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<boolean>>('/api/user/profile/update', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

export async function updateCurrentUserEmail(
  body: UserSelfEmailUpdateRequest,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<boolean>>('/api/user/profile/email/update', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

export async function changeCurrentUserPassword(
  body: UserSelfPasswordChangeRequest,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<boolean>>('/api/user/profile/change-password', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

export async function commitCurrentUserAvatar(
  body: UserAvatarCommitRequest,
  options?: Record<string, unknown>,
) {
  return request<ResultEnvelope<boolean | Record<string, unknown>>>('/api/user/profile/avatar/commit', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}
