package facade

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// ExternalProjection is a display-only fact owned by another Workforce module.
// Body must already be an allowlisted safe projection; commands, raw output,
// machine authority and secrets are prohibited.
type ExternalProjection struct {
	ID, TenantID, TaskID, TaskTurnID, AttemptID int64
	Version                                     int64
	Type, SourceKind, SourceKey, Digest         string
	Body                                        json.RawMessage
	AttentionRequired                           bool
	OccurredAt                                  time.Time
}

type ExternalProjector interface {
	ProjectExternalTx(context.Context, pgx.Tx, ExternalProjection) error
}
