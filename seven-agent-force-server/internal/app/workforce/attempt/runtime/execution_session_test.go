package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func sessionTestReply(writer io.Writer, id string, owner int64) error {
	return json.NewEncoder(writer).Encode(map[string]any{"id": id, "response": supervisorResponse{OK: true, State: supervisorStateSnapshot{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: owner, State: "RUNNING"}}})
}

func TestExecutionSessionReuseAndReconnectSnapshotWithoutReplay(t *testing.T) {
	var mu sync.Mutex
	var commands []string
	dials := 0
	var servers sync.WaitGroup
	dial := func(context.Context, string) (executionTransport, error) {
		dials++
		connection := dials
		client, server := net.Pipe()
		servers.Add(1)
		go func() {
			defer servers.Done()
			defer server.Close()
			decoder := json.NewDecoder(server)
			for {
				var request struct {
					ID      string `json:"id"`
					Request struct {
						Command string `json:"command"`
					} `json:"request"`
				}
				if decoder.Decode(&request) != nil {
					return
				}
				mu.Lock()
				commands = append(commands, request.Request.Command)
				mu.Unlock()
				if connection == 1 && request.Request.Command == "start-harness" {
					return
				}
				if sessionTestReply(server, request.ID, 4) != nil {
					return
				}
			}
		}()
		return client, nil
	}
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, dial)
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		if _, err := s.Call(ctx, "container", map[string]any{"command": "diagnostics"}); err != nil {
			t.Fatal(err)
		}
	}
	if dials != 1 {
		t.Fatalf("per-request connection: %d", dials)
	}
	if _, err := s.Call(ctx, "container", map[string]any{"command": "start-harness"}); err == nil {
		t.Fatal("lost response accepted")
	}
	if _, err := s.Call(ctx, "container", map[string]any{"command": "evaluation-state"}); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	servers.Wait()
	mu.Lock()
	defer mu.Unlock()
	if got := strings.Join(commands, ","); got != "state,diagnostics,diagnostics,start-harness,state,evaluation-state" {
		t.Fatalf("unexpected replay: %s", got)
	}
}

func TestExecutionSessionFaultsCloseReaders(t *testing.T) {
	for _, fault := range []string{"wrong-id", "oversized", "truncated", "blocked-read", "blocked-write", "stale-owner"} {
		t.Run(fault, func(t *testing.T) {
			done := make(chan struct{})
			dial := func(context.Context, string) (executionTransport, error) {
				client, server := net.Pipe()
				go func() {
					defer close(done)
					defer server.Close()
					if fault == "blocked-write" {
						_, _ = server.Write([]byte("wait"))
						return
					}
					var request struct {
						ID string `json:"id"`
					}
					if json.NewDecoder(server).Decode(&request) != nil {
						return
					}
					switch fault {
					case "wrong-id":
						_ = sessionTestReply(server, "other", 4)
					case "oversized":
						_, _ = io.WriteString(server, strings.Repeat("x", executionResponseLimit+2))
					case "truncated":
						_, _ = io.WriteString(server, `{"id":`)
					case "blocked-read":
						_, _ = io.Copy(io.Discard, server)
					case "stale-owner":
						_ = sessionTestReply(server, request.ID, 5)
					}
				}()
				return client, nil
			}
			s := newExecutionSession(context.Background(), 1, 2, 3, 4, dial)
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if _, err := s.Call(ctx, "container", map[string]any{"command": "state"}); err == nil {
				t.Fatal("fault accepted")
			}
			_ = s.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("pipe goroutine leaked")
			}
		})
	}
}

func TestExecutionSessionCloseCancelsInFlightAndBoundsQueue(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			_, _ = bufio.NewReader(server).ReadBytes('\n')
			close(started)
			_, _ = io.Copy(io.Discard, server)
			close(done)
		}()
		return client, nil
	})
	callDone := make(chan error, 1)
	go func() {
		_, err := s.Call(context.Background(), "container", map[string]any{"command": "state"})
		callDone <- err
	}()
	<-started
	// Saturate the bounded admission slots without starting more readers.
	for i := 1; i < cap(s.slots); i++ {
		s.slots <- struct{}{}
	}
	if _, err := s.Call(context.Background(), "container", nil); err == nil {
		t.Fatal("full queue accepted")
	}
	closed := make(chan struct{})
	go func() { _ = s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close blocked")
	}
	if err := <-callDone; err == nil {
		t.Fatal("closed call succeeded")
	}
	<-done
	if _, err := s.Call(context.Background(), "container", nil); err == nil {
		t.Fatal("closed session reopened")
	}
}

func TestExecutionSessionLocalProcessIsReaped(t *testing.T) {
	cmd := exec.Command("cat")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	transport := &dockerExecutionTransport{ReadCloser: output, WriteCloser: input, cmd: cmd}
	if err := transport.Close(); err != nil {
		t.Fatal(err)
	}
	if cmd.ProcessState == nil {
		t.Fatal("process was not waited")
	}
	_ = transport.Close()
}

func TestExecutionSessionContextRoutesExistingControlHelper(t *testing.T) {
	called := false
	s := newExecutionSession(context.Background(), 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) { called = true; return nil, io.ErrClosedPipe })
	defer s.Close()
	ctx := context.WithValue(context.Background(), executionSessionKey{}, s)
	if _, err := callSupervisorContext(ctx, "no-docker", map[string]any{"command": "state"}); err == nil || !called {
		t.Fatalf("helper bypassed session: %v", err)
	}
}

func TestExecutionSessionParentCancellationClosesIdleConnection(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	s := newExecutionSession(parent, 1, 2, 3, 4, func(context.Context, string) (executionTransport, error) {
		client, server := net.Pipe()
		go func() {
			defer close(done)
			defer server.Close()
			decoder := json.NewDecoder(server)
			for {
				var request struct {
					ID string `json:"id"`
				}
				if decoder.Decode(&request) != nil {
					return
				}
				if sessionTestReply(server, request.ID, 4) != nil {
					return
				}
			}
		}()
		return client, nil
	})
	defer s.Close()
	ctx, timeout := context.WithTimeout(parent, time.Second)
	defer timeout()
	if _, err := s.Call(ctx, "container", map[string]any{"command": "state"}); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle connection survived parent cancellation")
	}
}
