package agentprotocol

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

var ErrControlReceiptPending = errors.New("ADAPTER_CONTROL_RECEIPT_UNCONFIRMED")

type controlReceipt struct {
	method     string
	round      string
	logical    string
	responseID string
	received   string
}

func (p *Process) HasCapability(capability string) bool {
	return slices.Contains(p.ready.Capabilities, capability)
}

func (p *Process) interactionEnabled(capability string) bool {
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	return p.interactive && p.HasCapability(capability)
}

// SendControl serializes stdin independently of Round's sole stdout reader.
// A returned key means bytes were written, not delivered, authorized, or executed.
// Correlated delivery observations are passed to Round's observe callback.
func (p *Process) SendControl(ctx context.Context, method string, payload any) (string, error) {
	switch method {
	case "interaction.respond", "user.message", "execution.suspend", "execution.interrupt", "tool.result":
	default:
		return "", ErrUnsupported
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", ErrInvalid
	}
	if err := ValidateInteractionPayload(method, body); err != nil {
		return "", err
	}
	if err := ValidateToolPayload(method, body); err != nil {
		return "", err
	}
	if method == "execution.interrupt" && !exactObject(body, "roundKey") {
		return "", ErrInvalid
	}
	var binding struct {
		RoundKey         string `json:"roundKey"`
		InteractionKey   string `json:"interactionKey"`
		ResponseID       string `json:"responseId"`
		MessageKey       string `json:"messageKey"`
		OperationID      string `json:"operationId"`
		LocalToolCallKey string `json:"localToolCallKey"`
	}
	if json.Unmarshal(body, &binding) != nil || !keyPattern.MatchString(binding.RoundKey) {
		return "", ErrInvalid
	}
	logical := binding.InteractionKey
	if method == "user.message" {
		logical = binding.MessageKey
	}
	if method == "tool.result" {
		logical = binding.LocalToolCallKey
	}
	return p.sendPrepared(ctx, method, json.RawMessage(body), func(key string) error {
		p.controlMu.Lock()
		defer p.controlMu.Unlock()
		if p.activeRound == "" || p.activeRound != binding.RoundKey {
			return errors.New("ADAPTER_ROUND_MISMATCH")
		}
		if !p.HasCapability(method) || (!p.interactive && method != "execution.interrupt") {
			return ErrUnsupported
		}
		if len(p.controls) >= 256 && method != "execution.interrupt" {
			return errors.New("ADAPTER_CONTROL_LIMIT")
		}
		p.controls[key] = controlReceipt{method: method, round: binding.RoundKey, logical: logical, responseID: binding.ResponseID}
		return nil
	})
}

func (p *Process) knownControl(key string) bool {
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	_, ok := p.controls[key]
	return ok
}

func (p *Process) acceptControlReceipt(frame Frame) error {
	p.controlMu.Lock()
	defer p.controlMu.Unlock()
	receipt, ok := p.controls[frame.ReplyTo]
	if !ok || !p.interactive || !p.HasCapability(frame.Method) {
		return ErrInvalid
	}
	expected := map[string]string{"interaction.respond": "interaction.received", "user.message": "message.received", "execution.suspend": "execution.suspended", "tool.result": "tool.received"}[receipt.method]
	var binding struct {
		RoundKey         string `json:"roundKey"`
		InteractionKey   string `json:"interactionKey"`
		ResponseID       string `json:"responseId"`
		MessageKey       string `json:"messageKey"`
		OperationID      string `json:"operationId"`
		LocalToolCallKey string `json:"localToolCallKey"`
	}
	if json.Unmarshal(frame.Payload, &binding) != nil || binding.RoundKey != receipt.round || frame.Method != expected {
		return ErrInvalid
	}
	logical := binding.InteractionKey
	if frame.Method == "message.received" {
		logical = binding.MessageKey
	}
	if frame.Method == "tool.received" {
		logical = binding.LocalToolCallKey
	}
	if logical != receipt.logical || binding.ResponseID != receipt.responseID {
		return ErrInvalid
	}
	var payload any
	_ = json.Unmarshal(frame.Payload, &payload)
	canonical, _ := json.Marshal(payload)
	if receipt.received != "" && receipt.received != string(canonical) {
		return ErrInvalid
	}
	receipt.received = string(canonical)
	p.controls[frame.ReplyTo] = receipt
	return nil
}

// Terminal execution frames do not excuse dropping an already-written control.
// Fence new sends and drain late correlated receipts, which asynchronous SDK
// cancellation may emit after its terminal frame. Ambiguity cannot be completion.
func (p *Process) finishControlReceipts(ctx context.Context, observe func(Frame)) error {
	p.controlMu.Lock()
	p.activeRound = ""
	p.controlMu.Unlock()
	grace, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		p.controlMu.Lock()
		pending := false
		for _, receipt := range p.controls {
			if receipt.received == "" && (receipt.method == "interaction.respond" || receipt.method == "user.message" || receipt.method == "tool.result") {
				pending = true
				break
			}
		}
		p.controlMu.Unlock()
		if !pending {
			return nil
		}
		frame, err := p.receive(grace)
		if err != nil {
			p.Close()
			return ErrControlReceiptPending
		}
		if frame.Method == "control.ack" && p.knownControl(frame.ReplyTo) {
			continue
		}
		if (frame.Method != "interaction.received" && frame.Method != "message.received" && frame.Method != "tool.received") || p.acceptControlReceipt(frame) != nil {
			p.Close()
			return ErrControlReceiptPending
		}
		if observe != nil {
			observe(frame)
		}
	}
}
