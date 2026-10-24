import { LocalStorageUtil } from '@/lib/utils/LocalStorageUtil';

const DEVICE_ID_STORAGE_KEY = 'seven_device_id';

function generateDeviceId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `device_${Date.now()}_${Math.random().toString(16).slice(2, 10)}`;
}

export function getOrCreateDeviceId(): string {
  const existing = LocalStorageUtil.getItem(DEVICE_ID_STORAGE_KEY);
  if (existing) {
    return existing;
  }
  const next = generateDeviceId();
  LocalStorageUtil.setItem(DEVICE_ID_STORAGE_KEY, next);
  return next;
}

export function syncDeviceId(deviceId: string | null | undefined) {
  const normalizedDeviceId = (deviceId || '').trim();
  if (!normalizedDeviceId) {
    return;
  }
  LocalStorageUtil.setItem(DEVICE_ID_STORAGE_KEY, normalizedDeviceId);
}
