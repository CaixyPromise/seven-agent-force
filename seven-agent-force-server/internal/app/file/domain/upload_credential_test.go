package domain

import (
	"testing"
	"time"
)

func TestUploadCredentialAuthorityIsUserScopeFileAndTimeBound(t *testing.T) {
	now := time.Date(2026, 7, 30, 4, 0, 0, 0, time.UTC)
	expireAt := now.Add(time.Hour)
	credential := UploadTask{
		UserID:             1001,
		ScopeID:            "node:sg-1",
		FileID:             42,
		Status:             UploadTaskClean,
		CredentialID:       "credential-42",
		CredentialVersion:  UploadCredentialVersion1,
		CredentialExpireAt: &expireAt,
	}
	if !credential.Authorizes(1001, "node:sg-1", 42, now) {
		t.Fatal("completed current credential should authorize its exact subject")
	}
	for _, candidate := range []struct {
		name    string
		userID  int64
		scopeID string
		fileID  int64
		at      time.Time
	}{
		{name: "other user", userID: 1002, scopeID: "node:sg-1", fileID: 42, at: now},
		{name: "other scope", userID: 1001, scopeID: "node:sg-2", fileID: 42, at: now},
		{name: "other file", userID: 1001, scopeID: "node:sg-1", fileID: 43, at: now},
		{name: "expired", userID: 1001, scopeID: "node:sg-1", fileID: 42, at: expireAt},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			if credential.Authorizes(candidate.userID, candidate.scopeID, candidate.fileID, candidate.at) {
				t.Fatal("credential unexpectedly authorized mismatched or expired subject")
			}
		})
	}
}

func TestHistoricalAndRevokedUploadTasksNeverAuthorize(t *testing.T) {
	now := time.Now().UTC()
	expireAt := now.Add(time.Hour)
	base := UploadTask{
		UserID:             1001,
		ScopeID:            "local",
		FileID:             42,
		Status:             UploadTaskClean,
		CredentialID:       "credential-42",
		CredentialVersion:  UploadCredentialVersion1,
		CredentialExpireAt: &expireAt,
	}

	historical := base
	historical.CredentialVersion = 0
	if historical.Authorizes(1001, "local", 42, now) {
		t.Fatal("historical version-zero task must not gain authority")
	}

	revoked := base
	revokedAt := now.Add(-time.Second)
	revoked.RevokedAt = &revokedAt
	if revoked.Authorizes(1001, "local", 42, now) {
		t.Fatal("revoked credential must not authorize")
	}
}
