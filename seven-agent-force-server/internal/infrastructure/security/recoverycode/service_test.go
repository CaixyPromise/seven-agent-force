package recoverycode

import (
	"strings"
	"testing"
)

func TestGenerateCodes(t *testing.T) {
	service := New()
	codes, err := service.GenerateCodes(3)
	if err != nil {
		t.Fatalf("generate codes: %v", err)
	}
	if len(codes) != 3 {
		t.Fatalf("expected 3 codes, got %d", len(codes))
	}
	for _, code := range codes {
		if strings.ToUpper(code) != code {
			t.Fatalf("expected uppercase code, got %s", code)
		}
		if !strings.Contains(code, "-") {
			t.Fatalf("expected grouped code, got %s", code)
		}
	}
}

func TestHashAndVerify(t *testing.T) {
	service := New()
	salt, err := service.GenerateSalt()
	if err != nil {
		t.Fatalf("generate salt: %v", err)
	}
	hash, err := service.HashCode("ABCD-EFGH-IJKL", salt, DefaultIterationCount)
	if err != nil {
		t.Fatalf("hash code: %v", err)
	}
	if !service.VerifyCode("abcdefghijkl", salt, DefaultIterationCount, hash) {
		t.Fatalf("expected verify to succeed after normalization")
	}
	if service.VerifyCode("WRONG-CODE", salt, DefaultIterationCount, hash) {
		t.Fatalf("expected verify to fail for wrong code")
	}
}
