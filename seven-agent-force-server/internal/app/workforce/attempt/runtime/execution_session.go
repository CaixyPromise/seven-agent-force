package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// The native answer cap is 16KiB before the machine/control envelope. Keep
// bounded room for that envelope below the CLI's 56KiB request ceiling.
const executionRequestLimit = 36 << 10
const executionResponseLimit = 64 << 10

// Close must unblock concurrent Read and Write and reap any owned process.
type executionTransport interface {
	io.Reader
	io.Writer
	io.Closer
}
type executionDial func(context.Context, string) (executionTransport, error)
type executionSessionKey struct{}

// ExecutionSession owns one Attempt's control transport. Each connection has
// one reader and one writer. Uncertain calls are never automatically replayed.
type ExecutionSession struct {
	ctx                                       context.Context
	cancel                                    context.CancelFunc
	dial                                      executionDial
	gate                                      chan struct{} // connection establishment only, never response dispatch
	slots                                     chan struct{}
	mu                                        sync.Mutex
	container                                 string
	connection                                *executionConnection
	lastErr                                   error
	handler                                   ExecutionNotificationHandler
	taskID, attemptID, generation, ownerEpoch int64
	onReconnect                               func(supervisorStateSnapshot)
}

func newExecutionSession(ctx context.Context, taskID, attemptID, generation, ownerEpoch int64, dial executionDial) *ExecutionSession {
	lifetime, cancel := context.WithCancel(ctx)
	s := &ExecutionSession{ctx: lifetime, cancel: cancel, dial: dial, gate: make(chan struct{}, 1), slots: make(chan struct{}, 16), taskID: taskID, attemptID: attemptID, generation: generation, ownerEpoch: ownerEpoch}
	context.AfterFunc(lifetime, func() { _ = s.Close() })
	return s
}

func executionSessionScope(ctx context.Context, taskID, attemptID, generation, ownerEpoch int64, sinks ...EventSink) (context.Context, *ExecutionSession) {
	s := newExecutionSession(ctx, taskID, attemptID, generation, ownerEpoch, dialDockerExecution)
	if len(sinks) > 0 && sinks[0] != nil {
		s.onReconnect = func(snapshot supervisorStateSnapshot) {
			if snapshot.ObservationSequence > 0 {
				sinks[0]("harness.observation_window", map[string]any{"lastSequence": strconv.FormatInt(snapshot.ObservationSequence, 10), "gap": snapshot.ObservationGap, "scope": "bounded-in-memory"})
			}
		}
	}
	return context.WithValue(ctx, executionSessionKey{}, s), s
}

// SetNotificationsHandler installs a Worker-owned consumer. It must honor ctx
// cancellation and must not call Close (Close joins the dispatcher). Returning
// nil does not send a receipt: persist first, then explicitly SendLocalReceipt.
func (s *ExecutionSession) SetNotificationsHandler(handler ExecutionNotificationHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// Err reports the most recent asynchronous transport/notification failure,
// including idle overflow. It remains observable after a successful reconnect.
func (s *ExecutionSession) Err() error { s.mu.Lock(); defer s.mu.Unlock(); return s.lastErr }

func (s *ExecutionSession) Close() error {
	s.cancel()
	// Cancellation closes an active connection before waiting for establishment.
	s.mu.Lock()
	c := s.connection
	s.mu.Unlock()
	if c != nil {
		c.fail(context.Canceled)
	}
	s.gate <- struct{}{}
	s.mu.Lock()
	c = s.connection
	s.mu.Unlock()
	if c != nil {
		c.fail(context.Canceled)
		c.wg.Wait()
	}
	<-s.gate
	return nil
}

func (s *ExecutionSession) connected(ctx context.Context, container string) (*executionConnection, error) {
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.gate }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.container != "" && s.container != container {
		s.mu.Unlock()
		return nil, errors.New("execution session container mismatch")
	}
	s.container = container
	c := s.connection
	s.mu.Unlock()
	if c != nil {
		select {
		case <-c.ctx.Done():
			if ctx.Value(executionNotificationContextKey{}) == c {
				return nil, c.err()
			}
			c.wg.Wait()
		default:
			return c, nil
		}
	}
	t, err := s.dial(ctx, container)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("execution control connection failed")
	}
	if err = ctx.Err(); err != nil {
		_ = t.Close()
		return nil, err
	}
	c = newExecutionConnection(s, t)
	s.mu.Lock()
	s.connection = c
	s.mu.Unlock()
	c.start()
	snapshot, err := c.call(ctx, map[string]any{"command": "state"})
	if err == nil && (!snapshot.OK || snapshot.State.TaskID != s.taskID || snapshot.State.AttemptID != s.attemptID || snapshot.State.Generation != s.generation || snapshot.State.OwnerEpoch != s.ownerEpoch) {
		err = errors.New("execution session snapshot identity or owner mismatch")
	}
	if err != nil {
		c.fail(err)
		c.wg.Wait()
		return nil, err
	}
	close(c.ready) // notifications are never delivered before authority validation
	if s.onReconnect != nil {
		s.onReconnect(snapshot.State)
	}
	return c, nil
}

func (s *ExecutionSession) Call(ctx context.Context, container string, request any) (supervisorResponse, error) {
	var empty supervisorResponse
	raw, err := s.CallRaw(ctx, container, request)
	if err != nil {
		return empty, err
	}
	if json.Unmarshal(raw, &empty) != nil {
		return empty, errors.New("invalid execution control response")
	}
	return empty, nil
}

// CallRaw preserves unknown response fields for additive Supervisor commands.
// Like Call it never retries a request whose outcome may be uncertain.
func (s *ExecutionSession) CallRaw(ctx context.Context, container string, request any) (json.RawMessage, error) {
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, errors.New("execution control queue is full")
	}
	defer func() { <-s.slots }()
	callCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	defer cancel()
	c, err := s.connected(callCtx, container)
	if err != nil {
		return nil, err
	}
	return c.callRaw(callCtx, request)
}

// Notify dispatches a polled notification through the same bounded queue as
// unsolicited {"notification": ...} frames. Success means the handler completed,
// not just queue admission. A durable handler must Put before returning nil.
func (s *ExecutionSession) Notify(ctx context.Context, notification json.RawMessage) error {
	if ctx.Value(executionNotificationContextKey{}) != nil {
		return errors.New("execution notification cannot dispatch recursively")
	}
	if len(notification) == 0 || len(notification) > executionResponseLimit || !json.Valid(notification) {
		return errors.New("invalid execution notification")
	}
	s.mu.Lock()
	c := s.connection
	s.mu.Unlock()
	if c == nil {
		return errors.New("execution control is not connected")
	}
	select {
	case <-c.ready:
	default:
		return errors.New("execution control is not validated")
	}
	n := executionNotification{body: append(json.RawMessage(nil), notification...), done: make(chan error, 1), ctx: ctx}
	if err := c.enqueue(n); err != nil {
		return err
	}
	select {
	case err := <-n.done:
		return err
	case <-ctx.Done():
		c.fail(ctx.Err())
		return ctx.Err()
	case <-c.ctx.Done():
		return c.err()
	}
}

// SendLocalReceipt explicitly acknowledges a contiguous, durably persisted
// notification prefix to the CLI. It does not delete Worker spool entries and
// confers no Server authority. The caller owns the contiguous-prefix proof.
func (s *ExecutionSession) SendLocalReceipt(ctx context.Context, container string, through int64) error {
	if through <= 0 {
		return errors.New("invalid execution local receipt")
	}
	raw, err := s.CallRaw(ctx, container, map[string]any{"schemaVersion": 2, "command": "notifications-received", "through": strconv.FormatInt(through, 10), "taskId": strconv.FormatInt(s.taskID, 10), "attemptId": strconv.FormatInt(s.attemptID, 10), "generation": strconv.FormatInt(s.generation, 10), "ownerEpoch": strconv.FormatInt(s.ownerEpoch, 10)})
	if err != nil {
		return err
	}
	var reply struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal(raw, &reply) != nil || !reply.OK {
		return errors.New("execution local receipt rejected")
	}
	return nil
}

type dockerExecutionTransport struct {
	io.ReadCloser
	io.WriteCloser
	cmd  *exec.Cmd
	once sync.Once
}

func (t *dockerExecutionTransport) Close() error {
	t.once.Do(func() {
		_ = t.WriteCloser.Close()
		_ = t.ReadCloser.Close()
		if t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		_ = t.cmd.Wait()
	})
	return nil
}
func dialDockerExecution(ctx context.Context, container string) (executionTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd := exec.Command("docker", "exec", "-i", container, "/usr/local/bin/seven-worker-cli", "control", "--stream")
	cmd.WaitDelay = time.Second
	cmd.Stderr = io.Discard
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		_ = input.Close()
		_ = output.Close()
		return nil, fmt.Errorf("start execution control stream: %w", err)
	}
	return &dockerExecutionTransport{ReadCloser: output, WriteCloser: input, cmd: cmd}, nil
}
