package main

// targetEgressGateway is the shared, target-pinned TCP relay core used by
// trusted Worker runtimes. It is deliberately not an HTTP or CONNECT proxy:
// one process listens on one port and can dial only one pre-resolved IP:port.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const targetEgressMaxAuthority = 30 * time.Minute

type targetEgressGatewayConfig struct {
	ListenPort int
	TargetIP   net.IP
	TargetPort int
	ExpiresAt  time.Time
}

func parseTargetEgressGatewayArgs(args []string, now time.Time) (targetEgressGatewayConfig, error) {
	flags := flag.NewFlagSet("target-egress-gateway", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	listenPort := flags.Int("listen-port", 0, "original target port exposed to the runner")
	targetIP := flags.String("target-ip", "", "frozen authorized target IP")
	targetPort := flags.Int("target-port", 0, "frozen authorized target port")
	deadline := flags.String("deadline", "", "RFC3339 authority deadline")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return targetEgressGatewayConfig{}, errors.New("TARGET_EGRESS_GATEWAY_ARGUMENT_INVALID")
	}
	ip := net.ParseIP(strings.TrimSpace(*targetIP))
	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*deadline))
	if ip == nil || *listenPort < 1 || *listenPort > 65535 || *targetPort < 1 || *targetPort > 65535 {
		return targetEgressGatewayConfig{}, errors.New("TARGET_EGRESS_GATEWAY_ARGUMENT_INVALID")
	}
	now = now.UTC()
	if err != nil || !expiresAt.After(now) || expiresAt.Sub(now) > targetEgressMaxAuthority {
		return targetEgressGatewayConfig{}, errors.New("TARGET_EGRESS_GATEWAY_AUTHORITY_INVALID")
	}
	return targetEgressGatewayConfig{ListenPort: *listenPort, TargetIP: ip, TargetPort: *targetPort, ExpiresAt: expiresAt}, nil
}

func targetEgressGateway(args []string) error {
	config, err := parseTargetEgressGatewayArgs(args, time.Now())
	if err != nil {
		return err
	}
	return runTargetEgressGateway(context.Background(), config)
}

func runTargetEgressGateway(parent context.Context, config targetEgressGatewayConfig) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("0.0.0.0", strconv.Itoa(config.ListenPort)))
	if err != nil {
		return fmt.Errorf("TARGET_EGRESS_GATEWAY_LISTEN_FAILED: %w", err)
	}
	defer listener.Close()
	ctx, cancel := context.WithDeadline(parent, config.ExpiresAt)
	defer cancel()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	var active sync.WaitGroup
	defer active.Wait()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil || errors.Is(acceptErr, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("TARGET_EGRESS_GATEWAY_ACCEPT_FAILED: %w", acceptErr)
		}
		active.Add(1)
		go relayTargetConnection(ctx, &active, connection, config.TargetIP, config.TargetPort)
	}
}

func relayTargetConnection(ctx context.Context, active *sync.WaitGroup, inbound net.Conn, ip net.IP, port int) {
	defer active.Done()
	defer inbound.Close()
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	outbound, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err != nil {
		return
	}
	defer outbound.Close()
	var copies sync.WaitGroup
	copies.Add(2)
	go func() { defer copies.Done(); _, _ = io.Copy(outbound, inbound); closeWrite(outbound) }()
	go func() { defer copies.Done(); _, _ = io.Copy(inbound, outbound); closeWrite(inbound) }()
	copies.Wait()
}
