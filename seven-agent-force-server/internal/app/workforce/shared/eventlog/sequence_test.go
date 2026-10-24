package eventlog

import (
	"bytes"
	"testing"
)

func TestAppendRejectsMissingIdentityBeforeUsingTransaction(t *testing.T) {
	t.Parallel()
	if err := Append(t.Context(), nil, 0, 0, "event", map[string]any{}); err == nil {
		t.Fatal("expected missing task id to fail")
	}
	if err := Append(t.Context(), nil, 1001, 0, "", map[string]any{}); err == nil {
		t.Fatal("expected missing event type to fail")
	}
}

func TestMarshalLosslessPayloadQuotesSnowflakeMapValues(t *testing.T) {
	t.Parallel()
	body, err := marshalLosslessPayload(map[string]any{
		"taskId":     int64(9007199254740993),
		"actor":      int64(9007199254740995),
		"generation": int64(3),
		"nested":     map[string]any{"checkpointId": int64(9007199254740997), "sizeBytes": int64(8192)},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"taskId":"9007199254740993"`, `"actor":"9007199254740995"`, `"checkpointId":"9007199254740997"`, `"generation":3`, `"sizeBytes":8192`} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("lossless event payload missing %s: %s", expected, body)
		}
	}
}
