/**
 * 配置管理 API
 */
import { request } from './request';
import type {
  ConfigGroup,
  ConfigItem,
  CreateConfigGroupRequest,
  UpdateConfigGroupRequest,
  ConfigGroupQuery,
  CreateConfigRequest,
  UpdateConfigRequest,
  ConfigQuery,
  ConfigPageResult,
  ConfigSensitiveRevealResponse,
  ConfigChangeLog,
  PendingConfig,
  ConfigScopeGrant,
} from '@/types/config';

/** POST /system/cache/refresh — protected, global application-owned cache refresh. */
export async function refreshSystemCache(): Promise<{ state?: string }> {
  const res = await request<API.Result<{ state?: string }>>('/api/system/cache/refresh', {
    method: 'POST',
  });
  return res.data ?? {};
}

/**
 * ========== 配置分组管理 (/config-groups) ==========
 */

/**
 * 分页查询配置分组列表
 * GET /api/config-groups/page
 */
export async function getConfigGroupPage(params: ConfigGroupQuery) {
  const res = await request<API.Result<API.PageResult<ConfigGroup>>>(`/api/config-groups/page`, {
    method: 'GET',
    params,
  });
  return res.data ?? { records: [], total: 0 };
}

/**
 * 获取所有配置分组 (不分页)
 * GET /api/config-groups/page (不传分页参数或size很大)
 */
export async function getAllConfigGroups(module?: string): Promise<ConfigGroup[]> {
  const res = await request<API.Result<API.PageResult<ConfigGroup>>>(`/api/config-groups/page`, {
    method: 'GET',
    params: {
      pageNum: 1,
      pageSize: 1000,
      ...(module ? { module } : {})
    },
  });
  const page = res.data;
  return page?.list ?? page?.records ?? [];
}

/**
 * 获取配置分组详情
 * GET /api/config-groups/{id}
 */
export async function getConfigGroupById(id: API.Int64) {
  return request<API.Result<ConfigGroup>>(`/api/config-groups/${id}`, {
    method: 'GET',
  });
}

/**
 * 创建配置分组
 * POST /api/config-groups
 */
export async function createConfigGroup(data: CreateConfigGroupRequest) {
  return request<API.Result<API.Int64>>(`/api/config-groups`, {
    method: 'POST',
    data,
  });
}

/**
 * 更新配置分组
 * POST /api/config-groups/update
 */
export async function updateConfigGroup(data: UpdateConfigGroupRequest) {
  return request<API.Result<boolean>>(`/api/config-groups/update`, {
    method: 'POST',
    data,
  });
}

/**
 * 删除配置分组
 * POST /api/config-groups/delete
 */
export async function deleteConfigGroup(id: API.Int64) {
  return request<API.Result<boolean>>(`/api/config-groups/delete`, {
    method: 'POST',
    params: { id },
  });
}

/**
 * 移动配置分组到指定位置
 * POST /api/config-groups/{id}/move
 * 按照拖拽排序规范，使用 beforeId/afterId 描述目标位置
 */
export async function moveConfigGroup(
  id: API.Int64,
  beforeId?: API.Int64 | null,
  afterId?: API.Int64 | null,
) {
  return request<API.Result<boolean>>(`/api/config-groups/${id}/move`, {
    method: 'POST',
    data: { beforeId, afterId },
  });
}

/**
 * ========== 配置项管理 (/config) ==========
 */

/**
 * 获取配置项列表 (分页)
 * GET /api/config
 */
export async function getConfigs(params: ConfigQuery): Promise<ConfigPageResult> {
  const keyword = params.searchText ?? params.keyword;
  const res = await request<API.Result<API.PageResult<ConfigItem>>>(`/api/config`, {
    method: 'GET',
    params: {
      pageNum: params.pageNum ?? 1,
      pageSize: params.pageSize ?? 20,
      keyword,
      ...params,
    },
  });
  const pageData = res.data;
  const records = pageData?.list ?? pageData?.records ?? [];
  const total = Number(pageData?.total ?? 0);
  const pageSize = Number(pageData?.size ?? params.pageSize ?? 20);
  return {
    records,
    total,
    pageNum: Number(pageData?.current ?? params.pageNum ?? 1),
    pageSize,
    pages: pageSize > 0 ? Math.ceil(total / pageSize) : 0,
  };
}

/**
 * 获取配置项详情
 * GET /api/config/{id}
 */
export async function getConfigById(id: API.Int64) {
  return request<API.Result<ConfigItem>>(`/api/config/${id}`, {
    method: 'GET',
  });
}

/**
 * 创建配置项
 * POST /api/config
 */
export async function createConfig(data: CreateConfigRequest) {
  return request<API.Result<API.Int64>>(`/api/config`, {
    method: 'POST',
    data,
  });
}

/**
 * 更新配置项
 * POST /api/config/update
 */
export async function updateConfig(data: UpdateConfigRequest) {
  return request<API.Result<boolean>>(`/api/config/update`, {
    method: 'POST',
    data,
  });
}

/**
 * 删除配置项
 * POST /api/config/delete
 */
export async function deleteConfig(id: API.Int64) {
  return request<API.Result<boolean>>(`/api/config/delete`, {
    method: 'POST',
    params: { id },
  });
}

/**
 * 修改配置项启用状态
 * POST /api/config/enabled
 */
export async function changeConfigEnabled(id: API.Int64, isEnabled: number) {
  return request<API.Result<boolean>>(`/api/config/enabled`, {
    method: 'POST',
    params: { id, isEnabled },
  });
}

/**
 * 敏感配置回显（返回前端公钥加密后的密文）
 * POST /api/config/{id}/sensitive/reveal
 */
export async function revealSensitiveConfigValue(
  id: API.Int64,
  obfuscatedClientPublicKey: string,
): Promise<ConfigSensitiveRevealResponse> {
  const res = await request<API.Result<ConfigSensitiveRevealResponse>>(`/api/config/${id}/sensitive/reveal`, {
    method: 'POST',
    data: { obfuscatedClientPublicKey },
  });
  const payload = res as API.Result<ConfigSensitiveRevealResponse>;
  if (payload?.data) {
    return payload.data;
  }
  return res as unknown as ConfigSensitiveRevealResponse;
}

/**
 * ========== 配置变更管理 ==========
 */

/**
 * 应用待生效配置
 * POST /api/config/apply-pending
 */
export async function applyPendingConfigs() {
  return request<API.Result<number>>(`/api/config/apply-pending`, {
    method: 'POST',
  });
}

/**
 * 查询待生效配置列表
 * GET /api/config/pending
 */
export async function getPendingConfigs() {
  const res = await request<API.Result<PendingConfig[]>>(`/api/config/pending`, {
    method: 'GET',
  });
  return res.data ?? [];
}

/**
 * 查询配置变更历史
 * GET /api/config/{configId}/history
 */
export async function getConfigChangeHistory(configId: API.Int64, limit: number = 20) {
  const res = await request<API.Result<ConfigChangeLog[]>>(`/api/config/${configId}/history`, {
    method: 'GET',
    params: { limit },
  });
  return res.data ?? [];
}

/**
 * 回滚配置变更
 * POST /api/config/rollback
 */
export async function rollbackConfigChange(logId: API.Int64, reason?: string) {
  return request<API.Result<boolean>>(`/api/config/rollback`, {
    method: 'POST',
    params: { logId, reason },
  });
}

/**
 * 查询操作链
 * GET /api/config/operation-chain/{logId}
 */
export async function getOperationChain(logId: API.Int64) {
  const res = await request<API.Result<ConfigChangeLog[]>>(`/api/config/operation-chain/${logId}`, {
    method: 'GET',
  });
  return res.data ?? [];
}

/**
 * 查询审计日志
 * GET /api/config/audit-logs
 */
export async function getAuditLogs(params: {
  configId?: API.Int64;
  operationType?: string;
  status?: string;
  startTime?: string;
  endTime?: string;
  limit?: number;
}) {
  const res = await request<API.Result<ConfigChangeLog[]>>(`/api/config/audit-logs`, {
    method: 'GET',
    params,
  });
  return res.data ?? [];
}

/**
 * 获取角色配置范围授权
 * GET /api/config-scopes/roles/{roleId}
 */
export async function getRoleConfigScopes(roleId: API.Int64): Promise<ConfigScopeGrant[]> {
  const res = await request<API.Result<ConfigScopeGrant[]>>(`/api/config-scopes/roles/${roleId}`, {
    method: 'GET',
  });
  return res.data ?? [];
}

/**
 * 保存角色配置范围授权
 * POST /api/config-scopes/roles/{roleId}
 */
export async function assignRoleConfigScopes(roleId: API.Int64, grants: ConfigScopeGrant[]) {
  return request<API.Result<boolean>>(`/api/config-scopes/roles/${roleId}`, {
    method: 'POST',
    data: { grants },
  });
}
