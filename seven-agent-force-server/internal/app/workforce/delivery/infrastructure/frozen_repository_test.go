package infrastructure

import (
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	"strings"
	"testing"
)

func deliveryBindingFixture() (frozenRepositoryPlan, projectfacade.DeliveryRepositoryBinding) {
	p := frozenRepositoryPlan{ProjectID: 24, ProjectVersionID: 99, ProviderType: "GITLAB", Scheme: "https", Host: "127.0.0.1", Port: 3444, RepositoryPath: "team/fixture.git", DisplayURL: "https://127.0.0.1:3444/team/fixture.git", WebURL: "https://127.0.0.1:3444/team/fixture", RepositoryConfigDigest: strings.Repeat("a", 64), TargetConfigDigest: strings.Repeat("a", 64), Credential: application.EncryptedCredentialEnvelope{Version: 1, Ciphertext: "fixture-original-ciphertext", EDEK: "fixture-original-edek", WrapKeyRef: "fixture-key"}, BaseBranch: "main", BaseCommit: "fixture-base", ResultBranch: "codex/fixture", DeliveryMode: "NEW_BRANCH_AND_DRAFT_REVIEW", PlanDigest: strings.Repeat("e", 64), PushAuthorizationDigest: strings.Repeat("b", 64), ReviewAuthorizationDigest: strings.Repeat("f", 64)}
	b := projectfacade.DeliveryRepositoryBinding{ProjectID: 24, VersionID: 101, CredentialVersion: 2, Current: true, ProviderType: p.ProviderType, Scheme: p.Scheme, Host: p.Host, Port: p.Port, RepositoryPath: p.RepositoryPath, WebURL: p.WebURL, ConfigDigest: strings.Repeat("c", 64), Ciphertext: "fixture-new-ciphertext", EDEK: "fixture-new-edek", WrapKeyRef: "fixture-key", Push: true, DraftReview: true}
	return p, b
}

func TestDeliveryBindingChangesOnlyCredentialAuthority(t *testing.T) {
	p, b := deliveryBindingFixture()
	got, err := bindDeliveryRepository(p, b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Credential.Version != 2 || got.Credential.Ciphertext != b.Ciphertext || got.TargetConfigDigest != b.ConfigDigest {
		t.Fatal("replacement not bound")
	}
	if got.BaseBranch != p.BaseBranch || got.BaseCommit != p.BaseCommit || got.ResultBranch != p.ResultBranch || got.PlanDigest != p.PlanDigest || got.ProjectVersionID != p.ProjectVersionID {
		t.Fatal("frozen plan changed")
	}
	if p.Credential.Version != 1 || p.Credential.Ciphertext == got.Credential.Ciphertext {
		t.Fatal("original authority overwritten")
	}
	for _, c := range []struct {
		name   string
		change func(*projectfacade.DeliveryRepositoryBinding)
	}{
		{"other project", func(b *projectfacade.DeliveryRepositoryBinding) { b.ProjectID++ }},
		{"other host", func(b *projectfacade.DeliveryRepositoryBinding) { b.Host = "other.example" }},
		{"other port", func(b *projectfacade.DeliveryRepositoryBinding) { b.Port++ }},
		{"other repository", func(b *projectfacade.DeliveryRepositoryBinding) { b.RepositoryPath = "other/repo.git" }},
		{"other protocol", func(b *projectfacade.DeliveryRepositoryBinding) { b.Scheme = "http" }},
		{"other provider", func(b *projectfacade.DeliveryRepositoryBinding) { b.ProviderType = "GITHUB" }},
		{"old credential", func(b *projectfacade.DeliveryRepositoryBinding) { b.CredentialVersion = 1 }},
		{"unverified push", func(b *projectfacade.DeliveryRepositoryBinding) { b.Push = false }},
		{"unverified review", func(b *projectfacade.DeliveryRepositoryBinding) { b.DraftReview = false }},
		{"empty envelope", func(b *projectfacade.DeliveryRepositoryBinding) { b.Ciphertext = "" }},
	} {
		t.Run(c.name, func(t *testing.T) {
			bad := b
			c.change(&bad)
			if _, err := bindDeliveryRepository(p, bad); !errors.Is(err, domain.ErrReauthorization) {
				t.Fatalf("unsafe replacement accepted: %v", err)
			}
		})
	}
}
