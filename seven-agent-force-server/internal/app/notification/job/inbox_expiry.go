package job

import (
	"context"
	"fmt"
	"time"
)

// InboxExpiryService is the bounded application use case that makes due
// recipient projections visibly expire without deleting audit history.
type InboxExpiryService interface {
	ExpireInboxRecipients(ctx context.Context, limit int) (int, error)
}

// InboxExpiryJob periodically advances the mailbox sequence for due
// recipients. It intentionally delegates all locking and state mutation to the
// application service so the scheduler never bypasses domain rules.
type InboxExpiryJob struct {
	service InboxExpiryService
	spec    string
	limit   int
}

// NewInboxExpiryJob creates a bounded periodic expiry worker.
func NewInboxExpiryJob(service InboxExpiryService, intervalMs int, limit int) *InboxExpiryJob {
	if intervalMs <= 0 {
		intervalMs = 5000
	}
	seconds := intervalMs / int(time.Second/time.Millisecond)
	if seconds <= 0 {
		seconds = 5
	}
	if limit <= 0 {
		limit = 50
	}
	return &InboxExpiryJob{service: service, spec: fmt.Sprintf("@every %ds", seconds), limit: limit}
}

// Name returns the stable scheduler job name.
func (j *InboxExpiryJob) Name() string { return "notification_inbox_expiry" }

// Spec returns the scheduler interval.
func (j *InboxExpiryJob) Spec() string { return j.spec }

// Run processes at most the configured number of due recipients.
func (j *InboxExpiryJob) Run(ctx context.Context) error {
	if j == nil || j.service == nil {
		return nil
	}
	_, err := j.service.ExpireInboxRecipients(ctx, j.limit)
	return err
}
