package cachepolicy

import (
	"errors"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
)

func TestDecodeInvalidationEnvelopeRejectsOversizedPayloadBeforeSonicAcceptance(t *testing.T) {
	event := InvalidationEnvelope{
		SchemaVersion: SchemaVersionV1,
		EventID:       strings.Repeat("e", 1024),
		ScopeID:       StorageScopeSystemGlobal,
		DataClass:     DataClassConfigPublicScalar,
		TargetDigest:  ClassTargetDigest(DataClassConfigPublicScalar),
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("encode oversized otherwise-valid envelope: %v", err)
	}
	if len(payload) <= 1024 {
		t.Fatalf("oversized envelope fixture is only %d bytes", len(payload))
	}
	if _, err := DecodeInvalidationEnvelope(payload); !errors.Is(err, ErrInvalidationEnvelope) {
		t.Fatalf("oversized invalidation envelope was accepted: %v", err)
	}
}

func TestV1InvalidationCannotAddressTargetedSessionClass(t *testing.T) {
	if _, err := NewInvalidationEnvelope("v1-session-bypass", DataClassActiveSessionValidity); !errors.Is(err, ErrInvalidationEnvelope) {
		t.Fatalf("V1 accepted targeted session class: %v", err)
	}
}

func TestFanoutRejectionDiagnosticIsContentFree(t *testing.T) {
	diagnostic, err := NewFanoutRejectionDiagnostic("diag-1", "instance-a", FanoutRejectionInvalidEnvelope)
	if err != nil {
		t.Fatalf("create fanout rejection diagnostic: %v", err)
	}
	payload, err := sonic.Marshal(diagnostic)
	if err != nil {
		t.Fatalf("encode fanout rejection diagnostic: %v", err)
	}
	text := string(payload)
	for _, forbidden := range []string{"themePrimaryColor", "secret", "rawBody", "payload", "targetDigest", "instance-a"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("content-free fanout rejection diagnostic exposed %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"category":"invalid-envelope"`) || !strings.Contains(text, `"scopeId":"system:global"`) {
		t.Fatalf("fanout rejection diagnostic lost its fixed safe fields: %s", text)
	}
}

func TestCacheRefreshV3IsStrictAndIndependentFromV1V2(t *testing.T) {
	event, err := NewCacheRefreshEnvelope("refresh-v3-event")
	if err != nil {
		t.Fatalf("new refresh envelope: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal refresh envelope: %v", err)
	}
	decoded, err := DecodeCacheRefreshEnvelope(payload)
	if err != nil || decoded != event {
		t.Fatalf("decode refresh envelope: got=%+v err=%v", decoded, err)
	}
	if _, err := DecodeInvalidationEnvelope(payload); !errors.Is(err, ErrInvalidationEnvelope) {
		t.Fatalf("v1 decoder accepted v3: %v", err)
	}
	if _, err := DecodeTargetedInvalidationEnvelope(payload); !errors.Is(err, ErrInvalidationEnvelope) {
		t.Fatalf("v2 decoder accepted v3: %v", err)
	}
	if _, err := DecodeCacheRefreshEnvelope([]byte(`{"schemaVersion":3,"eventId":"x","scopeId":"system:global","operation":"SYSTEM_CACHE_REFRESH","targetDigest":"unexpected"}`)); !errors.Is(err, ErrInvalidationEnvelope) {
		t.Fatalf("v3 decoder accepted unknown field: %v", err)
	}
}
