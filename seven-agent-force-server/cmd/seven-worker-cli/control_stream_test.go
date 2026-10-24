package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestControlStreamPersistentDispatchAndOwnerFencing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &controlServer{spec: attemptSpec{TaskID: 1, AttemptID: 2, Generation: 3}, state: &supervisorState{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, State: "RUNNING", cancel: cancel}}
	client, server := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { s.handle(server); close(done) }()
	_ = client.SetDeadline(time.Now().Add(3 * time.Second))
	reader := bufio.NewReader(client)
	for i, command := range []string{"state", "diagnostics", "evaluation-state", "renew-authority", "interrupt"} {
		request := map[string]any{"schemaVersion": 2, "command": command, "taskId": "1", "attemptId": "2", "generation": "3", "ownerEpoch": "3", "leaseReceiptDigest": strings.Repeat("a", 64), "leaseExpiresAt": time.Now().Add(20 * time.Second).UTC().Format(time.RFC3339Nano)}
		if err := json.NewEncoder(client).Encode(map[string]any{"id": strconv.Itoa(i), "request": request}); err != nil {
			t.Fatal(err)
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var reply struct {
			ID       string `json:"id"`
			Response struct {
				OK     bool   `json:"ok"`
				Reason string `json:"reasonCode"`
			} `json:"response"`
		}
		if err := json.Unmarshal(line, &reply); err != nil {
			t.Fatal(err)
		}
		if reply.ID != strconv.Itoa(i) {
			t.Fatalf("wrong association: %s", line)
		}
		wantOK := i < 3
		if reply.Response.OK != wantOK {
			t.Fatalf("unexpected dispatch result: %s", line)
		}
	}
	if ctx.Err() != nil {
		t.Fatal("stale owner interrupted harness")
	}
	if err := json.NewEncoder(client).Encode(map[string]any{"id": "current-owner", "request": map[string]any{"command": "interrupt", "taskId": "1", "attemptId": "2", "generation": "3", "ownerEpoch": "4"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("current owner could not interrupt harness")
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler leaked")
	}
}

func TestControlStreamRejectsMalformedAndOversizedFrames(t *testing.T) {
	for _, frame := range []string{`{"id":"1","request":{"command":"state"},"extra":true}` + "\n", `{"id":"1","request":{}} {}` + "\n", strings.Repeat("x", controlStreamRequestLimit+1) + "\n"} {
		client, server := net.Pipe()
		done := make(chan struct{})
		go func() { (&controlServer{}).handle(server); close(done) }()
		_ = client.SetDeadline(time.Now().Add(time.Second))
		go func() { _, _ = io.WriteString(client, frame) }()
		_, _ = io.ReadAll(client)
		_ = client.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("invalid frame handler leaked")
		}
	}
}

func TestControlStreamLegacyStateStillCallable(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go (&controlServer{state: &supervisorState{State: "READY"}}).handle(server)
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_, _ = io.WriteString(client, "{\"command\":\"state\"}\n")
	var response map[string]any
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response["ok"] != true || response["id"] != nil {
		t.Fatalf("legacy response changed: %v", response)
	}
}

func TestControlStreamBridgeClosesBothDirections(t *testing.T) {
	for _, closeInput := range []bool{true, false} {
		input, producer := io.Pipe()
		consumer, output := io.Pipe()
		bridge, peer := net.Pipe()
		done := make(chan error, 1)
		go func() { done <- bridgeControlStream(input, output, bridge) }()
		if closeInput {
			_ = producer.Close()
		} else {
			_ = peer.Close()
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("bridge reader leaked on EOF")
		}
		_ = producer.Close()
		_ = consumer.Close()
		_ = peer.Close()
	}
}

func TestControlStreamBridgeForwardsPersistentFrames(t *testing.T) {
	input, producer := io.Pipe()
	consumer, output := io.Pipe()
	bridge, server := net.Pipe()
	bridgeDone := make(chan error, 1)
	serverDone := make(chan struct{})
	go func() { bridgeDone <- bridgeControlStream(input, output, bridge) }()
	go func() { (&controlServer{state: &supervisorState{State: "RUNNING"}}).handle(server); close(serverDone) }()
	defer producer.Close()
	defer consumer.Close()
	for _, id := range []string{"one", "two"} {
		if err := json.NewEncoder(producer).Encode(map[string]any{"id": id, "request": map[string]any{"command": "state"}}); err != nil {
			t.Fatal(err)
		}
		var reply struct {
			ID       string `json:"id"`
			Response struct {
				OK bool `json:"ok"`
			} `json:"response"`
		}
		if err := json.NewDecoder(consumer).Decode(&reply); err != nil {
			t.Fatal(err)
		}
		if reply.ID != id || !reply.Response.OK {
			t.Fatalf("bridge changed response: %+v", reply)
		}
	}
	_ = producer.Close()
	select {
	case err := <-bridgeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bridge did not exit")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("server did not exit")
	}
}

func TestControlStreamInterruptBypassesBlockedLifecycleCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &controlServer{state: &supervisorState{TaskID: 1, AttemptID: 2, Generation: 3, OwnerEpoch: 4, cancel: cancel}}
	s.startMu.Lock()
	defer s.startMu.Unlock()
	client, server := net.Pipe()
	defer client.Close()
	go s.handle(server)
	_ = client.SetDeadline(time.Now().Add(time.Second))
	for _, request := range []struct {
		id   string
		body any
	}{{"blocked", map[string]any{"command": "start-harness"}}, {"cancel", map[string]any{"command": "interrupt", "taskId": "1", "attemptId": "2", "generation": "3", "ownerEpoch": "4"}}} {
		if err := json.NewEncoder(client).Encode(map[string]any{"id": request.id, "request": request.body}); err != nil {
			t.Fatal(err)
		}
	}
	var response struct {
		ID       string `json:"id"`
		Response struct {
			OK bool `json:"ok"`
		} `json:"response"`
	}
	if err := json.NewDecoder(client).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != "cancel" || !response.Response.OK || ctx.Err() == nil {
		t.Fatalf("interrupt waited for lifecycle call: %+v", response)
	}
}
