package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestDownloadTokenIssueVerifyAndExpire(t *testing.T) {
	service, err := NewDownloadTokenService(config.FileDistributionConfig{
		SignedURLSecret:     "01234567890123456789012345678901",
		SignedURLTTLSeconds: 60,
		AllowIPBind:         true,
	}, nil)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	service.nowFunc = func() time.Time { return now }
	token, err := service.Issue(context.Background(), 1001, 2002, "org:22", "127.0.0.1")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	claims, err := service.Verify(context.Background(), token, "127.0.0.1")
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}
	if claims.FileID != 1001 || claims.UserID != 2002 || claims.ScopeID != "org:22" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if _, err := service.Verify(context.Background(), token, "127.0.0.2"); err == nil {
		t.Fatal("expected ip mismatch to be rejected")
	}
	service.nowFunc = func() time.Time { return now.Add(61 * time.Second) }
	if _, err := service.Verify(context.Background(), token, "127.0.0.1"); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}

func TestDownloadTokenRejectsWeakSecret(t *testing.T) {
	if _, err := NewDownloadTokenService(config.FileDistributionConfig{
		SignedURLSecret:     "weak",
		SignedURLTTLSeconds: 60,
	}, nil); err == nil {
		t.Fatal("expected weak secret to be rejected")
	}
}
