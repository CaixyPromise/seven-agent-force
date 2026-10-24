package controller

import (
	"context"
	"github.com/cloudwego/hertz/pkg/app"
	"strings"
	"testing"
)

func TestEvaluationReplayControllerRejectsUnauthenticatedRead(t *testing.T) {
	c := app.NewContext(1)
	c.Request.SetBodyString(`{"workerId":"7","instanceId":"i","hostBootId":"b","generation":"1","ownerEpoch":"2","attemptLeaseToken":"lease","pending":{"cycleId":"5","cycleNumber":1,"sessionDigest":"session"}}`)
	(&MachineHandler{}).readEvaluation(context.Background(), c)
	if c.Response.StatusCode() != 401 || !strings.Contains(string(c.Response.Body()), "WORKER_UNAUTHORIZED") {
		t.Fatalf("unexpected response: %d", c.Response.StatusCode())
	}
}

func TestEvaluationReplayControllerRejectsMalformedRead(t *testing.T) {
	c := app.NewContext(1)
	c.Request.SetBodyString(`{"pending":{"cycleId":5}}`)
	(&MachineHandler{}).readEvaluation(context.Background(), c)
	if c.Response.StatusCode() != 400 {
		t.Fatalf("numeric ID accepted: %d", c.Response.StatusCode())
	}
}
