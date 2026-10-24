package controller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/cloudwego/hertz/pkg/app"
)

func TestHistoricalContextErrorNeverEchoesInvalidJSON(t *testing.T) {
	const raw = `{"mode":"INVALID_PRIVATE_MODE","script":"PRIVATE_SCRIPT","instructions":"PRIVATE_CONTEXT","hostPath":"/private/host"}`
	for _, tc := range []struct {
		err  error
		code string
	}{
		{fmt.Errorf("%s: %w", raw, domain.ErrInvalidReviewInput), "INDEPENDENT_REVIEW_CONTEXT_UNAVAILABLE"},
		{errors.New(raw), "INTERNAL_ERROR"},
	} {
		c := &app.RequestContext{}
		(&Handler{}).writeDecisionError(c, tc.err)
		body := string(c.Response.Body())
		if !strings.Contains(body, tc.code) {
			t.Fatalf("missing stable error discriminant: %s", body)
		}
		for _, forbidden := range []string{"PRIVATE", "/private/host", "script", "hostPath"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("error echoed raw context: %s", body)
			}
		}
	}
}
