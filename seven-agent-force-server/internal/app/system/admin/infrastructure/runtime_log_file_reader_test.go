package infrastructure

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/domain"
)

func TestRuntimeLogTraceIDFilterUsesExactMatch(t *testing.T) {
	const traceID = "34343434343434343434343434343434"
	item := domain.RuntimeLogLine{TraceID: traceID}
	if !matchRuntimeLogLine(item, domain.RuntimeLogPageQuery{TraceID: traceID}, 0) {
		t.Fatal("exact Trace ID did not match")
	}
	for _, partial := range []string{"34343434", traceID[:31], traceID + "ff"} {
		if matchRuntimeLogLine(item, domain.RuntimeLogPageQuery{TraceID: partial}, 0) {
			t.Fatalf("partial Trace ID %q unexpectedly matched %q", partial, traceID)
		}
	}
}
