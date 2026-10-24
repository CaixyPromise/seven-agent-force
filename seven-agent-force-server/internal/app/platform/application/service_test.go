package application

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestPlatformBrandRejectsLegacyLogoURLAndProjectsOnlyFiniteFields(t *testing.T) {
	for _, raw := range []string{
		`{"logoUrl":"https://example.test/logo.png"}`,
		`{"logoUrl":"data:image/png;base64,AAAA"}`,
		`{"logoUrl":"blob:https://console.test/asset"}`,
		`{"logoUrl":"/var/private/logo.png"}`,
		`{"title":"Seven","unknownUrl":"https://example.test"}`,
	} {
		if _, err := platformFromSaveRequest(facade.PlatformSaveRequest{
			PlatformCode: "console", PlatformName: "Console", BrandJSON: raw,
		}); err == nil {
			t.Fatalf("unsafe/unknown brand field unexpectedly accepted: %s", raw)
		}
	}

	stored, err := platformFromSaveRequest(facade.PlatformSaveRequest{
		PlatformCode: "console", PlatformName: "Console",
		BrandJSON: `{"title":" Console ","subtitle":" Ops ","theme":"DARK"}`,
	})
	if err != nil {
		t.Fatalf("finite brand fields rejected: %v", err)
	}
	if stored.BrandJSON != `{"title":"Console","subtitle":"Ops","theme":"dark"}` {
		t.Fatalf("brand JSON did not normalize finite fields: %s", stored.BrandJSON)
	}

	legacy := mapBrand(domain.Platform{
		PlatformName: "Console",
		BrandJSON:    `{"title":"Console","subtitle":"Ops","theme":"dark","logoUrl":"https://attacker.test/logo.png"}`,
	})
	if legacy.Title != "Console" || legacy.Subtitle != "Ops" || legacy.Theme != "dark" {
		t.Fatalf("unexpected safe brand projection: %#v", legacy)
	}
	payload, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal safe brand projection: %v", err)
	}
	if strings.Contains(string(payload), "logoUrl") || strings.Contains(string(payload), "attacker.test") {
		t.Fatalf("legacy raw logo escaped the public login brand contract: %s", payload)
	}
}

func TestResolveLoginOptionsUsesTrustedClientIDAndFiltersMethods(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "console", PlatformName: "Console", BrandJSON: `{"title":"Console","subtitle":"Ops","theme":"cyan"}`, Status: domain.StatusActive},
		},
		ssoBindings: []domain.SSOClientBinding{
			{PlatformCode: "console", ClientID: "authorization-console", Status: domain.StatusActive},
		},
		authzSessions: map[string]*ssofacade.AuthorizationSessionSnapshot{
			"txn_1": {LoginTransactionID: "txn_1", ClientID: "authorization-console", RedirectURI: "http://127.0.0.1:5291/"},
		},
		sourceRules: []domain.SourceRule{
			{PlatformCode: "default", MatchType: domain.MatchHost, MatchValue: "127.0.0.1:5291", Priority: 999, Status: domain.StatusActive},
		},
		methods: map[string][]domain.LoginMethod{
			"console": {
				{PlatformCode: "console", MethodType: domain.MethodPassword, DisplayName: "Password", SortOrder: 20, DisplayEnabled: true, LoginEnabled: true},
				{PlatformCode: "console", MethodType: domain.MethodPasskey, DisplayName: "Passkey", SortOrder: 10, DisplayEnabled: true, LoginEnabled: false},
				{PlatformCode: "console", MethodType: domain.MethodExternalOAuth, ProviderCode: "github", DisplayName: "GitHub", SortOrder: 30, DisplayEnabled: true, LoginEnabled: true},
			},
		},
	})

	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ClientID:           "authorization-console",
		LoginTransactionID: "txn_1",
		RedirectURL:        "http://127.0.0.1:5291/",
		TrustedSource:      facade.TrustedSource{ClientID: "authorization-console", Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if result.PlatformCode != "console" {
		t.Fatalf("platform code = %q, want console", result.PlatformCode)
	}
	if result.Brand.Title != "Console" || result.Brand.Subtitle != "Ops" {
		t.Fatalf("unexpected brand: %+v", result.Brand)
	}
	if len(result.Methods) != 2 {
		t.Fatalf("method count = %d, want 2", len(result.Methods))
	}
	if result.Methods[0].MethodType != domain.MethodPassword {
		t.Fatalf("first method = %q, want PASSWORD", result.Methods[0].MethodType)
	}
	if result.Methods[1].LoginURL == "" {
		t.Fatalf("external method login url should be populated")
	}
	if !strings.Contains(result.Methods[1].LoginURL, "loginContextId="+result.LoginContextID) {
		t.Fatalf("external method login url %q should include loginContextId %q", result.Methods[1].LoginURL, result.LoginContextID)
	}
	if !strings.Contains(result.Methods[1].LoginURL, "clientId=authorization-console") {
		t.Fatalf("external method login url %q should include trusted clientId", result.Methods[1].LoginURL)
	}
}

func TestResolveLoginOptionsIgnoresExplicitCodeWithoutTrustedSource(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "attacker", PlatformName: "Attacker", Status: domain.StatusActive},
		},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})

	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ExplicitCode: "attacker",
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if result.PlatformCode != "default" {
		t.Fatalf("platform code = %q, want default", result.PlatformCode)
	}
}

func TestResolveLoginOptionsDoesNotTrustPublicClientID(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "console", PlatformName: "Console", Status: domain.StatusActive},
		},
		ssoBindings: []domain.SSOClientBinding{
			{PlatformCode: "console", ClientID: "authorization-console", Status: domain.StatusActive},
		},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})

	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ClientID: "authorization-console",
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if result.PlatformCode != "default" {
		t.Fatalf("platform code = %q, want default", result.PlatformCode)
	}
}

func TestResolveLoginOptionsKeepsDefaultFallbackForCompatiblePlatform(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "seven-admin", PlatformName: "Seven Admin", IsDefault: true, Status: domain.StatusActive},
		},
		methods: map[string][]domain.LoginMethod{
			"seven-admin": {{PlatformCode: "seven-admin", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})

	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ExplicitCode: "attacker",
		TrustedSource: facade.TrustedSource{
			ClientID:    "attacker-client",
			RedirectURL: "https://evil.example/callback",
		},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if result.PlatformCode != "seven-admin" {
		t.Fatalf("platform code = %q, want seven-admin fallback", result.PlatformCode)
	}
}

func TestResolveLoginOptionsStrictDefaultRejectsUntrustedFallback(t *testing.T) {
	for _, settingsJSON := range []string{
		`{"requireTrustedSource":true}`,
		`{"sourcePolicy":"STRICT_MATCH"}`,
	} {
		t.Run(settingsJSON, func(t *testing.T) {
			service := newTestService(t, &fakeRepository{
				platforms: []domain.Platform{
					{
						PlatformCode: "seven-admin",
						PlatformName: "Seven Admin",
						IsDefault:    true,
						SettingsJSON: settingsJSON,
						Status:       domain.StatusActive,
					},
					{PlatformCode: "console", PlatformName: "Console", Status: domain.StatusActive},
				},
				ssoBindings: []domain.SSOClientBinding{
					{PlatformCode: "console", ClientID: "authorization-console", Status: domain.StatusActive},
				},
				methods: map[string][]domain.LoginMethod{
					"seven-admin": {{PlatformCode: "seven-admin", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
				},
			})

			_, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
				ExplicitCode: "console",
				TrustedSource: facade.TrustedSource{
					ClientID:    "attacker-client",
					RedirectURL: "https://evil.example/callback",
				},
			})
			if err == nil {
				t.Fatal("expected strict default platform to reject untrusted fallback")
			}
			if got := apperrors.From(err).Code(); got != apperrors.CodeForbidden {
				t.Fatalf("error code = %d, want forbidden", got)
			}
		})
	}
}

func TestResolveLoginOptionsRejectsKnownClientWithMismatchedRedirect(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{
				PlatformCode: "seven-admin",
				PlatformName: "Seven Admin",
				IsDefault:    true,
				SettingsJSON: `{"requireTrustedSource":true}`,
				Status:       domain.StatusActive,
			},
			{PlatformCode: "console", PlatformName: "Console", Status: domain.StatusActive},
		},
		ssoBindings: []domain.SSOClientBinding{
			{PlatformCode: "console", ClientID: "authorization-console", Status: domain.StatusActive},
		},
		authzSessions: map[string]*ssofacade.AuthorizationSessionSnapshot{
			"txn_console": {
				LoginTransactionID: "txn_console",
				ClientID:           "authorization-console",
				RedirectURI:        "https://console.example.com/callback",
			},
		},
		methods: map[string][]domain.LoginMethod{
			"console": {{PlatformCode: "console", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})

	_, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ClientID:           "authorization-console",
		LoginTransactionID: "txn_console",
		RedirectURL:        "https://evil.example/callback",
		TrustedSource: facade.TrustedSource{
			Host: "127.0.0.1:5291",
		},
	})
	if err == nil {
		t.Fatal("expected mismatched redirect to be rejected")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeForbidden {
		t.Fatalf("error code = %d, want forbidden", got)
	}
}

func TestValidateLoginContextRejectsSourceMismatch(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive}},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if _, err := service.ValidateLoginContext(context.Background(), result.LoginContextID, facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{Host: "evil.example.com"},
	}); err == nil {
		t.Fatal("expected source mismatch error")
	}
}

func TestValidateLoginContextAllowsBrowserHeaderDrift(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive}},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{
			Host:    "127.0.0.1:5291",
			Origin:  "",
			Referer: "http://127.0.0.1:5291/login",
		},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if _, err := service.ValidateLoginContext(context.Background(), result.LoginContextID, facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{
			Host:    "127.0.0.1:5291",
			Origin:  "http://127.0.0.1:5291",
			Referer: "http://127.0.0.1:5291/oauth/landing/github",
		},
	}); err != nil {
		t.Fatalf("ValidateLoginContext should allow Origin/Referer drift: %v", err)
	}
}

func TestValidateLoginContextRejectsMismatchedClientID(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "default", PlatformName: "Default", IsDefault: true, Status: domain.StatusActive}},
		authzSessions: map[string]*ssofacade.AuthorizationSessionSnapshot{
			"txn_console": {
				LoginTransactionID: "txn_console",
				ClientID:           "authorization-console",
				RedirectURI:        "http://127.0.0.1:5291/",
			},
		},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	result, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ClientID:           "authorization-console",
		LoginTransactionID: "txn_console",
		RedirectURL:        "http://127.0.0.1:5291/",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	if _, err := service.ValidateLoginContext(context.Background(), result.LoginContextID, facade.ResolvePlatformRequest{
		ClientID:           "attacker-client",
		LoginTransactionID: "txn_console",
		RedirectURL:        "http://127.0.0.1:5291/",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	}); err == nil {
		t.Fatal("expected mismatched clientId to be rejected")
	}
}

func TestProvisioningPolicyRequiresIssuedAuthority(t *testing.T) {
	defaultOrgID := int64(41)
	defaultDeptID := int64(21)
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{
			PlatformCode:      "default",
			PlatformName:      "Default",
			IsDefault:         true,
			AllowAutoRegister: true,
			DefaultDeptID:     &defaultDeptID,
			SettingsJSON:      `{"defaultOrgId":41,"defaultPostIds":[31,32,31,0]}`,
			Status:            domain.StatusActive,
		}},
		authzSessions: map[string]*ssofacade.AuthorizationSessionSnapshot{
			"txn_1": {LoginTransactionID: "txn_1", ClientID: "authorization-console", RedirectURI: "http://127.0.0.1:5291/"},
		},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	service.transactor = platformTestSnapshotTransactor{}
	options, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		LoginTransactionID: "txn_1",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	authority, err := service.IssueProvisioningAuthority(context.Background(), options.LoginContextID, facade.ResolvePlatformRequest{
		LoginTransactionID: "txn_1",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("IssueProvisioningAuthority returned error: %v", err)
	}
	if strings.TrimSpace(authority.AuthorityID) == "" || authority.PlatformCode != "default" {
		t.Fatalf("unexpected authority: %#v", authority)
	}
	if _, err := service.GetProvisioningPolicy(context.Background(), facade.ProvisioningAuthority{PlatformCode: "default", Authority: facade.AuthorityProvisioning}); err == nil {
		t.Fatal("expected missing authority id to be rejected")
	}
	policy, err := service.GetProvisioningPolicy(context.Background(), *authority)
	if err != nil {
		t.Fatalf("GetProvisioningPolicy returned error: %v", err)
	}
	if !policy.AllowAutoRegister || policy.PlatformCode != "default" {
		t.Fatalf("unexpected policy: %#v", policy)
	}
	if policy.DefaultDeptID == nil || *policy.DefaultDeptID != defaultDeptID {
		t.Fatalf("default dept id = %#v, want %d", policy.DefaultDeptID, defaultDeptID)
	}
	if policy.DefaultOrgID == nil || *policy.DefaultOrgID != defaultOrgID {
		t.Fatalf("default org id = %#v, want %d", policy.DefaultOrgID, defaultOrgID)
	}
	if got := policy.DefaultPostIDs; len(got) != 2 || got[0] != 31 || got[1] != 32 {
		t.Fatalf("default post ids = %#v, want [31 32]", got)
	}
	if _, err := service.GetProvisioningPolicy(context.Background(), *authority); err == nil {
		t.Fatal("expected provisioning authority to be single-use")
	}
}

type platformTestSnapshotTransactor struct{}

func (platformTestSnapshotTransactor) Enabled() bool { return true }
func (platformTestSnapshotTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
func (platformTestSnapshotTransactor) WithinReadOnlySnapshot(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

var _ store.Snapshotter = platformTestSnapshotTransactor{}

func TestPlatformDefaultRolesFailClosedWithoutSnapshotter(t *testing.T) {
	service := newTestService(t, &fakeRepository{})
	service.transactor = nil
	_, err := service.buildRegistrationPolicy(context.Background(), domain.Platform{
		PlatformCode:      "default",
		AllowFormRegister: true,
	}, true, true)
	if err == nil {
		t.Fatal("expected platform default-role composition to fail without a consistent snapshot")
	}
}

func TestIssueProvisioningAuthorityRejectsPresentationOnlyContext(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "default", PlatformName: "Default", IsDefault: true, AllowAutoRegister: true, Status: domain.StatusActive}},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	options, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	_, err = service.IssueProvisioningAuthority(context.Background(), options.LoginContextID, facade.ResolvePlatformRequest{
		TrustedSource: facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err == nil {
		t.Fatal("expected presentation-only login context to reject provisioning authority")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeForbidden {
		t.Fatalf("error code = %d, want forbidden", got)
	}
}

func TestIssueProvisioningAuthorityConsumesLoginContext(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "default", PlatformName: "Default", IsDefault: true, AllowAutoRegister: true, Status: domain.StatusActive}},
		authzSessions: map[string]*ssofacade.AuthorizationSessionSnapshot{
			"txn_1": {LoginTransactionID: "txn_1", ClientID: "authorization-console", RedirectURI: "http://127.0.0.1:5291/"},
		},
		methods: map[string][]domain.LoginMethod{
			"default": {{PlatformCode: "default", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	})
	options, err := service.ResolveLoginOptions(context.Background(), facade.ResolvePlatformRequest{
		ClientID:           "authorization-console",
		LoginTransactionID: "txn_1",
		RedirectURL:        "http://127.0.0.1:5291/",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("ResolveLoginOptions returned error: %v", err)
	}
	req := facade.ResolvePlatformRequest{
		ClientID:           "authorization-console",
		LoginTransactionID: "txn_1",
		RedirectURL:        "http://127.0.0.1:5291/",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
	}
	if _, err := service.IssueProvisioningAuthority(context.Background(), options.LoginContextID, req); err != nil {
		t.Fatalf("first IssueProvisioningAuthority returned error: %v", err)
	}
	if _, err := service.IssueProvisioningAuthority(context.Background(), options.LoginContextID, req); err == nil {
		t.Fatal("expected reused login context to reject provisioning authority")
	}
}

func TestListAndGetPlatformMapAdminDetail(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{ID: 10, PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		methods: map[string][]domain.LoginMethod{
			"seven-admin": {{ID: 20, PlatformCode: "seven-admin", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
		sourceRulesByPlatform: map[string][]domain.SourceRule{
			"seven-admin": {{ID: 30, PlatformCode: "seven-admin", MatchType: domain.MatchHost, MatchValue: "127.0.0.1:5291", Status: domain.StatusActive}},
		},
		defaultRolesByPlatform: map[string][]domain.DefaultRole{
			"seven-admin": {{ID: 40, PlatformCode: "seven-admin", RoleID: 101, AutoAssignEnabled: true, Status: domain.StatusActive}},
		},
	})
	service.transactor = platformTestSnapshotTransactor{}

	page, err := service.ListPlatforms(context.Background(), facade.PlatformQuery{Current: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("ListPlatforms returned error: %v", err)
	}
	if page.Total != 1 || len(page.Records) != 1 || page.Records[0].PlatformCode != "seven-admin" {
		t.Fatalf("unexpected page: %+v", page)
	}
	detail, err := service.GetPlatform(context.Background(), "seven-admin")
	if err != nil {
		t.Fatalf("GetPlatform returned error: %v", err)
	}
	if len(detail.LoginMethods) != 1 || len(detail.SourceRules) != 1 || len(detail.DefaultRoles) != 1 {
		t.Fatalf("detail did not include child records: %+v", detail)
	}
}

func TestListPlatformsUsesFixedChildQueryCount(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "a1", Status: domain.StatusActive},
			{PlatformCode: "a2", Status: domain.StatusActive},
			{PlatformCode: "a3", Status: domain.StatusActive},
		},
		methods:                map[string][]domain.LoginMethod{},
		sourceRulesByPlatform:  map[string][]domain.SourceRule{},
		defaultRolesByPlatform: map[string][]domain.DefaultRole{},
	}
	service := newTestService(t, repo)
	service.transactor = platformTestSnapshotTransactor{}
	if _, err := service.ListPlatforms(context.Background(), facade.PlatformQuery{Current: 1, PageSize: 10}); err != nil {
		t.Fatalf("ListPlatforms() error=%v", err)
	}
	if repo.detailReadCalls != 3 {
		t.Fatalf("child repository calls=%d, want fixed 3", repo.detailReadCalls)
	}
}

func TestPlatformSensitiveMutationsRequireStepUpAndReason(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", IsDefault: true, Status: domain.StatusActive}},
	})
	err := service.ReplaceDefaultRoles(context.Background(), 7, "seven-admin", []facade.DefaultRoleSaveRequest{{RoleID: 101, AutoAssignEnabled: true}}, stepup.ProofMetadata{})
	if err == nil {
		t.Fatal("expected step-up proof error")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeForbidden {
		t.Fatalf("error code = %d, want forbidden", got)
	}
	err = service.UpdatePlatformStatus(context.Background(), 7, "seven-admin", facade.PlatformStatusRequest{Status: domain.StatusDisabled}, validPlatformProof(StepUpActionPlatformStatusChange, BuildPlatformStatusOperationBinding("seven-admin", domain.StatusDisabled)))
	if err == nil {
		t.Fatal("expected missing reason error")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeParamsError {
		t.Fatalf("error code = %d, want params", got)
	}
}

func TestPlatformDefaultRoleRejectsHighRiskRole(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		roleSafety: []domain.RoleSafety{{
			RoleID:          101,
			Exists:          true,
			Active:          true,
			AutoAssignable:  true,
			PermissionCodes: []string{"system:user:list"},
		}},
	})
	err := service.ReplaceDefaultRoles(context.Background(), 7, "seven-admin", []facade.DefaultRoleSaveRequest{{RoleID: 101, AutoAssignEnabled: true}}, validPlatformProof(StepUpActionPlatformDefaultRolesReplace, BuildPlatformDefaultRolesOperationBinding("seven-admin")))
	if err == nil {
		t.Fatal("expected high-risk role rejection")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeOperateError {
		t.Fatalf("error code = %d, want operation error", got)
	}
}

func TestReplaceLoginMethodsRejectsUnknownExternalProvider(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		providers: map[string]bool{"github": true},
	})
	err := service.ReplaceLoginMethods(context.Background(), 7, "seven-admin", []facade.LoginMethodSaveRequest{{
		MethodType:     domain.MethodExternalOAuth,
		ProviderCode:   "unknown",
		DisplayName:    "Unknown",
		DisplayEnabled: true,
		LoginEnabled:   true,
	}}, validPlatformProof(StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding("seven-admin")))
	if err == nil {
		t.Fatal("expected unknown provider rejection")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeParamsError {
		t.Fatalf("error code = %d, want params", got)
	}
}

func TestPlatformPolicyMutationArraysAreHardBoundedBeforeRepositoryReads(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		providers: map[string]bool{},
		roleSafety: []domain.RoleSafety{{
			RoleID:         101,
			Exists:         true,
			Active:         true,
			AutoAssignable: true,
		}},
	}
	service := newTestService(t, repo)

	methods := make([]facade.LoginMethodSaveRequest, 101)
	for index := range methods {
		providerCode := "provider-" + strconv.Itoa(index)
		repo.providers[providerCode] = true
		methods[index] = facade.LoginMethodSaveRequest{
			MethodType:     domain.MethodExternalOAuth,
			ProviderCode:   providerCode,
			DisplayName:    "Provider " + strconv.Itoa(index),
			DisplayEnabled: true,
			LoginEnabled:   true,
		}
	}
	err := service.ReplaceLoginMethods(context.Background(), 7, "seven-admin", methods, validPlatformProof(StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding("seven-admin")))
	if err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("oversized login methods err=%v", err)
	}
	if repo.providerLookupCalls != 0 {
		t.Fatalf("oversized login methods performed provider reads=%d, want 0", repo.providerLookupCalls)
	}

	rules := make([]facade.SourceRuleSaveRequest, 201)
	for index := range rules {
		rules[index] = facade.SourceRuleSaveRequest{
			MatchType:  domain.MatchHost,
			MatchValue: "host-" + strconv.Itoa(index) + ".example.test",
			Status:     domain.StatusActive,
		}
	}
	err = service.ReplaceSourceRules(context.Background(), 7, "seven-admin", rules, validPlatformProof(StepUpActionPlatformSourceRulesReplace, BuildPlatformSourceRulesOperationBinding("seven-admin")))
	if err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("oversized source rules err=%v", err)
	}
	if repo.replaceSourceRuleCalls != 0 {
		t.Fatalf("oversized source rules reached replace=%d, want 0", repo.replaceSourceRuleCalls)
	}

	roles := []facade.DefaultRoleSaveRequest{
		{RoleID: 101, AutoAssignEnabled: true},
		{RoleID: 101, AutoAssignEnabled: true},
		{RoleID: 101, AutoAssignEnabled: true},
		{RoleID: 101, AutoAssignEnabled: true},
	}
	err = service.ReplaceDefaultRoles(context.Background(), 7, "seven-admin", roles, validPlatformProof(StepUpActionPlatformDefaultRolesReplace, BuildPlatformDefaultRolesOperationBinding("seven-admin")))
	if err == nil || apperrors.From(err).Code() != apperrors.CodeOperateError {
		t.Fatalf("oversized default roles err=%v", err)
	}
	if repo.validateDefaultRoleCalls != 0 {
		t.Fatalf("oversized default roles performed validation reads=%d, want 0", repo.validateDefaultRoleCalls)
	}
}

func TestReplaceLoginMethodsValidatesExternalProvidersWithOneSetLookup(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		providers: map[string]bool{
			"github": true,
			"google": true,
			"gitlab": true,
		},
	}
	service := newTestService(t, repo)
	err := service.ReplaceLoginMethods(context.Background(), 7, "seven-admin", []facade.LoginMethodSaveRequest{
		{MethodType: domain.MethodExternalOAuth, ProviderCode: "github", DisplayName: "GitHub", DisplayEnabled: true, LoginEnabled: true},
		{MethodType: domain.MethodExternalOAuth, ProviderCode: "google", DisplayName: "Google", DisplayEnabled: true, LoginEnabled: true},
		{MethodType: domain.MethodExternalOAuth, ProviderCode: "gitlab", DisplayName: "GitLab", DisplayEnabled: true, LoginEnabled: true},
	}, validPlatformProof(StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding("seven-admin")))
	if err != nil {
		t.Fatalf("ReplaceLoginMethods returned error: %v", err)
	}
	if repo.providerLookupCalls != 1 {
		t.Fatalf("provider lookup calls=%d, want one bounded set lookup", repo.providerLookupCalls)
	}
}

func TestReplaceLoginMethodsRevokesDisabledMethodSessions(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		providers: map[string]bool{"github": true, "google": true},
		methods: map[string][]domain.LoginMethod{
			"seven-admin": {
				{PlatformCode: "seven-admin", MethodType: domain.MethodExternalOAuth, ProviderCode: "github", DisplayName: "GitHub", DisplayEnabled: true, LoginEnabled: true},
				{PlatformCode: "seven-admin", MethodType: domain.MethodExternalOAuth, ProviderCode: "google", DisplayName: "Google", DisplayEnabled: true, LoginEnabled: true},
				{PlatformCode: "seven-admin", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true},
			},
		},
	}
	service := newTestService(t, repo)

	err := service.ReplaceLoginMethods(context.Background(), 7, "seven-admin", []facade.LoginMethodSaveRequest{
		{MethodType: domain.MethodExternalOAuth, ProviderCode: "github", DisplayName: "GitHub", DisplayEnabled: true, LoginEnabled: false},
		{MethodType: domain.MethodExternalOAuth, ProviderCode: "google", DisplayName: "Google", DisplayEnabled: true, LoginEnabled: true},
		{MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true},
	}, validPlatformProof(StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding("seven-admin")))
	if err != nil {
		t.Fatalf("ReplaceLoginMethods returned error: %v", err)
	}
	if len(repo.sessions.revokedMethods) != 1 {
		t.Fatalf("revoked methods = %+v, want exactly one", repo.sessions.revokedMethods)
	}
	if got := repo.sessions.revokedMethods[0]; got != "seven-admin|EXTERNAL_OAUTH|github" {
		t.Fatalf("revoked method = %q, want github external OAuth", got)
	}
}

func TestReplaceLoginMethodsFailsClosedWhenRevokerMissingForDisabledMethod(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", Status: domain.StatusActive}},
		methods: map[string][]domain.LoginMethod{
			"seven-admin": {{PlatformCode: "seven-admin", MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
		},
	}
	service := newTestService(t, repo)
	service.BindSessions(nil)

	err := service.ReplaceLoginMethods(context.Background(), 7, "seven-admin", []facade.LoginMethodSaveRequest{
		{MethodType: domain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: false},
	}, validPlatformProof(StepUpActionPlatformLoginMethodsReplace, BuildPlatformLoginMethodsOperationBinding("seven-admin")))
	if err == nil {
		t.Fatal("expected disabled method to fail closed without session revoker")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeSystemError {
		t.Fatalf("error code = %d, want system error", got)
	}
}

func TestDisablePlatformRejectsDefaultPlatform(t *testing.T) {
	service := newTestService(t, &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "backup-admin", PlatformName: "Backup Admin", PlatformType: "ADMIN", Status: domain.StatusActive},
		},
	})
	err := service.UpdatePlatformStatus(context.Background(), 7, "seven-admin", facade.PlatformStatusRequest{Status: domain.StatusDisabled, Reason: "maintenance"}, validPlatformProof(StepUpActionPlatformStatusChange, BuildPlatformStatusOperationBinding("seven-admin", domain.StatusDisabled)))
	if err == nil {
		t.Fatal("expected disabling default platform to be rejected")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeOperateError {
		t.Fatalf("error code = %d, want operation error", got)
	}
}

func TestDisablePlatformRevokesActivePlatformSessions(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "backup-admin", PlatformName: "Backup Admin", PlatformType: "ADMIN", Status: domain.StatusActive},
		},
	}
	service := newTestService(t, repo)
	err := service.UpdatePlatformStatus(context.Background(), 7, "backup-admin", facade.PlatformStatusRequest{Status: domain.StatusDisabled, Reason: "maintenance"}, validPlatformProof(StepUpActionPlatformStatusChange, BuildPlatformStatusOperationBinding("backup-admin", domain.StatusDisabled)))
	if err != nil {
		t.Fatalf("UpdatePlatformStatus returned error: %v", err)
	}
	if repo.sessions.revokedPlatformCode != "backup-admin" {
		t.Fatalf("revoked platform = %q, want backup-admin", repo.sessions.revokedPlatformCode)
	}
	if repo.sessions.revokePlatformCalls != 1 {
		t.Fatalf("revoke calls = %d, want 1", repo.sessions.revokePlatformCalls)
	}
}

func TestDisablePlatformFailsClosedWhenSessionRevokerMissing(t *testing.T) {
	repo := &fakeRepository{
		platforms: []domain.Platform{
			{PlatformCode: "seven-admin", PlatformName: "Seven Admin", PlatformType: "ADMIN", IsDefault: true, Status: domain.StatusActive},
			{PlatformCode: "backup-admin", PlatformName: "Backup Admin", PlatformType: "ADMIN", Status: domain.StatusActive},
		},
	}
	service := newTestService(t, repo)
	service.BindSessions(nil)

	err := service.UpdatePlatformStatus(context.Background(), 7, "backup-admin", facade.PlatformStatusRequest{Status: domain.StatusDisabled, Reason: "maintenance"}, validPlatformProof(StepUpActionPlatformStatusChange, BuildPlatformStatusOperationBinding("backup-admin", domain.StatusDisabled)))
	if err == nil {
		t.Fatal("expected missing session revoker to fail closed")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeSystemError {
		t.Fatalf("error code = %d, want system error", got)
	}
}

func TestCreatePlatformAllowsDisabledStatus(t *testing.T) {
	repo := &fakeRepository{}
	service := newTestService(t, repo)
	disabled := domain.StatusDisabled

	detail, err := service.CreatePlatform(context.Background(), 7, facade.PlatformSaveRequest{
		PlatformCode: "copy-admin",
		PlatformName: "Copy Admin",
		PlatformType: "ADMIN",
		Status:       &disabled,
		Reason:       "copy platform",
	}, stepup.ProofMetadata{})
	if err != nil {
		t.Fatalf("CreatePlatform returned error: %v", err)
	}
	if detail.Status != domain.StatusDisabled {
		t.Fatalf("status = %d, want disabled", detail.Status)
	}
	if len(repo.platforms) != 1 || repo.platforms[0].Status != domain.StatusDisabled {
		t.Fatalf("inserted platforms = %+v, want one disabled platform", repo.platforms)
	}
}

func TestCreatePlatformRejectsDisabledDefault(t *testing.T) {
	service := newTestService(t, &fakeRepository{})
	disabled := domain.StatusDisabled

	_, err := service.CreatePlatform(context.Background(), 7, facade.PlatformSaveRequest{
		PlatformCode: "default-admin",
		PlatformName: "Default Admin",
		PlatformType: "ADMIN",
		IsDefault:    true,
		Status:       &disabled,
		Reason:       "copy platform",
	}, stepup.ProofMetadata{})
	if err == nil {
		t.Fatal("expected disabled default platform to be rejected")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeOperateError {
		t.Fatalf("error code = %d, want operation error", got)
	}
}

type fakeRepository struct {
	platforms                []domain.Platform
	ssoBindings              []domain.SSOClientBinding
	sourceRules              []domain.SourceRule
	sourceRulesByPlatform    map[string][]domain.SourceRule
	methods                  map[string][]domain.LoginMethod
	defaultRolesByPlatform   map[string][]domain.DefaultRole
	providers                map[string]bool
	roleSafety               []domain.RoleSafety
	authzSessions            map[string]*ssofacade.AuthorizationSessionSnapshot
	sessions                 *fakePlatformSessions
	detailReadCalls          int
	providerLookupCalls      int
	replaceSourceRuleCalls   int
	validateDefaultRoleCalls int
}

func (f *fakeRepository) ListActivePlatforms(context.Context) ([]domain.Platform, error) {
	return append([]domain.Platform(nil), f.platforms...), nil
}

func (f *fakeRepository) ListPlatforms(context.Context, domain.PlatformQuery) ([]domain.Platform, int64, error) {
	return append([]domain.Platform(nil), f.platforms...), int64(len(f.platforms)), nil
}

func (f *fakeRepository) FindPlatform(_ context.Context, platformCode string) (*domain.Platform, error) {
	for i := range f.platforms {
		if f.platforms[i].PlatformCode == platformCode {
			return &f.platforms[i], nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) ListActiveSSOClientBindings(context.Context) ([]domain.SSOClientBinding, error) {
	return append([]domain.SSOClientBinding(nil), f.ssoBindings...), nil
}

func (f *fakeRepository) ListActiveSourceRules(context.Context) ([]domain.SourceRule, error) {
	return append([]domain.SourceRule(nil), f.sourceRules...), nil
}

func (f *fakeRepository) ListLoginMethods(_ context.Context, platformCode string) ([]domain.LoginMethod, error) {
	f.detailReadCalls++
	return append([]domain.LoginMethod(nil), f.methods[platformCode]...), nil
}

func (f *fakeRepository) ListSourceRules(_ context.Context, platformCode string) ([]domain.SourceRule, error) {
	f.detailReadCalls++
	return append([]domain.SourceRule(nil), f.sourceRulesByPlatform[platformCode]...), nil
}

func (f *fakeRepository) ListDefaultRoleRecords(_ context.Context, platformCode string) ([]domain.DefaultRole, error) {
	f.detailReadCalls++
	return append([]domain.DefaultRole(nil), f.defaultRolesByPlatform[platformCode]...), nil
}

func (f *fakeRepository) ListLoginMethodsByPlatformCodes(_ context.Context, platformCodes []string) ([]domain.LoginMethod, error) {
	f.detailReadCalls++
	var result []domain.LoginMethod
	for _, code := range platformCodes {
		result = append(result, f.methods[code]...)
	}
	return result, nil
}

func (f *fakeRepository) ListSourceRulesByPlatformCodes(_ context.Context, platformCodes []string) ([]domain.SourceRule, error) {
	f.detailReadCalls++
	var result []domain.SourceRule
	for _, code := range platformCodes {
		result = append(result, f.sourceRulesByPlatform[code]...)
	}
	return result, nil
}

func (f *fakeRepository) ListDefaultRoleRecordsByPlatformCodes(_ context.Context, platformCodes []string) ([]domain.DefaultRole, error) {
	f.detailReadCalls++
	var result []domain.DefaultRole
	for _, code := range platformCodes {
		result = append(result, f.defaultRolesByPlatform[code]...)
	}
	return result, nil
}

func (f *fakeRepository) ListDefaultRoles(context.Context, string, int) ([]domain.DefaultRole, error) {
	return nil, nil
}

func (f *fakeRepository) FindDefaultPlatform(context.Context) (*domain.Platform, error) {
	for i := range f.platforms {
		if f.platforms[i].IsDefault {
			return &f.platforms[i], nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) FindManagedDefaultPlatform(ctx context.Context) (*domain.Platform, error) {
	return f.FindDefaultPlatform(ctx)
}

func (f *fakeRepository) FindManagedDefaultPlatformForUpdate(ctx context.Context) (*domain.Platform, error) {
	return f.FindDefaultPlatform(ctx)
}

func (f *fakeRepository) ListManagedLoginMethods(ctx context.Context, platformCode string) ([]domain.LoginMethod, error) {
	return f.ListLoginMethods(ctx, platformCode)
}

func (f *fakeRepository) ListManagedLoginMethodsForUpdate(ctx context.Context, platformCode string) ([]domain.LoginMethod, error) {
	return f.ListLoginMethods(ctx, platformCode)
}

func (f *fakeRepository) ListManagedSourceRules(ctx context.Context, platformCode string) ([]domain.SourceRule, error) {
	return f.ListSourceRules(ctx, platformCode)
}

func (f *fakeRepository) ListManagedSourceRulesForUpdate(ctx context.Context, platformCode string) ([]domain.SourceRule, error) {
	return f.ListSourceRules(ctx, platformCode)
}

func (f *fakeRepository) InsertPlatform(_ context.Context, platform domain.Platform, _ int64) error {
	f.platforms = append(f.platforms, platform)
	return nil
}
func (f *fakeRepository) UpdatePlatform(context.Context, domain.Platform, int64) error { return nil }
func (f *fakeRepository) UpdatePlatformStatus(_ context.Context, platformCode string, status int, _ int64) error {
	for i := range f.platforms {
		if f.platforms[i].PlatformCode == platformCode {
			f.platforms[i].Status = status
		}
	}
	return nil
}
func (f *fakeRepository) ReplaceLoginMethods(context.Context, string, []domain.LoginMethod, int64) error {
	return nil
}
func (f *fakeRepository) ReplaceSourceRules(context.Context, string, []domain.SourceRule, int64) error {
	f.replaceSourceRuleCalls++
	return nil
}
func (f *fakeRepository) ReplaceDefaultRoles(context.Context, string, []domain.DefaultRole, int64) error {
	return nil
}
func (f *fakeRepository) ListAvailableExternalProviderCodes(_ context.Context, providerCodes []string) ([]string, error) {
	f.providerLookupCalls++
	result := make([]string, 0, len(providerCodes))
	for _, code := range providerCodes {
		if f.providers[code] {
			result = append(result, code)
		}
	}
	return result, nil
}
func (f *fakeRepository) ListManagedExternalProviderCodes(ctx context.Context, providerCodes []string) ([]string, error) {
	return f.ListAvailableExternalProviderCodes(ctx, providerCodes)
}
func (f *fakeRepository) ValidateDefaultRoles(context.Context, []int64) ([]domain.RoleSafety, error) {
	f.validateDefaultRoleCalls++
	return append([]domain.RoleSafety(nil), f.roleSafety...), nil
}

func newTestService(t *testing.T, repo *fakeRepository) *Service {
	t.Helper()
	generator, err := xid.New(8)
	if err != nil {
		t.Fatalf("create xid generator: %v", err)
	}
	service := NewService(repo, newFakeCache(), generator)
	service.BindAuthorizationSessions(&fakeAuthorizationSessions{sessions: repo.authzSessions})
	if repo.sessions == nil {
		repo.sessions = &fakePlatformSessions{}
	}
	service.BindSessions(repo.sessions)
	return service
}

type fakePlatformSessions struct {
	revokePlatformCalls int
	revokedPlatformCode string
	revokedMethods      []string
}

func (f *fakePlatformSessions) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakePlatformSessions) ListActiveSessions(context.Context) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakePlatformSessions) CountActiveSessions(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakePlatformSessions) RevokeSession(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakePlatformSessions) RevokeSessionsByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (f *fakePlatformSessions) RevokeSessionsByPlatformCode(_ context.Context, platformCode string) (int64, error) {
	f.revokePlatformCalls++
	f.revokedPlatformCode = platformCode
	return 2, nil
}

func (f *fakePlatformSessions) RevokeSessionsByPlatformLoginMethod(_ context.Context, platformCode string, loginMethod string, externalProviderCode string) (int64, error) {
	f.revokedMethods = append(f.revokedMethods, strings.TrimSpace(platformCode)+"|"+strings.TrimSpace(loginMethod)+"|"+strings.TrimSpace(externalProviderCode))
	return 1, nil
}

func (f *fakePlatformSessions) RevokeSessionsByExternalProvider(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakePlatformSessions) RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error) {
	return 0, nil
}

func (f *fakePlatformSessions) ResolveActiveSessionRecord(context.Context, string) (*ssofacade.SessionRecord, error) {
	return nil, nil
}

type fakeAuthorizationSessions struct {
	sessions map[string]*ssofacade.AuthorizationSessionSnapshot
}

func (f *fakeAuthorizationSessions) CreateAuthorizationSession(context.Context, ssofacade.CreateAuthorizationSessionRequest) (*ssofacade.AuthorizationSessionSnapshot, error) {
	return nil, nil
}

func (f *fakeAuthorizationSessions) GetAuthorizationSession(_ context.Context, loginTransactionID string) (*ssofacade.AuthorizationSessionSnapshot, error) {
	if f == nil {
		return nil, nil
	}
	return f.sessions[loginTransactionID], nil
}

func (f *fakeAuthorizationSessions) RemoveAuthorizationSession(context.Context, string) error {
	return nil
}

func (f *fakeAuthorizationSessions) AcquireCompletionLock(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeAuthorizationSessions) ReleaseCompletionLock(context.Context, string) error {
	return nil
}

func (f *fakeAuthorizationSessions) MarkSessionFinalized(context.Context, string) (bool, error) {
	return true, nil
}

func (f *fakeAuthorizationSessions) ReleaseSessionFinalized(context.Context, string) error {
	return nil
}

type fakeCache struct {
	mu     sync.Mutex
	values map[string]any
}

func newFakeCache() *fakeCache {
	return &fakeCache{values: map[string]any{}}
}

func (f *fakeCache) Get(_ context.Context, cacheKey string, dest any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[cacheKey]
	if !ok {
		return false, nil
	}
	assignFakeCachePayload(value, dest)
	return true, nil
}

func (f *fakeCache) GetDel(_ context.Context, cacheKey string, dest any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[cacheKey]
	if !ok {
		return false, nil
	}
	assignFakeCachePayload(value, dest)
	delete(f.values, cacheKey)
	return true, nil
}

func (f *fakeCache) Set(_ context.Context, cacheKey string, value any, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[cacheKey] = value
	return nil
}

func (f *fakeCache) CompareAndDelete(_ context.Context, cacheKey string, expected any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	value, ok := f.values[cacheKey]
	if !ok || value != expected {
		return false, nil
	}
	delete(f.values, cacheKey)
	return true, nil
}

func assignFakeCachePayload(value any, dest any) {
	switch out := dest.(type) {
	case *loginContextPayload:
		*out = value.(loginContextPayload)
	case *provisioningAuthorityPayload:
		*out = value.(provisioningAuthorityPayload)
	default:
		panic("unsupported cache payload")
	}
}

func validPlatformProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-1",
		ChallengeIdentifier:   "challenge-1",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
}
