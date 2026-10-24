package application

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func TestEnsureBuiltinWorkforceScenesPublishesOnceAndNeverReplacesHistory(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	service := newSceneRevisionTestService(t, repo)
	if err := service.EnsureBuiltinWorkforceScenes(context.Background()); err != nil {
		t.Fatalf("first bootstrap error=%v", err)
	}
	if len(repo.templateDefinitions) != 5 || len(repo.templateRevisions) != 5 || len(repo.sceneDefinitions) != 5 || len(repo.sceneRevisions) != 5 {
		t.Fatalf("bootstrap counts templates=%d revisions=%d scenes=%d sceneRevisions=%d", len(repo.templateDefinitions), len(repo.templateRevisions), len(repo.sceneDefinitions), len(repo.sceneRevisions))
	}
	for _, sceneCode := range []string{WorkforceSceneTaskActionRequired, WorkforceSceneTaskInterrupted, WorkforceSceneTaskCompleted, WorkforceSceneDeliveryApprovalRequired, WorkforceSceneDeliveryManualReview} {
		definition, err := repo.FindSceneDefinitionByCodeAndReceiverKind(context.Background(), "local", sceneCode, domain.SceneReceiverKindInApp)
		if err != nil || definition == nil || definition.CurrentPublishedRevisionID == nil {
			t.Fatalf("scene %s not published: definition=%+v err=%v", sceneCode, definition, err)
		}
	}
	resolution, err := service.resolveSceneReceiver(context.Background(), repo, WorkforceSceneTaskActionRequired, domain.SceneReceiverKindInApp, 0, map[string]any{
		"taskName": "补充用户查询接口", "turnNumber": "2", "statusLabel": "需要处理", "summary": "请确认分页兼容策略。", "reasonLabel": "需要补充信息", "occurredAt": "2026-08-30T10:00:00Z",
	})
	if err != nil || resolution == nil || resolution.rendered.PrimaryActionLabel != "查看任务" || resolution.rendered.Subject != "任务需要处理：补充用户查询接口" {
		t.Fatalf("builtin scene render=%+v err=%v", resolution, err)
	}
	if _, err := service.resolveSceneReceiver(context.Background(), repo, WorkforceSceneTaskActionRequired, domain.SceneReceiverKindInApp, 0, map[string]any{
		"taskName": "任务", "turnNumber": "1", "summary": "摘要", "unsafeURL": "https://evil.example",
	}); err == nil {
		t.Fatal("unknown Workforce scene variable must fail closed")
	}
	templateAuditCount, sceneAuditCount := len(repo.templateAudits), len(repo.sceneAudits)
	if err := service.EnsureBuiltinWorkforceScenes(context.Background()); err != nil {
		t.Fatalf("idempotent bootstrap error=%v", err)
	}
	if len(repo.templateDefinitions) != 5 || len(repo.sceneDefinitions) != 5 || len(repo.templateAudits) != templateAuditCount || len(repo.sceneAudits) != sceneAuditCount {
		t.Fatalf("second bootstrap changed durable history")
	}
}

func TestEnsureBuiltinWorkforceScenesFailsClosedOnReservedUnpublishedCode(t *testing.T) {
	repo := newSceneRevisionTestRepository()
	repo.templateDefinitions[1] = &domain.TemplateDefinition{ID: 1, ScopeID: "local", TemplateCode: WorkforceSceneTaskActionRequired, TemplateName: "冲突草稿", Version: 1}
	service := newSceneRevisionTestService(t, repo)
	if err := service.EnsureBuiltinWorkforceScenes(context.Background()); err == nil {
		t.Fatal("reserved unpublished builtin code must fail closed")
	}
}
