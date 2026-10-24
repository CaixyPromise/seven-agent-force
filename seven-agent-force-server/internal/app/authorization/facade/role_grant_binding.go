package facade

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RoleGrantRequestHash returns the stable digest used by idempotency and step-up binding.
func RoleGrantRequestHash(request RoleGrantBundleRequest) (string, error) {
	canonical := struct {
		ExpectedRevision int64
		MenuIDs          []int64
		PermissionIDs    []int64
		DataScope        int
		DeptIDs          []int64
		ConfigScopes     []RoleConfigScopeGrantVO
		Reason           string
	}{
		ExpectedRevision: request.ExpectedRevision,
		MenuIDs:          sortedPositiveIDs([]int64(request.MenuIDs)),
		PermissionIDs:    sortedPositiveIDs([]int64(request.PermissionIDs)),
		DataScope:        request.DataScope,
		DeptIDs:          sortedPositiveIDs([]int64(request.DeptIDs)),
		ConfigScopes:     sortedConfigScopes(request.ConfigScopes),
		Reason:           strings.TrimSpace(request.Reason),
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode role grant request: %w", err)
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

// TemporaryPermissionOperationBinding binds one user permission mutation to its protected proof.
func TemporaryPermissionOperationBinding(action string, userID int64, permissionCode string, expireAt *time.Time, reason string) string {
	expiration := ""
	if expireAt != nil {
		expiration = expireAt.UTC().Format(time.RFC3339Nano)
	}
	payload := strings.Join([]string{
		strings.TrimSpace(action), fmt.Sprintf("%d", userID), strings.TrimSpace(permissionCode),
		expiration, strings.TrimSpace(reason),
	}, "\x00")
	sum := sha256.Sum256([]byte(payload))
	return fmt.Sprintf("user:%d|permission:%s|changeHash:%s", userID, strings.TrimSpace(permissionCode), hex.EncodeToString(sum[:]))
}

// RoleGrantOperationBinding returns the exact protected-operation binding for a grant bundle.
func RoleGrantOperationBinding(roleID int64, request RoleGrantBundleRequest) (string, error) {
	hash, err := RoleGrantRequestHash(request)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("role:%d|revision:%d|grantHash:%s", roleID, request.ExpectedRevision, hash), nil
}

func sortedPositiveIDs(values []int64) []int64 {
	set := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value > 0 {
			set[value] = struct{}{}
		}
	}
	result := make([]int64, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func sortedConfigScopes(values []RoleConfigScopeGrantVO) []RoleConfigScopeGrantVO {
	result := append([]RoleConfigScopeGrantVO(nil), values...)
	for index := range result {
		result[index].GroupCode = strings.TrimSpace(result[index].GroupCode)
		result[index].ConfigKey = strings.TrimSpace(result[index].ConfigKey)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].GroupCode+"\x00"+result[i].ConfigKey, result[j].GroupCode+"\x00"+result[j].ConfigKey
		if left != right {
			return left < right
		}
		if result[i].CanRead != result[j].CanRead {
			return result[i].CanRead < result[j].CanRead
		}
		if result[i].CanWrite != result[j].CanWrite {
			return result[i].CanWrite < result[j].CanWrite
		}
		return result[i].CanDelete < result[j].CanDelete
	})
	return result
}
