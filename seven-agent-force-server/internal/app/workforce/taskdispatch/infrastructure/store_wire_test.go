package infrastructure

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestFinalizationWorkMessageQuotesAllInt64ProtocolIdentities(t *testing.T) {
	t.Parallel()
	message := finalizationWorkMessage{
		SchemaVersion: 1,
		Kind:          "seven.finalization-run/v1",
		MessageID:     9007199254740993,
		WorkerID:      9007199254740994,
		ResourceID:    9007199254740995,
		Generation:    9007199254740996,
	}
	body, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"messageId":"9007199254740993"`,
		`"workerId":"9007199254740994"`,
		`"resourceId":"9007199254740995"`,
		`"generation":"9007199254740996"`,
	} {
		if !bytes.Contains(body, []byte(expected)) {
			t.Fatalf("finalization message omitted lossless field %s: %s", expected, body)
		}
	}
}
