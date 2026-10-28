package machineidempotency

import (
	"bytes"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/cloudwego/hertz/pkg/app"
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

func TestFailureReplacesPreviouslyWrittenSuccessfulResponse(t *testing.T) {
	c := app.NewContext(0)
	response.Success(c, map[string]any{"lease": "private-success-body"})
	writeError(c, http.StatusServiceUnavailable, "MACHINE_IDEMPOTENCY_PERSIST_FAILED", "machine response could not be made replayable")
	var body struct {
		Code int `json:"code"`
		Data struct {
			ReasonCode string `json:"reasonCode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
		t.Fatalf("failure response must be a single valid JSON envelope: %v", err)
	}
	if c.Response.StatusCode() != http.StatusServiceUnavailable || body.Data.ReasonCode != "MACHINE_IDEMPOTENCY_PERSIST_FAILED" || bytes.Contains(c.Response.Body(), []byte("private-success-body")) {
		t.Fatal("failure leaked or appended to previous success response")
	}
}
