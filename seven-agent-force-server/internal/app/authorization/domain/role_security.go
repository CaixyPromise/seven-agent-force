package domain

import "strings"

const (
	// RoleTypeSystem identifies roles provisioned by migration or bootstrap code.
	RoleTypeSystem = 1
	// RoleStatusEnabled identifies a role that participates in effective authorization.
	RoleStatusEnabled = 0
	// AuthorizationRootSystemKey is the immutable internal identity of the global authorization root.
	AuthorizationRootSystemKey = "AUTHORIZATION_ROOT"
)

// RoleProtectionViolation identifies an immutable SYSTEM role field mutation.
type RoleProtectionViolation string

const (
	RoleProtectionNone      RoleProtectionViolation = ""
	RoleProtectionCode      RoleProtectionViolation = "code"
	RoleProtectionType      RoleProtectionViolation = "type"
	RoleProtectionStatus    RoleProtectionViolation = "status"
	RoleProtectionDataScope RoleProtectionViolation = "dataScope"
)

// SuperAdminInvariantSnapshot is read while the authorization-root role row is locked.
type SuperAdminInvariantSnapshot struct {
	// ActiveUserCount is the number of enabled, non-deleted users with a direct active root relation.
	ActiveUserCount int
	// TargetUserActive reports whether the target user is included in ActiveUserCount.
	TargetUserActive bool
}

// AuthorizationRootSecuritySnapshot is a non-locking view used by the security status API.
type AuthorizationRootSecuritySnapshot struct {
	Role            RoleRecord
	ActiveUserCount int
}

type AuthorizationRootBootstrapResult struct {
	Role               RoleRecord
	AlreadyInitialized bool
}

// IsSystem reports whether the role is managed by migration or bootstrap code.
func (r RoleRecord) IsSystem() bool {
	return r.Type == RoleTypeSystem
}

// IsAuthorizationRoot reports whether the role is the stable authorization root.
func (r RoleRecord) IsAuthorizationRoot() bool {
	return strings.EqualFold(strings.TrimSpace(r.SystemKey), AuthorizationRootSystemKey)
}

// IsActiveSuperAdmin reports whether the role currently grants direct authorization-root authority.
func (r RoleRecord) IsActiveSuperAdmin() bool {
	return r.IsAuthorizationRoot() && r.Status == RoleStatusEnabled
}

// ProtectedMutation returns the protected SYSTEM role field changed by the next record.
func (r RoleRecord) ProtectedMutation(next RoleRecord) RoleProtectionViolation {
	if !r.IsSystem() {
		return RoleProtectionNone
	}
	if strings.TrimSpace(r.Code) != strings.TrimSpace(next.Code) {
		return RoleProtectionCode
	}
	if r.Type != next.Type {
		return RoleProtectionType
	}
	if r.Status != next.Status {
		return RoleProtectionStatus
	}
	if r.DataScope != next.DataScope {
		return RoleProtectionDataScope
	}
	return RoleProtectionNone
}

// WouldRemoveLastUser reports whether replacing the target user's roles would remove the final active administrator.
func (s SuperAdminInvariantSnapshot) WouldRemoveLastUser(nextHasSuperAdmin bool) bool {
	return s.TargetUserActive && !nextHasSuperAdmin && s.ActiveUserCount <= 1
}

// WouldRemoveActiveRole reports whether a role mutation would remove the only active authorization root from all users.
func (s SuperAdminInvariantSnapshot) WouldRemoveActiveRole(current, next RoleRecord) bool {
	return current.IsActiveSuperAdmin() && !next.IsActiveSuperAdmin() && s.ActiveUserCount > 0
}
