package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestWriteInboxSSEEventSerializesOnlyContentFreeHint(t *testing.T) {
	recorder := httptest.NewRecorder()
	hint := facade.InboxRealtimeHint{
		ChangeToken: "opaque-user-bound-change-token",
		NewUnread:   true,
	}
	if err := writeInboxSSEEvent(recorder, "notification.changed", hint); err != nil {
		t.Fatalf("write inbox SSE event: %v", err)
	}

	body := recorder.Body.String()
	if !strings.HasPrefix(body, "event: notification.changed\n") || !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("unexpected SSE frame: %q", body)
	}
	for _, forbidden := range []string{"recipientId", "title", "summary", "content", "deepLink", "userId", "scopeId"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("SSE frame leaked %q: %s", forbidden, body)
		}
	}

	data := strings.TrimSuffix(strings.TrimPrefix(body, "event: notification.changed\ndata: "), "\n\n")
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("decode SSE data: %v", err)
	}
	if len(payload) != 2 || payload["changeToken"] != hint.ChangeToken || payload["newUnread"] != hint.NewUnread {
		t.Fatalf("unexpected SSE payload: %#v", payload)
	}
}
