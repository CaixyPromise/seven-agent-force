package totp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	libtotp "github.com/pquerna/otp/totp"
)

func TestGenerateSecretAndBuildURI(t *testing.T) {
	service := New()
	enrollment, err := service.GenerateSecret(context.Background(), "SevenFramework", "demo@example.com")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	if enrollment.Secret == "" {
		t.Fatalf("expected non-empty secret")
	}
	uri, err := service.BuildKeyURI(enrollment.Secret, "SevenFramework", "demo@example.com")
	if err != nil {
		t.Fatalf("build uri: %v", err)
	}
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("unexpected uri: %s", uri)
	}
}

func TestBuildURIEncodesIssuerAndAccount(t *testing.T) {
	service := New()
	uri, err := service.BuildKeyURI("JBSWY3DPEHPK3PXP", "Seven Framework & Team", "demo:user@example.com")
	if err != nil {
		t.Fatalf("build uri: %v", err)
	}
	if !strings.Contains(uri, "Seven%20Framework%20%26%20Team:demo%3Auser%40example.com") {
		t.Fatalf("expected encoded label in uri, got %s", uri)
	}
	if !strings.Contains(uri, "issuer=Seven%20Framework%20%26%20Team") {
		t.Fatalf("expected encoded issuer query in uri, got %s", uri)
	}
}

func TestVerifyCode(t *testing.T) {
	service := New()
	ts := time.Unix(1_700_000_000, 0)
	code, err := libtotp.GenerateCodeCustom("JBSWY3DPEHPK3PXP", ts, libtotp.ValidateOpts{
		Period:    DefaultPeriod,
		Skew:      0,
		Digits:    DefaultDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("generate totp code: %v", err)
	}
	if !service.VerifyCode("JBSWY3DPEHPK3PXP", code, ts, DefaultSkew) {
		t.Fatalf("expected code verification to succeed")
	}
	if service.VerifyCode("JBSWY3DPEHPK3PXP", "000000", ts, DefaultSkew) {
		t.Fatalf("expected invalid code verification to fail")
	}
}
