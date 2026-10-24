package xcontext

import (
	"context"
	"regexp"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestTraceIDContextRoundTrip(t *testing.T) {
	ctx := WithTraceID(context.Background(), " trace-service-audit ")

	if got := TraceIDFromContext(ctx); got != "trace-service-audit" {
		t.Fatalf("TraceIDFromContext() = %q, want trace-service-audit", got)
	}
}

func TestWithTraceIDIgnoresBlankTrace(t *testing.T) {
	ctx := WithTraceID(context.Background(), " ")

	if got := TraceIDFromContext(ctx); got != "" {
		t.Fatalf("TraceIDFromContext() = %q, want empty", got)
	}
}

func TestEnsureTraceIDGeneratesForBlankHeader(t *testing.T) {
	reqCtx := &app.RequestContext{}
	reqCtx.Request.Header.Set(TraceIDHeader, "   ")

	traceID := EnsureTraceID(reqCtx)
	if traceID == "" || traceID == "   " {
		t.Fatalf("EnsureTraceID() = %q, want generated trace", traceID)
	}
	if got := reqCtx.GetString(TraceIDKey); got != traceID {
		t.Fatalf("stored trace id = %q, want %q", got, traceID)
	}
}

func TestEnsureTraceIDAcceptsOnlyCanonicalTraceIDs(t *testing.T) {
	const valid = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{name: "valid", header: valid, want: valid},
		{name: "uppercase", header: "0123456789ABCDEF0123456789ABCDEF"},
		{name: "legacy text", header: "trace-fixed"},
		{name: "all zero", header: "00000000000000000000000000000000"},
	}
	canonical := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reqCtx := &app.RequestContext{}
			reqCtx.Request.Header.Set(TraceIDHeader, tc.header)
			got := EnsureTraceID(reqCtx)
			if tc.want != "" && got != tc.want {
				t.Fatalf("EnsureTraceID() = %q, want %q", got, tc.want)
			}
			if !canonical.MatchString(got) || got == "00000000000000000000000000000000" {
				t.Fatalf("EnsureTraceID() = %q, want non-zero canonical ID", got)
			}
			if tc.want == "" && got == tc.header {
				t.Fatalf("EnsureTraceID() retained invalid input %q", got)
			}
		})
	}
}

func TestNewTraceIDAlwaysReturnsCanonicalValue(t *testing.T) {
	canonical := regexp.MustCompile(`^[0-9a-f]{32}$`)
	for range 100 {
		traceID := NewTraceID()
		if !canonical.MatchString(traceID) || traceID == "00000000000000000000000000000000" {
			t.Fatalf("NewTraceID() = %q, want non-zero canonical ID", traceID)
		}
	}
}
