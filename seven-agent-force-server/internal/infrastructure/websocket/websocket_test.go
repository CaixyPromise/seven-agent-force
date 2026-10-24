package websocket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
)

type testServer struct {
	http  *httptest.Server
	wsURL string
}

func newTestServer(t *testing.T, handler func(*coderws.Conn, *http.Request)) *testServer {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		handler(conn, r)
	}))
	t.Cleanup(server.Close)
	return &testServer{
		http:  server,
		wsURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}
}

func wsCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func TestTempClientDialAndEcho(t *testing.T) {
	headerSeen := make(chan string, 1)
	server := newTestServer(t, func(conn *coderws.Conn, r *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		headerSeen <- r.Header.Get("X-Test-Header")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		typ, payload, err := conn.Read(ctx)
		if err != nil {
			return
		}
		_ = conn.Write(ctx, typ, payload)
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	client, err := DialTemp(ctx, TempOptions{
		URL:     server.wsURL,
		Headers: map[string]string{"X-Test-Header": "header-value"},
	})
	if err != nil {
		t.Fatalf("DialTemp() error = %v", err)
	}
	defer client.Close()

	if err := client.SendText(ctx, "hello"); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	got, err := client.ReadText(ctx)
	if err != nil {
		t.Fatalf("ReadText() error = %v", err)
	}
	if got != "hello" {
		t.Fatalf("ReadText() = %q, want %q", got, "hello")
	}

	select {
	case value := <-headerSeen:
		if value != "header-value" {
			t.Fatalf("header = %q, want %q", value, "header-value")
		}
	case <-ctx.Done():
		t.Fatal("did not observe handshake header")
	}
}

func TestDialTempAsyncAndJSON(t *testing.T) {
	type payload struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	server := newTestServer(t, func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		typ, message, err := conn.Read(ctx)
		if err != nil {
			return
		}
		_ = conn.Write(ctx, typ, message)
	})

	ctx, cancel := wsCtx(t)
	defer cancel()

	resultCh := DialTempAsync(ctx, TempOptions{URL: server.wsURL})
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("DialTempAsync() error = %v", result.Err)
	}
	client := result.Value
	defer client.Close()

	if err := client.SendJSON(ctx, payload{ID: 42, Name: "codex"}); err != nil {
		t.Fatalf("SendJSON() error = %v", err)
	}

	var got payload
	if err := client.ReadJSON(ctx, &got); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}
	if got.ID != 42 || got.Name != "codex" {
		t.Fatalf("ReadJSON() = %+v, want {ID:42 Name:codex}", got)
	}
}

func TestTempClientListenAndCloseIdempotent(t *testing.T) {
	server := newTestServer(t, func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Write(ctx, coderws.MessageText, []byte("one"))
		_ = conn.Write(ctx, coderws.MessageText, []byte("stop"))
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	client, err := DialTemp(ctx, TempOptions{URL: server.wsURL})
	if err != nil {
		t.Fatalf("DialTemp() error = %v", err)
	}

	values, err := client.ListenText(ctx, func(s string) bool { return s == "stop" })
	if err != nil {
		t.Fatalf("ListenText() error = %v", err)
	}
	if len(values) != 2 || values[0] != "one" || values[1] != "stop" {
		t.Fatalf("ListenText() = %#v, want [one stop]", values)
	}

	if err := client.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestTempClientReadTextDrainsBufferedMessageBeforeError(t *testing.T) {
	for i := 0; i < 100; i++ {
		client := &tempClient{
			textQueue: make(chan string, 1),
			errorCh:   make(chan error, 1),
			done:      make(chan struct{}),
		}
		client.textQueue <- "stop"
		client.errorCh <- wrapOperation("WebSocket 读取失败", context.Canceled)

		got, err := client.ReadText(context.Background())
		if err != nil {
			t.Fatalf("ReadText() error = %v", err)
		}
		if got != "stop" {
			t.Fatalf("ReadText() = %q, want stop", got)
		}
	}
}

func TestTempClientReadBinaryDrainsBufferedMessageBeforeError(t *testing.T) {
	for i := 0; i < 100; i++ {
		client := &tempClient{
			binaryQueue: make(chan []byte, 1),
			errorCh:     make(chan error, 1),
			done:        make(chan struct{}),
		}
		client.binaryQueue <- []byte("stop")
		client.errorCh <- wrapOperation("WebSocket 读取失败", context.Canceled)

		got, err := client.ReadBinary(context.Background())
		if err != nil {
			t.Fatalf("ReadBinary() error = %v", err)
		}
		if string(got) != "stop" {
			t.Fatalf("ReadBinary() = %q, want stop", string(got))
		}
	}
}

func TestTempClientBinaryAndMessageSizeLimit(t *testing.T) {
	server := newTestServer(t, func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := conn.Write(ctx, coderws.MessageBinary, []byte("bin")); err != nil {
			return
		}
		_ = conn.Write(ctx, coderws.MessageText, []byte("toolarge"))
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	client, err := DialTemp(ctx, TempOptions{
		URL:             server.wsURL,
		MaxMessageBytes: 4,
	})
	if err != nil {
		t.Fatalf("DialTemp() error = %v", err)
	}
	defer client.Close()

	data, err := client.ReadBinary(ctx)
	if err != nil {
		t.Fatalf("ReadBinary() error = %v", err)
	}
	if string(data) != "bin" {
		t.Fatalf("ReadBinary() = %q, want %q", string(data), "bin")
	}

	if _, err := client.ReadText(ctx); err == nil {
		t.Fatal("ReadText() expected message size limit error")
	}
}

func TestClientStartReceiveReconnectAndClose(t *testing.T) {
	var connectCount atomic.Int32
	server := newTestServer(t, func(conn *coderws.Conn, _ *http.Request) {
		count := connectCount.Add(1)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if count == 1 {
			_ = conn.Write(ctx, coderws.MessageText, []byte("first"))
			_ = conn.Close(coderws.StatusNormalClosure, "first closed")
			return
		}
		defer conn.Close(coderws.StatusNormalClosure, "done")
		_ = conn.Write(ctx, coderws.MessageText, []byte("second"))
		<-ctx.Done()
	})

	client := NewClient(ClientOptions{
		URL:                   server.wsURL,
		ReconnectEnabled:      true,
		ReconnectInitialDelay: 20 * time.Millisecond,
		ReconnectMaxDelay:     50 * time.Millisecond,
		HeartbeatInterval:     0,
		CallbackWorkers:       2,
	})

	received := make(chan string, 4)
	closed := make(chan error, 1)
	client.OnText(func(_ context.Context, message string) {
		received <- message
	})
	client.OnClose(func(_ context.Context, err error) {
		closed <- err
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	want := []string{"first", "second"}
	for _, expected := range want {
		select {
		case got := <-received:
			if got != expected {
				t.Fatalf("received message = %q, want %q", got, expected)
			}
		case <-ctx.Done():
			t.Fatalf("did not receive %q before timeout", expected)
		}
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("did not observe close callback")
	}
}

func TestClientCallbackDoesNotBlockReadLoop(t *testing.T) {
	server := newTestServer(t, func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = conn.Write(ctx, coderws.MessageText, []byte("one"))
		_ = conn.Write(ctx, coderws.MessageText, []byte("two"))
		<-ctx.Done()
	})

	client := NewClient(ClientOptions{
		URL:               server.wsURL,
		HeartbeatInterval: 0,
		CallbackWorkers:   1,
	})

	block := make(chan struct{})
	received := make(chan string, 2)
	var calls atomic.Int32
	client.OnText(func(_ context.Context, message string) {
		if calls.Add(1) == 1 {
			received <- message
			<-block
			return
		}
		received <- message
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case got := <-received:
		if got != "one" {
			t.Fatalf("first callback = %q, want one", got)
		}
	case <-ctx.Done():
		t.Fatal("did not receive first callback")
	}

	time.Sleep(50 * time.Millisecond)
	close(block)

	select {
	case got := <-received:
		if got != "two" {
			t.Fatalf("second callback = %q, want two", got)
		}
	case <-ctx.Done():
		t.Fatal("did not receive second callback")
	}

	_ = client.Close()
}

func TestClientQueueFull(t *testing.T) {
	client := NewClient(ClientOptions{
		URL:               "ws://example.invalid",
		SendQueueCapacity: 1,
		CallbackWorkers:   1,
		HeartbeatInterval: 0,
	})

	ctx, cancel := wsCtx(t)
	defer cancel()
	if err := client.SendText(ctx, "first"); err != nil {
		t.Fatalf("first SendText() error = %v", err)
	}
	if err := client.SendText(ctx, "second"); err == nil {
		t.Fatal("second SendText() expected queue full error")
	}
	_ = client.Close()
}
