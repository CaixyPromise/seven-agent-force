package facade

import (
	"context"
	"encoding/json"
)

// UsageReader returns persisted facts for authorized Task IDs. Missing usage is
// explicit; monetary estimates and request limits are deliberately absent.
type UsageReader interface {
	UsageForTasks(context.Context, int64, []int64) (json.RawMessage, error)
}
