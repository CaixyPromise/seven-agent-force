package infrastructure

import (
	"errors"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
)

func TestDecodeSummaryDefinitionDoesNotInterpretObsoleteDefinition(t *testing.T) {
	const digest = "1c5eeb4d19097ebacdf02d95d2d737c5927a7556c4e91101701555f8a0abb762"
	input, unavailable, err := decodeSummaryDefinition([]byte(`{"mode":"COMMAND","command":"true","reviewFocus":"obsolete"}`), digest)
	if err != nil || input != nil || !unavailable {
		t.Fatalf("input=%+v unavailable=%v err=%v", input, unavailable, err)
	}
}

func TestDecodeSummaryDefinitionRejectsMalformedOrUnboundDefinition(t *testing.T) {
	for _, test := range []struct {
		raw    string
		digest string
	}{
		{raw: `{"mode":`, digest: "1c5eeb4d19097ebacdf02d95d2d737c5927a7556c4e91101701555f8a0abb762"},
		{raw: `{"mode":"COMMAND","command":"true","unknown":true}`, digest: ""},
		{raw: `{"mode":"COMMAND","command":"true","unknown":true}`, digest: "1c5eeb4d19097ebacdf02d95d2d737c5927a7556c4e91101701555f8a0abb762"},
		{raw: `{"mode":"COMMAND","command":"true","reviewFocus":true}`, digest: "1c5eeb4d19097ebacdf02d95d2d737c5927a7556c4e91101701555f8a0abb762"},
	} {
		if _, _, err := decodeSummaryDefinition([]byte(test.raw), test.digest); !errors.Is(err, facade.ErrConflict) {
			t.Fatalf("raw=%q err=%v", test.raw, err)
		}
	}
}
