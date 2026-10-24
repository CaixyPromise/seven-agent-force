package main

import "testing"

func TestAcceptanceFaultInjectorIsSingleUse(t *testing.T) {
	t.Setenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS", "true")
	t.Setenv("SEVEN_WORKFORCE_ACCEPTANCE_FAULTS", "fail-finalization-once,fail-upload-before-write-once,unknown")
	injector := newAcceptanceFaultInjector()
	if !injector.consume("fail-finalization-once") || injector.consume("fail-finalization-once") {
		t.Fatal("finalization fault must fire exactly once")
	}
	if !injector.consume("fail-upload-before-write-once") || injector.consume("unknown") {
		t.Fatal("only allowlisted upload fault must fire")
	}
}

func TestAcceptanceFaultInjectorRequiresExplicitGate(t *testing.T) {
	t.Setenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS", "false")
	t.Setenv("SEVEN_WORKFORCE_ACCEPTANCE_FAULTS", "fail-finalization-once")
	if newAcceptanceFaultInjector().consume("fail-finalization-once") {
		t.Fatal("fault injection must be disabled without the acceptance gate")
	}
}

func TestAcceptanceHostBootChangeIsExplicitlyGated(t *testing.T) {
	t.Setenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS", "")
	t.Setenv("SEVEN_WORKFORCE_ACCEPTANCE_FAULTS", "simulate-host-boot-change")
	if got := acceptanceHostBootID("boot_real"); got != "boot_real" {
		t.Fatalf("disabled acceptance fault changed host boot: %q", got)
	}
	t.Setenv("SEVEN_WORKFORCE_ENABLE_ACCEPTANCE_FAULTS", "true")
	changed := acceptanceHostBootID("boot_real")
	if changed == "boot_real" || changed == "" {
		t.Fatalf("enabled acceptance fault did not change host boot: %q", changed)
	}
	if changed != acceptanceHostBootID("boot_real") {
		t.Fatalf("host boot fault must be deterministic: %q", changed)
	}
}
