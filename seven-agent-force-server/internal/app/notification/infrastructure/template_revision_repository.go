package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

// ListTemplateDefinitions reads only the G6.1 workspace. The legacy runtime
// table sys_notification_template is intentionally not part of this query.
func (r *Repository) ListTemplateDefinitions(ctx context.Context, query domain.TemplateDefinitionQuery) ([]domain.TemplateDefinition, int64, error) {
	where, args := []string{"isDeleted=0", "scopeId=?"}, []any{strings.TrimSpace(query.ScopeID)}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		where = append(where, "(templateCode LIKE ? OR templateName LIKE ?)")
		keyword = "%" + keyword + "%"
		args = append(args, keyword, keyword)
	}
	return selectInboxPage[domain.TemplateDefinition](ctx, r, templateDefinitionSelectBase(), where, "updateTime DESC, id DESC", query.Current, query.PageSize, args...)
}

func (r *Repository) FindTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	var item domain.TemplateDefinition
	query := templateDefinitionSelectBase() + " WHERE scopeId=? AND templateCode=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(scopeID), strings.TrimSpace(templateCode)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindTemplateDefinitionByID(ctx context.Context, definitionID int64) (*domain.TemplateDefinition, error) {
	if definitionID <= 0 {
		return nil, nil
	}
	var item domain.TemplateDefinition
	query := templateDefinitionSelectBase() + " WHERE id=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), definitionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// LockTemplateDefinitionByCode is used only under the application service's
// transaction boundary before a draft clone or publish pointer transition.
func (r *Repository) LockTemplateDefinitionByCode(ctx context.Context, scopeID, templateCode string) (*domain.TemplateDefinition, error) {
	var item domain.TemplateDefinition
	query := templateDefinitionSelectBase() + " WHERE scopeId=? AND templateCode=? AND isDeleted=0 LIMIT 1 FOR UPDATE"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(scopeID), strings.TrimSpace(templateCode)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindTemplateRevisionByID(ctx context.Context, revisionID int64) (*domain.TemplateRevision, error) {
	if revisionID <= 0 {
		return nil, nil
	}
	var item domain.TemplateRevision
	query := templateRevisionSelectBase(r) + " WHERE id=? LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), revisionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListTemplateRevisionsByIDs(ctx context.Context, revisionIDs []int64) ([]domain.TemplateRevision, error) {
	revisionIDs = uniquePositiveRevisionIDs(revisionIDs)
	if len(revisionIDs) == 0 {
		return []domain.TemplateRevision{}, nil
	}
	if len(revisionIDs) > 400 {
		return nil, fmt.Errorf("notification template revision batch exceeds limit")
	}
	query, args, err := sqlx.In(templateRevisionSelectBase(r)+" WHERE id IN (?) ORDER BY id ASC", revisionIDs)
	if err != nil {
		return nil, fmt.Errorf("build notification template revision batch query: %w", err)
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	items := make([]domain.TemplateRevision, 0, len(revisionIDs))
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		return nil, fmt.Errorf("list notification template revisions by id: %w", err)
	}
	return items, nil
}

func (r *Repository) FindTemplateRevisionByDefinitionAndState(ctx context.Context, definitionID int64, state string) (*domain.TemplateRevision, error) {
	if definitionID <= 0 || strings.TrimSpace(state) == "" {
		return nil, nil
	}
	var item domain.TemplateRevision
	query := templateRevisionSelectBase(r) + " WHERE templateDefinitionId=? AND state=? ORDER BY revisionNo DESC LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), definitionID, strings.ToUpper(strings.TrimSpace(state))); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// ListTemplateRevisionsByDefinition keeps published and superseded template
// content readable. It deliberately has no state filter: history is part of
// the G6.1 authoring record, not a runtime delivery selector.
func (r *Repository) ListTemplateRevisionsByDefinition(ctx context.Context, definitionID int64) ([]domain.TemplateRevision, error) {
	if definitionID <= 0 {
		return []domain.TemplateRevision{}, nil
	}
	items := make([]domain.TemplateRevision, 0)
	query := templateRevisionSelectBase(r) + " WHERE templateDefinitionId=? ORDER BY revisionNo DESC, id DESC"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), definitionID); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) InsertTemplateDefinition(ctx context.Context, item *domain.TemplateDefinition) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.TemplateCode) == "" {
		return fmt.Errorf("notification template definition is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_template_definition (id, scopeId, templateCode, templateName, locale, currentDraftRevisionId, currentPublishedRevisionId, version, creatorId, updaterId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.ScopeID, item.TemplateCode, item.TemplateName, item.Locale, item.CurrentDraftRevisionID, item.CurrentPublishedRevisionID, item.Version, item.CreatorID, item.UpdaterID)
	return err
}

func (r *Repository) InsertTemplateRevision(ctx context.Context, item *domain.TemplateRevision) error {
	if item == nil || item.ID <= 0 || item.TemplateDefinitionID <= 0 || item.RevisionNo <= 0 || strings.TrimSpace(item.State) == "" || strings.TrimSpace(item.VariableSchemaJSON) == "" || strings.TrimSpace(item.ContentDigest) == "" {
		return fmt.Errorf("notification template revision is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_template_revision (id, templateDefinitionId, revisionNo, state, revisionVersion, subjectTemplate, textTemplate, htmlTemplate, markdownTemplate, primaryActionLabelTemplate, variableSchemaJson, contentDigest, publishedAt, publishedBy, creatorId, updaterId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.TemplateDefinitionID, item.RevisionNo, item.State, item.RevisionVersion, nullIfBlank(item.SubjectTemplate), nullIfBlank(item.TextTemplate), nullIfBlank(item.HTMLTemplate), nullIfBlank(item.MarkdownTemplate), nullIfBlank(item.PrimaryActionLabelTemplate), item.VariableSchemaJSON, item.ContentDigest, item.PublishedAt, item.PublishedBy, item.CreatorID, item.UpdaterID)
	return err
}

func (r *Repository) UpdateTemplateDefinitionMetadata(ctx context.Context, definitionID int64, templateName, locale string, actorID int64) error {
	if definitionID <= 0 || strings.TrimSpace(templateName) == "" || strings.TrimSpace(locale) == "" {
		return fmt.Errorf("notification template definition metadata is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_template_definition SET templateName=?, locale=?, updaterId=?, updateTime=NOW() WHERE id=? AND isDeleted=0`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(templateName), strings.TrimSpace(locale), actorID, definitionID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.ErrTemplateDefinitionNotFound
	}
	return nil
}

func (r *Repository) UpdateTemplateRevisionDraft(ctx context.Context, item *domain.TemplateRevision, expectedVersion int) (bool, error) {
	if item == nil || item.ID <= 0 || expectedVersion <= 0 {
		return false, fmt.Errorf("notification template draft update is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_template_revision SET subjectTemplate=?, textTemplate=?, htmlTemplate=?, markdownTemplate=?, primaryActionLabelTemplate=?, variableSchemaJson=?, contentDigest=?, revisionVersion=revisionVersion+1, updaterId=?, updateTime=NOW() WHERE id=? AND state=? AND revisionVersion=?`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), nullIfBlank(item.SubjectTemplate), nullIfBlank(item.TextTemplate), nullIfBlank(item.HTMLTemplate), nullIfBlank(item.MarkdownTemplate), nullIfBlank(item.PrimaryActionLabelTemplate), item.VariableSchemaJSON, item.ContentDigest, item.UpdaterID, item.ID, domain.TemplateRevisionStateDraft, expectedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *Repository) SetTemplateDefinitionDraft(ctx context.Context, definitionID, revisionID int64, expectedDefinitionVersion int) (bool, error) {
	if definitionID <= 0 || revisionID <= 0 || expectedDefinitionVersion <= 0 {
		return false, fmt.Errorf("notification template definition draft pointer is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_template_definition SET currentDraftRevisionId=?, version=version+1, updateTime=NOW() WHERE id=? AND version=? AND currentDraftRevisionId IS NULL AND isDeleted=0`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), revisionID, definitionID, expectedDefinitionVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// PublishTemplateRevision relies on the caller holding the definition row
// lock inside a transaction. The conditional draft update is still retained as
// a fencing check so a stale publish cannot claim a later saved draft.
func (r *Repository) PublishTemplateRevision(ctx context.Context, definitionID, revisionID int64, expectedRevisionVersion int, actorID int64, publishedAt time.Time) (bool, error) {
	if definitionID <= 0 || revisionID <= 0 || expectedRevisionVersion <= 0 || publishedAt.IsZero() {
		return false, fmt.Errorf("notification template publish is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	publishQuery := `UPDATE sys_notification_template_revision SET state=?, revisionVersion=revisionVersion+1, publishedAt=?, publishedBy=?, updaterId=?, updateTime=NOW() WHERE id=? AND templateDefinitionId=? AND state=? AND revisionVersion=?`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(publishQuery)), domain.TemplateRevisionStatePublished, publishedAt, actorID, actorID, revisionID, definitionID, domain.TemplateRevisionStateDraft, expectedRevisionVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	// Only the one current published pointer can be superseded. The definition
	// lock makes this precise; the state predicate also makes a repeated call a
	// harmless conflict instead of a duplicate historical mutation.
	if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_template_revision SET state=?, updateTime=NOW() WHERE templateDefinitionId=? AND id<>? AND state=?`)), domain.TemplateRevisionStateSuperseded, definitionID, revisionID, domain.TemplateRevisionStatePublished); err != nil {
		return false, err
	}
	pointerQuery := `UPDATE sys_notification_template_definition SET currentDraftRevisionId=NULL, currentPublishedRevisionId=?, version=version+1, updaterId=?, updateTime=NOW() WHERE id=? AND currentDraftRevisionId=? AND isDeleted=0`
	pointerResult, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(pointerQuery)), revisionID, actorID, definitionID, revisionID)
	if err != nil {
		return false, err
	}
	pointerRows, err := pointerResult.RowsAffected()
	if err != nil || pointerRows == 0 {
		return false, err
	}
	return true, nil
}

func (r *Repository) InsertTemplateRevisionAudit(ctx context.Context, item *domain.TemplateRevisionAudit) error {
	if item == nil || item.ID <= 0 || item.TemplateDefinitionID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.Action) == "" {
		return fmt.Errorf("notification template revision audit is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_template_revision_audit (id, templateDefinitionId, scopeId, action, fromRevisionNo, toRevisionNo, actorId) VALUES (?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.TemplateDefinitionID, item.ScopeID, item.Action, item.FromRevisionNo, item.ToRevisionNo, item.ActorID)
	return err
}

func templateDefinitionSelectBase() string {
	return `SELECT id, scopeId, templateCode, templateName, locale, currentDraftRevisionId, currentPublishedRevisionId, version, creatorId, updaterId, createTime, updateTime, isDeleted FROM sys_notification_template_definition`
}

func templateRevisionSelectBase(r *Repository) string {
	return `SELECT id, templateDefinitionId, revisionNo, state, revisionVersion, COALESCE(subjectTemplate, '') subjectTemplate, COALESCE(textTemplate, '') textTemplate, COALESCE(htmlTemplate, '') htmlTemplate, COALESCE(markdownTemplate, '') markdownTemplate, COALESCE(primaryActionLabelTemplate, '') primaryActionLabelTemplate, ` + r.jsonText("variableSchemaJson") + ` variableSchemaJson, contentDigest, publishedAt, publishedBy, creatorId, updaterId, createTime, updateTime FROM sys_notification_template_revision`
}

func uniquePositiveRevisionIDs(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
