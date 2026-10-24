import { request } from '@/api/request';
import {
  buildReasonedRequest,
  buildHubChallengeFingerprint,
  buildSessionRevokeRequest,
  buildUserStatusRequest,
  normalizeHubFederationStatus,
  normalizeHubLoginPolicy,
  normalizeHubNode,
  normalizeHubNodeHealth,
  normalizeHubNodePage,
  normalizeHubSessionPage,
  normalizeHubUser,
  normalizeHubUserPage,
  unwrapHubResponse,
  type HubApiResponse,
  type HubDiscoveryTypeValue,
  type HubFederationStatus,
  type HubLoginPolicy,
  type HubNodeHealth,
  type HubNodePage,
  type HubNodeRecord,
  type HubNodeStatusValue,
  type HubSessionPage,
  type HubUserPage,
  type HubUserRecord,
  type HubUserStatusValue,
  type SessionRevokeCommand,
  type UserStatusCommand,
} from '@/app/system/hub-node/controllerContract';

export * from '@/app/system/hub-node/controllerContract';

export const HUB_NODE_ENDPOINT = '/api/system/hub/nodes';

export interface HubNodePageQuery {
  current?: number;
  size?: number;
  keyword?: string;
  status?: HubNodeStatusValue;
}

export interface HubNodeSaveRequest {
  nodeCode: string;
  nodeName: string;
  status: HubNodeStatusValue;
  discoveryType: HubDiscoveryTypeValue;
  serviceName?: string;
  managementBaseUrl?: string;
  hubIssuer: string;
  managementBearer?: string;
  capabilities: string[];
}

export interface HubNodeCopyRequest {
  nodeCode: string;
  nodeName: string;
  managementBearer?: string;
}

export interface HubUserPageQuery {
  current?: number;
  size?: number;
  keyword?: string;
  status?: HubUserStatusValue;
}

export interface HubSessionPageQuery {
  current?: number;
  size?: number;
}

export interface HubLoginPolicyApplyRequest extends HubLoginPolicy {
  reason: string;
}

export interface HubFederationProvisionRequest {
  connectionVersion: string;
  displayName: string;
  redirectUri: string;
  rotateSecret?: boolean;
  reason: string;
}

function nodePath(nodeCode: string) {
  return `${HUB_NODE_ENDPOINT}/${encodeURIComponent(nodeCode)}`;
}

function userPath(nodeCode: string, userId: string) {
  return `${nodePath(nodeCode)}/users/${encodeURIComponent(userId)}`;
}

async function send<T>(
  url: string,
  options: NonNullable<Parameters<typeof request>[1]>,
  fallback: string,
) {
  const challengeSafeOptions = {
    ...options,
    _challengeFingerprint: await buildHubChallengeFingerprint(
      options.method || 'GET',
      url,
      options.data,
    ),
  };
  const response = await request<HubApiResponse<T>>(url, challengeSafeOptions);
  return unwrapHubResponse(response, fallback);
}

function savePayload(data: HubNodeSaveRequest) {
  const managementBearer = data.managementBearer?.trim();
  return {
    nodeCode: data.nodeCode.trim(),
    nodeName: data.nodeName.trim(),
    status: data.status,
    discoveryType: data.discoveryType,
    serviceName: data.discoveryType === 'CONSUL' ? data.serviceName?.trim() : undefined,
    managementBaseUrl:
      data.discoveryType === 'STATIC' ? data.managementBaseUrl?.trim() : undefined,
    hubIssuer: data.hubIssuer.trim(),
    ...(managementBearer ? { managementBearer } : {}),
    capabilitiesJson: JSON.stringify(data.capabilities),
  };
}

export async function listHubNodes(params: HubNodePageQuery): Promise<HubNodePage> {
  const data = await send<unknown>(HUB_NODE_ENDPOINT, { method: 'GET', params }, '节点列表加载失败');
  return normalizeHubNodePage(data);
}

export async function getHubNode(nodeCode: string): Promise<HubNodeRecord> {
  const data = await send<unknown>(nodePath(nodeCode), { method: 'GET' }, '节点详情加载失败');
  return normalizeHubNode(data);
}

export async function createHubNode(data: HubNodeSaveRequest): Promise<HubNodeRecord> {
  const result = await send<unknown>(
    HUB_NODE_ENDPOINT,
    { method: 'POST', data: savePayload(data) },
    '节点创建失败',
  );
  return normalizeHubNode(result);
}

export async function updateHubNode(
  originalNodeCode: string,
  data: HubNodeSaveRequest,
): Promise<HubNodeRecord> {
  const result = await send<unknown>(
    nodePath(originalNodeCode),
    { method: 'PUT', data: savePayload(data) },
    '节点更新失败',
  );
  return normalizeHubNode(result);
}

export async function copyHubNode(
  sourceNodeCode: string,
  data: HubNodeCopyRequest,
): Promise<HubNodeRecord> {
  const managementBearer = data.managementBearer?.trim();
  const result = await send<unknown>(
    `${nodePath(sourceNodeCode)}/copy`,
    {
      method: 'POST',
      data: {
        nodeCode: data.nodeCode.trim(),
        nodeName: data.nodeName.trim(),
        ...(managementBearer ? { managementBearer } : {}),
      },
    },
    '节点复制失败',
  );
  return normalizeHubNode(result);
}

export async function setHubNodeStatus(nodeCode: string, status: HubNodeStatusValue) {
  return send<unknown>(
    `${nodePath(nodeCode)}/status`,
    { method: 'PUT', data: { status } },
    '节点状态更新失败',
  );
}

export async function testHubNodeConnection(nodeCode: string): Promise<HubNodeHealth> {
  const data = await send<unknown>(
    `${nodePath(nodeCode)}/connection-test`,
    { method: 'POST', data: {} },
    '节点连接测试失败',
  );
  return normalizeHubNodeHealth(data);
}

export async function listHubNodeUsers(
  nodeCode: string,
  params: HubUserPageQuery,
): Promise<HubUserPage> {
  const data = await send<unknown>(
    `${nodePath(nodeCode)}/users`,
    { method: 'GET', params },
    'Node 用户加载失败',
  );
  return normalizeHubUserPage(data);
}

export async function getHubNodeUser(nodeCode: string, userId: string): Promise<HubUserRecord> {
  const data = await send<unknown>(userPath(nodeCode, userId), { method: 'GET' }, '用户详情加载失败');
  return normalizeHubUser(data);
}

export async function setHubNodeUserStatus(
  nodeCode: string,
  userId: string,
  data: UserStatusCommand,
) {
  const descriptor = await buildUserStatusRequest(nodeCode, userId, data);
  return send<unknown>(descriptor.url, descriptor, '用户状态更新失败');
}

export async function listHubNodeSessions(
  nodeCode: string,
  userId: string,
  params: HubSessionPageQuery,
): Promise<HubSessionPage> {
  const data = await send<unknown>(
    `${userPath(nodeCode, userId)}/sessions`,
    { method: 'GET', params },
    '用户会话加载失败',
  );
  return normalizeHubSessionPage(data);
}

export async function revokeHubNodeSessions(
  nodeCode: string,
  userId: string,
  data: SessionRevokeCommand,
) {
  const descriptor = await buildSessionRevokeRequest(nodeCode, userId, data);
  return send<unknown>(descriptor.url, descriptor, '用户会话撤销失败');
}

export async function getHubNodeLoginPolicy(nodeCode: string): Promise<HubLoginPolicy> {
  const data = await send<unknown>(
    `${nodePath(nodeCode)}/login-policy`,
    { method: 'GET' },
    '登录策略加载失败',
  );
  return normalizeHubLoginPolicy(data);
}

export async function applyHubNodeLoginPolicy(
  nodeCode: string,
  data: HubLoginPolicyApplyRequest,
) {
  const descriptor = await buildReasonedRequest(`${nodePath(nodeCode)}/login-policy/apply`, data);
  return send<unknown>(descriptor.url, descriptor, '登录策略应用失败');
}

export async function getHubNodeFederation(nodeCode: string): Promise<HubFederationStatus> {
  const data = await send<unknown>(
    `${nodePath(nodeCode)}/federation`,
    { method: 'GET' },
    '联邦连接状态加载失败',
  );
  return normalizeHubFederationStatus(data);
}

export async function provisionHubNodeFederation(
  nodeCode: string,
  data: HubFederationProvisionRequest,
) {
  const descriptor = await buildReasonedRequest(`${nodePath(nodeCode)}/federation/provision`, data);
  return send<unknown>(descriptor.url, descriptor, '联邦连接编排失败');
}
