package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/domain"
	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	hubinfra "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/infrastructure"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
)

func TestListNodeUsersDoesNotPersistRemoteUsers(t *testing.T) {
	repo := newFakeRepository(activeNode())
	remote := &fakeNodeClient{users: &nodefacade.UserPage{Current: 1, Size: 20, Total: 1, Records: []nodefacade.UserSummary{{UserID: "42", Username: "remote"}}}}
	service := newTestService(repo, remote, &fakeManagedSSO{})

	page, err := service.ListNodeUsers(context.Background(), "order-admin", nodefacade.UserPageQuery{Current: 1, Size: 20})
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("ListNodeUsers() page=%+v err=%v", page, err)
	}
	if repo.remoteUserWrites != 0 {
		t.Fatalf("remote user writes=%d want 0", repo.remoteUserWrites)
	}
}

func TestSaveNodePreservesSecretsAndRejectsIssuerChangeAfterActive(t *testing.T) {
	node := activeNode()
	node.ManagementBearer = domain.EncryptedSecret{Ciphertext: "old-cipher", EDEK: "old-edek", WrapKeyRef: "old-key"}
	repo := newFakeRepository(node)
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})

	_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
		OriginalNodeCode:  node.NodeCode,
		NodeCode:          node.NodeCode,
		NodeName:          "Order Admin 2",
		Status:            domain.NodeStatusEnabled,
		DiscoveryType:     domain.DiscoveryStatic,
		ManagementBaseURL: "https://node.example.com:9443",
		HubIssuer:         "https://other-hub.example.com",
	})
	if err == nil {
		t.Fatal("ACTIVE issuer change must fail")
	}
	if got := repo.nodes[node.NodeCode].ManagementBearer; got != node.ManagementBearer {
		t.Fatalf("omitted secret replaced: got=%+v want=%+v", got, node.ManagementBearer)
	}
}

func TestNodeStatusMutationsCoordinateManagedClientStatus(t *testing.T) {
	for _, path := range []string{"SetNodeStatus", "SaveNode"} {
		t.Run(path, func(t *testing.T) {
			node := activeNode()
			sso := &fakeManagedSSO{}
			remote := &fakeNodeClient{}
			service := newTestService(newFakeRepository(node), remote, sso)
			var err error
			switch path {
			case "SetNodeStatus":
				err = service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled})
			case "SaveNode":
				_, err = service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: node.NodeName, Status: domain.NodeStatusDisabled, DiscoveryType: node.DiscoveryType, ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer})
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(sso.statusCalls) != 1 || sso.statusCalls[0] != domain.NodeStatusDisabled {
				t.Fatalf("managed SSO status calls=%v", sso.statusCalls)
			}
			if len(remote.applyCommands) != 1 {
				t.Fatalf("remote hub-connection commands=%d want 1", len(remote.applyCommands))
			}
			disable := remote.applyCommands[0]
			if disable.Enabled || disable.ConnectionVersion == "" || disable.TargetRevision != 2 || disable.IdempotencyKey == "" || disable.ClientSecret != "" {
				t.Fatalf("unsafe managed Provider disable projection=%+v", disable)
			}
			if disable.Issuer != node.HubIssuer || disable.ClientID != node.OIDCClientID || disable.RedirectURI == "" {
				t.Fatalf("incomplete managed Provider disable projection=%+v", disable)
			}
			if remote.revokeCalls != 0 {
				t.Fatalf("Node disable revoked existing sessions: calls=%d", remote.revokeCalls)
			}
		})
	}
}

func TestDisableManagedFederationFailureIsReplaySafeAndFailClosed(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{applyFailOnce: true}
	sso := &fakeManagedSSO{}
	service := newTestService(repo, remote, sso)
	command := hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled}

	if err := service.SetNodeStatus(context.Background(), command); err == nil {
		t.Fatal("remote disable failure must be returned")
	}
	if stored := repo.get(node.NodeCode); stored.Status != domain.NodeStatusDisabled {
		t.Fatalf("remote failure did not preserve fail-closed Node status=%d", stored.Status)
	}
	if len(sso.statusCalls) == 0 || sso.statusCalls[0] != domain.NodeStatusDisabled {
		t.Fatalf("Hub SSO client was not fail-closed before remote cleanup: %v", sso.statusCalls)
	}
	if err := service.SetNodeStatus(context.Background(), command); err != nil {
		t.Fatalf("disable replay error=%v", err)
	}
	if len(remote.applyCommands) != 2 || !reflect.DeepEqual(remote.applyCommands[0], remote.applyCommands[1]) {
		t.Fatalf("disable replay changed remote command: %#v", remote.applyCommands)
	}
	if stored := repo.get(node.NodeCode); stored.Status != domain.NodeStatusDisabled {
		t.Fatalf("successful replay status=%d want disabled", stored.Status)
	}
	if remote.applyBusinessCount != 1 || remote.revokeCalls != 0 {
		t.Fatalf("disable business/revoke calls=%d/%d want 1/0", remote.applyBusinessCount, remote.revokeCalls)
	}
}

func TestReenabledNodeCanProvisionManagedFederationWithNewVersion(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{}
	sso := &fakeManagedSSO{secret: "managed-secret", hasSecret: true}
	service := newTestService(repo, remote, sso)

	if err := service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled}); err != nil {
		t.Fatalf("disable error=%v", err)
	}
	if err := service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusEnabled}); err != nil {
		t.Fatalf("re-enable error=%v", err)
	}
	if err := service.ProvisionNodeConnection(context.Background(), provisionCommand(node.NodeCode, "v3")); err != nil {
		t.Fatalf("provision after re-enable error=%v", err)
	}
	if len(remote.applyCommands) != 2 || remote.applyCommands[0].Enabled || !remote.applyCommands[1].Enabled {
		t.Fatalf("disable/re-enable projections=%+v", remote.applyCommands)
	}
	if remote.applyCommands[0].TargetRevision != 2 || remote.applyCommands[1].TargetRevision != 3 {
		t.Fatalf("disable/re-enable revisions=%+v", remote.applyCommands)
	}
	if stored := repo.get(node.NodeCode); stored.Status != domain.NodeStatusEnabled || stored.ConnectionStatus != domain.ConnectionActive || stored.ConnectionVersion != "v3" {
		t.Fatalf("re-enabled connection state=%+v", stored)
	}
}

func TestDelayedDisableProjectionCannotOverrideNewerReenableProvision(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	disableStarted := make(chan struct{})
	releaseDisable := make(chan struct{})
	remote := &fakeNodeClient{disableStarted: disableStarted, releaseDisable: releaseDisable}
	service := newTestService(repo, remote, &fakeManagedSSO{secret: "managed-secret", hasSecret: true})

	disableErr := make(chan error, 1)
	go func() {
		disableErr <- service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled})
	}()
	<-disableStarted
	if err := service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusEnabled}); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if err := service.ProvisionNodeConnection(context.Background(), provisionCommand(node.NodeCode, "v3")); err != nil {
		t.Fatalf("newer provision: %v", err)
	}
	close(releaseDisable)
	if err := <-disableErr; err != nil {
		t.Fatalf("delayed disable: %v", err)
	}

	remote.mu.Lock()
	defer remote.mu.Unlock()
	if !remote.projectedEnabled || remote.projectedRevision != 3 {
		t.Fatalf("stale disable won: enabled=%v revision=%d commands=%+v", remote.projectedEnabled, remote.projectedRevision, remote.applyCommands)
	}
}

func TestSaveNodeRejectsNodeCodeRename(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})

	_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
		OriginalNodeCode: node.NodeCode, NodeCode: "renamed-node", NodeName: node.NodeName,
		Status: node.Status, DiscoveryType: node.DiscoveryType,
		ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer,
	})
	if err == nil {
		t.Fatal("ordinary save must reject nodeCode rename")
	}
	if _, exists := repo.nodes["renamed-node"]; exists {
		t.Fatal("rename created a new replay/ownership identity")
	}
	if stored := repo.get(node.NodeCode); stored.OIDCClientID != node.OIDCClientID || stored.NodeCode != node.NodeCode {
		t.Fatalf("rename changed managed SSO ownership identity: %+v", stored)
	}
}

func TestSaveNodeRejectsIssuerChangeAfterActiveTransitionsToPendingOrError(t *testing.T) {
	for _, status := range []string{domain.ConnectionPending, domain.ConnectionError} {
		t.Run(status, func(t *testing.T) {
			node := activeNode()
			now := time.Now().UTC()
			node.IssuerLockedAt = &now
			node.ConnectionStatus = status
			service := newTestService(newFakeRepository(node), &fakeNodeClient{}, &fakeManagedSSO{})
			_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
				OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: node.NodeName,
				Status: domain.NodeStatusEnabled, DiscoveryType: domain.DiscoveryStatic,
				ManagementBaseURL: node.ManagementBaseURL, HubIssuer: "https://other-hub.example.com",
			})
			if err == nil {
				t.Fatalf("issuer change after activation must fail from %s", status)
			}
		})
	}
}

func TestGetNodeExposesSafeIssuerLockTimestamp(t *testing.T) {
	node := activeNode()
	lockedAt := time.Now().UTC().Truncate(time.Microsecond)
	node.IssuerLockedAt = &lockedAt
	service := newTestService(newFakeRepository(node), &fakeNodeClient{}, &fakeManagedSSO{})
	detail, err := service.GetNode(context.Background(), node.NodeCode)
	if err != nil || detail.IssuerLockedAt == nil || !detail.IssuerLockedAt.Equal(lockedAt) {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
}

func TestSaveNodeEncryptsSplitSecretAndAllowsSharedHubIssuer(t *testing.T) {
	repo := newFakeRepository()
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
	for _, code := range []string{"node-a", "node-b"} {
		result, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
			NodeCode: code, NodeName: code, Status: domain.NodeStatusEnabled,
			DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: "https://node.example.com:9443",
			HubIssuer: "https://hub.example.com", ManagementBearer: "bearer-" + code,
		})
		if err != nil {
			t.Fatalf("SaveNode(%s) error=%v", code, err)
		}
		if result.HubIssuer != "https://hub.example.com" {
			t.Fatalf("issuer=%q", result.HubIssuer)
		}
		stored := repo.nodes[code].ManagementBearer
		if stored.Ciphertext != "cipher:bearer-"+code || stored.EDEK == "" || stored.WrapKeyRef == "" {
			t.Fatalf("split secret=%+v", stored)
		}
	}
}

func TestNormalizeSavePreservesExactIssuerAndRejectsUnrepresentableFederationValues(t *testing.T) {
	base := hubfacade.SaveNodeCommand{
		NodeCode: "node-a", NodeName: "Node A", Status: domain.NodeStatusEnabled,
		DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: "https://node.example.com:9443",
		HubIssuer: "  https://hub.example.com/  ",
	}
	normalized, err := normalizeSave(base)
	if err != nil {
		t.Fatalf("normalize valid node: %v", err)
	}
	if normalized.HubIssuer != "https://hub.example.com/" {
		t.Fatalf("issuer changed exact identifier to %q", normalized.HubIssuer)
	}

	tooLongIssuer := base
	tooLongIssuer.HubIssuer = "https://hub.example.com/" + strings.Repeat("a", 513-len("https://hub.example.com/"))
	if _, err := normalizeSave(tooLongIssuer); err == nil {
		t.Fatal("accepted issuer exceeding managed provider representation")
	}

	tooLongOwner := base
	tooLongOwner.NodeCode = strings.Repeat("a", 61)
	if _, err := normalizeSave(tooLongOwner); err == nil {
		t.Fatal("accepted nodeCode that cannot fit hub:<owner> provider code")
	}
}

func TestSaveNodeRejectsSSRFInvalidStaticURL(t *testing.T) {
	service := newTestService(newFakeRepository(), &fakeNodeClient{}, &fakeManagedSSO{})
	_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
		NodeCode: "node-a", NodeName: "Node A", Status: domain.NodeStatusEnabled, DiscoveryType: domain.DiscoveryStatic,
		ManagementBaseURL: "https://user:pass@node.example.com:9443/admin?token=x", HubIssuer: "https://hub.example.com", ManagementBearer: "secret",
	})
	if err == nil {
		t.Fatal("SSRF-invalid static URL must fail")
	}
}

func TestSaveNodeRejectsSSRFRestrictedLiteralAddress(t *testing.T) {
	service := newTestService(newFakeRepository(), &fakeNodeClient{}, &fakeManagedSSO{})
	for _, managementURL := range []string{"http://127.0.0.1:9777", "http://[::1]:9777", "http://169.254.169.254:80", "http://10.0.0.1:9777"} {
		_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
			NodeCode: "node-a", NodeName: "Node A", Status: domain.NodeStatusEnabled, DiscoveryType: domain.DiscoveryStatic,
			ManagementBaseURL: managementURL, HubIssuer: "https://hub.example.com", ManagementBearer: "secret",
		})
		if err == nil {
			t.Fatalf("SSRF-restricted static URL accepted: %s", managementURL)
		}
	}
}

func TestSaveNodeAllowsHTTPHubIssuerOnlyWhenDevelopmentOptionEnabled(t *testing.T) {
	command := hubfacade.SaveNodeCommand{
		NodeCode: "dev-node", NodeName: "Dev Node", Status: domain.NodeStatusEnabled,
		DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: "https://node.example.com:9443",
		HubIssuer: "http://hub.localhost:18080/api/sso", ManagementBearer: "dev-bearer",
	}
	strict := NewService(newFakeRepository(), &fakeNodeClient{}, &fakeManagedSSO{}, fakeSecrets{}, func() int64 { return 101 })
	if _, err := strict.SaveNode(context.Background(), command); err == nil {
		t.Fatal("default service accepted HTTP Hub issuer")
	}
	developmentRepo := newFakeRepository()
	developmentSSO := &fakeManagedSSO{}
	development := NewService(developmentRepo, &fakeNodeClient{}, developmentSSO, fakeSecrets{}, func() int64 { return 101 }, WithDevelopmentHTTPIssuer())
	development.BindTransactor(&fakeTransactor{repo: developmentRepo, sso: developmentSSO})
	if _, err := development.SaveNode(context.Background(), command); err != nil {
		t.Fatalf("development service rejected HTTP Hub issuer: %v", err)
	}
}

func TestSaveNodeSkipsUnchangedStatusWrite(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
	_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
		OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: "Renamed Node",
		Status: node.Status, DiscoveryType: node.DiscoveryType, ManagementBaseURL: node.ManagementBaseURL,
		HubIssuer: node.HubIssuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.statusUpdates != 0 {
		t.Fatalf("unchanged status writes=%d want 0", repo.statusUpdates)
	}
}

func TestProvisionAllowsHTTPCallbackOnlyWhenDevelopmentOptionEnabled(t *testing.T) {
	command := provisionCommand("order-admin", "dev-v1")
	command.RedirectURI = "http://node-a.localhost:18080/api/login/external/hub-node-a/callback"
	if err := (&Service{}).validateProvision(command); err == nil {
		t.Fatal("default service accepted HTTP callback")
	}
	service := NewService(nil, nil, nil, nil, nil, WithDevelopmentHTTPIssuer())
	if err := service.validateProvision(command); err != nil {
		t.Fatalf("development service rejected HTTP callback: %v", err)
	}
}

func TestCopyNodeDefaultsDisabledAndNonActive(t *testing.T) {
	repo := newFakeRepository(activeNode())
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})

	copy, err := service.CopyNode(context.Background(), "order-admin", hubfacade.CopyNodeCommand{NodeCode: "order-admin-copy", NodeName: "Copy"})
	if err != nil {
		t.Fatalf("CopyNode() error=%v", err)
	}
	if copy.Status != domain.NodeStatusDisabled || copy.ConnectionStatus != domain.ConnectionPending {
		t.Fatalf("copy state=%d/%s want disabled/pending", copy.Status, copy.ConnectionStatus)
	}
	if repo.nodes[copy.NodeCode].OIDCClientSecret.Present() {
		t.Fatal("copy must not inherit OIDC client secret")
	}
}

func TestApplicationDoesNotAddASecondTransportRetryCycle(t *testing.T) {
	repo := newFakeRepository(activeNode())
	remote := &fakeNodeClient{statusTimeoutOnce: true}
	service := newTestService(repo, remote, &fakeManagedSSO{})
	command := hubfacade.NodeUserStatusCommand{NodeCode: "order-admin", UserID: "42", Status: nodefacade.UserStatusDisabled, Reason: "security", IdempotencyKey: "cmd-42"}

	if err := service.SetNodeUserStatus(context.Background(), command); !errors.Is(err, ErrAmbiguousTransport) {
		t.Fatalf("SetNodeUserStatus() error=%v want ambiguous transport", err)
	}
	if want := []string{"cmd-42"}; !reflect.DeepEqual(remote.statusKeys, want) {
		t.Fatalf("keys=%v want=%v", remote.statusKeys, want)
	}
	if len(remote.statusBodies) != 1 {
		t.Fatalf("application sends=%d want 1 transport-owned cycle", len(remote.statusBodies))
	}
}

func TestPageAndRemoteUserKeywordsAreBounded(t *testing.T) {
	service := newTestService(newFakeRepository(activeNode()), &fakeNodeClient{}, &fakeManagedSSO{})
	if _, err := service.PageNodes(context.Background(), hubfacade.NodePageQuery{Keyword: strings.Repeat("x", 257)}); err == nil {
		t.Fatal("registry keyword over 256 bytes must fail")
	}
	if _, err := service.ListNodeUsers(context.Background(), "order-admin", nodefacade.UserPageQuery{Current: 1, Size: 20, Keyword: strings.Repeat("x", 257)}); err == nil {
		t.Fatal("remote-user keyword over 256 bytes must fail")
	}
}

func TestSaveBoundsManagementBaseURLBeforePersistence(t *testing.T) {
	service := newTestService(newFakeRepository(), &fakeNodeClient{}, &fakeManagedSSO{})
	tooLongURL := "https://" + strings.Repeat("a", 2040) + ":9443"
	if _, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
		NodeCode: "node-a", NodeName: "Node A", Status: domain.NodeStatusEnabled,
		DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: tooLongURL,
		HubIssuer: "https://hub.example.com", ManagementBearer: "secret",
	}); err == nil {
		t.Fatal("managementBaseUrl over 2048 bytes must fail before persistence")
	}
}

func TestCopyBoundsReplacementBearerBeforeEncryption(t *testing.T) {
	repo := newFakeRepository(activeNode())
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
	if _, err := service.CopyNode(context.Background(), "order-admin", hubfacade.CopyNodeCommand{NodeCode: "copy-node", NodeName: "Copy", ManagementBearer: strings.Repeat("b", 8193)}); err == nil {
		t.Fatal("copy managementBearer over 8192 bytes must fail before encryption")
	}
}

func TestListNodeUsersTrimsKeywordBeforeTransport(t *testing.T) {
	remote := &fakeNodeClient{users: &nodefacade.UserPage{}}
	service := newTestService(newFakeRepository(activeNode()), remote, &fakeManagedSSO{})
	if _, err := service.ListNodeUsers(context.Background(), "order-admin", nodefacade.UserPageQuery{Current: 1, Size: 20, Keyword: "  needle  "}); err != nil {
		t.Fatal(err)
	}
	if remote.userQuery.Keyword != "needle" {
		t.Fatalf("transmitted keyword=%q want trimmed", remote.userQuery.Keyword)
	}
}

func TestListNodeUserSessionsBoundsUserID(t *testing.T) {
	service := newTestService(newFakeRepository(activeNode()), &fakeNodeClient{}, &fakeManagedSSO{})
	_, err := service.ListNodeUserSessions(context.Background(), "order-admin", strings.Repeat("u", 129), nodefacade.SessionPageQuery{Current: 1, Size: 20})
	if err == nil {
		t.Fatal("userId over 128 bytes must fail before transport")
	}
}

func TestProvisioningSagaResumesAfterNodeApplyFailure(t *testing.T) {
	node := activeNode()
	node.ConnectionStatus = domain.ConnectionError
	node.ConnectionVersion = ""
	node.OIDCClientID = ""
	node.OIDCClientSecret = domain.EncryptedSecret{}
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{applyFailOnce: true}
	sso := &fakeManagedSSO{secret: "managed-secret"}
	service := newTestService(repo, remote, sso)
	command := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v3", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-v3"}

	if err := service.ProvisionNodeConnection(context.Background(), command); err == nil {
		t.Fatal("first provision must report remote failure")
	}
	if got := repo.nodes[node.NodeCode].ConnectionStatus; got != domain.ConnectionError {
		t.Fatalf("first status=%s want ERROR", got)
	}
	if err := service.ProvisionNodeConnection(context.Background(), command); err != nil {
		t.Fatalf("replay provision error=%v", err)
	}
	if got := repo.nodes[node.NodeCode].ConnectionStatus; got != domain.ConnectionActive {
		t.Fatalf("replay status=%s want ACTIVE", got)
	}
	if sso.rotateCalls != 0 || sso.upsertCalls != 1 {
		t.Fatalf("managed SSO calls upsert=%d rotate=%d", sso.upsertCalls, sso.rotateCalls)
	}
	if remote.applyBusinessCount != 1 {
		t.Fatalf("business apply count=%d want 1", remote.applyBusinessCount)
	}
	if len(remote.applyCommands) != 2 || !reflect.DeepEqual(remote.applyCommands[0], remote.applyCommands[1]) {
		t.Fatalf("replay changed command: %#v", remote.applyCommands)
	}
	projection := remote.applyCommands[0]
	if projection.ConnectionVersion != "v3" || !projection.Enabled || projection.DisplayName != "Hub" || projection.Issuer != node.HubIssuer ||
		projection.ClientID != "hub-node-order-admin" || projection.ClientSecret != "managed-secret" || projection.RedirectURI != command.RedirectURI ||
		projection.Reason != command.Reason || projection.IdempotencyKey != command.IdempotencyKey {
		t.Fatalf("Hub Saga managed Provider projection=%+v", projection)
	}
}

func TestProvisioningExplicitRotationRunsOncePerNewVersion(t *testing.T) {
	repo := newFakeRepository(activeNode())
	remote := &fakeNodeClient{applyFailOnce: true}
	sso := &fakeManagedSSO{secret: "rotated-secret"}
	service := newTestService(repo, remote, sso)
	command := hubfacade.ProvisionConnectionCommand{NodeCode: "order-admin", ConnectionVersion: "v4", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", RotateSecret: true, Reason: "rotate", IdempotencyKey: "connect-v4"}

	_ = service.ProvisionNodeConnection(context.Background(), command)
	if err := service.ProvisionNodeConnection(context.Background(), command); err != nil {
		t.Fatalf("replay rotation error=%v", err)
	}
	if sso.rotateCalls != 1 {
		t.Fatalf("rotation calls=%d want 1", sso.rotateCalls)
	}
}

func TestProvisioningRejectsSupersededConnectionVersion(t *testing.T) {
	node := activeNode()
	v1 := provisionCommand(node.NodeCode, "v1")
	v1Hash, err := provisionRequestHash(v1)
	if err != nil {
		t.Fatal(err)
	}
	node.ConnectionVersion = v1.ConnectionVersion
	node.ConnectionRequestHash = v1Hash
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{}
	sso := &fakeManagedSSO{secret: "managed-secret", hasSecret: true}
	service := newTestService(repo, remote, sso)

	if err := service.ProvisionNodeConnection(context.Background(), provisionCommand(node.NodeCode, "v2")); err != nil {
		t.Fatalf("v2 provision error=%v", err)
	}
	service = newTestService(repo, &fakeNodeClient{}, sso)
	if err := service.ProvisionNodeConnection(context.Background(), v1); err == nil {
		t.Fatal("superseded v1 must never become current again")
	}
	stored := repo.get(node.NodeCode)
	if stored.ConnectionVersion != "v2" || stored.ConnectionStatus != domain.ConnectionActive {
		t.Fatalf("superseded replay changed current connection: %+v", stored)
	}
}

func TestProvisionCompletionRejectsChangedTargetCredentialAndDisable(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Service, domain.Node) error
	}{
		{name: "static URL", mutate: func(service *Service, node domain.Node) error {
			_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: node.NodeName, Status: node.Status, DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: "https://replacement.example.com:9443", HubIssuer: node.HubIssuer})
			return err
		}},
		{name: "Consul service", mutate: func(service *Service, node domain.Node) error {
			_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: node.NodeName, Status: node.Status, DiscoveryType: domain.DiscoveryConsul, ServiceName: "replacement-node", HubIssuer: node.HubIssuer})
			return err
		}},
		{name: "management Bearer", mutate: func(service *Service, node domain.Node) error {
			_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: node.NodeName, Status: node.Status, DiscoveryType: node.DiscoveryType, ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer, ManagementBearer: "replacement-bearer"})
			return err
		}},
		{name: "disable", mutate: func(service *Service, node domain.Node) error {
			return service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := activeNode()
			repo := newFakeRepository(node)
			remote := &fakeNodeClient{applyStarted: make(chan struct{}), releaseApply: make(chan struct{})}
			service := newTestService(repo, remote, &fakeManagedSSO{secret: "managed-secret", hasSecret: true})
			errCh := make(chan error, 1)
			go func() {
				errCh <- service.ProvisionNodeConnection(context.Background(), provisionCommand(node.NodeCode, "v3"))
			}()
			<-remote.applyStarted
			if err := test.mutate(service, node); err != nil {
				t.Fatalf("mutate target error=%v", err)
			}
			close(remote.releaseApply)
			if err := <-errCh; err == nil {
				t.Fatal("stale remote success must not complete the replacement target")
			}
			if stored := repo.get(node.NodeCode); stored.ConnectionStatus == domain.ConnectionActive {
				t.Fatalf("replacement target incorrectly became ACTIVE: %+v", stored)
			}
		})
	}
}

func TestProvisionTerminalStateSurvivesRequestCancellation(t *testing.T) {
	for _, remoteFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[remoteFailure], func(t *testing.T) {
			node := activeNode()
			repo := newFakeRepository(node)
			ctx, cancel := context.WithCancel(context.Background())
			remote := &fakeNodeClient{cancelAfterApply: cancel}
			if remoteFailure {
				remote.applyError = errors.New("remote failed after cancellation")
			}
			sso := &fakeManagedSSO{secret: "managed-secret", hasSecret: true}
			service := NewService(repo, remote, sso, fakeSecrets{}, func() int64 { return 101 })
			service.BindTransactor(&fakeTransactor{repo: repo, sso: sso, respectCancellation: true})

			_ = service.ProvisionNodeConnection(ctx, provisionCommand(node.NodeCode, "v-cancel"))
			stored := repo.get(node.NodeCode)
			if stored.ConnectionStatus == domain.ConnectionPending {
				t.Fatalf("canceled request stranded terminal state: %+v", stored)
			}
		})
	}
}

func TestRevokeNodeSessionsUsesSharedHundredReferenceLimit(t *testing.T) {
	node := activeNode()
	remote := &fakeNodeClient{}
	service := newTestService(newFakeRepository(node), remote, &fakeManagedSSO{})
	refs := make([]string, 100)
	for index := range refs {
		refs[index] = fmt.Sprintf("session-%d", index)
	}
	command := hubfacade.RevokeNodeSessionsCommand{NodeCode: node.NodeCode, UserID: "42", SessionRefs: refs, Reason: "incident", IdempotencyKey: "revoke-100"}
	if err := service.RevokeNodeUserSessions(context.Background(), command); err != nil {
		t.Fatalf("100 refs rejected: %v", err)
	}
	command.SessionRefs = append(command.SessionRefs, "session-101")
	command.IdempotencyKey = "revoke-101"
	if err := service.RevokeNodeUserSessions(context.Background(), command); err == nil {
		t.Fatal("101 refs must be rejected before transport")
	}
	if remote.revokeCalls != 1 {
		t.Fatalf("transport calls=%d want 1", remote.revokeCalls)
	}
}

func provisionCommand(nodeCode, version string) hubfacade.ProvisionConnectionCommand {
	return hubfacade.ProvisionConnectionCommand{NodeCode: nodeCode, ConnectionVersion: version, RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-" + version}
}

func TestProvisioningRollsBackOneTimeSecretWhenHubHandoffPersistenceFails(t *testing.T) {
	node := activeNode()
	node.ConnectionStatus = domain.ConnectionError
	node.ConnectionVersion = ""
	node.OIDCClientID = ""
	node.OIDCClientSecret = domain.EncryptedSecret{}
	repo := newFakeRepository(node)
	repo.failSecretHandoffOnce = true
	sso := &fakeManagedSSO{secret: "one-time-secret"}
	service := newTestService(repo, &fakeNodeClient{}, sso)
	service.BindTransactor(&fakeTransactor{repo: repo, sso: sso})
	command := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v5", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-v5"}

	if err := service.ProvisionNodeConnection(context.Background(), command); err == nil {
		t.Fatal("first provision must fail at Hub secret handoff persistence")
	}
	if stored := repo.get(node.NodeCode); stored.OIDCClientSecret.Present() || stored.ConnectionVersion == "v5" {
		t.Fatalf("failed transaction leaked partial Hub state: %+v", stored)
	}
	if sso.secretCommitted() {
		t.Fatal("failed Hub handoff committed one-time SSO secret hash")
	}
	if err := service.ProvisionNodeConnection(context.Background(), command); err != nil {
		t.Fatalf("replay after rollback error=%v", err)
	}
	if stored := repo.get(node.NodeCode); !stored.OIDCClientSecret.Present() || stored.ConnectionStatus != domain.ConnectionActive {
		t.Fatalf("replay did not converge: %+v", stored)
	}
}

func TestConcurrentSameVersionProvisionRotatesOnlyOnce(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	sso := &fakeManagedSSO{secret: "rotated-once"}
	service := newTestService(repo, &fakeNodeClient{}, sso)
	service.BindTransactor(&fakeTransactor{repo: repo, sso: sso})
	command := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v6", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", RotateSecret: true, Reason: "rotate", IdempotencyKey: "connect-v6"}

	start := make(chan struct{})
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			errs <- service.ProvisionNodeConnection(context.Background(), command)
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent provision error=%v", err)
		}
	}
	if got := sso.rotationCount(); got != 1 {
		t.Fatalf("rotation count=%d want 1", got)
	}
}

func TestLateSameVersionFailureDoesNotDowngradeActiveConnection(t *testing.T) {
	current := activeNode()
	current.ConnectionVersion = "v7"
	command := hubfacade.ProvisionConnectionCommand{NodeCode: current.NodeCode, ConnectionVersion: "v7", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-v7"}
	hash, err := provisionRequestHash(command)
	if err != nil {
		t.Fatal(err)
	}
	current.ConnectionRequestHash = hash
	repo := newFakeRepository(current)
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
	stale := current
	stale.ConnectionStatus = domain.ConnectionPending

	if err := service.failSaga(context.Background(), &stale, errors.New("late timeout")); err == nil {
		t.Fatal("remote failure must still be returned")
	}
	if stored := repo.get(current.NodeCode); stored.ConnectionStatus != domain.ConnectionActive {
		t.Fatalf("late failure downgraded successful connection: %+v", stored)
	}
}

func TestLateOldVersionSuccessCannotOverwriteNewSaga(t *testing.T) {
	current := activeNode()
	current.ConnectionStatus = domain.ConnectionPending
	current.ConnectionVersion = "v8"
	current.ConnectionRequestHash = "new-hash"
	repo := newFakeRepository(current)
	service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
	stale := current
	stale.ConnectionVersion = "v7"
	stale.ConnectionRequestHash = "old-hash"

	if err := service.completeSaga(context.Background(), &stale); err == nil {
		t.Fatal("stale success must be rejected")
	}
	if stored := repo.get(current.NodeCode); stored.ConnectionStatus != domain.ConnectionPending || stored.ConnectionVersion != "v8" {
		t.Fatalf("stale success overwrote new saga: %+v", stored)
	}
}

func TestHealthWriterDoesNotOverwriteConcurrentSagaState(t *testing.T) {
	node := activeNode()
	node.ConnectionStatus = domain.ConnectionPending
	node.ConnectionVersion = "v1"
	node.ConnectionRequestHash = "old-hash"
	node.IssuerLockedAt = nil
	repo := newFakeRepository(node)
	describeStarted := make(chan struct{})
	releaseDescribe := make(chan struct{})
	remote := &fakeNodeClient{describeStarted: describeStarted, releaseDescribe: releaseDescribe}
	service := newTestService(repo, remote, &fakeManagedSSO{})
	errCh := make(chan error, 1)

	go func() {
		_, err := service.TestConnection(context.Background(), node.NodeCode)
		errCh <- err
	}()
	<-describeStarted
	now := time.Now().UTC()
	committed := repo.get(node.NodeCode)
	committed.ConnectionStatus = domain.ConnectionActive
	committed.ConnectionVersion = "v2"
	committed.ConnectionRequestHash = "new-hash"
	committed.OIDCClientSecret = domain.EncryptedSecret{Ciphertext: "cipher:new-secret", EDEK: "new-edek", WrapKeyRef: "new-key"}
	committed.IssuerLockedAt = &now
	repo.mu.Lock()
	repo.nodes[node.NodeCode] = committed
	repo.mu.Unlock()
	close(releaseDescribe)
	if err := <-errCh; err != nil {
		t.Fatalf("TestConnection() error=%v", err)
	}

	stored := repo.get(node.NodeCode)
	if stored.ConnectionStatus != domain.ConnectionActive || stored.ConnectionVersion != "v2" || stored.ConnectionRequestHash != "new-hash" || stored.OIDCClientSecret != committed.OIDCClientSecret || stored.IssuerLockedAt == nil {
		t.Fatalf("stale health writer overwrote Saga-owned state: %+v", stored)
	}
}

func TestMetadataAndStatusWritersDoNotOverwriteConcurrentSagaState(t *testing.T) {
	for _, writer := range []string{"metadata", "status"} {
		t.Run(writer, func(t *testing.T) {
			node := activeNode()
			node.ConnectionStatus = domain.ConnectionPending
			node.ConnectionVersion = "v1"
			node.ConnectionRequestHash = "old-hash"
			node.IssuerLockedAt = nil
			repo := newFakeRepository(node)
			repo.findReturned = make(chan struct{})
			repo.releaseFind = make(chan struct{})
			service := newTestService(repo, &fakeNodeClient{}, &fakeManagedSSO{})
			errCh := make(chan error, 1)

			go func() {
				switch writer {
				case "metadata":
					_, err := service.SaveNode(context.Background(), hubfacade.SaveNodeCommand{
						OriginalNodeCode: node.NodeCode, NodeCode: node.NodeCode, NodeName: "Updated name",
						Status: node.Status, DiscoveryType: node.DiscoveryType,
						ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer,
					})
					errCh <- err
				case "status":
					errCh <- service.SetNodeStatus(context.Background(), hubfacade.SetNodeStatusCommand{NodeCode: node.NodeCode, Status: domain.NodeStatusDisabled})
				}
			}()
			<-repo.findReturned
			committed := commitNewSagaState(repo, node.NodeCode)
			close(repo.releaseFind)
			if err := <-errCh; err != nil {
				t.Fatalf("%s writer error=%v", writer, err)
			}
			stored := repo.get(node.NodeCode)
			if writer == "metadata" {
				assertSagaStatePreserved(t, stored, committed)
				return
			}
			if stored.ConnectionVersion != committed.ConnectionVersion || stored.ConnectionRequestHash != committed.ConnectionRequestHash || stored.OIDCClientSecret != committed.OIDCClientSecret || stored.IssuerLockedAt == nil {
				t.Fatalf("disable overwrote concurrent Saga identity/secret: %+v", stored)
			}
			if stored.Status != domain.NodeStatusDisabled || stored.ConnectionStatus == domain.ConnectionActive || stored.TargetRevision != committed.TargetRevision+1 {
				t.Fatalf("disable did not invalidate concurrent generation: %+v", stored)
			}
		})
	}
}

func commitNewSagaState(repo *fakeRepository, nodeCode string) domain.Node {
	now := time.Now().UTC()
	committed := repo.get(nodeCode)
	committed.ConnectionStatus = domain.ConnectionActive
	committed.ConnectionVersion = "v2"
	committed.ConnectionRequestHash = "new-hash"
	committed.OIDCClientSecret = domain.EncryptedSecret{Ciphertext: "cipher:new-secret", EDEK: "new-edek", WrapKeyRef: "new-key"}
	committed.IssuerLockedAt = &now
	repo.mu.Lock()
	repo.nodes[nodeCode] = committed
	repo.mu.Unlock()
	return committed
}

func assertSagaStatePreserved(t *testing.T, stored, committed domain.Node) {
	t.Helper()
	if stored.ConnectionStatus != committed.ConnectionStatus || stored.ConnectionVersion != committed.ConnectionVersion || stored.ConnectionRequestHash != committed.ConnectionRequestHash || stored.OIDCClientSecret != committed.OIDCClientSecret || stored.IssuerLockedAt == nil {
		t.Fatalf("ordinary writer overwrote Saga-owned state: %+v", stored)
	}
}

func TestProvisioningRejectsDifferentRequestForSameVersion(t *testing.T) {
	node := activeNode()
	node.ConnectionStatus = domain.ConnectionError
	first := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v3", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-v3"}
	hash, err := provisionRequestHash(first)
	if err != nil {
		t.Fatal(err)
	}
	node.ConnectionVersion = "v3"
	node.ConnectionRequestHash = hash
	service := newTestService(newFakeRepository(node), &fakeNodeClient{}, &fakeManagedSSO{secret: "managed-secret"})
	changed := first
	changed.RedirectURI = "https://evil.example.com/callback"
	if err := service.ProvisionNodeConnection(context.Background(), changed); err == nil {
		t.Fatal("different same-version replay must fail")
	}
}

func TestSagaDoesNotPersistSecretsReflectedByRemoteNode(t *testing.T) {
	node := activeNode()
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{applyError: reflectedRemoteError(t, "managed-secret", "bearer")}
	service := newTestService(repo, remote, &fakeManagedSSO{})
	command := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v-secret-test", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-secret-test"}

	if err := service.ProvisionNodeConnection(context.Background(), command); err == nil {
		t.Fatal("reflected remote failure must be returned")
	}
	stored := repo.get(node.NodeCode)
	for _, value := range []string{stored.LastConnectionError, stored.LastConnectionTraceID} {
		if strings.Contains(value, "managed-secret") || strings.Contains(value, "bearer") {
			t.Fatalf("Saga persisted reflected outbound secret: %+v", stored)
		}
	}
}

func TestSagaPersistsHubCanonicalTraceInsteadOfRemoteTrace(t *testing.T) {
	const canonicalTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	node := activeNode()
	repo := newFakeRepository(node)
	remote := &fakeNodeClient{applyError: fakeRemoteTraceError{traceID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}
	service := newTestService(repo, remote, &fakeManagedSSO{})
	command := hubfacade.ProvisionConnectionCommand{NodeCode: node.NodeCode, ConnectionVersion: "v-canonical", RedirectURI: "https://node.example.com/callback", DisplayName: "Hub", Reason: "provision", IdempotencyKey: "connect-canonical"}
	ctx := xcontext.WithTraceID(context.Background(), canonicalTraceID)

	if err := service.ProvisionNodeConnection(ctx, command); err == nil {
		t.Fatal("expected remote failure")
	}
	if got := repo.get(node.NodeCode).LastConnectionTraceID; got != canonicalTraceID {
		t.Fatalf("stored trace=%q, want %q", got, canonicalTraceID)
	}
}

type fakeRemoteTraceError struct{ traceID string }

func (e fakeRemoteTraceError) Error() string         { return "remote failure" }
func (e fakeRemoteTraceError) RemoteTraceID() string { return e.traceID }

func reflectedRemoteError(t *testing.T, oidcSecret, managementBearer string) error {
	t.Helper()
	transport := appReflectedTransport{response: &microservice.ServiceResponse{
		StatusCode: http.StatusBadGateway,
		Header:     http.Header{},
		Body:       []byte(`{"code":50201,"data":null,"message":"bad ` + oidcSecret + `","traceId":"trace-` + managementBearer + `"}`),
	}}
	client := hubinfra.NewNodeClient(transport, transport, appSecretService{plaintext: managementBearer}, nil)
	err := client.Do(context.Background(), hubinfra.NodeTarget{NodeCode: "node-a", DiscoveryType: "STATIC", ManagementBaseURL: "https://node.example.com:9443", ManagementBearer: hubinfra.EncryptedValue{Ciphertext: "cipher"}}, http.MethodPut, "/internal/node/v1/hub-connection", nil, map[string]string{"clientSecret": oidcSecret}, "connect-v1", nil)
	if err == nil {
		t.Fatal("expected remote error")
	}
	return err
}

type appReflectedTransport struct{ response *microservice.ServiceResponse }

func (t appReflectedTransport) Do(context.Context, microservice.ServiceRequest) (*microservice.ServiceResponse, error) {
	return t.response, nil
}

type appSecretService struct{ plaintext string }

func (appSecretService) EncryptString(context.Context, string) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (s appSecretService) DecryptString(context.Context, secretvalueinfra.SecretValue) (string, error) {
	return s.plaintext, nil
}
func (appSecretService) EncryptBytes(context.Context, []byte) (secretvalueinfra.SecretValue, error) {
	panic("not used")
}
func (appSecretService) DecryptBytes(context.Context, secretvalueinfra.SecretValue) ([]byte, error) {
	panic("not used")
}

func newTestService(repo *fakeRepository, remote *fakeNodeClient, sso *fakeManagedSSO) *Service {
	service := NewService(repo, remote, sso, fakeSecrets{}, func() int64 { return int64(len(repo.nodes) + 100) })
	service.BindTransactor(&fakeTransactor{repo: repo, sso: sso})
	return service
}

func activeNode() domain.Node {
	return domain.Node{ID: 1, NodeCode: "order-admin", NodeName: "Order Admin", Status: domain.NodeStatusEnabled, DiscoveryType: domain.DiscoveryStatic, ManagementBaseURL: "https://node.example.com:9443", HubIssuer: "https://hub.example.com", OIDCClientID: "hub-node-order-admin", OIDCClientSecret: domain.EncryptedSecret{Ciphertext: "cipher:managed-secret", EDEK: "edek", WrapKeyRef: "key"}, ManagementBearer: domain.EncryptedSecret{Ciphertext: "cipher:bearer", EDEK: "edek", WrapKeyRef: "key"}, ConnectionStatus: domain.ConnectionActive, ConnectionVersion: "v2", TargetRevision: 1}
}

type fakeRepository struct {
	mu                    sync.Mutex
	nodes                 map[string]domain.Node
	commands              map[string]domain.ConnectionCommand
	remoteUserWrites      int
	statusUpdates         int
	failSecretHandoffOnce bool
	findReturned          chan struct{}
	releaseFind           chan struct{}
	findOnce              sync.Once
}

func newFakeRepository(nodes ...domain.Node) *fakeRepository {
	repo := &fakeRepository{nodes: map[string]domain.Node{}, commands: map[string]domain.ConnectionCommand{}}
	for _, node := range nodes {
		repo.nodes[node.NodeCode] = node
	}
	return repo
}

func (r *fakeRepository) Page(context.Context, domain.NodePageQuery) ([]domain.Node, int64, error) {
	panic("not used")
}
func (r *fakeRepository) Find(_ context.Context, code string) (*domain.Node, error) {
	r.mu.Lock()
	n, ok := r.nodes[code]
	r.mu.Unlock()
	if !ok {
		return nil, nil
	}
	if r.findReturned != nil {
		r.findOnce.Do(func() { close(r.findReturned) })
		<-r.releaseFind
	}
	return &n, nil
}
func (r *fakeRepository) FindForUpdate(ctx context.Context, code string) (*domain.Node, error) {
	return r.Find(ctx, code)
}
func (r *fakeRepository) Insert(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nodes[n.NodeCode] = *n
	return nil
}
func (r *fakeRepository) UpdateMetadata(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.nodes[n.NodeCode]
	current.NodeName = n.NodeName
	current.DiscoveryType = n.DiscoveryType
	current.ServiceName = n.ServiceName
	current.ManagementBaseURL = n.ManagementBaseURL
	current.HubIssuer = n.HubIssuer
	current.CapabilitiesJSON = n.CapabilitiesJSON
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) ReplaceManagementBearer(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.nodes[n.NodeCode]
	current.ManagementBearer = n.ManagementBearer
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) UpdateStatus(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.nodes[n.NodeCode]
	r.statusUpdates++
	current.Status = n.Status
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) UpdateHealth(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.nodes[n.NodeCode]
	current.LastHealthyAt = n.LastHealthyAt
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) UpdateTargetState(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current := r.nodes[n.NodeCode]
	current.TargetRevision = n.TargetRevision
	current.ConnectionStatus = n.ConnectionStatus
	current.LastConnectionError = n.LastConnectionError
	current.LastConnectionTraceID = n.LastConnectionTraceID
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) UpdateConnection(_ context.Context, n *domain.Node) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSecretHandoffOnce && n.OIDCClientSecret.Present() {
		r.failSecretHandoffOnce = false
		return errors.New("injected Hub secret handoff persistence failure")
	}
	current := r.nodes[n.NodeCode]
	current.OIDCClientID = n.OIDCClientID
	current.OIDCClientSecret = n.OIDCClientSecret
	current.ConnectionStatus = n.ConnectionStatus
	current.ConnectionVersion = n.ConnectionVersion
	current.ConnectionRequestHash = n.ConnectionRequestHash
	current.IssuerLockedAt = n.IssuerLockedAt
	current.LastConnectionError = n.LastConnectionError
	current.LastConnectionTraceID = n.LastConnectionTraceID
	current.UpdatedAt = n.UpdatedAt
	r.nodes[n.NodeCode] = current
	return nil
}
func (r *fakeRepository) FindConnectionCommandForUpdate(_ context.Context, nodeCode, version string) (*domain.ConnectionCommand, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	command, ok := r.commands[nodeCode+"\x00"+version]
	if !ok {
		return nil, nil
	}
	copy := command
	return &copy, nil
}
func (r *fakeRepository) SaveConnectionCommand(_ context.Context, command *domain.ConnectionCommand) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[command.NodeCode+"\x00"+command.ConnectionVersion] = *command
	return nil
}
func (r *fakeRepository) get(code string) domain.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodes[code]
}
func (r *fakeRepository) snapshot() map[string]domain.Node {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]domain.Node, len(r.nodes))
	for key, node := range r.nodes {
		copy[key] = node
	}
	return copy
}
func (r *fakeRepository) restore(nodes map[string]domain.Node) {
	r.mu.Lock()
	r.nodes = nodes
	r.mu.Unlock()
}
func (r *fakeRepository) commandSnapshot() map[string]domain.ConnectionCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]domain.ConnectionCommand, len(r.commands))
	for key, command := range r.commands {
		copy[key] = command
	}
	return copy
}
func (r *fakeRepository) restoreCommands(commands map[string]domain.ConnectionCommand) {
	r.mu.Lock()
	r.commands = commands
	r.mu.Unlock()
}

type fakeNodeClient struct {
	mu                 sync.Mutex
	users              *nodefacade.UserPage
	userQuery          nodefacade.UserPageQuery
	statusTimeoutOnce  bool
	statusKeys         []string
	statusBodies       []nodefacade.SetUserStatusCommand
	applyFailOnce      bool
	applyCommands      []nodefacade.ApplyHubConnectionCommand
	applyBusinessCount int
	applyError         error
	applyStarted       chan struct{}
	releaseApply       chan struct{}
	disableStarted     chan struct{}
	releaseDisable     chan struct{}
	disableStartOnce   sync.Once
	applyStartOnce     sync.Once
	projectedRevision  int64
	projectedEnabled   bool
	cancelAfterApply   context.CancelFunc
	revokeCalls        int
	describeStarted    chan struct{}
	releaseDescribe    chan struct{}
}

func (f *fakeNodeClient) Describe(context.Context, domain.Node) (*nodefacade.NodeDescriptor, error) {
	if f.describeStarted != nil {
		close(f.describeStarted)
	}
	if f.releaseDescribe != nil {
		<-f.releaseDescribe
	}
	return &nodefacade.NodeDescriptor{Health: "UP"}, nil
}
func (f *fakeNodeClient) ListUsers(_ context.Context, _ domain.Node, query nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	f.userQuery = query
	return f.users, nil
}
func (f *fakeNodeClient) GetUser(context.Context, domain.Node, string) (*nodefacade.UserDetail, error) {
	panic("not used")
}
func (f *fakeNodeClient) SetUserStatus(_ context.Context, _ domain.Node, command nodefacade.SetUserStatusCommand) error {
	f.statusKeys = append(f.statusKeys, command.IdempotencyKey)
	f.statusBodies = append(f.statusBodies, command)
	if f.statusTimeoutOnce {
		f.statusTimeoutOnce = false
		return ErrAmbiguousTransport
	}
	return nil
}
func (f *fakeNodeClient) ListUserSessions(context.Context, domain.Node, string, nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	panic("not used")
}
func (f *fakeNodeClient) RevokeUserSessions(context.Context, domain.Node, nodefacade.RevokeUserSessionsCommand) error {
	f.revokeCalls++
	return nil
}
func (f *fakeNodeClient) GetLoginPolicy(context.Context, domain.Node) (*nodefacade.ManagedLoginPolicy, error) {
	panic("not used")
}
func (f *fakeNodeClient) ApplyLoginPolicy(context.Context, domain.Node, nodefacade.ApplyLoginPolicyCommand) error {
	panic("not used")
}
func (f *fakeNodeClient) ApplyHubConnection(_ context.Context, _ domain.Node, command nodefacade.ApplyHubConnectionCommand) error {
	if !command.Enabled && f.disableStarted != nil {
		f.disableStartOnce.Do(func() { close(f.disableStarted) })
	}
	if !command.Enabled && f.releaseDisable != nil {
		<-f.releaseDisable
	}
	if command.Enabled && f.applyStarted != nil {
		f.applyStartOnce.Do(func() { close(f.applyStarted) })
	}
	if command.Enabled && f.releaseApply != nil {
		<-f.releaseApply
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applyCommands = append(f.applyCommands, command)
	if command.TargetRevision >= f.projectedRevision {
		f.projectedRevision = command.TargetRevision
		f.projectedEnabled = command.Enabled
	}
	if f.cancelAfterApply != nil {
		f.cancelAfterApply()
	}
	if f.applyError != nil {
		return f.applyError
	}
	if f.applyFailOnce {
		f.applyFailOnce = false
		return errors.New("remote unavailable")
	}
	f.applyBusinessCount++
	return nil
}

type fakeManagedSSO struct {
	mu                       sync.Mutex
	secret                   string
	upsertCalls, rotateCalls int
	hasSecret                bool
	statusCalls              []int
}

func (f *fakeManagedSSO) UpsertManagedClient(_ context.Context, command hubfacade.ManagedSSOClientCommand) (*hubfacade.ManagedSSOClientResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertCalls++
	if command.RotateSecret {
		f.rotateCalls++
	}
	issued := ""
	if !f.hasSecret || command.RotateSecret {
		issued = f.secret
		f.hasSecret = issued != ""
	}
	return &hubfacade.ManagedSSOClientResult{ClientID: command.ClientID, ClientSecret: issued}, nil
}
func (f *fakeManagedSSO) SetManagedClientStatus(_ context.Context, command hubfacade.ManagedSSOClientStatusCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls = append(f.statusCalls, command.Status)
	return nil
}
func (f *fakeManagedSSO) secretCommitted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hasSecret
}
func (f *fakeManagedSSO) rotationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rotateCalls
}

type fakeTransactor struct {
	mu                  sync.Mutex
	repo                *fakeRepository
	sso                 *fakeManagedSSO
	respectCancellation bool
}

func (t *fakeTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.respectCancellation && ctx.Err() != nil {
		return ctx.Err()
	}
	nodes := t.repo.snapshot()
	commands := t.repo.commandSnapshot()
	t.sso.mu.Lock()
	hadSecret := t.sso.hasSecret
	t.sso.mu.Unlock()
	if err := fn(ctx); err != nil {
		t.repo.restore(nodes)
		t.repo.restoreCommands(commands)
		t.sso.mu.Lock()
		t.sso.hasSecret = hadSecret
		t.sso.mu.Unlock()
		return err
	}
	return nil
}

type fakeSecrets struct{}

func (fakeSecrets) Encrypt(_ context.Context, plain string) (domain.EncryptedSecret, error) {
	return domain.EncryptedSecret{Ciphertext: "cipher:" + plain, EDEK: "edek", WrapKeyRef: "key"}, nil
}
func (fakeSecrets) Decrypt(_ context.Context, value domain.EncryptedSecret) (string, error) {
	return value.Ciphertext[len("cipher:"):], nil
}
