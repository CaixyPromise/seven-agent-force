package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/worker/agentprotocol"
)

const controlRequestLimit = 56 << 10
const controlStreamRequestLimit = 64 << 10
const controlStreamResponseLimit = 64 << 10

type controlStreamRequest struct {
	ID      string          `json:"id"`
	Request json.RawMessage `json:"request"`
}

func readControlFrame(r *bufio.Reader, limit int) ([]byte, error) {
	line, err := r.ReadSlice('\n')
	if len(line) > limit || errors.Is(err, bufio.ErrBufferFull) {
		return nil, errors.New("control stream frame exceeds limit")
	}
	return line, err
}

// A stream is just correlated calls to the existing control dispatcher. It has
// no event authority, durable spool, or independent business methods.
func (s *controlServer) handleControlStream(conn net.Conn, reader *bufio.Reader, first []byte) bool {
	var marker map[string]json.RawMessage
	if json.Unmarshal(first, &marker) != nil || marker["id"] == nil {
		return false
	}
	line := first
	var writes sync.Mutex
	workers := make(chan struct{}, 8)
	for {
		if agentprotocol.ValidateDocument(line) != nil {
			return true
		}
		var request controlStreamRequest
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || request.ID == "" || len(request.ID) > 64 || len(request.Request) == 0 || len(request.Request) > controlRequestLimit {
			return true
		}
		var extra any
		if !errors.Is(decoder.Decode(&extra), io.EOF) {
			return true
		}
		wrapped := &controlResponseConn{Conn: conn, id: request.ID, writes: &writes}
		var command struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(request.Request, &command) != nil {
			return true
		}
		// The reader remains available to process cancellation while an adapter
		// write/start is outstanding. Saturation rejects work instead of queuing
		// ahead of interrupt. CLI ask uses its own one-shot connection.
		if command.Command == "interrupt" {
			s.handleControlRequest(wrapped, request.Request)
		} else if command.Command == "interaction-ask" {
			_ = json.NewEncoder(wrapped).Encode(map[string]any{"ok": false, "reasonCode": "SUPERVISOR_ASK_REQUIRES_ONESHOT"})
		} else {
			select {
			case workers <- struct{}{}:
				go func(body []byte, response *controlResponseConn) {
					defer func() { <-workers }()
					s.handleControlRequest(response, body)
				}(append([]byte(nil), request.Request...), wrapped)
			default:
				_ = json.NewEncoder(wrapped).Encode(map[string]any{"ok": false, "reasonCode": "SUPERVISOR_CONTROL_BUSY"})
			}
		}
		// Idle sessions expire; the Worker reconnects with a state snapshot.
		_ = conn.SetReadDeadline(time.Now().Add(time.Minute))
		var err error
		line, err = readControlFrame(reader, controlStreamRequestLimit)
		if err != nil {
			return true
		}
	}
}

type controlResponseConn struct {
	net.Conn
	id     string
	err    error
	writes *sync.Mutex
}

// Per-call deadlines cannot change the shared stream's reader deadline.
func (c *controlResponseConn) SetDeadline(time.Time) error { return nil }

func (c *controlResponseConn) Write(body []byte) (int, error) {
	if c.writes != nil {
		c.writes.Lock()
		defer c.writes.Unlock()
	}
	_ = c.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	frame, err := json.Marshal(struct {
		ID       string          `json:"id"`
		Response json.RawMessage `json:"response"`
	}{c.id, body})
	if err == nil && len(frame)+1 > controlStreamResponseLimit {
		err = errors.New("control response exceeds limit")
	}
	if err == nil {
		_, err = c.Conn.Write(append(frame, '\n'))
	}
	c.err = err
	if err != nil {
		return 0, err
	}
	return len(body), nil
}

func controlMain(args []string) error {
	if len(args) == 0 {
		return controlJSON()
	}
	if len(args) != 1 || args[0] != "--stream" {
		return errors.New("usage: control [--stream]")
	}
	socket := os.Getenv("SEVEN_CONTROL_SOCKET")
	if socket == "" {
		return errors.New("SEVEN_CONTROL_SOCKET is required")
	}
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return err
	}
	return bridgeControlStream(os.Stdin, os.Stdout, conn)
}

// Copy bounded JSONL frames without interpreting requests or responses. Either
// half closing unblocks the other half, including a reader waiting on stdin.
func bridgeControlStream(input io.ReadCloser, output io.WriteCloser, conn net.Conn) error {
	results := make(chan error, 2)
	copyFrames := func(dst io.Writer, src io.Reader, limit int) {
		r := bufio.NewReaderSize(src, limit+1)
		for {
			frame, err := readControlFrame(r, limit)
			if err != nil {
				results <- err
				return
			}
			if _, err = dst.Write(frame); err != nil {
				results <- err
				return
			}
		}
	}
	go copyFrames(conn, input, controlStreamRequestLimit)
	go copyFrames(output, conn, controlStreamResponseLimit)
	err := <-results
	_ = conn.Close()
	_ = input.Close()
	_ = output.Close()
	<-results
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
