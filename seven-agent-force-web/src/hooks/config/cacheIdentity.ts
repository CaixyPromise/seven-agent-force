import type { LoginUser } from '@/lib/http/types';

export function buildConfigCacheIdentity(user: LoginUser | null): string {
  if (!user?.id) return 'anonymous|scope=server:local|authz=0';
  const primaryScope =
    user.primaryOrgId === undefined || String(user.primaryOrgId).trim() === ''
      ? 'server:local'
      : `org:${String(user.primaryOrgId).trim()}`;
  const authzGeneration =
    user.authVersion === undefined || String(user.authVersion).trim() === ''
      ? '0'
      : String(user.authVersion).trim();
  return `account=${user.id}|scope=${primaryScope}|authz=${authzGeneration}`;
}
