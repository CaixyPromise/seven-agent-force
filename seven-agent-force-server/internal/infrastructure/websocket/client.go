package websocket

import (
	"context"
	stderrors "errors"
	"sync"
	"time"

	coderws "github.com/coder/websocket"
	"go.uber.org/zap"
)

type Client interface {
	Start(ctx context.Context) error
	Close() error

	SendText(ctx context.Context, payload string) error
	SendBinary(ctx context.Context, payload []byte) error
	SendJSON(ctx context.Context, payload any) error

	OnText(func(context.Context, string))
	OnBinary(func(context.Context, []byte))
	OnError(func(context.Context, error))
	OnClose(func(context.Context, error))
}

type client struct {
	opts          clientSettings
	hooks         hookSet
	sendQueue     chan outboundMessage
	callbackQueue chan func()
	connMu        sync.RWMutex
	conn          *coderws.Conn
	startOnce     sync.Once
	closeOnce     sync.Once
	baseCtx       context.Context
	cancel        context.CancelFunc
	readyCh       chan error
	doneCh        chan struct{}
	workerWG      sync.WaitGroup
	runWG         sync.WaitGroup
	readyOnce     sync.Once
	closeNotify   sync.Once
	initErr       error
}

func NewClient(opts ClientOptions) Client {
	settings, err := normalizeClientOptions(opts)
	instance := &client{
		opts:    settings,
		initErr: err,
		readyCh: make(chan error, 1),
		doneCh:  make(chan struct{}),
	}
	if err == nil {
		instance.sendQueue = make(chan outboundMessage, settings.SendQueueCapacity)
		instance.callbackQueue = make(chan func(), settings.CallbackWorkers*16)
	}
	return instance
}

func (c *client) Start(ctx context.Context) error {
	if c.initErr != nil {
		return c.initErr
	}
	var startErr error
	c.startOnce.Do(func() {
		c.baseCtx, c.cancel = context.WithCancel(context.Background())
		c.startCallbackWorkers()
		c.runWG.Add(1)
		go func() {
			defer c.runWG.Done()
			c.run(ctx)
		}()
		startErr = <-c.readyCh
	})
	return startErr
}

func (c *client) Close() error {
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		close(c.doneCh)
		c.connMu.Lock()
		if c.conn != nil {
			_ = c.conn.Close(coderws.StatusNormalClosure, "closed")
			c.conn = nil
		}
		c.connMu.Unlock()
		if c.sendQueue != nil {
			close(c.sendQueue)
		}
		c.runWG.Wait()
		c.dispatchClose(context.Background(), nil)
		if c.callbackQueue != nil {
			close(c.callbackQueue)
		}
		c.workerWG.Wait()
	})
	return nil
}

func (c *client) SendText(ctx context.Context, payload string) error {
	return c.enqueue(ctx, outboundMessage{typ: messageTypeText, payload: []byte(payload)})
}

func (c *client) SendBinary(ctx context.Context, payload []byte) error {
	return c.enqueue(ctx, outboundMessage{typ: messageTypeBinary, payload: append([]byte(nil), payload...)})
}

func (c *client) SendJSON(ctx context.Context, payload any) error {
	encoded, err := marshalJSON(payload)
	if err != nil {
		return wrapOperation("WebSocket JSON 编码失败", err)
	}
	return c.enqueue(ctx, outboundMessage{typ: messageTypeText, payload: encoded})
}

func (c *client) OnText(fn func(context.Context, string)) {
	c.hooks.setText(fn)
}

func (c *client) OnBinary(fn func(context.Context, []byte)) {
	c.hooks.setBinary(fn)
}

func (c *client) OnError(fn func(context.Context, error)) {
	c.hooks.setError(fn)
}

func (c *client) OnClose(fn func(context.Context, error)) {
	c.hooks.setClose(fn)
}

func (c *client) run(ctx context.Context) {
	bo := newBackoff(c.opts.ReconnectInitialDelay, c.opts.ReconnectMaxDelay)
	first := true

	for {
		select {
		case <-c.baseCtx.Done():
			return
		default:
		}

		conn, err := c.dial(ctx)
		if err != nil {
			if first && !c.opts.ReconnectEnabled {
				c.readyOnce.Do(func() { c.readyCh <- err })
				return
			}
			c.opts.Logger.Warn("websocket_client_reconnecting", zap.String("url", c.opts.URL), zap.Error(err))
			c.dispatchError(ctx, err)
			if !c.waitReconnect(bo.Next()) {
				c.readyOnce.Do(func() { c.readyCh <- wrapReconnectExhausted("WebSocket 初次连接失败", err) })
				return
			}
			continue
		}

		c.setConn(conn)
		bo.Reset()
		c.opts.Logger.Info("websocket_client_connected", zap.String("url", c.opts.URL))
		if first {
			first = false
			c.readyOnce.Do(func() { c.readyCh <- nil })
		}

		runErr := c.runConnection(conn)
		c.clearConn(conn)
		if c.isClosing() {
			return
		}
		if runErr != nil {
			c.dispatchError(ctx, runErr)
			c.opts.Logger.Warn("websocket_client_reconnecting", zap.String("url", c.opts.URL), zap.Error(runErr))
		}
		if !c.opts.ReconnectEnabled {
			c.dispatchClose(ctx, runErr)
			return
		}
		if !c.waitReconnect(bo.Next()) {
			c.dispatchClose(ctx, wrapReconnectExhausted("WebSocket 重连耗尽", runErr))
			return
		}
	}
}

func (c *client) runConnection(conn *coderws.Conn) error {
	errCh := make(chan error, 3)
	ctx, cancel := context.WithCancel(c.baseCtx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		c.readLoop(ctx, conn, errCh)
	}()
	go func() {
		defer wg.Done()
		c.writeLoop(ctx, conn, errCh)
	}()
	go func() {
		defer wg.Done()
		c.heartbeatLoop(ctx, conn, errCh)
	}()

	var result error
	select {
	case <-c.baseCtx.Done():
		result = nil
	case result = <-errCh:
	}
	cancel()
	_ = conn.Close(coderws.StatusNormalClosure, "reconnect")
	wg.Wait()
	return result
}

func (c *client) readLoop(ctx context.Context, conn *coderws.Conn, errCh chan<- error) {
	for {
		readCtx, cancel := context.WithCancel(ctx)
		if c.opts.ReadTimeout > 0 {
			readCtx, cancel = context.WithTimeout(ctx, c.opts.ReadTimeout)
		}
		typ, payload, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if stderrors.Is(err, context.Canceled) || ctx.Err() != nil || c.isClosing() {
				return
			}
			c.opts.Logger.Error("websocket_client_read_error", zap.String("url", c.opts.URL), zap.Error(err))
			select {
			case errCh <- classifyReadErr(err):
			default:
			}
			return
		}
		switch typ {
		case coderws.MessageText:
			if hook := c.hooks.text(); hook != nil {
				message := string(payload)
				c.dispatchCallback(func() { hook(context.Background(), message) })
			}
		case coderws.MessageBinary:
			if hook := c.hooks.binary(); hook != nil {
				message := append([]byte(nil), payload...)
				c.dispatchCallback(func() { hook(context.Background(), message) })
			}
		}
	}
}

func (c *client) writeLoop(ctx context.Context, conn *coderws.Conn, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-c.sendQueue:
			if !ok {
				return
			}
			writeCtx, cancel := context.WithCancel(ctx)
			if c.opts.WriteTimeout > 0 {
				writeCtx, cancel = context.WithTimeout(ctx, c.opts.WriteTimeout)
			}
			typ := coderws.MessageText
			if msg.typ == messageTypeBinary {
				typ = coderws.MessageBinary
			}
			err := conn.Write(writeCtx, typ, msg.payload)
			cancel()
			if err != nil {
				if ctx.Err() != nil || c.isClosing() {
					return
				}
				c.opts.Logger.Error("websocket_client_write_error", zap.String("url", c.opts.URL), zap.Error(err))
				select {
				case errCh <- wrapOperation("WebSocket 写入失败", err):
				default:
				}
				return
			}
		}
	}
}

func (c *client) heartbeatLoop(ctx context.Context, conn *coderws.Conn, errCh chan<- error) {
	if c.opts.HeartbeatInterval <= 0 {
		return
	}
	ticker := time.NewTicker(c.opts.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, c.opts.WriteTimeout)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				if ctx.Err() != nil || c.isClosing() {
					return
				}
				select {
				case errCh <- wrapOperation("WebSocket 心跳失败", err):
				default:
				}
				return
			}
		}
	}
}

func (c *client) enqueue(ctx context.Context, msg outboundMessage) error {
	if c.isClosing() {
		return wrapClosed("WebSocket 客户端已关闭", ErrWebSocketClosed)
	}
	select {
	case c.sendQueue <- msg:
		return nil
	case <-ctx.Done():
		return wrapTimeout("WebSocket 发送入队超时", ctx.Err())
	default:
		return wrapQueueFull("WebSocket 发送队列已满")
	}
}

func (c *client) dial(ctx context.Context) (*coderws.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.opts.ConnectTimeout)
	defer cancel()
	c.opts.Logger.Info("websocket_client_connecting", zap.String("url", c.opts.URL))
	conn, _, err := coderws.Dial(dialCtx, c.opts.URL, &coderws.DialOptions{
		HTTPHeader: toHTTPHeader(c.opts.Headers),
	})
	if err != nil {
		return nil, wrapOperation("WebSocket 连接失败", err)
	}
	conn.SetReadLimit(c.opts.MaxMessageBytes)
	return conn, nil
}

func (c *client) waitReconnect(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-c.baseCtx.Done():
		return false
	}
}

func (c *client) dispatchCallback(fn func()) {
	if fn == nil {
		return
	}
	select {
	case c.callbackQueue <- fn:
	default:
		go fn()
	}
}

func (c *client) dispatchError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	if hook := c.hooks.error(); hook != nil {
		c.dispatchCallback(func() { hook(ctx, err) })
	}
}

func (c *client) dispatchClose(ctx context.Context, err error) {
	c.closeNotify.Do(func() {
		if hook := c.hooks.close(); hook != nil {
			c.dispatchCallback(func() { hook(ctx, err) })
		}
		c.opts.Logger.Info("websocket_client_closed", zap.String("url", c.opts.URL), zap.Error(err))
	})
}

func (c *client) startCallbackWorkers() {
	for i := 0; i < c.opts.CallbackWorkers; i++ {
		c.workerWG.Add(1)
		go func() {
			defer c.workerWG.Done()
			for callback := range c.callbackQueue {
				if callback != nil {
					callback()
				}
			}
		}()
	}
}

func (c *client) setConn(conn *coderws.Conn) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == nil && conn != nil {
		activeConnections.Add(1)
	}
	c.conn = conn
}

func (c *client) clearConn(conn *coderws.Conn) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn == conn {
		activeConnections.Add(-1)
		c.conn = nil
	}
}

func (c *client) isClosing() bool {
	select {
	case <-c.doneCh:
		return true
	default:
		return false
	}
}
