package domain

import (
	"testing"
	"time"
)

func TestIsSafeInternalDeepLinkRequiresCanonicalUnescapedPath(t *testing.T) {
	allowed := []string{
		"",
		"/",
		"/notifications",
		"/notifications/messages?filter=unread#latest",
		"/account/security",
		"/workforce/tasks/1",
		"/workforce/tasks/2065424359060983808?turn=2065424359060983810&focus=evaluation",
		"/workforce/tasks/9223372036854775807?focus=result",
	}
	for _, value := range allowed {
		if !IsSafeInternalDeepLink(value) {
			t.Fatalf("IsSafeInternalDeepLink(%q)=false, want true", value)
		}
	}

	rejected := []string{
		"/notifications/../admin",
		"/notifications/%2e%2e/admin",
		"/notifications/%2E%2E/admin",
		"/notifications/%252e%252e/admin",
		"/notifications/%2f..%2fadmin",
		"/notifications//admin",
		"https://attacker.example/notifications",
		"//attacker.example/notifications",
		"/workforce",
		"/workforce/",
		"/workforce/tasks",
		"/workforce/tasks/",
		"/workforce/tasks/0",
		"/workforce/tasks/-1",
		"/workforce/tasks/+1",
		"/workforce/tasks/01",
		"/workforce/tasks/9223372036854775808",
		"/workforce/tasks/1/",
		"/workforce/tasks/1/evaluations",
		"/workforce/tasks/1/../2",
		"/workforce/tasks/%31",
		"/workforce/tasks/%2e%2e",
		"/workforce/tasks/1%2f2",
		"https://attacker.example/workforce/tasks/1",
		"//attacker.example/workforce/tasks/1",
	}
	for _, value := range rejected {
		if IsSafeInternalDeepLink(value) {
			t.Fatalf("IsSafeInternalDeepLink(%q)=true, want false", value)
		}
	}
}

func TestRecipientExpiryIsOneWayAndOnlyAppliesWhenDue(t *testing.T) {
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	recipient := &Recipient{ExpiresAt: &future}
	changed, err := recipient.ApplyInboxAction(InboxActionExpire, now)
	if err != nil || changed || recipient.ExpiredAt != nil {
		t.Fatalf("future expiry changed=%t expiredAt=%v err=%v", changed, recipient.ExpiredAt, err)
	}

	past := now.Add(-time.Second)
	recipient.ExpiresAt = &past
	changed, err = recipient.ApplyInboxAction(InboxActionExpire, now)
	if err != nil || !changed || recipient.ExpiredAt == nil || !recipient.ExpiredAt.Equal(now) {
		t.Fatalf("due expiry changed=%t expiredAt=%v err=%v", changed, recipient.ExpiredAt, err)
	}
	changed, err = recipient.ApplyInboxAction(InboxActionExpire, now.Add(time.Second))
	if err != nil || changed {
		t.Fatalf("second expiry changed=%t err=%v, want no-op", changed, err)
	}
}
