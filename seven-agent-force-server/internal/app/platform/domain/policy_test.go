package domain

import "testing"

func TestVisibleLoginMethodsFiltersDisabledAndSorts(t *testing.T) {
	methods := []LoginMethod{
		{MethodType: MethodExternalOAuth, ProviderCode: "google", DisplayName: "Google", SortOrder: 30, DisplayEnabled: true, LoginEnabled: true},
		{MethodType: MethodPassword, ProviderCode: "", DisplayName: "Password", SortOrder: 10, DisplayEnabled: true, LoginEnabled: true},
		{MethodType: MethodExternalOAuth, ProviderCode: "github", DisplayName: "GitHub", SortOrder: 20, DisplayEnabled: false, LoginEnabled: true},
		{MethodType: MethodPasskey, ProviderCode: "", DisplayName: "Passkey", SortOrder: 40, DisplayEnabled: true, LoginEnabled: false},
	}

	got := VisibleLoginMethods(methods)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].MethodType != MethodPassword || got[1].ProviderCode != "google" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

func TestSourceRulePriority(t *testing.T) {
	rules := []SourceRule{
		{PlatformCode: "low", MatchType: MatchHost, MatchValue: "127.0.0.1:5291", Priority: 1000, Status: StatusActive},
		{PlatformCode: "high", MatchType: MatchClientID, MatchValue: "authorization-console", Priority: 1, Status: StatusActive},
	}
	source := RequestSource{ClientID: "authorization-console", Host: "127.0.0.1:5291"}

	got, ok := MatchPlatformCode(rules, source)
	if !ok {
		t.Fatal("expected match")
	}
	if got != "high" {
		t.Fatalf("platform=%s want high", got)
	}
}

func TestExplicitPlatformCodeDoesNotMatchSourceRules(t *testing.T) {
	rules := []SourceRule{
		{PlatformCode: "privileged", MatchType: MatchExplicitCode, MatchValue: "privileged", Priority: 1000, Status: StatusActive},
		{PlatformCode: "trusted", MatchType: MatchHost, MatchValue: "127.0.0.1:5291", Priority: 10, Status: StatusActive},
	}
	source := RequestSource{Host: "127.0.0.1:5291", ExplicitCodeHint: "privileged"}

	got, ok := MatchPlatformCode(rules, source)
	if !ok {
		t.Fatal("expected trusted match")
	}
	if got != "trusted" {
		t.Fatalf("platform=%s want trusted", got)
	}
}

func TestSourceRuleMatchesRedirectHostAndPrefix(t *testing.T) {
	rules := []SourceRule{
		{PlatformCode: "host", MatchType: MatchRedirectHost, MatchValue: "console.example.com", Priority: 20, Status: StatusActive},
		{PlatformCode: "prefix", MatchType: MatchRedirectPrefix, MatchValue: "https://console.example.com/oauth/", Priority: 10, Status: StatusActive},
	}
	source := RequestSource{RedirectURL: "https://console.example.com/oauth/callback?client_id=abc"}

	got, ok := MatchPlatformCode(rules, source)
	if !ok {
		t.Fatal("expected redirect match")
	}
	if got != "host" {
		t.Fatalf("platform=%s want host", got)
	}
}

func TestRedirectPrefixRequiresPathBoundary(t *testing.T) {
	rules := []SourceRule{
		{PlatformCode: "prefix", MatchType: MatchRedirectPrefix, MatchValue: "https://console.example.com/oauth", Priority: 10, Status: StatusActive},
	}
	rejected := []string{
		"https://console.example.com/oauth.evil/callback",
		"https://console.example.com/oauth/../admin",
		"https://console.example.com/oauth/%2e%2e/admin",
		"https://console.example.com/oauth/%252e%252e/admin",
		"https://console.example.com/oauth/safe/../callback",
		"https://console.example.com.evil/oauth/callback",
		"https://console.example.com.:443/oauth/callback",
		"https://user@console.example.com.evil/oauth/callback",
		"https://console.example.com:444/oauth/callback",
	}
	for _, redirectURL := range rejected {
		if got, ok := MatchPlatformCode(rules, RequestSource{RedirectURL: redirectURL}); ok {
			t.Fatalf("redirect %q matched platform=%s, want no match", redirectURL, got)
		}
	}

	got, ok := MatchPlatformCode(rules, RequestSource{RedirectURL: "https://console.example.com/oauth//callback"})
	if !ok {
		t.Fatal("expected path-boundary prefix match")
	}
	if got != "prefix" {
		t.Fatalf("platform=%s want prefix", got)
	}
}

func TestRedirectPrefixNormalizesDefaultPort(t *testing.T) {
	rules := []SourceRule{
		{PlatformCode: "prefix", MatchType: MatchRedirectPrefix, MatchValue: "https://console.example.com/oauth", Priority: 10, Status: StatusActive},
	}

	got, ok := MatchPlatformCode(rules, RequestSource{RedirectURL: "https://console.example.com:443/oauth/callback"})
	if !ok {
		t.Fatal("expected default-port redirect match")
	}
	if got != "prefix" {
		t.Fatalf("platform=%s want prefix", got)
	}
}
