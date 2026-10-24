package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("system admin repository requires datasource provider")
	}
	return &Repository{
		db:       provider.SQLX(),
		postgres: strings.Contains(strings.ToLower(provider.Driver()), "postgres") || strings.Contains(strings.ToLower(provider.Driver()), "pgx"),
	}, nil
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return adminDialectExecutor{SQLX: store.SQLXExecutor(ctx, r.db), postgres: r.postgres}
}

func (r *Repository) requireExecutor(ctx context.Context) (store.SQLX, error) {
	exec := r.executor(ctx)
	if exec == nil {
		return nil, fmt.Errorf("system admin repository datasource is not configured")
	}
	return exec, nil
}

type operationLogRow struct {
	ID              int64          `db:"id"`
	UserID          sql.NullInt64  `db:"userId"`
	UserName        sql.NullString `db:"userName"`
	NickName        sql.NullString `db:"nickName"`
	OperationType   string         `db:"operationType"`
	OperationDesc   sql.NullString `db:"operationDesc"`
	MethodName      sql.NullString `db:"methodName"`
	RequestMethod   sql.NullString `db:"requestMethod"`
	RequestURL      sql.NullString `db:"requestUrl"`
	TraceID         sql.NullString `db:"traceId"`
	RequestParams   sql.NullString `db:"requestParams"`
	ResponseResult  sql.NullString `db:"responseResult"`
	RequestIP       sql.NullString `db:"requestIp"`
	RequestLocation sql.NullString `db:"requestLocation"`
	UserAgent       sql.NullString `db:"userAgent"`
	Browser         sql.NullString `db:"browser"`
	OS              sql.NullString `db:"os"`
	OperationTime   sql.NullTime   `db:"operationTime"`
	ExecutionTime   sql.NullInt64  `db:"executionTime"`
	Status          int            `db:"status"`
	ErrorMsg        sql.NullString `db:"errorMsg"`
	CreateTime      sql.NullTime   `db:"createTime"`
	UpdateTime      sql.NullTime   `db:"updateTime"`
	IsDeleted       bool           `db:"isDeleted"`
}

func (r *Repository) InsertOperationLog(ctx context.Context, item *domain.OperationLog) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := exec.Rebind(`
INSERT INTO sys_operation_log (
	userId, userName, nickName, operationType, operationDesc, methodName, requestMethod, requestUrl,
	traceId, requestParams, responseResult, requestIp, requestLocation, userAgent, browser, os, operationTime,
	executionTime, status, errorMsg, createTime, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	args := []any{
		nullIfZero(item.UserID),
		nullIfBlank(item.UserName),
		nullIfBlank(item.NickName),
		strings.TrimSpace(item.OperationType),
		nullIfBlank(item.OperationDesc),
		nullIfBlank(item.MethodName),
		nullIfBlank(item.RequestMethod),
		nullIfBlank(item.RequestURL),
		nullIfBlank(item.TraceID),
		nullIfBlank(item.RequestParams),
		nullIfBlank(item.ResponseResult),
		nullIfBlank(item.RequestIP),
		nullIfBlank(item.RequestLocation),
		nullIfBlank(item.UserAgent),
		nullIfBlank(item.Browser),
		nullIfBlank(item.OS),
		timeOrNow(item.OperationTime),
		nullIfZero(item.ExecutionTime),
		item.Status,
		nullIfBlank(item.ErrorMsg),
		timeOrNow(item.CreateTime),
		timeOrNow(item.UpdateTime),
		r.boolValue(item.IsDeleted != 0),
	}
	if r.postgres {
		var id int64
		if err := exec.QueryRowxContext(ctx, query+` RETURNING id`, args...).Scan(&id); err != nil {
			return 0, fmt.Errorf("insert operation log: %w", err)
		}
		return id, nil
	}
	result, err := exec.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("insert operation log: %w", err)
	}
	return lastInsertID(result)
}

func (r *Repository) FindOperationLogByID(ctx context.Context, id int64) (*domain.OperationLog, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row operationLogRow
	query := exec.Rebind(`
SELECT id, userId, userName, nickName, operationType, operationDesc, methodName, requestMethod, requestUrl,
	traceId, requestParams, responseResult, requestIp, requestLocation, userAgent, browser, os, operationTime, executionTime,
	status, errorMsg, createTime, updateTime, isDeleted
FROM sys_operation_log
WHERE id = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find operation log by id: %w", err)
	}
	item := mapOperationLog(row)
	return &item, nil
}

func (r *Repository) QueryOperationLogs(ctx context.Context, query domain.OperationLogPageQuery) ([]domain.OperationLog, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	current := query.Current
	if current <= 0 {
		current = 1
	}
	size := query.Size
	if size <= 0 {
		size = 10
	}
	whereParts := []string{"isDeleted = 0"}
	args := make([]any, 0, 8)
	if query.UserID > 0 {
		whereParts = append(whereParts, "userId = ?")
		args = append(args, query.UserID)
	}
	if opType := strings.TrimSpace(query.OperationType); opType != "" {
		whereParts = append(whereParts, "operationType = ?")
		args = append(args, opType)
	}
	if username := strings.TrimSpace(query.Username); username != "" {
		whereParts = append(whereParts, "userName LIKE ?")
		args = append(args, "%"+username+"%")
	}
	if method := strings.TrimSpace(query.RequestMethod); method != "" {
		whereParts = append(whereParts, "requestMethod = ?")
		args = append(args, strings.ToUpper(method))
	}
	if requestURL := strings.TrimSpace(query.RequestURL); requestURL != "" {
		whereParts = append(whereParts, "requestUrl LIKE ?")
		args = append(args, "%"+requestURL+"%")
	}
	if query.ExecutionTimeMin != nil {
		whereParts = append(whereParts, "executionTime >= ?")
		args = append(args, *query.ExecutionTimeMin)
	}
	if query.ExecutionTimeMax != nil {
		whereParts = append(whereParts, "executionTime <= ?")
		args = append(args, *query.ExecutionTimeMax)
	}
	if query.StartTime != nil {
		whereParts = append(whereParts, "operationTime >= ?")
		args = append(args, query.StartTime.UTC())
	}
	if query.EndTime != nil {
		whereParts = append(whereParts, "operationTime <= ?")
		args = append(args, query.EndTime.UTC())
	}
	whereSQL := strings.Join(whereParts, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, exec.Rebind(`SELECT COUNT(1) FROM sys_operation_log WHERE `+whereSQL), args...); err != nil {
		return nil, 0, fmt.Errorf("count operation logs: %w", err)
	}
	if total == 0 {
		return []domain.OperationLog{}, 0, nil
	}
	offset := (current - 1) * size
	rows := make([]operationLogRow, 0)
	querySQL := exec.Rebind(`
SELECT id, userId, userName, nickName, operationType, operationDesc, methodName, requestMethod, requestUrl,
	traceId, requestParams, responseResult, requestIp, requestLocation, userAgent, browser, os, operationTime, executionTime,
	status, errorMsg, createTime, updateTime, isDeleted
FROM sys_operation_log
WHERE ` + whereSQL + `
ORDER BY operationTime DESC, id DESC
LIMIT ? OFFSET ?`)
	queryArgs := append(append([]any{}, args...), size, offset)
	if err := sqlx.SelectContext(ctx, exec, &rows, querySQL, queryArgs...); err != nil {
		return nil, 0, fmt.Errorf("query operation logs: %w", err)
	}
	items := make([]domain.OperationLog, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapOperationLog(row))
	}
	return items, total, nil
}

func (r *Repository) DeleteOperationLogsBeforeDays(ctx context.Context, days int) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	before := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	result, err := exec.ExecContext(ctx, exec.Rebind(`DELETE FROM sys_operation_log WHERE operationTime < ?`), before)
	if err != nil {
		return 0, fmt.Errorf("clean operation logs: %w", err)
	}
	return result.RowsAffected()
}

func (r *Repository) DeleteOperationLogsByTimeRange(ctx context.Context, start, end string) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, exec.Rebind(`DELETE FROM sys_operation_log WHERE operationTime >= ? AND operationTime <= ?`), start, end)
	if err != nil {
		return 0, fmt.Errorf("delete operation logs by time range: %w", err)
	}
	return result.RowsAffected()
}

func mapOperationLog(row operationLogRow) domain.OperationLog {
	return domain.OperationLog{
		ID:              row.ID,
		UserID:          nullableInt64(row.UserID),
		UserName:        nullableString(row.UserName),
		NickName:        nullableString(row.NickName),
		OperationType:   strings.TrimSpace(row.OperationType),
		OperationDesc:   nullableString(row.OperationDesc),
		MethodName:      nullableString(row.MethodName),
		RequestMethod:   nullableString(row.RequestMethod),
		RequestURL:      nullableString(row.RequestURL),
		TraceID:         nullableString(row.TraceID),
		RequestParams:   nullableString(row.RequestParams),
		ResponseResult:  nullableString(row.ResponseResult),
		RequestIP:       nullableString(row.RequestIP),
		RequestLocation: nullableString(row.RequestLocation),
		UserAgent:       nullableString(row.UserAgent),
		Browser:         nullableString(row.Browser),
		OS:              nullableString(row.OS),
		OperationTime:   nullableTime(row.OperationTime),
		ExecutionTime:   nullableInt64(row.ExecutionTime),
		Status:          row.Status,
		ErrorMsg:        nullableString(row.ErrorMsg),
		CreateTime:      nullableTime(row.CreateTime),
		UpdateTime:      nullableTime(row.UpdateTime),
		IsDeleted:       boolToInt(row.IsDeleted),
	}
}

func (r *Repository) boolValue(value bool) any {
	if r != nil && r.postgres {
		return value
	}
	return boolToInt(value)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return strings.TrimSpace(value.String)
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func nullableInt64(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nullIfZero(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func timeOrNow(value *time.Time) time.Time {
	if value != nil {
		return value.UTC()
	}
	return time.Now().UTC()
}

func lastInsertID(result sql.Result) (int64, error) {
	if result == nil {
		return 0, fmt.Errorf("last insert id unavailable")
	}
	id, err := result.LastInsertId()
	if err == nil {
		return id, nil
	}
	rows, rowErr := result.RowsAffected()
	if rowErr == nil && rows > 0 {
		return rows, nil
	}
	return 0, err
}
