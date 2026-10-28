// Package facade exposes work-item ownership without exposing persistence.
package facade

import "context"

// TaskMembership identifies a task owned by a work item. Ordinary user-created
// continuation turns must be routed through that item's stage decisions.
type TaskMembership interface {
	IsWorkItemTask(context.Context, int64, int64) (bool, error)
}
