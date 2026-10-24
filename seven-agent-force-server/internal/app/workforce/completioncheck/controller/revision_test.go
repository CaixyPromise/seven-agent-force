package controller

import (
	"strings"
	"testing"
)

func TestRevisionDecodeRejectsReplacementAuthorityAndAmbiguity(t *testing.T) {
	for _, b := range []string{`null`, `{"expectedVersion":"1","expectedVersion":"2"}`, `{"expectedVersion":"1","mandatory":false}`, `{"completionCheck":{"mode":"NONE","mode":"COMMAND","command":"true"}}`, `{"expectedVersion":1}`, `{} {}`, strings.Repeat(" ", 128<<10) + "{}"} {
		if _, e := decodeRevision([]byte(b)); e == nil {
			t.Fatal("invalid accepted", b[:min(len(b), 100)])
		}
	}
	if _, e := decodeRevision([]byte(`{"expectedVersion":"7","idempotencyKey":"one","completionCheck":{"mode":"COMMAND","command":"go test ./...","instructions":"old\nnew"}}`)); e != nil {
		t.Fatal(e)
	}
}
