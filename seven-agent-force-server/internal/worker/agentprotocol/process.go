package agentprotocol

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// Process owns one admitted adapter process and its private stdio session.
// The caller must validate the immutable image entrypoint before OpenProcess.
// No executable or environment is resolved from a Task instruction here.
type Process struct {
	command     *exec.Cmd
	input       io.WriteCloser
	output      io.ReadCloser
	frames      chan Frame
	readDone    chan struct{}
	processDone chan struct{}
	closing     chan struct{}
	closeOnce   sync.Once
	runMu       sync.Mutex
	writeGate   chan struct{}
	controlMu   sync.Mutex
	activeRound string
	interactive bool
	controls    map[string]controlReceipt
	sequence    uint64
	rounds      map[string]*roundReceipt // runMu-owned; never persisted or shared across processes.
	ready       struct {
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
	}
}

type roundReceipt struct {
	identity [32]byte
	started  bool
	result   *NativeResult
}

type NativeResult struct {
	Outcome          string `json:"outcome"`
	Summary          string `json:"summary"`
	SessionReference string `json:"sessionReference"`
}

func OpenProcess(ctx context.Context, executable string, args, environment []string) (*Process, error) {
	command := exec.Command(executable, args...)
	command.Env = environment
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Raw adapter/native stderr is not a safe platform event.
	command.Stderr = io.Discard
	input, err := command.StdinPipe()
	if err != nil {
		return nil, errors.New("ADAPTER_PIPE_FAILED")
	}
	output, err := command.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, errors.New("ADAPTER_PIPE_FAILED")
	}
	if err := command.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, errors.New("ADAPTER_START_FAILED")
	}
	p := &Process{command: command, input: input, output: output, frames: make(chan Frame, 64), readDone: make(chan struct{}), processDone: make(chan struct{}), closing: make(chan struct{}), writeGate: make(chan struct{}, 1)}
	go func() { defer close(p.processDone); <-p.readDone; _ = command.Wait() }()
	go func() {
		defer close(p.readDone)
		scanner := bufio.NewScanner(output)
		scanner.Buffer(make([]byte, 4096), MaxFrameBytes)
		scanner.Split(jsonLine)
		for scanner.Scan() {
			line := append(append([]byte(nil), scanner.Bytes()...), '\n')
			frame, err := Decode(line, "up")
			if err != nil {
				return
			}
			select {
			case p.frames <- frame:
			case <-p.closing:
				return
			}
		}
	}()
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	key, err := p.send(handshake, "initialize", map[string]any{})
	if err != nil {
		p.Close()
		return nil, err
	}
	frame, err := p.receive(handshake)
	if err != nil || frame.Method != "ready" || frame.ReplyTo != key {
		p.Close()
		return nil, errors.New("ADAPTER_HANDSHAKE_FAILED")
	}
	if json.Unmarshal(frame.Payload, &p.ready) != nil || p.ready.Name == "" || p.ready.Version == "" || len(p.ready.Capabilities) > 16 {
		p.Close()
		return nil, errors.New("ADAPTER_HANDSHAKE_FAILED")
	}
	return p, nil
}

func (p *Process) ValidateReady(expected Descriptor) error {
	actual := append([]string(nil), p.ready.Capabilities...)
	wanted := append([]string(nil), expected.Capabilities...)
	sort.Strings(actual)
	sort.Strings(wanted)
	if p.ready.Name != expected.ID || p.ready.Version != expected.Version || !slices.Equal(actual, wanted) {
		return errors.New("ADAPTER_CAPABILITY_MISMATCH")
	}
	return nil
}

func (p *Process) send(ctx context.Context, method string, payload any) (string, error) {
	return p.sendPrepared(ctx, method, payload, nil)
}

func (p *Process) sendPrepared(ctx context.Context, method string, payload any, prepare func(string) error) (string, error) {
	select {
	case p.writeGate <- struct{}{}:
		defer func() { <-p.writeGate }()
	case <-ctx.Done():
		return "", ctx.Err()
	case <-p.closing:
		return "", errors.New("ADAPTER_STREAM_CLOSED")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	p.sequence++
	key := "supervisor:" + strconv.FormatUint(p.sequence, 10)
	body, err := json.Marshal(payload)
	if err != nil {
		return "", ErrInvalid
	}
	raw, err := json.Marshal(Frame{Protocol: Version, Method: method, Key: key, Payload: body})
	if err != nil {
		return "", ErrInvalid
	}
	raw = append(raw, '\n')
	if _, err = Decode(raw, "down"); err != nil {
		return "", err
	}
	if prepare != nil {
		if err := prepare(key); err != nil {
			return "", err
		}
	}
	done := make(chan error, 1)
	go func() { _, err := p.input.Write(raw); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			return "", errors.New("ADAPTER_WRITE_FAILED")
		}
		return key, nil
	case <-ctx.Done():
		p.input.Close()
		<-done
		return "", ctx.Err()
	}
}

func (p *Process) receive(ctx context.Context) (Frame, error) {
	if err := ctx.Err(); err != nil {
		return Frame{}, err
	}
	// Prefer already received terminal frames when the adapter subsequently exits.
	select {
	case f := <-p.frames:
		return f, nil
	default:
	}
	select {
	case f := <-p.frames:
		return f, nil
	case <-p.readDone:
		select {
		case f := <-p.frames:
			return f, nil
		default:
			return Frame{}, errors.New("ADAPTER_STREAM_CLOSED")
		}
	case <-ctx.Done():
		return Frame{}, ctx.Err()
	}
}

// Round never retries a start after stream failure. The outer execution owner
// decides whether a native session can be restored using a new authority.
func (p *Process) Round(ctx context.Context, roundKey string, inputs any, sessionReference string, started func(int) error, observe func(Frame)) (NativeResult, error) {
	p.runMu.Lock()
	defer p.runMu.Unlock()
	method := "execution.start"
	payload := map[string]any{"roundKey": roundKey, "inputs": inputs}
	if sessionReference != "" {
		method = "execution.resume"
		payload["sessionReference"] = sessionReference
	}
	identityBody, err := json.Marshal(map[string]any{"method": method, "payload": payload})
	if err != nil {
		return NativeResult{}, ErrInvalid
	}
	identity := sha256.Sum256(identityBody)
	if p.rounds == nil {
		p.rounds = make(map[string]*roundReceipt)
	}
	receipt := p.rounds[roundKey]
	if receipt != nil && receipt.identity != identity {
		return NativeResult{}, errors.New("ADAPTER_IDEMPOTENCY_CONFLICT")
	}
	if receipt == nil {
		if len(p.rounds) >= 256 {
			return NativeResult{}, errors.New("ADAPTER_SESSION_LIMIT")
		}
		receipt = &roundReceipt{identity: identity}
		p.rounds[roundKey] = receipt
	}
	// Only an already observed completion in this same Process permits a cached
	// finished frame without another started event. An ack alone never does.
	replay := receipt.started && receipt.result != nil
	p.controlMu.Lock()
	p.activeRound = roundKey
	var mode struct {
		Mode           string           `json:"mode"`
		AvailableTools []ToolDefinition `json:"availableTools"`
	}
	inputBody, _ := json.Marshal(inputs)
	_ = json.Unmarshal(inputBody, &mode)
	allowedTools := make(map[string]struct{}, len(mode.AvailableTools))
	for _, definition := range mode.AvailableTools {
		allowedTools[definition.Name] = struct{}{}
	}
	p.interactive = mode.Mode != "REVIEW" && observe != nil
	p.controls = make(map[string]controlReceipt)
	p.controlMu.Unlock()
	defer func() {
		p.controlMu.Lock()
		p.activeRound = ""
		p.interactive = false
		p.controls = nil
		p.controlMu.Unlock()
	}()
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	key, err := p.send(writeCtx, method, payload)
	cancel()
	if err != nil {
		return NativeResult{}, err
	}
	didStart := false
	for {
		frame, err := p.receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				interrupt, cancel := context.WithTimeout(context.Background(), time.Second)
				_, _ = p.send(interrupt, "execution.interrupt", map[string]any{"roundKey": roundKey})
				cancel()
			}
			p.Close()
			return NativeResult{}, err
		}
		var identity struct {
			RoundKey  string `json:"roundKey"`
			NativePID int    `json:"nativePid"`
		}
		if json.Unmarshal(frame.Payload, &identity) != nil {
			p.Close()
			return NativeResult{}, ErrInvalid
		}
		if frame.Method == "error" {
			p.Close()
			return NativeResult{}, errors.New("ADAPTER_EXECUTION_FAILED")
		}
		if identity.RoundKey != roundKey {
			p.Close()
			return NativeResult{}, errors.New("ADAPTER_ROUND_MISMATCH")
		}
		switch frame.Method {
		case "control.ack":
			if frame.ReplyTo != key {
				if !p.knownControl(frame.ReplyTo) {
					p.Close()
					return NativeResult{}, ErrInvalid
				}
			}
			// Receipt is not native startup, execution completion or authorization.
			continue
		case "execution.started":
			if frame.ReplyTo != key || didStart || replay || identity.NativePID <= 0 {
				p.Close()
				return NativeResult{}, errors.New("ADAPTER_NATIVE_START_INVALID")
			}
			didStart = true
			receipt.started = true
			if started != nil {
				if err := started(identity.NativePID); err != nil {
					p.Close()
					return NativeResult{}, err
				}
			}
		case "message", "plan", "activity":
			if observe != nil {
				observe(frame)
			}
		case "interaction.request", "interaction.closed":
			if !didStart || !p.interactionEnabled("interaction.request") || frame.ReplyTo != key {
				p.Close()
				return NativeResult{}, ErrUnsupported
			}
			observe(frame)
		case "tool.catalog", "tool.request", "tool.closed":
			if !didStart || !p.interactionEnabled("tool.request") || frame.ReplyTo != key {
				p.Close()
				return NativeResult{}, ErrUnsupported
			}
			if frame.Method == "tool.catalog" {
				var catalog ToolCatalogPayload
				if json.Unmarshal(frame.Payload, &catalog) != nil || len(catalog.ToolNames) != len(allowedTools) {
					p.Close()
					return NativeResult{}, ErrInvalid
				}
				for _, name := range catalog.ToolNames {
					if _, ok := allowedTools[name]; !ok {
						p.Close()
						return NativeResult{}, ErrUnsupported
					}
				}
			} else if frame.Method == "tool.request" {
				var request ToolRequestPayload
				if json.Unmarshal(frame.Payload, &request) != nil {
					p.Close()
					return NativeResult{}, ErrInvalid
				}
				if _, ok := allowedTools[request.ToolName]; !ok {
					p.Close()
					return NativeResult{}, ErrUnsupported
				}
			}
			observe(frame)
		case "tool.received":
			if !didStart || p.acceptControlReceipt(frame) != nil {
				p.Close()
				return NativeResult{}, ErrInvalid
			}
			if observe != nil {
				observe(frame)
			}
		case "interaction.received", "message.received", "execution.suspended":
			if frame.Method == "execution.suspended" && replay && frame.ReplyTo == key && receipt.result.Outcome == "SUSPENDED" {
				var result NativeResult
				_ = json.Unmarshal(frame.Payload, &result)
				result.Outcome = "SUSPENDED"
				if result != *receipt.result {
					p.Close()
					return NativeResult{}, errors.New("ADAPTER_REPLAY_MISMATCH")
				}
				return result, nil
			}
			if !didStart || p.acceptControlReceipt(frame) != nil {
				p.Close()
				return NativeResult{}, ErrInvalid
			}
			if observe != nil {
				observe(frame)
			}
			if frame.Method == "execution.suspended" {
				if err := p.finishControlReceipts(ctx, observe); err != nil {
					return NativeResult{}, err
				}
				var result NativeResult
				_ = json.Unmarshal(frame.Payload, &result)
				result.Outcome = "SUSPENDED"
				receipt.result = &result
				return result, nil
			}
		case "execution.finished":
			if frame.ReplyTo != key || (!didStart && !replay) {
				p.Close()
				return NativeResult{}, ErrInvalid
			}
			var result NativeResult
			if json.Unmarshal(frame.Payload, &result) != nil {
				p.Close()
				return NativeResult{}, ErrInvalid
			}
			switch result.Outcome {
			case "FINISHED", "FAILED", "CANCELLED":
				if replay && result != *receipt.result {
					p.Close()
					return NativeResult{}, errors.New("ADAPTER_REPLAY_MISMATCH")
				}
				receipt.result = &result
				if err := p.finishControlReceipts(ctx, observe); err != nil {
					return NativeResult{}, err
				}
				return result, nil
			default:
				p.Close()
				return NativeResult{}, ErrInvalid
			}
		default:
			p.Close()
			return NativeResult{}, ErrUnsupported
		}
	}
}

func (p *Process) Close() {
	p.closeOnce.Do(func() {
		close(p.closing)
		_ = p.input.Close()
		select {
		case <-p.processDone:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGTERM)
			select {
			case <-p.processDone:
			case <-time.After(2 * time.Second):
				_ = syscall.Kill(-p.command.Process.Pid, syscall.SIGKILL)
				// A detached descendant can inherit stdout. Close our endpoint
				// so that its pipe cannot keep the reader/reaper alive forever.
				_ = p.output.Close()
				<-p.processDone
			}
		}
		<-p.readDone
		_ = p.output.Close()
	})
}

func jsonLine(data []byte, atEOF bool) (int, []byte, error) {
	if index := bytes.IndexByte(data, '\n'); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) != 0 {
		return 0, nil, ErrInvalid
	}
	return 0, nil, nil
}
