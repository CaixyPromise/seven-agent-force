import { completionCheckId, completionCheckText } from './completionCheckContract';
import type { WorkforceArtifact } from './workforceController';

function artifactSize(value: unknown): number {
  // The public serializer emits int64 byte counts as decimal strings too.
  // Convert only this bounded quantity; entity identities remain strings.
  if (typeof value === 'string') {
    if (!/^(0|[1-9]\d*)$/.test(value) || value.length > 16 || (value.length === 16 && value > '9007199254740991')) throw new TypeError('INVALID_TASK_ARTIFACTS');
    value = Number(value);
  }
  if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) throw new TypeError('INVALID_TASK_ARTIFACTS');
  return value;
}

export function normalizeTaskArtifacts(value: unknown, taskId: API.Int64): WorkforceArtifact[] {
  completionCheckId(taskId);
  if (!Array.isArray(value) || value.length > 1000) throw new TypeError('INVALID_TASK_ARTIFACTS');
  const items = value.map((value): WorkforceArtifact => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new TypeError('INVALID_TASK_ARTIFACTS');
    const r = value as Record<string, unknown>;
    const id = completionCheckId(r.id);
    if (r.taskId !== taskId || typeof r.sha256 !== 'string' || !/^[a-f0-9]{64}$/.test(r.sha256)) throw new TypeError('INVALID_TASK_ARTIFACTS');
    const sizeBytes = artifactSize(r.sizeBytes);
    return {
      id, taskId, attemptId: completionCheckId(r.attemptId),
      type: completionCheckText(r.type, 'artifact', 128, true)!,
      name: completionCheckText(r.name, 'artifact', 4096, true)!,
      sha256: r.sha256, sizeBytes,
      mediaType: completionCheckText(r.mediaType, 'artifact', 256, true)!,
      status: completionCheckText(r.status, 'artifact', 64, true)!,
      // Downloads always use the authenticated same-origin artifact endpoint.
      downloadUrl: `/api/workforce/tasks/${taskId}/artifacts/${id}/download`,
    };
  });
  if (new Set(items.map((item) => item.id)).size !== items.length) throw new TypeError('INVALID_TASK_ARTIFACTS');
  return items;
}
