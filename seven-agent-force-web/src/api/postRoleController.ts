import { request } from './request';

type ApiIdentifier = string | number;

type ResultEnvelope<T> = {
  code?: number;
  message?: string;
  data?: T;
};

export function getPostRoleIds(postId: ApiIdentifier) {
  return request<ResultEnvelope<ApiIdentifier[]>>(`/api/system/post/${postId}/roles`, {
    method: 'GET',
  });
}

export function replacePostRoleIds(postId: ApiIdentifier, roleIds: ApiIdentifier[]) {
  return request<ResultEnvelope<boolean>>(`/api/system/post/${postId}/roles`, {
    method: 'POST',
    data: roleIds.map(String),
  });
}
