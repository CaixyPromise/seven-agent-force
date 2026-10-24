package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func TestEnsureEnterpriseDirectRouteIsOrganizationScopedAndSecretless(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	service := newSceneRevisionTestService(t, repo)
	if err := service.EnsureBuiltinWorkforceScenes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.EnsureEnterpriseDirectRoute(context.Background(), 101, 301, 1, "connection-digest-v1", "FEISHU", true, 201); err != nil {
		t.Fatalf("provision direct route: %v", err)
	}
	channel := repo.channels["enterprise_101_feishu"]
	if channel == nil || channel.ScopeType != "ORGANIZATION" || channel.OrganizationID == nil || *channel.OrganizationID != 101 || channel.EnterpriseConnectionID == nil || *channel.EnterpriseConnectionID != 301 {
		t.Fatalf("channel = %#v", channel)
	}
	if channel.SecretCiphertext != "" || channel.SecretEDEK != "" || channel.SecretWrapKeyRef != "" {
		t.Fatal("notification route duplicated enterprise application secret")
	}
	code := enterpriseSceneCode(WorkforceSceneTaskActionRequired, domain.SceneReceiverKindFeishuOpenID, 101)
	definition, err := repo.FindSceneDefinitionByCodeAndReceiverKind(context.Background(), "local", code, domain.SceneReceiverKindFeishuOpenID)
	if err != nil || definition == nil || definition.CurrentPublishedRevisionID == nil {
		t.Fatalf("direct scene definition=%#v err=%v", definition, err)
	}

	if err := service.EnsureEnterpriseDirectRoute(context.Background(), 101, 301, 2, "connection-digest-v2", "FEISHU", true, 201); err != nil {
		t.Fatalf("rotate direct route: %v", err)
	}
	definition, _ = repo.FindSceneDefinitionByCodeAndReceiverKind(context.Background(), "local", code, domain.SceneReceiverKindFeishuOpenID)
	published, _ := repo.FindSceneRevisionByID(context.Background(), *definition.CurrentPublishedRevisionID)
	if published == nil || published.RevisionNo != 2 {
		t.Fatalf("rotated scene = %#v", published)
	}
}
