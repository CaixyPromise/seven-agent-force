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

func newBenchmarkServer(handler func(*coderws.Conn, *http.Request)) *testServer {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, &coderws.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		handler(conn, r)
	}))
	return &testServer{
		http:  server,
		wsURL: "ws" + strings.TrimPrefix(server.URL, "http"),
	}
}

func BenchmarkTempClientSendRead(b *testing.B) {
	server := newBenchmarkServer(func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx := context.Background()
		for {
			typ, payload, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if err := conn.Write(ctx, typ, payload); err != nil {
				return
			}
		}
	})
	defer server.http.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := DialTemp(ctx, TempOptions{URL: server.wsURL})
	if err != nil {
		b.Fatalf("DialTemp() error = %v", err)
	}
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.SendText(ctx, "payload"); err != nil {
			b.Fatalf("SendText() error = %v", err)
		}
		if _, err := client.ReadText(ctx); err != nil {
			b.Fatalf("ReadText() error = %v", err)
		}
	}
}

func BenchmarkClientCallbackDispatch(b *testing.B) {
	server := newBenchmarkServer(func(conn *coderws.Conn, _ *http.Request) {
		defer conn.Close(coderws.StatusNormalClosure, "done")
		ctx := context.Background()
		for {
			typ, payload, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if err := conn.Write(ctx, typ, payload); err != nil {
				return
			}
		}
	})
	defer server.http.Close()

	client := NewClient(ClientOptions{
		URL:               server.wsURL,
		HeartbeatInterval: 0,
		CallbackWorkers:   4,
		SendQueueCapacity: benchmarkQueueCapacity(b.N),
	})
	var received atomic.Int64
	done := make(chan struct{}, 1)
	client.OnText(func(_ context.Context, _ string) {
		if received.Add(1) == int64(b.N) {
			done <- struct{}{}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.SendText(ctx, "payload"); err != nil {
			b.Fatalf("SendText() error = %v", err)
		}
	}
	<-done
}

func benchmarkQueueCapacity(n int) int {
	if n < 1024 {
		return 1024
	}
	return n * 2
}

func BenchmarkClientReconnectPath(b *testing.B) {
	var count atomic.Int64
	server := newBenchmarkServer(func(conn *coderws.Conn, _ *http.Request) {
		current := count.Add(1)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Write(ctx, coderws.MessageText, []byte("hello"))
		_ = conn.Close(coderws.StatusNormalClosure, "cycle")
		if current >= int64(b.N+1) {
			return
		}
	})
	defer server.http.Close()

	client := NewClient(ClientOptions{
		URL:                   server.wsURL,
		ReconnectEnabled:      true,
		ReconnectInitialDelay: time.Millisecond,
		ReconnectMaxDelay:     5 * time.Millisecond,
		HeartbeatInterval:     0,
		CallbackWorkers:       2,
	})
	var received atomic.Int64
	done := make(chan struct{}, 1)
	client.OnText(func(_ context.Context, _ string) {
		if received.Add(1) == int64(b.N+1) {
			done <- struct{}{}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	defer client.Close()

	b.ResetTimer()
	<-done
}
