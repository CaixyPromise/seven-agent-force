package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/mcpserver"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("seven-agent-force-mcp-server", flag.ContinueOnError)
	configPath := flags.String("config", "configs/mcp.yaml", "path to MCP server configuration")
	showVersion := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Println(buildinfo.String())
		return 0
	}
	config, err := mcpserver.LoadRuntimeConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load configuration: %v\n", err)
		return 1
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	gateway, err := mcpserver.NewHTTPGateway(config.GatewayConfig())
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure Open API gateway: %v\n", err)
		return 1
	}
	handler, err := mcpserver.NewHandler(config.HandlerConfig(), gateway, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configure MCP handler: %v\n", err)
		return 1
	}
	server := newHTTPServer(config, handler)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	stopContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "run MCP server: %v\n", err)
			return 1
		}
		return 0
	case <-stopContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown MCP server: %v\n", err)
			return 1
		}
		return 0
	}
}

func newHTTPServer(config mcpserver.RuntimeConfig, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              config.Server.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(config.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(config.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
