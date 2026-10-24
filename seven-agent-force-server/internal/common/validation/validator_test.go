package validation

import (
	"testing"
	"time"
)

func TestValidateStructRequired(t *testing.T) {
	type request struct {
		Name string `json:"name" validate:"required"`
	}

	err := ValidateStruct(&request{})
	if err == nil {
		t.Fatal("expected validation error")
	}

	typed, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("unexpected error type: %T", err)
	}
	if len(typed.Violations) != 1 {
		t.Fatalf("unexpected violations: %+v", typed.Violations)
	}
	if typed.Violations[0].Field != "name" {
		t.Fatalf("unexpected field: %+v", typed.Violations[0])
	}
}

func TestValidateStructAfterCurrentTime(t *testing.T) {
	type request struct {
		At time.Time `json:"at" validate:"after_current_time"`
	}

	if err := ValidateStruct(&request{At: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("expected future time to pass: %v", err)
	}
	if err := ValidateStruct(&request{At: time.Now().Add(-time.Hour)}); err == nil {
		t.Fatal("expected past time to fail")
	}
}

func TestValidateStructNotBeforeCurrentTimeWithPointerAndAllowNull(t *testing.T) {
	type request struct {
		At *time.Time `json:"at" validate:"not_before_current_time=false"`
	}

	if err := ValidateStruct(&request{}); err == nil {
		t.Fatal("expected nil pointer to fail when allowNull=false")
	}

	future := time.Now().Add(time.Minute)
	if err := ValidateStruct(&request{At: &future}); err != nil {
		t.Fatalf("expected future pointer to pass: %v", err)
	}
}
