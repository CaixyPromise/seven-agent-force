package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/mcpserver"
)

func TestHTTPServerUsesBoundedReadAndWriteTimeouts(t *testing.T) {
	config := mcpserver.RuntimeConfig{}
	config.Server.ListenAddress = "127.0.0.1:18081"
	config.Server.ReadTimeoutSeconds = 7
	config.Server.WriteTimeoutSeconds = 19
	server := newHTTPServer(config, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 7*time.Second || server.WriteTimeout != 19*time.Second || server.IdleTimeout != 60*time.Second {
		t.Fatalf("unexpected timeouts: header=%s read=%s write=%s idle=%s", server.ReadHeaderTimeout, server.ReadTimeout, server.WriteTimeout, server.IdleTimeout)
	}
}
