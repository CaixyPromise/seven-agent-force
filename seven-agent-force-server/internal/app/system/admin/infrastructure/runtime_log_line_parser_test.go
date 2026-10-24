package infrastructure

import (
	"strings"
	"testing"
)

func TestRuntimeLogLineParserParsesZapJSONLine(t *testing.T) {
	parser := NewRuntimeLogLineParser(NewRuntimeLogMaskingSupport([]string{"password"}, 256))

	item, ok := parser.Parse(`{"timestamp":"2026-04-24T12:34:56.789+0800","level":"info","threadName":"main","loggerName":"http.server","trace_id":"trace-abc-123","message":"password=secret ok","caller":"server.go:42","method":"GET","path":"/api/demo","payload":{"query":{"password":"secret"}}}`, "json-1")
	if !ok {
		t.Fatal("expected JSON log line to parse")
	}
	if item.Level != "INFO" {
		t.Fatalf("expected INFO level, got %q", item.Level)
	}
	if item.ThreadName != "main" || item.LoggerName != "http.server" {
		t.Fatalf("unexpected thread/logger: %#v", item)
	}
	if item.LogTime == nil {
		t.Fatal("expected timestamp with numeric timezone to parse")
	}
	if item.FileName != "server.go" || item.LineNumber != 42 {
		t.Fatalf("unexpected caller mapping: %#v", item)
	}
	if item.TraceID != "trace-abc-123" {
		t.Fatalf("expected trace id, got %q", item.TraceID)
	}
	if item.Message != "password=****** ok" {
		t.Fatalf("expected masked message, got %q", item.Message)
	}
	if item.Source["method"] != "GET" || item.Source["path"] != "/api/demo" {
		t.Fatalf("expected source fields to be retained, got %#v", item.Source)
	}
	if strings.Contains(strings.ToLower(item.Source["payload"].(map[string]any)["query"].(map[string]any)["password"].(string)), "secret") {
		t.Fatalf("expected source payload to be masked, got %#v", item.Source)
	}
}

func TestRuntimeLogLineParserParsesJavaTextLine(t *testing.T) {
	parser := NewRuntimeLogLineParser(NewRuntimeLogMaskingSupport(nil, 256))

	item, ok := parser.Parse("2026-04-24 12:34:56.789 INFO [http-nio-8080-exec-1] com.seven.AdminHandler - runtime log page requested", "text-1")
	if !ok {
		t.Fatal("expected Java text line to parse")
	}
	if item.Level != "INFO" {
		t.Fatalf("expected INFO level, got %q", item.Level)
	}
	if item.ThreadName != "http-nio-8080-exec-1" {
		t.Fatalf("unexpected thread name: %#v", item)
	}
	if item.LoggerName != "com.seven.AdminHandler" {
		t.Fatalf("unexpected logger name: %#v", item)
	}
	if item.Message != "runtime log page requested" {
		t.Fatalf("unexpected message: %q", item.Message)
	}
}

func TestRuntimeLogLineParserMasksSensitiveConfigFields(t *testing.T) {
	parser := NewRuntimeLogLineParser(NewRuntimeLogMaskingSupport(nil, 256))

	item, ok := parser.Parse(`{"level":"info","message":"payload configKey=payment.gateway configValue=plain-secret-updated isSensitive=1"}`, "json-2")
	if !ok {
		t.Fatal("expected JSON log line to parse")
	}
	for _, leaked := range []string{"plain-secret-updated", "isSensitive=1"} {
		if strings.Contains(item.Message, leaked) {
			t.Fatalf("runtime log leaked %q in %q", leaked, item.Message)
		}
	}
	if !strings.Contains(item.Message, "configKey=payment.gateway") {
		t.Fatalf("configKey should remain visible in %q", item.Message)
	}
}
