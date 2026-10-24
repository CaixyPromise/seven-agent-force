// Package domain contains Task and Attempt lifecycle rules.
package domain

// TaskStatus is the durable business state of a coding task.
type TaskStatus string

const (
	TaskCreated              TaskStatus = "CREATED"
	TaskQueued               TaskStatus = "QUEUED"
	TaskPreparing            TaskStatus = "PREPARING"
	TaskRunning              TaskStatus = "RUNNING"
	TaskInteractionSuspended TaskStatus = "INTERACTION_SUSPENDED"
	TaskValidating           TaskStatus = "VALIDATING"
	TaskReviewing            TaskStatus = "REVIEWING"
	TaskPackaging            TaskStatus = "PACKAGING"
	TaskUploading            TaskStatus = "UPLOADING"
	TaskSucceeded            TaskStatus = "SUCCEEDED"
	TaskRetryWait            TaskStatus = "RETRY_WAIT"
	TaskCancelling           TaskStatus = "CANCELLING"
	TaskCancelled            TaskStatus = "CANCELLED"
	TaskTimedOut             TaskStatus = "TIMED_OUT"
	TaskManualReview         TaskStatus = "MANUAL_REVIEW"
	TaskFailed               TaskStatus = "FAILED"
)

const CompletionProposalAccepted = "ACCEPTED"

var allowedTransitions = map[TaskStatus]map[TaskStatus]struct{}{
	TaskCreated:              set(TaskQueued, TaskCancelled),
	TaskQueued:               set(TaskPreparing, TaskCancelling, TaskCancelled, TaskTimedOut, TaskFailed),
	TaskPreparing:            set(TaskRunning, TaskRetryWait, TaskCancelling, TaskTimedOut, TaskFailed),
	TaskRunning:              set(TaskInteractionSuspended, TaskValidating, TaskRetryWait, TaskCancelling, TaskTimedOut, TaskFailed),
	TaskInteractionSuspended: set(TaskQueued, TaskCancelled, TaskFailed),
	TaskValidating:           set(TaskRunning, TaskReviewing, TaskPackaging, TaskRetryWait, TaskCancelling, TaskTimedOut, TaskFailed),
	TaskReviewing:            set(TaskRunning, TaskPackaging, TaskManualReview, TaskCancelling, TaskCancelled, TaskFailed),
	TaskPackaging:            set(TaskUploading, TaskCancelling, TaskFailed),
	TaskUploading:            set(TaskReviewing, TaskSucceeded, TaskCancelling, TaskFailed),
	TaskRetryWait:            set(TaskQueued, TaskCancelling, TaskFailed),
	TaskCancelling:           set(TaskCancelled, TaskFailed),
}

// CanTransition reports whether the durable state machine permits a change.
// Packaging, upload and future delivery retries are tracked by their own
// entities and therefore do not transition a Task back to RETRY_WAIT.
func (s TaskStatus) CanTransition(next TaskStatus) bool {
	_, ok := allowedTransitions[s][next]
	return ok
}

// Terminal reports whether the Task cannot advance automatically.
func (s TaskStatus) Terminal() bool {
	switch s {
	case TaskSucceeded, TaskCancelled, TaskTimedOut, TaskManualReview, TaskFailed:
		return true
	default:
		return false
	}
}

func set(statuses ...TaskStatus) map[TaskStatus]struct{} {
	result := make(map[TaskStatus]struct{}, len(statuses))
	for _, status := range statuses {
		result[status] = struct{}{}
	}
	return result
}
