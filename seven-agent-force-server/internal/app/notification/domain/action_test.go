package domain

import "testing"

func TestNotificationActionAcceptsOnlySafeInternalPrimaryAction(t *testing.T) {
	link, err := ParseInternalDeepLink("/workforce/tasks/2065424359060983808?turn=2065424359060983812&focus=evaluation")
	if err != nil {
		t.Fatal(err)
	}
	action, err := NewPrimaryInternalAction("查看任务", link)
	if err != nil {
		t.Fatal(err)
	}
	if action.Type != NotificationActionTypeOpenInternal || action.Style != NotificationActionStylePrimary || action.Link != link.String() {
		t.Fatalf("unexpected action: %#v", action)
	}
	for _, value := range []string{"https://evil.example", "//evil.example", "javascript:alert(1)", "/workforce/tasks/0"} {
		if _, err := ParseInternalDeepLink(value); err == nil {
			t.Fatalf("expected unsafe link %q to fail", value)
		}
	}
	if _, err := NewPrimaryInternalAction("bad\nlabel", link); err == nil {
		t.Fatal("expected control character in action label to fail")
	}
}

func TestPublicWebBaseURLPreservesPathPrefixAndNeverInfersMissingOrigin(t *testing.T) {
	base, err := ParsePublicWebBaseURL("https://workforce.example.com/console/", true)
	if err != nil {
		t.Fatal(err)
	}
	link, _ := ParseInternalDeepLink("/workforce/tasks/123?turn=456&focus=result")
	resolved, err := base.Resolve(link)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != "https://workforce.example.com/console/workforce/tasks/123?turn=456&focus=result" {
		t.Fatalf("resolved URL=%q", resolved)
	}
	empty, err := ParsePublicWebBaseURL("", true)
	if err != nil || empty.Configured() {
		t.Fatalf("empty URL should be valid and unconfigured: %#v %v", empty, err)
	}
	if _, err := empty.Resolve(link); err == nil || err.Error() != ReasonPublicWebBaseURLNotConfigured {
		t.Fatalf("missing URL error=%v", err)
	}
	for _, value := range []string{
		"http://workforce.example.com",
		"https://user:pass@workforce.example.com",
		"https://workforce.example.com?origin=evil",
		"https://workforce.example.com/#fragment",
		"https://workforce.example.com/a/../b",
	} {
		if _, err := ParsePublicWebBaseURL(value, true); err == nil {
			t.Fatalf("expected invalid production base URL %q", value)
		}
	}
	if _, err := ParsePublicWebBaseURL("http://127.0.0.1:5177", false); err != nil {
		t.Fatalf("loopback HTTP should be accepted in local mode: %v", err)
	}
}

func TestSelectDeliveryPresentationIsDeterministic(t *testing.T) {
	capabilities := DriverCapabilities{SupportedPresentations: []string{DeliveryPresentationText, DeliveryPresentationCard}}
	selected, err := SelectDeliveryPresentation(DeliveryPresentationAuto, capabilities)
	if err != nil || selected != DeliveryPresentationCard {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
	if _, err := SelectDeliveryPresentation(DeliveryPresentationHTML, capabilities); err == nil {
		t.Fatal("unsupported explicit presentation must fail before send")
	}
}
