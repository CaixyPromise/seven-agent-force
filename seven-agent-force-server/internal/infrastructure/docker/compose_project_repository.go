package docker

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
)

type ComposeProjectRecord struct {
	ID                 int64          `db:"id"`
	ProjectID          string         `db:"projectId"`
	ProjectName        string         `db:"projectName"`
	WorkingDir         sql.NullString `db:"workingDir"`
	ConfigFilesJSON    sql.NullString `db:"configFilesJson"`
	ComposeYaml        sql.NullString `db:"composeYaml"`
	ComposeFilePath    sql.NullString `db:"composeFilePath"`
	FileManifestJSON   sql.NullString `db:"fileManifestJson"`
	Description        sql.NullString `db:"description"`
	Status             string         `db:"status"`
	LastPreviewJSON    sql.NullString `db:"lastPreviewJson"`
	LastValidationJSON sql.NullString `db:"lastValidationJson"`
	LastOperationID    sql.NullInt64  `db:"lastOperationId"`
	Source             string         `db:"source"`
	CreatedBy          sql.NullInt64  `db:"createdBy"`
	Deleted            int            `db:"deleted"`
	CreateTime         sql.NullTime   `db:"createTime"`
	UpdateTime         sql.NullTime   `db:"updateTime"`
}

type ComposeProjectRepository struct {
	db store.SQLX
}

func NewComposeProjectRepository(provider store.Provider) (*ComposeProjectRepository, error) {
	if provider == nil || provider.SQLX() == nil {
		return nil, fmt.Errorf("docker compose project repository requires datasource provider")
	}
	return &ComposeProjectRepository{db: provider.SQLX()}, nil
}

func (r *ComposeProjectRepository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *ComposeProjectRepository) List(ctx context.Context) ([]ComposeProjectRecord, error) {
	exec := r.executor(ctx)
	var rows []ComposeProjectRecord
	query := exec.Rebind(`
SELECT id, projectId, projectName, workingDir, configFilesJson, composeYaml, composeFilePath, fileManifestJson, description, status,
	lastPreviewJson, lastValidationJson, lastOperationId, source, createdBy, deleted, createTime, updateTime
FROM docker_compose_project
WHERE deleted = 0
ORDER BY updateTime DESC, id DESC
LIMIT 1001`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query); err != nil {
		return nil, fmt.Errorf("list docker compose projects: %w", err)
	}
	if len(rows) > 1000 {
		return nil, fmt.Errorf("managed docker compose project set exceeds 1000")
	}
	return rows, nil
}

func (r *ComposeProjectRepository) GetByProjectID(ctx context.Context, projectID string) (*ComposeProjectRecord, error) {
	exec := r.executor(ctx)
	var row ComposeProjectRecord
	query := exec.Rebind(`
SELECT id, projectId, projectName, workingDir, configFilesJson, composeYaml, composeFilePath, fileManifestJson, description, status,
	lastPreviewJson, lastValidationJson, lastOperationId, source, createdBy, deleted, createTime, updateTime
FROM docker_compose_project
WHERE projectId = ? AND deleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, strings.TrimSpace(projectID)); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get docker compose project: %w", err)
	}
	return &row, nil
}

func (r *ComposeProjectRepository) ProjectNameExists(ctx context.Context, projectName string, excludeProjectID string) (bool, error) {
	exec := r.executor(ctx)
	args := []any{strings.TrimSpace(projectName)}
	sqlText := `SELECT COUNT(1) FROM docker_compose_project WHERE deleted = 0 AND projectName = ?`
	if strings.TrimSpace(excludeProjectID) != "" {
		sqlText += ` AND projectId <> ?`
		args = append(args, strings.TrimSpace(excludeProjectID))
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(sqlText), args...); err != nil {
		return false, fmt.Errorf("count docker compose project by name: %w", err)
	}
	return count > 0, nil
}

func (r *ComposeProjectRepository) Insert(ctx context.Context, row ComposeProjectRecord) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO docker_compose_project (
	id, projectId, projectName, workingDir, configFilesJson, composeYaml, composeFilePath, fileManifestJson, description, status,
	lastPreviewJson, lastValidationJson, lastOperationId, source, createdBy, deleted, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NOW(), NOW())`),
		row.ID, row.ProjectID, row.ProjectName, nullableString(row.WorkingDir.String),
		nullableString(row.ConfigFilesJSON.String), nullableString(row.ComposeYaml.String),
		nullableString(row.ComposeFilePath.String), nullableString(row.FileManifestJSON.String),
		nullableString(row.Description.String), row.Status, nullableString(row.LastPreviewJSON.String),
		nullableString(row.LastValidationJSON.String), nullableInt64(row.LastOperationID.Int64),
		row.Source, nullableInt64(row.CreatedBy.Int64))
	if err != nil {
		return fmt.Errorf("insert docker compose project: %w", err)
	}
	return nil
}

func (r *ComposeProjectRepository) UpdateCompose(ctx context.Context, row ComposeProjectRecord) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE docker_compose_project
SET workingDir = ?, configFilesJson = ?, composeYaml = ?, composeFilePath = ?, fileManifestJson = ?,
	lastPreviewJson = ?, lastValidationJson = ?, status = ?, updateTime = NOW()
WHERE projectId = ? AND deleted = 0`),
		nullableString(row.WorkingDir.String), nullableString(row.ConfigFilesJSON.String),
		nullableString(row.ComposeYaml.String), nullableString(row.ComposeFilePath.String),
		nullableString(row.FileManifestJSON.String), nullableString(row.LastPreviewJSON.String),
		nullableString(row.LastValidationJSON.String), row.Status, row.ProjectID)
	if err != nil {
		return fmt.Errorf("update docker compose project yaml: %w", err)
	}
	return nil
}

func (r *ComposeProjectRepository) UpdateDiagnostics(ctx context.Context, projectID string, previewJSON, validationJSON, status string) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE docker_compose_project
SET lastPreviewJson = ?, lastValidationJson = ?, status = ?, updateTime = NOW()
WHERE projectId = ? AND deleted = 0`),
		nullableString(previewJSON), nullableString(validationJSON), strings.TrimSpace(status), strings.TrimSpace(projectID))
	if err != nil {
		return fmt.Errorf("update docker compose project diagnostics: %w", err)
	}
	return nil
}

func (r *ComposeProjectRepository) UpdateLastOperation(ctx context.Context, projectID string, operationID int64) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE docker_compose_project
SET lastOperationId = ?, updateTime = NOW()
WHERE projectId = ? AND deleted = 0`), nullableInt64(operationID), strings.TrimSpace(projectID))
	if err != nil {
		return fmt.Errorf("update docker compose project last operation: %w", err)
	}
	return nil
}
