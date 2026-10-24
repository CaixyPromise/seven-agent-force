package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
)

func TestTemplateRevisionLifecycleIsScopedImmutableAndPreviewOnly(t *testing.T) {
	repo := newTemplateRevisionMemoryRepository()
	service := NewService(externalTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("scope-a")

	created, err := service.CreateTemplateDefinition(context.Background(), facade.TemplateDefinitionCreateRequest{
		TemplateCode: "account_notice",
		Draft:        templateRevisionInput("账户提醒", "{{.name}}，余额 {{.amount}}"),
	}, 11)
	if err != nil {
		t.Fatalf("CreateTemplateDefinition() error=%v", err)
	}
	if created.CurrentDraft == nil || created.CurrentDraft.RevisionNo != 1 || created.CurrentPublished != nil {
		t.Fatalf("unexpected create result: %+v", created)
	}
	if len(repo.definitions) != 1 || len(repo.revisions) != 1 || len(repo.audits) != 1 {
		t.Fatalf("unexpected durable writes: defs=%d revisions=%d audits=%d", len(repo.definitions), len(repo.revisions), len(repo.audits))
	}

	beforePreviewDefinitions, beforePreviewRevisions, beforePreviewAudits := len(repo.definitions), len(repo.revisions), len(repo.audits)
	preview, err := service.PreviewTemplateRevision(context.Background(), facade.TemplateRevisionPreviewRequest{
		Draft:     templateRevisionInput("临时预览", "{{.name}}，余额 {{.amount}}"),
		Variables: map[string]any{"name": "小七", "amount": 18.5},
	})
	if err != nil {
		t.Fatalf("PreviewTemplateRevision() error=%v", err)
	}
	if preview.Text != "小七，余额 18.5" || len(repo.definitions) != beforePreviewDefinitions || len(repo.revisions) != beforePreviewRevisions || len(repo.audits) != beforePreviewAudits {
		t.Fatalf("preview must be side-effect free: preview=%+v defs=%d revisions=%d audits=%d", preview, len(repo.definitions), len(repo.revisions), len(repo.audits))
	}

	saved, err := service.SaveTemplateRevisionDraft(context.Background(), created.CurrentDraft.ID, facade.TemplateRevisionSaveRequest{
		ExpectedVersion: created.CurrentDraft.RevisionVersion,
		Draft:           templateRevisionInput("账户提醒更新", "{{.name}}，新余额 {{.amount}}"),
	}, 12)
	if err != nil {
		t.Fatalf("SaveTemplateRevisionDraft() error=%v", err)
	}
	if saved.CurrentDraft == nil || saved.CurrentDraft.RevisionVersion != 2 || saved.TemplateName != "账户提醒更新" {
		t.Fatalf("unexpected saved draft: %+v", saved)
	}
	_, err = service.SaveTemplateRevisionDraft(context.Background(), created.CurrentDraft.ID, facade.TemplateRevisionSaveRequest{
		ExpectedVersion: 1,
		Draft:           templateRevisionInput("应冲突", "{{.name}}，余额 {{.amount}}"),
	}, 13)
	if !errors.Is(err, domain.ErrTemplateRevisionConflict) {
		t.Fatalf("stale draft save error=%v, want conflict", err)
	}

	published, err := service.PublishTemplateRevision(context.Background(), saved.CurrentDraft.ID, facade.TemplateRevisionPublishRequest{ExpectedVersion: saved.CurrentDraft.RevisionVersion}, 14)
	if err != nil {
		t.Fatalf("PublishTemplateRevision() error=%v", err)
	}
	if published.CurrentDraft != nil || published.CurrentPublished == nil || published.CurrentPublished.State != domain.TemplateRevisionStatePublished {
		t.Fatalf("unexpected published result: %+v", published)
	}
	_, err = service.SaveTemplateRevisionDraft(context.Background(), published.CurrentPublished.ID, facade.TemplateRevisionSaveRequest{
		ExpectedVersion: published.CurrentPublished.RevisionVersion,
		Draft:           templateRevisionInput("不应修改", "{{.name}}，余额 {{.amount}}"),
	}, 15)
	if !errors.Is(err, domain.ErrTemplateRevisionImmutable) {
		t.Fatalf("published draft save error=%v, want immutable", err)
	}

	next, err := service.CreateTemplateDraftFromPublished(context.Background(), "account_notice", 16)
	if err != nil {
		t.Fatalf("CreateTemplateDraftFromPublished() error=%v", err)
	}
	if next.CurrentDraft == nil || next.CurrentDraft.RevisionNo != 2 || next.CurrentPublished == nil || next.CurrentPublished.RevisionNo != 1 {
		t.Fatalf("published clone changed history: %+v", next)
	}
	publishedNext, err := service.PublishTemplateRevision(context.Background(), next.CurrentDraft.ID, facade.TemplateRevisionPublishRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion}, 17)
	if err != nil {
		t.Fatalf("PublishTemplateRevision(next) error=%v", err)
	}
	if len(publishedNext.Revisions) != 2 || publishedNext.Revisions[0].RevisionNo != 2 || publishedNext.Revisions[0].State != domain.TemplateRevisionStatePublished || publishedNext.Revisions[1].RevisionNo != 1 || publishedNext.Revisions[1].State != domain.TemplateRevisionStateSuperseded {
		t.Fatalf("published history was not readable and immutable: %+v", publishedNext.Revisions)
	}
	detail, err := service.GetTemplateDefinition(context.Background(), "account_notice")
	if err != nil || len(detail.Revisions) != 2 || detail.Revisions[1].TextTemplate != "{{.name}}，新余额 {{.amount}}" {
		t.Fatalf("GetTemplateDefinition() history=%+v error=%v", detail, err)
	}

	otherScope := NewService(externalTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	otherScope.SetScopeID("scope-b")
	if _, err := otherScope.GetTemplateDefinition(context.Background(), "account_notice"); err == nil {
		t.Fatal("other scope read unexpectedly succeeded")
	}
}

func TestListTemplateDefinitionsLoadsCurrentRevisionsInOneBoundedBatch(t *testing.T) {
	repo := newTemplateRevisionMemoryRepository()
	for definitionID := int64(1); definitionID <= 3; definitionID++ {
		draftID := definitionID * 10
		publishedID := draftID + 1
		repo.definitions[definitionID] = &domain.TemplateDefinition{
			ID:                         definitionID,
			ScopeID:                    "scope-a",
			TemplateCode:               fmt.Sprintf("template_%d", definitionID),
			TemplateName:               fmt.Sprintf("Template %d", definitionID),
			Locale:                     "zh-CN",
			CurrentDraftRevisionID:     int64Ptr(draftID),
			CurrentPublishedRevisionID: int64Ptr(publishedID),
		}
		repo.revisions[draftID] = &domain.TemplateRevision{ID: draftID, TemplateDefinitionID: definitionID, State: domain.TemplateRevisionStateDraft, VariableSchemaJSON: "[]"}
		repo.revisions[publishedID] = &domain.TemplateRevision{ID: publishedID, TemplateDefinitionID: definitionID, State: domain.TemplateRevisionStatePublished, VariableSchemaJSON: "[]"}
	}
	service := NewService(externalTestTransactor{}, repo, domain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID("scope-a")

	page, err := service.ListTemplateDefinitions(context.Background(), domain.TemplateDefinitionQuery{Current: 1, PageSize: 200})
	if err != nil {
		t.Fatalf("ListTemplateDefinitions() error=%v", err)
	}
	if len(page.Records) != 3 {
		t.Fatalf("records=%d, want 3", len(page.Records))
	}
	if repo.findRevisionByIDCalls != 0 || repo.listRevisionsByIDsCalls != 1 {
		t.Fatalf("revision queries: FindByID=%d ListByIDs=%d, want 0 and 1", repo.findRevisionByIDCalls, repo.listRevisionsByIDsCalls)
	}
	if got := len(repo.lastRevisionBatchIDs); got != 6 {
		t.Fatalf("batched revision IDs=%d, want 6", got)
	}
}

func templateRevisionInput(name, text string) facade.TemplateRevisionDraftInput {
	return facade.TemplateRevisionDraftInput{
		TemplateName:    name,
		Locale:          "zh-CN",
		SubjectTemplate: "{{.name}}",
		TextTemplate:    text,
		Variables: []facade.TemplateRevisionVariable{
			{Name: "name", Type: domain.TemplateVariableTypeString, Required: true, MaxLength: 80, Classification: domain.TemplateVariableClassificationPublic},
			{Name: "amount", Type: domain.TemplateVariableTypeNumber, Required: false, Classification: domain.TemplateVariableClassificationPublic},
		},
	}
}

type templateRevisionMemoryRepository struct {
	domain.Repository
	definitions             map[int64]*domain.TemplateDefinition
	revisions               map[int64]*domain.TemplateRevision
	audits                  []*domain.TemplateRevisionAudit
	legacy                  map[string]*domain.Template
	findRevisionByIDCalls   int
	listRevisionsByIDsCalls int
	lastRevisionBatchIDs    []int64
}

func newTemplateRevisionMemoryRepository() *templateRevisionMemoryRepository {
	return &templateRevisionMemoryRepository{
		definitions: map[int64]*domain.TemplateDefinition{},
		revisions:   map[int64]*domain.TemplateRevision{},
		legacy:      map[string]*domain.Template{},
	}
}

func (r *templateRevisionMemoryRepository) ListTemplateDefinitions(_ context.Context, query domain.TemplateDefinitionQuery) ([]domain.TemplateDefinition, int64, error) {
	items := make([]domain.TemplateDefinition, 0)
	for _, item := range r.definitions {
		if item.ScopeID != query.ScopeID || item.IsDeleted != 0 {
			continue
		}
		if query.Keyword != "" && !strings.Contains(item.TemplateCode, query.Keyword) && !strings.Contains(item.TemplateName, query.Keyword) {
			continue
		}
		items = append(items, *cloneTemplateDefinition(item))
	}
	return items, int64(len(items)), nil
}

func (r *templateRevisionMemoryRepository) FindTemplateDefinitionByCode(_ context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	for _, item := range r.definitions {
		if item.ScopeID == scopeID && item.TemplateCode == templateCode && item.IsDeleted == 0 {
			return cloneTemplateDefinition(item), nil
		}
	}
	return nil, nil
}

func (r *templateRevisionMemoryRepository) FindTemplateDefinitionByID(_ context.Context, definitionID int64) (*domain.TemplateDefinition, error) {
	return cloneTemplateDefinition(r.definitions[definitionID]), nil
}

func (r *templateRevisionMemoryRepository) LockTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	return r.FindTemplateDefinitionByCode(ctx, scopeID, templateCode)
}

func (r *templateRevisionMemoryRepository) FindTemplateRevisionByID(_ context.Context, revisionID int64) (*domain.TemplateRevision, error) {
	r.findRevisionByIDCalls++
	return cloneTemplateRevision(r.revisions[revisionID]), nil
}

func (r *templateRevisionMemoryRepository) ListTemplateRevisionsByIDs(_ context.Context, revisionIDs []int64) ([]domain.TemplateRevision, error) {
	r.listRevisionsByIDsCalls++
	r.lastRevisionBatchIDs = append([]int64(nil), revisionIDs...)
	items := make([]domain.TemplateRevision, 0, len(revisionIDs))
	for _, revisionID := range revisionIDs {
		if item := r.revisions[revisionID]; item != nil {
			items = append(items, *cloneTemplateRevision(item))
		}
	}
	return items, nil
}

func (r *templateRevisionMemoryRepository) FindTemplateRevisionByDefinitionAndState(_ context.Context, definitionID int64, state string) (*domain.TemplateRevision, error) {
	var best *domain.TemplateRevision
	for _, item := range r.revisions {
		if item.TemplateDefinitionID == definitionID && item.State == state && (best == nil || item.RevisionNo > best.RevisionNo) {
			best = item
		}
	}
	return cloneTemplateRevision(best), nil
}

func (r *templateRevisionMemoryRepository) ListTemplateRevisionsByDefinition(_ context.Context, definitionID int64) ([]domain.TemplateRevision, error) {
	items := make([]domain.TemplateRevision, 0)
	for _, item := range r.revisions {
		if item.TemplateDefinitionID == definitionID {
			items = append(items, *cloneTemplateRevision(item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].RevisionNo == items[j].RevisionNo {
			return items[i].ID > items[j].ID
		}
		return items[i].RevisionNo > items[j].RevisionNo
	})
	return items, nil
}

func (r *templateRevisionMemoryRepository) InsertTemplateDefinition(_ context.Context, item *domain.TemplateDefinition) error {
	if item == nil {
		return errors.New("nil definition")
	}
	if _, found := r.definitions[item.ID]; found {
		return errors.New("definition already exists")
	}
	copy := cloneTemplateDefinition(item)
	copy.CreateTime = time.Now()
	copy.UpdateTime = copy.CreateTime
	r.definitions[copy.ID] = copy
	return nil
}

func (r *templateRevisionMemoryRepository) InsertTemplateRevision(_ context.Context, item *domain.TemplateRevision) error {
	if item == nil {
		return errors.New("nil revision")
	}
	if _, found := r.revisions[item.ID]; found {
		return errors.New("revision already exists")
	}
	copy := cloneTemplateRevision(item)
	copy.CreateTime = time.Now()
	copy.UpdateTime = copy.CreateTime
	r.revisions[copy.ID] = copy
	return nil
}

func (r *templateRevisionMemoryRepository) UpdateTemplateDefinitionMetadata(_ context.Context, definitionID int64, templateName, locale string, actorID int64) error {
	item := r.definitions[definitionID]
	if item == nil {
		return domain.ErrTemplateDefinitionNotFound
	}
	item.TemplateName = templateName
	item.Locale = locale
	item.UpdaterID = int64Ptr(actorID)
	item.UpdateTime = time.Now()
	return nil
}

func (r *templateRevisionMemoryRepository) UpdateTemplateRevisionDraft(_ context.Context, item *domain.TemplateRevision, expectedVersion int) (bool, error) {
	current := r.revisions[item.ID]
	if current == nil || current.State != domain.TemplateRevisionStateDraft || current.RevisionVersion != expectedVersion {
		return false, nil
	}
	current.SubjectTemplate = item.SubjectTemplate
	current.TextTemplate = item.TextTemplate
	current.HTMLTemplate = item.HTMLTemplate
	current.MarkdownTemplate = item.MarkdownTemplate
	current.VariableSchemaJSON = item.VariableSchemaJSON
	current.ContentDigest = item.ContentDigest
	current.UpdaterID = item.UpdaterID
	current.RevisionVersion++
	current.UpdateTime = time.Now()
	return true, nil
}

func (r *templateRevisionMemoryRepository) SetTemplateDefinitionDraft(_ context.Context, definitionID, revisionID int64, expectedDefinitionVersion int) (bool, error) {
	definition := r.definitions[definitionID]
	if definition == nil || definition.CurrentDraftRevisionID != nil || definition.Version != expectedDefinitionVersion {
		return false, nil
	}
	definition.CurrentDraftRevisionID = int64Ptr(revisionID)
	definition.Version++
	definition.UpdateTime = time.Now()
	return true, nil
}

func (r *templateRevisionMemoryRepository) PublishTemplateRevision(_ context.Context, definitionID, revisionID int64, expectedRevisionVersion int, actorID int64, publishedAt time.Time) (bool, error) {
	definition := r.definitions[definitionID]
	candidate := r.revisions[revisionID]
	if definition == nil || candidate == nil || definition.CurrentDraftRevisionID == nil || *definition.CurrentDraftRevisionID != revisionID || candidate.State != domain.TemplateRevisionStateDraft || candidate.RevisionVersion != expectedRevisionVersion {
		return false, nil
	}
	for _, item := range r.revisions {
		if item.TemplateDefinitionID == definitionID && item.State == domain.TemplateRevisionStatePublished {
			item.State = domain.TemplateRevisionStateSuperseded
		}
	}
	candidate.State = domain.TemplateRevisionStatePublished
	candidate.RevisionVersion++
	candidate.PublishedAt = &publishedAt
	candidate.PublishedBy = int64Ptr(actorID)
	candidate.UpdaterID = int64Ptr(actorID)
	candidate.UpdateTime = time.Now()
	definition.CurrentDraftRevisionID = nil
	definition.CurrentPublishedRevisionID = int64Ptr(revisionID)
	definition.Version++
	definition.UpdaterID = int64Ptr(actorID)
	definition.UpdateTime = time.Now()
	return true, nil
}

func (r *templateRevisionMemoryRepository) InsertTemplateRevisionAudit(_ context.Context, item *domain.TemplateRevisionAudit) error {
	if item == nil {
		return errors.New("nil audit")
	}
	copy := *item
	copy.CreateTime = time.Now()
	r.audits = append(r.audits, &copy)
	return nil
}

func (r *templateRevisionMemoryRepository) FindTemplateByCode(_ context.Context, templateCode string) (*domain.Template, error) {
	item := r.legacy[templateCode]
	if item == nil {
		return nil, nil
	}
	copy := *item
	return &copy, nil
}

func cloneTemplateDefinition(item *domain.TemplateDefinition) *domain.TemplateDefinition {
	if item == nil {
		return nil
	}
	copy := *item
	if item.CurrentDraftRevisionID != nil {
		copy.CurrentDraftRevisionID = int64Ptr(*item.CurrentDraftRevisionID)
	}
	if item.CurrentPublishedRevisionID != nil {
		copy.CurrentPublishedRevisionID = int64Ptr(*item.CurrentPublishedRevisionID)
	}
	return &copy
}

func cloneTemplateRevision(item *domain.TemplateRevision) *domain.TemplateRevision {
	if item == nil {
		return nil
	}
	copy := *item
	if item.PublishedAt != nil {
		value := *item.PublishedAt
		copy.PublishedAt = &value
	}
	if item.PublishedBy != nil {
		copy.PublishedBy = int64Ptr(*item.PublishedBy)
	}
	return &copy
}
