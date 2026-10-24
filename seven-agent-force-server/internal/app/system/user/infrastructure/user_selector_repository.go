package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	"github.com/jmoiron/sqlx"
)

// ListUserOptions returns enabled users visible in the requested data scope.
func (r *Repository) ListUserOptions(ctx context.Context, query domain.UserSelectorQuery) ([]domain.UserSelectorRecord, error) {
	if r == nil || r.db == nil {
		return []domain.UserSelectorRecord{}, nil
	}
	where, args := buildUserSelectorWhere(query)
	args = append(args, query.Limit)
	statement := `
SELECT u.id, u.userAccount, u.nickName, COALESCE(u.userAvatar, '') AS userAvatar, u.status
FROM sys_user u ` + where + `
ORDER BY u.nickName ASC, u.id ASC
LIMIT ?`
	records := make([]domain.UserSelectorRecord, 0, query.Limit)
	exec := r.executor(ctx)
	if err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(statement), args...); err != nil {
		return nil, fmt.Errorf("list user options: %w", err)
	}
	return records, nil
}

// FindVisibleUserOptionByID returns a minimum user projection inside the requested data scope.
func (r *Repository) FindVisibleUserOptionByID(ctx context.Context, userID int64, scope domain.DataScopeFilter) (*domain.UserSelectorRecord, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, nil
	}
	conditions := []string{"u.id = ?", "u.isDeleted = 0"}
	args := []any{userID}
	if scope.Enabled {
		scopeCondition, scopeArgs := userScopeCondition(scope)
		conditions = append(conditions, scopeCondition)
		args = append(args, scopeArgs...)
	}
	statement := `
SELECT u.id, u.userAccount, u.nickName, COALESCE(u.userAvatar, '') AS userAvatar, u.status
FROM sys_user u
WHERE ` + strings.Join(conditions, " AND ") + `
LIMIT 1`
	var record domain.UserSelectorRecord
	exec := r.executor(ctx)
	if err := sqlx.GetContext(ctx, exec, &record, exec.Rebind(statement), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find visible user option: %w", err)
	}
	return &record, nil
}

func buildUserSelectorWhere(query domain.UserSelectorQuery) (string, []any) {
	conditions := []string{"u.isDeleted = 0", "u.status = 0"}
	args := make([]any, 0, 8)
	keyword := strings.TrimSpace(query.Keyword)
	if keyword != "" {
		conditions = append(conditions, "(u.userAccount LIKE ? OR u.nickName LIKE ?)")
		value := "%" + keyword + "%"
		args = append(args, value, value)
	}
	if query.DeptID > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM sys_user_dept selected_dept WHERE selected_dept.userId = u.id AND selected_dept.deptId = ?)")
		args = append(args, query.DeptID)
	}
	if query.Scope.Enabled {
		scopeCondition, scopeArgs := userScopeCondition(query.Scope)
		conditions = append(conditions, scopeCondition)
		args = append(args, scopeArgs...)
	}
	return "WHERE " + strings.Join(conditions, " AND "), args
}
