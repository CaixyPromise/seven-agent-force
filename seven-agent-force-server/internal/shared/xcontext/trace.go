package xcontext

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
)

const (
	TraceIDKey    = "trace_id"
	TraceIDHeader = "X-Trace-Id"
)

type traceContextKey struct{}

var (
	canonicalTraceIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	fallbackTraceCounter    atomic.Uint64
)

func EnsureTraceID(c *app.RequestContext) string {
	traceID := strings.TrimSpace(c.GetString(TraceIDKey))
	if IsCanonicalTraceID(traceID) {
		return traceID
	}

	traceID = strings.TrimSpace(c.Request.Header.Get(TraceIDHeader))
	if !IsCanonicalTraceID(traceID) {
		traceID = NewTraceID()
	}

	c.Set(TraceIDKey, traceID)
	c.Header(TraceIDHeader, traceID)
	return traceID
}

func TraceID(c *app.RequestContext) string {
	if c == nil {
		return ""
	}
	return c.GetString(TraceIDKey)
}

func WithTraceID(ctx context.Context, traceID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return context.WithValue(ctx, traceContextKey{}, strings.TrimSpace(traceID))
}

func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceContextKey{}).(string)
	return strings.TrimSpace(value)
}

// EnsureContextTraceID returns a context carrying a canonical Trace ID.
func EnsureContextTraceID(ctx context.Context) (context.Context, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID := TraceIDFromContext(ctx)
	if !IsCanonicalTraceID(traceID) {
		traceID = NewTraceID()
	}
	return WithTraceID(ctx, traceID), traceID
}

func NewTraceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil && !allZero(buf) {
		return hex.EncodeToString(buf)
	}
	fallback := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", time.Now().UnixNano(), fallbackTraceCounter.Add(1))))
	if allZero(fallback[:16]) {
		fallback[15] = 1
	}
	return hex.EncodeToString(fallback[:16])
}

// IsCanonicalTraceID reports whether value is a non-zero 32-character lowercase hexadecimal ID.
func IsCanonicalTraceID(value string) bool {
	if !canonicalTraceIDPattern.MatchString(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && !allZero(decoded)
}

func allZero(value []byte) bool {
	for _, item := range value {
		if item != 0 {
			return false
		}
	}
	return true
}
