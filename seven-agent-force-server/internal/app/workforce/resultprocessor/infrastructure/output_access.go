package infrastructure

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

// AvailableOutputRecord is internal to the Result Processor module. ObjectKey
// never crosses the facade boundary exposed to HTTP or other modules.
type AvailableOutputRecord struct {
	Output    facade.AvailableOutput
	ObjectKey string
}

func (s *Store) ListAvailableOutputs(ctx context.Context, tenantID, taskID int64) ([]AvailableOutputRecord, error) {
	rows, err := s.db.Query(ctx, availableOutputsSQL+` ORDER BY o.created_at,o.id LIMIT 64`, taskID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AvailableOutputRecord, 0)
	for rows.Next() {
		item, scanErr := scanAvailableOutput(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AvailableOutput(ctx context.Context, tenantID, taskID, outputID int64) (AvailableOutputRecord, error) {
	item, err := scanAvailableOutput(s.db.QueryRow(ctx, availableOutputsSQL+` AND o.id=$3`, taskID, tenantID, outputID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AvailableOutputRecord{}, ErrNotFound
	}
	if err != nil {
		return AvailableOutputRecord{}, err
	}
	return item, nil
}

// availableOutputsSQL deliberately reads exactly three physical table
// instances, which is the repository-wide SQL governance ceiling. Every
// Result Output/Artifact relation is revalidated here so corrupt or stale
// rows fail closed instead of exposing an object-store locator.
const availableOutputsSQL = `SELECT
o.id,o.task_id,o.name,o.output_kind,o.status,a.media_type,o.sha256,o.size_bytes,a.object_key
FROM workforce_result_outputs o
JOIN workforce_artifacts a
  ON a.id=o.artifact_id
 AND a.task_id=o.task_id
 AND a.attempt_id=o.attempt_id
 AND a.artifact_type='RESULT_OUTPUT'
 AND a.status='AVAILABLE'
 AND a.sha256=o.sha256
 AND a.size_bytes=o.size_bytes
JOIN workforce_tasks t ON t.id=o.task_id
WHERE o.task_id=$1
  AND t.tenant_id=$2
  AND o.storage='IMMEDIATE'
  AND o.status='AVAILABLE'
  AND length(o.sha256)=64
  AND o.size_bytes>=0
  AND a.object_key<>''`

type availableOutputScanner interface {
	Scan(dest ...any) error
}

func scanAvailableOutput(row availableOutputScanner) (AvailableOutputRecord, error) {
	var item AvailableOutputRecord
	err := row.Scan(
		&item.Output.ID,
		&item.Output.TaskID,
		&item.Output.Name,
		&item.Output.OutputKind,
		&item.Output.Status,
		&item.Output.MediaType,
		&item.Output.SHA256,
		&item.Output.SizeBytes,
		&item.ObjectKey,
	)
	return item, err
}
