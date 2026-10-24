package job

import (
	"context"
	"fmt"
	"time"
)

type OutboxRelayService interface {
	RelayOutbox(ctx context.Context, limit int) error
}

type OutboxRelayJob struct {
	service OutboxRelayService
	spec    string
	limit   int
}

func NewOutboxRelayJob(service OutboxRelayService, interval time.Duration, limit int) *OutboxRelayJob {
	if interval <= 0 {
		interval = time.Second
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return &OutboxRelayJob{service: service, spec: fmt.Sprintf("@every %ds", max(1, int(interval/time.Second))), limit: limit}
}

func (j *OutboxRelayJob) Name() string { return "cache_governance_outbox_relay" }
func (j *OutboxRelayJob) Spec() string { return j.spec }
func (j *OutboxRelayJob) Run(ctx context.Context) error {
	if j == nil || j.service == nil {
		return nil
	}
	return j.service.RelayOutbox(ctx, j.limit)
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
