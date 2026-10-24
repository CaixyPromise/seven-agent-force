package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestEnterpriseGrantsAreExplicitAndDoNotImplyLogin(t *testing.T) {
	grants, err := NormalizeEnterpriseGrants([]EnterpriseIdentityGrant{GrantNotificationDirect, GrantIdentityLink, GrantNotificationDirect})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(grants, []EnterpriseIdentityGrant{GrantIdentityLink, GrantNotificationDirect}) {
		t.Fatalf("unexpected grants: %v", grants)
	}
	for _, grant := range grants {
		if grant == GrantLogin {
			t.Fatal("notification grants silently implied LOGIN")
		}
	}
}

func TestPublicWebBaseURLContract(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		production bool
		want       string
		wantErr    error
	}{
		{name: "production https", value: "https://workforce.example.com/console/", production: true, want: "https://workforce.example.com/console"},
		{name: "local loopback", value: "http://127.0.0.1:5177", want: "http://127.0.0.1:5177"},
		{name: "missing", wantErr: ErrPublicWebURLNotConfigured},
		{name: "production http", value: "http://workforce.example.com", production: true, wantErr: ErrDeepLinkInvalid},
		{name: "non loopback http", value: "http://workforce.example.com", wantErr: ErrDeepLinkInvalid},
		{name: "userinfo", value: "https://user:pass@workforce.example.com", wantErr: ErrDeepLinkInvalid},
		{name: "query", value: "https://workforce.example.com?next=evil", wantErr: ErrDeepLinkInvalid},
		{name: "fragment", value: "https://workforce.example.com/#x", wantErr: ErrDeepLinkInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParsePublicWebBaseURL(test.value, test.production)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error=%v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil || result.String() != test.want {
				t.Fatalf("result=%q err=%v, want %q", result.String(), err, test.want)
			}
		})
	}
}

func TestPublicWebBaseURLResolvesOnlyPlatformTaskLink(t *testing.T) {
	base, err := ParsePublicWebBaseURL("https://workforce.example.com/console/", true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := base.ResolveTaskLink(9007199254740993, 9007199254740995, FocusEvaluation)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://workforce.example.com/console/workforce/tasks/9007199254740993?turn=9007199254740995&focus=evaluation"
	if got != want {
		t.Fatalf("task URL=%q want=%q", got, want)
	}
	if _, err = base.ResolveTaskLink(1, 0, FocusEvaluation); !errors.Is(err, ErrDeepLinkInvalid) {
		t.Fatalf("invalid relation identity error=%v", err)
	}
}
