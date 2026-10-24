package domain

import (
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) IsAdmin(roles []RoleRecord) bool {
	for _, role := range roles {
		if role.IsActiveSuperAdmin() {
			return true
		}
	}
	return false
}

func (s *Service) NormalizeCodes(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := set[trimmed]; ok {
			continue
		}
		set[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func (s *Service) FilterPermissionsByModule(values []string, module string) []string {
	module = strings.TrimSpace(strings.ToLower(module))
	if module == "" {
		return s.NormalizeCodes(values)
	}
	module = strings.ReplaceAll(module, "-", ":")
	result := make([]string, 0, len(values))
	for _, item := range values {
		candidate := strings.TrimSpace(strings.ToLower(item))
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, module) || strings.Contains(candidate, module) {
			result = append(result, item)
		}
	}
	return s.NormalizeCodes(result)
}

func (s *Service) EffectiveDataScope(roles []RoleRecord, customDeptIDs, deptIDs, orgIDs []int64, deptHierarchies map[int64]string, allDeptIDsByHierarchy map[string][]int64) *UserDataScope {
	if len(roles) == 0 {
		return &UserDataScope{DeptIDs: []int64{}, OrgIDs: []int64{}, ScopeType: securitycontext.DataScopeNone}
	}
	best := 99
	for _, role := range roles {
		switch role.DataScope {
		case 1:
			best = minInt(best, 1)
		case 2:
			best = minInt(best, 2)
		case 3:
			best = minInt(best, 3)
		case 4:
			best = minInt(best, 4)
		case 5:
			best = minInt(best, 5)
		}
	}
	switch best {
	case 1:
		return &UserDataScope{DeptIDs: []int64{}, OrgIDs: []int64{}, ScopeType: securitycontext.DataScopeAll}
	case 2:
		return &UserDataScope{DeptIDs: uniqueInt64(customDeptIDs), OrgIDs: uniqueInt64(orgIDs), ScopeType: securitycontext.DataScopeCustom}
	case 3:
		return &UserDataScope{DeptIDs: uniqueInt64(deptIDs), OrgIDs: uniqueInt64(orgIDs), ScopeType: securitycontext.DataScopeDept}
	case 4:
		expanded := make([]int64, 0, len(deptIDs))
		for _, deptID := range uniqueInt64(deptIDs) {
			hierarchy := strings.TrimSpace(deptHierarchies[deptID])
			if hierarchy == "" {
				expanded = append(expanded, deptID)
				continue
			}
			expanded = append(expanded, allDeptIDsByHierarchy[hierarchy]...)
		}
		return &UserDataScope{DeptIDs: uniqueInt64(expanded), OrgIDs: uniqueInt64(orgIDs), ScopeType: securitycontext.DataScopeDeptAndChild}
	case 5:
		return &UserDataScope{DeptIDs: uniqueInt64(deptIDs), OrgIDs: uniqueInt64(orgIDs), ScopeType: securitycontext.DataScopeSelf}
	default:
		return &UserDataScope{DeptIDs: uniqueInt64(deptIDs), OrgIDs: uniqueInt64(orgIDs), ScopeType: securitycontext.DataScopeNone}
	}
}

func (s *Service) IsTempPermissionValid(item TemporaryPermissionRecord, now time.Time) bool {
	if item.Type != 1 {
		return true
	}
	return item.ExpireAt == nil || item.ExpireAt.After(now.UTC())
}

func uniqueInt64(values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	set := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, item := range values {
		if item <= 0 {
			continue
		}
		if _, ok := set[item]; ok {
			continue
		}
		set[item] = struct{}{}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
