import { request } from './request';
import { toApiIdParam } from '@/lib/apiId';

type Envelope<T> = { data: T };

export type HookRuntime = 'shell' | 'node';
export type HookEvent = 'attempt.prepare' | 'attempt.propose' | 'attempt.end' | 'codex.PreToolUse' | 'codex.PostToolUse';

export interface HookSourceFile { path: string; content: string; mode?: number }
export interface HookEntrypoint { event: HookEvent; path: string; timeoutMs: number; failureMode: 'BLOCK' | 'OBSERVE' }
export interface HookManifest {
  schemaVersion: 'seven.hook-package/v1';
  name: string;
  version?: string;
  trustClass: 'PLATFORM_TRUSTED';
  runtime: { kind: HookRuntime; requirement?: string };
  entrypoints: HookEntrypoint[];
  limits: { stdinBytes: number; stdoutBytes: number; stderrBytes: number };
}
export interface HookPackage {
  id: API.Int64; name: string; category: string; description: string; trustClass: string;
  revisionId: API.Int64; revision: number; revisionStatus: 'DRAFT' | 'PUBLISHED'; manifest: HookManifest; files: HookSourceFile[];
  validation?: { valid: boolean; errors: string[]; warnings: string[] };
  latestVersion?: string; updatedAt: string;
}
export interface HookRelease {
  id: API.Int64; packageId: API.Int64; version: string; runtimeKind: HookRuntime;
  manifest: HookManifest; files: HookSourceFile[]; archiveSha256: string; archiveSizeBytes: number; publishedAt: string;
}
export interface HarnessChannel {
  alias: string;
  harnessReleaseId: API.Int64;
  harnessType: string;
  harnessVersion: string;
  imageDigest: string;
  supervisorVersion: string;
}

export const listHookPackages = async () => (await request<Envelope<HookPackage[]>>('/api/workforce/hook-packages', { method: 'GET' })).data ?? [];
export const getHookPackage = async (id: API.Int64) => (await request<Envelope<HookPackage>>(`/api/workforce/hook-packages/${encodeURIComponent(toApiIdParam(id))}`, { method: 'GET' })).data;
export const createHookRevision = async (id: API.Int64) => (await request<Envelope<HookPackage>>(`/api/workforce/hook-packages/${encodeURIComponent(toApiIdParam(id))}/revisions`, { method: 'POST' })).data;
export const createHookPackage = async (input: { name: string; category: string; description?: string; runtime: HookRuntime }) => {
  const path = input.runtime === 'shell' ? 'hooks/attempt-prepare.sh' : 'hooks/pre-tool-use.mjs';
  const event: HookEvent = input.runtime === 'shell' ? 'attempt.prepare' : 'codex.PreToolUse';
  const content = input.runtime === 'shell'
    ? '#!/bin/sh\nprintf \'%s\\n\' \'{"schemaVersion":1,"decision":"ALLOW","reasonCode":"OK","operatorMessage":"allowed","evidence":[]}\'\n'
    : 'let input = "";\nfor await (const chunk of process.stdin) input += chunk;\nconsole.log(JSON.stringify({ schemaVersion: 1, decision: "ALLOW", reasonCode: "OK", operatorMessage: "allowed", evidence: [] }));\n';
  const manifest: HookManifest = { schemaVersion: 'seven.hook-package/v1', name: input.name, trustClass: 'PLATFORM_TRUSTED', runtime: { kind: input.runtime, requirement: input.runtime === 'node' ? '>=22 <23' : '' }, entrypoints: [{ event, path, timeoutMs: 5000, failureMode: 'BLOCK' }], limits: { stdinBytes: 1048576, stdoutBytes: 1048576, stderrBytes: 524288 } };
  return (await request<Envelope<HookPackage>>('/api/workforce/hook-packages', { method: 'POST', data: { ...input, manifest, files: [{ path, content, mode: 420 }] } })).data;
};
export const saveHookRevision = async (item: HookPackage) => (await request<Envelope<HookPackage>>(`/api/workforce/hook-revisions/${encodeURIComponent(toApiIdParam(item.revisionId))}`, { method: 'PUT', data: { expectedRevision: item.revision, manifest: item.manifest, files: item.files } })).data;
export const importHookArchive = async (item: HookPackage, file: File) => (await request<Envelope<HookPackage>>(`/api/workforce/hook-revisions/${encodeURIComponent(toApiIdParam(item.revisionId))}/archive-import`, { method: 'POST', params: { expectedRevision: item.revision }, headers: { 'Content-Type': 'application/gzip' }, data: file })).data;
export const validateHookRevision = async (revisionId: API.Int64) => (await request<Envelope<{ valid: boolean; errors: string[]; warnings: string[] }>>(`/api/workforce/hook-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/validate`, { method: 'POST' })).data;
export const testHookRevision = async (revisionId: API.Int64, event: HookEvent, fixture: Record<string, unknown>) => (await request<Envelope<{ id: API.Int64; decision: 'ALLOW' | 'DENY'; reasonCode: string; operatorMessage: string; diagnostics: string; durationMs: number }>>(`/api/workforce/hook-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/test-runs`, { method: 'POST', data: { event, fixture } })).data;
export const publishHookRevision = async (revisionId: API.Int64, version: string) => (await request<Envelope<HookRelease>>(`/api/workforce/hook-revisions/${encodeURIComponent(toApiIdParam(revisionId))}/publish`, { method: 'POST', data: { version } })).data;
export const listHookReleases = async () => (await request<Envelope<HookRelease[]>>('/api/workforce/hook-package-releases', { method: 'GET' })).data ?? [];
/** Used only by the Result Processor fixture runner; Projects and Hooks do not bind a Harness. */
export const listHarnessChannels = async () => (await request<Envelope<HarnessChannel[]>>('/api/workforce/harness-channels', { method: 'GET' })).data ?? [];
