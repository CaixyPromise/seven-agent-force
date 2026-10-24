import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type DataEnvelope<T> = { data: T };

export type OpenIntegrationStatus = 'ENABLED' | 'DISABLED';
export type WebhookDeliveryStatus = 'PENDING' | 'SENDING' | 'RETRY_WAIT' | 'SUSPENDED' | 'DELIVERED' | 'EXHAUSTED';

export interface IntegrationCredentialSummary {
  credentialId: API.Int64;
  version: number;
  hint: string;
}

export interface IntegrationCredentialGrace {
  credentialId: API.Int64;
  version: number;
  validUntil: string;
}

export interface OpenIntegrationApplication {
  id: API.Int64;
  name: string;
  webhookUrl: string;
  status: OpenIntegrationStatus;
  apiCredential: IntegrationCredentialSummary;
  signingCredential?: IntegrationCredentialSummary;
  apiKeyGrace?: IntegrationCredentialGrace;
  signingSecretGrace?: IntegrationCredentialGrace;
  latestDeliveryStatus?: WebhookDeliveryStatus;
  createdAt: string;
  updatedAt: string;
  lastUsedAt?: string;
}

export interface CreatedOpenIntegration {
  application: OpenIntegrationApplication;
  apiKey: string;
  signingSecret?: string;
  apiCredentialId: API.Int64;
  apiCredentialVersion: number;
  signingCredentialId?: API.Int64;
  signingCredentialVersion?: number;
}

export interface APIKeyRotationResult {
  applicationId: API.Int64;
  credentialId: API.Int64;
  credentialVersion: number;
  apiKey: string;
  credentialHint: string;
  previousCredentialId: API.Int64;
  previousValidUntil: string;
  createdAt: string;
}

export interface SigningSecretRotationResult {
  applicationId: API.Int64;
  signingCredentialId: API.Int64;
  signingCredentialVersion: number;
  signingSecret: string;
  signingSecretHint: string;
  previousCredentialId: API.Int64;
  previousValidUntil: string;
  createdAt: string;
}

export interface WebhookDelivery {
  deliveryId: API.Int64;
  eventId: API.Int64;
  taskId: API.Int64;
  eventType: string;
  status: WebhookDeliveryStatus;
  attemptCount: number;
  lastHttpStatus?: number;
  nextAttemptAt?: string;
  updatedAt: string;
}

export async function listOpenIntegrations(): Promise<OpenIntegrationApplication[]> {
  const response = await request<DataEnvelope<OpenIntegrationApplication[]>>('/api/workforce/open-integrations', { method: 'GET' });
  return response.data ?? [];
}

export async function createOpenIntegration(input: { name: string; webhookUrl?: string }): Promise<CreatedOpenIntegration> {
  const response = await request<DataEnvelope<CreatedOpenIntegration>>('/api/workforce/open-integrations', { method: 'POST', data: input });
  return response.data;
}

export async function setOpenIntegrationEnabled(id: API.Int64, enabled: boolean): Promise<void> {
  await request<DataEnvelope<{ status: OpenIntegrationStatus }>>(`/api/workforce/open-integrations/${encodeURIComponent(toApiIdParam(id))}/${enabled ? 'enable' : 'disable'}`, { method: 'POST' });
}

export async function listWebhookDeliveries(id: API.Int64): Promise<WebhookDelivery[]> {
  const response = await request<DataEnvelope<WebhookDelivery[]>>(`/api/workforce/open-integrations/${encodeURIComponent(toApiIdParam(id))}/deliveries`, { method: 'GET', params: { limit: 50 } });
  return response.data ?? [];
}

export async function rotateOpenIntegrationAPIKey(id: API.Int64): Promise<APIKeyRotationResult> {
  const response = await request<DataEnvelope<APIKeyRotationResult>>(`/api/workforce/open-integrations/${encodeURIComponent(toApiIdParam(id))}/api-key/rotate`, { method: 'POST' });
  return response.data;
}

export async function rotateOpenIntegrationSigningSecret(id: API.Int64): Promise<SigningSecretRotationResult> {
  const response = await request<DataEnvelope<SigningSecretRotationResult>>(`/api/workforce/open-integrations/${encodeURIComponent(toApiIdParam(id))}/signing-secret/rotate`, { method: 'POST' });
  return response.data;
}
