package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const (
	templateRevisionAuditCreateDraft              = "CREATE_DRAFT"
	templateRevisionAuditSaveDraft                = "SAVE_DRAFT"
	templateRevisionAuditCreateDraftFromPublished = "CREATE_DRAFT_FROM_PUBLISHED"
	templateRevisionAuditPublish                  = "PUBLISH"
)

// ListTemplateDefinitions lists versioned templates in the service-owned
// scope.
func (s *Service) ListTemplateDefinitions(ctx context.Context, query domain.TemplateDefinitionQuery) (*facade.PageResult[facade.TemplateDefinitionRecord], error) {
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	query.ScopeID = s.templateRevisionScopeID()
	items, total, err := repo.ListTemplateDefinitions(ctx, query)
	if err != nil {
		return nil, err
	}
	revisionIDs := make([]int64, 0, len(items)*2)
	seenRevisionIDs := make(map[int64]struct{}, len(items)*2)
	for _, item := range items {
		for _, revisionID := range []*int64{item.CurrentDraftRevisionID, item.CurrentPublishedRevisionID} {
			if revisionID == nil || *revisionID <= 0 {
				continue
			}
			if _, found := seenRevisionIDs[*revisionID]; found {
				continue
			}
			seenRevisionIDs[*revisionID] = struct{}{}
			revisionIDs = append(revisionIDs, *revisionID)
		}
	}
	if len(revisionIDs) > currentRevisionBatchMaxIDs {
		return nil, fmt.Errorf("notification template current revision batch exceeds limit")
	}
	revisions, err := repo.ListTemplateRevisionsByIDs(ctx, revisionIDs)
	if err != nil {
		return nil, err
	}
	revisionsByID := make(map[int64]*domain.TemplateRevision, len(revisions))
	for index := range revisions {
		revision := revisions[index]
		revisionsByID[revision.ID] = &revision
	}
	records := make([]facade.TemplateDefinitionRecord, 0, len(items))
	for _, item := range items {
		var draft, published *domain.TemplateRevision
		if item.CurrentDraftRevisionID != nil {
			draft = revisionsByID[*item.CurrentDraftRevisionID]
		}
		if item.CurrentPublishedRevisionID != nil {
			published = revisionsByID[*item.CurrentPublishedRevisionID]
		}
		records = append(records, *mapTemplateDefinition(item, draft, published))
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	return &facade.PageResult[facade.TemplateDefinitionRecord]{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

func (s *Service) GetTemplateDefinition(ctx context.Context, templateCode string) (*facade.TemplateDefinitionRecord, error) {
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	definition, err := repo.FindTemplateDefinitionByCode(ctx, s.templateRevisionScopeID(), strings.TrimSpace(templateCode))
	if err != nil {
		return nil, err
	}
	if definition == nil {
		return nil, apperrors.NotFound("版本化通知模板不存在")
	}
	return s.mapTemplateDefinitionWithCurrent(ctx, repo, *definition, true)
}

// CreateTemplateDefinition creates one definition and its first editable
// draft. It is a G6.1 authoring record only and never emits an Outbox event.
func (s *Service) CreateTemplateDefinition(ctx context.Context, request facade.TemplateDefinitionCreateRequest, actorID int64) (*facade.TemplateDefinitionRecord, error) {
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(request.TemplateCode)
	if err := domain.ValidateTemplateDefinitionCode(code); err != nil {
		return nil, err
	}
	draft, variablesJSON, digest, err := normalizeTemplateRevisionDraft(request.Draft)
	if err != nil {
		return nil, err
	}
	scopeID := s.templateRevisionScopeID()
	if existing, err := repo.FindTemplateDefinitionByCode(ctx, scopeID, code); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, apperrors.Operation("模板编码已存在")
	}

	definition := domain.TemplateDefinition{
		ID:           s.nextID(),
		ScopeID:      scopeID,
		TemplateCode: code,
		TemplateName: draft.TemplateName,
		Locale:       draft.Locale,
		Version:      1,
		CreatorID:    int64Ptr(actorID),
		UpdaterID:    int64Ptr(actorID),
	}
	revision := domain.TemplateRevision{
		ID:                         s.nextID(),
		TemplateDefinitionID:       definition.ID,
		RevisionNo:                 1,
		State:                      domain.TemplateRevisionStateDraft,
		RevisionVersion:            1,
		SubjectTemplate:            draft.SubjectTemplate,
		TextTemplate:               draft.TextTemplate,
		HTMLTemplate:               draft.HTMLTemplate,
		MarkdownTemplate:           draft.MarkdownTemplate,
		PrimaryActionLabelTemplate: draft.PrimaryActionLabelTemplate,
		VariableSchemaJSON:         variablesJSON,
		ContentDigest:              digest,
		CreatorID:                  int64Ptr(actorID),
		UpdaterID:                  int64Ptr(actorID),
	}
	definition.CurrentDraftRevisionID = int64Ptr(revision.ID)
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := repo.InsertTemplateDefinition(txCtx, &definition); err != nil {
			return err
		}
		if err := repo.InsertTemplateRevision(txCtx, &revision); err != nil {
			return err
		}
		return repo.InsertTemplateRevisionAudit(txCtx, &domain.TemplateRevisionAudit{
			ID:                   s.nextID(),
			TemplateDefinitionID: definition.ID,
			ScopeID:              scopeID,
			Action:               templateRevisionAuditCreateDraft,
			ToRevisionNo:         intPtr(revision.RevisionNo),
			ActorID:              int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	return s.mapTemplateDefinitionWithCurrent(ctx, repo, definition, true)
}

// SaveTemplateRevisionDraft changes only an editable draft. The conditional
// revision-version update is the concurrency boundary; published revisions are
// rejected even if a caller supplies a matching version.
func (s *Service) SaveTemplateRevisionDraft(ctx context.Context, revisionID int64, request facade.TemplateRevisionSaveRequest, actorID int64) (*facade.TemplateDefinitionRecord, error) {
	if revisionID <= 0 || request.ExpectedVersion <= 0 {
		return nil, apperrors.Params("草稿版本不能为空")
	}
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	draft, variablesJSON, digest, err := normalizeTemplateRevisionDraft(request.Draft)
	if err != nil {
		return nil, err
	}
	var definition *domain.TemplateDefinition
	var saved *domain.TemplateRevision
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		current, err := repo.FindTemplateRevisionByID(txCtx, revisionID)
		if err != nil {
			return err
		}
		if current == nil {
			return apperrors.NotFound("模板草稿不存在")
		}
		if current.State != domain.TemplateRevisionStateDraft {
			return domain.ErrTemplateRevisionImmutable
		}
		definition, err = repo.FindTemplateDefinitionByID(txCtx, current.TemplateDefinitionID)
		if err != nil {
			return err
		}
		if !s.templateDefinitionBelongsToCurrentScope(definition) {
			return domain.ErrTemplateDefinitionNotFound
		}
		if definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != current.ID {
			return domain.ErrTemplateRevisionConflict
		}
		current.SubjectTemplate = draft.SubjectTemplate
		current.TextTemplate = draft.TextTemplate
		current.HTMLTemplate = draft.HTMLTemplate
		current.MarkdownTemplate = draft.MarkdownTemplate
		current.PrimaryActionLabelTemplate = draft.PrimaryActionLabelTemplate
		current.VariableSchemaJSON = variablesJSON
		current.ContentDigest = digest
		current.UpdaterID = int64Ptr(actorID)
		changed, err := repo.UpdateTemplateRevisionDraft(txCtx, current, request.ExpectedVersion)
		if err != nil {
			return err
		}
		if !changed {
			return domain.ErrTemplateRevisionConflict
		}
		if err := repo.UpdateTemplateDefinitionMetadata(txCtx, definition.ID, draft.TemplateName, draft.Locale, actorID); err != nil {
			return err
		}
		saved, err = repo.FindTemplateRevisionByID(txCtx, revisionID)
		if err != nil {
			return err
		}
		if saved == nil {
			return domain.ErrTemplateRevisionNotFound
		}
		return repo.InsertTemplateRevisionAudit(txCtx, &domain.TemplateRevisionAudit{
			ID:                   s.nextID(),
			TemplateDefinitionID: definition.ID,
			ScopeID:              s.templateRevisionScopeID(),
			Action:               templateRevisionAuditSaveDraft,
			FromRevisionNo:       intPtr(saved.RevisionNo),
			ToRevisionNo:         intPtr(saved.RevisionNo),
			ActorID:              int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	definition.TemplateName = draft.TemplateName
	definition.Locale = draft.Locale
	return s.mapTemplateDefinitionWithCurrent(ctx, repo, *definition, true)
}

// CreateTemplateDraftFromPublished creates the next revision without changing
// the already-published content. A definition has one active draft at a time.
func (s *Service) CreateTemplateDraftFromPublished(ctx context.Context, templateCode string, actorID int64) (*facade.TemplateDefinitionRecord, error) {
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	var definition *domain.TemplateDefinition
	var published *domain.TemplateRevision
	var draft *domain.TemplateRevision
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		definition, err = repo.LockTemplateDefinitionByCode(txCtx, s.templateRevisionScopeID(), strings.TrimSpace(templateCode))
		if err != nil {
			return err
		}
		if definition == nil {
			return apperrors.NotFound("版本化通知模板不存在")
		}
		if definition.CurrentDraftRevisionID != nil {
			return domain.ErrTemplateRevisionConflict
		}
		published, err = s.currentPublishedTemplateRevision(txCtx, repo, definition)
		if err != nil {
			return err
		}
		if published == nil {
			return apperrors.Operation("尚无可复制的已发布版本")
		}
		draft = &domain.TemplateRevision{
			ID:                         s.nextID(),
			TemplateDefinitionID:       definition.ID,
			RevisionNo:                 published.RevisionNo + 1,
			State:                      domain.TemplateRevisionStateDraft,
			RevisionVersion:            1,
			SubjectTemplate:            published.SubjectTemplate,
			TextTemplate:               published.TextTemplate,
			HTMLTemplate:               published.HTMLTemplate,
			MarkdownTemplate:           published.MarkdownTemplate,
			PrimaryActionLabelTemplate: published.PrimaryActionLabelTemplate,
			VariableSchemaJSON:         published.VariableSchemaJSON,
			ContentDigest:              published.ContentDigest,
			CreatorID:                  int64Ptr(actorID),
			UpdaterID:                  int64Ptr(actorID),
		}
		if err := repo.InsertTemplateRevision(txCtx, draft); err != nil {
			return err
		}
		changed, err := repo.SetTemplateDefinitionDraft(txCtx, definition.ID, draft.ID, definition.Version)
		if err != nil {
			return err
		}
		if !changed {
			return domain.ErrTemplateRevisionConflict
		}
		return repo.InsertTemplateRevisionAudit(txCtx, &domain.TemplateRevisionAudit{
			ID:                   s.nextID(),
			TemplateDefinitionID: definition.ID,
			ScopeID:              s.templateRevisionScopeID(),
			Action:               templateRevisionAuditCreateDraftFromPublished,
			FromRevisionNo:       intPtr(published.RevisionNo),
			ToRevisionNo:         intPtr(draft.RevisionNo),
			ActorID:              int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	definition.CurrentDraftRevisionID = int64Ptr(draft.ID)
	definition.Version++
	return s.mapTemplateDefinitionWithCurrent(ctx, repo, *definition, true)
}

// PublishTemplateRevision atomically promotes the current draft, supersedes
// the old published revision and advances the definition pointers. It does not
// bind or send anything; G6.2 is responsible for later runtime selection.
func (s *Service) PublishTemplateRevision(ctx context.Context, revisionID int64, request facade.TemplateRevisionPublishRequest, actorID int64) (*facade.TemplateDefinitionRecord, error) {
	if revisionID <= 0 || request.ExpectedVersion <= 0 {
		return nil, apperrors.Params("草稿版本不能为空")
	}
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	var definition *domain.TemplateDefinition
	var published *domain.TemplateRevision
	var oldPublished *domain.TemplateRevision
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		candidate, err := repo.FindTemplateRevisionByID(txCtx, revisionID)
		if err != nil {
			return err
		}
		if candidate == nil {
			return apperrors.NotFound("模板草稿不存在")
		}
		candidateDefinition, err := repo.FindTemplateDefinitionByID(txCtx, candidate.TemplateDefinitionID)
		if err != nil {
			return err
		}
		if !s.templateDefinitionBelongsToCurrentScope(candidateDefinition) {
			return domain.ErrTemplateDefinitionNotFound
		}
		definition, err = repo.LockTemplateDefinitionByCode(txCtx, s.templateRevisionScopeID(), candidateDefinition.TemplateCode)
		if err != nil {
			return err
		}
		if definition == nil {
			return domain.ErrTemplateDefinitionNotFound
		}
		candidate, err = repo.FindTemplateRevisionByID(txCtx, revisionID)
		if err != nil {
			return err
		}
		if candidate == nil || candidate.TemplateDefinitionID != definition.ID {
			return domain.ErrTemplateRevisionNotFound
		}
		if candidate.State != domain.TemplateRevisionStateDraft {
			return domain.ErrTemplateRevisionImmutable
		}
		if candidate.RevisionVersion != request.ExpectedVersion || definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != candidate.ID {
			return domain.ErrTemplateRevisionConflict
		}
		oldPublished, err = s.currentPublishedTemplateRevision(txCtx, repo, definition)
		if err != nil {
			return err
		}
		changed, err := repo.PublishTemplateRevision(txCtx, definition.ID, candidate.ID, request.ExpectedVersion, actorID, s.now())
		if err != nil {
			return err
		}
		if !changed {
			return domain.ErrTemplateRevisionConflict
		}
		published, err = repo.FindTemplateRevisionByID(txCtx, candidate.ID)
		if err != nil {
			return err
		}
		if published == nil || published.State != domain.TemplateRevisionStatePublished {
			return domain.ErrTemplateRevisionConflict
		}
		var from *int
		if oldPublished != nil {
			from = intPtr(oldPublished.RevisionNo)
		}
		return repo.InsertTemplateRevisionAudit(txCtx, &domain.TemplateRevisionAudit{
			ID:                   s.nextID(),
			TemplateDefinitionID: definition.ID,
			ScopeID:              s.templateRevisionScopeID(),
			Action:               templateRevisionAuditPublish,
			FromRevisionNo:       from,
			ToRevisionNo:         intPtr(published.RevisionNo),
			ActorID:              int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	definition.CurrentDraftRevisionID = nil
	definition.CurrentPublishedRevisionID = int64Ptr(published.ID)
	definition.Version++
	return s.mapTemplateDefinitionWithCurrent(ctx, repo, *definition, true)
}

// PreviewTemplateRevision renders only the supplied form data. It never looks
// up a saved definition, so it cannot leak another scope's template or mutate
// delivery/inbox/outbox/realtime state.
func (s *Service) PreviewTemplateRevision(_ context.Context, request facade.TemplateRevisionPreviewRequest) (*facade.TemplateRevisionPreviewResponse, error) {
	draft := domainTemplateRevisionDraft(request.Draft)
	rendered, err := domain.RenderTemplateRevision(draft, request.Variables)
	if err != nil {
		return nil, err
	}
	return &facade.TemplateRevisionPreviewResponse{Subject: rendered.Subject, Text: rendered.Text, HTML: rendered.HTML, Markdown: rendered.Markdown, PrimaryActionLabel: rendered.PrimaryActionLabel}, nil
}

func (s *Service) templateRevisionRepository() (domain.TemplateRevisionRepository, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	repo, ok := s.repo.(domain.TemplateRevisionRepository)
	if !ok {
		return nil, fmt.Errorf("notification template revision repository is not configured")
	}
	return repo, nil
}

func (s *Service) templateRevisionScopeID() string {
	if s == nil || strings.TrimSpace(s.scopeID) == "" {
		return "local"
	}
	return strings.TrimSpace(s.scopeID)
}

func (s *Service) templateDefinitionBelongsToCurrentScope(definition *domain.TemplateDefinition) bool {
	return definition != nil && strings.TrimSpace(definition.ScopeID) == s.templateRevisionScopeID()
}

func (s *Service) mapTemplateDefinitionWithCurrent(ctx context.Context, repo domain.TemplateRevisionRepository, definition domain.TemplateDefinition, includeHistory bool) (*facade.TemplateDefinitionRecord, error) {
	var draft, published *domain.TemplateRevision
	var err error
	if definition.CurrentDraftRevisionID != nil {
		draft, err = repo.FindTemplateRevisionByID(ctx, *definition.CurrentDraftRevisionID)
		if err != nil {
			return nil, err
		}
	}
	if definition.CurrentPublishedRevisionID != nil {
		published, err = repo.FindTemplateRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		if err != nil {
			return nil, err
		}
	}
	record := mapTemplateDefinition(definition, draft, published)
	if !includeHistory {
		return record, nil
	}
	revisions, err := repo.ListTemplateRevisionsByDefinition(ctx, definition.ID)
	if err != nil {
		return nil, err
	}
	record.Revisions = mapTemplateRevisions(revisions)
	return record, nil
}

func (s *Service) currentPublishedTemplateRevision(ctx context.Context, repo domain.TemplateRevisionRepository, definition *domain.TemplateDefinition) (*domain.TemplateRevision, error) {
	if definition == nil || definition.CurrentPublishedRevisionID == nil {
		return nil, nil
	}
	revision, err := repo.FindTemplateRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
	if err != nil {
		return nil, err
	}
	if revision != nil && revision.State != domain.TemplateRevisionStatePublished {
		return nil, domain.ErrTemplateRevisionConflict
	}
	return revision, nil
}

func normalizeTemplateRevisionDraft(input facade.TemplateRevisionDraftInput) (domain.TemplateRevisionDraft, string, string, error) {
	draft := domainTemplateRevisionDraft(input)
	if err := domain.ValidateTemplateRevisionDraft(&draft); err != nil {
		return domain.TemplateRevisionDraft{}, "", "", err
	}
	variablesJSON, err := domain.TemplateVariablesJSON(draft.Variables)
	if err != nil {
		return domain.TemplateRevisionDraft{}, "", "", err
	}
	digest, err := domain.TemplateRevisionDigest(draft)
	if err != nil {
		return domain.TemplateRevisionDraft{}, "", "", err
	}
	return draft, variablesJSON, digest, nil
}

func intPtr(value int) *int { return &value }
