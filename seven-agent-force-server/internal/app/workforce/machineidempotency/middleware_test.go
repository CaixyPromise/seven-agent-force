package machineidempotency

import (
	"net/http"
	"testing"
)

func TestRequestDigestCanonicalizesJSONAndBindsIdentity(t *testing.T) {
	t.Parallel()
	first, err := requestDigest(1001, "POST", "/api/internal/workforce/v1/attempts/att_1/outcome", []byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := requestDigest(1001, "POST", "/api/internal/workforce/v1/attempts/att_1/outcome", []byte("{\n  \"a\": 1, \"b\": 2\n}"))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical JSON digest drift: %s != %s", first, second)
	}
	different, _ := requestDigest(1001, "POST", "/api/internal/workforce/v1/attempts/att_2/outcome", []byte(`{"a":1,"b":2}`))
	if different == first {
		t.Fatal("route parameter identity was not bound into request digest")
	}
}

func TestRequestDigestRejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	if _, err := requestDigest(1001, "POST", "/x", []byte(`{} {}`)); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestReplayableResponseStatusRejectsServerFailures(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusConflict} {
		if !replayableResponseStatus(status) {
			t.Fatalf("expected status %d to be replayable", status)
		}
	}
	for _, status := range []int{0, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		if replayableResponseStatus(status) {
			t.Fatalf("expected status %d to be released for retry", status)
		}
	}
}
