package main

import (
	"testing"
	"time"
)

func TestParseTargetEgressGatewayArgs(t *testing.T) {
	now := time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)
	config, err := parseTargetEgressGatewayArgs([]string{
		"--listen-port", "443", "--target-ip", "203.0.113.8", "--target-port", "443",
		"--deadline", now.Add(time.Minute).Format(time.RFC3339),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if config.ListenPort != 443 || config.TargetPort != 443 || config.TargetIP.String() != "203.0.113.8" {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestParseTargetEgressGatewayArgsRejectsInvalidAuthority(t *testing.T) {
	now := time.Date(2026, 9, 11, 1, 0, 0, 0, time.UTC)
	for name, args := range map[string][]string{
		"hostname": {"--listen-port", "443", "--target-ip", "example.com", "--target-port", "443", "--deadline", now.Add(time.Minute).Format(time.RFC3339)},
		"expired":  {"--listen-port", "443", "--target-ip", "203.0.113.8", "--target-port", "443", "--deadline", now.Format(time.RFC3339)},
		"too-long": {"--listen-port", "443", "--target-ip", "203.0.113.8", "--target-port", "443", "--deadline", now.Add(targetEgressMaxAuthority + time.Second).Format(time.RFC3339)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseTargetEgressGatewayArgs(args, now); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}
