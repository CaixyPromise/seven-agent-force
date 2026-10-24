package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
)

// ExecutionNotificationHandler runs separately from control I/O. It must
// honor cancellation; all connection goroutines, including this handler, are
// joined before reconnect/Close returns. Errors must not contain frame content.
type ExecutionNotificationHandler func(context.Context, json.RawMessage) error

type executionNotificationContextKey struct{}

type executionReply struct {
	body json.RawMessage
	err  error
}
type executionWrite struct {
	body []byte
	done chan error
}
type executionNotification struct {
	body json.RawMessage
	ctx  context.Context
	done chan error
}

type executionEnvelope struct {
	ID           string
	Response     json.RawMessage
	Notification json.RawMessage
}

// Reject duplicate envelope fields before unmarshalling can erase ambiguous
// correlation. The response remains opaque so additive fields are preserved.
func decodeExecutionEnvelope(line []byte) (executionEnvelope, error) {
	var envelope executionEnvelope
	invalid := errors.New("invalid execution control frame")
	d := json.NewDecoder(bytes.NewReader(line))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return envelope, invalid
	}
	seen := make(map[string]bool, 3)
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return envelope, invalid
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return envelope, invalid
		}
		seen[key] = true
		switch key {
		case "id":
			err = d.Decode(&envelope.ID)
		case "response":
			err = d.Decode(&envelope.Response)
		case "notification":
			err = d.Decode(&envelope.Notification)
		default:
			return envelope, invalid
		}
		if err != nil {
			return envelope, invalid
		}
	}
	if token, err = d.Token(); err != nil || token != json.Delim('}') {
		return envelope, invalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return envelope, invalid
	}
	if seen["notification"] && (seen["id"] || seen["response"]) {
		return envelope, invalid
	}
	return envelope, nil
}

type executionConnection struct {
	session       *ExecutionSession
	t             executionTransport
	ctx           context.Context
	cancel        context.CancelFunc
	ready         chan struct{}
	writes        chan executionWrite
	notifications chan executionNotification
	mu            sync.Mutex
	pending       map[string]chan executionReply
	sequence      uint64
	failure       error
	once          sync.Once
	wg            sync.WaitGroup
	ioWG          sync.WaitGroup
}

func newExecutionConnection(s *ExecutionSession, t executionTransport) *executionConnection {
	ctx, cancel := context.WithCancel(s.ctx)
	return &executionConnection{session: s, t: t, ctx: ctx, cancel: cancel, ready: make(chan struct{}), writes: make(chan executionWrite, 16), notifications: make(chan executionNotification, 32), pending: make(map[string]chan executionReply)}
}

func (c *executionConnection) start() {
	c.wg.Add(3)
	c.ioWG.Add(2)
	go func() { defer c.wg.Done(); defer c.ioWG.Done(); c.readLoop() }()
	go func() { defer c.wg.Done(); defer c.ioWG.Done(); c.writeLoop() }()
	go func() { defer c.wg.Done(); c.dispatchLoop() }()
}

func (c *executionConnection) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return c.failure
	}
	return context.Canceled
}

func (c *executionConnection) fail(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.failure = err
		c.pending = make(map[string]chan executionReply)
		c.mu.Unlock()
		c.session.mu.Lock()
		c.session.lastErr = err
		c.session.mu.Unlock()
		c.cancel()
		_ = c.t.Close()
	})
}

func (c *executionConnection) call(ctx context.Context, request any) (supervisorResponse, error) {
	var response supervisorResponse
	raw, err := c.callRaw(ctx, request)
	if err == nil && json.Unmarshal(raw, &response) != nil {
		err = errors.New("invalid execution control response")
		c.fail(err)
	}
	return response, err
}

func (c *executionConnection) callRaw(ctx context.Context, request any) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, errors.New("invalid execution control request")
	}
	if len(body) > 32<<10 {
		return nil, errors.New("execution control request exceeds limit")
	}
	c.mu.Lock()
	if c.failure != nil {
		err = c.failure
		c.mu.Unlock()
		return nil, err
	}
	c.sequence++
	id := strconv.FormatUint(c.sequence, 10)
	result := make(chan executionReply, 1)
	c.pending[id] = result
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	frame, _ := json.Marshal(struct {
		ID      string          `json:"id"`
		Request json.RawMessage `json:"request"`
	}{id, body})
	if len(frame)+1 > executionRequestLimit {
		return nil, errors.New("execution control frame exceeds limit")
	}
	if err = c.write(ctx, append(frame, '\n')); err != nil {
		return nil, err
	}
	select {
	case reply := <-result:
		return reply.body, reply.err
	case <-ctx.Done():
		c.fail(ctx.Err())
		c.ioWG.Wait()
		return nil, ctx.Err()
	case <-c.ctx.Done():
		c.ioWG.Wait()
		select {
		case reply := <-result:
			return reply.body, reply.err
		default:
		}
		return nil, c.err()
	}
}

func (c *executionConnection) write(ctx context.Context, body []byte) error {
	w := executionWrite{body: body, done: make(chan error, 1)}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		c.ioWG.Wait()
		return c.err()
	default:
	}
	select {
	case c.writes <- w:
	default:
		return errors.New("execution writer queue is full")
	}
	select {
	case err := <-w.done:
		return err
	case <-ctx.Done():
		c.fail(ctx.Err())
		c.ioWG.Wait()
		return ctx.Err()
	case <-c.ctx.Done():
		return c.err()
	}
}

func (c *executionConnection) writeLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case w := <-c.writes:
			if c.ctx.Err() != nil {
				return
			}
			n, err := c.t.Write(w.body)
			if err != nil || n != len(w.body) {
				err = errors.New("execution control write failed")
				c.fail(err)
			}
			w.done <- err
			if err != nil {
				return
			}
		}
	}
}

func (c *executionConnection) readLoop() {
	r := bufio.NewReaderSize(c.t, executionResponseLimit+1)
	for {
		line, err := r.ReadSlice('\n')
		if err != nil || len(line) > executionResponseLimit {
			failure := errors.New("execution control read failed or frame exceeds limit")
			// Existing evaluation recovery recognizes clean/truncated EOF. Preserve
			// those safe sentinel identities without exposing arbitrary I/O bodies.
			if errors.Is(err, io.EOF) {
				if len(line) == 0 {
					failure = io.EOF
				} else {
					failure = io.ErrUnexpectedEOF
				}
			}
			c.fail(failure)
			return
		}
		envelope, decodeErr := decodeExecutionEnvelope(line)
		if decodeErr != nil {
			c.fail(errors.New("invalid execution control frame"))
			return
		}
		if envelope.ID == "" && len(envelope.Response) == 0 && len(envelope.Notification) > 0 {
			if err := c.enqueue(executionNotification{body: envelope.Notification, ctx: c.ctx}); err != nil {
				return
			}
			continue
		}
		if envelope.ID == "" || len(envelope.ID) > 64 || len(envelope.Notification) > 0 || len(envelope.Response) == 0 || bytes.TrimSpace(envelope.Response)[0] != '{' {
			c.fail(errors.New("invalid execution control response"))
			return
		}
		c.mu.Lock()
		result, ok := c.pending[envelope.ID]
		delete(c.pending, envelope.ID)
		c.mu.Unlock()
		if !ok {
			c.fail(errors.New("execution control response association mismatch"))
			return
		}
		result <- executionReply{body: envelope.Response}
	}
}

func (c *executionConnection) enqueue(n executionNotification) error {
	if c.ctx.Err() != nil {
		return c.err()
	}
	select {
	case c.notifications <- n:
		return nil
	default:
		err := errors.New("execution notification queue is full")
		c.fail(err)
		return err
	}
}

func (c *executionConnection) dispatchLoop() {
	select {
	case <-c.ctx.Done():
		return
	case <-c.ready:
	}
	for {
		select {
		case <-c.ctx.Done():
			return
		case n := <-c.notifications:
			if c.ctx.Err() != nil {
				return
			}
			c.session.mu.Lock()
			handler := c.session.handler
			c.session.mu.Unlock()
			err := errors.New("execution notification handler unavailable")
			if handler != nil {
				ctx, cancel := context.WithCancel(context.WithValue(c.ctx, executionNotificationContextKey{}, c))
				stop := context.AfterFunc(n.ctx, cancel)
				err = handler(ctx, n.body)
				stop()
				cancel()
				if err != nil {
					err = errors.New("execution notification handling failed")
				}
			}
			if n.done != nil {
				n.done <- err
			}
			if err != nil {
				c.fail(err)
				return
			}
		}
	}
}
