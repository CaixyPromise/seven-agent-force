package controller

import (
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/facade"
)

func TestDecodeRejectsDuplicateUnknownAndOversizedInput(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{"action":"DENY","action":"ALLOW_ONCE","expectedVersion":"1","reason":"","idempotencyKey":"k"}`, `{"action":"DENY","expectedVersion":"1","reason":"","idempotencyKey":"k","tenantId":"1"}`, `{"action":"DENY","expectedVersion":1,"reason":"","idempotencyKey":"k"}`, strings.Repeat(" ", 8193) + `{}`} {
		c := app.NewContext(0)
		c.Request.SetBodyString(body)
		var req facade.DecisionRequest
		if decode(c, &req, 8<<10) == nil {
			t.Fatalf("accepted %q", body)
		}
	}
	c := app.NewContext(0)
	c.Request.SetBodyString(`{"action":"DENY","expectedVersion":"1","reason":"","idempotencyKey":"k"}`)
	var req facade.DecisionRequest
	if err := decode(c, &req, 8<<10); err != nil || req.ExpectedVersion != 1 {
		t.Fatal(req, err)
	}
}

func TestProblemNeverEchoesDependencyError(t *testing.T) {
	c := app.NewContext(0)
	problem(c, errors.New("SQL provider token=secret"))
	body := string(c.Response.Body())
	if strings.Contains(body, "secret") || !strings.Contains(body, domain.ErrUnavailable.Error()) || c.Response.StatusCode() != 500 {
		t.Fatal(body)
	}
}

func TestPositiveDecimalPreservesInt64Identity(t *testing.T) {
	for _, raw := range []string{"0", "-1", "01", "+1", "1e3", "9223372036854775808"} {
		if _, err := positive(raw); err == nil {
			t.Fatal(raw)
		}
	}
	if got, err := positive("9007199254740993"); err != nil || got != 9007199254740993 {
		t.Fatal(got, err)
	}
}
