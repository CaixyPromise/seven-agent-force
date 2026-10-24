package main

// The delivery egress gateway is deliberately a tiny TCP relay.  It is not a
// general proxy: one short-lived container gets exactly one frozen IP:port and
// listens on exactly the original target port on its private network.  The
// custom Delivery runner has no default route, so its only useful outbound
// connection is this gateway alias.  TLS and SSH both retain their native
// end-to-end host identity checks because the relay never terminates traffic.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

func deliveryEgressGateway(args []string) error {
	flags := flag.NewFlagSet("delivery-egress-gateway", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listenPort := flags.Int("listen-port", 0, "original target port exposed to the runner")
	targetIP := flags.String("target-ip", "", "frozen authorized target IP")
	targetPort := flags.Int("target-port", 0, "frozen authorized target port")
	deadline := flags.String("deadline", "", "RFC3339 authority deadline")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("usage: seven-worker-cli delivery-egress-gateway --listen-port <port> --target-ip <ip> --target-port <port> --deadline <rfc3339>")
	}
	ip := net.ParseIP(strings.TrimSpace(*targetIP))
	if ip == nil || *listenPort < 1 || *listenPort > 65535 || *targetPort < 1 || *targetPort > 65535 {
		return errors.New("DELIVERY_EGRESS_GATEWAY_ARGUMENT_INVALID")
	}
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*deadline))
	if err != nil || !expiresAt.After(time.Now().UTC()) || expiresAt.Sub(time.Now().UTC()) > 15*time.Minute {
		return errors.New("DELIVERY_EGRESS_GATEWAY_AUTHORITY_INVALID")
	}
	return runTargetEgressGateway(context.Background(), targetEgressGatewayConfig{ListenPort: *listenPort, TargetIP: ip, TargetPort: *targetPort, ExpiresAt: expiresAt})
}

func closeWrite(connection net.Conn) {
	if tcp, ok := connection.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
}

// deliveryEgressProbeTarget exists solely for the Worker admission self-test.
// It is never part of a Task or Delivery.  Keeping it in the same immutable
// CLI image proves that the actual gateway binary and Docker networking path
// work together before the Worker advertises egress enforcement.
func deliveryEgressProbeTarget(args []string) error {
	flags := flag.NewFlagSet("delivery-egress-probe-target", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	port := flags.Int("port", 0, "probe port")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *port < 1024 || *port > 65535 {
		return errors.New("DELIVERY_EGRESS_PROBE_TARGET_ARGUMENT_INVALID")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", fmt.Sprint(*port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return acceptErr
		}
		go func(c net.Conn) {
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			buffer := make([]byte, 1024)
			_, _ = c.Read(buffer)
			_, _ = io.WriteString(c, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		}(connection)
	}
}
