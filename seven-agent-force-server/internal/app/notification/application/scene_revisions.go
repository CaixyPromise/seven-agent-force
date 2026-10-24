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
	sceneRevisionAuditCreateDraft              = "CREATE_DRAFT"
	sceneRevisionAuditSaveDraft                = "SAVE_DRAFT"
	sceneRevisionAuditCreateDraftFromPublished = "CREATE_DRAFT_FROM_PUBLISHED"
	sceneRevisionAuditPublish                  = "PUBLISH"
	sceneRevisionAuditStop                     = "STOP"
)

// ListSceneDefinitions lists current versioned scene identities.
func (s *Service) ListSceneDefinitions(ctx context.Context, query domain.SceneDefinitionQuery) (*facade.PageResult[facade.SceneDefinitionRecord], error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	query.ScopeID = s.sceneRevisionScopeID()
	items, total, err := repo.ListSceneDefinitions(ctx, query)
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
		return nil, fmt.Errorf("notification scene current revision batch exceeds limit")
	}
	revisions, err := repo.ListSceneRevisionsByIDs(ctx, revisionIDs)
	if err != nil {
		return nil, err
	}
	revisionsByID := make(map[int64]*domain.SceneRevision, len(revisions))
	for index := range revisions {
		revision := revisions[index]
		revisionsByID[revision.ID] = &revision
	}
	records := make([]facade.SceneDefinitionRecord, 0, len(items))
	for _, item := range items {
		var draft, published *domain.SceneRevision
		if item.CurrentDraftRevisionID != nil {
			draft = revisionsByID[*item.CurrentDraftRevisionID]
		}
		if item.CurrentPublishedRevisionID != nil {
			published = revisionsByID[*item.CurrentPublishedRevisionID]
		}
		records = append(records, *mapSceneDefinition(item, draft, published))
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	return &facade.PageResult[facade.SceneDefinitionRecord]{Records: records, Total: total, Current: current, PageSize: pageSize}, nil
}

// GetSceneDefinition returns G6.2 history only after a scope-bound identity
// lookup. The receiver kind prevents one target category from reading another.
func (s *Service) GetSceneDefinition(ctx context.Context, sceneCode, receiverKind string) (*facade.SceneDefinitionRecord, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	kind, err := domain.NormalizeSceneReceiverKind(receiverKind)
	if err != nil {
		return nil, apperrors.Params(err.Error())
	}
	definition, err := repo.FindSceneDefinitionByCodeAndReceiverKind(ctx, s.sceneRevisionScopeID(), strings.TrimSpace(sceneCode), kind)
	if err != nil {
		return nil, err
	}
	if definition == nil {
		return nil, apperrors.NotFound("新版通知场景不存在")
	}
	return s.mapSceneDefinitionWithCurrent(ctx, repo, *definition, true)
}

// CreateSceneDefinition creates the one stable identity and its first draft.
// It does not publish or emit any Outbox event.
func (s *Service) CreateSceneDefinition(ctx context.Context, request facade.SceneDefinitionCreateRequest, actorID int64) (*facade.SceneDefinitionRecord, error) {
	return s.createSceneDefinition(ctx, request, actorID, sceneRevisionAuditCreateDraft)
}

// createSceneDefinition persists a single scene identity and its first draft.
func (s *Service) createSceneDefinition(ctx context.Context, request facade.SceneDefinitionCreateRequest, actorID int64, auditAction string) (*facade.SceneDefinitionRecord, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	code := strings.TrimSpace(request.SceneCode)
	if err := domain.ValidateSceneDefinitionCode(code); err != nil {
		return nil, apperrors.Params(err.Error())
	}
	draft, err := s.validateSceneRevisionDraft(ctx, request.Draft, true)
	if err != nil {
		return nil, err
	}
	scopeID := s.sceneRevisionScopeID()
	if existing, err := repo.FindSceneDefinitionByCodeAndReceiverKind(ctx, scopeID, code, draft.receiverKind); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, apperrors.Operation("场景编码和接收对象类别已存在")
	}

	definition := domain.SceneDefinition{
		ID:           s.nextID(),
		ScopeID:      scopeID,
		SceneCode:    code,
		SceneName:    draft.sceneName,
		ReceiverKind: draft.receiverKind,
		Version:      1,
		CreatorID:    int64Ptr(actorID),
		UpdaterID:    int64Ptr(actorID),
	}
	revision := domain.SceneRevision{
		ID:                 s.nextID(),
		SceneDefinitionID:  definition.ID,
		RevisionNo:         1,
		State:              domain.SceneRevisionStateDraft,
		RevisionVersion:    1,
		Enabled:            draft.enabled,
		TemplateRevisionID: draft.templateRevision.ID,
		ConnectionRef:      draft.connectionRef,
		ConnectionDigest:   draft.connectionDigest,
		CreatorID:          int64Ptr(actorID),
		UpdaterID:          int64Ptr(actorID),
	}
	definition.CurrentDraftRevisionID = int64Ptr(revision.ID)
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		if err := repo.InsertSceneDefinition(txCtx, &definition); err != nil {
			return err
		}
		if err := repo.InsertSceneRevision(txCtx, &revision); err != nil {
			return err
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definition.ID,
			ScopeID:           scopeID,
			Action:            auditAction,
			ToRevisionNo:      intPtr(revision.RevisionNo),
			ActorID:           int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	return s.mapSceneDefinitionWithCurrent(ctx, repo, definition, true)
}

// SaveSceneRevisionDraft changes only the active editable revision. A draft
// cannot change the immutable scene code or receiver kind identity.
func (s *Service) SaveSceneRevisionDraft(ctx context.Context, revisionID int64, request facade.SceneRevisionSaveRequest, actorID int64) (*facade.SceneDefinitionRecord, error) {
	if revisionID <= 0 || request.ExpectedVersion <= 0 {
		return nil, apperrors.Params("场景草稿版本不能为空")
	}
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	var definition *domain.SceneDefinition
	var saved *domain.SceneRevision
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		current, findErr := repo.FindSceneRevisionByID(txCtx, revisionID)
		if findErr != nil {
			return findErr
		}
		if current == nil {
			return apperrors.NotFound("场景草稿不存在")
		}
		if current.State != domain.SceneRevisionStateDraft {
			return domain.ErrSceneRevisionImmutable
		}
		definition, findErr = repo.FindSceneDefinitionByID(txCtx, current.SceneDefinitionID)
		if findErr != nil {
			return findErr
		}
		if !s.sceneDefinitionBelongsToCurrentScope(definition) {
			return domain.ErrSceneDefinitionNotFound
		}
		if definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != current.ID {
			return domain.ErrSceneRevisionConflict
		}
		draft, validateErr := s.validateSceneRevisionDraft(ctx, request.Draft, true)
		if validateErr != nil {
			return validateErr
		}
		if draft.receiverKind != definition.ReceiverKind {
			return apperrors.Params("场景草稿不能修改接收对象类别")
		}
		current.Enabled = draft.enabled
		current.TemplateRevisionID = draft.templateRevision.ID
		current.ConnectionRef = draft.connectionRef
		current.ConnectionDigest = draft.connectionDigest
		current.UpdaterID = int64Ptr(actorID)
		changed, updateErr := repo.UpdateSceneRevisionDraft(txCtx, current, request.ExpectedVersion)
		if updateErr != nil {
			return updateErr
		}
		if !changed {
			return domain.ErrSceneRevisionConflict
		}
		if err := repo.UpdateSceneDefinitionMetadata(txCtx, definition.ID, draft.sceneName, actorID); err != nil {
			return err
		}
		saved, findErr = repo.FindSceneRevisionByID(txCtx, revisionID)
		if findErr != nil {
			return findErr
		}
		if saved == nil {
			return domain.ErrSceneRevisionNotFound
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definition.ID,
			ScopeID:           s.sceneRevisionScopeID(),
			Action:            sceneRevisionAuditSaveDraft,
			FromRevisionNo:    intPtr(saved.RevisionNo),
			ToRevisionNo:      intPtr(saved.RevisionNo),
			ActorID:           int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	definition.SceneName = strings.TrimSpace(request.Draft.SceneName)
	definition.Version++
	_ = saved
	return s.mapSceneDefinitionWithCurrent(ctx, repo, *definition, true)
}

// CreateSceneDraftFromPublished starts the next immutable version without
// altering the currently published sending behavior.
func (s *Service) CreateSceneDraftFromPublished(ctx context.Context, sceneCode, receiverKind string, actorID int64) (*facade.SceneDefinitionRecord, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	kind, err := domain.NormalizeSceneReceiverKind(receiverKind)
	if err != nil {
		return nil, apperrors.Params(err.Error())
	}
	var definition *domain.SceneDefinition
	var draft *domain.SceneRevision
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		definition, err = repo.LockSceneDefinitionByCodeAndReceiverKind(txCtx, s.sceneRevisionScopeID(), strings.TrimSpace(sceneCode), kind)
		if err != nil {
			return err
		}
		if definition == nil {
			return apperrors.NotFound("新版通知场景不存在")
		}
		if definition.CurrentDraftRevisionID != nil {
			return domain.ErrSceneRevisionConflict
		}
		published, findErr := s.currentPublishedSceneRevision(txCtx, repo, definition)
		if findErr != nil {
			return findErr
		}
		if published == nil {
			return apperrors.Operation("尚无可复制的已发布场景版本")
		}
		draft = &domain.SceneRevision{
			ID:                 s.nextID(),
			SceneDefinitionID:  definition.ID,
			RevisionNo:         published.RevisionNo + 1,
			State:              domain.SceneRevisionStateDraft,
			RevisionVersion:    1,
			Enabled:            published.Enabled,
			TemplateRevisionID: published.TemplateRevisionID,
			ConnectionRef:      published.ConnectionRef,
			ConnectionDigest:   published.ConnectionDigest,
			CreatorID:          int64Ptr(actorID),
			UpdaterID:          int64Ptr(actorID),
		}
		if err := repo.InsertSceneRevision(txCtx, draft); err != nil {
			return err
		}
		changed, setErr := repo.SetSceneDefinitionDraft(txCtx, definition.ID, draft.ID, definition.Version)
		if setErr != nil {
			return setErr
		}
		if !changed {
			return domain.ErrSceneRevisionConflict
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definition.ID,
			ScopeID:           s.sceneRevisionScopeID(),
			Action:            sceneRevisionAuditCreateDraftFromPublished,
			FromRevisionNo:    intPtr(published.RevisionNo),
			ToRevisionNo:      intPtr(draft.RevisionNo),
			ActorID:           int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	definition.CurrentDraftRevisionID = int64Ptr(draft.ID)
	definition.Version++
	return s.mapSceneDefinitionWithCurrent(ctx, repo, *definition, true)
}

// PublishSceneRevision promotes exactly the active draft. It validates that
// the referenced template is already published in the same scope and that the
// one configured connection matches the immutable receiver kind.
func (s *Service) PublishSceneRevision(ctx context.Context, revisionID int64, request facade.SceneRevisionPublishRequest, actorID int64) (*facade.SceneDefinitionRecord, error) {
	if revisionID <= 0 || request.ExpectedVersion <= 0 {
		return nil, apperrors.Params("场景草稿版本不能为空")
	}
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	var definition *domain.SceneDefinition
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		candidate, findErr := repo.FindSceneRevisionByID(txCtx, revisionID)
		if findErr != nil {
			return findErr
		}
		if candidate == nil {
			return apperrors.NotFound("场景草稿不存在")
		}
		candidateDefinition, findErr := repo.FindSceneDefinitionByID(txCtx, candidate.SceneDefinitionID)
		if findErr != nil {
			return findErr
		}
		if !s.sceneDefinitionBelongsToCurrentScope(candidateDefinition) {
			return domain.ErrSceneDefinitionNotFound
		}
		definition, findErr = repo.LockSceneDefinitionByCodeAndReceiverKind(txCtx, s.sceneRevisionScopeID(), candidateDefinition.SceneCode, candidateDefinition.ReceiverKind)
		if findErr != nil {
			return findErr
		}
		if definition == nil {
			return domain.ErrSceneDefinitionNotFound
		}
		candidate, findErr = repo.FindSceneRevisionByID(txCtx, revisionID)
		if findErr != nil {
			return findErr
		}
		if candidate == nil || candidate.SceneDefinitionID != definition.ID {
			return domain.ErrSceneRevisionNotFound
		}
		if candidate.State != domain.SceneRevisionStateDraft {
			return domain.ErrSceneRevisionImmutable
		}
		if candidate.RevisionVersion != request.ExpectedVersion || definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != candidate.ID {
			return domain.ErrSceneRevisionConflict
		}
		normalized, validateErr := s.validateSceneRevisionDraft(txCtx, facade.SceneRevisionDraftInput{
			SceneName:          definition.SceneName,
			ReceiverKind:       definition.ReceiverKind,
			TemplateRevisionID: candidate.TemplateRevisionID,
			ConnectionRef:      candidate.ConnectionRef,
			Enabled:            candidate.Enabled,
		}, candidate.Enabled)
		if validateErr != nil {
			return validateErr
		}
		if normalized.connectionDigest != candidate.ConnectionDigest {
			return apperrors.Operation("场景发送方式已变更，请重新保存草稿")
		}
		oldPublished, findErr := s.currentPublishedSceneRevision(txCtx, repo, definition)
		if findErr != nil {
			return findErr
		}
		changed, publishErr := repo.PublishSceneRevision(txCtx, definition.ID, candidate.ID, request.ExpectedVersion, actorID, s.now())
		if publishErr != nil {
			return publishErr
		}
		if !changed {
			return domain.ErrSceneRevisionConflict
		}
		var from *int
		if oldPublished != nil {
			from = intPtr(oldPublished.RevisionNo)
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definition.ID,
			ScopeID:           s.sceneRevisionScopeID(),
			Action:            sceneRevisionAuditPublish,
			FromRevisionNo:    from,
			ToRevisionNo:      intPtr(candidate.RevisionNo),
			ActorID:           int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	refreshed, err := repo.FindSceneDefinitionByID(ctx, definition.ID)
	if err != nil {
		return nil, err
	}
	if !s.sceneDefinitionBelongsToCurrentScope(refreshed) {
		return nil, domain.ErrSceneDefinitionNotFound
	}
	return s.mapSceneDefinitionWithCurrent(ctx, repo, *refreshed, true)
}

// StopSceneDefinition publishes a next immutable revision with enabled=false.
// It never mutates a published scene in place or silently changes its sender.
func (s *Service) StopSceneDefinition(ctx context.Context, sceneCode, receiverKind string, actorID int64) (*facade.SceneDefinitionRecord, error) {
	repo, err := s.sceneRevisionRepository()
	if err != nil {
		return nil, err
	}
	kind, err := domain.NormalizeSceneReceiverKind(receiverKind)
	if err != nil {
		return nil, apperrors.Params(err.Error())
	}
	var definition *domain.SceneDefinition
	if err := s.withinTx(ctx, func(txCtx context.Context) error {
		definition, err = repo.LockSceneDefinitionByCodeAndReceiverKind(txCtx, s.sceneRevisionScopeID(), strings.TrimSpace(sceneCode), kind)
		if err != nil {
			return err
		}
		if definition == nil {
			return apperrors.NotFound("新版通知场景不存在")
		}
		if definition.CurrentDraftRevisionID != nil {
			return domain.ErrSceneRevisionConflict
		}
		published, findErr := s.currentPublishedSceneRevision(txCtx, repo, definition)
		if findErr != nil {
			return findErr
		}
		if published == nil {
			return apperrors.Operation("尚无可停用的已发布场景")
		}
		draft := &domain.SceneRevision{
			ID:                 s.nextID(),
			SceneDefinitionID:  definition.ID,
			RevisionNo:         published.RevisionNo + 1,
			State:              domain.SceneRevisionStateDraft,
			RevisionVersion:    1,
			Enabled:            false,
			TemplateRevisionID: published.TemplateRevisionID,
			ConnectionRef:      published.ConnectionRef,
			ConnectionDigest:   published.ConnectionDigest,
			CreatorID:          int64Ptr(actorID),
			UpdaterID:          int64Ptr(actorID),
		}
		if err := s.validateStoredSceneRevision(txCtx, definition, draft, false); err != nil {
			return err
		}
		if err := repo.InsertSceneRevision(txCtx, draft); err != nil {
			return err
		}
		changed, setErr := repo.SetSceneDefinitionDraft(txCtx, definition.ID, draft.ID, definition.Version)
		if setErr != nil {
			return setErr
		}
		if !changed {
			return domain.ErrSceneRevisionConflict
		}
		publishedChanged, publishErr := repo.PublishSceneRevision(txCtx, definition.ID, draft.ID, draft.RevisionVersion, actorID, s.now())
		if publishErr != nil {
			return publishErr
		}
		if !publishedChanged {
			return domain.ErrSceneRevisionConflict
		}
		return repo.InsertSceneRevisionAudit(txCtx, &domain.SceneRevisionAudit{
			ID:                s.nextID(),
			SceneDefinitionID: definition.ID,
			ScopeID:           s.sceneRevisionScopeID(),
			Action:            sceneRevisionAuditStop,
			FromRevisionNo:    intPtr(published.RevisionNo),
			ToRevisionNo:      intPtr(draft.RevisionNo),
			ActorID:           int64Ptr(actorID),
		})
	}); err != nil {
		return nil, err
	}
	refreshed, err := repo.FindSceneDefinitionByID(ctx, definition.ID)
	if err != nil {
		return nil, err
	}
	if !s.sceneDefinitionBelongsToCurrentScope(refreshed) {
		return nil, domain.ErrSceneDefinitionNotFound
	}
	return s.mapSceneDefinitionWithCurrent(ctx, repo, *refreshed, true)
}

type normalizedSceneRevisionDraft struct {
	sceneName        string
	receiverKind     string
	templateRevision *domain.TemplateRevision
	connectionRef    string
	connectionDigest string
	enabled          bool
}

func (s *Service) validateSceneRevisionDraft(ctx context.Context, input facade.SceneRevisionDraftInput, requireActiveConnection bool) (normalizedSceneRevisionDraft, error) {
	name := strings.TrimSpace(input.SceneName)
	if name == "" || len(name) > 128 {
		return normalizedSceneRevisionDraft{}, apperrors.Params("场景名称不能为空且长度不能超过128")
	}
	kind, err := domain.NormalizeSceneReceiverKind(input.ReceiverKind)
	if err != nil {
		return normalizedSceneRevisionDraft{}, apperrors.Params(err.Error())
	}
	templateRevision, err := s.validatePublishedTemplateRevision(ctx, input.TemplateRevisionID)
	if err != nil {
		return normalizedSceneRevisionDraft{}, err
	}
	connectionRef := strings.TrimSpace(input.ConnectionRef)
	var channel *domain.Channel
	if kind != domain.SceneReceiverKindInApp {
		channel, err = s.repo.FindChannelByCode(ctx, connectionRef)
		if err != nil {
			return normalizedSceneRevisionDraft{}, err
		}
		if channel == nil || !s.channelBelongsToCurrentScope(channel) {
			return normalizedSceneRevisionDraft{}, apperrors.Operation("场景发送方式不可用")
		}
		if requireActiveConnection && channel.Status != domain.ChannelStatusEnabled {
			return normalizedSceneRevisionDraft{}, apperrors.Operation("场景发送方式已停用")
		}
	}
	if err := domain.ValidateSceneConnection(kind, channel, connectionRef); err != nil {
		return normalizedSceneRevisionDraft{}, apperrors.Params(err.Error())
	}
	if kind != domain.SceneReceiverKindInApp {
		if err := s.validateSceneChannelReadiness(*channel, requireActiveConnection); err != nil {
			return normalizedSceneRevisionDraft{}, err
		}
	}
	return normalizedSceneRevisionDraft{sceneName: name, receiverKind: kind, templateRevision: templateRevision, connectionRef: connectionRef, connectionDigest: domain.SceneConnectionDigest(channel), enabled: input.Enabled}, nil
}

func (s *Service) validatePublishedTemplateRevision(ctx context.Context, revisionID int64) (*domain.TemplateRevision, error) {
	if revisionID <= 0 {
		return nil, apperrors.Params("场景必须选择已发布模板")
	}
	repo, err := s.templateRevisionRepository()
	if err != nil {
		return nil, err
	}
	revision, err := repo.FindTemplateRevisionByID(ctx, revisionID)
	if err != nil {
		return nil, err
	}
	if revision == nil || revision.State != domain.TemplateRevisionStatePublished {
		return nil, apperrors.Params("场景必须选择已发布模板")
	}
	definition, err := repo.FindTemplateDefinitionByID(ctx, revision.TemplateDefinitionID)
	if err != nil {
		return nil, err
	}
	if !s.templateDefinitionBelongsToCurrentScope(definition) {
		return nil, domain.ErrTemplateRevisionNotFound
	}
	variables, err := domain.TemplateVariablesFromJSON(revision.VariableSchemaJSON)
	if err != nil {
		return nil, apperrors.Operation("已发布模板变量配置无效")
	}
	if err := domain.ValidateTemplateRevisionDraft(&domain.TemplateRevisionDraft{TemplateName: definition.TemplateName, Locale: definition.Locale, SubjectTemplate: revision.SubjectTemplate, TextTemplate: revision.TextTemplate, HTMLTemplate: revision.HTMLTemplate, MarkdownTemplate: revision.MarkdownTemplate, Variables: variables}); err != nil {
		return nil, apperrors.Operation("已发布模板内容无效")
	}
	return revision, nil
}

func (s *Service) validateSceneChannelReadiness(channel domain.Channel, requireActive bool) error {
	if requireActive && channel.Status != domain.ChannelStatusEnabled {
		return apperrors.Operation("场景发送方式已停用")
	}
	if domain.IsEnterpriseApplicationChannelType(channel.ChannelType) {
		if channel.EnterpriseConnectionID != nil {
			if channel.ScopeType != "ORGANIZATION" || channel.OrganizationID == nil || *channel.OrganizationID <= 0 || *channel.EnterpriseConnectionID <= 0 || channel.EnterpriseConnectionVersion <= 0 || strings.TrimSpace(channel.EnterpriseConnectionDigest) == "" {
				return apperrors.Operation("企业身份连接引用不完整")
			}
			return nil
		}
		if _, err := domain.ParseEnterpriseApplicationConfig(channel.ChannelType, channel.ConfigJSON); err != nil {
			return apperrors.Operation("企业应用连接配置不完整")
		}
		if strings.TrimSpace(channel.SecretCiphertext) == "" || strings.TrimSpace(channel.SecretEDEK) == "" || strings.TrimSpace(channel.SecretWrapKeyRef) == "" {
			return apperrors.Operation("企业应用连接密钥未配置")
		}
		return nil
	}
	if domain.IsStaticHTTPChannelType(channel.ChannelType) {
		if _, err := staticHTTPSnapshotFromChannel(1, "scene-check", channel, s.scopeID); err != nil {
			return apperrors.Operation("受控 HTTP 连接配置不完整")
		}
	}
	return nil
}

func (s *Service) validateStoredSceneRevision(ctx context.Context, definition *domain.SceneDefinition, revision *domain.SceneRevision, requireActiveConnection bool) error {
	if definition == nil || revision == nil {
		return domain.ErrSceneRevisionNotFound
	}
	_, templateRevision, err := s.loadSceneTemplateRevision(ctx, revision.TemplateRevisionID)
	if err != nil {
		return err
	}
	if templateRevision == nil {
		return apperrors.Operation("场景引用的模板版本不可用")
	}
	connectionRef := strings.TrimSpace(revision.ConnectionRef)
	var channel *domain.Channel
	if definition.ReceiverKind != domain.SceneReceiverKindInApp {
		channel, err = s.repo.FindChannelByCode(ctx, connectionRef)
		if err != nil {
			return err
		}
		if channel == nil || !s.channelBelongsToCurrentScope(channel) {
			return apperrors.Operation("场景发送方式不可用")
		}
		if err := s.validateSceneChannelReadiness(*channel, requireActiveConnection); err != nil {
			return err
		}
	}
	if err := domain.ValidateSceneConnection(definition.ReceiverKind, channel, connectionRef); err != nil {
		return apperrors.Params(err.Error())
	}
	if domain.SceneConnectionDigest(channel) != revision.ConnectionDigest {
		return apperrors.Operation("场景发送方式已变更，请重新保存草稿")
	}
	return nil
}

func (s *Service) sceneRevisionRepository() (domain.SceneRevisionRepository, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("notification service is not configured")
	}
	repo, ok := s.repo.(domain.SceneRevisionRepository)
	if !ok {
		return nil, fmt.Errorf("notification scene revision repository is not configured")
	}
	return repo, nil
}

func (s *Service) sceneRevisionScopeID() string {
	return s.templateRevisionScopeID()
}

func (s *Service) sceneDefinitionBelongsToCurrentScope(definition *domain.SceneDefinition) bool {
	return definition != nil && strings.TrimSpace(definition.ScopeID) == s.sceneRevisionScopeID()
}

func (s *Service) currentPublishedSceneRevision(ctx context.Context, repo domain.SceneRevisionRepository, definition *domain.SceneDefinition) (*domain.SceneRevision, error) {
	if definition == nil || definition.CurrentPublishedRevisionID == nil {
		return nil, nil
	}
	revision, err := repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
	if err != nil {
		return nil, err
	}
	if revision != nil && revision.State != domain.SceneRevisionStatePublished {
		return nil, domain.ErrSceneRevisionConflict
	}
	return revision, nil
}

func (s *Service) mapSceneDefinitionWithCurrent(ctx context.Context, repo domain.SceneRevisionRepository, definition domain.SceneDefinition, includeHistory bool) (*facade.SceneDefinitionRecord, error) {
	var draft, published *domain.SceneRevision
	var err error
	if definition.CurrentDraftRevisionID != nil {
		draft, err = repo.FindSceneRevisionByID(ctx, *definition.CurrentDraftRevisionID)
		if err != nil {
			return nil, err
		}
	}
	if definition.CurrentPublishedRevisionID != nil {
		published, err = repo.FindSceneRevisionByID(ctx, *definition.CurrentPublishedRevisionID)
		if err != nil {
			return nil, err
		}
	}
	record := mapSceneDefinition(definition, draft, published)
	if !includeHistory {
		return record, nil
	}
	revisions, err := repo.ListSceneRevisionsByDefinition(ctx, definition.ID)
	if err != nil {
		return nil, err
	}
	record.Revisions = mapSceneRevisions(revisions, definition.ReceiverKind)
	return record, nil
}
