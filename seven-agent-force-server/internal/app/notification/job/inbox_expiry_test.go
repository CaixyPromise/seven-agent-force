package job

import (
	"context"
	"errors"
	"testing"
)

type inboxExpiryTestService struct {
	limit int
	err   error
}

func (s *inboxExpiryTestService) ExpireInboxRecipients(_ context.Context, limit int) (int, error) {
	s.limit = limit
	return 1, s.err
}

func TestInboxExpiryJobUsesBoundedApplicationService(t *testing.T) {
	service := &inboxExpiryTestService{}
	job := NewInboxExpiryJob(service, 0, 0)
	if job.Name() != "notification_inbox_expiry" || job.Spec() != "@every 5s" {
		t.Fatalf("job identity name=%q spec=%q", job.Name(), job.Spec())
	}
	if err := job.Run(context.Background()); err != nil || service.limit != 50 {
		t.Fatalf("job run err=%v limit=%d", err, service.limit)
	}
	service.err = errors.New("database unavailable")
	if err := job.Run(context.Background()); !errors.Is(err, service.err) {
		t.Fatalf("job error=%v, want propagated service error", err)
	}
}
