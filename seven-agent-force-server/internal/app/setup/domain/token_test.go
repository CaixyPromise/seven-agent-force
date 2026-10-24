package domain

import (
	"strings"
	"testing"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func TestSetupTokenGenerateValidateAndRejectReplayEpochExpiry(t *testing.T) {
	now := time.Unix(1713830400, 0).UTC()
	service, err := NewTokenService(strings.Repeat("s", 32), 300, 12345)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	token, err := service.Generate(now)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	payload, err := service.Validate(token, now.Add(time.Second))
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if payload.Nonce == "" || payload.StartupEpoch != 12345 || payload.Exp != now.Add(300*time.Second).Unix() {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if _, err := service.Validate(token, now.Add(301*time.Second)); apperrors.From(err).Code() != apperrors.CodeNoAuth {
		t.Fatalf("expected expired token no-auth error, got %v", err)
	}
	otherEpoch, err := NewTokenService(strings.Repeat("s", 32), 300, 67890)
	if err != nil {
		t.Fatalf("new other epoch service: %v", err)
	}
	if _, err := otherEpoch.Validate(token, now); apperrors.From(err).Code() != apperrors.CodeNoAuth {
		t.Fatalf("expected startup epoch mismatch no-auth error, got %v", err)
	}
}

func TestSetupTokenRejectsWeakConfiguredSecret(t *testing.T) {
	if _, err := NewTokenService("short", 300, 1); err == nil {
		t.Fatal("expected weak token secret to be rejected")
	}
}

func TestValidateOriginMatchesJavaRules(t *testing.T) {
	input := OriginCheckInput{
		Origin:                "http://127.0.0.1:5177/page",
		AllowedOriginPatterns: []string{"http://127.0.0.1:*", "http://localhost:*"},
		RequireOriginHeader:   true,
	}
	if !ValidateOrigin(input) {
		t.Fatal("expected configured localhost origin to pass")
	}
	input.Origin = "http://evil.example"
	if ValidateOrigin(input) {
		t.Fatal("expected untrusted origin to fail")
	}
	input.Origin = ""
	input.SecFetchSite = "same-origin"
	input.SecFetchMode = "cors"
	if ValidateOrigin(input) {
		t.Fatal("expected missing origin to fail when origin header is required, even with fetch metadata")
	}
	input.RequireOriginHeader = false
	if !ValidateOrigin(input) {
		t.Fatal("expected trusted fetch metadata to pass only when origin header is not required")
	}
	input.SecFetchSite = ""
	if ValidateOrigin(input) {
		t.Fatal("expected missing origin and fetch metadata to fail")
	}
}
