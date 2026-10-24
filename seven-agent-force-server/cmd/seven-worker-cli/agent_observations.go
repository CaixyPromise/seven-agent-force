package main

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

const observationWindow = 8

var observationCredential = regexp.MustCompile(`(?i)(-----BEGIN [A-Z ]*PRIVATE KEY-----|bearer\s+\S+|\bsk-[A-Za-z0-9_-]{8,}|(?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*\S+)`)

type agentObservation struct {
	Sequence int64           `json:"sequence,string"`
	Method   string          `json:"method"`
	Payload  json.RawMessage `json:"payload"`
}

func safeAgentText(value string) string {
	value = observationCredential.ReplaceAllString(value, "[redacted]")
	value = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	if len(value) > 2048 {
		value = value[:2048]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
		value += "…"
	}
	return value
}

// This is a small reconnect window, not an authoritative event journal. Only
// display fields enter it; nested raw native bodies and execution authority do not.
func projectAgentObservation(frame agentprotocol.Frame) (json.RawMessage, bool) {
	var raw map[string]json.RawMessage
	if json.Unmarshal(frame.Payload, &raw) != nil {
		return nil, false
	}
	text := func(key string) (string, bool) {
		var value string
		if json.Unmarshal(raw[key], &value) != nil {
			return "", false
		}
		return safeAgentText(value), true
	}
	safe := map[string]any{}
	if round, ok := text("roundKey"); ok {
		safe["roundKey"] = round
	} else {
		return nil, false
	}
	switch frame.Method {
	case "message":
		value, ok := text("text")
		if !ok {
			return nil, false
		}
		safe["text"] = value
	case "activity":
		for _, key := range []string{"activityKey", "title", "status"} {
			value, ok := text(key)
			if !ok {
				return nil, false
			}
			safe[key] = value
		}
		switch safe["status"] {
		case "STARTED", "COMPLETED", "SUCCEEDED", "FAILED":
		default:
			return nil, false
		}
	case "plan":
		var steps []string
		if json.Unmarshal(raw["steps"], &steps) != nil || len(steps) > 32 {
			return nil, false
		}
		for i, step := range steps {
			encoded, _ := json.Marshal(step)
			raw["step"] = encoded
			steps[i], _ = text("step")
		}
		safe["steps"] = steps
	default:
		return nil, false
	}
	body, err := json.Marshal(safe)
	return body, err == nil && len(body) <= 4096
}

func (state *supervisorState) observeAgent(frame agentprotocol.Frame) {
	if state.observeInteraction(frame) {
		return
	}
	body, ok := projectAgentObservation(frame)
	if !ok {
		return
	}
	state.mu.Lock()
	state.observationSequence++
	state.observations = append(state.observations, agentObservation{state.observationSequence, frame.Method, body})
	if len(state.observations) > observationWindow {
		state.observations = append([]agentObservation(nil), state.observations[len(state.observations)-observationWindow:]...)
		state.observationGap = true
	}
	safeFrame := frame
	safeFrame.Payload = body
	state.appendActivityNotification(safeFrame)
	state.mu.Unlock()
	// Display activity is polled through notifications and this window. Writing it to stdout
	// here could block Round's sole reader behind a slow log consumer, starving
	// critical interaction receipts and cancellation.
}
