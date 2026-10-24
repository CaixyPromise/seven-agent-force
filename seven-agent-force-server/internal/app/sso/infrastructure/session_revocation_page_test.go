package infrastructure

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestListActiveSessionsByExternalProviderUsesBoundedKeysetPage(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	cutoff := time.Now().UTC()

	mock.ExpectQuery(regexp.QuoteMeta(`WHERE externalProviderCode = ? AND status = ? AND isDeleted = 0 AND createTime <= ?
  AND id > ?
ORDER BY id ASC
LIMIT ?`)).
		WithArgs("github", domain.SessionStatusActive, cutoff, int64(41), 100).
		WillReturnRows(sqlmock.NewRows(sessionRevocationColumns()).
			AddRow(int64(42), "session-42", int64(1001), "client-a", nil, nil, nil, nil, nil, nil, "EXTERNAL_OAUTH", "github", int64(501), time.Now(), nil, time.Now().Add(time.Hour), nil, domain.SessionStatusActive, nil, time.Now(), time.Now()))

	items, err := repo.ListActiveSessionsByExternalProviderPage(context.Background(), " github ", cutoff, 41, 1000)
	if err != nil {
		t.Fatalf("ListActiveSessionsByExternalProviderPage() error=%v", err)
	}
	if len(items) != 1 || items[0].ID != 42 {
		t.Fatalf("items=%#v", items)
	}
	assertSQLExpectations(t, mock)
}

func sessionRevocationColumns() []string {
	return []string{
		"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson",
		"loginMethod", "externalProviderCode", "externalIdentityId",
		"loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime",
	}
}
