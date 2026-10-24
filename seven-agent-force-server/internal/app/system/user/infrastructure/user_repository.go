package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

type userDialectExecutor struct {
	store.SQLX
	postgres bool
}

func (e userDialectExecutor) Rebind(query string) string {
	if e.postgres {
		query = userPostgresRenderer.RenderPostgres(query)
	}
	return e.SQLX.Rebind(query)
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("system user repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.Contains(strings.ToLower(provider.Driver()), "postgres") || strings.Contains(strings.ToLower(provider.Driver()), "pgx"),
	}, nil
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return userDialectExecutor{SQLX: store.SQLXExecutor(ctx, r.db), postgres: r.postgres}
}

func (r *Repository) FindSubjectByID(ctx context.Context, userID int64) (*domain.SubjectRecord, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, nil
	}
	return r.findOne(ctx, baseSubjectQuery()+` WHERE id = ? AND isDeleted = 0 LIMIT 1`, userID)
}

func (r *Repository) FindSubjectByAccount(ctx context.Context, account string) (*domain.SubjectRecord, error) {
	account = strings.TrimSpace(account)
	if r == nil || r.db == nil || account == "" {
		return nil, nil
	}
	return r.findOne(ctx, baseSubjectQuery()+` WHERE userAccount = ? AND isDeleted = 0 LIMIT 1`, account)
}

func (r *Repository) FindSubjectByEmail(ctx context.Context, email string) (*domain.SubjectRecord, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if r == nil || r.db == nil || email == "" {
		return nil, nil
	}
	exec := r.executor(ctx)
	var results []domain.SubjectRecord
	query := baseSubjectQuery() + ` WHERE LOWER(userEmail) = ? AND status = 0 AND isDeleted = 0 LIMIT 2`
	if err := sqlx.SelectContext(ctx, exec, &results, exec.Rebind(query), email); err != nil {
		return nil, fmt.Errorf("query system user by email: %w", err)
	}
	if len(results) > 1 {
		return nil, apperrors.Operation("邮箱匹配到多个用户，禁止自动绑定")
	}
	if len(results) == 0 {
		return nil, nil
	}
	return &results[0], nil
}

func (r *Repository) ExistsByID(ctx context.Context, userID int64) (bool, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return false, nil
	}
	query := `SELECT COUNT(1) FROM sys_user WHERE id = ? AND isDeleted = 0`
	exec := r.executor(ctx)
	var count int
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(query), userID); err != nil {
		return false, fmt.Errorf("exists user by id: %w", err)
	}
	return count > 0, nil
}

func (r *Repository) CountByPhoneExcludingUserID(ctx context.Context, userID int64, phone string) (int, error) {
	phone = strings.TrimSpace(phone)
	if r == nil || r.db == nil || phone == "" {
		return 0, nil
	}
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user WHERE userPhone = ? AND id <> ? AND isDeleted = 0`, phone, userID)
}

func (r *Repository) CountByEmailExcludingUserID(ctx context.Context, userID int64, email string) (int, error) {
	email = strings.TrimSpace(email)
	if r == nil || r.db == nil || email == "" {
		return 0, nil
	}
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user WHERE userEmail = ? AND id <> ? AND isDeleted = 0`, email, userID)
}

func (r *Repository) CountByEmail(ctx context.Context, email string) (int, error) {
	email = strings.TrimSpace(email)
	if r == nil || r.db == nil || email == "" {
		return 0, nil
	}
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user WHERE userEmail = ? AND isDeleted = 0`, email)
}

func (r *Repository) CountByAccountExcludingUserID(ctx context.Context, userID int64, account string) (int, error) {
	account = strings.TrimSpace(account)
	if r == nil || r.db == nil || account == "" {
		return 0, nil
	}
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user WHERE userAccount = ? AND id <> ? AND isDeleted = 0`, account, userID)
}

func (r *Repository) CreateOwnerUser(ctx context.Context, record *domain.OwnerUserRecord) error {
	if r == nil || r.db == nil || record == nil || record.UserID <= 0 {
		return nil
	}
	return r.exec(ctx, `
INSERT INTO sys_user (
	id, userAccount, nickName, status, creatorId, updaterId,
	userEmail, userGender, userAvatar, userProfile, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
		record.UserID,
		strings.TrimSpace(record.AccountName),
		strings.TrimSpace(record.NickName),
		record.Status,
		record.UserID,
		record.UserID,
		strings.TrimSpace(record.Email),
		r.boolValue(record.Gender != 0),
		strings.TrimSpace(record.Avatar),
		strings.TrimSpace(record.Profile),
		r.boolValue(false),
	)
}

func (r *Repository) UpdateProfile(ctx context.Context, userID int64, nickName, phone, avatar, profile *string) error {
	if r == nil || r.db == nil || userID <= 0 {
		return nil
	}
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if nickName != nil {
		sets = append(sets, "nickName = ?")
		args = append(args, strings.TrimSpace(*nickName))
	}
	if phone != nil {
		sets = append(sets, "userPhone = ?")
		args = append(args, nullableString(*phone))
	}
	if avatar != nil {
		sets = append(sets, "userAvatar = ?")
		args = append(args, nullableString(*avatar))
	}
	if profile != nil {
		sets = append(sets, "userProfile = ?")
		args = append(args, nullableString(*profile))
	}
	if len(sets) == 0 {
		return nil
	}
	sets = append(sets, "updateTime = NOW()")
	query := `UPDATE sys_user SET ` + strings.Join(sets, ", ") + ` WHERE id = ? AND isDeleted = 0`
	args = append(args, userID)
	return r.exec(ctx, query, args...)
}

func (r *Repository) UpdateEmail(ctx context.Context, userID int64, email string) error {
	if r == nil || r.db == nil || userID <= 0 {
		return nil
	}
	return r.exec(ctx, `UPDATE sys_user SET userEmail = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, strings.TrimSpace(email), userID)
}

func (r *Repository) CompareAndSetVerifiedEmail(ctx context.Context, userID int64, email string, expectedVersion int64, verificationSource string, verifiedAt time.Time) (bool, error) {
	if r == nil || r.db == nil || userID <= 0 || expectedVersion <= 0 {
		return false, nil
	}
	exec := r.executor(ctx)
	result, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_user
SET userEmail=?, email_verified_at=?, email_verification_source=?, email_version=email_version+1, updateTime=NOW()
WHERE id=? AND email_version=? AND isDeleted=0`),
		strings.TrimSpace(email), verifiedAt.UTC(), strings.TrimSpace(verificationSource), userID, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("compare and set verified user email: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read verified email update count: %w", err)
	}
	return affected == 1, nil
}

func (r *Repository) UpdateLockState(ctx context.Context, userID int64, status int, unsealAt *time.Time) error {
	if r == nil || r.db == nil || userID <= 0 {
		return nil
	}
	return r.exec(ctx, `UPDATE sys_user SET status = ?, unsealTime = ?, statusVersion = statusVersion + 1, statusCommandHash = NULL, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, status, unsealAt, userID)
}

func (r *Repository) CompareAndSetManagedUserStatus(ctx context.Context, userID int64, expectedStatus int, expectedVersion uint64, status int, unsealAt *time.Time, statusCommandHash string) (bool, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return false, nil
	}
	exec := r.executor(ctx)
	result, err := exec.ExecContext(ctx, exec.Rebind(`UPDATE sys_user
	SET status = ?, unsealTime = ?, statusVersion = statusVersion + 1, statusCommandHash = ?, updateTime = NOW()
WHERE id = ? AND status = ? AND statusVersion = ? AND isDeleted = 0`),
		status, unsealAt, strings.TrimSpace(statusCommandHash), userID, expectedStatus, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("compare and set managed user status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read managed user status update count: %w", err)
	}
	return affected > 0, nil
}

func (r *Repository) QueryAdminUsers(ctx context.Context, query domain.AdminUserQuery) ([]domain.AdminUserRecord, int64, error) {
	if r == nil || r.db == nil {
		return nil, 0, nil
	}
	candidateIDs, constrained, err := r.resolveAdminUserCandidateIDs(ctx, query)
	if err != nil {
		return nil, 0, err
	}
	if constrained && len(candidateIDs) == 0 {
		return []domain.AdminUserRecord{}, 0, nil
	}
	where, args, err := buildUserWhere(query, candidateIDs, constrained)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.count(ctx, `SELECT COUNT(1) FROM sys_user u `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	current, size := pageArgs(query.Current, query.Size)
	args = append(args, size, (current-1)*size)
	sql := `
SELECT u.id, u.userAccount, u.nickName, COALESCE(u.userAvatar, '') AS userAvatar,
	COALESCE(u.userEmail, '') AS userEmail, COALESCE(u.userPhone, '') AS userPhone,
	` + r.genderProjection("u.userGender") + `, COALESCE(u.userProfile, '') AS userProfile, u.status, u.statusVersion, COALESCE(u.statusCommandHash, '') AS statusCommandHash, u.createTime, u.updateTime
FROM sys_user u ` + where + ` ORDER BY u.createTime DESC, u.id DESC LIMIT ? OFFSET ?`
	var records []domain.AdminUserRecord
	exec := r.executor(ctx)
	if err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(sql), args...); err != nil {
		return nil, 0, fmt.Errorf("query admin users: %w", err)
	}
	return records, int64(total), nil
}

const adminUserIntermediateLimit = 500

func (r *Repository) resolveAdminUserCandidateIDs(ctx context.Context, query domain.AdminUserQuery) ([]int64, bool, error) {
	var candidate []int64
	constrained := false
	intersect := func(ids []int64) {
		ids = normalizeIDs(ids)
		if !constrained {
			candidate = ids
			constrained = true
			return
		}
		allowed := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			allowed[id] = struct{}{}
		}
		filtered := candidate[:0]
		for _, id := range candidate {
			if _, ok := allowed[id]; ok {
				filtered = append(filtered, id)
			}
		}
		candidate = filtered
	}
	load := func(kind string, values []int64) error {
		ids, err := r.listDimensionUserIDs(ctx, kind, values)
		if err != nil {
			return err
		}
		intersect(ids)
		return nil
	}
	if query.OrgID > 0 {
		if err := load("org", []int64{query.OrgID}); err != nil {
			return nil, false, err
		}
	}
	deptIDs := []int64{}
	if query.DeptID > 0 {
		deptIDs = []int64{query.DeptID}
	}
	if query.Scope.Enabled {
		switch {
		case query.Scope.None:
			return []int64{}, true, nil
		case query.Scope.SelfUserID > 0:
			intersect([]int64{query.Scope.SelfUserID})
		default:
			scopeDeptIDs := normalizeIDs(query.Scope.DeptIDs)
			if len(deptIDs) > 0 {
				inScope := false
				for _, id := range scopeDeptIDs {
					if id == deptIDs[0] {
						inScope = true
						break
					}
				}
				if !inScope {
					return []int64{}, true, nil
				}
			} else {
				deptIDs = scopeDeptIDs
			}
		}
	}
	if len(deptIDs) > 0 {
		if err := load("dept", deptIDs); err != nil {
			return nil, false, err
		}
	}
	if query.PostID > 0 {
		if err := load("post", []int64{query.PostID}); err != nil {
			return nil, false, err
		}
	}
	sort.Slice(candidate, func(i, j int) bool { return candidate[i] < candidate[j] })
	return candidate, constrained, nil
}

func (r *Repository) listDimensionUserIDs(ctx context.Context, kind string, values []int64) ([]int64, error) {
	values = normalizeIDs(values)
	if len(values) == 0 {
		return []int64{}, nil
	}
	var baseSQL string
	switch kind {
	case "org":
		baseSQL = `SELECT userId FROM sys_user_org WHERE orgId IN (?) AND isDeleted = 0 ORDER BY userId LIMIT ?`
	case "dept":
		baseSQL = `SELECT userId FROM sys_user_dept WHERE deptId IN (?) ORDER BY userId LIMIT ?`
	case "post":
		baseSQL = `SELECT userId FROM sys_user_position WHERE postId IN (?) AND isDeleted = 0 ORDER BY userId LIMIT ?`
	default:
		return nil, fmt.Errorf("unsupported admin user dimension %q", kind)
	}
	query, args, err := sqlx.In(baseSQL, values, adminUserIntermediateLimit+1)
	if err != nil {
		return nil, err
	}
	exec := r.executor(ctx)
	var ids []int64
	if err := sqlx.SelectContext(ctx, exec, &ids, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list %s candidate users: %w", kind, err)
	}
	if len(ids) > adminUserIntermediateLimit {
		return nil, fmt.Errorf("%s candidate user set exceeds %d", kind, adminUserIntermediateLimit)
	}
	return ids, nil
}

func buildUserWhere(query domain.AdminUserQuery, candidateIDs []int64, constrained bool) (string, []any, error) {
	conditions := []string{"u.isDeleted = 0"}
	args := make([]any, 0, 8)
	if strings.TrimSpace(query.Account) != "" {
		conditions = append(conditions, "u.userAccount LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Account)+"%")
	}
	if strings.TrimSpace(query.Nickname) != "" {
		conditions = append(conditions, "u.nickName LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Nickname)+"%")
	}
	if query.Status != nil {
		conditions = append(conditions, "u.status = ?")
		args = append(args, *query.Status)
	}
	if constrained {
		inCondition, inArgs, err := sqlx.In("u.id IN (?)", candidateIDs)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, inCondition)
		args = append(args, inArgs...)
	}
	return " WHERE " + strings.Join(conditions, " AND "), args, nil
}

func userScopeCondition(scope domain.DataScopeFilter) (string, []any) {
	if scope.None {
		return "1 = 0", nil
	}
	if scope.SelfUserID > 0 {
		return "u.id = ?", []any{scope.SelfUserID}
	}
	if condition, values := existsInCondition("sys_user_dept", "deptId", "userId", "u.id", scope.DeptIDs); condition != "" {
		return condition, values
	}
	return "1 = 0", nil
}

func (r *Repository) FindAdminUserByID(ctx context.Context, userID int64) (*domain.AdminUserRecord, error) {
	if r == nil || r.db == nil || userID <= 0 {
		return nil, nil
	}
	exec := r.executor(ctx)
	var result domain.AdminUserRecord
	err := sqlx.GetContext(ctx, exec, &result, exec.Rebind(`
SELECT id, userAccount, nickName, COALESCE(userAvatar, '') AS userAvatar,
	COALESCE(userEmail, '') AS userEmail, COALESCE(userPhone, '') AS userPhone,
	`+r.genderProjection("userGender")+`, COALESCE(userProfile, '') AS userProfile, status, statusVersion, COALESCE(statusCommandHash, '') AS statusCommandHash, createTime, updateTime
FROM sys_user WHERE id = ? AND isDeleted = 0 LIMIT 1`), userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find admin user: %w", err)
	}
	return &result, nil
}

func (r *Repository) CreateAdminUser(ctx context.Context, record domain.AdminUserCreateRecord) error {
	gender := 0
	if record.Gender != nil {
		gender = *record.Gender
	}
	return r.exec(ctx, `
INSERT INTO sys_user (
	id, userAccount, nickName, status, creatorId, updaterId, userPhone, userEmail,
	userGender, userAvatar, userProfile, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, NOW(), NOW(), 0)`,
		record.ID, strings.TrimSpace(record.AccountName), strings.TrimSpace(record.NickName), record.Status,
		nullableInt64(record.OperatorID), nullableInt64(record.OperatorID), nullableString(record.Phone),
		nullableString(record.Email), r.boolValue(gender != 0), nullableString(record.Profile))
}

func (r *Repository) CreateExternalSubject(ctx context.Context, record domain.ExternalSubjectCreateRecord) error {
	return r.exec(ctx, `
INSERT INTO sys_user (
	id, userAccount, nickName, status, creatorId, updaterId, userPhone, userEmail,
	userGender, userAvatar, userProfile, registerPlatformCode, registerProviderCode,
	createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, NULL, NULL, NULL, ?, 0, ?, '', ?, ?, NOW(), NOW(), 0)`,
		record.ID, strings.TrimSpace(record.AccountName), strings.TrimSpace(record.NickName),
		record.Status, strings.TrimSpace(record.Email), nullableString(record.Avatar),
		nullableString(record.RegisterPlatformCode), nullableString(record.RegisterProviderCode))
}

func (r *Repository) CreateFormSubject(ctx context.Context, record domain.FormSubjectCreateRecord) error {
	return r.exec(ctx, `
INSERT INTO sys_user (
	id, userAccount, nickName, status, creatorId, updaterId, userPhone, userEmail,
	userGender, userAvatar, userProfile, registerPlatformCode, registerProviderCode,
	createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, NULL, NULL, NULL, ?, 0, '', '', ?, ?, NOW(), NOW(), 0)`,
		record.ID, strings.TrimSpace(record.AccountName), strings.TrimSpace(record.NickName),
		record.Status, strings.TrimSpace(record.Email), nullableString(record.RegisterPlatformCode),
		nullableString(record.RegisterProviderCode))
}

func (r *Repository) UpdateAdminUser(ctx context.Context, record domain.AdminUserUpdateRecord) error {
	if record.ID <= 0 {
		return nil
	}
	sets := []string{"nickName = ?", "userEmail = ?", "userPhone = ?", "updaterId = ?", "updateTime = NOW()"}
	args := []any{strings.TrimSpace(record.NickName), nullableString(record.Email), nullableString(record.Phone), nullableInt64(record.OperatorID)}
	if record.Gender != nil {
		sets = append(sets, "userGender = ?")
		args = append(args, r.boolValue(*record.Gender != 0))
	}
	if record.Profile != nil {
		sets = append(sets, "userProfile = ?")
		args = append(args, nullableString(*record.Profile))
	}
	if record.Status != nil {
		sets = append(sets, "status = ?", "statusVersion = statusVersion + 1", "statusCommandHash = NULL")
		args = append(args, *record.Status)
	}
	args = append(args, record.ID)
	return r.exec(ctx, `UPDATE sys_user SET `+strings.Join(sets, ", ")+` WHERE id = ? AND isDeleted = 0`, args...)
}

func (r *Repository) SoftDeleteUser(ctx context.Context, userID, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_user SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, nullableInt64(operatorID), userID)
}

func (r *Repository) ListUserRoleIDs(ctx context.Context, userID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT roleId FROM sys_user_role WHERE userId = ? AND isDeleted = 0 ORDER BY roleId ASC`, userID)
}

func (r *Repository) ListUserOrgIDs(ctx context.Context, userID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT orgId FROM sys_user_org WHERE userId = ? AND isDeleted = 0 ORDER BY isPrimary DESC, orgId ASC`, userID)
}

func (r *Repository) ListUserDeptIDs(ctx context.Context, userID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT deptId FROM sys_user_dept WHERE userId = ? ORDER BY isPrimary DESC, deptId ASC`, userID)
}

func (r *Repository) ListUserPostIDs(ctx context.Context, userID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT postId FROM sys_user_position WHERE userId = ? AND isDeleted = 0 ORDER BY isPrimary DESC, postId ASC`, userID)
}

func (r *Repository) ListActiveUserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	if roleID <= 0 {
		return []int64{}, nil
	}
	return r.listIDs(ctx, r.activeUserIDsByRoleQuery(false), roleID)
}

func (r *Repository) ListActiveUserIDsByRoleIDPage(ctx context.Context, roleID, afterUserID int64, limit int) ([]int64, error) {
	if roleID <= 0 {
		return []int64{}, nil
	}
	if afterUserID < 0 {
		afterUserID = 0
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return r.listIDs(ctx, r.activeUserIDsByRoleQuery(true), roleID, afterUserID, limit)
}

// activeUserIDsByRoleQuery keeps the notification audience facade portable
// across the project's MySQL and quoted-camelCase PostgreSQL schemas.
func (r *Repository) activeUserIDsByRoleQuery(paged bool) string {
	if r.isPostgres() {
		query := `
SELECT DISTINCT u.id
FROM sys_user u
JOIN sys_user_role ur ON ur."userId" = u.id AND ur."isDeleted" = FALSE
WHERE ur."roleId" = ? AND u."isDeleted" = FALSE AND u.status = 0`
		if paged {
			query += ` AND u.id > ?`
		}
		query += `
ORDER BY u.id ASC`
		if paged {
			query += `
LIMIT ?`
		}
		return query
	}
	query := `
SELECT DISTINCT u.id
FROM sys_user u
JOIN sys_user_role ur ON ur.userId = u.id AND ur.isDeleted = 0
WHERE ur.roleId = ? AND u.isDeleted = 0 AND u.status = 0`
	if paged {
		query += ` AND u.id > ?`
	}
	query += `
ORDER BY u.id ASC`
	if paged {
		query += `
LIMIT ?`
	}
	return query
}

func (r *Repository) isPostgres() bool {
	db, ok := r.db.(*sqlx.DB)
	return ok && store.IsPostgres(db)
}

func (r *Repository) ReplaceUserOrgs(ctx context.Context, userID int64, orgIDs []int64, primaryOrgID int64, operatorID int64) error {
	if store.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("replace user orgs requires active transaction")
	}
	ids := normalizeIDs(orgIDs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > 100 {
		return fmt.Errorf("user org set exceeds 100")
	}
	if err := r.exec(ctx, `DELETE FROM sys_user_org WHERE userId = ?`, userID); err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*5)
		for _, id := range ids[start:end] {
			clauses = append(clauses, `(?, ?, ?, ?, ?, NOW(), NOW(), ?)`)
			args = append(args, userID, id, r.boolValue(relationPrimary(id, &primaryOrgID) != 0), nullableInt64(operatorID), nullableInt64(operatorID), r.boolValue(false))
		}
		if err := r.exec(ctx, `INSERT INTO sys_user_org (userId, orgId, isPrimary, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES `+strings.Join(clauses, ", "), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ReplaceUserDepts(ctx context.Context, userID int64, deptIDs []int64, primaryDeptID int64, operatorID int64) error {
	if store.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("replace user depts requires active transaction")
	}
	ids := normalizeIDs(deptIDs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > 100 {
		return fmt.Errorf("user dept set exceeds 100")
	}
	if err := r.exec(ctx, `DELETE FROM sys_user_dept WHERE userId = ?`, userID); err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*4)
		for _, id := range ids[start:end] {
			clauses = append(clauses, `(?, ?, ?, ?, NOW())`)
			args = append(args, userID, id, r.boolValue(relationPrimary(id, &primaryDeptID) != 0), nullableInt64(operatorID))
		}
		if err := r.exec(ctx, `INSERT INTO sys_user_dept (userId, deptId, isPrimary, creatorId, createTime) VALUES `+strings.Join(clauses, ", "), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ReplaceUserPosts(ctx context.Context, userID int64, postIDs []int64, primaryPostID int64, operatorID int64) error {
	if store.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("replace user posts requires active transaction")
	}
	ids := normalizeIDs(postIDs)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) > 100 {
		return fmt.Errorf("user post set exceeds 100")
	}
	if err := r.exec(ctx, `DELETE FROM sys_user_position WHERE userId = ?`, userID); err != nil {
		return err
	}
	for start := 0; start < len(ids); start += 50 {
		end := min(start+50, len(ids))
		clauses := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*5)
		for _, id := range ids[start:end] {
			clauses = append(clauses, `(?, ?, ?, ?, ?, NOW(), NOW(), ?)`)
			args = append(args, userID, id, r.boolValue(relationPrimary(id, &primaryPostID) != 0), nullableInt64(operatorID), nullableInt64(operatorID), r.boolValue(false))
		}
		if err := r.exec(ctx, `INSERT INTO sys_user_position (userId, postId, isPrimary, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES `+strings.Join(clauses, ", "), args...); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) findOne(ctx context.Context, query string, args ...any) (*domain.SubjectRecord, error) {
	exec := r.executor(ctx)
	var result domain.SubjectRecord
	if err := sqlx.GetContext(ctx, exec, &result, exec.Rebind(query), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query system user: %w", err)
	}
	return &result, nil
}

func (r *Repository) CreateOrg(ctx context.Context, record domain.OrgRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_org (id, code, name, parentId, status, sortOrder, leaderUserId, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
		record.ID, strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), record.ParentID, record.Status, record.SortOrder, nullableInt64(record.LeaderUserID), nullableInt64(operatorID), nullableInt64(operatorID), r.boolValue(false))
}

func (r *Repository) UpdateOrg(ctx context.Context, record domain.OrgRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_org SET code = ?, name = ?, parentId = ?, status = ?, sortOrder = ?, leaderUserId = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), record.ParentID, record.Status, record.SortOrder, nullableInt64(record.LeaderUserID), nullableInt64(operatorID), record.ID)
}

func (r *Repository) DeleteOrg(ctx context.Context, orgID int64) error {
	return r.exec(ctx, `UPDATE sys_org SET isDeleted = 1, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, orgID)
}

func (r *Repository) UpdateOrgStatus(ctx context.Context, orgID int64, status int, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_org SET status = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, status, nullableInt64(operatorID), orgID)
}

func (r *Repository) UpdateOrgParent(ctx context.Context, orgID, parentID, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_org SET parentId = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, parentID, nullableInt64(operatorID), orgID)
}

func (r *Repository) FindOrgByID(ctx context.Context, orgID int64) (*domain.OrgRecord, error) {
	return findOneRecord[domain.OrgRecord](ctx, r.executor(ctx), `SELECT id, code, name, parentId, status, sortOrder, COALESCE(leaderUserId, 0) AS leaderUserId FROM sys_org WHERE id = ? AND isDeleted = 0 LIMIT 1`, orgID)
}

func (r *Repository) FindOrgsByIDs(ctx context.Context, orgIDs []int64) ([]domain.OrgRecord, error) {
	ids := normalizeIDs(orgIDs)
	if len(ids) == 0 {
		return []domain.OrgRecord{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, code, name, parentId, status, sortOrder, COALESCE(leaderUserId, 0) AS leaderUserId FROM sys_org WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return nil, err
	}
	var records []domain.OrgRecord
	exec := r.executor(ctx)
	if err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("find orgs by ids: %w", err)
	}
	return records, nil
}

func (r *Repository) FindOrgByCode(ctx context.Context, code string) (*domain.OrgRecord, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, nil
	}
	return findOneRecord[domain.OrgRecord](ctx, r.executor(ctx), `SELECT id, code, name, parentId, status, sortOrder, COALESCE(leaderUserId, 0) AS leaderUserId FROM sys_org WHERE code = ? AND isDeleted = 0 LIMIT 1`, code)
}

func (r *Repository) FindOrgByUserID(ctx context.Context, userID int64) (*domain.OrgRecord, error) {
	return findOneRecord[domain.OrgRecord](ctx, r.executor(ctx), `
SELECT so.id, so.code, so.name, so.parentId, so.status, so.sortOrder, COALESCE(so.leaderUserId, 0) AS leaderUserId
FROM sys_org so
JOIN sys_user_org suo ON suo.orgId = so.id AND suo.userId = ? AND suo.isDeleted = 0
WHERE so.isDeleted = 0
ORDER BY suo.isPrimary DESC, so.sortOrder ASC, so.id ASC
LIMIT 1`, userID)
}

func (r *Repository) ListOrgs(ctx context.Context, enabledOnly bool) ([]domain.OrgRecord, error) {
	cond := `WHERE isDeleted = 0`
	if enabledOnly {
		cond += ` AND status = 0`
	}
	var records []domain.OrgRecord
	exec := r.executor(ctx)
	err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(`SELECT id, code, name, parentId, status, sortOrder, COALESCE(leaderUserId, 0) AS leaderUserId FROM sys_org `+cond+` ORDER BY sortOrder ASC, id ASC`))
	return records, err
}

func (r *Repository) ListOrgChildren(ctx context.Context, parentID int64) ([]domain.OrgRecord, error) {
	var records []domain.OrgRecord
	exec := r.executor(ctx)
	err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(`SELECT id, code, name, parentId, status, sortOrder, COALESCE(leaderUserId, 0) AS leaderUserId FROM sys_org WHERE parentId = ? AND isDeleted = 0 ORDER BY sortOrder ASC, id ASC`), parentID)
	return records, err
}

func (r *Repository) CountOrgCodeExcludingID(ctx context.Context, orgID int64, code string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_org WHERE code = ? AND id <> ? AND isDeleted = 0`, strings.TrimSpace(code), orgID)
}

func (r *Repository) CountOrgChildren(ctx context.Context, orgID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_org WHERE parentId = ? AND isDeleted = 0`, orgID)
}

func (r *Repository) CountDeptByOrgID(ctx context.Context, orgID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_dept WHERE orgId = ? AND isDeleted = 0`, orgID)
}

func (r *Repository) CountUserOrgByOrgID(ctx context.Context, orgID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user_org WHERE orgId = ? AND isDeleted = 0`, orgID)
}

func (r *Repository) CreateDept(ctx context.Context, record domain.DeptRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_dept (id, code, name, orgId, parentId, leaderUserId, status, sortOrder, hierarchy, level, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
		record.ID, strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), record.OrgID, record.ParentID, nullableInt64(record.LeaderUserID), record.Status, record.SortOrder, record.Hierarchy, record.Level, nullableInt64(operatorID), nullableInt64(operatorID), r.boolValue(false))
}

func (r *Repository) UpdateDept(ctx context.Context, record domain.DeptRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_dept SET code = ?, name = ?, orgId = ?, parentId = ?, leaderUserId = ?, status = ?, sortOrder = ?, hierarchy = ?, level = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), record.OrgID, record.ParentID, nullableInt64(record.LeaderUserID), record.Status, record.SortOrder, record.Hierarchy, record.Level, nullableInt64(operatorID), record.ID)
}

func (r *Repository) DeleteDept(ctx context.Context, deptID int64) error {
	return r.exec(ctx, `UPDATE sys_dept SET isDeleted = 1, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, deptID)
}

func (r *Repository) FindDeptByID(ctx context.Context, deptID int64) (*domain.DeptRecord, error) {
	return findOneRecord[domain.DeptRecord](ctx, r.executor(ctx), `SELECT id, code, name, orgId, parentId, COALESCE(leaderUserId, 0) AS leaderUserId, status, sortOrder, COALESCE(hierarchy, '') AS hierarchy, level FROM sys_dept WHERE id = ? AND isDeleted = 0 LIMIT 1`, deptID)
}

func (r *Repository) FindDeptsByIDs(ctx context.Context, deptIDs []int64) ([]domain.DeptRecord, error) {
	ids := normalizeIDs(deptIDs)
	if len(ids) == 0 {
		return []domain.DeptRecord{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, code, name, orgId, parentId, COALESCE(leaderUserId, 0) AS leaderUserId, status, sortOrder, COALESCE(hierarchy, '') AS hierarchy, level FROM sys_dept WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return nil, err
	}
	var records []domain.DeptRecord
	exec := r.executor(ctx)
	if err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("find depts by ids: %w", err)
	}
	return records, nil
}

func (r *Repository) ListDepts(ctx context.Context, enabledOnly bool, keyword string, orgID int64, status *int, limit int) ([]domain.DeptRecord, error) {
	conditions := []string{"isDeleted = 0"}
	args := make([]any, 0, 5)
	if enabledOnly {
		conditions = append(conditions, "status = 0")
	}
	if strings.TrimSpace(keyword) != "" {
		conditions = append(conditions, "(name LIKE ? OR code LIKE ?)")
		kw := "%" + strings.TrimSpace(keyword) + "%"
		args = append(args, kw, kw)
	}
	if orgID > 0 {
		conditions = append(conditions, "orgId = ?")
		args = append(args, orgID)
	}
	if status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *status)
	}
	sql := `SELECT id, code, name, orgId, parentId, COALESCE(leaderUserId, 0) AS leaderUserId, status, sortOrder, COALESCE(hierarchy, '') AS hierarchy, level FROM sys_dept WHERE ` + strings.Join(conditions, " AND ") + ` ORDER BY sortOrder ASC, id ASC`
	if limit > 0 {
		sql += ` LIMIT ?`
		args = append(args, limit)
	}
	var records []domain.DeptRecord
	exec := r.executor(ctx)
	err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(sql), args...)
	return records, err
}

func (r *Repository) ListChildDeptIDs(ctx context.Context, deptID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT id FROM sys_dept WHERE parentId = ? AND isDeleted = 0 ORDER BY id ASC`, deptID)
}

func (r *Repository) CountDeptNameUnderParent(ctx context.Context, deptID, parentID int64, name string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_dept WHERE parentId = ? AND name = ? AND id <> ? AND isDeleted = 0`, parentID, strings.TrimSpace(name), deptID)
}

func (r *Repository) CountDeptChildren(ctx context.Context, deptID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_dept WHERE parentId = ? AND isDeleted = 0`, deptID)
}

func (r *Repository) CountUserDeptByDeptID(ctx context.Context, deptID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user_dept WHERE deptId = ?`, deptID)
}

func (r *Repository) QueryPosts(ctx context.Context, query domain.PostQuery) ([]domain.PostRecord, int64, error) {
	conditions := []string{"isDeleted = 0"}
	args := make([]any, 0, 4)
	if strings.TrimSpace(query.Name) != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Name)+"%")
	}
	if strings.TrimSpace(query.Code) != "" {
		conditions = append(conditions, "code LIKE ?")
		args = append(args, "%"+strings.TrimSpace(query.Code)+"%")
	}
	if query.Status != nil {
		conditions = append(conditions, "status = ?")
		args = append(args, *query.Status)
	}
	if query.Scope.Enabled {
		scopeCondition, scopeArgs := postScopeCondition(query.Scope)
		conditions = append(conditions, scopeCondition)
		args = append(args, scopeArgs...)
	}
	where := ` WHERE ` + strings.Join(conditions, " AND ")
	total, err := r.count(ctx, `SELECT COUNT(1) FROM sys_post`+where, args...)
	if err != nil {
		return nil, 0, err
	}
	current, size := pageArgs(query.Current, query.Size)
	selectArgs := append(append([]any{}, args...), size, (current-1)*size)
	var records []domain.PostRecord
	exec := r.executor(ctx)
	err = sqlx.SelectContext(ctx, exec, &records, exec.Rebind(`SELECT id, code, name, COALESCE(deptId, 0) AS deptId, COALESCE(orgId, 0) AS orgId, sortOrder, status, COALESCE(remark, '') AS remark FROM sys_post`+where+` ORDER BY sortOrder ASC, id ASC LIMIT ? OFFSET ?`), selectArgs...)
	return records, int64(total), err
}

func postScopeCondition(scope domain.DataScopeFilter) (string, []any) {
	if scope.None {
		return "1 = 0", nil
	}
	if condition, values := inCondition("deptId", scope.DeptIDs); condition != "" {
		return condition, values
	}
	return "1 = 0", nil
}

func existsInCondition(table, idColumn, fkColumn, outerColumn string, ids []int64) (string, []any) {
	condition, args := inCondition(idColumn, ids)
	if condition == "" {
		return "", nil
	}
	return "EXISTS (SELECT 1 FROM " + table + " scoped WHERE scoped." + fkColumn + " = " + outerColumn + " AND " + strings.Replace(condition, idColumn, "scoped."+idColumn, 1) + ")", args
}

func inCondition(column string, ids []int64) (string, []any) {
	normalized := normalizeIDs(ids)
	if len(normalized) == 0 {
		return "", nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(normalized)), ",")
	args := make([]any, 0, len(normalized))
	for _, id := range normalized {
		args = append(args, id)
	}
	return column + " IN (" + placeholders + ")", args
}

func (r *Repository) ListEnabledPosts(ctx context.Context) ([]domain.PostRecord, error) {
	var records []domain.PostRecord
	exec := r.executor(ctx)
	err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(`SELECT id, code, name, COALESCE(deptId, 0) AS deptId, COALESCE(orgId, 0) AS orgId, sortOrder, status, COALESCE(remark, '') AS remark FROM sys_post WHERE status = 0 AND isDeleted = 0 ORDER BY sortOrder ASC, id ASC`))
	return records, err
}

func (r *Repository) FindPostByID(ctx context.Context, postID int64) (*domain.PostRecord, error) {
	return findOneRecord[domain.PostRecord](ctx, r.executor(ctx), `SELECT id, code, name, COALESCE(deptId, 0) AS deptId, COALESCE(orgId, 0) AS orgId, sortOrder, status, COALESCE(remark, '') AS remark FROM sys_post WHERE id = ? AND isDeleted = 0 LIMIT 1`, postID)
}

func (r *Repository) FindPostsByIDs(ctx context.Context, postIDs []int64) ([]domain.PostRecord, error) {
	ids := normalizeIDs(postIDs)
	if len(ids) == 0 {
		return []domain.PostRecord{}, nil
	}
	query, args, err := sqlx.In(`SELECT id, code, name, COALESCE(deptId, 0) AS deptId, COALESCE(orgId, 0) AS orgId, sortOrder, status, COALESCE(remark, '') AS remark FROM sys_post WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return nil, err
	}
	var records []domain.PostRecord
	exec := r.executor(ctx)
	if err := sqlx.SelectContext(ctx, exec, &records, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("find posts by ids: %w", err)
	}
	return records, nil
}

func (r *Repository) CreatePost(ctx context.Context, record domain.PostRecord, operatorID int64) error {
	return r.exec(ctx, `INSERT INTO sys_post (id, code, name, deptId, orgId, sortOrder, status, remark, creatorId, updaterId, createTime, updateTime, isDeleted) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
		record.ID, strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), nullableInt64(record.DeptID), nullableInt64(record.OrgID), record.SortOrder, record.Status, nullableString(record.Remark), nullableInt64(operatorID), nullableInt64(operatorID), r.boolValue(false))
}

func (r *Repository) UpdatePost(ctx context.Context, record domain.PostRecord, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_post SET code = ?, name = ?, deptId = ?, orgId = ?, sortOrder = ?, status = ?, remark = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`,
		strings.TrimSpace(record.Code), strings.TrimSpace(record.Name), nullableInt64(record.DeptID), nullableInt64(record.OrgID), record.SortOrder, record.Status, nullableString(record.Remark), nullableInt64(operatorID), record.ID)
}

func (r *Repository) DeletePost(ctx context.Context, postID int64) error {
	if store.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("delete post requires an active transaction")
	}
	if err := r.exec(ctx, `DELETE FROM sys_post_role WHERE postId = ?`, postID); err != nil {
		return err
	}
	return r.exec(ctx, `UPDATE sys_post SET isDeleted = 1, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, postID)
}

func (r *Repository) ListReferencedPostIDs(ctx context.Context, postIDs []int64) ([]int64, error) {
	ids := normalizeIDs(postIDs)
	if len(ids) == 0 {
		return []int64{}, nil
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("post set exceeds 100")
	}
	query, args, err := sqlx.In(`
SELECT DISTINCT postId
FROM sys_user_position
WHERE postId IN (?) AND isDeleted = 0
ORDER BY postId`, ids)
	if err != nil {
		return nil, err
	}
	exec := r.executor(ctx)
	var referenced []int64
	if err := sqlx.SelectContext(ctx, exec, &referenced, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list referenced post ids: %w", err)
	}
	return referenced, nil
}

func (r *Repository) DeletePosts(ctx context.Context, postIDs []int64) error {
	ids := normalizeIDs(postIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 100 {
		return fmt.Errorf("post set exceeds 100")
	}
	if store.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("delete posts requires an active transaction")
	}
	deleteChildren, childArgs, err := sqlx.In(`DELETE FROM sys_post_role WHERE postId IN (?)`, ids)
	if err != nil {
		return err
	}
	if err := r.exec(ctx, deleteChildren, childArgs...); err != nil {
		return err
	}
	query, args, err := sqlx.In(`UPDATE sys_post SET isDeleted = 1, updateTime = NOW() WHERE id IN (?) AND isDeleted = 0`, ids)
	if err != nil {
		return err
	}
	return r.exec(ctx, query, args...)
}

func (r *Repository) UpdatePostStatus(ctx context.Context, postID int64, status int, operatorID int64) error {
	return r.exec(ctx, `UPDATE sys_post SET status = ?, updaterId = ?, updateTime = NOW() WHERE id = ? AND isDeleted = 0`, status, nullableInt64(operatorID), postID)
}

func (r *Repository) CountPostCodeExcludingID(ctx context.Context, postID int64, code string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_post WHERE code = ? AND id <> ? AND isDeleted = 0`, strings.TrimSpace(code), postID)
}

func (r *Repository) CountPostNameExcludingID(ctx context.Context, postID int64, name string) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_post WHERE name = ? AND id <> ? AND isDeleted = 0`, strings.TrimSpace(name), postID)
}

func (r *Repository) CountUserPostByPostID(ctx context.Context, postID int64) (int, error) {
	return r.count(ctx, `SELECT COUNT(1) FROM sys_user_position WHERE postId = ? AND isDeleted = 0`, postID)
}

func (r *Repository) ListPostRoleIDs(ctx context.Context, postID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT roleId FROM sys_post_role WHERE postId = ? ORDER BY roleId ASC`, postID)
}

func (r *Repository) ReplacePostRoles(ctx context.Context, postID int64, roleIDs []int64, operatorID int64) error {
	if err := r.exec(ctx, `DELETE FROM sys_post_role WHERE postId = ?`, postID); err != nil {
		return err
	}
	ids := normalizeIDs(roleIDs)
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > 100 {
		return fmt.Errorf("post role set exceeds 100")
	}
	values := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids)*2)
	for _, id := range ids {
		values = append(values, "(?, ?)")
		args = append(args, postID, id)
	}
	return r.exec(ctx, `INSERT INTO sys_post_role (postId, roleId) VALUES `+strings.Join(values, ", "), args...)
}

func (r *Repository) ListPostIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return r.listIDs(ctx, `SELECT postId FROM sys_post_role WHERE roleId = ? ORDER BY postId ASC`, roleID)
}

func (r *Repository) ListUserIDsByPostIDPage(ctx context.Context, postID, afterUserID int64, limit int) ([]int64, error) {
	if postID <= 0 {
		return []int64{}, nil
	}
	if afterUserID < 0 {
		afterUserID = 0
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if r.isPostgres() {
		return r.listIDs(ctx, `
SELECT "userId"
FROM sys_user_position
WHERE "postId" = ? AND "userId" > ? AND "isDeleted" = FALSE
ORDER BY "userId" ASC
LIMIT ?`, postID, afterUserID, limit)
	}
	return r.listIDs(ctx, `
SELECT userId
FROM sys_user_position
WHERE postId = ? AND userId > ? AND isDeleted = 0
ORDER BY userId ASC
LIMIT ?`, postID, afterUserID, limit)
}

func (r *Repository) count(ctx context.Context, query string, args ...any) (int, error) {
	exec := r.executor(ctx)
	var count int
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(query), args...); err != nil {
		return 0, fmt.Errorf("count system user: %w", err)
	}
	return count, nil
}

func (r *Repository) exec(ctx context.Context, query string, args ...any) error {
	exec := r.executor(ctx)
	if _, err := exec.ExecContext(ctx, exec.Rebind(query), args...); err != nil {
		return fmt.Errorf("exec system user command: %w", err)
	}
	return nil
}

func (r *Repository) boolValue(value bool) any {
	if r.postgres {
		return value
	}
	if value {
		return 1
	}
	return 0
}

// genderProjection keeps the public admin contract numeric while PostgreSQL
// stores the underlying field as BOOLEAN. The identifier argument is always a
// source-controlled repository column, never request input.
func (r *Repository) genderProjection(identifier string) string {
	if r != nil && r.postgres {
		return "CASE WHEN " + identifier + " THEN 1 ELSE 0 END AS userGender"
	}
	return identifier
}

func (r *Repository) listIDs(ctx context.Context, query string, args ...any) ([]int64, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	exec := r.executor(ctx)
	var ids []int64
	if err := sqlx.SelectContext(ctx, exec, &ids, exec.Rebind(query), args...); err != nil {
		return nil, fmt.Errorf("list system user relation ids: %w", err)
	}
	return ids, nil
}

func relationPrimary(id int64, primaryID *int64) int {
	if primaryID == nil {
		return 0
	}
	if id == *primaryID || *primaryID <= 0 {
		*primaryID = id
		return 1
	}
	return 0
}

func findOneRecord[T any](ctx context.Context, db store.SQLX, query string, args ...any) (*T, error) {
	if db == nil {
		return nil, nil
	}
	exec := db
	var result T
	err := sqlx.GetContext(ctx, exec, &result, exec.Rebind(query), args...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query system user record: %w", err)
	}
	return &result, nil
}

func pageArgs(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	if size > 200 {
		size = 200
	}
	return current, size
}

func normalizeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func nullableInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func baseSubjectQuery() string {
	return `
SELECT
	id AS user_id,
	userAccount AS account_name,
	nickName AS nick_name,
	userEmail AS email,
	COALESCE(userPhone, '') AS phone,
	COALESCE(userAvatar, '') AS avatar,
	COALESCE(userProfile, '') AS profile,
	status AS status,
	unsealTime AS unseal_at,
	email_verified_at,
	email_version,
	email_verification_source
FROM sys_user`
}

func nullableString(value string) any {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}
