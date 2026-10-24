package websocket

import (
	"context"
	stderrors "errors"
	"net/http"
	"sync"
	"time"

	coderws "github.com/coder/websocket"
	"go.uber.org/zap"
)

type TempClient interface {
	SendText(ctx context.Context, payload string) error
	SendBinary(ctx context.Context, payload []byte) error
	SendJSON(ctx context.Context, payload any) error

	ReadText(ctx context.Context) (string, error)
	ReadBinary(ctx context.Context) ([]byte, error)
	ReadJSON(ctx context.Context, dest any) error

	ListenText(ctx context.Context, stop func(string) bool) ([]string, error)
	ListenBinary(ctx context.Context, stop func([]byte) bool) ([][]byte, error)

	Close() error
}

type tempClient struct {
	conn        *coderws.Conn
	opts        tempSettings
	textQueue   chan string
	binaryQueue chan []byte
	errorCh     chan error
	done        chan struct{}
	readCtx     context.Context
	readCancel  context.CancelFunc
	closeOnce   sync.Once
	errorOnce   sync.Once
	closeNotify sync.Once
	closing     bool
	mu          sync.RWMutex
	lastErr     error
}

func DialTemp(ctx context.Context, opts TempOptions) (TempClient, error) {
	settings, err := normalizeTempOptions(opts)
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := withTimeout(ctx, settings.ConnectTimeout)
	defer cancel()

	settings.Logger.Info("websocket_client_connecting", zap.String("url", settings.URL))
	conn, _, err := coderws.Dial(dialCtx, settings.URL, &coderws.DialOptions{
		HTTPHeader: toHTTPHeader(settings.Headers),
	})
	if err != nil {
		return nil, wrapOperation("WebSocket 连接失败", err)
	}
	conn.SetReadLimit(settings.MaxMessageBytes)

	readCtx, cancel := context.WithCancel(context.Background())
	client := &tempClient{
		conn:        conn,
		opts:        settings,
		textQueue:   make(chan string, settings.QueueCapacity),
		binaryQueue: make(chan []byte, settings.QueueCapacity),
		errorCh:     make(chan error, 1),
		done:        make(chan struct{}),
		readCtx:     readCtx,
		readCancel:  cancel,
	}
	activeConnections.Add(1)
	if settings.SessionTimeout > 0 {
		go client.watchSessionTimeout(settings.SessionTimeout)
	}
	go client.readLoop()
	settings.Logger.Info("websocket_client_connected", zap.String("url", settings.URL))
	return client, nil
}

func DialTempAsync(ctx context.Context, opts TempOptions) <-chan Result[TempClient] {
	ch := make(chan Result[TempClient], 1)
	go func() {
		client, err := DialTemp(ctx, opts)
		ch <- Result[TempClient]{Value: client, Err: err}
		close(ch)
	}()
	return ch
}

func (c *tempClient) SendText(ctx context.Context, payload string) error {
	return c.write(ctx, messageTypeText, []byte(payload))
}

func (c *tempClient) SendBinary(ctx context.Context, payload []byte) error {
	copied := append([]byte(nil), payload...)
	return c.write(ctx, messageTypeBinary, copied)
}

func (c *tempClient) SendJSON(ctx context.Context, payload any) error {
	encoded, err := marshalJSON(payload)
	if err != nil {
		return wrapOperation("WebSocket JSON 编码失败", err)
	}
	return c.write(ctx, messageTypeText, encoded)
}

func (c *tempClient) ReadText(ctx context.Context) (string, error) {
	for {
		select {
		case value := <-c.textQueue:
			return value, nil
		default:
		}

		select {
		case value := <-c.textQueue:
			return value, nil
		case err := <-c.errorCh:
			return "", err
		case <-c.done:
			return "", c.closedErr()
		case <-ctx.Done():
			return "", wrapTimeout("WebSocket 读取文本超时", ctx.Err())
		}
	}
}

func (c *tempClient) ReadBinary(ctx context.Context) ([]byte, error) {
	for {
		select {
		case value := <-c.binaryQueue:
			return append([]byte(nil), value...), nil
		default:
		}

		select {
		case value := <-c.binaryQueue:
			return append([]byte(nil), value...), nil
		case err := <-c.errorCh:
			return nil, err
		case <-c.done:
			return nil, c.closedErr()
		case <-ctx.Done():
			return nil, wrapTimeout("WebSocket 读取二进制超时", ctx.Err())
		}
	}
}

func (c *tempClient) ReadJSON(ctx context.Context, dest any) error {
	payload, err := c.ReadText(ctx)
	if err != nil {
		return err
	}
	if err := unmarshalJSON([]byte(payload), dest); err != nil {
		return wrapOperation("WebSocket JSON 解码失败", err)
	}
	return nil
}

func (c *tempClient) ListenText(ctx context.Context, stop func(string) bool) ([]string, error) {
	collected := make([]string, 0, 4)
	for {
		value, err := c.ReadText(ctx)
		if err != nil {
			return nil, err
		}
		collected = append(collected, value)
		if stop != nil && stop(value) {
			return collected, nil
		}
	}
}

func (c *tempClient) ListenBinary(ctx context.Context, stop func([]byte) bool) ([][]byte, error) {
	collected := make([][]byte, 0, 4)
	for {
		value, err := c.ReadBinary(ctx)
		if err != nil {
			return nil, err
		}
		collected = append(collected, append([]byte(nil), value...))
		if stop != nil && stop(value) {
			return collected, nil
		}
	}
}

func (c *tempClient) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closing = true
		c.mu.Unlock()
		c.readCancel()
		_ = c.conn.Close(coderws.StatusNormalClosure, "closed")
		activeConnections.Add(-1)
		close(c.done)
		c.notifyClosed(nil)
	})
	return nil
}

func (c *tempClient) write(ctx context.Context, kind messageType, payload []byte) error {
	if c.isClosed() {
		return c.closedErr()
	}
	writeCtx, cancel := withTimeout(ctx, c.opts.WriteTimeout)
	defer cancel()

	var typ coderws.MessageType
	if kind == messageTypeBinary {
		typ = coderws.MessageBinary
	} else {
		typ = coderws.MessageText
	}
	if err := c.conn.Write(writeCtx, typ, payload); err != nil {
		c.setError(wrapOperation("WebSocket 写入失败", err))
		return wrapOperation("WebSocket 写入失败", err)
	}
	return nil
}

func (c *tempClient) readLoop() {
	for {
		if c.isClosed() {
			return
		}
		readCtx, cancel := withTimeout(c.readCtx, c.opts.ReadTimeout)
		typ, payload, err := c.conn.Read(readCtx)
		cancel()
		if err != nil {
			if c.isClosed() || stderrors.Is(err, context.Canceled) {
				return
			}
			c.setError(classifyReadErr(err))
			_ = c.Close()
			return
		}
		switch typ {
		case coderws.MessageText:
			if !enqueueString(c.textQueue, string(payload)) {
				c.setError(wrapQueueFull("WebSocket 文本消息队列已满"))
				_ = c.Close()
				return
			}
		case coderws.MessageBinary:
			if !enqueueBytes(c.binaryQueue, payload) {
				c.setError(wrapQueueFull("WebSocket 二进制消息队列已满"))
				_ = c.Close()
				return
			}
		}
	}
}

func (c *tempClient) watchSessionTimeout(timeout time.Duration) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		c.setError(wrapTimeout("WebSocket 会话超时", context.DeadlineExceeded))
		_ = c.Close()
	case <-c.done:
		return
	}
}

func (c *tempClient) setError(err error) {
	if err == nil {
		return
	}
	c.errorOnce.Do(func() {
		c.mu.Lock()
		c.lastErr = err
		c.mu.Unlock()
		select {
		case c.errorCh <- err:
		default:
		}
		c.opts.Logger.Error("websocket_client_read_error", zap.String("url", c.opts.URL), zap.Error(err))
		c.notifyClosed(err)
	})
}

func (c *tempClient) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *tempClient) closedErr() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastErr != nil {
		return c.lastErr
	}
	return wrapClosed("WebSocket 已关闭", ErrWebSocketClosed)
}

func enqueueString(ch chan string, value string) bool {
	select {
	case ch <- value:
		return true
	default:
		return false
	}
}

func enqueueBytes(ch chan []byte, value []byte) bool {
	payload := append([]byte(nil), value...)
	select {
	case ch <- payload:
		return true
	default:
		return false
	}
}

func toHTTPHeader(headers map[string]string) http.Header {
	if len(headers) == 0 {
		return nil
	}
	result := make(http.Header, len(headers))
	for key, value := range headers {
		if key != "" {
			result.Set(key, value)
		}
	}
	return result
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func classifyReadErr(err error) error {
	if err == nil {
		return nil
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return wrapTimeout("WebSocket 读取超时", err)
	}
	return wrapOperation("WebSocket 读取失败", err)
}

func (c *tempClient) notifyClosed(err error) {
	c.closeNotify.Do(func() {
		c.opts.Logger.Info("websocket_client_closed", zap.String("url", c.opts.URL), zap.Error(err))
	})
}
