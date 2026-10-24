package facade

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
)

// ErrInvalidTaskModelSelection marks only known caller selection failures.
// Transport adapters map it to a fixed safe response, never err.Error().
var ErrInvalidTaskModelSelection = errors.New("invalid task model selection")

// TaskSelectionValidator is the only model-credential contract needed by Task.
// It freezes an active catalog selection in the Task creation transaction.
type TaskSelectionValidator interface {
	ValidateTaskSelection(context.Context, int64, int64, string) (domain.RuntimeSelection, error)
	ValidateTaskSelectionInTransaction(context.Context, pgx.Tx, int64, int64, string) (domain.RuntimeSelection, error)
}

// ModelOption contains only configured selection metadata, never credentials.
type ModelOption = domain.ModelOption

// TaskModelCatalog reads the existing tenant-level task selection catalog.
// Integration Applications currently have no per-project or credential grants;
// callers must authenticate and validate their Project before exposing it.
// This query does not discover models upstream or inherit operator RBAC.
type TaskModelCatalog interface {
	ListTaskModelOptions(context.Context, int64) ([]ModelOption, error)
}
