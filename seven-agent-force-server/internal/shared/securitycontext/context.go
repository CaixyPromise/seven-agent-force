package securitycontext

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const requestContextKey = "__seven_auth_user_context__"

type userContextKey struct{}

// OrganizationScope is the server-derived organization boundary used by
// organization-owned resources. Source records whether the primary
// organization or the single-membership fallback selected the scope.
type OrganizationScope struct {
	OrgID   int64
	ScopeID string
	Source  string
}

type DataScopeType string

const (
	DataScopeNone         DataScopeType = "NONE"
	DataScopeAll          DataScopeType = "ALL"
	DataScopeCustom       DataScopeType = "CUSTOM"
	DataScopeDept         DataScopeType = "DEPT"
	DataScopeDeptAndChild DataScopeType = "DEPT_AND_CHILD"
	DataScopeSelf         DataScopeType = "SELF"
)

type UserContext struct {
	UserID           int64         `json:"userId"`
	Username         string        `json:"username,omitempty"`
	Nickname         string        `json:"nickname,omitempty"`
	RoleIDs          []int64       `json:"roleIds,omitempty"`
	Roles            []string      `json:"roles,omitempty"`
	Permissions      []string      `json:"permissions,omitempty"`
	PrimaryOrgID     int64         `json:"primaryOrgId,omitempty"`
	OrgIDs           []int64       `json:"orgIds,omitempty"`
	PrimaryDeptID    int64         `json:"primaryDeptId,omitempty"`
	DeptIDs          []int64       `json:"deptIds,omitempty"`
	PostIDs          []int64       `json:"postIds,omitempty"`
	PostCodes        []string      `json:"postCodes,omitempty"`
	DataScopeDeptIDs []int64       `json:"dataScopeDeptIds,omitempty"`
	DataScopeOrgIDs  []int64       `json:"dataScopeOrgIds,omitempty"`
	DataScopeType    DataScopeType `json:"dataScopeType,omitempty"`
	SessionID        string        `json:"sessionId,omitempty"`
	AuthVersion      int64         `json:"authVersion,omitempty"`
	SessionVersion   int64         `json:"sessionVersion,omitempty"`
	IssuedAtEpoch    int64         `json:"issuedAtEpoch,omitempty"`
	ExpireAtEpoch    int64         `json:"expireAtEpoch,omitempty"`
	Source           string        `json:"source,omitempty"`
	IsAdmin          bool          `json:"isAdmin"`
	IsAnonymous      bool          `json:"isAnonymous"`
	AuthenticatedBy  []string      `json:"authenticatedBy,omitempty"`
}

func Anonymous() *UserContext {
	return &UserContext{
		Username:         "anonymous",
		Nickname:         "匿名用户",
		RoleIDs:          []int64{},
		Roles:            []string{},
		Permissions:      []string{},
		OrgIDs:           []int64{},
		DeptIDs:          []int64{},
		PostIDs:          []int64{},
		PostCodes:        []string{},
		DataScopeDeptIDs: []int64{},
		DataScopeOrgIDs:  []int64{},
		DataScopeType:    DataScopeNone,
		Source:           "anonymous",
		IsAnonymous:      true,
	}
}

func Set(reqCtx *app.RequestContext, user *UserContext) {
	if reqCtx == nil || user == nil {
		return
	}
	reqCtx.Set(requestContextKey, user)
}

func Clear(reqCtx *app.RequestContext) {
	if reqCtx == nil {
		return
	}
	reqCtx.Set(requestContextKey, nil)
}

func Get(reqCtx *app.RequestContext) *UserContext {
	if reqCtx == nil {
		return nil
	}
	value, ok := reqCtx.Get(requestContextKey)
	if !ok || value == nil {
		return nil
	}
	if user, ok := value.(*UserContext); ok {
		return user
	}
	return nil
}

// WithUser attaches the already-authenticated user context to a standard Go
// context for application and cross-module facade calls.
func WithUser(ctx context.Context, user *UserContext) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if user == nil {
		return ctx
	}
	return context.WithValue(ctx, userContextKey{}, user)
}

// FromContext returns the authenticated user attached by the HTTP boundary.
func FromContext(ctx context.Context) *UserContext {
	if ctx == nil {
		return nil
	}
	user, _ := ctx.Value(userContextKey{}).(*UserContext)
	return user
}

// ResolveOrganizationScope derives a non-ambiguous organization scope from
// server-authenticated membership data. PrimaryOrgID is authoritative. A
// fallback is allowed only when exactly one positive organization membership
// exists, so the fallback remains deterministic and auditable.
func ResolveOrganizationScope(user *UserContext) (OrganizationScope, error) {
	if user == nil || user.IsAnonymous || user.UserID <= 0 {
		return OrganizationScope{}, fmt.Errorf("authenticated user context is required")
	}
	orgs := make(map[int64]struct{}, len(user.OrgIDs))
	for _, orgID := range user.OrgIDs {
		if orgID > 0 {
			orgs[orgID] = struct{}{}
		}
	}
	if user.PrimaryOrgID > 0 {
		if len(orgs) > 0 {
			if _, ok := orgs[user.PrimaryOrgID]; !ok {
				return OrganizationScope{}, fmt.Errorf("primary organization is not an authenticated membership")
			}
		}
		return organizationScope(user.PrimaryOrgID, "primary-org"), nil
	}
	if len(orgs) != 1 {
		return OrganizationScope{}, fmt.Errorf("organization scope is missing or ambiguous")
	}
	for orgID := range orgs {
		return organizationScope(orgID, "single-org-fallback"), nil
	}
	return OrganizationScope{}, fmt.Errorf("organization scope is missing")
}

func organizationScope(orgID int64, source string) OrganizationScope {
	return OrganizationScope{
		OrgID:   orgID,
		ScopeID: "org:" + strconv.FormatInt(orgID, 10),
		Source:  source,
	}
}

func Require(reqCtx *app.RequestContext) *UserContext {
	user := Get(reqCtx)
	if user == nil {
		return Anonymous()
	}
	return user
}

func IsLogin(reqCtx *app.RequestContext) bool {
	user := Get(reqCtx)
	return user != nil && !user.IsAnonymous && user.UserID > 0
}

func CurrentUserID(reqCtx *app.RequestContext) (int64, bool) {
	user := Get(reqCtx)
	if user == nil || user.IsAnonymous || user.UserID <= 0 {
		return 0, false
	}
	return user.UserID, true
}

func CurrentUsername(reqCtx *app.RequestContext) (string, bool) {
	user := Get(reqCtx)
	if user == nil || user.IsAnonymous || strings.TrimSpace(user.Username) == "" {
		return "", false
	}
	return user.Username, true
}

func HasPermission(reqCtx *app.RequestContext, permission string) bool {
	user := Get(reqCtx)
	if user == nil || user.IsAnonymous {
		return false
	}
	if user.IsAdmin {
		return true
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	for _, item := range user.Permissions {
		candidate := strings.TrimSpace(item)
		if PermissionMatches(candidate, permission) {
			return true
		}
	}
	return false
}

func HasRole(reqCtx *app.RequestContext, role string) bool {
	user := Get(reqCtx)
	if user == nil || user.IsAnonymous {
		return false
	}
	role = strings.TrimSpace(role)
	for _, item := range user.Roles {
		if strings.TrimSpace(item) == role {
			return true
		}
	}
	return false
}

// PermissionMatches reports whether a granted permission code covers a required code.
func PermissionMatches(candidate string, permission string) bool {
	candidate = strings.TrimSpace(candidate)
	permission = strings.TrimSpace(permission)
	if candidate == "" || permission == "" {
		return false
	}
	if candidate == "*" || candidate == permission {
		return true
	}
	if strings.HasSuffix(candidate, ":*") {
		prefix := strings.TrimSuffix(candidate, "*")
		return strings.HasPrefix(permission, prefix)
	}
	return false
}
