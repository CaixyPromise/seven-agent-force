package application

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

type fakeSystemSubjectDigester struct{}

func (fakeSystemSubjectDigester) DigestSystemSubject(_ context.Context, _ string, providerCode, issuer, subject string) (string, string, error) {
	return "protected:" + providerCode + ":" + issuer + ":" + subject, "digest-key", nil
}

func TestListLoginMethodsReturnsOnlyDisplayableEnabledProviders(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.providers["google"] = activeProvider("google")
	fixture.repo.providers["hidden"] = activeProvider("hidden")
	fixture.repo.providers["hidden"].DisplayEnabled = false
	fixture.repo.providers["disabled"] = activeProvider("disabled")
	fixture.repo.providers["disabled"].Status = domain.ProviderStatusDisabled

	methods, err := fixture.service.ListLoginMethods(context.Background(), facade.ListLoginMethodsRequest{})
	if err != nil {
		t.Fatalf("list methods: %v", err)
	}
	if len(methods) != 2 {
		t.Fatalf("methods=%#v, want two displayable active providers", methods)
	}
	if methods[0].ProviderCode != "github" || methods[1].ProviderCode != "google" {
		t.Fatalf("unexpected methods order/content: %#v", methods)
	}
	if !strings.Contains(methods[0].LoginURL, "/login/external/github/start") {
		t.Fatalf("login URL should point at provider start route: %#v", methods[0])
	}
}

func TestStartExternalLoginRejectsDisabledProvider(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.Status = domain.ProviderStatusDisabled
	fixture.repo.providers["github"] = provider

	_, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err == nil {
		t.Fatal("expected disabled provider to reject start")
	}
	if len(fixture.repo.loginStates) != 0 {
		t.Fatalf("disabled provider stored login state: %#v", fixture.repo.loginStates)
	}
}

func TestStartExternalLoginDoesNotPersistPlaintextOAuthState(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")

	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	stateHash := hashString(start.StateID)
	persisted := fixture.repo.loginStates[stateHash]
	if persisted == nil {
		t.Fatalf("login state not persisted by hash %s", stateHash)
	}
	if persisted.StateID == start.StateID {
		t.Fatalf("plaintext OAuth state was persisted as stateId")
	}
	if _, ok := fixture.cache.states[start.StateID]; ok {
		t.Fatalf("plaintext OAuth state was used as cache key")
	}
	if cached := fixture.cache.states[persisted.StateID]; cached.StateHash != stateHash {
		t.Fatalf("cache should store state shadow under non-secret state id: %#v", fixture.cache.states)
	}
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	if _, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	}); err != nil {
		t.Fatalf("callback should still work with plaintext state from browser: %v", err)
	}
}

func TestStartExternalLoginFailsClosedWhenPlatformFacadeMissing(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.service.platform = nil

	_, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode:       "github",
		LoginContextID:     "plctx-missing-platform",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	})
	if err == nil {
		t.Fatal("expected missing platform facade to fail closed")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeSystemError {
		t.Fatalf("error code = %d, want system error", got)
	}
	if len(fixture.repo.loginStates) != 0 {
		t.Fatalf("missing platform facade stored login state: %#v", fixture.repo.loginStates)
	}
}

func TestCompleteExternalCallbackRejectsDisabledProviderAfterStart(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	fixture.repo.providers["github"].Status = domain.ProviderStatusDisabled

	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	if err == nil {
		t.Fatal("expected callback to fail closed after provider disable")
	}
	if fixture.driver.exchangeCount != 0 {
		t.Fatalf("disabled callback exchanged code: %d", fixture.driver.exchangeCount)
	}
}

func TestCompleteExternalCallbackRejectsManagedProviderDisabledDuringExchange(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("hub:node-a")
	provider.ProtocolType = domain.ProtocolTypeOIDC
	provider.Issuer = "https://hub.example"
	provider.MetadataJSON = `{"managedBy":"hub_control","ownerNodeCode":"node-a","connectionVersion":"v1","connectionHash":"hash-1","targetRevision":1}`
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.repo.identities[fakeIdentityKey(provider.ProviderCode, provider.Issuer, "subject-1")] = &domain.ExternalIdentity{ID: 77, ProviderCode: provider.ProviderCode, ExternalIssuer: provider.Issuer, ExternalSubject: "subject-1", UserID: 101, Status: domain.IdentityStatusActive}
	fixture.drivers.drivers[provider.ProviderCode] = fixture.driver
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	fixture.driver.exchangeStarted = exchangeStarted
	fixture.driver.releaseExchange = releaseExchange
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest(provider.ProviderCode))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, callbackErr := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID})
		done <- callbackErr
	}()
	<-exchangeStarted
	provider.Status = domain.ProviderStatusDisabled
	provider.LoginEnabled = false
	close(releaseExchange)
	if err := <-done; err == nil {
		t.Fatal("callback completed after managed provider disable")
	}
	if fixture.sso.bootstrapCount != 0 {
		t.Fatalf("bootstrapped %d sessions after disable", fixture.sso.bootstrapCount)
	}
}

func TestCurrentUserBindingRejectsManagedProviderDisabledDuringExchange(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("hub:node-a")
	provider.ProtocolType = domain.ProtocolTypeOIDC
	provider.Issuer = "https://hub.example"
	provider.MetadataJSON = `{"managedBy":"hub_control","ownerNodeCode":"node-a","connectionVersion":"v1","connectionHash":"hash-1","targetRevision":1}`
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.drivers.drivers[provider.ProviderCode] = fixture.driver
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	fixture.driver.exchangeStarted = exchangeStarted
	fixture.driver.releaseExchange = releaseExchange
	req := platformStartRequest(provider.ProviderCode)
	req.BindUserID = 101
	start, err := fixture.service.StartExternalLogin(context.Background(), req)
	if err != nil {
		t.Fatalf("start binding: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, callbackErr := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID})
		done <- callbackErr
	}()
	<-exchangeStarted
	provider.Status = domain.ProviderStatusDisabled
	provider.BindEnabled = false
	close(releaseExchange)
	if err := <-done; err == nil {
		t.Fatal("binding completed after managed provider disable")
	}
	if got := fixture.repo.identities[fakeIdentityKey(provider.ProviderCode, provider.Issuer, "subject-1")]; got != nil {
		t.Fatalf("identity persisted after disable: %#v", got)
	}
}

func TestCurrentUserBindingProtectsSystemProviderSubject(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.service.identityDigester = fakeSystemSubjectDigester{}
	provider := activeProvider("github")
	fixture.repo.providers[provider.ProviderCode] = provider
	req := platformStartRequest(provider.ProviderCode)
	req.BindUserID = 101
	start, err := fixture.service.StartExternalLogin(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID})
	if err != nil {
		t.Fatal(err)
	}
	identity := fixture.repo.identitiesByID[result.ExternalIdentityID]
	if identity == nil || identity.ExternalSubject == "subject-1" || identity.SubjectDigest != "protected:github::subject-1" || identity.SubjectCiphertext == "" || identity.SubjectDigestKeyRef != "digest-key" {
		t.Fatalf("identity subject was not protected: %#v", identity)
	}
	if _, err = fixture.service.ResolveIdentity(context.Background(), "github", "subject-1"); err != nil {
		t.Fatalf("protected subject lookup: %v", err)
	}
}

func TestCompleteExternalCallbackRejectsManagedProviderRevisionChangedDuringExchange(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("hub:node-a")
	provider.ProtocolType = domain.ProtocolTypeOIDC
	provider.Issuer = "https://hub.example"
	provider.MetadataJSON = `{"managedBy":"hub_control","ownerNodeCode":"node-a","connectionVersion":"v1","connectionHash":"hash-1","targetRevision":1}`
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.repo.identities[fakeIdentityKey(provider.ProviderCode, provider.Issuer, "subject-1")] = &domain.ExternalIdentity{ID: 78, ProviderCode: provider.ProviderCode, ExternalIssuer: provider.Issuer, ExternalSubject: "subject-1", UserID: 101, Status: domain.IdentityStatusActive}
	fixture.drivers.drivers[provider.ProviderCode] = fixture.driver
	exchangeStarted := make(chan struct{})
	releaseExchange := make(chan struct{})
	fixture.driver.exchangeStarted = exchangeStarted
	fixture.driver.releaseExchange = releaseExchange
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest(provider.ProviderCode))
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, callbackErr := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID})
		done <- callbackErr
	}()
	<-exchangeStarted
	provider.MetadataJSON = `{"managedBy":"hub_control","ownerNodeCode":"node-a","connectionVersion":"v2","connectionHash":"hash-2","targetRevision":2}`
	close(releaseExchange)
	if err := <-done; err == nil {
		t.Fatal("callback completed after managed provider revision changed")
	}
	if fixture.sso.bootstrapCount != 0 {
		t.Fatalf("bootstrapped %d sessions after revision change", fixture.sso.bootstrapCount)
	}
}

func TestCompleteExternalCallbackRejectsDisabledPlatformMethodAfterStart(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	platform := &fakePlatformFacade{platformCode: "seven-admin"}
	fixture.service.platform = platform
	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode:       "github",
		LoginContextID:     "plctx-method",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	platform.loginMethodErr = apperrors.ObjectState("平台登录方式已禁用")

	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	if err == nil {
		t.Fatal("expected callback to fail closed after platform method disable")
	}
	if fixture.driver.exchangeCount != 0 {
		t.Fatalf("disabled platform method exchanged code: %d", fixture.driver.exchangeCount)
	}
	if platform.requireLoginMethodCalls < 2 {
		t.Fatalf("expected start and callback to both check platform method, calls=%d", platform.requireLoginMethodCalls)
	}
}

func TestCompleteExternalCallbackRejectsReplayState(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	if _, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code-1",
		State:        start.StateID,
	}); err != nil {
		t.Fatalf("first callback: %v", err)
	}

	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code-2",
		State:        start.StateID,
	})
	if err == nil {
		t.Fatal("expected replayed state to be rejected")
	}
	if fixture.driver.exchangeCount != 1 {
		t.Fatalf("replay should not exchange code again, exchanges=%d", fixture.driver.exchangeCount)
	}
}

func TestCompleteExternalCallbackCompletesSSOTransactionForBoundIdentity(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	startReq := platformStartRequest("github")
	startReq.LoginTransactionID = "ltx-1"
	start, err := fixture.service.StartExternalLogin(context.Background(), startReq)
	if err != nil {
		t.Fatalf("start login: %v", err)
	}

	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if !result.Authenticated || result.LoginTransactionID != "ltx-1" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if fixture.sso.completeCount != 1 || fixture.sso.bootstrapCount != 0 {
		t.Fatalf("expected SSO completion only, complete=%d bootstrap=%d", fixture.sso.completeCount, fixture.sso.bootstrapCount)
	}
	cmd := fixture.sso.lastComplete
	if cmd.LoginMethod != "EXTERNAL_OAUTH" || cmd.ExternalProviderCode != "github" || cmd.ExternalIdentityID != 9001 {
		t.Fatalf("missing external login session metadata: %#v", cmd)
	}
	if len(cmd.AMR) != 2 || cmd.AMR[0] != "oauth" || cmd.AMR[1] != "oauth:github" {
		t.Fatalf("unexpected amr: %#v", cmd.AMR)
	}
}

func TestCompleteExternalCallbackBootstrapsFirstPartySessionWithoutTransaction(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}

	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	if !result.Authenticated || result.AccessToken != "access-local" || result.RefreshCookieHeaderValue == "" {
		t.Fatalf("unexpected bootstrap result: %#v", result)
	}
	if fixture.sso.bootstrapCount != 1 || fixture.sso.completeCount != 0 {
		t.Fatalf("expected bootstrap only, complete=%d bootstrap=%d", fixture.sso.completeCount, fixture.sso.bootstrapCount)
	}
}

func TestCompleteExternalCallbackSyncsExternalProfileToLocalUser(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	fixture.driver.profile = domain.ExternalProfile{
		Subject:       "subject-1",
		Login:         "octocat",
		Email:         "octocat@example.com",
		EmailVerified: true,
		DisplayName:   "Octo Cat",
		AvatarURL:     "https://avatars.example.com/octocat.png",
		RawProfile:    `{"login":"octocat"}`,
	}
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}

	if _, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	synced := fixture.profiles.lastSync
	if synced == nil {
		t.Fatal("expected local profile sync to be called")
	}
	if synced.UserID != 101 || synced.ProviderCode != "github" || synced.NickName != "Octo Cat" || synced.UserEmail != "octocat@example.com" || synced.UserAvatar == "" {
		t.Fatalf("unexpected synced profile: %#v", synced)
	}
}

func TestCompleteExternalCallbackPassesExpectedNonceToDriver(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.ProtocolType = domain.ProtocolTypeOIDC
	fixture.repo.providers["github"] = provider
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	fixture.driver.requireNonce = true
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}

	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	if err != nil {
		t.Fatalf("callback should pass encrypted nonce to driver: %v", err)
	}
	if fixture.driver.lastExchangeNonce == "" {
		t.Fatal("driver did not receive callback nonce")
	}
	if fixture.driver.lastExchangeNonce != fixture.driver.lastAuthorizationNonce {
		t.Fatalf("callback nonce=%q, want authorization nonce=%q", fixture.driver.lastExchangeNonce, fixture.driver.lastAuthorizationNonce)
	}
}

func TestCompleteExternalCallbackWrapsProviderProfileFailureAsOperationError(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.driver.profileErr = errors.New("resolve github emails: upstream timeout")

	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: "github",
		Code:         "code",
		State:        start.StateID,
	})
	appErr := apperrors.From(err)
	if appErr.Code() != apperrors.CodeOperateError || !strings.Contains(appErr.Message(), "获取GitHub账号资料失败") {
		t.Fatalf("expected operation error for provider profile failure, got %#v from %v", appErr, err)
	}
}

func TestCompleteExternalCallbackAutoBindsOnlyVerifiedEmailWhenEnabled(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.EmailAutoBindEnabled = true
	fixture.repo.providers["github"] = provider
	fixture.subjects.byEmail["alice@example.com"] = &userfacade.SubjectRecord{UserID: 202, AccountName: "alice", Email: "alice@example.com", Enabled: true}

	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	fixture.driver.profile = domain.ExternalProfile{Subject: "new-subject", Email: "alice@example.com", EmailVerified: false}
	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID})
	if err == nil || !strings.Contains(err.Error(), "外部账号未绑定") {
		t.Fatalf("expected unverified email to reject auto bind, got %v", err)
	}

	start, err = fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login again: %v", err)
	}
	fixture.driver.profile = domain.ExternalProfile{Subject: "new-subject", Email: "alice@example.com", EmailVerified: true, Login: "alice-gh"}
	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID})
	if err != nil {
		t.Fatalf("verified callback: %v", err)
	}
	if result.UserID != 202 {
		t.Fatalf("auto-bound wrong user: %#v", result)
	}
	identity := fixture.repo.identities["github|new-subject"]
	if identity == nil || identity.UserID != 202 || identity.Status != domain.IdentityStatusActive {
		t.Fatalf("expected active auto-bound identity: %#v", identity)
	}
}

func TestCompleteExternalCallbackAutoCreatesUserWhenEnabled(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.EmailAutoBindEnabled = true
	provider.AccountAutoCreateEnabled = true
	fixture.repo.providers["github"] = provider
	fixture.subjects.createdUserID = 303

	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	fixture.driver.profile = domain.ExternalProfile{
		Subject:       "new-subject",
		Email:         "new-user@example.com",
		EmailVerified: true,
		Login:         "new-user",
		DisplayName:   "New User",
	}
	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID})
	if err != nil {
		t.Fatalf("verified callback: %v", err)
	}
	if result.UserID != 303 {
		t.Fatalf("auto-created wrong user: %#v", result)
	}
	identity := fixture.repo.identities["github|new-subject"]
	if identity == nil || identity.UserID != 303 || identity.ExternalEmail != "new-user@example.com" || identity.Status != domain.IdentityStatusActive {
		t.Fatalf("expected active identity for auto-created user: %#v", identity)
	}
}

func TestCompleteExternalCallbackPassesPlatformDefaultRBACToProvisioning(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.EmailAutoBindEnabled = true
	provider.AccountAutoCreateEnabled = true
	fixture.repo.providers["github"] = provider
	fixture.subjects.createdUserID = 303
	defaultOrgID := int64(41)
	defaultDeptID := int64(21)
	fixture.service.platform = &fakePlatformFacade{
		platformCode: "seven-admin",
		policy: &platformfacade.ProvisioningPolicy{
			PlatformCode:      "seven-admin",
			AllowAutoRegister: true,
			DefaultOrgID:      &defaultOrgID,
			DefaultDeptID:     &defaultDeptID,
			DefaultPostIDs:    []int64{31, 32},
			DefaultRoleIDs:    []int64{11, 12},
		},
	}

	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode:       "github",
		LoginContextID:     "plctx-roles",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	fixture.driver.profile = domain.ExternalProfile{
		Subject:       "new-subject",
		Email:         "new-user@example.com",
		EmailVerified: true,
		Login:         "new-user",
	}
	if _, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID}); err != nil {
		t.Fatalf("callback: %v", err)
	}
	if fixture.subjects.lastCreate == nil {
		t.Fatal("expected external subject creation")
	}
	if fixture.subjects.lastCreate.RegisterPlatformCode != "seven-admin" || fixture.subjects.lastCreate.RegisterProviderCode != "github" {
		t.Fatalf("registration provenance not propagated: %#v", fixture.subjects.lastCreate)
	}
	if got := fixture.subjects.lastCreate.DefaultRoleIDs; len(got) != 2 || got[0] != 11 || got[1] != 12 {
		t.Fatalf("default roles not propagated: %#v", fixture.subjects.lastCreate)
	}
	if fixture.subjects.lastCreate.DefaultDeptID == nil || *fixture.subjects.lastCreate.DefaultDeptID != defaultDeptID {
		t.Fatalf("default dept not propagated: %#v", fixture.subjects.lastCreate)
	}
	if fixture.subjects.lastCreate.DefaultOrgID == nil || *fixture.subjects.lastCreate.DefaultOrgID != defaultOrgID {
		t.Fatalf("default org not propagated: %#v", fixture.subjects.lastCreate)
	}
	if got := fixture.subjects.lastCreate.DefaultPostIDs; len(got) != 2 || got[0] != 31 || got[1] != 32 {
		t.Fatalf("default posts not propagated: %#v", fixture.subjects.lastCreate)
	}
}

func TestCompleteExternalCallbackBlocksAutoCreateWhenPlatformDisallows(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := activeProvider("github")
	provider.EmailAutoBindEnabled = true
	provider.AccountAutoCreateEnabled = true
	fixture.repo.providers["github"] = provider
	fixture.subjects.createdUserID = 303
	fixture.service.platform = &fakePlatformFacade{
		platformCode: "seven-admin",
		policy:       &platformfacade.ProvisioningPolicy{PlatformCode: "seven-admin", AllowAutoRegister: false},
	}

	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode:       "github",
		LoginContextID:     "plctx-1",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	fixture.driver.profile = domain.ExternalProfile{
		Subject:       "new-subject",
		Email:         "new-user@example.com",
		EmailVerified: true,
		Login:         "new-user",
	}
	_, err = fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID})
	if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeOperateError {
		t.Fatalf("expected unbound operation failure, got %v", err)
	}
	if fixture.subjects.createdCount != 0 {
		t.Fatalf("platform disabled auto register but created %d users", fixture.subjects.createdCount)
	}
	if identity := fixture.repo.identities["github|new-subject"]; identity != nil {
		t.Fatalf("platform disabled auto register but inserted identity: %#v", identity)
	}
}

func TestStartExternalLoginPersistsLoginContextPlatformNotExplicitQuery(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.service.platform = &fakePlatformFacade{platformCode: "seven-admin"}

	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode:       "github",
		LoginContextID:     "plctx-trusted",
		PlatformCode:       "attacker",
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	})
	if err != nil {
		t.Fatalf("start login: %v", err)
	}
	persisted := fixture.repo.loginStates[hashString(start.StateID)]
	if persisted == nil {
		t.Fatal("login state not persisted")
	}
	if persisted.PlatformCode != "seven-admin" {
		t.Fatalf("state platformCode=%q, want loginContext platform seven-admin", persisted.PlatformCode)
	}
}

func TestUpdateProviderStatusDisablesLoginRevokesSessionsAndTokens(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")

	err := fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{
		Status: domain.ProviderStatusDisabled,
		Reason: "risk",
	}, validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding("github", domain.ProviderStatusDisabled)))
	if err != nil {
		t.Fatalf("disable provider: %v", err)
	}
	if fixture.repo.providers["github"].Status != domain.ProviderStatusDisabled {
		t.Fatalf("provider not disabled: %#v", fixture.repo.providers["github"])
	}
	if fixture.sso.revokedProvider != "github" || fixture.repo.revokedProvider != "github" {
		t.Fatalf("sessions/tokens not revoked, sso=%q repo=%q", fixture.sso.revokedProvider, fixture.repo.revokedProvider)
	}
	if fixture.tx.calls != 1 || !fixture.tx.committed {
		t.Fatalf("disable should commit one transaction: %#v", fixture.tx)
	}
}

func TestUpdateProviderStatusRejectsDisableWithoutReason(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")

	err := fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{
		Status: domain.ProviderStatusDisabled,
		Reason: " \t\n ",
	}, validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding("github", domain.ProviderStatusDisabled)))
	if err == nil {
		t.Fatal("expected missing reason to reject provider disable")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeParamsError {
		t.Fatalf("error code = %d, want invalid params", got)
	}
	if fixture.repo.providers["github"].Status != domain.ProviderStatusActive {
		t.Fatalf("provider status changed despite missing reason: %#v", fixture.repo.providers["github"])
	}
	if fixture.tx.calls != 0 {
		t.Fatalf("disable without reason should not start transaction: %#v", fixture.tx)
	}
	if fixture.sso.revokedProvider != "" || fixture.repo.revokedProvider != "" {
		t.Fatalf("revoke should not run without reason, sso=%q repo=%q", fixture.sso.revokedProvider, fixture.repo.revokedProvider)
	}
}

func TestUpdateProviderStatusRollsBackProviderStatusWhenSessionRevokeFails(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.sso.revokeProviderErr = errors.New("sso revoke failed")

	err := fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{
		Status: domain.ProviderStatusDisabled,
		Reason: "risk",
	},
		validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding("github", domain.ProviderStatusDisabled)))
	if err == nil {
		t.Fatal("expected session revoke failure")
	}
	if fixture.repo.providers["github"].Status != domain.ProviderStatusActive {
		t.Fatalf("provider status was not rolled back: %#v", fixture.repo.providers["github"])
	}
	if fixture.tx.committed {
		t.Fatal("failed transaction should not commit")
	}
}

func TestUpdateProviderStatusFailsClosedWhenSessionRevokerMissing(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.service.sessions = nil

	err := fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{
		Status: domain.ProviderStatusDisabled,
		Reason: "risk",
	}, validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding("github", domain.ProviderStatusDisabled)))
	if err == nil {
		t.Fatal("expected missing session revoker to fail closed")
	}
	if got := apperrors.From(err).Code(); got != apperrors.CodeSystemError {
		t.Fatalf("error code = %d, want system error", got)
	}
	if fixture.repo.providers["github"].Status != domain.ProviderStatusActive {
		t.Fatalf("provider status should be rolled back: %#v", fixture.repo.providers["github"])
	}
	if fixture.repo.revokedProvider != "" {
		t.Fatalf("tokens revoked despite rollback: %q", fixture.repo.revokedProvider)
	}
}

func TestUpdateProviderStatusReturnsErrorWhenProviderStatusNotAffected(t *testing.T) {
	fixture := newServiceFixture(t)

	err := fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{
		Status: domain.ProviderStatusDisabled,
		Reason: "risk",
	},
		validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding("github", domain.ProviderStatusDisabled)))
	if err == nil {
		t.Fatal("expected missing provider status update to fail")
	}
	if fixture.sso.revokedProvider != "" || fixture.repo.revokedProvider != "" {
		t.Fatalf("revoke should not run when provider update affects no rows, sso=%q repo=%q", fixture.sso.revokedProvider, fixture.repo.revokedProvider)
	}
}

func TestUpdateIdentityStatusRevokesIdentitySessionsAndTokens(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.identitiesByID[9001] = activeIdentity(101)

	err := fixture.service.UpdateIdentityStatus(context.Background(), 7, 9001, facade.IdentityStatusRequest{
		Status: domain.IdentityStatusDisabled,
		Reason: "risk",
	}, validStepUpProof(StepUpActionExternalLoginIdentityStatusChange, BuildIdentityStatusOperationBinding(9001, domain.IdentityStatusDisabled)))
	if err != nil {
		t.Fatalf("disable identity: %v", err)
	}
	if fixture.repo.identitiesByID[9001].Status != domain.IdentityStatusDisabled {
		t.Fatalf("identity not disabled: %#v", fixture.repo.identitiesByID[9001])
	}
	if fixture.sso.revokedIdentity != 9001 || fixture.repo.revokedIdentity != 9001 {
		t.Fatalf("sessions/tokens not revoked, sso=%d repo=%d", fixture.sso.revokedIdentity, fixture.repo.revokedIdentity)
	}
}

func TestUpdateIdentityStatusReturnsErrorWhenIdentityStatusNotAffected(t *testing.T) {
	fixture := newServiceFixture(t)

	err := fixture.service.UpdateIdentityStatus(context.Background(), 7, 9001, facade.IdentityStatusRequest{Status: domain.IdentityStatusDisabled},
		validStepUpProof(StepUpActionExternalLoginIdentityStatusChange, BuildIdentityStatusOperationBinding(9001, domain.IdentityStatusDisabled)))
	if err == nil {
		t.Fatal("expected missing identity status update to fail")
	}
	if fixture.sso.revokedIdentity != 0 || fixture.repo.revokedIdentity != 0 {
		t.Fatalf("revoke should not run when identity update affects no rows, sso=%d repo=%d", fixture.sso.revokedIdentity, fixture.repo.revokedIdentity)
	}
}

func TestUpdateIdentityStatusRollsBackIdentityStatusWhenTokenRevokeFails(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.identitiesByID[9001] = activeIdentity(101)
	fixture.repo.revokeIdentityErr = errors.New("token revoke failed")

	err := fixture.service.UpdateIdentityStatus(context.Background(), 7, 9001, facade.IdentityStatusRequest{Status: domain.IdentityStatusDisabled},
		validStepUpProof(StepUpActionExternalLoginIdentityStatusChange, BuildIdentityStatusOperationBinding(9001, domain.IdentityStatusDisabled)))
	if err == nil {
		t.Fatal("expected token revoke failure")
	}
	if fixture.repo.identitiesByID[9001].Status != domain.IdentityStatusActive {
		t.Fatalf("identity status was not rolled back: %#v", fixture.repo.identitiesByID[9001])
	}
}

func TestRevokeTokenRequiresStepUpProof(t *testing.T) {
	fixture := newServiceFixture(t)
	if err := fixture.service.RevokeToken(context.Background(), 7, 55, "manual", stepup.ProofMetadata{}); err == nil {
		t.Fatal("expected missing proof to reject token revoke")
	}
	if fixture.repo.revokedToken != 0 {
		t.Fatalf("token revoked without proof: %d", fixture.repo.revokedToken)
	}

	err := fixture.service.RevokeToken(context.Background(), 7, 55, "manual",
		validStepUpProof(StepUpActionExternalOAuthTokenRevoke, BuildTokenRevokeOperationBinding(55)))
	if err != nil {
		t.Fatalf("revoke with proof: %v", err)
	}
	if fixture.repo.revokedToken != 55 {
		t.Fatalf("token not revoked: %d", fixture.repo.revokedToken)
	}
}

func TestAdminMutationsRequireStepUpProof(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identitiesByID[9001] = activeIdentity(101)

	checks := []struct {
		name string
		run  func() error
	}{
		{"create provider", func() error {
			_, err := fixture.service.CreateProvider(context.Background(), 7, providerSaveRequest("gitlab"), stepup.ProofMetadata{})
			return err
		}},
		{"update provider", func() error {
			_, err := fixture.service.UpdateProvider(context.Background(), 7, "github", providerUpdateRequest("github"), stepup.ProofMetadata{})
			return err
		}},
		{"provider status", func() error {
			return fixture.service.UpdateProviderStatus(context.Background(), 7, "github", facade.ProviderStatusRequest{Status: domain.ProviderStatusDisabled}, stepup.ProofMetadata{})
		}},
		{"secret rotate", func() error {
			return fixture.service.RotateClientSecret(context.Background(), 7, "github", facade.RotateClientSecretRequest{ClientSecret: "new"}, stepup.ProofMetadata{})
		}},
		{"identity status", func() error {
			return fixture.service.UpdateIdentityStatus(context.Background(), 7, 9001, facade.IdentityStatusRequest{Status: domain.IdentityStatusDisabled}, stepup.ProofMetadata{})
		}},
		{"token revoke", func() error {
			return fixture.service.RevokeToken(context.Background(), 7, 55, "manual", stepup.ProofMetadata{})
		}},
	}
	for _, tt := range checks {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected forbidden step-up rejection, got %v", err)
			}
		})
	}
}

type serviceFixture struct {
	service  *Service
	repo     *fakeExternalLoginRepository
	cache    *fakeStateCache
	drivers  *fakeDriverRegistry
	driver   *fakeProviderDriver
	subjects *fakeSubjectFacade
	profiles *fakeProfileFacade
	sso      *fakeSSOFacades
	tx       *fakeTransactor
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	idGen, err := xid.New(1)
	if err != nil {
		t.Fatalf("id generator: %v", err)
	}
	repo := newFakeExternalLoginRepository()
	cache := &fakeStateCache{}
	driver := &fakeProviderDriver{
		profile: domain.ExternalProfile{
			Subject:       "subject-1",
			Login:         "octocat",
			Email:         "octocat@example.com",
			EmailVerified: true,
			DisplayName:   "Octo Cat",
		},
	}
	drivers := &fakeDriverRegistry{drivers: map[string]ProviderDriverPort{"github": driver, "google": driver}}
	subjects := &fakeSubjectFacade{
		byID: map[int64]*userfacade.SubjectRecord{
			101: {UserID: 101, AccountName: "bound", Email: "bound@example.com", Enabled: true},
			202: {UserID: 202, AccountName: "alice", Email: "alice@example.com", Enabled: true},
		},
		byEmail: map[string]*userfacade.SubjectRecord{},
	}
	profiles := &fakeProfileFacade{}
	sso := &fakeSSOFacades{}
	tx := &fakeTransactor{repo: repo}
	service := NewService(ServiceDeps{
		Config: config.ExternalLoginConfig{
			Enabled:         true,
			CallbackBaseURL: "https://seven.example.com",
			StateTTLSeconds: 300,
			FailClosed:      true,
		},
		Transactor:             tx,
		IDGen:                  idGen,
		Repository:             repo,
		StateCache:             cache,
		Drivers:                drivers,
		SecretValue:            fakeSecretValueService{},
		Subjects:               subjects,
		Profiles:               profiles,
		AuthenticationComplete: sso,
		BootstrapSession:       sso,
		Sessions:               sso,
		Platform:               &fakePlatformFacade{platformCode: "seven-admin"},
	})
	return &serviceFixture{service: service, repo: repo, cache: cache, drivers: drivers, driver: driver, subjects: subjects, profiles: profiles, sso: sso, tx: tx}
}

func activeProvider(code string) *domain.Provider {
	return &domain.Provider{
		ID:                    1,
		ProviderCode:          code,
		ProviderName:          strings.ToUpper(code),
		ProtocolType:          domain.ProtocolTypeOAuth2,
		AuthorizationEndpoint: "https://provider.example.com/auth",
		TokenEndpoint:         "https://provider.example.com/token",
		ClientID:              "client-" + code,
		Scopes:                []string{"openid", "email"},
		RedirectURI:           "https://seven.example.com/login/external/" + code + "/callback",
		DisplayName:           strings.ToUpper(code),
		SortOrder:             map[string]int{"github": 10, "google": 20}[code],
		DisplayEnabled:        true,
		LoginEnabled:          true,
		BindEnabled:           true,
		Status:                domain.ProviderStatusActive,
	}
}

func platformStartRequest(providerCode string) facade.StartExternalLoginRequest {
	return facade.StartExternalLoginRequest{
		ProviderCode:       providerCode,
		LoginContextID:     "plctx-" + providerCode,
		TrustedSource:      facade.TrustedSource{Host: "127.0.0.1:5291"},
		RedirectAfterLogin: "/",
	}
}

func activeIdentity(userID int64) *domain.ExternalIdentity {
	return &domain.ExternalIdentity{
		ID:              9001,
		ProviderCode:    "github",
		ExternalSubject: "subject-1",
		UserID:          userID,
		Status:          domain.IdentityStatusActive,
		EmailVerified:   true,
	}
}

func providerSaveRequest(code string) facade.ProviderSaveRequest {
	p := activeProvider(code)
	return facade.ProviderSaveRequest{
		ProviderCode:          p.ProviderCode,
		ProviderName:          p.ProviderName,
		ProtocolType:          p.ProtocolType,
		AuthorizationEndpoint: p.AuthorizationEndpoint,
		TokenEndpoint:         p.TokenEndpoint,
		ClientID:              p.ClientID,
		Scopes:                p.Scopes,
		RedirectURI:           p.RedirectURI,
		DisplayName:           p.DisplayName,
		DisplayEnabled:        true,
		LoginEnabled:          true,
		BindEnabled:           true,
	}
}

func providerUpdateRequest(code string) facade.ProviderUpdateRequest {
	save := providerSaveRequest(code)
	return facade.ProviderUpdateRequest{
		ProviderName:          save.ProviderName,
		ProtocolType:          save.ProtocolType,
		AuthorizationEndpoint: save.AuthorizationEndpoint,
		TokenEndpoint:         save.TokenEndpoint,
		ClientID:              save.ClientID,
		Scopes:                save.Scopes,
		RedirectURI:           save.RedirectURI,
		DisplayName:           save.DisplayName,
		DisplayEnabled:        true,
		LoginEnabled:          true,
		BindEnabled:           true,
	}
}

func validStepUpProof(action, binding string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        action,
		OperationBinding:      binding,
		ProofIdentifier:       "proof-1",
		ChallengeIdentifier:   "challenge-1",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
}

type fakeExternalLoginRepository struct {
	providers             map[string]*domain.Provider
	identities            map[string]*domain.ExternalIdentity
	identitiesByID        map[int64]*domain.ExternalIdentity
	loginStates           map[string]*domain.LoginState
	consumedHashes        map[string]bool
	tokens                map[int64]domain.OAuthToken
	revokedProvider       string
	revokedIdentity       int64
	revokedToken          int64
	revokeIdentityErr     error
	providerWrites        int
	managedCommands       map[string]*domain.ManagedProviderCommand
	insertIdentityStarted chan struct{}
	releaseIdentityInsert chan struct{}
}

func newFakeExternalLoginRepository() *fakeExternalLoginRepository {
	return &fakeExternalLoginRepository{
		providers:       map[string]*domain.Provider{},
		identities:      map[string]*domain.ExternalIdentity{},
		identitiesByID:  map[int64]*domain.ExternalIdentity{},
		loginStates:     map[string]*domain.LoginState{},
		consumedHashes:  map[string]bool{},
		tokens:          map[int64]domain.OAuthToken{},
		managedCommands: map[string]*domain.ManagedProviderCommand{},
	}
}

func (f *fakeExternalLoginRepository) snapshot() (map[string]*domain.Provider, map[int64]*domain.ExternalIdentity) {
	providers := map[string]*domain.Provider{}
	for key, value := range f.providers {
		copy := *value
		providers[key] = &copy
	}
	identities := map[int64]*domain.ExternalIdentity{}
	for key, value := range f.identitiesByID {
		copy := *value
		identities[key] = &copy
	}
	return providers, identities
}

func (f *fakeExternalLoginRepository) restore(providers map[string]*domain.Provider, identities map[int64]*domain.ExternalIdentity) {
	f.providers = providers
	f.identitiesByID = identities
}

func (f *fakeExternalLoginRepository) ListLoginMethods(context.Context) ([]domain.Provider, error) {
	items := make([]domain.Provider, 0, len(f.providers))
	for _, provider := range f.providers {
		if provider.Status == domain.ProviderStatusActive && provider.DisplayEnabled && provider.LoginEnabled {
			items = append(items, *provider)
		}
	}
	if len(items) == 2 && items[0].SortOrder > items[1].SortOrder {
		items[0], items[1] = items[1], items[0]
	}
	return items, nil
}

func (f *fakeExternalLoginRepository) FindProvider(_ context.Context, providerCode string) (*domain.Provider, error) {
	provider := f.providers[providerCode]
	if provider == nil {
		return nil, nil
	}
	copy := *provider
	return &copy, nil
}

func (f *fakeExternalLoginRepository) FindProviderForUpdate(ctx context.Context, providerCode string) (*domain.Provider, error) {
	return f.FindProvider(ctx, providerCode)
}

func (f *fakeExternalLoginRepository) InsertProvider(_ context.Context, item *domain.Provider, _ int64) error {
	copy := *item
	f.providers[item.ProviderCode] = &copy
	f.providerWrites++
	return nil
}

func (f *fakeExternalLoginRepository) UpdateProvider(_ context.Context, item *domain.Provider, _ int64) error {
	copy := *item
	if existing := f.providers[item.ProviderCode]; existing != nil {
		copy.Status = existing.Status
	}
	f.providers[item.ProviderCode] = &copy
	f.providerWrites++
	return nil
}

func (f *fakeExternalLoginRepository) UpdateProviderStatus(_ context.Context, providerCode string, status int, _ int64, _ time.Time) (bool, error) {
	if provider := f.providers[providerCode]; provider != nil {
		provider.Status = status
		return true, nil
	}
	return false, nil
}

func (f *fakeExternalLoginRepository) ListProviders(context.Context, domain.ProviderQuery) ([]domain.Provider, int64, error) {
	items := make([]domain.Provider, 0, len(f.providers))
	for _, provider := range f.providers {
		items = append(items, *provider)
	}
	return items, int64(len(items)), nil
}

func (f *fakeExternalLoginRepository) ListProviderMethods(context.Context, string) ([]domain.ProviderMethod, error) {
	return nil, nil
}

func (f *fakeExternalLoginRepository) ReplaceProviderMethods(context.Context, string, []domain.ProviderMethod) error {
	return nil
}

func (f *fakeExternalLoginRepository) InsertLoginState(_ context.Context, item *domain.LoginState) error {
	copy := *item
	f.loginStates[item.StateHash] = &copy
	return nil
}

func (f *fakeExternalLoginRepository) ConsumeLoginState(_ context.Context, stateHash string, _ time.Time) (*domain.LoginState, error) {
	if f.consumedHashes[stateHash] {
		return nil, nil
	}
	item := f.loginStates[stateHash]
	if item == nil {
		return nil, nil
	}
	f.consumedHashes[stateHash] = true
	copy := *item
	copy.Status = domain.LoginStateStatusConsumed
	return &copy, nil
}

func (f *fakeExternalLoginRepository) FindIdentityBySubject(_ context.Context, providerCode, externalIssuer, externalSubject string) (*domain.ExternalIdentity, error) {
	identity := f.identities[fakeIdentityKey(providerCode, externalIssuer, externalSubject)]
	if identity == nil {
		return nil, nil
	}
	copy := *identity
	return &copy, nil
}

func (f *fakeExternalLoginRepository) InsertIdentity(_ context.Context, item *domain.ExternalIdentity, _ int64) error {
	if f.insertIdentityStarted != nil {
		close(f.insertIdentityStarted)
		f.insertIdentityStarted = nil
	}
	if f.releaseIdentityInsert != nil {
		<-f.releaseIdentityInsert
		f.releaseIdentityInsert = nil
	}
	if item.ID == 0 {
		item.ID = 10000 + int64(len(f.identities))
	}
	copy := *item
	f.identities[fakeIdentityKey(item.ProviderCode, item.ExternalIssuer, item.ExternalSubject)] = &copy
	f.identitiesByID[item.ID] = &copy
	return nil
}

func (f *fakeExternalLoginRepository) CountIdentitiesByProvider(_ context.Context, providerCode string) (int64, error) {
	var count int64
	for _, identity := range f.identitiesByID {
		if identity.ProviderCode == providerCode {
			count++
		}
	}
	return count, nil
}

func (f *fakeExternalLoginRepository) FindManagedProviderCommand(_ context.Context, providerCode, connectionVersion string) (*domain.ManagedProviderCommand, error) {
	item := f.managedCommands[providerCode+"\x00"+connectionVersion]
	if item == nil {
		return nil, nil
	}
	copy := *item
	return &copy, nil
}

func (f *fakeExternalLoginRepository) InsertManagedProviderCommand(_ context.Context, command *domain.ManagedProviderCommand) error {
	copy := *command
	f.managedCommands[command.ProviderCode+"\x00"+command.ConnectionVersion] = &copy
	return nil
}

func fakeIdentityKey(providerCode, issuer, subject string) string {
	if strings.TrimSpace(issuer) != "" {
		return issuer + "\x00" + subject
	}
	return providerCode + "|" + subject
}

func (f *fakeExternalLoginRepository) ListIdentities(context.Context, domain.IdentityQuery) ([]domain.ExternalIdentity, int64, error) {
	items := make([]domain.ExternalIdentity, 0, len(f.identitiesByID))
	for _, identity := range f.identitiesByID {
		items = append(items, *identity)
	}
	return items, int64(len(items)), nil
}

func (f *fakeExternalLoginRepository) UpdateIdentityStatus(_ context.Context, identityID int64, status int, _ int64, _ time.Time) (bool, error) {
	if identity := f.identitiesByID[identityID]; identity != nil {
		identity.Status = status
		return true, nil
	}
	return false, nil
}

func (f *fakeExternalLoginRepository) TouchIdentityLogin(context.Context, int64, domain.ExternalProfile, time.Time) error {
	return nil
}

func (f *fakeExternalLoginRepository) InsertToken(_ context.Context, item *domain.OAuthToken) error {
	if item.ID == 0 {
		item.ID = int64(len(f.tokens) + 1)
	}
	f.tokens[item.ID] = *item
	return nil
}

func (f *fakeExternalLoginRepository) FindActiveToken(_ context.Context, providerCode string, identityID int64, userID int64, tokenPurpose string, scopeHash string) (*domain.OAuthToken, error) {
	for _, token := range f.tokens {
		if token.ProviderCode == providerCode &&
			token.IdentityID == identityID &&
			token.UserID == userID &&
			token.TokenPurpose == tokenPurpose &&
			token.ScopeHash == scopeHash &&
			token.Status == domain.TokenStatusActive &&
			token.RevokedAt == nil {
			copy := token
			return &copy, nil
		}
	}
	return nil, nil
}

func (f *fakeExternalLoginRepository) UpdateTokenSet(context.Context, *domain.OAuthToken, int) (bool, error) {
	return true, nil
}

func (f *fakeExternalLoginRepository) RevokeToken(_ context.Context, tokenID int64, _ time.Time, _ string) (bool, error) {
	f.revokedToken = tokenID
	return true, nil
}

func (f *fakeExternalLoginRepository) RevokeTokensByProvider(_ context.Context, providerCode string, _ time.Time, _ string) (int64, error) {
	f.revokedProvider = providerCode
	return 1, nil
}

func (f *fakeExternalLoginRepository) RevokeTokensByIdentity(_ context.Context, identityID int64, _ time.Time, _ string) (int64, error) {
	if f.revokeIdentityErr != nil {
		return 0, f.revokeIdentityErr
	}
	f.revokedIdentity = identityID
	return 1, nil
}

func (f *fakeExternalLoginRepository) ListTokens(context.Context, domain.TokenQuery) ([]domain.OAuthToken, int64, error) {
	items := make([]domain.OAuthToken, 0, len(f.tokens))
	for _, token := range f.tokens {
		items = append(items, token)
	}
	return items, int64(len(items)), nil
}

type fakeStateCache struct {
	states map[string]domain.LoginState
}

func (f *fakeStateCache) Put(_ context.Context, item domain.LoginState, _ time.Duration) error {
	if f.states == nil {
		f.states = map[string]domain.LoginState{}
	}
	f.states[item.StateID] = item
	return nil
}

func (f *fakeStateCache) Get(_ context.Context, stateID string) (*domain.LoginState, error) {
	if f.states == nil {
		return nil, nil
	}
	item, ok := f.states[stateID]
	if !ok {
		return nil, nil
	}
	return &item, nil
}

func (f *fakeStateCache) Delete(_ context.Context, stateID string) error {
	delete(f.states, stateID)
	return nil
}

type fakeDriverRegistry struct {
	drivers map[string]ProviderDriverPort
}

func (f *fakeDriverRegistry) Get(providerCode string) (ProviderDriverPort, bool) {
	driver, ok := f.drivers[providerCode]
	return driver, ok
}

func (f *fakeDriverRegistry) Capabilities() map[string]domain.ProviderCapability {
	return domain.BuiltInProviderCapabilities()
}

type fakeProviderDriver struct {
	profile                domain.ExternalProfile
	profileErr             error
	exchangeErr            error
	exchangeCount          int
	requireNonce           bool
	lastAuthorizationNonce string
	lastExchangeNonce      string
	exchangeStarted        chan struct{}
	releaseExchange        chan struct{}
}

func (f *fakeProviderDriver) BuildAuthorizationURL(_ context.Context, provider domain.Provider, request AuthorizationRequest) (string, error) {
	f.lastAuthorizationNonce = request.Nonce
	values := url.Values{}
	values.Set("client_id", provider.ClientID)
	values.Set("redirect_uri", request.RedirectURI)
	values.Set("state", request.State)
	values.Set("code_challenge", request.CodeChallenge)
	values.Set("code_challenge_method", request.CodeChallengeMethod)
	return provider.AuthorizationEndpoint + "?" + values.Encode(), nil
}

func (f *fakeProviderDriver) ExchangeCode(_ context.Context, _ domain.Provider, request TokenExchangeRequest) (*TokenExchangeResult, error) {
	f.exchangeCount++
	if f.exchangeStarted != nil {
		close(f.exchangeStarted)
	}
	if f.releaseExchange != nil {
		<-f.releaseExchange
	}
	f.lastExchangeNonce = request.Nonce
	if f.requireNonce && strings.TrimSpace(request.Nonce) == "" {
		return nil, errors.New("expected nonce is required")
	}
	if f.exchangeErr != nil {
		return nil, f.exchangeErr
	}
	expiresAt := time.Now().UTC().Add(time.Hour)
	return &TokenExchangeResult{
		TokenSet: domain.TokenSet{
			AccessToken: "upstream-access",
			TokenType:   "Bearer",
			Scopes:      []string{"openid", "email"},
			ExpiresAt:   &expiresAt,
		},
	}, nil
}

func (f *fakeProviderDriver) ResolveProfile(context.Context, domain.Provider, TokenExchangeResult) (*domain.ExternalProfile, error) {
	if f.profileErr != nil {
		return nil, f.profileErr
	}
	profile := f.profile
	return &profile, nil
}

func (f *fakeProviderDriver) RevokeToken(context.Context, domain.Provider, domain.TokenSet) error {
	return nil
}

type fakeSubjectFacade struct {
	byID          map[int64]*userfacade.SubjectRecord
	byEmail       map[string]*userfacade.SubjectRecord
	createdUserID int64
	createdCount  int
	lastCreate    *userfacade.CreateExternalSubjectCommand
}

func (f *fakeSubjectFacade) FindSubjectByID(_ context.Context, userID int64) (*userfacade.SubjectRecord, error) {
	subject := f.byID[userID]
	if subject == nil {
		return nil, nil
	}
	copy := *subject
	return &copy, nil
}

func (f *fakeSubjectFacade) FindSubjectByAccount(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeSubjectFacade) FindSubjectByEmail(_ context.Context, email string) (*userfacade.SubjectRecord, error) {
	subject := f.byEmail[email]
	if subject == nil {
		return nil, nil
	}
	copy := *subject
	return &copy, nil
}

func (f *fakeSubjectFacade) CreateExternalSubject(_ context.Context, command userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	f.createdCount++
	copyCommand := command
	f.lastCreate = &copyCommand
	if f.createdUserID == 0 {
		f.createdUserID = 3001
	}
	subject := &userfacade.SubjectRecord{
		UserID:      f.createdUserID,
		AccountName: command.AccountName,
		Email:       command.UserEmail,
		Enabled:     true,
	}
	if f.byID == nil {
		f.byID = make(map[int64]*userfacade.SubjectRecord)
	}
	if f.byEmail == nil {
		f.byEmail = make(map[string]*userfacade.SubjectRecord)
	}
	f.byID[subject.UserID] = subject
	f.byEmail[command.UserEmail] = subject
	copy := *subject
	return &copy, nil
}

func (f *fakeSubjectFacade) CreateFormSubject(context.Context, userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}

func (f *fakeSubjectFacade) ExistsByID(_ context.Context, userID int64) (bool, error) {
	return f.byID[userID] != nil, nil
}

func (f *fakeSubjectFacade) BuildPrincipalSeed(context.Context, int64) (*userfacade.UserPrincipalSeed, error) {
	return nil, nil
}

type fakePlatformFacade struct {
	platformCode            string
	policy                  *platformfacade.ProvisioningPolicy
	lastAuthority           platformfacade.ProvisioningAuthority
	loginMethodErr          error
	requireLoginMethodCalls int
}

func (f *fakePlatformFacade) ResolveLoginOptions(context.Context, platformfacade.ResolvePlatformRequest) (*platformfacade.LoginOptionResult, error) {
	return nil, nil
}
func (f *fakePlatformFacade) ResolvePlatformCode(context.Context, platformfacade.ResolvePlatformRequest) (string, error) {
	return f.platformCode, nil
}
func (f *fakePlatformFacade) ValidateLoginContext(_ context.Context, loginContextID string, _ platformfacade.ResolvePlatformRequest) (*platformfacade.LoginContextValidation, error) {
	return &platformfacade.LoginContextValidation{LoginContextID: loginContextID, PlatformCode: f.platformCode, Authority: "PRESENTATION"}, nil
}
func (f *fakePlatformFacade) IssueProvisioningAuthority(_ context.Context, loginContextID string, _ platformfacade.ResolvePlatformRequest) (*platformfacade.ProvisioningAuthority, error) {
	return &platformfacade.ProvisioningAuthority{AuthorityID: "plprov_test", LoginContextID: loginContextID, PlatformCode: f.platformCode, Authority: platformfacade.AuthorityProvisioning}, nil
}
func (f *fakePlatformFacade) GetProvisioningPolicy(_ context.Context, authority platformfacade.ProvisioningAuthority) (*platformfacade.ProvisioningPolicy, error) {
	f.lastAuthority = authority
	if strings.TrimSpace(authority.AuthorityID) == "" {
		return nil, apperrors.Forbidden("平台注册授权无效")
	}
	if f.policy == nil {
		return &platformfacade.ProvisioningPolicy{PlatformCode: f.platformCode, AllowAutoRegister: true}, nil
	}
	copy := *f.policy
	return &copy, nil
}
func (f *fakePlatformFacade) GetFormRegistrationPolicy(context.Context, string) (*platformfacade.ProvisioningPolicy, error) {
	return &platformfacade.ProvisioningPolicy{PlatformCode: f.platformCode, AllowFormRegister: true}, nil
}
func (f *fakePlatformFacade) RequireLoginMethod(context.Context, string, string, string) error {
	f.requireLoginMethodCalls++
	return f.loginMethodErr
}

type fakeProfileFacade struct {
	lastSync *userfacade.SyncExternalProfileCommand
}

func (f *fakeProfileFacade) GetProfileByUserID(context.Context, int64) (*userfacade.UserProfile, error) {
	return nil, nil
}

func (f *fakeProfileFacade) UpdateSelfProfile(context.Context, userfacade.UpdateSelfProfileCommand) error {
	return nil
}

func (f *fakeProfileFacade) CommitCurrentUserAvatar(context.Context, int64, int64) (string, error) {
	return "", nil
}

func (f *fakeProfileFacade) UpdateSelfEmail(context.Context, userfacade.UpdateSelfEmailCommand) error {
	return nil
}

func (f *fakeProfileFacade) SyncExternalProfile(_ context.Context, command userfacade.SyncExternalProfileCommand) error {
	copy := command
	f.lastSync = &copy
	return nil
}

type fakeSSOFacades struct {
	completeCount     int
	bootstrapCount    int
	lastComplete      ssofacade.CompleteInteractiveAuthenticationCommand
	revokedProvider   string
	revokedIdentity   int64
	revokeProviderErr error
}

func (f *fakeSSOFacades) CompleteInteractiveAuthentication(_ context.Context, command ssofacade.CompleteInteractiveAuthenticationCommand) (*ssofacade.AuthenticationCompletionResult, error) {
	f.completeCount++
	f.lastComplete = command
	return &ssofacade.AuthenticationCompletionResult{
		Authenticated:            true,
		LoginTransactionID:       command.LoginTransactionID,
		RedirectURL:              "/console",
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=session; Path=/",
	}, nil
}

func (f *fakeSSOFacades) BootstrapFirstPartySession(_ context.Context, command ssofacade.BootstrapSessionCommand) (*ssofacade.BootstrapSessionResult, error) {
	f.bootstrapCount++
	return &ssofacade.BootstrapSessionResult{
		AccessToken:              "access-local",
		TokenType:                "Bearer",
		AccessTTLSeconds:         900,
		SessionCookieHeaderValue: "SEVEN_SSO_SESSION=session; Path=/",
		RefreshCookieHeaderValue: "__Host-seven_sso_rt=refresh; Path=/",
	}, nil
}

func (f *fakeSSOFacades) RevokeSessionsByExternalProvider(_ context.Context, providerCode string) (int64, error) {
	if f.revokeProviderErr != nil {
		return 0, f.revokeProviderErr
	}
	f.revokedProvider = providerCode
	return 1, nil
}

func (f *fakeSSOFacades) RevokeSessionsByPlatformCode(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeSSOFacades) RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (f *fakeSSOFacades) RevokeSessionsByExternalIdentity(_ context.Context, identityID int64) (int64, error) {
	f.revokedIdentity = identityID
	return 1, nil
}

func (f *fakeSSOFacades) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSSOFacades) ListActiveSessions(context.Context) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (f *fakeSSOFacades) CountActiveSessions(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeSSOFacades) RevokeSession(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeSSOFacades) RevokeSessionsByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (f *fakeSSOFacades) ResolveActiveSessionRecord(context.Context, string) (*ssofacade.SessionRecord, error) {
	return nil, nil
}

type fakeTransactor struct {
	repo      *fakeExternalLoginRepository
	calls     int
	committed bool
}

func (f *fakeTransactor) Enabled() bool {
	return true
}

func (f *fakeTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.calls++
	providers, identities := f.repo.snapshot()
	err := fn(ctx)
	if err != nil {
		f.repo.restore(providers, identities)
		return err
	}
	f.committed = true
	return nil
}

var _ TransactorPort = (*fakeTransactor)(nil)
