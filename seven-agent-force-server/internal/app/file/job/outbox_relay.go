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

func NewOutboxRelayJob(service OutboxRelayService, intervalMs int, limit int) *OutboxRelayJob {
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
	return &OutboxRelayJob{
		service: service,
		spec:    fmt.Sprintf("@every %ds", seconds),
		limit:   limit,
	}
}

func (j *OutboxRelayJob) Name() string {
	return "file_outbox_relay"
}

func (j *OutboxRelayJob) Spec() string {
	return j.spec
}

func (j *OutboxRelayJob) Run(ctx context.Context) error {
	if j == nil || j.service == nil {
		return nil
	}
	return j.service.RelayOutbox(ctx, j.limit)
}
