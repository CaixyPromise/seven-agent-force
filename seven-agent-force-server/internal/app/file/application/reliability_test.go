package application

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/file/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestRelayOutboxDeadLettersUnknownOwnedEventsWhenRabbitIsDisabled(t *testing.T) {
	store := &unknownFileOutboxStore{}
	service := NewService(nil, nil, store, nil, nil, disabledFilePublisher{}, nil, config.FileDistributionConfig{}, true)

	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("RelayOutbox() error = %v", err)
	}
	if store.markedStatus != "DEAD" {
		t.Fatalf("unknown file outbox status=%q, want DEAD even when RabbitMQ is disabled", store.markedStatus)
	}
}

func TestRelayOutboxRejectsMalformedFileProcessPayloadWithoutPublishing(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{name: "invalid JSON", payload: "{"},
		{name: "missing task", payload: `{"fileId":9,"taskType":"thumbnail"}`},
		{name: "missing file", payload: `{"taskId":7,"taskType":"thumbnail"}`},
		{name: "missing type", payload: `{"taskId":7,"fileId":9}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &readyFileOutboxStore{event: domain.OutboxEvent{
				ID: 51, EventID: "file-event-51", EventType: "FILE_PROCESS_TASK", Payload: tt.payload,
			}}
			publisher := &recordingFilePublisher{}
			service := NewService(nil, nil, store, nil, nil, publisher, nil, config.FileDistributionConfig{}, true)

			if err := service.RelayOutbox(context.Background(), 1); err != nil {
				t.Fatalf("RelayOutbox() error = %v", err)
			}
			if publisher.fileProcessCalls != 0 {
				t.Fatalf("PublishFileProcessTask calls=%d, want 0", publisher.fileProcessCalls)
			}
			if store.markedStatus != "DEAD" {
				t.Fatalf("malformed file outbox status=%q, want DEAD", store.markedStatus)
			}
			if store.markedError == "" {
				t.Fatal("malformed file outbox error is empty")
			}
		})
	}
}

type unknownFileOutboxStore struct {
	OutboxPort
	markedStatus string
}

func (s *unknownFileOutboxStore) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{{ID: 41, EventType: "file.unknown", Status: "PENDING"}}, nil
}

func (s *unknownFileOutboxStore) TryClaimOutbox(context.Context, int64, string, string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "unknown-file-lease", Until: time.Now().Add(time.Minute)}, true, nil
}

func (s *unknownFileOutboxStore) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, _ string, _ int, _ *time.Time) (bool, error) {
	s.markedStatus = status
	return true, nil
}

type readyFileOutboxStore struct {
	OutboxPort
	event        domain.OutboxEvent
	markedStatus string
	markedError  string
}

func (s *readyFileOutboxStore) ListUnknownOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return nil, nil
}

func (s *readyFileOutboxStore) ListReadyOutbox(context.Context, int) ([]domain.OutboxEvent, error) {
	return []domain.OutboxEvent{s.event}, nil
}

func (s *readyFileOutboxStore) TryClaimOutbox(context.Context, int64, string, string) (*domain.OutboxLease, bool, error) {
	return &domain.OutboxLease{Token: "ready-file-lease", Until: time.Now().Add(time.Minute)}, true, nil
}

func (s *readyFileOutboxStore) MarkOutbox(_ context.Context, _ int64, _ string, _ string, status, lastError string, _ int, _ *time.Time) (bool, error) {
	s.markedStatus = status
	s.markedError = lastError
	return true, nil
}

type disabledFilePublisher struct{ MessagePublisherPort }

func (disabledFilePublisher) Enabled() bool { return false }

type recordingFilePublisher struct {
	MessagePublisherPort
	fileProcessCalls int
	err              error
}

func (p *recordingFilePublisher) Enabled() bool { return true }

func (p *recordingFilePublisher) PublishFileProcessTask(context.Context, domain.FileProcessMessage) error {
	p.fileProcessCalls++
	return p.err
}
