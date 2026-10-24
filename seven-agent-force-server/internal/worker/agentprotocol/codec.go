// Package agentprotocol validates untrusted image Adapter frames. A valid
// frame is an observation, never authorization or trusted completion.
package agentprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"strconv"
	"unicode/utf8"
)

const Version = "seven.agent-adapter/v1"
const MaxFrameBytes = 1 << 20
const MaxMessageBytes = 64 << 10

var ErrInvalid = errors.New("ADAPTER_PROTOCOL_INVALID")
var ErrUnsupported = errors.New("ADAPTER_METHOD_UNSUPPORTED")
var keyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

// ValidateDocument rejects duplicate fields before a UDS dispatcher can lose
// them during unmarshalling. It carries no authorization semantics.
func ValidateDocument(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxFrameBytes || !utf8.Valid(raw) || !strictDocument(raw) {
		return ErrInvalid
	}
	return nil
}

type Frame struct {
	Protocol string          `json:"protocol"`
	Method   string          `json:"method"`
	Key      string          `json:"key"`
	ReplyTo  string          `json:"replyTo,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

func allowed(method, direction string) bool {
	switch direction {
	case "down":
		switch method {
		case "initialize", "execution.start", "execution.resume", "execution.interrupt", "interaction.respond", "user.message", "execution.suspend", "tool.result":
			return true
		}
	case "up":
		switch method {
		case "ready", "execution.started", "execution.finished", "message", "plan", "activity", "control.ack", "error", "interaction.request", "interaction.received", "interaction.closed", "message.received", "execution.suspended", "tool.catalog", "tool.request", "tool.received", "tool.closed":
			return true
		}
	}
	return false
}

// strictValue rejects duplicate keys before ordinary JSON unmarshalling can
// discard them. Depth and IDs are checked independently of float conversion.
func strictValue(d *json.Decoder, depth int) error {
	if depth > 34 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return ErrInvalid
	}
	if number, ok := token.(json.Number); ok {
		value, err := strconv.ParseFloat(string(number), 64)
		if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
			return ErrInvalid
		}
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			token, err = d.Token()
			if err != nil {
				return ErrInvalid
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return ErrInvalid
			}
			seen[key] = true
			switch key {
			case "taskId", "taskTurnId", "attemptId", "reviewRunId", "reviewRunAttemptId":
				token, err = d.Token()
				if err != nil {
					return ErrInvalid
				}
				value, ok := token.(string)
				if !ok || !idPattern.MatchString(value) {
					return ErrInvalid
				}
				if _, err := strconv.ParseInt(value, 10, 64); err != nil {
					return ErrInvalid
				}
			default:
				if err := strictValue(d, depth+1); err != nil {
					return err
				}
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim('}') {
			return ErrInvalid
		}
	case '[':
		for d.More() {
			if err := strictValue(d, depth+1); err != nil {
				return err
			}
		}
		token, err = d.Token()
		if err != nil || token != json.Delim(']') {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func Decode(raw []byte, direction string) (Frame, error) {
	var f Frame
	if !utf8.Valid(raw) {
		return f, ErrInvalid
	}
	if direction != "up" && direction != "down" {
		return f, ErrInvalid
	}
	if len(raw) == 0 || len(raw) > MaxFrameBytes || raw[len(raw)-1] != '\n' || bytes.ContainsRune(raw[:len(raw)-1], '\n') {
		return f, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := strictValue(d, 0); err != nil {
		return f, err
	}
	if _, err := d.Token(); err != io.EOF {
		return f, ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&f); err != nil {
		return Frame{}, ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return Frame{}, ErrInvalid
	}
	if reply, present := fields["replyTo"]; present && (bytes.Equal(bytes.TrimSpace(reply), []byte("null")) || f.ReplyTo == "") {
		return Frame{}, ErrInvalid
	}
	if f.Protocol != Version || !keyPattern.MatchString(f.Key) || (f.ReplyTo != "" && !keyPattern.MatchString(f.ReplyTo)) {
		return Frame{}, ErrInvalid
	}
	if !allowed(f.Method, direction) {
		return Frame{}, ErrUnsupported
	}
	payload := bytes.TrimSpace(f.Payload)
	if len(payload) < 2 || payload[0] != '{' {
		return Frame{}, ErrInvalid
	}
	if f.Method == "message" || f.Method == "plan" || f.Method == "activity" {
		var compact bytes.Buffer
		if json.Compact(&compact, payload) != nil || compact.Len()+1 > MaxMessageBytes {
			return Frame{}, ErrInvalid
		}
	}
	if err := ValidateInteractionPayload(f.Method, f.Payload); err != nil {
		return Frame{}, err
	}
	if err := ValidateToolPayload(f.Method, f.Payload); err != nil {
		return Frame{}, err
	}
	return f, nil
}
