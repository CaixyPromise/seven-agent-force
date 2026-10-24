package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestProblemUsesNumericFrameworkEnvelopeAndStableReason(t *testing.T) {
	c := app.NewContext(0)
	problem(c, errors.New("database detail that must stay private"))

	var body struct {
		Code int `json:"code"`
		Data struct {
			ReasonCode string `json:"reasonCode"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(c.Response.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if c.Response.StatusCode() != http.StatusInternalServerError || body.Code != apperrors.CodeSystemError || body.Data.ReasonCode != "COMPLETION_CHECK_UNAVAILABLE" || body.Message != "完成检查暂时不可用" {
		t.Fatalf("unexpected error response: status=%d body=%s", c.Response.StatusCode(), c.Response.Body())
	}
}
