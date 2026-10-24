import { request } from '@/api/request';
import type { ApiResponse } from '@/lib/http/types';

const BASE = '/api/notification';

export type NotificationChannelType =
  | 'MOCK'
  | 'EMAIL'
  | 'FEISHU'
  | 'WECOM'
  | 'DINGTALK'
  | 'WEBHOOK'
  | 'FEISHU_APP'
  | 'WECOM_APP'
  | 'FEISHU_WEBHOOK'
  | 'WECOM_WEBHOOK'
  | 'HTTP_CONNECTOR';
export type NotificationDeliveryStatus =
  | 'PENDING'
  | 'SENDING'
  | 'SENT'
  | 'PROVIDER_ACCEPTED'
  | 'UNKNOWN'
  | 'FAILED'
  | 'CANCELED';

export interface NotificationGroupRoute {
  id: API.Int64;
  name: string;
  provider: 'FEISHU' | 'WECOM';
  transport: 'FEISHU_APP_CHAT' | 'FEISHU_WEBHOOK' | 'WECOM_WEBHOOK';
  requiredPermissionCode: string;
  status: 'DRAFT' | 'VERIFIED' | 'ENABLED' | 'DISABLED' | 'RETIRED';
  version: API.Int64;
  lastProbeStatus?: string;
  lastProbeReasonCode?: string;
  lastProbeAt?: string;
  referencedTaskCount: API.Int64;
  createdAt: string;
  updatedAt: string;
}

export interface NotificationGroupRouteImpact {
  activeTasks: API.Int64;
  resumableTasks: API.Int64;
  subscriptions: API.Int64;
  pendingRecords: API.Int64;
  pendingDeliveries: API.Int64;
  historicalReferences: API.Int64;
  canDelete: boolean;
}
export interface NotificationGroupCandidate { candidateToken: string; displayName: string; description?: string }

export type WorkNotificationEventFamily = 'TASK_ACTION_REQUIRED' | 'TASK_INTERRUPTED' | 'TASK_COMPLETED';
export type WorkNotificationChannelKind = 'IN_APP' | 'EMAIL_USER' | 'FEISHU_DIRECT' | 'WECOM_DIRECT';
export interface WorkNotificationPreference {
  id: API.Int64;
  organizationId: API.Int64;
  userId: API.Int64;
  eventFamily: WorkNotificationEventFamily;
  channelKind: WorkNotificationChannelKind;
  enabled: boolean;
  version: API.Int64;
}

export interface PageResult<T> {
  records: T[];
  total: number;
  current: number;
  pageSize: number;
}

export interface NotificationChannel {
  id?: API.Int64;
  channelCode: string;
  channelName: string;
  channelType: NotificationChannelType;
  status: number;
  priority: number;
  configJson?: string;
  secretPlain?: string;
  rateLimitJson?: string;
  metadataJson?: string;
  providerConfig?: ProviderChannelConfig;
  httpConnectorConfig?: HTTPConnectorConfig;
  webhookProfileConfig?: WebhookProfileConfig;
  /** Write-only fixed group webhook URL. It is never returned by the API. */
  webhookUrl?: string;
  /** Write-only Feishu signing secret. It is never returned by the API. */
  webhookSigningSecret?: string;
  providerParameterCatalog?: ProviderParameterDescriptor[];
  providerParameterSettings?: ProviderParameterSetting[];
  secretConfigured?: boolean;
  createTime?: string;
  updateTime?: string;
}

export interface ProviderChannelConfig {
  feishuAppId?: string;
  weComCorpId?: string;
  weComAgentId?: string;
}

/** A static connection prepared by an administrator. It is not a caller-owned webhook request. */
export interface HTTPConnectorConfig {
  endpointUrl: string;
  egressPolicyRef?: string;
  method: 'POST';
  authenticationMode: 'NONE' | 'BEARER' | 'BASIC' | 'HMAC_SHA256';
  fieldMappings: HTTPConnectorFieldMapping[];
  headerAllowlist?: string[];
  idempotencyHeader: 'Idempotency-Key';
  timeoutMilliseconds: number;
  successStatusCodes?: number[];
}

/** Maps one approved notification value to a top-level request property. */
export interface HTTPConnectorFieldMapping {
  source: 'SUBJECT' | 'TEXT' | 'EVENT_KEY' | 'CATEGORY' | 'PRIORITY' | 'TRACE_ID' | 'DEEP_LINK' | 'ACTION_URL';
  target: string;
}

/** Non-secret fixed-profile settings for Feishu and WeCom group robots. */
export interface WebhookProfileConfig {
  timeoutMilliseconds: number;
  successStatusCodes?: number[];
}

export interface ProviderParameterDescriptor {
  key: string;
  label: string;
  valueType: string;
  maxItems?: number;
  maxValueBytes?: number;
  allowDefault: boolean;
}

export interface ProviderParameterSetting {
  key: string;
  enabled: boolean;
  defaultValue?: string[];
}

export type TemplateRevisionState = 'DRAFT' | 'PUBLISHED' | 'SUPERSEDED';
export type TemplateVariableType = 'STRING' | 'NUMBER' | 'BOOLEAN' | 'DATETIME';
export type TemplateVariableClassification = 'PUBLIC' | 'SENSITIVE';

/** Structured authoring row for the G6.1 versioned template workspace. */
export interface TemplateRevisionVariable {
  name: string;
  type: TemplateVariableType;
  required: boolean;
  maxLength?: number;
  sampleValue?: string | number | boolean;
  classification: TemplateVariableClassification;
}

/** This shape intentionally contains no channel, route, target, or raw JSON field. */
export interface TemplateRevisionDraftInput {
  templateName: string;
  locale: string;
  subjectTemplate?: string;
  textTemplate?: string;
  htmlTemplate?: string;
  markdownTemplate?: string;
  primaryActionLabelTemplate?: string;
  variables: TemplateRevisionVariable[];
}

export interface TemplateRevision {
  id: API.Int64;
  revisionNo: number;
  state: TemplateRevisionState;
  revisionVersion: number;
  subjectTemplate?: string;
  textTemplate?: string;
  htmlTemplate?: string;
  markdownTemplate?: string;
  primaryActionLabelTemplate?: string;
  variables: TemplateRevisionVariable[];
  contentDigest?: string;
  publishedAt?: string;
  publishedBy?: API.Int64;
  createTime?: string;
  updateTime?: string;
}

/** Additive G6.1 definition; it is not selectable by the current delivery path. */
export interface VersionedNotificationTemplate {
  id: API.Int64;
  templateCode: string;
  templateName: string;
  locale: string;
  currentDraft?: TemplateRevision;
  currentPublished?: TemplateRevision;
  /** Detail/mutation responses retain every read-only historical revision. */
  revisions?: TemplateRevision[];
  version: number;
  createTime?: string;
  updateTime?: string;
}

export interface TemplateDefinitionCreateRequest {
  templateCode: string;
  draft: TemplateRevisionDraftInput;
}

export interface TemplateRevisionSaveRequest {
  expectedVersion: number;
  draft: TemplateRevisionDraftInput;
}

export interface TemplateRevisionPreviewRequest {
  draft: TemplateRevisionDraftInput;
  variables: Record<string, unknown>;
}

export interface TemplateRevisionPreviewResponse {
  subject: string;
  text: string;
  html: string;
  markdown: string;
  primaryActionLabel?: string;
}

export type SceneRevisionState = 'DRAFT' | 'PUBLISHED' | 'SUPERSEDED';
export type SceneReceiverKind =
  | 'IN_APP'
  | 'EMAIL_USER'
  | 'FEISHU_OPEN_ID'
  | 'FEISHU_CHAT_ID'
  | 'WECOM_USERID'
  | 'FEISHU_WEBHOOK_GROUP'
  | 'WECOM_WEBHOOK_GROUP'
  | 'HTTP_CONNECTOR_FIXED'
  | 'FIXED_CONNECTION';

/** One immutable scene revision chooses one published template and one sending way. */
export interface SceneRevision {
  id: API.Int64;
  revisionNo: number;
  state: SceneRevisionState;
  revisionVersion: number;
  enabled: boolean;
  templateRevisionId: API.Int64;
  connectionRef?: string;
  sendingWay: string;
  publishedAt?: string;
  publishedBy?: API.Int64;
  createTime?: string;
  updateTime?: string;
}

export interface VersionedNotificationScene {
  id: API.Int64;
  sceneCode: string;
  sceneName: string;
  receiverKind: SceneReceiverKind;
  currentDraft?: SceneRevision;
  currentPublished?: SceneRevision;
  revisions?: SceneRevision[];
  version: number;
  createTime?: string;
  updateTime?: string;
}

/** No target, group, URL, credential, route list, or fallback belongs here. */
export interface SceneRevisionDraftInput {
  sceneName: string;
  receiverKind: SceneReceiverKind;
  templateRevisionId: API.Int64;
  connectionRef?: string;
  enabled: boolean;
}

export interface SceneDefinitionCreateRequest {
  sceneCode: string;
  draft: SceneRevisionDraftInput;
}

export interface SceneRevisionSaveRequest {
  expectedVersion: number;
  draft: SceneRevisionDraftInput;
}

export interface NotificationDelivery {
  id?: API.Int64;
  deliveryId: string;
  sceneCode: string;
  channelCode: string;
  channelType: NotificationChannelType;
  templateCode: string;
  targetMasked?: string;
  status: NotificationDeliveryStatus;
  retryCount: number;
  maxRetry: number;
  nextRetryAt?: string;
  /** A controlled, non-provider-specific failure class for the normal list. */
  failureCode?: string;
  /** A short controlled failure hint for the normal list. */
  failureMessage?: string;
  traceId?: string;
  sentAt?: string;
  createTime?: string;
  updateTime?: string;
}

export type DeliveryDiagnosticReason = 'INCIDENT' | 'CUSTOMER_SUPPORT' | 'SECURITY_REVIEW' | 'OTHER';

/** Required context for one accountable content read; never accepts content or targets. */
export interface DeliveryDiagnosticContentRequest {
  reasonCode: DeliveryDiagnosticReason;
  ticketReference?: string;
}

/** Plain text only. This response is intentionally never placed in a query cache. */
export interface DeliveryDiagnosticContent {
  deliveryId: string;
  contentTier: 'PUBLIC' | 'SENSITIVE' | 'SECRET_EPHEMERAL';
  subject?: string;
  text?: string;
  expiresAt?: string;
}

export interface NotificationQuery {
  keyword?: string;
  sceneCode?: string;
  channelCode?: string;
  channelType?: string;
  status?: string | number;
  enabled?: boolean;
  current?: number;
  pageSize?: number;
}

export interface EnterpriseConnectionTestRequest {
  connectionRef: string;
  identityKind: 'FEISHU_OPEN_ID' | 'FEISHU_CHAT_ID' | 'WECOM_USERID';
  subject: string;
  providerParams?: Record<string, unknown>;
  text?: string;
}

export interface EnterpriseConnectionTestResult {
  status: string;
  failureClass?: string;
  providerReference?: string;
  diagnostic?: string;
  providerError?: {
    provider: string;
    httpStatus?: number;
    code?: string;
    message?: string;
    logId?: string;
  };
  warnings?: Array<{ provider: string; key: string; reason: string }>;
}

/** A non-persistent probe of one saved HTTP Connector or fixed group webhook. */
export interface StaticConnectionTestRequest {
  connectionRef: string;
  text?: string;
}

export interface StaticConnectionTestResult {
  status: string;
  failureClass?: string;
  providerReference?: string;
  diagnostic?: string;
  providerError?: {
    provider: string;
    httpStatus?: number;
    code?: string;
    message?: string;
    logId?: string;
  };
}

export interface NotificationReadinessCapability {
  available: boolean;
  recipientReady?: boolean;
  directReady?: boolean;
  groupReady?: boolean;
  boundRouteName?: string;
  reasonCode?: string;
}

export interface NotificationReadiness {
  sceneCode: string;
  inApp: NotificationReadinessCapability;
  email: NotificationReadinessCapability;
  feishu: NotificationReadinessCapability;
  wecom: NotificationReadinessCapability;
  taskLink: NotificationReadinessCapability;
  action: { configured: boolean };
}

function unwrap<T>(response: ApiResponse<T>, fallback: string): T {
  if (!response || typeof response !== 'object' || !('code' in response)) {
    return response as T;
  }
  if (response.code !== 0 && response.code !== 200) {
    throw new Error(response.message || fallback);
  }
  return response.data as T;
}

export async function listNotificationChannels(params: NotificationQuery) {
  return unwrap(
    await request<ApiResponse<PageResult<NotificationChannel>>>(`${BASE}/channels`, { params }),
    '查询通知渠道失败',
  );
}

export async function saveNotificationChannel(body: NotificationChannel) {
  return unwrap(
    await request<ApiResponse<NotificationChannel>>(`${BASE}/channels`, { method: 'POST', data: body }),
    '保存通知渠道失败',
  );
}

export async function listVersionedNotificationTemplates(params: NotificationQuery) {
  return unwrap(
    await request<ApiResponse<PageResult<VersionedNotificationTemplate>>>(`${BASE}/template-definitions`, { params }),
    '查询版本化通知模板失败',
  );
}

export async function getVersionedNotificationTemplate(templateCode: string) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationTemplate>>(`${BASE}/template-definitions/${encodeURIComponent(templateCode)}`),
    '查询版本化通知模板详情失败',
  );
}

export async function createVersionedNotificationTemplate(body: TemplateDefinitionCreateRequest) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationTemplate>>(`${BASE}/template-definitions`, { method: 'POST', data: body }),
    '创建版本化通知模板失败',
  );
}

export async function saveVersionedNotificationTemplateDraft(revisionId: API.Int64, body: TemplateRevisionSaveRequest) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationTemplate>>(`${BASE}/template-revisions/${encodeURIComponent(String(revisionId))}`, {
      method: 'POST',
      data: body,
    }),
    '保存模板草稿失败',
  );
}

export async function createVersionedNotificationTemplateDraft(templateCode: string) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationTemplate>>(`${BASE}/template-definitions/${encodeURIComponent(templateCode)}/drafts`, {
      method: 'POST',
      data: {},
    }),
    '新建模板版本失败',
  );
}

export async function previewVersionedNotificationTemplate(body: TemplateRevisionPreviewRequest) {
  return unwrap(
    await request<ApiResponse<TemplateRevisionPreviewResponse>>(`${BASE}/template-revisions/preview`, { method: 'POST', data: body }),
    '预览版本化通知模板失败',
  );
}

export async function publishVersionedNotificationTemplate(revisionId: API.Int64, expectedVersion: number) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationTemplate>>(`${BASE}/template-revisions/${encodeURIComponent(String(revisionId))}/publish`, {
      method: 'POST',
      data: { expectedVersion },
    }),
    '发布模板失败',
  );
}

export async function listVersionedNotificationScenes(params: NotificationQuery) {
  return unwrap(
    await request<ApiResponse<PageResult<VersionedNotificationScene>>>(`${BASE}/scene-definitions`, { params }),
    '查询新版通知场景失败',
  );
}

export async function getVersionedNotificationScene(sceneCode: string, receiverKind: SceneReceiverKind) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-definitions/${encodeURIComponent(sceneCode)}`, {
      params: { receiverKind },
    }),
    '查询新版通知场景详情失败',
  );
}

export async function createVersionedNotificationScene(body: SceneDefinitionCreateRequest) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-definitions`, { method: 'POST', data: body }),
    '创建新版通知场景失败',
  );
}

export async function saveVersionedNotificationSceneDraft(revisionId: API.Int64, body: SceneRevisionSaveRequest) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-revisions/${encodeURIComponent(String(revisionId))}`, {
      method: 'POST',
      data: body,
    }),
    '保存场景草稿失败',
  );
}

export async function createVersionedNotificationSceneDraft(sceneCode: string, receiverKind: SceneReceiverKind) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-definitions/${encodeURIComponent(sceneCode)}/drafts`, {
      method: 'POST',
      params: { receiverKind },
      data: {},
    }),
    '新建场景版本失败',
  );
}

export async function publishVersionedNotificationScene(revisionId: API.Int64, expectedVersion: number) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-revisions/${encodeURIComponent(String(revisionId))}/publish`, {
      method: 'POST',
      data: { expectedVersion },
    }),
    '发布场景失败',
  );
}

export async function stopVersionedNotificationScene(sceneCode: string, receiverKind: SceneReceiverKind) {
  return unwrap(
    await request<ApiResponse<VersionedNotificationScene>>(`${BASE}/scene-definitions/${encodeURIComponent(sceneCode)}/stop`, {
      method: 'POST',
      params: { receiverKind },
      data: {},
    }),
    '停用场景失败',
  );
}

export async function listNotificationDeliveries(params: NotificationQuery) {
  return unwrap(
    await request<ApiResponse<PageResult<NotificationDelivery>>>(`${BASE}/deliveries`, { params }),
    '查询通知投递日志失败',
  );
}

// This is deliberately a direct request rather than a query/mutation cache.
// The server also returns no-store headers; the caller keeps the value only in
// the open modal and clears it when the modal or account changes.
export async function readNotificationDeliveryDiagnosticContent(
  deliveryId: string,
  body: DeliveryDiagnosticContentRequest,
) {
  return unwrap(
    await request<ApiResponse<DeliveryDiagnosticContent>>(
      `${BASE}/deliveries/${encodeURIComponent(deliveryId)}/diagnostic-content`,
      {
        method: 'POST',
        data: body,
        headers: {
          'Cache-Control': 'no-store',
          Pragma: 'no-cache',
        },
      },
    ),
    '读取投递内容失败',
  );
}

export async function testEnterpriseConnection(body: EnterpriseConnectionTestRequest) {
  return unwrap(
    await request<ApiResponse<EnterpriseConnectionTestResult>>(`${BASE}/channels/test-connection`, { method: 'POST', data: body }),
    '测试企业应用连接失败',
  );
}

export async function testStaticConnection(body: StaticConnectionTestRequest) {
  return unwrap(
    await request<ApiResponse<StaticConnectionTestResult>>(`${BASE}/channels/test-static-connection`, { method: 'POST', data: body }),
    '测试受控 HTTP 连接失败',
  );
}

export async function getNotificationReadiness(sceneCode: string) {
  return unwrap(
    await request<ApiResponse<NotificationReadiness>>(`${BASE}/readiness`, { params: { sceneCode } }),
    '检查通知可用性失败',
  );
}

export async function listNotificationGroupRoutes() {
  return unwrap(await request<ApiResponse<NotificationGroupRoute[]>>(`${BASE}/group-routes`), '加载通知群失败');
}

export async function saveNotificationGroupRoute(input: {
  id?: API.Int64;
  name: string;
  provider: 'FEISHU' | 'WECOM';
  transport: 'FEISHU_APP_CHAT' | 'FEISHU_WEBHOOK' | 'WECOM_WEBHOOK';
  requiredPermissionCode: string;
  webhookUrl?: string;
  webhookSigningSecret?: string;
  enterpriseConnectionId?: API.Int64;
  candidateToken?: string;
  expectedVersion?: API.Int64;
}) {
  const path = input.id ? `${BASE}/group-routes/${encodeURIComponent(input.id)}` : `${BASE}/group-routes`;
  return unwrap(await request<ApiResponse<NotificationGroupRoute>>(path, {
    method: input.id ? 'PUT' : 'POST',
    data: input,
  }), '保存通知群失败');
}

export async function listFeishuGroupCandidates(connectionId: API.Int64) {
  return unwrap(await request<ApiResponse<NotificationGroupCandidate[]>>(`${BASE}/group-routes/feishu-group-candidates`, { params: { connectionId } }), '加载飞书群失败');
}

export async function changeNotificationGroupRoute(route: NotificationGroupRoute, action: 'probe' | 'enable' | 'disable' | 'retire') {
  return unwrap(await request<ApiResponse<NotificationGroupRoute>>(`${BASE}/group-routes/${encodeURIComponent(route.id)}/${action}`, {
    method: 'POST', data: { expectedVersion: route.version },
  }), '更新通知群失败');
}

export async function getNotificationGroupRouteImpact(routeId: API.Int64) {
  return unwrap(await request<ApiResponse<NotificationGroupRouteImpact>>(`${BASE}/group-routes/${encodeURIComponent(routeId)}/impact`), '检查通知群影响失败');
}

export async function deleteNotificationGroupRoute(routeId: API.Int64) {
  return unwrap(await request<ApiResponse<{ deleted: boolean }>>(`${BASE}/group-routes/${encodeURIComponent(routeId)}`, { method: 'DELETE' }), '删除通知群失败');
}

export async function listMyWorkNotificationPreferences() {
  return unwrap(await request<ApiResponse<WorkNotificationPreference[]>>(`${BASE}/me/work-preferences`), '加载工作通知设置失败');
}

export async function updateMyWorkNotificationPreference(eventFamily: WorkNotificationEventFamily, channelKind: WorkNotificationChannelKind, enabled: boolean, expectedVersion: API.Int64) {
  return unwrap(await request<ApiResponse<WorkNotificationPreference>>(`${BASE}/me/work-preferences/${eventFamily}/${channelKind}`, { method: 'PUT', data: { enabled, expectedVersion } }), '保存工作通知设置失败');
}
