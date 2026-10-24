package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
)

type UserCredentialRepository struct {
	db store.DBTX
}

func NewUserCredentialRepository(db store.DBTX) *UserCredentialRepository {
	return &UserCredentialRepository{db: db}
}

func (r *UserCredentialRepository) executor(ctx context.Context) store.DBTX {
	return store.Executor(ctx, r.db)
}

func (r *UserCredentialRepository) FindSingleActive(ctx context.Context, userID int64, credentialType domain.CredentialType, credentialKey string) (*domain.CredentialRecord, error) {
	return r.findOne(ctx, `
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND credentialKey = ? AND status = ? AND isDeleted = 0
LIMIT 1`, userID, string(credentialType), credentialKey, int(domain.CredentialStatusActive))
}

func (r *UserCredentialRepository) FindSingleAny(ctx context.Context, userID int64, credentialType domain.CredentialType, credentialKey string) (*domain.CredentialRecord, error) {
	return r.findOne(ctx, `
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND credentialKey = ? AND isDeleted = 0
LIMIT 1`, userID, string(credentialType), credentialKey)
}

func (r *UserCredentialRepository) FindAnyByTypeAndKey(ctx context.Context, credentialType domain.CredentialType, credentialKey string) (*domain.CredentialRecord, error) {
	return r.findOne(ctx, `
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE credentialType = ? AND credentialKey = ? AND isDeleted = 0
LIMIT 1`, string(credentialType), credentialKey)
}

func (r *UserCredentialRepository) ListActiveByUserAndType(ctx context.Context, userID int64, credentialType domain.CredentialType) ([]domain.CredentialRecord, error) {
	return r.list(ctx, `
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND status = ? AND isDeleted = 0
ORDER BY createTime ASC`, userID, string(credentialType), int(domain.CredentialStatusActive))
}

func (r *UserCredentialRepository) CountActiveByUserAndType(ctx context.Context, userID int64, credentialType domain.CredentialType) (int, error) {
	var count int
	if err := r.executor(ctx).QueryRowContext(ctx, `
SELECT COUNT(*)
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND status = ? AND isDeleted = 0`,
		userID, string(credentialType), int(domain.CredentialStatusActive)).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active user credentials: %w", err)
	}
	return count, nil
}

func (r *UserCredentialRepository) Insert(ctx context.Context, record *domain.CredentialRecord) error {
	if record == nil {
		return fmt.Errorf("credential record must not be nil")
	}
	_, err := r.executor(ctx).ExecContext(ctx, `
INSERT INTO sys_user_credential (
    id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson,
    status, verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
    creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID,
		record.UserID,
		string(record.CredentialType),
		record.CredentialKey,
		nullIfBlank(record.SecretHash),
		nullIfBlank(record.SecretCiphertext),
		nullIfBlank(record.CredentialPayloadJSON),
		int(record.Status),
		record.VerifiedAt,
		record.LastUsedAt,
		record.InvalidatedAt,
		nullIfBlank(record.MetadataJSON),
		boolToInt(record.MustChangePassword),
		record.PasswordChangedAt,
		record.CreatorID,
		record.CreateTime,
		record.UpdaterID,
		record.UpdateTime,
		record.IsDeleted,
	)
	if err != nil {
		return fmt.Errorf("insert user credential: %w", err)
	}
	return nil
}

func (r *UserCredentialRepository) Update(ctx context.Context, record *domain.CredentialRecord) error {
	if record == nil {
		return fmt.Errorf("credential record must not be nil")
	}
	_, err := r.executor(ctx).ExecContext(ctx, `
UPDATE sys_user_credential
SET userId = ?, credentialType = ?, credentialKey = ?, secretHash = ?, secretCiphertext = ?, credentialPayloadJson = ?,
    status = ?, verifiedAt = ?, lastUsedAt = ?, invalidatedAt = ?, metadataJson = ?, mustChangePassword = ?,
    passwordChangedAt = ?, updaterId = ?, updateTime = ?
WHERE id = ? AND isDeleted = 0`,
		record.UserID,
		string(record.CredentialType),
		record.CredentialKey,
		nullIfBlank(record.SecretHash),
		nullIfBlank(record.SecretCiphertext),
		nullIfBlank(record.CredentialPayloadJSON),
		int(record.Status),
		record.VerifiedAt,
		record.LastUsedAt,
		record.InvalidatedAt,
		nullIfBlank(record.MetadataJSON),
		boolToInt(record.MustChangePassword),
		record.PasswordChangedAt,
		record.UpdaterID,
		record.UpdateTime,
		record.ID,
	)
	if err != nil {
		return fmt.Errorf("update user credential: %w", err)
	}
	return nil
}

func (r *UserCredentialRepository) UpdateStatusByScope(ctx context.Context, userID int64, credentialType domain.CredentialType, credentialKey string, fromStatus domain.CredentialStatus, toStatus domain.CredentialStatus, invalidatedAt time.Time) (int64, error) {
	result, err := r.executor(ctx).ExecContext(ctx, `
UPDATE sys_user_credential
SET status = ?, invalidatedAt = ?, updateTime = ?
WHERE userId = ? AND credentialType = ? AND credentialKey = ? AND status = ? AND isDeleted = 0`,
		int(toStatus), invalidatedAt, invalidatedAt, userID, string(credentialType), credentialKey, int(fromStatus),
	)
	if err != nil {
		return 0, fmt.Errorf("update user credential status: %w", err)
	}
	return result.RowsAffected()
}

func (r *UserCredentialRepository) UpdateLastUsedByScope(ctx context.Context, userID int64, credentialType domain.CredentialType, credentialKey string, status domain.CredentialStatus, usedAt time.Time) error {
	_, err := r.executor(ctx).ExecContext(ctx, `
UPDATE sys_user_credential
SET lastUsedAt = ?, updateTime = ?
WHERE userId = ? AND credentialType = ? AND credentialKey = ? AND status = ? AND isDeleted = 0`,
		usedAt, usedAt, userID, string(credentialType), credentialKey, int(status),
	)
	if err != nil {
		return fmt.Errorf("update credential last used time: %w", err)
	}
	return nil
}

func (r *UserCredentialRepository) ListActiveRecoveryCodes(ctx context.Context, userID int64) ([]domain.CredentialRecord, error) {
	return r.list(ctx, `
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND status = ? AND isDeleted = 0
ORDER BY createTime ASC`, userID, string(domain.CredentialTypeRecoveryCode), int(domain.CredentialStatusActive))
}

func (r *UserCredentialRepository) InvalidateActiveRecoveryCodes(ctx context.Context, userID int64, invalidatedAt time.Time) (int64, error) {
	result, err := r.executor(ctx).ExecContext(ctx, `
UPDATE sys_user_credential
SET status = ?, invalidatedAt = ?, updateTime = ?
WHERE userId = ? AND credentialType = ? AND status = ? AND isDeleted = 0`,
		int(domain.CredentialStatusInvalidated), invalidatedAt, invalidatedAt, userID, string(domain.CredentialTypeRecoveryCode), int(domain.CredentialStatusActive),
	)
	if err != nil {
		return 0, fmt.Errorf("invalidate active recovery codes: %w", err)
	}
	return result.RowsAffected()
}

func (r *UserCredentialRepository) ConsumeRecoveryCodeByID(ctx context.Context, id int64, usedAt time.Time) (bool, error) {
	result, err := r.executor(ctx).ExecContext(ctx, `
UPDATE sys_user_credential
SET status = ?, lastUsedAt = ?, invalidatedAt = ?, updateTime = ?
WHERE id = ? AND status = ? AND isDeleted = 0`,
		int(domain.CredentialStatusConsumed), usedAt, usedAt, usedAt, id, int(domain.CredentialStatusActive),
	)
	if err != nil {
		return false, fmt.Errorf("consume recovery code: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read consume recovery code result: %w", err)
	}
	return rows > 0, nil
}

func (r *UserCredentialRepository) findOne(ctx context.Context, query string, args ...any) (*domain.CredentialRecord, error) {
	row := r.executor(ctx).QueryRowContext(ctx, query, args...)
	record, err := scanCredential(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return record, nil
}

func (r *UserCredentialRepository) list(ctx context.Context, query string, args ...any) ([]domain.CredentialRecord, error) {
	rows, err := r.executor(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query user credentials: %w", err)
	}
	defer rows.Close()

	records := make([]domain.CredentialRecord, 0)
	for rows.Next() {
		record, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user credentials: %w", err)
	}
	return records, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCredential(scanner rowScanner) (*domain.CredentialRecord, error) {
	var (
		record                domain.CredentialRecord
		credentialType        string
		secretHash            sql.NullString
		secretCiphertext      sql.NullString
		credentialPayloadJSON sql.NullString
		verifiedAt            sql.NullTime
		lastUsedAt            sql.NullTime
		invalidatedAt         sql.NullTime
		metadataJSON          sql.NullString
		mustChangePassword    sql.NullInt64
		passwordChangedAt     sql.NullTime
		creatorID             sql.NullInt64
		createTime            sql.NullTime
		updaterID             sql.NullInt64
		updateTime            sql.NullTime
	)
	if err := scanner.Scan(
		&record.ID,
		&record.UserID,
		&credentialType,
		&record.CredentialKey,
		&secretHash,
		&secretCiphertext,
		&credentialPayloadJSON,
		&record.Status,
		&verifiedAt,
		&lastUsedAt,
		&invalidatedAt,
		&metadataJSON,
		&mustChangePassword,
		&passwordChangedAt,
		&creatorID,
		&createTime,
		&updaterID,
		&updateTime,
		&record.IsDeleted,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("scan user credential: %w", err)
	}
	record.CredentialType = domain.CredentialType(credentialType)
	record.SecretHash = nullableString(secretHash)
	record.SecretCiphertext = nullableString(secretCiphertext)
	record.CredentialPayloadJSON = nullableString(credentialPayloadJSON)
	record.VerifiedAt = nullableTime(verifiedAt)
	record.LastUsedAt = nullableTime(lastUsedAt)
	record.InvalidatedAt = nullableTime(invalidatedAt)
	record.MetadataJSON = nullableString(metadataJSON)
	record.MustChangePassword = mustChangePassword.Valid && mustChangePassword.Int64 == 1
	record.PasswordChangedAt = nullableTime(passwordChangedAt)
	record.CreatorID = nullableInt64(creatorID)
	record.CreateTime = nullableTime(createTime)
	record.UpdaterID = nullableInt64(updaterID)
	record.UpdateTime = nullableTime(updateTime)
	return &record, nil
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	ts := value.Time.UTC()
	return &ts
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
