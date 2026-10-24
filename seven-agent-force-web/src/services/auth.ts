import { getCurrentUser } from '@/api/authController';
import { toApiIdParam } from '@/lib/apiId';
import type {
	DataScopeType,
	LoginResponse,
	LoginUser,
	OidcTokenResponse,
	RefreshResponse,
} from '@/lib/http/types';

type UserSource = Record<string, unknown>;
type LoginUserItem = { name?: string; roleName?: string; postName?: string; code?: string; roleCode?: string };

const normalizeStringList = (value: unknown): string[] => {
  if (!value) return [];
  if (Array.isArray(value)) {
    return value
      .map((item) => {
        if (typeof item === 'string') return item;
        if (item && typeof item === 'object') {
          const typedItem = item as LoginUserItem;
          return typedItem.name || typedItem.roleName || typedItem.postName || typedItem.code;
        }
        return undefined;
      })
      .filter((item): item is string => Boolean(item));
  }
  return [];
};

const DATA_SCOPE_TYPES = new Set<DataScopeType>([
  'ALL',
  'CUSTOM',
  'DEPT',
  'DEPT_AND_CHILD',
  'SELF',
  'NONE',
]);

const normalizeIdentifier = (value: unknown): API.Int64 | undefined => {
	try {
		return toApiIdParam(value);
	} catch {
		return undefined;
	}
};

const normalizeIdentifierList = (value: unknown): API.Int64[] =>
	Array.isArray(value)
		? value.map(normalizeIdentifier).filter((item): item is API.Int64 => Boolean(item))
		: [];

const normalizeDataScope = (value: unknown, fallbackUserId?: API.Int64) => {
  const source = value && typeof value === 'object' ? (value as UserSource) : {};
  const rawScopeType = typeof source.scopeType === 'string' ? source.scopeType.toUpperCase() : 'NONE';
  const scopeType = DATA_SCOPE_TYPES.has(rawScopeType as DataScopeType)
    ? (rawScopeType as DataScopeType)
    : 'NONE';
  return {
		userId: normalizeIdentifier(source.userId) ?? fallbackUserId,
    deptIds: normalizeIdentifierList(source.deptIds),
    orgIds: normalizeIdentifierList(source.orgIds),
    scopeType,
  };
};

export const normalizeCurrentUser = (raw: unknown): LoginUser => {
  if (!raw) return {};
  const source = raw as UserSource;

  const userRole = normalizeStringList(source.userRole);
  const userPosition = normalizeStringList(source.userPosition);

	const id = normalizeIdentifier(source.id) ?? normalizeIdentifier(source.userId);
  return {
    id,
    username: (source.username as string | undefined) ?? (source.userAccount as string | undefined),
    nickname:
      (source.nickname as string | undefined)
      ?? (source.userNickname as string | undefined)
      ?? (source.nickName as string | undefined)
      ?? (source.userName as string | undefined),
    avatar: (source.avatar as string | undefined) ?? (source.userAvatar as string | undefined),
    userAvatar: (source.userAvatar as string | undefined) ?? (source.avatar as string | undefined),
    userRole,
    userPosition,
    organizations: normalizeStringList(source.organizations),
    departments: normalizeStringList(source.departments),
    permissions: (source.permissions as string[] | undefined) ?? [],
    roleCodes:
      (source.roleCodes as string[] | undefined) ??
      (Array.isArray(source.userRole)
        ? source.userRole
          .map((item) => {
            if (!item || typeof item !== 'object') {
              return undefined;
            }
            const typedItem = item as LoginUserItem;
            return typedItem.code || typedItem.roleCode;
          })
          .filter((item): item is string => Boolean(item))
        : []),
    postCodes: (source.postCodes as string[] | undefined) ?? [],
    orgCodes: (source.orgCodes as string[] | undefined) ?? [],
    deptCodes: (source.deptCodes as string[] | undefined) ?? [],
    isAdmin: source.isAdmin === true,
		primaryOrgId: normalizeIdentifier(source.primaryOrgId),
		authVersion: normalizeIdentifier(source.authVersion),
    dataScope: normalizeDataScope(source.dataScope, id),
  };
};

export const normalizeLoginResponse = (raw: unknown): LoginResponse => {
  if (!raw) return {};
  const source = raw as Record<string, unknown>;
  return {
    user: source.user ? normalizeCurrentUser(source.user) : undefined,
    accessToken: source.accessToken as string | undefined,
    tokenType: source.tokenType as string | undefined,
    accessTtlSec: source.accessTtlSec as number | undefined,
    firstLogin: source.firstLogin as boolean | undefined,
  };
};

export const normalizeRefreshResponse = (raw: unknown): RefreshResponse => {
  if (!raw) return {};
  const source = raw as Record<string, unknown>;
  return {
    accessToken:
      (source.accessToken as string | undefined) ?? (source.access_token as string | undefined),
    tokenType:
      (source.tokenType as string | undefined) ?? (source.token_type as string | undefined),
    accessTtlSec:
      (source.accessTtlSec as number | undefined) ?? (source.expires_in as number | undefined),
    refreshToken:
      (source.refreshToken as string | undefined) ?? (source.refresh_token as string | undefined),
  };
};

export const normalizeOidcTokenResponse = (raw: unknown): OidcTokenResponse => {
  if (!raw) return {};
  const source = raw as Record<string, unknown>;
  return {
    accessToken:
      (source.accessToken as string | undefined) ?? (source.access_token as string | undefined),
    tokenType:
      (source.tokenType as string | undefined) ?? (source.token_type as string | undefined),
    accessTtlSec:
      (source.accessTtlSec as number | undefined) ?? (source.expires_in as number | undefined),
    refreshToken:
      (source.refreshToken as string | undefined) ?? (source.refresh_token as string | undefined),
    idToken: (source.idToken as string | undefined) ?? (source.id_token as string | undefined),
    scope: source.scope as string | undefined,
  };
};

export function fetchCurrentUser(options?: Record<string, unknown>) {
  return getCurrentUser(options);
}
