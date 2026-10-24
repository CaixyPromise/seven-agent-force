package mysql

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestFindSingleActive(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewUserCredentialRepository(db)
	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "userId", "credentialType", "credentialKey", "secretHash", "secretCiphertext", "credentialPayloadJson", "status",
		"verifiedAt", "lastUsedAt", "invalidatedAt", "metadataJson", "mustChangePassword", "passwordChangedAt",
		"creatorId", "createTime", "updaterId", "updateTime", "isDeleted",
	}).AddRow(1, 2, "PASSWORD", "PRIMARY", "hash", nil, nil, 0, now, nil, nil, nil, 1, now, nil, now, nil, now, 0)

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT id, userId, credentialType, credentialKey, secretHash, secretCiphertext, credentialPayloadJson, status,
       verifiedAt, lastUsedAt, invalidatedAt, metadataJson, mustChangePassword, passwordChangedAt,
       creatorId, createTime, updaterId, updateTime, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND credentialKey = ? AND status = ? AND isDeleted = 0
LIMIT 1`)).
		WithArgs(int64(2), "PASSWORD", "PRIMARY", 0).
		WillReturnRows(rows)

	record, err := repo.FindSingleActive(context.Background(), 2, domain.CredentialTypePassword, domain.PrimaryCredentialKey)
	if err != nil {
		t.Fatalf("find single active: %v", err)
	}
	if record == nil || record.SecretHash != "hash" || !record.MustChangePassword {
		t.Fatalf("unexpected record: %#v", record)
	}
}

func TestConsumeRecoveryCodeByID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	defer db.Close()

	repo := NewUserCredentialRepository(db)
	usedAt := time.Now()
	mock.ExpectExec(regexp.QuoteMeta(`
UPDATE sys_user_credential
SET status = ?, lastUsedAt = ?, invalidatedAt = ?, updateTime = ?
WHERE id = ? AND status = ? AND isDeleted = 0`)).
		WithArgs(2, usedAt, usedAt, usedAt, int64(7), 0).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := repo.ConsumeRecoveryCodeByID(context.Background(), 7, usedAt)
	if err != nil {
		t.Fatalf("consume recovery code by id: %v", err)
	}
	if !ok {
		t.Fatalf("expected consumed=true")
	}
}
