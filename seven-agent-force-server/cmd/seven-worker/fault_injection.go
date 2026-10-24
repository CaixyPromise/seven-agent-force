package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"sync"
)

// acceptanceFaultInjector is deliberately inert unless both environment
// switches are present. It exists only for the local FakeHarness fault matrix;
// business requests, queue messages, and Attempt containers cannot enable it.
type acceptanceFaultInjector struct {
	mu        sync.Mutex
	remaining map[string]int
}

func newAcceptanceFaultInjector() *acceptanceFaultInjector {
	injector := &acceptanceFaultInjector{remaining: map[string]int{}}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS")), "true") {
		return injector
	}
	known := map[string]bool{
		"fail-finalization-once":        true,
		"fail-upload-before-write-once": true,
		"fail-readback-once":            true,
		"simulate-host-boot-change":     true,
	}
	for _, value := range strings.Split(os.Getenv("SEVEN_WORKFORCE_ACCEPTANCE_FAULTS"), ",") {
		name := strings.TrimSpace(value)
		if known[name] {
			injector.remaining[name] = 1
		}
	}
	return injector
}

func acceptanceHostBootID(actual string) string {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS")), "true") {
		return actual
	}
	for _, value := range strings.Split(os.Getenv("SEVEN_WORKFORCE_ACCEPTANCE_FAULTS"), ",") {
		if strings.TrimSpace(value) == "simulate-host-boot-change" {
			sum := sha256.Sum256([]byte("seven-fault-host-boot\x00" + actual))
			return fmt.Sprintf("boot_fault_%x", sum[:12])
		}
	}
	return actual
}

func (f *acceptanceFaultInjector) consume(name string) bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.remaining[name] < 1 {
		return false
	}
	f.remaining[name]--
	return true
}
