package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type reliableRequest struct {
	ID      string                     `json:"id"`
	Request map[string]json.RawMessage `json:"request"`
}

func (r reliableRequest) command() string {
	var command string
	_ = json.Unmarshal(r.Request["command"], &command)
	return command
}

func reliableSession(t *testing.T, handler func(net.Conn, reliableRequest) bool) *ExecutionSession {
	t.Helper()
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			decoder := json.NewDecoder(server)
			for {
				var r reliableRequest
				if decoder.Decode(&r) != nil {
					return
				}
				if r.command() == "state" {
					if sessionTestReply(server, r.ID, 4) != nil {
						return
					}
					continue
				}
				if !handler(server, r) {
					return
				}
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func testDeadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestExecutionSessionConcurrentOutOfOrderPreservesUnknownFields(t *testing.T) {
	var requests []reliableRequest
	s := reliableSession(t, func(conn net.Conn, r reliableRequest) bool {
		requests = append(requests, r)
		if len(requests) == 2 {
			for i := 1; i >= 0; i-- {
				request := requests[i]
				if json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "response": map[string]any{"ok": true, "unknown": map[string]any{"command": request.command(), "bigId": "9007199254740993"}, "notifications": []any{map[string]any{"sequence": "1"}}, "through": "1", "hasMore": true}}) != nil {
					return false
				}
			}
		}
		return true
	})
	ctx := testDeadline(t)
	var wg sync.WaitGroup
	for _, command := range []string{"slow", "control"} {
		wg.Add(1)
		go func(command string) {
			defer wg.Done()
			raw, err := s.CallRaw(ctx, "container", map[string]any{"command": command})
			if err != nil {
				t.Error(err)
				return
			}
			var reply struct {
				Unknown struct {
					Command string `json:"command"`
					BigID   string `json:"bigId"`
				} `json:"unknown"`
				HasMore bool `json:"hasMore"`
			}
			if json.Unmarshal(raw, &reply) != nil || reply.Unknown.Command != command || reply.Unknown.BigID != "9007199254740993" || !reply.HasMore {
				t.Errorf("lost/associated unknown fields: %s", raw)
			}
		}(command)
	}
	wg.Wait()
}

func TestExecutionSessionAnswerEnvelopeHasBoundedHeadroom(t *testing.T) {
	s := reliableSession(t, func(conn net.Conn, request reliableRequest) bool {
		return json.NewEncoder(conn).Encode(map[string]any{"id": request.ID, "response": map[string]any{"ok": true}}) == nil
	})
	ctx := testDeadline(t)
	if _, err := s.CallRaw(ctx, "container", map[string]any{"command": "agent-control", "payload": strings.Repeat("a", 17<<10)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CallRaw(ctx, "container", map[string]any{"command": "agent-control", "payload": strings.Repeat("a", 33<<10)}); err == nil {
		t.Fatal("unbounded control request accepted")
	}
}

func TestExecutionSessionNotificationsDoNotBlockControlsAndOverflowIsVisible(t *testing.T) {
	entered := make(chan struct{})
	exited := make(chan struct{})
	s := reliableSession(t, func(conn net.Conn, r reliableRequest) bool { return sessionTestReply(conn, r.ID, 4) == nil })
	s.SetNotificationsHandler(func(ctx context.Context, raw json.RawMessage) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	})
	ctx := testDeadline(t)
	if _, err := s.Call(ctx, "container", map[string]any{"command": "warm"}); err != nil {
		t.Fatal(err)
	}
	notified := make(chan error, 1)
	go func() { notified <- s.Notify(ctx, json.RawMessage(`{"sequence":"1"}`)) }()
	<-entered
	if _, err := s.Call(ctx, "container", map[string]any{"command": "interrupt"}); err != nil {
		t.Fatal("notification blocked control", err)
	}
	s.mu.Lock()
	c := s.connection
	s.mu.Unlock()
	for i := 0; i < cap(c.notifications); i++ {
		if err := c.enqueue(executionNotification{body: json.RawMessage(`{}`), ctx: ctx}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.enqueue(executionNotification{body: json.RawMessage(`{}`), ctx: ctx}); err == nil {
		t.Fatal("overflow silently dropped")
	}
	if err := s.Err(); err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("idle overflow not observable: %v", err)
	}
	if err := <-notified; err == nil {
		t.Fatal("failed notification received success")
	}
	_ = s.Close()
	<-exited
}

func TestExecutionSessionNotifyWaitsForDurableHandlerAndExplicitReceipt(t *testing.T) {
	ctx := testDeadline(t)
	entered := make(chan struct{})
	persisted := make(chan struct{})
	receipt := make(chan reliableRequest, 1)
	s := reliableSession(t, func(conn net.Conn, r reliableRequest) bool {
		if r.command() == "notifications-received" {
			receipt <- r
		}
		return sessionTestReply(conn, r.ID, 4) == nil
	})
	s.SetNotificationsHandler(func(ctx context.Context, frame json.RawMessage) error {
		close(entered)
		select {
		case <-persisted:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if _, err := s.Call(ctx, "container", map[string]any{"command": "warm"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Notify(ctx, json.RawMessage(`{"sequence":"1","frame":{"key":"event:1"}}`)) }()
	<-entered
	select {
	case <-done:
		t.Fatal("Notify confirmed before persistence")
	default:
	}
	select {
	case <-receipt:
		t.Fatal("automatic local receipt")
	default:
	}
	close(persisted)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := s.SendLocalReceipt(ctx, "container", 1); err != nil {
		t.Fatal(err)
	}
	r := <-receipt
	for field, want := range map[string]string{"schemaVersion": "2", "taskId": `"1"`, "attemptId": `"2"`, "generation": `"3"`, "ownerEpoch": `"4"`, "through": `"1"`} {
		if string(r.Request[field]) != want {
			t.Errorf("receipt %s: %s", field, r.Request[field])
		}
	}
}

func TestExecutionSessionNotificationFailureIsSafeAndNoHandlerFailsClosed(t *testing.T) {
	for _, install := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "failure"}[install], func(t *testing.T) {
			s := reliableSession(t, func(conn net.Conn, r reliableRequest) bool { return sessionTestReply(conn, r.ID, 4) == nil })
			ctx := testDeadline(t)
			if install {
				s.SetNotificationsHandler(func(context.Context, json.RawMessage) error { return errors.New("secret provider body") })
			}
			if _, err := s.Call(ctx, "container", map[string]any{"command": "warm"}); err != nil {
				t.Fatal(err)
			}
			if err := s.Notify(ctx, json.RawMessage(`{"critical":true}`)); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("notification failure: %v", err)
			}
			_ = s.Close()
			if s.Err() == nil {
				t.Fatal("failure not observable")
			}
		})
	}
}

func TestExecutionSessionReconnectJoinsOldNotificationConsumer(t *testing.T) {
	ctx := testDeadline(t)
	entered := make(chan struct{})
	exited := make(chan struct{})
	calls := 0
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		calls++
		if calls == 2 {
			select {
			case <-exited:
			default:
				t.Error("reconnected before old consumer exit")
			}
		}
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			decoder := json.NewDecoder(server)
			for {
				var r reliableRequest
				if decoder.Decode(&r) != nil {
					return
				}
				if r.command() == "disconnect" {
					return
				}
				if sessionTestReply(server, r.ID, 4) != nil {
					return
				}
			}
		}()
		return client, nil
	})
	defer s.Close()
	s.SetNotificationsHandler(func(ctx context.Context, frame json.RawMessage) error {
		close(entered)
		<-ctx.Done()
		close(exited)
		return ctx.Err()
	})
	if _, err := s.Call(ctx, "container", map[string]any{"command": "warm"}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Notify(ctx, json.RawMessage(`{}`)) }()
	<-entered
	if _, err := s.Call(ctx, "container", map[string]any{"command": "disconnect"}); err == nil {
		t.Fatal("EOF accepted")
	}
	if _, err := s.Call(ctx, "container", map[string]any{"command": "control"}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("old notification succeeded")
	}
}

func TestExecutionSessionRejectsNotificationsBeforeAuthority(t *testing.T) {
	ctx := testDeadline(t)
	handled := make(chan struct{}, 1)
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			var r reliableRequest
			_ = json.NewDecoder(server).Decode(&r)
			_ = json.NewEncoder(server).Encode(map[string]any{"notification": map[string]any{"critical": true}})
			_ = sessionTestReply(server, r.ID, 999)
		}()
		return client, nil
	})
	defer s.Close()
	s.SetNotificationsHandler(func(context.Context, json.RawMessage) error { handled <- struct{}{}; return nil })
	if _, err := s.Call(ctx, "container", map[string]any{"command": "start-harness"}); err == nil {
		t.Fatal("unvalidated owner accepted")
	}
	select {
	case <-handled:
		t.Fatal("unvalidated notification dispatched")
	default:
	}
}

type shortExecutionWrite struct{ executionTransport }

func (t shortExecutionWrite) Write(b []byte) (int, error) { return len(b) - 1, nil }
func TestExecutionSessionShortWriteAndConcurrentClose(t *testing.T) {
	ctx := testDeadline(t)
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		client, server := net.Pipe()
		go func() { defer server.Close(); _, _ = io.Copy(io.Discard, server) }()
		return shortExecutionWrite{client}, nil
	})
	if _, err := s.Call(ctx, "container", map[string]any{"command": "state"}); err == nil {
		t.Fatal("short write accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = s.Close() }()
	}
	wg.Wait()
}

func TestExecutionSessionAmbiguousEnvelopeFailsClosed(t *testing.T) {
	for _, body := range []string{`{"id":"1","id":"2","response":{}}`, `{"id":"1","response":{},"notification":{}}`, `{"id":"1","response":null}`, `{"id":"1","response":[]}`, `{"id":"1","response":{},"unknown":true}`, `{"id":"1","response":{}} {}`} {
		t.Run(body, func(t *testing.T) {
			s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
				client, server := net.Pipe()
				go func() {
					defer server.Close()
					var r reliableRequest
					_ = json.NewDecoder(server).Decode(&r)
					_, _ = io.WriteString(server, body+"\n")
				}()
				return client, nil
			})
			defer s.Close()
			if _, err := s.Call(testDeadline(t), "container", map[string]any{"command": "start-harness"}); err == nil {
				t.Fatal("ambiguous frame accepted")
			}
		})
	}
}

func TestExecutionSessionCallCancellationReapsBothIOWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	s := reliableSession(t, func(conn net.Conn, r reliableRequest) bool {
		close(started)
		_, _ = io.Copy(io.Discard, conn)
		return false
	})
	done := make(chan error, 1)
	go func() { _, err := s.Call(ctx, "container", map[string]any{"command": "blocked"}); done <- err }()
	<-started
	s.mu.Lock()
	old := s.connection
	s.mu.Unlock()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation identity lost: %v", err)
	}
	joined := make(chan struct{})
	go func() { old.ioWG.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("call returned with live I/O workers")
	}
}
