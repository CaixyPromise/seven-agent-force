package cache

import (
	"testing"
	"time"
)

func TestCodecRoundTripsTimePointers(t *testing.T) {
	codec, err := NewCodec("sonic")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}

	now := time.Date(2026, 4, 23, 18, 1, 10, 685000000, time.UTC)
	type payload struct {
		CreatedAt *time.Time `json:"createdAt"`
		ExpiresAt *time.Time `json:"expiresAt"`
	}

	raw, err := codec.Marshal(payload{
		CreatedAt: &now,
		ExpiresAt: timePointer(now.Add(5 * time.Minute)),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded payload
	if err := codec.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.CreatedAt == nil || !decoded.CreatedAt.Equal(now) {
		t.Fatalf("unexpected createdAt: %+v", decoded.CreatedAt)
	}
	expectedExpiry := now.Add(5 * time.Minute)
	if decoded.ExpiresAt == nil || !decoded.ExpiresAt.Equal(expectedExpiry) {
		t.Fatalf("unexpected expiresAt: %+v", decoded.ExpiresAt)
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}
