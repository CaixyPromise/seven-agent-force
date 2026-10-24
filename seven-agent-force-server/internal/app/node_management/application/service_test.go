package application

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	nodedomain "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/domain"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	platformapp "github.com/CaixyPromise/seven-agent/force/internal/app/platform/application"
	platformdomain "github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userapp "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/application"
	userdomain "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
)

func TestSetUserStatusRequiresIdempotencyAndDelegatesMandatoryDisable(t *testing.T) {
	users := &fakeUsers{detail: adminUser(2001, 0)}
	service := newTestService(users, &fakeSessions{}, &fakePolicies{}, nil)
	_, err := service.SetUserStatus(context.Background(), nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "cmd-1", Reason: "security response"})
	if err != nil {
		t.Fatalf("set user status: %v", err)
	}
	if users.managedCalls != 1 || users.lastManaged.Status != nodefacade.UserStatusDisabled {
		t.Fatalf("managed status calls=%d command=%+v", users.managedCalls, users.lastManaged)
	}

	_, err = service.SetUserStatus(context.Background(), nodefacade.SetUserStatusCommand{UserID: "2001", Status: 0, Reason: "missing key"})
	if apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("missing idempotency code=%d err=%v", apperrors.From(err).Code(), err)
	}
}

func TestManagedDisablePersistsBeforeRevokeAndRetryOnlyRepeatsRevoke(t *testing.T) {
	repo := &managedUserRepository{status: nodefacade.UserStatusNormal}
	sessions := &managedSessionFacade{failOnce: true, cutoffChanged: 1}
	service := newManagedUserService(repo)
	service.BindSessions(sessions)
	service.BindManagedSessions(sessions)
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	command := userfacade.SetManagedUserStatusCommand{UserID: 2001, Status: nodefacade.UserStatusDisabled, Cutoff: cutoff, StatusCommandHash: strings.Repeat("a", 64)}

	if _, err := service.SetManagedUserStatus(context.Background(), command); err == nil {
		t.Fatal("first disable must report revoke failure")
	}
	if repo.status != nodefacade.UserStatusDisabled || repo.updateCalls != 1 || sessions.cutoffRevokeCalls != 1 {
		t.Fatalf("first attempt status=%d updates=%d cutoffRevokes=%d", repo.status, repo.updateCalls, sessions.cutoffRevokeCalls)
	}
	changed, err := service.SetManagedUserStatus(context.Background(), command)
	if err != nil {
		t.Fatalf("retry disable: %v", err)
	}
	if changed != 1 {
		t.Fatalf("retry changed=%d want 1 for newly revoked session state", changed)
	}
	if repo.updateCalls != 1 || sessions.cutoffRevokeCalls != 2 {
		t.Fatalf("retry repeated completed effect: updates=%d cutoffRevokes=%d", repo.updateCalls, sessions.cutoffRevokeCalls)
	}
}

func TestManagedUserStatusNoOpReturnsZeroAndUsesAcceptanceCutoff(t *testing.T) {
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	repo := &managedUserRepository{status: nodefacade.UserStatusDisabled}
	sessions := &managedSessionFacade{}
	service := newManagedUserService(repo)
	service.BindManagedSessions(sessions)

	changed, err := service.SetManagedUserStatus(context.Background(), userfacade.SetManagedUserStatusCommand{
		UserID:            2001,
		Status:            nodefacade.UserStatusDisabled,
		ExpectedStatus:    nodefacade.UserStatusDisabled,
		Cutoff:            cutoff,
		StatusCommandHash: strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatalf("managed status no-op: %v", err)
	}
	if changed != 0 {
		t.Fatalf("managed status no-op changed=%d want 0", changed)
	}
	if sessions.cutoffRevokeCalls != 1 || !sessions.lastCutoff.Equal(cutoff) {
		t.Fatalf("cutoff revokes=%d cutoff=%s want %s", sessions.cutoffRevokeCalls, sessions.lastCutoff, cutoff)
	}
}

func TestManagedStatusRejectsSameTargetAtNewerUnrelatedRevision(t *testing.T) {
	repo := &managedUserRepository{status: nodefacade.UserStatusDisabled, version: 3}
	service := newManagedUserService(repo)
	service.BindManagedSessions(&managedSessionFacade{})
	_, err := service.SetManagedUserStatus(context.Background(), userfacade.SetManagedUserStatusCommand{
		UserID:            2001,
		Status:            nodefacade.UserStatusDisabled,
		ExpectedStatus:    nodefacade.UserStatusNormal,
		ExpectedVersion:   0,
		Cutoff:            time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
		StatusCommandHash: strings.Repeat("c", 64),
	})
	if err == nil || apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("same target at newer revision err=%v", err)
	}
	if repo.status != nodefacade.UserStatusDisabled || repo.version != 3 {
		t.Fatalf("stale same-target command mutated status=%d version=%d", repo.status, repo.version)
	}
}

func TestManagedPolicyAppliesFullSafeSnapshotAndPreservesLocalOnlyFields(t *testing.T) {
	deptID := int64(9)
	repo := &managedPolicyRepository{
		platform: platformdomain.Platform{PlatformCode: "seven-admin", PlatformName: "Admin", PlatformType: "ADMIN", IsDefault: true, Status: platformdomain.StatusActive, DefaultDeptID: &deptID, SettingsJSON: `{"private":true}`, BrandJSON: `{"logo":"private"}`},
		methods:  []platformdomain.LoginMethod{{PlatformCode: "seven-admin", MethodType: platformdomain.MethodPassword, DisplayName: "Old Password", DisplayEnabled: true, LoginEnabled: true, MetadataJSON: `{"localMethod":true}`}},
		rules:    []platformdomain.SourceRule{{PlatformCode: "seven-admin", MatchType: platformdomain.MatchHost, MatchValue: "node.example.com", Priority: 1, Status: platformdomain.StatusActive, MetadataJSON: `{"localRule":true}`}},
	}
	sessions := &managedSessionFacade{}
	service := platformapp.NewService(repo, nil, nil, &hookTransactor{})
	service.BindSessions(sessions)
	command := platformfacade.ApplyManagedLoginPolicyCommand{ManagedLoginPolicy: platformfacade.ManagedLoginPolicy{
		PlatformCode: "seven-admin", Status: platformdomain.StatusActive, AllowAutoRegister: true, AllowFormRegister: true,
		LoginMethods: []platformfacade.ManagedLoginMethod{{MethodType: platformdomain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: false}},
		SourceRules:  []platformfacade.ManagedSourceRule{{MatchType: platformdomain.MatchHost, MatchValue: "node.example.com", Priority: 2, Status: platformdomain.StatusActive}},
	}}
	changed, err := service.ApplyManagedLoginPolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("apply managed policy: %v", err)
	}
	if changed != 1 {
		t.Fatalf("first apply changed=%d want 1", changed)
	}
	if repo.platformUpdates != 1 || repo.statusUpdates != 0 || repo.methodReplacements != 1 || repo.ruleReplacements != 1 {
		t.Fatalf("unexpected persistence counts: platform=%d status=%d methods=%d rules=%d", repo.platformUpdates, repo.statusUpdates, repo.methodReplacements, repo.ruleReplacements)
	}
	if repo.updated.SettingsJSON != `{"private":true}` || repo.updated.DefaultDeptID == nil || *repo.updated.DefaultDeptID != deptID || repo.updated.BrandJSON != `{"logo":"private"}` {
		t.Fatalf("local-only fields changed: %+v", repo.updated)
	}
	if repo.methods[0].MetadataJSON != `{"localMethod":true}` || repo.rules[0].MetadataJSON != `{"localRule":true}` {
		t.Fatalf("managed policy lost local metadata: methods=%+v rules=%+v", repo.methods, repo.rules)
	}
	if sessions.methodRevokeCalls != 1 || sessions.platformRevokeCalls != 0 {
		t.Fatalf("unexpected session revokes: methods=%d platform=%d", sessions.methodRevokeCalls, sessions.platformRevokeCalls)
	}

	changed, err = service.ApplyManagedLoginPolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("replay managed policy: %v", err)
	}
	if changed != 0 {
		t.Fatalf("equal replay changed=%d want 0", changed)
	}
	if repo.platformUpdates != 1 || repo.statusUpdates != 0 || repo.methodReplacements != 1 || repo.ruleReplacements != 1 {
		t.Fatalf("equal replay persisted again: platform=%d status=%d methods=%d rules=%d", repo.platformUpdates, repo.statusUpdates, repo.methodReplacements, repo.ruleReplacements)
	}
	if sessions.methodRevokeCalls != 1 || sessions.platformRevokeCalls != 0 {
		t.Fatalf("equal replay revoked again: methods=%d platform=%d", sessions.methodRevokeCalls, sessions.platformRevokeCalls)
	}
}

func TestManagedPolicyCanDisableDefaultAndPersistsStatusColumn(t *testing.T) {
	repo := &managedPolicyRepository{
		platform: platformdomain.Platform{PlatformCode: "seven-admin", PlatformName: "Admin", PlatformType: "ADMIN", IsDefault: true, Status: platformdomain.StatusActive},
		methods:  []platformdomain.LoginMethod{{PlatformCode: "seven-admin", MethodType: platformdomain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
	}
	sessions := &managedSessionFacade{}
	service := platformapp.NewService(repo, nil, nil, &hookTransactor{})
	service.BindSessions(sessions)
	command := platformfacade.ApplyManagedLoginPolicyCommand{ManagedLoginPolicy: platformfacade.ManagedLoginPolicy{
		PlatformCode: "seven-admin", Status: platformdomain.StatusDisabled,
		LoginMethods: []platformfacade.ManagedLoginMethod{{MethodType: platformdomain.MethodPassword, DisplayName: "Password", DisplayEnabled: true, LoginEnabled: true}},
	}}

	changed, err := service.ApplyManagedLoginPolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("managed disable: %v", err)
	}
	if changed != 1 {
		t.Fatalf("managed disable changed=%d want 1", changed)
	}
	if repo.platform.Status != platformdomain.StatusDisabled || repo.statusUpdates != 1 {
		t.Fatalf("status=%d statusUpdates=%d", repo.platform.Status, repo.statusUpdates)
	}
	if sessions.platformRevokeCalls != 1 {
		t.Fatalf("platform revoke calls=%d want 1", sessions.platformRevokeCalls)
	}
	managed, err := service.GetManagedLoginPolicy(context.Background())
	if err != nil || managed == nil || managed.Status != platformdomain.StatusDisabled {
		t.Fatalf("managed read after disable=%+v err=%v", managed, err)
	}
	changed, err = service.ApplyManagedLoginPolicy(context.Background(), command)
	if err != nil {
		t.Fatalf("managed disable replay: %v", err)
	}
	if changed != 0 {
		t.Fatalf("equal disable replay changed=%d want 0", changed)
	}
	if repo.statusUpdates != 1 || sessions.platformRevokeCalls != 1 {
		t.Fatalf("equal disable replay wrote again: status=%d revoke=%d", repo.statusUpdates, sessions.platformRevokeCalls)
	}
}

func TestManagedPolicyReadsAndPreservesMetadataInsideTransaction(t *testing.T) {
	repo := &managedPolicyRepository{
		platform: platformdomain.Platform{PlatformCode: "seven-admin", PlatformName: "Admin", PlatformType: "ADMIN", IsDefault: true, Status: platformdomain.StatusActive},
		methods: []platformdomain.LoginMethod{{PlatformCode: "seven-admin", MethodType: platformdomain.MethodPassword, DisplayName: "Old", DisplayEnabled: true, LoginEnabled: true,
			MetadataJSON: `{"owner":"before"}`}, {PlatformCode: "seven-admin", MethodType: platformdomain.MethodExternalOAuth, ProviderCode: "hidden-provider", DisplayName: "Hidden", DisplayEnabled: false, LoginEnabled: false,
			MetadataJSON: `{"hidden":"local"}`}},
	}
	tx := &hookTransactor{hook: func() { repo.methods[0].MetadataJSON = `{"owner":"concurrent"}` }}
	service := platformapp.NewService(repo, nil, nil, tx)
	command := platformfacade.ApplyManagedLoginPolicyCommand{ManagedLoginPolicy: platformfacade.ManagedLoginPolicy{
		PlatformCode: "seven-admin", Status: platformdomain.StatusActive,
		LoginMethods: []platformfacade.ManagedLoginMethod{{MethodType: platformdomain.MethodPassword, DisplayName: "Managed", DisplayEnabled: true, LoginEnabled: true},
			{MethodType: platformdomain.MethodExternalOAuth, ProviderCode: "hidden-provider", DisplayName: "Hidden", DisplayEnabled: false, LoginEnabled: false}},
	}}

	if _, err := service.ApplyManagedLoginPolicy(context.Background(), command); err != nil {
		t.Fatalf("apply managed policy: %v", err)
	}
	if got := repo.methods[0].MetadataJSON; got != `{"owner":"concurrent"}` {
		t.Fatalf("concurrent local metadata overwritten: %s", got)
	}
	if got := repo.methods[1].MetadataJSON; got != `{"hidden":"local"}` {
		t.Fatalf("hidden method metadata overwritten: %s", got)
	}
	if repo.managedLockReads != 3 {
		t.Fatalf("managed locked reads=%d want 3", repo.managedLockReads)
	}
}

func TestUserAndSessionViewsAreSafeAndExpiredIsEffective(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	users := &fakeUsers{detail: &userfacade.AdminUserVO{ID: 2001, Username: "casey", Nickname: "Casey", Email: "casey@example.com", UserPhone: "13800138000", Avatar: "https://secret/avatar.png", UserProfile: "private", RoleIDs: []int64{7}}}
	expired := now.Add(-time.Minute)
	sessions := &fakeSessions{records: []ssofacade.SessionRecord{{SessionID: "raw-session-id", UserID: 2001, ClientID: "console", ExpiresAt: &expired, Status: "ACTIVE", MetadataJSON: `{"secret":true}`}}}
	service := newTestService(users, sessions, &fakePolicies{}, nil)
	service.now = func() time.Time { return now }

	got, err := service.GetUser(context.Background(), 2001)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got.EmailMasked != "c***@example.com" || got.PhoneMasked != "138****8000" {
		t.Fatalf("unsafe contact mapping: %+v", got)
	}
	page, err := service.ListUserSessions(context.Background(), 2001, nodefacade.SessionPageQuery{Current: 1, Size: 20})
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].Status != nodefacade.SessionStatusExpired || page.Records[0].SessionRef == "raw-session-id" {
		t.Fatalf("unsafe session mapping: %+v", page.Records)
	}
}

func TestListUserSessionsUsesBoundedSSOPage(t *testing.T) {
	sessions := &fakeSessions{records: make([]ssofacade.SessionRecord, 250)}
	for index := range sessions.records {
		sessions.records[index] = ssofacade.SessionRecord{SessionID: "session", UserID: 2001}
	}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	page, err := service.ListUserSessions(context.Background(), 2001, nodefacade.SessionPageQuery{Current: 3, Size: 25})
	if err != nil {
		t.Fatalf("list user sessions: %v", err)
	}
	if page.Total != 250 || len(page.Records) != 25 {
		t.Fatalf("page total=%d records=%d", page.Total, len(page.Records))
	}
	if sessions.listAllCalls != 0 || sessions.pageCalls != 1 || sessions.lastOffset != 50 || sessions.lastLimit != 25 {
		t.Fatalf("unbounded session read: all=%d page=%d offset=%d limit=%d", sessions.listAllCalls, sessions.pageCalls, sessions.lastOffset, sessions.lastLimit)
	}
}

func TestEmailMaskingUsesUnicodeRunes(t *testing.T) {
	if got := maskEmail("凯@example.com"); got != "凯***@example.com" {
		t.Fatalf("maskEmail()=%q want rune-safe mask", got)
	}
}

func TestRevokeSessionRejectsCrossUserReferenceBeforeMutation(t *testing.T) {
	sessions := &fakeSessions{}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	_, err := service.RevokeUserSessions(context.Background(), nodefacade.RevokeUserSessionsCommand{UserID: "2001", SessionRefs: []string{"ref-user-3001"}, IdempotencyKey: "cmd-2", Reason: "incident"})
	if apperrors.From(err).Code() != apperrors.CodeForbidden {
		t.Fatalf("cross-user code=%d err=%v", apperrors.From(err).Code(), err)
	}
	if sessions.revokeCalls != 0 {
		t.Fatalf("revoke calls=%d want 0", sessions.revokeCalls)
	}
}

func TestExplicitSessionRevocationUsesManagedNoAuditPath(t *testing.T) {
	sessions := &fakeSessions{}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)

	result, err := service.RevokeUserSessions(context.Background(), nodefacade.RevokeUserSessionsCommand{
		UserID:         "2001",
		SessionRefs:    []string{"ref-user-2001"},
		IdempotencyKey: "cmd-managed-session",
		Reason:         "incident",
	})
	if err != nil {
		t.Fatalf("revoke explicit session: %v", err)
	}
	if result.ChangedCount != 1 {
		t.Fatalf("changedCount=%d want 1", result.ChangedCount)
	}
	if sessions.managedRevokeCalls != 1 || sessions.revokeCalls != 0 {
		t.Fatalf("managed revokes=%d ordinary revokes=%d", sessions.managedRevokeCalls, sessions.revokeCalls)
	}
}

func TestRevokeAllReplayUsesSessionsPreparedAtFirstAcceptance(t *testing.T) {
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	sessions := &fakeSessions{
		records:        []ssofacade.SessionRecord{{SessionID: "accepted-session", UserID: 2001, LoginAt: &cutoff}},
		createdAt:      map[string]time.Time{"accepted-session": cutoff},
		databaseCutoff: cutoff,
		failNext:       true,
		revoked:        map[string]bool{},
	}
	replay := &preparedReplay{prepared: map[string][]byte{}}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	service.replay = replay
	service.now = func() time.Time { return cutoff }
	command := nodefacade.RevokeUserSessionsCommand{UserID: "2001", All: true, IdempotencyKey: "cmd-stable-set", Reason: "incident"}

	if _, err := service.RevokeUserSessions(context.Background(), command); err == nil {
		t.Fatal("first revoke must expose the simulated session failure")
	}
	later := cutoff.Add(time.Second)
	sessions.records = append(sessions.records, ssofacade.SessionRecord{SessionID: "later-session", UserID: 2001, LoginAt: &later})
	sessions.createdAt["later-session"] = later
	if _, err := service.RevokeUserSessions(context.Background(), command); err != nil {
		t.Fatalf("retry revoke: %v", err)
	}
	if !sessions.revoked["accepted-session"] {
		t.Fatal("session accepted with the command was not revoked")
	}
	if sessions.revoked["later-session"] {
		t.Fatal("replay revoked a session created after command acceptance")
	}
	if sessions.cutoffRevokeCalls != 2 || sessions.revokeCalls != 0 || sessions.listAllCalls != 0 {
		t.Fatalf("all revoke did not use cutoff bulk operation: cutoff=%d explicit=%d listAll=%d", sessions.cutoffRevokeCalls, sessions.revokeCalls, sessions.listAllCalls)
	}
}

func TestRevokeAllPreparationFailureIsFailClosedBeforeMutation(t *testing.T) {
	sessions := &fakeSessions{records: []ssofacade.SessionRecord{{SessionID: "accepted-session", UserID: 2001}}}
	replay := &failingPrepareReplay{}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	service.replay = replay
	_, err := service.RevokeUserSessions(context.Background(), nodefacade.RevokeUserSessionsCommand{UserID: "2001", All: true, IdempotencyKey: "cmd-storage-failure", Reason: "incident"})
	if apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("preparation failure code=%d err=%v", apperrors.From(err).Code(), err)
	}
	if replay.executed || sessions.revokeCalls != 0 || len(sessions.revoked) != 0 {
		t.Fatalf("preparation failure reached mutation: executed=%v calls=%d revoked=%v", replay.executed, sessions.revokeCalls, sessions.revoked)
	}
}

func TestSessionReferenceIsOpaqueAndBoundToNode(t *testing.T) {
	nodeA, err := NewSessionReferenceCodec("node-a", "shared-bearer")
	if err != nil {
		t.Fatalf("node A codec: %v", err)
	}
	nodeB, err := NewSessionReferenceCodec("node-b", "shared-bearer")
	if err != nil {
		t.Fatalf("node B codec: %v", err)
	}
	reference, err := nodeA.Encode(context.Background(), ssofacade.SessionRecord{SessionID: "raw-session-id", UserID: 2001})
	if err != nil {
		t.Fatalf("encode reference: %v", err)
	}
	if strings.Contains(reference, "raw-session-id") || strings.Contains(reference, "2001") {
		t.Fatalf("reference is not opaque: %s", reference)
	}
	decoded, err := nodeA.Decode(context.Background(), reference)
	if err != nil || decoded.SessionID != "raw-session-id" || decoded.UserID != 2001 {
		t.Fatalf("node A decode=%+v err=%v", decoded, err)
	}
	_, err = nodeB.Decode(context.Background(), reference)
	if err == nil {
		t.Fatal("cross-node reference unexpectedly decoded")
	}
	if apperrors.From(err).Code() != apperrors.CodeForbidden {
		t.Fatalf("cross-node reference code=%d err=%v", apperrors.From(err).Code(), err)
	}
}

func TestHubConnectionFailsClosedWhenPortIsUnbound(t *testing.T) {
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, nil)
	_, err := service.ApplyHubConnection(context.Background(), nodefacade.ApplyHubConnectionCommand{ConnectionVersion: "v1", Enabled: true, Issuer: "https://hub.example.com", ClientID: "node-a", ClientSecret: "secret", RedirectURI: "https://node.example.com/callback", IdempotencyKey: "cmd-3", Reason: "provision"})
	if apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("unbound hub port code=%d err=%v", apperrors.From(err).Code(), err)
	}
}

func TestApplyHubConnectionProjectsExactManagedOIDCCommand(t *testing.T) {
	port := &capturingHubConnectionPort{}
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, port)
	command := nodefacade.ApplyHubConnectionCommand{
		ConnectionVersion: "v7", TargetRevision: 7, Enabled: true, DisplayName: "Corporate Hub", Issuer: "https://hub.example.com",
		ClientID: "hub-node-order-admin", ClientSecret: "one-time-secret", RedirectURI: "https://node.example.com/callback",
		IdempotencyKey: "connect-v7", Reason: "provision",
	}
	if _, err := service.ApplyHubConnection(context.Background(), command); err != nil {
		t.Fatalf("apply hub connection: %v", err)
	}
	got := port.command
	if got.ConnectionVersion != command.ConnectionVersion || got.TargetRevision != command.TargetRevision || !got.Enabled || got.DisplayName != command.DisplayName || got.Issuer != command.Issuer ||
		got.ClientID != command.ClientID || got.ClientSecret != command.ClientSecret || got.RedirectURI != command.RedirectURI {
		t.Fatalf("managed projection=%+v", got)
	}
}

type capturingHubConnectionPort struct {
	command nodefacade.ManagedHubConnectionCommand
}

func (p *capturingHubConnectionPort) ApplyHubConnection(_ context.Context, command nodefacade.ManagedHubConnectionCommand) error {
	p.command = command
	return nil
}

func TestAuditContainsOnlyHashesAndAuditFailureLeavesCommandReplayable(t *testing.T) {
	audit := &fakeAudit{err: errors.New("audit unavailable")}
	users := &fakeUsers{detail: adminUser(2001, 0)}
	service := newTestService(users, &fakeSessions{}, &fakePolicies{}, nil)
	service.audit = audit
	command := nodefacade.SetUserStatusCommand{UserID: "2001", Status: 1, IdempotencyKey: "secret-command-key", Reason: "security response"}
	_, err := service.SetUserStatus(context.Background(), command)
	if !errors.Is(err, audit.err) {
		t.Fatalf("audit error=%v want %v", err, audit.err)
	}
	if strings.Contains(audit.entry.RequestParams, command.IdempotencyKey) || strings.Contains(audit.entry.RequestParams, command.Reason) {
		t.Fatalf("audit leaked command data: %s", audit.entry.RequestParams)
	}
}

func TestNodeManagementAuditUsesCanonicalTraceFromContext(t *testing.T) {
	const traceID = "56565656565656565656565656565656"
	audit := &fakeAudit{}
	users := &fakeUsers{detail: adminUser(2001, 0)}
	service := newTestService(users, &fakeSessions{}, &fakePolicies{}, nil)
	service.audit = audit
	ctx := xcontext.WithTraceID(context.Background(), traceID)

	_, err := service.SetUserStatus(ctx, nodefacade.SetUserStatusCommand{
		UserID: "2001", Status: nodefacade.UserStatusDisabled,
		IdempotencyKey: "trace-audit", Reason: "trace correlation acceptance",
	})
	if err != nil {
		t.Fatalf("set user status: %v", err)
	}
	if audit.entry.TraceID != traceID {
		t.Fatalf("audit trace=%q, want %q", audit.entry.TraceID, traceID)
	}
}

func TestNoOpManagedCommandsReturnAndAuditZeroChangedCount(t *testing.T) {
	users := &fakeUsers{detail: adminUser(2001, nodefacade.UserStatusDisabled)}
	policies := &fakePolicies{}
	audit := &fakeAudit{}
	service := newTestService(users, &fakeSessions{}, policies, nil)
	service.audit = audit

	statusResult, err := service.SetUserStatus(context.Background(), nodefacade.SetUserStatusCommand{
		UserID: "2001", Status: nodefacade.UserStatusDisabled,
		IdempotencyKey: "status-no-op", Reason: "confirm disabled",
	})
	if err != nil {
		t.Fatalf("status no-op: %v", err)
	}
	if statusResult.ChangedCount != 0 || !strings.Contains(audit.entry.RequestParams, `"changedCount":0`) {
		t.Fatalf("status result=%+v audit=%s", statusResult, audit.entry.RequestParams)
	}

	policyResult, err := service.ApplyLoginPolicy(context.Background(), nodefacade.ApplyLoginPolicyCommand{
		ManagedLoginPolicy: nodefacade.ManagedLoginPolicy{PlatformCode: "seven-admin"},
		IdempotencyKey:     "policy-no-op", Reason: "confirm policy",
	})
	if err != nil {
		t.Fatalf("policy no-op: %v", err)
	}
	if policyResult.ChangedCount != 0 || !strings.Contains(audit.entry.RequestParams, `"changedCount":0`) {
		t.Fatalf("policy result=%+v audit=%s", policyResult, audit.entry.RequestParams)
	}
}

func TestStatusCommandReplayUsesStableAcceptanceCutoff(t *testing.T) {
	acceptedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	now := acceptedAt
	effects := &statusCutoffEffect{
		status:   nodefacade.UserStatusNormal,
		sessions: map[string]time.Time{"accepted": acceptedAt},
		active:   map[string]bool{"accepted": true},
	}
	replay := &abandonedResultReplay{prepared: map[string][]byte{}}
	sessions := &fakeSessions{databaseCutoff: acceptedAt}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	service.managedUsers = effects
	service.replay = replay
	service.now = func() time.Time { return now }
	command := nodefacade.SetUserStatusCommand{
		UserID: "2001", Status: nodefacade.UserStatusDisabled,
		IdempotencyKey: "stable-status-cutoff", Reason: "incident",
	}

	if _, err := service.SetUserStatus(context.Background(), command); apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("first status command error=%v", err)
	}
	effects.status = nodefacade.UserStatusNormal
	now = acceptedAt.Add(time.Minute)
	effects.sessions["later"] = now
	effects.active["later"] = true
	result, err := service.SetUserStatus(context.Background(), command)
	if err != nil {
		t.Fatalf("status replay: %v", err)
	}
	if result.ChangedCount != 1 || sessions.captureCutoffCalls != 1 || len(effects.cutoffs) != 2 || !effects.cutoffs[0].Equal(acceptedAt) || !effects.cutoffs[1].Equal(acceptedAt) {
		t.Fatalf("result=%+v cutoffs=%v", result, effects.cutoffs)
	}
	if effects.active["accepted"] || !effects.active["later"] {
		t.Fatalf("stable cutoff revoked wrong sessions: %+v", effects.active)
	}
}

func TestOlderStatusCommandCannotOverwriteNewerCommittedCommand(t *testing.T) {
	repo := &managedUserRepository{status: nodefacade.UserStatusNormal}
	sessions := &managedSessionFacade{}
	managedUsers := newManagedUserService(repo)
	managedUsers.BindManagedSessions(sessions)
	replay := &abandonedResultReplay{prepared: map[string][]byte{}}
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, nil)
	service.managedUsers = managedUsers
	service.replay = replay

	disable := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "command-a", Reason: "disable"}
	if _, err := service.SetUserStatus(context.Background(), disable); apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("first disable error=%v", err)
	}
	enable := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusNormal, IdempotencyKey: "command-b", Reason: "enable"}
	if _, err := service.SetUserStatus(context.Background(), enable); err != nil {
		t.Fatalf("newer enable: %v", err)
	}
	if repo.status != nodefacade.UserStatusNormal || repo.version != 2 {
		t.Fatalf("after enable status=%d version=%d", repo.status, repo.version)
	}
	if _, err := service.SetUserStatus(context.Background(), disable); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("stale disable code=%d err=%v", apperrors.From(err).Code(), err)
	}
	if repo.status != nodefacade.UserStatusNormal || repo.version != 2 {
		t.Fatalf("stale retry overwrote newer state: status=%d version=%d", repo.status, repo.version)
	}
}

func TestDelayedStatusCommandFailsAfterNewerNoOpIntent(t *testing.T) {
	repo := &managedUserRepository{status: nodefacade.UserStatusNormal}
	managedUsers := newManagedUserService(repo)
	managedUsers.BindManagedSessions(&managedSessionFacade{})
	replay := &abandonBeforeExecutionReplay{prepared: map[string][]byte{}, abandonKey: "command-a"}
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, nil)
	service.managedUsers = managedUsers
	service.replay = replay

	disable := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "command-a", Reason: "disable"}
	if _, err := service.SetUserStatus(context.Background(), disable); apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("first delayed disable error=%v", err)
	}
	noOp := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusNormal, IdempotencyKey: "command-b", Reason: "confirm normal"}
	if result, err := service.SetUserStatus(context.Background(), noOp); err != nil || result.ChangedCount != 0 {
		t.Fatalf("newer no-op result=%+v err=%v", result, err)
	}
	if repo.status != nodefacade.UserStatusNormal || repo.version != 1 {
		t.Fatalf("newer no-op did not advance intent version: status=%d version=%d", repo.status, repo.version)
	}
	if _, err := service.SetUserStatus(context.Background(), disable); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("delayed status command error=%v", err)
	}
	if repo.status != nodefacade.UserStatusNormal || repo.version != 1 {
		t.Fatalf("delayed status command overwrote newer no-op: status=%d version=%d", repo.status, repo.version)
	}
}

func TestDifferentStatusCommandScopeCannotReplaySameTarget(t *testing.T) {
	repo := &managedUserRepository{status: nodefacade.UserStatusNormal}
	sessions := &managedSessionFacade{cutoffChanged: 1}
	managedUsers := newManagedUserService(repo)
	managedUsers.BindManagedSessions(sessions)
	replay := &abandonBeforeExecutionReplay{prepared: map[string][]byte{}, abandonKey: "command-a"}
	audit := &fakeAudit{}
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, nil)
	service.managedUsers = managedUsers
	service.replay = replay
	service.audit = audit

	commandA := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "command-a", Reason: "disable"}
	if _, err := service.SetUserStatus(context.Background(), commandA); apperrors.From(err).Code() != apperrors.CodeServiceUnavailable {
		t.Fatalf("delayed command A error=%v", err)
	}
	commandB := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "command-b", Reason: "disable"}
	if _, err := service.SetUserStatus(context.Background(), commandB); err != nil {
		t.Fatalf("newer command B: %v", err)
	}
	if repo.status != nodefacade.UserStatusDisabled || repo.version != 1 || repo.commandHash == "" {
		t.Fatalf("newer command state status=%d version=%d hash=%q", repo.status, repo.version, repo.commandHash)
	}
	if _, err := service.SetUserStatus(context.Background(), commandA); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("older same-target command error=%v", err)
	}
	if repo.status != nodefacade.UserStatusDisabled || repo.version != 1 || sessions.cutoffRevokeCalls != 1 || audit.calls != 1 {
		t.Fatalf("older command mutated status=%d version=%d revokes=%d audits=%d", repo.status, repo.version, sessions.cutoffRevokeCalls, audit.calls)
	}
}

func TestManagedCommandsCaptureCutoffFromSSOPortInsteadOfServiceClock(t *testing.T) {
	databaseCutoff := time.Date(2026, 7, 12, 8, 30, 0, 654321000, time.UTC)
	sessions := &fakeSessions{databaseCutoff: databaseCutoff}
	service := newTestService(&fakeUsers{detail: adminUser(2001, nodefacade.UserStatusNormal)}, sessions, &fakePolicies{}, nil)
	service.now = func() time.Time { return databaseCutoff.Add(24 * time.Hour) }

	if _, err := service.SetUserStatus(context.Background(), nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "db-cutoff-status", Reason: "incident"}); err != nil {
		t.Fatalf("status command: %v", err)
	}
	if _, err := service.RevokeUserSessions(context.Background(), nodefacade.RevokeUserSessionsCommand{UserID: "2001", All: true, IdempotencyKey: "db-cutoff-all", Reason: "incident"}); err != nil {
		t.Fatalf("all-session command: %v", err)
	}
	if sessions.captureCutoffCalls != 2 || !sessions.lastCutoff.Equal(databaseCutoff) {
		t.Fatalf("database cutoff calls=%d cutoff=%s want=%s", sessions.captureCutoffCalls, sessions.lastCutoff, databaseCutoff)
	}
}

func TestListUserSessionsRejectsOffsetOverflow(t *testing.T) {
	sessions := &fakeSessions{}
	service := newTestService(&fakeUsers{}, sessions, &fakePolicies{}, nil)
	maxInt := int64(^uint(0) >> 1)
	boundaryCurrent := maxInt/100 + 1
	if _, err := service.ListUserSessions(context.Background(), 2001, nodefacade.SessionPageQuery{Current: boundaryCurrent, Size: 100}); err != nil {
		t.Fatalf("boundary page: %v", err)
	}
	if sessions.lastOffset != int((boundaryCurrent-1)*100) {
		t.Fatalf("boundary offset=%d", sessions.lastOffset)
	}
	pageCalls := sessions.pageCalls
	_, err := service.ListUserSessions(context.Background(), 2001, nodefacade.SessionPageQuery{Current: boundaryCurrent + 1, Size: 100})
	if err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("overflow err=%v", err)
	}
	if sessions.pageCalls != pageCalls {
		t.Fatal("overflow page reached the downstream repository")
	}
}

func TestListUsersRejectsInt64OffsetOverflow(t *testing.T) {
	users := &fakeUsers{}
	service := newTestService(users, &fakeSessions{}, &fakePolicies{}, nil)
	_, err := service.ListUsers(context.Background(), nodefacade.UserPageQuery{Current: math.MaxInt64, Size: 100})
	if err == nil || apperrors.From(err).Code() != apperrors.CodeParamsError {
		t.Fatalf("overflow err=%v", err)
	}
	if users.queryCalls != 0 {
		t.Fatal("overflow page reached the downstream user query")
	}
}

func TestBusinessCompletionIsStoredBeforeAuditAndReplayOnlyRetriesAudit(t *testing.T) {
	audit := &fakeAudit{err: errors.New("audit unavailable")}
	effects := &disableEffect{status: nodefacade.UserStatusNormal, sessions: map[string]bool{"accepted": true}}
	service := newTestService(&fakeUsers{}, &fakeSessions{}, &fakePolicies{}, nil)
	service.managedUsers = effects
	service.replay = &memoryReplay{}
	service.audit = audit
	command := nodefacade.SetUserStatusCommand{UserID: "2001", Status: nodefacade.UserStatusDisabled, IdempotencyKey: "cmd-audit-order", Reason: "incident"}

	if _, err := service.SetUserStatus(context.Background(), command); !errors.Is(err, audit.err) {
		t.Fatalf("first command error=%v want audit failure", err)
	}
	if effects.calls != 1 || effects.status != nodefacade.UserStatusDisabled || effects.sessions["accepted"] {
		t.Fatalf("first business effect incomplete: %+v", effects)
	}
	effects.status = nodefacade.UserStatusNormal
	effects.sessions["later"] = true
	audit.err = nil
	result, err := service.SetUserStatus(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if result == nil || !result.Replayed {
		t.Fatalf("replay result=%+v", result)
	}
	if effects.calls != 1 || effects.status != nodefacade.UserStatusNormal || !effects.sessions["later"] {
		t.Fatalf("old command repeated business mutation: %+v", effects)
	}
}

func TestAuditCallerFitsOperationLogUsernameColumn(t *testing.T) {
	audit := &fakeAudit{}
	service := NewService(Config{NodeCode: "node-a", CallerIDHash: strings.Repeat("a", 64)}, Dependencies{Audit: audit})
	if err := service.writeAudit(context.Background(), "NODE_USER_STATUS", "PUT", "/internal/node/v1/users/1/status", "1", "key", 1); err != nil {
		t.Fatal(err)
	}
	if got := len(audit.entry.UserName); got > 64 {
		t.Fatalf("audit username length=%d exceeds sys_operation_log.userName", got)
	}
	if strings.Contains(audit.entry.UserName, "bearer") {
		t.Fatalf("audit username contains secret material: %q", audit.entry.UserName)
	}
}

func newTestService(users *fakeUsers, sessions *fakeSessions, policies *fakePolicies, hub nodefacade.HubConnectionPort) *Service {
	return NewService(Config{NodeCode: "order-admin", Version: "1.0.0", CallerIDHash: "caller-hash"}, Dependencies{Users: users, ManagedUsers: users, Sessions: sessions, Policies: policies, HubConnection: hub, Replay: directReplay{}, Audit: &fakeAudit{}, SessionRefs: fakeSessionRefs{}})
}

type directReplay struct{}

func (directReplay) Execute(ctx context.Context, _ nodedomain.CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	result, err := operation(ctx)
	return result, false, err
}

func (directReplay) Prepare(ctx context.Context, _ nodedomain.CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	return prepare(ctx)
}

type preparedReplay struct {
	prepared map[string][]byte
}

type memoryReplay struct {
	result []byte
}

type abandonedResultReplay struct {
	prepared map[string][]byte
	calls    int
}

type abandonBeforeExecutionReplay struct {
	prepared   map[string][]byte
	abandonKey string
	abandoned  bool
}

func (r *abandonBeforeExecutionReplay) Prepare(ctx context.Context, metadata nodedomain.CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	if value, ok := r.prepared[metadata.IdempotencyKey]; ok {
		return append([]byte(nil), value...), nil
	}
	value, err := prepare(ctx)
	if err == nil {
		r.prepared[metadata.IdempotencyKey] = append([]byte(nil), value...)
	}
	return value, err
}

func (r *abandonBeforeExecutionReplay) Execute(ctx context.Context, metadata nodedomain.CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	if metadata.IdempotencyKey == r.abandonKey && !r.abandoned {
		r.abandoned = true
		return nil, false, apperrors.ServiceUnavailable("command execution abandoned")
	}
	result, err := operation(ctx)
	return result, false, err
}

func (r *abandonedResultReplay) Prepare(ctx context.Context, metadata nodedomain.CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	if value, ok := r.prepared[metadata.IdempotencyKey]; ok {
		return append([]byte(nil), value...), nil
	}
	value, err := prepare(ctx)
	if err == nil {
		r.prepared[metadata.IdempotencyKey] = append([]byte(nil), value...)
	}
	return value, err
}

func (r *abandonedResultReplay) Execute(ctx context.Context, _ nodedomain.CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	r.calls++
	result, err := operation(ctx)
	if err != nil {
		return nil, false, err
	}
	if r.calls == 1 {
		return nil, false, apperrors.ServiceUnavailable("command result unavailable")
	}
	return result, false, nil
}

func (r *memoryReplay) Execute(ctx context.Context, _ nodedomain.CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	if r.result != nil {
		return append([]byte(nil), r.result...), true, nil
	}
	result, err := operation(ctx)
	if err == nil {
		r.result = append([]byte(nil), result...)
	}
	return result, false, err
}

func (*memoryReplay) Prepare(ctx context.Context, _ nodedomain.CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	return prepare(ctx)
}

type failingPrepareReplay struct {
	executed bool
}

func (r *failingPrepareReplay) Prepare(context.Context, nodedomain.CommandMetadata, func(context.Context) ([]byte, error)) ([]byte, error) {
	return nil, apperrors.ServiceUnavailable("prepared snapshot unavailable")
}

func (r *failingPrepareReplay) Execute(context.Context, nodedomain.CommandMetadata, func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	r.executed = true
	return nil, false, nil
}

func (r *preparedReplay) Execute(ctx context.Context, _ nodedomain.CommandMetadata, operation func(context.Context) ([]byte, error)) ([]byte, bool, error) {
	result, err := operation(ctx)
	return result, false, err
}

func (r *preparedReplay) Prepare(ctx context.Context, metadata nodedomain.CommandMetadata, prepare func(context.Context) ([]byte, error)) ([]byte, error) {
	if value, ok := r.prepared[metadata.IdempotencyKey]; ok {
		return append([]byte(nil), value...), nil
	}
	value, err := prepare(ctx)
	if err != nil {
		return nil, err
	}
	r.prepared[metadata.IdempotencyKey] = append([]byte(nil), value...)
	return value, nil
}

type fakeUsers struct {
	detail       *userfacade.AdminUserVO
	queryCalls   int
	managedCalls int
	lastManaged  userfacade.SetManagedUserStatusCommand
}

type disableEffect struct {
	status   int
	sessions map[string]bool
	calls    int
}

func (e *disableEffect) SetManagedUserStatus(_ context.Context, command userfacade.SetManagedUserStatusCommand) (int64, error) {
	e.calls++
	e.status = command.Status
	if command.Status == nodefacade.UserStatusDisabled {
		for sessionID := range e.sessions {
			e.sessions[sessionID] = false
		}
	}
	return 1, nil
}

func (e *disableEffect) GetManagedUserStatusSnapshot(context.Context, int64) (*userfacade.ManagedUserStatusSnapshot, error) {
	return &userfacade.ManagedUserStatusSnapshot{Status: e.status}, nil
}

type statusCutoffEffect struct {
	status   int
	sessions map[string]time.Time
	active   map[string]bool
	cutoffs  []time.Time
}

func (e *statusCutoffEffect) SetManagedUserStatus(_ context.Context, command userfacade.SetManagedUserStatusCommand) (int64, error) {
	changed := e.status != command.Status
	e.status = command.Status
	e.cutoffs = append(e.cutoffs, command.Cutoff)
	for sessionID, createdAt := range e.sessions {
		if e.active[sessionID] && !createdAt.After(command.Cutoff) {
			e.active[sessionID] = false
			changed = true
		}
	}
	if changed {
		return 1, nil
	}
	return 0, nil
}

func (e *statusCutoffEffect) GetManagedUserStatusSnapshot(context.Context, int64) (*userfacade.ManagedUserStatusSnapshot, error) {
	return &userfacade.ManagedUserStatusSnapshot{Status: e.status}, nil
}

func (f *fakeUsers) QueryUsers(context.Context, userfacade.AdminUserQuery) (*userfacade.PageResult[userfacade.AdminUserVO], error) {
	f.queryCalls++
	records := []userfacade.AdminUserVO{}
	if f.detail != nil {
		records = append(records, *f.detail)
	}
	return &userfacade.PageResult[userfacade.AdminUserVO]{Current: 1, Size: 20, Total: int64(len(records)), Records: records}, nil
}

func (f *fakeUsers) GetManagedUserStatusSnapshot(context.Context, int64) (*userfacade.ManagedUserStatusSnapshot, error) {
	status := nodefacade.UserStatusNormal
	if f.detail != nil {
		status = f.detail.Status
	}
	return &userfacade.ManagedUserStatusSnapshot{Status: status, Version: 0}, nil
}
func (f *fakeUsers) GetAdminUser(context.Context, int64) (*userfacade.AdminUserVO, error) {
	return f.detail, nil
}
func (f *fakeUsers) SetManagedUserStatus(_ context.Context, command userfacade.SetManagedUserStatusCommand) (int64, error) {
	f.managedCalls++
	f.lastManaged = command
	changed := f.detail == nil || f.detail.Status != command.Status
	if f.detail != nil {
		f.detail.Status = command.Status
	}
	if changed {
		return 1, nil
	}
	return 0, nil
}

type fakeSessions struct {
	records            []ssofacade.SessionRecord
	revokeCalls        int
	managedRevokeCalls int
	failNext           bool
	revoked            map[string]bool
	listAllCalls       int
	pageCalls          int
	lastOffset         int
	lastLimit          int
	cutoffRevokeCalls  int
	captureCutoffCalls int
	databaseCutoff     time.Time
	lastCutoff         time.Time
	createdAt          map[string]time.Time
}

func (f *fakeSessions) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	f.listAllCalls++
	return f.records, nil
}
func (f *fakeSessions) CountSessionsByUserID(context.Context, int64) (int64, error) {
	return int64(len(f.records)), nil
}
func (f *fakeSessions) ListSessionsByUserIDPage(_ context.Context, _ int64, offset, limit int) ([]ssofacade.SessionRecord, error) {
	f.pageCalls++
	f.lastOffset = offset
	f.lastLimit = limit
	if offset < 0 || offset >= len(f.records) {
		return nil, nil
	}
	end := offset + limit
	if end > len(f.records) {
		end = len(f.records)
	}
	return append([]ssofacade.SessionRecord(nil), f.records[offset:end]...), nil
}
func (f *fakeSessions) RevokeSession(_ context.Context, sessionID string) (bool, error) {
	f.revokeCalls++
	return f.revoke(sessionID)
}
func (f *fakeSessions) RevokeManagedSession(_ context.Context, sessionID string) (bool, error) {
	f.managedRevokeCalls++
	return f.revoke(sessionID)
}

func (f *fakeSessions) CaptureManagedSessionCutoff(context.Context) (time.Time, error) {
	f.captureCutoffCalls++
	if f.databaseCutoff.IsZero() {
		f.databaseCutoff = time.Date(2026, 7, 12, 8, 30, 0, 654321000, time.UTC)
	}
	return f.databaseCutoff, nil
}
func (f *fakeSessions) revoke(sessionID string) (bool, error) {
	if f.failNext {
		f.failNext = false
		return false, errors.New("revoke unavailable")
	}
	if f.revoked == nil {
		f.revoked = map[string]bool{}
	}
	if f.revoked[sessionID] {
		return false, nil
	}
	f.revoked[sessionID] = true
	return true, nil
}
func (f *fakeSessions) RevokeSessionsByUserID(_ context.Context, userID int64) (int64, error) {
	f.revokeCalls++
	if f.failNext {
		f.failNext = false
		return 0, errors.New("revoke unavailable")
	}
	if f.revoked == nil {
		f.revoked = map[string]bool{}
	}
	var changed int64
	for _, record := range f.records {
		if record.UserID == userID && !f.revoked[record.SessionID] {
			f.revoked[record.SessionID] = true
			changed++
		}
	}
	return changed, nil
}

func (f *fakeSessions) RevokeSessionsByUserIDAtOrBefore(_ context.Context, userID int64, cutoff time.Time) (int64, error) {
	f.cutoffRevokeCalls++
	f.lastCutoff = cutoff
	if f.failNext {
		f.failNext = false
		return 0, errors.New("revoke unavailable")
	}
	if f.revoked == nil {
		f.revoked = map[string]bool{}
	}
	var changed int64
	for _, record := range f.records {
		createdAt, known := f.createdAt[record.SessionID]
		if !known && record.LoginAt != nil {
			createdAt = *record.LoginAt
		}
		if record.UserID == userID && !createdAt.IsZero() && !createdAt.After(cutoff) && !f.revoked[record.SessionID] {
			f.revoked[record.SessionID] = true
			changed++
		}
	}
	return changed, nil
}

type fakePolicies struct {
	changed int64
}

func (*fakePolicies) GetManagedLoginPolicy(context.Context) (*platformfacade.ManagedLoginPolicy, error) {
	return &platformfacade.ManagedLoginPolicy{PlatformCode: "seven-admin"}, nil
}
func (f *fakePolicies) ApplyManagedLoginPolicy(context.Context, platformfacade.ApplyManagedLoginPolicyCommand) (int64, error) {
	return f.changed, nil
}

type fakeSessionRefs struct{}

func (fakeSessionRefs) Encode(_ context.Context, record ssofacade.SessionRecord) (string, error) {
	return "ref-user-" + nodefacade.FormatID(record.UserID), nil
}
func (fakeSessionRefs) Decode(_ context.Context, ref string) (nodefacade.SessionReference, error) {
	if ref == "ref-user-3001" {
		return nodefacade.SessionReference{UserID: 3001, SessionID: "raw-other"}, nil
	}
	return nodefacade.SessionReference{UserID: 2001, SessionID: "raw-session-id"}, nil
}

type fakeAudit struct {
	entry adminfacade.OperationLogEntry
	err   error
	calls int
}

type managedUserRepository struct {
	userdomain.Repository
	status      int
	version     uint64
	commandHash string
	updateCalls int
}

type allowUserRoleAssignments struct {
	authorizationfacade.UserRoleAssignmentFacade
}

func (allowUserRoleAssignments) GuardUserDeactivation(context.Context, int64) error {
	return nil
}

func newManagedUserService(repo userdomain.Repository) *userapp.Service {
	service := userapp.NewService(repo, userdomain.NewService(), nil, nil)
	service.BindRoleAssignments(allowUserRoleAssignments{})
	return service
}

func (r *managedUserRepository) FindAdminUserByID(context.Context, int64) (*userdomain.AdminUserRecord, error) {
	return &userdomain.AdminUserRecord{ID: 2001, Status: r.status, StatusVersion: r.version, StatusCommandHash: r.commandHash}, nil
}

func (r *managedUserRepository) UpdateLockState(_ context.Context, _ int64, status int, _ *time.Time) error {
	r.status = status
	r.version++
	r.commandHash = ""
	r.updateCalls++
	return nil
}

func (r *managedUserRepository) CompareAndSetManagedUserStatus(_ context.Context, _ int64, expectedStatus int, expectedVersion uint64, status int, _ *time.Time, commandHash string) (bool, error) {
	if r.status != expectedStatus || r.version != expectedVersion {
		return false, nil
	}
	r.status = status
	r.version++
	r.commandHash = commandHash
	r.updateCalls++
	return true, nil
}

type managedSessionFacade struct {
	ssofacade.SessionFacade
	failOnce            bool
	revokeCalls         int
	methodRevokeCalls   int
	platformRevokeCalls int
	cutoffRevokeCalls   int
	lastCutoff          time.Time
	cutoffChanged       int64
}

func (*managedSessionFacade) ListSessionsByUserIDPage(context.Context, int64, int, int) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}

func (*managedSessionFacade) CountSessionsByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}

func (*managedSessionFacade) RevokeSession(context.Context, string) (bool, error) {
	return false, nil
}

func (*managedSessionFacade) RevokeManagedSession(context.Context, string) (bool, error) {
	return false, nil
}

func (*managedSessionFacade) CaptureManagedSessionCutoff(context.Context) (time.Time, error) {
	return time.Date(2026, 7, 12, 8, 30, 0, 654321000, time.UTC), nil
}

func (s *managedSessionFacade) RevokeSessionsByUserIDAtOrBefore(_ context.Context, _ int64, cutoff time.Time) (int64, error) {
	s.cutoffRevokeCalls++
	s.lastCutoff = cutoff
	if s.failOnce {
		s.failOnce = false
		return 0, errors.New("revoke unavailable")
	}
	return s.cutoffChanged, nil
}

type managedPolicyRepository struct {
	platformdomain.Repository
	platform           platformdomain.Platform
	updated            platformdomain.Platform
	statusUpdates      int
	platformUpdates    int
	methodReplacements int
	ruleReplacements   int
	methods            []platformdomain.LoginMethod
	rules              []platformdomain.SourceRule
	managedLockReads   int
}

func (r *managedPolicyRepository) FindDefaultPlatform(context.Context) (*platformdomain.Platform, error) {
	copy := r.platform
	return &copy, nil
}

func (r *managedPolicyRepository) ListActivePlatforms(context.Context) ([]platformdomain.Platform, error) {
	return []platformdomain.Platform{r.platform}, nil
}

func (r *managedPolicyRepository) FindManagedDefaultPlatform(context.Context) (*platformdomain.Platform, error) {
	copy := r.platform
	return &copy, nil
}

func (r *managedPolicyRepository) FindManagedDefaultPlatformForUpdate(context.Context) (*platformdomain.Platform, error) {
	r.managedLockReads++
	copy := r.platform
	return &copy, nil
}

func (r *managedPolicyRepository) ListLoginMethods(context.Context, string) ([]platformdomain.LoginMethod, error) {
	return append([]platformdomain.LoginMethod(nil), r.methods...), nil
}

func (r *managedPolicyRepository) ListManagedLoginMethods(context.Context, string) ([]platformdomain.LoginMethod, error) {
	return append([]platformdomain.LoginMethod(nil), r.methods...), nil
}

func (r *managedPolicyRepository) ListManagedLoginMethodsForUpdate(context.Context, string) ([]platformdomain.LoginMethod, error) {
	r.managedLockReads++
	return append([]platformdomain.LoginMethod(nil), r.methods...), nil
}

func (r *managedPolicyRepository) ListManagedSourceRulesForUpdate(context.Context, string) ([]platformdomain.SourceRule, error) {
	r.managedLockReads++
	return append([]platformdomain.SourceRule(nil), r.rules...), nil
}

func (r *managedPolicyRepository) ListManagedSourceRules(context.Context, string) ([]platformdomain.SourceRule, error) {
	return append([]platformdomain.SourceRule(nil), r.rules...), nil
}

func (*managedPolicyRepository) ListAvailableExternalProviderCodes(context.Context, []string) ([]string, error) {
	return nil, nil
}

func (*managedPolicyRepository) ListManagedExternalProviderCodes(_ context.Context, providerCodes []string) ([]string, error) {
	return append([]string(nil), providerCodes...), nil
}

func (r *managedPolicyRepository) ListSourceRules(context.Context, string) ([]platformdomain.SourceRule, error) {
	return append([]platformdomain.SourceRule(nil), r.rules...), nil
}

func (r *managedPolicyRepository) UpdatePlatform(_ context.Context, platform platformdomain.Platform, _ int64) error {
	r.updated = platform
	r.platform = platform
	r.platformUpdates++
	return nil
}

func (r *managedPolicyRepository) UpdatePlatformStatus(_ context.Context, _ string, status int, _ int64) error {
	r.platform.Status = status
	r.statusUpdates++
	return nil
}

func (r *managedPolicyRepository) ReplaceLoginMethods(_ context.Context, _ string, methods []platformdomain.LoginMethod, _ int64) error {
	r.methods = append([]platformdomain.LoginMethod(nil), methods...)
	r.methodReplacements++
	return nil
}

func (r *managedPolicyRepository) ReplaceSourceRules(_ context.Context, _ string, rules []platformdomain.SourceRule, _ int64) error {
	r.rules = append([]platformdomain.SourceRule(nil), rules...)
	r.ruleReplacements++
	return nil
}

func (s *managedSessionFacade) RevokeSessionsByUserID(context.Context, int64) (int64, error) {
	s.revokeCalls++
	if s.failOnce {
		s.failOnce = false
		return 0, errors.New("revoke unavailable")
	}
	return 1, nil
}

func (s *managedSessionFacade) RevokeSessionsByPlatformCode(context.Context, string) (int64, error) {
	s.platformRevokeCalls++
	return 1, nil
}

func (s *managedSessionFacade) RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string) (int64, error) {
	s.methodRevokeCalls++
	return 1, nil
}

func (f *fakeAudit) SaveLog(_ context.Context, entry adminfacade.OperationLogEntry) error {
	f.entry = entry
	f.calls++
	return f.err
}

func adminUser(id int64, status int) *userfacade.AdminUserVO {
	return &userfacade.AdminUserVO{ID: id, Username: "casey", Nickname: "Casey", Status: status}
}

type hookTransactor struct{ hook func() }

func (*hookTransactor) Enabled() bool { return true }

func (t *hookTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	if t.hook != nil {
		t.hook()
	}
	return fn(ctx)
}
