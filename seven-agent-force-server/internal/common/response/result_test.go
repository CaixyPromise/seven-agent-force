package response

import (
	"encoding/json"
	"net/http"
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestErrorWithReasonKeepsNumericEnvelopeAndReasonInData(t *testing.T) {
	c := app.NewContext(0)
	ErrorWithReason(c, http.StatusServiceUnavailable, apperrors.ServiceUnavailable("完成检查暂时不可用"), "COMPLETION_CHECK_UNAVAILABLE", map[string]any{"retryable": true, "reasonCode": "cannot-override"})

	var body struct {
		Code int `json:"code"`
		Data struct {
			ReasonCode string `json:"reasonCode"`
			Retryable  bool   `json:"retryable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if c.Response.StatusCode() != http.StatusServiceUnavailable || body.Code != apperrors.CodeServiceUnavailable || body.Data.ReasonCode != "COMPLETION_CHECK_UNAVAILABLE" || !body.Data.Retryable {
		t.Fatalf("unexpected response: status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
}
