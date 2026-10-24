package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

const (
	WorkforceSceneTaskActionRequired       = "workforce_task_action_required"
	WorkforceSceneTaskInterrupted          = "workforce_task_interrupted"
	WorkforceSceneTaskCompleted            = "workforce_task_completed"
	WorkforceSceneDeliveryApprovalRequired = "workforce_delivery_approval_required"
	WorkforceSceneDeliveryManualReview     = "workforce_delivery_manual_review"

	builtinRevisionAuditBootstrap = "BOOTSTRAP_PUBLISHED"
)

type builtinWorkforceScene struct {
	code        string
	name        string
	subject     string
	text        string
	html        string
	markdown    string
	actionLabel string
}

var builtinWorkforceScenes = []builtinWorkforceScene{
	{
		code:        WorkforceSceneTaskActionRequired,
		name:        "任务需要处理",
		subject:     "任务需要处理：{{.taskName}}",
		text:        "{{.taskName}}\n第 {{.turnNumber}} 轮需要处理。\n{{.summary}}",
		html:        "<p><strong>{{.taskName}}</strong></p><p>第 {{.turnNumber}} 轮需要处理。</p><p>{{.summary}}</p>",
		markdown:    "**{{.taskName}}**\n\n第 {{.turnNumber}} 轮需要处理。\n\n{{.summary}}",
		actionLabel: "查看任务",
	},
	{
		code:        WorkforceSceneTaskInterrupted,
		name:        "任务执行中断",
		subject:     "任务执行中断：{{.taskName}}",
		text:        "{{.taskName}}\n第 {{.turnNumber}} 轮执行中断。\n{{.reasonLabel}}",
		html:        "<p><strong>{{.taskName}}</strong></p><p>第 {{.turnNumber}} 轮执行中断。</p><p>{{.reasonLabel}}</p>",
		markdown:    "**{{.taskName}}**\n\n第 {{.turnNumber}} 轮执行中断。\n\n{{.reasonLabel}}",
		actionLabel: "查看任务",
	},
	{
		code:        WorkforceSceneTaskCompleted,
		name:        "任务已完成",
		subject:     "任务已完成：{{.taskName}}",
		text:        "{{.taskName}}\n第 {{.turnNumber}} 轮已经完成。\n{{.summary}}",
		html:        "<p><strong>{{.taskName}}</strong></p><p>第 {{.turnNumber}} 轮已经完成。</p><p>{{.summary}}</p>",
		markdown:    "**{{.taskName}}**\n\n第 {{.turnNumber}} 轮已经完成。\n\n{{.summary}}",
		actionLabel: "查看任务",
	},
	{
		code:        WorkforceSceneDeliveryApprovalRequired,
		name:        "结果分支等待批准",
		subject:     "结果分支等待批准：{{.taskName}}",
		text:        "{{.taskName}}\n第 {{.turnNumber}} 轮的结果分支等待批准。\n{{.summary}}",
		html:        "<p><strong>{{.taskName}}</strong></p><p>第 {{.turnNumber}} 轮的结果分支等待批准。</p><p>{{.summary}}</p>",
		markdown:    "**{{.taskName}}**\n\n第 {{.turnNumber}} 轮的结果分支等待批准。\n\n{{.summary}}",
		actionLabel: "去处理",
	},
	{
		code:        WorkforceSceneDeliveryManualReview,
		name:        "交付需要处理",
		subject:     "交付需要处理：{{.taskName}}",
		text:        "{{.taskName}}\n第 {{.turnNumber}} 轮的交付需要人工处理。\n{{.reasonLabel}}",
		html:        "<p><strong>{{.taskName}}</strong></p><p>第 {{.turnNumber}} 轮的交付需要人工处理。</p><p>{{.reasonLabel}}</p>",
		markdown:    "**{{.taskName}}**\n\n第 {{.turnNumber}} 轮的交付需要人工处理。\n\n{{.reasonLabel}}",
		actionLabel: "去处理",
	},
}

// EnsureBuiltinWorkforceScenes publishes the platform-owned in-app baseline.
// It only creates missing definitions. Existing published administrator-owned
// revisions are never replaced, and a colliding unpublished definition fails
// closed so startup cannot silently select different content.
func (s *Service) EnsureBuiltinWorkforceScenes(ctx context.Context) error {
	templates, err := s.templateRevisionRepository()
	if err != nil {
		return err
	}
	scenes, err := s.sceneRevisionRepository()
	if err != nil {
		return err
	}
	for _, spec := range builtinWorkforceScenes {
		templateRevision, err := s.ensureBuiltinWorkforceTemplate(ctx, templates, spec)
		if err != nil {
			return fmt.Errorf("bootstrap notification template %s: %w", spec.code, err)
		}
		if err := s.ensureBuiltinWorkforceInAppScene(ctx, scenes, spec, templateRevision); err != nil {
			return fmt.Errorf("bootstrap notification scene %s: %w", spec.code, err)
		}
	}
	return nil
}

func (s *Service) ensureBuiltinWorkforceTemplate(ctx context.Context, repo domain.TemplateRevisionRepository, spec builtinWorkforceScene) (*domain.TemplateRevision, error) {
	scopeID := s.templateRevisionScopeID()
	definition, err := repo.FindTemplateDefinitionByCode(ctx, scopeID, spec.code)
	if err != nil {
		return nil, err
	}
	if definition != nil {
		if definition.CurrentPublishedRevisionID == nil || *definition.CurrentPublishedRevisionID <= 0 {
			return nil, fmt.Errorf("NOTIFICATION_SCENE_NOT_PUBLISHED: builtin template code is already reserved by an unpublished definition")
		}
		revision, err := repo.FindTemplateRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		if err != nil {
			return nil, err
		}
		if revision == nil || revision.State != domain.TemplateRevisionStatePublished {
			return nil, fmt.Errorf("NOTIFICATION_SCENE_NOT_PUBLISHED: builtin template published pointer is invalid")
		}
		return revision, nil
	}

	draft, variablesJSON, digest, err := normalizeTemplateRevisionDraft(builtinTemplateDraft(spec))
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	definitionID := s.nextID()
	revisionID := s.nextID()
	definition = &domain.TemplateDefinition{
		ID:                         definitionID,
		ScopeID:                    scopeID,
		TemplateCode:               spec.code,
		TemplateName:               draft.TemplateName,
		Locale:                     draft.Locale,
		CurrentPublishedRevisionID: int64Ptr(revisionID),
		Version:                    1,
	}
	revision := &domain.TemplateRevision{
		ID:                         revisionID,
		TemplateDefinitionID:       definitionID,
		RevisionNo:                 1,
		State:                      domain.TemplateRevisionStatePublished,
		RevisionVersion:            1,
		SubjectTemplate:            draft.SubjectTemplate,
		TextTemplate:               draft.TextTemplate,
		HTMLTemplate:               draft.HTMLTemplate,
		MarkdownTemplate:           draft.MarkdownTemplate,
		PrimaryActionLabelTemplate: draft.PrimaryActionLabelTemplate,
		VariableSchemaJSON:         variablesJSON,
		ContentDigest:              digest,
		PublishedAt:                timePtr(now),
	}
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := repo.InsertTemplateDefinition(txCtx, definition); err != nil {
			return err
		}
		if err := repo.InsertTemplateRevision(txCtx, revision); err != nil {
			return err
		}
		return repo.InsertTemplateRevisionAudit(txCtx, &domain.TemplateRevisionAudit{
			ID:                   s.nextID(),
			TemplateDefinitionID: definitionID,
			ScopeID:              scopeID,
			Action:               builtinRevisionAuditBootstrap,
			ToRevisionNo:         intPtr(1),
		})
	}); err != nil {
		// A concurrent bootstrap may have committed the same stable identity.
		definition, findErr := repo.FindTemplateDefinitionByCode(ctx, scopeID, spec.code)
		if findErr == nil && definition != nil && definition.CurrentPublishedRevisionID != nil {
			return repo.FindTemplateRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		}
		return nil, err
	}
	return revision, nil
}

func (s *Service) ensureBuiltinWorkforceInAppScene(ctx context.Context, repo domain.SceneRevisionRepository, spec builtinWorkforceScene, template *domain.TemplateRevision) error {
	if template == nil || template.ID <= 0 || template.State != domain.TemplateRevisionStatePublished {
		return fmt.Errorf("published template revision is required")
	}
	scopeID := s.sceneRevisionScopeID()
	definition, err := repo.FindSceneDefinitionByCodeAndReceiverKind(ctx, scopeID, spec.code, domain.SceneReceiverKindInApp)
	if err != nil {
		return err
	}
	if definition != nil {
		if definition.CurrentPublishedRevisionID == nil || *definition.CurrentPublishedRevisionID <= 0 {
			return fmt.Errorf("NOTIFICATION_SCENE_NOT_PUBLISHED: builtin scene code is already reserved by an unpublished definition")
		}
		revision, err := repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		if err != nil {
			return err
		}
		if revision == nil || revision.State != domain.SceneRevisionStatePublished || !revision.Enabled {
			return fmt.Errorf("NOTIFICATION_SCENE_NOT_PUBLISHED: builtin scene published pointer is invalid or disabled")
		}
		return nil
	}

	now := s.now().UTC()
	definitionID := s.nextID()
	revisionID := s.nextID()
	definition = &domain.SceneDefinition{
		ID:                         definitionID,
		ScopeID:                    scopeID,
		SceneCode:                  spec.code,
		SceneName:                  spec.name,
		ReceiverKind:               domain.SceneReceiverKindInApp,
		CurrentPublishedRevisionID: int64Ptr(revisionID),
		Version:                    1,
	}
	revision := &domain.SceneRevision{
		ID:                 revisionID,
		SceneDefinitionID:  definitionID,
		RevisionNo:         1,
		State:              domain.SceneRevisionStatePublished,
		RevisionVersion:    1,
		Enabled:            true,
		TemplateRevisionID: template.ID,
		PublishedAt:        timePtr(now),
	}
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := repo.InsertSceneDefinition(txCtx, definition); err != nil {
			return err
		}
		if err := repo.InsertSceneRevision(txCtx, revision); err != nil {
			return err
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definitionID,
			ScopeID:           scopeID,
			Action:            builtinRevisionAuditBootstrap,
			ToRevisionNo:      intPtr(1),
		})
	}); err != nil {
		definition, findErr := repo.FindSceneDefinitionByCodeAndReceiverKind(ctx, scopeID, spec.code, domain.SceneReceiverKindInApp)
		if findErr == nil && definition != nil && definition.CurrentPublishedRevisionID != nil {
			return nil
		}
		return err
	}
	return nil
}

func builtinTemplateDraft(spec builtinWorkforceScene) facade.TemplateRevisionDraftInput {
	draft := facade.TemplateRevisionDraftInput{
		TemplateName:               spec.name,
		Locale:                     "zh-CN",
		SubjectTemplate:            spec.subject,
		TextTemplate:               spec.text,
		HTMLTemplate:               spec.html,
		MarkdownTemplate:           spec.markdown,
		PrimaryActionLabelTemplate: spec.actionLabel,
	}
	for _, variable := range []facade.TemplateRevisionVariable{
		{Name: "taskName", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 120, Classification: domain.TemplateVariableClassificationPublic},
		{Name: "turnNumber", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 20, Classification: domain.TemplateVariableClassificationPublic},
		{Name: "statusLabel", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 80, Classification: domain.TemplateVariableClassificationPublic},
		{Name: "summary", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 500, Classification: domain.TemplateVariableClassificationPublic},
		{Name: "reasonLabel", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 160, Classification: domain.TemplateVariableClassificationPublic},
		{Name: "occurredAt", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 64, Classification: domain.TemplateVariableClassificationPublic},
	} {
		placeholder := "{{." + variable.Name + "}}"
		if strings.Contains(draft.SubjectTemplate, placeholder) || strings.Contains(draft.TextTemplate, placeholder) || strings.Contains(draft.HTMLTemplate, placeholder) || strings.Contains(draft.MarkdownTemplate, placeholder) || strings.Contains(draft.PrimaryActionLabelTemplate, placeholder) {
			draft.Variables = append(draft.Variables, variable)
		}
	}
	return draft
}

func timePtr(value time.Time) *time.Time { return &value }
