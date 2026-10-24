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

// ListSceneDefinitions reads only the additive G6.2 workspace. Legacy
// sys_notification_scene_binding records remain available through their existing
// endpoint and are never inferred here.
func (r *Repository) ListSceneDefinitions(ctx context.Context, query domain.SceneDefinitionQuery) ([]domain.SceneDefinition, int64, error) {
	where, args := []string{"isDeleted=0", "scopeId=?"}, []any{strings.TrimSpace(query.ScopeID)}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		keyword = "%" + keyword + "%"
		where = append(where, "(sceneCode LIKE ? OR sceneName LIKE ?)")
		args = append(args, keyword, keyword)
	}
	return selectInboxPage[domain.SceneDefinition](ctx, r, sceneDefinitionSelectBase(), where, "updateTime DESC, id DESC", query.Current, query.PageSize, args...)
}

func (r *Repository) FindSceneDefinitionByCodeAndReceiverKind(ctx context.Context, scopeID, sceneCode, receiverKind string) (*domain.SceneDefinition, error) {
	var item domain.SceneDefinition
	query := sceneDefinitionSelectBase() + " WHERE scopeId=? AND sceneCode=? AND receiverKind=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(scopeID), strings.TrimSpace(sceneCode), strings.ToUpper(strings.TrimSpace(receiverKind))); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindSceneDefinitionByID(ctx context.Context, definitionID int64) (*domain.SceneDefinition, error) {
	if definitionID <= 0 {
		return nil, nil
	}
	var item domain.SceneDefinition
	query := sceneDefinitionSelectBase() + " WHERE id=? AND isDeleted=0 LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), definitionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

// LockSceneDefinitionByCodeAndReceiverKind is used only under the service
// transaction boundary for clone/publish pointer changes.
func (r *Repository) LockSceneDefinitionByCodeAndReceiverKind(ctx context.Context, scopeID, sceneCode, receiverKind string) (*domain.SceneDefinition, error) {
	var item domain.SceneDefinition
	query := sceneDefinitionSelectBase() + " WHERE scopeId=? AND sceneCode=? AND receiverKind=? AND isDeleted=0 LIMIT 1 FOR UPDATE"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(scopeID), strings.TrimSpace(sceneCode), strings.ToUpper(strings.TrimSpace(receiverKind))); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) FindSceneRevisionByID(ctx context.Context, revisionID int64) (*domain.SceneRevision, error) {
	if revisionID <= 0 {
		return nil, nil
	}
	var item domain.SceneRevision
	query := sceneRevisionSelectBase() + " WHERE id=? LIMIT 1"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(r.inboxSQL(query)), revisionID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func (r *Repository) ListSceneRevisionsByIDs(ctx context.Context, revisionIDs []int64) ([]domain.SceneRevision, error) {
	revisionIDs = uniquePositiveRevisionIDs(revisionIDs)
	if len(revisionIDs) == 0 {
		return []domain.SceneRevision{}, nil
	}
	if len(revisionIDs) > 400 {
		return nil, fmt.Errorf("notification scene revision batch exceeds limit")
	}
	query, args, err := sqlx.In(sceneRevisionSelectBase()+" WHERE id IN (?) ORDER BY id ASC", revisionIDs)
	if err != nil {
		return nil, fmt.Errorf("build notification scene revision batch query: %w", err)
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	items := make([]domain.SceneRevision, 0, len(revisionIDs))
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), args...); err != nil {
		return nil, fmt.Errorf("list notification scene revisions by id: %w", err)
	}
	return items, nil
}

func (r *Repository) ListSceneRevisionsByDefinition(ctx context.Context, definitionID int64) ([]domain.SceneRevision, error) {
	if definitionID <= 0 {
		return []domain.SceneRevision{}, nil
	}
	items := make([]domain.SceneRevision, 0)
	query := sceneRevisionSelectBase() + " WHERE sceneDefinitionId=? ORDER BY revisionNo DESC, id DESC"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), definitionID); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) InsertSceneDefinition(ctx context.Context, item *domain.SceneDefinition) error {
	if item == nil || item.ID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.SceneCode) == "" || strings.TrimSpace(item.ReceiverKind) == "" {
		return fmt.Errorf("notification scene definition is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_scene_definition (id, scopeId, sceneCode, sceneName, receiverKind, currentDraftRevisionId, currentPublishedRevisionId, version, creatorId, updaterId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.ScopeID, item.SceneCode, item.SceneName, item.ReceiverKind, item.CurrentDraftRevisionID, item.CurrentPublishedRevisionID, item.Version, item.CreatorID, item.UpdaterID)
	return err
}

func (r *Repository) InsertSceneRevision(ctx context.Context, item *domain.SceneRevision) error {
	if item == nil || item.ID <= 0 || item.SceneDefinitionID <= 0 || item.RevisionNo <= 0 || strings.TrimSpace(item.State) == "" || item.TemplateRevisionID <= 0 {
		return fmt.Errorf("notification scene revision is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_scene_revision (id, sceneDefinitionId, revisionNo, state, revisionVersion, enabled, templateRevisionId, connectionRef, connectionDigest, publishedAt, publishedBy, creatorId, updaterId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.SceneDefinitionID, item.RevisionNo, item.State, item.RevisionVersion, item.Enabled, item.TemplateRevisionID, nullIfBlank(item.ConnectionRef), nullIfBlank(item.ConnectionDigest), item.PublishedAt, item.PublishedBy, item.CreatorID, item.UpdaterID)
	return err
}

func (r *Repository) UpdateSceneDefinitionMetadata(ctx context.Context, definitionID int64, sceneName string, actorID int64) error {
	if definitionID <= 0 || strings.TrimSpace(sceneName) == "" {
		return fmt.Errorf("notification scene definition metadata is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_scene_definition SET sceneName=?, updaterId=?, updateTime=NOW() WHERE id=? AND isDeleted=0`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), strings.TrimSpace(sceneName), actorID, definitionID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return domain.ErrSceneDefinitionNotFound
	}
	return nil
}

func (r *Repository) UpdateSceneRevisionDraft(ctx context.Context, item *domain.SceneRevision, expectedVersion int) (bool, error) {
	if item == nil || item.ID <= 0 || expectedVersion <= 0 {
		return false, fmt.Errorf("notification scene draft update is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_scene_revision SET enabled=?, templateRevisionId=?, connectionRef=?, connectionDigest=?, revisionVersion=revisionVersion+1, updaterId=?, updateTime=NOW() WHERE id=? AND state=? AND revisionVersion=?`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.Enabled, item.TemplateRevisionID, nullIfBlank(item.ConnectionRef), nullIfBlank(item.ConnectionDigest), item.UpdaterID, item.ID, domain.SceneRevisionStateDraft, expectedVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func (r *Repository) SetSceneDefinitionDraft(ctx context.Context, definitionID, revisionID int64, expectedDefinitionVersion int) (bool, error) {
	if definitionID <= 0 || revisionID <= 0 || expectedDefinitionVersion <= 0 {
		return false, fmt.Errorf("notification scene draft pointer is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `UPDATE sys_notification_scene_definition SET currentDraftRevisionId=?, version=version+1, updateTime=NOW() WHERE id=? AND version=? AND currentDraftRevisionId IS NULL AND isDeleted=0`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), revisionID, definitionID, expectedDefinitionVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// PublishSceneRevision atomically freezes the draft and advances the current
// pointer. The caller already holds the definition row lock; the conditional
// update remains a stale-writer fence.
func (r *Repository) PublishSceneRevision(ctx context.Context, definitionID, revisionID int64, expectedRevisionVersion int, actorID int64, publishedAt time.Time) (bool, error) {
	if definitionID <= 0 || revisionID <= 0 || expectedRevisionVersion <= 0 || publishedAt.IsZero() {
		return false, fmt.Errorf("notification scene publish is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	publishQuery := `UPDATE sys_notification_scene_revision SET state=?, revisionVersion=revisionVersion+1, publishedAt=?, publishedBy=?, updaterId=?, updateTime=NOW() WHERE id=? AND sceneDefinitionId=? AND state=? AND revisionVersion=?`
	result, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(publishQuery)), domain.SceneRevisionStatePublished, publishedAt, actorID, actorID, revisionID, definitionID, domain.SceneRevisionStateDraft, expectedRevisionVersion)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if _, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(`UPDATE sys_notification_scene_revision SET state=?, updateTime=NOW() WHERE sceneDefinitionId=? AND id<>? AND state=?`)), domain.SceneRevisionStateSuperseded, definitionID, revisionID, domain.SceneRevisionStatePublished); err != nil {
		return false, err
	}
	pointerQuery := `UPDATE sys_notification_scene_definition SET currentDraftRevisionId=NULL, currentPublishedRevisionId=?, version=version+1, updaterId=?, updateTime=NOW() WHERE id=? AND currentDraftRevisionId=? AND isDeleted=0`
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

func (r *Repository) InsertSceneRevisionAudit(ctx context.Context, item *domain.SceneRevisionAudit) error {
	if item == nil || item.ID <= 0 || item.SceneDefinitionID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.Action) == "" {
		return fmt.Errorf("notification scene revision audit is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_scene_revision_audit (id, sceneDefinitionId, scopeId, action, fromRevisionNo, toRevisionNo, errorCode, actorId) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.SceneDefinitionID, item.ScopeID, item.Action, item.FromRevisionNo, item.ToRevisionNo, nullIfBlank(item.ErrorCode), item.ActorID)
	return err
}

func (r *Repository) InsertSceneSnapshot(ctx context.Context, item *domain.SceneSnapshot) error {
	if item == nil || item.ID <= 0 || item.NotificationID <= 0 || strings.TrimSpace(item.ScopeID) == "" || strings.TrimSpace(item.SceneCode) == "" || strings.TrimSpace(item.ReceiverKind) == "" || item.SceneDefinitionID <= 0 || item.SceneRevisionID <= 0 || item.TemplateDefinitionID <= 0 || item.TemplateRevisionID <= 0 || strings.TrimSpace(item.TemplateContentDigest) == "" || strings.TrimSpace(item.RenderedDigest) == "" || strings.TrimSpace(item.VariableDigest) == "" || strings.TrimSpace(item.Resolution) == "" {
		return fmt.Errorf("notification scene snapshot is invalid")
	}
	exec := dbstore.SQLXExecutor(ctx, r.db)
	query := `INSERT INTO sys_notification_scene_snapshot (id, notificationId, scopeId, sceneCode, receiverKind, sceneDefinitionId, sceneRevisionId, templateDefinitionId, templateRevisionId, connectionRef, connectionDigest, templateContentDigest, renderedDigest, variableDigest, resolution) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := exec.ExecContext(ctx, exec.Rebind(r.inboxSQL(query)), item.ID, item.NotificationID, item.ScopeID, item.SceneCode, item.ReceiverKind, item.SceneDefinitionID, item.SceneRevisionID, item.TemplateDefinitionID, item.TemplateRevisionID, nullIfBlank(item.ConnectionRef), nullIfBlank(item.ConnectionDigest), item.TemplateContentDigest, item.RenderedDigest, item.VariableDigest, item.Resolution)
	return err
}

func (r *Repository) ListSceneSnapshotsByNotificationID(ctx context.Context, notificationID int64) ([]domain.SceneSnapshot, error) {
	if notificationID <= 0 {
		return []domain.SceneSnapshot{}, nil
	}
	items := make([]domain.SceneSnapshot, 0)
	query := sceneSnapshotSelectBase() + " WHERE notificationId=? ORDER BY id ASC"
	exec := dbstore.SQLXExecutor(ctx, r.db)
	if err := sqlx.SelectContext(ctx, exec, &items, exec.Rebind(r.inboxSQL(query)), notificationID); err != nil {
		return nil, err
	}
	return items, nil
}

func sceneDefinitionSelectBase() string {
	return `SELECT id, scopeId, sceneCode, sceneName, receiverKind, currentDraftRevisionId, currentPublishedRevisionId, version, creatorId, updaterId, createTime, updateTime, isDeleted FROM sys_notification_scene_definition`
}

func sceneRevisionSelectBase() string {
	return `SELECT id, sceneDefinitionId, revisionNo, state, revisionVersion, enabled, templateRevisionId, COALESCE(connectionRef, '') connectionRef, COALESCE(connectionDigest, '') connectionDigest, publishedAt, publishedBy, creatorId, updaterId, createTime, updateTime FROM sys_notification_scene_revision`
}

func sceneSnapshotSelectBase() string {
	return `SELECT id, notificationId, scopeId, sceneCode, receiverKind, sceneDefinitionId, sceneRevisionId, templateDefinitionId, templateRevisionId, COALESCE(connectionRef, '') connectionRef, COALESCE(connectionDigest, '') connectionDigest, templateContentDigest, renderedDigest, variableDigest, resolution, createTime, updateTime FROM sys_notification_scene_snapshot`
}
