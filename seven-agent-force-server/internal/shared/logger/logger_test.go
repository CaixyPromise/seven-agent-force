package logger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewWithConsoleWriterDevCanWriteConsoleAndFileWhenEnabled(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "app.log")
	cfg := config.LoggingConfig{
		Level:  "debug",
		Format: "console",
		File: config.FileLoggingConfig{
			Enabled: true,
			Path:    logPath,
		},
	}

	var console bytes.Buffer
	log, err := newWithConsoleWriter(cfg, "dev", &console)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	log.Info("dev console only")
	Sync(log)

	if !strings.Contains(console.String(), "dev console only") {
		t.Fatalf("console output missing log message: %s", console.String())
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read rolling file in dev: %v", err)
	}
	if !strings.Contains(string(content), "\"message\":\"dev console only\"") {
		t.Fatalf("file output missing dev log message: %s", string(content))
	}
}

func TestWithContextAddsCanonicalTraceAndSpan(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	base := zap.New(core)
	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	ctx = xcontext.WithTraceID(ctx, traceID.String())

	WithContext(ctx, base).Info("context log")
	fields := observed.All()[0].ContextMap()
	if fields["trace_id"] != traceID.String() || fields["span_id"] != spanID.String() {
		t.Fatalf("context fields = %#v", fields)
	}
}

func TestWithRuntimeIdentityAddsStableSourceFields(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	log := WithRuntimeIdentity(zap.New(core), RuntimeIdentity{
		ServiceName: "seven-hub", PlatformMode: "hub", NodeCode: "", Profile: "test", ServiceInstanceID: "hub-1",
	})
	log.Info("identity log")
	fields := observed.All()[0].ContextMap()
	for key, want := range map[string]string{
		"service_name": "seven-hub", "platform_mode": "hub", "node_code": "", "profile": "test", "service_instance_id": "hub-1",
	} {
		if fields[key] != want {
			t.Fatalf("%s = %#v, want %q; fields=%#v", key, fields[key], want, fields)
		}
	}
}

func TestHertzAdapterCtxLogUsesContextAndRealCaller(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	adapter := NewHertzAdapter(zap.New(core, zap.AddCaller()))
	ctx := xcontext.WithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")

	adapter.CtxInfof(ctx, "adapter %s", "message")
	entry := observed.All()[0]
	if entry.ContextMap()["trace_id"] != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("adapter fields = %#v", entry.ContextMap())
	}
	if !strings.HasSuffix(entry.Caller.File, "logger_test.go") {
		t.Fatalf("caller = %s, want logger_test.go", entry.Caller.File)
	}
}

func TestNewWithConsoleWriterProdWritesConsoleAndFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "app.log")
	cfg := config.LoggingConfig{
		Level:  "info",
		Format: "json",
		File: config.FileLoggingConfig{
			Enabled:    true,
			Path:       logPath,
			MaxSizeMB:  10,
			MaxBackups: 3,
			MaxAgeDays: 7,
			Compress:   false,
			LocalTime:  true,
		},
	}

	var console bytes.Buffer
	log, err := newWithConsoleWriter(cfg, "prod", &console)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	log.Info("prod dual output")
	Sync(log)

	if !strings.Contains(console.String(), "\"message\":\"prod dual output\"") {
		t.Fatalf("console output missing json log: %s", console.String())
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read rolling file: %v", err)
	}
	fileText := string(content)
	if !strings.Contains(fileText, "\"message\":\"prod dual output\"") {
		t.Fatalf("file output missing log message: %s", fileText)
	}
	if !strings.Contains(fileText, "\"level\":\"info\"") {
		t.Fatalf("file output missing level: %s", fileText)
	}
}
