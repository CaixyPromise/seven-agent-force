package controller

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
	"github.com/cloudwego/hertz/pkg/app"
	"strings"
	"testing"
)

func TestInteractionPublishEnvelopeBound(t *testing.T) {
	// Worst-case encoder HTML escaping must fit the HTTP envelope budget. Frame
	// semantic/size validation remains independently owned by agentprotocol.Decode.
	frame := json.RawMessage(`{"text":"` + strings.Repeat("<", agentprotocol.MaxFrameBytes-32) + `"}`)
	body, e := json.Marshal(struct {
		Frame json.RawMessage `json:"frame"`
	}{frame})
	if e != nil {
		t.Fatal(e)
	}
	if len(body) <= 128<<10 {
		t.Fatal("fixture below former failing bound")
	}
	c := &app.RequestContext{}
	c.Request.SetBody(body)
	var out struct {
		Frame json.RawMessage `json:"frame"`
	}
	if !decode(c, &out, interactionPublishEnvelopeBytes) {
		t.Fatal("valid bounded transport envelope rejected")
	}
	c.Request.SetBody([]byte(strings.Repeat(" ", interactionPublishEnvelopeBytes+1)))
	if decode(c, &out, interactionPublishEnvelopeBytes) {
		t.Fatal("oversized envelope accepted")
	}
}
