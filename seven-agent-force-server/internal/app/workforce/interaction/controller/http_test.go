package controller

import (
	"errors"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/cloudwego/hertz/pkg/app"
	"strings"
	"testing"
)

func TestDecodeRejectsAmbiguousAndUnknownFields(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `{"text":"one","text":"two","idempotencyKey":"x"}`, `{"text":"one","idempotencyKey":"x","tenantId":"99"}`, `{"text":"one","idempotencyKey":"x"} {}`, `{"text":"one","idempotencyKey":123}`, strings.Repeat(" ", 32<<10) + "{}"} {
		c := app.NewContext(0)
		c.Request.SetBodyString(body)
		var r facade.MessageRequest
		if decode(c, &r) != domain.ErrInvalid {
			t.Fatal("ambiguous request accepted")
		}
	}
	c := app.NewContext(0)
	c.Request.SetBodyString(`{"text":"one","idempotencyKey":"x"}`)
	var r facade.MessageRequest
	if e := decode(c, &r); e != nil || r.Text != "one" {
		t.Fatal(r, e)
	}
}
func TestProblemNeverEchoesDependencyBody(t *testing.T) {
	c := app.NewContext(0)
	problem(c, errors.New("SQL password=secret provider raw body"))
	body := string(c.Response.Body())
	if strings.Contains(body, "secret") || !strings.Contains(body, "INTERACTION_UNAVAILABLE") || c.Response.StatusCode() != 500 {
		t.Fatal(body)
	}
	problem(c, domain.ErrStale)
	if c.Response.StatusCode() != 409 {
		t.Fatal(c.Response.StatusCode())
	}
}
func TestTransportPositiveDecimal(t *testing.T) {
	for _, s := range []string{"0", "-1", "01", "+1", "1e5", "9223372036854775808"} {
		if _, e := positive(s); e == nil {
			t.Fatal(s)
		}
	}
	n, e := positive("9007199254740993")
	if e != nil || n != 9007199254740993 {
		t.Fatal(n, e)
	}
}
func TestSuspendInProgressIsRetryableConflict(t *testing.T) {
	c := app.NewContext(0)
	problem(c, continuation.ErrInteractionSuspendInProgress)
	if c.Response.StatusCode() != 409 || !strings.Contains(string(c.Response.Body()), `"retryable":true`) || string(c.Response.Header.Peek("Retry-After")) != "1" {
		t.Fatal(string(c.Response.Body()))
	}
}
