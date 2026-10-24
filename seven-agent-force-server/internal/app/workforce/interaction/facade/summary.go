package facade

import "context"

type TaskSummaryReader interface {
	TaskSummaries(context.Context, int64, int64, []int64, bool) (map[int64]TaskSummary, error)
}
