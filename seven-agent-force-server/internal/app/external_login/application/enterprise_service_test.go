package application

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestEnterpriseStepUpActionsMatchChallengeContract(t *testing.T) {
	tests := map[string]challengedomain.BusinessAction{
		StepUpActionEnterpriseConnectionSave:   challengedomain.BusinessActionEnterpriseConnectionSave,
		StepUpActionEnterpriseConnectionStatus: challengedomain.BusinessActionEnterpriseConnectionStatus,
		StepUpActionEnterpriseLoginAccess:      challengedomain.BusinessActionEnterpriseIdentityLoginAccess,
		StepUpActionEnterpriseIdentityUnbind:   challengedomain.BusinessActionEnterpriseIdentityUnbind,
	}
	for applicationAction, challengeAction := range tests {
		if applicationAction != string(challengeAction) {
			t.Fatalf("enterprise step-up action %q does not match challenge action %q", applicationAction, challengeAction)
		}
	}
}

func TestEnterpriseBindingDoesNotCreateLoginAuthority(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatalf("start binding: %v", err)
	}
	state := stateFromRedirect(t, start.RedirectURL)
	result, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: state, Code: "code"})
	if err != nil {
		t.Fatalf("complete binding: %v", err)
	}
	if result.Authenticated || result.Outcome != "BOUND" {
		t.Fatalf("binding result = %#v", result)
	}
	if hasActiveGrant(fixture.repo.grants[result.ExternalIdentityID], domain.EnterpriseGrantLogin) {
		t.Fatal("binding granted LOGIN")
	}
	if !hasActiveGrant(fixture.repo.grants[result.ExternalIdentityID], domain.EnterpriseGrantNotificationDirect) {
		t.Fatal("binding did not grant DIRECT")
	}
}

func TestUnpublishedEnterpriseLoginRejectsStartAndInflightCallbackButKeepsBinding(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	loginStart, err := fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: "FEISHU", OrganizationCode: "acme"})
	if err != nil {
		t.Fatalf("prepare login authorization: %v", err)
	}
	fixture.service.enterpriseLoginEnabled = false

	if _, err = fixture.service.ResolveEnterpriseLoginOptions(context.Background(), "acme"); err == nil {
		t.Fatal("unpublished enterprise login exposed discovery options")
	}
	if _, err = fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: "FEISHU", OrganizationCode: "acme"}); err == nil {
		t.Fatal("unpublished enterprise login accepted a new authorization")
	}
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, loginStart.RedirectURL), Code: "code"}); err == nil {
		t.Fatal("unpublished enterprise login completed an in-flight authorization")
	}
	if fixture.bootstrap.called {
		t.Fatal("unpublished enterprise login created a platform session")
	}

	bindingStart, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatalf("notification binding should remain available: %v", err)
	}
	result, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, bindingStart.RedirectURL), Code: "code"})
	if err != nil || result.Authenticated || result.Outcome != "BOUND" {
		t.Fatalf("notification binding result=%#v err=%v", result, err)
	}
}

func TestEnterpriseBindingInternalFailureTerminatesConsumedAuthorization(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.repo.insertIdentityErr = fmt.Errorf("database insert failed")
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatalf("start binding: %v", err)
	}
	state := stateFromRedirect(t, start.RedirectURL)
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: state, Code: "code"}); err == nil {
		t.Fatal("binding unexpectedly succeeded")
	}
	authorization := fixture.repo.authorizations[hashEnterpriseValue(state)]
	if authorization == nil {
		t.Fatal("authorization missing")
	}
	if authorization.Status != domain.EnterpriseAuthorizationFailed || authorization.ReasonCode != "ENTERPRISE_AUTHORIZATION_INTERNAL_ERROR" {
		t.Fatalf("authorization = %#v", authorization)
	}
}

func TestEnterpriseConnectionSetupExposesCallbacksBeforeConnectionsExist(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.repo.connections = map[int64]domain.EnterpriseConnection{}
	setup, err := fixture.service.GetEnterpriseConnectionSetup(context.Background(), 101)
	if err != nil {
		t.Fatalf("get setup: %v", err)
	}
	if len(setup.Providers) != 2 {
		t.Fatalf("providers = %#v", setup.Providers)
	}
	if setup.OrganizationCode != "acme" || setup.LoginURL != "https://console.example/login?org=acme" {
		t.Fatalf("organization login setup = %#v", setup)
	}
	for _, item := range setup.Providers {
		want := "https://console.example/external-identities/oauth/callback/" + strings.ToLower(item.Provider)
		if item.CallbackURL != want {
			t.Fatalf("%s callback = %q, want %q", item.Provider, item.CallbackURL, want)
		}
	}
}

func TestEnterpriseConnectionRouteProvisionFailureReturnsSafeActionableError(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.service.notificationRoutes = enterpriseRouteProvisionerFunc(func(context.Context, int64, int64, int64, string, string, bool, int64) error {
		return fmt.Errorf("database implementation detail")
	})
	request := facade.SaveEnterpriseConnectionRequest{
		Provider:                   "FEISHU",
		DisplayName:                "飞书",
		BindingEnabled:             true,
		NotificationDirectEnabled:  true,
		NotificationMentionEnabled: true,
		ExpectedVersion:            1,
	}
	proof := enterpriseProof(StepUpActionEnterpriseConnectionSave, "enterprise-connection:101:FEISHU|version:1")

	_, err := fixture.service.SaveEnterpriseConnection(context.Background(), 101, 201, request, proof)
	if err == nil {
		t.Fatal("route provision failure succeeded")
	}
	safe, ok := err.(interface {
		Message() string
		Details() any
	})
	if !ok {
		t.Fatalf("route provision error type=%T", err)
	}
	if safe.Message() != "连接已验证，但个人通知通道初始化失败，请稍后重试" {
		t.Fatalf("message=%q", safe.Message())
	}
	details, ok := safe.Details().(map[string]string)
	if !ok || details["reasonCode"] != "ENTERPRISE_NOTIFICATION_ROUTE_PROVISION_FAILED" {
		t.Fatalf("details=%#v", safe.Details())
	}
	if strings.Contains(err.Error(), "database implementation detail") {
		t.Fatalf("implementation detail leaked: %v", err)
	}
}

func TestDiscoverEnterpriseGroupsReturnsSafeActionableProviderGuidance(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.service.providers = enterpriseRegistry{adapter: enterpriseAdapter{
		provider:          domain.EnterpriseProviderFeishu,
		groupDiscoveryErr: testGroupDiscoveryRejection{},
	}}

	_, err := fixture.service.DiscoverEnterpriseGroups(context.Background(), 101, 301)
	if err == nil {
		t.Fatal("provider group discovery rejection succeeded")
	}
	safe, ok := err.(interface {
		Message() string
		Details() any
	})
	if !ok {
		t.Fatalf("group discovery error type=%T", err)
	}
	if !strings.Contains(safe.Message(), "im:chat:readonly") {
		t.Fatalf("message=%q", safe.Message())
	}
	details, ok := safe.Details().(map[string]string)
	if !ok || details["reasonCode"] != "ENTERPRISE_GROUP_DISCOVERY_FEISHU_SCOPE_REQUIRED" {
		t.Fatalf("details=%#v", safe.Details())
	}
	if details["managementUrl"] != "https://open.feishu.cn/app/app/auth" {
		t.Fatalf("managementUrl=%q", details["managementUrl"])
	}
}

func TestEnterpriseLoginRequiresIndependentGrantAndPreboundUser(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	identity := fixture.prebind()
	start, err := fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: "FEISHU", OrganizationCode: "acme"})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	_, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"})
	if err == nil {
		t.Fatal("login without LOGIN grant succeeded")
	}
	if fixture.bootstrap.called {
		t.Fatal("login without grant bootstrapped a session")
	}

	fixture.repo.grants[identity.ID] = append(fixture.repo.grants[identity.ID], activeEnterpriseGrant(identity, domain.EnterpriseGrantLogin))
	start, err = fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: "FEISHU", OrganizationCode: "acme"})
	if err != nil {
		t.Fatalf("restart login: %v", err)
	}
	result, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"})
	if err != nil {
		t.Fatalf("complete login: %v", err)
	}
	if !result.Authenticated || result.Outcome != "AUTHENTICATED" || !fixture.bootstrap.called {
		t.Fatalf("login result = %#v bootstrap=%v", result, fixture.bootstrap.called)
	}
}

func TestEnterpriseAuthorizationStateIsSingleUse(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatal(err)
	}
	callback := facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"}
	if _, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), callback); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), callback); err == nil {
		t.Fatal("replayed state succeeded")
	}
}

func TestEnterpriseLoginGrantRequiresEnabledVerifiedConnection(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	identity := fixture.prebind()
	connection := fixture.repo.connections[identity.ConnectionID]
	connection.LoginEnabled = false
	fixture.repo.connections[identity.ConnectionID] = connection
	proof := enterpriseProof(StepUpActionEnterpriseLoginAccess, fmt.Sprintf("enterprise-identity:%d|login:true|version:%d", identity.ID, identity.IdentityVersion))
	err := fixture.service.SetEnterpriseLoginAccess(context.Background(), 101, 999, identity.ID, facade.EnterpriseLoginAccessRequest{Enabled: true, ExpectedIdentityVersion: identity.IdentityVersion, Reason: "acceptance"}, proof)
	if err == nil || !strings.Contains(err.Error(), "请先在企业连接中开启并验证企业登录") {
		t.Fatalf("grant error = %v", err)
	}
	if hasActiveGrant(fixture.repo.grants[identity.ID], domain.EnterpriseGrantLogin) {
		t.Fatal("LOGIN grant was created while the connection login entry was disabled")
	}
}

func TestEnterpriseOAuthClosureForBothProviders(t *testing.T) {
	for _, provider := range []domain.EnterpriseProvider{domain.EnterpriseProviderFeishu, domain.EnterpriseProviderWeCom} {
		t.Run(string(provider), func(t *testing.T) {
			fixture := newEnterpriseFixtureForProvider(t, provider)
			start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: string(provider)})
			if err != nil {
				t.Fatalf("start binding: %v", err)
			}
			result, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: string(provider), State: stateFromRedirect(t, start.RedirectURL), Code: "code"})
			if err != nil {
				t.Fatalf("complete binding: %v", err)
			}
			if result.Authenticated || result.Outcome != "BOUND" {
				t.Fatalf("binding result=%#v", result)
			}
			identity := fixture.repo.identities[result.ExternalIdentityID]
			grants := fixture.repo.grants[identity.ID]
			for _, required := range []domain.EnterpriseGrantType{domain.EnterpriseGrantIdentityLink, domain.EnterpriseGrantNotificationDirect, domain.EnterpriseGrantNotificationMention} {
				if !hasActiveGrant(grants, required) {
					t.Fatalf("missing %s grant", required)
				}
			}
			if hasActiveGrant(grants, domain.EnterpriseGrantLogin) {
				t.Fatal("binding implicitly granted LOGIN")
			}
			ready, err := fixture.service.ResolveEnterpriseReadiness(context.Background(), 101, 201, string(provider))
			if err != nil || !ready.DirectReady || !ready.MentionReady {
				t.Fatalf("readiness=%#v err=%v", ready, err)
			}
			if err = fixture.service.SendMyEnterpriseTestMessage(context.Background(), 101, 201, identity.ID); err != nil {
				t.Fatalf("test message: %v", err)
			}

			proof := enterpriseProof(StepUpActionEnterpriseLoginAccess, fmt.Sprintf("enterprise-identity:%d|login:true|version:%d", identity.ID, identity.IdentityVersion))
			if err = fixture.service.SetEnterpriseLoginAccess(context.Background(), 101, 999, identity.ID, facade.EnterpriseLoginAccessRequest{Enabled: true, ExpectedIdentityVersion: identity.IdentityVersion, Reason: "acceptance"}, proof); err != nil {
				t.Fatalf("grant LOGIN: %v", err)
			}
			identity = fixture.repo.identities[identity.ID]
			login, err := fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: string(provider), OrganizationCode: "acme"})
			if err != nil {
				t.Fatalf("start login: %v", err)
			}
			loginResult, err := fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: string(provider), State: stateFromRedirect(t, login.RedirectURL), Code: "code"})
			if err != nil || !loginResult.Authenticated || !fixture.bootstrap.called {
				t.Fatalf("login=%#v bootstrap=%t err=%v", loginResult, fixture.bootstrap.called, err)
			}

			proof = enterpriseProof(StepUpActionEnterpriseLoginAccess, fmt.Sprintf("enterprise-identity:%d|login:false|version:%d", identity.ID, identity.IdentityVersion))
			if err = fixture.service.SetEnterpriseLoginAccess(context.Background(), 101, 999, identity.ID, facade.EnterpriseLoginAccessRequest{Enabled: false, ExpectedIdentityVersion: identity.IdentityVersion, Reason: "acceptance revoke"}, proof); err != nil {
				t.Fatalf("revoke LOGIN: %v", err)
			}
			if len(fixture.sessions.revoked) != 1 || fixture.sessions.revoked[0] != identity.ID {
				t.Fatalf("revoked sessions=%v", fixture.sessions.revoked)
			}
			if !hasActiveGrant(fixture.repo.grants[identity.ID], domain.EnterpriseGrantNotificationDirect) {
				t.Fatal("revoking LOGIN revoked DIRECT")
			}
		})
	}
}

func TestEnterpriseBindingRejectsConnectionDriftAndCrossUserSubject(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatal(err)
	}
	fixture.repo.connections[301] = func() domain.EnterpriseConnection { item := fixture.repo.connections[301]; item.Version++; return item }()
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"}); err == nil {
		t.Fatal("connection drift was accepted")
	}

	fixture = newEnterpriseFixture(t)
	foreign := fixture.prebind()
	foreign.UserID = 202
	fixture.repo.identities[foreign.ID] = foreign
	start, err = fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"}); err == nil {
		t.Fatal("subject already bound to another user was accepted")
	}
}

func TestEnterpriseAuthorizationOutcomeUnknownDoesNotCreateIdentityOrGrant(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.service.providers = enterpriseRegistry{adapter: enterpriseAdapter{provider: domain.EnterpriseProviderFeishu, exchangeErr: uncertainEnterpriseTestError{}}}
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "one-time-code"}); err == nil {
		t.Fatal("uncertain token exchange succeeded")
	}
	if len(fixture.repo.identities) != 0 || len(fixture.repo.grants) != 0 {
		t.Fatalf("uncertain exchange created identity/grants: %d/%d", len(fixture.repo.identities), len(fixture.repo.grants))
	}
	for _, authorization := range fixture.repo.authorizations {
		if authorization.Status != domain.EnterpriseAuthorizationOutcomeUnknown {
			t.Fatalf("authorization status=%s", authorization.Status)
		}
	}
}

func TestEnterpriseAuthorizationRejectsExternalRedirect(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	if _, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU", RedirectAfter: "https://evil.example/steal"}); err == nil {
		t.Fatal("absolute redirect was accepted")
	}
	if _, err := fixture.service.StartEnterpriseLogin(context.Background(), facade.EnterpriseLoginStartRequest{Provider: "FEISHU", OrganizationCode: "acme", RedirectAfter: "//evil.example"}); err == nil {
		t.Fatal("scheme-relative redirect was accepted")
	}
}

func TestEnterpriseAuthorizationRejectsProviderNonceMismatch(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	fixture.service.providers = enterpriseRegistry{adapter: enterpriseAdapter{provider: domain.EnterpriseProviderFeishu, supportsNonce: true, returnedNonce: "wrong-nonce"}}
	start, err := fixture.service.StartEnterpriseBinding(context.Background(), 101, 201, facade.EnterpriseAuthorizationStartRequest{Provider: "FEISHU"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.CompleteEnterpriseAuthorization(context.Background(), facade.EnterpriseAuthorizationCallbackRequest{Provider: "FEISHU", State: stateFromRedirect(t, start.RedirectURL), Code: "code"}); err == nil {
		t.Fatal("provider nonce mismatch was accepted")
	}
	if len(fixture.repo.identities) != 0 {
		t.Fatal("nonce mismatch created an identity")
	}
}

func TestEnterpriseLoginOptionsOnlyExposeVerifiedEnabledMethods(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	options, err := fixture.service.ResolveEnterpriseLoginOptions(context.Background(), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if options.OrganizationName != "Acme" || len(options.Methods) != 1 || options.Methods[0].Provider != "FEISHU" {
		t.Fatalf("options=%#v", options)
	}
	connection := fixture.repo.connections[301]
	connection.LoginEnabled = false
	fixture.repo.connections[301] = connection
	options, err = fixture.service.ResolveEnterpriseLoginOptions(context.Background(), "acme")
	if err != nil || len(options.Methods) != 0 {
		t.Fatalf("disabled options=%#v err=%v", options, err)
	}
}

func TestEnterpriseNotificationSettingsKeepDirectMentionAndLoginIndependent(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	identity := fixture.prebind()
	fixture.repo.grants[identity.ID] = append(fixture.repo.grants[identity.ID],
		activeEnterpriseGrant(identity, domain.EnterpriseGrantNotificationDirect),
		activeEnterpriseGrant(identity, domain.EnterpriseGrantNotificationMention),
		activeEnterpriseGrant(identity, domain.EnterpriseGrantLogin),
	)
	if err := fixture.service.UpdateMyEnterpriseNotificationSettings(context.Background(), 101, 201, identity.ID, facade.EnterpriseNotificationSettingsRequest{DirectEnabled: false, MentionEnabled: true, ExpectedIdentityVersion: identity.IdentityVersion}); err != nil {
		t.Fatal(err)
	}
	grants := fixture.repo.grants[identity.ID]
	if hasActiveGrant(grants, domain.EnterpriseGrantNotificationDirect) {
		t.Fatal("disabling personal messages left DIRECT active")
	}
	if !hasActiveGrant(grants, domain.EnterpriseGrantNotificationMention) || !hasActiveGrant(grants, domain.EnterpriseGrantLogin) {
		t.Fatal("changing personal messages changed Mention or Login")
	}
	if _, err := fixture.service.ResolveNotificationAuthority(context.Background(), facade.NotificationAuthorityRequest{OrganizationID: 101, UserID: 201, Provider: "FEISHU", GrantType: string(domain.EnterpriseGrantNotificationDirect)}); err == nil {
		t.Fatal("disabled DIRECT still resolved an authority")
	}
	if _, err := fixture.service.ResolveNotificationAuthority(context.Background(), facade.NotificationAuthorityRequest{OrganizationID: 101, UserID: 201, Provider: "FEISHU", GrantType: string(domain.EnterpriseGrantNotificationMention)}); err != nil {
		t.Fatalf("enabled MENTION authority: %v", err)
	}
}

func TestEnterpriseUnbindRevokesEveryGrantSessionAndStaleAuthority(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	identity := fixture.prebind()
	fixture.repo.grants[identity.ID] = append(fixture.repo.grants[identity.ID],
		activeEnterpriseGrant(identity, domain.EnterpriseGrantNotificationDirect),
		activeEnterpriseGrant(identity, domain.EnterpriseGrantNotificationMention),
		activeEnterpriseGrant(identity, domain.EnterpriseGrantLogin),
	)
	authority, err := fixture.service.ResolveNotificationAuthority(context.Background(), facade.NotificationAuthorityRequest{OrganizationID: 101, UserID: 201, Provider: "FEISHU", GrantType: string(domain.EnterpriseGrantNotificationDirect)})
	if err != nil {
		t.Fatal(err)
	}
	proof := enterpriseProof(StepUpActionEnterpriseIdentityUnbind, fmt.Sprintf("enterprise-identity:%d|unbind|version:%d", identity.ID, identity.IdentityVersion))
	if err = fixture.service.UnbindMyEnterpriseIdentity(context.Background(), 101, 201, identity.ID, facade.EnterpriseUnbindRequest{ExpectedIdentityVersion: identity.IdentityVersion}, proof); err != nil {
		t.Fatal(err)
	}
	for _, grant := range fixture.repo.grants[identity.ID] {
		if grant.Status != domain.EnterpriseGrantRevoked {
			t.Fatalf("grant %s remained %s", grant.GrantType, grant.Status)
		}
	}
	if got := fixture.repo.identities[identity.ID]; got.Status != domain.EnterpriseIdentityRevoked {
		t.Fatalf("identity status=%s", got.Status)
	}
	if len(fixture.sessions.revoked) != 1 || fixture.sessions.revoked[0] != identity.ID {
		t.Fatalf("revoked sessions=%v", fixture.sessions.revoked)
	}
	snapshot := facade.NotificationAuthoritySnapshot{OrganizationID: authority.OrganizationID, UserID: authority.UserID, ExternalIdentityID: authority.ExternalIdentityID, IdentityVersion: authority.IdentityVersion, EnterpriseConnectionID: authority.EnterpriseConnectionID, ConnectionVersion: authority.ConnectionVersion, AuthorityDigest: authority.AuthorityDigest, GrantType: authority.GrantType}
	if err = fixture.service.ValidateNotificationAuthority(context.Background(), snapshot); err == nil {
		t.Fatal("authority frozen before unbind remained valid")
	}
}

func TestEnterpriseSelfServiceRejectsForeignOrganizationAndUser(t *testing.T) {
	fixture := newEnterpriseFixture(t)
	identity := fixture.prebind()
	request := facade.EnterpriseNotificationSettingsRequest{DirectEnabled: true, MentionEnabled: true, ExpectedIdentityVersion: identity.IdentityVersion}
	if err := fixture.service.UpdateMyEnterpriseNotificationSettings(context.Background(), 102, 201, identity.ID, request); err == nil {
		t.Fatal("cross-organization settings mutation succeeded")
	}
	if err := fixture.service.UpdateMyEnterpriseNotificationSettings(context.Background(), 101, 202, identity.ID, request); err == nil {
		t.Fatal("another user changed the binding")
	}
}

type uncertainEnterpriseTestError struct{}

func (uncertainEnterpriseTestError) Error() string        { return "provider outcome unknown" }
func (uncertainEnterpriseTestError) OutcomeUnknown() bool { return true }

type enterpriseRouteProvisionerFunc func(context.Context, int64, int64, int64, string, string, bool, int64) error

func (f enterpriseRouteProvisionerFunc) EnsureEnterpriseDirectRoute(ctx context.Context, organizationID, connectionID, connectionVersion int64, connectionDigest, provider string, enabled bool, actorID int64) error {
	return f(ctx, organizationID, connectionID, connectionVersion, connectionDigest, provider, enabled, actorID)
}

func enterpriseProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{BusinessAction: action, OperationBinding: binding, ProofIdentifier: "proof", ChallengeIdentifier: "challenge", AssuranceLevel: "AAL2", AuthenticationMethods: []string{"TOTP"}}
}

type enterpriseFixture struct {
	service   *EnterpriseService
	repo      *enterpriseMemoryRepo
	bootstrap *enterpriseBootstrap
	sessions  *enterpriseSessions
}

func newEnterpriseFixture(t *testing.T) *enterpriseFixture {
	return newEnterpriseFixtureForProvider(t, domain.EnterpriseProviderFeishu)
}

func newEnterpriseFixtureForProvider(t *testing.T, provider domain.EnterpriseProvider) *enterpriseFixture {
	t.Helper()
	gen, err := xid.New(7)
	if err != nil {
		t.Fatal(err)
	}
	repo := &enterpriseMemoryRepo{connections: map[int64]domain.EnterpriseConnection{}, authorizations: map[string]*domain.EnterpriseAuthorization{}, identities: map[int64]domain.EnterpriseExternalIdentity{}, grants: map[int64][]domain.EnterpriseIdentityGrant{}}
	connection := domain.EnterpriseConnection{ID: 301, OrganizationID: 101, Provider: provider, AppID: "app", CorpID: "corp", AgentID: "agent", ClientSecretCiphertext: "secret", ClientSecretEDEK: "edek", ClientSecretWrapKeyRef: "kid", CredentialVersion: 1, BindingEnabled: true, NotificationDirectEnabled: true, NotificationMentionEnabled: true, LoginEnabled: true, ConfigDigest: "cfg", Status: domain.EnterpriseConnectionActive, LastProbeStatus: "PASSED", Version: 1}
	repo.connections[connection.ID] = connection
	bootstrap := &enterpriseBootstrap{}
	sessions := &enterpriseSessions{}
	service := NewEnterpriseService(EnterpriseServiceDeps{Config: "https://console.example", IDGen: gen, Repository: repo, SecretValue: enterpriseSecrets{}, Random: &enterpriseRandom{}, Digester: enterpriseDigester{}, Providers: enterpriseRegistry{adapter: enterpriseAdapter{provider: provider}}, Subjects: enterpriseSubjects{}, Relations: enterpriseRelations{}, Orgs: enterpriseOrgs{}, Bootstrap: bootstrap, Sessions: sessions, EnterpriseLoginEnabled: true})
	return &enterpriseFixture{service: service, repo: repo, bootstrap: bootstrap, sessions: sessions}
}

func (f *enterpriseFixture) prebind() domain.EnterpriseExternalIdentity {
	identity := domain.EnterpriseExternalIdentity{ID: 401, OrganizationID: 101, ConnectionID: 301, Provider: domain.EnterpriseProviderFeishu, UserID: 201, SubjectKind: "FEISHU_OPEN_ID", SubjectCiphertext: "subject", SubjectEDEK: "edek", SubjectWrapKeyRef: "kid", SubjectDigest: "digest:subject", Status: domain.EnterpriseIdentityActive, IdentityVersion: 1, VerifiedAt: time.Now()}
	f.repo.identities[identity.ID] = identity
	f.repo.grants[identity.ID] = []domain.EnterpriseIdentityGrant{activeEnterpriseGrant(identity, domain.EnterpriseGrantIdentityLink)}
	return identity
}

func activeEnterpriseGrant(identity domain.EnterpriseExternalIdentity, kind domain.EnterpriseGrantType) domain.EnterpriseIdentityGrant {
	return domain.EnterpriseIdentityGrant{ID: time.Now().UnixNano(), OrganizationID: identity.OrganizationID, ExternalIdentityID: identity.ID, GrantType: kind, Status: domain.EnterpriseGrantActive, Version: 1}
}
func stateFromRedirect(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.Query().Get("state")
}

type enterpriseMemoryRepo struct {
	connections       map[int64]domain.EnterpriseConnection
	authorizations    map[string]*domain.EnterpriseAuthorization
	identities        map[int64]domain.EnterpriseExternalIdentity
	grants            map[int64][]domain.EnterpriseIdentityGrant
	insertIdentityErr error
}

func (r *enterpriseMemoryRepo) ListEnterpriseConnections(_ context.Context, org int64) ([]domain.EnterpriseConnection, error) {
	items := make([]domain.EnterpriseConnection, 0, len(r.connections))
	for _, item := range r.connections {
		if item.OrganizationID == org {
			items = append(items, item)
		}
	}
	return items, nil
}
func (r *enterpriseMemoryRepo) FindEnterpriseConnection(_ context.Context, org int64, provider domain.EnterpriseProvider) (*domain.EnterpriseConnection, error) {
	for _, item := range r.connections {
		if item.OrganizationID == org && item.Provider == provider {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *enterpriseMemoryRepo) FindEnterpriseConnectionByID(_ context.Context, org, id int64) (*domain.EnterpriseConnection, error) {
	item, ok := r.connections[id]
	if !ok || item.OrganizationID != org {
		return nil, nil
	}
	copy := item
	return &copy, nil
}
func (r *enterpriseMemoryRepo) InsertEnterpriseConnection(context.Context, domain.EnterpriseConnection) error {
	return nil
}
func (r *enterpriseMemoryRepo) UpdateEnterpriseConnection(context.Context, domain.EnterpriseConnection, int64) (bool, error) {
	return true, nil
}
func (r *enterpriseMemoryRepo) InsertEnterpriseAuthorization(_ context.Context, item domain.EnterpriseAuthorization) error {
	copy := item
	r.authorizations[item.StateHash] = &copy
	return nil
}
func (r *enterpriseMemoryRepo) ConsumeEnterpriseAuthorization(_ context.Context, state string, now time.Time) (*domain.EnterpriseAuthorization, error) {
	item := r.authorizations[state]
	if item == nil || item.Status != domain.EnterpriseAuthorizationActive || !item.ExpiresAt.After(now) {
		return nil, nil
	}
	copy := *item
	item.Status = domain.EnterpriseAuthorizationProcessing
	copy.Status = item.Status
	return &copy, nil
}
func (r *enterpriseMemoryRepo) CompleteEnterpriseAuthorization(_ context.Context, id int64, status domain.EnterpriseAuthorizationStatus, identityID *int64, reason string, now time.Time) error {
	for _, item := range r.authorizations {
		if item.ID == id {
			if item.Status != domain.EnterpriseAuthorizationProcessing {
				return nil
			}
			item.Status = status
			item.ResultIdentityID = identityID
			item.ReasonCode = reason
			item.UpdatedAt = now
			return nil
		}
	}
	return nil
}
func (r *enterpriseMemoryRepo) FindEnterpriseIdentityByDigest(_ context.Context, org, conn int64, digest string) (*domain.EnterpriseExternalIdentity, error) {
	for _, item := range r.identities {
		if item.OrganizationID == org && item.ConnectionID == conn && item.SubjectDigest == digest {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *enterpriseMemoryRepo) FindEnterpriseIdentityByUser(_ context.Context, org, conn, user int64) (*domain.EnterpriseExternalIdentity, error) {
	for _, item := range r.identities {
		if item.OrganizationID == org && item.ConnectionID == conn && item.UserID == user {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}
func (r *enterpriseMemoryRepo) FindEnterpriseIdentityByID(_ context.Context, org, id int64) (*domain.EnterpriseExternalIdentity, error) {
	item, ok := r.identities[id]
	if !ok || item.OrganizationID != org {
		return nil, nil
	}
	copy := item
	return &copy, nil
}
func (r *enterpriseMemoryRepo) ListEnterpriseIdentitiesByUser(context.Context, int64, int64) ([]domain.EnterpriseExternalIdentity, error) {
	items := make([]domain.EnterpriseExternalIdentity, 0, len(r.identities))
	for _, item := range r.identities {
		items = append(items, item)
	}
	return items, nil
}
func (r *enterpriseMemoryRepo) ListEnterpriseIdentitiesByOrganization(context.Context, int64) ([]domain.EnterpriseExternalIdentity, error) {
	return nil, nil
}
func (r *enterpriseMemoryRepo) InsertEnterpriseIdentity(_ context.Context, item domain.EnterpriseExternalIdentity) error {
	if r.insertIdentityErr != nil {
		return r.insertIdentityErr
	}
	r.identities[item.ID] = item
	return nil
}
func (r *enterpriseMemoryRepo) UpdateEnterpriseIdentityStatus(_ context.Context, org, id, expected, _ int64, status domain.EnterpriseIdentityStatus, _ time.Time) (bool, error) {
	item, ok := r.identities[id]
	if !ok || item.OrganizationID != org || item.IdentityVersion != expected {
		return false, nil
	}
	item.Status = status
	item.IdentityVersion++
	r.identities[id] = item
	return true, nil
}
func (r *enterpriseMemoryRepo) AdvanceEnterpriseIdentityVersion(_ context.Context, org, id, expected, _ int64, _ time.Time) (bool, error) {
	item, ok := r.identities[id]
	if !ok || item.OrganizationID != org || item.IdentityVersion != expected {
		return false, nil
	}
	item.IdentityVersion++
	r.identities[id] = item
	return true, nil
}
func (r *enterpriseMemoryRepo) ListEnterpriseGrants(_ context.Context, _ int64, id int64) ([]domain.EnterpriseIdentityGrant, error) {
	return append([]domain.EnterpriseIdentityGrant(nil), r.grants[id]...), nil
}
func (r *enterpriseMemoryRepo) UpsertEnterpriseGrant(_ context.Context, item domain.EnterpriseIdentityGrant) error {
	items := r.grants[item.ExternalIdentityID]
	for i := range items {
		if items[i].GrantType == item.GrantType {
			items[i] = item
			r.grants[item.ExternalIdentityID] = items
			return nil
		}
	}
	r.grants[item.ExternalIdentityID] = append(items, item)
	return nil
}

type enterpriseSecrets struct{}

func (enterpriseSecrets) EncryptString(_ context.Context, v string) (EncryptedSecretValue, error) {
	return EncryptedSecretValue{CiphertextB64: v, EDEKB64: "edek", WrapKeyRef: "kid"}, nil
}
func (enterpriseSecrets) DecryptString(_ context.Context, v EncryptedSecretValue) (string, error) {
	return v.CiphertextB64, nil
}
func (enterpriseSecrets) EncryptBytes(_ context.Context, v []byte) (EncryptedSecretValue, error) {
	return enterpriseSecrets{}.EncryptString(context.Background(), string(v))
}
func (enterpriseSecrets) DecryptBytes(_ context.Context, v EncryptedSecretValue) ([]byte, error) {
	return []byte(v.CiphertextB64), nil
}

type enterpriseRandom struct{ n int }

func (r *enterpriseRandom) Token(context.Context) (string, error) {
	r.n++
	return fmt.Sprintf("token-%d", r.n), nil
}

type enterpriseDigester struct{}

func (enterpriseDigester) Digest(_ context.Context, _ string, _ int64, _ int64, _ domain.EnterpriseProvider, _ string, subject string) (string, string, error) {
	return "digest:" + subject, "kid", nil
}

type enterpriseAdapter struct {
	provider          domain.EnterpriseProvider
	exchangeErr       error
	groupDiscoveryErr error
	supportsNonce     bool
	returnedNonce     string
}

func (a enterpriseAdapter) Provider() domain.EnterpriseProvider {
	if a.provider == "" {
		return domain.EnterpriseProviderFeishu
	}
	return a.provider
}
func (a enterpriseAdapter) Capabilities() domain.EnterpriseProtocolCapabilities {
	return domain.EnterpriseProtocolCapabilities{SupportsAuthorizationCode: true, SupportsUserInfo: true, SupportsOIDCNonce: a.supportsNonce}
}
func (enterpriseAdapter) BuildAuthorizationURL(_ domain.EnterpriseConnection, state, _, _, _ string) (string, error) {
	return "https://fake.example/authorize?state=" + url.QueryEscape(state), nil
}
func (a enterpriseAdapter) ExchangeAndResolve(context.Context, domain.EnterpriseConnection, string, string, string, string) (EnterpriseResolvedSubject, error) {
	if a.exchangeErr != nil {
		return EnterpriseResolvedSubject{}, a.exchangeErr
	}
	return EnterpriseResolvedSubject{Subject: "subject", SubjectKind: domain.SubjectKindForProvider(a.Provider()), Nonce: a.returnedNonce}, nil
}
func (enterpriseAdapter) Probe(context.Context, domain.EnterpriseConnection, string) error {
	return nil
}
func (enterpriseAdapter) SendPersonalTest(context.Context, domain.EnterpriseConnection, string, string) error {
	return nil
}

func (a enterpriseAdapter) DiscoverGroups(context.Context, domain.EnterpriseConnection, string) ([]EnterpriseDiscoveredGroup, error) {
	if a.groupDiscoveryErr != nil {
		return nil, a.groupDiscoveryErr
	}
	return []EnterpriseDiscoveredGroup{{DisplayName: "研发群", Target: "oc_test"}}, nil
}

type testGroupDiscoveryRejection struct{}

func (testGroupDiscoveryRejection) Error() string { return "provider detail must stay private" }
func (testGroupDiscoveryRejection) GroupDiscoveryRejection() (string, string) {
	return "ENTERPRISE_GROUP_DISCOVERY_FEISHU_SCOPE_REQUIRED", "飞书应用缺少群信息读取权限，请开通 im:chat:readonly 并发布新版本后重试"
}

type enterpriseRegistry struct{ adapter EnterpriseProviderAdapter }

func (r enterpriseRegistry) GetEnterprise(provider domain.EnterpriseProvider) (EnterpriseProviderAdapter, bool) {
	return r.adapter, r.adapter.Provider() == provider
}

type enterpriseOrgs struct{ orgByID *userfacade.OrgVO }

func (enterpriseOrgs) GetOrgByCode(context.Context, string) (*userfacade.OrgVO, error) {
	return &userfacade.OrgVO{ID: 101, Code: "acme", Name: "Acme", Status: 0}, nil
}
func (enterpriseOrgs) CreateOrg(context.Context, userfacade.OrgCommand) error { return nil }
func (enterpriseOrgs) UpdateOrg(context.Context, userfacade.OrgCommand) error { return nil }
func (enterpriseOrgs) DeleteOrg(context.Context, int64) error                 { return nil }
func (o enterpriseOrgs) GetOrgByID(_ context.Context, id int64) (*userfacade.OrgVO, error) {
	if o.orgByID != nil && o.orgByID.ID == id {
		copy := *o.orgByID
		return &copy, nil
	}
	return &userfacade.OrgVO{ID: id, Code: "acme", Name: "Acme", Status: 0}, nil
}
func (enterpriseOrgs) GetOrgByUserID(context.Context, int64) (*userfacade.OrgVO, error) {
	return nil, nil
}
func (enterpriseOrgs) GetOrgTree(context.Context) ([]userfacade.OrgVO, error)     { return nil, nil }
func (enterpriseOrgs) ListActiveOrgs(context.Context) ([]userfacade.OrgVO, error) { return nil, nil }
func (enterpriseOrgs) ListOrgChildren(context.Context, int64) ([]userfacade.OrgVO, error) {
	return nil, nil
}
func (enterpriseOrgs) CheckOrgCode(context.Context, string, int64) (bool, error) { return false, nil }
func (enterpriseOrgs) ChangeOrgStatus(context.Context, int64, int, int64) error  { return nil }
func (enterpriseOrgs) MoveOrg(context.Context, int64, int64, int64) error        { return nil }

type enterpriseSubjects struct{}

func (enterpriseSubjects) FindSubjectByID(context.Context, int64) (*userfacade.SubjectRecord, error) {
	return &userfacade.SubjectRecord{UserID: 201, Enabled: true}, nil
}
func (enterpriseSubjects) FindSubjectByAccount(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (enterpriseSubjects) FindSubjectByEmail(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (enterpriseSubjects) CreateExternalSubject(context.Context, userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (enterpriseSubjects) CreateFormSubject(context.Context, userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (enterpriseSubjects) ExistsByID(context.Context, int64) (bool, error) { return true, nil }
func (enterpriseSubjects) BuildPrincipalSeed(context.Context, int64) (*userfacade.UserPrincipalSeed, error) {
	return &userfacade.UserPrincipalSeed{UserID: 201, Enabled: true}, nil
}

type enterpriseRelations struct{}

func (enterpriseRelations) ListUserRoleIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (enterpriseRelations) AssignUserRoles(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (enterpriseRelations) ListUserOrgIDs(context.Context, int64) ([]int64, error) {
	return []int64{101}, nil
}
func (enterpriseRelations) AssignUserOrgs(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (enterpriseRelations) ListUserDeptIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (enterpriseRelations) AssignUserDepts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (enterpriseRelations) ListUserPostIDs(context.Context, int64) ([]int64, error) { return nil, nil }
func (enterpriseRelations) AssignUserPosts(context.Context, userfacade.RelationAssignCommand) error {
	return nil
}
func (enterpriseRelations) ListActiveUserIDsByRoleID(context.Context, int64) ([]int64, error) {
	return nil, nil
}

type enterpriseBootstrap struct{ called bool }

func (b *enterpriseBootstrap) BootstrapFirstPartySession(context.Context, ssofacade.BootstrapSessionCommand) (*ssofacade.BootstrapSessionResult, error) {
	b.called = true
	return &ssofacade.BootstrapSessionResult{SessionCookieHeaderValue: "session", RefreshCookieHeaderValue: "refresh"}, nil
}

type enterpriseSessions struct {
	revoked []int64
}

func (s *enterpriseSessions) RevokeSessionsByExternalIdentity(_ context.Context, identityID int64) (int64, error) {
	s.revoked = append(s.revoked, identityID)
	return 1, nil
}
