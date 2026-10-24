export type PermissionMatchMode = 'any' | 'all';

function normalizePermissionCode(code: string | undefined | null) {
  return (code ?? '').trim();
}

function splitPermissionCode(code: string) {
  return normalizePermissionCode(code)
    .split(':')
    .map((segment) => segment.trim())
    .filter(Boolean);
}

export function matchesPermission(grantedPermission: string, requiredPermission: string) {
  const granted = normalizePermissionCode(grantedPermission);
  const required = normalizePermissionCode(requiredPermission);

  if (!granted || !required) {
    return false;
  }

  if (granted === '*' || granted === required) {
    return true;
  }

  const grantedSegments = splitPermissionCode(granted);
  const requiredSegments = splitPermissionCode(required);
  const wildcardIndex = grantedSegments.indexOf('*');

  if (wildcardIndex === -1) {
    return false;
  }

  const prefixSegments = grantedSegments.slice(0, wildcardIndex);
  if (requiredSegments.length < prefixSegments.length) {
    return false;
  }

  return prefixSegments.every((segment, index) => segment === requiredSegments[index]);
}

export function hasPermission(
  grantedPermissions: string[] | undefined | null,
  requiredPermission: string | undefined | null,
) {
  const required = normalizePermissionCode(requiredPermission);
  if (!required) {
    return true;
  }

  return (grantedPermissions ?? []).some((grantedPermission) =>
    matchesPermission(grantedPermission, required),
  );
}

export function hasAnyPermission(
  grantedPermissions: string[] | undefined | null,
  requiredPermissions: string[] | undefined | null,
) {
  const permissions = (requiredPermissions ?? []).map(normalizePermissionCode).filter(Boolean);
  if (permissions.length === 0) {
    return true;
  }

  return permissions.some((permission) => hasPermission(grantedPermissions, permission));
}

export function hasAllPermissions(
  grantedPermissions: string[] | undefined | null,
  requiredPermissions: string[] | undefined | null,
) {
  const permissions = (requiredPermissions ?? []).map(normalizePermissionCode).filter(Boolean);
  if (permissions.length === 0) {
    return true;
  }

  return permissions.every((permission) => hasPermission(grantedPermissions, permission));
}

export function hasPermissions(
  grantedPermissions: string[] | undefined | null,
  requiredPermissions: string[] | undefined | null,
  matchMode: PermissionMatchMode = 'any',
) {
  return matchMode === 'all'
    ? hasAllPermissions(grantedPermissions, requiredPermissions)
    : hasAnyPermission(grantedPermissions, requiredPermissions);
}
