package application

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
)

func TestManagedProviderCodePreservesOwnerAndRejectsOverflow(t *testing.T) {
	code, err := domain.ManagedProviderCode("order.admin-1")
	if err != nil {
		t.Fatalf("managed provider code: %v", err)
	}
	if code != "hub:order.admin-1" {
		t.Fatalf("provider code=%q", code)
	}
	if _, err := domain.ManagedProviderCode("a234567890123456789012345678901234567890123456789012345678901"); err == nil {
		t.Fatal("expected provider code overflow to be rejected")
	}
}

func TestManagedOIDCProviderApplyReplayDisableAndOwnership(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.service.discovery = fakeOIDCDiscovery{result: OIDCDiscoveryResult{
		Issuer: "https://hub.example.com", AuthorizationEndpoint: "https://hub.example.com/sso/oauth2/authorize",
		TokenEndpoint: "https://hub.example.com/sso/oauth2/token", UserinfoEndpoint: "https://hub.example.com/sso/oauth2/userinfo",
		JWKSURI: "https://hub.example.com/sso/oauth2/jwks",
	}}
	command := facade.ManagedOIDCProviderCommand{
		OwnerNodeCode: "order-admin", ConnectionVersion: "v1", Enabled: true, DisplayName: "Hub",
		Issuer: "https://hub.example.com", ClientID: "hub-node-order-admin", ClientSecret: "secret-v1",
		RedirectURI: "https://node.example.com/login/external/hub:order-admin/callback",
	}
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("apply managed provider: %v", err)
	}
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("replay managed provider: %v", err)
	}
	provider := fixture.repo.providers["hub:order-admin"]
	if provider == nil || provider.Issuer != command.Issuer || provider.ClientID != command.ClientID || provider.Status != domain.ProviderStatusActive {
		t.Fatalf("managed provider=%#v", provider)
	}
	if fixture.repo.providerWrites != 1 {
		t.Fatalf("provider writes=%d want 1", fixture.repo.providerWrites)
	}

	conflict := command
	conflict.ClientID = "different-client"
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), conflict); err == nil {
		t.Fatal("expected same-version conflict")
	}
	foreign := command
	foreign.OwnerNodeCode = "other-node"
	foreignMetadata := `{"managedBy":"hub_control","ownerNodeCode":"order-admin","connectionVersion":"v1","connectionHash":"different","persistLoginTokens":false}`
	fixture.repo.providers["hub:other-node"] = &domain.Provider{ProviderCode: "hub:other-node", MetadataJSON: foreignMetadata}
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), foreign); err == nil {
		t.Fatal("expected foreign ownership rejection")
	}
	fixture.service.discovery = fakeOIDCDiscovery{err: errors.New("hub unavailable")}
	if err := fixture.service.DisableManagedOIDCProvider(context.Background(), "order-admin", "v2", 0); err != nil {
		t.Fatalf("disable managed provider: %v", err)
	}
	if fixture.repo.providers["hub:order-admin"].Status != domain.ProviderStatusDisabled {
		t.Fatal("managed provider was not disabled")
	}
	methods, err := fixture.service.ListLoginMethods(context.Background(), facade.ListLoginMethodsRequest{})
	if err != nil {
		t.Fatalf("list login methods after disable: %v", err)
	}
	for _, method := range methods {
		if method.ProviderCode == "hub:order-admin" {
			t.Fatalf("disabled managed provider remained visible: %+v", method)
		}
	}
	if _, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("hub:order-admin")); err == nil {
		t.Fatal("disabled managed provider accepted a new federation start")
	}
	if fixture.sso.revokedProvider != "" {
		t.Fatalf("managed disable revoked local sessions: %q", fixture.sso.revokedProvider)
	}
}

func TestManagedProviderRejectsDelayedDisableFromOlderTargetRevision(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.service.discovery = fakeOIDCDiscovery{}
	enable := managedTestCommand("v2", "https://hub.example.com")
	enable.TargetRevision = 2
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), enable); err != nil {
		t.Fatalf("apply newer enable: %v", err)
	}
	if err := fixture.service.DisableManagedOIDCProvider(context.Background(), "order-admin", "disable-v1", 1); err == nil {
		t.Fatal("accepted delayed disable from older target revision")
	}
	provider := fixture.repo.providers["hub:order-admin"]
	if provider.Status != domain.ProviderStatusActive || !provider.LoginEnabled {
		t.Fatalf("stale disable changed provider: %#v", provider)
	}
}

func TestManagedProviderTargetRevisionReplayConflictAndReenable(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.service.discovery = fakeOIDCDiscovery{}
	enable := managedTestCommand("v1", "https://hub.example.com")
	enable.TargetRevision = 1
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), enable); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), enable); err != nil {
		t.Fatalf("replay same revision and digest: %v", err)
	}
	conflict := enable
	conflict.ClientID = "different-client"
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), conflict); err == nil {
		t.Fatal("accepted same revision conflict")
	}
	if err := fixture.service.DisableManagedOIDCProvider(context.Background(), "order-admin", "disable-v2", 2); err != nil {
		t.Fatalf("disable revision 2: %v", err)
	}
	if err := fixture.service.DisableManagedOIDCProvider(context.Background(), "order-admin", "disable-v2", 2); err != nil {
		t.Fatalf("replay same disable revision and digest: %v", err)
	}
	reenable := managedTestCommand("v3", "https://hub.example.com")
	reenable.TargetRevision = 3
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), reenable); err != nil {
		t.Fatalf("re-enable revision 3: %v", err)
	}
	provider := fixture.repo.providers["hub:order-admin"]
	if provider.Status != domain.ProviderStatusActive || !provider.LoginEnabled {
		t.Fatalf("provider not re-enabled: %#v", provider)
	}
}

func TestFederatedIdentityLookupUsesValidatedIssuerAndSubject(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.identities["https://hub-a.example\x00same-sub"] = &domain.ExternalIdentity{ID: 1, ProviderCode: "hub:node-a", ExternalIssuer: "https://hub-a.example", ExternalSubject: "same-sub", UserID: 1001, Status: domain.IdentityStatusActive}
	fixture.repo.identities["https://hub-b.example\x00same-sub"] = &domain.ExternalIdentity{ID: 2, ProviderCode: "hub:node-b", ExternalIssuer: "https://hub-b.example", ExternalSubject: "same-sub", UserID: 2001, Status: domain.IdentityStatusActive}

	a, err := fixture.repo.FindIdentityBySubject(context.Background(), "hub:node-a", "https://hub-a.example", "same-sub")
	if err != nil || a == nil || a.UserID != 1001 {
		t.Fatalf("issuer A lookup=%#v err=%v", a, err)
	}
	b, err := fixture.repo.FindIdentityBySubject(context.Background(), "hub:node-b", "https://hub-b.example", "same-sub")
	if err != nil || b == nil || b.UserID != 2001 {
		t.Fatalf("issuer B lookup=%#v err=%v", b, err)
	}
}

func TestManagedLoginNeverMergesEmailAndDoesNotPersistTokens(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := managedTestProvider("order-admin", "https://hub.example.com")
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.drivers.drivers["oidc"] = fixture.driver
	fixture.subjects.byEmail["alice@example.com"] = &userfacade.SubjectRecord{UserID: 202, AccountName: "alice", Email: "alice@example.com", Enabled: true}
	fixture.subjects.createdUserID = 303
	fixture.driver.profile = domain.ExternalProfile{Subject: "hub-sub", Email: "alice@example.com", EmailVerified: true, Login: "alice", DisplayName: "Alice"}

	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest(provider.ProviderCode))
	if err != nil {
		t.Fatalf("start managed login: %v", err)
	}
	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID,
	})
	if err != nil {
		t.Fatalf("complete managed login: %v", err)
	}
	if result.UserID != 303 || fixture.subjects.createdCount != 1 {
		t.Fatalf("managed login merged existing email: result=%#v created=%d", result, fixture.subjects.createdCount)
	}
	if fixture.subjects.lastCreate == nil || !fixture.subjects.lastCreate.DisableEmailMerge {
		t.Fatalf("managed user creation did not disable downstream email merge: %#v", fixture.subjects.lastCreate)
	}
	identity := fixture.repo.identities["https://hub.example.com\x00hub-sub"]
	if identity == nil || identity.ExternalIssuer != "https://hub.example.com" || identity.UserID != 303 {
		t.Fatalf("issuer-aware identity=%#v", identity)
	}
	if len(fixture.repo.tokens) != 0 {
		t.Fatalf("managed login persisted token vault rows: %#v", fixture.repo.tokens)
	}
}

func TestManagedCurrentUserBindingDoesNotPersistTokens(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := managedTestProvider("order-admin", "https://hub.example.com")
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.drivers.drivers["oidc"] = fixture.driver
	fixture.driver.profile = domain.ExternalProfile{Subject: "bound-sub", Email: "bound@example.com", EmailVerified: true}

	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{
		ProviderCode: provider.ProviderCode, BindUserID: 101, RedirectAfterLogin: "/account/settings",
	})
	if err != nil {
		t.Fatalf("start managed binding: %v", err)
	}
	result, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{
		ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID,
	})
	if err != nil {
		t.Fatalf("complete managed binding: %v", err)
	}
	if result.Authenticated || result.Outcome != "BOUND" || result.Purpose != "IDENTITY_LINK" {
		t.Fatalf("binding was misreported as login: %#v", result)
	}
	if len(fixture.repo.tokens) != 0 {
		t.Fatalf("managed binding persisted token vault rows: %#v", fixture.repo.tokens)
	}
}

func TestManagedProviderTokenPolicyFailsClosedOnMalformedMetadata(t *testing.T) {
	if persistLoginTokens(domain.Provider{ProviderCode: "hub:order-admin", MetadataJSON: "{"}) {
		t.Fatal("managed namespace enabled token persistence after metadata corruption")
	}
}

func TestOrdinaryProviderStillUsesProviderSubjectAndPersistsTokens(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.repo.providers["github"] = activeProvider("github")
	fixture.repo.identities["github|subject-1"] = activeIdentity(101)
	start, err := fixture.service.StartExternalLogin(context.Background(), platformStartRequest("github"))
	if err != nil {
		t.Fatalf("start ordinary login: %v", err)
	}
	if _, err := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: "github", Code: "code", State: start.StateID}); err != nil {
		t.Fatalf("complete ordinary login: %v", err)
	}
	if len(fixture.repo.tokens) != 1 {
		t.Fatalf("ordinary login token rows=%d want 1", len(fixture.repo.tokens))
	}
}

func TestManagedProviderIssuerFreezeAndSupersededVersion(t *testing.T) {
	fixture := newServiceFixture(t)
	discovery := fakeOIDCDiscovery{}
	fixture.service.discovery = discovery
	command := managedTestCommand("v1", "https://hub-a.example")
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("apply v1: %v", err)
	}
	command.ConnectionVersion = "v2"
	command.Issuer = "https://hub-b.example"
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("issuer change before first identity: %v", err)
	}
	old := managedTestCommand("v1", "https://hub-a.example")
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), old); err == nil {
		t.Fatal("expected superseded v1 replay rejection")
	}
	fixture.repo.identitiesByID[99] = &domain.ExternalIdentity{ID: 99, ProviderCode: "hub:order-admin", ExternalIssuer: "https://hub-b.example", ExternalSubject: "sub", UserID: 1}
	command.ConnectionVersion = "v3"
	command.Issuer = "https://hub-c.example"
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err == nil {
		t.Fatal("expected issuer mutation after first identity rejection")
	}
}

func TestManagedProviderExactReplayDoesNotRequireDiscovery(t *testing.T) {
	fixture := newServiceFixture(t)
	fixture.service.discovery = fakeOIDCDiscovery{}
	command := managedTestCommand("v1", "https://hub.example")
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("apply managed provider: %v", err)
	}
	fixture.service.discovery = fakeOIDCDiscovery{err: errors.New("hub unavailable")}
	if err := fixture.service.ApplyManagedOIDCProvider(context.Background(), command); err != nil {
		t.Fatalf("exact replay required discovery: %v", err)
	}
}

func TestConcurrentFirstBindFreezesManagedIssuer(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := managedTestProvider("order-admin", "https://hub-a.example")
	fixture.repo.providers[provider.ProviderCode] = provider
	fixture.drivers.drivers["oidc"] = fixture.driver
	fixture.driver.profile = domain.ExternalProfile{Subject: "first-sub", Email: "bound@example.com", EmailVerified: true}
	insertStarted := make(chan struct{})
	releaseInsert := make(chan struct{})
	fixture.repo.insertIdentityStarted = insertStarted
	fixture.repo.releaseIdentityInsert = releaseInsert
	fixture.service.transactor = &serialTransactor{}
	fixture.service.discovery = fakeOIDCDiscovery{}

	start, err := fixture.service.StartExternalLogin(context.Background(), facade.StartExternalLoginRequest{ProviderCode: provider.ProviderCode, BindUserID: 101})
	if err != nil {
		t.Fatalf("start first bind: %v", err)
	}
	bindErr := make(chan error, 1)
	go func() {
		_, callbackErr := fixture.service.CompleteExternalCallback(context.Background(), facade.CompleteExternalCallbackRequest{ProviderCode: provider.ProviderCode, Code: "code", State: start.StateID})
		bindErr <- callbackErr
	}()
	<-insertStarted
	applyErr := make(chan error, 1)
	go func() {
		applyErr <- fixture.service.ApplyManagedOIDCProvider(context.Background(), managedTestCommand("v2", "https://hub-b.example"))
	}()
	close(releaseInsert)
	if err := <-bindErr; err != nil {
		t.Fatalf("first bind: %v", err)
	}
	if err := <-applyErr; err == nil {
		t.Fatal("concurrent issuer mutation won after first identity bind")
	}
	if got := fixture.repo.providers[provider.ProviderCode].Issuer; got != "https://hub-a.example" {
		t.Fatalf("issuer=%q changed across first bind", got)
	}
}

func TestGenericProviderAdminRejectsManagedNamespaceAndRecords(t *testing.T) {
	fixture := newServiceFixture(t)
	provider := managedTestProvider("order-admin", "https://hub.example.com")
	fixture.repo.providers[provider.ProviderCode] = provider

	if _, err := fixture.service.CreateProvider(context.Background(), 7, providerSaveRequest(provider.ProviderCode),
		validStepUpProof(StepUpActionExternalLoginProviderCreate, BuildProviderCreateOperationBinding(provider.ProviderCode))); err == nil {
		t.Fatal("generic create accepted managed namespace")
	}
	if _, err := fixture.service.UpdateProvider(context.Background(), 7, provider.ProviderCode, providerUpdateRequest(provider.ProviderCode),
		validStepUpProof(StepUpActionExternalLoginProviderUpdate, BuildProviderUpdateOperationBinding(provider.ProviderCode))); err == nil {
		t.Fatal("generic update accepted managed provider")
	}
	if err := fixture.service.UpdateProviderStatus(context.Background(), 7, provider.ProviderCode, facade.ProviderStatusRequest{Status: domain.ProviderStatusDisabled, Reason: "test"},
		validStepUpProof(StepUpActionExternalLoginProviderStatusChange, BuildProviderStatusOperationBinding(provider.ProviderCode, domain.ProviderStatusDisabled))); err == nil {
		t.Fatal("generic status accepted managed provider")
	}
	if err := fixture.service.RotateClientSecret(context.Background(), 7, provider.ProviderCode, facade.RotateClientSecretRequest{ClientSecret: "new"},
		validStepUpProof(StepUpActionExternalLoginProviderSecretRotate, BuildProviderSecretRotateOperationBinding(provider.ProviderCode))); err == nil {
		t.Fatal("generic secret rotation accepted managed provider")
	}
}

func managedTestProvider(owner, issuer string) *domain.Provider {
	code, _ := domain.ManagedProviderCode(owner)
	return &domain.Provider{
		ID: 10, ProviderCode: code, ProviderName: "Hub OIDC", ProtocolType: domain.ProtocolTypeOIDC, Issuer: issuer,
		AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token", JWKSURI: issuer + "/jwks",
		ClientID: "hub-node-" + owner, ClientSecretCiphertext: "cipher:c2VjcmV0", ClientSecretEDEK: "edek", ClientSecretWrapKeyRef: "key",
		Scopes: []string{"openid", "profile", "email"}, RedirectURI: "https://node.example.com/callback", DisplayName: "Hub",
		DisplayEnabled: true, LoginEnabled: true, BindEnabled: true, AccountAutoCreateEnabled: true, Status: domain.ProviderStatusActive,
		MetadataJSON: `{"managedBy":"hub_control","ownerNodeCode":"` + owner + `","connectionVersion":"v1","connectionHash":"hash","persistLoginTokens":false}`,
	}
}

func managedTestCommand(version, issuer string) facade.ManagedOIDCProviderCommand {
	return facade.ManagedOIDCProviderCommand{OwnerNodeCode: "order-admin", ConnectionVersion: version, Enabled: true, DisplayName: "Hub", Issuer: issuer,
		ClientID: "hub-node-order-admin", ClientSecret: "secret", RedirectURI: "https://node.example.com/callback"}
}

type fakeOIDCDiscovery struct {
	result OIDCDiscoveryResult
	err    error
}

type serialTransactor struct{ mu sync.Mutex }

func (*serialTransactor) Enabled() bool { return true }
func (t *serialTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return fn(ctx)
}

func (f fakeOIDCDiscovery) DiscoverOIDC(_ context.Context, issuer string) (OIDCDiscoveryResult, error) {
	if f.result.Issuer == "" {
		return OIDCDiscoveryResult{Issuer: issuer, AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token", UserinfoEndpoint: issuer + "/userinfo", JWKSURI: issuer + "/jwks"}, f.err
	}
	return f.result, f.err
}
